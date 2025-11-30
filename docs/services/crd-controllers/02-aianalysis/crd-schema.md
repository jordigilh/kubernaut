## CRD Schema

**Version**: 2.0
**Last Updated**: 2025-11-28
**Status**: ✅ Aligned with DD-CONTRACT-001, ADR-041, ADR-043

**Full Schema**: See [docs/design/CRD/03_AI_ANALYSIS_CRD.md](../../design/CRD/03_AI_ANALYSIS_CRD.md)

**Note**: The examples below show the conceptual structure. The authoritative OpenAPI v3 schema is defined in `03_AI_ANALYSIS_CRD.md`.

---

## Key Design Decisions

| Document | Impact on CRD |
|----------|---------------|
| **DD-CONTRACT-001** | AIAnalysis uses `selectedWorkflow` instead of `recommendations` |
| **ADR-041** | LLM response contract defines `selected_workflow` format |
| **ADR-043** | Workflow schema definition (catalog integration) |
| **ADR-040** | Approval orchestration handled by RO, not AIAnalysis |

---

### Spec Fields

```yaml
apiVersion: kubernaut.io/v1alpha1
kind: AIAnalysis
metadata:
  name: aianalysis-abc123
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: kubernaut.io/v1alpha1
    kind: RemediationRequest
    name: remediation-abc12345
    uid: xyz789
    controller: true
spec:
  # Parent RemediationRequest reference (for audit/lineage only)
  remediationRequestRef:
    name: remediation-abc12345
    namespace: kubernaut-system

  # SELF-CONTAINED analysis request (complete data snapshot from SignalProcessing)
  # No need to read SignalProcessing - all enriched data copied here at creation
  analysisRequest:
    signalContext:
      # Basic signal identifiers
      fingerprint: "abc123def456"
      severity: critical
      environment: production
      businessPriority: p0

      # COMPLETE enriched payload (snapshot from SignalProcessing.status)
      enrichedPayload:
        originalSignal:
          labels:
            alertname: PodOOMKilled
            namespace: production
            pod: web-app-789
          annotations:
            summary: "Pod killed due to OOM"
            description: "Memory limit exceeded"

        kubernetesContext:
          podDetails:
            name: web-app-789
            namespace: production
            containers:
            - name: app
              memoryLimit: "512Mi"
              memoryUsage: "498Mi"
          deploymentDetails:
            name: web-app
            replicas: 3
          nodeDetails:
            name: node-1
            capacity: {...}

        businessContext:
          serviceOwner: "platform-team"
          criticality: "high"
          sla: "99.9%"

    analysisTypes:
    - investigation
    - root-cause
    - workflow-selection  # NEW: Replaces "recommendation-generation"

    investigationScope:
      timeWindow: "24h"
      resourceScope:
      - kind: Pod
        namespace: production
      correlationDepth: detailed
      includeHistoricalPatterns: true
```

---

### Status Fields (DD-CONTRACT-001 Aligned)

