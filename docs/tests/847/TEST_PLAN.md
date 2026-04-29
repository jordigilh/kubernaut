# Test Plan: Adversarial Due Diligence & Hierarchy-Aware Target Resolution

> **Template Version**: 2.0 — Hybrid IEEE 829 + Kubernaut

**Test Plan Identifier**: TP-847-v1.0
**Feature**: Adversarial due diligence framework for RCA validation and hierarchy-aware remediation target resolution
**Version**: 1.0
**Created**: 2026-04-29
**Author**: AI Assistant
**Status**: Active
**Branch**: `main`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the adversarial due diligence and hierarchy-aware target resolution features introduced by Issue #847. The feature enhances the Kubernaut Agent's investigation pipeline with:

1. **Adversarial due diligence**: Phase 1 RCA must include `causal_chain` (min 2 steps, five-whys style) and `due_diligence` (8 mandatory dimensions) to ensure thorough root cause analysis before remediation.
2. **Same-kind validation gate**: When the LLM's `remediation_target.kind` matches the signal's `resource_kind`, the investigator sends a corrective prompt and retries once to ensure the target is the actual root cause, not the symptom.
3. **Hierarchy-aware target resolution**: `InjectRemediationTarget` uses Kubernetes owner-chain enrichment to resolve the authoritative remediation target, pulling up to the hierarchy root when appropriate while preserving genuinely cross-type targets.

### 1.2 Objectives

1. Validate `causal_chain` and `due_diligence` are captured in Phase 1 and propagated to Phase 3 results
2. Validate same-kind sentinel gate behavior (retry, keep-original, explicitly-confirmed)
3. Validate hierarchy-aware target injection respects owner-chain vs cross-type targets
4. Validate forensic fields appear in audit events and API responses

### 1.3 Success Metrics

| Metric | Target |
|--------|--------|
| Unit test pass rate | 100% |
| Integration test pass rate | 100% |
| BR coverage | All ACs for BR-HAPI-847, BR-HAPI-261 |

---

## 2. References

### 2.1 Authoritative Documents

- [Issue #847](https://github.com/jordigilh/kubernaut/issues/847) — Adversarial due diligence
- [Issue #851](https://github.com/jordigilh/kubernaut/issues/851) — `aiagent.rca.complete` audit event
- BR-HAPI-847 — Adversarial due diligence framework
- BR-HAPI-261 — Hierarchy-aware target resolution (AC#4, AC#5, AC#7)
- DD-HAPI-847 — Design decision for due diligence dimensions

### 2.2 Implementation Files

| File | Role |
|------|------|
| `internal/kubernautagent/prompt/templates/incident_investigation.tmpl` | Phase 1 prompt with due diligence framework |
| `internal/kubernautagent/parser/schema.go` | `rcaResultSchemaJSON` with `causal_chain` and `due_diligence` |
| `internal/kubernautagent/parser/parser.go` | `llmRCA` parsing of forensic fields |
| `internal/kubernautagent/investigator/investigator.go` | `sameKindValidationGate`, `InjectRemediationTarget`, `isKindInOwnerChain`, `BuildPhase1Context`, `MergePhase1Fallbacks`, `ResultToAuditJSON` |
| `internal/kubernautagent/types/types.go` | `DueDiligenceReview` struct (8 fields), `InvestigationResult.CausalChain`, `.DueDiligence` |
| `internal/kubernautagent/prompt/builder.go` | `Phase1Data` with `CausalChain` and `DueDiligence` |
| `internal/kubernautagent/audit/ds_store.go` | `ResultToAuditJSON` serialization of forensic fields |

---

## 3. Risks

| Risk | Likelihood | Impact | Mitigation | Test IDs |
|------|-----------|--------|------------|----------|
| LLM omits `due_diligence` fields | Medium | Medium | Schema validation + fail-closed on missing mandatory fields | UT-KA-847-010..012 |
| Same-kind gate infinite loop | Low | High | Single retry limit; second same-kind is accepted | IT-KA-847-D-001 |
| Hierarchy pull-up on cross-type target | Medium | High | `isKindInOwnerChain` preserves genuinely cross-type targets | UT-KA-847-020..025 |

---

## 4. Scope

### 4.1 In Scope

- Phase 1 forensic field parsing and propagation
- Same-kind validation gate (retry, keep-original, explicitly-confirmed paths)
- Hierarchy-aware target injection via owner-chain
- Audit event serialization of `causal_chain` and `due_diligence`
- API response mapping of forensic fields

### 4.2 Out of Scope

- LLM prompt quality (tested via adversarial parity E2E, TP-433)
- Enrichment data gathering (separate feature)

---

## 5. Test Scenarios

### 5.1 Unit Tests

| ID | Description | BR |
|----|-------------|-----|
| UT-KA-847-010 | `BuildPhase1Context` captures `CausalChain` and `DueDiligence` | BR-HAPI-847 |
| UT-KA-847-011 | `MergePhase1Fallbacks` propagates forensic fields from Phase 1 to Phase 3 result | BR-HAPI-847 |
| UT-KA-847-012 | `ResultToAuditJSON` serializes `causal_chain` and `due_diligence` correctly | BR-HAPI-847 |
| UT-KA-847-020 | `InjectRemediationTarget` pulls up to hierarchy root for same-chain kind | BR-HAPI-261 |
| UT-KA-847-021 | `InjectRemediationTarget` preserves cross-type target (LLM picks Node for Pod signal) | BR-HAPI-261 |
| UT-KA-847-022 | `isKindInOwnerChain` returns true for Deployment→ReplicaSet→Pod chain | BR-HAPI-261 |
| UT-KA-847-023 | `isKindInOwnerChain` returns false for genuinely different kind (ConfigMap target for Pod signal) | BR-HAPI-261 |

### 5.2 Integration Tests

| ID | Description | BR |
|----|-------------|-----|
| IT-KA-847-D-001 | Same-kind gate keeps original target when retry drops `remediation_target` | BR-HAPI-847, DD-HAPI-847 |
| IT-KA-851-AP-001 | `aiagent.rca.complete` audit event contains `causal_chain` and `due_diligence` in `response_data` | BR-HAPI-847, BR-HAPI-851 |

### 5.3 E2E Tests

| ID | Description | BR |
|----|-------------|-----|
| E2E-KA-433-ADV | Adversarial parity E2E validates due diligence fields in full pipeline | BR-HAPI-433 |

---

## 6. Existing Test Coverage

| File | Test IDs | Tier |
|------|----------|------|
| `test/unit/kubernautagent/investigator/phase1_propagation_test.go` | UT-KA-847-010, -011, -012 | Unit |
| `test/integration/kubernautagent/investigator/investigator_test.go` | IT-KA-847-D-001 | Integration |
| `test/integration/kubernautagent/investigator/investigator_audit_parity_test.go` | IT-KA-851-AP-001 | Integration |
| `test/unit/kubernautagent/audit/ds_store_audit_parity_test.go` | UT-KA-851-AP-001..004 | Unit |
| `test/e2e/kubernautagent/adversarial_parity_e2e_test.go` | E2E-KA-433-ADV family | E2E |

---

## 7. Execution

```bash
# Unit tests
make test-unit-kubernautagent

# Integration tests
make test-integration-kubernautagent

# E2E (adversarial parity suite)
make test-e2e-kubernautagent
```

---

## 8. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-29 | Initial test plan — documents existing coverage for QE readiness |
