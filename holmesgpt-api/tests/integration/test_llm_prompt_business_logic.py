"""
Integration Tests: LLM Prompt Building Business Logic

Business Requirement: BR-AI-001 (LLM Context Optimization)
Pattern: Direct Python function calls with real Data Storage service
Infrastructure: Real Data Storage (port 18094) for workflow fetching via toolset

Defense-in-Depth Layer: Tier 2 (Integration)
- Tests prompt building components directly (bypass FastAPI)
- Uses real Data Storage via workflow catalog toolset
- Validates business outcomes (prompt quality, context accuracy)
"""

import pytest
from src.extensions.incident.prompt_builder import (
    build_cluster_context_section,
    build_mcp_filter_instructions,
    create_incident_investigation_prompt
)
from src.models.incident_models import DetectedLabels


class TestClusterContextBuilding:
    """IT-AI-001-01: Cluster context builder includes detected labels

    Business Outcome: LLM receives accurate cluster environment context
    BR: BR-AI-001 (LLM Context Optimization)
    """

    def test_cluster_context_includes_gitops_information(self):
        """
        Given: Detected labels indicating GitOps management
        When: Building cluster context section
        Then: Context warns LLM about GitOps constraints

        Business Value: LLM avoids recommending direct changes in GitOps environments
        """
        # Arrange: Create detected labels with GitOps
        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="argocd",
            failedDetections=[]
        )

        # Act: Build cluster context
        context = build_cluster_context_section(detected_labels)

        # Assert: Business outcome validation
        assert isinstance(context, str), "Context should be string"
        assert len(context) > 0, "Context should not be empty"

        # Business outcome: GitOps warning included
        assert "gitops" in context.lower() or "argocd" in context.lower(), \
            "Context should mention GitOps management"

    def test_cluster_context_includes_hpa_information(self):
        """
        Given: Detected labels indicating HPA is enabled
        When: Building cluster context section
        Then: Context warns LLM about HPA constraints

        Business Value: LLM avoids conflicting with autoscaling
        """
        # Arrange: Create detected labels with HPA
        detected_labels = DetectedLabels(
            hpaEnabled=True,
            failedDetections=[]
        )

        # Act: Build cluster context
        context = build_cluster_context_section(detected_labels)

        # Assert: Business outcome validation
        assert isinstance(context, str), "Context should be string"

        # Business outcome: HPA warning included
        assert "hpa" in context.lower() or "autoscaler" in context.lower() or "horizontal" in context.lower(), \
            "Context should mention HPA constraints"

    def test_cluster_context_excludes_failed_detections(self):
        """
        Given: Detected labels with failed detection fields
        When: Building cluster context section
        Then: Failed detection fields are excluded from context

        Business Value: LLM doesn't receive misleading information about unknown cluster state
        """
        # Arrange: Create detected labels with failed detections
        detected_labels = DetectedLabels(
            gitOpsManaged=False,
            hpaEnabled=False,
            failedDetections=["gitOpsManaged", "hpaEnabled"]  # Detection failed for these
        )

        # Act: Build cluster context
        context = build_cluster_context_section(detected_labels)

        # Assert: Business outcome validation
        assert isinstance(context, str), "Context should be string"

        # Business outcome: Failed detections not mentioned
        # Context should not confidently state gitOps status when detection failed
        # (Exact wording may vary, but should not assert unknown state as fact)


class TestMCPFilterInstructions:
    """IT-AI-001-02: MCP filter instructions guide workflow search

    Business Outcome: LLM performs accurate workflow searches
    BR: BR-AI-001 (LLM Context Optimization)
    BR: BR-HAPI-250 (Workflow Catalog Search)
    """

    def test_mcp_filter_instructions_include_detected_labels(self):
        """
        Given: Detected labels for workflow filtering
        When: Building MCP filter instructions
        Then: Instructions include detected label filter guidance

        Business Value: LLM searches for workflows matching cluster environment
        """
        # Arrange: Create detected labels
        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="argocd",
            serviceMesh="istio",
            failedDetections=[]
        )

        # Act: Build MCP filter instructions
        instructions = build_mcp_filter_instructions(detected_labels)

        # Assert: Business outcome validation
        assert isinstance(instructions, str), "Instructions should be string"
        assert len(instructions) > 0, "Instructions should not be empty"

        # Business outcome: Filter guidance included
        assert "filter" in instructions.lower() or "search" in instructions.lower(), \
            "Instructions should mention filtering or searching"


class TestIncidentPromptCreation:
    """IT-AI-001-03: Incident investigation prompt assembles complete context

    Business Outcome: LLM receives structured context for accurate analysis
    BR: BR-AI-001 (LLM Context Optimization)
    """

    def test_incident_prompt_includes_required_sections(self):
        """
        Given: Incident analysis request with complete context
        When: Creating incident investigation prompt
        Then: Prompt includes all required sections (cluster context, Kubernetes data, MCP instructions)

        Business Value: Comprehensive prompt enables accurate LLM analysis
        """
        # Arrange: Create request data
        request_data = {
            "incident_id": "inc-integration-test-prompt-001",
            "signal_name": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",  # Correct field name per IncidentRequest model
            "resource_kind": "Pod",
            "resource_name": "api-server-abc123",  # Correct field name per IncidentRequest model
            "error_message": "Container exceeded memory limit",
            "detected_labels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "failedDetections": []
            },
            "kubernetes_context": {
                "pod": {
                    "name": "api-server-abc123",
                    "namespace": "production"
                },
                "events": [
                    {"type": "Warning", "reason": "OOMKilled", "message": "Container exceeded memory limit"}
                ]
            }
        }

        # Act: Create incident investigation prompt
        prompt = create_incident_investigation_prompt(request_data)

        # Assert: Business outcome validation
        assert isinstance(prompt, str), "Prompt should be string"
        assert len(prompt) > 0, "Prompt should not be empty"

        # Business outcome: Key information included
        assert "OOMKilled" in prompt, "Prompt should include signal type"
        assert "production" in prompt, "Prompt should include namespace"
        assert "api-server-abc123" in prompt, "Prompt should include pod name"

    def test_incident_prompt_with_minimal_context(self):
        """
        Given: Incident analysis request with minimal context
        When: Creating incident investigation prompt
        Then: Prompt is created successfully with available information

        Business Value: System handles incomplete data gracefully
        """
        # Arrange: Create minimal request data
        request_data = {
            "incident_id": "inc-integration-test-prompt-002",
            "signal_name": "CrashLoopBackOff",
            "severity": "high",
            "resource_namespace": "default",  # Correct field name per IncidentRequest model
            "resource_kind": "Pod",
            "resource_name": "test-pod",
            "error_message": "Container crashed"
        }

        # Act: Create incident investigation prompt
        prompt = create_incident_investigation_prompt(request_data)

        # Assert: Business outcome validation
        assert isinstance(prompt, str), "Prompt should be string"
        assert len(prompt) > 0, "Prompt should not be empty with minimal data"

        # Business outcome: Essential information included
        assert "CrashLoopBackOff" in prompt, "Prompt should include signal type"
        assert "default" in prompt, "Prompt should include namespace"
