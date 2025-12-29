# DD-API-001: HTTP Header vs JSON Body Pattern

**Date**: November 28, 2025
**Status**: ✅ **APPROVED**
**Decision Maker**: Kubernaut Architecture Team
**Authority**: Kubernaut API Design Standards
**Affects**: All HTTP APIs across all services
**Version**: 1.0

---

## 📋 **Version History**

| Version | Date | Status | Notes |
|---------|------|--------|-------|
| 1.0 | Nov 28, 2025 | ✅ Approved | Initial design decision |

### Version 1.0 (November 28, 2025)
- **CREATED**: Established pattern for HTTP header vs JSON body usage
- **RATIONALE**: Ensure audit trail integrity and consistent API design
- **CROSS-REFERENCES**: DD-WORKFLOW-014 v2.1 (remediation_id pattern)

---

## 🎯 **Decision Summary**

**PRINCIPLE**: Business data goes in JSON body; infrastructure data goes in HTTP headers.

| Data Type | Transport | Rationale |
|-----------|-----------|-----------|
| **Business Logic Data** | JSON Body | Audit trail, proxy safety, single source of truth |
| **Infrastructure/Observability** | HTTP Headers | Standard practice, middleware processing |
| **Security/Auth** | HTTP Headers | Standard practice, security middleware |

---

## 📊 **Classification Matrix**

### ✅ **HTTP Headers (Infrastructure/Security)**

| Header | Purpose | Category |
|--------|---------|----------|
| `X-Request-ID` | Request tracing, logging | Infrastructure |
| `X-Correlation-ID` | Distributed tracing | Infrastructure |
| `X-Trace-ID`, `X-Span-ID`, `X-Parent-Span-ID` | OpenTelemetry | Infrastructure |
| `X-Forwarded-For`, `X-Real-IP` | Client IP extraction (proxy) | Infrastructure |
| `X-Response-Time` | Performance metrics | Infrastructure |
| `X-Service-Name`, `X-Service-Version` | Service identification | Infrastructure |
| `Authorization` | Authentication/Authorization | Security |
| `X-Content-Type-Options`, `X-Frame-Options` | Security headers | Security |
| `Content-Type`, `Accept` | Content negotiation | HTTP Standard |

### ❌ **JSON Body (Business Logic)**

| Data | Reason for JSON Body |
|------|---------------------|
| `remediation_id` | Audit trail correlation - must be logged with request |
| `reason` (disable workflow) | Audit trail - must be persisted with action |
| `event_type` (audit events) | Business classification - core to audit record |
| `resource_type`, `resource_id` | Business identifiers - core to audit record |
| `outcome` (audit events) | Business result - core to audit record |
| `labels`, `filters` | Query parameters - complex structured data |
| `workflow_id`, `version` | Entity identifiers - core to operation |

---

## 🔍 **Rationale**

### Why Business Data Must Be in JSON Body

1. **Audit Trail Integrity**
   - HTTP headers can be stripped/modified by proxies, load balancers, or WAFs
   - JSON body is preserved end-to-end
   - Single source of truth for logging and auditing

2. **Consistency**
   - All mutations use JSON body (POST, PUT, DELETE with body)
   - Reduces cognitive load for API consumers
   - Easier to document (single OpenAPI schema)

3. **Client SDK Generation**
   - OpenAPI generators handle JSON body automatically
   - Headers require manual handling in generated clients

4. **Logging & Debugging**
   - Request body logged as single unit
   - Headers often omitted from request logs
   - Easier to reproduce issues

5. **Proxy Compatibility**
   - Some proxies strip custom headers
   - JSON body always preserved
   - No header size limits to worry about

### Why Infrastructure Data Can Be in Headers

1. **Middleware Processing**
   - Request ID, correlation ID processed before body parsing
   - Enables early request tracking

2. **Standard Practice**
   - `X-Request-ID`, `X-Forwarded-For` are industry standards
   - Security headers (`X-Frame-Options`) are HTTP standards

3. **Cross-Cutting Concerns**
   - Infrastructure data applies to all requests
   - Not specific to any business operation

