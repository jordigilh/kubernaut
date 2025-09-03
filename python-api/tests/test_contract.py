"""
Contract tests to ensure mocks match real interfaces.

These tests verify that test mocks and builders provide the same
interface as the actual implementation.
"""

import inspect
import pytest
from unittest.mock import Mock, AsyncMock

from app.services.holmesgpt_wrapper import HolmesGPTWrapper
from app.models.requests import (
    AlertData, HolmesOptions, KubernetesContext,
    ContextData, InvestigationContext, AskRequest, InvestigateRequest
)
from app.models.responses import (
    AskResponse, InvestigateResponse, HealthStatus, HealthCheckResponse,
    Recommendation, AnalysisResult, InvestigationResult
)
from .test_builders import (
    AlertBuilder, HolmesOptionsBuilder, AskResponseBuilder,
    create_test_alert, create_test_options, create_test_ask_response
)


class TestModelContracts:
    """Test that our builders create objects with correct interfaces."""

    def test_alert_builder_contract(self):
        """Test AlertBuilder creates valid AlertData."""
        # Create alert using builder
        alert = AlertBuilder().critical().build()

        # Verify it's the correct type
        assert isinstance(alert, AlertData)

        # Verify required attributes exist
        assert hasattr(alert, 'name')
        assert hasattr(alert, 'severity')
        assert hasattr(alert, 'status')
        assert hasattr(alert, 'starts_at')

        # Verify builder methods work
        custom_alert = (AlertBuilder()
                       .with_name("CustomAlert")
                       .with_severity("critical")
                       .with_labels({"app": "test"})
                       .build())

        assert custom_alert.name == "CustomAlert"
        assert custom_alert.severity == "critical"
        assert custom_alert.labels["app"] == "test"

    def test_options_builder_contract(self):
        """Test HolmesOptionsBuilder creates valid HolmesOptions."""
        # Create options using builder
        options = HolmesOptionsBuilder().standard().build()

        # Verify it's the correct type
        assert isinstance(options, HolmesOptions)

        # Verify expected attributes exist
        assert hasattr(options, 'max_tokens')
        assert hasattr(options, 'temperature')
        assert hasattr(options, 'timeout')
        assert hasattr(options, 'context_window')

        # Verify values are reasonable
        assert options.max_tokens > 0
        assert 0.0 <= options.temperature <= 2.0
        assert options.timeout > 0
        assert options.context_window > 0

    def test_response_builder_contract(self):
        """Test AskResponseBuilder creates valid AskResponse."""
        # Create response using builder
        response = AskResponseBuilder().mock_successful().build()

        # Verify it's the correct type
        assert isinstance(response, AskResponse)

        # Verify required attributes exist
        assert hasattr(response, 'response')
        assert hasattr(response, 'confidence')
        assert hasattr(response, 'model_used')
        assert hasattr(response, 'processing_time')

        # Verify values are reasonable
        assert isinstance(response.response, str)
        assert 0.0 <= response.confidence <= 1.0
        assert isinstance(response.model_used, str)
        assert response.processing_time >= 0.0

    def test_factory_functions_contract(self):
        """Test factory functions create valid objects."""
        # Test factory functions
        alert = create_test_alert()
        options = create_test_options()
        response = create_test_ask_response()

        # Verify types
        assert isinstance(alert, AlertData)
        assert isinstance(options, HolmesOptions)
        assert isinstance(response, AskResponse)

        # Verify they have expected interfaces
        assert hasattr(alert, 'severity')
        assert hasattr(options, 'max_tokens')
        assert hasattr(response, 'confidence')


