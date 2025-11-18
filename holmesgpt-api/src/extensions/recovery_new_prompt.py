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

def _create_investigation_prompt(request_data: Dict[str, Any]) -> str:
    """
    Create investigation prompt with complete ADR-041 v3.1 hybrid format.

    Reference: ADR-041 v3.1 - LLM Prompt and Response Contract
    """
    # Extract fields
    signal_type = request_data.get("signal_type", "Unknown")
    severity = request_data.get("severity", "unknown")
    namespace = request_data.get("resource_namespace", "unknown")
    resource_kind = request_data.get("resource_kind", "unknown")
    resource_name = request_data.get("resource_name", "unknown")
    environment = request_data.get("environment", "unknown")
    priority = request_data.get("priority", "P2")
    risk_tolerance = request_data.get("risk_tolerance", "medium")
    business_category = request_data.get("business_category", "standard")

    failed_action = request_data.get("failed_action", {})
    failure_context = request_data.get("failure_context", {})
    error_message = failure_context.get("error_message", "Unknown error")
    description = failure_context.get("description", "")

    # Timing information
    firing_time = request_data.get('firing_time', 'Unknown')
    received_time = request_data.get('received_time', 'Unknown')

    # Deduplication and storm
    is_duplicate = request_data.get('is_duplicate', False)
    occurrence_count = request_data.get('occurrence_count', 0)
    first_seen = request_data.get('first_seen', 'Unknown')
    last_seen = request_data.get('last_seen', 'Unknown')
    is_storm = request_data.get('is_storm', False)
    storm_alert_count = request_data.get('storm_alert_count', 0)
    storm_type = request_data.get('storm_type', 'Unknown')
    storm_window = request_data.get('storm_window', '5m')
    affected_resources = request_data.get('affected_resources', [])

    # Cluster context
    cluster_name = request_data.get('cluster_name', 'unknown')
    signal_source = request_data.get('signal_source', 'unknown')
    signal_labels = request_data.get('signal_labels', {})

    # Generate contextual descriptions
    priority_descriptions = {
        "P0": f"P0 (highest priority) - This is a {business_category} service requiring immediate attention",
        "P1": "P1 (high priority) - This service requires prompt attention",
        "P2": "P2 (medium priority) - This service requires timely resolution",
        "P3": "P3 (low priority) - This service can be addressed during normal operations"
    }

    risk_guidance = {
        "low": "low (conservative remediation required - avoid aggressive restarts or scaling)",
        "medium": "medium (balanced approach - standard remediation actions permitted)",
        "high": "high (aggressive remediation permitted - prioritize recovery speed)"
    }

    priority_desc = priority_descriptions.get(priority, f"{priority} - Standard priority")
    risk_desc = risk_guidance.get(risk_tolerance, f"{risk_tolerance} risk tolerance")

    # Build incident summary with natural language
    incident_summary = f"A **{severity} {signal_type} event** has occurred in the **{namespace}/{resource_kind}/{resource_name}**."

    # Add deduplication fact if duplicate
    if is_duplicate and occurrence_count > 0:
        incident_summary += f" **Alert fired {occurrence_count} times**."

    # Add storm fact if storm detected
    if is_storm:
        resource_count = len(affected_resources) if affected_resources else "multiple"
        incident_summary += f" **Storm detected**: {storm_type} type, {storm_alert_count} alerts, {resource_count} resources."

    incident_summary += f"\n{error_message}"

    # Build complete ADR-041 v3.1 hybrid prompt
    prompt = f"""# Incident Analysis Request

## Incident Summary

{incident_summary}

**Business Impact Assessment**:
- **Priority**: {priority_desc}
- **Environment**: {environment}
- **Risk Tolerance**: {risk_desc}

**Technical Details**:
- Signal Type: {signal_type}
- Severity: {severity}
- Resource: {namespace}/{resource_kind}/{resource_name}
- Error: {error_message}
- Failed Action: {failed_action.get('type', 'N/A')} (target: {failed_action.get('target', 'N/A')})

## Error Details (FOR RCA INVESTIGATION)
- Error Message: {error_message}
- Description: {description if description else 'N/A'}
- Firing Time: {firing_time}
- Received Time: {received_time}
"""

    # Add Deduplication Context if applicable
    if is_duplicate and occurrence_count > 0:
        prompt += f"""
## Deduplication Context (FOR RCA INVESTIGATION)
- Is Duplicate: {is_duplicate}
- First Seen: {first_seen}
- Last Seen: {last_seen}
- Occurrence Count: {occurrence_count}

**What Deduplication Means**:
Deduplication tracks duplicate alerts from the monitoring system (Prometheus/Kubernetes). When the same
condition persists, Prometheus fires the same alert every evaluation interval (30-60 seconds). The Gateway
deduplicates these within a 5-minute window to avoid creating multiple RemediationRequest CRDs for the
same ongoing issue.

**RCA Implications**:
- `occurrence_count > 1` means the condition has been **continuously present** since `first_seen`
- This indicates a **persistent, ongoing issue** - not that remediation was attempted and failed
- Focus on understanding why the condition persists, not why remediation failed
- Higher occurrence counts suggest the condition is stable/consistent, not intermittent
"""

    # Add Storm Detection if applicable
    if is_storm:
        prompt += f"""
## Storm Detection (FOR RCA INVESTIGATION)
- Is Storm: {is_storm}
- Storm Type: {storm_type}
- Storm Window: {storm_window}
- Storm Alert Count: {storm_alert_count}
- Affected Resources: {len(affected_resources) if affected_resources else 'Unknown'}
"""
        if affected_resources and len(affected_resources) <= 10:
            prompt += "\n**Affected Resources List**:\n"
            for resource in affected_resources:
                prompt += f"- {resource}\n"
        elif affected_resources:
            prompt += f"\n**Affected Resources** (showing first 10 of {len(affected_resources)}):\n"
            for resource in affected_resources[:10]:
                prompt += f"- {resource}\n"

    # Add Cluster Context
    prompt += f"""
## Cluster Context (FOR RCA INVESTIGATION)
- Cluster: {cluster_name}
- Signal Source: {signal_source}
- Signal Labels: {signal_labels if signal_labels else 'N/A'}

## Business Context (FOR WORKFLOW SEARCH - NOT FOR RCA)
These fields are used by MCP workflow search tools to match workflows.
You do NOT need to consider these in your RCA analysis.

- Environment: {environment}
- Priority: {priority}
- Business Category: {business_category}
- Risk Tolerance: {risk_tolerance}

**Note**: When you call MCP workflow search tools (e.g., `search_workflow_catalog`), you must
pass these business context fields as parameters.

## Required Analysis

**INVESTIGATION APPROACH**:
Perform independent Root Cause Analysis (RCA) using available tools based on the signal type and incident context.

**Available Tools**:
- Kubernetes investigation tools (kubectl, API queries)
- Prometheus/metrics tools (if applicable to signal source)
- Log analysis tools
- Other tools as appropriate for the signal source

**Analysis Steps** (adapt based on signal source and incident):
1. Investigate the signal using appropriate tools for the signal source
2. Gather relevant context and evidence
3. Perform Root Cause Analysis based on your investigation
4. Formulate remediation strategies based on your RCA findings

**Guidance**:
- Use tools appropriate for the signal source (e.g., Kubernetes for pod failures, Prometheus for metric alerts)
- Base your analysis on actual investigation findings, not assumptions
- Consider cluster state and resource availability
- Focus on technical remediation based on RCA findings

## RCA Severity Assessment

After your investigation, assess the severity of the root cause using these levels.

**IMPORTANT**: Your RCA severity may differ from the input signal severity. Use your analysis to determine the actual severity based on business impact.

### Severity Levels:

**critical** - Immediate remediation required
- Production service completely unavailable
- Data loss or corruption occurring
- Security breach actively exploited
- SLA violation in progress
- Revenue-impacting outage
- Affects >50% of users
- Examples:
  - Production database offline
  - Payment processing completely failed
  - Authentication service down in production
  - Active data breach

**high** - Urgent remediation needed
- Significant service degradation (>50% performance loss)
- High error rate (>10% of requests failing)
- Production issue escalating toward critical
- Affects 10-50% of users
- SLA at risk
- Examples:
  - Production API response time >5s (normally <100ms)
  - 50% of requests failing
  - Memory leak causing frequent OOM crashes
  - Database connection pool exhausted

**medium** - Remediation recommended
- Minor service degradation (<50% performance loss)
- Moderate error rate (1-10% of requests failing)
- Non-production critical issues
- Affects <10% of users
- Staging/development critical issues
- Examples:
  - Staging environment service down
  - Development database performance degraded
  - Non-critical background job failing
  - Disk usage at 80% (not yet critical)

**low** - Remediation optional
- Informational issues
- Optimization opportunities
- Development environment issues
- No user impact
- Capacity planning alerts
- Examples:
  - Development pod restart
  - Staging performance slightly degraded
  - Capacity planning alert (disk at 60%)
  - Non-critical log warnings

### Assessment Factors (in order of importance):

1. **User Impact**: How many users/services are affected?
   - All users → critical
   - Many users (>50%) → critical
   - Some users (10-50%) → high
   - Few users (<10%) → medium
   - No users → low

2. **Environment**: Where is the issue?
   - Production + user impact → +1 severity level
   - Staging → same severity level
   - Development → -1 severity level (minimum: low)

3. **Business Impact**: Revenue/SLA/customer trust affected?
   - Revenue loss → critical
   - SLA violation → critical or high
   - Customer complaints → high
   - No business impact → low

4. **Escalation Risk**: Is the issue getting worse?
   - Rapidly escalating → +1 severity level
   - Stable → same severity level
   - Improving → consider lower severity

5. **Data Risk**: Is data at risk?
   - Data loss/corruption → critical
   - Data integrity risk → high
   - No data risk → no change

**Return your assessed severity in the response.**

## MCP Workflow Search Guidance

When searching for remediation workflows, use this taxonomy:

**Query Format**: `<signal_type> <severity> [optional_keywords]`
- Example: `"OOMKilled critical"` or `"CrashLoopBackOff high"`
- Use canonical Kubernetes event reasons for signal_type (from your RCA assessment)
- Use your RCA severity assessment (may differ from input signal)

**Canonical Signal Types** (examples - use any canonical Kubernetes event reason):
- `OOMKilled`: Container exceeded memory limit and was killed
- `CrashLoopBackOff`: Container repeatedly crashing
- `ImagePullBackOff`: Cannot pull container image
- `Evicted`: Pod evicted due to resource pressure
- `NodeNotReady`: Node is not ready
- `PodPending`: Pod stuck in pending state
- `FailedScheduling`: Scheduler cannot place pod
- `BackoffLimitExceeded`: Job exceeded retry limit
- `DeadlineExceeded`: Job exceeded active deadline
- `FailedMount`: Volume mount failed

**Note**: These are common examples. Use any canonical Kubernetes event reason that matches your RCA findings.
For complete list, see: https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/event-v1/#Event

**Label Parameters** (for MCP workflow search):
1. **signal_type** (Technical - from your RCA assessment)
2. **severity** (Technical - from your RCA assessment)
3. **environment** (Business - pass-through: `{environment}`)
4. **priority** (Business - pass-through: `{priority}`)
5. **risk_tolerance** (Business - pass-through: `{risk_tolerance}`)
6. **business_category** (Business - pass-through: `{business_category}`)

**Search Optimization**:
- Exact label matching increases confidence score
- Workflow descriptions should start with `"<signal_type> <severity>:"`
- Use all 6 label parameters for filtering

## Expected Response Format

Provide your analysis in two parts:

### Part 1: Natural Language Analysis

Explain your investigation findings, root cause analysis, and reasoning for workflow selection.

### Part 2: Structured JSON

```json
{{
  "root_cause_analysis": {{
    "summary": "Brief summary of root cause",
    "severity": "critical|high|medium|low",
    "contributing_factors": ["factor1", "factor2"]
  }},
  "selected_workflow": {{
    "workflow_id": "workflow-id-from-mcp-search-results",
    "version": "1.0.0",
    "confidence": 0.95,
    "rationale": "Why your search parameters led to this workflow selection (based on RCA findings)",
    "parameters": {{
      "PARAM_NAME": "value",
      "ANOTHER_PARAM": "value"
    }}
  }}
}}
```

**IMPORTANT**:
- Select ONE workflow per incident
- Populate ALL required parameters from the workflow schema
- Use your RCA findings to determine parameter values
- Pass-through business context fields (environment, priority, risk_tolerance, business_category) to MCP search
"""

    return prompt


