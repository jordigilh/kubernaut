"""
Unit Tests for Custom Labels Auto-Append Architecture (DD-HAPI-001)
and DetectedLabels Pass-Through (DD-WORKFLOW-001 v1.6)

Business Requirement: BR-HAPI-250 - Workflow Catalog Search
Design Decisions:
  - DD-HAPI-001 - Custom Labels Auto-Append Architecture
  - DD-WORKFLOW-001 v1.6 - DetectedLabels for workflow wildcard matching

Tests verify:
1. SearchWorkflowCatalogTool accepts custom_labels in constructor
2. SearchWorkflowCatalogTool accepts detected_labels in constructor
3. WorkflowCatalogToolset accepts and passes custom_labels and detected_labels
4. custom_labels are auto-appended to search filters
5. detected_labels are auto-appended to search filters
6. Empty labels are not appended
7. Label structures are preserved
"""

import pytest
from unittest.mock import patch, MagicMock
import json

from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool, WorkflowCatalogToolset


class TestSearchWorkflowCatalogToolCustomLabels:
    """Tests for SearchWorkflowCatalogTool custom_labels handling (DD-HAPI-001)"""

    def test_constructor_accepts_custom_labels(self):
        """DD-HAPI-001: Tool should accept custom_labels in constructor"""
        custom_labels = {
            "constraint": ["cost-constrained", "stateful-safe"],
            "team": ["name=payments"]
        }

        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            custom_labels=custom_labels
        )

        # Verify custom_labels are stored
        assert tool._custom_labels == custom_labels

    def test_constructor_defaults_to_empty_dict(self):
        """DD-HAPI-001: Tool should default custom_labels to empty dict"""
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001"
        )

        # Verify default is empty dict
        assert tool._custom_labels == {}

    def test_constructor_handles_none_custom_labels(self):
        """DD-HAPI-001: Tool should handle None custom_labels gracefully"""
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            custom_labels=None
        )

        # Verify None becomes empty dict
        assert tool._custom_labels == {}

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_auto_append_custom_labels_to_filters(self, mock_post):
        """DD-HAPI-001: custom_labels should be auto-appended to search filters"""
        # Setup mock response
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "workflows": [],
            "total_results": 0
        }
        mock_response.elapsed.total_seconds.return_value = 0.1
        mock_post.return_value = mock_response

        custom_labels = {
            "constraint": ["cost-constrained"],
            "team": ["name=payments"]
        }

        # Create tool with source_resource for validation
        source_resource = {"namespace": "default", "kind": "Pod", "name": "test-pod"}
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            custom_labels=custom_labels,
            source_resource=source_resource,
            owner_chain=[]  # Empty but provided
        )

        # Execute search with matching rca_resource
        rca_resource = {"kind": "Pod", "namespace": "default", "name": "test-pod"}
        tool._search_workflows("OOMKilled critical", rca_resource, {}, 3)

        # Verify request was made with custom_labels in filters
        mock_post.assert_called_once()
        call_args = mock_post.call_args
        request_data = call_args.kwargs.get('json') or call_args[1].get('json')

        assert "custom_labels" in request_data["filters"]
        assert request_data["filters"]["custom_labels"] == custom_labels

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_empty_custom_labels_not_appended(self, mock_post):
        """DD-HAPI-001: Empty custom_labels should NOT be appended to filters"""
        # Setup mock response
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "workflows": [],
            "total_results": 0
        }
        mock_response.elapsed.total_seconds.return_value = 0.1
        mock_post.return_value = mock_response

        # Create tool with empty custom_labels
        source_resource = {"namespace": "default", "kind": "Pod", "name": "test-pod"}
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            custom_labels={},
            source_resource=source_resource,
            owner_chain=[]
        )

        # Execute search with matching rca_resource
        rca_resource = {"kind": "Pod", "namespace": "default", "name": "test-pod"}
        tool._search_workflows("OOMKilled critical", rca_resource, {}, 3)

        # Verify request was made WITHOUT custom_labels in filters
        mock_post.assert_called_once()
        call_args = mock_post.call_args
        request_data = call_args.kwargs.get('json') or call_args[1].get('json')

        assert "custom_labels" not in request_data["filters"]

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_custom_labels_structure_preserved(self, mock_post):
        """DD-HAPI-001: custom_labels structure should be preserved (map[string][]string)"""
        # Setup mock response
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "workflows": [],
            "total_results": 0
        }
        mock_response.elapsed.total_seconds.return_value = 0.1
        mock_post.return_value = mock_response

        # Complex custom_labels structure
        custom_labels = {
            "constraint": ["cost-constrained", "stateful-safe", "no-downtime"],
            "team": ["name=payments"],
            "region": ["zone=us-east-1", "zone=us-west-2"],
            "compliance": ["pci-dss", "soc2"]
        }

        source_resource = {"namespace": "default", "kind": "Pod", "name": "test-pod"}
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            custom_labels=custom_labels,
            source_resource=source_resource,
            owner_chain=[]
        )

        # Execute search with matching rca_resource
        rca_resource = {"kind": "Pod", "namespace": "default", "name": "test-pod"}
        tool._search_workflows("OOMKilled critical", rca_resource, {}, 3)

        # Verify structure is preserved exactly
        call_args = mock_post.call_args
        request_data = call_args.kwargs.get('json') or call_args[1].get('json')

        assert request_data["filters"]["custom_labels"] == custom_labels
        # Verify each subdomain has list of values
        for subdomain, values in custom_labels.items():
            assert request_data["filters"]["custom_labels"][subdomain] == values

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_custom_labels_with_boolean_and_keyvalue_formats(self, mock_post):
        """DD-HAPI-001: custom_labels should support both boolean and key=value formats"""
        # Setup mock response
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "workflows": [],
            "total_results": 0
        }
        mock_response.elapsed.total_seconds.return_value = 0.1
        mock_post.return_value = mock_response

        # Mix of boolean keys and key=value pairs
        custom_labels = {
            "constraint": ["cost-constrained", "stateful-safe"],  # Boolean keys
            "team": ["name=payments", "owner=sre"],  # Key=value pairs
            "mixed": ["active", "priority=high"]  # Mixed
        }

        source_resource = {"namespace": "default", "kind": "Pod", "name": "test-pod"}
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            custom_labels=custom_labels,
            source_resource=source_resource,
            owner_chain=[]
        )

        # Execute search with matching rca_resource
        rca_resource = {"kind": "Pod", "namespace": "default", "name": "test-pod"}
        tool._search_workflows("OOMKilled critical", rca_resource, {}, 3)

        # Verify all formats are preserved
        call_args = mock_post.call_args
        request_data = call_args.kwargs.get('json') or call_args[1].get('json')

        assert request_data["filters"]["custom_labels"]["constraint"] == ["cost-constrained", "stateful-safe"]
        assert request_data["filters"]["custom_labels"]["team"] == ["name=payments", "owner=sre"]
        assert request_data["filters"]["custom_labels"]["mixed"] == ["active", "priority=high"]


