# Demo RBAC example bundles (Issue #2703)

Roles-only model: the operator renders ClusterRoles but never user-facing
ClusterRoleBindings, and the Kubernaut CR carries no user-access policy. Role
composition and bindings are the deployer's job, applied after the CR is
reconciled.

**The product primitive is the per-tool capability role.** Kubernaut exposes
roles and capabilities; it carries no opinion about which user gets which
functionality. Any group-to-role mapping an operator composes — including ones
that share none of the names below — is fully supported first-class behavior,
not a deviation. The six demo bundles in `example-tool-roles.yaml` are **one
worked example composition** (they happen to be one role per demo IdP group);
treat them as a starting point, not a contract.

## Files

| File | Contents |
|---|---|
| `example-tool-roles.yaml` | Six **example** capability-role bundles (`demo-tool-<name>`) + ClusterRoleBindings to the demo IdP's groups |
| `console-bindings.yaml` | ClusterRoleBindings for the operator-rendered console roles |

## Authorization model

Tool calls and console reads are authorized by `SubjectAccessReview`
(`kubernaut.ai/tools`, verb `use`, resourceName = tool name; `kubernaut.ai/console`
for the login gate), always as the logged-in user and always deny-by-default.
A binding grants nothing by itself — capabilities exist only where a role
enumerates them. Because each role enumerates tool names, you can compose
roles however you like: one role per team, one per capability, shared roles
across groups — the platform does not expect any particular set of roles or
groups.

The operator-rendered console roles are namespace-prefixed
(`kubernaut-system-console-access`, `-console-reader`,
`-console-inventory-reader`); adjust the prefix if your CR lives in another
namespace. The demo Keycloak realm
(`test/infrastructure/keycloak-realm-demo.json`) defines one group per demo
bundle and the user `sre-user` in `sre` — an example IdP shape, not a
requirement.

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
kubectl apply -f test/fixtures/demo/rbac/example-tool-roles.yaml
kubectl apply -f test/fixtures/demo/rbac/console-bindings.yaml
```

`test/infrastructure/demo_console_rbac_test.go` (UT-INFRA-2703-004/005)
parity-tests these manifests against the harness fixture contract
(`infrastructure.LoadPersonaTools`) and the harness-generated console
bindings, so drift fails CI. That contract pins the *demo fixture's* role
contents — it is not a statement about which roles or groups an operator
should compose.

For the full per-operation grant catalog (what to grant for "list workflows",
"approve a RAR", "initiate investigation", …), see the
[operation → capability grant catalog](../../../../docs/services/apifrontend/security/AUTHENTICATION_AND_RBAC.md#operation--capability-grant-catalog)
in the AF security model, and the
[composing your own roles quickstart](../../../../docs/services/apifrontend/security/AUTHENTICATION_AND_RBAC.md#composing-your-own-roles-quickstart)
for a copy-paste role + binding template.
