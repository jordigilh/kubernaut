# Critical Path Implementation Plan

**Date**: October 9, 2025
**Status**: 📋 **READY FOR EXECUTION**
**Timeline**: 15-20 weeks (3.75-5 months)
**Goal**: Complete end-to-end remediation system

---

## 📊 **Executive Summary**

### **Critical Path Services (6 total)**

| # | Service | Current | Target | Effort | Dependencies |
|---|---------|---------|--------|--------|--------------|
| 0 | **RemediationRequest** | ✅ 100% | ✅ 100% | DONE | None |
| 1 | **Gateway Service** | 🔨 30% | ✅ 100% | 1-2 weeks | None |
| 2 | **RemediationProcessing** | 🚧 5% | ✅ 100% | 3-4 weeks | Gateway |
| 3 | **AIAnalysis** | 🚧 5% | ✅ 100% | 4-5 weeks | RemediationProcessing |
| 4 | **WorkflowExecution** | 🚧 5% | ✅ 100% | 4-5 weeks | AIAnalysis |
| 5 | **KubernetesExecution** (DEPRECATED - ADR-025) | 🚧 5% | ✅ 100% | 3-4 weeks | WorkflowExecution |

**Total Timeline**: 15-20 weeks (optimistic-realistic)
**Target Deployment**: Week 19-26

---

## 🎯 **Phase 0: Gateway Service Completion**

**Timeline**: Week 1-2 (1-2 weeks)
**Status**: 🔨 30% → ✅ 100%
**Priority**: ⚡ CRITICAL (blocks all other work)

### **Current State**

✅ **Completed**:
- HTTP server with webhook endpoints (`pkg/gateway/service.go`, 414 lines)
- Prometheus webhook handler
- Authentication middleware
- Rate limiting
- Signal extraction logic (`pkg/gateway/signal_extraction.go`)
- Health check endpoint

❌ **Missing**:
- Kubernetes client integration
- RemediationRequest CRD creation
- Deduplication logic
- Comprehensive unit tests
- Integration tests with Kubernetes

### **Implementation Tasks**

#### **Week 1: Kubernetes Integration**

**Day 1-2: Setup & Planning**
- [ ] Review existing gateway code
- [ ] Design Kubernetes client integration pattern
- [ ] Define RemediationRequest creation interface
- [ ] Write integration test plan

**Day 3-5: Kubernetes Client Integration (TDD)**
- [ ] **RED**: Write tests for Kubernetes client operations
  - Test: Create RemediationRequest CRD
  - Test: Handle CRD creation errors
  - Test: Validate CRD schema
- [ ] **GREEN**: Implement Kubernetes client
  ```go
  type K8sClient interface {
      CreateRemediationRequest(ctx context.Context, req *RemediationRequest) error
      GetRemediationRequest(ctx context.Context, name string) (*RemediationRequest, error)
  }
  ```
- [ ] **GREEN**: Integrate with gateway service
- [ ] **REFACTOR**: Add error handling and retries

#### **Week 2: Testing & Deduplication**

**Day 1-3: Deduplication Logic (TDD)**
- [ ] **RED**: Write deduplication tests
  - Test: Duplicate fingerprint detection
  - Test: Deduplication window (5 minutes)
  - Test: Update existing RemediationRequest
- [ ] **GREEN**: Implement deduplication
  ```go
  func (g *Gateway) checkDuplicateRemediationRequest(ctx context.Context, fingerprint string) (*RemediationRequest, bool, error)
  ```
- [ ] **REFACTOR**: Add caching for performance

**Day 4-5: Integration Tests**
- [ ] Write integration tests with envtest
  - Test: Webhook → RemediationRequest CRD creation
  - Test: Duplicate handling
  - Test: Error scenarios
- [ ] Run full test suite
- [ ] Fix any issues

### **Deliverables**

- [ ] Kubernetes client integration complete
- [ ] RemediationRequest CRD creation working
- [ ] Deduplication logic implemented
- [ ] Unit tests: 20+ tests
- [ ] Integration tests: 5+ tests
- [ ] Documentation updated

### **Success Criteria**

✅ Webhook received → RemediationRequest CRD created in Kubernetes
✅ Duplicate signals handled correctly (no duplicate CRDs)
✅ All tests passing (unit + integration)
✅ Error handling complete
✅ Metrics and logging in place

---

## 🎯 **Phase 1: RemediationProcessing Controller**

**Timeline**: Week 3-6 (3-4 weeks)
**Status**: 🚧 5% → ✅ 100%
**Priority**: ⚡ CRITICAL (Service 01)
**Documentation**: `docs/services/crd-controllers/01-signalprocessing/`

