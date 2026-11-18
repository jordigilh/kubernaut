# DD-FLEET-012: Singleton FMC Writer Coordination

**Status:** Approved for the current singleton deployment

**Date:** 2026-11-15

**Business requirement:** BR-SCOPE-001 / issue #2593

## Decision

FMC currently runs as a single replica and uses a process-local writer
coordinator. The coordinator generates an in-process holder identity, session
identity, and monotonic fence for the existing Valkey proof contracts.

The current FMC deployment does not require:

- a Kubernetes `coordination.k8s.io/v1 Lease`;
- `POD_UID` from the downward API; or
- Lease-specific operator RBAC.

The existing Kubernetes Lease coordinator remains available as a future
multi-replica option but is not wired into the singleton production startup.

## Alternatives considered

1. **Kubernetes Lease coordination** — supports cross-pod writer fencing, but
   requires operator-owned Lease RBAC and `POD_UID` wiring that are outside the
   current singleton scope.
2. **Process-local writer coordination — selected** — removes deployment RBAC
   and pod-identity requirements while retaining the Valkey writer-fence,
   repair, and fail-closed proof contracts.
3. **Remove writer fencing entirely** — rejected because the Valkey status and
   repair protocol relies on a complete fence to reject stale or ambiguous
   mutations.

## Consequences

- FMC can start with the operator's existing deployment and ServiceAccount.
- A process restart creates a new local writer session and fence.
- Two FMC processes or pods are not supported by this decision; a future HA
  change must reintroduce a shared fencing authority and its deployment/RBAC
  contract.
