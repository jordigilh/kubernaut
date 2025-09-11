"""
API Models for HolmesGPT REST API
Pydantic models for request/response validation - BR-HAPI-044
"""

from typing import Dict, List, Any, Optional
from enum import Enum
from datetime import datetime

from pydantic import BaseModel, Field, validator, EmailStr


class Priority(str, Enum):
    """Investigation priority levels - BR-HAPI-003"""
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


class InvestigateRequest(BaseModel):
    """Alert investigation request - BR-HAPI-001, BR-HAPI-002"""
    alert_name: str = Field(..., description="Alert name or identifier")
    namespace: str = Field(..., description="Kubernetes namespace")
    labels: Dict[str, str] = Field(default_factory=dict, description="Alert labels")
    annotations: Dict[str, str] = Field(default_factory=dict, description="Alert annotations")
    priority: Priority = Field(default=Priority.MEDIUM, description="Investigation priority")
    async_processing: bool = Field(default=False, description="Enable asynchronous processing")
    include_context: bool = Field(default=True, description="Include enhanced context")

    class Config:
        schema_extra = {
            "example": {
                "alert_name": "PodCrashLooping",
                "namespace": "production",
                "labels": {"severity": "warning", "team": "platform"},
                "annotations": {"description": "Pod is crash looping"},
                "priority": "high",
                "async_processing": False,
                "include_context": True
            }
        }


class Recommendation(BaseModel):
    """Investigation recommendation - BR-HAPI-004"""
    title: str = Field(..., description="Recommendation title")
    description: str = Field(..., description="Detailed description")
    action_type: str = Field(..., description="Type of action (investigate, fix, scale, etc.)")
    command: Optional[str] = Field(None, description="Command to execute")
    priority: Priority = Field(..., description="Recommendation priority")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Confidence score (0-1)")


class InvestigateResponse(BaseModel):
    """Alert investigation response - BR-HAPI-004, BR-HAPI-005"""
    investigation_id: str = Field(..., description="Unique investigation identifier")
    status: str = Field(..., description="Investigation status")
    alert_name: str = Field(..., description="Alert name")
    namespace: str = Field(..., description="Kubernetes namespace")
    summary: str = Field(..., description="Investigation summary")
    root_cause: Optional[str] = Field(None, description="Identified root cause")
    recommendations: List[Recommendation] = Field(..., description="Recommended actions")
    context_used: Dict[str, Any] = Field(default_factory=dict, description="Context data used")
    timestamp: datetime = Field(..., description="Investigation timestamp")
    duration_seconds: float = Field(..., description="Investigation duration")

    class Config:
        schema_extra = {
            "example": {
                "investigation_id": "inv-123456",
                "status": "completed",
                "alert_name": "PodCrashLooping",
                "namespace": "production",
                "summary": "Pod is failing due to resource constraints",
                "root_cause": "Insufficient memory allocation",
                "recommendations": [
                    {
                        "title": "Increase memory limit",
                        "description": "Pod requires more memory than allocated",
                        "action_type": "scale",
                        "command": "kubectl patch deployment myapp -p '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"main\",\"resources\":{\"limits\":{\"memory\":\"512Mi\"}}}]}}}}'",
                        "priority": "high",
                        "confidence": 0.85
                    }
                ],
                "timestamp": "2025-01-15T10:30:00Z",
                "duration_seconds": 12.5
            }
        }


class ChatRequest(BaseModel):
    """Interactive chat request - BR-HAPI-006, BR-HAPI-007"""
    message: str = Field(..., min_length=1, description="User message")
    session_id: str = Field(..., description="Chat session identifier")
    namespace: Optional[str] = Field(None, description="Kubernetes namespace context")
    include_context: bool = Field(default=True, description="Include current cluster context")
    include_metrics: bool = Field(default=False, description="Include metrics data")
    stream: bool = Field(default=False, description="Enable streaming response")

    class Config:
        schema_extra = {
            "example": {
                "message": "Why is my pod crashing?",
                "session_id": "chat-abc123",
                "namespace": "production",
                "include_context": True,
                "include_metrics": False,
                "stream": False
            }
        }


