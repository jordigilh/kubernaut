"""
Tests for configuration management.
"""

import os
import pytest
from typing import Dict, Any
from unittest.mock import patch

from app.config import (
    Settings, DevelopmentSettings, ProductionSettings, TestEnvironmentSettings,
    get_settings, get_settings_no_cache
)


class TestSettings:
    """Test base Settings class."""

    def test_default_values(self):
        """Test default configuration values."""
        # Test base Settings class directly to avoid environment-specific settings
        with patch.dict(os.environ, {
            'ENVIRONMENT': '',
            'DEBUG_MODE': 'false',
            'LOG_LEVEL': 'INFO',
            'METRICS_ENABLED': 'false',
            'CACHE_ENABLED': 'false'
        }, clear=False):
            settings = Settings()

        # API defaults
        assert settings.app_name == "HolmesGPT REST API"
        assert settings.version == "1.0.0"
        assert settings.debug_mode is False
        assert settings.host == "0.0.0.0"
        assert settings.port == 8000
        assert settings.workers == 1

        # Logging defaults
        assert settings.log_level == "INFO"
        assert settings.log_format == "json"
        assert settings.log_file is None

        # Security defaults
        assert settings.enable_cors is True
        assert settings.cors_origins == ["*"]
        assert settings.api_key_required is False

        # HolmesGPT defaults
        assert settings.holmes_default_model == "gpt-oss:20b"
        assert settings.holmes_default_max_tokens == 4000
        assert settings.holmes_default_temperature == 0.3
        assert settings.holmes_llm_provider == "openai"

    def test_environment_variable_override(self, monkeypatch):
        """Test environment variable overrides."""
        test_values = {
            "APP_NAME": "Test API",
            "PORT": "9000",
            "DEBUG_MODE": "true",
            "LOG_LEVEL": "DEBUG",
            "HOLMES_DEFAULT_MODEL": "gpt-4",
            "HOLMES_DEFAULT_TEMPERATURE": "0.7",
        }

        for key, value in test_values.items():
            monkeypatch.setenv(key, value)

        settings = Settings()

        assert settings.app_name == "Test API"
        assert settings.port == 9000
        assert settings.debug_mode is True
        assert settings.log_level == "DEBUG"
        assert settings.holmes_default_model == "gpt-4"
        assert settings.holmes_default_temperature == 0.7

    def test_cors_origins_string_conversion(self, monkeypatch):
        """Test CORS origins JSON array parsing."""
        monkeypatch.setenv("CORS_ORIGINS", '["http://localhost:3000", "https://app.example.com"]')

        settings = Settings()
        assert settings.cors_origins == ["http://localhost:3000", "https://app.example.com"]

    def test_log_level_validation(self):
        """Test log level validation."""
        # Valid log level
        settings = Settings(log_level="WARNING")
        assert settings.log_level == "WARNING"

        # Invalid log level should raise ValueError
        with pytest.raises(ValueError, match="Log level must be one of"):
            Settings(log_level="INVALID")

    def test_log_format_validation(self):
        """Test log format validation."""
        # Valid formats
        settings_json = Settings(log_format="json")
        assert settings_json.log_format == "json"

        settings_text = Settings(log_format="text")
        assert settings_text.log_format == "text"

        # Invalid format should raise ValueError
        with pytest.raises(ValueError, match="Log format must be one of"):
            Settings(log_format="invalid")

    def test_temperature_validation(self):
        """Test temperature validation."""
        # Valid temperatures
        Settings(holmes_default_temperature=0.0)
        Settings(holmes_default_temperature=1.0)
        Settings(holmes_default_temperature=2.0)

        # Invalid temperatures
        with pytest.raises(ValueError, match="Temperature must be between 0.0 and 2.0"):
            Settings(holmes_default_temperature=-0.1)

        with pytest.raises(ValueError, match="Temperature must be between 0.0 and 2.0"):
            Settings(holmes_default_temperature=2.1)

    def test_max_tokens_validation(self):
        """Test max tokens validation."""
        # Valid values
        Settings(holmes_default_max_tokens=1000)
        Settings(holmes_default_max_tokens=4000)

        # Invalid values
        with pytest.raises(ValueError, match="Max tokens must be positive"):
            Settings(holmes_default_max_tokens=0)

        with pytest.raises(ValueError, match="Max tokens must be positive"):
            Settings(holmes_default_max_tokens=-100)

        with pytest.raises(ValueError, match="Max tokens should be reasonable"):
            Settings(holmes_default_max_tokens=200000)

    def test_workers_validation(self):
        """Test workers validation and capping."""
        # Valid values
        settings = Settings(workers=5)
        assert settings.workers == 5

        # Values capped at 10
        settings = Settings(workers=15)
        assert settings.workers == 10

        # Invalid values
        with pytest.raises(ValueError, match="Workers must be positive"):
            Settings(workers=0)

    def test_port_validation(self):
        """Test port validation."""
        # Valid ports
        Settings(port=8000)
        Settings(port=65535)

        # Invalid ports
        with pytest.raises(ValueError, match="Port must be between 1 and 65535"):
            Settings(port=0)

        with pytest.raises(ValueError, match="Port must be between 1 and 65535"):
            Settings(port=65536)

    def test_metrics_port_validation(self):
        """Test metrics port validation."""
        # Valid ports
        Settings(metrics_port=9090)
        Settings(metrics_port=1)

        # Invalid ports
        with pytest.raises(ValueError, match="Metrics port must be between 1 and 65535"):
            Settings(metrics_port=0)

        with pytest.raises(ValueError, match="Metrics port must be between 1 and 65535"):
            Settings(metrics_port=65536)


