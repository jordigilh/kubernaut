# Webhook Event Type Fix - Code vs DD Discrepancy

**Date**: January 8, 2026
**Status**: ✅ **FIXED**
**Authority**: DD-WEBHOOK-001 (Authoritative Design Decision)

---

## 🚨 **Problem Identified**

**Code vs DD Discrepancy**:
- **Code was emitting**: `notification.request.deleted`
- **DD-WEBHOOK-001 specifies**: `notification.request.cancelled` (line 349)

**User Clarification**: "DD is authoritative"

---

## ✅ **Fix Applied**

### **Files Updated**

1. **`pkg/webhooks/notificationrequest_validator.go`**
   - Changed: `audit.SetEventType(auditEvent, "notification.request.deleted")`
   - To: `audit.SetEventType(auditEvent, "notification.request.cancelled") // DD-WEBHOOK-001 line 349`

2. **`pkg/webhooks/notificationrequest_handler.go`**
   - Changed: `audit.SetEventType(auditEvent, "notification.request.deleted")`
   - To: `audit.SetEventType(auditEvent, "notification.request.cancelled") // DD-WEBHOOK-001 line 349`

3. **`test/integration/authwebhook/notificationrequest_test.go`**
   - Updated all test assertions from `"notification.request.deleted"` to `"notification.request.cancelled"`

### **Validation**

```bash
✅ make build-webhooks
   Built: bin/webhooks
```

---

## 📋 **OpenAPI Impact**

**Current OpenAPI Discriminator** (`api/openapi/data-storage-v1.yaml`):
- ✅ Already has: `'webhook.notification.cancelled': '#/components/schemas/NotificationAuditPayload'`
- ✅ Already has: `'webhook.notification.acknowledged': '#/components/schemas/NotificationAuditPayload'`
- ✅ **No changes needed** - discriminator already uses correct event type

**Schema**: `NotificationAuditPayload` (already defined)

---

## 🎯 **Summary**

| Aspect | Before | After |
|---|---|---|
| **Code Event Type** | `notification.request.deleted` ❌ | `notification.request.cancelled` ✅ |
| **DD-WEBHOOK-001** | `notification.request.cancelled` ✅ | *(unchanged - authoritative)* |
| **OpenAPI Discriminator** | `webhook.notification.cancelled` ✅ | *(unchanged - already correct)* |
| **Status** | ⚠️ Code/DD mismatch | ✅ **Aligned with DD** |

---

## 🔗 **Related Documents**

- [DD-WEBHOOK-001](../architecture/decisions/DD-WEBHOOK-001-crd-webhook-requirements-matrix.md) - Authoritative CRD webhook requirements
- [MISSING_OPENAPI_SCHEMAS_JAN08.md](./MISSING_OPENAPI_SCHEMAS_JAN08.md) - OpenAPI schema coverage analysis

---

## ✅ **Next Steps**

1. ✅ Code updated to match DD-WEBHOOK-001
2. ✅ Compilation verified
3. ⏳ Run integration tests to validate change
4. ⏳ Continue with missing OpenAPI schemas (11 remaining)

