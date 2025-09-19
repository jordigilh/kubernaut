"""
OAuth 2 Resource Server - Business Requirements BR-HAPI-045
Implements OAuth 2 resource server compatible with Kubernetes API server
"""

from abc import ABC, abstractmethod
from typing import Dict, List, Optional, Any
from datetime import datetime, timedelta
from enum import Enum
from dataclasses import dataclass

import structlog

from services.oauth2_constants import (
    BEARER_PREFIX, BEARER_PREFIX_LENGTH, ERROR_MESSAGES, LOG_CONTEXT_KEYS,
    SCOPE_HIERARCHY
)
from services.oauth2_helpers import (
    extract_bearer_token, format_subject_string, scope_values_to_list,
    handle_validation_errors
)

logger = structlog.get_logger(__name__)

# Working implementations imported locally to avoid circular imports


class OAuth2Scope(str, Enum):
    """OAuth 2 scopes for Kubernetes-compatible authorization"""
    # Cluster information access
    CLUSTER_INFO = "kubernetes:cluster-info"

    # Pod operations
    PODS_READ = "kubernetes:pods:read"
    PODS_WRITE = "kubernetes:pods:write"

    # Node operations
    NODES_READ = "kubernetes:nodes:read"
    NODES_WRITE = "kubernetes:nodes:write"

    # Alert investigation
    ALERTS_INVESTIGATE = "kubernetes:alerts:investigate"

    # Interactive chat
    CHAT_INTERACTIVE = "kubernetes:chat:interactive"

    # User management (admin)
    ADMIN_USERS = "kubernetes:admin:users"

    # System administration
    ADMIN_SYSTEM = "kubernetes:admin:system"

    # Dashboard access
    DASHBOARD = "kubernetes:dashboard"


@dataclass
class K8sServiceAccountInfo:
    """Kubernetes ServiceAccount token information"""
    name: str
    namespace: str
    uid: str
    audiences: List[str]
    scopes: List[OAuth2Scope]
    expires_at: datetime
    rbac_permissions: Optional[List[str]] = None


@dataclass
class OAuth2TokenInfo:
    """OAuth 2 token validation result"""
    token_type: str
    scopes: List[OAuth2Scope]
    subject: Optional[str]
    expires_at: datetime
    k8s_info: Optional[K8sServiceAccountInfo] = None


class OAuth2ResourceServerInterface(ABC):
    """
    Business contract for OAuth 2 Resource Server
    Handles validation of K8s tokens and scope-based authorization
    """

    @abstractmethod
    def validate_k8s_token(self, token: str) -> Optional[K8sServiceAccountInfo]:
        """
        Validate Kubernetes ServiceAccount token
        Business Requirement: Accept and validate K8s ServiceAccount tokens
        """
        pass

    @abstractmethod
    def validate_bearer_token(self, auth_header: str) -> Optional[OAuth2TokenInfo]:
        """
        Extract and validate Bearer token from Authorization header
        Business Requirement: Standard OAuth 2 Bearer token support
        """
        pass

    @abstractmethod
    def map_rbac_to_scopes(self, rbac_permissions: List[str]) -> List[OAuth2Scope]:
        """
        Map K8s RBAC permissions to OAuth 2 scopes
        Business Requirement: Convert K8s RBAC to OAuth 2 scope-based authorization
        """
        pass

    @abstractmethod
    def authorize_request(
        self,
        granted_scopes: List[OAuth2Scope],
        required_scope: OAuth2Scope
    ) -> bool:
        """
        Authorize request based on OAuth 2 scopes
        Business Requirement: Scope-based authorization for API endpoints
        """
        pass

    @abstractmethod
    def validate_with_k8s_api_server(
        self,
        token: str,
        api_server_url: str
    ) -> bool:
        """
        Validate token against Kubernetes API server
        Business Requirement: Direct integration with K8s API server TokenReview API
        """
        pass

    @abstractmethod
    def expand_scope_hierarchy(self, scopes: List[OAuth2Scope]) -> List[OAuth2Scope]:
        """
        Expand scope hierarchy (admin scopes include lower-level permissions)
        Business Requirement: Support OAuth 2 scope hierarchy
        """
        pass

    @abstractmethod
    def validate_k8s_token_audience(
        self,
        token: str,
        expected_audience: str
    ) -> Optional[K8sServiceAccountInfo]:
        """
        Validate ServiceAccount token audience
        Business Requirement: Ensure tokens are intended for this service
        """
        pass


