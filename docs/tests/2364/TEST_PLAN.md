# Test Plan: #2364 Ambient fleet-field tolerance in AF tool schemas

## 1. Introduction

### 1.1 Purpose

Fleet-hinted turns die at ADK strict schema validation: the console appends a
fleet cluster hint to investigate messages, the model propagates `cluster_id`
(occasionally `rr_id`) into tool calls, and `functiontool` (`additionalProperties:
false` at every struct level, ADK v2.0.0 + jsonschema-go v0.4.3) rejects the
call. After 5 consecutive failures the reinvoking-runner circuit breaker trips
and no `investigation_summary` artifact is emitted. Live evidence: 0 successful
`kubernaut_present_decision` calls vs 15 failures in 2h (14x `cluster_id`, 1x
`rr_id`+`cluster_id`).

### 1.2 Objectives

1. Every AF lifecycle tool schema tolerates ambient fleet/context fields
   (`cluster_id`, `rr_id`, `session_id`) as ignored `omitempty` hints.
2. Turn survival: a fleet-hinted `present_decision` / `discover` / `select` /
   `watch` call executes its handler and emits its artifact.
3. No regression: required fields stay required; handlers ignore new fields
   (never routing/authz); zero-value omission preserved.

### 1.3 Success Metrics

- UT + IT suites for touched packages green; `go build ./...` clean.
- New `UT-2364-*` / `IT-2364-*` all passing; no `XIt`/`Skip`.
- Fleet E2E lifecycle emits `investigation_summary` (or documents live-infra
  run in CI where kind hub+spoke exists).

## 2. References

### 2.1 Authority

- Issue #2364 (+ #2073/#2074 precedent: `tool_calls_count`/`llm_turns` omitempty).
- `BR-INTERACTIVE` (decision presentation), `BR-FLEET-003` (fleet context must
  not break selection close-out), `BR-AUDIT-021-030` (selection audit trail).
- FedRAMP SI-10 (input validation), AU-3 (audit content), AC-6 (least
  privilege); SOC2 CC7.2 (reconstruction); OWASP ASVS 5.1 (validation), 5.5.2
  (untrusted-field isolation).

### 2.2 Cross-References

- `pkg/apifrontend/tools/ka_tools.go` (PresentDecision/Discover/Select/RCAData/TargetInfo),
  `crd_tools_watch.go` (WatchArgs), `ka_investigate_mcp.go`, `ka_interactive.go`,
  `crd_tools.go`, `ds_tools.go`, `af_list_events.go`,
  `pkg/apifrontend/agent/prompt.txt:87`, `phase_guard.go`.

## 3. Risks & Mitigations

| Risk | Mitigation | Proving test |
|---|---|---|
| Spoofed `cluster_id` used for routing/authz | Handlers ignore new fields; authority stays `RRContext` + injected `Namespace` | UT-2364-H* (handler ignores extras) |
| Over-permissive schema masks malformed input | Core fields stay required; malformed payloads still rejected | UT-2364-S* (required-set intact) |
| Nested rejection persists | `TargetInfo`/`RCAData` also gain `cluster_id` (infer.go proves nested strictness) | UT-2364-N*, IT-2364-001 |
| Prompt keeps instructing cross-contamination | `prompt.txt:87` reworded (defense-in-depth) | Manual review |

## 4. Scope

### 4.1 Features to be Tested

Tier 1: `present_decision`, `discover_workflows`, `select_workflow`, `watch`
(+ nested `RCAData`/`TargetInfo`). Tier 2: `investigate`, interactive
actions, `complete_no_action`, `get_remediation`, `cancel_remediation`,
`get_audit_trail`, `list_events`. Tier 3 folds in iff mechanical.

### 4.2 Features Not to be Tested

Live kind hub+spoke fleet run in this session if no cluster is available
(spec written; execution deferred to CI with note). Console-side hint
formatting (healthy per Playwright repro in issue).

## 5. Approach

### 5.1 Coverage Policy

UT proves tolerant-schema logic; IT proves production `functiontool`+`phase_guard`
wiring; E2E proves the fleet journey. No `testing.T` (Ginkgo only).

