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
Mock LLM Responses for Integration Testing

Business Requirement: BR-HAPI-212 - Mock LLM Mode for Integration Testing

Provides deterministic mock responses when MOCK_LLM_MODE=true, allowing
consumers (AIAnalysis, etc.) to run integration tests without:
- LLM API costs
- Non-deterministic responses
- API key requirements in CI/CD

Usage:
    export MOCK_LLM_MODE=true
    # HAPI will return deterministic responses based on signal_type
"""

import os
import logging
from datetime import datetime, timezone
from typing import Dict, Any, Optional, List
from dataclasses import dataclass, field

logger = logging.getLogger(__name__)


def is_mock_mode_enabled() -> bool:
    """
    Check if mock LLM mode is enabled via environment variable.

    BR-HAPI-212: Mock mode for integration testing.

    Returns:
        True if MOCK_LLM_MODE=true (case-insensitive)
    """
    return os.getenv("MOCK_LLM_MODE", "").lower() == "true"


@dataclass
class MockScenario:
    """Configuration for a mock response scenario."""
    signal_type: str
    workflow_id: str
    workflow_title: str
    severity: str
    confidence: float
    root_cause_summary: str
    contributing_factors: List[str] = field(default_factory=list)
    parameters: Dict[str, str] = field(default_factory=dict)


# Pre-defined mock scenarios based on signal_type
MOCK_SCENARIOS: Dict[str, MockScenario] = {
    "OOMKilled": MockScenario(
        signal_type="OOMKilled",
        workflow_id="mock-oomkill-increase-memory-v1",
        workflow_title="OOMKill Recovery - Increase Memory Limits (MOCK)",
        severity="critical",
        confidence=0.92,
        root_cause_summary="Container exceeded memory limits due to traffic spike (MOCK)",
        contributing_factors=["memory_pressure", "traffic_spike", "insufficient_limits"],
        parameters={"NAMESPACE": "from-request", "MEMORY_LIMIT": "1Gi", "RESTART_POLICY": "Always"}
    ),
    "CrashLoopBackOff": MockScenario(
        signal_type="CrashLoopBackOff",
        workflow_id="mock-crashloop-config-fix-v1",
        workflow_title="CrashLoopBackOff - Configuration Fix (MOCK)",
        severity="high",
        confidence=0.88,
        root_cause_summary="Container failing due to missing configuration (MOCK)",
        contributing_factors=["missing_config", "env_var_error", "secret_not_mounted"],
        parameters={"NAMESPACE": "from-request", "CONFIG_MAP": "app-config", "RESTART_DELAY": "30s"}
    ),
    "NodeNotReady": MockScenario(
        signal_type="NodeNotReady",
        workflow_id="mock-node-drain-reboot-v1",
        workflow_title="NodeNotReady - Drain and Reboot (MOCK)",
        severity="critical",
        confidence=0.90,
        root_cause_summary="Node experiencing disk pressure and kernel issues (MOCK)",
        contributing_factors=["disk_pressure", "kernel_panic", "kubelet_unhealthy"],
        parameters={"NODE_NAME": "from-request", "GRACE_PERIOD": "300", "FORCE_DRAIN": "false"}
    ),
    "ImagePullBackOff": MockScenario(
        signal_type="ImagePullBackOff",
        workflow_id="mock-image-fix-v1",
        workflow_title="ImagePullBackOff - Image Registry Fix (MOCK)",
        severity="high",
        confidence=0.85,
        root_cause_summary="Cannot pull container image due to registry auth failure (MOCK)",
        contributing_factors=["registry_auth_failed", "image_not_found", "rate_limited"],
        parameters={"NAMESPACE": "from-request", "IMAGE_PULL_SECRET": "registry-creds"}
    ),
    "Evicted": MockScenario(
        signal_type="Evicted",
        workflow_id="mock-eviction-recovery-v1",
        workflow_title="Pod Eviction Recovery (MOCK)",
        severity="high",
        confidence=0.87,
        root_cause_summary="Pod evicted due to node resource pressure (MOCK)",
        contributing_factors=["node_memory_pressure", "ephemeral_storage_full", "priority_preemption"],
        parameters={"NAMESPACE": "from-request", "RESOURCE_REQUESTS": "increase"}
    ),
    "FailedScheduling": MockScenario(
        signal_type="FailedScheduling",
        workflow_id="mock-scheduling-fix-v1",
        workflow_title="Failed Scheduling - Resource Adjustment (MOCK)",
        severity="medium",
        confidence=0.82,
        root_cause_summary="Pod cannot be scheduled due to insufficient resources (MOCK)",
        contributing_factors=["insufficient_cpu", "insufficient_memory", "node_selector_mismatch"],
        parameters={"NAMESPACE": "from-request", "REDUCE_REQUESTS": "true"}
    ),
}

# Default scenario for unknown signal types
DEFAULT_SCENARIO = MockScenario(
    signal_type="Unknown",
    workflow_id="mock-generic-restart-v1",
    workflow_title="Generic Pod Restart (MOCK)",
    severity="medium",
    confidence=0.75,
    root_cause_summary="Unable to determine specific root cause - generic remediation recommended (MOCK)",
    contributing_factors=["unknown_issue", "requires_investigation"],
    parameters={"NAMESPACE": "from-request", "ACTION": "restart"}
)


# =============================================================================
# EDGE CASE SCENARIOS FOR NON-HAPPY PATH TESTING
# =============================================================================
# These special signal types trigger non-happy path responses for E2E testing.
# BR-HAPI-212: Mock mode must support testing of failure scenarios.

# Edge case: No workflow found - triggers needs_human_review=true
EDGE_CASE_NO_WORKFLOW = "MOCK_NO_WORKFLOW_FOUND"

# Edge case: Low confidence - triggers needs_human_review=true
EDGE_CASE_LOW_CONFIDENCE = "MOCK_LOW_CONFIDENCE"

# Edge case: Not reproducible - triggers can_recover=false for recovery
EDGE_CASE_NOT_REPRODUCIBLE = "MOCK_NOT_REPRODUCIBLE"

# Edge case: Max retries exhausted - triggers needs_human_review with validation history
EDGE_CASE_MAX_RETRIES = "MOCK_MAX_RETRIES_EXHAUSTED"


def is_edge_case_signal(signal_type: Optional[str]) -> bool:
    """Check if signal_type is a special edge case trigger."""
    if not signal_type:
        return False
    return signal_type.upper() in [
        EDGE_CASE_NO_WORKFLOW,
        EDGE_CASE_LOW_CONFIDENCE,
        EDGE_CASE_NOT_REPRODUCIBLE,
        EDGE_CASE_MAX_RETRIES,
    ]


def get_mock_scenario(signal_type: Optional[str]) -> MockScenario:
    """
    Get the mock scenario for a given signal type.

    Args:
        signal_type: The signal type from the incident request (can be None)

    Returns:
        MockScenario for the signal type, or DEFAULT_SCENARIO if not found or None
    """
    # Handle None or empty signal_type
    if not signal_type:
        logger.info("BR-HAPI-212: No signal_type provided, using default")
        return DEFAULT_SCENARIO

    # Try exact match first
    if signal_type in MOCK_SCENARIOS:
        return MOCK_SCENARIOS[signal_type]

    # Try case-insensitive match
    signal_lower = signal_type.lower()
    for key, scenario in MOCK_SCENARIOS.items():
        if key.lower() == signal_lower:
            return scenario

    logger.info(f"BR-HAPI-212: No mock scenario for signal_type='{signal_type}', using default")
    return DEFAULT_SCENARIO


def generate_mock_incident_response(request_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Generate a deterministic mock response for /incident/analyze endpoint.

    BR-HAPI-212: Mock LLM Mode for Integration Testing

    This function:
    1. Extracts signal_type from request
    2. Selects appropriate mock scenario (or edge case)
    3. Returns schema-compliant response without calling LLM

    Edge Case Signal Types (for testing non-happy paths):
    - MOCK_NO_WORKFLOW_FOUND: Returns needs_human_review=true, no workflow
    - MOCK_LOW_CONFIDENCE: Returns needs_human_review=true, low_confidence reason
    - MOCK_MAX_RETRIES_EXHAUSTED: Returns with validation_attempts_history

    Args:
        request_data: The incident request data (from IncidentRequest model)

    Returns:
        Dict matching IncidentResponse schema
    """
    incident_id = request_data.get("incident_id", "mock-incident-unknown")
    signal_type = request_data.get("signal_type", "Unknown")
    namespace = request_data.get("resource_namespace", "default")
    resource_name = request_data.get("resource_name", "unknown-resource")
    resource_kind = request_data.get("resource_kind", "Pod")

    timestamp = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    # Handle edge case: No workflow found
    if signal_type and signal_type.upper() == EDGE_CASE_NO_WORKFLOW:
        return _generate_no_workflow_response(
            incident_id, signal_type, namespace, resource_kind, resource_name, timestamp
        )

    # Handle edge case: Low confidence
    if signal_type and signal_type.upper() == EDGE_CASE_LOW_CONFIDENCE:
        return _generate_low_confidence_response(
            incident_id, signal_type, namespace, resource_kind, resource_name, timestamp
        )

    # Handle edge case: Max retries exhausted
    if signal_type and signal_type.upper() == EDGE_CASE_MAX_RETRIES:
        return _generate_max_retries_response(
            incident_id, signal_type, namespace, resource_kind, resource_name, timestamp
        )

    # Normal happy path scenario
    scenario = get_mock_scenario(signal_type)

    # Build parameters with values from request where applicable
    parameters = dict(scenario.parameters)
    if "NAMESPACE" in parameters and parameters["NAMESPACE"] == "from-request":
        parameters["NAMESPACE"] = namespace
    if "NODE_NAME" in parameters and parameters["NODE_NAME"] == "from-request":
        parameters["NODE_NAME"] = resource_name

    response = {
        "incident_id": incident_id,
        "analysis": f"""## Mock Analysis (BR-HAPI-212)

This is a **deterministic mock response** for integration testing.

### Signal Information
- Signal Type: {signal_type}
- Resource: {namespace}/{resource_kind}/{resource_name}

### Root Cause Analysis (MOCK)
{scenario.root_cause_summary}

### Selected Workflow
Workflow ID: `{scenario.workflow_id}`
Confidence: {scenario.confidence * 100:.0f}%

```json
{{
  "root_cause_analysis": {{
    "summary": "{scenario.root_cause_summary}",
    "severity": "{scenario.severity}",
    "contributing_factors": {scenario.contributing_factors}
  }},
  "selected_workflow": {{
    "workflow_id": "{scenario.workflow_id}",
    "version": "1.0.0",
    "confidence": {scenario.confidence},
    "rationale": "Mock selection based on {signal_type} signal type"
  }}
}}
```
""",
        "root_cause_analysis": {
            "summary": scenario.root_cause_summary,
            "severity": scenario.severity,
            "contributing_factors": scenario.contributing_factors
        },
        "selected_workflow": {
            "workflow_id": scenario.workflow_id,
            "title": scenario.workflow_title,
            "version": "1.0.0",
            "containerImage": f"kubernaut/mock-workflow-{scenario.workflow_id.replace('mock-', '')}:v1.0.0",
            "confidence": scenario.confidence,
            "rationale": f"Mock selection based on {signal_type} signal type (BR-HAPI-212)",
            "parameters": parameters
        },
        "alternative_workflows": [
            {
                "workflow_id": "mock-alternative-workflow-v1",
                "container_image": None,
                "confidence": scenario.confidence - 0.15,
                "rationale": "Alternative mock workflow for audit context"
            }
        ],
        "confidence": scenario.confidence,
        "timestamp": timestamp,
        "target_in_owner_chain": True,
        "warnings": [
            "MOCK_MODE: This response is deterministic for integration testing (BR-HAPI-212)",
            "MOCK_MODE: No LLM was called - response based on signal_type matching"
        ],
        "needs_human_review": False,
        "human_review_reason": None,
        "validation_attempts_history": []
    }

    logger.info({
        "event": "mock_incident_response_generated",
        "br": "BR-HAPI-212",
        "incident_id": incident_id,
        "signal_type": signal_type,
        "mock_workflow_id": scenario.workflow_id,
        "mock_confidence": scenario.confidence
    })

    return response


