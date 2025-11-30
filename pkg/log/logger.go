/*
Copyright 2025 Jordi Gil.

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

// Package log provides a unified logging interface for all Kubernaut services.
//
// Authority: DD-005 v2.0 (Observability Standards - Unified Logging Interface)
//
// This package provides:
// - logr.Logger as the unified interface across all services
// - zap as the high-performance backend via zapr adapter
// - Consistent configuration for production and development
// - Helper functions for common logging patterns
//
// Usage:
//
// Stateless HTTP Services (Gateway, Data Storage, Context API):
//
//	logger := log.NewLogger(log.Options{
//	    Development: false,
//	    Level:       0, // INFO
//	})
//	defer log.Sync(logger)
//
//	// Pass to components
//	server := NewServer(cfg, logger.WithName("server"))
//	handler := NewHandler(logger.WithName("handler"))
//
// CRD Controllers:
//
//	// Use native logr from controller-runtime
//	logger := ctrl.Log.WithName("notification-controller")
//
//	// Pass to shared libraries (compatible interface)
//	auditStore := audit.NewBufferedStore(client, config, "notification", logger.WithName("audit"))
//
// Shared Libraries (pkg/*):
//
//	// Accept logr.Logger - works with both stateless services and CRD controllers
//	func NewComponent(logger logr.Logger) *Component {
//	    return &Component{logger: logger.WithName("component")}
//	}
package log

import (
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Options configures the logger behavior.
//
// DD-005: Standard configuration options for all services
type Options struct {
	// Development enables development mode with human-readable output.
	// Default: false (production mode with JSON output)
	Development bool

	// Level sets the minimum logging level.
	// DD-005 verbosity levels:
	// - 0 = INFO (default, always shown)
	// - 1 = DEBUG (shown when verbosity >= 1)
	// - 2 = TRACE (shown when verbosity >= 2)
	//
	// In production, use 0. In development, use 1 or 2.
	Level int

	// ServiceName is added to all log entries for identification.
	// Example: "gateway", "data-storage", "notification-controller"
	ServiceName string

	// DisableCaller disables caller information in log entries.
	// Default: false (caller info enabled)
	DisableCaller bool

	// DisableStacktrace disables stack traces for error-level logs.
	// Default: false (stack traces enabled for errors)
	DisableStacktrace bool
}

// DefaultOptions returns production-ready default options.
//
// DD-005: Production defaults
// - JSON output format
// - INFO level (V=0)
// - Caller info enabled
// - Stack traces enabled for errors
func DefaultOptions() Options {
	return Options{
		Development:       false,
		Level:             0,
		DisableCaller:     false,
		DisableStacktrace: false,
	}
}

// DevelopmentOptions returns development-friendly options.
//
// DD-005: Development defaults
// - Human-readable console output
// - DEBUG level (V=1)
// - Caller info enabled
// - Stack traces enabled for errors
func DevelopmentOptions() Options {
	return Options{
		Development:       true,
		Level:             1,
		DisableCaller:     false,
		DisableStacktrace: false,
	}
}

// NewLogger creates a new logr.Logger with the specified options.
//
// DD-005 v2.0: This is the primary way to create loggers for stateless services.
// CRD controllers should use ctrl.Log from controller-runtime instead.
//
// Example:
//
//	logger := log.NewLogger(log.Options{
//	    Development: false,
//	    Level:       0,
//	    ServiceName: "data-storage",
//	})
//	defer log.Sync(logger)
func NewLogger(opts Options) logr.Logger {
	zapLogger := newZapLogger(opts)
	logger := zapr.NewLogger(zapLogger)

	if opts.ServiceName != "" {
		logger = logger.WithName(opts.ServiceName)
	}

	return logger
}

// NewLoggerFromEnvironment creates a logger configured from environment variables.
//
// Environment variables:
// - LOG_LEVEL: "debug", "info", "warn", "error" (default: "info")
// - LOG_FORMAT: "json", "console" (default: "json")
// - SERVICE_NAME: Service identifier for logs
//
// DD-005: Environment-driven configuration for Kubernetes deployments
func NewLoggerFromEnvironment() logr.Logger {
	opts := DefaultOptions()

	// Parse LOG_LEVEL
	switch os.Getenv("LOG_LEVEL") {
	case "debug":
		opts.Level = 1
	case "trace":
		opts.Level = 2
	default:
		opts.Level = 0 // INFO
	}

	// Parse LOG_FORMAT
	if os.Getenv("LOG_FORMAT") == "console" {
		opts.Development = true
	}

	// Parse SERVICE_NAME
	if name := os.Getenv("SERVICE_NAME"); name != "" {
		opts.ServiceName = name
	}

	return NewLogger(opts)
}

// Sync flushes any buffered log entries.
//
// Call this before the application exits to ensure all logs are written.
// This is a best-effort operation - errors are ignored.
//
// Example:
//
//	logger := log.NewLogger(opts)
//	defer log.Sync(logger)
func Sync(logger logr.Logger) {
	// Extract the underlying zap logger and sync it
	if underlying, ok := logger.GetSink().(zapr.Underlier); ok {
		_ = underlying.GetUnderlying().Sync()
	}
}

// GetZapLogger extracts the underlying *zap.Logger from a logr.Logger.
//
// This is useful for interoperability with libraries that require *zap.Logger.
// Returns nil if the logger is not backed by zap.
//
// DD-005: Migration helper - use sparingly, prefer logr.Logger interface
//
// Example:
//
//	logger := log.NewLogger(opts)
//	zapLogger := log.GetZapLogger(logger)
//	if zapLogger != nil {
//	    // Use with legacy code that requires *zap.Logger
//	    legacyComponent := NewLegacyComponent(zapLogger)
//	}
func GetZapLogger(logger logr.Logger) *zap.Logger {
	if underlying, ok := logger.GetSink().(zapr.Underlier); ok {
		return underlying.GetUnderlying()
	}
	return nil
}

// newZapLogger creates the underlying zap logger with the specified options.
func newZapLogger(opts Options) *zap.Logger {
	var config zap.Config

	if opts.Development {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		config = zap.NewProductionConfig()
	}

	// Set log level based on verbosity
	// logr uses positive V levels, zap uses negative levels
	// V(0) = INFO = zap.InfoLevel
	// V(1) = DEBUG = zap.DebugLevel
	switch opts.Level {
	case 0:
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case 1:
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	default:
		// V(2+) = TRACE = zap.DebugLevel (zap doesn't have TRACE)
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	config.DisableCaller = opts.DisableCaller
	config.DisableStacktrace = opts.DisableStacktrace

	// DD-005: Standard timestamp format
	config.EncoderConfig.TimeKey = "ts"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := config.Build()
	if err != nil {
		// Fallback to no-op logger if configuration fails
		return zap.NewNop()
	}

	return logger
}

