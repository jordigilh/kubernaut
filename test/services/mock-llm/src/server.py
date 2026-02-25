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
Mock LLM Server for Integration and E2E Tests

Provides a mock OpenAI-compatible LLM endpoint that returns predictable responses,
including tool calls for testing the full HolmesGPT workflow.

Features:
- Deterministic responses for E2E testing
- Tool call support (three-step discovery + legacy search_workflow_catalog)
- Multi-turn conversation handling
- Configurable scenarios for different test cases
- Validation of tool call format

Usage:
    # In pytest fixture
    with MockLLMServer() as server:
        os.environ["LLM_ENDPOINT"] = server.url
        os.environ["LLM_MODEL"] = "mock-model"
        # Run tests...

Architecture:
    Legacy two-phase flow (search_workflow_catalog):
      1. Initial request → Return tool call (search_workflow_catalog)
      2. Tool result received → Return final analysis with selected workflow

    DD-HAPI-017 three-step discovery flow:
      1. Initial request → Return list_available_actions tool call
      2. After step 1 result → Return list_workflows tool call
      3. After step 2 result → Return get_workflow tool call
      4. After step 3 result → Return final analysis with selected workflow
"""

import json
import os
import threading
import time
import uuid
from http.server import ThreadingHTTPServer, BaseHTTPRequestHandler
from typing import Optional, Dict, Any, List
from dataclasses import dataclass, field
from urllib.parse import urlparse, parse_qs
import logging

logger = logging.getLogger(__name__)


# ========================================
# MOCK SCENARIOS (Deterministic Responses)
# ========================================

@dataclass
class MockScenario:
    """Configuration for a mock LLM scenario."""
    name: str
    signal_name: str
    severity: str = "critical"
    workflow_name: str = ""  # Workflow name for UUID mapping (e.g., "oomkill-increase-memory-v1")
    workflow_id: str = "mock-workflow-v1"
    workflow_title: str = "Mock Workflow"
    confidence: float = 0.92
    root_cause: str = "Mock root cause analysis"
    rca_resource_kind: str = "Pod"
    rca_resource_namespace: str = "default"
    rca_resource_name: str = "test-pod"
    rca_resource_api_version: str = "v1"  # BR-HAPI-212: API version for GVK resolution
    include_affected_resource: bool = True  # BR-HAPI-212: Whether to include affectedResource in RCA
    parameters: Dict[str, str] = field(default_factory=dict)
    execution_engine: str = "tekton"  # BR-WE-014: Execution backend ("tekton" or "job")


# Pre-defined scenarios for common test cases
MOCK_SCENARIOS: Dict[str, MockScenario] = {
    "oomkilled": MockScenario(
        name="oomkilled",
        workflow_name="oomkill-increase-memory-v1",  # MUST match test_workflows.go WorkflowID exactly
        signal_name="OOMKilled",
        severity="critical",
        workflow_id="21053597-2865-572b-89bf-de49b5b685da",  # Placeholder - overwritten by config file
        workflow_title="OOMKill Recovery - Increase Memory Limits",
        confidence=0.95,
        root_cause="Container exceeded memory limits due to traffic spike",
        # BR-HAPI-191: RCA identifies Deployment as the affected resource (not the Pod)
        # The WFE creator will use this to set TargetResource to the Deployment
        rca_resource_kind="Deployment",
        rca_resource_namespace="production",
        rca_resource_name="api-server",
        # BR-HAPI-191: Parameter names MUST match workflow-schema.yaml definitions
        # (validated by HAPI WorkflowResponseValidator against DataStorage parameter schema)
        # Schema: NAMESPACE (required), DEPLOYMENT_NAME (required), MEMORY_INCREASE_PERCENT (optional)
        parameters={"NAMESPACE": "default", "DEPLOYMENT_NAME": "memory-eater", "MEMORY_INCREASE_PERCENT": "50"},
        # BR-WE-014: Full pipeline uses Job execution engine (not Tekton)
        execution_engine="job",
    ),
    "crashloop": MockScenario(
        name="crashloop",
        workflow_name="crashloop-config-fix-v1",  # MUST match test_workflows.go WorkflowID exactly
        signal_name="CrashLoopBackOff",
        severity="high",
        workflow_id="42b90a37-0d1b-5561-911a-2939ed9e1c30",  # Placeholder - overwritten by config file
        workflow_title="CrashLoopBackOff - Configuration Fix",
        confidence=0.88,
        root_cause="Container failing due to missing configuration",
        # BR-HAPI-191: RCA identifies Deployment as the affected resource (not the Pod)
        rca_resource_kind="Deployment",
        rca_resource_namespace="staging",
        rca_resource_name="worker",
        # BR-HAPI-191: Parameter names MUST match workflow-schema.yaml definitions
        # Schema: NAMESPACE (required), DEPLOYMENT_NAME (required), GRACE_PERIOD_SECONDS (optional)
        parameters={"NAMESPACE": "staging", "DEPLOYMENT_NAME": "worker"}
    ),
    "node_not_ready": MockScenario(
        name="node_not_ready",
        workflow_name="node-drain-reboot-v1",  # MUST match test_workflows.go WorkflowID exactly
        signal_name="NodeNotReady",
        severity="critical",
        workflow_id="node-drain-reboot-v1",  # Placeholder - overwritten by config file
        workflow_title="NodeNotReady - Drain and Reboot",
        confidence=0.90,
        root_cause="Node experiencing disk pressure",
        rca_resource_kind="Node",
        rca_resource_namespace="",  # Cluster-scoped
        rca_resource_name="worker-node-1",
        # BR-HAPI-191: Parameter names MUST match workflow-schema.yaml definitions
        # Schema: NODE_NAME (required), DRAIN_TIMEOUT_SECONDS (optional)
        parameters={"NODE_NAME": "worker-node-1"}
    ),
    "test_signal": MockScenario(
        name="test_signal",
        workflow_name="test-signal-handler-v1",
        signal_name="TestSignal",
        severity="critical",
        workflow_id="2faf3306-1d6c-5d2f-9e9f-2e1a4844ca70",  # DD-WORKFLOW-002 v3.0: Deterministic UUID for test-signal-handler-v1
        workflow_title="Test Signal Handler",
        confidence=0.90,
        root_cause="Test signal for graceful shutdown validation",
        rca_resource_kind="Pod",
        rca_resource_namespace="test",
        rca_resource_name="test-pod",
        # BR-HAPI-191: Parameter names MUST match workflow-schema.yaml definitions
        # Schema: NAMESPACE (required), POD_NAME (required)
        parameters={"NAMESPACE": "test", "POD_NAME": "test-pod"}
    ),
    "no_workflow_found": MockScenario(
        name="no_workflow_found",
        signal_name="MOCK_NO_WORKFLOW_FOUND",
        severity="critical",
        workflow_id="",  # Empty workflow_id indicates no workflow found
        workflow_title="",
        confidence=0.0,  # Zero confidence triggers human review
        root_cause="No suitable workflow found in catalog for this signal type",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="failing-pod",
        parameters={}
    ),
    "low_confidence": MockScenario(
        name="low_confidence",
        workflow_name="generic-restart-v1",
        signal_name="MOCK_LOW_CONFIDENCE",
        severity="critical",
        workflow_id="d2c84d90-55ba-5ae1-b48d-6cc16e0edb5c",  # DD-WORKFLOW-002 v3.0: Deterministic UUID for generic-restart-v1
        workflow_title="Generic Pod Restart",
        confidence=0.35,  # Low confidence (<0.5) triggers human review
        root_cause="Multiple possible root causes identified, requires human judgment",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="ambiguous-pod",
        # BR-HAPI-191: Parameter names MUST match workflow-schema.yaml definitions
        # Schema: NAMESPACE (required), POD_NAME (required)
        parameters={"NAMESPACE": "production", "POD_NAME": "ambiguous-pod"}
    ),
    "problem_resolved": MockScenario(
        name="problem_resolved",
        workflow_name="",  # No workflow needed - problem self-resolved
        signal_name="MOCK_PROBLEM_RESOLVED",
        severity="low",  # DD-SEVERITY-001 v1.1: Use normalized severity (critical/high/medium/low/unknown)
        workflow_id="",  # Empty workflow_id indicates no workflow needed
        workflow_title="",
        confidence=0.85,  # High confidence (>= 0.7) that problem is resolved
        root_cause="Problem self-resolved through auto-scaling or transient condition cleared",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="recovered-pod",
        parameters={}
    ),
    # E2E-HAPI-003: Max retries exhausted - LLM parsing failed
    "max_retries_exhausted": MockScenario(
        name="max_retries_exhausted",
        workflow_name="",  # No workflow - parsing failed
        signal_name="MOCK_MAX_RETRIES_EXHAUSTED",
        severity="high",
        workflow_id="",  # Empty workflow_id - couldn't parse/select workflow
        workflow_title="",
        confidence=0.0,  # Zero confidence indicates parsing failure
        root_cause="LLM analysis completed but failed validation after maximum retry attempts. Response format was unparseable or contained invalid data.",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="failed-analysis-pod",
        parameters={}
    ),
    "rca_incomplete": MockScenario(
        name="rca_incomplete",
        workflow_name="generic-restart-v1",
        signal_name="MOCK_RCA_INCOMPLETE",
        severity="critical",
        workflow_id="d2c84d90-55ba-5ae1-b48d-6cc16e0edb5c",  # DD-WORKFLOW-002 v3.0: Deterministic UUID for generic-restart-v1
        workflow_title="Generic Pod Restart",
        confidence=0.88,  # High confidence (>= 0.7) but incomplete RCA
        root_cause="Root cause identified but affected resource could not be determined from signal context",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="ambiguous-pod",
        rca_resource_api_version="v1",
        include_affected_resource=False,  # BR-HAPI-212: Trigger missing affectedResource scenario
        # BR-HAPI-191: Parameter names MUST match workflow-schema.yaml definitions
        # Schema: NAMESPACE (required), POD_NAME (required)
        parameters={"NAMESPACE": "production", "POD_NAME": "ambiguous-pod"}
    ),
    # ========================================
    # BR-AI-084 / ADR-054: Predictive Signal Mode Scenarios
    # ========================================
    "oomkilled_predictive": MockScenario(
        name="oomkilled_predictive",
        workflow_name="oomkill-increase-memory-v1",  # Same workflow catalog entry as reactive (SP normalizes signal type)
        signal_name="OOMKilled",  # Already normalized by SP from PredictedOOMKill
        severity="critical",
        workflow_id="21053597-2865-572b-89bf-de49b5b685da",  # Same as reactive oomkilled (same workflow)
        workflow_title="OOMKill Recovery - Increase Memory Limits",
        confidence=0.88,
        root_cause="Predicted OOMKill based on memory utilization trend analysis (predict_linear). Current memory usage is 85% of limit and growing at 50MB/min. Preemptive action recommended to increase memory limits before the predicted OOMKill event occurs.",
        # BR-HAPI-191: RCA identifies Deployment as the affected resource
        rca_resource_kind="Deployment",
        rca_resource_namespace="production",
        rca_resource_name="api-server",
        # BR-HAPI-191: Parameter names MUST match workflow-schema.yaml definitions
        # Schema: NAMESPACE (required), DEPLOYMENT_NAME (required), MEMORY_INCREASE_PERCENT (optional)
        parameters={"NAMESPACE": "production", "DEPLOYMENT_NAME": "api-server", "MEMORY_INCREASE_PERCENT": "50"},
        # BR-WE-014: Full pipeline uses Job execution engine (not Tekton)
        execution_engine="job",
    ),
    "predictive_no_action": MockScenario(
        name="predictive_no_action",
        workflow_name="",  # No workflow needed - prediction unlikely to materialize
        signal_name="OOMKilled",  # Normalized signal type
        severity="medium",
        workflow_id="",  # Empty workflow_id - no action needed
        workflow_title="",
        confidence=0.82,
        root_cause="Predicted OOMKill based on trend analysis, but current assessment shows the trend is reversing. Memory usage has stabilized at 60% of limit after recent deployment rollout. No preemptive action needed — the prediction is unlikely to materialize.",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="api-server-def456",
        parameters={}
    ),
}

# Default scenario if none matches
DEFAULT_SCENARIO = MockScenario(
    name="default",
    workflow_name="generic-restart-v1",  # Set workflow_name so UUID gets loaded from config
    signal_name="Unknown",
    severity="medium",
    workflow_id="placeholder-uuid-default",  # Placeholder - overwritten by config file
    workflow_title="Generic Pod Restart",
    confidence=0.75,
    root_cause="Unable to determine specific root cause",
    # BR-HAPI-191: Parameter names MUST match workflow-schema.yaml definitions
    # Schema: NAMESPACE (required), POD_NAME (required)
    parameters={"NAMESPACE": "default", "POD_NAME": "unknown-pod"}
)

# ========================================
# FILE-BASED CONFIGURATION (DD-TEST-011 v2.0)
# ========================================
# 📋 Design Decision: DD-TEST-011 v2.0 | ✅ Approved Pattern | Confidence: 95%
# See: docs/architecture/decisions/DD-TEST-011-mock-llm-self-discovery-pattern.md
#
# Mock LLM reads workflow UUIDs from YAML configuration file at startup.
# File can be:
#   - Direct path (integration tests, local dev)
#   - ConfigMap mount (E2E tests in Kubernetes)
# This eliminates timing issues and network dependencies.
# ========================================

def load_scenarios_from_file(config_path: str):
    """
    Load workflow UUIDs from YAML configuration file at startup.

    DD-TEST-011 v2.0: File-Based Configuration Pattern
    - Test suite writes configuration file with workflow UUIDs
    - Mock LLM reads file at startup (no HTTP calls, no network)
    - Deterministic ordering, environment-agnostic
    - In E2E: File is mounted via Kubernetes ConfigMap
    - In Integration: File is written directly to filesystem

    Args:
        config_path: Path to configuration file (e.g., /config/scenarios.yaml)

    Returns:
        bool: True if loaded successfully, False otherwise

    File Format (YAML):
        scenarios:
          oomkill-increase-memory-v1:production: "uuid1"
          crashloop-config-fix-v1:test: "uuid2"
        overrides:                          # Optional: per-scenario field overrides
          crashloop:
            execution_engine: "job"         # Override execution_engine for this scenario
          oomkilled:
            execution_engine: "job"
    """
    try:
        import yaml

        with open(config_path, 'r') as f:
            config = yaml.safe_load(f)

        if not config or 'scenarios' not in config:
            print(f"⚠️  Configuration file exists but has no 'scenarios' key")
            return False

        scenarios_config = config['scenarios']
        synced_count = 0

        # Update MOCK_SCENARIOS with UUIDs from configuration file
        # Format: "workflow_name:environment" → "uuid"
        # Example: {"oomkill-increase-memory-v1:production": "uuid1", "test-signal-handler-v1:test": "uuid2"}
        for workflow_key, workflow_uuid in scenarios_config.items():
            # workflow_key format: "workflow_name:environment"
            parts = workflow_key.split(':')
            if len(parts) != 2:
                print(f"  ⚠️  Invalid workflow key format: {workflow_key} (expected workflow_name:environment)")
                continue

            workflow_name_from_config = parts[0]
            env_from_config = parts[1]

            # Match against MOCK_SCENARIOS by workflow_name (environment-agnostic)
            # DD-TEST-011 v2.2: Match workflows for all environments (staging/production/test)
            # Tests may use different environments but expect same Mock LLM scenarios
            matched = False
            
            # Check MOCK_SCENARIOS
            for scenario_name, scenario in MOCK_SCENARIOS.items():
                if not scenario.workflow_name:
                    continue  # Skip scenarios without workflow_name

                # Match if workflow names match (ignore environment - use workflow from any environment)
                if scenario.workflow_name == workflow_name_from_config:
                    scenario.workflow_id = workflow_uuid
                    synced_count += 1
                    print(f"  ✅ Loaded {scenario_name} ({workflow_name_from_config}:{env_from_config}) → {workflow_uuid}")
                    matched = True
                    # DON'T BREAK - multiple scenarios may share same workflow_name (e.g., low_confidence + DEFAULT_SCENARIO)
            
            # Also check DEFAULT_SCENARIO (not in MOCK_SCENARIOS dict)
            # Check this ALWAYS, not just when matched=False, because multiple scenarios can share same workflow
            if DEFAULT_SCENARIO.workflow_name == workflow_name_from_config:
                DEFAULT_SCENARIO.workflow_id = workflow_uuid
                synced_count += 1  # Count DEFAULT_SCENARIO update for validation
                print(f"  ✅ Loaded default ({workflow_name_from_config}:{env_from_config}) → {workflow_uuid}")
                matched = True
            
            if not matched:
                # No match found for this config entry
                print(f"  ⚠️  No matching scenario for config entry: {workflow_key}")

        print(f"✅ Mock LLM loaded {synced_count}/{len(MOCK_SCENARIOS)} scenarios from file")

        # Apply scenario-level overrides (e.g., execution_engine)
        # Format: overrides:
        #           crashloop:
        #             execution_engine: "job"
        overrides_config = config.get('overrides', {})
        if overrides_config:
            for scenario_name, overrides in overrides_config.items():
                if scenario_name in MOCK_SCENARIOS:
                    scenario = MOCK_SCENARIOS[scenario_name]
                    for key, value in overrides.items():
                        if hasattr(scenario, key):
                            setattr(scenario, key, value)
                            print(f"  ✅ Override {scenario_name}.{key} = {value}")
                        else:
                            print(f"  ⚠️  Unknown override field: {scenario_name}.{key}")
                elif scenario_name == "default" and DEFAULT_SCENARIO:
                    for key, value in overrides.items():
                        if hasattr(DEFAULT_SCENARIO, key):
                            setattr(DEFAULT_SCENARIO, key, value)
                            print(f"  ✅ Override default.{key} = {value}")
                else:
                    print(f"  ⚠️  Unknown scenario for override: {scenario_name}")

        # Validate that all scenarios with workflow_name matched successfully
        # This prevents silent failures from workflow name drift between Mock LLM and test fixtures
        # Check both MOCK_SCENARIOS and DEFAULT_SCENARIO
        expected_scenarios_with_workflows = len([s for s in MOCK_SCENARIOS.values() if s.workflow_name])
        if DEFAULT_SCENARIO.workflow_name:
            expected_scenarios_with_workflows += 1
        
        if synced_count < expected_scenarios_with_workflows:
            missing = expected_scenarios_with_workflows - synced_count
            # Find scenarios that have workflow_name but still have placeholder UUIDs
            unsynced_scenarios = [name for name, s in MOCK_SCENARIOS.items() 
                                if s.workflow_name and s.workflow_id.startswith("placeholder-")]
            # Also check DEFAULT_SCENARIO
            if DEFAULT_SCENARIO.workflow_name and DEFAULT_SCENARIO.workflow_id.startswith("placeholder-"):
                unsynced_scenarios.append("DEFAULT_SCENARIO")
            
            error_msg = (
                f"\n❌ Mock LLM configuration error: {missing}/{expected_scenarios_with_workflows} scenarios failed to load UUIDs\n"
                f"   Unsynced scenarios: {unsynced_scenarios}\n"
                f"   This indicates Mock LLM scenario workflow_name doesn't match DataStorage workflow catalog\n"
                f"   Check that scenario workflow_name in server.py matches test workflow fixtures in:\n"
                f"     - test/integration/holmesgptapi/test_workflows.go\n"
                f"     - test/integration/aianalysis/test_workflows.go\n"
                f"\n   Integration tests will FAIL with 'workflow_not_found' errors until this is fixed.\n"
            )
            print(error_msg)
            raise RuntimeError(error_msg)
        
        print(f"✅ Mock LLM validation: All {expected_scenarios_with_workflows} scenarios synced successfully")
        return True

    except FileNotFoundError:
        print(f"❌ Configuration file not found: {config_path}")
        return False
    except yaml.YAMLError as e:
        print(f"❌ Error parsing YAML configuration: {e}")
        return False
    except Exception as e:
        print(f"❌ Error loading configuration file: {e}")
        return False


# ========================================
# TOOL CALL TRACKING
# ========================================

@dataclass
class ToolCallRecord:
    """Record of a tool call for validation."""
    tool_name: str
    arguments: Dict[str, Any]
    call_id: str
    timestamp: str = ""


class ToolCallTracker:
    """Tracks tool calls for test validation."""

    def __init__(self):
        self.calls: List[ToolCallRecord] = []
        self._lock = threading.Lock()

    def record(self, tool_name: str, arguments: Dict[str, Any], call_id: str):
        """Record a tool call."""
        with self._lock:
            self.calls.append(ToolCallRecord(
                tool_name=tool_name,
                arguments=arguments,
                call_id=call_id
            ))

    def get_calls(self, tool_name: Optional[str] = None) -> List[ToolCallRecord]:
        """Get recorded tool calls, optionally filtered by name."""
        with self._lock:
            if tool_name:
                return [c for c in self.calls if c.tool_name == tool_name]
            return list(self.calls)

    def clear(self):
        """Clear all recorded calls."""
        with self._lock:
            self.calls.clear()

    def assert_called(self, tool_name: str) -> ToolCallRecord:
        """Assert a tool was called and return the record."""
        calls = self.get_calls(tool_name)
        if not calls:
            raise AssertionError(f"Tool '{tool_name}' was never called. Recorded calls: {[c.tool_name for c in self.calls]}")
        return calls[-1]  # Return most recent

    def assert_called_with(self, tool_name: str, **expected_args):
        """Assert a tool was called with specific arguments."""
        call = self.assert_called(tool_name)
        for key, expected in expected_args.items():
            actual = call.arguments.get(key)
            if actual != expected:
                raise AssertionError(
                    f"Tool '{tool_name}' called with {key}={actual}, expected {expected}"
                )


# Global tracker instance (can be accessed by tests)
tool_call_tracker = ToolCallTracker()


# ========================================
# MOCK LLM REQUEST HANDLER
# ========================================

class MockLLMRequestHandler(BaseHTTPRequestHandler):
    """HTTP request handler that mimics OpenAI API with tool call support."""

    # Class-level configuration
    current_scenario: MockScenario = DEFAULT_SCENARIO
    force_text_response: bool = False  # For backward compatibility with integration tests

    def log_message(self, format, *args):
        """Enable HTTP request logging for debugging."""
        logger.info(format % args)

    def do_POST(self):
        """Handle POST requests (chat completions)."""
        # EXPLICIT LOGGING: Track all POST requests using logger (not print)
        logger.info(f"📥 Mock LLM received POST {self.path} from {self.client_address[0]}")
        
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length).decode('utf-8')

        try:
            request_data = json.loads(body) if body else {}
        except json.JSONDecodeError:
            request_data = {}

        # Route based on endpoint
        # IT-AA-095-02: Detect mock_rca_permanent_error scenario and return HTTP 500
        # This causes HAPI session to fail, triggering AA controller to move to Failed phase
        messages = request_data.get("messages", [])
        all_content = " ".join(str(m.get("content", "")) for m in messages if m.get("content")).lower()
        if "mock_rca_permanent_error" in all_content or "mock rca permanent error" in all_content:
            logger.info("🚨 MOCK_RCA_PERMANENT_ERROR detected - returning HTTP 500")
            self.send_response(500)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            error_response = json.dumps({
                "error": {
                    "message": "Mock permanent LLM error for testing",
                    "type": "server_error",
                    "code": "internal_error"
                }
            })
            self.wfile.write(error_response.encode("utf-8"))
            return

        if self.path in ["/v1/chat/completions", "/chat/completions"]:
            logger.info(f"  → Handling OpenAI chat completion request")
            response = self._handle_openai_request(request_data)
        elif self.path == "/api/generate" or self.path == "/api/chat":
            logger.info(f"  → Handling Ollama request")
            response = self._handle_ollama_request(request_data)
        else:
            logger.info(f"  → Unknown path {self.path}, returning OK")
            response = {"status": "ok", "path": self.path}

        self._send_json_response(response)
        logger.info(f"✅ Mock LLM sent response for {self.path}")

    def do_GET(self):
        """Handle GET requests (health checks, model list)."""
        if self.path == "/api/tags" or self.path == "/v1/models":
            response = {"models": [{"name": "mock-model", "size": 1000000}]}
        elif self.path == "/health" or self.path.startswith("/health"):
            # Health check always returns OK (file-based config loads at startup)
            response = {"status": "ok"}
        else:
            response = {"status": "ok"}
        self._send_json_response(response)


    def _handle_openai_request(self, request_data: Dict[str, Any]) -> Dict[str, Any]:
        """Generate OpenAI-compatible response with tool call support."""
        messages = request_data.get("messages", [])
        tools = request_data.get("tools", [])

        # Detect scenario from prompt content
        scenario = self._detect_scenario(messages)

        # Check if this is a tool result (multi-turn)
        has_tool_result = self._has_tool_result(messages)

        # Determine response type
        if self.force_text_response or not tools:
            # Backward compatibility: return text response
            return self._text_response(scenario, request_data)

        # DD-HAPI-017 + ADR-056 v1.4: Discovery protocol with optional resource context
        # When get_resource_context is available, run it first (4-step flow);
        # otherwise fall back to the original 3-step flow.
        if self._has_three_step_tools(tools):
            has_rc = self._has_resource_context_tool(tools)
            total_steps = 4 if has_rc else 3
            tool_result_count = self._count_tool_results(messages)
            logger.info(
                f"🔍 Discovery protocol ({total_steps}-step): "
                f"{tool_result_count} tool results so far"
            )
            if tool_result_count >= total_steps:
                return self._final_analysis_response(scenario, request_data)
            else:
                return self._discovery_tool_call_response(
                    scenario, request_data, tool_result_count, has_rc
                )

        # Legacy two-phase flow (search_workflow_catalog)
        if has_tool_result:
            # Phase 2: After tool result → return final analysis
            return self._final_analysis_response(scenario, request_data)
        else:
            # Phase 1: Initial request → return tool call
            return self._tool_call_response(scenario, request_data)

    def _detect_scenario(self, messages: List[Dict[str, Any]]) -> MockScenario:
        """Detect which scenario to use based on message content."""
        # Combine all message content for analysis (including stringified objects)
        content = " ".join(
            str(m.get("content", ""))
            for m in messages
            if m.get("content")
        ).lower()

        # Check all messages for tool_calls that might contain signal data
        all_text = " ".join(str(m) for m in messages).lower()

        # DEBUG: Log scenario detection
        logger.info(f"🔍 SCENARIO DETECTION - Content preview: {content[:200]}...")

        # Check for test-specific signal types first (human review tests)
        # FIX: Check BOTH 'content' (message text) AND 'all_text' (full message objects)
        # Prompts may embed the signal_name in structured content blocks or
        # JSON payloads that 'content' extraction doesn't capture reliably.
        # 'all_text' captures everything including {"signal_name": "MOCK_..."} in JSON.
        search_text = content + " " + all_text
        if "mock_no_workflow_found" in search_text or "mock no workflow found" in search_text:
            logger.info("✅ SCENARIO DETECTED: NO_WORKFLOW_FOUND (mock keyword match)")
            return MOCK_SCENARIOS.get("no_workflow_found", DEFAULT_SCENARIO)
        if "mock_low_confidence" in search_text or "mock low confidence" in search_text:
            logger.info("✅ SCENARIO DETECTED: LOW_CONFIDENCE (mock keyword match)")
            return MOCK_SCENARIOS.get("low_confidence", DEFAULT_SCENARIO)
        if "mock_problem_resolved" in search_text or "mock problem resolved" in search_text:
            logger.info("✅ SCENARIO DETECTED: PROBLEM_RESOLVED (mock keyword match)")
            return MOCK_SCENARIOS.get("problem_resolved", DEFAULT_SCENARIO)
        if "mock_not_reproducible" in search_text or "mock not reproducible" in search_text:
            logger.info("✅ SCENARIO DETECTED: NOT_REPRODUCIBLE (mock keyword match)")
            return MOCK_SCENARIOS.get("problem_resolved", DEFAULT_SCENARIO)  # Same scenario: issue self-resolved
        if "mock_rca_incomplete" in search_text or "mock rca incomplete" in search_text:
            logger.info("✅ SCENARIO DETECTED: RCA_INCOMPLETE (mock keyword match)")
            return MOCK_SCENARIOS.get("rca_incomplete", DEFAULT_SCENARIO)
        # E2E-HAPI-003: Max retries exhausted - LLM parsing failed
        if "mock_max_retries_exhausted" in search_text or "mock max retries exhausted" in search_text:
            logger.info("✅ SCENARIO DETECTED: MAX_RETRIES_EXHAUSTED (mock keyword match)")
            return MOCK_SCENARIOS.get("max_retries_exhausted", DEFAULT_SCENARIO)

        # Check for test signal (graceful shutdown tests)
        if "testsignal" in content or "test signal" in content:
            return MOCK_SCENARIOS.get("test_signal", DEFAULT_SCENARIO)

        # BR-AI-084 / ADR-054: Detect predictive signal mode
        # Predictive mode is indicated by "predictive" keyword in the prompt content,
        # specifically from the signal_mode field passed through the investigation prompt.
        is_predictive = ("predictive mode" in content or "predictive signal" in content or
                         "predicted" in content and "not yet occurred" in content)

        # Check for predictive-specific scenarios first
        if is_predictive:
            # Check for "no action" predictive scenario
            if "predictive_no_action" in content or "mock_predictive_no_action" in content:
                logger.info("✅ SCENARIO DETECTED: PREDICTIVE_NO_ACTION")
                return MOCK_SCENARIOS.get("predictive_no_action", DEFAULT_SCENARIO)
            # Default predictive scenario with OOMKilled
            if "oomkilled" in content:
                logger.info("✅ SCENARIO DETECTED: OOMKILLED_PREDICTIVE")
                return MOCK_SCENARIOS.get("oomkilled_predictive", DEFAULT_SCENARIO)

        # Check for signal types FIRST (most specific first to avoid false matches)
        # DD-TEST-010: Match exact signal types, not generic substrings
        # FIX: Move signal type checks BEFORE Category F scenarios to avoid incorrect matches
        # "crashloop" is more specific than "oom", check it first
        if "crashloop" in content:
            matched_scenario = MOCK_SCENARIOS.get("crashloop", DEFAULT_SCENARIO)
            logger.info(f"✅ PHASE 2: Matched 'crashloop' → scenario={matched_scenario.name}, workflow_id={matched_scenario.workflow_id}")
            return matched_scenario
        elif "oomkilled" in content:
            matched_scenario = MOCK_SCENARIOS.get("oomkilled", DEFAULT_SCENARIO)
            logger.info(f"✅ PHASE 2: Matched 'oomkilled' → scenario={matched_scenario.name}, workflow_id={matched_scenario.workflow_id}")
            return matched_scenario
        elif "memoryexceedslimit" in content or "memory exceeds limit" in content or "memoryexceeds" in content:
            # AlertManager E2E test: MemoryExceedsLimit Prometheus alert → same oomkilled workflow
            matched_scenario = MOCK_SCENARIOS.get("oomkilled", DEFAULT_SCENARIO)
            logger.info(f"✅ PHASE 2: Matched 'memoryexceedslimit' → scenario={matched_scenario.name}, workflow_id={matched_scenario.workflow_id}")
            return matched_scenario
        elif "nodenotready" in content or "node not ready" in content:
            matched_scenario = MOCK_SCENARIOS.get("node_not_ready", DEFAULT_SCENARIO)
            logger.info(f"✅ PHASE 2: Matched 'nodenotready' → scenario={matched_scenario.name}, workflow_id={matched_scenario.workflow_id}")
            return matched_scenario

        # PHASE 2: Fallback to current_scenario or DEFAULT_SCENARIO
        fallback_scenario = MockLLMRequestHandler.current_scenario
        logger.warning(f"⚠️  PHASE 2: NO MATCH - Falling back to current_scenario={fallback_scenario.name}, workflow_id={fallback_scenario.workflow_id}")
        logger.warning(f"⚠️  PHASE 2: Content preview for debugging: {content[:500]}")
        return fallback_scenario

    def _has_tool_result(self, messages: List[Dict[str, Any]]) -> bool:
        """Check if messages contain a tool result."""
        for msg in messages:
            if msg.get("role") == "tool":
                return True
            # Also check for tool_call_id in content (some formats)
            if "tool_call_id" in str(msg):
                return True
        return False

    def _count_tool_results(self, messages: List[Dict[str, Any]]) -> int:
        """
        Count the number of tool result messages in the conversation.

        DD-HAPI-017 + ADR-056 v1.4: Used by the discovery protocol to
        determine which step we're on.

        4-step flow (with get_resource_context):
          0 tool results → get_resource_context (ADR-056)
          1 tool result  → list_available_actions
          2 tool results → list_workflows
          3 tool results → get_workflow
          4+ tool results → final analysis

        3-step flow (without get_resource_context):
          0 tool results → list_available_actions
          1 tool result  → list_workflows
          2 tool results → get_workflow
          3+ tool results → final analysis
        """
        count = 0
        for msg in messages:
            if msg.get("role") == "tool":
                count += 1
        return count

    def _has_three_step_tools(self, tools: List[Dict[str, Any]]) -> bool:
        """
        Check if the tools list includes three-step discovery tools.

        DD-HAPI-017: When HAPI registers the three-step toolset, the tools
        list will include 'list_available_actions'. If it only has
        'search_workflow_catalog', we use the legacy two-phase flow.
        """
        for tool in tools:
            func = tool.get("function", {})
            if func.get("name") == "list_available_actions":
                return True
        return False

    def _has_resource_context_tool(self, tools: List[Dict[str, Any]]) -> bool:
        """
        Check if the tools list includes get_resource_context.

        ADR-056 v1.4: When HAPI registers the resource context toolset,
        the tools list will include 'get_resource_context'. The mock must
        call it before workflow discovery so that HAPI populates
        session_state["detected_labels"].
        """
        for tool in tools:
            func = tool.get("function", {})
            if func.get("name") == "get_resource_context":
                return True
        return False

    def _extract_resource_from_messages(
        self, messages: List[Dict[str, Any]], scenario: "MockScenario"
    ) -> tuple:
        """
        Extract resource identity (kind, name, namespace) from prompt messages.

        ADR-056 v1.4: The mock needs the actual resource coordinates to pass
        to get_resource_context. HAPI's prompt_builder formats the resource as:
          "Resource: {namespace}/{kind}/{name}"
        If not found, fall back to scenario defaults.

        Returns:
            (kind, name, namespace) tuple
        """
        import re
        for msg in messages:
            content = str(msg.get("content", ""))
            # Match "Resource: namespace/Kind/name" from prompt_builder.py line 343
            # Use [a-zA-Z0-9._-]+ to only match valid K8s name characters and prevent
            # capturing trailing \n- from double-encoded JSON or prompt formatting.
            match = re.search(r"Resource:\s*([a-zA-Z0-9._-]+)/([a-zA-Z0-9._-]+)/([a-zA-Z0-9._-]+)", content)
            if match:
                namespace = match.group(1).strip()
                kind = match.group(2).strip()
                name = match.group(3).strip()
                logger.info(
                    f"📍 ADR-056: Extracted resource from prompt: "
                    f"{kind}/{name} in {namespace}"
                )
                return (kind, name, namespace)
        logger.info(
            f"📍 ADR-056: Resource not found in prompt, using scenario defaults: "
            f"{scenario.rca_resource_kind}/{scenario.rca_resource_name} "
            f"in {scenario.rca_resource_namespace}"
        )
        return (
            scenario.rca_resource_kind,
            scenario.rca_resource_name,
            scenario.rca_resource_namespace,
        )

    def _discovery_tool_call_response(
        self,
        scenario: MockScenario,
        request_data: Dict[str, Any],
        step: int,
        has_resource_context: bool,
    ) -> Dict[str, Any]:
        """
        Generate the next tool call in the discovery sequence.

        ADR-056 v1.4 + DD-HAPI-017: When has_resource_context is True, use a
        4-step flow that calls get_resource_context first so HAPI populates
        session_state["detected_labels"]:

          Step 0: get_resource_context  (ADR-056 v1.4)
          Step 1: list_available_actions
          Step 2: list_workflows
          Step 3: get_workflow

        When has_resource_context is False, use the original 3-step flow:

          Step 0: list_available_actions
          Step 1: list_workflows
          Step 2: get_workflow

        Args:
            scenario: The detected MockScenario
            request_data: Original request data
            step: Current step index (tool result count)
            has_resource_context: Whether get_resource_context is available
        """
        call_id = f"call_{uuid.uuid4().hex[:12]}"
        messages = request_data.get("messages", [])

        # Normalize step to the 3-step offset when resource context is present
        effective_step = step - 1 if has_resource_context else step

        if has_resource_context and step == 0:
            # ADR-056 v1.4: Call get_resource_context first
            kind, name, namespace = self._extract_resource_from_messages(
                messages, scenario
            )
            tool_name = "get_resource_context"
            tool_args = {"kind": kind, "name": name, "namespace": namespace}
            logger.info(
                f"🔧 Discovery Step 0 (ADR-056): {tool_name}"
                f"({kind}/{name} in {namespace})"
            )

        elif effective_step == 0:
            tool_name = "list_available_actions"
            tool_args = {"limit": 100}
            logger.info(f"🔧 Discovery Step {step}: {tool_name}")

        elif effective_step == 1:
            action_type = self._scenario_to_action_type(scenario)
            tool_name = "list_workflows"
            tool_args = {"action_type": action_type, "limit": 10}
            logger.info(f"🔧 Discovery Step {step}: {tool_name} (action_type={action_type})")

        else:
            tool_name = "get_workflow"
            tool_args = {"workflow_id": scenario.workflow_id}
            logger.info(f"🔧 Discovery Step {step}: {tool_name} (workflow_id={scenario.workflow_id})")

        # Record for test validation
        tool_call_tracker.record(tool_name, tool_args, call_id)

        return {
            "id": f"chatcmpl-{uuid.uuid4().hex[:8]}",
            "object": "chat.completion",
            "created": 1701388800,
            "model": request_data.get("model", "mock-model"),
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": None,
                        "tool_calls": [
                            {
                                "id": call_id,
                                "type": "function",
                                "function": {
                                    "name": tool_name,
                                    "arguments": json.dumps(tool_args)
                                }
                            }
                        ]
                    },
                    "finish_reason": "tool_calls"
                }
            ],
            "usage": {
                "prompt_tokens": 500,
                "completion_tokens": 50,
                "total_tokens": 550
            }
        }

    @staticmethod
    def _scenario_to_action_type(scenario: MockScenario) -> str:
        """
        Map a MockScenario to its action_type for three-step discovery.

        DD-WORKFLOW-016: Action types must match values seeded in
        action_type_taxonomy by migration 025.
        """
        # Map by workflow_name (matches Go/Python fixture action_type assignments)
        action_type_map = {
            "oomkill-increase-memory-v1": "IncreaseMemoryLimits",
            "crashloop-config-fix-v1": "RestartDeployment",
            "node-drain-reboot-v1": "RestartPod",
            "image-pull-backoff-fix-credentials": "RollbackDeployment",
            "generic-restart-v1": "RestartPod",
            "test-signal-handler-v1": "RestartPod",
        }
        action_type = action_type_map.get(scenario.workflow_name, "ScaleReplicas")
        return action_type

    def _tool_call_response(self, scenario: MockScenario, request_data: Dict[str, Any]) -> Dict[str, Any]:
        """Generate a response with tool call (Phase 1)."""
        call_id = f"call_{uuid.uuid4().hex[:12]}"

        # Build tool call arguments
        tool_args = {
            "query": f"{scenario.signal_name} {scenario.severity}",
            "rca_resource": {
                "signal_name": scenario.signal_name,
                "kind": scenario.rca_resource_kind,
                "namespace": scenario.rca_resource_namespace,
                "name": scenario.rca_resource_name
            }
        }

        # Record for test validation
        tool_call_tracker.record("search_workflow_catalog", tool_args, call_id)

        return {
            "id": f"chatcmpl-{uuid.uuid4().hex[:8]}",
            "object": "chat.completion",
            "created": 1701388800,
            "model": request_data.get("model", "mock-model"),
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": None,
                        "tool_calls": [
                            {
                                "id": call_id,
                                "type": "function",
                                "function": {
                                    "name": "search_workflow_catalog",
                                    "arguments": json.dumps(tool_args)
                                }
                            }
                        ]
                    },
                    "finish_reason": "tool_calls"
                }
            ],
            "usage": {
                "prompt_tokens": 500,
                "completion_tokens": 50,
                "total_tokens": 550
            }
        }

    def _final_analysis_response(self, scenario: MockScenario, request_data: Dict[str, Any]) -> Dict[str, Any]:
        """Generate final analysis response after tool result (Phase 2)."""
        messages = request_data.get("messages", [])

        # Build the analysis content
        analysis_json = {
            "root_cause_analysis": {
                "summary": scenario.root_cause,
                "severity": scenario.severity,
                "signal_name": scenario.signal_name,
                "contributing_factors": ["identified_by_mock_llm"] if scenario.workflow_id else []
            },
            # E2E-HAPI-002: Always include confidence field in response
            "confidence": scenario.confidence
        }

        # BR-HAPI-212: Conditionally include affectedResource in RCA
        # This allows testing scenarios where affectedResource is missing despite workflow being selected
        if scenario.include_affected_resource:
            # ADR-056 / FP-E2E: Extract actual resource from the HAPI prompt so
            # the RCA response matches the real workload (namespace, name) instead
            # of the scenario's hardcoded defaults. This is critical for FP E2E
            # where the EM queries Prometheus using the namespace from the RCA.
            actual_kind, actual_name, actual_ns = self._extract_resource_from_messages(
                messages, scenario
            )
            affected_resource = {
                "kind": actual_kind,
                "name": actual_name,
            }
            # Add apiVersion if present (BR-HAPI-212: Optional field for GVK resolution)
            if scenario.rca_resource_api_version:
                affected_resource["apiVersion"] = scenario.rca_resource_api_version
            # Add namespace if present (not applicable for cluster-scoped resources)
            if actual_ns:
                affected_resource["namespace"] = actual_ns

            analysis_json["root_cause_analysis"]["affectedResource"] = affected_resource

        # BR-HAPI-200: Handle problem resolved case (investigation_outcome: "resolved")
        if scenario.name == "problem_resolved":
            analysis_json["selected_workflow"] = None
            analysis_json["investigation_outcome"] = "resolved"  # BR-HAPI-200: Signal problem self-resolved
            # Note: confidence already set at line 841
            analysis_json["can_recover"] = False  # E2E-HAPI-023: No recovery needed when problem self-resolved
            content = f"""Based on my investigation of the {scenario.signal_name} signal:

