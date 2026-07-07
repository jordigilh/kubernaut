# DD-LLM-008: LLM Identity (Provider+Model) Requires a Restart to Change

**Status**: ✅ Approved & Implemented
**Priority**: P1
**Owner**: KubernautAgent Team
**Scope**: `cmd/kubernautagent/llm_builder.go`, `cmd/kubernautagent/bootstrap.go`, `cmd/kubernautagent/main.go`, `cmd/apifrontend/main.go`, `internal/kubernautagent/config/config_types.go`
**Related**: [DD-LLM-005](./DD-LLM-005-model-aware-reasoning-support.md), [DD-LLM-007](./DD-LLM-007-af-ka-anthropic-client-divergence.md), [BR-HAPI-199](../../requirements/BR-HAPI-199-configmap-hot-reload.md) (superseded, see below), Issue #1599, #1578, #1580

---

## Context & Problem

Issue #1578 (LLM reasoning/thinking token support) and #1580 (Anthropic reasoning-block `Signature` handling) introduced provider-issued opaque signature bytes attached to reasoning content (e.g. Anthropic's thinking-block `Signature`, used to prove a reasoning block was genuinely produced by that model and hasn't been tampered with). While reviewing the pyramid-invariant coverage that landed with PR #1598, a pre-existing architectural gap surfaced: Kubernaut Agent's (KA) LLM hot-reload path allowed the `model` field — and, via `phaseModels` overrides, the `provider` field too — to change live, with no restart, at both the base and per-phase level.

This is a real risk, not a theoretical one: if a phase's LLM identity is hot-swapped from, say, Anthropic/`claude-sonnet` to OpenAI/`gpt-4o` mid-deployment, any reasoning-block replay logic (or a future feature building on the same signature-carrying data model) could end up attempting to validate or replay a signature issued by one provider against a completely different provider/model that never issued it. Even absent an active exploit, silently swapping the model backing a phase invalidates any assumption an operator or auditor makes about "which model made this decision" for that phase going forward — a correctness and auditability problem in its own right (see BR-AUDIT-005 / SOC2 CC8.1: audit reconstruction must be able to name the actual model that acted).

Two services were in scope:

- **KA** (`cmd/kubernautagent`): has a real hot-swap mechanism (`llm.SwappableClient`, wired through `llmRuntimeReloadCallback`) that actively swaps the live LLM client on every FileWatcher-detected change to `llm-runtime.yaml` — including, prior to this DD, the `model` field and, for `phaseModels` overrides, `provider` too.
- **API Frontend** (`cmd/apifrontend`, "AF"): investigated as part of this same effort. AF's config-drift-detection watcher (`configReloadCallback`, extracted from an anonymous closure in `main.go` as part of this change for testability) only *validates* candidate config for audit purposes — it was already, by construction, never wired to mutate the live `*config.Config` or rebuild AF's LLM client. AF requires a full process restart to apply *any* config change, LLM identity included. This DD's guard therefore applies only to KA; AF needed a regression test (not new production logic) proving this already-restart-only behavior holds, so a future refactor doesn't accidentally wire live-apply into that callback.

## Alternatives Considered

### Alternative A — Allow live provider/model swaps, but strip/ignore `Signature` on cross-provider swap (rejected)

