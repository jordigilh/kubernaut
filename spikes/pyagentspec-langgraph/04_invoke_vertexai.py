"""Follow-up Spike 2: OAS/LangGraph with real Vertex AI invocation.

Validates:
1. ChatAnthropic (with AnthropicVertex client) + LangGraph create_react_agent
2. Real LLM reasoning over mock tool results via Vertex AI
3. End-to-end: signal context -> tool calls -> structured RCA
"""

import asyncio
import json
import os

from anthropic import AnthropicVertex, AsyncAnthropicVertex
from langchain_anthropic import ChatAnthropic
from langchain_core.messages import AIMessage, HumanMessage
from langchain_core.tools import StructuredTool
from langgraph.prebuilt import create_react_agent

MOCK_TOOL_RESPONSES = {
    "kubectl_get": json.dumps({
        "apiVersion": "apps/v1", "kind": "Deployment",
        "metadata": {"name": "web-app", "namespace": "production"},
        "status": {"availableReplicas": 0, "replicas": 3,
                   "conditions": [{"type": "Available", "status": "False",
                                   "reason": "MinimumReplicasUnavailable"}]},
    }),
    "kubectl_list_events": json.dumps([
        {"type": "Warning", "reason": "OOMKilled",
         "message": "Container killed due to OOM", "count": 3},
    ]),
    "prometheus_query": json.dumps([
        {"metric": {"__name__": "container_memory_working_set_bytes"},
         "value": [1716415200, "536870912"]},
    ]),
    "submit_result": json.dumps({"status": "accepted"}),
}


def mock_tool_handler(tool_name: str):
    def handler(**kwargs):
        return MOCK_TOOL_RESPONSES.get(tool_name, json.dumps({"error": "unknown"}))
    return handler


def build_tools():
    tools = []
    for name, desc in [
        ("kubectl_get", "Get a Kubernetes resource by kind, name, namespace"),
        ("kubectl_list_events", "List Kubernetes events for a namespace/resource"),
        ("prometheus_query", "Execute a PromQL query and return results"),
        ("submit_result", "Submit the structured RCA investigation result"),
    ]:
        tools.append(StructuredTool.from_function(
            func=mock_tool_handler(name),
            name=name,
            description=desc,
        ))
    return tools


async def run_investigation():
    project = os.environ.get("GCP_PROJECT_ID", "YOUR_GCP_PROJECT")
    location = os.environ.get("GCP_LOCATION", "us-east5")
    model = "claude-sonnet-4@20250514"

    print(f"[INFO] Vertex AI project={project} location={location}")
    print(f"[INFO] Model: {model} via Vertex AI (AnthropicVertex)")
    print()

    vertex_client = AnthropicVertex(project_id=project, region=location)
    async_vertex_client = AsyncAnthropicVertex(project_id=project, region=location)

    llm = ChatAnthropic(
        model=model,
        anthropic_api_key="vertex-managed",
        max_tokens=2048,
    )
    llm._client = vertex_client
    llm._async_client = async_vertex_client

    tools = build_tools()
    graph = create_react_agent(llm, tools)

    signal_context = (
        "SIGNAL FIRED: OOMKill detected\n"
        "Namespace: production\n"
        "Resource: Deployment/web-app\n"
        "Severity: critical\n\n"
        "Investigate using the available tools. "
        "Call kubectl_get to check the deployment status, "
        "kubectl_list_events to see recent events, "
        "then submit_result with your root cause analysis including "
        "root_cause, confidence (0-1), affected_resources list, "
        "and remediation_suggested boolean."
    )

    print("[INFO] Invoking investigation via LangGraph ReAct agent + Vertex AI...")
    result = await graph.ainvoke(
        {"messages": [HumanMessage(content=signal_context)]},
    )

    messages = result["messages"]
    print(f"[PASS] Investigation completed with {len(messages)} messages")

    tool_calls_made = []
    for msg in messages:
        if isinstance(msg, AIMessage) and msg.tool_calls:
            for tc in msg.tool_calls:
                tool_calls_made.append(tc["name"])
                print(f"       Tool call: {tc['name']}({json.dumps(tc['args'])[:80]})")

    print(f"       Total tool calls: {len(tool_calls_made)}")

    final_ai_msg = None
    for msg in reversed(messages):
        if isinstance(msg, AIMessage) and not msg.tool_calls:
            final_ai_msg = msg
            break

    if final_ai_msg:
        print(f"\n[INFO] Final LLM response (truncated):")
        print(f"       {final_ai_msg.content[:200]}")

    submit_call = None
    for msg in messages:
        if isinstance(msg, AIMessage) and msg.tool_calls:
            for tc in msg.tool_calls:
                if tc["name"] == "submit_result":
                    submit_call = tc
                    break

    if submit_call:
        print(f"\n[PASS] submit_result was called with args:")
        print(f"       {json.dumps(submit_call['args'], indent=2)[:300]}")
    else:
        print("\n[WARN] No submit_result call found -- LLM may not have called it")

    print()
    print("=" * 60)
    print("  FOLLOW-UP SPIKE 2: Real Vertex AI through OAS/LangGraph")
    print("=" * 60)
    print()
    print(f"  Messages:   {len(messages)}")
    print(f"  Tool calls: {tool_calls_made}")
    print(f"  LLM used:   claude-sonnet-4@20250514 via Vertex AI")
    print(f"  submit_result called: {'YES' if submit_call else 'NO'}")
    print()
    if submit_call and tool_calls_made:
        print("  VERDICT: PASS - Real Vertex AI + LangGraph + tools validated")
    else:
        print("  VERDICT: PARTIAL - LLM responded but tool flow incomplete")


if __name__ == "__main__":
    asyncio.run(run_investigation())
