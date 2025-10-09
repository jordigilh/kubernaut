# Task 2.2 Implementation Guide: RemediationOrchestrator Field Mapping

**Task**: Update RemediationOrchestrator to populate 18 self-contained fields in RemediationProcessing CRD
**Estimated Time**: 1 hour 15 minutes
**Priority**: HIGH (Critical for Phase 1 self-containment pattern)
**Status**: Ready for implementation

---

## 🎯 **Objective**

Implement the field mapping logic in RemediationOrchestrator controller that copies all necessary data from RemediationRequest to RemediationProcessing, enabling the self-contained CRD pattern.

---

## 📋 **Prerequisites**

**Completed**:
- ✅ Task 2.1: RemediationProcessing CRD has 18 fields defined
- ✅ RemediationRequest CRD has complete Phase 1 schema
- ✅ Both CRDs generated with `make manifests`

**Required**:
- RemediationOrchestrator controller exists (scaffolded by Kubebuilder)
- Understanding of controller-runtime reconciliation loop

---

## 🏗️ **Architecture Context**

### **Current Controller State**

**File**: `internal/controller/remediationorchestrator/remediationorchestrator_controller.go`
**Status**: Scaffolded (no implementation)

**Note**: This controller reconciles `RemediationOrchestrator` CRD, NOT `RemediationRequest`.
**Clarification**: Based on service specs, the controller that orchestrates the workflow is likely the `RemediationRequest` controller (in `internal/controller/remediation/`), not the `RemediationOrchestrator` controller.

### **Correct Controller to Modify**

**Actual Target**: `internal/controller/remediation/remediationrequest_controller.go`

**Why**:
- `RemediationRequest` controller is the central orchestrator
- It creates and manages child CRDs (RemediationProcessing, AIAnalysis, etc.)
- It implements the "Targeting Data Pattern" from service specs

**Service Spec Reference**: `docs/services/crd-controllers/05-remediationorchestrator/README.md`

---

## 🔄 **Data Flow**

```
┌─────────────────────────────────────────┐
│ Gateway Service                          │
│ Creates RemediationRequest CRD          │
│ with complete signal data               │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│ RemediationRequest Controller           │
│ (Reconciles RemediationRequest)         │
│                                         │
│ 1. Reads RemediationRequest             │
│ 2. Extracts all 18 fields              │
│ 3. Creates RemediationProcessing CRD   │
│    with copied data                     │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│ RemediationProcessing CRD               │
│ (Self-contained with all data)          │
│                                         │
│ RemediationProcessor controller reads   │
│ this CRD ONLY (no external reads)       │
└─────────────────────────────────────────┘
```

---

## 📝 **Implementation Steps**

### **Step 1: Update RemediationRequest Controller** (45 min)

**File**: `internal/controller/remediation/remediationrequest_controller.go`

#### **Add Field Mapping Function**

