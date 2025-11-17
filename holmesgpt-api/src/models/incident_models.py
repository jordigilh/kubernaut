"""
Incident Analysis Models

Business Requirement: BR-HAPI-002 (Incident analysis request schema)
"""

from typing import Dict, Any, Optional, List
from pydantic import BaseModel, Field


class IncidentRequest(BaseModel):
    """Request model for initial incident analysis endpoint"""
    incident_id: str = Field(..., description="Unique incident identifier")
    signal_type: str = Field(..., description="Canonical signal type")
    severity: str = Field(..., description="Signal severity")
    signal_source: str = Field(..., description="Monitoring system")
    resource_namespace: str = Field(..., description="Kubernetes namespace")
    resource_kind: str = Field(..., description="Kubernetes resource kind")
    resource_name: str = Field(..., description="Resource name")
    error_message: str = Field(..., description="Error message")
    description: Optional[str] = Field(None, description="Additional description")
    environment: str = Field(..., description="Deployment environment")
    priority: str = Field(..., description="Business priority")
    risk_tolerance: str = Field(..., description="Risk tolerance")
    business_category: str = Field(..., description="Business category")
    cluster_name: str = Field(..., description="Kubernetes cluster name")
    is_duplicate: Optional[bool] = Field(False, description="Duplicate signal")
    occurrence_count: Optional[int] = Field(0, description="Occurrence count")
    deduplication_window_minutes: Optional[int] = Field(None, description="Dedup window")
    is_storm: Optional[bool] = Field(False, description="Storm detected")
    storm_signal_count: Optional[int] = Field(0, description="Storm signal count")
    storm_window_minutes: Optional[int] = Field(None, description="Storm window")
    storm_type: Optional[str] = Field(None, description="Storm type")
    affected_resources: Optional[List[str]] = Field(default_factory=list, description="Affected resources")
    firing_time: Optional[str] = Field(None, description="Firing time")
    received_time: Optional[str] = Field(None, description="Received time")
    first_seen: Optional[str] = Field(None, description="First seen")
    last_seen: Optional[str] = Field(None, description="Last seen")
    signal_labels: Optional[Dict[str, str]] = Field(default_factory=dict, description="Signal labels")


class IncidentResponse(BaseModel):
    """Response model for incident analysis endpoint"""
    incident_id: str
    analysis: str
    root_cause_analysis: Dict[str, Any]
    selected_workflow: Optional[Dict[str, Any]]
    confidence: float
    timestamp: str