- **Pros**: Preserves today's "everything is hot-reloadable" UX for LLM identity.
- **Cons**: Requires reliably detecting "this in-flight investigation phase's pinned client identity differs from what the resolver now serves" at every consumption point of reasoning data, forever, as a permanent defensive layer — fragile, and does nothing for the auditability concern (an operator still can't trust "which model acted" without cross-referencing hot-reload history). Rejected as treating a symptom instead of the cause.

### Alternative B — Restart-required LLM identity lock, tuning fields remain hot-reloadable (chosen)

- **Pros**: Provider+Model become a fixed identity contract for the life of the process, matching how a human operator already reasons about "which model is running" and eliminating the cross-provider replay risk at the source rather than downstream. All genuinely safe-to-hot-reload fields (temperature, timeouts, retries, endpoint, API key rotation, custom headers) keep working exactly as before — this is not a blanket "disable hot-reload" change. Symmetric with AF, which (per the investigation above) was already restart-only for every field, LLM identity included.
- **Cons**: Operators who were relying on live provider/model swaps (if any — no production use of this was found; `phaseModels` cross-provider examples existed only in docs, not confirmed live traffic) must restart the pod to change LLM identity going forward. Mitigated by: the Kubernaut Operator already forces a rolling restart on any `llm-runtime` ConfigMap change today (see Deployment Note below), so operator-managed deployments see no behavior change at all; only Helm-only/GitOps deployments without an equivalent restart trigger are newly affected, and are the ones this DD's documentation update specifically targets.

### Decision

**Alternative B.** Approved 2026-07-06 after preflight confirmed AF was unaffected (already restart-only) and confirmed KA's in-flight-phase-pinning mechanism (`DefaultPhaseResolver.ResolvePhase`, #783/#1470) already protects *running* investigations from any hot-swap — the risk was specifically about the *next* phase/investigation picking up a silently-changed identity, not about corrupting work in progress.

## Design

### What counts as "identity"

Provider + Model, evaluated per LLM configuration scope:

- **Base**: `staticCfg.AI.LLM.Provider` (static config, already immutable — `LLMRuntimeConfig` has no `Provider` field) + `llmRuntime.Model` (was hot-reloadable; now locked).
- **Per-phase** (`phaseModels.<phase>`): the *effective* provider+model for that phase after merging the override onto the base (`LLMRuntimeConfig.EffectivePhaseConfig`) — an override that sets neither `provider` nor `model` inherits the base identity and is therefore never an identity change by construction; a phase override that changes only `endpoint`, `apiKeyFile`, `azureApiVersion`, `vertexProject`, `vertexLocation`, or `bedrockRegion` is tuning, not identity.
- **`AlignmentCheckConfig.LLM`**: confirmed static (loaded once from `staticCfg`, never hot-reloaded) — out of scope, no change needed.

### Enforcement point and failure mode

`cmd/kubernautagent/llm_builder.go`:

- `llmRuntimeReloadCallback` takes a new `bootRuntime *kaconfig.LLMRuntimeConfig` parameter: the frozen `LLMRuntimeConfig` snapshot loaded once at process start (`cmd/kubernautagent/main.go`'s `llmRuntime`, threaded through `apiServerStartParams.bootRuntime` → `hotReloadParams.BootRuntime` → `wireLLMRuntimeWatcher`). This snapshot is never mutated across reloads — it is the single source of truth for "what identity is this process currently running," independent of what any later reload attempted (successful or rejected).
- `parseAndAuthorizeReload` (extracted from `llmRuntimeReloadCallback` to keep cognitive complexity within the AGENTS.md anti-pattern budget) rejects the *entire* candidate reload — atomically, before any client is built or any `SwappableClient.Swap` call happens — if either:
  1. `rt.Model != bootRuntime.Model` (base identity change), or
  2. `validatePhaseIdentity` finds any phase (new, existing, or being removed) whose effective provider/model differs between `bootRuntime` and the candidate `rt`.
- Rejection returns a normal error from the reload callback. `pkg/shared/hotreload.FileWatcher` (already, prior to this DD) treats any callback error as "keep the previous, known-good config" — no new rejection-handling mechanism was needed; #1599 only needed to make the callback itself judge more changes as errors.
- This makes the reload atomic in the sense that matters for #1599: an identity violation in *any* part of the payload (base or one phase among several) fails the whole reload, so a config that both wants to safely tune an unrelated field *and* illegally swap one phase's provider is rejected in full, not partially applied.

### What is intentionally unaffected

Every other hot-reloadable field keeps working exactly as before this DD: `temperature`, `maxRetries`, `timeoutSeconds`, `endpoint`, `apiKeyFile`/API key rotation, `customHeaders`, and non-identity phase overrides (adding/removing/tuning a phase override whose effective identity equals — and continues to equal — the base identity).

### AF

No production logic changed for the reload path itself. `cmd/apifrontend/main.go`'s previously-anonymous config-watcher callback was extracted into a named `configReloadCallback(cfg *config.Config) func([]byte) error` purely to make the following invariant independently testable: parsing and validating a candidate config (for CM-02 audit/drift-detection purposes) never mutates the live `*config.Config` passed to `startConfigWatcher`, regardless of what fields the candidate changes. `TestConfigReloadNeverMutatesLiveConfig`-equivalent coverage lives in `cmd/apifrontend/reload_identity_test.go`.

### Deployment note: who actually restarts on an LLM identity change today

This DD changes *what a config-file edit alone can do* — it does not, by itself, restart any pod. Whether an LLM identity change reaches a running KA pod at all depends on the deployment path:

- **Kubernaut Operator**: already computes a hash of the `llm-runtime` ConfigMap's contents and stamps it into the Deployment's pod template annotations (`internal/controller/kubernaut_controller.go`), forcing a rolling restart on *any* change to that ConfigMap — LLM identity or tuning field alike. No operator change was needed for this DD; operator-managed deployments were already restart-on-change before and after.
- **Helm chart (no operator)**: the chart renders the ConfigMap but does not itself trigger a restart on ConfigMap content changes — this is a standard Helm limitation, not specific to Kubernaut, and intentionally out of this DD's scope to change (the chart's job ends at rendering resources; restart-on-change is a deployment-topology concern).
- **GitOps (e.g. ArgoCD) managing the Helm chart or raw manifests directly**: same as the raw-Helm case — ArgoCD syncs the ConfigMap but does not restart pods on its own. Teams in this position who want automatic restarts on `llm-runtime` ConfigMap changes should use [`stakater/Reloader`](https://github.com/stakater/Reloader) (annotation-based restart triggers), with an ArgoCD `ignoreDifferences` rule for the pod-template annotation Reloader adds, to avoid ArgoCD reporting perpetual drift. This is documented as an external-tool recommendation in the configuration-reference docs, not implemented in the chart itself.

## Consequences

### Positive
- Closes the cross-provider signature-replay risk at the source (identity can't drift underneath in-flight or future reasoning-block validation logic) rather than requiring permanent downstream defensive checks.
- Matches operator intuition: "which model is this pod running" is now a boot-time fact, consistent for the process lifetime, improving audit trail trustworthiness (SOC2 CC8.1 reconstruction can name the actual model without cross-referencing hot-reload history).
- All non-identity hot-reload behavior (the vast majority of real-world tuning use cases) is fully preserved — this is a narrow, targeted restriction, not a hot-reload regression.
- AF required no production behavior change (was already compliant); the investigation converted an implicit invariant into an explicit, tested one.

### Negative
- Any operator relying on live cross-provider/model `phaseModels` swaps (undetected in production use, only present in now-corrected documentation examples) must restart the pod going forward.
- Helm-only and GitOps-managed deployments (without the Operator or an equivalent restart-trigger like Reloader) will silently keep running the old identity if an operator edits `llm-runtime.yaml`'s `model`/`provider`/phase-override identity fields expecting immediate effect — mitigated by documentation, not code, per this DD's Deployment Note.
- A pre-existing, narrower micro-race in `DefaultPhaseResolver.ResolvePhase` (client/model-name/runtime-params fetched via three separate calls rather than one atomic snapshot) was identified during this investigation. It was out of scope for this DD (unrelated to identity locking specifically) but has since been fixed under [#1610](https://github.com/jordigilh/kubernaut/issues/1610): `SwappableClient.Pin()` now captures all three under one lock acquisition, and both `DefaultPhaseResolver.ResolvePhase` and `Investigator.resolveForPhase`'s legacy branch use it.

---

**Document Control**:
- **Created**: 2026-07-06
- **Version**: 1.0
- **Status**: Approved & Implemented
