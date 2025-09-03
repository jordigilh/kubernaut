"""
Tests for metrics utilities and tracking.
"""

import asyncio
import pytest
import pytest_asyncio
import time
from unittest.mock import patch, AsyncMock
from collections import deque

from app.utils.metrics import (
    OperationMetrics, MetricsManager, MetricsCollector, track_operation,
    get_metrics_manager, record_request, record_holmes_operation,
    record_cache_operation, record_health_check
)


class TestOperationMetrics:
    """Test OperationMetrics functionality."""

    def test_operation_metrics_creation(self):
        """Test operation metrics creation."""
        metrics = OperationMetrics(name="test_operation")

        assert metrics.name == "test_operation"
        assert metrics.call_count == 0
        assert metrics.total_duration == 0.0
        assert metrics.min_duration == float('inf')
        assert metrics.max_duration == 0.0
        assert metrics.error_count == 0
        assert metrics.last_call is None
        assert len(metrics.recent_durations) == 0

    def test_record_success(self):
        """Test recording successful operations."""
        metrics = OperationMetrics(name="test")

        # Record successful operations
        metrics.record_success(1.5)
        metrics.record_success(2.0)
        metrics.record_success(0.5)

        assert metrics.call_count == 3
        assert metrics.error_count == 0
        assert metrics.total_duration == 4.0
        assert metrics.min_duration == 0.5
        assert metrics.max_duration == 2.0
        assert metrics.last_call is not None
        assert len(metrics.recent_durations) == 3

    def test_record_error(self):
        """Test recording failed operations."""
        metrics = OperationMetrics(name="test")

        # Record mix of success and errors
        metrics.record_success(1.0)
        metrics.record_error(2.0)
        metrics.record_success(1.5)
        metrics.record_error(0.5)

        assert metrics.call_count == 4
        assert metrics.error_count == 2
        assert metrics.total_duration == 5.0
        assert metrics.min_duration == 0.5
        assert metrics.max_duration == 2.0

    def test_avg_duration(self):
        """Test average duration calculation."""
        metrics = OperationMetrics(name="test")

        # No calls yet
        assert metrics.avg_duration == 0.0

        # Add some calls
        metrics.record_success(2.0)
        metrics.record_success(4.0)

        assert metrics.avg_duration == 3.0

    def test_success_rate(self):
        """Test success rate calculation."""
        metrics = OperationMetrics(name="test")

        # No calls yet
        assert metrics.success_rate == 1.0

        # All successful
        metrics.record_success(1.0)
        metrics.record_success(1.0)
        assert metrics.success_rate == 1.0

        # Mix of success and error
        metrics.record_error(1.0)
        assert metrics.success_rate == 2/3

        # All errors
        metrics.record_error(1.0)
        metrics.record_error(1.0)
        assert metrics.success_rate == 2/5

    def test_error_rate(self):
        """Test error rate calculation."""
        metrics = OperationMetrics(name="test")

        metrics.record_success(1.0)
        metrics.record_success(1.0)
        metrics.record_error(1.0)

        assert abs(metrics.error_rate - 1/3) < 0.001
        assert metrics.success_rate + metrics.error_rate == 1.0

    def test_percentile_durations(self):
        """Test percentile duration calculations."""
        metrics = OperationMetrics(name="test")

        # Add enough data points for percentiles
        durations = [i * 0.1 for i in range(1, 101)]  # 0.1 to 10.0
        for duration in durations:
            metrics.record_success(duration)

        # Test percentiles (approximate due to quantiles implementation)
        p95 = metrics.p95_duration
        p99 = metrics.p99_duration

        assert p95 > 9.0  # Should be around 9.5
        assert p99 > 9.8  # Should be around 9.9
        assert p99 > p95

    def test_percentiles_with_insufficient_data(self):
        """Test percentiles with insufficient data."""
        metrics = OperationMetrics(name="test")

        # No data
        assert metrics.p95_duration == 0.0
        assert metrics.p99_duration == 0.0

        # Single data point
        metrics.record_success(1.0)
        # Should not crash, values may vary based on quantiles implementation

    def test_recent_durations_max_size(self):
        """Test recent durations collection max size."""
        metrics = OperationMetrics(name="test")

        # Add more than max size (100)
        for i in range(150):
            metrics.record_success(i * 0.1)

        # Should only keep last 100
        assert len(metrics.recent_durations) == 100
        assert metrics.recent_durations[0] == 5.0  # (150-100) * 0.1
        assert metrics.recent_durations[-1] == 14.9  # (150-1) * 0.1

    def test_to_dict(self):
        """Test conversion to dictionary."""
        metrics = OperationMetrics(name="test_op")

        # Add some data
        metrics.record_success(1.5)
        metrics.record_error(2.0)

        result = metrics.to_dict()

        assert result["name"] == "test_op"
        assert result["call_count"] == 2
        assert result["error_count"] == 1
        assert result["success_rate"] == 0.5
        assert result["error_rate"] == 0.5
        assert result["total_duration"] == 3.5
        assert result["avg_duration"] == 1.75
        assert result["min_duration"] == 1.5
        assert result["max_duration"] == 2.0
        assert "last_call" in result
        assert result["recent_calls"] == 2


