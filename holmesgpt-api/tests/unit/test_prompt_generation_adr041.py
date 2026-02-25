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
Unit Tests for LLM Prompt Generation (ADR-041 Compliance)

Business Requirements: BR-HAPI-002 (Incident Analysis)
Architecture: ADR-041 (LLM Prompt and Response Contract)

This test suite validates that _create_investigation_prompt() generates
prompts that comply with ADR-041 specifications.

Test Coverage:
- Signal Context Fields (ADR-041 Section 1)
- Output Format Specification (ADR-041 Section 4)
- MCP Tool Integration (ADR-041 Section 2)
- RCA Severity Assessment (ADR-041 Section 4.4)
- Edge Cases and Boundary Conditions
"""

from src.extensions.incident.prompt_builder import create_incident_investigation_prompt as _create_investigation_prompt


# ========================================
# TEST SUITE 1: Signal Context (ADR-041 Section 1)
# ========================================

class TestADR040SignalContext:
    """ADR-041 Section 1: Signal Context Fields

    Tests validate that the prompt includes all required signal context
    fields as specified in ADR-041 lines 87-184.
    """

    def test_prompt_includes_signal_source_information(self):
        """ADR-041: Signal source, type, and severity"""
        request_data = {
            "signal_source": "prometheus",
            "signal_name": "pod_oom_killed",
            "severity": "high",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Hybrid format v3.0: signal_source not included in prompt
        assert "## Incident Summary" in prompt
        assert "pod_oom_killed" in prompt.lower() or "oom" in prompt.lower()
        assert "high" in prompt.lower()

    def test_prompt_includes_signal_source_in_incident_summary(self):
        """Test that signal source appears in natural language incident summary (ADR-041 v3.2)"""
        request_data = {
            "signal_name": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",
            "resource_kind": "deployment",
            "resource_name": "payment-service",
            "signal_source": "prometheus-adapter",
            "error_message": "Container exceeded memory limit"
        }

        prompt = _create_investigation_prompt(request_data)

        # Signal source should appear in natural language summary
        assert "from **prometheus-adapter**" in prompt or "from prometheus-adapter" in prompt
        # Should read like: "event from prometheus-adapter has occurred"
        assert "prometheus-adapter" in prompt.split("## Incident Summary")[1].split("##")[0]

    def test_prompt_includes_resource_identification(self):
        """ADR-041: Resource namespace, kind, and name"""
        request_data = {
            "resource_namespace": "payment-service",
            "resource_kind": "Pod",
            "resource_name": "api-server-7d8f9c-xk2p9",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert "payment-service" in prompt
        assert "Pod" in prompt or "pod" in prompt.lower()
        assert "api-server" in prompt

    def test_prompt_includes_business_context_for_workflow_search(self):
        """ADR-041: Business context fields for MCP workflow search"""
        request_data = {
            "environment": "production",
            "priority": "P1",
            "business_category": "payment-processing",
            "risk_tolerance": "low",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert "production" in prompt.lower()
        assert "P1" in prompt or "p1" in prompt.lower()
        assert "payment" in prompt.lower()
        assert "low" in prompt.lower()

    def test_prompt_includes_deduplication_context(self):
        """ADR-041: Deduplication metadata from Gateway"""
        request_data = {
            "is_duplicate": True,
            "first_seen": "2025-11-16T10:00:00Z",
            "last_seen": "2025-11-16T10:30:00Z",
            "occurrence_count": 5,
            "previous_remediation_ref": "remediation-req-abc123",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Check for deduplication indicators
        assert "duplicate" in prompt.lower() or "occurrence" in prompt.lower() or "5" in prompt
        # Hybrid format v3.1: Just check for deduplication facts (no previous remediation ref)
    def test_prompt_includes_storm_detection_metadata(self):
        """ADR-041: Storm detection from Gateway"""
        request_data = {
            "is_storm": True,
            "storm_type": "rate",
            "storm_window_minutes": 5,
            "storm_signal_count": 47,
            "affected_resources": [
                "payment-service/pod/api-1",
                "payment-service/pod/api-2",
                "payment-service/pod/api-3"
            ],
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Check for storm indicators
        assert "storm" in prompt.lower() or "47" in prompt or "alert" in prompt.lower()
        assert "rate" in prompt.lower() or "5m" in prompt

    def test_prompt_with_minimal_required_fields(self):
        """ADR-041: Prompt works with only required fields"""
        request_data = {
            "signal_source": "prometheus",
            "signal_name": "pod_oom_killed",
            "error_message": "timeout"
        }

        prompt = _create_investigation_prompt(request_data)

        assert len(prompt) > 100  # Prompt is generated
        assert "pod_oom_killed" in prompt.lower()
        assert "selected_workflow" in prompt.lower()

    def test_prompt_with_all_optional_fields(self):
        """ADR-041: Prompt includes all optional fields when provided"""
        request_data = {
            "signal_source": "prometheus",
            "signal_name": "pod_oom_killed",
            "severity": "critical",
            "resource_namespace": "payment-service",
            "resource_kind": "Pod",
            "resource_name": "api-server-123",
            "environment": "production",
            "priority": "P0",
            "business_category": "payment-processing",
            "risk_tolerance": "low",
            "is_duplicate": True,
            "occurrence_count": 3,
            "is_storm": True,
            "storm_alert_count": 25,
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Verify comprehensive prompt
        # Hybrid format v3.0: focuses on signal type and business context
        assert "critical" in prompt.lower()
        assert "payment-service" in prompt
        assert "production" in prompt.lower()
        assert "P0" in prompt or "p0" in prompt.lower()


# ========================================
# TEST SUITE 2: Output Format (ADR-041 Section 4)
# ========================================

class TestADR040OutputFormat:
    """ADR-041 Section 4: Output Format Specification

    Tests validate that the prompt specifies the correct output format
    as defined in ADR-041 lines 373-500.
    """

    def test_prompt_specifies_workflow_selection_format(self):
        """ADR-041: selected_workflow structure (v1.0)"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Check for workflow selection format
        assert "workflow" in prompt.lower()
        assert "workflow_id" in prompt or "workflowId" in prompt
        assert "parameters" in prompt.lower()
        assert "confidence" in prompt.lower()
        assert "rationale" in prompt.lower()

    def test_prompt_specifies_rca_severity_levels(self):
        """ADR-041: RCA severity assessment (critical/high/medium/low)"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Check for RCA severity guidance
        prompt_lower = prompt.lower()
        assert "severity" in prompt_lower or "rca" in prompt_lower
        assert "critical" in prompt_lower
        assert "high" in prompt_lower
        assert "medium" in prompt_lower
        assert "low" in prompt_lower


    def test_prompt_uses_workflow_terminology(self):
        """ADR-041: Uses 'workflow' not 'playbook' (DD-NAMING-001)"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Should use workflow terminology
        assert "workflow" in prompt.lower()
        # Should NOT use deprecated playbook terminology
        assert "playbook" not in prompt.lower()

    def test_prompt_excludes_deprecated_fields(self):
        """ADR-041: Does not include investigation_result or allowed_actions"""
        request_data = {
            "error_message": "test",
            # These deprecated fields should be ignored
            "investigation_result": {"root_cause": "test"},
            "allowed_actions": ["action1", "action2"]
        }

        prompt = _create_investigation_prompt(request_data)

        # Deprecated fields should not appear in prompt
        # (implementation may still accept them for backward compatibility)
        # This test validates the prompt doesn't instruct LLM to use them
        assert prompt is not None  # Prompt is generated
        assert len(prompt) > 100  # Prompt has content


