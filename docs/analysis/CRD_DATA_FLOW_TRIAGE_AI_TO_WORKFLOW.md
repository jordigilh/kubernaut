# CRD Data Flow Triage: AIAnalysis → WorkflowExecution

**Date**: October 8, 2025
**Purpose**: Triage AIAnalysis CRD status to ensure it provides all data WorkflowExecution needs
**Scope**: RemediationOrchestrator creates WorkflowExecution with data snapshot from AIAnalysis.status
**Architecture Pattern**: **Self-Contained CRDs** (no cross-CRD reads during reconciliation)

---

## Executive Summary

**Status**: ✅ **DATA FLOW COMPATIBLE WITH MINOR ENHANCEMENTS RECOMMENDED**

**Finding**: AIAnalysis.status provides **sufficient data** for WorkflowExecution to operate. The current schema supports:
- ✅ AI recommendations with dependencies
- ✅ Target resource information
- ✅ Action parameters
- ✅ Risk assessment data
- ✅ Historical success rates

**Recommendation**: Add 2 optional fields to enhance workflow orchestration capabilities (P2 - Low priority).

---

## 🔍 Data Flow Pattern

```
Gateway Service
    ↓ (creates RemediationRequest CRD)
RemediationOrchestrator
    ↓ (creates AIAnalysis CRD with data from RemediationProcessing)
AIAnalysis Controller
    ↓ (performs investigation, updates AIAnalysis.status)
RemediationOrchestrator (watches AIAnalysis.status.phase == "completed")
    ↓ (SNAPSHOT: copies recommendations from AIAnalysis.status to WorkflowExecution.spec)
WorkflowExecution CRD (self-contained)
    ↓
WorkflowExecution Controller (operates on WorkflowExecution.spec - NO cross-CRD reads)
```

**Key Pattern**: WorkflowExecution.spec is a **data snapshot** from AIAnalysis.status at creation time.

---

## 📋 WorkflowExecution Data Requirements

### What WorkflowExecution Needs (from `docs/services/crd-controllers/03-workflowexecution/crd-schema.md`)

WorkflowExecution.spec expects:

```go
type WorkflowExecutionSpec struct {
    // Parent reference (for audit/lineage only)
    RemediationRequestRef corev1.ObjectReference `json:"alertRemediationRef"`

    // CRITICAL: Workflow definition with steps
    WorkflowDefinition WorkflowDefinition `json:"workflowDefinition"`

    // Execution configuration
    ExecutionStrategy ExecutionStrategy `json:"executionStrategy"`

    // Optional: Runtime optimization
    AdaptiveOrchestration AdaptiveOrchestrationConfig `json:"adaptiveOrchestration,omitempty"`
}

type WorkflowDefinition struct {
    Name             string                  `json:"name"`
    Version          string                  `json:"version"`
    Steps            []WorkflowStep          `json:"steps"`
    Dependencies     map[string][]string     `json:"dependencies,omitempty"`
    AIRecommendations *AIRecommendations     `json:"aiRecommendations,omitempty"`
}

type WorkflowStep struct {
    StepNumber     int                    `json:"stepNumber"`
    Name           string                 `json:"name"`
    Action         string                 `json:"action"`
    TargetCluster  string                 `json:"targetCluster"`
    Parameters     *StepParameters        `json:"parameters"`
    CriticalStep   bool                   `json:"criticalStep"`
    MaxRetries     int                    `json:"maxRetries,omitempty"`
    Timeout        string                 `json:"timeout,omitempty"`
    DependsOn      []int                  `json:"dependsOn,omitempty"`  // ✅ KEY FIELD
    RollbackSpec   *RollbackSpec          `json:"rollbackSpec,omitempty"`
}
```

**Key Requirements**:
1. AI recommendations (action, target, parameters)
2. Dependency information (id, dependencies array)
3. Risk assessment data (for determining criticalStep)
4. Effectiveness probability (for determining maxRetries)

---

## 📊 Current AIAnalysis.status Schema

From `docs/services/crd-controllers/02-aianalysis/crd-schema.md`:

