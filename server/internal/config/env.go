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
	// AuthEnabled gates API key authentication. Off by default: the server runs
	// open with a default tenant so a self-hoster can ingest immediately. Set
	// APIKEY_AUTH=true to require keys minted by the `stonewrit` CLI.
	AuthEnabled bool
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
		AuthEnabled: boolEnv("APIKEY_AUTH"),
	}, nil
}

// boolEnv reports whether an env var is set to a truthy value.
func boolEnv(name string) bool {
	switch os.Getenv(name) {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true
	default:
		return false
	}
}
