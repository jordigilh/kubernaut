# DD-PLATFORM-007: LLM Profile Consolidation (Helm Chart)

**Date**: July 25, 2026
**Status**: 🟢 **IMPLEMENTED** (amended 2026-08-02 by kubernaut#1861 — see Addendum)
**Decision Date**: 2026-07-25
**Version**: 1.3
**Confidence**: 92%
**Deciders**: Kubernaut Platform (chart maintainers)
**Applies To**: `charts/kubernaut` (Helm chart); originally no Go code
changes (every consumption contract this DD wires already existed and was
already validated in `cmd/kubernautagent`, `cmd/apifrontend`,
`pkg/apifrontend/config`, `internal/kubernautagent/config`) — the
kubernaut#1861 amendment additionally touched `pkg/apifrontend/severity`,
`cmd/apifrontend`, and `pkg/apifrontend/launcher` to lift the #1731 guard
(see Addendum)

**Related Business Requirements**:
- BR-PLATFORM-008: Helm Chart LLM Configuration Parity

**Related Design Decisions**:
- DD-PLATFORM-004 / DD-PLATFORM-006: the shared-default/per-consumer-
  override pattern (`kubernaut.affinity`, `global.podDefaults`,
  `global.defaultResources`) this DD generalizes to LLM identity via named
  profiles instead of a shared literal default (LLM identity has no
  sensible "default" the way pod scheduling does — every consumer must
  name a real profile).
- DD-LLM-008: Restart-required LLM identity lock. **Unaffected by this
  DD** — it governs KA's *runtime* hot-reload behavior after the chart
  has already rendered `llm-runtime.yaml`; this DD only changes how that
  same file gets assembled at `helm template`/`helm upgrade` time.
- DD-LLM-007: AF/KA Anthropic client divergence — relevant background for
  why AF's severity-triage dispatch (`cmd/apifrontend/backend_deps.go`)
  has separate Anthropic-direct and Anthropic-on-Vertex code paths this DD
  must render config for correctly.

**Related Issues** (tracked separately, cross-referenced):
- #1589: Helm/Operator parity triage (same family as this DD's AF
  `agent.llm`/`severityTriage.llm` findings).
- #1731: vertex_ai's ambient-Application-Default-Credentials constraint —
  a profile and any consumer resolving to it must share
  `credentialsSecretName` when both are `vertex_ai`. This DD originally
  enforced it with a `fail()` guard; **lifted in kubernaut#1861** (see
  Changelog v1.3 and the Addendum below) once both AF severityTriage
  Vertex constructors stopped depending on the shared ambient env var.
- #1870 / #1861: the release/v1.5 and main-line fixes (respectively) that
  made the #1731 constraint liftable, by giving AF's severityTriage
  Claude-on-Vertex and Gemini-on-Vertex constructors their own explicit,
  independent credentials.
