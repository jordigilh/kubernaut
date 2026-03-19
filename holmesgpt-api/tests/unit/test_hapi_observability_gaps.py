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
Unit Tests for HAPI Observability Gaps (#442)

Gap 2: Dead code removal (PrometheusMetricsMiddleware, record_investigation_start)
Gap 4: /config endpoint wiring
"""

import pytest


class TestGap2DeadCodeRemoval:
    """#442 Gap 2: Verify dead code is removed."""

    def test_no_prometheus_metrics_middleware_class(self):
        """PrometheusMetricsMiddleware should not exist in metrics module (#294)."""
        import src.middleware.metrics as metrics_mod
        assert not hasattr(metrics_mod, "PrometheusMetricsMiddleware"), \
            "PrometheusMetricsMiddleware should be removed (no-op, #294)"

    def test_metrics_endpoint_still_exists(self):
        """metrics_endpoint() must survive the cleanup -- it serves /metrics."""
        from src.middleware.metrics import metrics_endpoint
        assert callable(metrics_endpoint)

    def test_no_record_investigation_start(self):
        """record_investigation_start() is dead code -- never called."""
        from src.metrics.instrumentation import HAMetrics
        from prometheus_client import CollectorRegistry
        m = HAMetrics(registry=CollectorRegistry())
        assert not hasattr(m, "record_investigation_start"), \
            "record_investigation_start should be removed (dead code)"

    def test_record_investigation_complete_still_exists(self):
        """record_investigation_complete() is used in production."""
        from src.metrics.instrumentation import HAMetrics
        from prometheus_client import CollectorRegistry
        m = HAMetrics(registry=CollectorRegistry())
        assert hasattr(m, "record_investigation_complete")


class TestGap4ConfigEndpoint:
    """#442 Gap 4: /config endpoint wiring."""

    @pytest.mark.asyncio
    async def test_config_endpoint_returns_sanitized_data(self):
        """When router.config is set, /config returns sanitized non-empty values."""
        from src.extensions.health import router, get_config

        router.config = {
            "llm": {"provider": "vertex_ai", "model": "claude-sonnet-4", "endpoint": "https://llm.example.com", "api_key": "sk-secret-123"},
            "environment": "production",
            "dev_mode": False,
            "api_port": 8080,
            "audit": {"buffer_size": 1000},
        }

        result = await get_config()

        assert result["llm"]["provider"] == "vertex_ai"
        assert result["llm"]["model"] == "claude-sonnet-4"
        assert result["llm"]["endpoint"] == "https://llm.example.com"
        assert result["dev_mode"] is False
        # Sensitive fields must NOT leak
        assert "api_key" not in result.get("llm", {})
        assert "audit" not in result
        assert "api_port" not in result

        # Cleanup: remove config to not pollute other tests
        if hasattr(router, "config"):
            delattr(router, "config")
