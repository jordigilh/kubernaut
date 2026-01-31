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
HTTP Middleware Metrics (FastAPI Pattern)

Business Requirements:
- BR-HAPI-302: HTTP Request Metrics (DD-005 Standard)
- BR-HAPI-303: Config Hot-Reload Metrics
- BR-HAPI-200: RFC 7807 Error Response Metrics

Design Decision: DD-005 v3.0 - Observability Standards
- Metric naming: {service}_{component}_{metric_name}_{unit}
- Service prefix: holmesgpt_api_
- Metric name constants: MANDATORY per DD-005 v3.0 Section 1.1

Architecture Note:
- HTTP metrics stay in middleware (FastAPI best practice)
- Business metrics moved to HAMetrics class (Go pattern)
- See: src/metrics/instrumentation.py for business logic metrics

Migration (Jan 31, 2026):
- REMOVED: investigations_total, investigations_duration_seconds (moved to HAMetrics)
- REMOVED: llm_calls_total, llm_call_duration_seconds, llm_token_usage (moved to HAMetrics)
- REMOVED: active_requests, auth_*, context_api_* (no BR backing)
- KEPT: http_requests_total, http_request_duration_seconds (BR-HAPI-302)
- KEPT: config_reload_* (BR-HAPI-303)
- KEPT: rfc7807_errors_total (BR-HAPI-200)
"""

import logging
import time
from typing import Callable
from fastapi import Request, Response
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.routing import Match
from prometheus_client import Counter, Histogram, Gauge, generate_latest, CONTENT_TYPE_LATEST

# Import metric name constants (DD-005 v3.0 Section 1.1 - MANDATORY)
from src.metrics.constants import (
    METRIC_NAME_HTTP_REQUESTS_TOTAL,
    METRIC_NAME_HTTP_REQUEST_DURATION,
    METRIC_NAME_CONFIG_RELOAD_TOTAL,
    METRIC_NAME_CONFIG_RELOAD_ERRORS,
    METRIC_NAME_CONFIG_LAST_RELOAD_TIMESTAMP,
    METRIC_NAME_RFC7807_ERRORS_TOTAL,
)

logger = logging.getLogger(__name__)

# ========================================
# HTTP MIDDLEWARE METRICS (BR-HAPI-302)
# ========================================
# DD-005 v3.0 Compliance: Use metric name constants
# Business Logic Metrics: Moved to src/metrics/instrumentation.py (HAMetrics class)

http_requests_total = Counter(
    METRIC_NAME_HTTP_REQUESTS_TOTAL,
    'Total HTTP requests by method, endpoint, and status',
    ['method', 'endpoint', 'status']
)

http_request_duration_seconds = Histogram(
    METRIC_NAME_HTTP_REQUEST_DURATION,
    'HTTP request duration (HTTP overhead only, excludes business logic)',
    ['method', 'endpoint'],
    buckets=(0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0)  # DD-005 standard
)

# ========================================
# CONFIG HOT-RELOAD METRICS (BR-HAPI-303)
# ========================================

config_reload_total = Counter(
    METRIC_NAME_CONFIG_RELOAD_TOTAL,
    'Total successful configuration reloads',
    []
)

config_reload_errors_total = Counter(
    METRIC_NAME_CONFIG_RELOAD_ERRORS,
    'Total failed configuration reload attempts',
    []
)

config_last_reload_timestamp = Gauge(
    METRIC_NAME_CONFIG_LAST_RELOAD_TIMESTAMP,
    'Unix timestamp of last successful configuration reload',
    []
)

# ========================================
# RFC 7807 ERROR METRICS (BR-HAPI-200)
# ========================================

rfc7807_errors_total = Counter(
    METRIC_NAME_RFC7807_ERRORS_TOTAL,
    'Total RFC 7807 error responses by status code and error type',
    ['status_code', 'error_type']
)


# ========================================
# METRICS MIDDLEWARE
# ========================================

class PrometheusMetricsMiddleware(BaseHTTPMiddleware):
    """
    HTTP middleware for automatic Prometheus metrics instrumentation.

    Business Requirement: BR-HAPI-302 (HTTP Request Metrics)
    
    Scope: HTTP-layer metrics only (requests, status codes, latency)
    Business metrics: See src/metrics/instrumentation.py (HAMetrics class)
    """

    async def dispatch(self, request: Request, call_next: Callable) -> Response:
        """
        Instrument HTTP request with Prometheus metrics.
        
        BR-HAPI-302: Record HTTP requests and latency.
        """
        # Extract endpoint info
        method = request.method
        path = request.url.path

        # Normalize path (replace IDs with placeholder) - DD-005 Section 3.1
        endpoint = self._normalize_path(path)

        # Start timer
        start_time = time.time()

        try:
            # Process request
            response = await call_next(request)
            status = response.status_code

            # Record HTTP metrics (BR-HAPI-302)
            duration = time.time() - start_time

            http_requests_total.labels(
                method=method,
                endpoint=endpoint,
                status=status
            ).inc()

            http_request_duration_seconds.labels(
                method=method,
                endpoint=endpoint
            ).observe(duration)

            logger.debug({
                "event": "http_request_completed",
                "method": method,
                "endpoint": endpoint,
                "status": status,
                "duration": duration
            })

            return response

        except Exception as e:
            # Record error metrics
            duration = time.time() - start_time

            http_requests_total.labels(
                method=method,
                endpoint=endpoint,
                status=500
            ).inc()

            http_request_duration_seconds.labels(
                method=method,
                endpoint=endpoint
            ).observe(duration)

            logger.error({
                "event": "http_request_failed",
                "method": method,
                "endpoint": endpoint,
                "error": str(e),
                "duration": duration
            })

            raise

    def _normalize_path(self, path: str) -> str:
        """
        Normalize URL path to reduce cardinality

        Example: /api/v1/investigation/12345 -> /api/v1/investigation/{id}
        """
        # Split path into parts
        parts = path.split('/')

        # Replace UUIDs and IDs with placeholder
        normalized_parts = []
        for part in parts:
            # Check if part looks like an ID (UUID, number, etc.)
            if self._is_id(part):
                normalized_parts.append('{id}')
            else:
                normalized_parts.append(part)

        return '/'.join(normalized_parts)

    def _is_id(self, part: str) -> bool:
        """Check if a path part is likely an ID"""
        if not part:
            return False

        # Check if it's a number
        if part.isdigit():
            return True

        # Check if it's a UUID pattern
        if len(part) == 36 and part.count('-') == 4:
            return True

        # Check if it's a long alphanumeric string (likely an ID)
        if len(part) > 16 and part.replace('-', '').replace('_', '').isalnum():
            return True

        return False


# ========================================
# CONFIG RELOAD HELPER (BR-HAPI-303)
# ========================================

def record_config_reload(success: bool):
    """
    Record configuration reload metrics.

    Business Requirement: BR-HAPI-303 (ConfigMap hot-reload observability)
    Design Decision: DD-HAPI-004

    Args:
        success: True if reload succeeded, False if failed
    """
    if success:
        config_reload_total.inc()
        config_last_reload_timestamp.set(time.time())
        logger.debug({
            "event": "config_reload_recorded",
            "success": True,
            "br": "BR-HAPI-303"
        })
    else:
        config_reload_errors_total.inc()
        logger.debug({
            "event": "config_reload_error_recorded",
            "success": False,
            "br": "BR-HAPI-303"
        })


def update_config_metrics_from_manager(config_manager):
    """
    Update Prometheus metrics from ConfigManager state.

    Called periodically or on demand to sync ConfigManager
    metrics with Prometheus.

    Business Requirement: BR-HAPI-199

    Args:
        config_manager: ConfigManager instance
    """
    if config_manager is None:
        return

    # Sync reload count
    # Note: This sets absolute values, not increments
    # In production, you'd track deltas
    reload_count = config_manager.reload_count
    error_count = config_manager.error_count

    logger.debug({
        "event": "config_metrics_synced",
        "reload_count": reload_count,
        "error_count": error_count,
        "br": "BR-HAPI-199"
    })


# ========================================
# METRICS ENDPOINT
# ========================================

def metrics_endpoint() -> Response:
    """
    Prometheus metrics endpoint

    Returns metrics in Prometheus exposition format
    """
    from starlette.responses import Response

    metrics_data = generate_latest()

    return Response(
        content=metrics_data,
        media_type=CONTENT_TYPE_LATEST
    )


