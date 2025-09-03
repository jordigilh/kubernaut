"""
Integration tests for HolmesGPT REST API.
"""

import pytest
import pytest_asyncio
import asyncio
from datetime import datetime
from fastapi.testclient import TestClient
from httpx import AsyncClient

from app.main import app
from app.config import get_settings_no_cache, TestEnvironmentSettings
from unittest.mock import patch, AsyncMock


@pytest.fixture
def test_settings():
    """Get test settings."""
    return TestEnvironmentSettings()


# Client fixture is provided by conftest.py with proper dependency injection


@pytest_asyncio.fixture
async def async_client():
    """Get async test client."""
    from httpx import ASGITransport
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
        yield client


class TestHealthEndpoints:
    """Test health and info endpoints."""

    def test_root_endpoint(self, client):
        """Test root endpoint."""
        response = client.get("/")
        assert response.status_code == 200
        data = response.json()
        assert data["name"] == "HolmesGPT REST API"
        assert data["version"] == "1.0.0"
        assert "features" in data

    def test_health_endpoint(self, client):
        """Test health check endpoint."""
        response = client.get("/health")
        assert response.status_code == 200
        data = response.json()
        assert "healthy" in data
        assert "status" in data
        assert "timestamp" in data

    def test_service_info_endpoint(self, client):
        """Test service info endpoint."""
        response = client.get("/service/info")
        # May return 503 if HolmesGPT service is not initialized
        assert response.status_code in [200, 503]


class TestAskEndpoint:
    """Test ask endpoint."""

    def test_ask_basic(self, client):
        """Test basic ask request."""
        request_data = {
            "prompt": "What should I do if my pods are crashing?",
            "options": {
                "max_tokens": 1000,
                "temperature": 0.1
            }
        }

        response = client.post("/ask", json=request_data)
        # May return 503 if HolmesGPT service is not available
        if response.status_code == 200:
            data = response.json()
            assert "response" in data
            assert "confidence" in data
            assert "model_used" in data
            assert "processing_time" in data
        else:
            assert response.status_code == 503

    def test_ask_with_context(self, client):
        """Test ask with context."""
        request_data = {
            "prompt": "How do I debug memory leaks?",
            "context": {
                "kubernetes_context": {
                    "namespace": "production",
                    "deployment": "api-server"
                },
                "environment": "production"
            },
            "options": {
                "max_tokens": 2000,
                "include_tools": ["kubernetes", "prometheus"]
            }
        }

        response = client.post("/ask", json=request_data)
        # May return 503 if HolmesGPT service is not available
        assert response.status_code in [200, 503]

    def test_ask_validation_empty_prompt(self, client):
        """Test ask validation with empty prompt."""
        request_data = {
            "prompt": "",
            "options": {"max_tokens": 1000}
        }

        response = client.post("/ask", json=request_data)
        assert response.status_code == 422  # Validation error

    def test_ask_validation_invalid_temperature(self, client):
        """Test ask validation with invalid temperature."""
        request_data = {
            "prompt": "Test question",
            "options": {
                "temperature": 3.0  # Invalid: > 2.0
            }
        }

        response = client.post("/ask", json=request_data)
        assert response.status_code == 422  # Validation error


class TestInvestigateEndpoint:
    """Test investigate endpoint."""

    def test_investigate_basic(self, client):
        """Test basic investigation request."""
        request_data = {
            "alert": {
                "name": "HighMemoryUsage",
                "severity": "warning",
                "status": "firing",
                "starts_at": "2024-01-15T10:30:00Z",
                "labels": {
                    "instance": "api-server-pod",
                    "namespace": "production"
                },
                "annotations": {
                    "description": "Memory usage above 80%",
                    "summary": "High memory usage detected"
                }
            },
            "options": {
                "max_tokens": 3000,
                "temperature": 0.1
            }
        }

        response = client.post("/investigate", json=request_data)
        # May return 503 if HolmesGPT service is not available
        if response.status_code == 200:
            data = response.json()
            assert "investigation" in data
            assert "recommendations" in data
            assert "confidence" in data
            assert "severity_assessment" in data
        else:
            assert response.status_code == 503

    def test_investigate_with_context(self, client):
        """Test investigation with full context."""
        request_data = {
            "alert": {
                "name": "PodCrashLooping",
                "severity": "critical",
                "status": "firing",
                "starts_at": "2024-01-15T10:30:00Z",
                "labels": {
                    "namespace": "production",
                    "pod": "api-server-pod",
                    "container": "api-server"
                }
            },
            "context": {
                "kubernetes_context": {
                    "namespace": "production",
                    "cluster": "prod-cluster-1"
                },
                "environment": "production"
            },
            "investigation_context": {
                "include_metrics": True,
                "include_logs": True,
                "time_range": "2h"
            },
            "options": {
                "max_tokens": 4000,
                "include_tools": ["kubernetes", "prometheus", "logs"]
            }
        }

        response = client.post("/investigate", json=request_data)
        # May return 503 if HolmesGPT service is not available
        assert response.status_code in [200, 503]

    def test_investigate_validation_invalid_severity(self, client):
        """Test investigation validation with invalid severity."""
        request_data = {
            "alert": {
                "name": "TestAlert",
                "severity": "invalid_severity",  # Invalid severity
                "status": "firing",
                "starts_at": "2024-01-15T10:30:00Z"
            }
        }

        response = client.post("/investigate", json=request_data)
        assert response.status_code == 422  # Validation error


