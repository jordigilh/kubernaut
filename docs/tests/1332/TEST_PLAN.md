# Test Plan: Intent-Based Tool Redesign (#1332)

## 1. Test Plan Identifier

**TP-1332-INTENT-TOOLS-001**

| Field | Value |
|-------|-------|
| Version | 2.0 |
| Date | 2026-05-30 |
| Author | AI Agent |
| Status | In Progress (Phase 4) |
| Business Requirement | BR-INTERACTIVE-010, BR-WORKFLOW-001 |
| Related Issues | #1293 (IS as universal signal), #1189 (A2A 4-phase) |

## 2. Introduction

This test plan covers the Intent-Based Tool Redesign feature (#1332), which replaces the internal `af_create_rr` tool with two intent-based tools:

- **`kubernaut_remediate`**: Autonomous remediation (creates RR, no IS CRD)
- **`kubernaut_investigate`** (enhanced): Interactive investigation (creates RR + IS CRD, then starts investigation)

### 2.1 Scope

- AF: Tool replacement (`af_create_rr` → `kubernaut_remediate` + enhanced `kubernaut_investigate`)
- AF: Audit callback refactoring (remove unconditional IS creation, preserve A2A correlation)
- AF: Prompt rewrite (new tool journey instructions)
- AF: Part converter updates (SSE status messages)
- AF: RBAC tool allowlist updates
- Mock-LLM: Scenario migration (`af_create_rr` → `kubernaut_remediate`)
- E2E: Full pipeline test redesign (FP-1189-002 autonomous, FP-1189-003 interactive)

### 2.2 Out of Scope

- KA changes (unchanged — receives investigation requests regardless of tool name)
- AA changes (unchanged — checks IS existence at reconcile time)
- RO changes (unchanged — creates AA based on RR existence)
- Gateway changes (unchanged — creates RRs for AlertManager signals)
- `kubernaut_get_workflow` tool (deferred to separate issue)

## 3. Test Items

| ID | Component | Item | Version |
|----|-----------|------|---------|
| TI-01 | AF | `kubernaut_remediate` tool — `HandleRemediate` handler | 1.5.1 |
| TI-02 | AF | `kubernaut_investigate` extended args — `Namespace/Kind/Name` fields | 1.5.1 |
| TI-03 | AF | `kubernaut_investigate` RR creation path (internal `HandleCreateRR`) | 1.5.1 |
| TI-04 | AF | `kubernaut_investigate` IS creation (synchronous, pre-polling) | 1.5.1 |
| TI-05 | AF | Audit callback — A2A task-to-RR correlation for both tools | 1.5.1 |
| TI-06 | AF | Audit callback — IS creation removed (no longer unconditional) | 1.5.1 |
| TI-07 | AF | `buildToolList` — `af_create_rr` removed, `kubernaut_remediate` added | 1.5.1 |
| TI-08 | AF | `prompt.txt` — new tool journey, mode detection keywords | 1.5.1 |
| TI-09 | AF | `prompt.go` — `BuildInstruction` references new tool | 1.5.1 |
| TI-10 | AF | Part converter — `kubernaut_remediate` status/summary messages | 1.5.1 |
| TI-11 | AF | RBAC — `kubernaut_remediate` in persona allowlists | 1.5.1 |
| TI-12 | Mock-LLM | `kubernaut_remediate` scenario (replaces `af_create_rr`) | 1.5.1 |
| TI-13 | E2E FP | `E2E-FP-1189-002` autonomous via `kubernaut_remediate` | 1.5.1 |
| TI-14 | E2E FP | `E2E-FP-1189-003` interactive via `kubernaut_investigate` | 1.5.1 |

## 4. Features to be Tested

### 4.1 Functional Features

| Feature | Priority | Risk |
|---------|----------|------|
| F-01: `kubernaut_remediate` creates RR without IS | P0 | High |
| F-02: `kubernaut_investigate` creates RR + IS when namespace/kind/name provided | P0 | High |
| F-03: `kubernaut_investigate` creates IS only when rr_id provided (existing RR) | P0 | High |
| F-04: `kubernaut_investigate` blocks until investigation completes (blocking mode) | P0 | Medium |
| F-05: A2A task-to-RR correlation preserved for both tools | P1 | Medium |
| F-06: `af_create_rr` completely removed from tool registry | P0 | Low |
| F-07: Prompt instructs LLM to use correct tool per intent | P1 | Medium |
| F-08: Part converter shows correct status messages | P2 | Low |
| F-09: RBAC permits `kubernaut_remediate` for sre/ai-orchestrator | P1 | Low |

### 4.2 Non-Functional Features

| Feature | Priority | Risk |
|---------|----------|------|
| NF-01: RR + IS creation atomicity (cleanup on failure) | P1 | Medium |
| NF-02: `HandleAwaitSession` 60s timeout sufficient for pipeline | P2 | Low |
| NF-03: Singleflight deduplication preserved in new tool | P1 | Low |
| NF-04: Prompt prevents LLM from bypassing kubernaut tools with kubectl DIY investigation | P1 | Medium |
| NF-05: Prompt decision algorithm resolves ambiguous intent without oscillation | P1 | Medium |

## 5. Approach

### 5.1 Test Pyramid (Pyramid Invariant)

> UT proves logic. IT proves wiring. E2E proves the journey.

| Tier | Target | Proves |
|------|--------|--------|
| **UT** | >= 80% of unit-testable code | Tool handler logic, argument validation, error paths |
| **IT** | >= 80% of integration-testable code | Tool registration in ADK, audit callback wiring, prompt assembly |
| **E2E** | >= 80% of journey-testable code | Full A2A → mock-LLM → tool → pipeline flow |

### 5.2 TDD Methodology

Each test scenario follows RED → GREEN → REFACTOR:

1. **RED**: Write failing test with correct assertions
2. **GREEN**: Minimal implementation to pass (+ CHECKPOINT W: wiring verification)
3. **REFACTOR**: Improve code quality, validate against 100 Go mistakes

## 6. Pass/Fail Criteria

### 6.1 Pass Criteria

- All UT/IT/E2E tests pass
- `go build ./...` succeeds with zero errors
- `golangci-lint run` has no new violations
- E2E-FP-1189-002 (autonomous) completes without IS creation
- E2E-FP-1189-003 (interactive) completes with IS creation and full pipeline
- No references to `af_create_rr` in production code tool registry

### 6.2 Fail Criteria

- Any existing test regresses
- IS CRD created during autonomous remediation flow
- IS CRD NOT created during interactive investigation flow
- A2A task-to-RR correlation lost (Issue #1189 AC 12)

## 7. Test Scenarios

### 7.1 Unit Tests — `kubernaut_remediate` (TI-01)

| ID | Scenario | Input | Expected | Status |
|----|----------|-------|----------|--------|
| UT-AF-1332-001 | Create RR with valid namespace/kind/name | `{ns: "prod", kind: "Deployment", name: "web"}` | RR created, `rr_id` returned, no IS | ✅ |
| UT-AF-1332-002 | Deduplication — existing non-terminal RR | Same fingerprint as existing RR | `already_exists=true`, existing `rr_id` | ✅ |
| UT-AF-1332-003 | Invalid namespace rejected | `{ns: "", kind: "Deployment", name: "web"}` | `ErrInvalidInput` | ✅ |
| UT-AF-1332-004 | Invalid kind rejected | `{ns: "prod", kind: "", name: "web"}` | `ErrInvalidInput` | ✅ |
| UT-AF-1332-005 | Invalid name rejected | `{ns: "prod", kind: "Deployment", name: ""}` | `ErrInvalidInput` | ✅ |
| UT-AF-1332-006 | K8s client unavailable | `client=nil` | `ErrK8sUnavailable` | ✅ |
| UT-AF-1332-007 | Severity triage wired | Triager provided | RR has triage-resolved severity | ✅ |
| UT-AF-1332-008 | Existing rr_id path | `{rr_id: "rr-abc"}` | Looks up existing RR, returns status | ✅ |

### 7.2 Unit Tests — `kubernaut_investigate` Enhanced (TI-02, TI-03, TI-04)

| ID | Scenario | Input | Expected | Status |
|----|----------|-------|----------|--------|
| UT-AF-1332-010 | Investigation with existing rr_id | `{rr_id: "rr-abc"}` | IS created, then polls AA, starts investigation | ✅ |
| UT-AF-1332-011 | Investigation with namespace/kind/name (new RR) | `{ns, kind, name}` | RR created, IS created, polls AA | ✅ |
| UT-AF-1332-012 | Empty args rejected | `{}` | Error: "rr_id or namespace/kind/name required" | ✅ |
| UT-AF-1332-013 | Partial args rejected | `{ns: "prod"}` (missing kind/name) | Error: "kind and name required" | ✅ |
| UT-AF-1332-014 | RR creation failure — no IS created | HandleCreateRR returns error | Error propagated, no IS cleanup needed | ✅ |
| UT-AF-1332-015 | IS creation failure after RR — RR cleaned up | InitializeSessionByRR fails | RR deleted, error returned | ✅ |
| UT-AF-1332-016 | SA caller blocked from IS creation | `identity.IsServiceAccount=true` | Error: "interactive sessions cannot be created by service accounts" | ✅ |
| UT-AF-1332-017 | Blocking mode returns RCA summary | Events channel completes | `{session_id, status: "completed", summary}` | ✅ |

### 7.3 Unit Tests — Audit Callback (TI-05, TI-06)

| ID | Scenario | Input | Expected | Status |
|----|----------|-------|----------|--------|
| UT-AF-1332-020 | Correlation on kubernaut_remediate success | Tool=remediate, output has rr_id | `sc.RRName` and `sc.RRNamespace` set | ✅ |
| UT-AF-1332-021 | Correlation on kubernaut_investigate success | Tool=investigate, output has rr_id | `sc.RRName` and `sc.RRNamespace` set | ✅ |
| UT-AF-1332-022 | NO IS creation on kubernaut_remediate | Tool=remediate, sessionSvc provided | `InitializeSessionByRR` NOT called | ✅ |
| UT-AF-1332-023 | NO IS creation on kubernaut_investigate | Tool=investigate, sessionSvc provided | `InitializeSessionByRR` NOT called (IS created inside tool) | ✅ |
| UT-AF-1332-024 | af_create_rr no longer triggers callback | Tool=af_create_rr (hypothetical) | No correlation, no IS | ✅ |

### 7.4 Unit Tests — Part Converter (TI-10)

| ID | Scenario | Input | Expected | Status |
|----|----------|-------|----------|--------|
| UT-AF-1332-030 | Remediate status message | FunctionCall name=kubernaut_remediate | "Creating remediation request..." | ✅ |
| UT-AF-1332-031 | Remediate summary — new RR | FunctionResponse with rr_id | "Remediation request created: ..." | ✅ |
| UT-AF-1332-032 | Remediate summary — existing RR | FunctionResponse with already_exists=true | "Remediation request already exists: ..." | ✅ |

### 7.5 Unit Tests — Prompt (TI-08, TI-09)

| ID | Scenario | Input | Expected | Status |
|----|----------|-------|----------|--------|
| UT-AF-1332-035 | Prompt contains kubernaut_remediate | BuildInstruction("kubernaut-system") | Contains "kubernaut_remediate" | ✅ |
| UT-AF-1332-036 | Prompt does NOT contain af_create_rr | BuildInstruction("kubernaut-system") | Does NOT contain "af_create_rr" | ✅ |
| UT-AF-1332-037 | Prompt mode detection keywords | defaultInstruction() | Autonomous→remediate, Interactive→investigate | ✅ |
| UT-AF-1332-038 | Prompt contains kubectl bypass prevention rule | defaultInstruction() | Contains "NEVER use kubectl tools to perform root-cause analysis" | ✅ |
| UT-AF-1332-039 | Prompt contains decision algorithm | defaultInstruction() | Contains "Does the user just want it fixed" | ✅ |
| UT-AF-1332-040 | Prompt WHAT/WHY boundary in observation mode | defaultInstruction() | Contains "kubectl queries answer WHAT" | ✅ |

### 7.6 Integration Tests — Wiring (TI-07, TI-11)

| ID | Scenario | Entry Point | Expected | Status |
|----|----------|-------------|----------|--------|
| IT-AF-1332-W01 | `kubernaut_remediate` registered in ADK | `buildToolList()` | Tool present with correct name | Pending |
| IT-AF-1332-W02 | `kubernaut_remediate` creates RR in envtest | `HandleRemediate` via envtest dynamic client | RR CRD exists in controller namespace | Pending |
| IT-AF-1332-W03 | `kubernaut_investigate` creates RR+IS in envtest | `HandleInvestigationMCPWithRegistry` | Both RR and IS CRDs exist | Pending |
| IT-AF-1332-W04 | `af_create_rr` NOT in tool list | `buildToolList()` | Tool absent | Pending |
| IT-AF-1332-W05 | RBAC allows `kubernaut_remediate` for sre | RBAC guard with sre persona | Permitted | Pending |
| IT-AF-1332-W06 | Audit callback emits correlation on remediate | After-callback with tool=kubernaut_remediate | `a2a_task_id` + `rr_id` in audit detail | Pending |

### 7.7 E2E Tests — Full Pipeline (TI-13, TI-14)

| ID | Scenario | Components | Expected | Status |
|----|----------|------------|----------|--------|
| E2E-FP-1332-001 | Autonomous: A2A → kubernaut_remediate → pipeline | AF → mock-LLM → RO → SP → AA → KA → WE | RR created, NO IS, pipeline completes | Pending |
| E2E-FP-1332-002 | Interactive: A2A → kubernaut_investigate → pipeline | AF → mock-LLM → RO → SP → AA(interactive) → KA → WE | RR + IS created, AA submits interactive=true | Pending |

### 7.8 E2E Tests — AF (existing, updated)

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| E2E-AF-1332-001 | Severity triage via kubernaut_remediate | RR created with correct severity | Pending |
| E2E-AF-1332-002 | A2A RBAC: kubernaut_remediate permitted for sre | 200 OK, RR created | Pending |
| E2E-AF-1332-003 | A2A RBAC: kubernaut_remediate denied for approver | RBAC rejection | Pending |

## 8. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `kubernaut_remediate` | `buildToolList()` | `pkg/apifrontend/agent/root.go` | IT-AF-1332-W01 |
| `HandleRemediate` | `NewRemediateTool()` | `pkg/apifrontend/tools/ka_remediate.go` | IT-AF-1332-W02 |
| `kubernaut_investigate` RR+IS path | `HandleInvestigationMCPWithRegistry` | `pkg/apifrontend/tools/ka_investigate_mcp.go` | IT-AF-1332-W03 |
| A2A correlation (remediate) | `newAuditToolCallback` | `pkg/apifrontend/agent/root.go` | IT-AF-1332-W06 |
| Part converter (remediate) | `toolStatusMessages` map | `pkg/apifrontend/launcher/part_converter.go` | UT-AF-1332-030 |
| RBAC allowlist | Helm values + E2E RBAC | `charts/kubernaut/values.yaml` | IT-AF-1332-W05 |
| Mock-LLM scenario | `DefaultRegistryFull` | `test/services/mock-llm/scenarios/` | E2E-FP-1332-001 |

## 9. Environmental Needs

| Environment | Purpose | Configuration |
|-------------|---------|---------------|
| Local (unit) | UT execution | `go test ./pkg/apifrontend/...` |
| envtest (integration) | IT with real K8s API | `test/integration/apifrontend/` with envtest |
| Kind cluster (E2E) | Full pipeline | `make test-e2e-fullpipeline` |

## 10. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Mock-LLM keyword collision (autonomous vs interactive) | Medium | High | Distinct keywords per scenario; confidence tiers |
| Blocking `kubernaut_investigate` timeout in Kind | Medium | Medium | 90s await + 5min A2A invoke timeout |
| A2A correlation loss during refactor | Low | High | Explicit UT for callback (UT-AF-1332-020/021) |
| Phase guard blocks interactive flow | Low | High | UT validates phase_guard unchanged |
| Cross-test contamination (FP RRs) | Medium | Medium | Namespace-scoped `fpWaitForRRWithTargetNS` |

## 11. Schedule

| Phase | Duration | Deliverable | Status |
|-------|----------|-------------|--------|
| Phase 1-3: Core logic (UT) | Complete | 32 UT scenarios passing | ✅ |
| Phase 4A: RBAC + Mock-LLM (TDD Red) | 30 min | Failing IT/E2E tests for RBAC + mock scenarios | 🔲 |
| Phase 4B: RBAC + Mock-LLM (TDD Green) | 1 hour | RBAC updated, mock-LLM migrated, tests pass | 🔲 |
| Phase 4C: E2E Redesign (TDD Red) | 30 min | Failing E2E tests (07/08/09 updated expectations) | 🔲 |
| Phase 4D: E2E Redesign (TDD Green) | 1-2 hours | E2E tests passing in Kind | 🔲 |
| Phase 4E: Integration Wiring (TDD Red) | 30 min | IT-AF-1332-W01..W06 failing | 🔲 |
| Phase 4F: Integration Wiring (TDD Green) | 1 hour | IT tests pass in envtest | 🔲 |
| Phase 4G: TDD Refactor | 30 min | Stale comments, docs, 100 Go mistakes | 🔲 |
| CHECKPOINT 4: Final GA Audit | 15 min | All dimensions green, confidence ≥95% | 🔲 |

## 12. Approvals

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Developer | AI Agent | 2026-05-29 | Phase 1-3 ✅ |
| Developer | AI Agent | 2026-05-30 | Phase 4 in progress |
| Reviewer | — | — | Pending |
