# BR-PLATFORM-003: Helm Chart Observability and Autoscaling Parity

**Business Requirement ID**: BR-PLATFORM-003
**Category**: Platform
**Priority**: P2
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-07-06

---

## Business Need

### Problem Statement

The Kubernaut Operator (`../kubernaut-operator`) provisions `ServiceMonitor`, `PrometheusRule`,
and `HorizontalPodAutoscaler` resources for the services it manages, giving OpenShift/OLM-based
deployments observability and autoscaling out of the box. The Helm chart (`charts/kubernaut/`),
which is the only deployment path for non-OpenShift (vanilla Kubernetes) clusters, has neither.
A Helm-chart operator has no way to get Prometheus Operator scrape configs, alerting rules, or
horizontal pod autoscaling without hand-authoring them outside the chart — the exact kind of
manual, error-prone step the chart's Helm-native auto-discovery UX work (PR #1571) is designed to
eliminate.

**Current Limitations**:
- No `ServiceMonitor` resources for any of the 11 Helm-chart-managed services, so a
  `kube-prometheus-stack`/Prometheus Operator install on a vanilla cluster does not automatically
  discover Kubernaut's `/metrics` endpoints.
- No `PrometheusRule` for DataStorage or APIFrontend (both already have Operator-side alerting
  rules covering availability, latency, and error-budget conditions that Helm-chart users cannot
  benefit from).
- No `HorizontalPodAutoscaler` for DataStorage or APIFrontend, so Helm-chart deployments cannot
  scale these services under load without a hand-authored HPA manifest applied out-of-band.

**Impact**:
- Non-OCP Kubernaut deployments have materially worse production-readiness than OCP/Operator
  deployments for the same services, undermining the chart's goal of being a first-class,
  non-OCP-native deployment path (Issue #1589).
- Operators must hand-roll monitoring/scaling manifests that drift from the Operator's
  already-reviewed, production-validated definitions.

---

## Business Objective

Bring the Helm chart to observability and autoscaling parity with the Kubernaut Operator for the
portable (non-OCP-specific) subset of monitoring/autoscaling resources, without requiring the
Prometheus Operator CRDs or `autoscaling/v2` HPA support to be present for chart users who don't
need them.

### Success Criteria

1. Every Helm-chart-managed service optionally renders a `ServiceMonitor` when
   `monitoring.serviceMonitor.enabled=true` **and** the `monitoring.coreos.com/v1` API is present
   on the target cluster (CRD-gated, matching the chart's existing `kubernaut.hasClusterAccess`-style
   discovery pattern) — zero impact on clusters without the Prometheus Operator installed.
2. DataStorage and APIFrontend optionally render a `PrometheusRule` under the same CRD gate, with
   alerting rule content matching the Operator's `internal/resources/monitoring.go` definitions.
3. DataStorage and APIFrontend optionally render a `HorizontalPodAutoscaler`
   (`autoscaling/v2`, unconditional — no CRD gate needed, this is a stable core API) when
   `<service>.autoscaling.enabled=true`, matching the Operator's `internal/resources/hpa.go`
   scaling targets (CPU 75% / Memory 80%, `minReplicas: 1` / `maxReplicas: 5`).
4. All three resource types default to disabled (`enabled: false`) — no behavior change for
   existing chart installs that don't opt in.

---

## Functional Requirements

- **FR-1**: New `kubernaut.monitoring.serviceMonitor` helper in `templates/_helpers.tpl` generating
  one `ServiceMonitor` per service (gateway, datastorage, aianalysis, signalprocessing,
  remediationorchestrator, workflowexecution, effectivenessmonitor, notification, kubernaut-agent,
  authwebhook, apifrontend), scraping the `metrics` port at `/metrics` every 15s.
- **FR-2**: `templates/datastorage/prometheusrule.yaml` and `templates/apifrontend/prometheusrule.yaml`,
  each containing the alert groups already defined for that service in the Operator
  (`monitoring.go`), rendered only when the CRD is present and `monitoring.serviceMonitor.enabled=true`.
- **FR-3**: `templates/datastorage/hpa.yaml` and `templates/apifrontend/hpa.yaml`
  (`autoscaling/v2` `HorizontalPodAutoscaler`), with `<service>.autoscaling.{enabled,minReplicas,
  maxReplicas,cpuTarget,memoryTarget}` values controlling behavior, default `enabled: false`.
- **FR-4**: `values.schema.json` and `README.md` updated to document the new value blocks.

---

## Non-Goals

- No OCP-specific monitoring resources (Thanos querier / alertmanager-main auto-discovery is
  covered separately and is being *removed*, not extended — see Issue #1589 Part B).
- No live E2E validation against a real Prometheus Operator install in CI for this requirement;
  covered via `helm template --api-versions monitoring.coreos.com/v1` template-only tests
  (`ST-CHART-MON-001/002`) plus a live `autoscaling/v2` HPA assertion (`ST-CHART-MON-003`), since
  `autoscaling/v2` is a stable core API already present in the smoke-test Kind cluster.

---

## Related Decisions

- **Tracked in**: [Issue #1589](https://github.com/jordigilh/kubernaut/issues/1589) (Helm chart
  Operator-parity gaps + OCP removal).
- **Source of truth for ported content**: `kubernaut-operator/internal/resources/monitoring.go`,
  `kubernaut-operator/internal/resources/hpa.go`.

---

**Document Status**: ✅ Approved
**Priority**: P2 — production-readiness parity, not a functional blocker
