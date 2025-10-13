# ⚠️  DEPRECATED - Notification Service Documentation

> **🚨 CRITICAL NOTICE: This documentation is DEPRECATED as of 2025-10-12**
>
> The Notification Service has been **redesigned from a stateless HTTP API to a CRD Controller**.
>
> **New Location**: [docs/services/crd-controllers/06-notification/](../../crd-controllers/06-notification/)
>
> **Start Here**: [06-notification/README.md](../../crd-controllers/06-notification/README.md)
>
> **See**: [DEPRECATION_NOTICE.md](./DEPRECATION_NOTICE.md) for full migration details.
>
> **Removal Date**: 2026-01-10 (90 days from deprecation)

---

# Notification Service - Documentation Hub (ARCHIVED)

**Version**: 1.0
**Last Updated**: 2025-10-06
**Service Type**: Stateless HTTP API Service (DEPRECATED)
**Status**: ⛔ DEPRECATED - Migrated to CRD Controller
**Priority**: P0 - CRITICAL (MIGRATION COMPLETE)

---

## 📋 Quick Navigation

### **Core Documentation** (Start Here)
1. [overview.md](./overview.md) - Service architecture, business requirements, and design decisions
2. [api-specification.md](./api-specification.md) - HTTP API endpoints, request/response schemas
3. [security-configuration.md](./security-configuration.md) - RBAC, secrets management, data sanitization
4. [observability-logging.md](./observability-logging.md) - Metrics, logging, and monitoring
5. [testing-strategy.md](./testing-strategy.md) - Unit, integration, and E2E test patterns
6. [integration-points.md](./integration-points.md) - Service dependencies and integration patterns
7. [implementation-checklist.md](./implementation-checklist.md) - APDC-TDD implementation guide

### **Historical Design & Analysis** (Archive)
- **Archived Working Documents**: [archive/](./archive/) - Historical triage, solutions, revisions, and summaries
  - **Triage**: 28 issues identified and prioritized
  - **Solutions**: Production-ready solutions for CRITICAL/HIGH/MEDIUM issues
  - **Revisions**: Architectural corrections
  - **Summaries**: Completion summaries and progress tracking

---

## 🎯 Purpose

**Deliver multi-channel notifications with comprehensive escalation context, sensitive data sanitization, and direct action links to external services.**

**Core Capabilities**:
- ✅ Escalation notifications with comprehensive context (BR-NOT-026 through BR-NOT-037)
- ✅ Multi-channel delivery (Email, Slack, Teams, SMS, webhooks)
- ✅ Sensitive data protection and sanitization
- ✅ Channel-specific formatting adapters
- ✅ External service action links (GitHub, Grafana, Prometheus, K8s Dashboard)

---

## 🏗️ Service Configuration

| Aspect | Value |
|--------|-------|
| **HTTP Port** | 8080 |
| **Metrics Port** | 9090 (with auth) |
| **Health Endpoints** | `/health`, `/ready` (no auth) |
| **API Endpoints** | `/api/v1/notify/*` (with TokenReviewer auth) |
| **Namespace** | `prometheus-alerts-slm` |
| **ServiceAccount** | `notification-service` |

---

## 📊 Business Requirements Coverage

### **Primary Requirements** (Escalation Notifications)
- **BR-NOT-026** through **BR-NOT-037**: Comprehensive escalation context, sanitization, external links

### **Secondary Requirements** (Multi-Channel Delivery)
- **BR-NOT-001** through **BR-NOT-005**: Email, Slack, Teams, SMS, webhooks

### **V1 Scope**
- ✅ Escalation notification delivery
- ✅ Multi-channel adapters (5 types)
- ✅ Sensitive data sanitization
- ✅ External service action links
- ✅ Template-based rendering

### **V2 Future** (Excluded from V1)
- ❌ Access-aware link rendering (BR-NOT-038)
- ❌ Localization/i18n support (BR-NOT-039)
- ❌ Template validation framework (BR-NOT-040)

---

## 🔍 Key Architectural Decisions

### **1. Stateless HTTP Service**
- No database persistence
- No queue management (CRD controllers handle retries)
- Single-pass delivery per request

### **2. Sensitive Data Sanitization**
- **CRITICAL SECURITY**: All data sanitized before notification
- Protects: Secrets, API keys, passwords, PII, connection strings
- Uses configurable regex patterns + semantic detection

### **3. Channel-Specific Formatting**
- Email: Rich HTML (1MB limit)
- Slack: Markdown blocks (40KB limit)
- Teams: Adaptive Cards (28KB limit)
- SMS: Plain text (160 chars)