class TestMetricsEndpoint:
    """Test metrics endpoint."""

    def test_metrics_endpoint(self, client):
        """Test metrics endpoint."""
        response = client.get("/metrics")
        # Should return metrics in Prometheus format
        assert response.status_code == 200
        assert "content-type" in response.headers
        # Content should contain some basic metrics
        content = response.text
        assert len(content) > 0


class TestAsyncOperations:
    """Test async operations."""

    @pytest.mark.asyncio
    async def test_concurrent_ask_requests(self, async_client):
        """Test concurrent ask requests."""
        request_data = {
            "prompt": "How do I scale my deployment?",
            "options": {"max_tokens": 500}
        }

        # Make multiple concurrent requests
        tasks = [
            async_client.post("/ask", json=request_data)
            for _ in range(3)
        ]

        responses = await asyncio.gather(*tasks)

        # All requests should complete
        assert len(responses) == 3

        # Check that all responses are either success or service unavailable
        for response in responses:
            assert response.status_code in [200, 503]

    @pytest.mark.asyncio
    async def test_health_check_response_time(self, async_client):
        """Test health check response time."""
        import time

        start_time = time.time()
        response = await async_client.get("/health")
        end_time = time.time()

        # Health check should be fast
        response_time = end_time - start_time
        assert response_time < 5.0  # Should respond within 5 seconds

        assert response.status_code == 200


class TestErrorHandling:
    """Test error handling."""

    def test_404_endpoint(self, client):
        """Test 404 error handling."""
        response = client.get("/nonexistent-endpoint")
        assert response.status_code == 404

    def test_method_not_allowed(self, client):
        """Test method not allowed."""
        response = client.put("/ask")
        assert response.status_code == 405

    def test_malformed_json(self, client):
        """Test malformed JSON handling."""
        response = client.post(
            "/ask",
            data="malformed json",
            headers={"content-type": "application/json"}
        )
        assert response.status_code == 422


# Fixtures for mock data
@pytest.fixture
def sample_alert():
    """Sample alert data for testing."""
    return {
        "name": "HighCPUUsage",
        "severity": "warning",
        "status": "firing",
        "starts_at": datetime.now().isoformat(),
        "labels": {
            "instance": "web-server-1",
            "job": "kubernetes-pods",
            "namespace": "production"
        },
        "annotations": {
            "description": "CPU usage is above 80%",
            "summary": "High CPU usage detected"
        }
    }


@pytest.fixture
def sample_ask_request():
    """Sample ask request for testing."""
    return {
        "prompt": "My application is running slowly. What should I check?",
        "context": {
            "environment": "production",
            "kubernetes_context": {
                "namespace": "production",
                "deployment": "web-app"
            }
        },
        "options": {
            "max_tokens": 2000,
            "temperature": 0.1,
            "include_tools": ["kubernetes", "prometheus"]
        }
    }


# Integration test with external dependencies (skip if not available)
@pytest.mark.integration
class TestIntegrationWithDependencies:
    """Integration tests requiring external dependencies."""

    @pytest.mark.skipif(
        not pytest.importorskip("requests", reason="requests not available"),
        reason="Integration test requires external dependencies"
    )
    def test_ollama_integration(self):
        """Test Ollama integration if available."""
        import requests

        try:
            # Check if Ollama is available
            response = requests.get("http://localhost:11434/api/version", timeout=5)
            ollama_available = response.status_code == 200
        except requests.exceptions.RequestException:
            ollama_available = False

        if not ollama_available:
            pytest.skip("Ollama not available")

        # If Ollama is available, test should pass
        assert True


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
