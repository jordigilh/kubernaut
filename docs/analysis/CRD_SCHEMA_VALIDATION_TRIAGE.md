# CRD Schema Validation Triage

**Date**: 2025-10-09  
**Status**: 🔍 Triage Complete  
**Priority**: P1 - Important (API-level validation and documentation)

---

## 📋 **Executive Summary**

This document provides a comprehensive triage of schema validations to be added to all Kubernaut CRDs. Kubebuilder validation markers provide:
- **API-level validation**: Kubernetes API server enforces constraints
- **Better error messages**: Users get immediate feedback on invalid input
- **Self-documenting API**: OpenAPI schema documents expected values
- **Prevention of invalid states**: Catches errors before reconciliation

---

## 🎯 **Validation Categories**

### **Category 1: Enum Values (P0 - Critical)**
Fields with fixed set of valid values should use `+kubebuilder:validation:Enum`

### **Category 2: Numeric Ranges (P0 - Critical)**
Fields with logical min/max bounds should use `+kubebuilder:validation:Minimum/Maximum`

### **Category 3: String Patterns (P1 - Important)**
Fields with format requirements should use `+kubebuilder:validation:Pattern` or `+kubebuilder:validation:MaxLength`

### **Category 4: Required Fields (P1 - Important)**
Critical fields should be marked as required in the parent struct

### **Category 5: Format Validation (P2 - Nice to Have)**
Fields with specific formats (URLs, durations, etc.)

---

## 📊 **CRD-by-CRD Analysis**

### **1. RemediationRequest CRD** (`api/remediation/v1alpha1/remediationrequest_types.go`)

#### **Enum Validations (P0)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `Severity` | string | `+kubebuilder:validation:Enum=critical;warning;info` | Fixed set defined in architecture |
| `Environment` | string | `+kubebuilder:validation:Enum=prod;staging;dev` | Fixed set defined in architecture |
| `Priority` | string | `+kubebuilder:validation:Enum=P0;P1;P2` | Gateway assigns only these values |
| `SignalType` | string | No validation (extensible) | V2 will add aws-cloudwatch, datadog-monitor, etc. |
| `TargetType` | string | `+kubebuilder:validation:Enum=kubernetes;aws;azure;gcp;datadog` | Fixed set for V1/V2 |
| `Status.OverallPhase` | string | `+kubebuilder:validation:Enum=pending;processing;analyzing;executing;completed;failed;timeout` | State machine phases |

#### **String Length Validations (P1)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `SignalFingerprint` | string | `+kubebuilder:validation:MaxLength=64` | SHA256 hex = 64 chars |
| `SignalName` | string | `+kubebuilder:validation:MaxLength=253` | DNS subdomain length |
| `SignalSource` | string | `+kubebuilder:validation:MaxLength=63` | Kubernetes label value limit |

#### **Pattern Validations (P1)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `SignalFingerprint` | string | `+kubebuilder:validation:Pattern="^[a-f0-9]{64}$"` | SHA256 hex format |
| `Priority` | string | `+kubebuilder:validation:Pattern="^P[0-2]$"` | P0, P1, or P2 format |

#### **Required Fields (P1)**

```go
type RemediationRequestSpec struct {
    // +kubebuilder:validation:Required
    SignalFingerprint string `json:"signalFingerprint"`
    
    // +kubebuilder:validation:Required
    SignalName string `json:"signalName"`
    
    // +kubebuilder:validation:Required
    Severity string `json:"severity"`
    
    // +kubebuilder:validation:Required
    Environment string `json:"environment"`
    
    // +kubebuilder:validation:Required
    Priority string `json:"priority"`
    
    // +kubebuilder:validation:Required
    SignalType string `json:"signalType"`
    
    // +kubebuilder:validation:Required
    TargetType string `json:"targetType"`
}
```

---

### **2. RemediationProcessing CRD** (`api/remediationprocessing/v1alpha1/remediationprocessing_types.go`)

