# ADR-041: LLM Prompt and Response Contract for Workflow Selection

**Status**: Proposed
**Date**: 2025-11-16
**Deciders**: Architecture Team
**Related**: DD-WORKFLOW-003, DD-STORAGE-008, DD-WORKFLOW-002, BR-WORKFLOW-001
**Version**: 3.3

---

## Context

The HolmesGPT API sends prompts to the LLM for Root Cause Analysis (RCA) and remediation workflow selection. The LLM must understand the prompt structure and return a structured JSON response that the system can parse and execute.

### Problem

Without a single authoritative definition of the prompt/response contract:
- Prompt structure and response format can drift out of sync
- Multiple documents define pieces of the contract (recovery.py, DD-WORKFLOW-003, etc.)
- No single source of truth for validation
- Difficult to maintain consistency across v1.0, v1.1, v2.0

### Requirements

1. Define the complete LLM prompt structure
2. Define the expected JSON response format
3. Ensure alignment with DD-STORAGE-008 v1.2 (workflow catalog schema)
4. Ensure alignment with DD-WORKFLOW-003 v2.2 (parameterized actions)
5. Support v1.0 MVP testing and production deployment

---

## Decision

**Create a single authoritative ADR defining the LLM prompt structure and expected response format for workflow selection.**

This ADR serves as the contract between:
- HolmesGPT API (prompt generator)
- LLM Provider (Claude 4.5 Haiku - current testing model, subject to change)
- Response Parser (holmesgpt-api)
- Downstream services (RemediationExecution)

---

## Design Principles

### Input Principle: Observable Facts Only

**CRITICAL**: The LLM prompt must contain ONLY observable facts from the signal/incident, NOT pre-analyzed conclusions.

**Allowed Input** (Observable Facts):
- ✅ Failed action details (type, target, namespace)
- ✅ Error messages and error types (from Kubernetes/system)
- ✅ Cluster context (cluster name, namespace, priority)
- ✅ Business context (priority level, environment classification)
- ✅ Signal categorization (OOMKilled, CrashLoopBackOff, etc.)
- ✅ Recovery attempt history (number of previous attempts)
- ✅ Operational constraints (max attempts, timeout)