- #1599: restart-required identity lock (see DD-LLM-008).
- #1582: `bedrock` provider not yet wired in either client dispatch —
  stays schema-rejected, unchanged by this DD.

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-07-25 | Platform (AI-assisted analysis) | Initial draft. User selected Alternative A (full profile-map mirror of the Operator) after a scoped comparison against Alternative B (minimal, KA-only fix) and Alternative C (defer). Scope grew during preflight beyond the original "reduce duplication" framing once the actual Go consumption contracts were inspected: found one dead config field (`alignmentCheck.llm.apiKey` vs. the Go struct's `apiKeyFile`) and two fully-implemented, zero-Helm-exposure Go capabilities (AF's `agent.llm`, AF's `severityTriage.llm`). |
| 1.1 | 2026-07-25 | Platform (AI-assisted analysis) | User confirmed keeping AF wiring in scope (no split), then requested a confidence assessment. Resolved 3 of 4 previously-flagged unknowns by reading the Operator's reconciler directly (`kubernaut-operator/internal/resources/configmaps.go`/`deployments.go`/`common.go`, not just the CRD types): (1) confirmed `phaseModels` override field subset excludes `azureApiVersion`/`bedrockRegion` even though the Go struct has them — corrected the Resolution Mechanism section to match the Operator's actual rendering, not the theoretical max; (2) confirmed and specified the exact multi-secret-mount convention (`/etc/kubernaut-agent/phase-credentials/<phase>/{api_key\|credentials.json}`, inherit-if-same-secret optimization) — verified this chart's current KA Deployment volume list is static, so this is genuinely new template logic, not an extension of an existing loop; (3) **corrected** the #1731 vertex_ai constraint's scope from "any phase/severity-triage override" to **AF-specific only** (`apifrontend.llmProfileRef` vs. `apifrontend.severityTriage.llmProfileRef`) — KA's Vertex client is architecturally different (DD-LLM-007) and has no equivalent limitation; the original v1.0 draft had over-generalized this. Confidence raised 78%→86% (verified design detail, but multi-secret-mount Sprig logic remains genuinely first-of-its-kind for this chart — still below the 90% "proceed directly" gate). |
| 1.2 | 2026-07-25 | Platform (AI-assisted analysis) | User chose the time-boxed spike option at 86% confidence. Ran a 2-hour-budget spike (isolated throwaway chart, not committed) prototyping exactly the multi-secret-mount mechanism against `helm template` — see "Spike Findings" section. Result: **YES**, achievable with plain Sprig, no exotic constructs; `rca`/`workflow_discovery`/`validation` test case rendered correctly (inherit-if-same-secret, per-consumer-vs-base dedup, provider-specific credential filename, deterministic `sortAlpha` ordering, both `fail()` guards firing correctly). Confidence raised 86%→92% — the single highest-risk mechanism is now proven, not just designed; remaining work is integration effort into the real chart, not open uncertainty. |
| 1.3 | 2026-08-02 | Platform (AI-assisted analysis) | **Amendment (kubernaut#1861)**: the AF-scoped `fail()` guard this DD introduced for #1731 has been removed. It existed because `cmd/apifrontend`'s two severityTriage Vertex constructors (`newAnthropicTriagerForVertex`, `newGenAITriagerForVertex`) relied solely on the process-wide `GOOGLE_APPLICATION_CREDENTIALS` ambient-ADC env var, making two independent vertex_ai profiles (AF's own + severityTriage's) unable to coexist with different `credentialsSecretName` values. #1861 (mirroring release/v1.5's #1870) fixed both constructors to resolve credentials explicitly from each profile's own mounted `apiKeyFile`/`apiKey` instead — the same technique AF's own `agent.llm` Gemini-on-Vertex path already used (#1801's `newVertexGeminiModel`). This is now a Go-code-affecting DD (superseding this document's original "no Go code changes" scope note for the vertex_ai case only); everything else in this DD's Wiring Manifest remains a chart-only change. See "Addendum: kubernaut#1861" below for full rationale, including the one remaining ambient-ADC dependency this fix does *not* remove. |

---

## Context & Problem

### Current State (verified against Go source, not just the chart)

**Kubernaut Agent (KA)** — `internal/kubernautagent/config/config_types.go`:
- `LLMRuntimeConfig` (hot-reloadable; maps to the `kubernaut-agent-llm-runtime`
  ConfigMap): `model`, `endpoint`, `apiKeyFile`, `temperature`, `maxRetries`,
  `timeoutSeconds`, `customHeaders`, **`phaseModels`** (map of phase name ->
  `*LLMOverrideConfig`). The Helm chart renders every field in this struct
  **except `phaseModels`** — there is no `values.schema.json` path to set
  it, even though `EffectivePhaseConfig` and `ValidPhaseNames`
  (`rca`/`workflow_discovery`/`validation`) are fully implemented and
  covered by DD-LLM-008's identity-lock design.
- `AlignmentCheckConfig.LLM *LLMOverrideConfig` (static, never hot-reloaded
  per DD-LLM-008): fields `provider`, `endpoint`, `model`, `apiKeyFile`,
  `azureApiVersion`, `vertexProject`, `vertexLocation`, `bedrockRegion`,
  `reasoning`. **The Helm chart's `alignmentCheck.llm.apiKey` field does
  not exist in this struct** — `templates/kubernaut-agent/kubernaut-agent.yaml`
  renders `apiKey: {{ .apiKey }}` into a struct whose YAML tag is
  `apiKeyFile`. This field has had no effect on any Helm-chart deployment
  since it was added.

**API Frontend (AF)** — `pkg/apifrontend/config/config.go`:
- `Config.Agent.LLM types.LLMConfig` (required, non-pointer) — AF's own
  agent-loop LLM connection. `Validate()` no-ops when `Provider == ""`, so
  today's Helm-deployed AF boots fine, but this feature is **silently
  inert**: `templates/apifrontend/apifrontend.yaml`'s `agent:` block
  renders only `kaBaseURL`/`dsBearerTokenFile` — no `llm:` sub-key exists
  in `values.schema.json` for `apifrontend.agent.llm` at all.
- `Config.SeverityTriage.LLM *types.LLMConfig` (optional) — feeds
  `cmd/apifrontend/backend_deps.go`'s `newLLMTriagerFromConfig`, which
  fully dispatches to Anthropic-direct, Anthropic-on-Vertex, Gemini, or
  OpenAI-compatible triagers depending on `Provider`/`Model`. Same gap:
  `apifrontend.yaml`'s `severityTriage:` block renders only
  `cacheTTLSeconds`/`llmConfidence` — no `llm:` sub-key exists at all.
  **Both AF capabilities are fully implemented, fully validated
  (`validateLLM`, `validateSeverityTriage`), and fully unreachable via the
  Helm chart today.**

**The shared type** — `pkg/shared/types.LLMConfig` (used by both KA's
static config and AF's `Agent.LLM`/`SeverityTriage.LLM`): `provider`,
`model`, `endpoint`, `apiKeyFile`, `vertexProject`, `vertexLocation`,
`azureApiVersion`, `bedrockRegion`, `temperature`, `maxRetries`,
`timeoutSeconds`, `tlsCaFile`, `tlsCertFile`, `tlsKeyFile`, `oauth2`,
`circuitBreaker`, `customHeaders`, `reasoning`. This struct is
*already* the Go-native equivalent of the Operator's `LLMProfileSpec` —
both AF and KA consume the identical shape. A "profile" in Helm terms
is nothing more than a named instance of this struct, resolved at
`helm template` time instead of by the Operator's Go reconciler.

**The Operator's pattern** (`kubernaut-operator/api/v1alpha1/kubernaut_types.go`,
for reference — not being changed by this DD): `spec.llmProfiles:
map[string]LLMProfileSpec`, referenced by `kubernautAgent.llmProfileRef`
(required), `kubernautAgent.phaseModels` (map[phase]->profile name),
`apiFrontend.llmProfileRef` (optional, defaults to KA's),
`apiFrontend.severityTriage.llmProfileRef` (optional, defaults to AF's
resolved profile). Extensively validated in
`internal/resources/validation_test.go` (VL-001..027).

### Why this is a DD, not just an implementation task

This introduces a new indirection mechanism (named profile map + reference
field, resolved via new Helm template/helper logic) rather than
consolidating or removing existing knobs — a materially different kind of
change from DD-PLATFORM-006 (hence a separate DD per user decision), and
one that touches credential-Secret-mounting logic, which has real
correctness risk (a resolution bug here doesn't just render a
sub-optimally-shaped `values.yaml`, it can point a container at the wrong
credentials).

---

## Constraints

- **No backward-compatibility requirement** — same precedent as
  DD-PLATFORM-006 (pre-GA, #1725 already set this bar for this chart).
  `kubernautAgent.llm.*`/`alignmentCheck.llm.*` as literal blocks are
  replaced outright, not aliased.
- **No Go code changes.** Every field this DD renders must match an
  existing Go struct's YAML tag exactly (`apiKeyFile`, not `apiKey` — the
  one bug this DD is required to fix, not just avoid repeating).
- **Preserve DD-LLM-008's identity-lock semantics.** This DD only changes
  how `llm-runtime.yaml` and the static SDK config get assembled at
  render time; KA's hot-reload/identity-lock behavior at runtime is
  unaffected.
- **Enforce the #1731 vertex_ai constraint**: a consumer's resolved
  profile and any phase/severity-triage override profile must share
  `credentialsSecretName` when both use `vertex_ai` — `fail()` if violated.
- **Follow the existing shared-helper pattern** (`kubernaut.affinity`,
  `kubernaut.pdb`, `global.fleet.*`) for the resolution mechanism rather
  than inventing an unrelated idiom.

---

## Decision Drivers

1. One confirmed dead config field (`alignmentCheck.llm.apiKey`) that must
   be fixed regardless of the profiles decision.
2. Two confirmed, fully-implemented Go capabilities (AF `agent.llm`,
   `severityTriage.llm`) with zero current Helm exposure — a real parity
   gap, not speculative future-proofing.
3. One confirmed Go-supported mechanism (KA `phaseModels`) with zero
   current Helm exposure.
4. Direct user request and explicit choice (Alternative A) to mirror the
   Operator's reference-by-name pattern exactly, rather than a
   Helm-specific shape.

---

## Alternatives Considered

### Alternative A — Full profile-map mirror of the Operator ✅ CHOSEN (user decision)

`global.llmProfiles: map[string]<profile>` (profile shape = the
Helm-rendering-relevant subset of `pkg/shared/types.LLMConfig`'s fields:
`provider`, `model`, `credentialsSecretName`, `endpoint`, `temperature`,
`maxRetries`, `timeoutSeconds`, `vertexProject`, `vertexLocation`,
`azureApiVersion`, `tlsCaFile`, `oauth2.*`, `reasoning.*`).

References:
- `kubernautAgent.llmProfileRef` (required)
- `kubernautAgent.phaseModels: map[string]string` (phase -> profile name) — **new**
- `kubernautAgent.alignmentCheck.llmProfileRef` (optional; empty inherits KA's) — replaces the broken `apiKey` field
- `apifrontend.llmProfileRef` (optional; empty defaults to KA's) — **new**
- `apifrontend.severityTriage.llmProfileRef` (optional; empty inherits AF's resolved profile; unset/disabled = rule-based-only triage) — **new**

**Pros**: exact parity with the Operator's authoring model (the thing the
user explicitly asked for); fixes the `apiKey` bug as a natural
consequence of routing every consumer through the same resolution path
instead of ad hoc field lists; closes both AF parity gaps in the same
pass instead of as separate future work; a profile defined once is
trivially reusable by any future consumer without re-deriving its field
set.
**Cons**: the most implementation effort of the three alternatives —
requires new Helm-side resolution logic (a `_helpers.tpl` merge helper
mirroring `EffectiveLLM`/`EffectivePhaseConfig` field-for-field) and new
multi-Secret-volume-mounting logic (today's templates mount exactly one
LLM credentials Secret; profiles allow several distinct
`credentialsSecretName`s across KA/its phases/alignmentCheck/AF/AF's
severity-triage, each needing its own mount + `apiKeyFile` path). This is
new complexity for this chart, not a refactor of existing complexity.
**Confidence**: 78% (chosen) — lower than DD-PLATFORM-006's confidence
because the multi-secret-mounting mechanics and exact `phaseModels` Helm
UX are genuinely new ground for this chart, not a proven existing pattern
being extended.

### Alternative B — Minimal, scoped to today's actual KA-only duplication ❌ NOT CHOSEN

`kubernautAgent.llmProfiles: {primary: {...}, alignmentCheck: {...}}`,
`alignmentCheck.llmProfileRef` defaulting to `primary`. Fixes the `apiKey`
bug and the one real duplication that exists, but leaves both AF gaps
(`agent.llm`, `severityTriage.llm`) unaddressed — the smaller, safer
option, and would have been my default recommendation absent a specific
user preference for full parity.
**Confidence**: 90% for its own (narrower) scope — not chosen because it
doesn't achieve what the user asked for (mirror the Operator, not just
fix KA's local duplication).

### Alternative C — Defer entirely ❌ NOT CHOSEN

Track as its own future Helm/Operator parity item (#1589 family) and
revisit once a concrete near-term need surfaces. Rejected by the same
user decision that selected A — the AF gaps found during preflight make
"defer" a weaker case than it looked before that research, since two
real (if currently unused) Go capabilities sit completely unreachable via
Helm today.

---

## Decision

**Alternative A**, per explicit user selection (2026-07-25).

### Profile Schema (proposed)

```yaml
global:
  llmProfiles:
    <name>:
      provider: ""              # openai | anthropic | vertex_ai | openai_compatible
      model: ""
      credentialsSecretName: ""
      endpoint: ""
      temperature: 0.7
      maxRetries: 3
      timeoutSeconds: 120
      vertexProject: ""
      vertexLocation: ""
      azureApiVersion: ""
      tlsCaFile: ""
      oauth2:
        enabled: false
        tokenURL: ""
        credentialsSecretRef: ""
        scopes: ""
      reasoning:
        enabled: false
        budgetTokens: 0
        effort: ""
        capabilityOverride: ""
```

`tlsCertFile`/`tlsKeyFile`/`tlsClientSecretRef` (mTLS) and `customHeaders`
exist on `pkg/shared/types.LLMConfig` but are not exposed by **any**
current Helm consumer either — explicitly left out of this profile schema
(not a regression, since nothing renders them today); tracked as a future
addition only if a concrete need arises (no speculative fields).
`bedrockRegion` stays out for the same reason `provider: bedrock` stays
schema-rejected (#1582 — not wired in either client dispatch yet).

### Resolution Mechanism

A new `_helpers.tpl` entry (working name `kubernaut.llm.resolveProfile`)
takes `(profileName, overrideDict, $)` and returns the merged field set:
start from `global.llmProfiles[profileName]` (`fail()` if the name doesn't
exist — mirrors the Operator's VL-011/VL-014 validation), then apply any
non-empty override fields on top — the same "non-zero fields win" rule as
`EffectiveLLM`/`EffectivePhaseConfig`. Each existing render site
(`kubernaut-agent.yaml`'s static SDK config block, `llm-runtime.yaml`,
the new `apifrontend.yaml` `agent.llm`/`severityTriage.llm` blocks) calls
this helper instead of dereferencing a literal `.Values.<service>.llm.*`
block, and renders the exact same downstream YAML shape as today (field
names, ConfigMap/Secret split) — this DD does not change what KA/AF read,
only how the chart assembles it.

**`phaseModels` field subset (verified against the Operator's own
rendering code, `kubernaut-operator/internal/resources/configmaps.go`,
not just the max struct shape)**: a phase override renders only
`provider`, `model`, `endpoint`, `vertexProject`, `vertexLocation`,
`reasoning`, and a conditional `apiKeyFile` — **not** `azureApiVersion` or
`bedrockRegion`, even though `LLMOverrideConfig` (the Go struct) has both
fields. The Operator itself never populates them from a phase's resolved
profile. Helm's phase-override rendering mirrors this exact subset, not
the theoretical maximum — matching actual precedent rather than the
broadest-possible interpretation of the struct.

### Secret-Volume Mounting

Extended to mount one Secret volume per distinct `credentialsSecretName`
actually resolved across all referenced profiles for a given Deployment.
**Verified this is genuinely new logic for this chart**: today's KA
Deployment (`templates/kubernaut-agent/kubernaut-agent.yaml`) has a
*static*, hardcoded `volumes`/`volumeMounts` list — one fixed
`llm-credentials-file` entry pointing at
`.Values.kubernautAgent.llm.credentialsSecretName` — not a loop. This DD
converts that static entry into a `range` over the de-duplicated set of
`credentialsSecretName`s actually resolved across the base profile + any
phase overrides + alignmentCheck's profile, following the Operator's own
convention verified in `internal/resources/common.go`/`deployments.go`:
- Mount path: `/etc/kubernaut-agent/phase-credentials/<phase>/<file>`
  (base profile keeps today's `/etc/kubernaut-agent/credentials` path
  unchanged).
- Credential file name within the mount: `api_key` for every provider
  **except** `vertex_ai`, which uses `credentials.json` (JSON service-
  account key convention) — a real, easy-to-miss detail the Operator's
  `configmaps.go` encodes explicitly (`credFile := "api_key"` /
  `"credentials.json"` branch).
- **Optimization mirrored from the Operator**: a phase/consumer override
  only gets its own dedicated mount + `apiKeyFile` value when its resolved
  `credentialsSecretName` *differs* from the base profile's — if it
  matches, the override inherits the base's already-mounted path instead
  of mounting a redundant duplicate Secret.

**(Historical — superseded by kubernaut#1861, see Addendum below)** A
`fail()` guard originally enforced #1731 here — **scope corrected after
verifying `kubernaut-operator/internal/resources/validation.go`
directly**: this constraint was **AF-specific** (API Frontend's own
resolved profile vs. `severityTriage`'s resolved profile), not a general
KA/phaseModels rule. AF's Vertex AI clients relied solely on ambient
Application Default Credentials process-wide, so two different
`credentialsSecretName`s both resolving to `vertex_ai` within the same AF
process would silently collide on whichever ADC happens to be ambient.
KA's own Vertex client (`pkg/kubernautagent/llm/anthropicfamily/client.go`)
is architecturally different (DD-LLM-007) and supports explicit
per-instance credentials via `resolveADCAuth`, which is exactly why the
Operator's own validation only guarded AF, never KA's `phaseModels`.
kubernaut#1861 closed the same gap for AF's two severityTriage Vertex
constructors, so the guard is no longer needed for either KA or AF.

---

## Spike Findings (Pre-Implementation Workflow Step 2)

**Time-boxed spike, 2026-07-25, well under the 2-hour budget.** Built an
isolated, throwaway chart (`/tmp/llm-mount-spike`, outside this repo, not
committed) to prototype exactly the multi-secret-mount mechanism — the
single highest-risk, first-of-its-kind piece of this DD — against the
Operator's verified per-consumer-vs-base convention (no cross-consumer
dedup; sorted iteration for determinism; provider-specific credential
filename).

**Test values**: base profile `primary` (secret `llm-credentials-primary`)
+ three `phaseModels`: `rca` -> `primary` (same secret as base), 
`workflow_discovery` -> `lightweight`/`openai_compatible` (different
secret), `validation` -> `gcp-profile`/`vertex_ai` (different secret,
different provider than `workflow_discovery`'s, to prove no accidental
cross-phase collision).

**`helm template` output** (verified, not assumed):
```yaml
volumeMounts:
  - name: llm-credentials-file
    mountPath: /etc/kubernaut-agent/credentials
    readOnly: true
  - name: phase-credentials-validation
    mountPath: /etc/kubernaut-agent/phase-credentials/validation
    readOnly: true
  - name: phase-credentials-workflow_discovery
    mountPath: /etc/kubernaut-agent/phase-credentials/workflow_discovery
    readOnly: true
volumes:
  - name: llm-credentials-file
    secret: {secretName: llm-credentials-primary}
  - name: phase-credentials-validation
    secret: {secretName: llm-credentials-gcp}
  - name: phase-credentials-workflow_discovery
    secret: {secretName: llm-credentials-lightweight}
```
and the corresponding `llm-runtime.yaml`:
```yaml
phaseModels:
  rca: {provider: anthropic}                                              # no apiKeyFile — inherits base's mount, exactly as designed
  validation: {provider: vertex_ai, apiKeyFile: /etc/.../validation/credentials.json}
  workflow_discovery: {provider: openai_compatible, apiKeyFile: /etc/.../workflow_discovery/api_key}
```

**Confirmed**:
- `rca` (same secret as base) correctly gets **no** dedicated mount and
  **no** `apiKeyFile` — inherits the base path implicitly, matching the
  Operator's optimization exactly.
- `workflow_discovery` and `validation` (different secrets from base *and*
  from each other) each get their own mount with zero collision —
  confirms dedup is per-consumer-vs-base only, no cross-consumer logic
  needed, exactly as verified in the Operator's `deployments.go`.
- Provider-specific credential filename (`api_key` vs. `credentials.json`
  for `vertex_ai`) renders correctly.
- `{{ .Values.kubernautAgent.phaseModels | keys | sortAlpha }}` gives
  deterministic ordering (`rca`, `validation`, `workflow_discovery` —
  alphabetical), matching the Operator's `sort.Strings(phases)` intent,
  achieved with plain Sprig, no custom function needed.
- `fail()` guards for an undefined `llmProfileRef` and an undefined
  `phaseModels` entry both fire correctly with a clear error message and
  non-zero exit — verified via `helm template --set
  kubernautAgent.llmProfileRef=nonexistent` and the equivalent for
  `phaseModels`.

**Decision: YES.** The mechanism is achievable with plain Sprig
(`range`/`keys`/`sortAlpha`/`index`/`and`/`fail`) — no exotic template
gymnastics, no need for a `lookup` call or post-processing. The remaining
work is mechanical integration into the real chart's larger templates
(`kubernaut-agent.yaml`, new `apifrontend.yaml` blocks) and building out
the full helm-unittest suite, not open design questions.

---

## Wiring Manifest (Plan Phase — Mandatory)

| Component | Production Entry Point | Wiring Code Location | IT Test ID (planned) |
|-----------|------------------------|----------------------|----------------------|
| `kubernaut.llm.resolveProfile` helper | Every LLM-consuming template below | `charts/kubernaut/templates/_helpers.tpl` | IT-PLATFORM-LLM-001 (missing profile name -> `fail()`) |
| KA static SDK config (`provider`, `credentialsSecretName`-derived, `vertexProject`, `vertexLocation`, `azureApiVersion`, `tlsCaFile`, `oauth2`, `reasoning`) | `kubernaut-agent` Deployment env/ConfigMap | `templates/kubernaut-agent/kubernaut-agent.yaml` | IT-PLATFORM-LLM-002 (KA resolves `llmProfileRef`) |
| KA `llm-runtime.yaml` ConfigMap (`model`, `endpoint`, `apiKeyFile`, `temperature`, `maxRetries`, `timeoutSeconds`) | same | same | IT-PLATFORM-LLM-003 |
| KA `phaseModels` (**new**) | same | same | IT-PLATFORM-LLM-004 (phase override resolves; unknown phase -> `fail()`; VL-017/VL-021 equivalents) |
| KA `alignmentCheck.llm` (bug fix: `apiKeyFile` not `apiKey`; profile-ref) | same | same | IT-PLATFORM-LLM-005 (empty ref inherits KA's profile; VL-024/025 equivalents) |
| AF `agent.llm` (**new**) | `apifrontend` Deployment/ConfigMap | `templates/apifrontend/apifrontend.yaml` | IT-PLATFORM-LLM-006 (empty ref defaults to KA's; VL-015/016 equivalents) |
| AF `severityTriage.llm` (**new**) | same | same | IT-PLATFORM-LLM-007 (empty ref inherits AF's resolved profile; disabled -> rule-based-only; VL-024..027 equivalents) |
| Multi-Secret-volume mounting | `kubernaut-agent`/`apifrontend` Deployments | `_helpers.tpl` + both Deployment templates | IT-PLATFORM-LLM-008 (distinct `credentialsSecretName`s each get a mount; #1731 vertex_ai violation -> `fail()`) |

Final IT test IDs to be assigned during RED phase; the table above fixes
the *set* of required wiring proofs so CHECKPOINT W has a concrete
checklist once implementation starts.

---

## Consequences

### Positive Consequences
1. Fixes a real, currently-silent bug (`alignmentCheck.llm.apiKey`).
2. Closes two real Helm/Operator parity gaps (AF `agent.llm`,
   `severityTriage.llm`) that were previously unreachable via Helm despite
   full Go-side implementation.
3. Newly exposes KA's already-implemented `phaseModels` mechanism.
4. Matches the Operator's authoring model exactly — a profile written once
   in either deployment path means the same thing.
5. Future LLM-consuming components need only a `llmProfileRef` field, not
   a re-derived copy of the full field set.

### Negative Consequences
1. Materially more Helm template complexity than exists today for LLM
   config — new resolution helper, new multi-secret-mounting logic. This
   is the most complex single change in the DD-PLATFORM-00x series so far.
2. `kubernautAgent.llm.*`/`alignmentCheck.llm.*` as literal blocks are
   removed outright (no backward compatibility, per constraint) —
   accepted, matching the precedent already set.
3. Two previously-inert AF features (`agent.llm`, `severityTriage.llm`)
   become configurable and therefore, once configured, load-bearing in
   production — operators enabling them for the first time via Helm
   should expect the same behavior AF's Operator-managed deployments
   already exhibit, since both read the identical Go struct.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Helm-side merge helper diverges from Go's exact `EffectiveLLM`/`EffectivePhaseConfig` precedence for some field | Low (verified field-for-field against the Operator's own `configmaps.go` rendering, not just the Go struct's max shape — including the non-obvious `azureApiVersion`/`bedrockRegion` exclusion from phase overrides) | High (silently wrong LLM identity/credentials in production) | Helm-unittest suite mirroring VL-001..027 field-for-field; explicit test per field asserting override-wins and inherit-when-empty |
| Multi-Secret-volume mounting logic (new to this chart — converting a static entry to a `range` over de-duplicated secret names, with per-provider credential filenames) has an implementation edge case | Medium (design is now precisely specified against the Operator's verified convention, but this chart has never done a dynamic multi-secret mount before — first-of-its-kind Sprig work here) | Medium (extra unnecessary mounts, or a missed mount) | Dedicated helm-unittest cases: same secret referenced by 2 consumers -> 1 mount + inherited path; different secrets -> N mounts with correct `api_key`/`credentials.json` filenames |
| #1731 vertex_ai constraint applied to the wrong scope (e.g. accidentally also guarding KA's `phaseModels`, which has no such limitation per DD-LLM-007) | Low (scope corrected and verified directly against `kubernaut-operator/internal/resources/validation.go` — AF-only: `apifrontend.llmProfileRef` vs. `apifrontend.severityTriage.llmProfileRef`) | Medium (an overbroad guard would falsely reject valid KA `phaseModels` configs) | `fail()` guard scoped explicitly to AF's two profile refs only; helm-unittest case confirming KA `phaseModels` with two different vertex_ai profiles renders successfully (no false-positive block) |
| Repeat of the `apiKey`/`apiKeyFile`-style field-name mismatch on a different field | Low (this DD's whole point is closing that class of bug) | High if it recurs | Manual cross-check of every rendered field name against the consuming Go struct's YAML tag before merge, documented in the PR description |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| BR-PLATFORM-008 | 🟡 Pending | This DD is the implementation design for that BR |

---

## Validation Strategy

1. **Render-validity gate**: `helm lint` + `helm template` clean for a
   representative `values.yaml` exercising: single profile shared by all
   consumers; distinct profiles per consumer with distinct
   `credentialsSecretName`s; `phaseModels` override; disabled
   severity-triage.
2. **helm-unittest suite** (target: parity with the Operator's VL-001..027
   coverage): missing/undefined profile reference -> `fail()`; empty
   optional ref inherits the documented default; override precedence
   (non-empty override field wins); vertex_ai shared-credential
   constraint; multi-secret-mount correctness.
3. **Field-name audit**: every field in the resolved profile cross-checked
   against the consuming Go struct's YAML tag
   (`pkg/shared/types.LLMConfig`, `internal/kubernautagent/config.LLMOverrideConfig`)
   — the specific check that would have caught the `apiKey`/`apiKeyFile`
   bug earlier.
4. **Manual review**: PR description documents the profile-schema-to-Go-
   struct field mapping explicitly, for reviewer cross-check.

---

## References

- `internal/kubernautagent/config/config_types.go`, `pkg/apifrontend/config/config.go`,
  `pkg/shared/types/llm.go`, `cmd/apifrontend/backend_deps.go`
- `kubernaut-operator/api/v1alpha1/kubernaut_types.go` (`LLMProfileSpec`,
  `llmProfiles`, `llmProfileRef` fields), `internal/resources/validation_test.go` (VL-001..027)
- DD-PLATFORM-004, DD-PLATFORM-006, DD-LLM-007, DD-LLM-008
- BR-PLATFORM-008: Helm Chart LLM Configuration Parity
- Issues #1589, #1731, #1599, #1582, #1870, #1861

---

## Addendum: kubernaut#1861 — lifting the #1731 vertex_ai guard

**Date**: 2026-08-02

The `fail()` guard this DD introduced in `charts/kubernaut/templates/
apifrontend/apifrontend.yaml` blocked one specific combination: AF's own
`apifrontend.llmProfileRef` and `apifrontend.config.severityTriage.
llmProfileRef` both resolving to `provider: vertex_ai` with *different*
`credentialsSecretName` values. The reason was that both of
`cmd/apifrontend/backend_deps.go`'s severityTriage Vertex constructors
(`newAnthropicTriagerForVertex`, `newGenAITriagerForVertex`) called
`pkg/apifrontend/launcher.InjectAmbientGoogleCredentials`, which sets the
single, process-wide `GOOGLE_APPLICATION_CREDENTIALS` env var — two
different Secrets could never both be visible to the SDK's ADC lookup at
the same time.

kubernaut#1861 (a main-line port of release/v1.5's #1870) fixed both
constructors to resolve credentials explicitly instead:

- `newAnthropicTriagerForVertex` now passes `llmCfg.APIKey` straight
  through to `severity.NewAnthropicVertexClient`'s new `credentialsJSON`
  parameter, which uses `anthropic-sdk-go/vertex.WithCredentials` — this
  package's prior assumption that Claude-on-Vertex has no
  explicit-credentials option was **incorrect** for this SDK.
- `newGenAITriagerForVertex` now resolves `llmCfg.APIKey` via
  `cloud.google.com/go/auth/credentials.DetectDefault` and sets
  `genai.ClientConfig.Credentials` directly — mirroring the pattern AF's
  own `agent.llm` Gemini-on-Vertex path (`newVertexGeminiModel`) already
  used since kubernaut#1801.

With both severityTriage constructors now credential-independent, the two
profiles never contend for the same env var, so the guard was removed
(`IT-PLATFORM-LLM-008`'s "fails render" case became two "renders
successfully" cases proving separate mounts, separate `apiKeyFile` values,
and no static `GOOGLE_APPLICATION_CREDENTIALS`).

**What #1861 does *not* fix**: AF's own `agent.llm` Claude-on-Vertex
connection (`newVertexAnthropicModel` in `pkg/apifrontend/launcher/
model.go`) goes through the third-party `adk-anthropic-go` module rather
than `pkg/apifrontend/severity`'s direct `anthropic-sdk-go` usage. As of
`adk-anthropic-go` v1.0.0, its Vertex AI variant hardcodes
`vertex.WithGoogleAuth` internally with no credentials-override field on
its `Config` struct — verified by reading the vendored module source
(`anthropic.go`'s `VariantVertexAI` case), not assumed. This path still
relies on ambient ADC via `InjectAmbientGoogleCredentials`. This remains
safe: since severityTriage's two Vertex constructors no longer touch that
env var at all, nothing else in the process depends on what this one
remaining call site sets it to, so there is no collision to guard against.
Closing this specific gap would require either a fix upstream in
`adk-anthropic-go`, or bypassing it for AF's own agent.llm Claude-on-Vertex
construction — out of scope here; DD-LLM-007 already documents this as an
intentional architectural boundary between AF's ADK-based launcher and
KA's own, framework-independent Vertex client.

---

**Document Version**: 1.3
**Last Updated**: 2026-08-02
**Status**: 🟢 Implemented (chart + Go). Original v1.2 scope ("no Go code
changes") implemented as designed; the #1861 amendment above additionally
touched `pkg/apifrontend/severity`, `cmd/apifrontend`, and
`pkg/apifrontend/launcher` to lift the #1731 guard.
