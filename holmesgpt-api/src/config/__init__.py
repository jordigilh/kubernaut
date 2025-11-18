# Copyright 2025 Jordi Gil.
# SPDX-License-Identifier: Apache-2.0

"""
Configuration module for holmesgpt-api.

Provides centralized configuration management including logging.
"""

from .logging_config import setup_logging, get_log_level, is_debug_enabled

__all__ = [
    "setup_logging",
    "get_log_level",
    "is_debug_enabled",
]

