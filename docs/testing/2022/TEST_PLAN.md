# Test Plan: AF Tool-Layer Scope Validation Before RR/Session Creation

**Issue**: [#2022](https://github.com/jordigilh/kubernaut/issues/2022) (`release/v1.5`)
**Authority**: ADR-053 (Resource Scope Management) — [Addendum: Point 3 — AF Tool-Layer Validation](../../architecture/decisions/ADR-053-resource-scope-management.md#addendum-point-3--af-tool-layer-validation-issue-2022-august-8-2026)
**Secondary fix (same branch)**: [#2023](https://github.com/jordigilh/kubernaut/issues/2023) internal-error opacity for `discover_workflows` — see Section 8
**Branch**: `fix/2022-af-scope-validation` (off `release/v1.5`)
**Created**: 2026-08-08
**Status**: Implementation complete — verification in progress

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

## 8. Partial Mitigation for Issue #2023 (QE-reported, same branch)

**This section does NOT close #2023.** #2023's core ask — a deterministic grounding check (or
prompt-level guardrail) preventing `kubernaut_present_decision` from rendering a fabricated
RCA/audit-trail narrative when no real investigation content exists — is a separate, larger design
task, explicitly flagged in the issue as needing product/design input before implementation. That
design has **not yet been approved or implemented** and is tracked separately (a design plan is
pending user approval as of this writing). This section documents only the one concrete
contributing factor identified and fixed during #2022 triage.

**Symptom**: When `discover_workflows` finds no conversation content to extract an RCA from
(stored RCA absent, live conversation empty, audit-trail reconstruction empty — the case
`len(messages) == 0` at `internal/kubernautagent/mcp/tools/investigate.go:927`), it previously
returned a plain `fmt.Errorf(...)`. `ErrorBoundary` (`internal/kubernautagent/mcp/tools/errors.go`)
redacts unrecognized error types to the generic `ErrCodeInternalError` before they reach the
client — indistinguishable from an actual server bug. #2023 identifies this as *one* condition
that leaves the calling LLM with an ambiguous signal instead of a clear "no data" fact, which is
part of the room the LLM used to fabricate a narrative in the reported repro. Fixing this
opaqueness is necessary-but-not-sufficient: it removes one ambiguous-error trigger, but does not
add the grounding check #2023 actually asks for (a fabrication is possible from a *successful*
sparse response too, not only from an error path).

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

### Wiring Manifest (#2023)

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `ErrCodeNoConversationContext` | `discover_workflows` action handler | `internal/kubernautagent/mcp/tools/investigate.go:927` | UT-KA-DW-016 |
| `ErrorBoundary` pass-through for the new code | `kubernaut_investigate` MCP tool dispatch | `internal/kubernautagent/mcp/tools/registration.go:47-49` | IT-KA-2023-001 |

**No new wiring points**: this fix reuses the existing `ErrorBoundary`/`MCPError` mechanism
end-to-end; only a new error value and its one call site changed. `registration.go`'s dispatch
(`tool.Handle` → `ErrorBoundary`, unwrapped) has no intermediate layer to independently wire.

### Remaining Design Work for #2023 (Not Yet Implemented)

- A deterministic grounding check before `kubernaut_present_decision` fires, rejecting/short-circuiting
  when there is no real summary/RCA content in the session to attribute a decision card to.
- And/or a prompt-level guardrail instructing the model that RCA narration must only include facts
  traceable to actual tool response fields.
- Per the issue, this needs product/design discussion on where "helpful synthesis" ends and
  "invention of content no tool ever returned" begins — a short design plan will be presented for
  approval separately before any implementation.

## 9. Coverage Summary

| Metric | Target | Actual |
|---|---|---|
| BR/Control coverage (AC-6, SI-10, AU-3/AU-12, SI-11) | 100% | ✅ (Section 4) |
| Wiring Manifest rows with passing IT/UT evidence | 100% | ✅ (Section 5) |
| CHECKPOINT W (no orphaned `pkg/` code) | Pass | ✅ (Section 6) |
| Build (`go build ./...`, `go vet ./...`) | Pass | ✅ (Section 7) |
| #2023 secondary fix coverage | 100% | ✅ (Section 8) |

## 10. Out of Scope

- **`RemediationScope` CRD, dynamic (Rego) scope policies**: deferred to V2.0 per ADR-053; this fix
  only adds a new *caller* of the existing static-label `IsManaged()` contract.
- **AF client caching**: accepted trade-off (Section 2) — not addressed by this fix.
- **Gateway (Point 1) / RO (Point 2) changes**: unaffected; this fix adds Point 3 only.

## 11. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | AI Assistant | 2026-08-08 | ⏸️ |
| Reviewer | Jordi Gil | | ⏸️ |
| Approver | | | ⏸️ |
