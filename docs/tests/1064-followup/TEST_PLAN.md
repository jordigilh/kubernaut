# Test Plan: Multi-Version Kind Resolution in kubectl Tool Resolver

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1064-followup-v1
**Feature**: kubectl tools resolve multi-version CRD kinds via RESTMappings fallback
**Version**: 1.0
**Created**: 2026-05-09
**Author**: AI Assistant
**Status**: Complete
**Branch**: `fix/1064-multi-version-resolution`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

Post-merge validation of PR #1063 revealed that `kubectl_get_by_kind_in_namespace` and
`kubectl_get_by_kind_in_cluster` resolve `AuthorizationPolicy` to `security.istio.io/v1beta1`
instead of `security.istio.io/v1`, returning empty results. The root cause is in the
`resolveMappings` fallback path in `resolver.go`: when `ResourcesFor` fails (stale mapper
cache), `resolveMapping` calls `RESTMapping(gk)` (singular), which returns a **single**
"preferred" version mapping. On a `MultiRESTMapper` (production composition), this can also
return `AmbiguousKindError`, causing a hard `"unsupported kind"` failure.

The fix replaces `RESTMapping(gk)` with `RESTMappings(gk)` (plural) in the fallback,
returning ALL versions for the GroupKind.

### 1.2 Objectives