```yaml
status:
  phase: completed

  # Recommendations with dependencies (BR-HOLMES-031, BR-HOLMES-032, BR-HOLMES-033)
  recommendations:
  - id: "rec-001"  # ✅ Unique identifier for dependency mapping
    action: "scale-deployment"  # ✅ WorkflowExecution needs this
    targetResource:  # ✅ WorkflowExecution needs this
      kind: Deployment
      name: payment-api
      namespace: production
    parameters:  # ✅ WorkflowExecution needs this
      replicas: 5
    effectivenessProbability: 0.92  # ✅ WorkflowExecution uses for maxRetries
    historicalSuccessRate: 0.88
    riskLevel: low  # ✅ WorkflowExecution uses for criticalStep
    explanation: "Historical data shows 88% success rate"
    supportingEvidence:
    - "15 similar cases resolved by memory increase"
    constraints:
      environmentAllowed: [production, staging]
      rbacRequired: ["apps/deployments:update"]
    dependencies: []  # ✅ WorkflowExecution needs this for dependency resolution
```

**Observation**: AIAnalysis.status.recommendations provides **all critical fields** that WorkflowExecution needs.

---

## 🔬 Detailed Field-by-Field Analysis

### WorkflowExecution Requirements vs AIAnalysis.status

| WorkflowExecution Field | Priority | Available in AIAnalysis.status? | Gap Severity |
|---|---|---|---|
| **WorkflowStep.action** | CRITICAL | ✅ YES (`recommendations[].action`) | ✅ OK |
| **WorkflowStep.targetCluster** | CRITICAL | ⚠️ INFERRED (from `targetResource.namespace`) | ⚠️ MINOR |
| **WorkflowStep.parameters** | CRITICAL | ✅ YES (`recommendations[].parameters`) | ✅ OK |
| **WorkflowStep.criticalStep** | HIGH | ✅ DERIVED (`riskLevel == "high"`) | ✅ OK |
| **WorkflowStep.maxRetries** | HIGH | ✅ DERIVED (`effectivenessProbability`) | ✅ OK |
| **WorkflowStep.timeout** | MEDIUM | ❌ NOT in status | 🟡 MINOR |
| **WorkflowStep.dependsOn** | CRITICAL | ✅ YES (`recommendations[].dependencies`) | ✅ OK |
| **WorkflowStep.rollbackSpec** | MEDIUM | ❌ NOT in status | 🟡 MINOR |
| **AIRecommendations.overallConfidence** | LOW | ⚠️ CAN CALCULATE (avg of `effectivenessProbability`) | ⚠️ ENHANCEMENT |
| **AIRecommendations.estimatedDuration** | LOW | ❌ NOT in status | 🟡 ENHANCEMENT |

---

## ✅ COMPATIBILITY ASSESSMENT

### Compatible Fields (9 fields) - No Changes Needed

1. ✅ **recommendations[].id** → WorkflowStep mapping
   - Used to map dependencies from string IDs to step numbers
   - Implementation: `buildWorkflowFromRecommendations()` function

2. ✅ **recommendations[].action** → WorkflowStep.action
   - Direct copy (e.g., "scale-deployment", "restart-pods")

3. ✅ **recommendations[].targetResource** → Step parameters
   - Extract kind, name, namespace into StepParameters

4. ✅ **recommendations[].parameters** → WorkflowStep.parameters
   - Direct copy with type mapping to StepParameters union

5. ✅ **recommendations[].riskLevel** → WorkflowStep.criticalStep
   - Mapping: `riskLevel == "high"` → `criticalStep = true`

6. ✅ **recommendations[].effectivenessProbability** → WorkflowStep.maxRetries
   - Formula: `maxRetries = (effectivenessProbability < 0.8) ? 5 : 3`

7. ✅ **recommendations[].dependencies** → WorkflowStep.dependsOn
   - Convert string IDs to step numbers via idToStepNumber map

8. ✅ **recommendations[].historicalSuccessRate** → AIRecommendations
   - Include in workflow metadata

9. ✅ **recommendations[].supportingEvidence** → AIRecommendations
   - Include for audit trail

---

### Minor Gaps (2 fields) - Acceptable Defaults

