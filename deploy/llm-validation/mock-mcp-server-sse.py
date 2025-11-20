from flask import Flask, request, jsonify, Response
import logging
import json
import time

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Mock Workflow Database (updated from playbooks to workflows per DD-WORKFLOW-002)
MOCK_WORKFLOWS = {
    "oomkill-increase-memory": {
        "workflow_id": "oomkill-increase-memory",
        "title": "OOMKill Remediation - Increase Memory Limits",
        "description": "Increases memory limits for pods experiencing OOMKilled. Use when: (1) Single pod repeatedly OOMKilled, (2) Memory usage consistently at limit, (3) Application legitimately needs more memory.",
        "signal_types": ["OOMKilled"],
        "estimated_duration": "10 minutes",
        "success_rate": 0.85,
        "similarity_score": 0.92  # Mock semantic similarity
    },
    "oomkill-scale-down": {
        "workflow_id": "oomkill-scale-down",
        "title": "OOMKill Remediation - Scale Down Replicas",
        "description": "Reduces replica count for deployments experiencing OOMKilled due to node memory pressure. Use when: (1) Node memory utilization >90%, (2) Multiple pods OOMKilled on same node, (3) Application can tolerate reduced capacity.",
        "signal_types": ["OOMKilled", "NodePressure"],
        "estimated_duration": "5 minutes",
        "success_rate": 0.80,
        "similarity_score": 0.78
    },
    "oomkill-optimize-application": {
        "workflow_id": "oomkill-optimize-application",
        "title": "OOMKill Remediation - Optimize Application Configuration",
        "description": "Optimizes application configuration to reduce memory footprint. Use when: (1) Application has tunable memory settings, (2) Memory leak suspected, (3) Inefficient configuration detected.",
        "signal_types": ["OOMKilled"],
        "estimated_duration": "15 minutes",
        "success_rate": 0.75,
        "similarity_score": 0.65
    }
}

@app.route('/health', methods=['GET'])
def health():
    return jsonify({"status": "healthy"}), 200

@app.route('/sse', methods=['GET', 'POST'])
def sse_endpoint():
    """
    SSE endpoint for HolmesGPT SDK MCP integration

    The SDK expects:
    1. POST to /sse to initialize connection
    2. Server responds with SSE stream
    3. Client sends tool call requests via SSE
    4. Server responds with tool results via SSE

    For this mock, we'll return a simple SSE response with available tools.
    """
    def generate_sse_events():
        # Send initial connection message
        yield f"data: {json.dumps({'type': 'connection', 'status': 'connected', 'server': 'mock-mcp-workflow-catalog'})}\n\n"

        # Send available tools list
        tools = [{
            "name": "search_workflow_catalog",
            "description": "Search for remediation workflows based on incident characteristics",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Natural language query describing the incident (e.g., 'OOMKilled critical')"
                    },
                    "filters": {
                        "type": "object",
                        "description": "Optional filters for workflow search",
                        "properties": {
                            "signal_types": {"type": "array", "items": {"type": "string"}},
                            "business_category": {"type": "string"},
                            "risk_tolerance": {"type": "string"},
                            "environment": {"type": "string"}
                        }
                    },
                    "top_k": {
                        "type": "integer",
                        "description": "Number of top results to return (default: 3)"
                    }
                },
                "required": ["query"]
            }
        }]

        yield f"data: {json.dumps({'type': 'tools/list', 'tools': tools})}\n\n"

        # Keep connection alive
        time.sleep(0.1)
        yield f"data: {json.dumps({'type': 'ping'})}\n\n"

    return Response(generate_sse_events(), mimetype='text/event-stream')

