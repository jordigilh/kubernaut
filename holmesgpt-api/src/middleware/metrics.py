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
Prometheus Metrics Endpoint

Provides the /metrics endpoint for Prometheus scraping.
Business metrics are defined in src/metrics/instrumentation.py (HAMetrics class).
HTTP middleware metrics were removed per GitHub #294 (internal-only, no BR backing).
"""

from __future__ import annotations

from prometheus_client import generate_latest, CONTENT_TYPE_LATEST
from starlette.responses import Response


# ========================================
# METRICS ENDPOINT
# ========================================

def metrics_endpoint() -> Response:
    """
    Prometheus metrics endpoint

    Returns metrics in Prometheus exposition format
    """
    metrics_data = generate_latest()

    return Response(
        content=metrics_data,
        media_type=CONTENT_TYPE_LATEST
    )


