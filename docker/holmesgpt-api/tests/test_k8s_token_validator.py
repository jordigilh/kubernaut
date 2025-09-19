"""
K8s Token Validator Tests - Business Requirements BR-HAPI-045
Following TDD principles: Test Kubernetes ServiceAccount token validation
"""

import pytest
from datetime import datetime, timedelta
from typing import List

from services.k8s_token_validator import K8sTokenValidator, MockK8sTokenValidator
from services.oauth2_service import K8sServiceAccountInfo, OAuth2Scope
from services.oauth2_helpers import create_k8s_jwt_token
from config import Settings


class TestK8sTokenValidator:
    """Test K8s Token Validator following business requirements"""

    @pytest.fixture
    def k8s_validator(self, test_settings: Settings) -> K8sTokenValidator:
        """Create K8s token validator instance for testing"""
        return K8sTokenValidator(test_settings)

    def test_validator_initializes_correctly(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: K8s token validator must initialize with proper configuration
        Business Requirement: Service initialization for K8s integration
        """
        # Business validation: Validator should be properly initialized
        assert k8s_validator.settings is not None, "Validator should have settings"
        assert k8s_validator.rbac_to_scope_mapping is not None, "Should have RBAC mapping"
        assert hasattr(k8s_validator, 'verify_k8s_tokens'), "Should have verification setting"

    def test_validate_k8s_service_account_token_valid(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: Validator must accept valid Kubernetes ServiceAccount tokens
        Business Requirement: Integration with K8s ServiceAccount authentication
        """
        # Create valid K8s ServiceAccount token using helper
        valid_k8s_token = create_k8s_jwt_token("test-sa", "default")

        # Business requirement: Valid K8s token should be validated successfully
        sa_info = k8s_validator.validate_k8s_token(valid_k8s_token)

        # Business validations
        assert sa_info is not None, "Valid K8s token should be recognized"
        assert sa_info.name == "test-sa", "ServiceAccount name should be extracted"
        assert sa_info.namespace == "default", "ServiceAccount namespace should be extracted"
        assert sa_info.uid is not None, "ServiceAccount UID should be extracted"
        assert len(sa_info.scopes) > 0, "ServiceAccount should have mapped OAuth 2 scopes"
        assert sa_info.expires_at > datetime.utcnow(), "Token should not be expired"

    def test_validate_k8s_service_account_token_invalid_format(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: Validator must reject invalid token formats
        Business Requirement: Security - prevent malformed token acceptance
        """
        # Invalid token formats
        invalid_tokens = [
            "not-a-jwt-token",
            "invalid.jwt.format",
            "",
            "Bearer invalid-token",
            "eyJhbGciOiJIUzI1NiJ9.invalid.signature"  # Not a K8s SA token
        ]

        for invalid_token in invalid_tokens:
            # Business requirement: Invalid tokens should be rejected
            sa_info = k8s_validator.validate_k8s_token(invalid_token)

            # Business validation
            assert sa_info is None, f"Invalid token should be rejected: {invalid_token[:20]}..."

    def test_validate_k8s_token_non_service_account_subject(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: Validator must reject tokens with non-ServiceAccount subjects
        Business Requirement: Only accept ServiceAccount tokens for K8s integration
        """
        # Mock JWT with non-ServiceAccount subject
        user_token = """eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwic3ViIjoidXNlcjp0ZXN0LXVzZXIiLCJleHAiOjE5MDAwMDAwMDB9.signature"""

        # Business requirement: Non-ServiceAccount tokens should be rejected
        sa_info = k8s_validator.validate_k8s_token(user_token)

        # Business validation
        assert sa_info is None, "Non-ServiceAccount token should be rejected"

    def test_map_service_account_to_oauth2_scopes_default(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: Validator must map ServiceAccount to OAuth 2 scopes
        Business Requirement: Convert K8s permissions to OAuth 2 scopes
        """
        # Default ServiceAccount mapping
        namespace = "default"
        sa_name = "test-sa"
        token_payload = {
            "iss": "kubernetes/serviceaccount",
            "sub": "system:serviceaccount:default:test-sa"
        }

        # Business requirement: Should map to default scopes
        scopes = k8s_validator._map_serviceaccount_to_scopes(namespace, sa_name, token_payload)

        # Business validations
        assert OAuth2Scope.CLUSTER_INFO in scopes, "Should include cluster info scope"
        assert OAuth2Scope.PODS_READ in scopes, "Should include pods read scope"
        assert len(scopes) >= 2, "Should have at least default scopes"

    def test_map_service_account_to_oauth2_scopes_admin(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: Validator must map admin ServiceAccounts to admin scopes
        Business Requirement: Grant appropriate permissions based on SA name
        """
        # Admin ServiceAccount mapping
        namespace = "kube-system"
        sa_name = "admin-sa"
        token_payload = {
            "iss": "kubernetes/serviceaccount",
            "sub": "system:serviceaccount:kube-system:admin-sa"
        }

        # Business requirement: Should map to admin scopes
        scopes = k8s_validator._map_serviceaccount_to_scopes(namespace, sa_name, token_payload)

        # Business validations
        assert OAuth2Scope.ADMIN_SYSTEM in scopes, "Admin SA should have system admin scope"
        assert OAuth2Scope.ADMIN_USERS in scopes, "Admin SA should have user admin scope"
        assert OAuth2Scope.PODS_WRITE in scopes, "Admin SA should have write permissions"

    def test_map_service_account_to_oauth2_scopes_holmesgpt(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: Validator must map HolmesGPT ServiceAccounts to appropriate scopes
        Business Requirement: Grant HolmesGPT-specific permissions
        """
        # HolmesGPT ServiceAccount mapping
        namespace = "holmesgpt"
        sa_name = "holmesgpt-investigator"
        token_payload = {
            "iss": "kubernetes/serviceaccount",
            "sub": "system:serviceaccount:holmesgpt:holmesgpt-investigator"
        }

        # Business requirement: Should map to HolmesGPT scopes
        scopes = k8s_validator._map_serviceaccount_to_scopes(namespace, sa_name, token_payload)

        # Business validations
        assert OAuth2Scope.ALERTS_INVESTIGATE in scopes, "HolmesGPT SA should investigate alerts"
        assert OAuth2Scope.CHAT_INTERACTIVE in scopes, "HolmesGPT SA should use chat"
        assert OAuth2Scope.CLUSTER_INFO in scopes, "HolmesGPT SA should access cluster info"

    def test_validate_k8s_api_server_token_integration(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: Validator must validate tokens against K8s API server
        Business Requirement: Direct integration with K8s API server for validation
        Note: This will use mocks for unit tests, real API calls for integration tests
        """
        # Token to validate against API server
        k8s_token = "valid-k8s-api-token"
        api_server_url = "https://k8s-api.test.local"

        # Business requirement: Should validate against K8s API server
        is_valid = k8s_validator.validate_k8s_api_server_token(k8s_token, api_server_url)

        # Business validation
        assert isinstance(is_valid, bool), "Should return boolean validation result"

    def test_extract_k8s_permissions_from_token(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: Validator should extract K8s permissions from token context
        Business Requirement: Understand ServiceAccount permissions for scope mapping
        """
        # Token with K8s permissions context
        k8s_token = "token-with-permissions"

        # Business requirement: Should extract K8s permissions
        permissions = k8s_validator.extract_k8s_permissions(k8s_token)

        # Business validations
        assert isinstance(permissions, list), "Should return list of permissions"
        if permissions:  # If permissions are found
            k8s_verbs = ["get", "list", "watch", "create", "update", "patch", "delete"]
            for permission in permissions:
                assert permission in k8s_verbs, f"Permission should be valid K8s verb: {permission}"

    def test_get_service_account_roles(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: Validator should query ServiceAccount roles from K8s RBAC
        Business Requirement: Understand bound roles for accurate scope mapping
        """
        # ServiceAccount to query roles for
        namespace = "production"
        sa_name = "app-service-account"

        # Business requirement: Should get bound roles for ServiceAccount
        roles = k8s_validator.get_serviceaccount_roles(namespace, sa_name)

        # Business validations
        assert isinstance(roles, list), "Should return list of roles"
        if roles:  # If roles are found
            k8s_roles = ["admin", "edit", "view", "cluster-admin", "system:serviceaccount"]
            for role in roles:
                # Business validation: Should be valid K8s role names
                assert any(k8s_role in role for k8s_role in k8s_roles), f"Should be valid K8s role: {role}"

    def test_scope_mapping_consistency(self, k8s_validator: K8sTokenValidator):
        """
        BR-HAPI-045: RBAC to OAuth 2 scope mapping must be consistent and comprehensive
        Business Requirement: Reliable permission translation between systems
        """
        # Business validation: RBAC mapping should be comprehensive
        rbac_mapping = k8s_validator.rbac_to_scope_mapping

        # Should include all standard K8s roles
        standard_roles = ["cluster-admin", "admin", "edit", "view"]
        for role in standard_roles:
            assert role in rbac_mapping, f"Should include standard K8s role: {role}"
            assert len(rbac_mapping[role]) > 0, f"Role {role} should have mapped scopes"

        # Should include HolmesGPT-specific roles
        holmesgpt_roles = ["holmesgpt:investigator", "holmesgpt:operator"]
        for role in holmesgpt_roles:
            assert role in rbac_mapping, f"Should include HolmesGPT role: {role}"

            # HolmesGPT roles should include investigation scopes
            role_scopes = rbac_mapping[role]
            assert OAuth2Scope.ALERTS_INVESTIGATE in role_scopes, f"HolmesGPT role should investigate alerts: {role}"


class TestMockK8sTokenValidator:
    """Test Mock K8s Token Validator for unit testing"""

    @pytest.fixture
    def mock_validator(self) -> MockK8sTokenValidator:
        """Create mock K8s token validator for testing"""
        return MockK8sTokenValidator()

    def test_mock_validator_accepts_configured_tokens(self, mock_validator: MockK8sTokenValidator):
        """
        BR-HAPI-045: Mock validator must support configurable token responses
        Business Requirement: Enable comprehensive unit testing
        """
        # Configure mock validator with test token
        test_token = "test-k8s-token"
        test_sa_info = K8sServiceAccountInfo(
            name="mock-sa",
            namespace="test",
            uid="mock-uid",
            audiences=["https://kubernetes.default.svc"],
            scopes=[OAuth2Scope.CLUSTER_INFO, OAuth2Scope.PODS_READ],
            expires_at=datetime.utcnow() + timedelta(hours=1)
        )

        mock_validator.add_valid_token(test_token, test_sa_info)

        # Business requirement: Should return configured token info
        result = mock_validator.validate_k8s_token(test_token)

        # Business validations
        assert result is not None, "Mock should return configured token info"
        assert result.name == test_sa_info.name, "Should return correct SA name"
        assert result.namespace == test_sa_info.namespace, "Should return correct namespace"
        assert result.scopes == test_sa_info.scopes, "Should return correct scopes"

    def test_mock_validator_rejects_unconfigured_tokens(self, mock_validator: MockK8sTokenValidator):
        """
        BR-HAPI-045: Mock validator must reject unconfigured tokens
        Business Requirement: Realistic test behavior for invalid tokens
        """
        # Business requirement: Unconfigured tokens should be rejected
        result = mock_validator.validate_k8s_token("unknown-token")

        # Business validation
        assert result is None, "Mock should reject unconfigured tokens"

    def test_mock_validator_configurable_api_server_responses(self, mock_validator: MockK8sTokenValidator):
        """
        BR-HAPI-045: Mock validator must support configurable API server responses
        Business Requirement: Test API server integration scenarios
        """
        # Configure API server response
        test_token = "api-server-token"
        mock_validator.set_api_server_response(test_token, True)

        # Business requirement: Should return configured API server response
        result = mock_validator.validate_k8s_api_server_token(test_token, "https://mock-api-server")

        # Business validation
        assert result == True, "Mock should return configured API server response"

        # Test negative case
        mock_validator.set_api_server_response("invalid-token", False)
        result = mock_validator.validate_k8s_api_server_token("invalid-token", "https://mock-api-server")
        assert result == False, "Mock should return configured negative response"
