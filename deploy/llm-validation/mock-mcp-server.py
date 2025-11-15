from flask import Flask, request, jsonify
import logging

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Mock Playbook Database
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
    "oomkill-cost-optimized": {
        "playbook_id": "oomkill-cost-optimized",
        "version": "1.0.0",
        "title": "Cost-Optimized OOMKill Remediation for Cost-Management Namespace",
        "description": "Specific steps to resolve OOMKill events for applications in the 'cost-management' namespace, prioritizing cost efficiency.",
        "labels": {
            "signal_type": "OOMKilled",
            "severity": "high",
            "component": "*",
            "environment": "*",
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "cost-management"
        },
        "remediation_steps": [
            "Immediately check pod logs for OOMKilled events in 'cost-management' namespace.",
            "Verify if the application is essential or can be scaled down/off-peak.",
            "Prioritize application-level memory optimization over increasing resource limits.",
            "If limits must be increased, ensure strict approval process due to cost implications.",
            "Investigate potential memory leaks within the application code."
        ],
        "estimated_time_minutes": 60,
        "impact": "Medium",
        "confidence_score": 0.95
    },
    "crashloop-configmap": {
        "playbook_id": "crashloop-configmap",
        "version": "1.0.0",
        "title": "CrashLoopBackOff due to Missing ConfigMap",
        "description": "Steps to resolve CrashLoopBackOff caused by a missing Kubernetes ConfigMap.",
        "labels": {
            "signal_type": "CrashLoopBackOff",
            "severity": "high",
            "component": "*",
            "environment": "*",
            "priority": "P1",
            "risk_tolerance": "medium",
            "business_category": "*"
        },
        "remediation_steps": [
            "Check pod events for 'FailedMount' or 'ConfigMap not found' messages.",
            "Verify if the referenced ConfigMap exists in the namespace (`kubectl get configmap <name> -n <namespace>`).",
            "If missing, create the ConfigMap with correct data.",
            "If exists, check for typos in deployment/pod spec referencing the ConfigMap.",
            "Ensure correct permissions for the service account to access ConfigMaps."
        ],
        "estimated_time_minutes": 20,
        "impact": "High",
        "confidence_score": 0.88
    },
    "imagepullbackoff": {
        "playbook_id": "imagepullbackoff",
        "version": "1.0.0",
        "title": "ImagePullBackOff Remediation",
        "description": "Steps to resolve ImagePullBackOff status in Kubernetes pods.",
        "labels": {
            "signal_type": "ImagePullBackOff",
            "severity": "medium",
            "component": "*",
            "environment": "*",
            "priority": "P2",
            "risk_tolerance": "medium",
            "business_category": "*"
        },
        "remediation_steps": [
            "Check pod events for 'Failed to pull image' or 'ErrImagePull' messages.",
            "Verify the image name and tag in the pod/deployment spec are correct.",
            "Check if the image exists in the specified registry.",
            "Ensure image pull secrets are correctly configured and accessible by the service account.",
            "Check network connectivity to the image registry."
        ],
        "estimated_time_minutes": 15,
        "impact": "Medium",
        "confidence_score": 0.85
    }
}

@app.route('/mcp/tools/search_playbook_catalog', methods=['POST'])
def search_playbook_catalog():
    data = request.json

    logger.info("=" * 80)
    logger.info("EVENT 1: LLM -> MCP Tool Call")
    logger.info("=" * 80)
    logger.info(f"Request Data: {data}")

    # Extract ALL 7 mandatory search criteria per DD-PLAYBOOK-001
    signal_type = data.get('signal_type', '*')
    severity = data.get('severity', '*')
    component = data.get('component', '*')
    environment = data.get('environment', '*')
    priority = data.get('priority', '*')
    risk_tolerance = data.get('risk_tolerance', '*')
    business_category = data.get('business_category', '*')
    query = data.get('query', '')

    matched_playbooks = []

    for playbook_id, playbook in MOCK_PLAYBOOKS.items():
        labels = playbook['labels']
        match_score = 0

        # Exact match required: signal_type, severity, component, risk_tolerance (DD-PLAYBOOK-001 Line 38)

        # Match signal_type (exact match required)
        if signal_type != '*':
            if labels['signal_type'] == signal_type:
                match_score += 10  # Exact match
            elif labels['signal_type'] == '*':
                match_score += 5   # Wildcard match
            else:
                continue  # No match, skip this playbook

        # Match severity (exact match required)
        if severity != '*':
            if labels['severity'] == severity:
                match_score += 10  # Exact match
            elif labels['severity'] == '*':
                match_score += 5   # Wildcard match
            else:
                continue  # No match, skip this playbook

        # Match component (exact match required)
        if component != '*':
            if labels['component'] == component:
                match_score += 10  # Exact match
            elif labels['component'] == '*':
                match_score += 5   # Wildcard match
            else:
                continue  # No match, skip this playbook

        # Match risk_tolerance (exact match required)
        if risk_tolerance != '*':
            if labels['risk_tolerance'] == risk_tolerance:
                match_score += 10  # Exact match
            elif labels['risk_tolerance'] == '*':
                match_score += 5   # Wildcard match
            else:
                continue  # No match, skip this playbook

        # Wildcard support: environment, priority, business_category (DD-PLAYBOOK-001 Line 39)

        # Match environment (wildcard support)
        if environment != '*':
            if labels['environment'] == environment:
                match_score += 10  # Exact match
            elif labels['environment'] == '*':
                match_score += 5   # Wildcard match
            else:
                continue  # No match, skip this playbook

        # Match priority (wildcard support)
        if priority != '*':
            if labels['priority'] == priority:
                match_score += 10  # Exact match
            elif labels['priority'] == '*':
                match_score += 5   # Wildcard match
            else:
                continue  # No match, skip this playbook

        # Match business_category (wildcard support, highest priority)
        if business_category != '*':
            if labels['business_category'] == business_category:
                match_score += 20  # Exact match - highest priority
            elif labels['business_category'] == '*':
                match_score += 3   # Wildcard match - lower priority
            else:
                continue  # No match, skip this playbook

        # Simple text matching for query (optional - boosts score if matches)
        if query:
            query_lower = query.lower()
            if query_lower in playbook['title'].lower() or query_lower in playbook['description'].lower():
                match_score += 5

        matched_playbooks.append({
            'playbook': playbook,
            'score': match_score
        })

    # Sort by score (highest first)
    matched_playbooks.sort(key=lambda x: x['score'], reverse=True)

    # Extract just the playbooks
    result_playbooks = [item['playbook'] for item in matched_playbooks]

    logger.info(f"Response: {len(result_playbooks)} playbooks returned")
    logger.info("=" * 80)

    return jsonify({
        'playbooks': result_playbooks,
        'total_results': len(result_playbooks)
    })

@app.route('/health', methods=['GET'])
def health():
    return jsonify({'status': 'ok'})

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8081, debug=True)

