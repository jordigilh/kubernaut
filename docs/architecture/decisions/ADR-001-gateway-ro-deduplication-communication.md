# ADR-001: Gateway ↔ RO Deduplication Communication Pattern

**Date**: 2025-12-07
**Status**: ✅ **ACCEPTED**
**Deciders**: Architecture Team, Gateway Team, RO Team
**Technical Story**: Fix spec immutability violation in deduplication tracking

---

## Context

The Gateway service creates `RemediationRequest` CRDs when signals (alerts) arrive. For deduplication, Gateway needs to track occurrence counts (how many times the same signal fired).

**Problem discovered**: Gateway was updating `RR.Spec.Deduplication.OccurrenceCount` after initial creation, which **violates Kubernetes spec immutability principles**.

**Additional requirements identified during discussion**:
1. Prevent infinite failure loops (signal keeps failing → new RR → fails → repeat)
2. Maintain audit trail of signal occurrences
3. Minimize etcd pressure (already 5-6 CRDs per signal)

---

## Decision Drivers

| Driver | Priority | Description |
|--------|----------|-------------|
| **Spec Immutability** | P0 | Must not update spec after creation |
| **etcd Capacity** | P0 | Cannot add CRDs per signal at scale |
| **Audit Trail** | P1 | Dedup data must be visible and persist |
| **Operational Simplicity** | P1 | Prefer simpler solutions |
| **Industry Standards** | P2 | Follow established K8s patterns |

---

## Considered Options

### Option 1: SignalIngestion CRD (Rejected)

**Description**: Introduce a new `SignalIngestion` CRD owned by Gateway. Gateway creates SI with signal data, RO watches SI and creates RR referencing it.

```
Signal → Gateway → SignalIngestion (Gateway owns) → RO watches → RemediationRequest (RO owns)
```

**Schema**:
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: SignalIngestion
spec:
  fingerprint: "abc123"
  signalName: "KubePodCrashLooping"
  # ... signal data
status:
  deduplication:
    occurrenceCount: 5  # Gateway updates this
```

| Pros | Cons |
|------|------|
| ✅ Clean CRD ownership | ❌ **+1 CRD per signal** (etcd pressure) |
| ✅ Gateway doesn't touch RR | ❌ More operational complexity |
| ✅ Clear separation | ❌ Gateway becomes CRD controller |
| | ❌ Additional lookup hop (SI → RR) |

**Why Rejected**: etcd capacity concern. At scale (dozens to hundreds of signals during incidents), adding another CRD per signal is unacceptable. Already have 5-6 CRDs per signal.

---

### Option 2: Three-Stage Pipeline with RawSignal CRD (Rejected)

**Description**: Maximum separation with dedicated deduplication controller.

```
Signal → Gateway (REST) → RawSignal CRD → DedupController → SignalIngestion → RO → RR
```

| Pros | Cons |
|------|------|
| ✅ Gateway stays pure REST | ❌ **+2 CRDs per signal** |
| ✅ Dedup is isolated controller | ❌ 3 hops of latency |
| ✅ Maximum separation | ❌ 3 services to operate |

**Why Rejected**: Even more CRD proliferation. Overengineered for the problem at hand.

---

### Option 3: Redis for Deduplication State (Rejected)

**Description**: Use Redis to track deduplication state. Gateway checks Redis, creates RR only for new signals.

```
Signal → Gateway → Check Redis → Create RR (if new) or Update Redis count (if dup)
```

| Pros | Cons |
|------|------|
| ✅ Zero additional CRDs | ❌ **Dedup data not in RR** |
| ✅ Gateway stays stateless | ❌ No audit trail in K8s |
| ✅ Fast (~1ms lookups) | ❌ Redis failure = no dedup |
| | ❌ Data not persisted to long-term DB with RR |

**Why Rejected**: Loses visibility of deduplication in the RR. Operators can't see occurrence counts when inspecting RRs. Audit trail is critical.

---

### Option 4: Shared Status Ownership (Accepted) ✅

**Description**: Move deduplication from `Spec` to `Status`. Gateway and RO each own distinct sections of `RR.Status`.

```yaml
status:
  # Gateway-owned section
  deduplication:
    occurrenceCount: 5
    firstOccurrence: "2025-12-07T10:00:00Z"
    lastOccurrence: "2025-12-07T10:05:00Z"

  # RO-owned section
  overallPhase: "Processing"
  signalProcessingRef: { ... }
  # ...