```yaml
status:
  # Phase tracks current analysis stage
  # NOTE: AIAnalysis does NOT have "Approving" phase - it completes with approvalRequired=true
  # RemediationOrchestrator handles approval orchestration (ADR-040)
  phase: Completed  # Pending, Investigating, Analyzing, Completed, Failed

  # Phase transition tracking
  phaseTransitions:
    investigating: "2025-01-15T10:00:00Z"
    analyzing: "2025-01-15T10:15:00Z"
    completed: "2025-01-15T10:30:00Z"

  # Investigation results (Phase 1)
  investigationResult:
    rootCauseHypotheses:
    - hypothesis: "Pod memory limit too low"
      confidence: 0.85
      evidence:
      - "OOMKilled events in pod history"
      - "Memory usage consistently near 95%"
    correlatedSignals:
    - fingerprint: "abc123def456"
      timestamp: "2025-01-15T10:30:00Z"
    investigationReport: "..."
    contextualAnalysis: "..."

  # ================================================================
  # SELECTED WORKFLOW (DD-CONTRACT-001 v1.2, ADR-041)
  # Replaces old "recommendations" format
  # containerImage resolved by HolmesGPT-API during MCP search
  # ================================================================
  selectedWorkflow:
    # WorkflowID is the catalog lookup key
    workflowId: "oomkill-increase-memory"

    # Version of the selected workflow
    version: "1.0.0"

    # ContainerImage - OCI bundle reference (resolved by HolmesGPT-API from catalog)
    # RO passes this through to WorkflowExecution without additional lookup
    containerImage: "quay.io/kubernaut/workflow-oomkill:v1.0.0"

    # ContainerDigest for audit trail and reproducibility
    containerDigest: "sha256:abc123def456..."

    # Confidence score from MCP search (0.0-1.0)
    confidence: 0.92

    # Parameters populated by LLM based on RCA (per DD-WORKFLOW-003)
    # Keys are UPPER_SNAKE_CASE per Tekton convention
    parameters:
      NAMESPACE: "production"
      DEPLOYMENT_NAME: "web-app"
      NEW_MEMORY_LIMIT: "1Gi"

    # Rationale explains why this workflow was selected
    rationale: "MCP search matched OOMKilled signal with high historical success rate. Memory increase has 88% success rate for similar incidents."

  # Alternative workflows considered (backup options)
  alternativeWorkflows:
  - workflowId: "oomkill-restart-pods"
    version: "1.0.0"
    confidence: 0.65
    rationale: "Lower confidence due to only addressing symptoms, not root cause"

  # ================================================================
  # APPROVAL CONTEXT (ADR-040, DD-CONTRACT-001)
  # RO orchestrates approval based on these fields
  # ================================================================

  # ApprovalRequired indicates if manual approval is needed
  # Triggers RemediationOrchestrator to create RemediationApprovalRequest (ADR-040)
  approvalRequired: false  # true if confidence < 80%

  # ApprovalReason explains why approval is required (when approvalRequired=true)
  approvalReason: ""  # "Confidence 65% below 80% threshold"

  # ApprovalContext provides rich context for operator decision
  # Only populated when approvalRequired=true
  approvalContext:
    investigationSummary: "Memory leak detected in payment processing pods..."
    evidenceCollected:
    - "OOMKilled events in last 24h"
    - "Linear memory growth 50MB/hour per pod"
    - "No recent deployments to production namespace"
    alternativesConsidered:
    - workflowId: "oomkill-restart-pods"
      rationale: "Would address symptoms but not root cause"
      confidence: 0.45

  # WorkflowExecutionRef references the created WorkflowExecution CRD
  # Populated by RO after creating WorkflowExecution
  workflowExecutionRef:
    name: aianalysis-abc123-workflow-1
    namespace: kubernaut-system

  # Observability
  conditions:
  - type: InvestigationComplete
    status: "True"
    reason: RootCauseIdentified
  - type: WorkflowSelected
    status: "True"
    reason: HighConfidenceMatch
  - type: AnalysisComplete
    status: "True"
    reason: WorkflowSelectedSuccessfully
```

---

## Go Type Definitions (DD-CONTRACT-001)

