"""
Pytest configuration and shared fixtures for the HolmesGPT API test suite.
"""

import asyncio
import os
import pytest
import pytest_asyncio
import tempfile
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, Any, AsyncGenerator
from unittest.mock import AsyncMock, MagicMock, patch

import aiohttp
from fastapi.testclient import TestClient
from httpx import AsyncClient

# Import response models for proper mocking
from app.models.responses import (
    AskResponse, InvestigateResponse, HealthCheckResponse, ServiceInfoResponse,
    InvestigationResult, AnalysisResult, HealthStatus, Recommendation
)

# Import application modules
from app.main import app, create_app
from app.config import Settings, TestEnvironmentSettings, get_settings_no_cache
from app.services.holmes_service import HolmesGPTService
from app.services.holmesgpt_wrapper import HolmesGPTWrapper
# Cache functionality removed
from app.utils.metrics import MetricsManager
from app.models.requests import AlertData, ContextData, HolmesOptions, InvestigationContext
from app.models.responses import (
    AskResponse, InvestigateResponse, HealthCheckResponse,
    Recommendation, AnalysisResult, InvestigationResult, HealthStatus
)


# Test configuration
@pytest.fixture(scope="session")
def event_loop():
    """Create an instance of the default event loop for the test session."""
    loop = asyncio.get_event_loop_policy().new_event_loop()
    yield loop
    loop.close()


@pytest.fixture
def test_settings() -> TestEnvironmentSettings:
    """Provide test settings with safe defaults."""
    return TestEnvironmentSettings(
        debug_mode=True,
        test_mode=True,
        log_level="DEBUG",
        metrics_enabled=False,
        cache_enabled=False,
        background_tasks_enabled=False,
        holmes_cli_fallback=False,
        holmes_direct_import=True,
        holmes_enable_debug=True,
        port=8001,  # Different port for tests
        metrics_port=9091,
    )


