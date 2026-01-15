# AIAnalysis Team - DD-SEVERITY-001 Week 4 Implementation Handoff

**Date**: 2026-01-15
**Assigned To**: AIAnalysis Team
**Priority**: P0 (Blocks customer onboarding with custom severity schemes)
**Depends On**: Weeks 1-2 (SignalProcessing) ✅ **COMPLETE**

---

## 🎯 **Executive Summary**

**Your Task**: Update AIAnalysis to use **normalized severity** from `SignalProcessing.Status.Severity` instead of **external severity** from `RemediationRequest.Spec.Severity`.

**Why**: Customers use custom severity schemes (Sev1-4, P0-P4, etc.). SignalProcessing Rego now normalizes these to "critical/warning/info". AIAnalysis must use normalized values for consistent LLM prompts.

**Estimated Effort**: 1-2 hours
**Risk Level**: Low (straightforward field change)

---

## ✅ **What's Already Done (Weeks 1-2)**

### **Week 1: CRD Changes** ✅ **COMPLETE**
- ✅ `RemediationRequest.Spec.Severity` - **Enum removed** (accepts "Sev1", "P0", etc.)
- ✅ `SignalProcessing.Spec.Signal.Severity` - **Enum removed** (stores external value)
- ✅ `SignalProcessing.Status.Severity` - **ADDED** (stores normalized value: "critical", "warning", "info")
- ✅ `SignalProcessing.Status.PolicyHash` - **ADDED** (audit trail for policy version)

### **Week 2: Rego Implementation** ✅ **COMPLETE**
- ✅ `SeverityClassifier` implemented (`pkg/signalprocessing/classifier/severity.go`)
- ✅ Controller integration complete (reconcileClassifying phase)
- ✅ Hot-reload support active (ConfigMap fsnotify)
- ✅ Integration tests passing (92/92 specs)

---

## 📋 **Your Implementation Task**

### **File to Change**: `pkg/remediationorchestrator/creator/aianalysis.go`

**Current Code** (Line 170-183):
```go
return aianalysisv1.SignalContextInput{
    Fingerprint:      rr.Spec.SignalFingerprint,
    Severity:         rr.Spec.Severity,  // ❌ External "Sev1"
    SignalType:       rr.Spec.SignalType,
    Environment:      environment,
    BusinessPriority: priority,
    TargetResource: aianalysisv1.TargetResource{
        Kind:      rr.Spec.TargetResource.Kind,
        Name:      rr.Spec.TargetResource.Name,
        Namespace: rr.Spec.TargetResource.Namespace,
    },
    EnrichmentResults: c.buildEnrichmentResults(sp),
}
```

**Required Change** (Line 172):
```go
return aianalysisv1.SignalContextInput{
    Fingerprint:      rr.Spec.SignalFingerprint,
    Severity:         sp.Status.Severity,  // ✅ Normalized "critical" from Rego
    SignalType:       rr.Spec.SignalType,
    Environment:      environment,
    BusinessPriority: priority,
    TargetResource: aianalysisv1.TargetResource{
        Kind:      rr.Spec.TargetResource.Kind,
        Name:      rr.Spec.TargetResource.Name,
        Namespace: rr.Spec.TargetResource.Namespace,
    },
    EnrichmentResults: c.buildEnrichmentResults(sp),
}
```

**That's it!** One line change: `rr.Spec.Severity` → `sp.Status.Severity`

---

## 🧪 **Testing Your Change**

### **1. Unit Tests** (Recommended)

Create a test in `pkg/remediationorchestrator/creator/aianalysis_test.go`:

```go
func TestAIAnalysisCreator_UseNormalizedSeverity(t *testing.T) {
    // GIVEN: RemediationRequest with external severity "Sev1"
    rr := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{Name: "test-rr", Namespace: "default"},
        Spec: remediationv1.RemediationRequestSpec{
            Severity: "Sev1",  // External severity
            // ... other required fields
        },
    }

    // AND: SignalProcessing with normalized severity "critical"
    sp := &signalprocessingv1.SignalProcessing{
        ObjectMeta: metav1.ObjectMeta{Name: "test-sp", Namespace: "default"},
        Spec: signalprocessingv1.SignalProcessingSpec{
            Signal: signalprocessingv1.SignalData{
                Severity: "Sev1",  // External (same as RR)
            },
        },
        Status: signalprocessingv1.SignalProcessingStatus{
            Severity: "critical",  // ✅ Normalized by Rego
        },
    }

    // WHEN: Building AIAnalysis signal context
    creator := NewAIAnalysisCreator(fakeClient, scheme, metrics)
    signalContext := creator.buildSignalContext(rr, sp)

    // THEN: AIAnalysis receives normalized severity
    assert.Equal(t, "critical", signalContext.Severity,
        "AIAnalysis should receive normalized severity from sp.Status.Severity")
}
```