def _generate_no_workflow_response(
    incident_id: str, signal_type: str, namespace: str,
    resource_kind: str, resource_name: str, timestamp: str
) -> Dict[str, Any]:
    """Generate mock response for no matching workflow scenario (BR-HAPI-197)."""
    logger.info({
        "event": "mock_edge_case_no_workflow",
        "br": "BR-HAPI-212",
        "incident_id": incident_id,
        "edge_case": "no_matching_workflows"
    })

    return {
        "incident_id": incident_id,
        "analysis": f"""## Mock Analysis - No Workflow Found (BR-HAPI-212)

This is a **deterministic mock response** simulating the edge case where
no matching workflow is found in the catalog.

### Signal Information
- Signal Type: {signal_type}
- Resource: {namespace}/{resource_kind}/{resource_name}

### Root Cause Analysis (MOCK)
The signal indicates an unusual condition that does not match any known
remediation workflow in the catalog.

### Result
**No workflow selected** - Human review required.
""",
        "root_cause_analysis": {
            "summary": "Unable to find matching workflow for this signal type (MOCK edge case)",
            "severity": "medium",
            "contributing_factors": ["unknown_signal_pattern", "no_catalog_match"]
        },
        "selected_workflow": None,  # Key difference: no workflow
        "alternative_workflows": [],
        "confidence": 0.0,
        "timestamp": timestamp,
        "target_in_owner_chain": True,
        "warnings": [
            "MOCK_MODE: Edge case - no matching workflow found",
            "BR-HAPI-197: needs_human_review=true, reason=no_matching_workflows"
        ],
        "needs_human_review": True,
        "human_review_reason": "no_matching_workflows",
        "validation_attempts_history": []
    }


