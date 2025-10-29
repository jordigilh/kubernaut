# Day 1 CRITICAL ISSUE: Gateway Using Wrong Logging Framework

**Date**: October 28, 2025
**Severity**: 🔴 **CRITICAL** - Architectural Standard Violation
**Impact**: Gateway service violates project-wide logging standard

---

## 🚨 Issue Summary

**Problem**: Gateway service uses `github.com/sirupsen/logrus` instead of project standard `go.uber.org/zap`

**Evidence**:
- Gateway files using logrus: 8+ files
- Project standard: `go.uber.org/zap` (99.8% of codebase - 496/497 files)
- Official standard: [docs/architecture/LOGGING_STANDARD.md](docs/architecture/LOGGING_STANDARD.md)

---

## 📊 Impact Analysis

### Affected Files
```
pkg/gateway/server.go                                    ❌ logrus
pkg/gateway/processing/classification.go                 ❌ logrus
pkg/gateway/processing/crd_creator.go                    ❌ logrus
pkg/gateway/processing/deduplication.go                  ❌ logrus
pkg/gateway/processing/priority.go                       ❌ logrus
pkg/gateway/processing/remediation_path.go               ❌ logrus
pkg/gateway/processing/redis_health.go                   ❌ logrus
pkg/gateway/adapters/registry.go                         ❌ logrus
```

### Why This Matters

1. **Performance**: Logrus is 5x slower than Zap (2,500 ns/op vs 500 ns/op)
2. **Maintenance**: Logrus is in maintenance mode (no new features)
3. **Consistency**: Gateway is the ONLY service using logrus
4. **Technical Debt**: Using deprecated library
5. **Industry Standard**: Community migrating away from logrus

---

## 📋 Project Logging Standard

From [LOGGING_STANDARD.md](docs/architecture/LOGGING_STANDARD.md):

### Split Strategy by Service Type

| Service Type | Standard Import | Rationale |
|--------------|----------------|-----------|
| **CRD Controllers** | `sigs.k8s.io/controller-runtime/pkg/log/zap` | Official integration, Kubernetes flags, logr.Logger interface |
| **HTTP Services** | `go.uber.org/zap` | Full control, consistent configuration, advanced features |
| **Background Workers** | `go.uber.org/zap` | Advanced features (sampling, batching, custom encoders) |

**Gateway Service Type**: HTTP Service → **Should use `go.uber.org/zap`**

---

## ✅ Correct Pattern for Gateway

### What Gateway SHOULD Use

```go
// pkg/gateway/server.go
package gateway

import (
    "go.uber.org/zap"
)

type Server struct {
    logger *zap.Logger  // ✅ CORRECT
    // ... other fields
}

func NewServer(logger *zap.Logger, ...) *Server {
    return &Server{
        logger: logger,
        // ...
    }
}

// Usage
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) error {
    s.logger.Info("Processing signal",
        zap.String("fingerprint", signal.Fingerprint),
        zap.String("alertName", signal.AlertName),
        zap.String("namespace", signal.Namespace),
    )
    // ...
}
```

### What Gateway Currently Uses (WRONG)

```go
// pkg/gateway/server.go
package gateway

import (
    "github.com/sirupsen/logrus"  // ❌ WRONG
)

type Server struct {
    logger *logrus.Logger  // ❌ WRONG
    // ... other fields
}

func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) error {
    s.logger.WithFields(logrus.Fields{  // ❌ WRONG
        "fingerprint": signal.Fingerprint,
        "alertName":   signal.AlertName,
    }).Info("Processing signal")
    // ...
}
```

---

## 🔧 Migration Required

### Scope
- **Files to migrate**: 8+ implementation files
- **Test files**: Unknown (need to check)
- **Estimated effort**: 2-3 hours
- **Priority**: 🔴 **BLOCKING** - Must fix before Day 2

### Migration Steps

1. **Replace imports**:
   ```go
   // OLD
   import "github.com/sirupsen/logrus"

   // NEW
   import "go.uber.org/zap"
   ```

2. **Update type declarations**:
   ```go
   // OLD
   logger *logrus.Logger

   // NEW
   logger *zap.Logger
   ```

3. **Update logging calls**:
   ```go
   // OLD (logrus)
   logger.WithFields(logrus.Fields{
       "key": "value",
   }).Info("message")

   // NEW (zap)
   logger.Info("message",
       zap.String("key", "value"),
   )
   ```

4. **Update error logging**:
   ```go
   // OLD (logrus)
   logger.WithError(err).Error("message")

   // NEW (zap)
   logger.Error("message", zap.Error(err))
   ```

5. **Update field logging**:
   ```go
   // OLD (logrus)
   logger.WithFields(logrus.Fields{
       "namespace": ns,
       "alertName": alert,
   }).Warn("message")

   // NEW (zap)
   logger.Warn("message",
       zap.String("namespace", ns),
       zap.String("alertName", alert),
   )
   ```

---

## 📊 Day 1 Validation Status Update

### Original Day 1 Success Criteria
1. ✅ Package structure created (`pkg/gateway/*`) - **PASS**
2. ⏸️ Basic types defined (`NormalizedSignal`, `ResourceInfo`) - **PENDING VALIDATION**
3. ⏸️ Server skeleton created (can start/stop) - **PENDING VALIDATION**
4. ⏸️ Redis client initialized and tested - **PENDING VALIDATION**
5. ⏸️ Kubernetes client initialized and tested - **PENDING VALIDATION**
6. ❌ Zero lint errors - **FAIL** (10 errors → 2 deprecation warnings after fixes)
7. ⏸️ Foundation tests passing - **PENDING VALIDATION**

### NEW Critical Issue
8. ❌ **Logging framework compliance** - **FAIL** (using logrus instead of zap)

---

## 🎯 Recommendation

**BLOCK Day 2 implementation until logging framework is migrated to Zap**

**Rationale**:
1. Architectural standard violation
2. Technical debt accumulation
3. Performance impact (5x slower)
4. Inconsistency with 99.8% of codebase
5. Migration cost increases with more code

**Next Steps**:
1. Migrate Gateway logging from logrus → zap (2-3 hours)
2. Re-validate Day 1 compilation and lint
3. Proceed to Day 2 validation

---

## 📚 References

- [LOGGING_STANDARD.md](docs/architecture/LOGGING_STANDARD.md) - Official project logging standard
- [LOGGING_STANDARD_SUMMARY.md](docs/architecture/LOGGING_STANDARD_SUMMARY.md) - Quick reference
- [Zap Documentation](https://pkg.go.dev/go.uber.org/zap) - Official Zap docs

---

## 🔍 How This Was Missed

**Root Cause**: Day 1 plan in IMPLEMENTATION_PLAN_V2.12.md specified logrus:

```markdown
### **APDC PLAN PHASE** (1 hour)

**Server Skeleton**:
```go
import (
    "github.com/sirupsen/logrus"  // ❌ WRONG - Plan specified wrong framework
)
```

**Action Required**: Update implementation plan to specify Zap for all Gateway components.

---

## ✅ Validation Checklist

Before proceeding to Day 2:
- [ ] All Gateway files migrated from logrus → zap
- [ ] Compilation passes
- [ ] Lint passes (0 errors, 2 deprecation warnings acceptable)
- [ ] Tests updated and passing
- [ ] Implementation plan updated to specify zap

