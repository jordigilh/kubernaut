# ADR-038: LLM Prompt and Response Contract for Playbook Selection

**Status**: Proposed
**Date**: 2025-11-16
**Deciders**: Architecture Team
**Related**: DD-PLAYBOOK-003, DD-STORAGE-008, DD-PLAYBOOK-002, BR-PLAYBOOK-001
**Version**: 2.0

---

## Context

The HolmesGPT API sends prompts to the LLM for Root Cause Analysis (RCA) and remediation playbook selection. The LLM must understand the prompt structure and return a structured JSON response that the system can parse and execute.

### Problem

Without a single authoritative definition of the prompt/response contract:
- Prompt structure and response format can drift out of sync
- Multiple documents define pieces of the contract (recovery.py, DD-PLAYBOOK-003, etc.)
- No single source of truth for validation
- Difficult to maintain consistency across v1.0, v1.1, v2.0

### Requirements

1. Define the complete LLM prompt structure
2. Define the expected JSON response format
3. Ensure alignment with DD-STORAGE-008 v1.2 (playbook catalog schema)
4. Ensure alignment with DD-PLAYBOOK-003 v2.2 (parameterized actions)
5. Support v1.0 MVP testing and production deployment

---

## Decision

**Create a single authoritative ADR defining the LLM prompt structure and expected response format for playbook selection.**

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
- The LLM selects playbooks and populates parameters based on its RCA

---


## LLM Prompt Structure

### Section 1: Incident Context (Observable Facts Only)

```markdown
# Investigation Request

## Signal Information (FOR RCA INVESTIGATION)
- Signal Type: {signal_type}           # e.g., "OOMKilled", "CrashLoopBackOff" (DD-PLAYBOOK-001)
- Severity: {severity}                  # e.g., "critical", "high", "medium", "low" (DD-PLAYBOOK-001)
- Component: {component}                # e.g., "pod", "deployment", "node" (DD-PLAYBOOK-001)
- Alert Name: {alert_name}              # Human-readable name from source
- Namespace: {namespace}                # Kubernetes namespace
- Resource: {resource_kind}/{resource_name}  # e.g., "deployment/my-app"

## Error Details (FOR RCA INVESTIGATION)
- Error Message: {error_message}        # From signal annotations
- Description: {description}            # From signal annotations
- Firing Time: {firing_time}            # When signal started
- Received Time: {received_time}        # When Gateway received signal

## Deduplication Context (FOR RCA INVESTIGATION)
- Is Duplicate: {is_duplicate}          # True if this signal has been seen before
- First Seen: {first_seen}              # When this signal fingerprint was first seen
- Last Seen: {last_seen}                # When this signal fingerprint was last seen
- Occurrence Count: {occurrence_count}  # Total count of occurrences (deduplication count)
- Previous Remediation: {previous_remediation_ref}  # Reference to previous RemediationRequest (if duplicate)

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
These fields are used by MCP playbook search tools to match playbooks.
You do NOT need to consider these in your RCA analysis.

- Environment: {environment}            # e.g., "production", "staging" (for playbook matching)
- Priority: {priority}                  # e.g., "P0", "P1", "P2", "P3" (for playbook matching)
- Business Category: {business_category}  # e.g., "payment-service" (for playbook matching)
- Risk Tolerance: {risk_tolerance}      # e.g., "low", "medium", "high" (for playbook matching)

**Note**: When you call MCP playbook search tools (e.g., `search_playbook_catalog`), you must
pass these business context fields as parameters when calling playbook search tools.
```

**Purpose**: Provides observable facts from the signal for LLM investigation

**Backing**:
- **DD-PLAYBOOK-001**: Mandatory Playbook Label Schema (7 labels)
- **DD-CATEGORIZATION-001**: Gateway vs Signal Processing Categorization Split
- **NormalizedSignal** (pkg/gateway/types/types.go): Gateway output structure
- **RemediationProcessingSpec** (api/remediationprocessing/v1alpha1): Signal Processing output
- **RemediationRequestSpec** (api/remediation/v1alpha1): Gateway output with deduplication and storm detection

**CRITICAL PRINCIPLE**: This section contains ONLY observable facts. NO pre-analysis, NO playbook recommendations, NO root cause assessment, NO historical patterns that could contaminate the LLM's independent RCA.

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
**For Playbook Search** (pass these to MCP tools):

**Environment/Context Fields** (use as-is from prompt):
- `environment`: Matches playbook's environment label (production/staging/etc.)
- `priority`: Matches playbook's priority label (P0/P1/P2/P3)
- `business_category`: Matches playbook's business_category label
- `risk_tolerance`: Matches playbook's risk_tolerance label (low/medium/high)
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



