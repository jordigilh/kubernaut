"""
Tests for Mock LLM file-based configuration loading.

DD-TEST-011 v2.0: File-Based Configuration Pattern
Tests validate that Mock LLM correctly loads workflow UUIDs from YAML files.
"""

import tempfile
import os
import pytest
import yaml
from pathlib import Path

# Add src to path for imports
import sys
sys.path.insert(0, str(Path(__file__).parent.parent))

from src.server import load_scenarios_from_file, MOCK_SCENARIOS


class TestFileBasedConfigLoading:
    """Test suite for file-based configuration loading."""

    def setup_method(self):
        """Reset scenarios to defaults before each test."""
        # Reset scenarios to default UUIDs
        MOCK_SCENARIOS["oomkilled"].workflow_id = "21053597-2865-572b-89bf-de49b5b685da"
        MOCK_SCENARIOS["crashloop"].workflow_id = "30152a85-3975-682c-8ae8-cf50b6c796eb"

    def _minimal_complete_config(self):
        """Config with all workflow names required by validation (DD-TEST-011)."""
        return {
            'oomkill-increase-memory-v1:production': 'test-uuid-123',
            'crashloop-config-fix-v1:production': 'test-uuid-456',
            'node-drain-reboot-v1:production': 'test-uuid-789',
            'test-signal-handler-v1:test': 'test-uuid-abc',
            'generic-restart-v1:production': 'test-uuid-def',
        }

    def test_load_scenarios_from_valid_yaml_file(self):
        """
        Test loading scenarios from a valid YAML configuration file.

        Validates:
        - File parsing succeeds
        - Scenarios are updated with new UUIDs
        - Function returns True on success
        """
        # Create temp config file (must include all workflow names for validation)
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            config = {'scenarios': self._minimal_complete_config()}
            yaml.dump(config, f)
            config_path = f.name

        try:
            # Load scenarios
            result = load_scenarios_from_file(config_path)

            # Verify success
            assert result is True, "load_scenarios_from_file should return True on success"

            # Verify scenarios updated
            assert MOCK_SCENARIOS['oomkilled'].workflow_id == 'test-uuid-123', \
                "oomkilled scenario should use UUID from config file"
            assert MOCK_SCENARIOS['crashloop'].workflow_id == 'test-uuid-456', \
                "crashloop scenario should use UUID from config file"
        finally:
            os.unlink(config_path)

    def test_load_scenarios_with_multiple_environments(self):
        """
        Test loading scenarios for different environments (production, staging, test).

        Validates:
        - Environment matching works correctly
        - Production scenarios use production UUIDs
        - Test scenarios use test UUIDs
        """
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            config = {
                'scenarios': {
                    'oomkill-increase-memory-v1:production': 'prod-uuid-123',
                    'crashloop-config-fix-v1:production': 'prod-uuid-456',
                    'node-drain-reboot-v1:production': 'prod-uuid-aaa',
                    'test-signal-handler-v1:test': 'test-uuid-bbb',
                    'generic-restart-v1:production': 'prod-uuid-ccc',
                }
            }
            yaml.dump(config, f)
            config_path = f.name

        try:
            result = load_scenarios_from_file(config_path)
            assert result is True

            # Production scenarios use production UUIDs
            assert MOCK_SCENARIOS['oomkilled'].workflow_id == 'prod-uuid-123'
            assert MOCK_SCENARIOS['crashloop'].workflow_id == 'prod-uuid-456'
        finally:
            os.unlink(config_path)

    def test_load_scenarios_file_not_found(self):
        """
        Test handling of missing configuration file.

        Validates:
        - Function returns False when file doesn't exist
        - Scenarios retain default UUIDs
        - No exceptions raised
        """
        # Use non-existent path
        result = load_scenarios_from_file("/tmp/nonexistent-config-12345.yaml")

        # Verify graceful failure
        assert result is False, "load_scenarios_from_file should return False when file not found"

        # Verify scenarios unchanged (still have defaults)
        assert MOCK_SCENARIOS['oomkilled'].workflow_id == "21053597-2865-572b-89bf-de49b5b685da"

    def test_load_scenarios_invalid_yaml(self):
        """
        Test handling of malformed YAML configuration file.

        Validates:
        - Function returns False on YAML parsing error
        - Scenarios retain default UUIDs
        - No exceptions propagate to caller
        """
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("invalid: yaml: content: {[")  # Malformed YAML
            config_path = f.name

        try:
            result = load_scenarios_from_file(config_path)

            # Verify graceful failure
            assert result is False, "load_scenarios_from_file should return False on invalid YAML"

            # Verify scenarios unchanged
            assert MOCK_SCENARIOS['oomkilled'].workflow_id == "21053597-2865-572b-89bf-de49b5b685da"
        finally:
            os.unlink(config_path)

    def test_load_scenarios_missing_scenarios_key(self):
        """
        Test handling of valid YAML but missing 'scenarios' key.

        Validates:
        - Function returns False when scenarios key missing
        - Scenarios retain default UUIDs
        """
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            config = {'other_key': 'value'}  # Valid YAML but no 'scenarios'
            yaml.dump(config, f)
            config_path = f.name

        try:
            result = load_scenarios_from_file(config_path)

            # Verify graceful failure
            assert result is False, "load_scenarios_from_file should return False when 'scenarios' key missing"

            # Verify scenarios unchanged
            assert MOCK_SCENARIOS['oomkilled'].workflow_id == "21053597-2865-572b-89bf-de49b5b685da"
        finally:
            os.unlink(config_path)

    def test_load_scenarios_empty_file(self):
        """
        Test handling of empty configuration file.

        Validates:
        - Function returns False for empty file
        - Scenarios retain default UUIDs
        """
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            # Write empty file
            config_path = f.name

        try:
            result = load_scenarios_from_file(config_path)

            # Verify graceful failure
            assert result is False, "load_scenarios_from_file should return False for empty file"
        finally:
            os.unlink(config_path)

    def test_load_scenarios_invalid_key_format(self):
        """
        Test handling of invalid workflow key format in config file.

        Validates:
        - Function handles invalid key format gracefully
        - Valid entries still processed
        - Invalid entries skipped with warning
        """
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            config = {
                'scenarios': {
                    'oomkill-increase-memory-v1:production': 'valid-uuid-123',
                    'invalid-key-without-env': 'should-be-skipped',  # Invalid format
                    'crashloop-config-fix-v1:production': 'valid-uuid-456',
                    'node-drain-reboot-v1:production': 'valid-uuid-789',
                    'test-signal-handler-v1:test': 'valid-uuid-abc',
                    'generic-restart-v1:production': 'valid-uuid-def',
                }
            }
            yaml.dump(config, f)
            config_path = f.name

        try:
            result = load_scenarios_from_file(config_path)

            # Should succeed despite invalid entry
            assert result is True

            # Valid entries should be loaded
            assert MOCK_SCENARIOS['oomkilled'].workflow_id == 'valid-uuid-123'
            assert MOCK_SCENARIOS['crashloop'].workflow_id == 'valid-uuid-456'
        finally:
            os.unlink(config_path)

    def test_load_scenarios_partial_match(self):
        """
        Test loading when config file has all required workflows.
        Scenarios sharing workflow_name (e.g. oomkilled + oomkilled_predictive) get same UUID.

        Validates:
        - All scenarios with workflow_name get UUIDs (DD-TEST-011 validation)
        - Multiple scenarios can share same workflow_name and receive same UUID
        """
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            scenarios = self._minimal_complete_config()
            scenarios['oomkill-increase-memory-v1:production'] = 'partial-uuid-123'
            config = {'scenarios': scenarios}
            yaml.dump(config, f)
            config_path = f.name

        try:
            result = load_scenarios_from_file(config_path)
            assert result is True

            # oomkilled and oomkilled_predictive share workflow_name, both get same UUID
            assert MOCK_SCENARIOS['oomkilled'].workflow_id == 'partial-uuid-123'
            assert MOCK_SCENARIOS['oomkilled_predictive'].workflow_id == 'partial-uuid-123'

            # crashloop gets its own UUID
            assert MOCK_SCENARIOS['crashloop'].workflow_id == 'test-uuid-456'
        finally:
            os.unlink(config_path)

    def test_load_scenarios_e2e_realistic(self):
        """
        Test realistic E2E scenario: Full config with all standard workflows.

        Validates:
        - Complete E2E configuration loads successfully
        - All standard scenarios updated
        - Multiple environments handled correctly
        """
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            config = {
                'scenarios': {
                    # Production workflows
                    'oomkill-increase-memory-v1:production': '42b90a37-0d1b-5561-911a-2939ed9e1c30',
                    'crashloop-config-fix-v1:production': '5e8f2a1b-3c7d-4e0f-9a6b-1d2c3e4f5a6b',
                    'node-drain-reboot-v1:production': '6f9a3b2c-4d8e-5f1a-0b7c-2e3f4a5b6c7d',
                    'generic-restart-v1:production': '8b1c5d4e-6f0a-7b3c-2d9e-4f5a6b7c8d9e',
                    # Test workflows
                    'test-signal-handler-v1:test': 'test-uuid-e2e-789',
                }
            }
            yaml.dump(config, f)
            config_path = f.name

        try:
            result = load_scenarios_from_file(config_path)
            assert result is True

            # Verify all production scenarios loaded
            assert MOCK_SCENARIOS['oomkilled'].workflow_id == '42b90a37-0d1b-5561-911a-2939ed9e1c30'
            assert MOCK_SCENARIOS['crashloop'].workflow_id == '5e8f2a1b-3c7d-4e0f-9a6b-1d2c3e4f5a6b'
            assert MOCK_SCENARIOS['node_not_ready'].workflow_id == '6f9a3b2c-4d8e-5f1a-0b7c-2e3f4a5b6c7d'
            assert MOCK_SCENARIOS['low_confidence'].workflow_id == '8b1c5d4e-6f0a-7b3c-2d9e-4f5a6b7c8d9e'
        finally:
            os.unlink(config_path)


