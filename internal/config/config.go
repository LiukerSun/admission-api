package config

import (
	platformconfig "admission-api/internal/platform/config"
)

// Deprecated: use admission-api/internal/platform/config instead.
type Config = platformconfig.Config

// Deprecated: use admission-api/internal/platform/config.Load instead.
func Load() (*Config, error) {
	return platformconfig.Load()
}
