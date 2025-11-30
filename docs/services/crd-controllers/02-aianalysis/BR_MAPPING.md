# AIAnalysis Service - Business Requirements Mapping

**Service**: AIAnalysis Controller
**Version**: 1.0
**Date**: November 29, 2025
**Status**: V1.0 Scope Defined

---

## Overview

This document maps all business requirements (BRs) relevant to the AIAnalysis Service, categorized by ownership (direct vs. indirect) and V1.0 scope.

**Source Documents**:
- `docs/requirements/02_AI_MACHINE_LEARNING.md` - Primary AI/ML requirements
- `docs/architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md` - Integration contracts
- `docs/architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md` - Recovery flow

---

## V1.0 Scope Summary

| Category | BR Count | Description |
|----------|----------|-------------|
| **Core AI Analysis** | 15 | Investigation, RCA, recommendations |
| **Approval & Policy** | 5 | Rego policies, approval signaling |
| **Data Management** | 3 | Payload handling, timeouts, fallback |
| **Quality Assurance** | 5 | Validation, hallucination detection |
| **Workflow Selection** | 2 | Output format, approval context |
| **Recovery Flow** | 4 | Recovery attempt handling |
| **TOTAL V1.0** | **34** | |

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
| **BR-AI-015** | Support custom investigation scopes and time windows | ✅ | `spec.analysisRequest.investigationScope` |
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
| **BR-AI-021** | Validate AI responses for completeness and accuracy | ✅ | Response schema validation |
| **BR-AI-022** | Implement confidence thresholds for automated decisions | ✅ | 80% threshold for auto-approval |
| **BR-AI-023** | Detect and handle AI hallucinations | ✅ | Response validation + fallback |
| **BR-AI-024** | Provide fallback when AI services unavailable | ✅ | Graceful degradation |
| **BR-AI-025** | Maintain response quality metrics | ✅ | Prometheus metrics |

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

## Category 7: Dependency Validation (3 BRs)

### Direct Ownership - Service Implements

| BR ID | Description | V1.0 | Implementation Notes |
|-------|-------------|------|---------------------|
| **BR-AI-051** | Validate AI responses for dependency completeness | ✅ | Dependency ID validation |
| **BR-AI-052** | Detect circular dependencies in recommendation graphs | ✅ | Kahn's algorithm |
| **BR-AI-053** | Handle missing/invalid dependencies with fallback | ✅ | Sequential execution fallback |

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

| BR ID | Description | Deferred Reason |
|-------|-------------|-----------------|
| **BR-AI-071-074** | AI-driven dependency cycle correction | V1.2 extension scope |
| **BR-AI-037** | Historical pattern analysis before resource increases | V2.0 advanced analytics |
| **BR-LLM-001-005** | Multi-provider LLM support | V2.0 multi-provider |
| **BR-LLM-010** | Cost optimization with model selection | V2.0 multi-provider |
| **BR-COND-001-020** | AI Conditions Engine | V2.0 advanced features |
| **BR-INS-001-020** | AI Insights Service (full) | V1.0 has graceful degradation only |

---

## Design Decision References

| DD ID | Title | Relevance |
|-------|-------|-----------|
| **DD-CONTRACT-001** | AIAnalysis ↔ WorkflowExecution Alignment | Schema contract |
| **DD-CONTRACT-002** | Service Integration Contracts | Integration flow |
| **DD-RECOVERY-002** | Direct AIAnalysis Recovery Flow | Recovery pattern |
| **DD-RECOVERY-003** | Recovery Prompt Design | HolmesGPT-API integration |
| **DD-AIANALYSIS-001** | Rego Policy Loading Strategy | Approval policy implementation |
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
| Quality Assurance | BR-AI-021-025 | Response validation | Graceful degradation |
| Data Management | BR-AI-031-033 | Payload handling | Timeout scenarios |
| Workflow Selection | BR-AI-075-076 | Contract validation | RO integration |
| Recovery Flow | BR-AI-080-083 | Recovery analysis | Recovery E2E |

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-29 | Initial comprehensive BR mapping for V1.0 scope |

---

**Document Version**: 1.0
**Last Updated**: November 29, 2025
**Maintained By**: AIAnalysis Service Team