class K8sTokenValidatorInterface(ABC):
    """
    Business contract for Kubernetes token validation
    Handles K8s ServiceAccount tokens and API server integration
    """

    @abstractmethod
    def validate_k8s_token(
        self,
        token: str
    ) -> Optional[K8sServiceAccountInfo]:
        """
        Validate Kubernetes ServiceAccount token
        Business Requirement: Accept and validate K8s ServiceAccount tokens
        """
        pass

    @abstractmethod
    def map_k8s_to_oauth2_scopes(
        self,
        k8s_info: K8sServiceAccountInfo
    ) -> List[OAuth2Scope]:
        """
        Map K8s permissions to OAuth 2 scopes
        Business Requirement: Integration with K8s RBAC system
        """
        pass

    @abstractmethod
    def validate_k8s_api_server_token(
        self,
        token: str,
        api_server_url: str
    ) -> bool:
        """
        Validate token against K8s API server
        Business Requirement: Direct integration with K8s API server
        """
        pass


class OAuth2ScopeValidatorInterface(ABC):
    """
    Business contract for OAuth 2 scope validation and mapping
    """

    @abstractmethod
    def validate_scope_access(
        self,
        required_scope: OAuth2Scope,
        granted_scopes: List[OAuth2Scope]
    ) -> bool:
        """
        Validate if granted scopes include required scope
        Business Requirement: Scope-based authorization
        """
        pass

    @abstractmethod
    def get_endpoint_required_scope(
        self,
        endpoint_path: str,
        http_method: str
    ) -> OAuth2Scope:
        """
        Get required OAuth 2 scope for API endpoint
        Business Requirement: Map API endpoints to required scopes
        """
        pass

    @abstractmethod
    def migrate_rbac_to_scopes(
        self,
        roles: List[str]
    ) -> List[OAuth2Scope]:
        """
        Migrate existing RBAC roles to OAuth 2 scopes
        Business Requirement: Migration path from current RBAC system
        """
        pass


