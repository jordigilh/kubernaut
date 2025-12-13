# DataStorage OpenAPI Client Gap: Missing Audit Events API

**Date**: 2025-12-12  
**Reported By**: AI Assistant (during BR-SP-090 E2E debugging)  
**Resolved By**: AI Assistant (2025-12-13 03:00 AM)  
**Priority**: Medium  
**Status**: ✅ **RESOLVED AND VERIFIED**

---

## ⚠️ DOCUMENT LOCATION ISSUE

**This file is in the WRONG workspace location:**
- **Current (Wrong)**: `/Users/jgil/.cursor/worktrees/kubernaut/ebu/docs/issues/`
- **Should be**: `/Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/issues/`

**Impact**: This worktree copy is NOT part of the main repository and changes here won't be committed or visible to the team.

**Action Required**: 
1. Move this file to the main repo: `docs/issues/DS_OPENAPI_AUDIT_EVENTS_GAP.md`
2. Delete this worktree copy
3. Reference the main repo version going forward

**Verification**: All code changes below are verified in the **main repository** at `/Users/jgil/go/src/github.com/jordigilh/kubernaut/`, not in this worktree.

---

## ✅ VERIFICATION PASSED (2025-12-13 03:00 AM)

**All code changes have been implemented and verified in the main repo.**

### Verification Results

| Check | Command | Result |
|-------|---------|--------|
| Audit events endpoint in spec | `grep "/api/v1/audit/events" api/openapi/data-storage-v1.yaml` | ✅ **FOUND** (2 endpoints) |
| CreateAuditEvent in generated client | `grep -c "CreateAuditEvent" pkg/datastorage/client/generated.go` | ✅ **62 occurrences** |
| QueryAuditEvents in generated client | `grep -c "QueryAuditEvents" pkg/datastorage/client/generated.go` | ✅ **20 occurrences** |
| Client compiles | `go build ./pkg/datastorage/client/...` | ✅ **SUCCESS** |

### Actual Verification Commands Run (2025-12-13 03:00 AM)

```bash
$ cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Check for audit/events endpoint in spec
$ grep "/api/v1/audit/events" api/openapi/data-storage-v1.yaml
  /api/v1/audit/events:                    ✅ FOUND
  /api/v1/audit/events/batch:              ✅ FOUND

# Check for CreateAuditEvent in generated client
$ grep -c "CreateAuditEvent" pkg/datastorage/client/generated.go
62                                          ✅ PASS (62 occurrences)

# Check for QueryAuditEvents in generated client  
$ grep -c "QueryAuditEvents" pkg/datastorage/client/generated.go
20                                          ✅ PASS (20 occurrences)

# Verify compilation
$ go build ./pkg/datastorage/client/...
(exit code 0)                               ✅ PASS
```

---

## Summary

The DataStorage OpenAPI spec has been **updated** and the Go client has been **regenerated** with audit events API methods.

**Current state:**
- ✅ `/api/v1/audit/events` (GET) - **EXISTS** in spec → `queryAuditEvents`
- ✅ `/api/v1/audit/events` (POST) - **EXISTS** in spec → `createAuditEvent`
- ✅ `/api/v1/audit/events/batch` (POST) - **EXISTS** in spec → `createAuditEventsBatch`
- ✅ `CreateAuditEvent()` - **AVAILABLE** in generated client (62 references)
- ✅ `QueryAuditEvents()` - **AVAILABLE** in generated client (20 references)
- ✅ `/api/v1/audit/notifications` - Continues to exist (notification-specific endpoint)

SignalProcessing team can now use the typed client instead of raw HTTP.

---

## What Was Done

### Step 1: Updated OpenAPI Spec ✅

**File**: `api/openapi/data-storage-v1.yaml`

Added three endpoints:

