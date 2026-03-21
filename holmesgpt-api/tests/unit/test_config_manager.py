"""
Unit tests for ConfigManager - Thread-safe configuration management with hot-reload.

BR-HAPI-199: ConfigMap Hot-Reload
DD-HAPI-004: ConfigMap Hot-Reload Design

TDD Phase: Tests for ConfigManager class.
"""

import logging
import os
import tempfile
import threading
import time

import pytest


class TestConfigManagerInitialization:
    """Test ConfigManager initialization and lifecycle."""

    def test_config_manager_loads_initial_config(self):
        """
        BR-HAPI-199: Service SHALL reload configuration from ConfigMap.

        ConfigManager should load initial config on start().
        """
        from src.config.hot_reload import ConfigManager

        config_content = """
llm:
  model: gpt-4
  provider: openai
  temperature: 0.7
toolsets:
  kubernetes/core: {}
log_level: INFO
"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()

            assert manager.get_llm_model() == "gpt-4"
            assert manager.get_llm_provider() == "openai"
            assert manager.get_llm_temperature() == 0.7
            assert "kubernetes/core" in manager.get_toolsets()
            assert manager.get_log_level() == "INFO"

            manager.stop()
        finally:
            os.unlink(config_path)

    def test_config_manager_uses_defaults_for_missing_fields(self):
        """
        BR-HAPI-199: Service SHALL gracefully degrade on invalid configuration.

        ConfigManager should use defaults for missing fields.
        """
        from src.config.hot_reload import ConfigManager

        # Minimal config - missing most fields
        config_content = """
llm:
  model: claude-3
"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()

            # Specified value
            assert manager.get_llm_model() == "claude-3"

            # Defaults
            assert manager.get_llm_provider() == "openai"  # Default
            assert manager.get_llm_max_retries() == 3  # Default
            assert manager.get_llm_timeout() == 60  # Default
            assert manager.get_log_level() == "INFO"  # Default

            manager.stop()
        finally:
            os.unlink(config_path)

    def test_config_manager_stop_cleanup(self):
        """
        BR-HAPI-199: ConfigManager.stop() should clean up resources.
        """
        from src.config.hot_reload import ConfigManager

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("llm:\n  model: test\n")
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()
            manager.stop()

            # Should be able to call stop multiple times without error
            manager.stop()
        finally:
            os.unlink(config_path)


class TestConfigManagerGetters:
    """Test ConfigManager typed getters."""

    @pytest.fixture
    def config_manager(self):
        """Create a ConfigManager with full config for testing (hot-reload disabled for speed)."""
        from src.config.hot_reload import ConfigManager

        config_content = """
llm:
  model: gpt-4-turbo
  provider: vertex_ai
  endpoint: http://custom-endpoint:8080
  max_retries: 5
  timeout_seconds: 120
  temperature: 0.5
  max_tokens_per_request: 8192
toolsets:
  kubernetes/core: {}
  kubernetes/logs: {}
  workflow/catalog:
    enabled: true
log_level: DEBUG
"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        logger = logging.getLogger("test")
        # Disable hot-reload for getter tests - no need for FileWatcher thread
        manager = ConfigManager(config_path, logger, enable_hot_reload=False)
        manager.start()

        yield manager

        manager.stop()
        os.unlink(config_path)

    def test_get_llm_model(self, config_manager):
        """BR-HAPI-199: get_llm_model() returns current LLM model."""
        assert config_manager.get_llm_model() == "gpt-4-turbo"

    def test_get_llm_provider(self, config_manager):
        """BR-HAPI-199: get_llm_provider() returns current LLM provider."""
        assert config_manager.get_llm_provider() == "vertex_ai"

    def test_get_llm_endpoint(self, config_manager):
        """BR-HAPI-199: get_llm_endpoint() returns custom endpoint."""
        assert config_manager.get_llm_endpoint() == "http://custom-endpoint:8080"

    def test_get_llm_max_retries(self, config_manager):
        """BR-HAPI-199: get_llm_max_retries() returns retry count."""
        assert config_manager.get_llm_max_retries() == 5

    def test_get_llm_timeout(self, config_manager):
        """BR-HAPI-199: get_llm_timeout() returns timeout in seconds."""
        assert config_manager.get_llm_timeout() == 120

    def test_get_llm_temperature(self, config_manager):
        """BR-HAPI-199: get_llm_temperature() returns temperature."""
        assert config_manager.get_llm_temperature() == 0.5

    def test_get_toolsets(self, config_manager):
        """BR-HAPI-199: get_toolsets() returns toolsets configuration."""
        toolsets = config_manager.get_toolsets()
        assert "kubernetes/core" in toolsets
        assert "kubernetes/logs" in toolsets
        assert "workflow/catalog" in toolsets
        assert toolsets["workflow/catalog"]["enabled"] is True

    def test_get_log_level(self, config_manager):
        """BR-HAPI-199: get_log_level() returns log level."""
        assert config_manager.get_log_level() == "DEBUG"

    def test_get_raw_config(self, config_manager):
        """BR-HAPI-199: get_raw_config() returns full config dict."""
        raw = config_manager.get_raw_config()
        assert "llm" in raw
        assert "toolsets" in raw
        assert raw["llm"]["model"] == "gpt-4-turbo"


class TestConfigManagerHotReload:
    """Test ConfigManager hot-reload behavior."""

    def test_config_reload_updates_values(self, wait_for):
        """
        BR-HAPI-199: Configuration reload latency < 90 seconds.

        ConfigManager should reflect updated values after file change.
        """
        from src.config.hot_reload import ConfigManager

        config_content = """
