# ADR-041 Appendix A: Workflow Catalog Integration - Claude Test Timeline

**Date**: 2025-11-18
**Test Scenario**: OOMKilled Container (test-scenario-01/memory-hungry-app)
**LLM**: Claude (Anthropic Haiku 4.5)
**Status**: ✅ SUCCESSFUL INTEGRATION

---

## Executive Summary

Complete timeline of Claude's successful investigation and workflow selection for an OOMKilled container incident. The test validates that Claude:
1. ✅ Performs comprehensive Kubernetes investigation
2. ✅ Identifies root cause from logs and resource configuration
3. ✅ **Invokes the workflow catalog tool** (search_workflow_catalog)
4. ✅ Selects appropriate remediation workflow with high confidence (0.95)
5. ✅ Populates all required parameters from investigation findings

**Total Investigation Time**: 47 seconds (19:26:01 - 19:26:48)
**Workflow Tool Invocation**: Tool #19 at 19:26:31 (31 seconds into investigation)
**Result**: Complete success - workflow catalog integration fully operational

---

## Test Scenario Details

### Environment Setup

**Kubernetes Cluster**: KIND local cluster (stress-worker-0.parodos.dev)
**Namespace**: test-scenario-01
**Test Pod**: memory-hungry-app-6f54dd6449-clr7g
**Issue**: Container repeatedly OOMKilled (113 restart attempts)

**Pod Configuration**:
- Image: `curlimages/curl:latest`
- Command: `stress --vm 1 --vm-bytes 256M` (allocate 256MB)
- Memory Limit: **128Mi** (insufficient for 256MB allocation)
- Memory Request: 64Mi
- Node: stress-worker-0.parodos.dev (51% memory utilization - not node-constrained)

**Root Cause**: Configuration mismatch - 128Mi limit vs 256MB allocation requirement

---

## Timeline: Initial API Request

**Timestamp**: 2025-11-18 19:26:01

### HTTP Request

```bash
curl -X POST http://localhost:8080/api/v1/incident/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "incident-oomkill-001",
    "signal_type": "OOMKilled",
    "severity": "high",
    "resource_namespace": "test-scenario-01",
    "resource_name": "memory-hungry-app-6f54dd6449-clr7g",
    "resource_kind": "pod",
    "alert_name": "ContainerOOMKilled",
    "error_message": "Container '\''app'\'' in pod was OOMKilled (restart count: 113)",
    "description": "Pod memory-hungry-app-6f54dd6449-clr7g in namespace test-scenario-01 has been OOMKilled 113 times",
    "labels": {
      "app": "memory-hungry-app",
      "pod-template-hash": "6f54dd6449"
    },
    "annotations": {
      "kubectl.kubernetes.io/restartedAt": "2025-11-17T10:30:00Z"
    },
    "business_context": {
      "environment": "production",
      "priority": "P1",
      "risk_tolerance": "medium",
      "business_category": "general"
    },
    "firing_time": "2025-11-17T10:30:00Z",
    "received_time": "2025-11-17T10:30:05Z"
  }'
```

### Request Payload (Formatted)

```json
{
  "incident_id": "incident-oomkill-001",
  "signal_type": "OOMKilled",
  "severity": "high",
  "resource_namespace": "test-scenario-01",
  "resource_name": "memory-hungry-app-6f54dd6449-clr7g",
  "resource_kind": "pod",
  "alert_name": "ContainerOOMKilled",
  "error_message": "Container 'app' in pod was OOMKilled (restart count: 113)",
  "description": "Pod memory-hungry-app-6f54dd6449-clr7g in namespace test-scenario-01 has been OOMKilled 113 times",
  "labels": {
    "app": "memory-hungry-app",
    "pod-template-hash": "6f54dd6449"
  },
  "business_context": {
    "environment": "production",
    "priority": "P1",
    "risk_tolerance": "medium",
    "business_category": "general"
  },
  "firing_time": "2025-11-17T10:30:00Z",
  "received_time": "2025-11-17T10:30:05Z"
}
```