---

## 📝 **Implementation Guidelines**

### API Design Checklist

When designing a new API endpoint, ask:

1. **Is this data needed for audit trail?** → JSON Body
2. **Is this data a business identifier?** → JSON Body
3. **Is this data used for request routing/tracing?** → HTTP Header
4. **Is this data for security/auth?** → HTTP Header
5. **Would losing this data break the operation?** → JSON Body

### Example: Correct Pattern

```bash
# ✅ CORRECT: Business data in JSON body
curl -X DELETE http://api/v1/workflows/wf-123 \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: req-456" \           # Infrastructure - OK in header
  -H "X-Correlation-ID: corr-789" \      # Infrastructure - OK in header
  -d '{
    "reason": "Deprecated - replaced by v2"  # Business data - MUST be in body
  }'
```

### Example: Incorrect Pattern

```bash
# ❌ INCORRECT: Business data in HTTP header
curl -X DELETE http://api/v1/workflows/wf-123 \
  -H "X-Disable-Reason: Deprecated"  # Business data - should be in body
```

### Example: Audit Event API

```bash
# ✅ CORRECT: All audit fields in JSON body
curl -X POST http://data-storage:8080/api/v1/audit/events \
  -H "Content-Type: application/json" \
  -H "X-Correlation-ID: rr-2025-001" \   # Infrastructure - OK in header
  -d '{
    "event_type": "workflow.search.completed",
    "service": "data-storage",
    "resource_type": "workflow",
    "resource_id": "wf-123",
    "outcome": "success",
    "correlation_id": "rr-2025-001",      # Also in body for audit record
    "data": { ... }
  }'
```

---

## 🔗 **Cross-References**

| Document | Relevance |
|----------|-----------|
| [DD-WORKFLOW-014](./DD-WORKFLOW-014-workflow-selection-audit-trail.md) | `remediation_id` moved from header to body (v2.1) |
| [DD-STORAGE-011](../services/stateless/data-storage/implementation/DD-STORAGE-011-V1.1-IMPLEMENTATION-PLAN.md) | `reason` in JSON body for disable workflow |
| [ADR-034](./ADR-034-unified-audit-table-design.md) | Audit table design |
| [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE](../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) | Template updated with this pattern |

---

## ⚠️ **Migration Notes**

### Existing APIs to Review

| API | Current | Action |
|-----|---------|--------|
| Audit Event API (not implemented) | Headers proposed in V5.7/V5.8 docs | ❌ Fix before implementation |
| Workflow Disable (not implemented) | Header proposed | ✅ Fixed in DD-STORAGE-011 v3.2 |
| `remediation_id` (implemented) | Was header | ✅ Fixed in DD-WORKFLOW-014 v2.1 |

### External System Exceptions

| API | Header | Reason to Keep |
|-----|--------|----------------|
| Gateway Webhook | `X-Timestamp` | External systems (Prometheus, Alertmanager) use this |
| Gateway Webhook | `X-Signal-Source` | External system integration (optional, auto-detect preferred) |

---

## ✅ **Compliance Checklist**

For any new API endpoint:

- [ ] Business data is in JSON body
- [ ] Infrastructure data (request ID, correlation ID) is in headers
- [ ] Security data (Authorization) is in headers
- [ ] Audit-relevant data is in JSON body (even if also in header for tracing)
- [ ] OpenAPI spec reflects JSON body schema
- [ ] Tests validate JSON body parsing (not header extraction for business data)

---

## 📊 **Confidence Assessment**

| Criterion | Score | Evidence |
|-----------|-------|----------|
| Industry Alignment | 95% | REST API best practices, JSON:API spec |
| Audit Trail Integrity | 100% | JSON body preserved through proxies |
| Developer Experience | 90% | Consistent pattern, easier SDK generation |
| Migration Feasibility | 100% | No production APIs using headers for business data |

**Overall Confidence**: 96%

---

**Document Version**: 1.0
**Created**: November 28, 2025
**Last Updated**: November 28, 2025
**Status**: ✅ **APPROVED**

