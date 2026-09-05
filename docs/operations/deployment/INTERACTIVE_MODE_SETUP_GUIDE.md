# Interactive Mode Setup Guide (Console + APIFrontend + OIDC)

**Status**: Operator-facing setup guide, condensed from the Helm chart's own reference
(`charts/kubernaut/README.md`).
**Audience**: Operators and SREs who want chat-driven investigation — a human (or an
external agent) asks Kubernaut to investigate or remediate something on demand, via a
browser console or directly against APIFrontend's MCP/API surface.
**Authority**: `charts/kubernaut/README.md` ("Optional: Web Console", "Enable/Disable
Gateway or APIFrontend"), Issue #2162, BR-PLATFORM-006.

---

## What you're building

```
Browser ──OIDC login──▶ oauth2-proxy (Console) ──▶ Console (static SPA)
                                                          │ A2A chat
                                                          ▼
                                                     APIFrontend
                                                          │ MCP tool calls (kubernaut_investigate, kubernaut_approve, ...)
                                                          ▼
                                RemediationRequest CRD created directly (no Gateway involved)
                                                          │
                                                          ▼
                    SignalProcessing → AIAnalysis → RemediationWorkflow → WorkflowExecution
```

APIFrontend creates `RemediationRequest` CRs directly via the Kubernetes API when a chat
investigation asks for one (`pkg/apifrontend/tools/af_create_rr.go`) — it never calls
Gateway's webhook endpoint. Gateway and APIFrontend are independent entry points into the
same pipeline (Issue #2162); this guide only sets up APIFrontend's.

If you also want the alert-driven path, see
[AUTONOMOUS_MODE_SETUP_GUIDE.md](./AUTONOMOUS_MODE_SETUP_GUIDE.md) — the two are
independent and can both be enabled on the same install.

### Does enabling this let Gateway/AlertManager "take over" too?

No. Leave `gateway.enabled` at its Helm default (`true`) — it costs nothing to run
alongside Interactive mode, because `gateway.auth.signalSources` (the actual
authorization gate on who can call Gateway's webhook endpoint) defaults to an **empty
list**. With no entry in it, Gateway rejects every request with `403` regardless of
whether it's running — there's no external sender authorized to reach it until you
explicitly add one (see [AUTONOMOUS_MODE_SETUP_GUIDE.md](./AUTONOMOUS_MODE_SETUP_GUIDE.md)
step 1). Disabling Gateway outright is unnecessary defense-in-depth for a purely
interactive deployment, not a required step.

---

## Prerequisites

- A Kubernetes cluster with a dynamic-provisioning `StorageClass` (for PostgreSQL/Valkey)
- Helm 3.12+
- An API key from an LLM provider (OpenAI, Anthropic, etc.)
- An OIDC provider (Keycloak, Dex, or any standards-compliant OIDC IdP) reachable from the
  cluster, with a browser-login-capable client for Console and (optionally) an audience
  claim for APIFrontend

---

## 1. Configure your OIDC provider

Create an OIDC client for Console (confidential, redirect URI `https://<your console
host>/oauth2/callback`). APIFrontend validates the resulting JWTs — either the simple
single-provider form or the production multi-provider form:

```yaml
# Simple (dev/test convenience)
apifrontend:
  config:
    auth:
      issuerURL: "https://keycloak.example.com/realms/kagenti"

# Production (multi-provider, #1436)
apifrontend:
  config:
    auth:
      jwtProviders:
        - name: "keycloak"
          issuerURL: "https://keycloak.example.com/realms/kagenti"
          jwksURL: "http://keycloak-service.keycloak:8080/realms/kagenti/protocol/openid-connect/certs"
          audiences: ["kubernaut-apifrontend"]
          claimMappings:
            username: "preferred_username"
            groups: "groups"
```

If your IdP doesn't inject an audience claim naming `kubernaut-apifrontend` by default
(Keycloak doesn't, out of the box), add a client scope with an audience mapper and assign
it as a default scope on Console's client — otherwise APIFrontend's strict audience-list
match rejects every console-issued token.

## 2. Pre-create Console's OIDC client Secret

```bash
kubectl create secret generic console-oauth-creds \
  --from-literal=client-id=kubernaut-console \
  --from-literal=client-secret="$OIDC_CLIENT_SECRET" \
  --from-literal=cookie-secret="$(openssl rand -base64 32 | head -c 32 | base64)" \
  -n kubernaut-system
```

`cookie-secret` is a random 32-byte value oauth2-proxy uses to encrypt its session cookie —
not from your OIDC provider.

## 3. Install with Console + APIFrontend enabled

Follow [charts/kubernaut/README.md](../../../charts/kubernaut/README.md)'s Quick Start for
the namespace and the three credential Secrets (PostgreSQL, Valkey, LLM provider API key),
plus your Rego policies, then install with Console and its OIDC wiring:

```bash
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set global.llmProfiles.primary.provider=openai \
  --set global.llmProfiles.primary.model=gpt-4o \
  --set global.llmProfiles.primary.endpoint=https://api.openai.com/v1 \
  --set global.llmProfiles.primary.credentialsSecretName=llm-credentials \
  --set kubernautAgent.llmProfileRef=primary \
  --set-file signalprocessing.policies.content=path/to/policy.rego \
  --set-file aianalysis.policies.content=path/to/approval.rego \
  --set apifrontend.config.auth.issuerURL="https://keycloak.example.com/realms/kagenti" \
  --set console.enabled=true \
  --set console.auth.secretName=console-oauth-creds \
  --set console.ingress.host=console.apps.example.com \
  --set console.ingress.enabled=true
```

`apifrontend.enabled` defaults to `true` — no need to set it. `console.enabled=true`
requires `apifrontend.enabled=true` (Console proxies to APIFrontend only) and a resolvable
OIDC issuer (either `apifrontend.config.auth.issuerURL` above, or
`jwtProviders`) — the chart fails fast at `helm template`/`install` time, before anything
is applied, if either is missing.

`console.ingress.host` is **required** whenever `console.enabled=true`, even if you leave
`console.ingress.enabled=false` (the default, per BR-PLATFORM-009's opt-in-only stance on
externally exposing Kubernaut's ingress points) to front Console with your own
Ingress/Route/mesh gateway instead — oauth2-proxy needs the browser-facing hostname for its
OIDC redirect URL regardless of who creates the Ingress.

## 4. RBAC: gate real tool calls, not just login

Logging in successfully is not the same as being authorized to call MCP tools
(`kubernaut_investigate`, `kubernaut_approve`, etc.) — those go through a real Kubernetes
`SubjectAccessReview`. Two things commonly need explicit setup on a fresh OIDC realm with
no prior human-user model:

- A claim (typically `groups`) mapping the logged-in user to a role/persona your RBAC
  config recognizes. Configure `apifrontend.config.rbac` with `roleBindings` mapping OIDC
  groups to Kubernaut personas (e.g. `sre`).
- `apifrontend.config.rbac.consoleAccessAuthorizationCheckEnabled` defaults to `false`
  (#2150) — authentication-only, so a fresh install works with zero RBAC config for
  dev/eval. Set it `true` once you've configured real personas/groups for production, to
  additionally gate the console-access check itself (per-tool authorization already
  applies either way).

Without a matching group claim and role binding, the symptom looks like "login works, then
every tool call silently fails" rather than an obvious auth error — check for `403`s from a
real `SubjectAccessReview` denial, not a login/redirect loop.

---

## Verify

```bash
kubectl get pods -n kubernaut-system -l app=console,app=apifrontend
```

Open `https://<console.ingress.host>` in a browser, log in via your OIDC provider, and
start a chat asking Kubernaut to investigate a real resource in your cluster. Confirm an
`InvestigationSession`/`AgentSession` is created:

```bash
kubectl get investigationsessions,agentsessions -n kubernaut-system
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Console redirects to login in a loop, never completes | Missing/incorrect audience claim on the OIDC token for Console's own client ID | Add a self-referential audience mapper (`aud` must include `kubernaut-console` itself, not just `kubernaut-apifrontend`) |
| Login succeeds, but every real tool call fails | No RBAC role binding for the logged-in user's group | See step 4 — configure `apifrontend.config.rbac.roleBindings` |
| `helm install` fails before applying anything | Missing `console.auth.secretName`, OIDC issuer, or `console.ingress.host` while `console.enabled=true` | The chart's fail-fast validation names exactly which one is missing |
| APIFrontend rejects every console-issued token outright | No audience mapper injecting `kubernaut-apifrontend` into `aud` | See step 1's note on audience mappers — common on a fresh Keycloak realm with no existing kubernaut-apifrontend-aware client scope |
| Investigating a spoke resource fails with "cannot determine which cluster it belongs to" (or "not managed") despite the `kubernaut.ai/managed=true` label | No `cluster_id` was supplied and the alert/rule carries no `cluster` label, so fleet scope attribution is ambiguous — by design this refuses rather than guessing (Issue #2362) | Pass `cluster_id` (e.g. from the alert's `cluster` label, see `list_clusters` for IDs); for alerts, bake a static `cluster:` label into the alerting rule so every evaluation carries attribution |

---

## References

- [charts/kubernaut/README.md](../../../charts/kubernaut/README.md) — full Helm chart reference
- [Optional: Web Console](../../../charts/kubernaut/README.md#optional-web-console-br-platform-006)
- [Enable/Disable Gateway or APIFrontend](../../../charts/kubernaut/README.md#enable-disable-gateway-or-apifrontend-issue-2162)
- [AUTONOMOUS_MODE_SETUP_GUIDE.md](./AUTONOMOUS_MODE_SETUP_GUIDE.md) — the complementary alert-driven entry point
- [FLEET_SETUP_GUIDE.md](./FLEET_SETUP_GUIDE.md) — layering multi-cluster fleet mode on top of this
- `pkg/apifrontend/tools/af_create_rr.go` — source of truth for how AF creates `RemediationRequest` directly
- [Issue #2162: Gateway/APIFrontend independent enable toggles](https://github.com/jordigilh/kubernaut/issues/2162)