### **Current State**

✅ **Completed**:
- CRD schema defined (`api/remediationprocessing/v1alpha1/`)
- Controller scaffold (63 lines)
- Documentation (4,500+ lines across 11 files)

❌ **Missing**:
- Reconciliation logic (100%)
- Kubernetes context enrichment
- Signal classification
- All tests (0 tests currently)

### **Implementation Tasks**

#### **Week 3: Core Reconciliation & Context Enrichment**

**Day 1-2: Setup & Analysis**
- [ ] Review RemediationProcessing documentation
- [ ] Study RemediationRequest controller as reference
- [ ] Map business requirements (BR-RP-001 to BR-RP-025)
- [ ] Design reconciliation state machine

**Day 3-5: Reconciliation Logic (TDD)**
- [ ] **RED**: Write reconciliation tests
  - Test: Watch RemediationRequest CRD
  - Test: Phase progression (pending → enriching → classifying → completed)
  - Test: Status updates
- [ ] **GREEN**: Implement basic reconciliation loop
  ```go
  func (r *RemediationProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
      // Fetch RemediationProcessing
      // Handle deletion (finalizer)
      // Process based on phase
      // Update status
  }
  ```
- [ ] **GREEN**: Implement phase handlers
  - `handlePendingPhase()` - Initialize
  - `handleEnrichingPhase()` - Kubernetes context gathering
  - `handleClassifyingPhase()` - Signal classification
- [ ] **REFACTOR**: Add error handling and retries

#### **Week 4: Kubernetes Context Enrichment (TDD)**

**Day 1-3: Resource Discovery**
- [ ] **RED**: Write context enrichment tests
  - Test: Discover resources from labels
  - Test: Extract pod logs
  - Test: Get resource status
  - Test: Gather recent events
- [ ] **GREEN**: Implement Kubernetes client integration
  ```go
  func (r *RemediationProcessingReconciler) enrichKubernetesContext(ctx context.Context, signal *Signal) (map[string]string, error)
  ```
- [ ] **GREEN**: Implement resource discovery
  - Pod discovery and log extraction
  - Deployment/StatefulSet discovery
  - Node status gathering
  - Recent Kubernetes events
- [ ] **REFACTOR**: Add caching and optimization

**Day 4-5: Context Data Preparation**
- [ ] Format context data for AIAnalysis
- [ ] Validate context adequacy
- [ ] Write unit tests for context formatting
- [ ] Integration tests with real cluster

#### **Week 5: Signal Classification (TDD)**

**Day 1-3: Classification Logic**
- [ ] **RED**: Write classification tests
  - Test: Severity classification
  - Test: Priority assignment
  - Test: Signal type detection
  - Test: Resource type identification
- [ ] **GREEN**: Implement classification algorithms
  ```go
  func (r *RemediationProcessingReconciler) classifySignal(ctx context.Context, processing *RemediationProcessing) error
  ```
- [ ] **GREEN**: Implement severity scoring
- [ ] **REFACTOR**: Add ML-based classification (optional for V1)

**Day 4-5: Status Management**
- [ ] Implement phase transition logic
- [ ] Add timeout handling (5 min per phase)
- [ ] Implement failure detection
- [ ] Write status update tests

#### **Week 6: Testing & Integration**

**Day 1-2: Unit Tests**
- [ ] Write comprehensive unit tests
  - Context enrichment: 15+ tests
  - Classification: 10+ tests
  - Phase transitions: 8+ tests
  - Error handling: 5+ tests
- [ ] Target: 40+ unit tests
- [ ] Run table-driven tests (Ginkgo)

**Day 3-4: Integration Tests**
- [ ] Write integration tests with envtest
  - Test: RemediationRequest → RemediationProcessing creation
  - Test: Context enrichment with real cluster
  - Test: Classification workflow
  - Test: Completion and status updates
- [ ] Target: 8+ integration tests

**Day 5: Finalization**
- [ ] Fix all test failures
- [ ] Add metrics (8+ Prometheus metrics)
- [ ] Add events (6+ Kubernetes events)
- [ ] Update documentation
- [ ] Code review

### **Deliverables**

- [ ] RemediationProcessing controller fully implemented
- [ ] Context enrichment working (Pod logs, events, status)
- [ ] Classification logic complete
- [ ] Unit tests: 40+ tests
- [ ] Integration tests: 8+ tests
- [ ] Metrics: 8 Prometheus metrics
- [ ] Events: 6 Kubernetes events
- [ ] Documentation complete

