from flask import Flask, request, jsonify
import logging

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Mock Playbook Database - Single Remediation Per Playbook
MOCK_PLAYBOOKS = {
    "oomkill-general": {
        "playbook_id": "oomkill-general",
        "version": "1.0.0",
        "title": "General OOMKill Remediation",
        "description": "Provides steps to diagnose and resolve OutOfMemoryKill events for any application.",
        "labels": {
            "signal_type": "OOMKilled",
            "severity": "high",
            "component": "*",
            "environment": "*",
            "priority": "*",
            "risk_tolerance": "medium",
            "business_category": "*"
        },
        "remediation_steps": [
            "Check pod logs for OOMKilled events.",
            "Review resource limits and requests for the container.",
            "Analyze application memory usage patterns.",
            "Consider increasing memory limits if application requires more resources.",
            "Optimize application memory consumption."
        ],
        "estimated_time_minutes": 30,
        "impact": "High",
        "confidence_score": 0.9
    },

    # SINGLE REMEDIATION PLAYBOOK 1: Scale Down
    "oomkill-scale-down": {
        "playbook_id": "oomkill-scale-down",
        "version": "1.0.0",
        "title": "OOMKill Remediation - Scale Down Replicas",
        "description": "Reduces replica count for deployments experiencing OOMKilled due to node memory pressure. Use when: (1) Node memory utilization >90%, (2) Multiple pods OOMKilled on same node, (3) Application can tolerate reduced capacity.",
        "execution_bundle": "quay.io/kubernaut/playbook-oomkill-scale-down:v1.0.0",
        "labels": {
            "signal_type": "OOMKilled",
            "severity": "high",
            "category": "resource-management",
            "tags": ["oomkill", "scaling", "cost-optimization"],
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "cost-management"
        },
        "parameters": [
            {
                "name": "TARGET_RESOURCE_KIND",
                "description": "Kubernetes resource kind (Deployment, StatefulSet, DaemonSet)",
                "type": "string",
                "required": True,
                "enum": ["Deployment", "StatefulSet", "DaemonSet"]
            },
            {
                "name": "TARGET_RESOURCE_NAME",
                "description": "Name of the Kubernetes resource experiencing OOMKilled",
                "type": "string",
                "required": True,
                "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
            },
            {
                "name": "TARGET_NAMESPACE",
                "description": "Kubernetes namespace of the affected resource",
                "type": "string",
                "required": True,
                "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
            },
            {
                "name": "SCALE_TARGET_REPLICAS",
                "description": "Target replica count to scale down to",
                "type": "integer",
                "required": True,
                "minimum": 0,
                "maximum": 100
            }
        ],
        "estimated_time_minutes": 5,
        "impact": "Medium",
        "confidence_score": 0.85
    },

    # SINGLE REMEDIATION PLAYBOOK 2: Increase Memory
    "oomkill-increase-memory": {
        "playbook_id": "oomkill-increase-memory",
        "version": "1.0.0",
        "title": "OOMKill Remediation - Increase Memory Limits",
        "description": "Increases memory limits for pods experiencing OOMKilled. Use when: (1) Single pod repeatedly OOMKilled, (2) Memory usage consistently at limit, (3) Application legitimately needs more memory.",
        "execution_bundle": "quay.io/kubernaut/playbook-oomkill-increase-memory:v1.0.0",
        "labels": {
            "signal_type": "OOMKilled",
            "severity": "high",
            "category": "resource-management",
            "tags": ["oomkill", "memory", "capacity"],
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "cost-management"
        },
        "parameters": [
            {
                "name": "TARGET_RESOURCE_KIND",
                "description": "Kubernetes resource kind (Deployment, StatefulSet, DaemonSet)",
                "type": "string",
                "required": True,
                "enum": ["Deployment", "StatefulSet", "DaemonSet"]
            },
            {
                "name": "TARGET_RESOURCE_NAME",
                "description": "Name of the Kubernetes resource experiencing OOMKilled",
                "type": "string",
                "required": True,
                "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
            },
            {
                "name": "TARGET_NAMESPACE",
                "description": "Kubernetes namespace of the affected resource",
                "type": "string",
                "required": True,
                "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
            },
            {
                "name": "MEMORY_LIMIT_NEW",
                "description": "New memory limit to apply (e.g., 256Mi, 1Gi, 2Gi)",
                "type": "string",
                "required": True,
                "pattern": "^[0-9]+(Mi|Gi)$",
                "examples": ["256Mi", "1Gi", "2Gi"]
            }
        ],
        "estimated_time_minutes": 10,
        "impact": "Medium",
        "confidence_score": 0.80
    },

    # SINGLE REMEDIATION PLAYBOOK 3: Optimize Application
    "oomkill-optimize-application": {
        "playbook_id": "oomkill-optimize-application",
        "version": "1.0.0",
        "title": "OOMKill Remediation - Optimize Application Configuration",
        "description": "Optimizes application configuration to reduce memory footprint. Use when: (1) Application has tunable memory settings, (2) Memory leak suspected, (3) Inefficient configuration detected.",
        "execution_bundle": "quay.io/kubernaut/playbook-oomkill-optimize-app:v1.0.0",
        "labels": {
            "signal_type": "OOMKilled",
            "severity": "high",
            "category": "resource-management",
            "tags": ["oomkill", "optimization", "configuration"],
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "cost-management"
        },
        "parameters": [
            {
                "name": "TARGET_RESOURCE_KIND",
                "description": "Kubernetes resource kind (Deployment, StatefulSet, DaemonSet)",
                "type": "string",
                "required": True,
                "enum": ["Deployment", "StatefulSet", "DaemonSet"]
            },
            {
                "name": "TARGET_RESOURCE_NAME",
                "description": "Name of the Kubernetes resource experiencing OOMKilled",
                "type": "string",
                "required": True,
                "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
            },
            {
                "name": "TARGET_NAMESPACE",
                "description": "Kubernetes namespace of the affected resource",
                "type": "string",
                "required": True,
                "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
            }
        ],
        "estimated_time_minutes": 15,
        "impact": "Low",
        "confidence_score": 0.75
    },

    "pod-restart-general": {
        "playbook_id": "pod-restart-general",
        "version": "1.0.0",
        "title": "General Pod Restart",
        "description": "Restarts pods to recover from transient failures.",
        "labels": {
            "signal_type": "PodCrashLooping",
            "severity": "medium",
            "component": "*",
            "environment": "*",
            "priority": "*",
            "risk_tolerance": "medium",
            "business_category": "*"
        },
        "remediation_steps": [
            "Identify the failing pod.",
            "Check pod logs for error messages.",
            "Delete the pod to trigger a restart.",
            "Monitor pod status after restart."
        ],
        "estimated_time_minutes": 10,
        "impact": "Low",
        "confidence_score": 0.8
    }
}

