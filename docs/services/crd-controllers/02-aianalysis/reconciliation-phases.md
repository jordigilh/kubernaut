## Reconciliation Architecture

### Phase Transitions

```
investigating → analyzing → recommending → completed
     ↓              ↓              ↓              ↓
  (15 min)      (15 min)       (15 min)       (final)
```

### Phase Breakdown

#### 1. **investigating** (BR-AI-011, BR-AI-012, BR-AI-013)

**Purpose**: Trigger HolmesGPT investigation and gather root cause evidence

**Actions**:
- Trigger HolmesGPT investigation via HTTP API (port 8080)
- Retrieve historical patterns from Data Storage Service (BR-AI-011)
- Correlate related alerts across time windows (BR-AI-013)
- Identify root cause candidates with evidence (BR-AI-012)
- Update `status.investigationResult` with findings

**Timeout**: 15 minutes (configurable via annotation)

**Transition Criteria**:
```go
if investigationComplete && rootCauseIdentified {
    phase = "analyzing"
} else if timeout {
    phase = "failed"
    reason = "investigation_timeout"
}
```

**Example CRD Update**:
```yaml
status:
  phase: investigating
  investigationResult:
    rootCauseHypotheses:
    - hypothesis: "Pod memory limit too low"
      confidence: 0.85
      evidence:
      - "OOMKilled events in pod history"
      - "Memory usage consistently near 95% of limit"
    correlatedAlerts:
    - fingerprint: "abc123def456"
      timestamp: "2025-01-15T10:30:00Z"
```

#### 2. **analyzing** (BR-AI-001, BR-AI-002, BR-AI-003, BR-AI-021, BR-AI-023)

**Purpose**: Perform contextual AI analysis and validate results

**Actions**:
- Execute diagnostic, predictive, and prescriptive analysis (BR-AI-002)
- Generate confidence scores for analysis results (BR-AI-003)
- Validate AI responses for completeness (BR-AI-021)
- Detect and handle AI hallucinations (BR-AI-023)
- Perform contextual analysis of Kubernetes state (BR-AI-001)
- Update `status.analysisResult` with validated findings

**Timeout**: 15 minutes (configurable via annotation)

**Transition Criteria**:
```go
if analysisComplete && validationPassed && confidenceAboveThreshold {
    phase = "recommending"
} else if hallucinationDetected || validationFailed {
    phase = "failed"
    reason = "invalid_ai_response"
}
```

**Example CRD Update**:
```yaml
status:
  phase: analyzing
  analysisResult:
    contextualAnalysis: "Memory pressure due to insufficient resource limits"
    analysisTypes:
    - type: diagnostic
      result: "Container memory limit (512Mi) insufficient for workload"
      confidence: 0.9
    - type: predictive
      result: "Pod will continue OOMKill cycle without intervention"
      confidence: 0.85
    - type: prescriptive
      result: "Increase memory limit to 1Gi based on historical usage"
      confidence: 0.88
    validationStatus:
      completeness: true
      hallucinationDetected: false
      confidenceThresholdMet: true
```

#### 3. **recommending** (BR-AI-006, BR-AI-007, BR-AI-008, BR-AI-009, BR-AI-010)

**Purpose**: Generate and rank remediation recommendations

**Actions**:
- Generate remediation recommendations from AI analysis (BR-AI-006)
- Rank recommendations by effectiveness probability (BR-AI-007)
- Incorporate historical success rates from vector DB (BR-AI-008)
- Apply constraint-based filtering (environment, RBAC) (BR-AI-009)
- Provide explanations with supporting evidence (BR-AI-010)
- **Validate recommendation dependencies** (BR-AI-051, BR-AI-052, BR-AI-053)
- Update `status.recommendations` with ranked actions

**Timeout**: 15 minutes (configurable via annotation)

