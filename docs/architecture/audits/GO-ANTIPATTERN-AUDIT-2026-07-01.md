# Go Anti-Pattern & Complexity Audit — GA Readiness

**Tracking issue**: [#1520](https://github.com/jordigilh/kubernaut/issues/1520)
**Date**: 2026-07-01
**Branch**: `feat/1505-gap-01-cosign-ci-signing` (base: `feature/multi-cluster-federation`)
**Commit**: `d0bc884f7`
**Scope**: `cmd/`, `pkg/`, `internal/` — 936 non-test, non-generated `.go` files (~160.5k LOC)
**Excluded**: `*_test.go`, `zz_generated.*`, `pkg/datastorage/ogen-client/`, `pkg/agentclient/`, `oas_*_gen.go` (all machine-generated)
**Authority**: `AGENTS.md` § [Go Anti-Pattern Checklist](../../../AGENTS.md#go-anti-pattern-checklist), [100 Go Mistakes](https://100go.co/)

## Methodology

Automated static analysis, no manual sampling — every number below is reproducible:

| Tool | Checks | Command |
|---|---|---|
| `gocyclo` | Cyclomatic complexity | `gocyclo -over 15 -avg ./cmd ./pkg ./internal` |
| `golangci-lint` (`funlen`, `gocognit`, `nestif`, `maintidx`) | Function length, cognitive complexity, nesting depth, maintainability index | custom audit config, `max-issues-per-linter: 0` |
| `golangci-lint` (`revive`) | 8+ param functions, error-string format, naked returns, unused params | `argument-limit: 7`, `error-strings`, `bare-return` |
| `golangci-lint` (`govet` shadow) | Variable shadowing | `govet.enable: [shadow]` |
| `golangci-lint` (`prealloc`) | Missing slice/map capacity hints | default settings |
| `golangci-lint` (`errcheck`) | Ignored errors | already enabled in repo `.golangci.yml` |
| Python AST-free regex scan | Struct field counts, interface method counts, `context.Context` struct fields, raw param counts (cross-check on revive) | custom script (counts top-level commas/fields, ignores comments) |

None of the audit-only linters (`funlen`, `gocognit`, `nestif`, `maintidx`, `cyclop`, `gochecknoglobals`, shadow-`govet`) are enabled in the committed `.golangci.yml` today — this audit does not change CI behavior, it is a snapshot for planning purposes.

---

## Executive Summary

| Category | Count | AGENTS.md rule | Status |
|---|---|---|---|
| Functions with 8+ parameters | **32** (2 have 12) | 8+ params → Options pattern | 🔴 Needs fixing |
| God structs (15+ fields) | **30** | 15+ fields → decompose | 🟡 Mixed (some are legit DTOs) |
| Interface pollution (5+ methods) | **16** | 5+ methods → split into role interfaces | 🟡 Needs review |
| `context.Context` stored in struct | **2** | Pass as first param | 🟡 Both have documented rationale |
| Functions over cyclomatic complexity 15 | **149** (25 over 20, worst = 88) | N/A (project convention) | 🔴 Needs fixing |
| Functions over cognitive complexity 20 | **161** (worst = 133) | N/A (project convention) | 🔴 Needs fixing |
| Functions over 80 lines / 60 statements | **54** | Visual inspection / nesting | 🔴 Needs fixing |
| Deeply nested (`nestif`, complexity ≥5) | **111** | Unnecessary nesting (>3 levels) | 🔴 Needs fixing |
| Low maintainability index (<20) | included above via funlen/gocognit overlap | N/A | — |
| Missing slice/map pre-allocation | **13** | Inefficient pre-allocation | 🟢 Small, easy fix |
| Variable shadowing | **120** (114 are `err`, low-risk) | Shadowing → rename | 🟢 Mostly benign, low priority |
| Ignored errors (`errcheck`) | **0** | Never ignore errors | ✅ Clean |
| Error string format (`revive error-strings`) | **0** | lowercase, no punctuation | ✅ Clean |
| Naked returns (`revive bare-return`) | **0** | Explicit returns | ✅ Clean |
| `context.Context` as non-first param | **0** | ctx first | ✅ Clean |
| Files over 700 lines | **29** (worst = 3,414) | Deep nesting/God file (implied) | 🔴 Needs fixing |

**Bottom line**: error handling, error-string formatting, naked returns, and context-parameter ordering are already clean project-wide (previous REFACTOR passes did their job there). The **outstanding gaps are concentrated in complexity/length and parameter-count anti-patterns**, and they cluster heavily in **4 services**: KubernautAgent, DataStorage, APIFrontend, and RemediationOrchestrator.

---

## Findings by Service (ranked by total anti-pattern findings)

Counts = `funlen` + `gocognit` + `gocyclo` + `nestif` + `revive argument-limit` findings, deduplicated by file:line.

| Rank | Service | Findings | Worst offender |
|---|---|---|---|
| 1 | **KubernautAgent** (`internal/kubernautagent` + `pkg/kubernautagent` + `cmd/kubernautagent`) | 83 | `cmd/kubernautagent/main.go: main()` — cognitive complexity 108, 1,695-line file |
| 2 | **APIFrontend** (`pkg/apifrontend` + `cmd/apifrontend` + `internal/controller/apifrontend`) | 59 | `pkg/apifrontend/tools/crd_tools.go: HandleWatch()` — cognitive complexity 133 (highest in repo) |
| 3 | **DataStorage** (`pkg/datastorage` + `cmd/datastorage`) | 59 | `pkg/datastorage/server/workflow_handlers.go` — 1,714-line file |
| 4 | **RemediationOrchestrator** (`internal/controller/remediationorchestrator` + `pkg/remediationorchestrator` + `cmd/remediationorchestrator`) | 50 | `internal/controller/remediationorchestrator/reconciler.go` — **3,414 lines**, 68 funcs, `Reconcile()` cyclomatic complexity 39 |
| 5 | **Gateway** (`pkg/gateway` + `cmd/gateway`) | 24 | `pkg/gateway/server.go` — 2,511 lines, 47 funcs |
| 6 | **WorkflowExecution** (`internal/controller/workflowexecution` + `pkg/workflowexecution` + `cmd/workflowexecution`) | 14 | `workflowexecution_controller.go: reconcilePending()` — cyclomatic complexity 49 |
| 7 | **SignalProcessing** | 10 | `signalprocessing_controller.go: reconcileEnriching()` — cyclomatic complexity 38 |
| 8 | **Notification** | 12 | `pkg/notification/delivery/orchestrator.go: DeliverToChannels()` — **12 parameters** |
| 9 | **AIAnalysis** | 12 | `pkg/aianalysis/handlers/response_processor.go: ProcessIncidentResponse()` — cyclomatic complexity 29 |
| 10 | **EffectivenessMonitor** | 6 | `reconciler.go: NewReconciler()` — **10 parameters** |
| — | Everything else (shared, ogenx, pii, notification, auth, signalprocessing, config) | ~25 combined | — |

---

## 1. Functions with 8+ Parameters (32 total)

Confirms the user's direct observation — the two worst are exactly 12 parameters:

| Params | Function | Location |
|---|---|---|
| **12** | `DeliverToChannels` | `pkg/notification/delivery/orchestrator.go:195` |
| **12** | `buildMCPHandler` | `cmd/kubernautagent/main.go:1401` |
| 11 | `NewReconciler` | `internal/controller/effectivenessmonitor/reconciler.go:139` |
| 10 | `buildCompletionBody` | `pkg/remediationorchestrator/creator/notification.go:436` |
| 10 | `sameKindValidationGate` | `internal/kubernautagent/investigator/investigator_gates.go:33` |
| 10 | `runWorkflowSelection` | `internal/kubernautagent/investigator/investigator.go:917` |
| 9 | `BuildApprovalDecisionEvent` / `BuildApprovalRequestedEvent` | `pkg/remediationorchestrator/audit/manager.go:566,519` |
| 9 | `apiVersionValidationGate` | `internal/kubernautagent/investigator/investigator_gates.go:150` |
| 9 | `retryWorkflowSubmit` | `internal/kubernautagent/investigator/investigator.go:1134` |
| 9 | `NewReconciler` | `internal/controller/remediationorchestrator/reconciler.go:205` |
| 8 (×19 more) | see `revive argument-limit` raw output | `pkg/gateway/server.go`, `pkg/gateway/processing/errors.go`, `internal/kubernautagent/session/manager.go`, `internal/kubernautagent/enrichment/label_detector.go`, `internal/kubernautagent/tools/custom/resource_context.go`, `internal/controller/notification/notificationrequest_controller.go`, `internal/controller/workflowexecution/workflowexecution_controller.go`, `internal/controller/remediationorchestrator/{wfe_creation_helper,executing_handler}.go` |

**Recommended fix**: Options pattern (functional options) or a request/config struct per AGENTS.md. `BuildApprovalDecisionEvent`/`BuildApprovalRequestedEvent`/`BuildManualReviewEvent`/`BuildFailureEvent` in `manager.go` (4 functions, 8–9 params each) are the best starting point — they're pure builders with an obvious `AuditEventInput` struct extraction.

---

## 2. Excessive Complexity (Cyclomatic + Cognitive)

Top 20 by **cognitive complexity** (closer to human-perceived difficulty than cyclomatic):

| Cognitive | Cyclomatic | Function | Location |
|---|---|---|---|
| **133** | 51 | `HandleWatch` | `pkg/apifrontend/tools/crd_tools.go:645` |
| **117** | 88 | `buildEventData` | `internal/kubernautagent/audit/ds_store.go:88` |
| **108** | 70 | `main` | `cmd/kubernautagent/main.go:98` |
| **105** | 67 | `HandleInvestigationMCPWithRegistry` | `pkg/apifrontend/tools/ka_investigate_mcp.go:176` |
| 97 | 43 | `newPhaseGuard` | `pkg/apifrontend/agent/phase_guard.go:54` |
| 83 | 40 | `handleSubscribe` | `pkg/apifrontend/handler/status_handler.go:64` |
| 79 | 32 | `validateParameters` | `internal/kubernautagent/parser/validator.go:213` |
| 72 | 56 | `(*Config).Validate` | `pkg/datastorage/config/config.go:421` |
| 71 | 47 | `(*Config).Validate` | `internal/kubernautagent/config/config.go:528` |
| 70 | 27 | `MergeAuditData` | `pkg/datastorage/reconstruction/mapper.go:166` |
| 70 | 49 | `reconcilePending` | `internal/controller/workflowexecution/workflowexecution_controller.go:358` |
| 70 | 39 | `(*Reconciler).Reconcile` | `internal/controller/remediationorchestrator/reconciler.go:689` |

`buildEventData` (cyclomatic **88**, the single worst function in the repo) is a type-switch dispatcher over ~30 audit event types — a classic candidate for a lookup-table/registry-of-handlers refactor instead of one giant `switch`.

`Config.Validate` appearing twice in the top 10 (DataStorage and KubernautAgent) suggests the same anti-pattern repeated: one big validation function instead of composing smaller per-field/per-section validators.

Full ranked list (149 functions over complexity 15): `/tmp/gocyclo_report.txt` (regenerate with the command in Methodology).

---

## 3. Oversized Files (the "1000-line controller" problem)

| Lines | File | Functions in file |
|---|---|---|
| **3,414** | `internal/controller/remediationorchestrator/reconciler.go` | 68 |
| 2,511 | `pkg/gateway/server.go` | 47 |
| 1,897 | `internal/controller/workflowexecution/workflowexecution_controller.go` | — |
| 1,714 | `pkg/datastorage/server/workflow_handlers.go` | — |
| 1,695 | `cmd/kubernautagent/main.go` | — |
| 1,617 | `cmd/apifrontend/main.go` | — |
| 1,507 | `internal/kubernautagent/investigator/investigator.go` | — |
| 1,464 | `pkg/remediationorchestrator/creator/notification.go` | — |
| 1,326 | `internal/controller/notification/notificationrequest_controller.go` | — |
| 1,282 | `internal/kubernautagent/mcp/tools/investigate.go` | — |
| 1,195 | `pkg/remediationorchestrator/routing/blocking.go` / `internal/controller/signalprocessing/signalprocessing_controller.go` | — |

Note on `reconciler.go`: the RemediationOrchestrator package has **already** split several concerns into dedicated files (`analyzing_handler.go`, `blocked_handler.go`, `executing_handler.go`, `notification_handler.go`, `verifying_handler.go`, etc. — 17 files total). `reconciler.go` itself is what's left: the top-level `Reconcile()` dispatch (complexity 39), `NewReconciler()` (26 dependencies wired — complexity 25), timeout handling, effectiveness-assessment triggering, and phase-transition helpers. This is a **God file**, not a God function — the fix is continuing the same split pattern the package already uses for the remaining ~10 responsibilities still living in `reconciler.go`.

`pkg/gateway/server.go` (2,511 lines / 47 funcs) mixes server bootstrap, signal processing, and audit emission — same pattern, same fix.

---

## 4. God Structs (15+ fields) — 30 found

Two distinct sub-categories, **treated differently**:

**a) Legitimate wide data models / DTOs** (lower priority — flattening would hurt, not help, readability):
`RemediationWorkflow` (43 fields, DB row mapping), `InvestigationResult` (35), `AuditEvent` ×2 (32, 28), `SignalContext` (29), `RemediationAuditResult`, `RemediationAudit`, `WorkflowSearchResult` — these mirror external schemas (DB tables, OpenAPI payloads) and splitting them would require touching serialization boundaries for no behavioral gain.

**b) Behavioral "god objects"** (real anti-pattern — flag for decomposition):