**Prohibited Input** (Pre-Analyzed Conclusions):
- ❌ Root cause analysis (would contaminate LLM's independent RCA)
- ❌ Symptoms assessment (would bias investigation)
- ❌ Pre-selected remediation strategies (would limit LLM's options)
- ❌ Confidence scores (LLM must assess independently)
- ❌ Risk assessments (LLM must evaluate based on investigation)

**Rationale**:
- The LLM must perform **independent Root Cause Analysis (RCA)** without bias
- Pre-conditioning the input with conclusions would:
  - Contaminate the analysis with potentially incorrect assumptions
  - Limit the LLM's ability to discover alternative root causes
  - Reduce confidence in the LLM's recommendations
  - Create circular reasoning (input conclusions → output conclusions)

**Output Freedom**:
- The LLM has complete freedom in its analysis and conclusions
- The output format is strictly defined (natural language + structured JSON)
- The LLM must justify all conclusions based on its investigation
- The LLM selects workflows and populates parameters based on its RCA

---


## LLM Prompt Structure

### Section 1: Incident Context (Hybrid Format)

**Design Principle**: Use natural language narrative for quick understanding, followed by structured technical details for precision.

```markdown
# Recovery Analysis Request

## Incident Summary

A **{severity} {signal_type} event** from **{signal_source}** has occurred in the **{namespace}/{resource_kind}/{resource_name}**.
{error_message}

**Business Impact Assessment**:
- **Priority**: {priority} - {business_category_description}
- **Environment**: {environment}
- **Risk Tolerance**: {risk_tolerance} - {risk_guidance}

**Technical Details**:
- Signal Type: {signal_type}
- Severity: {severity}
- Resource: {namespace}/{resource_kind}/{resource_name}
- Error: {error_message}
- Failed Action: {failed_action_type} (target: {failed_action_target})
```

**Field Mapping**:

| Field | Source | Purpose | Example |
|-------|--------|---------|---------|
| `signal_type` | Signal Processing | Canonical event type for workflow search | `OOMKilled` |
| `severity` | Signal Processing | Initial severity (LLM may override) | `critical` |
| `namespace` | Signal metadata | Kubernetes namespace | `production` |
| `resource_kind` | Signal metadata | Resource type | `deployment` |
| `resource_name` | Signal metadata | Resource identifier | `payment-service` |
| `error_message` | Failure context | What went wrong | `Container exceeded memory limit` |
| `priority` | Business classification | Business priority level | `P0`, `P1`, `P2`, `P3` |
| `business_category` | Business classification | Business impact category | `revenue-critical` |
| `environment` | Business classification | Deployment environment | `production`, `staging` |
| `risk_tolerance` | Business classification | Remediation risk policy | `low`, `medium`, `high` |
| `failed_action_type` | Failure context | What action failed | `restart`, `scale`, `update` |
| `failed_action_target` | Failure context | What was targeted | `pod`, `deployment` |

**Contextual Descriptions** (Generated Dynamically):

- **Priority Descriptions**:
  - `P0` → "P0 (highest priority) - This is a {business_category} service requiring immediate attention"
  - `P1` → "P1 (high priority) - This service requires prompt attention"
  - `P2` → "P2 (medium priority) - This service requires timely resolution"
  - `P3` → "P3 (low priority) - This service can be addressed during normal operations"

- **Risk Tolerance Guidance**:
  - `low` → "low (conservative remediation required - avoid aggressive restarts or scaling)"
  - `medium` → "medium (balanced approach - standard remediation actions permitted)"
  - `high` → "high (aggressive remediation permitted - prioritize recovery speed)"

**Example Prompt** (First-Time Incident):

```markdown
# Recovery Analysis Request

## Incident Summary

A **critical OOMKilled event** from **prometheus-adapter** has occurred in the **production/deployment/payment-service**.
Container exceeded memory limit.

**Business Impact Assessment**:
- **Priority**: P0 (highest priority) - This is a revenue-critical service requiring immediate attention
- **Environment**: production
- **Risk Tolerance**: low (conservative remediation required - avoid aggressive restarts or scaling)

**Technical Details**:
- Signal Type: OOMKilled
- Severity: critical
- Resource: production/deployment/payment-service
- Error: Container exceeded memory limit
- Failed Action: restart (target: pod)
```



## Error Details (FOR RCA INVESTIGATION)
- Error Message: {error_message}        # From signal annotations
- Description: {description}            # From signal annotations
- Firing Time: {firing_time}            # When signal started
- Received Time: {received_time}        # When Gateway received signal

## Deduplication Context (FOR RCA INVESTIGATION)
- Is Duplicate: {is_duplicate}          # True if this signal fingerprint has been seen before
- First Seen: {first_seen}              # When this signal fingerprint was first received by Gateway
- Last Seen: {last_seen}                # When this signal fingerprint was last received by Gateway
- Occurrence Count: {occurrence_count}  # Number of times this identical signal was received within 5-minute window

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

## Storm Detection (FOR RCA INVESTIGATION)
- Is Storm: {is_storm}                  # True if part of an alert storm
- Storm Type: {storm_type}              # "rate" (frequency-based) or "pattern" (similar alerts)
- Storm Window: {storm_window}          # Time window for storm detection (e.g., "5m")
- Storm Alert Count: {storm_alert_count}  # Number of alerts in the storm
- Affected Resources: {affected_resources}  # List of affected resources in aggregated storm (if applicable)

## Cluster Context (FOR RCA INVESTIGATION)
- Cluster: {cluster_name}               # Target cluster
- Signal Source: {signal_source}        # e.g., "prometheus-adapter", "kubernetes-event-adapter"
- Signal Labels: {signal_labels}        # Source-specific labels (key-value pairs)

## Business Context (FOR PLAYBOOK SEARCH - NOT FOR RCA)
These fields are used by MCP workflow search tools to match workflows.
You do NOT need to consider these in your RCA analysis.

- Environment: {environment}            # e.g., "production", "staging" (for workflow matching)
- Priority: {priority}                  # e.g., "P0", "P1", "P2", "P3" (for workflow matching)
- Business Category: {business_category}  # e.g., "payment-service" (for workflow matching)
- Risk Tolerance: {risk_tolerance}      # e.g., "low", "medium", "high" (for workflow matching)

**Note**: When you call MCP workflow search tools (e.g., `search_workflow_catalog`), you must
pass these business context fields as parameters when calling workflow search tools.
```

**Purpose**: Provides observable facts from the signal for LLM investigation

**Backing**:
- **DD-WORKFLOW-001**: Mandatory Workflow Label Schema (7 labels)
- **DD-CATEGORIZATION-001**: Gateway vs Signal Processing Categorization Split
- **NormalizedSignal** (pkg/gateway/types/types.go): Gateway output structure
- **RemediationProcessingSpec** (api/remediationprocessing/v1alpha1): Signal Processing output
- **RemediationRequestSpec** (api/remediation/v1alpha1): Gateway output with deduplication and storm detection

**CRITICAL PRINCIPLE**: This section contains ONLY observable facts. NO pre-analysis, NO workflow recommendations, NO root cause assessment, NO historical patterns that could contaminate the LLM's independent RCA.

**Field Usage**:

**For RCA Investigation** (use these in your analysis):
- `signal_type`: What happened (e.g., "OOMKilled") - investigate this event type
- `severity`: How critical the issue is
- `component`: What resource type is affected
- `alert_name`, `namespace`, `resource_kind`, `resource_name`: What to investigate
- `error_message`, `description`: Error details to analyze
- `firing_time`, `received_time`: When the issue occurred
- `cluster_name`, `signal_source`, `signal_labels`: Investigation context
- Deduplication count (occurrence_count)
- Storm detection flags (is_storm, storm_type, storm_alert_count)
**For Workflow Search** (pass these to MCP tools):

**Environment/Context Fields** (use as-is from prompt):
- `environment`: Matches workflow's environment label (production/staging/etc.)
- `priority`: Matches workflow's priority label (P0/P1/P2/P3)
- `business_category`: Matches workflow's business_category label
- `risk_tolerance`: Matches workflow's risk_tolerance label (low/medium/high)
**Technical Fields** (determined by YOUR investigation, may differ from input signal):
- `signal_type`: The actual issue type you identified (e.g., input says "HighMemory" but you determine root cause is "OOMKilled")
- `severity`: Your assessed severity based on investigation (may differ from input)
- `target_resource`: The specific resource that needs remediation, with full identification:
  - For namespaced resources: `namespace/kind/name` (e.g., "production/Deployment/payment-service")
  - For cluster resources: `kind/name` (e.g., "Node/worker-node-3")
  - Include API group if non-core: `namespace/group/kind/name` (e.g., "production/apps/StatefulSet/database")
- `component`: The resource type category (e.g., "deployment", "statefulset", "node", "pod")

**CRITICAL**: Use your RCA results for technical fields, not the input signal values. The input signal may be a symptom, but your investigation identifies the root cause and the actual resource that needs remediation.

**Example**: Input signal shows `pod/payment-service-abc123` with HighMemory, but your investigation determines the root cause is the `Deployment/payment-service` needs resource limit adjustment.



**Note**: The business context fields are metadata for workflow matching. When you call MCP workflow search tools after your RCA, pass these fields as parameters to filter and rank workflows. You do NOT need to factor them into your technical analysis.

---

### Section 3: Analysis Instructions

```markdown
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
```

**Purpose**: Guides LLM to perform independent investigation appropriate to the signal type

**Backing**:
- **BR-HAPI-001**: AI-Powered Investigation Endpoint (holmesgpt-api/BUSINESS_REQUIREMENTS.md)
- **HolmesGPT SDK**: Investigation workflow (holmes.core.investigation.investigate_issues)

**Note**: Investigation steps are NOT prescriptive - LLM determines appropriate investigation based on signal source and incident type.

## Your Investigation Workflow

**CRITICAL**: Follow this sequence in order. Do NOT search for workflows before investigating.

### Phase 1: Investigate the Incident

Use available tools to investigate the incident:
- Check pod status, events, and logs (kubectl)
- Review resource usage and limits
- Examine node conditions
- Analyze metrics from signal source (if prometheus-adapter)
- Gather evidence about what actually happened

**Goal**: Understand what actually happened and why.

**Input Signal Provided**: `{signal_type}` (starting point for investigation)

### Phase 2: Determine Root Cause (RCA)

Based on your investigation findings, identify the root cause:
- What is the underlying problem?
- Is the input signal the root cause, or just a symptom?
- What evidence supports your conclusion?

**Example**:
- Input: OOMKilled
- Investigation: Pod memory usage 580Mi, limit 512Mi, no other issues
- RCA: Root cause IS insufficient memory limit

### Phase 3: Identify Signal Type That Describes the Effect

Based on your RCA, determine the signal_type that best describes the effect:

**If investigation confirms input signal is the root cause**:
- Input: OOMKilled → Investigation confirms memory limit exceeded → Signal Type: "OOMKilled"

**If investigation reveals different root cause**:
- Input: OOMKilled → Investigation shows node memory pressure affecting multiple pods → Signal Type: "NodePressure" or "Evicted"
- Input: CrashLoopBackOff → Investigation shows image pull failure → Signal Type: "ImagePullBackOff"

**Important**: The signal_type for workflow search comes from YOUR investigation findings, not the input signal.

Also assess the actual severity based on business impact (may differ from input severity).

### Phase 4: Search for Workflow

Call MCP `search_workflow_catalog` tool with:
- **Query**: `"<YOUR_RCA_SIGNAL_TYPE> <YOUR_RCA_SEVERITY>"`
  - Example: "OOMKilled critical" (if RCA confirms OOM is root cause)
  - Example: "NodePressure high" (if RCA reveals node issue)
- **Label Filters**: Pass business context values (environment, priority, risk_tolerance, business_category)

The MCP search will return:
- `workflow_id`: Selected workflow identifier
- `version`: Workflow version
- `confidence`: Similarity score (0.0-1.0)
- `description`: What the workflow does
- `parameters`: JSON schema of required parameters

### Phase 5: Return Summary + JSON Payload

Provide your response in two parts:

**Part 1: Natural Language Summary**
- Explain your investigation findings
- Describe the root cause
- Explain why you selected this workflow
- Justify parameter values

**Part 2: Structured JSON**
```json
{
  "root_cause_analysis": {
    "summary": "Brief summary of root cause from investigation",
    "severity": "critical|high|medium|low",
    "contributing_factors": ["factor1", "factor2"]
  },
  "selected_workflow": {
    "workflow_id": "workflow-id-from-mcp-search",
    "version": "1.0.0",
    "confidence": 0.95,
    "rationale": "Why your RCA findings led to this workflow selection",
    "parameters": {
      "PARAM_NAME": "value-from-investigation",
      "ANOTHER_PARAM": "value-from-investigation"
    }
  }
}
```

**If MCP search fails or returns no workflows**:
```json
{
  "root_cause_analysis": { ... },
  "selected_workflow": null,
  "rationale": "MCP search failed: [error details]. RCA completed but workflow selection unavailable."
}
```

---

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

---

### Section 4: Output Format Specification

⚠️ **MVP VALIDATION CODE - NOT FINAL**

**MCP Tool Response Format**:
When you call `search_workflow_catalog`, the MCP tool returns workflow results with these fields:
- `workflow_id`: Unique identifier for the workflow
- `title`: Human-readable workflow name (for display only, not returned in LLM response)
- `description`: Detailed workflow description
- `parameters`: Array of parameter definitions (name, type, required, description)
- `similarity_score`: Semantic match score (0.0-1.0)
- `estimated_duration`: Expected execution time
- `success_rate`: Historical success rate (0.0-1.0)

Use the `workflow_id` and `title` from the MCP search results in your response.
⚠️ **NEEDS REVIEW**: This output format includes workflow_id and parameters for MVP testing.
Final v2.0 implementation: LLM calls MCP tools to search workflows AFTER RCA, not before.

```markdown
**OUTPUT FORMAT**: Respond with a JSON object containing your analysis:
{
  "analysis_summary": "Brief summary of root cause analysis",
  "root_cause_assessment": "Your assessment of the root cause based on investigation",
  "rca_severity": "critical|high|medium|low",  // Your assessed severity (may differ from input signal)
  "selected_workflow": {
    "workflow_id": "string",           // REQUIRED: Use workflow_id from MCP search_workflow_catalog results
    "confidence": 0.85,                // Your confidence this workflow addresses the root cause (0.0-1.0)
    "rationale": "Why this workflow is the best match for the identified root cause",
    "parameters": {                    // Populate parameters from workflow schema
      "NAMESPACE": "value",
      "DEPLOYMENT_NAME": "value",
      "TARGET_REPLICAS": "value"
      // ... other parameters as defined in workflow schema
    }
  },
  "warnings": ["warning1", "warning2"],  // Any concerns about the remediation
  "alternative_workflows": [           // Optional: other viable options with lower confidence
    {
      "workflow_id": "string",
      "confidence": 0.65,
      "rationale": "Why this is a secondary option"
    }
  ]
}
}
```
**CRITICAL REQUIREMENTS**:
1. **analysis_summary**: Brief overview of the issue and approach
2. **root_cause_assessment**: Your independent RCA findings
3. **remediation_recommendations**: Actionable remediation strategies
4. **confidence**: Your confidence level (0.0-1.0) in each strategy
5. **rationale**: Explain WHY this approach addresses the root cause
6. **estimated_risk**: Your assessment of risk (low/medium/high)

**ANALYSIS GUIDANCE**:
- Prioritize strategies by confidence and risk
- Consider root cause, not just symptoms
- Assess cluster resource availability
- Evaluate business impact and priority
- Base all recommendations on actual investigation findings
```

**Purpose**: Defines expected JSON response structure for MVP testing

**⚠️ v1.0 IMPLEMENTATION**: After LLM performs RCA and calls MCP tools, the response includes `workflow_id` and `parameters` fields populated by the LLM after searching the workflow catalog via MCP.

---

## LLM Response Format (Authoritative)

### Response Schema

```json
{
  "analysis_summary": "string",
  "root_cause_assessment": "string",
  "strategies": [
    {
      "workflow_id": "string (REQUIRED)",
      "version": "string (OPTIONAL, defaults to latest)",
      "confidence": "number (0.0-1.0, REQUIRED)",
      "rationale": "string (REQUIRED)",
      "estimated_risk": "enum[low|medium|high] (REQUIRED)",
      "parameters": {
        "PARAM_NAME": "value (type matches parameter schema)"
      }
    }
  ],
  "warnings": ["string"],
  "context_used": {
    "cluster_state": "string",
    "resource_availability": "string",
    "blast_radius": "string"
}
```

### Field Definitions

#### Root Level Fields

| Field | Type | Required | Description | Source |
|-------|------|----------|-------------|--------|
| `analysis_summary` | string | Yes | Brief summary of failure and approach | LLM-generated |
| `root_cause_assessment` | string | Yes | LLM's RCA conclusion | LLM-generated |
| `strategies` | array | Yes | Remediation strategies (1+ items) | LLM-generated |
| `warnings` | array | No | LLM-identified risks/warnings | LLM-generated |
| `context_used` | object | Yes | Context LLM considered | LLM-generated |

#### Strategy Object Fields

| Field | Type | Required | Description | Validation |
|-------|------|----------|-------------|------------|
| `workflow_id` | string | Yes | Selected workflow ID | Must match provided workflows |
| `version` | string | No | Workflow version | Must match provided version, defaults to latest |
| `confidence` | number | Yes | LLM confidence (0.0-1.0) | 0.0 ≤ value ≤ 1.0 |
| `rationale` | string | Yes | Why this workflow is appropriate | Non-empty string |
| `estimated_risk` | enum | Yes | Risk assessment | "low", "medium", or "high" |
| `parameters` | object | Conditional | Populated parameter values | Required if workflow has parameters |

#### Parameters Object

**Structure**: Flat key-value pairs matching workflow parameter schema

**Example**:
```json
{
  "TARGET_RESOURCE_KIND": "Deployment",
  "TARGET_RESOURCE_NAME": "my-app",
  "TARGET_NAMESPACE": "production",
  "SCALE_TARGET_REPLICAS": 2
}
```

**Validation Rules**:
1. All required parameters MUST be present
2. Parameter names MUST match exactly (case-sensitive)
3. Parameter types MUST match schema (string, integer, boolean, etc.)
4. Enum values MUST be from allowed list
5. Integer values MUST respect min/max constraints
6. String values MUST match regex pattern if specified

#### Context Used Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cluster_state` | string | Yes | Assessment of cluster health |
| `resource_availability` | string | Yes | Assessment of resources |
| `blast_radius` | string | Yes | Potential impact scope |

---

## Removed Fields (Not in Response)

The following fields are **NOT** included in the LLM response because they are already defined in the workflow:

| Removed Field | Reason | Where It Exists |
|---------------|--------|-----------------|
| `action_type` | Redundant with workflow_id | Workflow definition |
| `prerequisites` | Workflow metadata | Workflow content |
| `steps` | Workflow metadata | Workflow content |
| `expected_outcome` | Workflow metadata | Workflow content |
| `rollback_plan` | Workflow metadata | Workflow content |

**Rationale**: The workflow is the single source of truth for execution details. The LLM's responsibility is:
1. Select the appropriate workflow
2. Provide rationale for selection
3. Assess risk
4. Populate parameter values based on investigation

---

## Response Validation

### Parser Requirements

The response parser (`holmesgpt-api/src/extensions/recovery.py`) MUST validate:

1. **JSON Structure**: Response is valid JSON matching schema
2. **Required Fields**: All required fields are present
3. **Workflow ID**: Matches one of the provided workflows
4. **Version**: Matches provided version (if specified)
5. **Confidence**: Value between 0.0 and 1.0
6. **Risk Level**: One of "low", "medium", "high"
7. **Parameters**: All required parameters present and valid

### Validation Failures

If validation fails, the parser MUST:
1. Log the validation error with details
2. Return error response to caller
3. NOT attempt to execute the workflow
4. Include original LLM response in error details for debugging

---

## Examples

### Example 1: OOMKilled Remediation

**Prompt** (abbreviated):
```markdown
## Failed Action
- Type: pod_restart
- Target: deployment/my-app
- Namespace: production

## Failure Context
- Error: OOMKilled
- Error Message: Container killed due to OOM

## Recommended Workflows (3 found)

### Workflow 1: OOMKill Remediation - Scale Down
- **ID**: oomkill-scale-down
- **Version**: 1.0.0
- **Description**: Reduces replica count for deployments experiencing OOMKilled

**Parameters for OOMKill Remediation - Scale Down (oomkill-scale-down)**:
- `TARGET_RESOURCE_KIND` (string, **REQUIRED**): Kubernetes resource kind
  - Allowed values: ['Deployment', 'StatefulSet', 'DaemonSet']
- `TARGET_RESOURCE_NAME` (string, **REQUIRED**): Name of the resource
- `TARGET_NAMESPACE` (string, **REQUIRED**): Kubernetes namespace
- `SCALE_TARGET_REPLICAS` (integer, **REQUIRED**): Target replica count
  - Constraints: min: 0, max: 100
```

**Expected Response**:
```json
{
  "analysis_summary": "Deployment my-app in production namespace experiencing OOMKilled events. Node worker-2 shows 95% memory pressure. Recommend scaling down to reduce memory pressure.",
  "root_cause_assessment": "Node memory exhaustion due to multiple pods on worker-2. Application can tolerate reduced capacity.",
  "strategies": [
    {
      "workflow_id": "oomkill-scale-down",
      "version": "1.0.0",
      "confidence": 0.85,
      "rationale": "Node worker-2 is at 95% memory capacity with multiple OOMKilled events. Scaling down my-app from 5 to 3 replicas will reduce memory pressure while maintaining service availability based on current traffic patterns.",
      "estimated_risk": "low",
      "parameters": {
        "TARGET_RESOURCE_KIND": "Deployment",
        "TARGET_RESOURCE_NAME": "my-app",
        "TARGET_NAMESPACE": "production",
        "SCALE_TARGET_REPLICAS": 3
      }
    }
  ],
  "warnings": [
    "Scaling down will reduce capacity by 40%",
    "Monitor traffic patterns after scaling",
    "Consider node memory upgrade if issue persists"
  ],
  "context_used": {
    "cluster_state": "Node worker-2 at 95% memory, 3 pods OOMKilled in last 10 minutes",
    "resource_availability": "Other nodes have capacity, but pod affinity rules restrict placement",
    "blast_radius": "Single deployment, estimated 40% capacity reduction, low customer impact based on current traffic"
}
```

---

### Example 2: Multiple Strategy Options

**Expected Response**:
```json
{
  "analysis_summary": "Deployment my-app experiencing OOMKilled. Two viable strategies identified.",
  "root_cause_assessment": "Application memory leak combined with insufficient memory limits.",
  "strategies": [
    {
      "workflow_id": "oomkill-increase-memory",
      "version": "1.0.0",
      "confidence": 0.90,
      "rationale": "Memory usage consistently at limit. Application legitimately needs more memory based on workload analysis.",
      "estimated_risk": "low",
      "parameters": {
        "TARGET_RESOURCE_KIND": "Deployment",
        "TARGET_RESOURCE_NAME": "my-app",
        "TARGET_NAMESPACE": "production",
        "MEMORY_LIMIT_NEW": "2Gi"
      }
    },
    {
      "workflow_id": "oomkill-optimize-application",
      "version": "1.0.0",
      "confidence": 0.75,
      "rationale": "Memory leak detected in application logs. Restart may temporarily resolve issue.",
      "estimated_risk": "medium",
      "parameters": {
        "TARGET_RESOURCE_KIND": "Deployment",
        "TARGET_RESOURCE_NAME": "my-app",
        "TARGET_NAMESPACE": "production"
      }
    }
  ],
  "warnings": [
    "Increasing memory is temporary fix if memory leak exists",
    "Application restart will cause brief service interruption"
  ],
  "context_used": {
    "cluster_state": "Cluster healthy, sufficient memory available",
    "resource_availability": "Node has 8Gi available, can accommodate increase",
    "blast_radius": "Single deployment, brief interruption acceptable for P2 service"
}
```

---

## Version Compatibility

### v1.0 (Current)
- Workflows manually inserted via SQL (DD-STORAGE-008 v1.2)
- Parameters manually defined in database
- Pre-fetch workflows and include in prompt
- Response format as defined in this ADR

### v1.1 (Planned)
- Workflows registered via CRD (DD-WORKFLOW-008)
- Parameters extracted from container images
- Same prompt/response format
- Enhanced validation (schema extraction)

### v2.0 (Future)
- LLM calls MCP tools to search workflows (DD-WORKFLOW-002)
- Dynamic workflow discovery
- Prompt structure changes (no pre-fetched workflows)
- Response format remains compatible

---

## Implementation Notes

### Prompt Generation

**File**: `holmesgpt-api/src/extensions/recovery.py`
**Function**: `_create_investigation_prompt(request_data: Dict[str, Any]) -> str`

**Requirements**:
1. Follow prompt structure defined in this ADR
2. Include all workflow fields from DD-STORAGE-008 v1.2
3. Display parameter schemas from DD-WORKFLOW-003 v2.2
4. Handle missing/optional fields gracefully

### Response Parsing

**File**: `holmesgpt-api/src/extensions/recovery.py`
**Function**: `_parse_recovery_response(response: str) -> Dict[str, Any]`

**Requirements**:
1. Validate JSON structure
2. Validate all required fields
3. Validate workflow_id against provided workflows
4. Validate parameter values against schema
5. Return detailed error on validation failure

### Database Schema Alignment

**Table**: `workflow_catalog` (DD-STORAGE-008 v1.2)

**Required Fields for Prompt**:
- `workflow_id` (VARCHAR 255)
- `version` (VARCHAR 50)
- `name` (VARCHAR 255)
- `description` (TEXT)
- `labels` (JSONB) - contains signal_type, severity, component, etc.
- `parameters` (JSONB) - parameter schema array
- `content` (TEXT) - contains steps (Tekton Task YAML)

---

## Consequences

### Positive

1. **Single Source of Truth**: One document defines prompt and response contract
2. **Validation**: Clear validation rules for response parsing
3. **Maintainability**: Easier to update both prompt and response together
4. **Alignment**: Ensures prompt and response stay in sync
5. **Documentation**: Clear examples for developers and LLM integration
6. **Version Control**: Track changes to contract over time

### Negative

1. **Coupling**: Prompt and response tightly coupled in one document
2. **Size**: Large ADR covering multiple concerns
3. **Updates**: Changes require updating entire contract

### Mitigations

1. Use version control to track contract evolution
2. Reference this ADR from implementation files
3. Update mock MCP server to match this contract
4. Add integration tests validating contract compliance

---

## Related Documents

- **DD-STORAGE-008 v1.2**: Workflow Catalog Schema (defines database fields)
- **DD-WORKFLOW-003 v2.2**: Parameterized Remediation Actions (defines parameter schema)
- **DD-WORKFLOW-002 v1.0**: MCP Workflow Catalog Architecture (defines v2.0 flow)
- **DD-WORKFLOW-008**: Version Roadmap (defines v1.0 vs v1.1 features)
- **BR-WORKFLOW-001**: Workflow Registry Management (defines business requirements)

---

## Changelog

### Version 3.3 (2025-11-16)
- **Critical Enhancement**: Added explicit 5-phase investigation workflow sequence
- Clarifies that LLM must investigate FIRST, then determine RCA signal_type based on findings
- Signal_type for workflow search comes from investigation, not input signal
- Phases: 1) Investigate → 2) Determine RCA → 3) Identify signal_type → 4) Search workflow → 5) Return results
- Rationale: Ensures LLM follows correct sequence and understands signal_type determination is based on investigation findings

