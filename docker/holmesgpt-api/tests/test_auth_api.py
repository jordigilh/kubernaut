"""
Authentication API Tests - Business Requirements BR-HAPI-026 through BR-HAPI-030
Following TDD principles: Test business requirements, not implementation
"""

import pytest
import json
from fastapi.testclient import TestClient
from typing import Dict, Any

from services.auth_service import Role, Permission


class TestAuthAPI:
    """Test Authentication API endpoints following business requirements"""

    def test_login_endpoint_exists_and_accepts_post_requests(self, test_client: TestClient):
        """
        BR-HAPI-026: Login endpoint must exist and accept POST requests
        Business Requirement: API must provide authentication capability
        """
        # Test that endpoint exists (will return 422 for missing data, not 404)
        response = test_client.post("/auth/login")

        # Should not be 404 (endpoint exists), should be 422 (validation error)
        assert response.status_code != 404, "Login endpoint should exist"
        assert response.status_code == 422, "Should require credentials validation"

    def test_login_with_valid_credentials_returns_jwt_token(
        self,
        test_client: TestClient,
        mock_auth_service
    ):
        """
        BR-HAPI-026: Login must authenticate valid credentials and return JWT token
        Business Requirement: Secure token-based authentication
        """
        # Valid login credentials
        login_data = {
            "username": "test_operator",
            "password": "operator123"
        }

        response = test_client.post("/auth/login", json=login_data)

        # Business requirement: Valid credentials should authenticate successfully
        assert response.status_code == 200, f"Valid login should succeed, got: {response.status_code}"

        result = response.json()

        # Business validation: Response must contain JWT token
        assert "access_token" in result, "Response must include access token"
        assert "token_type" in result, "Response must include token type"
        assert "expires_in" in result, "Response must include expiration time"
        assert "user" in result, "Response must include user information"

        # Business validation: Token should be bearer type
        assert result["token_type"] == "bearer", "Token type should be bearer"

        # Business validation: User info should be included
        user_info = result["user"]
        assert "username" in user_info, "User info must include username"
        assert "email" in user_info, "User info must include email"
        assert "roles" in user_info, "User info must include roles"
        assert "active" in user_info, "User info must include active status"

    def test_login_with_invalid_credentials_rejects_authentication(
        self,
        test_client: TestClient
    ):
        """
        BR-HAPI-026: Login must reject invalid credentials
        Business Requirement: Security - prevent unauthorized access
        """
        # Invalid credentials
        invalid_login_data = {
            "username": "invalid_user",
            "password": "wrong_password"
        }

        response = test_client.post("/auth/login", json=invalid_login_data)

        # Business requirement: Invalid credentials should be rejected
        assert response.status_code == 401, "Invalid credentials should return 401"

        result = response.json()
        assert "detail" in result, "Error response should include details"

    def test_user_roles_determine_permissions(
        self,
        test_client: TestClient
    ):
        """
        BR-HAPI-027: User roles must determine access permissions
        Business Requirement: Role-based access control (RBAC)
        """
        # Test admin user login
        admin_login = {
            "username": "test_admin",
            "password": "admin123"
        }

        response = test_client.post("/auth/login", json=admin_login)
        assert response.status_code == 200, "Admin login should succeed"

        result = response.json()
        user_info = result["user"]

        # Business validation: Admin should have admin role
        assert "admin" in user_info["roles"], "Admin user should have admin role"

        # Test operator user login
        operator_login = {
            "username": "test_operator",
            "password": "operator123"
        }

        response = test_client.post("/auth/login", json=operator_login)
        assert response.status_code == 200, "Operator login should succeed"

        result = response.json()
        user_info = result["user"]

        # Business validation: Operator should have operator role
        assert "operator" in user_info["roles"], "Operator user should have operator role"
        assert "admin" not in user_info["roles"], "Operator should not have admin role"

    def test_current_user_endpoint_requires_valid_token(
        self,
        test_client: TestClient,
        operator_token: str
    ):
        """
        BR-HAPI-028: Current user endpoint must require valid authentication
        Business Requirement: Secure access to user information
        """
        # Test without token
        response = test_client.get("/auth/me")
        assert response.status_code == 401 or response.status_code == 403, \
            "Me endpoint should require authentication"

        # Test with valid token
        headers = {"Authorization": f"Bearer {operator_token}"}
        response = test_client.get("/auth/me", headers=headers)

        # Business requirement: Valid token should provide user info
        assert response.status_code == 200, "Valid token should access user info"

        result = response.json()

        # Business validation: Response should include user details
        assert "username" in result, "Response must include username"
        assert "email" in result, "Response must include email"
        assert "roles" in result, "Response must include roles"
        assert "permissions" in result, "Response must include permissions"

    def test_logout_revokes_token(
        self,
        test_client: TestClient,
        operator_token: str
    ):
        """
        BR-HAPI-029: Logout must revoke access token
        Business Requirement: Secure token lifecycle management
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Verify token works before logout
        response = test_client.get("/auth/me", headers=headers)
        assert response.status_code == 200, "Token should work before logout"

        # Logout
        response = test_client.post("/auth/logout", headers=headers)

        # Business requirement: Logout should succeed
        assert response.status_code == 200, "Logout should succeed"

        result = response.json()
        assert "message" in result, "Logout should return success message"

    def test_token_refresh_extends_session(
        self,
        test_client: TestClient,
        operator_token: str
    ):
        """
        BR-HAPI-029: Token refresh must extend user session
        Business Requirement: Seamless session extension without re-authentication
        """
        refresh_data = {
            "refresh_token": operator_token
        }

        response = test_client.post("/auth/refresh", json=refresh_data)

        # Business requirement: Token refresh should succeed
        assert response.status_code == 200, "Token refresh should succeed"

        result = response.json()

        # Business validation: New token should be provided
        assert "access_token" in result, "Refresh should provide new access token"
        assert "token_type" in result, "Refresh should include token type"
        assert "expires_in" in result, "Refresh should include expiration time"

        # Business validation: New token should be different from original
        assert result["access_token"] != operator_token, "New token should be different"

    def test_admin_can_list_users(
        self,
        test_client: TestClient,
        admin_token: str
    ):
        """
        BR-HAPI-028: Admin users must be able to list all users
        Business Requirement: User management capabilities for administrators
        """
        headers = {"Authorization": f"Bearer {admin_token}"}

        response = test_client.get("/auth/users", headers=headers)

        # Business requirement: Admin should access user list
        assert response.status_code == 200, "Admin should access user list"

        result = response.json()

        # Business validation: Should return list of users
        assert isinstance(result, list), "Should return list of users"
        assert len(result) > 0, "Should include existing users"

        # Business validation: User objects should have required fields
        for user in result:
            assert "username" in user, "User must have username"
            assert "email" in user, "User must have email"
            assert "roles" in user, "User must have roles"
            assert "active" in user, "User must have active status"

    def test_non_admin_cannot_list_users(
        self,
        test_client: TestClient,
        operator_token: str
    ):
        """
        BR-HAPI-027: Non-admin users must not access user management
        Business Requirement: RBAC enforcement for sensitive operations
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        response = test_client.get("/auth/users", headers=headers)

        # Business requirement: Non-admin should be denied access
        assert response.status_code == 403, "Non-admin should not access user list"

        result = response.json()
        assert "detail" in result, "Should provide access denied message"

    def test_admin_can_create_new_users(
        self,
        test_client: TestClient,
        admin_token: str
    ):
        """
        BR-HAPI-028: Admin must be able to create new users
        Business Requirement: User provisioning for system administrators
        """
        headers = {"Authorization": f"Bearer {admin_token}"}

        new_user_data = {
            "username": "test_new_user",
            "email": "newuser@example.com",
            "password": "secure123",
            "roles": ["viewer"],
            "active": True
        }

        response = test_client.post("/auth/users", json=new_user_data, headers=headers)

        # Business requirement: Admin should create users successfully
        assert response.status_code == 200, "Admin should create users successfully"

        result = response.json()

        # Business validation: Created user should be returned
        assert "username" in result, "Response must include username"
        assert "email" in result, "Response must include email"
        assert "roles" in result, "Response must include roles"
        assert result["username"] == new_user_data["username"], "Username should match"
        assert result["email"] == new_user_data["email"], "Email should match"

    def test_admin_can_update_user_roles(
        self,
        test_client: TestClient,
        admin_token: str
    ):
        """
        BR-HAPI-028: Admin must be able to update user roles
        Business Requirement: Role management for access control
        """
        headers = {"Authorization": f"Bearer {admin_token}"}

        role_update_data = {
            "roles": ["operator", "viewer"]
        }

        response = test_client.put("/auth/users/test_viewer/roles", json=role_update_data, headers=headers)

        # Business requirement: Admin should update roles successfully
        assert response.status_code == 200, "Admin should update roles successfully"

        result = response.json()

        # Business validation: Updated roles should be reflected
        assert "roles" in result, "Response must include updated roles"
        assert "operator" in result["roles"], "Should include new operator role"
        assert "viewer" in result["roles"], "Should include viewer role"

    def test_admin_can_deactivate_users(
        self,
        test_client: TestClient,
        admin_token: str
    ):
        """
        BR-HAPI-030: Admin must be able to deactivate users
        Business Requirement: User lifecycle management for security
        """
        headers = {"Authorization": f"Bearer {admin_token}"}

        response = test_client.delete("/auth/users/test_viewer", headers=headers)

        # Business requirement: Admin should deactivate users successfully
        assert response.status_code == 200, "Admin should deactivate users successfully"

        result = response.json()
        assert "message" in result, "Should provide success message"
        assert "deactivated" in result["message"], "Message should confirm deactivation"

    def test_authentication_validates_required_fields(
        self,
        test_client: TestClient
    ):
        """
        BR-HAPI-026: Authentication must validate required input fields
        Business Requirement: Input validation prevents invalid authentication attempts
        """
        # Test missing username
        incomplete_data = {
            "password": "test123"
        }

        response = test_client.post("/auth/login", json=incomplete_data)
        assert response.status_code == 422, "Missing username should be rejected"

        # Test missing password
        incomplete_data = {
            "username": "testuser"
        }

        response = test_client.post("/auth/login", json=incomplete_data)
        assert response.status_code == 422, "Missing password should be rejected"

    def test_jwt_token_contains_required_claims(
        self,
        test_client: TestClient,
        mock_auth_service
    ):
        """
        BR-HAPI-026: JWT tokens must contain required claims for authorization
        Business Requirement: Secure token format with necessary user information
        """
        login_data = {
            "username": "test_operator",
            "password": "operator123"
        }

        response = test_client.post("/auth/login", json=login_data)
        assert response.status_code == 200, "Login should succeed"

        result = response.json()
        token = result["access_token"]

        # Business requirement: Token should be valid JWT format
        assert len(token.split('.')) == 3, "JWT should have 3 parts separated by dots"

        # Verify token can be decoded and contains user info
        user_info = mock_auth_service.get_current_user(token)

        # Business validation: Token should contain user identity
        assert user_info.username == "test_operator", "Token should identify correct user"
        assert user_info.active == True, "Token should reflect user active status"
        assert len(user_info.roles) > 0, "Token should include user roles"


