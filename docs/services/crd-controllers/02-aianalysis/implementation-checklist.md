# Implementation Checklist

**Version**: v2.2
**Status**: ✅ Complete - V1.0 Aligned
**Last Updated**: 2025-12-02

---

## 📋 Changelog

| Version | Date | Changes |
|---------|------|---------|
| **v2.2** | 2025-12-02 | Added `FailedDetections` handling tasks per DD-WORKFLOW-001 v2.1; Added Rego policy input for failed detections |
| v2.1 | 2025-12-02 | Added Go client generation from OpenAPI spec; Added `TargetInOwnerChain` and `Warnings` handling from HAPI response; Updated endpoints to correct paths |
| v2.0 | 2025-11-30 | **V1.0 ALIGNMENT**: Updated to 31 BRs (per BR_MAPPING.md v1.2); Updated to 4-phase flow (Pending → Validating → Investigating → Ready/Failed); Removed AIApprovalRequest references; Added DetectedLabels/CustomLabels/OwnerChain phases |
| v1.0 | 2025-10-15 | Initial specification |

---

## Overview

**Note**: Follow APDC-TDD phases for each implementation step (see [Core Development Methodology](.cursor/rules/00-core-development-methodology.mdc))

**Business Requirements**: 31 V1.0 BRs mapped (per [BR_MAPPING.md](./BR_MAPPING.md) v1.2)

---

## Phase 0: Project Setup (30 min) [BEFORE ANALYSIS]

- [ ] **Verify cmd/ structure**: Check [cmd/README.md](../../../../cmd/README.md)
- [ ] **Create service directory**: `mkdir -p cmd/aianalysis` (no hyphens - Go convention)
- [ ] **Copy main.go template**: From `cmd/signalprocessing/main.go`
- [ ] **Update package imports**: Change to service-specific controller (AIAnalysisReconciler)
- [ ] **Verify build**: `go build -o bin/ai-analysis ./cmd/aianalysis` (binary can have hyphens)
- [ ] **Reference documentation**: [cmd/ directory guide](../../../../cmd/README.md)

**Note**: Directory names use Go convention (no hyphens), binaries can use hyphens for readability.

### Logging Library

- **Library**: `sigs.k8s.io/controller-runtime/pkg/log/zap`
- **Rationale**: Official controller-runtime integration with opinionated defaults for Kubernetes controllers
- **Setup**: Initialize in `main.go` with `ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))`
- **Usage**: `log := ctrl.Log.WithName("aianalysis")`

---

## Phase 1: ANALYSIS & CRD Setup (1-2 days) [RED Phase Preparation]

### ANALYSIS Phase

- [ ] **ANALYSIS**: Search existing AI implementations (`codebase_search "AI analysis implementations"`)
- [ ] **ANALYSIS**: Map business requirements across V1.0 scope:
  - **Total V1.0 BRs**: 31 (per BR_MAPPING.md v1.2)
  - **Core AI Analysis** (15 BRs): BR-AI-001 to BR-AI-025
  - **Approval & Policy** (5 BRs): BR-AI-030 to BR-AI-035
  - **Data Management** (3 BRs): BR-AI-020 to BR-AI-028
  - **Quality Assurance** (5 BRs): BR-AI-023 (catalog validation), etc.
  - **Workflow Selection** (2 BRs): BR-AI-075, BR-AI-076
  - **Recovery Flow** (4 BRs): BR-AI-080 to BR-AI-083
- [ ] **ANALYSIS**: Identify integration points with:
  - SignalProcessing (upstream)
  - HolmesGPT-API (external)
  - Remediation Orchestrator (parent)
  - WorkflowExecution (downstream, via RO)
- [ ] **ANALYSIS**: Review authoritative documents:
  - [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md)
  - [DD-RECOVERY-002](../../../architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md)
  - [DD-RECOVERY-003](../../../architecture/decisions/DD-RECOVERY-003-recovery-prompt-design.md)

### CRD Setup

- [ ] **CRD RED**: Write AIAnalysisReconciler tests (should fail - no controller yet)
- [ ] **CRD GREEN**: Generate CRD + controller skeleton (tests pass)
  - [ ] Verify AIAnalysis CRD types exist in `api/aianalysis/v1alpha1/`
  - [ ] Generate Kubebuilder controller scaffold
  - [ ] Implement AIAnalysisReconciler with finalizers
  - [ ] Configure owner references from RemediationRequest CRD
