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
Context Filter Validation Tests (DD-HAPI-017)

Business Requirement: BR-HAPI-017-003 (Validator with Context Filters)
Design Decisions:
- DD-HAPI-017: Three-Step Workflow Discovery Integration
- DD-HAPI-002 v1.2: Workflow Response Validation Architecture

Tests validate BUSINESS OUTCOMES:
- Validator passes signal context filters to DS security gate
- DS 404 (security gate rejection) treated as validation failure
- Error messages guide LLM to select a different workflow
- Happy path: matching context returns valid workflow for parameter checks

Test IDs: UT-HAPI-017-003-001 through UT-HAPI-017-003-004
"""

from unittest.mock import Mock


# Simulate DS client's NotFoundException (status=404) without importing generated client
class _MockNotFoundException(Exception):
    """Simulates datastorage.exceptions.NotFoundException."""
    def __init__(self):
        self.status = 404
        super().__init__("Not Found")


# ============================================================
# UT-HAPI-017-003-001: Validator passes context filters to get_workflow call
# ============================================================

class TestValidatorPassesContextFilters:
    """
    UT-HAPI-017-003-001: Validator passes context filters to get_workflow call

    Business Outcome: When context filters are provided, the validator
    forwards them to the DS client's get_workflow_by_id call so the
    DS security gate can evaluate workflow compatibility.

    BR: BR-HAPI-017-003
    DD: DD-HAPI-017, DD-HAPI-002 v1.2
    """

    def test_context_filters_forwarded_to_ds_client(self):
        """DS client receives severity, component, environment, priority as kwargs."""
        from src.validation.workflow_response_validator import WorkflowResponseValidator

        mock_ds_client = Mock()
        mock_workflow = Mock()
        mock_workflow.execution_bundle = "quay.io/kubernaut-ai/test:v1"
        mock_workflow.parameters = {"schema": {"parameters": []}}
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow

        validator = WorkflowResponseValidator(
            mock_ds_client,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        validator.validate(
            workflow_id="uuid-1",
            execution_bundle="quay.io/kubernaut-ai/test:v1",
            parameters={},
        )

        # Assert DS client called with workflow_id AND all context filters
        mock_ds_client.get_workflow_by_id.assert_called_once_with(
            "uuid-1",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

    def test_no_context_filters_calls_without_kwargs(self):
        """Without context filters, DS client called with workflow_id only (backward compat)."""
        from src.validation.workflow_response_validator import WorkflowResponseValidator

        mock_ds_client = Mock()
        mock_workflow = Mock()
        mock_workflow.execution_bundle = "quay.io/kubernaut-ai/test:v1"
        mock_workflow.parameters = {"schema": {"parameters": []}}
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow

        validator = WorkflowResponseValidator(mock_ds_client)

        validator.validate(
            workflow_id="uuid-1",
            execution_bundle=None,
            parameters={},
        )

        # Assert DS client called with workflow_id only (no extra kwargs)
        mock_ds_client.get_workflow_by_id.assert_called_once_with("uuid-1")


# ============================================================
# UT-HAPI-017-003-002: Validator treats 404 as validation failure
# ============================================================

class TestValidatorTreats404AsFailure:
    """
    UT-HAPI-017-003-002: Validator treats 404 as validation failure

    Business Outcome: When DS returns 404 (security gate rejection),
    the validator marks validation as failed, preventing the LLM from
    selecting a workflow incompatible with the signal context.

    BR: BR-HAPI-017-003
    DD: DD-HAPI-017
    """

    def test_404_results_in_invalid_validation(self):
        """DS 404 (security gate) → ValidationResult.is_valid=False."""
        from src.validation.workflow_response_validator import WorkflowResponseValidator

        mock_ds_client = Mock()
        mock_ds_client.get_workflow_by_id.side_effect = _MockNotFoundException()

        validator = WorkflowResponseValidator(
            mock_ds_client,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = validator.validate(
            workflow_id="uuid-1",
            execution_bundle=None,
            parameters={},
        )

        assert result.is_valid is False
        assert len(result.errors) >= 1

    def test_404_without_context_filters_is_regular_not_found(self):
        """Without context filters, 404 is treated as regular 'not found' (backward compat)."""
        from src.validation.workflow_response_validator import WorkflowResponseValidator

        mock_ds_client = Mock()
        mock_ds_client.get_workflow_by_id.side_effect = _MockNotFoundException()

        validator = WorkflowResponseValidator(mock_ds_client)

        result = validator.validate(
            workflow_id="uuid-1",
            execution_bundle=None,
            parameters={},
        )

        assert result.is_valid is False
        # Without context filters, error should say "not found in catalog" (original behavior)
        error_text = " ".join(result.errors).lower()
        assert "not found" in error_text


# ============================================================
# UT-HAPI-017-003-003: Validator error message includes context mismatch detail
# ============================================================

class TestValidatorContextMismatchErrorMessage:
    """
    UT-HAPI-017-003-003: Validator error message includes context mismatch detail

    Business Outcome: When the security gate rejects a workflow, the error
    message is actionable — it tells the LLM which workflow failed and
    that it should select a different one via the discovery tools.

    BR: BR-HAPI-017-003
    DD: DD-HAPI-017
    """

    def test_error_message_contains_workflow_id(self):
        """Error message mentions the rejected workflow ID."""
        from src.validation.workflow_response_validator import WorkflowResponseValidator

        mock_ds_client = Mock()
        mock_ds_client.get_workflow_by_id.side_effect = _MockNotFoundException()

        validator = WorkflowResponseValidator(
            mock_ds_client,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = validator.validate(
            workflow_id="uuid-rejected-001",
            execution_bundle=None,
            parameters={},
        )

        error_text = " ".join(result.errors)
        assert "uuid-rejected-001" in error_text

    def test_error_message_guides_to_select_different_workflow(self):
        """Error message tells LLM to select a different workflow."""
        from src.validation.workflow_response_validator import WorkflowResponseValidator

        mock_ds_client = Mock()
        mock_ds_client.get_workflow_by_id.side_effect = _MockNotFoundException()

        validator = WorkflowResponseValidator(
            mock_ds_client,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = validator.validate(
            workflow_id="uuid-rejected-001",
            execution_bundle=None,
            parameters={},
        )

        error_text = " ".join(result.errors).lower()
        assert "different workflow" in error_text

    def test_error_message_mentions_context_mismatch(self):
        """Error message indicates this is a context mismatch, not just 'not found'."""
        from src.validation.workflow_response_validator import WorkflowResponseValidator

        mock_ds_client = Mock()
        mock_ds_client.get_workflow_by_id.side_effect = _MockNotFoundException()

        validator = WorkflowResponseValidator(
            mock_ds_client,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = validator.validate(
            workflow_id="uuid-rejected-001",
            execution_bundle=None,
            parameters={},
        )

        error_text = " ".join(result.errors).lower()
        # Must indicate context/signal mismatch, not just "not found in catalog"
        assert "context" in error_text or "signal" in error_text


# ============================================================
# UT-HAPI-017-003-004: Validator happy path with matching context
# ============================================================

class TestValidatorHappyPathWithContext:
    """
    UT-HAPI-017-003-004: Validator happy path with matching context

    Business Outcome: When DS returns 200 (workflow matches context),
    the existing parameter schema validation proceeds normally.

    BR: BR-HAPI-017-003
    DD: DD-HAPI-017, DD-HAPI-002 v1.2
    """

    def test_matching_context_returns_valid_result(self):
        """Workflow matching context → is_valid=True with parameter validation."""
        from src.validation.workflow_response_validator import WorkflowResponseValidator

        mock_ds_client = Mock()
        mock_workflow = Mock()
        mock_workflow.workflow_id = "uuid-1"
        mock_workflow.execution_bundle = "quay.io/kubernaut-ai/scale:v1.0.0"
        mock_workflow.action_type = "ScaleReplicas"
        mock_workflow.parameters = {
            "schema": {
                "parameters": [
                    {"name": "replicas", "type": "int", "required": True, "minimum": 1, "maximum": 10}
                ]
            }
        }
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow
        # Gap 3: cross-check expects list_available_actions to return matching action_type
        mock_ds_client.list_available_actions.return_value = {
            "action_types": [{"action_type": "ScaleReplicas"}]
        }

        validator = WorkflowResponseValidator(
            mock_ds_client,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = validator.validate(
            workflow_id="uuid-1",
            execution_bundle=None,
            parameters={"replicas": 3},
        )

        assert result.is_valid is True
        assert len(result.errors) == 0
        assert result.validated_execution_bundle == "quay.io/kubernaut-ai/scale:v1.0.0"

    def test_matching_context_still_validates_parameters(self):
        """Context match + invalid parameters → is_valid=False with param errors."""
        from src.validation.workflow_response_validator import WorkflowResponseValidator

        mock_ds_client = Mock()
        mock_workflow = Mock()
        mock_workflow.workflow_id = "uuid-1"
        mock_workflow.execution_bundle = "quay.io/kubernaut-ai/scale:v1.0.0"
        mock_workflow.action_type = "ScaleReplicas"
        mock_workflow.parameters = {
            "schema": {
                "parameters": [
                    {"name": "replicas", "type": "int", "required": True, "minimum": 1, "maximum": 10}
                ]
            }
        }
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow
        # Gap 3: cross-check expects list_available_actions to return matching action_type
        mock_ds_client.list_available_actions.return_value = {
            "action_types": [{"action_type": "ScaleReplicas"}]
        }

        validator = WorkflowResponseValidator(
            mock_ds_client,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = validator.validate(
            workflow_id="uuid-1",
            execution_bundle=None,
            parameters={},  # Missing required 'replicas'
        )

        assert result.is_valid is False
        error_text = " ".join(result.errors).lower()
        assert "required" in error_text
        assert "replicas" in error_text