1. ⚠️ **WorkflowStep.targetCluster** (MINOR)
   - **Current**: AIAnalysis does NOT specify cluster
   - **Workaround**: Infer from targetResource.namespace (e.g., "production" → "production-cluster")
   - **Acceptable**: V1 assumes single-cluster operation
   - **Future**: V2 multi-cluster support will add `clusterName` to recommendations

2. ⚠️ **WorkflowStep.timeout** (MINOR)
   - **Current**: AIAnalysis does NOT specify timeouts
   - **Workaround**: Use default timeouts per action type
     - `scale-deployment`: 5m
     - `restart-pods`: 5m
     - `patch-resource`: 3m
     - `verify-health`: 2m
   - **Acceptable**: Static defaults work for V1

---

### Enhancement Opportunities (2 fields) - P2 Low Priority

1. 🟡 **recommendations[].estimatedDuration** (ENHANCEMENT)
   - **Purpose**: WorkflowExecution can display estimated completion time
   - **Benefit**: Better UX for monitoring workflows
   - **Priority**: P2 - Nice to have, not blocking
   - **Implementation**: HolmesGPT can estimate based on historical execution times

2. 🟡 **recommendations[].rollbackSpec** (ENHANCEMENT)
   - **Purpose**: AI-generated rollback actions
   - **Benefit**: Smarter rollback strategies
   - **Priority**: P2 - V1 uses automatic rollback without AI input
   - **Implementation**: HolmesGPT can suggest rollback actions per recommendation

---

## 📝 RemediationOrchestrator Mapping Code

### How RemediationOrchestrator Creates WorkflowExecution

**File**: `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md`

