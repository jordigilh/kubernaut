# BR-PLATFORM-006: Helm Chart Web Console Parity with the Kubernaut Operator

**Business Requirement ID**: BR-PLATFORM-006
**Category**: Platform
**Priority**: P2
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-07-06

---

## Business Need

### Problem Statement

The Kubernaut Operator deploys an optional standalone web console (`ConsoleDeployment`/
`ConsoleService`/`ConsoleRoute` in `kubernaut-operator/internal/resources/console.go`) — a static
SPA fronted by an `oauth2-proxy` sidecar, giving users a browser-based A2A chat UI authenticated
against the same OIDC provider as APIFrontend. The Helm chart has no equivalent at all: there is no
console Deployment, Service, ConfigMap, or ingress path in `charts/kubernaut`. Helm-chart
deployments therefore have no supported way to run the web console, only the MCP/A2A API surface
via APIFrontend directly.

**Impact**: Users choosing the Helm chart over the Operator (e.g. for non-OpenShift clusters) lose
a user-facing feature that Operator-managed deployments get by default (once opted in), with no
documented workaround.

---

## Business Objective

Bring the Helm chart to functional parity with the Kubernaut Operator's console feature, adapted to
the chart's vanilla-Kubernetes-only positioning (no OCP Route dependency, per BR-PLATFORM-004's
OCP-removal scope).

### Success Criteria

1. `console.enabled=true` renders a Deployment (oauth2-proxy + console containers), Service, and
   nginx ConfigMap functionally equivalent to the Operator's, reverse-proxying `/a2a/`, `/mcp`, and
   `/.well-known/` to APIFrontend with the same CSP/security headers and rate limits.
2. Opt-in, disabled by default (`console.enabled=false`), matching `ConsoleSpec.Enabled`.
3. External (browser) access is provided via a standard `networking.k8s.io/v1` Ingress — the
   vanilla-Kubernetes equivalent of the Operator's OCP Route. Toggleable via
   `console.ingress.enabled` for users who front the `console` Service with their own
   Ingress/Route/mesh gateway instead.
   > **Superseded by BR-PLATFORM-009**: this criterion originally made `console.ingress.enabled`
   > default to `true` (opt-out), on the rationale that Console -- unlike machine-facing
   > Gateway/APIFrontend -- is browser-facing so "a working default matters more here."
   > BR-PLATFORM-009 flips this to opt-in (`false`), for consistency with Gateway/APIFrontend's
   > `ingress.enabled` (both later added, also opt-in) and because Console is itself optional,
   > replaceable UI tooling in front of APIFrontend -- not something the chart should expose
   > externally without an explicit choice. This is a deliberate deviation from the Operator's
   > `ConsoleRouteSpec`, which defaults to opt-out.
4. Fails fast at template-render time (before any resources are applied) when `console.enabled=true`
   and any of the following are missing: `console.auth.secretName`, a resolvable OIDC issuer
   (`apifrontend.config.auth.issuerURL` or `.jwtProviders[0].issuerURL`), or `console.ingress.host`
   (needed for the oauth2-proxy OIDC redirect URL regardless of whether the chart's own Ingress is
   used).
5. Zero regression for the (default) disabled state — no console resources render, no new required
   fields for existing installs.

---

## Functional Requirements

- **FR-1**: `console.{enabled,replicas,pdb,auth.secretName,oauth2Proxy.image,resources}` in
  `values.yaml`/`values.schema.json`, mirroring `ConsoleSpec` (`auth.secretName` ↔
  `ConsoleAuthSpec.SecretName`; `resources` ↔ `ConsoleSpec.Resources`).
- **FR-2**: `console.ingress.{enabled,className,host,annotations,tls.secretName}` controls a new
  `templates/console/ingress.yaml` — the Route-to-Ingress adaptation for vanilla Kubernetes.
- **FR-3**: `networkPolicies.console.{enabled,ingressNamespaces}` controls a new
  `templates/console/networkpolicy.yaml` (default-deny + explicit allow, consistent with every
  other chart component — a chart-only enhancement, since the Operator has no NetworkPolicy for
  Console).
- **FR-4**: The console Deployment sets `automountServiceAccountToken: false` (the console makes no
  Kubernetes API calls) — a chart-only hardening enhancement beyond what the Operator does today.
- **FR-5**: The `kubernaut.console.issuerURL` helper mirrors
  `KubernautSpec.ConsoleIssuerURL()` exactly: `jwtProviders[0].issuerURL` takes precedence over the
  single-provider `issuerURL` shortcut.
- **FR-6**: Validation happens via `{{ fail ... }}` guards at the top of
  `templates/console/console.yaml`, consistent with the chart's existing required-field validation
  pattern (Rego policies, LLM config, fleet OAuth2, etc.) — not via `values.schema.json` `required`
  arrays, since the requirement is conditional on `console.enabled`.

---

## Non-Goals

- Does not port the Operator's `ConsoleRoute` (OCP-specific) — replaced by a generic Ingress per
  FR-2, consistent with the chart's vanilla-Kubernetes-only scope.
- Does not make `console.replicas` >1 meaningfully different from the Operator (which hardcodes 1) —
  the chart exposes it as configurable for flexibility, but this is not a parity requirement.
- Does not add K8s API RBAC for the console — it has none in the Operator either, and the chart's
  `automountServiceAccountToken: false` makes this explicit rather than implicit.

---

## Related Decisions

- **Tracked in**: [Issue #1589](https://github.com/jordigilh/kubernaut/issues/1589) (follow-up
  triage after BR-PLATFORM-003/004/005).
- **Builds on**: BR-PLATFORM-005 (security/NetworkPolicy parity) — same triage methodology
  (compare `kubernaut-operator/internal/resources/*.go` against the Helm chart), same Issue #1589
  initiative.

---

**Document Status**: ✅ Approved
**Priority**: P2 — closes a user-facing feature gap for non-Operator deployments
