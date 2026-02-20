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
LLM Configuration Utilities

Business Requirement: BR-HAPI-001 - Multi-provider LLM support
Business Requirement: BR-HAPI-211 - LLM Input Sanitization
Provides shared utilities for configuring LLM providers across different endpoints.

Supported Providers:
- Ollama (local, OpenAI-compatible endpoint)
- OpenAI (remote, official API)
- Claude/Anthropic (remote, official API)
- Vertex AI (remote, Google Cloud)

Security Features:
- BR-HAPI-211: Tool result sanitization (credentials redacted before LLM sees them)
"""

import os
import logging
import tempfile
import yaml
from pathlib import Path
from typing import Dict, Any, Optional, List

from holmes.config import Config
from holmes.core.toolset_manager import ToolsetManager

# BR-HAPI-211: Import sanitization module for credential protection
from src.sanitization import sanitize_for_llm

# Import DetectedLabels for type hints
from src.models.incident_models import DetectedLabels

logger = logging.getLogger(__name__)


def _wrap_tool_results_with_sanitization(tool_executor) -> None:
    """
    Wrap ALL Tool.invoke() methods with credential sanitization.

    Business Requirement: BR-HAPI-211 - LLM Input Sanitization
    Design Decision: DD-HAPI-005 - Comprehensive LLM Input Sanitization Layer

    This function monkey-patches every tool in every toolset to sanitize
    the StructuredToolResult.data and StructuredToolResult.error fields
    before they're returned to the LLM.

    CRITICAL: This is the primary defense against credential leakage.
    Tool results (especially kubectl logs, kubectl get) may contain:
    - Database passwords in application logs
    - API keys in environment variables
    - Tokens in error messages
    - Secrets in ConfigMaps/events

    NOTE: Some HolmesGPT SDK tools (e.g., YAMLTool) don't have an invoke() method.
    We skip those gracefully and log a debug message.

    Args:
        tool_executor: HolmesGPT SDK tool executor with .toolsets list
    """
    sanitized_tools_count = 0
    skipped_tools_count = 0

    for toolset in tool_executor.toolsets:
        toolset_name = getattr(toolset, 'name', 'unknown')

        for tool in toolset.tools:
            tool_name = getattr(tool, 'name', 'unknown')

            # Check if tool has invoke method (some SDK tools like YAMLTool don't)
            if not hasattr(tool, 'invoke') or not callable(getattr(tool, 'invoke', None)):
                logger.debug({
                    "event": "tool_sanitization_skipped",
                    "br": "BR-HAPI-211",
                    "toolset": toolset_name,
                    "tool": tool_name,
                    "reason": "no_invoke_method",
                    "tool_type": type(tool).__name__,
                })
                skipped_tools_count += 1
                continue

            original_invoke = tool.invoke

            def make_sanitized_invoke(orig_invoke, t_name, ts_name):
                """Factory to capture original_invoke and tool names in closure."""
                def sanitized_invoke(params, tool_number=None, user_approved=False):
                    # Call original tool
                    result = orig_invoke(params, tool_number, user_approved)

                    # BR-HAPI-211: Sanitize data field (handles str, dict, list, None)
                    if result is not None and hasattr(result, 'data') and result.data is not None:
                        original_data = result.data
                        result.data = sanitize_for_llm(result.data)

                        # Log if sanitization modified content (for audit/debugging)
                        if result.data != original_data:
                            logger.debug({
                                "event": "tool_result_sanitized",
                                "br": "BR-HAPI-211",
                                "toolset": ts_name,
                                "tool": t_name,
                                "data_type": type(original_data).__name__,
                            })

                    # BR-HAPI-211: Sanitize error message if present
                    if result is not None and hasattr(result, 'error') and result.error:
                        original_error = result.error
                        result.error = sanitize_for_llm(result.error)

                        if result.error != original_error:
                            logger.debug({
                                "event": "tool_error_sanitized",
                                "br": "BR-HAPI-211",
                                "toolset": ts_name,
                                "tool": t_name,
                            })

                    return result
                return sanitized_invoke

            # Apply the sanitized wrapper
            # NOTE: Pydantic models may be frozen, so we use object.__setattr__
            try:
                object.__setattr__(tool, 'invoke', make_sanitized_invoke(original_invoke, tool_name, toolset_name))
                sanitized_tools_count += 1
            except (TypeError, ValueError, AttributeError) as e:
                # Some tool implementations may not allow attribute setting
                logger.debug({
                    "event": "tool_sanitization_skipped",
                    "br": "BR-HAPI-211",
                    "toolset": toolset_name,
                    "tool": tool_name,
                    "reason": "cannot_set_invoke",
                    "error": str(e),
                })
                skipped_tools_count += 1

    logger.info({
        "event": "tool_sanitization_wrapped",
        "br": "BR-HAPI-211",
        "tools_wrapped": sanitized_tools_count,
        "tools_skipped": skipped_tools_count,
        "toolsets_count": len(tool_executor.toolsets),
    })


def format_model_name_for_litellm(
    model_name: str,
    provider: str,
    llm_endpoint: Optional[str] = None
) -> str:
    """
    Format model name for litellm compatibility based on provider.

    LiteLLM expects different model name formats for different providers:
    - OpenAI: "model-name" (official API) or "openai/model-name" (custom endpoints like Ollama)
    - Vertex AI: "vertex_ai/model-name"
    - Anthropic/Claude: "model-name" (litellm handles the prefix internally)
    - Others: "model-name" (as-is)

    Args:
        model_name: Base model name (e.g., "claude-3-5-sonnet@20240620", "gpt-4", "llama2")
        provider: LLM provider (e.g., "openai", "vertex_ai", "anthropic", "claude")
        llm_endpoint: Optional custom endpoint URL (e.g., "http://localhost:8081/v1" for Ollama)

    Returns:
        Formatted model name for litellm

    Examples:
        >>> format_model_name_for_litellm("llama2", "openai", "http://localhost:8081/v1")
        "openai/llama2"  # Ollama via OpenAI-compatible endpoint

        >>> format_model_name_for_litellm("gpt-4", "openai", None)
        "gpt-4"  # Official OpenAI API

        >>> format_model_name_for_litellm("claude-3-5-sonnet@20240620", "vertex_ai", None)
        "vertex_ai/claude-3-5-sonnet@20240620"  # Vertex AI

        >>> format_model_name_for_litellm("claude-3-5-sonnet-20241022", "anthropic", None)
        "claude-3-5-sonnet-20241022"  # Anthropic API (litellm handles prefix)
    """
    # Already has correct prefix, return as-is
    if "/" in model_name and model_name.split("/")[0] in ["openai", "vertex_ai", "anthropic"]:
        return model_name

    # Vertex AI always needs prefix
    if provider == "vertex_ai":
        return f"vertex_ai/{model_name}"

    # OpenAI provider with custom endpoint (e.g., Ollama, LM Studio, LocalAI, Mock LLM)
    # These are OpenAI-compatible endpoints that need the "openai/" prefix
    if provider == "openai" and llm_endpoint:
        # Check if it's NOT the official OpenAI endpoint
        official_openai_endpoints = [
            "https://api.openai.com",
            "api.openai.com"
        ]
        is_custom_endpoint = not any(official in llm_endpoint for official in official_openai_endpoints)

        if is_custom_endpoint:
            return f"openai/{model_name}"

    # All other cases: use model name as-is
    # - Official OpenAI API (provider="openai", no custom endpoint)
    # - Anthropic/Claude (litellm handles the routing)
    # - Other providers
    return model_name


def get_model_config_for_sdk(app_config: Optional[Dict[str, Any]] = None) -> tuple[str, str]:
    """
    Get formatted model name and provider for HolmesGPT SDK.

    Resolution order:
    1. LLM_MODEL environment variable (takes precedence)
    2. config.llm.model (from YAML config file)
    3. Raise error if neither is set

    Args:
        app_config: Application configuration dictionary (loaded from YAML)

    Returns:
        Tuple of (formatted_model_name, provider)

    Raises:
        ValueError: If no model name is configured

    Example:
        >>> # With Ollama
        >>> os.environ["LLM_MODEL"] = "llama2"
        >>> os.environ["LLM_ENDPOINT"] = "http://localhost:8081/v1"
        >>> config = {"llm": {"provider": "openai", "model": "llama2"}}
        >>> get_model_config_for_sdk(config)
        ("openai/llama2", "openai")

        >>> # With Vertex AI
        >>> os.environ["LLM_MODEL"] = "claude-3-5-sonnet@20240620"
        >>> config = {"llm": {"provider": "vertex_ai"}}
        >>> get_model_config_for_sdk(config)
        ("vertex_ai/claude-3-5-sonnet@20240620", "vertex_ai")
    """
    # Get model name from env or config
    model_name = os.getenv("LLM_MODEL") or (app_config.get("llm", {}).get("model") if app_config else None)

    if not model_name:
        raise ValueError("LLM_MODEL environment variable or config.llm.model is required")

    # Get provider from config (defaults to openai for backwards compatibility)
    provider = app_config.get("llm", {}).get("provider", "openai") if app_config else "openai"

    # Get custom endpoint if set
    llm_endpoint = os.getenv("LLM_ENDPOINT") or (app_config.get("llm", {}).get("endpoint") if app_config else None)

    # Format model name for litellm
    formatted_model = format_model_name_for_litellm(model_name, provider, llm_endpoint)

    logger.info(
        f"LLM configuration: provider={provider}, "
        f"model={model_name} -> {formatted_model}, "
        f"endpoint={llm_endpoint or 'default'}"
    )

    return formatted_model, provider


def inject_detected_labels(
    result: Dict[str, Any],
    session_state: Optional[Dict[str, Any]],
) -> None:
    """
    Inject detected_labels from session_state into the response result dict.

    ADR-056: After LLM investigation completes, the detected_labels computed
    on-demand by WorkflowDiscoveryToolset are included in the HAPI response
    so AIAnalysis can store them in PostRCAContext for Rego policy evaluation.

    Args:
        result: Mutable response dict (IncidentResponse or RecoveryResponse format)
        session_state: Shared session state dict, or None
    """
    if session_state and "detected_labels" in session_state:
        result["detected_labels"] = session_state["detected_labels"]


def register_workflow_discovery_toolset(
    config: Config,
    app_config: Optional[Dict[str, Any]] = None,
    remediation_id: Optional[str] = None,
    custom_labels: Optional[Dict[str, List[str]]] = None,
    detected_labels: Optional[DetectedLabels] = None,
    severity: str = "",
    component: str = "",
    environment: str = "",
    priority: str = "",
    session_state: Optional[Dict[str, Any]] = None,
) -> Config:
    """
    Register the three-step workflow discovery toolset with HolmesGPT SDK Config.

    Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
    Business Requirement: BR-HAPI-017-001 (Three-Step Tool Implementation)
    Business Requirement: BR-HAPI-017-005 (remediationId Propagation)

    The three-step discovery protocol provides:
      1. list_available_actions -- Discover action types
      2. list_workflows -- List workflows for an action type
      3. get_workflow -- Get full workflow with parameter schema

    Signal context filters (severity, component, environment, priority) are
    set once at toolset creation and propagated to all three tools as query
    parameters for the DS security gate (DD-HAPI-017).

    ADR-056 v1.4: Labels are now detected by get_resource_context and stored
    in session_state. Workflow discovery reads them from session_state.

    Args:
        config: HolmesGPT SDK Config instance (already initialized)
        app_config: Optional application configuration (for logging context)
        remediation_id: Remediation request ID for audit correlation (DD-WORKFLOW-002 v2.2)
        custom_labels: Custom labels for auto-append (DD-HAPI-001)
        detected_labels: Auto-detected infrastructure labels (DD-HAPI-017, DD-WORKFLOW-001 v2.1)
        severity: Signal severity (critical/high/medium/low)
        component: K8s resource kind (pod/deployment/node/etc.)
        environment: Namespace-derived environment (production/staging/development)
        priority: Severity-mapped priority (P0/P1/P2/P3)
        session_state: Shared mutable dict for inter-tool communication (ADR-056 v1.4).
            Labels are populated by get_resource_context and consumed by discovery tools.

    Returns:
        The same Config instance with workflow discovery registered via monkey-patch
    """
    from src.toolsets.workflow_discovery import WorkflowDiscoveryToolset
    from src.clients.datastorage_auth_session import create_workflow_discovery_session

    # DD-AUTH-005: Create authenticated requests.Session for workflow discovery.
    # This injects the ServiceAccount token so all three discovery tools
    # authenticate with DataStorage (fixes 401 Unauthorized on /api/v1/workflows/*).
    http_session = create_workflow_discovery_session()

    discovery_toolset = WorkflowDiscoveryToolset(
        enabled=True,
        remediation_id=remediation_id,
        severity=severity,
        component=component,
        environment=environment,
        priority=priority,
        custom_labels=custom_labels,
        detected_labels=detected_labels,
        http_session=http_session,
        session_state=session_state,
    )

    # Initialize toolset manager if needed
    if not hasattr(config, 'toolset_manager') or config.toolset_manager is None:
        config.toolset_manager = ToolsetManager(toolsets=config.toolsets)

    logger.info(
        f"DD-HAPI-017: WorkflowDiscoveryToolset created "
        f"(enabled={discovery_toolset.enabled}, tools={len(discovery_toolset.tools)}). "
        f"Severity={severity}, component={component}, env={environment}, priority={priority}. "
        f"Will inject via monkey-patched methods."
    )

    # DD-HAPI-017: Monkey-patch list_server_toolsets to inject discovery toolset
    original_list_server_toolsets = config.toolset_manager.list_server_toolsets
    discovery_injected = [False]

    def patched_list_server_toolsets(dal=None, refresh_status=True):
        """Wrapper that injects workflow discovery toolset into the list"""
        toolsets = original_list_server_toolsets(dal=dal, refresh_status=refresh_status)

        if not discovery_injected[0]:
            has_discovery = any(
                hasattr(t, 'name') and t.name == "workflow/discovery"
                for t in toolsets
            )
            if not has_discovery:
                toolsets.append(discovery_toolset)
                discovery_injected[0] = True
                logger.info(
                    f"DD-HAPI-017: Injected workflow/discovery toolset into SDK "
                    f"(total toolsets: {len(toolsets)}, tools: {len(discovery_toolset.tools)})"
                )

        return toolsets

    config.toolset_manager.list_server_toolsets = patched_list_server_toolsets

    # DD-HAPI-017: Also patch create_tool_executor() (same pattern as catalog)
    import types

    original_create_tool_executor = config.create_tool_executor
    discovery_injected_executor = [False]

    def patched_create_tool_executor(self, dal=None):
        """Wrapper that injects workflow discovery into tool executor's toolsets list"""
        tool_executor = original_create_tool_executor(dal=dal)

        if not discovery_injected_executor[0]:
            has_discovery = any(
                hasattr(ts, 'name') and ts.name == "workflow/discovery"
                for ts in tool_executor.toolsets
            )
            if not has_discovery:
                tool_executor.toolsets.append(discovery_toolset)
                discovery_injected_executor[0] = True
                logger.info(
                    f"DD-HAPI-017: Injected workflow/discovery into tool_executor.toolsets "
                    f"(total: {len(tool_executor.toolsets)}). LLM WILL NOW SEE THREE-STEP TOOLS."
                )

        # BR-HAPI-211: Wrap ALL tool invocations with credential sanitization
        _wrap_tool_results_with_sanitization(tool_executor)

        return tool_executor

    object.__setattr__(config, 'create_tool_executor', types.MethodType(patched_create_tool_executor, config))

    logger.info(
        "DD-HAPI-017: Monkey-patched list_server_toolsets() AND create_tool_executor() "
        "to inject workflow discovery (three-step protocol) at LLM-visible layer"
    )

    return config


