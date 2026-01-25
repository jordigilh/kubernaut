"""
Unit tests for container_image and container_digest fields in workflow catalog tool

Business Requirement: BR-AI-075 - Workflow Selection Contract
Design Decision: DD-WORKFLOW-002 v2.4, DD-CONTRACT-001 v1.2
Authority: AIAnalysis crd-schema.md - SelectedWorkflow.containerImage field

TDD Phase: RED (failing tests - implementation not yet done)

⚠️ CRITICAL: DD-CONTRACT-001 v1.2 Architecture Change
- HolmesGPT API must return container_image and container_digest in search results
- These fields are resolved by Data Storage Service from workflow catalog
- AIAnalysis controller passes these through to RemediationOrchestrator
- WorkflowExecution uses container_image to create Tekton PipelineRun

Test Coverage:
- Unit Test 1-2: _transform_api_response extracts container_image/digest
- Unit Test 3-4: Graceful handling when fields are missing
- Unit Test 5-6: Search result JSON includes the fields
- Unit Test 7-8: Format validation for OCI reference and sha256 digest
"""

import pytest
import json
from unittest.mock import patch


class TestWorkflowCatalogContainerImage:
    """
    Unit tests for container_image and container_digest propagation

    Business Requirement: BR-AI-075 - Workflow Selection Contract
    Design Decision: DD-WORKFLOW-002 v2.4 - search_workflow_catalog returns container_image
    Design Decision: DD-CONTRACT-001 v1.2 - AIAnalysis includes container_image in status

    Test Strategy:
    - Test _transform_api_response extracts container_image and container_digest
    - Test graceful handling when fields are missing (backward compatibility)
    - Test search result JSON includes the fields
    - Test format validation for OCI reference and sha256 digest
    """

    @pytest.fixture
    def mock_api_response_with_container_image(self):
        """
        Mock Data Storage API response WITH container_image and container_digest

        Per DD-WORKFLOW-002 v3.0: FLAT response structure (no nested 'workflow' object)
        Per DD-WORKFLOW-009: Workflow catalog stores container_image and container_digest
        """
        return [
            {
                # DD-WORKFLOW-002 v3.0: Flat structure
                "workflow_id": "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d",
                "title": "OOM Remediation",
                "description": "Remediate OOMKilled pods",
                "signal_type": "OOMKilled",
                "container_image": "quay.io/kubernaut/workflow-oomkill:v1.0.0",
                "container_digest": "sha256:abc123def456789012345678901234567890123456789012345678901234abcd",
                "confidence": 0.92
            },
            {
                "workflow_id": "2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e",
                "title": "CrashLoop Fix",
                "description": "Fix CrashLoopBackOff issues",
                "signal_type": "CrashLoopBackOff",
                "container_image": "quay.io/kubernaut/workflow-crashloop:v2.1.0",
                "container_digest": "sha256:def456abc789012345678901234567890123456789012345678901234567ef01",
                "confidence": 0.88
            }
        ]

    @pytest.fixture
    def mock_api_response_without_container_image(self):
        """
        Mock Data Storage API response WITHOUT container_image (legacy format)

        Backward compatibility: Old workflows may not have these fields
        DD-WORKFLOW-002 v3.0: Flat structure
        """
        return [
            {
                # DD-WORKFLOW-002 v3.0: Flat structure
                "workflow_id": "3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f",
                "title": "Legacy Workflow",
                "description": "Old workflow without container_image",
                "signal_type": "OOMKilled",
                "confidence": 0.85
                # NOTE: NO container_image or container_digest
            }
        ]

    @pytest.fixture
    def mock_data_storage_search_response(self, mock_api_response_with_container_image):
        """Mock full Data Storage search response"""
        return {
            "workflows": mock_api_response_with_container_image,
            "total_results": len(mock_api_response_with_container_image)
        }

    # ========================================
    # UNIT TEST 1: container_image extraction
    # ========================================

    def test_transform_includes_container_image_when_present(
        self, mock_api_response_with_container_image
    ):
        """
        BR-AI-075: container_image is extracted from Data Storage response

        BEHAVIOR: _transform_api_response extracts container_image field
        CORRECTNESS: Transformed result contains exact value from API response

        TDD Phase: RED (this test should FAIL initially)
        Expected failure: KeyError or assertion failure on missing field
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

        # ARRANGE
        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        # ACT
        result = tool._transform_api_response(mock_api_response_with_container_image)

        # ASSERT BEHAVIOR: Field extracted and present in result
        assert len(result) == 2, "Should transform both workflows"
        assert "container_image" in result[0], \
            "BR-AI-075: First workflow must have container_image field"
        assert "container_image" in result[1], \
            "BR-AI-075: Second workflow must have container_image field"

        # ASSERT CORRECTNESS: Values match API response exactly
        assert result[0]["container_image"] == "quay.io/kubernaut/workflow-oomkill:v1.0.0", \
            f"BR-AI-075: Expected 'quay.io/kubernaut/workflow-oomkill:v1.0.0', got '{result[0].get('container_image')}'"
        assert result[1]["container_image"] == "quay.io/kubernaut/workflow-crashloop:v2.1.0", \
            f"BR-AI-075: Expected 'quay.io/kubernaut/workflow-crashloop:v2.1.0', got '{result[1].get('container_image')}'"

    # ========================================
    # UNIT TEST 2: container_digest extraction
    # ========================================

    def test_transform_includes_container_digest_when_present(
        self, mock_api_response_with_container_image
    ):
        """
        BR-AI-075: container_digest is extracted from Data Storage response

        BEHAVIOR: _transform_api_response extracts container_digest field
        CORRECTNESS: Transformed result contains exact value from API response

        TDD Phase: RED (this test should FAIL initially)
        Expected failure: KeyError or assertion failure on missing field
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

        # ARRANGE
        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        # ACT
        result = tool._transform_api_response(mock_api_response_with_container_image)

        # ASSERT BEHAVIOR: Field extracted and present in result
        assert "container_digest" in result[0], \
            "BR-AI-075: First workflow must have container_digest field"
        assert "container_digest" in result[1], \
            "BR-AI-075: Second workflow must have container_digest field"

        # ASSERT CORRECTNESS: Values match API response exactly
        expected_digest_1 = "sha256:abc123def456789012345678901234567890123456789012345678901234abcd"
        expected_digest_2 = "sha256:def456abc789012345678901234567890123456789012345678901234567ef01"

        assert result[0]["container_digest"] == expected_digest_1, \
            f"BR-AI-075: First digest mismatch - expected '{expected_digest_1}', got '{result[0].get('container_digest')}'"
        assert result[1]["container_digest"] == expected_digest_2, \
            f"BR-AI-075: Second digest mismatch - expected '{expected_digest_2}', got '{result[1].get('container_digest')}'"

    # ========================================
    # UNIT TEST 3: Missing container_image handling
    # ========================================

    def test_transform_handles_missing_container_image_gracefully(
        self, mock_api_response_without_container_image
    ):
        """
        BR-AI-075: Graceful handling when container_image is missing

        BEHAVIOR: No crash when field is absent (backward compatibility)
        CORRECTNESS: Returns None or empty string for missing field

        TDD Phase: RED (this test should FAIL initially)
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

        # ARRANGE
        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        # ACT: Should not raise exception
        result = tool._transform_api_response(mock_api_response_without_container_image)

        # ASSERT BEHAVIOR: No crash, result returned
        assert len(result) == 1, "Should transform the workflow"

        # ASSERT BEHAVIOR: Field is present (even if None/empty)
        assert "container_image" in result[0], \
            "BR-AI-075: container_image field must be present (even if None)"

        # ASSERT CORRECTNESS: None or empty for missing field
        container_image = result[0]["container_image"]
        assert container_image is None or container_image == "", \
            f"BR-AI-075: Missing container_image should be None or empty, got '{container_image}'"

    # ========================================
    # UNIT TEST 4: Missing container_digest handling
    # ========================================

    def test_transform_handles_missing_container_digest_gracefully(
        self, mock_api_response_without_container_image
    ):
        """
        BR-AI-075: Graceful handling when container_digest is missing

        BEHAVIOR: No crash when field is absent (backward compatibility)
        CORRECTNESS: Returns None or empty string for missing field

        TDD Phase: RED (this test should FAIL initially)
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

        # ARRANGE
        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        # ACT: Should not raise exception
        result = tool._transform_api_response(mock_api_response_without_container_image)

        # ASSERT BEHAVIOR: Field is present (even if None/empty)
        assert "container_digest" in result[0], \
            "BR-AI-075: container_digest field must be present (even if None)"

        # ASSERT CORRECTNESS: None or empty for missing field
        container_digest = result[0]["container_digest"]
        assert container_digest is None or container_digest == "", \
            f"BR-AI-075: Missing container_digest should be None or empty, got '{container_digest}'"

    # ========================================
    # UNIT TEST 5: Search result JSON contains container_image
    # ========================================

    def test_search_result_json_contains_container_image(
        self, mock_data_storage_search_response
    ):
        """
        BR-AI-075: Tool result JSON includes container_image field

        BEHAVIOR: Final tool result (JSON) includes container_image
        CORRECTNESS: JSON key exists and value is correct

        TDD Phase: RED (this test should FAIL initially)
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool
        from holmes.core.tools import StructuredToolResultStatus

        # ARRANGE
        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        with patch('datastorage.api.workflow_catalog_api_api.WorkflowCatalogAPIApi.search_workflows') as mock_search:
            # Mock OpenAPI response
            from datastorage.models.workflow_search_response import WorkflowSearchResponse
            from datastorage.models.workflow_search_result import WorkflowSearchResult
            from uuid import UUID

            mock_workflow = WorkflowSearchResult(
                workflow_id=str(UUID("1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d")),
                title="OOM Remediation",
                description="Remediate OOMKilled pods",
                signal_type="OOMKilled",
                confidence=0.92,
                final_score=0.92,
                rank=1,
                container_image="quay.io/kubernaut/workflow-oomkill:v1.0.0",
                container_digest="sha256:abc123def456789012345678901234567890123456789012345678901234abcd"
            )
            mock_response = WorkflowSearchResponse(workflows=[mock_workflow], total_results=1)
            mock_search.return_value = mock_response

            # ACT
            result = tool._invoke(params={
                "query": "OOMKilled critical",
                "filters": {},
                "top_k": 3
            })

            # ASSERT BEHAVIOR: Successful search
            assert result.status == StructuredToolResultStatus.SUCCESS, \
                f"BR-AI-075: Search must succeed, got error: {result.error}"

            # ASSERT CORRECTNESS: JSON contains container_image
            data = json.loads(result.data)
            workflows = data.get("workflows", [])

            assert len(workflows) > 0, "BR-AI-075: Must return workflows"

            for wf in workflows:
                assert "container_image" in wf, \
                    f"BR-AI-075: Workflow {wf.get('workflow_id')} missing container_image in JSON output"
                assert wf["container_image"] is not None, \
                    f"BR-AI-075: container_image must not be None for {wf.get('workflow_id')}"

    # ========================================
    # UNIT TEST 6: Search result JSON contains container_digest
    # ========================================

    def test_search_result_json_contains_container_digest(
        self, mock_data_storage_search_response
    ):
        """
        BR-AI-075: Tool result JSON includes container_digest field

        BEHAVIOR: Final tool result (JSON) includes container_digest
        CORRECTNESS: JSON key exists and value is correct

        TDD Phase: RED (this test should FAIL initially)
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

        # ARRANGE
        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        with patch('datastorage.api.workflow_catalog_api_api.WorkflowCatalogAPIApi.search_workflows') as mock_search:
            # Mock OpenAPI response
            from datastorage.models.workflow_search_response import WorkflowSearchResponse
            from datastorage.models.workflow_search_result import WorkflowSearchResult
            from uuid import UUID

            mock_workflow = WorkflowSearchResult(
                workflow_id=str(UUID("2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e")),
                title="CrashLoop Fix",
                description="Fix CrashLoopBackOff issues",
                signal_type="CrashLoopBackOff",
                confidence=0.88,
                final_score=0.88,
                rank=1,
                container_image="quay.io/kubernaut/workflow-crashloop:v2.1.0",
                container_digest="sha256:def456abc789012345678901234567890123456789012345678901234567ef01"
            )
            mock_response = WorkflowSearchResponse(workflows=[mock_workflow], total_results=1)
            mock_search.return_value = mock_response

            # ACT
            result = tool._invoke(params={
                "query": "OOMKilled critical",
                "filters": {},
                "top_k": 3
            })

            # ASSERT CORRECTNESS: JSON contains container_digest
            data = json.loads(result.data)
            workflows = data.get("workflows", [])

            for wf in workflows:
                assert "container_digest" in wf, \
                    f"BR-AI-075: Workflow {wf.get('workflow_id')} missing container_digest in JSON output"
                assert wf["container_digest"] is not None, \
                    f"BR-AI-075: container_digest must not be None for {wf.get('workflow_id')}"

    # ========================================
    # UNIT TEST 7: container_image format validation
    # ========================================

    def test_container_image_format_is_valid_oci_reference(
        self, mock_api_response_with_container_image
    ):
        """
        BR-AI-075: container_image format is valid OCI reference

        BEHAVIOR: Accepts valid OCI image reference format
        CORRECTNESS: Format matches 'registry/repo:tag' or 'registry/repo@sha256:...'

        TDD Phase: RED (this test should FAIL initially)
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

        # ARRANGE
        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        # ACT
        result = tool._transform_api_response(mock_api_response_with_container_image)

        # ASSERT CORRECTNESS: OCI reference format validation
        for wf in result:
            container_image = wf.get("container_image")
            if container_image:
                # Valid OCI format: registry/path:tag or registry/path@digest
                assert "/" in container_image, \
                    f"BR-AI-075: container_image must contain registry path, got '{container_image}'"
                # Should have either :tag or @sha256:
                has_tag = ":" in container_image.split("/")[-1]
                has_digest = "@sha256:" in container_image
                assert has_tag or has_digest, \
                    f"BR-AI-075: container_image must have :tag or @sha256:, got '{container_image}'"

    # ========================================
    # UNIT TEST 8: container_digest format validation
    # ========================================

    def test_container_digest_format_is_valid_sha256(
        self, mock_api_response_with_container_image
    ):
        """
        BR-AI-075: container_digest format is valid sha256 digest

        BEHAVIOR: Accepts valid sha256 digest format
        CORRECTNESS: Format matches 'sha256:' followed by 64 hex characters

        TDD Phase: RED (this test should FAIL initially)
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool
        import re

        # ARRANGE
        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        # ACT
        result = tool._transform_api_response(mock_api_response_with_container_image)

        # ASSERT CORRECTNESS: sha256 digest format validation
        sha256_pattern = re.compile(r'^sha256:[a-f0-9]{64}$')

        for wf in result:
            container_digest = wf.get("container_digest")
            if container_digest:
                assert sha256_pattern.match(container_digest), \
                    f"BR-AI-075: container_digest must be 'sha256:' + 64 hex chars, got '{container_digest}'"


