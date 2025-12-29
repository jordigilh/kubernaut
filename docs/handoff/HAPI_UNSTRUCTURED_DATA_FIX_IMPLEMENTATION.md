# HAPI Unstructured Data Fix - Implementation Complete (Phase 1 Started)

**Date**: December 17, 2025
**Status**: Phase 1 In Progress (1/8 files completed)
**Total Effort**: Estimated 6-8 hours for all 3 phases

---

## ✅ **Phase 1 Progress: DetectedLabels Fixes**

### **Completed**

| File | Status | Functions Fixed | Notes |
|------|--------|----------------|-------|
| `incident/prompt_builder.py` | ✅ **COMPLETE** | 2/2 functions | Type hints + attribute access updated |

**Changes Made**:
1. ✅ Added import: `from src.models.incident_models import DetectedLabels`
2. ✅ Updated `build_cluster_context_section(detected_labels: DetectedLabels)`
3. ✅ Updated `build_mcp_filter_instructions(detected_labels: DetectedLabels)`
4. ✅ Changed dict access (`.get()`) to attribute access (`.fieldName`)
5. ✅ Updated `create_incident_investigation_prompt()` to construct `DetectedLabels` objects

### **Remaining Phase 1 Tasks**

| File | Functions | Priority | Estimated Time |
|------|-----------|----------|----------------|
| `recovery/prompt_builder.py` | 2 functions | HIGH | 20 min |
| `toolsets/workflow_catalog.py` | 3 functions | HIGH | 30 min |
| `extensions/llm_config.py` | 1 function | HIGH | 10 min |
| `incident/llm_integration.py` | Pass-through | HIGH | 10 min |
| `recovery/llm_integration.py` | Pass-through | HIGH | 10 min |
| `recovery_models.py` (EnrichmentResults) | 1 field | HIGH | 30 min |

**Total Remaining Phase 1**: ~2 hours

---

## 📋 **Implementation Pattern (Established)**

### **Pattern for DetectedLabels**

```python
# STEP 1: Add import
from src.models.incident_models import DetectedLabels

# STEP 2: Update function signature
# BEFORE:
def function_name(detected_labels: Dict[str, Any]) -> str:

# AFTER:
def function_name(detected_labels: DetectedLabels) -> str:

# STEP 3: Update field access
# BEFORE:
failed_fields = set(detected_labels.get('failedDetections', []))
if detected_labels.get("gitOpsManaged"):
    tool = detected_labels.get("gitOpsTool", "unknown")

# AFTER:
failed_fields = set(detected_labels.failedDetections)
if detected_labels.gitOpsManaged:
    tool = detected_labels.gitOpsTool or "unknown"

# STEP 4: For dynamic field access (loops), use getattr
# BEFORE:
value = detected_labels.get(label_field)

# AFTER:
value = getattr(detected_labels, label_field, None)

# STEP 5: Convert dict to DetectedLabels when extracting
# BEFORE:
detected_labels = enrichment_results.get('detectedLabels', {})

# AFTER:
dl = enrichment_results.get('detectedLabels', {})
detected_labels = DetectedLabels(**dl) if isinstance(dl, dict) else dl
```

---

## 🎯 **Remaining Implementation Plan**

### **Phase 1: DetectedLabels (1.5 hours remaining)**

#### **File 2: src/extensions/recovery/prompt_builder.py**

```python
# Line 143: _build_cluster_context_section
# Line 208: _build_mcp_filter_instructions

# APPLY SAME PATTERN AS incident/prompt_builder.py
# 1. Add import
# 2. Update type hints (2 functions)
# 3. Change .get() to attribute access
# 4. Update prompt function that calls these
```

#### **File 3: src/toolsets/workflow_catalog.py**

```python
# Line 103: strip_failed_detections
def strip_failed_detections(detected_labels: DetectedLabels) -> DetectedLabels:
    """Strip fields where detection failed."""
    failed = detected_labels.failedDetections
    cleaned_dict = detected_labels.model_dump(exclude_none=True)
    for field in failed:
        cleaned_dict.pop(field, None)
    return DetectedLabels(**cleaned_dict)

# Line 324: WorkflowCatalogToolset.get_tools
# Line 1041: WorkflowCatalogToolset.search_workflows
# Update type hints + pass DetectedLabels to DS API client
```

