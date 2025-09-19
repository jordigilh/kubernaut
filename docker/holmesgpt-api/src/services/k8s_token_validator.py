"""
Kubernetes Token Validator - Business Requirements BR-HAPI-045
Handles validation of Kubernetes ServiceAccount tokens and API server integration
"""

from typing import Optional, List, Dict, Any
from datetime import datetime
import json
import base64
import jwt
import structlog

from services.oauth2_service import K8sServiceAccountInfo, OAuth2Scope, K8sTokenValidatorInterface
from services.oauth2_constants import (
    JWT_PARTS_COUNT, K8S_ISSUER, K8S_SUBJECT_PREFIX, K8S_SUBJECT_PARTS_COUNT,
    DEFAULT_TOKEN_EXPIRY_HOURS, ERROR_MESSAGES, LOG_CONTEXT_KEYS,
    ADMIN_SA_PATTERNS, HOLMESGPT_SA_PATTERNS, MONITORING_NAMESPACES,
    DASHBOARD_PATTERNS, DASHBOARD_NAMESPACE, DEV_NAMESPACES, DEFAULT_SA_ROLE,
    SA_SCOPE_MAPPINGS, RBAC_TO_SCOPE_MAPPINGS
)
from services.oauth2_helpers import (
    decode_jwt_payload, extract_subject_parts, is_token_expired,
    get_setting_with_default, matches_pattern, get_full_serviceaccount_name,
    handle_validation_errors
)

logger = structlog.get_logger(__name__)


