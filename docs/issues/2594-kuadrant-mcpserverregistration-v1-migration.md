# Issue #2594: Kuadrant MCPServerRegistration v1 migration

## Supported contract

Kubernaut's Kuadrant fleet registry watches `mcp.kuadrant.io/v1`
`MCPServerRegistration` resources. The registry uses the registration name as the
cluster ID, reads `spec.prefix` for the tool prefix, includes only resources labeled
`kubernaut.ai/managed: "true"`, and excludes registrations with
`spec.state: Disabled`.

Kuadrant MCP Gateway v1.0.2 is the upstream evidence for this contract: its tagged CRD
serves `mcp.kuadrant.io/v1` as storage and explicitly sets
`mcp.kuadrant.io/v1alpha1` to `served: false` and `storage: false`.

## Upgrade path

The Kubernaut build containing #2594 is **not** backward-compatible with a Kuadrant
installation that exposes only `mcp.kuadrant.io/v1alpha1`. Do not assume that an older
Kuadrant release serves `v1` or performs an automatic conversion.

1. Upgrade the Kuadrant MCP Gateway/controller to a release that serves
   `mcp.kuadrant.io/v1` (v1.0.2 is the validated CRD reference for this change).
2. Confirm the installed CRD serves v1 before upgrading Kubernaut:

   ```bash
   kubectl get crd mcpserverregistrations.mcp.kuadrant.io \
     -o jsonpath='{range .spec.versions[*]}{.name}{" served="}{.served}{" storage="}{.storage}{"\n"}{end}'
   ```

   The output must include `v1 served=true storage=true`.
3. Re-apply each fleet registration with `apiVersion: mcp.kuadrant.io/v1`, preserving
   its name, managed label, `spec.prefix`, `spec.state`, credentials, and target route.
   Treat this as a declarative migration; do not rely on an unverified conversion.
4. Verify the v1 objects and labels, then upgrade/restart Kubernaut fleet-aware services.
   Registry readiness should become healthy after the v1 registrations are listed and
   watched.

If the CRD does not serve v1, stop the Kubernaut upgrade and complete the Kuadrant
upgrade first. The registry intentionally has no v1alpha1 fallback because the supported
Kuadrant contract is v1 and the latest gateway explicitly removes v1alpha1 serving.

## Evidence boundary

- **Upstream evidence**: the Kuadrant MCP Gateway v1.0.2 tagged CRD described above.
- **Repository evidence**: Ginkgo unit tests, envtest CRD/fixtures, and active fleet E2E
  manifests use v1 and prove the registry's filtering and readiness behavior.
- **Live evidence**: no live Kuadrant deployment was available for issue #2594 in this
  checkout. This change does not claim live OCP or production qualification.
