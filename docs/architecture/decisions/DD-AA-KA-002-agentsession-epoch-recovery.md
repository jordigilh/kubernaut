# DD-AA-KA-002: AgentSession Dispatch Lease Epoch Recovery

**Status:** Approved and implementing
**Date:** 2026-10-18
**Related:** DD-AA-KA-001, Issue #2527, BR-AA-KA-065.3/.4/.9

## Context

`AgentSession` dispatch Leases last 15 minutes and are renewed by the owning
Kubernaut Agent. A pod or process crash can therefore leave a non-terminal
AgentSession blocked until the Lease expires, even though a replacement KA is
ready to continue the investigation.

Shortening the dispatch Lease or deleting all Leases at startup would either
increase false takeovers or duplicate healthy investigations.

## Decision

KA uses a separate, short-lived epoch Lease for each process start:

- `HolderIdentity` contains the pod identity plus a fresh process epoch.
- The epoch Lease is renewed every 10 seconds with a 30-second duration.
- Startup relists existing non-terminal AgentSessions and reconsiders them from
  their immutable `Spec`.
- A dispatch Lease is reclaimed before its 15-minute expiry only when its
  holder's epoch Lease is absent or stale.
- Reclaim updates the dispatch Lease with resource-version/CAS protection.
- Dispatch Lease renewal and deletion verify the current `HolderIdentity`; an
  old epoch cannot renew or delete a replacement owner's Lease.
- Recovery updates `Status.SessionID` to the new in-memory attempt while
  preserving the AgentSession name, UID, Spec, and remediation correlation.
- Recovery emits the existing typed `aiagent.session.resumed` audit event with
  the new attempt identity and recovery reason.
- No prompt history, Valkey state, or completed-turn checkpoint is added. A
  durable checkpoint design remains deferred to Issue #2532.

## Alternatives rejected

1. **Wait for the existing 15-minute Lease:** bounded too slowly for restart
   recovery and leaves active work blocked.
2. **Use Pod existence/UID alone:** does not distinguish a process restart in
   the same Pod and requires broader Kubernetes reads.
3. **Shorten the dispatch Lease or delete all Leases at startup:** risks
   duplicate investigations and violates the live-owner exclusion boundary.

## Consequences

The design adds one namespace-scoped Lease per active KA epoch and a small
startup/reconciliation protocol. It preserves the existing dispatch Lease as
the work ownership authority, uses existing Lease RBAC, and makes stale-owner
fencing observable through status identity and audit correlation.

## Control mapping

- **AC-4/AC-6:** only the holder of the current epoch may mutate its dispatch
  Lease; existing namespace-scoped Lease RBAC is reused.
- **SI-4 / CC7.2:** the epoch heartbeat detects failed owners and recovery is
  reconstructable from AgentSession status and audit events.
- **AU-2/AU-3/AU-9/AU-11:** the recovery boundary uses the existing typed KA
  audit pipeline and remediation correlation ID.