## Root Cause Analysis

{scenario.root_cause}

## Investigation Outcome

The problem has self-resolved. No remediation workflow is needed.

```json
{json.dumps(analysis_json, indent=2)}
```
"""
        # E2E-HAPI-002: Handle low confidence case with alternative workflows
        elif scenario.name == "low_confidence":
            # Primary workflow with low confidence
            analysis_json["selected_workflow"] = {
                "workflow_id": scenario.workflow_id,
                "title": scenario.workflow_title,
                "version": "1.0.0",
                "confidence": scenario.confidence,  # 0.35 - triggers human review
                "rationale": "Multiple possible causes identified, confidence is low",
                "execution_engine": scenario.execution_engine,  # BR-WE-014
                "parameters": scenario.parameters
            }
            # E2E-HAPI-002: Add alternative workflows for human review
            alternatives_list = [
                {
                    "workflow_id": "d3c95ea1-66cb-6bf2-c59e-7dd27f1fec6d",  # Mock alternative 1
                    "title": "Alternative Diagnostic Workflow",
                    "confidence": 0.28,
                    "rationale": "Alternative approach for ambiguous root cause"
                },
                {
                    "workflow_id": "e4d06fb2-77dc-7cg3-d60f-8ee38g2gfd7e",  # Mock alternative 2
                    "title": "Manual Investigation Required",
                    "confidence": 0.22,
                    "rationale": "Requires human expertise to determine correct remediation"
                }
            ]
            analysis_json["alternative_workflows"] = alternatives_list
            # BR-HAPI-197: HAPI does NOT enforce confidence thresholds — that's AIAnalysis's job.
            # Explicitly set needs_human_review=false so HAPI's parser doesn't infer true.
            analysis_json["needs_human_review"] = False
            analysis_json["human_review_reason"] = None
            content = f"""Based on my investigation of the {scenario.signal_name} signal:

