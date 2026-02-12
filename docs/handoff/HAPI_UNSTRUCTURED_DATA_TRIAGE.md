# HAPI Unstructured Data Triage - Dict[str, Any] Analysis

**Date**: December 17, 2025
**Service**: HolmesGPT API (HAPI)
**Analysis**: Identify `Dict[str, Any]` usage where Pydantic models exist
**Impact**: Type safety, validation, IDE autocomplete, API documentation

---

## 🎯 **Executive Summary**

**Total Findings**: **47 instances** where `Dict[str, Any]` is used when structured Pydantic models already exist

| Category | Violations | Severity | Fix Effort |
|----------|-----------|----------|------------|
| **DetectedLabels** | 8 instances | 🔴 HIGH | 1-2 hours |
| **EnrichmentResults** | 2 instances | 🔴 HIGH | 30 min |
| **Audit event_data** | 7 instances | 🟡 MEDIUM | 2-3 hours |
| **Config/app_config** | 6 instances | 🟢 LOW | 1 hour |
| **Auto-generated OpenAPI** | 24 instances | ⚪ IGNORE | N/A |

**Priority**: Fix **DetectedLabels** and **EnrichmentResults** first (business-critical validation)

---

## 🔴 **Category 1: DetectedLabels - HIGH PRIORITY**

### **Problem**
`DetectedLabels` Pydantic model exists (`src/models/incident_models.py:79`) but is used as `Dict[str, Any]` in 8 locations, bypassing validation and type safety.

### **Impact**
- ❌ No validation of `failedDetections` field (DD-WORKFLOW-001 v2.1)
- ❌ Missing field name validation (gitOpsManaged, pdbProtected, etc.)
- ❌ No IDE autocomplete for developers
- ❌ Runtime errors if incorrect structure passed

### **Existing Model**

```python
# src/models/incident_models.py
class DetectedLabels(BaseModel):
    """Auto-detected cluster characteristics from SignalProcessing."""
    failedDetections: List[str] = Field(default_factory=list)
    gitOpsManaged: bool = Field(default=False)
    gitOpsTool: str = Field(default="")
    pdbProtected: bool = Field(default=False)
    hpaEnabled: bool = Field(default=False)
    stateful: bool = Field(default=False)
    helmManaged: bool = Field(default=False)
    networkIsolated: bool = Field(default=False)
    serviceMesh: str = Field(default="")
```

### **Violations Found**

#### **1. src/extensions/recovery/llm_integration.py**

**Line 59**:
```python
# ❌ CURRENT (UNSTRUCTURED)
def _build_recovery_investigation_prompt(
    request_data: Dict[str, Any],
    config: Optional[HolmesGPTConfig] = None,
    app_config: Dict[str, Any] = None,
    source_resource: Optional[Dict[str, str]] = None,
    detected_labels: Optional[Dict[str, Any]] = None,  # ❌ VIOLATION
) -> str:
```

**RECOMMENDED FIX**:
```python
# ✅ FIXED (STRUCTURED)
from src.models.incident_models import DetectedLabels

def _build_recovery_investigation_prompt(
    request_data: Dict[str, Any],
    config: Optional[HolmesGPTConfig] = None,
    app_config: Dict[str, Any] = None,
    source_resource: Optional[Dict[str, str]] = None,
    detected_labels: Optional[DetectedLabels] = None,  # ✅ STRUCTURED
) -> str:
    # Convert to dict only when needed for templates
    detected_labels_dict = detected_labels.model_dump() if detected_labels else {}
```

---

#### **2. src/extensions/recovery/prompt_builder.py**

**Lines 143, 208**:
```python
# ❌ VIOLATIONS
def _build_cluster_context_section(detected_labels: Dict[str, Any]) -> str:  # Line 143
def _build_mcp_filter_instructions(detected_labels: Dict[str, Any]) -> str:  # Line 208
```

**RECOMMENDED FIX**:
```python
# ✅ FIXED
from src.models.incident_models import DetectedLabels

def _build_cluster_context_section(detected_labels: DetectedLabels) -> str:
    """Convert DetectedLabels to natural language for LLM context."""
    # Access fields with type safety
    if detected_labels.gitOpsManaged:
        context += f"- GitOps: Managed by {detected_labels.gitOpsTool or 'unknown tool'}\n"
    # ...

def _build_mcp_filter_instructions(detected_labels: DetectedLabels) -> str:
    """Build MCP workflow search filter instructions."""
    filters = {}

    # Type-safe field access with IDE autocomplete
    if detected_labels.gitOpsManaged and "gitOpsManaged" not in detected_labels.failedDetections:
        filters["gitOpsManaged"] = True
    # ...
```

---

#### **3. src/extensions/incident/prompt_builder.py**

