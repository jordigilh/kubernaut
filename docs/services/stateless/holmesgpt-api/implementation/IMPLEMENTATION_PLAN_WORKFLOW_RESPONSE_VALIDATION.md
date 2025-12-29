# Implementation Plan: Workflow Response Validation (DD-HAPI-002 v1.2)

**Version**: 1.1
**Date**: December 6, 2025
**Status**: ✅ **IMPLEMENTATION COMPLETE**
**Priority**: 🔴 V1.0
**Authoritative DD**: [DD-HAPI-002 v1.2](../../../../architecture/decisions/DD-HAPI-002-workflow-parameter-validation.md)

---

## Overview

Implement automatic workflow response validation in HolmesGPT-API per DD-HAPI-002 v1.2:

| Validation | Description | Priority |
|------------|-------------|----------|
| **Workflow Existence** | Verify `workflow_id` exists in catalog | 🔴 V1.0 |
| **Container Image Consistency** | Verify `container_image` matches catalog | 🔴 V1.0 |
| **Parameter Schema** | Verify parameters conform to schema | 🔴 V1.0 |

---

## Business Requirements Mapping

| BR ID | Description | Validation |
|-------|-------------|------------|
| **BR-AI-023** | Hallucination detection | Workflow existence |
| **BR-HAPI-191** | Parameter validation in chat session | Parameter schema |
| **NEW: BR-HAPI-196** | Container image consistency | Image validation |

---

## Architecture Summary

```
LLM returns JSON response
         │
         ▼
┌─────────────────────────────────────────┐
│  WorkflowResponseValidator (NEW)        │
│                                         │
│  ┌─────────────────────────────────┐   │
│  │ Step 1: Workflow Existence      │   │
│  │ GET /api/v1/workflows/{id}      │   │
│  │ ├─ 200 OK → Continue            │   │
│  │ └─ 404 → Error to LLM           │   │
│  └─────────────────────────────────┘   │
│                │                        │
│                ▼                        │
│  ┌─────────────────────────────────┐   │
│  │ Step 2: Container Image Match   │   │
│  │ LLM_image vs Catalog_image      │   │
│  │ ├─ Match → Continue             │   │
│  │ ├─ Null → Use catalog (OK)      │   │
│  │ └─ Mismatch → Error to LLM      │   │
│  └─────────────────────────────────┘   │
│                │                        │
│                ▼                        │
│  ┌─────────────────────────────────┐   │
│  │ Step 3: Parameter Schema        │   │
│  │ ├─ Required                     │   │
│  │ ├─ Type (str/int/bool/float)    │   │
│  │ ├─ Length (min/max)             │   │
│  │ ├─ Range (min/max)              │   │
│  │ └─ Enum                         │   │
│  └─────────────────────────────────┘   │
│                │                        │
│                ▼                        │
│  ┌─────────────┐    ┌──────────────┐   │
│  │  ALL VALID  │    │  ANY INVALID │   │
│  │  Return OK  │    │  Error→LLM   │   │
│  └─────────────┘    │  Self-correct│   │
│                     │  (max 3)     │   │
│                     └──────────────┘   │
└─────────────────────────────────────────┘
```

---

## Implementation Phases

### Phase 1: Data Storage Client Enhancement (Day 1)

#### 1.1 Add `get_workflow` Method

**File**: `holmesgpt-api/src/clients/data_storage.py`

**TDD RED** - Write failing tests first:

