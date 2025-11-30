# Handoff Request: HolmesGPT-API Prompt Enhancements

---

## Document Version

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| **3.1** | Nov 30, 2025 | AIAnalysis Team | Fixed `customLabels` type: `Dict[str, str]` → `Dict[str, List[str]]` (subdomain-based) |
| 3.0 | Nov 30, 2025 | HolmesGPT-API Team | **MAJOR**: Removed DEV_MODE anti-pattern, added mock LLM server, removed legacy backward compatibility |
| 2.0 | Nov 30, 2025 | AIAnalysis Team | Added DetectedLabels for workflow filtering |
| 1.0 | Nov 29, 2025 | AIAnalysis Team | Initial recovery prompt design |

---

## 📢 Changelog

### ✨ v3.0 (Nov 30, 2025) - Architecture Cleanup

| Change | Description |
|--------|-------------|
| **DEV_MODE Removed** | Eliminated anti-pattern - tests use same code path as production |
| **Mock LLM Server** | New `tests/mock_llm_server.py` - Ollama-compatible mock for integration tests |
| **Legacy Fields Removed** | `failed_action` and `failure_context` removed - use `PreviousExecution` format |
| **router.config Removed** | All configuration via environment variables |
| **Stub Functions Removed** | `_stub_recovery_analysis()` and `_stub_incident_analysis()` deleted |

### ✨ v2.0 (Nov 30, 2025) - DetectedLabels

| Addition | Description |
|----------|-------------|
| **Section 4: DetectedLabels** | Auto-detected cluster characteristics for workflow filtering |
| `DetectedLabels` Pydantic model | Strongly-typed model matching Go struct |
| `_build_cluster_context_section()` | Convert labels to natural language for LLM |
| MCP filter instructions | LLM instructed to include labels in workflow search |
| Updated testing requirements | Tests for DetectedLabels handling |

### 🔄 What's New for HolmesGPT-API

1. **Receive** `DetectedLabels` from AIAnalysis via `enrichment_results`
2. **Express** labels as natural language in prompt (e.g., "This namespace is GitOps-managed...")
3. **Instruct** LLM to include labels in MCP `search_workflow_catalog` request

---

## ✅ **IMPLEMENTATION STATUS: COMPLETED (v3.0)**

**Date**: 2025-11-30
**Implemented by**: HolmesGPT-API Team
**Tests**: 57 passing (47 unit + 10 integration)

### v3.0 Architecture Changes (Nov 30, 2025)

| Change | Description |
|--------|-------------|
| **DEV_MODE Removed** | Eliminated DEV_MODE anti-pattern - tests now use same code path as production |
| **Mock LLM Server** | Created `tests/mock_llm_server.py` - inline Ollama-compatible mock server for integration tests |
| **Backward Compatibility Removed** | Legacy `failed_action` and `failure_context` fields removed per architecture decision |
| **router.config Removed** | Configuration now exclusively via environment variables |
| **Stub Functions Removed** | Removed `_stub_recovery_analysis()` and `_stub_incident_analysis()` |

### Files Modified

| File | Changes |
|------|---------|
| `src/models/recovery_models.py` | Added `OriginalRCA`, `SelectedWorkflowSummary`, `ExecutionFailure`, `PreviousExecution` models; `RecoveryRequest` now requires `PreviousExecution` for recovery attempts |
| `src/models/incident_models.py` | Added `DetectedLabels`, `EnrichmentResults` models; Updated `IncidentRequest` with `enrichment_results` field |
| `src/extensions/recovery.py` | **v3.0**: Removed DEV_MODE check, removed `_stub_recovery_analysis()`, proper HTTP error handling instead of stub fallback; Added recovery prompt generation with DetectedLabels |
| `src/extensions/incident.py` | **v3.0**: Removed DEV_MODE check, removed `_stub_incident_analysis()`; Added `_build_cluster_context_section()`, `_build_mcp_filter_instructions()` |
| `src/main.py` | **v3.0**: Removed all `router.config` assignments - configuration via env vars only |
| `tests/mock_llm_server.py` | **NEW**: Mock Ollama/OpenAI-compatible HTTP server for integration tests |
| `tests/conftest.py` | **v3.0**: Session-scoped mock LLM server fixture; Updated fixtures to use new `PreviousExecution` format |

### Tests

