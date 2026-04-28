# Logging Standard - Kubernaut Services

**Version**: v2.0
**Last Updated**: April 27, 2026
**Status**: APPROVED & STANDARDIZED
**Scope**: All 10 Services
**Standard**: `go.uber.org/zap` via `pkg/log` (logr.Logger interface)
**Log Level Source**: Config file only (no CLI flags) -- Issue #875
**Hot-Reload**: Supported via `zap.AtomicLevel` + `FileWatcher`

---

## Official Standard: Config-File-Only Log Level (v2.0)

### v2.0 Changes (Issue #875)

| Aspect | v1.0 (Old) | v2.0 (Current) |
|--------|-----------|----------------|
| **Log level source** | CLI flags (`--zap-log-level`) for CRD controllers; hardcoded for HTTP services | Config file (`logging.level` in YAML ConfigMap) for ALL services |
| **Hot-reload** | Not supported | Supported via `zap.AtomicLevel` + `pkg/shared/hotreload/FileWatcher` |
| **RBAC impact** | Changing log level required Deployment edit (deployments write access) | Changing log level requires ConfigMap edit only (configmaps write access) |
| **Restart required** | Yes | No (hot-reload applies level change without pod restart) |
| **Shared type** | None (each service rolled its own) | `internal/config.LoggingConfig` with `ZapLevel()`, `NewAtomicLevel()`, `ParseAndSetLevel()` |

### Codebase Analysis

