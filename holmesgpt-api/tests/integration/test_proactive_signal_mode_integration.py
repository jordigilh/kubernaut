"""
Integration Tests: Proactive Signal Mode Prompt Adaptation

Business Requirement: BR-AI-084 (Proactive Signal Mode Prompt Strategy)
Architecture: ADR-054 (Proactive Signal Mode Classification and Prompt Strategy)
Pattern: Direct Python function calls (bypass FastAPI)

Defense-in-Depth Layer: Tier 2 (Integration)
- Tests prompt building components directly
- Validates signal_mode drives prompt content changes
- Ensures proactive vs. reactive investigation strategies differ

Test Scenarios:
- IT-HAPI-084-001: Proactive signal mode adapts prompt for preemptive analysis
- IT-HAPI-084-002: Reactive signal mode produces standard RCA prompt
- IT-HAPI-084-003: Missing signal_mode defaults to reactive
"""

import pytest
from src.extensions.incident.prompt_builder import create_incident_investigation_prompt


class TestProactiveSignalModePromptAdaptation:
    """IT-HAPI-084-001: Proactive signal mode adapts investigation prompt

    Business Context:
    When SP classifies a signal as "proactive" (e.g., Prometheus predict_linear()),
    HAPI must adapt its 5-phase investigation prompt to perform preemptive analysis
    instead of reactive RCA.

    Data Flow:
    SP(signalMode=proactive) → RO → AA(signalMode=proactive) → HAPI(prompt adaptation)

    BR: BR-AI-084 (Proactive Signal Mode Prompt Strategy)
    ADR: ADR-054 (Proactive Signal Mode Classification)
    """

    def test_proactive_mode_includes_prediction_context_in_prompt(self):
        """
        Given: Incident request with signal_mode="proactive"
        When: Building incident investigation prompt
        Then: Prompt includes proactive analysis context (not RCA)

        Business Value: LLM performs preemptive analysis instead of root cause investigation
        """
        # Arrange: Create request data with proactive signal mode
        request_data = {
            "incident_id": "inc-proactive-integration-001",
            "signal_name": "OOMKilled",  # Normalized name from SP (not PredictedOOMKill)
            "signal_mode": "proactive",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_namespace": "production",
            "resource_kind": "Deployment",
            "resource_name": "api-server",
            "error_message": "Predicted memory exhaustion based on trend analysis",
        }

        # Act: Create incident investigation prompt
        prompt = create_incident_investigation_prompt(request_data)

        # Assert: Business outcome validation
        assert isinstance(prompt, str), "Prompt should be string"
        assert len(prompt) > 0, "Prompt should not be empty"

        # Business outcome: Proactive context included
        prompt_lower = prompt.lower()
        assert "predict" in prompt_lower, \
            "Proactive prompt should mention prediction/predicted (not just RCA)"

        # Business outcome: Should NOT use standard RCA language
        # In proactive mode, the incident has NOT yet occurred
        assert "has not" in prompt_lower or "not yet" in prompt_lower or "predicted" in prompt_lower, \
            "Proactive prompt should indicate incident has not yet occurred"

    def test_proactive_mode_includes_prevention_guidance(self):
        """
        Given: Incident request with signal_mode="proactive"
        When: Building incident investigation prompt
        Then: Prompt includes prevention/preemptive action guidance

        Business Value: LLM recommends prevention actions, not just diagnosis
        """
        # Arrange
        request_data = {
            "incident_id": "inc-proactive-integration-002",
            "signal_name": "OOMKilled",
            "signal_mode": "proactive",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_namespace": "production",
            "resource_kind": "Pod",
            "resource_name": "worker-abc123",
            "error_message": "Memory usage trending toward limit",
        }

        # Act
        prompt = create_incident_investigation_prompt(request_data)

        # Assert: Prevention guidance included
        prompt_lower = prompt.lower()
        assert "prevent" in prompt_lower or "preemptive" in prompt_lower or "no action" in prompt_lower, \
            "Proactive prompt should include prevention or preemptive guidance"