#### **File 4: src/extensions/llm_config.py**

```python
# Line 285: create_holmes_investigation_request
from src.models.incident_models import DetectedLabels

def create_holmes_investigation_request(
    # ...
    detected_labels: Optional[DetectedLabels] = None,
):
    # Function already handles conversion internally
```

#### **File 5: src/extensions/incident/llm_integration.py**

```python
# This file extracts detected_labels and passes to prompt_builder
# No changes needed - prompt_builder now handles DetectedLabels objects
# Verify extraction logic matches pattern from incident/prompt_builder.py
```

#### **File 6: src/extensions/recovery/llm_integration.py**

```python
# Similar to File 5 - verify extraction/pass-through logic
```

#### **File 7: src/models/recovery_models.py (EnrichmentResults)**

```python
# Line 145: RecoveryRequest.enrichment_results
from src.models.incident_models import EnrichmentResults

class RecoveryRequest(BaseModel):
    enrichment_results: Optional[EnrichmentResults] = Field(
        None,
        description="Enriched context including DetectedLabels for workflow filtering"
    )
```

---

### **Phase 2: Audit Models (2-3 hours)**

#### **File 8: Create src/models/audit_models.py**

```python
"""
Audit Event Data Models (ADR-034 Compliance)

Business Requirement: BR-AUDIT-005 (Audit Trail)
Design Decision: ADR-034 (Unified Audit Table Design)

This module provides Pydantic models for audit event_data payloads,
ensuring compile-time ADR-034 compliance.
"""

from pydantic import BaseModel, Field
from typing import List, Any

class LLMRequestEventData(BaseModel):
    """event_data structure for llm_request audit events (ADR-034)."""
    event_id: str = Field(..., description="Unique event identifier")
    incident_id: str = Field(..., description="Incident identifier")
    model: str = Field(..., description="LLM model name (e.g., 'claude-3-5-sonnet')")
    prompt_length: int = Field(..., ge=0, description="Length of prompt in characters")
    prompt_preview: str = Field(..., description="First 500 chars of prompt")
    toolsets_enabled: List[str] = Field(default_factory=list, description="List of enabled toolsets")
    mcp_servers: List[str] = Field(default_factory=list, description="List of MCP servers")

class LLMResponseEventData(BaseModel):
    """event_data structure for llm_response audit events (ADR-034)."""
    event_id: str = Field(..., description="Unique event identifier")
    incident_id: str = Field(..., description="Incident identifier")
    has_analysis: bool = Field(..., description="Whether LLM returned analysis")
    analysis_length: int = Field(..., ge=0, description="Length of analysis text")
    analysis_preview: str = Field(..., description="First 500 chars of analysis")
    tool_call_count: int = Field(..., ge=0, description="Number of tool calls made")

class LLMToolCallEventData(BaseModel):
    """event_data structure for llm_tool_call audit events (ADR-034)."""
    event_id: str = Field(..., description="Unique event identifier")
    incident_id: str = Field(..., description="Incident identifier")
    tool_call_index: int = Field(..., ge=0, description="Index of tool call in sequence")
    tool_name: str = Field(..., description="Name of tool invoked")
    tool_arguments: dict = Field(default_factory=dict, description="Arguments passed to tool")
    tool_result: Any = Field(None, description="Result returned by tool")

class WorkflowValidationEventData(BaseModel):
    """event_data structure for workflow_validation_attempt audit events (ADR-034)."""
    event_id: str = Field(..., description="Unique event identifier")
    incident_id: str = Field(..., description="Incident identifier")
    attempt: int = Field(..., ge=1, description="Current attempt number (1-indexed)")
    max_attempts: int = Field(..., ge=1, description="Maximum allowed attempts")
    is_valid: bool = Field(..., description="Whether validation passed")
    errors: List[str] = Field(default_factory=list, description="List of validation error messages")
    workflow_id: str = Field(default="", description="Workflow ID being validated")
    human_review_reason: str = Field(default="", description="Reason code if needs_human_review")
    is_final_attempt: bool = Field(..., description="Whether this is the final attempt")
```

