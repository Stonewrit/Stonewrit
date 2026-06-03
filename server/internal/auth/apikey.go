package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
)

// HashKey returns the SHA-256 hex digest of a bearer token. Only the digest is
// ever stored; the token itself is shown once when the CLI mints it. The CLI
// and the server must agree on this function.
func HashKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

var ErrInvalidKey = errors.New("api key invalid or revoked")

type Verifier struct {
	Q *queries.Queries
}

// Verify resolves a bearer token to its scope, or returns ErrInvalidKey.
func (v *Verifier) Verify(ctx context.Context, token string) (*Context, error) {
	row, err := v.Q.LookupApiKeyByHash(ctx, HashKey(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidKey
	}
	if err != nil {
		return nil, err
	}

	return &Context{
		OrganizationID: row.OrganizationID,
		ProjectID:      uuid.UUID(row.ProjectID.Bytes).String(),
		EnvironmentID:  uuid.UUID(row.EnvironmentID.Bytes).String(),
		APIKeyID:       uuid.UUID(row.ID.Bytes).String(),
		Scopes:         row.Scopes,
	}, nil
}
