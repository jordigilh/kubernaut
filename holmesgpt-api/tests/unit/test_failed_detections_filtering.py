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
FailedDetections Filtering Tests (DD-WORKFLOW-001 v2.1)

Business Requirement: BR-HAPI-194 (Honor failedDetections in workflow filtering)
Design Decision: DD-WORKFLOW-001 v2.1 (DetectedLabels failedDetections field)

Tests validate BUSINESS OUTCOMES:
- Fields in failedDetections are EXCLUDED from workflow search filters
- Fields in failedDetections are EXCLUDED from cluster context (LLM prompt)
- Only fields NOT in failedDetections are used for workflow filtering

Key Distinction (per SignalProcessing team):
| Scenario    | pdbProtected | failedDetections     | Meaning                    |
|-------------|--------------|----------------------|----------------------------|
| PDB exists  | true         | []                   | ✅ Has PDB - use for filter |
| No PDB      | false        | []                   | ✅ No PDB - use for filter  |
| RBAC denied | false        | ["pdbProtected"]     | ⚠️ Unknown - skip filter    |

"Resource doesn't exist" ≠ detection failure - it's a successful detection with result `false`.
"""


from src.models.incident_models import DetectedLabels


class TestMCPFilterInstructionsFailedDetections:
    """
    Tests for _build_mcp_filter_instructions honoring failedDetections.

    Business Outcome: Workflow search filters EXCLUDE fields where detection failed,
    preventing incorrect workflow matches based on unknown values.
    """

    def test_mcp_filter_excludes_single_failed_detection(self):
        """
        Business Outcome: pdbProtected filter excluded when detection failed (RBAC denied)

        Scenario: SignalProcessing couldn't check PDB due to RBAC - we don't know if PDB exists
        Expected: pdb_protected should NOT appear in filters
        """
        from src.extensions.incident import _build_mcp_filter_instructions

        detected_labels = DetectedLabels(
            failedDetections=["pdbProtected"],
            gitOpsManaged=True,
            gitOpsTool="argocd",
            pdbProtected=False,  # Value is unreliable - detection failed
            stateful=True
        )

        instructions = _build_mcp_filter_instructions(detected_labels)

        # Business outcome: pdb_protected should NOT be in filters
        assert "pdb_protected" not in instructions.lower()
        # But other labels should still be present
        assert "gitops_managed" in instructions.lower()
        assert "stateful" in instructions.lower()

    def test_mcp_filter_excludes_multiple_failed_detections(self):
        """
        Business Outcome: Multiple failed detections are all excluded from filters

        Scenario: RBAC denied access to both PDB and HPA checks
        Expected: Neither pdb_protected nor hpa_enabled in filters
        """
        from src.extensions.incident import _build_mcp_filter_instructions

        detected_labels = DetectedLabels(
            failedDetections=["pdbProtected", "hpaEnabled"],
            gitOpsManaged=True,
            pdbProtected=False,   # Unreliable
            hpaEnabled=False,     # Unreliable
            stateful=True,        # This one is reliable
            helmManaged=True      # This one is reliable
        )

        instructions = _build_mcp_filter_instructions(detected_labels)

        # Business outcome: Failed detections excluded
        assert "pdb_protected" not in instructions.lower()
        # Note: hpaEnabled doesn't have a direct filter mapping currently
        # But other labels should still be present
        assert "gitops_managed" in instructions.lower()
        assert "stateful" in instructions.lower()
        assert "helm_managed" in instructions.lower()

    def test_mcp_filter_includes_all_when_no_failed_detections(self):
        """
        Business Outcome: All labels included when failedDetections is empty

        Scenario: All detections succeeded
        Expected: All labels appear in filters
        """
        from src.extensions.incident import _build_mcp_filter_instructions

        detected_labels = DetectedLabels(
            failedDetections=[],
            gitOpsManaged=True,
            pdbProtected=True,
            stateful=False
        )

        instructions = _build_mcp_filter_instructions(detected_labels)

        # Business outcome: All labels included
        assert "gitops_managed" in instructions.lower()
        assert "pdb_protected" in instructions.lower()
        assert "stateful" in instructions.lower()

    def test_mcp_filter_handles_missing_failed_detections_key(self):
        """
        Business Outcome: Backward compatibility - no failedDetections = all detections succeeded

        Scenario: Old-format DetectedLabels without failedDetections field
        Expected: All labels included (assume all detections succeeded)
        """
        from src.extensions.incident import _build_mcp_filter_instructions

        detected_labels = DetectedLabels(
            # failedDetections defaults to []
            gitOpsManaged=True,
            pdbProtected=False,
            stateful=True
        )

        instructions = _build_mcp_filter_instructions(detected_labels)

        # Business outcome: All labels included when key missing
        assert "gitops_managed" in instructions.lower()
        assert "pdb_protected" in instructions.lower()
        assert "stateful" in instructions.lower()

    def test_mcp_filter_excludes_gitops_when_detection_failed(self):
        """
        Business Outcome: GitOps filter excluded when ArgoCD/Flux detection failed

        Scenario: Couldn't determine if namespace is GitOps-managed
        Expected: gitops_managed and gitops_tool NOT in filters
        """
        from src.extensions.incident import _build_mcp_filter_instructions

        detected_labels = DetectedLabels(
            failedDetections=["gitOpsManaged"],
            gitOpsManaged=False,  # Unreliable
            gitOpsTool="",        # Should also be excluded
            pdbProtected=True,    # Reliable
            stateful=True         # Reliable
        )

        instructions = _build_mcp_filter_instructions(detected_labels)

        # Business outcome: GitOps fields excluded
        assert "gitops_managed" not in instructions.lower()
        # Note: gitOpsTool should also be excluded when gitOpsManaged failed
        # Other labels should be present
        assert "pdb_protected" in instructions.lower()
        assert "stateful" in instructions.lower()


class TestClusterContextSectionFailedDetections:
    """
    Tests for _build_cluster_context_section honoring failedDetections.

    Business Outcome: LLM context EXCLUDES characteristics where detection failed,
    preventing misleading recommendations based on unknown cluster state.
    """

    def test_cluster_context_excludes_pdb_when_detection_failed(self):
        """
        Business Outcome: LLM not told about PDB when we couldn't detect it

        Scenario: RBAC denied PDB check
        Expected: No PDB-related text in cluster context
        """
        from src.extensions.incident import _build_cluster_context_section

        detected_labels = DetectedLabels(
            failedDetections=["pdbProtected"],
            pdbProtected=False,  # Unreliable
            gitOpsManaged=True,
            gitOpsTool="argocd"
        )

        context = _build_cluster_context_section(detected_labels)

        # Business outcome: PDB not mentioned when detection failed
        assert "PDB" not in context.upper()
        assert "PodDisruptionBudget" not in context
        # But GitOps should still be mentioned
        assert "GitOps" in context
        assert "argocd" in context

    def test_cluster_context_excludes_gitops_when_detection_failed(self):
        """
        Business Outcome: LLM not told about GitOps when detection failed

        Scenario: Couldn't determine GitOps status
        Expected: No GitOps-related text in cluster context
        """
        from src.extensions.incident import _build_cluster_context_section

        detected_labels = DetectedLabels(
            failedDetections=["gitOpsManaged"],
            gitOpsManaged=True,   # Value present but unreliable
            gitOpsTool="argocd",  # Also unreliable
            pdbProtected=True,    # Reliable
            stateful=True         # Reliable
        )

        context = _build_cluster_context_section(detected_labels)

        # Business outcome: GitOps not mentioned when detection failed
        assert "GitOps" not in context
        assert "argocd" not in context.lower()
        # But other labels should be mentioned
        assert "PDB" in context.upper() or "PodDisruptionBudget" in context
        assert "STATEFUL" in context.upper() or "StatefulSet" in context

    def test_cluster_context_excludes_stateful_when_detection_failed(self):
        """
        Business Outcome: LLM not told about stateful nature when detection failed

        Scenario: Couldn't determine if workload is stateful
        Expected: No stateful-related text in cluster context
        """
        from src.extensions.incident import _build_cluster_context_section

        detected_labels = DetectedLabels(
            failedDetections=["stateful"],
            stateful=True,       # Unreliable
            gitOpsManaged=True,  # Reliable
            gitOpsTool="flux"
        )

        context = _build_cluster_context_section(detected_labels)

        # Business outcome: Stateful not mentioned when detection failed
        assert "STATEFUL" not in context.upper()
        assert "StatefulSet" not in context
        assert "PVC" not in context.upper()
        # But GitOps should still be mentioned
        assert "GitOps" in context
        assert "flux" in context

    def test_cluster_context_includes_all_when_no_failed_detections(self):
        """
        Business Outcome: All characteristics included when all detections succeeded
        """
        from src.extensions.incident import _build_cluster_context_section

        detected_labels = DetectedLabels(
            failedDetections=[],
            gitOpsManaged=True,
            gitOpsTool="argocd",
            pdbProtected=True,
            stateful=True
        )

        context = _build_cluster_context_section(detected_labels)

        # Business outcome: All characteristics mentioned
        assert "GitOps" in context
        assert "argocd" in context
        assert "PDB" in context.upper() or "PodDisruptionBudget" in context
        assert "STATEFUL" in context.upper() or "StatefulSet" in context


class TestSearchWorkflowsFailedDetections:
    """
    Tests for _search_workflows stripping failedDetections from detected_labels.

    Business Outcome: Data Storage Service receives ONLY reliable detected_labels,
    ensuring workflow filtering is based on known cluster characteristics.
    """

    def test_search_workflows_strips_failed_detection_fields(self):
        """
        Business Outcome: Data Storage receives only reliable labels

        Scenario: pdbProtected detection failed
        Expected: detected_labels sent to DS should NOT include pdbProtected
        """
        from src.toolsets.workflow_discovery import strip_failed_detections

        detected_labels = DetectedLabels(
            failedDetections=["pdbProtected"],
            gitOpsManaged=True,
            gitOpsTool="argocd",
            pdbProtected=False,  # Should be stripped
            stateful=True
        )

        clean_labels = strip_failed_detections(detected_labels)
        clean_dict = clean_labels.model_dump(exclude_defaults=True, exclude_none=True)

        # Business outcome: Failed detection field removed
        assert "pdbProtected" not in clean_dict
        # But other fields preserved
        assert clean_dict.get("gitOpsManaged") is True
        assert clean_dict.get("gitOpsTool") == "argocd"
        assert clean_dict.get("stateful") is True
        # failedDetections itself should also be stripped (not needed by DS)
        assert "failedDetections" not in clean_dict

    def test_search_workflows_strips_multiple_failed_detections(self):
        """
        Business Outcome: Multiple failed detections all stripped
        """
        from src.toolsets.workflow_discovery import strip_failed_detections

        detected_labels = DetectedLabels(
            failedDetections=["pdbProtected", "hpaEnabled", "networkIsolated"],
            gitOpsManaged=True,
            pdbProtected=False,
            hpaEnabled=False,
            networkIsolated=False,
            stateful=True,
            helmManaged=True
        )

        clean_labels = strip_failed_detections(detected_labels)
        clean_dict = clean_labels.model_dump(exclude_defaults=True, exclude_none=True)

        # Business outcome: All failed detections removed
        assert "pdbProtected" not in clean_dict
        assert "hpaEnabled" not in clean_dict
        assert "networkIsolated" not in clean_dict
        # Reliable fields preserved
        assert clean_dict.get("gitOpsManaged") is True
        assert clean_dict.get("stateful") is True
        assert clean_dict.get("helmManaged") is True

    def test_search_workflows_preserves_all_when_no_failures(self):
        """
        Business Outcome: All labels preserved when no detection failures
        """
        from src.toolsets.workflow_discovery import strip_failed_detections

        detected_labels = DetectedLabels(
            failedDetections=[],
            gitOpsManaged=True,
            pdbProtected=True,
            stateful=False
        )

        clean_labels = strip_failed_detections(detected_labels)
        clean_dict = clean_labels.model_dump(exclude_defaults=True, exclude_none=True)

        # Business outcome: All fields preserved (except failedDetections meta field)
        assert clean_dict.get("gitOpsManaged") is True
        assert clean_dict.get("pdbProtected") is True
        # stateful is False, which is the default, so it won't be in clean_dict when using exclude_defaults
        assert clean_labels.stateful is False  # Check the model attribute directly
        assert "failedDetections" not in clean_dict

    def test_search_workflows_handles_missing_failed_detections_key(self):
        """
        Business Outcome: Backward compatibility - works without failedDetections key
        """
        from src.toolsets.workflow_discovery import strip_failed_detections

        detected_labels = DetectedLabels(
            # failedDetections defaults to []
            gitOpsManaged=True,
            pdbProtected=False
        )

        clean_labels = strip_failed_detections(detected_labels)
        clean_dict = clean_labels.model_dump(exclude_defaults=True, exclude_none=True)

        # Business outcome: All fields preserved
        assert clean_dict.get("gitOpsManaged") is True
        # pdbProtected is False, which is the default, so it won't be in clean_dict when using exclude_defaults
        assert clean_labels.pdbProtected is False  # Check the model attribute directly
