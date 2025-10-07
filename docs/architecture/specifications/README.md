# Architecture Specifications

**Purpose**: This directory contains cross-service technical specifications that are referenced by multiple components.

---

## 📋 Specification Index

| Specification | Services Affected | Status | Description |
|---------------|------------------|--------|-------------|
| [Notification Payload Schema](./notification-payload-schema.md) | All CRD Controllers, Notification Service | ✅ Authoritative | Unified escalation notification payload format |
| [BR Mapping Matrix](./br-mapping-matrix.md) | All Services | ✅ Authoritative | Business requirement mapping across services |

---

## 🎯 Purpose of Specifications

Specifications in this directory:
- ✅ Define **authoritative schemas** used by multiple services
- ✅ Document **cross-service contracts** and interfaces
- ✅ Serve as **single source of truth** for shared data structures
- ✅ Enable **consistent implementation** across service boundaries

---

## 📝 Key Specifications

### **Notification Payload Schema**
**File**: `notification-payload-schema.md`
**Status**: ✅ Authoritative Schema

Defines the unified notification payload structure used by all CRD controllers when sending escalation notifications to the Notification Service.

**Consumers**:
- Remediation Orchestrator (timeout escalations)
- AIAnalysis Controller (rejection escalations)
- WorkflowExecution Controller (failure escalations)
- RemediationProcessor (validation failure escalations)

**Provider**:
- Notification Service (`POST /api/v1/notify/escalation`)

---

### **BR Mapping Matrix**
**File**: `br-mapping-matrix.md`
**Status**: ✅ Authoritative Reference

Comprehensive mapping of business requirements (BR-XXX-XXX format) across all services, showing which services implement which business requirements.

**Use Cases**:
- Verify BR coverage across services
- Track business requirement implementation
- Identify gaps in BR implementation
- Facilitate BR-driven development

---

## 🔗 Related Documentation

- **Architecture Decisions**: Decision records for major architectural choices → [../decisions/](../decisions/)
- **References**: Visual diagrams and reference materials → [../references/](../references/)
- **Service Docs**: Individual service specifications → [../../services/](../../services/)

---

**Maintained By**: Kubernaut Architecture Team
**Last Updated**: October 7, 2025
