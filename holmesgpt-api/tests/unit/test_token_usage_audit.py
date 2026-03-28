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
Unit tests for LLM token usage in audit traces (#435).

Test Plan: docs/tests/435/TEST_PLAN.md
Issue: https://github.com/jordigilh/kubernaut/issues/435

Authority:
  - DD-AUDIT-003: Service audit trace requirements
  - ADR-034: Unified audit table design
  - BR-HAPI-301: LLM observability metrics
"""

import pytest


# ========================================
# Phase 2: TokenAccumulator Tests
# ========================================

class TestTokenAccumulator:
    """UT-HAPI-435-001 through UT-HAPI-435-004: TokenAccumulator pure logic."""

    def test_ut_hapi_435_001_zero_totals_when_no_calls(self):
        """UT-HAPI-435-001: Accumulator returns zero totals when no LLM calls recorded."""
        from src.metrics.token_accumulator import TokenAccumulator

        acc = TokenAccumulator()
        assert acc.prompt_tokens == 0
        assert acc.completion_tokens == 0
        assert acc.total() == 0

    def test_ut_hapi_435_002_multi_call_accumulation(self):
        """UT-HAPI-435-002: Accumulator correctly sums prompt and completion
        tokens across multiple add() calls."""
        from src.metrics.token_accumulator import TokenAccumulator

        acc = TokenAccumulator()
        acc.add(150, 80)
        acc.add(200, 120)

        assert acc.prompt_tokens == 350
        assert acc.completion_tokens == 200
        assert acc.total() == 550

    def test_ut_hapi_435_003_zero_token_calls_handled(self):
        """UT-HAPI-435-003: Accumulator handles zero-token LLM calls without error."""
        from src.metrics.token_accumulator import TokenAccumulator

        acc = TokenAccumulator()
        acc.add(100, 50)
        acc.add(0, 0)

        assert acc.prompt_tokens == 100
        assert acc.completion_tokens == 50

    def test_ut_hapi_435_004_total_returns_combined_count(self):
        """UT-HAPI-435-004: total() returns combined prompt + completion count."""
        from src.metrics.token_accumulator import TokenAccumulator

        acc = TokenAccumulator()
        acc.add(500, 200)

        assert acc.total() == 700
        assert acc.total() == acc.prompt_tokens + acc.completion_tokens


# ========================================
# Phase 3: Audit Event Token Field Tests
# ========================================

def _minimal_response_data():
    """Minimal valid IncidentResponse dict for audit event tests."""
    return {
        "incidentId": "inc-test-435",
        "analysis": "Root cause: OOMKilled pod",
        "rootCauseAnalysis": {
            "summary": "Memory limit exceeded",
            "severity": "high",
            "contributingFactors": ["pod memory limit too low"],
        },
        "confidence": 0.85,
        "timestamp": "2026-03-04T10:00:00Z",
    }


class TestAuditEventTokenFields:
    """UT-HAPI-435-005 through UT-HAPI-435-008: Audit event token enrichment."""

    def test_ut_hapi_435_005_response_complete_with_tokens(self):
        """UT-HAPI-435-005: aiagent.response.complete event contains
        total_prompt_tokens and total_completion_tokens when provided."""
        from src.audit.events import create_aiagent_response_complete_event

        event = create_aiagent_response_complete_event(
            incident_id="inc-test-435",
            remediation_id="rem-test-435",
            response_data=_minimal_response_data(),
            total_prompt_tokens=350,
            total_completion_tokens=200,
        )

        assert event.event_type == "aiagent.response.complete"
        assert event.event_category == "aiagent"
        assert event.version == "1.0"

        payload = event.event_data.actual_instance
        assert payload.total_prompt_tokens == 350
        assert payload.total_completion_tokens == 200

    def test_ut_hapi_435_006_response_complete_backward_compat(self):
        """UT-HAPI-435-006: aiagent.response.complete event remains ADR-034
        compliant when tokens are None (backward compat)."""
        from src.audit.events import create_aiagent_response_complete_event

        event = create_aiagent_response_complete_event(
            incident_id="inc-test-435",
            remediation_id="rem-test-435",
            response_data=_minimal_response_data(),
        )

        assert event.version == "1.0"
        assert event.event_type == "aiagent.response.complete"
        assert event.correlation_id == "rem-test-435"

        payload = event.event_data.actual_instance
        assert payload.total_prompt_tokens is None
        assert payload.total_completion_tokens is None

    def test_ut_hapi_435_007_llm_response_with_tokens_used(self):
        """UT-HAPI-435-007: aiagent.llm.response event contains tokens_used
        when provided."""
        from src.audit.events import create_llm_response_event

        event = create_llm_response_event(
            incident_id="inc-test-435",
            remediation_id="rem-test-435",
            has_analysis=True,
            analysis_length=500,
            analysis_preview="Root cause analysis...",
            tool_call_count=2,
            tokens_used=550,
        )

        assert event.event_type == "aiagent.llm.response"
        payload = event.event_data.actual_instance
        assert payload.tokens_used == 550

    def test_ut_hapi_435_008_llm_response_tokens_used_none_preserved(self):
        """UT-HAPI-435-008: aiagent.llm.response event handles
        tokens_used=None (existing behavior preserved)."""
        from src.audit.events import create_llm_response_event

        event = create_llm_response_event(
            incident_id="inc-test-435",
            remediation_id="rem-test-435",
            has_analysis=True,
            analysis_length=500,
            analysis_preview="Root cause analysis...",
            tool_call_count=2,
        )

        assert event.event_type == "aiagent.llm.response"
        payload = event.event_data.actual_instance
        assert payload.tokens_used is None


# ========================================
# Phase 4: Callback Accumulator Tests
# ========================================

from datetime import datetime, timezone
from unittest.mock import MagicMock, patch
from prometheus_client import CollectorRegistry


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


class TestCallbackAccumulator:
    """UT-HAPI-435-009 through UT-HAPI-435-011: LiteLLM callback accumulator."""

    def test_ut_hapi_435_009_callback_accumulates_via_contextvar(self):
        """UT-HAPI-435-009: LiteLLM callback accumulates tokens when
        ContextVar has active accumulator."""
        from src.metrics.instrumentation import HAMetrics
        from src.metrics.litellm_callback import KubernautLiteLLMCallback
        from src.metrics.token_accumulator import (
            TokenAccumulator, set_token_accumulator,
        )

        registry = CollectorRegistry()
        metrics = HAMetrics(registry=registry)
        cb = KubernautLiteLLMCallback(metrics)

        acc = TokenAccumulator()
        set_token_accumulator(acc)
        try:
            kwargs = {"model": "vertex_ai/claude-sonnet-4-20250514"}
            response = _make_response(usage=_make_usage(150, 80))
            start = datetime(2026, 1, 1, tzinfo=timezone.utc)
            end = datetime(2026, 1, 1, 0, 0, 2, tzinfo=timezone.utc)

            cb.log_success_event(kwargs, response, start, end)

            assert acc.prompt_tokens == 150
            assert acc.completion_tokens == 80
        finally:
            set_token_accumulator(None)

    def test_ut_hapi_435_010_callback_works_without_accumulator(self):
        """UT-HAPI-435-010: LiteLLM callback works normally when ContextVar
        is empty (no accumulator)."""
        from src.metrics.instrumentation import HAMetrics
        from src.metrics.litellm_callback import KubernautLiteLLMCallback
        from src.metrics.token_accumulator import set_token_accumulator

        registry = CollectorRegistry()
        metrics = HAMetrics(registry=registry)
        cb = KubernautLiteLLMCallback(metrics)

        set_token_accumulator(None)

        kwargs = {"model": "vertex_ai/claude-sonnet-4-20250514"}
        response = _make_response(usage=_make_usage(100, 50))
        start = datetime(2026, 1, 1, tzinfo=timezone.utc)
        end = datetime(2026, 1, 1, 0, 0, 1, tzinfo=timezone.utc)

        cb.log_success_event(kwargs, response, start, end)

        val = registry.get_sample_value(
            "aiagent_api_llm_calls_total",
            {"provider": "vertex_ai", "model": "claude-sonnet-4-20250514", "status": "success"},
        )
        assert val == 1.0

    def test_ut_hapi_435_011_accumulator_error_swallowed(self):
        """UT-HAPI-435-011: Accumulator error does not disrupt metrics
        recording or LLM call."""
        from src.metrics.instrumentation import HAMetrics
        from src.metrics.litellm_callback import KubernautLiteLLMCallback
        from src.metrics.token_accumulator import set_token_accumulator

        registry = CollectorRegistry()
        metrics = HAMetrics(registry=registry)
        cb = KubernautLiteLLMCallback(metrics)

        broken_acc = MagicMock()
        broken_acc.add.side_effect = RuntimeError("boom")
        set_token_accumulator(broken_acc)
        try:
            kwargs = {"model": "gpt-4"}
            response = _make_response(model="gpt-4", usage=_make_usage(10, 5))
            start = datetime(2026, 1, 1, tzinfo=timezone.utc)
            end = datetime(2026, 1, 1, 0, 0, 1, tzinfo=timezone.utc)

            cb.log_success_event(kwargs, response, start, end)

            val = registry.get_sample_value(
                "aiagent_api_llm_calls_total",
                {"provider": "openai", "model": "gpt-4", "status": "success"},
            )
            assert val == 1.0
        finally:
            set_token_accumulator(None)


# ========================================
# Phase G3: Multi-attempt Accumulation
# ========================================

class TestMultiAttemptAccumulation:
    """UT-HAPI-435-012: Per-call delta and session total across retry loop."""

    def test_ut_hapi_435_012_per_call_delta_across_retries(self):
        """UT-HAPI-435-012: Snapshot/delta approach produces correct per-call
        tokens_used while session accumulator tracks the full total."""
        from src.metrics.instrumentation import HAMetrics
        from src.metrics.litellm_callback import KubernautLiteLLMCallback
        from src.metrics.token_accumulator import (
            TokenAccumulator, set_token_accumulator, get_token_accumulator,
        )

        registry = CollectorRegistry()
        metrics = HAMetrics(registry=registry)
        cb = KubernautLiteLLMCallback(metrics)

        acc = TokenAccumulator()
        set_token_accumulator(acc)
        try:
            start = datetime(2026, 1, 1, tzinfo=timezone.utc)
            end = datetime(2026, 1, 1, 0, 0, 1, tzinfo=timezone.utc)

            per_call_deltas = []
            for prompt_tok, comp_tok in [(300, 150), (200, 80), (100, 70)]:
                snapshot = acc.total()
                cb.log_success_event(
                    {"model": "vertex_ai/claude-sonnet-4-20250514"},
                    _make_response(usage=_make_usage(prompt_tok, comp_tok)),
                    start, end,
                )
                delta = acc.total() - snapshot
                per_call_deltas.append(delta)

            assert per_call_deltas == [450, 280, 170]

            assert acc.prompt_tokens == 600
            assert acc.completion_tokens == 300
            assert acc.total() == 900
        finally:
            set_token_accumulator(None)
