package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
)

// HashKey replicates Better Auth's @better-auth/api-key defaultKeyHasher:
// base64url-unpadded SHA-256 of the plaintext bearer token.
//
// Reference: @better-auth/api-key/dist/index.mjs
//
//	const defaultKeyHasher = async (key) => {
//	  const hash = await createHash("SHA-256").digest(new TextEncoder().encode(key));
//	  return base64Url.encode(new Uint8Array(hash), { padding: false });
//	};
//
// Lookup is then `SELECT FROM apikey WHERE key = HashKey(bearer)`.
func HashKey(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

var (
	ErrInvalidKey       = errors.New("api key invalid or revoked")
	ErrKeyExpired       = errors.New("api key expired")
	ErrKeyDisabled      = errors.New("api key disabled")
	ErrKeyNoEnvironment = errors.New("api key not bound to an environment")
)

type Verifier struct {
	Q *queries.Queries

	// lastStamped throttles last_used_at writes: apiKeyMetadataID → last write
	// time, per process. A busy key would otherwise issue a DB write on every
	// request; minute-granularity "last used" is plenty.
	lastStamped sync.Map
}

// lastUsedThrottle is the minimum gap between last_used_at writes for a key.
const lastUsedThrottle = time.Minute

// stampLastUsed records that a key just authenticated. Throttled (at most once
// per lastUsedThrottle per key per process) and async + best-effort: it never
// blocks or fails the request, and a dropped write just delays the displayed
// timestamp by a few minutes.
func (v *Verifier) stampLastUsed(metadataID pgtype.UUID) {
	if !metadataID.Valid {
		return
	}
	key := metadataID.String()
	now := time.Now()
	if last, ok := v.lastStamped.Load(key); ok {
		if now.Sub(last.(time.Time)) < lastUsedThrottle {
			return
		}
	}
	v.lastStamped.Store(key, now)

	go func() {
		// Detached, bounded context - the request's ctx is canceled when it
		// returns, which would abort this fire-and-forget write.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = v.Q.TouchApiKeyLastUsed(ctx, metadataID)
	}()
}

// Verify resolves a bearer token to an AuthContext or returns one of the
// typed errors above. Caller maps the error to HTTP 401/403.
func (v *Verifier) Verify(ctx context.Context, plaintextBearer string) (*Context, error) {
	hashed := HashKey(plaintextBearer)

	row, err := v.Q.LookupApiKeyByHash(ctx, hashed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidKey
	}
	if err != nil {
		return nil, err
	}

	// Better Auth's apikey.enabled is NOT NULL with default true - sqlc still
	// emits *bool because the inference is conservative. Treat nil as enabled.
	if row.BakEnabled != nil && !*row.BakEnabled {
		return nil, ErrKeyDisabled
	}
	if row.BakExpiresAt.Valid && row.BakExpiresAt.Time.Before(time.Now()) {
		return nil, ErrKeyExpired
	}
	if row.MetadataRevokedAt.Valid {
		return nil, ErrInvalidKey
	}
	if row.MetadataExpiresAt.Valid && row.MetadataExpiresAt.Time.Before(time.Now()) {
		return nil, ErrKeyExpired
	}
	if !row.EnvironmentID.Valid {
		return nil, ErrKeyNoEnvironment
	}

	envSlug := ""
	if row.EnvironmentSlug != nil {
		envSlug = *row.EnvironmentSlug
	}

	// Key authenticated successfully → record usage (throttled, async).
	v.stampLastUsed(row.ApiKeyMetadataID)

	return &Context{
		OrganizationID:   row.OrganizationID,
		ProjectID:        row.ProjectID.String(),
		EnvironmentID:    row.EnvironmentID.String(),
		EnvironmentSlug:  envSlug,
		BetterAuthKeyID:  row.BetterAuthKeyID,
		APIKeyMetadataID: row.ApiKeyMetadataID.String(),
		Scopes:           row.Scopes,
	}, nil
}
