# Test Plan: Enrichment Retry Infrastructure & `rca_incomplete` Detection

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-704-v2
**Feature**: Enrichment retry infrastructure with `rca_incomplete` detection
**Version**: 2.0
**Created**: 2026-04-06
**Updated**: 2026-04-16
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/700-parser-driven-escalation`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that KA's enrichment layer supports configurable retry with exponential backoff for infrastructure calls (GetOwnerChain), error classification (transient vs permanent), and a HardFail signal that the investigator uses to trigger `rca_incomplete` before workflow selection.

### 1.2 Objectives

1. **Retry behavior**: Transient K8s errors trigger exponential backoff retries up to `MaxRetries`
2. **Error classification**: Permanent errors (NotFound, Forbidden) skip retry; transient errors (Timeout, 503, 500, 429) are retried
3. **HardFail signaling**: `EnrichmentResult.HardFail=true` after retry exhaustion or permanent error (only when `MaxRetries > 0`)
4. **Best-effort mode**: With `MaxRetries=0` (default), no retry, no HardFail — preserves backward compatibility
5. **Investigator gate**: HardFail on re-enrichment triggers `rca_incomplete` before workflow selection
6. **Re-enrichment merge safety**: HardFail checked BEFORE label merge to prevent silent drop (CRITICAL-1 fix)

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/enrichment/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/investigator/...` |
| Build | 0 errors | `go build ./...` |
| Lint | 0 new errors | `go vet` + linter |
| Backward compatibility | 0 regressions | All existing enrichment + investigator tests pass |

---

## 2. References

### 2.1 Authority

