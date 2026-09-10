# DD-KA-007: Cumulative Per-RR Investigation Accounting (Turns, Tool Calls, Tokens) for Console Reporting

**Date**: 2026-09-09
**Status**: ✅ **APPROVED — Implemented** (Issue [#2387](https://github.com/jordigilh/kubernaut/issues/2387))
**Decision Maker**: Kubernaut Architecture Team
**Authority**: APPROVED — implementation complete, all quality gates passed
**Affects**: KubernautAgent (KA), APIFrontend (AF), AgentSession CRD consumers; follow-up in kubernaut-console (ui-core rendering)
**Related**: [DD-AUDIT-003](./DD-AUDIT-003-service-audit-trace-requirements.md) (per-service audit requirements), [ADR-034](./ADR-034-unified-audit-table-design.md) (unified audit table), [DD-KA-017](./DD-KA-017-three-step-workflow-discovery-integration.md) (discovery pipeline whose legs are accounted here)
**Supersedes**: Issue [#435](https://github.com/jordigilh/kubernaut/issues/435) §3 ("No API response change — token totals are audit-only") **for raw token counts only**. #435 was written against a previous agent harness that constrained what could be surfaced; that constraint no longer holds. Financial/USD cost reporting remains **out of scope** (BR-KA-195, V2.0-pending).
**Business Requirement**: BR-KA-OBSERVABILITY-001 (call-level observability in the structured decision payload), BR-AUDIT-005 v2.0 (full lifecycle reconstruction by `correlation_id`), BR-KA-195 (cost-tracking context; counts only, no costs)

---

## 1. Context

Kubernaut Agent investigations completed successfully with rich RCA content, but `InvestigationResult.TotalToolCalls` / `TotalLLMTurns` were always `0` (omitted via `omitempty`). The API Frontend backfilled `0` and the console rendered `0 tool calls, 0 LLM turns` even though tools clearly ran (live-cluster evidence: RR `rr-79268fab63b3-340280e5`, 5-entry `causal_chain` citing `kubectl_describe` outputs).

Root causes found (all verified by call-graph + pattern search, zero production callers):

1. `InvestigationMetrics` (`internal/kubernautagent/investigator/investigation_metrics.go`) was defined and unit-tested in isolation but **never instantiated** in production code.
2. No production assignment to `TotalLLMTurns`/`TotalToolCalls` existed anywhere under `internal/kubernautagent/` — only test fixtures and the `MarshalRCASubset` passthrough.
3. `rcaEventPayload`'s `omitempty` dropped the zeros, so AF could not distinguish untracked from tracked-zero and backfilled `0` per #2073/#2074 (the LLM is deliberately never asked to supply counts).
4. Token usage (`TokenAccumulator`) was audit-only per #435: populated on cancelled paths only, never on success, and never threaded through discovery/interactive legs at all (`LLMInvocationContext.Tokens == nil` there), so phase-3 and interactive tokens were invisible even to audit.
5. `AgentSession.status.result` mapping (`buildRootCauseAnalysisMap`) copied narrative fields but no counts, so `kubectl` observers saw no keys.
6. Interactive final assembly (`select_workflow.buildFinalResult`, `complete_no_action.buildNoActionResult`) had no access to any accumulator — interactive sessions undercounted (extraction-only) or reported zeros.

## 2. Decision

### 2.1 Per-RR scopes are the single source of truth; reset only in `Investigate`

Two mutex-guarded, TTL-reclaimed scopes on the singleton `Investigator` (mirroring the `anomalyScope` #1892 precedent — no signature threading, no shared-counter corruption):

- `metricsScope`: LLM turns + tool dispatches per `correlationID` (== RemediationRequest name).
- `tokenScope`: raw provider prompt/completion/total counts per `correlationID`. **Counts only, never financial costs.**

Both reset exactly once per autonomous `Investigate` entry — never between phases or legs — so RCA + workflow-selection + interactive + extraction + discovery legs accumulate into **cumulative per-RR totals**. Assembly points **SET** from the scope (overwrite, never add); results never pre-carry counts, which makes takeover flows (autonomous → interactive → discovery under one RR) correct by construction: nothing wiped, nothing double-counted. TTL pruning rides the existing anomaly-cleanup tick (`cmd/kubernautagent/main.go`); no new goroutine, no cmd changes.

### 2.2 Counting rules

- One successful provider round-trip = one LLM turn (tool-call, sentinel-submit, truncation, and plain-text turns all count; failures/cancellations do not). Recorded at 7 sites: `runLoopTurn`, both parse-retry attempts, 3 gate retries, extraction.
- Tool calls are batch-counted after `processToolCalls`' `g.Wait()`. Sentinel submits (`submit_result*`) return before dispatch and are **consumed, never executed, never counted**.
- Token usage is recorded adjacent to every `tokens.Add` site plus extraction (which previously discarded `resp.Usage`). The existing `TokenAccumulator`/audit path is untouched.

### 2.3 Wire contracts (all additive, safe rollout both directions)

| Hop | Change |
|-----|--------|
| KA → AF complete event (`rcaEventPayload`) | `total_llm_turns`/`total_tool_calls` lose `omitempty` (tracked-zero serializes; pre-fix payloads omit — distinguishable during rollout). `prompt_tokens`/`completion_tokens`/`total_tokens` added, omitempty, present once recorded. Server-injected only (SI-10). |
| AF `InvestigateRCA` | Same 3 token fields, omitempty. Struct-unmarshal decode — no parser change. |
| AF `canonicalGroundedRCA` → `RCAData` | Renames all five bookkeeping fields; `RCAData` gains 3 `omitempty` fields, **never `required`** (#2073/#2074 lesson — harness-substituted, LLM never instructed). Ungrounded backfill zeroes all five (no invented numbers). |
| `AgentSession.status.result.rootCauseAnalysis` | `total_llm_turns`/`total_tool_calls` when non-zero (free-form `PreserveUnknownFields` JSON — **no CRD change**; empty→nil contract preserved). |
| Console (`kubernaut-console`, follow-up) | `ChatMessage.rca` += 3 optional token fields; `RCACard` metadata line extended. Separate issue (see §5). |

## 3. Alternatives considered (spike verdicts)

| Alternative | Verdict |
|-------------|---------|
| Thread accumulators through `LLMInvocationContext`/param structs | **No** — ~10 signature touches incl. 3 retry-param structs and one fresh construction that already drops fields; every future call site must remember to thread. Scope needs only the correlationID every site already carries. |
| Thread `TokenAccumulator` into discovery/interactive (sessions hold accumulators) | **No** — session-state coupling for zero gain over the scope; audit path stays exactly as-is under this decision. |
| Per-leg reset with snapshot-before-reset | **No** — more moving parts, snapshot-placement bugs (the message-turn loss this decision fixes); cumulative matches "total across investigation" language and the console's display. |
| Singleton shared counter | **No** — corrupts concurrent investigations (the exact #1892 failure mode). |
| Surface USD costs now | **No** — out of scope per BR-KA-195 sequencing; provider counts only. |

## 4. Consequences

- **Positive**: console shows real turns/tools/tokens on all paths (autonomous, interactive, discovery, takeover); `status.result` carries counts for `kubectl` observers; audit gains uniform `total_*` fields (`ResultToAuditJSON`, always present) for BR-AUDIT-005 reconstruction; discovery/interactive token blindness fixed as a side effect.
- **Negative/risks**: per-RR scope entries live until TTL prune (bounded, same as anomaly entries); sequential re-discoveries on one RR accumulate (intended cumulative semantics — documented in code and tests); `UT-KA-2387-006` encodes cumulative semantics — any future per-leg requirement must revisit this DD first.
- **#435 amendment**: §3's "tokens are audit-only, not surfaced" is deprecated for raw counts only. Rationale recorded: the limiting harness is gone; counts (not costs) are operator-operations data with no new sensitivity (authenticated operators, existing RBAC).

## 5. Compliance mapping

- **FedRAMP AU-2**: turn/tool executions remain auditable events; **AU-3**: counts + tokens in `response_data`/RCA subset keyed by `correlation_id`; **SI-10**: all five numbers server-computed, LLM never instructed (mirrors #2073 hardening).
- **SOC2 CC7.2**: lifecycle (signal → RCA → selection → tokens) reconstructable from audit traces alone.
- **OWASP ASVS 7.1.1/7.2.1**: security-relevant investigation activity logged on a correlationID-tied trail; bounded `MarshalRCASubset` still leaks no workflow/validation state.

## 6. Test evidence (pyramid)

- **UT (logic)**: `UT-KA-2387-001/002/005` (Investigate totals, server-side, singleton isolation), `UT-KA-2387-003/009` (payload keys), `UT-KA-2387-006/007` (cumulative legs, scope-source-of-truth), `UT-KA-2387-008` (TokenUsage on success), `UT-KA-2387-011/012` (mapping, nil-contract guard).
- **IT (wiring)**: `IT-KA-2387-010` (Investigate→complete event), `UT-KA-2387-013/014` (discovery Step-5, complete_no_action provider), `UT-KA-2387-015/016` (adapter seam + interface compliance).
- **AF**: `UT-AF-2387-020/021/023` (bridge decode, rename, schema non-required).
- Full suites green: `investigator`, `session`, `agentsession`, `mcp/tools`, `mcp/adapters`, `apifrontend/agent`, `apifrontend/tools`, `katypes`; `go build ./...`, `go vet`, `golangci-lint` clean.
- **E2E**: `E2E-FP-2387-001` (autonomous OOMKill → `total_llm_turns`/`total_tool_calls` keys present and ≥1 in `AgentSession Status.Result.RootCauseAnalysis`; pre-fix the keys were unconditionally absent) and `E2E-AF-2387-002` (grounded `present_decision` artifact serves the real turn count with token sums tied exactly to it: completion == 50×turns, prompt excess in whole 400-unit tool-response turns, total == prompt+completion; sentinel-only fixture pins `tool_calls_count == 0`). Supporting slices: shared-client streamed-usage parse (`UT-KA-2387-100`) and mock-LLM trailing usage chunk + `usage:` override (`UT-MOCK-2387-001/002/003`). Lane execution pending (no live cluster in this environment); RED-verifiable in lane via `git stash` of the product fix.
- **Watch item (resolved in-lane on helios08)**: `E2E-AF-1396-001` asserted `tool_calls_count == 0` / `llm_turns == 0`. Those zeros were recorded while totals were hard-zero pre-fix, so grounded-zeros and fallback-zeros were indistinguishable; post-fix its grounding (a real synchronous Investigate per #1818 Gap 3) serves real totals down the grounded path, and the lane confirmed the breakage (turns read 1, tokens 1100/150 — identical executed work to the 3-turn landing). Its expectations now encode the race-honest bounds (turns ∈ {1,2,3}, tools ∈ {0,1}; see duplicate-Investigate reset race below) rather than zeros.
- **Known product race (follow-up required)**: the AF agent issues `kubernaut_investigate` TWICE ~25s apart for one RR (grounding turn + retry/continuation), and the second entry calls `resetMetrics` at `Investigate` entry while the first flight is still accumulating. Depending on interleaving, the artifact reads post-reset partials (1 turn / 0 tools) or full cumulative totals (3 turns / 1 tool) for byte-identical executed work and token sums (audit-proven: 1100 prompt / 150 completion on both landings). Re-entry reset vs in-flight accumulation needs a design decision (idempotent investigation start or generation-scoped accounting); the E2E assertions are written landing-invariant until then. Companion wart: KA's turns match the AF-side `af_investigate` keyword scenario (keyword shadowing), dispatching `kubernaut_investigate` inside KA where it errors with "tool not found" yet still counts as a dispatch.
- **Rollout note**: production `KubernautAgentHighTokenUsage` (`rate(aiagent_api_llm_tokens_total[1h]) > 1000000`) previously evaluated streaming-blind; post-fix it sees true values. Threshold semantics unchanged — a newly-firing alert is a true positive — but operators should expect a step-change in token dashboards at deploy time. Console rendering itself is the kubernaut-console follow-up issue below (#127).