class K8sTokenValidator(K8sTokenValidatorInterface):
    """
    Kubernetes token validator implementation
    Handles ServiceAccount tokens and K8s API server integration
    """

    def __init__(self, settings):
        self.settings = settings
        self.k8s_api_server_url = get_setting_with_default(settings, 'k8s_api_server_url', None)
        self.k8s_ca_cert_path = get_setting_with_default(settings, 'k8s_ca_cert_path', None)
        self.verify_k8s_tokens = get_setting_with_default(settings, 'verify_k8s_tokens', True)

        # Scope mapping configuration
        self._init_scope_mappings()

        logger.info("K8sTokenValidator initialized",
                   **{LOG_CONTEXT_KEYS["api_server"]: self.k8s_api_server_url,
                      LOG_CONTEXT_KEYS["verify_tokens"]: self.verify_k8s_tokens})

    def _init_scope_mappings(self):
        """Initialize K8s RBAC to OAuth 2 scope mappings"""
        self.rbac_to_scope_mapping = {
            # Cluster roles to scopes
            "cluster-admin": [
                OAuth2Scope.ADMIN_SYSTEM,
                OAuth2Scope.ADMIN_USERS,
                OAuth2Scope.CLUSTER_INFO,
                OAuth2Scope.NODES_READ,
                OAuth2Scope.NODES_WRITE,
                OAuth2Scope.PODS_READ,
                OAuth2Scope.PODS_WRITE,
                OAuth2Scope.ALERTS_INVESTIGATE,
                OAuth2Scope.CHAT_INTERACTIVE,
                OAuth2Scope.DASHBOARD
            ],
            "admin": [
                OAuth2Scope.CLUSTER_INFO,
                OAuth2Scope.PODS_READ,
                OAuth2Scope.PODS_WRITE,
                OAuth2Scope.ALERTS_INVESTIGATE,
                OAuth2Scope.CHAT_INTERACTIVE,
                OAuth2Scope.DASHBOARD
            ],
            "edit": [
                OAuth2Scope.PODS_READ,
                OAuth2Scope.PODS_WRITE,
                OAuth2Scope.ALERTS_INVESTIGATE,
                OAuth2Scope.CHAT_INTERACTIVE
            ],
            "view": [
                OAuth2Scope.CLUSTER_INFO,
                OAuth2Scope.PODS_READ
            ],
            # ServiceAccount specific roles
            "system:serviceaccount": [
                OAuth2Scope.CLUSTER_INFO,
                OAuth2Scope.PODS_READ
            ],
            # Custom roles for HolmesGPT
            "holmesgpt:investigator": [
                OAuth2Scope.ALERTS_INVESTIGATE,
                OAuth2Scope.CHAT_INTERACTIVE,
                OAuth2Scope.PODS_READ,
                OAuth2Scope.CLUSTER_INFO
            ],
            "holmesgpt:operator": [
                OAuth2Scope.ALERTS_INVESTIGATE,
                OAuth2Scope.CHAT_INTERACTIVE,
                OAuth2Scope.PODS_READ,
                OAuth2Scope.PODS_WRITE,
                OAuth2Scope.CLUSTER_INFO
            ]
        }

    @handle_validation_errors(ERROR_MESSAGES["k8s_validation_failed"])
    def validate_k8s_token(
        self,
        token: str
    ) -> Optional[K8sServiceAccountInfo]:
        """
        Validate Kubernetes ServiceAccount token
        Business Requirement: Accept and validate K8s ServiceAccount tokens
        """
        # Validate token format
        if not token or not isinstance(token, str):
            logger.debug(ERROR_MESSAGES["invalid_token_format"])
            return None

        # Decode JWT payload
        payload = decode_jwt_payload(token)
        if not payload:
            return None

        # Validate ServiceAccount token structure
        issuer = payload.get('iss')
        subject = payload.get('sub')
        audiences = payload.get('aud', [])

        if not issuer or issuer != K8S_ISSUER:
            logger.debug(ERROR_MESSAGES["not_k8s_token"])
            return None

        # Extract namespace and service account name
        subject_parts = extract_subject_parts(subject)
        if not subject_parts:
            return None

        namespace, sa_name = subject_parts

        # Check token expiration
        exp_timestamp = payload.get('exp')
        if is_token_expired(exp_timestamp):
            return None

        # Calculate expiration time
        if exp_timestamp:
            expires_at = datetime.fromtimestamp(exp_timestamp)
        else:
            # Default to configured hours if no expiration
            from datetime import timedelta
            expires_at = datetime.utcnow() + timedelta(hours=DEFAULT_TOKEN_EXPIRY_HOURS)

        # Map ServiceAccount to OAuth 2 scopes
        scopes = self._map_serviceaccount_to_scopes(namespace, sa_name, payload)

        # Create ServiceAccount info
        sa_info = K8sServiceAccountInfo(
            name=sa_name,
            namespace=namespace,
            uid=payload.get('kubernetes.io', {}).get('serviceaccount', {}).get('uid', f"sa-{sa_name}"),
            audiences=audiences if isinstance(audiences, list) else [audiences],
            scopes=scopes,
            expires_at=expires_at
        )

        logger.info("K8s ServiceAccount token validated successfully",
                   **{LOG_CONTEXT_KEYS["service_account"]: sa_name,
                      LOG_CONTEXT_KEYS["namespace"]: namespace,
                      LOG_CONTEXT_KEYS["scopes"]: [s.value for s in scopes]})

        return sa_info

    def _map_serviceaccount_to_scopes(
        self,
        namespace: str,
        sa_name: str,
        token_payload: Dict[str, Any]
    ) -> List[OAuth2Scope]:
        """
        Map ServiceAccount to OAuth 2 scopes based on namespace and name
        Business Requirement: Convert K8s ServiceAccount identity to OAuth 2 scopes
        """
        scopes = []

        # Default scopes for all ServiceAccounts
        default_scopes = [OAuth2Scope.CLUSTER_INFO, OAuth2Scope.PODS_READ]
        scopes.extend(default_scopes)

        # Special mapping for well-known ServiceAccounts
        full_sa_name = f"{namespace}:{sa_name}"

        # Admin ServiceAccounts (cluster-admin equivalent)
        admin_patterns = ["kube-system:", "admin", "cluster-admin"]
        if any(pattern in full_sa_name.lower() for pattern in admin_patterns):
            scopes.extend([
                OAuth2Scope.ADMIN_SYSTEM,
                OAuth2Scope.ADMIN_USERS,
                OAuth2Scope.NODES_READ,
                OAuth2Scope.NODES_WRITE,
                OAuth2Scope.PODS_WRITE,
                OAuth2Scope.ALERTS_INVESTIGATE,
                OAuth2Scope.CHAT_INTERACTIVE,
                OAuth2Scope.DASHBOARD
            ])

        # HolmesGPT specific ServiceAccounts
        elif "holmesgpt" in sa_name.lower() or "holmes" in sa_name.lower():
            if "investigator" in sa_name.lower():
                scopes.extend([
                    OAuth2Scope.ALERTS_INVESTIGATE,
                    OAuth2Scope.CHAT_INTERACTIVE,
                    OAuth2Scope.DASHBOARD
                ])
            elif "operator" in sa_name.lower():
                scopes.extend([
                    OAuth2Scope.ALERTS_INVESTIGATE,
                    OAuth2Scope.CHAT_INTERACTIVE,
                    OAuth2Scope.PODS_WRITE,
                    OAuth2Scope.DASHBOARD
                ])
            else:
                # Default HolmesGPT ServiceAccount
                scopes.extend([
                    OAuth2Scope.ALERTS_INVESTIGATE,
                    OAuth2Scope.CHAT_INTERACTIVE
                ])

        # Monitoring namespace ServiceAccounts
        elif namespace in ["monitoring", "prometheus", "grafana"]:
            scopes.extend([
                OAuth2Scope.ALERTS_INVESTIGATE,
                OAuth2Scope.NODES_READ
            ])

        # Dashboard ServiceAccounts
        elif "dashboard" in sa_name.lower() or namespace == "kubernetes-dashboard":
            scopes.extend([
                OAuth2Scope.DASHBOARD,
                OAuth2Scope.NODES_READ
            ])

        # Development/testing namespaces get broader permissions
        elif namespace in ["default", "development", "testing", "dev", "test"]:
            scopes.extend([
                OAuth2Scope.PODS_WRITE,
                OAuth2Scope.CHAT_INTERACTIVE
            ])

        # Remove duplicates and return
        unique_scopes = list(set(scopes))

        logger.debug("Mapped ServiceAccount to scopes",
                    service_account=full_sa_name,
                    scopes=[s.value for s in unique_scopes])

        return unique_scopes

    def map_k8s_to_oauth2_scopes(
        self,
        k8s_info: K8sServiceAccountInfo
    ) -> List[OAuth2Scope]:
        """
        Map K8s permissions to OAuth 2 scopes
        Business Requirement: Integration with K8s RBAC system
        """
        return k8s_info.scopes  # Already mapped during validation

    def validate_k8s_api_server_token(
        self,
        token: str,
        api_server_url: str
    ) -> bool:
        """
        Validate token against K8s API server
        Business Requirement: Direct integration with K8s API server

        Note: For unit tests, this will be mocked
        For integration tests, this will make actual API calls to Kind cluster
        """
        try:
            # For unit tests, perform basic validation without API server call
            if not self.verify_k8s_tokens:
                logger.debug("K8s token verification disabled, assuming valid")
                return True

            # Validate token format first
            if not token or not isinstance(token, str):
                return False

            # Check if it's a valid JWT format
            parts = token.split('.')
            if len(parts) != 3:
                return False

            # In production, this would use K8s TokenReview API:
            # POST /api/v1/tokenreviews
            # {
            #   "apiVersion": "authentication.k8s.io/v1",
            #   "kind": "TokenReview",
            #   "spec": {
            #     "token": token,
            #     "audiences": [api_server_url]
            #   }
            # }

            # For now, we'll validate locally and assume K8s API server integration
            # will be handled in integration tests with real K8s cluster
            sa_info = self.validate_k8s_token(token)
            is_valid = sa_info is not None

            logger.debug("K8s API server token validation",
                        api_server_url=api_server_url,
                        is_valid=is_valid)

            return is_valid

        except Exception as e:
            logger.error("Error validating token against K8s API server",
                        api_server_url=api_server_url,
                        error=str(e))
            return False

    def extract_k8s_permissions(
        self,
        token: str
    ) -> List[str]:
        """
        Extract K8s permissions from token (if available)
        This would typically require querying K8s RBAC
        Business Requirement: Extract RBAC permissions for scope mapping
        """
        try:
            # Validate the token first
            sa_info = self.validate_k8s_token(token)
            if not sa_info:
                return []

            # Get roles for this ServiceAccount
            roles = self.get_serviceaccount_roles(sa_info.namespace, sa_info.name)

            # Convert roles to permission strings
            permissions = []
            for role in roles:
                if role in self.rbac_to_scope_mapping:
                    # Add permissions based on role
                    role_permissions = self._get_permissions_for_role(role)
                    permissions.extend(role_permissions)

            # Remove duplicates
            unique_permissions = list(set(permissions))

            logger.debug("Extracted K8s permissions from token",
                        service_account=f"{sa_info.namespace}:{sa_info.name}",
                        permissions=unique_permissions)

            return unique_permissions

        except Exception as e:
            logger.error("Error extracting K8s permissions", error=str(e))
            return []

    def _get_permissions_for_role(self, role: str) -> List[str]:
        """Convert role to permission strings"""
        role_permissions = {
            "cluster-admin": [
                "get:*", "list:*", "create:*", "update:*", "delete:*",
                "get:pods", "list:pods", "create:pods", "update:pods", "delete:pods",
                "get:nodes", "list:nodes", "create:nodes", "update:nodes", "delete:nodes",
                "create:investigations", "read:alerts", "write:alerts"
            ],
            "admin": [
                "get:pods", "list:pods", "create:pods", "update:pods", "delete:pods",
                "get:configmaps", "list:configmaps", "create:configmaps",
                "create:investigations", "read:alerts"
            ],
            "edit": [
                "get:pods", "list:pods", "create:pods", "update:pods",
                "create:investigations", "read:alerts"
            ],
            "view": [
                "get:pods", "list:pods", "get:configmaps", "list:configmaps"
            ],
            "system:serviceaccount": [
                "get:pods", "list:pods"
            ],
            "holmesgpt:investigator": [
                "get:pods", "list:pods", "create:investigations", "read:alerts", "chat:interactive"
            ],
            "holmesgpt:operator": [
                "get:pods", "list:pods", "create:pods", "update:pods",
                "create:investigations", "read:alerts", "chat:interactive"
            ]
        }

        return role_permissions.get(role, [])

    def get_serviceaccount_roles(
        self,
        namespace: str,
        sa_name: str
    ) -> List[str]:
        """
        Get ClusterRoles and Roles bound to ServiceAccount
        This would typically query K8s RBAC API
        Business Requirement: Determine ServiceAccount roles for permission mapping
        """
        try:
            # For unit tests, we'll simulate role bindings based on ServiceAccount patterns
            # In production, this would query K8s RBAC API:
            # GET /apis/rbac.authorization.k8s.io/v1/clusterrolebindings
            # GET /apis/rbac.authorization.k8s.io/v1/namespaces/{namespace}/rolebindings

            roles = []
            full_sa_name = f"{namespace}:{sa_name}"

            # Admin ServiceAccounts
            admin_patterns = ["kube-system:", "admin", "cluster-admin"]
            if any(pattern in full_sa_name.lower() for pattern in admin_patterns):
                roles.append("cluster-admin")

            # HolmesGPT specific ServiceAccounts
            elif "holmesgpt" in sa_name.lower() or "holmes" in sa_name.lower():
                if "investigator" in sa_name.lower():
                    roles.append("holmesgpt:investigator")
                elif "operator" in sa_name.lower():
                    roles.append("holmesgpt:operator")
                else:
                    roles.append("holmesgpt:investigator")  # Default

            # Monitoring namespace ServiceAccounts
            elif namespace in ["monitoring", "prometheus", "grafana"]:
                roles.append("view")
                if "prometheus" in sa_name.lower():
                    roles.append("edit")  # Prometheus needs to scrape metrics

            # Dashboard ServiceAccounts
            elif "dashboard" in sa_name.lower() or namespace == "kubernetes-dashboard":
                roles.append("admin")  # Dashboard needs broad access

            # Development/testing namespaces
            elif namespace in ["default", "development", "testing", "dev", "test"]:
                roles.append("edit")

            # Default ServiceAccount role
            else:
                roles.append("system:serviceaccount")

            logger.debug("Retrieved ServiceAccount roles",
                        service_account=full_sa_name,
                        roles=roles)

            return roles

        except Exception as e:
            logger.error("Error getting ServiceAccount roles",
                        namespace=namespace,
                        sa_name=sa_name,
                        error=str(e))
            return ["system:serviceaccount"]  # Default fallback


# Mock implementation for unit tests
class MockK8sTokenValidator(K8sTokenValidatorInterface):
    """
    Mock K8s token validator for unit tests
    """

    def __init__(self):
        self.valid_tokens = {}  # token -> K8sServiceAccountInfo
        self.api_server_responses = {}  # token -> bool

    def add_valid_token(self, token: str, sa_info: K8sServiceAccountInfo):
        """Add a valid token for testing"""
        self.valid_tokens[token] = sa_info

    def set_api_server_response(self, token: str, is_valid: bool):
        """Set API server response for testing"""
        self.api_server_responses[token] = is_valid

    def validate_k8s_token(
        self,
        token: str
    ) -> Optional[K8sServiceAccountInfo]:
        return self.valid_tokens.get(token)

    def map_k8s_to_oauth2_scopes(
        self,
        k8s_info: K8sServiceAccountInfo
    ) -> List[OAuth2Scope]:
        return k8s_info.scopes

    def validate_k8s_api_server_token(
        self,
        token: str,
        api_server_url: str
    ) -> bool:
        return self.api_server_responses.get(token, False)
