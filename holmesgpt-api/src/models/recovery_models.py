"""
Recovery Analysis Models

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
"""

from typing import Dict, Any, List, Optional, Literal
from pydantic import BaseModel, Field


class RecoveryStrategy(BaseModel):
    """Individual recovery strategy option"""
    action_type: str = Field(..., description="Type of recovery action")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Confidence in strategy")
    rationale: str = Field(..., description="Why this strategy is recommended")
    estimated_risk: Literal["low", "medium", "high"] = Field(..., description="Risk level")
    prerequisites: List[str] = Field(default_factory=list, description="Prerequisites for execution")


class RecoveryRequest(BaseModel):
    """
    Request model for recovery analysis endpoint

    Business Requirement: BR-HAPI-001 (Recovery request schema)
    """
    incident_id: str = Field(..., description="Unique incident identifier")
    failed_action: Dict[str, Any] = Field(..., description="Details of the failed action")
    failure_context: Dict[str, Any] = Field(..., description="Context at time of failure")
    investigation_result: Optional[Dict[str, Any]] = Field(None, description="AI investigation results")
    context: Optional[Dict[str, Any]] = Field(None, description="Additional context")
    constraints: Optional[Dict[str, Any]] = Field(None, description="Recovery constraints")

    class Config:
        json_schema_extra = {
            "example": {
                "incident_id": "inc-001",
                "failed_action": {
                    "type": "scale_deployment",
                    "target": "nginx",
                    "desired_replicas": 5
                },
                "failure_context": {
                    "error": "insufficient_resources",
                    "cluster_state": "high_load"
                },
                "investigation_result": {
                    "root_cause": "resource_exhaustion"
                },
                "context": {
                    "namespace": "production",
                    "cluster": "us-west-2"
                },
                "constraints": {
                    "max_attempts": 3,
                    "timeout": "5m"
                }
            }
        }


class RecoveryResponse(BaseModel):
    """
    Response model for recovery analysis endpoint

    Business Requirement: BR-HAPI-002 (Recovery response schema)
    """
    incident_id: str = Field(..., description="Incident identifier from request")
    can_recover: bool = Field(..., description="Whether recovery is possible")
    strategies: List[RecoveryStrategy] = Field(default_factory=list, description="Recommended recovery strategies")
    primary_recommendation: Optional[str] = Field(None, description="Primary recovery action type")
    analysis_confidence: float = Field(..., ge=0.0, le=1.0, description="Overall confidence")
    warnings: List[str] = Field(default_factory=list, description="Warnings about recovery")
    metadata: Dict[str, Any] = Field(default_factory=dict, description="Additional metadata")

    class Config:
        json_schema_extra = {
            "example": {
                "incident_id": "inc-001",
                "can_recover": True,
                "strategies": [
                    {
                        "action_type": "scale_down_gradual",
                        "confidence": 0.9,
                        "rationale": "Gradually reduce load to allow recovery",
                        "estimated_risk": "low",
                        "prerequisites": ["verify_resource_availability"]
                    }
                ],
                "primary_recommendation": "scale_down_gradual",
                "analysis_confidence": 0.85,
                "warnings": ["High cluster load may affect other services"],
                "metadata": {"analysis_time_ms": 1500}
            }
        }
