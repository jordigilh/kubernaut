# Logging Standard - Quick Summary

**Version**: v1.1 (Split Strategy)
**Status**: ✅ **APPROVED**
**Confidence**: **98%**

---

## 🎯 **TL;DR: Which Import Do I Use?**

```
CRD Controllers → sigs.k8s.io/controller-runtime/pkg/log/zap
HTTP Services   → go.uber.org/zap
```

---

## 📊 **Split Strategy**

| Service Type | Import | Example Services |
|--------------|--------|------------------|
| **CRD Controllers** | `sigs.k8s.io/controller-runtime/pkg/log/zap` | Remediation Orchestrator, AI Analysis, Workflow Execution, Kubernetes Executor, Remediation Processor |
| **HTTP Services** | `go.uber.org/zap` | Gateway, Context API, Data Storage, Notification, Dynamic Toolset, HolmesGPT |

---

## 🚀 **Quick Start**

### **CRD Controller Template**

```go
import (
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
    opts := zap.Options{Development: true}
    opts.BindFlags(flag.CommandLine)
    flag.Parse()

    ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
    // ... controller setup ...
}
```

**Run with**: `./controller --zap-log-level=debug --zap-encoder=console`

---

### **HTTP Service Template**

```go
import "go.uber.org/zap"

func main() {
    logger, _ := zap.NewProduction() // or zap.NewDevelopment()
    defer logger.Sync()

    service := NewService(logger)
    service.Start()
}
```

---

## ✅ **Benefits**

### **Why Split Strategy?**

1. ✅ **CRD Controllers**: Get official controller-runtime integration + built-in flags
2. ✅ **HTTP Services**: Get full control + advanced configuration
3. ✅ **Performance**: Identical (both use Uber Zap underneath)
4. ✅ **Consistency**: All services use Zap (no Logrus)
5. ✅ **Best Practice**: Use the right tool for the job

---

## 📚 **Full Documentation**

See [LOGGING_STANDARD.md](./LOGGING_STANDARD.md) for:
- Complete code examples
- Performance benchmarks
- Migration guide from Logrus
- Structured logging best practices
- Correlation ID integration
- Production deployment patterns

---

**Approved**: ✅ October 6, 2025
**Compliance**: 99.8% (496/497 files)
**Standard**: Zap Logging (Split Strategy)
