# Findings Summary for the Console Team — Issue #1853

**Audience**: kubernaut-console team
**Related**: [#1853](https://github.com/jordigilh/kubernaut/issues/1853) (this fix), [#1855](https://github.com/jordigilh/kubernaut/issues/1855) (formal documentation of the 3 modes below — tracked separately, closes in a follow-up PR)
**Status**: Fix implemented, unit tests green; standalone-investigate fallback fix AND mode-2/mode-3 chaining are both live-validated against the shared cluster (direct reproduction, not yet via the Ginkgo E2E specs themselves — see caveat below)

---

## ⚡ If you were seeing `invalid rr_id` errors on the shared cluster

**That's fixed and already deployed** to the `fullpipeline-e2e` cluster you're using for Playwright testing (as of this update). It was a **separate, pre-existing defect** in the same test-double code area, found while investigating #1853 — not something #1853 introduced. Root cause and fix are in "What we actually fixed", item 3 below. If you send a bare "investigate ..." message with no prior "remediate" in the same session, it now correctly creates a new investigation against `kubernaut-system/Deployment/memory-eater` instead of erroring. No action needed on your side — just confirming your Playwright runs should stop hitting this.

---

## TL;DR

A single chat message that combines "investigate" and "remediate" intent — e.g. *"the memory-eater deployment is OOMKilling, investigate and fix it"* — was completely broken end-to-end in our `fullpipeline` E2E test environment. It's fixed now. While fixing it, we discovered AF actually supports **three** distinct behaviors for this kind of request, not one, and only one of the three had any test coverage before this work. All three now have E2E coverage. Read below to know which one your UI copy/flow should assume for a given phrasing, and what each one actually does.

---

## The 3 modes AF supports today

These are defined in AF's production system prompt (`pkg/apifrontend/agent/prompt.txt`) and are **already live in production** — this issue only added test coverage for them, it did not add new AF behavior.

### Mode 1 — Autonomous ("fire-and-forget fix")

**Triggered by**: "fix", "remediate", "address", "resolve", "heal" + a target, when the user does not expect to interact further.
**Example**: *"remediate the memory-eater deployment"*

**What happens**: AF calls `kubernaut_remediate` once and confirms. That's it — no RCA is streamed to the chat, no findings are shown, no workflow choice is presented. The remediation pipeline (investigation, workflow selection, execution) runs entirely server-side. If your UI wants to show progress for this mode, it needs to poll/watch the RR separately (e.g. via `kubernaut_get_remediation` or a list view) — the chat turn itself will not narrate it.

**Was this broken by #1853?** No. A single tool call has nothing to chain, so this mode was never affected.

### Mode 2 — Interactive ("investigate, then wait for me")

**Triggered by**: "investigate", "what's wrong with", "diagnose", "look into".
**Example**: *"investigate the memory-eater deployment"*, or the combined form *"create and investigate a remediation for memory-eater"*

**What happens**: AF calls `kubernaut_investigate`, streams live reasoning/tool-call events during the investigation, then presents the root-cause findings and **stops**. It will not discover or select a workflow until the user says so in a follow-up message (e.g. "show me remediation options").

**Was this broken by #1853?** **Yes — this is the original bug report.** When a single message combined create-a-remediation + investigate intent, our E2E mock-LLM test fixture had no way to chain `kubernaut_remediate` → `kubernaut_investigate` within one turn while correctly passing the real, server-generated `rr_id` from the first call into the second. It's fixed now (see "What we fixed" below). **This was a test-fixture defect only — AF's real production code path for this was already correct** (confirmed via code review of `HandleRemediate`/`HandleInvestigate`); the bug only prevented our E2E suite from *proving* it worked.

### Mode 3 — Full Interactive Remediation ("investigate and fix, no manual pause")

**Triggered by**: "investigate and fix", "diagnose and remediate", "look into and fix" — i.e. combined intent where the user wants the fix applied but still wants full transparency into what was found and what's being done.
**Example**: *"investigate and fix the memory-eater deployment"*

**What happens**: AF calls `kubernaut_investigate` (same RCA transparency as mode 2 — findings ARE streamed to the user), but instead of stopping, it **automatically continues**: discovers available workflows, **auto-selects the highest-confidence one** (no "please choose a workflow" prompt), and watches it through to completion — all within the same conversation turn, driven by one user message. Approval gates (if the selected workflow requires one) still pause and require the console's Approve/Reject buttons exactly as in any other flow; the "no pause" here refers only to *workflow selection*, not to safety approval gates.

**Was this broken by #1853?** **Yes, and more severely than mode 2** — this needs a 4-deep tool call chain (`investigate` → `discover_workflows` → `select_workflow` → `watch`) with the `rr_id` correctly propagated to 3 different downstream calls. Our test infrastructure had a hard cap of exactly one chained call, so this mode had **zero test coverage at all** before this work — it wasn't just broken, it was silently unverifiable. This mode's *behavior* was not previously documented anywhere the console team would have seen it either (that's what issue #1855 tracks fixing).

---

## What we actually fixed

Three changes, all confined to **test infrastructure** (`test/`) — no AF production code (`pkg/`, `cmd/`) changed:

1. **Mock-LLM chaining depth**: our E2E test double for the LLM previously supported chaining exactly one follow-up tool call after the first. We generalized it to support a chain of any length, so scenarios like mode 3's 4-call sequence can be scripted as a single scenario instead of requiring the (impossible, in mode 3's case) workaround of multiple separate user turns.
2. **`$from_tool` template propagation**: when chaining tool call B after tool call A, B's arguments can reference a field from A's real result (e.g. `rr_id`) via a `$from_tool:A:rr_id` placeholder. This placeholder was only being resolved for the *first* call in a chain, not any subsequent link — so a 2-or-more-deep chain would send the literal, unresolved string `"$from_tool:kubernaut_investigate:rr_id"` as the argument instead of the real ID, and the downstream call would fail.
3. **`fallback_arguments` for unresolvable `$from_tool` references** (found live on the shared cluster while validating the above): the `af_investigate` scenario always sent `rr_id: "$from_tool:kubernaut_remediate:rr_id"`, on the assumption that a prior `kubernaut_remediate` call always exists in the session. When "investigate" is sent as the *first* message in a session (no prior remediate), that placeholder had nothing to resolve against and was sent to AF verbatim as a literal string — which AF correctly rejected as an invalid resource name (`invalid rr_id: invalid resource name "$from_tool:kubernaut_remediate:rr_id"`), exactly matching what showed up in the AF pod logs on the shared cluster. We added `fallback_arguments` to the mock-LLM's tool-call schema: when a `$from_tool` reference can't resolve, the entire argument set now falls back to `namespace/kind/name` (targeting the same `kubernaut-system/memory-eater` fixture used elsewhere), mirroring `kubernaut_investigate`'s real, mutually-exclusive `rr_id`-vs-`namespace/kind/name` argument contract. This is a test-double-only fix — production AF was never sent a bad `rr_id` by choice, it just correctly rejected the bad one our test double was sending it.