### **Success Criteria**

✅ RemediationRequest → SignalProcessing CRD created
✅ Kubernetes context enriched (logs, events, status)
✅ Signal classified (severity, priority, type)
✅ Context data prepared for AIAnalysis
✅ All tests passing (40+ unit, 8+ integration)
✅ Metrics and events working
✅ Phase transitions correct (pending → enriching → classifying → completed)

---

## 🎯 **Phase 2: AIAnalysis Controller**

**Timeline**: Week 7-11 (4-5 weeks)
**Status**: 🚧 5% → ✅ 100%
**Priority**: ⚡ CRITICAL (Service 02)
**Documentation**: `docs/services/crd-controllers/02-aianalysis/`

### **Current State**

✅ **Completed**:
- CRD schema defined (`api/aianalysis/v1alpha1/`, 300 lines)
- Controller scaffold (63 lines)
- Documentation (5,200+ lines across 16 files)
- HolmesGPT integration design

❌ **Missing**:
- HolmesGPT client integration
- Root cause analysis logic
- Recommendation generation
- Approval workflow
- All tests (0 tests currently)

### **Implementation Tasks**

#### **Week 7: Core Reconciliation & HolmesGPT Client**

**Day 1-2: Setup & Analysis**
- [ ] Review AIAnalysis documentation
- [ ] Study HolmesGPT API documentation
- [ ] Map business requirements (BR-AI-001 to BR-AI-030)
- [ ] Design HolmesGPT integration pattern

**Day 3-5: Reconciliation & Client (TDD)**
- [ ] **RED**: Write reconciliation tests
  - Test: Watch RemediationProcessing completion
  - Test: Phase progression (pending → analyzing → generating → approval → completed)
  - Test: Status updates
- [ ] **GREEN**: Implement basic reconciliation loop
- [ ] **RED**: Write HolmesGPT client tests
  - Test: API authentication
  - Test: Analysis request
  - Test: Response parsing
  - Test: Error handling
- [ ] **GREEN**: Implement HolmesGPT client
  ```go
  type HolmesGPTClient interface {
      AnalyzeSignal(ctx context.Context, req *AnalysisRequest) (*AnalysisResponse, error)
      GenerateRecommendation(ctx context.Context, rootCause string) (*Recommendation, error)
  }
  ```
- [ ] **REFACTOR**: Add retry logic and circuit breaker

#### **Week 8: Root Cause Analysis (TDD)**

**Day 1-3: Analysis Logic**
- [ ] **RED**: Write root cause analysis tests
  - Test: Context adequacy validation
  - Test: AI analysis invocation
  - Test: Root cause extraction
  - Test: Confidence scoring
- [ ] **GREEN**: Implement analysis orchestration
  ```go
  func (r *AIAnalysisReconciler) performRootCauseAnalysis(ctx context.Context, analysis *AIAnalysis) error
  ```
- [ ] **GREEN**: Integrate with HolmesGPT
  - Context preparation
  - API call with timeout
  - Response parsing
  - Confidence validation
- [ ] **REFACTOR**: Add error handling and fallbacks

**Day 4-5: Historical Pattern Matching (Optional)**
- [ ] Query vector database for similar patterns
- [ ] Apply historical insights
- [ ] Boost confidence with patterns
- [ ] Write pattern matching tests

#### **Week 9: Recommendation Generation (TDD)**

**Day 1-3: Recommendation Logic**
- [ ] **RED**: Write recommendation tests
  - Test: Recommendation generation from root cause
  - Test: Action mapping (root cause → action)
  - Test: Workflow definition creation
  - Test: Safety validation
- [ ] **GREEN**: Implement recommendation engine
  ```go
  func (r *AIAnalysisReconciler) generateRecommendation(ctx context.Context, rootCause string) (*Recommendation, error)
  ```
- [ ] **GREEN**: Map to predefined actions
  - restart_pod
  - scale_deployment
  - update_config
  - rollback_deployment
- [ ] **REFACTOR**: Add validation and safety checks

**Day 4-5: Workflow Definition Creation**
- [ ] Convert recommendation to workflow definition
- [ ] Add dependency information
- [ ] Set step parameters
- [ ] Validate workflow structure

#### **Week 10: Approval Workflow (TDD)**

**Day 1-3: Approval Logic**
- [ ] **RED**: Write approval workflow tests
  - Test: AI approval (high confidence)
  - Test: Human approval required (low confidence)
  - Test: Approval status handling
  - Test: Timeout on pending approval