# root_cause_analysis
{json.dumps(analysis_json["root_cause_analysis"])}

# confidence
{scenario.confidence}

# selected_workflow
{json.dumps(analysis_json["selected_workflow"])}

# alternative_workflows
{json.dumps(alternatives_list)}

# needs_human_review
false

# human_review_reason
null
"""
        # Handle no workflow found case
        elif not scenario.workflow_id:
            analysis_json["selected_workflow"] = None
            # Note: confidence already set at line 841
            
            # E2E-HAPI-001: Set needs_human_review for incident with no workflow
            analysis_json["needs_human_review"] = True
            analysis_json["human_review_reason"] = "no_matching_workflows"
            
            # E2E-HAPI-003: Set human_review fields for max retries exhausted (incident)
            if scenario.name == "max_retries_exhausted":
                analysis_json["needs_human_review"] = True
                analysis_json["human_review_reason"] = "llm_parsing_error"
                if "validation_attempts_history" not in analysis_json:
                    # E2E-HAPI-003: Match Pydantic ValidationAttempt model structure
                    from datetime import datetime, timezone
                    base_time = datetime.now(timezone.utc)
                    analysis_json["validation_attempts_history"] = [
                        {
                            "attempt": 1,
                            "workflow_id": None,
                            "is_valid": False,
                            "errors": ["Invalid JSON structure"],
                            "timestamp": base_time.isoformat().replace("+00:00", "Z")
                        },
                        {
                            "attempt": 2,
                            "workflow_id": None,
                            "is_valid": False,
                            "errors": ["Missing required field"],
                            "timestamp": base_time.isoformat().replace("+00:00", "Z")
                        },
                        {
                            "attempt": 3,
                            "workflow_id": None,
                            "is_valid": False,
                            "errors": ["Schema validation failed"],
                            "timestamp": base_time.isoformat().replace("+00:00", "Z")
                        }
                    ]
            
            # E2E-HAPI-003: Use section header format for SDK compatibility
            content = f"""Based on my investigation of the {scenario.signal_name} signal:

