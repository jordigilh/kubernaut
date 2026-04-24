"""
HAPI Metrics Module

This module provides metrics instrumentation following the Go service pattern.

Design Decision: DD-005 v3.0 Section 1.1 - Metric Name Constants (MANDATORY)
Business Requirements: BR-HAPI-011, BR-HAPI-301

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
from .litellm_callback import KubernautLiteLLMCallback
from .constants import (
    # Investigation Metrics (BR-HAPI-011)
    METRIC_NAME_INVESTIGATIONS_TOTAL,
    METRIC_NAME_INVESTIGATIONS_DURATION,
    
    # LLM Metrics (BR-HAPI-301)
    METRIC_NAME_LLM_CALLS_TOTAL,
    METRIC_NAME_LLM_CALL_DURATION,
    METRIC_NAME_LLM_TOKEN_USAGE,
    
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
    'KubernautLiteLLMCallback',
    
    # Metric name constants (Investigation)
    'METRIC_NAME_INVESTIGATIONS_TOTAL',
    'METRIC_NAME_INVESTIGATIONS_DURATION',
    
    # Metric name constants (LLM)
    'METRIC_NAME_LLM_CALLS_TOTAL',
    'METRIC_NAME_LLM_CALL_DURATION',
    'METRIC_NAME_LLM_TOKEN_USAGE',
    
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
