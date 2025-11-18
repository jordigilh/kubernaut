# ADR-041: LLM Prompt and Response Contract for Workflow Selection

**Status**: Proposed
**Date**: 2025-11-16
**Deciders**: Architecture Team
**Related**: DD-WORKFLOW-003, DD-STORAGE-008, DD-WORKFLOW-002, BR-WORKFLOW-001
**Version**: 2.7

## Changelog

### Version 2.7 (2025-11-16)
- Removed deduplication context section (not yet discussed for v1.0)
- Removed storm detection section (not yet discussed for v1.0)
- Removed recovery attempts field (not yet discussed for v1.0)
- Simplified to first-time incident prompt only

### Version 2.6 (2025-11-16)
- Removed `previous_remediation_ref` from prompt - reference without details is useless to LLM
- Removed ADR-041 reference from prompt - LLM cannot access the file
- Rationale: LLM can only use information it can actually access and understand

### Version 2.5 (2025-11-16)
- Fixed: Changed `similarity_score` to `confidence` throughout document to align with DD-STORAGE-008 API specification
- MCP tool returns `confidence` (not `similarity_score`) as the semantic match score

### Version 2.4 (2025-11-16)
- Renumbered from ADR-040 to ADR-041 (ADR-040 was already assigned to RemediationApprovalRequest CRD)
- Removed redundant `component` field from Signal Information section (duplicates `resource_kind`)

### Version 2.3 (2025-11-16)
- **BREAKING**: Cleaned up response format - removed outdated "strategies" array format
- Removed `estimated_risk` field (LLM cannot assess risk without knowing workflow internals)
- Clarified `confidence` is MCP confidence (pass-through), not LLM assessment
- Clarified `rationale` explains search logic, not workflow appropriateness
- Removed `name` and `description` from MCP tool response (avoid biasing LLM)
- Added `depends_on` to parameter schema fields
- Unified field definitions with single authoritative table

### Version 2.2 (2025-11-16)
- Renumbered from ADR-039 to ADR-040 (ADR-039 was already assigned to complex decision documentation pattern)

### Version 2.1 (2025-11-16)
- Renumbered from ADR-038 to ADR-039 (ADR-038 was already assigned to async audit ingestion)

### Version 2.0 (2025-11-16)
- Previous changelog entries...

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

### Principle 1: Single Workflow per Incident

**CRITICAL**: The LLM must select ONE workflow per incident, not multiple playbooks.

**Industry Alignment** (95% confidence):
- **PagerDuty**: Single runbook per incident
- **Datadog**: Single remediation workflow per alert
- **AWS Systems Manager**: Single automation document per execution
- **Ansible Tower/AWX**: Single workflow per job
- **Rundeck**: Single job per execution

**Rationale**:
1. ✅ **Auditability**: Clear 1:1 mapping (Incident → Workflow → Outcome)
2. ✅ **Rollback Simplicity**: Single execution to rollback, no complex ordering
3. ✅ **Blast Radius Control**: Predictable impact, easier risk assessment
4. ✅ **LLM Reasoning Simplicity**: LLM selects top-ranked workflow from MCP search
5. ❌ **Multi-Playbook Complexity**: LLM cannot reason about dependencies (doesn't know workflow internals)

**For Complex Remediations**:
- **Option A** (Recommended): Single workflow with multiple internal steps
  ```yaml
  # Workflow handles complexity internally
  apiVersion: tekton.dev/v1
  kind: Pipeline
  spec:
    tasks:
      - name: increase-memory
      - name: restart-pods
        runAfter: [increase-memory]
  ```
- **Option B** (Industry Standard): Sequential incidents for retry
  ```
  Incident 1 → increase-memory → Failed
  Incident 2 → restart-pods → Success (with history context)
  ```

**Alternative Playbooks**:
- `alternative_playbooks` field is for:
  - Human review (operator can choose alternative)
  - Automatic retry (if primary fails, try alternative in new incident)
- NOT for parallel or sequential execution within same incident

### Principle 2: Observable Facts Only for Input

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
- Signal Type: {signal_type}           # e.g., "OOMKilled", "CrashLoopBackOff" (DD-WORKFLOW-001)
- Severity: {severity}                  # e.g., "critical", "high", "medium", "low" (DD-WORKFLOW-001)
- Alert Name: {alert_name}              # Human-readable name from source
- Namespace: {namespace}                # Kubernetes namespace
- Resource: {resource_kind}/{resource_name}  # e.g., "deployment/my-app"

## Error Details (FOR RCA INVESTIGATION)
- Error Message: {error_message}        # From signal annotations
- Description: {description}            # From signal annotations
- Firing Time: {firing_time}            # When signal started
- Received Time: {received_time}        # When Gateway received signal


---

## Appendix A: Workflow Catalog Integration Timeline

**Complete debugging timeline and technical analysis**: See [ADR-041-APPENDIX-WORKFLOW-CATALOG-INTEGRATION-TIMELINE.md](./ADR-041-APPENDIX-WORKFLOW-CATALOG-INTEGRATION-TIMELINE.md)

### Quick Summary

**Date**: 2025-11-18
**Status**: ✅ **COMPLETE SUCCESS**
**Root Cause**: Custom toolset missing `status=ToolsetStatusEnum.ENABLED`
**Fix Complexity**: 1 line of code
**Investigation Time**: ~4 hours

#### The Critical Fix

```python
class WorkflowCatalogToolset(Toolset):
    def __init__(self, enabled: bool = True):
        super().__init__(
            name="workflow/catalog",
            enabled=enabled,
            status=ToolsetStatusEnum.ENABLED,  # ← CRITICAL: Must explicitly set
            tools=[SearchWorkflowCatalogTool()],
            # ... rest of init
        )
```

#### Test Result: OOMKilled Container (test-scenario-01)

**Claude's Performance**:
- ✅ Completed full investigation (pod status, events, logs, resource config)
- ✅ Identified root cause: 128Mi limit vs 256MB allocation
- ✅ **Invoked workflow catalog** (tool #19)
- ✅ **Selected workflow**: `oomkill-increase-memory` (confidence 0.95)
- ✅ Populated all parameters correctly
- ✅ Provided clear rationale linking RCA to workflow selection

**Key Logs**:
```
19:26:31 INFO: Running tool #19 [bold]search_workflow_catalog[/bold]
19:26:31 INFO: BR-HAPI-250: Workflow catalog search completed - 2 workflows found
```

#### Key Learnings

1. **HolmesGPT SDK filters by `status`, not `enabled`** - Must set `status=ToolsetStatusEnum.ENABLED`
2. **Verify in OpenAI schema** - Use `tool_executor.get_all_tools_openai_format(model)` to confirm visibility
3. **Layer-by-layer debugging** - Check toolsets → enabled_toolsets → OpenAI schema conversion
4. **Compare with working toolsets** - kubernetes/core was the reference for correct configuration

**For complete timeline with all diagnostic steps, troubleshooting techniques, and lessons learned**, see the full appendix document linked above.