```python
# tests/unit/test_data_storage_client.py

class TestDataStorageClient:
    """Tests for Data Storage client."""

    def test_get_workflow_returns_workflow_when_exists(self, mock_http_client):
        """BR-AI-023: Workflow existence validation."""
        mock_http_client.get.return_value.json.return_value = {
            "workflow_id": "restart-pod-v1",
            "container_image": "ghcr.io/kubernaut/restart:v1.0.0",
            "schema": {
                "parameters": [
                    {"name": "namespace", "type": "string", "required": True}
                ]
            }
        }

        client = DataStorageClient(mock_http_client)
        workflow = client.get_workflow("restart-pod-v1")

        assert workflow is not None
        assert workflow.workflow_id == "restart-pod-v1"
        assert workflow.container_image == "ghcr.io/kubernaut/restart:v1.0.0"

    def test_get_workflow_returns_none_when_not_found(self, mock_http_client):
        """BR-AI-023: Hallucination detection - workflow not found."""
        mock_http_client.get.return_value.status_code = 404
        mock_http_client.get.return_value.json.side_effect = HTTPError(404)

        client = DataStorageClient(mock_http_client)
        workflow = client.get_workflow("non-existent-workflow")

        assert workflow is None

    def test_get_workflow_includes_parameter_schema(self, mock_http_client):
        """BR-HAPI-191: Parameter schema available for validation."""
        mock_http_client.get.return_value.json.return_value = {
            "workflow_id": "scale-deployment-v1",
            "container_image": "ghcr.io/kubernaut/scale:v1.0.0",
            "schema": {
                "parameters": [
                    {"name": "replicas", "type": "int", "required": True, "minimum": 1, "maximum": 100},
                    {"name": "namespace", "type": "string", "required": True, "min_length": 1, "max_length": 63},
                    {"name": "strategy", "type": "string", "enum": ["RollingUpdate", "Recreate"]}
                ]
            }
        }

        client = DataStorageClient(mock_http_client)
        workflow = client.get_workflow("scale-deployment-v1")

        assert len(workflow.schema.parameters) == 3
        assert workflow.schema.parameters[0].name == "replicas"
        assert workflow.schema.parameters[0].minimum == 1
```

**TDD GREEN** - Minimal implementation:

```python
# src/clients/data_storage.py

from typing import Optional
from pydantic import BaseModel
from typing import List, Any

class ParameterSchema(BaseModel):
    name: str
    type: str
    required: bool = False
    minimum: Optional[float] = None
    maximum: Optional[float] = None
    min_length: Optional[int] = None
    max_length: Optional[int] = None
    enum: Optional[List[str]] = None

class WorkflowSchema(BaseModel):
    parameters: List[ParameterSchema]

class Workflow(BaseModel):
    workflow_id: str
    container_image: str
    schema: WorkflowSchema

class DataStorageClient:
    def __init__(self, http_client):
        self.http_client = http_client
        self.base_url = config.data_storage_url

    def get_workflow(self, workflow_id: str) -> Optional[Workflow]:
        """
        Get workflow by ID from Data Storage.

        Returns:
            Workflow if found, None if not found (404)
        """
        try:
            response = self.http_client.get(
                f"{self.base_url}/api/v1/workflows/{workflow_id}"
            )
            response.raise_for_status()
            return Workflow(**response.json())
        except HTTPError as e:
            if e.response.status_code == 404:
                return None
            raise
```

---

### Phase 2: WorkflowResponseValidator Class (Day 2-3)

#### 2.1 Workflow Existence Validation

**File**: `holmesgpt-api/src/validation/workflow_response_validator.py`

**TDD RED** - Write failing tests:

```python
# tests/unit/test_workflow_response_validator.py

class TestWorkflowExistenceValidation:
    """Tests for Step 1: Workflow existence validation."""

    def test_validate_returns_error_when_workflow_not_found(self, mock_ds_client):
        """BR-AI-023: Hallucination detection - workflow doesn't exist."""
        mock_ds_client.get_workflow.return_value = None

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="hallucinated-workflow",
            container_image=None,
            parameters={}
        )

        assert result.is_valid is False
        assert len(result.errors) == 1
        assert "not found in catalog" in result.errors[0]
        assert "hallucinated-workflow" in result.errors[0]

    def test_validate_continues_when_workflow_exists(self, mock_ds_client, mock_workflow):
        """BR-AI-023: Workflow found - continue validation."""
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={}
        )

        # May have other errors, but not "not found"
        assert not any("not found" in err for err in result.errors)
```

#### 2.2 Container Image Consistency Validation

