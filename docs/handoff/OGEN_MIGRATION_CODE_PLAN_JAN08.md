# Ogen Migration - Code Refactoring Plan

**Date**: January 8, 2026
**Status**: ✅ **OGEN GENERATED** - Now planning code migration
**Ogen Output**: `pkg/datastorage/ogen-client/oas_*_gen.go` (1.4MB, 19 files)

---

## 🎯 **Ogen Generated Structure - PERFECT!**

### How Ogen Handles oneOf (Tagged Union)

**File**: `pkg/datastorage/ogen-client/oas_schemas_gen.go`

```go
type AuditEventRequestEventData struct {
    Type AuditEventRequestEventDataType // ✅ Discriminator field!

    // All 26 possible payload types as fields
    GatewayAuditPayload                  GatewayAuditPayload
    RemediationOrchestratorAuditPayload  RemediationOrchestratorAuditPayload
    SignalProcessingAuditPayload         SignalProcessingAuditPayload
    AIAnalysisAuditPayload               AIAnalysisAuditPayload
    WorkflowExecutionAuditPayload        WorkflowExecutionAuditPayload
    NotificationAuditPayload             NotificationAuditPayload
    // ... 20 more payload types
    LLMRequestPayload                    LLMRequestPayload    // ✅ For HAPI!
    LLMResponsePayload                   LLMResponsePayload   // ✅ For HAPI!
    LLMToolCallPayload                   LLMToolCallPayload   // ✅ For HAPI!
    WorkflowValidationPayload            WorkflowValidationPayload // ✅ For HAPI!
}

// Helper methods for type checking
func (s AuditEventRequestEventData) IsLLMRequestPayload() bool {
    return s.Type == LLMRequestPayloadAuditEventRequestEventData
}
```

**This is EXACTLY what we need!** No `json.RawMessage`, no marshaling, just properly typed structs!

---

## 📋 **Migration Strategy**

### Phase 1: Update Makefile ✅ (5 min)
### Phase 2: Go Code Migration ⏳ (2-3 hours)
### Phase 3: Python Code Migration ⏳ (1-2 hours)
### Phase 4: Testing & Validation ⏳ (1 hour)
### Phase 5: Cleanup ⏳ (30 min)

---

## 🔧 **Phase 1: Update Makefile** ✅

**File**: `Makefile`

**Change**:
```makefile
.PHONY: generate-datastorage-client
generate-datastorage-client: ogen ## Generate DataStorage OpenAPI client from spec (DD-API-001)
	@echo "📋 Generating DataStorage clients (Go + Python) from api/openapi/data-storage-v1.yaml..."
	@echo ""
	@echo "🔧 [1/2] Generating Go client with ogen..."
	@go generate ./pkg/datastorage/ogen-client/...
	@echo "✅ Go client generated: pkg/datastorage/ogen-client/oas_*_gen.go"
	@echo ""
	@echo "🔧 [2/2] Generating Python client..."
	@rm -rf holmesgpt-api/src/clients/datastorage
	@podman run --rm -v "$(PWD)":/local:z openapitools/openapi-generator-cli:v7.2.0 generate \
		-i /local/api/openapi/data-storage-v1.yaml \
		-g python \
		-o /local/holmesgpt-api/src/clients/datastorage \
		--package-name datastorage \
		--additional-properties=packageVersion=1.0.0
	@echo "✅ Python client generated: holmesgpt-api/src/clients/datastorage/"
	@echo ""
	@echo "✨ Both clients generated successfully!"
	@echo "   Go (ogen):  pkg/datastorage/ogen-client/"
	@echo "   Python:     holmesgpt-api/src/clients/datastorage/"
```

---

## 🔧 **Phase 2: Go Code Migration** (2-3 hours)

### Step 2.1: Update Import Paths (~20 files)

**Find and Replace**:
```bash
# Old import
"github.com/jordigilh/kubernaut/pkg/datastorage/client"
alias: dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"

# New import
"github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
alias: ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
```