| Test File | Tests | Purpose |
|-----------|-------|---------|
| `tests/unit/test_recovery_models_dd003.py` | 22 | Model validation (OriginalRCA, SelectedWorkflowSummary, ExecutionFailure, PreviousExecution, DetectedLabels, EnrichmentResults) |
| `tests/unit/test_recovery_prompt_dd003.py` | 17 | Prompt generation, failure reason guidance, cluster context, MCP filter instructions |
| `tests/unit/test_incident_detected_labels.py` | 9 | DetectedLabels integration in incident prompts |
| `tests/integration/test_recovery_dd003_integration.py` | 9 | End-to-end recovery and incident endpoint tests with mock LLM |

### Answers to Questions

1. **Model Location**: New models added to `recovery_models.py` for recovery-specific types and `incident_models.py` for shared types (DetectedLabels, EnrichmentResults).

2. **Backward Compatibility**: ❌ **NOT maintained** - Legacy `failed_action` and `failure_context` fields were **removed** per architecture decision. All callers must use the new `PreviousExecution` format.

3. **Test Coverage**: ✅ Complete - 57 tests covering all requirements using mock LLM server (same code path as production).

### Test Architecture (v3.0)

```
Integration Tests
       │
       ▼
┌──────────────────┐
│  FastAPI Client  │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐     ┌─────────────────────┐
│  HolmesGPT API   │────▶│  Mock LLM Server    │
│  (Same as Prod)  │     │  (tests/mock_llm_   │
└──────────────────┘     │   server.py)        │
                         └─────────────────────┘
```

**Key benefit**: Tests exercise the exact same code path as production. No DEV_MODE branches, no stub functions.

---

## Summary

The AIAnalysis service team has finalized several design decisions that require HolmesGPT-API updates:

1. **Recovery Prompt Design** - Structured prompts for recovery attempts (DD-RECOVERY-002, DD-RECOVERY-003)
2. **DetectedLabels Integration** - Auto-detected cluster characteristics for workflow filtering

**Reference Design Decisions**:
- [DD-RECOVERY-002: Direct AIAnalysis Recovery Flow](../../../architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md)
- [DD-RECOVERY-003: Recovery Prompt Design](../../../architecture/decisions/DD-RECOVERY-003-recovery-prompt-design.md)

---

## Required Changes

### 1. Update `RecoveryRequest` Model

**File**: `src/models/recovery_models.py`

**Current State**: Basic recovery request with unstructured `failed_action` and `failure_context` dicts.

**Required State**: Structured recovery request with explicit previous execution context.

```python
# src/models/recovery_models.py

from typing import Dict, Any, List, Optional
from pydantic import BaseModel, Field


class OriginalRCA(BaseModel):
    """Summary of the original root cause analysis from initial AIAnalysis"""
    summary: str = Field(..., description="Brief RCA summary from initial investigation")
    signal_type: str = Field(..., description="Signal type determined by original RCA (e.g., 'OOMKilled')")
    severity: str = Field(..., description="Severity determined by original RCA")
    contributing_factors: List[str] = Field(default_factory=list, description="Factors that contributed to the issue")


class SelectedWorkflowSummary(BaseModel):
    """Summary of the workflow that was executed and failed"""
    workflow_id: str = Field(..., description="Workflow identifier that was executed")
    version: str = Field(..., description="Workflow version")
    container_image: str = Field(..., description="Container image used for execution")
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
    """Complete context about the previous execution attempt that failed"""
    workflow_execution_ref: str = Field(..., description="Name of failed WorkflowExecution CRD")
    original_rca: OriginalRCA = Field(..., description="RCA from initial AIAnalysis")
    selected_workflow: SelectedWorkflowSummary = Field(..., description="Workflow that was executed")
    failure: ExecutionFailure = Field(..., description="Structured failure details")


class RecoveryRequest(BaseModel):
    """
    Extended recovery request with previous execution context.

    Design Decision: DD-RECOVERY-002, DD-RECOVERY-003

    Key Changes from Current Implementation:
    1. Added structured PreviousExecution instead of loose dicts
    2. Added is_recovery_attempt and recovery_attempt_number
    3. Added enrichment_results for original enriched context
    """
    # Identifiers
    incident_id: str = Field(..., description="Unique incident identifier")
    remediation_id: str = Field(
        ...,
        min_length=1,
        description="Remediation request ID for audit correlation (DD-WORKFLOW-002 v2.2)"
    )

    # Recovery-specific fields (NEW)
    is_recovery_attempt: bool = Field(default=True, description="Always true for recovery requests")
    recovery_attempt_number: int = Field(..., ge=1, description="Which recovery attempt this is (1, 2, 3...)")
    previous_execution: PreviousExecution = Field(..., description="Context from previous failed attempt")

    # Original enriched context (reused from SignalProcessing)
    enrichment_results: Dict[str, Any] = Field(..., description="Original enriched context from SignalProcessing")

    # Standard signal fields (same as IncidentRequest)
    signal_type: str = Field(..., description="Current signal type (may have changed)")
    severity: str = Field(..., description="Current severity")
    resource_namespace: str = Field(..., description="Kubernetes namespace")
    resource_kind: str = Field(..., description="Kubernetes resource kind")
    resource_name: str = Field(..., description="Kubernetes resource name")
    environment: str = Field(default="unknown", description="Environment classification")
    priority: str = Field(default="P2", description="Priority level")

    # Customer-derived labels (via Rego policies) - may be empty
    # These are EXAMPLES - customers define their own keys/values
    # Kubernaut passes these through without validation
    risk_tolerance: Optional[str] = Field(None, description="Risk tolerance (customer-derived via Rego)")
    business_category: Optional[str] = Field(None, description="Business category (customer-derived via Rego)")

    # Optional context
    error_message: Optional[str] = Field(None, description="Current error message")
    cluster_name: Optional[str] = Field(None, description="Cluster name")
    signal_source: Optional[str] = Field(None, description="Signal source")

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
                        "container_image": "kubernaut/workflow-scale:v1.0.0",
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
                "enrichment_results": {"...": "original enriched context"},
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
```

