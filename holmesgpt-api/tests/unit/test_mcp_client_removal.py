"""
Unit tests for MCPClient removal validation

Business Requirement: BR-HAPI-250 - Workflow Catalog via WorkflowCatalogToolset (not MCPClient)
Design Decision: DD-WORKFLOW-002 v2.4 - WorkflowCatalogToolset is the authoritative implementation
Technical Debt: Remove legacy MCPClient that calls mock MCP server

TDD Phase: RED (failing tests - MCPClient should be removed)

Test Strategy:
1. Verify recovery.py doesn't import MCPClient (should use WorkflowCatalogToolset via SDK)
2. Verify incident.py doesn't import MCPClient (orphan import)
3. Verify mcp_client.py is deleted
4. Verify _get_workflow_recommendations doesn't exist (dead code)
"""

import pytest
import ast
from pathlib import Path


class TestMCPClientRemoval:
    """
    Tests to verify MCPClient has been properly removed from the codebase.

    Business Requirement: BR-HAPI-250 - Workflow Catalog Search Tool
    Design Decision: DD-WORKFLOW-002 v2.4 - WorkflowCatalogToolset is authoritative

    The MCPClient was a legacy implementation that called a mock MCP server.
    The correct implementation uses WorkflowCatalogToolset which calls the
    real Data Storage Service REST API.
    """

    @pytest.fixture
    def src_path(self):
        """Get the src directory path"""
        return Path(__file__).parent.parent.parent / "src"

    @pytest.fixture
    def recovery_py_path(self, src_path):
        """Path to recovery package (refactored from recovery.py)"""
        return src_path / "extensions" / "recovery"

    @pytest.fixture
    def incident_py_path(self, src_path):
        """Path to incident package (refactored from incident.py)"""
        return src_path / "extensions" / "incident"

    @pytest.fixture
    def mcp_client_path(self, src_path):
        """Path to mcp_client.py"""
        return src_path / "clients" / "mcp_client.py"

    def test_mcp_client_file_deleted(self, mcp_client_path):
        """
        Verify mcp_client.py has been deleted

        BEHAVIOR: mcp_client.py should not exist
        CORRECTNESS: File should be removed from codebase

        TDD Phase: RED - This test should FAIL until mcp_client.py is deleted
        """
        assert not mcp_client_path.exists(), \
            f"DD-WORKFLOW-002 v2.4: mcp_client.py should be deleted - " \
            f"WorkflowCatalogToolset is the authoritative implementation. " \
            f"Found at: {mcp_client_path}"

    def test_recovery_py_no_mcp_client_import(self, recovery_py_path):
        """
        Verify recovery package doesn't import MCPClient

        BEHAVIOR: recovery package should not import MCPClient
        CORRECTNESS: Uses WorkflowCatalogToolset via SDK instead

        TDD Phase: RED - This test should FAIL until import is removed

        Note: recovery.py was refactored into a package (recovery/__init__.py, recovery/llm_integration.py, etc.)
        """
        assert recovery_py_path.exists(), f"recovery package not found at {recovery_py_path}"

        # Check all Python files in the recovery package
        for py_file in recovery_py_path.glob("*.py"):
            with open(py_file, "r") as f:
                content = f.read()

            # Check for MCPClient import
            assert "from src.clients.mcp_client import MCPClient" not in content, \
                f"DD-WORKFLOW-002 v2.4: {py_file.name} should not import MCPClient - " \
                f"WorkflowCatalogToolset is registered via register_workflow_catalog_toolset()"

            assert "MCPClient" not in content, \
                f"DD-WORKFLOW-002 v2.4: {py_file.name} should not reference MCPClient anywhere"

    def test_incident_py_no_mcp_client_import(self, incident_py_path):
        """
        Verify incident package doesn't import MCPClient (orphan import)

        BEHAVIOR: incident package should not import MCPClient
        CORRECTNESS: MCPClient import was orphan (never used)

        TDD Phase: RED - This test should FAIL until import is removed

        Note: incident.py was refactored into a package (incident/__init__.py, incident/llm_integration.py, etc.)
        """
        assert incident_py_path.exists(), f"incident package not found at {incident_py_path}"

        # Check all Python files in the incident package
        for py_file in incident_py_path.glob("*.py"):
            with open(py_file, "r") as f:
                content = f.read()

            # Check for MCPClient import
            assert "from src.clients.mcp_client import MCPClient" not in content, \
                f"DD-WORKFLOW-002 v2.4: {py_file.name} should not import MCPClient - " \
                f"it was an orphan import (never used)"

    def test_no_get_workflow_recommendations_function(self, recovery_py_path):
        """
        Verify _get_workflow_recommendations function has been removed

        BEHAVIOR: _get_workflow_recommendations should not exist
        CORRECTNESS: Function was dead code - results were never used

        TDD Phase: RED - This test should FAIL until function is removed

        Note: recovery.py was refactored into a package
        """
        assert recovery_py_path.exists()

        # Check all Python files in the recovery package
        for py_file in recovery_py_path.glob("*.py"):
            with open(py_file, "r") as f:
                content = f.read()

            # Check for function definition
            assert "async def _get_workflow_recommendations" not in content, \
                f"DD-WORKFLOW-002 v2.4: _get_workflow_recommendations should be removed from {py_file.name} - " \
                "it was dead code (results stored but never used in prompt)"

            assert "def _get_workflow_recommendations" not in content, \
                f"DD-WORKFLOW-002 v2.4: _get_workflow_recommendations should be removed from {py_file.name}"

    def test_no_workflow_recommendations_in_request_data(self, recovery_py_path):
        """
        Verify workflow_recommendations is not injected into request_data

        BEHAVIOR: request_data["workflow_recommendations"] should not be set
        CORRECTNESS: This was dead code - data was never used

        TDD Phase: RED - This test should FAIL until code is removed

        Note: recovery.py was refactored into a package
        """
        assert recovery_py_path.exists()

        # Check all Python files in the recovery package
        for py_file in recovery_py_path.glob("*.py"):
            with open(py_file, "r") as f:
                content = f.read()

            # Check for workflow_recommendations assignment
            assert 'request_data["workflow_recommendations"]' not in content, \
                f"DD-WORKFLOW-002 v2.4: workflow_recommendations injection should be removed from {py_file.name}"

    def test_no_mcp_config_parameter_in_analyze_recovery(self, recovery_py_path):
        """
        Verify analyze_recovery doesn't have mcp_config parameter

        BEHAVIOR: analyze_recovery should not accept mcp_config
        CORRECTNESS: MCPClient is removed, so mcp_config is unnecessary

        TDD Phase: RED - This test should FAIL until parameter is removed

        Note: recovery.py was refactored into a package
        """
        assert recovery_py_path.exists()

        # Check all Python files in the recovery package
        for py_file in recovery_py_path.glob("*.py"):
            with open(py_file, "r") as f:
                tree = ast.parse(f.read())

            # Find analyze_recovery function
            for node in ast.walk(tree):
                if isinstance(node, ast.AsyncFunctionDef) and node.name == "analyze_recovery":
                    param_names = [arg.arg for arg in node.args.args]
                    assert "mcp_config" not in param_names, \
                        f"DD-WORKFLOW-002 v2.4: analyze_recovery in {py_file.name} should not have mcp_config parameter"
                    break