```python
class TestContainerImageConsistencyValidation:
    """Tests for Step 2: Container image consistency."""

    def test_validate_accepts_matching_container_image(self, mock_ds_client, mock_workflow):
        """BR-HAPI-196: Container image matches catalog."""
        mock_workflow.container_image = "ghcr.io/kubernaut/restart:v1.0.0"
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image="ghcr.io/kubernaut/restart:v1.0.0",
            parameters={}
        )

        assert not any("mismatch" in err.lower() for err in result.errors)

    def test_validate_accepts_null_container_image(self, mock_ds_client, mock_workflow):
        """BR-HAPI-196: Null image - use catalog value."""
        mock_workflow.container_image = "ghcr.io/kubernaut/restart:v1.0.0"
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,  # LLM didn't specify
            parameters={}
        )

        assert result.validated_container_image == "ghcr.io/kubernaut/restart:v1.0.0"
        assert not any("mismatch" in err.lower() for err in result.errors)

    def test_validate_rejects_mismatched_container_image(self, mock_ds_client, mock_workflow):
        """BR-HAPI-196: Image mismatch - hallucination detected."""
        mock_workflow.container_image = "ghcr.io/kubernaut/restart:v1.0.0"
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image="ghcr.io/evil/malware:latest",  # WRONG!
            parameters={}
        )

        assert result.is_valid is False
        assert any("mismatch" in err.lower() for err in result.errors)
        assert "ghcr.io/evil/malware:latest" in str(result.errors)
```

#### 2.3 Parameter Schema Validation

```python
class TestParameterSchemaValidation:
    """Tests for Step 3: Parameter schema validation."""

    # --- Required Parameter Tests ---

    def test_validate_rejects_missing_required_parameter(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: Required parameter missing."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="namespace", type="string", required=True)
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={}  # Missing 'namespace'
        )

        assert result.is_valid is False
        assert any("Missing required parameter" in err for err in result.errors)
        assert any("namespace" in err for err in result.errors)

    def test_validate_accepts_optional_parameter_missing(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: Optional parameter can be omitted."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="delay", type="int", required=False)
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={}  # Optional 'delay' not provided
        )

        assert not any("Missing required" in err for err in result.errors)

    # --- Type Validation Tests ---

    def test_validate_rejects_wrong_type_string(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: Type mismatch - expected string."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="namespace", type="string", required=True)
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={"namespace": 12345}  # Wrong type!
        )

        assert result.is_valid is False
        assert any("expected string" in err for err in result.errors)

    def test_validate_rejects_wrong_type_int(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: Type mismatch - expected int."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="replicas", type="int", required=True)
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={"replicas": "five"}  # Wrong type!
        )

        assert result.is_valid is False
        assert any("expected int" in err for err in result.errors)

    def test_validate_accepts_correct_types(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: All types correct."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="namespace", type="string", required=True),
            ParameterSchema(name="replicas", type="int", required=True),
            ParameterSchema(name="enabled", type="bool", required=True),
            ParameterSchema(name="threshold", type="float", required=True)
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={
                "namespace": "production",
                "replicas": 3,
                "enabled": True,
                "threshold": 0.95
            }
        )

        assert not any("expected" in err for err in result.errors)

    # --- String Length Validation Tests ---

    def test_validate_rejects_string_too_short(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: String length below minimum."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="namespace", type="string", required=True, min_length=3)
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={"namespace": "a"}  # Too short!
        )

        assert result.is_valid is False
        assert any("length must be >=" in err for err in result.errors)

    def test_validate_rejects_string_too_long(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: String length above maximum."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="namespace", type="string", required=True, max_length=63)
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={"namespace": "a" * 100}  # Too long!
        )

        assert result.is_valid is False
        assert any("length must be <=" in err for err in result.errors)

    # --- Numeric Range Validation Tests ---

    def test_validate_rejects_number_below_minimum(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: Numeric value below minimum."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="replicas", type="int", required=True, minimum=1)
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={"replicas": 0}  # Below minimum!
        )

        assert result.is_valid is False
        assert any("must be >=" in err for err in result.errors)

    def test_validate_rejects_number_above_maximum(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: Numeric value above maximum."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="replicas", type="int", required=True, maximum=100)
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={"replicas": 1000}  # Above maximum!
        )

        assert result.is_valid is False
        assert any("must be <=" in err for err in result.errors)

    # --- Enum Validation Tests ---

    def test_validate_rejects_invalid_enum_value(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: Value not in enum."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="strategy", type="string", required=True,
                          enum=["RollingUpdate", "Recreate"])
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={"strategy": "Invalid"}  # Not in enum!
        )

        assert result.is_valid is False
        assert any("must be one of" in err for err in result.errors)

    def test_validate_accepts_valid_enum_value(self, mock_ds_client, mock_workflow):
        """BR-HAPI-191: Value in enum."""
        mock_workflow.schema.parameters = [
            ParameterSchema(name="strategy", type="string", required=True,
                          enum=["RollingUpdate", "Recreate"])
        ]
        mock_ds_client.get_workflow.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={"strategy": "RollingUpdate"}
        )

        assert not any("must be one of" in err for err in result.errors)
```

