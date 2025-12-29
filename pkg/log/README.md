# Shared Logging Library

**Authority**: DD-005 v2.0 (Observability Standards - Unified Logging Interface)

## Overview

This package provides a unified logging interface for all Kubernaut services using:
- **`logr.Logger`** as the standard interface across all services
- **`zap`** as the high-performance backend (hidden inside this package)

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Your Code                                │
│                                                              │
│   logger.Info("Working", "key", "value")                    │
│              │                                               │
│              ▼                                               │
│   ┌─────────────────────┐                                   │
│   │   logr.Logger       │  ← Interface (what your code uses)│
│   │   (unified API)     │                                   │
│   └─────────┬───────────┘                                   │
│             │                                                │
│             ▼                                                │
│   ┌─────────────────────┐                                   │
│   │   zapr adapter      │  ← Converts logr calls to zap     │
│   │   (pkg/log only)    │                                   │
│   └─────────┬───────────┘                                   │
│             │                                                │
│             ▼                                                │
│   ┌─────────────────────┐                                   │
│   │   zap.Logger        │  ← High-performance backend       │
│   │   (hidden inside    │     (JSON output, zero-alloc)     │
│   │    pkg/log)         │                                   │
│   └─────────────────────┘                                   │
└─────────────────────────────────────────────────────────────┘
```

## Key Rule: When to Use What

| Scenario | What to Import | Example |
|----------|----------------|---------|
| **main.go** (create logger) | `kubelog "github.com/jordigilh/kubernaut/pkg/log"` | `logger := kubelog.NewLogger(opts)` |
| **Struct fields** | `"github.com/go-logr/logr"` | `logger logr.Logger` |
| **Function parameters** | `"github.com/go-logr/logr"` | `func New(logger logr.Logger)` |
| **Business code** | `"github.com/go-logr/logr"` | `logger.Info("message")` |

**⚠️ NEVER import `"go.uber.org/zap"` directly in business code.**

The `zap` backend is hidden inside `pkg/log`. Your code only sees `logr.Logger`.

## Why `logr.Logger`?

| Benefit | Description |
|---------|-------------|
| **Unified Interface** | Single `logr.Logger` type across stateless and CRD controller services |
| **controller-runtime Native** | CRD controllers use `logr` natively |
| **zap Performance** | High performance (zero-allocation) via `zapr` adapter |
| **Shared Library Consistency** | All `pkg/*` libraries accept `logr.Logger` |
| **Structured JSON Output** | Consistent format across all services |
| **Industry Standard** | `logr` is the Kubernetes ecosystem standard |

## Usage

### Stateless HTTP Services (Gateway, Data Storage, Context API)

```go
import "github.com/jordigilh/kubernaut/pkg/log"

func main() {
    // Create logger with production defaults
    logger := log.NewLogger(log.Options{
        Development: false,
        Level:       0, // INFO
        ServiceName: "data-storage",
    })
    defer log.Sync(logger)

    // Pass to components
    server := NewServer(cfg, logger.WithName("server"))
    handler := NewHandler(logger.WithName("handler"))
}
```

### CRD Controllers

```go
import (
    ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
    // Use native logr from controller-runtime
    logger := ctrl.Log.WithName("notification-controller")

    // Pass to shared libraries (compatible interface)
    auditStore := audit.NewBufferedStore(client, config, "notification", logger.WithName("audit"))
}
```

### Shared Libraries (pkg/*)

```go
import "github.com/go-logr/logr"  // ✅ Import interface only, NOT zap

type Component struct {
    logger logr.Logger  // ✅ Store interface, NOT *zap.Logger
}

// Accept logr.Logger - works with both stateless services and CRD controllers
func NewComponent(logger logr.Logger) *Component {
    return &Component{logger: logger.WithName("component")}
}

func (c *Component) DoWork() {
    // ✅ Use logr syntax (key-value pairs)
    c.logger.Info("Processing request",
        "key", "value",
        "count", 42,
    )
}
```

**⚠️ DO NOT do this in shared libraries:**
```go
import "go.uber.org/zap"  // ❌ WRONG - don't import zap directly

type Component struct {
    logger *zap.Logger    // ❌ WRONG - don't use zap type
}
```

### Environment-Based Configuration

```go
// Reads LOG_LEVEL, LOG_FORMAT, SERVICE_NAME from environment
logger := log.NewLoggerFromEnvironment()
```

Environment variables:
- `LOG_LEVEL`: "debug", "info", "warn", "error" (default: "info")
- `LOG_FORMAT`: "json", "console" (default: "json")
- `SERVICE_NAME`: Service identifier for logs

## Log Levels (DD-005)

| Level | V() | Use Case | Example |
|-------|-----|----------|---------|
| INFO | V(0) | Normal operational events | `logger.Info("Request processed")` |
| DEBUG | V(1) | Detailed debugging information | `logger.V(1).Info("Parsing payload")` |
| TRACE | V(2) | Very detailed tracing | `logger.V(2).Info("Field value", "field", value)` |
| ERROR | N/A | Error conditions (actionable) | `logger.Error(err, "Failed to process")` |

## Standard Fields

Use the constants in `fields.go` for consistent field names:

```go
import "github.com/jordigilh/kubernaut/pkg/log"

logger.Info("Request processed",
    log.KeyRequestID, requestID,
    log.KeyDurationMS, durationMs,
    log.KeyStatusCode, statusCode,
    log.KeyEndpoint, "/api/v1/workflows",
)
```

## Migration from `*zap.Logger`

### Before (old pattern)

```go
import "go.uber.org/zap"

type Component struct {
    logger *zap.Logger
}

func NewComponent(logger *zap.Logger) *Component {
    return &Component{logger: logger}
}

func (c *Component) DoWork() {
    c.logger.Info("Working",
        zap.String("key", "value"),
        zap.Int("count", 42),
    )
}
```

### After (new pattern)

```go
import "github.com/go-logr/logr"  // ✅ Interface only

type Component struct {
    logger logr.Logger  // ✅ Interface type
}

func NewComponent(logger logr.Logger) *Component {
    return &Component{logger: logger}
}

func (c *Component) DoWork() {
    // ✅ logr syntax: key-value pairs (zap runs underneath!)
    c.logger.Info("Working",
        "key", "value",
        "count", 42,
    )
}
```

**Note**: Even though you're using `logr.Logger`, the actual logging is done by `zap` under the hood. You get zap's performance without importing it directly.

### Key Differences

| Aspect | `*zap.Logger` | `logr.Logger` |
|--------|---------------|---------------|
| Field syntax | `zap.String("key", "value")` | `"key", "value"` |
| Error logging | `logger.Error("msg", zap.Error(err))` | `logger.Error(err, "msg")` |
| Debug logging | `logger.Debug("msg")` | `logger.V(1).Info("msg")` |
| Named loggers | `logger.Named("sub")` | `logger.WithName("sub")` |
| With fields | `logger.With(zap.String(...))` | `logger.WithValues("key", "value")` |

## Interoperability

If you need a `*zap.Logger` for legacy code:

```go
logger := log.NewLogger(opts)
zapLogger := log.GetZapLogger(logger)
if zapLogger != nil {
    legacyComponent := NewLegacyComponent(zapLogger)
}
```

## Files

- `logger.go` - Core logger creation and configuration
- `fields.go` - Standard field keys and values
- `README.md` - This documentation

## Related

- [DD-005 Observability Standards](../../docs/architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)
- [DD-014 Binary Version Logging](../../docs/architecture/decisions/DD-014-binary-version-logging-standard.md)

