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
Cycle 2.4: session_state wiring in registration code

ADR-056 v1.4: register_resource_context_toolset accepts session_state so
that get_resource_context can detect labels and store them in session_state.
register_workflow_discovery_toolset also accepts session_state so discovery
tools can read labels written by resource_context (no k8s_client needed --
labels come from session_state exclusively).

Authority: ADR-056 v1.4, DD-HAPI-017 Section 8
Business Requirement: BR-HAPI-102

Test IDs:
  UT-HAPI-056-059: register_workflow_discovery_toolset passes session_state
  UT-HAPI-056-060: register_resource_context_toolset accepts session_state
  UT-HAPI-056-061: Both registrations share the same session_state dict instance
  UT-HAPI-056-062: session_state flows to individual tools in discovery toolset
"""

import pytest
from unittest.mock import patch, MagicMock, AsyncMock, ANY


class TestSessionStateWiring:
    """
    UT-HAPI-056-059 through UT-HAPI-056-062: Verify session_state wiring
    from registration functions through to Toolset constructors.
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

    def test_ut_hapi_056_060_resource_context_accepts_session_state(self):
        """UT-HAPI-056-060: register_resource_context_toolset accepts session_state for label storage."""
        from src.extensions.llm_config import register_resource_context_toolset
        import inspect

        sig = inspect.signature(register_resource_context_toolset)
        param_names = set(sig.parameters.keys())
        assert "session_state" in param_names

    @patch("src.clients.k8s_client.get_k8s_client")
    @patch("src.toolsets.resource_context.ResourceContextToolset")
    @patch("src.clients.datastorage_auth_session.create_workflow_discovery_session")
    @patch("src.toolsets.workflow_discovery.WorkflowDiscoveryToolset")
    def test_ut_hapi_056_061_both_share_same_session_state(
        self, mock_discovery_cls, mock_discovery_session, mock_context_cls, mock_k8s
    ):
        """UT-HAPI-056-061: Both registrations share the same session_state dict instance."""
        from src.extensions.llm_config import (
            register_resource_context_toolset,
            register_workflow_discovery_toolset,
        )

        mock_k8s.return_value = MagicMock()
        mock_discovery_session.return_value = MagicMock()

        mock_context_instance = MagicMock()
        mock_context_instance.name = "resource_context"
        mock_context_instance.tools = [MagicMock()]
        mock_context_cls.return_value = mock_context_instance

        mock_discovery_instance = MagicMock()
        mock_discovery_instance.enabled = True
        mock_discovery_instance.tools = [MagicMock(), MagicMock(), MagicMock()]
        mock_discovery_cls.return_value = mock_discovery_instance

        config = MagicMock()
        config.toolset_manager = MagicMock()
        config.toolset_manager.list_server_toolsets = MagicMock(return_value=[])

        session_state = {}
        register_resource_context_toolset(config, session_state=session_state)
        register_workflow_discovery_toolset(config, session_state=session_state)

        context_kwargs = mock_context_cls.call_args.kwargs
        discovery_kwargs = mock_discovery_cls.call_args.kwargs
        assert context_kwargs.get("session_state") is session_state
        assert discovery_kwargs.get("session_state") is session_state
        assert context_kwargs["session_state"] is discovery_kwargs["session_state"]

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
