"""Minimal MCP server that exposes Kubernaut investigation tools.
Used to validate Goose CLI -> MCP tool call roundtrip."""

import json
import logging
from mcp.server.fastmcp import FastMCP

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("kubernaut-mcp-spike")

mcp = FastMCP("kubernaut-tools-spike", host="127.0.0.1", port=9876)

TOOL_CALL_LOG = []


@mcp.tool()
def kubectl_get(kind: str, name: str, namespace: str) -> str:
    """Get a Kubernetes resource by kind, name, and namespace."""
    logger.info(f"kubectl_get called: kind={kind}, name={name}, namespace={namespace}")
    TOOL_CALL_LOG.append({"tool": "kubectl_get", "args": {"kind": kind, "name": name, "namespace": namespace}})
    return json.dumps({
        "apiVersion": "apps/v1",
        "kind": kind,
        "metadata": {"name": name, "namespace": namespace},
        "status": {"availableReplicas": 0, "replicas": 3,
                   "conditions": [{"type": "Available", "status": "False",
                                   "reason": "MinimumReplicasUnavailable"}]},
    })


@mcp.tool()
def kubectl_list_events(namespace: str, resource_name: str) -> str:
    """List Kubernetes events for a resource in a namespace."""
    logger.info(f"kubectl_list_events called: namespace={namespace}, resource_name={resource_name}")
    TOOL_CALL_LOG.append({"tool": "kubectl_list_events", "args": {"namespace": namespace, "resource_name": resource_name}})
    return json.dumps([
        {"type": "Warning", "reason": "OOMKilled",
         "message": "Container killed due to OOM", "count": 3,
         "lastTimestamp": "2026-05-22T21:55:00Z"},
        {"type": "Warning", "reason": "BackOff",
         "message": "Back-off restarting failed container", "count": 15,
         "lastTimestamp": "2026-05-22T22:00:00Z"},
    ])


@mcp.tool()
def submit_result(root_cause: str, confidence: float, affected_resources: list[str], remediation_suggested: bool) -> str:
    """Submit the structured RCA investigation result."""
    logger.info(f"submit_result called: root_cause={root_cause}, confidence={confidence}")
    TOOL_CALL_LOG.append({
        "tool": "submit_result",
        "args": {
            "root_cause": root_cause,
            "confidence": confidence,
            "affected_resources": affected_resources,
            "remediation_suggested": remediation_suggested,
        },
    })
    return json.dumps({"status": "accepted", "tool_calls_total": len(TOOL_CALL_LOG)})


if __name__ == "__main__":
    mcp.run(transport="streamable-http")
