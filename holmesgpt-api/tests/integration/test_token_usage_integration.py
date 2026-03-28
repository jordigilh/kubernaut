#
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
#

"""
Integration tests for LLM token usage in audit traces (#435).

Test Plan: docs/tests/435/TEST_PLAN.md
Issue: https://github.com/jordigilh/kubernaut/issues/435

Tests the full wiring between:
  - TokenAccumulator (ContextVar lifecycle)
  - KubernautLiteLLMCallback (accumulation)
  - audit event factories (token enrichment)
  - investigation_helpers (tokens_used passthrough)

Mock LLM: always mocked per TESTING_GUIDELINES.md
Audit store: spy capture (no external DataStorage required)
"""

import pytest
from datetime import datetime, timezone
from unittest.mock import MagicMock
from prometheus_client import CollectorRegistry

from src.metrics.token_accumulator import (
    TokenAccumulator, set_token_accumulator, get_token_accumulator,
)
from src.metrics.instrumentation import HAMetrics
from src.metrics.litellm_callback import KubernautLiteLLMCallback
from src.audit.events import (
    create_aiagent_response_complete_event,
    create_llm_response_event,
)


def _make_usage(prompt_tokens=0, completion_tokens=0):
    """Build a mock Usage object matching litellm.types.utils.Usage shape."""
    usage = MagicMock()
    usage.prompt_tokens = prompt_tokens
    usage.completion_tokens = completion_tokens
    usage.total_tokens = prompt_tokens + completion_tokens
    return usage


def _make_response(model="vertex_ai/claude-sonnet-4-20250514", usage=None):
    """Build a mock ModelResponse matching litellm response shape."""
    resp = MagicMock()
    resp.model = model
    resp.usage = usage
    return resp


def _minimal_response_data():
    """Minimal valid IncidentResponse dict for audit event tests."""
    return {
        "incidentId": "inc-it-435",
        "analysis": "Root cause: OOMKilled pod due to memory limit",
        "rootCauseAnalysis": {
            "summary": "Memory limit exceeded",
            "severity": "high",
            "contributingFactors": ["pod memory limit too low"],
        },
        "confidence": 0.85,
        "timestamp": "2026-03-04T10:00:00Z",
    }


class TestTokenUsageIntegration:
    """IT-HAPI-435-001 and IT-HAPI-435-002: End-to-end token usage wiring."""

    def test_it_hapi_435_001_full_flow_response_complete_with_tokens(self):
        """IT-HAPI-435-001: Full investigation flow emits
        aiagent.response.complete audit event with accumulated token totals.

        Simulates: endpoint sets ContextVar -> LiteLLM callback fires twice
        -> endpoint reads totals -> audit event enriched with tokens.
        """
        registry = CollectorRegistry()
        metrics = HAMetrics(registry=registry)
        cb = KubernautLiteLLMCallback(metrics)

        acc = TokenAccumulator()
        set_token_accumulator(acc)
        try:
            start = datetime(2026, 1, 1, tzinfo=timezone.utc)
            end = datetime(2026, 1, 1, 0, 0, 2, tzinfo=timezone.utc)

            cb.log_success_event(
                {"model": "vertex_ai/claude-sonnet-4-20250514"},
                _make_response(usage=_make_usage(300, 150)),
                start, end,
            )
            cb.log_success_event(
                {"model": "vertex_ai/claude-sonnet-4-20250514"},
                _make_response(usage=_make_usage(200, 50)),
                start, end,
            )

            assert acc.prompt_tokens == 500
            assert acc.completion_tokens == 200

            event = create_aiagent_response_complete_event(
                incident_id="inc-it-435",
                remediation_id="rem-it-435",
                response_data=_minimal_response_data(),
                total_prompt_tokens=acc.prompt_tokens,
                total_completion_tokens=acc.completion_tokens,
            )

            assert event.event_type == "aiagent.response.complete"
            payload = event.event_data.actual_instance
            assert payload.total_prompt_tokens == 500
            assert payload.total_completion_tokens == 200
        finally:
            set_token_accumulator(None)

    def test_it_hapi_435_002_llm_response_with_tokens_used(self):
        """IT-HAPI-435-002: aiagent.llm.response event contains tokens_used
        populated from accumulated LLM calls."""
        registry = CollectorRegistry()
        metrics = HAMetrics(registry=registry)
        cb = KubernautLiteLLMCallback(metrics)

        acc = TokenAccumulator()
        set_token_accumulator(acc)
        try:
            start = datetime(2026, 1, 1, tzinfo=timezone.utc)
            end = datetime(2026, 1, 1, 0, 0, 3, tzinfo=timezone.utc)

            cb.log_success_event(
                {"model": "vertex_ai/claude-sonnet-4-20250514"},
                _make_response(usage=_make_usage(400, 300)),
                start, end,
            )

            event = create_llm_response_event(
                incident_id="inc-it-435",
                remediation_id="rem-it-435",
                has_analysis=True,
                analysis_length=500,
                analysis_preview="Root cause analysis...",
                tool_call_count=2,
                tokens_used=acc.total(),
            )

            assert event.event_type == "aiagent.llm.response"
            payload = event.event_data.actual_instance
            assert payload.tokens_used == 700
        finally:
            set_token_accumulator(None)

    def test_it_hapi_435_003_multi_attempt_delta_and_session_total(self):
        """IT-HAPI-435-003: Multi-attempt investigation produces correct
        per-call deltas on aiagent.llm.response and correct session total
        on aiagent.response.complete."""
        registry = CollectorRegistry()
        metrics = HAMetrics(registry=registry)
        cb = KubernautLiteLLMCallback(metrics)

        acc = TokenAccumulator()
        set_token_accumulator(acc)
        try:
            start = datetime(2026, 1, 1, tzinfo=timezone.utc)
            end = datetime(2026, 1, 1, 0, 0, 2, tzinfo=timezone.utc)

            llm_response_events = []
            for prompt_tok, comp_tok in [(300, 150), (200, 100)]:
                snapshot = acc.total()
                cb.log_success_event(
                    {"model": "vertex_ai/claude-sonnet-4-20250514"},
                    _make_response(usage=_make_usage(prompt_tok, comp_tok)),
                    start, end,
                )
                delta = acc.total() - snapshot

                event = create_llm_response_event(
                    incident_id="inc-it-435-multi",
                    remediation_id="rem-it-435-multi",
                    has_analysis=True,
                    analysis_length=200,
                    analysis_preview="Analysis...",
                    tool_call_count=1,
                    tokens_used=delta,
                )
                llm_response_events.append(event)

            assert llm_response_events[0].event_data.actual_instance.tokens_used == 450
            assert llm_response_events[1].event_data.actual_instance.tokens_used == 300

            complete_event = create_aiagent_response_complete_event(
                incident_id="inc-it-435-multi",
                remediation_id="rem-it-435-multi",
                response_data=_minimal_response_data(),
                total_prompt_tokens=acc.prompt_tokens,
                total_completion_tokens=acc.completion_tokens,
            )

            payload = complete_event.event_data.actual_instance
            assert payload.total_prompt_tokens == 500
            assert payload.total_completion_tokens == 250
        finally:
            set_token_accumulator(None)
