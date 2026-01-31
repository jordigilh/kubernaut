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


# Pre-defined scenarios for common test cases
MOCK_SCENARIOS: Dict[str, MockScenario] = {
    "oomkilled": MockScenario(
        name="oomkilled",
        workflow_name="oomkill-increase-memory-limits",  # DD-TEST-011 v2.0: Match actual DataStorage workflow names (no -v1 suffix)
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
        workflow_name="crashloop-fix-configuration",  # DD-TEST-011 v2.0: Match actual DataStorage workflow names (no -v1 suffix)
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
        workflow_name="node-not-ready-drain-and-reboot",  # DD-TEST-011 v2.0: Match actual DataStorage workflow names (no -v1 suffix)
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
        workflow_name="oomkill-scale-down-replicas",  # DD-TEST-011 v2.0: Use different OOMKill workflow for recovery (staging environment)
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
}

# Default scenario if none matches
DEFAULT_SCENARIO = MockScenario(
    name="default",
    signal_type="Unknown",
    severity="medium",
    workflow_id="generic-restart-v1",
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
            for scenario_name, scenario in MOCK_SCENARIOS.items():
                if not scenario.workflow_name:
                    continue  # Skip scenarios without workflow_name

                # Match if workflow names match (ignore environment - use workflow from any environment)
                if scenario.workflow_name == workflow_name_from_config:
                    scenario.workflow_id = workflow_uuid
                    synced_count += 1
                    print(f"  ✅ Loaded {scenario_name} ({workflow_name_from_config}:{env_from_config}) → {workflow_uuid}")
                    break  # Found match, move to next config entry
            else:
                # No match found for this config entry
                print(f"  ⚠️  No matching scenario for config entry: {workflow_key}")

        print(f"✅ Mock LLM loaded {synced_count}/{len(MOCK_SCENARIOS)} scenarios from file")
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
        """Suppress default logging to reduce test noise."""
        pass

    def do_POST(self):
        """Handle POST requests (chat completions)."""
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length).decode('utf-8')

        try:
            request_data = json.loads(body) if body else {}
        except json.JSONDecodeError:
            request_data = {}

        # Route based on endpoint
        if self.path in ["/v1/chat/completions", "/chat/completions"]:
            response = self._handle_openai_request(request_data)
        elif self.path == "/api/generate" or self.path == "/api/chat":
            response = self._handle_ollama_request(request_data)
        else:
            response = {"status": "ok", "path": self.path}

        self._send_json_response(response)

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
        if "mock_rca_incomplete" in content or "mock rca incomplete" in content:
            return MOCK_SCENARIOS.get("rca_incomplete", DEFAULT_SCENARIO)

        # Check for test signal (graceful shutdown tests)
        if "testsignal" in content or "test signal" in content:
            return MOCK_SCENARIOS.get("test_signal", DEFAULT_SCENARIO)

        # Check for recovery scenario (has priority over regular signals)
        # DD-TEST-011 v2.1: Detect recovery via JSON fields OR prompt keywords
        # Recovery requests include: {"is_recovery_attempt": true, "recovery_attempt_number": 1}
        if ("is_recovery_attempt" in all_text or "recovery_attempt_number" in all_text) or \
           ("recovery" in content and ("previous remediation" in content or "failed attempt" in content or "previous execution" in content)) or \
           ("workflow execution failed" in content and "recovery" in content):
            logger.info("✅ SCENARIO DETECTED: RECOVERY")
            return MOCK_SCENARIOS.get("recovery", DEFAULT_SCENARIO)

        # Check for signal types (most specific first to avoid false matches)
        # DD-TEST-010: Match exact signal types, not generic substrings
        # "crashloop" is more specific than "oom", check it first
        if "crashloop" in content:
            return MOCK_SCENARIOS.get("crashloop", DEFAULT_SCENARIO)
        elif "oomkilled" in content:
            return MOCK_SCENARIOS.get("oomkilled", DEFAULT_SCENARIO)
        elif "nodenotready" in content or "node not ready" in content:
            return MOCK_SCENARIOS.get("node_not_ready", DEFAULT_SCENARIO)

        return MockLLMRequestHandler.current_scenario

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
            }
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
            analysis_json["confidence"] = scenario.confidence  # BR-HAPI-200: High confidence (>=0.7) that problem is resolved
            content = f"""Based on my investigation of the {scenario.signal_type} signal:

## Root Cause Analysis

{scenario.root_cause}

## Investigation Outcome

The problem has self-resolved. No remediation workflow is needed.

```json
{json.dumps(analysis_json, indent=2)}
```
"""
        # Handle no workflow found case
        elif not scenario.workflow_id:
            analysis_json["selected_workflow"] = None
            analysis_json["confidence"] = scenario.confidence  # Low confidence (0.0) triggers human review
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
                content = f"""Based on my investigation of the {scenario.signal_type} signal:

## Root Cause Analysis

{scenario.root_cause}

## Workflow Search Result

No suitable workflow found in the catalog for this scenario. Human review required.

```json
{json.dumps(analysis_json, indent=2)}
```
"""
        else:
            analysis_json["selected_workflow"] = {
                "workflow_id": scenario.workflow_id,
                "title": scenario.workflow_title,
                "version": "1.0.0",
                "confidence": scenario.confidence,
                "rationale": f"Selected based on {scenario.signal_type} signal analysis",
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
  "selected_workflow": null
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
    "rationale": "Alternative approach after failed attempt"
  }}
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
    "rationale": "Selected based on signal analysis"
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
