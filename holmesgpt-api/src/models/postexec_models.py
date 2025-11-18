"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Post-Execution Analysis Models

Business Requirements: BR-HAPI-051 to 115 (Post-Execution Analysis)
"""

from typing import Dict, Any, List, Optional, Literal
from pydantic import BaseModel, Field


class EffectivenessAssessment(BaseModel):
    """Assessment of execution effectiveness"""
    success: bool = Field(..., description="Whether execution achieved objectives")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Confidence in assessment")
    reasoning: str = Field(..., description="Detailed reasoning for assessment")
    metrics_analysis: Dict[str, Any] = Field(default_factory=dict, description="Analysis of metrics")


class PostExecRequest(BaseModel):
    """
    Request model for post-execution analysis endpoint

    Business Requirement: BR-HAPI-051 (Post-exec request schema)
    """
    execution_id: str = Field(..., description="Unique execution identifier")
    action_id: str = Field(..., description="Action that was executed")
    action_type: str = Field(..., description="Type of action executed")
    action_details: Dict[str, Any] = Field(..., description="Details of executed action")
    execution_success: bool = Field(..., description="Whether execution completed without errors")
    execution_result: Dict[str, Any] = Field(..., description="Technical execution results")
    pre_execution_state: Optional[Dict[str, Any]] = Field(None, description="State before execution")
    post_execution_state: Optional[Dict[str, Any]] = Field(None, description="State after execution")
    context: Optional[Dict[str, Any]] = Field(None, description="Additional context")

    class Config:
        json_schema_extra = {
            "example": {
                "execution_id": "exec-001",
                "action_id": "action-001",
                "action_type": "scale_deployment",
                "action_details": {
                    "deployment": "nginx",
                    "replicas": 3
                },
                "execution_success": True,
                "execution_result": {
                    "status": "scaled",
                    "duration_ms": 2500
                },
                "pre_execution_state": {
                    "replicas": 1,
                    "cpu_usage": 0.95
                },
                "post_execution_state": {
                    "replicas": 3,
                    "cpu_usage": 0.35
                },
                "context": {
                    "namespace": "production",
                    "cluster": "us-west-2"
                }
            }
        }


class PostExecResponse(BaseModel):
    """
    Response model for post-execution analysis endpoint

    Business Requirement: BR-HAPI-052 (Post-exec response schema)
    """
    execution_id: str = Field(..., description="Execution identifier from request")
    effectiveness: EffectivenessAssessment = Field(..., description="Effectiveness assessment")
    objectives_met: bool = Field(..., description="Whether objectives were met")
    side_effects: List[str] = Field(default_factory=list, description="Detected side effects")
    recommendations: List[str] = Field(default_factory=list, description="Recommendations for improvement")
    pattern_improvements: List[Dict[str, Any]] = Field(default_factory=list, description="Pattern learning data")
    metadata: Dict[str, Any] = Field(default_factory=dict, description="Additional metadata")

    class Config:
        json_schema_extra = {
            "example": {
                "execution_id": "exec-001",
                "effectiveness": {
                    "success": True,
                    "confidence": 0.9,
                    "reasoning": "CPU usage reduced from 95% to 35%",
                    "metrics_analysis": {"cpu_reduction": "63%"}
                },
                "objectives_met": True,
                "side_effects": [],
                "recommendations": ["Monitor for oscillation"],
                "pattern_improvements": [],
                "metadata": {"analysis_time_ms": 1200}
            }
        }
