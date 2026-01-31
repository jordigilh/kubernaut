# Copyright 2025 Jordi Gil.
# SPDX-License-Identifier: Apache-2.0

"""
Centralized logging configuration for holmesgpt-api.

This module provides a single place to configure logging levels for the entire application.

Usage:
    from src.config.logging_config import setup_logging

    # Setup logging (call once at startup)
    setup_logging(app_config)
"""

import os
import logging
from typing import Optional

from src.models.config_models import AppConfig

logger = logging.getLogger(__name__)


def get_log_level(app_config: Optional[AppConfig] = None) -> str:
    """
    Get log level from environment variable or config file.

    Priority (highest to lowest):
    1. Environment variable (LOG_LEVEL)
    2. App configuration file (log_level)
    3. Default (INFO)

    Args:
        app_config: Application configuration TypedDict

    Returns:
        Log level string (DEBUG, INFO, WARNING, ERROR)

    Examples:
        >>> os.environ['LOG_LEVEL'] = 'DEBUG'
        >>> get_log_level()
        'DEBUG'

        >>> app_config: AppConfig = {'log_level': 'WARNING'}
        >>> get_log_level(app_config)
        'WARNING'
    """
    # Get from environment variable (highest priority)
    env_level = os.getenv("LOG_LEVEL", "").upper()
    if env_level in ("DEBUG", "INFO", "WARNING", "ERROR"):
        return env_level

    # Get from app config
    if app_config and "log_level" in app_config:
        config_level = str(app_config["log_level"]).upper()
        if config_level in ("DEBUG", "INFO", "WARNING", "ERROR"):
            return config_level

    # Default
    return "INFO"


def setup_logging(app_config: Optional[AppConfig] = None) -> None:
    """
    Setup logging for the entire holmesgpt-api application.

    This function should be called once at application startup to:
    1. Set log level for all holmesgpt-api modules
    2. Configure LiteLLM logging (DEBUG level enables detailed API request logs)
    3. Apply log level to HolmesGPT SDK modules

    Args:
        app_config: Application configuration TypedDict

    Side Effects:
        - Modifies environment variables (LITELLM_LOG, LITELLM_DROP_PARAMS)
        - Adjusts Python logging levels for all modules

    Examples:
        >>> # In main.py
        >>> config = load_config()
        >>> setup_logging(config)
    """
    log_level = get_log_level(app_config)
    log_level_int = getattr(logging, log_level)

    # Configure holmesgpt-api modules
    holmesgpt_modules = [
        "src.extensions.llm_config",
        "src.extensions.incident",
        "src.extensions.recovery",
        "src.toolsets.workflow_catalog",
        "src.config",
        "src.auth",        # Authentication/authorization (DD-AUTH-014)
        "src.middleware",  # Auth middleware
    ]

    for module in holmesgpt_modules:
        logging.getLogger(module).setLevel(log_level_int)
    
    # Set root logger level to ensure all logs are visible
    logging.getLogger().setLevel(log_level_int)

    # Configure LiteLLM logging
    if log_level == "DEBUG":
        # DEBUG: Show actual API requests to LLM
        os.environ["LITELLM_LOG"] = "DEBUG"
        os.environ["LITELLM_DROP_PARAMS"] = "false"
        logger.info("Log level set to DEBUG - detailed LLM interactions and function schemas will be logged")
    else:
        # INFO/WARNING/ERROR: Standard LiteLLM logging
        os.environ["LITELLM_LOG"] = log_level
        logger.info(f"Log level set to {log_level}")


def is_debug_enabled() -> bool:
    """
    Check if DEBUG logging is enabled.

    Useful for gating expensive debug operations.

    Returns:
        True if current log level is DEBUG

    Examples:
        >>> if is_debug_enabled():
        >>>     logger.debug(f"Expensive debug info: {compute_expensive_data()}")
    """
    return os.getenv("LOG_LEVEL", "INFO").upper() == "DEBUG" or \
           logging.getLogger("src.extensions.llm_config").isEnabledFor(logging.DEBUG)

