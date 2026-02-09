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
- Tool call support (search_workflow_catalog, etc.)
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
    1. Initial request → Return tool call (search_workflow_catalog)
    2. Tool result received → Return final analysis with selected workflow
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
    signal_type: str
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
        signal_type="OOMKilled",
        severity="critical",
        workflow_id="21053597-2865-572b-89bf-de49b5b685da",  # Placeholder - overwritten by config file
        workflow_title="OOMKill Recovery - Increase Memory Limits",
        confidence=0.95,
        root_cause="Container exceeded memory limits due to traffic spike",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="api-server-abc123",
        parameters={"MEMORY_LIMIT": "1Gi", "NAMESPACE": "production"}
    ),
    "crashloop": MockScenario(
        name="crashloop",
        workflow_name="crashloop-config-fix-v1",  # MUST match test_workflows.go WorkflowID exactly
        signal_type="CrashLoopBackOff",
        severity="high",
        workflow_id="42b90a37-0d1b-5561-911a-2939ed9e1c30",  # Placeholder - overwritten by config file
        workflow_title="CrashLoopBackOff - Configuration Fix",
        confidence=0.88,
        root_cause="Container failing due to missing configuration",
        rca_resource_kind="Pod",
        rca_resource_namespace="staging",
        rca_resource_name="worker-xyz789",
        parameters={"CONFIG_MAP": "app-config", "NAMESPACE": "staging"}
    ),
    "node_not_ready": MockScenario(
        name="node_not_ready",
        workflow_name="node-drain-reboot-v1",  # MUST match test_workflows.go WorkflowID exactly
        signal_type="NodeNotReady",
        severity="critical",
        workflow_id="node-drain-reboot-v1",  # Placeholder - overwritten by config file
        workflow_title="NodeNotReady - Drain and Reboot",
        confidence=0.90,
        root_cause="Node experiencing disk pressure",
        rca_resource_kind="Node",
        rca_resource_namespace="",  # Cluster-scoped
        rca_resource_name="worker-node-1",
        parameters={"NODE_NAME": "worker-node-1", "GRACE_PERIOD": "300"}
    ),
    "recovery": MockScenario(
        name="recovery",
        workflow_name="memory-optimize-v1",  # MUST match test_workflows.go WorkflowID exactly (alternative workflow)
        signal_type="OOMKilled",
        severity="critical",
        workflow_id="99f4a9b8-d6b5-5191-85a4-93e5dbf61321",  # Placeholder - overwritten by config file
        workflow_title="Memory Optimization - Alternative Approach",
        confidence=0.85,
        root_cause="Previous scaling approach failed, requires optimization",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="api-server-abc123",
        parameters={"OPTIMIZATION_LEVEL": "aggressive", "MEMORY_TARGET": "512Mi"}
    ),
    "test_signal": MockScenario(
        name="test_signal",
        workflow_name="test-signal-handler-v1",
        signal_type="TestSignal",
        severity="critical",
        workflow_id="2faf3306-1d6c-5d2f-9e9f-2e1a4844ca70",  # DD-WORKFLOW-002 v3.0: Deterministic UUID for test-signal-handler-v1
        workflow_title="Test Signal Handler",
        confidence=0.90,
        root_cause="Test signal for graceful shutdown validation",
        rca_resource_kind="Pod",
        rca_resource_namespace="test",
        rca_resource_name="test-pod",
        parameters={"TEST_MODE": "true", "ACTION": "validate"}
    ),
    "no_workflow_found": MockScenario(
        name="no_workflow_found",
        signal_type="MOCK_NO_WORKFLOW_FOUND",
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
        signal_type="MOCK_LOW_CONFIDENCE",
        severity="critical",
        workflow_id="d2c84d90-55ba-5ae1-b48d-6cc16e0edb5c",  # DD-WORKFLOW-002 v3.0: Deterministic UUID for generic-restart-v1
        workflow_title="Generic Pod Restart",
        confidence=0.35,  # Low confidence (<0.5) triggers human review
        root_cause="Multiple possible root causes identified, requires human judgment",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="ambiguous-pod",
        parameters={"ACTION": "restart"}
    ),
    "problem_resolved": MockScenario(
        name="problem_resolved",
        workflow_name="",  # No workflow needed - problem self-resolved
        signal_type="MOCK_PROBLEM_RESOLVED",
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
        signal_type="MOCK_MAX_RETRIES_EXHAUSTED",
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
        signal_type="MOCK_RCA_INCOMPLETE",
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
        parameters={"ACTION": "restart"}
    ),
    # ========================================
    # BR-AI-084 / ADR-054: Predictive Signal Mode Scenarios
    # ========================================
    "oomkilled_predictive": MockScenario(
        name="oomkilled_predictive",
        workflow_name="oomkill-increase-memory-v1",  # Same workflow catalog entry as reactive (SP normalizes signal type)
        signal_type="OOMKilled",  # Already normalized by SP from PredictedOOMKill
        severity="critical",
        workflow_id="21053597-2865-572b-89bf-de49b5b685da",  # Same as reactive oomkilled (same workflow)
        workflow_title="OOMKill Recovery - Increase Memory Limits",
        confidence=0.88,
        root_cause="Predicted OOMKill based on memory utilization trend analysis (predict_linear). Current memory usage is 85% of limit and growing at 50MB/min. Preemptive action recommended to increase memory limits before the predicted OOMKill event occurs.",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="api-server-abc123",
        parameters={"MEMORY_LIMIT": "2Gi", "NAMESPACE": "production"}
    ),
    "predictive_no_action": MockScenario(
        name="predictive_no_action",
        workflow_name="",  # No workflow needed - prediction unlikely to materialize
        signal_type="OOMKilled",  # Normalized signal type
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
    # ========================================
    # Category F: Advanced Recovery Scenarios (Mock LLM)
    # E2E-HAPI-049 to E2E-HAPI-054
    # ========================================
    # ========================================
    # Category F: Advanced Recovery Scenarios (NOT YET IMPLEMENTED - E2E-HAPI-049 to 054)
    # These scenarios use hardcoded workflow_id until workflows are seeded
    # ========================================
    "multi_step_recovery": MockScenario(
        name="multi_step_recovery",
        # workflow_name="autoscaler-enable-v1",  # TODO: Enable when workflow seeded for E2E-HAPI-049
        signal_type="InsufficientResources",
        severity="high",
        workflow_id="",  # PHASE 1 FIX: Empty until workflow seeded (returns no_matching_workflows)
        workflow_title="Enable Cluster Autoscaler",
        confidence=0.85,
        root_cause="Step 1 (memory increase) succeeded. Step 2 (scale deployment) failed due to cluster capacity exhaustion. Need to add nodes or reduce scope.",
        rca_resource_kind="Deployment",
        rca_resource_namespace="production",
        rca_resource_name="api-server",
        parameters={"ACTION": "enable_autoscaler", "MIN_NODES": "3", "MAX_NODES": "10"}
    ),
    "cascading_failure": MockScenario(
        name="cascading_failure",
        # workflow_name="memory-increase-v1",  # TODO: Enable when workflow seeded for E2E-HAPI-050
        signal_type="CrashLoopBackOff",
        severity="high",
        workflow_id="",  # PHASE 1 FIX: Empty until workflow seeded (returns no_matching_workflows)
        workflow_title="Increase Memory Limit (Leak Mitigation)",
        confidence=0.75,
        root_cause="Memory leak detected. Constant growth rate (50MB/min) repeats after restart. Pattern indicates application bug, not load-based issue. Restart failed previously.",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="api-server",
        parameters={"ACTION": "increase_memory", "CURRENT_LIMIT": "2Gi", "NEW_LIMIT": "4Gi"}
    ),
    "near_attempt_limit": MockScenario(
        name="near_attempt_limit",
        # workflow_name="rollback-deployment-v1",  # TODO: Enable when workflow seeded for E2E-HAPI-051
        signal_type="DatabaseConnectionError",
        severity="critical",
        workflow_id="",  # PHASE 1 FIX: Empty until workflow seeded (returns no_matching_workflows)
        workflow_title="Rollback to Last Known Good Version",
        confidence=0.90,
        root_cause="Database migration broke compatibility. This is the final recovery attempt (2 of 3 exhausted). Both forward fixes failed with different errors. Conservative rollback is most reliable strategy.",
        rca_resource_kind="Deployment",
        rca_resource_namespace="production",
        rca_resource_name="payment-service",
        parameters={"ACTION": "rollback", "TARGET_REVISION": "previous", "REASON": "final_attempt_conservative"}
    ),
    "noisy_neighbor": MockScenario(
        name="noisy_neighbor",
        # workflow_name="resource-quota-v1",  # TODO: Enable when workflow seeded for E2E-HAPI-052
        signal_type="HighDatabaseLatency",
        severity="high",
        workflow_id="",  # PHASE 1 FIX: Empty until workflow seeded (returns no_matching_workflows)
        workflow_title="Set Resource Quotas for Namespace",
        confidence=0.80,
        root_cause="Noisy neighbor detected. ML batch job in ml-workloads namespace consuming excessive resources on same nodes as database. Database pods experiencing CPU throttling.",
        rca_resource_kind="Namespace",
        rca_resource_namespace="ml-workloads",
        rca_resource_name="ml-workloads",
        parameters={"ACTION": "set_quota", "NAMESPACE": "ml-workloads", "CPU_LIMIT": "16", "MEMORY_LIMIT": "64Gi"}
    ),
    "network_partition": MockScenario(
        name="network_partition",
        # workflow_name="wait-for-heal-v1",  # TODO: Enable when workflow seeded for E2E-HAPI-053
        signal_type="NodeUnreachable",
        severity="high",
        workflow_id="",  # PHASE 1 FIX: Empty until workflow seeded (returns no_matching_workflows)
        workflow_title="Wait for Network Partition Heal",
        confidence=0.70,
        root_cause="Network partition detected (3 nodes unreachable for 8+ minutes). Split-brain risk. Conservative approach: wait for partition to heal before taking action.",
        rca_resource_kind="Node",
        rca_resource_namespace="",  # Cluster-scoped
        rca_resource_name="node-3",
        parameters={"ACTION": "wait_for_heal", "MAX_WAIT": "15m", "MONITOR_INTERVAL": "30s"}
    ),
    "recovery_basic": MockScenario(
        name="recovery_basic",
        # workflow_name="memory-increase-basic-v1",  # TODO: Enable when workflow seeded for E2E-HAPI-054
        signal_type="OOMKilled",
        severity="high",
        workflow_id="",  # PHASE 1 FIX: Empty until workflow seeded (returns no_matching_workflows)
        workflow_title="Increase Memory Limit",
        confidence=0.85,
        root_cause="Container killed due to out of memory. Simple recovery: increase memory limit from current 512Mi to 1Gi to prevent OOMKilled errors.",
        rca_resource_kind="Pod",
        rca_resource_namespace="production",
        rca_resource_name="api-pod",
        parameters={"ACTION": "increase_memory", "CURRENT_LIMIT": "512Mi", "NEW_LIMIT": "1Gi"}
    ),
}