# root_cause_analysis
{json.dumps(analysis_json["root_cause_analysis"])}

# confidence
{analysis_json.get("confidence", 0.0)}

# selected_workflow
{json.dumps(analysis_json.get("selected_workflow"))}

# needs_human_review
{json.dumps(analysis_json.get("needs_human_review", False))}

# human_review_reason
{json.dumps(analysis_json.get("human_review_reason", ""))}

# validation_attempts_history
{json.dumps(analysis_json.get("validation_attempts_history", []))}
"""
        else:
            analysis_json["selected_workflow"] = {
                "workflow_id": scenario.workflow_id,
                "title": scenario.workflow_title,
                "version": "1.0.0",
                "confidence": scenario.confidence,
                "rationale": f"Selected based on {scenario.signal_name} signal analysis",
                "execution_engine": scenario.execution_engine,  # BR-WE-014
                "parameters": scenario.parameters
            }
            # BR-HAPI-197: Explicitly set needs_human_review=false for valid workflow selections.
            # Without this, HAPI's parser may infer needs_human_review=true from missing field.
            analysis_json["needs_human_review"] = False
            analysis_json["human_review_reason"] = None
            # Format as markdown with JSON block (like real LLM would)
            content = f"""Based on my investigation of the {scenario.signal_name} signal:

## Root Cause Analysis

{scenario.root_cause}

## Recommended Workflow

I've identified a suitable remediation workflow from the catalog.

```json
{json.dumps(analysis_json, indent=2)}
```
"""

        # DEBUG: Log what we're returning
        logger.info(f"📤 FINAL RESPONSE - Scenario: {scenario.name}, analysis_json keys: {list(analysis_json.keys())}")

        return {
            "id": f"chatcmpl-{uuid.uuid4().hex[:8]}",
            "object": "chat.completion",
            "created": 1701388800,
            "model": request_data.get("model", "mock-model"),
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": content
                    },
                    "finish_reason": "stop"
                }
            ],
            "usage": {
                "prompt_tokens": 800,
                "completion_tokens": 200,
                "total_tokens": 1000
            }
        }

    def _text_response(self, scenario: MockScenario, request_data: Dict[str, Any]) -> Dict[str, Any]:
        """Generate text-only response (backward compatibility)."""
        content = self._incident_text_response(scenario)

        return {
            "id": f"chatcmpl-{uuid.uuid4().hex[:8]}",
            "object": "chat.completion",
            "created": 1701388800,
            "model": request_data.get("model", "mock-model"),
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": content
                    },
                    "finish_reason": "stop"
                }
            ],
            "usage": {
                "prompt_tokens": 100,
                "completion_tokens": 50,
                "total_tokens": 150
            }
        }

    def _incident_text_response(self, scenario: MockScenario) -> str:
        """Generate incident analysis text response."""
        # Handle no workflow found case
        if not scenario.workflow_id:
            return f"""Based on my investigation of the incident:

## Root Cause Analysis

{scenario.root_cause}

```json
{{
  "root_cause_analysis": {{
    "summary": "{scenario.root_cause}",
    "severity": "{scenario.severity}",
    "contributing_factors": []
  }},
  "selected_workflow": null
}}
```
"""

        return f"""Based on my investigation of the incident:

## Root Cause Analysis

{scenario.root_cause}

```json
{{
  "root_cause_analysis": {{
    "summary": "{scenario.root_cause}",
    "severity": "{scenario.severity}",
    "contributing_factors": ["traffic_spike", "resource_limits"]
  }},
  "selected_workflow": {{
    "workflow_id": "{scenario.workflow_id}",
    "version": "1.0.0",
    "confidence": {scenario.confidence},
    "rationale": "Selected based on signal analysis",
    "execution_engine": "{scenario.execution_engine}"
  }}
}}
```
"""

    def _handle_ollama_request(self, request_data: Dict[str, Any]) -> Dict[str, Any]:
        """Generate Ollama-compatible response."""
        messages = request_data.get("messages", [])
        scenario = self._detect_scenario(messages)

        return {
            "model": request_data.get("model", "mock-model"),
            "created_at": "2025-11-30T00:00:00Z",
            "response": self._incident_text_response(scenario),
            "done": True,
            "context": [],
            "total_duration": 1000000000,
            "load_duration": 100000000,
            "prompt_eval_count": 100,
            "eval_count": 50
        }

    def _send_json_response(self, response: Dict[str, Any], status_code: int = 200):
        """Send JSON response with appropriate headers."""
        response_body = json.dumps(response).encode('utf-8')

        self.send_response(status_code)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', len(response_body))
        self.end_headers()
        self.wfile.write(response_body)


# ========================================
# MOCK LLM SERVER
# ========================================

class MockLLMServer:
    """
    Context manager for running a mock LLM server.

    Usage:
        # Basic usage
        with MockLLMServer(port=11434) as server:
            print(f"Mock LLM running at {server.url}")
            # Run tests...

        # With custom scenario
        with MockLLMServer() as server:
            server.set_scenario("oomkilled")
            # Run OOMKilled-specific tests...

        # Force text responses (no tool calls)
        with MockLLMServer(force_text_response=True) as server:
            # Backward compatible tests...
    """

    def __init__(
        self,
        host: str = "127.0.0.1",
        port: int = 0,
        force_text_response: bool = False
    ):
        """
        Initialize mock LLM server.

        Args:
            host: Host to bind to (default: 127.0.0.1)
            port: Port to bind to (default: 0 = auto-assign)
            force_text_response: If True, always return text (no tool calls)
        """
        self.host = host
        self.port = port
        self.force_text_response = force_text_response
        self.server: Optional[ThreadingHTTPServer] = None
        self.thread: Optional[threading.Thread] = None

    def __enter__(self) -> "MockLLMServer":
        """Start the mock server."""
        self.start()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Stop the mock server."""
        self.stop()

    def start(self):
        """Start the mock LLM server in a background thread."""
        # Configure handler
        MockLLMRequestHandler.force_text_response = self.force_text_response
        MockLLMRequestHandler.current_scenario = DEFAULT_SCENARIO

        # Clear tool call tracker
        tool_call_tracker.clear()

        self.server = ThreadingHTTPServer((self.host, self.port), MockLLMRequestHandler)

        # Get the actual port if auto-assigned
        if self.port == 0:
            self.port = self.server.server_address[1]

        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

        logger.info(f"Mock LLM server started at {self.url}")

    def stop(self):
        """Stop the mock LLM server."""
        if self.server:
            self.server.shutdown()
            self.server.server_close()
            logger.info("Mock LLM server stopped")

    @property
    def url(self) -> str:
        """Get the server URL."""
        return f"http://{self.host}:{self.port}"

    def set_scenario(self, scenario_name: str):
        """
        Set the current scenario for responses.

        Args:
            scenario_name: One of "oomkilled", "crashloop", "node_not_ready"
        """
        if scenario_name in MOCK_SCENARIOS:
            MockLLMRequestHandler.current_scenario = MOCK_SCENARIOS[scenario_name]
        else:
            logger.warning(f"Unknown scenario '{scenario_name}', using default")
            MockLLMRequestHandler.current_scenario = DEFAULT_SCENARIO

    def get_tool_calls(self, tool_name: Optional[str] = None) -> List[ToolCallRecord]:
        """Get recorded tool calls for validation."""
        return tool_call_tracker.get_calls(tool_name)

    def assert_tool_called(self, tool_name: str) -> ToolCallRecord:
        """Assert a tool was called."""
        return tool_call_tracker.assert_called(tool_name)

    def assert_tool_called_with(self, tool_name: str, **expected_args):
        """Assert a tool was called with specific arguments."""
        tool_call_tracker.assert_called_with(tool_name, **expected_args)

    def clear_tool_calls(self):
        """Clear recorded tool calls."""
        tool_call_tracker.clear()


