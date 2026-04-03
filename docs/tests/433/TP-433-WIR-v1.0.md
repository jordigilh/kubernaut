# Test Plan: Kubernaut Agent Wiring and Adapter Integration (#433 — Gaps A)

> **Template**: IEEE 829-2008 + Kubernaut Hybrid v2.0

**Test Plan Identifier**: TP-433-WIR-v1.0
**Feature**: Wire remaining Category A components (adapters, sanitization, anomaly, validator, summarizer, audit store) into the Kubernaut Agent investigator pipeline. Validates that components tested individually in TP-433 produce correct business outcomes when integrated as a connected system.
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3`
**Parent Issue**: #433 (Go Language Migration)

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the integration wiring of nine Category A components that were implemented and unit-tested in TP-433 but not yet connected into the production investigator pipeline:

1. **DataStorage adapter** — `enrichment.DataStorageClient` backed by ogen `*ogenclient.Client`
2. **K8s owner-chain adapter** — `enrichment.K8sClient` backed by `dynamic.Interface` + `meta.RESTMapper`
3. **Enricher wiring** — `enrichment.Enricher` injected into `investigator.New()` (replacing nil)
4. **Custom tools wiring** — 5 custom tools registered in the tool registry when DS is configured
5. **Sanitization pipeline wiring** — G4 + I1 stages applied to tool output in `executeTool`
6. **Anomaly detector wiring** — I7 behavioral thresholds enforced pre/post tool execution
7. **Validator wiring** — I5 workflow allowlist + self-correction loop after `resultParser.Parse()`
8. **Summarizer wiring** — `llm_summarize` transformer applied to oversized tool output
9. **Audit store adapter** — `audit.DSAuditStore` replacing `NopAuditStore{}` when audit enabled

### 1.2 Relationship to TP-433

TP-433 validates individual component correctness (unit tests for sanitization patterns, anomaly thresholds, validator logic, etc.). **This plan validates the wiring**: that these components are correctly injected, called at the right points in the pipeline, and produce the expected business outcomes as an integrated system.

### 1.3 Objectives

1. Adapter type fidelity: ogen types map correctly to enrichment domain types with zero data loss
2. Enrichment integration: owner chain and remediation history appear in RCA system prompts
3. Custom tool availability: 5 tools appear in registry when DS is configured, absent when not
4. Pipeline ordering: `executeTool` executes anomaly check → tool → sanitization → summarization
5. Validator integration: workflow selection results are validated and self-corrected before returning
6. Audit persistence: investigation events flow through `DSAuditStore` to DataStorage (as side effects)
7. Graceful degradation: nil/disabled components produce correct behavior without crashes

### 1.4 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test -v -race ./test/unit/kubernautagent/...` |
| Integration test pass rate | 100% | `go test -v -race ./test/integration/kubernautagent/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable adapter packages |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable packages |
| Data races | 0 | `go test -race ./...` |
| Existing test regressions | 0 | Full Kubernaut Agent test suite passes |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-HAPI-433: Go Language Migration (parent BR)
- BR-HAPI-433-001: Framework Evaluation (LangChainGo selection)
- BR-HAPI-433-002: Kubernetes Toolset + Summarizer
- BR-HAPI-433-004: Security Requirements (I1, G4, I5, I7)
- BR-HAPI-197: Human Review Flag (needs_human_review on exhaustion)
- BR-HAPI-211: Credential Scrubbing (17 DD-005 pattern categories)
- ADR-038: Fire-and-Forget Audit (BufferedAuditStore)
- DD-HAPI-019: Go Rewrite Design (v1.1)
- DD-HAPI-019-002: Toolset Implementation (v1.1)
- DD-HAPI-019-003: Security Architecture (v1.1, layered defense)

### 2.2 Cross-References

- [TP-433-v1.0](./TEST_PLAN.md) — Component-level test plan (106 scenarios)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md) — Anti-patterns, coverage targets, tier definitions
- [APDC Quick Reference](../../development/methodology/APDC_QUICK_REFERENCE.md)
- Issue #433: Kubernaut Agent Go rewrite

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| WR1 | **Ogen type mismatch** — DS `RemediationHistoryEntry` has 15+ fields with `Opt*` wrappers; mapping drops data | Enrichment missing remediation context for LLM | Medium | UT-KA-433W-001..003, IT-KA-433W-001 | UT-KA-433W-001 asserts all mapped fields. CHECKPOINT 1 audits nil handling. |
| WR2 | **Circular ownerReference** — Malformed cluster metadata creates infinite loop in K8s adapter | Enrichment hangs; investigation times out | Low | UT-KA-433W-007, IT-KA-433W-003 | UT-KA-433W-007 verifies max-depth termination (10 levels). |
| WR3 | **Pipeline ordering error** — Sanitization or summarization applied in wrong order | Credentials leak to LLM (if summarizer runs before G4) or injection passes through (if I1 skipped) | High | IT-KA-433W-009..011, IT-KA-433W-017..018 | CHECKPOINT 3 explicitly verifies pipeline order in source code. |
| WR4 | **Anomaly false positives** — Thresholds too aggressive for legitimate multi-tool investigations | Investigation aborts prematurely on valid scenarios | Medium | IT-KA-433W-012..014 | Thresholds are configurable. TP-433 UT-KA-433-058 validates below-threshold passes. |
| WR5 | **Self-correction token budget** — Validator correction loop makes unbounded LLM calls | Excessive token consumption on invalid LLM responses | Medium | IT-KA-433W-015..016 | `maxAttempts=3` hard limit. IT-KA-433W-016 verifies exhaustion path. |
| WR6 | **Audit type mapping drift** — `AuditEvent` fields diverge from `ogenclient.AuditEventRequest` | Audit events silently dropped or rejected by DataStorage | Medium | UT-KA-433W-008..009, IT-KA-433W-019 | UT-KA-433W-008 validates field mapping. CHECKPOINT 4 audits full event flow. |

---

## 4. Scope

### 4.1 Features to be Tested

- **DataStorage adapter**: `DSAdapter` wrapping `ogenclient.Client.GetRemediationHistoryContext` with type mapping
- **K8s adapter**: `K8sAdapter` walking `metadata.ownerReferences` via `dynamic.Interface`
- **Enricher wiring**: `Investigator` with non-nil `Enricher` includes enrichment in prompts
- **Custom tools wiring**: Registry includes 5 custom tools when DS URL configured
- **Sanitization wiring**: `executeTool` applies G4 + I1 sanitization pipeline to tool output
- **Anomaly detector wiring**: `executeTool` enforces per-tool, total, and repeated-failure limits
- **Validator wiring**: `runWorkflowSelection` validates results against workflow allowlist with self-correction
- **Summarizer wiring**: `executeTool` applies `llm_summarize` to oversized tool output after sanitization
- **Audit store adapter**: `DSAuditStore` maps `AuditEvent` to `ogenclient.AuditEventRequest`
- **Config defaults**: New fields (summarizer threshold, anomaly thresholds) have correct defaults

### 4.2 Features Not to be Tested

- **Individual sanitization patterns** (G4/I1 regex correctness) — Covered by TP-433 UT-KA-433-039..053
- **Individual anomaly detection logic** — Covered by TP-433 UT-KA-433-054..059
- **Individual validator logic** (bounds, allowlist) — Covered by TP-433 UT-KA-433-023..027
- **Individual summarizer threshold logic** — Covered by TP-433 UT-KA-433-036..038
- **LangChainGo adapter correctness** — Covered by TP-433 UT-KA-433-004, IT-KA-433-005
- **E2E investigation** — Requires Kind cluster; covered by TP-433 E2E-KA-433-001..009
- **LLM provider APIs** — External dependency, mocked
- **DataStorage service internals** — Tested by TP-DataStorage

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Separate test plan from TP-433 | TP-433 is 480 lines / 106 scenarios. Wiring tests validate a different concern (integration) and would dilute the component-level focus. |
| `httptest.Server` for DS adapter IT | Mocks the external DS API, not our own transport. Consistent with TESTING_GUIDELINES anti-pattern policy. |
| `fake.NewSimpleDynamicClient` for K8s adapter IT | Provides realistic ownerReference behavior without envtest overhead. |
| Mock `llm.Client` for summarizer and validator | LLM is an external dependency. Only the wiring call path is tested, not LLM quality. |
| Audit events verified as side effects | IT-KA-433W-019 triggers `Investigate()` and captures audit POSTs, never calls `StoreAudit()` directly. |

---

## 5. Test Approach

### 5.1 Test Framework

- **Framework**: Ginkgo v2 / Gomega (BDD)
- **Test ID format**: `{TIER}-KA-433W-{SEQ}` (W suffix distinguishes wiring from component scenarios)
- **Runner**: `go test` with Ginkgo CLI for parallel execution

### 5.2 Test Tiers

| Tier | What | Mock Strategy | Location |
|------|------|---------------|----------|
| Unit (UT) | Pure mapping logic: type conversions, config defaults | No mocks needed (pure functions) | `test/unit/kubernautagent/` |
| Integration (IT) | Wired pipeline: adapter I/O, investigator with injected components | Mock `llm.Client`, `fake.NewSimpleDynamicClient`, `httptest.Server` for DS API | `test/integration/kubernautagent/` |

### 5.3 Anti-Pattern Compliance

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md):

| Anti-Pattern | Enforcement |
|--------------|-------------|
| No `time.Sleep()` | All async waits use `Eventually()` / `Consistently()` with configurable timeouts |
| No `Skip()` | All 33 scenarios implemented or removed; no `XIt` or pending tests |
| No HTTP in integration tests | `httptest.Server` mocks *external* DS API only. Integration tests call business logic directly: `enricher.Enrich()`, `investigator.Investigate()`, `executeTool()` |
| No direct audit testing | IT-KA-433W-019 triggers `Investigate()` and verifies audit events as side effects, never calls `StoreAudit()` directly |
| No direct metrics testing | N/A for this plan (no metrics scenarios) |
| Mock ONLY external deps | Mock `llm.Client` for summarizer/validator; `httptest.Server` for DS API; `fake.NewSimpleDynamicClient` for K8s API. All `pkg/` business logic runs real. |

---

## 6. Code Partitioning

### 6.1 Unit-Testable Code (pure logic, no I/O) — target >=80%

| Package | Key Files | Approx Lines |
|---------|-----------|-------------|
| `internal/kubernautagent/enrichment/ds_adapter.go` | Type mapping: `ogenclient.RemediationHistoryEntry` -> `enrichment.RemediationHistoryEntry` | ~60 |
| `internal/kubernautagent/enrichment/k8s_adapter.go` | Owner-chain walk algorithm | ~80 |
| `internal/kubernautagent/audit/ds_store.go` | `AuditEvent` -> `ogenclient.AuditEventRequest` mapping | ~50 |
| `internal/kubernautagent/config/config.go` | Summarizer threshold default, anomaly config defaults, Azure/Vertex config fields | ~20 (additions) |

### 6.2 Integration-Testable Code (I/O-dependent) — target >=80%

| Package | Key Files | Approx Lines |
|---------|-----------|-------------|
| `internal/kubernautagent/enrichment/ds_adapter.go` | ogen client call + error handling | ~40 (I/O path) |
| `internal/kubernautagent/enrichment/k8s_adapter.go` | dynamic client calls | ~60 (I/O path) |
| `internal/kubernautagent/investigator/investigator.go` | `executeTool` pipeline, `runWorkflowSelection` validator wiring | ~80 (additions) |
| `cmd/kubernautagent/main.go` | Component wiring in `main()` | ~40 (additions) |

---

## 7. BR Coverage Matrix

| Business Requirement | Unit Tests | Integration Tests |
|---------------------|-----------|------------------|
| BR-HAPI-433 (parent: enricher + tools wiring) | UT-KA-433W-001..007, 010 | IT-KA-433W-001..008, 011 |
| BR-HAPI-433-001 (Framework: Azure/Vertex config) | UT-KA-433W-012, 013 | — |
| BR-HAPI-433-002 (Summarizer wiring) | UT-KA-433W-010 | IT-KA-433W-017, 018 |
| BR-HAPI-433-004 (Security: I1, I5, I7 wiring) | UT-KA-433W-011 | IT-KA-433W-009, 010, 012..016 |
| BR-HAPI-197 (Human Review on exhaustion) | — | IT-KA-433W-014, 016 |
| BR-HAPI-211 (Credential Scrub wiring) | — | IT-KA-433W-009 |
| ADR-038 (Audit store wiring) | UT-KA-433W-008, 009 | IT-KA-433W-019, 020 |

---

## 8. Test Scenarios

### 8.1 Tier 1: Unit Tests (13 scenarios)

#### Phase 1a — DataStorage Adapter (3 UT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| UT-KA-433W-001 | DS adapter maps ogen `RemediationHistoryEntry` to enrichment type preserving `RemediationUID` as `WorkflowID`, `Outcome.Value` as `Outcome`, and `CompletedAt` as `Timestamp` | BR-HAPI-433 | 1a |
| UT-KA-433W-002 | DS adapter returns empty `[]enrichment.RemediationHistoryEntry` for empty DS history response | BR-HAPI-433 | 1a |
| UT-KA-433W-003 | DS adapter handles nil `Opt*` fields in ogen `RemediationHistoryEntry` without panic (e.g., `SignalFingerprint.Set=false`) | BR-HAPI-433 | 1a |

#### Phase 1b — K8s Owner-Chain Adapter (4 UT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| UT-KA-433W-004 | K8s adapter walks single-level ownerReference (Pod -> ReplicaSet) returning `[{Kind: "ReplicaSet", Name: "rs-abc", Namespace: "default"}]` | BR-HAPI-433 | 1b |
| UT-KA-433W-005 | K8s adapter walks multi-level chain (Pod -> ReplicaSet -> Deployment) returning both entries in order | BR-HAPI-433 | 1b |
| UT-KA-433W-006 | K8s adapter returns empty chain `[]OwnerChainEntry{}` when resource has no ownerReference | BR-HAPI-433 | 1b |
| UT-KA-433W-007 | K8s adapter terminates at `maxOwnerChainDepth` (10) to prevent circular ownerReference loops | BR-HAPI-433 | 1b |

#### Phase 4 — Anomaly Config (1 UT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| UT-KA-433W-011 | `config.DefaultConfig()` applies anomaly thresholds `MaxToolCallsPerTool=5`, `MaxTotalToolCalls=30`, `MaxRepeatedFailures=3` | BR-HAPI-433-004 (I7) | 4 |

#### Phase 6 — Summarizer Config (1 UT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| UT-KA-433W-010 | `config.DefaultConfig()` applies summarizer threshold default (10000 chars) | BR-HAPI-433-002 | 6 |

#### Phase 7 — Audit Store Adapter (2 UT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| UT-KA-433W-008 | Audit store adapter maps `AuditEvent{EventType: "aiagent.llm.request", EventCategory: "aiagent", CorrelationID: "abc123", Data: {"model": "gpt-4"}}` to `ogenclient.AuditEventRequest` with matching `EventType`, `EventCategory`, `CorrelationID` fields | ADR-038 | 7 |
| UT-KA-433W-009 | Audit store adapter preserves correlation ID and event category across mapping of multiple distinct event types (request, response, complete) | ADR-038 | 7 |

#### Phase 8 — Azure/Vertex Config (2 UT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| UT-KA-433W-012 | `config.DefaultConfig()` with `LLM.Provider="azure"` applies default `LLM.APIVersion="2024-02-15-preview"` | BR-HAPI-433-001 | 8 |
| UT-KA-433W-013 | `config.Validate()` returns error when `LLM.Provider="vertex"` but `LLM.GCPProjectID` or `LLM.GCPRegion` is empty | BR-HAPI-433-001 | 8 |

### 8.2 Tier 2: Integration Tests (20 scenarios)

#### Phase 1a — DataStorage Adapter I/O (2 IT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| IT-KA-433W-001 | DS adapter calls `/api/v1/remediation-history-context` with correct `targetKind`, `targetName`, `targetNamespace`, `currentSpecHash` query parameters (verified via `httptest.Server` request capture) | BR-HAPI-433 | 1a |
| IT-KA-433W-002 | DS adapter returns structured error wrapping HTTP 500 from `httptest.Server` (error contains "DS history query failed") | BR-HAPI-433 | 1a |

#### Phase 1b — K8s Adapter I/O (2 IT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| IT-KA-433W-003 | K8s adapter resolves Pod -> ReplicaSet -> Deployment chain against `fake.NewSimpleDynamicClient` with ownerReference fixtures; returns 2-entry chain | BR-HAPI-433 | 1b |
| IT-KA-433W-004 | K8s adapter resolves cluster-scoped resource (Node) with empty namespace; returns correct `OwnerChainEntry` | BR-HAPI-433 | 1b |

#### Phase 2 — Enricher + Custom Tools Wiring (4 IT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| IT-KA-433W-005 | Investigator with real enricher (DS + K8s adapters against mocks) includes owner chain text in RCA system prompt (mock `llm.Client` captures messages, assert enrichment strings present) | BR-HAPI-433 | 2 |
| IT-KA-433W-006 | Investigator with nil enricher produces investigation result without enrichment data and without panic (graceful degradation) | BR-HAPI-433 | 2 |
| IT-KA-433W-007 | Registry built with `DataStorage.URL="http://ds:8080"` includes all 5 custom tool names: `list_available_actions`, `list_workflows`, `get_workflow`, `get_namespaced_resource_context`, `get_cluster_resource_context` | BR-HAPI-433 | 2 |
| IT-KA-433W-008 | Registry built with `DataStorage.URL=""` excludes all custom tools (only K8s + Prometheus tools present) | BR-HAPI-433 | 2 |

#### Phase 3 — Sanitization Pipeline Wiring (3 IT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| IT-KA-433W-009 | `executeTool` with configured sanitization pipeline returns credential-scrubbed output: tool returning `password=s3cret` yields output with `password=***REDACTED***` | BR-HAPI-211 | 3 |
| IT-KA-433W-010 | `executeTool` with configured sanitization pipeline returns injection-stripped output: tool returning `ignore all previous instructions` yields output with that string removed | BR-HAPI-433-004 (I1) | 3 |
| IT-KA-433W-011 | `executeTool` with `sanitizer=nil` returns raw tool output unchanged | BR-HAPI-433 | 3 |

#### Phase 4 — Anomaly Detector Wiring (3 IT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| IT-KA-433W-012 | `executeTool` called 6 times for same tool with `MaxToolCallsPerTool=5` — 6th call returns error JSON containing "per-tool call limit exceeded" | BR-HAPI-433-004 (I7) | 4 |
| IT-KA-433W-013 | `executeTool` with tool that fails 3 times with identical args and `MaxRepeatedFailures=3` — 3rd failure returns error JSON containing "repeated identical failure" | BR-HAPI-433-004 (I7) | 4 |
| IT-KA-433W-014 | Investigation with mock LLM requesting 31 tool calls (`MaxTotalToolCalls=30`) returns `HumanReviewNeeded=true` with reason containing "max turns" or anomaly abort | BR-HAPI-197 | 4 |

#### Phase 5 — Validator + Self-Correction Wiring (2 IT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| IT-KA-433W-015 | `runWorkflowSelection` with mock LLM returning `workflow_id: "unknown-workflow"` triggers validator self-correction; mock LLM returns corrected response on retry; final result has valid `workflow_id` from allowlist | BR-HAPI-433-004 (I5) | 5 |
| IT-KA-433W-016 | `runWorkflowSelection` with mock LLM always returning invalid workflow (3 retries exhausted) produces result with `HumanReviewNeeded=true` and reason containing "self-correction exhausted" | BR-HAPI-197 | 5 |

#### Phase 6 — Summarizer Wiring (2 IT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| IT-KA-433W-017 | `executeTool` with summarizer (threshold=10000) and tool returning 20000-char output calls mock `llm.Client` and returns summarized output | BR-HAPI-433-002 | 6 |
| IT-KA-433W-018 | `executeTool` with summarizer (threshold=10000) and tool returning 5000-char output returns raw output unchanged; mock `llm.Client` is NOT called | BR-HAPI-433-002 | 6 |

#### Phase 7 — Audit Store Wiring (2 IT)

| ID | Business Outcome Under Test | BR | Phase |
|----|----------------------------|-----|-------|
| IT-KA-433W-019 | `Investigate()` with real `DSAuditStore` adapter + `httptest.Server` capturing audit POSTs emits >= 3 audit events (LLM request, LLM response, response.complete) — audit verified as side effect of business logic, not direct `StoreAudit()` call | ADR-038 | 7 |
| IT-KA-433W-020 | `Investigate()` with `cfg.Audit.Enabled=false` uses `NopAuditStore` — zero audit HTTP calls to `httptest.Server` | ADR-038 | 7 |

---

## 9. Test Execution

### 9.1 Commands

```bash
# Unit tests (adapter mapping, config defaults)
go test -v -race ./test/unit/kubernautagent/enrichment/...
go test -v -race ./test/unit/kubernautagent/audit/...
go test -v -race ./test/unit/kubernautagent/config/...