**Note**: The business context fields are metadata for playbook matching. When you call MCP playbook search tools after your RCA, pass these fields as parameters to filter and rank playbooks. You do NOT need to factor them into your technical analysis.

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
When you call `search_playbook_catalog`, the MCP tool returns playbook results with these fields:
- `playbook_id`: Unique identifier for the playbook
- `title`: Human-readable playbook name (for display only, not returned in LLM response)
- `description`: Detailed playbook description
- `parameters`: Array of parameter definitions (name, type, required, description)
- `similarity_score`: Semantic match score (0.0-1.0)
- `estimated_duration`: Expected execution time
- `success_rate`: Historical success rate (0.0-1.0)

Use the `playbook_id` and `title` from the MCP search results in your response.
⚠️ **NEEDS REVIEW**: This output format includes playbook_id and parameters for MVP testing.
Final v2.0 implementation: LLM calls MCP tools to search playbooks AFTER RCA, not before.

```markdown
**OUTPUT FORMAT**: Respond with a JSON object containing your analysis:
{
  "analysis_summary": "Brief summary of root cause analysis",
  "root_cause_assessment": "Your assessment of the root cause based on investigation",
  "rca_severity": "critical|high|medium|low",  // Your assessed severity (may differ from input signal)
  "selected_playbook": {
    "playbook_id": "string",           // REQUIRED: Use playbook_id from MCP search_playbook_catalog results
    "confidence": 0.85,                // Your confidence this playbook addresses the root cause (0.0-1.0)
    "rationale": "Why this playbook is the best match for the identified root cause",
    "parameters": {                    // Populate parameters from playbook schema
      "NAMESPACE": "value",
      "DEPLOYMENT_NAME": "value",
      "TARGET_REPLICAS": "value"
      // ... other parameters as defined in playbook schema
    }
  },
  "warnings": ["warning1", "warning2"],  // Any concerns about the remediation
  "alternative_playbooks": [           // Optional: other viable options with lower confidence
    {
      "playbook_id": "string",
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

**⚠️ v1.0 IMPLEMENTATION**: After LLM performs RCA and calls MCP tools, the response includes `playbook_id` and `parameters` fields populated by the LLM after searching the playbook catalog via MCP.

---

## LLM Response Format (Authoritative)

### Response Schema

```json
{
  "analysis_summary": "string",
  "root_cause_assessment": "string",
  "strategies": [
    {
      "playbook_id": "string (REQUIRED)",
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
| `playbook_id` | string | Yes | Selected playbook ID | Must match provided playbooks |
| `version` | string | No | Playbook version | Must match provided version, defaults to latest |
| `confidence` | number | Yes | LLM confidence (0.0-1.0) | 0.0 ≤ value ≤ 1.0 |
| `rationale` | string | Yes | Why this playbook is appropriate | Non-empty string |
| `estimated_risk` | enum | Yes | Risk assessment | "low", "medium", or "high" |
| `parameters` | object | Conditional | Populated parameter values | Required if playbook has parameters |

#### Parameters Object

**Structure**: Flat key-value pairs matching playbook parameter schema

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

The following fields are **NOT** included in the LLM response because they are already defined in the playbook:

| Removed Field | Reason | Where It Exists |
|---------------|--------|-----------------|
| `action_type` | Redundant with playbook_id | Playbook definition |
| `prerequisites` | Playbook metadata | Playbook content |
| `steps` | Playbook metadata | Playbook content |
| `expected_outcome` | Playbook metadata | Playbook content |
| `rollback_plan` | Playbook metadata | Playbook content |

**Rationale**: The playbook is the single source of truth for execution details. The LLM's responsibility is:
1. Select the appropriate playbook
2. Provide rationale for selection
3. Assess risk
4. Populate parameter values based on investigation

---

## Response Validation

### Parser Requirements

The response parser (`holmesgpt-api/src/extensions/recovery.py`) MUST validate:

1. **JSON Structure**: Response is valid JSON matching schema
2. **Required Fields**: All required fields are present
3. **Playbook ID**: Matches one of the provided playbooks
4. **Version**: Matches provided version (if specified)
5. **Confidence**: Value between 0.0 and 1.0
6. **Risk Level**: One of "low", "medium", "high"
7. **Parameters**: All required parameters present and valid

### Validation Failures

If validation fails, the parser MUST:
1. Log the validation error with details
2. Return error response to caller
3. NOT attempt to execute the playbook
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

## Recommended Playbooks (3 found)

### Playbook 1: OOMKill Remediation - Scale Down
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
      "playbook_id": "oomkill-scale-down",
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
      "playbook_id": "oomkill-increase-memory",
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
      "playbook_id": "oomkill-optimize-application",
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
- Playbooks manually inserted via SQL (DD-STORAGE-008 v1.2)
- Parameters manually defined in database
- Pre-fetch playbooks and include in prompt
- Response format as defined in this ADR

### v1.1 (Planned)
- Playbooks registered via CRD (DD-PLAYBOOK-008)
- Parameters extracted from container images
- Same prompt/response format
- Enhanced validation (schema extraction)

### v2.0 (Future)
- LLM calls MCP tools to search playbooks (DD-PLAYBOOK-002)
- Dynamic playbook discovery
- Prompt structure changes (no pre-fetched playbooks)
- Response format remains compatible

---

## Implementation Notes

### Prompt Generation

**File**: `holmesgpt-api/src/extensions/recovery.py`
**Function**: `_create_investigation_prompt(request_data: Dict[str, Any]) -> str`

**Requirements**:
1. Follow prompt structure defined in this ADR
2. Include all playbook fields from DD-STORAGE-008 v1.2
3. Display parameter schemas from DD-PLAYBOOK-003 v2.2
4. Handle missing/optional fields gracefully

### Response Parsing

**File**: `holmesgpt-api/src/extensions/recovery.py`
**Function**: `_parse_recovery_response(response: str) -> Dict[str, Any]`

**Requirements**:
1. Validate JSON structure
2. Validate all required fields
3. Validate playbook_id against provided playbooks
4. Validate parameter values against schema
5. Return detailed error on validation failure

### Database Schema Alignment

**Table**: `playbook_catalog` (DD-STORAGE-008 v1.2)

**Required Fields for Prompt**:
- `playbook_id` (VARCHAR 255)
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

- **DD-STORAGE-008 v1.2**: Playbook Catalog Schema (defines database fields)
- **DD-PLAYBOOK-003 v2.2**: Parameterized Remediation Actions (defines parameter schema)
- **DD-PLAYBOOK-002 v1.0**: MCP Playbook Catalog Architecture (defines v2.0 flow)
- **DD-PLAYBOOK-008**: Version Roadmap (defines v1.0 vs v1.1 features)
- **BR-PLAYBOOK-001**: Playbook Registry Management (defines business requirements)

---

## Changelog

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
- Corrected implementation note: playbook_id and parameters are v1.0 functionality, not v2.0
- Clarified that LLM calls MCP tools in v1.0 (not a future feature)

### Version 1.4 (2025-11-15)
- Added MCP Tool Response Format section documenting fields returned by search_playbook_catalog
- Clarified that playbook_id and title come from MCP search results
- Updated comments to explicitly state "Use playbook_id from MCP search_playbook_catalog results"

### Version 1.3 (2025-11-15)
- Added explicit target_resource field for specific resource identification (namespace/kind/name format)
- Clarified resource identification for namespaced vs cluster resources
- Added API group support for non-core resources
- Added concrete example of symptom (pod) vs root cause (deployment)

### Version 1.2 (2025-11-15)
- Clarified field usage for playbook search: Environment/context fields use prompt values as-is, technical fields use LLM investigation results
- Added critical note that LLM should use RCA results for signal_type, severity, component (not input signal values)
- Emphasized that input signal may be symptom while investigation identifies root cause

### Version 1.1 (2025-11-15) - Part 2
- Fixed response format: Changed from generic "remediation_recommendations" to "selected_playbook" with single highest-confidence playbook
- Added "alternative_playbooks" field for secondary options
- Aligned response format with single-playbook selection pattern

### Version 1.1 (2025-11-15) - Part 1
- Added deduplication context fields (is_duplicate, first_seen, last_seen, occurrence_count, previous_remediation_ref)
- Added storm detection fields (is_storm, storm_type, storm_window, storm_alert_count, affected_resources)
- Added RemediationRequestSpec backing reference
- Updated field usage list to include deduplication and storm detection

### Version 1.0 (2025-11-16)
- Initial ADR creation
- Defined prompt structure for v1.0 MVP
- Defined response format aligned with DD-STORAGE-008 v1.2
- Removed redundant playbook metadata fields from response
- Added comprehensive examples
- Added validation requirements

---

**Status**: Proposed
**Next Steps**: Review and approval by architecture team

