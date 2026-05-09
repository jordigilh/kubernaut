# Test Plan: kubectl Tool Multi-Group Kind Resolution & Signal Label Override for Tool Context

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1064-v1
**Feature**: kubectl tools resolve ambiguous kinds via multi-group fallback; signal label overrides propagate to workflow discovery tool context
**Version**: 1.0
**Created**: 2026-05-08
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/1061-1062-signal-target-and-ambiguous-kind`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

PR #1063 fixed multi-group kind resolution for the enrichment/owner-chain adapter (`K8sAdapter`), but the kubectl tool layer used by the LLM during investigation has the same bug on a separate code path. When the LLM calls `kubectl_get_by_kind_in_namespace({kind: "Subscription"})`, it resolves to `messaging.knative.dev/v1` instead of `operators.coreos.com/v1alpha1`, returning an empty `SubscriptionList` from the wrong API group. The OLM `Subscription/etcd` resource is invisible to the LLM.

Additionally, `list_available_actions` and `list_workflows` filter the DataStorage workflow catalog using `Component = strings.ToLower(signal.ResourceKind)`, where `signal` comes from the raw `SignalContext` (enrichment-resolved). When enrichment resolves to the wrong kind (due to the ambiguous-kind bug), the catalog filter returns no matches. The `target_resource_kind` signal label override from #1061 should propagate to the tool context as defense-in-depth.

### 1.2 Objectives

1. **Multi-group fallback for Get**: When a kubectl tool calls `resolver.Get` for an ambiguous kind, it tries all API groups until the resource is found (NotFound/Forbidden triggers fallback to next group).
2. **Multi-group fallback for List**: When a kubectl tool calls `resolver.List` for an ambiguous kind, it returns the first non-empty result across API groups.
3. **Fallback logging**: Each fallback attempt emits a structured log entry for observability (FedRAMP AU-2).
4. **Single-group regression**: Kinds that exist in only one API group continue to resolve correctly.
5. **Signal label override to tool context**: `ApplySignalLabelOverrides` extracts and validates label overrides and applies them to the `SignalContext` before attaching to the tool context.
6. **DRY override logic**: `SignalToPrompt` is refactored to use `ApplySignalLabelOverrides` (no duplication).
7. **Workflow discovery defense-in-depth**: `list_available_actions` and `list_workflows` see the label-overridden `ResourceKind` in the context, producing correct `Component` filter values.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/tools/k8s/... --ginkgo.focus="Issue.*1064"` |
| Unit-testable code coverage | >=80% | Coverage on `resolveMappings`, `Get`, `List`, `ApplySignalLabelOverrides` |
| Integration test pass rate | 100% | `make test-integration-kubernautagent` |
| Backward compatibility | 0 regressions | Full k8s tools + investigator suite passes |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority

