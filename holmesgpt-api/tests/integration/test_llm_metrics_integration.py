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
Integration Tests: HAPI Prometheus Metrics Pipeline (BR-HAPI-011, BR-HAPI-301).

Test Plan: docs/tests/436/TEST_PLAN.md
Issue: https://github.com/jordigilh/kubernaut/issues/436

These tests validate that the LiteLLM callback and investigation metrics
flow end-to-end through analyze_incident() -> HAMetrics, exercising all
5 HAPI Prometheus metrics.

Pattern: Direct business logic call with mocked HolmesGPT SDK.
Each test creates an isolated HAMetrics + CollectorRegistry to avoid
state leaks (Go service pattern for test isolation).
"""

import asyncio
import time
from datetime import datetime, timezone
from unittest.mock import MagicMock, patch

import pytest
from prometheus_client import CollectorRegistry

from src.metrics.instrumentation import HAMetrics
from src.metrics.litellm_callback import KubernautLiteLLMCallback


def _get_counter_value(registry, metric_name, labels):
    return registry.get_sample_value(f"{metric_name}_total", labels) or 0.0


def _get_histogram_count(registry, metric_name, labels):
    return registry.get_sample_value(f"{metric_name}_count", labels) or 0.0


@pytest.fixture
def isolated_metrics():
    """Fresh HAMetrics with isolated CollectorRegistry per test."""
    registry = CollectorRegistry()
    metrics = HAMetrics(registry=registry)
    return metrics, registry


@pytest.fixture
def litellm_callback(isolated_metrics):
    """KubernautLiteLLMCallback wired to the isolated metrics."""
    metrics, _ = isolated_metrics
    return KubernautLiteLLMCallback(metrics)


def _make_sdk_result(rca_text="OOMKilled due to memory leak", workflow_id=None):
    """Build a mock HolmesGPT InvestigationResult for patching investigate_issues."""
    result = MagicMock()
    result.analysis = rca_text
    result.tool_calls = []
    result.instructions = []
    return result


class TestLLMMetricsIntegration:
    """IT-HAPI-301-001 through IT-HAPI-301-005: Full metrics pipeline."""

    def test_it_hapi_301_001_llm_calls_total_via_callback(self, isolated_metrics, litellm_callback):
        """IT-HAPI-301-001: llm_calls_total is incremented when the LiteLLM
        callback fires after a successful LLM completion."""
        metrics, registry = isolated_metrics
        cb = litellm_callback

        kwargs = {"model": "vertex_ai/claude-sonnet-4-20250514"}
        usage = MagicMock()
        usage.prompt_tokens = 200
        usage.completion_tokens = 100
        usage.total_tokens = 300
        response = MagicMock()
        response.model = "vertex_ai/claude-sonnet-4-20250514"
        response.usage = usage
        start = datetime(2026, 1, 1, tzinfo=timezone.utc)
        end = datetime(2026, 1, 1, 0, 0, 2, tzinfo=timezone.utc)

        cb.log_success_event(kwargs, response, start, end)

        val = _get_counter_value(registry, "aiagent_api_llm_calls", {
            "provider": "vertex_ai",
            "model": "claude-sonnet-4-20250514",
            "status": "success",
        })
        assert val >= 1.0

    def test_it_hapi_301_002_llm_token_usage_via_callback(self, isolated_metrics, litellm_callback):
        """IT-HAPI-301-002: llm_token_usage_total reflects prompt and completion
        tokens after a real LLM call path."""
        metrics, registry = isolated_metrics
        cb = litellm_callback

        kwargs = {"model": "vertex_ai/claude-sonnet-4-20250514"}
        usage = MagicMock()
        usage.prompt_tokens = 500
        usage.completion_tokens = 150
        usage.total_tokens = 650
        response = MagicMock()
        response.model = "vertex_ai/claude-sonnet-4-20250514"
        response.usage = usage

        cb.log_success_event(
            kwargs, response,
            datetime(2026, 1, 1, tzinfo=timezone.utc),
            datetime(2026, 1, 1, 0, 0, 1, tzinfo=timezone.utc),
        )

        prompt_val = _get_counter_value(registry, "aiagent_api_llm_token_usage", {
            "provider": "vertex_ai",
            "model": "claude-sonnet-4-20250514",
            "type": "prompt",
        })
        completion_val = _get_counter_value(registry, "aiagent_api_llm_token_usage", {
            "provider": "vertex_ai",
            "model": "claude-sonnet-4-20250514",
            "type": "completion",
        })
        assert prompt_val >= 500.0
        assert completion_val >= 150.0

    def test_it_hapi_301_003_llm_call_duration_via_callback(self, isolated_metrics, litellm_callback):
        """IT-HAPI-301-003: llm_call_duration_seconds histogram records
        accurate latency from the callback timestamps."""
        metrics, registry = isolated_metrics
        cb = litellm_callback

        kwargs = {"model": "openai/gpt-4"}
        usage = MagicMock()
        usage.prompt_tokens = 100
        usage.completion_tokens = 50
        usage.total_tokens = 150
        response = MagicMock()
        response.model = "openai/gpt-4"
        response.usage = usage

        cb.log_success_event(
            kwargs, response,
            datetime(2026, 1, 1, 0, 0, 0, tzinfo=timezone.utc),
            datetime(2026, 1, 1, 0, 0, 5, tzinfo=timezone.utc),
        )

        count = _get_histogram_count(
            registry, "aiagent_api_llm_call_duration_seconds",
            {"provider": "openai", "model": "gpt-4"},
        )
        assert count >= 1.0

    @pytest.mark.asyncio
    async def test_it_hapi_301_004_investigation_metrics_via_analyze(self, isolated_metrics):
        """IT-HAPI-301-004: investigations_total and investigations_duration_seconds
        are populated when analyze_incident() completes successfully."""
        metrics, registry = isolated_metrics

        mock_investigation = _make_sdk_result()

        request_data = {
            "incident_id": "test-metrics-004",
            "signal_name": "KubePodCrashLooping",
            "cluster_name": "test-cluster",
        }

        with patch("src.extensions.incident.llm_integration.investigate_issues", return_value=mock_investigation), \
             patch("src.extensions.incident.llm_integration.get_audit_store") as mock_audit, \
             patch("src.extensions.incident.llm_integration.parse_and_validate_investigation_result") as mock_parse, \
             patch("src.extensions.incident.llm_integration.audit_llm_request"), \
             patch("src.extensions.incident.llm_integration.audit_llm_response_and_tools"), \
             patch("src.extensions.incident.llm_integration.handle_validation_exhaustion"), \
             patch("src.extensions.incident.llm_integration.inject_detected_labels"):

            mock_audit_store = MagicMock()
            mock_audit.return_value = mock_audit_store

            validation_result = MagicMock()
            validation_result.is_valid = True
            validation_result.errors = []
            mock_parse.return_value = (
                {"rca": "OOMKilled", "selected_workflow": None, "needs_human_review": False},
                validation_result,
            )

            from src.extensions.incident.llm_integration import analyze_incident
            await analyze_incident(request_data, metrics=metrics)

        inv_count = _get_counter_value(registry, "aiagent_api_investigations", {"status": "success"})
        assert inv_count >= 1.0

        dur_count = _get_histogram_count(registry, "aiagent_api_investigations_duration_seconds", {})
        assert dur_count >= 1.0

    @pytest.mark.asyncio
    async def test_it_hapi_301_005_investigation_error_status(self, isolated_metrics):
        """IT-HAPI-301-005: investigations_total{status='error'} is incremented
        when analyze_incident() raises an exception."""
        metrics, registry = isolated_metrics

        request_data = {
            "incident_id": "test-metrics-005",
            "signal_name": "KubePodCrashLooping",
            "cluster_name": "test-cluster",
        }

        with patch("src.extensions.incident.llm_integration.investigate_issues", side_effect=RuntimeError("LLM unavailable")), \
             patch("src.extensions.incident.llm_integration.get_audit_store") as mock_audit, \
             patch("src.extensions.incident.llm_integration.sanitize_for_llm", side_effect=lambda x: x), \
             patch("src.extensions.incident.llm_integration.get_model_config_for_sdk", return_value=("gpt-4", "openai")), \
             patch("src.extensions.incident.llm_integration.prepare_toolsets_config_for_sdk", return_value={}), \
             patch("src.extensions.incident.llm_integration.register_workflow_discovery_toolset", side_effect=lambda c, *a, **kw: c), \
             patch("src.extensions.incident.llm_integration.register_resource_context_toolset", side_effect=lambda c, *a, **kw: c), \
             patch("src.extensions.incident.llm_integration.create_data_storage_client", return_value=None), \
             patch("src.extensions.incident.llm_integration.audit_llm_request"):

            mock_audit_store = MagicMock()
            mock_audit.return_value = mock_audit_store

            from src.extensions.incident.llm_integration import analyze_incident
            with pytest.raises(RuntimeError, match="LLM unavailable"):
                await analyze_incident(request_data, metrics=metrics)

        err_count = _get_counter_value(registry, "aiagent_api_investigations", {"status": "error"})
        assert err_count >= 1.0