```go
// pkg/api/aianalysis/v1alpha1/types.go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIAnalysisSpec defines the desired state of AIAnalysis
type AIAnalysisSpec struct {
    // RemediationRequestRef references the parent RemediationRequest
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // AnalysisRequest contains the self-contained analysis request
    AnalysisRequest AnalysisRequest `json:"analysisRequest"`
}

// AnalysisRequest contains all data needed for AI analysis
type AnalysisRequest struct {
    // SignalContext contains the enriched signal data
    SignalContext SignalContext `json:"signalContext"`

    // AnalysisTypes specifies what analysis to perform
    AnalysisTypes []string `json:"analysisTypes"`

    // InvestigationScope defines analysis boundaries
    InvestigationScope InvestigationScope `json:"investigationScope,omitempty"`
}

// AIAnalysisStatus defines the observed state of AIAnalysis
type AIAnalysisStatus struct {
    // Phase tracks current analysis stage
    // NOTE: AIAnalysis does NOT have "Approving" phase - it completes with approvalRequired=true
    // RemediationOrchestrator handles the approval orchestration (ADR-040)
    // +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Completed;Failed
    Phase string `json:"phase"`

    // PhaseTransitions records timestamps for each phase
    PhaseTransitions map[string]metav1.Time `json:"phaseTransitions,omitempty"`

    // InvestigationResult contains HolmesGPT investigation findings
    InvestigationResult *InvestigationResult `json:"investigationResult,omitempty"`

    // SelectedWorkflow contains the LLM-selected workflow (per ADR-041)
    // Populated after successful investigation and analysis
    SelectedWorkflow *SelectedWorkflow `json:"selectedWorkflow,omitempty"`

    // AlternativeWorkflows contains backup options (per ADR-041)
    AlternativeWorkflows []AlternativeWorkflow `json:"alternativeWorkflows,omitempty"`

    // ApprovalRequired indicates if manual approval is needed
    // Triggers RemediationOrchestrator to create RemediationApprovalRequest (ADR-040)
    ApprovalRequired bool `json:"approvalRequired,omitempty"`

    // ApprovalReason explains why approval is required
    ApprovalReason string `json:"approvalReason,omitempty"`

    // ApprovalContext provides rich context for operator decision
    ApprovalContext *ApprovalContext `json:"approvalContext,omitempty"`

    // WorkflowExecutionRef references the created WorkflowExecution CRD
    WorkflowExecutionRef *corev1.LocalObjectReference `json:"workflowExecutionRef,omitempty"`

    // Conditions provide detailed status information
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// SelectedWorkflow represents the LLM's workflow selection (per ADR-041, DD-CONTRACT-001 v1.2)
// NOTE: containerImage and containerDigest are resolved by HolmesGPT-API during MCP search
// RO passes these through to WorkflowExecution (no separate catalog lookup needed)
type SelectedWorkflow struct {
    // WorkflowID is the catalog lookup key
    // +kubebuilder:validation:Required
    WorkflowID string `json:"workflowId"`

    // Version of the selected workflow
    Version string `json:"version,omitempty"`

    // ContainerImage is the OCI bundle reference (resolved by HolmesGPT-API from catalog)
    // +kubebuilder:validation:Required
    ContainerImage string `json:"containerImage"`

    // ContainerDigest for audit trail and reproducibility (resolved by HolmesGPT-API)
    ContainerDigest string `json:"containerDigest,omitempty"`

    // Confidence score from MCP search (0.0-1.0)
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=1
    Confidence float64 `json:"confidence"`

    // Parameters populated by LLM based on RCA (per DD-WORKFLOW-003)
    // Keys are UPPER_SNAKE_CASE per Tekton convention
    Parameters map[string]string `json:"parameters"`

    // Rationale explains why this workflow was selected
    Rationale string `json:"rationale"`
}

// AlternativeWorkflow represents backup workflow options
type AlternativeWorkflow struct {
    WorkflowID string  `json:"workflowId"`
    Version    string  `json:"version,omitempty"`
    Confidence float64 `json:"confidence"`
    Rationale  string  `json:"rationale,omitempty"`
}

// ApprovalContext provides rich context for operator approval decisions
type ApprovalContext struct {
    // InvestigationSummary provides a brief summary of findings
    InvestigationSummary string `json:"investigationSummary,omitempty"`

    // EvidenceCollected lists key evidence points
    EvidenceCollected []string `json:"evidenceCollected,omitempty"`

    // AlternativesConsidered lists other workflow options
    AlternativesConsidered []AlternativeWorkflow `json:"alternativesConsidered,omitempty"`
}

// InvestigationResult contains the HolmesGPT investigation findings
type InvestigationResult struct {
    // RootCauseHypotheses contains potential root causes
    RootCauseHypotheses []RootCauseHypothesis `json:"rootCauseHypotheses,omitempty"`

    // CorrelatedSignals contains related signals
    CorrelatedSignals []CorrelatedSignal `json:"correlatedSignals,omitempty"`

    // InvestigationReport contains the full investigation report
    InvestigationReport string `json:"investigationReport,omitempty"`

    // ContextualAnalysis contains contextual analysis
    ContextualAnalysis string `json:"contextualAnalysis,omitempty"`
}

// RootCauseHypothesis represents a potential root cause
type RootCauseHypothesis struct {
    Hypothesis string   `json:"hypothesis"`
    Confidence float64  `json:"confidence"`
    Evidence   []string `json:"evidence,omitempty"`
}

// CorrelatedSignal represents a related signal
type CorrelatedSignal struct {
    Fingerprint string      `json:"fingerprint"`
    Timestamp   metav1.Time `json:"timestamp"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="WorkflowID",type=string,JSONPath=`.status.selectedWorkflow.workflowId`
//+kubebuilder:printcolumn:name="Confidence",type=number,JSONPath=`.status.selectedWorkflow.confidence`
//+kubebuilder:printcolumn:name="ApprovalRequired",type=boolean,JSONPath=`.status.approvalRequired`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AIAnalysis is the Schema for the aianalyses API
type AIAnalysis struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   AIAnalysisSpec   `json:"spec,omitempty"`
    Status AIAnalysisStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AIAnalysisList contains a list of AIAnalysis
type AIAnalysisList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []AIAnalysis `json:"items"`
}