#### **File 9: Update src/audit/events.py**

```python
from src.models.audit_models import (
    LLMRequestEventData,
    LLMResponseEventData,
    LLMToolCallEventData,
    WorkflowValidationEventData
)

def create_llm_request_event(...) -> Dict[str, Any]:
    # Construct Pydantic model for validation
    event_data_model = LLMRequestEventData(
        event_id=str(uuid.uuid4()),
        incident_id=incident_id,
        model=model,
        prompt_length=len(prompt),
        prompt_preview=prompt[:500] + "..." if len(prompt) > 500 else prompt,
        toolsets_enabled=toolsets_enabled,
        mcp_servers=mcp_servers or []
    )

    # Convert to dict for JSON serialization
    return _create_adr034_event(
        event_type="llm_request",
        operation="llm_request_sent",
        outcome="success",
        correlation_id=remediation_id or "",
        event_data=event_data_model.model_dump()
    )

# Repeat for other 3 event factory functions
```

---

### **Phase 3: Config TypedDict (1 hour)**

#### **File 10: Create src/models/config_models.py**

```python
"""
Application Configuration Type Hints

This module provides TypedDict for config objects to improve type safety
without requiring full Pydantic validation (config is loaded from YAML).
"""

from typing import TypedDict, Optional, Dict, Any

class LLMConfig(TypedDict, total=False):
    """LLM configuration section."""
    provider: str
    model: str
    endpoint: Optional[str]
    temperature: float
    max_tokens: int

class AppConfig(TypedDict, total=False):
    """Application configuration type hints (no validation)."""
    service_name: str
    version: str
    llm: LLMConfig
    data_storage_url: str
    auth_enabled: bool
    dev_mode: bool
    log_level: str
    toolsets: Dict[str, Any]
    mcp_servers: Dict[str, Any]
```

#### **Files 11-16: Update config usage**

```python
# src/main.py
from src.models.config_models import AppConfig

def load_config() -> AppConfig:
    # ...

# src/middleware/auth.py
from src.models.config_models import AppConfig

class TokenReviewMiddleware:
    def __init__(self, app, config: AppConfig):
        # ...

# src/extensions/recovery/llm_integration.py
from src.models.config_models import AppConfig

def _build_recovery_investigation_prompt(
    request_data: Dict[str, Any],
    config: Optional[HolmesGPTConfig] = None,
    app_config: Optional[AppConfig] = None,
    # ...
):

# Similar updates for:
# - src/extensions/incident/llm_integration.py
# - src/extensions/recovery/llm_integration.py (analyze_recovery)
# - src/extensions/incident/llm_integration.py (create_data_storage_client, analyze_incident)
```

---

## 🧪 **Testing Strategy**

### **Unit Tests**

```python
# tests/unit/test_detected_labels_structured.py
from src.models.incident_models import DetectedLabels
from src.extensions.incident.prompt_builder import build_cluster_context_section

def test_build_cluster_context_with_pydantic_model():
    """Test that functions accept DetectedLabels Pydantic model."""
    detected_labels = DetectedLabels(
        gitOpsManaged=True,
        gitOpsTool="argocd",
        pdbProtected=True,
        failedDetections=[]
    )

    context = build_cluster_context_section(detected_labels)

    assert "GitOps" in context
    assert "argocd" in context
    assert "PodDisruptionBudget" in context

def test_build_cluster_context_honors_failed_detections():
    """Test that failedDetections are excluded."""
    detected_labels = DetectedLabels(
        gitOpsManaged=False,  # Value is false
        pdbProtected=True,    # Value is true
        failedDetections=["pdbProtected"]  # But detection failed!
    )

    context = build_cluster_context_section(detected_labels)

    # pdbProtected should NOT appear (detection failed)
    assert "PodDisruptionBudget" not in context
```

### **Integration Tests**

