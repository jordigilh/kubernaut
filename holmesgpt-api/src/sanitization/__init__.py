#
# Copyright 2025 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

"""
LLM Input Sanitization Module

Business Requirement: BR-HAPI-211 - LLM Input Sanitization
Design Decision: DD-HAPI-005 - Comprehensive LLM Input Sanitization Layer

This module sanitizes ALL data flowing to external LLM providers (OpenAI, Anthropic, etc.)
to prevent credential leakage. It mirrors patterns from pkg/shared/sanitization/sanitizer.go
for consistency across Go and Python services.

Usage:
    from src.sanitization import sanitize_for_llm

    # Sanitize string
    safe_prompt = sanitize_for_llm(user_prompt)

    # Sanitize dict (JSON)
    safe_data = sanitize_for_llm({"password": "secret123"})
    # Returns: {"password": "[REDACTED]"}
"""

from src.sanitization.llm_sanitizer import (
    sanitize_for_llm,
    sanitize_with_fallback,
    REDACTED_PLACEHOLDER,
    LLMSanitizer,
)

__all__ = [
    "sanitize_for_llm",
    "sanitize_with_fallback",
    "REDACTED_PLACEHOLDER",
    "LLMSanitizer",
]