class TestHolmesConfiguration:
    """Test HolmesGPT-specific configuration methods."""

    def test_get_holmes_config(self):
        """Test get_holmes_config method."""
        settings = Settings(
            holmes_direct_import=True,
            holmes_cli_fallback=False,
            holmes_default_model="gpt-4",
            holmes_default_temperature=0.5,
            ollama_url="http://localhost:11434"
        )

        config = settings.get_holmes_config()

        assert config["direct_import"] is True
        assert config["cli_fallback"] is False
        assert config["default_model"] == "gpt-4"
        assert config["default_temperature"] == 0.5
        assert config["ollama_url"] == "http://localhost:11434"
        assert config["llm_provider"] == "openai"  # default

    def test_get_llm_config_openai(self):
        """Test LLM config for OpenAI provider."""
        settings = Settings(
            holmes_llm_provider="openai",
            openai_api_key="test-key",
            openai_base_url="https://api.openai.com/v1",
            openai_organization="test-org",
            holmes_default_model="gpt-4"
        )

        config = settings.get_llm_config()

        assert config["provider"] == "openai"
        assert config["model"] == "gpt-4"
        assert config["api_key"] == "test-key"
        assert config["base_url"] == "https://api.openai.com/v1"
        assert config["organization"] == "test-org"

    def test_get_llm_config_azure(self):
        """Test LLM config for Azure provider."""
        settings = Settings(
            holmes_llm_provider="azure",
            azure_openai_api_key="test-azure-key",
            azure_openai_endpoint="https://test.openai.azure.com/",
            azure_openai_api_version="2023-12-01-preview",
            holmes_default_model="gpt-4"
        )

        config = settings.get_llm_config()

        assert config["provider"] == "azure"
        assert config["model"] == "gpt-4"
        assert config["api_key"] == "test-azure-key"
        assert config["endpoint"] == "https://test.openai.azure.com/"
        assert config["api_version"] == "2023-12-01-preview"

    def test_get_llm_config_anthropic(self):
        """Test LLM config for Anthropic provider."""
        settings = Settings(
            holmes_llm_provider="anthropic",
            anthropic_api_key="test-anthropic-key",
            holmes_default_model="claude-3-sonnet"
        )

        config = settings.get_llm_config()

        assert config["provider"] == "anthropic"
        assert config["model"] == "claude-3-sonnet"
        assert config["api_key"] == "test-anthropic-key"

    def test_get_llm_config_bedrock(self):
        """Test LLM config for AWS Bedrock provider."""
        settings = Settings(
            holmes_llm_provider="bedrock",
            aws_access_key_id="test-access-key",
            aws_secret_access_key="test-secret-key",
            aws_region="us-west-2",
            holmes_default_model="anthropic.claude-3-sonnet-20240229-v1:0"
        )

        config = settings.get_llm_config()

        assert config["provider"] == "bedrock"
        assert config["model"] == "anthropic.claude-3-sonnet-20240229-v1:0"
        assert config["aws_access_key_id"] == "test-access-key"
        assert config["aws_secret_access_key"] == "test-secret-key"
        assert config["region"] == "us-west-2"

    def test_get_monitoring_config(self):
        """Test get_monitoring_config method."""
        settings = Settings(
            prometheus_url="http://prometheus:9090",
            prometheus_timeout=60,
            alertmanager_url="http://alertmanager:9093",
            alertmanager_timeout=30
        )

        config = settings.get_monitoring_config()

        assert config["prometheus_url"] == "http://prometheus:9090"
        assert config["prometheus_timeout"] == 60
        assert config["alertmanager_url"] == "http://alertmanager:9093"
        assert config["alertmanager_timeout"] == 30

    def test_get_kubernetes_config(self):
        """Test get_kubernetes_config method."""
        settings = Settings(
            kube_config_path="/path/to/kubeconfig",
            kube_in_cluster=True,
            kube_namespace="production"
        )

        config = settings.get_kubernetes_config()

        assert config["config_path"] == "/path/to/kubeconfig"
        assert config["in_cluster"] is True
        assert config["namespace"] == "production"