### **4. External Service Action Links**
- Direct links to GitHub, Grafana, Prometheus, K8s Dashboard
- Authentication enforced by target service (not by Notification Service)
- No access control checking (BR-NOT-037)

---

## 🚀 Getting Started

### **For Implementation**
1. Read [overview.md](./overview.md) for architecture (10 min)
2. Read [api-specification.md](./api-specification.md) for HTTP API (15 min)
3. Review [testing-strategy.md](./testing-strategy.md) for TDD approach (10 min)
4. Follow [implementation-checklist.md](./implementation-checklist.md) for APDC-TDD phases (35 min)

**Total**: 70 minutes to full understanding

### **For Integration**
1. Read [integration-points.md](./integration-points.md) (5 min)
2. Review [api-specification.md](./api-specification.md) → "HTTP API Specification" section (10 min)
3. Check [security-configuration.md](./security-configuration.md) for auth requirements (5 min)

**Total**: 20 minutes to integrate

### **For Operations**
1. Read [observability-logging.md](./observability-logging.md) for metrics/logging (10 min)
2. Review [security-configuration.md](./security-configuration.md) for RBAC/secrets (10 min)

**Total**: 20 minutes for operational readiness

---

## 📁 Document Structure

```
notification-service/
├── README.md                       ← You are here
├── overview.md                     ← Architecture & business requirements
├── api-specification.md            ← HTTP API, data schemas
├── security-configuration.md       ← RBAC, secrets, sanitization
├── observability-logging.md        ← Metrics, logging, tracing
├── testing-strategy.md             ← Unit, integration, E2E tests
├── integration-points.md           ← Service dependencies
├── implementation-checklist.md     ← APDC-TDD implementation guide
├── RESTRUCTURE_COMPLETE.md         ← Historical restructuring record
├── RESTRUCTURING_COMPLETE.md       ← Historical restructuring record
└── archive/                        ← Historical working documents
    ├── triage/                    ← 28 issues identified & prioritized
    ├── solutions/                 ← Production-ready solutions
    ├── revisions/                 ← Architectural corrections
    └── summaries/                 ← Completion tracking
```

---

## 🎯 Implementation Effort

| Phase | Duration | Status |
|-------|----------|--------|
| **APDC Analysis** | 1-2 days | ⏸️ Pending |
| **TDD RED** | 2-3 days | ⏸️ Pending |
| **TDD GREEN** | 2-3 days | ⏸️ Pending |
| **TDD REFACTOR** | 2-3 days | ⏸️ Pending |
| **Total** | **8-10 days** | **⏸️ NOT STARTED** |

---

## 📊 Confidence Assessment

### **Overall Confidence**: 88%

**Strengths**:
- ✅ Clear business requirements (BR-NOT-026 through BR-NOT-037)
- ✅ Well-defined API contracts
- ✅ Comprehensive triage (28 issues resolved)
- ✅ Production-ready solutions documented

**Risks**:
- ⚠️ Channel adapter complexity (5 different formats)
- ⚠️ Sensitive data sanitization edge cases
- ⚠️ External service URL generation patterns

---

## 🔗 Related Services

### **Clients** (Services that call Notification Service)
1. **Alert Remediation** (Remediation Orchestrator) - Timeout escalations
2. **AI Analysis Controller** - Analysis-triggered escalations
3. **Workflow Execution Controller** - Workflow-triggered escalations
4. **Kubernetes Executor** - Execution failures

### **External Integrations**
- Email (SMTP)
- Slack (Webhooks)
- Microsoft Teams (Webhooks)
- SMS (Twilio/SNS)
- PagerDuty (API)

---

## 📚 Architecture References

- [Multi-CRD Reconciliation Architecture](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
- [Approved Microservices Architecture](../../architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md)
- [Service Connectivity Specification](../../architecture/SERVICE_CONNECTIVITY_SPECIFICATION.md)
- [Notification Payload Schema](../../architecture/specifications/notification-payload-schema.md)

---

## 🔄 Documentation Version History

| Version | Date | Changes |
|---------|------|---------|
| **1.0** | 2025-10-06 | Restructured from monolithic 06-notification-service.md (2,604 lines) to directory structure |
| **0.9** | 2025-10-02 | Original monolithic specification complete |

---

## 📞 Questions?

**Where should I start?**
→ Read [overview.md](./overview.md) for architecture overview (10 min)

**How do I integrate with this service?**
→ Read [integration-points.md](./integration-points.md) and [api-specification.md](./api-specification.md)

**What are the security requirements?**
→ Read [security-configuration.md](./security-configuration.md)

**How do I implement this service?**
→ Follow [implementation-checklist.md](./implementation-checklist.md) with APDC-TDD methodology

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete Restructure (from 2,604-line monolith)

