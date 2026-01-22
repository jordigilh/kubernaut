"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Unit Tests for Workflow Response Validation (DD-HAPI-002 v1.2)

TDD RED Phase: These tests are written FIRST, before implementation.

Business Requirements:
- BR-AI-023: Hallucination detection (workflow existence validation)
- BR-HAPI-191: Parameter validation in chat session
- BR-HAPI-196: Container image consistency validation

Design Decision: DD-HAPI-002 v1.2 - Workflow Response Validation Architecture
"""

import pytest
from unittest.mock import Mock
from typing import Dict, Any, List

# Import patch for use in decorators

# These imports will fail until we implement the classes (TDD RED)
# from src.clients.datastorage.client import DataStorageClient
# from src.validation.workflow_response_validator import (
#     WorkflowResponseValidator,
#     ValidationResult,
# )
# from datastorage.models import (
#     RemediationWorkflow,
#     ParameterSchema,
#     WorkflowParameterDef,
# )


# =============================================================================
# PHASE 1: Data Storage Client - get_workflow_by_id() Tests
# =============================================================================

class TestDataStorageClientGetWorkflowByUUID:
    """
    Tests for Data Storage Client get_workflow_by_id() method.

    Business Requirement: BR-AI-023 (Hallucination Detection)
    Design Decision: DD-HAPI-002 v1.2 Step 1
    """

    @pytest.fixture
    def mock_response_workflow(self) -> Dict[str, Any]:
        """Sample workflow response from Data Storage."""
        return {
            "workflow_id": "restart-pod-v1",
            "version": "1.0.0",
            "name": "Restart Pod Workflow",
            "description": "Restarts a failing pod",
            "content": "apiVersion: tekton.dev/v1beta1...",
            "content_hash": "sha256:abc123",
            "labels": {"signal_type": "OOMKilled", "severity": "critical"},
            "container_image": "ghcr.io/kubernaut/restart-pod:v1.0.0",
            "container_digest": "sha256:def456",
            "status": "active",
            "is_latest_version": True,
            "parameters": {
                "schema": {
                    "parameters": [
                        {
                            "name": "namespace",
                            "type": "string",
                            "required": True,
                            "description": "Target namespace",
                            "min_length": 1,
                            "max_length": 63
                        },
                        {
                            "name": "delay_seconds",
                            "type": "int",
                            "required": False,
                            "description": "Delay before restart",
                            "minimum": 0,
                            "maximum": 300,
                            "default": 30
                        },
                        {
                            "name": "strategy",
                            "type": "string",
                            "required": False,
                            "description": "Restart strategy",
                            "enum": ["graceful", "force"]
                        }
                    ]
                }
            },
            "created_at": "2025-12-01T00:00:00Z"
        }

    # NOTE: Removed obsolete test_get_workflow_by_id_* tests
    # These tests were for an obsolete DataStorageClient wrapper.
    # The OpenAPI client (WorkflowCatalogAPIApi) is tested via integration tests.
    # See: tests/integration/test_workflow_catalog_data_storage_integration.py
    pass


# =============================================================================
# PHASE 2: WorkflowResponseValidator - Step 1: Workflow Existence
# =============================================================================

class TestWorkflowExistenceValidation:
    """
    Tests for Step 1: Workflow existence validation.

    Business Requirement: BR-AI-023 (Hallucination Detection)
    Design Decision: DD-HAPI-002 v1.2 Step 1
    """

    @pytest.fixture
    def mock_ds_client(self):
        """Mock Data Storage client."""
        return Mock()

    @pytest.fixture
    def mock_workflow(self, mock_ds_client) -> Mock:
        """Sample workflow from Data Storage."""
        workflow = Mock()
        workflow.workflow_id = "restart-pod-v1"
        workflow.container_image = "ghcr.io/kubernaut/restart-pod:v1.0.0"
        workflow.container_digest = "sha256:def456"
        workflow.parameters = {
            "schema": {
                "parameters": []  # Empty for existence tests
            }
        }
        return workflow

    def test_validate_returns_error_when_workflow_not_found(self, mock_ds_client):
        """
        BR-AI-023: Hallucination detection - workflow doesn't exist.

        Given: LLM returns workflow_id that doesn't exist in catalog
        When: validate() is called
        Then: Returns ValidationResult with is_valid=False and error message
        """
        # Arrange
        mock_ds_client.get_workflow_by_id.return_value = None

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="hallucinated-workflow-xyz",
            container_image=None,
            parameters={}
        )

        # Assert
        assert result.is_valid is False
        assert len(result.errors) >= 1
        assert "not found in catalog" in result.errors[0].lower()
        assert "hallucinated-workflow-xyz" in result.errors[0]

    def test_validate_continues_when_workflow_exists(self, mock_ds_client, mock_workflow):
        """
        BR-AI-023: Workflow found - continue to next validation step.

        Given: LLM returns valid workflow_id that exists in catalog
        When: validate() is called
        Then: Does NOT return "not found" error
        """
        # Arrange
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,
            parameters={}
        )

        # Assert - no "not found" error (may have other errors)
        not_found_errors = [e for e in result.errors if "not found" in e.lower()]
        assert len(not_found_errors) == 0


# =============================================================================
# PHASE 2: WorkflowResponseValidator - Step 2: Container Image Consistency
# =============================================================================

class TestContainerImageConsistencyValidation:
    """
    Tests for Step 2: Container image consistency validation.

    Business Requirement: BR-HAPI-196 (Container Image Consistency)
    Design Decision: DD-HAPI-002 v1.2 Step 2
    """

    @pytest.fixture
    def mock_ds_client(self):
        """Mock Data Storage client."""
        return Mock()

    @pytest.fixture
    def mock_workflow(self) -> Mock:
        """Sample workflow with container image."""
        workflow = Mock()
        workflow.workflow_id = "restart-pod-v1"
        workflow.container_image = "ghcr.io/kubernaut/restart-pod:v1.0.0"
        workflow.container_digest = "sha256:def456"
        workflow.parameters = {"schema": {"parameters": []}}
        return workflow

    def test_validate_accepts_matching_container_image(self, mock_ds_client, mock_workflow):
        """
        BR-HAPI-196: Container image matches catalog.

        Given: LLM provides container_image that matches catalog
        When: validate() is called
        Then: No "mismatch" error
        """
        # Arrange
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image="ghcr.io/kubernaut/restart-pod:v1.0.0",  # Matches catalog
            parameters={}
        )

        # Assert
        mismatch_errors = [e for e in result.errors if "mismatch" in e.lower()]
        assert len(mismatch_errors) == 0

    def test_validate_accepts_null_container_image(self, mock_ds_client, mock_workflow):
        """
        BR-HAPI-196: Null image - use catalog value.

        Given: LLM provides null/empty container_image
        When: validate() is called
        Then: Returns validated_container_image from catalog
        """
        # Arrange
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,  # LLM didn't specify
            parameters={}
        )

        # Assert
        assert result.validated_container_image == "ghcr.io/kubernaut/restart-pod:v1.0.0"
        mismatch_errors = [e for e in result.errors if "mismatch" in e.lower()]
        assert len(mismatch_errors) == 0

    def test_validate_rejects_mismatched_container_image(self, mock_ds_client, mock_workflow):
        """
        BR-HAPI-196: Image mismatch - hallucination detected.

        Given: LLM provides container_image that doesn't match catalog
        When: validate() is called
        Then: Returns error with both images mentioned
        """
        # Arrange
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image="ghcr.io/evil/malware:latest",  # WRONG!
            parameters={}
        )

        # Assert
        assert result.is_valid is False
        mismatch_errors = [e for e in result.errors if "mismatch" in e.lower()]
        assert len(mismatch_errors) >= 1
        # Error should mention both the wrong image and the correct one
        error_text = " ".join(result.errors)
        assert "ghcr.io/evil/malware:latest" in error_text
        assert "ghcr.io/kubernaut/restart-pod:v1.0.0" in error_text


# =============================================================================
# PHASE 2: WorkflowResponseValidator - Step 3: Parameter Schema Validation
# =============================================================================

class TestParameterSchemaValidation:
    """
    Tests for Step 3: Parameter schema validation.

    Business Requirement: BR-HAPI-191 (Parameter Validation)
    Design Decision: DD-HAPI-002 v1.2 Step 3
    """

    @pytest.fixture
    def mock_ds_client(self):
        """Mock Data Storage client."""
        return Mock()

    def create_workflow_with_params(self, param_defs: List[Dict[str, Any]]) -> Mock:
        """Helper to create workflow with specific parameter schema."""
        workflow = Mock()
        workflow.workflow_id = "test-workflow"
        workflow.container_image = "ghcr.io/kubernaut/test:v1.0.0"
        workflow.container_digest = "sha256:abc123"
        workflow.parameters = {
            "schema": {
                "parameters": param_defs
            }
        }
        return workflow

    # --- Required Parameter Tests ---

    def test_validate_rejects_missing_required_parameter(self, mock_ds_client):
        """
        BR-HAPI-191: Required parameter missing.

        Given: Workflow schema requires 'namespace' parameter
        When: validate() called without 'namespace'
        Then: Returns error mentioning missing required parameter
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "namespace", "type": "string", "required": True}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={}  # Missing 'namespace'!
        )

        # Assert
        assert result.is_valid is False
        required_errors = [e for e in result.errors if "required" in e.lower()]
        assert len(required_errors) >= 1
        assert "namespace" in " ".join(result.errors).lower()

    def test_validate_accepts_optional_parameter_missing(self, mock_ds_client):
        """
        BR-HAPI-191: Optional parameter can be omitted.

        Given: Workflow schema has optional 'delay' parameter
        When: validate() called without 'delay'
        Then: No error about missing parameter
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "delay", "type": "int", "required": False}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={}  # Optional 'delay' not provided - OK
        )

        # Assert
        required_errors = [e for e in result.errors if "required" in e.lower() and "delay" in e.lower()]
        assert len(required_errors) == 0

    # --- Type Validation Tests ---

    def test_validate_rejects_wrong_type_expected_string(self, mock_ds_client):
        """
        BR-HAPI-191: Type mismatch - expected string, got int.

        Given: Workflow schema requires 'namespace' as string
        When: validate() called with namespace=12345 (int)
        Then: Returns type error
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "namespace", "type": "string", "required": True}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={"namespace": 12345}  # Wrong type!
        )

        # Assert
        assert result.is_valid is False
        type_errors = [e for e in result.errors if "string" in e.lower()]
        assert len(type_errors) >= 1

    def test_validate_rejects_wrong_type_expected_int(self, mock_ds_client):
        """
        BR-HAPI-191: Type mismatch - expected int, got string.

        Given: Workflow schema requires 'replicas' as int
        When: validate() called with replicas="five" (string)
        Then: Returns type error
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "replicas", "type": "int", "required": True}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={"replicas": "five"}  # Wrong type!
        )

        # Assert
        assert result.is_valid is False
        type_errors = [e for e in result.errors if "int" in e.lower()]
        assert len(type_errors) >= 1

    def test_validate_accepts_all_correct_types(self, mock_ds_client):
        """
        BR-HAPI-191: All types correct - validation passes.

        Given: Workflow schema with multiple typed parameters
        When: validate() called with correct types
        Then: No type errors
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "namespace", "type": "string", "required": True},
            {"name": "replicas", "type": "int", "required": True},
            {"name": "enabled", "type": "bool", "required": True},
            {"name": "threshold", "type": "float", "required": True}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={
                "namespace": "production",
                "replicas": 3,
                "enabled": True,
                "threshold": 0.95
            }
        )

        # Assert - no type errors
        type_errors = [e for e in result.errors if "expected" in e.lower() and "got" in e.lower()]
        assert len(type_errors) == 0

    # --- String Length Validation Tests ---

    def test_validate_rejects_string_too_short(self, mock_ds_client):
        """
        BR-HAPI-191: String length below minimum.

        Given: Workflow schema requires namespace with min_length=3
        When: validate() called with namespace="a" (length 1)
        Then: Returns length error
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "namespace", "type": "string", "required": True, "min_length": 3}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={"namespace": "a"}  # Too short!
        )

        # Assert
        assert result.is_valid is False
        length_errors = [e for e in result.errors if "length" in e.lower() and ">=" in e]
        assert len(length_errors) >= 1

    def test_validate_rejects_string_too_long(self, mock_ds_client):
        """
        BR-HAPI-191: String length above maximum.

        Given: Workflow schema requires namespace with max_length=63
        When: validate() called with namespace of 100 chars
        Then: Returns length error
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "namespace", "type": "string", "required": True, "max_length": 63}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={"namespace": "a" * 100}  # Too long!
        )

        # Assert
        assert result.is_valid is False
        length_errors = [e for e in result.errors if "length" in e.lower() and "<=" in e]
        assert len(length_errors) >= 1

    # --- Numeric Range Validation Tests ---

    def test_validate_rejects_number_below_minimum(self, mock_ds_client):
        """
        BR-HAPI-191: Numeric value below minimum.

        Given: Workflow schema requires replicas with minimum=1
        When: validate() called with replicas=0
        Then: Returns range error
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "replicas", "type": "int", "required": True, "minimum": 1}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={"replicas": 0}  # Below minimum!
        )

        # Assert
        assert result.is_valid is False
        range_errors = [e for e in result.errors if ">=" in e]
        assert len(range_errors) >= 1

    def test_validate_rejects_number_above_maximum(self, mock_ds_client):
        """
        BR-HAPI-191: Numeric value above maximum.

        Given: Workflow schema requires replicas with maximum=100
        When: validate() called with replicas=1000
        Then: Returns range error
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "replicas", "type": "int", "required": True, "maximum": 100}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={"replicas": 1000}  # Above maximum!
        )

        # Assert
        assert result.is_valid is False
        range_errors = [e for e in result.errors if "<=" in e]
        assert len(range_errors) >= 1

    # --- Enum Validation Tests ---

    def test_validate_rejects_invalid_enum_value(self, mock_ds_client):
        """
        BR-HAPI-191: Value not in enum.

        Given: Workflow schema requires strategy in ["RollingUpdate", "Recreate"]
        When: validate() called with strategy="Invalid"
        Then: Returns enum error
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "strategy", "type": "string", "required": True,
             "enum": ["RollingUpdate", "Recreate"]}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={"strategy": "Invalid"}  # Not in enum!
        )

        # Assert
        assert result.is_valid is False
        enum_errors = [e for e in result.errors if "must be one of" in e.lower()]
        assert len(enum_errors) >= 1

    def test_validate_accepts_valid_enum_value(self, mock_ds_client):
        """
        BR-HAPI-191: Value in enum - validation passes.

        Given: Workflow schema requires strategy in ["RollingUpdate", "Recreate"]
        When: validate() called with strategy="RollingUpdate"
        Then: No enum error
        """
        # Arrange
        workflow = self.create_workflow_with_params([
            {"name": "strategy", "type": "string", "required": True,
             "enum": ["RollingUpdate", "Recreate"]}
        ])
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image=None,
            parameters={"strategy": "RollingUpdate"}  # Valid enum value
        )

        # Assert
        enum_errors = [e for e in result.errors if "must be one of" in e.lower()]
        assert len(enum_errors) == 0


# =============================================================================
# PHASE 2: WorkflowResponseValidator - Complete Validation Flow
# =============================================================================

class TestCompleteValidationFlow:
    """
    Tests for complete validation flow (all 3 steps).

    Design Decision: DD-HAPI-002 v1.2
    """

    @pytest.fixture
    def mock_ds_client(self):
        """Mock Data Storage client."""
        return Mock()

    def test_validate_returns_all_errors_combined(self, mock_ds_client):
        """
        DD-HAPI-002: All validation errors returned together.

        Given: Multiple validation failures (image mismatch + missing param)
        When: validate() is called
        Then: Returns all errors in single response
        """
        # Arrange
        workflow = Mock()
        workflow.workflow_id = "test-workflow"
        workflow.container_image = "ghcr.io/kubernaut/correct:v1.0.0"
        workflow.container_digest = "sha256:abc123"
        workflow.parameters = {
            "schema": {
                "parameters": [
                    {"name": "namespace", "type": "string", "required": True}
                ]
            }
        }
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            container_image="ghcr.io/wrong/image:v1.0.0",  # WRONG
            parameters={}  # Missing required 'namespace'
        )

        # Assert
        assert result.is_valid is False
        assert len(result.errors) >= 2  # At least 2 errors
        error_text = " ".join(result.errors).lower()
        assert "mismatch" in error_text or "wrong" in error_text.lower()
        assert "required" in error_text or "namespace" in error_text

    def test_validate_returns_success_when_all_valid(self, mock_ds_client):
        """
        DD-HAPI-002: All validation passes.

        Given: Valid workflow_id, correct container_image, valid parameters
        When: validate() is called
        Then: Returns is_valid=True with no errors
        """
        # Arrange
        workflow = Mock()
        workflow.workflow_id = "restart-pod-v1"
        workflow.container_image = "ghcr.io/kubernaut/restart:v1.0.0"
        workflow.container_digest = "sha256:abc123"
        workflow.parameters = {
            "schema": {
                "parameters": [
                    {"name": "namespace", "type": "string", "required": True},
                    {"name": "delay", "type": "int", "required": False, "minimum": 0, "maximum": 300}
                ]
            }
        }
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="restart-pod-v1",
            container_image=None,  # Use catalog value
            parameters={"namespace": "production", "delay": 30}
        )

        # Assert
        assert result.is_valid is True
        assert len(result.errors) == 0
        assert result.validated_container_image == "ghcr.io/kubernaut/restart:v1.0.0"

