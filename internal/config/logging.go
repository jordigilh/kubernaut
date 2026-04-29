package config

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LoggingConfig holds log-level configuration shared by all services.
// The config file is the single source of truth (no CLI flags).
//
// Rationale: ConfigMap-driven log level separates RBAC responsibility —
// changing log level requires configmaps write access, not deployments.
// Combined with hot-reload, level changes take effect without pod restarts.
type LoggingConfig struct {
	Level string `yaml:"level"` // debug, info, warn, error
}

// DefaultLoggingConfig returns production defaults.
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{Level: "info"}
}

// ValidLevels contains the accepted log level strings (lowercase canonical form).
var ValidLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// Validate checks that the configured level is recognised.
func (l LoggingConfig) Validate() error {
	if l.Level == "" {
		return nil
	}
	if !ValidLevels[strings.ToLower(l.Level)] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", l.Level)
	}
	return nil
}

// ZapLevel converts the configured level string to a zapcore.Level.
// Defaults to zapcore.InfoLevel for empty or unrecognised values.
func (l LoggingConfig) ZapLevel() zapcore.Level {
	switch strings.ToLower(l.Level) {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// NewAtomicLevel creates a zap.AtomicLevel set to the configured level.
// The returned AtomicLevel can be mutated at runtime for hot-reload.
func (l LoggingConfig) NewAtomicLevel() zap.AtomicLevel {
	return zap.NewAtomicLevelAt(l.ZapLevel())
}

// ParseAndSetLevel parses a level string and applies it to the given
// AtomicLevel. Returns an error if the level is invalid. This is the
// hot-reload callback helper: parse the new level from config, validate,
// then atomically update the running logger.
func ParseAndSetLevel(atomicLvl zap.AtomicLevel, level string) error {
	normalized := strings.ToLower(strings.TrimSpace(level))
	if normalized == "" {
		return nil
	}
	if !ValidLevels[normalized] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", level)
	}
	cfg := LoggingConfig{Level: normalized}
	atomicLvl.SetLevel(cfg.ZapLevel())
	return nil
}
