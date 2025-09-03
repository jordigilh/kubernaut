"""
Interface adapters for handling external library changes.

This module provides adapters that handle interface changes in external libraries,
making tests more resilient to API changes.
"""

import asyncio
from typing import Any, Dict, List, Optional, Union
from unittest.mock import MagicMock, AsyncMock
from dataclasses import dataclass


@dataclass
class MockResponse:
    """Standardized mock response structure."""
    response: str
    confidence: float = 0.8
    model_used: str = "test-model"
    processing_time: float = 1.0
    recommendations: List[Dict[str, Any]] = None
    sources: List[str] = None
    tokens_used: Optional[int] = None

    def __post_init__(self):
        if self.recommendations is None:
            self.recommendations = []
        if self.sources is None:
            self.sources = []


class HolmesInterfaceAdapter:
    """Adapter for different versions of Holmes library interfaces."""

    def __init__(self, mock_responses: Optional[Dict[str, Any]] = None):
        self.mock_responses = mock_responses or {}
        self._call_count = 0

    def create_compatible_holmes_mock(self) -> MagicMock:
        """Create a Holmes mock that's compatible with multiple API versions."""
        holmes_mock = MagicMock()

        # Add support for different method names that might exist
        method_variants = [
            'ask', 'query', 'investigate', 'analyze',
            'get_response', 'process_query', 'chat'
        ]

        for method_name in method_variants:
            mock_method = MagicMock()
            mock_method.side_effect = self._create_response_handler(method_name)
            setattr(holmes_mock, method_name, mock_method)

        # Add common properties that might be expected
        holmes_mock.model = "test-model"
        holmes_mock.config = {"max_tokens": 2000, "temperature": 0.7}
        holmes_mock.initialized = True

        return holmes_mock

    def _create_response_handler(self, method_name: str):
        """Create a response handler for a specific method."""
        def handler(*args, **kwargs):
            self._call_count += 1

            # Return different responses based on method and arguments
            if method_name in ['ask', 'query', 'chat']:
                return self._create_ask_response(*args, **kwargs)
            elif method_name in ['investigate', 'analyze']:
                return self._create_investigate_response(*args, **kwargs)
            else:
                return self._create_generic_response(*args, **kwargs)

        return handler

    def _create_ask_response(self, *args, **kwargs) -> Dict[str, Any]:
        """Create a standardized ask response."""
        prompt = args[0] if args else kwargs.get('prompt', 'default prompt')

        # Create response based on prompt content
        if 'error' in str(prompt).lower():
            response_text = "Error handling response"
            confidence = 0.6
        elif 'concurrent' in str(prompt).lower():
            response_text = f"Concurrent response {self._call_count}"
            confidence = 0.8
        else:
            response_text = "This is a test response from HolmesGPT"
            confidence = 0.9

        return {
            'response': response_text,
            'confidence': confidence,
            'model': 'test-model',
            'processing_time': 1.5,
            'recommendations': [
                {
                    'action': 'test_action',
                    'description': 'Test recommendation',
                    'risk': 'low',
                    'confidence': 0.95
                }
            ],
            'sources': ['test-source'],
            'tokens_used': 150
        }

    def _create_investigate_response(self, *args, **kwargs) -> Dict[str, Any]:
        """Create a standardized investigate response."""
        alert = args[0] if args else kwargs.get('alert')

        return {
            'analysis': {
                'summary': 'Complete investigation analysis',
                'root_cause': 'Test root cause',
                'urgency_level': 'medium',
                'affected_components': ['test-component']
            },
            'recommendations': [
                {
                    'action': 'restart_service',
                    'description': 'Restart the affected service',
                    'risk': 'low',
                    'confidence': 0.9
                }
            ],
            'confidence': 0.85,
            'severity_assessment': 'medium',
            'processing_time': 2.5,
            'tokens_used': 200
        }

    def _create_generic_response(self, *args, **kwargs) -> Dict[str, Any]:
        """Create a generic response for unknown methods."""
        return {
            'response': 'Generic test response',
            'confidence': 0.8,
            'processing_time': 1.0,
            'success': True
        }