- [ ] **CRD REFACTOR**: Enhance controller with error handling
  - [ ] Add controller-specific Prometheus metrics
  - [ ] Implement cross-CRD reference validation
  - [ ] Add phase timeout detection (default: 5 min per phase)

---

## Phase 2: Package Structure & Business Logic (2-3 days) [RED-GREEN-REFACTOR]

### Package Structure

- [ ] **Package RED**: Write tests for Analyzer interface (fail - no interface yet)
- [ ] **Package GREEN**: Create `pkg/aianalysis/` package structure
  - [ ] Define `Analyzer` interface (no "Service" suffix)
  - [ ] Create phase handlers: Pending, Validating, Investigating, Ready/Failed

### Core Business Logic

- [ ] **Package REFACTOR**: Enhance with sophisticated logic
  - [ ] Implement input validation (BR-AI-020, BR-AI-021)
  - [ ] Implement catalog validation - workflow ID, schema, parameters (BR-AI-023)
  - [ ] Implement confidence scoring algorithms

---

## Phase 3: Reconciliation Phases (2-3 days) [RED-GREEN-REFACTOR]

**V1.0 Phase Flow**: Pending → Validating → Investigating → Ready/Failed

### Pending Phase

- [ ] **RED**: Write tests for Pending phase (fail)
- [ ] **GREEN**: Implement minimal Pending logic
  - [ ] Set initial phase to "Pending"
  - [ ] Emit Kubernetes event: "AIAnalysisCreated"
- [ ] **REFACTOR**: Add timeout handling

### Validating Phase

- [ ] **RED**: Write tests for Validating phase (fail)
- [ ] **GREEN**: Implement Validating logic
  - [ ] Validate SignalContext structure
  - [ ] Validate DetectedLabels presence
  - [ ] Validate CustomLabels structure (map[string][]string)
  - [ ] Validate OwnerChain structure
  - [ ] Validate required fields (Environment, BusinessPriority as free-text)
- [ ] **REFACTOR**: Add detailed validation error messages

> **Note**: `EnrichmentQuality` validation removed (Dec 2025) - field no longer exists

### Investigating Phase

- [ ] **RED**: Write tests for Investigating phase (fail)
- [ ] **GREEN**: Implement Investigating logic
  - [ ] **Call HolmesGPT-API** with investigation request
  - [ ] **Pass DetectedLabels** for workflow filtering context
  - [ ] **Pass CustomLabels** for workflow filtering context
  - [ ] **Pass OwnerChain** for DetectedLabels validation
  - [ ] **Handle recovery context** if `isRecoveryAttempt = true`
  - [ ] **Parse HolmesGPT-API response**:
    - Root Cause Analysis (RCA)
    - Workflow Selection (workflow ID, parameters, confidence)
    - `TargetInOwnerChain` → store in `status.targetInOwnerChain`
    - `Warnings[]` → store in `status.warnings`, emit K8s events
  - [ ] **Evaluate Rego approval policies** (BR-AI-030)
    - Include `targetInOwnerChain` in Rego input for approval decisions
  - [ ] **Set approvalRequired flag** based on Rego evaluation
- [ ] **REFACTOR**: Enhanced error handling
  - [ ] HolmesGPT-API timeout handling (30s default)
  - [ ] Rego policy evaluation error handling
  - [ ] Retry logic for transient failures

### Ready/Failed Phase

- [ ] **RED**: Write tests for Ready/Failed phases (fail)
- [ ] **GREEN**: Implement terminal phase logic
  - [ ] **Ready**: Set status fields, emit completion event
  - [ ] **Failed**: Set error message, emit failure event
- [ ] **REFACTOR**: Add audit recording

---

## Phase 4: Integration & Testing (2-3 days) [RED-GREEN-REFACTOR]

### HolmesGPT-API Integration

- [ ] **Generate Go client from OpenAPI spec** (prerequisite):
  ```bash
  # Use ogen for native OpenAPI 3.1.0 support
  # Install: go install github.com/ogen-go/ogen/cmd/ogen@latest
  mkdir -p pkg/clients/holmesgpt
  ogen -package holmesgpt -target pkg/clients/holmesgpt \
      holmesgpt-api/api/openapi.json
  ```
  - [ ] Verify generated types include `IncidentResponse`, `RecoveryResponse`
  - [ ] Verify `TargetInOwnerChain` and `Warnings` fields present