# Integration tests (wired pipeline)
go test -v -race ./test/integration/kubernautagent/enrichment/...
go test -v -race ./test/integration/kubernautagent/investigator/...

# All kubernautagent tests (regression check)
go test -v -race ./test/unit/kubernautagent/... ./test/integration/kubernautagent/...

# Coverage (unit-testable)
go test -coverprofile=cover-ut.out ./test/unit/kubernautagent/...

# Coverage (integration-testable)
go test -coverprofile=cover-it.out ./test/integration/kubernautagent/...
```

### 9.2 Dependencies

| Dependency | Version | Purpose |
|-----------|---------|---------|
| Ginkgo v2 | latest | BDD test framework |
| Gomega | latest | BDD assertion library |
| client-go (fake) | matches cluster version | `fake.NewSimpleDynamicClient` for K8s adapter tests |
| net/http/httptest | stdlib | Mock external DS API |
| ogen-client | generated | DS type definitions for adapter mapping |

---

## 10. Quality Checkpoints

Four checkpoints are integrated into the TDD implementation plan. Each is a hard quality gate.

| Checkpoint | Trigger | Key Validations |
|-----------|---------|-----------------|
| CHECKPOINT 1 | After Phase 1b-REFACTOR (both adapters done) | Build, race detector, type safety compile checks, adapter edge cases, coverage >=80% |
| CHECKPOINT 2 | After Phase 2-REFACTOR (enricher + tools wired) | Full regression sweep, constructor signature health, custom tool name alignment, commit readiness |
| CHECKPOINT 3 | After Phase 6-REFACTOR (sanitizer + anomaly + validator + summarizer wired) | Pipeline ordering audit, per-tier coverage, anti-pattern scan, constructor bloat check |
| CHECKPOINT 4 | After Phase 8-REFACTOR (all code done, including Azure/Vertex) | Full build + race, all-service regression, anti-pattern scan, scenario traceability (33 W-scenarios + 6 provider scenarios), dependency audit |

---

## 11. Approval

| Role | Name | Date | Status |
|------|------|------|--------|
| Test Plan Author | AI Assistant | 2026-03-04 | Draft |
| Technical Lead | — | — | Pending |
| QA Lead | — | — | Pending |

---

**Document Version**: 1.0
**Last Updated**: 2026-03-04
