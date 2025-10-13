# ⚠️  DEPRECATION NOTICE - Notification Service Documentation

**Status**: DEPRECATED (as of 2025-10-12)
**Confidence**: 95%
**Migration Path**: Complete

---

## 📢 **This Documentation Has Been Migrated**

The **Notification Service** has been **redesigned from a stateless HTTP API to a CRD Controller** and its documentation has been relocated.

### **New Location**

All Notification documentation is now in:
```
docs/services/crd-controllers/06-notification/
```

**Start here**: [06-notification/README.md](../../crd-controllers/06-notification/README.md)

---

## 🔄 **What Changed?**

### **Architecture Migration**

| Aspect | Before (HTTP API) | After (CRD Controller) |
|--------|------------------|----------------------|
| **Service Type** | Stateless HTTP API | CRD Controller |
| **Data Persistence** | In-memory only (lost on restart) | etcd (durable, replicated) |
| **Audit Trail** | None | Complete (all attempts tracked) |
| **Retry** | Caller responsibility | Automatic (controller reconciliation) |
| **Delivery Guarantee** | None | At-least-once |
| **Data Loss Risk** | High (pod restart loses data) | Zero (etcd persistence) |
| **Documentation Location** | `docs/services/stateless/notification-service/` | `docs/services/crd-controllers/06-notification/` |

### **Why the Change?**

The migration from HTTP API to CRD Controller addresses critical production requirements:

1. **BR-NOT-050: Zero Data Loss** - etcd persistence guarantees no notification data is lost on pod restart
2. **BR-NOT-051: Complete Audit Trail** - All delivery attempts tracked in CRD status and Data Storage
3. **BR-NOT-052: Automatic Retry** - Controller reconciliation provides automatic retry with exponential backoff
4. **BR-NOT-053: At-least-once Delivery** - CRD-based state guarantees at-least-once delivery semantics
5. **BR-NOT-054: Real-time Observability** - CRD status provides real-time delivery status

**Confidence**: 95% (from 45% with HTTP API)

---

## 📚 **Relocated Documentation**

All documents from this directory have been moved to the new location:

| Old Path | New Path |
|----------|----------|
| `UPDATED_BUSINESS_REQUIREMENTS_CRD.md` | [06-notification/UPDATED_BUSINESS_REQUIREMENTS_CRD.md](../../crd-controllers/06-notification/UPDATED_BUSINESS_REQUIREMENTS_CRD.md) |
| `DECLARATIVE_CRD_DESIGN_SUMMARY.md` | [06-notification/DECLARATIVE_CRD_DESIGN_SUMMARY.md](../../crd-controllers/06-notification/DECLARATIVE_CRD_DESIGN_SUMMARY.md) |
| `CRD_CONTROLLER_DESIGN.md` | [06-notification/CRD_CONTROLLER_DESIGN.md](../../crd-controllers/06-notification/CRD_CONTROLLER_DESIGN.md) |
| `ARCHITECTURE_IMPERATIVE_VS_DECLARATIVE_TRIAGE.md` | [06-notification/ARCHITECTURE_IMPERATIVE_VS_DECLARATIVE_TRIAGE.md](../../crd-controllers/06-notification/ARCHITECTURE_IMPERATIVE_VS_DECLARATIVE_TRIAGE.md) |
| `api-specification.md` | [06-notification/api-specification.md](../../crd-controllers/06-notification/api-specification.md) (DEPRECATED, reference only) |
| `integration-points.md` | [06-notification/integration-points.md](../../crd-controllers/06-notification/integration-points.md) |
| `observability-logging.md` | [06-notification/observability-logging.md](../../crd-controllers/06-notification/observability-logging.md) |
| `overview.md` | [06-notification/overview.md](../../crd-controllers/06-notification/overview.md) |
| `security-configuration.md` | [06-notification/security-configuration.md](../../crd-controllers/06-notification/security-configuration.md) |
| `testing-strategy.md` | [06-notification/testing-strategy.md](../../crd-controllers/06-notification/testing-strategy.md) |
| `implementation-checklist.md` | [06-notification/implementation-checklist.md](../../crd-controllers/06-notification/implementation-checklist.md) |

---

## 🎯 **Quick Migration Guide**

### **For Developers**

If you were working with the HTTP API documentation, update your references:

1. **Old**: `docs/services/stateless/notification-service/`
2. **New**: `docs/services/crd-controllers/06-notification/`

### **For Architecture References**

- **Service Catalog**: Updated to reflect "Notification Controller" (CRD-based)
- **Architecture Overview**: Updated to include 6 CRD controllers (not 5)
- **ADR-014** (Notification Service External Auth): Marked as SUPERSEDED

### **For Implementation**

- Follow the new CRD-based implementation plan in [06-notification/implementation-checklist.md](../../crd-controllers/06-notification/implementation-checklist.md)
- Use `NotificationRequest` CRD instead of HTTP POST to `/api/v1/notifications`
- Integrate with RemediationOrchestrator for automatic notification creation

---

## ⏰ **Timeline**

- **2025-10-07**: Decision to migrate from HTTP API to CRD Controller
- **2025-10-12**: Documentation migrated to `crd-controllers/06-notification/`
- **2025-10-12**: This deprecation notice added

---

## 📞 **Questions or Issues?**

### **About the Migration**
- See: [ARCHITECTURE_IMPERATIVE_VS_DECLARATIVE_TRIAGE.md](../../crd-controllers/06-notification/ARCHITECTURE_IMPERATIVE_VS_DECLARATIVE_TRIAGE.md)
- Contact: Kubernaut Architecture Team

### **Implementation Guidance**
- See: [06-notification/README.md](../../crd-controllers/06-notification/README.md)
- See: [06-notification/implementation-checklist.md](../../crd-controllers/06-notification/implementation-checklist.md)

### **Business Requirements**
- See: [06-notification/UPDATED_BUSINESS_REQUIREMENTS_CRD.md](../../crd-controllers/06-notification/UPDATED_BUSINESS_REQUIREMENTS_CRD.md)

---

## 🗑️  **Removal Plan**

This directory (`docs/services/stateless/notification-service/`) will remain for **90 days** (until 2026-01-10) to allow for:
- Link updates across external documentation
- Developer awareness and migration
- Gradual transition

**After 2026-01-10**: This directory will be removed entirely.

---

**Deprecation Date**: 2025-10-12
**Removal Date**: 2026-01-10 (90 days notice)
**Migration Status**: ✅ Complete (100%)
**Confidence**: 95%