- [ ] **Integration RED**: Write tests for HolmesGPT-API client (fail)
- [ ] **Integration GREEN**: Implement HolmesGPT-API client (tests pass)
  - [ ] Investigation endpoint (`/api/v1/incident/analyze`)
  - [ ] Recovery analysis endpoint (`/api/v1/recovery/analyze`)
  - [ ] Health check endpoint (`/health`)
- [ ] **Integration REFACTOR**: Enhance with error handling and retries
  - [ ] Handle `TargetInOwnerChain=false` → store in status, consider in Rego
  - [ ] Handle `Warnings[]` → store in status, emit as events, track metrics

### FailedDetections Handling (DD-WORKFLOW-001 v2.1)

- [ ] **FailedDetections RED**: Write tests for detection failure handling
  - [ ] Test: Field in `FailedDetections` → value is ignored in Rego
  - [ ] Test: Empty `FailedDetections` → all values trusted
  - [ ] Test: Invalid field name in `FailedDetections` → validation error
- [ ] **FailedDetections GREEN**: Implement detection failure handling
  - [ ] Parse `FailedDetections` from HolmesGPT-API response
  - [ ] Include `failed_detections` in Rego policy input
  - [ ] Emit metrics for detection failures (`aianalysis_detection_failures_total`)
- [ ] **FailedDetections REFACTOR**: Add logging and events
  - [ ] Log detection failures at WARN level
  - [ ] Emit K8s event: "DetectionFailed" with affected fields

### Rego Policy Integration

- [ ] **Rego RED**: Write tests for Rego policy evaluation (fail)
- [ ] **Rego GREEN**: Implement Rego evaluator (tests pass)
  - [ ] Load policies from ConfigMap (`ai-approval-policies` in `kubernaut-system`)
  - [ ] Evaluate with ApprovalPolicyInput schema (per [REGO_POLICY_EXAMPLES.md](./REGO_POLICY_EXAMPLES.md) v1.4)
  - [ ] Handle `failed_detections` in policy input (ignore affected field values)
  - [ ] Return approval decision
- [ ] **Rego REFACTOR**: Add policy caching and refresh

### Storage Integration

- [ ] **Storage RED**: Write tests for Data Storage integration (fail)
- [ ] **Storage GREEN**: Implement storage client (tests pass)
  - [ ] Audit recording
  - [ ] Historical pattern lookup
- [ ] **CRD Integration**: Verify RO creates WorkflowExecution (not AIAnalysis controller)

### Main App Integration

- [ ] **Main App Integration**: Verify AIAnalysisReconciler instantiated in cmd/aianalysis/ (MANDATORY)
- [ ] **Port Configuration**: Health port 8081, Metrics port 9090, Service host port 8084

---

## Phase 5: Testing & Validation (1-2 days) [CHECK Phase]

### Unit Tests (70%+ coverage target)

- [ ] **CHECK**: Verify unit test coverage (test/unit/aianalysis/)
  - [ ] Reconciler tests with fake K8s client, mocked HolmesGPT-API
  - [ ] Validating phase tests (input validation)
  - [ ] Investigating phase tests (HolmesGPT-API call, Rego evaluation)
  - [ ] Catalog validation tests (BR-AI-023)
  - [ ] Rego policy evaluation tests (BR-AI-030)
  - [ ] Recovery context handling tests (BR-AI-080 to BR-AI-083)

### Integration Tests (20% coverage target)

- [ ] **CHECK**: Run integration tests (test/integration/aianalysis/)
  - [ ] Real HolmesGPT-API integration (mock or stub)
  - [ ] Real K8s API (KIND) CRD lifecycle tests
  - [ ] Cross-CRD coordination tests (RO → AIAnalysis)
  - [ ] Rego policy ConfigMap loading tests

### E2E Tests (10% coverage target)

- [ ] **CHECK**: Execute E2E tests (test/e2e/aianalysis/)
  - [ ] Complete signal-to-workflow flow
  - [ ] Recovery attempt scenarios
  - [ ] Approval signaling scenarios