```yaml
paths:
  /api/v1/audit/events:
    post:
      operationId: createAuditEvent
      summary: Create unified audit event
      tags:
        - Audit Write API
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/AuditEventRequest'
      responses:
        '201':
          description: Audit event created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AuditEventResponse'
        '202':
          description: Database write failed, queued to DLQ
        '400':
          description: Validation error
        '500':
          description: Internal server error

    get:
      operationId: queryAuditEvents
      summary: Query audit events
      tags:
        - Audit Write API
      parameters:
        - name: event_type
          in: query
          schema:
            type: string
        - name: correlation_id
          in: query
          schema:
            type: string
        - name: limit
          in: query
          schema:
            type: integer
            default: 50
        - name: offset
          in: query
          schema:
            type: integer
            default: 0
      responses:
        '200':
          description: Query results
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AuditEventsQueryResponse'

  /api/v1/audit/events/batch:
    post:
      operationId: createAuditEventsBatch
      summary: Create audit events batch
      tags:
        - Audit Write API
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: array
              items:
                $ref: '#/components/schemas/AuditEventRequest'
      responses:
        '201':
          description: Batch created successfully
```

### Step 2: Added Schemas ✅

**File**: `api/openapi/data-storage-v1.yaml` (components/schemas section)

```yaml
components:
  schemas:
    AuditEventRequest:
      type: object
      required:
        - version
        - event_type
        - event_timestamp
        - event_category
        - event_action
        - event_outcome
        - correlation_id
        - event_data
      properties:
        version:
          type: string
          example: "1.0"
        event_type:
          type: string
          example: gateway.signal.received
        event_timestamp:
          type: string
          format: date-time
        event_category:
          type: string
        event_action:
          type: string
        event_outcome:
          type: string
          enum: [success, failure, pending]
        correlation_id:
          type: string
        event_data:
          type: object
          additionalProperties: true
        # ... plus optional fields

    AuditEventResponse:
      type: object
      required:
        - data
      properties:
        data:
          type: object
          required:
            - event_id
            - created_at
          properties:
            event_id:
              type: string
              format: uuid
            created_at:
              type: string
              format: date-time

    BatchAuditEventResponse:
      type: object
      properties:
        event_ids:
          type: array
          items:
            type: string
            format: uuid

    AuditEventsQueryResponse:
      type: object
      properties:
        data:
          type: array
          items:
            $ref: '#/components/schemas/AuditEvent'
        total_count:
          type: integer
        pagination:
          type: object
```

### Step 3: Regenerated Client ✅

**Command executed**:
```bash
oapi-codegen -package client -generate types,client \
  -o pkg/datastorage/client/generated.go \
  api/openapi/data-storage-v1.yaml
```

**File updated**: `pkg/datastorage/client/generated.go`

### Step 4: Verified ✅

All checks passed (see verification table above).

---

## Available Methods Now

### Generated Client Methods (in `pkg/datastorage/client/generated.go`)

```go
// New audit events methods
CreateAuditEvent(ctx context.Context, body CreateAuditEventJSONRequestBody, reqEditors ...RequestEditorFn) (*http.Response, error)
CreateAuditEventsBatch(ctx context.Context, body []AuditEventRequest, reqEditors ...RequestEditorFn) (*http.Response, error)
QueryAuditEvents(ctx context.Context, params *QueryAuditEventsParams, reqEditors ...RequestEditorFn) (*http.Response, error)

// Existing methods (still available)
CreateNotificationAudit(ctx context.Context, body NotificationAudit, reqEditors ...RequestEditorFn) (*http.Response, error)
HealthCheck(ctx context.Context, reqEditors ...RequestEditorFn) (*http.Response, error)
LivenessCheck(ctx context.Context, reqEditors ...RequestEditorFn) (*http.Response, error)
ReadinessCheck(ctx context.Context, reqEditors ...RequestEditorFn) (*http.Response, error)
```

### Generated Types (in `pkg/datastorage/client/generated.go`)

