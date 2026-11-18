# DD-FLEET-013: FMC proof lifecycle

Status: user approved A1/B1/C1, 2026-11-15, PR #2637 / issue #2593.
Authority: BR-SCOPE-001, BR-SECURITY-016, BR-INTEGRATION-065 and the amended
`docs/tests/2593/TEST_PLAN.md`. DD-FLEET-012 singleton coordination is unchanged.

## Selected decisions and alternatives

1. **A1: atomic field ownership.** Prerequisite publishers update only prerequisite
   fields; writer transitions validate a complete active fence and loss invalidates
   only its matching fence. All partition publication/read paths compare the full
   active global fence. Whole-view CAS was rejected as unnecessary coupling;
   comparison-only read patches were rejected because they leave stale publishers.
2. **B1: separate generations (internal contract amendment).** Registration
   revisions advance only for changed extracted registration data (including endpoint,
   labels, namespace, tool prefix or UID), deletion and recreation. Unrelated or
   identical registration updates do not invalidate another registration's proof.
   A separate registry mutation generation fences refresh/startup publication,
   including every accepted informer event (even extracted-data no-ops),
   watch-health and stop changes. Readiness reconciles exact
   ClusterID+RegistrationUID+revision membership from an authoritative versioned
   registry snapshot; retired starts and completions are rejected. Registry-wide
   proof invalidation was rejected because it couples unrelated partitions;
   event-only retirement was rejected because the channel can drop events.
3. **C1: explicit empty finalization (internal contract amendment).** A fenced Lua
   finalization verifies zero residual index members and status expiry/PTTL against
   server TIME. Record/index TTLs are explicitly not applicable for proven empty
   partitions, never fabricated as valid. Commit rechecks empty state and status
   TTL. Marker records were rejected because they invent resources; zero-valued TTL
   acceptance was rejected because it proves nothing. Missing targets remain
   unavailable, not unmanaged or allowed.

## Boundaries

No HA, Lease deployment, POD_UID, RBAC, public v1 or parser relaxation changes.
The process coordinator does not spontaneously take over; stale-session regressions
test the safety contract rather than claiming an observed production exploit.
Broker connection readiness is distinct from observation-backed clean state; no new
startup observation gate is required. HPA read grants exist in the pinned CI operator
ae3d9e40b56dad5ddc6e9899eeb99f5b9fda3902; no role changes are warranted.

## Verification

Deterministic Ginkgo regressions cover stale refresh/startup publication, unrelated
registration changes, retirement, delayed old-session loss, prerequisite races,
complete-empty and stale-to-empty repair, expired/wrong-type status and interrupted
cleanup. Production Syncer -> registry/status store/readiness -> Client.Check wiring
must retain fail-closed behavior. Applicable controls: AC-4/AC-6, SI-10; no new audit
event types or compliance claims are introduced.
