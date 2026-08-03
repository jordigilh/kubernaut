# Test Plan: fullpipeline E2E mock-llm fixture support for combined remediate+investigate intent (all 3 AF modes)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1853-v1
**Feature**: Add mock-llm keyword scenarios (and the N-deep `NextToolCall` chaining + template-resolution fix they depend on) so a single free-form A2A `message/stream` request combining "create a remediation + investigate it" intent works in the `fullpipeline` E2E environment — for **all three** AF operational modes (autonomous, interactive, full-interactive-remediation), not just the one literally reported in the issue
**Version**: 2.0
**Created**: 2026-08-02
**Author**: AI agent (Cursor)
**Status**: Draft
**Branch**: `main` (to be created: `fix/1853-fp-mock-llm-combined-investigate`)

---

## 1. Introduction

### 1.1 Purpose

Issue [#1853](https://github.com/jordigilh/kubernaut/issues/1853) reports that a single chat
message combining "remediate + investigate" intent fails every time against the
`fullpipeline` E2E cluster. The root cause is confirmed to live entirely in test
infrastructure (`test/infrastructure/shared_e2e.go`, `test/services/mock-llm/`), not in AF
production code: the mock-llm's `NextToolCall` chaining mechanism (a) capped chains at exactly
one link, and (b) never resolved `$from_tool:<tool>:<field>` templates inside a chained call's
own arguments (only the primary `tool_call`'s arguments were resolved).

While triaging, we confirmed against the authoritative production system prompt
(`pkg/apifrontend/agent/prompt.txt`, cross-checked with `hindsight-docs`) that AF actually
supports **three** distinct operational modes for combined "investigate + remediate" intent,
not one:

1. **Autonomous** — `kubernaut_remediate` alone. Fire-and-forget; no RCA is streamed to the
   user, no pause.
2. **Interactive** — `kubernaut_investigate`, which produces RCA/early findings, then
   **pauses** for the user to manually pick a workflow (`discover_workflows` →
   `select_workflow` → `watch` happen in later turns, driven by the user).
3. **Full Interactive Remediation** ("autonomous-interactive") — `kubernaut_investigate`
   followed by an **auto-selected**, highest-confidence workflow, streaming full RCA
   transparency but with **no pause** for manual selection — all the way through
   `discover_workflows` → `select_workflow` → `watch` in one turn.

The issue as originally filed only reproduced mode 2's single-message chaining defect
(2 tool calls: `remediate` → `investigate`). Mode 3 hits the *same* root-cause defect at a
worse depth (4 tool calls: `investigate` → `discover_workflows` → `select_workflow` → `watch`),
and was previously **untested and undocumented** for kubernaut-console — see
[#1855](https://github.com/jordigilh/kubernaut/issues/1855) (documentation gap, tracked
separately per user direction; this plan closes the *test coverage* gap for all 3 modes so the
console team has a working example + expectations for each, while #1855 will close the
*documentation* gap in a follow-up PR).

This plan therefore: (a) generalizes mock-llm's `NextToolCall` from a single link to an
arbitrary-depth linked list with `$from_tool` resolution at every link, and (b) adds one E2E
scenario per combined-intent mode that actually needs multi-turn-in-one-message chaining
(modes 2 and 3 — mode 1 is a single tool call and already works today, see §4.2).

### 1.2 Objectives

1. **O1 (mode 2 — the originally-reported case)**: A single `message/stream` request
   combining "create a remediation" + "investigate" intent triggers `kubernaut_remediate`
   then `kubernaut_investigate` in sequence, with a real, server-generated `rr_id` correctly
   propagated between the two — not a literal unresolved template placeholder. The turn stops
   at RCA (no auto-selection), matching Interactive Mode's documented contract.
2. **O2 (mode 3 — new scope)**: A single `message/stream` request combining "investigate" +
   "fix"/"remediate" intent triggers `kubernaut_investigate` → `kubernaut_discover_workflows` →
   `kubernaut_select_workflow` (auto-picking the highest-confidence recommended workflow) →
   `kubernaut_watch`, all in one turn, with `rr_id` correctly propagated to every link —
   proving the chaining mechanism generalizes past 2 links.
3. **O3**: `NextToolCall` (mock-llm scenario config) supports chains of arbitrary depth, and
   `$from_tool:<tool>:<field>` templates resolve correctly at every link in the chain, not just
   the first — closing the asymmetry that caused O1 to fail and generalizing it for O2.
4. **O4**: The new scenarios do not change the behavior of any existing FP suite test
   (no keyword collision / scenario hijack), and do not introduce cross-request state leakage
   in the now-shared `NextToolCall` chain nodes.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| New unit tests pass | 100% | `go test ./test/services/mock-llm/... -ginkgo.focus="UT-ML-1853"` |
| New E2E tests pass | 2/2 | `ginkgo -v ./test/e2e/fullpipeline/... --label-filter=issue-1853` |
| Existing FP suite regressions | 0 | Full `fullpipeline` E2E run unaffected (spot-checked: E2E-FP-1189-003/-004/-005, E2E-AF-1409-001) |
| Existing mock-llm unit suite regressions | 0 | `go test ./test/services/mock-llm/...` |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue [#1853](https://github.com/jordigilh/kubernaut/issues/1853): fullpipeline E2E mock-llm fixture has no scenario for single-message combined remediate+investigate intent
- Issue [#1855](https://github.com/jordigilh/kubernaut/issues/1855): documentation gap for the 3 AF modes (out of scope here — tracked separately per user direction; this plan only closes the *test* gap)
- Issue #1818 (referenced): the AF production fix this issue was validated against (unrelated code path, confirmed still correct)
- `pkg/apifrontend/agent/prompt.txt`: **authoritative** production system prompt defining the 3 AF operational modes (autonomous / interactive / full-interactive-remediation) — the source of truth for §1.1's mode descriptions and for mode 3's auto-select-highest-confidence-workflow behavior

### 2.2 Cross-References

- [Wiring Verification](../../../.cursor/rules/10-wiring-verification.mdc)
- `test/infrastructure/shared_e2e.go` — `afKeywordYAML`, `DeployMockLLMInNamespace`
- `test/services/mock-llm/handlers/{gemini,openai}.go` — template resolution + NextToolCall dispatch
- Prior art in the same file: `fleetClusterIDScenarioYAML` (#1409), `af_investigate` (#1189), `kaToolE2EConfidence` doc comment (E2E-FLEET-017 scenario-hijack fix, same defect class as this issue)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | New keyword scenarios' confidence ties (1.0) with `af_investigate` and get shadowed by it (both new keywords contain the substring "investigate") | New scenarios never fire; regression to current broken behavior | Medium (this is exactly how #1853 happens today) | E2E-FP-1853-001, -002 | Register both new scenarios textually **before** `af_investigate` in `afKeywordYAML` (Registry.Detect keeps the first-registered match on confidence ties — confirmed by code reading) |
| R2 | New scenarios' keywords accidentally match an existing test's message, hijacking it (same defect class as the E2E-FLEET-017 scenario-hijack fix and #1795) | Existing FP tests (E2E-FP-1189-003/-004/-005) break | Low | All existing `fp a2a interactive` specs | Keywords are the literal phrases `"create and investigate remediation"` (mode 2) and `"investigate and fix remediation"` (mode 3), distinct from every other FP suite chat message (grep-confirmed) |
| R3 | `NextToolCall` is a pointer field aliased across concurrent requests to the same scenario singleton; naively mutating a chain node's `Arguments` in place (at any depth) would leak resolved values into other requests | Flaky/incorrect cross-request state (data race) | Medium if not handled | UT-ML-1853-004 | `resolveTemplateArgsMap` (shared helper, `handlers/chain.go`) allocates a **new** map with the resolved args instead of mutating the shared one in place, at every link in the chain — mirrors the existing `cloneAnyMap` rationale already documented for `ToolCallArgs` |
| R4 | Issue's own "suggested fix" (hardcode a shared `rr_id` on both scripted calls via `MultiToolCalls`) turns out to be inconsistent with `HandleRemediate`'s real semantics: supplying `rr_id` on `kubernaut_remediate` makes AF treat it as a dedup **lookup**, not a create — no RR would ever be created | Suggested fix would silently no-op the RR creation, defeating the test's purpose | Confirmed (code-read, not just theoretical) | N/A — design deviation, see §4.3 | Use `tool_call` + `next_tool_call` (sequential chaining) instead of `MultiToolCalls` (same-turn parallel dispatch), propagating the *real* server-generated `rr_id` via `$from_tool`, fixed by R3's mitigation |
| R5 | Mode 3's 4-deep chain (`investigate` → `discover_workflows` → `select_workflow` → `watch`) was the original 2-link-only `NextToolCall` design's exact breaking point — a shallow fix (e.g. hardcoding a 2nd optional field) would silently cap at depth 2 and fail mode 3 without any compile error | Mode 3 scenario silently truncates after `discover_workflows`, masking a regression | Medium (this is precisely why mode 3 was chosen as the generalization proof, not just mode 2 again) | UT-ML-1853-002/003, E2E-FP-1853-002 | `NextToolCall`/`ToolCallOverride` made self-referential (linked list); `flattenToolCallChain`/`nextChainCallByCount` (`handlers/chain.go`) walk the *entire* chain by count of prior tool responses, with no depth cap |
| R6 | `$from_tool:kubernaut_investigate:rr_id` must resolve correctly across 3 hops (into `discover_workflows`, `select_workflow`, *and* `watch`'s `name` argument), and `select_workflow`'s `workflow_id` cannot be resolved via `$from_tool` at all (the recommended workflow ID is nested inside `discover_workflows`' response under `recommended.workflow_id`, which the existing `$from_tool:<tool>:<field>` grammar cannot address) | Mode 3 scenario emits an invalid/empty `workflow_id`, causing `kubernaut_select_workflow` to fail | Medium | E2E-FP-1853-002 | `workflow_id` is hardcoded in the scenario YAML to the real seeded catalog UUID (`afSelectWorkflowID`, resolved via the pre-existing `resolveWorkflowUUID` helper — same approach the manual-select `af_select_workflow` scenario already uses), simulating the LLM "reading" the recommended workflow from context the same way a real LLM would |
| R7 | (Found live on the shared `fullpipeline-e2e` cluster while validating the above, not part of the original bug report) `af_investigate`'s `rr_id: "$from_tool:kubernaut_remediate:rr_id"` assumes a prior `kubernaut_remediate` call always exists in the session. When "investigate" is sent as the *first* message (no prior remediate — a legitimate standalone use case AF's real `kubernaut_investigate` tool already supports via `namespace`/`kind`/`name`), the placeholder has nothing to resolve against and is sent to AF **verbatim as a literal string**, which AF correctly rejects (`invalid rr_id: invalid resource name "$from_tool:kubernaut_remediate:rr_id"` — confirmed in `apifrontend` pod logs on the shared cluster) | Standalone "investigate" (no prior remediate in-session) hard-fails every time — this was actively blocking the console team's Playwright runs on the shared cluster | Confirmed (reproduced live, not just theoretical) | UT-ML-1853-005/006 | Added `fallback_arguments` to the mock-LLM tool-call schema (`ToolCallOverride`/`MultiToolCallEntry`/`MockScenarioConfig`): `resolveTemplateArgsMap` now swaps in a full fallback argument set (namespace/kind/name) whenever **any** `$from_tool` placeholder fails to resolve, instead of returning the half-resolved map with the literal placeholder string still in it. Mirrors `kubernaut_investigate`'s real, mutually-exclusive `rr_id`-vs-`namespace/kind/name` argument contract (confirmed via `pkg/apifrontend/tools/ka_investigate_mcp.go`) |

### 3.1 Risk-to-Test Traceability

R1 and R2 are both proven by E2E-FP-1853-001/-002 passing while the full existing FP label set (spot-checked) keeps passing unmodified. R3 is proven directly by UT-ML-1853-004 asserting a resolved value on one request does not leak into a sibling/repeated request. R5 is proven directly by UT-ML-1853-002/003 (Gemini/OpenAI) asserting a chain link *beyond* the historical depth-2 cap fires correctly, and end-to-end by E2E-FP-1853-002 reaching `kubernaut_watch` (link 4). R6 is proven by E2E-FP-1853-002 asserting the `WorkflowExecution` actually completes (i.e. `select_workflow` received a valid, real `workflow_id`). R7 is proven by UT-ML-1853-005 (YAML parsing of `fallback_arguments`), UT-ML-1853-006 (Gemini + OpenAI: fallback fires when unresolved, does NOT fire when resolution succeeds — no regression to existing multi-turn flows), and live-validated directly against the shared cluster: a standalone "start investigation" A2A request (no prior remediate in-session) was sent via `curl` after redeploying the fix, producing a real new `RemediationRequest`/`InvestigationSession` pair targeting `kubernaut-system/Deployment/memory-eater` with zero `invalid rr_id` errors in the `apifrontend` logs.

### 3.2 Live-Cluster Validation Evidence (Mode 2 / Mode 3, R1/R5/R6)

Before committing, mode 2 and mode 3 were validated by direct reproduction against the shared `fullpipeline-e2e` cluster instead of a full Ginkgo run, to avoid an unrelated ~20-30min unconditional rebuild of every FP service image (the harness's `BuildImageForKind` always rebuilds with `--no-cache` and a fresh random tag per invocation — there is no content-hash cache-hit path — so a full suite run would have rebuilt ~10+ unchanged service images just to exercise 2 specs). Since the already-deployed `mock-llm` binary already contained the full N-deep-chaining + `fallback_arguments` fix (built and loaded earlier for R7), the two new scenarios were injected directly into the live `mock-llm-scenarios` ConfigMap (byte-identical logical content to `combinedRemediateInvestigateScenarioYAML`/`fullInteractiveRemediationScenarioYAML` in `test/infrastructure/shared_e2e.go`, using fixed namespace names instead of test-generated hashes), and exercised via real `curl` A2A requests against the live `apifrontend` service — no code or image changes, only a scenario-config swap.

**Result — Mode 2** (`create and investigate remediation`): `kubernaut_remediate` fired, then `kubernaut_investigate` fired with the real server-generated `rr_id` correctly propagated via `$from_tool`. A real `RemediationRequest` and `InvestigationSession` were created; the turn stopped at RCA findings (`phase: Investigating`) with no workflow auto-selected — matching Mode 2's contract exactly.

**Result — Mode 3** (`investigate and fix remediation`): the full 4-deep chain fired — `kubernaut_investigate` → `kubernaut_discover_workflows` → `kubernaut_select_workflow` (auto-selected `oomkill-increase-memory-v1`, 95% confidence, no pause) → `kubernaut_watch`. The `RemediationRequest`, `InvestigationSession`, and `WorkflowExecution` all reached `Completed`/`Remediated`, and the target `memory-eater` `Deployment`'s memory limit was actually patched from `50Mi` to `512Mi` by the real remediation workflow (pod went from `OOMKilled` to `Running`) — proof the chain's `rr_id` propagation was correct at every one of the 3 downstream hops, not just superficially "not erroring."

**A footgun found and fixed during this exercise (not a defect in the committed fix)**: the first manual ConfigMap injection nested `next_tool_call:` one level too deep — as a child of `tool_call:` (populating the unused `ToolCallOverride.NextToolCall` field) instead of as `tool_call:`'s YAML *sibling* at the `KeywordScenarioOverride` level (populating `KeywordScenarioOverride.NextToolCall`, which is what `registry_default.go`'s `convertToolCallChain(ks.NextToolCall)` actually reads). This silently produced `cfg.NextToolCall == nil`, so the chain never fired and AF's ADK loop re-invoked the LLM 3 times before giving up, falling through to the fully-autonomous RR pipeline instead. Re-verifying `combinedRemediateInvestigateScenarioYAML`/`fullInteractiveRemediationScenarioYAML`'s actual indentation in `test/infrastructure/shared_e2e.go` confirmed **both already use the correct sibling-level nesting** — this was a hand-transcription mistake made while replicating the scenario into the live ConfigMap by hand, not a defect in the committed Go source. Fixing the indentation in the manual ConfigMap copy (to match the Go source exactly) immediately produced the correct behavior described above. No code changes resulted from this finding; it is recorded here as evidence that the sibling-vs-nested distinction is easy to get wrong by hand and worth calling out for anyone else hand-authoring `keyword_scenarios` YAML.

All scratch resources (2 temporary namespaces, their `memory-eater` fixtures, and the temporary ConfigMap scenario entries) were deleted after validation; the live ConfigMap was restored to contain only the R7 `fallback_arguments` fix (re-confirmed working via a final standalone-investigate smoke test after the restore).

---

## 4. Scope

### 4.1 Features to be Tested

- **Mock-llm N-deep `NextToolCall` chaining + template resolution** (`test/services/mock-llm/scenarios/types.go`, `config/overrides.go`, `registry_default.go`, `handlers/{gemini,openai,chain}.go`): `NextToolCall`/`ToolCallOverride` generalized from a single optional link to a self-referential linked list of arbitrary depth; `$from_tool:<tool>:<field>` placeholders resolve correctly at **every** link (previously only the primary `tool_call`'s arguments were resolved — the actual root cause).
- **New `af_remediate_investigate_combined_1853` keyword scenario** (mode 2, `test/infrastructure/shared_e2e.go`): a single combined-intent message scripts `kubernaut_remediate` → `kubernaut_investigate` sequentially with the real `rr_id` propagated, then stops (no auto-select), per Interactive Mode's contract.
- **New `af_full_interactive_remediation_1853` keyword scenario** (mode 3, `test/infrastructure/shared_e2e.go`): a single combined-intent message scripts `kubernaut_investigate` → `kubernaut_discover_workflows` → `kubernaut_select_workflow` → `kubernaut_watch` (4-deep chain) with `rr_id` propagated to all 3 dependent links and the highest-confidence `workflow_id` auto-selected, per Full Interactive Remediation Mode's contract.
- **E2E-FP-1853-001/-002**: prove both scenarios above are reachable end-to-end via AF's real binary, real ADK tool-calling loop, and the real FP mock-llm ConfigMap (not just unit-level).
- **`fallback_arguments` for unresolvable `$from_tool` references** (R7, `test/services/mock-llm/{config/overrides,scenarios/types,scenarios/registry_default,handlers/chain,handlers/gemini,handlers/openai}.go`): when a `$from_tool:<tool>:<field>` placeholder cannot resolve (the referenced tool was never called earlier in the session), the entire argument set now falls back to a scenario-provided literal set instead of sending the half-resolved map with the literal placeholder string in it. Applied to the existing `af_investigate` scenario (`test/infrastructure/shared_e2e.go`) so a standalone "investigate" with no prior "remediate" in-session creates a new RR from `namespace/kind/name` instead of hard-failing.

### 4.2 Features Not to be Tested

- AF production code paths for `kubernaut_remediate`/`kubernaut_investigate`/`kubernaut_discover_workflows`/`kubernaut_select_workflow`/`kubernaut_watch` themselves — already proven working individually by existing E2E-FP-1189-003/-004/-005 and E2E-AF-1409-001 (this issue is purely a test-fixture gap in *chaining* them within one message, confirmed via `kubectl logs deploy/apifrontend` in the original issue report showing no `kubernaut_remediate` call was ever attempted).
- **Mode 1 (autonomous)**: not affected by this issue at all — a single `kubernaut_remediate` call has no `NextToolCall` chain to break. Already covered by the existing `"autonomous remediation"` keyword scenario / E2E-FP-1189 coverage; no new test needed.
- Console-side rendering of the resulting `investigation_summary` — out of scope for kubernaut (tracked in kubernaut-console#39).
- Formal documentation of the 3-mode contract itself — tracked separately in [#1855](https://github.com/jordigilh/kubernaut/issues/1855) per user direction (this plan closes the test-coverage gap only, not the docs gap).

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use `tool_call` + `next_tool_call` (sequential) instead of the issue's literally-suggested `MultiToolCalls` + hardcoded shared `rr_id` | Confirmed via `pkg/apifrontend/tools/ka_remediate.go` (`HandleRemediate`) that supplying a caller-chosen `rr_id` to `kubernaut_remediate` makes AF perform a dedup **lookup** (`Get`) instead of creating a new RR — if the fixed value doesn't already exist, `HandleRemediate` returns `AlreadyExists: false` and creates nothing. `MultiToolCalls` also dispatches both tool calls in the same LLM turn (parallel, before either result is known), so there is no way to know the real `rr_id` in advance anyway. Sequential `next_tool_call` chaining (already an established pattern — `fleetClusterIDScenarioYAML`, #1409) is the only mechanism that can correctly propagate AF's real, server-generated `rr_id`. |
| Generalize `NextToolCall`/`ToolCallOverride` to a self-referential linked list (arbitrary depth) rather than adding a 2nd hardcoded `NextToolCall2` field | Mode 3 needs 4 sequential tool calls, not 2. A depth-capped field-based approach would need a new named field per additional depth level and would resurface the identical defect class at the next mode/use case that needs depth 5+. A linked list (`NextToolCall *MultiToolCallEntry`, mirrored in the YAML-facing `ToolCallOverride`) supports arbitrary depth with one recursive conversion function (`convertToolCallChain`) and one shared chain-walking helper (`flattenToolCallChain` + `nextChainCallByCount`) reused by both handlers. **User-approved architectural change.** |
| Mode 3 uses `kubernaut_investigate` directly from `namespace`/`kind`/`name` (not a prior `kubernaut_remediate` call) to create the RR+IS | Confirmed via `pkg/apifrontend/tools/ka_investigate_mcp.go` (`InvestigateMCPArgs`/`InvestigateMCPResult`) that `kubernaut_investigate` already supports creating a brand-new RR directly from resource identity args and returns the new `rr_id` in its result — matching the real Full Interactive Remediation Mode contract in `prompt.txt`, which does not require a separate remediate step first. |
| Mode 3's `select_workflow.workflow_id` is a hardcoded real catalog UUID (`afSelectWorkflowID`, via the pre-existing `resolveWorkflowUUID` helper), not a `$from_tool` reference | `$from_tool:<tool>:<field>` can only address a *top-level* field of a prior tool's JSON result. The recommended workflow ID lives nested under `discover_workflows`' response (`recommended.workflow_id`), which the grammar cannot reach. The existing manual-select `af_select_workflow` scenario already solves this identically by hardcoding the same seeded UUID — mode 3 reuses that precedent rather than inventing a new mechanism. |
| Fix the `NextToolCall.Arguments` template gap in **both** `gemini.go` and `openai.go`, even though FP's Helm values pin `provider=openai_compatible` (so `openai.go` is the only path this issue's repro exercises) | The two handlers are intentionally kept symmetric (near-identical doc comments, shared `templatePrefix`/`cloneAnyMap` helpers, now further unified by the new shared `handlers/chain.go`); leaving `gemini.go` unfixed would silently reintroduce the exact same class of gap for any suite (e.g. KA/AA) that configures the Gemini-native provider. |
| New dedicated namespace keys `"combined-investigate"` (mode 2) and `"full-interactive"` (mode 3) in `afRemediateNS`, each with a fresh zero-replica `memory-eater` Deployment created inside its own test | Matches the established per-test namespace isolation convention (`"autonomous"`, `"interactive"`, `"fleet"`, `"interactive-streaming"`) that exists specifically to keep parallel Ginkgo processes' RR fingerprints from colliding — same pattern as E2E-FP-1189-003/-005. |
| Keywords = the literal phrases `"create and investigate remediation"` (mode 2) and `"investigate and fix remediation"` (mode 3) | Distinct, unambiguous phrases chosen to (a) not collide with any existing FP suite chat message (grep-confirmed) and (b) both be registered **before** the bare `"investigate"` keyword (`af_investigate`) in `afKeywordYAML`, since `Registry.Detect` resolves equal-confidence ties by registration order (R1). |

---

## 5. Approach

### 5.1 Coverage Policy

This is a **test-infrastructure-only** change (`test/`, no `pkg/`/`cmd/` production code). Per `AGENTS.md`'s Pre-Implementation Workflow, "test-only modifications" qualify for Standard TDD (RED → GREEN → REFACTOR) without the full APDC preflight/spike/readiness-audit ceremony; this document + the preflight investigation already performed serve as the equivalent light-weight record.

- **Unit**: the new N-deep chain-walking helpers (`flattenToolCallChain`, `nextChainCallByCount`, `resolveTemplateArgsMap` in `handlers/chain.go`) and `resolveGeminiTemplateArgs`/`resolveOpenAITemplateArgs`'s use of them — 100% of the new logic.
- **E2E**: both new scenarios' real reachability through AF's production dispatch path (the closest analog to "wiring" for a test-fixture change, since there is no `cmd/`/`pkg/` production entry point involved).

### 5.2 Two-Tier Minimum

UT (logic: N-deep chain walking + template resolution) + E2E (both new scenarios are genuinely reachable via the real FP mock-llm ConfigMap + real AF binary + real ADK loop). No IT tier applies — this is test-double code, not a `pkg/`/`cmd/` production component.

### 5.4 Pass/Fail Criteria

**PASS**: all UT-ML-1853-* pass; E2E-FP-1853-001 and -002 pass; the full existing FP suite (spot-checked labels above) shows zero regressions; `go build ./...` and `golangci-lint run` are clean.

**FAIL**: any of the above fails, or any existing FP/mock-llm test's behavior changes.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `test/services/mock-llm/scenarios/types.go` | `MultiToolCallEntry.NextToolCall` (self-referential field, new) | ~10 |
| `test/services/mock-llm/config/overrides.go` | `ToolCallOverride.NextToolCall` (self-referential field, new) | ~10 |
| `test/services/mock-llm/scenarios/registry_default.go` | `convertToolCallChain` (new, recursive) | ~15 |
| `test/services/mock-llm/handlers/chain.go` | `flattenToolCallChain`, `nextChainCallByCount`, `resolveTemplateArgsMap` (new file; R7: now also takes a `fallback` map and swaps in the full fallback set on any unresolved placeholder) | ~70 |
| `test/services/mock-llm/handlers/gemini.go` | `resolveGeminiTemplateArgs` (refactored onto shared helper), chain-firing logic (generalized) | ~30 |
| `test/services/mock-llm/handlers/openai.go` | `resolveOpenAITemplateArgs` (refactored onto shared helper), chain-firing logic (generalized) | ~30 |
| `test/services/mock-llm/response/gemini.go` | `CountFunctionResponses` (new) | ~10 |
| `test/services/mock-llm/config/overrides.go` | `ToolCallOverride.FallbackArguments` (R7, new field) | ~10 |
| `test/services/mock-llm/scenarios/types.go` | `MultiToolCallEntry.FallbackArguments`, `MockScenarioConfig.FallbackArguments` (R7, new fields) | ~10 |

### 6.2 E2E-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `test/infrastructure/shared_e2e.go` | `combinedRemediateInvestigateScenarioYAML` (new, mode 2), `fullInteractiveRemediationScenarioYAML` (new, mode 3), `afKeywordYAML` construction | ~60 |
| `test/infrastructure/fullpipeline_e2e.go` | `afRemediateNS` map literal (2 new keys: `combined-investigate`, `full-interactive`) | ~10 |

---

## 7. BR / Issue Coverage Matrix

| Authority | Description | Priority | Tier | Test ID | Status |
|-----------|-------------|----------|------|---------|--------|
| Issue #1853 | Nested `next_tool_call:` YAML parses into an arbitrary-depth chain | P0 | Unit | UT-ML-1853-001 | Passing |
| Issue #1853 | Gemini fires chain link N (N>1) with `$from_tool` resolved against a non-adjacent prior response | P0 | Unit | UT-ML-1853-002 | Passing |
| Issue #1853 | OpenAI fires chain link N (N>1) with `$from_tool` resolved against a non-adjacent prior response | P0 | Unit | UT-ML-1853-003 | Passing |
| Issue #1853 | No cross-request state leak when resolving chain-link arguments (shared scenario singleton) | P0 | Unit | UT-ML-1853-004 | Passing |
| Issue #1853 | Mode 2 (Interactive): combined remediate+investigate single message succeeds end-to-end, stops at RCA | P0 | E2E | E2E-FP-1853-001 | Written, live-validated by direct reproduction (§3.2); Ginkgo run still pending |
| Issue #1853 | Mode 3 (Full Interactive Remediation): combined investigate+fix single message auto-chains 4 deep to a completed workflow, no manual pause | P0 | E2E | E2E-FP-1853-002 | Written, live-validated by direct reproduction (§3.2); Ginkgo run still pending |
| Issue #1853 (R7) | `fallback_arguments` YAML parses as a sibling of `arguments` under `tool_call` | P0 | Unit | UT-ML-1853-005 | Passing |
| Issue #1853 (R7) | Standalone "investigate" (no prior remediate in-session) falls back to `namespace/kind/name`; resolved `rr_id` case is unaffected (no regression) — Gemini + OpenAI | P0 | Unit | UT-ML-1853-006 | Passing |
| Issue #1853 (R7) | Live-cluster validation: standalone "start investigation" A2A request against the shared `fullpipeline-e2e` cluster produces a real new RR/IS with zero `invalid rr_id` errors | P0 | Manual/Live | N/A (curl smoke test, see §3.1) | Passing |
| Issue #1853 (R1/R5/R6) | Live-cluster validation: mode-2 and mode-3 chains fire correctly against the real, deployed mock-llm binary + AF + full controller stack (RR → IS → AIAnalysis → WorkflowExecution, ending `Completed`/`Remediated`, real memory limit patched 50Mi→512Mi) | P0 | Manual/Live | N/A (curl smoke test, see §3.2) | Passing |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `UT-ML-1853-001` | A YAML `next_tool_call:` block nested to arbitrary depth parses into the correct `MultiToolCallEntry`/`ToolCallOverride` linked list (no depth cap) | Passing |
| `UT-ML-1853-002` | Gemini handler fires the 2nd (and beyond) chain link with `$from_tool` resolved against an earlier — not just immediately-prior — `FunctionResponse`, instead of leaving the literal placeholder string | Passing |
| `UT-ML-1853-003` | OpenAI handler does the same via prior tool-result messages | Passing |
| `UT-ML-1853-004` | Resolving a chain link's `Arguments` for one request does not mutate the shared scenario singleton's config (no cross-request leakage — R3) | Passing |
| `UT-ML-1853-005` | `fallback_arguments:` parses from YAML as a sibling of `arguments:` under `tool_call:` | Passing |
| `UT-ML-1853-006` | When a `$from_tool` reference can't resolve (no prior call), the entire argument set falls back to the scenario's `fallback_arguments` (Gemini + OpenAI); when resolution succeeds, the fallback does NOT fire (no regression) | Passing |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `E2E-FP-1853-001` | Mode 2 (Interactive): a single combined "create and investigate remediation" message produces a real RemediationRequest via `kubernaut_remediate`, then a successful `kubernaut_investigate` tool call (no `invalid rr_id` error, real `rr_id` propagated) that creates an `InvestigationSession` — and the turn **stops there** (no auto-selected workflow), matching Interactive Mode's documented contract | Written; behavior live-validated by direct reproduction, §3.2. Ginkgo run pending |
| `E2E-FP-1853-002` | Mode 3 (Full Interactive Remediation): a single combined "investigate and fix remediation" message auto-chains `kubernaut_investigate` → `kubernaut_discover_workflows` → `kubernaut_select_workflow` → `kubernaut_watch` (4 deep) with `rr_id` propagated to every dependent link and the highest-confidence workflow auto-selected, reaching a completed `WorkflowExecution` with **no manual pause** | Written; behavior live-validated by direct reproduction, §3.2. Ginkgo run pending |

### Tier Skip Rationale

- **Integration**: not applicable — no `pkg/`/`cmd/` production code changes; nothing to wire into a production entry point.

---

## 9. Test Cases (P0 detail)

### UT-ML-1853-001: nested `next_tool_call:` YAML parsing (arbitrary depth)

**Type**: Unit
**File**: `test/services/mock-llm/repeat_tool_call_test.go`

**Test Steps**:
1. **Given**: a YAML `keyword_scenarios` entry with `tool_call:` followed by a `next_tool_call:` nested 3 levels deep (4 tool calls total, mirroring mode 3's shape).
2. **When**: the YAML is unmarshalled into `config.KeywordScenarioOverride` and converted via `convertToolCallChain`.
3. **Then**: the resulting `MultiToolCallEntry` linked list has exactly 4 nodes in the correct order, each with its own `Name`/`Arguments`.

### UT-ML-1853-002: Gemini fires chain link N with non-adjacent `$from_tool` resolution

**Type**: Unit
**File**: `test/services/mock-llm/repeat_tool_call_test.go`

**Test Steps** (mirrors existing `UT-ML-1407-002` harness, extended past depth 1):
1. **Given**: a 3-deep `NextToolCall` chain where link 3's arguments reference `$from_tool:kubernaut_investigate:rr_id` (link 1's tool, not link 2's).
2. **When**: an `httptest` request is sent whose conversation already contains `FunctionResponse`s for links 1 and 2.
3. **Then**: the emitted link-3 `FunctionCall.Args["rr_id"]` equals link 1's real `rr_id`, not the literal template string — proving resolution reaches back across non-adjacent responses, not just the immediately-prior one.

### UT-ML-1853-003: OpenAI fires chain link N with non-adjacent `$from_tool` resolution

**Type**: Unit
**File**: `test/services/mock-llm/repeat_tool_call_test.go`

Same scenario as UT-ML-1853-002, via the OpenAI-compatible tool-call/tool-result message format instead of Gemini's `FunctionCall`/`FunctionResponse`.

### UT-ML-1853-004: no cross-request state leak resolving chain-link arguments

**Type**: Unit
**File**: `test/services/mock-llm/repeat_tool_call_test.go`

**Test Steps**:
1. **Given**: a shared scenario singleton whose `NextToolCall.Arguments` contains a `$from_tool` template.
2. **When**: two concurrent/sequential requests matching the same scenario resolve the template with two *different* prior-response values.
3. **Then**: each request's resolved args reflect only its own conversation's value; the singleton's original `Arguments` map (and any other in-flight request reading it) is never mutated.

### E2E-FP-1853-001: Mode 2 (Interactive) — combined remediate+investigate single message

**Type**: E2E
**File**: `test/e2e/fullpipeline/16_af_a2a_combined_investigate_test.go`

**Test Steps**:
1. **Given**: a dedicated, zero-replica `memory-eater` Deployment in a fresh isolated namespace (`fpRemediateNS["combined-investigate"]`).
2. **When**: a single `message/stream` A2A request is sent: "create and investigate remediation for deployment memory-eater".
3. **Then**: HTTP 200; a real RemediationRequest is created in the dedicated namespace via `kubernaut_remediate`; an `InvestigationSession` owned by that RR is created via `kubernaut_investigate`, proving the real server-generated `rr_id` was propagated (not left as an unresolved `$from_tool` template); the turn stops there (no workflow discovery/selection/watch is triggered).

### E2E-FP-1853-002: Mode 3 (Full Interactive Remediation) — combined investigate+fix, 4-deep auto-chain

**Type**: E2E
**File**: `test/e2e/fullpipeline/17_af_a2a_full_interactive_remediation_test.go`

**Test Steps**:
1. **Given**: a dedicated, zero-replica `memory-eater` Deployment in a fresh isolated namespace (`fpRemediateNS["full-interactive"]`).
2. **When**: a single `message/stream` A2A request is sent: "investigate and fix remediation for deployment memory-eater".
3. **Then**: HTTP 200; a real RemediationRequest is created directly via `kubernaut_investigate` (no separate remediate call); the 4-deep chain auto-continues through `kubernaut_discover_workflows` → `kubernaut_select_workflow` (real seeded workflow UUID, no pause for manual input) → `kubernaut_watch`; a `WorkflowExecution` for the RR reaches completion — all triggered by the single initial message.

---

## 10. Environmental Needs

- **Unit**: Ginkgo/Gomega, `httptest`, in-process — no cluster required.
- **E2E**: existing `fullpipeline` Kind cluster infrastructure (`test/infrastructure/fullpipeline_e2e*.go`), unchanged.

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **RED**: UT-ML-1853-001/002/003/004 written against current (single-link-capped, unfixed) handlers — fail.
2. **GREEN**: make `MultiToolCallEntry`/`ToolCallOverride` self-referential; add `convertToolCallChain`; add `handlers/chain.go` (`flattenToolCallChain`, `nextChainCallByCount`, `resolveTemplateArgsMap`); refactor `resolveGeminiTemplateArgs`/`resolveOpenAITemplateArgs` and each handler's chain-firing logic onto it — UTs pass.
3. **GREEN (E2E, mode 2)**: add `afRemediateNS["combined-investigate"]` key + `combinedRemediateInvestigateScenarioYAML` + insert into `afKeywordYAML` before `af_investigate`; add E2E-FP-1853-001 — passes against a real FP cluster.
4. **GREEN (E2E, mode 3)**: add `afRemediateNS["full-interactive"]` key + `fullInteractiveRemediationScenarioYAML` (4-deep chain) + insert into `afKeywordYAML` before `af_investigate`; add E2E-FP-1853-002 — passes against a real FP cluster, proving the chain generalizes past depth 2.
5. **REFACTOR**: shared chain-walking/template-resolution helpers already extracted in step 2 (`handlers/chain.go`) rather than deferred — no separate refactor pass needed beyond that extraction.

---

## 12. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/testing/1853/TEST_PLAN.md` |
| Unit tests | `test/services/mock-llm/repeat_tool_call_test.go` |
| E2E test (mode 2) | `test/e2e/fullpipeline/16_af_a2a_combined_investigate_test.go` |
| E2E test (mode 3) | `test/e2e/fullpipeline/17_af_a2a_full_interactive_remediation_test.go` |
| Console-team findings summary | `docs/testing/1853/CONSOLE_TEAM_SUMMARY.md` |

---

## 13. Execution

```bash
# Unit tests
go test ./test/services/mock-llm/... -ginkgo.focus="UT-ML-1853"

# E2E (requires a running/creatable fullpipeline Kind cluster)
ginkgo -v ./test/e2e/fullpipeline/... --label-filter="issue-1853"
```

---

## 14. Wiring Verification (test-infra analog)

No `pkg/`/`cmd/` production entry point exists for this change (mock-llm and the FP test harness are test-only code — not part of the release artifact per prior team note). The E2E tests are the closest equivalent proof: they confirm both new scenarios are reachable via AF's real binary and real ADK tool-calling loop against the real FP mock-llm ConfigMap, not just at the unit level.

| Code Path | Entry Point | Exit Point | Proving Test | Status |
|-----------|-------------|------------|---------------|--------|
| `af_remediate_investigate_combined_1853` scenario (mode 2) | Real AF `/a2a/invoke` `message/stream` | `InvestigationSession` owned by the new RR | E2E-FP-1853-001 | Pending |
| `af_full_interactive_remediation_1853` scenario (mode 3, 4-deep chain) | Real AF `/a2a/invoke` `message/stream` | Completed `WorkflowExecution` for the new RR | E2E-FP-1853-002 | Pending |
| N-deep `NextToolCall` chain walking + `$from_tool` resolution at every link | mock-llm HTTP handler (`/v1/chat/completions`, `:generateContent`) | Resolved `FunctionCall`/`ToolCall` args in mock-llm's own response, at every chain depth | UT-ML-1853-001/002/003/004 | Pending |

---

## 15. Existing Tests Requiring Updates

None expected. If E2E-FP-1189-003/-004/-005 or E2E-AF-1409-001 regress, that indicates an unexpected keyword collision (R2) and must be fixed before merge, not worked around.

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-02 | Initial test plan (mode 2 / originally-reported scope only) |
| 2.0 | 2026-08-02 | Expanded to all 3 AF modes per user direction ("factor into 1853 the 3 modes so that the console team can test them"): generalized `NextToolCall` to arbitrary depth (was previously scoped as a single-link fix), added mode 3 (Full Interactive Remediation, 4-deep chain) scenario + E2E-FP-1853-002, added UT-ML-1853-003/004, added console-team findings summary deliverable, cross-referenced the new documentation-gap issue #1855 |
| 3.0 | 2026-08-02 | Added R7: a separate, pre-existing defect found live on the shared `fullpipeline-e2e` cluster while validating the above (standalone "investigate" with no prior "remediate" in-session sends AF a literal, unresolved `$from_tool` placeholder as `rr_id`, which AF correctly rejects — blocking the console team's Playwright runs). Added `fallback_arguments` to the mock-LLM tool-call schema (whole-argument-set fallback when any `$from_tool` reference fails to resolve, mirroring `kubernaut_investigate`'s real mutually-exclusive `rr_id`-vs-`namespace/kind/name` contract), applied it to `af_investigate`, added UT-ML-1853-005/006, and live-validated the fix directly against the shared cluster (new image + ConfigMap deployed, confirmed via a real A2A request producing a new RR/IS with zero errors). Work now tracked on branch `fix/1853-fp-mock-llm-combined-investigate` (rebased onto latest `main`) rather than directly on `main`. |
| 3.1 | 2026-08-02 | Re-ran the full `test/services/mock-llm/...` unit suite (all green) and flipped UT-ML-1853-001..004 from Pending to Passing. Live-validated mode 2 and mode 3 by direct reproduction against the shared cluster (§3.2) instead of a full Ginkgo run, to avoid an unconditional ~20-30min rebuild of every FP service image on a harness with no build-cache-hit path (`BuildImageForKind` always uses `--no-cache` + a fresh random tag). Both modes' full chains fired correctly and reached `Completed`/`Remediated` (mode 3's target `Deployment` had its memory limit genuinely patched 50Mi→512Mi by the real workflow). Found and fixed a hand-transcription footgun during this exercise (nesting `next_tool_call:` as a child of `tool_call:` instead of its sibling silently produces `cfg.NextToolCall == nil`) — confirmed this is **not** present in the committed `shared_e2e.go` source, only in the ad-hoc manual ConfigMap copy used for this validation. All scratch cluster resources cleaned up afterward; ConfigMap restored to R7-fix-only state and re-smoke-tested. |
