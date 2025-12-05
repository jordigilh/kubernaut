# AI Analysis Service - Implementation Plan

**Filename**: `IMPLEMENTATION_PLAN_V1.0.md`
**Version**: v1.8
**Last Updated**: 2025-12-05
**Timeline**: 10 days (2 calendar weeks)
**Status**: 📋 DRAFT - Ready for Review
**Quality Level**: Matches SignalProcessing V1.19 and Template V3.0 standards
**Template Reference**: [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v3.0](../../SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md)

**Change Log**:
- **v1.8** (2025-12-05): **BREAKING** Recommending phase removed per spec alignment
  - ✅ **Spec Authority**: `reconciliation-phases.md` v2.0 defines 4 phases: `Pending → Investigating → Analyzing → Completed`
  - ✅ **Recommending Removed**: Phase provided no value; workflow data already captured in Investigating
  - ✅ **AnalyzingHandler**: Now transitions directly to `Completed` and populates `ApprovalContext`
  - ✅ **Day 4 Repurposed**: Now covers ApprovalContext population + Midpoint checkpoint
  - ❌ **RecommendingHandler**: Removed (logic merged into AnalyzingHandler)
  - 📏 **Authority**: `docs/services/crd-controllers/02-aianalysis/reconciliation-phases.md` v2.0
- **v1.7** (2025-12-05): PolicyInput schema implementation complete
  - ✅ **Day 3 Complete**: AnalyzingHandler + Rego Policy Engine fully implemented
  - ✅ **PolicyInput Schema**: Extended to match plan lines 1756-1785 (ApprovalInput)
    - Signal context: `SignalType`, `Severity`, `BusinessPriority`
    - Target resource: `Kind`, `Name`, `Namespace`
    - Recovery context: `IsRecoveryAttempt`, `RecoveryAttemptNumber`
  - ✅ **Recovery Rules**: Test policy includes 3+ attempt escalation, high severity + recovery
  - ✅ **Tests**: 61 total tests passing (13 new recovery/signal context tests)
  - 📏 **Files**: `pkg/aianalysis/rego/evaluator.go`, `pkg/aianalysis/handlers/analyzing.go`
- **v1.6** (2025-12-05): OPA v1 Rego syntax documentation
  - ✅ **OPA v1 Syntax**: All Rego policies MUST use OPA v1 syntax (`if` keyword, `:=` operator)
  - ✅ **Import Statement**: Policies should use `import rego.v1` or explicit v1 syntax
  - ✅ **Breaking Change**: Old Rego syntax (`default x = false`, rules without `if`) will NOT compile
  - 📏 **Package**: `github.com/open-policy-agent/opa/v1/rego`
  - 📏 **Reference**: See test policies in `test/unit/aianalysis/testdata/policies/`
- **v1.5** (2025-12-05): CRD schema update + Day 2 InvestigatingHandler enhancement
  - ✅ **CRD Schema**: Added `AlternativeWorkflow` type and `AlternativeWorkflows []AlternativeWorkflow` to status
  - ✅ **Architecture Clarification**: `/incident/analyze` returns ALL data (RCA + workflow + alternatives) in one call
  - ✅ **Day 2 Enhancement**: InvestigatingHandler now captures full response (RCA, SelectedWorkflow, AlternativeWorkflows)
  - ✅ **Key Principle**: "Alternatives are for CONTEXT, not EXECUTION" per HolmesGPT-API team
  - 📏 **Reference**: [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../../../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) Q12-Q13
- **v1.4** (2025-12-05): CRD phase alignment - removed Validating phase
  - ✅ **Phase Alignment**: Removed `Validating` phase references (not in CRD spec)
  - ✅ **Day 2 Update**: Validation logic moves to `Pending` → `Investigating` transition
  - ✅ **Day Structure**: Day 2 now covers PendingHandler with validation + InvestigatingHandler prep
  - 📏 **Authority**: `reconciliation-phases.md` v2.0
- **v1.3** (2025-12-04): Test package naming compliance fix
  - ✅ **Package Naming Fix**: Changed all `package aianalysis_test` → `package aianalysis`
  - ✅ **Compliance**: Now compliant with TEST_PACKAGE_NAMING_STANDARD.md (white-box testing)
  - ✅ **Files Fixed**: DAY_05_07, DAY_08_10, APPENDIX_D (12 violations corrected)
  - 📏 **Authority**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v3.0 lines 2586-2619
- **v1.2** (2025-12-04): Expanded plan with appendices and day-by-day breakdown
  - ✅ **Appendices**: Created 4 detailed appendix documents
  - ✅ **Day-by-Day**: Created 5 detailed day breakdown documents
  - ✅ **Port Allocation**: Added complete Kind config and NodePort details
  - 📏 **Plan size**: ~2,600 lines (core) + ~1,800 lines (appendices) + ~2,400 lines (days)
- **v1.1** (2025-12-04): Template v3.0 full alignment
  - ✅ **Edge Case Categories**: Added template section ⭐ V3.0
  - ✅ **Metrics Validation Commands**: Added Day 7 template ⭐ V3.0
  - ✅ **Lessons Learned Template**: Added Day 10 deliverable ⭐ V3.0
  - ✅ **Technical Debt Template**: Added Day 10 deliverable ⭐ V3.0
  - ✅ **Team Handoff Notes Template**: Added Day 10 deliverable ⭐ V3.0
  - ✅ **CRD API Group Standard**: Added reference section ⭐ V3.0
  - 📏 **Plan size**: ~3,800 lines
- **v1.0** (2025-12-03): Initial implementation plan
  - ✅ **Template v3.0 Compliance**: Cross-team validation, risk mitigation tracking
  - ✅ **Rego Policy Testing Strategy**: Adapted from SignalProcessing V1.19
  - ✅ **DD-WORKFLOW-001 v2.2**: PodSecurityLevel removed, FailedDetections included
  - ✅ **KIND Integration Tests**: MockLLMServer pattern from HolmesGPT-API
  - ✅ **Existing Infrastructure Verified**: CRD types, Go client, shared types
  - 📏 **Plan size**: ~3,500 lines

---

## 🎯 Quick Reference

**Use this plan for**: AI Analysis CRD Controller implementation
**Based on**: SignalProcessing V1.19 + Template V3.0 patterns
**Methodology**: APDC-TDD with Defense-in-Depth Testing (Unit → Integration → E2E)
**Parallel Execution**: **4 concurrent processes** for all test tiers
**Test Environment**: 🔴 **KIND** (writes CRDs + external dependencies)

---

## 📑 **Table of Contents**