class TestWorkflowCatalogToolsetCustomLabels:
    """Tests for WorkflowCatalogToolset custom_labels handling (DD-HAPI-001)"""

    def test_toolset_accepts_custom_labels(self):
        """DD-HAPI-001: Toolset should accept custom_labels in constructor"""
        custom_labels = {
            "constraint": ["cost-constrained"],
            "team": ["name=payments"]
        }

        toolset = WorkflowCatalogToolset(
            enabled=True,
            remediation_id="test-req-001",
            custom_labels=custom_labels
        )

        # Verify toolset is created
        assert toolset.enabled is True
        assert len(toolset.tools) == 1

    def test_toolset_passes_custom_labels_to_tool(self):
        """DD-HAPI-001: Toolset should pass custom_labels to SearchWorkflowCatalogTool"""
        custom_labels = {
            "constraint": ["cost-constrained"],
            "team": ["name=payments"]
        }

        toolset = WorkflowCatalogToolset(
            enabled=True,
            remediation_id="test-req-001",
            custom_labels=custom_labels
        )

        # Get the tool and verify custom_labels
        tool = toolset.tools[0]
        assert isinstance(tool, SearchWorkflowCatalogTool)
        assert tool._custom_labels == custom_labels

    def test_toolset_handles_none_custom_labels(self):
        """DD-HAPI-001: Toolset should handle None custom_labels gracefully"""
        toolset = WorkflowCatalogToolset(
            enabled=True,
            remediation_id="test-req-001",
            custom_labels=None
        )

        # Get the tool and verify empty custom_labels
        tool = toolset.tools[0]
        assert tool._custom_labels == {}

    def test_toolset_defaults_custom_labels_to_none(self):
        """DD-HAPI-001: Toolset should default custom_labels to None (then empty dict in tool)"""
        toolset = WorkflowCatalogToolset(
            enabled=True,
            remediation_id="test-req-001"
        )

        # Get the tool and verify empty custom_labels
        tool = toolset.tools[0]
        assert tool._custom_labels == {}