All three fixes have new unit test coverage (`UT-ML-1853-001` through `-006`) proving the chain-walking, template resolution, and fallback behavior work correctly, including checks that concurrent requests don't leak resolved values into each other and that the fallback doesn't fire when resolution succeeds (no regression to existing multi-turn flows).

---

## What you can test right now

Two new E2E specs exist in `test/e2e/fullpipeline/`, each driving AF through its real A2A `message/stream` endpoint exactly the way the console does:

| Mode | Test ID | File | Trigger phrase used in the test |
|------|---------|------|----------------------------------|
| 2 (Interactive) | `E2E-FP-1853-001` | `16_af_a2a_combined_investigate_test.go` | "create and investigate remediation for deployment memory-eater" |
| 3 (Full Interactive Remediation) | `E2E-FP-1853-002` | `17_af_a2a_full_interactive_remediation_test.go` | "investigate and fix remediation for deployment memory-eater" |

Run them with:

```bash
ginkgo -v ./test/e2e/fullpipeline/... --label-filter="issue-1853"
```

**Important caveat for your own testing against a real LLM (not our mock)**: our E2E fixture recognizes fixed keyword phrases because it's a scripted test double, not a real LLM. Against the real production AF (backed by Gemini/OpenAI), mode selection depends on the LLM correctly classifying free-form user intent per the trigger phrases documented in `pkg/apifrontend/agent/prompt.txt` — those phrases are examples, not an exhaustive/exact-match list. If you're building automated console-side tests against a real LLM backend, expect some phrasing variance in which mode fires, and consider that a separate (LLM prompt-tuning) concern from this fix.

---

## What's still open

- **Documentation** ([#1855](https://github.com/jordigilh/kubernaut/issues/1855)): the 3-mode contract above lives today only in AF's internal system prompt. There is no console-facing or integration-guide documentation describing it. That issue tracks writing it; this PR does not close it.
- **Ginkgo E2E run**: `E2E-FP-1853-001`/`-002` compile, and the exact behavior they assert has now been **live-validated directly against your shared cluster** (real `curl` A2A requests against the real `apifrontend` + the already-deployed `mock-llm` binary containing the fix — see `TEST_PLAN.md` §3.2 for full evidence: real RR/IS/WorkflowExecution CRDs reaching `Completed`, and mode 3's target `Deployment`'s memory limit genuinely patched 50Mi→512Mi by the real remediation workflow). The Ginkgo specs themselves haven't been run yet, only because doing so would trigger an unrelated ~20-30min unconditional rebuild of every FP service image on this harness (no build-cache-hit path); this is tracked as a follow-up, not a blocker. The scenario config used for this validation was temporary and has been removed from your shared cluster's ConfigMap — only the `fallback_arguments` fix (item 3 above) remains live there. Mode 2/3 scenarios will be present on the shared cluster automatically once this PR merges and the cluster is next refreshed through normal CI/automation.

---

## Quick reference: which mode does my UI copy map to?

| If your UI says... | AF mode | User sees RCA findings? | User picks the workflow? |
|---|---|---|---|
| "Auto-remediate" / "Fix now" (no review) | 1 — Autonomous | No | No (fully automatic) |
| "Investigate" / "Diagnose" | 2 — Interactive | Yes | Yes (separate step, user-driven) |
| "Investigate and fix" / "Diagnose and remediate" | 3 — Full Interactive Remediation | Yes | No (auto-selected, but shown to user) |