**Lines 40, 105**:
```python
# ❌ VIOLATIONS
def build_cluster_context_section(detected_labels: Dict[str, Any]) -> str:  # Line 40
def build_mcp_filter_instructions(detected_labels: Dict[str, Any]) -> str:  # Line 105
```

**RECOMMENDED FIX**: Same pattern as recovery/prompt_builder.py above.

---

#### **4. src/toolsets/workflow_catalog.py**

**Lines 103, 324, 1041**:
```python
# ❌ VIOLATIONS
def strip_failed_detections(detected_labels: Dict[str, Any]) -> Dict[str, Any]:  # Line 103

class WorkflowCatalogToolset(ToolsetBase):
    def get_tools(
        self,
        query: str,
        detected_labels: Optional[Dict[str, Any]] = None,  # Line 324
        # ...
    ):

    def search_workflows(
        self,
        query: str,
        detected_labels: Optional[Dict[str, Any]] = None,  # Line 1041
        # ...
    ):
```

**RECOMMENDED FIX**:
```python
# ✅ FIXED
from src.models.incident_models import DetectedLabels

def strip_failed_detections(detected_labels: DetectedLabels) -> DetectedLabels:
    """Strip fields where detection failed from DetectedLabels."""
    failed = detected_labels.failedDetections  # Type-safe access
    cleaned_dict = detected_labels.model_dump(exclude_none=True)

    # Remove failed fields
    for field in failed:
        cleaned_dict.pop(field, None)

    # Return as DetectedLabels (not dict)
    return DetectedLabels(**cleaned_dict)


class WorkflowCatalogToolset(ToolsetBase):
    def get_tools(
        self,
        query: str,
        detected_labels: Optional[DetectedLabels] = None,  # ✅ STRUCTURED
        # ...
    ):
        # Convert to dict only for DS API client
        if detected_labels:
            filters = WorkflowSearchFilters(
                detected_labels=detected_labels  # Direct pass-through
            )
```

---

#### **5. src/extensions/llm_config.py**

**Line 285**:
```python
# ❌ VIOLATION
def create_holmes_investigation_request(
    # ...
    detected_labels: Optional[Dict[str, Any]] = None,  # Line 285
):
```

**RECOMMENDED FIX**:
```python
# ✅ FIXED
from src.models.incident_models import DetectedLabels

def create_holmes_investigation_request(
    # ...
    detected_labels: Optional[DetectedLabels] = None,
):
```

---

### **DetectedLabels Triage Summary**

| File | Lines | Function/Class | Fix Effort |
|------|-------|----------------|------------|
| `recovery/llm_integration.py` | 59 | `_build_recovery_investigation_prompt` | 15 min |
| `recovery/prompt_builder.py` | 143, 208 | 2 functions | 20 min |
| `incident/prompt_builder.py` | 40, 105 | 2 functions | 20 min |
| `toolsets/workflow_catalog.py` | 103, 324, 1041 | 3 functions | 30 min |
| `extensions/llm_config.py` | 285 | `create_holmes_investigation_request` | 10 min |

**Total DetectedLabels Fixes**: 8 instances, **~2 hours**

---

## 🔴 **Category 2: EnrichmentResults - HIGH PRIORITY**

### **Problem**
`EnrichmentResults` Pydantic model exists (`src/models/incident_models.py:133`) but is used as `Dict[str, Any]` in 2 locations.

### **Existing Model**

```python
# src/models/incident_models.py
class EnrichmentResults(BaseModel):
    """Enrichment context from SignalProcessing."""
    detectedLabels: Optional[DetectedLabels] = Field(None)
    detectedLabelsScope: str = Field(default="namespace")
    kubernetesContext: Optional[Dict[str, Any]] = Field(None)
    enrichmentQuality: float = Field(default=0.0, ge=0.0, le=1.0)
```

### **Violations Found**

#### **1. src/models/recovery_models.py**

**Line 145**:
```python
# ❌ VIOLATION
class RecoveryRequest(BaseModel):
    enrichment_results: Optional[Dict[str, Any]] = Field(
        None,
        description="Enriched context including DetectedLabels for workflow filtering"
    )
```

**RECOMMENDED FIX**:
```python
# ✅ FIXED
from src.models.incident_models import EnrichmentResults

class RecoveryRequest(BaseModel):
    enrichment_results: Optional[EnrichmentResults] = Field(
        None,
        description="Enriched context including DetectedLabels for workflow filtering"
    )
```

**Impact**: This is in the API request model! Users get validation errors immediately if they send incorrect structure.

---

### **EnrichmentResults Triage Summary**

| File | Line | Model/Field | Fix Effort |
|------|------|-------------|------------|
| `models/recovery_models.py` | 145 | `RecoveryRequest.enrichment_results` | 30 min |

