# Test Plan: CRD Field Validation in Per-Suite E2E Tests

**Feature**: Extend CRD status field validation from fullpipeline E2E to individual CRD controller E2E test suites
**Version**: 1.2
**Created**: 2026-02-22
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feat/crd-type-unification-113`

**Authority**:
- [Issue #163](https://github.com/jordigilh/kubernaut/issues/163): Extend CRD field validation to individual E2E test suites
- [Issue #118](https://github.com/jordigilh/kubernaut/issues/118): FullPipeline CRD status validation (predecessor)

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [FullPipeline Status Validation Test Plan](../test-plans/FULLPIPELINE_E2E_STATUS_VALIDATION_TEST_PLAN.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **SignalProcessing (SP)**: Validate status fields across reactive, predictive, degraded, and recovery scenarios
- **AIAnalysis (AA)**: Validate RCA output, analysis timing, alternative workflows, and validation history
- **WorkflowExecution (WE)**: Validate execution runtime status, comprehensive failure details, and block clearance
- **NotificationRequest (NT)**: Validate delivery lifecycle timestamps, per-channel counters, and failure explanation
- **EffectivenessAssessment (EA)**: Validate scheduling fields, conditions, and failure paths
- **RemediationRequest (RR)**: Validate phase timestamps, deduplication, blocking, skip, failure, and outcome fields
- **RemediationApprovalRequest (RAR)**: Validate decision message, conditions, and expiry path

### Out of Scope

- **Unit and integration test gaps**: Tracked separately; this plan covers E2E tier only
- **Gateway CRD**: Gateway creates RRs but does not reconcile status; field validation is not applicable
- **FullPipeline E2E validators**: Already covered by Issue #118; this plan addresses per-suite gaps
- **New CRD fields or schema changes**: This plan validates existing fields that are already populated but not asserted

### Design Decisions

- **Validators with functional options**: Extend existing `ValidateXXXStatus()` functions in `test/shared/validators/crd_status.go` with option functions (e.g., `WithDegradedMode()`, `WithRecoveryContext()`) rather than creating new validators. This keeps a single source of truth per CRD.
- **Business outcome focus**: Each test validates "what does the operator/system get?" -- behavior, correctness, and data accuracy -- not "what function was called?"
- **RO tests mock child CRDs intentionally**: RO E2E tests set child CRD statuses directly to control reconciler paths. This is by design (testing the RO controller, not all services) and new RO scenarios follow this pattern.
- **Per-suite E2E only**: This plan scopes to E2E validation gaps. No new unit or integration tests are proposed here.

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **E2E**: This plan targets the E2E tier exclusively, extending validation to cover >=80% of CRD status fields across realistic business scenarios in each per-suite E2E test

### Business Outcome Quality Bar

Tests validate **business outcomes** -- behavior, correctness, and data accuracy -- not just code path coverage. Each test scenario answers: "what does the user/operator/system get?" not "what function is called?"

Specifically:
- **Behavior**: Does the controller produce the correct terminal state for this scenario?
- **Correctness**: Are the status fields populated with the exact expected values for the controlled input?
- **Accuracy**: Are timestamps temporally ordered, counters equal to exact expected counts, and conditions matching exact expected states?

### Deterministic Assertion Policy

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md): since E2E tests control the input, all assertions MUST use exact expected values. Weak assertions (`>= 1`, `non-empty`, `at least one`) are anti-patterns -- they mask regressions and don't prove the controller produced the correct result for the given input. The only exception is timestamp values, where temporal ordering (A < B) replaces exact value assertions.

---

## 3. Current Coverage Inventory

### Existing Validators (`test/shared/validators/crd_status.go`)

| Validator | CRD | Checks | Used In |
|-----------|-----|--------|---------|
| `ValidateSPStatus` | SignalProcessing | 23 | fullpipeline only |
| `ValidateAAStatus` | AIAnalysis | 23 base + 9 approval | fullpipeline only |
| `ValidateRRStatus` | RemediationRequest | 15 base + 2 approval | fullpipeline only |
| `ValidateWEStatus` | WorkflowExecution | 9 | fullpipeline only |
| `ValidateNTStatus` | NotificationRequest | 8 | fullpipeline only |
| `ValidateEAStatus` | EffectivenessAssessment | 8 | fullpipeline only |
| `ValidateRARStatus` | RemediationApprovalRequest | 6 | fullpipeline only |
| `ValidateRARSpec` | RemediationApprovalRequest | 13 | fullpipeline only |

### Per-Suite E2E Field Coverage (Before This Plan)

| CRD | Total Status Fields | Validated in Per-Suite E2E | Gap |
|-----|--------------------|-----------------------------|-----|
| SignalProcessing | 18 | ~8 (Phase, KubernetesContext, EnvClassification, Priority) | ~10 |
| AIAnalysis | 27 | ~21 (Phase, Approval*, SelectedWorkflow, Investigation*, PostRCA, Conditions) | ~6 |
| WorkflowExecution | 11 | ~8 (Phase, timestamps, Duration, ExecutionRef, FailureDetails partial, Conditions) | ~3 |
| NotificationRequest | 12 | ~3 (Phase, DeliveryAttempts presence) | ~9 |
| EffectivenessAssessment | 9 | ~6 (Phase, Components, AssessmentReason, CompletedAt) | ~3 |
| RemediationRequest | 47 | ~7 (OverallPhase, child refs, Outcome, SelectedWorkflowRef) | ~25+ |
| RemediationApprovalRequest | 11 | ~6 (Decision, DecidedBy, DecidedAt, CreatedAt, Expired) | ~5 |

---

## 4. BR Coverage Matrix

| BR/ADR ID | Description | CRD | Test ID | Status |
|-----------|-------------|-----|---------|--------|
| BR-SP-001 | Signal processing produces enriched context | SP | E2E-SP-163-001 | Pending |
| BR-SP-070 | Rego policy determines severity | SP | E2E-SP-163-002 | Pending |
| BR-SP-100 | Signal mode classification | SP | E2E-SP-163-003 | Pending |
| BR-SP-001 | Recovery context for escalation | SP | E2E-SP-163-004 | Pending |
| ADR-032 | Controller health via conditions | SP | E2E-SP-163-005 | Pending |
| BR-AI-011 | Root cause analysis output | AA | E2E-AA-163-001 | Pending |
| BR-AI-013 | Analysis SLA tracking | AA | E2E-AA-163-002 | Pending |
| BR-AI-011 | Alternative workflow transparency | AA | E2E-AA-163-003 | Pending |
| BR-AI-011 | Validation attempt tracking | AA | E2E-AA-163-004 | Pending |
| BR-WE-001 | Execution runtime status | WE | E2E-WE-163-001 | Pending |
| BR-WE-004 | Comprehensive failure details | WE | E2E-WE-163-002 | Pending |
| BR-AUDIT-006 | Block clearance attribution | WE | E2E-WE-163-003 | Pending |
| BR-NOT-069 | Delivery lifecycle timestamps | NT | E2E-NT-163-001 | Pending |
| BR-NOT-069 | Delivery counters for SLA | NT | E2E-NT-163-002 | Pending |
| BR-NOT-069 | Per-channel delivery details | NT | E2E-NT-163-003 | Pending |
| ADR-032 | Notification conditions | NT | E2E-NT-163-004 | Pending |
| BR-NOT-069 | Failure explanation | NT | E2E-NT-163-005 | Pending |
| BR-ORCH-025 | Assessment scheduling visibility | EA | E2E-EA-163-001 | Pending |
| ADR-032 | Assessment conditions | EA | E2E-EA-163-002 | Pending |
| BR-ORCH-025 | Assessment failure explanation | EA | E2E-EA-163-003 | Pending |
| BR-ORCH-025 | Phase timestamp tracking | RR | E2E-RO-163-001 | Pending |
| BR-GATEWAY-185 | Deduplication transparency | RR | E2E-RO-163-002 | Pending |
| BR-ORCH-042 | Blocking reason visibility | RR | E2E-RO-163-003 | Pending |
| BR-ORCH-025 | Skip reason visibility | RR | E2E-RO-163-004 | Pending |
| BR-ORCH-025 | Failure post-mortem | RR | E2E-RO-163-005 | Pending |
| BR-ORCH-025 | Outcome reporting | RR | E2E-RO-163-006 | Pending |
| ADR-032 | RR conditions | RR | E2E-RO-163-007 | Pending |
| BR-AUDIT-006 | Approval rationale audit | RAR | E2E-RAR-163-001 | Pending |
| ADR-040 | RAR condition lifecycle | RAR | E2E-RAR-163-002 | Pending |
| ADR-040 | Approval expiry handling | RAR | E2E-RAR-163-003 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `E2E-{SERVICE}-163-{SEQUENCE}`

- **TIER**: `E2E` (all scenarios in this plan)
- **SERVICE**: SP, AA, WE, NT, EA, RO, RAR
- **163**: Issue number
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

---

### 5.1 SignalProcessing (SP)

**Existing E2E scenarios** (`test/e2e/signalprocessing/business_requirements_test.go`):
- Node enrichment with Pod scheduled on node
- Degraded mode when target Pod is missing
- Priority P0 for production namespace
- Environment classification from namespace labels
- Owner chain resolution (ReplicaSet -> Deployment)
- CustomLabels from workload labels
- Workload types (Deployment, StatefulSet, DaemonSet, ReplicaSet)
- Audit event emission (signal.processed)

**Fields already validated**: Phase, KubernetesContext (Workload, Namespace, OwnerChain, CustomLabels, DegradedMode), EnvironmentClassification (Environment, Source), PriorityAssignment (Priority, Source)

**Fields NOT validated (gap)**:

| Field | Why It Matters |
|-------|----------------|
| StartTime, CompletionTime | Audit compliance requires processing timestamps |
| Severity | Downstream consumers (RO, AA) use severity for routing |
| PolicyHash | Tracks which Rego policy version produced the result |
| SignalMode | Distinguishes reactive vs predictive signals |
| SignalType, OriginalSignalType | Signal classification for routing and analytics |
| RecoveryContext | Consecutive failure escalation requires previous attempt data |
| Conditions | Standard K8s conditions for controller health monitoring |
| BusinessClassification | Business context for prioritization (if populated) |

**New scenarios**:

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| E2E-SP-163-001 | SP populates processing timestamps (StartTime, CompletionTime) for audit trail compliance | Pending |
| E2E-SP-163-002 | SP records Rego-determined severity and policy hash for downstream prioritization and versioning | Pending |
| E2E-SP-163-003 | SP distinguishes predictive vs reactive signal modes for routing differentiation | Pending |
| E2E-SP-163-004 | SP populates RecoveryContext (PreviousRemediationID, AttemptCount) for consecutive failure escalation | Pending |
| E2E-SP-163-005 | SP records exact conditions (Ready=True, EnrichmentComplete=True, ClassificationComplete=True, ProcessingComplete=True) for health monitoring | Pending |

**Deferred fields** (not covered by new scenarios):
- `BusinessClassification`: Only populated when enterprise business-context labels are present on the namespace. Default E2E namespaces do not carry these labels. Deferring until enterprise configuration E2E environment is available.

---

### 5.2 AIAnalysis (AA)

**Existing E2E scenarios** (`test/e2e/aianalysis/` -- 8 test files):
- Production incident 4-phase cycle (03_full_flow)
- Approval required for production (BR-AI-013)
- Staging auto-approve
- Recovery escalation with 3+ attempts (04_recovery_flow)
- Data quality warnings (BR-AI-011)
- Failed with NeedsHumanReview (BR-HAPI-197)
- Predictive OOMKill (07_predictive_signal_mode)
- Session-based async flow (08_session_async_flow)
- DetectedLabels and PDB detection (09_detected_labels)
- Audit trail completeness (05_audit_trail, 06_error_audit_trail)
- Metrics (02_metrics)

**Fields already validated**: Phase (Completed, Failed), ApprovalRequired, ApprovalReason, ApprovalContext, SelectedWorkflow, InvestigationID, InvestigationSession, PostRCAContext (DetectedLabels, SetAt), Conditions (partial), Warnings, Reason/SubReason/Message (Failed)

**Fields NOT validated (gap)**:

| Field | Why It Matters |
|-------|----------------|
| RootCauseAnalysis (Summary, Severity, ContributingFactors, AffectedResource) | Core RCA output for operator review and decision making |
| TotalAnalysisTime | SLA monitoring and performance tracking |
| AlternativeWorkflows | Multi-candidate transparency for operator decision support |
| ValidationAttemptsHistory | Diagnostic transparency for retry/validation failures |
| DegradedMode | Indicates HolmesGPT unavailability for operational awareness |
| ObservedGeneration | Resource version tracking for controller correctness |

**New scenarios**:

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| E2E-AA-163-001 | AA produces structured root cause analysis (Summary, Severity, AffectedResource) for operator review | Pending |
| E2E-AA-163-002 | AA records total analysis time (non-zero, wall-clock dependent) for SLA monitoring | Pending |
| E2E-AA-163-003 | AA surfaces exact alternative workflows (count, names, confidence ordering) for operator decision support | Pending |
| E2E-AA-163-004 | AA tracks validation attempts for diagnostic transparency on retry failures | Pending |

**Deferred fields** (not covered by new scenarios):
- `DegradedMode`: Requires HolmesGPT to be unavailable during E2E. Current E2E infrastructure always deploys a healthy mock HolmesGPT. Deferring until a "degraded HolmesGPT" E2E environment variant is supported.
- `ObservedGeneration`: Standard Kubernetes controller-runtime field automatically managed by the framework. Low business value for explicit E2E validation; controller correctness is already validated by phase transitions.

---

### 5.3 WorkflowExecution (WE)

**Existing E2E scenarios** (`test/e2e/workflowexecution/` -- 3 test files):
- BR-WE-001: Completion lifecycle (Tekton + Job)
- BR-WE-004: Failure details on intentional failure
- BR-WE-006: Conditions (ExecutionCreated, ExecutionComplete, AuditRecorded)
- BR-WE-010: Cooldown without CompletionTime
- BR-WE-005: K8s events, external PipelineRun deletion
- BR-WE-007: External deletion handling
- BR-WE-008: Metrics
- BR-WE-014: Job backend (success, failure, status sync, spec correctness, naming)

**Fields already validated**: Phase (Running, Completed, Failed), CompletionTime, StartTime, Duration, ExecutionRef, FailureDetails (Message, Reason), Conditions (ExecutionCreated, ExecutionComplete, AuditRecorded)

**Fields NOT validated (gap)**:

| Field | Why It Matters |
|-------|----------------|
| ExecutionStatus (Status, CompletedTasks, TotalTasks) | Runtime progress monitoring for long-running workflows |
| BlockClearance (ClearedBy, ClearReason, ClearMethod) | SOC 2 attribution for approval/clearance decisions |
| FailureDetails sub-fields (ExitCode, FailedTaskIndex, FailedTaskName, FailedStepName, NaturalLanguageSummary) | Comprehensive post-mortem details beyond Message/Reason |

**New scenarios**:

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| E2E-WE-163-001 | WE records execution runtime status (Status, CompletedTasks, TotalTasks) for progress monitoring | Pending |
| E2E-WE-163-002 | WE captures comprehensive failure details (ExitCode, FailedTaskIndex, FailedTaskName, FailedStepName, NaturalLanguageSummary) for post-mortem | Pending |
| E2E-WE-163-003 | WE records block clearance attribution (ClearedBy, ClearReason) for SOC 2 audit trail | Pending |

---

### 5.4 NotificationRequest (NT)

**Existing E2E scenarios** (`test/e2e/notification/` -- 2 test files):
- Full notification lifecycle: Create NR -> Phase=Sent -> audit events (01_notification_lifecycle_audit)
- Failed delivery: Email channel failure -> audit, multi-channel partial failure (04_failed_delivery_audit)

**Fields already validated**: Phase (Sent), DeliveryAttempts (presence and count)

**Fields NOT validated (gap)**:

| Field | Why It Matters |
|-------|----------------|
| QueuedAt, ProcessingStartedAt, CompletionTime | Audit compliance requires complete delivery timeline |
| TotalAttempts, SuccessfulDeliveries, FailedDeliveries | SLA reporting requires accurate delivery counters |
| DeliveryAttempt sub-fields (Channel, Status, DurationSeconds) | Troubleshooting requires per-channel delivery details |
| Conditions | Standard K8s conditions for controller health monitoring |
| Reason, Message | Operator investigation requires failure explanation |

**New scenarios**:

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| E2E-NT-163-001 | NT records complete delivery timeline (QueuedAt < ProcessingStartedAt < CompletionTime) for audit compliance | Pending |
| E2E-NT-163-002 | NT provides accurate delivery counts (TotalAttempts, SuccessfulDeliveries, FailedDeliveries) for SLA reporting | Pending |
| E2E-NT-163-003 | NT captures exact per-channel delivery: Channel=="console", Status=="success", DurationSeconds > 0 (wall-clock) | Pending |
| E2E-NT-163-004 | NT records exact conditions: Ready=True (Reason="Ready"), RoutingResolved=True (Reason="RoutingRuleMatched" or "RoutingFallback") | Pending |
| E2E-NT-163-005 | NT provides failure explanation (Phase=Failed, Reason, Message) for operator investigation | Pending |

---

### 5.5 EffectivenessAssessment (EA)

**Existing E2E scenarios** (`test/e2e/effectivenessmonitor/` -- 5 test files):
- Full pipeline assessment with 4 components (lifecycle_e2e)
- Health score 0.0 for missing Pod (health_e2e)
- Spec drift detection and audit (spec_drift_e2e)
- Alert resolution: resolved (score 1.0) and active (score 0.0) (alert_e2e)
- Metrics and operational behavior (metrics_e2e, operational_e2e)

**Fields already validated**: Phase (Completed), Components (HealthAssessed, HashComputed, PostRemediationSpecHash, CurrentSpecHash, HealthScore, AlertScore, MetricsScore, AlertAssessed, MetricsAssessed), AssessmentReason, CompletedAt

**Fields NOT validated (gap)**:

| Field | Why It Matters |
|-------|----------------|
| ValidityDeadline, PrometheusCheckAfter, AlertManagerCheckAfter | Operational visibility into assessment scheduling |
| Conditions | Standard K8s conditions for controller health monitoring |
| Message | Assessment explanation for operator understanding |

**New scenarios**:

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| E2E-EA-163-001 | EA records assessment scheduling fields (ValidityDeadline, PrometheusCheckAfter) for operational visibility | Pending |
| E2E-EA-163-002 | EA records exact conditions (AssessmentComplete=True, with expected Reason) for controller health monitoring | Pending |
| E2E-EA-163-003 | EA provides failure explanation (Phase=Failed, Message) when assessment cannot complete | Pending |

---

### 5.6 RemediationRequest (RR) -- via RO E2E

**Existing E2E scenarios** (`test/e2e/remediationorchestrator/` -- 12 test files):
- Full remediation lifecycle (lifecycle_e2e)
- RAR audit trail (approval_e2e)
- EA creation (ea_creation_e2e)
- Completion notification (completion_notification_e2e)
- Needs human review (needs_human_review_e2e)
- Predictive signal mode (predictive_signal_mode_e2e)
- Scope blocking and duplicate blocking (scope_blocking_e2e, blocking_e2e)
- Routing cooldown (routing_cooldown_e2e)
- Notification cascade (notification_cascade_e2e)
- Audit wiring, metrics, webhook (audit_wiring_e2e, metrics_e2e, gap8_webhook)

**Note**: RO E2E tests mock child CRD statuses to control reconciler paths. This is by design -- the subject under test is the RO controller, not all services. New scenarios follow this pattern.

**Fields already validated**: OverallPhase (Completed), child CRD refs (SP, AA, WE, NT, EA), Outcome, SelectedWorkflowRef

**Fields NOT validated (gap)**:

| Field | Why It Matters |
|-------|----------------|
| ProcessingStartTime, AnalyzingStartTime, ExecutingStartTime | Latency monitoring per pipeline phase |
| DuplicateOf (top-level field), Deduplication.OccurrenceCount, Deduplication.FirstSeenAt | Duplicate signal transparency for operators |
| BlockReason, BlockMessage, NextAllowedExecution | Operator investigation of blocked remediations |
| SkipReason, SkipMessage | Visibility into why a remediation was bypassed |
| FailurePhase, FailureReason, ConsecutiveFailureCount | Post-mortem for failed remediations |
| Outcome (success/failure/timeout) | Pipeline result reporting |
| PreRemediationSpecHash | Spec drift detection baseline |
| Conditions | Standard K8s conditions for controller health monitoring |
| TimeoutConfig | Timeout configuration visibility |

**New scenarios**:

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| E2E-RO-163-001 | RR tracks per-phase timestamps (ProcessingStartTime, AnalyzingStartTime, ExecutingStartTime) for latency monitoring | Pending |
| E2E-RO-163-002 | RR records exact deduplication data (Deduplication.OccurrenceCount == 2, Deduplication.FirstSeenAt, DuplicateOf == original RR name) for transparency | Pending |
| E2E-RO-163-003 | RR captures blocking reason (BlockReason, BlockMessage, NextAllowedExecution) for operator investigation | Pending |
| E2E-RO-163-004 | RR records skip reason (SkipReason, SkipMessage) when remediation is bypassed | Pending |
| E2E-RO-163-005 | RR records failure details (FailurePhase, FailureReason, ConsecutiveFailureCount) for post-mortem | Pending |
| E2E-RO-163-006 | RR populates Outcome == "success" for completed pipeline, tested alongside failure and timeout variants | Pending |
| E2E-RO-163-007 | RR records exact conditions: SignalProcessingComplete=True, AIAnalysisComplete=True, WorkflowExecutionComplete=True, Ready=True | Pending |

**Deferred fields** (not covered by new scenarios):
- `PreRemediationSpecHash`: Populated during the Executing phase to snapshot the workload spec before remediation. Already validated indirectly by the EA spec-drift E2E tests which depend on this hash being present. Adding an explicit RO E2E assertion would duplicate EA coverage without new business insight.
- `TimeoutConfig`: A read-only reflection of the controller's timeout configuration. Validating this in E2E confirms config propagation, not business behavior. Deferring in favor of scenarios that test timeout *effects* (e.g., E2E-RO-163-006 timeout variant).

---

### 5.7 RemediationApprovalRequest (RAR) -- via RO E2E

**Existing E2E scenarios** (fullpipeline `02_approval_lifecycle_test.go` + RO `approval_e2e_test.go`):
- Decision=Approved with DecidedBy, DecidedAt, CreatedAt
- Not expired (Expired=false)

**Fields already validated**: Decision, DecidedBy, DecidedAt, CreatedAt, Expired (false)

**Fields NOT validated (gap)**:

| Field | Why It Matters |
|-------|----------------|
| DecisionMessage | Approval rationale for audit trail and operator review |
| Conditions (ApprovalPending, ApprovalDecided, Ready, AuditRecorded) | Full lifecycle tracking for monitoring and compliance |
| Expired=true | Timeout handling path validation |
| TimeRemaining | Pending state visibility for operators |

**New scenarios**:

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| E2E-RAR-163-001 | RAR captures approval rationale (DecisionMessage) for audit trail | Pending |
| E2E-RAR-163-002 | RAR tracks exact conditions: ApprovalPending=False, ApprovalDecided=True, Ready=True, AuditRecorded=True (approved path) | Pending |
| E2E-RAR-163-003 | RAR correctly marks expired approvals (Expired=true) for timeout handling | Pending |

**Deferred fields** (not covered by new scenarios):
- `TimeRemaining`: A computed field that decreases as the approval TTL counts down. Validating exact values is non-deterministic (wall-clock dependent) and would require precise timing control. The business outcome (expiry) is already covered by E2E-RAR-163-003; `TimeRemaining` adds no additional correctness signal.

---

## 6. Test Cases (Detail)

### E2E-SP-163-001: Processing Timestamps

**BR**: BR-SP-001
**Type**: E2E
**File**: `test/e2e/signalprocessing/business_requirements_test.go` (extend existing)

**Given**: A Prometheus alert fires for a Pod in the `production` namespace
**When**: SignalProcessing completes enrichment
**Then**: `Status.StartTime` is populated and <= `Status.CompletionTime`, both non-nil

**Acceptance Criteria**:
- StartTime is populated (HaveValue matcher, not BeNil)
- CompletionTime is populated (HaveValue matcher, not BeNil)
- StartTime <= CompletionTime (temporal ordering -- wall-clock dependent, exact values not deterministic)

---

### E2E-SP-163-002: Severity and PolicyHash

**BR**: BR-SP-070
**Type**: E2E
**File**: `test/e2e/signalprocessing/business_requirements_test.go` (extend existing)

**Given**: A critical alert fires in the `production` namespace with Rego policies loaded (which map production + critical alert to severity "critical")
**When**: SignalProcessing completes Rego evaluation
**Then**: `Status.Severity` == "critical" (deterministic from controlled Rego policy + input) and `Status.PolicyHash` matches the SHA256 of the deployed policy ConfigMap

**Acceptance Criteria**:
- Severity == "critical" (exact match for production namespace + critical alert input)
- PolicyHash == SHA256 of the deployed Rego policy ConfigMap content (deterministic)

---

### E2E-SP-163-003: Signal Mode Classification

**BR**: BR-SP-100
**Type**: E2E
**File**: `test/e2e/signalprocessing/business_requirements_test.go` (new context)

**Given**: A predictive OOMKill signal is ingested (alert with `signal_mode: predictive` label)
**When**: SignalProcessing completes classification
**Then**: `Status.SignalMode` == "predictive", `Status.SignalType` == the classified type for OOMKill, `Status.OriginalSignalType` == the original alert type before reclassification

**Acceptance Criteria**:
- SignalMode == "predictive" (exact match for predictive input signal)
- SignalType == expected classified type (exact, determined by classification logic for OOMKill)
- OriginalSignalType == original alert type from the ingested signal (exact)

---

### E2E-SP-163-004: Recovery Context

**BR**: BR-SP-001
**Type**: E2E
**File**: `test/e2e/signalprocessing/business_requirements_test.go` (new context)

**Given**: A signal arrives for a resource that has exactly 1 previous failed remediation (RR name known from test setup)
**When**: SignalProcessing completes with recovery context
**Then**: `Status.RecoveryContext` is populated with the exact previous RR name and AttemptCount == 1

**Acceptance Criteria**:
- RecoveryContext is not nil
- RecoveryContext.PreviousRemediationID == name of the previous failed RR (exact, from test setup)
- RecoveryContext.AttemptCount == 1 (exact: one prior failed attempt)

---

### E2E-SP-163-005: Standard Conditions

**BR**: ADR-032
**Type**: E2E
**File**: `test/e2e/signalprocessing/business_requirements_test.go` (extend existing)

**Given**: A completed SignalProcessing (from an existing happy-path scenario)
**When**: Phase reaches Completed
**Then**: All expected conditions are present with Status=True

**Acceptance Criteria**:
- Conditions contains `Ready` with Status="True"
- Conditions contains `EnrichmentComplete` with Status="True"
- Conditions contains `ClassificationComplete` with Status="True"
- Conditions contains `ProcessingComplete` with Status="True"
- No conditions have Status="False" (happy path, no degraded enrichment)
- Note: Condition type names from `pkg/signalprocessing/conditions.go`

---

### E2E-NT-163-001: Delivery Timeline

**BR**: BR-NOT-069
**Type**: E2E
**File**: `test/e2e/notification/01_notification_lifecycle_audit_test.go` (extend existing)

**Given**: A NotificationRequest is created for Console delivery
**When**: Notification reaches Phase=Sent
**Then**: `QueuedAt` < `ProcessingStartedAt` < `CompletionTime` (temporal ordering)

**Acceptance Criteria**:
- QueuedAt is populated (HaveValue matcher)
- ProcessingStartedAt is populated (HaveValue matcher)
- CompletionTime is populated (HaveValue matcher)
- QueuedAt.Before(ProcessingStartedAt) (temporal ordering)
- ProcessingStartedAt.Before(CompletionTime) (temporal ordering)
- Note: Timestamp exact values are wall-clock dependent; temporal ordering is the correctness assertion

---

### E2E-NT-163-002: Delivery Counters

**BR**: BR-NOT-069
**Type**: E2E
**File**: `test/e2e/notification/01_notification_lifecycle_audit_test.go` (extend existing)

**Given**: A NotificationRequest is created targeting exactly 1 Console channel (no retries configured)
**When**: Notification reaches Phase=Sent
**Then**: Delivery counters reflect exactly 1 successful delivery to 1 channel with 0 failures

**Acceptance Criteria**:
- TotalAttempts == 1 (exact: 1 channel, 1 attempt, no retries)
- SuccessfulDeliveries == 1 (exact: Console delivery succeeded)
- FailedDeliveries == 0 (exact: no failures in happy path)

---

### E2E-NT-163-003: Per-Channel Delivery Details

**BR**: BR-NOT-069
**Type**: E2E
**File**: `test/e2e/notification/01_notification_lifecycle_audit_test.go` (extend existing)

**Given**: A NotificationRequest is created targeting exactly 1 Console channel
**When**: Notification reaches Phase=Sent
**Then**: DeliveryAttempts contains exactly 1 entry with exact per-channel details

**Acceptance Criteria**:
- DeliveryAttempts has exactly 1 entry (exact: 1 channel configured)
- DeliveryAttempts[0].Channel == "console" (exact: from `ChannelConsole` constant)
- DeliveryAttempts[0].Status == "success" (exact: from `pkg/notification/delivery/orchestrator.go`)
- DeliveryAttempts[0].DurationSeconds > 0 (wall-clock dependent, cannot assert exact value)
- Note: Status values from codebase are lowercase: "success", "failed" (not capitalized)

---

### E2E-NT-163-005: Failure Explanation

**BR**: BR-NOT-069
**Type**: E2E
**File**: `test/e2e/notification/04_failed_delivery_audit_test.go` (extend existing)

**Given**: A NotificationRequest targets only an unreachable Email channel (no other channels configured)
**When**: All delivery attempts fail
**Then**: Phase == "Failed", `Reason` contains the specific failure reason code, `Message` contains the channel name and error detail

**Acceptance Criteria**:
- Phase == "Failed" (exact)
- Reason == expected failure reason code (e.g., "DeliveryFailed" -- exact, determined by controller logic for exhausted retries)
- Message contains "email" (channel name) and describes the connection/delivery error (ContainSubstring assertion)

---

### E2E-RO-163-001: Phase Timestamps

**BR**: BR-ORCH-025
**Type**: E2E
**File**: `test/e2e/remediationorchestrator/lifecycle_e2e_test.go` (extend existing)

**Given**: A RemediationRequest progresses through Processing -> Analyzing -> Executing -> Completed
**When**: RR reaches OverallPhase=Completed
**Then**: Phase timestamps are populated: `ProcessingStartTime`, `AnalyzingStartTime`, `ExecutingStartTime` all non-nil, in temporal order

**Acceptance Criteria**:
- ProcessingStartTime is populated (HaveValue matcher)
- AnalyzingStartTime is populated (HaveValue matcher)
- ExecutingStartTime is populated (HaveValue matcher)
- ProcessingStartTime <= AnalyzingStartTime <= ExecutingStartTime (temporal ordering)
- Note: Exact timestamp values are wall-clock dependent; temporal ordering is the correctness assertion

---

### E2E-RO-163-005: Failure Post-Mortem

**BR**: BR-ORCH-025
**Type**: E2E
**File**: `test/e2e/remediationorchestrator/` (new test file or extend existing)

**Given**: A RemediationRequest where the WorkflowExecution fails (first failure for this resource, no prior failures)
**When**: RO handles the failure and transitions to Failed
**Then**: `FailurePhase` == "Executing", `FailureReason` matches the WE failure reason, `ConsecutiveFailureCount` == 1

**Acceptance Criteria**:
- OverallPhase == "Failed" (exact)
- FailurePhase == "Executing" (exact: WE is the failing phase in this scenario)
- FailureReason == expected reason string from the WE failure (exact or ContainSubstring for the WE error)
- ConsecutiveFailureCount == 1 (exact: first failure, no prior consecutive failures)

---

### E2E-RAR-163-003: Approval Expiry

**BR**: ADR-040
**Type**: E2E
**File**: `test/e2e/remediationorchestrator/approval_e2e_test.go` (new context)

**Given**: A RemediationApprovalRequest is created with a short expiry window (e.g., 5s TTL)
**When**: The expiry deadline passes without a decision
**Then**: `Expired` == true, `Decision` == "Expired" (or the exact expiry decision value from the controller), and the parent RR transitions to "Failed"

**Acceptance Criteria**:
- Expired == true (exact)
- Decision == "Expired" (exact: the controller sets this when TTL expires without a decision)
- Parent RR OverallPhase == "Failed" (exact: expiry is a terminal failure)

---

## 7. Test Infrastructure

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for integration/E2E (see [No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Infrastructure**: Kind cluster with all controllers deployed (per-suite E2E suites)
- **Validators**: Extended `test/shared/validators/crd_status.go` with functional options
- **Location**: Existing `test/e2e/{service}/` directories

### Validator Extension Pattern

```go
type SPValidationOption func(*spValidationConfig)