---

### Phase 3: Integration with Incident Parser (Day 4)

#### 3.1 Integrate Validator into `_parse_investigation_result`

**File**: `holmesgpt-api/src/extensions/incident.py`

**TDD RED** - Integration tests:

```python
# tests/integration/test_incident_validation_integration.py

class TestIncidentValidationIntegration:
    """Integration tests for workflow response validation."""

    def test_invalid_workflow_returns_errors_to_llm(self, client, mock_ds_client):
        """BR-AI-023: Invalid workflow triggers self-correction flow."""
        mock_ds_client.get_workflow.return_value = None  # Workflow not found

        # Simulate LLM returning invalid workflow
        response = client.post("/api/v1/incident/analyze", json={
            "alert_name": "HighCPU",
            "namespace": "production",
            "owner_chain": ["Deployment/app", "ReplicaSet/app-xyz", "Pod/app-xyz-123"]
        })

        # Check that validation error was returned (not a successful response)
        data = response.json()
        assert "validation_failed" in data or "error" in data

    def test_valid_workflow_returns_success(self, client, mock_ds_client, mock_workflow):
        """BR-HAPI-191: Valid workflow passes validation."""
        mock_ds_client.get_workflow.return_value = mock_workflow

        response = client.post("/api/v1/incident/analyze", json={
            "alert_name": "HighCPU",
            "namespace": "production",
            "owner_chain": ["Deployment/app", "ReplicaSet/app-xyz", "Pod/app-xyz-123"]
        })

        data = response.json()
        assert "selected_workflow" in data
        assert data["selected_workflow"]["workflow_id"] == "restart-pod-v1"
```

---

### Phase 4: LLM Self-Correction Loop (Day 5)

#### 4.1 Implement Retry Logic

**TDD RED** - Self-correction tests:

```python
class TestLLMSelfCorrection:
    """Tests for LLM self-correction on validation failure."""

    def test_llm_retries_on_validation_failure(self, client, mock_llm):
        """DD-HAPI-002: LLM self-corrects on validation error."""
        # First attempt: invalid workflow
        # Second attempt: valid workflow
        mock_llm.responses = [
            {"workflow_id": "invalid-workflow", "parameters": {}},  # Attempt 1
            {"workflow_id": "restart-pod-v1", "parameters": {"namespace": "prod"}}  # Attempt 2
        ]

        response = client.post("/api/v1/incident/analyze", json={...})

        assert mock_llm.call_count == 2  # Retried once
        assert response.json()["selected_workflow"]["workflow_id"] == "restart-pod-v1"

    def test_llm_fails_after_max_attempts(self, client, mock_llm):
        """DD-HAPI-002: Max 3 attempts, then needs_human_review."""
        mock_llm.responses = [
            {"workflow_id": "invalid-1", "parameters": {}},  # Attempt 1
            {"workflow_id": "invalid-2", "parameters": {}},  # Attempt 2
            {"workflow_id": "invalid-3", "parameters": {}},  # Attempt 3
        ]

        response = client.post("/api/v1/incident/analyze", json={...})

        assert mock_llm.call_count == 3
        assert response.json()["needs_human_review"] is True

    def test_validation_errors_passed_to_llm(self, client, mock_llm, capture_llm_prompts):
        """DD-HAPI-002: Errors returned to LLM for self-correction."""
        mock_llm.responses = [
            {"workflow_id": "invalid-workflow", "parameters": {}},
            {"workflow_id": "restart-pod-v1", "parameters": {"namespace": "prod"}}
        ]

        response = client.post("/api/v1/incident/analyze", json={...})

        # Second prompt should include validation errors
        second_prompt = capture_llm_prompts[1]
        assert "not found in catalog" in second_prompt
        assert "Please correct" in second_prompt
```

