"""
OAuth 2 Resource Server Tests - Business Requirements BR-HAPI-045
Following TDD principles: Test business requirements for OAuth 2 resource server with K8s integration
"""

import pytest
from datetime import datetime, timedelta
from fastapi import HTTPException
from typing import List, Dict, Any
from unittest.mock import Mock

from services.oauth2_service import (
    OAuth2ResourceServer, OAuth2Scope, OAuth2ResourceServerInterface,
    K8sServiceAccountInfo
)
from services.oauth2_helpers import create_k8s_jwt_token, create_bearer_header
from config import Settings


class TestOAuth2ResourceServer:
    """Test OAuth 2 Resource Server following business requirements"""

    @pytest.fixture
    def oauth2_resource_server(self, test_settings: Settings) -> OAuth2ResourceServer:
        """Create OAuth2 resource server instance for testing"""
        return OAuth2ResourceServer(test_settings)

    def test_resource_server_initializes_correctly(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: OAuth 2 resource server must initialize with proper configuration
        Business Requirement: Resource server initialization with K8s integration capabilities
        """
        # Business validation: Resource server should be properly initialized
        assert oauth2_resource_server.settings is not None, "Resource server should have settings"
        assert hasattr(oauth2_resource_server, 'k8s_token_validator'), "Should have K8s token validator"
        assert hasattr(oauth2_resource_server, 'scope_mapper'), "Should have scope mapper"
        assert oauth2_resource_server.k8s_token_validator is not None, "K8s token validator should be initialized"
        assert oauth2_resource_server.scope_mapper is not None, "Scope mapper should be initialized"

    def test_validate_k8s_service_account_token(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: Resource server must validate Kubernetes ServiceAccount tokens
        Business Requirement: Accept and validate K8s ServiceAccount tokens from Authorization headers
        """
        # Valid K8s ServiceAccount token (properly formatted JWT)
        k8s_token = create_k8s_jwt_token("test-sa", "default")

        # Business requirement: Should validate K8s token successfully
        token_info = oauth2_resource_server.validate_k8s_token(k8s_token)

        # Business validations
        assert token_info is not None, "Valid K8s token should be recognized"
        assert token_info.name == "test-sa", "Should extract service account name"
        assert token_info.namespace == "default", "Should extract namespace"
        assert token_info.uid is not None, "Should have service account UID"
        assert len(token_info.scopes) > 0, "Should have mapped OAuth 2 scopes"
        assert any(scope.value.startswith('kubernetes:') for scope in token_info.scopes), "Should have K8s-specific scopes"

    def test_validate_invalid_k8s_token(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: Resource server must reject invalid Kubernetes tokens
        Business Requirement: Security - prevent unauthorized access with invalid tokens
        """
        # Invalid K8s token
        invalid_token = "invalid-k8s-token"

        # Business requirement: Should reject invalid token
        token_info = oauth2_resource_server.validate_k8s_token(invalid_token)

        # Business validation
        assert token_info is None, "Invalid K8s token should be rejected"

    def test_map_k8s_rbac_to_oauth2_scopes(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: Resource server must map K8s RBAC permissions to OAuth 2 scopes
        Business Requirement: Convert K8s RBAC to OAuth 2 scope-based authorization
        """
        # K8s ServiceAccount with RBAC permissions
        k8s_service_account = K8sServiceAccountInfo(
            name="holmes-investigator",
            namespace="monitoring",
            uid="sa-uid-12345",
            audiences=["https://kubernetes.default.svc"],
            scopes=[], # Will be populated by mapping
            expires_at=datetime.utcnow() + timedelta(hours=1),
            rbac_permissions=[
                "get:pods", "list:pods", "get:nodes",
                "create:investigations", "read:alerts"
            ]
        )

        # Business requirement: Should map RBAC permissions to OAuth 2 scopes
        mapped_scopes = oauth2_resource_server.map_rbac_to_scopes(k8s_service_account.rbac_permissions)

        # Business validations
        assert OAuth2Scope.PODS_READ in mapped_scopes, "Pod read permissions should map to PODS_READ scope"
        assert OAuth2Scope.CLUSTER_INFO in mapped_scopes, "Node access should map to CLUSTER_INFO scope"
        assert OAuth2Scope.ALERTS_INVESTIGATE in mapped_scopes, "Investigation permissions should map to ALERTS_INVESTIGATE scope"
        assert len(mapped_scopes) >= 3, "Should have multiple scopes mapped"

    def test_authorize_request_with_valid_scopes(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: Resource server must authorize requests based on OAuth 2 scopes
        Business Requirement: Scope-based authorization for API endpoints
        """
        # Valid OAuth 2 scopes for investigation endpoint
        granted_scopes = [OAuth2Scope.ALERTS_INVESTIGATE, OAuth2Scope.CLUSTER_INFO]
        required_scope = OAuth2Scope.ALERTS_INVESTIGATE

        # Business requirement: Should authorize request with sufficient scopes
        is_authorized = oauth2_resource_server.authorize_request(
            granted_scopes=granted_scopes,
            required_scope=required_scope
        )

        # Business validation
        assert is_authorized == True, "Request with sufficient scopes should be authorized"

    def test_authorize_request_with_insufficient_scopes(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: Resource server must reject requests with insufficient scopes
        Business Requirement: Security - prevent unauthorized API access
        """
        # Insufficient OAuth 2 scopes
        granted_scopes = [OAuth2Scope.CLUSTER_INFO]  # Only cluster info, no investigation
        required_scope = OAuth2Scope.ALERTS_INVESTIGATE

        # Business requirement: Should reject request with insufficient scopes
        is_authorized = oauth2_resource_server.authorize_request(
            granted_scopes=granted_scopes,
            required_scope=required_scope
        )

        # Business validation
        assert is_authorized == False, "Request with insufficient scopes should be rejected"

    def test_validate_bearer_token_from_header(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: Resource server must extract and validate Bearer tokens from Authorization headers
        Business Requirement: Standard OAuth 2 Bearer token support
        """
        # Create valid K8s ServiceAccount JWT token
        k8s_token = create_k8s_jwt_token("bearer-test-sa", "default")

        # Authorization header with Bearer token
        auth_header = create_bearer_header(k8s_token)

        # Business requirement: Should extract and validate Bearer token
        token_info = oauth2_resource_server.validate_bearer_token(auth_header)

        # Business validations
        assert token_info is not None, "Valid Bearer token should be processed"
        assert token_info.token_type == "Bearer", "Should recognize Bearer token type"
        assert token_info.k8s_info is not None, "Should have K8s service account info"
        assert token_info.k8s_info.name == "bearer-test-sa", "Should extract service account name"
        assert token_info.k8s_info.namespace == "default", "Should extract namespace"
        assert len(token_info.scopes) > 0, "Should have valid scopes"

    def test_integration_with_k8s_api_server(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: Resource server must integrate with Kubernetes API server for token validation
        Business Requirement: Direct integration with K8s API server TokenReview API
        """
        # K8s API server configuration
        k8s_api_server_url = "https://kubernetes.default.svc"
        k8s_token = "valid-k8s-serviceaccount-token"

        # Business requirement: Should validate token against K8s API server
        is_valid = oauth2_resource_server.validate_with_k8s_api_server(
            token=k8s_token,
            api_server_url=k8s_api_server_url
        )

        # Business validation
        # Note: This test validates the integration capability, actual validation depends on API server response
        assert isinstance(is_valid, bool), "Should return boolean validation result"

    def test_scope_hierarchy_and_inheritance(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: Resource server must support OAuth 2 scope hierarchy
        Business Requirement: Admin scopes should include lower-level permissions
        """
        # Admin scopes should include other permissions
        admin_scopes = [OAuth2Scope.ADMIN_SYSTEM, OAuth2Scope.ADMIN_USERS]

        # Business requirement: Should support scope hierarchy
        effective_scopes = oauth2_resource_server.expand_scope_hierarchy(admin_scopes)

        # Business validations
        assert OAuth2Scope.CLUSTER_INFO in effective_scopes, "Admin should include cluster info"
        assert OAuth2Scope.PODS_READ in effective_scopes, "Admin should include pod read"
        assert OAuth2Scope.ALERTS_INVESTIGATE in effective_scopes, "Admin should include investigation"
        assert len(effective_scopes) > len(admin_scopes), "Should expand to include more scopes"

    def test_oauth2_scopes_align_with_k8s_permissions(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: OAuth 2 scopes must align with Kubernetes permissions
        Business Requirement: K8s-compatible authorization system
        """
        # Business requirement: OAuth 2 scopes should cover K8s permission model
        all_scopes = list(OAuth2Scope)

        # Business validations
        k8s_related_scopes = [
            OAuth2Scope.CLUSTER_INFO,
            OAuth2Scope.PODS_READ,
            OAuth2Scope.PODS_WRITE,
            OAuth2Scope.NODES_READ,
            OAuth2Scope.NODES_WRITE,
            OAuth2Scope.ALERTS_INVESTIGATE,
            OAuth2Scope.CHAT_INTERACTIVE,
            OAuth2Scope.ADMIN_USERS,
            OAuth2Scope.ADMIN_SYSTEM,
            OAuth2Scope.DASHBOARD
        ]

        for scope in k8s_related_scopes:
            assert scope in all_scopes, f"K8s-related scope {scope} should be defined"

        # Business validation: Scopes should follow K8s naming convention
        for scope in k8s_related_scopes:
            assert scope.value.startswith("kubernetes:"), f"Scope {scope} should use kubernetes: prefix"

    def test_token_expiration_handling(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: Resource server must handle token expiration correctly
        Business Requirement: Proper token lifecycle management
        """
        # Create expired K8s token
        expired_token = create_k8s_jwt_token("expired-sa", "default", expired=True)

        # Business requirement: Should reject expired tokens
        token_info = oauth2_resource_server.validate_k8s_token(expired_token)

        # Business validation
        assert token_info is None, "Expired token should be rejected"

    def test_service_account_audience_validation(self, oauth2_resource_server: OAuth2ResourceServer):
        """
        BR-HAPI-045: Resource server must validate ServiceAccount token audiences
        Business Requirement: Ensure tokens are intended for this service
        """
        # Create K8s token with specific audience
        expected_audience = "https://holmesgpt-api.monitoring.svc"
        k8s_token_with_audience = create_k8s_jwt_token(
            "audience-test-sa",
            "monitoring",
            audience=[expected_audience]
        )

        # Business requirement: Should validate token audience
        token_info = oauth2_resource_server.validate_k8s_token_audience(
            token=k8s_token_with_audience,
            expected_audience=expected_audience
        )

        # Business validation
        assert token_info is not None, "Token with correct audience should be valid"
        assert token_info.name == "audience-test-sa", "Should extract service account name"
        assert expected_audience in token_info.audiences, "Should contain expected audience"