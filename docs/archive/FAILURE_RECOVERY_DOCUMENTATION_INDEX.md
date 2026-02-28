> **DEPRECATED (Issue #180)**: The recovery flow (DD-RECOVERY-002/003) has been deprecated.
> The existing DS remediation-history flow (ADR-055) provides historical context on signal re-arrival.
> This document is preserved for historical reference only.

---


# Failure Recovery Documentation Index

**Document Version**: 1.0
**Date**: October 8, 2025
**Purpose**: Navigation guide for failure recovery architecture documentation
**Status**: ✅ **ACTIVE**
**Confidence**: 100% (Documentation index - comprehensive and validated)

---

## 📚 **Document Hierarchy**

This index provides a clear navigation path through the failure recovery architecture documentation suite.

---

## 🎯 **Start Here: Approved Implementation Reference**

### **1. [PROPOSED_FAILURE_RECOVERY_SEQUENCE.md](./PROPOSED_FAILURE_RECOVERY_SEQUENCE.md)** ⭐

**Status**: ✅ **APPROVED & ACTIVE - PRIMARY REFERENCE**
**Design Decision**: [DD-001 - Alternative 2](./DESIGN_DECISIONS.md#dd-001-recovery-context-enrichment-alternative-2)
**Confidence**: 95%

**Purpose**: Official sequence diagram showing the complete failure recovery flow

**Use This For**:
- Understanding controller interactions during recovery
- Implementing recovery coordination logic
- Debugging recovery flow issues
- Training new team members on recovery architecture

**Key Content**:
- Complete mermaid sequence diagram with 5 phases
- Recovery loop prevention logic (max 3 attempts)
- Context API integration patterns
- Recovery phase transitions
- CRD state progression examples

**Audience**: Developers, Architects, Operations

---

## 📖 **Supporting Documentation**

### **2. [STEP_FAILURE_RECOVERY_ARCHITECTURE.md](./STEP_FAILURE_RECOVERY_ARCHITECTURE.md)**

**Status**: ✅ **APPROVED & ALIGNED**

**Purpose**: Comprehensive design principles and architecture patterns

**Use This For**:
- Understanding architectural decisions and rationale
- Learning about multi-layered failure detection
- AI-enhanced failure analysis patterns
- Recovery strategies and fallback mechanisms
- Health-based decision making

**Key Content**:
- Failure detection hierarchy
- Failure classification matrix
- HolmesGPT investigation flow
- Multi-level fallback strategies
- Workflow health assessment
- Learning framework integration
- Controller responsibilities table

**Audience**: Architects, Senior Developers, System Designers

---

### **3. [FAILURE_RECOVERY_FLOW_CONFIDENCE_ASSESSMENT.md](./FAILURE_RECOVERY_FLOW_CONFIDENCE_ASSESSMENT.md)**

**Status**: ✅ **APPROVED & IMPLEMENTED**

**Purpose**: Architecture review and validation analysis

**Use This For**:
- Understanding why the current flow was chosen
- Learning about considered alternatives
- Reviewing critical issues and mitigations
- Architecture decision records (ADR-like)

**Key Content**:
- Confidence assessment (92% with mitigations)
- Strengths analysis (separation of concerns, Context API integration)
- Critical issues identified and resolved
- Recommended mitigations implemented
- CRD schema recommendations

**Audience**: Architects, Technical Leadership, Reviewers

---

## 📚 **Historical Reference**

### **4. [SCENARIO_A_RECOVERABLE_FAILURE_SEQUENCE.md](./SCENARIO_A_RECOVERABLE_FAILURE_SEQUENCE.md)**

**Status**: ⚠️ **SUPERSEDED - HISTORICAL REFERENCE ONLY**

**Purpose**: Original scenario diagram (before approval process)

**Use This For**:
- Historical context on evolution of recovery flow
- Understanding initial design considerations
- Comparing with approved implementation

**Key Differences from Approved Flow**:
- Included full alert ingestion flow (now focuses on recovery only)
- Used separate WO1/WO2 participants (now single WorkflowExecution Controller)
- Different level of detail and abstraction
- No recovery loop prevention

**⚠️ Do Not Use For Implementation** - Refer to `PROPOSED_FAILURE_RECOVERY_SEQUENCE.md` instead

**Audience**: Historical researchers, Architecture evolution tracking

---

## 🔄 **Document Relationships**

```ascii
┌─────────────────────────────────────────────────────────────┐
│                    DOCUMENTATION HIERARCHY                  │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  📊 PROPOSED_FAILURE_RECOVERY_SEQUENCE.md ⭐                │
│     ├─ Status: APPROVED & ACTIVE                           │
│     ├─ Audience: ALL (primary reference)                   │
│     └─ Content: Official sequence diagram                  │
│           │                                                 │
│           ├───► Supported By:                              │
│           │                                                 │
│           ├─ 📖 STEP_FAILURE_RECOVERY_ARCHITECTURE.md      │
│           │    ├─ Status: APPROVED & ALIGNED               │
│           │    ├─ Audience: Architects, Senior Developers  │
│           │    └─ Content: Design principles, patterns     │
│           │                                                 │
│           └─ 📝 FAILURE_RECOVERY_FLOW_CONFIDENCE_ASSESS... │
│                ├─ Status: APPROVED & IMPLEMENTED           │
│                ├─ Audience: Technical Leadership           │
│                └─ Content: Analysis, decision rationale    │
│                                                             │
│  📚 SCENARIO_A_RECOVERABLE_FAILURE_SEQUENCE.md             │
│     ├─ Status: SUPERSEDED (historical)                     │
│     ├─ Audience: Historical reference                      │
│     └─ Content: Original design (DO NOT IMPLEMENT)         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 🎯 **Quick Navigation by Use Case**

| **What You Need** | **Document to Read** | **Priority** |
|-------------------|----------------------|--------------|
| Implement recovery flow | `PROPOSED_FAILURE_RECOVERY_SEQUENCE.md` | ⭐ P0 |
| Understand architecture | `STEP_FAILURE_RECOVERY_ARCHITECTURE.md` | 📖 P1 |
| Review decision rationale | `FAILURE_RECOVERY_FLOW_CONFIDENCE_ASSESSMENT.md` | 📝 P2 |
| Understand design evolution | `SCENARIO_A_RECOVERABLE_FAILURE_SEQUENCE.md` | 📚 P3 |

---

## 🔑 **Key Concepts Across Documents**

### **Recovery Loop Prevention**
- **Max Recovery Attempts**: 3 (4 total workflow executions)
- **Pattern Detection**: Same failure twice → escalate
- **Termination Rate**: BR-WF-541 (<10% requirement)

### **Controller Architecture**
- **WorkflowExecution Controller**: Single instance managing multiple CRDs
- **AIAnalysis Controller**: Single instance managing multiple CRDs
- **Remediation Orchestrator**: Central recovery coordinator
- **K8s Executor**: Action execution with validation

### **Context API Integration**
- Historical failure data
- Previous workflow executions
- Pattern recognition
- Graceful degradation if unavailable

### **Recovery Phases**
```
Initial → Analyzing → Executing → [Failure] → Recovering → Completed ✅
                                                         ↓
                                              Failed (escalate) ❌
```

---

## 📋 **Document Update Log**

| Date | Document | Change | Version |
|------|----------|--------|---------|
| 2025-10-08 | `PROPOSED_FAILURE_RECOVERY_SEQUENCE.md` | Approved as authoritative reference | 1.0 |
| 2025-10-08 | `STEP_FAILURE_RECOVERY_ARCHITECTURE.md` | Aligned with approved sequence | 1.1 |
| 2025-10-08 | `FAILURE_RECOVERY_FLOW_CONFIDENCE_ASSESSMENT.md` | Marked as approved & implemented | 1.0 |
| 2025-10-08 | `SCENARIO_A_RECOVERABLE_FAILURE_SEQUENCE.md` | Marked as superseded | 1.0 |
| 2025-10-08 | `FAILURE_RECOVERY_DOCUMENTATION_INDEX.md` | Created navigation index | 1.0 |

---

## 🔗 **External References**

### **Business Requirements**
- BR-WF-541: <10% workflow termination rate
- BR-ORCH-004: Learning from execution outcomes
- BR-WF-HOLMESGPT-001: HolmesGPT investigation integration
- BR-REL-006: Error resilience with intelligent retry
- BR-REL-010: Graceful degradation

### **Related Documentation**
- `docs/services/crd-controllers/03-workflowexecution/` - WorkflowExecution controller
- `docs/services/crd-controllers/02-aianalysis/` - AIAnalysis controller
- `docs/services/crd-controllers/05-remediationorchestrator/` - Remediation Orchestrator
- `docs/services/crd-controllers/04-kubernetesexecutor/` - Kubernetes Executor
- `docs/architecture/RESILIENCE_PATTERNS.md` - Resilience patterns
- `docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md` - Workflow requirements

---

## 💡 **Getting Started Checklist**

For new developers working on failure recovery:

- [ ] Read `PROPOSED_FAILURE_RECOVERY_SEQUENCE.md` (sequence diagram)
- [ ] Review `STEP_FAILURE_RECOVERY_ARCHITECTURE.md` (design principles)
- [ ] Understand recovery loop prevention mechanisms
- [ ] Study Context API integration patterns
- [ ] Review CRD schemas for RemediationRequest, AIAnalysis, WorkflowExecution
- [ ] Understand controller watch patterns
- [ ] Review business requirements (BR-WF-541, BR-ORCH-004)

---

## 📞 **Questions?**

For questions about failure recovery architecture:
1. Check this index for the relevant document
2. Review the primary reference (`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`)
3. Consult the architecture document for design rationale
4. Review the confidence assessment for decision context

---

**Last Updated**: October 8, 2025
**Maintained By**: Architecture Team
**Review Cycle**: Quarterly or on significant changes