def _generate_low_confidence_response(
    incident_id: str, signal_type: str, namespace: str,
    resource_kind: str, resource_name: str, timestamp: str
) -> Dict[str, Any]:
    """Generate mock response for low confidence scenario (BR-HAPI-197)."""
    logger.info({
        "event": "mock_edge_case_low_confidence",
        "br": "BR-HAPI-212",
        "incident_id": incident_id,
        "edge_case": "low_confidence"
    })

    return {
        "incident_id": incident_id,
        "analysis": f"""## Mock Analysis - Low Confidence (BR-HAPI-212)

This is a **deterministic mock response** simulating the edge case where
AI analysis has low confidence in the selected workflow.

### Signal Information
- Signal Type: {signal_type}
- Resource: {namespace}/{resource_kind}/{resource_name}

### Root Cause Analysis (MOCK)
Analysis was inconclusive. Multiple potential causes identified but
confidence is too low for automated remediation.

### Result
Workflow tentatively selected but **human review required** due to low confidence.
""",
        "root_cause_analysis": {
            "summary": "Multiple potential causes - confidence too low for automation (MOCK edge case)",
            "severity": "medium",
            "contributing_factors": ["ambiguous_symptoms", "multiple_potential_causes"]
        },
        "selected_workflow": {
            "workflow_id": "mock-low-confidence-workflow-v1",
            "title": "Low Confidence Remediation (MOCK)",
            "version": "1.0.0",
            "containerImage": "kubernaut/mock-workflow-low-confidence:v1.0.0",
            "confidence": 0.45,  # Below threshold
            "rationale": "Tentative selection with low confidence (BR-HAPI-212 edge case)",
            "parameters": {"NAMESPACE": namespace}
        },
        "alternative_workflows": [
            {
                "workflow_id": "mock-alternative-1-v1",
                "container_image": None,
                "confidence": 0.42,
                "rationale": "Close alternative - human should choose"
            },
            {
                "workflow_id": "mock-alternative-2-v1",
                "container_image": None,
                "confidence": 0.40,
                "rationale": "Another possibility - human should evaluate"
            }
        ],
        "confidence": 0.45,
        "timestamp": timestamp,
        "target_in_owner_chain": True,
        "warnings": [
            "MOCK_MODE: Edge case - low confidence analysis",
            "BR-HAPI-197: needs_human_review=true, reason=low_confidence",
            "Multiple workflows have similar confidence - manual selection recommended"
        ],
        "needs_human_review": True,
        "human_review_reason": "low_confidence",
        "validation_attempts_history": []
    }


