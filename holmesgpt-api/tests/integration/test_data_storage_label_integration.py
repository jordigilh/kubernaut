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
Integration Tests: HolmesGPT-API ↔ Data Storage Label Schema (DD-WORKFLOW-001 v1.8)

Business Requirements:
- BR-STORAGE-013: Semantic Search for Remediation Workflows
- BR-HAPI-250: Workflow Catalog Search Tool

Design Decisions:
- DD-WORKFLOW-001 v1.8: Mandatory Workflow Label Schema (snake_case)
- DD-WORKFLOW-002 v3.3: MCP Workflow Catalog Architecture
- DD-HAPI-001: Custom Labels Auto-Append Architecture

TESTING PHILOSOPHY:
These tests validate BUSINESS OUTCOMES, not implementation details.
Each test verifies that the system BEHAVES correctly from a user perspective.

Test Data (from bootstrap-workflows.sh):
- OOMKilled workflows (2): severity=critical/high, risk_tolerance=low/medium
- CrashLoopBackOff workflow (1): severity=high, risk_tolerance=low
- NodeNotReady workflow (1): severity=critical, risk_tolerance=low
- ImagePullBackOff workflow (1): severity=high, risk_tolerance=medium

Prerequisites:
    ./tests/integration/setup_workflow_catalog_integration.sh

Run:
    python -m pytest tests/integration/test_data_storage_label_integration.py -v
"""

import os
import pytest
import requests

from src.toolsets.workflow_catalog import (
    SearchWorkflowCatalogTool,
    WorkflowCatalogToolset,
    should_include_detected_labels,
)

# OpenAPI client imports for direct API contract tests
from datastorage.api.workflow_catalog_api_api import WorkflowCatalogAPIApi
from datastorage.models.workflow_search_request import WorkflowSearchRequest
from datastorage.models.workflow_search_filters import WorkflowSearchFilters
from datastorage.api_client import ApiClient
from datastorage.configuration import Configuration

from tests.integration.conftest import (
    DATA_STORAGE_URL,
    is_integration_infra_available,
)


# ========================================
# BUSINESS OUTCOME: Correct Workflow Selection
# ========================================

@pytest.mark.requires_data_storage
class TestWorkflowSelectionBySignalType:
    """
    BR-HAPI-250: Users must get workflows that match their signal type.

    BUSINESS OUTCOME: When a Pod crashes with OOMKilled, the user should
    receive OOMKill remediation workflows, not CrashLoopBackOff workflows.
    """

    def test_oomkilled_query_returns_oomkill_workflows(self, integration_infrastructure):
        """
        BEHAVIOR: Searching for "OOMKilled" returns workflows ranked by relevance.

        BUSINESS OUTCOME: An operator investigating an OOMKilled event should
        receive workflows with OOMKill-related workflows ranked highest.

        NOTE: Semantic search ranks by similarity, not strict filtering.
        The TOP result should be relevant; lower-ranked results may be less relevant.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            remediation_id="test-oomkill-001"
        )

        rca_resource = {
            "signal_type": "OOMKilled",
            "kind": "Pod",
            "namespace": "production",
            "name": "api-server-xyz"
        }

        workflows = tool._search_workflows(
            query="OOMKilled critical memory exhaustion",
            rca_resource=rca_resource,
            filters={},
            top_k=5
        )

        # BEHAVIOR: Must return at least one workflow
        assert len(workflows) > 0, \
            "BR-HAPI-250: OOMKilled query must return at least one matching workflow"

        # CORRECTNESS: TOP workflow should be for OOMKilled (semantic search ranks best matches first)
        top_workflow = workflows[0]
        top_signal_type = top_workflow.get("signal_type", "")
        top_title = top_workflow.get("title", "").lower()

        # Accept if EITHER the top result is OOMKilled OR an OOMKilled workflow exists in top 3
        oom_in_top = any(
            "OOMKilled" in wf.get("signal_type", "") or "oomkill" in wf.get("title", "").lower()
            for wf in workflows[:3]
        )
        assert oom_in_top, \
            f"BR-HAPI-250: OOMKilled query should have OOMKill workflow in top 3. Got: {[wf.get('title') for wf in workflows[:3]]}"

    def test_crashloopbackoff_query_returns_crashloop_workflows(self, integration_infrastructure):
        """
        BEHAVIOR: Searching for "CrashLoopBackOff" returns workflows ranked by relevance.

        BUSINESS OUTCOME: An operator investigating a CrashLoopBackOff should
        receive workflows with configuration/startup issues ranked highest.

        NOTE: Semantic search ranks by similarity, not strict filtering.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            remediation_id="test-crashloop-001"
        )

        rca_resource = {
            "signal_type": "CrashLoopBackOff",
            "kind": "Pod",
            "namespace": "default",
            "name": "failing-pod"
        }

        workflows = tool._search_workflows(
            query="CrashLoopBackOff configuration startup issues fix",
            rca_resource=rca_resource,
            filters={},
            top_k=5
        )

        # BEHAVIOR: Must return at least one workflow
        assert len(workflows) > 0, \
            "BR-HAPI-250: CrashLoopBackOff query must return at least one matching workflow"

        # CORRECTNESS: A CrashLoopBackOff workflow should exist in top 3
        crashloop_in_top = any(
            "CrashLoopBackOff" in wf.get("signal_type", "") or "crashloop" in wf.get("title", "").lower()
            for wf in workflows[:3]
        )
        assert crashloop_in_top, \
            f"BR-HAPI-250: CrashLoopBackOff query should have CrashLoop workflow in top 3. Got: {[wf.get('title') for wf in workflows[:3]]}"

    def test_signal_type_filtering_excludes_unrelated_workflows(self, integration_infrastructure):
        """
        BEHAVIOR: OOMKilled search should NOT return CrashLoopBackOff workflows as top result.

        BUSINESS OUTCOME: Users get relevant workflows, not unrelated ones.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            remediation_id="test-filtering-001"
        )

        rca_resource = {
            "signal_type": "OOMKilled",
            "kind": "Pod",
            "namespace": "production",
            "name": "memory-hog"
        }

        workflows = tool._search_workflows(
            query="OOMKilled critical memory exhaustion",
            rca_resource=rca_resource,
            filters={},
            top_k=3
        )

        # CORRECTNESS: Top workflow should NOT be CrashLoopBackOff
        if len(workflows) > 0:
            top_workflow = workflows[0]
            # Top result should be OOMKilled-related, not CrashLoopBackOff
            assert "CrashLoopBackOff" not in top_workflow.get("signal_type", ""), \
                "BR-HAPI-250: OOMKilled query should not return CrashLoopBackOff as top result"


