#!/usr/bin/env python3
"""Extract JSON fixture snapshots from the Python Mock LLM for Go parity testing.

Starts the Python Mock LLM on a random port, sends requests with each scenario's
detection pattern, and saves the responses as JSON files in the current directory.

Detection patterns mirror Python server.py _detect_scenario:
  - mock_* keywords for: no_workflow_found, low_confidence, problem_resolved,
    problem_resolved_contradiction, not_reproducible, rca_incomplete, max_retries_exhausted
  - "test signal" keyword for: test_signal
  - "- Signal Name: <X>" for: oomkilled, crashloop, node_not_ready, cert_not_ready
  - Signal + proactive markers for: oomkilled_predictive, predictive_no_action
  - mock_rca_permanent_error for: permanent_error (HTTP 500, checked pre-routing)
"""

import json
import os
import sys
import time
import http.client

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'services', 'mock-llm'))

from src.server import MockLLMServer

SCENARIOS = {
    "oomkilled": "Please investigate this alert.\n- Signal Name: OOMKilled\n- Namespace: default\n- Pod: test-pod-abc123",
    "crashloop": "Please investigate this alert.\n- Signal Name: CrashLoopBackOff\n- Namespace: default\n- Pod: test-pod-crash",
    "node_not_ready": "Please investigate this alert.\n- Signal Name: NodeNotReady\n- Node: worker-1",
    "cert_not_ready": "Please investigate this alert.\n- Signal Name: CertManagerCertNotReady\n- Namespace: cert-manager",
    "no_workflow_found": "Analyze this issue: mock_no_workflow_found",
    "low_confidence": "Analyze this issue: mock_low_confidence",
    "problem_resolved": "Analyze this issue: mock_problem_resolved",
    "problem_resolved_contradiction": "Analyze this issue: mock_problem_resolved_contradiction",
    "not_reproducible": "Analyze this issue: mock_not_reproducible",
    "rca_incomplete": "Analyze this issue: mock_rca_incomplete",
    "max_retries_exhausted": "Analyze this issue: mock_max_retries_exhausted",
    "test_signal": "Handle this test signal for graceful shutdown",
    "oomkilled_predictive": "Investigate in proactive mode.\n- Signal Name: OOMKilled\nThis is a predicted issue that has not yet occurred.",
    "predictive_no_action": "Investigate in proactive mode.\n- Signal Name: OOMKilled\nThis is a predicted issue that has not yet occurred. mock_predictive_no_action",
    "permanent_error": "Analyze this issue: mock_rca_permanent_error",
}

LEGACY_TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "search_workflow_catalog",
            "description": "Search for workflows",
            "parameters": {"type": "object", "properties": {"query": {"type": "string"}}}
        }
    }
]

THREE_STEP_TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "list_available_actions",
            "description": "List available actions",
            "parameters": {"type": "object", "properties": {}}
        }
    },
    {
        "type": "function",
        "function": {
            "name": "list_workflows",
            "description": "List workflows",
            "parameters": {"type": "object", "properties": {"action_type": {"type": "string"}}}
        }
    },
    {
        "type": "function",
        "function": {
            "name": "get_workflow",
            "description": "Get workflow details",
            "parameters": {"type": "object", "properties": {"workflow_id": {"type": "string"}}}
        }
    }
]


def make_request(conn, content, tools, mode_suffix=""):
    """Send a chat completion request and return the response."""
    body = json.dumps({
        "model": "mock-model",
        "messages": [
            {"role": "user", "content": content}
        ],
        "tools": tools if tools else None
    })
    conn.request("POST", "/v1/chat/completions", body=body,
                 headers={"Content-Type": "application/json"})
    resp = conn.getresponse()
    status = resp.status
    data = resp.read().decode("utf-8")
    try:
        parsed = json.loads(data)
    except json.JSONDecodeError:
        parsed = {"_raw": data}
    return {"status_code": status, "body": parsed}


def extract_all(host, port):
    """Extract fixtures for all scenarios."""
    conn = http.client.HTTPConnection(host, port, timeout=10)
    fixtures = {}

    for scenario_name, content in SCENARIOS.items():
        fixture = {}

        fixture["legacy_turn1"] = make_request(conn, content, LEGACY_TOOLS)
        fixture["three_step_turn1"] = make_request(conn, content, THREE_STEP_TOOLS)
        fixture["text_only"] = make_request(conn, content, None)

        fixtures[scenario_name] = fixture

        filename = f"{scenario_name}.json"
        with open(filename, "w") as f:
            json.dump(fixture, f, indent=2, sort_keys=True)
        print(f"  Extracted: {filename}")

    with open("all_scenarios.json", "w") as f:
        json.dump(fixtures, f, indent=2, sort_keys=True)
    print(f"  Combined: all_scenarios.json")

    conn.close()


def main():
    port = 18199
    print(f"Starting Python Mock LLM on port {port}...")

    with MockLLMServer(port=port) as server:
        time.sleep(0.5)

        conn = http.client.HTTPConnection("127.0.0.1", port, timeout=5)
        conn.request("GET", "/health")
        resp = conn.getresponse()
        resp.read()
        conn.close()
        print(f"  Health check: {resp.status}")

        print("Extracting fixtures...")
        extract_all("127.0.0.1", port)

    print("Done.")


if __name__ == "__main__":
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    main()
