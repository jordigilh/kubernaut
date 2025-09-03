"""
Configuration management for HolmesGPT REST API.
"""

import os
from typing import List, Optional, Dict, Any
from functools import lru_cache
from pydantic import Field, field_validator
from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Application settings."""

    # API Configuration
    app_name: str = Field(default="HolmesGPT REST API")
    version: str = Field(default="1.0.0")
    debug_mode: bool = Field(default=False)
    host: str = Field(default="0.0.0.0")
    port: int = Field(default=8000)
    workers: int = Field(default=1)

    # Logging Configuration
    log_level: str = Field(default="INFO")
    log_format: str = Field(default="json")  # json, text
    log_file: Optional[str] = Field(default=None)

    # Security Configuration
    enable_cors: bool = Field(default=True)
    cors_origins: List[str] = Field(default=["*"])
    api_key_required: bool = Field(default=False)
    api_key: Optional[str] = Field(default=None)

    # Documentation Configuration
    enable_docs: bool = Field(default=True)
    docs_url: str = Field(default="/docs")
    redoc_url: str = Field(default="/redoc")

    # Metrics Configuration
    metrics_enabled: bool = Field(default=True)
    metrics_port: int = Field(default=9090)
    metrics_path: str = Field(default="/metrics")

    # HolmesGPT Configuration
    holmes_direct_import: bool = Field(default=True)
    holmes_cli_fallback: bool = Field(default=True)
    holmes_cli_path: str = Field(default="holmes")
    holmes_working_dir: Optional[str] = Field(default=None)
    holmes_config_path: Optional[str] = Field(default=None)

    # HolmesGPT LLM Provider Configuration
    holmes_llm_provider: str = Field(default="openai")  # openai, anthropic, azure, bedrock, ollama, ramalama
    holmes_llm_api_key: Optional[str] = Field(default=None)
    holmes_llm_base_url: Optional[str] = Field(default=None)
    holmes_llm_model: Optional[str] = Field(default=None)

    # OpenAI Configuration
    openai_api_key: Optional[str] = Field(default=None)
    openai_base_url: Optional[str] = Field(default=None)
    openai_organization: Optional[str] = Field(default=None)

    # Azure OpenAI Configuration
    azure_openai_api_key: Optional[str] = Field(default=None)
    azure_openai_endpoint: Optional[str] = Field(default=None)
    azure_openai_api_version: str = Field(default="2023-12-01-preview")

    # AWS Bedrock Configuration
    aws_access_key_id: Optional[str] = Field(default=None)
    aws_secret_access_key: Optional[str] = Field(default=None)
    aws_region: str = Field(default="us-east-1")

    # Anthropic Configuration
    anthropic_api_key: Optional[str] = Field(default=None)

    # Default HolmesGPT Options
    holmes_default_model: str = Field(default="gpt-oss:20b")
    holmes_default_max_tokens: int = Field(default=4000)
    holmes_default_temperature: float = Field(default=0.3)
    holmes_default_timeout: int = Field(default=300)
    holmes_enable_streaming: bool = Field(default=True)
    holmes_enable_debug: bool = Field(default=False)

    # Ollama Configuration
    ollama_url: str = Field(default="http://localhost:11434")
    ollama_timeout: int = Field(default=30)
    ollama_verify_ssl: bool = Field(default=True)

    # Ramalama Configuration
    ramalama_url: str = Field(default="http://localhost:8080")
    ramalama_timeout: int = Field(default=30)
    ramalama_verify_ssl: bool = Field(default=True)

    # Kubernetes Configuration
    kube_config_path: Optional[str] = Field(default=None)
    kube_in_cluster: bool = Field(default=False)
    kube_namespace: Optional[str] = Field(default=None)

    # Prometheus Configuration
    prometheus_url: Optional[str] = Field(default=None)
    prometheus_timeout: int = Field(default=30)
    prometheus_verify_ssl: bool = Field(default=True)

    # AlertManager Configuration
    alertmanager_url: Optional[str] = Field(default=None)
    alertmanager_timeout: int = Field(default=30)
    alertmanager_verify_ssl: bool = Field(default=True)

    # Cache Configuration
    cache_enabled: bool = Field(default=True)
    cache_ttl: int = Field(default=300)  # seconds
    cache_max_size: int = Field(default=1000)

    # Rate Limiting Configuration
    rate_limit_enabled: bool = Field(default=False)
    rate_limit_requests: int = Field(default=100)
    rate_limit_period: int = Field(default=60)  # seconds

    # Performance Configuration
    max_concurrent_requests: int = Field(default=10)
    request_timeout: int = Field(default=600)
    background_tasks_enabled: bool = Field(default=True)

    # Development Configuration
    reload_on_change: bool = Field(default=False)
    profiling_enabled: bool = Field(default=False)
    test_mode: bool = Field(default=False)

    # External Services Configuration
    external_services: Dict[str, Any] = Field(
        default_factory=dict,
        env="EXTERNAL_SERVICES"
    )

    # Action History Configuration
    action_history_db_path: str = Field(default="/tmp/action_history.db")

    @field_validator('log_level')
    @classmethod
    def validate_log_level(cls, v):
        valid_levels = ['DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL']
        if v.upper() not in valid_levels:
            raise ValueError(f'Log level must be one of {valid_levels}')
        return v.upper()

    @field_validator('log_format')
    @classmethod
    def validate_log_format(cls, v):
        valid_formats = ['json', 'text']
        if v.lower() not in valid_formats:
            raise ValueError(f'Log format must be one of {valid_formats}')
        return v.lower()

    @field_validator('holmes_default_temperature')
    @classmethod
    def validate_temperature(cls, v):
        if not 0.0 <= v <= 2.0:
            raise ValueError('Temperature must be between 0.0 and 2.0')
        return v

    @field_validator('cors_origins', mode='before')
    @classmethod
    def validate_cors_origins(cls, v):
        if isinstance(v, str):
            return v.split(',')
        return v

    @field_validator('holmes_default_max_tokens')
    @classmethod
    def validate_max_tokens(cls, v):
        if v <= 0:
            raise ValueError('Max tokens must be positive')
        if v > 100000:
            raise ValueError('Max tokens should be reasonable (< 100k)')
        return v

    @field_validator('workers')
    @classmethod
    def validate_workers(cls, v):
        if v <= 0:
            raise ValueError('Workers must be positive')
        return min(v, 10)  # Reasonable upper limit

    @field_validator('port')
    @classmethod
    def validate_port(cls, v):
        if not 1 <= v <= 65535:
            raise ValueError('Port must be between 1 and 65535')
        return v

    @field_validator('metrics_port')
    @classmethod
    def validate_metrics_port(cls, v):
        if not 1 <= v <= 65535:
            raise ValueError('Metrics port must be between 1 and 65535')
        return v

    def get_holmes_config(self) -> Dict[str, Any]:
        """Get HolmesGPT-specific configuration."""
        return {
            'direct_import': self.holmes_direct_import,
            'cli_fallback': self.holmes_cli_fallback,
            'cli_path': self.holmes_cli_path,
            'working_dir': self.holmes_working_dir,
            'config_path': self.holmes_config_path,
            'default_model': self.holmes_default_model,
            'default_max_tokens': self.holmes_default_max_tokens,
            'default_temperature': self.holmes_default_temperature,
            'default_timeout': self.holmes_default_timeout,
            'enable_streaming': self.holmes_enable_streaming,
            'enable_debug': self.holmes_enable_debug,
            'ollama_url': self.ollama_url,
            'ollama_timeout': self.ollama_timeout,
            'llm_provider': self.holmes_llm_provider,
            'llm_api_key': self.holmes_llm_api_key,
            'llm_base_url': self.holmes_llm_base_url,
            'llm_model': self.holmes_llm_model,
        }

    def get_llm_config(self) -> Dict[str, Any]:
        """Get LLM provider configuration for HolmesGPT."""
        base_config = {
            'provider': self.holmes_llm_provider,
            'model': self.holmes_llm_model or self.holmes_default_model,
            'temperature': self.holmes_default_temperature,
            'max_tokens': self.holmes_default_max_tokens,
        }

        if self.holmes_llm_provider == "openai":
            base_config.update({
                'api_key': self.holmes_llm_api_key or self.openai_api_key,
                'base_url': self.holmes_llm_base_url or self.openai_base_url,
                'organization': self.openai_organization,
            })
        elif self.holmes_llm_provider == "azure":
            base_config.update({
                'api_key': self.holmes_llm_api_key or self.azure_openai_api_key,
                'endpoint': self.holmes_llm_base_url or self.azure_openai_endpoint,
                'api_version': self.azure_openai_api_version,
            })
        elif self.holmes_llm_provider == "anthropic":
            base_config.update({
                'api_key': self.holmes_llm_api_key or self.anthropic_api_key,
            })
        elif self.holmes_llm_provider == "bedrock":
            base_config.update({
                'aws_access_key_id': self.aws_access_key_id,
                'aws_secret_access_key': self.aws_secret_access_key,
                'region': self.aws_region,
            })
        elif self.holmes_llm_provider == "ollama":
            base_config.update({
                'base_url': self.holmes_llm_base_url or self.ollama_url,
                'timeout': self.ollama_timeout,
            })
        elif self.holmes_llm_provider == "ramalama":
            base_config.update({
                'base_url': self.holmes_llm_base_url or self.ramalama_url,
                'timeout': self.ramalama_timeout,
            })

        return base_config

    def get_monitoring_config(self) -> Dict[str, Any]:
        """Get monitoring configuration."""
        return {
            'prometheus_url': self.prometheus_url,
            'prometheus_timeout': self.prometheus_timeout,
            'prometheus_verify_ssl': self.prometheus_verify_ssl,
            'alertmanager_url': self.alertmanager_url,
            'alertmanager_timeout': self.alertmanager_timeout,
            'alertmanager_verify_ssl': self.alertmanager_verify_ssl,
        }

    def get_kubernetes_config(self) -> Dict[str, Any]:
        """Get Kubernetes configuration."""
        return {
            'config_path': self.kube_config_path,
            'in_cluster': self.kube_in_cluster,
            'namespace': self.kube_namespace,
        }

    def is_development(self) -> bool:
        """Check if running in development mode."""
        return self.debug_mode or self.test_mode or os.getenv('ENVIRONMENT', '').lower() in ['dev', 'development']

    def is_production(self) -> bool:
        """Check if running in production mode."""
        return os.getenv('ENVIRONMENT', '').lower() in ['prod', 'production']

    model_config = {"env_file": ".env", "env_file_encoding": "utf-8", "case_sensitive": False}


class DevelopmentSettings(Settings):
    """Development-specific settings."""
    debug_mode: bool = True
    log_level: str = "DEBUG"
    log_format: str = "text"
    enable_docs: bool = True
    reload_on_change: bool = True
    profiling_enabled: bool = True
    holmes_enable_debug: bool = True
    workers: int = 1


class ProductionSettings(Settings):
    """Production-specific settings."""
    debug_mode: bool = False
    log_level: str = "INFO"
    log_format: str = "json"
    enable_docs: bool = False
    reload_on_change: bool = False
    profiling_enabled: bool = False
    holmes_enable_debug: bool = False
    api_key_required: bool = True
    rate_limit_enabled: bool = True


class TestingEnvironmentSettings(Settings):
    """Test-specific settings - not a pytest test class."""
    debug_mode: bool = True
    test_mode: bool = True
    log_level: str = "DEBUG"
    metrics_enabled: bool = False
    cache_enabled: bool = False
    background_tasks_enabled: bool = False
    holmes_cli_fallback: bool = False  # Use mocks in tests


# Alias for backward compatibility (but avoid pytest collection warnings)
TestEnvironmentSettings = TestingEnvironmentSettings


@lru_cache()
def get_settings() -> Settings:
    """Get application settings with caching."""
    environment = os.getenv('ENVIRONMENT', 'development').lower()

    if environment in ['prod', 'production']:
        return ProductionSettings()
    elif environment in ['test', 'testing']:
        return TestEnvironmentSettings()
    else:
        return DevelopmentSettings()


def get_settings_no_cache() -> Settings:
    """Get application settings without caching (for testing)."""
    environment = os.getenv('ENVIRONMENT', 'development').lower()

    if environment in ['prod', 'production']:
        return ProductionSettings()
    elif environment in ['test', 'testing']:
        return TestEnvironmentSettings()
    else:
        return DevelopmentSettings()