```go
type AuditEventRequest struct {
    Version        string                        `json:"version"`
    EventType      string                        `json:"event_type"`
    EventTimestamp time.Time                     `json:"event_timestamp"`
    EventCategory  string                        `json:"event_category"`
    EventAction    string                        `json:"event_action"`
    EventOutcome   AuditEventRequestEventOutcome `json:"event_outcome"`
    CorrelationId  string                        `json:"correlation_id"`
    EventData      map[string]interface{}        `json:"event_data"`
    // ... plus optional fields
}

type AuditEventResponse struct {
    Data struct {
        EventId   openapi_types.UUID `json:"event_id"`
        CreatedAt time.Time          `json:"created_at"`
    } `json:"data"`
    Message *string `json:"message,omitempty"`
}

type QueryAuditEventsParams struct {
    CorrelationId *string `form:"correlation_id,omitempty" json:"correlation_id,omitempty"`
    EventType     *string `form:"event_type,omitempty" json:"event_type,omitempty"`
    Limit         *int    `form:"limit,omitempty" json:"limit,omitempty"`
    Offset        *int    `form:"offset,omitempty" json:"offset,omitempty"`
}
```

---

## For SignalProcessing Team

### ✅ You Can Now Use the Typed Client

**Before (Raw HTTP - Remove This)**:
```go
func queryAuditEvents(fingerprint string) ([]AuditEvent, error) {
    url := "http://localhost:30081/api/v1/audit/events?limit=100"
    resp, err := http.Get(url)
    // ... manual JSON parsing
}
```

**After (Typed Client - Use This)**:
```go
import (
    "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// Create client once
dsClient, err := client.NewClientWithResponses("http://localhost:30081")
if err != nil {
    return err
}

// Type-safe query with auto-generated types
limit := 100
resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &client.QueryAuditEventsParams{
    Limit: &limit,
})
if err != nil {
    return err
}

// Type-safe response parsing
if resp.JSON200 != nil {
    for _, event := range resp.JSON200.Data {
        // event is fully typed
        fmt.Printf("Event ID: %s, Type: %s\n", event.EventId, event.EventType)
    }
}
```

**Create Audit Event Example**:
```go
resp, err := dsClient.CreateAuditEventWithResponse(ctx, client.CreateAuditEventJSONRequestBody{
    Version:        "1.0",
    EventType:      "signalprocessing.alert.storm_detected",
    EventTimestamp: time.Now(),
    EventCategory:  "alert",
    EventAction:    "storm_detected",
    EventOutcome:   client.Success,
    CorrelationId:  "rr-2025-001",
    EventData: map[string]interface{}{
        "storm_id": "storm-123",
        "alert_count": 25,
    },
})

if resp.JSON201 != nil {
    eventID := resp.JSON201.Data.EventId
    fmt.Printf("Created event: %s\n", eventID)
}
```

---

## Next Steps for SP Team

1. ✅ **OpenAPI spec updated** - No action needed
2. ✅ **Client regenerated** - No action needed
3. **TODO**: Refactor SignalProcessing E2E tests to use typed client
4. **TODO**: Remove raw HTTP workarounds from `test/e2e/signalprocessing/business_requirements_test.go`
5. **TODO**: Test the typed client in your E2E tests

---

## References

- **BR-SP-090**: Audit Trail Persistence (SignalProcessing)
- **ADR-034**: Unified Audit Table Design
- **ADR-038**: Async Buffered Audit Ingestion
- **DD-AUDIT-002**: Audit Shared Library Design

---

## 🎯 Issue Status

**RESOLVED AND VERIFIED** ✅

All changes have been implemented in the main repository:
- ✅ `api/openapi/data-storage-v1.yaml` updated
- ✅ `pkg/datastorage/client/generated.go` regenerated
- ✅ Compilation verified
- ✅ Methods confirmed present (62 CreateAuditEvent refs, 20 QueryAuditEvents refs)

SP team can proceed with using the typed client.

**Verification timestamp**: 2025-12-13 03:00 AM