class TestCustomLabelsTypeModel:
    """Tests for CustomLabels type in incident_models.py (DD-HAPI-001)"""

    def test_custom_labels_type_is_dict_str_list_str(self):
        """DD-HAPI-001: CustomLabels should be Dict[str, List[str]]"""
        from src.models.incident_models import CustomLabels, EnrichmentResults

        # Verify type alias exists
        assert CustomLabels is not None

        # Verify EnrichmentResults accepts the correct type
        enrichment = EnrichmentResults(
            customLabels={
                "constraint": ["cost-constrained", "stateful-safe"],
                "team": ["name=payments"]
            }
        )

        assert enrichment.customLabels == {
            "constraint": ["cost-constrained", "stateful-safe"],
            "team": ["name=payments"]
        }

    def test_enrichment_results_custom_labels_default_none(self):
        """DD-HAPI-001: EnrichmentResults.customLabels should default to None"""
        from src.models.incident_models import EnrichmentResults

        enrichment = EnrichmentResults()
        assert enrichment.customLabels is None

    def test_enrichment_results_custom_labels_empty_dict(self):
        """DD-HAPI-001: EnrichmentResults should accept empty customLabels"""
        from src.models.incident_models import EnrichmentResults

        enrichment = EnrichmentResults(customLabels={})
        assert enrichment.customLabels == {}

    def test_enrichment_results_custom_labels_complex_structure(self):
        """DD-HAPI-001: EnrichmentResults should preserve complex customLabels structure"""
        from src.models.incident_models import EnrichmentResults

        complex_labels = {
            "constraint": ["cost-constrained", "stateful-safe", "no-downtime"],
            "team": ["name=payments", "owner=sre"],
            "region": ["zone=us-east-1"],
            "compliance": ["pci-dss", "soc2", "hipaa"]
        }

        enrichment = EnrichmentResults(customLabels=complex_labels)
        assert enrichment.customLabels == complex_labels


class TestRegisterWorkflowCatalogToolsetCustomLabels:
    """Tests for register_workflow_catalog_toolset with custom_labels (DD-HAPI-001)"""

    def test_register_accepts_custom_labels_parameter(self):
        """DD-HAPI-001: register_workflow_catalog_toolset should accept custom_labels"""
        from src.extensions.llm_config import register_workflow_catalog_toolset
        import inspect

        # Verify function signature includes custom_labels
        sig = inspect.signature(register_workflow_catalog_toolset)
        assert "custom_labels" in sig.parameters

    def test_register_custom_labels_default_none(self):
        """DD-HAPI-001: custom_labels parameter should default to None"""
        from src.extensions.llm_config import register_workflow_catalog_toolset
        import inspect

        sig = inspect.signature(register_workflow_catalog_toolset)
        custom_labels_param = sig.parameters["custom_labels"]
        assert custom_labels_param.default is None


# ========================================
# DETECTED LABELS PASS-THROUGH TESTS (DD-WORKFLOW-001 v1.6)
# ========================================

