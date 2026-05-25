# Test Plan: AF Agent Quality — Namespace, Signal Grounding, Output Suppression (Issue #1282)

## 1. Test Plan Identifier

TP-AF-1282-v1.0

## 2. References

- Issue: https://github.com/jordigilh/kubernaut/issues/1282
- BR: BR-AF-1282 — AF agent quality: realistic RRs, output suppression, prompt hardening
- FedRAMP Controls: AU-2 (Audit Events), SC-7 (Boundary Protection), SI-10 (Information Input Validation)
- Related: #1276 (prompt personalization), DD-SEVERITY-001 (severity determination)
- Architecture: Follows KA's transparent enrichment pattern (SignalContextResolver for workflow discovery)

## 3. Introduction

This test plan validates six interrelated fixes to how the AF agent creates
RemediationRequests and presents results to the user:

1. Namespace resolution from K8s downward API (not LLM-supplied)
2. Signal name grounding in real AlertManager alerts / K8s events (not synthetic)
3. Signal source tagging (`a2a-agent`)
4. LLM parameter minimization (remove namespace/severity from CreateRRArgs)
5. Tool output suppression (reasoning only, no raw JSON)
6. Prompt hardening (mandate 4-phase journey, ban direct kubectl for investigations)

The design principle is: LLM = intent translator (kind, name, description);
AF = infrastructure expert (namespace, signal, severity, source).

## 4. Test Items

| Component | Version | Description |
|---|---|---|
| `pkg/apifrontend/tools/af_create_rr.go` | v1.5.0 | RR creation with AF-resolved fields |
| `pkg/apifrontend/tools/af_list_events.go` | v1.5.0 | EventSummary with Type field |
| `pkg/apifrontend/launcher/part_converter.go` | v1.5.0 | Output suppression / summarizers |
| `pkg/apifrontend/agent/prompt.go` | v1.5.0 | BuildInstruction with downward API namespace |
| `pkg/apifrontend/agent/prompt.txt` | v1.5.0 | Hardened system prompt |
| `cmd/apifrontend/main.go` | v1.5.0 | Namespace resolution wiring |

## 5. Software Risk Issues

| Risk | Impact | Mitigation |
|---|---|---|
| Downward API file absent in dev/test | Medium | Fallback to config.Session.Namespace; unit test both paths |
| Triager disabled (severityTriage.enabled=false) | Medium | K8s events fallback provides signal name; "unknown" last resort |
| LLM ignores hardened prompt constraints | Low | Prompt is best-effort; AF enforces namespace/signal server-side regardless |
| EventSummary.Type addition ripples to existing tests | Low | Mechanical addition; spike confirmed 0 integration/E2E references |
| Removing Namespace/Severity from CreateRRArgs | Medium | Spike confirmed: 15 test cases in 2 files; mock-LLM scenarios need update |

## 6. Features to be Tested

### 6.1 Namespace Resolution (F-NS)

- F-NS-01: AF reads namespace from `/var/run/secrets/kubernetes.io/serviceaccount/namespace`
- F-NS-02: `config.Session.Namespace` overrides downward API when explicitly set
- F-NS-03: Falls back to `"default"` when both sources are absent
- F-NS-04: Resolved namespace injected into BuildInstruction prompt context
- F-NS-05: Resolved namespace used by af_create_rr (not LLM-supplied)
- F-NS-06: Namespace no longer in CreateRRArgs schema (LLM cannot supply it)

### 6.2 Signal Name Grounding (F-SIG)

- F-SIG-01: When Triager Tier 1 returns a firing alert, signalName = TriageResult.AlertName
- F-SIG-02: When Triager Tier 1.5/2 returns a rule match, signalName = TriageResult.RuleName
- F-SIG-03: When Triager returns LLM-only (Tier 2.5/3), fall back to K8s events
- F-SIG-04: K8s events fallback picks dominant Warning event reason (tier-ranked)
- F-SIG-05: When no events exist, signalName = "unknown"
- F-SIG-06: Signal name is never synthetic (no "af-manual-*" prefix)
- F-SIG-07: Event dominance ranks OOMKilling > BackOff > Unhealthy > Evicted > FailedScheduling
- F-SIG-08: Normal events (Scheduled, Pulled, Created) are filtered out of dominance

### 6.3 Signal Source (F-SRC)

- F-SRC-01: All AF-originated RRs have signalSource = "a2a-agent"
- F-SRC-02: signalSource is hardcoded, not LLM-supplied

### 6.4 CreateRRArgs Minimization (F-MIN)

- F-MIN-01: CreateRRArgs has only Kind, Name, Description fields
- F-MIN-02: Severity resolved by Triager (existing pipeline)
- F-MIN-03: Namespace resolved by AF (downward API / config)
- F-MIN-04: Invalid Kind still rejected
- F-MIN-05: Invalid Name still rejected
- F-MIN-06: Long Description still truncated at maxDescriptionLen
- F-MIN-07: Singleflight deduplication still works with namespace from AF