**Dependency Validation** (BR-AI-051, BR-AI-052, BR-AI-053):
```go
// Validate dependencies before transitioning to completed
func (r *AIAnalysisReconciler) validateRecommendationDependencies(
    recommendations []Recommendation,
) error {
    // BR-AI-051: Validate dependency completeness and correctness
    if err := validateDependencyReferences(recommendations); err != nil {
        return fmt.Errorf("dependency validation failed: %w", err)
    }

    // BR-AI-052: Detect circular dependencies
    if err := detectCircularDependencies(recommendations); err != nil {
        log.Error(err, "Circular dependency detected, falling back to sequential execution")
        // Fallback: Convert to sequential order
        recommendations = convertToSequentialOrder(recommendations)
    }

    // BR-AI-053: Handle missing dependencies
    for i, rec := range recommendations {
        if rec.Dependencies == nil {
            // Default to empty array (no dependencies)
            recommendations[i].Dependencies = []string{}
        }
    }

    return nil
}

// BR-AI-051: Validate all dependency IDs reference valid recommendations
func validateDependencyReferences(recommendations []Recommendation) error {
    recommendationIDs := make(map[string]bool)
    for _, rec := range recommendations {
        recommendationIDs[rec.ID] = true
    }

    for _, rec := range recommendations {
        for _, depID := range rec.Dependencies {
            if !recommendationIDs[depID] {
                return fmt.Errorf("recommendation %s has invalid dependency: %s (not found in recommendations list)", rec.ID, depID)
            }
            if depID == rec.ID {
                return fmt.Errorf("recommendation %s cannot depend on itself", rec.ID)
            }
        }
    }

    return nil
}

// BR-AI-052: Detect circular dependencies using topological sort
func detectCircularDependencies(recommendations []Recommendation) error {
    // Build adjacency list
    graph := make(map[string][]string)
    inDegree := make(map[string]int)

    for _, rec := range recommendations {
        graph[rec.ID] = rec.Dependencies
        if _, exists := inDegree[rec.ID]; !exists {
            inDegree[rec.ID] = 0
        }
        for _, dep := range rec.Dependencies {
            inDegree[rec.ID]++
        }
    }

    // Topological sort (Kahn's algorithm)
    queue := []string{}
    for id, degree := range inDegree {
        if degree == 0 {
            queue = append(queue, id)
        }
    }

    visited := 0
    for len(queue) > 0 {
        current := queue[0]
        queue = queue[1:]
        visited++

        // Remove edges from current node
        for _, neighbor := range graph[current] {
            inDegree[neighbor]--
            if inDegree[neighbor] == 0 {
                queue = append(queue, neighbor)
            }
        }
    }

    // If not all nodes visited, there's a cycle
    if visited != len(recommendations) {
        return fmt.Errorf("circular dependency detected in recommendation graph")
    }

    return nil
}
```

**Transition Criteria**:
```go
if recommendationsGenerated && constraintsApplied && dependenciesValidated {
    phase = "completed"
    createWorkflowExecutionCRD()
} else if dependencyValidationFailed {
    // Fallback to sequential execution or retry
    log.Warn("Dependency validation failed, using fallback strategy")
}
```

**Example CRD Update**:
```yaml
status:
  phase: recommending
  recommendations:
  - action: "increase-memory-limit"
    targetResource:
      kind: Deployment
      name: web-app
      namespace: production
    parameters:
      newMemoryLimit: "1Gi"
    effectivenessProbability: 0.92
    historicalSuccessRate: 0.88
    riskLevel: low
    explanation: "Historical data shows 88% success rate for memory increase in similar scenarios"
    supportingEvidence:
    - "15 similar cases resolved by memory increase"
    - "No side effects observed in production rollouts"
    constraints:
      environmentAllowed: [production, staging]
      rbacRequired: ["apps/deployments:update"]
```

#### 4. **completed** (BR-AI-014)

**Purpose**: Finalize analysis and create workflow execution

**Actions**:
- Create WorkflowExecution CRD with top recommendation (owner reference)
- Generate investigation report (BR-AI-014)
- Update audit database with analysis metadata
- Emit Kubernetes event: `AIAnalysisCompleted`
- Set `status.phase = "completed"`

