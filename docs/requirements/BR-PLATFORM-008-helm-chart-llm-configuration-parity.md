# BR-PLATFORM-008: Helm Chart LLM Configuration Parity

**Status**: 🟢 Implemented (amended 2026-08-02 by kubernaut#1861 — see
DD-PLATFORM-007's Addendum)
**Version**: 1.2
**Date**: 2026-07-25
**Category**: PLATFORM
**Priority**: P2
**Target Version**: V1.6

---

## Business Need

### Problem Statement

The Kubernaut Operator centralizes LLM provider configuration in a single
named-profile map (`spec.llmProfiles`), referenced by name
(`llmProfileRef`) from Kubernaut Agent (KA), API Frontend (AF), and AF's
severity-triage fallback tier, plus per-phase overrides
(`kubernautAgent.phaseModels`). The Helm chart does not have this
mechanism — and, on inspection of the actual Go consumption contracts
(`internal/kubernautagent/config/config_types.go`,
`pkg/apifrontend/config/config.go`, `pkg/shared/types.LLMConfig`), the gap
is larger than "the chart duplicates a few fields":

1. **A dead config field.** `kubernautAgent.alignmentCheck.llm.apiKey`
   (plaintext string, `values.schema.json`) is rendered verbatim by
   `templates/kubernaut-agent/kubernaut-agent.yaml` as `apiKey: <value>`.
   The Go struct it's unmarshaled into (`LLMOverrideConfig`) has no
   `apiKey` field — only `apiKeyFile` (a path to a Secret-mounted file,
   matching every other credential field in the codebase). Setting this
   field today has **no effect** on the running KA process.
2. **Two fully-implemented, fully-validated Go capabilities with zero Helm
   exposure.** AF's `Config.Agent.LLM` (`pkg/apifrontend/config/config.go`)
   and `Config.SeverityTriage.LLM` are both real `types.LLMConfig` fields,
   validated (`validateLLM`, `validateSeverityTriage`) and consumed
   (`cmd/apifrontend/backend_deps.go`'s `newLLMTriagerFromConfig`, with
   dispatch for Anthropic direct, Anthropic-on-Vertex, Gemini, and
   OpenAI-compatible providers). `templates/apifrontend/apifrontend.yaml`
   renders `agent:` and `severityTriage:` blocks today with **no `llm:`
   sub-key at all** — there is no `values.yaml` path to configure either.
   Both features are silently inert in every Helm-chart deployment.
3. **A Go-supported mechanism the chart never surfaced.** KA's
   `LLMRuntimeConfig.PhaseModels` (per-phase LLM override, keys
   `rca`/`workflow_discovery`/`validation`) is fully implemented
   (`EffectivePhaseConfig`) and covered by DD-LLM-008's restart-required
   identity-lock design, but `values.schema.json` has no `phaseModels`
   field — the Helm chart cannot express it.

This BR treats "Helm/Operator LLM configuration parity" as the umbrella
outcome, in the same family as BR-PLATFORM-003 (observability/autoscaling
parity), BR-PLATFORM-005 (security parity), and BR-PLATFORM-006 (console
parity).

### Business Objective

Bring the Helm chart's LLM configuration surface to functional parity with
the Kubernaut Operator's `llmProfiles`/`llmProfileRef` model — a single
named-profile map, referenced by name from every consumer — fixing the
`apiKey`/`apiKeyFile` mismatch and wiring AF's `agent.llm`/`severityTriage.llm`
and KA's `phaseModels` as part of the same change, since all three require
the same underlying resolution mechanism.

### Success Criteria

1. `global.llmProfiles` supports one or more named profiles, each shaped to
   match `pkg/shared/types.LLMConfig`'s Helm-relevant fields exactly
   (field names, not just concepts — no repeat of the `apiKey`/`apiKeyFile`
   mismatch).
2. `kubernautAgent.llmProfileRef`, `kubernautAgent.phaseModels`,
   `kubernautAgent.alignmentCheck.llmProfileRef`, `apifrontend.llmProfileRef`,
   and `apifrontend.severityTriage.llmProfileRef` all resolve correctly,
   with the same default-inheritance semantics documented on the
   Operator's equivalent fields (KA required; AF optional, defaults to
   KA's; severity-triage optional, defaults to AF's resolved profile).
3. A profile referenced by more than one consumer with the same
   `credentialsSecretName` mounts one Secret volume, shared; a consumer
   whose resolved profile has a *different* `credentialsSecretName` gets
   its own dedicated Secret volume and `apiKeyFile` path — mirroring the
   Operator's documented behavior.
