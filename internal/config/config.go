// Package config loads runtime configuration from environment variables with
// sensible defaults, so the service runs out of the box for local development.
package config

import "os"

// Config holds the server runtime configuration.
type Config struct {
	Port        string // HTTP port to listen on
	AllowOrigin string // CORS allowed origin
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		Port:        getenv("PERENCANAAN_PORT", "8082"),
		AllowOrigin: getenv("PERENCANAAN_ALLOW_ORIGIN", "*"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
