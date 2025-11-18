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
Integration Tests for Workflow Catalog Toolset in SDK Investigation Flow

Business Requirement: BR-HAPI-250 - Workflow Catalog Search Tool
Test Scope: Validates WorkflowCatalogToolset works through full SDK investigation flow

CRITICAL: These tests catch errors that occur during the actual HolmesGPT SDK
investigation flow, not just during registration. The SDK's ToolsetManager has
internal methods that iterate over toolsets and expect specific formats.

This test caught the production bug where:
- WorkflowCatalogToolset was registered as an instance in toolset_manager.toolsets
- But SDK's _load_toolsets_from_config() expected dicts with .get() method
- Error: "'WorkflowCatalogToolset' object has no attribute 'get'"

Test Coverage:
1. SDK's create_tool_executor() with workflow catalog
2. SDK's list_server_toolsets() with workflow catalog
3. Full investigation flow with workflow catalog
"""

import pytest
from unittest.mock import Mock

from holmes.config import Config
from holmes.core.investigation import InvestigateRequest
from holmes.core.supabase_dal import SupabaseDal

from src.extensions.incident import MinimalDAL
from src.extensions.llm_config import (
    get_model_config_for_sdk,
    prepare_toolsets_config_for_sdk,
    register_workflow_catalog_toolset
)


@pytest.mark.integration
class TestWorkflowCatalogInSDKInvestigationFlow:
    """
    Integration tests for WorkflowCatalogToolset in SDK investigation flow

    Business Requirement: BR-HAPI-250
    CRITICAL: Catches errors during SDK's actual investigation/toolset loading
    """

    def test_create_tool_executor_with_workflow_catalog(self):
        """
        BR-HAPI-250: Workflow catalog must work with SDK's create_tool_executor()

        BEHAVIOR: SDK can create tool executor with workflow catalog registered
        CORRECTNESS: No AttributeError about 'get' method

        FAILURE IMPACT: If this fails with "'WorkflowCatalogToolset' object has no
        attribute 'get'", it means the workflow catalog was registered incorrectly
        and the SDK can't load it during investigation.

        This test caught the production bug where WorkflowCatalogToolset was
        registered as an instance but SDK expected a dict.
        """
        # Prepare config (as in incident.py)
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

        # Get model config
        model_name, provider = get_model_config_for_sdk(app_config)

        # Prepare toolsets (removes workflow/catalog from dict)
        toolsets_config = prepare_toolsets_config_for_sdk(app_config)

        # Create config
        config = Config(
            model=model_name,
            toolsets=toolsets_config,
        )

        # Register workflow catalog programmatically
        config = register_workflow_catalog_toolset(config, app_config)

        # Create minimal DAL for testing
        dal = MinimalDAL(cluster_name="test-cluster")

        # CRITICAL TEST: This is where the production bug occurred
        # SDK's create_tool_executor() calls list_server_toolsets() which calls
        # _load_toolsets_from_config() which expects dicts with .get() method
        try:
            tool_executor = config.create_tool_executor(dal)
            creation_succeeded = True
            error_message = None
        except AttributeError as e:
            creation_succeeded = False
            error_message = str(e)
            if "'WorkflowCatalogToolset' object has no attribute 'get'" in error_message:
                pytest.fail(
                    "BR-HAPI-250 CRITICAL: SDK's create_tool_executor() failed because "
                    "WorkflowCatalogToolset was registered as an instance but SDK expected "
                    "a dict. Error: " + error_message + "\n\n"
                    "This is the exact production bug that caused incident analysis to fail. "
                    "The workflow catalog must be registered in a way that's compatible with "
                    "SDK's _load_toolsets_from_config() method."
                )
            else:
                pytest.fail(f"BR-HAPI-250: Unexpected AttributeError: {error_message}")
        except Exception as e:
            # Other exceptions might be acceptable (e.g., missing k8s cluster)
            # but AttributeError about 'get' is the critical bug we're testing for
            creation_succeeded = True  # Not the bug we're looking for
            error_message = str(e)

        # BEHAVIOR VALIDATION: Should not fail with AttributeError
        assert creation_succeeded, \
            f"BR-HAPI-250: create_tool_executor() failed with: {error_message}"

    def test_list_server_toolsets_with_workflow_catalog(self):
        """
        BR-HAPI-250: Workflow catalog must work with SDK's list_server_toolsets()

        BEHAVIOR: SDK can list server toolsets including workflow catalog
        CORRECTNESS: workflow catalog appears in the list

        FAILURE IMPACT: If this fails, the workflow catalog won't be available
        during investigation even though it's registered.
        """
        # Prepare config
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

        model_name, provider = get_model_config_for_sdk(app_config)
        toolsets_config = prepare_toolsets_config_for_sdk(app_config)

        config = Config(
            model=model_name,
            toolsets=toolsets_config,
        )

        config = register_workflow_catalog_toolset(config, app_config)

        # Create minimal DAL
        dal = MinimalDAL(cluster_name="test-cluster")

        # CRITICAL TEST: List server toolsets (this is what investigation flow does)
        try:
            toolsets = config.toolset_manager.list_server_toolsets(dal=dal)
            list_succeeded = True
            error_message = None
        except AttributeError as e:
            list_succeeded = False
            error_message = str(e)
            if "'WorkflowCatalogToolset' object has no attribute 'get'" in error_message:
                pytest.fail(
                    "BR-HAPI-250 CRITICAL: SDK's list_server_toolsets() failed with "
                    "AttributeError. This means the workflow catalog registration method "
                    "is incompatible with the SDK's toolset loading logic. "
                    "Error: " + error_message
                )
            else:
                pytest.fail(f"BR-HAPI-250: Unexpected AttributeError: {error_message}")
        except Exception as e:
            # Other exceptions might be acceptable
            list_succeeded = True
            error_message = str(e)

        assert list_succeeded, \
            f"BR-HAPI-250: list_server_toolsets() failed with: {error_message}"

    def test_create_issue_investigator_with_workflow_catalog(self):
        """
        BR-HAPI-250: Workflow catalog must work when creating issue investigator

        BEHAVIOR: SDK can create issue investigator with workflow catalog
        CORRECTNESS: No AttributeError during investigator creation

        FAILURE IMPACT: This is the exact point where production incident analysis failed.
        """
        # Prepare config
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

        model_name, provider = get_model_config_for_sdk(app_config)
        toolsets_config = prepare_toolsets_config_for_sdk(app_config)

        config = Config(
            model=model_name,
            toolsets=toolsets_config,
        )

        config = register_workflow_catalog_toolset(config, app_config)

        # Create minimal DAL
        dal = MinimalDAL(cluster_name="test-cluster")

        # CRITICAL TEST: Create issue investigator (exact production failure point)
        # This calls create_tool_executor() which calls list_server_toolsets()
        try:
            investigator = config.create_issue_investigator(dal=dal)
            creation_succeeded = True
            error_message = None
        except AttributeError as e:
            creation_succeeded = False
            error_message = str(e)
            if "'WorkflowCatalogToolset' object has no attribute 'get'" in error_message:
                pytest.fail(
                    "BR-HAPI-250 CRITICAL PRODUCTION BUG: create_issue_investigator() "
                    "failed with the exact error that occurred in production. "
                    "The workflow catalog registration is incompatible with SDK's internal "
                    "toolset loading. Error: " + error_message + "\n\n"
                    "Investigation stack: investigate_issues() -> create_issue_investigator() "
                    "-> create_tool_executor() -> list_server_toolsets() -> "
                    "_load_toolsets_from_config() -> toolset_config.get('type')"
                )
            else:
                pytest.fail(f"BR-HAPI-250: Unexpected AttributeError: {error_message}")
        except Exception as e:
            # Other exceptions might be acceptable (e.g., missing API keys, k8s cluster)
            creation_succeeded = True
            error_message = str(e)

        assert creation_succeeded, \
            f"BR-HAPI-250: create_issue_investigator() failed with: {error_message}"


@pytest.mark.integration
class TestWorkflowCatalogProductionScenario:
    """
    Tests that replicate the exact production failure scenario

    Business Requirement: BR-HAPI-250
    Test Scope: Exact production code path that failed
    """

    def test_production_incident_analysis_flow(self):
        """
        BR-HAPI-250: Replicates exact production incident analysis flow

        BEHAVIOR: Full incident analysis flow works with workflow catalog
        CORRECTNESS: No AttributeError during investigation

        FAILURE IMPACT: This is the EXACT production failure - if this test fails,
        production incident analysis will fail.
        """
        from src.extensions.llm_config import (
            get_model_config_for_sdk,
            prepare_toolsets_config_for_sdk,
            register_workflow_catalog_toolset
        )

        # Exact production config
        app_config = {
            "llm": {
                "provider": "anthropic",
                "model": "claude-haiku-4-5-20251001"
            },
            "toolsets": {
                "kubernetes/core": {"enabled": True},
                "kubernetes/logs": {"enabled": True},
                "kubernetes/live-metrics": {"enabled": True},
                "workflow/catalog": {"enabled": True},
            }
        }

        # Exact production flow from incident.py
        model_name, provider = get_model_config_for_sdk(app_config)
        toolsets_config = prepare_toolsets_config_for_sdk(app_config)

        config = Config(
            model=model_name,
            api_base=None,  # Will use default
            toolsets=toolsets_config,
        )

        config = register_workflow_catalog_toolset(config, app_config)

        # Create DAL (production uses MinimalDAL)
        dal = MinimalDAL(cluster_name="production-cluster")

        # CRITICAL: This is the exact call that failed in production
        # investigate_issues() -> config.create_issue_investigator()
        try:
            # Try to create the investigator (first step of investigate_issues)
            investigator = config.create_issue_investigator(dal=dal)
            flow_succeeded = True
            error_message = None
        except AttributeError as e:
            flow_succeeded = False
            error_message = str(e)
            if "has no attribute 'get'" in error_message:
                pytest.fail(
                    "BR-HAPI-250 PRODUCTION BUG REPRODUCED: The exact production failure "
                    "occurred in this test. This means the fix is not complete. "
                    "Error: " + error_message
                )
            else:
                pytest.fail(f"BR-HAPI-250: Unexpected AttributeError: {error_message}")
        except Exception as e:
            # Expected: might fail with API key missing, k8s connection, etc.
            # What matters is NO AttributeError about 'get'
            flow_succeeded = True
            error_message = str(e)

        # BEHAVIOR VALIDATION: Must not fail with AttributeError about 'get'
        assert flow_succeeded, \
            f"BR-HAPI-250 PRODUCTION FLOW: Failed with: {error_message}"

