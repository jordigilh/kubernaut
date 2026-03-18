#
# Copyright 2025 Jordi Gil.
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
Unit tests for LiteLLM metrics callback (BR-HAPI-301).

Test Plan: docs/tests/436/TEST_PLAN.md
Issue: https://github.com/jordigilh/kubernaut/issues/436

Tests validate that the KubernautLiteLLMCallback correctly extracts
provider, model, status, duration, and token usage from LiteLLM
responses and records them via HAMetrics.record_llm_call().
"""

from datetime import datetime, timezone
from unittest.mock import MagicMock, patch

import pytest
from prometheus_client import CollectorRegistry

from src.metrics.instrumentation import HAMetrics
from src.metrics.litellm_callback import KubernautLiteLLMCallback


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


def _get_counter_value(registry, metric_name, labels):
    """Read a counter value from a CollectorRegistry."""
    return registry.get_sample_value(f"{metric_name}_total", labels) or 0.0


def _get_histogram_count(registry, metric_name, labels):
    """Read histogram _count from a CollectorRegistry."""
    return registry.get_sample_value(f"{metric_name}_count", labels) or 0.0


def _get_histogram_sum(registry, metric_name, labels):
    """Read histogram _sum from a CollectorRegistry."""
    return registry.get_sample_value(f"{metric_name}_sum", labels) or 0.0


@pytest.fixture
def isolated_metrics():
    """Fresh HAMetrics with isolated CollectorRegistry per test."""
    registry = CollectorRegistry()
    return HAMetrics(registry=registry), registry


@pytest.fixture
def callback(isolated_metrics):
    metrics, _ = isolated_metrics
    return KubernautLiteLLMCallback(metrics)


class TestLiteLLMMetricsCallback:
    """UT-HAPI-301-001 through UT-HAPI-301-007: LiteLLM callback metrics."""

    def test_ut_hapi_301_001_success_increments_call_counter(self, isolated_metrics):
        """UT-HAPI-301-001: Operator sees llm_calls_total incremented with
        correct provider/model/status labels after each LLM call."""
        metrics, registry = isolated_metrics
        cb = KubernautLiteLLMCallback(metrics)

        kwargs = {"model": "vertex_ai/claude-sonnet-4-20250514"}
        response = _make_response(
            model="vertex_ai/claude-sonnet-4-20250514",
            usage=_make_usage(100, 50),
        )
        start = datetime(2026, 1, 1, 0, 0, 0, tzinfo=timezone.utc)
        end = datetime(2026, 1, 1, 0, 0, 2, tzinfo=timezone.utc)

        cb.log_success_event(kwargs, response, start, end)

        val = _get_counter_value(registry, "holmesgpt_api_llm_calls", {
            "provider": "vertex_ai",
            "model": "claude-sonnet-4-20250514",
            "status": "success",
        })
        assert val == 1.0

    def test_ut_hapi_301_002_duration_recorded_accurately(self, isolated_metrics):
        """UT-HAPI-301-002: Operator sees llm_call_duration_seconds histogram
        populated with accurate call latency."""
        metrics, registry = isolated_metrics
        cb = KubernautLiteLLMCallback(metrics)

        kwargs = {"model": "vertex_ai/claude-sonnet-4-20250514"}
        response = _make_response(usage=_make_usage(100, 50))
        start = datetime(2026, 1, 1, 0, 0, 0, tzinfo=timezone.utc)
        end = datetime(2026, 1, 1, 0, 0, 3, 500000, tzinfo=timezone.utc)

        cb.log_success_event(kwargs, response, start, end)

        labels = {"provider": "vertex_ai", "model": "claude-sonnet-4-20250514"}
        count = _get_histogram_count(registry, "holmesgpt_api_llm_call_duration_seconds", labels)
        total = _get_histogram_sum(registry, "holmesgpt_api_llm_call_duration_seconds", labels)
        assert count == 1.0
        assert abs(total - 3.5) < 0.01

    def test_ut_hapi_301_003_token_usage_extracted(self, isolated_metrics):
        """UT-HAPI-301-003: Operator sees llm_token_usage_total with accurate
        prompt and completion token counts for cost tracking."""
        metrics, registry = isolated_metrics
        cb = KubernautLiteLLMCallback(metrics)

        kwargs = {"model": "vertex_ai/claude-sonnet-4-20250514"}
        response = _make_response(usage=_make_usage(1500, 350))
        start = datetime(2026, 1, 1, tzinfo=timezone.utc)
        end = datetime(2026, 1, 1, 0, 0, 1, tzinfo=timezone.utc)

        cb.log_success_event(kwargs, response, start, end)

        prompt_val = _get_counter_value(registry, "holmesgpt_api_llm_token_usage", {
            "provider": "vertex_ai",
            "model": "claude-sonnet-4-20250514",
            "type": "prompt",
        })
        completion_val = _get_counter_value(registry, "holmesgpt_api_llm_token_usage", {
            "provider": "vertex_ai",
            "model": "claude-sonnet-4-20250514",
            "type": "completion",
        })
        assert prompt_val == 1500.0
        assert completion_val == 350.0

    @pytest.mark.parametrize("model_string,expected_provider,expected_model", [
        ("vertex_ai/claude-sonnet-4-20250514", "vertex_ai", "claude-sonnet-4-20250514"),
        ("gpt-4", "openai", "gpt-4"),
        ("claude-sonnet-4-20250514", "anthropic", "claude-sonnet-4-20250514"),
        ("openai/llama2", "openai", "llama2"),
    ])
    def test_ut_hapi_301_004_provider_model_extraction(
        self, isolated_metrics, model_string, expected_provider, expected_model
    ):
        """UT-HAPI-301-004: Operator sees correct provider and model labels
        regardless of LiteLLM's internal model name formatting."""
        metrics, registry = isolated_metrics
        cb = KubernautLiteLLMCallback(metrics)

        kwargs = {"model": model_string}
        response = _make_response(model=model_string, usage=_make_usage(10, 5))
        start = datetime(2026, 1, 1, tzinfo=timezone.utc)
        end = datetime(2026, 1, 1, 0, 0, 1, tzinfo=timezone.utc)

        cb.log_success_event(kwargs, response, start, end)

        val = _get_counter_value(registry, "holmesgpt_api_llm_calls", {
            "provider": expected_provider,
            "model": expected_model,
            "status": "success",
        })
        assert val == 1.0, f"Expected provider={expected_provider}, model={expected_model} for '{model_string}'"

    def test_ut_hapi_301_005_missing_usage_handled(self, isolated_metrics):
        """UT-HAPI-301-005: Call counted and duration recorded even when
        response has no token usage data."""
        metrics, registry = isolated_metrics
        cb = KubernautLiteLLMCallback(metrics)

        kwargs = {"model": "gpt-4"}
        response = _make_response(model="gpt-4", usage=None)
        start = datetime(2026, 1, 1, tzinfo=timezone.utc)
        end = datetime(2026, 1, 1, 0, 0, 2, tzinfo=timezone.utc)

        cb.log_success_event(kwargs, response, start, end)

        call_val = _get_counter_value(registry, "holmesgpt_api_llm_calls", {
            "provider": "openai",
            "model": "gpt-4",
            "status": "success",
        })
        assert call_val == 1.0

        duration_count = _get_histogram_count(
            registry, "holmesgpt_api_llm_call_duration_seconds",
            {"provider": "openai", "model": "gpt-4"},
        )
        assert duration_count == 1.0

        prompt_val = registry.get_sample_value(
            "holmesgpt_api_llm_token_usage_total",
            {"provider": "openai", "model": "gpt-4", "type": "prompt"},
        )
        assert prompt_val is None

    def test_ut_hapi_301_006_callback_error_swallowed(self, isolated_metrics):
        """UT-HAPI-301-006: LLM investigation is not disrupted if the
        metrics callback raises an unexpected error."""
        metrics, _ = isolated_metrics
        cb = KubernautLiteLLMCallback(metrics)

        with patch.object(metrics, "record_llm_call", side_effect=RuntimeError("boom")):
            kwargs = {"model": "gpt-4"}
            response = _make_response(model="gpt-4", usage=_make_usage(10, 5))
            start = datetime(2026, 1, 1, tzinfo=timezone.utc)
            end = datetime(2026, 1, 1, 0, 0, 1, tzinfo=timezone.utc)

            cb.log_success_event(kwargs, response, start, end)

    def test_ut_hapi_301_007_failure_records_error_status(self, isolated_metrics):
        """UT-HAPI-301-007: Operator sees llm_calls_total{status='error'}
        incremented when an LLM call fails."""
        metrics, registry = isolated_metrics
        cb = KubernautLiteLLMCallback(metrics)

        kwargs = {"model": "vertex_ai/claude-sonnet-4-20250514"}
        start = datetime(2026, 1, 1, tzinfo=timezone.utc)
        end = datetime(2026, 1, 1, 0, 0, 1, tzinfo=timezone.utc)
        exception = Exception("AuthenticationError: ANTHROPIC_API_KEY missing")

        cb.log_failure_event(kwargs, exception, start, end)

        val = _get_counter_value(registry, "holmesgpt_api_llm_calls", {
            "provider": "vertex_ai",
            "model": "claude-sonnet-4-20250514",
            "status": "error",
        })
        assert val == 1.0

        duration_count = _get_histogram_count(
            registry, "holmesgpt_api_llm_call_duration_seconds",
            {"provider": "vertex_ai", "model": "claude-sonnet-4-20250514"},
        )
        assert duration_count == 1.0
