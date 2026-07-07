# API Frontend configuration reference

Narrowly-scoped reference for API Frontend (AF, `cmd/apifrontend`) configuration
and hot-reload behavior, written specifically to document the restart-required
LLM identity rule ([#1599](https://github.com/jordigilh/kubernaut/issues/1599) /
[DD-LLM-008](../architecture/decisions/DD-LLM-008-restart-required-llm-identity-lock.md))
for this service. AF did not previously have a configuration-reference doc; this
is not (yet) an exhaustive field-by-field reference the way
[kubernaut-agent/configuration-reference.md](../kubernaut-agent/configuration-reference.md)
is — it covers the config file, the config-drift-detection watcher, and the
LLM-identity-relevant fields specifically.

Derived from:

- `pkg/apifrontend/config/config.go` (`Config` struct, `DefaultConfig`, `Load`, `Validate`, `ResolveDefaults`)
- `cmd/apifrontend/main.go` (`configReloadCallback`, `startConfigWatcher`)
- `charts/kubernaut/templates/apifrontend/apifrontend.yaml`

## 1. Configuration file

Unlike Kubernaut Agent (KA), AF has a **single** configuration file — there is
no separate hot-reloadable LLM runtime file.

| Item | Detail |
|------|--------|
| Path | `/etc/apifrontend/config.yaml` (constant `configPath` in `cmd/apifrontend/main.go`; no CLI flag override) |
| Format | YAML |
| Reload | **Restart required for every field, including LLM identity** — see §2 |

## 2. Config-drift-detection watcher is validate-only, not hot-apply

AF does run a `config.FileWatcher` on `configPath` (`startConfigWatcher`,
wired for CM-02 config-drift-detection + audit trail purposes), but — unlike
KA's `llm-runtime.yaml` watcher, which actively swaps a live `llm.SwappableClient`
— AF's watcher callback (`configReloadCallback`) only **parses and validates**
the candidate file content. It builds a fresh, disposable `config.Config` value
from the candidate YAML, runs `ResolveDefaults()` + `Validate()` against it for
audit-trail/drift-detection purposes, and then discards it — the result is
never assigned back into the live `*config.Config` the running process is using,
and no client (LLM or otherwise) is rebuilt from it.

**Practical consequence: every field in AF's config, not just LLM identity, is
restart-only.** This was true before #1599 and required no production
behavior change to remain true — #1599 added `TestAPIFrontendCmdReloadIdentity`
(`cmd/apifrontend/reload_identity_test.go`) as regression coverage proving this
invariant specifically for `agent.llm.provider`/`.model` and
`severityTriage.llm.provider`/`.model`, so a future change to
`configReloadCallback` that started wiring live-apply couldn't silently
reintroduce the cross-provider identity risk DD-LLM-008 describes.

## 3. LLM-identity-relevant fields

AF has two independent LLM configurations, both static (config-file, restart-only):

| YAML path | Go field | Purpose |
|-----------|----------|---------|
| `agent.llm` | `Config.Agent.LLM` (`types.LLMConfig`) | Primary LLM client used by AF's agent-facing tooling (`launcher.NewModelFromConfig`, built once at startup in `cmd/apifrontend/mcp_a2a_handlers.go`). |
| `severityTriage.llm` | `Config.SeverityTriage.LLM` (`*types.LLMConfig`, nilable — feature is entirely optional) | Severity-triage LLM client, only constructed when `severityTriage.enabled` is true and `llm` is non-nil. |

Both are `types.LLMConfig` (`pkg/shared/types/llm.go`) — the same struct type KA's static `ai.llm` uses. Key sub-fields relevant to identity:

| YAML key | Description |
|----------|-------------|
| `provider` | `openai`, `anthropic`, `vertex_ai`, `openai_compatible`, etc. Empty `provider` on `agent.llm` means "LLM not configured" (`LLMConfig.Validate` returns nil early) — not every AF deployment configures an LLM at all. |
| `model` | Model id for the provider. |
| `endpoint`, `apiKeyFile`, `azureApiVersion`, `vertexProject`, `vertexLocation`, `bedrockRegion`, `oauth2.*` | Same semantics as KA's `ai.llm` — see [kubernaut-agent/configuration-reference.md §4.1](../kubernaut-agent/configuration-reference.md#41-aillm-static) for the shared struct's field reference. |

Since **nothing** in AF hot-reloads (§2), there is no separate "identity vs.
tuning" distinction to make here the way there is for KA's `phaseModels` (KA
§6.1) — every field above, identity or not, requires a restart to change. The
distinction only matters for KA, which has an actual hot-swap mechanism to
guard.

## 4. Helm mapping

Source: `charts/kubernaut/templates/apifrontend/apifrontend.yaml`.

As of this writing, the chart renders `agent.kaBaseURL`, `agent.dsBearerTokenFile`,
and `severityTriage.cacheTTLSeconds`/`.llmConfidence`, but does **not** render
`agent.llm.*` or `severityTriage.llm.*` — AF's LLM configuration (when used) is
not currently exposed via primary Helm values and must be supplied via a
ConfigMap patch/overlay or chart fork. This is an accurate description of the
current chart, not a statement that it should stay this way; if `agent.llm`
gains first-class Helm support in the future, cross-reference this file and
DD-LLM-008 when documenting it.

## 5. Deployment topology and restart triggers

Since every AF config field (LLM identity included) requires a restart to take
effect, the question "will my ConfigMap edit actually reach the running pod"
depends entirely on deployment topology — identical concern and identical
guidance to KA's (see
[kubernaut-agent/configuration-reference.md §13](../kubernaut-agent/configuration-reference.md#13-deployment-topology-and-restart-triggers-for-llm-identity-changes)):

| Deployment path | Restarts on `config.yaml` ConfigMap change? |
|------------------|:---:|
| Kubernaut Operator | ✅ Yes — hashes ConfigMap contents into pod template annotations |
| Helm chart only (no operator) | ❌ No — manual `kubectl rollout restart` needed |
| GitOps (e.g. ArgoCD) | ❌ No by default — use [`stakater/Reloader`](https://github.com/stakater/Reloader) if you want automatic restarts on ConfigMap change |

---

Treat this reference as authoritative for AF's LLM-identity restart semantics.
For everything else in AF's configuration surface (auth, RBAC, rate limiting,
fleet, session, etc.), read `pkg/apifrontend/config/config.go` directly until a
full field-by-field reference exists for this service.
