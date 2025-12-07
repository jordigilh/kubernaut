"""
Integration tests for ConfigMap Hot-Reload functionality.

BR-HAPI-199: ConfigMap Hot-Reload
DD-HAPI-004: ConfigMap Hot-Reload Design

These tests verify the end-to-end hot-reload behavior.
"""

import os
import tempfile
import time

import pytest
from fastapi.testclient import TestClient


class TestHotReloadIntegration:
    """Integration tests for hot-reload functionality."""

    def test_app_starts_with_config_manager(self):
        """
        BR-HAPI-199: ConfigManager should initialize on app startup.
        """
        # Create temp config file
        config_content = """
llm:
  model: test-model
  provider: openai
log_level: DEBUG
"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        try:
            # Set config file path
            os.environ["CONFIG_FILE"] = config_path
            os.environ["HOT_RELOAD_ENABLED"] = "true"

            # Import and create app (re-import to pick up new env)
            # Note: This is a simplified test - full integration would use subprocess
            from src.config.hot_reload import ConfigManager
            import logging

            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()

            assert manager.get_llm_model() == "test-model"
            assert manager.get_llm_provider() == "openai"
            assert manager.get_log_level() == "DEBUG"

            manager.stop()
        finally:
            os.unlink(config_path)
            # Clean up env vars
            if "CONFIG_FILE" in os.environ:
                del os.environ["CONFIG_FILE"]
            if "HOT_RELOAD_ENABLED" in os.environ:
                del os.environ["HOT_RELOAD_ENABLED"]

    def test_config_reload_reflects_in_getters(self):
        """
        BR-HAPI-199: Configuration changes should be reflected in getters.
        """
        from src.config.hot_reload import ConfigManager
        import logging

        # Initial config
        config_content = """
llm:
  model: gpt-4
  temperature: 0.7
toolsets:
  kubernetes/core: {}
"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()

            # Verify initial values
            assert manager.get_llm_model() == "gpt-4"
            assert manager.get_llm_temperature() == 0.7
            assert "kubernetes/core" in manager.get_toolsets()

            # Update config
            time.sleep(1.5)  # Wait for poll interval
            with open(config_path, 'w') as f:
                f.write("""
llm:
  model: claude-3-5-sonnet
  temperature: 0.3
toolsets:
  kubernetes/core: {}
  workflow/catalog: {}
""")
            time.sleep(1.5)  # Wait for reload

            # Verify updated values
            assert manager.get_llm_model() == "claude-3-5-sonnet"
            assert manager.get_llm_temperature() == 0.3
            assert "workflow/catalog" in manager.get_toolsets()

            manager.stop()
        finally:
            os.unlink(config_path)

    def test_hot_reload_disabled_via_env(self):
        """
        BR-HAPI-199: Hot-reload can be disabled via environment variable.
        """
        from src.config.hot_reload import ConfigManager
        import logging

        config_content = """
llm:
  model: static-model
"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger, enable_hot_reload=False)
            manager.start()

            # Initial value
            assert manager.get_llm_model() == "static-model"

            # Update config
            time.sleep(0.5)
            with open(config_path, 'w') as f:
                f.write("""
llm:
  model: changed-model
""")
            time.sleep(1.5)

            # Should NOT reload (hot-reload disabled)
            assert manager.get_llm_model() == "static-model"

            manager.stop()
        finally:
            os.unlink(config_path)

    def test_graceful_degradation_on_invalid_yaml(self):
        """
        BR-HAPI-199: Invalid YAML should not crash the service.
        """
        from src.config.hot_reload import ConfigManager
        import logging

        config_content = """
llm:
  model: initial-model
"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()

            # Initial value
            assert manager.get_llm_model() == "initial-model"
            initial_error_count = manager.error_count

            # Write invalid YAML
            time.sleep(1.5)
            with open(config_path, 'w') as f:
                f.write("invalid: yaml: syntax: {{")
            time.sleep(1.5)

            # Should keep previous config
            assert manager.get_llm_model() == "initial-model"

            # Error count should increase
            assert manager.error_count > initial_error_count

            # Service should still be functional
            # (verify by writing valid config again)
            with open(config_path, 'w') as f:
                f.write("""
llm:
  model: recovered-model
""")
            time.sleep(1.5)

            # Should pick up the valid config
            assert manager.get_llm_model() == "recovered-model"

            manager.stop()
        finally:
            os.unlink(config_path)


class TestMetricsIntegration:
    """Integration tests for hot-reload metrics."""

    def test_metrics_record_reload(self):
        """
        BR-HAPI-199: Metrics should track reload events.
        """
        from src.middleware.metrics import record_config_reload

        # Should not raise
        record_config_reload(success=True)
        record_config_reload(success=False)

    def test_metrics_available_in_endpoint(self):
        """
        BR-HAPI-199: Hot-reload metrics should be exposed at /metrics.
        """
        from src.middleware.metrics import metrics_endpoint

        response = metrics_endpoint()
        content = response.body.decode('utf-8')

        # Verify our metrics are in the output
        assert 'holmesgpt_config_reload_total' in content
        assert 'holmesgpt_config_reload_errors_total' in content
        assert 'holmesgpt_config_last_reload_timestamp' in content