### Version 3.2 (2025-11-16)
- **Enhancement**: Added signal source to Incident Summary natural language narrative
- Rationale: Provides immediate context about alert source (prometheus-adapter vs kubernetes-event-adapter) for better investigation strategy selection
- Example: "A **critical OOMKilled event** from **prometheus-adapter** has occurred..."
- Signal source remains in Cluster Context section for structured reference
- Improves LLM's ability to choose appropriate investigation tools earlier

### Version 3.1 (2025-11-16)
- **CRITICAL FIX**: Corrected deduplication explanation - it tracks duplicate alerts from monitoring system, NOT failed remediation attempts
- Clarified that occurrence_count > 1 means the condition is persistent/ongoing, not that remediation failed
- Added "What Deduplication Means" section explaining 5-minute window and Prometheus alert firing behavior
- Removed incorrect "Previous Remediation" field reference

### Version 3.0 (2025-11-16)
- **MAJOR UPDATE**: Changed prompt format from structured bullet lists to hybrid natural language + structured format
- Added "Incident Summary" section with natural language narrative for better LLM comprehension
- Added "Business Impact Assessment" with contextual descriptions (e.g., "P0 (highest priority)")
- Added dynamic risk tolerance guidance (e.g., "low (conservative remediation required)")
- Removed "(FOR RCA INVESTIGATION)" and "(FOR WORKFLOW SEARCH)" labels - LLM uses all context for both
- Merged "Signal Information" and "Failure Context" into cohesive sections
- Rationale: Natural language improves LLM understanding while maintaining structured data for precision