class TestSearchWorkflowCatalogToolDetectedLabels:
    """Tests for SearchWorkflowCatalogTool detected_labels handling (DD-WORKFLOW-001 v1.6)"""

    def test_constructor_accepts_detected_labels(self):
        """DD-WORKFLOW-001 v1.6: Tool should accept detected_labels in constructor"""
        detected_labels = {
            "gitOpsManaged": True,
            "gitOpsTool": "argocd",
            "pdbProtected": True,
            "stateful": False
        }

        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            detected_labels=detected_labels
        )

        # Verify detected_labels are stored
        assert tool._detected_labels == detected_labels

    def test_constructor_defaults_detected_labels_to_empty_dict(self):
        """DD-WORKFLOW-001 v1.6: Tool should default detected_labels to empty dict"""
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001"
        )

        # Verify default is empty dict
        assert tool._detected_labels == {}

    def test_constructor_handles_none_detected_labels(self):
        """DD-WORKFLOW-001 v1.6: Tool should handle None detected_labels gracefully"""
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            detected_labels=None
        )

        # Verify None becomes empty dict
        assert tool._detected_labels == {}

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_auto_append_detected_labels_to_filters(self, mock_post):
        """DD-WORKFLOW-001 v1.7: detected_labels should be auto-appended when resource matches"""
        # Setup mock response
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "workflows": [],
            "total_results": 0
        }
        mock_response.elapsed.total_seconds.return_value = 0.1
        mock_post.return_value = mock_response

        detected_labels = {
            "gitOpsManaged": True,
            "gitOpsTool": "argocd",
            "pdbProtected": True
        }

        # source_resource and owner_chain for 100% safe validation
        source_resource = {"namespace": "production", "kind": "Pod", "name": "api-server-xyz"}
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            detected_labels=detected_labels,
            source_resource=source_resource,
            owner_chain=[]  # Empty but provided - allows same ns/kind match
        )

        # Execute search with MATCHING rca_resource (same Pod)
        rca_resource = {"kind": "Pod", "namespace": "production", "name": "api-server-xyz"}
        tool._search_workflows("OOMKilled critical", rca_resource, {}, 3)

        # Verify request was made with detected_labels in filters (proven relationship)
        mock_post.assert_called_once()
        call_args = mock_post.call_args
        request_data = call_args.kwargs.get('json') or call_args[1].get('json')

        assert "detected_labels" in request_data["filters"]
        assert request_data["filters"]["detected_labels"] == detected_labels

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_empty_detected_labels_not_appended(self, mock_post):
        """DD-WORKFLOW-001 v1.7: Empty detected_labels should NOT be appended to filters"""
        # Setup mock response
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "workflows": [],
            "total_results": 0
        }
        mock_response.elapsed.total_seconds.return_value = 0.1
        mock_post.return_value = mock_response

        # Create tool with empty detected_labels
        source_resource = {"namespace": "production", "kind": "Pod", "name": "api-server-xyz"}
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            detected_labels={},
            source_resource=source_resource,
            owner_chain=[]
        )

        # Execute search with matching rca_resource
        rca_resource = {"kind": "Pod", "namespace": "production", "name": "api-server-xyz"}
        tool._search_workflows("OOMKilled critical", rca_resource, {}, 3)

        # Verify request was made WITHOUT detected_labels in filters
        mock_post.assert_called_once()
        call_args = mock_post.call_args
        request_data = call_args.kwargs.get('json') or call_args[1].get('json')

        assert "detected_labels" not in request_data["filters"]

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_detected_labels_boolean_and_string_types(self, mock_post):
        """DD-WORKFLOW-001 v1.7: detected_labels should support booleans and strings"""
        # Setup mock response
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "workflows": [],
            "total_results": 0
        }
        mock_response.elapsed.total_seconds.return_value = 0.1
        mock_post.return_value = mock_response

        # Full detected_labels with all field types
        # DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED (PSP deprecated)
        detected_labels = {
            # Boolean fields
            "gitOpsManaged": True,
            "pdbProtected": True,
            "hpaEnabled": False,  # Per Boolean Normalization Rule, false should be omitted in real usage
            "stateful": True,
            "helmManaged": False,
            "networkIsolated": True,
            # String fields
            "gitOpsTool": "argocd",
            "serviceMesh": "istio"
        }

        source_resource = {"namespace": "production", "kind": "Pod", "name": "api-server-xyz"}
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            detected_labels=detected_labels,
            source_resource=source_resource,
            owner_chain=[]
        )

        # Execute search with matching rca_resource
        rca_resource = {"kind": "Pod", "namespace": "production", "name": "api-server-xyz"}
        tool._search_workflows("OOMKilled critical", rca_resource, {}, 3)

        # Verify all types are preserved
        call_args = mock_post.call_args
        request_data = call_args.kwargs.get('json') or call_args[1].get('json')

        assert request_data["filters"]["detected_labels"] == detected_labels
        assert request_data["filters"]["detected_labels"]["gitOpsManaged"] is True
        assert request_data["filters"]["detected_labels"]["gitOpsTool"] == "argocd"

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_both_custom_and_detected_labels_appended(self, mock_post):
        """DD-HAPI-001 + DD-WORKFLOW-001 v1.7: Both labels should be auto-appended when resource matches"""
        # Setup mock response
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "workflows": [],
            "total_results": 0
        }
        mock_response.elapsed.total_seconds.return_value = 0.1
        mock_post.return_value = mock_response

        custom_labels = {
            "constraint": ["cost-constrained"],
            "team": ["name=payments"]
        }

        detected_labels = {
            "gitOpsManaged": True,
            "gitOpsTool": "argocd"
        }

        source_resource = {"namespace": "production", "kind": "Pod", "name": "api-server-xyz"}
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://test:8080",
            remediation_id="test-req-001",
            custom_labels=custom_labels,
            detected_labels=detected_labels,
            source_resource=source_resource,
            owner_chain=[]
        )

        # Execute search with matching rca_resource
        rca_resource = {"kind": "Pod", "namespace": "production", "name": "api-server-xyz"}
        tool._search_workflows("OOMKilled critical", rca_resource, {}, 3)

        # Verify BOTH are appended
        call_args = mock_post.call_args
        request_data = call_args.kwargs.get('json') or call_args[1].get('json')

        assert "custom_labels" in request_data["filters"]
        assert "detected_labels" in request_data["filters"]
        assert request_data["filters"]["custom_labels"] == custom_labels
        assert request_data["filters"]["detected_labels"] == detected_labels


