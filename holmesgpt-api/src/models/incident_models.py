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
Incident Analysis Models

Business Requirement: BR-HAPI-002 (Incident analysis request schema)
Business Requirement: BR-AUDIT-001 (Unified audit trail - remediation_id)
Design Decision: DD-WORKFLOW-002 v2.2 (remediation_id mandatory)
Design Decision: DD-RECOVERY-003 (DetectedLabels for workflow filtering)
Design Decision: DD-HAPI-001 (Custom Labels Auto-Append Architecture)
"""

from typing import Dict, Any, Optional, List
from enum import Enum
from pydantic import BaseModel, Field, field_validator


# ========================================
# HUMAN REVIEW REASON ENUM (BR-HAPI-197)
# ========================================

class HumanReviewReason(str, Enum):
    """
    Structured reason for needs_human_review=true.

    Business Requirements: BR-HAPI-197, BR-HAPI-200, BR-HAPI-212
    Design Decision: DD-HAPI-002 v1.2

    AIAnalysis uses this for reliable subReason mapping instead of parsing warnings.
    """
    WORKFLOW_NOT_FOUND = "workflow_not_found"
    IMAGE_MISMATCH = "image_mismatch"
    PARAMETER_VALIDATION_FAILED = "parameter_validation_failed"
    NO_MATCHING_WORKFLOWS = "no_matching_workflows"
    LOW_CONFIDENCE = "low_confidence"
    LLM_PARSING_ERROR = "llm_parsing_error"
    # BR-HAPI-200: LLM investigation did not yield conclusive results
    # Use when LLM couldn't determine root cause or current state
    INVESTIGATION_INCONCLUSIVE = "investigation_inconclusive"
    # BR-HAPI-212: RCA is incomplete - workflow selected but affectedResource missing
    # Use when selected_workflow is not None but affectedResource is missing from root_cause_analysis
    RCA_INCOMPLETE = "rca_incomplete"


# ========================================
# SIGNAL MODE ENUM (ADR-054)
# ========================================

class SignalMode(str, Enum):
    """
    Signal processing mode for investigation strategy selection.

    Architecture Decision: ADR-054 (Predictive Signal Mode Classification)
    Business Requirement: BR-AI-084 (Predictive signal mode prompt strategy)

    - reactive: Incident has occurred, perform RCA (root cause analysis)
    - predictive: Incident is predicted, perform predict & prevent strategy
    """
    REACTIVE = "reactive"
    PREDICTIVE = "predictive"


# ========================================
# TYPE ALIASES (DD-HAPI-001)
# ========================================

# Custom labels type: subdomain → list of label values
# Key = subdomain (e.g., "constraint", "team")
# Value = list of strings (boolean keys or "key=value" pairs)
# Example: {"constraint": ["cost-constrained", "stateful-safe"], "team": ["name=payments"]}
CustomLabels = Dict[str, List[str]]


# ========================================
# DETECTED LABELS MODELS (DD-RECOVERY-003)
# ========================================

# Valid field names for failedDetections validation
# DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED (PSP deprecated, PSS is namespace-level)
DETECTED_LABELS_FIELD_NAMES = {
    "gitOpsManaged", "gitOpsTool", "pdbProtected", "hpaEnabled",
    "stateful", "helmManaged", "networkIsolated", "serviceMesh"
}


class DetectedLabels(BaseModel):
    """
    Auto-detected cluster characteristics from SignalProcessing.

    These labels are used for:
    1. LLM context (natural language) - help LLM understand cluster environment
    2. MCP workflow filtering - filter workflows to only compatible ones

    Design Decision: DD-WORKFLOW-001 v2.2, DD-RECOVERY-003

    Changes:
    - v2.1: Added `failedDetections` field to track which detections failed
    - v2.2: Removed `podSecurityLevel` (PSP deprecated, PSS is namespace-level)

    Consumer logic: if field is in failedDetections, ignore its value
    """
    # Detection failure tracking (DD-WORKFLOW-001 v2.1)
    failedDetections: List[str] = Field(
        default_factory=list,
        description="Field names where detection failed. Consumer should ignore values of these fields. "
                    "Valid values: gitOpsManaged, pdbProtected, hpaEnabled, stateful, helmManaged, "
                    "networkIsolated, serviceMesh"
    )

    # GitOps Management
    gitOpsManaged: bool = Field(default=False, description="Whether namespace is managed by GitOps")
    gitOpsTool: str = Field(default="", description="GitOps tool: 'argocd', 'flux', or ''")

    # Workload Protection
    pdbProtected: bool = Field(default=False, description="Whether PodDisruptionBudget protects this workload")
    hpaEnabled: bool = Field(default=False, description="Whether HorizontalPodAutoscaler is active")

    # Workload Characteristics
    stateful: bool = Field(default=False, description="Whether this is a stateful workload (StatefulSet or has PVCs)")
    helmManaged: bool = Field(default=False, description="Whether resource is managed by Helm")

    # Security Posture
    # DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED (PSP deprecated, PSS is namespace-level)
    networkIsolated: bool = Field(default=False, description="Whether NetworkPolicy restricts traffic")
    serviceMesh: str = Field(default="", description="Service mesh: 'istio', 'linkerd', ''")

    @field_validator('failedDetections')
    @classmethod
    def validate_failed_detections(cls, v: List[str]) -> List[str]:
        """Validate that failedDetections only contains known field names."""
        invalid_fields = set(v) - DETECTED_LABELS_FIELD_NAMES
        if invalid_fields:
            raise ValueError(
                f"Invalid field names in failedDetections: {invalid_fields}. "
                f"Valid values: {DETECTED_LABELS_FIELD_NAMES}"
            )
        return v


class EnrichmentResults(BaseModel):
    """
    Enrichment results from SignalProcessing.

    Contains Kubernetes context, auto-detected labels, and custom labels
    that are used for workflow filtering and LLM context.

    Design Decision: DD-RECOVERY-003, DD-HAPI-001

    Custom Labels (DD-HAPI-001):
    - Format: map[string][]string (subdomain → list of values)
    - Keys are subdomains (e.g., "constraint", "team")
    - Values are lists of strings (boolean keys or "key=value" pairs)
    - Example: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
    - Auto-appended to MCP workflow search (invisible to LLM)
    """
    kubernetesContext: Optional[Dict[str, Any]] = Field(None, description="Kubernetes resource context")
    detectedLabels: Optional[DetectedLabels] = Field(None, description="Auto-detected cluster characteristics")
    customLabels: Optional[CustomLabels] = Field(
        None,
        description="Custom labels from SignalProcessing (subdomain → values). Auto-appended to workflow search per DD-HAPI-001."
    )
    enrichmentQuality: float = Field(default=0.0, ge=0.0, le=1.0, description="Quality score of enrichment (0-1)")


class IncidentRequest(BaseModel):
    """
    Request model for initial incident analysis endpoint

    Business Requirements:
    - BR-HAPI-002: Incident analysis request schema
    - BR-AUDIT-001: Unified audit trail (remediation_id)

    Design Decision: DD-WORKFLOW-002 v2.2
    - remediation_id is MANDATORY for audit trail correlation
    - remediation_id is for CORRELATION ONLY - do NOT use for RCA or workflow matching

    Design Decision: DD-RECOVERY-003
    - enrichment_results contains DetectedLabels for workflow filtering
    """
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

    # Enrichment results with DetectedLabels (DD-RECOVERY-003)
    enrichment_results: Optional[EnrichmentResults] = Field(None, description="Enriched context from SignalProcessing")

    # Signal mode: reactive (incident occurred) or predictive (incident predicted)
    # BR-AI-084: Predictive Signal Mode Prompt Strategy
    # ADR-054: Predictive Signal Mode Classification
    # Used by prompt builder to switch investigation strategy:
    # - "reactive": RCA (root cause analysis) - the incident has occurred
    # - "predictive": Predict & prevent - incident is predicted but not yet occurred
    # Defaults to None (treated as "reactive" by prompt builder for backwards compatibility)
    signal_mode: Optional[SignalMode] = Field(None, description="Signal mode: 'reactive' or 'predictive'. Controls prompt strategy (ADR-054).")

    @field_validator('remediation_id')
    @classmethod
    def validate_remediation_id(cls, v: str) -> str:
        """
        Validate remediation_id is non-empty (E2E-HAPI-008).
        BR-HAPI-200: Input validation
        DD-WORKFLOW-002 v2.2: remediation_id is MANDATORY
        """
        if not v or not v.strip():
            raise ValueError('remediation_id is required and cannot be empty')
        return v


class ValidationAttempt(BaseModel):
    """
    Record of a single validation attempt during LLM self-correction.

    Business Requirement: BR-HAPI-197 (needs_human_review field)
    Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)

    Used for:
    1. Operator notification - natural language description of why validation failed
    2. Audit trail - complete history of all validation attempts
    3. Debugging - understand LLM behavior when workflows fail
    """
    attempt: int = Field(..., ge=1, description="Attempt number (1-indexed)")
    workflow_id: Optional[str] = Field(None, description="Workflow ID being validated (if any)")
    is_valid: bool = Field(..., description="Whether validation passed")
    errors: List[str] = Field(default_factory=list, description="Validation errors (empty if valid)")
    timestamp: str = Field(..., description="ISO timestamp of validation attempt")


class AlternativeWorkflow(BaseModel):
    """
    Alternative workflow recommendation for operator context.

    Design Decision: ADR-045 v1.2 (Alternative Workflows for Audit)

    IMPORTANT: Alternatives are for CONTEXT, not EXECUTION.
    Per APPROVAL_REJECTION_BEHAVIOR_DETAILED.md:
    - ✅ Purpose: Help operator make an informed decision
    - ✅ Content: Pros/cons of alternative approaches
    - ❌ NOT: A fallback queue for automatic execution

    Only `selected_workflow` is executed. Alternatives provide:
    - Audit trail of what options were considered
    - Context for operator approval decisions
    - Transparency into AI reasoning
    """
    workflow_id: str = Field(..., description="Workflow identifier")
    container_image: Optional[str] = Field(None, description="OCI image reference")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Confidence score for this alternative")
    rationale: str = Field(..., description="Why this alternative was considered but not selected")


class IncidentResponse(BaseModel):
    """
    Response model for incident analysis endpoint

    Business Requirement: BR-HAPI-002 (Incident analysis response schema)
    Design Decision: DD-WORKFLOW-001 v1.7 (OwnerChain validation)
    Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)
    Design Decision: ADR-045 v1.2 (Alternative Workflows for Audit)

    Fields added per AIAnalysis team requests:
    - target_in_owner_chain: Whether RCA target was found in OwnerChain (Dec 2, 2025)
    - warnings: Non-fatal warnings for transparency (Dec 2, 2025)
    - alternative_workflows: Other workflows considered (Dec 5, 2025) - INFORMATIONAL ONLY
    - needs_human_review: AI could not produce reliable result (Dec 6, 2025)
    """
    incident_id: str = Field(..., description="Incident identifier from request")
    analysis: str = Field(..., description="Natural language analysis from LLM")
    root_cause_analysis: Dict[str, Any] = Field(..., description="Structured RCA with summary, severity, contributing_factors")
    selected_workflow: Optional[Dict[str, Any]] = Field(None, description="Selected workflow with workflow_id, containerImage, confidence, parameters")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Overall confidence in analysis")
    timestamp: str = Field(..., description="ISO timestamp of analysis completion")

    # Human review flag (DD-HAPI-002 v1.2, Dec 6, 2025)
    # True when AI could not produce a reliable result
    needs_human_review: bool = Field(
        default=False,
        description="True when AI analysis could not produce a reliable result. "
                    "Reasons include: workflow validation failures after retries, LLM parsing errors, "
                    "no suitable workflow found, or other AI reliability issues. "
                    "When true, AIAnalysis should NOT create WorkflowExecution - requires human intervention. "
                    "Check 'human_review_reason' for structured reason or 'warnings' for details."
    )

    # Structured reason for human review (BR-HAPI-197, Dec 6, 2025)
    # Provides reliable enum for AIAnalysis subReason mapping
    human_review_reason: Optional[HumanReviewReason] = Field(
        default=None,
        description="Structured reason when needs_human_review=true. "
                    "Use this for reliable subReason mapping instead of parsing warnings. "
                    "Values: workflow_not_found, image_mismatch, parameter_validation_failed, "
                    "no_matching_workflows, low_confidence, llm_parsing_error"
    )

    # OwnerChain validation fields (DD-WORKFLOW-001 v1.7, AIAnalysis request Dec 2025)
    target_in_owner_chain: bool = Field(
        default=True,
        description="Whether RCA-identified target resource was found in OwnerChain. "
                    "If false, DetectedLabels may be from different scope than affected resource."
    )
    warnings: List[str] = Field(
        default_factory=list,
        description="Non-fatal warnings (e.g., OwnerChain validation issues, low confidence)"
    )

    # Alternative workflows for audit/context (ADR-045 v1.2, Dec 5, 2025)
    # IMPORTANT: These are for INFORMATIONAL purposes only - NOT for automatic execution
    # Only selected_workflow is executed. Alternatives help operators make informed decisions.
    alternative_workflows: List[AlternativeWorkflow] = Field(
        default_factory=list,
        description="Other workflows considered but not selected. For operator context and audit trail only - "
                    "NOT for automatic fallback execution. Helps operators understand AI reasoning."
    )

    # Validation attempts history (BR-HAPI-197, DD-HAPI-002 v1.2, Dec 6, 2025)
    # Complete history of all validation attempts during LLM self-correction loop.
    # Used for:
    # - Operator notifications: Natural language description of why validation failed
    # - Audit trail: Complete record of all attempts (also emitted to audit store)
    # - Debugging: Understanding LLM behavior and failure patterns
    validation_attempts_history: List[ValidationAttempt] = Field(
        default_factory=list,
        description="History of all validation attempts during LLM self-correction. "
                    "Each attempt records workflow_id, validation result, and any errors. "
                    "Empty if validation passed on first attempt or no workflow was selected."
    )