### Version 2.0 (2025-11-16) - Major Update
- **BREAKING**: Added RCA severity assessment (critical, high, medium, low)
- Added comprehensive severity assessment criteria with examples
- Added context-aware assessment factors (user impact, environment, business impact, escalation risk, data risk)
- Clarified separation of input signal severity vs RCA severity
- Added industry best practices (PagerDuty, AWS, Google SRE patterns)
- Documented that RCA severity drives remediation decisions, not input signal severity
- Added severity assessment rationale requirement
- Aligned with industry standard 4-level severity model

### Version 1.5 (2025-11-15)
- Corrected implementation note: workflow_id and parameters are v1.0 functionality, not v2.0
- Clarified that LLM calls MCP tools in v1.0 (not a future feature)

### Version 1.4 (2025-11-15)
- Added MCP Tool Response Format section documenting fields returned by search_workflow_catalog
- Clarified that workflow_id and title come from MCP search results
- Updated comments to explicitly state "Use workflow_id from MCP search_workflow_catalog results"

### Version 1.3 (2025-11-15)
- Added explicit target_resource field for specific resource identification (namespace/kind/name format)
- Clarified resource identification for namespaced vs cluster resources
- Added API group support for non-core resources
- Added concrete example of symptom (pod) vs root cause (deployment)