class ChatResponse(BaseModel):
    """Interactive chat response - BR-HAPI-008"""
    response: str = Field(..., description="AI response")
    session_id: str = Field(..., description="Chat session identifier")
    context_used: Optional[Dict[str, Any]] = Field(None, description="Context data referenced")
    suggestions: List[str] = Field(default_factory=list, description="Follow-up suggestions")
    timestamp: datetime = Field(..., description="Response timestamp")

    class Config:
        schema_extra = {
            "example": {
                "response": "I can see your pod is experiencing memory pressure...",
                "session_id": "chat-abc123",
                "suggestions": ["Check pod resource limits", "Review recent deployments"],
                "timestamp": "2025-01-15T10:30:00Z"
            }
        }


class HealthResponse(BaseModel):
    """Health check response - BR-HAPI-016, BR-HAPI-019"""
    status: str = Field(..., description="Overall health status")
    timestamp: float = Field(..., description="Health check timestamp")
    services: Dict[str, str] = Field(..., description="Individual service health")
    version: str = Field(default="1.0.0", description="API version")

    class Config:
        schema_extra = {
            "example": {
                "status": "healthy",
                "timestamp": 1642176600.0,
                "services": {
                    "holmesgpt_sdk": "healthy",
                    "context_api": "healthy",
                    "llm_provider": "healthy"
                },
                "version": "1.0.0"
            }
        }


class StatusResponse(BaseModel):
    """Service status response - BR-HAPI-020"""
    service: str = Field(..., description="Service name")
    version: str = Field(..., description="Service version")
    status: str = Field(..., description="Service status")
    capabilities: List[str] = Field(..., description="Available capabilities")
    timestamp: float = Field(..., description="Status timestamp")

    class Config:
        schema_extra = {
            "example": {
                "service": "holmesgpt-api",
                "version": "1.0.0",
                "status": "running",
                "capabilities": ["alert_investigation", "chat", "toolsets"],
                "timestamp": 1642176600.0
            }
        }


class ConfigResponse(BaseModel):
    """Configuration response - BR-HAPI-021"""
    llm_provider: str = Field(..., description="Current LLM provider")
    llm_model: str = Field(..., description="Current LLM model")
    available_toolsets: List[str] = Field(..., description="Available toolsets")
    max_concurrent_investigations: int = Field(..., description="Max concurrent investigations")

    class Config:
        schema_extra = {
            "example": {
                "llm_provider": "openai",
                "llm_model": "gpt-4",
                "available_toolsets": ["kubernetes", "prometheus", "logs"],
                "max_concurrent_investigations": 10
            }
        }


class Toolset(BaseModel):
    """Toolset definition - BR-HAPI-033"""
    name: str = Field(..., description="Toolset name")
    description: str = Field(..., description="Toolset description")
    version: str = Field(..., description="Toolset version")
    capabilities: List[str] = Field(..., description="Toolset capabilities")
    enabled: bool = Field(..., description="Whether toolset is enabled")


class ToolsetsResponse(BaseModel):
    """Available toolsets response - BR-HAPI-022"""
    toolsets: List[Toolset] = Field(..., description="Available toolsets")

    class Config:
        schema_extra = {
            "example": {
                "toolsets": [
                    {
                        "name": "kubernetes",
                        "description": "Kubernetes cluster investigation tools",
                        "version": "1.0.0",
                        "capabilities": ["pod_logs", "resource_usage", "events"],
                        "enabled": True
                    }
                ]
            }
        }


class Model(BaseModel):
    """LLM Model definition"""
    name: str = Field(..., description="Model name")
    provider: str = Field(..., description="Model provider")
    description: str = Field(..., description="Model description")
    available: bool = Field(..., description="Whether model is available")