4. The vertex_ai shared-ambient-credentials constraint (kubernaut#1731) was
   originally enforced with an explicit `fail()` guard rather than left as
   a silent misconfiguration. **Superseded by kubernaut#1861**: AF's two
   severityTriage Vertex constructors now resolve credentials explicitly
   per-profile instead of relying on the shared ambient env var, so the
   constraint no longer exists and the guard was removed — AF's own
   `agent.llm` and `severityTriage.llm` may now independently use
   `vertex_ai` with different `credentialsSecretName` values.
5. `helm lint` + `helm template` render validity, plus a `helm-unittest`
   suite covering profile resolution/override/default-inheritance edge
   cases, comparable in coverage to the Operator's `validation_test.go`
   (VL-001 through VL-027).
6. Zero Go code changes required — this is a Helm chart change only; all
   consumption contracts already exist and are already validated.

## Functional Requirements

- FR-1: `global.llmProfiles: map[string]<profile>` — see DD-PLATFORM-007
  for the exact field set.
- FR-2: `kubernautAgent.llmProfileRef` (required) replaces
  `kubernautAgent.llm.*` as a literal block.
- FR-3: `kubernautAgent.phaseModels: map[string]string` (phase name ->
  profile name) — newly exposed, not previously reachable via Helm.
- FR-4: `kubernautAgent.alignmentCheck.llmProfileRef` (optional; empty
  inherits KA's resolved profile) replaces `alignmentCheck.llm.*`,
  fixing the `apiKey`/`apiKeyFile` mismatch as part of the same change.
- FR-5: `apifrontend.llmProfileRef` (optional; empty defaults to KA's
  profile) — newly exposed.
- FR-6: `apifrontend.severityTriage.llmProfileRef` (optional; empty
  inherits AF's resolved profile; absent/disabled falls back to
  rule-based-only triage) — newly exposed.

## Non-Goals

- No Go code changes in KA, AF, or the Operator — every consumption
  contract this BR wires already exists and is already validated. **Narrow
  exception (kubernaut#1861)**: lifting the #1731 vertex_ai guard required
  fixing `pkg/apifrontend/severity` and `cmd/apifrontend` to resolve
  severityTriage's Vertex credentials explicitly rather than via ambient
  ADC — a correctness fix to an existing capability, not new chart-facing
  functionality, so it doesn't change this BR's functional requirements
  above.
- No change to DD-LLM-008's restart-required identity-lock semantics —
  this BR changes how the chart *authors* LLM config (named references
  instead of literal blocks); the rendered runtime ConfigMap/Secret shape
  each binary reads is unchanged in kind, only in how it's assembled.
- No new provider support (`bedrock` remains schema-rejected per #1582;
  `tlsCertFile`/`tlsKeyFile`/`tlsClientSecretRef`/`customHeaders` exist on
  the shared Go struct but aren't exposed by any current Helm consumer
  either — left out of this BR's scope; tracked as a future addition if a
  concrete need arises, not built speculatively here).

---

## FedRAMP Control Mapping (kubernaut#1861 addendum)

Scoped to Success Criterion 4 only (the vertex_ai explicit-credentials fix) —
the rest of this BR is chart-authoring ergonomics with no independent
compliance surface.

| Control | Objective | How This Fix Serves It |
|---------|-----------|-------------------------|
| **AC-6** | Least privilege | AF's own `agent.llm` connection and `severityTriage.llm` connection can now each authenticate with an independently-scoped GCP service account (different `credentialsSecretName`) instead of being forced to share one ambient-ADC identity — enabling, e.g., a more narrowly-scoped IAM role for severityTriage than for the main agent |
| **SI-10** | Information input validation | `resolveAnthropicVertexAuth` (`pkg/apifrontend/severity/anthropic.go`) validates credential JSON structure and rejects disallowed credential types (e.g. `external_account`) before constructing an authenticated client, rather than passing untrusted material straight to the SDK |

**Ceiling of proof for this control pair**: UT + IT (not E2E). A genuine
end-to-end proof would require either live GCP credentials in CI (against
this repo's own secrets-handling conventions) or a new mock Google
OAuth2-token/Vertex-AI double in the E2E harness — infrastructure that
doesn't exist today for *any* `vertex_ai` path (the existing E2E harness only
ever configures `provider: openai_compatible` against mock-LLM; see
`test/infrastructure/fullpipeline_e2e_helm.go`). None of the three prior
fixes in this exact credential-resolution code path (#1731/#1870, #1801,
#1792) built this either, so IT is the accepted, consistent ceiling for this
credential domain rather than a gap unique to #1861.

## Test Coverage (kubernaut#1861)

| Test ID | Tier | FedRAMP | What It Proves |
|---------|------|---------|-----------------|
| UT-AF-1861-001 | UT | AC-6 | Client constructs from explicit credentials JSON alone, no ambient ADC |
| UT-AF-1861-002 | UT | SI-10 | Malformed credentials JSON is rejected |
| UT-AF-1861-003 | UT | SI-10 | Disallowed credential type (`external_account`) is rejected |
| UT-AF-1861-004 | UT | SI-10 | Empty project errors before credential resolution runs |
| UT-AF-1861-005 | UT | AC-6 | Falls back to ambient ADC when no explicit credentials given (backward compatibility) |
| IT-AF-1861-001 | IT | AC-6 | `newLLMTriagerFromConfig` routes a vertex_ai+claude profile's own credentials to a working AnthropicTriager, no ambient ADC |
| IT-AF-1861-002 | IT | AC-6 | `newLLMTriagerFromConfig` routes a vertex_ai+gemini profile's own credentials to a working GenAITriager, no ambient ADC |
| IT-PLATFORM-LLM-008 | IT (Helm) | AC-6 | Chart renders two independent Secret volumes/mounts when AF main and severityTriage both resolve to vertex_ai with different `credentialsSecretName` |

---

## Related Decisions

- **Tracked in**: DD-PLATFORM-007 (LLM Profile Consolidation) —
  implementation design, alternatives, and Wiring Manifest.
- **Builds on**: DD-PLATFORM-004/DD-PLATFORM-006 (the
  shared-default-plus-per-consumer-override pattern this BR generalizes
  to LLM identity), DD-LLM-008 (restart-required identity lock — preserved
  unchanged), DD-LLM-007 (AF/KA Anthropic client divergence — relevant to
  per-consumer provider dispatch).
- **Same parity family as**: BR-PLATFORM-003, BR-PLATFORM-005,
  BR-PLATFORM-006 (each closes a different Helm/Operator functional gap).
- **Related issues**: #1589 (Helm/Operator parity triage — this BR's `AF
  agent.llm`/`severityTriage.llm` findings belong in the same tracking
  bucket), #1731 (vertex_ai shared-credentials constraint, lifted by
  #1861), #1870 (release/v1.5 fix that proved the lift was safe), #1861
  (main-line port of #1870, removed this BR's `fail()` guard), #1599
  (restart-required identity lock).
