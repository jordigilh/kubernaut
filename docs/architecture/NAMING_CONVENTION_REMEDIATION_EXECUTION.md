# Naming Convention Summary

**Date**: 2025-11-15
**Status**: Approved

---

## Authoritative Naming Convention

Based on **ADR-035** and **KUBERNAUT_CRD_ARCHITECTURE.md**, the following naming convention is established:

### **Option C: Contextual Usage** (APPROVED)

Use both names depending on context:

#### 1. **"RemediationExecution"** (Technical Context)
**When to use**:
- CRD specifications and schemas
- Kubernetes resource names
- API group references (`workflowexecution.kubernaut.io/v1alpha1`)
- Controller code and implementation
- RBAC policies and ServiceAccounts

**Examples**:
- `RemediationExecution` CRD
- `RemediationExecutionReconciler` controller
- `workflowexecution.kubernaut.io` API group

#### 2. **"Remediation Execution Engine"** (Architectural Context)
**When to use**:
- Architecture diagrams and overviews
- High-level documentation
- Design decisions and ADRs
- Service descriptions
- User-facing documentation

**Examples**:
- "Remediation Execution Engine orchestrates multi-step workflows"
- "The Remediation Execution Engine creates Tekton PipelineRuns"
- Architecture diagram labels

---

## Corrected Documents

The following documents have been updated to use the correct naming:

1. ✅ `KUBERNAUT_ARCHITECTURE_OVERVIEW.md`
2. ✅ `KUBERNAUT_SERVICE_CATALOG.md`
3. ✅ `DD-PLAYBOOK-003-parameterized-actions.md`
4. ✅ `DD-PLAYBOOK-011-tekton-oci-bundles.md`
5. ✅ `MICROSERVICES_COMMUNICATION_ARCHITECTURE.md`
6. ✅ `SCENARIO_A_RECOVERABLE_FAILURE_SEQUENCE.md`
7. ✅ `STEP_FAILURE_RECOVERY_ARCHITECTURE.md`
8. ✅ `KUBERNAUT_IMPLEMENTATION_ROADMAP.md`
9. ✅ `README.md`
10. ✅ `ADR-035-remediation-execution-engine.md`
11. ✅ `ADR-033-remediation-playbook-catalog.md`
12. ✅ `STORAGE_DATA_MANAGEMENT_ARCHITECTURE.md`
13. ✅ `RESILIENT_WORKFLOW_AI_SEQUENCE_DIAGRAM.md`
14. ✅ `WORKFLOW_ENGINE_ORCHESTRATION_ARCHITECTURE.md` (content updated, filename unchanged)

---

## Deprecated Terms

❌ **DO NOT USE**:
- "Workflow Engine" (too generic, conflicts with industry terminology)
- "Workflow Service" (incorrect service name)
- "WorkflowExecution" without "Remediation" prefix (ambiguous)

---

## Authority

- **Primary**: ADR-035 (Remediation Execution Engine - Tekton Pipelines)
- **Secondary**: KUBERNAUT_CRD_ARCHITECTURE.md (CRD specifications)

---

**Note**: The filename `WORKFLOW_ENGINE_ORCHESTRATION_ARCHITECTURE.md` remains unchanged to preserve git history and external references. The document content has been updated to use the correct terminology.