@pytest.fixture
def test_app(test_settings):
    """Create test FastAPI application."""
    from unittest.mock import AsyncMock
    from app.services.holmes_service import HolmesGPTService
    from app.utils.metrics import MetricsManager

    # Mock the global variables and lifespan initialization
    mock_service = AsyncMock(spec=HolmesGPTService)
    mock_metrics = AsyncMock(spec=MetricsManager)

    # Set up mock service responses with proper structure
    mock_ask_response = AskResponse(
        response="This is a test response from HolmesGPT",
        analysis=AnalysisResult(
            summary="Test analysis summary",
            root_cause="Test root cause",
            impact_assessment="Test impact assessment",
            urgency_level="medium",
            affected_components=["test-component"],
            related_metrics={"cpu": "50%", "memory": "60%"}
        ),
        recommendations=[
            Recommendation(
                action="test_action",
                description="Test recommendation description",
                risk="low",
                confidence=0.8
            )
        ],
        confidence=0.85,
        model_used="test-model",
        tokens_used=100,
        processing_time=1.5,
        sources=["prometheus", "kubernetes"],
        limitations=["Test limitation"],
        follow_up_questions=["Test follow-up question"]
    )

    mock_investigate_response = InvestigateResponse(
        investigation=InvestigationResult(
            alert_analysis=AnalysisResult(
                summary="Test investigation summary",
                root_cause="Test investigation root cause",
                impact_assessment="Test investigation impact",
                urgency_level="medium",
                affected_components=["test-component"],
                related_metrics={"cpu": "50%", "memory": "60%"}
            ),
            evidence={"logs": "test logs", "metrics": "test metrics"},
            metrics_data={"cpu_usage": 0.5, "memory_usage": 0.6},
            logs_summary="Test logs summary",
            remediation_plan=[
                Recommendation(
                    action="test_remediation",
                    description="Test remediation description",
                    risk="low",
                    confidence=0.9
                )
            ]
        ),
        recommendations=[
            Recommendation(
                action="test_recommendation",
                description="Test recommendation description",
                risk="medium",
                confidence=0.85
            )
        ],
        confidence=0.9,
        severity_assessment="medium",
        estimated_resolution_time="5-10 minutes",
        requires_human_intervention=False,
        auto_executable_actions=[],
        model_used="test-model",
        tokens_used=150,
        processing_time=2.5,
        data_sources=["prometheus", "kubernetes", "logs"]
    )

    current_timestamp = time.time()
    mock_health_response = HealthCheckResponse(
        healthy=True,
        status="all_systems_operational",
        message="All systems are operational",
        checks={
            "holmesgpt": HealthStatus(
                component="holmesgpt",
                status="healthy",
                message="HolmesGPT service is responsive",
                last_check=datetime.now(timezone.utc),
                response_time=0.5,
                details={"api_available": True}
            ),
            "ollama": HealthStatus(
                component="ollama",
                status="healthy",
                message="Ollama service is operational",
                last_check=datetime.now(timezone.utc),
                response_time=1.2,
                details={"models_loaded": True}
            )
        },
        system_info={
            "python_version": "3.12.0",
            "memory_usage": "45%",
            "cpu_usage": "12%"
        },
        timestamp=current_timestamp,
        version="1.0.0",
        uptime=3600.5
    )

    mock_service_info_response = {
        "service": "HolmesGPT Python API Service",
        "version": "1.0.0",
        "api_available": True,
        "operations_count": 100,
        "total_processing_time": 250.5,
        "average_processing_time": 2.5,
        "model": "test-model",
        "cache_enabled": True,
        "initialization_time": 1.2
    }

    # Configure async mock methods properly
    async def mock_ask(*args, **kwargs):
        return mock_ask_response

    async def mock_investigate(*args, **kwargs):
        return mock_investigate_response

    async def mock_health_check(*args, **kwargs):
        return mock_health_response

    async def mock_get_service_info(*args, **kwargs):
        return mock_service_info_response

    async def mock_reload(*args, **kwargs):
        return {"status": "reloaded", "message": "Service reloaded successfully"}

    # Assign the async functions to the mock service
    mock_service.ask = mock_ask
    mock_service.investigate = mock_investigate
    mock_service.health_check = mock_health_check
    mock_service.get_service_info = mock_get_service_info
    mock_service.reload = mock_reload

    async def mock_get_holmes_service():
        return mock_service

    with patch('app.config.get_settings', return_value=test_settings), \
         patch('app.main.holmes_service', mock_service), \
         patch('app.main.metrics_manager', mock_metrics):
        # Import the global app instance that has routes attached
        from app.main import app, get_holmes_service

        # Override the dependency for testing
        app.dependency_overrides[get_holmes_service] = mock_get_holmes_service

        return app


@pytest.fixture
def client(test_app):
    """Provide synchronous test client."""
    return TestClient(test_app)


@pytest.fixture
async def async_client(test_app) -> AsyncGenerator[AsyncClient, None]:
    """Provide asynchronous test client."""
    from httpx import ASGITransport
    async with AsyncClient(transport=ASGITransport(app=test_app), base_url="http://test") as client:
        yield client