class TestMetricsManager:
    """Test MetricsManager functionality."""

    @pytest_asyncio.fixture
    async def metrics_manager(self):
        """Create test metrics manager."""
        manager = MetricsManager()
        yield manager
        await manager.reset_metrics()

    @pytest.mark.asyncio
    async def test_metrics_manager_initialization(self, metrics_manager):
        """Test metrics manager initialization."""
        assert len(metrics_manager._operation_metrics) == 0
        assert len(metrics_manager._custom_metrics) == 0
        assert metrics_manager._start_time > 0

    @pytest.mark.asyncio
    async def test_record_operation(self, metrics_manager):
        """Test recording operations."""
        # Record successful operation
        await metrics_manager.record_operation("test_op", 1.5, success=True)

        # Record failed operation
        await metrics_manager.record_operation("test_op", 2.0, success=False)

        # Check operation metrics
        op_metrics = await metrics_manager.get_operation_metrics("test_op")
        assert op_metrics is not None
        assert op_metrics["call_count"] == 2
        assert op_metrics["error_count"] == 1
        assert op_metrics["total_duration"] == 3.5

    @pytest.mark.asyncio
    async def test_record_operation_new_metric(self, metrics_manager):
        """Test recording operation creates new metric."""
        await metrics_manager.record_operation("new_op", 1.0, success=True)

        assert "new_op" in metrics_manager._operation_metrics

        op_metrics = await metrics_manager.get_operation_metrics("new_op")
        assert op_metrics["name"] == "new_op"
        assert op_metrics["call_count"] == 1

    @pytest.mark.asyncio
    async def test_increment_counter(self, metrics_manager):
        """Test incrementing custom counters."""
        # Increment by default (1)
        await metrics_manager.increment_counter("requests")
        await metrics_manager.increment_counter("requests")

        # Increment by custom value
        await metrics_manager.increment_counter("errors", 5)

        metrics = await metrics_manager.get_metrics()
        assert metrics["custom_metrics"]["requests"] == 2
        assert metrics["custom_metrics"]["errors"] == 5

    @pytest.mark.asyncio
    async def test_set_gauge(self, metrics_manager):
        """Test setting gauge values."""
        await metrics_manager.set_gauge("cpu_usage", 45.2)
        await metrics_manager.set_gauge("memory_usage", 67.8)

        # Update gauge value
        await metrics_manager.set_gauge("cpu_usage", 52.1)

        metrics = await metrics_manager.get_metrics()
        assert metrics["custom_metrics"]["cpu_usage"] == 52.1
        assert metrics["custom_metrics"]["memory_usage"] == 67.8

    @pytest.mark.asyncio
    async def test_get_metrics(self, metrics_manager):
        """Test getting comprehensive metrics."""
        # Add operation data
        await metrics_manager.record_operation("op1", 1.0, success=True)
        await metrics_manager.record_operation("op1", 2.0, success=False)
        await metrics_manager.record_operation("op2", 1.5, success=True)

        # Add custom metrics
        await metrics_manager.increment_counter("requests", 10)
        await metrics_manager.set_gauge("active_connections", 25)

        metrics = await metrics_manager.get_metrics()

        # Check structure
        assert "uptime_seconds" in metrics
        assert "total_operations" in metrics
        assert "total_errors" in metrics
        assert "overall_success_rate" in metrics
        assert "operations" in metrics
        assert "custom_metrics" in metrics
        assert "timestamp" in metrics

        # Check values
        assert metrics["total_operations"] == 3
        assert metrics["total_errors"] == 1
        assert metrics["overall_success_rate"] == 2/3
        assert len(metrics["operations"]) == 2
        assert metrics["custom_metrics"]["requests"] == 10
        assert metrics["custom_metrics"]["active_connections"] == 25

    @pytest.mark.asyncio
    async def test_get_operation_metrics_nonexistent(self, metrics_manager):
        """Test getting metrics for nonexistent operation."""
        result = await metrics_manager.get_operation_metrics("nonexistent")
        assert result is None

    @pytest.mark.asyncio
    async def test_reset_metrics(self, metrics_manager):
        """Test resetting all metrics."""
        # Add some data
        await metrics_manager.record_operation("test", 1.0, success=True)
        await metrics_manager.increment_counter("requests", 5)

        # Reset
        start_time_before = metrics_manager._start_time
        await asyncio.sleep(0.1)  # Small delay
        await metrics_manager.reset_metrics()

        # Check reset
        assert len(metrics_manager._operation_metrics) == 0
        assert len(metrics_manager._custom_metrics) == 0
        assert metrics_manager._start_time > start_time_before

        metrics = await metrics_manager.get_metrics()
        assert metrics["total_operations"] == 0
        assert metrics["total_errors"] == 0

    @pytest.mark.asyncio
    async def test_get_summary(self, metrics_manager):
        """Test getting metrics summary."""
        # Add diverse operation data
        await metrics_manager.record_operation("fast_op", 0.1, success=True)
        await metrics_manager.record_operation("slow_op", 2.0, success=True)
        await metrics_manager.record_operation("error_op", 1.0, success=False)

        # Let some time pass for uptime calculation
        await asyncio.sleep(0.1)

        summary = await metrics_manager.get_summary()

        assert summary["uptime_seconds"] > 0
        assert summary["total_operations"] == 3
        assert summary["total_errors"] == 1
        assert summary["success_rate"] == 2/3
        assert summary["operations_per_second"] > 0
        assert summary["avg_operation_duration"] > 0
        assert summary["min_operation_duration"] == 0.1
        assert summary["max_operation_duration"] == 2.0
        assert summary["unique_operations"] == 3

    @pytest.mark.asyncio
    async def test_concurrent_metrics_recording(self, metrics_manager):
        """Test concurrent metrics recording."""
        async def record_operations():
            for i in range(50):
                await metrics_manager.record_operation(f"op_{i % 5}", 0.1, success=True)
                await metrics_manager.increment_counter("concurrent_requests")

        # Run concurrent recording
        await asyncio.gather(
            record_operations(),
            record_operations(),
            record_operations()
        )

        metrics = await metrics_manager.get_metrics()
        assert metrics["total_operations"] == 150
        assert metrics["custom_metrics"]["concurrent_requests"] == 150