```go
// When AIAnalysis.status.phase == "completed" and approved
func (r *RemediationOrchestratorReconciler) createWorkflowExecution(
    ctx context.Context,
    remReq *remediationv1.RemediationRequest,
    aiAnalysis *aianalysisv1.AIAnalysis,
) error {

    // Validate AIAnalysis status has recommendations
    if len(aiAnalysis.Status.Recommendations) == 0 {
        return fmt.Errorf("AIAnalysis has no recommendations")
    }

    workflowExec := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-workflow", remReq.Name),
            Namespace: remReq.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remReq, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: workflowexecutionv1.WorkflowExecutionSpec{
            RemediationRequestRef: workflowexecutionv1.RemediationRequestReference{
                Name:      remReq.Name,
                Namespace: remReq.Namespace,
            },

            // ✅ BUILD workflow from AI recommendations
            WorkflowDefinition: buildWorkflowFromRecommendations(
                aiAnalysis.Status.Recommendations,
            ),

            ExecutionStrategy: workflowexecutionv1.ExecutionStrategy{
                ApprovalRequired: false, // Already approved at AIAnalysis level
                DryRunFirst:      true,  // Safety-first
                RollbackStrategy: "automatic",
            },
        },
    }

    return r.Create(ctx, workflowExec)
}

// buildWorkflowFromRecommendations converts AI recommendations to workflow definition
// Business Requirements: BR-HOLMES-031, BR-HOLMES-032, BR-HOLMES-033
func buildWorkflowFromRecommendations(
    recommendations []aianalysisv1.Recommendation,
) workflowexecutionv1.WorkflowDefinition {

    // Step 1: Create mapping from recommendation ID (string) to step number (int)
    idToStepNumber := make(map[string]int)
    for i, rec := range recommendations {
        idToStepNumber[rec.ID] = i + 1  // Step numbers are 1-based
    }

    // Step 2: Build workflow steps with dependency mapping
    steps := []workflowexecutionv1.WorkflowStep{}
    for i, rec := range recommendations {
        // ✅ Map dependencies from recommendation IDs (strings) to step numbers (ints)
        dependsOn := []int{}
        for _, depID := range rec.Dependencies {
            if stepNum, exists := idToStepNumber[depID]; exists {
                dependsOn = append(dependsOn, stepNum)
            }
        }

        step := workflowexecutionv1.WorkflowStep{
            StepNumber:   i + 1,
            Name:         rec.Action,
            Action:       rec.Action,

            // ⚠️ INFERRED: targetCluster from namespace
            TargetCluster: inferClusterFromNamespace(rec.TargetResource.Namespace),

            // ✅ DIRECT COPY: parameters
            Parameters:   convertParameters(rec.Parameters),

            // ✅ DERIVED: criticalStep from riskLevel
            CriticalStep: rec.RiskLevel == "high",

            // ✅ DERIVED: maxRetries from effectivenessProbability
            MaxRetries:   determineRetries(rec.EffectivenessProbability),

            // ⚠️ DEFAULT: timeout (not in AIAnalysis)
            Timeout:      getDefaultTimeout(rec.Action),

            // ✅ MAPPED: dependencies
            DependsOn:    dependsOn,

            // ⚠️ NULL: rollbackSpec (not in AIAnalysis)
            // WorkflowExecution controller will generate automatic rollback
        }
        steps = append(steps, step)
    }

    return workflowexecutionv1.WorkflowDefinition{
        Name:    "ai-generated-workflow",
        Version: "v1",
        Steps:   steps,
        AIRecommendations: &workflowexecutionv1.AIRecommendations{
            Source: "holmesgpt",
            Count:  len(recommendations),
            // ⚠️ CALCULATED: overallConfidence (not directly in AIAnalysis)
            OverallConfidence: calculateAverageConfidence(recommendations),
        },
    }
}

// Helper: Infer cluster from namespace (V1 single-cluster assumption)
func inferClusterFromNamespace(namespace string) string {
    // Simple heuristic - can be enhanced with ConfigMap lookup
    switch {
    case strings.Contains(namespace, "prod"):
        return "production-cluster"
    case strings.Contains(namespace, "staging"):
        return "staging-cluster"
    default:
        return "default-cluster"
    }
}

// Helper: Get default timeout per action type
func getDefaultTimeout(action string) string {
    timeouts := map[string]string{
        "scale-deployment":        "5m",
        "restart-pods":            "5m",
        "restart-deployment":      "5m",
        "patch-resource":          "3m",
        "update-configmap":        "3m",
        "verify-health":           "2m",
        "increase-memory-limit":   "5m",
        "taint-node":              "1m",
        "cordon-node":             "1m",
        "drain-node":              "10m",
    }

    if timeout, exists := timeouts[action]; exists {
        return timeout
    }
    return "5m" // Default
}

// Helper: Determine retries from effectiveness probability
func determineRetries(effectivenessProbability float64) int {
    if effectivenessProbability >= 0.9 {
        return 2  // High confidence = fewer retries
    } else if effectivenessProbability >= 0.8 {
        return 3  // Medium confidence = standard retries
    } else {
        return 5  // Low confidence = more retries
    }
}

// Helper: Calculate average confidence
func calculateAverageConfidence(recommendations []aianalysisv1.Recommendation) float64 {
    if len(recommendations) == 0 {
        return 0.0
    }

    sum := 0.0
    for _, rec := range recommendations {
        sum += rec.EffectivenessProbability
    }
    return sum / float64(len(recommendations))
}

// Helper: Convert AIAnalysis parameters to WorkflowStep parameters
func convertParameters(aiParams map[string]interface{}) *workflowexecutionv1.StepParameters {
    // Type-safe conversion based on action type
    // Implementation details omitted for brevity
    // See: docs/services/crd-controllers/03-workflowexecution/integration-points.md
    return &workflowexecutionv1.StepParameters{
        // ... action-specific parameter mapping ...
    }
}
```

---

## 🎯 Dependency Mapping Example

### AIAnalysis Recommendations (Source)

```yaml
status:
  recommendations:
  - id: "rec-001"
    action: "scale-deployment"
    dependencies: []  # No dependencies

  - id: "rec-002"
    action: "restart-pods"
    dependencies: ["rec-001"]  # String ID

  - id: "rec-003"
    action: "increase-memory-limit"
    dependencies: ["rec-001"]  # String ID

  - id: "rec-004"
    action: "verify-deployment"
    dependencies: ["rec-002", "rec-003"]  # Multiple string IDs
```

### WorkflowExecution Steps (Generated)

