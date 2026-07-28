# BR-PLATFORM-009: Helm Chart Gateway/APIFrontend Ingress Parity with the Kubernaut Operator

**Business Requirement ID**: BR-PLATFORM-009
**Category**: Platform
**Priority**: P2
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-07-27

---

## Business Need

### Problem Statement

The Kubernaut Operator exposes Gateway and APIFrontend externally via OCP `Route` resources
(`GatewayRoute`/`APIFrontendRoute` in `kubernaut-operator/internal/resources/ocp.go`), each gated
by an opt-in/opt-out `Enabled` flag on the CR. The Helm chart has no equivalent for either service:
there is no `templates/gateway/ingress.yaml` or `templates/apifrontend/ingress.yaml`, so
Helm-chart deployments that need external access to these two machine-facing pipeline entry points
(e.g. AlertManager running outside the cluster, or an external MCP/API client) have no
chart-supported way to get it, other than a manually pinned `NodePort` (test/CI-oriented,
[Issue #1737](https://github.com/jordigilh/kubernaut/issues/1737)) or resources managed entirely
outside the chart.

Separately, `console.ingress.enabled` (BR-PLATFORM-006) defaulted to `true` (opt-out) on the
rationale that Console, being browser-facing, needed "a working default." In practice this made
Console's default external-exposure posture inconsistent with the rest of the platform's pipeline
entry points, and didn't reflect that Console is itself optional, replaceable UI tooling — not a
pipeline component a deployment depends on.

**Impact**: No chart-supported path to expose Gateway/APIFrontend externally without a manual
NodePort or out-of-chart resources; Console's opt-out Ingress default was inconsistent with the
opt-in posture appropriate for external exposure of pipeline-adjacent services.

---

## Business Objective

Bring the Helm chart to parity with the Kubernaut Operator's Gateway/APIFrontend Route feature,
adapted to the chart's vanilla-Kubernetes-only positioning, and align all three externally-facing
services (Gateway, APIFrontend, Console) on a consistent opt-in exposure posture.

### Success Criteria

1. `gateway.ingress.enabled=true` renders a `networking.k8s.io/v1` Ingress targeting
   `gateway-service`'s `http` port — the vanilla-Kubernetes equivalent of `GatewayRoute`. Opt-in,
   disabled by default (`gateway.ingress.enabled=false`).
2. `apifrontend.ingress.enabled=true` renders an Ingress targeting `apifrontend`'s `https` port —
   the equivalent of `APIFrontendRoute`. Opt-in, disabled by default
   (`apifrontend.ingress.enabled=false`), gated on `apifrontend.enabled=true`.
3. Unlike `console.ingress.host` (required whenever `console.enabled=true`, per BR-PLATFORM-006),
   neither Gateway's nor APIFrontend's `ingress.host` is required: no internal component depends
   on the hostname being set (Console's requirement comes specifically from oauth2-proxy's OIDC
   redirect URL construction, which has no analog here). An empty `host` renders a catch-all
   Ingress rule.
4. `console.ingress.enabled` default flips from `true` to `false` (see BR-PLATFORM-006's
   Success Criterion 3 superseded note), aligning Console with Gateway/APIFrontend's opt-in
   posture. `console.ingress.host` remains required whenever `console.enabled=true`, independent
   of `console.ingress.enabled`'s value.
5. Zero regression for the (default) disabled state on all three — no Ingress resources render,
   no new required fields for existing installs (Console's pre-existing `console.ingress.host`
   requirement, gated on `console.enabled`, is unaffected).

---

## Functional Requirements

- **FR-1**: `gateway.ingress.{enabled,className,host,annotations,tls.secretName}` in
  `values.yaml`/`values.schema.json` controls a new `templates/gateway/ingress.yaml`.
- **FR-2**: `apifrontend.ingress.{enabled,className,host,annotations,tls.secretName}` controls a
  new `templates/apifrontend/ingress.yaml`, gated on `apifrontend.enabled` (mirroring every other
  APIFrontend resource).
- **FR-3**: A shared `definitions.httpIngress` schema in `values.schema.json` backs all three
  services' `ingress` property (Gateway, APIFrontend, Console), avoiding duplicated shape
  definitions and guaranteeing the `enabled: false` default stays consistent across all three.
- **FR-4**: `console.ingress.enabled`'s default value changes from `true` to `false` in both
  `values.yaml` and `values.schema.json` (via FR-3's shared definition). No template logic change
  to `templates/console/console.yaml`'s existing `console.ingress.host` requirement.
- **FR-5**: APIFrontend's HTTPS backend (self-signed inter-service certificate) requires a
  controller-specific backend-protocol/TLS-passthrough annotation to reencrypt correctly (e.g.
  ingress-nginx: `nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"` +
  `nginx.ingress.kubernetes.io/proxy-ssl-verify: "off"`, since the backend cert isn't signed by a
  CA the controller trusts). The chart does not hardcode one controller's annotation syntax —
  documented as a requirement of `apifrontend.ingress.annotations` instead, since vanilla
  `networking.k8s.io/v1` has no equivalent of OCP Route's
  `TLSConfig.DestinationCACertificate` for a self-signed backend.
- **FR-6**: NetworkPolicy ingress from an external ingress-controller namespace/CIDR to
  Gateway/APIFrontend/Console is already covered by the existing
  `networkPolicies.<service>.{ingressNamespaces,ingressCIDRs,ingressNamespaceSelectors}` knobs
  (Issue #1737 follow-up) — no new NetworkPolicy work required by this BR.

---

## Non-Goals

- Does not port the Operator's `GatewayRoute`/`APIFrontendRoute` (OCP-specific) — replaced by a
  generic Ingress per FR-1/FR-2, consistent with the chart's vanilla-Kubernetes-only scope.
- Does not add a convenience field abstracting ingress-controller-specific backend-protocol
  annotations (e.g. a `backendProtocol: HTTPS` shorthand) — annotation syntax varies enough across
  controllers (ingress-nginx, Traefik, HAProxy, Istio Gateway API) that a chart-level abstraction
  would either be incomplete or require per-controller branching; raw `annotations` passthrough is
  simpler and already the established pattern (Console's Ingress, ServiceMonitor, etc.).
- Does not change `console.ingress.host`'s required-when-enabled behavior (BR-PLATFORM-006 FR-6) —
  only the `ingress.enabled` default.

---

## Related Decisions

- **Builds on**: BR-PLATFORM-006 (Console Ingress; this BR flips its default and extends the same
  pattern to Gateway/APIFrontend), BR-PLATFORM-005 (APIFrontend NetworkPolicy parity).
- **Tracked in**: [Issue #1737](https://github.com/jordigilh/kubernaut/issues/1737) follow-up
  (E2E Helm migration surfaced the need for configurable external exposure alongside the
  NetworkPolicy `ingressCIDRs`/`ingressNamespaceSelectors` work in the same issue).

---

**Document Status**: ✅ Approved
**Priority**: P2 — closes a chart-supported external-access gap for Gateway/APIFrontend and
aligns Console's exposure default with the rest of the platform
