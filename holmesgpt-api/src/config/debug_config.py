# Copyright 2025 Jordi Gil.
# SPDX-License-Identifier: Apache-2.0

"""
Centralized debug configuration for holmesgpt-api.

This module provides a single place to configure debug logging levels
for various components including LiteLLM, HolmesGPT SDK, and custom toolset injection.

Business Requirement: BR-HAPI-250 - Workflow Catalog Toolset
Design Decision: DD-DEBUG-001 - Centralized Debug Configuration

Usage:
    from src.config.debug_config import get_debug_config, apply_debug_config

    # Get debug settings
    debug_cfg = get_debug_config(app_config)

    # Apply debug settings (call once at startup)
    apply_debug_config(debug_cfg)
"""

import os
import logging
from typing import Optional

from src.models.config_models import AppConfig

logger = logging.getLogger(__name__)


class DebugConfig:
    """
    Centralized debug configuration.

    Attributes:
        enabled: Master debug flag (enables all debug features)
        litellm: Enable LiteLLM DEBUG logging (shows actual API requests)
        toolset_injection: Enable detailed toolset injection logging
    """

    def __init__(
        self,
        enabled: bool = False,
        litellm: bool = False,
        toolset_injection: bool = False
    ):
        self.enabled = enabled
        self.litellm = litellm or enabled  # If master enabled, enable litellm
        self.toolset_injection = toolset_injection or enabled  # If master enabled, enable injection logs

    def __repr__(self) -> str:
        return (
            f"DebugConfig(enabled={self.enabled}, "
            f"litellm={self.litellm}, "
            f"toolset_injection={self.toolset_injection})"
        )


def get_debug_config(app_config: Optional[AppConfig] = None) -> DebugConfig:
    """
    Get debug configuration from app config and environment variables.

    Priority (highest to lowest):
    1. Environment variables (DEBUG_MODE, DEBUG_LITELLM, DEBUG_TOOLSET_INJECTION)
    2. App configuration file (debug.enabled, debug.litellm, debug.toolset_injection)
    3. Defaults (all false)

    Args:
        app_config: Application configuration TypedDict

    Returns:
        DebugConfig instance with resolved settings

    Examples:
        >>> # From environment variable
        >>> os.environ['DEBUG_MODE'] = 'true'
        >>> cfg = get_debug_config()
        >>> cfg.enabled
        True

        >>> # From config file
        >>> app_config: AppConfig = {'debug': {'enabled': True, 'litellm': True}}
        >>> cfg = get_debug_config(app_config)
        >>> cfg.litellm
        True
    """
    # Get from environment variables (highest priority)
    env_enabled = os.getenv("DEBUG_MODE", "").lower() == "true"
    env_litellm = os.getenv("DEBUG_LITELLM", "").lower() == "true"
    env_toolset = os.getenv("DEBUG_TOOLSET_INJECTION", "").lower() == "true"

    # Get from app config
    debug_config = app_config.get("debug", {}) if app_config else {}
    config_enabled = debug_config.get("enabled", False)
    config_litellm = debug_config.get("litellm", False)
    config_toolset = debug_config.get("toolset_injection", False)

    # Merge (env overrides config)
    enabled = env_enabled or config_enabled
    litellm = env_litellm or config_litellm
    toolset_injection = env_toolset or config_toolset

    cfg = DebugConfig(
        enabled=enabled,
        litellm=litellm,
        toolset_injection=toolset_injection
    )

    if cfg.enabled or cfg.litellm or cfg.toolset_injection:
        logger.info(f"Debug configuration: {cfg}")

    return cfg


def apply_debug_config(debug_cfg: DebugConfig) -> None:
    """
    Apply debug configuration to the environment and logging system.

    This function should be called once at application startup to:
    1. Set LiteLLM logging level
    2. Configure Python logging levels
    3. Set any required environment variables

    Args:
        debug_cfg: DebugConfig instance to apply

    Side Effects:
        - Modifies environment variables (LITELLM_LOG, LITELLM_DROP_PARAMS)
        - Adjusts Python logging levels
    """
    if debug_cfg.litellm:
        # Enable LiteLLM DEBUG logging to see actual API requests
        os.environ["LITELLM_LOG"] = "DEBUG"
        os.environ["LITELLM_DROP_PARAMS"] = "false"
        logger.info("Enabled LiteLLM DEBUG logging (shows actual API requests to LLM)")
    else:
        # Ensure LiteLLM is at INFO level
        os.environ["LITELLM_LOG"] = os.getenv("LITELLM_LOG", "INFO")

    if debug_cfg.enabled:
        # Set Python logging to DEBUG for our modules
        logging.getLogger("src.extensions.llm_config").setLevel(logging.DEBUG)
        logging.getLogger("src.toolsets.workflow_catalog").setLevel(logging.DEBUG)
        logger.info("Enabled DEBUG logging for holmesgpt-api modules")

    if debug_cfg.toolset_injection:
        # Enable detailed toolset injection logging
        logging.getLogger("src.extensions.llm_config").setLevel(logging.DEBUG)
        logger.info("Enabled detailed toolset injection logging")


def should_log_toolset_injection(debug_cfg: Optional[DebugConfig] = None) -> bool:
    """
    Check if toolset injection logging should be enabled.

    Args:
        debug_cfg: Optional DebugConfig instance. If None, reads from environment.

    Returns:
        True if toolset injection logging should be enabled
    """
    if debug_cfg:
        return debug_cfg.toolset_injection

    # Fallback: check environment
    return os.getenv("DEBUG_TOOLSET_INJECTION", "").lower() == "true" or \
           os.getenv("DEBUG_MODE", "").lower() == "true"

