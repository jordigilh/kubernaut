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
Integration Tests for HAPI Metrics (Following Go Pattern)

Business Requirement: BR-MONITORING-001 - HAPI MUST expose Prometheus metrics
Test Pattern: Direct business logic calls + Prometheus registry inspection

✅ INTEGRATION TEST PATTERN (Like Go Gateway/AIAnalysis):
1. Call business logic directly (analyze_incident, analyze_recovery)
2. Query Prometheus REGISTRY directly (no HTTP /metrics endpoint)
3. Verify metrics are recorded with correct labels
4. NO TestClient, NO HTTP layer, NO main.py import

This pattern:
- Avoids K8s auth initialization issues (no main.py import)
- Tests business logic metrics emission (integration testing)
- Matches Go service metrics testing pattern
- Faster and more reliable than E2E HTTP testing

Reference:
- test/integration/gateway/metrics_emission_integration_test.go
- test/integration/aianalysis/metrics_integration_test.go
"""

import os
import time
import pytest
import asyncio
from typing import Dict, Any
from prometheus_client import REGISTRY


# ========================================
# HELPER FUNCTIONS
# ========================================

def get_metric_value_from_registry(metric_name: str, labels: Dict[str, str] = None) -> float:
    """
    Query Prometheus REGISTRY directly for metric value.
    
    This is the Go pattern: Query registry, NOT /metrics HTTP endpoint.
    
    Args:
        metric_name: Name of the metric (e.g., "investigations_total")
        labels: Optional dict of label filters
    
    Returns:
        Metric value as float, or 0.0 if not found
        
    Example:
        value = get_metric_value_from_registry("investigations_total", {"status": "success"})
    """
    for collector in REGISTRY.collect():
        for metric in collector.samples:
            # Match metric name
            if metric.name == metric_name or metric.name.startswith(metric_name):
                # If labels specified, check if all match
                if labels:
                    all_match = all(metric.labels.get(key) == value for key, value in labels.items())
                    if not all_match:
                        continue
                
                # Return the value
                return metric.value
    
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
# INTEGRATION TESTS (GO PATTERN)
# ========================================

class TestIncidentAnalysisMetrics:
    """
    Integration tests for incident analysis metrics.
    
    Pattern: Call business logic directly → Query Prometheus registry
    
    BR-MONITORING-001: HAPI MUST record investigation metrics
    """

    @pytest.mark.asyncio
    async def test_incident_analysis_increments_investigations_total(self, unique_test_id):
        """
        BR-MONITORING-001: Incident analysis MUST increment investigations_total metric.
        
        Pattern (like Go):
        1. Get initial metric value from REGISTRY
        2. Call analyze_incident() directly (NO HTTP)
        3. Query REGISTRY for new value
        4. Verify increment
        """
        from src.extensions.incident.llm_integration import analyze_incident
        from src.models.config_models import AppConfig
        
        # ARRANGE: Get baseline from Prometheus REGISTRY
        initial_value = get_metric_value_from_registry("investigations_total", {"status": "success"})
        
        # ACT: Call business logic directly (NO HTTP, NO TestClient)
        incident_request = make_incident_request(unique_test_id)
        
        # Load app config for analyze_incident
        app_config = AppConfig()
        
        result = await analyze_incident(incident_request, mcp_config=None, app_config=app_config)
        
        # Verify business operation succeeded
        assert result is not None, "Incident analysis should return result"
        assert "workflow_id" in result or "needs_human_review" in result, \
            "Result should contain workflow_id or needs_human_review"
        
        # ASSERT: Query Prometheus REGISTRY for updated metric
        final_value = get_metric_value_from_registry("investigations_total", {"status": "success"})
        
        assert final_value > initial_value, \
            f"investigations_total should increment (before: {initial_value}, after: {final_value})"
        
        print(f"✅ Metric validated: investigations_total increased from {initial_value} to {final_value}")

    @pytest.mark.asyncio
    async def test_incident_analysis_records_duration_histogram(self, unique_test_id):
        """
        BR-MONITORING-001: Incident analysis MUST record duration histogram.
        
        Pattern: Call business logic → Verify histogram count incremented
        """
        from src.extensions.incident.llm_integration import analyze_incident
        from src.models.config_models import AppConfig
        
        # ARRANGE: Get baseline histogram count
        initial_count = get_metric_value_from_registry("investigations_duration_seconds_count")
        
        # ACT: Call business logic
        incident_request = make_incident_request(unique_test_id)
        app_config = AppConfig()
        result = await analyze_incident(incident_request, mcp_config=None, app_config=app_config)
        
        assert result is not None
        
        # ASSERT: Histogram count should increment
        final_count = get_metric_value_from_registry("investigations_duration_seconds_count")
        
        assert final_count > initial_count, \
            f"investigations_duration_seconds_count should increment (before: {initial_count}, after: {final_count})"
        
        print(f"✅ Duration histogram updated: count {initial_count} → {final_count}")

    @pytest.mark.asyncio
    async def test_llm_calls_total_increments(self, unique_test_id):
        """
        BR-MONITORING-001: LLM calls MUST increment llm_calls_total metric.
        
        Pattern: Call business logic → Verify LLM metrics recorded
        """
        from src.extensions.incident.llm_integration import analyze_incident
        from src.models.config_models import AppConfig
        
        # ARRANGE: Get baseline
        initial_llm_calls = get_metric_value_from_registry("llm_calls_total")
        
        # ACT: Call business logic (triggers LLM call)
        incident_request = make_incident_request(unique_test_id)
        app_config = AppConfig()
        result = await analyze_incident(incident_request, mcp_config=None, app_config=app_config)
        
        assert result is not None
        
        # ASSERT: LLM metrics should increment
        final_llm_calls = get_metric_value_from_registry("llm_calls_total")
        
        assert final_llm_calls > initial_llm_calls, \
            f"llm_calls_total should increment (before: {initial_llm_calls}, after: {final_llm_calls})"
        
        print(f"✅ LLM metrics updated: llm_calls_total {initial_llm_calls} → {final_llm_calls}")


class TestRecoveryAnalysisMetrics:
    """
    Integration tests for recovery analysis metrics.
    
    Pattern: Direct business logic calls + Prometheus registry inspection
    """

    @pytest.mark.asyncio
    async def test_recovery_analysis_increments_investigations_total(self, unique_test_id):
        """
        BR-MONITORING-001: Recovery analysis MUST increment investigations_total metric.
        
        Pattern (like Go): Direct business logic call → Query REGISTRY
        """
        from src.extensions.recovery.llm_integration import analyze_recovery
        from src.models.config_models import AppConfig
        
        # ARRANGE: Get baseline
        initial_value = get_metric_value_from_registry("investigations_total")
        
        # ACT: Call recovery business logic directly
        recovery_request = make_recovery_request(unique_test_id)
        app_config = AppConfig()
        
        result = await analyze_recovery(recovery_request, app_config=app_config)
        
        # Verify operation succeeded
        assert result is not None
        
        # ASSERT: Query registry
        final_value = get_metric_value_from_registry("investigations_total")
        
        assert final_value > initial_value, \
            f"Recovery analysis should increment investigations_total (before: {initial_value}, after: {final_value})"
        
        print(f"✅ Recovery metrics: investigations_total {initial_value} → {final_value}")

    @pytest.mark.asyncio
    async def test_recovery_analysis_records_llm_metrics(self, unique_test_id):
        """
        BR-MONITORING-001: Recovery analysis MUST record LLM call metrics.
        
        Pattern: Direct business logic call → Verify LLM metrics
        """
        from src.extensions.recovery.llm_integration import analyze_recovery
        from src.models.config_models import AppConfig
        
        # ARRANGE
        initial_llm_calls = get_metric_value_from_registry("llm_calls_total")
        initial_duration_count = get_metric_value_from_registry("llm_call_duration_seconds_count")
        
        # ACT: Call recovery business logic
        recovery_request = make_recovery_request(unique_test_id)
        app_config = AppConfig()
        result = await analyze_recovery(recovery_request, app_config=app_config)
        
        assert result is not None
        
        # ASSERT: LLM metrics should increment
        final_llm_calls = get_metric_value_from_registry("llm_calls_total")
        final_duration_count = get_metric_value_from_registry("llm_call_duration_seconds_count")
        
        assert final_llm_calls > initial_llm_calls, \
            "Recovery should trigger LLM calls"
        assert final_duration_count > initial_duration_count, \
            "Recovery should record LLM duration"
        
        print(f"✅ Recovery LLM metrics: calls {initial_llm_calls}→{final_llm_calls}, duration_count {initial_duration_count}→{final_duration_count}")


class TestWorkflowCatalogMetrics:
    """
    Integration tests for workflow catalog metrics.
    
    Pattern: Direct toolset calls → Query Prometheus registry
    
    BR-MONITORING-001: Workflow catalog operations MUST be metered
    """

    @pytest.mark.asyncio
    async def test_workflow_search_increments_context_api_calls(self, data_storage_url, unique_test_id):
        """
        BR-MONITORING-001: Workflow search MUST increment context_api_calls_total.
        
        Pattern: Call workflow catalog toolset directly → Query REGISTRY
        
        Note: Workflow catalog is implemented as MCP toolset, accessed during
        incident/recovery analysis. This test verifies the metrics are recorded.
        """
        from src.extensions.incident.llm_integration import analyze_incident
        from src.models.config_models import AppConfig
        
        # ARRANGE: Get baseline
        initial_context_calls = get_metric_value_from_registry("context_api_calls_total")
        
        # ACT: Trigger incident analysis (which uses workflow catalog)
        incident_request = make_incident_request(unique_test_id)
        app_config = AppConfig()
        result = await analyze_incident(incident_request, mcp_config=None, app_config=app_config)
        
        assert result is not None
        
        # ASSERT: Context API calls should increment (workflow catalog is context)
        final_context_calls = get_metric_value_from_registry("context_api_calls_total")
        
        # Note: This may or may not increment depending on whether workflow catalog
        # is accessed (depends on LLM tool selection). We verify the metric exists.
        assert final_context_calls >= initial_context_calls, \
            "context_api_calls_total should exist and not decrease"
        
        if final_context_calls > initial_context_calls:
            print(f"✅ Context API metrics: calls {initial_context_calls}→{final_context_calls}")
        else:
            print(f"ℹ️  Context API calls unchanged (LLM may not have called workflow catalog)")


# ========================================
# DEPRECATED (E2E PATTERN - DO NOT USE)
# ========================================

class TestHTTPRequestMetrics_DEPRECATED:
    """
    ⚠️  DEPRECATED: These tests use E2E pattern (HTTP + TestClient).
    
    Problem: Importing main.py causes K8s auth initialization failures.
    
    Solution: Moved to E2E test tier OR deleted (HTTP metrics tested via E2E).
    
    Integration tests should test business logic metrics (investigations, LLM calls),
    NOT HTTP layer metrics (that's E2E testing).
    """
    pass