---

### 2. Update Recovery Prompt Generation

**File**: `src/extensions/recovery.py`

**Function**: `_create_investigation_prompt` → rename to `_create_recovery_investigation_prompt`

**Key Changes**:

1. Add "Previous Remediation Attempt" section at the TOP of the prompt
2. Include structured failure context with Kubernetes reason code
3. Add reason-specific recovery guidance
4. Add explicit instructions NOT to repeat the failed workflow

```python
# src/extensions/recovery.py

def _create_recovery_investigation_prompt(request_data: Dict[str, Any]) -> str:
    """
    Create investigation prompt for recovery analysis.

    Design Decision: DD-RECOVERY-003

    Key Differences from Incident Prompt:
    1. Adds "Previous Remediation Attempt" section at TOP
    2. Includes Kubernetes reason code with specific guidance
    3. Instructs LLM NOT to repeat failed workflow
    4. Expects signal type may have CHANGED
    """
    # Extract previous execution context
    previous = request_data.get("previous_execution", {})
    original_rca = previous.get("original_rca", {})
    selected_workflow = previous.get("selected_workflow", {})
    failure = previous.get("failure", {})
    attempt_number = request_data.get("recovery_attempt_number", 1)

    # Get Kubernetes reason code
    failure_reason = failure.get("reason", "Unknown")

    # Build recovery context section (appears BEFORE standard sections)
    prompt = f"""# Recovery Analysis Request (Attempt {attempt_number})

## ⚠️ Previous Remediation Attempt - CRITICAL CONTEXT

**This is a RECOVERY attempt**. A previous remediation was tried and FAILED.
You must understand what was attempted and why it failed before recommending alternatives.

---

### What Was Originally Determined

**Original Root Cause Analysis (from initial investigation)**:
- **Summary**: {original_rca.get('summary', 'Unknown')}
- **Signal Type** (RCA determination): `{original_rca.get('signal_type', 'Unknown')}`
- **Severity**: {original_rca.get('severity', 'unknown')}
- **Contributing Factors**: {', '.join(original_rca.get('contributing_factors', ['None recorded']))}

**Workflow Selected Based on RCA**:
- **Workflow ID**: `{selected_workflow.get('workflow_id', 'Unknown')}`
- **Version**: {selected_workflow.get('version', 'Unknown')}
- **Container Image**: `{selected_workflow.get('container_image', 'Unknown')}`
- **Selection Rationale**: {selected_workflow.get('rationale', 'Not recorded')}
"""

    # Add parameters if present
    params = selected_workflow.get('parameters', {})
    if params:
        prompt += "\n**Parameters Used**:\n"
        for key, value in params.items():
            prompt += f"- `{key}`: `{value}`\n"

    # Add failure details with Kubernetes reason code
    prompt += f"""
---

### What Failed During Execution

**Execution Failure Details**:
- **Failed Step**: Step {failure.get('failed_step_index', '?')} - `{failure.get('failed_step_name', 'Unknown')}`
- **Kubernetes Reason**: **`{failure_reason}`**
- **Error Message**: {failure.get('message', 'No message')}
- **Exit Code**: {failure.get('exit_code', 'N/A')}
- **Execution Duration**: {failure.get('execution_time', 'Unknown')} before failure
- **Failed At**: {failure.get('failed_at', 'Unknown')}

**Failure Reason Interpretation** (`{failure_reason}`):
{_get_failure_reason_guidance(failure_reason)}

---

### Your Recovery Investigation Task

**CRITICAL INSTRUCTIONS**:

1. **DO NOT** select the same workflow (`{selected_workflow.get('workflow_id', 'Unknown')}`) with the same parameters
   - The previous attempt already failed with this approach
   - You must find an ALTERNATIVE solution

2. **INVESTIGATE** the CURRENT cluster state:
   - Start from the failure point, not the original signal
   - Check if the failed step partially executed and changed state
   - Determine if the resource is now in a different condition

3. **DETERMINE** if the signal type has CHANGED:
   - The workflow execution may have altered the cluster state
   - Example: OOMKilled → workflow tried to scale → now "InsufficientCPU"
   - Your workflow search should use the CURRENT signal type, not the original

4. **CONSIDER** alternative approaches based on failure reason:
   - `{failure_reason}` suggests specific recovery strategies (see guidance above)
   - Search for workflows that handle this specific failure mode
   - Consider less aggressive remediation if original was too aggressive

5. **SEARCH** for alternative workflows using:
   - Query: `"<CURRENT_SIGNAL_TYPE> <CURRENT_SEVERITY> recovery"`
   - Include the failure reason in your search rationale

---

"""

    # Now append the standard incident prompt sections
    # (reuse existing code from incident.py for consistency)
    standard_sections = _create_standard_incident_sections(request_data)
    prompt += standard_sections

    return prompt


def _get_failure_reason_guidance(reason: str) -> str:
    """
    Provide reason-specific recovery guidance based on Kubernetes reason codes.

    These are canonical Kubernetes reason codes - the API contract between
    WorkflowExecution status and AIAnalysis recovery.
    """
    guidance_map = {
        # Resource-related failures
        "OOMKilled": """
  - Container exceeded memory limits during remediation
  - Consider: Workflow with lower memory footprint, or scale resources first
  - Alternative: Gradual remediation instead of aggressive action
""",
        "InsufficientCPU": """
  - Not enough CPU available to execute remediation
  - Consider: Wait for resources, or request resource increase first
  - Alternative: Lower-priority workflow that uses less CPU
""",
        "InsufficientMemory": """
  - Not enough memory available in cluster
  - Consider: Evict lower-priority workloads first, or use smaller workflow
  - Alternative: Remediation that doesn't require additional memory
""",

        # Scheduling failures
        "FailedScheduling": """
  - Kubernetes scheduler couldn't place the remediation pod
  - Consider: Node affinity issues, resource constraints, or taints
  - Alternative: Workflow that can run on different nodes
""",
        "Unschedulable": """
  - Pod marked as unschedulable
  - Consider: Check node conditions, tolerations, and affinity rules
  - Alternative: Workflow that removes scheduling constraints
""",

        # Image-related failures
        "ImagePullBackOff": """
  - Could not pull workflow container image
  - Consider: Image doesn't exist, registry auth, network issues
  - Alternative: Workflow with different container image
""",
        "ErrImagePull": """
  - Failed to pull container image
  - Consider: Check image name, tag, and registry access
  - Alternative: Use cached image or different workflow
""",

        # Execution failures
        "DeadlineExceeded": """
  - Workflow execution exceeded time limit
  - Consider: Task taking longer than expected, or stuck
  - Alternative: Workflow with longer timeout, or faster approach
""",
        "BackoffLimitExceeded": """
  - Workflow exceeded retry attempts
  - Consider: Persistent failure, requires different approach
  - Alternative: Completely different remediation strategy
""",
        "Error": """
  - Generic execution error
  - Consider: Check logs for specific error details
  - Alternative: Based on error message analysis
""",

        # Permission failures
        "Unauthorized": """
  - Workflow lacks required permissions
  - Consider: RBAC configuration, service account permissions
  - Alternative: Workflow that doesn't require elevated permissions
""",
        "Forbidden": """
  - Action forbidden by security policy
  - Consider: PodSecurityPolicy, NetworkPolicy, or admission controller
  - Alternative: Workflow that complies with security policies
""",

        # Volume/Storage failures
        "FailedMount": """
  - Could not mount required volume
  - Consider: PVC issues, storage class problems, or capacity
  - Alternative: Workflow that doesn't require persistent storage
""",
        "FailedAttachVolume": """
  - Could not attach volume to node
  - Consider: Volume already attached elsewhere, or node issues
  - Alternative: Workflow that uses different storage approach
""",

        # Network failures
        "NetworkNotReady": """
  - Network not available for pod
  - Consider: CNI issues, network policy blocking
  - Alternative: Workflow that can work with limited network
""",

        # Node failures
        "NodeNotReady": """
  - Node became unavailable during execution
  - Consider: Node health issues, draining, or cordoning
  - Alternative: Workflow that can run on different nodes
""",
        "Evicted": """
  - Pod was evicted during execution
  - Consider: Resource pressure on node
  - Alternative: Workflow with resource requests/limits, or different node
""",
    }

    return guidance_map.get(reason, f"""
  - Kubernetes reason: `{reason}`
  - Investigate the specific failure mode
  - Search for workflows that handle this condition
""")


def _create_standard_incident_sections(request_data: Dict[str, Any]) -> str:
    """
    Create standard incident sections (reused from incident.py).

    This ensures consistency between incident and recovery prompts.
    """
    # Extract standard fields
    signal_type = request_data.get("signal_type", "Unknown")
    severity = request_data.get("severity", "unknown")
    namespace = request_data.get("resource_namespace", "unknown")
    resource_kind = request_data.get("resource_kind", "unknown")
    resource_name = request_data.get("resource_name", "unknown")
    environment = request_data.get("environment", "unknown")
    priority = request_data.get("priority", "P2")
    risk_tolerance = request_data.get("risk_tolerance", "medium")
    business_category = request_data.get("business_category", "standard")
    error_message = request_data.get("error_message", "Unknown error")

    return f"""
## Current Signal Context

**Technical Details**:
- Signal Type: {signal_type}
- Severity: {severity}
- Resource: {namespace}/{resource_kind}/{resource_name}
- Error: {error_message}

## Business Context (FOR MCP WORKFLOW SEARCH)

- Environment: {environment}
- Priority: {priority}
- Business Category: {business_category}
- Risk Tolerance: {risk_tolerance}

## Your Investigation Workflow (Recovery Mode)

### Phase 1: Assess Current State
- Check the CURRENT state of the resource (may have changed due to failed workflow)
- Determine if the failure left the system in a degraded state
- Look for side effects from the partial execution

### Phase 2: Re-evaluate Root Cause
- The original RCA was: `{request_data.get('previous_execution', {}).get('original_rca', {}).get('signal_type', 'Unknown')}`
- Determine if the signal type has CHANGED after the failed workflow
- If changed, use the NEW signal type for workflow search

### Phase 3: Search for Alternative Workflow (MANDATORY)
**YOU MUST** call MCP `search_workflow_catalog` tool with:
- **Query**: `"<CURRENT_SIGNAL_TYPE> <CURRENT_SEVERITY> recovery"`
- **Constraint**: Do NOT select the previously failed workflow

### Phase 4: Return Recovery Recommendation
Provide structured JSON with alternative workflow and updated parameters.

## Expected Response Format (Recovery)

```json
{{
  "recovery_analysis": {{
    "previous_attempt_assessment": {{
      "failure_understood": true,
      "failure_reason_analysis": "Explanation of why previous attempt failed",
      "state_changed": true,
      "current_signal_type": "Current signal type after failure"
    }},
    "current_rca": {{
      "summary": "Updated RCA based on current state",
      "severity": "current severity",
      "signal_type": "current signal type",
      "contributing_factors": ["factor1", "factor2"]
    }}
  }},
  "selected_workflow": {{
    "workflow_id": "alternative-workflow-id",
    "version": "1.0.0",
    "confidence": 0.85,
    "rationale": "Why this alternative was selected and how it differs from failed attempt",
    "parameters": {{
      "PARAM_NAME": "value"
    }}
  }},
  "recovery_strategy": {{
    "approach": "description of recovery approach",
    "differs_from_previous": true,
    "why_different": "Explanation of why this approach is different"
  }}
}}
```
"""
```