---

## Test Summary

### Unit Tests (Phase 1-3)

| Category | Tests | BR Coverage |
|----------|-------|-------------|
| Data Storage Client | 3 | BR-AI-023 |
| Workflow Existence | 2 | BR-AI-023 |
| Container Image | 3 | BR-HAPI-196 |
| Required Parameters | 2 | BR-HAPI-191 |
| Type Validation | 3 | BR-HAPI-191 |
| String Length | 2 | BR-HAPI-191 |
| Numeric Range | 2 | BR-HAPI-191 |
| Enum Validation | 2 | BR-HAPI-191 |
| **Total Unit** | **19** | |

### Integration Tests (Phase 3)

| Category | Tests | BR Coverage |
|----------|-------|-------------|
| Validation Integration | 2 | BR-AI-023, BR-HAPI-191 |

### E2E Tests (Phase 4)

| Category | Tests | BR Coverage |
|----------|-------|-------------|
| Self-Correction Loop | 3 | DD-HAPI-002 |

### Total New Tests: **24**

---

## Timeline

| Phase | Duration | Status |
|-------|----------|--------|
| Phase 1: Data Storage Client | 1 day | ✅ **DONE** |
| Phase 2: WorkflowResponseValidator | 2 days | ✅ **DONE** |
| Phase 3: Integration | 1 day | ✅ **DONE** |
| Phase 4: Self-Correction Loop | 1 day | ✅ **DONE** |
| **Total** | **5 days** | |

---

## Files to Create/Modify

### New Files

| File | Purpose |
|------|---------|
| `src/validation/workflow_response_validator.py` | Validator class |
| `src/validation/__init__.py` | Package init |
| `src/models/workflow_schema.py` | Schema models |
| `tests/unit/test_workflow_response_validator.py` | Unit tests |
| `tests/integration/test_incident_validation_integration.py` | Integration tests |

### Modified Files

| File | Changes |
|------|---------|
| `src/clients/data_storage.py` | Add `get_workflow()` method |
| `src/extensions/incident.py` | Integrate validator into parser |
| `src/extensions/recovery.py` | Same validation for recovery flow |
| `tests/conftest.py` | Add validation fixtures |

---

## Triage Against DD-HAPI-002 v1.2

### Consistency Check

| DD Requirement | Implementation | Status |
|----------------|----------------|--------|
| Workflow existence via GET /workflows/{id} | Phase 1: `get_workflow()` | ✅ Consistent |
| Container image comparison | Phase 2: `_validate_container_image()` | ✅ Consistent |
| Parameter schema validation | Phase 2: `_validate_parameters()` | ✅ Consistent |
| Automatic validation (not tool) | Phase 3: Integrated in parser | ✅ Consistent |
| LLM self-correction (max 3) | Phase 4: Retry loop | ✅ Consistent |
| needs_human_review on failure | Phase 4: After max attempts | ✅ Consistent |
| Schema hint in errors | Phase 2: `schema_hint` in result | ✅ Consistent |

### Gap Analysis: None

All DD-HAPI-002 v1.2 requirements are covered in the implementation plan.

---

## Risk Assessment

| Risk | Mitigation | Impact |
|------|------------|--------|
| Data Storage API unavailable | Circuit breaker, graceful degradation | Medium |
| LLM doesn't understand errors | Clear error messages, schema hints | Low |
| Performance impact | Cache workflow schemas | Low |
| Schema changes | Workflow immutability (per DD) | None |

---

## Approval

| Role | Approver | Status |
|------|----------|--------|
| Architecture | TBD | ⏳ Pending |
| HolmesGPT-API Team | TBD | ⏳ Pending |
| QA | TBD | ⏳ Pending |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-05 | Initial implementation plan |