**Run Test**:
```bash
cd pkg/remediationorchestrator/creator
go test -v -run TestAIAnalysisCreator_UseNormalizedSeverity
```

---

### **2. Integration Tests** (Required)

Run existing RemediationOrchestrator integration tests:

```bash
make test-integration-remediationorchestrator
```

**Expected**: All tests should pass. If any fail, check if they assert on specific severity values.

---

### **3. Manual Verification** (Optional)

Create a test RemediationRequest with custom severity:

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationRequest
metadata:
  name: test-custom-severity
  namespace: default
spec:
  severity: "Sev1"  # Custom severity
  signalFingerprint: "abc123..."
  # ... other required fields
```

**Check**:
1. SignalProcessing CRD is created
2. `kubectl get signalprocessing test-custom-severity -o yaml`
3. Verify `status.severity: "critical"` (normalized)
4. AIAnalysis CRD is created
5. `kubectl get aianalysis ai-test-custom-severity -o yaml`
6. Verify `spec.analysisRequest.signalContext.severity: "critical"` (not "Sev1")

---

## 📊 **Impact Analysis**

### **What Changes**:
- ✅ AIAnalysis LLM prompts receive **consistent** severity values ("critical/warning/info")
- ✅ AI decision-making uses **normalized** severity (not custom "Sev1", "P0", etc.)
- ✅ Cross-customer consistency (all customers map to same 3 values)

### **What Stays the Same**:
- ✅ Notifications **keep external severity** ("Sev1" shown to operators)
- ✅ WorkflowExecution messages **keep external severity** ("Sev1" in failure messages)
- ✅ RemediationRequest schema unchanged (still stores external "Sev1")

### **Rationale** (Approved Decision Q1):
**Q: Why does AIAnalysis use normalized but Notifications use external?**
**A**: Operators configured "Sev1", so notifications should show "Sev1" for familiarity. But AI needs consistent values ("critical") for prompt engineering.

---

## 🚫 **What NOT to Change**

### **DO NOT Change These Files**:
1. ❌ `pkg/remediationorchestrator/creator/notification.go` - Keep `rr.Spec.Severity` (external)
2. ❌ `pkg/remediationorchestrator/handler/workflowexecution.go` - Keep `rr.Spec.Severity` (external)
3. ❌ `api/aianalysis/v1alpha1/aianalysis_types.go` - Keep enum (already accepts "critical/warning/info")

**Why**: These components intentionally use external severity per approved decision Q1.

---

## 🔍 **SignalProcessing.Status.Severity Field Details**

### **CRD Location**: `api/signalprocessing/v1alpha1/signalprocessing_types.go`

**Field Definition** (Lines 187-193):
```go
type SignalProcessingStatus struct {
    // ... other fields ...

    // Severity determination (DD-SEVERITY-001)
    // Normalized severity determined by Rego policy: "critical", "warning", or "info"
    // +kubebuilder:validation:Enum=critical;warning;info
    // +optional
    Severity string `json:"severity,omitempty"`

    // PolicyHash is the SHA256 hash of the Rego policy used
    // +optional
    PolicyHash string `json:"policyHash,omitempty"`
}
```

**Valid Values**: `"critical"`, `"warning"`, `"info"`

**When Set**: During SignalProcessing `Classifying` phase (after environment classification, before business categorization)

**How Determined**: Rego policy evaluates `sp.Spec.Signal.Severity` (external) → returns normalized value

---

## 📚 **Example Rego Policy Mappings**

### **Default Policy** (Shipped with Kubernaut):
```rego
package signalprocessing.severity
import rego.v1