1. **Multi-version fallback**: When `ResourcesFor` fails and the fallback fires, `resolveMappings` returns ALL versions (v1beta1 + v1) for the GroupKind via `RESTMappings`.
2. **List multi-version**: `resolver.List` correctly returns non-empty results by iterating through all version mappings, finding items in v1 when v1beta1 is empty.
3. **Get multi-version**: `resolver.Get` correctly returns the resource found in v1 after v1beta1 returns NotFound.
4. **kindIndex group hint**: The fallback uses `kindIndex` to provide the correct API group for CRD kinds.
5. **Empty-group graceful failure**: Without kindIndex, CRD kinds produce a clear error (not a panic or silent empty result).
6. **Multi-group regression**: Existing multi-group resolution (Subscription across operators.coreos.com + messaging.knative.dev) continues to work.
7. **Structured logging**: Fallback attempts emit V(1) structured log entries for observability (FedRAMP AU-2).

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-kubernautagent` |
| Unit-testable code coverage | >=80% | Coverage on `resolveMappings` fallback, `resolveGroupKind`, `Get`, `List` |
| Integration test pass rate | 100% | `make test-integration-kubernautagent` |
| Backward compatibility | 0 regressions | All existing k8s tool tests pass |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority

- Issue #1064: kubectl tool calls resolve ambiguous kinds to wrong API group
- Issue #1064 follow-up comment: `kubectl_get_by_kind_in_namespace` resolves AuthorizationPolicy to v1beta1 instead of v1
- PR #1063: Initial fix for multi-group kind resolution (merged)
- FedRAMP AU-2: Auditable Events (fallback attempts)
- FedRAMP SI-10: Input Validation (kind string validation)

### 2.2 Cross-References

- [Test Plan #1064](../1064/TEST_PLAN.md) — Original multi-group resolution test plan
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — Refactor phase validation

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Mitigation |
|----|------|--------|-------------|------------|
| RISK-1 | `RESTMappings` returns versions in non-deterministic order | List returns items from unexpected version | Low | List-loop tries all versions; first non-empty wins. Both versions serve same storage via CRD conversion. PoC 12 validated. |
| RISK-2 | Empty-group GroupKind fails to match CRD kinds | Hard error for kinds not in kindIndex | Low | Same behavior as current code. Test UT-MVR-005 validates graceful error. |
| RISK-3 | Multi-group resolution (#1064) regresses | Subscription resolution breaks | Low | UT-MVR-006 regression test + full existing test suite. |

---

## 4. Test Infrastructure

### 4.1 failResourcesForMapper

A thin mapper wrapper that implements `meta.RESTMapper`, forces `ResourcesFor` to return an
error (simulating stale discovery cache), and delegates all other methods to a real
`DefaultRESTMapper`. Validated in PoC 7 and PoC 10.

### 4.2 Multi-version mapper setup

Two `DefaultRESTMapper` instances (one per GroupVersion) or a single mapper with both versions
in `defaultGroupVersions`. Both patterns validated in PoCs.

---

## 5. Test Scenarios

### 5.1 Unit Tests — Multi-Version Fallback Resolution

| ID | Scenario | Input | Expected | TDD Phase | Status |
|----|----------|-------|----------|-----------|--------|
| UT-MVR-001 | resolveMappings fallback returns all versions when ResourcesFor fails | kind=AuthorizationPolicy, mapper has v1beta1+v1, ResourcesFor fails via wrapper, kindIndex has security.istio.io group | Both v1beta1 and v1 mappings returned (2 mappings) | GREEN | Pass |
| UT-MVR-002 | List returns v1 items when v1beta1 is empty (fallback mapper) | List AuthorizationPolicy in ns, v1beta1 empty, v1 has items, ResourcesFor fails | Items from v1 returned | GREEN | Pass |
| UT-MVR-003 | Get returns v1 resource when v1beta1 is NotFound (fallback mapper) | Get AuthorizationPolicy/deny-all in ns, v1beta1 NotFound, v1 has resource, ResourcesFor fails | v1 resource returned | GREEN | Pass |
| UT-MVR-004 | Fallback uses kindIndex group hint for CRD kinds | kind=AuthorizationPolicy, kindIndex maps to security.istio.io | RESTMappings called with GroupKind{Group: "security.istio.io", Kind: "AuthorizationPolicy"} | GREEN | Pass |
| UT-MVR-005 | Fallback with empty group fails gracefully for CRD kinds | kind=AuthorizationPolicy, empty kindIndex, ResourcesFor fails | Error returned (no panic, no empty result) | GREEN | Pass |
| UT-MVR-006 | Multi-group resolution regression (Subscription) | kind=Subscription, two groups in mapper, ResourcesFor succeeds | Both groups resolved, OLM Subscription found | GREEN | Pass |
| UT-MVR-007 | Fallback log emitted at V(1) with version count | kind=AuthorizationPolicy, fallback fires with 2 versions | Log entry contains "fallback", version count, group | GREEN | Pass |

### 5.2 Unit Tests — Adversarial & Edge Cases

| ID | Scenario | Input | Expected | TDD Phase | Status |
|----|----------|-------|----------|-----------|--------|
| UT-MVR-ADV-001 | Fallback with single version in kindIndex (no multi-version) | kind=Deployment, only apps/v1, ResourcesFor fails | Single mapping returned, Get/List work normally | GREEN | Pass |
| UT-MVR-ADV-002 | Fallback with nil kindIndex | kindIndex=nil, ResourcesFor fails | Error returned (GroupKind with empty group won't find CRD) | GREEN | Pass |

---

## 6. TDD Phase Tracking

### Phase 1: RED (Write Failing Tests)

- Write tests UT-MVR-001 through UT-MVR-007 and UT-MVR-ADV-001 through UT-MVR-ADV-002
- Validate existing tests pass, new tests fail
- CHECKPOINT 1: 9-category audit on test quality

### Phase 2: GREEN (Implement to Pass)

- Extract `resolveGroupKind` helper
- Update `resolveMappings` fallback to use `RESTMappings(gk)` (plural)
- Add structured logging for fallback
- Validate all tests pass
- CHECKPOINT 2: 9-category audit on implementation

### Phase 3: REFACTOR (Improve Code Quality)

- Remove unused `resolveMapping` method
- Consolidate doc comments and log messages
- 100 Go Mistakes checklist validation
- Full test suite validation (unit + integration + e2e)
- CHECKPOINT 3: Final 9-category audit

---

## 7. Anti-Pattern Compliance

| Anti-Pattern | Status | Evidence |
|-------------|--------|----------|
| NULL-TESTING | Compliant | All tests assert specific values, not just nil/non-nil |
| STATIC DATA TESTING | Compliant | Tests use realistic Istio CRD kinds and versions |
| MOCK OVERUSE | Compliant | Only external dependency (mapper) is mocked; real DefaultRESTMapper used for delegation |
| NARROW ASSERTION | Compliant | Tests verify mapping count, version strings, and returned resource content |
| IMPLEMENTATION TESTING | Compliant | Tests verify behavioral outcomes (items found, error returned), not internal state |