llm:
  model: gpt-4
  temperature: 0.7
"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()

            # Initial values
            assert manager.get_llm_model() == "gpt-4"
            assert manager.get_llm_temperature() == 0.7

            # Update config file
            with open(config_path, 'w') as f:
                f.write("""
llm:
  model: claude-3-5-sonnet
  temperature: 0.3
""")

            # Wait for reload (typically <100ms instead of 3s with 2× sleep(1.5))
            wait_for(lambda: manager.get_llm_model() == "claude-3-5-sonnet", timeout=2.0, error_msg="Config should reload")

            # Values should be updated
            assert manager.get_llm_temperature() == 0.3

            manager.stop()
        finally:
            os.unlink(config_path)

    def test_invalid_yaml_keeps_previous_config(self, wait_for):
        """
        BR-HAPI-199: Service SHALL gracefully degrade on invalid configuration.

        Invalid YAML should not crash the service - keep previous config.
        """
        from src.config.hot_reload import ConfigManager

        config_content = """
llm:
  model: gpt-4
"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()

            # Initial value
            assert manager.get_llm_model() == "gpt-4"

            # Write invalid YAML
            with open(config_path, 'w') as f:
                f.write("this: is: not: valid: yaml: {{")

            # Wait for error to be detected (typically <100ms instead of 3s with 2× sleep(1.5))
            wait_for(lambda: manager.error_count >= 1, timeout=2.0, error_msg="Error should be detected")

            # Should keep previous config
            assert manager.get_llm_model() == "gpt-4"

            manager.stop()
        finally:
            os.unlink(config_path)


class TestConfigManagerThreadSafety:
    """Test ConfigManager thread safety."""

    def test_concurrent_reads_are_safe(self):
        """
        BR-HAPI-199: Thread-safe configuration access required.

        Multiple threads reading config simultaneously should be safe.
        """
        from src.config.hot_reload import ConfigManager

        config_content = """
llm:
  model: gpt-4
  provider: openai
"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()

            results = []
            errors = []

            def read_config():
                try:
                    for _ in range(100):
                        model = manager.get_llm_model()
                        provider = manager.get_llm_provider()
                        results.append((model, provider))
                except Exception as e:
                    errors.append(e)

            # Start multiple reader threads
            threads = [threading.Thread(target=read_config) for _ in range(5)]
            for t in threads:
                t.start()
            for t in threads:
                t.join()

            manager.stop()

            # No errors should occur
            assert len(errors) == 0
            # All reads should return valid values
            assert len(results) == 500
            for model, provider in results:
                assert model == "gpt-4"
                assert provider == "openai"
        finally:
            os.unlink(config_path)


class TestConfigManagerMetrics:
    """Test ConfigManager metrics."""

    def test_reload_count_increments(self, wait_for):
        """
        BR-HAPI-199: Metrics exposed for reload count and errors.
        """
        from src.config.hot_reload import ConfigManager

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("llm:\n  model: v1\n")
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()

            initial_count = manager.reload_count

            # Trigger reload
            with open(config_path, 'w') as f:
                f.write("llm:\n  model: v2\n")

            # Wait for reload count to increment (typically <100ms instead of 3s with 2× sleep(1.5))
            wait_for(lambda: manager.reload_count > initial_count, timeout=2.0, error_msg="Reload count should increment")

            manager.stop()
        finally:
            os.unlink(config_path)

    def test_last_hash_updates(self, wait_for):
        """
        BR-HAPI-199: Configuration hash logged on reload for audit trail.
        """
        from src.config.hot_reload import ConfigManager

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("llm:\n  model: v1\n")
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger)
            manager.start()

            initial_hash = manager.last_hash
            assert len(initial_hash) > 0

            # Trigger reload
            with open(config_path, 'w') as f:
                f.write("llm:\n  model: v2\n")

            # Wait for hash to update (typically <100ms instead of 3s with 2× sleep(1.5))
            wait_for(lambda: manager.last_hash != initial_hash, timeout=2.0, error_msg="Hash should update after reload")

            manager.stop()
        finally:
            os.unlink(config_path)