class TestReactiveSignalModePromptUnchanged:
    """IT-HAPI-084-002: Reactive signal mode produces standard RCA prompt

    Business Context:
    Reactive signals (standard incidents that have occurred) must produce
    the standard RCA investigation prompt. This validates backwards compatibility.

    BR: BR-AI-084 (Proactive Signal Mode Prompt Strategy)
    """

    def test_reactive_mode_produces_standard_rca_prompt(self):
        """
        Given: Incident request with signal_mode="reactive"
        When: Building incident investigation prompt
        Then: Prompt uses standard RCA investigation language

        Business Value: Existing reactive signals continue working unchanged
        """
        # Arrange: Create request data with explicit reactive signal mode
        request_data = {
            "incident_id": "inc-reactive-integration-001",
            "signal_name": "OOMKilled",
            "signal_mode": "reactive",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_namespace": "production",
            "resource_kind": "Pod",
            "resource_name": "api-server-def456",
            "error_message": "Container killed due to OOM",
        }

        # Act
        prompt = create_incident_investigation_prompt(request_data)

        # Assert: Standard RCA language
        assert isinstance(prompt, str), "Prompt should be string"
        prompt_lower = prompt.lower()

        # Business outcome: RCA analysis language present
        assert "root cause" in prompt_lower or "occurred" in prompt_lower or "investigate" in prompt_lower, \
            "Reactive prompt should include RCA investigation language"

    def test_reactive_mode_does_not_include_proactive_context(self):
        """
        Given: Incident request with signal_mode="reactive"
        When: Building incident investigation prompt
        Then: Prompt does NOT include "Proactive Signal Mode" context section

        Business Value: Reactive prompt is not polluted with proactive language
        """
        # Arrange
        request_data = {
            "incident_id": "inc-reactive-integration-002",
            "signal_name": "OOMKilled",
            "signal_mode": "reactive",
            "severity": "high",
            "signal_source": "kubernetes",
            "resource_namespace": "staging",
            "resource_kind": "Deployment",
            "resource_name": "cache-service",
            "error_message": "Pod terminated with OOMKilled",
        }

        # Act
        prompt = create_incident_investigation_prompt(request_data)

        # Assert: No proactive-specific context block
        assert "Proactive Signal Mode" not in prompt, \
            "Reactive prompt should NOT contain 'Proactive Signal Mode' context section"


class TestSignalModeDefaultBehavior:
    """IT-HAPI-084-003: Missing signal_mode defaults to reactive

    Business Context:
    For backwards compatibility, requests without signal_mode should
    default to reactive behavior (standard RCA).

    BR: BR-AI-084 (Proactive Signal Mode Prompt Strategy)
    """

    def test_missing_signal_mode_defaults_to_reactive(self):
        """
        Given: Incident request WITHOUT signal_mode field
        When: Building incident investigation prompt
        Then: Prompt uses standard reactive/RCA language (same as signal_mode="reactive")

        Business Value: Backwards compatibility with existing HAPI clients
        """
        # Arrange: Request without signal_mode field
        request_data = {
            "incident_id": "inc-default-integration-001",
            "signal_name": "CrashLoopBackOff",
            "severity": "high",
            "signal_source": "kubernetes",
            "resource_namespace": "default",
            "resource_kind": "Pod",
            "resource_name": "test-pod",
            "error_message": "Container crashed",
        }

        # Act
        prompt = create_incident_investigation_prompt(request_data)

        # Assert: Standard RCA behavior (default)
        assert isinstance(prompt, str), "Prompt should be string"
        assert "Proactive Signal Mode" not in prompt, \
            "Default (no signal_mode) should NOT include proactive context"
        assert len(prompt) > 100, \
            "Prompt should be substantive (multi-phase investigation)"

    def test_empty_signal_mode_defaults_to_reactive(self):
        """
        Given: Incident request with signal_mode="" (empty string)
        When: Building incident investigation prompt
        Then: Prompt uses standard reactive/RCA language

        Business Value: Handles edge case of empty signal_mode gracefully
        """
        # Arrange: Request with empty signal_mode
        request_data = {
            "incident_id": "inc-empty-mode-integration-001",
            "signal_name": "NodeNotReady",
            "signal_mode": "",  # Explicitly empty
            "severity": "critical",
            "signal_source": "kubernetes",
            "resource_namespace": "kube-system",
            "resource_kind": "Node",
            "resource_name": "worker-node-1",
            "error_message": "Node condition NotReady",
        }

        # Act
        prompt = create_incident_investigation_prompt(request_data)

        # Assert: Defaults to reactive
        assert isinstance(prompt, str), "Prompt should be string"
        assert "Proactive Signal Mode" not in prompt, \
            "Empty signal_mode should default to reactive (no proactive context)"
