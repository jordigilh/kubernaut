package logging

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerKeyType struct{}

var loggerKey = loggerKeyType{}

// NewLogger creates a logr.Logger backed by zap with the given AtomicLevel.
// The AtomicLevel can be changed at runtime for hot-reload support (OPS-4).
func NewLogger(level zap.AtomicLevel) (logr.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.Level = level
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zapLogger, err := cfg.Build()
	if err != nil {
		return logr.Discard(), err
	}
	return zapr.NewLogger(zapLogger), nil
}

// WithLogger returns a context with the given logger attached.
func WithLogger(ctx context.Context, logger logr.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext extracts the logger from context, or returns a discard logger.
func FromContext(ctx context.Context) logr.Logger {
	if l, ok := ctx.Value(loggerKey).(logr.Logger); ok {
		return l
	}
	return logr.Discard()
}