# Default scenario if none matches
DEFAULT_SCENARIO = MockScenario(
    name="default",
    workflow_name="generic-restart-v1",  # Set workflow_name so UUID gets loaded from config
    signal_type="Unknown",
    severity="medium",
    workflow_id="placeholder-uuid-default",  # Placeholder - overwritten by config file
    workflow_title="Generic Pod Restart",
    confidence=0.75,
    root_cause="Unable to determine specific root cause",
    parameters={"ACTION": "restart"}
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
                if not matched:  # Only increment synced_count if this is the first match
                    synced_count += 1
                print(f"  ✅ Loaded default ({workflow_name_from_config}:{env_from_config}) → {workflow_uuid}")
                matched = True
            
            if not matched:
                # No match found for this config entry
                print(f"  ⚠️  No matching scenario for config entry: {workflow_key}")

        print(f"✅ Mock LLM loaded {synced_count}/{len(MOCK_SCENARIOS)} scenarios from file")
        
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
        elif has_tool_result:
            # Phase 2: After tool result → return final analysis
            return self._final_analysis_response(scenario, request_data)
        else:
            # Phase 1: Initial request → return tool call
            return self._tool_call_response(scenario, request_data)

    def _detect_scenario(self, messages: List[Dict[str, Any]]) -> MockScenario:
        """Detect which scenario to use based on message content."""
        # Combine all message content for analysis (including stringified objects)
        # This ensures we catch JSON fields like {"is_recovery_attempt": true}
        content = " ".join(
            str(m.get("content", ""))
            for m in messages
            if m.get("content")
        ).lower()

        # ALSO check all messages for tool_calls that might contain recovery request data
        all_text = " ".join(str(m) for m in messages).lower()

        # DEBUG: Log scenario detection
        logger.info(f"🔍 SCENARIO DETECTION - Content preview: {content[:200]}...")
        logger.info(f"🔍 SCENARIO DETECTION - Has 'is_recovery_attempt': {('is_recovery_attempt' in all_text)}")
        logger.info(f"🔍 SCENARIO DETECTION - Has 'recovery_attempt_number': {('recovery_attempt_number' in all_text)}")
        logger.info(f"🔍 SCENARIO DETECTION - Has 'recovery' + 'previous': {('recovery' in content and 'previous' in content)}")

        # Check for test-specific signal types first (human review tests)
        if "mock_no_workflow_found" in content or "mock no workflow found" in content:
            return MOCK_SCENARIOS.get("no_workflow_found", DEFAULT_SCENARIO)
        if "mock_low_confidence" in content or "mock low confidence" in content:
            return MOCK_SCENARIOS.get("low_confidence", DEFAULT_SCENARIO)
        if "mock_problem_resolved" in content or "mock problem resolved" in content:
            return MOCK_SCENARIOS.get("problem_resolved", DEFAULT_SCENARIO)
        if "mock_not_reproducible" in content or "mock not reproducible" in content:
            return MOCK_SCENARIOS.get("problem_resolved", DEFAULT_SCENARIO)  # Same scenario: issue self-resolved
        if "mock_rca_incomplete" in content or "mock rca incomplete" in content:
            return MOCK_SCENARIOS.get("rca_incomplete", DEFAULT_SCENARIO)
        # E2E-HAPI-003: Max retries exhausted - LLM parsing failed
        if "mock_max_retries_exhausted" in content or "mock max retries exhausted" in content:
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
        elif "nodenotready" in content or "node not ready" in content:
            matched_scenario = MOCK_SCENARIOS.get("node_not_ready", DEFAULT_SCENARIO)
            logger.info(f"✅ PHASE 2: Matched 'nodenotready' → scenario={matched_scenario.name}, workflow_id={matched_scenario.workflow_id}")
            return matched_scenario

        # Check for generic recovery scenario (has priority over Category F scenarios)
        # DD-TEST-011 v2.1: Detect recovery via JSON fields OR prompt keywords
        # Recovery requests include: {"is_recovery_attempt": true, "recovery_attempt_number": 1}
        if ("is_recovery_attempt" in all_text or "recovery_attempt_number" in all_text) or \
           ("recovery" in content and ("previous remediation" in content or "failed attempt" in content or "previous execution" in content)) or \
           ("workflow execution failed" in content and "recovery" in content):
            logger.info("✅ SCENARIO DETECTED: RECOVERY (generic)")
            return MOCK_SCENARIOS.get("recovery", DEFAULT_SCENARIO)

        # Check for Category F recovery scenarios (E2E-HAPI-049 to E2E-HAPI-054)
        # These are checked LAST to avoid over-matching generic keywords
        if "mock_multi_step_recovery" in content or "multi_step_recovery" in content or "multi step recovery" in content:
            logger.info("✅ SCENARIO DETECTED: MULTI_STEP_RECOVERY")
            return MOCK_SCENARIOS.get("multi_step_recovery", DEFAULT_SCENARIO)
        if "mock_cascading_failure" in content or "cascading_failure" in content or "cascading failure" in content:
            logger.info("✅ SCENARIO DETECTED: CASCADING_FAILURE")
            return MOCK_SCENARIOS.get("cascading_failure", DEFAULT_SCENARIO)
        if "mock_near_attempt_limit" in content or "near_attempt_limit" in content or "near attempt limit" in content:
            logger.info("✅ SCENARIO DETECTED: NEAR_ATTEMPT_LIMIT")
            return MOCK_SCENARIOS.get("near_attempt_limit", DEFAULT_SCENARIO)
        if "mock_noisy_neighbor" in content or "noisy_neighbor" in content or "noisy neighbor" in content:
            logger.info("✅ SCENARIO DETECTED: NOISY_NEIGHBOR")
            return MOCK_SCENARIOS.get("noisy_neighbor", DEFAULT_SCENARIO)
        if "mock_network_partition" in content or "network_partition" in content or "network partition" in content:
            logger.info("✅ SCENARIO DETECTED: NETWORK_PARTITION")
            return MOCK_SCENARIOS.get("network_partition", DEFAULT_SCENARIO)
        if "mock_recovery_basic" in content or "recovery_basic" in content or ("recovery" in content and "basic" in content):
            logger.info("✅ SCENARIO DETECTED: RECOVERY_BASIC")
            return MOCK_SCENARIOS.get("recovery_basic", DEFAULT_SCENARIO)

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

    def _tool_call_response(self, scenario: MockScenario, request_data: Dict[str, Any]) -> Dict[str, Any]:
        """Generate a response with tool call (Phase 1)."""
        call_id = f"call_{uuid.uuid4().hex[:12]}"

        # Build tool call arguments
        tool_args = {
            "query": f"{scenario.signal_type} {scenario.severity}",
            "rca_resource": {
                "signal_type": scenario.signal_type,
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

    def _get_category_f_strategies(self, scenario: MockScenario) -> List[Dict[str, Any]]:
        """
        Get recovery strategies for Category F scenarios (E2E-HAPI-049 to E2E-HAPI-054)
        
        Returns structured strategies array for advanced recovery scenarios.
        These scenarios support multiple strategy alternatives with varying confidence.
        """
        strategies_map = {
            "multi_step_recovery": [{
                "action_type": "enable_autoscaler",
                "confidence": 0.85,
                "rationale": "Step 1 memory increase successful. Step 2 failed due to cluster capacity. Enable cluster autoscaler to add nodes for scaling.",
                "estimated_risk": "low",
                "prerequisites": []
            }],
            "cascading_failure": [{
                "action_type": "increase_memory_limit",
                "confidence": 0.75,
                "rationale": "Memory leak detected. Constant growth rate (50MB/min) repeats after restart. Increase memory limit as temporary mitigation while root cause investigation continues.",
                "estimated_risk": "medium",
                "prerequisites": []
            }, {
                "action_type": "rollback_deployment",
                "confidence": 0.70,
                "rationale": "Memory leak pattern indicates application bug. Rollback to last known good version to restore service.",
                "estimated_risk": "low",
                "prerequisites": []
            }],
            "near_attempt_limit": [{
                "action_type": "rollback_deployment",
                "confidence": 0.90,
                "rationale": "This is the final recovery attempt (2 of 3 exhausted). Both forward fixes failed with different errors. Conservative rollback to last known good version is most reliable strategy to restore service.",
                "estimated_risk": "low",
                "prerequisites": []
            }],
            "noisy_neighbor": [{
                "action_type": "set_resource_quotas",
                "confidence": 0.80,
                "rationale": "Noisy neighbor detected. ML batch job consuming excessive resources on same nodes as database. Set resource quotas for ml-workloads namespace to enforce fairness.",
                "estimated_risk": "low",
                "prerequisites": []
            }, {
                "action_type": "set_priority_classes",
                "confidence": 0.75,
                "rationale": "Database is P0 service but lacks priority class. Set high priority for database pods to ensure scheduling preference during contention.",
                "estimated_risk": "low",
                "prerequisites": []
            }],
            "network_partition": [{
                "action_type": "wait_for_partition_heal",
                "confidence": 0.70,
                "rationale": "Network partition detected (3 nodes unreachable for 8+ minutes). Wait for partition to heal before taking action to avoid split-brain scenario. Monitor partition status.",
                "estimated_risk": "medium",
                "prerequisites": []
            }, {
                "action_type": "drain_partition_nodes",
                "confidence": 0.65,
                "rationale": "If partition persists, drain affected nodes and reschedule pods to healthy side of cluster. Risk: service disruption during drain.",
                "estimated_risk": "medium",
                "prerequisites": []
            }],
            "recovery_basic": [{
                "action_type": "increase_memory",
                "confidence": 0.85,
                "rationale": "Container killed due to out of memory. Increase memory limit from current 512Mi to 1Gi to prevent OOMKilled errors.",
                "estimated_risk": "low",
                "prerequisites": []
            }]
        }
        
        return strategies_map.get(scenario.name, [])

    def _final_analysis_response(self, scenario: MockScenario, request_data: Dict[str, Any]) -> Dict[str, Any]:
        """Generate final analysis response after tool result (Phase 2)."""
        # Detect if this is a recovery scenario
        # Priority 1: Check for recovery fields in messages (structured data from HolmesGPT SDK)
        messages = request_data.get("messages", [])
        is_recovery = False
        
        # Check each message for recovery context (SDK passes it as structured data)
        for msg in messages:
            msg_content = msg.get("content", "")
            # Check if the content contains recovery-related structured data
            if isinstance(msg_content, str):
                # HolmesGPT SDK may pass recovery context as string representation
                if "is_recovery_attempt" in msg_content.lower() or "recovery_attempt_number" in msg_content.lower():
                    is_recovery = True
                    break
                # Also check for recovery keywords in prompt text
                if "recovery" in msg_content.lower() and ("previous remediation" in msg_content.lower() or "previous execution" in msg_content.lower()):
                    is_recovery = True
                    break
        
        logger.info(f"🔍 Recovery detection: is_recovery={is_recovery}, scenario={scenario.name}")

        # Build the analysis content
        analysis_json = {
            "root_cause_analysis": {
                "summary": scenario.root_cause,
                "severity": scenario.severity,
                "signal_type": scenario.signal_type,
                "contributing_factors": ["identified_by_mock_llm"] if scenario.workflow_id else []
            },
            # E2E-HAPI-002: Always include confidence field in response
            "confidence": scenario.confidence
        }

        # BR-AI-081: Add recovery_analysis for recovery scenarios
        if is_recovery:
            logger.info(f"✅ RECOVERY DETECTED in _final_analysis_response - Adding recovery_analysis field")
            analysis_json["recovery_analysis"] = {
                "previous_attempt_assessment": {
                    "failure_understood": True,
                    "failure_reason_analysis": scenario.root_cause,
                    "state_changed": False,
                    "current_signal_type": scenario.signal_type
                }
            }
            
            # Category F: Advanced Recovery Scenarios (E2E-HAPI-049 to E2E-HAPI-054)
            # Return structured recovery format with multiple strategies
            if scenario.name in ["multi_step_recovery", "cascading_failure", "near_attempt_limit", 
                                  "noisy_neighbor", "network_partition", "recovery_basic"]:
                logger.info(f"✅ CATEGORY F SCENARIO DETECTED: {scenario.name} - Returning structured recovery format")
                analysis_json["strategies"] = self._get_category_f_strategies(scenario)
                analysis_json["can_recover"] = True
                analysis_json["confidence"] = scenario.confidence
        else:
            logger.info(f"⚠️  NO RECOVERY detected in _final_analysis_response - is_recovery={is_recovery}")

        # BR-HAPI-212: Conditionally include affectedResource in RCA
        # This allows testing scenarios where affectedResource is missing despite workflow being selected
        if scenario.include_affected_resource:
            affected_resource = {
                "kind": scenario.rca_resource_kind,
                "name": scenario.rca_resource_name,
            }
            # Add apiVersion if present (BR-HAPI-212: Optional field for GVK resolution)
            if scenario.rca_resource_api_version:
                affected_resource["apiVersion"] = scenario.rca_resource_api_version
            # Add namespace if present (not applicable for cluster-scoped resources)
            if scenario.rca_resource_namespace:
                affected_resource["namespace"] = scenario.rca_resource_namespace

            analysis_json["root_cause_analysis"]["affectedResource"] = affected_resource

        # BR-HAPI-200: Handle problem resolved case (investigation_outcome: "resolved")
        if scenario.name == "problem_resolved":
            analysis_json["selected_workflow"] = None
            analysis_json["investigation_outcome"] = "resolved"  # BR-HAPI-200: Signal problem self-resolved
            # Note: confidence already set at line 841
            analysis_json["can_recover"] = False  # E2E-HAPI-023: No recovery needed when problem self-resolved
            content = f"""Based on my investigation of the {scenario.signal_type} signal:

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
            content = f"""Based on my investigation of the {scenario.signal_type} signal:

# root_cause_analysis
{json.dumps(analysis_json["root_cause_analysis"])}

# confidence
{scenario.confidence}

# selected_workflow
{json.dumps(analysis_json["selected_workflow"])}

# alternative_workflows
{json.dumps(alternatives_list)}
"""
        # Handle no workflow found case
        elif not scenario.workflow_id:
            analysis_json["selected_workflow"] = None
            # Note: confidence already set at line 841
            
            # E2E-HAPI-024: Set can_recover and needs_human_review for no workflow found
            if is_recovery:
                analysis_json["can_recover"] = True  # Manual recovery possible
                analysis_json["needs_human_review"] = True
                analysis_json["human_review_reason"] = "no_matching_workflows"
            # E2E-HAPI-001: Set needs_human_review for incident with no workflow
            else:  # incident scenario
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
            
            # Use recovery format if this is a recovery attempt
            if is_recovery:
                content = f"""Based on my investigation of the recovery scenario:

## Previous Attempt Assessment

The previous remediation attempt failed. I've analyzed the current cluster state.

## Root Cause Analysis

{scenario.root_cause}

## Workflow Search Result

No suitable alternative workflow found. Human review required.

```json
{json.dumps(analysis_json, indent=2)}
```
"""
            else:
                # E2E-HAPI-003: Use section header format for SDK compatibility
                content = f"""Based on my investigation of the {scenario.signal_type} signal:

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
                "rationale": f"Selected based on {scenario.signal_type} signal analysis",
                "execution_engine": scenario.execution_engine,  # BR-WE-014
                "parameters": scenario.parameters
            }
            # Format as markdown with JSON block (like real LLM would)
            # Use recovery format if this is a recovery attempt
            if is_recovery:
                content = f"""Based on my investigation of the recovery scenario:

## Previous Attempt Assessment

The previous remediation attempt failed. I've analyzed the current cluster state.

## Root Cause Analysis

{scenario.root_cause}

## Alternative Workflow Recommendation

I've identified an alternative remediation workflow from the catalog.

```json
{json.dumps(analysis_json, indent=2)}
```
"""
            else:
                content = f"""Based on my investigation of the {scenario.signal_type} signal:

## Root Cause Analysis

{scenario.root_cause}

## Recommended Workflow

I've identified a suitable remediation workflow from the catalog.

```json
{json.dumps(analysis_json, indent=2)}
```
"""

        # DEBUG: Log what we're returning
        logger.info(f"📤 FINAL RESPONSE - Scenario: {scenario.name}, is_recovery: {is_recovery}")
        logger.info(f"📤 FINAL RESPONSE - analysis_json keys: {list(analysis_json.keys())}")
        if "recovery_analysis" in analysis_json:
            logger.info(f"✅ recovery_analysis IS in analysis_json")
        else:
            logger.info(f"❌ recovery_analysis NOT in analysis_json")

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
        messages = request_data.get("messages", [])
        prompt = messages[-1].get("content", "") if messages else ""

        # Use existing text response logic
        if "recovery" in prompt.lower():
            content = self._recovery_text_response(scenario)
        else:
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

    def _recovery_text_response(self, scenario: MockScenario) -> str:
        """Generate recovery analysis text response."""
        # ADR-045 v1.2: Generate alternative workflows for audit/context
        alternatives_list = []
        if scenario.alternatives:
            for alt in scenario.alternatives:
                alternatives_list.append({
                    "workflow_id": alt["workflow_id"],
                    "title": alt.get("title", "Alternative Recovery Workflow"),
                    "confidence": alt.get("confidence", 0.25),
                    "rationale": alt.get("rationale", "Alternative recovery approach")
                })
        
        # Handle no workflow found case
        if not scenario.workflow_id:
            return f"""Based on my investigation of the recovery scenario:

## Analysis

The previous remediation attempt failed. I've analyzed the current cluster state but found no suitable workflow.

```json
{{
  "recovery_analysis": {{
    "previous_attempt_assessment": {{
      "failure_understood": true,
      "failure_reason_analysis": "{scenario.root_cause}",
      "state_changed": false,
      "current_signal_type": "{scenario.signal_type}"
    }}
  }},
  "selected_workflow": null,
  "alternative_workflows": {json.dumps(alternatives_list)}
}}
```
"""

        return f"""Based on my investigation of the recovery scenario:

## Analysis

The previous remediation attempt failed. I've analyzed the current cluster state.

```json
{{
  "recovery_analysis": {{
    "previous_attempt_assessment": {{
      "failure_understood": true,
      "failure_reason_analysis": "{scenario.root_cause}",
      "state_changed": false,
      "current_signal_type": "{scenario.signal_type}"
    }}
  }},
  "selected_workflow": {{
    "workflow_id": "{scenario.workflow_id}",
    "version": "1.0.0",
    "confidence": {scenario.confidence},
    "rationale": "Alternative approach after failed attempt",
    "execution_engine": "{scenario.execution_engine}"
  }},
  "alternative_workflows": {json.dumps(alternatives_list)}
}}
```
"""

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
            scenario_name: One of "oomkilled", "crashloop", "node_not_ready", "recovery"
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
