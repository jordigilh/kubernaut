"""
OAuth 2 Resource Server Models - Business Requirements BR-HAPI-045
Pydantic models for OAuth 2 resource server operations compatible with K8s
"""

from typing import Dict, List, Any, Optional
from datetime import datetime

from pydantic import BaseModel, Field
from services.oauth2_service import OAuth2Scope


# Token Introspection (RFC 7662)
class OAuth2TokenIntrospectionRequest(BaseModel):
    """OAuth 2 token introspection request (RFC 7662)"""
    token: str = Field(..., description="Token to introspect")
    token_type_hint: Optional[str] = Field(None, description="Token type hint")
    client_id: str = Field(..., description="Client identifier")
    client_secret: Optional[str] = Field(None, description="Client secret")


class OAuth2TokenIntrospectionResponse(BaseModel):
    """OAuth 2 token introspection response (RFC 7662)"""
    active: bool = Field(..., description="Whether token is active")
    scope: Optional[str] = Field(None, description="Token scopes")
    client_id: Optional[str] = Field(None, description="Client identifier")
    username: Optional[str] = Field(None, description="Username")
    token_type: Optional[str] = Field(None, description="Token type")
    exp: Optional[int] = Field(None, description="Expiration timestamp")
    iat: Optional[int] = Field(None, description="Issued at timestamp")
    sub: Optional[str] = Field(None, description="Subject identifier")
    aud: Optional[str] = Field(None, description="Audience")
    iss: Optional[str] = Field(None, description="Issuer")


# Token Revocation (RFC 7009)
class OAuth2TokenRevocationRequest(BaseModel):
    """OAuth 2 token revocation request (RFC 7009)"""
    token: str = Field(..., description="Token to revoke")
    token_type_hint: Optional[str] = Field(None, description="Token type hint")
    client_id: str = Field(..., description="Client identifier")
    client_secret: Optional[str] = Field(None, description="Client secret")


# UserInfo Response (OIDC)
class OAuth2UserInfoResponse(BaseModel):
    """OAuth 2 UserInfo response (OIDC)"""
    sub: str = Field(..., description="Subject identifier")
    iss: str = Field(..., description="Issuer")
    aud: str = Field(..., description="Audience")
    preferred_username: Optional[str] = Field(None, description="Preferred username")
    email: Optional[str] = Field(None, description="Email address")
    groups: Optional[List[str]] = Field(None, description="User groups")
    kubernetes_namespace: Optional[str] = Field(None, description="K8s namespace")
    kubernetes_service_account: Optional[str] = Field(None, description="K8s service account")


# Error Response (RFC 6749)
class OAuth2ErrorResponse(BaseModel):
    """OAuth 2 error response (RFC 6749)"""
    error: str = Field(..., description="Error code")
    error_description: Optional[str] = Field(None, description="Human-readable error description")
    error_uri: Optional[str] = Field(None, description="URI to error information")
    state: Optional[str] = Field(None, description="State parameter")

    class Config:
        schema_extra = {
            "example": {
                "error": "invalid_token",
                "error_description": "The access token provided is expired, revoked, malformed, or invalid",
                "error_uri": "https://tools.ietf.org/html/rfc6750#section-3.1"
            }
        }


# Kubernetes Integration Models
class K8sServiceAccountTokenInfo(BaseModel):
    """Kubernetes ServiceAccount token information"""
    name: str = Field(..., description="ServiceAccount name")
    namespace: str = Field(..., description="ServiceAccount namespace")
    uid: str = Field(..., description="ServiceAccount UID")
    audiences: List[str] = Field(..., description="Token audiences")
    expiration: datetime = Field(..., description="Token expiration")
    scopes: List[OAuth2Scope] = Field(..., description="Mapped OAuth 2 scopes")


class K8sTokenValidationRequest(BaseModel):
    """Request to validate K8s token"""
    token: str = Field(..., description="Kubernetes token to validate")
    api_server_url: Optional[str] = Field(None, description="K8s API server URL")


class K8sTokenValidationResponse(BaseModel):
    """Response from K8s token validation"""
    valid: bool = Field(..., description="Whether token is valid")
    service_account_info: Optional[K8sServiceAccountTokenInfo] = Field(None, description="SA info if valid")
    error: Optional[str] = Field(None, description="Error message if invalid")


# Scope Migration Models (for RBAC transition)
class RbacToScopeMigrationRequest(BaseModel):
    """Request to migrate RBAC roles to OAuth 2 scopes"""
    roles: List[str] = Field(..., description="Legacy RBAC roles")


class RbacToScopeMigrationResponse(BaseModel):
    """Response with mapped OAuth 2 scopes"""
    roles: List[str] = Field(..., description="Original RBAC roles")
    scopes: List[OAuth2Scope] = Field(..., description="Mapped OAuth 2 scopes")
    migration_notes: List[str] = Field(default=[], description="Migration notes and warnings")


# Bearer Token Validation Models
class BearerTokenValidationRequest(BaseModel):
    """Request to validate Bearer token from Authorization header"""
    authorization_header: str = Field(..., description="Authorization header value")


class BearerTokenValidationResponse(BaseModel):
    """Response from Bearer token validation"""
    valid: bool = Field(..., description="Whether token is valid")
    token_type: Optional[str] = Field(None, description="Token type (Bearer)")
    scopes: Optional[List[OAuth2Scope]] = Field(None, description="Granted OAuth 2 scopes")
    subject: Optional[str] = Field(None, description="Token subject")
    expires_at: Optional[datetime] = Field(None, description="Token expiration")
    k8s_service_account: Optional[str] = Field(None, description="K8s ServiceAccount if applicable")
    k8s_namespace: Optional[str] = Field(None, description="K8s namespace if applicable")
    error: Optional[str] = Field(None, description="Error message if invalid")


# Scope Authorization Models
class ScopeAuthorizationRequest(BaseModel):
    """Request to check scope-based authorization"""
    granted_scopes: List[OAuth2Scope] = Field(..., description="Scopes granted to token")
    required_scope: OAuth2Scope = Field(..., description="Scope required for operation")
    endpoint_path: Optional[str] = Field(None, description="API endpoint path")
    http_method: Optional[str] = Field(None, description="HTTP method")


class ScopeAuthorizationResponse(BaseModel):
    """Response from scope authorization check"""
    authorized: bool = Field(..., description="Whether request is authorized")
    granted_scopes: List[OAuth2Scope] = Field(..., description="Scopes granted to token")
    required_scope: OAuth2Scope = Field(..., description="Scope required for operation")
    reason: Optional[str] = Field(None, description="Authorization decision reason")