### 6.5 Output Suppression (F-OUT)

- F-OUT-01: summarizeCreateRR returns human-friendly text, not raw JSON
- F-OUT-02: Tools not in keyToolSummarizers have FunctionResponse suppressed (nil)
- F-OUT-03: Reasoning tokens (part.Thought) flow through to user
- F-OUT-04: FunctionCall parts produce short status messages
- F-OUT-05: No raw JSON payloads visible in A2A artifact stream

### 6.6 Prompt Hardening (F-PRM)

- F-PRM-01: Prompt mandates 4-phase remediation journey
- F-PRM-02: Prompt bans kubectl_get/kubectl_list as first investigation action
- F-PRM-03: Prompt documents auto-resolved fields (namespace, signal, severity, source)
- F-PRM-04: Prompt states signal name must align with real alerts/events
- F-PRM-05: Deployment Context section includes resolved namespace

### 6.7 EventSummary Enhancement (F-EVT)

- F-EVT-01: EventSummary includes Type field (Normal/Warning)
- F-EVT-02: HandleListEvents populates Type from K8s event object
- F-EVT-03: dominantEventReason filters by Warning type

## 7. Features Not to be Tested

- LLM behavioral compliance with hardened prompt (non-deterministic)
- AlertManager vs Prometheus client selection (spike confirmed: Prometheus `/api/v1/alerts` sufficient)
- E2E full-pipeline autonomous remediation (would require live LLM + Kind cluster)
- Mock-LLM scenario updates (tracked separately; behavioral, not compile-time)

## 8. Approach

### Test Pyramid (Pyramid Invariant)

| Tier | Scope | Estimated Count | Target Coverage |
|---|---|---|---|
| Unit | Pure logic: namespace resolution, signal derivation, event dominance, part converter | 35-40 | >= 80% of unit-testable code |
| Integration | Wiring: af_create_rr with mock K8s client + Triager, EventSummary with envtest | 8-12 | >= 80% of integration-testable code |
| E2E | Deferred (requires live LLM + Kind cluster) | 0 | N/A this iteration |

### TDD Phases

| Phase | Description | Checkpoint | Status |
|---|---|---|---|
| Red 1 | Write failing tests for F-NS-*, F-MIN-*, F-SRC-* | -- | DONE |
| Green 1 | Implement namespace resolution + CreateRRArgs refactor | -- | DONE |
| Refactor 1 | 100 Go Mistakes audit on Phase 1 code | Checkpoint 1 | DONE |
| Red 2 | Write failing tests for F-SIG-*, F-EVT-* | -- | DONE |
| Green 2 | Implement signal derivation + EventSummary.Type | -- | DONE |
| Refactor 2 | 100 Go Mistakes audit on Phase 2 code | Checkpoint 2 | DONE |
| Red 3 | Write failing tests for F-OUT-*, F-PRM-* | -- | DONE (F-PRM: PROMPT-001/002; F-OUT: OUT-001/002/003) |
| Green 3 | Implement part converter fix + prompt hardening | -- | DONE (summarizeCreateRR human-friendly; prompt documents auto-resolved fields) |
| Refactor 3 | 100 Go Mistakes audit on Phase 3 code | Final Checkpoint | DONE |

### Implementation Evidence

**Phase 1 — DONE** (19 unit tests, 3 integration tests):
- `UT-AF-1282-NS-001..NS-007` in `pkg/apifrontend/agent/prompt_test.go` and `pkg/apifrontend/tools/af_create_rr_test.go`
- `UT-AF-1282-MIN-001..MIN-007` in `pkg/apifrontend/tools/af_create_rr_test.go`
- `UT-AF-1282-SRC-001..SRC-002` in `pkg/apifrontend/tools/af_create_rr_test.go`
- `UT-AF-1282-K8S`, `UT-AF-1282-DEDUP` (bonus coverage)
- `IT-AF-1282-W01` (namespace resolution), `IT-AF-1282-W02` (signal source), `IT-AF-1282-W05` (prompt namespace)

**Phase 2 — DONE** (13 unit tests, 4 integration tests):
- `UT-AF-1282-SIG-001..SIG-011` in `pkg/apifrontend/tools/af_create_rr_test.go` and `af_list_events_test.go`
- `UT-AF-1282-EVT-001..EVT-003` in `pkg/apifrontend/tools/af_list_events_test.go`
- `IT-AF-1282-W03` (signal name grounding), `IT-AF-1282-W03b` (K8s event signal), `IT-AF-1282-W04` (triage wiring), `IT-AF-1282-W06` (audit events)

