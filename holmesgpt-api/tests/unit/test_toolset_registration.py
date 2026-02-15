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
Toolset Registration Unit Tests

Business Requirement: BR-HAPI-017-001 (Three-Step Tool Implementation)
Design Decision: DD-HAPI-017 (Three-Step Workflow Discovery Integration)

Tests the registration of WorkflowDiscoveryToolset into the HolmesGPT SDK
via register_workflow_discovery_toolset(), and verifies that
prepare_toolsets_config_for_sdk() properly excludes both catalog and discovery
toolsets from the config dict (they are registered programmatically).

TDD Phase: RED — these tests define the contract for Phase 3e.
"""

import pytest
from unittest.mock import ANY, Mock, patch

# Patch target for WorkflowDiscoveryToolset (source module)
DISCOVERY_TOOLSET_CLASS = "src.toolsets.workflow_discovery.WorkflowDiscoveryToolset"
# Patch target for sanitization wrapper (avoid real tool wrapping in tests)
SANITIZATION_WRAPPER = "src.extensions.llm_config._wrap_tool_results_with_sanitization"
# Patch target for session factory (avoid filesystem access in unit tests)
SESSION_FACTORY = "src.extensions.llm_config.create_workflow_discovery_session"


def _make_mock_discovery_toolset():
    """Create a mock that behaves like a WorkflowDiscoveryToolset instance."""
    toolset = Mock()
    toolset.name = "workflow/discovery"
    toolset.enabled = True
    toolset.tools = [Mock(), Mock(), Mock()]  # Three tools
    return toolset


def _make_mock_config():
    """Create a mock HolmesGPT Config with toolset_manager and create_tool_executor."""
    config = Mock()
    config.toolsets = {}
    config.toolset_manager = Mock()
    config.toolset_manager.list_server_toolsets = Mock(return_value=[])
    # create_tool_executor returns an executor with a real list for toolsets
    mock_executor = Mock()
    mock_executor.toolsets = []
    config.create_tool_executor = Mock(return_value=mock_executor)
    return config


class TestRegisterWorkflowDiscoveryToolset:
    """
    Tests for register_workflow_discovery_toolset function.

    Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
    Business Requirement: BR-HAPI-017-001

    The function creates a WorkflowDiscoveryToolset with signal context
    filters and injects it into the HolmesGPT SDK via monkey-patching.
    """

    def test_function_exists_and_is_callable_ut_reg_001(self):
        """
        UT-HAPI-017-REG-001: register_workflow_discovery_toolset exists in llm_config.

        BR: BR-HAPI-017-001
        Type: Unit / Smoke
        The function must be importable from src.extensions.llm_config and callable.
        """
        from src.extensions.llm_config import register_workflow_discovery_toolset
        assert callable(register_workflow_discovery_toolset)

    def test_creates_toolset_with_context_filters_ut_reg_002(self):
        """
        UT-HAPI-017-REG-002: Function passes context filters to WorkflowDiscoveryToolset.

        BR: BR-HAPI-017-001
        Type: Unit / Happy Path
        Signal context filters (severity, component, environment, priority) and
        remediation_id must be forwarded to the WorkflowDiscoveryToolset constructor.
        DD-AUTH-005: http_session (ServiceAccountAuthSession) must be provided.
        """
        with (
            patch(DISCOVERY_TOOLSET_CLASS) as mock_cls,
            patch(SANITIZATION_WRAPPER),
            patch(SESSION_FACTORY) as mock_session_factory,
        ):
            mock_cls.return_value = _make_mock_discovery_toolset()
            mock_session = Mock()
            mock_session_factory.return_value = mock_session

            from src.extensions.llm_config import register_workflow_discovery_toolset

            register_workflow_discovery_toolset(
                _make_mock_config(),
                app_config={},
                remediation_id="rem-abc-123",
                custom_labels={"team": ["payments"]},
                severity="critical",
                component="Deployment",
                environment="production",
                priority="P0",
            )

            # Assert session factory was called (DD-AUTH-005)
            mock_session_factory.assert_called_once()

            # Assert WorkflowDiscoveryToolset was created with correct params
            mock_cls.assert_called_once_with(
                enabled=True,
                remediation_id="rem-abc-123",
                severity="critical",
                component="Deployment",
                environment="production",
                priority="P0",
                custom_labels={"team": ["payments"]},
                detected_labels=None,
                http_session=mock_session,
            )

    def test_injects_via_list_server_toolsets_ut_reg_003(self):
        """
        UT-HAPI-017-REG-003: Function injects toolset into list_server_toolsets.

        BR: BR-HAPI-017-001
        Type: Unit / Happy Path
        After registration, calling list_server_toolsets() on the config's
        toolset_manager must include a toolset named "workflow/discovery".
        """
        with (
            patch(DISCOVERY_TOOLSET_CLASS) as mock_cls,
            patch(SANITIZATION_WRAPPER),
            patch(SESSION_FACTORY),
        ):
            mock_cls.return_value = _make_mock_discovery_toolset()
            config = _make_mock_config()

            from src.extensions.llm_config import register_workflow_discovery_toolset

            result_config = register_workflow_discovery_toolset(
                config,
                severity="critical",
                component="Deployment",
                environment="production",
                priority="P0",
            )

            # After registration, list_server_toolsets should include discovery
            toolsets = result_config.toolset_manager.list_server_toolsets()
            toolset_names = [
                ts.name for ts in toolsets if hasattr(ts, "name")
            ]
            assert "workflow/discovery" in toolset_names

    def test_injects_via_create_tool_executor_ut_reg_004(self):
        """
        UT-HAPI-017-REG-004: Function injects toolset into create_tool_executor.

        BR: BR-HAPI-017-001
        Type: Unit / Happy Path
        After registration, calling create_tool_executor() on the config must
        return an executor whose toolsets list includes "workflow/discovery".
        This is the layer the LLM actually sees.
        """
        with (
            patch(DISCOVERY_TOOLSET_CLASS) as mock_cls,
            patch(SANITIZATION_WRAPPER),
            patch(SESSION_FACTORY),
        ):
            mock_discovery = _make_mock_discovery_toolset()
            mock_cls.return_value = mock_discovery
            config = _make_mock_config()

            from src.extensions.llm_config import register_workflow_discovery_toolset

            result_config = register_workflow_discovery_toolset(
                config,
                severity="high",
                component="Pod",
                environment="staging",
                priority="P1",
            )

            # Call create_tool_executor — should include discovery toolset
            executor = result_config.create_tool_executor(dal=None)
            toolset_names = [
                ts.name for ts in executor.toolsets if hasattr(ts, "name")
            ]
            assert "workflow/discovery" in toolset_names

    def test_returns_same_config_instance_ut_reg_005(self):
        """
        UT-HAPI-017-REG-005: Function returns the same Config instance.

        BR: BR-HAPI-017-001
        Type: Unit / Contract
        The function modifies config in-place via monkey-patching and returns it.
        """
        with (
            patch(DISCOVERY_TOOLSET_CLASS) as mock_cls,
            patch(SANITIZATION_WRAPPER),
            patch(SESSION_FACTORY),
        ):
            mock_cls.return_value = _make_mock_discovery_toolset()
            config = _make_mock_config()

            from src.extensions.llm_config import register_workflow_discovery_toolset

            result = register_workflow_discovery_toolset(config, severity="high")
            assert result is config


class TestPrepareToolsetsRemovesDiscovery:
    """
    Tests that prepare_toolsets_config_for_sdk excludes workflow/discovery.

    Both workflow/catalog and workflow/discovery must be removed from the
    config dict because they are registered programmatically as Toolset
    instances via their respective registration functions.
    """

    def test_removes_workflow_discovery_from_config_ut_reg_006(self):
        """
        UT-HAPI-017-REG-006: prepare_toolsets_config_for_sdk removes workflow/discovery.

        BR: BR-HAPI-017-001
        Type: Unit / Happy Path
        Both "workflow/catalog" and "workflow/discovery" must be absent from
        the returned config dict.
        """
        from src.extensions.llm_config import prepare_toolsets_config_for_sdk

        app_config = {
            "toolsets": {
                "kubernetes/core": {"enabled": True},
                "workflow/catalog": {"enabled": True},
                "workflow/discovery": {"enabled": True},
            }
        }
        result = prepare_toolsets_config_for_sdk(app_config)

        assert "workflow/catalog" not in result
        assert "workflow/discovery" not in result
        assert "kubernetes/core" in result
