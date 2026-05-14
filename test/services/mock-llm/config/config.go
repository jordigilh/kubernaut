/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package config

import (
	"fmt"
	"os"
	"strings"
)

// Mode constants define the behavioral modes for the Mock LLM service.
const (
	ModeInteractive = "interactive"
	ModeAutonomous  = "autonomous"
	ModeFull        = "full"
)

// validModes enumerates all accepted MOCK_LLM_MODE values.
var validModes = map[string]bool{
	ModeInteractive: true,
	ModeAutonomous:  true,
	ModeFull:        true,
}

// Config holds the Mock LLM server configuration loaded from environment variables.
type Config struct {
	Host      string
	Port      string
	ForceText bool
	Mode      string
	LogLevel  string

	ConfigPath    string
	RecordHeaders string
	GoldenDir     string
}

// LoadFromEnv reads configuration from environment variables with sensible defaults.
func LoadFromEnv() *Config {
	cfg := &Config{
		Host:          envOrDefault("MOCK_LLM_HOST", "0.0.0.0"),
		Port:          envOrDefault("MOCK_LLM_PORT", "8080"),
		ForceText:     parseBool(envOrDefault("MOCK_LLM_FORCE_TEXT", "false")),
		LogLevel:      envOrDefault("MOCK_LLM_LOG_LEVEL", "info"),
		ConfigPath:    os.Getenv("MOCK_LLM_CONFIG_PATH"),
		RecordHeaders: os.Getenv("MOCK_LLM_RECORD_HEADERS"),
		GoldenDir:     os.Getenv("MOCK_LLM_GOLDEN_DIR"),
	}
	cfg.Mode = ResolveMode(os.Getenv("MOCK_LLM_MODE"), cfg.ForceText)
	return cfg
}

// ResolveMode determines the effective mode from the explicit MOCK_LLM_MODE env
// var, falling back to a derivation from ForceText for backward compatibility.
// Returns an error-description string via ValidateMode if the resolved mode is
// not in the allowed set.
func ResolveMode(explicit string, forceText bool) string {
	if explicit != "" {
		return explicit
	}
	if forceText {
		return ModeAutonomous
	}
	return ModeFull
}

// ValidateMode checks that mode is one of the accepted values. Returns nil on
// success or a descriptive error for unknown modes.
func ValidateMode(mode string) error {
	if validModes[mode] {
		return nil
	}
	return fmt.Errorf("MOCK_LLM_MODE invalid: %q (must be %s|%s|%s)",
		mode, ModeInteractive, ModeAutonomous, ModeFull)
}

// ListenAddr returns the "host:port" string for net.Listen.
func (c *Config) ListenAddr() string {
	return c.Host + ":" + c.Port
}

func envOrDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func parseBool(s string) bool {
	return strings.EqualFold(s, "true") || s == "1"
}