class TestEnvironmentSpecificSettings:
    """Test environment-specific settings classes."""

    def test_development_settings(self):
        """Test development settings."""
        settings = DevelopmentSettings()

        assert settings.debug_mode is True
        assert settings.log_level == "DEBUG"
        assert settings.log_format == "text"
        assert settings.enable_docs is True
        assert settings.reload_on_change is True
        assert settings.profiling_enabled is True
        assert settings.holmes_enable_debug is True
        assert settings.workers == 1

    def test_production_settings(self):
        """Test production settings."""
        with patch.dict(os.environ, {
            'DEBUG_MODE': 'false',
            'LOG_LEVEL': 'INFO',
            'ENABLE_DOCS': 'false',
            'HOLMES_ENABLE_DEBUG': 'false'
        }, clear=False):
            settings = ProductionSettings()

        assert settings.debug_mode is False
        assert settings.log_level == "INFO"
        assert settings.log_format == "json"
        assert settings.enable_docs is False
        assert settings.reload_on_change is False
        assert settings.profiling_enabled is False
        assert settings.holmes_enable_debug is False
        assert settings.api_key_required is True
        assert settings.rate_limit_enabled is True

    def test_test_settings(self):
        """Test test settings."""
        settings = TestEnvironmentSettings()

        assert settings.debug_mode is True
        assert settings.test_mode is True
        assert settings.log_level == "DEBUG"
        assert settings.metrics_enabled is False
        assert settings.cache_enabled is False
        assert settings.background_tasks_enabled is False
        assert settings.holmes_cli_fallback is False

    def test_environment_detection_methods(self):
        """Test environment detection methods."""
        # Development
        dev_settings = DevelopmentSettings()
        assert dev_settings.is_development() is True
        assert dev_settings.is_production() is False

        # Production
        with patch.dict(os.environ, {"ENVIRONMENT": "production", "DEBUG_MODE": "false", "LOG_LEVEL": "INFO"}):
            prod_settings = ProductionSettings()
            assert prod_settings.is_development() is False
            assert prod_settings.is_production() is True

        # Test
        test_settings = TestEnvironmentSettings()
        assert test_settings.is_development() is True  # debug_mode=True
        assert test_settings.is_production() is False


class TestSettingsFactory:
    """Test settings factory functions."""

    def test_get_settings_development(self):
        """Test get_settings returns development settings."""
        with patch.dict(os.environ, {"ENVIRONMENT": "development"}):
            settings = get_settings_no_cache()
            assert isinstance(settings, DevelopmentSettings)
            assert settings.debug_mode is True

    def test_get_settings_production(self):
        """Test get_settings returns production settings."""
        with patch.dict(os.environ, {
            "ENVIRONMENT": "production",
            "DEBUG_MODE": "false",
            "LOG_LEVEL": "INFO",
            "HOLMES_ENABLE_DEBUG": "false"
        }):
            settings = get_settings_no_cache()
            assert isinstance(settings, ProductionSettings)
            assert settings.debug_mode is False

    def test_get_settings_test(self):
        """Test get_settings returns test settings."""
        with patch.dict(os.environ, {"ENVIRONMENT": "test"}):
            settings = get_settings_no_cache()
            assert isinstance(settings, TestEnvironmentSettings)
            assert settings.test_mode is True

    def test_get_settings_default(self):
        """Test get_settings returns development settings by default."""
        with patch.dict(os.environ, {}, clear=True):
            settings = get_settings_no_cache()
            assert isinstance(settings, DevelopmentSettings)

    def test_get_settings_cached(self):
        """Test get_settings caching behavior."""
        with patch.dict(os.environ, {"ENVIRONMENT": "test"}):
            # First call
            settings1 = get_settings()
            # Second call should return same instance (cached)
            settings2 = get_settings()
            assert settings1 is settings2

    def test_get_settings_no_cache_not_cached(self):
        """Test get_settings_no_cache returns new instances."""
        with patch.dict(os.environ, {"ENVIRONMENT": "test"}):
            settings1 = get_settings_no_cache()
            settings2 = get_settings_no_cache()
            # Should be different instances
            assert settings1 is not settings2
            # But same type and values
            assert type(settings1) == type(settings2)
            assert settings1.test_mode == settings2.test_mode


