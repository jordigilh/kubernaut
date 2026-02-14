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
Integration Tests: Workflow Validation Security Gate (DD-HAPI-017)

Business Requirements:
- BR-HAPI-017-003: Context filter security gate

Design Decisions:
- DD-WORKFLOW-016: Action-Type Workflow Catalog Indexing
- DD-HAPI-017: Three-Step Workflow Discovery Integration

Test IDs: IT-HAPI-017-003-001, IT-HAPI-017-003-002

Prerequisites:
    Real Data Storage with PostgreSQL (started by Go infrastructure).
    Migration 025 applied (action_type_taxonomy seeded).

Run:
    python -m pytest tests/integration/test_workflow_validation_integration.py -v
"""

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
    """
    Bootstrap test workflows for validation tests.
    Returns results with workflow_id_map for known IDs.
    """
    taxonomy = bootstrap_action_type_taxonomy(data_storage_url)
    assert taxonomy["available"], "Taxonomy not available"

    results = bootstrap_workflows(data_storage_url, workflows=TEST_WORKFLOWS)
    total_ok = len(results["created"]) + len(results["existing"])
    assert total_ok > 0, f"No workflows seeded: {results}"
    return results


@pytest.fixture(scope="module")
def known_workflow_id(data_storage_url, seeded_workflows):
    """
    Discover a known workflow_id from DS for validation tests.

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
# IT-HAPI-017-003-001: Security gate -- mismatched context returns 404
# ========================================

@pytest.mark.requires_data_storage
class TestSecurityGateMismatch:
    """
    BR-HAPI-017-003: GetWorkflow with context filters that don't match
    the workflow returns 404 (security gate).

    IT-HAPI-017-003-001: Security gate -- mismatched context returns 404.
    """

    def test_mismatched_context_returns_validation_failure(
        self, data_storage_url, known_workflow_id
    ):
        """
        IT-HAPI-017-003-001: Validator treats 404 from DS security gate
        as a context mismatch validation failure.
        """
        # ARRANGE — Create validator with MISMATCHED context
        # The workflow is severity=critical, environment=production
        # We pass severity=warning, environment=staging (mismatch)
        api_client, discovery_api = create_authenticated_datastorage_client(
            data_storage_url, api_type="discovery"
        )

        with api_client:
            validator = WorkflowResponseValidator(
                data_storage_client=discovery_api,
                severity="warning",
                component="statefulset",
                environment="staging",
                priority="P3",
            )

            # ACT
            result = validator.validate(
                workflow_id=known_workflow_id,
                container_image=None,
                parameters={},
            )

            # ASSERT
            assert not result.is_valid, "Expected validation to fail on context mismatch"
            assert len(result.errors) >= 1, "Expected at least 1 error"

            # The error message should indicate context mismatch
            error_text = " ".join(result.errors).lower()
            assert "context" in error_text or "not match" in error_text or "not found" in error_text, (
                f"Expected context mismatch error, got: {result.errors}"
            )


# ========================================
# IT-HAPI-017-003-002: Security gate -- matching context returns workflow
# ========================================

@pytest.mark.requires_data_storage
class TestSecurityGateMatch:
    """
    BR-HAPI-017-003: GetWorkflow with matching context returns full
    workflow detail (security gate allows the request).

    IT-HAPI-017-003-002: Security gate -- matching context returns workflow.
    """

    def test_matching_context_returns_valid_workflow(
        self, data_storage_url, known_workflow_id
    ):
        """
        IT-HAPI-017-003-002: Validator passes when context filters match
        the workflow's label context.
        """
        # ARRANGE — Create validator with MATCHING context
        # DD-WORKFLOW-016: Go seeds oomkill-increase-memory-v1 with
        # severity=critical, component=pod, environment=production,
        # priority=P0, action_type=IncreaseMemoryLimits
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

            # ACT
            # Note: Go-seeded workflows default to TARGET_RESOURCE as the only
            # required parameter (buildWorkflowSchemaContent in workflow_seeding.go)
            result = validator.validate(
                workflow_id=known_workflow_id,
                container_image=None,  # Skip image validation for this test
                parameters={"TARGET_RESOURCE": "my-deployment"},
            )

            # ASSERT
            assert result.is_valid, (
                f"Expected validation to pass with matching context, "
                f"but got errors: {result.errors}"
            )
            assert result.validated_container_image is not None, (
                "Expected container image from catalog"
            )