# ========================================
# PYTEST FIXTURES
# ========================================

def create_mock_llm_fixture(force_text_response: bool = False):
    """
    Create a pytest fixture for the mock LLM server.

    Usage in conftest.py:
        from tests.mock_llm_server import create_mock_llm_fixture
        mock_llm = create_mock_llm_fixture()

        # For E2E with tool calls:
        mock_llm_e2e = create_mock_llm_fixture(force_text_response=False)
    """
    import pytest

    @pytest.fixture(scope="session")
    def mock_llm_server():
        """Session-scoped mock LLM server."""
        with MockLLMServer(force_text_response=force_text_response) as server:
            yield server

    return mock_llm_server


# For quick testing
def start_server(host="0.0.0.0", port=8080):
    """
    Start the Mock LLM server using the existing MockLLMServer class.

    This is a simple wrapper that keeps the server running until interrupted.
    """
    print(f"Starting Mock LLM server on {host}:{port}...")
    with MockLLMServer(host=host, port=port) as server:  # ← FIX: Pass host parameter!
        print(f"✅ Mock LLM running at http://{host}:{port}")
        print("Press Ctrl+C to stop")
        try:
            import time
            while True:
                time.sleep(1)
        except KeyboardInterrupt:
            print("\nStopping Mock LLM server...")


if __name__ == "__main__":
    # NOTE: When running via Dockerfile (python -m src), __main__.py is the entrypoint
    # This block only executes when running server.py directly: python src/server.py
    print("🚀 Mock LLM v2.0 - Self-Discovery Pattern (DD-TEST-011)")
    print("   Running via direct server.py execution")
    print("=" * 70)

    # DD-TEST-011: Sync workflows from DataStorage BEFORE serving traffic
    sync_enabled = os.getenv("SYNC_ON_STARTUP", "true").lower() == "true"

    if sync_enabled:
        sync_workflows_from_datastorage()
    else:
        print("ℹ️  Self-discovery disabled (SYNC_ON_STARTUP=false)")
        print("   Using default scenario UUIDs")

    print("=" * 70)

    # Start HTTP server
    host = os.getenv("MOCK_LLM_HOST", "0.0.0.0")
    port = int(os.getenv("MOCK_LLM_PORT", "8080"))
    start_server(host=host, port=port)
