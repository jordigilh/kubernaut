"""
Authentication Service - JWT and RBAC implementation
Implements security requirements - BR-HAPI-026 through BR-HAPI-030
"""

import json
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Union
from enum import Enum

import jwt
import structlog
from passlib.context import CryptContext
from fastapi import HTTPException, status
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials

from config import Settings

logger = structlog.get_logger(__name__)

# Password encryption context
pwd_context = CryptContext(schemes=["bcrypt"], deprecated="auto")

# Security scheme for FastAPI
security = HTTPBearer()


class Role(str, Enum):
    """User roles for RBAC"""
    ADMIN = "admin"
    OPERATOR = "operator"
    VIEWER = "viewer"
    API_USER = "api_user"


class Permission(str, Enum):
    """Granular permissions"""
    INVESTIGATE_ALERTS = "investigate:alerts"
    CHAT_INTERACTIVE = "chat:interactive"
    VIEW_HEALTH = "view:health"
    VIEW_CONFIG = "view:config"
    MANAGE_USERS = "manage:users"
    ADMIN_SYSTEM = "admin:system"


# Role-based permissions mapping
ROLE_PERMISSIONS = {
    Role.VIEWER: [
        Permission.VIEW_HEALTH,
        Permission.VIEW_CONFIG
    ],
    Role.API_USER: [
        Permission.INVESTIGATE_ALERTS,
        Permission.CHAT_INTERACTIVE,
        Permission.VIEW_HEALTH,
        Permission.VIEW_CONFIG
    ],
    Role.OPERATOR: [
        Permission.INVESTIGATE_ALERTS,
        Permission.CHAT_INTERACTIVE,
        Permission.VIEW_HEALTH,
        Permission.VIEW_CONFIG
    ],
    Role.ADMIN: [
        Permission.INVESTIGATE_ALERTS,
        Permission.CHAT_INTERACTIVE,
        Permission.VIEW_HEALTH,
        Permission.VIEW_CONFIG,
        Permission.MANAGE_USERS,
        Permission.ADMIN_SYSTEM
    ]
}


class User:
    """User model for authentication"""

    def __init__(
        self,
        username: str,
        email: str,
        roles: List[Role],
        hashed_password: Optional[str] = None,
        active: bool = True,
        metadata: Optional[Dict] = None
    ):
        self.username = username
        self.email = email
        self.roles = roles
        self.hashed_password = hashed_password
        self.active = active
        self.metadata = metadata or {}

    @property
    def permissions(self) -> List[Permission]:
        """Get all permissions for this user based on roles"""
        perms = set()
        for role in self.roles:
            perms.update(ROLE_PERMISSIONS.get(role, []))
        return list(perms)

    def has_permission(self, permission: Permission) -> bool:
        """Check if user has specific permission"""
        return permission in self.permissions

    def has_any_role(self, roles: List[Role]) -> bool:
        """Check if user has any of the specified roles"""
        return any(role in self.roles for role in roles)