**No Timeout** (terminal state)

**Example CRD Update**:
```yaml
status:
  phase: completed
  workflowExecutionRef:
    name: aianalysis-abc123-workflow-1
    namespace: kubernaut-system
  investigationReport: |
    Root Cause: Pod memory limit insufficient for workload demands
    Evidence: OOMKilled events, 95% memory utilization
    Recommendation: Increase memory limit to 1Gi
    Expected Impact: 92% resolution probability
    Historical Success: 88% in similar scenarios
  completionTime: "2025-01-15T11:15:00Z"
```


---

### CRD-Based Coordination Patterns

#### Event-Driven Coordination

This service uses **CRD-based reconciliation** for coordination with RemediationRequest controller and approval workflow:

1. **Created By**: RemediationRequest controller creates AIAnalysis CRD (with owner reference)
2. **Watch Pattern (Upstream)**: RemediationRequest watches AIAnalysis status for completion
3. **Watch Pattern (Downstream)**: AIAnalysis watches AIApprovalRequest status for approval
4. **Status Propagation**: Status updates trigger RemediationRequest reconciliation automatically (<1s latency)
5. **Event Emission**: Emit Kubernetes events for operational visibility

**Coordination Flow (Two Layers)**:
```
Layer 1: RemediationRequest → AIAnalysis
    RemediationRequest.status.overallPhase = "analyzing"
        ↓
    RemediationRequest Controller creates AIAnalysis CRD
        ↓
    AIAnalysis Controller reconciles (this controller)
        ↓
    AIAnalysis.status.phase = "completed"
        ↓ (watch trigger in RemediationRequest)
    RemediationRequest Controller reconciles (detects completion)
        ↓
    RemediationRequest Controller creates WorkflowExecution CRD

Layer 2: AIAnalysis → AIApprovalRequest → Approval
    AIAnalysis.status.phase = "recommendations"
        ↓
    AIAnalysis Controller creates AIApprovalRequest CRD (owned)
        ↓
    AIApprovalRequest Controller watches for Approval CRD
        ↓ (manual/auto approval via Rego policy)
    Approval CRD created
        ↓ (watch trigger)
    AIApprovalRequest.status.approved = true
        ↓ (watch trigger in AIAnalysis)
    AIAnalysis Controller reconciles
        ↓
    AIAnalysis.status.phase = "completed"
```

---

#### Owner Reference Management

**This CRD (AIAnalysis)**:
- **Owned By**: RemediationRequest (parent CRD)
- **Owner Reference**: Set at creation by RemediationRequest controller
- **Cascade Deletion**: Deleted automatically when RemediationRequest is deleted
- **Owns**: AIApprovalRequest (child CRD for approval workflow)
- **Watches**: AIApprovalRequest (for approval status changes)

**Two-Layer Coordination Pattern**:

AIAnalysis is a **middle controller** in the remediation workflow:
- ✅ **Owned by RemediationRequest**: Parent controller manages lifecycle
- ✅ **Creates AIApprovalRequest**: Child CRD for approval workflow
- ✅ **Watches AIApprovalRequest**: Event-driven approval detection
- ✅ **Does NOT create WorkflowExecution**: RemediationRequest does this after AIAnalysis completes

**Lifecycle**:
```
RemediationRequest Controller
    ↓ (creates with owner reference)
AIAnalysis CRD
    ↓ (investigates with HolmesGPT)
AIAnalysis.status.phase = "recommendations"
    ↓ (creates with owner reference)
AIApprovalRequest CRD
    ↓ (watches for approval)
AIApprovalRequest.status.approved = true
    ↓ (watch trigger)
AIAnalysis.status.phase = "completed"
    ↓ (watch trigger in RemediationRequest)
RemediationRequest Controller (creates WorkflowExecution)
```

---

#### No Direct HTTP Calls Between Controllers

**Anti-Pattern (Avoided)**: ❌ AIAnalysis calling WorkflowExecution or other controllers via HTTP

