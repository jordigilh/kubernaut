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
| Functions with 8+ parameters | **32** (2 have 12) — corrected to **21** real production functions after `revive argument-limit` re-verification | 8+ params → Options pattern | ✅ RESOLVED (Phase 2) |
| God structs (15+ fields) | **30** | 15+ fields → decompose | 🟡 Mixed (some are legit DTOs) |
| Interface pollution (5+ methods) | **16** | 5+ methods → split into role interfaces | ✅ 5/16 split (Phase 5 + Wave 0b), 1 flagged dead-code, 1 deferred to Wave 1, 9 reclassified cohesive |
| `context.Context` stored in struct | **2** | Pass as first param | 🟡 Both have documented rationale |
| Functions over cyclomatic complexity 15 | **149** (25 over 20, worst = 88) → **145** post-Phase-4/re-scan, minus 6 `cmd/*/main.go` (Wave 0) | N/A (project convention) | 🟡 In progress (Phase 6+, Wave 0 done) |
| Functions over cognitive complexity 20 | **161** (worst = 133) | N/A (project convention) | 🟡 In progress (Phase 6+) |
| Functions over 80 lines / 60 statements | **54** | Visual inspection / nesting | 🟡 In progress (Phase 6+) |
| Deeply nested (`nestif`, complexity ≥5) | **111** | Unnecessary nesting (>3 levels) | 🟡 In progress (Phase 6+) |
| Low maintainability index (<20) | included above via funlen/gocognit overlap | N/A | — |
| Missing slice/map pre-allocation | **13** | Inefficient pre-allocation | ✅ RESOLVED (Phase 1) |
| Variable shadowing | **120** (114 are `err`, low-risk) | Shadowing → rename | 🟢 `err`/`ok` lint-excluded (pre-declared); 6 outliers rename pass prototyped, not yet committed |
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

**✅ RESOLVED (Phase 4, see §7f)** for the 5 functions actually in Phase 4's approved scope: `buildEventData` 88→2 (registry pattern, DD-AUDIT-008), `HandleWatch` 51→18, `main` (KubernautAgent) 70→43, and both `Config.Validate` implementations decomposed into per-section validators (DataStorage 56→9, KubernautAgent 47→8). `run` (apifrontend, not in this table but decomposed alongside KubernautAgent's `main` per the roadmap's Phase 4 description) 50→41. `HandleInvestigationMCPWithRegistry`, `newPhaseGuard`, `handleSubscribe`, `validateParameters`, `MergeAuditData`, `reconcilePending`, and `(*Reconciler).Reconcile` were **not** in Phase 4's approved scope and remain open — none of them dropped below complexity 30 as a side effect (see §7f's re-run table).

Full ranked list (149 functions over complexity 15): `/tmp/gocyclo_report.txt` (regenerate with the command in Methodology; now stale post-Phase-4 — see §7f for current top offenders).

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

**✅ Annotated**: no golangci-lint linter in this repo's toolchain checks struct field count (the 30-count above comes from this audit's own regex-based scan, not a `go vet`/`golangci-lint` rule), so there is no lint suppression to apply here. Instead, each of the 8 hand-written DTOs above (excluding OpenAPI-generated `ogen-client` schemas, which are regenerated and would lose manual comments) got a short doc-comment addition pointing back to this section, for the benefit of future readers/auditors who re-run this scan: `pkg/kubernautagent/types/types.go` (`InvestigationResult`, `SignalContext`), `pkg/datastorage/models/workflow.go` (`RemediationWorkflow`, `WorkflowSearchResult`), `pkg/datastorage/models/audit.go` (`RemediationAudit`), `pkg/datastorage/query/types.go` (`RemediationAuditResult`), `pkg/audit/event.go` and `pkg/datastorage/repository/audit_events_repository.go` (the two `AuditEvent` structs).

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

**✅ RESOLVED (Phase 5, see §7g)**: both split into focused role interfaces, composed back under their original names for the existing single consumer of each.

**✅ RESOLVED (Phase 6 Wave 0b, see §7h)**: `Engine`, `ClusterRegistry`, and `SessionManager` split the same way, after a preflight confirmed each had a real query-vs-lifecycle usage split among its consumers (not just a hypothetical one). `Client` (notification) has **zero consumers repo-wide** — flagged as a dead-code question, not an ISP fix (see §7h). `MCPClient` is deferred to Wave 1 (APIFrontend) — its 2 implementers are not method-uniform and its ~10+ call sites each use only one method, so the real ISP value requires call-site narrowing disproportionate to a "quick win" wave.

**Reclassified as assessed, no fix needed**: the remaining 9 interfaces (`AuditClientInterface`, `WorkflowQuerier`, `RemediationHistoryQuerier`, `SignalAdapter`, `Builder`, `WorkflowContentIntegrityRepository`, `AgentClientInterface`, `ToolMetrics`, `PolicyEvaluator`) were spot-checked and found to be genuinely cohesive — pure builders, deliberate ADR-060 consolidation, or single-lifecycle facades with no mixed query/mutation concern to split. No further action tracked for these.

---

## 6. `context.Context` Stored in Struct Fields — 2 found

Both have **documented design rationale** in code comments — flag for lead review, not blind removal:

1. `pkg/audit/store.go:112` — `ctx context.Context // Store-scoped context for retry loop cancellation` in `BufferedAuditStore`. Used to cancel a long-lived background flush goroutine, not a request-scoped operation.
2. `internal/kubernautagent/alignment/observer.go:68` — `evalCtx context.Context` in `Observer`, with an explicit `ARCH-3` comment explaining it decouples shadow-evaluation cancellation from the parent investigation context on purpose.

Both are the "object-lifecycle context" exception Go teams generally tolerate (vs. the forbidden "request context in struct" pattern) — but neither is unit-tested for the cancellation behavior it claims to provide. **Recommendation**: keep the pattern, add a short code comment reference to this audit + a targeted unit test proving the cancellation semantics, rather than restructuring.

**✅ Annotated**: both fields now carry `//nolint:containedctx // ... reviewed and accepted in GO-ANTIPATTERN-AUDIT-2026-07-01 §6` directives. The `containedctx` linter (available in golangci-lint v2.9.0, confirmed via `golangci-lint linters`) is not enabled in the committed `.golangci.yml` today, so this is a forward-looking suppression — if a future PR enables `containedctx` repo-wide, these two pre-reviewed fields won't break CI while every other struct-embedded context is still caught. The targeted unit test proving cancellation semantics remains open follow-up work, not covered by this annotation pass.

---

## 7. Missing Slice/Map Pre-allocation — 13 found (`prealloc` linter) — ✅ RESOLVED (Phase 1)

Small, mechanical, zero-risk fixes:

`cmd/kubernautagent/main.go:848,1185`, `cmd/workflowexecution/main.go:346`, `internal/controller/notification/routing_handler.go:208`, `internal/kubernautagent/investigator/investigator_gates.go:87,237`, `pkg/datastorage/server/server.go:377`, `pkg/kubernautagent/llm/vertexanthropic/client.go:247`, `pkg/kubernautagent/tools/k8s/tools.go:61`, `pkg/notification/delivery/teams_cards.go:84`, `pkg/shared/hash/configmap.go:171`, `pkg/shared/tls/profile.go:90`, `pkg/workflowexecution/executor/job.go:330`.