| Fields | Struct | Location |
|---|---|---|
| 30 | `Reconciler` | `internal/controller/remediationorchestrator/reconciler.go:92` |
| 26 | `Server` | `pkg/gateway/server.go:131` |
| 26 | `Server` | `pkg/datastorage/server/server.go:64` |
| 26 | `AgentConfig` | `pkg/apifrontend/agent/config.go:30` |
| 21 | `NotificationRequestReconciler` | `internal/controller/notification/notificationrequest_controller.go:51` |
| 18 | `AnalyzingCallbacks` | `internal/controller/remediationorchestrator/analyzing_handler.go:54` |
| 18 | `Reconciler` | `internal/controller/effectivenessmonitor/reconciler.go:62` |
| 16 | `InvestigateTool` / `Investigator` / `Config` | `internal/kubernautagent/mcp/tools/investigate.go` / `internal/kubernautagent/investigator/investigator.go` (×2) |
| 15 | `Observer` | `internal/kubernautagent/alignment/observer.go:59` |
| 15 | `BufferedAuditStore` | `pkg/audit/store.go:102` |

**Recommended fix**: group related dependencies into sub-structs (e.g. `Reconciler{ deps ReconcilerDeps; timeouts TimeoutConfig; audit AuditDeps }`) — this is what the Options-pattern constructors above should build, closing the loop with the 8+-param finding (the same 4-5 reconcilers/servers appear in both lists).