**Correct Pattern (Used)**: ✅ CRD status update + RemediationRequest watch-based coordination

**Why This Matters**:
- **Reliability**: CRD status persists in etcd (HTTP calls can fail silently)
- **Observability**: Status visible via `kubectl get aianalysis` (HTTP calls are opaque)
- **Kubernetes-Native**: Leverages built-in watch/reconcile patterns (no custom HTTP infrastructure)
- **Decoupling**: AIAnalysis doesn't need to know about WorkflowExecution existence or endpoint
- **Approval Workflow**: AIApprovalRequest is also a CRD (Kubernetes-native approval pattern)

**What AIAnalysis Does NOT Do**:
- ❌ Call WorkflowExecution controller via HTTP
- ❌ Create WorkflowExecution CRD (RemediationRequest does this)
- ❌ Watch WorkflowExecution status (RemediationRequest does this)
- ❌ Coordinate directly with Workflow Execution Service

**What AIAnalysis DOES Do**:
- ✅ Process its own AIAnalysis CRD
- ✅ Create AIApprovalRequest CRD (owned child)
- ✅ Watch AIApprovalRequest status for approval
- ✅ Update its own status to "completed" after approval
- ✅ Trust RemediationRequest to create WorkflowExecution

---

#### Watch Configuration

**1. RemediationRequest Watches AIAnalysis (Upstream)**:

```go
// In RemediationRequestReconciler.SetupWithManager()
err = c.Watch(
    &source.Kind{Type: &aianalysisv1.AIAnalysis{}},
    handler.EnqueueRequestsFromMapFunc(r.aiAnalysisToRemediation),
)

// Mapping function
func (r *RemediationRequestReconciler) aiAnalysisToRemediation(obj client.Object) []ctrl.Request {
    ai := obj.(*aianalysisv1.AIAnalysis)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      ai.Spec.RemediationRequestRef.Name,
                Namespace: ai.Spec.RemediationRequestRef.Namespace,
            },
        },
    }
}
```

**2. AIAnalysis Watches AIApprovalRequest (Downstream)**:

```go
// In AIAnalysisReconciler.SetupWithManager()
err = c.Watch(
    &source.Kind{Type: &approvalv1.AIApprovalRequest{}},
    handler.EnqueueRequestsFromMapFunc(r.approvalRequestToAnalysis),
)

// Mapping function
func (r *AIAnalysisReconciler) approvalRequestToAnalysis(obj client.Object) []ctrl.Request {
    approval := obj.(*approvalv1.AIApprovalRequest)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      approval.Spec.AIAnalysisRef.Name,
                Namespace: approval.Spec.AIAnalysisRef.Namespace,
            },
        },
    }
}
```

**Result**: Bi-directional event propagation with ~100ms latency:
- RemediationRequest detects AIAnalysis completion within ~100ms
- AIAnalysis detects approval within ~100ms

---

#### Approval Workflow Pattern

**Unique Pattern**: AIAnalysis is the only service with a **child approval workflow**

**Why AIApprovalRequest is Needed**:
- **Separation of Concerns**: Approval logic isolated from AI analysis logic
- **Policy-Based Approval**: Rego policy determines auto-approval eligibility
- **Manual Override**: Operators can approve high-risk actions manually
- **Audit Trail**: Approval decisions tracked in dedicated CRD

**Approval Decision Flow**:
```
AIAnalysis generates recommendations
    ↓
AIAnalysis creates AIApprovalRequest CRD
    ↓
AIApprovalRequest Controller evaluates Rego policy
    ↓ (policy decision)
If auto-approve: Create Approval CRD automatically
If manual:      Wait for operator to create Approval CRD
    ↓ (watch trigger)
AIApprovalRequest.status.approved = true
    ↓ (watch trigger in AIAnalysis)
AIAnalysis.status.phase = "completed"
```

