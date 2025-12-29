# DD-SP-002: SignalProcessing Kubernetes Conditions Specification

**Status**: ✅ **APPROVED** (2025-12-16)
**Priority**: 🚨 **V1.0 MANDATORY** (Per DD-CRD-002)
**Last Reviewed**: 2025-12-16
**Confidence**: 95%
**Owner**: SignalProcessing Team
**Implements**: [DD-CRD-002](./DD-CRD-002-kubernetes-conditions-standard.md)

---

## 📋 Context & Problem

### Problem Statement

SignalProcessing CRD has a `Conditions []metav1.Condition` field in the schema (line 181 of `signalprocessing_types.go`), but:
- No `pkg/signalprocessing/conditions.go` infrastructure exists
- Controller does not set conditions during phase transitions
- Operators cannot use `kubectl describe` for detailed status
- Automation cannot use `kubectl wait --for=condition=X`

### Business Requirements Mapping

| BR ID | Description | Condition Mapping |
|-------|-------------|-------------------|
| **BR-SP-001** | K8s Context Enrichment | `EnrichmentComplete` |
| **BR-SP-051-053** | Environment Classification | `ClassificationComplete` |
| **BR-SP-070-072** | Priority Assignment | `ClassificationComplete` |
| **BR-SP-090** | Categorization Audit Trail | `ProcessingComplete` |

---

## 🎯 Decision

### Condition Types

SignalProcessing will implement **4 Kubernetes Conditions** aligned with its 4-phase processing flow:

```
Pending → Enriching → Classifying → Categorizing → Completed
         └── EnrichmentComplete
                      └── ClassificationComplete
                                    └── CategorizationComplete
                                                   └── ProcessingComplete
```

---

## 📐 Condition Specifications

### Condition 1: `EnrichmentComplete`

**Purpose**: Indicates K8s context enrichment phase completed
**Phase Alignment**: `Enriching` → `Classifying` transition
**BR Reference**: BR-SP-001 (K8s Context Enrichment)

| Field | Success | Failure |
|-------|---------|---------|
| **Type** | `EnrichmentComplete` | `EnrichmentComplete` |
| **Status** | `True` | `False` |
| **Reason** | `EnrichmentSucceeded` | See failure reasons |
| **Message** | Context details | Error description |

**Failure Reasons**:
| Reason | Description | Recovery |
|--------|-------------|----------|
| `EnrichmentFailed` | Generic enrichment failure | Check K8s API connectivity |
| `K8sAPITimeout` | K8s API call timed out | Retry or check cluster health |
| `ResourceNotFound` | Target resource doesn't exist | Verify target exists |
| `RBACDenied` | Insufficient RBAC permissions | Check controller RBAC |
| `DegradedMode` | Enrichment ran in degraded mode | Non-fatal, processing continues |

**Example (Success)**:
```yaml
conditions:
  - type: EnrichmentComplete
    status: "True"
    reason: EnrichmentSucceeded
    message: "K8s context enriched: Pod payments-api-abc123, Deployment payments-api, Node node-1"
    lastTransitionTime: "2025-12-16T10:30:00Z"
```

**Example (Failure)**:
```yaml
conditions:
  - type: EnrichmentComplete
    status: "False"
    reason: K8sAPITimeout
    message: "Failed to fetch Pod details: timeout after 30s"
    lastTransitionTime: "2025-12-16T10:30:30Z"
```

**Example (Degraded)**:
```yaml
conditions:
  - type: EnrichmentComplete
    status: "True"
    reason: DegradedMode
    message: "Enrichment completed in degraded mode (K8s API unavailable), using signal labels"
    lastTransitionTime: "2025-12-16T10:30:00Z"
```

---

### Condition 2: `ClassificationComplete`

**Purpose**: Indicates environment and priority classification completed
**Phase Alignment**: `Classifying` → `Categorizing` transition
**BR Reference**: BR-SP-051-053 (Environment), BR-SP-070-072 (Priority)

