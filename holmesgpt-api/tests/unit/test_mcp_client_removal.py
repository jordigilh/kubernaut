"""
Unit tests for MCPClient removal validation

Business Requirement: BR-HAPI-250 - Workflow Catalog via WorkflowCatalogToolset (not MCPClient)
Design Decision: DD-WORKFLOW-002 v2.4 - WorkflowCatalogToolset is the authoritative implementation
Technical Debt: Remove legacy MCPClient that calls mock MCP server

TDD Phase: RED (failing tests - MCPClient should be removed)

Test Strategy:
1. Verify incident.py doesn't import MCPClient (orphan import)
2. Verify mcp_client.py is deleted
"""

import pytest
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


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])



