"""
Metrics Service - Prometheus integration
Implements metrics collection and monitoring - BR-HAPI-016 through BR-HAPI-020
"""

import time
from typing import Dict, Any, Optional
from functools import wraps

import structlog
from prometheus_client import (
    Counter, Histogram, Gauge, Info, Enum,
    CollectorRegistry, generate_latest, CONTENT_TYPE_LATEST,
    start_http_server
)

logger = structlog.get_logger(__name__)


class MetricsService:
    """
    Prometheus Metrics Collection Service

    Provides comprehensive metrics for HolmesGPT REST API:
    - HTTP request metrics (BR-HAPI-016)
    - Investigation performance metrics (BR-HAPI-017)
    - Chat session metrics (BR-HAPI-018)
    - System health metrics (BR-HAPI-019)
    - Context API integration metrics (BR-HAPI-020)
    """

    def __init__(self, registry: Optional[CollectorRegistry] = None):
        self.registry = registry or CollectorRegistry()
        self._init_metrics()

    def _init_metrics(self):
        """Initialize all Prometheus metrics"""

        # Application Info
        self.app_info = Info(
            'holmesgpt_api_info',
            'HolmesGPT REST API Information',
            registry=self.registry
        )

        # HTTP Request Metrics
        self.http_requests_total = Counter(
            'holmesgpt_api_http_requests_total',
            'Total HTTP requests',
            ['method', 'endpoint', 'status'],
            registry=self.registry
        )

        self.http_request_duration_seconds = Histogram(
            'holmesgpt_api_http_request_duration_seconds',
            'HTTP request duration in seconds',
            ['method', 'endpoint'],
            buckets=(0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1.0, 2.5, 5.0, 7.5, 10.0),
            registry=self.registry
        )

        self.http_requests_in_progress = Gauge(
            'holmesgpt_api_http_requests_in_progress',
            'Number of HTTP requests currently being processed',
            ['method', 'endpoint'],
            registry=self.registry
        )

        # Investigation Metrics
        self.investigations_total = Counter(
            'holmesgpt_api_investigations_total',
            'Total number of investigations performed',
            ['status', 'alert_type', 'priority'],
            registry=self.registry
        )

        self.investigation_duration_seconds = Histogram(
            'holmesgpt_api_investigation_duration_seconds',
            'Investigation duration in seconds',
            ['alert_type', 'priority'],
            buckets=(1, 2, 5, 10, 15, 30, 60, 120, 300, 600),
            registry=self.registry
        )

        self.active_investigations = Gauge(
            'holmesgpt_api_investigations_active',
            'Number of investigations currently in progress',
            registry=self.registry
        )

        # Chat Session Metrics
        self.chat_sessions_total = Counter(
            'holmesgpt_api_chat_sessions_total',
            'Total number of chat sessions',
            ['status'],
            registry=self.registry
        )

        self.chat_messages_total = Counter(
            'holmesgpt_api_chat_messages_total',
            'Total number of chat messages processed',
            ['session_type'],
            registry=self.registry
        )

        self.chat_response_duration_seconds = Histogram(
            'holmesgpt_api_chat_response_duration_seconds',
            'Chat response duration in seconds',
            ['session_type'],
            buckets=(0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 15.0, 30.0),
            registry=self.registry
        )

        self.active_chat_sessions = Gauge(
            'holmesgpt_api_chat_sessions_active',
            'Number of active chat sessions',
            registry=self.registry
        )

        # HolmesGPT SDK Metrics
        self.holmesgpt_sdk_status = Enum(
            'holmesgpt_api_sdk_status',
            'HolmesGPT SDK status',
            states=['initializing', 'healthy', 'degraded', 'unhealthy'],
            registry=self.registry
        )

        self.holmesgpt_operations_total = Counter(
            'holmesgpt_api_sdk_operations_total',
            'Total SDK operations performed',
            ['operation', 'toolset', 'status'],
            registry=self.registry
        )

        self.holmesgpt_operation_duration_seconds = Histogram(
            'holmesgpt_api_sdk_operation_duration_seconds',
            'HolmesGPT operation duration in seconds',
            ['operation', 'toolset'],
            buckets=(0.5, 1.0, 2.5, 5.0, 10.0, 15.0, 30.0, 60.0, 120.0),
            registry=self.registry
        )

        # Context API Integration Metrics
        self.context_api_requests_total = Counter(
            'holmesgpt_api_context_requests_total',
            'Total Context API requests',
            ['operation', 'status'],
            registry=self.registry
        )

        self.context_api_duration_seconds = Histogram(
            'holmesgpt_api_context_duration_seconds',
            'Context API request duration in seconds',
            ['operation'],
            buckets=(0.1, 0.25, 0.5, 1.0, 2.0, 5.0, 10.0),
            registry=self.registry
        )

        self.context_api_status = Enum(
            'holmesgpt_api_context_status',
            'Context API connection status',
            states=['connected', 'degraded', 'disconnected'],
            registry=self.registry
        )

        # System Resource Metrics
        self.memory_usage_bytes = Gauge(
            'holmesgpt_api_memory_usage_bytes',
            'Memory usage in bytes',
            ['type'],
            registry=self.registry
        )

        self.cpu_usage_percent = Gauge(
            'holmesgpt_api_cpu_usage_percent',
            'CPU usage percentage',
            registry=self.registry
        )

        # Authentication Metrics
        self.auth_requests_total = Counter(
            'holmesgpt_api_auth_requests_total',
            'Total authentication requests',
            ['method', 'status'],
            registry=self.registry
        )

        self.active_sessions = Gauge(
            'holmesgpt_api_sessions_active',
            'Number of active user sessions',
            registry=self.registry
        )

        # Error Metrics
        self.errors_total = Counter(
            'holmesgpt_api_errors_total',
            'Total number of errors',
            ['type', 'component', 'severity'],
            registry=self.registry
        )

    def set_app_info(self, version: str, build_date: str, git_commit: str):
        """Set application information"""
        self.app_info.info({
            'version': version,
            'build_date': build_date,
            'git_commit': git_commit
        })

    def record_http_request(
        self,
        method: str,
        endpoint: str,
        status_code: int,
        duration: float
    ):
        """Record HTTP request metrics"""
        self.http_requests_total.labels(
            method=method,
            endpoint=endpoint,
            status=str(status_code)
        ).inc()

        self.http_request_duration_seconds.labels(
            method=method,
            endpoint=endpoint
        ).observe(duration)

    def track_investigation(
        self,
        alert_type: str,
        priority: str,
        status: str,
        duration: Optional[float] = None
    ):
        """Track investigation metrics"""
        self.investigations_total.labels(
            status=status,
            alert_type=alert_type,
            priority=priority
        ).inc()

        if duration is not None:
            self.investigation_duration_seconds.labels(
                alert_type=alert_type,
                priority=priority
            ).observe(duration)

    def track_chat_message(
        self,
        session_type: str,
        duration: Optional[float] = None
    ):
        """Track chat message metrics"""
        self.chat_messages_total.labels(
            session_type=session_type
        ).inc()

        if duration is not None:
            self.chat_response_duration_seconds.labels(
                session_type=session_type
            ).observe(duration)

    def track_holmesgpt_operation(
        self,
        operation: str,
        toolset: str,
        status: str,
        duration: Optional[float] = None
    ):
        """Track HolmesGPT SDK operation metrics"""
        self.holmesgpt_operations_total.labels(
            operation=operation,
            toolset=toolset,
            status=status
        ).inc()

        if duration is not None:
            self.holmesgpt_operation_duration_seconds.labels(
                operation=operation,
                toolset=toolset
            ).observe(duration)

    def track_context_api_request(
        self,
        operation: str,
        status: str,
        duration: Optional[float] = None
    ):
        """Track Context API request metrics"""
        self.context_api_requests_total.labels(
            operation=operation,
            status=status
        ).inc()

        if duration is not None:
            self.context_api_duration_seconds.labels(
                operation=operation
            ).observe(duration)

    def track_auth_request(self, method: str, status: str):
        """Track authentication request metrics"""
        self.auth_requests_total.labels(
            method=method,
            status=status
        ).inc()

    def record_error(self, error_type: str, component: str, severity: str):
        """Record error metrics"""
        self.errors_total.labels(
            type=error_type,
            component=component,
            severity=severity
        ).inc()

    def update_system_metrics(self, memory_usage: Dict[str, int], cpu_usage: float):
        """Update system resource metrics"""
        for mem_type, usage in memory_usage.items():
            self.memory_usage_bytes.labels(type=mem_type).set(usage)

        self.cpu_usage_percent.set(cpu_usage)

    def set_service_status(self, service: str, status: str):
        """Set service status"""
        if service == 'holmesgpt':
            self.holmesgpt_sdk_status.state(status)
        elif service == 'context_api':
            self.context_api_status.state(status)

    def increment_gauge(self, gauge_name: str, labels: Dict[str, str] = None):
        """Increment a gauge metric"""
        gauge = getattr(self, gauge_name, None)
        if gauge:
            if labels:
                gauge.labels(**labels).inc()
            else:
                gauge.inc()

    def decrement_gauge(self, gauge_name: str, labels: Dict[str, str] = None):
        """Decrement a gauge metric"""
        gauge = getattr(self, gauge_name, None)
        if gauge:
            if labels:
                gauge.labels(**labels).dec()
            else:
                gauge.dec()

    def get_metrics(self) -> str:
        """Get metrics in Prometheus format"""
        return generate_latest(self.registry).decode('utf-8')

    def get_content_type(self) -> str:
        """Get Prometheus metrics content type"""
        return CONTENT_TYPE_LATEST


