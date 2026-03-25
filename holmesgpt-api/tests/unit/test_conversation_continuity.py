# Copyright 2026 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
Unit tests for BR-HAPI-263: Conversation Continuity.

TDD Group 1: SDK-level conversation continuity.
Tests that the Holmes SDK supports threading messages across
investigation calls for the #529 three-phase RCA architecture.
"""

import pytest
from unittest.mock import MagicMock, patch, AsyncMock
from typing import List, Dict, Optional

from holmes.core.models import InvestigationResult, InvestigateRequest


class TestSDKConversationContinuity:
    """G1: SDK conversation continuity (BR-HAPI-263)."""

    def test_ut_hapi_263_001_investigate_issues_accepts_previous_messages(self):
        """UT-HAPI-263-001: investigate_issues accepts previous_messages parameter.

        BR-HAPI-263: The SDK investigate_issues function must accept an optional
        previous_messages parameter to seed the LLM conversation with prior context.
        """
        from holmes.core.investigation import investigate_issues
        import inspect

        sig = inspect.signature(investigate_issues)
        assert "previous_messages" in sig.parameters, (
            "investigate_issues must accept 'previous_messages' parameter "
            "for conversation continuity (BR-HAPI-263)"
        )
        param = sig.parameters["previous_messages"]
        assert param.default is None, (
            "previous_messages must default to None for backward compatibility"
        )

    def test_ut_hapi_263_002_investigation_result_includes_messages(self):
        """UT-HAPI-263-002: InvestigationResult includes messages list.

        BR-HAPI-263: InvestigationResult must expose the full message history
        from the LLM conversation so HAPI can thread it to the next phase.
        """
        result = InvestigationResult(
            analysis="test analysis",
            messages=[
                {"role": "system", "content": "system prompt"},
                {"role": "user", "content": "investigate this"},
                {"role": "assistant", "content": "analysis result"},
            ],
        )
        assert result.messages is not None
        assert len(result.messages) == 3
        assert result.messages[0]["role"] == "system"
        assert result.messages[2]["role"] == "assistant"

    def test_ut_hapi_263_004_none_previous_messages_backward_compatible(self):
        """UT-HAPI-263-004: None previous_messages preserves current behavior.

        BR-HAPI-263: When previous_messages is None (default), the SDK must
        behave identically to the current implementation — no conversation
        seeding, fresh investigation from scratch.
        """
        result = InvestigationResult(analysis="test")
        assert result.messages is None, (
            "Default InvestigationResult must have messages=None "
            "for backward compatibility"
        )
