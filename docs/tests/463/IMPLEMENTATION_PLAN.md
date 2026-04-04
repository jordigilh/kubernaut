# Implementation Plan: Unified Prometheus/AlertManager Monitoring Config

**Issue**: #463
**Test Plan**: [TP-463-v1.0](TEST_PLAN.md)
**Branch**: `development/v1.4`
**Created**: 2026-03-04

---

## Overview

This plan adds a single `monitoring:` top-level block to the Helm chart that the templates propagate to both EffectivenessMonitor (EM) and Kubernaut Agent (KA). Legacy keys become fallback aliases. OCP is auto-detected via `.Capabilities.APIVersions`.

### Current state (two places)

```yaml
# Place 1: EM
effectivenessmonitor:
  external:
    prometheusUrl: "http://..."
    prometheusEnabled: false

# Place 2: KA
kubernautAgent:
  prometheus:
    enabled: false
    url: ""
```

### Target state (one place + fallback)

```yaml
# NEW: single source of truth
monitoring:
  prometheus:
    enabled: false
    url: ""
    tlsCaFile: ""
  alertManager:
    enabled: false
    url: ""
    tlsCaFile: ""

# LEGACY: still works as fallback, deprecated in NOTES.txt
effectivenessmonitor:
  external: ...
kubernautAgent:
  prometheus: ...
```

### Files to modify

| # | File | Change |
|---|------|--------|
| 1 | `charts/kubernaut/values.yaml` | Add `monitoring:` top-level block |
| 2 | `charts/kubernaut/values.schema.json` | Add schema for `monitoring:` block |
| 3 | `charts/kubernaut/templates/_helpers.tpl` | Add monitoring helper templates (resolved URL, fallback logic, OCP detection) |
| 4 | `charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml` | Read from monitoring helpers instead of `effectivenessmonitor.external.*` directly |
| 5 | `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml` | Read from monitoring helpers instead of `kubernautAgent.prometheus.*` directly |
| 6 | `charts/kubernaut/values-ocp.yaml` | Add `monitoring:` overrides alongside existing legacy keys |
| 7 | `charts/kubernaut/templates/NOTES.txt` | Add deprecation warning |
| 8 | `scripts/helm-smoke-test.sh` | Extend with monitoring tests |

---

## Phase 1: TDD RED — Failing Smoke Tests

**Goal**: Add smoke test cases to `scripts/helm-smoke-test.sh` that fail because the `monitoring:` block doesn't exist yet.

### Phase 1.1: Add test cases

Extend `scripts/helm-smoke-test.sh` with a new test tier for monitoring:

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-MON-463-001 | `monitoring.prometheus.enabled=true` + URL → EM ConfigMap has `prometheusUrl` and KA ConfigMap has `prometheus_url` | No `monitoring` key in values |
| UT-MON-463-002 | EM ConfigMap reads `monitoring.prometheus.url` | Template reads from `effectivenessmonitor.external` only |
| UT-MON-463-003 | KA SDK ConfigMap reads `monitoring.prometheus.url` | Template reads from `kubernautAgent.prometheus` only |
| UT-MON-463-004 | `monitoring.prometheus.enabled=false` → no Prometheus in either | May pass trivially (validates contract) |
| UT-MON-463-005 | Both Prometheus and AlertManager enabled → both configured | `monitoring` key doesn't exist |
| UT-MON-463-006 | Legacy EM keys still work | Should pass (existing behavior) |
| UT-MON-463-007 | Legacy KA keys still work | Should pass (existing behavior) |
| UT-MON-463-008 | OCP auto-detection with `--api-versions route.openshift.io/v1` | No OCP-aware helper exists |
| UT-MON-463-009 | TLS CA volume mount when `tlsCaFile` set | No `monitoring.prometheus.tlsCaFile` key |
| UT-MON-463-010 | OCP RBAC ClusterRoleBinding when monitoring enabled | Existing RBAC is per-component, not unified |
| UT-MON-463-011 | `monitoring.*` overrides legacy keys when both set | `monitoring` key doesn't exist |
| UT-MON-463-012 | NOTES.txt deprecation warning with legacy keys | No deprecation logic |
| UT-MON-463-013 | `helm lint --strict` passes | Schema doesn't include `monitoring` |

### Phase 1 Checkpoint