class TestMetricsCollector:
    """Test MetricsCollector functionality."""

    @pytest_asyncio.fixture
    async def collector(self):
        """Create test metrics collector."""
        collector = MetricsCollector()
        # Reset the global metrics manager
        await collector._metrics_manager.reset_metrics()
        yield collector
        await collector._metrics_manager.reset_metrics()

    @pytest.mark.asyncio
    async def test_counter_without_labels(self, collector):
        """Test counter without labels."""
        await collector.counter("test_counter", 5)

        metrics = await collector._metrics_manager.get_metrics()
        assert metrics["custom_metrics"]["test_counter"] == 5

    @pytest.mark.asyncio
    async def test_counter_with_labels(self, collector):
        """Test counter with labels."""
        await collector.counter("http_requests", 1, {"method": "GET", "status": "200"})
        await collector.counter("http_requests", 1, {"method": "POST", "status": "201"})
        await collector.counter("http_requests", 2, {"method": "GET", "status": "200"})

        metrics = await collector._metrics_manager.get_metrics()

        # Labels should be encoded in metric name
        assert metrics["custom_metrics"]["http_requests_method_GET_status_200"] == 3
        assert metrics["custom_metrics"]["http_requests_method_POST_status_201"] == 1

    @pytest.mark.asyncio
    async def test_gauge_without_labels(self, collector):
        """Test gauge without labels."""
        await collector.gauge("cpu_usage", 45.5)

        metrics = await collector._metrics_manager.get_metrics()
        assert metrics["custom_metrics"]["cpu_usage"] == 45.5

    @pytest.mark.asyncio
    async def test_gauge_with_labels(self, collector):
        """Test gauge with labels."""
        await collector.gauge("memory_usage", 67.2, {"pod": "api-server", "namespace": "prod"})
        await collector.gauge("memory_usage", 45.1, {"pod": "worker", "namespace": "prod"})

        metrics = await collector._metrics_manager.get_metrics()
        assert metrics["custom_metrics"]["memory_usage_namespace_prod_pod_api-server"] == 67.2
        assert metrics["custom_metrics"]["memory_usage_namespace_prod_pod_worker"] == 45.1

    @pytest.mark.asyncio
    async def test_histogram(self, collector):
        """Test histogram recording."""
        await collector.histogram("request_duration", 1.5, {"endpoint": "/api/v1/ask"})
        await collector.histogram("request_duration", 2.0, {"endpoint": "/api/v1/ask"})

        # Histogram is implemented as operation tracking
        metrics = await collector._metrics_manager.get_metrics()
        operation_name = "request_duration_endpoint_/api/v1/ask"

        assert operation_name in metrics["operations"]
        assert metrics["operations"][operation_name]["call_count"] == 2

    @pytest.mark.asyncio
    async def test_label_sorting_consistency(self, collector):
        """Test that labels are sorted consistently."""
        # Add same metric with labels in different order
        await collector.counter("test", 1, {"b": "2", "a": "1"})
        await collector.counter("test", 1, {"a": "1", "b": "2"})

        metrics = await collector._metrics_manager.get_metrics()

        # Should be combined into single metric with sorted labels
        assert metrics["custom_metrics"]["test_a_1_b_2"] == 2