### Version 1.2 (2025-11-15)
- Clarified field usage for workflow search: Environment/context fields use prompt values as-is, technical fields use LLM investigation results
- Added critical note that LLM should use RCA results for signal_type, severity, component (not input signal values)
- Emphasized that input signal may be symptom while investigation identifies root cause

### Version 1.1 (2025-11-15) - Part 2
- Fixed response format: Changed from generic "remediation_recommendations" to "selected_workflow" with single highest-confidence workflow
- Added "alternative_workflows" field for secondary options
- Aligned response format with single-workflow selection pattern

### Version 1.1 (2025-11-15) - Part 1
- Added deduplication context fields (is_duplicate, first_seen, last_seen, occurrence_count, previous_remediation_ref)
- Added storm detection fields (is_storm, storm_type, storm_window, storm_alert_count, affected_resources)
- Added RemediationRequestSpec backing reference
- Updated field usage list to include deduplication and storm detection

### Version 1.0 (2025-11-16)
- Initial ADR creation
- Defined prompt structure for v1.0 MVP
- Defined response format aligned with DD-STORAGE-008 v1.2
- Removed redundant workflow metadata fields from response
- Added comprehensive examples
- Added validation requirements

---

**Status**: Proposed
**Next Steps**: Review and approval by architecture team