class TestConfigManagerDisableHotReload:
    """Test ConfigManager with hot-reload disabled."""

    def test_disable_hot_reload(self):
        """
        BR-HAPI-199: Hot-reload can be disabled via enable_hot_reload=False.
        """
        from src.config.hot_reload import ConfigManager

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("llm:\n  model: static\n")
            config_path = f.name

        try:
            logger = logging.getLogger("test")
            manager = ConfigManager(config_path, logger, enable_hot_reload=False)
            manager.start()

            # Initial value loaded
            assert manager.get_llm_model() == "static"

            # Change file
            with open(config_path, 'w') as f:
                f.write("llm:\n  model: changed\n")

            # Brief wait to ensure no reload happens (reduced from 2s to 0.2s - hot reload disabled)
            time.sleep(0.2)

            # Should NOT reload (hot-reload disabled)
            assert manager.get_llm_model() == "static"

            manager.stop()
        finally:
            os.unlink(config_path)


class TestConfigManagerSDKMerge:
    """Test ConfigManager SDK config merge for accurate toolset reporting.

    Issue #470: HAPI startup reports toolsets=[] despite SDK config containing
    prometheus/metrics toolset. ConfigManager must merge SDK config so
    DD-HAPI-004 log accurately reports toolsets.

    BR-HAPI-199: ConfigMap Hot-Reload
    DD-HAPI-004: ConfigMap Hot-Reload Design
    """

    def test_log_reports_sdk_toolsets(self, caplog):
        """
        UT-HAPI-470-001: DD-HAPI-004 log reports SDK toolsets after startup.

        Given: Main config has no toolsets, SDK config has prometheus/metrics
        When: ConfigManager starts with both paths
        Then: DD-HAPI-004 log contains 'prometheus/metrics'
        """
        from src.config.hot_reload import ConfigManager

        main_content = "llm:\n  model: gpt-4\n"
        sdk_content = (
            "llm:\n  model: gpt-4\n"
            "toolsets:\n  prometheus/metrics:\n    enabled: true\n"
        )

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as mf:
            mf.write(main_content)
            main_path = mf.name
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as sf:
            sf.write(sdk_content)
            sdk_path = sf.name

        try:
            logger = logging.getLogger("test-470-001")
            with caplog.at_level(logging.INFO, logger="test-470-001"):
                manager = ConfigManager(
                    main_path, logger,
                    sdk_config_path=sdk_path,
                    enable_hot_reload=False,
                )
                manager.start()

            assert "prometheus/metrics" in caplog.text, (
                f"UT-HAPI-470-001: DD-HAPI-004 log must report SDK toolsets, got: {caplog.text}"
            )
            manager.stop()
        finally:
            os.unlink(main_path)
            os.unlink(sdk_path)

    def test_get_toolsets_returns_merged(self):
        """
        UT-HAPI-470-002: get_toolsets() returns merged main + SDK toolsets.

        Given: Main config has kubernetes/core, SDK config has prometheus/metrics
        When: ConfigManager starts with both paths
        Then: get_toolsets() returns both
        """
        from src.config.hot_reload import ConfigManager

        main_content = (
            "llm:\n  model: gpt-4\n"
            "toolsets:\n  kubernetes/core: {}\n"
        )
        sdk_content = (
            "llm:\n  model: gpt-4\n"
            "toolsets:\n  prometheus/metrics:\n    enabled: true\n"
        )

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as mf:
            mf.write(main_content)
            main_path = mf.name
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as sf:
            sf.write(sdk_content)
            sdk_path = sf.name

        try:
            logger = logging.getLogger("test-470-002")
            manager = ConfigManager(
                main_path, logger,
                sdk_config_path=sdk_path,
                enable_hot_reload=False,
            )
            manager.start()

            toolsets = manager.get_toolsets()
            assert "kubernetes/core" in toolsets, (
                "UT-HAPI-470-002: main config toolset must be present"
            )
            assert "prometheus/metrics" in toolsets, (
                "UT-HAPI-470-002: SDK config toolset must be present"
            )
            manager.stop()
        finally:
            os.unlink(main_path)
            os.unlink(sdk_path)

    def test_hot_reload_preserves_sdk_toolsets(self, wait_for):
        """
        UT-HAPI-470-003: Hot-reload of main config preserves SDK toolsets.

        Given: ConfigManager running with merged SDK toolsets
        When: Main config file is updated (llm.model changes)
        Then: get_toolsets() still contains SDK toolsets after reload
        """
        from src.config.hot_reload import ConfigManager

        main_content = "llm:\n  model: gpt-4\n"
        sdk_content = (
            "llm:\n  provider: openai\n"
            "toolsets:\n  prometheus/metrics:\n    enabled: true\n"
        )

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as mf:
            mf.write(main_content)
            main_path = mf.name
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as sf:
            sf.write(sdk_content)
            sdk_path = sf.name

        try:
            logger = logging.getLogger("test-470-003")
            manager = ConfigManager(
                main_path, logger,
                sdk_config_path=sdk_path,
            )
            manager.start()

            assert "prometheus/metrics" in manager.get_toolsets()

            with open(main_path, 'w') as f:
                f.write("llm:\n  model: claude-3-5-sonnet\n")

            wait_for(
                lambda: manager.get_llm_model() == "claude-3-5-sonnet",
                timeout=5.0,
                error_msg="Main config should reload",
            )

            toolsets = manager.get_toolsets()
            assert "prometheus/metrics" in toolsets, (
                "UT-HAPI-470-003: SDK toolsets must survive main config reload"
            )
            manager.stop()
        finally:
            os.unlink(main_path)
            os.unlink(sdk_path)

    def test_missing_sdk_path_graceful_degradation(self, caplog):
        """
        UT-HAPI-470-004: Missing/empty SDK path -- graceful degradation.

        Given: Main config exists, SDK path is empty or non-existent file
        When: ConfigManager starts
        Then: No exception, main config toolsets only, warning logged
        """
        from src.config.hot_reload import ConfigManager

        main_content = (
            "llm:\n  model: gpt-4\n"
            "toolsets:\n  kubernetes/core: {}\n"
        )

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as mf:
            mf.write(main_content)
            main_path = mf.name

        try:
            logger = logging.getLogger("test-470-004")
            with caplog.at_level(logging.WARNING, logger="test-470-004"):
                manager = ConfigManager(
                    main_path, logger,
                    sdk_config_path="/nonexistent/sdk-config.yaml",
                    enable_hot_reload=False,
                )
                manager.start()

            assert "kubernetes/core" in manager.get_toolsets(), (
                "UT-HAPI-470-004: main config toolsets must still be available"
            )
            assert "SDK config merge skipped" in caplog.text or "nonexistent" in caplog.text, (
                f"UT-HAPI-470-004: warning must be logged for missing SDK, got: {caplog.text}"
            )
            manager.stop()
        finally:
            os.unlink(main_path)

    def test_invalid_sdk_file_graceful_degradation(self, caplog):
        """
        UT-HAPI-470-005: Invalid SDK file -- graceful degradation.

        Given: Main config exists, SDK file contains invalid YAML
        When: ConfigManager starts
        Then: No exception, main config toolsets only, warning logged
        """
        from src.config.hot_reload import ConfigManager

        main_content = (
            "llm:\n  model: gpt-4\n"
            "toolsets:\n  kubernetes/core: {}\n"
        )
        sdk_content = "not: valid: yaml: {{"

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as mf:
            mf.write(main_content)
            main_path = mf.name
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as sf:
            sf.write(sdk_content)
            sdk_path = sf.name

        try:
            logger = logging.getLogger("test-470-005")
            with caplog.at_level(logging.WARNING, logger="test-470-005"):
                manager = ConfigManager(
                    main_path, logger,
                    sdk_config_path=sdk_path,
                    enable_hot_reload=False,
                )
                manager.start()

            assert "kubernetes/core" in manager.get_toolsets(), (
                "UT-HAPI-470-005: main config toolsets must still be available"
            )
            assert "SDK config merge skipped" in caplog.text, (
                f"UT-HAPI-470-005: warning must be logged for invalid SDK, got: {caplog.text}"
            )
            manager.stop()
        finally:
            os.unlink(main_path)
            os.unlink(sdk_path)

