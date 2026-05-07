package logging

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
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

// WithStandardFields enriches the logger with request-scoped context values
// (request_id, user_id, session_id) extracted from the context. The auth
// middleware must call WithUserID to propagate user identity for logging.
// Fields with empty values are omitted.
func WithStandardFields(ctx context.Context, logger logr.Logger) logr.Logger {
	if rid := requestid.FromContext(ctx); rid != "" {
		logger = logger.WithValues("request_id", rid)
	}
	if uid, ok := ctx.Value(userIDKey).(string); ok && uid != "" {
		logger = logger.WithValues("user_id", uid)
	}
	if sid, ok := ctx.Value(sessionIDKey).(string); ok && sid != "" {
		logger = logger.WithValues("session_id", sid)
	}
	return logger
}

type userIDKeyType struct{}
type sessionIDKeyType struct{}

var userIDKey = userIDKeyType{}
var sessionIDKey = sessionIDKeyType{}

// WithUserID attaches a user ID to the context for structured logging.
func WithUserID(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, userIDKey, uid)
}

// WithSessionID attaches a session ID to the context for structured logging.
func WithSessionID(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sid)
}

// NewSlogLogger creates a *slog.Logger that writes structured JSON to stderr
// and shares the dynamic log level from the given zap.AtomicLevel.
// This bridges components that require *slog.Logger (e.g. ADK) to the same
// logging pipeline as the rest of the service.
//
// NOTE for SRE: slog uses {"time":..., "level":"INFO"} while zap uses
// {"ts":..., "level":"info"}. Log aggregation parsers must handle both schemas.
func NewSlogLogger(level zap.AtomicLevel) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: &atomicLeveler{level: level},
	}))
}

// atomicLeveler adapts zap.AtomicLevel to slog.Leveler so that hot-reloads
// of the zap level are reflected in slog output without reconnection.
type atomicLeveler struct {
	level zap.AtomicLevel
}

// Level implements slog.Leveler by mapping the current zap level to slog.
func (a *atomicLeveler) Level() slog.Level {
	switch a.level.Level() {
	case zapcore.DebugLevel:
		return slog.LevelDebug
	case zapcore.WarnLevel:
		return slog.LevelWarn
	case zapcore.ErrorLevel:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