| Field | Success | Failure |
|-------|---------|---------|
| **Type** | `ClassificationComplete` | `ClassificationComplete` |
| **Status** | `True` | `False` |
| **Reason** | `ClassificationSucceeded` | See failure reasons |
| **Message** | Classification results | Error description |

**Failure Reasons**:
| Reason | Description | Recovery |
|--------|-------------|----------|
| `ClassificationFailed` | Generic classification failure | Check classifier logic |
| `RegoEvaluationError` | Rego policy execution failed | Check Rego syntax |
| `PolicyNotFound` | Required Rego policy missing | Deploy classification policies |
| `InvalidNamespaceLabels` | Namespace labels malformed | Fix namespace labels |

**Example (Success)**:
```yaml
conditions:
  - type: ClassificationComplete
    status: "True"
    reason: ClassificationSucceeded
    message: "Classified: environment=production (source=namespace-labels), priority=P1 (source=rego-policy, rule=production-critical)"
    lastTransitionTime: "2025-12-16T10:30:01Z"
```

**Example (Failure)**:
```yaml
conditions:
  - type: ClassificationComplete
    status: "False"
    reason: RegoEvaluationError
    message: "Rego evaluation failed: undefined function 'invalid_func' in priority.rego:45"
    lastTransitionTime: "2025-12-16T10:30:01Z"
```

---

### Condition 3: `CategorizationComplete`

**Purpose**: Indicates business categorization completed
**Phase Alignment**: `Categorizing` → `Completed` transition
**BR Reference**: BR-SP-080-081 (Business Classification)

| Field | Success | Failure |
|-------|---------|---------|
| **Type** | `CategorizationComplete` | `CategorizationComplete` |
| **Status** | `True` | `False` |
| **Reason** | `CategorizationSucceeded` | See failure reasons |
| **Message** | Categorization results | Error description |

**Failure Reasons**:
| Reason | Description | Recovery |
|--------|-------------|----------|
| `CategorizationFailed` | Generic categorization failure | Check business logic |
| `InvalidBusinessUnit` | Business unit lookup failed | Verify labels/Rego rules |
| `InvalidSLATier` | SLA tier determination failed | Check SLA configuration |

**Example (Success)**:
```yaml
conditions:
  - type: CategorizationComplete
    status: "True"
    reason: CategorizationSucceeded
    message: "Categorized: businessUnit=payments, serviceOwner=payments-team, criticality=critical, sla=platinum"
    lastTransitionTime: "2025-12-16T10:30:02Z"
```

---

### Condition 4: `ProcessingComplete`

**Purpose**: Indicates entire signal processing pipeline completed
**Phase Alignment**: Transition to `Completed` or `Failed` phase
**BR Reference**: BR-SP-090 (Audit Trail)

| Field | Success | Failure |
|-------|---------|---------|
| **Type** | `ProcessingComplete` | `ProcessingComplete` |
| **Status** | `True` | `False` |
| **Reason** | `ProcessingSucceeded` | See failure reasons |
| **Message** | Processing summary | Error description |

**Failure Reasons**:
| Reason | Description | Recovery |
|--------|-------------|----------|
| `ProcessingFailed` | Generic processing failure | Check controller logs |
| `AuditWriteFailed` | Audit event write failed | Check DataStorage connectivity |
| `ValidationFailed` | Initial signal validation failed | Check signal format |

**Example (Success)**:
```yaml
conditions:
  - type: ProcessingComplete
    status: "True"
    reason: ProcessingSucceeded
    message: "Signal processed successfully in 4.2s: P1 production alert ready for remediation"
    lastTransitionTime: "2025-12-16T10:30:03Z"
```

**Example (Failure)**:
```yaml
conditions:
  - type: ProcessingComplete
    status: "False"
    reason: ValidationFailed
    message: "Signal validation failed: fingerprint missing (required per CRD spec)"
    lastTransitionTime: "2025-12-16T10:30:00Z"
```

---

## 📊 Condition Lifecycle State Machine

