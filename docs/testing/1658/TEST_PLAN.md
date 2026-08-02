# Test Plan: AF Raw Tool-Call Arg Leak on Unavailable Alert Tools

**Issue**: [#1658](https://github.com/jordigilh/kubernaut/issues/1658)
**Business Requirements**: BR-AI-023 (detect/handle AI hallucinations or invalid responses), BR-AI-024 (graceful degradation when a dependency is unavailable)
**Branch**: `fix/1658-af-tool-error-leak` (originally against `main`); this copy backported verbatim to `release/v1.5` on `fix/1658-backport-v1.5` — same root cause and fix present on both branches, only `main`'s fleet-federation `ResourceReader` abstraction (`DynamicResourceReader`) is absent on v1.5, so `UT-AF-1658-020` calls `HandleKubectlList` with the raw `dynamic.Interface` there instead.
**Created**: 2026-08-01
**Status**: Complete — all scenarios in Section 3 pass

---

## 1. Purpose

When `severityTriage.enabled: false` (`cfg.PromClient == nil`), API Frontend does not
register `list_alerts`/`get_alert_details`/`kubernaut_investigate_alert` (gated in
`pkg/apifrontend/agent/root.go`'s `buildToolList` on `cfg.PromClient != nil`). The system
prompt (`pkg/apifrontend/agent/prompt.go`'s `BuildInstruction`), however, unconditionally
advertised these tools regardless of whether they were actually registered. If a user then
asked to "list active alerts", the LLM had no purpose-built alert tool available in its
function-calling schema and fell back to the generic `kubectl_list` tool, guessing
`kind: "Alert"` (not a real Kubernetes/Kubernaut resource kind). The tool call failed
(`ErrInvalidInput: cannot resolve GVK for kind "Alert"`), and instead of explaining the
failure in natural language, the model's next-turn text echoed the raw tool-call arguments
back to the user as the final chat response.

### Root cause (confirmed via code read, not just issue inference)

Two independent gaps, both required to fully close this class of bug:

1. **Prompt/registration mismatch** (`pkg/apifrontend/agent/prompt.go`): `BuildInstruction`
   took only a `namespace` parameter and had no way to know whether `alertToolConstructors`
   (`root.go:183`) were actually included in the tool list passed to the LLM. The model was
   told alert tools exist unconditionally, so a confused/weak model would attempt to use them
   via the closest available substitute (`kubectl_list`) rather than saying they're
   unavailable.
2. **No generic tool-error-to-natural-language mandate**: the existing
   `## Structured Artifact Contract (#1408)` directive in `prompt.txt` only mandates
   `present_decision`-based failure handling for `kubernaut_investigate`/
   `kubernaut_discover_workflows` — it does not cover generic observation-tool failures like
   `kubectl_list`/`kubectl_get`. Nothing told the model it must never surface raw tool-call
   arguments/JSON as its final answer for *any* tool.

Kubernaut's own error-plumbing (`pkg/apifrontend/launcher/part_converter.go`'s
`toolErrorPart`, #1302) already correctly routes tool-error `FunctionResponse`s to the
reasoning/thinking stream, never as a definitive artifact, and the ADK agent loop always
re-invokes the LLM after a tool error (never bypasses it). The leak was therefore not a
plumbing bypass — it was the LLM's own next-turn text, produced with no guardrail against
restating/echoing a failed call's arguments, passing through untouched as genuine assistant
text.

## 2. Fix Design

1. **Conditional prompt content** (`pkg/apifrontend/agent/prompt.go`): `BuildInstruction`
   and `NewInstructionProvider` take a new `alertToolsEnabled bool` parameter, threaded from
   the same `cfg.PromClient != nil` check used by `buildToolList` (wired at the single
   production call site, `cmd/apifrontend/mcp_a2a_handlers.go`'s `buildRootAgentConfig`).
   When `false`, the appended "Tool Usage Rules" section omits `list_alerts`/
   `get_alert_details`/`kubernaut_investigate_alert` from the tool list and observation
   permission line, and instead explicitly states these tools are unavailable, that "Alert"
   is not a Kubernetes resource kind, and that the model should explain unavailability rather
   than guess. The immutable embedded `prompt.txt` base is untouched (SC-7) — this is
   additive, per-deployment guidance exactly like the existing namespace/CRD-type context.
2. **Generic tool-failure mandate** (`pkg/apifrontend/agent/prompt.txt`): added
   `## Behavioral Constraints` item 5 — any tool error MUST be explained in natural language;
   raw tool-call arguments/JSON/error objects must never be surfaced verbatim. This is
   deliberately broader than the existing `present_decision`-scoped mandate (#1408, item 3 of
   the Structured Artifact Contract) since it must cover every tool, not just the two
   investigation-flow tools that already funnel through a structured artifact.
3. **Test coverage gap closed**: `kubectl_list` had no test for an unresolvable `Kind` (the
   exact failure mode in this issue), unlike its `kubectl_get` sibling
   (`UT-AF-1230-009`). Added `UT-AF-1658-020` mirroring that pattern with `Kind: "Alert"`.

Out of scope (not needed once 1-2 above close the root cause, and avoids unnecessary
special-casing): a `kubectl_list`/`kubectl_get` runtime guard for well-known "looks like it
should exist but doesn't" kind guesses (issue's suggested fix #3). The unresolved-kind error
message (`cannot resolve GVK for kind "Alert"`) already gives the LLM enough signal once it
is also told (via fix 1) that alert tools are unavailable and (via fix 2) that it must
explain tool failures rather than echo them.

## 3. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | BR | Test File |
|---|---|---|---|---|
| UT-AF-1658-001 | Unit | `alertToolsEnabled=true` preserves existing behavior: prompt advertises `list_alerts`/`get_alert_details`/`kubernaut_investigate_alert` | BR-AI-024 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-1658-002 | Unit | `alertToolsEnabled=false` omits alert tools from the observation-tool permission line and the Prometheus/Thanos usage guidance | BR-AI-024 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-1658-003 | Unit | `alertToolsEnabled=false` explicitly states alert querying is unavailable in this deployment | BR-AI-024 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-1658-004 | Unit | `alertToolsEnabled=false` instructs the model not to guess a `kubectl_list`/`kubectl_get` kind (e.g. `"Alert"`) for alert queries | BR-AI-023 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-1658-005 | Unit | `alertToolsEnabled=false` still contains the immutable core prompt (SC-7 regression guard) | BR-AI-024 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-1658-010 | Unit | Base embedded prompt mandates a natural-language explanation for any tool error and forbids surfacing raw tool-call arguments | BR-AI-023 | `pkg/apifrontend/agent/prompt_test.go` |
| UT-AF-1658-020 | Unit | `kubectl_list` with an unresolvable, guessed non-K8s kind (e.g. `"Alert"`) returns an `invalid input` error (closes prior coverage gap relative to `kubectl_get`'s `UT-AF-1230-009`) | BR-AI-023 | `pkg/apifrontend/tools/kubectl_list_test.go` |

### Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `alertToolsEnabled` conditioning | `cmd/apifrontend/main.go` startup (A2A handler construction) | `cmd/apifrontend/mcp_a2a_handlers.go`'s `buildRootAgentConfig`, threading `d.Backends.PromClient != nil` into both `BuildInstruction` and `NewInstructionProvider` | Existing `IT-AF-1367-*`/`IT-AF-1372-*` (root.go tool-registration gating, unchanged) prove the same `PromClient` gate; UT-AF-1658-001..005 prove the prompt side matches it |

No new production components — this makes an existing config-derived tool-registration gate
also drive the prompt content that describes those tools, and adds a missing general
error-handling directive to the existing prompt.

## 4. Final Results

All scenarios pass locally against `origin/main` (`go build ./...`, `go vet ./...`,
`golangci-lint run` all clean):

| ID | Result |
|---|---|
| UT-AF-1658-001..005, 010 | PASS |
| UT-AF-1658-020 | PASS |
| Full `pkg/apifrontend/agent` suite (167 specs) | PASS (0 regressions) |
| Full `pkg/apifrontend/tools` suite (632 specs) | PASS (0 regressions) |
| Full `pkg/apifrontend/...` suite | PASS (0 regressions) |
| `go build ./...`, `go vet ./...` | PASS |
| `golangci-lint run` (changed packages) | 0 issues |