class ModelsResponse(BaseModel):
    """Supported models response - BR-HAPI-023"""
    models: List[Model] = Field(..., description="Supported models")

    class Config:
        schema_extra = {
            "example": {
                "models": [
                    {
                        "name": "gpt-4",
                        "provider": "openai",
                        "description": "GPT-4 model for complex reasoning",
                        "available": True
                    }
                ]
            }
        }


class APIError(BaseModel):
    """Consistent error response model - BR-HAPI-043"""
    error: str = Field(..., description="Error type identifier")
    message: str = Field(..., description="Human-readable error message")
    details: Optional[Dict[str, Any]] = Field(None, description="Additional error context")
    timestamp: str = Field(..., description="Error timestamp in ISO format")


# Authentication Models - BR-HAPI-026 through BR-HAPI-030

class Role(str, Enum):
    """User roles for RBAC - BR-HAPI-027"""
    ADMIN = "admin"
    OPERATOR = "operator"
    VIEWER = "viewer"
    API_USER = "api_user"


class Permission(str, Enum):
    """Granular permissions - BR-HAPI-027"""
    INVESTIGATE_ALERTS = "investigate:alerts"
    CHAT_INTERACTIVE = "chat:interactive"
    VIEW_HEALTH = "view:health"
    VIEW_CONFIG = "view:config"
    MANAGE_USERS = "manage:users"
    ADMIN_SYSTEM = "admin:system"


class LoginRequest(BaseModel):
    """User login request - BR-HAPI-026"""
    username: str = Field(..., min_length=1, description="Username")
    password: str = Field(..., min_length=1, description="Password")

    class Config:
        schema_extra = {
            "example": {
                "username": "operator",
                "password": "operator123"
            }
        }


class UserResponse(BaseModel):
    """User information response - BR-HAPI-028"""
    username: str = Field(..., description="Username")
    email: str = Field(..., description="User email")
    roles: List[Role] = Field(..., description="User roles")
    active: bool = Field(..., description="Whether user is active")
    permissions: Optional[List[Permission]] = Field(None, description="User permissions")

    class Config:
        schema_extra = {
            "example": {
                "username": "operator",
                "email": "operator@kubernaut.local",
                "roles": ["operator"],
                "active": True,
                "permissions": ["investigate:alerts", "chat:interactive"]
            }
        }


class LoginResponse(BaseModel):
    """Login response with JWT token - BR-HAPI-026"""
    access_token: str = Field(..., description="JWT access token")
    token_type: str = Field(default="bearer", description="Token type")
    expires_in: int = Field(..., description="Token expiration time in seconds")
    user: UserResponse = Field(..., description="User information")

    class Config:
        schema_extra = {
            "example": {
                "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
                "token_type": "bearer",
                "expires_in": 3600,
                "user": {
                    "username": "operator",
                    "email": "operator@kubernaut.local",
                    "roles": ["operator"],
                    "active": True
                }
            }
        }


class RefreshRequest(BaseModel):
    """Token refresh request - BR-HAPI-029"""
    refresh_token: str = Field(..., description="Current access token to refresh")


class CreateUserRequest(BaseModel):
    """Create user request - BR-HAPI-028"""
    username: str = Field(..., min_length=1, description="Username")
    email: EmailStr = Field(..., description="User email")
    password: str = Field(..., min_length=6, description="Password")
    roles: List[Role] = Field(..., description="User roles")
    active: bool = Field(default=True, description="Whether user is active")

    class Config:
        schema_extra = {
            "example": {
                "username": "new_operator",
                "email": "new.operator@kubernaut.local",
                "password": "secure123",
                "roles": ["operator"],
                "active": True
            }
        }


class UpdateUserRolesRequest(BaseModel):
    """Update user roles request - BR-HAPI-028"""
    roles: List[Role] = Field(..., description="New user roles")

    class Config:
        schema_extra = {
            "example": {
                "roles": ["operator", "viewer"]
            }
        }