**Why NOT Embed Approval in AIAnalysis**:
- ❌ **Mixing Concerns**: AI analysis logic mixed with approval logic
- ❌ **Testing Complexity**: Hard to test approval independently
- ❌ **Policy Changes**: Rego policy updates require AI controller changes
- ❌ **Audit Trail**: Approval history buried in AIAnalysis status

**Why AIApprovalRequest CRD is Better**:
- ✅ **Clean Separation**: AI analysis and approval are separate phases
- ✅ **Independent Testing**: Each controller tested independently
- ✅ **Policy Evolution**: Rego policy changes don't affect AI controller
- ✅ **Clear Audit**: `kubectl get aiapprovalrequest` shows all approvals

---

## Approval Context Population & Decision Tracking (V1.0)

**Business Requirements**: BR-AI-059 (Approval Context), BR-AI-060 (Approval Decision Tracking)
**ADR Reference**: ADR-018 (Approval Notification V1.0 Integration)

### Approval Context Population (BR-AI-059)

**Purpose**: Enable RemediationOrchestrator to create rich operator notifications by populating comprehensive approval context when AI recommendations require human review.

**Trigger**: HolmesGPT response with confidence 60-79% (medium confidence threshold)

**Action**: Populate `status.approvalContext` with:
1. **Investigation Summary**: Concise description of root cause analysis
2. **Evidence Collected**: List of supporting evidence from Context API and cluster state
3. **Recommended Actions**: Structured list of actions with rationales
4. **Alternatives Considered**: Other approaches considered with pros/cons
5. **Why Approval Required**: Clear explanation of why human review is needed

**Code Reference**: `populateApprovalContext()` function in controller implementation

**Validation Requirements**:
- ✅ MUST have `investigationSummary` (non-empty)
- ✅ MUST have at least 1 `recommendedAction`
- ✅ MUST have at least 1 `evidenceCollected` item
- ✅ SHOULD have at least 1 `alternativeConsidered` (for informed decisions)

**Example Approval Context**:
```yaml
status:
  approvalContext:
    reason: "Medium confidence (72.5%) - requires human review"
    confidenceScore: 72.5
    confidenceLevel: "medium"
    investigationSummary: "Memory leak detected in payment processing coroutine (50MB/hr growth rate)"
    evidenceCollected:
      - "Linear memory growth 50MB/hour per pod over 4-hour observation window"
      - "Similar incident resolved 3 weeks ago with 92% success rate (memory increase)"
      - "No code deployment in last 24h - rules out recent regression"
    recommendedActions:
      - action: "collect_diagnostics"
        rationale: "Capture heap dump before making changes for post-mortem analysis"
      - action: "increase_resources"
        rationale: "Increase memory limit 2Gi → 3Gi based on observed growth rate"
      - action: "restart_pod"
        rationale: "Rolling restart to clear leaked memory and restore service"
    alternativesConsidered:
      - approach: "Wait and monitor"
        prosCons: "Pros: No disruption. Cons: OOM risk in ~4 hours based on current trend"
      - approach: "Immediate restart without memory increase"
        prosCons: "Pros: Fast recovery. Cons: Doesn't address root cause, will recur"
    whyApprovalRequired: "Historical pattern requires validation (71-86% HolmesGPT accuracy on generic K8s memory issues)"
```

**Integration**: RemediationOrchestrator watches `AIAnalysis.status.phase = "Approving"` and uses `approvalContext` to format rich notifications for Slack/Console delivery.

---

### Approval Decision Tracking (BR-AI-060)

**Purpose**: Maintain complete audit trail of operator approval decisions for compliance, system learning, and effectiveness tracking.

**Trigger**: AIApprovalRequest status update (decision = "approved" or "rejected")

**Action**: Update `status` with approval decision metadata:
1. **Approval Status**: "approved", "rejected", or "pending"
2. **Decision Metadata**: Approver/rejector identity, timestamp, method (console/slack/api)
3. **Justification**: Operator-provided reason for their decision
4. **Timing**: Duration from approval request to decision

**Code Reference**: `updateApprovalDecisionStatus()` function in controller implementation

