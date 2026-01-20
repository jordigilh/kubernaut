# BR-HAPI-197 Test Plans - Human Review Required Flag

**Business Requirement**: [BR-HAPI-197](../../requirements/BR-HAPI-197-needs-human-review-field.md)
**Status**: Implementation In Progress
**Date**: January 20, 2026

---

## 📋 **Test Plan Organization**

This directory contains all test plans related to **BR-HAPI-197** (Human Review Required Flag) implementation.

### **Why Organize by BR?**
- ✅ Single BR affects multiple services (AIAnalysis, RO)
- ✅ Easy to find all related test plans
- ✅ Clear traceability from BR to tests
- ✅ Keeps `docs/testing/` clean and organized

---

## 📁 **Test Plans in This Directory**

| Service | Test Plan | Status | Coverage |
|---------|-----------|--------|----------|
| **AIAnalysis** | [aianalysis_test_plan_v1.0.md](aianalysis_test_plan_v1.0.md) | ✅ Complete | 18 unit + 8 integration + 3 E2E |
| **RemediationOrchestrator** | [remediationorchestrator_test_plan_v1.0.md](remediationorchestrator_test_plan_v1.0.md) | 🔄 In Progress | TBD |
| **Cross-Service Integration** | [integration_test_plan_v1.0.md](integration_test_plan_v1.0.md) | 📅 Planned | TBD |

---

## 🎯 **BR-HAPI-197 Implementation Scope**

### **What's Being Tested**
1. **HAPI Service**: ✅ Already tested (17 test files, 30+ tests)
   - See [BR-HAPI-197-TEST-COVERAGE-TRIAGE-JAN20-2026.md](../../handoff/BR-HAPI-197-TEST-COVERAGE-TRIAGE-JAN20-2026.md)

2. **AIAnalysis Service**: 🔄 Test plan complete, implementation pending
   - CRD schema extension (`NeedsHumanReview`, `HumanReviewReason` fields)
   - Response processor storage logic
   - Metric emission (`kubernaut_aianalysis_human_review_required_total`)

3. **RemediationOrchestrator Service**: 🔄 Test plan in progress
   - Two-flag routing logic (`needs_human_review` vs `needs_approval`)
   - NotificationRequest creation
   - Integration with AIAnalysis CRD

---

## 📊 **Overall Coverage Strategy (Defense-in-Depth)**

### **Unit Tests** (70%+ Coverage)
- **AIAnalysis**: 18 tests (CRD schema, response processor, metrics)
- **RO**: TBD tests (routing logic, notification creation)

### **Integration Tests** (50% Coverage)
- **AIAnalysis**: 8 tests (HAPI → CRD flow)
- **RO**: TBD tests (AIAnalysis → NotificationRequest flow)
- **Cross-Service**: TBD tests (Full cascade)

### **E2E Tests** (<10% Coverage)
- **AIAnalysis**: 3 tests (Complete business workflows)
- **RO**: TBD tests (End-to-end remediation flows)

---

## 🔗 **Related Documentation**

### **Business Requirements**
- [BR-HAPI-197](../../requirements/BR-HAPI-197-needs-human-review-field.md) - Human Review Required Flag
- [BR-HAPI-212](../../requirements/BR-HAPI-212-rca-target-resource.md) - RCA Target Resource (extends BR-HAPI-197)

### **Design Decisions**
- [DD-CONTRACT-002](../../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md) - Service Integration Contracts
- [DD-HAPI-006](../../architecture/decisions/DD-HAPI-006-affectedResource-in-rca.md) - affectedResource in RCA

### **Implementation Plans**
- [BR-HAPI-197-COMPLETION-PLAN-JAN20-2026.md](../../handoff/BR-HAPI-197-COMPLETION-PLAN-JAN20-2026.md) - Complete implementation plan
- [BR-HAPI-197-TEST-COVERAGE-TRIAGE-JAN20-2026.md](../../handoff/BR-HAPI-197-TEST-COVERAGE-TRIAGE-JAN20-2026.md) - HAPI test coverage analysis

---

## ⚡ **Quick Reference**

### **Run All BR-HAPI-197 Tests**
```bash
# Unit tests
make test FOCUS="BR-HAPI-197"

# Integration tests
make test-integration-kind FOCUS="BR-HAPI-197"

# E2E tests
make test-e2e-kind FOCUS="BR-HAPI-197"

# All tests
make test-all FOCUS="BR-HAPI-197"
```

### **Test Plan Status**
- ✅ HAPI: Complete (no new tests needed)
- 🔄 AIAnalysis: Test plan complete, awaiting implementation
- 🔄 RO: Test plan in progress
- 📅 Integration: Planned after service tests complete

---

**Last Updated**: January 20, 2026
**Confidence**: 90%
