# Kubernaut Authoritative Standards Index

**Authority**: 🏛️ **SYSTEM GOVERNANCE**
**Purpose**: Index of all authoritative documents that govern development across Kubernaut
**Maintenance**: Architecture Team

---

## 🏛️ **What Makes a Document Authoritative?**

An **authoritative document** is the single source of truth for a particular aspect of the system. It:
- **Supersedes** all conflicting documentation or implementation
- **Requires** Architecture Team approval to modify
- **Mandates** compliance from all service teams
- **Governs** cross-service patterns and standards

---

## 📋 **Current Authoritative Standards**

### **1. BR-COMMON-001: Phase Value Format Standard** 🏛️

**Location**: `docs/requirements/BR-COMMON-001-phase-value-format-standard.md`
**Authority**: Governing Standard for All Services
**Created**: 2025-12-11
**Scope**: All CRD phase/status fields system-wide

**Governs**:
- Phase value capitalization (MUST be capitalized per Kubernetes conventions)
- Enum validation format
- Terminal phase naming (`Completed`/`Failed`)
- Multi-word phase formatting (PascalCase)

**Compliance**:
- ✅ SignalProcessing (fixed 2025-12-11)
- ✅ WorkflowExecution
- ✅ AIAnalysis
- ✅ RemediationRequest
- ✅ Notification
- ✅ RemediationApprovalRequest

**Enforcement**:
- Pre-merge: Automated CRD validation in CI
- Code review: Architecture team verification
- Integration: Cross-service tests validate compliance

---

### **2. RO Viceversa Pattern: Cross-Service Phase Consumption** 🏛️

**Location**: (internal development reference, removed in v1.0)
**Authority**: Mandatory Pattern for Cross-Service Integration
**Created**: 2025-12-11
**Scope**: All services consuming phase values from other CRDs

**Governs**:
- Use of typed constants when available (`string(servicev1.PhaseCompleted)`)
- Fallback to documented string literals when typed constants don't exist
- Cross-service dependency management
- Compile-time safety requirements

**Example Compliance**:
```go
// ✅ CORRECT: Uses source service's typed constants
import signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"

switch sp.Status.Phase {
case string(signalprocessingv1.PhaseCompleted):  // Authoritative pattern
    // Handle completion
}

// ❌ WRONG: Hardcoded string
switch sp.Status.Phase {
case "Completed":  // Violates viceversa pattern
    // Breaks single source of truth
}
```

**Enforcement**:
- Pre-merge: Code review checklist item
- Static analysis: Linter checks for hardcoded phase strings
- Architecture review: Pattern compliance verification

---

## 🔗 **Relationship Between Authoritative Standards**

### **BR-COMMON-001 + Viceversa Pattern = Complete Phase Governance**

| Aspect | Governed By |
|--------|-------------|
| **Phase Value Format** | BR-COMMON-001 |
| **Phase Definition** | BR-COMMON-001 |
| **Phase Consumption** | Viceversa Pattern |
| **Cross-Service Integration** | Viceversa Pattern |

**Together they ensure**:
1. ✅ Consistent phase values across all services (BR-COMMON-001)
2. ✅ Type-safe phase consumption (Viceversa Pattern)
3. ✅ Automatic propagation of changes (Viceversa Pattern)
4. ✅ Compile-time error detection (Both)

---

## 📊 **Adding New Authoritative Standards**

### **Criteria for Authoritative Status**

A document becomes authoritative when it:
1. **Governs system-wide behavior** (not service-specific)
2. **Requires universal compliance** (no opt-out)
3. **Impacts multiple teams** (cross-service)
4. **Prevents critical failures** (safety/reliability)

### **Approval Process**

1. **Proposal**: Document created with `[PROPOSAL]` status
2. **Review**: Architecture team + affected teams review
3. **Implementation**: Pilot implementation in 1-2 services
4. **Validation**: Verify benefit and feasibility
5. **Approval**: Architecture team marks as 🏛️ **AUTHORITATIVE**
6. **Index**: Add to this document

### **Modification Process**

**Authoritative documents require**:
1. Architecture team approval for any changes
2. Impact assessment across all services
3. Migration plan for breaking changes
4. Communication to all teams

---

## 📚 **Historical Context**

### **Why Authoritative Standards Were Created**

**Date**: 2025-12-11
**Trigger**: SignalProcessing phase capitalization bug

**Problem**:
- SP used lowercase phases (`"completed"`)
- RO expected capitalized (`"Completed"`)
- Integration tests failed, blocking RO team
- No single source of truth for phase format

**Solution**:
- Created BR-COMMON-001 as authoritative format standard
- Created Viceversa Pattern as authoritative consumption pattern
- Fixed SP implementation same day
- All services validated for compliance

**Lesson**: Without authoritative standards, each team makes independent decisions that break integration.

---

## 🎯 **Compliance Metrics**

### **BR-COMMON-001 Compliance**

| Service | Status | Date Verified |
|---------|--------|---------------|
| SignalProcessing | ✅ Compliant | 2025-12-11 |
| AIAnalysis | ✅ Compliant | 2025-12-11 |
| WorkflowExecution | ✅ Compliant | 2025-12-11 |
| RemediationRequest | ✅ Compliant | 2025-12-11 |
| Notification | ✅ Compliant | 2025-12-11 |
| RemediationApprovalRequest | ✅ Compliant | 2025-12-11 |
| **System-Wide** | **100%** ✅ | 2025-12-11 |

### **Viceversa Pattern Adoption**

| Consumer Service | Source Service | Status | Date Implemented |
|-----------------|----------------|--------|------------------|
| RemediationOrchestrator | SignalProcessing | ✅ Uses typed constants | 2025-12-11 |
| RemediationOrchestrator | AIAnalysis | ✅ Documented literals | 2025-12-11 |
| RemediationOrchestrator | WorkflowExecution | ✅ Documented literals | 2025-12-11 |

---

## 🚀 **Future Authoritative Standards**

**Candidates under consideration**:
- Error handling patterns (BR-COMMON-002)
- Audit event format standard (BR-COMMON-003)
- Retry/backoff strategy (BR-COMMON-004)
- Timeout standardization (BR-COMMON-005)

---

## 📞 **Questions & Governance**

**For authoritative standard questions**:
- **Format/Compliance**: Reference the authoritative document directly
- **Exceptions**: Request Architecture Team review
- **New Standards**: Propose to Architecture Team
- **Modifications**: Submit RFC with impact assessment

**Architecture Team**: architecture@kubernaut.dev (or relevant contact)

---

**Document Status**: 🏛️ **AUTHORITATIVE INDEX**
**Maintained By**: Architecture Team
**Last Updated**: 2025-12-11
**Next Review**: Quarterly

---

## ✅ **Summary**

Kubernaut currently has **2 authoritative standards** governing phase-related development:

1. 🏛️ **BR-COMMON-001**: Phase Value Format Standard
2. 🏛️ **Viceversa Pattern**: Cross-Service Phase Consumption

Both are **mandatory**, **system-wide**, and **actively enforced** through CI, code review, and architecture governance.