---

### 3. Update Response Parsing

**File**: `src/extensions/recovery.py`

**Function**: `_parse_investigation_result`

Add handling for recovery-specific response fields:

```python
def _parse_investigation_result(investigation: InvestigationResult, request_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Parse HolmesGPT InvestigationResult into recovery response format.

    Handles recovery-specific fields: recovery_analysis, recovery_strategy
    """
    analysis_text = investigation.analysis or ""

    # Try to extract structured JSON from response
    json_match = re.search(r'```json\s*(.*?)\s*```', analysis_text, re.DOTALL)
    if json_match:
        try:
            structured = json.loads(json_match.group(1))

            # Extract recovery-specific fields if present
            recovery_analysis = structured.get("recovery_analysis", {})
            recovery_strategy = structured.get("recovery_strategy", {})
            selected_workflow = structured.get("selected_workflow")

            return {
                "incident_id": request_data.get("incident_id"),
                "is_recovery_attempt": True,
                "recovery_attempt_number": request_data.get("recovery_attempt_number", 1),
                "recovery_analysis": recovery_analysis,
                "recovery_strategy": recovery_strategy,
                "selected_workflow": selected_workflow,
                "can_recover": selected_workflow is not None,
                "analysis_confidence": selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0,
                "raw_analysis": analysis_text,
            }
        except json.JSONDecodeError:
            pass

    # Fallback to basic parsing if structured extraction fails
    return {
        "incident_id": request_data.get("incident_id"),
        "is_recovery_attempt": True,
        "recovery_attempt_number": request_data.get("recovery_attempt_number", 1),
        "can_recover": False,
        "analysis_confidence": 0.0,
        "raw_analysis": analysis_text,
        "parse_error": "Failed to extract structured response from LLM output"
    }
```