#### **Enum Validations (P0)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `Severity` | string | `+kubebuilder:validation:Enum=critical;warning;info` | Same as RemediationRequest |
| `Environment` | string | `+kubebuilder:validation:Enum=prod;staging;dev` | Same as RemediationRequest |
| `Priority` | string | `+kubebuilder:validation:Enum=P0;P1;P2` | Same as RemediationRequest |
| `TargetType` | string | `+kubebuilder:validation:Enum=kubernetes;aws;azure;gcp;datadog` | Same as RemediationRequest |
| `Status.Phase` | string | `+kubebuilder:validation:Enum=pending;enriching;classifying;routing;completed;failed` | Processing phases |

#### **String Length Validations (P1)**

Same as RemediationRequest for copied fields.

#### **Required Fields (P1)**

```go
type RemediationProcessingSpec struct {
    // +kubebuilder:validation:Required
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`
    
    // +kubebuilder:validation:Required
    SignalFingerprint string `json:"signalFingerprint"`
    
    // +kubebuilder:validation:Required
    SignalType string `json:"signalType"`
    
    // +kubebuilder:validation:Required
    TargetType string `json:"targetType"`
}
```

---

### **3. AIAnalysis CRD** (`api/aianalysis/v1alpha1/aianalysis_types.go`)

#### **Enum Validations (P0)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `LLMProvider` | string | `+kubebuilder:validation:Enum=openai;anthropic;local;holmesgpt` | Supported LLM providers |
| `Status.Phase` | string | `+kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Recommending;Completed;Failed` | AI analysis phases |

#### **Numeric Range Validations (P0)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `Temperature` | float64 | `+kubebuilder:validation:Minimum=0.0`<br>`+kubebuilder:validation:Maximum=1.0` | **✅ ALREADY DONE** |
| `Confidence` | float64 | `+kubebuilder:validation:Minimum=0.0`<br>`+kubebuilder:validation:Maximum=1.0` | **✅ ALREADY DONE** |
| `MaxTokens` | int | `+kubebuilder:validation:Minimum=1`<br>`+kubebuilder:validation:Maximum=100000` | Reasonable LLM token limits |
| `TokensUsed` | int | `+kubebuilder:validation:Minimum=0` | Cannot be negative |
| `InvestigationTime` | int64 | `+kubebuilder:validation:Minimum=0` | Duration in seconds, non-negative |

#### **String Length Validations (P1)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `RemediationRequestRef` | string | `+kubebuilder:validation:MaxLength=253` | Kubernetes name limit |
| `LLMProvider` | string | `+kubebuilder:validation:MaxLength=63` | Provider name |
| `LLMModel` | string | `+kubebuilder:validation:MaxLength=253` | Model identifier (can be long) |
| `InvestigationID` | string | `+kubebuilder:validation:MaxLength=253` | HolmesGPT investigation ID |
| `RootCause` | string | `+kubebuilder:validation:MaxLength=2048` | Reasonable description length |
| `RecommendedAction` | string | `+kubebuilder:validation:MaxLength=2048` | Reasonable action description |

#### **Required Fields (P1)**

```go
type AIAnalysisSpec struct {
    // +kubebuilder:validation:Required
    RemediationRequestRef string `json:"remediationRequestRef"`
    
    // +kubebuilder:validation:Required
    SignalType string `json:"signalType"`
    
    // +kubebuilder:validation:Required
    LLMProvider string `json:"llmProvider"`
    
    // +kubebuilder:validation:Required
    LLMModel string `json:"llmModel"`
}
```

---

### **4. WorkflowExecution CRD** (`api/workflowexecution/v1alpha1/workflowexecution_types.go`)

#### **Enum Validations (P0)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `ExecutionStrategy.RollbackStrategy` | string | `+kubebuilder:validation:Enum=automatic;manual;none` | Fixed rollback strategies |
| `Status.Phase` | string | `+kubebuilder:validation:Enum=planning;validating;executing;monitoring;completed;failed;paused` | Workflow phases |
| `StepStatus.Status` | string | `+kubebuilder:validation:Enum=pending;executing;completed;failed;rolled_back;skipped` | Step execution states |
| `WorkflowResult.Outcome` | string | `+kubebuilder:validation:Enum=success;partial_success;failed;unknown` | Final outcomes |
| `WorkflowResult.ResourceHealth` | string | `+kubebuilder:validation:Enum=healthy;degraded;unhealthy;unknown` | Health states |
| `ExecutionPlan.Strategy` | string | `+kubebuilder:validation:Enum=sequential;parallel;sequential-with-parallel` | Execution strategies |

#### **Numeric Range Validations (P0)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `WorkflowStep.StepNumber` | int | `+kubebuilder:validation:Minimum=1` | Steps start at 1 |
| `WorkflowStep.MaxRetries` | int | `+kubebuilder:validation:Minimum=0`<br>`+kubebuilder:validation:Maximum=10` | Reasonable retry limit |
| `ExecutionStrategy.MaxRetries` | int | `+kubebuilder:validation:Minimum=0`<br>`+kubebuilder:validation:Maximum=10` | Reasonable retry limit |
| `StepStatus.RetriesAttempted` | int | `+kubebuilder:validation:Minimum=0` | Non-negative |
| `Status.CurrentStep` | int | `+kubebuilder:validation:Minimum=0` | 0-based index |
| `Status.TotalSteps` | int | `+kubebuilder:validation:Minimum=0` | Non-negative |
| `Status.CompletedCount` | int | `+kubebuilder:validation:Minimum=0` | Non-negative |
| `ExecutionMetrics.StepSuccessRate` | float64 | `+kubebuilder:validation:Minimum=0.0`<br>`+kubebuilder:validation:Maximum=1.0` | Percentage as fraction |
| `WorkflowResult.EffectivenessScore` | float64 | `+kubebuilder:validation:Minimum=0.0`<br>`+kubebuilder:validation:Maximum=1.0` | Score as fraction |
| `AIRecommendations.OverallConfidence` | float64 | `+kubebuilder:validation:Minimum=0.0`<br>`+kubebuilder:validation:Maximum=1.0` | Confidence as fraction |
| `StepOptimization.Confidence` | float64 | `+kubebuilder:validation:Minimum=0.0`<br>`+kubebuilder:validation:Maximum=1.0` | Confidence as fraction |
| `RiskFactor.Probability` | float64 | `+kubebuilder:validation:Minimum=0.0`<br>`+kubebuilder:validation:Maximum=1.0` | Probability as fraction |

#### **String Pattern Validations (P1)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `WorkflowStep.Timeout` | string | `+kubebuilder:validation:Pattern="^[0-9]+(s\|m\|h)$"` | Duration format (e.g., "5m", "30s") |
| `ExecutionMetrics.TotalDuration` | string | `+kubebuilder:validation:Pattern="^[0-9]+(s\|m\|h)$"` | Duration format |

#### **Required Fields (P1)**

```go
type WorkflowExecutionSpec struct {
    // +kubebuilder:validation:Required
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`
    
    // +kubebuilder:validation:Required
    WorkflowDefinition WorkflowDefinition `json:"workflowDefinition"`
    
    // +kubebuilder:validation:Required
    ExecutionStrategy ExecutionStrategy `json:"executionStrategy"`
}

type WorkflowDefinition struct {
    // +kubebuilder:validation:Required
    Name string `json:"name"`
    
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinItems=1
    Steps []WorkflowStep `json:"steps"`
}
```

---

### **5. KubernetesExecution CRD** (`api/kubernetesexecution/v1alpha1/kubernetesexecution_types.go`)

#### **Enum Validations (P0)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `Action` | string | `+kubebuilder:validation:Enum=scale_deployment;rollout_restart;delete_pod;patch_deployment;cordon_node;drain_node;uncordon_node;update_configmap;update_secret;apply_manifest` | Fixed action types |
| `PatchDeploymentParams.PatchType` | string | `+kubebuilder:validation:Enum=strategic;merge;json` | Kubernetes patch types |
| `Status.Phase` | string | `+kubebuilder:validation:Enum=validating;validated;waiting_approval;executing;rollback_ready;completed;failed` | Execution phases |

#### **Numeric Range Validations (P0)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| `StepNumber` | int | `+kubebuilder:validation:Minimum=1` | Steps start at 1 |
| `MaxRetries` | int | `+kubebuilder:validation:Minimum=0`<br>`+kubebuilder:validation:Maximum=5` | Reasonable retry limit for K8s operations |
| `ScaleDeploymentParams.Replicas` | int32 | `+kubebuilder:validation:Minimum=0`<br>`+kubebuilder:validation:Maximum=1000` | Reasonable replica limits |
| `DeletePodParams.GracePeriodSeconds` | int64 | `+kubebuilder:validation:Minimum=0`<br>`+kubebuilder:validation:Maximum=3600` | 1 hour max grace period |
| `DrainNodeParams.GracePeriodSeconds` | int64 | `+kubebuilder:validation:Minimum=0`<br>`+kubebuilder:validation:Maximum=3600` | 1 hour max grace period |
| `ExecutionResults.RetriesAttempted` | int | `+kubebuilder:validation:Minimum=0` | Non-negative |

#### **String Length Validations (P1)**

| Field | Current | Recommended | Justification |
|-------|---------|-------------|---------------|
| Namespace fields | string | `+kubebuilder:validation:MaxLength=63` | Kubernetes namespace limit |
| Name fields (Pod, Deployment, etc.) | string | `+kubebuilder:validation:MaxLength=253` | Kubernetes name limit |
| `Action` | string | `+kubebuilder:validation:MaxLength=63` | Action name limit |

#### **Required Fields (P1)**

```go
type KubernetesExecutionSpec struct {
    // +kubebuilder:validation:Required
    WorkflowExecutionRef corev1.ObjectReference `json:"workflowExecutionRef"`
    
    // +kubebuilder:validation:Required
    StepNumber int `json:"stepNumber"`
    
    // +kubebuilder:validation:Required
    Action string `json:"action"`
    
    // +kubebuilder:validation:Required
    Parameters *ActionParameters `json:"parameters"`
}
```

---

## 🎯 **Implementation Priority**

### **Phase 1: Critical Validations (P0)** - **IMPLEMENT FIRST**
1. ✅ Enum validations for all phase fields
2. ✅ Numeric range validations for confidence/probability fields (0.0-1.0)
3. ✅ Numeric range validations for retry counts
4. ✅ Numeric range validations for replica counts

### **Phase 2: Important Validations (P1)** - **IMPLEMENT SECOND**
1. ✅ Required field markers
2. ✅ String length validations (MaxLength)
3. ✅ Pattern validations for durations and fingerprints
4. ✅ MinItems validation for array fields

### **Phase 3: Nice-to-Have (P2)** - **IMPLEMENT IF TIME ALLOWS**
1. ✅ Format validations for URLs
2. ✅ More complex pattern validations
3. ✅ Cross-field validations (CEL expressions)

---

## 📝 **Kubebuilder Marker Reference**

### **Common Validation Markers**

```go
// Enum validation
// +kubebuilder:validation:Enum=value1;value2;value3

// Numeric range
// +kubebuilder:validation:Minimum=0
// +kubebuilder:validation:Maximum=100
// +kubebuilder:validation:ExclusiveMinimum=false
// +kubebuilder:validation:ExclusiveMaximum=false

// String validation
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:Pattern="^[a-z0-9-]+$"

// Array validation
// +kubebuilder:validation:MinItems=1
// +kubebuilder:validation:MaxItems=100
// +kubebuilder:validation:UniqueItems=true

// Required field (on parent struct)
// +kubebuilder:validation:Required

// Format validation
// +kubebuilder:validation:Format=date-time
// +kubebuilder:validation:Format=uri
// +kubebuilder:validation:Format=email

// Custom validation (CEL)
// +kubebuilder:validation:XValidation:rule="self.minReplicas <= self.maxReplicas",message="minReplicas must be less than or equal to maxReplicas"
```

---

## ✅ **Testing Strategy**

After adding validations:

1. **Positive Tests**: Verify valid values are accepted
   ```bash
   kubectl apply -f valid-remediation-request.yaml
   ```

2. **Negative Tests**: Verify invalid values are rejected
   ```bash
   # Should fail with validation error
   kubectl apply -f invalid-severity.yaml
   # Error: ValidationError: spec.severity: Unsupported value: "unknown": supported values: "critical", "warning", "info"
   ```

3. **Edge Cases**: Test boundary values
   - Minimum/maximum numeric values
   - Empty vs. omitted optional fields
   - MaxLength boundaries

4. **Manifest Verification**: Check generated CRD YAMLs
   ```bash
   cat config/crd/bases/remediation.kubernaut.io_remediationrequests.yaml | grep -A 10 "validation:"
   ```

---

## 📊 **Estimated Impact**

### **Benefits**
- ✅ **Better UX**: Users get immediate feedback on invalid input
- ✅ **Self-documenting**: OpenAPI schema documents valid values
- ✅ **Fewer bugs**: Invalid states caught at API level, not reconciliation
- ✅ **Performance**: Less reconciliation work on invalid CRDs
- ✅ **Security**: Prevents malformed data injection

### **Effort Estimate**
- Phase 1 (P0): **2 hours** - Critical enums and numeric ranges
- Phase 2 (P1): **3 hours** - Required fields and string validations
- Phase 3 (P2): **2 hours** - Nice-to-have validations
- **Total**: **7 hours**

### **Risk Assessment**
- **Low Risk**: Validations only affect new CRD creation
- **No Breaking Changes**: Existing CRDs are grandfathered
- **Rollback**: Remove markers and regenerate if issues occur

---

## 🚀 **Implementation Plan**

### **Step 1: Add Validation Markers**
For each CRD type file, add appropriate `+kubebuilder:validation:*` markers above fields.

### **Step 2: Regenerate CRD Manifests**
```bash
make manifests
```

### **Step 3: Verify Generated Validations**
```bash
# Check that validations appear in OpenAPI schema
cat config/crd/bases/*.yaml | grep -A 5 "enum:"
cat config/crd/bases/*.yaml | grep -A 5 "minimum:"
cat config/crd/bases/*.yaml | grep -A 5 "pattern:"
```

### **Step 4: Test Validations**
Create test YAML files with:
- Valid values (should succeed)
- Invalid enum values (should fail)
- Out-of-range numeric values (should fail)
- Invalid string patterns (should fail)

### **Step 5: Update Documentation**
Document validation rules in:
- CRD schema documentation (`docs/architecture/CRD_SCHEMAS.md`)
- User-facing API documentation
- Error handling guides

---

## 📚 **References**

- **Kubebuilder Book**: [CRD Validation](https://book.kubebuilder.io/reference/markers/crd-validation.html)
- **Kubernetes API Conventions**: [Validation](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#validation)
- **OpenAPI v3 Schema**: [Validation Keywords](https://swagger.io/docs/specification/data-models/keywords/)
- **CEL in Kubernetes**: [Common Expression Language](https://kubernetes.io/docs/reference/using-api/cel/)

---

**Status**: ✅ **Triage Complete** - Ready for implementation  
**Next Step**: Apply Phase 1 (P0) validations to all CRDs  
**Confidence**: 95% - Comprehensive analysis with clear implementation path