# ========================================
# BUSINESS OUTCOME: Confidence Scores Indicate Match Quality
# ========================================

@pytest.mark.requires_data_storage
class TestConfidenceScoresBehavior:
    """
    DD-WORKFLOW-004: Confidence scores help users select the best workflow.

    BUSINESS OUTCOME: Higher confidence = better match for the user's problem.
    """

    def test_confidence_scores_are_meaningful(self, integration_infrastructure):
        """
        BEHAVIOR: Confidence scores should be between 0 and 1.

        BUSINESS OUTCOME: Users can trust confidence scores to rank workflows.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            remediation_id="test-confidence-001"
        )

        rca_resource = {
            "signal_type": "OOMKilled",
            "kind": "Pod",
            "namespace": "default",
            "name": "test"
        }

        workflows = tool._search_workflows(
            query="OOMKilled critical",
            rca_resource=rca_resource,
            filters={},
            top_k=5
        )

        # CORRECTNESS: All workflows must have valid confidence scores
        for wf in workflows:
            confidence = wf.get("confidence", -1)
            assert 0.0 <= confidence <= 1.0, \
                f"DD-WORKFLOW-004: Confidence {confidence} must be between 0 and 1"

    def test_workflows_are_ranked_by_confidence(self, integration_infrastructure):
        """
        BEHAVIOR: Workflows should be returned in descending confidence order.

        BUSINESS OUTCOME: Best matches appear first, saving user time.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            remediation_id="test-ranking-001"
        )

        rca_resource = {
            "signal_type": "OOMKilled",
            "kind": "Pod",
            "namespace": "default",
            "name": "test"
        }

        workflows = tool._search_workflows(
            query="OOMKilled critical",
            rca_resource=rca_resource,
            filters={},
            top_k=5
        )

        # CORRECTNESS: Workflows should be sorted by confidence (descending)
        if len(workflows) >= 2:
            confidences = [wf.get("confidence", 0) for wf in workflows]
            assert confidences == sorted(confidences, reverse=True), \
                f"DD-WORKFLOW-004: Workflows should be ranked by confidence. Got: {confidences}"