---

### 4. Handle DetectedLabels for Workflow Filtering

**NEW REQUIREMENT**: SignalProcessing now auto-detects cluster characteristics and passes them to AIAnalysis. HolmesGPT-API must use these for both LLM context AND MCP workflow filtering.

**File**: `src/extensions/incident.py` and `src/extensions/recovery.py`

**Data Flow**:
```
SignalProcessing → AIAnalysis → HolmesGPT-API → LLM prompt (natural language)
                                                      ↓
                                                LLM calls MCP tool
                                                      ↓
                                               Data Storage (filters)
```

**DetectedLabels Structure** (from AIAnalysis enrichment_results):
```python
{
    "detectedLabels": {
        # GitOps Management
        "gitOpsManaged": true,          # bool
        "gitOpsTool": "argocd",         # "argocd" | "flux" | ""

        # Workload Protection
        "pdbProtected": true,           # bool
        "hpaEnabled": false,            # bool

        # Workload Characteristics
        "stateful": false,              # bool
        "helmManaged": true,            # bool

        # Security Posture
        "networkIsolated": true,        # bool
        "podSecurityLevel": "restricted", # "privileged" | "baseline" | "restricted" | ""
        "serviceMesh": "istio"          # "istio" | "linkerd" | ""
    }
}
```

