# Demo RBAC: roles-only model (Issue #2703)

Since the operator RBAC decoupling ([kubernaut-operator#682](https://github.com/jordigilh/kubernaut-operator/issues/682),
umbrella [kubernaut#2701](https://github.com/jordigilh/kubernaut/issues/2701)):

- The **Kubernaut CR deploys the platform**. It no longer accepts
  `spec.apiFrontend.rbac.roleBindings` or `consoleAccessGroups` — user-access
  policy is an operational decision, not a deployment decision.
- The **operator renders roles only**: `console-access` (login gate),
  `console-reader` (console read capabilities) and
  `console-inventory-reader` (local-target inventory reads), all rendered
  **unbound** with the CR's namespace prefixed (e.g.
  `kubernaut-system-console-access`). It never renders a user-facing
  ClusterRoleBinding, and the six built-in persona ClusterRoles are gone.
- **Role composition and bindings are the demo operator's SOP**, applied as
  external ClusterRoleBindings after reconciliation.

## Files

| File | Purpose |
|---|---|
| `persona-tool-roles.yaml` | The six former persona bundles as demo-owned ClusterRoles + external bindings to the demo IdP's per-persona groups |
| `console-bindings.yaml` | External bindings for the operator-rendered console roles |

## Deny-by-default capability SARs

Every tool call and console read goes through a real Kubernetes
`SubjectAccessReview` as the logged-in user (`kubernaut.ai/tools`, verb `use`,
resourceName = tool name; `kubernaut.ai/console` for the coarse-grained gate).
These checks are **deny-by-default**: any capability not listed in a role
bound to one of the user's groups is denied. A binding grants nothing by
itself — capabilities exist only where a role enumerates them (FedRAMP AC-6
least privilege; SOC2 CC6.1 logical access controls stay outside the platform
CR).

## The both-names alert rule

The alert capability has two authorization identities: `list_alerts` /
`get_alert_details` on the A2A/chat surface and `kubernaut_list_alerts` on
the raw MCP bridge (which the Console proxies). Alert-capable bundles must
carry all three names or the same capability is denied on one surface
([kubernaut-operator#682](https://github.com/jordigilh/kubernaut-operator/issues/682),
DD-636 row 4d). SAR on a never-invoked tool name is inert, so carrying both
is safe if upstream later unifies them.

## Applying

The automated demo harness applies the equivalents automatically after
operator reconciliation (`infrastructure.BindAFPersonaRBAC` +
`infrastructure.BindDemoConsoleRBAC`, see `test/infrastructure/demo_operator.go`
and `make bind-fleet-af-rbac`). To apply manually:

```bash
kubectl apply -f test/fixtures/demo/rbac/persona-tool-roles.yaml
kubectl apply -f test/fixtures/demo/rbac/console-bindings.yaml
```

Adjust the `kubernaut-system-` role-name prefix if your Kubernaut CR lives in
another namespace, and the Group subjects to your IdP's group names. The demo
Keycloak realm (`test/infrastructure/keycloak-realm-demo.json`) issues one
group per persona plus the `sre` user `sre-user`.

## Keeping this in sync

`test/infrastructure/demo_console_rbac_test.go` (UT-INFRA-2703-004/005)
parity-tests these manifests against the shared persona contract
(`infrastructure.LoadPersonaTools`) and the harness-generated console
bindings, so drift fails CI.