determine_severity := "critical" if {
    lower(input.signal.severity) == "critical"
} else := "warning" if {
    lower(input.signal.severity) == "warning"
} else := "info" if {
    lower(input.signal.severity) == "info"
} else := "critical" if {
    true  # Conservative fallback
}
```

### **Customer Policy** (Sev1-4 scheme):
```rego
package signalprocessing.severity
import rego.v1

determine_severity := "critical" if {
    lower(input.signal.severity) in ["sev1", "sev2"]
} else := "warning" if {
    lower(input.signal.severity) == "sev3"
} else := "info" if {
    lower(input.signal.severity) == "sev4"
} else := "critical" if {
    true  # Fallback
}
```

---

## ⚠️ **Edge Cases to Consider**

### **1. Empty Status.Severity**
**When**: SignalProcessing hasn't reached Classifying phase yet
**Your Code**:
```go
severity := sp.Status.Severity
if severity == "" {
    severity = "critical"  // Defensive fallback (should not happen in production)
}
```

### **2. SignalProcessing is nil**
**When**: RemediationOrchestrator called before SP created (race condition)
**Your Code**: Already handles this - SP is required parameter

### **3. Invalid Severity Value**
**When**: Rego policy returns invalid value (policy bug)
**Your Code**: Trust CRD enum validation (`+kubebuilder:validation:Enum=critical;warning;info`)

---

## 📋 **Checklist for Implementation**

### **Before You Start**:
- [ ] Read DD-SEVERITY-001 document (full context)
- [ ] Verify Weeks 1-2 complete (check `SignalProcessing.Status.Severity` exists)
- [ ] Understand Q1 decision (AA uses normalized, Notifications use external)

### **Implementation**:
- [ ] Change line 172 in `aianalysis.go`: `rr.Spec.Severity` → `sp.Status.Severity`
- [ ] Add defensive check for empty `sp.Status.Severity` (optional, low risk)
- [ ] Update comments to reference DD-SEVERITY-001

### **Testing**:
- [ ] Write unit test for normalized severity usage
- [ ] Run `make test-integration-remediationorchestrator`
- [ ] Verify all tests pass
- [ ] Manual verification with test RemediationRequest (optional)

### **Documentation**:
- [ ] Update `BUSINESS_REQUIREMENTS.md` if needed
- [ ] Add comment explaining DD-SEVERITY-001 refactoring
- [ ] Update any service documentation referencing severity

---

## 🔗 **Related Documentation**

### **Primary References**:
- **[DD-SEVERITY-001](../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md)**: Full design decision
- **[BR-SP-105](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)**: Severity Determination via Rego Policy
- **[BR-GATEWAY-111](../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md)**: Gateway Signal Pass-Through

### **Implementation Status**:
- **[SignalProcessing Integration Tests](../../test/integration/signalprocessing/)**: 100% pass rate (92/92)
- **[E2E Tests](../../test/e2e/signalprocessing/)**: 100% pass rate (27/27)

### **Code References**:
- **SeverityClassifier**: `pkg/signalprocessing/classifier/severity.go`
- **Controller Integration**: `internal/controller/signalprocessing/signalprocessing_controller.go`
- **Your File**: `pkg/remediationorchestrator/creator/aianalysis.go`

---

## 💬 **Questions & Support**

### **Common Questions**:

**Q: What if Gateway hasn't been updated yet (Week 3)?**
**A**: Your change is **independent**. SignalProcessing already normalizes severity. Gateway refactoring just removes hardcoding.

**Q: What if a customer doesn't configure a Rego policy?**
**A**: Default policy ships with Kubernaut (1:1 mapping for "critical/warning/info" + conservative fallback).

**Q: What about audit events - should they have both severities?**
**A**: Yes, but that's a separate task (Decision Q2). Your task is just the AIAnalysis creator change.

**Q: Do I need to update AIAnalysis CRD schema?**
**A**: No, `AIAnalysis.SignalContextInput.Severity` already has the correct enum (`critical;warning;info`).

---

## 📞 **Contact**

**Questions?** Reach out to:
- SignalProcessing Team (Weeks 1-2 implementation)
- Gateway Team (Week 3 coordination if needed)
- Document Author: AI Assistant (Cursor)

---

**Document Status**: ✅ Complete - Ready for Implementation
**Created**: 2026-01-15
**Last Updated**: 2026-01-15
**Priority**: P0
**Estimated Effort**: 1-2 hours
**Risk**: Low
