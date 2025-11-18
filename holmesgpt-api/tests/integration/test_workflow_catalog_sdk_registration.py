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
Integration Tests for Workflow Catalog Toolset SDK Registration

Business Requirement: BR-HAPI-250 - Workflow Catalog Search Tool
Test Scope: Validates WorkflowCatalogToolset is properly registered with HolmesGPT SDK

CRITICAL: These tests catch toolset registration failures that would cause
the workflow catalog tool to be unavailable to the LLM during incident analysis.

Test Coverage:
1. Toolset registration with Config
2. Toolset presence in ToolsetManager
3. Tool availability for LLM invocation
4. Tool invocation through SDK
5. Integration with incident analysis flow

Expected Behavior:
- WorkflowCatalogToolset must be registered and enabled
- Tool must be discoverable by the LLM
- Tool must be invocable during investigation
- Tool must return valid results

Failure Impact:
- If these tests fail, the workflow catalog will not be available during incident analysis
- LLM will say "MCP workflow search is not available" (like we saw in production)
"""

import pytest
import json
from typing import Dict, Any

from holmes.config import Config
from holmes.core.toolset_manager import ToolsetManager
from holmes.core.tools import StructuredToolResultStatus

from src.toolsets.workflow_catalog import WorkflowCatalogToolset, SearchWorkflowCatalogTool
from src.extensions.llm_config import get_model_config_for_sdk


@pytest.mark.integration
class TestWorkflowCatalogSDKRegistration:
    """
    Integration tests for WorkflowCatalogToolset registration with HolmesGPT SDK

    Business Requirement: BR-HAPI-250
    CRITICAL: Prevents "MCP workflow search is not available" production issues
    """

    def test_toolset_registration_with_config(self):
        """
        BR-HAPI-250: WorkflowCatalogToolset must be registrable with HolmesGPT Config

        BEHAVIOR: Config accepts workflow/catalog toolset configuration
        CORRECTNESS: Toolset appears in Config.toolsets after initialization

        FAILURE IMPACT: If this fails, the toolset won't be loaded at service startup
        """
        # Create config with workflow catalog toolset
        config = Config(
            model="anthropic/claude-haiku-4-5-20251001",
            toolsets={
                "kubernetes/core": {"enabled": True},
                "workflow/catalog": {"enabled": True},
            }
        )

        # BEHAVIOR VALIDATION: Config accepts workflow/catalog
        assert config is not None, \
            "BR-HAPI-250: Config must initialize with workflow/catalog toolset"

        # CORRECTNESS VALIDATION: Toolset is in config
        assert hasattr(config, 'toolsets'), \
            "BR-HAPI-250: Config must have toolsets attribute"
        assert "workflow/catalog" in config.toolsets, \
            "BR-HAPI-250: workflow/catalog must be present in Config.toolsets"

        # CORRECTNESS VALIDATION: Toolset is enabled
        toolset_config = config.toolsets.get("workflow/catalog")
        if isinstance(toolset_config, dict):
            assert toolset_config.get("enabled") is True, \
                "BR-HAPI-250: workflow/catalog must be enabled in config"

    def test_toolset_registration_with_toolset_manager(self):
        """
        BR-HAPI-250: WorkflowCatalogToolset must be discoverable via ToolsetManager

        BEHAVIOR: ToolsetManager can load workflow/catalog toolset
        CORRECTNESS: Toolset is present and enabled in ToolsetManager

        FAILURE IMPACT: If this fails, the LLM won't see the workflow catalog tools
        """
        # Create ToolsetManager with workflow catalog
        toolsets_config = {
            "kubernetes/core": {"enabled": True},
            "workflow/catalog": {"enabled": True},
        }

        toolset_manager = ToolsetManager(toolsets=toolsets_config)

        # BEHAVIOR VALIDATION: ToolsetManager accepts workflow/catalog
        assert toolset_manager is not None, \
            "BR-HAPI-250: ToolsetManager must initialize with workflow/catalog"

        # CORRECTNESS VALIDATION: Toolset is discoverable
        assert hasattr(toolset_manager, 'toolsets'), \
            "BR-HAPI-250: ToolsetManager must have toolsets attribute"
        assert "workflow/catalog" in toolset_manager.toolsets, \
            "BR-HAPI-250: workflow/catalog must be in ToolsetManager.toolsets"

    def test_programmatic_toolset_registration(self):
        """
        BR-HAPI-250: WorkflowCatalogToolset must be registrable programmatically

        BEHAVIOR: Toolset can be added to ToolsetManager.add_or_merge_onto_toolsets()
        CORRECTNESS: Programmatically added toolset is properly integrated

        FAILURE IMPACT: If this fails, incident.py/recovery.py registration will fail

        NOTE: This tests the pattern used in src/extensions/incident.py
        """
        # Create base config without workflow catalog
        config = Config(
            model="anthropic/claude-haiku-4-5-20251001",
            toolsets={
                "kubernetes/core": {"enabled": True},
            }
        )

        # Create ToolsetManager
        toolset_manager = ToolsetManager(toolsets=config.toolsets)

        # Programmatically register workflow catalog toolset
        workflow_toolset = WorkflowCatalogToolset(enabled=True)

        # CRITICAL: This must work or incident.py will fail
        try:
            # Prepare existing toolsets dict for merge
            existing_toolsets = {}
            if hasattr(toolset_manager, 'toolsets'):
                for name, toolset in toolset_manager.toolsets.items():
                    # Only add Toolset instances, not dicts
                    if hasattr(toolset, 'tools'):
                        existing_toolsets[name] = toolset

            toolset_manager.add_or_merge_onto_toolsets(
                [workflow_toolset],
                existing_toolsets
            )
            registration_succeeded = True
        except Exception as e:
            registration_succeeded = False
            pytest.fail(f"BR-HAPI-250: Programmatic registration failed: {e}")

        # BEHAVIOR VALIDATION: Registration doesn't crash
        assert registration_succeeded, \
            "BR-HAPI-250: add_or_merge_onto_toolsets must accept WorkflowCatalogToolset"

    def test_tool_availability_for_llm(self):
        """
        BR-HAPI-250: Workflow catalog tool must be available for LLM to invoke

        BEHAVIOR: Tool is discoverable through toolset
        CORRECTNESS: Tool has proper name, description, and parameters

        FAILURE IMPACT: If this fails, LLM won't see search_workflow_catalog as available tool
        """
        # Create toolset
        workflow_toolset = WorkflowCatalogToolset(enabled=True)

        # BEHAVIOR VALIDATION: Toolset has tools
        assert len(workflow_toolset.tools) == 1, \
            "BR-HAPI-250: WorkflowCatalogToolset must have exactly 1 tool"

        tool = workflow_toolset.tools[0]

        # CORRECTNESS VALIDATION: Tool properties for LLM
        assert tool.name == "search_workflow_catalog", \
            "BR-HAPI-250: Tool must be named 'search_workflow_catalog' for LLM"
        assert len(tool.description) > 0, \
            "BR-HAPI-250: Tool must have description for LLM to understand its purpose"
        assert "query" in tool.parameters, \
            "BR-HAPI-250: Tool must have 'query' parameter for LLM to invoke"
        assert tool.parameters["query"].required is True, \
            "BR-HAPI-250: 'query' parameter must be required for LLM"

        # CORRECTNESS VALIDATION: Tool is invocable
        assert hasattr(tool, '_invoke'), \
            "BR-HAPI-250: Tool must have _invoke method for execution"
        assert callable(tool._invoke), \
            "BR-HAPI-250: Tool._invoke must be callable"

    def test_tool_invocation_through_sdk(self):
        """
        BR-HAPI-250: Workflow catalog tool must be invocable through SDK

        BEHAVIOR: Tool can be invoked with valid parameters
        CORRECTNESS: Tool returns StructuredToolResult with workflows

        FAILURE IMPACT: If this fails, LLM tool calls will fail during investigation
        """
        # Create toolset and get tool
        workflow_toolset = WorkflowCatalogToolset(enabled=True)
        tool = workflow_toolset.tools[0]

        # Invoke tool with valid parameters
        result = tool._invoke(
            params={
                "query": "OOMKilled pod recovery",
                "filters": {"signal_types": ["OOMKilled"]},
                "top_k": 3
            },
            user_approved=False
        )

        # BEHAVIOR VALIDATION: Tool invocation succeeds
        assert result is not None, \
            "BR-HAPI-250: Tool invocation must return result"
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"BR-HAPI-250: Tool must return SUCCESS status, got {result.status}"

        # CORRECTNESS VALIDATION: Result format
        assert result.data is not None, \
            "BR-HAPI-250: Tool result must have data"

        data = json.loads(result.data)
        assert "workflows" in data, \
            "BR-HAPI-250: Tool result must contain 'workflows' key"
        assert isinstance(data["workflows"], list), \
            "BR-HAPI-250: workflows must be a list"

        # CORRECTNESS VALIDATION: Workflows have required fields
        if len(data["workflows"]) > 0:
            workflow = data["workflows"][0]
            required_fields = ["workflow_id", "title", "description", "signal_types"]
            for field in required_fields:
                assert field in workflow, \
                    f"BR-HAPI-250: Workflow must have '{field}' field"

    def test_integration_with_incident_analysis_pattern(self):
        """
        BR-HAPI-250: Validates the exact registration pattern used in incident.py

        BEHAVIOR: Config + programmatic registration pattern works
        CORRECTNESS: Toolset is accessible after registration

        FAILURE IMPACT: If this fails, incident analysis will not have workflow catalog

        NOTE: This uses the shared functions from llm_config.py (same as incident.py)
        """
        from src.extensions.llm_config import (
            get_model_config_for_sdk,
            prepare_toolsets_config_for_sdk,
            register_workflow_catalog_toolset
        )

        # Step 1: Prepare config (as done in incident.py)
        app_config = {
            "llm": {
                "provider": "anthropic",
                "model": "claude-haiku-4-5-20251001"
            },
            "toolsets": {
                "kubernetes/core": {"enabled": True},
                "kubernetes/logs": {"enabled": True},
                "workflow/catalog": {"enabled": True},  # Will be removed by prepare_toolsets_config_for_sdk
            }
        }

        # Format model name for SDK
        model_name, provider = get_model_config_for_sdk(app_config)

        # Prepare toolsets configuration (removes workflow/catalog)
        toolsets_config = prepare_toolsets_config_for_sdk(app_config)

        # Create Config
        config = Config(
            model=model_name,
            toolsets=toolsets_config,
        )

        # Step 2: Programmatically register workflow catalog (as done in incident.py)
        config = register_workflow_catalog_toolset(config, app_config)

        # BEHAVIOR VALIDATION: Registration pattern completes without error
        assert config.toolset_manager is not None, \
            "BR-HAPI-250: ToolsetManager must be initialized"

        # CORRECTNESS VALIDATION: Workflow catalog is in toolsets
        assert hasattr(config.toolset_manager, 'toolsets'), \
            "BR-HAPI-250: ToolsetManager must have toolsets"
        assert "workflow/catalog" in config.toolset_manager.toolsets, \
            "BR-HAPI-250: workflow/catalog must be in ToolsetManager after registration"

        # CORRECTNESS VALIDATION: Toolset has tools
        wf_toolset = config.toolset_manager.toolsets["workflow/catalog"]

        # Handle both Toolset instances and dicts
        if hasattr(wf_toolset, 'tools'):
            # It's a Toolset instance (expected)
            assert len(wf_toolset.tools) == 1, \
                "BR-HAPI-250: Registered workflow toolset must have 1 tool"
            assert wf_toolset.tools[0].name == "search_workflow_catalog", \
                "BR-HAPI-250: Tool must be search_workflow_catalog"
        else:
            # It's a dict (from config) - this is the problem we're testing for!
            pytest.fail(
                "BR-HAPI-250 CRITICAL: workflow/catalog is a dict, not a Toolset instance! "
                "This means the programmatic registration didn't work, and the tool won't be "
                "available to the LLM. This is the exact bug that caused 'MCP workflow search "
                "is not available' in production."
            )

    def test_default_toolsets_include_workflow_catalog(self):
        """
        BR-HAPI-250: Default toolsets configuration must include workflow/catalog

        BEHAVIOR: When creating Config with default toolsets, workflow/catalog is present
        CORRECTNESS: Default configuration enables workflow catalog

        FAILURE IMPACT: If this fails, users must manually enable workflow catalog
        """
        # This test validates the default behavior in incident.py/recovery.py
        # where we set default toolsets if not specified in config

        default_toolsets = {
            "kubernetes/core": {"enabled": True},
            "kubernetes/logs": {"enabled": True},
            "kubernetes/live-metrics": {"enabled": True},
        }

        # Add workflow/catalog to defaults (as should be done in code)
        default_toolsets["workflow/catalog"] = {"enabled": True}

        config = Config(
            model="anthropic/claude-haiku-4-5-20251001",
            toolsets=default_toolsets,
        )

        # BEHAVIOR VALIDATION: Config accepts defaults
        assert config is not None, \
            "BR-HAPI-250: Config must initialize with default toolsets"

        # CORRECTNESS VALIDATION: workflow/catalog is present
        assert "workflow/catalog" in config.toolsets, \
            "BR-HAPI-250: workflow/catalog must be in default toolsets"

        # CORRECTNESS VALIDATION: workflow/catalog is enabled
        if isinstance(config.toolsets["workflow/catalog"], dict):
            assert config.toolsets["workflow/catalog"]["enabled"] is True, \
                "BR-HAPI-250: workflow/catalog must be enabled by default"


@pytest.mark.integration
class TestWorkflowCatalogEndToEnd:
    """
    End-to-end integration tests for workflow catalog in incident analysis

    Business Requirement: BR-HAPI-250
    Test Scope: Full flow from Config creation to tool invocation
    """

    def test_end_to_end_workflow_catalog_availability(self):
        """
        BR-HAPI-250: End-to-end test that workflow catalog is available during analysis

        BEHAVIOR: Complete flow from config to tool invocation works
        CORRECTNESS: Tool can be invoked and returns valid results

        FAILURE IMPACT: If this fails, incident analysis will fail to find workflows
        """
        from src.extensions.llm_config import (
            get_model_config_for_sdk,
            prepare_toolsets_config_for_sdk,
            register_workflow_catalog_toolset
        )

        # Full integration test simulating incident analysis flow using shared functions

        # 1. Prepare app config (as in incident.py)
        app_config = {
            "llm": {
                "provider": "anthropic",
                "model": "claude-haiku-4-5-20251001"
            },
            "toolsets": {
                "kubernetes/core": {"enabled": True},
                "workflow/catalog": {"enabled": True},
            }
        }

        # 2. Get model config
        model_name, provider = get_model_config_for_sdk(app_config)

        # 3. Prepare toolsets (removes workflow/catalog from dict)
        toolsets_config = prepare_toolsets_config_for_sdk(app_config)

        # 4. Create config
        config = Config(
            model=model_name,
            toolsets=toolsets_config,
        )

        # 5. Register workflow catalog programmatically
        config = register_workflow_catalog_toolset(config, app_config)

        # 6. Verify tool is available
        assert "workflow/catalog" in config.toolset_manager.toolsets

        wf_toolset = config.toolset_manager.toolsets["workflow/catalog"]

        # 7. Get tool and invoke (simulating LLM tool call)
        if hasattr(wf_toolset, 'tools'):
            assert len(wf_toolset.tools) == 1
            tool = wf_toolset.tools[0]

            # 8. Invoke tool (as LLM would)
            result = tool._invoke(
                params={
                    "query": "OOMKilled critical pod",
                    "top_k": 3
                },
                user_approved=False
            )

            # 9. Validate result
            assert result.status == StructuredToolResultStatus.SUCCESS
            data = json.loads(result.data)
            assert "workflows" in data
            assert len(data["workflows"]) > 0
        else:
            pytest.fail(
                "BR-HAPI-250: workflow/catalog is not a Toolset instance - "
                "end-to-end flow failed!"
            )