func init() {
    SchemeBuilder.Register(&AIAnalysis{}, &AIAnalysisList{})
}
```

---

## Complete Example: AIAnalysis with High Confidence

```yaml
apiVersion: kubernaut.io/v1alpha1
kind: AIAnalysis
metadata:
  name: aianalysis-payment-oom-001
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: kubernaut.io/v1alpha1
    kind: RemediationRequest
    name: remediation-payment-oom
    uid: abc-123-def-456
    controller: true
spec:
  remediationRequestRef:
    name: remediation-payment-oom
    namespace: kubernaut-system
  analysisRequest:
    signalContext:
      fingerprint: "sha256:payment-oom-fingerprint"
      severity: critical
      environment: production
      businessPriority: p0
      enrichedPayload:
        originalSignal:
          labels:
            alertname: PodOOMKilled
            namespace: payment
            pod: payment-api-5d8f9b7c6-x9z2k
        kubernetesContext:
          podDetails:
            name: payment-api-5d8f9b7c6-x9z2k
            namespace: payment
            containers:
            - name: api
              memoryLimit: "512Mi"
              memoryUsage: "510Mi"
          deploymentDetails:
            name: payment-api
            replicas: 3
            availableReplicas: 2
    analysisTypes:
    - investigation
    - root-cause
    - workflow-selection
status:
  phase: Completed
  phaseTransitions:
    investigating: "2025-11-28T10:00:00Z"
    analyzing: "2025-11-28T10:05:00Z"
    completed: "2025-11-28T10:10:00Z"

  investigationResult:
    rootCauseHypotheses:
    - hypothesis: "Container memory limit insufficient for current workload"
      confidence: 0.92
      evidence:
      - "Memory usage at 99.6% of limit before OOMKill"
      - "3 OOMKill events in past 24 hours"
      - "Memory usage growth correlates with traffic increase"
    investigationReport: |
      Investigation identified memory pressure as root cause.
      Pod payment-api-5d8f9b7c6-x9z2k was terminated due to exceeding
      its 512Mi memory limit. Analysis of memory metrics shows linear
      growth correlating with increased traffic over the past 4 hours.

  # HIGH CONFIDENCE - No approval required
  selectedWorkflow:
    workflowId: "oomkill-increase-memory"
    version: "1.0.0"
    containerImage: "quay.io/kubernaut/workflow-oomkill:v1.0.0"  # Resolved by HolmesGPT-API
    containerDigest: "sha256:abc123def456789..."                  # For audit trail
    confidence: 0.92
    parameters:
      NAMESPACE: "payment"
      DEPLOYMENT_NAME: "payment-api"
      NEW_MEMORY_LIMIT: "1Gi"
    rationale: |
      Selected based on:
      1. High confidence root cause (memory limit too low)
      2. Historical success rate of 88% for similar incidents
      3. Low risk - only increases resource allocation
      4. No recent deployments that could explain memory growth

  alternativeWorkflows:
  - workflowId: "oomkill-restart-pods"
    version: "1.0.0"
    confidence: 0.45
    rationale: "Would temporarily resolve but not address root cause"
  - workflowId: "oomkill-scale-deployment"
    version: "1.0.0"
    confidence: 0.60
    rationale: "Could distribute load but doesn't fix per-pod memory issue"

  approvalRequired: false  # confidence >= 80%

  conditions:
  - type: InvestigationComplete
    status: "True"
    reason: RootCauseIdentified
    message: "Root cause identified with 92% confidence"
    lastTransitionTime: "2025-11-28T10:05:00Z"
  - type: WorkflowSelected
    status: "True"
    reason: HighConfidenceMatch
    message: "Workflow oomkill-increase-memory selected with 92% confidence"
    lastTransitionTime: "2025-11-28T10:10:00Z"
  - type: AnalysisComplete
    status: "True"
    reason: WorkflowSelectedSuccessfully
    message: "Analysis completed, workflow ready for execution"
    lastTransitionTime: "2025-11-28T10:10:00Z"
```

---

## Complete Example: AIAnalysis with Low Confidence (Requires Approval)

```yaml
apiVersion: kubernaut.io/v1alpha1
kind: AIAnalysis
metadata:
  name: aianalysis-payment-unknown-001
  namespace: kubernaut-system
spec:
  remediationRequestRef:
    name: remediation-payment-unknown
    namespace: kubernaut-system
  analysisRequest:
    signalContext:
      fingerprint: "sha256:payment-unknown-fingerprint"
      severity: warning
      environment: production
      businessPriority: p1