def register_resource_context_toolset(
    config: Config,
    app_config: Optional[Dict[str, Any]] = None,
    session_state: Optional[Dict[str, Any]] = None,
) -> Config:
    """
    Register the resource context toolset with HolmesGPT SDK Config.

    ADR-055: LLM-Driven Context Enrichment (Post-RCA)
    ADR-056 v1.4: DetectedLabels computation for the RCA target resource.

    After RCA, the LLM calls get_resource_context to fetch owner chain,
    spec hash, remediation history, and infrastructure labels for the
    identified target resource.

    Args:
        config: HolmesGPT SDK Config instance (already initialized)
        app_config: Optional application configuration
        session_state: Shared mutable dict for inter-tool communication (ADR-056 v1.4).
            Labels are detected once and stored here for downstream workflow discovery tools.

    Returns:
        The same Config instance with resource context toolset registered
    """
    from src.toolsets.resource_context import ResourceContextToolset
    from src.clients.k8s_client import get_k8s_client

    try:
        k8s = get_k8s_client()
    except Exception as e:
        logger.warning({"event": "resource_context_k8s_unavailable", "error": str(e)})
        logger.info("ADR-055: Resource context toolset not registered (K8s client unavailable)")
        return config

    # History fetcher wraps the remediation history client
    history_fetcher = None
    try:
        from src.clients.remediation_history_client import (
            create_remediation_history_api,
            fetch_remediation_history_for_request,
        )
        rh_api = create_remediation_history_api(app_config)

        def _fetch_history(resource_kind, resource_name, resource_namespace, current_spec_hash):
            request_data = {
                "resource_kind": resource_kind,
                "resource_name": resource_name,
                "resource_namespace": resource_namespace,
            }
            return fetch_remediation_history_for_request(
                api=rh_api,
                request_data=request_data,
                current_spec_hash=current_spec_hash,
            )
        history_fetcher = _fetch_history
    except (ImportError, Exception) as e:
        logger.warning({"event": "resource_context_history_unavailable", "error": str(e)})

    try:
        context_toolset = ResourceContextToolset(
            k8s_client=k8s,
            history_fetcher=history_fetcher,
            session_state=session_state,
        )
    except Exception as e:
        logger.warning({"event": "resource_context_toolset_creation_failed", "error": str(e)})
        logger.info("ADR-055: Resource context toolset not registered (creation failed)")
        return config

    # Initialize toolset manager if needed
    if not hasattr(config, 'toolset_manager') or config.toolset_manager is None:
        from holmes.core.tool_calling_llm import ToolsetManager
        config.toolset_manager = ToolsetManager(toolsets=config.toolsets)

    # Monkey-patch to inject resource context toolset (same pattern as workflow discovery)
    original_list = config.toolset_manager.list_server_toolsets
    context_injected = [False]

    def patched_list(dal=None, refresh_status=True):
        toolsets = original_list(dal=dal, refresh_status=refresh_status)
        if not context_injected[0]:
            has_context = any(
                hasattr(t, 'name') and t.name == "resource_context"
                for t in toolsets
            )
            if not has_context:
                toolsets.append(context_toolset)
                context_injected[0] = True
                logger.info("ADR-055: Injected resource_context toolset into SDK")
        return toolsets

    config.toolset_manager.list_server_toolsets = patched_list

    import types
    original_executor = config.create_tool_executor
    context_injected_executor = [False]

    def patched_executor(self, dal=None):
        tool_executor = original_executor(dal=dal)
        if not context_injected_executor[0]:
            has_context = any(
                hasattr(ts, 'name') and ts.name == "resource_context"
                for ts in tool_executor.toolsets
            )
            if not has_context:
                tool_executor.toolsets.append(context_toolset)
                context_injected_executor[0] = True
                logger.info("ADR-055: Injected resource_context into tool_executor")
        _wrap_tool_results_with_sanitization(tool_executor)
        return tool_executor

    object.__setattr__(config, 'create_tool_executor', types.MethodType(patched_executor, config))

    logger.info("ADR-055: Registered resource_context toolset for post-RCA context enrichment")

    return config


