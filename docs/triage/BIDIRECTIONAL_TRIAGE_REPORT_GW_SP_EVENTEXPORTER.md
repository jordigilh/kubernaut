# Bidirectional Triage Report: Gateway, Signal Processing, Event Exporter

**Scope**: Gateway Service, Signal Processing Service, Event Exporter  
**Date**: 2026-03-04  
**Direction 1**: Code → Docs (things in code not documented or documented incorrectly)  
**Direction 2**: Docs → Code (things in docs/BRs planned for v1.0 but not implemented or not wired)

---

## Gateway Service

### Direction 1: Code → Docs

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| GW-1 | INCONSISTENCY | **HIGH** | `kubernaut-docs/docs/user-guide/signals.md` | **Webhook paths**: Docs say `POST /api/v1/alerts` and `POST /api/v1/events`. Code uses `POST /api/v1/signals/prometheus` and `POST /api/v1/signals/kubernetes-event`. | `pkg/gateway/adapters/prometheus_adapter.go:75` → `GetRoute() = "/api/v1/signals/prometheus"`; `pkg/gateway/adapters/kubernetes_event_adapter.go:114` → `GetRoute() = "/api/v1/signals/kubernetes-event"`. |
| GW-2 | INCONSISTENCY | **HIGH** | `kubernaut-docs/docs/user-guide/signals.md` | **AlertManager URL**: Docs show `http://gateway.kubernaut-system.svc:8080/api/v1/alerts`. Code uses `/api/v1/signals/prometheus`. Service name: `gateway-service` (not `gateway`). | `charts/kubernaut/templates/gateway/gateway.yaml:166` → Service `gateway-service`; `deploy/demo/helm/kube-prometheus-stack-values.yaml:57` → correct URL. |
| GW-3 | ~~INCONSISTENCY~~ **RESOLVED** | ~~**HIGH**~~ | `charts/kubernaut/templates/gateway/gateway.yaml` | **Fixed in #267**: Helm ConfigMap and all deploy configs now use `processing.deduplication.cooldownPeriod: 5m`, matching Go struct. Also fixed in `deploy/demo/base/platform/gateway.yaml`, `deploy/gateway/base/02-configmap.yaml`, `deploy/gateway/02-configmap.yaml`, and `test/unit/gateway/config/testdata/valid-config.yaml`. | Commits 6cb95ef9f, follow-up |
| GW-4 | GAP-IN-DOCS | MEDIUM | `kubernaut-docs/docs/user-guide/signals.md` | **Readiness probe**: Docs say "Redis + K8s connectivity". Gateway is Redis-free (DD-GATEWAY-012). | `pkg/gateway/server.go:956-961` → `readinessHandler` comments say "Redis check REMOVED - Gateway is now Redis-free". |
| GW-5 | GAP-IN-DOCS | MEDIUM | `kubernaut-docs/docs/api-reference/crds.md` | **RemediationRequest spec**: `signalFingerprint` not in doc table. CRD has it as top-level spec field. | `api/remediation/v1alpha1/remediationrequest_types.go:249` → `SignalFingerprint string`; CRD schema `config/crd/bases/kubernaut.ai_remediationrequests.yaml:166`. |
| GW-6 | INCONSISTENCY | LOW | `docs/services/stateless/gateway-service/implementation.md` | **Example URL**: Docs show `http://gateway-service.kubernaut-system:8080/api/v1/alerts/prometheus`. Code uses `/api/v1/signals/prometheus`. | Line 1348. |

### Direction 2: Docs → Code

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| GW-7 | None | — | — | No doc/BR features found that are missing in code. | — |

---

## Signal Processing Service

### Direction 1: Code → Docs

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| SP-1 | GAP-IN-DOCS | MEDIUM | `kubernaut-docs/docs/architecture/signal-processing.md` | **Rego policy locations**: Doc lists policies but not file paths. Code has: `environment.rego`, `priority.rego`, `business.rego`, `severity.rego`, `customlabels.rego` at `/etc/signalprocessing/policies/`. | `cmd/signalprocessing/main.go:228-363`; `deploy/signalprocessing/policies/` has `environment.rego`, `priority.rego`, `business.rego` only. |
| SP-2 | GAP-IN-DOCS | MEDIUM | `kubernaut-docs/docs/architecture/signal-processing.md` | **Rego policy names**: Doc lists `environment`, `severity`, `priority`, `business`, `customlabels`, `signalmode`. `signalmode` is YAML-based (config), not Rego. | `cmd/signalprocessing/main.go:334-337` → `SignalModeClassifier` uses `proactive-signal-mappings.yaml` (BR-SP-106). |
| SP-3 | GAP-IN-DOCS | LOW | — | **Deploy vs Helm**: `deploy/signalprocessing/policies/` has only 3 policies; Helm chart embeds all 5 (environment, priority, business, severity, customlabels) inline. Doc doesn't clarify deploy vs Helm policy source. | `charts/kubernaut/templates/signalprocessing/signalprocessing.yaml` embeds all policies. |
| SP-4 | GAP-IN-DOCS | LOW | `kubernaut-docs/docs/api-reference/crds.md` | **SignalProcessing CRD**: Doc mentions `enrichmentConfig` but not enrichment timeout, cache TTL, or Rego ConfigMap names. | `internal/config/signalprocessing/config.go` (or config package) has `RegoConfigMapName`, `RegoConfigMapKey`. |

