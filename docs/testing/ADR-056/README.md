# ADR-056 Test Plans - Relocate DetectedLabels Computation to HAPI Post-RCA

**Architecture Decision**: [ADR-056](../../architecture/decisions/ADR-056-post-rca-label-computation.md)
**Issue**: #102
**Status**: Implementation Complete
**Date**: February 17, 2026

---

## Test Plan Organization

This directory contains all test plans related to **ADR-056** (Relocate DetectedLabels computation to HAPI post-RCA).

### Why Organize by ADR?

- ADR-056 affects multiple services (HAPI, AIAnalysis)
- Easy to find all related test plans from the architectural decision
- Clear traceability from ADR to tests
- Keeps `docs/testing/` clean and organized

---

## Test Plans in This Directory

| Service | Test Plan | Status | Test Count |
|---------|-----------|--------|------------|
| **HAPI (Python)** | [hapi_test_plan_v1.0.md](hapi_test_plan_v1.0.md) | Active | 80 unit, 7 integration, 3 E2E |
| **AIAnalysis (Go)** | [aianalysis_test_plan_v1.0.md](aianalysis_test_plan_v1.0.md) | Active | 18 unit, 6 integration, 3 E2E |

---

## ADR-056 Implementation Scope

### What's Being Tested

1. **HAPI Service** (Python): Label detection, K8s client queries, session_state wiring, flow enforcement, context params, prompt removal, response model
2. **AIAnalysis Service** (Go): PostRCAContext CRD type, ResponseProcessor extraction, AnalyzingHandler Rego integration, EnrichmentResults cleanup

### Affected Business Requirements

- **BR-SP-101**: DetectedLabels auto-detection (8 characteristics) -- scope changes from pipeline-wide to SP-internal
- **BR-SP-103**: FailedDetections tracking -- stays within SP
- **BR-HAPI-194**: Honor `failedDetections` in workflow filtering -- moves to HAPI-computed labels
- **BR-HAPI-250/252**: DetectedLabels integration with Data Storage -- labels now computed by HAPI

### Related Design Documents

- [DD-HAPI-017](../../architecture/decisions/DD-HAPI-017-three-step-workflow-discovery-integration.md) - Three-step workflow discovery integration
- [DD-HAPI-018](../../architecture/decisions/DD-HAPI-018-detected-labels-detection-specification.md) - DetectedLabels detection specification

---

## Defense-in-Depth Coverage

Following `TESTING_GUIDELINES.md` defense-in-depth strategy:

| Tier | HAPI (Python) | AIAnalysis (Go) | Focus |
|------|---------------|-----------------|-------|
| **Unit** | 80 tests | 18 tests | Business logic, field extraction, error handling, API contracts |
| **Integration** | 7 tests | 6 tests | K8s mock fixtures, controller reconciliation, multi-component flow |
| **E2E** | 3 tests | 3 tests | Kind cluster, real K8s resources, full pipeline validation |

---

## Test Naming Convention

**Format**: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

| Component | HAPI Tests | AIAnalysis Tests |
|-----------|------------|------------------|
| **TIER** | `UT` (Unit Test) | `UT` (Unit Test) |
| **SERVICE** | `HAPI` | `AA` |
| **BR_NUMBER** | `056` (from ADR-056) | `056` (from ADR-056) |
| **SEQUENCE** | `001` - `080` | `001` - `019` (018 merged into 011) |

**Examples**:
- `UT-HAPI-056-001`: HAPI unit test #1 (ArgoCD pod annotation -> gitOpsManaged)
- `UT-AA-056-003`: AIAnalysis unit test #3 (ProcessIncidentResponse populates PostRCAContext)

---

## Related Documentation

### Architecture Decisions
- [ADR-056](../../architecture/decisions/ADR-056-post-rca-label-computation.md) - Relocate DetectedLabels computation to HAPI post-RCA

### Testing
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) - Testing standards and anti-patterns
- [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md) - Template used for these plans

---

**Last Updated**: February 19, 2026
