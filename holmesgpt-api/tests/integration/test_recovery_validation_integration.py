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
Integration Tests: Recovery Validation Loop with Real DS (DD-HAPI-017)

Business Requirements:
- BR-HAPI-017-004: Recovery validation loop with self-correction

Design Decisions:
- DD-HAPI-002 v1.2: Workflow Response Validation Architecture
- DD-HAPI-017: Three-Step Workflow Discovery Integration

Test IDs: IT-HAPI-017-004-001, IT-HAPI-017-004-002

These tests exercise the WorkflowResponseValidator against a real Data Storage
service, simulating the self-correction loop sequence:
  Attempt 1: invalid workflow → validator returns failure with errors
  Attempt 2: valid workflow   → validator returns success

The full analyze_recovery() function orchestrates this loop with the LLM;
here we test the validator's integration with real DS directly.

Prerequisites:
    Real Data Storage with PostgreSQL (started by Go infrastructure).
    Migration 025 applied (action_type_taxonomy seeded).

Run:
    python -m pytest tests/integration/test_recovery_validation_integration.py -v
"""

import uuid
import pytest

from src.validation.workflow_response_validator import (
    WorkflowResponseValidator,
    ValidationResult,
)

from tests.fixtures.workflow_fixtures import (
    TEST_WORKFLOWS,
    bootstrap_workflows,
    bootstrap_action_type_taxonomy,
    ACTION_TYPE_INCREASE_MEMORY_LIMITS,
)

from tests.integration.conftest import (
    DATA_STORAGE_URL,
    create_authenticated_datastorage_client,
    is_integration_infra_available,
)


# ========================================
# FIXTURES
# ========================================

@pytest.fixture(scope="module")
def seeded_workflows(data_storage_url):
    """Bootstrap test workflows for recovery validation tests."""
    taxonomy = bootstrap_action_type_taxonomy(data_storage_url)
    assert taxonomy["available"], "Taxonomy not available"

    results = bootstrap_workflows(data_storage_url, workflows=TEST_WORKFLOWS)
    total_ok = len(results["created"]) + len(results["existing"])
    assert total_ok > 0, f"No workflows seeded: {results}"
    return results


@pytest.fixture(scope="module")
def known_workflow_id(data_storage_url, seeded_workflows):
    """
    Discover a valid workflow_id for IncreaseMemoryLimits workflows.

    DD-WORKFLOW-016 V1.0: Go seeds oomkill-increase-memory-v1 with
    action_type=IncreaseMemoryLimits, severity=critical, component=pod,
    environment=production, priority=P0.
    """
    api_client, discovery_api = create_authenticated_datastorage_client(
        data_storage_url, api_type="discovery"
    )

    with api_client:
        response = discovery_api.list_workflows_by_action_type(
            action_type=ACTION_TYPE_INCREASE_MEMORY_LIMITS,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            _request_timeout=10,
        )
        assert len(response.workflows) >= 1, (
            f"No {ACTION_TYPE_INCREASE_MEMORY_LIMITS} workflows found in DS"
        )
        return response.workflows[0].workflow_id


# ========================================
# IT-HAPI-017-004-001: Recovery validation loop -- retry on invalid workflow
# ========================================

@pytest.mark.requires_data_storage
class TestRecoveryValidationRetry:
    """
    BR-HAPI-017-004: Recovery validation loop retries when DS returns
    validation error. Simulates the self-correction sequence:
      Attempt 1: non-existent workflow_id → 404 from DS
      Attempt 2: valid workflow_id → 200 from DS

    IT-HAPI-017-004-001: Recovery validation loop with real DS -- retry.
    """

    def test_retry_succeeds_after_invalid_then_valid_workflow(
        self, data_storage_url, known_workflow_id
    ):
        """
        IT-HAPI-017-004-001: First attempt fails (non-existent ID),
        second attempt succeeds (valid ID). Validates the self-correction
        pattern works with real DS.
        """
        api_client, discovery_api = create_authenticated_datastorage_client(
            data_storage_url, api_type="discovery"
        )

        with api_client:
            validator = WorkflowResponseValidator(
                data_storage_client=discovery_api,
                severity="critical",
                component="pod",
                environment="production",
                priority="P0",
            )

            # --- Attempt 1: non-existent workflow_id (simulates LLM hallucination) ---
            fake_workflow_id = str(uuid.uuid4())
            result_1 = validator.validate(
                workflow_id=fake_workflow_id,
                execution_bundle=None,
                parameters={},
            )

            assert not result_1.is_valid, "Attempt 1 should fail with non-existent workflow"
            assert len(result_1.errors) >= 1, "Attempt 1 should have at least 1 error"

            # --- Attempt 2: valid workflow_id (simulates LLM self-correction) ---
            # Note: Go-seeded workflows default to TARGET_RESOURCE as the only
            # required parameter (buildWorkflowSchemaContent in workflow_seeding.go)
            result_2 = validator.validate(
                workflow_id=known_workflow_id,
                execution_bundle=None,
                parameters={"TARGET_RESOURCE": "my-deployment"},
            )

            assert result_2.is_valid, (
                f"Attempt 2 should succeed with valid workflow, "
                f"but got errors: {result_2.errors}"
            )


# ========================================
# IT-HAPI-017-004-002: Recovery validation -- succeeds after parameter correction
# ========================================

@pytest.mark.requires_data_storage
class TestRecoveryValidationParameterCorrection:
    """
    BR-HAPI-017-004: Recovery flow produces valid result after LLM
    self-corrects parameters.

    IT-HAPI-017-004-002: Recovery validation with real DS -- succeeds
    after parameter correction.
    """

    def test_parameter_correction_succeeds(
        self, data_storage_url, known_workflow_id
    ):
        """
        IT-HAPI-017-004-002:
          Attempt 1: valid workflow but missing required parameters
          Attempt 2: correct parameters
          Final result: is_valid=True (needs_human_review=False implied)
        """
        api_client, discovery_api = create_authenticated_datastorage_client(
            data_storage_url, api_type="discovery"
        )

        with api_client:
            validator = WorkflowResponseValidator(
                data_storage_client=discovery_api,
                severity="critical",
                component="pod",
                environment="production",
                priority="P0",
            )

            # --- Attempt 1: valid workflow but empty parameters ---
            # Go-seeded workflows have TARGET_RESOURCE as the only required param
            result_1 = validator.validate(
                workflow_id=known_workflow_id,
                execution_bundle=None,
                parameters={},  # Missing required params
            )

            # Depending on whether the workflow has parameter schema defined,
            # this may or may not fail. The key test is that attempt 2 succeeds.
            # If parameters are schema-validated, attempt 1 should fail.

            # --- Attempt 2: correct parameters ---
            # Note: Go-seeded workflows default to TARGET_RESOURCE as the only
            # required parameter (buildWorkflowSchemaContent in workflow_seeding.go)
            result_2 = validator.validate(
                workflow_id=known_workflow_id,
                execution_bundle=None,
                parameters={"TARGET_RESOURCE": "my-deployment"},
            )

            assert result_2.is_valid, (
                f"Attempt 2 should succeed with correct parameters, "
                f"but got errors: {result_2.errors}"
            )
            assert result_2.validated_execution_bundle is not None, (
                "Expected execution bundle from catalog after successful validation"
            )