- Issue #1064: kubectl tool calls resolve ambiguous kinds to wrong API group
- Issue #1065: list_available_actions returns empty catalog during active investigation (consequence of #1064)
- Issue #1062: K8sAdapter multi-group fallback (merged in PR #1063, established pattern)
- Issue #1061: SignalToPrompt label override (merged in PR #1063, override logic)
- FedRAMP AU-2: Auditable Events (fallback attempts)
- FedRAMP AU-3: Audit Content (structured log fields)
- FedRAMP AC-6: Least Privilege (Forbidden error handling in fallback)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Anti-Pattern Detection](../../testing/ANTI_PATTERN_DETECTION.md)
- [Test Plan #1061](../1061/TEST_PLAN.md) — Signal label override logic
- [Test Plan #1062](../1062/TEST_PLAN.md) — K8sAdapter multi-group fallback pattern
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — Refactor phase validation

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | List fallback returns items from wrong group when both groups have items | Confusing LLM results | Low | UT-KA-1064-002, 003 | First non-empty strategy; don't merge across groups |
| R2 | `ResourcesFor` returns GVRs in different order on different clusters | Non-deterministic first-group preference | Low | UT-KA-1064-002 | Acceptable: correct group always has items; order doesn't affect correctness |
| R3 | Non-NotFound/Forbidden errors trigger unnecessary fallback | Performance degradation | Medium | UT-KA-1064-007 | Hard-stop on unexpected errors; only NotFound/Forbidden trigger fallback |
| R4 | `ApplySignalLabelOverrides` modifies shared `SignalLabels` map | Data race | Low | UT-KA-1064-016 | Read-only access to map; Go value semantics on struct copy |
| R5 | Stale REST mapper cache after CRD install | Kind not found | Low | UT-KA-1064-008 | `resettableMapper` retry pattern from K8sAdapter |
| R6 | `NewDynamicResolver` signature change breaks callers | Build failure | High | All | All 10 call sites updated (trivial: pass `logr.Discard()` in tests) |

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- `resolveMappings()` in `pkg/kubernautagent/tools/k8s/resolver.go` (new)
- `Get()` with multi-group fallback in `resolver.go` (modified)
- `List()` with first-non-empty fallback in `resolver.go` (modified)
- `NewDynamicResolver()` with logger parameter in `resolver.go` (modified)
- `ApplySignalLabelOverrides()` in `internal/kubernautagent/investigator/investigator_phases.go` (new)
- `SignalToPrompt()` refactored to use `ApplySignalLabelOverrides` (modified)
- Signal context override in `runWorkflowSelection()` in `investigator.go` (modified)

### 4.2 Features Not to be Tested

- `K8sAdapter.resolveMappingsAll` / `getResourceWithFallback` (tested by #1062)
- `isValidK8sIdentifier` (tested by #1061)
- `LogLabelOverrideOrRejection` (tested by #1061)
- LLM prompt rendering (tested by prompt package)
- DataStorage catalog server-side filtering (external service)
- Log tools, metrics tools, events tool (no kind→GVR resolution)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| First non-empty for List (not merge) | Merging items from different API groups would confuse the LLM with mixed apiVersions |
| Logger as concrete-only (not in `ResourceResolver` interface) | Logging is an implementation detail; interface contract is Get/List |
| `resettableMapper` retry included | Consistency with K8sAdapter; handles CRD discovery cache staleness |
| `ApplySignalLabelOverrides` returns a copy | Value semantics; original `SignalContext` not modified |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

- **Unit**: >=80% of `resolveMappings`, `Get`, `List` (resolver), `ApplySignalLabelOverrides` (investigator)
- **Integration**: >=80% of tool execution path with multi-group mapper
- **E2E**: Skipped — resolver logic is exercised via unit and integration tests with fake clients

### 5.2 Pass/Fail Criteria

**PASS**: All unit + integration tests pass, no regressions in k8s tools or investigator suites, no `Skip()`, `time.Sleep`, or anti-patterns.

**FAIL**: Any test failure, or multi-group fallback not logged at V(1) level.

### 5.3 Anti-Patterns Explicitly Avoided

| Anti-Pattern | How Avoided |
|---|---|
| NULL-TESTING | Every test asserts specific behavioral outcomes |
| STATIC DATA TESTING | Tests use realistic multi-group mapper setups (Subscription in two groups) |
| LIBRARY TESTING | Tests validate tool behavior, not K8s client internals |
| IMPLEMENTATION TESTING | Tests assert what the tool returns, not how resolution works internally |
| MOCK OVERUSE | Uses K8s `DefaultRESTMapper` and `dynamicfake` (real K8s infra), not hand-written mocks |
| `time.Sleep` | Not used; all operations are synchronous |
| `Skip()` / `XIt` | Not used |

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested.

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `pkg/kubernautagent/tools/k8s/resolver.go` | `resolveMappings`, `Get`, `List`, `NewDynamicResolver` | ~80 modified/added |
| `internal/kubernautagent/investigator/investigator_phases.go` | `ApplySignalLabelOverrides`, `SignalToPrompt` (refactored) | ~20 modified/added |
| `internal/kubernautagent/investigator/investigator.go` | `runWorkflowSelection` (context override) | ~5 modified |
| `cmd/kubernautagent/main.go` | `registerK8sTools` (logger wiring) | ~2 modified |

---

## 7. BR Coverage Matrix

| BR / Issue ID | Description | Priority | Tier | Test ID | Status |
|---------------|-------------|----------|------|---------|--------|
| #1064 | Get resolves ambiguous kind via multi-group fallback | P0 | Unit | UT-KA-1064-001 | Pass |
| #1064 | List returns first non-empty result for ambiguous kind | P0 | Unit | UT-KA-1064-002 | Pass |
| #1064 | List returns items from correct group even if first group is empty | P0 | Unit | UT-KA-1064-003 | Pass |
| #1064 | Single-group kind Get regression | P0 | Unit | UT-KA-1064-004 | Pass |
| #1064 | Single-group kind List regression | P0 | Unit | UT-KA-1064-005 | Pass |
| #1064 | Unknown kind Get returns actionable error | P1 | Unit | UT-KA-1064-006 | Pass |
| #1064 | Unknown kind List returns actionable error | P1 | Unit | UT-KA-1064-007 | Pass |
| FedRAMP AU-2 | Multi-group fallback emits structured log on success | P1 | Unit | UT-KA-1064-008 | Pass |
| FedRAMP AC-6 | Non-NotFound/Forbidden error stops fallback immediately | P0 | Unit | UT-KA-1064-009 | Pass |
| #1064 | kubectl_find_resource with ambiguous kind finds correct items | P1 | Unit | UT-KA-1064-010 | Pass |
| #1064 | kubernetes_jq_query with ambiguous kind queries correct group | P1 | Unit | UT-KA-1064-011 | Pass |
| #1064/#1065 | ApplySignalLabelOverrides applies valid kind override | P0 | Unit | UT-KA-1064-012 | Pass |
| #1064/#1065 | ApplySignalLabelOverrides applies valid name override | P0 | Unit | UT-KA-1064-013 | Pass |
| #1064/#1065 | ApplySignalLabelOverrides rejects invalid label values | P0 | Unit | UT-KA-1064-014 | Pass |
| #1064/#1065 | ApplySignalLabelOverrides no-op without labels | P1 | Unit | UT-KA-1064-015 | Pass |
| #1064/#1065 | ApplySignalLabelOverrides does not modify original | P0 | Unit | UT-KA-1064-016 | Pass |
| #1064/#1065 | list_available_actions uses overridden ResourceKind | P0 | Unit | UT-KA-1064-017 | Pass |
| #1064/#1065 | list_workflows uses overridden ResourceKind | P0 | Unit | UT-KA-1064-018 | Pass |
| #1064 | Multi-group Get with ambiguous kind in integration setup | P0 | Integration | IT-KA-1064-001 | Pass |
| #1064 | Multi-group List with ambiguous kind in integration setup | P0 | Integration | IT-KA-1064-002 | Pass |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-KA-{ISSUE}-{SEQUENCE}` (Unit), `IT-KA-{ISSUE}-{SEQUENCE}` (Integration)

### Tier 1: Unit Tests — Resolver Multi-Group Fallback

| ID | Business Outcome Under Test | Given | When | Then |
|----|----------------------------|-------|------|------|
| UT-KA-1064-001 | LLM can retrieve a resource whose kind exists in multiple API groups | Subscription exists in `operators.coreos.com/v1alpha1` but NOT in `messaging.knative.dev/v1`; both groups registered in mapper | `kubectl_get_by_name({kind: "Subscription", name: "etcd", namespace: "demo-operator"})` | Returns the OLM Subscription object; response contains `"etcd"` |
| UT-KA-1064-002 | LLM sees non-empty resource list for ambiguous kind | Subscription items exist in `operators.coreos.com` namespace but NOT in `messaging.knative.dev`; both groups registered | `kubectl_get_by_kind_in_namespace({kind: "Subscription", namespace: "demo-operator"})` | Returns list with >=1 item; items contain `"etcd"` |
| UT-KA-1064-003 | LLM sees correct items when both groups have resources but only one matches the namespace | Subscription `alpha` in `operators.coreos.com` in `demo-operator` ns; Subscription `beta` in `messaging.knative.dev` in `other` ns | `kubectl_get_by_kind_in_namespace({kind: "Subscription", namespace: "demo-operator"})` | Returns list containing `alpha` but not `beta` |
| UT-KA-1064-004 | Single-group kind Get works unchanged (regression) | Deployment `api-server` exists in `apps/v1` (only group) | `kubectl_describe({kind: "Deployment", name: "api-server", namespace: "default"})` | Returns the Deployment; response contains `"api-server"` |
| UT-KA-1064-005 | Single-group kind List works unchanged (regression) | Multiple Deployments exist in `apps/v1` | `kubectl_get_by_kind_in_namespace({kind: "Deployment", namespace: "default"})` | Returns non-empty list containing seeded Deployments |
| UT-KA-1064-006 | Unknown kind Get produces actionable error | No `FooBarBaz` kind registered in mapper | `resolver.Get(ctx, "FooBarBaz", "test", "default")` | Error contains `"unsupported kind"` and `"FooBarBaz"` |
| UT-KA-1064-007 | Unknown kind List produces actionable error | No `FooBarBaz` kind registered in mapper | `resolver.List(ctx, "FooBarBaz", "default")` | Error contains `"unsupported kind"` and `"FooBarBaz"` |
| UT-KA-1064-008 | Multi-group fallback success emits structured log | Subscription exists in second group only; logger captures output | `resolver.Get(ctx, "Subscription", "etcd", "demo-operator")` | Log contains `"multi-group kind resolved"`, `"kind"`, `"api_group"`, `"result"` |
| UT-KA-1064-009 | Non-NotFound/Forbidden error stops fallback immediately | First group returns a connection error (not NotFound/Forbidden) | `resolver.Get(ctx, "Subscription", "etcd", "demo-operator")` | Error is returned immediately; second group is NOT attempted |
| UT-KA-1064-010 | kubectl_find_resource resolves ambiguous kind correctly | Subscription `etcd` in `operators.coreos.com` in cluster | `kubectl_find_resource({kind: "Subscription", keyword: "etcd"})` | Returns match containing `"etcd"` |
| UT-KA-1064-011 | kubernetes_jq_query resolves ambiguous kind correctly | Subscription items in `operators.coreos.com` | `kubernetes_jq_query({kind: "Subscription", jq_expr: ".items[].metadata.name"})` | Result contains the Subscription name |

### Tier 1: Unit Tests — Signal Label Override for Tool Context

| ID | Business Outcome Under Test | Given | When | Then |
|----|----------------------------|-------|------|------|
| UT-KA-1064-012 | Valid target_resource_kind label overrides ResourceKind | Signal with `ResourceKind="Namespace"` and `SignalLabels["target_resource_kind"]="Subscription"` | `ApplySignalLabelOverrides(signal)` | Returned signal has `ResourceKind="Subscription"` |
| UT-KA-1064-013 | Valid target_resource_name label overrides ResourceName | Signal with `ResourceName="demo-operator"` and `SignalLabels["target_resource_name"]="etcd"` | `ApplySignalLabelOverrides(signal)` | Returned signal has `ResourceName="etcd"` |
| UT-KA-1064-014 | Invalid label values are rejected (enrichment fallback) | Signal with `SignalLabels["target_resource_kind"]="../etc/passwd"` | `ApplySignalLabelOverrides(signal)` | Returned signal has `ResourceKind` unchanged from original |
| UT-KA-1064-015 | No-op when SignalLabels is nil | Signal with `SignalLabels=nil` | `ApplySignalLabelOverrides(signal)` | Returned signal is identical to input |
| UT-KA-1064-016 | Original SignalContext not modified (value semantics) | Signal with `ResourceKind="Namespace"` and valid override label | `ApplySignalLabelOverrides(signal)` | Original signal still has `ResourceKind="Namespace"` |
| UT-KA-1064-017 | list_available_actions uses label-overridden ResourceKind for Component | Context carries signal with `ResourceKind="Namespace"` overridden to `"Subscription"` via label | `list_available_actions` tool executes | DS query `Component` param equals `"subscription"` |
| UT-KA-1064-018 | list_workflows uses label-overridden ResourceKind for Component | Context carries signal with `ResourceKind="Namespace"` overridden to `"Subscription"` via label | `list_workflows` tool executes | DS query `Component` param equals `"subscription"` |

### Tier 2: Integration Tests — Multi-Group Resolver in Full Tool Stack

| ID | Business Outcome Under Test | Given | When | Then |
|----|----------------------------|-------|------|------|
| IT-KA-1064-001 | kubectl_get_by_name resolves ambiguous kind through full tool stack | Multi-group mapper with `Subscription` in two groups; fake dynamic client with resource in OLM group only | Tool executed via `registry.Execute` | Returns the OLM Subscription JSON |
| IT-KA-1064-002 | kubectl_get_by_kind_in_namespace lists correct group through full tool stack | Multi-group mapper; items in OLM group only | Tool executed via `registry.Execute` | Returns non-empty list from correct API group |

### Tier Skip Rationale

- **E2E**: Skipped. Resolver logic is exercised via unit and integration tests with K8s fake infrastructure. E2E would require a real cluster with two CRDs sharing a kind name, which is an exotic setup not warranted for this pure resolution logic.

---

## 9. Environmental Needs

### 9.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Fake infrastructure**: `k8s.io/apimachinery/pkg/api/meta.DefaultRESTMapper`, `k8s.io/client-go/dynamic/fake.NewSimpleDynamicClient`, `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured`
- **Logger capture**: `github.com/go-logr/logr/funcr` for structured log assertion
- **Location**: `test/unit/kubernautagent/tools/k8s/multi_group_resolution_1064_test.go` (resolver), `test/unit/kubernautagent/investigator/signal_label_tool_context_1064_test.go` (override + tool context)

### 9.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Fake infrastructure**: Same as unit (fake dynamic client + DefaultRESTMapper), but exercised through registry.Execute
- **Location**: `test/integration/kubernautagent/tools/k8s/k8s_tools_test.go` (add scenarios to existing suite)

---

## 10. TDD Implementation Phases

### Phase 1: TDD RED — Write Failing Tests

**Objective**: Write all test scenarios from §8 that fail because the implementation does not yet exist.

**Deliverables**:
- `test/unit/kubernautagent/tools/k8s/multi_group_resolution_1064_test.go` — UT-KA-1064-001 through 011
- `test/unit/kubernautagent/investigator/signal_label_tool_context_1064_test.go` — UT-KA-1064-012 through 018
- Integration test additions — IT-KA-1064-001, 002

**Expected state**: All new tests fail; all existing tests continue to pass.

**Validation**:
```bash
go test ./test/unit/kubernautagent/tools/k8s/... --ginkgo.focus="Issue.*1064" -count=1
# Expected: FAIL (new tests fail)

go test ./test/unit/kubernautagent/investigator/... --ginkgo.focus="Issue.*1064" -count=1
# Expected: FAIL (new tests fail)

go test ./test/unit/kubernautagent/tools/k8s/... --ginkgo.skip="Issue.*1064" -count=1
# Expected: PASS (existing tests unaffected)
```

### Phase 2: TDD GREEN — Minimal Implementation to Pass

**Objective**: Implement the minimum code to make all RED tests pass.

**Deliverables**:
- `pkg/kubernautagent/tools/k8s/resolver.go` — `resolveMappings`, modified `Get`/`List`, `NewDynamicResolver` with logger
- `internal/kubernautagent/investigator/investigator_phases.go` — `ApplySignalLabelOverrides`, refactored `SignalToPrompt`
- `internal/kubernautagent/investigator/investigator.go` — `runWorkflowSelection` context override
- `cmd/kubernautagent/main.go` — logger wiring in `registerK8sTools`
- All test call sites updated for `NewDynamicResolver` signature change

**Expected state**: All tests pass (new + existing). Build succeeds.

**Validation**:
```bash
go build ./...
go test ./test/unit/kubernautagent/tools/k8s/... -count=1 -race
go test ./test/unit/kubernautagent/investigator/... -count=1 -race
go test ./test/unit/kubernautagent/tools/custom/... -count=1 -race
```

### Phase 3: TDD REFACTOR — Improve Code Quality

**Objective**: Improve code without changing behavior. Validate against 100 Go Mistakes.

**Go Mistakes Checklist** (from [100-go-mistakes](https://github.com/teivah/100-go-mistakes)):

| # | Mistake | Relevance | Check |
|---|---------|-----------|-------|
| 1 | Unintended variable shadowing | `err` in fallback loops | Verify no shadowed `err` in `Get`/`List` fallback |
| 2 | Unnecessary nested code | Fallback loop readability | Flatten where possible |
| 5 | Interface pollution | `ResourceResolver` unchanged | Confirmed: logger not in interface |
| 9 | Generics confusion | N/A | Not using generics |
| 26 | Slices and maps are pointers | `SignalLabels` map in `ApplySignalLabelOverrides` | Confirmed: read-only access; struct passed by value |
| 28 | Maps and memory leaks | N/A | No long-lived maps added |
| 48 | Ignoring errors | `resolveMappings` error handling | Verify all errors checked |
| 49 | Not wrapping errors | Fallback error chain | Use `%w` verb consistently |
| 53 | Not handling nil maps | `SignalLabels` nil safety | `ApplySignalLabelOverrides` safe on nil map (Go map index returns zero) |
| 54 | Misusing context | Context propagation in fallback | Verify `ctx` passed to all client calls |
| 77 | Not closing resources | N/A | No closeable resources added |
| 88 | Not using testing utilities | Test helpers | Use `buildAmbiguousKindMapper()` helper |

**Additional refactor checks**:
- Doc comments on all exported functions
- `PROD-1`: Update `BuildKindIndex` doc comment to reflect secondary role
- `ARCH-2`: Document parallel with `K8sAdapter.resolveMappingsAll`
- No dead code introduced
- Lint clean: `golangci-lint run --timeout=5m` on modified files

**Validation**:
```bash
go build ./...
go test ./test/unit/kubernautagent/tools/k8s/... -count=1 -race
go test ./test/unit/kubernautagent/investigator/... -count=1 -race
go test ./test/unit/kubernautagent/tools/custom/... -count=1 -race
make test-integration-kubernautagent
make test-e2e-kubernautagent
```

---

## 11. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/tests/1064/TEST_PLAN.md` |
| Unit test suite (resolver) | `test/unit/kubernautagent/tools/k8s/multi_group_resolution_1064_test.go` |
| Unit test suite (label override) | `test/unit/kubernautagent/investigator/signal_label_tool_context_1064_test.go` |
| Integration test additions | `test/integration/kubernautagent/tools/k8s/k8s_tools_test.go` |

---

## 12. Execution

```bash
# Unit — resolver multi-group
go test ./test/unit/kubernautagent/tools/k8s/... -count=1 --ginkgo.focus="Issue.*1064" -v

# Unit — signal label override for tool context
go test ./test/unit/kubernautagent/investigator/... -count=1 --ginkgo.focus="Issue.*1064" -v

# Unit — full regression
go test ./test/unit/kubernautagent/tools/k8s/... -count=1 -race
go test ./test/unit/kubernautagent/investigator/... -count=1 -race
go test ./test/unit/kubernautagent/tools/custom/... -count=1 -race

# Integration
make test-integration-kubernautagent

# E2E
make test-e2e-kubernautagent
```

---

## 13. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-08 | Initial test plan for Issue #1064 / #1065 defense-in-depth |