```
                          ┌─────────────────────────────────────────────────────────┐
                          │                    PENDING                               │
                          │ Conditions: (none yet)                                   │
                          └─────────────────┬───────────────────────────────────────┘
                                            │
                                            ▼
                          ┌─────────────────────────────────────────────────────────┐
                          │                    ENRICHING                             │
                          │ On Exit (success): EnrichmentComplete=True              │
                          │ On Exit (failure): EnrichmentComplete=False → FAILED    │
                          │ On Exit (degraded): EnrichmentComplete=True (DegradedMode)│
                          └─────────────────┬───────────────────────────────────────┘
                                            │
                                            ▼
                          ┌─────────────────────────────────────────────────────────┐
                          │                    CLASSIFYING                           │
                          │ On Exit (success): ClassificationComplete=True          │
                          │ On Exit (failure): ClassificationComplete=False → FAILED │
                          └─────────────────┬───────────────────────────────────────┘
                                            │
                                            ▼
                          ┌─────────────────────────────────────────────────────────┐
                          │                    CATEGORIZING                          │
                          │ On Exit (success): CategorizationComplete=True          │
                          │ On Exit (failure): CategorizationComplete=False → FAILED │
                          └─────────────────┬───────────────────────────────────────┘
                                            │
                                            ▼
                          ┌─────────────────────────────────────────────────────────┐
                          │                    COMPLETED                             │
                          │ On Entry: ProcessingComplete=True                       │
                          │ All 4 conditions should be True                         │
                          └─────────────────────────────────────────────────────────┘
                                            │
                        (On any failure)    ▼
                          ┌─────────────────────────────────────────────────────────┐
                          │                    FAILED                                │
                          │ ProcessingComplete=False (reason: specific failure)     │
                          │ Phase-specific condition also False                     │
                          └─────────────────────────────────────────────────────────┘
```

---

## 🔧 Implementation Specification

### File: `pkg/signalprocessing/conditions.go`

