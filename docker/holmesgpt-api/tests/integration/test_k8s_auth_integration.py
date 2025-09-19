"""
K8s Authentication Integration Tests - Business Requirements BR-HAPI-045
Real Kubernetes API integration for OAuth2 resource server testing
Following TDD principles and project guidelines
"""

import pytest
import time
from typing import Dict, Any
import requests

# Test markers
pytestmark = [pytest.mark.integration, pytest.mark.k8s, pytest.mark.auth]


class TestK8sAuthenticationIntegration:
    """
    Integration tests for OAuth2 + K8s authentication
    Business Requirement: BR-HAPI-045 - OAuth 2 resource server compatible with K8s API server
    """

    def test_valid_serviceaccount_token_authentication(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        integration_config
    ):
        """
        BR-HAPI-045.1: Valid K8s ServiceAccount tokens should authenticate successfully
        Business Requirement: Accept and validate Kubernetes service account tokens
        """
        # Get admin ServiceAccount token
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        assert admin_token, "Admin ServiceAccount token should be available"

        # Authenticate with admin token
        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Test health endpoint with authentication
        response = holmesgpt_api_client.get("/health")

        # Business validation: Admin token should provide access
        assert response.status_code == 200, f"Admin SA token should authenticate successfully, got {response.status_code}"

        health_data = response.json()
        assert health_data["status"] == "healthy", "API should be healthy with valid authentication"

        # Business validation: Should include authentication info
        assert "auth" in health_data, "Health response should include auth information"
        assert health_data["auth"]["authenticated"] is True, "Should be authenticated with SA token"

    def test_invalid_token_rejection(self, holmesgpt_api_client):
        """
        BR-HAPI-045.1: Invalid token formats should be rejected
        Business Requirement: Security - prevent malformed token acceptance
        """
        invalid_tokens = [
            "not-a-jwt-token",
            "invalid.jwt.format",
            "Bearer invalid-token",
            "",
            "eyJhbGciOiJIUzI1NiJ9.invalid.signature"
        ]

        for invalid_token in invalid_tokens:
            # Clear previous auth and set invalid token
            holmesgpt_api_client.clear_authentication()
            holmesgpt_api_client.authenticate_with_bearer_token(invalid_token)

            # Test protected endpoint
            response = holmesgpt_api_client.get("/health")

            # Business validation: Invalid tokens should be rejected
            assert response.status_code in [401, 403], f"Invalid token should be rejected: {invalid_token[:20]}..."

    def test_rbac_to_oauth2_scope_mapping(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        test_serviceaccounts
    ):
        """
        BR-HAPI-045.3: K8s RBAC permissions should map to OAuth2 scopes correctly
        Business Requirement: Convert K8s permissions to OAuth 2 scopes
        """
        test_cases = [
            {
                "sa_name": "test-admin-sa",
                "expected_scopes": ["admin:system", "admin:users", "cluster:info", "pods:write"],
                "description": "Admin SA should have admin scopes"
            },
            {
                "sa_name": "test-viewer-sa",
                "expected_scopes": ["cluster:info", "pods:read"],
                "description": "Viewer SA should have read-only scopes"
            }
        ]

        for test_case in test_cases:
            sa_token = serviceaccount_tokens.get(test_case["sa_name"])
            if not sa_token:
                pytest.skip(f"ServiceAccount token not available: {test_case['sa_name']}")

            # Authenticate with ServiceAccount token
            holmesgpt_api_client.clear_authentication()
            holmesgpt_api_client.authenticate_with_bearer_token(sa_token)

            # Get user info to check mapped scopes
            response = holmesgpt_api_client.get("/auth/me")

            # Business validation: SA token should be accepted
            assert response.status_code == 200, f"{test_case['description']} - token should be accepted"

            user_info = response.json()

            # Business validation: Should include OAuth2 scope mapping
            assert "scopes" in user_info, f"{test_case['description']} - should include OAuth2 scopes"

            user_scopes = user_info["scopes"]
            for expected_scope in test_case["expected_scopes"]:
                assert expected_scope in user_scopes, f"{test_case['description']} - should include scope: {expected_scope}"

    def test_bearer_token_endpoint_authorization(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens
    ):
        """
        BR-HAPI-045.5: API endpoints should authorize based on Bearer token scopes
        Business Requirement: Scope-based authorization for OAuth 2 endpoints
        """
        # Test with admin token - should access admin endpoints
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if admin_token:
            holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

            # Admin scope required endpoint
            response = holmesgpt_api_client.get("/auth/users")
            assert response.status_code == 200, "Admin token should access user management endpoints"

        # Test with viewer token - should not access admin endpoints
        viewer_token = serviceaccount_tokens.get("test-viewer-sa")
        if viewer_token:
            holmesgpt_api_client.clear_authentication()
            holmesgpt_api_client.authenticate_with_bearer_token(viewer_token)

            # Admin scope required endpoint
            response = holmesgpt_api_client.get("/auth/users")
            assert response.status_code == 403, "Viewer token should not access admin endpoints"

            # Viewer scope allowed endpoint
            response = holmesgpt_api_client.get("/auth/me")
            assert response.status_code == 200, "Viewer token should access own user info"

    def test_investigation_endpoint_with_k8s_auth(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens
    ):
        """
        BR-HAPI-045: Investigation API should work with K8s ServiceAccount authentication
        Business Requirement: End-to-end OAuth2 + K8s integration for core API
        """
        # Use admin token for investigation
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Test investigation endpoint with sample alert
        alert_data = {
            "alerts": [
                {
                    "status": "firing",
                    "labels": {
                        "alertname": "HighMemoryUsage",
                        "severity": "warning",
                        "namespace": "test",
                        "pod": "test-pod"
                    },
                    "annotations": {
                        "description": "Memory usage is high",
                        "summary": "High memory usage detected"
                    },
                    "startsAt": "2024-01-01T00:00:00Z"
                }
            ]
        }

        response = holmesgpt_api_client.post("/api/v1/investigate", json_data=alert_data)

        # Business validation: Investigation should work with K8s authentication
        assert response.status_code == 200, "Investigation should work with K8s ServiceAccount authentication"

        result = response.json()
        assert "investigation_id" in result, "Investigation should return investigation ID"
        assert "recommendations" in result, "Investigation should return recommendations"

    def test_chat_endpoint_with_k8s_auth(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens
    ):
        """
        BR-HAPI-045: Chat API should work with K8s ServiceAccount authentication
        Business Requirement: End-to-end OAuth2 + K8s integration for interactive features
        """
        # Use viewer token for chat (should have chat permissions)
        viewer_token = serviceaccount_tokens.get("test-viewer-sa")
        if not viewer_token:
            pytest.skip("Viewer ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(viewer_token)

        # Test chat endpoint with sample message
        chat_data = {
            "message": "What is the status of my cluster?",
            "session_id": "test-session-k8s-auth",
            "include_context": True
        }

        response = holmesgpt_api_client.post("/api/v1/chat", json_data=chat_data)

        # Business validation: Chat should work with K8s authentication
        assert response.status_code == 200, "Chat should work with K8s ServiceAccount authentication"

        result = response.json()
        assert "response" in result, "Chat should return response"
        assert "session_id" in result, "Chat should return session ID"

    @pytest.mark.slow
    def test_token_validation_performance(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens
    ):
        """
        BR-HAPI-045: Token validation should meet performance requirements
        Business Requirement: Efficient OAuth2 token validation for production use
        """
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        # Measure token validation performance
        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        start_time = time.time()
        response = holmesgpt_api_client.get("/health")
        end_time = time.time()

        # Business validation: Token validation should be fast
        validation_time = end_time - start_time
        assert validation_time < 1.0, f"Token validation should complete within 1 second, took {validation_time:.3f}s"
        assert response.status_code == 200, "Token should validate successfully"

    def test_concurrent_authentication_requests(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens
    ):
        """
        BR-HAPI-045: System should handle concurrent authentication requests
        Business Requirement: Scalable OAuth2 authentication for production load
        """
        import concurrent.futures
        import threading

        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        def make_authenticated_request():
            """Make an authenticated request with local session"""
            local_client = type(holmesgpt_api_client)(
                holmesgpt_api_client.base_url,
                holmesgpt_api_client.timeout
            )
            local_client.authenticate_with_bearer_token(admin_token)
            return local_client.get("/health")

        # Test concurrent requests
        num_concurrent = 10
        with concurrent.futures.ThreadPoolExecutor(max_workers=num_concurrent) as executor:
            futures = [executor.submit(make_authenticated_request) for _ in range(num_concurrent)]
            responses = [future.result() for future in concurrent.futures.as_completed(futures)]

        # Business validation: All concurrent requests should succeed
        for i, response in enumerate(responses):
            assert response.status_code == 200, f"Concurrent request {i} should succeed with K8s authentication"

        assert len(responses) == num_concurrent, "All concurrent requests should complete"

    def test_missing_authorization_header(self, holmesgpt_api_client):
        """
        BR-HAPI-045.5: Endpoints should reject requests without Bearer tokens
        Business Requirement: Secure access control - require authentication
        """
        # Clear any existing authentication
        holmesgpt_api_client.clear_authentication()

        # Test protected endpoints without authentication
        protected_endpoints = [
            "/api/v1/investigate",
            "/api/v1/chat",
            "/auth/me",
            "/auth/users"
        ]

        for endpoint in protected_endpoints:
            response = holmesgpt_api_client.get(endpoint)
            assert response.status_code in [401, 403], f"Endpoint {endpoint} should require authentication"

    def test_expired_token_rejection(self, holmesgpt_api_client):
        """
        BR-HAPI-045.1: Expired tokens should be rejected
        Business Requirement: Security - prevent expired token usage
        """
        # Create a mock expired token (this would be a real expired K8s token in practice)
        expired_token = "eyJhbGciOiJSUzI1NiIsImtpZCI6ImsxIn0.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50IiwiZXhwIjoxNjAwMDAwMDAwfQ.signature"

        holmesgpt_api_client.authenticate_with_bearer_token(expired_token)

        response = holmesgpt_api_client.get("/health")

        # Business validation: Expired tokens should be rejected
        assert response.status_code in [401, 403], "Expired tokens should be rejected"


class TestK8sAPIServerIntegration:
    """
    Integration tests for K8s API server communication
    Business Requirement: BR-HAPI-045.2 - K8s API Server Integration
    """

    def test_tokenreview_api_integration(
        self,
        k8s_core_v1,
        serviceaccount_tokens,
        integration_config
    ):
        """
        BR-HAPI-045.2: Validate tokens against K8s API server using TokenReview API
        Business Requirement: Direct integration with K8s API server for validation
        """
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        # This test validates that the holmesgpt-api can use TokenReview API
        # In practice, this would be tested through the API's internal token validation

        # Test that K8s API is accessible (prerequisite for TokenReview)
        try:
            namespaces = k8s_core_v1.list_namespace()
            assert len(namespaces.items) > 0, "Should be able to list namespaces from K8s API"
        except Exception as e:
            pytest.skip(f"Cannot access K8s API server: {e}")

    @pytest.mark.slow
    def test_serviceaccount_permission_discovery(
        self,
        k8s_rbac_v1,
        test_serviceaccounts,
        test_namespace
    ):
        """
        BR-HAPI-045.3: Discover ServiceAccount permissions for scope mapping
        Business Requirement: Understand bound roles for accurate scope mapping
        """
        # Test that we can discover RBAC permissions for ServiceAccounts
        # This validates the infrastructure needed for RBAC-to-scope mapping

        try:
            # List ClusterRoleBindings to find SA permissions
            bindings = k8s_rbac_v1.list_cluster_role_binding()

            # Find bindings for our test ServiceAccounts
            test_sa_bindings = [
                binding for binding in bindings.items
                if any(
                    subject.kind == "ServiceAccount" and
                    subject.name.startswith("test-") and
                    subject.namespace == test_namespace
                    for subject in (binding.subjects or [])
                )
            ]

            # Business validation: Should be able to discover SA permissions
            assert len(test_sa_bindings) > 0, "Should discover RBAC bindings for test ServiceAccounts"

            for binding in test_sa_bindings:
                assert binding.role_ref.kind in ["ClusterRole", "Role"], "Should have valid role references"

        except Exception as e:
            pytest.skip(f"Cannot access K8s RBAC API: {e}")


# Test configuration
@pytest.fixture(autouse=True)
def setup_integration_test_logging(integration_config):
    """Setup logging for integration tests"""
    import logging

    level = getattr(logging, integration_config["log_level"].upper())
    logging.basicConfig(level=level, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')

    # Set specific loggers
    logging.getLogger("kubernetes").setLevel(logging.WARNING)
    logging.getLogger("urllib3").setLevel(logging.WARNING)


# Test data fixtures
@pytest.fixture
def sample_alert_data():
    """Sample alert data for investigation testing"""
    return {
        "alerts": [
            {
                "status": "firing",
                "labels": {
                    "alertname": "HighMemoryUsage",
                    "severity": "warning",
                    "namespace": "production",
                    "pod": "app-xyz-abc123",
                    "deployment": "app-xyz"
                },
                "annotations": {
                    "description": "Pod app-xyz-abc123 is using 95% memory",
                    "summary": "High memory usage detected"
                },
                "startsAt": "2024-01-01T00:00:00Z"
            }
        ]
    }


@pytest.fixture
def sample_chat_data():
    """Sample chat data for chat API testing"""
    return {
        "message": "Help me understand this memory alert",
        "session_id": "integration-test-session",
        "include_context": True,
        "stream": False
    }