class AsyncHolmesInterfaceAdapter(HolmesInterfaceAdapter):
    """Async version of Holmes interface adapter."""

    def create_compatible_holmes_mock(self) -> MagicMock:
        """Create an async-compatible Holmes mock."""
        holmes_mock = MagicMock()

        # Add support for async method variants
        async_method_variants = [
            'ask', 'query', 'investigate', 'analyze',
            'get_response', 'process_query', 'chat'
        ]

        for method_name in async_method_variants:
            async_method = AsyncMock()
            async_method.side_effect = self._create_async_response_handler(method_name)
            setattr(holmes_mock, method_name, async_method)

        # Add sync versions as fallback
        super_mock = super().create_compatible_holmes_mock()
        for attr_name in dir(super_mock):
            if not attr_name.startswith('_') and not hasattr(holmes_mock, attr_name):
                setattr(holmes_mock, attr_name, getattr(super_mock, attr_name))

        return holmes_mock

    def _create_async_response_handler(self, method_name: str):
        """Create an async response handler for a specific method."""
        async def async_handler(*args, **kwargs):
            # Add small delay to simulate async processing
            await asyncio.sleep(0.01)
            return self._create_response_handler(method_name)(*args, **kwargs)

        return async_handler


class ServiceInterfaceAdapter:
    """Adapter for service layer interface changes."""

    @staticmethod
    def create_compatible_service_mock(service_class) -> AsyncMock:
        """Create a service mock compatible with different interfaces."""
        service_mock = AsyncMock(spec=service_class)

        # Standard service methods that should always exist
        service_methods = [
            'initialize', 'ask', 'investigate', 'health_check',
            'get_service_info', 'reload', 'cleanup'
        ]

        for method_name in service_methods:
            if hasattr(service_class, method_name):
                async_method = AsyncMock()
                setattr(service_mock, method_name, async_method)

        # Set up reasonable default return values
        if hasattr(service_mock, 'ask'):
            service_mock.ask.return_value = MockResponse(
                response="Service ask response",
                confidence=0.9
            )

        if hasattr(service_mock, 'health_check'):
            service_mock.health_check.return_value = {
                'status': 'healthy',
                'checks': {
                    'holmesgpt': {'status': 'healthy'},
                    'ollama': {'status': 'healthy'},
                    'system': {'status': 'healthy'}
                }
            }

        return service_mock


class ConfigurationAdapter:
    """Adapter for handling configuration changes."""

    @staticmethod
    def normalize_model_name(model_name: str) -> str:
        """Normalize model names to handle variations."""
        model_mappings = {
            # Common variations
            'gpt-4': 'test-model',
            'gpt-3.5-turbo': 'test-model',
            'gpt-oss:20b': 'test-model',
            'claude-3': 'test-model',
            'llama2': 'test-model',
            # Keep test models as-is
            'test-model': 'test-model',
            'mock-gpt-4': 'test-model',
            'integration-test-model': 'test-model'
        }

        return model_mappings.get(model_name, 'test-model')

    @staticmethod
    def normalize_severity(severity: str) -> str:
        """Normalize severity levels to handle variations."""
        severity = severity.lower().strip()

        severity_mappings = {
            # Standard levels
            'critical': 'critical',
            'warning': 'warning',
            'info': 'info',
            # Common variations
            'high': 'critical',
            'medium': 'warning',
            'low': 'info',
            'error': 'critical',
            'warn': 'warning',
            'crit': 'critical'
        }

        return severity_mappings.get(severity, 'warning')

    @staticmethod
    def normalize_assessment(assessment: str) -> str:
        """Normalize assessment levels."""
        assessment = assessment.lower().strip()

        assessment_mappings = {
            'critical': 'critical',
            'high': 'high',
            'medium': 'medium',
            'low': 'low',
            # Map severity to assessment
            'warning': 'medium',
            'info': 'low'
        }

        return assessment_mappings.get(assessment, 'medium')


# Factory functions for easy test use
def create_holmes_mock() -> MagicMock:
    """Create a compatible Holmes mock for tests."""
    adapter = HolmesInterfaceAdapter()
    return adapter.create_compatible_holmes_mock()


def create_async_holmes_mock() -> MagicMock:
    """Create a compatible async Holmes mock for tests."""
    adapter = AsyncHolmesInterfaceAdapter()
    return adapter.create_compatible_holmes_mock()


def create_service_mock(service_class) -> AsyncMock:
    """Create a compatible service mock for tests."""
    return ServiceInterfaceAdapter.create_compatible_service_mock(service_class)
