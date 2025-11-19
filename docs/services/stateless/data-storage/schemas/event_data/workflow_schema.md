# Workflow Service - Event Data Schema

**Version**: 1.0
**Service**: workflow
**Purpose**: Workflow execution lifecycle, step tracking, approvals, outcomes

---

## Schema Structure

```json
{
  "version": "1.0",
  "service": "workflow",
  "event_type": "workflow.started|workflow.completed|workflow.failed",
  "timestamp": "2025-11-18T10:00:00Z",
  "data": {
    "workflow": {
      "workflow_id": "workflow-increase-memory",
      "execution_id": "exec-2025-11-18-001",
      "phase": "executing",
      "current_step": 3,
      "total_steps": 5,
      "step_name": "increase_memory_limits",
      "duration_ms": 45000,
      "outcome": "success",
      "approval_required": true,
      "approval_decision": "approved",
      "approver": "sre-team@example.com",
      "error_message": "Failed to apply resource"
    }
  }
}
```

---

## Field Definitions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `workflow_id` | string | Yes | Workflow identifier |
| `execution_id` | string | No | Unique execution ID |
| `phase` | string | No | `pending`, `executing`, `completed`, `failed` |
| `current_step` | integer | No | Current step number (1-indexed) |
| `total_steps` | integer | No | Total steps |
| `step_name` | string | No | Current step name |
| `duration_ms` | integer | No | Execution duration |
| `outcome` | string | No | `success`, `failed`, `cancelled` |
| `approval_required` | boolean | Yes | Approval flag |
| `approval_decision` | string | No | `approved`, `rejected` |
| `approver` | string | No | Approver identifier |
| `error_message` | string | No | Error message if failed |

---

## Go Builder Usage

```go
eventData, err := audit.NewWorkflowEvent("workflow.completed").
    WithWorkflowID("workflow-increase-memory").
    WithExecutionID("exec-2025-001").
    WithPhase("completed").
    WithCurrentStep(5, 5).
    WithStepName("verify_health").
    WithDuration(45000).
    WithOutcome("success").
    WithApprovalRequired(true).
    WithApprovalDecision("approved", "sre-team@example.com").
    Build()
```

---

## Query Examples

### Find workflows requiring approval

```sql
SELECT event_id, event_timestamp,
       event_data->'data'->'workflow'->>'workflow_id' AS workflow,
       event_data->'data'->'workflow'->>'approval_decision' AS decision
FROM audit_events
WHERE event_data->>'service' = 'workflow'
AND event_data->'data'->'workflow'->>'approval_required' = 'true'
ORDER BY event_timestamp DESC;
```

### Track workflow success rate

```sql
SELECT
    event_data->'data'->'workflow'->>'workflow_id' AS workflow,
    COUNT(*) AS total_executions,
    SUM(CASE WHEN event_data->'data'->'workflow'->>'outcome' = 'success' THEN 1 ELSE 0 END) AS successful,
    ROUND(100.0 * SUM(CASE WHEN event_data->'data'->'workflow'->>'outcome' = 'success' THEN 1 ELSE 0 END) / COUNT(*), 2) AS success_rate
FROM audit_events
WHERE event_data->>'service' = 'workflow'
AND event_type = 'workflow.completed'
GROUP BY workflow
ORDER BY success_rate DESC;
```

---

## Business Requirements

| BR ID | Requirement | Fields Used |
|-------|-------------|-------------|
| BR-STORAGE-033-010 | Workflow event structure | All workflow fields |
| BR-STORAGE-033-011 | Phase/step tracking | `phase`, `current_step`, `total_steps`, `duration_ms` |
| BR-STORAGE-033-012 | Approval/outcome metadata | `approval_required`, `approval_decision`, `approver`, `outcome` |

