"""
Unit tests for SDK ConfigMap split (Issue #390).

Tests validate business outcomes of two-file config loading:
- Operator gets merged config when both files exist
- SDK llm values override main config
- Toolsets and MCP servers from SDK are available to LLM
- Missing/empty SDK config causes fail-fast with clear errors

Test Plan: docs/tests/390/TEST_PLAN.md
Authority: Issue #390 (Option B+C: Split ConfigMaps with External Reference)
"""

import os
import tempfile

import pytest
import yaml


def _write_yaml(content: dict) -> str:
    """Write a dict as YAML to a temp file and return the path."""
    f = tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False)
    yaml.dump(content, f, default_flow_style=False)
    f.close()
    return f.name


def _make_main_config(**overrides) -> dict:
    """Build a minimal main config dict for testing."""
    base = {
        "logging": {"level": "INFO"},
        "data_storage": {"url": "http://ds:8080"},
        "audit": {
            "flush_interval_seconds": 0.1,
            "buffer_size": 10000,
            "batch_size": 50,
        },
    }
    base.update(overrides)
    return base


def _make_sdk_config(**overrides) -> dict:
    """Build a minimal SDK config dict for testing."""
    base = {
        "llm": {
            "provider": "openai",
            "model": "gpt-4-turbo",
            "endpoint": "http://llm:8080",
            "max_retries": 3,
            "timeout_seconds": 120,
            "temperature": 0.7,
        },
    }
    base.update(overrides)
    return base


def _call_merge_sdk_config(main_config: dict, sdk_path: str) -> dict:
    """Call the SDK config merge function under test.

    This imports from src.config.sdk_loader to avoid the heavy src.main
    import chain (which pulls in holmes SDK).
    """
    from src.config.sdk_loader import merge_sdk_config
    return merge_sdk_config(main_config, sdk_path)


class TestSDKConfigLoaded:
    """UT-HAPI-390-001: SDK config loaded and merged into app_config."""

    def test_merged_config_contains_keys_from_both_files(self):
        """Operator gets merged config when both service and SDK config files exist."""
        main_cfg = _make_main_config()
        sdk_cfg = _make_sdk_config()
        sdk_path = _write_yaml(sdk_cfg)
        try:
            config = _call_merge_sdk_config(main_cfg, sdk_path)

            assert config["logging"]["level"] == "INFO", "Main config logging preserved"
            assert config["data_storage"]["url"] == "http://ds:8080", "Main config data_storage preserved"
            assert config["llm"]["provider"] == "openai", "SDK llm provider loaded"
            assert config["llm"]["model"] == "gpt-4-turbo", "SDK llm model loaded"
        finally:
            os.unlink(sdk_path)

    def test_audit_config_preserved(self):
        """Main config audit settings are preserved after SDK merge."""
        main_cfg = _make_main_config()
        sdk_cfg = _make_sdk_config()
        sdk_path = _write_yaml(sdk_cfg)
        try:
            config = _call_merge_sdk_config(main_cfg, sdk_path)

            assert config["audit"]["flush_interval_seconds"] == 0.1
            assert config["audit"]["buffer_size"] == 10000
            assert config["audit"]["batch_size"] == 50
        finally:
            os.unlink(sdk_path)


class TestSDKLLMOverride:
    """UT-HAPI-390-002: SDK llm values override main config llm values."""

    def test_sdk_llm_takes_precedence(self):
        """SDK llm settings override main config (operator controls LLM via SDK ConfigMap)."""
        main_cfg = _make_main_config(llm={"provider": "mock", "model": "mock-model", "max_retries": 5})
        sdk_cfg = _make_sdk_config(llm={"provider": "vertex_ai", "model": "claude-sonnet-4"})
        sdk_path = _write_yaml(sdk_cfg)
        try:
            config = _call_merge_sdk_config(main_cfg, sdk_path)

            assert config["llm"]["provider"] == "vertex_ai", "SDK provider overrides main"
            assert config["llm"]["model"] == "claude-sonnet-4", "SDK model overrides main"
        finally:
            os.unlink(sdk_path)

    def test_main_llm_defaults_preserved_when_not_in_sdk(self):
        """Main config defaults for keys not specified in SDK are preserved via deep merge."""
        main_cfg = _make_main_config(llm={"provider": "mock", "model": "mock-model", "max_retries": 5})
        sdk_cfg = _make_sdk_config(llm={"provider": "vertex_ai", "model": "claude-sonnet-4"})
        sdk_path = _write_yaml(sdk_cfg)
        try:
            config = _call_merge_sdk_config(main_cfg, sdk_path)

            assert config["llm"]["provider"] == "vertex_ai", "SDK provider wins"
            assert config["llm"]["max_retries"] == 5, "Main max_retries preserved (not in SDK)"
        finally:
            os.unlink(sdk_path)