def _generate_max_retries_response(
    incident_id: str, signal_type: str, namespace: str,
    resource_kind: str, resource_name: str, timestamp: str
) -> Dict[str, Any]:
    """Generate mock response for max retries exhausted scenario (BR-HAPI-197)."""
    logger.info({
        "event": "mock_edge_case_max_retries",
        "br": "BR-HAPI-212",
        "incident_id": incident_id,
        "edge_case": "max_retries_exhausted"
    })

    # Simulate validation attempts history
    # BR-HAPI-197: ValidationAttempt model fields
    validation_history = [
        {
            "attempt": 1,
            "workflow_id": "mock-retry-workflow-1-v1",
            "is_valid": False,
            "errors": ["Image not found in catalog (MOCK)"],
            "timestamp": timestamp
        },
        {
            "attempt": 2,
            "workflow_id": "mock-retry-workflow-2-v1",
            "is_valid": False,
            "errors": ["Parameter validation failed (MOCK)"],
            "timestamp": timestamp
        },
        {
            "attempt": 3,
            "workflow_id": "mock-retry-workflow-3-v1",
            "is_valid": False,
            "errors": ["Version mismatch (MOCK)"],
            "timestamp": timestamp
        }
    ]

    return {
        "incident_id": incident_id,
        "analysis": f"""## Mock Analysis - Max Retries Exhausted (BR-HAPI-212)

This is a **deterministic mock response** simulating the edge case where
LLM self-correction exhausted all retry attempts.

### Signal Information
- Signal Type: {signal_type}
- Resource: {namespace}/{resource_kind}/{resource_name}

### Validation History
- Attempt 1: Image not found
- Attempt 2: Parameter validation failed
- Attempt 3: Version mismatch

### Result
All {len(validation_history)} validation attempts failed. **Human review required**.
""",
        "root_cause_analysis": {
            "summary": "LLM could not produce valid workflow after max retries (MOCK edge case)",
            "severity": "high",
            "contributing_factors": ["validation_failures", "catalog_mismatch", "llm_parsing_issues"]
        },
        "selected_workflow": None,  # Failed to select valid workflow
        "alternative_workflows": [],
        "confidence": 0.0,
        "timestamp": timestamp,
        "target_in_owner_chain": True,
        "warnings": [
            "MOCK_MODE: Edge case - max retries exhausted",
            "BR-HAPI-197: needs_human_review=true, reason=llm_parsing_error",
            f"All {len(validation_history)} validation attempts failed"
        ],
        "needs_human_review": True,
        "human_review_reason": "llm_parsing_error",
        "validation_attempts_history": validation_history
    }