### Direction 2: Docs → Code

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| SP-5 | GAP-IN-CODE | MEDIUM | `kubernaut-docs/docs/architecture/signal-processing.md` | **Rego policy locations**: Doc says "Rego policies" in `config/rego/`. Signal Processing policies live in `deploy/signalprocessing/policies/` and `config/` — no `config/rego/signalprocessing/` directory. | `config/rego/` only has `aianalysis/approval.rego`. |
| SP-6 | — | — | BR-SP-* | BR-SP-001 through BR-SP-030+ are high-level; many map to implemented features. No specific v1.0 BR found unimplemented. | — |

---

## Event Exporter

### Direction 1: Code → Docs

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| EE-1 | GAP-IN-DOCS | MEDIUM | `kubernaut-docs` | **Event Exporter**: No dedicated doc page. Mentioned in signals.md and architecture-overview.md but not in configuration reference. | `DOCUMENTATION_TRIAGE_REPORT.md` (kubernaut-docs) notes: "eventExporter — No table for eventExporter.image, eventExporter.resources". |
| EE-2 | GAP-IN-DOCS | LOW | `kubernaut-docs` | **Event Exporter config**: Helm chart uses `ghcr.io/resmoio/kubernetes-event-exporter`. ConfigMap structure (namespace filter, drop rules, webhook endpoint) not documented. | `charts/kubernaut/templates/event-exporter/event-exporter.yaml:51-73` |

### Direction 2: Docs → Code

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| EE-3 | — | — | — | Event Exporter is external (Resmo). Kubernaut deploys it; no Kubernaut code implements it. | — |

---

## Summary by Severity

| Severity | Count | Items |
|----------|-------|-------|
| HIGH | 3 | GW-1, GW-2, GW-3 |
| MEDIUM | 5 | GW-4, GW-5, SP-1, SP-2, EE-1 |
| LOW | 4 | GW-6, SP-3, SP-4, EE-2 |

---

## Recommended Actions

### Priority 1 (HIGH)

1. **GW-1**: Update `kubernaut-docs/docs/user-guide/signals.md` to use correct webhook paths:
   - `POST /api/v1/signals/prometheus` (AlertManager)
   - `POST /api/v1/signals/kubernetes-event` (Kubernetes Events)

2. **GW-2**: Update AlertManager URL example to use `gateway-service` and `/api/v1/signals/prometheus`.

### Priority 2 (HIGH)

3. **GW-3**: Fix Helm Gateway ConfigMap: change `processing.deduplication.ttl` to `processing.deduplication.cooldownPeriod` (prevents duplicate RRs after remediation completion).

4. **GW-4**: Update readiness probe docs to remove Redis references (Gateway is Redis-free).

5. **GW-5**: Add `signalFingerprint` to RemediationRequest spec table in CRD docs.

6. **SP-1, SP-2**: Document signal-processing Rego policy paths and clarify that `signalmode` uses YAML config.

7. **EE-1**: Add Event Exporter configuration section to user/configuration docs.

### Priority 4 (LOW)

8. **GW-6**: Fix implementation.md example URL.

9. **SP-3**: Document deploy vs Helm policy source (deploy has 3 files; Helm embeds all 5).

---

## Configuration Keys Reference (Gateway)

| Code Key | Helm/Config Value | Status |
|----------|-------------------|--------|
| `processing.deduplication.cooldownPeriod` | `cooldownPeriod: "5m"` | Correct |
| ~~`processing.deduplication.ttl`~~ | ~~`ttl: 5m` (Helm)~~ | **RESOLVED** — all configs now use `cooldownPeriod` |

---

## Webhook Endpoints Reference

| Source | Doc Path | Actual Path |
|--------|----------|-------------|
| AlertManager | `/api/v1/alerts` | `/api/v1/signals/prometheus` |
| K8s Events (Event Exporter) | `/api/v1/events` | `/api/v1/signals/kubernetes-event` |
