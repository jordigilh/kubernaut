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
Test Alternative Workflows for Audit/Context

Business Requirement: ADR-045 v1.2 (Alternative Workflows for Audit)
Design Decision: APPROVAL_REJECTION_BEHAVIOR_DETAILED.md

IMPORTANT: Alternatives are for CONTEXT, not EXECUTION.
- ✅ Purpose: Help operator make an informed decision
- ✅ Content: Pros/cons of alternative approaches
- ❌ NOT: A fallback queue for automatic execution

Only selected_workflow is executed. Alternatives provide audit trail
and context for operator approval decisions.
"""

import pytest
from src.models.incident_models import AlternativeWorkflow, IncidentResponse
from pydantic import ValidationError


class TestAlternativeWorkflowModel:
    """Test AlternativeWorkflow Pydantic model validation"""

    def test_valid_alternative_workflow(self):
        """Test creating a valid AlternativeWorkflow"""
        alt = AlternativeWorkflow(
            workflow_id="restart-pod-v1",
            execution_bundle="ghcr.io/kubernaut/workflows/restart-pod:v1.0.0",
            confidence=0.75,
            rationale="Simpler approach but doesn't address root cause of memory leak"
        )
        assert alt.workflow_id == "restart-pod-v1"
        assert alt.confidence == 0.75
        assert "memory leak" in alt.rationale

    def test_alternative_workflow_without_execution_bundle(self):
        """Test AlternativeWorkflow with optional execution_bundle omitted"""
        alt = AlternativeWorkflow(
            workflow_id="scale-horizontal-v1",
            confidence=0.65,
            rationale="Scaling approach considered but rejected due to cost constraints"
        )
        assert alt.workflow_id == "scale-horizontal-v1"
        assert alt.execution_bundle is None

    def test_confidence_must_be_between_0_and_1(self):
        """Test confidence validation bounds"""
        # Valid bounds
        alt_low = AlternativeWorkflow(
            workflow_id="test",
            confidence=0.0,
            rationale="Zero confidence"
        )
        assert alt_low.confidence == 0.0

        alt_high = AlternativeWorkflow(
            workflow_id="test",
            confidence=1.0,
            rationale="Full confidence"
        )
        assert alt_high.confidence == 1.0

        # Invalid: above 1.0
        with pytest.raises(ValidationError):
            AlternativeWorkflow(
                workflow_id="test",
                confidence=1.5,
                rationale="Invalid"
            )

        # Invalid: below 0.0
        with pytest.raises(ValidationError):
            AlternativeWorkflow(
                workflow_id="test",
                confidence=-0.1,
                rationale="Invalid"
            )


class TestIncidentResponseAlternatives:
    """Test alternative_workflows field in IncidentResponse"""

    def test_response_includes_alternatives(self):
        """Test that IncidentResponse can include alternative workflows"""
        response = IncidentResponse(
            incident_id="inc-001",
            analysis="Memory leak detected in nginx container",
            root_cause_analysis={
                "summary": "Container OOMKilled due to memory leak",
                "severity": "high",
                "contributing_factors": ["memory leak", "insufficient limits"]
            },
            selected_workflow={
                "workflow_id": "increase-memory-v1",
                "execution_bundle": "ghcr.io/kubernaut/workflows/memory:v1.0.0",
                "confidence": 0.90,
                "rationale": "Best match for OOMKilled with memory leak"
            },
            confidence=0.90,
            timestamp="2025-12-05T10:00:00Z",
            alternative_workflows=[
                AlternativeWorkflow(
                    workflow_id="restart-pod-v1",
                    execution_bundle="ghcr.io/kubernaut/workflows/restart:v1.0.0",
                    confidence=0.75,
                    rationale="Quick fix but doesn't address memory leak"
                ),
                AlternativeWorkflow(
                    workflow_id="scale-horizontal-v1",
                    confidence=0.60,
                    rationale="Scaling could help but cost-constrained environment"
                )
            ]
        )

        assert len(response.alternative_workflows) == 2
        assert response.alternative_workflows[0].workflow_id == "restart-pod-v1"
        assert response.alternative_workflows[1].confidence == 0.60

    def test_response_defaults_to_empty_alternatives(self):
        """Test that alternative_workflows defaults to empty list"""
        response = IncidentResponse(
            incident_id="inc-002",
            analysis="Simple issue with single solution",
            root_cause_analysis={"summary": "Simple", "severity": "low"},
            selected_workflow=None,
            confidence=0.50,
            timestamp="2025-12-05T10:00:00Z"
        )

        assert response.alternative_workflows == []

    def test_alternatives_serialization(self):
        """Test that alternatives serialize correctly to dict/JSON"""
        response = IncidentResponse(
            incident_id="inc-003",
            analysis="Test",
            root_cause_analysis={"summary": "Test", "severity": "medium"},
            confidence=0.80,
            timestamp="2025-12-05T10:00:00Z",
            alternative_workflows=[
                AlternativeWorkflow(
                    workflow_id="alt-1",
                    confidence=0.70,
                    rationale="Alternative approach"
                )
            ]
        )

        # Serialize to dict
        data = response.model_dump()

        assert "alternative_workflows" in data
        assert len(data["alternative_workflows"]) == 1
        assert data["alternative_workflows"][0]["workflow_id"] == "alt-1"
        assert data["alternative_workflows"][0]["confidence"] == 0.70


class TestAlternativeWorkflowsPurpose:
    """
    Test that alternative_workflows serve INFORMATIONAL purposes only.

    Per APPROVAL_REJECTION_BEHAVIOR_DETAILED.md:
    - ✅ Purpose: Help operator make an informed decision
    - ✅ Content: Pros/cons of alternative approaches
    - ❌ NOT: A fallback queue for automatic execution
    """

    def test_alternatives_are_informational_only(self):
        """
        Verify that alternatives are for context, not execution.

        The response should clearly separate:
        - selected_workflow: The ONE workflow that will be executed
        - alternative_workflows: Options considered but NOT executed
        """
        response = IncidentResponse(
            incident_id="inc-audit-001",
            analysis="Investigation complete",
            root_cause_analysis={
                "summary": "Database connection pool exhausted",
                "severity": "high",
                "contributing_factors": ["connection leak", "high traffic"]
            },
            selected_workflow={
                "workflow_id": "restart-connection-pool-v1",
                "confidence": 0.85,
                "rationale": "Direct fix for connection pool issue"
            },
            confidence=0.85,
            timestamp="2025-12-05T10:00:00Z",
            alternative_workflows=[
                AlternativeWorkflow(
                    workflow_id="scale-database-v1",
                    confidence=0.70,
                    rationale="Would help but expensive; selected workflow is more targeted"
                ),
                AlternativeWorkflow(
                    workflow_id="restart-app-v1",
                    confidence=0.50,
                    rationale="Blunt instrument; loses in-flight requests"
                )
            ]
        )

        # Only selected_workflow is for execution
        assert response.selected_workflow is not None
        assert response.selected_workflow["confidence"] > 0.8

        # Alternatives explain WHY they weren't selected (audit trail)
        for alt in response.alternative_workflows:
            assert alt.rationale  # Must have explanation
            assert alt.confidence < response.selected_workflow["confidence"]  # Lower than selected

    def test_alternatives_provide_audit_trail(self):
        """
        Verify alternatives provide useful audit information.

        Operators and post-incident reviewers should understand:
        - What options were considered
        - Why each was rejected in favor of selected_workflow
        - Confidence levels for decision transparency
        """
        response = IncidentResponse(
            incident_id="inc-audit-002",
            analysis="Investigation complete",
            root_cause_analysis={"summary": "Pod crash loop", "severity": "critical"},
            selected_workflow={
                "workflow_id": "increase-memory-limits-v1",
                "confidence": 0.92,
                "rationale": "OOMKilled pattern detected"
            },
            confidence=0.92,
            timestamp="2025-12-05T10:00:00Z",
            alternative_workflows=[
                AlternativeWorkflow(
                    workflow_id="restart-pod-v1",
                    confidence=0.75,
                    rationale="Quick fix but doesn't prevent recurrence"
                )
            ]
        )

        # Audit trail verification
        data = response.model_dump()

        # Selected workflow clearly identified
        assert data["selected_workflow"]["workflow_id"] == "increase-memory-limits-v1"

        # Alternatives documented with rationale
        alt = data["alternative_workflows"][0]
        assert alt["workflow_id"] == "restart-pod-v1"
        assert "doesn't prevent recurrence" in alt["rationale"]  # Explains why not selected