- [ ] **GREEN**: Implement approval logic
  ```go
  func (r *AIAnalysisReconciler) determineApproval(confidence float64) ApprovalType
  ```
- [ ] **GREEN**: Implement approval state machine
  - Auto-approve if confidence > 0.9
  - Human approval if confidence 0.7-0.9
  - Reject if confidence < 0.7
- [ ] **REFACTOR**: Add notification integration

**Day 4-5: Integration with WorkflowExecution**
- [ ] Prepare AIAnalysis output for WorkflowExecution
- [ ] Set WorkflowExecution spec fields
- [ ] Test data propagation
- [ ] Validate workflow creation

#### **Week 11: Testing & Integration**

**Day 1-2: Unit Tests**
- [ ] Write comprehensive unit tests
  - HolmesGPT client: 12+ tests
  - Root cause analysis: 10+ tests
  - Recommendation generation: 8+ tests
  - Approval workflow: 6+ tests
  - Phase transitions: 8+ tests
- [ ] Target: 45+ unit tests

**Day 3-4: Integration Tests**
- [ ] Write integration tests with envtest
  - Test: RemediationProcessing → AIAnalysis creation
  - Test: Root cause analysis workflow
  - Test: Recommendation generation
  - Test: Approval scenarios
  - Test: WorkflowExecution creation
- [ ] Target: 10+ integration tests
- [ ] Mock HolmesGPT for integration tests

**Day 5: Finalization**
- [ ] Fix all test failures
- [ ] Add metrics (10+ Prometheus metrics)
- [ ] Add events (7+ Kubernetes events)
- [ ] Update documentation
- [ ] Code review

### **Deliverables**

- [ ] AIAnalysis controller fully implemented
- [ ] HolmesGPT client integrated
- [ ] Root cause analysis working
- [ ] Recommendation generation complete
- [ ] Approval workflow implemented
- [ ] Unit tests: 45+ tests
- [ ] Integration tests: 10+ tests
- [ ] Metrics: 10 Prometheus metrics
- [ ] Events: 7 Kubernetes events
- [ ] Documentation complete

### **Success Criteria**

✅ RemediationProcessing → AIAnalysis CRD created
✅ HolmesGPT integration working
✅ Root cause identified with confidence score
✅ Recommendations generated
✅ Approval workflow functional
✅ Workflow definition created for WorkflowExecution
✅ All tests passing (45+ unit, 10+ integration)
✅ Metrics and events working

---

## 🎯 **Phase 3: WorkflowExecution Controller**

**Timeline**: Week 12-16 (4-5 weeks)
**Status**: 🚧 5% → ✅ 100%
**Priority**: ⚡ CRITICAL (Service 03)
**Documentation**: `docs/services/crd-controllers/03-workflowexecution/`

### **Current State**

✅ **Completed**:
- CRD schema defined (`api/workflowexecution/v1alpha1/`, 520 lines)
- Controller scaffold (63 lines)
- Documentation (6,800+ lines across 14 files)
- Workflow engine design in `pkg/workflow/`

❌ **Missing**:
- Workflow planning logic
- Safety validation (RBAC, Rego)
- Step orchestration
- KubernetesExecution CRD creation
- All tests (0 tests currently)

### **Implementation Tasks**

#### **Week 12: Core Reconciliation & Planning Phase**

**Day 1-2: Setup & Analysis**
- [ ] Review WorkflowExecution documentation
- [ ] Study existing workflow engine code in `pkg/workflow/`
- [ ] Map business requirements (BR-WF-001 to BR-WF-021)
- [ ] Design workflow state machine

**Day 3-5: Reconciliation & Planning (TDD)**
- [ ] **RED**: Write reconciliation tests
  - Test: Watch AIAnalysis completion
  - Test: Phase progression (pending → planning → validation → execution → monitoring → completed)
  - Test: Status updates
- [ ] **GREEN**: Implement basic reconciliation loop
- [ ] **RED**: Write planning phase tests
  - Test: Workflow analysis
  - Test: Dependency resolution
  - Test: Execution strategy planning
  - Test: Resource estimation
- [ ] **GREEN**: Implement planning logic
  ```go
  func (r *WorkflowExecutionReconciler) planWorkflow(ctx context.Context, workflow *WorkflowExecution) error
  ```
- [ ] **REFACTOR**: Add parallel execution detection

#### **Week 13: Validation Phase (TDD)**

**Day 1-3: Safety Validation**
- [ ] **RED**: Write validation tests
  - Test: RBAC checks
  - Test: Rego policy validation
  - Test: Resource availability checks
  - Test: Dry-run validation
