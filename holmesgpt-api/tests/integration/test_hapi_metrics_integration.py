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
Integration Tests for HAPI Metrics (Go Pattern)

Business Requirements:
- BR-HAPI-011: Investigation Metrics
- BR-HAPI-301: LLM Observability Metrics

Test Pattern: Direct business logic calls + Prometheus registry inspection (like Go)

✅ INTEGRATION TEST PATTERN (Matches Go Gateway/AIAnalysis):
1. Create custom Prometheus registry (test isolation)
2. Create HAMetrics instance with test registry
3. Call business logic directly (analyze_incident, analyze_recovery)
4. Query test registry for metric values
5. Verify metrics incremented correctly

This pattern:
- Follows Go service testing pattern (Gateway, AIAnalysis)
- Tests business logic metrics emission (integration testing)
- No HTTP layer (no TestClient, no main.py import)
- No K8s auth initialization issues

Reference:
- test/integration/gateway/metrics_emission_integration_test.go
- test/integration/aianalysis/metrics_integration_test.go
"""

import time
import pytest
from typing import Dict, Any
from prometheus_client import CollectorRegistry

# Import business logic and metrics
from src.extensions.incident.llm_integration import analyze_incident
from src.extensions.recovery.llm_integration import analyze_recovery
from src.metrics import HAMetrics
from src.models.config_models import AppConfig


# ========================================
# HELPER FUNCTIONS
# ========================================

def get_counter_value(test_metrics: HAMetrics, counter_name: str, labels: Dict[str, str] = None) -> float:
    """
    Get counter value from test metrics registry (like Go's getCounterValue).
    
    Args:
        test_metrics: HAMetrics instance with custom registry
        counter_name: Metric attribute name (e.g., 'investigations_total')
        labels: Optional label filters
    
    Returns:
        Counter value as float
        
    Example:
        value = get_counter_value(test_metrics, 'investigations_total', {'status': 'success'})
    """
    # Get the metric name from the counter
    counter = getattr(test_metrics, counter_name, None)
    if counter is None:
        print(f"⚠️  Counter {counter_name} not found on metrics instance")
        return 0.0
    
    # Collect metrics from registry (most reliable method)
    try:
        for collector in test_metrics.registry.collect():
            for sample in collector.samples:
                # Match metric name
                if sample.name == counter._name or sample.name.startswith(counter._name):
                    # If labels specified, check if all match
                    if labels:
                        all_match = all(sample.labels.get(k) == v for k, v in labels.items())
                        if not all_match:
                            continue
                    
                    # Return the value
                    return float(sample.value)
        
        # No matching sample found
        return 0.0
        
    except Exception as e:
        print(f"⚠️  Error collecting metrics: {e}")
        import traceback
        traceback.print_exc()
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
    
    Pattern: Custom registry → Inject metrics → Call business logic → Query registry
    
    BR-HAPI-011: Investigation request metrics
    """

    @pytest.mark.asyncio
    async def test_incident_analysis_increments_investigations_total(self, unique_test_id):
        """
        BR-HAPI-011: Incident analysis MUST increment investigations_total metric.
        
        Pattern (like Go):
        1. Create test registry
        2. Create HAMetrics with test registry
        3. Get initial metric value
        4. Call analyze_incident() with test metrics
        5. Query test registry for new value
        6. Verify increment
        """
        # ARRANGE: Create test registry (like Go's prometheus.NewRegistry())
        test_registry = CollectorRegistry()
        test_metrics = HAMetrics(registry=test_registry)
        
        # Get baseline
        initial_value = get_counter_value(test_metrics, 'investigations_total', {'status': 'success'})
        
        # ACT: Call business logic with test metrics (Go pattern)
        incident_request = make_incident_request(unique_test_id)
        app_config = AppConfig()
        
        result = await analyze_incident(
            request_data=incident_request,
            mcp_config=None,
            app_config=app_config,
            metrics=test_metrics  # ✅ Inject test metrics
        )
        
        # Verify business operation succeeded
        assert result is not None, "Incident analysis should return result"
        
        # ASSERT: Query test registry for updated metric
        final_value = get_counter_value(test_metrics, 'investigations_total', {'status': 'success'})
        
        assert final_value == initial_value + 1, \
            f"investigations_total should increment by 1 (before: {initial_value}, after: {final_value})"
        
        print(f"✅ Metric validated: investigations_total increased from {initial_value} to {final_value}")

    @pytest.mark.asyncio
    async def test_incident_analysis_records_duration_histogram(self, unique_test_id):
        """
        BR-HAPI-011: Incident analysis MUST record duration histogram.
        
        Pattern: Inject test metrics → Call business logic → Verify histogram count
        """
        # ARRANGE
        test_registry = CollectorRegistry()
        test_metrics = HAMetrics(registry=test_registry)
        
        # Get baseline histogram count (query from registry)
        initial_count = 0.0
        for collector in test_registry.collect():
            for sample in collector.samples:
                if sample.name.endswith('_count') and 'investigations_duration' in sample.name:
                    initial_count = float(sample.value)
                    break
        
        # ACT: Call business logic
        incident_request = make_incident_request(unique_test_id)
        app_config = AppConfig()
        result = await analyze_incident(request_data=incident_request, mcp_config=None, app_config=app_config, metrics=test_metrics)
        
        assert result is not None
        
        # ASSERT: Histogram count should increment (query from registry)
        final_count = 0.0
        for collector in test_registry.collect():
            for sample in collector.samples:
                if sample.name.endswith('_count') and 'investigations_duration' in sample.name:
                    final_count = float(sample.value)
                    break
        
        assert final_count == initial_count + 1, \
            f"investigations_duration_seconds count should increment (before: {initial_count}, after: {final_count})"
        
        print(f"✅ Duration histogram updated: count {initial_count} → {final_count}")

    @pytest.mark.asyncio
    async def test_incident_analysis_records_needs_review_status(self, unique_test_id):
        """
        BR-HAPI-011: Incident analysis with needs_human_review MUST record correct status.
        
        Pattern: Test metrics recording for different outcomes
        """
        # ARRANGE
        test_registry = CollectorRegistry()
        test_metrics = HAMetrics(registry=test_registry)
        
        # ACT: Call business logic
        incident_request = make_incident_request(unique_test_id)
        app_config = AppConfig()
        result = await analyze_incident(request_data=incident_request, mcp_config=None, app_config=app_config, metrics=test_metrics)
        
        assert result is not None
        
        # ASSERT: Check appropriate status counter incremented
        success_count = get_counter_value(test_metrics, 'investigations_total', {'status': 'success'})
        needs_review_count = get_counter_value(test_metrics, 'investigations_total', {'status': 'needs_review'})
        
        if result.get("needs_human_review", False):
            assert needs_review_count >= 1, "needs_review counter should increment for needs_human_review=True"
            print(f"✅ Recorded needs_review status: count={needs_review_count}")
        else:
            assert success_count >= 1, "success counter should increment for successful analysis"
            print(f"✅ Recorded success status: count={success_count}")


class TestRecoveryAnalysisMetrics:
    """
    Integration tests for recovery analysis metrics.
    
    Pattern: Custom registry → Inject metrics → Call business logic → Query registry
    
    BR-HAPI-011: Investigation request metrics
    """

    @pytest.mark.asyncio
    async def test_recovery_analysis_increments_investigations_total(self, unique_test_id):
        """
        BR-HAPI-011: Recovery analysis MUST increment investigations_total metric.
        
        Pattern (like Go): Direct business logic call with injected test metrics
        """
        # ARRANGE
        test_registry = CollectorRegistry()
        test_metrics = HAMetrics(registry=test_registry)
        
        initial_value = get_counter_value(test_metrics, 'investigations_total', {'status': 'success'})
        
        # ACT: Call recovery business logic with test metrics
        recovery_request = make_recovery_request(unique_test_id)
        app_config = AppConfig()
        
        result = await analyze_recovery(
            request_data=recovery_request,
            app_config=app_config,
            metrics=test_metrics  # ✅ Inject test metrics
        )
        
        # Verify operation succeeded
        assert result is not None
        
        # ASSERT: Query test registry
        final_value = get_counter_value(test_metrics, 'investigations_total', {'status': 'success'})
        
        assert final_value == initial_value + 1, \
            f"Recovery analysis should increment investigations_total (before: {initial_value}, after: {final_value})"
        
        print(f"✅ Recovery metrics: investigations_total {initial_value} → {final_value}")

    @pytest.mark.asyncio
    async def test_recovery_analysis_records_duration(self, unique_test_id):
        """
        BR-HAPI-011: Recovery analysis MUST record duration histogram.
        
        Pattern: Inject test metrics → Verify histogram updated
        """
        # ARRANGE
        test_registry = CollectorRegistry()
        test_metrics = HAMetrics(registry=test_registry)
        
        # Get baseline histogram count (query from registry)
        initial_count = 0.0
        for collector in test_registry.collect():
            for sample in collector.samples:
                if sample.name.endswith('_count') and 'investigations_duration' in sample.name:
                    initial_count = float(sample.value)
                    break
        
        # ACT: Call recovery business logic
        recovery_request = make_recovery_request(unique_test_id)
        app_config = AppConfig()
        result = await analyze_recovery(request_data=recovery_request, app_config=app_config, metrics=test_metrics)
        
        assert result is not None
        
        # ASSERT: Histogram count incremented (query from registry)
        final_count = 0.0
        for collector in test_registry.collect():
            for sample in collector.samples:
                if sample.name.endswith('_count') and 'investigations_duration' in sample.name:
                    final_count = float(sample.value)
                    break
        
        assert final_count == initial_count + 1, \
            "Recovery should record duration histogram"
        
        print(f"✅ Recovery duration: histogram count {initial_count}→{final_count}")


class TestMetricsIsolation:
    """
    Verify test metrics isolation (custom registry pattern).
    
    BR-HAPI-011: Test isolation via custom registry
    """

    @pytest.mark.asyncio
    async def test_custom_registry_isolates_test_metrics(self, unique_test_id):
        """
        Integration test: Custom registry isolates test metrics from global registry.
        
        Pattern: Create two HAMetrics instances, verify independence
        """
        # ARRANGE: Create two independent test registries
        test_registry_1 = CollectorRegistry()
        test_metrics_1 = HAMetrics(registry=test_registry_1)
        
        test_registry_2 = CollectorRegistry()
        test_metrics_2 = HAMetrics(registry=test_registry_2)
        
        # ACT: Call business logic with metrics_1
        incident_request = make_incident_request(unique_test_id)
        app_config = AppConfig()
        await analyze_incident(request_data=incident_request, mcp_config=None, app_config=app_config, metrics=test_metrics_1)
        
        # ASSERT: Only metrics_1 should increment, metrics_2 should be zero
        value_1 = get_counter_value(test_metrics_1, 'investigations_total', {'status': 'success'})
        value_2 = get_counter_value(test_metrics_2, 'investigations_total', {'status': 'success'})
        
        assert value_1 >= 1, "metrics_1 should be incremented"
        assert value_2 == 0, "metrics_2 should remain at zero (isolated)"
        
        print(f"✅ Metrics isolation verified: registry_1={value_1}, registry_2={value_2}")