def generate_mock_recovery_response(request_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Generate a deterministic mock response for /recovery/analyze endpoint.

    BR-HAPI-212: Mock LLM Mode for Integration Testing

    Edge Case Signal Types (for testing non-happy paths):
    - MOCK_NOT_REPRODUCIBLE: Returns can_recover=false (issue resolved itself)
    - MOCK_NO_WORKFLOW_FOUND: Returns needs_human_review=true, no recovery workflow
    - MOCK_LOW_CONFIDENCE: Returns needs_human_review=true for recovery

    Args:
        request_data: The recovery request data

    Returns:
        Dict matching RecoveryResponse schema
    """
    remediation_id = request_data.get("remediation_id", "mock-remediation-unknown")
    signal_type = request_data.get("signal_type", request_data.get("current_signal_type", "Unknown"))
    previous_workflow_id = request_data.get("previous_workflow_id", "unknown-workflow")
    namespace = request_data.get("namespace", request_data.get("resource_namespace", "default"))
    incident_id = request_data.get("incident_id", "mock-incident-unknown")

    timestamp = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    # Handle edge case: Signal not reproducible (issue resolved itself)
    if signal_type and signal_type.upper() == EDGE_CASE_NOT_REPRODUCIBLE:
        return _generate_not_reproducible_recovery_response(
            incident_id, remediation_id, previous_workflow_id, timestamp
        )

    # Handle edge case: No recovery workflow available
    if signal_type and signal_type.upper() == EDGE_CASE_NO_WORKFLOW:
        return _generate_no_recovery_workflow_response(
            incident_id, remediation_id, signal_type, previous_workflow_id, timestamp
        )

    # Handle edge case: Low confidence for recovery
    if signal_type and signal_type.upper() == EDGE_CASE_LOW_CONFIDENCE:
        return _generate_low_confidence_recovery_response(
            incident_id, remediation_id, signal_type, previous_workflow_id, namespace, timestamp
        )

    # Normal happy path scenario
    scenario = get_mock_scenario(signal_type)

    # For recovery, we use a slightly different workflow to simulate "trying something new"
    recovery_workflow_id = f"{scenario.workflow_id}-recovery"

    response = {
        "incident_id": incident_id,  # Required field
        "remediation_id": remediation_id,
        "can_recover": True,  # Required field - mock always returns recoverable
        "analysis_confidence": scenario.confidence - 0.05,  # Required field
        "strategies": [  # Required field - BR-HAPI-002
            {
                "action_type": f"recovery_{scenario.signal_type.lower()}_v1",
                "confidence": scenario.confidence - 0.05,
                "rationale": f"Mock recovery strategy after failed {previous_workflow_id} (BR-HAPI-212)",
                "estimated_risk": "medium",
                "prerequisites": ["verify_resource_state", "check_cluster_capacity"]
            }
        ],
        "primary_recommendation": f"recovery_{scenario.signal_type.lower()}_v1",
        "analysis": f"""## Mock Recovery Analysis (BR-HAPI-212)

This is a **deterministic mock response** for integration testing.

### Recovery Context
- Previous Workflow: {previous_workflow_id}
- Current Signal Type: {signal_type}

### Recovery Analysis (MOCK)
The previous remediation attempt was analyzed. An alternative approach is recommended.

{scenario.root_cause_summary}

### Selected Recovery Workflow
Workflow ID: `{recovery_workflow_id}`
""",
        "recovery_analysis": {
            "previous_attempt_assessment": {
                "workflow_id": previous_workflow_id,
                "failure_understood": True,
                "failure_reason_analysis": "Mock analysis: Previous workflow did not resolve the issue (BR-HAPI-212)",
                "state_changed": False,
                "current_signal_type": signal_type
            },
            "root_cause_refinement": scenario.root_cause_summary
        },
        "selected_workflow": {
            "workflow_id": recovery_workflow_id,
            "title": f"{scenario.workflow_title} - Recovery",
            "version": "1.0.0",
            "confidence": scenario.confidence - 0.05,  # Slightly lower for recovery
            "rationale": f"Mock recovery selection after failed {previous_workflow_id} (BR-HAPI-212)",
            "parameters": {
                "NAMESPACE": namespace,
                "RECOVERY_MODE": "true",
                "PREVIOUS_WORKFLOW": previous_workflow_id
            }
        },
        "alternative_workflows": [],
        "confidence": scenario.confidence - 0.05,
        "timestamp": timestamp,
        "warnings": [
            "MOCK_MODE: This response is deterministic for integration testing (BR-HAPI-212)",
            "MOCK_MODE: No LLM was called - response based on signal_type matching"
        ],
        "needs_human_review": False,
        "human_review_reason": None,
        "metadata": {
            "analysis_time_ms": 150,
            "mock_mode": True
        }
    }

    logger.info({
        "event": "mock_recovery_response_generated",
        "br": "BR-HAPI-212",
        "remediation_id": remediation_id,
        "signal_type": signal_type,
        "previous_workflow_id": previous_workflow_id,
        "mock_workflow_id": recovery_workflow_id,
        "mock_confidence": scenario.confidence - 0.05
    })

    return response


def _generate_not_reproducible_recovery_response(
    incident_id: str, remediation_id: str, previous_workflow_id: str, timestamp: str
) -> Dict[str, Any]:
    """
    Generate mock response for signal not reproducible scenario.

    BR-HAPI-212: When the original signal cannot be reproduced, the issue
    may have resolved itself (e.g., pod restarted and is now healthy).
    In this case, can_recover=false because no recovery action is needed.
    """
    logger.info({
        "event": "mock_edge_case_not_reproducible",
        "br": "BR-HAPI-212",
        "remediation_id": remediation_id,
        "edge_case": "signal_not_reproducible"
    })

    return {
        "incident_id": incident_id,
        "remediation_id": remediation_id,
        "can_recover": False,  # Key difference: no recovery needed
        "analysis_confidence": 0.95,  # High confidence that issue resolved
        "strategies": [],  # Empty - no recovery strategies needed (BR-HAPI-002)
        "primary_recommendation": None,  # No recommendation - issue resolved
        "analysis": """## Mock Recovery Analysis - Signal Not Reproducible (BR-HAPI-212)

This is a **deterministic mock response** simulating the edge case where
the original signal cannot be reproduced.

### Recovery Context
The original issue appears to have resolved itself. The pod/resource
is now in a healthy state.

### Analysis
- Original signal no longer present
- Resource health checks passing
- No recovery action required

### Result
**No recovery needed** - Issue self-resolved.
""",
        "recovery_analysis": {
            "previous_attempt_assessment": {
                "workflow_id": previous_workflow_id,
                "failure_understood": True,
                "failure_reason_analysis": "Previous workflow may have worked - signal no longer reproducible (MOCK)",
                "state_changed": True,  # State changed to healthy
                "current_signal_type": None  # No signal anymore
            },
            "root_cause_refinement": "Issue appears to have self-resolved. Original signal not reproducible."
        },
        "selected_workflow": None,  # No workflow needed
        "alternative_workflows": [],
        "confidence": 0.95,
        "timestamp": timestamp,
        "warnings": [
            "MOCK_MODE: Edge case - signal not reproducible",
            "BR-HAPI-212: can_recover=false, issue self-resolved"
        ],
        "needs_human_review": False,  # No review needed - issue resolved
        "human_review_reason": None,
        "metadata": {
            "analysis_time_ms": 120,
            "mock_mode": True
        }
    }


def _generate_no_recovery_workflow_response(
    incident_id: str, remediation_id: str, signal_type: str,
    previous_workflow_id: str, timestamp: str
) -> Dict[str, Any]:
    """Generate mock response for no recovery workflow available."""
    logger.info({
        "event": "mock_edge_case_no_recovery_workflow",
        "br": "BR-HAPI-212",
        "remediation_id": remediation_id,
        "edge_case": "no_recovery_workflow"
    })

    return {
        "incident_id": incident_id,
        "remediation_id": remediation_id,
        "can_recover": True,  # Recovery might be possible...
        "analysis_confidence": 0.0,  # ...but we have no workflow
        "strategies": [],  # Empty - no recovery strategies available (BR-HAPI-002)
        "primary_recommendation": None,  # No recommendation - no workflows found
        "analysis": f"""## Mock Recovery Analysis - No Workflow Found (BR-HAPI-212)

This is a **deterministic mock response** simulating the edge case where
no suitable recovery workflow can be found.

### Recovery Context
- Previous Workflow: {previous_workflow_id}
- Current Signal Type: {signal_type}

### Analysis
The previous workflow failed but no alternative recovery workflow
matches the current state. Manual intervention required.

### Result
**No recovery workflow** - Human review required.
""",
        "recovery_analysis": {
            "previous_attempt_assessment": {
                "workflow_id": previous_workflow_id,
                "failure_understood": True,
                "failure_reason_analysis": "Previous workflow failed, but no alternative found (MOCK)",
                "state_changed": False,
                "current_signal_type": signal_type
            },
            "root_cause_refinement": "Unable to find recovery workflow for this scenario."
        },
        "selected_workflow": None,
        "alternative_workflows": [],
        "confidence": 0.0,
        "timestamp": timestamp,
        "warnings": [
            "MOCK_MODE: Edge case - no recovery workflow found",
            "BR-HAPI-197: needs_human_review=true, reason=no_matching_workflows"
        ],
        "needs_human_review": True,
        "human_review_reason": "no_matching_workflows",
        "metadata": {
            "analysis_time_ms": 100,
            "mock_mode": True
        }
    }


def _generate_low_confidence_recovery_response(
    incident_id: str, remediation_id: str, signal_type: str,
    previous_workflow_id: str, namespace: str, timestamp: str
) -> Dict[str, Any]:
    """Generate mock response for low confidence recovery."""
    logger.info({
        "event": "mock_edge_case_low_confidence_recovery",
        "br": "BR-HAPI-212",
        "remediation_id": remediation_id,
        "edge_case": "low_confidence_recovery"
    })

    return {
        "incident_id": incident_id,
        "remediation_id": remediation_id,
        "can_recover": True,
        "analysis_confidence": 0.35,
        "strategies": [  # Low confidence strategies - BR-HAPI-002
            {
                "action_type": "cautious_recovery_v1",
                "confidence": 0.35,
                "rationale": "Low confidence recovery - human review recommended (BR-HAPI-212)",
                "estimated_risk": "high",
                "prerequisites": ["verify_state", "backup_config", "human_approval"]
            },
            {
                "action_type": "alternative_recovery_v1",
                "confidence": 0.32,
                "rationale": "Alternative approach with similar confidence",
                "estimated_risk": "high",
                "prerequisites": ["verify_state", "human_approval"]
            }
        ],
        "primary_recommendation": "cautious_recovery_v1",
        "analysis": f"""## Mock Recovery Analysis - Low Confidence (BR-HAPI-212)

This is a **deterministic mock response** simulating the edge case where
recovery is possible but confidence is too low for automation.

### Recovery Context
- Previous Workflow: {previous_workflow_id}
- Current Signal Type: {signal_type}

### Analysis
A potential recovery workflow was identified but confidence is below
the automation threshold. Human review recommended.

### Result
Recovery workflow tentatively selected but **human review required**.
""",
        "recovery_analysis": {
            "previous_attempt_assessment": {
                "workflow_id": previous_workflow_id,
                "failure_understood": False,  # Low confidence in understanding
                "failure_reason_analysis": "Failure reason unclear - low confidence analysis (MOCK)",
                "state_changed": False,
                "current_signal_type": signal_type
            },
            "root_cause_refinement": "Root cause analysis inconclusive - multiple possibilities."
        },
        "selected_workflow": {
            "workflow_id": "mock-low-confidence-recovery-v1",
            "title": "Low Confidence Recovery (MOCK)",
            "version": "1.0.0",
            "confidence": 0.35,
            "rationale": "Tentative recovery selection with low confidence (BR-HAPI-212)",
            "parameters": {
                "NAMESPACE": namespace,
                "RECOVERY_MODE": "cautious",
                "PREVIOUS_WORKFLOW": previous_workflow_id
            }
        },
        "alternative_workflows": [
            {
                "workflow_id": "mock-recovery-alternative-v1",
                "container_image": None,
                "confidence": 0.32,
                "rationale": "Alternative with similar confidence"
            }
        ],
        "confidence": 0.35,
        "timestamp": timestamp,
        "warnings": [
            "MOCK_MODE: Edge case - low confidence recovery",
            "BR-HAPI-197: needs_human_review=true, reason=low_confidence"
        ],
        "needs_human_review": True,
        "human_review_reason": "low_confidence",
        "metadata": {
            "analysis_time_ms": 180,
            "mock_mode": True
        }
    }


# NOTE: generate_mock_postexec_response is NOT implemented for V1.0
# per DD-017 (Effectiveness Monitor V1.1 Deferral).
# The /postexec/analyze endpoint is not exposed until V1.1.
# Uncomment and implement when Effectiveness Monitor is available in V1.1.