class TestWorkflowCatalogContainerImageEdgeCases:
    """
    Edge case tests for container_image propagation

    Business Requirement: BR-AI-075
    Design Decision: DD-WORKFLOW-002 v2.4

    Test Strategy:
    - Empty string handling
    - Partial data (only image, no digest)
    - Multiple workflows with mixed data
    """

    def test_empty_container_image_string_handled(self):
        """
        BR-AI-075: Empty string container_image is handled gracefully

        BEHAVIOR: Empty string is treated as missing
        CORRECTNESS: Returns empty string (not crash)
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

        # ARRANGE: API response with empty string (DD-WORKFLOW-002 v3.0 flat format)
        api_workflows = [{
            "workflow_id": "4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a",
            "title": "Test",
            "description": "Test workflow",
            "signal_type": "OOMKilled",
            "container_image": "",  # Empty string
            "container_digest": "",  # Empty string
            "confidence": 0.85
        }]

        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        # ACT
        result = tool._transform_api_response(api_workflows)

        # ASSERT: No crash, empty strings preserved
        assert result[0].get("container_image") == "" or result[0].get("container_image") is None
        assert result[0].get("container_digest") == "" or result[0].get("container_digest") is None

    def test_partial_data_only_image_no_digest(self):
        """
        BR-AI-075: Workflow with container_image but no container_digest

        BEHAVIOR: Handle partial data gracefully
        CORRECTNESS: Image present, digest None/empty
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

        # ARRANGE: API response with only container_image (DD-WORKFLOW-002 v3.0 flat format)
        api_workflows = [{
            "workflow_id": "5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b",
            "title": "Partial",
            "description": "Partial workflow",
            "signal_type": "OOMKilled",
            "container_image": "quay.io/kubernaut/workflow:v1.0.0",
            # NO container_digest
            "confidence": 0.85
        }]

        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        # ACT
        result = tool._transform_api_response(api_workflows)

        # ASSERT: Image present, digest handled gracefully
        assert result[0].get("container_image") == "quay.io/kubernaut/workflow:v1.0.0"
        assert "container_digest" in result[0]  # Field should exist
        assert result[0]["container_digest"] is None or result[0]["container_digest"] == ""

    def test_mixed_workflows_some_with_image_some_without(self):
        """
        BR-AI-075: Mixed results - some workflows have container_image, some don't

        BEHAVIOR: Handle mixed data in single response
        CORRECTNESS: Each workflow handled independently
        """
        from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

        # ARRANGE: Mixed API response (DD-WORKFLOW-002 v3.0 flat format)
        api_workflows = [
            {
                "workflow_id": "6f7a8b9c-0d1e-2f3a-4b5c-6d7e8f9a0b1c",
                "title": "New Workflow",
                "description": "New with container_image",
                "signal_type": "OOMKilled",
                "container_image": "quay.io/kubernaut/new:v2.0.0",
                "container_digest": "sha256:aabb123456789012345678901234567890123456789012345678901234ccdd",
                "confidence": 0.95
            },
            {
                "workflow_id": "7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d",
                "title": "Legacy Workflow",
                "description": "Legacy without container_image",
                "signal_type": "OOMKilled",
                # NO container_image or container_digest
                "confidence": 0.80
            }
        ]

        tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:8080")

        # ACT
        result = tool._transform_api_response(api_workflows)

        # ASSERT: Both workflows processed
        assert len(result) == 2

        # First workflow has container_image
        assert result[0]["container_image"] == "quay.io/kubernaut/new:v2.0.0"
        assert result[0]["container_digest"] == "sha256:aabb123456789012345678901234567890123456789012345678901234ccdd"

        # Second workflow has None/empty for missing fields
        assert "container_image" in result[1]
        assert result[1]["container_image"] is None or result[1]["container_image"] == ""


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])