status:
  phase: Completed  # NOTE: Completed, NOT "Approving"

  investigationResult:
    rootCauseHypotheses:
    - hypothesis: "Possible memory leak in application code"
      confidence: 0.55
      evidence:
      - "Gradual memory increase over 72 hours"
      - "No correlation with traffic patterns"
    - hypothesis: "External dependency causing memory pressure"
      confidence: 0.35
      evidence:
      - "New database client library deployed 3 days ago"

  # LOW CONFIDENCE - Requires approval
  selectedWorkflow:
    workflowId: "memory-leak-collect-diagnostics"
    version: "1.0.0"
    containerImage: "quay.io/kubernaut/workflow-diagnostics:v1.0.0"  # Resolved by HolmesGPT-API
    containerDigest: "sha256:def456abc789..."
    confidence: 0.65  # Below 80% threshold
    parameters:
      NAMESPACE: "payment"
      DEPLOYMENT_NAME: "payment-api"
      DIAGNOSTIC_DURATION: "30m"
    rationale: |
      Uncertain root cause - recommending diagnostic collection
      before any remediation action. Low confidence due to:
      1. Multiple possible root causes
      2. No clear historical pattern match
      3. Novel failure signature

  alternativeWorkflows:
  - workflowId: "oomkill-increase-memory"
    version: "1.0.0"
    confidence: 0.45
    rationale: "May address symptom but could mask underlying issue"

  # APPROVAL REQUIRED
  approvalRequired: true
  approvalReason: "Confidence 65% below 80% threshold - multiple possible root causes"

  approvalContext:
    investigationSummary: |
      Uncertain root cause for memory growth in payment-api. Two hypotheses:
      1. Application memory leak (55% confidence)
      2. External dependency issue (35% confidence)
      Recommending diagnostic collection to gather more data before remediation.
    evidenceCollected:
    - "Gradual memory increase over 72 hours"
    - "No correlation with traffic patterns"
    - "New database client library deployed 3 days ago"
    alternativesConsidered:
    - workflowId: "oomkill-increase-memory"
      confidence: 0.45
      rationale: "May address symptom but could mask underlying issue"

  conditions:
  - type: InvestigationComplete
    status: "True"
    reason: RootCauseUncertain
    message: "Investigation complete but root cause uncertain"
  - type: WorkflowSelected
    status: "True"
    reason: LowConfidenceMatch
    message: "Workflow selected with 65% confidence - approval required"
  - type: AnalysisComplete
    status: "True"
    reason: ApprovalRequired
    message: "Analysis complete, awaiting manual approval"
```

---

## Migration from v1.x Schema

| Old Field (v1.x) | New Field (v2.0) | Notes |
|------------------|------------------|-------|
| `recommendations[]` | `selectedWorkflow` | Single workflow, not array |
| `recommendations[].action` | `selectedWorkflow.workflowId` | Uses catalog ID |
| `recommendations[].parameters` | `selectedWorkflow.parameters` | UPPER_SNAKE_CASE keys |
| `recommendations[].effectivenessProbability` | `selectedWorkflow.confidence` | Same semantic |
| `recommendations[].explanation` | `selectedWorkflow.rationale` | Renamed |
| `approvalStatus` | Moved to RemediationApprovalRequest CRD | Per ADR-040 |
| `approvedBy`, `approvalTime`, etc. | Moved to RemediationApprovalRequest CRD | Per ADR-040 |

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| **DD-CONTRACT-001** | Authoritative for AIAnalysis ↔ WorkflowExecution contract |
| **ADR-041** | LLM Response Contract (defines `selected_workflow` format) |
| **ADR-043** | Workflow Schema Definition (catalog integration) |
| **ADR-040** | RemediationApprovalRequest Architecture |
| **DD-WORKFLOW-003** | Parameterized Actions (UPPER_SNAKE_CASE parameters) |
| **BR-AI-075** | Workflow Selection Output Format |
| **BR-AI-076** | Approval Context for Low Confidence |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.1 | 2025-11-28 | Added `containerImage` and `containerDigest` to `SelectedWorkflow` (resolved by HolmesGPT-API during MCP search). RO passes through without catalog lookup. Per DD-CONTRACT-001 v1.2. |
| 2.0 | 2025-11-28 | **Breaking**: Replaced `recommendations[]` with `selectedWorkflow` per DD-CONTRACT-001 and ADR-041. Removed approval lifecycle fields (moved to RemediationApprovalRequest per ADR-040). Added `alternativeWorkflows`. |
| 1.x | Prior | Legacy schema with `recommendations[]` and embedded approval fields |

