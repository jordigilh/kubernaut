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
LLM Input Sanitizer

Business Requirement: BR-HAPI-211 - LLM Input Sanitization
Design Decision: DD-HAPI-005 - Comprehensive LLM Input Sanitization Layer

Provides regex-based credential detection and redaction for ALL data
sent to external LLM providers. Patterns ported from pkg/shared/sanitization/sanitizer.go
for consistency with Go services (DD-005).

Pattern Categories (17 total):
- P0: Database URLs, Passwords (JSON/plain/URL), API Keys (OpenAI/generic),
      Bearer tokens, JWT tokens, GitHub tokens
- P1: AWS credentials, Private keys, K8s secrets, Base64 secrets, Authorization headers
- P2: Generic secrets

CRITICAL: Pattern ordering matters! Container patterns (generatorURL, annotations)
must be applied FIRST to prevent sub-patterns from corrupting larger structures.
"""

import re
import json
import logging
from typing import Any, Optional, Tuple, List
from dataclasses import dataclass

logger = logging.getLogger(__name__)

# Standard placeholder for redacted content (matches Go pkg/shared/sanitization)
REDACTED_PLACEHOLDER = "[REDACTED]"


@dataclass
class SanitizationRule:
    """A pattern-based sanitization rule."""
    name: str
    pattern: re.Pattern
    replacement: str
    description: str


class LLMSanitizer:
    """
    Configurable credential sanitizer for LLM input.

    BR-HAPI-211: Sanitize ALL data before sending to external LLM providers.
    DD-HAPI-005: Use patterns consistent with Go shared library.

    Example:
        sanitizer = LLMSanitizer()
        clean = sanitizer.sanitize('{"password":"secret123"}')
        # Returns: '{"password":"[REDACTED]"}'
    """

    def __init__(self, rules: Optional[List[SanitizationRule]] = None):
        """Initialize sanitizer with default or custom rules."""
        self.rules = rules if rules is not None else default_rules()
        self._compiled = True  # All patterns pre-compiled in default_rules()

    def sanitize(self, content: str) -> str:
        """
        Apply all sanitization rules to redact sensitive data.

        Args:
            content: String content to sanitize

        Returns:
            Sanitized content with credentials replaced by [REDACTED]
        """
        if not content:
            return content

        result = content
        for rule in self.rules:
            if rule.pattern.search(result):
                result = rule.pattern.sub(rule.replacement, result)
        return result

    def sanitize_with_fallback(self, content: str) -> Tuple[str, Optional[Exception]]:
        """
        Sanitize with automatic fallback on regex errors.

        Implements graceful degradation per DD-HAPI-005 FR-5:
        - When: Regex processing fails (e.g., catastrophic backtracking)
        - Action: Log error, use simple string matching fallback
        - Recovery: Automatic (degraded but safe)

        Args:
            content: String content to sanitize

        Returns:
            Tuple of (sanitized_content, error_if_fallback_used)
        """
        try:
            return self.sanitize(content), None
        except Exception as e:
            logger.warning({
                "event": "sanitization_regex_failed",
                "br": "BR-HAPI-211",
                "error": str(e),
                "fallback": "simple_string_matching"
            })
            return self.safe_fallback(content), e

    def safe_fallback(self, content: str) -> str:
        """
        Simple string-based sanitization without regex.

        Used when regex engine fails or for ultra-safe processing.
        Cannot panic, uses only simple string operations.

        Args:
            content: String content to sanitize

        Returns:
            Sanitized content using simple keyword matching
        """
        output = content

        # Common secret patterns (simple string matching, no regex)
        keywords = [
            "password:", "passwd:", "pwd:",
            "token:", "api_token:", "access_token:",
            "key:", "api_key:", "apikey:",
            "secret:", "client_secret:",
            "credential:", "credentials:",
            "authorization:", "bearer:",
        ]

        for keyword in keywords:
            output = self._redact_keyword_simple(output, keyword)

        return output

    def _redact_keyword_simple(self, content: str, keyword: str) -> str:
        """
        Redact values following a keyword using simple string ops.

        This is the fallback when regex is not safe to use.
        """
        output = content
        lower_output = output.lower()

        idx = lower_output.find(keyword)
        while idx != -1:
            # Find the value after the keyword
            value_start = idx + len(keyword)

            # Skip whitespace
            while value_start < len(output) and output[value_start] in ' \t':
                value_start += 1

            if value_start >= len(output):
                break

            # Find end of value
            value_end = value_start
            in_quotes = False
            quote_char = ''

            if output[value_start] in '"\'':
                in_quotes = True
                quote_char = output[value_start]
                value_start += 1
                value_end = value_start

            while value_end < len(output):
                ch = output[value_end]
                if in_quotes:
                    if ch == quote_char:
                        break
                else:
                    if ch in ' \t\n\r,"\'}]':
                        break
                value_end += 1

            if value_end > value_start:
                output = output[:value_start] + REDACTED_PLACEHOLDER + output[value_end:]
                lower_output = output.lower()

            # Search for next occurrence
            search_start = idx + len(keyword) + len(REDACTED_PLACEHOLDER)
            if search_start >= len(lower_output):
                break

            remaining_idx = lower_output[search_start:].find(keyword)
            if remaining_idx == -1:
                break
            idx = search_start + remaining_idx

        return output


def default_rules() -> List[SanitizationRule]:
    """
    Return comprehensive sanitization rules matching Go pkg/shared/sanitization.

    IMPORTANT: Pattern order matters! Container patterns must come FIRST
    to prevent sub-patterns from corrupting larger structures.

    Returns:
        List of SanitizationRule with pre-compiled regex patterns
    """
    return [
        # ========================================
        # PRIORITY: Container patterns (process first)
        # These match larger structures that may contain sub-patterns
        # ========================================
        SanitizationRule(
            name="generator-url",
            pattern=re.compile(r'(?i)"generatorURL?"\s*:\s*"([^"]+)"'),
            replacement=f'"generatorURL":"{REDACTED_PLACEHOLDER}"',
            description="Redact Prometheus/Alertmanager generator URLs",
        ),
        SanitizationRule(
            name="annotations-json",
            pattern=re.compile(r'(?i)"annotations"\s*:\s*\{[^}]*\}'),
            replacement=f'"annotations":{REDACTED_PLACEHOLDER}',
            description="Redact webhook annotations",
        ),

        # ========================================
        # Database Connection Strings (P0)
        # ========================================
        SanitizationRule(
            name="postgresql-url",
            pattern=re.compile(r'postgresql://([^:]+):([^@]+)@'),
            replacement=f'postgresql://\\1:{REDACTED_PLACEHOLDER}@',
            description="Redact PostgreSQL URLs",
        ),
        SanitizationRule(
            name="mysql-url",
            pattern=re.compile(r'mysql://([^:]+):([^@]+)@'),
            replacement=f'mysql://\\1:{REDACTED_PLACEHOLDER}@',
            description="Redact MySQL URLs",
        ),
        SanitizationRule(
            name="mongodb-url",
            pattern=re.compile(r'mongodb://([^:]+):([^@]+)@'),
            replacement=f'mongodb://\\1:{REDACTED_PLACEHOLDER}@',
            description="Redact MongoDB URLs",
        ),
        SanitizationRule(
            name="redis-url",
            pattern=re.compile(r'redis://([^:]+):([^@]+)@'),
            replacement=f'redis://\\1:{REDACTED_PLACEHOLDER}@',
            description="Redact Redis URLs",
        ),
        SanitizationRule(
            name="generic-url-password",
            pattern=re.compile(r'://([^:/@\s]+):([^@\s]+)@'),
            replacement=f'://\\1:{REDACTED_PLACEHOLDER}@',
            description="Redact passwords in URLs",
        ),

        # ========================================
        # Password Patterns (P0)
        # ========================================
        SanitizationRule(
            name="password-json",
            pattern=re.compile(r'(?i)"(password|passwd|pwd)"\s*:\s*"([^"]+)"'),
            replacement=f'"\\1":"{REDACTED_PLACEHOLDER}"',
            description="Redact password in JSON",
        ),
        SanitizationRule(
            name="password-plain",
            pattern=re.compile(r'(?i)(password|passwd|pwd)\s*[:=]\s*["\']?([^\s"\',}]+)["\']?'),
            replacement=f'\\1: {REDACTED_PLACEHOLDER}',
            description="Redact password fields",
        ),

        # ========================================
        # API Key Patterns (P0)
        # ========================================
        SanitizationRule(
            name="openai-key",
            pattern=re.compile(r'sk-[A-Za-z0-9_\-]{4,}'),
            replacement=REDACTED_PLACEHOLDER,
            description="Redact OpenAI API keys",
        ),
        SanitizationRule(
            name="api-key-json",
            pattern=re.compile(r'(?i)"(api[_-]?key|apikey)"\s*:\s*"([^"]+)"'),
            replacement=f'"\\1":"{REDACTED_PLACEHOLDER}"',
            description="Redact API key in JSON",
        ),
        SanitizationRule(
            name="api-key-plain",
            pattern=re.compile(r'(?i)(api[_-]?key|apikey)\s*[:=]\s*["\']?([^\s"\',}]+)["\']?'),
            replacement=f'\\1: {REDACTED_PLACEHOLDER}',
            description="Redact API keys",
        ),

        # ========================================
        # Token Patterns (P0)
        # ========================================
        SanitizationRule(
            name="bearer-token",
            pattern=re.compile(r'(?i)Bearer\s+([A-Za-z0-9\-_\.]+)'),
            replacement=f'Bearer {REDACTED_PLACEHOLDER}',
            description="Redact Bearer tokens",
        ),
        SanitizationRule(
            name="jwt-token",
            pattern=re.compile(r'eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+'),
            replacement=f'{REDACTED_PLACEHOLDER}_JWT',
            description="Redact JWT tokens",
        ),
        SanitizationRule(
            name="github-token",
            pattern=re.compile(r'ghp_[A-Za-z0-9]{36,}'),
            replacement=f'{REDACTED_PLACEHOLDER}_GITHUB_TOKEN',
            description="Redact GitHub tokens",
        ),
        SanitizationRule(
            name="github-token-gho",
            pattern=re.compile(r'gho_[A-Za-z0-9]{36,}'),
            replacement=f'{REDACTED_PLACEHOLDER}_GITHUB_TOKEN',
            description="Redact GitHub OAuth tokens",
        ),
        SanitizationRule(
            name="token-json",
            pattern=re.compile(r'(?i)"(token|access[_-]?token)"\s*:\s*"([^"]+)"'),
            replacement=f'"\\1":"{REDACTED_PLACEHOLDER}"',
            description="Redact token in JSON",
        ),
        SanitizationRule(
            name="token-plain",
            pattern=re.compile(r'(?i)\b(token|access[_-]?token)\s*[:=]\s*["\']?([^\s"\',}]+)["\']?'),
            replacement=f'\\1: {REDACTED_PLACEHOLDER}',
            description="Redact generic tokens",
        ),

        # ========================================
        # Secret Patterns (P0/P2)
        # ========================================
        SanitizationRule(
            name="secret-json",
            pattern=re.compile(r'(?i)"(secret|client_secret)"\s*:\s*"([^"]+)"'),
            replacement=f'"\\1":"{REDACTED_PLACEHOLDER}"',
            description="Redact secret in JSON",
        ),
        SanitizationRule(
            name="secret-plain",
            pattern=re.compile(r'(?i)(secret|client_secret)\s*[:=]\s*["\']?([^\s"\',}]+)["\']?'),
            replacement=f'\\1: {REDACTED_PLACEHOLDER}',
            description="Redact secrets",
        ),

        # ========================================
        # Authorization Headers (P1)
        # ========================================
        SanitizationRule(
            name="authorization-header",
            pattern=re.compile(r'(?i)(authorization)\s*:\s*["\']?([^\s"\',}]+)["\']?'),
            replacement=f'\\1: {REDACTED_PLACEHOLDER}',
            description="Redact authorization headers",
        ),

        # ========================================
        # Cloud Provider Credentials (P1)
        # ========================================
        SanitizationRule(
            name="aws-access-key-id",
            pattern=re.compile(r'(?i)(AWS_ACCESS_KEY_ID|aws_access_key)\s*[:=]\s*["\']?([A-Z0-9]{20})["\']?'),
            replacement=f'\\1={REDACTED_PLACEHOLDER}',
            description="Redact AWS access key IDs",
        ),
        SanitizationRule(
            name="aws-secret-key",
            pattern=re.compile(r'(?i)(AWS_SECRET_ACCESS_KEY|aws_secret_key)\s*[:=]\s*["\']?([A-Za-z0-9/+=]{40})["\']?'),
            replacement=f'\\1={REDACTED_PLACEHOLDER}',
            description="Redact AWS secret keys",
        ),
        SanitizationRule(
            name="aws-access-key-inline",
            pattern=re.compile(r'AKIA[A-Z0-9]{16}'),
            replacement=f'{REDACTED_PLACEHOLDER}_AWS_KEY',
            description="Redact inline AWS access key IDs",
        ),

        # ========================================
        # Certificates and Keys (P1)
        # ========================================
        SanitizationRule(
            name="pem-certificate",
            pattern=re.compile(r'-----BEGIN CERTIFICATE-----[\s\S]*?-----END CERTIFICATE-----'),
            replacement=REDACTED_PLACEHOLDER,
            description="Redact PEM certificates",
        ),
        SanitizationRule(
            name="private-key",
            pattern=re.compile(r'-----BEGIN (?:RSA |EC )?PRIVATE KEY-----[\s\S]*?-----END (?:RSA |EC )?PRIVATE KEY-----'),
            replacement=REDACTED_PLACEHOLDER,
            description="Redact private keys",
        ),

        # ========================================
        # Kubernetes Secrets (P1)
        # ========================================
        SanitizationRule(
            name="k8s-secret-data",
            pattern=re.compile(r'(?m)^\s*(username|password|token|key|secret|credential):\s*([A-Za-z0-9+/=]{8,})\s*$'),
            replacement=f'  \\1: {REDACTED_PLACEHOLDER}',
            description="Redact Kubernetes secret data (base64)",
        ),

        # ========================================
        # Credential Patterns (P2)
        # ========================================
        SanitizationRule(
            name="credential-json",
            pattern=re.compile(r'(?i)"(credential|credentials)"\s*:\s*"([^"]+)"'),
            replacement=f'"\\1":"{REDACTED_PLACEHOLDER}"',
            description="Redact credential in JSON",
        ),
        SanitizationRule(
            name="credential-plain",
            pattern=re.compile(r'(?i)(credential|credentials)\s*[:=]\s*["\']?([^\s"\',}]+)["\']?'),
            replacement=f'\\1: {REDACTED_PLACEHOLDER}',
            description="Redact credentials",
        ),

        # ========================================
        # Additional Password Variants (P2)
        # ========================================
        SanitizationRule(
            name="registry-password-json",
            pattern=re.compile(r'(?i)"([a-z_]*password[a-z_]*)"\s*:\s*"([^"]+)"'),
            replacement=f'"\\1":"{REDACTED_PLACEHOLDER}"',
            description="Redact *password* variants in JSON (registry_password, db_password, etc.)",
        ),
    ]


# Default sanitizer instance for convenience functions
_default_sanitizer = LLMSanitizer()


def sanitize_for_llm(content: Any) -> Any:
    """
    Sanitize any content type before sending to LLM.

    BR-HAPI-211: ALL data sent to external LLM providers must be sanitized.
    DD-HAPI-005: Handles str, dict, list, None types.

    Args:
        content: Content to sanitize (str, dict, list, or None)

    Returns:
        Sanitized content of the same type

    Example:
        # String
        safe = sanitize_for_llm('password: secret123')
        # Returns: 'password: [REDACTED]'

        # Dict
        safe = sanitize_for_llm({"password": "secret"})
        # Returns: {"password": "[REDACTED]"}

        # List
        safe = sanitize_for_llm(["password: x", "api_key: y"])
        # Returns: ["password: [REDACTED]", "api_key: [REDACTED]"]
    """
    if content is None:
        return None

    if isinstance(content, str):
        return _default_sanitizer.sanitize(content)

    if isinstance(content, dict):
        # Serialize to JSON, sanitize, deserialize
        try:
            json_str = json.dumps(content, default=str)
            sanitized_str = _default_sanitizer.sanitize(json_str)
            return json.loads(sanitized_str)
        except (json.JSONDecodeError, TypeError) as e:
            logger.warning({
                "event": "sanitize_dict_json_error",
                "br": "BR-HAPI-211",
                "error": str(e),
                "fallback": "string_sanitization"
            })
            # Fallback: sanitize string representation
            return _default_sanitizer.sanitize(str(content))

    if isinstance(content, list):
        return [sanitize_for_llm(item) for item in content]

    # Fallback: convert to string and sanitize
    return _default_sanitizer.sanitize(str(content))


def sanitize_with_fallback(content: str) -> Tuple[str, Optional[Exception]]:
    """
    Sanitize string with automatic fallback on errors.

    Convenience wrapper for LLMSanitizer.sanitize_with_fallback().

    Args:
        content: String content to sanitize

    Returns:
        Tuple of (sanitized_content, error_if_fallback_used)
    """
    return _default_sanitizer.sanitize_with_fallback(content)

