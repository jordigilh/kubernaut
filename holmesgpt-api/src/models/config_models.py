"""
Config TypedDict Models for HolmesGPT API Service.

BR-TECHNICAL-DEBT: Eliminate Dict[str, Any] for configuration
Phase 3: Config TypedDict replacement

This module provides structured type definitions for application configuration
to replace unstructured Dict[str, Any] usage.
"""

from typing import TypedDict, Optional, Dict, Any


class LLMConfig(TypedDict, total=False):
    """
    LLM provider configuration.

    Using total=False to allow partial configuration with defaults.
    """
    model: str
    provider: str
    endpoint: Optional[str]
    max_retries: int
    timeout_seconds: int
    temperature: float
    max_tokens_per_request: int


class ContextAPIConfig(TypedDict, total=False):
    """Context API configuration."""
    enabled: bool
    url: str
    timeout_seconds: int


class KubernetesConfig(TypedDict, total=False):
    """Kubernetes cluster configuration."""
    in_cluster: bool
    kubeconfig_path: Optional[str]


class MetricsConfig(TypedDict, total=False):
    """Metrics and observability configuration."""
    enabled: bool
    port: int
    path: str


class AuditConfig(TypedDict, total=False):
    """
    Audit configuration (BR-AUDIT-005, ADR-038).

    Controls buffered audit event persistence to Data Storage.
    """
    flush_interval_seconds: float
    buffer_size: int
    batch_size: int


class PromptConfig(TypedDict, total=False):
    """
    Prompt engineering configuration (Issue #224, BR-HAPI-016).

    Controls thresholds and behavior for LLM prompt enrichment.
    """
    repeated_remediation_escalation_threshold: int
    confidence_threshold_human_review: float  # BR-HAPI-198: configurable via config.yaml


class AppConfig(TypedDict, total=False):
    """
    Main application configuration structure.

    Using total=False to allow partial configuration with defaults.
    This replaces Dict[str, Any] for app_config parameters.

    Example:
        config: AppConfig = {
            "service_name": "holmesgpt-api",
            "version": "1.0.0",
            "log_level": "INFO",
            "llm": {
                "model": "gpt-4",
                "provider": "openai"
            },
            "audit": {
                "flush_interval_seconds": 5.0,
                "buffer_size": 10000,
                "batch_size": 50
            }
        }
    """
    service_name: str
    version: str
    log_level: str
    llm: LLMConfig
    toolsets: Dict[str, Any]  # Toolset configs vary by type
    mcp_servers: Dict[str, Any]  # MCP server configs vary by type
    context_api: ContextAPIConfig
    kubernetes: KubernetesConfig
    metrics: MetricsConfig
    audit: AuditConfig
    prompt: PromptConfig