# Concrete implementation placeholder for TDD
class OAuth2ResourceServer(OAuth2ResourceServerInterface):
    """
    OAuth 2 Resource Server implementation
    Business Requirement: Act as OAuth 2 resource server that validates K8s tokens
    """

    def __init__(self, settings):
        self.settings = settings
        # Initialize K8s token validator and scope mapper (local import to avoid circular dependency)
        from services.k8s_token_validator import K8sTokenValidator as WorkingK8sTokenValidator
        self.k8s_token_validator = WorkingK8sTokenValidator(settings)
        self.scope_mapper = OAuth2ScopeValidator(settings)
        logger.info("OAuth2ResourceServer initialized with K8s integration")

    @handle_validation_errors(ERROR_MESSAGES["k8s_validation_failed"])
    def validate_k8s_token(self, token: str) -> Optional[K8sServiceAccountInfo]:
        """
        Validate Kubernetes ServiceAccount token
        Business Requirement: Accept and validate K8s ServiceAccount tokens
        """
        # Use K8s token validator to validate the token
        k8s_info = self.k8s_token_validator.validate_k8s_token(token)
        if k8s_info:
            # Map K8s permissions to OAuth 2 scopes
            k8s_info.scopes = self.k8s_token_validator.map_k8s_to_oauth2_scopes(k8s_info)
            logger.info("K8s token validated successfully",
                       **{LOG_CONTEXT_KEYS["service_account"]: k8s_info.name,
                          LOG_CONTEXT_KEYS["namespace"]: k8s_info.namespace,
                          LOG_CONTEXT_KEYS["scopes"]: scope_values_to_list(k8s_info.scopes)})
            return k8s_info
        else:
            logger.debug(ERROR_MESSAGES["k8s_validation_failed"])
            return None

    @handle_validation_errors(ERROR_MESSAGES["bearer_validation_failed"])
    def validate_bearer_token(self, auth_header: str) -> Optional[OAuth2TokenInfo]:
        """
        Extract and validate Bearer token from Authorization header
        Business Requirement: Standard OAuth 2 Bearer token support
        """
        # Extract Bearer token from header
        token = extract_bearer_token(auth_header)
        if not token:
            return None

        # Validate as K8s token
        k8s_info = self.validate_k8s_token(token)
        if k8s_info:
            # Create OAuth 2 token info from K8s info
            token_info = OAuth2TokenInfo(
                token_type="Bearer",
                scopes=k8s_info.scopes,
                subject=format_subject_string(k8s_info.namespace, k8s_info.name),
                expires_at=k8s_info.expires_at,
                k8s_info=k8s_info
            )
            logger.debug("Bearer token validated successfully",
                        **{LOG_CONTEXT_KEYS["subject"]: token_info.subject,
                           LOG_CONTEXT_KEYS["scopes"]: scope_values_to_list(token_info.scopes)})
            return token_info
        else:
            logger.debug(ERROR_MESSAGES["bearer_validation_failed"])
            return None

    def map_rbac_to_scopes(self, rbac_permissions: List[str]) -> List[OAuth2Scope]:
        """
        Map K8s RBAC permissions to OAuth 2 scopes
        Business Requirement: Convert K8s RBAC to OAuth 2 scope-based authorization
        """
        # Use scope mapper to convert RBAC permissions
        return self.scope_mapper.migrate_rbac_to_scopes(rbac_permissions)

    def authorize_request(
        self,
        granted_scopes: List[OAuth2Scope],
        required_scope: OAuth2Scope
    ) -> bool:
        """
        Authorize request based on OAuth 2 scopes
        Business Requirement: Scope-based authorization for API endpoints
        """
        # Use scope validator to check authorization
        return self.scope_mapper.validate_scope_access(required_scope, granted_scopes)

    def validate_with_k8s_api_server(
        self,
        token: str,
        api_server_url: str
    ) -> bool:
        """
        Validate token against Kubernetes API server
        Business Requirement: Direct integration with K8s API server TokenReview API
        """
        # Use K8s token validator to check with API server
        return self.k8s_token_validator.validate_k8s_api_server_token(token, api_server_url)

    @handle_validation_errors("Error expanding scope hierarchy")
    def expand_scope_hierarchy(self, scopes: List[OAuth2Scope]) -> List[OAuth2Scope]:
        """
        Expand scope hierarchy (admin scopes include lower-level permissions)
        Business Requirement: Support OAuth 2 scope hierarchy
        """
        expanded_scopes = set(scopes)

        # Expand scopes based on hierarchy configuration
        for scope in scopes:
            scope_value = scope.value
            if scope_value in SCOPE_HIERARCHY:
                # Add all scopes included by this scope
                included_scope_values = SCOPE_HIERARCHY[scope_value]
                for included_value in included_scope_values:
                    # Convert string back to OAuth2Scope enum
                    try:
                        included_scope = OAuth2Scope(included_value)
                        expanded_scopes.add(included_scope)
                    except ValueError:
                        # Skip invalid scope values
                        logger.warning("Invalid scope in hierarchy configuration",
                                     scope=included_value)

        expanded_list = list(expanded_scopes)
        logger.debug("Scope hierarchy expanded",
                    **{LOG_CONTEXT_KEYS["scopes"]: scope_values_to_list(scopes),
                       "expanded_scopes": scope_values_to_list(expanded_list)})

        return expanded_list

    def validate_k8s_token_audience(
        self,
        token: str,
        expected_audience: str
    ) -> Optional[K8sServiceAccountInfo]:
        """
        Validate ServiceAccount token audience
        Business Requirement: Ensure tokens are intended for this service
        """
        # First validate the token
        k8s_info = self.validate_k8s_token(token)
        if not k8s_info:
            return None

        # Check if expected audience is in token audiences
        if expected_audience in k8s_info.audiences:
            logger.debug("Token audience validated successfully",
                        expected_audience=expected_audience,
                        token_audiences=k8s_info.audiences)
            return k8s_info
        else:
            logger.warning("Token audience validation failed",
                          expected_audience=expected_audience,
                          token_audiences=k8s_info.audiences)
            return None


# K8sTokenValidator implementation moved to services/k8s_token_validator.py