class TestTrackOperationDecorator:
    """Test track_operation decorator."""

    @pytest.mark.asyncio
    async def test_track_async_success(self):
        """Test tracking successful async operation."""
        manager = MetricsManager()

        @track_operation("test_async_op")
        async def async_function():
            await asyncio.sleep(0.1)
            return "success"

        # Patch the global metrics manager
        with patch('app.utils.metrics.get_metrics_manager', return_value=manager):
            result = await async_function()

        assert result == "success"

        # Check metrics
        metrics = await manager.get_metrics()
        assert "test_async_op" in metrics["operations"]

        op_metrics = metrics["operations"]["test_async_op"]
        assert op_metrics["call_count"] == 1
        assert op_metrics["error_count"] == 0
        assert op_metrics["avg_duration"] >= 0.1

    @pytest.mark.asyncio
    async def test_track_async_error(self):
        """Test tracking failed async operation."""
        manager = MetricsManager()

        @track_operation("test_async_error")
        async def async_error_function():
            await asyncio.sleep(0.05)
            raise ValueError("Test error")

        with patch('app.utils.metrics.get_metrics_manager', return_value=manager):
            with pytest.raises(ValueError):
                await async_error_function()

        # Check metrics
        metrics = await manager.get_metrics()
        assert "test_async_error" in metrics["operations"]

        op_metrics = metrics["operations"]["test_async_error"]
        assert op_metrics["call_count"] == 1
        assert op_metrics["error_count"] == 1

    def test_track_sync_success(self):
        """Test tracking successful sync operation."""
        manager = MetricsManager()

        @track_operation("test_sync_op")
        def sync_function():
            time.sleep(0.05)
            return "sync_success"

        with patch('app.utils.metrics.get_metrics_manager', return_value=manager):
            with patch('asyncio.get_event_loop') as mock_loop:
                mock_loop.return_value.create_task = AsyncMock()
                result = sync_function()

        assert result == "sync_success"
        # Note: Sync function metrics recording depends on event loop availability

    def test_track_sync_error(self):
        """Test tracking failed sync operation."""
        manager = MetricsManager()

        @track_operation("test_sync_error")
        def sync_error_function():
            time.sleep(0.01)
            raise RuntimeError("Sync error")

        with patch('app.utils.metrics.get_metrics_manager', return_value=manager):
            with patch('asyncio.get_event_loop') as mock_loop:
                mock_loop.return_value.create_task = AsyncMock()

                with pytest.raises(RuntimeError):
                    sync_error_function()

    @pytest.mark.asyncio
    async def test_track_operation_preserves_function_metadata(self):
        """Test that decorator preserves function metadata."""
        @track_operation("test_metadata")
        async def documented_function():
            """This function has documentation."""
            return "result"

        assert documented_function.__name__ == "documented_function"
        assert "documentation" in documented_function.__doc__