- [ ] **GREEN**: Implement safety validation
  ```go
  func (r *WorkflowExecutionReconciler) validateSafety(ctx context.Context, workflow *WorkflowExecution) error
  ```
- [ ] **GREEN**: RBAC validation
  - Check service account permissions
  - Validate resource access
  - Verify namespace permissions
- [ ] **GREEN**: Rego policy integration
  - Load safety policies
  - Evaluate workflow against policies
  - Handle policy violations
- [ ] **REFACTOR**: Add policy caching

**Day 4-5: Approval Integration**
- [ ] Check AIAnalysis approval status
- [ ] Handle pending approvals
- [ ] Implement approval timeout
- [ ] Write approval integration tests

#### **Week 14: Execution Phase - Step Orchestration (TDD)**

**Day 1-3: Step Orchestration**
- [ ] **RED**: Write step orchestration tests
  - Test: Sequential step execution
  - Test: Parallel step execution
  - Test: Dependency handling
  - Test: Step failure handling
- [ ] **GREEN**: Implement step orchestration
  ```go
  func (r *WorkflowExecutionReconciler) executeSteps(ctx context.Context, workflow *WorkflowExecution) error
  ```
- [ ] **GREEN**: Create KubernetesExecution CRD per step
  - Map workflow step to KubernetesExecution spec
  - Set owner references
  - Configure step parameters
- [ ] **REFACTOR**: Add adaptive optimization

**Day 4-5: Dependency Resolution**
- [ ] Parse step dependencies
- [ ] Build execution DAG
- [ ] Detect parallel opportunities
- [ ] Schedule steps in correct order
- [ ] Write dependency resolution tests

#### **Week 15: Monitoring & Completion (TDD)**

**Day 1-3: Step Monitoring**
- [ ] **RED**: Write monitoring tests
  - Test: Watch KubernetesExecution status
  - Test: Detect step completion
  - Test: Handle step failures
  - Test: Aggregate step results
- [ ] **GREEN**: Implement monitoring logic
  ```go
  func (r *WorkflowExecutionReconciler) monitorSteps(ctx context.Context, workflow *WorkflowExecution) error
  ```
- [ ] **GREEN**: Watch KubernetesExecution CRDs
- [ ] **GREEN**: Update workflow status based on steps
- [ ] **REFACTOR**: Add effectiveness tracking

**Day 4-5: Rollback Support (Optional for V1)**
- [ ] Design rollback strategy
- [ ] Implement rollback on failure
- [ ] Write rollback tests
- [ ] Validate rollback workflow

#### **Week 16: Testing & Integration**

**Day 1-2: Unit Tests**
- [ ] Write comprehensive unit tests
  - Planning phase: 12+ tests
  - Validation phase: 10+ tests
  - Execution phase: 15+ tests
  - Monitoring phase: 8+ tests
- [ ] Target: 45+ unit tests

**Day 3-4: Integration Tests**
- [ ] Write integration tests with envtest
  - Test: AIAnalysis → WorkflowExecution creation
  - Test: Planning and validation workflow
  - Test: Step execution and KubernetesExecution creation
  - Test: Step monitoring and completion
  - Test: Failure handling
- [ ] Target: 10+ integration tests

**Day 5: Finalization**
- [ ] Fix all test failures
- [ ] Add metrics (12+ Prometheus metrics)
- [ ] Add events (8+ Kubernetes events)
- [ ] Update documentation
- [ ] Code review

### **Deliverables**

- [ ] WorkflowExecution controller fully implemented
- [ ] Planning phase complete
- [ ] Validation phase with RBAC and Rego
- [ ] Step orchestration working
- [ ] KubernetesExecution CRD creation per step
- [ ] Monitoring and completion logic
- [ ] Unit tests: 45+ tests
- [ ] Integration tests: 10+ tests
- [ ] Metrics: 12 Prometheus metrics
- [ ] Events: 8 Kubernetes events
- [ ] Documentation complete

### **Success Criteria**

✅ AIAnalysis → WorkflowExecution CRD created
✅ Workflow planned and validated
✅ Safety checks passed (RBAC, Rego)
✅ Steps orchestrated correctly (sequential/parallel)
✅ KubernetesExecution CRDs created per step
✅ Step monitoring working
✅ All tests passing (45+ unit, 10+ integration)
✅ Metrics and events working

---

## 🎯 **Phase 4: KubernetesExecution Controller** (DEPRECATED - ADR-025)

**Timeline**: Week 17-20 (3-4 weeks)
**Status**: 🚧 5% → ✅ 100%
**Priority**: ⚡ CRITICAL (Service 04)
**Documentation**: `docs/services/crd-controllers/04-kubernetesexecutor/`