- [ ] New test cases added to `scripts/helm-smoke-test.sh`
- [ ] Tests UT-MON-463-001 through -005, -008, -009, -010, -011, -012: FAIL
- [ ] Tests UT-MON-463-006, -007: PASS (backward compat — existing behavior)
- [ ] Test UT-MON-463-013: FAIL (schema missing monitoring)

---

## Phase 2: TDD GREEN — Minimal Implementation

**Goal**: Make all failing tests pass.

### Phase 2.1: Values + Schema

**File**: `charts/kubernaut/values.yaml`

Add at the top level (before `networkPolicies:`):
```yaml
# -- Unified monitoring configuration: single source for Prometheus/AlertManager endpoints.
# Propagated to both EffectivenessMonitor and Kubernaut Agent.
# Legacy keys (effectivenessmonitor.external.*, kubernautAgent.prometheus.*) are
# deprecated fallback aliases. See NOTES.txt for migration guidance.
monitoring:
  prometheus:
    enabled: false
    url: ""
    tlsCaFile: ""
  alertManager:
    enabled: false
    url: ""
    tlsCaFile: ""
```

**File**: `charts/kubernaut/values.schema.json`

Add `monitoring` property at root with `prometheus` and `alertManager` sub-objects.

**Verification**: `helm lint --strict` passes → UT-MON-463-013 passes

### Phase 2.2: Helper templates

**File**: `charts/kubernaut/templates/_helpers.tpl`

Add helper templates:

```
{{- define "kubernaut.monitoring.prometheus.enabled" -}}
  Resolves: monitoring.prometheus.enabled → effectivenessmonitor.external.prometheusEnabled → kubernautAgent.prometheus.enabled → false
{{- end -}}

{{- define "kubernaut.monitoring.prometheus.url" -}}
  Resolves: monitoring.prometheus.url → effectivenessmonitor.external.prometheusUrl → kubernautAgent.prometheus.url → ""
{{- end -}}

{{- define "kubernaut.monitoring.alertManager.enabled" -}}
  Resolves: monitoring.alertManager.enabled → effectivenessmonitor.external.alertManagerEnabled → false
{{- end -}}

{{- define "kubernaut.monitoring.alertManager.url" -}}
  Resolves: monitoring.alertManager.url → effectivenessmonitor.external.alertManagerUrl → ""
{{- end -}}

{{- define "kubernaut.monitoring.isOCP" -}}
  Returns "true" if .Capabilities.APIVersions.Has "route.openshift.io/v1"
{{- end -}}

{{- define "kubernaut.monitoring.prometheus.tlsCaFile" -}}
  Resolves: monitoring.prometheus.tlsCaFile → effectivenessmonitor.external.tlsCaFile → ""
  If OCP detected and empty, defaults to "/etc/ssl/certs/service-ca.crt"
{{- end -}}

{{- define "kubernaut.monitoring.usesLegacyKeys" -}}
  Returns "true" if effectivenessmonitor.external.* or kubernautAgent.prometheus.* are set
  while monitoring.* is not. Used for deprecation warning.
{{- end -}}
```

**Verification**: Helpers callable from templates

### Phase 2.3: EM template update

**File**: `charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml`

Replace direct references to `effectivenessmonitor.external.prometheusUrl` etc. with calls to the monitoring helper templates. The helpers resolve new keys first, then fall back to legacy keys.

**Verification**: UT-MON-463-002, UT-MON-463-006 pass

### Phase 2.4: KA template update

**File**: `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`

Replace direct references to `kubernautAgent.prometheus.*` with calls to the monitoring helper templates for the SDK ConfigMap and Prometheus toolset configuration.

**Verification**: UT-MON-463-003, UT-MON-463-007 pass

### Phase 2.5: OCP detection + defaults

In helper templates, when `.Capabilities.APIVersions.Has "route.openshift.io/v1"` is true and `monitoring.prometheus.url` is empty:
- Default Prometheus URL to `https://prometheus-k8s.openshift-monitoring.svc:9091`
- Default AlertManager URL to `https://alertmanager-main.openshift-monitoring.svc:9094`
- Default TLS CA to `/etc/ssl/certs/service-ca.crt`

**Verification**: UT-MON-463-008 passes

### Phase 2.6: TLS volume mounts

Add conditional volume and volumeMount to both EM and KA Deployment templates when `tlsCaFile` is set. Use existing OCP service-CA ConfigMap injection pattern from the EM template.