**HolmesGPT-API MUST**:

1. **Express as Natural Language** in the prompt:
```python
def _build_cluster_context_section(detected_labels: Dict[str, Any]) -> str:
    """Convert DetectedLabels to natural language for LLM context."""
    sections = []

    # GitOps
    if detected_labels.get("gitOpsManaged"):
        tool = detected_labels.get("gitOpsTool", "unknown")
        sections.append(f"This namespace is managed by GitOps ({tool}). "
                       "DO NOT make direct changes - recommend GitOps-aware workflows.")

    # Protection
    if detected_labels.get("pdbProtected"):
        sections.append("A PodDisruptionBudget protects this workload. "
                       "Workflows must respect PDB constraints.")

    if detected_labels.get("hpaEnabled"):
        sections.append("HorizontalPodAutoscaler is active. "
                       "Manual scaling may conflict with HPA - prefer HPA-aware workflows.")

    # Workload type
    if detected_labels.get("stateful"):
        sections.append("This is a STATEFUL workload (StatefulSet or has PVCs). "
                       "Use stateful-aware remediation workflows.")

    if detected_labels.get("helmManaged"):
        sections.append("This resource is managed by Helm. "
                       "Consider Helm-compatible workflows.")

    # Security
    if detected_labels.get("networkIsolated"):
        sections.append("NetworkPolicy restricts traffic in this namespace. "
                       "Workflows may need network exceptions.")

    pss = detected_labels.get("podSecurityLevel", "")
    if pss == "restricted":
        sections.append("Pod Security Standard is RESTRICTED. "
                       "Workflows must not require privileged access.")

    mesh = detected_labels.get("serviceMesh", "")
    if mesh:
        sections.append(f"Service mesh ({mesh}) is present. "
                       "Consider service mesh-aware workflows.")

    return "\n".join(sections) if sections else "No special cluster characteristics detected."
```

