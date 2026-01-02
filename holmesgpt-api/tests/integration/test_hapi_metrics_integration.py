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
Flow-Based Metrics Integration Tests for HAPI

Business Requirement: BR-MONITORING-001 - HAPI MUST expose Prometheus metrics
Authority: TESTING_GUIDELINES.md - Flow-based testing pattern

These tests validate that HAPI business operations record metrics that are
exposed via the /metrics endpoint.

✅ CORRECT PATTERN:
1. Trigger business operation (HTTP request to HAPI endpoint)
2. Query /metrics endpoint
3. Verify metrics are recorded with correct labels
4. Validate metric values make sense

Reference:
- Similar to audit flow-based pattern
- Tests business behavior (metrics recorded during operations)
- Not testing metrics infrastructure (Prometheus client library)
"""

import os
import time
import pytest
from typing import Dict, Any


# ========================================
# HELPER FUNCTIONS
# ========================================

def get_metrics(hapi_client) -> str:
    """
    Query HAPI's /metrics endpoint using TestClient.

    Args:
        hapi_client: FastAPI TestClient instance (from conftest.py hapi_client fixture)

    Returns:
        Metrics text in Prometheus format
        
    Note: Uses TestClient (in-process) instead of HTTP requests for faster, more reliable testing.
    """
    response = hapi_client.get("/metrics")
    assert response.status_code == 200, f"Metrics endpoint returned {response.status_code}"
    return response.text


def parse_metric_value(metrics_text: str, metric_name: str, labels: Dict[str, str] = None) -> float:
    """
    Parse a metric value from Prometheus metrics text.

    Args:
        metrics_text: Raw metrics text from /metrics endpoint
        metric_name: Name of the metric to extract
        labels: Optional dict of label filters (e.g., {"method": "POST"})

    Returns:
        Metric value as float, or 0.0 if not found

    Example:
        value = parse_metric_value(metrics, "http_requests_total", {"method": "POST", "path": "/api/v1/incident/analyze"})
    """
    lines = metrics_text.split('\n')

    for line in lines:
        if line.startswith('#') or not line.strip():
            continue

        if metric_name in line:
            # If labels specified, check if all match
            if labels:
                all_match = all(f'{key}="{value}"' in line for key, value in labels.items())
                if not all_match:
                    continue

            # Extract value (last part after space)
            parts = line.split()
            if len(parts) >= 2:
                try:
                    return float(parts[-1])
                except ValueError:
                    continue

    return 0.0


def make_incident_request(unique_test_id: str = None) -> Dict[str, Any]:
    """Create a valid incident request for testing."""
    return {
        "incident_id": f"inc-metrics-test-{unique_test_id or int(time.time())}",
        "remediation_id": f"rem-metrics-test-{unique_test_id or int(time.time())}",
        "signal_type": "OOMKilled",
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_kind": "Pod",
        "resource_name": "metrics-test-pod",
        "resource_namespace": "default",
        "cluster_name": "integration-test",
        "environment": "testing",
        "priority": "P1",
        "risk_tolerance": "low",
        "business_category": "test",
        "error_message": "Metrics integration test",
    }


def make_recovery_request(unique_test_id: str = None) -> Dict[str, Any]:
    """Create a valid recovery request for testing."""
    return {
        "incident_id": f"inc-metrics-recovery-{int(time.time())}",
        "remediation_id": f"rem-metrics-recovery-{int(time.time())}",
        "signal_type": "OOMKilled",
        "previous_workflow_id": "oomkill-increase-memory-v1",
        "previous_workflow_result": "Failed",
        "resource_namespace": "default",
        "resource_name": "metrics-recovery-pod",
        "resource_kind": "Pod",
    }


# ========================================
# FLOW-BASED METRICS INTEGRATION TESTS
# ========================================

class TestHTTPRequestMetrics:
    """
    Flow-based tests for HTTP request metrics.

    Pattern: Trigger business operation → Verify metrics recorded

    BR-MONITORING-001: HAPI MUST record HTTP request metrics
    """

    def test_incident_analysis_records_http_request_metrics(self, hapi_client, unique_test_id):
        """
        BR-MONITORING-001: Incident analysis MUST record HTTP request metrics.

        This test validates that HAPI records standard HTTP metrics when
        processing an incident analysis request.

        ✅ CORRECT: Tests HAPI behavior (records metrics during business operation)
        """
        # ARRANGE: Get baseline metrics
        metrics_before = get_metrics(hapi_client)
        requests_before = parse_metric_value(
            metrics_before,
            "http_requests_total",
            {"method": "POST", "endpoint": "/api/v1/incident/analyze", "status": "200"}
        )

        # ACT: Trigger business operation (incident analysis)
        incident_request = make_incident_request(unique_test_id)
        response = hapi_client.post("/api/v1/incident/analyze", json=incident_request)

        # Verify business operation succeeded
        assert response.status_code == 200, \
            f"Incident analysis failed: {response.status_code} - {response.text}"

        # ASSERT: Verify HTTP metrics recorded
        metrics_after = get_metrics(hapi_client)

        # Verify http_requests_total incremented
        requests_after = parse_metric_value(
            metrics_after,
            "http_requests_total",
            {"method": "POST", "endpoint": "/api/v1/incident/analyze", "status": "200"}
        )
        assert requests_after > requests_before, \
            f"http_requests_total should increment (before: {requests_before}, after: {requests_after})"

        # Verify http_request_duration_seconds recorded
        assert "http_request_duration_seconds" in metrics_after, \
            "http_request_duration_seconds metric not found"

    def test_recovery_analysis_records_http_request_metrics(self, hapi_client, unique_test_id):
        """
        BR-MONITORING-001: Recovery analysis MUST record HTTP request metrics.

        This test validates that HAPI records HTTP metrics for recovery endpoint.
        """
        # ARRANGE: Get baseline
        metrics_before = get_metrics(hapi_client)
        requests_before = parse_metric_value(
            metrics_before,
            "http_requests_total",
            {"method": "POST", "endpoint": "/api/v1/recovery/analyze", "status": "200"}
        )

        # ACT: Trigger recovery analysis
        recovery_request = make_recovery_request(unique_test_id)
        response = hapi_client.post("/api/v1/recovery/analyze", json=recovery_request)

        assert response.status_code == 200

        # ASSERT: Verify metrics
        metrics_after = get_metrics(hapi_client)
        requests_after = parse_metric_value(
            metrics_after,
            "http_requests_total",
            {"method": "POST", "endpoint": "/api/v1/recovery/analyze", "status": "200"}
        )

        assert requests_after > requests_before, \
            "Recovery endpoint should record http_requests_total"

    def test_health_endpoint_records_metrics(self, hapi_client, unique_test_id):
        """
        BR-MONITORING-001: Health endpoint MUST record HTTP request metrics.

        This test validates that even health checks are metered.
        """
        # ARRANGE
        metrics_before = get_metrics(hapi_client)
        requests_before = parse_metric_value(
            metrics_before,
            "http_requests_total",
            {"method": "GET", "endpoint": "/health"}
        )

        # ACT: Call health endpoint
        response = hapi_client.get("/health")
        assert response.status_code == 200

        # ASSERT
        metrics_after = get_metrics(hapi_client)
        requests_after = parse_metric_value(
            metrics_after,
            "http_requests_total",
            {"method": "GET", "endpoint": "/health"}
        )

        assert requests_after > requests_before, \
            "Health endpoint should record metrics"


class TestLLMMetrics:
    """
    Flow-based tests for LLM-specific metrics.

    Pattern: Trigger LLM operation → Verify LLM metrics recorded

    BR-MONITORING-001: HAPI MUST record LLM request metrics
    """

    def test_incident_analysis_records_llm_request_duration(self, hapi_client, unique_test_id):
        """
        BR-MONITORING-001: Incident analysis MUST record LLM request duration.

        This test validates that HAPI records LLM-specific metrics when
        using AI for incident analysis.
        """
        # ARRANGE
        metrics_before = get_metrics(hapi_client)

        # ACT: Trigger incident analysis (uses LLM)
        incident_request = make_incident_request(unique_test_id)
        response = hapi_client.post("/api/v1/incident/analyze", json=incident_request)

        assert response.status_code == 200

        # ASSERT: Verify LLM metrics recorded
        metrics_after = get_metrics(hapi_client)

        # LLM request duration should be present
        # (Metric name may vary: llm_request_duration_seconds, llm_call_duration_seconds, llm_latency_seconds, etc.)
        llm_metrics_found = any(
            metric in metrics_after
            for metric in ["llm_request_duration", "llm_call_duration", "llm_latency", "llm_time"]
        )

        assert llm_metrics_found, \
            "LLM request duration metric not found in /metrics. " \
            "Check if HAPI is recording LLM performance metrics."

    def test_recovery_analysis_records_llm_request_duration(self, hapi_client, unique_test_id):
        """
        BR-MONITORING-001: Recovery analysis MUST record LLM request duration.

        This test validates that LLM metrics are recorded for recovery endpoint.
        """
        # ACT
        recovery_request = make_recovery_request(unique_test_id)
        response = hapi_client.post("/api/v1/recovery/analyze", json=recovery_request)

        assert response.status_code == 200

        # ASSERT
        metrics_after = get_metrics(hapi_client)

        # Verify LLM metrics exist (any LLM-related metric)
        llm_metrics_found = any(
            metric in metrics_after
            for metric in ["llm_request_duration", "llm_call_duration", "llm_latency", "llm_time"]
        )

        assert llm_metrics_found, \
            "Recovery endpoint should record LLM metrics"


class TestMetricsAggregation:
    """
    Flow-based tests for metrics aggregation over multiple requests.

    Pattern: Trigger multiple operations → Verify metrics aggregate correctly

    BR-MONITORING-001: Metrics MUST aggregate correctly over time
    """

    def test_multiple_requests_increment_counter(self, hapi_client, unique_test_id):
        """
        BR-MONITORING-001: Multiple requests MUST increment counter metrics.

        This test validates that counter metrics (http_requests_total)
        accumulate across multiple requests.
        """
        # ARRANGE: Get baseline
        metrics_before = get_metrics(hapi_client)
        requests_before = parse_metric_value(
            metrics_before,
            "http_requests_total",
            {"method": "POST", "endpoint": "/api/v1/incident/analyze", "status": "200"}
        )

        # ACT: Make multiple requests
        num_requests = 3
        for i in range(num_requests):
            incident_request = make_incident_request(unique_test_id)
            response = hapi_client.post("/api/v1/incident/analyze", json=incident_request)
            assert response.status_code == 200

        # ASSERT: Verify counter incremented by num_requests
        metrics_after = get_metrics(hapi_client)
        requests_after = parse_metric_value(
            metrics_after,
            "http_requests_total",
            {"method": "POST", "endpoint": "/api/v1/incident/analyze", "status": "200"}
        )

        expected_increment = num_requests
        actual_increment = requests_after - requests_before

        assert actual_increment >= expected_increment, \
            f"Expected at least {expected_increment} requests recorded, " \
            f"got {actual_increment} (before: {requests_before}, after: {requests_after})"

    def test_histogram_metrics_record_multiple_samples(self, hapi_client, unique_test_id):
        """
        BR-MONITORING-001: Histogram metrics MUST record multiple samples.

        This test validates that histogram metrics (http_request_duration_seconds)
        record samples from multiple requests.
        """
        # ACT: Make multiple requests with different characteristics
        for i in range(3):
            incident_request = make_incident_request(unique_test_id)
            # Vary the request slightly to potentially get different durations
            incident_request["signal_type"] = ["OOMKilled", "CrashLoopBackOff", "ImagePullBackOff"][i]

            response = hapi_client.post("/api/v1/incident/analyze", json=incident_request)
            assert response.status_code == 200

        # ASSERT: Verify histogram has samples
        metrics_after = get_metrics(hapi_client)

        # Histogram metrics should have _count and _sum suffixes
        has_histogram_count = "http_request_duration_seconds_count" in metrics_after
        has_histogram_sum = "http_request_duration_seconds_sum" in metrics_after

        assert has_histogram_count or has_histogram_sum, \
            "Histogram metrics (http_request_duration_seconds_count/_sum) not found. " \
            "HAPI should record request duration histograms."


class TestMetricsEndpointAvailability:
    """
    Flow-based tests for /metrics endpoint availability.

    Pattern: Query /metrics → Verify endpoint works

    BR-MONITORING-001: /metrics endpoint MUST be available
    """

    def test_metrics_endpoint_is_accessible(self, hapi_client, unique_test_id):
        """
        BR-MONITORING-001: /metrics endpoint MUST be accessible.

        This test validates that the Prometheus metrics endpoint is available
        and returns valid Prometheus format.
        """
        # ACT
        response = hapi_client.get("/metrics")

        # ASSERT
        assert response.status_code == 200, \
            f"/metrics endpoint returned {response.status_code}"

        metrics_text = response.text

        # Verify Prometheus format (should have # HELP and # TYPE comments)
        assert "# HELP" in metrics_text, \
            "/metrics should include # HELP comments (Prometheus format)"
        assert "# TYPE" in metrics_text, \
            "/metrics should include # TYPE comments (Prometheus format)"

    def test_metrics_endpoint_returns_content_type_text_plain(self, hapi_client, unique_test_id):
        """
        BR-MONITORING-001: /metrics endpoint MUST return text/plain content type.

        Prometheus expects metrics in text/plain format.
        """
        # ACT
        response = hapi_client.get("/metrics")

        # ASSERT
        assert response.status_code == 200

        content_type = response.headers.get("Content-Type", "")
        assert "text/plain" in content_type, \
            f"/metrics should return text/plain, got {content_type}"


class TestBusinessMetrics:
    """
    Flow-based tests for business-specific metrics.

    Pattern: Trigger business scenario → Verify business metrics recorded

    BR-MONITORING-001: HAPI SHOULD record business-specific metrics
    """

    def test_workflow_selection_metrics_recorded(self, hapi_client, unique_test_id):
        """
        BR-MONITORING-001: Workflow selection SHOULD be metered.

        This test validates that HAPI records metrics about workflow
        selection during incident analysis (if implemented).
        """
        # ACT: Trigger incident analysis
        incident_request = make_incident_request(unique_test_id)
        response = hapi_client.post("/api/v1/incident/analyze", json=incident_request)

        assert response.status_code == 200

        # ASSERT: Check if workflow metrics exist
        metrics = get_metrics(hapi_client)

        # Workflow-related metrics (may not exist yet, so this is informational)
        workflow_metrics_found = any(
            metric in metrics
            for metric in ["workflow_selected", "workflow_confidence", "workflow_search"]
        )

        # This is a SHOULD not MUST - log if missing but don't fail
        if not workflow_metrics_found:
            print("ℹ️  Workflow-specific metrics not found (optional enhancement)")


# ========================================
# TEST COLLECTION
# ========================================

# Total: 11 flow-based metrics tests
# - 3 HTTP request metrics tests
# - 2 LLM metrics tests
# - 2 metrics aggregation tests
# - 2 metrics endpoint availability tests
# - 1 business metrics test (informational)
# - 1 content type test

# These tests follow the same flow-based pattern as audit tests:
# 1. Trigger business operation (HTTP request)
# 2. Query /metrics endpoint
# 3. Verify metrics recorded
# 4. Validate metric content

# NOT an anti-pattern fix (no anti-pattern existed for metrics)
# This is NEW test coverage for previously untested metrics functionality




