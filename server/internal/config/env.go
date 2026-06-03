package config

import (
	"errors"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Env struct {
	DatabaseURL string
	Port        string
	LogLevel    string
	PoolMax     int32
	Billing     BillingEnv
}

// BillingEnv carries the metered-overage tunables. Defaults are production;
// override in dev to exercise the overage path without pushing 250k events
// (e.g. STARTER_INCLUDED_EVENTS=10, OVERAGE_UNIT_EVENTS=10). These MUST match
// the Node billing config (packages/billing/config) and the Stripe metered
// price tiers, or the API's 402 boundary and Stripe's invoice will disagree.
type BillingEnv struct {
	StarterIncludedEvents      int64 // included events/mo on Starter
	ProIncludedEvents          int64 // included events/mo on Pro
	OverageUnitEvents          int64 // billing unit size (events per priced unit)
	StarterOverageCentsPerUnit int64 // ¢ per unit over the Starter quota
	ProOverageCentsPerUnit     int64 // ¢ per unit over the Pro quota
	// No default spend cap: overage is uncapped (billed, never blocked) unless
	// the org owner opts into an explicit cap (org_billing_state).
}

func Load() (*Env, error) {
	_ = godotenv.Load(".env", ".env.local")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3002"
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	poolMax := int32(30)
	if v := os.Getenv("DATABASE_POOL_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			poolMax = int32(n)
		}
	}

	return &Env{
		DatabaseURL: dbURL,
		Port:        port,
		LogLevel:    logLevel,
		PoolMax:     poolMax,
		Billing: BillingEnv{
			StarterIncludedEvents:      int64Env("STARTER_INCLUDED_EVENTS", 250_000),
			ProIncludedEvents:          int64Env("PRO_INCLUDED_EVENTS", 5_000_000),
			OverageUnitEvents:          int64Env("OVERAGE_UNIT_EVENTS", 1_000),
			StarterOverageCentsPerUnit: int64Env("STARTER_OVERAGE_CENTS_PER_UNIT", 60),
			ProOverageCentsPerUnit:     int64Env("PRO_OVERAGE_CENTS_PER_UNIT", 25),
		},
	}, nil
}

// int64Env reads a non-negative integer env var, falling back to def.
func int64Env(name string, def int64) int64 {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			return n
		}
	}
	return def
}