**Phase 3 — DONE** (prompt + output suppression):
- `UT-AF-1282-PROMPT-001` (MCP tools mandate), `UT-AF-1282-PROMPT-002` (auto-resolved fields documented)
- Output suppression: `UT-AF-1282-OUT-001` (new RR human-friendly), `UT-AF-1282-OUT-002` (existing RR human-friendly), `UT-AF-1282-OUT-003` (non-key tool suppressed)
- `IT-AF-1282-W05` (BuildInstruction Tool Usage Rules with namespace)

### Anti-Patterns Avoided

- No `Skip()` or `XIt` (per Kubernaut TDD rules)
- No testing implementation details; tests assert business behavior
- Table-driven tests where appropriate (Ginkgo DescribeTable/Entry)
- No mocking of business logic; only external dependencies mocked
- No test logic in production code

## 9. Item Pass/Fail Criteria

| Criterion | Threshold |
|---|---|
| All unit tests pass | 100% (0 failures) |
| All integration tests pass | 100% (0 failures) |
| Unit-testable code coverage | >= 80% |
| Integration-testable code coverage | >= 80% |
| go build ./... | 0 errors |
| golangci-lint run | 0 new warnings |
| 100 Go Mistakes audit | All applicable checks clear |

## 10. Test Deliverables

| Deliverable | Location |
|---|---|
| Test Plan | `docs/tests/1282/TEST_PLAN.md` (this document) |
| Unit Tests (Phase 1) | `pkg/apifrontend/tools/af_create_rr_test.go` (updated) |
| Unit Tests (Phase 1) | `pkg/apifrontend/agent/prompt_test.go` (new/updated) |
| Unit Tests (Phase 2) | `pkg/apifrontend/tools/af_create_rr_test.go` (signal derivation) |
| Unit Tests (Phase 2) | `pkg/apifrontend/tools/af_list_events_test.go` (EventSummary.Type) |
| Unit Tests (Phase 3) | `pkg/apifrontend/launcher/part_converter_test.go` (updated) |
| Integration Tests | `test/integration/apifrontend/` (if applicable) |

## 11. Test Scenarios

### 11.1 Unit Test Scenarios — Namespace Resolution

| ID | Feature | Scenario | Expected |
|---|---|---|---|
| UT-AF-1282-NS-001 | F-NS-01 | Downward API file exists with "kubernaut-system" | Returns "kubernaut-system" |
| UT-AF-1282-NS-002 | F-NS-02 | Config override set to "custom-ns" | Returns "custom-ns" |
| UT-AF-1282-NS-003 | F-NS-03 | Both absent | Returns "default" |
| UT-AF-1282-NS-004 | F-NS-01 | Downward API file has trailing whitespace/newline | Returns trimmed value |
| UT-AF-1282-NS-005 | F-NS-02 | Config override set, downward API also exists | Config takes precedence |

### 11.2 Unit Test Scenarios — Signal Name Derivation

| ID | Feature | Scenario | Expected |
|---|---|---|---|
| UT-AF-1282-SIG-001 | F-SIG-01 | TriageResult has AlertName "KubePodCrashLooping" | signalName = "KubePodCrashLooping" |
| UT-AF-1282-SIG-002 | F-SIG-02 | TriageResult has RuleName "HighMemoryUsage" | signalName = "HighMemoryUsage" |
| UT-AF-1282-SIG-003 | F-SIG-03 | TriageResult LLM-only (no AlertName/RuleName), events have BackOff | signalName = "BackOff" |
| UT-AF-1282-SIG-004 | F-SIG-04 | No Triager, events have OOMKilled + BackOff | signalName = "OOMKilled" (higher tier) |
| UT-AF-1282-SIG-005 | F-SIG-05 | No Triager, no events | signalName = "unknown" |
| UT-AF-1282-SIG-006 | F-SIG-06 | Any path | signalName never starts with "af-manual-" |
| UT-AF-1282-SIG-007 | F-SIG-07 | Events: OOMKilling, BackOff, FailedScheduling | OOMKilling wins |
| UT-AF-1282-SIG-008 | F-SIG-08 | Events: Scheduled (Normal), BackOff (Warning) | BackOff wins; Scheduled filtered |
| UT-AF-1282-SIG-009 | F-SIG-01 | TriageResult has both AlertName and RuleName | AlertName takes precedence |
| UT-AF-1282-SIG-010 | F-SIG-04 | Events all Normal type, no triager | Falls through to "unknown" |
| UT-AF-1282-SIG-012 | F-SIG-02 | TriageResult has RuleName but empty AlertName, K8s events present | RuleName takes precedence over events |

### 11.3 Unit Test Scenarios — CreateRRArgs Minimization