```go
// mapRemediationRequestToProcessingSpec maps RemediationRequest data to RemediationProcessing spec
// Phase 1: Implements self-contained CRD pattern - all data copied, no external reads required
func mapRemediationRequestToProcessingSpec(rr *remediationv1alpha1.RemediationRequest) remediationprocessingv1alpha1.RemediationProcessingSpec {
	return remediationprocessingv1alpha1.RemediationProcessingSpec{
		// ========================================
		// PARENT REFERENCE (Audit/Lineage Only)
		// ========================================
		RemediationRequestRef: corev1.ObjectReference{
			APIVersion: rr.APIVersion,
			Kind:       rr.Kind,
			Name:       rr.Name,
			Namespace:  rr.Namespace,
			UID:        rr.UID,
		},

		// ========================================
		// SIGNAL IDENTIFICATION (From RemediationRequest)
		// ========================================
		SignalFingerprint: rr.Spec.SignalFingerprint,
		SignalName:        rr.Spec.SignalName,
		Severity:          rr.Spec.Severity,

		// ========================================
		// SIGNAL CLASSIFICATION (From RemediationRequest)
		// ========================================
		Environment:  rr.Spec.Environment,
		Priority:     rr.Spec.Priority,
		SignalType:   rr.Spec.SignalType,
		SignalSource: rr.Spec.SignalSource,
		TargetType:   rr.Spec.TargetType,

		// ========================================
		// SIGNAL METADATA (From RemediationRequest)
		// ========================================
		SignalLabels:      deepCopyStringMap(rr.Spec.SignalLabels),
		SignalAnnotations: deepCopyStringMap(rr.Spec.SignalAnnotations),

		// ========================================
		// TARGET RESOURCE (From RemediationRequest)
		// ========================================
		TargetResource: extractTargetResource(rr),

		// ========================================
		// TIMESTAMPS (From RemediationRequest)
		// ========================================
		FiringTime:   rr.Spec.FiringTime,
		ReceivedTime: rr.Spec.ReceivedTime,

		// ========================================
		// DEDUPLICATION (From RemediationRequest)
		// ========================================
		Deduplication: mapDeduplicationInfo(rr.Spec.Deduplication),

		// ========================================
		// PROVIDER DATA (From RemediationRequest)
		// ========================================
		ProviderData:    deepCopyBytes(rr.Spec.ProviderData),
		OriginalPayload: deepCopyBytes(rr.Spec.OriginalPayload),

		// ========================================
		// STORM DETECTION (From RemediationRequest)
		// ========================================
		IsStorm:         rr.Spec.IsStorm,
		StormAlertCount: rr.Spec.StormAlertCount,

		// ========================================
		// CONFIGURATION (Optional, defaults if not specified)
		// ========================================
		EnrichmentConfig: getDefaultEnrichmentConfig(),
	}
}
```

---

#### **Add Helper Functions**

```go
// deepCopyStringMap creates a deep copy of string map
func deepCopyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	copied := make(map[string]string, len(m))
	for k, v := range m {
		copied[k] = v
	}
	return copied
}

// deepCopyBytes creates a deep copy of byte slice
func deepCopyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	copied := make([]byte, len(b))
	copy(copied, b)
	return copied
}

// extractTargetResource extracts target resource from RemediationRequest
// Phase 1: Parses providerData to identify Kubernetes resource
// Future: Support for AWS/Azure/GCP resources
func extractTargetResource(rr *remediationv1alpha1.RemediationRequest) remediationprocessingv1alpha1.ResourceIdentifier {
	// Default empty resource if extraction fails
	resource := remediationprocessingv1alpha1.ResourceIdentifier{}

	// For Kubernetes signals, parse providerData
	if rr.Spec.TargetType == "kubernetes" {
		// Parse Kubernetes-specific fields from providerData
		var kubernetesData struct {
			Namespace string `json:"namespace"`
			Resource  struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"resource"`
		}

		if err := json.Unmarshal(rr.Spec.ProviderData, &kubernetesData); err == nil {
			resource.Namespace = kubernetesData.Namespace
			resource.Kind = kubernetesData.Resource.Kind
			resource.Name = kubernetesData.Resource.Name
		}
	}

	// Fallback: Try to extract from signal labels
	if resource.Namespace == "" {
		if ns, ok := rr.Spec.SignalLabels["namespace"]; ok {
			resource.Namespace = ns
		}
	}
	if resource.Kind == "" {
		// Common label keys for resource kind
		for _, key := range []string{"kind", "resource_kind", "object_kind"} {
			if kind, ok := rr.Spec.SignalLabels[key]; ok {
				resource.Kind = kind
				break
			}
		}
	}
	if resource.Name == "" {
		// Common label keys for resource name
		for _, key := range []string{"pod", "deployment", "statefulset", "daemonset", "resource_name"} {
			if name, ok := rr.Spec.SignalLabels[key]; ok {
				resource.Name = name
				if resource.Kind == "" {
					// Infer kind from label key
					resource.Kind = strings.Title(key)
				}
				break
			}
		}
	}

	return resource
}