---

## 5. Interface Pollution (5+ methods) — 16 found

| Methods | Interface | Location |
|---|---|---|
| 13 | `AutonomousSessionManager` | `internal/kubernautagent/mcp/tools/investigate.go:100` |
| 11 | `AWXClient` | `pkg/workflowexecution/executor/ansible.go:64` |
| 7 | `AuditClientInterface` | `pkg/aianalysis/handlers/interfaces.go:72` |
| 6 (×6) | `WorkflowQuerier`, `RemediationHistoryQuerier`, `Engine`, `Client` (notification), `SignalAdapter`, `ClusterRegistry` | see raw list |
| 5 (×4) | `Builder` (effectivenessmonitor audit), `WorkflowContentIntegrityRepository`, `MCPClient`, `AgentClientInterface`, `ToolMetrics`, `SessionManager`, `PolicyEvaluator` | see raw list |

`AutonomousSessionManager` (13 methods) and `AWXClient` (11 methods) are the two clearest candidates — both mix lifecycle management, querying, and mutation in one interface. Recommended split: separate read-only query interfaces from lifecycle/mutation interfaces (ISP — Interface Segregation Principle), consistent with AGENTS.md's "split into focused role interfaces."

---

## 6. `context.Context` Stored in Struct Fields — 2 found