**Status Fields Populated**:
```yaml
status:
  # Decision metadata
  approvalStatus: "approved"  # or "rejected" or "pending"
  approvalTime: "2025-10-20T14:32:45Z"
  approvalDuration: "2m15s"
  approvalMethod: "console"
  approvalJustification: "Approved - low risk change in staging environment"

  # Approval path (if approved)
  approvedBy: "ops-engineer@company.com"

  # Rejection path (if rejected)
  rejectedBy: "ops-engineer@company.com"
  rejectionReason: "Resource constraints - cannot increase memory at this time"
```

**Audit Trail Benefits**:
- ✅ **Compliance**: Complete record of who approved/rejected what and why
- ✅ **System Learning**: AI learns from operator decisions to improve future confidence scoring
- ✅ **Effectiveness Tracking**: Track approval success rates for different recommendation types
- ✅ **Operator Patterns**: Identify frequently rejected action types for training improvement

**Watch Pattern**: AIAnalysis controller watches AIApprovalRequest status changes and updates its own status when decisions are made.

---

#### Coordination Benefits

**For AIAnalysis Controller**:
- ✅ **Focused**: Only handles AI investigation and recommendations
- ✅ **Decoupled**: Doesn't know about WorkflowExecution
- ✅ **Approval Separation**: Approval logic isolated in AIApprovalRequest
- ✅ **Testable**: Unit tests only need fake K8s client + approval CRD

**For RemediationRequest Controller**:
- ✅ **Visibility**: Can query AIAnalysis status anytime
- ✅ **Control**: Decides when to create WorkflowExecution
- ✅ **Timeout Detection**: Can detect if AIAnalysis takes too long
- ✅ **Approval Awareness**: Sees approval status in AIAnalysis

**For Operations**:
- ✅ **Debuggable**: `kubectl get aianalysis -o yaml` shows full investigation state
- ✅ **Approval Transparency**: `kubectl get aiapprovalrequest` shows approval decisions
- ✅ **Observable**: Kubernetes events show investigation and approval progress
- ✅ **Traceable**: CRD history shows complete workflow with approvals

---


## Phase-Specific Timeouts & Fallback Mechanisms

### Phase Timeout Configuration (BR-AI-032)

**Default Timeouts**:
- **investigating**: 15 minutes (HolmesGPT investigation)
- **analyzing**: 10 minutes (AI analysis and validation)
- **recommending**: 5 minutes (Recommendation generation)
- **completed**: No timeout (terminal state)

**Configurable via Annotation**:
```yaml
apiVersion: aianalysis.kubernaut.io/v1
kind: AIAnalysis
metadata:
  annotations:
    aianalysis.kubernaut.io/investigating-timeout: "20m"
    aianalysis.kubernaut.io/analyzing-timeout: "15m"
    aianalysis.kubernaut.io/recommending-timeout: "10m"
spec:
  # ... analysis request
```

**Implementation**:
```go
// pkg/ai/analysis/phases/timeout.go
package phases

import (
    "time"
    "strconv"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
)

const (
    DefaultInvestigatingTimeout = 15 * time.Minute
    DefaultAnalyzingTimeout     = 10 * time.Minute
    DefaultRecommendingTimeout  = 5 * time.Minute
)

func GetPhaseTimeout(aiAnalysis *aianalysisv1.AIAnalysis) time.Duration {
    phase := aiAnalysis.Status.Phase

    // Check for annotation override
    annotationKey := fmt.Sprintf("aianalysis.kubernaut.io/%s-timeout", phase)
    if timeoutStr, ok := aiAnalysis.Annotations[annotationKey]; ok {
        if timeout, err := time.ParseDuration(timeoutStr); err == nil {
            return timeout
        }
    }

    // Return default based on phase
    switch phase {
    case "investigating":
        return DefaultInvestigatingTimeout
    case "analyzing":
        return DefaultAnalyzingTimeout
    case "recommending":
        return DefaultRecommendingTimeout
    default:
        return 15 * time.Minute
    }
}

func (r *AIAnalysisReconciler) checkPhaseTimeout(aiAnalysis *aianalysisv1.AIAnalysis) (bool, string) {
    timeout := GetPhaseTimeout(aiAnalysis)

    phaseStartTime := aiAnalysis.Status.PhaseTransitions[aiAnalysis.Status.Phase]
    if time.Since(phaseStartTime.Time) > timeout {
        return true, fmt.Sprintf("phase %s exceeded timeout of %v", aiAnalysis.Status.Phase, timeout)
    }

    return false, ""
}
```

