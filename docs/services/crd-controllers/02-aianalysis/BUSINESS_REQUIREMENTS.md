# AIAnalysis Service - Business Requirements

**Service**: AIAnalysis Controller
**Service Type**: CRD Controller
**CRD**: AIAnalysis
**Controller**: AIAnalysisReconciler
**Version**: 1.0
**Last Updated**: November 28, 2025
**Status**: In Development

---

## 📋 Overview

The **AIAnalysis Service** is a Kubernetes CRD controller that orchestrates HolmesGPT-powered alert investigation, root cause analysis, and remediation workflow selection. It receives enriched signal data from SignalProcessing and produces workflow recommendations for execution.

### Service Responsibilities

1. **HolmesGPT Integration**: Trigger AI-powered investigation via HolmesGPT-API
2. **Root Cause Analysis**: Identify root cause candidates with supporting evidence
3. **Workflow Selection**: Select appropriate remediation workflow from catalog
4. **Confidence Assessment**: Evaluate recommendation confidence for approval routing
5. **Approval Signaling**: Signal approval requirement for low-confidence recommendations

---

## 🎯 Business Requirements

### Category 1: Workflow Selection Contract

#### BR-AI-075: Workflow Selection Output Format

**Description**: AIAnalysis MUST store selected workflow in status using `workflow_id` + `parameters` format per ADR-041 LLM Response Contract, enabling downstream services to resolve the workflow from the catalog.

**Priority**: P0 (CRITICAL)

**Rationale**: Consistent contract between AIAnalysis and downstream services (RemediationOrchestrator, WorkflowExecution) is essential for reliable remediation execution. The `workflow_id` maps to the workflow catalog for container image resolution.

**Implementation**:
- `status.selectedWorkflow.workflowId`: Catalog lookup key
- `status.selectedWorkflow.version`: Workflow version
- `status.selectedWorkflow.confidence`: MCP search confidence (0.0-1.0)
- `status.selectedWorkflow.parameters`: UPPER_SNAKE_CASE parameters per DD-WORKFLOW-003
- `status.selectedWorkflow.rationale`: LLM reasoning for selection

**Acceptance Criteria**:
- ✅ `workflow_id` matches catalog entry
- ✅ Parameters use UPPER_SNAKE_CASE naming
- ✅ Confidence score between 0.0 and 1.0
- ✅ Rationale provides actionable explanation

**Test Coverage**:
- Unit: Status field population and validation
- Integration: HolmesGPT response parsing to status
- E2E: End-to-end workflow selection validation

**Related DDs**: DD-CONTRACT-001 (AIAnalysis ↔ WorkflowExecution Alignment)
**Related ADRs**: ADR-041 (LLM Response Contract)

---

#### BR-AI-076: Approval Context for Low Confidence

**Description**: AIAnalysis MUST populate comprehensive `approvalContext` when confidence is below 80% threshold, providing operators with sufficient information to make informed approval decisions.

**Priority**: P0 (CRITICAL)

**Rationale**: Low-confidence recommendations require human review. Rich approval context reduces approval latency and improves decision quality. Per ADR-018, approval miss rate target is <5%.

**Implementation**:
- `status.approvalRequired`: Boolean flag for approval routing
- `status.approvalReason`: Human-readable explanation
- `status.approvalContext.confidence`: Numeric confidence value
- `status.approvalContext.investigationSummary`: Brief RCA summary
- `status.approvalContext.evidenceCollected`: Supporting evidence list
- `status.approvalContext.alternativesConsidered`: Other workflow options

**Acceptance Criteria**:
- ✅ `approvalRequired = true` when confidence < 80%
- ✅ `approvalReason` explains why approval needed
- ✅ `approvalContext` includes investigation summary
- ✅ Evidence and alternatives provided for review

**Test Coverage**:
- Unit: Approval context population logic
- Integration: Confidence threshold triggering
- E2E: Approval workflow with rich context

**Related ADRs**: ADR-018 (Approval Notification Integration), ADR-040 (RemediationApprovalRequest)

---

## 📊 Test Coverage Summary

### Unit Tests
- **Status**: Planned
- **Target Coverage**: 70%+

### Integration Tests
- **Status**: Planned
- **Target Coverage**: 50%+

### E2E Tests
- **Status**: Planned
- **Target Coverage**: 10-15%

---

## 🔗 Related Documentation

- [AIAnalysis Overview](./overview.md)
- [CRD Schema](./crd-schema.md)
- [Controller Implementation](./controller-implementation.md)
- [DD-CONTRACT-001: AIAnalysis ↔ WorkflowExecution Alignment](../../../architecture/decisions/DD-CONTRACT-001-aianalysis-workflowexecution-alignment.md)
- [ADR-041: LLM Response Contract](../../../architecture/decisions/adr-041-llm-contract/ADR-041-llm-prompt-response-contract.md)

---

## 📝 Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-28 | Initial BR document with workflow selection contract requirements |

---

**Document Version**: 1.0
**Last Updated**: November 28, 2025
**Maintained By**: Kubernaut Architecture Team
**Status**: In Development

