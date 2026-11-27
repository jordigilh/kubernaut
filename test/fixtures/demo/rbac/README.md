# Demo RBAC example bundles (Issue #2703)

Roles-only model: the operator renders ClusterRoles but never user-facing
ClusterRoleBindings, and the Kubernaut CR carries no user-access policy. Role
composition and bindings are the deployer's job, applied after the CR is
reconciled.

## Files

| File | Contents |
|---|---|
| `persona-tool-roles.yaml` | Six persona ClusterRoles (`demo-tool-<persona>`) + ClusterRoleBindings to the demo IdP's per-persona groups |
| `console-bindings.yaml` | ClusterRoleBindings for the operator-rendered console roles |

## Authorization model

Tool calls and console reads are authorized by `SubjectAccessReview`
(`kubernaut.ai/tools`, verb `use`, resourceName = tool name; `kubernaut.ai/console`
for the login gate), always as the logged-in user and always deny-by-default.
A binding grants nothing by itself — capabilities exist only where a role
enumerates them.

The operator-rendered console roles are namespace-prefixed
(`kubernaut-system-console-access`, `-console-reader`,
`-console-inventory-reader`); adjust the prefix if your CR lives in another
namespace. The demo Keycloak realm
(`test/infrastructure/keycloak-realm-demo.json`) defines one group per persona
and the user `sre-user` in `sre`.

## Alert bundles: single canonical name

Since #2713 unified the alert tool name, `kubernaut_list_alerts` has one
authorization identity that works on both the chat/A2A surface and the raw MCP
bridge (which the Console proxies). `kubernaut_get_alert_details` follows the
same naming convention but is a chat/A2A-only capability — it is not registered
on the raw MCP bridge, so granting it does not make detail lookup available
through `/mcp`. Bundles that grant alert access carry the canonical names.

## Applying

The demo harness applies the equivalents automatically after operator
reconciliation (`infrastructure.BindAFPersonaRBAC` +
`infrastructure.BindDemoConsoleRBAC`); `make bind-fleet-af-rbac` does the same
for a fleet-core cluster. Manually:

```bash
kubectl apply -f test/fixtures/demo/rbac/persona-tool-roles.yaml
kubectl apply -f test/fixtures/demo/rbac/console-bindings.yaml
```

`test/infrastructure/demo_console_rbac_test.go` (UT-INFRA-2703-004/005)
parity-tests these manifests against the shared persona contract
(`infrastructure.LoadPersonaTools`) and the harness-generated console
bindings, so drift fails CI.