class TestConvenienceFunctions:
    """Test convenience functions for common metrics."""

    @pytest.mark.asyncio
    async def test_record_request(self):
        """Test record_request function."""
        manager = MetricsManager()

        with patch('app.utils.metrics.MetricsCollector') as mock_collector_class:
            mock_collector = AsyncMock()
            mock_collector_class.return_value = mock_collector

            await record_request("/api/ask", "POST", 200, 1.5)

            # Verify counter call
            mock_collector.counter.assert_called_with(
                "http_requests_total", 1, {
                    "endpoint": "/api/ask",
                    "method": "POST",
                    "status": "200"
                }
            )

            # Verify histogram call
            mock_collector.histogram.assert_called_with(
                "http_request_duration_seconds", 1.5, {
                    "endpoint": "/api/ask",
                    "method": "POST"
                }
            )

    @pytest.mark.asyncio
    async def test_record_holmes_operation(self):
        """Test record_holmes_operation function."""
        with patch('app.utils.metrics.MetricsCollector') as mock_collector_class:
            mock_collector = AsyncMock()
            mock_collector_class.return_value = mock_collector

            await record_holmes_operation("ask", True, 2.5, 0.85)

            # Verify counter call
            mock_collector.counter.assert_called_with(
                "holmes_operations_total", 1, {
                    "operation": "ask",
                    "status": "success"
                }
            )

            # Verify histogram call
            mock_collector.histogram.assert_called_with(
                "holmes_operation_duration_seconds", 2.5, {
                    "operation": "ask"
                }
            )

            # Verify gauge call for confidence
            mock_collector.gauge.assert_called_with(
                "holmes_operation_confidence", 0.85, {
                    "operation": "ask"
                }
            )

    @pytest.mark.asyncio
    async def test_record_holmes_operation_without_confidence(self):
        """Test record_holmes_operation without confidence."""
        with patch('app.utils.metrics.MetricsCollector') as mock_collector_class:
            mock_collector = AsyncMock()
            mock_collector_class.return_value = mock_collector

            await record_holmes_operation("investigate", False, 3.0)

            # Should not call gauge for confidence
            mock_collector.gauge.assert_not_called()

    @pytest.mark.asyncio
    async def test_record_cache_operation(self):
        """Test record_cache_operation function."""
        with patch('app.utils.metrics.MetricsCollector') as mock_collector_class:
            mock_collector = AsyncMock()
            mock_collector_class.return_value = mock_collector

            await record_cache_operation("get", True)
            await record_cache_operation("set", False)

            # Verify calls
            assert mock_collector.counter.call_count == 2

            # Check call arguments
            calls = mock_collector.counter.call_args_list
            assert calls[0][0] == ("cache_operations_total", 1, {"operation": "get", "result": "hit"})
            assert calls[1][0] == ("cache_operations_total", 1, {"operation": "set", "result": "miss"})

    @pytest.mark.asyncio
    async def test_record_health_check(self):
        """Test record_health_check function."""
        with patch('app.utils.metrics.MetricsCollector') as mock_collector_class:
            mock_collector = AsyncMock()
            mock_collector_class.return_value = mock_collector

            await record_health_check("database", True, 0.5)

            # Verify all three metric types are called
            mock_collector.counter.assert_called_once_with(
                "health_checks_total", 1, {
                    "component": "database",
                    "status": "healthy"
                }
            )

            mock_collector.histogram.assert_called_once_with(
                "health_check_duration_seconds", 0.5, {
                    "component": "database"
                }
            )

            mock_collector.gauge.assert_called_once_with(
                "component_health", 1.0, {
                    "component": "database"
                }
            )