### **Current State**

✅ **Completed**:
- CRD schema defined (`api/kubernetesexecution/v1alpha1/`)
- Controller scaffold (63 lines)
- Documentation (5,600+ lines across 15 files)
- Predefined actions design

❌ **Missing**:
- Kubernetes Job creation and management
- Predefined action execution
- Custom script execution
- Safety validation
- All tests (0 tests currently)

### **Implementation Tasks**

#### **Week 17: Core Reconciliation & Job Management**

**Day 1-2: Setup & Analysis**
- [ ] Review KubernetesExecution documentation
- [ ] Study Kubernetes Job API
- [ ] Map business requirements (BR-KE-001 to BR-KE-020)
- [ ] Design execution strategy

**Day 3-5: Reconciliation & Job Creation (TDD)**
- [ ] **RED**: Write reconciliation tests
  - Test: Watch WorkflowExecution step creation
  - Test: Phase progression (pending → validating → executing → verifying → completed)
  - Test: Status updates
- [ ] **GREEN**: Implement basic reconciliation loop
- [ ] **RED**: Write Job creation tests
  - Test: Create Kubernetes Job
  - Test: Job configuration
  - Test: Resource limits
  - Test: Error handling
- [ ] **GREEN**: Implement Job creation
  ```go
  func (r *KubernetesExecutionReconciler) createExecutionJob(ctx context.Context, ke *KubernetesExecution) error
  ```
- [ ] **REFACTOR**: Add Job template management

#### **Week 18: Predefined Actions (TDD)**

**Day 1-3: Action Implementation**
- [ ] **RED**: Write predefined action tests for each action:
  - Test: restart_pod
  - Test: scale_deployment
  - Test: update_config
  - Test: rollback_deployment
  - Test: delete_pod
  - Test: cordon_node
  - Test: drain_node
  - Test: restart_service
  - Test: clear_cache
  - Test: update_hpa
- [ ] **GREEN**: Implement each predefined action
  ```go
  func (r *KubernetesExecutionReconciler) executeAction(ctx context.Context, action string, params map[string]string) error
  ```
- [ ] **GREEN**: Action-specific logic:
  ```go
  func (r *KubernetesExecutionReconciler) restartPod(ctx context.Context, params map[string]string) error
  func (r *KubernetesExecutionReconciler) scaleDeployment(ctx context.Context, params map[string]string) error
  // ... more actions
  ```
- [ ] **REFACTOR**: Add action parameter validation

**Day 4-5: Custom Script Execution**
- [ ] Design custom script execution pattern
- [ ] Implement script execution in Job
- [ ] Add script validation
- [ ] Write custom script tests

#### **Week 19: Safety & Health Verification (TDD)**

**Day 1-3: Safety Validation**
- [ ] **RED**: Write safety validation tests
  - Test: Rego policy validation
  - Test: Resource impact assessment
  - Test: Blast radius calculation
  - Test: Safety approval
- [ ] **GREEN**: Implement safety validation
  ```go
  func (r *KubernetesExecutionReconciler) validateExecutionSafety(ctx context.Context, ke *KubernetesExecution) error
  ```
- [ ] **GREEN**: Rego policy integration
  - Load execution policies
  - Evaluate action safety
  - Handle policy violations
- [ ] **REFACTOR**: Add policy caching

**Day 4-5: Health Verification**
- [ ] **RED**: Write health verification tests
  - Test: Post-execution health checks
  - Test: Resource status validation
  - Test: Service availability checks
  - Test: Rollback trigger
- [ ] **GREEN**: Implement health verification
  ```go
  func (r *KubernetesExecutionReconciler) verifyHealth(ctx context.Context, ke *KubernetesExecution) error
  ```
- [ ] **GREEN**: Check resource health after execution
- [ ] **REFACTOR**: Add retry logic for health checks

#### **Week 20: Testing & Integration**

**Day 1-2: Unit Tests**
- [ ] Write comprehensive unit tests
  - Job creation: 10+ tests
  - Predefined actions: 20+ tests (2 per action)
  - Safety validation: 8+ tests
  - Health verification: 8+ tests
- [ ] Target: 45+ unit tests

**Day 3-4: Integration Tests**
- [ ] Write integration tests with envtest
  - Test: WorkflowExecution → KubernetesExecution creation
  - Test: Job creation and execution
  - Test: Predefined action execution (restart_pod, scale_deployment)
  - Test: Health verification
  - Test: Failure and rollback