| Library | Files | Status | Action |
|---------|-------|--------|--------|
| **go.uber.org/zap** | 496+ files (99.8%) | STANDARD | APPROVED |
| **log/slog** | 0 files (migrated) | Removed | Completed (Issue #885) |

### Unified Strategy (v2.0)

All services use the same pattern regardless of service type:

1. **Config file** (`logging.level`) is the single source of truth
2. **`zap.AtomicLevel`** enables runtime level changes without logger recreation
3. **`pkg/shared/hotreload/FileWatcher`** watches the ConfigMap-mounted config file
4. **`internal/config.LoggingConfig`** provides shared parsing, validation, and level mapping

---

## Architecture

### Shared Type: `internal/config.LoggingConfig`

All services embed `internal/config.LoggingConfig` in their config struct:

```go
type LoggingConfig struct {
    Level string `yaml:"level"` // DEBUG, INFO, WARN, ERROR
}
```

Key methods:
- `ZapLevel()` -- converts to `zapcore.Level`
- `NewAtomicLevel()` -- creates a `zap.AtomicLevel` for runtime mutation
- `Validate()` -- rejects unrecognised level strings
- `SlogLevel()` -- bridge for services still using `log/slog` (kubernaut-agent)

### Hot-Reload Helper: `ParseAndSetLevel()`

```go
func ParseAndSetLevel(atomicLvl zap.AtomicLevel, level string) error
```

Validates the new level string and atomically updates the running logger.
Used as the callback body inside `FileWatcher`.

---

## Standard Pattern: CRD Controllers (AA, EM, SP, WE, RO)

```go
// 1. Bootstrap at INFO before config is loaded
atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
ctrl.SetLogger(zap.New(zap.Level(atomicLevel)))

// 2. Load config from YAML file
cfg, err := config.LoadFromFile(configPath)

// 3. Apply config-driven level
atomicLevel.SetLevel(cfg.Logging.ZapLevel())

// 4. Hot-reload watcher (before mgr.Start)
logLevelWatcher, _ := hotreload.NewFileWatcher(configPath, func(content string) error {
    var partial struct { Logging internalconfig.LoggingConfig `yaml:"logging"` }
    yaml.Unmarshal([]byte(content), &partial)
    return internalconfig.ParseAndSetLevel(atomicLevel, partial.Logging.Level)
}, setupLog.WithName("log-level-watcher"))
logLevelWatcher.Start(ctx)
```

No CLI flags (`--zap-log-level`, `--zap-devel`, etc.) are used. The config file is the single source of truth.

---

## Standard Pattern: kubelog Services (Gateway, Notification, DataStorage)

```go
// 1. Bootstrap logger with AtomicLevel
bootstrapLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
logger := kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
    ServiceName: "gateway",
}, bootstrapLevel)

// 2. Load config and apply level
cfg, _ := config.LoadFromFile(configPath)
atomicLevel := cfg.Logging.NewAtomicLevel()
logger = kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
    ServiceName: "gateway",
}, atomicLevel)

// 3. Hot-reload watcher (same pattern as CRD controllers)
```

### `pkg/log.NewLoggerWithAtomicLevel()`

Added in v2.0 (Issue #875). Creates a `logr.Logger` backed by zap whose level is
controlled by the caller-provided `zap.AtomicLevel`. Mutations to the `AtomicLevel`
take effect immediately on all log calls.

---

## AuthWebhook

Same pattern as CRD controllers, but uses `zap.New(zap.Level(atomicLevel))` directly
(no `BindFlags`, no `UseFlagOptions`). Previously hardcoded `zap.UseDevMode(true)`.

---

## Kubernaut Agent

Fully migrated from `log/slog` to `pkg/log` (zap-backed `logr.Logger`) as part of
Issue #885. Uses `kubelog.NewLoggerWithAtomicLevel()` with the shared
`internal/config.LoggingConfig` for config-driven log level and `FileWatcher`
hot-reload, identical to the gateway/notification/datastorage pattern.

---

## Helm Chart Configuration

All services expose `logging.level` in their Helm values:

```yaml
gateway:
  logging:
    level: "INFO"

aianalysis:
  logging:
    level: "INFO"
```

The level is rendered into each service's ConfigMap. Changing the value and
performing `helm upgrade` (or editing the ConfigMap directly) triggers a
hot-reload -- no pod restart required.

Valid values: `DEBUG`, `INFO`, `WARN`, `ERROR`. Default: `INFO`.

---

## 🎯 **Structured Logging Best Practices**

### **1. Use Structured Fields** (Type-Safe)

```go
// ✅ CORRECT: Structured logging with type-safe fields
logger.Info("User action",
    zap.String("user_id", userID),
    zap.String("action", "login"),
    zap.String("ip", remoteIP),
    zap.Duration("response_time", duration),
    zap.Int("status_code", 200),
)

// ❌ WRONG: String interpolation (loses structure, poor performance)
logger.Info(fmt.Sprintf("User %s performed action %s from %s", userID, "login", remoteIP))
```

**Why Structured Logging?**
- ✅ **Queryable**: Log aggregation tools can search by field
- ✅ **Type-safe**: Compile-time error detection
- ✅ **Performance**: Zero-allocation fields
- ✅ **Consistent**: Standardized field names

---

### **2. Log Levels** (Use Appropriately)

| Level | Use Case | Example | When to Use |
|-------|----------|---------|-------------|
| **Debug** | Detailed diagnostic info | Function entry/exit, variable values | Development only |
| **Info** | General informational | Request received, operation completed | Normal operation |
| **Warn** | Non-critical issues | Deprecated API usage, fallback triggered | Potential problems |
| **Error** | Error conditions | Operation failed, external service error | Recoverable errors |
| **DPanic** | Panic in development | Critical bug detected | Development assertions |
| **Panic** | Panic immediately | Unrecoverable error | Critical failures |
| **Fatal** | Log + os.Exit(1) | Application cannot continue | Startup failures |

**Example**:
```go
// Debug (development only)
logger.Debug("Entering function",
    zap.String("function", "ProcessSignal"),
    zap.Any("args", args),
)

// Info (normal operation)
logger.Info("Request processed",
    zap.String("path", "/api/v1/signals"),
    zap.Int("status", 200),
    zap.Duration("latency", 45*time.Millisecond),
)

// Warn (potential issue)
logger.Warn("Using fallback provider",
    zap.String("primary", "redis"),
    zap.String("fallback", "memory"),
    zap.Error(redisErr),
)

// Error (recoverable error)
logger.Error("Database query failed",
    zap.Error(err),
    zap.String("query", query),
    zap.String("table", "incident_embeddings"),
)

// Fatal (unrecoverable)
logger.Fatal("Failed to connect to database",
    zap.Error(err),
    zap.String("host", dbHost),
)
```

---

### **3. Error Logging** (With Context)

```go
// ✅ CORRECT: Error with full context
logger.Error("Database query failed",
    zap.Error(err),
    zap.String("query", query),
    zap.String("table", tableName),
    zap.Duration("duration", queryDuration),
    zap.String("correlation_id", correlationID),
)

// ✅ CORRECT: Error with stack trace (development)
logger.DPanic("Critical error",
    zap.Error(err),
    zap.Stack("stacktrace"),
)

// ❌ WRONG: Error without context
logger.Error("Query failed", zap.Error(err))
```

**Why Context Matters**:
- ✅ Easier debugging (know what failed)
- ✅ Better root cause analysis
- ✅ Correlation across services
- ✅ Performance insights

---

### **4. Correlation IDs** (Distributed Tracing)

**Integration**: See [LOG_CORRELATION_ID_STANDARD.md](./LOG_CORRELATION_ID_STANDARD.md)

```go
// pkg/correlation/correlation.go
package correlation

import "context"

type correlationIDKey struct{}

func NewCorrelationID() string {
    return fmt.Sprintf("req-%s-%s",
        time.Now().Format("20060102150405"),
        uuid.New().String()[:8])
}

func WithCorrelationID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, correlationIDKey{}, id)
}

func FromContext(ctx context.Context) string {
    if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
        return id
    }
    return "unknown"
}
```

**Usage**:
```go
// Extract correlation ID from context
correlationID := correlation.FromContext(ctx)

logger.Info("Processing request",
    zap.String("correlation_id", correlationID),
    zap.String("path", r.URL.Path),
    zap.String("method", r.Method),
)
```

---

### **5. Performance Considerations**

```go
// ✅ CORRECT: Check log level before expensive operations
if logger.Core().Enabled(zap.DebugLevel) {
    expensiveData := computeExpensiveData()
    logger.Debug("Expensive debug data",
        zap.Any("data", expensiveData),
    )
}

// ✅ CORRECT: Use Logger for structured (fastest)
logger.Info("User logged in",
    zap.String("username", username),
    zap.String("email", email),
)

// ⚠️ ACCEPTABLE: Use SugaredLogger for printf-style (slightly slower)
sugar := logger.Sugar()
sugar.Infof("User %s logged in from %s", username, ip)
defer sugar.Sync()

// ❌ WRONG: Format in hot path
logger.Info(fmt.Sprintf("User %s logged in", username))
```

**Performance Tips**:
- ✅ Use `logger.Core().Enabled()` for expensive debug logs
- ✅ Reuse logger instances (don't create per-request)
- ✅ Use type-specific fields (`zap.String()`, not `zap.Any()`)
- ⚠️ `SugaredLogger` is convenient but 10-20% slower

---

## 📦 **Common Field Names** (Standardized)

### **Standard Fields**

| Field Name | Type | Description | Example |
|------------|------|-------------|---------|
| `correlation_id` | string | Request tracing ID | `req-20251006-abc123` |
| `user_id` | string | User identifier | `user-12345` |
| `namespace` | string | Kubernetes namespace | `production` |
| `service` | string | Service name | `gateway` |
| `component` | string | Component name | `signal-processor` |
| `operation` | string | Operation name | `process_signal` |
| `duration` | duration | Operation duration | `45ms` |
| `status_code` | int | HTTP status code | `200` |
| `error` | error | Error object | `err` |
| `path` | string | HTTP path | `/api/v1/signals` |
| `method` | string | HTTP method | `POST` |

**Example**:
```go
logger.Info("Signal processed",
    zap.String("correlation_id", correlationID),
    zap.String("service", "gateway"),
    zap.String("component", "signal-processor"),
    zap.String("operation", "process_signal"),
    zap.String("namespace", namespace),
    zap.Duration("duration", duration),
    zap.Int("status_code", 200),
)
```

---

---

## Quick Reference: Service Logging Matrix

| Service | Logger Type | Config Import | Hot-Reload | Status |
|---------|------------|--------------|------------|--------|
| **AI Analysis** | controller-runtime/zap | `internal/config` | FileWatcher | Done (Issue #875) |
| **Effectiveness Monitor** | controller-runtime/zap | `internal/config` | FileWatcher | Done (Issue #875) |
| **Signal Processing** | controller-runtime/zap | `internal/config` | FileWatcher | Done (Issue #875) |
| **Workflow Execution** | controller-runtime/zap | `internal/config` | FileWatcher | Done (Issue #875) |
| **Remediation Orchestrator** | controller-runtime/zap | `internal/config` | FileWatcher | Done (Issue #875) |
| **AuthWebhook** | controller-runtime/zap | `internal/config` | FileWatcher | Done (Issue #876) |
| **Gateway** | pkg/log (kubelog) | `internal/config` | FileWatcher | Done (Issue #877) |
| **Notification** | pkg/log (kubelog) | `internal/config` | FileWatcher | Done (Issue #878) |
| **DataStorage** | pkg/log (kubelog) | `internal/config` | FileWatcher | Done (Issue #875) |
| **Kubernaut Agent** | pkg/log (kubelog) | `internal/config` | FileWatcher | Done (Issue #885) |

---

## Logging Standard Checklist (v2.0)

### For ALL Services (v2.0 mandatory):

1. Log level read from config file (`logging.level` in YAML)
2. No CLI flags for log level (`--zap-log-level` removed)
3. `internal/config.LoggingConfig` embedded in service config struct
4. `zap.AtomicLevel` used for runtime-mutable log level
5. `FileWatcher` registered to watch config path for hot-reload
6. Bootstrap logger at INFO before config load, re-configure after load
7. Helm `values.yaml` exposes `<service>.logging.level` with default `"INFO"`
8. `values.schema.json` validates level is one of `DEBUG|INFO|WARN|ERROR`

### For Structured Logging (unchanged from v1.0):

1. Use type-safe fields (`zap.String()`, `zap.Int()`, etc.)
2. Include correlation IDs in all request-scoped logs
3. Include error context: `log.Error(err, "message", "key", value)`
4. Check log level before expensive debug operations
5. Use standardized field names (see Common Field Names below)
6. `defer kubelog.Sync(logger)` or `defer logger.Sync()` on exit

---

## Related Documentation

- [LOG_CORRELATION_ID_STANDARD.md](./LOG_CORRELATION_ID_STANDARD.md) - Correlation ID patterns
- [ERROR_RESPONSE_STANDARD.md](./ERROR_RESPONSE_STANDARD.md) - Error handling
- [OPERATIONAL_STANDARDS.md](./OPERATIONAL_STANDARDS.md) - Operational best practices

---

## Decision Summary

### Approved Standard: Config-File-Only Log Level + Hot-Reload (v2.0)

**Issue**: #875 (hardcoded log levels), #876 (authwebhook), #877 (gateway), #878 (notification)

**Rationale**:
1. RBAC separation: ConfigMap edit (low privilege) vs Deployment edit (high privilege)
2. No pod restart needed for log level changes
3. Single source of truth for all services (`logging.level` in config YAML)
4. `zap.AtomicLevel` is thread-safe and zero-allocation

**Migration**:
- All 10 services fully migrated to config-file-only log level with hot-reload
- kubernaut-agent: fully migrated from log/slog to pkg/log (zap-backed logr.Logger)

---

**Document Status**: Complete & Approved
**Standard**: `go.uber.org/zap` via config file
**Last Updated**: April 27, 2026
**Version**: 2.0