**Total EnrichmentResults Fixes**: 1 instance (1 more found in response models), **~30 min**

---

## 🟡 **Category 3: Audit event_data - MEDIUM PRIORITY**

### **Problem**
Audit events use `Dict[str, Any]` for `event_data` field. While flexible, this bypasses validation for ADR-034 compliance.

### **Current Usage**

```python
# src/audit/events.py
def create_llm_request_event(...) -> Dict[str, Any]:
    event_data = {
        "event_id": str(uuid.uuid4()),
        "incident_id": incident_id,
        "model": model,
        "prompt_length": len(prompt),
        "prompt_preview": prompt[:500],
        "toolsets_enabled": toolsets_enabled,
        "mcp_servers": mcp_servers or [],
    }
    # Returns Dict[str, Any]
```

### **Proposed Solution**

Create Pydantic models for each audit event type's `event_data`:

```python
# NEW FILE: src/models/audit_models.py
from pydantic import BaseModel, Field
from typing import List, Optional

class LLMRequestEventData(BaseModel):
    """event_data structure for llm_request audit events (ADR-034)."""
    event_id: str
    incident_id: str
    model: str
    prompt_length: int
    prompt_preview: str
    toolsets_enabled: List[str]
    mcp_servers: List[str]

class LLMResponseEventData(BaseModel):
    """event_data structure for llm_response audit events (ADR-034)."""
    event_id: str
    incident_id: str
    has_analysis: bool
    analysis_length: int
    analysis_preview: str
    tool_call_count: int

class LLMToolCallEventData(BaseModel):
    """event_data structure for llm_tool_call audit events (ADR-034)."""
    event_id: str
    incident_id: str
    tool_call_index: int
    tool_name: str
    tool_arguments: dict  # Still flexible for different tools
    tool_result: any  # Still flexible for different tools

class WorkflowValidationEventData(BaseModel):
    """event_data structure for workflow_validation_attempt audit events (ADR-034)."""
    event_id: str
    incident_id: str
    attempt: int
    max_attempts: int
    is_valid: bool
    errors: List[str]
    workflow_id: str
    human_review_reason: str
    is_final_attempt: bool


# Update src/audit/events.py factory functions
def create_llm_request_event(...) -> Dict[str, Any]:
    event_data = LLMRequestEventData(
        event_id=str(uuid.uuid4()),
        incident_id=incident_id,
        model=model,
        prompt_length=len(prompt),
        prompt_preview=prompt[:500] + "..." if len(prompt) > 500 else prompt,
        toolsets_enabled=toolsets_enabled,
        mcp_servers=mcp_servers or []
    )

    return _create_adr034_event(
        event_type="llm_request",
        operation="llm_request_sent",
        outcome="success",
        correlation_id=remediation_id or "",
        event_data=event_data.model_dump()  # Convert to dict for JSON serialization
    )
```

### **Benefits**
- ✅ Validate event_data structure at creation time
- ✅ Catch schema violations before sending to DS service
- ✅ IDE autocomplete for event_data fields
- ✅ Self-documenting ADR-034 compliance

### **Audit event_data Triage Summary**

| Event Type | Factory Function | Current Type | Proposed Model |
|-----------|------------------|--------------|----------------|
| `aiagent.llm.request` | `create_llm_request_event` | `Dict[str, Any]` | `LLMRequestEventData` |
| `aiagent.llm.response` | `create_llm_response_event` | `Dict[str, Any]` | `LLMResponseEventData` |
| `aiagent.llm.tool_call` | `create_tool_call_event` | `Dict[str, Any]` | `LLMToolCallEventData` |
| `aiagent.workflow.validation_attempt` | `create_validation_attempt_event` | `Dict[str, Any]` | `WorkflowValidationEventData` |

**Total Audit Fixes**: Create 4 Pydantic models + update 4 factory functions, **~2-3 hours**

---

## 🟢 **Category 4: Config/app_config - LOW PRIORITY**

### **Problem**
Application configuration passed as `Dict[str, Any]` instead of structured config objects.

### **Violations Found**

| File | Line | Function | Usage |
|------|------|----------|-------|
| `recovery/llm_integration.py` | 56 | `_build_recovery_investigation_prompt` | `app_config: Dict[str, Any]` |
| `recovery/llm_integration.py` | 159 | `analyze_recovery` | `app_config: Optional[Dict[str, Any]]` |
| `incident/llm_integration.py` | 132 | `create_data_storage_client` | `app_config: Optional[Dict[str, Any]]` |
| `incident/llm_integration.py` | 164 | `analyze_incident` | `app_config: Optional[Dict[str, Any]]` |
| `middleware/auth.py` | 80 | `TokenReviewMiddleware.__init__` | `config: Dict[str, Any]` |
| `main.py` | 103 | `load_config` | `-> Dict[str, Any]` |

