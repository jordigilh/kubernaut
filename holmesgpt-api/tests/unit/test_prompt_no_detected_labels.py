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
Cycle 2.3: Remove detected_labels from LLM prompt construction

ADR-056 relocates DetectedLabels computation to HAPI runtime (via
get_resource_context / LabelDetector). The initial LLM prompt should
NOT include the "Cluster Environment Characteristics" section derived
from enrichment_results.detectedLabels, because labels are now
computed at runtime by tools and injected into DataStorage queries
via session_state.

Authority: ADR-056, DD-HAPI-018
Business Requirement: BR-HAPI-102

Test IDs:
  UT-HAPI-056-056: Incident prompt omits cluster context section
  UT-HAPI-056-057: Recovery prompt omits cluster context section
  UT-HAPI-056-058: Both prompts still build correctly without cluster context
"""

import pytest
from unittest.mock import patch, MagicMock

from src.extensions.incident.prompt_builder import create_incident_investigation_prompt
from src.extensions.recovery.prompt_builder import _create_recovery_investigation_prompt


ENRICHMENT_WITH_LABELS = {
    "detectedLabels": {
        "gitOpsManaged": True,
        "gitOpsTool": "argocd",
        "pdbProtected": True,
        "hpaEnabled": False,
        "stateful": False,
        "helmManaged": False,
        "networkIsolated": False,
        "serviceMesh": "",
        "failedDetections": [],
    },
    "ownerChain": [
        {"kind": "ReplicaSet", "namespace": "production", "name": "api-abc123"},
        {"kind": "Deployment", "namespace": "production", "name": "api"},
    ],
}


INCIDENT_REQUEST = {
    "severity": "critical",
    "signal_type": "CrashLoopBackOff",
    "resource_namespace": "production",
    "resource_kind": "Pod",
    "resource_name": "api-xyz",
    "environment": "production",
    "priority": "P0",
    "risk_tolerance": "medium",
    "business_category": "standard",
    "error_message": "Container restarting",
    "cluster_name": "prod-us-east-1",
    "signal_source": "alertmanager",
    "enrichment_results": ENRICHMENT_WITH_LABELS,
}


RECOVERY_REQUEST = {
    "severity": "critical",
    "signal_type": "CrashLoopBackOff",
    "resource_namespace": "production",
    "resource_kind": "Pod",
    "resource_name": "api-xyz",
    "environment": "production",
    "priority": "P0",
    "risk_tolerance": "medium",
    "business_category": "standard",
    "error_message": "Container restarting",
    "cluster_name": "prod-us-east-1",
    "signal_source": "alertmanager",
    "enrichment_results": ENRICHMENT_WITH_LABELS,
    "remediation_id": "rem-001",
    "attempt_number": 2,
    "original_rca": {
        "summary": "OOMKilled",
        "signal_type": "OOMKilled",
    },
    "selected_workflow": {
        "workflow_id": "wf-001",
        "workflow_name": "restart-pod",
        "action_type": "RestartPod",
    },
    "failure_reason": "pod did not recover",
}


class TestPromptNoDetectedLabels:
    """
    UT-HAPI-056-056 through UT-HAPI-056-058: Verify prompts no longer inject the
    Cluster Environment Characteristics section from enrichment_results.
    """

    def test_ut_hapi_056_056_incident_prompt_omits_cluster_context(self):
        """UT-HAPI-056-056: Incident prompt does not include Cluster Environment section."""
        prompt = create_incident_investigation_prompt(INCIDENT_REQUEST)

        assert "Cluster Environment Characteristics" not in prompt
        assert "AUTO-DETECTED" not in prompt
        assert "GitOps (argocd)" not in prompt
        assert "PodDisruptionBudget protects" not in prompt

    def test_ut_hapi_056_057_recovery_prompt_omits_cluster_context(self):
        """UT-HAPI-056-057: Recovery prompt does not include Cluster Environment section."""
        prompt = _create_recovery_investigation_prompt(RECOVERY_REQUEST)

        assert "Cluster Environment Characteristics" not in prompt
        assert "AUTO-DETECTED" not in prompt
        assert "GitOps (argocd)" not in prompt
        assert "PodDisruptionBudget protects" not in prompt

    def test_ut_hapi_056_058_prompts_build_correctly_without_cluster_context(self):
        """UT-HAPI-056-058: Both prompts still produce valid output without the section."""
        incident_prompt = create_incident_investigation_prompt(INCIDENT_REQUEST)
        recovery_prompt = _create_recovery_investigation_prompt(RECOVERY_REQUEST)

        assert "Incident Analysis Request" in incident_prompt
        assert "Recovery Analysis Request" in recovery_prompt
        assert len(incident_prompt) > 500
        assert len(recovery_prompt) > 500