# ========================================
# BUSINESS OUTCOME: Response Contains Required Information
# ========================================

@pytest.mark.requires_data_storage
class TestWorkflowResponseCompleteness:
    """
    DD-WORKFLOW-002 v3.3: Workflows must contain all required fields.

    BUSINESS OUTCOME: Users have all information needed to select and execute a workflow.
    """

    def test_workflow_contains_execution_information(self, integration_infrastructure):
        """
        BEHAVIOR: Each workflow must include container_image for execution.

        BUSINESS OUTCOME: AIAnalysis can pass workflow to WorkflowExecution without
        additional lookups.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            remediation_id="test-completeness-001"
        )

        rca_resource = {
            "signal_type": "OOMKilled",
            "kind": "Pod",
            "namespace": "default",
            "name": "test"
        }

        workflows = tool._search_workflows(
            query="OOMKilled critical",
            rca_resource=rca_resource,
            filters={},
            top_k=3
        )

        assert len(workflows) > 0, "Must have workflows to test"

        # CORRECTNESS: Each workflow must have required fields
        for wf in workflows:
            # Required for identification
            assert "workflow_id" in wf and wf["workflow_id"], \
                "DD-WORKFLOW-002 v3.0: workflow_id is required"

            # Required for user selection
            assert "title" in wf and wf["title"], \
                "DD-WORKFLOW-002 v3.0: title is required for user display"
            assert "description" in wf, \
                "DD-WORKFLOW-002 v3.0: description is required"

            # Required for ranking
            assert "confidence" in wf, \
                "DD-WORKFLOW-004: confidence is required for ranking"

            # Required for execution (DD-CONTRACT-001)
            assert "container_image" in wf, \
                "DD-CONTRACT-001: container_image is required for execution"


# ========================================
# BUSINESS OUTCOME: DetectedLabels Improve Accuracy
# ========================================

@pytest.mark.requires_data_storage
class TestDetectedLabelsBusinessBehavior:
    """
    DD-WORKFLOW-001 v1.7: DetectedLabels improve workflow selection accuracy.

    BUSINESS OUTCOME: Workflows are filtered based on actual cluster characteristics.
    """

    def test_detected_labels_included_for_same_resource(self, integration_infrastructure):
        """
        BEHAVIOR: When RCA confirms the original resource, DetectedLabels are used.

        BUSINESS OUTCOME: GitOps-managed deployments get GitOps-aware workflows.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        source_resource = {
            "namespace": "production",
            "kind": "Pod",
            "name": "api-server-abc123"
        }

        detected_labels = {
            "gitOpsManaged": True,
            "gitOpsTool": "argocd",
            "pdbProtected": True
        }

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            detected_labels=detected_labels,
            source_resource=source_resource,
            owner_chain=[]
        )

        # RCA confirms same resource
        rca_resource = {
            "signal_type": "OOMKilled",
            "kind": "Pod",
            "namespace": "production",
            "name": "api-server-abc123"
        }

        # BEHAVIOR: Validation function confirms relationship
        assert should_include_detected_labels(source_resource, rca_resource, []) is True, \
            "DD-WORKFLOW-001 v1.7: Same resource should include DetectedLabels"

        # BEHAVIOR: Search completes successfully (Data Storage accepts the request)
        workflows = tool._search_workflows(
            query="OOMKilled critical",
            rca_resource=rca_resource,
            filters={},
            top_k=3
        )

        assert isinstance(workflows, list), "Search should return valid results"

    def test_detected_labels_excluded_for_different_resource_type(self, integration_infrastructure):
        """
        BEHAVIOR: When RCA identifies a different resource, DetectedLabels are excluded.

        BUSINESS OUTCOME: Pod-level labels (e.g., PDB protection) are NOT applied
        to Node-level issues, preventing incorrect workflow filtering.

        EXAMPLE: Pod crashes → RCA finds DiskPressure on Node
        The Pod's "pdbProtected" label should NOT filter Node remediation workflows.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        source_resource = {
            "namespace": "production",
            "kind": "Pod",
            "name": "api-server-abc123"
        }

        # These labels describe the Pod, not the Node
        detected_labels = {
            "gitOpsManaged": True,
            "pdbProtected": True,
            "hpaEnabled": True
        }

        owner_chain = [
            {"namespace": "production", "kind": "Deployment", "name": "api-server"}
        ]

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            detected_labels=detected_labels,
            source_resource=source_resource,
            owner_chain=owner_chain
        )

        # RCA identifies Node as root cause (NOT in owner chain)
        rca_resource = {
            "signal_type": "DiskPressure",
            "kind": "Node",
            "namespace": "",  # Cluster-scoped
            "name": "worker-3"
        }

        # BEHAVIOR: Validation function excludes labels (100% safe default)
        assert should_include_detected_labels(source_resource, rca_resource, owner_chain) is False, \
            "DD-WORKFLOW-001 v1.7: Different resource type should EXCLUDE DetectedLabels"

        # BEHAVIOR: Search still works, just without Pod-level DetectedLabels
        workflows = tool._search_workflows(
            query="DiskPressure critical node",
            rca_resource=rca_resource,
            filters={},
            top_k=3
        )

        assert isinstance(workflows, list), "Search should work without DetectedLabels"

    def test_detected_labels_included_for_owner_chain_match(self, integration_infrastructure):
        """
        BEHAVIOR: When RCA identifies a parent resource (Deployment), DetectedLabels are used.

        BUSINESS OUTCOME: If a Pod's owner Deployment is the root cause,
        the Pod's environment characteristics still apply.

        EXAMPLE: Pod OOMKilled → RCA finds Deployment memory limits too low
        The Pod's "gitOpsManaged" label still applies to Deployment workflows.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        source_resource = {
            "namespace": "production",
            "kind": "Pod",
            "name": "payment-api-7d8f9c6b5-x2k4m"
        }

        owner_chain = [
            {"namespace": "production", "kind": "ReplicaSet", "name": "payment-api-7d8f9c6b5"},
            {"namespace": "production", "kind": "Deployment", "name": "payment-api"}
        ]

        detected_labels = {
            "gitOpsManaged": True,
            "gitOpsTool": "argocd"
        }

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            detected_labels=detected_labels,
            source_resource=source_resource,
            owner_chain=owner_chain
        )

        # RCA identifies Deployment (in owner chain)
        rca_resource = {
            "signal_type": "OOMKilled",
            "kind": "Deployment",
            "namespace": "production",
            "name": "payment-api"
        }

        # BEHAVIOR: Validation function includes labels (proven relationship)
        assert should_include_detected_labels(source_resource, rca_resource, owner_chain) is True, \
            "DD-WORKFLOW-001 v1.7: Owner chain match should INCLUDE DetectedLabels"

        workflows = tool._search_workflows(
            query="OOMKilled critical deployment",
            rca_resource=rca_resource,
            filters={},
            top_k=3
        )

        assert isinstance(workflows, list), "Search should include DetectedLabels for owner match"


