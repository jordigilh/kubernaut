# Service Maturity Requirements

**Version**: 1.2.0
**Last Updated**: 2025-12-20
**Status**: ✅ **ACTIVE**

---

## Overview

This document defines the **mandatory maturity requirements** for all Kubernaut services. All services MUST meet these requirements before being considered production-ready.

> **Living Document**: This document is updated as new ADRs/DDs are created. Check the changelog for updates.

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.2.0 | 2025-12-20 | **BREAKING**: Audit test validation now P0 (mandatory). All audit tests MUST use `testutil.ValidateAuditEvent` for structured validation (DD-AUDIT-003, V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md). This ensures audit trail quality and consistency across all services. |
| 1.1.0 | 2025-12-20 | **CRITICAL**: Added DD-METRICS-001 (Controller Metrics Wiring Pattern) - Dependency injection mandatory. Updated P0 requirements to reference DD-METRICS-001 for metrics wiring implementation. Added E2E testing tier for graceful shutdown (defense-in-depth). |
| 1.0.0 | 2025-12-19 | Initial version based on V1.0 maturity triage |

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md) | Testing requirements for maturity features |
| [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](./SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) | Service implementation checklist |
| [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md) | Test plan template (updated with E2E graceful shutdown) |
| [DD-005: Observability Standards](../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) | Metrics naming convention and standards |
| [DD-METRICS-001: Controller Metrics Wiring Pattern](../architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md) | **MANDATORY**: Dependency injection pattern for controller metrics |
| [DD-007: Graceful Shutdown](../architecture/decisions/DD-007-graceful-shutdown.md) | Shutdown pattern and audit flush requirements |

---

## CRD Controller Requirements

### P0 - Blockers (MUST have before release)

| Requirement | Reference | Test Tier |
|-------------|-----------|-----------|
| **Metrics wired to controller (dependency injection)** | DD-METRICS-001, DD-005 | Integration + E2E |
| **Metrics registered with controller-runtime** | DD-METRICS-001, DD-005 | E2E |
| **EventRecorder configured** | K8s best practices | E2E |
| **Graceful shutdown (flush audit)** | DD-007, ADR-032 | Unit + Integration + E2E |
| **Audit integration** | DD-AUDIT-003 | Integration |
| **Audit tests use testutil.ValidateAuditEvent** | DD-AUDIT-003, V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md | Integration + E2E |

### P1 - High Priority (SHOULD have before release)

| Requirement | Reference | Test Tier |
|-------------|-----------|-----------|
| **Predicates (event filtering)** | K8s best practices | Unit |
| **Healthz probes** | K8s best practices | E2E |

### P2 - Medium Priority (CAN defer)

| Requirement | Reference | Test Tier |
|-------------|-----------|-----------|
| **Logger field in struct** | DD-005 | N/A |
| **Config validation** | ADR-030 | Unit |

---

## Stateless HTTP Service Requirements

### P0 - Blockers (MUST have before release)

| Requirement | Reference | Test Tier |
|-------------|-----------|-----------|
| **Prometheus metrics** | DD-005 | Integration + E2E |
| **Health endpoints** | K8s best practices | E2E |
| **Graceful shutdown (audit flush, connection drain)** | DD-007, ADR-032 | Unit + Integration + E2E |
| **RFC 7807 errors** | DD-004 | Integration |
| **Audit integration (if applicable)** | DD-AUDIT-003 | Integration |
| **Audit tests use testutil.ValidateAuditEvent (if audit integration present)** | DD-AUDIT-003, V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md | Integration + E2E |

### P1 - High Priority (SHOULD have before release)

| Requirement | Reference | Test Tier |
|-------------|-----------|-----------|
| **Request logging** | DD-005 | N/A |
| **OpenAPI spec** | Best practices | Contract test |
| **Config validation** | ADR-030 | Unit |

---

## Metrics Naming Convention

**Reference**: [DD-005: Observability Standards](../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)

**Format**: `{service}_{component}_{metric_name}_{unit}`

### Examples

```go
// CRD Controllers
signalprocessing_reconciler_reconciliations_total{phase, result}
signalprocessing_enricher_duration_seconds{resource_kind}
workflowexecution_reconciler_pipelinerun_creations_total

// Stateless HTTP
gateway_http_requests_total{method, path, status}
gateway_http_request_duration_seconds{method, path}
datastorage_audit_events_written_total{service}
```

---

## Standard EventRecorder Reasons

All CRD controllers MUST use these standard event reasons:

| Reason | Type | When |
|--------|------|------|
| `ReconcileStarted` | Normal | Reconciliation begins |
| `ReconcileComplete` | Normal | Reconciliation succeeds |
| `ReconcileFailed` | Warning | Reconciliation fails |
| `PhaseTransition` | Normal | Phase changes |
| `ValidationFailed` | Warning | Spec validation fails |
| `DependencyMissing` | Warning | Required resource missing |

---

## Validation

### CI Enforcement Strategy

**Enforcement Point**: PRs to `main` branch only

| Branch | Enforcement | Rationale |
|--------|-------------|-----------|
| Feature branches | ❌ None | Allows iterative development |
| PRs to `main` | ✅ Blocking | Ensures production readiness |

**Benefits of this approach**:
- New services can be developed incrementally on feature branches
- Commits are never blocked during development
- Maturity is enforced before merge to `main`
- Developers can run validation locally to check progress

**CI Workflow**: `.github/workflows/service-maturity-validation.yml`

### Local Validation

Run the maturity validation script locally:

```bash
# Local validation (informational, no failure)
make validate-maturity

# CI mode (fails on P0 violations - same as PR check)
make validate-maturity-ci
```

### Manual Checklist

Use the checklist in [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](./SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#v10-mandatory-maturity-checklist).

---

## Current Status by Service

> **Note**: This section is auto-generated by `./scripts/validate-service-maturity.sh`
> See latest status in [docs/reports/maturity-status.md](../reports/maturity-status.md)

### CRD Controllers

| Service | Metrics | EventRecorder | Predicates | Shutdown | Audit |
|---------|---------|---------------|------------|----------|-------|
| SignalProcessing | 🔄 | 🔄 | 🔄 | ✅ | ✅ |
| WorkflowExecution | ✅ | ✅ | ✅ | ✅ | ✅ |
| AIAnalysis | 🔄 | ✅ | ✅ | ✅ | ✅ |
| Notification | ✅ | 🔄 | ✅ | ✅ | ✅ |
| RemediationOrchestrator | 🔄 | 🔄 | 🔄 | ✅ | ✅ |

### Stateless HTTP

| Service | Metrics | Health | Shutdown | RFC 7807 | Audit |
|---------|---------|--------|----------|----------|-------|
| Gateway | ✅ | ✅ | ✅ | ✅ | ✅ |
| DataStorage | ✅ | ✅ | ✅ | ✅ | ✅ |
| HolmesGPT-API | ✅ | ✅ | ✅ | ✅ | ✅ |

**Legend**: ✅ Complete | 🔄 In Progress | ❌ Missing

---

## Adding New Services

When creating a new service:

1. Use [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](./SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md)
2. Complete the V1.0 Maturity Checklist
3. Add tests per [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md)
4. Run `./scripts/validate-service-maturity.sh` before PR
5. Update status in this document

---

## Maintenance

When creating new ADRs/DDs that affect maturity:

1. Update this document with new requirements
2. Update [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md)
3. Update [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](./SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md)
4. Notify all teams via handoff document
5. Update `scripts/validate-service-maturity.sh` if needed