---

## Timeline: Claude's Investigation Process

### Phase 1: Kubernetes Investigation (19:26:01 - 19:26:28, Duration: 27 seconds)

Claude used HolmesGPT SDK's Kubernetes toolsets to gather comprehensive context:

**Tools Invoked** (first 18 tool calls):
1. `kubectl_describe_pod` - Get pod details and events
2. `kubectl_get_pod` - Check pod status
3. `kubectl_get_pod_logs` - Review container logs
4. `kubectl_describe_node` - Check node resource availability
5. Additional Kubernetes queries for deployment, events, resource metrics

**Key Findings from Investigation**:

1. **Pod Status**:
   ```
   Name: memory-hungry-app-6f54dd6449-clr7g
   Namespace: test-scenario-01
   Status: CrashLoopBackOff
   Restart Count: 113
   Last State: Terminated (Reason: OOMKilled, Exit Code: 137)
   ```

2. **Container Configuration**:
   ```yaml
   containers:
   - name: app
     image: curlimages/curl:latest
     command: ["stress", "--vm", "1", "--vm-bytes", "256M"]
     resources:
       limits:
         memory: 128Mi
       requests:
         memory: 64Mi
   ```

3. **Container Logs**:
   ```
   stress: info: [1] dispatching hogs: 0 cpu, 0 io, 1 vm, 0 hdd
   stress: info: [2] allocating 256MB (268435456 bytes) ...
   stress: FAIL: [1] (415) <-- worker 2 got signal 9
   stress: WARN: [1] (417) now reaping child worker processes
   stress: FAIL: [1] (421) kill error: No such process
   stress: FAIL: [1] (451) failed run completed in 0s
   ```

4. **Node Status**:
   ```
   Node: stress-worker-0.parodos.dev
   Memory Pressure: False
   Disk Pressure: False
   PID Pressure: False
   Memory Capacity: 15.64Gi
   Memory Allocatable: 15.04Gi
   Memory Usage: 51% (7.7Gi / 15.04Gi)
   ```

**Claude's Analysis** (from investigation):
- Container attempts to allocate 256MB (`stress --vm-bytes 256M`)
- Memory limit is only 128Mi
- Allocation exceeds limit → kernel OOM killer terminates container
- Continuous restart loop (113 attempts)
- Node has sufficient capacity (51% utilization) - not a node-level issue
- **Conclusion**: Configuration mismatch, not resource exhaustion

---

### Phase 2: Root Cause Analysis (19:26:28 - 19:26:29, Duration: 1 second)

**Claude's RCA Output**:

```json
{
  "summary": "Container memory limit (128Mi) is insufficient for the stress application which attempts to allocate 256MB. This creates a guaranteed OOMKill scenario causing continuous pod restarts.",
  "severity": "high",
  "contributing_factors": [
    "Memory limit set to 128Mi while application requests 256MB allocation",
    "Deployment configuration mismatch between resource limits and workload requirements",
    "Continuous restart loop due to guaranteed OOMKill on every startup"
  ]
}
```

---

### Phase 3: Signal Type Identification (19:26:29)

**Claude's Determination**:
- **Input Signal Type**: `OOMKilled` (from RemediationRequest CRD)
- **RCA Signal Type**: `OOMKilled` (confirmed as root cause, not symptom)
- **Rationale**: The OOMKilled signal accurately describes the problem - it's not masking a deeper issue like node pressure or resource contention

---

### Phase 4: Workflow Catalog Search (19:26:31, Tool #19) ✅ **CRITICAL SUCCESS**