@pytest.fixture
def temp_dir():
    """Provide temporary directory for tests."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def temp_file(temp_dir):
    """Provide temporary file for tests."""
    temp_file = temp_dir / "test_file.json"
    yield temp_file


# Mock fixtures
@pytest.fixture
def mock_holmes_wrapper():
    """Mock HolmesGPT wrapper."""
    wrapper = AsyncMock(spec=HolmesGPTWrapper)

    # Mock initialization
    wrapper.initialize.return_value = True
    wrapper.is_available.return_value = True

    # Mock health check
    wrapper.health_check.return_value = HealthStatus(
        component="holmesgpt_wrapper",
        status="healthy",
        message="Mock HolmesGPT wrapper is operational",
        last_check=datetime.now(),
        response_time=0.1
    )

    # Mock ask method
    wrapper.ask.return_value = AskResponse(
        response="Mock response from HolmesGPT",
        confidence=0.85,
        model_used="mock-gpt-4",
        processing_time=1.5,
        sources=["mock-prometheus", "mock-kubernetes"],
        recommendations=[
            Recommendation(
                action="mock_action",
                description="Mock recommendation",
                risk="low",
                confidence=0.9
            )
        ]
    )

    # Mock investigate method
    wrapper.investigate.return_value = InvestigateResponse(
        investigation=InvestigationResult(
            alert_analysis=AnalysisResult(
                summary="Mock analysis of alert",
                urgency_level="medium",
                affected_components=["mock-component"]
            ),
            remediation_plan=[
                Recommendation(
                    action="mock_remediation",
                    description="Mock remediation step",
                    risk="low",
                    confidence=0.95
                )
            ]
        ),
        recommendations=[],
        confidence=0.9,
        severity_assessment="medium",
        requires_human_intervention=False,
        auto_executable_actions=[],
        model_used="mock-gpt-4",
        processing_time=2.3,
        data_sources=["mock-logs", "mock-metrics"]
    )

    return wrapper


@pytest.fixture
def mock_holmes_service(mock_holmes_wrapper, test_settings):
    """Mock HolmesGPT service."""
    service = AsyncMock(spec=HolmesGPTService)
    service.settings = test_settings

    # Use the wrapper mock
    service._holmes_wrapper = mock_holmes_wrapper
    service._api_available = True

    # Mock service methods to delegate to wrapper
    service.ask = mock_holmes_wrapper.ask
    service.investigate = mock_holmes_wrapper.investigate
    service.health_check.return_value = HealthCheckResponse(
        healthy=True,
        status="healthy",
        message="All systems operational",
        checks={"holmesgpt": mock_holmes_wrapper.health_check.return_value},
        timestamp=datetime.now().timestamp(),
        service_info={
            "operations_count": 10,
            "average_processing_time": 1.5,
            "api_available": True
        }
    )

    service.get_service_info.return_value = {
        "service": "Mock HolmesGPT API Service",
        "version": "1.0.0-test",
        "api_available": True,
        "operations_count": 10,
        "total_processing_time": 15.0,
        "average_processing_time": 1.5
    }

    return service


@pytest.fixture
def mock_aiohttp_session():
    """Mock aiohttp session."""
    session = AsyncMock(spec=aiohttp.ClientSession)

    # Mock successful health check response
    response = AsyncMock()
    response.status = 200
    response.json.return_value = {"version": "mock-ollama-v1.0"}

    session.get.return_value.__aenter__.return_value = response
    session.get.return_value.__aexit__.return_value = None

    return session


# Test data fixtures
@pytest.fixture
def sample_alert_data() -> AlertData:
    """Provide sample alert data for testing."""
    return AlertData(
        name="TestHighMemoryUsage",
        severity="warning",
        status="firing",
        starts_at=datetime.now(timezone.utc),
        labels={
            "instance": "test-pod-123",
            "namespace": "test-namespace",
            "job": "kubernetes-pods"
        },
        annotations={
            "description": "Memory usage above 80% for test pod",
            "summary": "High memory usage detected in test environment"
        }
    )


@pytest.fixture
def sample_context_data() -> ContextData:
    """Provide sample context data for testing."""
    return ContextData(
        kubernetes_context={
            "namespace": "test-namespace",
            "cluster": "test-cluster",
            "deployment": "test-deployment"
        },
        prometheus_context={
            "instance": "test-prometheus:9090"
        },
        environment="test",
        namespace="test-namespace",
        cluster="test-cluster"
    )


@pytest.fixture
def sample_holmes_options() -> HolmesOptions:
    """Provide sample HolmesGPT options for testing."""
    return HolmesOptions(
        max_tokens=2000,
        temperature=0.1,
        timeout=30,
        include_tools=["kubernetes", "prometheus"],
        debug=True,
        model="gpt-4"
    )


@pytest.fixture
def sample_investigation_context() -> InvestigationContext:
    """Provide sample investigation context for testing."""
    return InvestigationContext(
        include_metrics=True,
        include_logs=True,
        include_events=True,
        include_resources=True,
        time_range="1h",
        custom_queries=["up{job='kubernetes-pods'}", "rate(http_requests_total[5m])"]
    )


# Cache fixtures
@pytest_asyncio.fixture
async def test_cache():
    """Provide test cache instance."""
    cache = AsyncCache(ttl=5, max_size=10)
    yield cache
    await cache.clear()


@pytest.fixture
async def cache_manager():
    """Provide test cache manager."""
    manager = CacheManager()
    yield manager
    await manager.clear_all()


# Metrics fixtures
@pytest.fixture
async def metrics_manager():
    """Provide test metrics manager."""
    manager = MetricsManager()
    yield manager
    await manager.reset_metrics()


# Environment fixtures
@pytest.fixture(autouse=True)
def setup_test_environment(monkeypatch):
    """Set up test environment variables."""
    test_env = {
        "ENVIRONMENT": "test",
        "DEBUG_MODE": "true",
        "LOG_LEVEL": "DEBUG",
        "METRICS_ENABLED": "false",
        "CACHE_ENABLED": "false",
        "HOLMES_CLI_FALLBACK": "false",
        "HOLMES_DIRECT_IMPORT": "true",
        "HOLMES_ENABLE_DEBUG": "true",
    }

    for key, value in test_env.items():
        monkeypatch.setenv(key, value)


# Patches and mocks for external dependencies
@pytest.fixture
def mock_holmesgpt_import():
    """Mock HolmesGPT library import."""
    mock_holmes = MagicMock()
    mock_llm_config = MagicMock()

    with patch.dict('sys.modules', {
        'holmesgpt': mock_holmes,
        'holmesgpt.core': MagicMock(),
        'holmesgpt.core.llm': MagicMock(LLMConfig=mock_llm_config)
    }):
        yield mock_holmes, mock_llm_config


@pytest.fixture
def mock_external_services():
    """Mock external service calls."""
    patches = [
        patch('aiohttp.ClientSession'),
        patch('psutil.virtual_memory'),
        patch('psutil.cpu_percent'),
    ]

    mocks = []
    for p in patches:
        mock = p.start()
        mocks.append(mock)

    # Configure psutil mocks
    mocks[1].return_value = MagicMock(percent=45.0)  # Memory usage
    mocks[2].return_value = 12.0  # CPU usage

    yield mocks

    for p in patches:
        p.stop()


# Test utility functions
def create_test_request_data(
    prompt: str = "Test prompt",
    context: Dict[str, Any] = None,
    options: Dict[str, Any] = None
) -> Dict[str, Any]:
    """Create test request data for API endpoints."""
    data = {"prompt": prompt}

    if context:
        data["context"] = context

    if options:
        data["options"] = options

    return data


def create_test_alert_data(
    name: str = "TestAlert",
    severity: str = "warning",
    status: str = "firing"
) -> Dict[str, Any]:
    """Create test alert data."""
    return {
        "name": name,
        "severity": severity,
        "status": status,
        "starts_at": datetime.now(timezone.utc).isoformat(),
        "labels": {"test": "true"},
        "annotations": {"description": "Test alert"}
    }


def create_test_investigate_data(
    alert_data: Dict[str, Any] = None,
    context: Dict[str, Any] = None,
    options: Dict[str, Any] = None
) -> Dict[str, Any]:
    """Create test investigation request data."""
    data = {
        "alert": alert_data or create_test_alert_data()
    }

    if context:
        data["context"] = context

    if options:
        data["options"] = options

    return data


# Async test utilities
async def wait_for_condition(condition_func, timeout: float = 5.0, interval: float = 0.1):
    """Wait for a condition to become true."""
    start_time = asyncio.get_event_loop().time()

    while asyncio.get_event_loop().time() - start_time < timeout:
        if await condition_func():
            return True
        await asyncio.sleep(interval)

    return False


# Test markers
def pytest_configure(config):
    """Configure custom pytest markers."""
    config.addinivalue_line(
        "markers", "integration: mark test as integration test requiring external dependencies"
    )
    config.addinivalue_line(
        "markers", "slow: mark test as slow running"
    )
    config.addinivalue_line(
        "markers", "unit: mark test as unit test"
    )
    config.addinivalue_line(
        "markers", "e2e: mark test as end-to-end test"
    )


# Skip conditions
def skip_if_no_ollama():
    """Skip test if Ollama is not available."""
    try:
        import requests
        response = requests.get("http://localhost:11434/api/version", timeout=1)
        return response.status_code != 200
    except Exception:
        return True


def skip_if_no_holmesgpt():
    """Skip test if HolmesGPT library is not available."""
    try:
        import holmesgpt
        return False
    except ImportError:
        return True