class TestWorkflowCatalogToolsetIntegration:
    """
    Tests to verify WorkflowCatalogToolset is properly integrated

    Business Requirement: BR-HAPI-250 - Workflow Catalog Search Tool
    Design Decision: DD-WORKFLOW-002 v2.4
    """

    @pytest.fixture
    def src_path(self):
        """Get the src directory path"""
        return Path(__file__).parent.parent.parent / "src"

    @pytest.fixture
    def recovery_py_path(self, src_path):
        """Path to recovery package (refactored from recovery.py)"""
        return src_path / "extensions" / "recovery"

    def test_recovery_py_uses_register_workflow_discovery_toolset(self, recovery_py_path):
        """
        Verify recovery package uses register_workflow_discovery_toolset

        BEHAVIOR: recovery package should use register_workflow_discovery_toolset
        CORRECTNESS: This is the correct integration pattern per DD-HAPI-017

        TDD Phase: GREEN - This test should PASS (already implemented)

        Note: recovery.py was refactored into a package
        """
        assert recovery_py_path.exists()

        # Check all Python files in the recovery package
        found = False
        for py_file in recovery_py_path.glob("*.py"):
            with open(py_file, "r") as f:
                content = f.read()

            if "register_workflow_discovery_toolset" in content:
                found = True
                break

        assert found, \
            "DD-HAPI-017: recovery package should use register_workflow_discovery_toolset"


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])