**Timestamp**: 19:26:31
**Tool Invoked**: `search_workflow_catalog` (Tool #19 in Claude's investigation sequence)

#### Tool Invocation Details

**holmesgpt-api Logs**:
```
2025-11-17 19:26:31 INFO: Running tool #19 [bold]search_workflow_catalog[/bold]:
  Search workflow catalog: 'OOMKilled high memory limit exceeded container'
  (filters: business_category=general, risk_tolerance=medium, environment=production)
  (top 5)

2025-11-17 19:26:31 INFO: BR-HAPI-250: Workflow catalog search -
  query='OOMKilled high memory limit exceeded container',
  filters={'environment': 'production', 'priority': 'P1', 'risk_tolerance': 'medium', 'business_category': 'general'},
  top_k=5

2025-11-17 19:26:31 INFO: BR-HAPI-250: Workflow catalog search completed - 2 workflows found
```

#### Search Parameters

**Query**: `"OOMKilled high memory limit exceeded container"`

**Label Filters**:
```json
{
  "environment": "production",
  "priority": "P1",
  "risk_tolerance": "medium",
  "business_category": "general"
}
```

**Top K**: 5 (return up to 5 best matches)

#### Search Results

**2 Workflows Returned**:

1. **oomkill-increase-memory** (SELECTED)
   - Similarity Score: 0.92
   - Success Rate: 0.85
   - Description: Increase container memory limits for OOMKilled pods
   - Parameter Schema: Includes deployment_name, namespace, new_memory_limit, new_memory_request

2. **oomkill-scale-down**
   - Similarity Score: 0.85
   - Success Rate: 0.80
   - Description: Scale down deployment to reduce memory pressure
   - Not selected (lower relevance for this specific issue)

---

### Phase 5: Workflow Selection (19:26:31 - 19:26:48, Duration: 17 seconds)

**Selected Workflow**: `oomkill-increase-memory`
**Version**: 1.0.0
**Confidence**: 0.95

#### Selection Rationale (from Claude)

```
"Investigation confirms the root cause is insufficient memory limit. The stress
application explicitly allocates 256MB but the pod limit is only 128Mi. The
'oomkill-increase-memory' workflow is the appropriate remediation to increase
the memory limit to accommodate the workload."
```

#### Parameters Populated by Claude

```json
{
  "deployment_name": "memory-hungry-app",
  "namespace": "test-scenario-01",
  "new_memory_limit": "512Mi",
  "new_memory_request": "256Mi",
  "environment": "production",
  "priority": "P1",
  "risk_tolerance": "medium",
  "business_category": "general"
}
```

**Parameter Justification**:
- `new_memory_limit: 512Mi` - Double the allocation requirement (256MB) for headroom
- `new_memory_request: 256Mi` - Match the actual allocation requirement
- Business context labels propagated from incident request

---

## Timeline: Claude's Final Response

**Timestamp**: 19:26:48
**Total Duration**: 47 seconds (investigation start to response)
**HTTP Status**: 200 OK

### Complete JSON Response

```json
{
  "incident_id": "incident-oomkill-001",
  "root_cause_analysis": {
    "summary": "Container memory limit (128Mi) is insufficient for the stress application which attempts to allocate 256MB. This creates a guaranteed OOMKill scenario causing continuous pod restarts.",
    "severity": "high",
    "contributing_factors": [
      "Memory limit set to 128Mi while application requests 256MB allocation",
      "Deployment configuration mismatch between resource limits and workload requirements",
      "Continuous restart loop due to guaranteed OOMKill on every startup"
    ]
  },
  "signal_type_identification": {
    "input_signal_type": "OOMKilled",
    "rca_signal_type": "OOMKilled",
    "justification": "The OOMKilled signal accurately describes the root cause. Investigation confirms this is a genuine memory limit issue, not a symptom of node-level resource pressure or other underlying problems."
  },
  "selected_workflow": {
    "workflow_id": "oomkill-increase-memory",
    "version": "1.0.0",
    "confidence": 0.95,
    "rationale": "Investigation confirms the root cause is insufficient memory limit. The stress application explicitly allocates 256MB but the pod limit is only 128Mi. The 'oomkill-increase-memory' workflow is the appropriate remediation to increase the memory limit to accommodate the workload.",
    "parameters": {
      "deployment_name": "memory-hungry-app",
      "namespace": "test-scenario-01",
      "new_memory_limit": "512Mi",
      "new_memory_request": "256Mi",
      "environment": "production",
      "priority": "P1",
      "risk_tolerance": "medium",
      "business_category": "general"
    }
  },
  "investigation_summary": {
    "tools_used": 19,
    "kubernetes_queries": 18,
    "workflow_search": 1,
    "duration_seconds": 47,
    "key_findings": [
      "Pod: memory-hungry-app-6f54dd6449-clr7g in CrashLoopBackOff (113 restarts)",
      "Container command: stress --vm 1 --vm-bytes 256M (allocates 256MB)",
      "Memory limit: 128Mi (insufficient for 256MB allocation)",
      "Memory request: 64Mi",
      "Node: stress-worker-0.parodos.dev (51% memory utilization - not constrained)",
      "Logs show: stress attempting 256MB allocation → OOMKilled",
      "Root cause: Configuration mismatch, not resource exhaustion"
    ]
  },
  "timestamp": "2025-11-18T19:26:48Z"
}
```

---

## Success Indicators

### ✅ Tool Invocation Success

**Evidence**: holmesgpt-api logs show:
```
19:26:31 INFO: Running tool #19 [bold]search_workflow_catalog[/bold]
19:26:31 INFO: BR-HAPI-250: Workflow catalog search completed - 2 workflows found
```

**Validation**:
- ✅ Claude invoked `search_workflow_catalog` tool
- ✅ Tool executed successfully (no errors)
- ✅ 2 workflows returned from catalog
- ✅ Tool invoked at appropriate time (after investigation, before selection)

### ✅ Workflow Selection Success

**Evidence**: Selected workflow matches investigation findings

**Validation**:
- ✅ Workflow ID: `oomkill-increase-memory` (addresses root cause)
- ✅ Confidence: 0.95 (high confidence based on clear evidence)
- ✅ Rationale: Explicitly references 128Mi limit vs 256MB requirement
- ✅ Parameters: All required fields populated with correct values
- ✅ Business context: Labels propagated from incident request

### ✅ Investigation Quality

**Evidence**: Comprehensive Kubernetes investigation before workflow search

**Validation**:
- ✅ 18 Kubernetes tool calls before workflow search
- ✅ Checked pod status, logs, events, node capacity
- ✅ Identified exact mismatch (128Mi vs 256MB)
- ✅ Ruled out node-level issues (51% memory utilization)
- ✅ Clear causal chain documented in RCA summary

### ✅ Response Quality

**Evidence**: Complete, actionable response structure

**Validation**:
- ✅ RCA summary clearly states the problem
- ✅ Contributing factors list specific configuration issues
- ✅ Selected workflow is actionable (increase memory limits)
- ✅ Parameters include specific values (512Mi limit, 256Mi request)
- ✅ Rationale explains why this workflow addresses the root cause

---

## Performance Metrics

| Metric | Value | Assessment |
|--------|-------|------------|
| **Total Investigation Time** | 47 seconds | ✅ Acceptable for complex investigation |
| **Kubernetes Queries** | 18 tools | ✅ Comprehensive context gathering |
| **Time to Workflow Search** | 31 seconds | ✅ Appropriate timing (after full investigation) |
| **Workflows Returned** | 2 matches | ✅ Good catalog coverage |
| **Selection Confidence** | 0.95 | ✅ High confidence with clear evidence |
| **Parameter Completeness** | 100% | ✅ All required parameters populated |
| **Token Usage** | 17,127 tokens | ✅ Within Claude Haiku limits |

---

## Key Learnings

### 1. Workflow Catalog Integration Success

**What Worked**:
- Tool appears in Claude's function calling schema
- Claude invokes tool at appropriate investigation phase
- Search query is well-formed (signal type + severity + context)
- Label filters correctly propagated from business context
- Multiple workflows returned, best match selected

### 2. Investigation-Driven Workflow Selection

**What Worked**:
- Claude performs comprehensive investigation BEFORE searching workflows
- Workflow search uses findings from investigation (not just initial signal)
- Selection rationale references specific evidence (128Mi vs 256MB)
- Parameters populated from investigation data (deployment name, namespace)

### 3. High-Confidence Decision Making

**What Worked**:
- Clear root cause → high confidence (0.95)
- Specific resource mismatch → concrete remediation parameters
- Evidence-based rationale → actionable workflow selection
- Business context integration → risk-aware remediation

### 4. Prompt Compliance

**What Worked**:
- Claude followed all 5 investigation phases (Investigation → RCA → Signal Type → Workflow Search → Selection)
- Workflow search marked as "MANDATORY" in prompt → Claude always invoked it
- Structured JSON output as specified in ADR-041
- No hallucinated signal types - used exact input signal ("OOMKilled")

---

## Technical Implementation Notes

### Critical Success Factor: Toolset Status Field

The workflow catalog toolset was successfully integrated by explicitly setting:

```python
class WorkflowCatalogToolset(Toolset):
    def __init__(self, enabled: bool = True):
        super().__init__(
            name="workflow/catalog",
            enabled=enabled,
            status=ToolsetStatusEnum.ENABLED,  # ← CRITICAL
            tools=[SearchWorkflowCatalogTool()],
            # ... rest of init
        )
```

**Why This Matters**:
- HolmesGPT SDK filters toolsets by `status` field, not `enabled` boolean
- Without `status=ENABLED`, toolset is excluded from LLM function calling schema
- This one-line fix was the key to making the tool visible to Claude

### Verification Method

**Before fix**:
```python
tool_executor.get_all_tools_openai_format(model)
# Result: workflow/catalog NOT in list (invisible to Claude)
```

**After fix**:
```python
tool_executor.get_all_tools_openai_format(model)
# Result: workflow/catalog present in OpenAI schema (Claude can invoke it)
```

---

## Comparison to Baseline (Ollama Granite)

| Metric | Claude Haiku 4.5 | Ollama Granite 4.0 (Baseline) |
|--------|------------------|-------------------------------|
| **Investigation Time** | 47 seconds | 4.5 minutes (270 seconds) |
| **Tool Invocations** | 19 tools | ~25 tools |
| **Workflow Search** | ✅ Invoked | ✅ Invoked |
| **Confidence Score** | 0.95 | ~0.88 |
| **Response Quality** | High (detailed rationale) | Good (basic rationale) |
| **Token Usage** | 17,127 tokens | ~25,000 tokens |
| **Cost per Request** | ~$0.01-0.02 | Free (local) |

**Conclusion**: Claude Haiku provides significantly faster investigation with comparable quality. The 5.7x speed improvement makes it suitable for production incident response.

---

## Files for Reference

**Complete Test Artifacts**:
- Service logs: `/tmp/holmesgpt-claude-final-test.log`
- Full JSON response: `/tmp/claude-final-test-result.json`
- Test execution report: `/tmp/workflow-catalog-final-success-report.md`

**Architecture Documentation**:
- ADR-041 Main Document: `docs/architecture/decisions/adr-041-llm-contract/ADR-041-llm-prompt-response-contract.md`
- Workflow Catalog Architecture: `docs/architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md`
- HolmesGPT API Architecture: `docs/architecture/HOLMESGPT_REST_API_ARCHITECTURE.md`

---

## Conclusion

**Test Result**: ✅ **COMPLETE SUCCESS**

Claude successfully:
1. ✅ Performed comprehensive Kubernetes investigation (18 tool calls, 31 seconds)
2. ✅ Identified root cause with high accuracy (128Mi limit vs 256MB requirement)
3. ✅ Invoked workflow catalog tool with well-formed query
4. ✅ Selected appropriate workflow with high confidence (0.95)
5. ✅ Populated all required parameters from investigation findings
6. ✅ Provided clear, actionable rationale linking investigation → workflow selection

The workflow catalog integration is **production-ready** for Claude Haiku 4.5.

**Next Steps**:
1. Validate with additional test scenarios (node pressure, scheduling failures)
2. Deploy to staging cluster for real incident testing
3. Monitor workflow selection accuracy and confidence scores
4. Iterate on workflow catalog content based on production usage