**Files to Update**:
- `pkg/*/audit/manager.go` (~8 files)
- `pkg/audit/helpers.go` (1 file)
- `test/integration/*/audit_*.go` (~15 files)

---

### Step 2.2: Refactor `pkg/audit/helpers.go` - MAJOR SIMPLIFICATION

**Current** (with oapi-codegen + json.RawMessage):
```go
package audit

import (
	"encoding/json"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	// ...
)

// SetEventData marshals the data to JSON and assigns to event_data
func SetEventData(e *dsgen.AuditEventRequest, data interface{}) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		// Handle error
		return
	}
	e.EventData = dsgen.AuditEventRequest_EventData{union: jsonBytes}
}

// SetEventDataFromEnvelope marshals envelope to JSON and assigns
func SetEventDataFromEnvelope(e *dsgen.AuditEventRequest, envelope *CommonEnvelope) {
	jsonBytes, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	e.EventData = dsgen.AuditEventRequest_EventData{union: jsonBytes}
}
```

**New** (with ogen + typed unions):
```go
package audit

import (
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	// ...
)

// SetEventData creates appropriate union type from payload
// This is still useful for creating the discriminated union wrapper
func SetEventData(e *ogenclient.AuditEventRequest, eventType string, data interface{}) {
	switch eventType {
	case "gateway.signal.received", "gateway.signal.deduplicated", "gateway.crd.created", "gateway.crd.failed":
		payload, ok := data.(*ogenclient.GatewayAuditPayload)
		if !ok {
			return
		}
		e.EventData = ogenclient.NewGatewayAuditPayloadAuditEventRequestEventData(*payload)

	case "llm_request":
		payload, ok := data.(*ogenclient.LLMRequestPayload)
		if !ok {
			return
		}
		e.EventData = ogenclient.NewLLMRequestPayloadAuditEventRequestEventData(*payload)

	case "llm_response":
		payload, ok := data.(*ogenclient.LLMResponsePayload)
		if !ok {
			return
		}
		e.EventData = ogenclient.NewLLMResponsePayloadAuditEventRequestEventData(*payload)

	// ... 35 more cases for all event types
	}
}

// Or even simpler - services can call constructors directly:
// e.EventData = ogenclient.NewLLMRequestPayloadAuditEventRequestEventData(payload)
```

**Better Approach - Eliminate Helper Entirely**:
```go
// Services call constructor directly:
event.EventData = ogenclient.NewLLMRequestPayloadAuditEventRequestEventData(
	ogenclient.LLMRequestPayload{
		EventID:       uuid.New().String(),
		IncidentID:    incidentID,
		Model:         "claude-3-5-sonnet",
		PromptLength:  len(prompt),
		// ...
	},
)
```

---

### Step 2.3: Update Service Audit Managers (~8 files)

**Example**: `pkg/workflowexecution/audit/manager.go`

**Current**:
```go
import (
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution"
)

func (m *Manager) RecordFailure(...) {
	event := m.createBaseEvent(...)

	payload := &workflowexecution.WorkflowExecutionAuditPayload{
		WorkflowName:   wfe.Spec.WorkflowName,
		FailureReason:  string(reason),
		ErrorDetails:   errorDetails,
		// ...
	}

	audit.SetEventData(event, payload)  // Marshals to json.RawMessage
	m.auditClient.CreateAuditEvent(ctx, event)
}
```

**New**:
```go
import (
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

func (m *Manager) RecordFailure(...) {
	event := m.createBaseEvent(...)

	payload := ogenclient.WorkflowExecutionAuditPayload{
		WorkflowName:   wfe.Spec.WorkflowName,
		FailureReason:  string(reason),
		ErrorDetails:   ogenclient.ErrorDetails{...},  // ✅ Use ogen types!
		// ...
	}

	// Direct constructor - no marshaling!
	event.EventData = ogenclient.NewWorkflowExecutionAuditPayloadAuditEventRequestEventData(payload)
	m.auditClient.CreateAuditEvent(ctx, event)
}
```