```go
/*
Copyright 2025 Jordi Gil.
*/

package signalprocessing

import (
    "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// ========================================
// KUBERNETES CONDITIONS (DD-SP-002)
// 📋 Design Decision: DD-SP-002 | ✅ Approved Design | Confidence: 95%
// See: docs/architecture/decisions/DD-SP-002-kubernetes-conditions-specification.md
// ========================================
//
// Kubernetes Conditions provide detailed status for SignalProcessing CRD.
// Enables operators to debug processing issues via `kubectl describe`.
//
// CONDITION LIFECYCLE:
// 1. EnrichmentComplete    → After K8s context enrichment (BR-SP-001)
// 2. ClassificationComplete → After environment/priority classification (BR-SP-051-072)
// 3. CategorizationComplete → After business categorization (BR-SP-080-081)
// 4. ProcessingComplete     → Terminal state (BR-SP-090)
// ========================================

// Condition types for SignalProcessing
const (
    // ConditionEnrichmentComplete indicates K8s context enrichment finished
    // Phase Alignment: Enriching → Classifying transition
    // BR Reference: BR-SP-001 (K8s Context Enrichment)
    ConditionEnrichmentComplete = "EnrichmentComplete"

    // ConditionClassificationComplete indicates environment/priority classification finished
    // Phase Alignment: Classifying → Categorizing transition
    // BR Reference: BR-SP-051-053 (Environment), BR-SP-070-072 (Priority)
    ConditionClassificationComplete = "ClassificationComplete"

    // ConditionCategorizationComplete indicates business categorization finished
    // Phase Alignment: Categorizing → Completed transition
    // BR Reference: BR-SP-080-081 (Business Classification)
    ConditionCategorizationComplete = "CategorizationComplete"

    // ConditionProcessingComplete indicates entire signal processing finished
    // Phase Alignment: Completed or Failed phase
    // BR Reference: BR-SP-090 (Audit Trail)
    ConditionProcessingComplete = "ProcessingComplete"
)

// Condition reasons for EnrichmentComplete
const (
    ReasonEnrichmentSucceeded = "EnrichmentSucceeded"
    ReasonEnrichmentFailed    = "EnrichmentFailed"
    ReasonK8sAPITimeout       = "K8sAPITimeout"
    ReasonResourceNotFound    = "ResourceNotFound"
    ReasonRBACDenied          = "RBACDenied"
    ReasonDegradedMode        = "DegradedMode"
)

// Condition reasons for ClassificationComplete
const (
    ReasonClassificationSucceeded = "ClassificationSucceeded"
    ReasonClassificationFailed    = "ClassificationFailed"
    ReasonRegoEvaluationError     = "RegoEvaluationError"
    ReasonPolicyNotFound          = "PolicyNotFound"
    ReasonInvalidNamespaceLabels  = "InvalidNamespaceLabels"
)

// Condition reasons for CategorizationComplete
const (
    ReasonCategorizationSucceeded = "CategorizationSucceeded"
    ReasonCategorizationFailed    = "CategorizationFailed"
    ReasonInvalidBusinessUnit     = "InvalidBusinessUnit"
    ReasonInvalidSLATier          = "InvalidSLATier"
)

// Condition reasons for ProcessingComplete
const (
    ReasonProcessingSucceeded = "ProcessingSucceeded"
    ReasonProcessingFailed    = "ProcessingFailed"
    ReasonAuditWriteFailed    = "AuditWriteFailed"
    ReasonValidationFailed    = "ValidationFailed"
)

// SetCondition sets or updates a condition on the SignalProcessing status
func SetCondition(sp *spv1.SignalProcessing, conditionType string, status metav1.ConditionStatus, reason, message string) {
    condition := metav1.Condition{
        Type:               conditionType,
        Status:             status,
        LastTransitionTime: metav1.Now(),
        Reason:             reason,
        Message:            message,
    }
    meta.SetStatusCondition(&sp.Status.Conditions, condition)
}

// GetCondition returns the condition with the specified type, or nil if not found
func GetCondition(sp *spv1.SignalProcessing, conditionType string) *metav1.Condition {
    return meta.FindStatusCondition(sp.Status.Conditions, conditionType)
}

// IsConditionTrue returns true if the condition exists and has status True
func IsConditionTrue(sp *spv1.SignalProcessing, conditionType string) bool {
    condition := GetCondition(sp, conditionType)
    return condition != nil && condition.Status == metav1.ConditionTrue
}

// ========================================
// HIGH-LEVEL CONDITION SETTERS
// ========================================

// SetEnrichmentComplete sets the EnrichmentComplete condition
func SetEnrichmentComplete(sp *spv1.SignalProcessing, succeeded bool, reason, message string) {
    status := metav1.ConditionTrue
    if !succeeded {
        status = metav1.ConditionFalse
    }
    if reason == "" {
        reason = ReasonEnrichmentSucceeded
        if !succeeded {
            reason = ReasonEnrichmentFailed
        }
    }
    SetCondition(sp, ConditionEnrichmentComplete, status, reason, message)
}

// SetClassificationComplete sets the ClassificationComplete condition
func SetClassificationComplete(sp *spv1.SignalProcessing, succeeded bool, reason, message string) {
    status := metav1.ConditionTrue
    if !succeeded {
        status = metav1.ConditionFalse
    }
    if reason == "" {
        reason = ReasonClassificationSucceeded
        if !succeeded {
            reason = ReasonClassificationFailed
        }
    }
    SetCondition(sp, ConditionClassificationComplete, status, reason, message)
}

// SetCategorizationComplete sets the CategorizationComplete condition
func SetCategorizationComplete(sp *spv1.SignalProcessing, succeeded bool, reason, message string) {
    status := metav1.ConditionTrue
    if !succeeded {
        status = metav1.ConditionFalse
    }
    if reason == "" {
        reason = ReasonCategorizationSucceeded
        if !succeeded {
            reason = ReasonCategorizationFailed
        }
    }
    SetCondition(sp, ConditionCategorizationComplete, status, reason, message)
}

// SetProcessingComplete sets the ProcessingComplete condition
func SetProcessingComplete(sp *spv1.SignalProcessing, succeeded bool, reason, message string) {
    status := metav1.ConditionTrue
    if !succeeded {
        status = metav1.ConditionFalse
    }
    if reason == "" {
        reason = ReasonProcessingSucceeded
        if !succeeded {
            reason = ReasonProcessingFailed
        }
    }
    SetCondition(sp, ConditionProcessingComplete, status, reason, message)
}
```

