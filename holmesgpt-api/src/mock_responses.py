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
    2. Selects appropriate mock scenario
    3. Returns schema-compliant response without calling LLM

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

    scenario = get_mock_scenario(signal_type)

    # Build parameters with values from request where applicable
    parameters = dict(scenario.parameters)
    if "NAMESPACE" in parameters and parameters["NAMESPACE"] == "from-request":
        parameters["NAMESPACE"] = namespace
    if "NODE_NAME" in parameters and parameters["NODE_NAME"] == "from-request":
        parameters["NODE_NAME"] = resource_name

    timestamp = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

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


def generate_mock_recovery_response(request_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Generate a deterministic mock response for /recovery/analyze endpoint.

    BR-HAPI-212: Mock LLM Mode for Integration Testing

    Args:
        request_data: The recovery request data

    Returns:
        Dict matching RecoveryResponse schema
    """
    remediation_id = request_data.get("remediation_id", "mock-remediation-unknown")
    signal_type = request_data.get("signal_type", request_data.get("current_signal_type", "Unknown"))
    previous_workflow_id = request_data.get("previous_workflow_id", "unknown-workflow")
    namespace = request_data.get("namespace", request_data.get("resource_namespace", "default"))

    scenario = get_mock_scenario(signal_type)

    # For recovery, we use a slightly different workflow to simulate "trying something new"
    recovery_workflow_id = f"{scenario.workflow_id}-recovery"

    timestamp = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    # Get incident_id from request (required by response schema)
    incident_id = request_data.get("incident_id", "mock-incident-unknown")

    response = {
        "incident_id": incident_id,  # Required field
        "remediation_id": remediation_id,
        "can_recover": True,  # Required field - mock always returns recoverable
        "analysis_confidence": scenario.confidence - 0.05,  # Required field
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
        "human_review_reason": None
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


# NOTE: generate_mock_postexec_response is NOT implemented for V1.0
# per DD-017 (Effectiveness Monitor V1.1 Deferral).
# The /postexec/analyze endpoint is not exposed until V1.1.
# Uncomment and implement when Effectiveness Monitor is available in V1.1.