// mapDeduplicationInfo maps RemediationRequest deduplication to RemediationProcessing format
func mapDeduplicationInfo(dedupInfo remediationv1alpha1.DeduplicationInfo) remediationprocessingv1alpha1.DeduplicationContext {
	return remediationprocessingv1alpha1.DeduplicationContext{
		FirstOccurrence: dedupInfo.FirstSeen,
		LastOccurrence:  dedupInfo.LastSeen,
		OccurrenceCount: dedupInfo.OccurrenceCount,
		CorrelationID:   dedupInfo.PreviousRemediationRequestRef, // Optional correlation
	}
}

// getDefaultEnrichmentConfig returns default enrichment configuration
func getDefaultEnrichmentConfig() *remediationprocessingv1alpha1.EnrichmentConfiguration {
	return &remediationprocessingv1alpha1.EnrichmentConfiguration{
		EnableClusterState: true,  // Enable by default for Kubernetes signals
		EnableMetrics:      true,  // Enable by default for context
		EnableHistorical:   false, // Disabled by default (performance)
	}
}
```

---

### **Step 2: Create RemediationProcessing CRD in Reconcile Loop** (20 min)

```go
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the RemediationRequest instance
	var remediationRequest remediationv1alpha1.RemediationRequest
	if err := r.Get(ctx, req.NamespacedName, &remediationRequest); err != nil {
		// Handle not found or other errors
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if RemediationProcessing CRD already exists
	processingName := fmt.Sprintf("%s-processing", remediationRequest.Name)
	var existingProcessing remediationprocessingv1alpha1.RemediationProcessing
	err := r.Get(ctx, client.ObjectKey{
		Name:      processingName,
		Namespace: remediationRequest.Namespace,
	}, &existingProcessing)

	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to check for existing RemediationProcessing")
		return ctrl.Result{}, err
	}

	// Create RemediationProcessing if it doesn't exist
	if errors.IsNotFound(err) {
		processing := &remediationprocessingv1alpha1.RemediationProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      processingName,
				Namespace: remediationRequest.Namespace,
				Labels: map[string]string{
					"remediation-request": remediationRequest.Name,
					"signal-type":         remediationRequest.Spec.SignalType,
					"environment":         remediationRequest.Spec.Environment,
					"priority":            remediationRequest.Spec.Priority,
				},
				Annotations: map[string]string{
					"signal-fingerprint": remediationRequest.Spec.SignalFingerprint,
					"created-by":         "remediation-orchestrator",
				},
			},
			Spec: mapRemediationRequestToProcessingSpec(&remediationRequest),
		}

		// Set owner reference for cascade deletion
		if err := ctrl.SetControllerReference(&remediationRequest, processing, r.Scheme); err != nil {
			log.Error(err, "Failed to set owner reference")
			return ctrl.Result{}, err
		}

		// Create the RemediationProcessing CRD
		if err := r.Create(ctx, processing); err != nil {
			log.Error(err, "Failed to create RemediationProcessing")
			return ctrl.Result{}, err
		}

		log.Info("Created RemediationProcessing CRD",
			"name", processingName,
			"signal-fingerprint", remediationRequest.Spec.SignalFingerprint)
	}

	return ctrl.Result{}, nil
}
```

---

### **Step 3: Add RBAC Permissions** (5 min)

Add to controller file (above Reconcile function):

```go
// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=remediationprocessing.kubernaut.io,resources=remediationprocessings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=remediationprocessing.kubernaut.io,resources=remediationprocessings/status,verbs=get;update;patch
```

---

### **Step 4: Update SetupWithManager** (5 min)

```go
func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1alpha1.RemediationRequest{}).
		Owns(&remediationprocessingv1alpha1.RemediationProcessing{}). // Watch owned CRDs
		Named("remediation-remediationrequest").
		Complete(r)
}
```

---

## ✅ **Validation**

### **Build and Generate**

```bash
# Generate RBAC manifests
make manifests

# Build controller
go build ./cmd/main.go

# Verify no compilation errors
echo "✅ Build successful"
```

### **Test CRD Creation**

```bash
# Apply CRDs to cluster
make install