**Key Changes**:
1. Import `ogen-client` instead of `client`
2. Delete local `audit_types.go` files - use ogen-generated types
3. Call ogen constructors directly - no helper needed
4. No marshaling, no `json.RawMessage`

---

### Step 2.4: Files Requiring Updates

**Service Audit Managers** (8 files):
1. `pkg/gateway/audit/manager.go`
2. `pkg/remediationorchestrator/audit/manager.go`
3. `pkg/signalprocessing/audit/client.go`
4. `pkg/aianalysis/audit/audit.go`
5. `pkg/workflowexecution/audit/manager.go`
6. `pkg/notification/audit/manager.go`
7. `pkg/webhooks/*_handler.go` (4 files)
8. `pkg/datastorage/audit/workflow_*.go` (2 files)

**Integration Tests** (~15 files):
- `test/integration/*/audit_*.go`

**Helper** (1 file):
- `pkg/audit/helpers.go`

---

## 🔧 **Phase 3: Python Code Migration** (1-2 hours)

### Step 3.1: Refactor `holmesgpt-api/src/audit/events.py`

**Current** (returns dict):
```python
from typing import Dict, Any

def create_llm_request_event(...) -> Dict[str, Any]:  # ❌ Dict!
    event_data_model = LLMRequestEventData(...)

    return {  # ❌ Dict
        "version": "1.0",
        "event_data": event_data_model.model_dump(),  # ❌ Convert to dict
        ...
    }
```

**New** (returns Pydantic model):
```python
from datastorage.models.audit_event_request import AuditEventRequest
from datastorage.models.audit_event_request_event_data import AuditEventRequestEventData
from datastorage.models.llm_request_payload import LLMRequestPayload

def create_llm_request_event(...) -> AuditEventRequest:  # ✅ Typed!
    event_data_payload = LLMRequestPayload(
        event_id=str(uuid.uuid4()),
        incident_id=incident_id,
        model=model,
        prompt_length=len(prompt),
        # ...
    )

    # Wrap in union type (Pydantic discriminator)
    event_data_union = AuditEventRequestEventData(
        actual_instance=event_data_payload
    )

    return AuditEventRequest(  # ✅ Return Pydantic model!
        version="1.0",
        event_category="analysis",
        event_type="llm_request",
        event_timestamp=datetime.now(timezone.utc),
        correlation_id=remediation_id or "",
        event_action="llm_request_sent",
        event_outcome="success",
        actor_type="Service",
        actor_id="holmesgpt-api",
        event_data=event_data_union,  # ✅ Typed union!
    )
```

---

### Step 3.2: Refactor `holmesgpt-api/src/audit/buffered_store.py`

**Current** (dict parameter + manual conversion):
```python
def store_audit(self, event: Dict[str, Any]):  # ❌ Dict parameter
    self._buffer.append(event)

def _write_single_event_with_retry(self, event: Dict[str, Any]) -> bool:
    # Lines 434-435: Manual conversion
    event_data_obj = AuditEventRequestEventData.from_dict(event["event_data"])  # ❌
    event_copy["event_data"] = event_data_obj
    audit_request = AuditEventRequest(**event_copy)

    response = self._audit_api.create_audit_event(audit_event_request=audit_request)
```

**New** (Pydantic model parameter - no conversion):
```python
from datastorage.models.audit_event_request import AuditEventRequest

def store_audit(self, event: AuditEventRequest):  # ✅ Typed parameter!
    self._buffer.append(event)

def _write_single_event_with_retry(self, event: AuditEventRequest) -> bool:
    # No conversion needed - already typed! ✅
    response = self._audit_api.create_audit_event(audit_event_request=event)
    return True
```

**Changes**:
1. Change `List[Dict[str, Any]]` → `List[AuditEventRequest]` for buffer
2. Remove lines 434-435 (no conversion needed)
3. Update type hints throughout

---

### Step 3.3: Files Requiring Updates

