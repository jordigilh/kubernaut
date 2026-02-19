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
Cycle 2.4: session_state and k8s_client wiring in registration code

ADR-056 SoC Refactoring: register_resource_context_toolset no longer
accepts session_state (label detection moved to workflow discovery).
register_workflow_discovery_toolset now also accepts k8s_client +
resource identity for on-demand label detection.

Authority: ADR-056, DD-HAPI-017 Section 8
Business Requirement: BR-HAPI-102

Test IDs:
  UT-HAPI-056-059: register_workflow_discovery_toolset passes session_state
  UT-HAPI-056-060: register_resource_context_toolset no longer accepts session_state
  UT-HAPI-056-061: register_workflow_discovery_toolset passes k8s_client + resource identity
  UT-HAPI-056-062: session_state flows to individual tools in discovery toolset
"""

import pytest
from unittest.mock import patch, MagicMock, AsyncMock, ANY


class TestSessionStateWiring:
    """
    UT-HAPI-056-059 through UT-HAPI-056-062: Verify session_state and k8s_client
    wiring from registration functions through to Toolset constructors.
    """

    @patch("src.clients.datastorage_auth_session.create_workflow_discovery_session")
    @patch("src.toolsets.workflow_discovery.WorkflowDiscoveryToolset")
    def test_ut_hapi_056_059_register_discovery_passes_session_state(
        self, mock_cls, mock_session
    ):
        """UT-HAPI-056-059: register_workflow_discovery_toolset passes session_state to toolset."""
        from src.extensions.llm_config import register_workflow_discovery_toolset

        mock_session.return_value = MagicMock()
        mock_instance = MagicMock()
        mock_instance.enabled = True
        mock_instance.tools = [MagicMock(), MagicMock(), MagicMock()]
        mock_cls.return_value = mock_instance

        config = MagicMock()
        config.toolset_manager = MagicMock()
        config.toolset_manager.list_server_toolsets = MagicMock(return_value=[])

        session_state = {}
        register_workflow_discovery_toolset(
            config, session_state=session_state, severity="critical",
        )

        mock_cls.assert_called_once()
        call_kwargs = mock_cls.call_args.kwargs
        assert call_kwargs.get("session_state") is session_state

    def test_ut_hapi_056_060_resource_context_no_session_state(self):
        """UT-HAPI-056-060: register_resource_context_toolset no longer accepts session_state."""
        from src.extensions.llm_config import register_resource_context_toolset
        import inspect

        sig = inspect.signature(register_resource_context_toolset)
        param_names = set(sig.parameters.keys())
        assert "session_state" not in param_names

    @patch("src.clients.datastorage_auth_session.create_workflow_discovery_session")
    @patch("src.toolsets.workflow_discovery.WorkflowDiscoveryToolset")
    def test_ut_hapi_056_061_register_discovery_passes_k8s_client(
        self, mock_cls, mock_session
    ):
        """UT-HAPI-056-061: register_workflow_discovery_toolset passes k8s_client + resource identity."""
        from src.extensions.llm_config import register_workflow_discovery_toolset

        mock_session.return_value = MagicMock()
        mock_instance = MagicMock()
        mock_instance.enabled = True
        mock_instance.tools = [MagicMock(), MagicMock(), MagicMock()]
        mock_cls.return_value = mock_instance

        config = MagicMock()
        config.toolset_manager = MagicMock()
        config.toolset_manager.list_server_toolsets = MagicMock(return_value=[])

        mock_k8s = AsyncMock()
        register_workflow_discovery_toolset(
            config,
            session_state={},
            severity="critical",
            component="Pod",
            k8s_client=mock_k8s,
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        mock_cls.assert_called_once()
        call_kwargs = mock_cls.call_args.kwargs
        assert call_kwargs.get("k8s_client") is mock_k8s
        assert call_kwargs.get("resource_name") == "api-pod-abc"
        assert call_kwargs.get("resource_namespace") == "production"

    @patch("src.clients.datastorage_auth_session.create_workflow_discovery_session")
    def test_ut_hapi_056_062_session_state_flows_to_discovery_tools(self, mock_session):
        """UT-HAPI-056-062: session_state reaches individual tools in WorkflowDiscoveryToolset."""
        from src.extensions.llm_config import register_workflow_discovery_toolset

        mock_session.return_value = MagicMock()

        config = MagicMock()
        config.toolset_manager = MagicMock()
        config.toolset_manager.list_server_toolsets = MagicMock(return_value=[])

        session_state = {"detected_labels": {"gitOpsManaged": True}}
        register_workflow_discovery_toolset(config, session_state=session_state)

        patched_fn = config.toolset_manager.list_server_toolsets
        toolsets = patched_fn()

        discovery = None
        for ts in toolsets:
            if hasattr(ts, "name") and ts.name == "workflow/discovery":
                discovery = ts
                break

        assert discovery is not None, "WorkflowDiscoveryToolset not found"
        for tool in discovery.tools:
            assert tool._session_state is session_state
