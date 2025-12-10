# Response: WorkflowExecution Team - E2E Migration Library Proposal

**Team**: WorkflowExecution
**Date**: December 10, 2025
**Decision**: ✅ **APPROVED - REQUIRED**

---

## Feedback

### 1. Agreement
**Yes** - WE requires the shared migration library for BR-WE-005 E2E testing.

### 2. Required Migrations

| Table | Reason |
|-------|--------|
| `audit_events` | BR-WE-005 audit persistence |
| `audit_events_y2025m12` | Current month partition |
| `audit_events_y2026m01` | Next month partition |

**Indexes Required**:
- `idx_audit_events_correlation` - Query by `correlation_id` (WFE name)
- `idx_audit_events_event_type` - Query by event type
- `idx_audit_events_timestamp` - Query by time range

### 3. Concerns
None - this is a clean solution.

### 4. Preferred Location
`test/infrastructure/migrations.go` - consistent with existing E2E infrastructure code.

### 5. Additional Requirements
None.

---

## Why WE Needs This Library

### Business Requirement: BR-WE-005

> **BR-WE-005**: Audit events MUST persist to PostgreSQL via Data Storage for compliance tracking.

### WE Controller Emits Audit Events

The WE controller is configured with `--datastorage-url` and emits:

| Event Type | When Emitted |
|------------|--------------|
| `workflowexecution.workflow.started` | WFE transitions to Running |
| `workflowexecution.workflow.completed` | PipelineRun succeeds |
| `workflowexecution.workflow.failed` | PipelineRun fails |

**Code Reference**: `internal/controller/workflowexecution/workflowexecution_controller.go`

```go
auditEvent := audit.NewEventBuilder().
    WithEventType("workflowexecution.workflow.started").
    WithCorrelationID(wfe.Name).
    Build()
r.auditStore.Store(ctx, auditEvent)
```

### E2E Test Requires audit_events Table

**File**: `test/e2e/workflowexecution/02_observability_test.go:269`

```go
Context("BR-WE-005: Audit Persistence in PostgreSQL (E2E)", Label("datastorage", "audit"), func() {
    const dataStorageServiceURL = "http://localhost:8081"

    It("should persist audit events to Data Storage for completed workflow", func() {
        // Queries DS for audit events by correlation_id
        auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s",
            dataStorageServiceURL, wfe.Name)

        // FAILS with HTTP 500 if audit_events table doesn't exist
        Eventually(func() int {
            resp, err := http.Get(auditQueryURL)
            // ...
        }, 60*time.Second).Should(BeNumerically(">=", 2))
    })
})
```

---

## Current Status Without Shared Library

| Test Tier | Status | Notes |
|-----------|--------|-------|
| Unit Tests | ✅ Pass | Uses mock audit store |
| Integration Tests | ✅ Pass | `podman-compose.test.yml` has migrate service |
| E2E Tests | ❌ **BLOCKED** | Kind cluster has no migrations |

### Error Without Migrations

```
ERROR: relation "audit_events" does not exist (SQLSTATE 42P01)
```

---

## Proposed Usage in WE Infrastructure

Once DS implements the shared library:

```go
// In test/infrastructure/workflowexecution.go
import "github.com/jordigilh/kubernaut/test/infrastructure"

func CreateWorkflowExecutionCluster(...) error {
    // ... deploy PostgreSQL, wait for ready ...

    // Apply migrations using shared library
    if err := infrastructure.ApplyAuditEventsMigration(kubeconfigPath, namespace, output); err != nil {
        return fmt.Errorf("failed to apply migrations: %w", err)
    }

    // ... deploy DS ...
}
```

---

## Summary

| Question | Answer |
|----------|--------|
| Do you agree this should be shared? | ✅ Yes |
| What migrations does WE need? | `audit_events` + partitions + indexes |
| Any concerns? | None |
| Preferred location? | `test/infrastructure/migrations.go` |
| Additional requirements? | None |

---

**Document Version**: 1.0
**Created**: December 10, 2025
**Contact**: WorkflowExecution Team

