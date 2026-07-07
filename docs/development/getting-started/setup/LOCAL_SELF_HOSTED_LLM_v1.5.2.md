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

## Related documents

- [`docs/services/kubernaut-agent/configuration-reference.md`](../../../services/kubernaut-agent/configuration-reference.md) — authoritative KA config reference (1.6.0/`main`)
- [`docs/services/apifrontend/configuration-reference.md`](../../../services/apifrontend/configuration-reference.md) — authoritative AF config reference (1.6.0/`main`)
- [DD-LLM-004](../../../architecture/decisions/DD-LLM-004-langchaingo-removal-generalized-clients.md) — why `langchaingo` was removed in 1.6.0
- [DD-LLM-005](../../../architecture/decisions/DD-LLM-005-model-aware-reasoning-support.md) — model-aware reasoning support design (BR-AI-086)
- [DD-LLM-008](../../../architecture/decisions/DD-LLM-008-restart-required-llm-identity-lock.md) — restart-required LLM identity lock (1.6.0-only)
