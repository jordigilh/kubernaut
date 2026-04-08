# Custom Resources (CRDs)

Kubernaut API reference for all Custom Resource Definitions.

API Group: `kubernaut.ai/v1alpha1`


## AIAnalysis


AIAnalysis is the Schema for the aianalyses API.


| Field| Type| Description|
| ---| ---| ---|
| `apiVersion`| _string_| `kubernaut.ai/v1alpha1`|
| `kind`| _string_| `AIAnalysis`|
| `metadata`| _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectmeta-v1-meta)_| Refer to the Kubernetes API documentation for fields of `metadata`.|
| `spec`| _[AIAnalysisSpec](#aianalysisspec)_||
| `status`| _[AIAnalysisStatus](#aianalysisstatus)_||


### AIAnalysisReason

_Underlying type:_ _string_

AIAnalysisReason represents the umbrella failure or completion reason.

_Appears in:_
- [AIAnalysisStatus](#aianalysisstatus)

_Validation:_
- Enum: [AnalysisCompleted WorkflowResolutionFailed WorkflowNotNeeded NoWorkflowSelected RegoEvaluationError TransientError APIError]

| Value| Description|
| ---| ---|
| `AnalysisCompleted`||
| `WorkflowResolutionFailed`||
| `WorkflowNotNeeded`||
| `NoWorkflowSelected`||
| `RegoEvaluationError`||
| `TransientError`||
| `APIError`||


### AIAnalysisSpec


AIAnalysisSpec defines the desired state of AIAnalysis.

 Spec Immutability
AIAnalysis represents an immutable event (AI investigation).
Once created by RemediationOrchestrator, spec cannot be modified to ensure:
- Audit trail integrity (AI investigation matches original RCA request)
- No tampering with RCA targets post-HAPI validation
- No workflow selection modification after AI recommendation

To re-analyze, delete and recreate the AIAnalysis CRD.

_Appears in:_
- [AIAnalysis](#aianalysis)

| Field| Type| Description|
| ---| ---| ---|
| `remediationRequestRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_| Reference to parent RemediationRequest CRD for audit trail|
| `remediationId`| _string_| Remediation ID for audit correlation|
| `analysisRequest`| _[AnalysisRequest](#analysisrequest)_| Complete analysis request with structured context|
| `timeoutConfig`| _[AIAnalysisTimeoutConfig](#aianalysistimeoutconfig)_| TIMEOUT CONFIGURATION <br />Replaces deprecated annotation-based timeout (security + validation)<br />Passed through from RR.Status.TimeoutConfig.AIAnalysisTimeout by RO ( moved to Status)<br />Optional timeout configuration for this analysis<br />If nil, AIAnalysis controller uses defaults (Investigating: 60s, Analyzing: 5s)|


### AIAnalysisStatus


AIAnalysisStatus defines the observed state of AIAnalysis.

_Appears in:_
- [AIAnalysis](#aianalysis)

| Field| Type| Description|
| ---| ---| ---|
| `observedGeneration`| _integer_| ObservedGeneration is the most recent generation observed by the controller.<br />Used to prevent duplicate reconciliations and ensure idempotency.<br />Per Standard pattern for all Kubernetes controllers.|
| `phase`| _string_| Phase tracking (no "Approving" or "Recommending" phase - simplified 4-phase flow)|
| `message`| _string_||
| `reason`| _[AIAnalysisReason](#aianalysisreason)_| Reason provides the umbrella failure or completion category.|
| `subReason`| _string_| SubReason provides specific failure cause within the Reason category<br /> Maps to needs_human_review triggers from HolmesGPT-API<br /> Added InvestigationInconclusive, ProblemResolved for new investigation outcomes|
| `startedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| Timestamps|
| `completedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_||
| `rootCause`| _string_| Identified root cause|
| `rootCauseAnalysis`| _[RootCauseAnalysis](#rootcauseanalysis)_| Root cause analysis details|
| `selectedWorkflow`| _[SelectedWorkflow](#selectedworkflow)_| Selected workflow for execution (populated when phase=Completed)|
| `alternativeWorkflows`| _[AlternativeWorkflow](#alternativeworkflow) array_| ALTERNATIVE WORKFLOWS <br />Alternative workflows considered but not selected.<br />INFORMATIONAL ONLY - NOT for automatic execution.<br />Helps operators make informed approval decisions and provides audit trail.<br />Per HolmesGPT-API team: Alternatives are for CONTEXT, not EXECUTION.|
| `approvalRequired`| _boolean_| True if approval is required (confidence < 80% or policy requires)|
| `approvalReason`| _string_| Reason why approval is required (when ApprovalRequired=true)|
| `approvalContext`| _[ApprovalContext](#approvalcontext)_| Rich context for approval notification|
| `needsHumanReview`| _boolean_| Set by HAPI when AI cannot produce reliable result<br />True if human review required (HAPI decision: RCA incomplete/unreliable)<br /> Triggers NotificationRequest creation in RO<br />BR-496 v2: Set when root_owner missing (rca_incomplete) or validation/confidence issues.|
| `humanReviewReason`| _string_| Reason why human review needed (when NeedsHumanReview=true)<br /> Maps to HAPI's human_review_reason enum values|
| `actionability`| _string_| #388: LLM's assessment of whether the alert warrants action.<br />Empty when not yet assessed (pre-investigation or error paths).<br />"Actionable" when the LLM determines the alert warrants action (default for all processed alerts).<br />"NotActionable" when the LLM determines the alert is benign (e.g., orphaned PVCs).|
| `investigationId`| _string_| HolmesGPT investigation ID for correlation|
| `investigationTime`| _integer_| Investigation duration in seconds|
| `warnings`| _string array_| Non-fatal warnings from HolmesGPT-API (e.g., low confidence)|
| `validationAttemptsHistory`| _[ValidationAttempt](#validationattempt) array_| ValidationAttemptsHistory contains complete history of all HAPI validation attempts<br />Per HAPI retries up to 3 times with LLM self-correction<br />This field provides audit trail for operator notifications and debugging|
| `degradedMode`| _boolean_| DegradedMode indicates if the analysis ran with degraded capabilities<br />(e.g., Rego policy evaluation failed, using safe defaults)|
| `totalAnalysisTime`| _integer_| TotalAnalysisTime is the total duration of the analysis in seconds|
| `consecutiveFailures`| _integer_| ConsecutiveFailures tracks retry attempts for exponential backoff<br /> Reset to 0 on success, increment on transient failure<br />Used with for retry logic with jitter|
| `investigationSession`| _[InvestigationSession](#investigationsession)_| Tracks the async submit/poll session with HAPI<br />InvestigationSession tracks the async HAPI session for submit/poll pattern|
| `postRCAContext`| _[PostRCAContext](#postrcacontext)_| Runtime-computed cluster characteristics from HAPI<br />PostRCAContext holds data computed by HAPI after RCA (e.g., DetectedLabels).<br />Immutable once set — use CEL validation on the PostRCAContext type.|
| `conditions`| _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#condition-v1-meta) array_| Conditions|


### AIAnalysisTimeoutConfig


AIAnalysisTimeoutConfig defines timeout settings for AIAnalysis phases


_Appears in:_
- [AIAnalysisSpec](#aianalysisspec)

| Field| Type| Description|
| ---| ---| ---|
| `investigatingTimeout`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| Timeout for Investigating phase (HolmesGPT-API call)<br />Default: 60s if not specified|
| `analyzingTimeout`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| Timeout for Analyzing phase (Rego policy evaluation)<br />Default: 5s if not specified|


### ActionLink


ActionLink represents an external service action link

_Appears in:_
- [NotificationRequestSpec](#notificationrequestspec)

| Field| Type| Description|
| ---| ---| ---|
| `service`| _[ActionLinkServiceType](#actionlinkservicetype)_| Service name (github, grafana, prometheus, kubernetes-dashboard, etc.)|
| `url`| _string_| Action link URL|
| `label`| _string_| Human-readable label for the link|


### ActionLinkServiceType

_Underlying type:_ _string_


_Appears in:_
- [ActionLink](#actionlink)


| Value| Description|
| ---| ---|
| `grafana`||
| `prometheus`||


## ActionType


ActionType is the Schema for the actiontypes API.
 Kubernetes-native action type taxonomy definition.


| Field| Type| Description|
| ---| ---| ---|
| `apiVersion`| _string_| `kubernaut.ai/v1alpha1`|
| `kind`| _string_| `ActionType`|
| `metadata`| _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectmeta-v1-meta)_| Refer to the Kubernetes API documentation for fields of `metadata`.|
| `spec`| _[ActionTypeSpec](#actiontypespec)_||
| `status`| _[ActionTypeStatus](#actiontypestatus)_||


### ActionTypeDescription


ActionTypeDescription provides structured information about an action type.

_Appears in:_
- [ActionTypeSpec](#actiontypespec)

| Field| Type| Description|
| ---| ---| ---|
| `what`| _string_| What describes what this action type concretely does.|
| `whenToUse`| _string_| WhenToUse describes conditions under which this action type is appropriate.|
| `whenNotToUse`| _string_| WhenNotToUse describes specific exclusion conditions.|
| `preconditions`| _string_| Preconditions describes conditions that must be verified before use.|


### ActionTypeSpec


ActionTypeSpec defines the desired state of ActionType.
 ActionType CRD lifecycle management.

_Appears in:_
- [ActionType](#actiontype)

| Field| Type| Description|
| ---| ---| ---|
| `name`| _string_| Name is the PascalCase action type identifier (e.g., RestartPod, ScaleReplicas).<br />Immutable after creation.|
| `description`| _[ActionTypeDescription](#actiontypedescription)_| Description provides structured information about the action type.<br />Only this field is mutable after creation.|


### ActionTypeStatus


ActionTypeStatus defines the observed state of ActionType.

_Appears in:_
- [ActionType](#actiontype)

| Field| Type| Description|
| ---| ---| ---|
| `registered`| _boolean_| Registered indicates whether the action type has been successfully registered in the DS catalog.|
| `registeredAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| RegisteredAt is the timestamp of initial registration in the catalog.|
| `registeredBy`| _string_| RegisteredBy is the identity of the registrant (K8s SA or user).|
| `previouslyExisted`| _boolean_| PreviouslyExisted indicates if this action type was re-enabled after being disabled.|
| `activeWorkflowCount`| _integer_| ActiveWorkflowCount is the number of active RemediationWorkflows referencing this action type.<br />Best-effort, updated asynchronously by the RW admission webhook handler.|
| `catalogStatus`| _[CatalogStatus](#catalogstatus)_| CatalogStatus reflects the DS catalog lifecycle state.|


### AlternativeApproach


AlternativeApproach describes an alternative approach with pros/cons

_Appears in:_
- [ApprovalContext](#approvalcontext)

| Field| Type| Description|
| ---| ---| ---|
| `approach`| _string_| Approach description|
| `prosCons`| _string_| ProsCons analysis|


### AlternativeWorkflow


AlternativeWorkflow contains alternative workflows considered but not selected.
INFORMATIONAL ONLY - NOT for automatic execution.
Helps operators understand AI reasoning during approval decisions.

_Appears in:_
- [AIAnalysisStatus](#aianalysisstatus)

| Field| Type| Description|
| ---| ---| ---|
| `workflowId`| _string_| Workflow identifier (catalog lookup key)|
| `executionBundle`| _string_| Execution bundle OCI reference (digest-pinned) - resolved by HolmesGPT-API|
| `confidence`| _float_| Confidence score (0.0-1.0) - shows why it wasn't selected|
| `rationale`| _string_| Rationale explaining why this workflow was considered|


### AnalysisContext


AnalysisContext captures AI analysis results.

_Appears in:_
- [NotificationContext](#notificationcontext)

| Field| Type| Description|
| ---| ---| ---|
| `approvalReason`| _string_| ApprovalReason explains why approval was required.|
| `rootCause`| _string_| RootCause is the AI-determined root cause summary.|
| `outcome`| _string_| Outcome is the remediation outcome (e.g., "Success", "Failed").|


### AnalysisRequest


AnalysisRequest contains the structured analysis request
 Self-contained context for AIAnalysis

_Appears in:_
- [AIAnalysisSpec](#aianalysisspec)

| Field| Type| Description|
| ---| ---| ---|
| `signalContext`| _[SignalContextInput](#signalcontextinput)_| Signal context from SignalProcessing enrichment|
| `analysisTypes`| _[AnalysisType](#analysistype) array_| Analysis types to perform|


### AnalysisType

_Underlying type:_ _string_

AnalysisType represents a type of analysis to perform.

_Appears in:_
- [AnalysisRequest](#analysisrequest)

_Validation:_
- Enum: [Investigation RootCause WorkflowSelection]

| Value| Description|
| ---| ---|
| `Investigation`||
| `RootCause`||
| `WorkflowSelection`||


### ApprovalAlternative


ApprovalAlternative describes an alternative approach with pros/cons

_Appears in:_
- [RemediationApprovalRequestSpec](#remediationapprovalrequestspec)

| Field| Type| Description|
| ---| ---| ---|
| `approach`| _string_| Alternative approach description|
| `prosCons`| _string_| Pros and cons analysis|


### ApprovalContext


ApprovalContext contains rich context for approval notifications

_Appears in:_
- [AIAnalysisStatus](#aianalysisstatus)

| Field| Type| Description|
| ---| ---| ---|
| `reason`| _string_| Reason why approval is required|
| `confidenceScore`| _float_| ConfidenceScore from AI analysis (0.0-1.0)|
| `confidenceLevel`| _string_| ConfidenceLevel: "low" \| "medium" \| "high"|
| `investigationSummary`| _string_| InvestigationSummary from HolmesGPT analysis|
| `evidenceCollected`| _string array_| EvidenceCollected that led to this conclusion|
| `recommendedActions`| _[RecommendedAction](#recommendedaction) array_| RecommendedActions with rationale|
| `alternativesConsidered`| _[AlternativeApproach](#alternativeapproach) array_| AlternativesConsidered with pros/cons|
| `whyApprovalRequired`| _string_| WhyApprovalRequired explains the need for human review|
| `policyEvaluation`| _[PolicyEvaluation](#policyevaluation)_| PolicyEvaluation contains Rego policy evaluation details|


### ApprovalDecision

_Underlying type:_ _string_

ApprovalDecision represents the operator's decision on an approval request

_Appears in:_
- [RemediationApprovalRequestStatus](#remediationapprovalrequeststatus)

_Validation:_
- Enum: [ Approved Rejected Expired]

| Value| Description|
| ---| ---|
| ``| ApprovalDecisionPending indicates no decision has been made yet|
| `Approved`| ApprovalDecisionApproved indicates the operator approved the remediation|
| `Rejected`| ApprovalDecisionRejected indicates the operator rejected the remediation|
| `Expired`| ApprovalDecisionExpired indicates the approval request timed out|


### ApprovalPolicyEvaluation


ApprovalPolicyEvaluation contains Rego policy evaluation results

_Appears in:_
- [RemediationApprovalRequestSpec](#remediationapprovalrequestspec)

| Field| Type| Description|
| ---| ---| ---|
| `policyName`| _string_| Policy name that was evaluated|
| `matchedRules`| _string array_| Rules that matched and triggered approval requirement|
| `decision`| _string_| Policy decision (PascalCase per K8s enum convention, values from PolicyDecision type)|


### ApprovalRecommendedAction


ApprovalRecommendedAction describes a recommended action with rationale

_Appears in:_
- [RemediationApprovalRequestSpec](#remediationapprovalrequestspec)

| Field| Type| Description|
| ---| ---| ---|
| `action`| _string_| Action description|
| `rationale`| _string_| Rationale for this action|


### BlockClearanceDetails


BlockClearanceDetails tracks the clearing of PreviousExecutionFailed blocks
Required for SOC2 CC7.3 (Immutability), CC7.4 (Completeness), CC8.1 (Attribution)
Preserves audit trail when operators clear execution blocks after investigation

_Appears in:_
- [WorkflowExecutionStatus](#workflowexecutionstatus)

| Field| Type| Description|
| ---| ---| ---|
| `clearedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| ClearedAt is the timestamp when the block was cleared|
| `clearedBy`| _string_| ClearedBy is the Kubernetes user who cleared the block<br />Extracted from request context (if available) or annotation value<br />Format: username@domain or service-account:namespace:name<br />Example: "admin@kubernaut.ai" or "service-account:kubernaut-system:operator"|
| `clearReason`| _string_| ClearReason is the operator-provided reason for clearing<br />Required for audit trail accountability<br />Example: "manual investigation complete, cluster state verified"|
| `clearMethod`| _string_| ClearMethod indicates how the block was cleared<br />Annotation: Via kubernaut.ai/clear-execution-block annotation<br />APIEndpoint: Via dedicated clearing API endpoint (future)<br />StatusField: Via direct status field update (future)|


### BlockReason

_Underlying type:_ _string_

BlockReason represents the reason why a RemediationRequest is blocked (non-terminal).

_Appears in:_
- [RemediationRequestStatus](#remediationrequeststatus)

_Validation:_
- Enum: [ConsecutiveFailures DuplicateInProgress ResourceBusy RecentlyRemediated ExponentialBackoff UnmanagedResource IneffectiveChain]

| Value| Description|
| ---| ---|
| `ConsecutiveFailures`| BlockReasonConsecutiveFailures indicates remediation failed 3+ times consecutively.<br />This is a temporary block with a 1-hour cooldown period.|
| `DuplicateInProgress`| BlockReasonDuplicateInProgress indicates another RR with the same fingerprint is active.<br />This prevents Gateway RR flood by keeping the duplicate in non-terminal Blocked state.|
| `ResourceBusy`| BlockReasonResourceBusy indicates another WorkflowExecution is running on the same target.<br />This prevents concurrent modifications to the same Kubernetes resource.|
| `RecentlyRemediated`| BlockReasonRecentlyRemediated indicates the same workflow+target was executed recently.<br />This enforces a cooldown period (default 5 minutes) to prevent redundant executions.|
| `ExponentialBackoff`| BlockReasonExponentialBackoff indicates pre-execution failures require a backoff period.<br />This implements graduated retry for transient infrastructure failures.|
| `UnmanagedResource`| BlockReasonUnmanagedResource indicates the target resource is not managed by Kubernaut.<br />The resource or namespace does not have the kubernaut.ai/managed=true label.<br />RO will retry with exponential backoff (5s → 10s → ... → 5min) until RR times out.|
| `IneffectiveChain`| BlockReasonIneffectiveChain indicates consecutive remediations for the same target<br />have been ineffective (resource keeps reverting or health doesn't improve).<br />Escalates to human review via NotificationRequest.|


### DedupContext


DedupContext captures deduplication context .

_Appears in:_
- [NotificationContext](#notificationcontext)

| Field| Type| Description|
| ---| ---| ---|
| `duplicateCount`| _string_| DuplicateCount is the number of duplicate signals.|


### DeduplicationStatus


DeduplicationStatus tracks signal occurrence for deduplication.
OWNER: Gateway Service (exclusive write access)

_Appears in:_
- [RemediationRequestStatus](#remediationrequeststatus)

| Field| Type| Description|
| ---| ---| ---|
| `firstSeenAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| FirstSeenAt is when this signal fingerprint was first observed|
| `lastSeenAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| LastSeenAt is when this signal fingerprint was last observed|
| `occurrenceCount`| _integer_| OccurrenceCount tracks how many times this signal has been seen|


### DeliveryAttempt


DeliveryAttempt records a single delivery attempt to a channel

_Appears in:_
- [NotificationRequestStatus](#notificationrequeststatus)

| Field| Type| Description|
| ---| ---| ---|
| `channel`| _[DeliveryChannelName](#deliverychannelname)_| Channel name|
| `attempt`| _integer_| Attempt number (1-based)|
| `timestamp`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| Timestamp of this attempt|
| `status`| _[DeliveryAttemptStatus](#deliveryattemptstatus)_| Status of this attempt (success, failed, timeout, invalid)|
| `error`| _string_| Error message if failed|
| `durationSeconds`| _float_| Duration of delivery attempt in seconds|


### DeliveryAttemptStatus

_Underlying type:_ _string_


_Appears in:_
- [DeliveryAttempt](#deliveryattempt)

_Validation:_
- Enum: [success failed timeout invalid]

| Value| Description|
| ---| ---|
| `success`||
| `failed`||
| `timeout`||
| `invalid`||


### DeliveryChannelName

_Underlying type:_ _string_


_Appears in:_
- [DeliveryAttempt](#deliveryattempt)


### EAComponents


EAComponents tracks the completion state and scores of each assessment component.
The EM updates these fields as each component check completes.
This enables restart recovery: if EM restarts mid-assessment, it can skip
already-completed components by checking these flags.

_Appears in:_
- [EffectivenessAssessmentStatus](#effectivenessassessmentstatus)

| Field| Type| Description|
| ---| ---| ---|
| `healthAssessed`| _boolean_| HealthAssessed indicates whether the health check has been completed.|
| `healthScore`| _float_| HealthScore is the health check score (0.0-1.0), nil if not yet assessed.|
| `hashComputed`| _boolean_| HashComputed indicates whether the spec hash comparison has been completed.|
| `postRemediationSpecHash`| _string_| PostRemediationSpecHash is the hash of the target resource spec after remediation.|
| `currentSpecHash`| _string_| CurrentSpecHash is the most recent hash of the target resource spec,<br />re-computed on each reconcile after HashComputed is true .<br />If it differs from PostRemediationSpecHash, spec drift was detected.|
| `alertAssessed`| _boolean_| AlertAssessed indicates whether the alert resolution check has been completed.|
| `alertScore`| _float_| AlertScore is the alert resolution score (0.0 or 1.0), nil if not yet assessed.|
| `metricsAssessed`| _boolean_| MetricsAssessed indicates whether the metric comparison has been completed.|
| `metricsScore`| _float_| MetricsScore is the metric comparison score (0.0-1.0), nil if not yet assessed.|
| `alertDecayRetries`| _integer_| AlertDecayRetries tracks the number of times the EM re-checked a firing alert<br />during decay monitoring. Incremented each reconcile where isAlertDecay returns true.<br />A non-zero value means the EM confirmed the resource was healthy but the alert<br />persisted, indicating Prometheus lookback window decay.|


### EAConfig


EAConfig contains assessment configuration set by the RO at creation time.
StabilizationWindow controls how long the EM waits after remediation before
starting assessment checks. HashComputeDelay and AlertCheckDelay are optional
Duration-based delays that the RO computes based on target type and signal mode.
All other assessment parameters (PrometheusEnabled, AlertManagerEnabled,
ValidityWindow) are EM-internal configuration read from effectivenessmonitor.Config.
The EM emits individual component audit events to DataStorage; the overall
effectiveness score is computed by DataStorage on demand, not by the EM.

_Appears in:_
- [EffectivenessAssessmentSpec](#effectivenessassessmentspec)

| Field| Type| Description|
| ---| ---| ---|
| `stabilizationWindow`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| StabilizationWindow is the duration to wait after remediation before assessment.<br />Set by the Remediation Orchestrator. The EM uses this to delay assessment<br />until the system stabilizes post-remediation.|
| `hashComputeDelay`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| HashComputeDelay is the duration to defer post-remediation spec hash computation<br />after EA creation. Set by the RO for async-managed targets (GitOps, operator<br />CRDs) where spec changes propagate after the WorkflowExecution completes.<br />The EM computes the deferral deadline as: creation + HashComputeDelay.<br />Nil means compute immediately (sync workflows, backward compatible).|
| `alertCheckDelay`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| AlertCheckDelay is an additional duration to defer alert resolution checks<br />beyond the StabilizationWindow. Set by the RO for proactive (predictive) alerts<br />where the underlying Prometheus alert (e.g. predict_linear) requires extra time<br />to resolve after remediation.<br />The EM computes AlertManagerCheckAfter as:<br /> creation + StabilizationWindow + AlertCheckDelay<br />Nil means no additional delay (AlertManagerCheckAfter = PrometheusCheckAfter).|


## EffectivenessAssessment


EffectivenessAssessment is the Schema for the effectivenessassessments API.
It is created by the Remediation Orchestrator and watched by the Effectiveness Monitor.


| Field| Type| Description|
| ---| ---| ---|
| `apiVersion`| _string_| `kubernaut.ai/v1alpha1`|
| `kind`| _string_| `EffectivenessAssessment`|
| `metadata`| _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectmeta-v1-meta)_| Refer to the Kubernetes API documentation for fields of `metadata`.|
| `spec`| _[EffectivenessAssessmentSpec](#effectivenessassessmentspec)_||
| `status`| _[EffectivenessAssessmentStatus](#effectivenessassessmentstatus)_||


### EffectivenessAssessmentSpec


EffectivenessAssessmentSpec defines the desired state of an EffectivenessAssessment.

The spec is set by the Remediation Orchestrator at creation time and is immutable.
Immutability is enforced by CEL validation (self == oldSelf) to prevent tampering.

_Appears in:_
- [EffectivenessAssessment](#effectivenessassessment)

| Field| Type| Description|
| ---| ---| ---|
| `correlationID`| _string_| CorrelationID is the name of the parent RemediationRequest.<br />Used as the correlation ID for audit events .|
| `remediationRequestPhase`| _string_| RemediationRequestPhase is the RemediationRequest's OverallPhase at the time<br />the EA was created. Captured as an immutable spec field so the EM can branch<br />assessment logic based on the RR outcome (Verifying, Completed, Failed, TimedOut).<br />Verifying: happy path — WFE succeeded, EA created while RR awaits assessment .<br />Previously stored as the mutable label kubernaut.ai/rr-phase; moved to spec<br />for immutability and security.|
| `signalTarget`| _[TargetResource](#targetresource)_| SignalTarget is the resource that triggered the alert.<br />Source: RR.Spec.TargetResource (from Gateway alert extraction).<br />Used by: health assessment, alert resolution, metrics queries .|
| `remediationTarget`| _[TargetResource](#targetresource)_| RemediationTarget is the resource the workflow modified.<br />Source: AA.Status.RootCauseAnalysis.RemediationTarget (from HAPI RCA resolution).<br />Used by: spec hash computation, drift detection .|
| `config`| _[EAConfig](#eaconfig)_| Config contains the assessment configuration parameters.|
| `remediationCreatedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| RemediationCreatedAt is the creation timestamp of the parent RemediationRequest.<br />Set by the RO at EA creation time from rr.CreationTimestamp.<br />Used by the audit manager to compute resolution_time_seconds in the<br />assessment.completed event (CompletedAt - RemediationCreatedAt).|
| `signalName`| _string_| SignalName is the original alert/signal name from the parent RemediationRequest.<br />Set by the RO at EA creation time from rr.Spec.SignalName.<br />Used by the audit manager to populate the signal_name field in assessment.completed<br />events (OBS-1: distinct from CorrelationID which is the RR name).|
| `preRemediationSpecHash`| _string_| PreRemediationSpecHash is the canonical spec hash of the target resource BEFORE<br />remediation was applied. Copied from rr.Status.PreRemediationSpecHash by the RO<br />at EA creation time. The EM uses this to compare pre vs post-remediation state<br />for spec drift detection, eliminating the need to query DataStorage audit events.|


### EffectivenessAssessmentStatus


EffectivenessAssessmentStatus defines the observed state of an EffectivenessAssessment.

_Appears in:_
- [EffectivenessAssessment](#effectivenessassessment)

| Field| Type| Description|
| ---| ---| ---|
| `phase`| _string_| Phase is the current lifecycle phase of the assessment.|
| `validityDeadline`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| ValidityDeadline is the absolute time after which the assessment expires.<br />Computed by the EM controller on first reconciliation as:<br /> EA.creationTimestamp + validityWindow (from EM config).<br />This follows Kubernetes spec/status convention: the RO sets desired state<br />(StabilizationWindow in spec), and the EM computes observed/derived state<br />(ValidityDeadline in status). This prevents misconfiguration where<br />StabilizationWindow > ValidityDeadline.|
| `prometheusCheckAfter`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| PrometheusCheckAfter is the earliest time to query Prometheus for metrics.<br />Computed by the EM controller on first reconciliation as:<br /> EA.creationTimestamp + StabilizationWindow (from EA spec).<br />Stored in status to avoid recomputation on every reconcile and for<br />operator observability of the assessment timeline.|
| `alertManagerCheckAfter`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| AlertManagerCheckAfter is the earliest time to check AlertManager for alert resolution.<br />Computed by the EM controller on first reconciliation as:<br /> EA.creationTimestamp + StabilizationWindow + AlertCheckDelay (if set).<br />When AlertCheckDelay is nil, equals PrometheusCheckAfter.<br />Stored in status to avoid recomputation on every reconcile and for<br />operator observability of the assessment timeline.|
| `components`| _[EAComponents](#eacomponents)_| Components tracks the completion state of each assessment component.|
| `assessmentReason`| _string_| AssessmentReason describes why the assessment completed with this outcome.|
| `completedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| CompletedAt is the timestamp when the assessment finished.|
| `message`| _string_| Message provides human-readable details about the current state.|
| `conditions`| _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#condition-v1-meta) array_| Conditions represent the latest available observations of the EA's state.|


### EnrichmentConfig


EnrichmentConfig specifies per-signal enrichment settings.
V2.0 PLACEHOLDER: These fields are currently NOT read by the controller.
All signals use the global enrichment config from the controller's YAML configuration
(enrichment.cacheTtl, enrichment.timeout). Per-signal overrides will be implemented in V2.0.

_Appears in:_
- [SignalProcessingSpec](#signalprocessingspec)

| Field| Type| Description|
| ---| ---| ---|
| `enableClusterState`| _boolean_| Enable cluster state enrichment|
| `enableMetrics`| _boolean_| Enable metrics enrichment|
| `enableHistorical`| _boolean_| Enable historical enrichment|
| `timeout`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| Timeout for enrichment operations|


### Environment

_Underlying type:_ _string_

Environment represents a canonical deployment environment.
 4 canonical environments + Unknown fallback.

_Appears in:_
- [EnvironmentClassification](#environmentclassification)

_Validation:_
- Enum: [Production Staging Development Test Unknown]

| Value| Description|
| ---| ---|
| `Production`||
| `Staging`||
| `Development`||
| `Test`||
| `Unknown`||


### EnvironmentClassification


EnvironmentClassification from .

 V2.0: Removed signal-labels source (security vulnerability)

_Appears in:_
- [SignalProcessingStatus](#signalprocessingstatus)

| Field| Type| Description|
| ---| ---| ---|
| `environment`| _[Environment](#environment)_||
| `source`| _string_| Source of classification: namespace-labels, rego-inference, default|
| `classifiedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| When classification was performed|


### ExecutionConfig


ExecutionConfig contains minimal execution settings.
 ServiceAccountName resolved at runtime from DS into Status.

_Appears in:_
- [WorkflowExecutionSpec](#workflowexecutionspec)

| Field| Type| Description|
| ---| ---| ---|
| `timeout`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| Timeout for the entire workflow (Tekton PipelineRun timeout)<br />Default: use global timeout from RemediationRequest or 30m|


### ExecutionContext


ExecutionContext captures execution and retry data.

_Appears in:_
- [NotificationContext](#notificationcontext)

| Field| Type| Description|
| ---| ---| ---|
| `retryCount`| _string_| RetryCount is the number of retries attempted.|
| `maxRetries`| _string_| MaxRetries is the maximum number of retries allowed.|
| `lastExitCode`| _string_| LastExitCode is the last exit code from the workflow execution.|
| `previousExecution`| _string_| PreviousExecution is the name of the previous WorkflowExecution.|
| `timeoutPhase`| _string_| TimeoutPhase is the phase that timed out.|
| `phaseTimeout`| _string_| PhaseTimeout is the duration string for the phase timeout.|


### ExecutionStatusSummary


ExecutionStatusSummary captures key execution resource status fields
Lightweight summary for both Tekton PipelineRun and K8s Job backends

_Appears in:_
- [WorkflowExecutionStatus](#workflowexecutionstatus)

| Field| Type| Description|
| ---| ---| ---|
| `status`| _[ConditionStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#conditionstatus-v1-core)_| Status of the execution resource (Unknown, True, False)|
| `reason`| _string_| Reason from the execution resource (e.g., "Succeeded", "Failed", "Running")|
| `message`| _string_| Message from the execution resource|
| `completedTasks`| _integer_| CompletedTasks count|
| `totalTasks`| _integer_| TotalTasks count (from pipeline spec)|


### FailureDetails


FailureDetails contains structured failure classification information

_Appears in:_
- [WorkflowExecutionStatus](#workflowexecutionstatus)

| Field| Type| Description|
| ---| ---| ---|
| `failedTaskIndex`| _integer_| FailedTaskIndex is 0-indexed position of failed task in pipeline|
| `failedTaskName`| _string_| FailedTaskName is the name of the failed Tekton Task|
| `failedStepName`| _string_| FailedStepName is the name of the failed step within the task (if available)<br />Tekton tasks can have multiple steps; this identifies the specific step|
| `reason`| _string_| Reason is a Kubernetes-style reason code<br />Used for deterministic failure classification by RO|
| `message`| _string_| Message is human-readable error message (for logging/UI/notifications)|
| `exitCode`| _integer_| ExitCode from container (if applicable)<br />Useful for script-based tasks that return specific exit codes|
| `failedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| FailedAt is the timestamp when the failure occurred|
| `executionTimeBeforeFailure`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| ExecutionTimeBeforeFailure is how long the workflow ran before failing|
| `naturalLanguageSummary`| _string_| NaturalLanguageSummary is a human/LLM-readable failure description<br />Generated by WE controller from structured data above<br />Used by:<br /> - RO: Included in failure notifications<br /> - Notification: Included in user-facing failure alerts|
| `wasExecutionFailure`| _boolean_| WasExecutionFailure indicates whether the failure occurred during workflow execution<br />true = workflow RAN and failed (non-idempotent actions may have occurred)<br />false = workflow failed BEFORE execution (validation, image pull, quota, etc.)<br />CRITICAL: Execution failures (true) block ALL future retries for this target<br /> Pre-execution failures (false) get exponential backoff|


### FailurePhase

_Underlying type:_ _string_

FailurePhase represents the orchestration phase where a failure occurred.
 PascalCase for CRD phase values.

_Appears in:_
- [RemediationRequestStatus](#remediationrequeststatus)

_Validation:_
- Enum: [Configuration SignalProcessing AIAnalysis Approval WorkflowExecution Blocked Deduplicated]

| Value| Description|
| ---| ---|
| `Configuration`||
| `SignalProcessing`||
| `AIAnalysis`||
| `Approval`||
| `WorkflowExecution`||
| `Blocked`||
| `Deduplicated`| FailurePhaseDeduplicated indicates an RR that inherited a failure from a<br />deduplicated WorkflowExecution collision . Excluded from<br />consecutive failure counting per .|


### InvestigationSession


InvestigationSession tracks the async HAPI session lifecycle.
 AA controller session tracking
 Session regeneration on 404 (HAPI restart)

_Appears in:_
- [AIAnalysisStatus](#aianalysisstatus)

| Field| Type| Description|
| ---| ---| ---|
| `id`| _string_| Session ID returned by HAPI on submit (cleared on session loss)|
| `generation`| _integer_| Generation counter tracking session regenerations (0 = first session, incremented on 404)|
| `lastPolled`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| LastPolled timestamp of the last poll attempt|
| `createdAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| CreatedAt timestamp when the current session was created|
| `pollCount`| _integer_| PollCount tracks the number of poll attempts for observability<br /> Constant 15s poll interval (configurable 1s–5m)|


### LineageContext


LineageContext tracks parent resource references for audit correlation .

_Appears in:_
- [NotificationContext](#notificationcontext)

| Field| Type| Description|
| ---| ---| ---|
| `remediationRequest`| _string_| RemediationRequest is the name of the parent RemediationRequest.|
| `aiAnalysis`| _string_| AIAnalysis is the name of the parent AIAnalysis.|


### NotificationContext


NotificationContext provides structured context for a notification,
replacing the former unstructured Metadata map[string]string.

_Appears in:_
- [NotificationRequestSpec](#notificationrequestspec)

| Field| Type| Description|
| ---| ---| ---|
| `lineage`| _[LineageContext](#lineagecontext)_| Lineage tracks parent resource references for audit correlation.|
| `workflow`| _[WorkflowContext](#workflowcontext)_| Workflow captures selected workflow details (approval/completion notifications).|
| `analysis`| _[AnalysisContext](#analysiscontext)_| Analysis captures AI analysis results (approval/completion notifications).|
| `review`| _[ReviewContext](#reviewcontext)_| Review captures manual review context (manual-review notifications).|
| `execution`| _[ExecutionContext](#executioncontext)_| Execution captures execution and retry context (manual-review WE source, timeout notifications).|
| `dedup`| _[DedupContext](#dedupcontext)_| Dedup captures deduplication context (bulk duplicate notifications).|
| `target`| _[TargetContext](#targetcontext)_| Target captures target resource context (timeout notifications).|
| `verification`| _[VerificationContext](#verificationcontext)_| Verification captures EA verification results (completion notifications, #318).<br />Enables routing rules to match on verification outcome (e.g., inconclusive -> escalation).|


### NotificationPhase

_Underlying type:_ _string_


_Appears in:_
- [NotificationRequestStatus](#notificationrequeststatus)

_Validation:_
- Enum: [Pending Sending Retrying Sent PartiallySent Failed]

| Value| Description|
| ---| ---|
| `Pending`||
| `Sending`||
| `Retrying`||
| `Sent`||
| `PartiallySent`||
| `Failed`||


### NotificationPriority

_Underlying type:_ _string_


_Appears in:_
- [NotificationRequestSpec](#notificationrequestspec)

_Validation:_
- Enum: [Critical High Medium Low]

| Value| Description|
| ---| ---|
| `Critical`||
| `High`||
| `Medium`||
| `Low`||


## NotificationRequest


NotificationRequest is the Schema for the notificationrequests API


| Field| Type| Description|
| ---| ---| ---|
| `apiVersion`| _string_| `kubernaut.ai/v1alpha1`|
| `kind`| _string_| `NotificationRequest`|
| `metadata`| _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectmeta-v1-meta)_| Refer to the Kubernetes API documentation for fields of `metadata`.|
| `spec`| _[NotificationRequestSpec](#notificationrequestspec)_||
| `status`| _[NotificationRequestStatus](#notificationrequeststatus)_||


### NotificationRequestSpec


NotificationRequestSpec defines the desired state of NotificationRequest

 Spec Immutability
ALL spec fields are immutable after CRD creation. Users cannot update
notification content once created. To change a notification, delete
and recreate the CRD.

Rationale: Notifications are immutable events, not mutable resources.
This prevents race conditions, simplifies controller logic, and provides
perfect audit trail.

Cancellation: Delete the NotificationRequest CRD to cancel delivery.

_Appears in:_
- [NotificationRequest](#notificationrequest)

| Field| Type| Description|
| ---| ---| ---|
| `remediationRequestRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_| Reference to parent RemediationRequest (if applicable)<br />Used for audit correlation and lineage tracking <br />Optional: NotificationRequest can be standalone (e.g., system-generated alerts)|
| `type`| _[NotificationType](#notificationtype)_| Type of notification (escalation, simple, status-update)|
| `priority`| _[NotificationPriority](#notificationpriority)_| Priority of notification (critical, high, medium, low)|
| `subject`| _string_| Subject line for notification|
| `body`| _string_| Notification body content|
| `severity`| _string_| Severity from the originating signal (used for routing)<br /> promoted from mutable label to immutable spec field|
| `phase`| _string_| Phase that triggered this notification (for phase-timeout notifications)<br /> promoted from mutable label to immutable spec field|
| `reviewSource`| _[ReviewSourceType](#reviewsourcetype)_| ReviewSource indicates what triggered manual review (for manual-review notifications)<br /> promoted from mutable label to immutable spec field|
| `context`| _[NotificationContext](#notificationcontext)_| Context provides typed, structured notification context replacing the<br />former unstructured Metadata map. Each sub-struct is optional (nil means<br />not applicable for this notification type).|
| `extensions`| _object (keys:string, values:string)_| Extensions holds arbitrary key-value pairs for routing and custom data<br />that don't fit the typed Context schema (e.g., test routing overrides,<br />vendor-specific tags). Routing rules can match on these keys.|
| `actionLinks`| _[ActionLink](#actionlink) array_| Action links to external services|
| `retryPolicy`| _[RetryPolicy](#retrypolicy)_| Retry policy for delivery|
| `retentionDays`| _integer_| Retention period in days after completion|


### NotificationRequestStatus


NotificationRequestStatus defines the observed state of NotificationRequest

_Appears in:_
- [NotificationRequest](#notificationrequest)

| Field| Type| Description|
| ---| ---| ---|
| `phase`| _[NotificationPhase](#notificationphase)_| Phase of notification lifecycle (Pending, Sending, Sent, PartiallySent, Failed)|
| `conditions`| _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#condition-v1-meta) array_| Conditions represent the latest available observations of the notification's state|
| `deliveryAttempts`| _[DeliveryAttempt](#deliveryattempt) array_| List of all delivery attempts across all channels|
| `totalAttempts`| _integer_| Total number of delivery attempts across all channels|
| `successfulDeliveries`| _integer_| Number of successful deliveries|
| `failedDeliveries`| _integer_| Number of failed deliveries|
| `queuedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| Time when notification was queued for processing|
| `processingStartedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| Time when processing started|
| `completionTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| Time when all deliveries completed (success or failure)|
| `observedGeneration`| _integer_| Observed generation from spec|
| `reason`| _[NotificationStatusReason](#notificationstatusreason)_| Reason for current phase|
| `message`| _string_| Human-readable message about current state|


### NotificationStatusReason

_Underlying type:_ _string_


_Appears in:_
- [NotificationRequestStatus](#notificationrequeststatus)


| Value| Description|
| ---| ---|
| `AllDeliveriesSucceeded`||
| `PartialDeliverySuccess`||
| `AllDeliveriesFailed`||
| `NoChannelsResolved`||
| `PartialFailureRetrying`||
| `MaxRetriesExhausted`||


### NotificationType

_Underlying type:_ _string_


_Appears in:_
- [NotificationRequestSpec](#notificationrequestspec)

_Validation:_
- Enum: [Escalation Simple StatusUpdate Approval ManualReview Completion]

| Value| Description|
| ---| ---|
| `Escalation`||
| `Simple`||
| `StatusUpdate`||
| `Approval`| NotificationTypeApproval is used for approval request notifications <br />Added Dec 2025 per RO team request for explicit approval workflow support|
| `ManualReview`| NotificationTypeManualReview is used for manual intervention required notifications <br />Added Dec 2025 for ExhaustedRetries/PreviousExecutionFailed scenarios requiring operator action<br />Distinct from 'escalation' to enable spec-field-based routing rules |
| `Completion`| NotificationTypeCompletion is used for successful remediation completion notifications <br />Created when WorkflowExecution completes successfully and RR transitions to Completed phase<br />Enables operators to track successful autonomous remediations|


### ObjectRef


ObjectRef is a lightweight reference to another object in the same namespace

_Appears in:_
- [RemediationApprovalRequestSpec](#remediationapprovalrequestspec)

| Field| Type| Description|
| ---| ---| ---|
| `name`| _string_| Name of the referenced object|


### ObjectReference


ObjectReference contains enough information to let you locate the referenced object.

_Appears in:_
- [SignalProcessingSpec](#signalprocessingspec)

| Field| Type| Description|
| ---| ---| ---|
| `apiVersion`| _string_| API version of the referent|
| `kind`| _string_| Kind of the referent|
| `name`| _string_| Name of the referent|
| `namespace`| _string_| Namespace of the referent|
| `uid`| _string_| UID of the referent|


### PolicyDecision

_Underlying type:_ _string_

PolicyDecision represents the Rego policy evaluation outcome.

_Appears in:_
- [PolicyEvaluation](#policyevaluation)

_Validation:_
- Enum: [Approved ManualReviewRequired Denied DegradedMode]

| Value| Description|
| ---| ---|
| `Approved`||
| `ManualReviewRequired`||
| `Denied`||
| `DegradedMode`||


### PolicyEvaluation


PolicyEvaluation contains Rego policy evaluation results

_Appears in:_
- [ApprovalContext](#approvalcontext)

| Field| Type| Description|
| ---| ---| ---|
| `policyName`| _string_| Policy name that was evaluated|
| `matchedRules`| _string array_| Rules that matched|
| `decision`| _[PolicyDecision](#policydecision)_| Decision from policy evaluation|


### PostRCAContext


PostRCAContext holds data computed by HAPI after the RCA phase.
 DetectedLabels are computed at runtime by HAPI's LabelDetector
and returned in the HAPI response for storage in the AIAnalysis status.
This data is used by Rego policies for approval gating (e.g., stateful
workload detection) and is immutable once set.

_Appears in:_
- [AIAnalysisStatus](#aianalysisstatus)

| Field| Type| Description|
| ---| ---| ---|
| `detectedLabels`| _DetectedLabels_| DetectedLabels contains cluster characteristics computed by HAPI's<br />LabelDetector during get_namespaced_resource_context or get_cluster_resource_context tool invocations.|
| `setAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| SetAt records when the PostRCAContext was populated.<br />Used as the immutability guard: once SetAt is non-nil, the entire<br />PostRCAContext becomes immutable via CEL validation.|


### Priority

_Underlying type:_ _string_

Priority represents an operational priority level.


_Appears in:_
- [PriorityAssignment](#priorityassignment)

_Validation:_
- Enum: [P0 P1 P2 P3]

| Value| Description|
| ---| ---|
| `P0`||
| `P1`||
| `P2`||
| `P3`||


### PriorityAssignment


PriorityAssignment from .


_Appears in:_
- [SignalProcessingStatus](#signalprocessingstatus)

| Field| Type| Description|
| ---| ---| ---|
| `priority`| _[Priority](#priority)_||
| `source`| _string_| Source of assignment: rego-policy, severity-fallback, default|
| `policyName`| _string_| Which Rego rule matched (if applicable)|
| `assignedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| When assignment was performed|


### RecommendedAction


RecommendedAction describes a remediation action with rationale

_Appears in:_
- [ApprovalContext](#approvalcontext)

| Field| Type| Description|
| ---| ---| ---|
| `workflowId`| _string_| WorkflowId is the catalog workflow identifier for this recommendation|
| `rationale`| _string_| Rationale explaining why this action is recommended|


### RecommendedWorkflowSummary


RecommendedWorkflowSummary contains a summary of the recommended workflow

_Appears in:_
- [RemediationApprovalRequestSpec](#remediationapprovalrequestspec)

| Field| Type| Description|
| ---| ---| ---|
| `workflowId`| _string_| Workflow identifier from catalog|
| `version`| _string_| Workflow version|
| `executionBundle`| _string_| Execution bundle OCI reference (digest-pinned)|
| `rationale`| _string_| Rationale for selecting this workflow|


## RemediationApprovalRequest


RemediationApprovalRequest is the Schema for the remediationapprovalrequests API.

 RemediationApprovalRequest CRD Architecture
- Follows Kubernetes CertificateSigningRequest pattern (immutable spec, mutable status)
- Owned by RemediationRequest 
- AIAnalysis controller uses field index on spec.aiAnalysisRef.name for efficient lookup
- Timeout expiration handled by dedicated controller

Lifecycle:
1. RO creates when AIAnalysis.status.approvalRequired=true
2. Operator approves/rejects via status.conditions update
3. Dedicated controller detects decision or timeout
4. AIAnalysis controller watches and transitions phase accordingly


| Field| Type| Description|
| ---| ---| ---|
| `apiVersion`| _string_| `kubernaut.ai/v1alpha1`|
| `kind`| _string_| `RemediationApprovalRequest`|
| `metadata`| _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectmeta-v1-meta)_| Refer to the Kubernetes API documentation for fields of `metadata`.|
| `spec`| _[RemediationApprovalRequestSpec](#remediationapprovalrequestspec)_||
| `status`| _[RemediationApprovalRequestStatus](#remediationapprovalrequeststatus)_||


### RemediationApprovalRequestSpec


RemediationApprovalRequestSpec defines the desired state of RemediationApprovalRequest.

 Spec Immutability
ALL spec fields are immutable after CRD creation (follows CertificateSigningRequest pattern).
This provides a complete audit trail and prevents race conditions.

_Appears in:_
- [RemediationApprovalRequest](#remediationapprovalrequest)

| Field| Type| Description|
| ---| ---| ---|
| `remediationRequestRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_| Reference to parent RemediationRequest CRD (owner)<br />RemediationRequest owns this CRD via ownerReferences|
| `aiAnalysisRef`| _[ObjectRef](#objectref)_| Reference to the AIAnalysis that requires approval<br />Used by AIAnalysis controller for efficient field-indexed lookup|
| `confidence`| _float_| Confidence score from AI analysis (0.0-1.0)<br />Typically 0.6-0.79 triggers approval (below auto-approve threshold)|
| `confidenceLevel`| _string_| Confidence level derived from score|
| `reason`| _string_| Reason why approval is required|
| `recommendedWorkflow`| _[RecommendedWorkflowSummary](#recommendedworkflowsummary)_| Recommended workflow from AI analysis|
| `investigationSummary`| _string_| Investigation summary from HolmesGPT|
| `evidenceCollected`| _string array_| Evidence collected during investigation|
| `recommendedActions`| _[ApprovalRecommendedAction](#approvalrecommendedaction) array_| Recommended actions with rationale|
| `alternativesConsidered`| _[ApprovalAlternative](#approvalalternative) array_| Alternative approaches considered|
| `whyApprovalRequired`| _string_| Detailed explanation of why approval is required|
| `policyEvaluation`| _[ApprovalPolicyEvaluation](#approvalpolicyevaluation)_| Policy evaluation results if Rego policy triggered approval|
| `requiredBy`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| Deadline for approval decision (approval expires after this time)<br />Calculated by RO using hierarchy: per-request → policy → namespace → default (15m)|


### RemediationApprovalRequestStatus


RemediationApprovalRequestStatus defines the observed state of RemediationApprovalRequest.

_Appears in:_
- [RemediationApprovalRequest](#remediationapprovalrequest)

| Field| Type| Description|
| ---| ---| ---|
| `decision`| _[ApprovalDecision](#approvaldecision)_| Decision made by operator or system (timeout)<br />Empty string indicates pending decision|
| `decidedBy`| _string_| Who made the decision (username or "system" for timeout)|
| `decidedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| When the decision was made|
| `decisionMessage`| _string_| Optional message from the decision maker|
| `workflowOverride`| _[WorkflowOverride](#workflowoverride)_| Operator workflow/parameter override (#594, ).<br />Only valid when Decision is Approved. Webhook validates the referenced RW.|
| `conditions`| _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#condition-v1-meta) array_| Conditions represent the latest available observations<br />Standard condition types:<br />- "Approved" - Decision is Approved<br />- "Rejected" - Decision is Rejected<br />- "Expired" - Decision timed out|
| `createdAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| Time when the approval request was created|
| `timeRemaining`| _string_| Time remaining until expiration (human-readable, e.g., "5m30s")<br />Updated by controller periodically|
| `expired`| _boolean_| True if the approval request has expired|
| `observedGeneration`| _integer_| ObservedGeneration is the most recent generation observed|
| `reason`| _string_| Reason for current state (machine-readable)|
| `message`| _string_| Human-readable message about current state|


### RemediationPhase

_Underlying type:_ _string_

RemediationPhase represents the orchestration phase of a RemediationRequest.
These constants are exported for external consumers (e.g., Gateway) to enable
type-safe cross-service integration .

Capitalized phase values per Kubernetes API conventions.


_Appears in:_
- [RemediationRequestStatus](#remediationrequeststatus)

_Validation:_
- Enum: [Pending Processing Analyzing AwaitingApproval Executing Verifying Blocked Completed Failed TimedOut Skipped Cancelled]

| Value| Description|
| ---| ---|
| `Pending`| PhasePending is the initial state when RemediationRequest is created.|
| `Processing`| PhaseProcessing indicates SignalProcessing is enriching the signal.|
| `Analyzing`| PhaseAnalyzing indicates AIAnalysis is determining remediation workflow.|
| `AwaitingApproval`| PhaseAwaitingApproval indicates human approval is required.|
| `Executing`| PhaseExecuting indicates WorkflowExecution is running remediation.|
| `Verifying`| PhaseVerifying indicates remediation succeeded and EffectivenessAssessment is running.<br />Non-terminal: Gateway deduplicates signals while EA assesses remediation effectiveness.<br />RO transitions to Completed when EA reaches a terminal state or VerificationDeadline expires.|
| `Blocked`| PhaseBlocked indicates remediation cannot proceed due to external blocking condition.<br />This is a NON-terminal phase (Gateway deduplicates, prevents RR flood).<br />V1.0: Unified blocking for 6 scenarios:<br />- ConsecutiveFailures: After cooldown → Failed <br />- ResourceBusy: When resource available → Proceeds to execute<br />- RecentlyRemediated: After cooldown → Proceeds to execute <br />- ExponentialBackoff: After backoff window → Retries execution <br />- DuplicateInProgress: When original completes → Inherits outcome<br />- UnmanagedResource: Retries until scope label added or RR times out|
| `Completed`| PhaseCompleted is the terminal success state.|
| `Failed`| PhaseFailed is the terminal failure state.|
| `TimedOut`| PhaseTimedOut is the terminal timeout state.|
| `Skipped`| PhaseSkipped is the terminal state when remediation was not needed.|
| `Cancelled`| PhaseCancelled is the terminal state when remediation was manually cancelled.<br />Gateway treats this as terminal (allows new RR creation for retry)|


## RemediationRequest


RemediationRequest is the Schema for the remediationrequests API.
 Printer columns for operational triage


| Field| Type| Description|
| ---| ---| ---|
| `apiVersion`| _string_| `kubernaut.ai/v1alpha1`|
| `kind`| _string_| `RemediationRequest`|
| `metadata`| _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectmeta-v1-meta)_| Refer to the Kubernetes API documentation for fields of `metadata`.|
| `spec`| _[RemediationRequestSpec](#remediationrequestspec)_||
| `status`| _[RemediationRequestStatus](#remediationrequeststatus)_||


### RemediationRequestSpec


RemediationRequestSpec defines the desired state of RemediationRequest.

 Spec Immutability
RemediationRequest represents an immutable event (signal received, remediation required).
Once created (by Gateway or external source), spec cannot be modified to ensure:
- Audit trail integrity (remediation matches original signal)
- No signal metadata tampering during remediation lifecycle
- Consistent signal data across all child CRDs (SignalProcessing, AIAnalysis, WorkflowExecution)

Cancellation: Delete the RemediationRequest CRD (Kubernetes-native pattern).
Status updates: Controllers update .status fields (not affected by spec immutability).

Note: Individual field immutability (e.g., signalFingerprint) is redundant with full spec immutability,
but retained for explicit documentation of critical fields.

_Appears in:_
- [RemediationRequest](#remediationrequest)

| Field| Type| Description|
| ---| ---| ---|
| `signalFingerprint`| _string_| Core Signal Identification<br />Unique fingerprint for deduplication (SHA256 of alert/event key fields)<br />This field is immutable and used for querying all occurrences of the same problem|
| `signalName`| _string_| Human-readable signal name (e.g., "HighMemoryUsage", "CrashLoopBackOff")|
| `severity`| _string_| Signal Classification<br />Severity level (external value from signal provider)<br />Examples: "Sev1", "P0", "critical", "HIGH", "warning"<br />SignalProcessing will normalize via Rego policy|
| `signalType`| _string_| Signal type: "alert" (generic signal type; adapter-specific values are deprecated)<br />Used for signal-aware remediation strategies|
| `signalSource`| _string_| Adapter that ingested the signal (e.g., "prometheus-adapter", "k8s-event-adapter")|
| `targetType`| _string_| Target system type: "kubernetes", "aws", "azure", "gcp", "datadog"<br />Indicates which infrastructure system the signal targets|
| `targetResource`| _[ResourceIdentifier](#resourceidentifier)_| TargetResource identifies the Kubernetes resource that triggered this signal.<br />Populated by Gateway from NormalizedSignal.Resource - REQUIRED.<br />Used by SignalProcessing for context enrichment and RO for workflow routing.<br />For Kubernetes signals, this contains Kind, Name, Namespace of the affected resource.|
| `firingTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| Temporal Data<br />When the signal first started firing (from upstream source)|
| `receivedTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| When Gateway received the signal|
| `signalLabels`| _object (keys:string, values:string)_| Signal labels and annotations extracted from provider-specific data<br />These are populated by Gateway Service after parsing providerData|
| `signalAnnotations`| _object (keys:string, values:string)_||
| `providerData`| _string_| Provider-specific fields in raw JSON format<br />Gateway adapter populates this based on signal source<br />Controllers parse this based on targetType/signalType<br />For Kubernetes (targetType="kubernetes"):<br /> \{"namespace": "...", "resource": \{"kind": "...", "name": "..."\}, "alertmanagerURL": "...", ...\}<br />For AWS (targetType="aws"):<br /> \{"region": "...", "accountId": "...", "instanceId": "...", "resourceType": "...", ...\}<br />For Datadog (targetType="datadog"):<br /> \{"monitorId": 123, "host": "...", "tags": [...], "metricQuery": "...", ...\}|
| `originalPayload`| _string_| Complete original webhook payload for debugging and audit<br /> stored as string to avoid base64 encoding in CEL validation|


### RemediationRequestStatus


RemediationRequestStatus defines the observed state of RemediationRequest.

_Appears in:_
- [RemediationRequest](#remediationrequest)

| Field| Type| Description|
| ---| ---| ---|
| `deduplication`| _[DeduplicationStatus](#deduplicationstatus)_| Deduplication tracks signal occurrence for this remediation.<br />OWNER: Gateway Service (exclusive write access)|
| `observedGeneration`| _integer_| ObservedGeneration is the most recent generation observed by the controller.<br />Used to prevent duplicate reconciliations and ensure idempotency.<br />Per Standard pattern for all Kubernetes controllers.|
| `overallPhase`| _[RemediationPhase](#remediationphase)_| Phase tracking for orchestration.<br />Uses typed RemediationPhase constants for type safety and cross-service integration.<br />Capitalized phase values per Kubernetes API conventions.|
| `message`| _string_| Human-readable message describing current status|
| `startTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| Timestamps|
| `completedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_||
| `processingStartTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| ProcessingStartTime is when SignalProcessing phase started.<br />Used for per-phase timeout detection (default: 5 minutes).|
| `analyzingStartTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| AnalyzingStartTime is when AIAnalysis phase started.<br />Used for per-phase timeout detection (default: 10 minutes).|
| `executingStartTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| ExecutingStartTime is when WorkflowExecution phase started.<br />Used for per-phase timeout detection (default: 30 minutes).|
| `verificationDeadline`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| VerificationDeadline is the deadline for the Verifying phase.<br />Computed by RO as EA.Status.ValidityDeadline + 30s buffer.<br />If exceeded, RR transitions to Completed with Outcome "VerificationTimedOut".|
| `signalProcessingRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_| References to downstream CRDs|
| `remediationProcessingRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_||
| `aiAnalysisRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_||
| `workflowExecutionRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_||
| `notificationRequestRefs`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core) array_| NotificationRequestRefs tracks all notification CRDs created for this remediation.<br />Provides audit trail for compliance and instant visibility for debugging.|
| `effectivenessAssessmentRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_| EffectivenessAssessmentRef tracks the EffectivenessAssessment CRD created for this remediation.<br />Set by the RO after creating the EA CRD on terminal phase transitions.|
| `preRemediationSpecHash`| _string_| PreRemediationSpecHash is the canonical spec hash of the target resource captured<br />by the RO BEFORE launching the remediation workflow. This enables the EM to compare<br />pre vs post-remediation state without querying DataStorage audit events.<br />Set once by the RO during the transition to WorkflowExecution phase; immutable after.|
| `approvalNotificationSent`| _boolean_| Approval notification tracking <br />Prevents duplicate notifications when AIAnalysis requires approval|
| `skipReason`| _[SkipReason](#skipreason)_| SkipReason indicates why this remediation was skipped.<br />Only set when OverallPhase = Skipped or Failed.|
| `skipMessage`| _string_| SkipMessage provides human-readable details about why remediation was skipped<br />Examples:<br />- "Same workflow executed recently. Cooldown: 3m15s remaining"<br />- "Another workflow is running on target: wfe-abc123"<br />- "Backoff active. Next allowed: 2025-12-15T10:30:00Z"<br />Only set when OverallPhase = "Skipped" or "Failed"|
| `blockingWorkflowExecution`| _string_| BlockingWorkflowExecution references the WorkflowExecution causing the block<br />Set for block reasons: ResourceBusy, RecentlyRemediated, ExponentialBackoff<br />Nil for: ConsecutiveFailures, DuplicateInProgress<br />Enables operators to investigate the blocking WFE for troubleshooting|
| `duplicateOf`| _string_| DuplicateOf references the parent RemediationRequest that this is a duplicate of<br />V1.0: Set when OverallPhase = "Blocked" with BlockReason = "DuplicateInProgress"<br />Old behavior: Set when OverallPhase = "Skipped" due to resource lock deduplication|
| `duplicateCount`| _integer_| DuplicateCount tracks the number of duplicate remediations that were skipped<br />because this RR's workflow was already executing (resource lock)<br />Only populated on parent RRs that have duplicates|
| `duplicateRefs`| _string array_| DuplicateRefs lists the names of RemediationRequests that were skipped<br />because they targeted the same resource as this RR<br />Only populated on parent RRs that have duplicates|
| `deduplicatedByWE`| _string_| DeduplicatedByWE stores the name of the original WorkflowExecution whose<br />outcome this RR is waiting to inherit . Set when the RR's own<br />WFE was marked Failed/Deduplicated. Used as a field index key for the<br />cross-WE watch to trigger reconciliation when the original WFE completes.<br />Immutable after initial assignment.|
| `blockReason`| _[BlockReason](#blockreason)_| BlockReason indicates why this remediation is blocked (non-terminal)<br />Valid values:<br />- "ConsecutiveFailures": Max consecutive failures reached, in cooldown <br />- "ResourceBusy": Another workflow is using the target resource<br />- "RecentlyRemediated": Target recently remediated, cooldown active <br />- "ExponentialBackoff": Pre-execution failures, backoff window active <br />- "DuplicateInProgress": Duplicate of an active remediation<br />Only set when OverallPhase = "Blocked"|
| `blockMessage`| _string_| BlockMessage provides human-readable details about why remediation is blocked<br />Examples:<br />- "Another workflow is running on target deployment/my-app: wfe-abc123"<br />- "Recently remediated. Cooldown: 3m15s remaining"<br />- "Backoff active. Next retry: 2025-12-15T10:30:00Z"<br />- "Duplicate of active remediation rr-original-abc123"<br />- "3 consecutive failures. Cooldown expires: 2025-12-15T11:00:00Z"<br />Only set when OverallPhase = "Blocked"|
| `blockedUntil`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| BlockedUntil indicates when blocking expires (time-based blocks)<br />Set for: ConsecutiveFailures, RecentlyRemediated, ExponentialBackoff<br />Nil for: ResourceBusy, DuplicateInProgress (event-based, cleared when condition resolves)<br />After this time passes, RR will retry or transition to Failed (for ConsecutiveFailures)|
| `nextAllowedExecution`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| NextAllowedExecution indicates when this RR can be retried after exponential backoff.<br />Set when RR fails due to pre-execution failures (infrastructure, validation, etc.).<br />Implements progressive delay: 1m, 2m, 4m, 8m, capped at 10m.<br />Formula: min(Base × 2^(failures-1), Max)<br />Nil means no exponential backoff is active.|
| `consecutiveFailureCount`| _integer_| ConsecutiveFailureCount tracks how many times this fingerprint has failed consecutively.<br />Updated by RO when RR transitions to Failed phase.<br />Reset to 0 when RR completes successfully.|
| `failurePhase`| _[FailurePhase](#failurephase)_| FailurePhase indicates which orchestration phase failed.<br />Only set when OverallPhase = Failed.|
| `failureReason`| _string_| FailureReason provides a human-readable reason for the failure<br />Only set when OverallPhase = "failed"|
| `requiresManualReview`| _boolean_| RequiresManualReview indicates that this remediation cannot proceed automatically<br />and requires operator intervention. Set when:<br />- WE skip reason is "ExhaustedRetries" (5+ consecutive pre-execution failures)<br />- WE skip reason is "PreviousExecutionFailed" (execution failure, cluster state unknown)<br />- AIAnalysis WorkflowResolutionFailed with LowConfidence or WorkflowNotFound|
| `outcome`| _string_| Outcome indicates the remediation result when completed.<br />Values:<br />- "Remediated": Workflow executed successfully<br />- "NoActionRequired": AIAnalysis determined no action needed (problem self-resolved)<br />- "ManualReviewRequired": Requires operator intervention<br />- "VerificationTimedOut": EA assessment did not complete within deadline|
| `timeoutPhase`| _[RemediationPhase](#remediationphase)_| TimeoutPhase indicates which orchestration phase timed out.<br />Only set when OverallPhase = TimedOut.|
| `timeoutTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| TimeoutTime records when the timeout occurred<br />Only set when OverallPhase = "timeout"|
| `retentionExpiryTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| RetentionExpiryTime indicates when this CRD should be cleaned up (24 hours after completion)|
| `notificationStatus`| _string_| NotificationStatus tracks the delivery status of notification(s) for this remediation.<br />Values: "Pending", "InProgress", "Sent", "Failed", "Cancelled"<br />Status Mapping from NotificationRequest.Status.Phase:<br />- NotificationRequest Pending → "Pending"<br />- NotificationRequest Sending → "InProgress"<br />- NotificationRequest Sent → "Sent"<br />- NotificationRequest Failed → "Failed"<br />- NotificationRequest deleted by user → "Cancelled"<br />For bulk notifications , this reflects the status of the consolidated notification.|
| `conditions`| _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#condition-v1-meta) array_| Conditions represent observations of RemediationRequest state.<br />Standard condition types:<br />- "NotificationDelivered": True if notification sent successfully, False if cancelled/failed<br /> - Reason "DeliverySucceeded": Notification sent<br /> - Reason "UserCancelled": User deleted NotificationRequest before delivery<br /> - Reason "DeliveryFailed": NotificationRequest failed to deliver<br />Conditions follow Kubernetes API conventions (KEP-1623).|
| `timeoutConfig`| _[TimeoutConfig](#timeoutconfig)_| TimeoutConfig provides operational timeout overrides for this remediation.<br />OWNER: Remediation Orchestrator (sets defaults on first reconcile)<br />MUTABLE BY: Operators (can adjust mid-remediation via kubectl edit)|
| `lastModifiedBy`| _string_| LastModifiedBy tracks the last operator who modified this RR's status.<br />Populated by RemediationRequest mutating webhook.|
| `lastModifiedAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| LastModifiedAt tracks when the last status modification occurred.<br />Populated by RemediationRequest mutating webhook.|
| `currentProcessingRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_| CurrentProcessingRef references the current SignalProcessing CRD|
| `selectedWorkflowRef`| _[WorkflowReference](#workflowreference)_| SelectedWorkflowRef captures the workflow selected by AI for this remediation.<br />Populated from workflowexecution.selection.completed audit event.|
| `executionRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_| ExecutionRef references the WorkflowExecution CRD for this remediation.<br />Populated from workflowexecution.execution.started audit event.|
| `remediationTarget`| _[ResourceIdentifier](#resourceidentifier)_| RemediationTarget identifies the Kubernetes resource the LLM determined should be<br />remediated. Populated from AIAnalysis.Status.RootCauseAnalysis.AffectedResource.<br />May differ from Spec.TargetResource (e.g., Deployment vs Pod).|
| `targetDisplay`| _string_| TargetDisplay is the Kubernetes-idiomatic Kind/Name of the RCA target<br />(e.g., "Deployment/web-frontend"). Populated when RemediationTarget is set.|
| `confidence`| _string_| Confidence is the AI analysis confidence score as a display string<br />(e.g., "0.97"). Populated from AIAnalysis.SelectedWorkflow.Confidence.|
| `workflowDisplayName`| _string_| WorkflowDisplayName is the human-readable workflow identifier<br />(e.g., "GitRevertCommit:git-revert-v2"). Populated from AIAnalysis.SelectedWorkflow.|
| `signalTargetDisplay`| _string_| SignalTargetDisplay is the Kubernetes-idiomatic Kind/Name of the signal target<br />(e.g., "Pod/web-frontend-cdbdbc4f8-6kn6j"). Populated from Spec.TargetResource.|


### RemediationTarget


RemediationTarget identifies the Kubernetes resource identified by the LLM as the
actual target for remediation. This may differ from the signal's source resource
(e.g., the signal comes from a Pod, but the Deployment should be patched).

_Appears in:_
- [RootCauseAnalysis](#rootcauseanalysis)

| Field| Type| Description|
| ---| ---| ---|
| `kind`| _string_| Kind is the Kubernetes resource kind (e.g., "Deployment", "StatefulSet", "DaemonSet")|
| `name`| _string_| Name is the resource name|
| `namespace`| _string_| Namespace is the resource namespace. Empty for cluster-scoped resources (e.g., Node, PersistentVolume).|


## RemediationWorkflow


RemediationWorkflow is the Schema for the remediationworkflows API.
 Kubernetes-native workflow schema definition.


| Field| Type| Description|
| ---| ---| ---|
| `apiVersion`| _string_| `kubernaut.ai/v1alpha1`|
| `kind`| _string_| `RemediationWorkflow`|
| `metadata`| _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectmeta-v1-meta)_| Refer to the Kubernetes API documentation for fields of `metadata`.|
| `spec`| _[RemediationWorkflowSpec](#remediationworkflowspec)_||
| `status`| _[RemediationWorkflowStatus](#remediationworkflowstatus)_||


### RemediationWorkflowDependencies


RemediationWorkflowDependencies declares infrastructure resources

_Appears in:_
- [RemediationWorkflowSpec](#remediationworkflowspec)

| Field| Type| Description|
| ---| ---| ---|
| `secrets`| _[RemediationWorkflowResourceDependency](#remediationworkflowresourcedependency) array_||
| `configMaps`| _[RemediationWorkflowResourceDependency](#remediationworkflowresourcedependency) array_||


### RemediationWorkflowDescription


RemediationWorkflowDescription provides structured information about a workflow

_Appears in:_
- [RemediationWorkflowSpec](#remediationworkflowspec)

| Field| Type| Description|
| ---| ---| ---|
| `what`| _string_| What describes what this workflow concretely does|
| `whenToUse`| _string_| WhenToUse describes conditions under which this workflow is appropriate|
| `whenNotToUse`| _string_| WhenNotToUse describes specific exclusion conditions|
| `preconditions`| _string_| Preconditions describes conditions that must be verified through investigation|


### RemediationWorkflowExecution


RemediationWorkflowExecution contains execution engine configuration

_Appears in:_
- [RemediationWorkflowSpec](#remediationworkflowspec)

| Field| Type| Description|
| ---| ---| ---|
| `engine`| _string_| Engine is the execution engine type|
| `bundle`| _string_| Bundle is the execution bundle or container image reference|
| `bundleDigest`| _string_| BundleDigest is the digest of the execution bundle|
| `engineConfig`| _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#json-v1-apiextensions-k8s-io)_| EngineConfig holds engine-specific configuration|
| `serviceAccountName`| _string_| ServiceAccountName is the pre-existing ServiceAccount for the execution<br />resource (Job, PipelineRun, or Ansible TokenRequest).<br /> Operators pre-create SAs with appropriate RBAC in the<br />execution namespace. If absent, K8s assigns the namespace's default SA<br />(Job/Tekton) or the Ansible executor uses the controller's in-cluster<br />credentials (#500 fallback).|


### RemediationWorkflowLabels


RemediationWorkflowLabels contains mandatory matching/filtering criteria

_Appears in:_
- [RemediationWorkflowSpec](#remediationworkflowspec)

| Field| Type| Description|
| ---| ---| ---|
| `severity`| _string array_| Severity is the severity level(s)|
| `environment`| _string array_| Environment is the target environment(s)|
| `component`| _string_| Component is the Kubernetes resource type|
| `priority`| _string_| Priority is the business priority level|


### RemediationWorkflowMaintainer


RemediationWorkflowMaintainer contains maintainer contact information

_Appears in:_
- [RemediationWorkflowSpec](#remediationworkflowspec)

| Field| Type| Description|
| ---| ---| ---|
| `name`| _string_||
| `email`| _string_||


### RemediationWorkflowParameter


RemediationWorkflowParameter defines a workflow input parameter

_Appears in:_
- [RemediationWorkflowSpec](#remediationworkflowspec)

| Field| Type| Description|
| ---| ---| ---|
| `name`| _string_||
| `type`| _string_||
| `required`| _boolean_||
| `description`| _string_||
| `enum`| _string array_||
| `pattern`| _string_||
| `minimum`| _float_||
| `maximum`| _float_||
| `default`| _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#json-v1-apiextensions-k8s-io)_||
| `dependsOn`| _string array_||


### RemediationWorkflowResourceDependency


RemediationWorkflowResourceDependency identifies a Kubernetes resource by name

_Appears in:_
- [RemediationWorkflowDependencies](#remediationworkflowdependencies)

| Field| Type| Description|
| ---| ---| ---|
| `name`| _string_||


### RemediationWorkflowSpec


RemediationWorkflowSpec defines the desired state of RemediationWorkflow.
Declared as a Kubernetes resource; registered via kubectl apply .
Workflow name is derived from the CRD's metadata.name (not duplicated in spec).

_Appears in:_
- [RemediationWorkflow](#remediationworkflow)

| Field| Type| Description|
| ---| ---| ---|
| `version`| _string_| Version is the semantic version (e.g., "1.0.0")|
| `description`| _[RemediationWorkflowDescription](#remediationworkflowdescription)_| Description is a structured description for LLM and operator consumption|
| `actionType`| _string_| ActionType is the action type from the taxonomy (PascalCase).|
| `labels`| _[RemediationWorkflowLabels](#remediationworkflowlabels)_| Labels contains mandatory matching/filtering criteria for discovery|
| `customLabels`| _object (keys:string, values:string)_| CustomLabels contains operator-defined key-value labels for additional filtering|
| `detectedLabels`| _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#json-v1-apiextensions-k8s-io)_| DetectedLabels contains author-declared infrastructure requirements|
| `execution`| _[RemediationWorkflowExecution](#remediationworkflowexecution)_| Execution contains execution engine configuration|
| `dependencies`| _[RemediationWorkflowDependencies](#remediationworkflowdependencies)_| Dependencies declares infrastructure resources required by the workflow|
| `maintainers`| _[RemediationWorkflowMaintainer](#remediationworkflowmaintainer) array_| Maintainers is optional maintainer information|
| `parameters`| _[RemediationWorkflowParameter](#remediationworkflowparameter) array_| Parameters defines the workflow input parameters|
| `rollbackParameters`| _[RemediationWorkflowParameter](#remediationworkflowparameter) array_| RollbackParameters defines parameters needed for rollback|


### RemediationWorkflowStatus


RemediationWorkflowStatus defines the observed state of RemediationWorkflow

_Appears in:_
- [RemediationWorkflow](#remediationworkflow)

| Field| Type| Description|
| ---| ---| ---|
| `workflowId`| _string_| WorkflowID is the UUID assigned by Data Storage upon registration|
| `catalogStatus`| _[CatalogStatus](#catalogstatus)_| CatalogStatus reflects the DS catalog lifecycle state.|
| `registeredBy`| _string_| RegisteredBy is the identity of the registrant|
| `registeredAt`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| RegisteredAt is the timestamp of initial registration|
| `previouslyExisted`| _boolean_| PreviouslyExisted indicates if this workflow was re-registered after deletion|


### ResourceIdentifier


ResourceIdentifier identifies the target resource for remediation.

_Appears in:_
- [SignalData](#signaldata)

| Field| Type| Description|
| ---| ---| ---|
| `kind`| _string_| Resource kind (e.g., "Pod", "Deployment", "StatefulSet")|
| `name`| _string_| Resource name|
| `namespace`| _string_| Resource namespace. Empty for cluster-scoped resources (e.g., Node, PersistentVolume).|


### RetryPolicy


RetryPolicy defines retry behavior for notification delivery

_Appears in:_
- [NotificationRequestSpec](#notificationrequestspec)

| Field| Type| Description|
| ---| ---| ---|
| `maxAttempts`| _integer_| Maximum number of delivery attempts|
| `initialBackoffSeconds`| _integer_| Initial backoff duration in seconds|
| `backoffMultiplier`| _integer_| Backoff multiplier (exponential backoff)|
| `maxBackoffSeconds`| _integer_| Maximum backoff duration in seconds|


### ReviewContext


ReviewContext captures manual review details .

_Appears in:_
- [NotificationContext](#notificationcontext)

| Field| Type| Description|
| ---| ---| ---|
| `reason`| _string_| Reason is the high-level failure reason (e.g., "WorkflowResolutionFailed").|
| `subReason`| _string_| SubReason provides granular detail (e.g., "WorkflowNotFound").|
| `humanReviewReason`| _string_| HumanReviewReason from HAPI when needs_human_review=true .|
| `rootCauseAnalysis`| _string_| RootCauseAnalysis from AIAnalysis if available.|


### ReviewSourceType

_Underlying type:_ _string_


_Appears in:_
- [NotificationRequestSpec](#notificationrequestspec)

_Validation:_
- Enum: [AIAnalysis WorkflowExecution]

| Value| Description|
| ---| ---|
| `AIAnalysis`||
| `WorkflowExecution`||


### RootCauseAnalysis


RootCauseAnalysis contains detailed RCA results

_Appears in:_
- [AIAnalysisStatus](#aianalysisstatus)

| Field| Type| Description|
| ---| ---| ---|
| `summary`| _string_| Brief summary of root cause|
| `severity`| _string_| Severity determined by RCA <br /> Aligned with HAPI/workflow catalog (critical, high, medium, low, unknown)|
| `signalType`| _string_| Signal type determined by RCA (may differ from input)|
| `contributingFactors`| _string array_| Contributing factors|
| `remediationTarget`| _[RemediationTarget](#remediationtarget)_| RemediationTarget identifies the actual resource the LLM determined should be remediated.<br /> The LLM may identify a higher-level resource (e.g., Deployment) rather than<br />the Pod that generated the signal. The WFE creator should prefer this over the RR's<br />TargetResource when available to ensure the correct resource is patched.|


### SelectedWorkflow


SelectedWorkflow contains the AI-selected workflow for execution
 Output format for RO to create WorkflowExecution

_Appears in:_
- [AIAnalysisStatus](#aianalysisstatus)

| Field| Type| Description|
| ---| ---| ---|
| `workflowId`| _string_| Workflow identifier (catalog lookup key)|
| `actionType`| _string_| Action type from taxonomy (e.g., ScaleReplicas, RestartPod).<br />Propagated from HAPI three-step discovery protocol to RO audit events.|
| `version`| _string_| Workflow version|
| `executionBundle`| _string_| Execution bundle OCI reference (digest-pinned) - resolved by HolmesGPT-API|
| `executionBundleDigest`| _string_| Execution bundle digest for audit trail|
| `confidence`| _float_| Confidence score (0.0-1.0)|
| `parameters`| _object (keys:string, values:string)_| Workflow parameters (UPPER_SNAKE_CASE keys)|
| `rationale`| _string_| Rationale explaining why this workflow was selected|
| `executionEngine`| _string_| ExecutionEngine specifies the backend engine for workflow execution.<br />Populated from HolmesGPT-API workflow recommendation.<br />When empty, defaults to "tekton" for backwards compatibility.|
| `engineConfig`| _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#json-v1-apiextensions-k8s-io)_| EngineConfig holds engine-specific configuration .<br />For ansible: \{"playbookPath": "...", "jobTemplateName": "...", "inventoryName": "..."\}.|


### SignalContextInput


SignalContextInput contains enriched signal context from SignalProcessing
 Structured types replace map[string]string anti-pattern

_Appears in:_
- [AnalysisRequest](#analysisrequest)

| Field| Type| Description|
| ---| ---| ---|
| `fingerprint`| _string_| Signal fingerprint for correlation|
| `severity`| _string_| Signal severity: critical, high, medium, low, unknown (normalized by SignalProcessing Rego - )|
| `signalName`| _string_| Signal name (e.g., OOMKilled, CrashLoopBackOff)<br />Normalized by SignalProcessing: proactive names mapped to base names|
| `signalMode`| _string_| SignalMode indicates whether this is a reactive or proactive signal.<br /> Proactive Signal Mode Prompt Strategy<br />Copied from SignalProcessing status by RemediationOrchestrator.<br />Used by HAPI to switch investigation prompt (RCA vs. predict & prevent).|
| `environment`| _string_| Environment classification<br />Examples: "production", "staging", "development", "qa-eu", "canary"|
| `businessPriority`| _string_| Business priority<br />Best practice examples: P0 (critical), P1 (high), P2 (normal), P3 (low)|
| `targetResource`| _[TargetResource](#targetresource)_| Target resource identification|
| `enrichmentResults`| _EnrichmentResults_| Complete enrichment results from SignalProcessing|
| `signalAnnotations`| _object (keys:string, values:string)_| SignalAnnotations from the original alert (e.g., description, summary from AlertManager).<br />Untrusted content — sanitized by KA prompt builder before reaching the LLM.|


### SignalData


SignalData contains all signal information copied from RemediationRequest.
This makes SignalProcessing self-contained for processing.

_Appears in:_
- [SignalProcessingSpec](#signalprocessingspec)

| Field| Type| Description|
| ---| ---| ---|
| `fingerprint`| _string_| Unique fingerprint for deduplication (SHA256 of signal key fields)|
| `name`| _string_| Human-readable signal name (e.g., "HighMemoryUsage", "CrashLoopBackOff")|
| `severity`| _string_| Severity level (external/raw value from monitoring system)<br /> No enum restriction - allows external severity schemes (Sev1-4, P0-P4, etc.)<br />Normalized severity is stored in Status.Severity|
| `type`| _string_| Signal type: "alert" (generic signal type; adapter-specific values like "prometheus-alert" or "kubernetes-event" are deprecated)|
| `source`| _string_| Adapter that ingested the signal|
| `targetType`| _string_| Target system type.<br />V2.0 PLACEHOLDER: Currently only "kubernetes" is supported by the enricher.<br />Non-kubernetes values are accepted by validation but enrichment will run in degraded mode.|
| `targetResource`| _[ResourceIdentifier](#resourceidentifier)_| Target resource identification|
| `labels`| _object (keys:string, values:string)_| Signal labels extracted from provider-specific data|
| `annotations`| _object (keys:string, values:string)_| Signal annotations extracted from provider-specific data|
| `firingTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| When the signal first started firing|
| `receivedTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| When Gateway received the signal|
| `providerData`| _string_| Provider-specific fields in raw JSON format|


## SignalProcessing


SignalProcessing is the Schema for the signalprocessings API.
 

| Field| Type| Description|
| ---| ---| ---|
| `apiVersion`| _string_| `kubernaut.ai/v1alpha1`|
| `kind`| _string_| `SignalProcessing`|
| `metadata`| _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectmeta-v1-meta)_| Refer to the Kubernetes API documentation for fields of `metadata`.|
| `spec`| _[SignalProcessingSpec](#signalprocessingspec)_||
| `status`| _[SignalProcessingStatus](#signalprocessingstatus)_||


### SignalProcessingPhase

_Underlying type:_ _string_

SignalProcessingPhase represents the current phase of SignalProcessing reconciliation.
 Phase State Machine
 Capitalized phase values per Kubernetes API conventions

_Appears in:_
- [SignalProcessingStatus](#signalprocessingstatus)

_Validation:_
- Enum: [Pending Enriching Classifying Categorizing Completed Failed]

| Value| Description|
| ---| ---|
| `Pending`| PhasePending is the initial state when SignalProcessing is created.|
| `Enriching`| PhaseEnriching is when K8s context enrichment is in progress.|
| `Classifying`| PhaseClassifying is when environment/priority classification is in progress.|
| `Categorizing`| PhaseCategorizing is when business categorization is in progress.|
| `Completed`| PhaseCompleted is the terminal success state.|
| `Failed`| PhaseFailed is the terminal error state.|


### SignalProcessingSpec


SignalProcessingSpec defines the desired state of SignalProcessing.


 Spec Immutability
SignalProcessing represents an immutable event (signal enrichment).
Once created by RemediationOrchestrator, spec cannot be modified to ensure:
- Audit trail integrity (processed signal matches original signal)
- No signal data tampering during enrichment
- Consistent context passed to AIAnalysis

To reprocess a signal, delete and recreate the SignalProcessing CRD.

_Appears in:_
- [SignalProcessing](#signalprocessing)

| Field| Type| Description|
| ---| ---| ---|
| `remediationRequestRef`| _[ObjectReference](#objectreference)_| Reference to parent RemediationRequest|
| `signal`| _[SignalData](#signaldata)_| Signal data (copied from RemediationRequest for processing)|
| `enrichmentConfig`| _[EnrichmentConfig](#enrichmentconfig)_| Configuration for processing|


### SignalProcessingStatus


SignalProcessingStatus defines the observed state of SignalProcessing.


_Appears in:_
- [SignalProcessing](#signalprocessing)

| Field| Type| Description|
| ---| ---| ---|
| `observedGeneration`| _integer_| ObservedGeneration is the most recent generation observed by the controller.<br />Used to prevent duplicate reconciliations and ensure idempotency.<br />Per Standard pattern for all Kubernetes controllers.|
| `phase`| _[SignalProcessingPhase](#signalprocessingphase)_| Phase: Pending, Enriching, Classifying, Categorizing, Completed, Failed|
| `startTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| Processing timestamps|
| `completionTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_||
| `kubernetesContext`| _KubernetesContext_| Enrichment results|
| `environmentClassification`| _[EnvironmentClassification](#environmentclassification)_| Categorization results|
| `priorityAssignment`| _[PriorityAssignment](#priorityassignment)_||
| `businessClassification`| _BusinessClassification_||
| `severity`| _string_| Severity determination <br />Normalized severity determined by Rego policy: "critical", "high", "medium", "low", or "unknown"<br />Aligned with HAPI/workflow catalog severity levels for consistency across platform<br />Enables downstream services (AIAnalysis, RemediationOrchestrator, Notification)<br />to interpret alert urgency without understanding external severity schemes.|
| `policyHash`| _string_| PolicyHash is the SHA256 hash of the Rego policy used for severity determination<br />Provides audit trail and policy version tracking for compliance requirements<br />Expected format: 64-character hexadecimal string (SHA256 hash)|
| `signalMode`| _string_| SignalMode indicates whether this is a reactive or proactive signal.<br /> Proactive Signal Mode Classification<br /> Proactive Signal Mode Classification and Prompt Strategy<br />Set during the Classifying phase alongside severity, environment, and priority.<br />All signals MUST be classified — "reactive" is the default for unmapped types.|
| `signalName`| _string_| SignalName is the normalized signal name after proactive-to-base mapping.<br /> Signal Name Normalization<br />For proactive signals (e.g., "PredictedOOMKill"), this is the base name (e.g., "OOMKilled").<br />For reactive signals, this matches Spec.Signal.Name unchanged.<br />This is the AUTHORITATIVE signal name for all downstream consumers (RO, AA, HAPI).|
| `sourceSignalName`| _string_| SourceSignalName preserves the pre-normalization signal name for audit trail.<br /> Audit trail preservation (SOC2 CC7.4)<br />Only populated for proactive signals (e.g., "PredictedOOMKill").<br />Empty for reactive signals.|
| `conditions`| _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#condition-v1-meta) array_| Conditions for detailed status|
| `error`| _string_| Error information|
| `consecutiveFailures`| _integer_| ConsecutiveFailures tracks the number of consecutive transient failures.<br />Used with shared backoff for exponential retry delays .<br />Reset to 0 on successful phase transition.|
| `lastFailureTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| LastFailureTime records when the last failure occurred.<br />Used to determine if enough time has passed for retry.|


### SkipReason

_Underlying type:_ _string_

SkipReason represents the reason why a RemediationRequest was skipped.

_Appears in:_
- [RemediationRequestStatus](#remediationrequeststatus)

_Validation:_
- Enum: [RecentlyRemediated ResourceBusy ExhaustedRetries PreviousExecutionFailed]

| Value| Description|
| ---| ---|
| `RecentlyRemediated`||
| `ResourceBusy`||
| `ExhaustedRetries`||
| `PreviousExecutionFailed`||


### TargetContext


TargetContext captures target resource context.

_Appears in:_
- [NotificationContext](#notificationcontext)

| Field| Type| Description|
| ---| ---| ---|
| `targetResource`| _string_| TargetResource in "Kind/Name" format.|


### TargetResource


TargetResource identifies a Kubernetes resource by kind, name, and namespace.

_Appears in:_
- [EffectivenessAssessmentSpec](#effectivenessassessmentspec)

| Field| Type| Description|
| ---| ---| ---|
| `kind`| _string_| Kind is the Kubernetes resource kind (e.g., "Deployment", "StatefulSet").|
| `name`| _string_| Name is the resource name.|
| `namespace`| _string_| Namespace is the resource namespace.<br />Empty for cluster-scoped resources (e.g., Node, PersistentVolume).|


### TimeoutConfig


TimeoutConfig provides fine-grained timeout configuration for remediations.
Supports both global workflow timeout and per-phase timeouts for granular control.


_Appears in:_
- [RemediationRequestStatus](#remediationrequeststatus)

| Field| Type| Description|
| ---| ---| ---|
| `global`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| Global timeout for entire remediation workflow.<br />Overrides controller-level default (1 hour).|
| `processing`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| Processing phase timeout (SignalProcessing enrichment).<br />Overrides controller-level default (5 minutes).|
| `analyzing`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| Analyzing phase timeout (AIAnalysis investigation).<br />Overrides controller-level default (10 minutes).|
| `executing`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| Executing phase timeout (WorkflowExecution remediation).<br />Overrides controller-level default (30 minutes).|


### ValidationAttempt


ValidationAttempt contains details of a single HAPI validation attempt
Per HAPI retries up to 3 times with LLM self-correction
Each attempt feeds validation errors back to the LLM for correction

_Appears in:_
- [AIAnalysisStatus](#aianalysisstatus)

| Field| Type| Description|
| ---| ---| ---|
| `attempt`| _integer_| Attempt number (1, 2, or 3)|
| `workflowId`| _string_| WorkflowID that the LLM tried in this attempt|
| `isValid`| _boolean_| Whether validation passed (always false for failed attempts in history)|
| `errors`| _string array_| Validation errors encountered|
| `timestamp`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| When this attempt occurred|


### VerificationContext


VerificationContext captures EA verification results for completion notifications .
Enables programmatic routing (e.g., inconclusive outcomes -> escalation channel).

_Appears in:_
- [NotificationContext](#notificationcontext)

| Field| Type| Description|
| ---| ---| ---|
| `assessed`| _boolean_| Assessed indicates whether verification was performed at all.|
| `outcome`| _string_| Outcome is the high-level result: "passed", "completed", "partial", "inconclusive", "unavailable".<br />"completed" indicates all components were assessed but some scores < 1.0 .|
| `reason`| _string_| Reason maps to EffectivenessAssessment.Status.AssessmentReason.|
| `summary`| _string_| Summary is the operator-facing human-readable message.|
| `degraded`| _boolean_| Degraded indicates that the EA was unable to reliably compare pre- and<br />post-remediation state because hash capture failed .<br />Routing rules can match on this to escalate degraded notifications.|
| `degradedReason`| _string_| DegradedReason describes why the EA is degraded (e.g., RBAC Forbidden for<br />the target CRD). Empty when Degraded is false.|


### WorkflowContext


WorkflowContext captures selected workflow details.

_Appears in:_
- [NotificationContext](#notificationcontext)

| Field| Type| Description|
| ---| ---| ---|
| `selectedWorkflow`| _string_| SelectedWorkflow is the ID of the workflow selected by AI.|
| `confidence`| _string_| Confidence is the AI confidence score (as string, e.g. "0.95").|
| `workflowId`| _string_| WorkflowID is the ID of the executed workflow.|
| `executionEngine`| _string_| ExecutionEngine is the engine used to execute the workflow.|


## WorkflowExecution


WorkflowExecution is the Schema for the workflowexecutions API


| Field| Type| Description|
| ---| ---| ---|
| `apiVersion`| _string_| `kubernaut.ai/v1alpha1`|
| `kind`| _string_| `WorkflowExecution`|
| `metadata`| _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectmeta-v1-meta)_| Refer to the Kubernetes API documentation for fields of `metadata`.|
| `spec`| _[WorkflowExecutionSpec](#workflowexecutionspec)_||
| `status`| _[WorkflowExecutionStatus](#workflowexecutionstatus)_||


### WorkflowExecutionSpec


WorkflowExecutionSpec defines the desired state of WorkflowExecution

 Spec Immutability
WorkflowExecution represents an immutable event (workflow execution attempt).
Once created by RemediationOrchestrator, spec cannot be modified to ensure:
- Audit trail integrity (executed spec matches approved spec)
- No parameter tampering after HAPI validation
- No target resource changes after routing decisions

To change execution parameters, delete and recreate the WorkflowExecution.

_Appears in:_
- [WorkflowExecution](#workflowexecution)

| Field| Type| Description|
| ---| ---| ---|
| `remediationRequestRef`| _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#objectreference-v1-core)_| RemediationRequestRef references the parent RemediationRequest CRD|
| `workflowRef`| _[WorkflowRef](#workflowref)_| WorkflowRef contains the workflow catalog reference<br />Resolved from AIAnalysis.Status.SelectedWorkflow by RemediationOrchestrator|
| `targetResource`| _string_| TargetResource identifies the K8s resource being remediated<br />Used for resource locking  - prevents parallel workflows on same target<br />Format: "namespace/kind/name" for namespaced resources<br /> "kind/name" for cluster-scoped resources<br />Example: "payment/deployment/payment-api", "node/worker-node-1"|
| `parameters`| _object (keys:string, values:string)_| Parameters from LLM selection <br />Keys are UPPER_SNAKE_CASE for Tekton PipelineRun params|
| `confidence`| _float_| Confidence score from LLM (for audit trail)|
| `rationale`| _string_| Rationale from LLM (for audit trail)|
| `executionConfig`| _[ExecutionConfig](#executionconfig)_| ExecutionConfig contains minimal execution settings|


### WorkflowExecutionStatus


WorkflowExecutionStatus defines the observed state

_Appears in:_
- [WorkflowExecution](#workflowexecution)

| Field| Type| Description|
| ---| ---| ---|
| `observedGeneration`| _integer_| ObservedGeneration is the most recent generation observed by the controller.<br />Used to prevent duplicate reconciliations and ensure idempotency.<br />Per Standard pattern for all Kubernetes controllers.|
| `phase`| _string_| Phase tracks current execution stage<br />V1.0: Skipped phase removed - RO makes routing decisions before WFE creation|
| `startTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| StartTime when execution started|
| `completionTime`| _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#time-v1-meta)_| CompletionTime when execution completed (success or failure)|
| `duration`| _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#duration-v1-meta)_| Duration of the execution|
| `executionRef`| _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#localobjectreference-v1-core)_| ExecutionRef references the created execution resource (PipelineRun or Job)|
| `executionStatus`| _[ExecutionStatusSummary](#executionstatussummary)_| ExecutionStatus mirrors key execution resource status fields|
| `failureReason`| _string_| FailureReason explains why execution failed (if applicable)<br />DEPRECATED: Use FailureDetails for structured failure information|
| `failureDetails`| _[FailureDetails](#failuredetails)_| FailureDetails contains structured failure information<br />Populated when Phase=Failed|
| `blockClearance`| _[BlockClearanceDetails](#blockclearancedetails)_| BlockClearance tracks the clearing of PreviousExecutionFailed blocks<br />When set, allows new executions despite previous execution failure<br />Preserves audit trail of WHO cleared the block and WHY|
| `ephemeralCredentialIDs`| _integer array_| EphemeralCredentialIDs stores AWX credential IDs created by the ansible<br />executor for cleanup after execution . Written via the status<br />subresource to avoid violating spec immutability .|
| `executionEngine`| _string_| ExecutionEngine is the backend engine resolved from the DS workflow catalog<br />at runtime by the WE controller. Set once during Pending phase via<br />WorkflowQuerier.GetWorkflowSchemaMetadata; immutable thereafter.<br />Values: "tekton", "job", "ansible".|
| `serviceAccountName`| _string_| ServiceAccountName is the pre-existing ServiceAccount resolved from the<br />DS workflow catalog at runtime by the WE controller .<br />Set once during Pending phase via ResolveWorkflowCatalogMetadata; immutable<br />thereafter. If empty, K8s assigns the namespace's default SA (Job/Tekton)<br />or the Ansible executor falls back to the controller's in-cluster credentials.|
| `deduplicatedBy`| _string_| DeduplicatedBy stores the name of the original WorkflowExecution that owns<br />the conflicting execution resource. Set atomically inside AtomicStatusUpdate<br />when FailureDetails.Reason == Deduplicated (, M5 constraint).<br />Immutable after initial assignment.|
| `conditions`| _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#condition-v1-meta) array_| Conditions provide detailed status information|


### WorkflowOverride


WorkflowOverride allows operators to override the AI-recommended workflow
and/or parameters when approving a RemediationApprovalRequest .
The RO resolves the final spec from either this override or the AIA default.

_Appears in:_
- [RemediationApprovalRequestStatus](#remediationapprovalrequeststatus)

| Field| Type| Description|
| ---| ---| ---|
| `workflowName`| _string_| Name of a RemediationWorkflow CRD to use instead of the AI-recommended one.<br />If set, the webhook validates the RW exists and has CatalogStatus=Active.|
| `parameters`| _object (keys:string, values:string)_| Override parameters for the workflow execution.<br />If non-nil (including empty map), replaces the AI-recommended parameters.<br />If nil, the AI-recommended parameters are preserved.|
| `rationale`| _string_| Rationale for the override decision (audit trail).|


### WorkflowRef


WorkflowRef contains catalog-resolved workflow reference

_Appears in:_
- [WorkflowExecutionSpec](#workflowexecutionspec)

| Field| Type| Description|
| ---| ---| ---|
| `workflowId`| _string_| WorkflowID is the catalog lookup key|
| `version`| _string_| Version of the workflow|
| `executionBundle`| _string_| ExecutionBundle resolved from workflow catalog (Data Storage API)<br />OCI bundle reference for Tekton PipelineRun|
| `executionBundleDigest`| _string_| ExecutionBundleDigest for audit trail and reproducibility|
| `engineConfig`| _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v/#json-v1-apiextensions-k8s-io)_| EngineConfig holds engine-specific configuration .<br />For ansible: \{"playbookPath": "...", "jobTemplateName": "...", "inventoryName": "..."\}<br />For tekton/job: nil.|


### WorkflowReference


WorkflowReference captures workflow catalog information for audit trail.
Used in RemediationRequestStatus.SelectedWorkflowRef .

_Appears in:_
- [RemediationRequestStatus](#remediationrequeststatus)

| Field| Type| Description|
| ---| ---| ---|
| `workflowId`| _string_| WorkflowID is the catalog lookup key|
| `version`| _string_| Version of the workflow|
| `executionBundle`| _string_| ExecutionBundle resolved from workflow catalog<br />OCI bundle reference for Tekton PipelineRun|
| `executionBundleDigest`| _string_| ExecutionBundleDigest for audit trail and reproducibility|