---

## 🧪 Testing Requirements

### Unit Tests

**File**: `test/unit/signalprocessing/conditions_test.go`

**Test Cases**:
1. `SetCondition` - sets condition correctly
2. `GetCondition` - returns nil for non-existent condition
3. `IsConditionTrue` - returns true/false correctly
4. `SetEnrichmentComplete` - success and failure cases
5. `SetClassificationComplete` - success and failure cases
6. `SetCategorizationComplete` - success and failure cases
7. `SetProcessingComplete` - success and failure cases

### Integration Tests

Verify conditions are populated during reconciliation:
- After enrichment phase → `EnrichmentComplete` condition exists
- After classification phase → `ClassificationComplete` condition exists
- After categorization phase → `CategorizationComplete` condition exists
- After completion → All 4 conditions are True
- On failure → Relevant condition is False with correct reason

---

## 📋 Operator Experience

### Before (No Conditions)

```bash
$ kubectl describe signalprocessing sp-123
Status:
  Phase: Classifying
  Error:
  # No indication of what succeeded or why stuck
```

### After (With Conditions)

```bash
$ kubectl describe signalprocessing sp-123
Status:
  Phase: Classifying
  Conditions:
    Type:     EnrichmentComplete
    Status:   True
    Reason:   EnrichmentSucceeded
    Message:  K8s context enriched: Pod payments-api-abc123, Deployment payments-api

    Type:     ClassificationComplete
    Status:   False
    Reason:   RegoEvaluationError
    Message:  Rego evaluation failed: undefined function 'invalid_func' in priority.rego:45
```

### Automation Support

```bash
# Wait for specific condition
kubectl wait --for=condition=ProcessingComplete signalprocessing/sp-123 --timeout=60s

# Check condition status programmatically
kubectl get signalprocessing sp-123 -o jsonpath='{.status.conditions[?(@.type=="EnrichmentComplete")].status}'
```

---

## ✅ Cross-Team Validation

### Validation Requested From

| Team | Aspect | Status |
|------|--------|--------|
| **RemediationOrchestrator** | Consumes SP status - conditions useful? | ⏳ Pending |
| **AIAnalysis** | Reference implementation - consistent? | ⏳ Pending |
| **Platform** | DD-CRD-002 compliance | ✅ Compliant |

### Validation Questions

1. **RO Team**: Do you need to check SP conditions before processing?
2. **AA Team**: Should SP conditions follow same pattern as AIAnalysis?
3. **All Teams**: Are the failure reasons sufficient for debugging?

---

## 🔗 Related Documents

- **Parent Standard**: [DD-CRD-002](./DD-CRD-002-kubernetes-conditions-standard.md) - Kubernetes Conditions Standard
- **Implementation Plan**: [IMPLEMENTATION_PLAN_CONDITIONS_V1.0.md](../../services/crd-controllers/01-signalprocessing/IMPLEMENTATION_PLAN_CONDITIONS_V1.0.md)
- **Reference Implementation**: `pkg/aianalysis/conditions.go`
- **CRD Types**: `api/signalprocessing/v1alpha1/signalprocessing_types.go`

---

## 📊 Implementation Metrics

| Metric | Target |
|--------|--------|
| **Conditions Count** | 4 (EnrichmentComplete, ClassificationComplete, CategorizationComplete, ProcessingComplete) |
| **Failure Reasons** | 16 (4 per condition average) |
| **Helper Functions** | 8 (Set/Get for each condition + generic) |
| **Unit Test Coverage** | 100% of helper functions |
| **Integration Coverage** | All phase transitions |

---

**Document Version**: 1.0
**Created**: 2025-12-16
**Author**: SignalProcessing Team (@jgil)
**File**: `docs/architecture/decisions/DD-SP-002-kubernetes-conditions-specification.md`




