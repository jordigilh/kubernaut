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
HAPI Metrics Instrumentation (Go Pattern)

Design Decision: DD-HAPI-METRICS-001 - Metrics in Business Logic
Pattern: Match Go Gateway/AIAnalysis pattern for consistency

Business Requirements:
- BR-HAPI-011: Investigation Metrics
- BR-HAPI-301: LLM Observability Metrics
- BR-HAPI-302: HTTP Request Metrics (DD-005 Standard)
- BR-HAPI-303: Config Hot-Reload Metrics

This module implements metrics following the Go service pattern:
1. Metrics class with injectable registry (like Go's metrics.Metrics struct)
2. Metrics incremented in business logic (NOT middleware)
3. Custom registry support for integration test isolation
4. Metric name constants (DD-005 v3.0 compliance)

Reference Implementation: pkg/gateway/metrics/metrics.go
"""

import time
from typing import Optional
from prometheus_client import Counter, Histogram, Gauge, CollectorRegistry, REGISTRY

from .constants import (
    # Investigation Metrics
    METRIC_NAME_INVESTIGATIONS_TOTAL,
    METRIC_NAME_INVESTIGATIONS_DURATION,
    
    # LLM Metrics
    METRIC_NAME_LLM_CALLS_TOTAL,
    METRIC_NAME_LLM_CALL_DURATION,
    METRIC_NAME_LLM_TOKEN_USAGE,
    
    # HTTP Metrics
    METRIC_NAME_HTTP_REQUESTS_TOTAL,
    METRIC_NAME_HTTP_REQUEST_DURATION,
    
    # Config Metrics
    METRIC_NAME_CONFIG_RELOAD_TOTAL,
    METRIC_NAME_CONFIG_RELOAD_ERRORS,
    METRIC_NAME_CONFIG_LAST_RELOAD_TIMESTAMP,
    
    # RFC 7807 Metrics
    METRIC_NAME_RFC7807_ERRORS_TOTAL,
    
    # Label constants
    LABEL_STATUS_SUCCESS,
    LABEL_STATUS_ERROR,
    LABEL_STATUS_NEEDS_REVIEW,
    LABEL_TOKEN_TYPE_PROMPT,
    LABEL_TOKEN_TYPE_COMPLETION,
)


class HAMetrics:
    """
    HAPI Prometheus metrics instrumentation (Go pattern).
    
    Pattern: Like Go's metrics.Metrics struct
    - Injectable via constructor
    - Testable with custom registry
    - Used directly in business logic
    
    Business Requirements:
    - BR-HAPI-011: Investigation request metrics
    - BR-HAPI-301: LLM API call metrics
    - BR-HAPI-302: HTTP request metrics (DD-005)
    - BR-HAPI-303: Config hot-reload metrics
    
    Usage:
        # Production (default registry)
        metrics = HAMetrics()
        
        # Integration tests (custom registry for isolation)
        test_registry = CollectorRegistry()
        test_metrics = HAMetrics(registry=test_registry)
    """
    
    def __init__(self, registry: Optional[CollectorRegistry] = None):
        """
        Initialize HAPI metrics with optional custom registry.
        
        Args:
            registry: Prometheus registry (defaults to global REGISTRY).
                     Integration tests inject custom registry for isolation.
        """
        self.registry = registry or REGISTRY
        
        # ========================================
        # INVESTIGATION METRICS (BR-HAPI-011)
        # ========================================
        
        self.investigations_total = Counter(
            METRIC_NAME_INVESTIGATIONS_TOTAL,
            'Total investigation requests by outcome',
            ['status'],  # success | error | needs_review
            registry=self.registry
        )
        
        self.investigations_duration = Histogram(
            METRIC_NAME_INVESTIGATIONS_DURATION,
            'Time spent processing investigation requests',
            buckets=(0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0, 120.0),
            registry=self.registry
        )
        
        # ========================================
        # LLM METRICS (BR-HAPI-301)
        # ========================================
        
        self.llm_calls_total = Counter(
            METRIC_NAME_LLM_CALLS_TOTAL,
            'Total LLM API calls by provider, model, and outcome',
            ['provider', 'model', 'status'],
            registry=self.registry
        )
        
        self.llm_call_duration = Histogram(
            METRIC_NAME_LLM_CALL_DURATION,
            'LLM API call latency distribution',
            ['provider', 'model'],
            buckets=(0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0),
            registry=self.registry
        )
        
        self.llm_token_usage = Counter(
            METRIC_NAME_LLM_TOKEN_USAGE,
            'Total tokens consumed by LLM calls',
            ['provider', 'model', 'type'],  # type: prompt | completion
            registry=self.registry
        )
        
        # ========================================
        # HTTP METRICS (BR-HAPI-302, DD-005)
        # ========================================
        
        # Note: HTTP metrics remain in middleware (FastAPI best practice)
        # These are exposed via middleware, not business logic
        # See: src/middleware/metrics.py for HTTP metric definitions
        
        # ========================================
        # CONFIG HOT-RELOAD METRICS (BR-HAPI-303)
        # ========================================
        
        # Note: Config metrics are in config manager
        # See: src/config/manager.py for config reload metric definitions
    
    def record_investigation_start(self) -> float:
        """
        Record investigation start (for timing).
        
        Returns:
            Start timestamp (use with record_investigation_complete)
        """
        return time.time()
    
    def record_investigation_complete(self, start_time: float, status: str):
        """
        Record investigation completion with metrics.
        
        Business Requirement: BR-HAPI-011
        
        Args:
            start_time: Start timestamp from record_investigation_start()
            status: Investigation outcome (success | error | needs_review)
        """
        import logging
        logger = logging.getLogger(__name__)
        
        duration = time.time() - start_time
        
        logger.info(f"🔍 METRICS DEBUG: Recording investigation_complete - status={status}, duration={duration:.2f}s")
        logger.info(f"🔍 METRICS DEBUG: Counter before inc: {self.investigations_total}")
        
        self.investigations_total.labels(status=status).inc()
        self.investigations_duration.observe(duration)
        
        logger.info(f"🔍 METRICS DEBUG: Metrics recorded successfully")
        logger.info(f"🔍 METRICS DEBUG: Registry: {self.registry}")
    
    def record_llm_call(
        self,
        provider: str,
        model: str,
        status: str,
        duration: float,
        prompt_tokens: int = 0,
        completion_tokens: int = 0
    ):
        """
        Record LLM call metrics (calls, latency, token usage).
        
        Business Requirement: BR-HAPI-301
        
        Args:
            provider: LLM provider (openai | anthropic | ollama)
            model: Model name (gpt-4 | claude-3 | ...)
            status: Call outcome (success | error | timeout)
            duration: Call duration in seconds
            prompt_tokens: Number of tokens in prompt
            completion_tokens: Number of tokens in completion
        """
        # Record call count and status
        self.llm_calls_total.labels(
            provider=provider,
            model=model,
            status=status
        ).inc()
        
        # Record latency
        self.llm_call_duration.labels(
            provider=provider,
            model=model
        ).observe(duration)
        
        # Record token usage (for cost tracking)
        if prompt_tokens > 0:
            self.llm_token_usage.labels(
                provider=provider,
                model=model,
                type=LABEL_TOKEN_TYPE_PROMPT
            ).inc(prompt_tokens)
        
        if completion_tokens > 0:
            self.llm_token_usage.labels(
                provider=provider,
                model=model,
                type=LABEL_TOKEN_TYPE_COMPLETION
            ).inc(completion_tokens)


# ========================================
# GLOBAL METRICS INSTANCE (Production Mode)
# ========================================

# Module-level singleton for production use (like Go's global metrics)
# Integration tests create their own HAMetrics instance with custom registry
_global_metrics: Optional[HAMetrics] = None


def get_global_metrics() -> HAMetrics:
    """
    Get or create the global HAMetrics instance.
    
    Production code uses this to access the default metrics.
    Integration tests create their own HAMetrics instances.
    
    Returns:
        Global HAMetrics instance (uses default REGISTRY)
    """
    global _global_metrics
    if _global_metrics is None:
        _global_metrics = HAMetrics()
    return _global_metrics