Both have **documented design rationale** in code comments — flag for lead review, not blind removal:

1. `pkg/audit/store.go:112` — `ctx context.Context // Store-scoped context for retry loop cancellation` in `BufferedAuditStore`. Used to cancel a long-lived background flush goroutine, not a request-scoped operation.
2. `internal/kubernautagent/alignment/observer.go:68` — `evalCtx context.Context` in `Observer`, with an explicit `ARCH-3` comment explaining it decouples shadow-evaluation cancellation from the parent investigation context on purpose.

Both are the "object-lifecycle context" exception Go teams generally tolerate (vs. the forbidden "request context in struct" pattern) — but neither is unit-tested for the cancellation behavior it claims to provide. **Recommendation**: keep the pattern, add a short code comment reference to this audit + a targeted unit test proving the cancellation semantics, rather than restructuring.

---

## 7. Missing Slice/Map Pre-allocation — 13 found (`prealloc` linter) — ✅ RESOLVED (Phase 1)

Small, mechanical, zero-risk fixes:

`cmd/kubernautagent/main.go:848,1185`, `cmd/workflowexecution/main.go:346`, `internal/controller/notification/routing_handler.go:208`, `internal/kubernautagent/investigator/investigator_gates.go:87,237`, `pkg/datastorage/server/server.go:377`, `pkg/kubernautagent/llm/vertexanthropic/client.go:247`, `pkg/kubernautagent/tools/k8s/tools.go:61`, `pkg/notification/delivery/teams_cards.go:84`, `pkg/shared/hash/configmap.go:171`, `pkg/shared/tls/profile.go:90`, `pkg/workflowexecution/executor/job.go:330`.

