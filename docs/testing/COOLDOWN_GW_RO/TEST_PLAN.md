# Test Plan: Post-Completion Cooldown (Gateway) + Target Resource Case Fix (RO)

**Version**: 1.2
**Created**: 2026-02-24
**Status**: Active
**Services**: Gateway (GW), Remediation Orchestrator (RO)

## Tracking

| Change | Issue | Parent Issue |
|--------|-------|--------------|
| Gateway post-completion cooldown in `ShouldDeduplicate` | [#202](https://github.com/jordigilh/kubernaut/issues/202) | [#197](https://github.com/jordigilh/kubernaut/issues/197) |
| RO target resource case mismatch fix | [#203](https://github.com/jordigilh/kubernaut/issues/203) | [#197](https://github.com/jordigilh/kubernaut/issues/197) |

---

## Context

Two bugs prevent the 5-minute post-completion cooldown from functioning:

1. **Gateway gap** (#202): No cooldown enforcement at RR creation time. Signals arriving within the cooldown window after a successful remediation create new RRs, wasting LLM calls on stale data.
2. **RO case mismatch** (#203): The reconciler lowercases the Kind in `targetResource` (e.g., `deployment`) while the WFE creator preserves original casing (`Deployment`). Case-sensitive field selectors never match, breaking `CheckRecentlyRemediated` and `CheckResourceBusy`.

**Architectural decision**: Cooldown enforcement moves to the Gateway (`ShouldDeduplicate`) so that no RR is created during cooldown. The RO case fix remains as defense-in-depth.

---

## Business Requirements

| BR ID | Description | Affected Service |
|-------|-------------|------------------|
| BR-GATEWAY-011 | Gateway must deduplicate identical signals within TTL window | Gateway |
| BR-GATEWAY-012 | Gateway must expire deduplicated signals after configurable TTL | Gateway |
| BR-GATEWAY-181 | Status-based deduplication -- Gateway checks RR phase | Gateway |
| DD-WE-001 | Resource locking safety: target resource format, 5-min cooldown | WE / RO |
| DD-RO-002 Check 4 | Workflow-specific cooldown (same workflow+target blocked) | RO |

---

## Test Naming Convention

**Format**: `{TestType}-{ServiceCode}-{BR#}-{Sequence}`

Per DD-TEST-006 and V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md.

---

## BR Coverage Matrix

### Gateway: Post-Completion Cooldown in ShouldDeduplicate (#202)

| Test ID | BR | Description | Test File | Status |
|---------|----|-------------|-----------|--------|
| UT-GW-011-001 | BR-GATEWAY-011 | Signal during cooldown after successful remediation is deduplicated | `test/unit/gateway/processing/phase_checker_business_test.go` | Implemented |
| UT-GW-011-002 | BR-GATEWAY-012 | Signal after cooldown expires creates a new RR with fresh data | `test/unit/gateway/processing/phase_checker_business_test.go` | Implemented |
| UT-GW-011-003 | BR-GATEWAY-011 | Only Completed RRs trigger cooldown (Failed/TimedOut do not) | `test/unit/gateway/processing/phase_checker_business_test.go` | Implemented |
| UT-GW-011-004 | BR-GATEWAY-181 | Non-terminal RR takes priority over cooldown-eligible Completed RR | `test/unit/gateway/processing/phase_checker_business_test.go` | Implemented |
| UT-GW-011-005 | BR-GATEWAY-011 | Multiple Completed RRs: most recent CompletedAt determines cooldown | `test/unit/gateway/processing/phase_checker_business_test.go` | Implemented |
| IT-GW-011-001 | BR-GATEWAY-011 | Completed RR within cooldown triggers dedup via real K8s field selectors | `test/integration/gateway/processing/deduplication_integration_test.go` | Implemented |
| IT-GW-011-002 | BR-GATEWAY-012 | Completed RR outside cooldown allows new RR via real K8s field selectors | `test/integration/gateway/processing/deduplication_integration_test.go` | Implemented |

### RO: Target Resource Case Mismatch Fix (#203)

| Test ID | BR | Description | Test File | Status |
|---------|----|-------------|-----------|--------|
| UT-RO-WE001-001 | DD-WE-001 | CheckRecentlyRemediated finds WFE when target casing matches | `test/unit/remediationorchestrator/routing/blocking_test.go` | Implemented |
| UT-RO-WE001-002 | DD-WE-001 | Lowercased Kind fails to match WFE target (bug reproduction) | `test/unit/remediationorchestrator/routing/blocking_test.go` | Implemented |
| IT-RO-WE001-001 | DD-WE-001, DD-RO-002 | Full reconciler path: casing-preserved targetResource matches WFE, triggers RecentlyRemediated block | `test/integration/remediationorchestrator/routing_integration_test.go` | Implemented |

---

## Anti-Pattern Compliance

Per `docs/development/business-requirements/TESTING_GUIDELINES.md`:

- [x] No `time.Sleep()` -- all async waits use `Eventually()` (N/A: pure unit tests with fake client)
- [x] No `Skip()` -- all tests must pass or fail
- [x] Tests validate **business outcomes** (dedup behavior, cooldown enforcement), not implementation details
- [x] No direct infrastructure testing -- test behavior that produces the outcome
- [x] Ginkgo/Gomega BDD framework
- [x] Test IDs from this plan in Describe/Context/It descriptions
