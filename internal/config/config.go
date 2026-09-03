package config

import (
	"os"
)

// Config holds emulator configuration values.
type Config struct {
	Port     string
	Storage  string
	LogLevel string
	DataDir  string
}

// Load loads the configuration from environment variables, fallback to defaults.
func Load() *Config {
	return &Config{
		Port:     getEnv("FLOCI_PORT", "4566"),
		Storage:  getEnv("FLOCI_STORAGE", "memory"),
		LogLevel: getEnv("FLOCI_LOG_LEVEL", "info"),
		DataDir:  getEnv("FLOCI_DATA_DIR", "./data"),
	}
}

func getEnv(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultValue
}
