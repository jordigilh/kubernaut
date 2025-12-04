# Gateway Service - Logging Migration Guide (Logrus → Zap)

**Status**: 📋 **DOCUMENTATION COMPLETE**
**Code Migration**: ⚠️ **PENDING** (Low Priority)
**Last Updated**: 2025-01-23

---

## 🎯 **Overview**

**Current State**: Gateway uses `logrus` (legacy)
**Target State**: Gateway should use `go.uber.org/zap` (standard)
**Reason**: Align with kubernaut logging standard for HTTP services

---

## 📊 **Migration Status**

### **Documentation** ✅
- [x] Updated `observability-logging.md` to show zap examples
- [x] Created migration guide
- [x] Documented best practices

### **Code Migration** ⚠️ (Pending)
- [ ] Update `pkg/gateway/server/server.go`
- [ ] Update `pkg/gateway/server/responses.go`
- [ ] Update `pkg/gateway/processing/*.go` (8 files)
- [ ] Update tests
- [ ] Remove logrus dependencies

**Estimated Effort**: 2-3 hours
**Priority**: LOW (Gateway is production-ready with current logging)

---

## 🔍 **Files Requiring Migration**

### **Production Code** (8 files)

```
pkg/gateway/
├── server/
│   ├── server.go              # Main server with logger
│   └── responses.go           # Response logging
└── processing/
    ├── redis_health.go        # Redis health checks
    ├── deduplication.go       # Deduplication logging
    ├── storm_detection.go     # Storm detection logging
    ├── crd_creator.go         # CRD creation logging
    ├── priority.go            # Priority engine logging
    └── remediation_path.go    # Path decision logging
```

### **Test Code** (TBD)

Test files may need updates if they mock or verify logging behavior.

---

## 📝 **Migration Steps**

### **Step 1: Update Dependencies**

```bash
# Add zap dependency (if not already present)
go get go.uber.org/zap

# Remove logrus dependency (after migration)
go mod tidy
```

### **Step 2: Update Server Initialization**

**Before** (`pkg/gateway/server/server.go`):
```go
import "github.com/sirupsen/logrus"

type Server struct {
    logger *logrus.Logger
    // ...
}

func NewServer(..., logger *logrus.Logger, ...) (*Server, error) {
    // ...
}
```

**After**:
```go
import "go.uber.org/zap"

type Server struct {
    logger *zap.Logger
    // ...
}

func NewServer(..., logger *zap.Logger, ...) (*Server, error) {
    // ...
}
```

### **Step 3: Update Logging Calls**

**Pattern 1: Simple Info Logging**

**Before**:
```go
s.logger.Info("Alert processed successfully")
```

**After**:
```go
s.logger.Info("Alert processed successfully")
```
*(No change for simple messages)*

---

**Pattern 2: Structured Logging**

**Before**:
```go
s.logger.WithFields(logrus.Fields{
    "fingerprint": fingerprint,
    "alertName":   alertName,
    "environment": environment,
    "priority":    priority,
}).Info("Alert processed")
```

**After**:
```go
s.logger.Info("Alert processed",
    zap.String("fingerprint", fingerprint),
    zap.String("alertName", alertName),
    zap.String("environment", environment),
    zap.String("priority", priority),
)
```

---

**Pattern 3: Error Logging**

**Before**:
```go
s.logger.WithError(err).Error("Failed to create CRD")
```

**After**:
```go
s.logger.Error("Failed to create CRD", zap.Error(err))
```

---

**Pattern 4: Contextual Logging**

**Before**:
```go
log := s.logger.WithFields(logrus.Fields{
    "request_id": requestID,
    "fingerprint": fingerprint,
})
log.Info("Processing alert")
log.Info("Deduplication check")
```

**After**:
```go
logger := s.logger.With(
    zap.String("request_id", requestID),
    zap.String("fingerprint", fingerprint),
)
logger.Info("Processing alert")
logger.Info("Deduplication check")
```

---

### **Step 4: Update Main Application**

**Before** (`cmd/gateway/main.go`):
```go
import "github.com/sirupsen/logrus"

func main() {
    logger := logrus.New()
    logger.SetFormatter(&logrus.JSONFormatter{})
    logger.SetLevel(logrus.InfoLevel)

    server := gateway.NewServer(..., logger, ...)
}
```

**After**:
```go
import "go.uber.org/zap"

func main() {
    logger, err := zap.NewProduction()
    if err != nil {
        panic(err)
    }
    defer logger.Sync()

    server := gateway.NewServer(..., logger, ...)
}
```

---

### **Step 5: Update Tests**

**Before**:
```go
import "github.com/sirupsen/logrus"

func TestSomething(t *testing.T) {
    logger := logrus.New()
    logger.SetLevel(logrus.ErrorLevel) // Quiet during tests

    server := NewServer(..., logger, ...)
}
```