**Python Files** (3 files):
1. `holmesgpt-api/src/audit/events.py` - 5 functions to refactor
2. `holmesgpt-api/src/audit/buffered_store.py` - Remove conversion logic
3. `holmesgpt-api/src/models/audit_models.py` - Update imports (minimal change)

---

## 🧪 **Phase 4: Testing & Validation** (1 hour)

### Step 4.1: Compile Go Code
```bash
make build
# Expected: Compilation errors for import paths and type mismatches
# Fix systematically file by file
```

### Step 4.2: Run Go Integration Tests
```bash
# Test DataStorage first (most critical)
make test-integration-datastorage

# Then test services one by one
make test-integration-gateway
make test-integration-remediationorchestrator
# ... etc
```

### Step 4.3: Run Python Tests
```bash
# HAPI unit tests
make test-unit-holmesgpt-api
# Expected: 557/557 passing

# HAPI integration tests
make test-integration-holmesgpt-api
# Expected: 65/65 passing (fixes the 6 failures!)
```

---

## 🗑️ **Phase 5: Cleanup** (30 min)

### Step 5.1: Delete Old Client
```bash
rm -rf pkg/datastorage/client/
```

### Step 5.2: Delete Duplicate Type Files

**Files to Delete** (services were defining their own audit types):
- `pkg/gateway/audit_types.go`
- `pkg/remediationorchestrator/audit_types.go`
- `pkg/signalprocessing/audit_types.go`
- `pkg/aianalysis/audit/event_types.go` (if it exists)
- `pkg/workflowexecution/audit_types.go`
- `pkg/notification/audit/event_types.go` (if it exists)
- `pkg/webhooks/audit_types.go`

**All replaced by**: `pkg/datastorage/ogen-client/oas_schemas_gen.go`

### Step 5.3: Update Documentation
- Update `docs/handoff/OGEN_MIGRATION_PLAN_JAN08.md` with completion status
- Update `OPENAPI_UNSTRUCTURED_DATA_FIX_JAN08.md` with final results
- Create summary handoff document

---

## 📊 **Expected Results**

### Before (oapi-codegen)
```go
// ❌ Manual marshaling required
payload := &WorkflowExecutionAuditPayload{...}
jsonBytes, _ := json.Marshal(payload)
event.EventData = AuditEventRequest_EventData{union: jsonBytes}
```

### After (ogen)
```go
// ✅ Direct typed assignment
payload := ogenclient.WorkflowExecutionAuditPayload{...}
event.EventData = ogenclient.NewWorkflowExecutionAuditPayloadAuditEventRequestEventData(payload)
```

### Python Before
```python
# ❌ Dict conversions
event_dict = create_llm_request_event(...)
event_data_obj = AuditEventRequestEventData.from_dict(event_dict["event_data"])
```

### Python After
```python
# ✅ No conversions
event = create_llm_request_event(...)  # Returns AuditEventRequest
# Use directly - already typed!
```

---

## ✅ **Success Criteria**

- [ ] All Go code uses `pkg/datastorage/ogen-client`
- [ ] No `json.RawMessage` in audit event handling
- [ ] No manual marshaling in `pkg/audit/helpers.go`
- [ ] All service audit managers use ogen types directly
- [ ] Python returns `AuditEventRequest` (Pydantic models)
- [ ] No `Dict[str, Any]` in Python audit code
- [ ] No manual conversion in `buffered_store.py`
- [ ] All Go integration tests pass
- [ ] All Python integration tests pass (65/65 - fixes current 6 failures!)
- [ ] Old `client/` directory deleted
- [ ] Duplicate `audit_types.go` files deleted

---

## 🎯 **Confidence: 95%**

**Why High Confidence**:
- ✅ Ogen already generated successfully
- ✅ Generated types look perfect (tagged unions)
- ✅ Clear migration path for both Go and Python
- ✅ Mechanical refactoring (not complex logic changes)
- ✅ Comprehensive test coverage to validate

**Estimated Total Time**: **5-6 hours**

---

**Ready to proceed with Phase 2 (Go code migration)?**

