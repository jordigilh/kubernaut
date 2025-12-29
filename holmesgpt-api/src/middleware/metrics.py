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
Prometheus Metrics Middleware and Instrumentation

Business Requirements:
- BR-HAPI-100: Track investigation request counts
- BR-HAPI-101: Monitor investigation duration
- BR-HAPI-102: Track LLM API call metrics
- BR-HAPI-103: Monitor authentication failures
- BR-HAPI-200: RFC 7807 error response metrics

Design Decision: DD-HOLMESGPT-013 - Observability Strategy
- Prometheus metrics for production observability
- Structured logging for debugging
- Health checks for availability monitoring

Design Decision: DD-005 - Observability Standards
- Metric naming: {service}_{component}_{metric_name}_{unit}
- Service prefix: holmesgpt_api_ (NOT holmesgpt_)
- See: docs/architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md
"""

import logging
import time
from typing import Callable
from fastapi import Request, Response
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.routing import Match
from prometheus_client import Counter, Histogram, Gauge, generate_latest, CONTENT_TYPE_LATEST

logger = logging.getLogger(__name__)

# ========================================
# PROMETHEUS METRICS DEFINITIONS
# ========================================

# ========================================
# DD-005 COMPLIANT METRIC NAMING
# Format: {service}_{component}_{metric_name}_{unit}
# Service prefix: holmesgpt_api_ (per DD-005-OBSERVABILITY-STANDARDS.md)
# ========================================

# Investigation Requests
investigations_total = Counter(
    'holmesgpt_api_investigations_total',
    'Total number of investigation requests',
    ['method', 'endpoint', 'status']
)

investigations_duration_seconds = Histogram(
    'holmesgpt_api_investigations_duration_seconds',
    'Time spent processing investigation requests',
    ['method', 'endpoint'],
    buckets=(0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0, 120.0)
)

# LLM API Calls
llm_calls_total = Counter(
    'holmesgpt_api_llm_calls_total',
    'Total number of LLM API calls',
    ['provider', 'model', 'status']
)

llm_call_duration_seconds = Histogram(
    'holmesgpt_api_llm_call_duration_seconds',
    'Time spent on LLM API calls',
    ['provider', 'model'],
    buckets=(0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0)
)

llm_token_usage = Counter(
    'holmesgpt_api_llm_token_usage_total',
    'Total tokens consumed by LLM calls',
    ['provider', 'model', 'type']  # type: prompt, completion
)

# Authentication Failures
auth_failures_total = Counter(
    'holmesgpt_api_auth_failures_total',
    'Total number of authentication failures',
    ['reason', 'endpoint']
)

auth_success_total = Counter(
    'holmesgpt_api_auth_success_total',
    'Total number of successful authentications',
    ['username', 'role']
)

# Context API Integration
context_api_calls_total = Counter(
    'holmesgpt_api_context_calls_total',
    'Total number of Context API calls',
    ['endpoint', 'status']
)

context_api_duration_seconds = Histogram(
    'holmesgpt_api_context_duration_seconds',
    'Time spent on Context API calls',
    ['endpoint'],
    buckets=(0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0)
)

# ========================================
# CONFIG HOT-RELOAD METRICS (BR-HAPI-199)
# ========================================

config_reload_total = Counter(
    'holmesgpt_api_config_reload_total',
    'Total successful configuration reloads',
    []
)

config_reload_errors_total = Counter(
    'holmesgpt_api_config_reload_errors_total',
    'Total failed configuration reload attempts',
    []
)

config_last_reload_timestamp = Gauge(
    'holmesgpt_api_config_last_reload_timestamp',
    'Unix timestamp of last successful configuration reload',
    []
)

# Active Requests Gauge
active_requests = Gauge(
    'holmesgpt_api_active_requests',
    'Number of requests currently being processed',
    ['method', 'endpoint']
)

# HTTP Requests (General)
http_requests_total = Counter(
    'holmesgpt_api_http_requests_total',
    'Total HTTP requests',
    ['method', 'endpoint', 'status']
)

http_request_duration_seconds = Histogram(
    'holmesgpt_api_http_request_duration_seconds',
    'HTTP request duration',
    ['method', 'endpoint'],
    buckets=(0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0)
)

# RFC 7807 Error Responses
# BR-HAPI-200: RFC 7807 error response metrics
# REFACTOR phase: Track error responses by status code and error type
rfc7807_errors_total = Counter(
    'holmesgpt_api_rfc7807_errors_total',
    'Total RFC 7807 error responses by status code and error type',
    ['status_code', 'error_type']
)


# ========================================
# METRICS MIDDLEWARE
# ========================================

class PrometheusMetricsMiddleware(BaseHTTPMiddleware):
    """
    Middleware to automatically instrument HTTP requests with Prometheus metrics

    Business Requirement: BR-HAPI-100 to 103
    """

    async def dispatch(self, request: Request, call_next: Callable) -> Response:
        """
        Instrument HTTP request with Prometheus metrics
        """
        # Extract endpoint info
        method = request.method
        path = request.url.path

        # Normalize path (replace IDs with placeholder)
        endpoint = self._normalize_path(path)

        # Track active requests
        active_requests.labels(method=method, endpoint=endpoint).inc()

        # Start timer
        start_time = time.time()

        try:
            # Process request
            response = await call_next(request)
            status = response.status_code

            # Record metrics
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

            # Track investigation-specific metrics
            if endpoint.startswith('/api/v1/') and endpoint != '/api/v1/health':
                investigations_total.labels(
                    method=method,
                    endpoint=endpoint,
                    status=status
                ).inc()

                investigations_duration_seconds.labels(
                    method=method,
                    endpoint=endpoint
                ).observe(duration)

            logger.debug({
                "event": "request_completed",
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
                "event": "request_failed",
                "method": method,
                "endpoint": endpoint,
                "error": str(e),
                "duration": duration
            })

            raise

        finally:
            # Decrement active requests
            active_requests.labels(method=method, endpoint=endpoint).dec()

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
# HELPER FUNCTIONS FOR INSTRUMENTATION
# ========================================

def record_llm_call(
    provider: str,
    model: str,
    status: str,
    duration: float,
    prompt_tokens: int = 0,
    completion_tokens: int = 0
):
    """
    Record LLM API call metrics

    Business Requirement: BR-HAPI-102

    Args:
        provider: LLM provider (e.g., "vertex-ai", "ollama")
        model: Model name (e.g., "claude-3-5-sonnet")
        status: Call status ("success", "error", "timeout")
        duration: Call duration in seconds
        prompt_tokens: Number of tokens in prompt
        completion_tokens: Number of tokens in completion
    """
    llm_calls_total.labels(
        provider=provider,
        model=model,
        status=status
    ).inc()

    llm_call_duration_seconds.labels(
        provider=provider,
        model=model
    ).observe(duration)

    if prompt_tokens > 0:
        llm_token_usage.labels(
            provider=provider,
            model=model,
            type="prompt"
        ).inc(prompt_tokens)

    if completion_tokens > 0:
        llm_token_usage.labels(
            provider=provider,
            model=model,
            type="completion"
        ).inc(completion_tokens)

    logger.info({
        "event": "llm_call_recorded",
        "provider": provider,
        "model": model,
        "status": status,
        "duration": duration,
        "prompt_tokens": prompt_tokens,
        "completion_tokens": completion_tokens
    })


def record_auth_failure(reason: str, endpoint: str):
    """
    Record authentication failure

    Business Requirement: BR-HAPI-103

    Args:
        reason: Failure reason (e.g., "invalid_token", "expired_token", "no_token")
        endpoint: Endpoint where authentication failed
    """
    auth_failures_total.labels(
        reason=reason,
        endpoint=endpoint
    ).inc()

    logger.warning({
        "event": "auth_failure_recorded",
        "reason": reason,
        "endpoint": endpoint
    })


def record_auth_success(username: str, role: str):
    """
    Record successful authentication

    Business Requirement: BR-HAPI-103

    Args:
        username: Authenticated username
        role: User role
    """
    auth_success_total.labels(
        username=username,
        role=role
    ).inc()

    logger.debug({
        "event": "auth_success_recorded",
        "username": username,
        "role": role
    })


def record_context_api_call(endpoint: str, status: str, duration: float):
    """
    Record Context API call metrics

    Business Requirement: BR-HAPI-070

    Args:
        endpoint: Context API endpoint (e.g., "/api/v1/context/historical")
        status: Call status ("success", "error", "timeout")
        duration: Call duration in seconds
    """
    context_api_calls_total.labels(
        endpoint=endpoint,
        status=status
    ).inc()

    context_api_duration_seconds.labels(
        endpoint=endpoint
    ).observe(duration)

    logger.debug({
        "event": "context_api_call_recorded",
        "endpoint": endpoint,
        "status": status,
        "duration": duration
    })


def record_config_reload(success: bool):
    """
    Record configuration reload metrics

    Business Requirement: BR-HAPI-199
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
            "br": "BR-HAPI-199"
        })
    else:
        config_reload_errors_total.inc()
        logger.debug({
            "event": "config_reload_error_recorded",
            "success": False,
            "br": "BR-HAPI-199"
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


