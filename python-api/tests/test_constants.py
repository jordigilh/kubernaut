"""
Test constants and validation rules - Single source of truth.

This module provides constants used across tests to prevent hardcoding
and ensure consistency between test expectations and actual validation rules.
"""

from typing import Set, List

# Alert validation constants
VALID_SEVERITIES: Set[str] = {"critical", "warning", "info"}
VALID_ALERT_STATUSES: Set[str] = {"firing", "resolved", "pending"}

# Model constants
VALID_MODELS: Set[str] = {"gpt-4", "gpt-3.5-turbo", "gpt-oss:20b", "test-model", "mock-gpt-4"}

# Validation ranges
MIN_CONFIDENCE = 0.0
MAX_CONFIDENCE = 1.0
MIN_TEMPERATURE = 0.0
MAX_TEMPERATURE = 2.0
MIN_MAX_TOKENS = 1
MAX_MAX_TOKENS = 10000
MIN_TIMEOUT = 1
MAX_TIMEOUT = 300
MIN_CONTEXT_WINDOW = 512
MAX_CONTEXT_WINDOW = 32000

# HTTP Status codes
SUCCESS_STATUSES = {200, 201, 202}
CLIENT_ERROR_STATUSES = {400, 422}
SERVER_ERROR_STATUSES = {500, 503}

# Log levels
VALID_LOG_LEVELS: Set[str] = {"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"}

# Environment types
VALID_ENVIRONMENTS: Set[str] = {"development", "staging", "production", "test"}

# Health status types
VALID_HEALTH_STATUSES: Set[str] = {"healthy", "degraded", "unhealthy"}

# Assessment levels
VALID_URGENCY_LEVELS: Set[str] = {"low", "medium", "high", "critical"}
VALID_SEVERITY_ASSESSMENTS: Set[str] = {"low", "medium", "high", "critical"}
VALID_RISK_LEVELS: Set[str] = {"low", "medium", "high"}

# Default test values (ranges for property-based testing)
DEFAULT_CONFIDENCE_RANGE = (0.7, 0.95)
DEFAULT_PROCESSING_TIME_RANGE = (0.1, 10.0)
DEFAULT_TOKENS_RANGE = (100, 4000)

# Test response templates
TEST_RESPONSE_PATTERNS = [
    "This is a test response",
    "Mock response from HolmesGPT",
    "Simple investigation result",
    "Response {index}",  # For concurrent tests
]

# Kubernetes context defaults
DEFAULT_K8S_NAMESPACES = ["default", "production", "staging", "api-namespace", "prod"]
DEFAULT_K8S_DEPLOYMENTS = ["api-server", "web-app", "database", "cache"]
DEFAULT_K8S_SERVICES = ["api-service", "web-service", "db-service"]

def is_valid_severity(severity: str) -> bool:
    """Check if severity is valid."""
    return severity.lower() in VALID_SEVERITIES

def is_valid_confidence(confidence: float) -> bool:
    """Check if confidence is in valid range."""
    return MIN_CONFIDENCE <= confidence <= MAX_CONFIDENCE

def is_valid_temperature(temperature: float) -> bool:
    """Check if temperature is in valid range."""
    return MIN_TEMPERATURE <= temperature <= MAX_TEMPERATURE

def is_valid_processing_time(time: float) -> bool:
    """Check if processing time is reasonable."""
    return 0.0 <= time <= 60.0  # Max 1 minute

def get_test_confidence() -> float:
    """Get a valid test confidence value."""
    return 0.85  # Within DEFAULT_CONFIDENCE_RANGE

def get_test_processing_time() -> float:
    """Get a valid test processing time."""
    return 1.5  # Within DEFAULT_PROCESSING_TIME_RANGE