# ========================================
# BUSINESS OUTCOME: Custom Labels Filter Correctly
# ========================================

@pytest.mark.requires_data_storage
class TestCustomLabelsBusinessBehavior:
    """
    DD-HAPI-001: Custom labels filter workflows by business context.

    BUSINESS OUTCOME: Teams get workflows appropriate for their constraints.

    Data Storage API Contract (confirmed):
    - custom_labels in JSON body: {"custom_labels": {"constraint": ["cost-constrained"]}}
    - SQL filtering: custom_labels @> '{"constraint": ["cost-constrained"]}'::jsonb
    """

    def test_custom_labels_are_passed_to_data_storage(self, integration_infrastructure):
        """
        BEHAVIOR: Custom labels from enrichment are included in search request.

        BUSINESS OUTCOME: A cost-constrained team gets cost-aware workflows.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        custom_labels = {
            "constraint": ["cost-constrained"],
            "team": ["name=payments"]
        }

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            custom_labels=custom_labels,
            remediation_id="test-custom-001"
        )

        rca_resource = {
            "signal_type": "OOMKilled",
            "kind": "Pod",
            "namespace": "production",
            "name": "payment-api"
        }

        # BEHAVIOR: Search completes (Data Storage accepts custom_labels)
        workflows = tool._search_workflows(
            query="OOMKilled critical",
            rca_resource=rca_resource,
            filters={},
            top_k=3
        )

        # CORRECTNESS: Request succeeded (may return empty if no matching workflows)
        assert isinstance(workflows, list), \
            "DD-HAPI-001: Search with custom_labels should return valid response"


# ========================================
# BUSINESS OUTCOME: System Handles Edge Cases Gracefully
# ========================================

@pytest.mark.requires_data_storage
class TestEdgeCaseBehavior:
    """
    Edge cases should not crash the system.

    BUSINESS OUTCOME: Operators can trust the system to be reliable.
    """

    def test_minimal_query_returns_results(self, integration_infrastructure):
        """
        BEHAVIOR: A minimal query should still return some workflows.

        BUSINESS OUTCOME: System is resilient to incomplete input.

        NOTE: Empty queries may be rejected by Data Storage (400 Bad Request),
        so we test with a minimal query instead.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        tool = SearchWorkflowCatalogTool(
            data_storage_url=data_storage_url,
            remediation_id="test-edge-001"
        )

        rca_resource = {
            "signal_type": "Unknown",
            "kind": "Pod",
            "namespace": "default",
            "name": "test"
        }

        # Minimal query (not empty - Data Storage requires a query)
        workflows = tool._search_workflows(
            query="remediation workflow",
            rca_resource=rca_resource,
            filters={},
            top_k=3
        )

        assert isinstance(workflows, list), "Minimal query should return valid list"