```

| Pros | Cons |
|------|------|
| ✅ **Zero additional CRDs** | ⚠️ Two controllers update same CRD |
| ✅ **Industry standard pattern** | ⚠️ Requires conflict handling |
| ✅ Dedup visible in RR | |
| ✅ Audit trail preserved | |
| ✅ Spec remains immutable | |
| ✅ Minimal implementation change | |

**Why Accepted**:
1. Solves the problem with minimal change (validates good architecture)
2. Follows industry patterns (Node, Ingress, Argo, Knative)
3. No etcd pressure increase
4. Dedup data visible and auditable

---

### Option 5: Kubernetes Events for Dedup Tracking (Rejected)

**Description**: Emit K8s Events for each signal occurrence, aggregate for counting.

| Pros | Cons |
|------|------|
| ✅ Lightweight | ❌ Events are ephemeral (1hr default) |
| ✅ K8s native | ❌ Not reliable for counting |
| | ❌ Not queryable efficiently |

**Why Rejected**: Events are not designed for state tracking. Ephemeral and unreliable.

---

### Option 6: Message Queue (Kafka/NATS) (Rejected)

**Description**: Gateway publishes to message queue, separate consumer handles dedup and CRD creation.

| Pros | Cons |
|------|------|
| ✅ Natural separation | ❌ **Adds non-K8s infrastructure** |
| ✅ Built-in dedup (Kafka compaction) | ❌ Operational complexity |
| ✅ Battle-tested at scale | ❌ Not purely K8s-native |

**Why Rejected**: Adds infrastructure dependency. Want to stay K8s-native.

---

## Decision

**Accepted: Option 4 - Shared Status Ownership**

Gateway and RO each own distinct sections of `RemediationRequest.Status`:

| Status Section | Owner | Updates |
|----------------|-------|---------|
| `status.deduplication.*` | Gateway | On duplicate signals |
| `status.overallPhase`, `status.*Ref`, etc. | RO | On lifecycle transitions |

Both use optimistic concurrency (`resourceVersion`) and retry on conflict.

---

## Consequences

### Positive

- ✅ Spec immutability preserved
- ✅ Zero additional CRDs (no etcd pressure)
- ✅ Follows industry-standard K8s pattern
- ✅ Dedup data visible in RR for audit
- ✅ Minimal implementation change
- ✅ **Validates that existing architecture is sound**

### Negative

- ⚠️ Gateway must use informers (becomes a controller)
- ⚠️ Potential conflict retries (standard K8s, minimal overhead)
- ⚠️ Documentation must clearly define ownership boundaries

### Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Ownership violation | Low | High | Code review, clear documentation |
| Conflict storms | Low | Medium | Exponential backoff in retry |
| Field overlap | Low | High | Strict field ownership definition |

---

## Implementation

### Phase 1: API Changes (RO Team)
1. Add `DeduplicationStatus` to `RemediationRequestStatus`
2. Regenerate CRD manifests
3. Update RO to preserve deduplication on status updates

### Phase 2: Gateway Changes (Gateway Team)
1. Remove spec deduplication updates
2. Add status deduplication updates with retry
3. Implement informer for RR status reads
4. Implement blocking logic (consecutive failures)

### Phase 3: Testing (Both Teams)
1. Unit tests for conflict handling
2. Integration tests for dedup flow
3. Load tests for conflict scenarios

---

## Related Decisions

| Decision | Relationship |
|----------|--------------|
| DD-GATEWAY-011 | Implementation details of this ADR |
| DD-GATEWAY-009 | Deduplication logic (updated by this ADR) |

---

## Notes

### Key Insight from Discussion

> "Smaller changes are better than complete refactoring, and having a small change work is a statement that our architecture is solid."

The fact that moving deduplication from Spec to Status solves the problem cleanly validates that the `RemediationRequest` abstraction and overall architecture are sound.

### Industry Precedent

Multiple controllers updating the same CRD's status (with clear field ownership) is established in:
- **Kubernetes core**: Node (kubelet, cloud-controller, node-controller)
- **Kubernetes core**: Pod (kubelet, scheduler)
- **Ingress controllers**: Ingress (user creates, controller updates status)
- **cert-manager**: Certificate (cert-manager, ACME solver)
- **Argo Workflows**: Workflow (workflow-controller, executor)
- **Knative**: Service (serving, activator, autoscaler)

---

## Changelog

| Date | Change |
|------|--------|
| 2025-12-07 | Initial ADR created |