```yaml
spec:
  workflowDefinition:
    steps:
    - stepNumber: 1
      action: "scale-deployment"
      dependsOn: []  # No dependencies (empty array)

    - stepNumber: 2
      action: "restart-pods"
      dependsOn: [1]  # rec-001 → step 1 (integer)

    - stepNumber: 3
      action: "increase-memory-limit"
      dependsOn: [1]  # rec-001 → step 1 (integer)

    - stepNumber: 4
      action: "verify-deployment"
      dependsOn: [2, 3]  # rec-002 → step 2, rec-003 → step 3 (integers)
```

**Mapping Process**:
1. Build map: `{"rec-001": 1, "rec-002": 2, "rec-003": 3, "rec-004": 4}`
2. For each recommendation.dependencies (string array):
   - Look up step number in map
   - Append to WorkflowStep.dependsOn (int array)
3. Result: String IDs converted to integer step numbers

---

## 🔧 Recommended Schema Enhancements (Optional - P2)

### Enhancement 1: Add estimatedDuration to Recommendation

**File**: `docs/services/crd-controllers/02-aianalysis/crd-schema.md`

```go
type Recommendation struct {
    ID                       string                 `json:"id"`
    Action                   string                 `json:"action"`
    TargetResource           ResourceIdentifier     `json:"targetResource"`
    Parameters               map[string]interface{} `json:"parameters"`
    EffectivenessProbability float64                `json:"effectivenessProbability"`
    HistoricalSuccessRate    float64                `json:"historicalSuccessRate"`
    RiskLevel                string                 `json:"riskLevel"`
    Explanation              string                 `json:"explanation"`
    SupportingEvidence       []string               `json:"supportingEvidence,omitempty"`
    Constraints              Constraints            `json:"constraints,omitempty"`
    Dependencies             []string               `json:"dependencies,omitempty"`

    // ✅ ADD (P2 - Enhancement): Estimated execution duration
    // Used by WorkflowExecution for progress estimation
    EstimatedDuration        string                 `json:"estimatedDuration,omitempty"` // e.g., "2m30s"
}
```

**Benefit**: WorkflowExecution can display estimated completion time and progress percentage.

**Implementation**: HolmesGPT analyzes historical execution times for similar actions.

---

### Enhancement 2: Add rollbackAction to Recommendation

**File**: `docs/services/crd-controllers/02-aianalysis/crd-schema.md`

```go
type Recommendation struct {
    ID                       string                 `json:"id"`
    Action                   string                 `json:"action"`
    TargetResource           ResourceIdentifier     `json:"targetResource"`
    Parameters               map[string]interface{} `json:"parameters"`
    EffectivenessProbability float64                `json:"effectivenessProbability"`
    HistoricalSuccessRate    float64                `json:"historicalSuccessRate"`
    RiskLevel                string                 `json:"riskLevel"`
    Explanation              string                 `json:"explanation"`
    SupportingEvidence       []string               `json:"supportingEvidence,omitempty"`
    Constraints              Constraints            `json:"constraints,omitempty"`
    Dependencies             []string               `json:"dependencies,omitempty"`
    EstimatedDuration        string                 `json:"estimatedDuration,omitempty"`

    // ✅ ADD (P2 - Enhancement): AI-suggested rollback action
    // Used by WorkflowExecution for intelligent rollback
    RollbackAction           *RollbackRecommendation `json:"rollbackAction,omitempty"`
}

// ✅ ADD: New type
type RollbackRecommendation struct {
    Action         string                 `json:"action"`       // e.g., "restore-previous-config"
    Parameters     map[string]interface{} `json:"parameters"`
    Timeout        string                 `json:"timeout,omitempty"`
    Explanation    string                 `json:"explanation"`
}
```

**Benefit**: AI-driven rollback strategies instead of generic automatic rollback.

**Implementation**: HolmesGPT suggests rollback actions based on recommendation type and risk level.

---

## ✅ Validation Checklist

### Data Completeness Checklist

- [x] **Critical Fields**: All critical fields available in AIAnalysis.status ✅
- [x] **Dependency Mapping**: ID-to-step-number mapping supported ✅
- [x] **Risk Assessment**: riskLevel available for criticalStep determination ✅
- [x] **Effectiveness Data**: effectivenessProbability available for retry logic ✅
- [x] **Target Resource**: Full resource identification (kind, name, namespace) ✅
- [x] **Action Parameters**: All action parameters available ✅