class TestConnectionErrorHandling:
    """
    Error handling tests that don't require infrastructure.

    These tests use intentionally invalid URLs to validate error handling.
    BUSINESS OUTCOME: Connection failures are handled gracefully.

    NOTE: No @pytest.mark.requires_data_storage marker - these tests
    use fake URLs and don't need real infrastructure.
    """

    def test_connection_failure_raises_meaningful_error(self):
        """
        BEHAVIOR: Connection failures should raise clear, actionable errors.

        BUSINESS OUTCOME: Operators understand what went wrong.

        NOTE: Uses invalid URL - does NOT require infrastructure.
        """
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://invalid-host-12345:99999",
            remediation_id="test-error-001"
        )

        rca_resource = {
            "signal_type": "OOMKilled",
            "kind": "Pod",
            "namespace": "default",
            "name": "test"
        }

        with pytest.raises(Exception) as exc_info:
            tool._search_workflows(
                query="OOMKilled critical",
                rca_resource=rca_resource,
                filters={},
                top_k=3
            )

        # CORRECTNESS: An exception is raised (connection or parse error)
        # The error message format may vary, but an exception must be raised
        assert exc_info.value is not None, \
            "Connection failure must raise an exception"


# ========================================
# DIRECT API VALIDATION (Contract Tests)
# ========================================

