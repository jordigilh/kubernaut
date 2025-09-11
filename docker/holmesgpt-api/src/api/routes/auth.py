"""
Authentication API Routes
Implements authentication endpoints - BR-HAPI-026 through BR-HAPI-030
"""

from datetime import timedelta
from typing import List

import structlog
from fastapi import APIRouter, HTTPException, Depends, status
from fastapi.security import HTTPAuthorizationCredentials

from models.api_models import (
    LoginRequest, LoginResponse, RefreshRequest, UserResponse,
    CreateUserRequest, UpdateUserRolesRequest
)
from services.auth_service import (
    AuthService, User, Role, Permission,
    get_auth_service, security
)
from config import settings

logger = structlog.get_logger(__name__)

router = APIRouter()


def get_auth_service_dependency() -> AuthService:
    """Dependency injection for auth service"""
    return get_auth_service(settings)


async def get_current_user(
    credentials: HTTPAuthorizationCredentials = Depends(security),
    auth_service: AuthService = Depends(get_auth_service_dependency)
) -> User:
    """Dependency to get current authenticated user"""
    return auth_service.get_current_user(credentials.credentials)


async def get_current_active_user(
    current_user: User = Depends(get_current_user)
) -> User:
    """Dependency to get current active user"""
    if not current_user.active:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Inactive user"
        )
    return current_user


def require_admin(current_user: User = Depends(get_current_active_user)) -> User:
    """Dependency requiring admin role"""
    if Role.ADMIN not in current_user.roles:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Admin access required"
        )
    return current_user


@router.post("/login", response_model=LoginResponse, summary="User Login")
async def login(
    request: LoginRequest,
    auth_service: AuthService = Depends(get_auth_service_dependency)
):
    """
    Authenticate user and return JWT access token.

    **Required permissions**: None (public endpoint)
    """
    user = auth_service.authenticate_user(request.username, request.password)
    if not user:
        logger.warning(f"Failed login attempt for username: {request.username}")
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Incorrect username or password",
            headers={"WWW-Authenticate": "Bearer"}
        )

    # Create access token
    access_token_expires = timedelta(minutes=auth_service.access_token_expire_minutes)
    access_token = auth_service.create_access_token(
        data={
            "sub": user.username,
            "roles": [role.value for role in user.roles],
            "email": user.email
        },
        expires_delta=access_token_expires
    )

    logger.info(f"User '{user.username}' logged in successfully")

    return LoginResponse(
        access_token=access_token,
        token_type="bearer",
        expires_in=auth_service.access_token_expire_minutes * 60,
        user=UserResponse(
            username=user.username,
            email=user.email,
            roles=user.roles,
            active=user.active
        )
    )


@router.post("/logout", summary="User Logout")
async def logout(
    credentials: HTTPAuthorizationCredentials = Depends(security),
    auth_service: AuthService = Depends(get_auth_service_dependency)
):
    """
    Logout user and revoke access token.

    **Required permissions**: Valid authentication
    """
    # Revoke the token
    auth_service.revoke_token(credentials.credentials)

    logger.info("User logged out successfully")

    return {"message": "Successfully logged out"}


@router.get("/me", response_model=UserResponse, summary="Current User Info")
async def get_current_user_info(
    current_user: User = Depends(get_current_active_user)
):
    """
    Get current user information.

    **Required permissions**: Valid authentication
    """
    return UserResponse(
        username=current_user.username,
        email=current_user.email,
        roles=current_user.roles,
        active=current_user.active,
        permissions=current_user.permissions
    )


@router.post("/refresh", response_model=LoginResponse, summary="Refresh Token")
async def refresh_token(
    request: RefreshRequest,
    auth_service: AuthService = Depends(get_auth_service_dependency)
):
    """
    Refresh access token.

    **Required permissions**: Valid refresh token
    """
    try:
        # Verify the current token
        payload = auth_service.verify_token(request.refresh_token)
        username = payload.get("sub")

        # Get user and create new token
        user = auth_service.users_db.get(username)
        if not user or not user.active:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Invalid refresh token"
            )

        # Create new access token
        access_token_expires = timedelta(minutes=auth_service.access_token_expire_minutes)
        access_token = auth_service.create_access_token(
            data={
                "sub": user.username,
                "roles": [role.value for role in user.roles],
                "email": user.email
            },
            expires_delta=access_token_expires
        )

        logger.info(f"Token refreshed for user: {user.username}")

        return LoginResponse(
            access_token=access_token,
            token_type="bearer",
            expires_in=auth_service.access_token_expire_minutes * 60,
            user=UserResponse(
                username=user.username,
                email=user.email,
                roles=user.roles,
                active=user.active
            )
        )

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Token refresh failed: {e}")
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid refresh token"
        )


# Admin endpoints for user management

@router.get("/users", response_model=List[UserResponse], summary="List Users")
async def list_users(
    admin_user: User = Depends(require_admin),
    auth_service: AuthService = Depends(get_auth_service_dependency)
):
    """
    List all users (admin only).

    **Required permissions**: Admin role
    """
    users = [
        UserResponse(
            username=user.username,
            email=user.email,
            roles=user.roles,
            active=user.active
        )
        for user in auth_service.users_db.values()
    ]

    return users


@router.post("/users", response_model=UserResponse, summary="Create User")
async def create_user(
    request: CreateUserRequest,
    admin_user: User = Depends(require_admin),
    auth_service: AuthService = Depends(get_auth_service_dependency)
):
    """
    Create a new user (admin only).

    **Required permissions**: Admin role
    """
    user = auth_service.create_user(
        username=request.username,
        email=request.email,
        password=request.password,
        roles=request.roles,
        active=request.active
    )

    logger.info(f"User '{request.username}' created by admin '{admin_user.username}'")

    return UserResponse(
        username=user.username,
        email=user.email,
        roles=user.roles,
        active=user.active
    )


@router.put("/users/{username}/roles", response_model=UserResponse, summary="Update User Roles")
async def update_user_roles(
    username: str,
    request: UpdateUserRolesRequest,
    admin_user: User = Depends(require_admin),
    auth_service: AuthService = Depends(get_auth_service_dependency)
):
    """
    Update user roles (admin only).

    **Required permissions**: Admin role
    """
    user = auth_service.update_user_roles(username, request.roles)

    logger.info(f"User '{username}' roles updated by admin '{admin_user.username}'")

    return UserResponse(
        username=user.username,
        email=user.email,
        roles=user.roles,
        active=user.active
    )


@router.delete("/users/{username}", summary="Deactivate User")
async def deactivate_user(
    username: str,
    admin_user: User = Depends(require_admin),
    auth_service: AuthService = Depends(get_auth_service_dependency)
):
    """
    Deactivate a user (admin only).

    **Required permissions**: Admin role
    """
    auth_service.deactivate_user(username)

    logger.info(f"User '{username}' deactivated by admin '{admin_user.username}'")

    return {"message": f"User {username} deactivated successfully"}