class TestConfigurationIntegration:
    """Test configuration integration scenarios."""

    def test_complete_openai_configuration(self, monkeypatch):
        """Test complete OpenAI configuration."""
        config_vars = {
            "HOLMES_LLM_PROVIDER": "openai",
            "OPENAI_API_KEY": "sk-test123",
            "OPENAI_BASE_URL": "https://api.openai.com/v1",
            "OPENAI_ORGANIZATION": "org-test",
            "HOLMES_DEFAULT_MODEL": "gpt-4",
            "HOLMES_DEFAULT_MAX_TOKENS": "4000",
            "HOLMES_DEFAULT_TEMPERATURE": "0.1",
        }

        for key, value in config_vars.items():
            monkeypatch.setenv(key, value)

        settings = Settings()
        llm_config = settings.get_llm_config()

        assert llm_config["provider"] == "openai"
        assert llm_config["api_key"] == "sk-test123"
        assert llm_config["base_url"] == "https://api.openai.com/v1"
        assert llm_config["organization"] == "org-test"
        assert llm_config["model"] == "gpt-4"
        assert llm_config["max_tokens"] == 4000
        assert llm_config["temperature"] == 0.1

    def test_complete_ollama_configuration(self, monkeypatch):
        """Test complete Ollama configuration."""
        config_vars = {
            "OLLAMA_URL": "http://localhost:11434",
            "OLLAMA_TIMEOUT": "60",
            "HOLMES_DEFAULT_MODEL": "llama3.1:8b",
            "PROMETHEUS_URL": "http://prometheus:9090",
            "KUBE_IN_CLUSTER": "true",
        }

        for key, value in config_vars.items():
            monkeypatch.setenv(key, value)

        settings = Settings()

        assert settings.ollama_url == "http://localhost:11434"
        assert settings.ollama_timeout == 60
        assert settings.holmes_default_model == "llama3.1:8b"

        monitoring_config = settings.get_monitoring_config()
        assert monitoring_config["prometheus_url"] == "http://prometheus:9090"

        k8s_config = settings.get_kubernetes_config()
        assert k8s_config["in_cluster"] is True

    def test_security_configuration(self, monkeypatch):
        """Test security-related configuration."""
        config_vars = {
            "API_KEY_REQUIRED": "true",
            "API_KEY": "test-api-key",
            "ENABLE_CORS": "true",
            "CORS_ORIGINS": '["https://app.example.com", "https://admin.example.com"]',
            "RATE_LIMIT_ENABLED": "true",
            "RATE_LIMIT_REQUESTS": "50",
            "RATE_LIMIT_PERIOD": "60",
        }

        for key, value in config_vars.items():
            monkeypatch.setenv(key, value)

        settings = Settings()

        assert settings.api_key_required is True
        assert settings.api_key == "test-api-key"
        assert settings.enable_cors is True
        assert settings.cors_origins == ["https://app.example.com", "https://admin.example.com"]
        assert settings.rate_limit_enabled is True
        assert settings.rate_limit_requests == 50
        assert settings.rate_limit_period == 60

    def test_performance_configuration(self, monkeypatch):
        """Test performance-related configuration."""
        config_vars = {
            "WORKERS": "4",
            "MAX_CONCURRENT_REQUESTS": "20",
            "REQUEST_TIMEOUT": "300",
            "CACHE_ENABLED": "true",
            "CACHE_TTL": "600",
            "CACHE_MAX_SIZE": "2000",
        }

        for key, value in config_vars.items():
            monkeypatch.setenv(key, value)

        settings = Settings()

        assert settings.workers == 4
        assert settings.max_concurrent_requests == 20
        assert settings.request_timeout == 300
        assert settings.cache_enabled is True
        assert settings.cache_ttl == 600
        assert settings.cache_max_size == 2000

    def test_missing_required_configuration(self):
        """Test behavior with missing required configuration."""
        # Settings should still be valid with defaults
        settings = Settings()

        # But some LLM configs might be incomplete
        llm_config = settings.get_llm_config()

        # Default provider is openai, but no API key
        assert llm_config["provider"] == "openai"
        assert llm_config["api_key"] is None  # Should handle gracefully

    def test_configuration_inheritance(self):
        """Test that environment-specific settings inherit properly."""
        base_settings = Settings(port=9000)  # Custom port

        # Production should inherit the custom port but override other settings
        with patch.dict(os.environ, {
            'DEBUG_MODE': 'false',  # Override test environment
            'LOG_LEVEL': 'INFO',
            'ENABLE_DOCS': 'false',
            'HOLMES_ENABLE_DEBUG': 'false'
        }, clear=False):
            prod_settings = ProductionSettings(port=9000)
            assert prod_settings.port == 9000
            assert prod_settings.debug_mode is False  # Production override

        # Development should inherit and have its own overrides
        dev_settings = DevelopmentSettings(port=9000)
        assert dev_settings.port == 9000
        assert dev_settings.debug_mode is True  # Development override