- **BR-HAPI-261**: LLM-Provided Affected Resource with Owner Resolution (AC #7: owner chain resolution fails → `rca_incomplete`)
- **BR-HAPI-264**: Post-RCA Infrastructure Label Detection (AC #6: label detection fails → `rca_incomplete`)
- **DD-HAPI-006 v1.6**: Three-phase RCA architecture — enrichment failure → `rca_incomplete`
- **Issue #704**: Enrichment-driven `rca_incomplete` detection

### 2.2 Cross-References

- **Issue #700**: Parser-driven escalation (prerequisite — HR fields removed from LLM, parser-derived only)
- **TP-700-v1**: Test plan for parser-driven escalation
- **IT-KA-433-ENR-004/005**: Existing enrichment failure integration tests

---

## 3. Test Items

### 3.1 Production Code Under Test

| File | Change | Lines |
|------|--------|-------|
| `internal/kubernautagent/enrichment/enricher.go` | Add `RetryConfig`, `HardFail` field, `WithRetryConfig()`, `resolveOwnerChainWithRetry()`, `isTransientK8sError()` | ~65 lines |
| `internal/kubernautagent/investigator/investigator.go` | Add HardFail check before re-enrichment label merge | ~12 lines |

### 3.2 Test Code

| File | Change | Test ID |
|------|--------|---------|
| `test/unit/kubernautagent/enrichment/enricher_retry_test.go` | New unit tests | UT-704-E-001..004 |
| `test/integration/kubernautagent/investigator/investigator_test.go` | Updated integration test | IT-KA-704-001 |
| `test/e2e/kubernautagent/adversarial_parity_e2e_test.go` | Relaxed assertion (best-effort mode in E2E) | E2E-KA-433-ADV-016 |

---

## 4. Test Scenarios

### UT-704-E-001: Transient error retried, HardFail after exhaustion

**Tier**: Unit
**BR**: BR-HAPI-261 AC#7

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create enricher with `MaxRetries=3`, K8s client returning InternalError 4 times | Enricher configured |
| 2 | Call `Enrich()` | Retries 3 times (4 total calls) |
| 3 | Assert call count | 4 (initial + 3 retries) |
| 4 | Assert `OwnerChainError` | Non-nil |
| 5 | Assert `HardFail` | `true` |

### UT-704-E-002: Transient error succeeds on retry

**Tier**: Unit
**BR**: BR-HAPI-261 AC#7

| Step | Action | Expected |
|------|--------|----------|
| 1 | K8s client returns ServiceUnavailable on call 1, success on call 2 | Configured |
| 2 | Call `Enrich()` | Succeeds on 2nd attempt |
| 3 | Assert call count | 2 |
| 4 | Assert `OwnerChainError` | Nil |
| 5 | Assert `HardFail` | `false` |
| 6 | Assert `OwnerChain` | Populated from successful retry |

### UT-704-E-003: Permanent error triggers immediate HardFail

**Tier**: Unit
**BR**: BR-HAPI-261 AC#7

| Step | Action | Expected |
|------|--------|----------|
| 1 | K8s client returns NotFound error | Configured with `MaxRetries=3` |
| 2 | Call `Enrich()` | No retry (permanent error) |
| 3 | Assert call count | 1 (no retry for permanent errors) |
| 4 | Assert `OwnerChainError` | Non-nil |
| 5 | Assert `HardFail` | `true` |

### UT-704-E-004: Best-effort mode (retries=0)

**Tier**: Unit
**BR**: Backward compatibility

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create enricher with default config (no `WithRetryConfig`) | `MaxRetries=0` |
| 2 | K8s client returns error | Single call |
| 3 | Assert `OwnerChainError` | Non-nil |
| 4 | Assert `HardFail` | `false` (best-effort mode) |

### IT-KA-704-001: Owner chain failure triggers rca_incomplete

**Tier**: Integration
**BR**: BR-HAPI-261 AC#7
**Preconditions**: `fakeK8sClient` with `apierrors.NewNotFound`; enricher with `MaxRetries=3`; signal differs from RCA target to trigger re-enrichment

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create investigator with failing K8s client + strict retry config | Enricher in strict mode |
| 2 | Mock LLM returns valid RCA targeting different resource | Re-enrichment triggered |
| 3 | Re-enrichment fails with permanent error | HardFail=true |
| 4 | Investigator detects HardFail before label merge | Early return before workflow |
| 5 | Assert `HumanReviewNeeded=true`, `HumanReviewReason="rca_incomplete"` | Correct |
| 6 | Assert `RCASummary` populated | RCA phase completed before check |
| 7 | Assert `WorkflowID` empty | Workflow phase skipped |

### E2E-KA-433-ADV-016: rca_incomplete scenario (relaxed)

**Tier**: E2E
**BR**: Backward compatibility
**Status**: Relaxed — enrichment uses default `MaxRetries=0` in E2E, so HardFail is never set

| Step | Action | Expected |
|------|--------|----------|
| 1 | Send incident with `mock_rca_incomplete` keyword | Mock LLM returns RCA |
| 2 | Enrichment fails (Pod not in Kind cluster) | OwnerChainError set, HardFail=false |
| 3 | Assert `needs_human_review=false` | Best-effort mode, no rca_incomplete |

> **TODO(#704)**: Activate strict assertion when E2E fixtures support `RetryConfig` injection.

---

## 5. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Re-enrichment merge drops HardFail | Eliminated | n/a | CRITICAL-1 fix: check BEFORE merge |
| Existing tests regressed | Low | High | Full suite (54 UT + 58 IT) passes |
| Best-effort E2E doesn't catch bugs | Medium | Low | Strict behavior tested at UT/IT tiers |
| Backoff timing in tests | Low | Low | Tests use 1ms backoff; verify call count, not timing |

---

## 6. TDD Phase Tracking

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 0 — Revert | Complete | Reverted broken #704 gate, relaxed ADV-016, kept IT-704-001 as RED test |
| RED | Complete | UT-704-E-001..004 written + IT-KA-704-001 verified failing for right reasons |
| GREEN | Complete | RetryConfig, HardFail, retry loop, investigator gate — all tests pass |
| REFACTOR | Complete | Doc comments, adversarial audit, full regression check |
| Final Checkpoint | Complete | Full build clean, 54 UT + 58 IT + 22 KA packages pass |
