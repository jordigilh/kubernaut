# ADR-062: Phase Handler Registry Pattern for Remediation Orchestrator

**Status**: Accepted  
**Date**: 2026-03-04  
**Issue**: [#666](https://github.com/jordigilh/kubernaut/issues/666)  
**Supersedes**: None (refactoring of existing monolithic reconciler)

## Context

The Remediation Orchestrator (RO) controller's `Reconcile` method had grown into a monolithic state machine with a single `switch` statement dispatching to ~7 inline handler methods (`handlePendingPhase`, `handleProcessingPhase`, `handleAnalyzingPhase`, etc.). This caused several maintainability and testability problems:

1. **Monolithic reconciler**: `reconciler.go` exceeded 3,700 lines, making code review and navigation difficult.
2. **Tight coupling**: Each phase handler had direct access to all reconciler fields, creating hidden dependencies that were impossible to trace without reading the full method.
3. **Untestable in isolation**: Testing a single phase required constructing the entire `Reconciler` with all its dependencies (lock manager, routing engine, status aggregator, etc.), even when the phase under test only used 2-3 of them.
4. **Fragile refactoring**: Renaming or restructuring any reconciler field risked breaking unrelated phases.
5. **Scaling concern**: Adding new phases (e.g., for v1.4 DAG-based orchestration) would continue inflating the monolith.

## Decision

Extract each phase's reconciliation logic into a dedicated **PhaseHandler** implementing a common interface, registered in a **Registry** that the `Reconcile` method dispatches through.

### Core Interface

```go
type PhaseHandler interface {
    Phase() phase.Phase
    Handle(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error)
}
```

### TransitionIntent

Handlers return a declarative `TransitionIntent` instead of raw `ctrl.Result`. The reconciler's `ApplyTransition` method translates intents into Kubernetes results:

| Intent | Meaning |
|--------|---------|
| `Advance(targetPhase, reason)` | Transition to next phase |
| `NoOp(reason)` | Requeue with default delay |
| `Requeue(duration, reason)` | Requeue after specific duration |
| `Fail(failurePhase, err)` | Transition to Failed terminal |
| `InheritedCompleted(ref, kind)` | Inherited completion from dedup source |
| `InheritedFailed(err, ref, kind)` | Inherited failure from dedup source |

### Registry

```go
type Registry struct {
    handlers map[phase.Phase]PhaseHandler
}

func (r *Registry) Lookup(p phase.Phase) (PhaseHandler, bool)
func (r *Registry) MustRegister(h PhaseHandler)
```

### Callback Injection Pattern

Complex handlers receive their reconciler dependencies through explicit callback structs rather than direct field access. This makes dependencies visible and enables isolated unit testing with mock callbacks:

```go
type AnalyzingCallbacks struct {
    AtomicStatusUpdate     func(ctx, rr, fn) error
    IsWorkflowNotNeeded    func(ai) bool
    CreateApproval         func(ctx, rr, ai) (string, error)
    AcquireLock            func(ctx, target) (bool, error)
    // ... explicit dependency list
}
```

### Extracted Handlers

| Handler | Phase | Lines | Callbacks | Test Count |
|---------|-------|-------|-----------|------------|
| `PendingHandler` | Pending | ~50 | 0 (direct deps) | 5 |
| `ProcessingHandler` | Processing | ~60 | 0 (direct deps) | 5 |
| `ExecutingHandler` | Executing | ~80 | 0 (direct deps) | 8 |
| `VerifyingHandler` | Verifying | ~120 | 6 | 14 |
| `BlockedHandler` | Blocked | ~100 | 4 | 12 |
| `AnalyzingHandler` | Analyzing | ~250 | 16 | 20 |
| `AwaitingApprovalHandler` | AwaitingApproval | ~200 | 14 | 13 |

Shared utility: `WFECreationHelper` (used by Analyzing + AwaitingApproval, 6 tests).

### Reconcile Dispatch (After)

```go
func (r *Reconciler) Reconcile(ctx, req) (ctrl.Result, error) {
    // ... fetch RR, global timeout, terminal checks ...

    currentPhase := phase.Phase(rr.Status.OverallPhase)
    if h, ok := r.phaseRegistry.Lookup(currentPhase); ok {
        intent, err := h.Handle(ctx, rr)
        if err != nil {
            return ctrl.Result{}, fmt.Errorf("phase handler %s: %w", currentPhase, err)
        }
        return r.ApplyTransition(ctx, rr, intent)
    }

    logger.Info("Unknown phase", "phase", rr.Status.OverallPhase)
    return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil
}
```

## Consequences

### Positive

- **reconciler.go reduced from 3,743 to 3,038 lines** (-705 lines of dead code removed)
- **Each handler is independently testable** with mock callbacks — no envtest required for unit tests
- **83 new handler-specific unit tests** (total 299 controller specs)
- **3x test stability** verified with zero flakiness
- **Adding a new phase** requires only: (1) implement `PhaseHandler`, (2) register in `NewReconciler`
- **Explicit dependency contracts** — each handler's `Callbacks` struct documents exactly what it needs

### Negative

- **Callback boilerplate**: The wiring section in `NewReconciler` is verbose (~160 lines of callback assignments). This is an acceptable trade-off for explicit dependency visibility.
- **Indirection**: Debugging requires tracing through the registry lookup and callback indirection. Mitigated by clear naming conventions and the `Phase()` method on each handler.

### Neutral

- **No behavioral changes**: All characterization tests pass, confirming 1:1 behavioral fidelity with the original monolith.
- **Legacy helper methods remain**: Methods like `transitionToFailed`, `handleBlocked`, `transitionPhase` remain on the `Reconciler` as they serve as shared infrastructure used by multiple handlers via callbacks.

## Alternatives Considered

1. **DAG-based orchestration**: Full migration to a DAG execution engine. Deferred to v1.4 as it requires CRD schema changes and is orthogonal to the testability improvements achieved here. This refactoring is a prerequisite for the DAG migration.
2. **Interface-based dependency injection**: Each handler receives interfaces instead of callback functions. Rejected because it would create many single-method interfaces, adding indirection without proportional benefit. Callbacks are more Go-idiomatic for this use case.
3. **Embedded handler structs**: Handlers embed a base struct with common reconciler access. Rejected because it re-introduces tight coupling — the goal was explicit dependency contracts.

## References

- Issue [#666](https://github.com/jordigilh/kubernaut/issues/666): Refactor RO controller into Phase Handler Registry
- Test Plan: `docs/tests/666/TEST_PLAN.md`
- `pkg/remediationorchestrator/phase/transition.go`: TransitionIntent types
- `pkg/remediationorchestrator/phase/registry.go`: PhaseHandler interface and Registry
