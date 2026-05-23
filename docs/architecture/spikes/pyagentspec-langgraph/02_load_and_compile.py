"""Spike 1b: Load OAS YAML spec and compile to LangGraph CompiledStateGraph.

Validates:
1. YAML deserialization via AgentSpecLoader
2. LangGraph graph compilation (OAS -> ReAct agent)
3. Graph structure (nodes, edges, tool wiring)
4. State schema
"""

import json
import os

from pyagentspec.adapters.langgraph import AgentSpecLoader


def mock_tool_handler(tool_name: str):
    """Returns a mock tool handler that returns canned responses."""
    responses = {
        "kubectl_get": json.dumps({
            "apiVersion": "apps/v1",
            "kind": "Deployment",
            "metadata": {"name": "web-app", "namespace": "production"},
            "status": {"availableReplicas": 0, "replicas": 3,
                       "conditions": [{"type": "Available", "status": "False",
                                       "reason": "MinimumReplicasUnavailable"}]},
        }),
        "kubectl_list_events": json.dumps([
            {"type": "Warning", "reason": "BackOff",
             "message": "Back-off restarting failed container",
             "count": 15, "lastTimestamp": "2026-05-22T22:00:00Z"},
            {"type": "Warning", "reason": "OOMKilled",
             "message": "Container killed due to OOM",
             "count": 3, "lastTimestamp": "2026-05-22T21:55:00Z"},
        ]),
        "prometheus_query": json.dumps([
            {"metric": {"__name__": "container_memory_working_set_bytes"},
             "value": [1716415200, "536870912"]},
        ]),
        "submit_result": json.dumps({"status": "accepted"}),
    }
    async def handler(**kwargs):
        return responses.get(tool_name, json.dumps({"error": f"unknown tool: {tool_name}"}))
    return handler


with open("kubernaut-rca-investigator.yaml") as f:
    yaml_content = f.read()

tool_registry = {
    "kubectl_get": mock_tool_handler("kubectl_get"),
    "kubectl_list_events": mock_tool_handler("kubectl_list_events"),
    "prometheus_query": mock_tool_handler("prometheus_query"),
    "submit_result": mock_tool_handler("submit_result"),
}

print("[INFO] Loading OAS YAML spec via AgentSpecLoader...")
loader = AgentSpecLoader(tool_registry=tool_registry)

try:
    graph = loader.load_yaml(yaml_content)
    print("[PASS] LangGraph CompiledStateGraph compiled successfully")
    print(f"       Graph type: {type(graph).__name__}")
    print(f"       Nodes: {list(graph.nodes.keys())}")

    drawable = graph.get_graph()
    node_ids = list(drawable.nodes)
    edge_list = [(e.source, e.target) for e in drawable.edges]
    print(f"       Drawable node IDs: {node_ids}")
    print(f"       Drawable edges: {edge_list}")

    if hasattr(graph, 'get_state'):
        print(f"       State channels: available")

    print()
    print("[INFO] Graph structure analysis:")
    print(f"       Pattern: ReAct (reason-act loop)")
    print(f"       Entry: __start__ -> agent")

    has_tool_edge = any(e.source == "agent" and e.target == "tools" for e in drawable.edges)
    has_loop_edge = any(e.source == "tools" and e.target == "agent" for e in drawable.edges)
    has_end_edge = any(e.target == "__end__" for e in drawable.edges)

    print(f"       Agent->Tools edge: {'yes' if has_tool_edge else 'MISSING'}")
    print(f"       Tools->Agent loop: {'yes' if has_loop_edge else 'MISSING'}")
    print(f"       Exit to __end__: {'yes' if has_end_edge else 'MISSING'}")

    all_ok = has_tool_edge and has_loop_edge and has_end_edge
    print()
    if all_ok:
        print("[PASS] Graph structure validates ReAct agent pattern")
    else:
        print("[WARN] Graph structure incomplete -- may indicate adapter limitation")

except Exception as e:
    print(f"[FAIL] LangGraph compilation failed: {type(e).__name__}: {e}")
    import traceback
    traceback.print_exc()
