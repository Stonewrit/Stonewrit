// Package ratelimit ports apps/api/src/middleware/rate-limit.ts 1:1.
//
// Two windows per API key: burst (per-minute) and sustained (per-hour).
// Both windows floor `now_ms / interval_ms * interval_ms` so all servers
// agree on bucket boundaries without coordination. Atomic INSERT ... ON
// CONFLICT DO UPDATE keeps the increment server-side.
//
// Tier resolution caches per-org for 60s to avoid hammering subscription.
package ratelimit

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
)

type Tier string

const (
	// TierNone = no active/trialing subscription. NOT entitled - the ingest
	// path rejects with 402. This is the deliberate replacement for the old
	// "no subscription silently falls back to free" behavior.
	TierNone       Tier = "none"
	TierStarter    Tier = "starter"
	TierPro        Tier = "pro"
	TierEnterprise Tier = "enterprise"
)

type Bucket struct {
	Max        int32
	IntervalMs int64
}

type Limits struct {
	Burst     Bucket
	Sustained Bucket
}

// TierLimits - request-rate ceilings per tier (per API key). TierNone is
// omitted (no entry): callers treat a missing entry as "not entitled" and the
// ingest path rejects with 402 before rate limiting matters.
//
// These are abuse/DoS ceilings, NOT billing limits - overage above the included
// event allotment is metered, not blocked. With no free tier (Starter is paid),
// the limits are generous so legitimate spikes never hit them; sustained/hour
// comfortably exceeds each plan's included monthly volume averaged out, with
// large burst headroom on top.
var TierLimits = map[Tier]Limits{
	TierStarter: {
		Burst:     Bucket{Max: 600, IntervalMs: 60_000},       // 600/min
		Sustained: Bucket{Max: 50_000, IntervalMs: 3_600_000}, // 50k/hour
	},
	TierPro: {
		Burst:     Bucket{Max: 6_000, IntervalMs: 60_000},        // 6k/min
		Sustained: Bucket{Max: 1_000_000, IntervalMs: 3_600_000}, // 1M/hour
	},
	TierEnterprise: {
		Burst:     Bucket{Max: 120_000, IntervalMs: 60_000},       // 120k/min
		Sustained: Bucket{Max: 12_000_000, IntervalMs: 3_600_000}, // 12M/hour
	},
}

type Decision struct {
	Allowed            bool
	Tier               Tier
	Limits             Limits
	BurstUsed          int32
	SustainedUsed      int32
	BurstRemaining     int32
	SustainedRemaining int32
	DeniedReason       string // "burst" | "sustained" | ""
	RetryAfterMs       int64
}

type Limiter struct {
	Q         *queries.Queries
	tierCache sync.Map // org_id → cacheEntry
}

type cacheEntry struct {
	tier      Tier
	expiresAt time.Time
}

const tierCacheTTL = 60 * time.Second

// ResolveTier returns the highest tier the org has an active plan for,
// preferring cache if a fresh entry exists.
func (l *Limiter) ResolveTier(ctx context.Context, orgID string) (Tier, error) {
	if v, ok := l.tierCache.Load(orgID); ok {
		e := v.(cacheEntry)
		if time.Now().Before(e.expiresAt) {
			return e.tier, nil
		}
		l.tierCache.Delete(orgID)
	}

	rows, err := l.Q.ResolveOrgTier(ctx, orgID)
	if err != nil {
		return "", err
	}

	tier := pickHighest(rows)
	l.tierCache.Store(orgID, cacheEntry{
		tier:      tier,
		expiresAt: time.Now().Add(tierCacheTTL),
	})
	return tier, nil
}

func pickHighest(rows []queries.ResolveOrgTierRow) Tier {
	// Default DENY. No active/trialing subscription row → TierNone, which the
	// ingest path turns into a 402. This is the security-critical fix: we never
	// silently grant a free tier to unsubscribed orgs.
	best := TierNone
	rank := func(t Tier) int {
		switch t {
		case TierEnterprise:
			return 3
		case TierPro:
			return 2
		case TierStarter:
			return 1
		default:
			return 0
		}
	}
	for _, r := range rows {
		t := normalizePlan(r.Plan)
		if rank(t) > rank(best) {
			best = t
		}
	}
	return best
}

func normalizePlan(plan string) Tier {
	p := strings.ToLower(plan)
	switch {
	case strings.HasPrefix(p, "enterprise"):
		return TierEnterprise
	case strings.HasPrefix(p, "pro"):
		return TierPro
	case strings.HasPrefix(p, "starter"):
		return TierStarter
	default:
		return TierNone
	}
}

// Check resolves tier and increments both buckets atomically. Returns the
// decision; caller writes 429 if not allowed.
func (l *Limiter) Check(ctx context.Context, orgID string, apiKeyMetadataID pgtype.UUID) (*Decision, error) {
	tier, err := l.ResolveTier(ctx, orgID)
	if err != nil {
		return nil, err
	}

	// No subscription → don't rate-limit here. Let the request through so the
	// quota check in the accept path returns a clean 402 (subscription
	// required) rather than a misleading 429.
	if tier == TierNone {
		return &Decision{Allowed: true, Tier: TierNone}, nil
	}

	limits := TierLimits[tier]

	nowMs := time.Now().UnixMilli()
	burstWindow := windowStart(nowMs, limits.Burst.IntervalMs)
	sustainedWindow := windowStart(nowMs, limits.Sustained.IntervalMs)

	burstCount, err := l.Q.IncrementRateLimitBucket(ctx, queries.IncrementRateLimitBucketParams{
		ApiKeyMetadataID: apiKeyMetadataID,
		Bucket:           "burst",
		WindowStart:      pgtype.Timestamptz{Time: burstWindow, Valid: true},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	sustainedCount, err := l.Q.IncrementRateLimitBucket(ctx, queries.IncrementRateLimitBucketParams{
		ApiKeyMetadataID: apiKeyMetadataID,
		Bucket:           "sustained",
		WindowStart:      pgtype.Timestamptz{Time: sustainedWindow, Valid: true},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	d := &Decision{
		Allowed:            true,
		Tier:               tier,
		Limits:             limits,
		BurstUsed:          burstCount,
		SustainedUsed:      sustainedCount,
		BurstRemaining:     remaining(limits.Burst.Max, burstCount),
		SustainedRemaining: remaining(limits.Sustained.Max, sustainedCount),
	}

	if burstCount > limits.Burst.Max {
		d.Allowed = false
		d.DeniedReason = "burst"
		d.RetryAfterMs = (burstWindow.UnixMilli() + limits.Burst.IntervalMs) - nowMs
	} else if sustainedCount > limits.Sustained.Max {
		d.Allowed = false
		d.DeniedReason = "sustained"
		d.RetryAfterMs = (sustainedWindow.UnixMilli() + limits.Sustained.IntervalMs) - nowMs
	}
	return d, nil
}

func windowStart(nowMs, intervalMs int64) time.Time {
	return time.UnixMilli((nowMs / intervalMs) * intervalMs)
}

func remaining(limit, used int32) int32 {
	if used >= limit {
		return 0
	}
	return limit - used
}