### **Recommendation**

**DEFER** - Config is loaded from YAML and is intentionally flexible. Creating a full Pydantic config model would be extensive work with limited benefit.

**Alternative**: Use `TypedDict` for partial type hints without validation:

```python
from typing import TypedDict, Optional

class AppConfig(TypedDict, total=False):
    """Type hints for app config (no validation)."""
    llm: dict
    data_storage_url: str
    auth_enabled: bool
    dev_mode: bool
    # ... etc

def load_config() -> AppConfig:
    # ...
```

---

## ⚪ **Category 5: Auto-generated OpenAPI Client - IGNORE**

### **Files to Ignore**

All files in `src/clients/datastorage/models/` are auto-generated from OpenAPI spec:
- `audit_event_request.py`
- `audit_event.py`
- `workflow_search_result.py`
- `remediation_workflow.py`
- etc.

**DO NOT MODIFY** - These are regenerated from OpenAPI spec on every client update.

**Note**: Some use `Dict[str, Any]` correctly (e.g., `detected_labels` field in search results) because they're JSON pass-through from DS service.

---

## 📊 **Priority Roadmap**

### **Phase 1: Critical Business Logic** (2.5 hours)

1. ✅ Fix DetectedLabels usage (8 instances) - **2 hours**
   - Enables proper validation per DD-WORKFLOW-001 v2.1
   - Prevents runtime errors from invalid field names

2. ✅ Fix EnrichmentResults usage (1-2 instances) - **30 min**
   - API request model validation critical for user errors

### **Phase 2: Audit Trail Compliance** (2-3 hours)

3. ✅ Create audit event_data Pydantic models - **2-3 hours**
   - Ensures ADR-034 compliance at compile-time
   - Catches schema violations before DS service call

### **Phase 3: Config Typing** (DEFER)

4. ⚠️ DEFER: Config object structuring
   - Limited benefit, extensive refactor
   - Consider TypedDict for type hints only

---

## 🛠️ **Implementation Guide**

### **Step 1: Fix DetectedLabels**

```bash
# Files to update (in order):
1. src/extensions/incident/prompt_builder.py
2. src/extensions/recovery/prompt_builder.py
3. src/toolsets/workflow_catalog.py
4. src/extensions/llm_config.py
5. src/extensions/incident/llm_integration.py
6. src/extensions/recovery/llm_integration.py

# Pattern:
- Import: from src.models.incident_models import DetectedLabels
- Change param: detected_labels: Optional[DetectedLabels] = None
- Access fields: detected_labels.gitOpsManaged (not dict access)
- Convert to dict when needed: detected_labels.model_dump()
```

### **Step 2: Fix EnrichmentResults**

```bash
# Files to update:
1. src/models/recovery_models.py (line 145)
2. Check response models for same issue

# Pattern:
- Import: from src.models.incident_models import EnrichmentResults
- Change field: enrichment_results: Optional[EnrichmentResults] = None
```

### **Step 3: Create Audit Models (Optional)**

```bash
# Create new file:
src/models/audit_models.py

# Update factory functions:
src/audit/events.py

# Pattern:
- Create Pydantic model for each event type
- Factory returns Dict but constructs via Pydantic
- Validation happens at model instantiation
```

---

## 📈 **Expected Benefits**

| Benefit | Before | After |
|---------|--------|-------|
| **Type Safety** | ❌ Runtime errors | ✅ Compile-time errors |
| **IDE Support** | ❌ No autocomplete | ✅ Full autocomplete |
| **Validation** | ⚠️ Manual checks | ✅ Automatic Pydantic |
| **Documentation** | ⚠️ Comments only | ✅ Self-documenting schemas |
| **Error Messages** | ❌ "KeyError: 'field'" | ✅ "Missing required field: field" |
| **Test Coverage** | ⚠️ Need explicit tests | ✅ Pydantic validates automatically |

---

## 🎯 **Success Criteria**

**Phase 1 Complete** when:
- [ ] All 8 DetectedLabels violations fixed
- [ ] All EnrichmentResults violations fixed
- [ ] Unit tests pass with new types
- [ ] IDE shows autocomplete for DetectedLabels fields
- [ ] No mypy/pyright type errors

**Phase 2 Complete** when:
- [ ] 4 audit event_data Pydantic models created
- [ ] Factory functions use new models
- [ ] Integration tests validate ADR-034 compliance
- [ ] Audit events rejected by DS if invalid structure

---

**Prepared by**: AI Assistant
**Date**: December 17, 2025
**Next Action**: Review and approve Phase 1 implementation plan
**Estimated Effort**: Phase 1 = 2.5 hours, Phase 2 = 2-3 hours