### Validation

- [ ] **CHECK**: Validate business requirement coverage (31 V1.0 BRs)
- [ ] **CHECK**: Performance validation (investigation <30s, total <60s)
- [ ] **CHECK**: Provide confidence assessment (target 95%+)

---

## Phase 6: Metrics, Audit & Deployment (1 day)

### Metrics

- [ ] **Metrics**: Define and implement Prometheus metrics
  - [ ] Investigation, validation phase metrics
  - [ ] Implement metrics recording in reconciler
  - [ ] Setup metrics server on port 9090 (with auth)
  - [ ] Create Grafana dashboard queries
  - [ ] Set performance targets (p95 < 30s, confidence > 0.8)

### Audit

- [ ] **Audit**: Database integration for compliance
  - [ ] Implement audit client (`integration/audit.go`)
  - [ ] Record investigation results to PostgreSQL
  - [ ] Record workflow selection to PostgreSQL
  - [ ] Record DetectedLabels/CustomLabels to PostgreSQL
  - [ ] Implement historical queries

### Deployment

- [ ] **Deployment**: Binary and infrastructure
  - [ ] Create `cmd/aianalysis/main.go` entry point
  - [ ] Configure Kubebuilder manager with leader election
  - [ ] Add RBAC permissions for CRD operations
  - [ ] Create Kubernetes deployment manifests
  - [ ] Configure HolmesGPT-API service discovery

---

## Phase 7: Documentation (1 day)

- [ ] Update API documentation with AIAnalysis CRD
- [ ] Document HolmesGPT-API integration patterns
- [ ] Add troubleshooting guide for AI analysis
- [ ] Create runbook for catalog validation failures
- [ ] Document Rego policy authoring guide (per [REGO_POLICY_EXAMPLES.md](./REGO_POLICY_EXAMPLES.md))

---

## V1.0 Scope Boundaries

### ✅ In Scope

| Feature | BRs | Description |
|---------|-----|-------------|
| HolmesGPT-API Integration | BR-AI-001 to BR-AI-025 | Single AI provider |
| Workflow Selection | BR-AI-075, BR-AI-076 | Select from catalog |
| Rego Approval Policies | BR-AI-030 to BR-AI-035 | ConfigMap-based |
| Recovery Flow | BR-AI-080 to BR-AI-083 | Handle failed retries |
| Catalog Validation | BR-AI-023 | Validate workflow ID, schema, parameters |
| DetectedLabels | DD-WORKFLOW-001 v1.8 | Auto-detected labels |
| CustomLabels | DD-WORKFLOW-001 v1.5 | Rego-extracted labels |
| OwnerChain | DD-WORKFLOW-001 v1.8 | K8s ownership validation |

### ❌ Out of Scope (V1.1+)

| Feature | Target Version | Reason |
|---------|----------------|--------|
| AIApprovalRequest CRD | V1.1 | Approval orchestration via CRD |
| Multi-provider LLM | V2.0 | OpenAI, Anthropic, etc. |
| Dependency Validation | V2.0+ | Predefined workflows in V1.0 |
| AI Conditions Engine | V2.0 | Advanced condition evaluation |

---

## Quick Reference

### Authoritative Documents

| Document | Purpose |
|----------|---------|
| [BR_MAPPING.md](./BR_MAPPING.md) v1.2 | 31 V1.0 BRs |
| [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | Label schema |
| [DD-RECOVERY-002](../../../architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md) | Recovery flow |
| [REGO_POLICY_EXAMPLES.md](./REGO_POLICY_EXAMPLES.md) | Rego schema |
| [TESTING_GUIDELINES.md](../../../development/business-requirements/TESTING_GUIDELINES.md) | Testing patterns |

### Port Allocation

| Port | Purpose |
|------|---------|
| 8081 | Health/Ready (`/healthz`, `/readyz`) |
| 9090 | Metrics (`/metrics`) |
| 8084 | Service Host (Kind extraPortMappings) |

---

## References

- [Controller Implementation](./controller-implementation.md) - Reconciler logic
- [Reconciliation Phases](./reconciliation-phases.md) - 4-phase flow
- [Testing Strategy](./testing-strategy.md) - Test patterns
- [DD-006](../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md) - Scaffolding strategy
