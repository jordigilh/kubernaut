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
Recovery Prompt Generation Tests (DD-RECOVERY-003)

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
Design Decisions: DD-RECOVERY-002, DD-RECOVERY-003

Tests validate BUSINESS OUTCOMES:
- Recovery prompts must include previous execution context
- Prompts must instruct LLM NOT to repeat failed workflow
- Prompts must include Kubernetes failure reason guidance
- Prompts must support DetectedLabels for workflow filtering
"""


from src.models.incident_models import DetectedLabels


class TestRecoveryPromptGeneration:
    """
    Tests for _create_recovery_investigation_prompt()

    Business Outcome: LLM receives complete failure context to make
    intelligent recovery recommendations that differ from failed attempt.
    """

    def test_recovery_prompt_includes_previous_attempt_section(self):
        """
        Business Outcome: LLM understands what was already tried
        DD-RECOVERY-003: "Previous Remediation Attempt" section at TOP
        """
        from src.extensions.recovery import _create_recovery_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "is_recovery_attempt": True,
            "recovery_attempt_number": 2,
            "previous_execution": {
                "workflow_execution_ref": "req-001-we-1",
                "original_rca": {
                    "summary": "Memory exhaustion",
                    "signal_type": "OOMKilled",
                    "severity": "high",
                    "contributing_factors": ["memory leak"]
                },
                "selected_workflow": {
                    "workflow_id": "scale-horizontal-v1",
                    "version": "1.0.0",
                    "container_image": "kubernaut/workflow-scale:v1.0.0",
                    "parameters": {"TARGET_REPLICAS": "5"},
                    "rationale": "Scale to distribute load"
                },
                "failure": {
                    "failed_step_index": 2,
                    "failed_step_name": "scale_deployment",
                    "reason": "OOMKilled",
                    "message": "Container exceeded memory limit",
                    "exit_code": 137,
                    "failed_at": "2025-11-29T10:30:00Z",
                    "execution_time": "2m34s"
                }
            },
            "signal_type": "OOMKilled",
            "severity": "high"
        }

        prompt = _create_recovery_investigation_prompt(request_data)

        # Business outcome: Prompt includes previous attempt context
        assert "Previous Remediation Attempt" in prompt
        assert "RECOVERY attempt" in prompt
        assert "scale-horizontal-v1" in prompt
        assert "OOMKilled" in prompt

    def test_recovery_prompt_instructs_not_to_repeat_workflow(self):
        """
        Business Outcome: LLM knows to select ALTERNATIVE workflow
        DD-RECOVERY-003: Explicit instruction not to repeat failed workflow
        """
        from src.extensions.recovery import _create_recovery_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "is_recovery_attempt": True,
            "recovery_attempt_number": 1,
            "previous_execution": {
                "workflow_execution_ref": "req-001-we-1",
                "original_rca": {"summary": "Test", "signal_type": "OOMKilled", "severity": "high"},
                "selected_workflow": {
                    "workflow_id": "dangerous-workflow-v1",
                    "version": "1.0.0",
                    "container_image": "test:latest",
                    "parameters": {},
                    "rationale": "Test"
                },
                "failure": {
                    "failed_step_index": 0,
                    "failed_step_name": "test",
                    "reason": "Error",
                    "message": "Failed",
                    "failed_at": "2025-11-29T10:30:00Z",
                    "execution_time": "1m"
                }
            }
        }

        prompt = _create_recovery_investigation_prompt(request_data)

        # Business outcome: Clear instruction not to repeat
        assert "DO NOT" in prompt
        assert "dangerous-workflow-v1" in prompt
        assert "ALTERNATIVE" in prompt

    def test_recovery_prompt_includes_kubernetes_reason_guidance(self):
        """
        Business Outcome: LLM gets actionable guidance for specific failure types
        DD-RECOVERY-003: Failure reason interpretation with specific guidance
        """
        from src.extensions.recovery import _create_recovery_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "is_recovery_attempt": True,
            "recovery_attempt_number": 1,
            "previous_execution": {
                "workflow_execution_ref": "req-001-we-1",
                "original_rca": {"summary": "Test", "signal_type": "OOMKilled", "severity": "high"},
                "selected_workflow": {
                    "workflow_id": "test-v1",
                    "version": "1.0.0",
                    "container_image": "test:latest",
                    "parameters": {},
                    "rationale": "Test"
                },
                "failure": {
                    "failed_step_index": 0,
                    "failed_step_name": "test",
                    "reason": "DeadlineExceeded",
                    "message": "Task timed out",
                    "failed_at": "2025-11-29T10:30:00Z",
                    "execution_time": "30m"
                }
            }
        }

        prompt = _create_recovery_investigation_prompt(request_data)

        # Business outcome: Reason-specific guidance included
        assert "DeadlineExceeded" in prompt
        assert "Failure Reason Interpretation" in prompt
        # Should include guidance for this specific reason
        assert "time" in prompt.lower() or "timeout" in prompt.lower()

    def test_recovery_prompt_includes_attempt_number(self):
        """
        Business Outcome: LLM knows this is attempt N (helps with escalation)
        DD-RECOVERY-003: Recovery attempt tracking
        """
        from src.extensions.recovery import _create_recovery_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "is_recovery_attempt": True,
            "recovery_attempt_number": 3,  # Third attempt
            "previous_execution": {
                "workflow_execution_ref": "req-001-we-2",
                "original_rca": {"summary": "Test", "signal_type": "OOMKilled", "severity": "high"},
                "selected_workflow": {
                    "workflow_id": "test-v1",
                    "version": "1.0.0",
                    "container_image": "test:latest",
                    "parameters": {},
                    "rationale": "Test"
                },
                "failure": {
                    "failed_step_index": 0,
                    "failed_step_name": "test",
                    "reason": "Error",
                    "message": "Failed again",
                    "failed_at": "2025-11-29T10:30:00Z",
                    "execution_time": "1m"
                }
            }
        }

        prompt = _create_recovery_investigation_prompt(request_data)

        # Business outcome: Attempt number visible in prompt
        assert "Attempt 3" in prompt


class TestFailureReasonGuidance:
    """
    Tests for _get_failure_reason_guidance()

    Business Outcome: Each Kubernetes failure reason gets specific,
    actionable recovery guidance for the LLM.
    """

    def test_oomkilled_guidance_suggests_memory_alternatives(self):
        """
        Business Outcome: OOMKilled gets memory-focused recovery advice
        """
        from src.extensions.recovery import _get_failure_reason_guidance

        guidance = _get_failure_reason_guidance("OOMKilled")

        # Business outcome: Memory-related suggestions
        assert "memory" in guidance.lower()
        assert "limit" in guidance.lower() or "footprint" in guidance.lower()

    def test_deadline_exceeded_guidance_suggests_timeout_alternatives(self):
        """
        Business Outcome: DeadlineExceeded gets timeout-focused recovery advice
        """
        from src.extensions.recovery import _get_failure_reason_guidance

        guidance = _get_failure_reason_guidance("DeadlineExceeded")

        # Business outcome: Timeout-related suggestions
        assert "time" in guidance.lower()

    def test_image_pull_failure_guidance_suggests_image_alternatives(self):
        """
        Business Outcome: ImagePullBackOff gets image-focused recovery advice
        """
        from src.extensions.recovery import _get_failure_reason_guidance

        guidance = _get_failure_reason_guidance("ImagePullBackOff")

        # Business outcome: Image-related suggestions
        assert "image" in guidance.lower()

    def test_unauthorized_guidance_suggests_permission_alternatives(self):
        """
        Business Outcome: Unauthorized gets permission-focused recovery advice
        """
        from src.extensions.recovery import _get_failure_reason_guidance

        guidance = _get_failure_reason_guidance("Unauthorized")

        # Business outcome: Permission-related suggestions
        assert "permission" in guidance.lower() or "rbac" in guidance.lower()

    def test_unknown_reason_provides_generic_guidance(self):
        """
        Business Outcome: Unknown reasons still get useful guidance
        """
        from src.extensions.recovery import _get_failure_reason_guidance

        guidance = _get_failure_reason_guidance("SomeUnknownReason")

        # Business outcome: Still provides actionable guidance
        assert "SomeUnknownReason" in guidance
        assert "investigate" in guidance.lower()


class TestDetectedLabelsInPrompt:
    """
    Tests for DetectedLabels integration in recovery prompts

    Business Outcome: LLM receives cluster context and uses it for
    workflow filtering to avoid incompatible workflows.
    """

    def test_prompt_includes_gitops_warning_when_gitops_managed(self):
        """
        Business Outcome: GitOps-managed namespaces get explicit warning
        to prevent direct changes and use GitOps-aware workflows
        """
        from src.extensions.recovery import _build_cluster_context_section

        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="argocd"
        )

        context = _build_cluster_context_section(detected_labels)

        # Business outcome: GitOps warning present
        assert "GitOps" in context
        assert "argocd" in context
        assert "direct changes" in context.lower() or "gitops-aware" in context.lower()

    def test_prompt_includes_pdb_warning_when_pdb_protected(self):
        """
        Business Outcome: PDB-protected workloads get PDB constraint warning
        """
        from src.extensions.recovery import _build_cluster_context_section

        detected_labels = DetectedLabels(
            pdbProtected=True
        )

        context = _build_cluster_context_section(detected_labels)

        # Business outcome: PDB warning present
        assert "PodDisruptionBudget" in context or "PDB" in context

    def test_prompt_includes_stateful_warning_when_stateful(self):
        """
        Business Outcome: Stateful workloads get stateful-aware workflow recommendation
        """
        from src.extensions.recovery import _build_cluster_context_section

        detected_labels = DetectedLabels(
            stateful=True
        )

        context = _build_cluster_context_section(detected_labels)

        # Business outcome: Stateful warning present
        assert "STATEFUL" in context or "stateful" in context.lower()

    def test_prompt_returns_no_characteristics_when_empty(self):
        """
        Business Outcome: Empty labels don't produce confusing output
        """
        from src.extensions.recovery import _build_cluster_context_section

        context = _build_cluster_context_section({})

        # Business outcome: Clear message for empty labels
        assert "No special" in context

    def test_prompt_combines_multiple_characteristics(self):
        """
        Business Outcome: Multiple labels produce combined context
        """
        from src.extensions.recovery import _build_cluster_context_section

        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="flux",
            pdbProtected=True,
            stateful=True,
            helmManaged=True
        )

        context = _build_cluster_context_section(detected_labels)

        # Business outcome: All relevant warnings combined
        assert "GitOps" in context or "flux" in context
        assert "PDB" in context or "PodDisruptionBudget" in context
        assert "STATEFUL" in context or "stateful" in context.lower()
        assert "Helm" in context


class TestMCPFilterInstructions:
    """
    Tests for MCP workflow search filter instructions

    Business Outcome: LLM receives proper instructions to filter
    workflows based on cluster characteristics.
    """

    def test_mcp_instructions_include_json_filter_template(self):
        """
        Business Outcome: LLM gets JSON template for MCP search
        """
        from src.extensions.recovery import _build_mcp_filter_instructions

        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="argocd",
            stateful=False
        )

        instructions = _build_mcp_filter_instructions(detected_labels)

        # Business outcome: Workflow discovery context present (DD-HAPI-017: three-step protocol)
        assert "Workflow Discovery Context" in instructions or "workflow discovery" in instructions.lower()
        assert "gitops_managed" in instructions.lower()

    def test_mcp_instructions_empty_for_no_labels(self):
        """
        Business Outcome: No filter instructions for empty labels
        """
        from src.extensions.recovery import _build_mcp_filter_instructions

        instructions = _build_mcp_filter_instructions({})

        # Business outcome: Empty or minimal output
        assert instructions == "" or len(instructions) < 50

    def test_mcp_instructions_highlight_gitops_priority(self):
        """
        Business Outcome: GitOps-managed namespaces get priority instruction
        """
        from src.extensions.recovery import _build_mcp_filter_instructions

        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="argocd"
        )

        instructions = _build_mcp_filter_instructions(detected_labels)

        # Business outcome: GitOps priority mentioned
        assert "gitops_aware" in instructions.lower() or "prioritize" in instructions.lower()

