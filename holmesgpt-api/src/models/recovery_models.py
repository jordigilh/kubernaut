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
Recovery Analysis Models

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
Business Requirement: BR-AUDIT-001 (Unified audit trail - remediation_id)
Design Decision: DD-WORKFLOW-002 v2.2 (remediation_id mandatory)
Design Decision: DD-RECOVERY-002, DD-RECOVERY-003 (Recovery prompt design)
"""

from typing import Dict, Any, List, Optional, Literal
from pydantic import BaseModel, Field, field_validator

# Import shared types from incident_models
from src.models.incident_models import EnrichmentResults, AlternativeWorkflow, Severity


class RecoveryStrategy(BaseModel):
    """Individual recovery strategy option"""
    action_type: str = Field(..., description="Type of recovery action")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Confidence in strategy")
    rationale: str = Field(..., description="Why this strategy is recommended")
    estimated_risk: Literal["low", "medium", "high"] = Field(..., description="Risk level")
    prerequisites: List[str] = Field(default_factory=list, description="Prerequisites for execution")
    kubectl_command: Optional[str] = Field(None, description="kubectl command if action_type is kubectl_command")


# ========================================
# RECOVERY CONTEXT MODELS (DD-RECOVERY-002, DD-RECOVERY-003)
# ========================================

class OriginalRCA(BaseModel):
    """Summary of the original root cause analysis from initial AIAnalysis"""
    summary: str = Field(..., description="Brief RCA summary from initial investigation")
    signal_type: str = Field(..., description="Signal type determined by original RCA (e.g., 'OOMKilled')")
    severity: Severity = Field(..., description="Severity determined by original RCA (BR-SEVERITY-001)")
    contributing_factors: List[str] = Field(default_factory=list, description="Factors that contributed to the issue")


class SelectedWorkflowSummary(BaseModel):
    """Summary of the workflow that was executed and failed"""
    workflow_id: str = Field(..., description="Workflow identifier that was executed")
    version: str = Field(..., description="Workflow version")
    execution_bundle: str = Field(..., description="Execution bundle used for workflow execution")
    parameters: Dict[str, str] = Field(default_factory=dict, description="Parameters passed to workflow")
    rationale: str = Field(..., description="Why this workflow was originally selected")


class ExecutionFailure(BaseModel):
    """
    Structured failure information using Kubernetes reason codes.

    CRITICAL: The 'reason' field uses canonical Kubernetes reason codes as the API contract.
    This is NOT natural language - it's a structured enum-like value.

    Valid reason codes include:
    - Resource: OOMKilled, InsufficientCPU, InsufficientMemory, Evicted
    - Scheduling: FailedScheduling, Unschedulable
    - Image: ImagePullBackOff, ErrImagePull, InvalidImageName
    - Execution: DeadlineExceeded, BackoffLimitExceeded, Error
    - Permission: Unauthorized, Forbidden
    - Volume: FailedMount, FailedAttachVolume
    - Node: NodeNotReady, NodeUnreachable
    - Network: NetworkNotReady
    """
    failed_step_index: int = Field(..., ge=0, description="0-indexed step that failed")
    failed_step_name: str = Field(..., description="Name of the failed step")
    reason: str = Field(
        ...,
        description="Kubernetes reason code (e.g., 'OOMKilled', 'DeadlineExceeded'). NOT natural language."
    )
    message: str = Field(..., description="Human-readable error message (for logging/debugging)")
    exit_code: Optional[int] = Field(None, description="Exit code if applicable")
    failed_at: str = Field(..., description="ISO timestamp of failure")
    execution_time: str = Field(..., description="Duration before failure (e.g., '2m34s')")


class PreviousExecution(BaseModel):
    """
    Complete context about the previous execution attempt that failed.

    Business Requirement: BR-HAPI-192 (Recovery Context Consumption)
    - natural_language_summary is WE-generated LLM-friendly failure description
    - Provides context for better recovery workflow selection
    """
    workflow_execution_ref: str = Field(..., description="Name of failed WorkflowExecution CRD")
    original_rca: OriginalRCA = Field(..., description="RCA from initial AIAnalysis")
    selected_workflow: SelectedWorkflowSummary = Field(..., description="Workflow that was executed")
    failure: ExecutionFailure = Field(..., description="Structured failure details")
    # BR-HAPI-192: WE-generated natural language summary for LLM context
    natural_language_summary: Optional[str] = Field(
        None,
        description="WE-generated LLM-friendly failure description. "
                    "Includes workflow name, failure step, exit code, and human-readable context. "
                    "Used by LLM to understand what went wrong and avoid similar approaches."
    )


class RecoveryRequest(BaseModel):
    """
    Request model for recovery analysis endpoint

    Business Requirements:
    - BR-HAPI-001: Recovery request schema
    - BR-AUDIT-001: Unified audit trail (remediation_id)

    Design Decision: DD-WORKFLOW-002 v2.2
    - remediation_id is MANDATORY for audit trail correlation
    - remediation_id is for CORRELATION ONLY - do NOT use for RCA or workflow matching

    Design Decision: DD-RECOVERY-002, DD-RECOVERY-003
    - Structured PreviousExecution for recovery attempts
    - is_recovery_attempt and recovery_attempt_number for tracking
    - enrichment_results for DetectedLabels workflow filtering
    """
    # Identifiers
    incident_id: str = Field(..., description="Unique incident identifier")
    remediation_id: str = Field(
        ...,
        min_length=1,
        description=(
            "Remediation request ID for audit correlation (e.g., 'req-2025-11-27-abc123'). "
            "MANDATORY per DD-WORKFLOW-002 v2.2. This ID is for CORRELATION/AUDIT ONLY - "
            "do NOT use for RCA analysis or workflow matching."
        )
    )

    # Recovery-specific fields (DD-RECOVERY-002, DD-RECOVERY-003)
    is_recovery_attempt: bool = Field(default=False, description="True if this is a recovery attempt after failed workflow")
    recovery_attempt_number: Optional[int] = Field(None, ge=1, description="Which recovery attempt this is (1, 2, 3...)")
    previous_execution: Optional[PreviousExecution] = Field(None, description="Context from previous failed attempt")

    # Enriched context from SignalProcessing (includes DetectedLabels)
    enrichment_results: Optional[EnrichmentResults] = Field(None, description="Enriched context including DetectedLabels for workflow filtering")

    # Standard signal fields
    signal_type: Optional[str] = Field(None, description="Current signal type (may have changed)")
    severity: Optional[Severity] = Field(None, description="Current severity (BR-SEVERITY-001)")
    resource_namespace: Optional[str] = Field(None, description="Kubernetes namespace")
    resource_kind: Optional[str] = Field(None, description="Kubernetes resource kind")
    resource_name: Optional[str] = Field(None, description="Kubernetes resource name")
    environment: str = Field(default="unknown", description="Environment classification")
    priority: str = Field(default="P2", description="Priority level")
    risk_tolerance: str = Field(default="medium", description="Risk tolerance")
    business_category: str = Field(default="standard", description="Business category")

    # Optional context
    error_message: Optional[str] = Field(None, description="Current error message")
    cluster_name: Optional[str] = Field(None, description="Cluster name")
    signal_source: Optional[str] = Field(None, description="Signal source")

    @field_validator('remediation_id')
    @classmethod
    def validate_remediation_id(cls, v: str) -> str:
        """Validate remediation_id is not empty (DD-WORKFLOW-002 v2.2)."""
        if not v or not v.strip():
            raise ValueError("remediation_id is required and cannot be empty")
        return v
    
    @field_validator('incident_id')
    @classmethod
    def validate_incident_id(cls, v: str) -> str:
        """Validate incident_id is not empty."""
        if not v or not v.strip():
            raise ValueError("incident_id is required and cannot be empty")
        return v

    @field_validator('recovery_attempt_number')
    @classmethod
    def validate_recovery_attempt_number(cls, v: Optional[int]) -> Optional[int]:
        """
        Validate recovery_attempt_number >= 1 when provided (E2E-HAPI-018).
        BR-AI-080: Recovery flow validation
        """
        if v is not None and v < 1:
            raise ValueError('recovery_attempt_number must be >= 1')
        return v

    @field_validator('recovery_attempt_number')
    @classmethod
    def validate_recovery_attempt_number(cls, v: Optional[int], info) -> Optional[int]:
        """Validate recovery_attempt_number when is_recovery_attempt is True."""
        # Get is_recovery_attempt from the data being validated
        is_recovery = info.data.get('is_recovery_attempt', False)
        if is_recovery:
            if v is None:
                raise ValueError("recovery_attempt_number is required when is_recovery_attempt is True")
            if v < 1 or v > 3:
                raise ValueError("recovery_attempt_number must be between 1 and 3")
        return v

    class Config:
        json_schema_extra = {
            "example": {
                "incident_id": "inc-001",
                "remediation_id": "req-2025-11-29-abc123",
                "is_recovery_attempt": True,
                "recovery_attempt_number": 2,
                "previous_execution": {
                    "workflow_execution_ref": "req-2025-11-29-abc123-we-1",
                    "original_rca": {
                        "summary": "Memory exhaustion causing OOMKilled in production pod",
                        "signal_type": "OOMKilled",
                        "severity": "high",
                        "contributing_factors": ["memory leak", "insufficient limits"]
                    },
                    "selected_workflow": {
                        "workflow_id": "scale-horizontal-v1",
                        "version": "1.0.0",
                        "execution_bundle": "kubernaut/workflow-scale:v1.0.0",
                        "parameters": {"TARGET_REPLICAS": "5"},
                        "rationale": "Scaling out to distribute memory load"
                    },
                    "failure": {
                        "failed_step_index": 2,
                        "failed_step_name": "scale_deployment",
                        "reason": "OOMKilled",
                        "message": "Container exceeded memory limit during scale operation",
                        "exit_code": 137,
                        "failed_at": "2025-11-29T10:30:00Z",
                        "execution_time": "2m34s"
                    }
                },
                "enrichment_results": {"kubernetesContext": {}, "detectedLabels": {}},
                "signal_type": "OOMKilled",
                "severity": "high",
                "resource_namespace": "production",
                "resource_kind": "Deployment",
                "resource_name": "api-server",
                "environment": "production",
                "priority": "P1",
                "risk_tolerance": "medium",
                "business_category": "critical"
            }
        }


class RecoveryResponse(BaseModel):
    """
    Response model for recovery analysis endpoint

    Business Requirement: BR-HAPI-002 (Recovery response schema)
    Business Requirement: BR-AI-080 (Recovery attempt support)
    Business Requirement: BR-AI-081 (Previous execution context handling)
    """
    incident_id: str = Field(..., description="Incident identifier from request")
    can_recover: bool = Field(..., description="Whether recovery is possible")
    strategies: List[RecoveryStrategy] = Field(default_factory=list, description="Recommended recovery strategies")
    primary_recommendation: Optional[str] = Field(None, description="Primary recovery action type")
    analysis_confidence: float = Field(..., ge=0.0, le=1.0, description="Overall confidence")
    warnings: List[str] = Field(default_factory=list, description="Warnings about recovery")
    metadata: Dict[str, Any] = Field(default_factory=dict, description="Additional metadata")

    # BR-AI-080: Recovery attempt support - selected workflow for recovery
    selected_workflow: Optional[Dict[str, Any]] = Field(
        None,
        description="Selected workflow for recovery attempt (BR-AI-080)"
    )

    # BR-AI-081: Previous execution context - recovery-specific analysis
    recovery_analysis: Optional[Dict[str, Any]] = Field(
        None,
        description="Recovery-specific analysis including previous attempt assessment (BR-AI-081)"
    )

    # BR-HAPI-197: Human review flag for recovery scenarios
    # True when recovery analysis could not produce a reliable result
    # (matching IncidentResponse schema for consistency)
    needs_human_review: bool = Field(
        default=False,
        description="True when AI recovery analysis could not produce a reliable result. "
                    "Reasons include: no recovery workflow found, low confidence, or issue resolved itself. "
                    "When true, AIAnalysis should NOT create WorkflowExecution - requires human intervention. "
                    "Check 'human_review_reason' for structured reason."
    )

    # BR-HAPI-197: Structured reason for human review in recovery
    human_review_reason: Optional[str] = Field(
        default=None,
        description="Structured reason when needs_human_review=true. "
                    "Values: no_matching_workflows, low_confidence, signal_not_reproducible"
    )

    # ADR-045 v1.2: Alternative workflows for audit/context (Dec 5, 2025)
    alternative_workflows: List[AlternativeWorkflow] = Field(
        default_factory=list,
        description="Other workflows considered but not selected. "
                    "For operator context and audit trail only - NOT for automatic execution. "
                    "Helps operators understand AI reasoning and decision alternatives."
    )

    # ADR-056: DetectedLabels computed at runtime by HAPI's LabelDetector.
    detected_labels: Optional[Dict[str, Any]] = Field(
        default=None,
        description="Cluster characteristics detected at runtime by HAPI (ADR-056). "
                    "Includes: gitOpsManaged, pdbProtected, hpaEnabled, stateful, "
                    "helmManaged, networkIsolated, serviceMesh, failedDetections."
    )

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
