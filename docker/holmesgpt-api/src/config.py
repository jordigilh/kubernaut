"""
Configuration management for HolmesGPT REST API
Environment-based configuration with validation - BR-HAPI-021, BR-HAPI-024
"""

import os
import logging
from typing import Optional
from functools import lru_cache

import structlog
from pydantic import Field, field_validator
from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Application settings with validation - BR-HAPI-021"""

    # Server Configuration - BR-HAPI-051, BR-HAPI-052
    port: int = Field(default=8090, description="HTTP server port")
    metrics_port: int = Field(default=9091, description="Metrics server port")
    host: str = Field(default="0.0.0.0", description="Server bind address")

    # LLM Configuration - BR-HAPI-026, BR-HAPI-027
    llm_provider: str = Field(..., description="LLM provider (openai, anthropic, local, ollama)")
    llm_model: str = Field(..., description="LLM model name")
    llm_api_key: Optional[str] = Field(None, description="LLM API key")
    llm_base_url: Optional[str] = Field(None, description="LLM base URL for local/custom providers")

    # HolmesGPT Configuration
    holmesgpt_config_path: str = Field(default="/app/config/settings.yaml", description="HolmesGPT config file")
    holmesgpt_toolsets_dir: str = Field(default="/app/toolsets", description="Custom toolsets directory")

    # Context API Integration - BR-HAPI-011
    context_api_url: str = Field(default="http://kubernaut-context-api:8091", description="Context API base URL")
    context_api_timeout: int = Field(default=30, description="Context API request timeout")

    # Security Configuration - BR-HAPI-062
    jwt_secret_key: str = Field(default="development-key-change-in-production", description="JWT secret key")
    jwt_algorithm: str = Field(default="HS256", description="JWT signing algorithm")
    jwt_expire_minutes: int = Field(default=60, description="JWT token expiration minutes")
    access_token_expire_minutes: int = Field(default=60, description="Access token expiration minutes (alias for jwt_expire_minutes)")

    # Rate Limiting - BR-HAPI-067
    rate_limit_requests: int = Field(default=100, description="Rate limit requests per minute")
    rate_limit_window: int = Field(default=60, description="Rate limit time window in seconds")

    # Caching Configuration - BR-HAPI-014
    redis_url: Optional[str] = Field(None, description="Redis URL for caching")
    cache_ttl: int = Field(default=300, description="Cache TTL in seconds")

    # Logging Configuration
    log_level: str = Field(default="INFO", description="Log level")
    log_format: str = Field(default="json", description="Log format (json, text)")

    # Resource Limits - BR-HAPI-038
    max_concurrent_investigations: int = Field(default=10, description="Maximum concurrent investigations")
    investigation_timeout: int = Field(default=300, description="Investigation timeout in seconds")

    @field_validator('llm_provider')
    @classmethod
    def validate_llm_provider(cls, v):
        """Validate LLM provider - BR-HAPI-027"""
        valid_providers = ['openai', 'anthropic', 'local', 'ollama', 'azure', 'ramalama']
        if v not in valid_providers:
            raise ValueError(f'LLM provider must be one of: {valid_providers}')
        return v

    @field_validator('log_level')
    @classmethod
    def validate_log_level(cls, v):
        """Validate log level"""
        valid_levels = ['DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL']
        if v.upper() not in valid_levels:
            raise ValueError(f'Log level must be one of: {valid_levels}')
        return v.upper()

    model_config = {
        "env_prefix": "HOLMESGPT_",
        "case_sensitive": False,
        "env_file": "../.env",
        "env_file_encoding": "utf-8"
    }


@lru_cache()
def get_settings() -> Settings:
    """Get cached application settings - BR-HAPI-021"""
    return Settings()


def setup_logging():
    """Setup structured logging configuration"""
    settings = get_settings()

    # Configure structlog
    structlog.configure(
        processors=[
            structlog.stdlib.filter_by_level,
            structlog.stdlib.add_logger_name,
            structlog.stdlib.add_log_level,
            structlog.stdlib.PositionalArgumentsFormatter(),
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.processors.UnicodeDecoder(),
            structlog.processors.JSONRenderer() if settings.log_format == "json"
            else structlog.dev.ConsoleRenderer(),
        ],
        context_class=dict,
        logger_factory=structlog.stdlib.LoggerFactory(),
        wrapper_class=structlog.stdlib.BoundLogger,
        cache_logger_on_first_use=True,
    )

    # Configure standard logging
    logging.basicConfig(
        format="%(message)s",
        stream=None,
        level=getattr(logging, settings.log_level),
    )
