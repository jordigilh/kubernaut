#
# Copyright 2025 Jordi Gil.
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
HTTP Middleware Metrics (FastAPI Pattern)

Architecture Note:
- HTTP metrics stay in middleware (FastAPI best practice)
- Business metrics moved to HAMetrics class (Go pattern)
- See: src/metrics/instrumentation.py for business logic metrics

Migration (Jan 31, 2026):
- REMOVED: investigations_total, investigations_duration_seconds (moved to HAMetrics)
- REMOVED: llm_calls_total, llm_call_duration_seconds, llm_token_usage (moved to HAMetrics)
- REMOVED: active_requests, auth_*, context_api_* (no BR backing)
- REMOVED: http_requests_total, http_request_duration_seconds (GitHub #294 - internal-only)
- REMOVED: config_reload_* (GitHub #294 - internal-only)
- REMOVED: rfc7807_errors_total (GitHub #294 - internal-only)
"""

from typing import Callable
from fastapi import Request, Response
from starlette.middleware.base import BaseHTTPMiddleware
from prometheus_client import generate_latest, CONTENT_TYPE_LATEST


# ========================================
# METRICS MIDDLEWARE
# ========================================

class PrometheusMetricsMiddleware(BaseHTTPMiddleware):
    """
    HTTP middleware placeholder for Prometheus metrics instrumentation.

    Internal-only HTTP/config/RFC7807 metrics removed per GitHub #294.
    Business metrics: See src/metrics/instrumentation.py (HAMetrics class)
    """

    async def dispatch(self, request: Request, call_next: Callable) -> Response:
        """Pass through request without metric recording."""
        return await call_next(request)


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