class TestToolsetsAvailable:
    """UT-HAPI-390-003: Toolsets from SDK config available in app_config."""

    def test_prometheus_toolset_available(self):
        """Operator-configured Prometheus toolset is available to LLM via app_config."""
        sdk_cfg = _make_sdk_config(toolsets={
            "prometheus/metrics": {
                "enabled": True,
                "config": {"prometheus_url": "http://prom:9090"},
            }
        })
        sdk_path = _write_yaml(sdk_cfg)
        try:
            config = _call_merge_sdk_config(_make_main_config(), sdk_path)

            assert "toolsets" in config, "toolsets key must be in merged config"
            assert "prometheus/metrics" in config["toolsets"]
            prom = config["toolsets"]["prometheus/metrics"]
            assert prom["enabled"] is True
            assert prom["config"]["prometheus_url"] == "http://prom:9090"
        finally:
            os.unlink(sdk_path)

    def test_multiple_toolsets(self):
        """Multiple toolsets from SDK config are all available."""
        sdk_cfg = _make_sdk_config(toolsets={
            "prometheus/metrics": {"enabled": True, "config": {"prometheus_url": "http://prom:9090"}},
            "kubernetes/core": {"enabled": False},
        })
        sdk_path = _write_yaml(sdk_cfg)
        try:
            config = _call_merge_sdk_config(_make_main_config(), sdk_path)

            assert len(config["toolsets"]) == 2
            assert config["toolsets"]["kubernetes/core"]["enabled"] is False
        finally:
            os.unlink(sdk_path)


class TestMCPServers:
    """UT-HAPI-390-004: mcp_servers from SDK config available in app_config."""

    def test_mcp_servers_loaded(self):
        """Operator-configured MCP servers are available to LLM via app_config."""
        sdk_cfg = _make_sdk_config(mcp_servers={
            "custom": {"url": "http://mcp:8080"}
        })
        sdk_path = _write_yaml(sdk_cfg)
        try:
            config = _call_merge_sdk_config(_make_main_config(), sdk_path)

            assert "mcp_servers" in config, "mcp_servers key must be in merged config"
            assert config["mcp_servers"]["custom"]["url"] == "http://mcp:8080"
        finally:
            os.unlink(sdk_path)


class TestMissingSDKConfig:
    """UT-HAPI-390-005: Missing SDK config file causes fail-fast error."""

    def test_missing_sdk_config_raises_error(self):
        """Deployment fails fast with clear error when SDK config file missing."""
        nonexistent = "/tmp/nonexistent-sdk-config-390.yaml"
        with pytest.raises(FileNotFoundError) as exc_info:
            _call_merge_sdk_config(_make_main_config(), nonexistent)

        assert nonexistent in str(exc_info.value), "Error message contains missing path"
        assert "sdk" in str(exc_info.value).lower(), \
            "Error message mentions SDK for operator clarity"


class TestEmptySDKConfig:
    """UT-HAPI-390-006: Empty SDK config file causes fail-fast error."""

    def test_empty_sdk_config_raises_error(self):
        """Deployment fails fast with clear error when SDK config file empty."""
        sdk_path = tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False).name
        try:
            with pytest.raises(ValueError) as exc_info:
                _call_merge_sdk_config(_make_main_config(), sdk_path)

            assert "empty" in str(exc_info.value).lower(), "Error message mentions 'empty'"
            assert sdk_path in str(exc_info.value), "Error message contains the SDK config path"
        finally:
            os.unlink(sdk_path)


class TestMissingLLMKey:
    """UT-HAPI-390-007: SDK config without 'llm' key causes fail-fast error."""

    def test_sdk_without_llm_raises_error(self):
        """HAPI fails fast when SDK config exists but has no LLM configuration."""
        sdk_cfg = {"toolsets": {"prometheus/metrics": {"enabled": True}}}
        sdk_path = _write_yaml(sdk_cfg)
        try:
            with pytest.raises(ValueError) as exc_info:
                _call_merge_sdk_config(_make_main_config(), sdk_path)

            assert "llm" in str(exc_info.value).lower(), \
                "Error message mentions 'llm' for operator clarity"
            assert sdk_path in str(exc_info.value), \
                "Error message contains the SDK config path"
        finally:
            os.unlink(sdk_path)

    def test_sdk_with_only_mcp_servers_raises_error(self):
        """SDK config with only mcp_servers (no llm) also fails fast."""
        sdk_cfg = {"mcp_servers": {"custom": {"url": "http://mcp:8080"}}}
        sdk_path = _write_yaml(sdk_cfg)
        try:
            with pytest.raises(ValueError) as exc_info:
                _call_merge_sdk_config(_make_main_config(), sdk_path)

            assert "llm" in str(exc_info.value).lower()
        finally:
            os.unlink(sdk_path)


class TestLLMOnlyNoToolsets:
    """UT-HAPI-390-008: SDK config with llm only (no toolsets) is valid."""

    def test_llm_only_succeeds(self):
        """SDK config with only llm section is valid -- no extra toolsets needed."""
        sdk_cfg = {"llm": {"provider": "openai", "model": "gpt-4", "endpoint": "http://llm:8080"}}
        sdk_path = _write_yaml(sdk_cfg)
        try:
            config = _call_merge_sdk_config(_make_main_config(), sdk_path)

            assert config["llm"]["provider"] == "openai"
            assert config["llm"]["model"] == "gpt-4"
            assert "toolsets" not in config, "No toolsets when SDK has none"
            assert config["logging"]["level"] == "INFO", "Main config preserved"
        finally:
            os.unlink(sdk_path)

    def test_llm_with_empty_toolsets_succeeds(self):
        """SDK config with llm + empty toolsets is valid."""
        sdk_cfg = {"llm": {"provider": "openai", "model": "gpt-4", "endpoint": "http://llm:8080"}, "toolsets": {}}
        sdk_path = _write_yaml(sdk_cfg)
        try:
            config = _call_merge_sdk_config(_make_main_config(), sdk_path)

            assert config["llm"]["provider"] == "openai"
            assert config["toolsets"] == {}, "Empty toolsets merged as empty dict"
        finally:
            os.unlink(sdk_path)