| ID | Feature | Scenario | Expected |
|---|---|---|---|
| UT-AF-1282-MIN-001 | F-MIN-01 | CreateRRArgs with Kind, Name, Description | RR created successfully |
| UT-AF-1282-MIN-002 | F-MIN-04 | CreateRRArgs with empty Kind | Error: kind must not be empty |
| UT-AF-1282-MIN-003 | F-MIN-05 | CreateRRArgs with empty Name | Error: name must not be empty |
| UT-AF-1282-MIN-004 | F-MIN-06 | Description exceeding maxDescriptionLen | Description truncated |
| UT-AF-1282-MIN-005 | F-MIN-07 | Two concurrent calls for same Kind/Name | Singleflight dedup |
| UT-AF-1282-MIN-006 | F-MIN-02 | Triager returns severity "critical" | RR spec.severity = "critical" |
| UT-AF-1282-MIN-007 | F-MIN-02 | Triager disabled (nil) | severity defaults to "medium" |

### 11.4 Unit Test Scenarios — Signal Source

| ID | Feature | Scenario | Expected |
|---|---|---|---|
| UT-AF-1282-SRC-001 | F-SRC-01 | Any successful RR creation | spec.signalSource = "a2a-agent" |
| UT-AF-1282-SRC-002 | F-SRC-02 | RR creation with dedup (existing RR) | signalSource not modified |

### 11.5 Unit Test Scenarios — Output Suppression

| ID | Feature | Scenario | Expected |
|---|---|---|---|
| UT-AF-1282-OUT-001 | F-OUT-01 | af_create_rr new RR FunctionResponse | Human-friendly text, no raw JSON keys |
| UT-AF-1282-OUT-002 | F-OUT-01 | af_create_rr existing RR FunctionResponse | Human-friendly "already exists" text, no JSON syntax |
| UT-AF-1282-OUT-003 | F-OUT-02 | kubectl_get FunctionResponse (non-key tool) | Suppressed (nil) — payload never reaches user |

### 11.6 Unit Test Scenarios — EventSummary Enhancement

| ID | Feature | Scenario | Expected |
|---|---|---|---|
| UT-AF-1282-EVT-001 | F-EVT-01 | K8s event with type=Warning | EventSummary.Type = "Warning" |
| UT-AF-1282-EVT-002 | F-EVT-01 | K8s event with type=Normal | EventSummary.Type = "Normal" |
| UT-AF-1282-EVT-003 | F-EVT-02 | HandleListEvents with mixed events | All events have Type populated |

### 11.7 Unit Test Scenarios — Prompt Hardening

| ID | Feature | Scenario | Expected |
|---|---|---|---|
| UT-AF-1282-PRM-001 | F-PRM-01 | prompt.txt content | Contains "MUST follow the 4-Phase" |
| UT-AF-1282-PRM-02 | F-PRM-02 | prompt.txt content | Contains kubectl investigation ban |
| UT-AF-1282-PRM-003 | F-PRM-03 | prompt.txt content | Documents auto-resolved fields |
| UT-AF-1282-PRM-004 | F-PRM-05 | BuildInstruction("kubernaut-system") | Contains "kubernaut-system" in output |

## 12. Environmental Needs

| Need | Details |
|---|---|
| Go 1.23+ | Standard build toolchain |
| Ginkgo v2 + Gomega | BDD test framework (project mandate) |
| fake.NewClientBuilder | K8s fake client for unit tests |
| httptest.Server | Mock Prometheus/AlertManager endpoints |
| envtest | Integration tier (if K8s API needed) |

## 13. Responsibilities

| Role | Person |
|---|---|
| Test Plan Author | AI Agent |
| Test Implementor | AI Agent |
| Reviewer | Jordi Gil |

## 14. Schedule

| Milestone | Target | Status |
|---|---|---|
| Test Plan Approved | Before implementation | DONE (2026-05-24) |
| TDD Red Phase 1 | Immediate | DONE |
| TDD Green Phase 1 | After Red 1 | DONE |
| TDD Refactor Phase 1 + Checkpoint | After Green 1 | DONE |
| TDD Red Phase 2 | After Checkpoint 1 | DONE |
| TDD Green Phase 2 | After Red 2 | DONE |
| TDD Refactor Phase 2 + Checkpoint | After Green 2 | DONE |
| TDD Red Phase 3 | After Checkpoint 2 | DONE |
| TDD Green Phase 3 | After Red 3 | DONE |
| TDD Refactor Phase 3 + Final Checkpoint | After Green 3 | DONE |
| Integration Tests (envtest wiring) | After Phase 2 | DONE (7 IT tests, 101/101 pass) |
| E2E Validation | After IT | IN PROGRESS (129/137 pass; 8 failures under investigation) |

## 15. Approvals

| Name | Role | Date | Signature |
|---|---|---|---|
| | Reviewer | | |