2. **Instruct LLM to Include in MCP Request**:

Add to the prompt (in both incident and recovery modes):

```python
# Add to prompt generation
prompt += f"""
## Cluster Environment Characteristics (AUTO-DETECTED)

The following characteristics were automatically detected for the target resource.
**YOU MUST include these as filters in your MCP workflow search request.**

{_build_cluster_context_section(detected_labels)}

### MCP Workflow Search Instructions

When calling the `search_workflow_catalog` MCP tool, include detected labels as filters:

```json
{{
  "query": "<signal_type> <severity>",
  "filters": {{
    "gitops_managed": {str(detected_labels.get('gitOpsManaged', False)).lower()},
    "pdb_protected": {str(detected_labels.get('pdbProtected', False)).lower()},
    "stateful": {str(detected_labels.get('stateful', False)).lower()},
    "helm_managed": {str(detected_labels.get('helmManaged', False)).lower()},
    "gitops_tool": "{detected_labels.get('gitOpsTool', '')}",
    "service_mesh": "{detected_labels.get('serviceMesh', '')}",
    "pod_security_level": "{detected_labels.get('podSecurityLevel', '')}"
  }},
  "custom_labels": {json.dumps(custom_labels)}
}}
```

The Data Storage service will use these filters to return only workflows that are compatible
with the detected cluster environment.

**IMPORTANT**: If `gitOpsManaged=true`, prioritize workflows with `gitops_aware=true` tag.
"""
```

3. **Update Request Models** to accept DetectedLabels:

```python
# src/models/incident_models.py

class DetectedLabels(BaseModel):
    """Auto-detected cluster characteristics from SignalProcessing"""
    # GitOps
    gitOpsManaged: bool = Field(default=False)
    gitOpsTool: str = Field(default="")  # "argocd", "flux", ""

    # Protection
    pdbProtected: bool = Field(default=False)
    hpaEnabled: bool = Field(default=False)

    # Workload type
    stateful: bool = Field(default=False)
    helmManaged: bool = Field(default=False)

    # Security
    networkIsolated: bool = Field(default=False)
    podSecurityLevel: str = Field(default="")  # "privileged", "baseline", "restricted"
    serviceMesh: str = Field(default="")  # "istio", "linkerd", ""


class EnrichmentResults(BaseModel):
    """Enrichment results from SignalProcessing"""
    kubernetesContext: Optional[Dict[str, Any]] = None
    detectedLabels: Optional[DetectedLabels] = None
    # CustomLabels: Subdomain-based structure (v3.1)
    # Key = subdomain (e.g., "constraint", "team", "region")
    # Value = list of label values (boolean keys or "key=value" pairs)
    # Example: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
    customLabels: Optional[Dict[str, List[str]]] = None
    # EnrichmentQuality: For RO only - NOT for LLM/HolmesGPT
    enrichmentQuality: float = Field(default=0.0)


class IncidentRequest(BaseModel):
    # ... existing fields ...
    enrichment_results: EnrichmentResults = Field(..., description="Enriched context from SignalProcessing")
```

**Files to Modify**:

| File | Change |
|------|--------|
| `src/models/incident_models.py` | Add `DetectedLabels`, `EnrichmentResults` models |
| `src/extensions/incident.py` | Add `_build_cluster_context_section()`, update prompt |
| `src/extensions/recovery.py` | Same updates for recovery prompts |
| `tests/unit/test_detected_labels.py` | Test natural language generation |

