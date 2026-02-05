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


def register_workflow_catalog_toolset(
    config: Config,
    app_config: Optional[Dict[str, Any]] = None,
    remediation_id: Optional[str] = None,
    custom_labels: Optional[Dict[str, List[str]]] = None,
    detected_labels: Optional[DetectedLabels] = None,
    source_resource: Optional[Dict[str, str]] = None,
    owner_chain: Optional[List[Dict[str, str]]] = None
) -> Config:
    """
    Register the workflow catalog toolset with HolmesGPT SDK Config.

    Business Requirement: BR-HAPI-250 - Workflow Catalog Search Tool
    Business Requirement: BR-AUDIT-001 - Unified audit trail (remediation_id)
    Design Decision: DD-WORKFLOW-002 v2.2 - remediation_id mandatory
    Design Decision: DD-HAPI-001 - Custom Labels Auto-Append Architecture
    Design Decision: DD-WORKFLOW-001 v1.7 - DetectedLabels 100% safe validation

    CRITICAL: After extensive investigation, the HolmesGPT SDK does NOT support:
    1. Direct assignment of Toolset instances (causes '.get()' AttributeError during SDK's toolset loading)
    2. YAML files with Python class references (SDK's YAMLToolset expects tool definitions, not classes)
    3. Pre-loading custom toolsets before SDK initialization

    SOLUTION: Monkey-patch the ToolsetManager to inject our toolset AFTER SDK loads config toolsets
    but BEFORE they're used for investigation. This is done by wrapping list_server_toolsets().

    Args:
        config: HolmesGPT SDK Config instance (already initialized)
        app_config: Optional application configuration (for logging context)
        remediation_id: Remediation request ID for audit correlation (DD-WORKFLOW-002 v2.2)
                       MANDATORY per DD-WORKFLOW-002 v2.2. This ID is for CORRELATION/AUDIT ONLY -
                       do NOT use for RCA analysis or workflow matching.
        custom_labels: Custom labels for auto-append (DD-HAPI-001)
                      Format: map[string][]string (subdomain → list of values)
                      Example: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
                      Auto-appended to all MCP workflow search calls - invisible to LLM.
        detected_labels: Auto-detected labels for workflow matching (DD-WORKFLOW-001 v1.7)
                        Format: {"gitOpsManaged": true, "gitOpsTool": "argocd", ...}
                        Only included when relationship to RCA resource is PROVEN.
        source_resource: Original signal's resource for DetectedLabels validation
                        Format: {"namespace": "production", "kind": "Pod", "name": "api-xyz"}
                        Compared against LLM's rca_resource.
        owner_chain: K8s ownership chain from SignalProcessing enrichment
                    Format: [{"namespace": "prod", "kind": "ReplicaSet", "name": "..."}, ...]
                    Used for PROVEN relationship validation (100% safe).

    Returns:
        The same Config instance with workflow catalog registered via monkey-patch
    """
    from src.toolsets.workflow_catalog import WorkflowCatalogToolset

    # Create the workflow catalog toolset instance
    # BR-AUDIT-001: Pass remediation_id for audit correlation
    # DD-HAPI-001: Pass custom_labels for auto-append to workflow search
    # DD-WORKFLOW-001 v1.7: Pass detected_labels with source_resource and owner_chain for 100% safe validation
    workflow_toolset = WorkflowCatalogToolset(
        enabled=True,
        remediation_id=remediation_id,
        custom_labels=custom_labels,
        detected_labels=detected_labels,
        source_resource=source_resource,
        owner_chain=owner_chain
    )

    # Initialize toolset manager if needed
    if not hasattr(config, 'toolset_manager') or config.toolset_manager is None:
        config.toolset_manager = ToolsetManager(toolsets=config.toolsets)

    # BR-HAPI-250: DO NOT add toolset directly to toolsets dict!
    # The SDK's _load_toolsets_from_config() iterates over toolsets dict
    # and calls .get('type') on each value, expecting dicts not Toolset instances.
    # Instead, we inject the toolset through monkey-patched methods below.
    #
    # WRONG (causes AttributeError: 'WorkflowCatalogToolset' has no attribute 'get'):
    #   config.toolset_manager.toolsets["workflow/catalog"] = workflow_toolset
    #
    # CORRECT: Only inject through patched methods (see below)
    logger.info(
        f"BR-HAPI-250: WorkflowCatalogToolset created "
        f"(enabled={workflow_toolset.enabled}, tools={len(workflow_toolset.tools)}). "
        f"Will inject via monkey-patched methods."
    )

    # BR-HAPI-250: Monkey-patch list_server_toolsets to inject workflow catalog
    # This is the only reliable way to add custom Python toolsets to the SDK
    original_list_server_toolsets = config.toolset_manager.list_server_toolsets
    workflow_catalog_injected = [False]  # Mutable flag to track injection

    def patched_list_server_toolsets(dal=None, refresh_status=True):
        """Wrapper that injects workflow catalog toolset into the list"""
        # Get original toolsets from SDK
        toolsets = original_list_server_toolsets(dal=dal, refresh_status=refresh_status)

        # Inject workflow catalog toolset if not already present
        if not workflow_catalog_injected[0]:
            # Check if workflow catalog is already in the list
            has_workflow_catalog = any(t.name == "workflow/catalog" for t in toolsets if hasattr(t, 'name'))

            if not has_workflow_catalog:
                toolsets.append(workflow_toolset)
                workflow_catalog_injected[0] = True
                logger.info(
                    f"BR-HAPI-250: Injected workflow/catalog toolset into SDK "
                    f"(total toolsets: {len(toolsets)}, tools: {len(workflow_toolset.tools)})"
                )

        return toolsets

    # Apply the monkey-patch
    config.toolset_manager.list_server_toolsets = patched_list_server_toolsets

    # BR-HAPI-250 CRITICAL FIX: Also patch create_tool_executor()
    # Discovery: list_server_toolsets() gets workflow catalog, but create_tool_executor() doesn't use it!
    # The tool executor is what the LLM actually sees, so we MUST inject there too.
    import types

    original_create_tool_executor = config.create_tool_executor
    workflow_catalog_injected_executor = [False]

    def patched_create_tool_executor(self, dal=None):
        """Wrapper that injects workflow catalog into tool executor's toolsets list"""
        tool_executor = original_create_tool_executor(dal=dal)

        # DEBUG: Log state BEFORE injection
        logger.debug(f"BR-HAPI-250 DEBUG: Tool executor BEFORE injection:")
        logger.debug(f"  - Total toolsets: {len(tool_executor.toolsets)}")
        logger.debug(f"  - Toolset names: {[ts.name if hasattr(ts, 'name') else 'unknown' for ts in tool_executor.toolsets]}")

        # Inject workflow catalog into tool executor's toolsets list
        if not workflow_catalog_injected_executor[0]:
            # tool_executor.toolsets is a list of Toolset instances
            has_workflow_catalog = any(
                hasattr(ts, 'name') and ts.name == "workflow/catalog"
                for ts in tool_executor.toolsets
            )

            logger.debug(f"  - workflow/catalog already present: {has_workflow_catalog}")

            if not has_workflow_catalog:
                # DEBUG: Log details BEFORE appending
                logger.debug(f"BR-HAPI-250 DEBUG: About to inject workflow/catalog:")
                logger.debug(f"  - Toolset type: {type(workflow_toolset).__name__}")
                logger.debug(f"  - Toolset name: {workflow_toolset.name}")
                logger.debug(f"  - Enabled: {workflow_toolset.enabled}")
                logger.debug(f"  - Is Default: {workflow_toolset.is_default}")
                logger.debug(f"  - Experimental: {workflow_toolset.experimental}")
                logger.debug(f"  - Tags: {workflow_toolset.tags}")
                logger.debug(f"  - Tools count: {len(workflow_toolset.tools)}")
                if workflow_toolset.tools:
                    logger.debug(f"  - Tool[0] name: {workflow_toolset.tools[0].name}")
                    logger.debug(f"  - Tool[0] type: {type(workflow_toolset.tools[0]).__name__}")
                    logger.debug(f"  - Tool[0] description: {workflow_toolset.tools[0].description[:100]}...")

                # Inject
                tool_executor.toolsets.append(workflow_toolset)
                workflow_catalog_injected_executor[0] = True

                # DEBUG: Log state AFTER injection
                logger.info(
                    f"BR-HAPI-250 CRITICAL: Injected workflow/catalog into tool_executor.toolsets "
                    f"(total: {len(tool_executor.toolsets)}). LLM WILL NOW SEE THIS TOOL."
                )
                logger.debug(f"BR-HAPI-250 DEBUG: Tool executor AFTER injection:")
                logger.debug(f"  - Total toolsets: {len(tool_executor.toolsets)}")
                logger.debug(f"  - Last 5 toolsets: {[ts.name if hasattr(ts, 'name') else 'unknown' for ts in tool_executor.toolsets[-5:]]}")

        # BR-HAPI-211: Wrap ALL tool invocations with credential sanitization
        # This prevents credentials from leaking to external LLM providers
        _wrap_tool_results_with_sanitization(tool_executor)

        return tool_executor

    # Bind the patched method to the config instance using types.MethodType
    object.__setattr__(config, 'create_tool_executor', types.MethodType(patched_create_tool_executor, config))

    logger.info(
        "BR-HAPI-250: Monkey-patched list_server_toolsets() AND create_tool_executor() "
        "to inject workflow catalog at LLM-visible layer"
    )

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

    # BR-HAPI-250: CRITICAL - Remove workflow/catalog from toolsets_config if present
    # We will add it programmatically as a Toolset instance via register_workflow_catalog_toolset()
    # This prevents the "dict vs Toolset instance" registration bug
    if "workflow/catalog" in toolsets_config:
        logger.info(
            "BR-HAPI-250: Removing workflow/catalog from toolsets config "
            "(will be registered programmatically as Toolset instance)"
        )
        del toolsets_config["workflow/catalog"]

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