### HolmesGPT Fallback Strategy (BR-AI-024)

**Requirement**: AIAnalysis MUST provide fallback when HolmesGPT is unavailable

#### Fallback Decision Tree

```
HolmesGPT Investigation Request
        ↓
    Available?
    /        \
  YES        NO
   ↓          ↓
 Use     Check Historical
 HolmesGPT    Patterns
              ↓
          Found Similar?
          /        \
        YES        NO
         ↓          ↓
    Use Historical  Degraded
      Analysis      Mode
```

#### BR-AI-024: HolmesGPT Unavailability Fallback

**Implementation**: Multi-tier fallback strategy

**Tier 1: Historical Pattern Matching** (Primary Fallback)
- Query vector DB for similar alert fingerprints
- Retrieve past investigation results with >0.8 similarity
- Use cached HolmesGPT responses from similar scenarios

**Tier 2: Rule-Based Analysis** (Secondary Fallback)
- Apply predefined diagnostic rules for common patterns
- Use heuristic-based root cause identification
- Generate basic recommendations from knowledge base

**Tier 3: Degraded Mode** (Final Fallback)
- Mark analysis as "degraded"
- Escalate to manual review
- Provide basic context without AI analysis

#### Implementation: HolmesGPT Fallback

```go
// pkg/ai/analysis/integration/holmesgpt_fallback.go
package integration

import (
    "context"
    "fmt"
    "github.com/jordigilh/kubernaut/pkg/ai/analysis"
    "github.com/jordigilh/kubernaut/pkg/storage"
)

type HolmesGPTClientWithFallback struct {
    holmesClient  *HolmesGPTClient
    vectorDB      storage.VectorDBClient
    knowledgeBase *KnowledgeBase
}

func (h *HolmesGPTClientWithFallback) Investigate(
    ctx context.Context,
    req analysis.InvestigationRequest,
) (*analysis.InvestigationResult, error) {

    // Tier 1: Try HolmesGPT (primary)
    result, err := h.holmesClient.Investigate(ctx, req)
    if err == nil {
        return result, nil
    }

    log.Warn("HolmesGPT unavailable, using fallback", "error", err)

    // Tier 2: Historical Pattern Matching (fallback)
    historicalResult, err := h.useHistoricalPatterns(ctx, req)
    if err == nil && historicalResult.Confidence > 0.8 {
        log.Info("Using historical pattern fallback", "confidence", historicalResult.Confidence)
        return historicalResult, nil
    }

    // Tier 3: Rule-Based Analysis (fallback)
    ruleBasedResult, err := h.useRuleBasedAnalysis(ctx, req)
    if err == nil {
        log.Info("Using rule-based fallback")
        return ruleBasedResult, nil
    }

    // Tier 4: Degraded Mode (final fallback)
    return h.degradedModeAnalysis(ctx, req), nil
}

func (h *HolmesGPTClientWithFallback) useHistoricalPatterns(
    ctx context.Context,
    req analysis.InvestigationRequest,
) (*analysis.InvestigationResult, error) {

    // Vector similarity search for similar alerts
    similarAlerts, err := h.vectorDB.Search(ctx, storage.VectorSearchRequest{
        Query:     req.AlertContext.Fingerprint,
        TopK:      5,
        Threshold: 0.8,
    })

    if err != nil || len(similarAlerts) == 0 {
        return nil, fmt.Errorf("no historical patterns found")
    }

    // Use most similar past investigation
    bestMatch := similarAlerts[0]

    return &analysis.InvestigationResult{
        RootCauseHypotheses: bestMatch.PastInvestigation.RootCauseHypotheses,
        CorrelatedAlerts:    bestMatch.PastInvestigation.CorrelatedAlerts,
        InvestigationReport: fmt.Sprintf(
            "[HISTORICAL FALLBACK] Based on similar alert (similarity: %.2f)\n%s",
            bestMatch.Similarity,
            bestMatch.PastInvestigation.Report,
        ),
        ContextualAnalysis: bestMatch.PastInvestigation.ContextualAnalysis,
        Confidence:         bestMatch.Similarity,
        FallbackUsed:       "historical-patterns",
    }, nil
}

func (h *HolmesGPTClientWithFallback) useRuleBasedAnalysis(
    ctx context.Context,
    req analysis.InvestigationRequest,
) (*analysis.InvestigationResult, error) {

    // Apply predefined diagnostic rules
    rules := h.knowledgeBase.GetRulesForAlert(req.AlertContext)

    hypotheses := []analysis.RootCauseHypothesis{}
    for _, rule := range rules {
        if rule.Matches(req.AlertContext) {
            hypotheses = append(hypotheses, analysis.RootCauseHypothesis{
                Hypothesis: rule.RootCause,
                Confidence: rule.Confidence,
                Evidence:   rule.Evidence,
            })
        }
    }

    if len(hypotheses) == 0 {
        return nil, fmt.Errorf("no matching rules found")
    }

    return &analysis.InvestigationResult{
        RootCauseHypotheses: hypotheses,
        InvestigationReport: "[RULE-BASED FALLBACK] Analysis based on predefined diagnostic rules",
        FallbackUsed:        "rule-based",
    }, nil
}

func (h *HolmesGPTClientWithFallback) degradedModeAnalysis(
    ctx context.Context,
    req analysis.InvestigationRequest,
) *analysis.InvestigationResult {

    return &analysis.InvestigationResult{
        RootCauseHypotheses: []analysis.RootCauseHypothesis{
            {
                Hypothesis: "Manual investigation required (AI unavailable)",
                Confidence: 0.0,
                Evidence:   []string{"HolmesGPT unavailable", "No historical patterns found"},
            },
        },
        InvestigationReport: "[DEGRADED MODE] AI analysis unavailable, manual review required",
        ContextualAnalysis:  fmt.Sprintf("Alert: %s, Severity: %s, Environment: %s",
            req.AlertContext.Fingerprint,
            req.AlertContext.Severity,
            req.AlertContext.Environment,
        ),
        FallbackUsed: "degraded-mode",
        Confidence:   0.0,
    }
}
```

