# Test Plan: AF Tool-Layer Scope Validation Before RR/Session Creation

**Issue**: [#2022](https://github.com/jordigilh/kubernaut/issues/2022) (`release/v1.5`)
**Authority**: ADR-053 (Resource Scope Management) — [Addendum: Point 3 — AF Tool-Layer Validation](../../architecture/decisions/ADR-053-resource-scope-management.md#addendum-point-3--af-tool-layer-validation-issue-2022-august-8-2026)
**Same-branch fixes**:
- [#2023](https://github.com/jordigilh/kubernaut/issues/2023) — LLM content fabrication in `present_decision`; opaque-error removal + harness-enforced grounding guard — see Section 8
- `prompt.txt` accuracy fixes (terminal-phase list, tool-naming consistency) — see Section 9
- E2E fixture fix for PR [#2026](https://github.com/jordigilh/kubernaut/pull/2026) CI failure (`kubernaut.ai/managed` labeling gaps) — see Section 10
- [#2027](https://github.com/jordigilh/kubernaut/issues/2027) — `severity.Triager` cluster-scoped alert mis-correlation recurrence; confidence-gated ambiguity surfacing per [DD-AF-012](../../architecture/decisions/DD-AF-012-confidence-gated-severity-correlation.md) — see Section 11
**Branch**: `fix/2022-af-scope-validation` (off `release/v1.5`)
**Created**: 2026-08-08
**Status**: Implementation complete — build/vet/full test suites green

---

## 1. Purpose

`kubernaut_investigate_alert`, `kubernaut_investigate`, and `kubernaut_remediate` (API-frontend
agent tools) each call `HandleCreateRR` to create a `RemediationRequest` (RR) CRD — and, for the
two investigate tools, an `InvestigationSession` — without first checking whether the target
resource is within Kubernaut's management scope (`kubernaut.ai/managed`, ADR-053). The only
existing scope enforcement point downstream of these tools is the RemediationOrchestrator (RO),
which re-validates scope during routing and blocks the RR — but only *after* the CRD (and
interactive session) already exist, and with no signal surfaced back to the calling agent. The
agent is left believing an investigation/remediation started, when in fact RO silently blocked it.

This wastes a tool call, a CRD, and (for the interactive tools) a live session, and produces a
confusing agent-facing experience with no actionable feedback (SI-11 gap).

### Root Cause

All three tools share the same latent defect: no scope gate before `HandleCreateRR`. Point 1
(Gateway, signal-initiated RRs) and Point 2 (RO, temporal-drift re-validation) from ADR-053's
Decision #3 don't cover this path because these RRs are **agent-initiated**, not signal-initiated
— they never pass through Gateway's `ProcessSignal()` pipeline at all.

## 2. Fix Design

Add a third defense-in-depth point (ADR-053 Addendum "Point 3") symmetric to Gateway's Point 1:
validate scope at the AF tool layer, before any RR/session is created, reusing the existing
`scope.ScopeChecker` interface and `IsManaged()` contract unchanged (Decision #4 — no new
validation logic, only a new caller).

```
Tool call arrives (kubernaut_investigate_alert / kubernaut_investigate / kubernaut_remediate)
  → AF validates scope → Unmanaged? → Reject: Managed=false + explanatory message, no CRD created
                        → Managed?   → Proceed to HandleCreateRR (existing behavior)
```

- `pkg/apifrontend/tools/scope_helpers.go` (new): centralizes the shared `checkRRScope()` helper
  and context plumbing (`ContextWithScopeChecker`/`ScopeCheckerFromContext`,
  `ContextWithRESTMapper`/`RESTMapperFromContext`) reused by all three tools.
  - Graceful degradation: a `nil` checker (or absent from context) always returns managed=true —
    preserves backward compatibility for any caller that hasn't wired a `ScopeChecker` yet.
  - Fail-closed: an `IsManaged()` error is treated as unmanaged (mirrors RO's
    `CheckUnmanagedResource`), logged, and surfaced via the same rejection path.
  - On rejection: emits `EventRRScopeRejected` (new audit event, `pkg/apifrontend/audit/audit.go`)
    with namespace/kind/name/user, and returns a message mirroring RO's own block wording
    (`routing/blocking.go`).
- `af_investigate_alert.go`: `ScopeChecker` added as a config field (`InvestigateAlertConfig`) —
  this tool's config is a plain struct literal, not context-injected, matching its existing pattern.
- `ka_remediate.go`, `ka_investigate_mcp.go`: `ScopeChecker` threaded via context injection
  (`ContextWithScopeChecker`) into `HandleRemediate`/`HandleInvestigationMCPWithRegistry`, matching
  their existing `RESTMapper`-via-context pattern.
- All three result types (`InvestigateAlertResult`, `RemediateResult`, `InvestigateMCPResult`) gain
  a `Managed bool` field, explicitly set on every path (`true` on success, `false` on rejection) to
  avoid ambiguous zero-value omission.
- Wiring: `cmd/apifrontend/main.go` constructs a single `scope.NewManager(typedClient)` and threads
  it into both AF transports — `agent.AgentConfig` (A2A/ADK path, used by all three tools) and
  `handler.MCPBridgeConfig` (raw MCP bridge path, used only by `kubernaut_investigate`, which is
  dual-wired).

### Accepted Trade-off: Client Caching

AF's `TypedClient` is an uncached `crclient.WithWatch` — each `IsManaged()` check is a live API
read, unlike Gateway's metadata-only cache (ADR-053 Decision #5). Accepted given AF's low
interactive-tool-call volume relative to Gateway's signal volume; mirrors RO's original V1.0
direct-API approach.

## 3. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **AC-6** | Least Privilege | AF declines to exercise its elevated CRD-creation capability (AF's own ServiceAccount, not the caller's RBAC) for resources outside declared management scope. |
| **SI-10** | Information Input Validation | Target namespace/kind/name validated against declared scope, alongside existing `validate.*` checks already present in these handlers. |
| **AU-3 / AU-12** | Audit Content / Generation | New `EventRRScopeRejected` audit event captures namespace/kind/name/user for every rejected attempt — closes an audit gap where nothing previously distinguished "rejected for scope" from silence at this layer. |
| **SI-11** | Error Handling | Rejection returns a structured `Managed: false` + explanatory message instead of a misleading empty/silent session, mirroring RO's own block wording — the calling agent gets an actionable signal instead of silent waste. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control / BR | Test File |
|---|---|---|---|---|
| UT-AF-2022-040 | Unit | `ContextWithScopeChecker`/`ScopeCheckerFromContext` round-trip a checker through `context.Context`; nil-checker store is a no-op (mirrors `ContextWithRESTMapper`) | Foundational (shared helper) | `pkg/apifrontend/tools/scope_helpers_test.go` |
| UT-AF-2022-001 | Unit | `kubernaut_investigate_alert` rejects an unmanaged target resource without creating an RR, returning `Managed: false` + explanatory message | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/af_investigate_alert_test.go` |
| UT-AF-2022-002 | Unit | `kubernaut_investigate_alert` allows a managed target resource to proceed to RR creation | AC-6, SI-10 | `pkg/apifrontend/tools/af_investigate_alert_test.go` |
| UT-AF-2022-003 | Unit | `kubernaut_investigate_alert` with a `nil` `ScopeChecker` gracefully degrades to always-managed (backward compat for callers not yet wired) | Backward compat | `pkg/apifrontend/tools/af_investigate_alert_test.go` |
| UT-AF-2022-004 | Unit | `kubernaut_investigate_alert` rejection emits `EventRRScopeRejected` with namespace/kind/name/user | AU-3, AU-12 | `pkg/apifrontend/tools/af_investigate_alert_test.go` |
| UT-AF-2022-005 | Unit | `kubernaut_investigate_alert` fails closed (rejects) when the scope checker itself errors, mirroring RO's `CheckUnmanagedResource` | SI-11, fail-closed | `pkg/apifrontend/tools/af_investigate_alert_test.go` |
| UT-AF-2022-010 | Unit | `kubernaut_remediate` rejects an unmanaged target resource without creating an RR | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/ka_remediate_test.go` |
| UT-AF-2022-011 | Unit | `kubernaut_remediate` allows a managed target resource to proceed to RR creation | AC-6, SI-10 | `pkg/apifrontend/tools/ka_remediate_test.go` |
| UT-AF-2022-012 | Unit | `kubernaut_remediate` with no `ScopeChecker` in context gracefully degrades to always-managed | Backward compat | `pkg/apifrontend/tools/ka_remediate_test.go` |
| UT-AF-2022-013 | Unit | `kubernaut_remediate` rejection emits `EventRRScopeRejected` | AU-3, AU-12 | `pkg/apifrontend/tools/ka_remediate_test.go` |
| UT-AF-2022-020 | Unit | `kubernaut_investigate` (A2A path) rejects an unmanaged target resource without creating an RR **or starting an MCP session** | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/ka_investigate_intent_test.go` |
| UT-AF-2022-021 | Unit | `kubernaut_investigate` (A2A path) allows a managed target resource to proceed to RR creation and MCP session start | AC-6, SI-10 | `pkg/apifrontend/tools/ka_investigate_intent_test.go` |
| UT-AF-2022-022 | Unit | `kubernaut_investigate` (A2A path) with no `ScopeChecker` in context gracefully degrades to always-managed | Backward compat | `pkg/apifrontend/tools/ka_investigate_intent_test.go` |
| UT-AF-2022-023 | Unit | `kubernaut_investigate` (A2A path) rejection emits `EventRRScopeRejected` | AU-3, AU-12 | `pkg/apifrontend/tools/ka_investigate_intent_test.go` |
| IT-AF-2022-006 | Integration | `kubernaut_investigate` through the **raw MCP bridge** production dispatch path (`httptest` server → `NewMCPHandler` → tool registration closure) rejects an unmanaged resource: no RR, no MCP session start, `EventRRScopeRejected` audited, message surfaced in the tool response body | AC-6, SI-10, AU-3/AU-12, SI-11 | `pkg/apifrontend/handler/mcp_bridge_integration_test.go` |
| IT-AF-2022-007 | Integration | `kubernaut_investigate` through the raw MCP bridge allows a managed resource to proceed to RR creation and MCP session start | AC-6, SI-10 | `pkg/apifrontend/handler/mcp_bridge_integration_test.go` |

### Tier Coverage Rationale

- **UT** proves the scope-gate logic itself (rejection/pass-through/degradation/fail-closed/audit)
  for each of the three tool handlers, called directly — this is where the branching logic lives.
- **IT** proves wiring for the one dual-wired tool (`kubernaut_investigate`, registered
  independently in both the A2A/ADK agent and the raw MCP bridge, per the Wiring Manifest below).
  The raw MCP bridge path is the one production entry point not already exercised by a UT calling
  the handler function directly — `httptest`-backed IT-AF-2022-006/007 close that gap through the
  real dispatch chain (HTTP → `NewMCPHandler` → tool registration closure → `HandleInvestigationMCPWithRegistry`).
- **A2A/ADK path** (`agent/root.go`) is proven by the UT tier itself: `NewInvestigateMCPTool`/
  `NewRemediateTool`/`NewInvestigateAlertTool` are thin constructors with no additional logic
  between them and the handler functions the UTs already call directly; the wiring risk specific to
  this fix (a `ScopeChecker` silently not reaching the handler) is caught by CHECKPOINT W's grep
  evidence (Section 6), not by a redundant IT.
- **E2E**: not added. This is a pre-CRD-creation validation gate — the existing Gateway/RO E2E
  suites (`docs/testing/BR-SCOPE-001/TEST_PLAN.md`) already prove the end-to-end scope-management
  journey for the CRD-creation and routing-time re-validation points; this fix adds a third,
  earlier gate reusing the same `IsManaged()` contract, not a new journey.

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `checkRRScope` / `ContextWithScopeChecker` / `ScopeCheckerFromContext` | Shared helper called by all three tool handlers | `pkg/apifrontend/tools/scope_helpers.go` | UT-AF-2022-040 |
| `InvestigateAlertConfig.ScopeChecker` → `HandleInvestigateAlert` | `kubernaut_investigate_alert` tool constructor | `pkg/apifrontend/agent/root.go` (`NewInvestigateAlertTool` call, `ScopeChecker: cfg.ScopeChecker`) | UT-AF-2022-001..005 |
| `NewRemediateTool(..., scopeChecker)` → context → `HandleRemediate` | `kubernaut_remediate` tool constructor | `pkg/apifrontend/agent/root.go` (`NewRemediateTool` call, `cfg.ScopeChecker` positional arg) | UT-AF-2022-010..013 |
| `NewInvestigateMCPTool(..., scopeChecker)` → context → `HandleInvestigationMCPWithRegistry` | `kubernaut_investigate` tool constructor (A2A/ADK path) | `pkg/apifrontend/agent/root.go` (`NewInvestigateMCPTool` call, `cfg.ScopeChecker` positional arg) | UT-AF-2022-020..023 |
| `MCPBridgeConfig.ScopeChecker` → `ContextWithScopeChecker` → `HandleInvestigationMCPWithRegistry` | `kubernaut_investigate` tool registration (raw MCP bridge path) | `pkg/apifrontend/handler/mcp_bridge.go` (tool registration closure) | IT-AF-2022-006, IT-AF-2022-007 |
| `scope.NewManager(typedClient)` construction | AF startup | `cmd/apifrontend/main.go` (`deps.scopeChecker = scope.NewManager(...)`, threaded into both `AgentConfig` and `MCPBridgeConfig`) | Covered transitively by all rows above (both consumers of `deps.ScopeChecker()`) |
| `EventRRScopeRejected` | Emitted by `checkRRScope` on rejection | `pkg/apifrontend/audit/audit.go` (event type), `pkg/apifrontend/tools/scope_helpers.go` (emission site) | UT-AF-2022-004, -013, -023; IT-AF-2022-006 |

**Wiring gap found and fixed during CHECKPOINT W**: `agent/root.go`'s `InvestigateAlertConfig{}`
literal (for `kubernaut_investigate_alert`) was initially missing `ScopeChecker: cfg.ScopeChecker`
— the field existed on the config struct and was fully unit-tested via `HandleInvestigateAlert`
called directly, but the production constructor call in `root.go` never passed it through. This is
exactly the "built but not wired" failure mode the Pyramid Invariant guards against: all five
`UT-AF-2022-00x` tests passed throughout, because they call `HandleInvestigateAlert` directly and
never exercise the constructor. Caught by the CHECKPOINT W grep sweep (Section 6), not by a test —
confirming the sweep is required, not optional, even when UT coverage is complete. Fixed by adding
the missing field to the struct literal; re-verified via `go build ./...`, `go vet ./...`, and the
full `pkg/apifrontend/...` / `internal/kubernautagent/...` suites (all green).

## 6. CHECKPOINT W Evidence

```bash
$ grep -rn "ScopeChecker" cmd/apifrontend pkg/apifrontend/agent/config.go pkg/apifrontend/agent/root.go pkg/apifrontend/handler/mcp_bridge.go --include="*.go" | grep -v "_test.go"
cmd/apifrontend/main.go:502:	scopeChecker          scope.ScopeChecker
cmd/apifrontend/main.go:526:func (d *backendDeps) ScopeChecker() scope.ScopeChecker {
cmd/apifrontend/main.go:941:		ScopeChecker:          deps.ScopeChecker(),   # AgentConfig
cmd/apifrontend/main.go:1028:		ScopeChecker:          deps.ScopeChecker(),  # MCPBridgeConfig
pkg/apifrontend/agent/config.go:107:	ScopeChecker scope.ScopeChecker
pkg/apifrontend/agent/root.go:132:	...NewInvestigateMCPTool(..., cfg.ScopeChecker)
pkg/apifrontend/agent/root.go:157:	...NewRemediateTool(..., cfg.ScopeChecker)
pkg/apifrontend/agent/root.go:182:	ScopeChecker:       cfg.ScopeChecker,          # InvestigateAlertConfig (fixed — was missing)
pkg/apifrontend/handler/mcp_bridge.go:84:	ScopeChecker scope.ScopeChecker
pkg/apifrontend/handler/mcp_bridge.go:215:	ctx = tools.ContextWithScopeChecker(ctx, cfg.ScopeChecker)

$ grep -rn "checkRRScope(" pkg/apifrontend/tools --include="*.go" | grep -v "_test.go"
pkg/apifrontend/tools/ka_remediate.go:85
pkg/apifrontend/tools/af_investigate_alert.go:168
pkg/apifrontend/tools/ka_investigate_mcp.go:256
pkg/apifrontend/tools/scope_helpers.go:72   # definition
```

No orphaned `pkg/` code: every new symbol (`checkRRScope`, `ContextWithScopeChecker`,
`ScopeCheckerFromContext`, `EventRRScopeRejected`) has at least one production caller outside
`_test.go` files, confirmed above.

## 7. Build Validation

```bash
$ go build ./...        # exit 0
$ go vet ./...           # exit 0
$ go test ./pkg/apifrontend/... ./internal/kubernautagent/...   # all packages ok
```

## 8. Issue #2023 (QE-reported, same branch): Content-Grounding Fix

#2023's core ask is a deterministic mechanism preventing `kubernaut_present_decision` from
rendering a fabricated RCA/audit-trail narrative when no real investigation content exists. This
was addressed in two stages within this branch:

### 8.1 Stage 1 — Opaque Error Removal (Partial Mitigation)

**Symptom**: When `discover_workflows` finds no conversation content to extract an RCA from
(stored RCA absent, live conversation empty, audit-trail reconstruction empty — the case
`len(messages) == 0` at `internal/kubernautagent/mcp/tools/investigate.go:927`), it previously
returned a plain `fmt.Errorf(...)`. `ErrorBoundary` (`internal/kubernautagent/mcp/tools/errors.go`)
redacts unrecognized error types to the generic `ErrCodeInternalError` before they reach the
client — indistinguishable from an actual server bug. This was *one* condition that left the
calling LLM with an ambiguous signal instead of a clear "no data" fact — part of the room the LLM
used to fabricate a narrative in the reported repro. On its own this was necessary-but-not-sufficient:
it removes one ambiguous-error trigger, but a fabrication is also possible from a *successful*
sparse response, not only from an error path — which Stage 2 below closes.

**Fix**: Added `ErrCodeNoConversationContext` (`internal/kubernautagent/mcp/tools/errors.go`),
following the existing `MCPError` pattern (precedent: `ErrCodeToolBudgetExhausted`). Replaced the
`fmt.Errorf` at `investigate.go:927` with `ErrCodeNoConversationContext.WithDetail("rr_id", ...)`.
`ErrorBoundary`'s `errors.As(err, &mcpErr)` check passes typed `*MCPError` values through
unmodified, so the client now receives a distinguishable `no_conversation_context` code instead of
`internal_error`.

| ID | Tier | Business-Level Behavior Description | Control / BR | Test File |
|---|---|---|---|---|
| UT-KA-DW-016 | Unit | `discover_workflows` returns `ErrCodeNoConversationContext` (not an opaque `fmt.Errorf`) when no stored RCA, live conversation, or audit-trail reconstruction produced any content | SI-11 | `internal/kubernautagent/mcp/tools/investigate_test.go` |
| IT-KA-2023-001 | Integration | `ErrCodeNoConversationContext`, once returned by `Handle()`, survives `registration.go`'s production dispatch wrap (`tool.Handle(...)` → `ErrorBoundary(...)`, the exact unwrapped call at `registration.go:47-49`) as `no_conversation_context`, not redacted to `internal_error` | SI-11 | `internal/kubernautagent/mcp/tools/errors_test.go` |

**No new wiring points**: this fix reuses the existing `ErrorBoundary`/`MCPError` mechanism
end-to-end; only a new error value and its one call site changed. `registration.go`'s dispatch
(`tool.Handle` → `ErrorBoundary`, unwrapped) has no intermediate layer to independently wire.

### 8.2 Stage 2 — Harness-Enforced Content-Grounding Guard (Closes #2023's Core Ask)

**Design conflict resolved**: an initial design considered *blocking* `kubernaut_present_decision`
outright when no content is available, but `prompt.txt`'s existing "Edge Scenarios — Always Call
kubernaut_present_decision" contract (#1408, AU-3) mandates the model call it in every scenario,
including tool failures, so a structured `investigation_summary` artifact is always emitted for
audit traceability. Blocking the tool would violate that mandate. The resolution: never block the
call — **override its content in place** when it would otherwise be fabricated, so the tool always
executes and the AU-3 artifact is always emitted, but the artifact's content is forced truthful.

**Chosen mechanism**: a harness-side `BeforeToolCallback` guard (`enforceGroundingGuard`,
`pkg/apifrontend/agent/phase_guard.go`), extending the existing, already-wired `newPhaseGuard`
(#1307/#1899/#1918) rather than introducing a new callback registration:

1. `newPhaseGuard`'s `after` callback now also records, on every `kubernaut_investigate` call
   (success **or** failure), whether the response carried real, groundable RCA content
   (`investigateHasGroundedContent`) into `session.StateKeyGroundedContentAvailable`. A `status`
   of `unmanaged` (#2022) or `session_active` (#1922, a different-user state with its own fallback
   card) is never treated as grounded; an empty `summary` with no `rca` payload is never treated as
   grounded.
2. `newPhaseGuard`'s `before` callback now intercepts `kubernaut_present_decision` specifically:
   if the state key is `false` — **including the fail-safe default when it was never set at all**,
   mirroring #2022's own safe-default posture — it overwrites `args["summary"]`, deletes
   `args["rca"]`, and forces `args["options"] = []` in place with a fixed, honest
   `noGroundedContentSummary` payload, then returns `(nil, nil)` so the (now-truthful) call proceeds
   normally. A genuinely grounded call passes through completely untouched.
3. `prompt.txt` gained a complementary model-side instruction (Behavioral Constraints item 6, and
   Edge Scenarios item 4) telling the model itself never to fabricate ungrounded findings — the
   harness guard is a backstop the model must not rely on, not a substitute for the model's own
   discipline.

| ID | Tier | Business-Level Behavior Description | Control / BR | Test File |
|---|---|---|---|---|
| UT-AF-2023-001 | Unit | `present_decision` content is overridden to the honest "no data" payload when `kubernaut_investigate` was never called (fail-closed default) | SI-10, SI-11 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2023-002 | Unit | `present_decision` content is overridden when the prior `kubernaut_investigate` was rejected for scope (`status: unmanaged`, #2022) | SI-10, SI-11 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2023-003 | Unit | `present_decision` content is overridden when the prior `kubernaut_investigate` returned a generic tool error; `rca` is deleted, not left carrying invented fields | SI-10, SI-11 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2023-004 | Unit | `present_decision` content is overridden when the prior `kubernaut_investigate` returned `status: session_active` | SI-10, SI-11 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2023-005 | Unit | `present_decision` content passes through **untouched** after a successful `kubernaut_investigate` with a real `summary` | SI-10 (negative case) | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2023-006 | Unit | `present_decision` content passes through untouched when `kubernaut_investigate` succeeded with only an `rca` payload (no `summary` string) | SI-10 (negative case) | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2023-007 | Unit | `enforceGroundingGuard` handles a `nil` args map without panicking | SI-11 (defensive) | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2023-008 | Unit | The guard mutates args in place and never hard-rejects `present_decision` — `before` always returns `(nil, nil)` for this tool | AU-3 (artifact mandate preserved) | `pkg/apifrontend/agent/phase_guard_test.go` |
| IT-AF-2023-009 | Integration | Grounded state correctly flips `false` after a second, failed `kubernaut_investigate` call following an earlier successful one — no stale `grounded=true` leak across investigations in the same session | SI-10, SI-11 | `pkg/apifrontend/agent/phase_guard_test.go` |

### Wiring Manifest (#2023 Stage 2)

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `enforceGroundingGuard` | `newPhaseGuard`'s `before` (`llmagent.BeforeToolCallback`) | `pkg/apifrontend/agent/phase_guard.go`; registered in `pkg/apifrontend/agent/root.go:74-75` (`beforeCallbacks = append(beforeCallbacks, beforePhase)`) — pre-existing wiring, extended, not newly added | UT-AF-2023-001..008 |
| `investigateHasGroundedContent` / `session.StateKeyGroundedContentAvailable` | `newPhaseGuard`'s `after` (`llmagent.AfterToolCallback`) | `pkg/apifrontend/agent/phase_guard.go`; registered in `pkg/apifrontend/agent/root.go:89` (`AfterToolCallbacks: [...afterPhase...]`) — pre-existing wiring, extended | IT-AF-2023-009 |
| `session.StateKeyGroundedContentAvailable` constant | Shared session-state key | `pkg/apifrontend/session/consent.go` | Covered transitively by all rows above |
| `kubernaut_present_decision` prompt grounding instructions | Model-side behavioral contract | `pkg/apifrontend/agent/prompt.txt` (Behavioral Constraints item 6; Edge Scenarios item 4) | `pkg/apifrontend/agent/prompt_test.go` (existing `ContainSubstring` assertions cover retention; no new prompt-content UT added since the harness guard is the enforceable, testable half of this fix) |

**No new wiring points**: `newPhaseGuard` was already registered as both a `BeforeToolCallback`
and `AfterToolCallback` on the LLM agent (`root.go:74-75, 89`) before this fix, covering #1307,
#1899, and #1918. This fix extends the existing `before`/`after` closures with new branches for
`kubernaut_present_decision`/`kubernaut_investigate` respectively — CHECKPOINT W confirmed via the
same production registration already in place; no new callback registration was required. `kubernaut_present_decision` itself was already registered as a tool (`root.go:136`) prior to this
fix.

### 8.3 Remaining Judgment Call (Explicitly Flagged, Not Resolved Here)

Per earlier discussion: whether the harness should ever *nudge* the model to keep investigating
when there is genuinely nothing to present (vs. simply reporting "no data" and stopping) is a
product/UX judgment call, not resolved by this fix. This fix only guarantees the *content* of
whatever gets presented is truthful — it does not change whether/when `present_decision` is called.

## 9. Prompt Accuracy Fixes (`prompt.txt`, same branch)

A line-by-line cross-check of `prompt.txt` against the actual implementation surfaced two
inaccuracies, unrelated to #2022/#2023's core fixes but corrected in the same change since both
touch `prompt.txt`:

1. **Incomplete terminal-phase list**: Phase 4's watch-loop description listed only "Completed,
   Failed, Cancelled" as terminal states, but `tools.IsTerminalPhase`
   (`pkg/apifrontend/tools/helpers.go:104-110`) also treats `TimedOut` and `Skipped` as terminal.
   Fixed to list all five.
2. **Inconsistent tool naming**: several sections referred to the bare `present_decision` while the
   tool is registered as `kubernaut_present_decision` (`ka_tools.go:387`). Fixed all bare
   references for consistency; `prompt_test.go`'s `UT-AF-131-003` substring assertion updated to
   match (`MUST call kubernaut_present_decision`).

No new test IDs: these are prompt-text corrections covered by existing `prompt_test.go`
`ContainSubstring` assertions (plain-substring matches on `"present_decision"` continue to match
the now-prefixed name unchanged).

## 10. E2E Fixture Fix (PR #2026 CI Failure)

Merging #2022's scope-check fix caused PR #2026's `E2E (apifrontend)` CI job to fail
deterministically (9 specs) because several E2E fixture namespaces created before this check
existed were never labeled `kubernaut.ai/managed=true`:

- `test/e2e/apifrontend/severity_triage_test.go`'s `BeforeEach` creates `sev-tier1-ns`,
  `sev-tier15-ns`, `sev-tier2-ns`, `no-data-ns`, `no-rules-ns`, `sev-userhint-ns` unlabeled.
- `af-investigate-e2e` (targeted by the mock-LLM's `af_investigate`/`af_progressive_investigate`
  scenarios, `deploy/apifrontend/overlays/e2e/mock-llm.yaml`) was never created as a real
  `Namespace` object at all — only referenced as a string in RR specs and Prometheus alert labels.

**Fix**: added `infrastructure.EnsureManagedNamespace(ctx, client, name)`
(`test/infrastructure/apifrontend_scope_e2e.go`) — idempotent create-or-relabel, safe under Ginkgo
parallel-process races. `severity_triage_test.go`'s `BeforeEach` now calls it per-namespace instead
of duplicating raw `client.Create`/`IsAlreadyExists` handling; `e2e_suite_test.go`'s
`SynchronizedBeforeSuite` calls it once for `af-investigate-e2e` right after the controller-runtime
client is built.

This is a test-fixture correction, not a change to Point 3's validation logic — a real Kubernaut
deployment would already have these namespaces labeled, matching every other managed namespace.

## 11. Issue #2027 (QE-reported, same branch): Confidence-Gated Severity Correlation (DD-AF-012)

QE reported a recurrence of #2018's symptom: `severity.Triager` bound an RR's `severity`/`signalName`
to an unrelated, persistently-firing cluster-scoped alert instead of the target resource's own
signal. Root cause, decision rationale, and FedRAMP mapping are documented in full in
[DD-AF-012](../../architecture/decisions/DD-AF-012-confidence-gated-severity-correlation.md); this
section covers only the test-plan/coverage view.

**Fix summary**: `bestOverallMatch` now marks a cluster-tier-only match `Ambiguous: true` (no
verified relationship to the target resource — resource/namespace-tier matches are never
ambiguous). `Triage()` returns a new `*AmbiguousSeverityError` for an unconfirmed ambiguous result
instead of silently trusting it. `HandleCreateRR` translates that error into a normal (non-error)
`CreateRRResult{Ambiguous: true, CandidateSignalName, CandidateSeverity}` — a typed signal, not a Go
error — mirroring #2022's `Managed: false` shape. All three RR-creating tools
(`kubernaut_remediate`, `kubernaut_investigate`, `kubernaut_investigate_alert`) gained a
`ConfirmedSignalName` input field and `Ambiguous`/`CandidateSignalName`/`CandidateSeverity` result
fields, so the calling agent can ask the user to confirm the weak candidate and re-call with
`confirmed_signal_name` set to bypass the gate for that exact candidate (fail-closed: a *different*
candidate name does not bypass it). `prompt.txt` gained Behavioral Constraint 7, spike-validated
against a live model, instructing the agent to ask for confirmation, never guess or retry blindly,
and always close out via `kubernaut_present_decision` if no signal is confirmed.

| ID | Tier | Business-Level Behavior Description | Control / BR | Test File |
|---|---|---|---|---|
| UT-AF-2027-001 | Unit | `bestOverallMatch` sets `Ambiguous: true` only when the winning candidate is cluster-scoped; resource/namespace-tier wins stay `Ambiguous: false` even when a cluster-scoped alert also exists | SI-10 | `pkg/apifrontend/severity/triage_test.go` |
| UT-AF-2027-002 | Unit | `Triage()` returns `*AmbiguousSeverityError` with the candidate populated when `Ambiguous: true` and no matching `ConfirmedSignalName` | SI-10, SI-11 | `pkg/apifrontend/severity/triage_test.go` |
| UT-AF-2027-003 | Unit | `Triage()` bypasses the ambiguity gate and returns normally when `ConfirmedSignalName` exactly matches the candidate's `AlertName`; a *different* candidate name does not bypass (fail-closed) | AC-6, CM-3 | `pkg/apifrontend/severity/triage_test.go` |
| UT-AF-2027-004 | Unit | `HandleCreateRR` translates `*AmbiguousSeverityError` into `CreateRRResult{Ambiguous: true, ...}` with no Go error and no RR created | SI-11 | `pkg/apifrontend/tools/af_create_rr_test.go` |
| UT-AF-2027-004b | Unit | A matching `ConfirmedAmbiguousSignalName` proceeds to RR creation | AC-6, CM-3 | `pkg/apifrontend/tools/af_create_rr_test.go` |
| UT-AF-2027-005 | Unit | `kubernaut_remediate` surfaces `Ambiguous`/`CandidateSignalName`/`CandidateSeverity`, then proceeds to RR creation once `confirmed_signal_name` matches | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/ka_remediate_test.go` |
| UT-AF-2027-006 | Unit | `kubernaut_investigate` (A2A path) surfaces the same fields, then proceeds to RR creation and MCP session start once confirmed | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/ka_investigate_intent_test.go` |
| UT-AF-2027-007 | Unit | `kubernaut_investigate_alert` surfaces the same fields for the severity-only ambiguity case (the RR's `signalName` is already fixed by the user-supplied `alert_name`) | AC-6, SI-10, SI-11 | `pkg/apifrontend/tools/af_investigate_alert_test.go` |
| UT-AF-2027-008 | Unit | `EventSeverityTriageAmbiguous` is emitted exactly once per ambiguous (unconfirmed) `Triage()` call | AU-3, AU-12 | `pkg/apifrontend/severity/triage_test.go` |
| IT-AF-2027-009 | Integration | Full round trip through `mcp_bridge_integration_test.go`'s real MCP dispatch path (`httptest` server → `NewMCPHandler` → tool registration closure): first call with an ambiguous-only correlation returns `ambiguous: true` and starts no MCP session; the re-call with `confirmed_signal_name` proceeds to RR creation and MCP session start | AC-6, SI-10, AU-3/AU-12, SI-11 | `pkg/apifrontend/handler/mcp_bridge_integration_test.go` |
| UT-AF-2027-010..014 | Unit | `prompt.txt` contains the spike-validated ambiguous-signal instruction (asks for confirmation, forbids guessing, forbids blind retry, mandates a `kubernaut_present_decision` close-out) and leaks no internal issue numbers | AU-3 (model-side contract) | `pkg/apifrontend/agent/prompt_test.go` |

### Wiring Manifest (#2027)

This change modifies existing Args/Result shapes on tools already wired in production (per Section
5's #2022 Wiring Manifest) — it introduces no new wiring points, only new fields/branches reachable
through those same entry points.

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `TriageResult.Ambiguous` / `AmbiguousSeverityError` | `severity.Triager.Triage` (called from `HandleCreateRR`) | `pkg/apifrontend/severity/triage.go` | UT-AF-2027-001..003 |
| `CreateRRResult.Ambiguous` translation | `HandleCreateRR` | `pkg/apifrontend/tools/af_create_rr.go` | UT-AF-2027-004, -004b |
| `RemediateResult.Ambiguous` + `confirmed_signal_name` round trip | `kubernaut_remediate` tool call | `pkg/apifrontend/tools/ka_remediate.go`, wired in `agent/root.go` | UT-AF-2027-005 |
| `InvestigateMCPResult.Ambiguous` + `confirmed_signal_name` round trip | `kubernaut_investigate` tool call | `pkg/apifrontend/tools/ka_investigate_mcp.go`, wired in `agent/root.go` and `handler/mcp_bridge.go` | UT-AF-2027-006, IT-AF-2027-009 |
| `InvestigateAlertResult.Ambiguous` | `kubernaut_investigate_alert` tool call | `pkg/apifrontend/tools/af_investigate_alert.go`, wired in `agent/root.go` | UT-AF-2027-007 |
| `EventSeverityTriageAmbiguous` audit emission | `Triager.Triage` | `pkg/apifrontend/audit/audit.go` (constant) + emission in `triage.go` | UT-AF-2027-008 |
| Behavioral Constraint 7 prompt text | `BuildInstruction` (embeds `prompt.txt`) | `pkg/apifrontend/agent/prompt.txt` | UT-AF-2027-010..014 |

**CHECKPOINT W**: verified via `grep -rn "NewInvestigateMCPTool\|NewRemediateTool\|NewInvestigateAlertTool" pkg/apifrontend/agent/root.go` (all three tools still constructed there, unchanged call sites — no new construction needed since only the Args/Result shapes changed) and by the full test suite passing end-to-end through each production entry point, including the raw MCP bridge dispatch path (`IT-AF-2027-009`).

**Test blast-radius note**: `defaultTestTriager()` — a shared fixture used by ~46 pre-existing,
unrelated test cases across 7 files (`af_create_rr_test.go`, `ka_remediate_test.go`,
`ka_investigate_intent_test.go`, `af_investigate_alert_test.go`,
`af_investigate_alert_1440_test.go`, `af_investigate_alert_1440_it_test.go`,
`cluster_scope_1477_test.go`) to obtain *any* successful (non-ambiguous) `Triager` result — was
reworked to accept `(namespace, kind, name string)` and return a resource-scoped alert with a
verified relationship to that specific target, so it continues to resolve confidently under the new
ambiguity gate. A separate `ambiguousTestTriager()` (cluster-scoped-only, no verified relationship)
was introduced for tests that specifically exercise the ambiguous path. All call sites were updated
to pass the target's own `namespace`/`kind`/`name`; the full `pkg/apifrontend/tools/...` suite (602
specs) and `pkg/apifrontend/handler/...` suite (223 specs) pass with zero failures.

## 12. Coverage Summary

| Metric | Target | Actual |
|---|---|---|
| BR/Control coverage (AC-6, SI-10, AU-3/AU-12, SI-11) | 100% | ✅ (Sections 3, 8.2, 11) |
| Wiring Manifest rows with passing IT/UT evidence | 100% | ✅ (Sections 5, 8.2, 11) |
| CHECKPOINT W (no orphaned `pkg/` code, no orphaned callback registration) | Pass | ✅ (Section 6, 8.2, 11) |
| Build (`go build ./...`, `go vet ./...`) | Pass | ✅ (Section 7, 11) |
| #2023 fix coverage (opaque-error removal + harness grounding guard) | 100% | ✅ (Section 8) |
| E2E fixture fix (PR #2026 CI) | Fixed | ✅ (Section 10) |
| #2027 fix coverage (confidence-gated ambiguity surfacing, DD-AF-012) | 100% | ✅ (Section 11) |

## 13. Out of Scope

- **`RemediationScope` CRD, dynamic (Rego) scope policies**: deferred to V2.0 per ADR-053; this fix
  only adds a new *caller* of the existing static-label `IsManaged()` contract.
- **AF client caching**: accepted trade-off (Section 2) — not addressed by this fix.
- **Gateway (Point 1) / RO (Point 2) changes**: unaffected; this fix adds Point 3 only.
- **Product/UX judgment call on when to nudge continued investigation** (Section 8.3): explicitly
  flagged, not resolved by this fix.
- **Broader cluster-scoped-alert-as-legitimate-evidence support** (DD-AF-012): tracked as a separate,
  not-yet-scoped issue; #2027's fix is intentionally durable against that future work.

## 14. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | AI Assistant | 2026-08-08 | ⏸️ |
| Reviewer | Jordi Gil | | ⏸️ |
| Approver | | | ⏸️ |