| Section | Line | Purpose |
|---------|------|---------|
| [Quick Reference](#-quick-reference) | ~30 | Plan overview |
| [Service Overview](#-service-overview) | ~70 | AIAnalysis service context |
| [Prerequisites Checklist](#-prerequisites-checklist) | ~130 | Pre-Day 1 requirements |
| [Cross-Team Validation](#-cross-team-validation--v30) | ~210 | Multi-team dependency sign-off |
| [Integration Test Environment](#-integration-test-environment-decision) | ~330 | KIND decision and MockLLMServer |
| [Pre-Implementation Design Decisions](#-pre-implementation-design-decisions) | ~410 | Ambiguous requirement resolution |
| [Risk Assessment Matrix](#️-risk-assessment-matrix) | ~510 | Risk identification and mitigation |
| [Files Affected](#-files-affected-section) | ~610 | New/modified/deleted files |
| [Timeline Overview](#-timeline-overview) | ~710 | 10-day phase breakdown |
| [Day-by-Day Breakdown](#-day-by-day-breakdown) | ~760 | Detailed daily tasks |
| [Rego Policy Testing Strategy](#-rego-policy-testing-strategy) | ~1850 | Approval policy testing |
| [Business Requirements Coverage](#-business-requirements-coverage-matrix) | ~2050 | BR-AI-XXX mapping |
| [Production Readiness Checklist](#-production-readiness-checklist) | ~2250 | Deployment checklist |
| **V3.0 Templates** | | |
| ├─ [Edge Case Categories](#-edge-case-categories-template--v30) | ~2350 | Days 6-7 test coverage ⭐ V3.0 |
| ├─ [Metrics Validation Commands](#-metrics-validation-commands-template--v30) | ~2400 | Day 5 validation ⭐ V3.0 |
| ├─ [Lessons Learned](#-lessons-learned-template--v30) | ~2450 | Day 10 deliverable ⭐ V3.0 |
| ├─ [Technical Debt](#-technical-debt-template--v30) | ~2500 | Day 10 deliverable ⭐ V3.0 |
| ├─ [Team Handoff Notes](#-team-handoff-notes-template--v30) | ~2550 | Day 10 deliverable ⭐ V3.0 |
| └─ [CRD API Group Standard](#-crd-api-group-standard--v30) | ~2600 | DD-CRD-001 reference ⭐ V3.0 |
| [References](#-references) | ~2700 | ADR/DD documents |

---

## 📂 **Expanded Plan Structure**

This plan is organized into a core document and supporting files for easier navigation.

### **Appendices** (Detailed Reference)

| Document | Purpose |
|----------|---------|
| [APPENDIX_A_EOD_TEMPLATES.md](implementation/appendices/APPENDIX_A_EOD_TEMPLATES.md) | End-of-Day documentation templates |
| [APPENDIX_B_ERROR_HANDLING_PHILOSOPHY.md](implementation/appendices/APPENDIX_B_ERROR_HANDLING_PHILOSOPHY.md) | 5 error categories (A-E), retry logic, circuit breaker |
| [APPENDIX_C_CONFIDENCE_METHODOLOGY.md](implementation/appendices/APPENDIX_C_CONFIDENCE_METHODOLOGY.md) | Confidence calculation formula and scoring |
| [APPENDIX_D_TESTING_PATTERNS.md](implementation/appendices/APPENDIX_D_TESTING_PATTERNS.md) | Table-driven tests, mocks, fixtures |

### **Day-by-Day Breakdown** (Detailed Implementation)

| Document | Days | Focus |
|----------|------|-------|
| [DAY_01_FOUNDATION.md](implementation/days/DAY_01_FOUNDATION.md) | Day 1 | Package structure, reconciler, ValidatingHandler |
| [DAY_02_INVESTIGATING_HANDLER.md](implementation/days/DAY_02_INVESTIGATING_HANDLER.md) | Day 2 | HolmesGPT-API client, InvestigatingHandler |
| [DAY_03_04_ANALYZING_COMPLETION.md](implementation/days/DAY_03_04_ANALYZING_COMPLETION.md) | Days 3-4 | Rego evaluator, AnalyzingHandler, ApprovalContext, **Midpoint** |
| [DAY_05_07_INTEGRATION_TESTING.md](implementation/days/DAY_05_07_INTEGRATION_TESTING.md) | Days 5-7 | Error handling, metrics, KIND integration tests |
| [DAY_08_10_E2E_POLISH.md](implementation/days/DAY_08_10_E2E_POLISH.md) | Days 8-10 | E2E tests, production polish, **Final checkpoint** |

### **EOD Documents** (Created During Implementation)

**Directory**: `implementation/phase0/`

| Document | When Created |
|----------|--------------|
| `01-day1-complete.md` | End of Day 1 |
| `02-day4-midpoint.md` | End of Day 4 (Midpoint) |
| `03-day7-complete.md` | End of Day 7 (Integration Complete) |
| `04-implementation-complete.md` | End of Day 10 (Final) |

---

## 📋 **Service Overview**

### **Service Identity**

| Attribute | Value |
|-----------|-------|
| **Service Name** | AI Analysis |
| **CRD** | `AIAnalysis` |
| **API Group** | `kubernaut.ai/v1alpha1` |
| **Controller** | `AIAnalysisReconciler` |
| **Binary** | `cmd/aianalysis/main.go` |
| **Package** | `pkg/aianalysis/` |
| **Priority** | P0 - HIGH |

### **Port Allocation** (per [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md))

| Port | Purpose | Auth |
|------|---------|------|
| **8081** | Health/Ready (`/healthz`, `/readyz`) | No auth |
| **9090** | Metrics (`/metrics`) | Auth filter |
| **8084** | Service Host (Kind extraPortMappings) | — |

### **V1.0 Scope**

| Feature | Status | Reference |
|---------|--------|-----------|
| HolmesGPT-API Integration | ✅ In Scope | DD-CONTRACT-002 |
| Workflow Selection | ✅ In Scope | DD-WORKFLOW-001 v2.2 |
| Rego Approval Policies | ✅ In Scope | DD-AIANALYSIS-001 |
| Recovery Flow | ✅ In Scope | DD-RECOVERY-002 |
| DetectedLabels (8 fields) | ✅ In Scope | DD-WORKFLOW-001 v2.2 |
| FailedDetections Handling | ✅ In Scope | DD-WORKFLOW-001 v2.1 |
| AIApprovalRequest CRD | ❌ V1.1 | ADR-040 |
| Multi-provider LLM | ❌ V2.0 | — |

### **Existing Infrastructure** (Verified)

| Component | Location | Status |
|-----------|----------|--------|
| CRD Types | `api/aianalysis/v1alpha1/aianalysis_types.go` | ✅ Exists |
| Go Client | `pkg/clients/holmesgpt/` (18 files, ogen) | ✅ Generated |
| Shared Types | `pkg/shared/types/enrichment.go` | ✅ v2.2 (no podSecurityLevel) |
| CRD Manifest | `config/crd/bases/aianalysis.kubernaut.io_aianalyses.yaml` | ✅ Exists |
| Specifications | `docs/services/crd-controllers/02-aianalysis/` | ✅ v2.6 |

---

## ✅ **Prerequisites Checklist**

Before starting Day 1, ensure:

### **Universal Standards (ALL services)**
- [x] DD-004: RFC 7807 Error Responses (**MANDATORY**)
- [x] DD-005: Observability Standards (**MANDATORY** - `logr.Logger`)
- [x] DD-007: Kubernetes-Aware Graceful Shutdown (**MANDATORY**)
- [x] DD-014: Binary Version Logging (**MANDATORY**)
- [x] ADR-015: Alert-to-Signal Naming Migration (**MANDATORY**)

### **CRD Controller Standards**
- [x] DD-006: Controller Scaffolding (templates and patterns)
- [x] DD-013: K8s Client Initialization Standard (shared `pkg/k8sutil`)
- [x] ADR-004: Fake K8s Client (**MANDATORY** for unit tests)
- [x] DD-CRD-001: API Group Domain Selection (`kubernaut.io`)

### **Service-Specific Standards**
- [x] DD-WORKFLOW-001 v2.2: DetectedLabels schema (8 fields, no podSecurityLevel)
- [x] DD-RECOVERY-002: Direct AIAnalysis recovery flow
- [x] DD-RECOVERY-003: Recovery prompt with K8s reason codes
- [x] DD-CONTRACT-002: Service integration contracts
- [x] DD-AIANALYSIS-001: Rego policy loading strategy

### **Audit Standards** (P0 Service)
- [x] DD-AUDIT-003: Service Audit Trace Requirements
- [x] ADR-032: Data Access Layer Isolation (use Data Storage API)
- [x] ADR-034: Unified Audit Table Design
- [x] ADR-038: Async Buffered Audit Ingestion

### **Testing Standards**
- [x] DD-TEST-001: Port Allocation Strategy (8084 for AIAnalysis)

### **Existing Infrastructure Verified**
- [x] CRD types in `api/aianalysis/v1alpha1/` (confirmed exists)
- [x] Go client in `pkg/clients/holmesgpt/` (confirmed 18 files)
- [x] Shared types in `pkg/shared/types/enrichment.go` (confirmed v2.2)
- [x] Specifications in `docs/services/crd-controllers/02-aianalysis/` (confirmed v2.6)

### **Cross-Team Dependencies**
- [x] SignalProcessing team validation complete
- [x] HolmesGPT-API team validation complete
- [x] Data Storage team validation complete
- [x] RO team validation complete

---

## 🤝 **Cross-Team Validation** ⭐ V3.0

**Validation Status**: ✅ VALIDATED - All Dependencies Confirmed

### **Cross-Team Validation Evidence Table**

| Team | Topic | Status | Evidence Document | Resolution |
|------|-------|--------|-------------------|------------|
| **SignalProcessing** | EnrichmentResults path, DetectedLabels schema | ✅ Complete | [AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md](../../../handoff/AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md) | Path: `spec.analysisRequest.signalContext.enrichmentResults`; Schema v2.2 |
| **SignalProcessing** | FailedDetections schema | ✅ Complete | [DD-WORKFLOW-001 v2.1](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | `[]string` with enum validation |
| **SignalProcessing** | PodSecurityLevel removal | ✅ Acknowledged | [NOTICE_PODSECURITYLEVEL_REMOVED.md](../../../handoff/NOTICE_PODSECURITYLEVEL_REMOVED.md) | Field removed from DetectedLabels |
| **HolmesGPT-API** | Investigation endpoint | ✅ Complete | [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../../../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) | `/api/v1/incident/analyze` (port 8080) |
| **HolmesGPT-API** | Response schema | ✅ Complete | [RESPONSE_TO_AIANALYSIS_TEAM.md](../../../handoff/RESPONSE_TO_AIANALYSIS_TEAM.md) | `IncidentResponse` with `target_in_owner_chain`, `warnings` |
| **HolmesGPT-API** | Go client generation | ✅ Complete | `pkg/clients/holmesgpt/` | `ogen` from OpenAPI 3.1.0 |
| **HolmesGPT-API** | MockLLMServer | ✅ Available | `holmesgpt-api/tests/mock_llm_server.py` | For integration tests |
| **Data Storage** | Audit events schema | ✅ Complete | [QUESTIONS_FOR_DATA_STORAGE_TEAM.md](../../../handoff/QUESTIONS_FOR_DATA_STORAGE_TEAM.md) | AIAnalysis audit event type defined |
| **Data Storage** | FailedDetections impact | ✅ Acknowledged | [QUESTIONS_FOR_DATA_STORAGE_TEAM.md](../../../handoff/QUESTIONS_FOR_DATA_STORAGE_TEAM.md) | Workflow filtering handles `failedDetections` |
| **RO** | Contract alignment | ✅ Complete | [AIANALYSIS_TO_RO_TEAM.md](../../../handoff/AIANALYSIS_TO_RO_TEAM.md) | Environment/Priority as free-text |
| **RO** | Shared types import | ✅ Complete | [RO_TO_AIANALYSIS_CONTRACT_ALIGNMENT.md](../../../handoff/RO_TO_AIANALYSIS_CONTRACT_ALIGNMENT.md) | `pkg/shared/types/enrichment.go` |

### **Pre-Implementation Validation Gate**

> **✅ PASSED**: All cross-team validations complete. Ready for Day 1.

- [x] All upstream data contracts validated (SignalProcessing → AIAnalysis)
- [x] All downstream data contracts validated (AIAnalysis → HolmesGPT-API)
- [x] Shared type definitions aligned (`pkg/shared/types/enrichment.go`)
- [x] Naming conventions agreed (snake_case in JSON, CamelCase in Go)
- [x] Field paths confirmed (`spec.analysisRequest.signalContext.enrichmentResults`)
- [x] Integration points documented with examples

**Confidence Impact**: 100% achievable (all contracts verified)

---

## 🔍 **Integration Test Environment Decision**

### **Decision: 🔴 KIND Required**

| Question | Answer |
|----------|--------|
| Writes to Kubernetes? | ✅ YES - Creates/updates AIAnalysis CRD status |
| Needs RBAC? | ✅ YES - Controller needs CRD permissions |
| External dependencies? | ✅ YES - HolmesGPT-API, Data Storage |
| **Recommended Environment** | **🔴 KIND** |

### **KIND Requirements**

| Requirement | Implementation |
|-------------|----------------|
| KIND cluster | `make kind-create` |
| CRD installation | `make install` |
| HolmesGPT-API | Deploy in KIND with MockLLMServer |
| Data Storage | Deploy in KIND with PostgreSQL |
| Rego ConfigMap | Create in `kubernaut-system` namespace |

### **MockLLMServer Pattern** (from HolmesGPT-API)

AIAnalysis integration tests will use the same MockLLMServer pattern that HolmesGPT-API uses:

```go
// test/integration/aianalysis/setup_test.go
package aianalysis

import (
    "context"
    "os"
    "os/exec"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var (
    mockLLMProcess *exec.Cmd
    mockLLMURL     string
)

var _ = BeforeSuite(func() {
    ctx := context.Background()

    // Start MockLLMServer (Python process)
    mockLLMProcess = exec.CommandContext(ctx,
        "python3", "../../holmesgpt-api/tests/mock_llm_server.py",
        "--port", "11434",
    )
    Expect(mockLLMProcess.Start()).To(Succeed())

    // Wait for mock server to be ready
    mockLLMURL = "http://localhost:11434"
    Eventually(func() error {
        _, err := http.Get(mockLLMURL + "/health")
        return err
    }, 10*time.Second, 100*time.Millisecond).Should(Succeed())

    // Configure HolmesGPT-API to use mock LLM
    os.Setenv("LLM_ENDPOINT", mockLLMURL)
    os.Setenv("LLM_MODEL", "mock-model")
})

var _ = AfterSuite(func() {
    if mockLLMProcess != nil {
        mockLLMProcess.Process.Kill()
    }
})
```

### **Port Allocation** (per DD-TEST-001)

#### **AIAnalysis Controller Ports**

| Port Type | Port | Purpose |
|-----------|------|---------|
| **Health/Ready** | 8081 | `/healthz`, `/readyz` (no auth) |
| **Metrics** | 9090 | `/metrics` (auth filter) |
| **Host Port** | 8084 | Kind extraPortMappings |
| **NodePort (API)** | 30084 | E2E tests API access |
| **NodePort (Metrics)** | 30184 | E2E Prometheus scraping |
| **NodePort (Health)** | 30284 | E2E health checks |

#### **Kind Configuration**

**File**: `test/infrastructure/kind-aianalysis-config.yaml`

```bash
# Create KIND cluster for AIAnalysis E2E tests
kind create cluster --name aianalysis-e2e \
    --config test/infrastructure/kind-aianalysis-config.yaml

# Verify port mappings
docker port aianalysis-e2e-control-plane
# Expected:
# 30084/tcp -> 0.0.0.0:8084  (API)
# 30184/tcp -> 0.0.0.0:9184  (Metrics)
# 30284/tcp -> 0.0.0.0:8184  (Health)
```

#### **NodePort Service Template**

```yaml
# deploy/manifests/aianalysis-nodeport-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: aianalysis-controller-nodeport
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: aianalysis-controller
  ports:
  - name: health
    port: 8081
    targetPort: 8081
    nodePort: 30284
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30184
```

#### **Dependency Ports** (Integration & E2E)

| Service | Integration Tier | E2E Tier (Kind NodePort) |
|---------|------------------|--------------------------|
| **AIAnalysis** | — | Host: 8084, NodePort: 30084 |
| **HolmesGPT-API** | Podman: 18080 | In-cluster: 8080 |
| **Data Storage** | Podman: 18090 | NodePort: 30081 |
| **PostgreSQL** | Podman: 15433 | In-cluster: 5432 |
| **MockLLMServer** | Port: 11434 | Port: 11434 |

#### **E2E Test URL Configuration**

```go
// test/e2e/aianalysis/config.go
const (
    // AIAnalysis accessible via Kind NodePort
    AIAnalysisHealthURL  = "http://localhost:8184/healthz"
    AIAnalysisMetricsURL = "http://localhost:9184/metrics"

    // HolmesGPT-API in-cluster (accessed via kubectl port-forward or NodePort)
    HolmesGPTAPIURL = "http://localhost:8080"

    // Data Storage via NodePort
    DataStorageURL = "http://localhost:8081"
)
```

---

## 🎯 **Pre-Implementation Design Decisions**

### **DD-1: Reconciliation Trigger Strategy**

| Question | Should reconciliation be triggered by all field changes or specific fields only? |
|----------|----------------------------------------------------------------------------------|
| **Decision** | **Option B**: Specific fields only (spec changes, not status) |
| **Rationale** | Prevents reconciliation loops from status updates |
| **Implementation** | Use `GenerationChangedPredicate` in controller builder |

### **DD-2: HolmesGPT-API Call Timing**

| Question | Should HolmesGPT-API calls be synchronous or use a job queue? |
|----------|---------------------------------------------------------------|
| **Decision** | **Option A**: Synchronous calls in reconciliation loop |
| **Rationale** | V1.0 simplicity; job queue adds complexity; CRD status tracks progress |
| **Implementation** | Call HolmesGPT-API directly in Investigating phase with 30s timeout |

### **DD-3: Rego Policy Evaluation Failure Behavior**

| Question | What happens when Rego policy evaluation fails? |
|----------|------------------------------------------------|
| **Decision** | **Option B**: Default to `approvalRequired=true` (safe default) |
| **Rationale** | Fail-safe behavior; operators can manually approve if policy fails |
| **Implementation** | Catch Rego errors, set `approvalRequired=true`, `approvalReason="Policy evaluation failed"` |

### **DD-4: Recovery Context Handling**

| Question | How should AIAnalysis handle `PreviousExecutions` slice? |
|----------|----------------------------------------------------------|
| **Decision** | **Option A**: Pass all previous executions to HolmesGPT-API |
| **Rationale** | LLM benefits from full history to avoid repeating failed approaches |
| **Implementation** | Include full `PreviousExecutions` slice in recovery analysis request |

### **DD-5: FailedDetections in Rego Input**

| Question | How should `FailedDetections` affect Rego approval policies? |
|----------|--------------------------------------------------------------|
| **Decision** | **Option A**: Include in Rego input, let policies decide |
| **Rationale** | Flexible; some policies may require approval when detections fail |
| **Implementation** | Add `failed_detections: []string` to Rego input schema |

### **Pre-Implementation Checklist**

- [x] All ambiguous requirements have documented decisions (DD-1 to DD-5)
- [x] Each decision has clear rationale
- [x] Implementation impact is documented
- [x] Decisions approved by stakeholder (user confirmed)

---

## ⚠️ **Risk Assessment Matrix**

### **Identified Risks**

| # | Risk | Probability | Impact | Mitigation | Owner |
|---|------|-------------|--------|------------|-------|
| 1 | HolmesGPT-API unavailable during investigation | Medium | High | Circuit breaker + retry with backoff | Dev |
| 2 | Rego policy syntax error in ConfigMap | Medium | Medium | Validation on load + fallback to default | Dev |
| 3 | Rego policy hot-reload race condition | Low | High | Mutex protection + version tracking | Dev |
| 4 | Data Storage audit write failure | Low | Medium | Async buffered store + retry | Dev |
| 5 | CRD status update conflict | Medium | Low | Optimistic locking + retry | Dev |
| 6 | Large enrichment payload causes timeout | Low | Medium | Payload size limits + chunking | Dev |

### **Risk Mitigation Status Tracking**

| Risk # | Action Required | Day | Status |
|--------|-----------------|-----|--------|
| 1 | Implement HolmesGPT-API client with circuit breaker | Day 3 | ⬜ Pending |
| 2 | Add Rego policy validation on ConfigMap load | Day 4 | ⬜ Pending |
| 3 | Add `sync.RWMutex` to Rego engine + version hash | Day 4 | ⬜ Pending |
| 4 | Use `audit.NewBufferedStore()` with retry | Day 5 | ⬜ Pending |
| 5 | Add optimistic locking in status updates | Day 2 | ⬜ Pending |
| 6 | Add enrichment size validation (max 1MB) | Day 2 | ⬜ Pending |

---

## 📋 **Files Affected Section**

### **New Files** (to be created)

| File | Purpose | Day |
|------|---------|-----|
| `cmd/aianalysis/main.go` | Service entry point | Day 1 |
| `internal/controller/aianalysis/aianalysis_controller.go` | Main reconciler | Day 2 |
| `internal/controller/aianalysis/metrics.go` | Prometheus metrics | Day 5 |
| `internal/controller/aianalysis/audit.go` | Audit client | Day 5 |
| `pkg/aianalysis/rego/engine.go` | OPA policy engine | Day 4 |
| `pkg/aianalysis/rego/input.go` | Rego input schema | Day 4 |
| `pkg/aianalysis/holmesgpt/client.go` | HolmesGPT-API wrapper | Day 3 |
| `pkg/aianalysis/holmesgpt/types.go` | Request/response types | Day 3 |
| `pkg/aianalysis/phases/pending.go` | Pending phase handler | Day 2 |
| `pkg/aianalysis/phases/validating.go` | Validating phase handler | Day 2 |
| `pkg/aianalysis/phases/investigating.go` | Investigating phase handler | Day 3 |
| `pkg/aianalysis/phases/terminal.go` | Ready/Failed phase handlers | Day 3 |
| `test/unit/aianalysis/controller_test.go` | Controller unit tests | Day 6 |
| `test/unit/aianalysis/rego_test.go` | Rego engine unit tests | Day 6 |
| `test/unit/aianalysis/holmesgpt_test.go` | HolmesGPT client unit tests | Day 6 |
| `test/integration/aianalysis/reconciler_test.go` | Reconciler integration tests | Day 7 |
| `test/integration/aianalysis/rego_integration_test.go` | Rego policy integration tests | Day 7 |
| `test/e2e/aianalysis/workflow_selection_test.go` | E2E workflow selection | Day 8 |
| `deploy/aianalysis/policies/approval.rego` | Approval policy example | Day 4 |

### **Modified Files** (existing files to update)

| File | Changes | Day |
|------|---------|-----|
| `api/aianalysis/v1alpha1/aianalysis_types.go` | Verify fields match spec | Day 1 |
| `Makefile` | Add aianalysis targets | Day 1 |
| `PROJECT` | Register AIAnalysis controller | Day 1 |

### **Verified Existing Files** (no changes needed)

| File | Status | Verified |
|------|--------|----------|
| `pkg/shared/types/enrichment.go` | ✅ Current (v2.2) | 2025-12-03 |
| `pkg/clients/holmesgpt/*.go` | ✅ Generated (ogen) | 2025-12-03 |
| `config/crd/bases/aianalysis.kubernaut.io_aianalyses.yaml` | ✅ Exists | 2025-12-03 |

---

## 🔄 **Enhancement Application Checklist** ⭐ V3.0

**Purpose**: Track which patterns and enhancements have been applied to which implementation days.

| Enhancement | Applied To | Status | Notes |
|-------------|------------|--------|-------|
| **Error Handling Philosophy** | Days 2-5 | ⬜ Pending | Apply error categories A-E |
| **Service-Specific Error Categories** | Day 5 EOD | ⬜ Pending | Document 5 error categories |
| **Retry with Exponential Backoff** | Day 3 | ⬜ Pending | HolmesGPT-API calls |
| **Circuit Breaker Pattern** | Day 3 | ⬜ Pending | External dependencies |
| **Graceful Degradation** | Day 4 | ⬜ Pending | Rego policy fallback |
| **Metrics Cardinality Audit** | Day 5 | ⬜ Pending | Per DD-005 |
| **Integration Test Anti-Flaky** | Day 7 | ⬜ Pending | Eventually() pattern |
| **Production Runbooks** | Day 9 | ⬜ Pending | 2-3 runbooks |

### **Day-by-Day Enhancement Application**

**Day 2** (Core Logic Start):
- [ ] Apply error classification for phase handlers (Category A, D)

**Day 3** (HolmesGPT-API Integration):
- [ ] Implement retry with exponential backoff (Category B)
- [ ] Add circuit breaker for HolmesGPT-API (Category B)

**Day 4** (Rego Engine):
- [ ] Add graceful degradation for Rego failures (Category E)
- [ ] Add auth error handling if applicable (Category C)

**Day 5** (Metrics & Audit):
- [ ] Add optimistic locking for status updates (Category D)
- [ ] Document all 5 error categories in Error Handling Philosophy

**Day 7** (Integration Tests):
- [ ] Apply anti-flaky patterns (Eventually(), 30s timeout)
- [ ] Test all edge case categories

**Day 9** (Documentation):
- [ ] Create 2-3 production runbooks
- [ ] Add Prometheus metrics for runbook automation

---

## 📅 **Timeline Overview**

| Phase | Days | Focus | Key Deliverables |
|-------|------|-------|------------------|
| **Foundation** | 1 | Types, interfaces, verify existing | Package structure, main.go |
| **Core Logic** | 2-5 | Reconciler, phases, integrations | All components implemented |
| **Testing** | 6-8 | Unit, Integration, E2E | 70%+ coverage |
| **Documentation** | 9 | API docs, runbooks | Production docs |
| **Production Readiness** | 10 | Checklist, handoff | Ready for deployment |

**Total**: 10 days (2 calendar weeks)

---

## 📋 **Day-by-Day Breakdown**

### **Day 1: Foundation (8h)**

#### ANALYSIS Phase (1h)

**Search existing patterns:**
```bash
codebase_search "AIAnalysis reconciler existing implementations"
codebase_search "Rego policy evaluation in controllers"
grep -r "holmesgpt" pkg/ internal/ --include="*.go"
```

**Verify existing infrastructure:**
- [x] CRD types in `api/aianalysis/v1alpha1/` (✅ verified)
- [x] Go client in `pkg/clients/holmesgpt/` (✅ verified)
- [x] Shared types in `pkg/shared/types/enrichment.go` (✅ verified v2.2)

**Map business requirements:**
- 31 V1.0 BRs mapped (per [BR_MAPPING.md](./BR_MAPPING.md))
- Core AI Analysis: 15 BRs
- Approval & Policy: 5 BRs
- Recovery Flow: 4 BRs

#### PLAN Phase (1h)

**TDD Strategy:**
1. RED: Write controller tests (fail - no controller)
2. GREEN: Implement minimal reconciler
3. REFACTOR: Add error handling, logging

**Integration points:**
- Main app: `cmd/aianalysis/main.go`
- Business logic: `pkg/aianalysis/`
- Tests: `test/unit/aianalysis/`, `test/integration/aianalysis/`, `test/e2e/aianalysis/`

---

### **Test Scenarios by Component** (Define Upfront per TDD) ⭐ V3.0

> **IMPORTANT**: Define concrete test scenarios BEFORE implementation. This aligns with TDD - know what you're testing before writing code.

#### **Reconciler** (`test/unit/aianalysis/controller_test.go`)

**Happy Path (6 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| R-HP-01 | Full happy path (Pending → Completed) | Valid AIAnalysis CR | Status.Phase="Completed", workflow selected |
| R-HP-02 | Phase transition Pending→Validating | New CR | Status.Phase="Validating" |
| R-HP-03 | Phase transition Validating→Investigating | Validated CR | Status.Phase="Investigating" |
| R-HP-04 | Phase transition Investigating→Completed | HolmesGPT response | Status.Phase="Completed" |
| R-HP-05 | Finalizer lifecycle | New CR, then delete | Finalizer added, then removed after cleanup |
| R-HP-06 | Recovery analysis path | IsRecoveryAttempt=true | PreviousExecutions passed to HolmesGPT |

**Edge Cases (5 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| R-EC-01 | CR deleted during processing | CR deleted mid-reconcile | Graceful termination |
| R-EC-02 | Already completed CR | CR with Status=Completed | No-op, no requeue |
| R-EC-03 | Concurrent reconciles | Same CR reconciled twice | Only one succeeds (optimistic locking) |
| R-EC-04 | FailedDetections in input | FailedDetections=["pdbProtected"] | Passed to HolmesGPT, Rego aware |
| R-EC-05 | Empty EnrichmentResults | No DetectedLabels | Validation passes, minimal context to HolmesGPT |

**Error Handling (5 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| R-ER-01 | HolmesGPT-API unavailable | Valid CR, API down | Requeue with exponential backoff |
| R-ER-02 | HolmesGPT-API timeout | Valid CR, slow response | Timeout error, requeue |
| R-ER-03 | Validation failure | Invalid CR (missing fingerprint) | Status=Failed, error in conditions |
| R-ER-04 | Status update conflict | CR, concurrent update | Retries with fresh version |
| R-ER-05 | Invalid FailedDetections field | FailedDetections=["invalidField"] | Validation error |

---

#### **Rego Engine** (`test/unit/aianalysis/rego_test.go`)

**Happy Path (4 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| E-HP-01 | Policy loads from ConfigMap | Valid Rego policy | Engine initialized, version hash set |
| E-HP-02 | Approval required (low confidence) | Confidence=0.6 | RequiresApproval=true |
| E-HP-03 | Approval not required (high confidence) | Confidence=0.95, production | RequiresApproval=false |
| E-HP-04 | Policy hot-reload | ConfigMap updated | New policy loaded, version updated |

**Edge Cases (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| E-EC-01 | Detection failure in input | FailedDetections=["pdbProtected"] | Policy can access failed_detections |
| E-EC-02 | TargetInOwnerChain=false | Production + not in chain | RequiresApproval=true |
| E-EC-03 | Warnings from HolmesGPT | warnings=["..."] | Policy can evaluate warnings |

**Error Handling (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| E-ER-01 | Invalid Rego syntax | Malformed policy | Default to RequiresApproval=true |
| E-ER-02 | ConfigMap not found | Missing ConfigMap | Error, cannot initialize |
| E-ER-03 | Policy timeout | Slow policy | Timeout, default to RequiresApproval=true |

---

#### **HolmesGPT Client** (`test/unit/aianalysis/holmesgpt_test.go`)

**Happy Path (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| H-HP-01 | Successful investigation | Valid request | IncidentResponse with workflow |
| H-HP-02 | Recovery analysis | IsRecoveryAttempt=true | Recovery response with new workflow |
| H-HP-03 | Response parsing | Full response | All fields parsed (TargetInOwnerChain, Warnings) |

**Error Handling (4 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| H-ER-01 | Connection timeout | Valid request, slow server | Timeout error after 30s |
| H-ER-02 | Rate limiting (429) | Valid request | Retry with backoff |
| H-ER-03 | Service unavailable (503) | Valid request | Retry with backoff |
| H-ER-04 | Malformed response | Invalid JSON | Error returned |

---

### **Test Count Summary**

| Component | Happy Path | Edge Cases | Error Handling | **Total** |
|-----------|------------|------------|----------------|-----------|
| Reconciler | 6 | 5 | 5 | **16** |
| Rego Engine | 4 | 3 | 3 | **10** |
| HolmesGPT Client | 3 | 0 | 4 | **7** |
| Phase Handlers | 4 | 2 | 2 | **8** |
| **Unit Total** | **17** | **10** | **14** | **41** |

| Test Type | Count | Target Coverage |
|-----------|-------|-----------------|
| **Unit Tests** | 41 | 70%+ |
| **Integration Tests** | ~15 | >50% (CRD coordination) |
| **E2E Tests** | ~5 | <10% (critical paths) |
| **Total** | **~61** | Defense-in-depth |

> **Note**: Test counts are estimates based on BR mapping. Realistic range for CRD controller: 40-60 unit, 15-25 integration.

---

**Package Structure:**
```
cmd/aianalysis/
├── main.go                    # Entry point
internal/controller/aianalysis/
├── aianalysis_controller.go   # Main reconciler
├── metrics.go                 # Prometheus metrics
├── audit.go                   # Audit client
pkg/aianalysis/
├── phases/                    # Phase handlers
│   ├── pending.go
│   ├── validating.go
│   ├── investigating.go
│   └── terminal.go
├── rego/                      # OPA policy engine
│   ├── engine.go
│   └── input.go
└── holmesgpt/                 # HolmesGPT-API wrapper
    ├── client.go
    └── types.go
```

#### DO Phase (5h)

**Step 1: Create service directory (30min)**
```bash
mkdir -p cmd/aianalysis
mkdir -p internal/controller/aianalysis
mkdir -p pkg/aianalysis/{phases,rego,holmesgpt}
mkdir -p test/unit/aianalysis
mkdir -p test/integration/aianalysis
mkdir -p test/e2e/aianalysis
mkdir -p deploy/aianalysis/policies
```

**Step 2: Create main.go (1h)**

```go
// cmd/aianalysis/main.go
package main

import (
    "flag"
    "os"

    "k8s.io/apimachinery/pkg/runtime"
    utilruntime "k8s.io/apimachinery/pkg/util/runtime"
    clientgoscheme "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/healthz"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/internal/controller/aianalysis"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")

    // Build information (set by ldflags)
    Version   = "dev"
    GitCommit = "unknown"
    BuildTime = "unknown"
)

func init() {
    utilruntime.Must(clientgoscheme.AddToScheme(scheme))
    utilruntime.Must(aianalysisv1.AddToScheme(scheme))
}

func main() {
    var metricsAddr string
    var enableLeaderElection bool
    var probeAddr string

    flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "The address the metric endpoint binds to.")
    flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
    flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election.")

    opts := zap.Options{Development: true}
    opts.BindFlags(flag.CommandLine)
    flag.Parse()

    ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

    // DD-014: Log version information at startup
    setupLog.Info("Starting AI Analysis Controller",
        "version", Version,
        "gitCommit", GitCommit,
        "buildTime", BuildTime,
    )

    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme:                 scheme,
        MetricsBindAddress:     metricsAddr,
        HealthProbeBindAddress: probeAddr,
        LeaderElection:         enableLeaderElection,
        LeaderElectionID:       "aianalysis.kubernaut.io",
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    if err = (&aianalysis.AIAnalysisReconciler{
        Client:   mgr.GetClient(),
        Scheme:   mgr.GetScheme(),
        Recorder: mgr.GetEventRecorderFor("aianalysis-controller"),
        Log:      ctrl.Log.WithName("controllers").WithName("AIAnalysis"),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "AIAnalysis")
        os.Exit(1)
    }

    if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
        setupLog.Error(err, "unable to set up health check")
        os.Exit(1)
    }
    if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
        setupLog.Error(err, "unable to set up ready check")
        os.Exit(1)
    }

    setupLog.Info("starting manager")
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        setupLog.Error(err, "problem running manager")
        os.Exit(1)
    }
}
```

**Step 3: Create minimal reconciler skeleton (2h)**

```go
// internal/controller/aianalysis/aianalysis_controller.go
package aianalysis

import (
    "context"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/predicate"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

const (
    FinalizerName = "aianalysis.kubernaut.ai/finalizer"
)

// AIAnalysisReconciler reconciles a AIAnalysis object
type AIAnalysisReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
    Log      logr.Logger
}

// +kubebuilder:rbac:groups=kubernaut.io,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubernaut.io,resources=aianalyses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubernaut.io,resources=aianalyses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := r.Log.WithValues("aianalysis", req.NamespacedName)
    log.Info("Reconciling AIAnalysis")

    // Fetch the AIAnalysis instance
    analysis := &aianalysisv1.AIAnalysis{}
    if err := r.Get(ctx, req.NamespacedName, analysis); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion
    if !analysis.DeletionTimestamp.IsZero() {
        return r.handleDeletion(ctx, analysis)
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(analysis, FinalizerName) {
        controllerutil.AddFinalizer(analysis, FinalizerName)
        if err := r.Update(ctx, analysis); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{Requeue: true}, nil
    }

    // Phase-based reconciliation
    switch analysis.Status.Phase {
    case "", "Pending":
        return r.reconcilePending(ctx, analysis)
    case "Validating":
        return r.reconcileValidating(ctx, analysis)
    case "Investigating":
        return r.reconcileInvestigating(ctx, analysis)
    case "Completed", "Failed":
        // Terminal states - no action needed
        return ctrl.Result{}, nil
    default:
        log.Info("Unknown phase", "phase", analysis.Status.Phase)
        return ctrl.Result{}, nil
    }
}

func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&aianalysisv1.AIAnalysis{}).
        WithEventFilter(predicate.GenerationChangedPredicate{}). // DD-1: Only spec changes
        Complete(r)
}

// Phase handlers (stubs for Day 1)
func (r *AIAnalysisReconciler) reconcilePending(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    // TODO: Implement in Day 2
    return ctrl.Result{}, nil
}

func (r *AIAnalysisReconciler) reconcileValidating(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    // TODO: Implement in Day 2
    return ctrl.Result{}, nil
}

func (r *AIAnalysisReconciler) reconcileInvestigating(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    // TODO: Implement in Day 3
    return ctrl.Result{}, nil
}

func (r *AIAnalysisReconciler) handleDeletion(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    log := r.Log.WithValues("aianalysis", analysis.Name)
    log.Info("Handling deletion")

    // Cleanup logic here (audit trail, etc.)

    // Remove finalizer
    controllerutil.RemoveFinalizer(analysis, FinalizerName)
    if err := r.Update(ctx, analysis); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

**Step 4: Update Makefile (30min)**

```makefile
# Add to Makefile
.PHONY: run-aianalysis
run-aianalysis: ## Run AIAnalysis controller locally
	go run ./cmd/aianalysis/main.go

.PHONY: build-aianalysis
build-aianalysis: ## Build AIAnalysis binary
	go build -o bin/aianalysis ./cmd/aianalysis

.PHONY: test-aianalysis
test-aianalysis: ## Run AIAnalysis tests
	go test -v -p 4 ./test/unit/aianalysis/...
	go test -v -p 4 ./test/integration/aianalysis/...
```

#### CHECK Phase (1h)

**Validation:**
```bash
# Build should succeed
go build ./cmd/aianalysis/...

# Types should be accessible
go run ./cmd/aianalysis/main.go --help
```

**EOD Day 1 Checklist:**
- [ ] `cmd/aianalysis/main.go` created and compiles
- [ ] Controller skeleton in `internal/controller/aianalysis/`
- [ ] Package directories created
- [ ] Makefile targets added
- [ ] CRD types verified (existing)
- [ ] Go client verified (existing)

**EOD Documentation** ⭐ V3.0:
- [ ] Create `docs/services/crd-controllers/02-aianalysis/implementation/phase0/01-day1-complete.md`
- [ ] Document architecture decisions made
- [ ] Note any deviations from plan

---

### **Day 2: Phase Handlers - Pending & Validating (8h)**

#### ANALYSIS Phase (30min)

**Review phase flow:**
```
Pending → Validating → Investigating → Completed/Failed
```

**Map to BRs:**
- Pending: BR-AI-001 (CRD created event)
- Validating: BR-AI-020, BR-AI-021 (input validation)

#### DO Phase (6h)

**Step 1: Implement Pending phase (1h)**

```go
// pkg/aianalysis/phases/pending.go
package phases

import (
    "context"
    "time"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

type PendingHandler struct{}

func NewPendingHandler() *PendingHandler {
    return &PendingHandler{}
}

func (h *PendingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
    now := metav1.NewTime(time.Now())

    // Set initial status
    analysis.Status.Phase = "Validating"
    analysis.Status.StartedAt = &now
    analysis.Status.Message = "AIAnalysis created, starting validation"

    return nil
}
```

**Step 2: Implement Validating phase (3h)**

```go
// pkg/aianalysis/phases/validating.go
package phases

import (
    "context"
    "fmt"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

type ValidatingHandler struct{}

func NewValidatingHandler() *ValidatingHandler {
    return &ValidatingHandler{}
}

func (h *ValidatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (*ValidationResult, error) {
    result := &ValidationResult{
        Valid:  true,
        Errors: []string{},
    }

    // Validate SignalContext
    sc := analysis.Spec.AnalysisRequest.SignalContext

    if sc.Fingerprint == "" {
        result.AddError("signalContext.fingerprint is required")
    }

    if sc.SignalType == "" {
        result.AddError("signalContext.signalType is required")
    }

    if sc.Environment == "" {
        result.AddError("signalContext.environment is required")
    }

    if sc.BusinessPriority == "" {
        result.AddError("signalContext.businessPriority is required")
    }

    // Validate TargetResource
    tr := sc.TargetResource
    if tr.Kind == "" || tr.Name == "" || tr.Namespace == "" {
        result.AddError("signalContext.targetResource (kind, name, namespace) is required")
    }

    // Validate EnrichmentResults
    er := sc.EnrichmentResults
    if er.KubernetesContext == nil && er.DetectedLabels == nil {
        result.AddError("signalContext.enrichmentResults must have kubernetesContext or detectedLabels")
    }

    // Validate OwnerChain (can be empty for orphan resources)
    // No validation needed - empty is valid

    // Validate DetectedLabels.FailedDetections (DD-WORKFLOW-001 v2.1)
    if er.DetectedLabels != nil {
        for _, field := range er.DetectedLabels.FailedDetections {
            if !isValidDetectedLabelField(field) {
                result.AddError(fmt.Sprintf("invalid field in failedDetections: %s", field))
            }
        }
    }

    // Update status
    if result.Valid {
        analysis.Status.Phase = "Investigating"
        analysis.Status.Message = "Validation passed, starting investigation"
    } else {
        analysis.Status.Phase = "Failed"
        analysis.Status.Reason = "ValidationFailed"
        analysis.Status.Message = fmt.Sprintf("Validation failed: %v", result.Errors)
    }

    return result, nil
}

type ValidationResult struct {
    Valid  bool
    Errors []string
}

func (r *ValidationResult) AddError(msg string) {
    r.Valid = false
    r.Errors = append(r.Errors, msg)
}

// Valid DetectedLabels fields (8 fields per DD-WORKFLOW-001 v2.2)
var validDetectedLabelFields = map[string]bool{
    "gitOpsManaged":    true,
    "pdbProtected":     true,
    "hpaEnabled":       true,
    "stateful":         true,
    "helmManaged":      true,
    "networkIsolated":  true,
    "serviceMesh":      true,
    // NOTE: podSecurityLevel REMOVED in v2.2
}

func isValidDetectedLabelField(field string) bool {
    return validDetectedLabelFields[field]
}
```

**Step 3: Update controller to use phase handlers (2h)**

```go
// internal/controller/aianalysis/aianalysis_controller.go (update)

import (
    // ... existing imports ...
    "github.com/jordigilh/kubernaut/pkg/aianalysis/phases"
)

type AIAnalysisReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Recorder          record.EventRecorder
    Log               logr.Logger
    PendingHandler    *phases.PendingHandler
    ValidatingHandler *phases.ValidatingHandler
}

func (r *AIAnalysisReconciler) reconcilePending(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    log := r.Log.WithValues("phase", "Pending")
    log.Info("Processing Pending phase")

    if err := r.PendingHandler.Handle(ctx, analysis); err != nil {
        return ctrl.Result{}, err
    }

    r.Recorder.Event(analysis, "Normal", "AIAnalysisCreated", "AIAnalysis processing started")

    if err := r.Status().Update(ctx, analysis); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{Requeue: true}, nil
}

func (r *AIAnalysisReconciler) reconcileValidating(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    log := r.Log.WithValues("phase", "Validating")
    log.Info("Processing Validating phase")

    result, err := r.ValidatingHandler.Handle(ctx, analysis)
    if err != nil {
        return ctrl.Result{}, err
    }

    if result.Valid {
        r.Recorder.Event(analysis, "Normal", "ValidationPassed", "Input validation successful")
    } else {
        r.Recorder.Event(analysis, "Warning", "ValidationFailed", analysis.Status.Message)
    }

    if err := r.Status().Update(ctx, analysis); err != nil {
        return ctrl.Result{}, err
    }

    if result.Valid {
        return ctrl.Result{Requeue: true}, nil
    }
    return ctrl.Result{}, nil // Terminal state
}
```

#### CHECK Phase (1.5h)

**Unit test for validation:**
```go
// test/unit/aianalysis/validating_test.go
package aianalysis  // Same package for unit tests (white-box testing per 03-testing-strategy.mdc)

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/phases"
    sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("BR-AI-020: Validating Phase Handler", func() {
    var handler *phases.ValidatingHandler

    BeforeEach(func() {
        handler = phases.NewValidatingHandler()
    })

    // ⭐ V3.0: Use DescribeTable for multiple similar scenarios
    DescribeTable("FailedDetections field validation",
        func(failedDetections []string, expectValid bool, expectedError string) {
            analysis := createTestAnalysis(failedDetections)
            result, err := handler.Handle(context.Background(), analysis)

            Expect(err).ToNot(HaveOccurred())
            Expect(result.Valid).To(Equal(expectValid))
            if !expectValid {
                Expect(result.Errors).To(ContainElement(ContainSubstring(expectedError)))
            }
        },
        Entry("valid field: gitOpsManaged", []string{"gitOpsManaged"}, true, ""),
        Entry("valid field: pdbProtected", []string{"pdbProtected"}, true, ""),
        Entry("valid field: hpaEnabled", []string{"hpaEnabled"}, true, ""),
        Entry("valid field: stateful", []string{"stateful"}, true, ""),
        Entry("valid field: helmManaged", []string{"helmManaged"}, true, ""),
        Entry("valid field: networkIsolated", []string{"networkIsolated"}, true, ""),
        Entry("valid field: serviceMesh", []string{"serviceMesh"}, true, ""),
        Entry("invalid field: podSecurityLevel (removed v2.2)", []string{"podSecurityLevel"}, false, "podSecurityLevel"),
        Entry("invalid field: unknownField", []string{"unknownField"}, false, "unknownField"),
        Entry("empty slice: valid", []string{}, true, ""),
        Entry("nil slice: valid", nil, true, ""),
        Entry("multiple valid fields", []string{"gitOpsManaged", "pdbProtected"}, true, ""),
        Entry("mixed valid/invalid", []string{"gitOpsManaged", "invalidField"}, false, "invalidField"),
    )

    Context("Valid input", func() {
        It("should pass validation with complete input", func() {
            analysis := &aianalysisv1.AIAnalysis{
                Spec: aianalysisv1.AIAnalysisSpec{
                    AnalysisRequest: aianalysisv1.AnalysisRequest{
                        SignalContext: aianalysisv1.SignalContextInput{
                            Fingerprint:      "sha256:abc123",
                            SignalType:       "OOMKilled",
                            Environment:      "production",
                            BusinessPriority: "P0",
                            TargetResource: aianalysisv1.TargetResource{
                                Kind:      "Pod",
                                Name:      "test-pod",
                                Namespace: "default",
                            },
                            EnrichmentResults: sharedtypes.EnrichmentResults{
                                DetectedLabels: &sharedtypes.DetectedLabels{
                                    GitOpsManaged: true,
                                },
                            },
                        },
                    },
                },
            }

            result, err := handler.Handle(context.Background(), analysis)
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Valid).To(BeTrue())
            Expect(analysis.Status.Phase).To(Equal("Investigating"))
        })
    })

    Context("Invalid FailedDetections", func() {
        It("should reject invalid field name in FailedDetections", func() {
            analysis := &aianalysisv1.AIAnalysis{
                Spec: aianalysisv1.AIAnalysisSpec{
                    AnalysisRequest: aianalysisv1.AnalysisRequest{
                        SignalContext: aianalysisv1.SignalContextInput{
                            Fingerprint:      "sha256:abc123",
                            SignalType:       "OOMKilled",
                            Environment:      "production",
                            BusinessPriority: "P0",
                            TargetResource: aianalysisv1.TargetResource{
                                Kind:      "Pod",
                                Name:      "test-pod",
                                Namespace: "default",
                            },
                            EnrichmentResults: sharedtypes.EnrichmentResults{
                                DetectedLabels: &sharedtypes.DetectedLabels{
                                    FailedDetections: []string{"invalidField"},
                                },
                            },
                        },
                    },
                },
            }

            result, err := handler.Handle(context.Background(), analysis)
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Valid).To(BeFalse())
            Expect(result.Errors).To(ContainElement(ContainSubstring("invalidField")))
        })

        // DD-WORKFLOW-001 v2.2: podSecurityLevel removed
        It("should reject podSecurityLevel in FailedDetections (removed in v2.2)", func() {
            analysis := &aianalysisv1.AIAnalysis{
                Spec: aianalysisv1.AIAnalysisSpec{
                    AnalysisRequest: aianalysisv1.AnalysisRequest{
                        SignalContext: aianalysisv1.SignalContextInput{
                            Fingerprint:      "sha256:abc123",
                            SignalType:       "OOMKilled",
                            Environment:      "production",
                            BusinessPriority: "P0",
                            TargetResource: aianalysisv1.TargetResource{
                                Kind:      "Pod",
                                Name:      "test-pod",
                                Namespace: "default",
                            },
                            EnrichmentResults: sharedtypes.EnrichmentResults{
                                DetectedLabels: &sharedtypes.DetectedLabels{
                                    FailedDetections: []string{"podSecurityLevel"}, // REMOVED in v2.2
                                },
                            },
                        },
                    },
                },
            }

            result, err := handler.Handle(context.Background(), analysis)
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Valid).To(BeFalse())
            Expect(result.Errors).To(ContainElement(ContainSubstring("podSecurityLevel")))
        })
    })
})
```

**EOD Day 2 Checklist:**
- [ ] Pending phase handler implemented
- [ ] Validating phase handler implemented
- [ ] Controller updated to use handlers
- [ ] Unit tests for validation (including FailedDetections)
- [ ] Risk #5 (status conflict) addressed with optimistic locking
- [ ] Risk #6 (payload size) addressed with validation

---

### **Day 3: Investigating Phase - HolmesGPT-API Integration (8h)**

#### Key Deliverables
- HolmesGPT-API client wrapper
- Investigating phase handler
- Circuit breaker + retry logic (Risk #1)

#### DO Phase (6h)

**Step 1: Create HolmesGPT-API wrapper (3h)**

```go
// pkg/aianalysis/holmesgpt/client.go
package holmesgpt

import (
    "context"
    "fmt"
    "time"

    "github.com/go-logr/logr"
    holmesgptclient "github.com/jordigilh/kubernaut/pkg/clients/holmesgpt"
)

const (
    DefaultTimeout     = 30 * time.Second
    MaxRetries         = 3
    RetryBackoffBase   = 1 * time.Second
)

type Client struct {
    client  *holmesgptclient.Client
    baseURL string
    timeout time.Duration
    log     logr.Logger
}

func NewClient(baseURL string, log logr.Logger) (*Client, error) {
    client, err := holmesgptclient.NewClient(baseURL)
    if err != nil {
        return nil, fmt.Errorf("failed to create HolmesGPT client: %w", err)
    }

    return &Client{
        client:  client,
        baseURL: baseURL,
        timeout: DefaultTimeout,
        log:     log.WithName("holmesgpt-client"),
    }, nil
}

func (c *Client) AnalyzeIncident(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
    ctx, cancel := context.WithTimeout(ctx, c.timeout)
    defer cancel()

    var lastErr error
    for attempt := 0; attempt < MaxRetries; attempt++ {
        if attempt > 0 {
            backoff := RetryBackoffBase * time.Duration(1<<uint(attempt-1))
            c.log.V(1).Info("Retrying HolmesGPT-API call",
                "attempt", attempt+1,
                "backoff", backoff,
            )
            time.Sleep(backoff)
        }

        resp, err := c.doAnalyzeIncident(ctx, req)
        if err == nil {
            return resp, nil
        }

        lastErr = err
        if !isRetryable(err) {
            return nil, err
        }
    }

    return nil, fmt.Errorf("HolmesGPT-API call failed after %d retries: %w", MaxRetries, lastErr)
}

func (c *Client) doAnalyzeIncident(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
    // Convert to generated client types and call
    // Uses pkg/clients/holmesgpt/ (ogen-generated)
    // Implementation details depend on generated client API
    return nil, nil // TODO: Implement with generated client
}

func isRetryable(err error) bool {
    // Retry on timeout, connection errors, 5xx
    // Don't retry on 4xx (client errors)
    return true // Simplified for now
}
```

**Step 2: Implement Investigating phase (3h)**

```go
// pkg/aianalysis/phases/investigating.go
package phases

import (
    "context"
    "time"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/holmesgpt"
)

type InvestigatingHandler struct {
    holmesGPTClient *holmesgpt.Client
}

func NewInvestigatingHandler(client *holmesgpt.Client) *InvestigatingHandler {
    return &InvestigatingHandler{
        holmesGPTClient: client,
    }
}

func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
    // Build request from CRD spec
    req := h.buildIncidentRequest(analysis)

    // Call HolmesGPT-API
    resp, err := h.holmesGPTClient.AnalyzeIncident(ctx, req)
    if err != nil {
        analysis.Status.Phase = "Failed"
        analysis.Status.Reason = "InvestigationFailed"
        analysis.Status.Message = err.Error()
        return nil // Don't return error - state captured in status
    }

    // Update status from response
    now := metav1.NewTime(time.Now())
    analysis.Status.Phase = "Completed"
    analysis.Status.CompletedAt = &now
    analysis.Status.RootCause = resp.RootCause
    analysis.Status.TargetInOwnerChain = resp.TargetInOwnerChain
    analysis.Status.Warnings = resp.Warnings

    // Set selected workflow
    if resp.SelectedWorkflow != nil {
        analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
            WorkflowID:      resp.SelectedWorkflow.WorkflowID,
            Version:         resp.SelectedWorkflow.Version,
            ContainerImage:  resp.SelectedWorkflow.ContainerImage,
            ContainerDigest: resp.SelectedWorkflow.ContainerDigest,
            Confidence:      resp.SelectedWorkflow.Confidence,
            Parameters:      resp.SelectedWorkflow.Parameters,
            Rationale:       resp.SelectedWorkflow.Rationale,
        }
    }

    // Tokens and timing
    analysis.Status.TokensUsed = resp.TokensUsed
    analysis.Status.InvestigationTime = resp.InvestigationTimeMs

    return nil
}

func (h *InvestigatingHandler) buildIncidentRequest(analysis *aianalysisv1.AIAnalysis) *holmesgpt.IncidentRequest {
    sc := analysis.Spec.AnalysisRequest.SignalContext

    return &holmesgpt.IncidentRequest{
        Fingerprint:      sc.Fingerprint,
        SignalType:       sc.SignalType,
        Severity:         sc.Severity,
        Environment:      sc.Environment,
        BusinessPriority: sc.BusinessPriority,
        TargetResource: holmesgpt.TargetResource{
            Kind:      sc.TargetResource.Kind,
            Name:      sc.TargetResource.Name,
            Namespace: sc.TargetResource.Namespace,
        },
        DetectedLabels:   sc.EnrichmentResults.DetectedLabels,
        CustomLabels:     sc.EnrichmentResults.CustomLabels,
        OwnerChain:       sc.EnrichmentResults.OwnerChain,
        // Recovery context if applicable
        IsRecoveryAttempt:     analysis.Spec.IsRecoveryAttempt,
        RecoveryAttemptNumber: analysis.Spec.RecoveryAttemptNumber,
        PreviousExecutions:    analysis.Spec.PreviousExecutions,
    }
}
```

**EOD Day 3 Checklist:**
- [ ] HolmesGPT-API client wrapper created
- [ ] Circuit breaker + retry logic (Risk #1)
- [ ] Investigating phase handler implemented
- [ ] Response parsing (TargetInOwnerChain, Warnings, SelectedWorkflow)
- [ ] Recovery context handling (PreviousExecutions)

---

### **Day 4: Rego Policy Engine (8h)**

#### Key Deliverables
- OPA Rego engine with ConfigMap loading
- Approval policy evaluation
- Hot-reload support (Risk #3)

#### DO Phase (6h)

**Step 1: Create Rego engine (4h)**

```go
// pkg/aianalysis/rego/engine.go
package rego

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "sync"

    "github.com/go-logr/logr"
    "github.com/open-policy-agent/opa/v1/rego"
    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
    ConfigMapName      = "aianalysis-approval-policies"
    ConfigMapNamespace = "kubernaut-system"
    PolicyKey          = "approval.rego"
    DefaultTimeout     = 100 * time.Millisecond // DD-3: Safe default for Rego timeout
)

type ApprovalEngine struct {
    k8sClient     client.Client
    query         rego.PreparedEvalQuery
    policyVersion string
    mu            sync.RWMutex // Risk #3: Mutex for hot-reload
    log           logr.Logger
}

func NewApprovalEngine(k8sClient client.Client, log logr.Logger) (*ApprovalEngine, error) {
    engine := &ApprovalEngine{
        k8sClient: k8sClient,
        log:       log.WithName("rego-engine"),
    }

    if err := engine.loadPolicy(context.Background()); err != nil {
        return nil, fmt.Errorf("failed to load initial policy: %w", err)
    }

    return engine, nil
}

func (e *ApprovalEngine) loadPolicy(ctx context.Context) error {
    // Fetch ConfigMap
    cm := &corev1.ConfigMap{}
    key := client.ObjectKey{Name: ConfigMapName, Namespace: ConfigMapNamespace}
    if err := e.k8sClient.Get(ctx, key, cm); err != nil {
        return fmt.Errorf("failed to get policy ConfigMap: %w", err)
    }

    policy, ok := cm.Data[PolicyKey]
    if !ok {
        return fmt.Errorf("policy key %q not found in ConfigMap", PolicyKey)
    }

    // Compile policy
    query, err := rego.New(
        rego.Query("data.aianalysis.approval"),
        rego.Module("approval.rego", policy),
    ).PrepareForEval(ctx)
    if err != nil {
        return fmt.Errorf("failed to compile Rego policy: %w", err)
    }

    // Calculate version hash
    hash := sha256.Sum256([]byte(policy))
    version := "sha256:" + hex.EncodeToString(hash[:8])

    // Update with mutex protection (Risk #3)
    e.mu.Lock()
    e.query = query
    e.policyVersion = version
    e.mu.Unlock()

    e.log.Info("Rego policy loaded", "version", version)
    return nil
}

func (e *ApprovalEngine) Evaluate(ctx context.Context, input *ApprovalInput) (*ApprovalResult, error) {
    e.mu.RLock()
    query := e.query
    version := e.policyVersion
    e.mu.RUnlock()

    ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
    defer cancel()

    results, err := query.Eval(ctx, rego.EvalInput(input))
    if err != nil {
        // DD-3: Default to approval required on policy failure
        return &ApprovalResult{
            RequiresApproval: true,
            Reason:           "Policy evaluation failed: " + err.Error(),
            PolicyVersion:    version,
        }, nil
    }

    if len(results) == 0 || len(results[0].Expressions) == 0 {
        return &ApprovalResult{
            RequiresApproval: true,
            Reason:           "No approval decision from policy",
            PolicyVersion:    version,
        }, nil
    }

    // Parse result
    resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
    if !ok {
        return &ApprovalResult{
            RequiresApproval: true,
            Reason:           "Invalid policy output format",
            PolicyVersion:    version,
        }, nil
    }

    return &ApprovalResult{
        RequiresApproval: getBool(resultMap, "requires_approval", true),
        Reason:           getString(resultMap, "reason", ""),
        PolicyVersion:    version,
    }, nil
}

// Reload policy from ConfigMap (for hot-reload)
func (e *ApprovalEngine) Reload(ctx context.Context) error {
    return e.loadPolicy(ctx)
}

func (e *ApprovalEngine) PolicyVersion() string {
    e.mu.RLock()
    defer e.mu.RUnlock()
    return e.policyVersion
}
```

**Step 2: Create Rego input schema (1h)**

```go
// pkg/aianalysis/rego/input.go
package rego

import sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

// ApprovalInput is the input schema for Rego approval policies
// Per REGO_POLICY_EXAMPLES.md v1.4
type ApprovalInput struct {
    // Signal context
    SignalType       string `json:"signal_type"`
    Severity         string `json:"severity"`
    Environment      string `json:"environment"`
    BusinessPriority string `json:"business_priority"`

    // Target resource
    TargetResource TargetResourceInput `json:"target_resource"`

    // Detected labels (auto-detected by SignalProcessing)
    DetectedLabels *DetectedLabelsInput `json:"detected_labels,omitempty"`

    // Custom labels (customer-defined via Rego)
    CustomLabels map[string][]string `json:"custom_labels,omitempty"`

    // HolmesGPT-API response data
    Confidence         float64 `json:"confidence"`
    TargetInOwnerChain bool    `json:"target_in_owner_chain"` // DD-5: Include in Rego input
    Warnings           []string `json:"warnings,omitempty"`

    // FailedDetections (DD-WORKFLOW-001 v2.1)
    FailedDetections []string `json:"failed_detections,omitempty"` // DD-5: Include in Rego input

    // Recovery context
    IsRecoveryAttempt     bool `json:"is_recovery_attempt"`
    RecoveryAttemptNumber int  `json:"recovery_attempt_number,omitempty"`
}

type TargetResourceInput struct {
    Kind      string `json:"kind"`
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

// DetectedLabelsInput matches DetectedLabels (8 fields, no podSecurityLevel)
type DetectedLabelsInput struct {
    FailedDetections []string `json:"failed_detections,omitempty"`
    GitOpsManaged    bool     `json:"gitops_managed"`
    GitOpsTool       string   `json:"gitops_tool,omitempty"`
    PDBProtected     bool     `json:"pdb_protected"`
    HPAEnabled       bool     `json:"hpa_enabled"`
    Stateful         bool     `json:"stateful"`
    HelmManaged      bool     `json:"helm_managed"`
    NetworkIsolated  bool     `json:"network_isolated"`
    ServiceMesh      string   `json:"service_mesh,omitempty"`
    // NOTE: podSecurityLevel REMOVED in DD-WORKFLOW-001 v2.2
}

type ApprovalResult struct {
    RequiresApproval bool   `json:"requires_approval"`
    Reason           string `json:"reason,omitempty"`
    PolicyVersion    string `json:"policy_version"`
}

// Helper functions
func getBool(m map[string]interface{}, key string, defaultVal bool) bool {
    if v, ok := m[key].(bool); ok {
        return v
    }
    return defaultVal
}

func getString(m map[string]interface{}, key string, defaultVal string) string {
    if v, ok := m[key].(string); ok {
        return v
    }
    return defaultVal
}
```

**Step 3: Create example approval policy (1h)**

> **⚠️ OPA v1 SYNTAX REQUIRED**: All Rego policies MUST use OPA v1 syntax with `if` keyword and `:=` operator.
> Using old syntax (without `if`) will cause `rego_parse_error: 'if' keyword is required before rule body`.

```rego
# deploy/aianalysis/policies/approval.rego
# OPA v1 Syntax - REQUIRED for github.com/open-policy-agent/opa/v1/rego
package aianalysis.approval

import rego.v1

# Default values using := operator (OPA v1)
default require_approval := false
default reason := ""

# Helper: Is this a production environment?
is_production if {
    input.environment == "production"
}

# Helper: Check if detection failed for a field
detection_failed(field) if {
    input.failed_detections[_] == field
}

# Helper: Risky actions (scale down, delete, restart)
is_risky_action if {
    # Would check selected_workflow.workflow_id for risky patterns
    # For now, simplified
    true
}

# Rule 1: Low confidence requires approval
require_approval if {
    input.confidence < 0.8
}
reason := "Confidence below 80% threshold" if {
    input.confidence < 0.8
}

# Rule 2: Production environment with risky action
require_approval if {
    is_production
    is_risky_action
}
reason := "Production environment requires approval for risky actions" if {
    is_production
    is_risky_action
}

# Rule 3: Target not in owner chain (data quality concern)
require_approval if {
    is_production
    not input.target_in_owner_chain
}
reason := "DetectedLabels may not match affected resource (target not in OwnerChain)" if {
    is_production
    not input.target_in_owner_chain
}

# Rule 4: Detection failures in critical fields
require_approval if {
    detection_failed("pdbProtected")
    is_production
}
reason := "PDB protection status unknown (detection failed)" if {
    detection_failed("pdbProtected")
    is_production
}

# Rule 5: Recovery attempts require approval
require_approval if {
    input.is_recovery_attempt
    input.recovery_attempt_number >= 2
}
reason := "Multiple recovery attempts require human review" if {
    input.is_recovery_attempt
    input.recovery_attempt_number >= 2
}

# Rule 6: Warnings from HolmesGPT-API
require_approval if {
    count(input.warnings) > 0
    is_risky_action
}
reason := concat(": ", ["HolmesGPT-API warnings present", input.warnings[0]]) if {
    count(input.warnings) > 0
    is_risky_action
}
```

**EOD Day 4 Checklist:**
- [ ] OPA Rego engine created with ConfigMap loading
- [ ] Rego input schema matches REGO_POLICY_EXAMPLES.md v1.4
- [ ] Mutex protection for hot-reload (Risk #3)
- [ ] Policy validation on load (Risk #2)
- [ ] Default to approval required on failure (DD-3)
- [ ] Example approval policy created
- [ ] FailedDetections included in Rego input (DD-5)

---

### **Day 5: Metrics & Audit (8h)**

#### Key Deliverables
- Prometheus metrics (DD-005)
- Audit client (Risk #4)
- Complete reconciler integration

#### DO Phase (6h)

**Step 1: Create Prometheus metrics (2h)**

```go
// internal/controller/aianalysis/metrics.go
package aianalysis

import (
    "github.com/prometheus/client_golang/prometheus"
    "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
    reconcileTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_reconcile_total",
            Help: "Total number of AIAnalysis reconciliations",
        },
        []string{"phase", "result"},
    )

    phaseDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_phase_duration_seconds",
            Help:    "Duration of AIAnalysis phases",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to 51.2s
        },
        []string{"phase"},
    )

    holmesgptCallDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_holmesgpt_call_duration_seconds",
            Help:    "Duration of HolmesGPT-API calls",
            Buckets: prometheus.ExponentialBuckets(0.5, 2, 8), // 0.5s to 64s
        },
        []string{"endpoint", "status"},
    )

    regoEvalDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "aianalysis_rego_eval_duration_seconds",
            Help:    "Duration of Rego policy evaluations",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to 512ms
        },
    )

    approvalRequired = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_approval_required_total",
            Help: "Total AIAnalysis results requiring approval",
        },
        []string{"reason"},
    )

    detectionFailures = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_detection_failures_total",
            Help: "Detection failures by field name",
        },
        []string{"field"},
    )

    confidenceHistogram = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "aianalysis_confidence_score",
            Help:    "Distribution of workflow selection confidence scores",
            Buckets: prometheus.LinearBuckets(0.5, 0.05, 11), // 0.5 to 1.0
        },
    )
)

func init() {
    metrics.Registry.MustRegister(
        reconcileTotal,
        phaseDuration,
        holmesgptCallDuration,
        regoEvalDuration,
        approvalRequired,
        detectionFailures,
        confidenceHistogram,
    )
}
```

**Step 2: Create audit client (2h)**

```go
// internal/controller/aianalysis/audit.go
package aianalysis

import (
    "context"
    "time"

    "github.com/go-logr/logr"
    "github.com/jordigilh/kubernaut/pkg/audit"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

type AuditClient struct {
    store audit.BufferedStore
    log   logr.Logger
}

func NewAuditClient(store audit.BufferedStore, log logr.Logger) *AuditClient {
    return &AuditClient{
        store: store,
        log:   log.WithName("audit"),
    }
}

func (c *AuditClient) RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
    event := audit.Event{
        EventType:     "aianalysis_completed",
        RemediationID: analysis.Spec.RemediationID,
        Timestamp:     time.Now(),
        Data: map[string]interface{}{
            "aianalysis_name":      analysis.Name,
            "namespace":            analysis.Namespace,
            "phase":                analysis.Status.Phase,
            "approval_required":    analysis.Status.ApprovalRequired,
            "approval_reason":      analysis.Status.ApprovalReason,
            "confidence":           analysis.Status.SelectedWorkflow.Confidence,
            "workflow_id":          analysis.Status.SelectedWorkflow.WorkflowID,
            "target_in_owner_chain": analysis.Status.TargetInOwnerChain,
            "warnings_count":       len(analysis.Status.Warnings),
            "tokens_used":          analysis.Status.TokensUsed,
            "is_recovery_attempt":  analysis.Spec.IsRecoveryAttempt,
        },
    }

    // Fire-and-forget (Risk #4: async buffered)
    if err := c.store.Write(ctx, event); err != nil {
        c.log.Error(err, "Failed to write audit event",
            "event_type", event.EventType,
            "remediation_id", event.RemediationID,
        )
        // Don't fail reconciliation on audit failure
    }
}
```

**EOD Day 5 Checklist:**
- [ ] Prometheus metrics created (DD-005)
- [ ] Audit client with buffered store (Risk #4)
- [ ] Metrics for HolmesGPT-API calls, Rego eval, approvals
- [ ] Detection failure metrics
- [ ] Fire-and-forget audit pattern
- [ ] **Create Error Handling Philosophy document** ⭐ V3.0

#### **Error Handling Philosophy Document** (Day 5 EOD Deliverable)

**File**: `docs/services/crd-controllers/02-aianalysis/implementation/design/ERROR_HANDLING_PHILOSOPHY.md`

**Required Sections**:
1. **Error Classification** (Transient, Permanent, User)
2. **Service-Specific Error Categories** (A-E):
   - **Category A**: CRD Not Found (normal during deletion)
   - **Category B**: HolmesGPT-API Errors (retry with backoff)
   - **Category C**: Rego Policy Errors (graceful degradation)
   - **Category D**: Status Update Conflicts (optimistic locking)
   - **Category E**: Audit Write Failures (fire-and-forget)
3. **Retry Strategy Table** (which errors retry, backoff times)
4. **Graceful Degradation Matrix** (what happens when dependencies fail)

---

### **Day 6: Unit Tests (8h)**

#### Key Deliverables
- Unit tests for all components
- 70%+ coverage target
- Fake K8s client (ADR-004)

**Test files to create:**
- `test/unit/aianalysis/controller_test.go`
- `test/unit/aianalysis/validating_test.go` (created Day 2)
- `test/unit/aianalysis/investigating_test.go`
- `test/unit/aianalysis/rego_test.go`
- `test/unit/aianalysis/holmesgpt_test.go`

**EOD Day 6 Checklist:**
- [ ] Controller unit tests (fake K8s client)
- [ ] Phase handler unit tests
- [ ] Rego engine unit tests
- [ ] HolmesGPT client unit tests
- [ ] 70%+ code coverage

---

### **Day 7: Integration Tests (8h)**

#### Key Deliverables
- KIND cluster setup
- MockLLMServer integration
- Rego policy integration tests

**See**: [Rego Policy Testing Strategy](#-rego-policy-testing-strategy) section for detailed patterns.

**EOD Day 7 Checklist:**
- [ ] KIND cluster configured
- [ ] MockLLMServer running
- [ ] Reconciler integration tests
- [ ] Rego policy integration tests (4 scenarios)
- [ ] Cross-CRD coordination tests

---

### **Day 8: E2E Tests (8h)**

#### Key Deliverables
- Complete workflow selection E2E
- Recovery flow E2E
- Approval signaling E2E

**EOD Day 8 Checklist:**
- [ ] Workflow selection E2E test
- [ ] Recovery flow E2E test
- [ ] Approval signaling E2E test
- [ ] Health/metrics validation

---

### **Day 9: Documentation (8h)**

#### Key Deliverables
- API documentation updates
- Runbooks
- Error handling philosophy document

**EOD Day 9 Checklist:**
- [ ] API documentation updated
- [ ] 3 production runbooks created
- [ ] Error handling philosophy documented
- [ ] Troubleshooting guide

---

### **Day 10: Production Readiness (8h)**

#### Key Deliverables
- Production readiness checklist complete
- Final validation
- Handoff notes

**EOD Day 10 Checklist:**
- [ ] Production readiness checklist 100%
- [ ] All tests passing
- [ ] Documentation complete
- [ ] Handoff notes written
- [ ] Confidence assessment: target 95%+

---

## 🧪 **Rego Policy Testing Strategy**

> **Context**: Rego policy integration testing is unique to services that use OPA for classification/approval. This section documents the dedicated testing approach adapted from SignalProcessing V1.19.

### **Why Dedicated Rego Testing?**

Unlike typical unit tests that mock the Rego engine, **integration tests must validate the full policy lifecycle**:

1. **ConfigMap Loading**: K8s ConfigMap → policy string extraction
2. **Policy Compilation**: OPA `rego.New()` → `PreparedEvalQuery`
3. **Policy Evaluation**: Input data → Rego evaluation → structured output
4. **Hot-Reload**: ConfigMap update → policy recompilation without restart
5. **Graceful Degradation**: Invalid policy → fallback to `approvalRequired=true`

### **Test File: `rego_integration_test.go`**

| Test Scenario | BR Coverage | Description |
|---------------|-------------|-------------|
| **ConfigMap → Policy Load** | BR-AI-030 | Create ConfigMap, verify policy loads correctly |
| **Hot-Reload Under Load** | BR-AI-032 | Update ConfigMap during active reconciliation, verify no race |
| **Invalid Policy Fallback** | BR-AI-031 | Invalid Rego syntax → default `approvalRequired=true` |
| **Policy Version Tracking** | BR-AI-033 | Audit trail includes policy version hash |

### **Integration Test Pattern**

```go
var _ = Describe("Rego Policy Integration", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        configMap *corev1.ConfigMap
        engine    *rego.ApprovalEngine
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = envTestClient // Real K8s API (KIND)

        configMap = &corev1.ConfigMap{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "aianalysis-approval-policies",
                Namespace: "kubernaut-system",
            },
            Data: map[string]string{
                "approval.rego": validApprovalPolicy,
            },
        }
        Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
    })

    AfterEach(func() {
        Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
    })

    // Test 1: ConfigMap → Policy Load (BR-AI-030)
    It("should load policy from ConfigMap", func() {
        engine, err := rego.NewApprovalEngine(k8sClient, ctrl.Log)
        Expect(err).ToNot(HaveOccurred())
        Expect(engine.PolicyVersion()).To(HavePrefix("sha256:"))
    })

    // Test 2: Hot-Reload Under Load (BR-AI-032)
    It("should hot-reload policy without race condition", func() {
        engine, _ := rego.NewApprovalEngine(k8sClient, ctrl.Log)

        var wg sync.WaitGroup
        for i := 0; i < 10; i++ {
            wg.Add(1)
            go func() {
                defer wg.Done()
                _, err := engine.Evaluate(ctx, testInput)
                Expect(err).ToNot(HaveOccurred())
            }()
        }

        // Update ConfigMap mid-evaluation
        configMap.Data["approval.rego"] = updatedApprovalPolicy
        Expect(k8sClient.Update(ctx, configMap)).To(Succeed())
        Expect(engine.Reload(ctx)).To(Succeed())

        wg.Wait()
        // No panic, no race = success
    })

    // Test 3: Invalid Policy Fallback (BR-AI-031)
    It("should fallback to approval required when policy is invalid", func() {
        configMap.Data["approval.rego"] = "invalid { rego syntax"
        Expect(k8sClient.Update(ctx, configMap)).To(Succeed())

        engine, err := rego.NewApprovalEngine(k8sClient, ctrl.Log)
        Expect(err).To(HaveOccurred())

        // On failure, should default to requiring approval (DD-3)
        result, err := engine.Evaluate(ctx, testInput)
        Expect(err).ToNot(HaveOccurred())
        Expect(result.RequiresApproval).To(BeTrue())
    })

    // Test 4: Policy Version in Audit (BR-AI-033)
    It("should include policy version in approval result", func() {
        engine, _ := rego.NewApprovalEngine(k8sClient, ctrl.Log)
        result, _ := engine.Evaluate(ctx, testInput)

        Expect(result.PolicyVersion).ToNot(BeEmpty())
        Expect(result.PolicyVersion).To(HavePrefix("sha256:"))
    })
})
```

### **Risk #3 Mitigation: Hot-Reload Race Condition**

| Risk | Mitigation | Test Validation |
|------|------------|-----------------|
| ConfigMap hot-reload race condition | `sync.RWMutex` protection on policy reload, version tracking | Test 2: 10 concurrent evaluations + ConfigMap update |

---

## 📊 **Business Requirements Coverage Matrix**

| BR ID | Description | Test File | Test Type | Coverage |
|-------|-------------|-----------|-----------|----------|
| **BR-AI-001** | CRD creation event | `controller_test.go` | Unit | ✅ |
| **BR-AI-020** | Input validation | `validating_test.go` | Unit | ✅ |
| **BR-AI-021** | SignalContext validation | `validating_test.go` | Unit | ✅ |
| **BR-AI-023** | Catalog validation | `investigating_test.go` | Unit | ✅ |
| **BR-AI-030** | Rego policy evaluation | `rego_test.go` | Unit + Integration | ✅ |
| **BR-AI-031** | Policy failure fallback | `rego_integration_test.go` | Integration | ✅ |
| **BR-AI-032** | Policy hot-reload | `rego_integration_test.go` | Integration | ✅ |
| **BR-AI-033** | Policy version tracking | `rego_integration_test.go` | Integration | ✅ |
| **BR-AI-075** | Workflow selection output | `investigating_test.go` | Unit | ✅ |
| **BR-AI-076** | Approval context | `controller_test.go` | Unit | ✅ |
| **BR-AI-080** | Recovery attempt handling | `investigating_test.go` | Unit | ✅ |
| **BR-AI-081** | Previous executions | `investigating_test.go` | Unit | ✅ |
| **BR-AI-082** | Recovery analysis | `holmesgpt_test.go` | Unit | ✅ |
| **BR-AI-083** | Recovery workflow selection | `investigating_test.go` | Unit | ✅ |

**Coverage**: 14/31 V1.0 BRs explicitly shown (remaining BRs covered by integration/E2E tests)

---

## ✅ **Production Readiness Checklist**

### **Code Quality**
- [ ] Zero lint errors (`golangci-lint run ./...`)
- [ ] Zero compilation errors
- [ ] 70%+ unit test coverage
- [ ] All BRs covered by tests

### **CRD Controller**
- [ ] Reconciliation loop handles all phases
- [ ] Status updates work correctly
- [ ] Finalizer implemented for cleanup
- [ ] RBAC rules complete

### **Observability**
- [ ] Prometheus metrics exposed (DD-005)
- [ ] Structured logging (`logr.Logger`)
- [ ] Health checks (`/healthz`, `/readyz`)
- [ ] Audit trail to Data Storage Service

### **Configuration**
- [ ] ConfigMap for Rego policies
- [ ] Environment variable overrides
- [ ] Validation for all required fields
- [ ] Hot-reload for Rego policies

### **Integration**
- [ ] HolmesGPT-API integration tested
- [ ] Data Storage audit integration tested
- [ ] SignalProcessing data flow validated
- [ ] RO coordination validated

---

## 🎯 **Edge Case Categories Template** ⭐ V3.0

> **Purpose**: Ensure comprehensive edge case testing for AIAnalysis
> **Reference**: Template V3.0, SignalProcessing V1.19 patterns

| Category | Description | AIAnalysis Test Pattern |
|----------|-------------|------------------------|
| **Configuration Changes** | Rego policy updated during reconciliation | Start analysis, update ConfigMap, verify policy reload |
| **Rate Limiting** | HolmesGPT-API rate limits | Mock 429 responses, verify exponential backoff |
| **Large Payloads** | EnrichmentResults exceeds typical size | Create large KubernetesContext, verify no OOM |
| **Concurrent Operations** | Multiple AIAnalysis CRDs reconciling | Parallel reconciliations, verify no race conditions |
| **Partial Failures** | HolmesGPT returns partial response | Mock incomplete response, verify graceful degradation |
| **Context Cancellation** | Reconciliation cancelled mid-investigation | Cancel context, verify cleanup and status update |
| **FailedDetections Handling** | DetectedLabels has failed fields | Mock RBAC failure, verify `failedDetections` propagation |
| **Recovery Flow Edge Cases** | Recovery attempt with stale context | Verify fresh enrichment used, not cached |

### **AIAnalysis Edge Case Test Pattern**
```go
var _ = Describe("Edge Cases", func() {
    Context("when Rego policy is updated during reconciliation", func() {
        BeforeEach(func() {
            // Setup: Create AIAnalysis in Investigating phase
            // Trigger: Update ConfigMap with new policy
        })

        It("should use the new policy for approval evaluation", func() {
            // Verify: New policy applied, old decision not cached
        })
    })

    Context("when HolmesGPT-API returns 429 rate limit", func() {
        It("should apply exponential backoff and retry", func() {
            // Mock: Return 429 for first 2 calls, success on 3rd
            // Verify: Delays increase (1s, 2s, 4s), eventually succeeds
        })
    })

    Context("when DetectedLabels has FailedDetections", func() {
        It("should propagate failed fields to HolmesGPT-API request", func() {
            // Setup: DetectedLabels with FailedDetections: ["gitOpsManaged"]
            // Verify: HolmesGPT request includes failedDetections array
        })
    })
})
```

---

## 📊 **Metrics Validation Commands Template** ⭐ V3.0

```bash
# Start AIAnalysis controller locally (for validation)
go run ./cmd/aianalysis/main.go \
    --metrics-bind-address=:9090 \
    --health-probe-bind-address=:8081

# Verify metrics endpoint
curl -s localhost:9090/metrics | grep aianalysis_

# Expected AIAnalysis metrics:
# aianalysis_reconciliations_total{phase="validating",status="success"} 0
# aianalysis_reconciliations_total{phase="investigating",status="success"} 0
# aianalysis_reconciliations_total{phase="analyzing",status="success"} 0
# aianalysis_reconciliations_total{phase="recommending",status="success"} 0
# aianalysis_holmesgpt_api_duration_seconds_bucket{endpoint="/incident/analyze",le="1"} 0
# aianalysis_rego_policy_evaluation_duration_seconds_bucket{policy="approval",le="0.1"} 0
# aianalysis_errors_total{error_type="holmesgpt_timeout"} 0
# aianalysis_errors_total{error_type="rego_policy_failure"} 0
# aianalysis_approval_decisions_total{decision="auto_approve"} 0
# aianalysis_approval_decisions_total{decision="manual_review"} 0

# Verify health endpoints
curl -s localhost:8081/healthz  # Should return 200
curl -s localhost:8081/readyz   # Should return 200

# Create test AIAnalysis resource
kubectl apply -f config/samples/aianalysis_v1alpha1_aianalysis.yaml

# Verify metrics increment
watch -n 1 'curl -s localhost:9090/metrics | grep aianalysis_reconciliations_total'
```

---

## 📝 **Lessons Learned Template** ⭐ V3.0

> **Purpose**: Capture insights for future implementations
> **Location**: `docs/services/crd-controllers/02-aianalysis/implementation/LESSONS_LEARNED.md`

### **What Worked Well**
1. [To be completed at Day 10]
   - **Evidence**: [How we know it worked]
   - **Recommendation**: [Should we continue/expand this?]

### **Technical Wins**
1. [To be completed at Day 10]
   - **Impact**: [Quantifiable impact if possible]

### **Challenges Overcome**
1. [To be completed at Day 10]
   - **Solution**: [How we solved it]
   - **Lesson**: [What we learned]

### **What Would We Do Differently**
1. [To be completed at Day 10]
   - **Reason**: [Why this would be better]
   - **Impact**: [Expected improvement]

---

## 🔧 **Technical Debt Template** ⭐ V3.0

> **Purpose**: Track known issues for future resolution
> **Location**: `docs/services/crd-controllers/02-aianalysis/implementation/TECHNICAL_DEBT.md`

### **Minor Issues (Non-Blocking)**
| Issue | Impact | Estimated Effort | Priority |
|-------|--------|------------------|----------|
| [To be completed at Day 10] | [Impact] | [Hours/Days] | P3 |

### **Future Enhancements (Post-V1.0)**
| Enhancement | Business Value | Estimated Effort | Target Version |
|-------------|---------------|------------------|----------------|
| AIApprovalRequest CRD | Dedicated approval workflow | 3-5 days | V1.1 |
| Multi-provider LLM support | Vendor flexibility | 5-7 days | V2.0 |
| Dynamic workflow generation | Advanced AI capabilities | 10+ days | V2.0+ |

### **Known Limitations**
1. **Single HolmesGPT-API Provider**: V1.0 supports only HolmesGPT-API, no fallback to other LLM providers
2. **Synchronous Approval Flow**: V1.0 signals approval to RO, dedicated CRD workflow in V1.1
3. **Predefined Workflows Only**: LLM selects from catalog, no dynamic workflow generation

---

## 🤝 **Team Handoff Notes Template** ⭐ V3.0

### **Key Files to Review**
| File | Purpose | Priority |
|------|---------|----------|
| `cmd/aianalysis/main.go` | Entry point, signal handling | High |
| `internal/controller/aianalysis/reconciler.go` | Main reconciliation logic | High |
| `pkg/aianalysis/phases/` | Phase handlers (validating, investigating, etc.) | High |
| `pkg/aianalysis/rego/` | Rego policy engine | High |
| `api/aianalysis/v1alpha1/aianalysis_types.go` | CRD type definitions | Medium |
| `docs/.../ERROR_HANDLING_PHILOSOPHY.md` | Error handling guide | Medium |

### **Running Locally**
```bash
# Terminal 1: Start KIND cluster with CRDs
make kind-create
make install

# Terminal 2: Start HolmesGPT-API with MockLLMServer
cd holmesgpt-api && python tests/mock_llm_server.py &
uvicorn main:app --host 0.0.0.0 --port 8080

# Terminal 3: Start AIAnalysis controller
make run-aianalysis

# Terminal 4: Create test AIAnalysis
kubectl apply -f config/samples/aianalysis_v1alpha1_aianalysis.yaml
kubectl get aianalysis -w
```

### **Debugging Tips**
```bash
# Common debugging commands
kubectl logs -l app=aianalysis-controller -n kubernaut-system --tail=100

# Force re-reconciliation
kubectl annotate aianalysis <name> force-reconcile=$(date +%s) --overwrite

# Check Rego policy ConfigMap
kubectl get configmap ai-approval-policies -n kubernaut-system -o yaml

# Check leader election
kubectl get lease aianalysis-controller-leader -n kubernaut-system -o yaml

# Profile memory/CPU
kubectl top pod -l app=aianalysis-controller -n kubernaut-system
```

### **Common Issues and Solutions**
| Issue | Symptom | Solution |
|-------|---------|----------|
| HolmesGPT-API connection failure | Phase stuck at `Investigating` | Check HolmesGPT-API pod health, verify port 8080 |
| Rego policy not loading | All decisions default to manual | Check ConfigMap exists, verify Rego syntax |
| FailedDetections validation error | CRD rejected on create | Ensure only valid field names in FailedDetections array |
| Recovery flow not triggering | `isRecoveryAttempt` ignored | Verify WorkflowExecution failure status, check RO creates new AIAnalysis |

---

## 🔷 **CRD API Group Standard** ⭐ V3.0

**Reference**: [DD-CRD-001: API Group Domain Selection](../../../architecture/decisions/DD-CRD-001-api-group-domain-selection.md)

### **Current Implementation**

AIAnalysis CRD uses the `kubernaut.io` domain:

```yaml
apiVersion: aianalysis.kubernaut.ai/v1alpha1
kind: AIAnalysis
```

**Note**: Template v3.0 specifies `.kubernaut.ai` per DD-CRD-001, but existing CRD types use `.kubernaut.io`. This is a project-wide decision to be addressed separately.

### **CRD Inventory (AIAnalysis Service)**

| CRD | API Group | Purpose |
|-----|-----------|---------|
| AIAnalysis | `aianalysis.kubernaut.ai/v1alpha1` | HolmesGPT RCA + workflow selection |

### **RBAC Markers (Current)**

```go
//+kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/finalizers,verbs=update
```

### **Industry Best Practices Analysis**

| Project | API Group Strategy | Pattern |
|---------|-------------------|---------|
| **Tekton** | `tekton.dev/v1` | ✅ Unified - all CRDs under single domain |
| **Istio** | `istio.io/v1` | ✅ Unified - network, security, config all under `istio.io` |
| **Cert-Manager** | `cert-manager.io/v1` | ✅ Unified - certificates, issuers, challenges |
| **ArgoCD** | `argoproj.io/v1alpha1` | ✅ Unified - applications, projects, rollouts |
| **Kubernaut** | `[service].kubernaut.ai/v1alpha1` | ✅ Unified - remediation workflow CRDs |

---

## 📚 **References**

### **Authoritative Documents**

| Document | Purpose |
|----------|---------|
| [BR_MAPPING.md](./BR_MAPPING.md) | 31 V1.0 business requirements |
| [crd-schema.md v2.4](./crd-schema.md) | CRD type definitions |
| [REGO_POLICY_EXAMPLES.md v1.4](./REGO_POLICY_EXAMPLES.md) | Rego input schema |
| [DD-WORKFLOW-001 v2.2](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | DetectedLabels (8 fields) |
| [DD-RECOVERY-002](../../../architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md) | Recovery flow |
| [DD-CONTRACT-002](../../../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md) | Integration contracts |

### **Cross-Team Handoff Documents**

| Document | Team | Status |
|----------|------|--------|
| [AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md](../../../handoff/AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md) | SignalProcessing | ✅ Resolved |
| [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../../../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) | HolmesGPT-API | ✅ Resolved |
| [AIANALYSIS_TO_RO_TEAM.md](../../../handoff/AIANALYSIS_TO_RO_TEAM.md) | RO | ✅ Resolved |
| [QUESTIONS_FOR_DATA_STORAGE_TEAM.md](../../../handoff/QUESTIONS_FOR_DATA_STORAGE_TEAM.md) | Data Storage | ✅ Resolved |

### **Template Reference**

- [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v3.0](../../SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md)

---

## 📝 **Version History**

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| **v1.1** | 2025-12-04 | AIAnalysis Team | Template v3.0 full alignment (Edge Cases, Metrics Validation, Lessons Learned, Technical Debt, Team Handoff, CRD API Group) |
| **v1.0** | 2025-12-03 | AIAnalysis Team | Initial implementation plan |

---

**Ready to implement?** Start with [Day 1: Foundation](#day-1-foundation-8h) 🚀

