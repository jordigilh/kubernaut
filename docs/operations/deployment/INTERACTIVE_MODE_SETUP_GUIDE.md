# Interactive Mode Setup Guide (Console + APIFrontend + OIDC)

**Status**: Operator-facing setup guide for the Kubernaut Operator deployment.
**Audience**: Operators and SREs who want chat-driven investigation — a human (or an
external agent) asks Kubernaut to investigate or remediate something on demand, via a
browser console or directly against APIFrontend's MCP/API surface.
**Authority**: [Kubernaut Operator installation guide](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md),
Issue #2162, BR-PLATFORM-006.

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

No. Leave Gateway enabled in the `Kubernaut` CR — it costs nothing to run
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

Install the Kubernaut Operator and apply a `Kubernaut` CR using the
[operator installation guide](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md).
Configure the CR's Console, APIFrontend, OIDC, LLM, policy, and dependency fields for
this mode; the application Helm chart is no longer shipped by this repository.

APIFrontend must be enabled when Console is enabled because Console proxies to it. Configure
a resolvable OIDC issuer in the `Kubernaut` CR; the Operator validates the resulting workload
configuration before reconciliation.

`console.ingress.host` is **required** whenever `console.enabled=true`, even if you leave
`console.ingress.enabled=false` (the default, per BR-PLATFORM-009's opt-in-only stance on
externally exposing Kubernaut's ingress points) to front Console with your own
Ingress/Route/mesh gateway instead — oauth2-proxy needs the browser-facing hostname for its
OIDC redirect URL regardless of who creates the Ingress.

For production, use the `kubernaut` realm (or provide your organization's full issuer URL).
The `kubernaut-demo` realm is reserved for the repository's explicit demo/test installation
path. When upgrading an installation that uses another realm, migrate the Keycloak client,
issuer configuration, and active sessions before changing `issuerURL`; do not switch realms
while existing tokens are expected to remain valid.

## 4. RBAC: roles-only model — compose and bind externally

Logging in successfully is not the same as being authorized to call MCP tools
(`kubernaut_investigate`, `kubernaut_approve`, ...) or read console
projections — every one of those goes through a real Kubernetes
`SubjectAccessReview` as the logged-in user, and the checks are
**deny-by-default**: a capability exists only where a ClusterRole bound to one
of the user's groups enumerates it (FedRAMP AC-6 least privilege).

Since the operator RBAC decoupling
([kubernaut-operator#682](https://github.com/jordigilh/kubernaut-operator/issues/682),
Issue #2701):

- The **Kubernaut CR deploys the platform**. It no longer accepts
  `spec.apiFrontend.rbac.roleBindings` or `consoleAccessGroups` — user-access
  policy is an operational decision, not a deployment decision.
- The **operator renders roles only**, all unbound, namespaced by the CR's
  namespace: `<ns>-console-access` (the coarse-grained login gate),
  `<ns>-console-reader` (console REST read capabilities) and
  `<ns>-console-inventory-reader` (local-target inventory reads). It never
  renders a user-facing ClusterRoleBinding, and the built-in persona
  ClusterRoles are gone — personas are documented use-case bundles you
  compose yourself.
- **Role composition and bindings are your SOP**: apply your own
  ClusterRoles/ClusterRoleBindings (GitOps, OpenShift RBAC, or `kubectl
  apply`) after the operator has reconciled the CR.

The demo's ready-made example bundles live in
[`test/fixtures/demo/rbac/`](../../../test/fixtures/demo/rbac/): the six
former persona bundles as demo-owned ClusterRoles with external bindings to
the demo IdP's groups, plus external bindings for the three operator-rendered
console roles. Alert-capable bundles must carry both alert surfaces' tool
names (`list_alerts`, `get_alert_details` **and** `kubernaut_list_alerts`) —
see the both-names rule in that directory's README.

Two AF-side settings remain in the CR (`spec.apiFrontend.rbac`):
`consoleAccessAuthorizationCheckEnabled` (default `false`, #2150 —
authentication-only gate for dev/eval) and `sarCacheTTL`. Set it `true` once
you have bound the `<ns>-console-access` role to your groups, so the console
gate's authorization check is enforced too — without the binding, a `true`
gate denies every user (deny-by-default).

Without a matching group claim and role binding, the symptom looks like
"login works, then every tool call silently fails" rather than an obvious
auth error — check for `403`s from a real `SubjectAccessReview` denial, not a
login/redirect loop.

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
| Login succeeds, but every real tool call fails | No ClusterRole bound to the logged-in user's group | See step 4 — compose roles and apply external ClusterRoleBindings ([demo examples](../../../test/fixtures/demo/rbac/)) |
| Console reconciliation fails | Missing Console secret, OIDC issuer, or ingress host | Inspect the `Kubernaut` CR conditions and Operator events |
| APIFrontend rejects every console-issued token outright | No audience mapper injecting `kubernaut-apifrontend` into `aud` | See step 1's note on audience mappers — common on a fresh Keycloak realm with no existing kubernaut-apifrontend-aware client scope |
| Investigating a spoke resource fails with "cannot determine which cluster it belongs to" (or "not managed") despite the `kubernaut.ai/managed=true` label | No `cluster_id` was supplied and the alert/rule carries no `cluster` label, so fleet scope attribution is ambiguous — by design this refuses rather than guessing (Issue #2362) | Pass `cluster_id` (e.g. from the alert's `cluster` label, see `list_clusters` for IDs); for alerts, bake a static `cluster:` label into the alerting rule so every evaluation carries attribution |

---

## References

- [Kubernaut Operator installation guide](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md)
- [AUTONOMOUS_MODE_SETUP_GUIDE.md](./AUTONOMOUS_MODE_SETUP_GUIDE.md) — the complementary alert-driven entry point
- [FLEET_SETUP_GUIDE.md](./FLEET_SETUP_GUIDE.md) — layering multi-cluster fleet mode on top of this
- `pkg/apifrontend/tools/af_create_rr.go` — source of truth for how AF creates `RemediationRequest` directly
- [Issue #2162: Gateway/APIFrontend independent enable toggles](https://github.com/jordigilh/kubernaut/issues/2162)
