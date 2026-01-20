# HolmesGPT Go Client Regeneration - BR-HAPI-197 Support

**Date**: January 20, 2026 (Evening)
**Purpose**: Regenerate HolmesGPT Go client to include `needs_human_review` and `HumanReviewReason` enum
**Status**: ✅ COMPLETE

---

## 🎯 **What Was Done**

### **1. Regenerated HolmesGPT Go Client**

**Command**:
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make generate-holmesgpt-client
```

**Result**: ✅ Go client successfully regenerated with BR-HAPI-197 fields

---

## ✅ **Generated Types & Constants**

### **Location**: `pkg/holmesgpt/client/oas_schemas_gen.go`

### **New `IncidentResponse` Fields**:
```go
type IncidentResponse struct {
    // ... existing fields ...

    // ✅ NEW: BR-HAPI-197 fields
    NeedsHumanReview  OptBool                   `json:"needs_human_review"`
    HumanReviewReason OptNilHumanReviewReason  `json:"human_review_reason"`

    // ... other fields ...
}
```

### **New `HumanReviewReason` Enum Type**:
```go
// Package: client
// File: pkg/holmesgpt/client/oas_schemas_gen.go

type HumanReviewReason string

const (
    HumanReviewReasonWorkflowNotFound          HumanReviewReason = "workflow_not_found"
    HumanReviewReasonImageMismatch             HumanReviewReason = "image_mismatch"
    HumanReviewReasonParameterValidationFailed HumanReviewReason = "parameter_validation_failed"
    HumanReviewReasonNoMatchingWorkflows       HumanReviewReason = "no_matching_workflows"
    HumanReviewReasonLowConfidence             HumanReviewReason = "low_confidence"
    HumanReviewReasonLlmParsingError           HumanReviewReason = "llm_parsing_error"
    HumanReviewReasonInvestigationInconclusive HumanReviewReason = "investigation_inconclusive"
)
```

---

## 📦 **Import Path for Tests**

### **Recommended Import** (with alias):
```go
import (
    holmesgpt "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)
```

### **Usage in Tests**:
```go
// ✅ CORRECT: Type-safe enum constants
humanReviewReason := string(holmesgpt.HumanReviewReasonWorkflowNotFound)

// ❌ INCORRECT: Hardcoded strings (typo-prone)
humanReviewReason := "workflow_not_found"
```

---

## 📋 **Test Plan Updates**

### **File**: `docs/testing/BR-HAPI-197/remediationorchestrator_test_plan_v1.0.md`

### **Changes Made**:

#### **1. Updated OpenAPI Dependencies Section**
- ✅ Changed status from "BLOCKER" to "READY"
- ✅ Documented generated types and constants
- ✅ Provided correct import path and usage examples

#### **2. Table-Driven Test Entries Already Use Constants**
```go
DescribeTable("should create NotificationRequest for all human review reasons",
    func(humanReviewReason, expectedMessage string) {
        // ... test logic ...
    },
    Entry("workflow_not_found", string(holmesgpt.HumanReviewReasonWorkflowNotFound), "workflow not found"),
    Entry("no_workflows_matched", string(holmesgpt.HumanReviewReasonNoMatchingWorkflows), "No matching workflows"),
    Entry("low_confidence", string(holmesgpt.HumanReviewReasonLowConfidence), "confidence below threshold"),
    Entry("llm_parsing_error", string(holmesgpt.HumanReviewReasonLlmParsingError), "parse LLM response"),
    Entry("parameter_validation_failed", string(holmesgpt.HumanReviewReasonParameterValidationFailed), "parameter validation"),
    Entry("container_image_mismatch", string(holmesgpt.HumanReviewReasonImageMismatch), "image mismatch"),
)
```

#### **3. Audit Event Structure Updated**
```go
// ✅ CORRECT: Typed OpenAPI struct
payload, ok := auditEvent.EventData.(datastorage.RemediationOrchestratorAuditPayload)
Expect(ok).To(BeTrue(), "event_data should be RemediationOrchestratorAuditPayload")
Expect(payload.HumanReviewReason).To(Equal(string(holmesgpt.HumanReviewReasonWorkflowNotFound)))
Expect(payload.RouteType).To(Equal("human_review"))

// ❌ INCORRECT: Unstructured map (anti-pattern)
Expect(auditEvent.EventData).To(HaveKeyWithValue("human_review_reason", "workflow_not_found"))
```

---

## 🔗 **Source of Truth**

### **Python Models** (Authoritative):
```python
# holmesgpt-api/src/models/incident_models.py
class HumanReviewReason(str, Enum):
    WORKFLOW_NOT_FOUND = "workflow_not_found"
    IMAGE_MISMATCH = "image_mismatch"
    PARAMETER_VALIDATION_FAILED = "parameter_validation_failed"
    NO_MATCHING_WORKFLOWS = "no_matching_workflows"
    LOW_CONFIDENCE = "low_confidence"
    LLM_PARSING_ERROR = "llm_parsing_error"
    INVESTIGATION_INCONCLUSIVE = "investigation_inconclusive"
```

### **OpenAPI Spec** (Intermediate):
```json
// holmesgpt-api/api/openapi.json
"HumanReviewReason": {
  "type": "string",
  "enum": [
    "workflow_not_found",
    "image_mismatch",
    "parameter_validation_failed",
    "no_matching_workflows",
    "low_confidence",
    "llm_parsing_error",
    "investigation_inconclusive"
  ],
  "title": "HumanReviewReason",
  "description": "Structured reason for needs_human_review=true..."
}
```

### **Go Client** (Generated):
```go
// pkg/holmesgpt/client/oas_schemas_gen.go
type HumanReviewReason string

const (
    HumanReviewReasonWorkflowNotFound          HumanReviewReason = "workflow_not_found"
    // ... other constants ...
)
```

---

## ✅ **Benefits of Type-Safe Enum Constants**

| Benefit | Impact |
|---------|--------|
| **Type Safety** | Compile-time error if constant name is misspelled |
| **IDE Autocomplete** | Developers get suggestions for valid values |
| **Refactoring Safety** | Rename refactorings update all usages automatically |
| **Documentation** | Constants are self-documenting with comments from OpenAPI spec |
| **Consistency** | Guaranteed alignment between Python models, OpenAPI spec, and Go client |

---

## 🔄 **Regeneration Process**

### **When to Regenerate**:
- Python `incident_models.py` changes (new enum values, field changes)
- OpenAPI spec updates
- After pulling changes from HolmesGPT-API team

### **How to Regenerate**:
```bash
# Step 1: Regenerate OpenAPI spec from Python models (if Python models changed)
cd holmesgpt-api
make generate  # Updates api/openapi.json

# Step 2: Regenerate Go client from OpenAPI spec
cd ../kubernaut
make generate-holmesgpt-client  # Updates pkg/holmesgpt/client/*.go
```

### **Verification**:
```bash
# Check that HumanReviewReason enum exists
grep "type HumanReviewReason string" pkg/holmesgpt/client/oas_schemas_gen.go

# Check that IncidentResponse has new fields
grep -A 5 "NeedsHumanReview" pkg/holmesgpt/client/oas_schemas_gen.go
```

---

## 📊 **Impact Summary**

### **Files Generated**:
- `pkg/holmesgpt/client/oas_schemas_gen.go` (✅ Updated)
- `pkg/holmesgpt/client/oas_json_gen.go` (✅ Updated)
- `pkg/holmesgpt/client/oas_validators_gen.go` (✅ Updated)
- ... (18 total generated files)

### **Test Plans Updated**:
- `docs/testing/BR-HAPI-197/remediationorchestrator_test_plan_v1.0.md` (✅ Updated)

### **Next Steps**:
1. ✅ Go client regenerated with BR-HAPI-197 fields
2. ✅ Test plan updated to use type-safe constants
3. ⏭️ Implement BR-HAPI-197 unit tests using constants
4. ⏭️ Implement BR-HAPI-197 integration tests
5. ⏭️ Implement BR-HAPI-197 E2E tests

---

**Status**: ✅ COMPLETE - Tests can now use type-safe `holmesgpt.HumanReviewReason` constants
**Next**: Proceed with TDD implementation of BR-HAPI-197 test plan
