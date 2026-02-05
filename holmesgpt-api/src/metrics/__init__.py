"""
HAPI Metrics Module

This module provides metrics instrumentation following the Go service pattern.

Design Decision: DD-005 v3.0 Section 1.1 - Metric Name Constants (MANDATORY)
Business Requirements: BR-HAPI-011, BR-HAPI-301, BR-HAPI-302, BR-HAPI-303

Architecture:
- HAMetrics class: Injectable metrics for business logic (like Go's metrics.Metrics)
- Metric constants: Type-safe metric names (DD-005 v3.0 compliance)
- HTTP middleware metrics: Remain in middleware (FastAPI pattern)

Usage:
    # Production
    from src.metrics import get_global_metrics
    metrics = get_global_metrics()
    metrics.record_investigation_complete(start_time, 'success')
    
    # Integration tests
    from src.metrics import HAMetrics
    from prometheus_client import CollectorRegistry
    test_registry = CollectorRegistry()
    test_metrics = HAMetrics(registry=test_registry)
"""

from .instrumentation import HAMetrics, get_global_metrics
from .constants import (
    # Investigation Metrics (BR-HAPI-011)
    METRIC_NAME_INVESTIGATIONS_TOTAL,
    METRIC_NAME_INVESTIGATIONS_DURATION,
    
    # LLM Metrics (BR-HAPI-301)
    METRIC_NAME_LLM_CALLS_TOTAL,
    METRIC_NAME_LLM_CALL_DURATION,
    METRIC_NAME_LLM_TOKEN_USAGE,
    
    # HTTP Metrics (BR-HAPI-302, DD-005 Standard)
    METRIC_NAME_HTTP_REQUESTS_TOTAL,
    METRIC_NAME_HTTP_REQUEST_DURATION,
    
    # Config Hot-Reload Metrics (BR-HAPI-303)
    METRIC_NAME_CONFIG_RELOAD_TOTAL,
    METRIC_NAME_CONFIG_RELOAD_ERRORS,
    METRIC_NAME_CONFIG_LAST_RELOAD_TIMESTAMP,
    
    # RFC 7807 Error Metrics (BR-HAPI-200)
    METRIC_NAME_RFC7807_ERRORS_TOTAL,
    
    # Label value constants
    LABEL_STATUS_SUCCESS,
    LABEL_STATUS_ERROR,
    LABEL_STATUS_NEEDS_REVIEW,
    LABEL_STATUS_TIMEOUT,
    LABEL_PROVIDER_OPENAI,
    LABEL_PROVIDER_ANTHROPIC,
    LABEL_PROVIDER_OLLAMA,
    LABEL_TOKEN_TYPE_PROMPT,
    LABEL_TOKEN_TYPE_COMPLETION,
)

__all__ = [
    # Metrics instrumentation classes
    'HAMetrics',
    'get_global_metrics',
    
    # Metric name constants (Investigation)
    'METRIC_NAME_INVESTIGATIONS_TOTAL',
    'METRIC_NAME_INVESTIGATIONS_DURATION',
    
    # Metric name constants (LLM)
    'METRIC_NAME_LLM_CALLS_TOTAL',
    'METRIC_NAME_LLM_CALL_DURATION',
    'METRIC_NAME_LLM_TOKEN_USAGE',
    
    # Metric name constants (HTTP)
    'METRIC_NAME_HTTP_REQUESTS_TOTAL',
    'METRIC_NAME_HTTP_REQUEST_DURATION',
    
    # Metric name constants (Config)
    'METRIC_NAME_CONFIG_RELOAD_TOTAL',
    'METRIC_NAME_CONFIG_RELOAD_ERRORS',
    'METRIC_NAME_CONFIG_LAST_RELOAD_TIMESTAMP',
    
    # Metric name constants (RFC 7807)
    'METRIC_NAME_RFC7807_ERRORS_TOTAL',
    
    # Label value constants
    'LABEL_STATUS_SUCCESS',
    'LABEL_STATUS_ERROR',
    'LABEL_STATUS_NEEDS_REVIEW',
    'LABEL_STATUS_TIMEOUT',
    'LABEL_PROVIDER_OPENAI',
    'LABEL_PROVIDER_ANTHROPIC',
    'LABEL_PROVIDER_OLLAMA',
    'LABEL_TOKEN_TYPE_PROMPT',
    'LABEL_TOKEN_TYPE_COMPLETION',
]