# Create test RemediationRequest
kubectl apply -f test/fixtures/remediation-request-sample.yaml

# Verify RemediationProcessing was created
kubectl get remediationprocessings

# Verify all 18 fields are populated
kubectl get remediationprocessing <name> -o yaml | grep -E "signalFingerprint|signalLabels|targetResource"
```

---

## 📊 **Expected Results**

After implementation:

1. ✅ RemediationRequest controller creates RemediationProcessing CRD
2. ✅ All 18 fields copied from RemediationRequest to RemediationProcessing
3. ✅ Owner reference set (cascade deletion works)
4. ✅ RemediationProcessing is self-contained (no external reads needed)
5. ✅ Labels and annotations added for filtering/observability

---

## 🚨 **Common Pitfalls**

### **Pitfall 1: Shallow Copy of Maps/Slices**
**Problem**: Direct assignment of maps/slices creates references, not copies
**Solution**: Use `deepCopyStringMap()` and `deepCopyBytes()` helpers

### **Pitfall 2: Missing Owner Reference**
**Problem**: RemediationProcessing not deleted when RemediationRequest is deleted
**Solution**: Always call `ctrl.SetControllerReference()` before Create()

### **Pitfall 3: Target Resource Extraction Failures**
**Problem**: `TargetResource` is empty if providerData parsing fails
**Solution**: Implement fallback extraction from signal labels

### **Pitfall 4: Nil Map Handling**
**Problem**: Nil maps cause issues when copied
**Solution**: Check for nil before copying, return nil (not empty map)

---

## 📝 **Testing Strategy**

### **Unit Tests** (Required)

Test file: `internal/controller/remediation/remediationrequest_controller_test.go`

```go
func TestMapRemediationRequestToProcessingSpec(t *testing.T) {
	// Test cases:
	// 1. All fields copied correctly
	// 2. Deep copy of maps (modification doesn't affect original)
	// 3. Deep copy of byte slices
	// 4. Target resource extraction from providerData
	// 5. Target resource extraction fallback from labels
	// 6. Deduplication info mapping
}
```

### **Integration Tests** (Recommended)

Test file: `test/integration/remediation/remediationrequest_test.go`

```go
func TestRemediationRequestCreatesProcessing(t *testing.T) {
	// Test cases:
	// 1. RemediationProcessing created when RemediationRequest is created
	// 2. Owner reference set correctly
	// 3. All 18 fields populated
	// 4. Cascade deletion works
}
```

---

## 📈 **Progress Tracking**

**Task 2.2 Subtasks**:
- [ ] Add field mapping function (`mapRemediationRequestToProcessingSpec`)
- [ ] Add helper functions (deep copy, extraction)
- [ ] Update Reconcile loop to create RemediationProcessing
- [ ] Add RBAC permissions
- [ ] Update SetupWithManager
- [ ] Generate manifests (`make manifests`)
- [ ] Write unit tests
- [ ] Write integration tests
- [ ] Manual validation in test cluster

---

## 🔗 **Related Files**

**To Modify**:
- `internal/controller/remediation/remediationrequest_controller.go` - Main implementation
- `internal/controller/remediation/remediationrequest_controller_test.go` - Unit tests

**To Reference**:
- `api/remediation/v1alpha1/remediationrequest_types.go` - Source CRD
- `api/remediationprocessing/v1alpha1/remediationprocessing_types.go` - Target CRD
- `docs/services/crd-controllers/05-remediationorchestrator/data-handling-architecture.md` - Pattern documentation

---

## 📚 **Business Requirements**

**Implements**:
- BR-REM-030 to BR-REM-040: Targeting Data Pattern for child CRDs
- BR-PROC-001: Self-contained RemediationProcessing CRD

**Enables**:
- Phase 1 self-containment pattern
- Performance: No cross-CRD reads
- Reliability: No external dependencies
- Isolation: CRD self-sufficiency

---

**Implementation Ready** - All specifications provided for Task 2.2 execution