All 13/13 fixed across two commits. The first 10 (single-source-length capacity, e.g. `make([]T, 0, len(src))`) landed directly. The remaining 3 aggregate capacity from multiple sub-slices/literals (`collectAllCredentialRefs`, `NewAllTools`, `buildCardBody`) and were initially deferred as "low value" — that judgment call was reassessed on request: the fix is equally mechanical (capture each sub-result in a local, sum `len()`s, `make` once), so all 3 were fixed too. `collectAllCredentialRefs` had zero prior test coverage and got a new characterization test (`routing_handler_test.go`) pinning aggregation order and empty-ref filtering before the refactor; the other two already had indirect coverage (`NewAllTools` via 4 existing test files, `buildCardBody` via `BuildTeamsPayload` tests). Zero regressions; `go build ./...`, targeted package tests, and `golangci-lint run` (repo config) all clean.

---

## 8. Variable Shadowing — 120 found, mostly low-risk

114/120 are `err` shadowing (`if err := f(); err != nil` repeated in the same scope — the single most common and least dangerous shadow pattern in Go, which is exactly why `govet -shadow` isn't part of default `go vet`). 1 `ctx` shadow, 1 `result`, 1 `username`, 1 `ok`, 1 `isString`. **No goroutine-closure-captures-loop-variable pattern was found** (the genuinely dangerous shadow bug) — none of the 120 hits are in a `for ... go func()` or `for ... defer func()` body.

**Recommendation**: low priority. Worth a mechanical rename pass only if the team wants `go vet -shadow` enabled permanently in CI; otherwise not worth the diff churn on its own.

---

## 9. Already Clean ✅

These AGENTS.md checks returned **zero findings** project-wide — no action needed:

- Ignored errors (`errcheck`)
- Error strings with uppercase/punctuation (`revive error-strings`)
- Naked returns (`revive bare-return`)
- `context.Context` not as first parameter (`revive context-as-argument`)

---

## Recommended Remediation Plan (proposed — not yet approved)

Per AGENTS.md, REFACTOR-phase cleanup must not introduce new types/components, must keep `go build ./...` green after every step, and needs explicit user approval before starting (Collaboration Rule 1 & 3 — this is a "refactoring that affects system complexity" architectural-adjacent change). Suggested phasing if approved:

| Phase | Scope | Risk | Est. effort |
|---|---|---|---|
| 1 | Mechanical, zero-behavior-change: `prealloc` (13), safe `err`-shadow renames if desired | Very low | 0.5 day |
| 2 | Options-pattern extraction for the 12 worst 8+-param functions (`DeliverToChannels`, `buildMCPHandler`, `NewReconciler` ×2, the 4 `Build*Event` audit builders) | Low-medium (touches call sites, needs UT re-run) | 2-3 days |
| 3 | Split `reconciler.go` (3,414→?) and `pkg/gateway/server.go` (2,511→?) into cohesive files following the existing `*_handler.go` pattern already used in RemediationOrchestrator | Medium (large diff, no logic change) | 3-4 days |
| 4 | Decompose the top complexity offenders: `buildEventData` (88), `HandleWatch` (cognitive 133), `Config.Validate` ×2, `main()` in `cmd/kubernautagent` and `cmd/apifrontend` | Medium-high (behavior-preserving refactor of dense logic, needs careful UT coverage first) | 5-7 days |
| 5 | Interface segregation for `AutonomousSessionManager` (13 methods) and `AWXClient` (11 methods) | Medium (ripples to all implementers/mocks) | 2-3 days |

**Not recommended for action**: DTO/data-model "god structs" (category 4a), `context.Context` struct fields (both documented), `any`/`interface{}` usage (spot-checked — the ~849 raw hits are overwhelmingly idiomatic JSON-decoding, `sync.Map`/`singleflight` third-party API signatures, and generic-JSON-passthrough code; no material violations found beyond one worth a look: `BuildTriagePrompt(input TriageInput, rules interface{})` in `pkg/apifrontend/severity/types.go:113`).

---

## Raw Data

Reproducible via the commands in Methodology; intermediate files used to build this report (not committed, regenerate as needed):
`/tmp/gocyclo_report.txt`, `/tmp/audit_lint_full.txt`, `/tmp/audit_lint2.txt`, `/tmp/audit_shadow.txt`.