# Decorators for automatic metrics collection

def track_request_metrics(metrics_service: MetricsService):
    """Decorator to automatically track HTTP request metrics"""
    def decorator(func):
        @wraps(func)
        async def wrapper(*args, **kwargs):
            start_time = time.time()

            # Extract request info from FastAPI request object
            request = kwargs.get('request') or (args[0] if args else None)
            method = getattr(request, 'method', 'unknown')
            url_path = getattr(request, 'url', {})
            endpoint = getattr(url_path, 'path', 'unknown')

            # Track in-progress request
            metrics_service.increment_gauge('http_requests_in_progress', {
                'method': method,
                'endpoint': endpoint
            })

            try:
                result = await func(*args, **kwargs)
                status_code = 200  # Default success
                return result
            except Exception as e:
                status_code = getattr(e, 'status_code', 500)
                raise
            finally:
                duration = time.time() - start_time

                # Record request metrics
                metrics_service.record_http_request(
                    method=method,
                    endpoint=endpoint,
                    status_code=status_code,
                    duration=duration
                )

                # Track completion
                metrics_service.decrement_gauge('http_requests_in_progress', {
                    'method': method,
                    'endpoint': endpoint
                })

        return wrapper
    return decorator


def track_investigation_metrics(metrics_service: MetricsService):
    """Decorator to automatically track investigation metrics"""
    def decorator(func):
        @wraps(func)
        async def wrapper(*args, **kwargs):
            start_time = time.time()

            # Track active investigation
            metrics_service.increment_gauge('active_investigations')

            try:
                result = await func(*args, **kwargs)
                status = 'success'
                return result
            except Exception as e:
                status = 'error'
                metrics_service.record_error(
                    error_type=type(e).__name__,
                    component='investigation',
                    severity='high'
                )
                raise
            finally:
                duration = time.time() - start_time

                # Extract investigation details from request
                request = kwargs.get('request') or (args[0] if args else None)
                alert_type = getattr(request, 'alert_name', 'unknown')
                priority = getattr(request, 'priority', 'medium')

                metrics_service.track_investigation(
                    alert_type=alert_type,
                    priority=priority,
                    status=status,
                    duration=duration
                )

                # Track completion
                metrics_service.decrement_gauge('active_investigations')

        return wrapper
    return decorator


# Global metrics service instance
_metrics_service = None

def get_metrics_service() -> MetricsService:
    """Get or create metrics service singleton"""
    global _metrics_service
    if _metrics_service is None:
        _metrics_service = MetricsService()
    return _metrics_service
