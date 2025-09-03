"""
Pydantic models for API responses.
"""

from typing import Dict, List, Optional, Any, Union
from datetime import datetime
from pydantic import BaseModel, Field

class Recommendation(BaseModel):
    """Recommendation model."""
    action: str = Field(..., description="Recommended action type")
    description: str = Field(..., description="Human-readable description")
    command: Optional[str] = Field(None, description="Command to execute (if applicable)")
    risk: str = Field(..., description="Risk level: low, medium, high")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Confidence score (0.0-1.0)")
    parameters: Dict[str, Any] = Field(default_factory=dict, description="Action parameters")
    estimated_time: Optional[str] = Field(None, description="Estimated execution time")
    prerequisites: Optional[List[str]] = Field(None, description="Prerequisites before executing")
    rollback_steps: Optional[List[str]] = Field(None, description="Steps to rollback if needed")

class AnalysisResult(BaseModel):
    """Analysis result model."""
    summary: str = Field(..., description="Analysis summary")
    root_cause: Optional[str] = Field(None, description="Identified root cause")
    impact_assessment: Optional[str] = Field(None, description="Impact assessment")
    urgency_level: str = Field(..., description="Urgency level: low, medium, high, critical")
    affected_components: List[str] = Field(default_factory=list, description="Affected system components")
    related_metrics: Dict[str, Any] = Field(default_factory=dict, description="Related metrics data")
    timeline: Optional[List[Dict[str, Any]]] = Field(None, description="Event timeline")

class AskResponse(BaseModel):
    """Response model for ask endpoint."""
    response: str = Field(..., description="HolmesGPT's response to the question")
    analysis: Optional[AnalysisResult] = Field(None, description="Detailed analysis (if applicable)")
    recommendations: List[Recommendation] = Field(default_factory=list, description="Recommended actions")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Overall confidence score")
    model_used: str = Field(..., description="Model used for the response")
    tokens_used: Optional[int] = Field(None, description="Tokens used in the operation")
    processing_time: float = Field(..., description="Processing time in seconds")
    sources: List[str] = Field(default_factory=list, description="Data sources used")
    limitations: Optional[List[str]] = Field(None, description="Analysis limitations")
    follow_up_questions: Optional[List[str]] = Field(None, description="Suggested follow-up questions")

class InvestigationResult(BaseModel):
    """Investigation result model."""
    alert_analysis: AnalysisResult = Field(..., description="Analysis of the alert")
    evidence: Dict[str, Any] = Field(default_factory=dict, description="Evidence gathered during investigation")
    metrics_data: Dict[str, Any] = Field(default_factory=dict, description="Relevant metrics data")
    logs_summary: Optional[str] = Field(None, description="Summary of relevant logs")
    kubernetes_events: Optional[List[Dict[str, Any]]] = Field(None, description="Relevant Kubernetes events")
    similar_incidents: Optional[List[Dict[str, Any]]] = Field(None, description="Similar past incidents")
    remediation_plan: List[Recommendation] = Field(default_factory=list, description="Step-by-step remediation plan")
    preventive_measures: Optional[List[str]] = Field(None, description="Preventive measures for the future")
    escalation_criteria: Optional[List[str]] = Field(None, description="When to escalate the issue")

class InvestigateResponse(BaseModel):
    """Response model for investigate endpoint."""
    investigation: InvestigationResult = Field(..., description="Investigation results")
    recommendations: List[Recommendation] = Field(default_factory=list, description="Prioritized recommendations")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Overall confidence in the analysis")
    severity_assessment: str = Field(..., description="Assessed severity: low, medium, high, critical")
    estimated_resolution_time: Optional[str] = Field(None, description="Estimated time to resolve")
    requires_human_intervention: bool = Field(..., description="Whether human intervention is required")
    auto_executable_actions: List[Recommendation] = Field(default_factory=list, description="Actions that can be auto-executed")
    model_used: str = Field(..., description="Model used for the investigation")
    tokens_used: Optional[int] = Field(None, description="Tokens used in the operation")
    processing_time: float = Field(..., description="Processing time in seconds")
    data_sources: List[str] = Field(default_factory=list, description="Data sources consulted")

class HealthStatus(BaseModel):
    """Health status model."""
    component: str = Field(..., description="Component name")
    status: str = Field(..., description="Status: healthy, degraded, unhealthy")
    message: Optional[str] = Field(None, description="Status message")
    last_check: datetime = Field(..., description="Last check timestamp")
    response_time: Optional[float] = Field(None, description="Response time in seconds")
    details: Dict[str, Any] = Field(default_factory=dict, description="Additional details")

class HealthCheckResponse(BaseModel):
    """Response model for health check endpoint."""
    healthy: bool = Field(..., description="Overall health status")
    status: str = Field(..., description="Overall status description")
    message: str = Field(..., description="Health status message")
    checks: Dict[str, HealthStatus] = Field(default_factory=dict, description="Individual component checks")
    system_info: Dict[str, Any] = Field(default_factory=dict, description="System information")
    timestamp: float = Field(..., description="Check timestamp")
    version: str = Field(default="1.0.0", description="API version")
    uptime: Optional[float] = Field(None, description="Service uptime in seconds")

class ErrorResponse(BaseModel):
    """Error response model."""
    error: str = Field(..., description="Error code")
    message: str = Field(..., description="Error message")
    details: Dict[str, Any] = Field(default_factory=dict, description="Error details")
    timestamp: float = Field(default_factory=lambda: datetime.now().timestamp(), description="Error timestamp")
    request_id: Optional[str] = Field(None, description="Request ID for tracing")

class ServiceInfoResponse(BaseModel):
    """Service information response model."""
    name: str = Field(..., description="Service name")
    version: str = Field(..., description="Service version")
    description: str = Field(..., description="Service description")
    status: str = Field(..., description="Service status")
    features: Dict[str, Any] = Field(default_factory=dict, description="Available features")
    endpoints: Optional[List[str]] = Field(None, description="Available endpoints")
    configuration: Optional[Dict[str, Any]] = Field(None, description="Current configuration")

class BatchResponse(BaseModel):
    """Response model for batch operations."""
    results: List[Dict[str, Any]] = Field(..., description="Results for each operation")
    successful_count: int = Field(..., description="Number of successful operations")
    failed_count: int = Field(..., description="Number of failed operations")
    total_processing_time: float = Field(..., description="Total processing time")
    parallel_execution: bool = Field(..., description="Whether operations were executed in parallel")

class MetricsResponse(BaseModel):
    """Response model for metrics endpoint."""
    metrics: Dict[str, Any] = Field(..., description="Service metrics")
    holmes_metrics: Dict[str, Any] = Field(default_factory=dict, description="HolmesGPT-specific metrics")
    system_metrics: Dict[str, Any] = Field(default_factory=dict, description="System metrics")
    timestamp: float = Field(..., description="Metrics timestamp")

