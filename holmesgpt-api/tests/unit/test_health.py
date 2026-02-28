"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Health and Readiness Endpoint Tests

Business Requirements: BR-HAPI-126 to 145 (Health/Monitoring)
"""



class TestHealthEndpoint:
    """Tests for /health endpoint"""

    def test_health_returns_200(self, client):
        """Business Requirement: Health check returns 200"""
        response = client.get("/health")
        assert response.status_code == 200

    def test_health_returns_service_info(self, client):
        """Business Requirement: Health check includes service metadata"""
        response = client.get("/health")
        data = response.json()

        assert data["status"] == "healthy"
        assert data["service"] == "holmesgpt-api"
        assert "endpoints" in data
        assert "features" in data

    def test_health_lists_correct_endpoints(self, client):
        """Business Requirement: Health check lists available endpoints"""
        response = client.get("/health")
        data = response.json()

        endpoints = data["endpoints"]
        assert "/api/v1/incident/analyze" in endpoints
        # DD-017: PostExec endpoint deferred to V1.1 - Effectiveness Monitor not in V1.0
        # assert "/api/v1/postexec/analyze" in endpoints
        assert "/health" in endpoints
        assert "/ready" in endpoints

    def test_health_lists_enabled_features(self, client):
        """Business Requirement: Health check lists enabled features"""
        response = client.get("/health")
        data = response.json()

        features = data["features"]
        assert features["incident_analysis"] is True
        # DD-017: PostExec feature deferred to V1.1 - Effectiveness Monitor not in V1.0
        # assert features["postexec_analysis"] is True
        assert features["authentication"] is True


class TestReadinessEndpoint:
    """Tests for /ready endpoint"""

    def test_ready_returns_200_when_healthy(self, client):
        """Business Requirement: Readiness check returns 200 when ready"""
        response = client.get("/ready")
        assert response.status_code == 200

    def test_ready_checks_dependencies(self, client):
        """Business Requirement: Readiness check validates dependencies"""
        response = client.get("/ready")
        data = response.json()

        assert data["status"] == "ready"
        assert "dependencies" in data

        deps = data["dependencies"]
        assert "sdk" in deps
        assert "context_api" in deps
        assert "prometheus" in deps


class TestConfigEndpoint:
    """Tests for /config endpoint"""

    def test_config_returns_200(self, client):
        """Business Requirement: Config endpoint returns 200"""
        response = client.get("/config")
        assert response.status_code == 200

    def test_config_returns_sanitized_config(self, client):
        """Business Requirement: Config endpoint does not expose secrets"""
        response = client.get("/config")
        data = response.json()

        assert "llm" in data
        assert "environment" in data
        assert "dev_mode" in data

        # Verify no secrets exposed
        assert "api_key" not in str(data).lower()
        assert "password" not in str(data).lower()
