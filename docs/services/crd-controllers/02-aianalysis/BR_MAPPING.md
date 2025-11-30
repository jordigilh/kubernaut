# AIAnalysis Service - Business Requirements Mapping

**Service**: AIAnalysis Controller
**Version**: 1.1
**Date**: November 30, 2025
**Status**: V1.0 Scope Defined

---

## Changelog

### Version 1.1 (2025-11-30)
- **REMOVED FROM V1.0**: BR-AI-051, BR-AI-052, BR-AI-053 (Dependency Validation)
  - **Reason**: With predefined workflows from the catalog (DD-WORKFLOW-002), the LLM **selects** workflows, not **generates** them
  - Circular dependency detection (Kahn's algorithm) was designed for dynamically-generated workflow DAGs
  - Predefined workflows are **pre-validated** at registration time - no runtime validation needed
  - **Deferred To**: V2.0+ if dynamic workflow generation is added
- **CLARIFIED**: BR-AI-023 (Hallucination Detection)
  - **Old**: "Detect and handle AI hallucinations" (implied runtime DAG validation)
  - **New**: Clarified to mean **catalog validation** - ensure selected `workflowId` exists in catalog
  - Also includes: Schema validation, parameter validation, `containerImage` format validation
  - **Reference**: DD-WORKFLOW-002 v3.3 (MCP Workflow Catalog Architecture)
- **V1.0 BR Count**: Reduced from 34 to **31**

### Version 1.0 (2025-11-29)
- Initial comprehensive BR mapping for V1.0 scope

---

## Overview

This document maps all business requirements (BRs) relevant to the AIAnalysis Service, categorized by ownership (direct vs. indirect) and V1.0 scope.

**Source Documents**:
- `docs/requirements/02_AI_MACHINE_LEARNING.md` - Primary AI/ML requirements
- `docs/architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md` - Integration contracts
- `docs/architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md` - Recovery flow
- `docs/architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md` - Workflow catalog (predefined workflows)

---

## V1.0 Scope Summary

| Category | BR Count | Description |
|----------|----------|-------------|
| **Core AI Analysis** | 15 | Investigation, RCA, recommendations |
| **Approval & Policy** | 5 | Rego policies, approval signaling |
| **Data Management** | 3 | Payload handling, timeouts, fallback |
| **Quality Assurance** | 5 | Catalog validation, schema validation |
| **Workflow Selection** | 2 | Output format, approval context |
| **Recovery Flow** | 4 | Recovery attempt handling |
| **~~Dependency Validation~~** | ~~3~~ | ~~Moved to V2.0+~~ |
| **TOTAL V1.0** | **31** | |

---

## Category 1: Core AI Investigation & Analysis (15 BRs)

### Direct Ownership - Service Implements

| BR ID | Description | V1.0 | Implementation Notes |
|-------|-------------|------|---------------------|
| **BR-AI-001** | Contextual analysis of K8s alerts and system state | ✅ | HolmesGPT-API `/incident/analyze` |
| **BR-AI-002** | Support multiple analysis types (diagnostic, predictive) | ✅ | Via `analysisTypes` in spec |
| **BR-AI-003** | Generate structured analysis results with confidence scoring | ✅ | `status.confidence`, `status.rootCause` |
| **BR-AI-006** | Generate actionable remediation recommendations | ✅ | `status.selectedWorkflow` |
| **BR-AI-007** | Rank recommendations by effectiveness probability | ✅ | Confidence score from HolmesGPT |
| **BR-AI-008** | Consider historical success rates in scoring | ✅ | HolmesGPT queries Data Storage |
| **BR-AI-010** | Provide recommendation explanations with evidence | ✅ | `status.selectedWorkflow.rationale` |
| **BR-AI-011** | Conduct intelligent investigation using historical patterns | ✅ | HolmesGPT + toolsets |
| **BR-AI-012** | Identify root cause candidates with evidence | ✅ | `status.rootCause` |
| **BR-AI-013** | Correlate alerts across time windows | ✅ | HolmesGPT correlation features |
| **BR-AI-014** | Generate investigation reports with actionable insights | ✅ | `status.approvalContext.investigationSummary` |
| **BR-AI-015** | Support custom investigation scopes and time windows | ✅ | HolmesGPT-API internal config (scope determined dynamically) |
| **BR-AI-016** | Provide real-time health status | ✅ | Controller health endpoints |
| **BR-AI-017** | Track service performance metrics | ✅ | Prometheus metrics |
| **BR-AI-020** | Maintain service availability above 99.5% SLA | ✅ | Circuit breaker, retries |

---

## Category 2: Approval & Policy Management (5 BRs)

### Direct Ownership - Service Implements

| BR ID | Description | V1.0 | Implementation Notes |
|-------|-------------|------|---------------------|
| **BR-AI-026** | Evaluate recommendations against configurable approval policies | ✅ | Rego policy engine (DD-AIANALYSIS-001) |
| **BR-AI-027** | Secure approval actions with Kubernetes RBAC | ✅ | `RemediationApprovalRequest` CRD (V1.1) |
| **BR-AI-028** | Implement Rego-based approval policies | ✅ | ConfigMap `ai-approval-policies` |
| **BR-AI-029** | Support zero-downtime policy updates | ✅ | ConfigMap watch + reload |
| **BR-AI-030** | Maintain policy audit trail for approval decisions | ✅ | `status.approvalContext.policyEvaluation` |

---

## Category 3: Quality Assurance (5 BRs)

### Direct Ownership - Service Implements

| BR ID | Description | V1.0 | Implementation Notes |
|-------|-------------|------|---------------------|
| **BR-AI-021** | Validate AI responses for completeness and accuracy | ✅ | Response schema validation (required fields present) |
| **BR-AI-022** | Implement confidence thresholds for automated decisions | ✅ | 80% threshold for auto-approval |
| **BR-AI-023** | Validate workflow selection against catalog | ✅ | See V1.0 clarification below |
| **BR-AI-024** | Provide fallback when AI services unavailable | ✅ | Graceful degradation |
| **BR-AI-025** | Maintain response quality metrics | ✅ | Prometheus metrics |

### BR-AI-023 V1.0 Clarification

**Context**: With predefined workflows (DD-WORKFLOW-002), "hallucination detection" means:

| Validation Type | Description | V1.0? |
|-----------------|-------------|-------|
| **Workflow ID Validation** | Ensure `workflowId` exists in catalog | ✅ |
| **Schema Validation** | Ensure response matches expected JSON schema | ✅ |
| **Parameter Validation** | Ensure parameters are valid for selected workflow | ✅ |
| **ContainerImage Format** | Ensure `containerImage` is valid OCI reference | ✅ |
| ~~Circular DAG Detection~~ | ~~Detect cycles in dynamically-generated workflows~~ | ❌ N/A |
| ~~Invalid Action Detection~~ | ~~Detect non-existent workflow steps~~ | ❌ N/A |

**Reference**: DD-WORKFLOW-002 v3.3 - LLM selects from catalog, does not generate workflows.

---

## Category 4: Data Management (3 BRs)

### Direct Ownership - Service Implements

| BR ID | Description | V1.0 | Implementation Notes |
|-------|-------------|------|---------------------|
| **BR-AI-031** | Handle large payloads without exceeding etcd limits | ✅ | Selective embedding (50KB/100KB thresholds) |
| **BR-AI-032** | Implement phase-specific timeouts | ✅ | Configurable via annotations |
| **BR-AI-033** | Gracefully handle missing historical success rate data | ✅ | Tiered fallback strategy |

---

## Category 5: Workflow Selection Contract (2 BRs)

### Direct Ownership - Service Implements

| BR ID | Description | V1.0 | Implementation Notes |
|-------|-------------|------|---------------------|
| **BR-AI-075** | Workflow selection output format per ADR-041 | ✅ | `status.selectedWorkflow` with `workflowId`, `containerImage`, `parameters` |
| **BR-AI-076** | Approval context for low confidence (<80%) | ✅ | `status.approvalContext` with rich context |

---

## Category 6: Recovery Flow (4 BRs)

### Direct Ownership - Service Implements (DD-RECOVERY-002)

| BR ID | Description | V1.0 | Implementation Notes |
|-------|-------------|------|---------------------|
| **BR-AI-080** | Support recovery attempts from failed WorkflowExecution | ✅ | `spec.isRecoveryAttempt`, `spec.recoveryAttemptNumber` |
| **BR-AI-081** | Accept previous execution context for recovery analysis | ✅ | `spec.previousExecution` with failure details |
| **BR-AI-082** | Call HolmesGPT-API recovery endpoint for failed workflows | ✅ | `POST /api/v1/recovery/analyze` |
| **BR-AI-083** | Reuse original enrichment without re-enriching | ✅ | `spec.enrichmentResults` (copied from original SignalProcessing) |

---

## ~~Category 7: Dependency Validation (3 BRs)~~ → Deferred to V2.0+

### ❌ NOT APPLICABLE FOR V1.0

**Reason**: V1.0 uses **predefined workflows** from the catalog (DD-WORKFLOW-002). These BRs were designed for a dynamic workflow generation architecture that was not implemented.

| BR ID | Description | V1.0 | Deferred Reason |
|-------|-------------|------|-----------------|
| **BR-AI-051** | Validate AI responses for dependency completeness | ❌ | Predefined workflows have no runtime dependencies |
| **BR-AI-052** | Detect circular dependencies in recommendation graphs | ❌ | No DAG generation - LLM selects from catalog |
| **BR-AI-053** | Handle missing/invalid dependencies with fallback | ❌ | Predefined workflows are pre-validated at registration |

**Reference**: DD-WORKFLOW-002 v3.3 - Workflow Catalog Architecture (predefined, immutable workflows)

**Future Scope**: If dynamic workflow generation is added in V2.0+, these BRs will be relevant.

---

## Indirect Dependencies (Upstream)

### SignalProcessing → AIAnalysis

| BR ID | Source | Relationship | Notes |
|-------|--------|--------------|-------|
| **BR-SP-001** | SignalProcessing | Provides `EnrichmentResults` | Structured K8s context |
| **BR-SP-002** | SignalProcessing | Provides `HistoricalContext` | Historical patterns |
| **BR-SP-003** | SignalProcessing | Provides `EnrichmentQuality` | Quality score (0.0-1.0) |

### HolmesGPT-API → AIAnalysis

| BR ID | Source | Relationship | Notes |
|-------|--------|--------------|-------|
| **BR-HAPI-001** | HolmesGPT-API | Investigation results | `/incident/analyze` response |
| **BR-HAPI-002** | HolmesGPT-API | Recovery analysis | `/recovery/analyze` response |
| **BR-HAPI-250** | HolmesGPT-API | Workflow catalog search | MCP tool with `containerImage` |

---

## Indirect Dependencies (Downstream)

### AIAnalysis → RemediationOrchestrator

| BR ID | Target | Relationship | Notes |
|-------|--------|--------------|-------|
| **BR-RO-010** | RO | Watches AIAnalysis.status | Creates WorkflowExecution |
| **BR-RO-011** | RO | Reads `selectedWorkflow` | Uses for WorkflowExecution.spec |
| **BR-RO-012** | RO | Reads `approvalRequired` | Orchestrates approval flow |

### AIAnalysis → WorkflowExecution

| BR ID | Target | Relationship | Notes |
|-------|--------|--------------|-------|
| **BR-WE-001** | WorkflowExecution | Receives `containerImage` | From `selectedWorkflow.containerImage` |
| **BR-WE-002** | WorkflowExecution | Receives `parameters` | From `selectedWorkflow.parameters` |

---

## Deferred to V1.1+

### Deferred - Dynamic Workflow Generation (V2.0+)

| BR ID | Description | Deferred Reason | Reference |
|-------|-------------|-----------------|-----------|
| **BR-AI-051** | Validate AI responses for dependency completeness | V1.0 uses predefined workflows | DD-WORKFLOW-002 |
| **BR-AI-052** | Detect circular dependencies in recommendation graphs | V1.0 uses predefined workflows | DD-WORKFLOW-002 |
| **BR-AI-053** | Handle missing/invalid dependencies with fallback | V1.0 uses predefined workflows | DD-WORKFLOW-002 |
| **BR-AI-071-074** | AI-driven dependency cycle correction | Requires dynamic workflow generation | V2.0+ |

### Deferred - Advanced Features (V1.1+/V2.0+)

| BR ID | Description | Deferred Reason | Target Version |
|-------|-------------|-----------------|----------------|
| **BR-AI-037** | Historical pattern analysis before resource increases | Advanced analytics | V2.0 |
| **BR-LLM-001-005** | Multi-provider LLM support | Multi-provider | V2.0 |
| **BR-LLM-010** | Cost optimization with model selection | Multi-provider | V2.0 |
| **BR-COND-001-020** | AI Conditions Engine | Advanced features | V2.0 |
| **BR-INS-001-020** | AI Insights Service (full) | V1.0 has graceful degradation only | V1.1+ |

---

## Design Decision References

| DD ID | Title | Relevance |
|-------|-------|-----------|
| **DD-CONTRACT-001** | AIAnalysis ↔ WorkflowExecution Alignment | Schema contract |
| **DD-CONTRACT-002** | Service Integration Contracts | Integration flow |
| **DD-RECOVERY-002** | Direct AIAnalysis Recovery Flow | Recovery pattern |
| **DD-RECOVERY-003** | Recovery Prompt Design | HolmesGPT-API integration |
| **DD-AIANALYSIS-001** | Rego Policy Loading Strategy | Approval policy implementation |
| **DD-WORKFLOW-002** | MCP Workflow Catalog Architecture | Predefined workflows (v3.3) - justifies BR-AI-051-053 deferral |
| **DD-WORKFLOW-012** | Workflow Immutability Constraints | Pre-validated workflows at registration |
| **ADR-041** | LLM Prompt and Response Contract | HolmesGPT response format |

---

## ADR References

| ADR ID | Title | Relevance |
|--------|-------|-----------|
| **ADR-018** | Approval Notification Integration | V1.0 approval flow |
| **ADR-040** | RemediationApprovalRequest Architecture | V1.1 approval CRD |
| **ADR-041** | LLM Prompt Response Contract | HolmesGPT integration |

---

## Test Coverage Mapping

| BR Category | Unit Tests | Integration Tests | E2E Tests |
|-------------|-----------|-------------------|-----------|
| Core AI Analysis | BR-AI-001-015 | HolmesGPT integration | Full flow |
| Approval & Policy | BR-AI-026-030 | Rego policy evaluation | Approval workflow |
| Quality Assurance | BR-AI-021-025 | Catalog validation, schema checks | Graceful degradation |
| Data Management | BR-AI-031-033 | Payload handling | Timeout scenarios |
| Workflow Selection | BR-AI-075-076 | Contract validation | RO integration |
| Recovery Flow | BR-AI-080-083 | Recovery analysis | Recovery E2E |

> **Note**: BR-AI-051-053 (Dependency Validation) removed from V1.0 - see Deferred section.

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2025-11-30 | Removed BR-AI-051-053 from V1.0 (predefined workflows); Clarified BR-AI-023 |
| 1.0 | 2025-11-29 | Initial comprehensive BR mapping for V1.0 scope |

---

**Document Version**: 1.1
**Last Updated**: November 30, 2025
**Maintained By**: AIAnalysis Service Team