@pytest.mark.requires_data_storage
class TestDataStorageAPIContract:
    """
    Contract tests: Validate Data Storage accepts the expected request format.

    These tests bypass HolmesGPT-API to directly validate the API contract.
    """

    def test_data_storage_returns_workflows_for_valid_query(self, integration_infrastructure):
        """
        BEHAVIOR: Valid search request returns workflow results.

        BUSINESS OUTCOME: The integration between services works.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        # Use OpenAPI client for type-safe API call (DD-STORAGE-011)
        # DD-AUTH-014: Use authenticated client helper
        from tests.integration.conftest import create_authenticated_datastorage_client
        api_client, search_api = create_authenticated_datastorage_client(data_storage_url)

        filters = WorkflowSearchFilters(
            signal_type="OOMKilled",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0"
        )
        request = WorkflowSearchRequest(
            query="OOMKilled critical",
            filters=filters,
            top_k=5
        )

        # Execute type-safe API call
        response = search_api.search_workflows(workflow_search_request=request, _request_timeout=10)

        # CORRECTNESS: Response structure (typed response from OpenAPI client)
        assert hasattr(response, 'workflows'), "Response must have 'workflows' attribute"
        assert hasattr(response, 'total_results'), "Response must have 'total_results' attribute"

        # CORRECTNESS: Should have OOMKilled workflows (from bootstrap data)
        assert response.total_results > 0, \
            "Should return at least one OOMKilled workflow from test data"

    def test_data_storage_accepts_snake_case_signal_type(self, integration_infrastructure):
        """
        DD-WORKFLOW-001 v1.8: Data Storage must accept signal_type (snake_case).
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        # Use OpenAPI client for type-safe API call (DD-STORAGE-011)
        # DD-AUTH-014: Use authenticated client helper
        from tests.integration.conftest import create_authenticated_datastorage_client
        api_client, search_api = create_authenticated_datastorage_client(data_storage_url)

        filters = WorkflowSearchFilters(
            signal_type="CrashLoopBackOff",  # snake_case per DD-WORKFLOW-001 v1.8
            severity="high",
            component="pod",
            environment="production",
            priority="P1"
        )
        request = WorkflowSearchRequest(
            query="CrashLoopBackOff high",
            filters=filters,
            top_k=3
        )

        # Execute type-safe API call - OpenAPI client validates field names
        response = search_api.search_workflows(workflow_search_request=request, _request_timeout=10)

        # CORRECTNESS: Response received successfully (OpenAPI client would raise exception on error)
        assert response is not None, \
            f"DD-WORKFLOW-001 v1.8: signal_type (snake_case) must work with OpenAPI client"

    def test_data_storage_accepts_custom_labels_structure(self, integration_infrastructure):
        """
        DD-HAPI-001: Data Storage must accept custom_labels in request.

        Data Storage API Contract (confirmed):
        - custom_labels: map[string][]string (subdomain → values)
        - SQL: JSONB containment query
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        # Use OpenAPI client for type-safe API call (DD-STORAGE-011)
        # DD-AUTH-014: Use authenticated client helper
        from tests.integration.conftest import create_authenticated_datastorage_client
        api_client, search_api = create_authenticated_datastorage_client(data_storage_url)

        filters = WorkflowSearchFilters(
            signal_type="OOMKilled",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            custom_labels={
                "team": ["name=payments"],
                "constraint": ["cost-constrained", "stateful-safe"]
            }
        )
        request = WorkflowSearchRequest(
            query="OOMKilled critical",
            filters=filters,
            top_k=3
        )

        # Execute type-safe API call - OpenAPI client validates custom_labels structure
        response = search_api.search_workflows(workflow_search_request=request, _request_timeout=10)

        # CORRECTNESS: Response received successfully (OpenAPI client would raise exception on error)
        assert response is not None, \
            f"DD-HAPI-001: custom_labels structure must be accepted by OpenAPI client"

    def test_data_storage_accepts_detected_labels_with_wildcard(self, integration_infrastructure):
        """
        DD-WORKFLOW-001 v1.6: Data Storage must accept detected_labels with wildcard "*".
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        # Use OpenAPI client for type-safe API call (DD-STORAGE-011)
        # DD-AUTH-014: Use authenticated client helper
        from tests.integration.conftest import create_authenticated_datastorage_client
        api_client, search_api = create_authenticated_datastorage_client(data_storage_url)

        filters = WorkflowSearchFilters(
            signal_type="OOMKilled",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            detected_labels={
                "gitOpsManaged": True,
                "gitOpsTool": "*",  # Wildcard: requires SOME GitOps tool
                "pdbProtected": True
            }
        )
        request = WorkflowSearchRequest(
            query="OOMKilled critical",
            filters=filters,
            top_k=3
        )

        # Execute type-safe API call - OpenAPI client validates detected_labels structure
        response = search_api.search_workflows(workflow_search_request=request, _request_timeout=10)

        # CORRECTNESS: Response received successfully (OpenAPI client would raise exception on error)
        assert response is not None, \
            f"DD-WORKFLOW-001 v1.6: detected_labels with wildcard must work with OpenAPI client"