All 13/13 fixed across two commits. The first 10 (single-source-length capacity, e.g. `make([]T, 0, len(src))`) landed directly. The remaining 3 aggregate capacity from multiple sub-slices/literals (`collectAllCredentialRefs`, `NewAllTools`, `buildCardBody`) and were initially deferred as "low value" — that judgment call was reassessed on request: the fix is equally mechanical (capture each sub-result in a local, sum `len()`s, `make` once), so all 3 were fixed too. `collectAllCredentialRefs` had zero prior test coverage and got a new characterization test (`routing_handler_test.go`) pinning aggregation order and empty-ref filtering before the refactor; the other two already had indirect coverage (`NewAllTools` via 4 existing test files, `buildCardBody` via `BuildTeamsPayload` tests). Zero regressions; `go build ./...`, targeted package tests, and `golangci-lint run` (repo config) all clean.

---

## 7b. Functions with 8+ Parameters — 21 found (`revive argument-limit`) — ✅ RESOLVED (Phase 2)

Preflight with `revive argument-limit: 7` corrected the initial 32-count estimate (which double-counted some findings across `funlen`/`gocognit` overlap and missed 2 functions later discovered during remediation) to **21 real production functions**, all fixed via the Options-pattern (a dedicated `*Params`/`*Deps` struct grouping the excess arguments), following AGENTS.md's TDD mandate: for every zero-coverage target, a characterization/smoke test was written first to pin existing behavior before the signature change.

| Function(s) | File | Coverage before fix | Fix |
|---|---|---|---|
| `BuildApprovalRequestedEvent`, `BuildApprovalDecisionEvent` | `pkg/remediationorchestrator/audit/manager.go` | Existing UTs (8 call sites) | `ApprovalEventContext` struct |
| `NewOperationError`, `NewCRDCreationError` | `pkg/gateway/processing/errors.go` | Existing UTs | `OperationErrorParams`, `CRDCreationErrorParams` structs |
| `DeliverToChannels` | `pkg/notification/delivery/orchestrator.go` | Existing UTs incl. TOCTOU race test | `DeliveryCallbacks` struct (6 callbacks) |
| `runRCA`, `retryRCASubmit`, `runWorkflowSelection`, `retryWorkflowSubmit`, `runLLMLoop`, `sameKindValidationGate`, `apiVersionValidationGate` | `internal/kubernautagent/investigator/{investigator,investigator_gates}.go` | Indirect coverage (32 test files) | shared `LLMInvocationContext` struct |
| `launchInvestigation`, `emitSessionEvent` | `internal/kubernautagent/session/manager.go` | Indirect coverage (22 test files) | `investigationLaunchParams`, `sessionEventParams` structs |
| `buildMCPHandler` | `cmd/kubernautagent/main.go` | **Zero** — new characterization test added (`mcp_handler_guards_test.go`) | `mcpHandlerParams` struct |
| `NewReconciler` (EffectivenessMonitor) | `internal/controller/effectivenessmonitor/reconciler.go` | **Zero** — new smoke test added (`reconciler_new_test.go`) | `ReconcilerDeps` struct |
| `detectCNV` | `internal/kubernautagent/enrichment/label_detector.go` | Existing UTs (5 test files) | `cnvDetectionTarget` struct |
| `fetchRemediationHistory` | `internal/kubernautagent/tools/custom/resource_context.go` | Existing UTs | `remediationHistoryQuery` struct |
| `createServerWithClients`, `NewServerForTesting` | `pkg/gateway/server.go` | Existing UTs (14+ test files); `NewServerForTesting` discovered mid-batch (also 8 params, 11 call sites across 8 test files) | `serverClients`, `ServerTestDeps` structs |
| `buildCompletionBody` | `pkg/remediationorchestrator/creator/notification.go` | Existing UTs (7 test files, indirect via `CreateCompletionNotification`) | `completionBodyParams` struct; dropped an unused `*AIAnalysis` param |
| `NewReconciler` (RemediationOrchestrator) | `internal/controller/remediationorchestrator/reconciler.go` | Existing UTs — **largest blast radius**: ~124 call sites across 28 files (1 production, 27 test) | `ReconcilerDeps` struct (8 fields + preserved variadic `eaCreator`) |

**Verification per batch**: `go build ./...`, targeted `go test` (all passing, zero regressions — the RO batch alone re-ran 370 Ginkgo specs), `gofmt -l` clean, and `golangci-lint run` (repo config + a scratch `argument-limit: 7` config) confirming 0 issues.

**Process note — a real near-miss, corrected**: the EffectivenessMonitor `NewReconciler` batch was declared complete after fixing its one production call site (`cmd/effectivenessmonitor/main.go`) and running `go build ./...`, which does **not** compile `_test.go` files. 18 call sites in `pkg/effectivenessmonitor/*_test.go` and `test/integration/effectivenessmonitor/**` were left on the old positional signature — a silent break that `go build` could not catch. It was caught by a final repo-wide `go vet ./...` sanity pass (which does compile test files) run at the end of Phase 2, and fixed in a follow-up commit. **Action item for future phases**: always run `go vet ./...` (not just `go build ./...`) across the full repo before declaring a signature-changing refactor complete, since `go build` alone gives false confidence when call sites live only in test files.

---

## 7c. Pre-Phase-3 Coverage Gate — Audit-Emission Gap Closure (BR-AUDIT-005, SOC2 CC8.1) — ✅ RESOLVED

Before starting Phase 3 (splitting `reconciler.go` and `pkg/gateway/server.go`), per-function coverage was pulled for both files (`go test -coverprofile` + `go tool cover -func`) to confirm regressions from the mechanical split would be caught by tests.

**RemediationOrchestrator `reconciler.go` — real gap found and closed.** Four audit-emission functions sat at 15.8–36.4% line coverage: `emitEACreatedAudit`, `emitVerificationCompletedAudit`, `emitVerificationTimedOutAudit`, `emitCompletionAudit`. Tracing every call site (`ea_creation_test.go`, `verifying_phase_test.go`, `cascade_terminal_test.go`, `apply_transition_test.go`, `characterization_test.go`) showed every one of them wires `AuditStore: nil` — so only the `if r.auditStore == nil { return }` guard clause was ever exercised. The actual business logic (EA-name/duration extraction from the RR, GitOps/CRD propagation-delay breakdown, correlation-ID wiring) had **zero** verification, which is a direct BR-AUDIT-005 / SOC2 CC8.1 (audit completeness) / DD-AUDIT-003 gap: these are exactly the events required to reconstruct the remediation lifecycle from audit traces.

Added `internal/controller/remediationorchestrator/audit_coverage_gaps_test.go` (6 new Ginkgo specs, `AE-COV-001..006`), each driving the reconciler through its public API (`Reconcile`/`ApplyTransition`) with a `MockAuditStore` and asserting on the emitted event's `RemediationOrchestratorAuditPayload` fields:

| Test | Function(s) exercised | Business assertion |
|---|---|---|
| AE-COV-001 | `emitEACreatedAudit` | plain Deployment target → `isGitopsManaged=false`, `isCrd=false`, no delay fields set |
| AE-COV-002 | `emitEACreatedAudit` | RCA-detected GitOps target → `isGitopsManaged=true`, `GitopsSyncDelay`/`HashComputeDelay` populated (BR-RO-103.2) |
| AE-COV-003 | `emitVerificationCompletedAudit` | EA terminal → correct `EaName`/`DurationMs` extracted from RR status |
| AE-COV-004 | `emitVerificationTimedOutAudit` + `emitCompletionAudit` | expired `VerificationDeadline` → both events fire, `CrdOutcome=VerificationTimedOut` |
| AE-COV-005 | `emitCompletionAudit` | inherited completion → `CrdOutcome=InheritedCompleted` |
| AE-COV-006 | `emitCompletionAudit` | dry-run completion (#712/#736) → `CrdOutcome=DryRun` |

**Result**: `emitEACreatedAudit` 15.8%→73.7%, `emitVerificationCompletedAudit` 23.1%→76.9%, `emitVerificationTimedOutAudit` 23.1%→76.9%, `emitCompletionAudit` 36.4%→72.7%; package coverage 73.6%→75.1%. All 6 new specs pass, full package suite (376 specs) green, `go build ./...` and `golangci-lint run` (repo config) both clean. Remaining uncovered lines are the `BuildXxxEvent`/`StoreAudit` error-logging branches (require a failing audit-manager/store double; lower value, deferred).

**Gateway `pkg/gateway/server.go` — no action needed (pyramid is working as designed).** UT-only coverage on the `emit*Audit` functions looks low (`emitSignalReceivedAudit` 9.1%, `emitSignalDeduplicatedAudit`/`emitCRDCreationFailedAudit` 0%, `emitCRDCreatedAudit` 12.5%), but `test/integration/gateway/audit_emission_integration_test.go` has 20 dedicated `GW-INT-AUD-*` scenarios (envtest-backed) mapped 1:1 to `BR-GATEWAY-055/056/057/058` that assert on these exact events and fields (correlation IDs, `occurrence_count`, `error_type` transient-vs-permanent, target-resource metadata). This matches the intended pyramid shape (UT proves logic in isolable pieces; IT proves the K8s-CRD-creation wiring these functions depend on) rather than a genuine gap. Caveat: this could not be re-verified by running the IT suite in this environment (no `envtest`/`etcd` binaries available locally) — confirmed instead by reading the IT spec descriptions and confirming field-level assertions exist for each function's behavior.

---

## 7d. Phase 3a: `reconciler.go` File Split — ✅ RESOLVED

Split `internal/controller/remediationorchestrator/reconciler.go` (3,435 lines) into 7 same-package files, in leaf-first order, one commit per file, verified after each extraction with `go build ./...`, `go vet ./...` (repo-wide), the full RO Ginkgo suite (376 specs), `gofmt -l`, and `golangci-lint run`. Pure structural moves — zero behavior change, zero public-API change, no test files touched (same-package moves are invisible to Go's symbol resolution).

| File | Contents | Lines |
|---|---|---|
| `reconciler.go` (trimmed) | `Reconciler` struct, `TimeoutConfig`/`ReconcilerDeps`, `errPhaseAlreadySet`, `NewReconciler`, `Reconcile`, `SetupWithManager` | 1,023 |
| `terminal_transitions.go` | `errPhaseConflict`, `transitionToInheritedCompleted`, `transitionToCompletedWithoutVerification`, `transitionToInheritedFailed`, `handleBlocked`, `transitionPhase`, `VerificationDeadlineBuffer`, `transitionToVerifying`, `transitionToFailed`, `handleGlobalTimeout`, `createEffectivenessAssessmentIfNeeded` | 985 |
| `audit_events.go` | All 14 `emit*Audit` methods (DD-AUDIT-003) + `resolveWorkflowDisplay` | 651 |
| `timeout_management.go` | Timeout defaults/validation, effective-timeout resolution, phase-timeout detection/handling, `validateTimeoutConfig` | 389 |
| `notification_creation.go` | `hasNotificationRef`, `ensureNotificationsCreated`, `buildNotificationRef`, `buildTimeoutContext`, `createPhaseTimeoutNotification` | 260 |
| `pre_remediation_hash.go` | `resolveDualTargets`, `formatRemediationTargetString`, `CapturePreRemediationHash` (exported, cross-package consumer), `resolveConfigMapHashes` | 204 |
| `config_accessors.go` | `IsTerminalPhase` + `Set*`/`Get*Exported` test-exposure shims | 142 |
| **Total** | | **3,654** |

**Result**: `reconciler.go` shrank from 3,435 → 1,023 lines (largest remaining file across the split is `terminal_transitions.go` at 985). All 376 Ginkgo specs pass identically before and after every one of the 6 extraction commits; `go build ./...`, `go vet ./...` (repo-wide), `gofmt -l`, and `golangci-lint run` all report zero issues on the final layout. Gateway `server.go` split (Phase 3b) tracked separately below.

---

## 7e. Phase 3b: `server.go` File Split — ✅ RESOLVED

Split `pkg/gateway/server.go` (2,552 lines) into 6 same-package files, in leaf-first order, one commit per file, verified after each extraction with `go build ./...`, `go vet ./...` (repo-wide), the full Gateway Ginkgo suite (154 specs), `gofmt -l`, and `golangci-lint run`. Pure structural moves — zero behavior change, zero public-API change, no test files touched (same-package moves are invisible to Go's symbol resolution).

| File | Contents | Lines |
|---|---|---|
| `server.go` (trimmed) | `Server` struct, event-type/action consts, `LivenessHandler`/`ReadinessHandler`/`MarkCacheReady`/`GetMetrics`, `setupRoutes`/`wrapWithMiddleware`/`performanceLoggingMiddleware`/`Handler`/`GetCachedClient`/`GetAPIReader`, `Start`/`Stop`, `healthHandler`/`readinessHandler`/`writeReadinessUnavailable` | 633 |
| `signal_ingestion.go` | `RegisterAdapter`, `createAdapterHandler`, `handleBatchRequest`, `processSingleSignal`, `processMultiSignalBatch`, `readParseValidateSignal`, `handleProcessingError`, `sendSuccessResponse`, `validateScope`, `ProcessSignal`, `handleDuplicateSignal`, `createRemediationRequestCRD` | 761 |
| `server_constructors.go` | `NewServer`, `NewServerWithK8sClient`, `newAuthMiddleware`, `ServerTestDeps`, `NewServerForTesting`, `NewServerWithMetrics`, `serverClients`, `createServerWithClients` | 642 |
| `audit_emission.go` | `extractRRReconstructionFields`, `EmitConfigReloadAudit`, `emitSignalReceivedAudit`, `emitSignalDeduplicatedAudit`, `emitCRDCreatedAudit`, `retryAuditObserver`+`OnRetryAttempt`, `emitCRDCreationFailedAudit`, `constructReadableCorrelationID` (DD-AUDIT-003) | 485 |
| `response_types.go` | `ProcessingResponse`, `StatusCreated`/`StatusDeduplicated`, `BatchProcessingResponse`, `ProcessingResult`, `BatchSummary`, `NewDuplicateResponseFromRR`, `NewCRDCreatedResponse` | 147 |
| `http_errors.go` | `writeJSONError`, `getErrorTypeAndTitle`, `writeValidationError`, `writeInternalError` | 121 |
| **Total** | | **2,789** |

**Result**: `server.go` shrank from 2,552 → 633 lines (largest remaining file across the split is `signal_ingestion.go` at 761). All 154 Ginkgo specs in `pkg/gateway` pass identically before and after every one of the 5 extraction commits; `go build ./...`, `go vet ./...` (repo-wide — the two pre-existing failures in `docs/spikes/multi-cluster-mcp-gateway/` predate this change and are unrelated to `pkg/gateway`), `gofmt -l`, and `golangci-lint run` all report zero issues on the final layout. Phase 3 (both sub-phases) is now complete.

---

## 7f. Phase 4: Decompose the Top Complexity Offenders — ✅ RESOLVED

Per AGENTS.md's Wiring-First TDD / coverage-before-refactor mandate, each target was preceded by a coverage check; two had real gaps that were closed with new UT before the refactor (§7f-0). All five decompositions are pure extract-method/extract-type refactors — no behavior change, no new exported types beyond what each target needed for its own decomposition, and no production wiring changed (`cmd/` entry points still call the same functions with the same signatures externally).

### 7f-0. Coverage gate: DataStorage `(*Config).Validate`

`pkg/datastorage/config/config.go`'s `Validate()` sat at 61.8% coverage (`-coverpkg=./pkg/datastorage/config/...`), with the production-only (`Environment: "production"`) branches for SC-8 (TLS enforcement) and AC-4 (auth-token requirement), the Redis TLS `CAFile`-exists check, and 7 duration-parse error branches all unexercised. Added `pkg/datastorage/config_production_security_test.go` with new Ginkgo specs mapped to **SC-8**, **AC-4**, and **SI-10** control objectives (per BR-AUDIT-005/ADR-034's control-mapping mandate), each asserting the specific business rule (e.g. "production + TLS disabled → rejected", "production + no auth token configured → rejected", "Redis TLS enabled + CAFile path does not exist → rejected") rather than just executing the line. Coverage rose to 80.6% (`go test ./pkg/datastorage/ -run TestDataStorageUnit -coverpkg=./pkg/datastorage/config/...`); KubernautAgent's `(*Config).Validate` already had adequate existing coverage and needed no new tests.

### 7f-1. `(*Config).Validate` ×2 — decomposed into per-section validators

| Function | File | Before | After |
|---|---|---|---|
| `(*Config).Validate` | `pkg/datastorage/config/config.go` | cyclomatic 56 | 9 (dispatches to `validateServer`, `validateDatabase`, `validateRedis`, `validateAudit`, `validateProduction`(SC-8/AC-4), `validateReconstruction`) |
| `(*Config).Validate` | `internal/kubernautagent/config/config.go` | cyclomatic 47 | 8 (dispatches to 5 per-section validators covering LLM/tools/session/audit/production settings) |

Each private validator returns a plain `error`; `Validate()` itself is now a flat sequence of `if err := v.validateX(); err != nil { return err }` calls — identical error messages and ordering preserved (characterization-tested by the pre-existing + new §7f-0 specs, all of which pass unchanged).

### 7f-2. `buildEventData` — registry pattern (DD-AUDIT-008)

`internal/kubernautagent/audit/ds_store.go`'s `buildEventData` (cyclomatic 88, cognitive 117 — the single worst function in the repo) was a 29-case type-switch mapping `AuditEvent.EventType` to one of 29 typed `ogenclient.AuditEventRequestEventData` payload variants. Per user decision (registry pattern over "extract helpers, keep the switch"), replaced the switch with an `eventDataBuilders map[string]eventDataBuilder` lookup table plus 29 extracted `build<EventType>Payload` functions — one per case, each a flat struct-literal builder with zero branching. `buildEventData` itself is now a 2-line map lookup (complexity 2). Full design rationale, alternatives considered, and consequences documented in [DD-AUDIT-008](../decisions/DD-AUDIT-008-audit-event-builder-registry-pattern.md). All 139 existing tests in the package pass unchanged; coverage held at 92%.

### 7f-3. `HandleWatch` — `watchLoopState` + per-event-type handler methods

`pkg/apifrontend/tools/crd_tools.go`'s `HandleWatch` (cyclomatic 51, cognitive 133 — highest cognitive complexity in the repo) ran a `for { select { ... } }` loop over 4 channels (context-done, RemediationRequest watch, RemediationApprovalRequest watch, lazily-started EffectivenessAssessment watch), each case body inlined. Extracted a private `watchLoopState` struct holding the loop's mutable state (`events`, `lastSeenPhase`, `eaCh`, `eaWatcher`, etc.) and three methods — `handleRREvent`, `handleRAREvent`, `handleEAEvent` — one per watched-resource-type case. The lazily-created `eaWatcher`'s lifecycle, previously stopped ad-hoc wherever the loop returned, is now guaranteed via a single `defer state.stopEAWatcher()` right after `state` is constructed. `HandleWatch` complexity: 51→18; no extracted method exceeds 19. Full `pkg/apifrontend` suite passes (one unrelated flaky timing test in `launcher/active_context_registry_test.go` was confirmed pre-existing and unrelated by isolated re-run).

### 7f-4. `cmd/kubernautagent main()` — 4 named builder functions

`main()` (cyclomatic 70) inlined LLM client construction, the shadow-agent alignment stack, the investigation runner (investigator + session store/manager + OGEN server), and hot-reload watcher wiring (certs, LLM runtime config, CA file) into one function body. Extracted `buildLLMClients`, `buildAlignmentStack`, `buildInvestigationRunner` (via an `investigationRunnerParams` struct — the block has too many cross-dependencies for a flat param list, consistent with AGENTS.md's Options-pattern rule), and `wireHotReload`, the last of which returns a single combined cleanup closure `defer`red once in `main()` (same pattern as §7f-3's `stopEAWatcher`). `main()`: 70→43; largest extracted helper (`buildInvestigationRunner`) at 17. Critical `os.Exit(1)`-on-error semantics and all `defer` ordering preserved exactly. Full package test suite passes.

### 7f-5. `cmd/apifrontend run()` — `buildRouterAndServers`

`run()` (cyclomatic 50) inlined chi-router construction, TLS configuration (including hot-reloadable cert and CA-file watchers), and the API/health/metrics `*http.Server` construction. Extracted `buildRouterAndServers` (with a `routerBuildParams` struct, mirroring §7f-4's pattern) which logs the identical failure messages the inline code used to on error and returns them to `run()`, which just does `return 1` (no duplicate logging) — and returns a single cleanup closure stopping both the cert and CA-file watchers, `defer`red once in `run()`. `run()`: 50→41. All 6 existing `cmd/apifrontend` test files pass unchanged.

**Repo-wide verification (Phase 4 exit gate)**: `go build ./...`, `go vet ./...`, `golangci-lint run ./...` (0 new issues — the 7 pre-existing `docs/spikes/` findings predate Phase 4 and are unrelated to any Phase 4 file), and `make test` (full unit-test tier across all services) all green. The one `make test` failure, `test-unit-spike-mcp-stream`, is a pre-existing empty Ginkgo suite in `cmd/spike-mcp-stream` (no `_test.go` files exist in that directory) — unrelated to and untouched by Phase 4. Re-running `gocyclo -over 30` repo-wide post-Phase-4 shows every hand-written function from the original top-20 table that was in scope now below the top offenders; the current top-30 list is dominated entirely by generated ogen client/server code (`pkg/datastorage/ogen-client/*_gen.go`, `pkg/agentclient/*_gen.go` — out of scope, not hand-written) plus a handful of `test/infrastructure/*.go` E2E setup functions and the intentionally-out-of-scope `reconcilePending`/`(*Reconciler).Reconcile`/`newPhaseGuard`/`HandleInvestigationMCPWithRegistry`. Phase 4 is now complete.

---

## 7g. Phase 5: Interface Segregation for `AutonomousSessionManager` and `AWXClient` — ✅ RESOLVED

Preflight mapped both interfaces to exactly **one** production consumer and **one** production implementer each (`AutonomousSessionManager` → `InvestigateTool`/`*session.Manager`; `AWXClient` → `AnsibleExecutor`/`AWXHTTPClient`), with all call sites and ~6 test doubles confined to 2 files per interface and zero type assertions to either interface anywhere in the repo. A spike confirmed the mechanical approach: because Go interfaces are structurally typed, splitting each into focused role interfaces and re-composing them under the original name (`type AutonomousSessionManager interface { AutonomousSessionQuerier; AutonomousSessionLifecycle }`, mirroring `io.ReadWriter`) requires **zero** changes to any implementer, mock, or call site — only the interface declaration itself changes. This made the blast radius smaller than any Phase 4 target, and no DD was required (interface composition into a named union is standard idiomatic Go, not a new pattern for this codebase).

| Original interface | Split into | Grouping rationale |
|---|---|---|
| `AutonomousSessionManager` (13, `internal/kubernautagent/mcp/tools/investigate.go`) | `AutonomousSessionQuerier` (5: `FindByRemediationID`, `FindPendingByRemediationID`, `GetLatestRCASummaryByRemediationID`, `GetLatestRCAResultByRemediationID`, `GetSessionLazySink`) + `AutonomousSessionLifecycle` (8: `CancelInvestigation`, `SuspendInvestigation`, `TransitionToUserDriving`, `ForceTransitionToUserDriving`, `UpgradeToInteractive`, `LaunchDeferredInvestigation`, `StartInvestigation`, `Subscribe`) | Read-only lookups vs. state-mutating/session-lifecycle actions, per the audit's own §5 recommendation |
| `AWXClient` (11, `pkg/workflowexecution/executor/ansible.go`) | `AWXJobClient` (5: `LaunchJobTemplate`, `LaunchJobTemplateWithCreds`, `GetJobStatus`, `CancelJob`, `FindJobTemplateByName`) + `AWXCredentialClient` (6: `CreateCredentialType`, `FindCredentialTypeByName`, `FindCredentialTypeByKind`, `CreateCredential`, `DeleteCredential`, `GetJobTemplateCredentials`) | Job-template operations vs. credential-lifecycle operations (BR-WE-015) — two distinct AWX/AAP API resource domains |

**Caveat, documented rather than hidden**: both interfaces have exactly one consumer today, so this is lower-value than typical ISP remediation (no second caller is currently blocked by the wide interface) — it was done because it was in the approved roadmap and effectively free (mechanical, zero downstream changes), and it sets up a clean seam if a narrower second consumer appears later.

**Verification**: `go build ./...`, `go vet ./...` (both repo-wide), `golangci-lint run` on both affected package trees (0 issues), and the full existing test suites for `internal/kubernautagent/mcp/...` and `pkg/workflowexecution/...` (0 test changes required, all green) — confirming the spike's zero-blast-radius prediction. Phase 5 is now complete; the remediation roadmap has no further open items.

---

## 7h. Phase 6 Wave 0: `cmd/*/main.go` Decomposition + 3-Interface ISP Split — ✅ RESOLVED

Phase 4 (§7f-4/7f-5) decomposed `main()`/`run()` for `cmd/kubernautagent` and `cmd/apifrontend` only. A post-Phase-5 re-scan (tracked as "Phase 6+ Complexity and File-Size Burndown", Wave 0 of a 7-wave plan) found the other 6 services' `cmd/*/main.go` had the exact same "wire everything inline" shape, all with **zero existing test coverage** (unlike KubernautAgent, which had 7 characterization tests before Phase 4). Wave 0 closed both gaps: characterization tests first (per AGENTS.md's TDD-before-refactor mandate), then mechanical `build*`/`wire*`/`register*` helper extraction, mirroring the Phase 4f pattern.

### 7h-1. `main()` decomposition — 6 services

| Service | `main()` before | `main()` after | New helpers | New characterization tests |
|---|---|---|---|---|
| `cmd/gateway` | 30 | 13 | `buildAPIRegistry`, `registerAdapters`, `wireHotReload` | `TestRegisterAdapters_FleetDisabled`, `TestRegisterAdapters_FleetEnabledUnreachable` |
| `cmd/remediationorchestrator` | 40 | 15 | `buildManager`, `buildAuditStore`, `buildRoutingEngine`, `buildReconciler`, `wireTLSHotReload` | `TestBuildAuditStore_ValidConfig`, `TestBuildAuditStore_EmptyDataStorageURL` |
| `cmd/notification` | >40 | 13 | `buildManager`, `buildDeliveryServices`, `buildAuditStore`, `buildDeliveryOrchestrator`, `wireHotReload` | `TestBuildAuditStore_ValidConfig`, `TestBuildDeliveryServices_Defaults`, `TestBuildDeliveryServices_FileDeliveryEnabled`, `TestBuildDeliveryServices_FileDeliveryUnwritableDir` |
| `cmd/datastorage` | 45 | 19 | `loadDataStorageConfig`, `buildK8sAuthDeps`, `buildDependencyValidator`, `buildServerWithRetry`, `startObservabilityServers`, `wireHotReload` | `TestLoadDataStorageConfig_MissingFile`, `TestLoadDataStorageConfig_ValidConfigWithSecrets`, `TestLoadDataStorageConfig_MissingSecretsFile` |
| `cmd/workflowexecution` | 40 | 10 | `loadWorkflowExecutionConfig`, `buildManager`, `buildAuditStore`, `wireTLS`, `buildClientFactory`, `buildExecutorRegistry`, `registerAnsibleExecutor`, `registerHealthChecks`, `wireShutdownHooks`, `wireLogLevelHotReload` | `TestLoadWorkflowExecutionConfig_{DefaultsWhenPathEmpty,MissingFile,AppliesLogLevelAndValidates}`, `TestBuildAuditStore_{ValidConfig,EmptyDataStorageURL}`, `TestRegisterAnsibleExecutor_{NilConfigIsNoop,MissingTokenSecretRefIsNoop}` |
| `cmd/effectivenessmonitor` | 34 | 13 | `loadEffectivenessMonitorConfig`, `buildManager`, `buildAuditStore`, `waitForCAFile`, `buildExternalHTTPClient`, `buildExternalClients`, `registerHealthChecks`, `wireHotReload` | `TestLoadEffectivenessMonitorConfig_{DefaultsWhenPathEmpty,MissingFile,AppliesLogLevel}`, `TestBuildAuditStore_{ValidConfig,EmptyDataStorageURL}`, `TestWaitForCAFile_{ReturnsImmediatelyWhenPresent,TimesOutWhenMissing}`, `TestBuildExternalClients_{DisabledReturnsNilClients,EnabledReturnsNonNilClients}` |

Two files needed extra care preserving non-trivial control flow (flagged in preflight, both landed without incident):

- **`cmd/effectivenessmonitor/main.go`**: the external-client wiring block has a blocking retry/poll loop (`waitForCAFile`) waiting for the OCP service-ca operator to populate a ConfigMap-mounted CA bundle, with a timeout deadline and a fatal exit on failure — extracted as a standalone, independently-testable function rather than inlined into the larger `buildExternalHTTPClient` helper.
- **`cmd/workflowexecution/main.go`**: the executor registry block has CRD-discovery branching (REST-mapper probe for Tekton) plus an optional Ansible/AWX secret read, with `knownOptionalEngines` availability bookkeeping threaded through — `buildExecutorRegistry` carries that state correctly, and the Ansible-specific guard clauses were further split into `registerAnsibleExecutor` to keep both extracted functions well under the complexity budget.

Every extraction is a pure behavior-preserving move: no `os.Exit`/`defer` ordering changed, no new exported types beyond the params/result structs each helper needed for its own signature, and no production wiring changed externally (each `cmd/<service>/main.go` still starts the same manager, registers the same controller, and serves the same health/metrics endpoints).

### 7h-2. Interface ISP split — `Engine`, `ClusterRegistry`, `SessionManager`

A companion spot-check of the 14 interfaces not already resolved in Phase 5 found 5 genuine mixed-concern candidates (query methods mixed with lifecycle/mutation methods, the same shape as `AutonomousSessionManager`/`AWXClient`). Preflight narrowed the "quick win" scope to 3 by checking blast radius (implementers, consumers, existing mocks) for each:

| Interface | Location | Split into | Blast radius |
|---|---|---|---|
| `Engine` | `pkg/remediationorchestrator/routing/blocking.go` | `BlockingConditionChecker` (routing decision logic) + `EngineConfigProvider` (config/backoff calculations) | 1 implementer (`RoutingEngine`), ~4 consumer call sites in one package, 2 existing hand-written mocks already implementing all methods |
| `ClusterRegistry` | `pkg/fleet/registry/types.go` | `ClusterQuerier` (read-only) + `ClusterRegistryLifecycle` (watcher lifecycle) | 2 implementers (`EAIGWRegistry`, `KuadrantRegistry`); 4 of 5 consumers are query-only, only `cmd/fleetmetadatacache/main.go` needs lifecycle — real evidence the split reflects actual usage |
| `SessionManager` | `internal/kubernautagent/mcp/interfaces.go` | `SessionLifecycle` (takeover/release) + `SessionQuerier` (read-only) | 1 implementer (`LeaseSessionManager`), 3 tool-struct consumers (one, `SelectWorkflowTool`, is query-only today) |

All three follow the `io.ReadWriter`-style composition pattern already used for `AutonomousSessionManager`/`AWXClient` in Phase 5 — the original interface name is preserved as a composition of the two new role interfaces, so every implementer, mock, and call site needed **zero** changes.

**Not fixed, flagged separately**:
- `Client` (`pkg/notification/client.go`) has **zero consumers anywhere in the repo** — `notification.NewClient` is never called (confirmed via repo-wide search); `pkg/remediationorchestrator/creator/notification.go` uses the raw controller-runtime client instead. This is a dead-code question, not an ISP violation (nothing depends on the fat interface), so it is **not** split. Follow-up recommendation: either remove `pkg/notification/client.go` entirely or find the missing wiring that was supposed to use it — outside this audit's scope to decide which.
- `MCPClient` (`pkg/apifrontend/ka/mcp_client.go`) is deferred to Wave 1 (APIFrontend) — 2 implementers are not method-uniform (`PooledMCPClient.Investigate`/`.StartInvestigation` are stubs) and ~10+ consumer call sites each use only one method; splitting the interface itself is mechanically safe, but the real ISP value needs those call sites narrowed too, which is materially more churn than the other three combined.

**Verification (Wave 0 exit gate)**: `go build ./...`, `go vet ./...` (both repo-wide), `golangci-lint run ./...` (0 new issues — the 7 pre-existing `docs/spikes/` findings predate this wave), `gofmt -l`/`goimports -l` clean on every touched file, and `make test` (full unit-test tier across all services) green. Each of the 6 `main()` decompositions was individually gated with `gocyclo`, `go vet`, `golangci-lint`, and its new characterization tests before moving to the next service. Wave 0 is now complete; Waves 1-6 (APIFrontend, RemediationOrchestrator/WorkflowExecution/SignalProcessing, DataStorage, KubernautAgent remainder, Notification/AIAnalysis/EffectivenessMonitor/Gateway batch, final sweep) remain open and each require a fresh preflight + confidence gate before starting, per the Phase 6+ plan.

---

## 7i. Phase 6 Wave 1: APIFrontend Complexity + File-Size Burndown — ✅ RESOLVED

Preflight + a time-boxed spike (per the user-approved "preflight/spike checkpoint before each wave" process) reached 91% confidence before starting; the user explicitly approved proceeding. Scope: the 5 highest-complexity APIFrontend functions left open after Phase 4 (§7f) plus the 3 largest APIFrontend files (`ka_investigate_mcp.go`, `crd_tools.go`, `cmd/apifrontend/main.go`).

### 7i-1. Function decomposition — 5 functions

| Function | File | Cyclomatic before → after | Technique |
|---|---|---|---|
| `buildEventData` | `pkg/apifrontend/audit/store_adapter.go` | 38 → **2** | Registry pattern (`map[EventType]eventDataBuilder`), reusing the same DD-AUDIT-008 approach already applied to KubernautAgent's `buildEventData` in Phase 4 (§7f-2) |
| `RegisterTools` | `pkg/apifrontend/handler/mcp_bridge.go` | 37 → **4** | Decomposed into domain-specific `register<Domain>Tools` helpers (CRD, investigation, KAMCP, DS, interactive, alert) orchestrated by a thin dispatcher; extracted `newToolGate`/`toolGate` and `finalizeSessionPhase` helpers |
| `newPhaseGuard` | `pkg/apifrontend/agent/phase_guard.go` | 43 → **1** | Extracted the `BeforeToolCallback`/`AfterToolCallback` closure bodies into standalone named functions (`phaseGuardBefore`, `phaseGuardAfter`, `driverIsActive`, `injectStoredRRID`, `toolCallSucceeded`, `refreshActiveContext`, `recordDriverEntryState`, `syncActiveContextRegistry`) |
| `handleSubscribe` | `pkg/apifrontend/handler/status_handler.go` | 40 → **7** | Extracted `subscribeStreamState` (mutable loop state) + `subscribeCleanup` (LIFO cleanup-closure list, mimicking `defer` across extracted methods) + `runSubscribeLoop`/`handleRRWatchEvent`/`handleEAWatchEvent`/`bootstrapEAWatch` |
| `HandleInvestigationMCPWithRegistry` | `pkg/apifrontend/tools/ka_investigate_mcp.go` | 67 → **10** | Extracted `resolveInvestigationRR`, `createRRForInvestigation`, `resolveClusterScoped`, `signalInteractiveSession`, `awaitInvestigationReady`, `startKAInvestigation`, `finalizeInvestigationStart`, `runBlockingInvestigation`, `handoffOrCloseSession`, `startNonBlockingBridge` |

`handleSubscribe` was the one target with materially low pre-existing test coverage (SSE streaming handler); per AGENTS.md's coverage-before-refactor mandate, the existing `test/integration/apifrontend/status_subscribe_test.go` suite (12 specs) was run and confirmed passing before and after the decomposition, with a targeted isolated re-run of a `--focus`ed spec (`IT-AF-1460-014`) to rule out a transient 3-spec failure that traced to a pre-existing test-infrastructure flake (namespace-name collisions from `time.Now().UnixNano()%100000` seeding, unrelated to this refactor) — not a regression.

A mid-decomposition `golangci-lint` pass (repo config) surfaced 4 new `revive argument-limit` violations introduced by the extraction itself (helper functions inheriting too many of the original function's locals as parameters): `runSubscribeLoop` (11 args), `handleRRWatchEvent` (10 args), `handleEAWatchEvent` (10 args) in `status_handler.go`, and `runBlockingInvestigation` (8 args) in `ka_investigate_mcp.go`. Fixed with the same Options-pattern used throughout Phase 2 (§7b): a `subscribeLoopCtx` struct grouping the request-scoped SSE dependencies (writer, flusher, request, RR list, object key, logger, cleanup) cut the three `status_handler.go` functions to 5 args each; a `blockingInvestigationParams` struct cut `runBlockingInvestigation` to 3 args. Re-run of `golangci-lint run ./pkg/apifrontend/... ./cmd/apifrontend/...` after the fix: 0 issues.

### 7i-2. File splits — 3 files

| File | Lines before | Split into | Lines after (max) |
|---|---|---|---|
| `pkg/apifrontend/tools/ka_investigate_mcp.go` | 1,111 | `ka_investigate_mcp.go` (core handler + helpers, 685), `ka_investigate_bridge.go` (event bridging/RCA emission, 388), `ka_investigate_registry.go` (`MonitorRegistry`, 84) | 685 |
| `pkg/apifrontend/tools/crd_tools.go` | 1,144 | `crd_tools.go` (list/get/approve/cancel CRD tools, 606), `crd_tools_watch.go` (`kubernaut_watch` + `watchLoopState`, 355), `crd_tools_session.go` (`kubernaut_await_session` + `AwaitISPhaseActive`, 215) | 606 |
| `cmd/apifrontend/main.go` | 1,701 | `main.go` (bootstrap/lifecycle: `main`/`run`/router+server construction/shutdown, 609), `backend_deps.go` (backend client wiring + LLM-triager routing + resilient-transport helper, 493), `auth_wiring.go` (JWT/replay-cache/OIDC wiring, 259), `session_infra.go` (session controller manager + preflight RBAC/CRD checks, 253), `mcp_a2a_handlers.go` (MCP/A2A handler construction, 165) | 609 |

All three splits are pure structural moves within the same package (`tools`/`main`) — zero behavior change, zero exported-API change, no test files touched. `goimports -w` regenerated each new file's import block mechanically after the move (avoiding manual import-list transcription errors); `gofmt -l` confirmed no formatting drift beyond pre-existing, untouched files.

**Verification (Wave 1 exit gate)**: `go build ./...`, `go vet ./...` (both repo-wide, clean), `golangci-lint run ./pkg/apifrontend/... ./cmd/apifrontend/...` (0 issues after the argument-limit fix above), and the full `pkg/apifrontend/...` + `cmd/apifrontend/...` unit-test tiers (all packages green, including the 88s `tools` suite and the 4.9s `cmd/apifrontend` suite) all pass. `gocyclo -over 5` re-scan of every touched file confirms no function above cyclomatic 41 remains (the new high-water mark, `run()` in `main.go`, is itself the pre-existing Phase 4-decomposed value — untouched by Wave 1). Wave 1 is now complete; Waves 2-6 (RemediationOrchestrator/WorkflowExecution/SignalProcessing, DataStorage, KubernautAgent remainder, Notification/AIAnalysis/EffectivenessMonitor/Gateway batch, final sweep) remain open per the Phase 6+ plan.

---

## 7j. Phase 6 Wave 2: RemediationOrchestrator/WorkflowExecution/SignalProcessing Complexity + File-Size Burndown — ✅ RESOLVED

Preflight + a time-boxed spike (re-scanning `gocyclo`/`gocognit` against current `main`, since the Phase 3a/7d file split had already changed line numbers referenced by the original audit) reached >90% confidence before starting; the user explicitly approved proceeding, with an explicit directive to close the pre-existing unit-test coverage gap on WorkflowExecution's `reconcilePending` orchestration logic (cooldown enforcement, audit idempotency, collision handling) with characterization tests tied to FedRAMP/SOC2 control objectives (SC-5 DoS protection via cooldown, AU-2/AU-3 audit content, CC8.1 audit completeness via idempotency) before decomposing it. Scope: the highest-complexity `Reconcile`-family functions and the largest files remaining in the three P0 CRD controllers.

### 7j-1. WorkflowExecution: `reconcilePending` decomposition (Wave 2c)

New Ginkgo characterization specs were added first (targeting the orchestration branches — cooldown block, audit-idempotency skip, collision handling — that had no direct unit coverage despite the black-box `Reconcile()` suites in `pkg/workflowexecution/...`), confirmed green against the pre-refactor implementation, then kept green through the decomposition below.

| Function | Cyclomatic before → after | Technique |
|---|---|---|
| `reconcilePending` | 49 → **7** | Extracted into 7 named steps, each independently testable: `refetchFreshPendingWFE` (5), `validateAndAnnouncePendingSpec` (3), `resolvePendingSchemaAndEngine` (13), `checkPendingCooldownOrBlock` (6), `recordPendingSelectionAudit` (13), `createPendingExecutionResource` (10), `finalizePendingToRunning` (5) |

A behavior-preservation bug was caught during extraction: `checkPendingCooldownOrBlock`'s original signature (`(ctrl.Result, bool)`) silently dropped a `TransitionTo`/`Status().Update()` error instead of propagating it. Fixed by adding the missing `error` return before merging. A `golangci-lint` variable-shadowing warning on the call site (`result`/`err` re-declared per branch) was fixed by renaming each branch's locals (`schemaResult`, `cooldownResult`, `auditResult`, `createResourceResult`).

**File split**: `workflowexecution_controller.go` (1,979 lines post-decomposition) → `workflowexecution_controller.go` (526, core dispatcher + `NewReconciler` + `ValidateSpec`), `workflowexecution_pending.go` (477, `reconcilePending` + helpers), `workflowexecution_status_marking.go` (487, `MarkCompleted`/`MarkFailed`/status helpers), `workflowexecution_lifecycle.go` (379, `reconcileRunning`/`ReconcileTerminal`/`ReconcileDelete`), `workflowexecution_collision.go` (193, `HandleAlreadyExists`/`FindWFEForOwnedResource`), `workflowexecution_catalog.go` (111, catalog/engine resolution).

### 7j-2. SignalProcessing: `Reconcile`/`reconcileEnriching`/`reconcileClassifying` decomposition (Wave 2b)

| Function | Cyclomatic before → after | Technique |
|---|---|---|
| `Reconcile` | 23 → **15** | Extracted `initializeNewSignalProcessing` (3) and `dispatchPhase` (5); further extracted the duplicate-generation guard into `isDuplicateGenerationReconcile` (4) to bring the dispatcher itself under the threshold after the first pass left it at 18 |
| `reconcileEnriching` | 38 → **3** | Extracted `checkEnrichingIdempotencyGuard`, `warnIfUnsupportedTargetType`, `performK8sEnrichment`, `applyEnrichmentCustomLabels`, `buildEnrichmentMessage`, `finalizeEnrichment` |
| `reconcileClassifying` | 36 → **8** | Extracted `checkClassifyingIdempotencyGuard`, `refreshClassifyingContext`, `failClassifyingPhase`, `evaluateSeverityOrFail`, `resolveSignalMode`, `buildClassificationMessage`, `finalizeClassification` |

**File split**: `signalprocessing_controller.go` (1,336 lines post-decomposition) → `signalprocessing_controller.go` (427, core dispatcher + `Reconcile`/`reconcilePending`/backoff helpers), `signalprocessing_enriching.go` (319), `signalprocessing_classifying.go` (373), `signalprocessing_categorizing.go` (226, unchanged `reconcileCategorizing`), `signalprocessing_audit.go` (68, audit-emission helpers).

### 7j-3. RemediationOrchestrator: constructor/dispatcher decomposition + 2 file splits (Wave 2a)

`NewReconciler`'s inflated cyclomatic score (inherited from inline `AnalyzingCallbacks`/`AwaitingApprovalCallbacks` closures) and `Reconcile`'s pending-phase bootstrap and terminal-phase housekeeping were extracted into named methods:

| Function | Cyclomatic before → after | Technique |
|---|---|---|
| `NewReconciler` | closures inflated the constructor's score | Extracted `applyTimeoutDefaults`, `newDefaultRoutingEngine`, `recordEvent`, and the RAR/lock/hash callback closures into standalone `(r *Reconciler)` methods |
| `Reconcile` | — → **14** | Extracted `shouldSkipPendingReconcile` (7), `initializeNewRemediationRequest` (7), `handleTerminalPhaseHousekeeping` (6) |

**File splits** (both discovered post-decomposition, once `reconciler.go` and `terminal_transitions.go` re-crossed the 700-line threshold after the Phase 3a/7d split had already reduced them once):

- `reconciler.go` (1,148 lines) → `reconciler.go` (381, struct/`NewReconciler`/timeout+routing-engine construction), `reconciler_callbacks.go` (254, event/lock/RAR/hash callback helpers + config accessors), `reconcile_loop.go` (556, `Reconcile`/pending-bootstrap/terminal-housekeeping/`SetupWithManager`/field indexes).
- `terminal_transitions.go` (1,032 lines) → `terminal_transitions.go` (565, inherited-completion/failure transitions, `transitionPhase`, `transitionToVerifying`/`transitionToFailed`), `blocked_transitions.go` (235, `handleBlocked` + escalation/notification/metrics helpers), `timeout_handling.go` (312, `handleGlobalTimeout` + `createEffectivenessAssessmentIfNeeded`).

An orphaned doc comment block (a stale `populateTimeoutDefaults` docstring left behind from an earlier refactor that had already moved the function to `timeout_management.go`, with no attached code) was deleted as part of the `reconciler.go` split — a zero-behavior-change cleanup, confirmed by `grep` showing the real implementation already lived elsewhere.

**Residual RO offenders (not in Wave 2 scope, carried to a future wave)**: `ApplyTransition` (19), `createEffectivenessAssessmentIfNeeded` (16), `transitionPhase` (16), `RARReconciler.Reconcile` (15) remain mildly over the cyclomatic-15 convention threshold; none were part of the `Reconcile`-family decomposition targeted this wave. Similarly in WorkflowExecution: `MarkFailed` (22) and `mapTektonReasonToFailureReason` (18) remain open. These are tracked for Wave 3+.

### 7j-4. Verification (Wave 2 exit gate)

`go build ./...` and `go vet ./...` (both repo-wide) clean. `golangci-lint run ./...` (repo-wide): 0 new issues — the 7 pre-existing `docs/spikes/` findings (4 `errcheck`, 1 `gosec`, 2 `unused`) predate this wave and are outside its scope. `gofmt -l`/`go vet` clean on every touched file. Unit tests: `internal/controller/{remediationorchestrator,signalprocessing,workflowexecution}/...` and their `pkg/...` counterparts all green, plus a full `make test` repo-wide run (one unrelated, timing-sensitive flake in `pkg/apifrontend/launcher` — `UT-AF-1446-002` — reproduced as green in isolation and on a second full-suite run, attributed to host CPU contention flagged mid-session, not a regression from this wave; `test-unit-spike-mcp-stream` fails on a pre-existing empty-test-suite gap in `cmd/spike-mcp-stream`, last touched May 29 2026, unrelated to this wave). Integration tests: `make test-integration-signalprocessing` (84+3 specs, 0 failed) and `make test-integration-workflowexecution` (104 specs, 0 failed) both pass, confirming the file splits and behavior-preservation fixes (cooldown error propagation, variable-shadowing renames) did not regress wiring. All touched files in all three services are now under the 700-line convention threshold. Wave 2 is now complete; Waves 3-6 (DataStorage, KubernautAgent remainder, Notification/AIAnalysis/EffectivenessMonitor/Gateway batch, final sweep) remain open per the Phase 6+ plan.

---

## 8. Variable Shadowing — 120 found, mostly low-risk

114/120 are `err` shadowing (`if err := f(); err != nil` repeated in the same scope — the single most common and least dangerous shadow pattern in Go, which is exactly why `govet -shadow` isn't part of default `go vet`). 1 `ctx` shadow, 1 `result`, 1 `username`, 1 `ok`, 1 `isString`. **No goroutine-closure-captures-loop-variable pattern was found** (the genuinely dangerous shadow bug) — none of the 120 hits are in a `for ... go func()` or `for ... defer func()` body.

**Recommendation**: low priority. Worth a mechanical rename pass only if the team wants `go vet -shadow` enabled permanently in CI; otherwise not worth the diff churn on its own.

**✅ Annotated**: rather than 114 scattered inline suppressions, `.golangci.yml` gained one `issues.exclusions.rules` entry excluding the `govet` `shadow: declaration of "(err|ok)" shadows declaration` message text. `govet`'s `shadow` analyzer is not in `settings.govet.enable` today, so this rule is inert until someone opts in — at which point the reviewed, idiomatic `err`/`ok` re-declarations are pre-excluded while shadowing of any other identifier still fails the build.

**Follow-up (not yet committed)**: a mechanical rename pass for the 6 non-`err`/`ok` outliers (`ctx`, 2×`result`, `username`, `isString`, plus a related `ctx`-as-struct-field cleanup in `cmd/kubernautagent/main.go`'s `mcpHandlerParams`), together with actually enabling `govet.shadow`/`prealloc`/`revive`/`containedctx` in `.golangci.yml` (vs. today's "pre-declared, inert" state), was prototyped and verified clean (`golangci-lint run ./...` 0 new issues) but deliberately held back from this commit pending a separate decision on enabling new repo-wide lint gates.

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

| Phase | Scope | Risk | Est. effort | Status |
|---|---|---|---|---|
| 1 | Mechanical, zero-behavior-change: `prealloc` (13), safe `err`-shadow renames if desired | Very low | 0.5 day | ✅ Done |
| 2 | Options-pattern extraction for all 21 real 8+-param functions (corrected scope, see §7b) | Low-medium (touches call sites, needs UT re-run) | 2-3 days | ✅ Done |
| 2.5 | Coverage gate before Phase 3: audit-emission gap closure on `reconciler.go` (see §7c) | Low (additive tests only) | 0.5 day | ✅ Done |
| 3a | Split `reconciler.go` (3,435→1,023 across 7 files, see §7d) into cohesive files following the existing `*_handler.go` pattern already used in RemediationOrchestrator | Medium (large diff, no logic change) | 3-4 days | ✅ Done |
| 3b | Split `pkg/gateway/server.go` (2,552→633 across 6 files, see §7e) into cohesive files (HTTP plumbing/construction/audit emission, mirroring the `processing/`/`adapters/`/`k8s/`/`middleware/` subpackage split already in Gateway) | Medium (large diff, no logic change) | 1-2 days | ✅ Done |
| 4 | Decompose the top complexity offenders: `buildEventData` (88), `HandleWatch` (cognitive 133), `Config.Validate` ×2, `main()` in `cmd/kubernautagent` and `cmd/apifrontend` | Medium-high (behavior-preserving refactor of dense logic, needs careful UT coverage first) | 5-7 days | ✅ Done (see §7f) |
| 5 | Interface segregation for `AutonomousSessionManager` (13 methods) and `AWXClient` (11 methods) | Medium (ripples to all implementers/mocks) | 2-3 days | ✅ Done (see §7g) |
| 6 (Wave 0) | `cmd/*/main.go` decomposition for the 6 remaining services + ISP split for `Engine`/`ClusterRegistry`/`SessionManager` (see §7h) | Low | 3-4 days | ✅ Done (see §7h) |
| 6 (Wave 1) | APIFrontend: 5 highest-complexity functions + 3 largest files (see §7i) | Medium | 3-4 days | ✅ Done (see §7i) |
| 6 (Wave 2) | RemediationOrchestrator/WorkflowExecution/SignalProcessing: `Reconcile`-family decomposition (`reconcilePending`, `reconcileEnriching`, `reconcileClassifying`, `Reconcile` ×2, `NewReconciler`) + 4 oversized files split into 13 (see §7j) | Medium-High | 3-4 days | ✅ Done (see §7j) |
| 6 (Wave 3-6) | DataStorage, KubernautAgent remainder, Notification/AIAnalysis/EffectivenessMonitor/Gateway batch, final sweep — remaining complexity (~130 functions, including RO/WE residuals noted in §7j-3) + oversized files (~27) burndown | Medium-High (per-wave) | ~15-20 days remaining | 🔲 Not started — tracked in the "Phase 6+ Complexity and File-Size Burndown" plan, each wave requires its own preflight + confidence gate |

**Not recommended for action**: DTO/data-model "god structs" (category 4a), `context.Context` struct fields (both documented), `any`/`interface{}` usage (spot-checked — the ~849 raw hits are overwhelmingly idiomatic JSON-decoding, `sync.Map`/`singleflight` third-party API signatures, and generic-JSON-passthrough code; no material violations found beyond one worth a look: `BuildTriagePrompt(input TriageInput, rules interface{})` in `pkg/apifrontend/severity/types.go:113`).

---

## Raw Data

Reproducible via the commands in Methodology; intermediate files used to build this report (not committed, regenerate as needed):
`/tmp/gocyclo_report.txt`, `/tmp/audit_lint_full.txt`, `/tmp/audit_lint2.txt`, `/tmp/audit_shadow.txt`.