class AuthService:
    """
    Authentication and Authorization Service

    Implements JWT token management and RBAC:
    - JWT token generation and validation (BR-HAPI-026)
    - Role-based access control (BR-HAPI-027)
    - User management (BR-HAPI-028)
    - Token refresh and revocation (BR-HAPI-029)
    """

    def __init__(self, settings: Settings):
        self.settings = settings
        self.algorithm = settings.jwt_algorithm
        self.secret_key = settings.jwt_secret_key
        self.access_token_expire_minutes = settings.access_token_expire_minutes

        # In production, this would be a proper user store (database, LDAP, etc.)
        self.users_db: Dict[str, User] = self._init_default_users()

        # Token blacklist for revocation (in production, use Redis or similar)
        self.blacklisted_tokens: set = set()

    def _init_default_users(self) -> Dict[str, User]:
        """Initialize default users - in production, load from external source"""
        return {
            "admin": User(
                username="admin",
                email="admin@kubernaut.local",
                roles=[Role.ADMIN],
                hashed_password=self.hash_password("admin123")
            ),
            "operator": User(
                username="operator",
                email="operator@kubernaut.local",
                roles=[Role.OPERATOR],
                hashed_password=self.hash_password("operator123")
            ),
            "api_service": User(
                username="api_service",
                email="api@kubernaut.local",
                roles=[Role.API_USER],
                hashed_password=self.hash_password("service123")
            )
        }

    def hash_password(self, password: str) -> str:
        """Hash a password for storing"""
        return pwd_context.hash(password)

    def verify_password(self, plain_password: str, hashed_password: str) -> bool:
        """Verify a password against hash"""
        return pwd_context.verify(plain_password, hashed_password)

    def authenticate_user(self, username: str, password: str) -> Optional[User]:
        """Authenticate user credentials"""
        user = self.users_db.get(username)
        if not user:
            logger.warning(f"Authentication failed: user '{username}' not found")
            return None

        if not user.active:
            logger.warning(f"Authentication failed: user '{username}' is inactive")
            return None

        if not self.verify_password(password, user.hashed_password):
            logger.warning(f"Authentication failed: invalid password for user '{username}'")
            return None

        logger.info(f"User '{username}' authenticated successfully")
        return user

    def create_access_token(
        self,
        data: Dict,
        expires_delta: Optional[timedelta] = None
    ) -> str:
        """Create JWT access token"""
        to_encode = data.copy()

        if expires_delta:
            expire = datetime.utcnow() + expires_delta
        else:
            expire = datetime.utcnow() + timedelta(minutes=self.access_token_expire_minutes)

        # Add unique identifier and issued at time to ensure token uniqueness
        import uuid
        to_encode.update({
            "exp": expire,
            "iat": datetime.utcnow(),  # Issued at time
            "jti": str(uuid.uuid4())   # JWT ID for uniqueness
        })

        try:
            encoded_jwt = jwt.encode(to_encode, self.secret_key, algorithm=self.algorithm)
            logger.info(f"JWT token created for user: {data.get('sub', 'unknown')}")
            return encoded_jwt
        except Exception as e:
            logger.error(f"Failed to create JWT token: {e}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Failed to create access token"
            )

    def verify_token(self, token: str) -> Dict:
        """Verify and decode JWT token"""
        if token in self.blacklisted_tokens:
            logger.warning("Attempted to use blacklisted token")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Token has been revoked"
            )

        try:
            payload = jwt.decode(token, self.secret_key, algorithms=[self.algorithm])
            username: str = payload.get("sub")
            if username is None:
                logger.warning("Token missing subject claim")
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Invalid token payload"
                )
            return payload
        except jwt.ExpiredSignatureError:
            logger.warning("Token has expired")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Token expired"
            )
        except jwt.PyJWTError as e:
            logger.warning(f"JWT validation error: {e}")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Invalid token"
            )

    def get_current_user(self, token: str) -> User:
        """Get current user from JWT token"""
        payload = self.verify_token(token)
        username = payload.get("sub")

        user = self.users_db.get(username)
        if user is None:
            logger.warning(f"User '{username}' not found in database")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="User not found"
            )

        if not user.active:
            logger.warning(f"User '{username}' is inactive")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Inactive user"
            )

        return user

    def revoke_token(self, token: str):
        """Add token to blacklist"""
        self.blacklisted_tokens.add(token)
        logger.info("Token revoked and added to blacklist")

    def require_permission(self, user: User, permission: Permission):
        """Check if user has required permission"""
        if not user.has_permission(permission):
            logger.warning(
                f"User '{user.username}' denied access: missing permission '{permission}'"
            )
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail=f"Insufficient permissions. Required: {permission}"
            )

    def require_role(self, user: User, roles: Union[Role, List[Role]]):
        """Check if user has required role(s)"""
        if isinstance(roles, Role):
            roles = [roles]

        if not user.has_any_role(roles):
            logger.warning(
                f"User '{user.username}' denied access: missing required roles {roles}"
            )
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail=f"Insufficient role. Required: {roles}"
            )

    def create_user(
        self,
        username: str,
        email: str,
        password: str,
        roles: List[Role],
        active: bool = True
    ) -> User:
        """Create a new user (admin only)"""
        if username in self.users_db:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Username already exists"
            )

        user = User(
            username=username,
            email=email,
            roles=roles,
            hashed_password=self.hash_password(password),
            active=active
        )

        self.users_db[username] = user
        logger.info(f"User '{username}' created with roles: {roles}")
        return user

    def update_user_roles(self, username: str, roles: List[Role]) -> User:
        """Update user roles (admin only)"""
        user = self.users_db.get(username)
        if not user:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="User not found"
            )

        user.roles = roles
        logger.info(f"User '{username}' roles updated to: {roles}")
        return user

    def deactivate_user(self, username: str) -> User:
        """Deactivate user (admin only)"""
        user = self.users_db.get(username)
        if not user:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="User not found"
            )

        user.active = False
        logger.info(f"User '{username}' deactivated")
        return user


# Global auth service instance
auth_service = None

def get_auth_service(settings: Settings) -> AuthService:
    """Get or create auth service singleton"""
    global auth_service
    if auth_service is None:
        auth_service = AuthService(settings)
    return auth_service
