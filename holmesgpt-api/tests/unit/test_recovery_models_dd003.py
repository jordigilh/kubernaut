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
Recovery Models Tests (DD-RECOVERY-002, DD-RECOVERY-003)

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
Design Decisions: DD-RECOVERY-002, DD-RECOVERY-003

Tests validate BUSINESS OUTCOMES:
- Recovery requests can carry previous execution context
- Failure information is structured with Kubernetes reason codes
- Models maintain backward compatibility with existing callers
"""

import pytest
from pydantic import ValidationError


class TestOriginalRCAModel:
    """
    Tests for OriginalRCA model

    Business Outcome: Capture the original root cause analysis
    from the initial AIAnalysis investigation.
    """

    def test_original_rca_captures_rca_summary(self):
        """
        Business Outcome: LLM can see what was originally determined
        """
        from src.models.recovery_models import OriginalRCA

        rca = OriginalRCA(
            summary="Memory exhaustion causing OOMKilled in production pod",
            signal_name="OOMKilled",
            severity="high",
            contributing_factors=["memory leak", "insufficient limits"]
        )

        assert rca.summary == "Memory exhaustion causing OOMKilled in production pod"
        assert rca.signal_name == "OOMKilled"
        assert rca.severity == "high"
        assert len(rca.contributing_factors) == 2

    def test_original_rca_requires_summary(self):
        """
        Business Outcome: Cannot have RCA without summary
        """
        from src.models.recovery_models import OriginalRCA

        with pytest.raises(ValidationError):
            OriginalRCA(
                signal_name="OOMKilled",
                severity="high"
                # Missing summary
            )

    def test_original_rca_allows_empty_contributing_factors(self):
        """
        Business Outcome: RCA can have no contributing factors
        """
        from src.models.recovery_models import OriginalRCA

        rca = OriginalRCA(
            summary="Unknown issue",
            signal_name="Error",
            severity="medium"
        )

        assert rca.contributing_factors == []


class TestSelectedWorkflowSummaryModel:
    """
    Tests for SelectedWorkflowSummary model

    Business Outcome: Capture what workflow was executed and failed,
    so recovery can select alternatives.
    """

    def test_workflow_summary_captures_execution_details(self):
        """
        Business Outcome: Full context of what was attempted
        """
        from src.models.recovery_models import SelectedWorkflowSummary

        workflow = SelectedWorkflowSummary(
            workflow_id="scale-horizontal-v1",
            version="1.0.0",
            execution_bundle="kubernaut/workflow-scale:v1.0.0",
            parameters={"TARGET_REPLICAS": "5", "NAMESPACE": "production"},
            rationale="Scaling out to distribute memory load"
        )

        assert workflow.workflow_id == "scale-horizontal-v1"
        assert workflow.version == "1.0.0"
        assert workflow.execution_bundle == "kubernaut/workflow-scale:v1.0.0"
        assert workflow.parameters["TARGET_REPLICAS"] == "5"
        assert workflow.rationale == "Scaling out to distribute memory load"

    def test_workflow_summary_allows_empty_parameters(self):
        """
        Business Outcome: Workflows can have no parameters
        """
        from src.models.recovery_models import SelectedWorkflowSummary

        workflow = SelectedWorkflowSummary(
            workflow_id="restart-pod-v1",
            version="1.0.0",
            execution_bundle="kubernaut/workflow-restart:v1.0.0",
            rationale="Simple restart"
        )

        assert workflow.parameters == {}


class TestExecutionFailureModel:
    """
    Tests for ExecutionFailure model

    Business Outcome: Structured failure information using Kubernetes
    reason codes as the API contract.
    """

    def test_execution_failure_captures_kubernetes_reason(self):
        """
        Business Outcome: Failure reason is a structured Kubernetes code
        """
        from src.models.recovery_models import ExecutionFailure

        failure = ExecutionFailure(
            failed_step_index=2,
            failed_step_name="scale_deployment",
            reason="OOMKilled",
            message="Container exceeded memory limit during scale operation",
            exit_code=137,
            failed_at="2025-11-29T10:30:00Z",
            execution_time="2m34s"
        )

        assert failure.reason == "OOMKilled"
        assert failure.failed_step_index == 2
        assert failure.exit_code == 137
        assert failure.execution_time == "2m34s"

    def test_execution_failure_step_index_must_be_non_negative(self):
        """
        Business Outcome: Step index is 0-based, cannot be negative
        """
        from src.models.recovery_models import ExecutionFailure

        with pytest.raises(ValidationError):
            ExecutionFailure(
                failed_step_index=-1,  # Invalid
                failed_step_name="test",
                reason="Error",
                message="Test",
                failed_at="2025-11-29T10:30:00Z",
                execution_time="1m"
            )

    def test_execution_failure_exit_code_is_optional(self):
        """
        Business Outcome: Not all failures have exit codes
        """
        from src.models.recovery_models import ExecutionFailure

        failure = ExecutionFailure(
            failed_step_index=0,
            failed_step_name="schedule",
            reason="FailedScheduling",
            message="No nodes available",
            failed_at="2025-11-29T10:30:00Z",
            execution_time="0s"
            # No exit_code - scheduling failures don't have one
        )

        assert failure.exit_code is None


class TestPreviousExecutionModel:
    """
    Tests for PreviousExecution model

    Business Outcome: Complete context about the previous failed
    execution attempt for intelligent recovery.
    """

    def test_previous_execution_combines_all_context(self):
        """
        Business Outcome: Single object contains all needed context
        """
        from src.models.recovery_models import (
            PreviousExecution, OriginalRCA, SelectedWorkflowSummary, ExecutionFailure
        )

        previous = PreviousExecution(
            workflow_execution_ref="req-2025-11-29-abc123-we-1",
            original_rca=OriginalRCA(
                summary="Memory exhaustion",
                signal_name="OOMKilled",
                severity="high"
            ),
            selected_workflow=SelectedWorkflowSummary(
                workflow_id="scale-horizontal-v1",
                version="1.0.0",
                execution_bundle="kubernaut/workflow-scale:v1.0.0",
                rationale="Scale to distribute load"
            ),
            failure=ExecutionFailure(
                failed_step_index=2,
                failed_step_name="scale_deployment",
                reason="OOMKilled",
                message="Container exceeded memory limit",
                failed_at="2025-11-29T10:30:00Z",
                execution_time="2m34s"
            )
        )

        assert previous.workflow_execution_ref == "req-2025-11-29-abc123-we-1"
        assert previous.original_rca.signal_name == "OOMKilled"
        assert previous.selected_workflow.workflow_id == "scale-horizontal-v1"
        assert previous.failure.reason == "OOMKilled"


class TestRecoveryRequestWithPreviousExecution:
    """
    Tests for RecoveryRequest with recovery-specific fields

    Business Outcome: RecoveryRequest can carry previous execution
    context while maintaining backward compatibility.
    """

    def test_recovery_request_accepts_previous_execution(self):
        """
        Business Outcome: Recovery requests include failed attempt context
        """
        from src.models.recovery_models import (
            RecoveryRequest, PreviousExecution, OriginalRCA,
            SelectedWorkflowSummary, ExecutionFailure
        )

        request = RecoveryRequest(
            incident_id="inc-001",
            remediation_id="req-2025-11-29-abc123",
            is_recovery_attempt=True,
            recovery_attempt_number=2,
            previous_execution=PreviousExecution(
                workflow_execution_ref="req-2025-11-29-abc123-we-1",
                original_rca=OriginalRCA(
                    summary="Memory issue",
                    signal_name="OOMKilled",
                    severity="high"
                ),
                selected_workflow=SelectedWorkflowSummary(
                    workflow_id="scale-v1",
                    version="1.0.0",
                    execution_bundle="test:latest",
                    rationale="Scale"
                ),
                failure=ExecutionFailure(
                    failed_step_index=0,
                    failed_step_name="scale",
                    reason="OOMKilled",
                    message="Failed",
                    failed_at="2025-11-29T10:30:00Z",
                    execution_time="1m"
                )
            )
        )

        assert request.is_recovery_attempt is True
        assert request.recovery_attempt_number == 2
        assert request.previous_execution.failure.reason == "OOMKilled"

    def test_recovery_attempt_number_must_be_positive(self):
        """
        Business Outcome: Attempt number starts at 1
        """
        from src.models.recovery_models import RecoveryRequest

        with pytest.raises(ValidationError):
            RecoveryRequest(
                incident_id="inc-001",
                remediation_id="req-001",
                is_recovery_attempt=True,
                recovery_attempt_number=0  # Invalid - must be >= 1
            )


class TestDetectedLabelsModel:
    """
    Tests for DetectedLabels model

    Business Outcome: Capture auto-detected cluster characteristics
    for workflow filtering and LLM context.
    """

    def test_detected_labels_captures_gitops_state(self):
        """
        Business Outcome: Know if namespace is GitOps-managed
        """
        from src.models.incident_models import DetectedLabels

        labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="argocd"
        )

        assert labels.gitOpsManaged is True
        assert labels.gitOpsTool == "argocd"

    def test_detected_labels_captures_protection_state(self):
        """
        Business Outcome: Know if workload has PDB/HPA protection
        """
        from src.models.incident_models import DetectedLabels

        labels = DetectedLabels(
            pdbProtected=True,
            hpaEnabled=True
        )

        assert labels.pdbProtected is True
        assert labels.hpaEnabled is True

    def test_detected_labels_captures_workload_characteristics(self):
        """
        Business Outcome: Know if workload is stateful/Helm-managed
        """
        from src.models.incident_models import DetectedLabels

        labels = DetectedLabels(
            stateful=True,
            helmManaged=True
        )

        assert labels.stateful is True
        assert labels.helmManaged is True

    def test_detected_labels_captures_security_posture(self):
        """
        Business Outcome: Know security constraints for workflow selection
        DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED (PSP deprecated)
        """
        from src.models.incident_models import DetectedLabels

        labels = DetectedLabels(
            networkIsolated=True,
            serviceMesh="istio"
        )

        assert labels.networkIsolated is True
        assert labels.serviceMesh == "istio"

    def test_detected_labels_defaults_to_safe_values(self):
        """
        Business Outcome: Default values don't assume capabilities
        """
        from src.models.incident_models import DetectedLabels

        labels = DetectedLabels()

        assert labels.gitOpsManaged is False
        assert labels.pdbProtected is False
        assert labels.stateful is False


class TestEnrichmentResultsModel:
    """
    Tests for EnrichmentResults model

    Business Outcome: Container for all enrichment data including
    DetectedLabels for workflow filtering.
    """

    def test_enrichment_results_contains_kubernetes_context_and_custom_labels(self):
        """
        Business Outcome: EnrichmentResults carries kubernetesContext and customLabels.
        ADR-056: detectedLabels removed (computed by HAPI post-RCA).
        """
        from src.models.incident_models import EnrichmentResults

        enrichment = EnrichmentResults(
            kubernetesContext={"pod_status": "Running"},
            customLabels={"team": ["platform"]},
        )

        assert enrichment.kubernetesContext["pod_status"] == "Running"
        assert enrichment.customLabels["team"] == ["platform"]

    def test_enrichment_results_allows_kubernetes_context(self):
        """
        Business Outcome: Raw Kubernetes context can be included
        """
        from src.models.incident_models import EnrichmentResults

        enrichment = EnrichmentResults(
            kubernetesContext={
                "pod_status": "Running",
                "node_name": "worker-1",
                "container_states": ["running", "running"]
            }
        )

        assert enrichment.kubernetesContext["pod_status"] == "Running"

    def test_enrichment_results_allows_custom_labels(self):
        """
        Business Outcome: Custom labels from resource annotations

        DD-HAPI-001: customLabels format is Dict[str, List[str]] (subdomain → list of values)
        """
        from src.models.incident_models import EnrichmentResults

        enrichment = EnrichmentResults(
            customLabels={
                "team": ["platform"],
                "cost-center": ["engineering"]
            }
        )

        assert enrichment.customLabels["team"] == ["platform"]

    def test_enrichment_results_all_fields_optional(self):
        """
        Business Outcome: All EnrichmentResults fields are optional.
        ADR-056: enrichmentQuality removed along with detectedLabels.
        """
        from src.models.incident_models import EnrichmentResults

        enrichment = EnrichmentResults()

        assert enrichment.kubernetesContext is None
        assert enrichment.customLabels is None


class TestIncidentRequestWithEnrichmentResults:
    """
    Tests for IncidentRequest with enrichment_results field

    Business Outcome: Incident requests can carry DetectedLabels
    for workflow filtering.
    """

    def test_incident_request_accepts_enrichment_results(self):
        """
        Business Outcome: Incident analysis includes cluster context
        """
        from src.models.incident_models import IncidentRequest, EnrichmentResults

        request = IncidentRequest(
            incident_id="inc-001",
            remediation_id="req-2025-11-29-abc123",
            signal_name="OOMKilled",
            severity="high",
            signal_source="prometheus",
            resource_namespace="production",
            resource_kind="Deployment",
            resource_name="api-server",
            error_message="Container exceeded memory limit",
            environment="production",
            priority="P1",
            risk_tolerance="medium",
            business_category="critical",
            cluster_name="prod-us-west-2",
            enrichment_results=EnrichmentResults(
                kubernetesContext={"pod_status": "Running"},
                customLabels={"team": ["platform"]},
            )
        )

        assert request.enrichment_results is not None
        assert request.enrichment_results.kubernetesContext["pod_status"] == "Running"

    def test_incident_request_works_without_enrichment_results(self):
        """
        Business Outcome: Backward compatible - enrichment optional
        """
        from src.models.incident_models import IncidentRequest

        request = IncidentRequest(
            incident_id="inc-001",
            remediation_id="req-2025-11-29-abc123",
            signal_name="OOMKilled",
            severity="high",
            signal_source="prometheus",
            resource_namespace="production",
            resource_kind="Deployment",
            resource_name="api-server",
            error_message="Container exceeded memory limit",
            environment="production",
            priority="P1",
            risk_tolerance="medium",
            business_category="critical",
            cluster_name="prod-us-west-2"
        )

        assert request.enrichment_results is None