class OAuth2ScopeValidator(OAuth2ScopeValidatorInterface):
    """
    OAuth 2 scope validator implementation - to be implemented after tests
    """

    def __init__(self, settings):
        self.settings = settings
        logger.info("OAuth2ScopeValidator initialized")

    def validate_scope_access(
        self,
        required_scope: OAuth2Scope,
        granted_scopes: List[OAuth2Scope]
    ) -> bool:
        """
        Validate if granted scopes include required scope
        Business Requirement: Scope-based authorization
        """
        try:
            # Direct scope match
            if required_scope in granted_scopes:
                logger.debug("Scope access granted - direct match",
                           required_scope=required_scope.value,
                           granted_scopes=[s.value for s in granted_scopes])
                return True

            # Check scope hierarchy - admin scopes include lower-level permissions
            if OAuth2Scope.ADMIN_SYSTEM in granted_scopes:
                # System admin has access to everything
                logger.debug("Scope access granted - admin system override",
                           required_scope=required_scope.value)
                return True

            if OAuth2Scope.ADMIN_USERS in granted_scopes:
                # User admin has access to user-related operations and basic cluster info
                user_admin_scopes = [
                    OAuth2Scope.CLUSTER_INFO,
                    OAuth2Scope.DASHBOARD,
                    OAuth2Scope.PODS_READ
                ]
                if required_scope in user_admin_scopes:
                    logger.debug("Scope access granted - admin users override",
                               required_scope=required_scope.value)
                    return True

            # Write permissions include read permissions
            write_read_mappings = {
                OAuth2Scope.PODS_WRITE: OAuth2Scope.PODS_READ,
                OAuth2Scope.NODES_WRITE: OAuth2Scope.NODES_READ
            }

            for write_scope, read_scope in write_read_mappings.items():
                if write_scope in granted_scopes and required_scope == read_scope:
                    logger.debug("Scope access granted - write includes read",
                               required_scope=required_scope.value,
                               granted_write_scope=write_scope.value)
                    return True

            # Access denied
            logger.debug("Scope access denied",
                        required_scope=required_scope.value,
                        granted_scopes=[s.value for s in granted_scopes])
            return False

        except Exception as e:
            logger.error("Error validating scope access",
                        required_scope=required_scope.value if required_scope else None,
                        error=str(e))
            return False

    def get_endpoint_required_scope(
        self,
        endpoint_path: str,
        http_method: str
    ) -> OAuth2Scope:
        """
        Get required OAuth 2 scope for API endpoint
        Business Requirement: Map API endpoints to required scopes
        """
        try:
            # Define endpoint to scope mappings
            endpoint_scope_mappings = {
                # Investigation endpoints
                ("/investigate", "POST"): OAuth2Scope.ALERTS_INVESTIGATE,
                ("/investigate", "GET"): OAuth2Scope.ALERTS_INVESTIGATE,

                # Chat endpoints
                ("/chat", "POST"): OAuth2Scope.CHAT_INTERACTIVE,
                ("/chat", "GET"): OAuth2Scope.CHAT_INTERACTIVE,

                # Health endpoints - basic cluster info
                ("/health", "GET"): OAuth2Scope.CLUSTER_INFO,
                ("/ready", "GET"): OAuth2Scope.CLUSTER_INFO,
                ("/status", "GET"): OAuth2Scope.CLUSTER_INFO,

                # Auth endpoints - user management
                ("/auth/users", "GET"): OAuth2Scope.ADMIN_USERS,
                ("/auth/users", "POST"): OAuth2Scope.ADMIN_USERS,
                ("/auth/users", "PUT"): OAuth2Scope.ADMIN_USERS,
                ("/auth/users", "DELETE"): OAuth2Scope.ADMIN_USERS,

                # Current user info - cluster info
                ("/auth/me", "GET"): OAuth2Scope.CLUSTER_INFO,
                ("/auth/login", "POST"): OAuth2Scope.CLUSTER_INFO,
                ("/auth/logout", "POST"): OAuth2Scope.CLUSTER_INFO,
                ("/auth/refresh", "POST"): OAuth2Scope.CLUSTER_INFO,

                # Configuration endpoints - system admin
                ("/config", "GET"): OAuth2Scope.ADMIN_SYSTEM,
                ("/config", "PUT"): OAuth2Scope.ADMIN_SYSTEM,

                # Metrics endpoints - cluster info
                ("/metrics", "GET"): OAuth2Scope.CLUSTER_INFO,

                # OAuth 2 endpoints (token introspection/revocation)
                ("/oauth2/introspect", "POST"): OAuth2Scope.CLUSTER_INFO,
                ("/oauth2/userinfo", "GET"): OAuth2Scope.CLUSTER_INFO,
                ("/oauth2/revoke", "POST"): OAuth2Scope.CLUSTER_INFO,
            }

            # Check for exact match
            key = (endpoint_path, http_method.upper())
            if key in endpoint_scope_mappings:
                required_scope = endpoint_scope_mappings[key]
                logger.debug("Endpoint scope mapping found",
                           endpoint=endpoint_path,
                           method=http_method,
                           required_scope=required_scope.value)
                return required_scope

            # Check for path patterns
            if endpoint_path.startswith("/investigate"):
                return OAuth2Scope.ALERTS_INVESTIGATE
            elif endpoint_path.startswith("/chat"):
                return OAuth2Scope.CHAT_INTERACTIVE
            elif endpoint_path.startswith("/auth"):
                if endpoint_path.startswith("/auth/users"):
                    return OAuth2Scope.ADMIN_USERS
                else:
                    return OAuth2Scope.CLUSTER_INFO
            elif endpoint_path.startswith("/config"):
                return OAuth2Scope.ADMIN_SYSTEM
            elif endpoint_path.startswith("/oauth2"):
                return OAuth2Scope.CLUSTER_INFO

            # Default to cluster info for unmatched endpoints
            logger.debug("Using default scope for endpoint",
                        endpoint=endpoint_path,
                        method=http_method,
                        default_scope=OAuth2Scope.CLUSTER_INFO.value)
            return OAuth2Scope.CLUSTER_INFO

        except Exception as e:
            logger.error("Error getting endpoint required scope",
                        endpoint=endpoint_path,
                        method=http_method,
                        error=str(e))
            return OAuth2Scope.CLUSTER_INFO  # Safe default

    def migrate_rbac_to_scopes(
        self,
        roles: List[str]
    ) -> List[OAuth2Scope]:
        """
        Migrate existing RBAC roles to OAuth 2 scopes
        Business Requirement: Migration path from current RBAC system
        """
        try:
            # Define RBAC role to OAuth 2 scope mappings
            rbac_scope_mappings = {
                # System admin roles
                "admin": [
                    OAuth2Scope.ADMIN_SYSTEM,
                    OAuth2Scope.ADMIN_USERS,
                    OAuth2Scope.CLUSTER_INFO,
                    OAuth2Scope.PODS_READ,
                    OAuth2Scope.PODS_WRITE,
                    OAuth2Scope.NODES_READ,
                    OAuth2Scope.NODES_WRITE,
                    OAuth2Scope.ALERTS_INVESTIGATE,
                    OAuth2Scope.CHAT_INTERACTIVE,
                    OAuth2Scope.DASHBOARD
                ],

                # Operator roles
                "operator": [
                    OAuth2Scope.CLUSTER_INFO,
                    OAuth2Scope.PODS_READ,
                    OAuth2Scope.PODS_WRITE,
                    OAuth2Scope.ALERTS_INVESTIGATE,
                    OAuth2Scope.CHAT_INTERACTIVE,
                    OAuth2Scope.DASHBOARD
                ],

                # Viewer roles
                "viewer": [
                    OAuth2Scope.CLUSTER_INFO,
                    OAuth2Scope.PODS_READ,
                    OAuth2Scope.DASHBOARD
                ],

                # API user roles
                "api_user": [
                    OAuth2Scope.ALERTS_INVESTIGATE,
                    OAuth2Scope.CHAT_INTERACTIVE
                ],

                # Investigator role (HolmesGPT specific)
                "investigator": [
                    OAuth2Scope.ALERTS_INVESTIGATE,
                    OAuth2Scope.CHAT_INTERACTIVE,
                    OAuth2Scope.CLUSTER_INFO,
                    OAuth2Scope.PODS_READ
                ],

                # Guest/read-only roles
                "guest": [
                    OAuth2Scope.CLUSTER_INFO
                ],
                "readonly": [
                    OAuth2Scope.CLUSTER_INFO,
                    OAuth2Scope.PODS_READ
                ]
            }

            # Collect all scopes from all roles
            migrated_scopes = []

            for role in roles:
                role_lower = role.lower()

                # Direct role mapping
                if role_lower in rbac_scope_mappings:
                    migrated_scopes.extend(rbac_scope_mappings[role_lower])
                    logger.debug("Migrated RBAC role to scopes",
                               role=role,
                               scopes=[s.value for s in rbac_scope_mappings[role_lower]])

                # Pattern matching for complex roles
                elif "admin" in role_lower:
                    migrated_scopes.extend(rbac_scope_mappings["admin"])
                elif "operator" in role_lower or "edit" in role_lower:
                    migrated_scopes.extend(rbac_scope_mappings["operator"])
                elif "view" in role_lower or "read" in role_lower:
                    migrated_scopes.extend(rbac_scope_mappings["viewer"])
                elif "investigat" in role_lower or "holmes" in role_lower:
                    migrated_scopes.extend(rbac_scope_mappings["investigator"])
                elif "api" in role_lower:
                    migrated_scopes.extend(rbac_scope_mappings["api_user"])
                else:
                    # Default to basic cluster info for unknown roles
                    migrated_scopes.append(OAuth2Scope.CLUSTER_INFO)
                    logger.debug("Unknown RBAC role, using default scope",
                               role=role,
                               default_scope=OAuth2Scope.CLUSTER_INFO.value)

            # Remove duplicates and return
            unique_scopes = list(set(migrated_scopes))

            logger.info("RBAC to OAuth 2 scope migration completed",
                       input_roles=roles,
                       output_scopes=[s.value for s in unique_scopes])

            return unique_scopes

        except Exception as e:
            logger.error("Error migrating RBAC to scopes",
                        roles=roles,
                        error=str(e))
            # Return basic scope as fallback
            return [OAuth2Scope.CLUSTER_INFO]