- [ ] Target: 10+ integration tests
- [ ] Use real Kubernetes cluster for action testing

**Day 5: Finalization**
- [ ] Fix all test failures
- [ ] Add metrics (10+ Prometheus metrics)
- [ ] Add events (7+ Kubernetes events)
- [ ] Update documentation
- [ ] Code review

### **Deliverables**

- [ ] KubernetesExecution controller fully implemented
- [ ] Kubernetes Job management working
- [ ] 10+ predefined actions implemented
- [ ] Custom script execution supported
- [ ] Safety validation with Rego
- [ ] Health verification complete
- [ ] Unit tests: 45+ tests
- [ ] Integration tests: 10+ tests
- [ ] Metrics: 10 Prometheus metrics
- [ ] Events: 7 Kubernetes events
- [ ] Documentation complete

### **Success Criteria**

✅ WorkflowExecution → KubernetesExecution CRD created
✅ Kubernetes Jobs created and executed
✅ Predefined actions working (restart_pod, scale_deployment, etc.)
✅ Safety validation passed (Rego policies)
✅ Health verification after execution
✅ All tests passing (45+ unit, 10+ integration)
✅ Metrics and events working
✅ **END-TO-END FLOW COMPLETE** ✅

---

## 🧪 **Phase 5: Integration Testing**

**Timeline**: Week 21-23 (2-3 weeks)
**Status**: ❌ Not Started
**Priority**: ⚡ CRITICAL

### **Week 21-22: End-to-End Integration Tests**

**Goal**: Validate complete system flow from webhook to execution

**Day 1-3: Happy Path Testing**
- [ ] Write E2E test: Webhook → RemediationRequest → RemediationProcessing → AIAnalysis → WorkflowExecution → KubernetesExecution → Completion
- [ ] Test with real Prometheus alerts
- [ ] Test with real HolmesGPT (if available) or mocked
- [ ] Validate all CRDs created correctly
- [ ] Verify data propagation between services
- [ ] Check timing and latency
- [ ] Target: 5+ E2E happy path tests

**Day 4-5: Failure Scenario Testing**
- [ ] Test failure at each phase:
  - RemediationProcessing failure
  - AIAnalysis failure (low confidence)
  - WorkflowExecution validation failure
  - KubernetesExecution job failure
- [ ] Test timeout scenarios
- [ ] Test retry logic
- [ ] Verify failure propagation
- [ ] Check error messages and events
- [ ] Target: 10+ E2E failure tests

**Week 22: Performance & Load Testing**

**Day 1-2: Performance Testing**
- [ ] Measure end-to-end latency
  - Target: P95 < 3 minutes for simple remediations
  - Target: P95 < 10 minutes for complex workflows
- [ ] Profile each controller
- [ ] Identify bottlenecks
- [ ] Optimize slow paths

**Day 3-5: Load Testing**
- [ ] Test with 10 concurrent remediations
- [ ] Test with 50 concurrent remediations
- [ ] Test with 100 concurrent remediations
- [ ] Monitor resource usage (CPU, memory)
- [ ] Check for race conditions
- [ ] Verify no data corruption
- [ ] Target: Support 100+ concurrent remediations

### **Week 23: Bug Fixes & Refinements**

**Day 1-3: Bug Fixing**
- [ ] Fix all identified bugs from testing
- [ ] Address performance issues
- [ ] Fix race conditions
- [ ] Improve error handling

**Day 4-5: Final Validation**
- [ ] Run full test suite (all 200+ tests)
- [ ] Verify all metrics working
- [ ] Check all events emitting
- [ ] Validate documentation accuracy
- [ ] Final code review

### **Deliverables**

- [ ] E2E integration tests: 15+ tests
- [ ] Performance benchmarks documented
- [ ] Load test results documented
- [ ] All bugs fixed
- [ ] Complete test suite passing (200+ tests)

### **Success Criteria**

✅ Complete E2E flow working (webhook → completion)
✅ All failure scenarios handled correctly
✅ Performance targets met (P95 < 3-10 min)
✅ Load testing passed (100+ concurrent)
✅ All 200+ tests passing
✅ Zero critical bugs
✅ System ready for deployment

---

## 📦 **Phase 6: Deployment Preparation**

**Timeline**: Week 24 (1 week)
**Status**: ❌ Not Started
**Priority**: ⚡ CRITICAL

### **Week 24: Production Readiness**

**Day 1: Documentation Review**
- [ ] Review all service documentation
- [ ] Update deployment guides
- [ ] Create operator runbooks
- [ ] Write troubleshooting guides
- [ ] Document configuration options