**After**:
```go
import (
    "go.uber.org/zap"
    "go.uber.org/zap/zaptest"
)

func TestSomething(t *testing.T) {
    logger := zaptest.NewLogger(t) // Test logger

    server := NewServer(..., logger, ...)
}
```

---

## 🔄 **Field Type Conversion**

| Logrus | Zap | Example |
|--------|-----|---------|
| `"key": value` | `zap.String("key", value)` | String fields |
| `"key": 123` | `zap.Int("key", 123)` | Integer fields |
| `"key": true` | `zap.Bool("key", true)` | Boolean fields |
| `"key": duration` | `zap.Duration("key", duration)` | Duration fields |
| `"key": err` | `zap.Error(err)` | Error fields |
| `"key": time` | `zap.Time("key", time)` | Time fields |

---

## ✅ **Verification Checklist**

After migration, verify:

- [ ] All imports updated (`logrus` → `zap`)
- [ ] All logging calls converted
- [ ] Tests pass
- [ ] Log output is JSON formatted
- [ ] Log levels work correctly (debug, info, warn, error)
- [ ] Request IDs appear in logs
- [ ] Structured fields are correct
- [ ] No logrus imports remain
- [ ] `go.mod` cleaned up (`go mod tidy`)

---

## 🧪 **Testing the Migration**

### **Manual Testing**

```bash
# Run Gateway with debug logging
LOG_LEVEL=debug go run cmd/gateway/main.go

# Send test request
curl -X POST http://localhost:8080/webhook/prometheus \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d @test-alert.json

# Verify log output is JSON with zap format
```

### **Expected Log Output**

```json
{
  "level": "info",
  "timestamp": "2025-01-23T10:00:05.123Z",
  "msg": "Alert processed",
  "fingerprint": "abc123",
  "alertName": "HighMemoryUsage",
  "environment": "production",
  "priority": "P0"
}
```

---

## 🚨 **Common Pitfalls**

### **1. Forgetting to Call logger.Sync()**

```go
// ❌ WRONG - logs may be lost
func main() {
    logger, _ := zap.NewProduction()
    // ... use logger ...
}

// ✅ CORRECT - flush logs on exit
func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()
    // ... use logger ...
}
```

### **2. Using String Formatting**

```go
// ❌ WRONG - inefficient
logger.Info(fmt.Sprintf("Processing alert %s", alertName))

// ✅ CORRECT - structured field
logger.Info("Processing alert", zap.String("alertName", alertName))
```

### **3. Not Using Typed Fields**

```go
// ❌ WRONG - loses type information
logger.Info("Request took time", zap.String("duration", duration.String()))

// ✅ CORRECT - preserves type
logger.Info("Request took time", zap.Duration("duration", duration))
```

---

## 📊 **Performance Comparison**

### **Logrus** (Current)
- Allocations: 3-5 per log call
- Speed: ~2,500 ns/op
- Memory: ~1,200 B/op

### **Zap** (Target)
- Allocations: 0-1 per log call
- Speed: ~500 ns/op
- Memory: ~200 B/op

**Result**: **5x faster, 6x less memory** ✅

---

## 🎯 **Migration Priority**

### **Why LOW Priority?**

1. ✅ **Gateway is production-ready** with current logging
2. ✅ **Security is complete** (Day 8)
3. ✅ **Logging works correctly** with logrus
4. ✅ **No functional issues** with current implementation
5. ⚠️ **Migration is optimization**, not critical fix

### **When to Migrate?**

**Recommended Timing**:
- After Day 12 (Redis Security Documentation)
- During a dedicated refactoring sprint
- When adding new logging-heavy features
- Before performance optimization work

**Not Recommended**:
- During critical bug fixes
- Right before production deployment
- When time-constrained

---

## 📝 **Migration Estimate**

| Task | Estimated Time |
|------|----------------|
| Update server.go | 30 min |
| Update responses.go | 15 min |
| Update processing/*.go (8 files) | 60 min |
| Update tests | 30 min |
| Testing & verification | 30 min |
| **Total** | **2.5-3 hours** |

**Confidence**: 90%

---

## 🎉 **Post-Migration Benefits**

1. ✅ **Performance**: 5x faster logging
2. ✅ **Standards Compliance**: Aligns with kubernaut standard
3. ✅ **Type Safety**: Zero-allocation structured fields
4. ✅ **Maintainability**: Active development (vs logrus maintenance mode)
5. ✅ **Consistency**: All kubernaut services use zap

---

## 📚 **References**

- [Kubernaut Logging Standard](../../../architecture/LOGGING_STANDARD.md)
- [Zap Documentation](https://pkg.go.dev/go.uber.org/zap)
- [Zap Performance Benchmarks](https://github.com/uber-go/zap#performance)
- [Gateway Observability & Logging](observability-logging.md)

---

**Status**: 📋 **DOCUMENTATION COMPLETE**
**Next Step**: Schedule migration during refactoring sprint
**Priority**: LOW
**Confidence**: 90%


