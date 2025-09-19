"""
OAuth 2 Resource Server Constants - Refactoring Phase
Central location for all constants used across OAuth 2 and K8s integration
"""

from typing import Dict, List

# =====================================================
# JWT and Token Constants
# =====================================================

# JWT structure constants
JWT_HEADER_RS256 = "eyJhbGciOiJSUzI1NiJ9"
JWT_PARTS_COUNT = 3
JWT_PADDING_CHAR = "="
JWT_PADDING_MODULO = 4

# Authorization header constants
BEARER_PREFIX = "Bearer "
BEARER_PREFIX_LENGTH = 7

# Kubernetes ServiceAccount token constants
K8S_ISSUER = "kubernetes/serviceaccount"
K8S_SUBJECT_PREFIX = "system:serviceaccount:"
K8S_SUBJECT_PARTS_COUNT = 4

# Default token expiration
DEFAULT_TOKEN_EXPIRY_HOURS = 1

# =====================================================
# Kubernetes Constants
# =====================================================

# Default audiences
DEFAULT_K8S_AUDIENCE = "https://kubernetes.default.svc"

# ServiceAccount patterns for role mapping
ADMIN_SA_PATTERNS = ["kube-system:", "admin", "cluster-admin"]
HOLMESGPT_SA_PATTERNS = ["holmesgpt", "holmes"]
MONITORING_NAMESPACES = ["monitoring", "prometheus", "grafana"]
DASHBOARD_PATTERNS = ["dashboard"]
DASHBOARD_NAMESPACE = "kubernetes-dashboard"
DEV_NAMESPACES = ["default", "development", "testing", "dev", "test"]

# Default roles
DEFAULT_SA_ROLE = "system:serviceaccount"

# =====================================================
# Scope Hierarchy Configuration
# =====================================================

# Admin scope hierarchy - what scopes are included by admin scopes
SCOPE_HIERARCHY: Dict[str, List[str]] = {
    "kubernetes:admin:system": [
        # System admin includes everything
        "kubernetes:cluster-info",
        "kubernetes:pods:read",
        "kubernetes:pods:write",
        "kubernetes:nodes:read",
        "kubernetes:nodes:write",
        "kubernetes:alerts:investigate",
        "kubernetes:chat:interactive",
        "kubernetes:admin:users",
        "kubernetes:dashboard"
    ],
    "kubernetes:admin:users": [
        # User admin includes user-related and basic operations
        "kubernetes:cluster-info",
        "kubernetes:dashboard",
        "kubernetes:pods:read"
    ],
    "kubernetes:pods:write": [
        # Write permissions include read permissions
        "kubernetes:pods:read"
    ],
    "kubernetes:nodes:write": [
        "kubernetes:nodes:read"
    ]
}

# =====================================================
# RBAC to Scope Mapping Configuration
# =====================================================

# ServiceAccount to scope mappings based on patterns
SA_SCOPE_MAPPINGS: Dict[str, List[str]] = {
    # Default ServiceAccount gets basic access
    "default": [
        "kubernetes:cluster-info",
        "kubernetes:pods:read",
        "kubernetes:chat:interactive",
        "kubernetes:pods:write"
    ],

    # Testing ServiceAccounts get similar access
    "test": [
        "kubernetes:cluster-info",
        "kubernetes:pods:read",
        "kubernetes:chat:interactive",
        "kubernetes:pods:write"
    ],

    # HolmesGPT investigator ServiceAccounts
    "holmesgpt-investigator": [
        "kubernetes:alerts:investigate",
        "kubernetes:cluster-info",
        "kubernetes:pods:read",
        "kubernetes:chat:interactive",
        "kubernetes:dashboard"
    ],

    # HolmesGPT operator ServiceAccounts
    "holmesgpt-operator": [
        "kubernetes:alerts:investigate",
        "kubernetes:cluster-info",
        "kubernetes:pods:read",
        "kubernetes:pods:write",
        "kubernetes:nodes:read",
        "kubernetes:chat:interactive",
        "kubernetes:dashboard"
    ],

    # Admin ServiceAccounts
    "admin": [
        "kubernetes:admin:system"  # This will expand via hierarchy
    ],

    # Monitoring ServiceAccounts
    "prometheus": [
        "kubernetes:cluster-info",
        "kubernetes:pods:read",
        "kubernetes:nodes:read"
    ],

    # Dashboard ServiceAccounts
    "dashboard": [
        "kubernetes:admin:users"  # This will expand via hierarchy
    ]
}

# RBAC permission to scope direct mappings
RBAC_TO_SCOPE_MAPPINGS: Dict[str, str] = {
    # Pod operations
    "get:pods": "kubernetes:pods:read",
    "list:pods": "kubernetes:pods:read",
    "watch:pods": "kubernetes:pods:read",
    "create:pods": "kubernetes:pods:write",
    "update:pods": "kubernetes:pods:write",
    "delete:pods": "kubernetes:pods:write",
    "patch:pods": "kubernetes:pods:write",

    # Node operations
    "get:nodes": "kubernetes:nodes:read",
    "list:nodes": "kubernetes:nodes:read",
    "create:nodes": "kubernetes:nodes:write",
    "update:nodes": "kubernetes:nodes:write",
    "delete:nodes": "kubernetes:nodes:write",

    # Investigation operations
    "create:investigations": "kubernetes:alerts:investigate",
    "read:alerts": "kubernetes:alerts:investigate",
    "get:alerts": "kubernetes:alerts:investigate",
    "list:investigations": "kubernetes:alerts:investigate",

    # Chat operations
    "create:chat": "kubernetes:chat:interactive",
    "read:chat": "kubernetes:chat:interactive",

    # Dashboard operations
    "access:dashboard": "kubernetes:dashboard",
    "view:dashboard": "kubernetes:dashboard",

    # Admin operations
    "create:users": "kubernetes:admin:users",
    "delete:users": "kubernetes:admin:users",
    "update:users": "kubernetes:admin:users",
    "manage:cluster": "kubernetes:admin:system"
}

# =====================================================
# Error Messages
# =====================================================

ERROR_MESSAGES = {
    "invalid_token_format": "Invalid token format",
    "invalid_jwt_format": "Invalid JWT format",
    "failed_jwt_decode": "Failed to decode JWT payload",
    "not_k8s_token": "Not a Kubernetes ServiceAccount token",
    "invalid_sa_subject": "Invalid ServiceAccount subject",
    "invalid_sa_subject_format": "Invalid ServiceAccount subject format",
    "token_expired": "Token has expired",
    "invalid_auth_header": "Invalid Authorization header format",
    "bearer_validation_failed": "Bearer token validation failed",
    "k8s_validation_failed": "K8s token validation failed",
    "audience_validation_failed": "Token audience validation failed"
}

# =====================================================
# Logging Contexts
# =====================================================

LOG_CONTEXT_KEYS = {
    "service_account": "service_account",
    "namespace": "namespace",
    "scopes": "scopes",
    "subject": "subject",
    "required_scope": "required_scope",
    "granted_scopes": "granted_scopes",
    "token_audiences": "token_audiences",
    "expected_audience": "expected_audience",
    "current_time": "current_time",
    "expires_at": "expires_at",
    "error": "error",
    "api_server": "api_server",
    "verify_tokens": "verify_tokens",
    "roles": "roles"
}


