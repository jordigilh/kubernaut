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
Incident Analysis DetectedLabels Integration Tests (DD-HAPI-001)

Business Requirements: BR-HAPI-002 (Incident Analysis)
Design Decision: DD-HAPI-001 (DetectedLabels for workflow filtering)

Tests validate BUSINESS OUTCOMES:
- Incident prompts include DetectedLabels when provided
- LLM receives cluster context for appropriate workflow selection
- MCP filter instructions are included for workflow filtering
"""

from src.models.incident_models import DetectedLabels


class TestIncidentPromptDetectedLabels:
    """
    Tests for DetectedLabels integration in incident prompts

    Business Outcome: Incident analysis receives cluster context
    for appropriate workflow filtering.
    """

    def test_incident_prompt_omits_detected_labels_section(self):
        """
        ADR-056: DetectedLabels are now computed at runtime by HAPI's
        get_resource_context tool, not injected into the initial prompt
        from enrichment_results.
        """
        from src.extensions.incident import _create_incident_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "signal_name": "OOMKilled",
            "severity": "high",
            "signal_source": "prometheus",
            "resource_namespace": "production",
            "resource_kind": "Deployment",
            "resource_name": "api-server",
            "error_message": "Container exceeded memory limit",
            "environment": "production",
            "priority": "P1",
            "risk_tolerance": "medium",
            "business_category": "critical",
            "cluster_name": "prod-us-west-2",
            "enrichment_results": {
                "detectedLabels": {
                    "gitOpsManaged": True,
                    "gitOpsTool": "argocd",
                    "pdbProtected": True
                }
            }
        }

        prompt = _create_incident_investigation_prompt(request_data)

        assert "Cluster Environment Characteristics" not in prompt
        assert "AUTO-DETECTED" not in prompt

    def test_incident_prompt_excludes_detected_labels_when_not_provided(self):
        """
        Business Outcome: No confusing empty section when no labels
        """
        from src.extensions.incident import _create_incident_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "signal_name": "OOMKilled",
            "severity": "high",
            "signal_source": "prometheus",
            "resource_namespace": "production",
            "resource_kind": "Deployment",
            "resource_name": "api-server",
            "error_message": "Container exceeded memory limit",
            "environment": "production",
            "priority": "P1",
            "risk_tolerance": "medium",
            "business_category": "critical",
            "cluster_name": "prod-us-west-2"
            # No enrichment_results
        }

        prompt = _create_incident_investigation_prompt(request_data)

        # Business outcome: No DetectedLabels section when not provided
        # The section should not appear
        assert "AUTO-DETECTED" not in prompt

    def test_incident_prompt_includes_mcp_filter_instructions(self):
        """
        Business Outcome: LLM knows to filter workflows by cluster characteristics
        """
        from src.extensions.incident import _create_incident_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "signal_name": "OOMKilled",
            "severity": "high",
            "signal_source": "prometheus",
            "resource_namespace": "production",
            "resource_kind": "Deployment",
            "resource_name": "api-server",
            "error_message": "Container exceeded memory limit",
            "environment": "production",
            "priority": "P1",
            "risk_tolerance": "medium",
            "business_category": "critical",
            "cluster_name": "prod-us-west-2",
            "enrichment_results": {
                "detectedLabels": {
                    "gitOpsManaged": True,
                    "gitOpsTool": "argocd",
                    "stateful": True
                }
            }
        }

        prompt = _create_incident_investigation_prompt(request_data)

        # Business outcome: Workflow discovery context present (DD-HAPI-017: three-step protocol)
        assert "Workflow Discovery Context" in prompt or "workflow discovery" in prompt.lower()
        assert "filters" in prompt.lower() or "detected" in prompt.lower()

    def test_incident_prompt_handles_empty_enrichment_results(self):
        """
        Business Outcome: Empty enrichment_results doesn't break prompt
        """
        from src.extensions.incident import _create_incident_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "signal_name": "OOMKilled",
            "severity": "high",
            "signal_source": "prometheus",
            "resource_namespace": "production",
            "resource_kind": "Deployment",
            "resource_name": "api-server",
            "error_message": "Container exceeded memory limit",
            "environment": "production",
            "priority": "P1",
            "risk_tolerance": "medium",
            "business_category": "critical",
            "cluster_name": "prod-us-west-2",
            "enrichment_results": {}  # Empty dict
        }

        # Should not raise
        prompt = _create_incident_investigation_prompt(request_data)
        assert "Incident Analysis Request" in prompt

    def test_incident_prompt_handles_none_detected_labels(self):
        """
        Business Outcome: None detectedLabels doesn't break prompt
        """
        from src.extensions.incident import _create_incident_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "signal_name": "OOMKilled",
            "severity": "high",
            "signal_source": "prometheus",
            "resource_namespace": "production",
            "resource_kind": "Deployment",
            "resource_name": "api-server",
            "error_message": "Container exceeded memory limit",
            "environment": "production",
            "priority": "P1",
            "risk_tolerance": "medium",
            "business_category": "critical",
            "cluster_name": "prod-us-west-2",
            "enrichment_results": {
                "detectedLabels": None  # Explicitly None
            }
        }

        # Should not raise
        prompt = _create_incident_investigation_prompt(request_data)
        assert "Incident Analysis Request" in prompt


class TestIncidentClusterContextSection:
    """
    Tests for _build_cluster_context_section in incident.py

    Business Outcome: Cluster context is properly converted to
    natural language for LLM understanding.
    """

    def test_gitops_context_warns_about_direct_changes(self):
        """
        Business Outcome: GitOps-managed namespaces get explicit warning
        """
        from src.extensions.incident import _build_cluster_context_section

        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="flux"
        )

        context = _build_cluster_context_section(detected_labels)

        assert "GitOps" in context
        assert "flux" in context
        assert "direct" in context.lower() or "gitops-aware" in context.lower()

    def test_security_context_mentions_restrictions(self):
        """
        Business Outcome: Security constraints are communicated
        DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED (PSP deprecated)
        """
        from src.extensions.incident import _build_cluster_context_section

        detected_labels = DetectedLabels(
            networkIsolated=True
        )

        context = _build_cluster_context_section(detected_labels)

        assert "NetworkPolicy" in context or "network" in context.lower()


class TestIncidentMCPFilterInstructions:
    """
    Tests for _build_mcp_filter_instructions in incident.py

    Business Outcome: MCP search requests include appropriate filters.
    """

    def test_mcp_instructions_include_all_detected_labels(self):
        """
        Business Outcome: All labels included in filter JSON
        """
        from src.extensions.incident import _build_mcp_filter_instructions

        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="argocd",
            pdbProtected=True,
            stateful=False,
            helmManaged=True
        )

        instructions = _build_mcp_filter_instructions(detected_labels)

        assert "gitops_managed" in instructions.lower()
        assert "pdb_protected" in instructions.lower()
        assert "helm_managed" in instructions.lower()

    def test_mcp_instructions_empty_for_all_false_labels(self):
        """
        Business Outcome: No filter clutter for default values
        """
        from src.extensions.incident import _build_mcp_filter_instructions

        detected_labels = DetectedLabels(
            gitOpsManaged=False,
            pdbProtected=False,
            stateful=False
        )

        instructions = _build_mcp_filter_instructions(detected_labels)

        # Should still include workflow discovery context with false values (DD-HAPI-017)
        assert "Workflow Discovery Context" in instructions or "workflow discovery" in instructions.lower()