```python
# tests/integration/test_audit_event_validation.py
from src.models.audit_models import LLMRequestEventData
from src.audit.events import create_llm_request_event

def test_create_llm_request_event_validates_schema():
    """Test that audit events validate at creation time."""
    # This should succeed
    event = create_llm_request_event(
        incident_id="inc-123",
        remediation_id="rem-456",
        model="claude-3-5-sonnet",
        prompt="Test",
        toolsets_enabled=["kubernetes/core"]
    )

    assert event["event_type"] == "llm_request"
    assert "event_data" in event

    # This should raise ValidationError (missing required fields)
    with pytest.raises(ValidationError):
        LLMRequestEventData(
            event_id="test",
            # Missing required fields!
        )
```

---

## 📊 **Progress Tracker**

| Phase | Task | Status | Time |
|-------|------|--------|------|
| **Phase 1** | DetectedLabels | 🟡 1/8 files | 2.5 hrs |
| ├─ | incident/prompt_builder.py | ✅ DONE | 20 min |
| ├─ | recovery/prompt_builder.py | ⏳ TODO | 20 min |
| ├─ | toolsets/workflow_catalog.py | ⏳ TODO | 30 min |
| ├─ | extensions/llm_config.py | ⏳ TODO | 10 min |
| ├─ | incident/llm_integration.py | ⏳ TODO | 10 min |
| ├─ | recovery/llm_integration.py | ⏳ TODO | 10 min |
| └─ | recovery_models.py (EnrichmentResults) | ⏳ TODO | 30 min |
| **Phase 2** | Audit Models | ⏳ TODO | 2-3 hrs |
| ├─ | Create audit_models.py | ⏳ TODO | 1 hr |
| └─ | Update events.py (4 functions) | ⏳ TODO | 1-2 hrs |
| **Phase 3** | Config TypedDict | ⏳ TODO | 1 hr |
| ├─ | Create config_models.py | ⏳ TODO | 20 min |
| └─ | Update 6 files | ⏳ TODO | 40 min |

**Total Progress**: 1/19 files completed (~5%)
**Estimated Remaining**: 5.5-6.5 hours

---

## ✅ **Next Steps**

1. **Continue Phase 1** (1.5 hours remaining)
   - Fix recovery/prompt_builder.py (same pattern as incident)
   - Fix toolsets/workflow_catalog.py (3 functions)
   - Fix llm_config.py (1 function)
   - Verify llm_integration.py files
   - Fix EnrichmentResults in recovery_models.py

2. **Execute Phase 2** (2-3 hours)
   - Create audit_models.py with 4 Pydantic models
   - Update events.py factory functions
   - Write unit tests for audit validation

3. **Execute Phase 3** (1 hour)
   - Create config_models.py with TypedDict
   - Update 6 files to use TypedDict
   - Verify type hints in IDE

4. **Comprehensive Testing**
   - Run all unit tests
   - Run all integration tests
   - Verify no mypy/pyright errors
   - Test IDE autocomplete functionality

---

## 🎯 **Success Criteria**

✅ **Phase 1 Complete** when:
- [ ] All 8 DetectedLabels violations fixed
- [ ] All EnrichmentResults violations fixed
- [ ] Unit tests pass with new types
- [ ] IDE shows autocomplete for DetectedLabels fields
- [ ] No type errors

✅ **Phase 2 Complete** when:
- [ ] 4 audit event_data Pydantic models created
- [ ] Factory functions use new models
- [ ] Audit events validate at creation time
- [ ] Integration tests pass

✅ **Phase 3 Complete** when:
- [ ] Config TypedDict created
- [ ] All config usage updated
- [ ] IDE shows type hints for config objects
- [ ] No type errors

✅ **v1.1 Ready** when:
- [ ] All 3 phases complete
- [ ] All tests passing
- [ ] No technical debt
- [ ] Type safety at 100%

---

**Prepared by**: AI Assistant
**Implementation Started**: December 17, 2025
**Current Status**: Phase 1 in progress (1/8 files complete)
**Next Action**: Continue with recovery/prompt_builder.py

