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
from pydantic import BaseModel, Field, field_validator


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

class DetectedLabels(BaseModel):
    """
    Auto-detected cluster characteristics from SignalProcessing.

    These labels are used for:
    1. LLM context (natural language) - help LLM understand cluster environment
    2. MCP workflow filtering - filter workflows to only compatible ones

    Design Decision: DD-RECOVERY-003
    """
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
    networkIsolated: bool = Field(default=False, description="Whether NetworkPolicy restricts traffic")
    podSecurityLevel: str = Field(default="", description="Pod Security Standard: 'privileged', 'baseline', 'restricted', ''")
    serviceMesh: str = Field(default="", description="Service mesh: 'istio', 'linkerd', ''")


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


class IncidentResponse(BaseModel):
    """Response model for incident analysis endpoint"""
    incident_id: str
    analysis: str
    root_cause_analysis: Dict[str, Any]
    selected_workflow: Optional[Dict[str, Any]]
    confidence: float
    timestamp: str
