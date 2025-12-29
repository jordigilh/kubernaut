# NOTICE: OpenAPI Spec Missing Batch Audit Endpoint

**Date**: 2025-12-11
**Version**: 1.0
**From**: Gateway Team
**To**: Data Storage Team
**Status**: 🟡 **DOCUMENTATION GAP**
**Priority**: LOW (implementation exists, docs only)

---

## 📋 Summary

The batch audit endpoint `POST /api/v1/audit/events/batch` is **implemented** but **not documented** in the OpenAPI specification.

---

## 🔍 Gap Details

### Implementation (Exists ✅)

**File**: `pkg/datastorage/server/server.go`

```go
// DD-AUDIT-002: Batch audit events API for HTTPDataStorageClient.StoreBatch()
r.Post("/audit/events/batch", s.handleCreateAuditEventsBatch)
```

**Handler**: `pkg/datastorage/server/audit_events_batch_handler.go`

**Client**: `pkg/audit/http_client.go` - `HTTPDataStorageClient.StoreBatch()`

### OpenAPI Spec (Missing ❌)

**File**: `docs/services/stateless/data-storage/api/audit-write-api.openapi.yaml`

The spec documents single event endpoints but not the batch endpoint.

### Impact

| Aspect | Impact |
|--------|--------|
| **Runtime** | ✅ None - endpoint works correctly |
| **Client Generation** | ⚠️ Generated clients won't have batch method |
| **Documentation** | ⚠️ API consumers unaware of batch capability |

---

## 📝 Recommended OpenAPI Addition

Add to `audit-write-api.openapi.yaml`:

```yaml
  /api/v1/audit/events/batch:
    post:
      summary: Create audit events in batch
      description: Write multiple audit events in a single request.
      operationId: createAuditEventsBatch
      tags:
        - Audit
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: array
              items:
                $ref: '#/components/schemas/AuditEvent'
              minItems: 1
              maxItems: 1000
      responses:
        '201':
          description: All audit events created successfully
        '400':
          description: Bad request - Invalid event data
        '500':
          description: Internal server error
```

---

## ✅ Acceptance Criteria

- [ ] Batch endpoint added to `audit-write-api.openapi.yaml`
- [ ] Request/response schemas documented
- [ ] Examples provided

---

## 📬 Response Requested

Please acknowledge and provide timeline for OpenAPI update.

---

**Document Status**: 🟡 **AWAITING DS TEAM RESPONSE**

