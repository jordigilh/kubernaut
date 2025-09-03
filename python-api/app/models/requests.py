"""
Pydantic models for API requests.
"""

from typing import Dict, List, Optional, Any
from datetime import datetime
from pydantic import BaseModel, Field, field_validator


class HolmesOptions(BaseModel):
    """Options for HolmesGPT operations."""
    max_tokens: Optional[int] = Field(None, ge=1, le=10000, description="Maximum tokens in response")
    temperature: Optional[float] = Field(None, ge=0.0, le=2.0, description="Response creativity (0.0-2.0)")
    timeout: Optional[int] = Field(None, ge=1, le=300, description="Request timeout in seconds")
    context_window: Optional[int] = Field(None, ge=512, le=32000, description="Context window size")
    include_tools: Optional[List[str]] = Field(None, description="Tools to include in analysis")


class KubernetesContext(BaseModel):
    """Kubernetes context information."""
    namespace: Optional[str] = Field(None, description="Target Kubernetes namespace")
    deployment: Optional[str] = Field(None, description="Deployment name")
    service: Optional[str] = Field(None, description="Service name")
    pod: Optional[str] = Field(None, description="Pod name")
    cluster: Optional[str] = Field(None, description="Cluster context")


class ContextData(BaseModel):
    """General context data model."""
    kubernetes_context: Optional[KubernetesContext] = Field(None, description="Kubernetes-specific context")
    time_range: Optional[str] = Field(None, description="Time range for analysis")
    environment: Optional[str] = Field(None, description="Environment (dev, staging, prod)")
    related_services: Optional[List[str]] = Field(None, description="Related services to consider")


class InvestigationContext(BaseModel):
    """Investigation context for structured analysis."""
    kubernetes_context: Optional[KubernetesContext] = Field(None, description="Kubernetes-specific context")
    time_range: Optional[str] = Field(None, description="Time range for investigation")
    environment: Optional[str] = Field(None, description="Environment (dev, staging, prod)")
    related_services: Optional[List[str]] = Field(None, description="Related services to consider")


class AskRequest(BaseModel):
    """Request model for ask endpoint."""
    prompt: str = Field(..., min_length=1, max_length=5000, description="Question or problem description")
    context: Optional[InvestigationContext] = Field(None, description="Additional context for the request")
    options: Optional[HolmesOptions] = Field(None, description="HolmesGPT operation options")

    @field_validator('prompt')
    @classmethod
    def validate_prompt(cls, v):
        if not v or v.isspace():
            raise ValueError('Prompt cannot be empty or whitespace only')
        return v.strip()


class AlertData(BaseModel):
    """Alert data model."""
    name: str = Field(..., description="Alert name")
    severity: str = Field(..., description="Alert severity (critical, warning, info)")
    status: str = Field(..., description="Alert status (firing, resolved)")
    starts_at: datetime = Field(..., description="Alert start time")
    ends_at: Optional[datetime] = Field(None, description="Alert end time (for resolved alerts)")
    labels: Dict[str, str] = Field(default_factory=dict, description="Alert labels")
    annotations: Dict[str, str] = Field(default_factory=dict, description="Alert annotations")

    @field_validator('severity')
    @classmethod
    def validate_severity(cls, v):
        allowed_severities = {'critical', 'warning', 'info'}
        if v.lower() not in allowed_severities:
            raise ValueError(f'Severity must be one of {allowed_severities}')
        return v.lower()

    @field_validator('status')
    @classmethod
    def validate_status(cls, v):
        allowed_statuses = {'firing', 'resolved', 'pending'}
        if v.lower() not in allowed_statuses:
            raise ValueError(f'Status must be one of {allowed_statuses}')
        return v.lower()


class InvestigateRequest(BaseModel):
    """Request model for investigate endpoint."""
    alert: AlertData = Field(..., description="Alert information to investigate")
    context: Optional[InvestigationContext] = Field(None, description="Investigation-specific context")
    investigation_context: Optional[InvestigationContext] = Field(None, description="Investigation-specific context")
    options: Optional[HolmesOptions] = Field(None, description="HolmesGPT operation options")


class HealthCheckRequest(BaseModel):
    """Request model for health check endpoint."""
    include_dependencies: bool = Field(False, description="Include dependency health in check")
    include_metrics: bool = Field(False, description="Include system metrics in health check")
    timeout: Optional[int] = Field(10, ge=1, le=60, description="Health check timeout in seconds")


class BatchRequest(BaseModel):
    """Request model for batch operations."""
    operations: List[Dict[str, Any]] = Field(..., min_length=1, max_length=10, description="List of operations to execute")
    parallel: bool = Field(True, description="Execute operations in parallel")
    fail_fast: bool = Field(False, description="Stop on first failure")
    timeout: Optional[int] = Field(300, ge=1, le=1800, description="Total timeout for all operations")

    @field_validator('operations')
    @classmethod
    def validate_operations(cls, v):
        for op in v:
            if 'type' not in op:
                raise ValueError('Each operation must have a "type" field')
            if op['type'] not in ['ask', 'investigate']:
                raise ValueError('Operation type must be "ask" or "investigate"')
        return v


class ServiceReloadRequest(BaseModel):
    """Request model for service reload endpoint."""
    holmes_config: Optional[Dict[str, Any]] = Field(None, description="HolmesGPT configuration updates")
    api_config: Optional[Dict[str, Any]] = Field(None, description="API configuration updates")
    restart_required: bool = Field(False, description="Whether restart is required after update")


class ConfigUpdateRequest(BaseModel):
    """Request model for configuration updates."""
    config_updates: Dict[str, Any] = Field(..., description="Configuration updates to apply")
    validate_only: bool = Field(False, description="Only validate the configuration without applying")
    restart_required: bool = Field(False, description="Whether service restart is required")


class MetricsRequest(BaseModel):
    """Request model for metrics endpoint."""
    include_system: bool = Field(True, description="Include system metrics")
    include_application: bool = Field(True, description="Include application metrics")
    time_range: Optional[str] = Field(None, description="Time range for metrics")