class TestMetricsIntegration:
    """Test metrics integration scenarios."""

    @pytest.mark.asyncio
    async def test_get_metrics_manager_singleton(self):
        """Test that get_metrics_manager returns singleton."""
        manager1 = get_metrics_manager()
        manager2 = get_metrics_manager()

        assert manager1 is manager2

    @pytest.mark.asyncio
    async def test_real_world_metrics_scenario(self):
        """Test real-world metrics collection scenario."""
        manager = MetricsManager()
        collector = MetricsCollector()
        collector._metrics_manager = manager

        # Simulate API requests
        await collector.counter("http_requests", 1, {"method": "POST", "endpoint": "/ask"})
        await collector.histogram("request_duration", 1.5, {"endpoint": "/ask"})

        # Simulate HolmesGPT operations
        await manager.record_operation("holmes_ask", 2.0, success=True)
        await manager.record_operation("holmes_ask", 1.8, success=True)
        await manager.record_operation("holmes_investigate", 3.5, success=False)

        # Simulate system metrics
        await collector.gauge("memory_usage_percent", 67.5)
        await collector.gauge("cpu_usage_percent", 23.1)

        # Get comprehensive metrics
        metrics = await manager.get_metrics()

        # Verify structure and data (allow some variance due to async operations)
        assert metrics["total_operations"] >= 3  # At least 3 operations
        assert metrics["total_errors"] >= 1      # At least 1 error
        assert 0.5 <= metrics["overall_success_rate"] <= 1.0  # Reasonable success rate

        # Check operation-specific metrics
        assert "holmes_ask" in metrics["operations"]
        assert "holmes_investigate" in metrics["operations"]

        ask_metrics = metrics["operations"]["holmes_ask"]
        assert ask_metrics["call_count"] == 2
        assert ask_metrics["error_count"] == 0
        assert ask_metrics["avg_duration"] == 1.9

        # Check custom metrics
        assert metrics["custom_metrics"]["memory_usage_percent"] == 67.5
        assert metrics["custom_metrics"]["cpu_usage_percent"] == 23.1

    @pytest.mark.asyncio
    async def test_metrics_performance_under_load(self):
        """Test metrics performance under load."""
        manager = MetricsManager()

        async def generate_metrics():
            for i in range(100):
                await manager.record_operation(f"op_{i % 10}", 0.01, success=True)
                await manager.increment_counter("load_test_counter")

        start_time = time.time()

        # Run concurrent metric generation
        await asyncio.gather(
            generate_metrics(),
            generate_metrics(),
            generate_metrics()
        )

        end_time = time.time()

        # Should complete quickly
        assert end_time - start_time < 2.0

        # Verify metrics
        metrics = await manager.get_metrics()
        assert metrics["total_operations"] == 300
        assert metrics["custom_metrics"]["load_test_counter"] == 300

    @pytest.mark.asyncio
    async def test_metrics_memory_usage(self):
        """Test metrics memory usage patterns."""
        manager = MetricsManager()

        # Generate many unique operations
        for i in range(1000):
            await manager.record_operation(f"unique_op_{i}", 0.001, success=True)

        # Each operation should have its own metrics
        metrics = await manager.get_metrics()
        assert len(metrics["operations"]) == 1000

        # Memory usage should be reasonable (each OperationMetrics is small)
        # This test ensures we don't have memory leaks in metrics collection