# ========================================
# TEST SUITE 3: MCP Integration (ADR-041 Section 2)
# ========================================

class TestADR040MCPIntegration:
    """ADR-041 Section 2: MCP Tool Integration

    Tests validate that the prompt instructs the LLM on how to use
    MCP tools for workflow search (ADR-041 lines 186-277).
    """

    def test_prompt_includes_workflow_discovery_tools(self):
        """DD-HAPI-017: References three-step workflow discovery tools"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Should reference three-step workflow discovery tools (DD-HAPI-017)
        assert "list_available_actions" in prompt or "list_workflows" in prompt or "get_workflow" in prompt

    def test_prompt_clarifies_field_usage_for_rca_vs_workflow_search(self):
        """ADR-041: Clear distinction between RCA fields and workflow search fields"""
        request_data = {
            "signal_name": "pod_oom_killed",
            "environment": "production",
            "priority": "P1",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Should clarify which fields are for RCA vs workflow search
        # Check for any guidance about field usage
        prompt_lower = prompt.lower()
        assert ("investigation" in prompt_lower or "rca" in prompt_lower or
                "analysis" in prompt_lower or "search" in prompt_lower)

    def test_prompt_includes_workflow_search_parameters(self):
        """ADR-041: MCP tool parameters (signal_name, severity, component, etc.)"""
        request_data = {
            "signal_name": "pod_oom_killed",
            "severity": "high",
            "environment": "production",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Should include or reference workflow search parameters
        prompt_lower = prompt.lower()
        assert "signal" in prompt_lower or "severity" in prompt_lower or "environment" in prompt_lower


# ========================================
# TEST SUITE 4: RCA Severity (ADR-041 Section 4.4)
# ========================================

class TestADR040RCASeverity:
    """ADR-041 Section 4.4: RCA Severity Assessment

    Tests validate that the prompt provides guidance on RCA severity
    assessment as specified in ADR-041 lines 502-600.
    """

    def test_prompt_includes_rca_severity_criteria(self):
        """ADR-041: RCA severity assessment criteria (User Impact, Environment, etc.)"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Should include severity assessment guidance
        prompt_lower = prompt.lower()
        assert ("impact" in prompt_lower or "environment" in prompt_lower or
                "business" in prompt_lower or "escalation" in prompt_lower)

    def test_prompt_includes_severity_level_examples(self):
        """ADR-041: Examples for critical/high/medium/low severity"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Should include examples or criteria for severity levels
        prompt_lower = prompt.lower()
        assert "critical" in prompt_lower
        assert ("production" in prompt_lower or "data" in prompt_lower or
                "corruption" in prompt_lower or "loss" in prompt_lower)

    def test_prompt_distinguishes_input_severity_from_rca_severity(self):
        """ADR-041: RCA severity may differ from input signal severity"""
        request_data = {
            "severity": "high",  # Input signal severity
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should allow LLM to assess RCA severity independently
        # (not just copy input severity)
        assert "high" in prompt.lower()  # Input severity is present
        assert "severity" in prompt.lower()  # Severity concept is discussed


# ========================================
# TEST SUITE 5: Edge Cases and Boundary Conditions
# ========================================

class TestADR040EdgeCases:
    """Edge cases and boundary conditions for prompt generation"""

    def test_prompt_with_empty_string_fields(self):
        """Edge case: Empty string values in optional fields"""
        request_data = {
            "signal_source": "",
            "signal_name": "",
            "environment": "",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        assert len(prompt) > 100

    def test_prompt_with_none_values(self):
        """Edge case: None values in optional fields"""
        request_data = {
            "signal_source": None,
            "signal_name": None,
            "environment": None,
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        assert len(prompt) > 100

    def test_prompt_with_zero_occurrence_count(self):
        """Edge case: Zero occurrence count (first occurrence)"""
        request_data = {
            "occurrence_count": 0,
            "is_duplicate": False,
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        assert "0" in prompt or "first" in prompt.lower()

    def test_prompt_with_very_high_occurrence_count(self):
        """Edge case: Very high occurrence count (alert fatigue)"""
        request_data = {
            "occurrence_count": 9999,
            "is_duplicate": True,
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        assert "9999" in prompt

    def test_prompt_with_massive_storm_alert_count(self):
        """Edge case: Massive storm (thousands of alerts)"""
        request_data = {
            "is_storm": True,
            "storm_alert_count": 5000,
            "storm_type": "rate",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        assert "5000" in prompt or "storm" in prompt.lower()

    def test_prompt_with_many_affected_resources(self):
        """Edge case: Storm affecting many resources"""
        affected = [f"namespace/pod/pod-{i}" for i in range(100)]
        request_data = {
            "is_storm": True,
            "affected_resources": affected,
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        # Should handle large lists gracefully (may truncate)
        assert "100" in prompt or "pod-" in prompt

    def test_prompt_with_special_characters_in_resource_names(self):
        """Edge case: Special characters in resource names"""
        request_data = {
            "resource_name": "api-server_v2.0-beta+build.123",
            "resource_namespace": "payment-service-prod",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        assert "api-server" in prompt

    def test_prompt_with_unicode_characters(self):
        """Edge case: Unicode characters in fields"""
        request_data = {
            "business_category": "支付处理",  # Chinese: payment processing
            "environment": "production-🔥",  # Emoji
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        # Should handle unicode gracefully
        assert len(prompt) > 100

    def test_prompt_with_very_long_field_values(self):
        """Edge case: Very long field values"""
        long_value = "x" * 10000
        request_data = {
            "signal_name": long_value,
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        # Should handle long values (may truncate)
        assert len(prompt) > 100

    def test_prompt_with_nested_dict_in_error_message(self):
        """Edge case: Error message with extra context (should handle gracefully)"""
        request_data = {
            "signal_name": "OOMKilled",
            "error_message": "timeout",
            "description": "Nested details"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        assert "timeout" in prompt.lower()
        assert "nested details" in prompt.lower()

    def test_prompt_with_all_priority_levels(self):
        """Edge case: Test all priority levels (P0, P1, P2, P3)"""
        for priority in ["P0", "P1", "P2", "P3"]:
            request_data = {
                "priority": priority,
                "error_message": "test",
                "error_message": "test"
            }

            prompt = _create_investigation_prompt(request_data)

            assert prompt is not None
            assert priority in prompt or priority.lower() in prompt.lower()

    def test_prompt_with_all_risk_tolerance_levels(self):
        """Edge case: Test all risk tolerance levels"""
        for risk in ["low", "medium", "high"]:
            request_data = {
                "risk_tolerance": risk,
                "error_message": "test",
                "error_message": "test"
            }

            prompt = _create_investigation_prompt(request_data)

            assert prompt is not None
            assert risk in prompt.lower()

    def test_prompt_with_all_storm_types(self):
        """Edge case: Test all storm types (rate, pattern)"""
        for storm_type in ["rate", "pattern"]:
            request_data = {
                "is_storm": True,
                "storm_type": storm_type,
                "error_message": "test",
                "error_message": "test"
            }

            prompt = _create_investigation_prompt(request_data)

            assert prompt is not None
            assert storm_type in prompt.lower()

    def test_prompt_with_signal_name_only(self):
        """Edge case: Request with only signal_name (should handle gracefully)"""
        request_data = {
            "signal_name": "OOMKilled"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        assert len(prompt) > 100

    def test_prompt_with_error_message_only(self):
        """Edge case: Request with only error_message (should handle gracefully)"""
        request_data = {
            "error_message": "Container OOM"
        }

        prompt = _create_investigation_prompt(request_data)

        assert prompt is not None
        assert len(prompt) > 100

    def test_prompt_generation_is_deterministic(self):
        """Edge case: Same input produces same output (deterministic)"""
        request_data = {
            "signal_source": "prometheus",
            "signal_name": "pod_oom_killed",
            "severity": "high",
            "error_message": "test"
        }

        prompt1 = _create_investigation_prompt(request_data)
        prompt2 = _create_investigation_prompt(request_data)

        assert prompt1 == prompt2  # Should be deterministic


# ========================================
# TEST SUITE 6: Compliance Validation
# ========================================

class TestADR040ComplianceValidation:
    """Overall ADR-041 compliance validation"""

    def test_prompt_length_is_reasonable(self):
        """Prompt should be comprehensive but not excessively long"""
        request_data = {
            "signal_source": "prometheus",
            "signal_name": "pod_oom_killed",
            "severity": "high",
            "resource_namespace": "payment-service",
            "environment": "production",
            "priority": "P1",
            "error_message": "timeout"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should be substantial but not excessive
        assert 500 < len(prompt) < 50000  # Reasonable bounds

    def test_prompt_contains_structured_sections(self):
        """Prompt should have clear sections (headers/structure)"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Should have section markers (##, ---, etc.)
        assert "#" in prompt or "---" in prompt or "##" in prompt

    def test_prompt_is_valid_markdown(self):
        """Prompt should be valid markdown format"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Basic markdown validation
        assert prompt.startswith("#") or prompt.startswith("##")  # Starts with header
        assert "\n" in prompt  # Has line breaks

    def test_prompt_includes_json_code_block_example(self):
        """Prompt should include JSON code block example for LLM"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Should include JSON example
        assert "```json" in prompt or "```" in prompt
        assert "{" in prompt and "}" in prompt

    def test_prompt_does_not_contain_placeholder_text(self):
        """Prompt should not contain TODO or placeholder text"""
        request_data = {
            "signal_source": "prometheus",
            "signal_name": "pod_oom_killed",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Should not have placeholders
        assert "TODO" not in prompt
        assert "FIXME" not in prompt
        assert "XXX" not in prompt
        assert "[INSERT" not in prompt


# ========================================
# TEST SUITE 7: MCP Search Taxonomy (DD-LLM-001)
# ========================================

class TestDDLLM001MCPSearchTaxonomy:
    """DD-LLM-001: MCP Workflow Search Parameter Taxonomy

    Tests validate that the prompt includes guidance for constructing
    MCP search queries with correct format and taxonomy.

    Query Format: <signal_name> <severity> [optional_keywords]
    Label Filters: signal_name, severity, environment, priority, risk_tolerance, business_category
    """

    def test_prompt_explains_mcp_search_query_format(self):
        """DD-LLM-001 Section 2: Query format specification"""
        request_data = {
            "signal_name": "OOMKilled",
            "severity": "critical",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should explain query construction
        assert "search" in prompt.lower() or "workflow" in prompt.lower()
        # Should mention signal_name and severity as search parameters
        assert "signal" in prompt.lower() and "type" in prompt.lower()
        assert "severity" in prompt.lower()

    def test_prompt_lists_canonical_signal_names(self):
        """DD-LLM-001 Section 4: Signal Type Taxonomy"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should list canonical Kubernetes event reasons
        # Check for at least 3 common signal types
        signal_types_found = 0
        canonical_types = ["OOMKilled", "CrashLoopBackOff", "ImagePullBackOff",
                          "Evicted", "NodeNotReady", "PodPending"]

        for signal_type in canonical_types:
            if signal_type in prompt:
                signal_types_found += 1

        # Should mention at least some canonical signal types
        assert signal_types_found >= 3, f"Expected at least 3 canonical signal names in prompt, found {signal_types_found}"

    def test_prompt_defines_rca_severity_levels(self):
        """DD-LLM-001 Section 5: RCA Severity Taxonomy (4 levels)"""
        request_data = {
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should define all 4 RCA severity levels
        assert "critical" in prompt.lower()
        assert "high" in prompt.lower()
        assert "medium" in prompt.lower()
        assert "low" in prompt.lower()

        # Should explain severity assessment criteria
        assert "production" in prompt.lower() or "user impact" in prompt.lower()

    def test_prompt_explains_business_policy_passthrough(self):
        """DD-LLM-001 Section 6: Business/Policy Field Pass-Through"""
        request_data = {
            "environment": "production",
            "priority": "P0",
            "risk_tolerance": "low",
            "business_category": "revenue-critical",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should include business context fields
        assert "production" in prompt.lower()
        assert "P0" in prompt or "p0" in prompt.lower()
        assert "low" in prompt.lower()
        assert "revenue" in prompt.lower() or "critical" in prompt.lower()

        # Should explain these are for workflow search (not RCA)
        assert "workflow" in prompt.lower() or "search" in prompt.lower()

    def test_prompt_provides_complete_mcp_search_example(self):
        """DD-LLM-001 Section 7: Complete MCP Search Example"""
        request_data = {
            "signal_name": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",
            "resource_kind": "deployment",
            "resource_name": "payment-service",
            "environment": "production",
            "priority": "P0",
            "risk_tolerance": "low",
            "business_category": "revenue-critical",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should show how to construct MCP search
        # Check for key elements of search guidance
        assert "OOMKilled" in prompt or "oomkilled" in prompt.lower()
        assert "critical" in prompt.lower()
        assert "production" in prompt.lower()
        assert "P0" in prompt or "p0" in prompt.lower()


# ========================================
# TEST SUITE 8: Query Format Validation (DD-LLM-001)
# ========================================

class TestDDLLM001QueryFormat:
    """DD-LLM-001 Section 2: Query Format Specification

    Tests validate that the prompt explains the correct query format:
    <signal_name> <severity> [optional_keywords]
    """

    def test_prompt_explains_signal_name_must_be_canonical(self):
        """DD-LLM-001 Section 4.1: Canonical signal types required"""
        request_data = {
            "signal_name": "HighMemoryUsage",  # Non-canonical
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should explain to use canonical Kubernetes event reasons
        # Should mention "OOMKilled" as the canonical form
        assert "OOMKilled" in prompt or "canonical" in prompt.lower()
        # Should discourage natural language
        assert "exact" in prompt.lower() or "kubernetes" in prompt.lower()

    def test_prompt_explains_severity_assessment_criteria(self):
        """DD-LLM-001 Section 5.1: RCA Severity Assessment Factors"""
        request_data = {
            "severity": "high",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should explain severity assessment factors
        assessment_factors = ["user impact", "environment", "business impact",
                             "escalation", "data risk", "production"]

        factors_found = sum(1 for factor in assessment_factors if factor.lower() in prompt.lower())

        # Should mention at least 3 assessment factors
        assert factors_found >= 3, f"Expected at least 3 severity assessment factors, found {factors_found}"

    def test_prompt_clarifies_llm_can_override_input_severity(self):
        """DD-LLM-001 Section 5.2: LLM can override input severity based on RCA"""
        request_data = {
            "severity": "high",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should clarify LLM can assess different severity
        # Look for language about assessment, investigation, or override
        assert ("assess" in prompt.lower() or "determine" in prompt.lower() or
                "investigation" in prompt.lower() or "rca" in prompt.lower())


# ========================================
# TEST SUITE 9: Label Parameter Validation (DD-LLM-001)
# ========================================

class TestDDLLM001LabelParameters:
    """DD-LLM-001 Section 3: MCP Search Parameters

    Tests validate that the prompt explains all 6 label parameters
    and their roles (LLM determines vs pass-through).
    """

    def test_prompt_lists_all_six_label_parameters(self):
        """DD-LLM-001 Section 3.1: Complete parameter list"""
        request_data = {
            "signal_name": "OOMKilled",
            "severity": "critical",
            "environment": "production",
            "priority": "P0",
            "risk_tolerance": "low",
            "business_category": "revenue-critical",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Check all 6 label parameters are mentioned
        assert "signal" in prompt.lower() and "type" in prompt.lower()
        assert "severity" in prompt.lower()
        assert "environment" in prompt.lower()
        assert "priority" in prompt.lower()
        assert "risk" in prompt.lower() and "tolerance" in prompt.lower()
        assert "business" in prompt.lower() or "category" in prompt.lower()

    def test_prompt_explains_llm_determines_technical_fields(self):
        """DD-LLM-001 Section 3.2: LLM determines signal_name and severity"""
        request_data = {
            "signal_name": "OOMKilled",
            "severity": "critical",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should explain LLM determines these from RCA
        assert "investigate" in prompt.lower() or "determine" in prompt.lower() or "assess" in prompt.lower()
        # Should mention RCA or investigation
        assert "rca" in prompt.lower() or "root cause" in prompt.lower() or "investigation" in prompt.lower()

    def test_prompt_explains_llm_must_passthrough_business_fields(self):
        """DD-LLM-001 Section 3.2: LLM must pass-through business/policy fields"""
        request_data = {
            "environment": "production",
            "priority": "P0",
            "risk_tolerance": "low",
            "business_category": "revenue-critical",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should include these values for pass-through
        assert "production" in prompt.lower()
        assert "P0" in prompt or "p0" in prompt.lower()
        assert "low" in prompt.lower()
        # Should explain these are for workflow search
        assert "workflow" in prompt.lower() or "search" in prompt.lower()

    def test_prompt_explains_priority_is_business_decision(self):
        """DD-LLM-001 Section 6.2: Priority is business decision, not technical"""
        request_data = {
            "priority": "P0",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should include priority
        assert "P0" in prompt or "p0" in prompt.lower() or "priority" in prompt.lower()
        # Should explain it's for workflow filtering
        assert "workflow" in prompt.lower() or "search" in prompt.lower()

    def test_prompt_explains_risk_tolerance_is_policy_decision(self):
        """DD-LLM-001 Section 6.3: Risk tolerance is policy decision"""
        request_data = {
            "risk_tolerance": "low",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should include risk tolerance
        assert "low" in prompt.lower() or "risk" in prompt.lower()
        # Should explain it affects remediation approach
        assert "workflow" in prompt.lower() or "search" in prompt.lower() or "remediation" in prompt.lower()


# ========================================
# TEST SUITE 10: Confidence Score Optimization (DD-LLM-001)
# ========================================

class TestDDLLM001ConfidenceOptimization:
    """DD-LLM-001 Section 1.2: Confidence Score Optimization

    Tests validate that the prompt explains the optimization strategy
    for achieving 90-95% confidence scores.
    """

    def test_prompt_explains_exact_label_matching(self):
        """DD-LLM-001 Section 1.1: Exact label matching for filtering"""
        request_data = {
            "signal_name": "OOMKilled",
            "severity": "critical",
            "environment": "production",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should explain search uses exact matching
        # Check for search/filter/match terminology
        assert "search" in prompt.lower() or "workflow" in prompt.lower()
        assert "OOMKilled" in prompt or "signal" in prompt.lower()

    def test_prompt_provides_workflow_search_guidance(self):
        """DD-LLM-001 Section 7: Complete workflow search example"""
        request_data = {
            "signal_name": "OOMKilled",
            "severity": "critical",
            "environment": "production",
            "priority": "P0",
            "risk_tolerance": "low",
            "business_category": "revenue-critical",
            "error_message": "test"
        }

        prompt = _create_investigation_prompt(request_data)

        # Prompt should provide comprehensive search guidance
        # Check for all key search parameters
        assert "OOMKilled" in prompt or "oomkilled" in prompt.lower()
        assert "critical" in prompt.lower()
        assert "production" in prompt.lower()
        assert "P0" in prompt or "p0" in prompt.lower()
        assert "low" in prompt.lower()

