# Appendix D: ADR/DD Reference Matrix

**Part of**: Signal Processing Implementation Plan V1.23
**Parent Document**: [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md)
**Last Updated**: 2025-12-04
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v3.0

---

## 📚 ADR/DD Reference Matrix

**Purpose**: Quick reference for which architecture decisions apply to Signal Processing

---

## Quick Reference: Which Documents Apply?

| Document Type | Signal Processing | Reason |
|---------------|-------------------|--------|
| Universal Standards | ✅ Required | All services |
| CRD Controller Standards | ✅ Required | SignalProcessing is a CRD controller |
| Kubernetes-Aware | ✅ Required | Reads K8s resources for enrichment |
| Audit Standards | ✅ Required | P1 audit per DD-AUDIT-003 |
| Testing Standards | ✅ Required | All services |
| HTTP API Standards | ❌ Not Applicable | No REST API (CRD-only) |

---

## Universal Standards (ALL Services)

| Document | Purpose | Signal Processing Application |
|----------|---------|-------------------------------|
| **DD-004** | [RFC 7807 Error Responses](../../../../../architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md) | Controller errors follow RFC 7807 structure in status |
| **DD-005** | [Observability Standards](../../../../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) | Logging with `logr.Logger`, metrics with Prometheus |
| **DD-007** | [K8s-Aware Graceful Shutdown](../../../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md) | Controller manager handles shutdown |
| **DD-014** | [Binary Version Logging](../../../../../architecture/decisions/DD-014-binary-version-logging-standard.md) | Log version on startup |
| **ADR-015** | [Signal Naming Migration](../../../../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md) | Uses "Signal" not "Alert" terminology |

---

## CRD Controller Standards

| Document | Purpose | Signal Processing Application |
|----------|---------|-------------------------------|
| **DD-006** | [Controller Scaffolding](../../../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md) | Uses kubebuilder scaffolding |
| **DD-013** | [K8s Client Initialization](../../../../../architecture/decisions/DD-013-kubernetes-client-initialization-standard.md) | Shared `pkg/k8sutil` client |
| **DD-CRD-001** | [API Group Domain](../../../../../architecture/decisions/DD-CRD-001-api-group-domain-selection.md) | `kubernaut.ai` |
| **ADR-004** | [Fake K8s Client](../../../../../architecture/decisions/ADR-004-fake-kubernetes-client.md) | **MANDATORY** for unit tests |

---

## Kubernetes-Aware Standards

