"""
Metrics utilities for tracking performance and operations.
"""

import asyncio
import time
import functools
from typing import Dict, Any, Callable, Optional
from dataclasses import dataclass, field
from collections import defaultdict, deque
import statistics


@dataclass
class OperationMetrics:
    """Metrics for a specific operation."""
    name: str
    call_count: int = 0
    total_duration: float = 0.0
    min_duration: float = float('inf')
    max_duration: float = 0.0
    error_count: int = 0
    last_call: Optional[float] = None
    recent_durations: deque = field(default_factory=lambda: deque(maxlen=100))

    @property
    def avg_duration(self) -> float:
        """Average duration."""
        return self.total_duration / max(self.call_count, 1)

    @property
    def success_rate(self) -> float:
        """Success rate (0.0 to 1.0)."""
        if self.call_count == 0:
            return 1.0
        return (self.call_count - self.error_count) / self.call_count

    @property
    def error_rate(self) -> float:
        """Error rate (0.0 to 1.0)."""
        return 1.0 - self.success_rate

    @property
    def p95_duration(self) -> float:
        """95th percentile duration."""
        if len(self.recent_durations) == 0:
            return 0.0
        elif len(self.recent_durations) == 1:
            return self.recent_durations[0]
        return statistics.quantiles(self.recent_durations, n=20)[18]  # 95th percentile

    @property
    def p99_duration(self) -> float:
        """99th percentile duration."""
        if len(self.recent_durations) == 0:
            return 0.0
        elif len(self.recent_durations) == 1:
            return self.recent_durations[0]
        return statistics.quantiles(self.recent_durations, n=100)[98]  # 99th percentile

    def record_success(self, duration: float) -> None:
        """Record a successful operation."""
        self.call_count += 1
        self.total_duration += duration
        self.min_duration = min(self.min_duration, duration)
        self.max_duration = max(self.max_duration, duration)
        self.last_call = time.time()
        self.recent_durations.append(duration)

    def record_error(self, duration: float) -> None:
        """Record a failed operation."""
        self.call_count += 1
        self.error_count += 1
        self.total_duration += duration
        self.min_duration = min(self.min_duration, duration)
        self.max_duration = max(self.max_duration, duration)
        self.last_call = time.time()
        self.recent_durations.append(duration)

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        return {
            "name": self.name,
            "call_count": self.call_count,
            "error_count": self.error_count,
            "success_rate": self.success_rate,
            "error_rate": self.error_rate,
            "total_duration": self.total_duration,
            "avg_duration": self.avg_duration,
            "min_duration": self.min_duration if self.min_duration != float('inf') else 0.0,
            "max_duration": self.max_duration,
            "p95_duration": self.p95_duration,
            "p99_duration": self.p99_duration,
            "last_call": self.last_call,
            "recent_calls": len(self.recent_durations)
        }


class MetricsManager:
    """Manager for collecting and reporting metrics."""

    def __init__(self):
        self._operation_metrics: Dict[str, OperationMetrics] = {}
        self._custom_metrics: Dict[str, Any] = defaultdict(int)
        self._start_time = time.time()
        self._lock = asyncio.Lock()

    async def record_operation(self, operation: str, duration: float, success: bool = True) -> None:
        """Record an operation metric."""
        async with self._lock:
            if operation not in self._operation_metrics:
                self._operation_metrics[operation] = OperationMetrics(name=operation)

            metrics = self._operation_metrics[operation]
            if success:
                metrics.record_success(duration)
            else:
                metrics.record_error(duration)

    async def increment_counter(self, counter: str, value: int = 1) -> None:
        """Increment a custom counter."""
        async with self._lock:
            self._custom_metrics[counter] += value

    async def set_gauge(self, gauge: str, value: float) -> None:
        """Set a gauge value."""
        async with self._lock:
            self._custom_metrics[gauge] = value

    async def get_metrics(self) -> Dict[str, Any]:
        """Get all metrics."""
        async with self._lock:
            uptime = time.time() - self._start_time

            # Operation metrics
            operations = {}
            for name, metrics in self._operation_metrics.items():
                operations[name] = metrics.to_dict()

            # Summary statistics
            total_calls = sum(m.call_count for m in self._operation_metrics.values())
            total_errors = sum(m.error_count for m in self._operation_metrics.values())

            return {
                "uptime_seconds": uptime,
                "total_operations": total_calls,
                "total_errors": total_errors,
                "overall_success_rate": (total_calls - total_errors) / max(total_calls, 1),
                "operations": operations,
                "custom_metrics": dict(self._custom_metrics),
                "timestamp": time.time()
            }

    async def get_operation_metrics(self, operation: str) -> Optional[Dict[str, Any]]:
        """Get metrics for a specific operation."""
        async with self._lock:
            if operation in self._operation_metrics:
                return self._operation_metrics[operation].to_dict()
            return None

    async def reset_metrics(self) -> None:
        """Reset all metrics."""
        async with self._lock:
            self._operation_metrics.clear()
            self._custom_metrics.clear()
            self._start_time = time.time()

    async def get_summary(self) -> Dict[str, Any]:
        """Get metrics summary."""
        async with self._lock:
            uptime = time.time() - self._start_time
            total_calls = sum(m.call_count for m in self._operation_metrics.values())
            total_errors = sum(m.error_count for m in self._operation_metrics.values())

            if self._operation_metrics:
                avg_duration = sum(m.avg_duration for m in self._operation_metrics.values()) / len(self._operation_metrics)
                min_duration = min(m.min_duration for m in self._operation_metrics.values() if m.min_duration != float('inf'))
                max_duration = max(m.max_duration for m in self._operation_metrics.values())
            else:
                avg_duration = min_duration = max_duration = 0.0

            return {
                "uptime_seconds": uptime,
                "total_operations": total_calls,
                "total_errors": total_errors,
                "success_rate": (total_calls - total_errors) / max(total_calls, 1),
                "operations_per_second": total_calls / max(uptime, 1),
                "avg_operation_duration": avg_duration,
                "min_operation_duration": min_duration,
                "max_operation_duration": max_duration,
                "unique_operations": len(self._operation_metrics),
                "custom_metrics_count": len(self._custom_metrics)
            }


