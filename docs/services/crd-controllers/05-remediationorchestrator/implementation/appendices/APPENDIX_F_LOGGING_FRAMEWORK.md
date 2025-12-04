# Appendix F: Logging Framework (DD-005 Compliance)

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](../IMPLEMENTATION_PLAN_V1.1.md)
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE V3.0
**Last Updated**: 2025-12-04

---

## Logging Framework Decision

### Decision: logr/zapr

**Rationale**: CRD controller using controller-runtime which provides logr interface

```go
import "sigs.k8s.io/controller-runtime/pkg/log"

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)
}
```

---

## Log Levels (DD-005)

| Level | Usage | Example |
|-------|-------|---------|
| Error | Failures requiring attention | `log.Error(err, "Failed to create CRD")` |
| Info | Normal operations | `log.Info("Phase transition")` |
| Debug | Verbose debugging | `log.V(1).Info("Processing status")` |

---

## Standard Log Fields

| Field | Description | Required |
|-------|-------------|----------|
| `remediationRequest` | RR name | Always |
| `namespace` | RR namespace | Always |
| `phase` | Current phase | On phase ops |
| `childCRD` | Child CRD name | On child ops |

---

## Example Usage

```go
// Info logging
log.Info("Creating child CRD",
    "remediationRequest", rr.Name,
    "namespace", rr.Namespace,
    "childKind", "SignalProcessing")

// Error logging
log.Error(err, "Failed to update status",
    "remediationRequest", rr.Name,
    "phase", rr.Status.Phase)

// Debug logging
log.V(1).Info("Status aggregation",
    "remediationRequest", rr.Name,
    "childCount", len(children))
```

---

## Log Event Catalog

### Reconciliation Events
- Reconcile Start: Info level
- Reconcile End: Info level with duration
- Reconcile Error: Error level

### Phase Events
- Phase Change: Info level
- Invalid Transition: Error level
- Terminal State: Info level

### Child CRD Events
- Child Created: Info level
- Child Failed: Error level
- Child Status Change: Debug level

### Timeout Events
- Timeout Detected: Info level
- Escalation Created: Info level

---

## Logger Configuration

```go
// cmd/remediationorchestrator/main.go
opts := zap.Options{
    Development: false,
    Level:       zapcore.InfoLevel,
    TimeEncoder: zapcore.ISO8601TimeEncoder,
}
ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
```

---

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](../IMPLEMENTATION_PLAN_V1.1.md)

