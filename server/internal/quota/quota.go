// Package quota enforces subscription entitlement + metered-overage spend caps
// at ingest time.
//
// Model (no free tier):
//   - No active/trialing subscription → ErrNoSubscription (HTTP 402).
//   - Within the tier's included monthly events → allow.
//   - Above included → metered overage: allow and let Stripe bill it, UNLESS
//     the owner has set a monthly overage SPEND cap that would be met/exceeded,
//     in which case ErrOverageLimitExceeded (HTTP 402). The cap is OPT-IN
//     (Vercel-style spend management); with no cap set, overage is uncapped -
//     events are never dropped, only billed, so audit evidence is never lost.
//   - Enterprise → unlimited, always allow.
//
// The Go service never talks to Stripe; it only enforces. A Node cron reports
// the metered usage to Stripe (see apps/web report-usage cron).
//
// Counts are cached briefly in-process so we don't COUNT(*) on every accept.
package quota

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stonewrit/stonewrit/server/internal/config"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/server/internal/ratelimit"
)

var (
	// ErrNoSubscription - org has no active/trialing subscription. 402.
	ErrNoSubscription = errors.New("no active subscription")
	// ErrOverageLimitExceeded - org is over its included quota AND has hit its
	// configured monthly overage spend cap. 402.
	ErrOverageLimitExceeded = errors.New("monthly overage spend limit reached")
	// ErrQuotaExceeded - retained for any residual hard-cap path. 402.
	ErrQuotaExceeded = errors.New("event volume quota exceeded for this billing period")
)

type Enforcer struct {
	Q       *queries.Queries
	Limiter *ratelimit.Limiter
	Billing config.BillingEnv

	capCache sync.Map // org_id → capEntry
}

type capEntry struct {
	limitCents  int64
	disabled    bool
	periodStart time.Time
	expiresAt   time.Time
}

const capCacheTTL = 60 * time.Second

// startOfMonthUTC is the fallback period anchor when the org has no
// cron-maintained period_start yet.
func startOfMonthUTC() time.Time {
	t := time.Now().UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// tierIncluded returns the included monthly event allotment for a tier, and
// whether the tier is metered (Starter/Pro) vs unlimited (Enterprise).
func (e *Enforcer) tierIncluded(t ratelimit.Tier) (included int64, metered bool, entitled bool) {
	switch t {
	case ratelimit.TierStarter:
		return e.Billing.StarterIncludedEvents, true, true
	case ratelimit.TierPro:
		return e.Billing.ProIncludedEvents, true, true
	case ratelimit.TierEnterprise:
		return 0, false, true // unlimited
	default: // TierNone
		return 0, false, false
	}
}

// overageCentsPerUnit returns the per-unit overage rate for a metered tier.
func (e *Enforcer) overageCentsPerUnit(t ratelimit.Tier) int64 {
	switch t {
	case ratelimit.TierStarter:
		return e.Billing.StarterOverageCentsPerUnit
	case ratelimit.TierPro:
		return e.Billing.ProOverageCentsPerUnit
	default:
		return 0
	}
}

// CheckAndIncrement decides whether to admit `delta` more events for an org.
func (e *Enforcer) CheckAndIncrement(ctx context.Context, orgID string, delta int64) error {
	tier, err := e.Limiter.ResolveTier(ctx, orgID)
	if err != nil {
		// Resolution failure (Postgres blip): fail SAFE-OPEN. Admit the event
		// but pin to Starter included semantics so a blip neither rejects
		// paying customers nor lets an unbounded flood through. The hourly
		// reconciler + Stripe metering still capture real overage later.
		tier = ratelimit.TierStarter
	}

	included, metered, entitled := e.tierIncluded(tier)
	if !entitled {
		return ErrNoSubscription
	}
	if !metered {
		return nil // Enterprise / unlimited
	}

	// Resolve the cap + the period anchor (cached). On read error, fail-open:
	// treat as uncapped and fall back to a calendar-month period.
	limitCents, disabled, periodStart, capErr := e.getCachedCap(ctx, orgID)
	if capErr != nil {
		disabled = true
		periodStart = startOfMonthUTC()
	}

	// Atomically increment the authoritative per-(org, period) counter and read
	// the running total back. This is exact across all ingest pods - unlike a
	// per-pod cache, it can't overshoot the cap; and being append-only, deleting
	// events can't lower it.
	projected, err := e.Q.BumpUsageCounter(ctx, queries.BumpUsageCounterParams{
		OrganizationID: orgID,
		PeriodStart:    pgtype.Timestamptz{Time: periodStart, Valid: true},
		AcceptedCount:  delta,
	})
	if err != nil {
		// Counter unavailable (DB blip) → admit; never block on a metering blip.
		return nil
	}

	// Within the included allotment, or no cap set → admit (already counted).
	if projected <= included || disabled {
		return nil
	}

	overageCents := e.overageSpendCents(tier, projected-included)
	if overageCents >= limitCents {
		// This accept is rejected, so roll its increment back out of the
		// counter - only ACCEPTED events should be counted/billed.
		_, _ = e.Q.BumpUsageCounter(ctx, queries.BumpUsageCounterParams{
			OrganizationID: orgID,
			PeriodStart:    pgtype.Timestamptz{Time: periodStart, Valid: true},
			AcceptedCount:  -delta,
		})
		return ErrOverageLimitExceeded
	}

	return nil
}

// overageSpendCents = ceil(overageEvents / unit) * centsPerUnit.
func (e *Enforcer) overageSpendCents(tier ratelimit.Tier, overageEvents int64) int64 {
	unit := e.Billing.OverageUnitEvents
	if unit <= 0 {
		unit = 1
	}
	units := (overageEvents + unit - 1) / unit // ceil
	return units * e.overageCentsPerUnit(tier)
}

// getCachedCap resolves the org's monthly overage cap (cents), whether the org
// is uncapped, and the current period anchor (the Stripe-aligned period_start
// the Node cron maintains, falling back to the calendar month). The cap is
// OPT-IN: with no billing-state row, the disabled flag, or a null limit, the
// org is uncapped - overage is billed but never blocked.
func (e *Enforcer) getCachedCap(ctx context.Context, orgID string) (int64, bool, time.Time, error) {
	if v, ok := e.capCache.Load(orgID); ok {
		entry := v.(capEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.limitCents, entry.disabled, entry.periodStart, nil
		}
		e.capCache.Delete(orgID)
	}

	// Defaults: uncapped, calendar-month period.
	limitCents := int64(0)
	disabled := true
	periodStart := startOfMonthUTC()

	row, err := e.Q.GetOrgBillingState(ctx, orgID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, periodStart, err
	}
	if err == nil {
		if !row.OverageLimitDisabled && row.OverageLimitCents != nil {
			limitCents = int64(*row.OverageLimitCents)
			disabled = false
		}
		if row.PeriodStart.Valid {
			periodStart = row.PeriodStart.Time
		}
	}

	e.capCache.Store(orgID, capEntry{
		limitCents:  limitCents,
		disabled:    disabled,
		periodStart: periodStart,
		expiresAt:   time.Now().Add(capCacheTTL),
	})
	return limitCents, disabled, periodStart, nil
}
