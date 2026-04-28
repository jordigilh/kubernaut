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
	Level string `yaml:"level"` // DEBUG, INFO, WARN, ERROR
}

// DefaultLoggingConfig returns production defaults.
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{Level: "INFO"}
}

// ValidLevels contains the accepted log level strings.
var ValidLevels = map[string]bool{
	"DEBUG": true,
	"INFO":  true,
	"WARN":  true,
	"ERROR": true,
}

// Validate checks that the configured level is recognised.
func (l LoggingConfig) Validate() error {
	if l.Level == "" {
		return nil
	}
	if !ValidLevels[strings.ToUpper(l.Level)] {
		return fmt.Errorf("invalid log level: %s (must be DEBUG, INFO, WARN, or ERROR)", l.Level)
	}
	return nil
}

// ZapLevel converts the configured level string to a zapcore.Level.
// Defaults to zapcore.InfoLevel for empty or unrecognised values.
func (l LoggingConfig) ZapLevel() zapcore.Level {
	switch strings.ToUpper(l.Level) {
	case "DEBUG":
		return zapcore.DebugLevel
	case "WARN":
		return zapcore.WarnLevel
	case "ERROR":
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
	normalized := strings.ToUpper(strings.TrimSpace(level))
	if normalized == "" {
		return nil
	}
	if !ValidLevels[normalized] {
		return fmt.Errorf("invalid log level: %s (must be DEBUG, INFO, WARN, or ERROR)", level)
	}
	cfg := LoggingConfig{Level: normalized}
	atomicLvl.SetLevel(cfg.ZapLevel())
	return nil
}
