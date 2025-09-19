"""
Auth Service Tests - Business Requirements BR-HAPI-026 through BR-HAPI-030
Following TDD principles: Test business requirements, not implementation
"""

import pytest
from datetime import timedelta
from fastapi import HTTPException

from services.auth_service import AuthService, User, Role, Permission
from config import Settings


class TestAuthService:
    """Test Auth Service following business requirements"""

    @pytest.fixture
    def auth_service(self, test_settings: Settings) -> AuthService:
        """Create auth service instance for testing"""
        return AuthService(test_settings)

    def test_service_initializes_with_default_users(self, auth_service: AuthService):
        """
        BR-HAPI-026: Service must initialize with default user accounts
        Business Requirement: Bootstrap authentication system with admin access
        """
        # Business validation: Default users should exist
        assert "admin" in auth_service.users_db, "Admin user should exist by default"
        assert "operator" in auth_service.users_db, "Operator user should exist by default"

        # Business validation: Admin should have admin role
        admin_user = auth_service.users_db["admin"]
        assert Role.ADMIN in admin_user.roles, "Admin user should have admin role"

        # Business validation: Users should be active by default
        assert admin_user.active == True, "Default users should be active"

    def test_password_hashing_and_verification(self, auth_service: AuthService):
        """
        BR-HAPI-026: Service must securely hash and verify passwords
        Business Requirement: Secure password storage and authentication
        """
        password = "test_password_123"

        # Business requirement: Password should be hashed
        hashed = auth_service.hash_password(password)

        # Business validation: Hash should be different from original
        assert hashed != password, "Hashed password should differ from original"
        assert len(hashed) > 20, "Hash should be substantial length"

        # Business validation: Verification should work correctly
        assert auth_service.verify_password(password, hashed) == True, "Should verify correct password"
        assert auth_service.verify_password("wrong_password", hashed) == False, "Should reject wrong password"

    def test_user_authentication_with_valid_credentials(self, auth_service: AuthService):
        """
        BR-HAPI-026: Service must authenticate users with valid credentials
        Business Requirement: Secure user authentication for API access
        """
        # Business requirement: Valid credentials should authenticate
        user = auth_service.authenticate_user("admin", "admin123")

        # Business validation: Authentication should succeed
        assert user is not None, "Valid credentials should authenticate"
        assert user.username == "admin", "Should return correct user"
        assert user.active == True, "Authenticated user should be active"

    def test_user_authentication_rejects_invalid_credentials(self, auth_service: AuthService):
        """
        BR-HAPI-026: Service must reject invalid credentials
        Business Requirement: Security - prevent unauthorized access
        """
        # Business requirement: Invalid username should be rejected
        user = auth_service.authenticate_user("nonexistent_user", "any_password")
        assert user is None, "Invalid username should be rejected"

        # Business requirement: Invalid password should be rejected
        user = auth_service.authenticate_user("admin", "wrong_password")
        assert user is None, "Invalid password should be rejected"

        # Business requirement: Inactive user should be rejected
        auth_service.users_db["admin"].active = False
        user = auth_service.authenticate_user("admin", "admin123")
        assert user is None, "Inactive user should be rejected"

    def test_jwt_token_creation_and_verification(self, auth_service: AuthService):
        """
        BR-HAPI-026: Service must create and verify JWT tokens
        Business Requirement: Token-based authentication for stateless API access
        """
        # Test data for token creation
        token_data = {
            "sub": "test_user",
            "roles": ["operator"],
            "email": "test@example.com"
        }

        # Business requirement: Should create valid JWT token
        token = auth_service.create_access_token(token_data)

        # Business validation: Token should be valid JWT format
        assert isinstance(token, str), "Token should be string"
        assert len(token.split('.')) == 3, "JWT should have 3 parts separated by dots"

        # Business validation: Token should be verifiable
        payload = auth_service.verify_token(token)
        assert payload["sub"] == "test_user", "Token should contain subject"
        assert "exp" in payload, "Token should have expiration"

    def test_jwt_token_expiration_handling(self, auth_service: AuthService):
        """
        BR-HAPI-029: Service must handle token expiration appropriately
        Business Requirement: Secure token lifecycle with automatic expiration
        """
        token_data = {"sub": "test_user"}

        # Create token with very short expiration
        short_expiry = timedelta(seconds=-1)  # Already expired
        expired_token = auth_service.create_access_token(token_data, short_expiry)

        # Business requirement: Expired token should be rejected
        with pytest.raises(HTTPException) as exc_info:
            auth_service.verify_token(expired_token)

        # Business validation: Should indicate token expiration
        assert exc_info.value.status_code == 401, "Expired token should return 401"
        assert "expired" in exc_info.value.detail.lower(), "Error should mention expiration"

    def test_token_revocation_blacklist(self, auth_service: AuthService):
        """
        BR-HAPI-029: Service must support token revocation
        Business Requirement: Secure logout and token invalidation
        """
        token_data = {"sub": "test_user"}
        token = auth_service.create_access_token(token_data)

        # Token should be valid initially
        payload = auth_service.verify_token(token)
        assert payload["sub"] == "test_user", "Token should be valid initially"

        # Business requirement: Token revocation should work
        auth_service.revoke_token(token)

        # Business validation: Revoked token should be rejected
        with pytest.raises(HTTPException) as exc_info:
            auth_service.verify_token(token)

        assert exc_info.value.status_code == 401, "Revoked token should return 401"
        assert "revoked" in exc_info.value.detail.lower(), "Error should mention revocation"

    def test_role_based_permissions_mapping(self, auth_service: AuthService):
        """
        BR-HAPI-027: Service must implement role-based permissions
        Business Requirement: Granular access control based on user roles
        """
        # Create users with different roles
        admin_user = User("admin_test", "admin@test.com", [Role.ADMIN])
        operator_user = User("operator_test", "op@test.com", [Role.OPERATOR])
        viewer_user = User("viewer_test", "view@test.com", [Role.VIEWER])

        # Business validation: Admin should have all permissions
        admin_permissions = admin_user.permissions
        assert Permission.ADMIN_SYSTEM in admin_permissions, "Admin should have admin permissions"
        assert Permission.INVESTIGATE_ALERTS in admin_permissions, "Admin should have investigate permissions"
        assert Permission.MANAGE_USERS in admin_permissions, "Admin should have user management permissions"

        # Business validation: Operator should have investigation permissions
        operator_permissions = operator_user.permissions
        assert Permission.INVESTIGATE_ALERTS in operator_permissions, "Operator should have investigate permissions"
        assert Permission.CHAT_INTERACTIVE in operator_permissions, "Operator should have chat permissions"
        assert Permission.MANAGE_USERS not in operator_permissions, "Operator should not have user management"

        # Business validation: Viewer should have limited permissions
        viewer_permissions = viewer_user.permissions
        assert Permission.VIEW_HEALTH in viewer_permissions, "Viewer should have health view permissions"
        assert Permission.INVESTIGATE_ALERTS not in viewer_permissions, "Viewer should not have investigate permissions"

    def test_permission_checking_methods(self, auth_service: AuthService):
        """
        BR-HAPI-027: Service must provide permission checking capabilities
        Business Requirement: Enforce access control based on user permissions
        """
        # Create test user with specific permissions
        test_user = User("test_user", "test@test.com", [Role.OPERATOR])

        # Business validation: Should correctly identify user permissions
        assert test_user.has_permission(Permission.INVESTIGATE_ALERTS) == True, "Should have investigation permission"
        assert test_user.has_permission(Permission.ADMIN_SYSTEM) == False, "Should not have admin permission"

        # Business validation: Should correctly check roles
        assert test_user.has_any_role([Role.OPERATOR]) == True, "Should have operator role"
        assert test_user.has_any_role([Role.ADMIN]) == False, "Should not have admin role"
        assert test_user.has_any_role([Role.ADMIN, Role.OPERATOR]) == True, "Should match any of multiple roles"

    def test_user_creation_by_admin(self, auth_service: AuthService):
        """
        BR-HAPI-028: Service must support user creation by administrators
        Business Requirement: User provisioning and management capabilities
        """
        # Business requirement: Should create new user successfully
        new_user = auth_service.create_user(
            username="new_test_user",
            email="new@test.com",
            password="secure123",
            roles=[Role.VIEWER],
            active=True
        )

        # Business validation: User should be created correctly
        assert new_user.username == "new_test_user", "User should have correct username"
        assert new_user.email == "new@test.com", "User should have correct email"
        assert Role.VIEWER in new_user.roles, "User should have assigned role"
        assert new_user.active == True, "User should be active"

        # Business validation: User should be stored in database
        assert "new_test_user" in auth_service.users_db, "User should be stored in database"

        # Business validation: Should prevent duplicate usernames
        with pytest.raises(HTTPException) as exc_info:
            auth_service.create_user(
                username="new_test_user",  # Same username
                email="different@test.com",
                password="password123",
                roles=[Role.VIEWER]
            )
        assert exc_info.value.status_code == 400, "Duplicate username should be rejected"

    def test_user_role_updates_by_admin(self, auth_service: AuthService):
        """
        BR-HAPI-028: Service must support user role updates by administrators
        Business Requirement: Dynamic role management for access control
        """
        # Create test user
        auth_service.create_user(
            username="role_test_user",
            email="roletest@test.com",
            password="test123",
            roles=[Role.VIEWER]
        )

        # Business requirement: Should update user roles successfully
        updated_user = auth_service.update_user_roles(
            username="role_test_user",
            roles=[Role.OPERATOR, Role.VIEWER]
        )

        # Business validation: Roles should be updated
        assert Role.OPERATOR in updated_user.roles, "Should have new operator role"
        assert Role.VIEWER in updated_user.roles, "Should retain viewer role"

        # Business validation: Non-existent user should be rejected
        with pytest.raises(HTTPException) as exc_info:
            auth_service.update_user_roles("nonexistent_user", [Role.VIEWER])
        assert exc_info.value.status_code == 404, "Non-existent user should return 404"

    def test_user_deactivation_by_admin(self, auth_service: AuthService):
        """
        BR-HAPI-030: Service must support user deactivation by administrators
        Business Requirement: User lifecycle management for security
        """
        # Create test user
        auth_service.create_user(
            username="deactivate_test_user",
            email="deactivate@test.com",
            password="test123",
            roles=[Role.VIEWER]
        )

        # Verify user is initially active
        user = auth_service.users_db["deactivate_test_user"]
        assert user.active == True, "User should be initially active"

        # Business requirement: Should deactivate user successfully
        deactivated_user = auth_service.deactivate_user("deactivate_test_user")

        # Business validation: User should be deactivated
        assert deactivated_user.active == False, "User should be deactivated"

        # Business validation: Deactivated user should not authenticate
        auth_result = auth_service.authenticate_user("deactivate_test_user", "test123")
        assert auth_result is None, "Deactivated user should not authenticate"

    def test_permission_enforcement_methods(self, auth_service: AuthService):
        """
        BR-HAPI-027: Service must enforce permission requirements
        Business Requirement: Access control enforcement for API endpoints
        """
        # Create users with different permission levels
        admin_user = User("admin_test", "admin@test.com", [Role.ADMIN])
        viewer_user = User("viewer_test", "viewer@test.com", [Role.VIEWER])

        # Business validation: Admin should pass permission checks
        try:
            auth_service.require_permission(admin_user, Permission.ADMIN_SYSTEM)
            # Should not raise exception
        except HTTPException:
            pytest.fail("Admin should have admin permissions")

        # Business validation: Viewer should fail admin permission check
        with pytest.raises(HTTPException) as exc_info:
            auth_service.require_permission(viewer_user, Permission.ADMIN_SYSTEM)
        assert exc_info.value.status_code == 403, "Insufficient permissions should return 403"

        # Business validation: Role enforcement should work
        try:
            auth_service.require_role(admin_user, Role.ADMIN)
            # Should not raise exception
        except HTTPException:
            pytest.fail("Admin should have admin role")

        with pytest.raises(HTTPException) as exc_info:
            auth_service.require_role(viewer_user, Role.ADMIN)
        assert exc_info.value.status_code == 403, "Insufficient role should return 403"

    def test_get_current_user_from_token(self, auth_service: AuthService):
        """
        BR-HAPI-026: Service must extract current user from JWT token
        Business Requirement: Token-based user identification for API requests
        """
        # Create token for existing user
        token_data = {
            "sub": "admin",
            "roles": ["admin"],
            "email": "admin@kubernaut.local"
        }
        token = auth_service.create_access_token(token_data)

        # Business requirement: Should extract user from valid token
        current_user = auth_service.get_current_user(token)

        # Business validation: Should return correct user
        assert current_user.username == "admin", "Should extract correct user"
        assert current_user.active == True, "Should return active user"
        assert Role.ADMIN in current_user.roles, "Should have correct roles"

        # Business validation: Invalid token should be rejected
        with pytest.raises(HTTPException) as exc_info:
            auth_service.get_current_user("invalid_token")
        assert exc_info.value.status_code == 401, "Invalid token should return 401"