class TestWorkflowCatalogToolsetDetectedLabels:
    """Tests for WorkflowCatalogToolset detected_labels handling (DD-WORKFLOW-001 v1.6)"""

    def test_toolset_accepts_detected_labels(self):
        """DD-WORKFLOW-001 v1.6: Toolset should accept detected_labels in constructor"""
        detected_labels = {
            "gitOpsManaged": True,
            "gitOpsTool": "argocd"
        }

        toolset = WorkflowCatalogToolset(
            enabled=True,
            remediation_id="test-req-001",
            detected_labels=detected_labels
        )

        # Verify toolset is created
        assert toolset.enabled is True
        assert len(toolset.tools) == 1

    def test_toolset_passes_detected_labels_to_tool(self):
        """DD-WORKFLOW-001 v1.6: Toolset should pass detected_labels to SearchWorkflowCatalogTool"""
        # DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED
        detected_labels = {
            "gitOpsManaged": True,
            "gitOpsTool": "argocd",
            "serviceMesh": "istio"
        }

        toolset = WorkflowCatalogToolset(
            enabled=True,
            remediation_id="test-req-001",
            detected_labels=detected_labels
        )

        # Get the tool and verify detected_labels
        tool = toolset.tools[0]
        assert isinstance(tool, SearchWorkflowCatalogTool)
        assert tool._detected_labels == detected_labels

    def test_toolset_handles_none_detected_labels(self):
        """DD-WORKFLOW-001 v1.6: Toolset should handle None detected_labels gracefully"""
        toolset = WorkflowCatalogToolset(
            enabled=True,
            remediation_id="test-req-001",
            detected_labels=None
        )

        # Get the tool and verify empty detected_labels
        tool = toolset.tools[0]
        assert tool._detected_labels == {}

    def test_toolset_defaults_detected_labels_to_none(self):
        """DD-WORKFLOW-001 v1.6: Toolset should default detected_labels to None"""
        toolset = WorkflowCatalogToolset(
            enabled=True,
            remediation_id="test-req-001"
        )

        # Get the tool and verify empty detected_labels
        tool = toolset.tools[0]
        assert tool._detected_labels == {}

    def test_toolset_accepts_both_labels(self):
        """DD-HAPI-001 + DD-WORKFLOW-001: Toolset should accept both custom and detected labels"""
        custom_labels = {
            "constraint": ["cost-constrained"],
            "team": ["name=payments"]
        }
        detected_labels = {
            "gitOpsManaged": True,
            "gitOpsTool": "argocd"
        }

        toolset = WorkflowCatalogToolset(
            enabled=True,
            remediation_id="test-req-001",
            custom_labels=custom_labels,
            detected_labels=detected_labels
        )

        # Get the tool and verify both labels
        tool = toolset.tools[0]
        assert tool._custom_labels == custom_labels
        assert tool._detected_labels == detected_labels


class TestRegisterWorkflowCatalogToolsetDetectedLabels:
    """Tests for register_workflow_catalog_toolset with detected_labels (DD-WORKFLOW-001 v1.6)"""

    def test_register_accepts_detected_labels_parameter(self):
        """DD-WORKFLOW-001 v1.6: register_workflow_catalog_toolset should accept detected_labels"""
        from src.extensions.llm_config import register_workflow_catalog_toolset
        import inspect

        # Verify function signature includes detected_labels
        sig = inspect.signature(register_workflow_catalog_toolset)
        assert "detected_labels" in sig.parameters

    def test_register_detected_labels_default_none(self):
        """DD-WORKFLOW-001 v1.6: detected_labels parameter should default to None"""
        from src.extensions.llm_config import register_workflow_catalog_toolset
        import inspect

        sig = inspect.signature(register_workflow_catalog_toolset)
        detected_labels_param = sig.parameters["detected_labels"]
        assert detected_labels_param.default is None