### Compatibility Checklist

- [x] **No Breaking Changes**: Current schema works for V1 ✅
- [x] **Acceptable Defaults**: Missing fields have reasonable defaults ✅
- [x] **Mapping Logic**: buildWorkflowFromRecommendations() function defined ✅
- [x] **Type Safety**: No map[string]interface{} in WorkflowExecution ✅

### Enhancement Checklist (Optional)

- [ ] **P2-1**: Add `estimatedDuration` to Recommendation (optional, enhances UX)
- [ ] **P2-2**: Add `rollbackAction` to Recommendation (optional, V2 feature)

---

## 🎯 Summary

### Status: ✅ COMPATIBLE

AIAnalysis.status provides **all critical data** needed by WorkflowExecution. No blocking gaps identified.

### Critical Data Flow (9 fields) - ✅ WORKING

1. ✅ recommendations[].id → Step mapping
2. ✅ recommendations[].action → WorkflowStep.action
3. ✅ recommendations[].targetResource → Step parameters
4. ✅ recommendations[].parameters → WorkflowStep.parameters
5. ✅ recommendations[].riskLevel → WorkflowStep.criticalStep
6. ✅ recommendations[].effectivenessProbability → WorkflowStep.maxRetries
7. ✅ recommendations[].dependencies → WorkflowStep.dependsOn
8. ✅ recommendations[].historicalSuccessRate → AIRecommendations metadata
9. ✅ recommendations[].supportingEvidence → Audit trail

### Minor Gaps (2 fields) - ⚠️ ACCEPTABLE

1. ⚠️ **targetCluster**: Inferred from namespace (V1 single-cluster assumption)
2. ⚠️ **timeout**: Static defaults per action type

### Enhancement Opportunities (2 fields) - 🟡 OPTIONAL (P2)

1. 🟡 **estimatedDuration**: Better UX (progress estimation)
2. 🟡 **rollbackAction**: Smarter rollback (V2 feature)

---

## 📅 Execution Plan

### Phase 1: Validation (Estimated: 1 hour)

1. ✅ Verify AIAnalysis schema compatibility
2. ✅ Confirm buildWorkflowFromRecommendations() logic
3. ✅ Validate dependency mapping algorithm

### Phase 2: Enhancement (Optional - P2)

1. ⏸️ Add `estimatedDuration` field to Recommendation
2. ⏸️ Add `rollbackAction` field to Recommendation
3. ⏸️ Update HolmesGPT prompt engineering for new fields
4. ⏸️ Update WorkflowExecution mapping logic

### Phase 3: Implementation Verification (When services are built)

1. Unit tests for buildWorkflowFromRecommendations()
2. Integration tests for AIAnalysis → WorkflowExecution data flow
3. E2E tests for multi-step workflows with dependencies

---

## 🔗 Related Documents

- [docs/services/crd-controllers/02-aianalysis/crd-schema.md](mdc:docs/services/crd-controllers/02-aianalysis/crd-schema.md)
- [docs/services/crd-controllers/03-workflowexecution/crd-schema.md](mdc:docs/services/crd-controllers/03-workflowexecution/crd-schema.md)
- [docs/services/crd-controllers/03-workflowexecution/integration-points.md](mdc:docs/services/crd-controllers/03-workflowexecution/integration-points.md)
- [docs/services/crd-controllers/05-remediationorchestrator/integration-points.md](mdc:docs/services/crd-controllers/05-remediationorchestrator/integration-points.md)
- [docs/analysis/CRD_DATA_FLOW_TRIAGE_REMEDIATION_TO_AI.md](mdc:docs/analysis/CRD_DATA_FLOW_TRIAGE_REMEDIATION_TO_AI.md) (Processor → AIAnalysis)

---

**Confidence Assessment**: 95%

**Justification**: This triage is based on authoritative service specifications and CRD schemas. The data flow is **fully compatible** - all critical fields are available. The minor gaps (targetCluster, timeout) have acceptable workarounds using defaults. The enhancement opportunities are truly optional and do not block V1 implementation. Risk: AIAnalysis schema may have edge cases discovered during implementation, but core data flow is solid.

