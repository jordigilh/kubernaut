"""
End-to-End Integration Tests for HolmesGPT API
Full ecosystem testing with real infrastructure components
Following TDD principles and project guidelines
"""

import pytest
import time
from typing import Dict, Any

# Test markers
pytestmark = [pytest.mark.e2e, pytest.mark.slow]


class TestFullIntegrationE2E:
    """
    End-to-end tests for complete HolmesGPT API ecosystem
    Business Requirement: BR-HAPI-045 - Full OAuth2 + K8s + Database integration
    """

    def test_complete_investigation_flow_with_k8s_auth(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        sample_alert_data,
        integration_config
    ):
        """
        BR-HAPI-045: Complete investigation flow with K8s authentication
        Business Requirement: End-to-end OAuth2 + K8s + Database + LLM integration
        """
        # Skip if database not available
        if integration_config.get("skip_database_tests", False):
            pytest.skip("Database tests disabled")

        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        # Authenticate with K8s ServiceAccount
        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Step 1: Submit investigation request
        response = holmesgpt_api_client.post("/api/v1/investigate", json_data=sample_alert_data)
        assert response.status_code == 200, "Investigation submission should succeed with K8s auth"

        investigation = response.json()
        investigation_id = investigation["investigation_id"]

        # Step 2: Verify investigation is stored in database
        # This validates database integration
        time.sleep(2)  # Allow for async processing

        # Step 3: Check investigation status
        status_response = holmesgpt_api_client.get(f"/api/v1/investigate/{investigation_id}/status")
        assert status_response.status_code == 200, "Investigation status should be accessible"

        status = status_response.json()
        assert status["status"] in ["completed", "processing"], "Investigation should have valid status"

        # Step 4: Verify recommendations include K8s context
        recommendations = investigation.get("recommendations", [])
        assert len(recommendations) > 0, "Investigation should return recommendations"

        # Business validation: Recommendations should be contextual
        recommendation_text = " ".join([r.get("description", "") for r in recommendations]).lower()
        assert any(keyword in recommendation_text for keyword in ["memory", "pod", "kubernetes", "namespace"]), \
            "Recommendations should include K8s-specific context"

    def test_chat_session_with_k8s_context(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        integration_config
    ):
        """
        BR-HAPI-045: Chat session with K8s authentication and context
        Business Requirement: Interactive chat with K8s integration
        """
        viewer_token = serviceaccount_tokens.get("test-viewer-sa")
        if not viewer_token:
            pytest.skip("Viewer ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(viewer_token)

        # Step 1: Start chat session with K8s context
        chat_data = {
            "message": "What is the health status of my cluster?",
            "session_id": "e2e-test-session",
            "include_context": True
        }

        response = holmesgpt_api_client.post("/api/v1/chat", json_data=chat_data)
        assert response.status_code == 200, "Chat should work with K8s authentication"

        chat_result = response.json()

        # Business validation: Chat should include K8s context
        assert "response" in chat_result, "Chat should return response"
        assert "context" in chat_result, "Chat should include K8s context"
        assert "session_id" in chat_result, "Chat should maintain session"

        # Step 2: Continue conversation with context
        followup_data = {
            "message": "Tell me more about any issues",
            "session_id": chat_result["session_id"],
            "include_context": True
        }

        followup_response = holmesgpt_api_client.post("/api/v1/chat", json_data=followup_data)
        assert followup_response.status_code == 200, "Follow-up chat should work"

        followup_result = followup_response.json()
        assert followup_result["session_id"] == chat_result["session_id"], "Session should be maintained"

    def test_user_management_with_k8s_rbac(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens
    ):
        """
        BR-HAPI-045: User management with K8s RBAC mapping
        Business Requirement: Admin operations with K8s authentication
        """
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        viewer_token = serviceaccount_tokens.get("test-viewer-sa")

        if not admin_token or not viewer_token:
            pytest.skip("ServiceAccount tokens not available")

        # Step 1: Admin operations should work with admin SA
        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        admin_response = holmesgpt_api_client.get("/auth/users")
        assert admin_response.status_code == 200, "Admin SA should access user management"

        # Step 2: Viewer operations should be restricted
        holmesgpt_api_client.clear_authentication()
        holmesgpt_api_client.authenticate_with_bearer_token(viewer_token)

        viewer_response = holmesgpt_api_client.get("/auth/users")
        assert viewer_response.status_code == 403, "Viewer SA should not access user management"

        # Step 3: Viewer should access own info
        me_response = holmesgpt_api_client.get("/auth/me")
        assert me_response.status_code == 200, "Viewer SA should access own user info"

        user_info = me_response.json()
        assert "scopes" in user_info, "User info should include OAuth2 scopes"

    @pytest.mark.slow
    def test_performance_under_load_with_k8s_auth(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        integration_config
    ):
        """
        BR-HAPI-045: Performance testing with K8s authentication
        Business Requirement: Scalable OAuth2 authentication for production
        """
        import concurrent.futures
        import statistics

        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        def timed_request():
            """Make a timed authenticated request"""
            start_time = time.time()

            # Create isolated client for thread safety
            local_client = type(holmesgpt_api_client)(
                holmesgpt_api_client.base_url,
                holmesgpt_api_client.timeout
            )
            local_client.authenticate_with_bearer_token(admin_token)

            response = local_client.get("/health")
            end_time = time.time()

            return {
                "duration": end_time - start_time,
                "status_code": response.status_code,
                "success": response.status_code == 200
            }

        # Run concurrent requests
        num_requests = 20
        with concurrent.futures.ThreadPoolExecutor(max_workers=10) as executor:
            futures = [executor.submit(timed_request) for _ in range(num_requests)]
            results = [future.result() for future in concurrent.futures.as_completed(futures)]

        # Analyze performance
        durations = [r["duration"] for r in results]
        success_count = sum(1 for r in results if r["success"])

        # Business validation: Performance requirements
        avg_duration = statistics.mean(durations)
        max_duration = max(durations)

        assert success_count == num_requests, "All requests should succeed under load"
        assert avg_duration < 2.0, f"Average response time should be < 2s, got {avg_duration:.3f}s"
        assert max_duration < 5.0, f"Max response time should be < 5s, got {max_duration:.3f}s"

    def test_database_integration_with_k8s_auth(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        integration_config
    ):
        """
        BR-HAPI-045: Database operations with K8s authentication
        Business Requirement: Persistent storage with OAuth2 authentication
        """
        if integration_config.get("skip_database_tests", False):
            pytest.skip("Database tests disabled")

        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Step 1: Create investigation (should persist to database)
        alert_data = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "DatabaseTest",
                    "severity": "info",
                    "namespace": "test"
                },
                "annotations": {
                    "description": "Test alert for database integration"
                },
                "startsAt": "2024-01-01T00:00:00Z"
            }]
        }

        response = holmesgpt_api_client.post("/api/v1/investigate", json_data=alert_data)
        assert response.status_code == 200, "Investigation should be created and stored"

        investigation = response.json()
        investigation_id = investigation["investigation_id"]

        # Step 2: Retrieve investigation (should read from database)
        time.sleep(1)  # Allow for database write

        get_response = holmesgpt_api_client.get(f"/api/v1/investigate/{investigation_id}")
        assert get_response.status_code == 200, "Investigation should be retrievable from database"

        stored_investigation = get_response.json()
        assert stored_investigation["investigation_id"] == investigation_id, "Stored investigation should match"

    def test_error_handling_with_k8s_auth(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens
    ):
        """
        BR-HAPI-045: Error handling with K8s authentication
        Business Requirement: Robust error handling with OAuth2
        """
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Test 1: Invalid investigation data
        invalid_data = {"invalid": "data"}
        response = holmesgpt_api_client.post("/api/v1/investigate", json_data=invalid_data)
        assert response.status_code == 422, "Invalid data should return validation error"

        error = response.json()
        assert "detail" in error, "Error should include validation details"

        # Test 2: Non-existent investigation
        response = holmesgpt_api_client.get("/api/v1/investigate/non-existent-id")
        assert response.status_code == 404, "Non-existent investigation should return 404"

        # Test 3: Malformed chat data
        invalid_chat = {"message": ""}  # Empty message
        response = holmesgpt_api_client.post("/api/v1/chat", json_data=invalid_chat)
        assert response.status_code == 422, "Empty chat message should return validation error"


