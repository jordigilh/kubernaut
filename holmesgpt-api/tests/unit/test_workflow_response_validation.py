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
            "labels": {"signal_type": "OOMKilled", "severity": ["critical"]},
            "schema_image": "ghcr.io/kubernaut/restart-pod:v1.0.0",
            "schema_digest": "sha256:def456",
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
                            "max_length": 63,
                        },
                        {
                            "name": "delay_seconds",
                            "type": "int",
                            "required": False,
                            "description": "Delay before restart",
                            "minimum": 0,
                            "maximum": 300,
                            "default": 30,
                        },
                        {
                            "name": "strategy",
                            "type": "string",
                            "required": False,
                            "description": "Restart strategy",
                            "enum": ["graceful", "force"],
                        },
                    ]
                }
            },
            "created_at": "2025-12-01T00:00:00Z",
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
        workflow.execution_bundle = "ghcr.io/kubernaut/restart-pod:v1.0.0"
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
            execution_bundle=None,
            parameters={},
        )

        # Assert
        assert result.is_valid is False
        assert len(result.errors) >= 1
        assert "not found in catalog" in result.errors[0].lower()
        assert "hallucinated-workflow-xyz" in result.errors[0]

    def test_validate_continues_when_workflow_exists(
        self, mock_ds_client, mock_workflow
    ):
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
            workflow_id="restart-pod-v1", execution_bundle=None, parameters={}
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
        """Sample workflow with execution bundle."""
        workflow = Mock()
        workflow.workflow_id = "restart-pod-v1"
        workflow.execution_bundle = "ghcr.io/kubernaut/restart-pod:v1.0.0"
        workflow.parameters = {"schema": {"parameters": []}}
        return workflow

    def test_validate_accepts_matching_execution_bundle(
        self, mock_ds_client, mock_workflow
    ):
        """
        BR-HAPI-196: Execution bundle matches catalog.

        Given: LLM provides execution_bundle that matches catalog
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
            execution_bundle="ghcr.io/kubernaut/restart-pod:v1.0.0",
            parameters={},
        )

        # Assert
        mismatch_errors = [e for e in result.errors if "mismatch" in e.lower()]
        assert len(mismatch_errors) == 0

    def test_validate_accepts_null_execution_bundle(
        self, mock_ds_client, mock_workflow
    ):
        """
        BR-HAPI-196: Null bundle - use catalog value.

        Given: LLM provides null/empty execution_bundle
        When: validate() is called
        Then: Returns validated_execution_bundle from catalog
        """
        # Arrange
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="restart-pod-v1", execution_bundle=None, parameters={}
        )

        # Assert
        assert (
            result.validated_execution_bundle == "ghcr.io/kubernaut/restart-pod:v1.0.0"
        )
        mismatch_errors = [e for e in result.errors if "mismatch" in e.lower()]
        assert len(mismatch_errors) == 0

    def test_validate_rejects_mismatched_execution_bundle(
        self, mock_ds_client, mock_workflow
    ):
        """
        BR-HAPI-196: Bundle mismatch - hallucination detected.

        Given: LLM provides execution_bundle that doesn't match catalog
        When: validate() is called
        Then: Returns error with both bundles mentioned
        """
        # Arrange
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="restart-pod-v1",
            execution_bundle="ghcr.io/evil/malware:latest",
            parameters={},
        )

        # Assert
        assert result.is_valid is False
        mismatch_errors = [e for e in result.errors if "mismatch" in e.lower()]
        assert len(mismatch_errors) >= 1
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
        workflow.execution_bundle = "ghcr.io/kubernaut/test:v1.0.0"
        workflow.execution_bundle_digest = "sha256:abc123"
        workflow.parameters = {"schema": {"parameters": param_defs}}
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
        workflow = self.create_workflow_with_params(
            [{"name": "namespace", "type": "string", "required": True}]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={},  # Missing 'namespace'!
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
        workflow = self.create_workflow_with_params(
            [{"name": "delay", "type": "int", "required": False}]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={},  # Optional 'delay' not provided - OK
        )

        # Assert
        required_errors = [
            e for e in result.errors if "required" in e.lower() and "delay" in e.lower()
        ]
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
        workflow = self.create_workflow_with_params(
            [{"name": "namespace", "type": "string", "required": True}]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={"namespace": 12345},  # Wrong type!
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
        workflow = self.create_workflow_with_params(
            [{"name": "replicas", "type": "int", "required": True}]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={"replicas": "five"},  # Wrong type!
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
        workflow = self.create_workflow_with_params(
            [
                {"name": "namespace", "type": "string", "required": True},
                {"name": "replicas", "type": "int", "required": True},
                {"name": "enabled", "type": "bool", "required": True},
                {"name": "threshold", "type": "float", "required": True},
            ]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={
                "namespace": "production",
                "replicas": 3,
                "enabled": True,
                "threshold": 0.95,
            },
        )

        # Assert - no type errors
        type_errors = [
            e for e in result.errors if "expected" in e.lower() and "got" in e.lower()
        ]
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
        workflow = self.create_workflow_with_params(
            [{"name": "namespace", "type": "string", "required": True, "min_length": 3}]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={"namespace": "a"},  # Too short!
        )

        # Assert
        assert result.is_valid is False
        length_errors = [
            e for e in result.errors if "length" in e.lower() and ">=" in e
        ]
        assert len(length_errors) >= 1

    def test_validate_rejects_string_too_long(self, mock_ds_client):
        """
        BR-HAPI-191: String length above maximum.

        Given: Workflow schema requires namespace with max_length=63
        When: validate() called with namespace of 100 chars
        Then: Returns length error
        """
        # Arrange
        workflow = self.create_workflow_with_params(
            [
                {
                    "name": "namespace",
                    "type": "string",
                    "required": True,
                    "max_length": 63,
                }
            ]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={"namespace": "a" * 100},  # Too long!
        )

        # Assert
        assert result.is_valid is False
        length_errors = [
            e for e in result.errors if "length" in e.lower() and "<=" in e
        ]
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
        workflow = self.create_workflow_with_params(
            [{"name": "replicas", "type": "int", "required": True, "minimum": 1}]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={"replicas": 0},  # Below minimum!
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
        workflow = self.create_workflow_with_params(
            [{"name": "replicas", "type": "int", "required": True, "maximum": 100}]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={"replicas": 1000},  # Above maximum!
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
        workflow = self.create_workflow_with_params(
            [
                {
                    "name": "strategy",
                    "type": "string",
                    "required": True,
                    "enum": ["RollingUpdate", "Recreate"],
                }
            ]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={"strategy": "Invalid"},  # Not in enum!
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
        workflow = self.create_workflow_with_params(
            [
                {
                    "name": "strategy",
                    "type": "string",
                    "required": True,
                    "enum": ["RollingUpdate", "Recreate"],
                }
            ]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters={"strategy": "RollingUpdate"},  # Valid enum value
        )

        # Assert
        enum_errors = [e for e in result.errors if "must be one of" in e.lower()]
        assert len(enum_errors) == 0

    # --- Undeclared Parameter Stripping Tests (Issue #241, DD-HAPI-002 v1.3) ---

    def test_undeclared_params_stripped_from_dict(self, mock_ds_client):
        """
        UT-HAPI-241-001: Undeclared params stripped; declared preserved.

        Given: Schema declares ["TARGET_NAMESPACE", "TARGET_RESOURCE_NAME"]
        When: LLM provides those plus GIT_PASSWORD and GIT_USERNAME
        Then: After validation, params dict contains only the declared keys
        """
        workflow = self.create_workflow_with_params(
            [
                {"name": "TARGET_NAMESPACE", "type": "string", "required": True},
                {"name": "TARGET_RESOURCE_NAME", "type": "string", "required": True},
            ]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        params = {
            "TARGET_NAMESPACE": "prod",
            "TARGET_RESOURCE_NAME": "cert",
            "GIT_PASSWORD": "secret123",
            "GIT_USERNAME": "admin",
        }
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters=params,
        )

        assert result.is_valid is True
        assert result.errors == []
        assert params == {"TARGET_NAMESPACE": "prod", "TARGET_RESOURCE_NAME": "cert"}

    def test_declared_params_preserved_unchanged(self, mock_ds_client):
        """
        UT-HAPI-241-002: All-declared params dict unchanged after validation.

        Given: Schema declares ["namespace", "replicas"]
        When: LLM provides exactly those (no extras)
        Then: params dict is identical to input
        """
        workflow = self.create_workflow_with_params(
            [
                {"name": "namespace", "type": "string", "required": True},
                {"name": "replicas", "type": "int", "required": True},
            ]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        params = {"namespace": "default", "replicas": 3}
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters=params,
        )

        assert result.is_valid is True
        assert params == {"namespace": "default", "replicas": 3}

    def test_mixed_declared_undeclared_only_declared_survive(self, mock_ds_client):
        """
        UT-HAPI-241-003: Mixed declared + undeclared: only declared survive.

        Given: Schema declares ["namespace"] (required, string)
        When: LLM sends namespace plus extra_param and another_extra
        Then: Only namespace remains in params dict
        """
        workflow = self.create_workflow_with_params(
            [
                {"name": "namespace", "type": "string", "required": True},
            ]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        params = {"namespace": "default", "extra_param": "val1", "another_extra": 42}
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters=params,
        )

        assert result.is_valid is True
        assert params == {"namespace": "default"}

    def test_no_schema_strips_all_params(self, mock_ds_client):
        """
        UT-HAPI-241-004: No schema: ALL params stripped.

        Given: Workflow has parameters=None (no schema)
        When: LLM provides arbitrary params
        Then: params dict is empty after validation
        """
        workflow = Mock()
        workflow.workflow_id = "test-workflow"
        workflow.execution_bundle = "ghcr.io/kubernaut/test:v1.0.0"
        workflow.parameters = None
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        params = {"any_param": "value", "another": "thing"}
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters=params,
        )

        assert result.is_valid is True
        assert params == {}

    def test_empty_params_no_error(self, mock_ds_client):
        """
        UT-HAPI-241-005: Empty params dict produces no errors.

        Given: Schema declares ["namespace"] (optional)
        When: LLM provides empty params
        Then: params dict stays empty, no exception
        """
        workflow = self.create_workflow_with_params(
            [
                {"name": "namespace", "type": "string", "required": False},
            ]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        params = {}
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters=params,
        )

        assert result.is_valid is True
        assert params == {}

    def test_credential_hallucination_stripped(self, mock_ds_client):
        """
        UT-HAPI-241-006: Credential-like hallucinated params stripped.

        Given: Schema declares only ["TARGET_NAMESPACE"]
        When: LLM provides TARGET_NAMESPACE plus GIT_PASSWORD, GIT_USERNAME, ADMIN_TOKEN
        Then: Only TARGET_NAMESPACE survives
        """
        workflow = self.create_workflow_with_params(
            [
                {"name": "TARGET_NAMESPACE", "type": "string", "required": True},
            ]
        )
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        params = {
            "TARGET_NAMESPACE": "demo",
            "GIT_PASSWORD": "kubernaut-token",
            "GIT_USERNAME": "kubernaut",
            "ADMIN_TOKEN": "abc",
        }
        result = validator.validate(
            workflow_id="test-workflow",
            execution_bundle=None,
            parameters=params,
        )

        assert result.is_valid is True
        assert params == {"TARGET_NAMESPACE": "demo"}


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
        workflow.execution_bundle = "ghcr.io/kubernaut/correct:v1.0.0"
        workflow.execution_bundle_digest = "sha256:abc123"
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
            execution_bundle="ghcr.io/wrong/image:v1.0.0",  # WRONG
            parameters={},  # Missing required 'namespace'
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

        Given: Valid workflow_id, correct execution_bundle, valid parameters
        When: validate() is called
        Then: Returns is_valid=True with no errors
        """
        # Arrange
        workflow = Mock()
        workflow.workflow_id = "restart-pod-v1"
        workflow.execution_bundle = "ghcr.io/kubernaut/restart:v1.0.0"
        workflow.execution_bundle_digest = "sha256:abc123"
        workflow.parameters = {
            "schema": {
                "parameters": [
                    {"name": "namespace", "type": "string", "required": True},
                    {
                        "name": "delay",
                        "type": "int",
                        "required": False,
                        "minimum": 0,
                        "maximum": 300,
                    },
                ]
            }
        }
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        # Act
        result = validator.validate(
            workflow_id="restart-pod-v1",
            execution_bundle=None,  # Use catalog value
            parameters={"namespace": "production", "delay": 30},
        )

        # Assert
        assert result.is_valid is True
        assert len(result.errors) == 0
        assert result.validated_execution_bundle == "ghcr.io/kubernaut/restart:v1.0.0"