**Day 2: Deployment Manifests**
- [ ] Create Kustomize overlays
  - Base configuration
  - Development overlay
  - Staging overlay
  - Production overlay
- [ ] Configure resource limits
- [ ] Set up network policies
- [ ] Configure service accounts
- [ ] Set RBAC rules

**Day 3: Monitoring Setup**
- [ ] Create Prometheus ServiceMonitors
- [ ] Design Grafana dashboards (6 dashboards)
  - Gateway metrics
  - RemediationRequest orchestration
  - RemediationProcessing metrics
  - AIAnalysis metrics
  - WorkflowExecution metrics
  - KubernetesExecution metrics
- [ ] Configure AlertManager rules
  - High failure rate alerts
  - High timeout rate alerts
  - Latency SLO violations
  - Resource exhaustion alerts

**Day 4: Configuration & Secrets**
- [ ] Set up HolmesGPT API credentials
- [ ] Configure database connections
- [ ] Set up notification channels
- [ ] Configure rate limits
- [ ] Set timeout values

**Day 5: Final Checks**
- [ ] Deployment checklist validation
- [ ] Rollback plan documented
- [ ] Incident response plan ready
- [ ] On-call rotation scheduled
- [ ] Go/No-Go decision

### **Deliverables**

- [ ] Deployment manifests complete
- [ ] Grafana dashboards created
- [ ] AlertManager rules configured
- [ ] Documentation complete
- [ ] Rollback plan documented
- [ ] Go/No-Go decision made

---

## 📊 **Timeline Summary**

| Phase | Service | Duration | Cumulative | Status |
|-------|---------|----------|------------|--------|
| **0** | Gateway | 1-2 weeks | Week 2 | 🔨 In Progress |
| **1** | RemediationProcessing | 3-4 weeks | Week 6 | ❌ Not Started |
| **2** | AIAnalysis | 4-5 weeks | Week 11 | ❌ Not Started |
| **3** | WorkflowExecution | 4-5 weeks | Week 16 | ❌ Not Started |
| **4** | KubernetesExecution (DEPRECATED - ADR-025) | 3-4 weeks | Week 20 | ❌ Not Started |
| **5** | Integration Testing | 2-3 weeks | Week 23 | ❌ Not Started |
| **6** | Deployment Prep | 1 week | Week 24 | ❌ Not Started |

**Total Timeline**: 18-24 weeks (optimistic-realistic-conservative)

**Target Deployment**: **Week 19 (optimistic)** to **Week 26 (conservative)**

---

## 🎯 **Success Metrics**

### **Development Metrics**

- [ ] **Code**: ~10,000+ lines of controller code
- [ ] **Tests**: 200+ tests (unit + integration + E2E)
- [ ] **Test Coverage**: >80% for all services
- [ ] **Documentation**: Complete for all services
- [ ] **Zero Critical Bugs**: No P0/P1 bugs at deployment

### **Performance Metrics**

- [ ] **End-to-End Latency**: P95 < 3 minutes (simple), P95 < 10 minutes (complex)
- [ ] **Concurrent Load**: Support 100+ concurrent remediations
- [ ] **Success Rate**: >95% for all phases
- [ ] **Timeout Rate**: <2% for all phases
- [ ] **Failure Recovery**: Automatic retry for transient failures

### **Production Readiness Metrics**

- [ ] **Observability**: 50+ Prometheus metrics, 40+ event types
- [ ] **Monitoring**: 6 Grafana dashboards, 20+ alert rules
- [ ] **Security**: RBAC configured, network policies in place
- [ ] **Resilience**: Finalizers, timeouts, retries, health checks
- [ ] **Documentation**: Deployment, operation, troubleshooting guides

---

## 🚀 **Next Steps**

### **Immediate (This Week)**

1. ✅ Review and approve this implementation plan
2. 🔨 Begin Gateway Service completion (Week 1-2)
3. 📋 Set up project tracking (Jira/GitHub Projects)
4. 👥 Assign team members to phases

### **Week 1 Kickoff**

1. Gateway Service: Start Kubernetes client integration
2. Review RemediationProcessing documentation
3. Set up development environment
4. Create feature branch: `feature/gateway-k8s-integration`

---

**Plan Status**: ✅ **READY FOR EXECUTION**
**Approval Required**: Yes
**Estimated Completion**: Week 19-26 (4.5-6 months)
**Risk Level**: Medium (dependencies on external services like HolmesGPT)

**Document Owner**: Development Team
**Last Updated**: October 9, 2025
**Next Review**: Weekly during execution