type spValidationConfig struct {
    expectDegradedMode    bool
    expectRecoveryContext bool
    expectPredictive      bool
    expectSeverity        string
}

func WithDegradedMode() SPValidationOption {
    return func(c *spValidationConfig) { c.expectDegradedMode = true }
}

func WithRecoveryContext() SPValidationOption {
    return func(c *spValidationConfig) { c.expectRecoveryContext = true }
}

func WithPredictiveSignalMode() SPValidationOption {
    return func(c *spValidationConfig) { c.expectPredictive = true }
}

func ValidateSPStatus(sp *v1.SignalProcessing, opts ...SPValidationOption) []string {
    cfg := &spValidationConfig{}
    for _, opt := range opts {
        opt(cfg)
    }
    var failures []string
    // base checks (always validated)
    // scenario-specific checks (based on cfg)
    return failures
}
```

---

## 8. Execution

```bash
# Run per-suite E2E tests
make test-e2e-signalprocessing
make test-e2e-aianalysis
make test-e2e-workflowexecution
make test-e2e-notification
make test-e2e-effectivenessmonitor
make test-e2e-remediationorchestrator

# Run specific test by ID
go test ./test/e2e/signalprocessing/... --ginkgo.focus="E2E-SP-163-001"
go test ./test/e2e/notification/... --ginkgo.focus="E2E-NT-163-001"
```

### Execution Order (by gap severity)

1. **RR** (25+ uncovered fields) -- highest impact
2. **NT** (9 uncovered) -- high gap
3. **SP** (10 uncovered) -- medium gap
4. **AA** (6 uncovered) -- medium gap
5. **WE** (3 uncovered) -- low gap
6. **EA** (3 uncovered) -- low gap
7. **RAR** (5 uncovered) -- low gap (partially covered by fullpipeline)

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-22 | Initial test plan: 30 scenarios across 7 CRDs, gap analysis, validator extension design |
| 1.1 | 2026-02-22 | Triage pass 1: Replaced all weak assertions (>=, non-empty, at-least-one) with exact expected values per controlled inputs. Added Deterministic Assertion Policy. |
| 1.2 | 2026-02-22 | Triage pass 2: Fixed condition type names to match codebase constants (SP: Ready/EnrichmentComplete/ClassificationComplete/ProcessingComplete; NT: Ready/RoutingResolved; RR: SignalProcessingComplete/AIAnalysisComplete/WorkflowExecutionComplete). Fixed DuplicateOf nesting (top-level, not under Deduplication). Added E2E-NT-163-003 detail test case with exact values (Channel=="console", Status=="success"). Added WE FailureDetails sub-fields (FailedTaskName, FailedStepName). Added deferred rationale for 6 gap fields (BusinessClassification, DegradedMode, ObservedGeneration, PreRemediationSpecHash, TimeoutConfig, TimeRemaining). |