class TestConfigFileIntegration:
    """Integration tests for file-based configuration in realistic scenarios."""

    def test_configmap_mount_simulation(self):
        """
        Simulate Kubernetes ConfigMap mount scenario.

        Validates:
        - Mock LLM can read from /tmp path (simulating /config mount)
        - Workflow UUIDs from DataStorage correctly sync via file
        """
        # Simulate ConfigMap mount path
        config_dir = "/tmp/mock-llm-test-config"
        os.makedirs(config_dir, exist_ok=True)
        config_path = os.path.join(config_dir, "scenarios.yaml")

        try:
            # Write config (simulating kubectl apply) - full config required by DD-TEST-011
            with open(config_path, 'w') as f:
                config = {
                    'scenarios': {
                        'oomkill-increase-memory-v1:production': 'configmap-uuid-123',
                        'crashloop-config-fix-v1:production': 'configmap-uuid-456',
                        'node-drain-reboot-v1:production': 'configmap-uuid-789',
                        'test-signal-handler-v1:test': 'configmap-uuid-abc',
                        'generic-restart-v1:production': 'configmap-uuid-def',
                    }
                }
                yaml.dump(config, f)

            # Load config (simulating Mock LLM startup)
            result = load_scenarios_from_file(config_path)
            assert result is True

            # Verify scenario loaded
            assert MOCK_SCENARIOS['oomkilled'].workflow_id == 'configmap-uuid-123'
        finally:
            os.unlink(config_path)
            os.rmdir(config_dir)

    def test_integration_test_direct_file(self):
        """
        Simulate integration test scenario: Direct file write.

        Validates:
        - Integration tests can write config directly to filesystem
        - Mock LLM loads config without Kubernetes
        """
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            config = {
                'scenarios': {
                    'oomkill-increase-memory-v1:production': 'integration-uuid-789',
                    'crashloop-config-fix-v1:production': 'integration-uuid-456',
                    'node-drain-reboot-v1:production': 'integration-uuid-aaa',
                    'test-signal-handler-v1:test': 'integration-uuid-bbb',
                    'generic-restart-v1:production': 'integration-uuid-ccc',
                }
            }
            yaml.dump(config, f)
            config_path = f.name

        try:
            # Simulate integration test setup
            result = load_scenarios_from_file(config_path)
            assert result is True

            # Verify scenario loaded for integration test
            assert MOCK_SCENARIOS['oomkilled'].workflow_id == 'integration-uuid-789'
        finally:
            os.unlink(config_path)


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
