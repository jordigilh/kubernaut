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
HAPI Metric Name Constants (DD-005 v3.0 Compliance)

Design Decision: DD-005 v3.0 Section 1.1 - Metric Name Constants (MANDATORY)

All metric names MUST be defined as constants to prevent typos and ensure
test/production parity. These constants are used in both production code and
integration tests.

Business Requirements:
- BR-HAPI-011: Investigation Metrics
- BR-HAPI-301: LLM Observability Metrics
- BR-HAPI-302: HTTP Request Metrics (DD-005 Standard)
- BR-HAPI-303: Config Hot-Reload Metrics

Why Constants Are MANDATORY:
- **Typo Prevention**: Compiler catches typos at build time, not runtime
- **Maintenance**: Update metric names in ONE location (DRY principle)
- **Test Safety**: Tests use same constants as production code
- **Refactoring**: IDE "Find Usages" + Rename works across codebase
- **Documentation**: Explicit Go doc comments on each constant

Reference Implementation: pkg/gateway/metrics/metrics.go (Go service)
"""

# ========================================
# INVESTIGATION METRICS (BR-HAPI-011)
# ========================================

METRIC_NAME_INVESTIGATIONS_TOTAL = 'holmesgpt_api_investigations_total'
"""
Total number of investigation requests by outcome.

Business Requirement: BR-HAPI-011
Type: Counter
Labels: status (success | error | needs_review)
SLO: Success rate > 95%
"""

METRIC_NAME_INVESTIGATIONS_DURATION = 'holmesgpt_api_investigations_duration_seconds'
"""
Time spent processing investigation requests (incident + recovery).

Business Requirement: BR-HAPI-011
Type: Histogram
Labels: none
Buckets: (0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0, 120.0)
SLO: P95 latency < 10 seconds
"""

# ========================================
# LLM METRICS (BR-HAPI-301)
# ========================================

METRIC_NAME_LLM_CALLS_TOTAL = 'holmesgpt_api_llm_calls_total'
"""
Total number of LLM API calls by provider, model, and outcome.

Business Requirement: BR-HAPI-301
Type: Counter
Labels: provider (openai | anthropic | ollama), model (gpt-4 | claude-3 | ...), status (success | error | timeout)
SLO: Error rate < 1%
"""

METRIC_NAME_LLM_CALL_DURATION = 'holmesgpt_api_llm_call_duration_seconds'
"""
LLM API call latency distribution (streaming excluded).

Business Requirement: BR-HAPI-301
Type: Histogram
Labels: provider, model
Buckets: (0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0)
SLO: OpenAI P95 < 5s, Claude P95 < 10s, Ollama P95 < 2s
"""

METRIC_NAME_LLM_TOKEN_USAGE = 'holmesgpt_api_llm_token_usage_total'
"""
Total tokens consumed by LLM calls (for cost tracking).

Business Requirement: BR-HAPI-301
Type: Counter
Labels: provider, model, type (prompt | completion)
Alert: > $100/day
"""

# ========================================
# HTTP REQUEST METRICS (BR-HAPI-302, DD-005)
# ========================================

METRIC_NAME_HTTP_REQUESTS_TOTAL = 'holmesgpt_api_http_requests_total'
"""
Total HTTP requests by method, endpoint, and status code.

Business Requirement: BR-HAPI-302
Design Standard: DD-005 Section 3.1 (HTTP Metrics)
Type: Counter
Labels: method (GET | POST), endpoint (/api/v1/incident/analyze), status (200 | 400 | 500)
SLO: Availability > 99.9%, Error rate < 0.1%
"""

METRIC_NAME_HTTP_REQUEST_DURATION = 'holmesgpt_api_http_request_duration_seconds'
"""
HTTP request latency distribution (excluding LLM time).

Business Requirement: BR-HAPI-302
Design Standard: DD-005 Section 3.1 (HTTP Metrics)
Type: Histogram
Labels: method, endpoint
Buckets: (0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0) - Per DD-005 standard
SLO: P95 latency < 100ms (HTTP overhead only)
"""

# ========================================
# CONFIG HOT-RELOAD METRICS (BR-HAPI-303)
# ========================================

METRIC_NAME_CONFIG_RELOAD_TOTAL = 'holmesgpt_api_config_reload_total'
"""
Total successful configuration reloads.

Business Requirement: BR-HAPI-303 (BR-HAPI-199 compliance)
Type: Counter
Labels: none
"""

METRIC_NAME_CONFIG_RELOAD_ERRORS = 'holmesgpt_api_config_reload_errors_total'
"""
Total failed configuration reload attempts.

Business Requirement: BR-HAPI-303 (BR-HAPI-199 compliance)
Type: Counter
Labels: none
"""

METRIC_NAME_CONFIG_LAST_RELOAD_TIMESTAMP = 'holmesgpt_api_config_last_reload_timestamp'
"""
Unix timestamp of last successful configuration reload.

Business Requirement: BR-HAPI-303 (BR-HAPI-199 compliance)
Type: Gauge
Labels: none
"""

# ========================================
# RFC 7807 ERROR METRICS (BR-HAPI-200)
# ========================================

METRIC_NAME_RFC7807_ERRORS_TOTAL = 'holmesgpt_api_rfc7807_errors_total'
"""
Total RFC 7807 error responses by status code and error type.

Business Requirement: BR-HAPI-200
Type: Counter
Labels: status_code (400 | 404 | 500), error_type (validation_error | not_found | internal_error)
"""

# ========================================
# LABEL VALUE CONSTANTS
# ========================================

# Investigation status labels (BR-HAPI-011)
LABEL_STATUS_SUCCESS = 'success'
LABEL_STATUS_ERROR = 'error'
LABEL_STATUS_NEEDS_REVIEW = 'needs_review'
LABEL_STATUS_TIMEOUT = 'timeout'

# LLM provider labels (BR-HAPI-301)
LABEL_PROVIDER_OPENAI = 'openai'
LABEL_PROVIDER_ANTHROPIC = 'anthropic'
LABEL_PROVIDER_OLLAMA = 'ollama'

# LLM token type labels (BR-HAPI-301)
LABEL_TOKEN_TYPE_PROMPT = 'prompt'
LABEL_TOKEN_TYPE_COMPLETION = 'completion'