#### Metrics for Fallback Usage

```go
var (
    aiHolmesGPTFallbackCount = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_ai_holmesgpt_fallback_total",
        Help: "Total HolmesGPT fallback operations",
    }, []string{"fallback_tier", "environment"})

    aiHolmesGPTAvailability = promauto.NewGaugeVec(prometheus.GaugeOpts{
        Name: "kubernaut_ai_holmesgpt_availability",
        Help: "HolmesGPT service availability (1 = available, 0 = unavailable)",
    }, []string{"endpoint"})
)

// Record fallback usage
aiHolmesGPTFallbackCount.WithLabelValues("historical-patterns", "production").Inc()
aiHolmesGPTAvailability.WithLabelValues("holmesgpt-api:8080").Set(0)
```

#### Status Field for Fallback Tracking

```yaml
status:
  phase: investigating
  investigationResult:
    rootCauseHypotheses: [...]
    investigationReport: "[HISTORICAL FALLBACK] Based on similar alert..."

    # Fallback metadata
    fallbackMetadata:
      fallbackUsed: "historical-patterns"  # or "rule-based", "degraded-mode", "none"
      holmesGPTAvailable: false
      fallbackConfidence: 0.85
      fallbackReason: "HolmesGPT service unavailable (connection timeout)"
```


---