@app.route('/mcp/tools/search_workflow_catalog', methods=['POST'])
def search_workflow_catalog():
    """
    MCP Tool: Search Workflow Catalog (DD-WORKFLOW-002 compliant)

    Input (per DD-WORKFLOW-002):
    {
        "query": "OOMKilled critical",  # Natural language query
        "filters": {                     # Optional
            "signal_types": ["OOMKilled"],
            "business_category": "general",
            "risk_tolerance": "medium",
            "environment": "production"
        },
        "top_k": 3
    }

    Output (per DD-WORKFLOW-002):
    {
        "workflows": [
            {
                "workflow_id": "oomkill-increase-memory",
                "title": "OOMKill Remediation - Increase Memory Limits",
                "description": "...",
                "signal_types": ["OOMKilled"],
                "estimated_duration": "10 minutes",
                "success_rate": 0.85,
                "similarity_score": 0.92
            }
        ]
    }
    """
    try:
        data = request.get_json()
        logger.info(f"📥 MCP Tool Call - search_workflow_catalog")
        logger.info(f"   Query: {data.get('query', 'N/A')}")
        logger.info(f"   Filters: {data.get('filters', {})}")
        logger.info(f"   Top K: {data.get('top_k', 3)}")

        # Extract search parameters
        query = data.get('query', '')
        filters = data.get('filters', {})
        top_k = data.get('top_k', 3)

        # Simple matching logic (in real implementation, this would be semantic search)
        matching_workflows = []

        for workflow_id, workflow in MOCK_WORKFLOWS.items():
            # Match signal types if provided in filters
            if filters.get('signal_types'):
                if not any(st in workflow['signal_types'] for st in filters['signal_types']):
                    continue

            # Match query keywords (simple keyword matching for mock)
            if query.lower():
                query_lower = query.lower()
                if 'oomkill' in query_lower or 'memory' in query_lower:
                    matching_workflows.append(workflow)

        # Sort by similarity_score and limit to top_k
        matching_workflows.sort(key=lambda w: w['similarity_score'], reverse=True)
        matching_workflows = matching_workflows[:top_k]

        logger.info(f"📤 MCP Tool Response - Found {len(matching_workflows)} workflows")
        for wf in matching_workflows:
            logger.info(f"   - {wf['workflow_id']} (similarity: {wf['similarity_score']})")

        return jsonify({
            "workflows": matching_workflows
        }), 200

    except Exception as e:
        logger.error(f"❌ Error in search_workflow_catalog: {str(e)}")
        return jsonify({"error": str(e)}), 500

if __name__ == '__main__':
    logger.info("🚀 Starting Mock MCP Server with SSE support")
    logger.info("   SSE Endpoint: http://0.0.0.0:8081/sse")
    logger.info("   Tool Endpoint: http://0.0.0.0:8081/mcp/tools/search_workflow_catalog")
    app.run(host='0.0.0.0', port=8081, debug=True)

                "signal_types": ["OOMKilled"],
                "estimated_duration": "10 minutes",
                "success_rate": 0.85,
                "similarity_score": 0.92
            }
        ]
    }
    """
    try:
        data = request.get_json()
        logger.info(f"📥 MCP Tool Call - search_workflow_catalog")
        logger.info(f"   Query: {data.get('query', 'N/A')}")
        logger.info(f"   Filters: {data.get('filters', {})}")
        logger.info(f"   Top K: {data.get('top_k', 3)}")

        # Extract search parameters
        query = data.get('query', '')
        filters = data.get('filters', {})
        top_k = data.get('top_k', 3)

        # Simple matching logic (in real implementation, this would be semantic search)
        matching_workflows = []

        for workflow_id, workflow in MOCK_WORKFLOWS.items():
            # Match signal types if provided in filters
            if filters.get('signal_types'):
                if not any(st in workflow['signal_types'] for st in filters['signal_types']):
                    continue

            # Match query keywords (simple keyword matching for mock)
            if query.lower():
                query_lower = query.lower()
                if 'oomkill' in query_lower or 'memory' in query_lower:
                    matching_workflows.append(workflow)

        # Sort by similarity_score and limit to top_k
        matching_workflows.sort(key=lambda w: w['similarity_score'], reverse=True)
        matching_workflows = matching_workflows[:top_k]

        logger.info(f"📤 MCP Tool Response - Found {len(matching_workflows)} workflows")
        for wf in matching_workflows:
            logger.info(f"   - {wf['workflow_id']} (similarity: {wf['similarity_score']})")

        return jsonify({
            "workflows": matching_workflows
        }), 200

    except Exception as e:
        logger.error(f"❌ Error in search_workflow_catalog: {str(e)}")
        return jsonify({"error": str(e)}), 500

if __name__ == '__main__':
    logger.info("🚀 Starting Mock MCP Server with SSE support")
    logger.info("   SSE Endpoint: http://0.0.0.0:8081/sse")
    logger.info("   Tool Endpoint: http://0.0.0.0:8081/mcp/tools/search_workflow_catalog")
    app.run(host='0.0.0.0', port=8081, debug=True)