---

## Testing Requirements

### Unit Tests

1. **Test `RecoveryRequest` model validation**:
   - Valid request with all fields
   - Missing required fields
   - Invalid recovery_attempt_number (< 1)
   - Invalid failure reason codes

2. **Test `_create_recovery_investigation_prompt`**:
   - Verify "Previous Remediation Attempt" section appears at top
   - Verify Kubernetes reason code is included
   - Verify reason-specific guidance is generated
   - Verify standard sections are appended

3. **Test `_get_failure_reason_guidance`**:
   - Test all canonical Kubernetes reason codes
   - Test unknown reason code fallback

4. **Test `DetectedLabels` handling** (NEW):
   - Valid DetectedLabels with all fields
   - Empty/null DetectedLabels (graceful handling)
   - Natural language generation for each label type
   - MCP filter JSON generation

5. **Test `_build_cluster_context_section`** (NEW):
   - GitOps-managed namespace → includes GitOps warning
   - PDB-protected workload → includes PDB guidance
   - Stateful workload → includes stateful guidance
   - Multiple labels → combines all guidance
   - No labels → returns "No special characteristics"

### Integration Tests

1. **Test recovery endpoint with structured request**:
   - POST `/api/v1/recovery/analyze` with full `PreviousExecution`
   - Verify prompt contains failure context
   - Verify response includes recovery-specific fields

2. **Test recovery vs incident endpoint differentiation**:
   - Same signal, different endpoint behavior
   - Recovery includes previous attempt context

3. **Test DetectedLabels in MCP workflow search** (NEW):
   - Verify LLM receives DetectedLabels in natural language
   - Verify MCP search request includes filters from DetectedLabels
   - Verify GitOps-managed namespace prioritizes gitops_aware workflows

---

## Files to Modify

| File | Change |
|------|--------|
| `src/models/recovery_models.py` | Add `PreviousExecution`, `OriginalRCA`, `SelectedWorkflowSummary`, `ExecutionFailure` models |
| `src/models/incident_models.py` | Add `DetectedLabels`, `EnrichmentResults` models |
| `src/extensions/incident.py` | Add `_build_cluster_context_section()`, update prompt with DetectedLabels |
| `src/extensions/recovery.py` | Update prompt generation, response parsing, and add DetectedLabels handling |
| `tests/unit/test_recovery_models.py` | Add model validation tests |
| `tests/unit/test_recovery_prompt.py` | Add prompt generation tests |
| `tests/unit/test_detected_labels.py` | Add DetectedLabels natural language and filtering tests |
| `tests/integration/test_recovery_endpoint.py` | Add integration tests |
| `tests/integration/test_workflow_filtering.py` | Add MCP workflow filter integration tests |

---

## Timeline

**Estimated Effort**: 3-4 days

| Task | Estimate |
|------|----------|
| Recovery model updates | 0.5 day |
| DetectedLabels model updates | 0.5 day |
| Prompt generation (recovery) | 1 day |
| Prompt generation (DetectedLabels/filtering) | 0.5 day |
| Response parsing | 0.5 day |
| Tests | 1 day |

---

## Questions for HolmesGPT-API Team (ANSWERED)

1. **Model Location**: ✅ **ANSWERED** - New models added to `recovery_models.py` for recovery-specific types and `incident_models.py` for shared types.

2. **Backward Compatibility**: ✅ **ANSWERED** - No backward compatibility maintained. Legacy fields removed. AIAnalysis team confirmed this is acceptable.

3. **Test Coverage**: ✅ **ANSWERED** - All tests updated to use new `PreviousExecution` format and mock LLM server.

---

## Contact

For questions about this handoff, contact the AIAnalysis service team.

**Reference Design Decisions**:
- DD-RECOVERY-002: Direct AIAnalysis Recovery Flow
- DD-RECOVERY-003: Recovery Prompt Design

---

## Implementation Review Checklist

For AIAnalysis team review:

- [ ] New `PreviousExecution` format matches expected API contract
- [ ] `DetectedLabels` integration generates correct MCP filter instructions
- [ ] Mock LLM server responses are sufficient for integration testing
- [ ] Legacy field removal is acceptable (no backward compatibility)
- [ ] All 57 tests pass with production code path (no DEV_MODE)