# =============================================================================
# PHASE 6: Action-Type Cross-Check Validation (DD-WORKFLOW-016, Gap 3)
# =============================================================================


class TestActionTypeCrossCheckValidation:
    """
    DD-WORKFLOW-016 Gap 3: Validator cross-checks the workflow's action_type
    against available action types from DS (queried directly).

    The validator queries GET /api/v1/workflows/actions with context filters
    to get available action types, then verifies the selected workflow's
    action_type is in that set.
    """

    @pytest.fixture
    def mock_ds_client(self):
        """Mock Data Storage client."""
        return Mock()

    def test_rejects_workflow_with_unknown_action_type(self, mock_ds_client):
        """
        Gap 3: Workflow with action_type not in available actions is rejected.

        Given: DS returns ["RestartPod", "ScaleReplicas"] as available actions
        And: LLM selected a workflow with action_type="CordonNode"
        When: validate() is called
        Then: Returns error about action_type not in available actions
        """
        workflow = Mock()
        workflow.workflow_id = "wf-001"
        workflow.action_type = "CordonNode"
        workflow.execution_bundle = "ghcr.io/kubernaut/cordon:v1.0.0"
        workflow.parameters = None
        mock_ds_client.get_workflow_by_id.return_value = workflow

        # DS returns available action types (without CordonNode)
        mock_ds_client.list_available_actions.return_value = {
            "action_types": [
                {"action_type": "RestartPod", "workflow_count": 3},
                {"action_type": "ScaleReplicas", "workflow_count": 2},
            ]
        }

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(
            mock_ds_client,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = validator.validate(
            workflow_id="wf-001",
            execution_bundle=None,
            parameters={},
        )

        assert result.is_valid is False
        assert any(
            "action_type" in e.lower() and "CordonNode" in e for e in result.errors
        ), f"Expected action_type cross-check error, got: {result.errors}"

    def test_passes_workflow_with_valid_action_type(self, mock_ds_client):
        """
        Gap 3: Workflow with action_type in available actions is accepted.

        Given: DS returns ["RestartPod", "ScaleReplicas"] as available actions
        And: LLM selected a workflow with action_type="RestartPod"
        When: validate() is called
        Then: No action_type cross-check error
        """
        workflow = Mock()
        workflow.workflow_id = "wf-001"
        workflow.action_type = "RestartPod"
        workflow.execution_bundle = "ghcr.io/kubernaut/restart:v1.0.0"
        workflow.parameters = None
        mock_ds_client.get_workflow_by_id.return_value = workflow

        mock_ds_client.list_available_actions.return_value = {
            "action_types": [
                {"action_type": "RestartPod", "workflow_count": 3},
                {"action_type": "ScaleReplicas", "workflow_count": 2},
            ]
        }

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(
            mock_ds_client,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = validator.validate(
            workflow_id="wf-001",
            execution_bundle=None,
            parameters={},
        )

        assert result.is_valid is True

    def test_skips_crosscheck_when_no_context_filters(self, mock_ds_client):
        """
        Gap 3: Cross-check is skipped when no context filters are set.

        Given: Validator constructed without context filters
        When: validate() is called
        Then: Cross-check is not performed (list_available_actions not called)
        """
        workflow = Mock()
        workflow.workflow_id = "wf-001"
        workflow.action_type = "AnyAction"
        workflow.execution_bundle = "ghcr.io/kubernaut/any:v1.0.0"
        workflow.parameters = None
        mock_ds_client.get_workflow_by_id.return_value = workflow

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        # No context filters
        validator = WorkflowResponseValidator(mock_ds_client)

        result = validator.validate(
            workflow_id="wf-001",
            execution_bundle=None,
            parameters={},
        )

        assert result.is_valid is True
        # list_available_actions should NOT have been called
        mock_ds_client.list_available_actions.assert_not_called()

    def test_crosscheck_graceful_on_ds_error(self, mock_ds_client):
        """
        Gap 3: Cross-check fails gracefully if DS call errors.

        Given: list_available_actions raises an exception
        When: validate() is called
        Then: Validation continues without action_type error (graceful degradation)
        """
        workflow = Mock()
        workflow.workflow_id = "wf-001"
        workflow.action_type = "RestartPod"
        workflow.execution_bundle = "ghcr.io/kubernaut/restart:v1.0.0"
        workflow.parameters = None
        mock_ds_client.get_workflow_by_id.return_value = workflow
        mock_ds_client.list_available_actions.side_effect = Exception(
            "Connection refused"
        )

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(
            mock_ds_client,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = validator.validate(
            workflow_id="wf-001",
            execution_bundle=None,
            parameters={},
        )

        # Should be valid -- graceful degradation on DS error
        assert result.is_valid is True