@app.route('/health', methods=['GET'])
def health():
    return jsonify({"status": "healthy"}), 200

@app.route('/api/v1/playbooks/search', methods=['POST'])
def search_playbooks():
    """
    Mock MCP Server - Playbook Catalog Search
    Simulates semantic search for playbook recommendations
    """
    try:
        data = request.get_json()
        logger.info(f"Received playbook search request: {data}")

        # Extract search criteria
        signal_type = data.get('signal_type', '')
        namespace = data.get('namespace', '')
        severity = data.get('severity', '')

        # Simple matching logic (in real implementation, this would be semantic search)
        matching_playbooks = []

        for playbook_id, playbook in MOCK_PLAYBOOKS.items():
            labels = playbook.get('labels', {})

            # Match signal type
            if signal_type and labels.get('signal_type') != '*':
                if labels.get('signal_type') != signal_type:
                    continue

            # Match business category (namespace-based)
            if 'cost-management' in namespace and labels.get('business_category') != '*':
                if labels.get('business_category') != 'cost-management':
                    continue

            matching_playbooks.append(playbook)

        logger.info(f"Found {len(matching_playbooks)} matching playbooks")

        return jsonify({
            "playbooks": matching_playbooks,
            "total_count": len(matching_playbooks)
        }), 200

    except Exception as e:
        logger.error(f"Error processing playbook search: {str(e)}")
        return jsonify({"error": str(e)}), 500

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8095, debug=True)

# NOTE: The above playbooks use separate container images.
# Operators can optionally use a SHARED container image with hidden parameters:
#
# Example: All three oomkill-* playbooks could use the same image:
#   "quay.io/kubernaut/playbook-oomkill-multi:v1.0.0"
#
# With hidden parameter:
#   "REMEDIATION_TYPE": "scale_down" | "increase_memory" | "optimize_application"
#
# This hidden parameter is:
# - Set during playbook registration (NOT by LLM)
# - Injected by Workflow Engine into Tekton PipelineRun
# - Used by container to route to correct remediation logic
# - NOT visible to LLM (maintains single remediation playbook pattern)
#
# Benefits:
# - Reduced image duplication
# - Shared validation and common logic
# - Operator flexibility
# - Simple LLM interface (no conditional logic)
#
# See DD-PLAYBOOK-003 "Advanced Pattern: Hidden Parameters" for details.