### 5.4 Pass/Fail Criteria

Pass: all listed scenarios green, build+lint clean, no new anti-patterns.
Fail: any scenario red, or any touched handler branches on the new fields.

## 6. Test Items

### 6.1 Unit-Testable Code

Arg-struct schemas (`jsonschema.For`), JSON omission/round-trip,
handler-ignores-extras.

### 6.2 Integration-Testable Code

Real `functiontool.New` tools invoked via `phase_guard` before-callback +
`Run` with ambient extras (2092-test pattern). E2E fleet lifecycle.

## 7. BR Coverage Matrix

| BR / Control | Proving test |
|---|---|
| BR-INTERACTIVE-001 (decision presentation) | UT-2364-001, IT-2364-001 |
| BR-FLEET-003 (fleet must not break close-out) | UT-2364-002, IT-2364-002/003, E2E-2364-001 |
| BR-AUDIT-021-030 (selection audit trail) | IT-2364-001 (artifact emitted) |
| SI-10 / ASVS 5.1.1, 5.1.4 | UT-2364-S*, UT-2364-001 |
| AU-3 / CC7.2 | IT-2364-*, E2E-2364-001 |
| AC-6 / ASVS 5.5.2 | UT-2364-H* |

## 8. Test Scenarios

### Test ID Naming Convention

`UT-2364-NNN` (unit), `IT-2364-NNN` (integration), `E2E-2364-NNN` (e2e).

### Tier 1: Unit Tests

| ID | Business behavior | Control |
|---|---|---|
| UT-2364-001 | `PresentDecisionArgs` schema: `cluster_id`/`rr_id` in Properties, absent from Required; `session_id`/`summary`/`rca` still required | SI-10 |
| UT-2364-002 | `DiscoverWorkflowsArgs`/`SelectWorkflowArgs` schemas tolerate `cluster_id`+`session_id`; `rr_id` still required | SI-10, BR-FLEET-003 |
| UT-2364-003 | `WatchArgs` schema tolerates `cluster_id`+`session_id` | SI-10 |
| UT-2364-004 | Tier-2 structs (`InvestigateMCPArgs`, `InteractiveActionArgs`, `CompleteNoActionArgs`, `GetRemediationArgs`, `CancelRemediationArgs`, `GetAuditTrailArgs`, `ListEventsArgs`) tolerate ambient trio | SI-10 |
| UT-2364-N01 | Nested `RCAData`/`TargetInfo` tolerate `cluster_id` | SI-10 |
| UT-2364-S01 | Zero-value ambient fields omitted from JSON; genuine values round-trip (no #2074 regression) | AU-3 |
| UT-2364-H01..H03 | `HandlePresentDecision`/`HandleDiscoverWorkflows`/`HandleSelectWorkflow` ignore ambient extras (same result with/without) | AC-6 |

### Tier 2: Integration Tests

| ID | Business behavior | Control |
|---|---|---|
| IT-2364-001 | Real `kubernaut_present_decision` tool via `phase_guard`+`Run` with `cluster_id`+`rr_id` extras succeeds end-to-end | SI-10, AU-3, BR-INTERACTIVE-001 |
| IT-2364-002 | Real `discover`/`select` tools with `cluster_id`+`session_id` extras survive validation | SI-10, BR-FLEET-003 |
| IT-2364-003 | `watch` tool schema tolerates ambient extras via real tool | SI-10 |

### Tier 3: E2E

| ID | Journey | Control |
|---|---|---|
| E2E-2364-001 | Fleet-hinted `investigate→discover→select→present→watch` emits `investigation_summary`, reconstructable by `correlation_id` | CC7.2, AU-3 |

**Execution status:** `IT-2364-001..003` pass against the real ADK
`functiontool` path. `E2E-2364-001` remains infrastructure-gated: the existing
fleet journey (`test/e2e/fullpipeline/10_af_fleet_cluster_id_test.go`) requires
a live hub+spoke fleet and is skipped when Fleet is unavailable. No new skipped
spec was added; the existing fleet E2E is the required follow-up proving the
complete journey and audit reconstruction in a fleet-enabled environment.