**Verification**: UT-MON-463-009 passes

### Phase 2.7: OCP RBAC

When monitoring is enabled and OCP is detected, create `ClusterRoleBinding` granting both EM and KA service accounts `cluster-monitoring-view` access. Extend or unify the existing per-component RBAC blocks.

**Verification**: UT-MON-463-010 passes

### Phase 2.8: values-ocp.yaml update

Add `monitoring:` block to the OCP overlay values, alongside the existing legacy keys (both present for transition):

```yaml
monitoring:
  prometheus:
    enabled: true
    url: "https://prometheus-k8s.openshift-monitoring.svc:9091"
    tlsCaFile: "/etc/ssl/certs/service-ca.crt"
  alertManager:
    enabled: true
    url: "https://alertmanager-main.openshift-monitoring.svc:9094"
    tlsCaFile: "/etc/ssl/certs/service-ca.crt"
```

### Phase 2.9: NOTES.txt deprecation warning

Add conditional block to `NOTES.txt`:
```
{{- if include "kubernaut.monitoring.usesLegacyKeys" . }}
⚠️  DEPRECATION: You are using legacy monitoring configuration keys:
   - effectivenessmonitor.external.prometheusUrl → use monitoring.prometheus.url
   - kubernautAgent.prometheus.url → use monitoring.prometheus.url
   These keys will be removed in v1.5. See Issue #463 for migration guide.
{{- end }}
```

**Verification**: UT-MON-463-012 passes

### Phase 2 Checkpoint

- [ ] All 13 smoke tests pass
- [ ] `helm lint --strict` passes
- [ ] `helm template` with legacy-only values produces same output as before (regression check)
- [ ] `helm template` with new `monitoring.*` values produces correct EM + KA config

---

## Phase 3: TDD REFACTOR — Code Quality

**Goal**: Improve template readability and reduce duplication.

### Phase 3.1: DRY helper consolidation

Ensure all monitoring logic is in `_helpers.tpl`, not inlined in component templates. Both EM and KA templates should only call helpers, not duplicate resolution logic.

### Phase 3.2: Comments and documentation

Add inline comments in `_helpers.tpl` explaining the resolution order (new → legacy → OCP default → empty).

### Phase 3.3: Schema documentation

Add `description` fields to all `monitoring.*` properties in `values.schema.json`.

### Phase 3 Checkpoint

- [ ] All 13 tests still pass
- [ ] Templates are readable with clear helper names
- [ ] No duplicated resolution logic between EM and KA templates

---

## Phase 4: Due Diligence & Commit

### Phase 4.1: Comprehensive audit

- [ ] All rendering permutations tested: new-only, legacy-only, both (new wins), neither (disabled), OCP, vanilla
- [ ] `helm lint --strict` passes for all permutations
- [ ] Legacy values produce identical ConfigMap content (backward compat)
- [ ] OCP overlay values produce correct OCP-specific defaults
- [ ] NOTES.txt deprecation warning renders correctly
- [ ] Schema validates new values structure
- [ ] No broken existing smoke tests

### Phase 4.2: Commit in logical groups

| Commit # | Scope | Files |
|----------|-------|-------|
| 1 | `test(#463): TDD RED — failing smoke tests for unified monitoring config` | `scripts/helm-smoke-test.sh` |
| 2 | `feat(#463): add monitoring: top-level block to Helm values and schema` | `values.yaml`, `values.schema.json` |
| 3 | `feat(#463): add monitoring helper templates with OCP detection and fallback` | `_helpers.tpl` |
| 4 | `feat(#463): wire EM and KA templates to use unified monitoring helpers` | EM template, KA template |
| 5 | `feat(#463): add TLS volume mounts and OCP RBAC for monitoring` | EM + KA Deployment templates, RBAC templates |
| 6 | `chore(#463): update OCP overlay and add deprecation warning` | `values-ocp.yaml`, `NOTES.txt` |

---

## Estimated Effort

| Phase | Effort |
|-------|--------|
| Phase 1 (RED) | 0.5 day |
| Phase 2 (GREEN) | 2 days |
| Phase 3 (REFACTOR) | 0.5 day |
| Phase 4 (Due Diligence) | 0.5 day |
| **Total** | **3.5 days** |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial implementation plan |
