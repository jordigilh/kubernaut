# ADR-060: Parallel Workflow Validation

**Date**: May 9, 2026
**Status**: Superseded — Pattern 1 (typed-result-slot parallelism) retired by Issue #1642; see [Update: Post-#1481/#1642](#update-post-1481-1642) below
**Version**: 1.0
**Authority**: AUTHORITATIVE for workflow registration validation concurrency patterns (historical — Pattern 2 remains active for `ValidateDependencies` callers outside registration)
**Related**: DD-WE-006 (Schema-Declared Dependencies), DD-WORKFLOW-016 (Action Type Taxonomy), ADR-058 (Webhook-Driven Registration), BR-STORAGE-014 (Workflow Catalog Management)
**GitHub Issue**: [#1070](https://github.com/jordigilh/kubernaut/issues/1070)

---

## Update: Post-#1481/#1642

Both of the two checks this ADR parallelized alongside action-type taxonomy have since been removed from `HandleCreateWorkflow`:

- **Issue #1481** removed step 5c, schema-declared dependency existence validation (`ValidateDependencies`). Kubernetes now validates dependency existence exclusively at runtime when the WorkflowExecution's Job/PipelineRun attempts to mount the volume/workspace (BR-WORKFLOW-008).
- **Issue #1642** removed step 5b, execution bundle existence validation (`ValidateBundleExists`). A pre-flight registry check running from the DataStorage pod's own network/credential context cannot validate self-signed or credential-required private registries reachable only by the actual workflow execution environment, so the check unconditionally blocked otherwise-valid registrations. Kubernetes now fails fast at Job/PipelineRun image-pull time instead, extending the same BR-WORKFLOW-008 pattern.

With only the action-type taxonomy check remaining, Pattern 1 (typed-result-slot + `sync.WaitGroup`) no longer parallelizes anything and has been collapsed back into a direct sequential call in `validateExternalChecks`. The historical context below (Context, Decision, Consequences, Alternatives) is preserved for reference but no longer describes the current implementation of `HandleCreateWorkflow`. Pattern 2 (`errgroup` inside `ValidateDependencies`) remains relevant wherever that validator is still invoked outside the registration path.

## Context

`HandleCreateWorkflow` performed three independent external validation checks during workflow registration:

1. **Action-type taxonomy** (PostgreSQL query via `ActionTypeExists`)
2. **Execution bundle existence** (OCI registry HEAD via `ValidateBundleExists`) — removed, Issue #1642
3. **Schema-declared dependencies** (Kubernetes API GETs via `ValidateDependencies`) — removed, Issue #1481

Prior to this change, these checks ran sequentially. Each call could take 50-500ms depending on backend latency, making worst-case registration latency the sum of all three (up to 1.5s+). Since the checks are independent, parallelization reduces wall-clock time to the duration of the slowest check.

## Decision

### Pattern 1: Typed-Result-Slot (Handler Level)

The three top-level validation checks run in parallel goroutines using `sync.WaitGroup`. Each goroutine writes its error (if any) to a **pre-assigned slot** in a fixed-size array, protected by a `sync.Mutex`. After all goroutines complete, slots are checked in **priority order** (action-type > bundle > dependency), and the first non-nil error is returned.

This pattern was chosen over `errgroup` at the handler level because:
- **Deterministic error priority**: `errgroup` returns the first error chronologically, not by semantic priority. The slot pattern preserves the original sequential error contract regardless of goroutine completion order.
- **No early cancellation**: All checks run to completion so that per-phase Prometheus metrics are always emitted, even if a higher-priority check fails.

### Pattern 2: errgroup (Dependency Validator Level)

Within `ValidateDependencies`, individual K8s API GETs are parallelized using `errgroup.WithContext` with `SetLimit(10)`. The first error cancels remaining checks via the derived context.

This pattern was chosen because:
- **All dependency errors are equivalent in priority**: There is no semantic ordering between "Secret A missing" and "ConfigMap B missing".
- **Early cancellation is desirable**: Once any dependency is missing, remaining checks add no value.
- **Concurrency cap**: `SetLimit(10)` prevents a schema with many dependencies from overwhelming the K8s API server.

### Timeout Budget

A 10-second `context.WithTimeout` wraps the entire `validateExternalChecks` call, preventing a degraded backend (e.g., slow OCI registry) from consuming the full server `WriteTimeout` (30s).

## Consequences

### Positive

- Registration latency reduced from sum-of-three to max-of-three (typically 30-60% improvement)
- Per-phase Prometheus histograms (`datastorage_workflow_validation_duration_seconds`) provide observability into individual backend latencies
- Error priority contract was preserved and locked by unit tests (UT-WF-1070-001, -002, -004; -003 was removed with the bundle-not-found error type it covered — Issue #1642)

### Negative

- **Non-deterministic dependency error messages**: When multiple dependencies are missing, `errgroup` returns whichever goroutine's error completes first. The error message is valid but not deterministic across requests. This is documented behavior, not a bug.
- **Increased peak goroutine count**: Under load, each concurrent registration spawns up to 3 + min(N, 10) goroutines (N = dependency count). Bounded by the 10-goroutine cap on the dependency validator.

## Alternatives Considered

| Alternative | Rejected Because |
|---|---|
| `errgroup` for all three top-level checks | Cannot guarantee error priority order |
| Channels with select | More complex, same slot pattern semantics achievable with WaitGroup + mutex |
| `multierror` accumulator for dependencies | Returns all errors but complicates the RFC 7807 single-error response contract |
| Sequential with caching | Caching adds state management complexity; parallelization gives comparable latency reduction without state |