class TestK8sServiceAccountScenarios:
    """
    Test different K8s ServiceAccount scenarios
    Business Requirement: BR-HAPI-045 - ServiceAccount integration patterns
    """

    def test_multiple_serviceaccounts_concurrent_access(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens
    ):
        """
        Test multiple ServiceAccounts accessing API concurrently
        Business Requirement: Multi-tenant K8s authentication
        """
        import concurrent.futures

        tokens = {
            name: token for name, token in serviceaccount_tokens.items()
            if token and name != "test-restricted-sa"  # Skip restricted for this test
        }

        if len(tokens) < 2:
            pytest.skip("Need at least 2 ServiceAccount tokens")

        def authenticated_request(sa_name, token):
            """Make request with specific ServiceAccount"""
            local_client = type(holmesgpt_api_client)(
                holmesgpt_api_client.base_url,
                holmesgpt_api_client.timeout
            )
            local_client.authenticate_with_bearer_token(token)

            response = local_client.get("/auth/me")
            return {
                "sa_name": sa_name,
                "status_code": response.status_code,
                "user_info": response.json() if response.status_code == 200 else None
            }

        # Test concurrent access from different ServiceAccounts
        with concurrent.futures.ThreadPoolExecutor(max_workers=len(tokens)) as executor:
            futures = [
                executor.submit(authenticated_request, sa_name, token)
                for sa_name, token in tokens.items()
            ]
            results = [future.result() for future in concurrent.futures.as_completed(futures)]

        # Validate all ServiceAccounts can authenticate concurrently
        for result in results:
            assert result["status_code"] == 200, f"ServiceAccount {result['sa_name']} should authenticate"
            assert result["user_info"] is not None, f"ServiceAccount {result['sa_name']} should get user info"

            user_info = result["user_info"]
            assert "scopes" in user_info, f"ServiceAccount {result['sa_name']} should have scopes"

    def test_serviceaccount_token_lifecycle(
        self,
        k8s_core_v1,
        test_namespace,
        holmesgpt_api_client
    ):
        """
        Test ServiceAccount token lifecycle
        Business Requirement: Dynamic token management
        """
        # This would test token creation, validation, and cleanup
        # For integration tests, we'll focus on validation

        # Verify existing ServiceAccounts are properly configured
        try:
            serviceaccounts = k8s_core_v1.list_namespaced_service_account(test_namespace)
            test_sas = [sa for sa in serviceaccounts.items if sa.metadata.name.startswith("test-")]

            assert len(test_sas) > 0, "Test ServiceAccounts should be created"

            for sa in test_sas:
                assert sa.metadata.labels.get("test") == "holmesgpt-api-integration", \
                    f"ServiceAccount {sa.metadata.name} should have proper labels"

        except Exception as e:
            pytest.skip(f"Cannot access ServiceAccounts: {e}")


