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


from src.extensions.recovery import _create_investigation_prompt


class TestFivePhaseWorkflowBehavior:
    """
    Tests validating the 5-phase investigation workflow guides LLM correctly.
    
    Business Requirements:
    - BR-WORKFLOW-001: LLM must investigate before selecting workflows
    - BR-AI-001: RCA must be based on investigation findings, not input assumptions
    """
    
    def test_workflow_enforces_investigation_first_sequence(self):
        """
        BEHAVIOR: Prompt must explicitly state investigation comes BEFORE workflow search.
        WHY: Prevents LLM from searching workflows before understanding root cause.
        """
        request_data = {
            "signal_type": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",
            "resource_kind": "deployment",
            "resource_name": "payment-service",
            "signal_source": "prometheus-adapter",
            "failure_context": {"error_message": "Container exceeded memory limit"},
            "failed_action": {"type": "restart", "target": "pod"}
        }
        
        prompt = _create_investigation_prompt(request_data)
        
        # BEHAVIOR VALIDATION: Prompt must contain explicit sequence warning
        assert "CRITICAL" in prompt, "Missing critical warning about sequence"
        assert "Follow this sequence in order" in prompt, "Missing explicit sequence instruction"
        assert "Do NOT search for workflows before investigating" in prompt,             "Missing explicit prohibition of premature workflow search"
        
        # BEHAVIOR VALIDATION: Investigation section must appear before workflow discovery section
        investigation_idx = prompt.find("Phase 1: Investigate")
        workflow_discovery_idx = prompt.find("Phase 4: Discover and Select Workflow")
        assert investigation_idx > 0, "Missing Phase 1: Investigate section"
        assert workflow_discovery_idx > 0, "Missing Phase 4: Discover and Select Workflow section"
        assert investigation_idx < workflow_discovery_idx, \
            "Investigation phase must appear BEFORE workflow discovery phase"
    
    def test_workflow_clarifies_input_signal_is_starting_point(self):
        """
        BEHAVIOR: Prompt must clarify input signal is for investigation, not workflow search.
        WHY: Prevents LLM from directly using input signal_type for MCP search.
        """
        request_data = {
            "signal_type": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",
            "resource_kind": "deployment",
            "resource_name": "payment-service",
            "signal_source": "prometheus-adapter",
            "failure_context": {"error_message": "Container exceeded memory limit"},
            "failed_action": {"type": "restart", "target": "pod"}
        }
        
        prompt = _create_investigation_prompt(request_data)
        
        # BEHAVIOR VALIDATION: Input signal must be labeled as starting point
        assert "Input Signal Provided" in prompt, "Missing input signal label"
        assert "starting point for investigation" in prompt,             "Missing clarification that input signal is for investigation"
        
        # BEHAVIOR VALIDATION: Must show input signal value
        assert "OOMKilled" in prompt, "Input signal value not shown"
    
    def test_workflow_provides_signal_type_decision_criteria(self):
        """
        BEHAVIOR: Prompt must provide clear criteria for when to use same vs different signal_type.
        WHY: LLM needs explicit guidance on signal_type determination logic.
        """
        request_data = {
            "signal_type": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",
            "resource_kind": "deployment",
            "resource_name": "payment-service",
            "signal_source": "prometheus-adapter",
            "failure_context": {"error_message": "Container exceeded memory limit"},
            "failed_action": {"type": "restart", "target": "pod"}
        }
        
        prompt = _create_investigation_prompt(request_data)
        
        # BEHAVIOR VALIDATION: Must explain when to use same signal_type
        assert "If investigation confirms input signal is the root cause" in prompt,             "Missing criteria for using same signal_type"
        
        # BEHAVIOR VALIDATION: Must explain when to use different signal_type
        assert "If investigation reveals different root cause" in prompt,             "Missing criteria for using different signal_type"
        
        # BEHAVIOR VALIDATION: Must provide concrete examples
        assert "Investigation confirms memory limit exceeded" in prompt,             "Missing example of confirming input signal"
        assert "Investigation shows node memory pressure" in prompt,             "Missing example of different root cause"
    
    def test_workflow_emphasizes_rca_determines_signal_type(self):
        """
        BEHAVIOR: Prompt must explicitly state signal_type comes from RCA, not input.
        WHY: Critical to prevent LLM from bypassing investigation.
        """
        request_data = {
            "signal_type": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",
            "resource_kind": "deployment",
            "resource_name": "payment-service",
            "signal_source": "prometheus-adapter",
            "failure_context": {"error_message": "Container exceeded memory limit"},
            "failed_action": {"type": "restart", "target": "pod"}
        }
        
        prompt = _create_investigation_prompt(request_data)
        
        # BEHAVIOR VALIDATION: Must explicitly state signal_type source
        assert "signal_type for workflow search comes from YOUR investigation findings" in prompt,             "Missing explicit statement that signal_type comes from investigation"
        assert "not the input signal" in prompt,             "Missing clarification that input signal is not used directly"
    
    def test_workflow_defines_all_five_phases(self):
        """
        BEHAVIOR: Prompt must define all 5 phases in correct order.
        WHY: Complete workflow ensures LLM follows entire process.
        """
        request_data = {
            "signal_type": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",
            "resource_kind": "deployment",
            "resource_name": "payment-service",
            "signal_source": "prometheus-adapter",
            "failure_context": {"error_message": "Container exceeded memory limit"},
            "failed_action": {"type": "restart", "target": "pod"}
        }
        
        prompt = _create_investigation_prompt(request_data)
        
        # BEHAVIOR VALIDATION: All 5 phases must be present (DD-HAPI-017: Phase 4 is now three-step discovery)
        assert "Phase 1: Investigate the Incident" in prompt, "Missing Phase 1"
        assert "Phase 2: Determine Root Cause" in prompt, "Missing Phase 2"
        assert "Phase 3: Identify Signal Type" in prompt, "Missing Phase 3"
        assert "Phase 4: Discover and Select Workflow" in prompt, "Missing Phase 4"
        assert "Phase 5: Return Summary" in prompt, "Missing Phase 5"
        
        # BEHAVIOR VALIDATION: Phases must appear in correct order
        phase1_idx = prompt.find("Phase 1:")
        phase2_idx = prompt.find("Phase 2:")
        phase3_idx = prompt.find("Phase 3:")
        phase4_idx = prompt.find("Phase 4:")
        phase5_idx = prompt.find("Phase 5:")
        
        assert phase1_idx < phase2_idx < phase3_idx < phase4_idx < phase5_idx,             "Phases must appear in sequential order 1→2→3→4→5"
    
    def test_workflow_includes_mcp_failure_handling(self):
        """
        BEHAVIOR: Prompt must instruct LLM how to handle MCP search failures.
        WHY: Graceful degradation ensures RCA is still useful even if workflow selection fails.
        """
        request_data = {
            "signal_type": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",
            "resource_kind": "deployment",
            "resource_name": "payment-service",
            "signal_source": "prometheus-adapter",
            "failure_context": {"error_message": "Container exceeded memory limit"},
            "failed_action": {"type": "restart", "target": "pod"}
        }
        
        prompt = _create_investigation_prompt(request_data)
        
        # BEHAVIOR VALIDATION: Must provide workflow discovery failure handling instructions (DD-HAPI-017)
        assert '"selected_workflow": null' in prompt, \
            "Missing instruction to return null workflow on discovery failure"
        assert "discovery" in prompt.lower() and "fail" in prompt.lower(), \
            "Missing workflow discovery failure scenario"
    
    def test_workflow_specifies_rca_signal_type_in_mcp_search(self):
        """
        BEHAVIOR: Prompt must specify MCP search uses RCA signal_type, not input.
        WHY: Ensures workflow search is based on investigation findings.
        """
        request_data = {
            "signal_type": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",
            "resource_kind": "deployment",
            "resource_name": "payment-service",
            "signal_source": "prometheus-adapter",
            "failure_context": {"error_message": "Container exceeded memory limit"},
            "failed_action": {"type": "restart", "target": "pod"}
        }
        
        prompt = _create_investigation_prompt(request_data)
        
        # BEHAVIOR VALIDATION: Workflow discovery must be based on RCA findings (DD-HAPI-017)
        assert "RCA" in prompt or "rca" in prompt.lower() or "root cause" in prompt.lower(), \
            "Missing instruction to use RCA findings for workflow discovery"
        assert "list_available_actions" in prompt or "list_workflows" in prompt, \
            "Missing three-step discovery tool references"
    
    def test_workflow_requires_investigation_tools_usage(self):
        """
        BEHAVIOR: Prompt must instruct LLM to use investigation tools.
        WHY: Investigation requires actual tool usage, not assumptions.
        """
        request_data = {
            "signal_type": "OOMKilled",
            "severity": "critical",
            "resource_namespace": "production",
            "resource_kind": "deployment",
            "resource_name": "payment-service",
            "signal_source": "prometheus-adapter",
            "failure_context": {"error_message": "Container exceeded memory limit"},
            "failed_action": {"type": "restart", "target": "pod"}
        }
        
        prompt = _create_investigation_prompt(request_data)
        
        # BEHAVIOR VALIDATION: Must mention investigation tools
        assert "kubectl" in prompt.lower(), "Missing kubectl tool reference"
        assert "logs" in prompt.lower(), "Missing logs investigation"
        assert "Use available tools" in prompt or "investigation tools" in prompt.lower(),             "Missing instruction to use investigation tools"