| Document | Purpose | Signal Processing Application |
|----------|---------|-------------------------------|
| **DD-WORKFLOW-001** | [Mandatory Label Schema](../../../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | OwnerChain, DetectedLabels, CustomLabels (v2.2) |
| **ADR-041** | Rego Policy Data Fetching | Separation of data fetching from policy evaluation |

---

## Audit Standards

| Document | Purpose | Signal Processing Application |
|----------|---------|-------------------------------|
| **DD-AUDIT-003** | [Service Audit Requirements](../../../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) | P1 audit - classification decisions |
| **ADR-032** | [Data Access Isolation](../../../../../architecture/decisions/ADR-032-data-access-layer-isolation.md) | **MANDATORY** - use Data Storage API |
| **ADR-034** | [Unified Audit Table](../../../../../architecture/decisions/ADR-034-unified-audit-table-design.md) | Audit schema compliance |
| **ADR-038** | [Async Buffered Audit](../../../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) | Fire-and-forget audit writes |

---

## Testing Standards

| Document | Purpose | Signal Processing Application |
|----------|---------|-------------------------------|
| **DD-TEST-001** | [Port Allocation Strategy](../../../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) | E2E ports: HostPort 30101, NodePort 30201 |

---

## Service-Specific Documents

| Document | Purpose | Status |
|----------|---------|--------|
| **DD-CATEGORIZATION-001** | Gateway/SignalProcessing split | ✅ Referenced |
| **DD-CONTRACT-002** | Service integration contracts | ✅ Referenced |

---

## Document Validation Checklist

**Before Day 1**, validate all referenced documents exist:

```bash
#!/bin/bash
# Run from repository root

echo "🔍 Validating Signal Processing ADR/DD references..."

ERRORS=0

# Universal Standards
for doc in \
  "DD-004-RFC7807-ERROR-RESPONSES.md" \
  "DD-005-OBSERVABILITY-STANDARDS.md" \
  "DD-007-kubernetes-aware-graceful-shutdown.md" \
  "DD-014-binary-version-logging-standard.md" \
  "ADR-015-alert-to-signal-naming-migration.md"; do
  if [ -f "docs/architecture/decisions/$doc" ]; then
    echo "✅ $doc"
  else
    echo "❌ MISSING: $doc"
    ERRORS=$((ERRORS + 1))
  fi
done

# CRD Controller Standards
for doc in \
  "DD-006-controller-scaffolding-strategy.md" \
  "DD-013-kubernetes-client-initialization-standard.md" \
  "DD-CRD-001-api-group-domain-selection.md" \
  "ADR-004-fake-kubernetes-client.md"; do
  if [ -f "docs/architecture/decisions/$doc" ]; then
    echo "✅ $doc"
  else
    echo "❌ MISSING: $doc"
    ERRORS=$((ERRORS + 1))
  fi
done

# Kubernetes-Aware Standards
for doc in \
  "DD-WORKFLOW-001-mandatory-label-schema.md"; do
  if [ -f "docs/architecture/decisions/$doc" ]; then
    echo "✅ $doc"
  else
    echo "❌ MISSING: $doc"
    ERRORS=$((ERRORS + 1))
  fi
done

# Audit Standards
for doc in \
  "DD-AUDIT-003-service-audit-trace-requirements.md" \
  "ADR-032-data-access-layer-isolation.md" \
  "ADR-034-unified-audit-table-design.md" \
  "ADR-038-async-buffered-audit-ingestion.md"; do
  if [ -f "docs/architecture/decisions/$doc" ]; then
    echo "✅ $doc"
  else
    echo "❌ MISSING: $doc"
    ERRORS=$((ERRORS + 1))
  fi
done

# Testing Standards
for doc in "DD-TEST-001-port-allocation-strategy.md"; do
  if [ -f "docs/architecture/decisions/$doc" ]; then
    echo "✅ $doc"
  else
    echo "❌ MISSING: $doc"
    ERRORS=$((ERRORS + 1))
  fi
done

echo ""
if [ $ERRORS -gt 0 ]; then
  echo "❌ VALIDATION FAILED: $ERRORS documents missing"
  exit 1
else
  echo "✅ ALL DOCUMENTS VALIDATED"
fi
```

---

## How to Use This Matrix

### During Implementation

1. **Day 1**: Review all "Universal" and "CRD Controller" standards
2. **Day 3**: Reference DD-WORKFLOW-001 for label detection implementation
3. **Day 8**: Reference DD-AUDIT-003 and ADR-038 for audit implementation
4. **Day 10**: Reference DD-TEST-001 for E2E test port allocation

### During Code Review

- Verify code follows DD-005 logging patterns
- Verify error handling follows DD-004 structure
- Verify tests follow ADR-004 fake client pattern
- Verify audit follows ADR-032/ADR-038 patterns

### During Production Readiness

- Validate all ADRs/DDs referenced in implementation are followed
- Document any deviations with justification
- Update confidence assessment based on compliance

---

## Related Documents

- [Main Implementation Plan](../IMPLEMENTATION_PLAN.md)
- [Appendix A: Integration Test Environment](APPENDIX_A_INTEGRATION_TEST_ENVIRONMENT.md)
- [Appendix B: CRD Controller Patterns](APPENDIX_B_CRD_CONTROLLER_PATTERNS.md)
- [Appendix C: Confidence Methodology](APPENDIX_C_CONFIDENCE_METHODOLOGY.md)
- [Business Requirements](../../BUSINESS_REQUIREMENTS.md)