# Global metrics manager instance
_metrics_manager: Optional[MetricsManager] = None


def get_metrics_manager() -> MetricsManager:
    """Get or create global metrics manager."""
    global _metrics_manager
    if _metrics_manager is None:
        _metrics_manager = MetricsManager()
    return _metrics_manager


def track_operation(operation_name: str):
    """Decorator to track operation metrics."""
    def decorator(func: Callable):
        if asyncio.iscoroutinefunction(func):
            @functools.wraps(func)
            async def async_wrapper(*args, **kwargs):
                start_time = time.time()
                success = False

                try:
                    result = await func(*args, **kwargs)
                    success = True
                    return result
                except Exception as e:
                    raise
                finally:
                    duration = time.time() - start_time
                    metrics_manager = get_metrics_manager()
                    await metrics_manager.record_operation(operation_name, duration, success)

            return async_wrapper
        else:
            @functools.wraps(func)
            def sync_wrapper(*args, **kwargs):
                start_time = time.time()
                success = False

                try:
                    result = func(*args, **kwargs)
                    success = True
                    return result
                except Exception as e:
                    raise
                finally:
                    duration = time.time() - start_time
                    metrics_manager = get_metrics_manager()
                    # For sync functions, we need to handle the async call differently
                    # This is a simplified approach - in practice, you might want to use a thread
                    try:
                        loop = asyncio.get_event_loop()
                        loop.create_task(metrics_manager.record_operation(operation_name, duration, success))
                    except RuntimeError:
                        # No event loop running, skip metrics
                        pass

            return sync_wrapper

    return decorator


class MetricsCollector:
    """Helper class for collecting custom metrics."""

    def __init__(self):
        self._metrics_manager = get_metrics_manager()

    async def counter(self, name: str, value: int = 1, labels: Optional[Dict[str, str]] = None) -> None:
        """Increment a counter."""
        metric_name = name
        if labels:
            label_str = "_".join(f"{k}_{v}" for k, v in sorted(labels.items()))
            metric_name = f"{name}_{label_str}"

        await self._metrics_manager.increment_counter(metric_name, value)

    async def gauge(self, name: str, value: float, labels: Optional[Dict[str, str]] = None) -> None:
        """Set a gauge value."""
        metric_name = name
        if labels:
            label_str = "_".join(f"{k}_{v}" for k, v in sorted(labels.items()))
            metric_name = f"{name}_{label_str}"

        await self._metrics_manager.set_gauge(metric_name, value)

    async def histogram(self, name: str, value: float, labels: Optional[Dict[str, str]] = None) -> None:
        """Record a histogram value (simplified as duration tracking)."""
        metric_name = name
        if labels:
            label_str = "_".join(f"{k}_{v}" for k, v in sorted(labels.items()))
            metric_name = f"{name}_{label_str}"

        await self._metrics_manager.record_operation(metric_name, value, True)


# Convenience functions for common metrics
async def record_request(endpoint: str, method: str, status_code: int, duration: float) -> None:
    """Record HTTP request metrics."""
    collector = MetricsCollector()
    await collector.counter("http_requests_total", 1, {
        "endpoint": endpoint,
        "method": method,
        "status": str(status_code)
    })
    await collector.histogram("http_request_duration_seconds", duration, {
        "endpoint": endpoint,
        "method": method
    })


async def record_holmes_operation(operation: str, success: bool, duration: float, confidence: Optional[float] = None) -> None:
    """Record HolmesGPT operation metrics."""
    collector = MetricsCollector()

    await collector.counter("holmes_operations_total", 1, {
        "operation": operation,
        "status": "success" if success else "error"
    })

    await collector.histogram("holmes_operation_duration_seconds", duration, {
        "operation": operation
    })

    if confidence is not None:
        await collector.gauge("holmes_operation_confidence", confidence, {
            "operation": operation
        })


async def record_cache_operation(operation: str, hit: bool) -> None:
    """Record cache operation metrics."""
    collector = MetricsCollector()
    await collector.counter("cache_operations_total", 1, {
        "operation": operation,
        "result": "hit" if hit else "miss"
    })


# Health metrics
async def record_health_check(component: str, healthy: bool, response_time: float) -> None:
    """Record health check metrics."""
    collector = MetricsCollector()

    await collector.counter("health_checks_total", 1, {
        "component": component,
        "status": "healthy" if healthy else "unhealthy"
    })

    await collector.histogram("health_check_duration_seconds", response_time, {
        "component": component
    })

    await collector.gauge("component_health", 1.0 if healthy else 0.0, {
        "component": component
    })