class TestServiceContracts:
    """Test that service mocks match actual service interfaces."""

    def test_wrapper_interface_contract(self):
        """Test HolmesGPTWrapper has expected public interface."""
        from app.config import TestEnvironmentSettings

        # Create real wrapper instance
        wrapper = HolmesGPTWrapper(TestEnvironmentSettings())

        # Verify expected methods exist
        expected_methods = [
            'ask',
            'investigate',
            'health_check',
            '_prepare_ask_options',
            '_prepare_context_prompt',
            '_parse_ask_result',
            '_parse_investigate_result'
        ]

        for method_name in expected_methods:
            assert hasattr(wrapper, method_name), f"Missing method: {method_name}"
            method = getattr(wrapper, method_name)
            assert callable(method), f"Method {method_name} is not callable"

    def test_async_method_signatures(self):
        """Test async methods have correct signatures."""
        from app.config import TestEnvironmentSettings

        wrapper = HolmesGPTWrapper(TestEnvironmentSettings())

        # Check ask method signature
        ask_sig = inspect.signature(wrapper.ask)
        ask_params = list(ask_sig.parameters.keys())
        assert 'prompt' in ask_params
        assert 'context' in ask_params
        assert 'options' in ask_params

        # Check investigate method signature
        investigate_sig = inspect.signature(wrapper.investigate)
        investigate_params = list(investigate_sig.parameters.keys())
        assert 'alert' in investigate_params
        assert 'context' in investigate_params
        assert 'investigation_context' in investigate_params
        assert 'options' in investigate_params

    @pytest.mark.asyncio
    async def test_mock_wrapper_matches_real_interface(self):
        """Test that mock wrappers provide same interface as real ones."""
        from app.config import TestEnvironmentSettings

        # Create real wrapper
        real_wrapper = HolmesGPTWrapper(TestEnvironmentSettings())

        # Create mock wrapper
        mock_wrapper = AsyncMock(spec=HolmesGPTWrapper)

        # Verify mock has same interface as real
        real_methods = [name for name, method in inspect.getmembers(real_wrapper, predicate=inspect.ismethod)]
        mock_methods = [name for name, method in inspect.getmembers(mock_wrapper, predicate=inspect.ismethod)]

        # Check that major async methods are present in both
        important_methods = ['ask', 'investigate', 'health_check']
        for method in important_methods:
            assert method in real_methods, f"Real wrapper missing {method}"
            assert hasattr(mock_wrapper, method), f"Mock wrapper missing {method}"


class TestValidationContracts:
    """Test validation contracts between models and tests."""

    def test_severity_validation_contract(self):
        """Test that severity validation is consistent."""
        from .test_constants import VALID_SEVERITIES, is_valid_severity

        # Test valid severities
        for severity in VALID_SEVERITIES:
            assert is_valid_severity(severity), f"Severity {severity} should be valid"

            # Test creating alert with valid severity
            alert = AlertBuilder().with_severity(severity).build()
            assert alert.severity == severity

    def test_confidence_validation_contract(self):
        """Test that confidence validation is consistent."""
        from .test_constants import is_valid_confidence, MIN_CONFIDENCE, MAX_CONFIDENCE

        # Test valid confidence values
        test_confidences = [0.0, 0.5, 0.85, 1.0]
        for confidence in test_confidences:
            assert is_valid_confidence(confidence), f"Confidence {confidence} should be valid"

            # Test creating response with valid confidence
            response = AskResponseBuilder().with_confidence(confidence).build()
            assert response.confidence == confidence

        # Test invalid confidence values
        invalid_confidences = [-0.1, 1.1, 2.0]
        for confidence in invalid_confidences:
            assert not is_valid_confidence(confidence), f"Confidence {confidence} should be invalid"

    def test_model_field_consistency(self):
        """Test that model fields are consistent with expectations."""
        # Create objects using builders
        alert = create_test_alert("critical")
        options = create_test_options()
        response = create_test_ask_response()

        # Test AlertData fields
        expected_alert_fields = ['name', 'severity', 'status', 'starts_at']
        for field in expected_alert_fields:
            assert hasattr(alert, field), f"AlertData missing field: {field}"

        # Test HolmesOptions fields
        expected_options_fields = ['max_tokens', 'temperature', 'timeout', 'context_window']
        for field in expected_options_fields:
            assert hasattr(options, field), f"HolmesOptions missing field: {field}"

        # Test AskResponse fields
        expected_response_fields = ['response', 'confidence', 'model_used', 'processing_time']
        for field in expected_response_fields:
            assert hasattr(response, field), f"AskResponse missing field: {field}"


class TestMockBehaviorContracts:
    """Test that mocks behave consistently with expected patterns."""

    @pytest.mark.asyncio
    async def test_mock_response_structure(self):
        """Test mock responses have expected structure."""
        # Create a mock response
        response = create_test_ask_response()

        # Verify structure matches what tests expect
        assert isinstance(response.response, str)
        assert len(response.response) > 0
        assert isinstance(response.confidence, (int, float))
        assert isinstance(response.model_used, str)
        assert isinstance(response.processing_time, (int, float))
        assert isinstance(response.recommendations, list)
        assert isinstance(response.sources, list)

    def test_builder_chaining_contract(self):
        """Test that builder methods chain correctly."""
        # Test method chaining
        alert = (AlertBuilder()
                .with_name("ChainTest")
                .with_severity("critical")
                .critical()  # Should override previous severity
                .with_labels({"test": "chain"})
                .build())

        assert alert.name == "ChainTest"
        assert alert.severity == "critical"  # Last call wins
        assert alert.labels["test"] == "chain"

        # Test options chaining
        options = (HolmesOptionsBuilder()
                  .with_max_tokens(1000)
                  .with_temperature(0.5)
                  .standard()  # Should override previous values
                  .build())

        # Standard() should set its own values
        assert options.max_tokens == 2000  # From standard()
        assert options.temperature == 0.7  # From standard()
