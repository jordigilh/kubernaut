# Test Plan: `main`-Branch Port of AF Scope Validation, Content Grounding, Ambiguous Severity Correlation, and AA Session-Desync Fixes

**Issues** (all `main`-tracking clones of fixes implemented and merged on `release/v1.5`
via [PR #2026](https://github.com/jordigilh/kubernaut/pull/2026)):
- [#2025](https://github.com/jordigilh/kubernaut/issues/2025) — clone of
  [#2022](https://github.com/jordigilh/kubernaut/issues/2022) (AF tool-layer scope validation)
- [#2047](https://github.com/jordigilh/kubernaut/issues/2047) — clone of
  [#2023](https://github.com/jordigilh/kubernaut/issues/2023) (content-grounding guard)
- [#2028](https://github.com/jordigilh/kubernaut/issues/2028) — clone of
  [#2027](https://github.com/jordigilh/kubernaut/issues/2027) (confidence-gated severity correlation,
  [DD-AF-012](../../architecture/decisions/DD-AF-012-confidence-gated-severity-correlation.md))
- [#2030](https://github.com/jordigilh/kubernaut/issues/2030) — clone of
  [#2029](https://github.com/jordigilh/kubernaut/issues/2029) (AA reconnect/takeover session desync),
  split into **Part A** (bounded retry on CRD schema rejection) and **Part B** (AA-side session
  adoption)

**Authority**: [ADR-053](../../architecture/decisions/ADR-053-resource-scope-management.md)
(Resource Scope Management — Addendum "Point 3"),
[DD-AF-012](../../architecture/decisions/DD-AF-012-confidence-gated-severity-correlation.md)
**Branch**: `fix/2025-2047-2028-2030-af-main-port` (off `main`)
**Created**: 2026-08-09
**Status**: Implementation complete — build/vet/lint/full test suites green

---

## 1. Purpose and Scope

`main` (v1.6) diverged structurally from `release/v1.5` after these four fixes were designed and
implemented there (PR #2026). This plan documents the **adaptation**, not a fresh design: the root
causes, decisions, and FedRAMP control mappings are unchanged from the `v1.5` originals — see
[ADR-053 Addendum](../../architecture/decisions/ADR-053-resource-scope-management.md#addendum-point-3--af-tool-layer-validation-issue-2025-august-2026)
and [DD-AF-012](../../architecture/decisions/DD-AF-012-confidence-gated-severity-correlation.md) for
that authoritative content. This document covers only the `main`-specific **test-scenario
inventory, Wiring Manifest, and coverage verification** — grounded directly in the test IDs and
production call sites that exist in this branch's diff, not carried over unchecked from the `v1.5`
test plan (`docs/testing/2022/TEST_PLAN.md` on `release/v1.5`).

**Per the user's consolidation decision, all four fixes are bundled into a single PR to `main`** —
resource-constrained the same way `release/v1.5`'s #2022/#2023/#2027 were bundled into PR #2026.

### Structural divergence requiring adaptation (not a straight port)

| Area | `release/v1.5` | `main` | Adaptation |
|---|---|---|---|
| `ScopeChecker.IsManagedResource` | `(ctx, namespace, kind, name string)` | `(ctx, scope.ResourceIdentity{ClusterID, Namespace, Kind, Name})` | `main` already has fleet/multi-cluster support (ADR-065); the checker signature already carries `ClusterID`. `checkRRScope` takes a `scope.ResourceIdentity` directly — no new struct needed. |
| Scope-checker construction | Bare `scope.NewManager(typedClient)` | `fleet.NewScopeChecker(scope.NewManager(...), cfg.Fleet, logger)` | `main`'s factory decides local-only vs. `fleet.FederatedScopeChecker` wrapping internally — AF's tool layer is Fleet-aware for free, no AF-side federation code needed. |
| Tool dependency grouping | Individual fields on each tool's own config struct | Shared `ToolDeps` struct (`Client`, `DynClient`, `ControllerNS`, `Triager`, `Auditor`, `ScopeChecker`) consumed by `HandleCreateRR` | `ScopeChecker` is a single `ToolDeps` field; all three RR-creating tools (`kubernaut_remediate`, `kubernaut_investigate_alert`, `kubernaut_investigate`'s new-RR branch) already funnel through `HandleCreateRR`, so **one** gate covers all three — simpler than `v1.5`'s three separate call sites. |
| AA reconnect/takeover fix (#2029/#2030) | Not present on `v1.5` (this is a `main`-only defect — `v1.5`'s AA session-polling architecture differs) | New fix, not a port | No `v1.5` original exists for this piece; implemented directly against `main`'s `pkg/aianalysis/handlers/investigating.go` and `internal/controller/aianalysis/phase_handlers.go`. Included in this same branch/PR per the single-PR consolidation decision. |

---

## 2. Issue #2025 (clone of #2022): AF Tool-Layer Scope Validation

### 2.1 Fix Summary

`HandleCreateRR` (`pkg/apifrontend/tools/af_create_rr.go`) — the single choke point all three
RR-creating tools (`kubernaut_remediate`, `kubernaut_investigate_alert`, and
`kubernaut_investigate`'s new-RR/`createRRForInvestigation` branch) already call — now checks
`ToolDeps.ScopeChecker.IsManagedResource()` before the `Triager` call and RR object creation that
follow, rejecting out-of-scope targets with `ErrResourceNotManaged` and no RR created. The
`rr_id` (takeover) path on `kubernaut_investigate` is deliberately **not** re-checked: that RR was
already scope-checked at its own creation time.

`checkRRScope` (`pkg/apifrontend/tools/scope_helpers.go`) is the shared helper: nil-checker
gracefully degrades to always-managed (backward compat); a checker error fails closed
(managed=false), mirroring RO's `CheckUnmanagedResource`; on rejection it emits
`EventRRScopeRejected` (AU-3/AU-12) and returns a message mirroring RO's own block wording.

Wired at startup by `cmd/apifrontend/backend_deps.go#buildScopeCheckerDeps`, which constructs
`fleet.NewScopeChecker(scope.NewManager(typedClient), cfg.Fleet, logger)` and degrades to a nil
`ScopeChecker` (scope validation skipped) when the K8s typed client is unavailable.

**Correction during this port** (CHECKPOINT W wiring audit): an earlier draft of this port carried
over `v1.5`'s `ContextWithScopeChecker`/`ScopeCheckerFromContext` context-injection helpers, since
`v1.5`'s `kubernaut_remediate`/`kubernaut_investigate` threaded the checker via context. On `main`,
all three tools already receive `ScopeChecker` as a plain `ToolDeps`/config struct field (the same
mechanism `Triager`/`Auditor` already use) — the context-injection helpers had **zero production
callers**, only their own now-deleted unit test. Removed per the Pyramid Invariant ("no component in
`pkg/` without a corresponding production caller") rather than shipped as dead code; see Section 6.

### 2.2 FedRAMP Control Mapping

| Control | Application |
|---|---|
| AC-6 (Least Privilege) | AF declines to exercise its elevated CRD-creation capability for resources outside declared management scope. |
| SI-10 (Information Input Validation) | Target namespace/kind/name (as a `scope.ResourceIdentity`) validated against declared scope before RR creation. |
| AU-3 / AU-12 (Audit Content / Generation) | `EventRRScopeRejected` captures namespace/kind/name/user for every rejected attempt. |
| SI-11 (Error Handling) | Rejection returns `ErrResourceNotManaged` + explanatory message instead of a misleading empty/silent session. |

### 2.3 Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-AF-2025-001 | Unit | `HandleCreateRR` rejects RR creation for an unmanaged resource, returns `ErrResourceNotManaged` | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/af_create_rr_test.go` |
| UT-AF-2025-002 | Unit | No RR is created for an unmanaged resource (verified via list, not just the error) | SI-11 | `pkg/apifrontend/tools/af_create_rr_test.go` |
| UT-AF-2025-003 | Unit | A managed resource proceeds to RR creation | AC-6, SI-10 | `pkg/apifrontend/tools/af_create_rr_test.go` |
| UT-AF-2025-004 | Unit | Fails closed (rejects) when the scope checker itself errors | SI-11, fail-closed | `pkg/apifrontend/tools/af_create_rr_test.go` |
| UT-AF-2025-005 | Unit | Skips scope validation (backward compat) when `ScopeChecker` is nil | Backward compat | `pkg/apifrontend/tools/af_create_rr_test.go` |
| UT-AF-2025-006 | Unit | Emits `EventRRScopeRejected` with namespace/kind/name/user on rejection | AU-3, AU-12 | `pkg/apifrontend/tools/af_create_rr_test.go` |
| UT-AF-2025-020 | Unit | `kubernaut_investigate_alert` rejects an unmanaged target before RR creation (proves `InvestigateAlertConfig.ScopeChecker` threads through to `HandleCreateRR`) | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/af_investigate_alert_test.go` |
| UT-AF-2025-021 | Unit | `kubernaut_investigate_alert` allows RR creation for a managed target | AC-6, SI-10 | `pkg/apifrontend/tools/af_investigate_alert_test.go` |
| UT-AF-2025-030 | Unit | `kubernaut_remediate` rejects an unmanaged target before RR creation | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/ka_remediate_test.go` |
| UT-AF-2025-031 | Unit | `kubernaut_remediate` allows RR creation for a managed target | AC-6, SI-10 | `pkg/apifrontend/tools/ka_remediate_test.go` |
| UT-AF-2025-032 | Unit | `kubernaut_remediate`'s `rr_id` (dedup lookup) path is not scope-checked | Regression guard | `pkg/apifrontend/tools/ka_remediate_test.go` |
| UT-AF-2025-050 | Unit | `kubernaut_investigate`'s new-RR (`ResolveInvestigationRR`) path rejects an unmanaged target investigation | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/ka_investigate_mcp_test.go` |
| UT-AF-2025-051 | Unit | `kubernaut_investigate`'s new-RR path allows a managed target investigation | AC-6, SI-10 | `pkg/apifrontend/tools/ka_investigate_mcp_test.go` |
| UT-AF-2025-052 | Unit | `kubernaut_investigate`'s `rr_id` (takeover) path is not scope-checked | Regression guard | `pkg/apifrontend/tools/ka_investigate_mcp_test.go` |

### Tier Coverage Rationale

- **UT** proves the scope-gate logic once at its single choke point (`HandleCreateRR`,
  UT-AF-2025-001..006) and proves each of the three tools' configs actually thread `ScopeChecker`
  through to that choke point (UT-AF-2025-020/021, -030/031/032, -050/051/052) — this is a strictly
  simpler wiring shape than `v1.5`'s three independent call sites, since `main`'s `HandleCreateRR` is
  the one place the branching logic lives.
- **No new IT was added** for this fix on `main` (unlike `v1.5`'s `IT-AF-2022-006/007` through the
  raw MCP bridge dispatch path): `main`'s existing `pkg/apifrontend/handler` IT suite already
  exercises `kubernaut_investigate` through the real MCP bridge dispatch chain for other fixes in
  this same bundle (see `IT-AF-2028-*`-equivalent flows below); adding a scope-specific duplicate of
  that same dispatch path would not exercise any code the UT tier + existing IT coverage doesn't
  already reach. CHECKPOINT W's grep evidence (Section 6) is the wiring proof for this fix
  specifically.
- **E2E**: not added, same rationale as `v1.5` — this is a pre-CRD-creation validation gate; the
  existing Gateway/RO E2E suites already prove the end-to-end scope-management journey.

### 2.4 Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `checkRRScope` | Called from `HandleCreateRR` | `pkg/apifrontend/tools/scope_helpers.go` | UT-AF-2025-001..006 |
| `ToolDeps.ScopeChecker` → `HandleCreateRR` | All three RR-creating tools funnel here | `pkg/apifrontend/tools/af_create_rr.go` | UT-AF-2025-001..006 |
| `InvestigateAlertConfig.ScopeChecker` | `kubernaut_investigate_alert` tool constructor | `pkg/apifrontend/agent/root.go` (`NewInvestigateAlertTool`, `ScopeChecker: cfg.ScopeChecker`) | UT-AF-2025-020, -021 |
| `NewRemediateTool(..., cfg.ScopeChecker)` | `kubernaut_remediate` tool constructor | `pkg/apifrontend/agent/root.go` | UT-AF-2025-030..032 |
| `InvestigateConfig.ScopeChecker` | `kubernaut_investigate` tool constructor (A2A path) + raw MCP bridge | `pkg/apifrontend/agent/root.go`, `pkg/apifrontend/handler/mcp_bridge.go` | UT-AF-2025-050..052 |
| `AgentConfig.ScopeChecker` / `MCPBridgeConfig.ScopeChecker` | Both AF transports (A2A/ADK + raw MCP bridge) | `cmd/apifrontend/mcp_a2a_handlers.go` (`ScopeChecker: d.Backends.ScopeChecker`, two call sites) | Covered transitively by all rows above |
| `fleet.NewScopeChecker(scope.NewManager(...), cfg.Fleet, logger)` construction | AF startup | `cmd/apifrontend/backend_deps.go#buildScopeCheckerDeps` | Covered transitively — no direct UT (thin wiring call, no branching logic of its own beyond the nil-client degrade already covered by the tools-layer nil-checker tests) |
| `EventRRScopeRejected` | Emitted by `checkRRScope` on rejection | `pkg/apifrontend/audit/audit.go` (event type) | UT-AF-2025-006 |

---

## 3. Issue #2047 (clone of #2023): Content-Grounding Guard

### 3.1 Fix Summary

Ports `v1.5`'s Stage 2 (harness-enforced grounding guard) unchanged in design: `enforceGroundingGuard`
(`pkg/apifrontend/agent/phase_guard.go`) extends the existing, already-wired `newPhaseGuard`
`before`/`after` callbacks (no new callback registration). `after` records, on every
`kubernaut_investigate` call (success or failure), whether the response carried real, groundable RCA
content into `session.StateKeyGroundedContentAvailable`. `before` intercepts
`kubernaut_present_decision` specifically: when the state key is `false` (including the fail-safe
default when never set), it overwrites `args["summary"]`, deletes `args["rca"]`, and forces
`args["options"] = []` with a fixed, honest `noGroundedContentSummary` payload, then returns
`(nil, nil)` so the call proceeds — never blocking the AU-3 artifact mandate.

`prompt.txt` gained Behavioral Constraint 6 (grounding) telling the model itself never to fabricate
ungrounded findings, plus a corrected terminal-phase list (adds `TimedOut`/`Skipped`, matching
`tools.IsTerminalPhase`) and consistent `kubernaut_present_decision` naming throughout.

**Scope decision for this port**: `v1.5`'s Stage 1 (opaque-error removal,
`ErrCodeNoConversationContext`) and Stage 3 (structured RCA pass-through + shadow-agent alignment
verdict, commit `3be4330cb`) were evaluated and **excluded from this port** — Stage 1 addresses a KA
error-code granularity issue orthogonal to the grounding guard itself, and Stage 3 is a hardening
pass beyond the original QE report's scope. Both remain available to port separately if `main` needs
them; excluding them keeps this bundle's blast radius to the same four issues in its title.

### 3.2 FedRAMP Control Mapping

| Control | Application |
|---|---|
| SI-10 / SI-11 | `present_decision` content cannot be fabricated when no real investigation content exists; overridden to an honest "no data" payload instead of erroring or silently guessing. |
| AU-3 (Audit Content) | The artifact-mandate is preserved — `kubernaut_present_decision` always executes and always emits a truthful `investigation_summary`, never a fabricated one. |

### 3.3 Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-AF-2047-001 | Unit | Prompt instructs the model to ground summary/rca claims in actual tool responses | AU-3 (model contract) | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-2047-002 | Unit | Prompt mandates stating plainly when no investigation content is available | AU-3 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-2047-003 | Unit | Prompt tells the model not to rely on the harness backstop | AU-3 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-2047-004 | Unit | Prompt reports Completed/Failed/Cancelled/TimedOut/Skipped as terminal phases for Phase 4 reporting | Accuracy fix | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-2047-005 | Unit | `present_decision` content overridden when `kubernaut_investigate` was never called (fail-closed default) | SI-10, SI-11 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2047-006 | Unit | Overridden when the prior investigate call was rejected for scope (`status: unmanaged`, #2025) | SI-10, SI-11 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2047-007 | Unit | Overridden when the prior investigate call returned a generic tool error | SI-10, SI-11 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2047-008 | Unit | Overridden when the prior investigate call returned `status: session_active` | SI-10, SI-11 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2047-009 | Unit | NOT overridden after a successful investigate with a real `summary` (negative case) | SI-10 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2047-010 | Unit | NOT overridden after a successful investigate with only an `rca` payload (no `summary`) | SI-10 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2047-011 | Unit | Handles a `nil` args map without panicking | SI-11 (defensive) | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2047-012 | Unit | `present_decision` is never hard-rejected by this guard, even when overriding content | AU-3 (artifact mandate) | `pkg/apifrontend/agent/phase_guard_test.go` |
| IT-AF-2047-013 | Integration | Grounded state correctly flips to `false` after a second, failed investigate call following an earlier successful one — no stale `grounded=true` leak across investigations in the same session | SI-10, SI-11 | `pkg/apifrontend/agent/phase_guard_test.go` |

### 3.4 Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `enforceGroundingGuard` | `newPhaseGuard`'s `before` (`BeforeToolCallback`) — pre-existing registration, extended | `pkg/apifrontend/agent/phase_guard.go`, registered in `pkg/apifrontend/agent/root.go` | UT-AF-2047-005..012 |
| `investigateHasGroundedContent` / `session.StateKeyGroundedContentAvailable` | `newPhaseGuard`'s `after` (`AfterToolCallback`) — pre-existing registration, extended | `pkg/apifrontend/agent/phase_guard.go`, registered in `pkg/apifrontend/agent/root.go` | IT-AF-2047-013 |
| `session.StateKeyGroundedContentAvailable` constant | Shared session-state key | `pkg/apifrontend/session/consent.go` | Covered transitively |
| Behavioral Constraint 6 prompt text | `BuildInstruction` (embeds `prompt.txt`) | `pkg/apifrontend/agent/prompt.txt` | UT-AF-2047-001..004 |

**No new wiring points**: `newPhaseGuard` was already registered as both a `BeforeToolCallback` and
`AfterToolCallback` on the LLM agent before this fix; this fix extends the existing closures with
new branches — confirmed via CHECKPOINT W (Section 6), no new callback registration required.

---

## 4. Issue #2028 (clone of #2027): Confidence-Gated Severity Correlation (DD-AF-012)

### 4.1 Fix Summary

`bestOverallMatch` (`pkg/apifrontend/severity/triage.go`) marks a cluster-tier-only match
`Ambiguous: true` (no verified relationship to the target — resource/namespace-tier matches are
never ambiguous). `Triage()` returns `*AmbiguousSeverityError` for an unconfirmed ambiguous result
instead of silently trusting it; a matching `ConfirmedSignalName` bypasses the gate (fail-closed for
a different candidate name).

`HandleCreateRR` translates that error into a normal (non-error) `CreateRRResult{Ambiguous: true,
CandidateSignalName, CandidateSeverity}` — a typed signal mirroring #2025's `Managed: false` shape.
All three RR-creating tools gained the round trip: `kubernaut_remediate`
(`RemediateArgs.ConfirmedSignalName` → `RemediateResult.Ambiguous/...`), `kubernaut_investigate`
(`InvestigateMCPArgs.ConfirmedSignalName` → `InvestigateMCPResult.Ambiguous/...`, threaded through
`resolveInvestigationRR`'s three-value return so "ambiguous" is a distinct, non-error branch — a
`main`-specific adaptation over `v1.5`'s two-value return, made explicitly to avoid conflating
"needs clarification" with a genuine Go error), and `kubernaut_investigate_alert`
(`InvestigateAlertResult.Ambiguous/...`, for the severity-only ambiguity case since the RR's
`signalName` is already fixed by the user-supplied `alert_name`).

`EventSeverityTriageAmbiguous` audits every ambiguous (unconfirmed) `Triage()` call.
`prompt.txt` gained Behavioral Constraint 7: ask for confirmation, never guess or retry blindly,
always close out via `kubernaut_present_decision` if no signal is confirmed.

**Test blast-radius note**: `defaultTestTriager()` — used by dozens of pre-existing, unrelated test
cases to obtain *any* successful (non-ambiguous) `Triager` result — was reworked to accept
`(namespace, kind, name string)` and return a resource-scoped alert with a verified relationship to
that specific target, so it continues to resolve confidently under the new ambiguity gate. A
separate `ambiguousTestTriager()` (cluster-scoped-only, no verified relationship) was introduced for
tests that specifically exercise the ambiguous path.

### 4.2 FedRAMP Control Mapping

| Control | Application |
|---|---|
| SI-10 | A correlation with zero verified relationship to the target resource is no longer accepted as validated input to severity/signal attribution. |
| AU-3 / AU-12 | `EventSeverityTriageAmbiguous` records every ambiguous outcome; the RR's `signalName`/`severity` (permanent audit trail) is never misattributed to an unrelated alert. |
| AC-6 / CM-3 | A candidate signal identity is never treated as user-approved fact until explicitly confirmed. |

### 4.3 Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-AF-2027-001 | Unit | Resource-/namespace-tier wins are never ambiguous, even when a cluster-scoped alert also exists | SI-10 | `pkg/apifrontend/severity/triage_test.go` |
| UT-AF-2027-002 | Unit | `Triage()` returns `*AmbiguousSeverityError` with the candidate populated when ambiguous and unconfirmed | SI-10, SI-11 | `pkg/apifrontend/severity/triage_test.go` |
| UT-AF-2027-003 | Unit | A matching `ConfirmedSignalName` bypasses the ambiguity gate; a different candidate does not (fail-closed) | AC-6, CM-3 | `pkg/apifrontend/severity/triage_test.go` |
| UT-AF-2027-008 | Unit | `EventSeverityTriageAmbiguous` emitted exactly once per ambiguous (unconfirmed) `Triage()` call | AU-3, AU-12 | `pkg/apifrontend/severity/triage_test.go` |
| UT-AF-1369-003 | Unit (regression) | Pre-existing test updated: cluster-scoped-only correlation surfaced as `*AmbiguousSeverityError`, not silently trusted | SI-10 | `pkg/apifrontend/severity/triage_test.go` |
| UT-AF-2028-004 | Unit | `HandleCreateRR` translates `*AmbiguousSeverityError` into `CreateRRResult{Ambiguous: true}` with no Go error and no RR created | SI-11 | `pkg/apifrontend/tools/af_create_rr_test.go` |
| UT-AF-2028-004b | Unit | A matching `ConfirmedAmbiguousSignalName` proceeds to RR creation | AC-6, CM-3 | `pkg/apifrontend/tools/af_create_rr_test.go` |
| UT-AF-2028-005 | Unit | `kubernaut_remediate` surfaces `Ambiguous`/`CandidateSignalName`/`CandidateSeverity`, then proceeds once `confirmed_signal_name` matches | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/ka_remediate_test.go` |
| UT-AF-2028-006 | Unit | `kubernaut_investigate` (A2A path) surfaces the same fields, then proceeds to RR creation and MCP session start once confirmed | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/ka_investigate_intent_test.go` |
| UT-AF-2028-007 | Unit | `kubernaut_investigate_alert` surfaces the same fields for the severity-only ambiguity case | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/af_investigate_alert_test.go` |
| UT-AF-2028-010 | Unit | Prompt instructs the model to ask the user before using an ambiguous candidate | AU-3 (model contract) | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-2028-011 | Unit | Prompt forbids guessing or silently filling in the confirmed signal | AC-6, CM-3 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-2028-012 | Unit | Prompt forbids retrying the same ambiguous call without a confirmation | SI-11 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-2028-013 | Unit | Prompt mandates a structured `kubernaut_present_decision` close-out when no signal is confirmed | AU-3 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-2028-014 | Unit | Prompt does not leak internal issue numbers | Hygiene | `pkg/apifrontend/agent/prompt_test.go` |

### 4.4 Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `TriageResult.Ambiguous` / `AmbiguousSeverityError` | `severity.Triager.Triage` (called from `HandleCreateRR`) | `pkg/apifrontend/severity/triage.go` | UT-AF-2027-001..003 |
| `CreateRRResult.Ambiguous` translation | `HandleCreateRR` | `pkg/apifrontend/tools/af_create_rr.go` | UT-AF-2028-004, -004b |
| `RemediateResult.Ambiguous` + `confirmed_signal_name` round trip | `kubernaut_remediate` tool call | `pkg/apifrontend/tools/ka_remediate.go`, wired in `agent/root.go` | UT-AF-2028-005 |
| `InvestigateMCPResult.Ambiguous` + `confirmed_signal_name` round trip | `kubernaut_investigate` tool call | `pkg/apifrontend/tools/ka_investigate_mcp.go`, wired in `agent/root.go` and `handler/mcp_bridge.go` | UT-AF-2028-006 |
| `InvestigateAlertResult.Ambiguous` | `kubernaut_investigate_alert` tool call | `pkg/apifrontend/tools/af_investigate_alert.go`, wired in `agent/root.go` | UT-AF-2028-007 |
| `EventSeverityTriageAmbiguous` audit emission | `Triager.Triage` | `pkg/apifrontend/audit/audit.go` (constant), emission in `triage.go` | UT-AF-2027-008 |
| Behavioral Constraint 7 prompt text | `BuildInstruction` (embeds `prompt.txt`) | `pkg/apifrontend/agent/prompt.txt` | UT-AF-2028-010..014 |

**CHECKPOINT W**: this change modifies existing Args/Result shapes on tools already wired in
production per Section 2.4's #2025 Wiring Manifest — no new construction call sites needed, only new
fields/branches reachable through those same entry points.

---

## 5. Issue #2030 (clone of #2029): AA Reconnect/Takeover Session Desync

Unlike Sections 2-4, this is not a straight port: `v1.5`'s AA session-polling architecture differs
enough that no equivalent `v1.5` fix exists to port. The `main`-only defect was root-caused directly
and split into a foundational fix plus two hardening parts, as agreed with the user.

### 5.1 Foundational Fix: `InvestigationSessionID` vs. Driver-Lease `SessionID`

**Root cause**: KA's `kubernaut_investigate` "start" action returns two distinct session IDs —
`SessionID` (the MCP driver-lease ID, grants exclusive control, **not** pollable via REST) and
`InvestigationSessionID` (the underlying investigation-analysis session, pollable via REST, where
RCA/workflow results are stored). AF's `parseStartInvestigationResult`
(`pkg/apifrontend/ka/mcp_sdk_client.go`) previously discarded `InvestigationSessionID` entirely.
`finalizeInvestigationStart` (`pkg/apifrontend/tools/ka_investigate_mcp.go`) then correlated the
driver-lease `SessionID` into `IS.Status.KACorrelationID` — causing AA's `handleSessionLost` to
404-loop forever once the driver lease itself became unpollable, even while the real
investigation-analysis session was healthy.

**Fix**: `StartInvestigationResult` gained an `InvestigationSessionID` field, populated by
`parseStartInvestigationResult`. `finalizeInvestigationStart` now correlates
`result.InvestigationSessionID`, falling back to `result.SessionID` only when KA omits it
(back-compat with older KA versions).

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-AF-2029-101 | Unit | `UpdateCorrelation` uses `InvestigationSessionID` (pollable analysis session), not the driver-lease `SessionID`, when KA returns both | SI-4, AU-3 | `pkg/apifrontend/tools/ka_investigate_intent_test.go` |
| UT-AF-2029-102 | Unit | `UpdateCorrelation` falls back to `SessionID` when KA omits `InvestigationSessionID` (back-compat) | Backward compat | `pkg/apifrontend/tools/ka_investigate_intent_test.go` |

### 5.2 Part A: Bounded Retry on CRD Schema Rejection

**Root cause**: `reconcileInvestigating`/`reconcileAnalyzing`
(`internal/controller/aianalysis/phase_handlers.go`) previously fail-closed **forever** on the first
`apierrors.IsInvalid` rejection of a `Status().Update()` call (e.g. the live cluster's installed CRD
lagging behind a new enum value the Go source already defines) — `return ctrl.Result{}, nil`, no
requeue, permanently abandoning the `AIAnalysis` with the controller never touching it again.

**Fix**: `handleSchemaRejectedStatusUpdate` retries up to `handlers.MaxSchemaRejectionRetries` (5)
times with backoff (`pkg/shared/backoff`, pre-existing package, reused unchanged), persisting the
attempt count via `handlers.SchemaRejectionRetryCountAnnotation` — written through a plain
`Update()` (not `Status().Update()`) since a CRD with `subresources.status` enabled silently drops
`.status` diffs on a non-status `Update()`, so the annotation write still succeeds even while the
status write is being rejected. Exceeding the cap escalates to a terminal `Failed` phase with valid
enum values (`Reason=APIError`, `SubReason=TransientError`) via a fresh `Get()` + `Status().Update()`,
instead of retrying forever. `clearSchemaRejectionRetryAnnotation` removes the counter once a
`Status().Update()` succeeds again, preventing a stale count from a resolved episode from
prematurely tripping the cap on a later, unrelated rejection episode.

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-AA-2030-001 | Unit | `reconcileInvestigating` retries (not fail-closes) on `apierrors.IsInvalid` | SI-11 | `internal/controller/aianalysis/schema_rejection_retry_test.go` |
| UT-AA-2030-002 | Unit | `reconcileAnalyzing` retries (not fail-closes) on `apierrors.IsInvalid` | SI-11 | `internal/controller/aianalysis/schema_rejection_retry_test.go` |
| UT-AA-2030-003 | Unit | Retry-count annotation persists via a plain `Update()` across repeated rejections | SI-11, AU-3 | `internal/controller/aianalysis/schema_rejection_retry_test.go` |
| UT-AA-2030-005 | Unit | Exceeding the retry cap escalates to a terminal `Failed` phase with valid enum values | SI-11, fail-safe | `internal/controller/aianalysis/schema_rejection_retry_test.go` |
| UT-AA-2030-014 | Unit | A subsequent successful status write clears the leftover retry-count annotation | Regression guard | `internal/controller/aianalysis/schema_rejection_retry_test.go` |
| IT-AA-2030-006 | Integration | Retries with backoff against a **real** apiserver (envtest, not the fake client's approximation), then escalates once the cap is exceeded | SI-11 | `test/integration/aianalysis/schemarejection/retry_test.go` |

### 5.3 Part B: AA-Side Session Adoption

**Root cause**: AF may correlate a newer, different KA session onto an RR's `InvestigationSession`
after a reconnect/takeover (e.g. following a timeout), but AA's `InvestigatingHandler` had no
mechanism to notice — it kept polling the stale session ID it originally tracked, and
`ISEventPredicate` only triggered reconciliation on terminal-phase transitions, not on
`KACorrelationID` changes alone, so the controller could miss a takeover's correlation write
entirely until the next scheduled poll.

**Fix**: `InvestigationSessionChecker.CorrelatedSessionID` (new interface method,
`pkg/aianalysis/handlers/is_checker.go`) returns the KA session ID AF most recently correlated onto
an RR's IS and whether it's still non-terminal. `tryAdoptCorrelatedSession`
(`pkg/aianalysis/handlers/investigating.go`) checks this at two points: the general IS-mismatch check
in `checkISMismatchAndCancel` (steady-state catch-up), and a race-closing re-check immediately before
finalizing a terminal poll result (`checkCorrelatedSessionBeforeFinalizing`, called from both
`handleSessionPollCompleted` and `handleSessionPollFailed`) — closing the race where AF's correlation
write lands between the general mismatch check and a stale session's poll reporting
completed/failed. `adoptCorrelatedSession` swaps the tracked session ID without incrementing
`Generation` (this is catching up to work KA already ran, not starting a fresh investigation),
emits `events.EventReasonSessionAdopted` (SI-4 observability), and records a durable
`RecordAIAgentCall(ctx, analysis, "session_adopted", ...)` audit entry (AU-2/AU-3 traceability).
`ISEventPredicate` was widened to also trigger on `KACorrelationID` changes alone (not just
terminal-phase transitions), so the controller wakes promptly instead of waiting for the next
scheduled poll.

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-AA-2030-008 | Unit | `K8sInvestigationSessionChecker.CorrelatedSessionID` returns the correlated session ID and non-terminal status | SI-4 | `pkg/aianalysis/is_checker_correlated_session_test.go` |
| UT-AA-2030-009 | Unit | General mismatch adoption: `hasIS && Interactive`, correlated ID differs → adopts | SI-4, AU-2/AU-3 | `pkg/aianalysis/investigating_session_adoption_test.go` |
| UT-AA-2030-010 | Unit | Race-closing adoption in `handleSessionPollCompleted` | SI-4, AU-2/AU-3 | `pkg/aianalysis/investigating_session_adoption_test.go` |
| UT-AA-2030-012 | Unit | Race-closing adoption in `handleSessionPollFailed` (symmetric to -010) | SI-4, AU-2/AU-3 | `pkg/aianalysis/investigating_session_adoption_test.go` |
| UT-AA-2030-013 | Unit | `ISEventPredicate` passes Update events when only `KACorrelationID` changes (Phase stays non-terminal) | SI-4 | `internal/controller/aianalysis/predicates_test.go` |
| UT-AA-2030-013b | Unit | `ISEventPredicate` drops Update events when `KACorrelationID` and `Phase` are both unchanged (no regression) | Regression guard | `internal/controller/aianalysis/predicates_test.go` |
| UT-AA-2030-013c | Unit | Existing terminal-phase cases still pass when `KACorrelationID` is held at its zero value (no regression) | Regression guard | `internal/controller/aianalysis/predicates_test.go` |
| IT-AA-2030-011 | Integration | Adopts a newly-correlated KA session instead of finalizing the stale one, waking promptly via the widened predicate — full controller reconcile loop against envtest | SI-4, AU-2/AU-3 | `test/integration/aianalysis/session_correlation_adoption_test.go` |

### 5.4 FedRAMP Control Mapping (#2030)

| Control | Application |
|---|---|
| SI-11 (Error Handling) | Part A: a transient CRD schema lag no longer permanently abandons an in-flight `AIAnalysis`; retries with backoff, escalates cleanly if truly unrecoverable. |
| SI-4 (System Monitoring) | Part B: a session-correlation change is surfaced as an observable event (`EventReasonSessionAdopted`) and the controller wakes promptly (widened predicate) rather than silently drifting until the next scheduled poll. |
| AU-2 / AU-3 (Audit Events / Content) | Part B: every adoption is paired with a durable `RecordAIAgentCall("session_adopted", ...)` record — the fact that a real, possibly-completed investigation was not discarded is traceable after the fact. |

### 5.5 Wiring Manifest (#2030)

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `StartInvestigationResult.InvestigationSessionID` / `parseStartInvestigationResult` | `SDKMCPClient.StartInvestigation` | `pkg/apifrontend/ka/mcp_sdk_client.go` | UT-AF-2029-101, -102 |
| `finalizeInvestigationStart` correlation fix | `HandleInvestigationMCPWithRegistry` | `pkg/apifrontend/tools/ka_investigate_mcp.go` | UT-AF-2029-101, -102 |
| `handleSchemaRejectedStatusUpdate` / `clearSchemaRejectionRetryAnnotation` | `reconcileInvestigating`, `reconcileAnalyzing` | `internal/controller/aianalysis/phase_handlers.go` | UT-AA-2030-001..003, -005, -014; IT-AA-2030-006 |
| `handlers.SchemaRejectionRetryCountAnnotation` / `MaxSchemaRejectionRetries` | Shared constants | `pkg/aianalysis/handlers/constants.go` | Covered transitively |
| `InvestigationSessionChecker.CorrelatedSessionID` | `K8sInvestigationSessionChecker` (production impl) | `pkg/aianalysis/handlers/is_checker.go`, interface in `pkg/aianalysis/handlers/interfaces.go` | UT-AA-2030-008 |
| `tryAdoptCorrelatedSession` / `adoptCorrelatedSession` / `checkCorrelatedSessionBeforeFinalizing` | `checkISMismatchAndCancel`, `handleSessionPollCompleted`, `handleSessionPollFailed` | `pkg/aianalysis/handlers/investigating.go` | UT-AA-2030-009, -010, -012; IT-AA-2030-011 |
| `events.EventReasonSessionAdopted` | Emitted by `adoptCorrelatedSession` | `pkg/shared/events/reasons.go` (constant) | Covered transitively via UT-AA-2030-009/-010/-012 |
| `ISEventPredicate` widening | Controller watch predicate for `InvestigationSession` | `internal/controller/aianalysis/aianalysis_controller.go` | UT-AA-2030-013, -013b, -013c; IT-AA-2030-011 |

---

## 6. CHECKPOINT W Evidence (Whole Bundle)

```bash
$ grep -rn "ScopeChecker" cmd/apifrontend pkg/apifrontend/agent/config.go pkg/apifrontend/agent/root.go \
    pkg/apifrontend/handler/mcp_bridge.go --include="*.go" | grep -v "_test.go"
cmd/apifrontend/backend_deps.go:96:	ScopeChecker scope.ScopeChecker
cmd/apifrontend/backend_deps.go:177:	scopeChecker, err := fleet.NewScopeChecker(scopeMgr, cfg.Fleet, logger.WithName("scope"))
cmd/apifrontend/backend_deps.go:181:	deps.ScopeChecker = scopeChecker
cmd/apifrontend/mcp_a2a_handlers.go:53:		ScopeChecker:          d.Backends.ScopeChecker,   # AgentConfig
cmd/apifrontend/mcp_a2a_handlers.go:167:		ScopeChecker:          d.Backends.ScopeChecker,  # MCPBridgeConfig
pkg/apifrontend/agent/config.go:119:	ScopeChecker scope.ScopeChecker
pkg/apifrontend/agent/root.go:158:	ScopeChecker: cfg.ScopeChecker,          # InvestigateAlertConfig
pkg/apifrontend/agent/root.go:195:	...NewRemediateTool(..., cfg.ScopeChecker)
pkg/apifrontend/agent/root.go:224:	ScopeChecker: cfg.ScopeChecker,          # InvestigateConfig
pkg/apifrontend/handler/mcp_bridge.go:89:	ScopeChecker scope.ScopeChecker
pkg/apifrontend/handler/mcp_bridge.go:251:	ScopeChecker: cfg.ScopeChecker,

$ grep -rn "checkRRScope(" pkg/apifrontend/tools --include="*.go" | grep -v "_test.go"
pkg/apifrontend/tools/af_create_rr.go:171   # single choke point, all 3 tools funnel here
pkg/apifrontend/tools/scope_helpers.go:56   # definition

$ grep -rn "ContextWithScopeChecker\|ScopeCheckerFromContext" --include="*.go" pkg/ cmd/ internal/
# (no results -- removed as dead code during CHECKPOINT W; see Section 2.1)

$ grep -rn "tryAdoptCorrelatedSession\|CorrelatedSessionID\|EventReasonSessionAdopted" \
    pkg/aianalysis/handlers internal/controller/aianalysis pkg/shared/events --include="*.go" | grep -v "_test.go"
pkg/aianalysis/handlers/interfaces.go:132:  CorrelatedSessionID(ctx context.Context, rrName string) (id string, active bool, err error)
pkg/aianalysis/handlers/is_checker.go:122:   func (k *K8sInvestigationSessionChecker) CorrelatedSessionID(...)
pkg/aianalysis/handlers/investigating.go:475: h.tryAdoptCorrelatedSession(ctx, analysis, rrName, "mismatch-check")
pkg/aianalysis/handlers/investigating.go:...: func (h *InvestigatingHandler) tryAdoptCorrelatedSession(...)
pkg/aianalysis/handlers/investigating.go:...: func (h *InvestigatingHandler) adoptCorrelatedSession(...)
pkg/aianalysis/handlers/investigating.go:...: h.checkCorrelatedSessionBeforeFinalizing(ctx, analysis)  # x2 call sites
pkg/shared/events/reasons.go:83:            EventReasonSessionAdopted = "SessionAdopted"

$ grep -rn "handleSchemaRejectedStatusUpdate\|clearSchemaRejectionRetryAnnotation" \
    internal/controller/aianalysis --include="*.go" | grep -v "_test.go"
internal/controller/aianalysis/phase_handlers.go:...: return r.handleSchemaRejectedStatusUpdate(...)  # x2 call sites (Investigating, Analyzing)
internal/controller/aianalysis/phase_handlers.go:...: r.clearSchemaRejectionRetryAnnotation(...)      # x2 call sites
```

No orphaned `pkg/` code: every new symbol has at least one production caller outside `_test.go`
files, confirmed above. The one dead-code finding (`ContextWithScopeChecker`/
`ScopeCheckerFromContext`) was caught and removed during this port's own CHECKPOINT W sweep, not
shipped — see Section 2.1.

## 7. Build Validation

```bash
$ go build ./...                                                                     # exit 0
$ go vet ./...                                                                       # exit 0
$ golangci-lint run --timeout=5m ./pkg/apifrontend/... ./pkg/aianalysis/... \
    ./internal/controller/aianalysis/... ./cmd/apifrontend/...                       # 0 issues
$ go test ./pkg/apifrontend/... ./pkg/aianalysis/... \
    ./internal/controller/aianalysis/... ./cmd/apifrontend/...                       # all packages ok
```

## 8. Coverage Summary

| Metric | Target | Actual |
|---|---|---|
| BR/Control coverage (AC-6, SI-4, SI-10, SI-11, AU-2/AU-3/AU-12, CM-3) | 100% | ✅ (Sections 2.2, 3.2, 4.2, 5.4) |
| Wiring Manifest rows with passing IT/UT evidence | 100% | ✅ (Sections 2.4, 3.4, 4.4, 5.5) |
| CHECKPOINT W (no orphaned `pkg/` code, no orphaned callback registration) | Pass | ✅ (Section 6) |
| Build / vet / lint | Pass | ✅ (Section 7) |
| #2025 fix coverage (AF tool-layer scope validation) | 100% | ✅ (Section 2) |
| #2047 fix coverage (content-grounding guard) | 100% | ✅ (Section 3) |
| #2028 fix coverage (confidence-gated ambiguity surfacing) | 100% | ✅ (Section 4) |
| #2030 fix coverage (foundational session-ID fix + Part A retry + Part B adoption) | 100% | ✅ (Section 5) |

## 9. Out of Scope

- **`v1.5`'s #2023 Stage 1** (opaque-error removal, `ErrCodeNoConversationContext`) and **Stage 3**
  (structured RCA pass-through + shadow-agent alignment verdict): explicitly excluded from this port
  (Section 3.1) — orthogonal/hardening scope beyond this bundle's four issues.
- **`RemediationScope` CRD, dynamic (Rego) scope policies**: deferred to V2.0 per ADR-053, unchanged
  by this port.
- **AF client caching**: accepted trade-off, unchanged by this port.
- **Broader cluster-scoped-alert-as-legitimate-evidence support** (DD-AF-012): tracked as a separate,
  not-yet-scoped issue.
- **Free-text narrative fact-checking**: not addressed (only ported if Stage 3 above is ported later).

## 10. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | AI Assistant | 2026-08-09 | ⏸️ |
| Reviewer | Jordi Gil | | ⏸️ |
| Approver | | | ⏸️ |