def prepare_toolsets_config_for_sdk(
    app_config: Optional[Dict[str, Any]] = None
) -> Dict[str, Dict[str, Any]]:
    """
    Prepare toolsets configuration for HolmesGPT SDK Config.

    Business Requirements:
    - BR-HAPI-002: Enable Kubernetes toolsets by default
    - BR-HAPI-250: Workflow catalog must be added programmatically (not as dict)

    This function:
    1. Sets default enabled state for core Kubernetes toolsets
    2. Merges with user-provided toolset configuration
    3. REMOVES workflow/catalog if present (will be added as Toolset instance)

    Args:
        app_config: Optional application configuration with toolsets section

    Returns:
        Dict of toolset configurations ready for HolmesGPT SDK Config

    Example:
        >>> config = {"toolsets": {"kubernetes/core": {"enabled": False}, "workflow/catalog": {"enabled": True}}}
        >>> toolsets = prepare_toolsets_config_for_sdk(config)
        >>> "kubernetes/core" in toolsets
        True
        >>> "workflow/catalog" in toolsets  # Removed - will be added programmatically
        False
    """
    # Get toolsets config from app_config
    toolsets_config = app_config.get("toolsets", {}).copy() if app_config else {}

    # BR-HAPI-002: Set default enabled state for core toolsets if not explicitly configured
    default_toolsets = {
        "kubernetes/core": {"enabled": True},
        "kubernetes/logs": {"enabled": True},
        "kubernetes/live-metrics": {"enabled": True},
    }

    # Merge defaults with user config (user config takes precedence)
    for toolset_name, default_config in default_toolsets.items():
        if toolset_name not in toolsets_config:
            toolsets_config[toolset_name] = default_config
        elif "enabled" not in toolsets_config[toolset_name]:
            toolsets_config[toolset_name]["enabled"] = default_config["enabled"]

    # BR-HAPI-250 / DD-HAPI-017: Remove workflow toolsets from config if present
    # These are added programmatically as Toolset instances via registration functions
    for wf_toolset_name in ("workflow/catalog", "workflow/discovery"):
        if wf_toolset_name in toolsets_config:
            logger.info(
                f"Removing {wf_toolset_name} from toolsets config "
                "(will be registered programmatically as Toolset instance)"
            )
            del toolsets_config[wf_toolset_name]

    logger.info(f"Prepared toolsets config: {list(toolsets_config.keys())}")

    return toolsets_config


# Provider detection for configuration examples
PROVIDER_EXAMPLES = {
    "ollama": {
        "config": {
            "llm": {
                "provider": "openai",  # Ollama uses OpenAI-compatible API
                "model": "llama2",  # or any Ollama model
                "endpoint": "http://localhost:11434/v1"  # or via SSH tunnel
            }
        },
        "description": "Local Ollama instance (OpenAI-compatible)"
    },
    "openai": {
        "config": {
            "llm": {
                "provider": "openai",
                "model": "gpt-4",
                # No custom endpoint - uses official OpenAI API
            }
        },
        "description": "Official OpenAI API"
    },
    "anthropic": {
        "config": {
            "llm": {
                "provider": "anthropic",  # or "claude"
                "model": "claude-3-5-sonnet-20241022",
                # No custom endpoint - uses official Anthropic API
            }
        },
        "description": "Official Anthropic Claude API"
    },
    "vertex_ai": {
        "config": {
            "llm": {
                "provider": "vertex_ai",
                "model": "claude-3-5-sonnet@20240620",
                "project": "your-gcp-project",
                "location": "us-central1"
            }
        },
        "description": "Google Cloud Vertex AI"
    }
}

