# Running KA + AF locally against a self-hosted OpenAI-compatible LLM (v1.5.2)

**Applies to**: `v1.5.2` only. Kubernaut Agent's (KA) LLM client stack changed
materially in 1.6.0 (`langchaingo` removal, shared `openaicompat` client,
[DD-LLM-008](../../../architecture/decisions/DD-LLM-008-restart-required-llm-identity-lock.md)
identity lock, [#1604](https://github.com/jordigilh/kubernaut/issues/1604)
reasoning-effort knob) — none of that exists at this tag. If you're running
`main`/1.6.0+, this doc's *architecture* section doesn't apply to you (though
the vLLM server-side recommendations still do); check
[`configuration-reference.md`](../../../services/kubernaut-agent/configuration-reference.md)
instead.

This doc is for operators/contributors who want to point a locally-running
KA and/or AF at a self-hosted, OpenAI-API-compatible model server (vLLM,
Ollama, LlamaStack, etc.) instead of a hosted provider (Anthropic, OpenAI,
Vertex AI, Gemini). It consolidates findings from a real pilot exercise
(vLLM serving `gpt-oss-120b`), but the configuration guidance applies to any
self-hosted OpenAI-compatible model of similar or lower agentic capability
than a frontier hosted model.

> If you just want a stale-doc pointer: this supersedes the "Quick Start for
> Local LLM" section of [`LLM_SETUP_GUIDE.md`](LLM_SETUP_GUIDE.md), which
> describes a pre-microservices-architecture `SLM_*` env var scheme that no
> longer applies to KA or AF.

---

## 1. Architecture — KA and AF use *different* LLM client stacks at v1.5.2

This is the single most important thing to understand before configuring
either service — they don't share code here, and their capabilities differ:

| | KA (`cmd/kubernautagent`) | AF main agent (`cmd/apifrontend`, A2A) | AF severity triage (`cmd/apifrontend`) |
|---|---|---|---|
| Client library | `tmc/langchaingo` (`pkg/kubernautagent/llm/langchaingo/adapter.go`) | in-house ADK adapter (`pkg/apifrontend/launcher/openai/adapter.go`) | Google GenAI SDK / Anthropic SDK only |
| Self-hosted OpenAI-compatible endpoint support | Yes — `provider: openai` + `endpoint` | Yes — `provider: openai` or `openai_compatible` + `endpoint` | **No** — provider switch only recognizes `vertex_ai`, `gemini`, `anthropic`; anything else errors at startup |
| Retry on transient errors | Only on a non-streaming fallback path production doesn't use in practice | None | N/A (not usable) |
| Resilience | None beyond `maxRetries` (see caveat in §3) | `circuitBreaker` (open/half-open/closed), applied to the transport | N/A |
| Reasoning-token capture (`reasoning_content`) | Not implemented — silently dropped | Not implemented | N/A |
| `tool_choice` forcing | Not implemented | Not implemented | N/A |

**Practical consequence**: if you're running fully local/self-hosted with no
cloud LLM credentials at all, **AF's severity-triage feature cannot use your
self-hosted model** and will fail to start if `severityTriage.enabled: true`
with a non-`vertex_ai`/`gemini`/`anthropic` provider. Either set
`severityTriage.enabled: false`, or point `severityTriage.llm` at a real
cloud credential separately from `agent.llm` (the two are independently
configurable — `severityTriage.llm` falls back to `agent.llm` only if unset,
per `triageLLMSource` in `cmd/apifrontend/main.go`). This is a tracked gap,
not a deliberate design decision — see
[#1618](https://github.com/jordigilh/kubernaut/issues/1618) — and is present
on `main` too, not just this release.

---

## 2. vLLM (or equivalent) server setup

Independent of Kubernaut's version. Example for `gpt-oss-120b`:

```bash
vllm serve openai/gpt-oss-120b \
  --tool-call-parser openai \
  --reasoning-parser openai_gptoss \
  --enable-auto-tool-choice \
  --default-chat-template-kwargs '{"reasoning_effort": "high"}' \
  --max-model-len 131072
```

- `--default-chat-template-kwargs` sets the server-side default reasoning
  effort. Neither KA nor AF sends a `reasoning_effort` request parameter at
  v1.5.2 (see §5), so this is the *only* way to control it — set it once at
  the server, not per-request.
- Verify `--reasoning-parser`/`--tool-call-parser` flag names against your
  installed vLLM version; they've changed across releases and gpt-oss
  tool-calling had multiple correctness bugs fixed through v0.10.2+. Pin a
  recent, tested version.
- Test your actual tool set through KA's `langchaingo` adapter and AF's
  in-house adapter directly before trusting a pilot — both are independent,
  simpler HTTP-mapping layers than a raw `curl` against vLLM, and each could
  mishandle gpt-oss-specific response quirks differently.

---

## 3. KA configuration (`config.yaml` + `llm-runtime.yaml`)

### Static config (`config.yaml`)

```yaml
ai:
  llm:
    provider: openai        # not "openai_compatible" — that value doesn't exist in KA at v1.5.2
    model: openai/gpt-oss-120b
    endpoint: http://<vllm-host>:8000   # langchaingo adapter appends /v1 itself
  investigation:
    maxTurns: 50             # default 40 — see §6 risk #1613/#1615 trade-off before raising further
  safety:
    anomaly:
      maxToolCallsPerTool: 15   # default 10 — soft limit, safe to raise
      maxTotalToolCalls: 45     # default 30 — HARD limit (see §6), give real headroom
      maxRepeatedFailures: 4    # default 3 — soft limit, safe to raise
```

### Hot-reloadable runtime config (`llm-runtime.yaml`)

```yaml
model: openai/gpt-oss-120b
endpoint: http://<vllm-host>:8000
apiKey: EMPTY          # plaintext field at v1.5.2 (no apiKeyFile) — vLLM usually ignores it
temperature: 0.3       # lower than the 0.7 default — reduces variance given weaker agentic reliability
timeoutSeconds: 300    # up from 120s default — see caveat below, err high
maxRetries: 3          # default 3 — see caveat below, mostly moot in production
```

**Caveats specific to what these fields actually do at v1.5.2** (verified
against the `v1.5.2` tag directly, not assumed from `main`):

- `apiKey` is **inline plaintext in the YAML** at this version — there is no
  `apiKeyFile` indirection for KA's runtime config (that's a later addition).
- `maxRetries`/backoff only applies to `ChatWithParams`
  (`pkg/kubernautagent/llm/chat_helpers.go`), a non-streaming fallback path
  the production interactive investigation loop doesn't call. The path it
  *does* call (`Investigator.chatOrStream` → `client.StreamChat`) has a
  single timeout and **no retry at all** — a transient error or timeout
  aborts the whole investigation immediately. Set `timeoutSeconds`
  generously; there's no safety net if you guess too low. (Tracked for a
  fix in 1.6.0: [#1612](https://github.com/jordigilh/kubernaut/issues/1612).)
- `maxTotalToolCalls` hits a **hard abort with no chance for the model to
  summarize** what it already found. Raising the ceiling is your only lever
  today. (Tracked: [#1613](https://github.com/jordigilh/kubernaut/issues/1613).)
- A truncated (`finish_reason=length`) response gets exactly one escalated
  retry (`2× completion_tokens`, capped at 16384); if truncated again, the
  partial/possibly-empty content is silently accepted as final. Reasoning
  models at high effort are more likely to double-truncate than a plain
  chat model. (Tracked: [#1614](https://github.com/jordigilh/kubernaut/issues/1614).)
- Conversation history is never trimmed/summarized across turns — a long,
  many-turn investigation can silently exceed a smaller-context model's
  window with no detection. Balance `maxTurns` against this: raising it to
  avoid premature give-up (per the point above) increases exposure here.
  (Tracked: [#1615](https://github.com/jordigilh/kubernaut/issues/1615).)
- No reasoning-effort config knob exists at v1.5.2 either way (that's the
  1.6.0-era [#1604](https://github.com/jordigilh/kubernaut/issues/1604)
  feature, and even there it doesn't yet recognize gpt-oss's model name —
  see [#1604](https://github.com/jordigilh/kubernaut/issues/1604) follow-on
  discussion). Effort is controlled entirely at the vLLM server (§2).
- `langchaingo`'s response parsing silently drops any `reasoning_content`
  field a self-hosted reasoning model returns — you get **zero visibility**
  into the model's chain-of-thought anywhere in KA's investigation
  transcript, logs, or audit trace on this version.
- `provider`/`model` changes via hot-reload of `llm-runtime.yaml` take
  effect **live, with no restart requirement or identity-consistency check**
  (the [#1599](https://github.com/jordigilh/kubernaut/issues/1599)/
  [DD-LLM-008](../../../architecture/decisions/DD-LLM-008-restart-required-llm-identity-lock.md)
  protection is 1.6.0-only). Treat "restart after changing the model" as a
  manual operational discipline for this version, not something the
  software enforces.

---

## 4. AF configuration (`config.yaml`)

```yaml
agent:
  llm:
    provider: openai_compatible
    model: openai/gpt-oss-120b
    endpoint: http://<vllm-host>:8000
    apiKeyFile: /path/to/dummy-key-file   # AF uses file-based apiKeyFile, unlike KA's inline apiKey
    timeoutSeconds: 300
    circuitBreaker:
      enabled: true       # disabled by default — enable it, since it's AF's only resilience mechanism here
      maxRequests: 5
      interval: 60s
      timeout: 30s
      failureThreshold: 3

severityTriage:
  enabled: false   # openai_compatible is NOT supported for severity triage — see §1 and #1618
  # If you need severity triage locally, either leave it disabled, or set an
  # explicit severityTriage.llm block pointing at a real vertex_ai/gemini/anthropic
  # credential — it does not have to match agent.llm. This is a known gap
  # (https://github.com/jordigilh/kubernaut/issues/1618), not a documented
  # design decision — AF's own ADR-002 states local-model support for
  # air-gapped deployments as an explicit goal, so this is expected to close
  # eventually. Tracked on `main` first; backport to release branches is a
  # separate decision.
```

Notes:

- Changes to `agent.llm` are **restart-required by design** at v1.5.2 —
  there's no hot-reload watcher for AF's config at all (consistent with AF's
  overall restart-only posture, unrelated to gpt-oss specifically).
- AF's `circuitBreaker` is the one piece of LLM-call resilience either
  service has at this version — there's no retry logic in the in-house
  `openai` adapter itself. It's worth enabling explicitly for a pilot since
  it defaults to `enabled: false`.
- Like KA, AF's adapter doesn't send `reasoning_effort` or `tool_choice` —
  same vLLM server-side workaround from §2 applies.

---

## 5. Operational recommendations for the pilot

### On the auto-approval confidence threshold: don't trust a number you haven't measured

The Rego approval policy's `confidence_threshold` (`rego.confidenceThreshold`,
default `0.8`) is, in practice, **the only gate standing between an LLM's
remediation suggestion and unattended execution** — approval-required actions
aside (sensitive resource kinds, missing remediation target,
infrastructure-provisioning actions), it's the sole confidence-based control
(`pkg/aianalysis/testdata/policies/approval.rego`: `is_high_confidence if {
input.confidence >= confidence_threshold }`). It is worth understanding
exactly what that number is before leaning on it as a pilot safety lever:

- `confidence` is **entirely LLM self-report** — a byproduct of the model
  narrating its own uncertainty in the RCA prompt's "Pre-Submit Adversarial
  Due Diligence" step ("Start at 1.0 and list each factor that reduced it...":
  `internal/kubernautagent/prompt/templates/incident_investigation.tmpl`).
  There is no log-prob-based score, no independent verifier, no
  self-consistency/ensemble check, and no historical calibration curve behind
  it — just the model grading its own homework.
- The JSON schema enforces that the calibration field is *present* as a
  string (`internal/kubernautagent/parser/schema.go`), not that it's truthful
  or substantive. A model can satisfy the schema with a generic placeholder
  sentence while still reporting `confidence: 0.95`.
- Raising the threshold (e.g. to `0.92`) only filters cases where the model
  is self-aware enough to flag its own gaps. It does **nothing** for a
  confidently wrong RCA — a hallucination the model sincerely (if
  miscalibrated-ly) believes is correct — which is the more dangerous class
  of error precisely because it clears any threshold you pick. Worse, the
  same weak agentic reliability that motivates using a higher threshold in
  the first place (missed evidence, premature investigation abort per
  [#1613](https://github.com/jordigilh/kubernaut/issues/1613), truncated
  output silently accepted per
  [#1614](https://github.com/jordigilh/kubernaut/issues/1614)) is exactly
  what undermines the model's ability to *recognize* those same gaps when
  self-scoring its confidence. Leaning on self-reported confidence to
  compensate for unreliable self-assessment is circular.
- **Kubernaut has no eval harness that runs candidate models against its own
  demo scenarios to measure RCA accuracy or confidence calibration.** Demo
  scenarios exist as a specification
  (`docs/requirements/BR-PLATFORM-002-demo-scenario-specification.md`), but
  nothing today replays them against a candidate model and compares
  self-reported confidence to human-verified correctness. Without that, any
  specific threshold value — `0.8`, `0.92`, or otherwise — is a guess, not a
  measurement, for any given model.

**Recommendation**: don't tune the threshold as your primary mitigation for
this pilot. Instead:

- Treat **100% human review** as the default for the pilot's duration,
  independent of whatever `confidence` the model reports — the threshold
  should not be trusted to do this job until it's been validated.
- If/when an eval harness against Kubernaut's own demo scenarios exists,
  use it to measure the model's actual confidence-vs-correctness calibration
  first, and only then decide whether any threshold value buys real
  precision — and what that value should be. (Tracked:
  [#1622](https://github.com/jordigilh/kubernaut/issues/1622).)
- Relax to threshold-based auto-approval only after that comparison shows
  the model's self-reported confidence is a meaningfully predictive signal
  for your workload, not before.

### Other recommendations

- **Validate tool-calling end-to-end through each service's actual adapter**
  (not just a raw `curl` to vLLM) before trusting the pilot — see §2.
- **Don't rely on hot-reload to swap models** on KA — restart manually (§3).
- Expect **no visibility into the model's reasoning trace** on this version
  (§3) — if RCA quality is hard to diagnose, that's a concrete argument for
  upgrading to 1.6.0 rather than trying to backport reasoning capture, which
  was built as a unit with the client-library replacement.

---

## 6. Known gaps tracked for 1.6.0 (not backported to v1.5.2)

| Issue | Gap | Generic beyond gpt-oss/vLLM? |
|---|---|---|
| [#1612](https://github.com/jordigilh/kubernaut/issues/1612) | No retry on LLM call errors/timeouts in KA's streaming/interactive path | Yes — any provider, any hosting mechanism |
| [#1613](https://github.com/jordigilh/kubernaut/issues/1613) | `maxTotalToolCalls` hard-aborts with no wrap-up turn | Yes — any less tool-call-efficient model |
| [#1614](https://github.com/jordigilh/kubernaut/issues/1614) | Second `max_tokens` truncation silently accepted as final content | Yes — any verbose/reasoning-heavy model |
| [#1615](https://github.com/jordigilh/kubernaut/issues/1615) | No context-window/history trimming for long investigations | Yes — any smaller-context or high-verbosity model |
| [#1616](https://github.com/jordigilh/kubernaut/issues/1616) | `phaseModels` doesn't support per-phase reasoning/effort overrides | 1.6.0-only feature gap (reasoning config doesn't exist at v1.5.2 at all) |
| [#1618](https://github.com/jordigilh/kubernaut/issues/1618) | AF severity triage doesn't support `openai`/`openai_compatible` provider | Yes — present on both v1.5.2 and `main`; not backend-version-specific |

#1612-#1616 are scoped to `main`/1.6.0 — none apply as "fixes" to v1.5.2,
since the underlying code (`openaicompat` client, reasoning config) doesn't
exist at this tag. #1618 is different: it's a gap in AF's severity-triage
provider dispatch that exists identically on both v1.5.2 and `main`, so a
fix there is a real candidate for backporting once it lands. They're listed
here so pilot operators know these failure modes are recognized and being
addressed upstream, not silently ignored.

---

## 7. Capability comparison vs. Claude Sonnet 4.6 (2026-07-19)

This section captures a capability-comparison exercise run against Kubernaut's own
golden-transcript corpus and the closest available real-world analog benchmarks. It
exists to justify, with evidence, why `claude-sonnet-4-6` remains the recommended
default for production investigation and gpt-oss-120b stays scoped to a pilot / air-
gapped tier. Unlike §1-§6, none of this is version-specific to v1.5.2 — it applies to
the model-selection decision regardless of which KA release is running.

### 7.1 Zero empirical validation exists for gpt-oss-120b on Kubernaut's own scenarios

All 91 golden transcripts in `kubernaut-demo-scenarios/golden-transcripts/` — every
scenario, every archived rerun back to v1.3.2 — were captured on `claude-sonnet-4-6`.
None were captured on gpt-oss-120b or any other model. No eval harness exists yet
(tracked: [#1622](https://github.com/jordigilh/kubernaut/issues/1622)) that would let a
candidate model be scored against these scenarios. Everything below is evidence-based
inference from analogous benchmarks, not a direct measurement of gpt-oss-120b on
Kubernaut's own workload — that remains the only way to get a certain answer.

### 7.2 Context window is not the constraint; agentic efficiency is

Computed directly from `traceStats`/`llmTrace` across the 90 non-empty golden
transcripts (all on claude-sonnet-4-6):

- Peak in-context size at any single turn tops out around 80K characters (~20-23K
  tokens at a ~3.5 chars/token estimate) — comfortably inside both gpt-oss-120b's
  131,072-token window and Sonnet's. KA does not trim conversation history across turns
  (#1615 above), but at this workload's actual scale that gap hasn't been exposed yet.
  Context capacity is not a differentiator for this workload — an earlier pass at this
  analysis assumed otherwise by conflating cumulative output-token spend with
  context-window pressure; the corrected number is the one above.
- Total LLM output tokens generated per investigation average 182K (median 175K, max
  430K for `memory-escalation-containeroomkilling`) — this is the real cost driver, not
  a context-window risk (see §7.5).
- Investigations average 17 turns / 21 tool calls; 5 of 90 scenarios already exceed
  KA's default `maxTotalToolCalls: 30` using Sonnet 4.6's own efficiency (up to 55 tool
  calls / 39 turns in the worst case, `memory-escalation-containeroomkilling`). A less
  agentically-efficient model needing more tool calls to reach the same conclusion would
  hit this ceiling — and the no-wrap-up hard-abort bug (#1613) — more often than Sonnet
  already does.

### 7.3 gpt-oss models cannot batch tool calls — architectural, not configurable

Sonnet 4.6 actively batches tool calls within a single turn in real golden transcripts
(e.g. 3, 3, and 2 simultaneous tool calls within one investigation, per the raw
`llmTrace` event log) — a real, exercised efficiency strategy, not theoretical. gpt-oss
models cannot do this at all: the tool-call token (`<|call|>`) is defined as a stop/EOS
token in the gpt-oss chat template, so generation halts the instant one tool call is
emitted. A vLLM maintainer confirmed this directly on the BFCL (Berkeley Function
Calling Leaderboard) tracker — gpt-oss-120b scores 0% on BFCL's parallel-call
subcategories because "gpt-oss-120b was never meant for parallel function call outputs"
([gorilla#1146](https://github.com/ShishirPatil/gorilla/issues/1146)).

**Consequence for any pilot**: raising `maxTotalToolCalls` alone does not give
gpt-oss-120b a fair shot at matching Sonnet's efficiency — it needs roughly one turn per
tool call where Sonnet needs one turn per 1-3, so the separate `maxTurns` ceiling
(default 40) would bind for it sooner than it does for Sonnet today. Raise both
ceilings together, not just the one this document's §3 already calls out.

### 7.4 Closest real-world analog: HolmesGPT's own published Kubernetes RCA evals

HolmesGPT (the CNCF-sandbox agentic K8s troubleshooting tool Kubernaut's own deprecated
HolmesGPT-API service was built on) continuously evaluates models on real K8s/cloud
troubleshooting scenarios — the closest available real-world analog to Kubernaut's
investigation task, run by a different team on a different harness
(holmesgpt.dev/development/evaluations/history/).

**n=525, judged by gpt-4o (2025-09-30):**

| Category | gpt-4o | gpt-4.1 | gpt-5 | sonnet-4 | sonnet-4-5 |
|---|---|---|---|---|---|
| chain-of-causation (five-whys style) | 0% | 3% | 40% | 63% | 70% |
| hard scenarios | 11% | 29% | 57% | 77% | 80% |
| kubernetes (tag) | 55% | 71% | 69% | 89% | 87% |
| Overall | 63% | 74% | 78% | 89% | 89% |

"chain-of-causation" is functionally the same skill as Kubernaut's five-whys RCA
requirement (`incident_investigation.tmpl`, ADR-041), and it's where the gap to Sonnet
is widest for every tested OpenAI model — including OpenAI's own flagship at the time.

**n=16 "Frontier 5 Models" fast-benchmark, judged by gpt-4.1 (2026-03-15)** — this run
includes open-weight/self-hostable models directly comparable in deployment model to
gpt-oss-120b (see §7.6 for whether any of these are actually comparable in capability):

| Model | Overall | Avg cost/run | Avg latency | Deployment |
|---|---|---|---|---|
| opus-4.6 | 100% (16/16) | $0.32 | 42.0s | Proprietary API |
| sonnet-4.6 | 100% (16/16) | $0.18 | 35.2s | Proprietary API |
| deepseek-r1-reasoner | 88% (14/16) | $0.02 | 304.2s | Open-weight, self-hostable |
| gemini-3.1-pro-preview | 88% (14/16) | $0.12 | 38.0s | Proprietary API |
| deepseek-v3.2-chat | 81% (13/16) | $0.02 | 188.1s | Open-weight, self-hostable |
| gpt-5.4 | 81% (13/16) | $0.13 | 47.9s | Proprietary API |
| qwen-next-80B-instruct | 75% (12/16) | $0.04 | 32.2s | Open-weight, self-hostable |
| qwen-next-80B-thinking | 44% (7/16) | $0.03 | 48.1s | Open-weight, self-hostable |
| gpt-5.3-codex | 38% (6/16) | $0.03 | 16.2s | Proprietary API |

No gpt-oss-120b-specific HolmesGPT run was found in public results. This run still
supports two points directly relevant to the gpt-oss-120b question: (1) even the
best-performing open-weight/self-hostable model tested (`deepseek-r1-reasoner`, 88%)
trails Sonnet/Opus's 100% and does so at roughly 9x the latency; and (2) HolmesGPT's own
published summary for this run states "gpt-5.3-codex and qwen-next-80B-thinking tended
to ask for additional context instead of proceeding with the investigation" — a
specific failure mode (excessive hesitation instead of autonomous tool-driven
investigation) that directly parallels the "weaker agentic reliability" risk this
document already flags for gpt-oss-120b in §5, now with a documented precedent from a
comparable real-world agentic-investigation harness.

### 7.5 Estimated cost per investigation

Using Sonnet 4.6's real average/worst-case output-token spend from the golden
transcripts (§7.2) against list pricing:

| Scenario | Output tokens | Sonnet 4.6 ($15/M) | gpt-oss-120b hosted (~$0.60/M) |
|---|---|---|---|
| Average investigation | 182,152 | $2.73 | $0.11 |
| Worst case (`memory-escalation-containeroomkilling`) | 430,467 | $6.46 | $0.26 |

gpt-oss-120b self-hosted via vLLM has no per-token fee at all beyond GPU infrastructure
(fits a single H100 per OpenAI's model card). This remains the strongest argument in
gpt-oss-120b's favor, and the reason it's still worth piloting for an air-gapped or
cost-constrained tier — not a reason to dismiss it outright.

### 7.6 Recommendation

Keep `claude-sonnet-4-6` as the default for production investigation — it is the only
model with any empirical validation against Kubernaut's own scenario corpus, and the
closest real-world analog (HolmesGPT) shows a consistent, non-trivial gap for every
GPT-family model tested, widest on causal-chain reasoning specifically. No open-weight
model tested by HolmesGPT closed that gap either, and the two "thinking"/reasoning-
tuned entrants in the March 2026 run (`qwen-next-80B-thinking`, `gpt-5.3-codex`) did
notably worse than their non-reasoning siblings due to excessive hesitation — a
cautionary data point for any self-hosted reasoning model, including gpt-oss-120b.

Continue to treat gpt-oss-120b (and any self-hosted/open-weight model) as a pilot
candidate for an air-gapped or cost-constrained tier only, gated on: raising
`maxTotalToolCalls` **and** `maxTurns` together (§7.3), upgrading to KA 1.6.0+ for
reasoning capture, keeping 100% human review regardless of self-reported confidence
(§5), and — before extending any trust threshold — building the #1622 eval harness to
measure actual RCA accuracy against the golden-transcript corpus rather than relying on
analogous benchmarks alone.

---

## 8. Kimi K2 family (added 2026-07-19 — no HolmesGPT or Kubernaut-specific run exists)

Moonshot AI's Kimi K2 (and the 2026 K2.5 / K2.6-Thinking updates) is a genuinely
open-weight (modified MIT license), agentic-native MoE model. Unlike gpt-oss, it does
**not** have a parallel-tool-call architectural ceiling: Moonshot's own paper states
"we support parallel tool calling by placing multiple tool calls in a single response
turn" ([arxiv.org/abs/2507.20534](https://arxiv.org/abs/2507.20534)), and Cloudflare
Workers AI's Kimi K2.5 API reference lists `parallel_tool_calls: true` as the default.
This is the same efficiency strategy Sonnet 4.6 actually uses in the golden transcripts
(§7.3) — the one gpt-oss-120b structurally cannot do.

No HolmesGPT run and no Kubernaut-scenario run exists for any Kimi variant. The numbers
below are general-purpose agentic/tool-use benchmarks, not K8s-RCA-specific, and are
correspondingly weaker evidence than the HolmesGPT table in §7.4:

| Model | Benchmark | Score | Source |
|---|---|---|---|
| Kimi-K2-Instruct (Jul 2025) | BFCL v3 | 59.1% | Official, evals.report |
| Kimi-K2-Instruct | τ²-Bench | 66.1 Pass@1 | Moonshot's own paper |
| Kimi-K2-Instruct | ACEBench (En) | 76.5 | Moonshot's own paper |
| Kimi-K2-Instruct | SWE-bench Verified | 65.8% | Moonshot's own paper |
| Kimi K2.5 Thinking | BFCL v4 | 68.3 (rank 6/18) | Third-party aggregator (voidsource.dev) |
| Kimi K2.6 Thinking | τ²-bench average | 72.4 (rank 22/37) | Third-party aggregator (voidsource.dev) |

Self-hosting footprint is the main practical obstacle: Kimi K2 is a 1T-total /
32B-active MoE model — roughly 8x gpt-oss-120b's 116.8B total parameters. gpt-oss-120b's
whole appeal for the pilot in §1-6 was fitting on a single 80GB H100; Kimi K2 needs a
multi-GPU cluster even quantized, a materially heavier deployment ask than anything
else discussed in this document.

KA already supports both a native `gemini` provider and an `openai_compatible` provider
(`pkg/shared/types/llm.go`) alongside `anthropic` and `openai`. The same
`openai_compatible` path already proven for gpt-oss-120b in §1-6 works unmodified for
Kimi's (or DeepSeek's, or Qwen's) OpenAI-compatible chat-completions API, whether
self-hosted or via Moonshot's own hosted API. **No new KA code is required** to trial
any of these candidates — the cost is model-validation and, if self-hosting, GPU
infrastructure, not integration engineering.

---

## 9. Action plan: ranking non-Anthropic candidates for Kubernaut (2026-07-19)

This ranking is inference from public/third-party benchmarks plus one small (n=16,
1 iteration) HolmesGPT regression run — not a Kubernaut measurement. Treat it as a
prioritized shortlist for the #1622 eval harness to validate, not as a decision
already made. `claude-sonnet-4-6` remains the production default per §7.6; this
section is about ordering the non-Anthropic follow-up work.

### 9.1 Full candidate table

| Model | Type | K8s-RCA proxy (HolmesGPT, §7.4) | Agentic benchmark (§8, where no proxy exists) | Parallel tool calls | Self-host footprint | Avg latency | KA integration path |
|---|---|---|---|---|---|---|---|
| gemini-3.1-pro-preview | Proprietary API | 88% (14/16) | — | Yes | N/A (API-only) | 38.0s | Native `gemini` provider — zero new code |
| deepseek-r1-reasoner | Open-weight | 88% (14/16) | — | Not confirmed | Heavy (~671B-class) | 304.2s | `openai_compatible` |
| gpt-5.4 | Proprietary API | 81% (13/16) | — | Yes | 47.9s | Native `openai` provider |
| deepseek-v3.2-chat | Open-weight | 81% (13/16) | — | Not confirmed | Heavy (~671B-class) | 188.1s | `openai_compatible` |
| qwen-next-80B-instruct | Open-weight | 75% (12/16) | — | Yes (Qwen family default) | Moderate (~80B) | 32.2s | `openai_compatible` |
| kimi-k2 / k2.5-thinking | Open-weight | No run | BFCL v3 59%, τ²-bench 66, ACEBench 76.5 | Yes (confirmed, §8) | Very heavy (1T total / 32B active) | Unknown | `openai_compatible` |
| gpt-oss-120b | Open-weight | No run | BFCL parallel-call subcategory: 0% (architectural) | No — stop-token limitation (§7.3) | Light (single H100; 116.8B total / 5.1B active) | Unknown | `openai_compatible` (documented, §1-6) |
| qwen-next-80B-thinking | Open-weight | 44% (7/16) — avoid | — | Yes | Moderate (~80B) | 48.1s | `openai_compatible` |
| gpt-5.3-codex | Proprietary API | 38% (6/16) — avoid | — | Yes | 16.2s | Native `openai` provider |

### 9.2 Tiered recommendation

**Tier 1 — pilot next, via hosted API (no infra investment yet):**

1. **deepseek-r1-reasoner** — closest RCA-proxy accuracy to Sonnet (88% vs. Sonnet's
   100%) of every non-Anthropic model HolmesGPT tested, open or closed, *and* the
   most implementation-ready of any candidate here today (§10.2: it's the only one
   with bespoke reasoning/effort compatibility already in KA's code). The 9x-Sonnet
   latency (304s vs. 35s average) is the single biggest risk — validate against KA's
   own turn/timeout budget before any further investment. If it blows KA's
   interactive SLA, this candidate is disqualified regardless of accuracy. Note also
   (§10.4) that its 88% is not statistically distinguishable from qwen-instruct's 75%
   at n=16 — treat the accuracy gap between them as a hypothesis, not a settled fact.
2. **kimi-k2.5-thinking** (or plain kimi-k2) — best architectural fit of any
   candidate: genuinely agentic-native training plus confirmed parallel tool-call
   support, unlike gpt-oss, which KA's investigator loop is already built to exploit
   in parallel via `errgroup` with zero new code (§10.1). No K8s-RCA-analog
   validation exists yet, so this is a pure hypothesis pending the eval harness. It
   also currently falls to KA's reasoning-compatibility floor same as gpt-oss
   (§10.2) — reasoning captured but never replayed — until it gets the same
   bespoke treatment DeepSeek already has. Test via Moonshot's own hosted API
   first — this decouples "is the model good enough" from "can we afford to
   self-host a 1T-parameter model," and needs zero new KA code
   (`openai_compatible`).

**Tier 2 — keep piloting, tightly gated (infrastructure already exists):**

3. **gpt-oss-120b** — the lightest self-host footprint of any open-weight candidate
   (single 80GB H100) and the only one with an existing documented KA pilot path
   (§1-6). Its tool-call-batching ceiling is architectural and permanent (§7.3), so
   keep treating it as a cost-constrained/air-gapped-tier candidate, not a
   Sonnet-parity target — §7.6's recommendation stands unchanged.

**Tier 3 — fallback only:**

4. **qwen-next-80B-instruct** — cheapest and fastest of the open-weight field
   ($0.04/run, 32.2s) but the largest accuracy gap among non-"thinking" candidates
   (75%). Reasonable if cost/latency dominates the decision and the accuracy gap is
   explicitly tested and accepted, not assumed away.

**Do not pilot for this workload:**

5. **qwen-next-80B-thinking** and **gpt-5.3-codex** — both scored worst in
   HolmesGPT's own run specifically because they "tended to ask for additional
   context instead of proceeding with the investigation" (§7.4). An autonomous
   investigation pipeline that stalls waiting for clarification is a worse failure
   mode than a wrong answer for Kubernaut's use case — this disqualifies both
   regardless of their other benchmark scores.

### 9.3 Sequencing

1. Build the #1622 eval harness first. Every ranking above is third-party inference;
   nothing here should drive an infrastructure decision until it's measured against
   Kubernaut's own golden-transcript corpus.
2. Run the harness against a representative sample (10-15 scenarios spanning easy,
   hard, and chain-of-causation cases) for `deepseek-r1-reasoner` and
   `kimi-k2.5-thinking` via their hosted APIs — zero new KA code, zero GPU
   procurement, fastest available signal.
3. Only invest in self-hosting infrastructure for whichever candidate clears
   Kubernaut's own bar in step 2 — this avoids committing GPU budget ahead of a
   quality gate.
4. Continue the existing gpt-oss-120b pilot in parallel (§1-6), since that
   infrastructure already exists, but keep expectations capped per §7.6.

---

## 10. Confidence-building without running new evals (2026-07-19)

§9's ranking is built entirely on third-party benchmarks and one small HolmesGPT run.
Before spending eval-harness or GPU budget on it, four zero-model-inference techniques
were applied to tighten (and in one case, correct) that confidence — none of them
required calling any of the candidate models.

### 10.1 Architecture verification: KA already executes multi-tool-call turns concurrently

`internal/kubernautagent/investigator/investigator_loop.go`'s `processToolCalls`
already dispatches every tool call in a turn concurrently via `errgroup.Group`:

```286:309:internal/kubernautagent/investigator/investigator_loop.go
func (inv *Investigator) processToolCalls(ctx context.Context, messages []llm.Message, resp llm.ChatResponse, turn int, phase string, correlationID string) (newMessages []llm.Message, sentinel LoopResult, budgetExhausted bool) {
	...
	toolResults := make([]string, len(resp.ToolCalls))
	var g errgroup.Group
	for i, tc := range resp.ToolCalls {
		...
		g.Go(func() error {
			toolResults[i] = inv.executeTool(ctx, tc.Name, json.RawMessage(tc.Arguments))
			return nil
		})
	}
	_ = g.Wait()
```

This upgrades the Kimi/DeepSeek parallel-tool-call claim (§7.3, §8) from "the model can
theoretically do this" to "KA will correctly exploit it today, with zero new code" —
a real confidence increase, for the cost of one file read.

### 10.2 New risk found the same way: Kimi and Qwen currently share gpt-oss's reasoning-capture gap

`pkg/shared/llm/openaicompat/reasoning.go`'s `DetectReasoningMode` and
`DetectEffortDialect` only special-case DeepSeek by name
(`deepseek-reasoner`/`deepseek-r1`/`deepseek-v4`) and real OpenAI o-series/gpt-5-family
models. DD-LLM-005 confirms DeepSeek is the only non-Anthropic/OpenAI model with
bespoke compatibility code in KA today. Everything else — including Kimi and Qwen —
falls to the compatibility floor (`ReasoningModeNone` / `EffortDialectNone`): no
reasoning-effort control, and reasoning content is captured for display but never
replayed on later turns. That is the exact "reasoning silently dropped" gap already
flagged for gpt-oss-120b elsewhere in this document (§7, DD-LLM-005) — Kimi and Qwen
have it too, and DeepSeek-R1 currently does not. Net effect on §9.2's ranking:
**deepseek-r1-reasoner is the most implementation-ready non-Anthropic candidate today**,
independent of its accuracy score.

### 10.3 Wire-format tool-call compatibility is already solved at the serving-stack layer

KA's `openaicompat.mapResponse` / `buildFinishResponse` expect the standard OpenAI wire
shape — `choices[].message.tool_calls[]` (`id`, `type`, `function.name`,
`function.arguments`) plus a `reasoning_content` string. vLLM, the serving stack this
document's pilot is built on, ships a dedicated, actively-maintained tool-call parser
per model family that translates each model's native output into exactly that shape:

| Model family | vLLM `--tool-call-parser` | Extra requirement |
|---|---|---|
| gpt-oss | `openai` (`GptOssToolParser`) | None beyond the flag |
| DeepSeek-V3 / R1 | `deepseek_v3` / `deepseek_v31` / `deepseek_v32` | Also needs an explicit `--chat-template` file (e.g. `tool_chat_template_deepseekr1.jinja`) |
| Kimi-K2 | `kimi_k2` (`KimiK2ToolParser`) | None beyond the flag |
| Qwen3-Next / Qwen3-Coder | `qwen3_xml` / `qwen3_coder`; older Qwen2-based models use `hermes` | None beyond the flag |

KA's own tool schemas (e.g. `pkg/kubernautagent/tools/prometheus/tools.go`) are already
in the most portable form for this: flat JSON-Schema objects with
`additionalProperties: false`, and KA never requests OpenAI's optional `strict` mode —
the same compatibility-floor principle §7.3/DD-LLM-005 apply elsewhere. Net effect:
**wire-level tool-call compatibility is a solved, already-maintained problem for all
four candidates** — a vLLM launch-flag choice, not new Kubernaut engineering work. This
derisks "will it even wire up," but says nothing about accuracy — that is still the
eval harness's job, not this check's.

### 10.4 Statistical rigor on the HolmesGPT n=16 numbers already in §7.4/§9.1

The n=16, single-iteration run behind §9.1's ranking has much wider uncertainty than
the raw percentages suggest. 95% Wilson confidence intervals:

| Model(s) | Score | 95% CI |
|---|---|---|
| opus-4.6 / sonnet-4.6 | 100% (16/16) | 80.6% – 100% |
| deepseek-r1-reasoner / gemini-3.1-pro-preview | 88% (14/16) | 64.0% – 96.5% |
| deepseek-v3.2-chat / gpt-5.4 / haiku-4.5 | 81% (13/16) | 57.0% – 93.4% |
| qwen-next-80B-instruct | 75% (12/16) | 50.5% – 89.8% |
| qwen-next-80B-thinking | 44% (7/16) | 27.1% – 70.8% |
| gpt-5.3-codex | 38% (6/16) | 18.5% – 61.4% |

Two things fall out for free: (1) the top four rows overlap enough that "88% beats 75%"
should be read as a rough cluster, not a settled ranking — §9.2's Tier-1/Tier-3
ordering between `deepseek-r1-reasoner` and `qwen-next-80B-instruct` is weaker than it
looks; (2) the "avoid" tier is robust even after widening for uncertainty —
`qwen-next-80B-thinking` and `gpt-5.3-codex`'s upper bounds (70.8%, 61.4%) barely reach
the top cluster's lower bound (80.6%), so that part of §9.2 survives the stress-test.

### 10.5 What these techniques cannot do

None of the above generates ground truth on RCA accuracy for Kubernaut's actual
workload — they reduce wiring/format risk (§10.1, §10.3), surface implementation-
readiness gaps (§10.2), and correct false precision in already-published numbers
(§10.4). Two further techniques were identified but not yet executed, for future
follow-up if deeper confidence is needed before committing to §9.3's sequencing:
fetching a larger-N HolmesGPT "Full" benchmark run (150+ scenarios vs. this document's
n=16) for the same models, and fitting a correlation between general agentic
benchmarks (BFCL/τ²-bench) and HolmesGPT's K8s score across the models where both exist,
to project a credible interval for Kimi's untested K8s-RCA performance instead of
treating its BFCL/τ²-bench numbers as directly comparable.

---

## Related documents

- [`docs/services/kubernaut-agent/configuration-reference.md`](../../../services/kubernaut-agent/configuration-reference.md) — authoritative KA config reference (1.6.0/`main`)
- [`docs/services/apifrontend/configuration-reference.md`](../../../services/apifrontend/configuration-reference.md) — authoritative AF config reference (1.6.0/`main`)
- [DD-LLM-004](../../../architecture/decisions/DD-LLM-004-langchaingo-removal-generalized-clients.md) — why `langchaingo` was removed in 1.6.0
- [DD-LLM-005](../../../architecture/decisions/DD-LLM-005-model-aware-reasoning-support.md) — model-aware reasoning support design (BR-AI-086)
- [DD-LLM-008](../../../architecture/decisions/DD-LLM-008-restart-required-llm-identity-lock.md) — restart-required LLM identity lock (1.6.0-only)
- `kubernaut-demo-scenarios/golden-transcripts/` (sibling repo, not part of this git tree) — the 91-scenario corpus §7 is computed from
- [HolmesGPT evaluation history](https://holmesgpt.dev/development/evaluations/history/) — external, third-party real-world K8s RCA benchmark cited in §7.4
- [BFCL / gpt-oss parallel tool-call issue](https://github.com/ShishirPatil/gorilla/issues/1146) — external confirmation of the architectural limitation cited in §7.3
- [Kimi K2: Open Agentic Intelligence](https://arxiv.org/abs/2507.20534) — Moonshot AI's own paper, cited in §8 for parallel tool-call support and agentic benchmark scores