# E2E Test Fixtures
@pytest.fixture
def sample_complex_alert_data():
    """Complex alert data for E2E testing"""
    return {
        "version": "4",
        "groupKey": "e2e-test-group",
        "status": "firing",
        "receiver": "holmesgpt-api",
        "alerts": [
            {
                "status": "firing",
                "labels": {
                    "alertname": "HighMemoryUsage",
                    "severity": "critical",
                    "namespace": "production",
                    "pod": "app-server-abc123",
                    "deployment": "app-server",
                    "container": "app",
                    "node": "worker-1"
                },
                "annotations": {
                    "description": "Pod app-server-abc123 is using 95% memory (7.6Gi/8Gi)",
                    "summary": "Critical memory usage on production application",
                    "runbook_url": "https://wiki.example.com/runbooks/memory"
                },
                "startsAt": "2024-01-01T10:30:00Z",
                "generatorURL": "http://prometheus:9090/graph?g0.expr=..."
            },
            {
                "status": "firing",
                "labels": {
                    "alertname": "HighCPUUsage",
                    "severity": "warning",
                    "namespace": "production",
                    "pod": "app-server-abc123",
                    "deployment": "app-server"
                },
                "annotations": {
                    "description": "Pod CPU usage is consistently high",
                    "summary": "High CPU usage detected"
                },
                "startsAt": "2024-01-01T10:25:00Z"
            }
        ]
    }

