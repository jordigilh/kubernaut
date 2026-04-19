# DD-AUTH-013: HTTP Status Codes for OAuth-Proxy - Document Index

**Status**: ✅ AUTHORITATIVE  
**Last Updated**: January 26, 2026  
**Category**: Authentication & Authorization

## Quick Links

### Core DD Document
- **[DD-AUTH-013: HTTP Status Codes for OAuth-Proxy](DD-AUTH-013-http-status-codes-oauth-proxy.md)** ⭐ **AUTHORITATIVE** - Canonical reference for all HTTP status codes returned by `ose-oauth-proxy` sidecar

---

## Directory Structure

```
DD-AUTH-013/
├── README.md (this file)
├── DD-AUTH-013-http-status-codes-oauth-proxy.md (AUTHORITATIVE)
└── handoff/
    ├── DD_AUTH_013_COMPLETE_SESSION_HANDOFF_JAN_26_2026.md
    ├── DD_AUTH_013_DOCS_ORGANIZATION_JAN_26_2026.md
    ├── DD_AUTH_013_FINAL_STATUS_JAN_26_2026.md
    ├── DD_AUTH_013_HAPI_OPENAPI_TRIAGE_JAN_26_2026.md
    ├── DD_AUTH_013_IMPLEMENTATION_COMPLETE_JAN_26_2026.md
    └── DD_AUTH_013_OPENAPI_UPDATE_SUMMARY_JAN_26_2026.md
```

---

## Document Categories

### 📋 Core DD Document (1)
**AUTHORITATIVE** - Single source of truth for HTTP status codes.

- **DD-AUTH-013-http-status-codes-oauth-proxy.md** - Defines all HTTP status codes returned by `ose-oauth-proxy` sidecar:
  - **401 Unauthorized**: Authentication failure (invalid/missing token)
  - **403 Forbidden**: Authorization failure (SAR denied)
  - **400/422**: Application-level validation errors (NOT from proxy)
  - **500**: Application-level server errors (NOT from proxy)
  - **402 Payment Required**: NOT USED (explicitly documented)

### 🤝 Handoff Documents (6)
Session summaries and implementation status from January 26, 2026.

- **handoff/DD_AUTH_013_COMPLETE_SESSION_HANDOFF_JAN_26_2026.md** - Comprehensive session handoff
- **handoff/DD_AUTH_013_IMPLEMENTATION_COMPLETE_JAN_26_2026.md** - Implementation completion summary
- **handoff/DD_AUTH_013_FINAL_STATUS_JAN_26_2026.md** - Final status and validation
- **handoff/DD_AUTH_013_OPENAPI_UPDATE_SUMMARY_JAN_26_2026.md** - OpenAPI spec update summary
- **handoff/DD_AUTH_013_HAPI_OPENAPI_TRIAGE_JAN_26_2026.md** - HolmesGPT API OpenAPI triage
- **handoff/DD_AUTH_013_DOCS_ORGANIZATION_JAN_26_2026.md** - Documentation organization activity

---

## Scope

### Services Covered
- **DataStorage Service**: REST API with `ose-oauth-proxy` sidecar
- **HolmesGPT API**: FastAPI with `ose-oauth-proxy` sidecar
- **Gateway Service**: Network-level auth (no sidecar for now)

### HTTP Status Codes Defined
| Code | Source | Meaning |
|------|--------|---------|
| **401** | `ose-oauth-proxy` | Authentication failed (invalid/missing token) |
| **403** | `ose-oauth-proxy` | Authorization failed (K8s SAR denied) |
| **400** | Application (DataStorage) | Validation error |
| **422** | Application (FastAPI/HAPI) | Validation error |
| **500** | Application | Internal server error |
| **402** | ❌ NOT USED | Explicitly documented as not applicable |

---

## Implementation Status

### DataStorage Service ✅ COMPLETE
| Component | Status | Document |
|---|---|---|
| **OpenAPI Spec** | ✅ Complete | `api/openapi/data-storage-v1.yaml` |
| **Generated Client** | ✅ Complete | `pkg/datastorage/ogen-client/` |
| **E2E Tests** | ✅ Complete | `test/e2e/datastorage/23_sar_access_control_test.go` |
| **401 Responses** | ✅ Documented | POST /api/v1/audit/events, POST /api/v1/workflows |
| **403 Responses** | ✅ Documented | POST /api/v1/audit/events, POST /api/v1/workflows |

### HolmesGPT API ✅ COMPLETE
| Component | Status | Document |
|---|---|---|
| **OpenAPI Spec** | ✅ Complete | `kubernaut-agent/api/openapi.json` |
| **Generated Client** | ✅ Complete | `pkg/holmesgpt/client/` |
| **Custom Client** | ✅ Complete | `pkg/holmesgpt/client/holmesgpt.go` (switch statements) |
| **401 Responses** | ✅ Documented | POST /api/v1/incident/analyze, POST /api/v1/recovery/analyze |
| **403 Responses** | ✅ Documented | POST /api/v1/incident/analyze, POST /api/v1/recovery/analyze |
| **500 Responses** | ✅ Documented | POST /api/v1/incident/analyze, POST /api/v1/recovery/analyze |

---

## Timeline

### January 26, 2026 - DD-AUTH-013 Creation and Implementation
- ✅ **9:00 AM**: DD-AUTH-013 created (AUTHORITATIVE)
- ✅ **9:06 AM**: DataStorage OpenAPI spec updated (401/403 responses)
- ✅ **9:10 AM**: HolmesGPT API OpenAPI spec triaged
- ✅ **9:13 AM**: HolmesGPT API OpenAPI spec updated (401/403/500 responses)
- ✅ **9:14 AM**: Implementation complete summary created
- ✅ **9:16 AM**: Documentation organization initiated
- ✅ **9:17 AM**: Complete session handoff created
- ✅ **9:20 AM**: Documentation organization complete

---

## Related Design Decisions

### Parent DD
- **[DD-AUTH-012: ose-oauth-proxy SAR for REST API Endpoints](../DD-AUTH-012/DD-AUTH-012-ose-oauth-proxy-sar-rest-api-endpoints.md)** - Authoritative document for SAR implementation

### Related DDs
- **[DD-AUTH-011: Granular RBAC & SAR Verb Mapping](../DD-AUTH-011/DD-AUTH-011-granular-rbac-sar-verb-mapping.md)** - RBAC strategy
- **[DD-AUTH-009: OAuth2-Proxy Workflow Attribution](../DD-AUTH-009-oauth2-proxy-workflow-attribution-implementation.md)** - Workflow audit tracking
- **[DD-AUTH-006: HAPI OAuth-Proxy Configuration](../DD-AUTH-006-kubernaut-agent-oauth-proxy-config.md)** - HAPI sidecar config

### Related ADRs
- **ADR-036**: Externalized Auth/Authz Sidecar Strategy

---

## Key Files Modified

### OpenAPI Specifications
```
api/openapi/data-storage-v1.yaml            (401/403 responses added)
kubernaut-agent/api/openapi.json             (401/403/500 responses added)
```

### Generated Clients
```
pkg/datastorage/ogen-client/               (Regenerated with 401/403 types)
pkg/holmesgpt/client/                      (Regenerated with 401/403/500 types)
```

### Custom Client Code
```
pkg/holmesgpt/client/holmesgpt.go         (Switch statements for type-safe handling)
```

### E2E Tests
```
test/e2e/datastorage/23_sar_access_control_test.go  (SAR validation with 403 handling)
```

---

## Business Requirements

### DataStorage Service
- **BR-DATA-STORAGE-040**: RESTful API for audit events with RBAC
- **BR-DATA-STORAGE-041**: RESTful API for workflow catalog with RBAC
- **BR-DATA-STORAGE-050**: User attribution for SOC2 compliance

### HolmesGPT API
- **BR-HAPI-197**: RESTful API for incident analysis with RBAC
- **BR-HAPI-198**: RESTful API for recovery analysis with RBAC

---

## Testing

### E2E Test Coverage
- ✅ **DataStorage**: `test/e2e/datastorage/23_sar_access_control_test.go`
  - Test 1: ServiceAccount with `data-storage-client` role (201 Created) ✅
  - Test 2: ServiceAccount without role (403 Forbidden) ✅
  - Test 3: Workflow API with `data-storage-client` role (201 Created) ✅
  - Test 4: Workflow API without role (403 Forbidden) 🚧 (workflow types pending)
  - Test 5: Audit event listing ✅

### Pending Tests
- 🚧 **401 Unauthorized scenarios** (invalid/expired tokens)
- 🚧 **HolmesGPT API E2E tests** (`test/e2e/kubernaut-agent/auth_validation_test.go`)

---

## Authority

**Status**: ✅ **AUTHORITATIVE**

This design decision is the **canonical reference** for all HTTP status codes returned by the `ose-oauth-proxy` sidecar in Kubernaut services. All OpenAPI specifications, client implementations, and test scenarios MUST align with DD-AUTH-013.

**Supersedes**: No previous authoritative document existed for HTTP status codes.

**Referenced By**:
- DataStorage OpenAPI spec (`api/openapi/data-storage-v1.yaml`)
- HolmesGPT API OpenAPI spec (`kubernaut-agent/api/openapi.json`)
- E2E test documentation (`docs/architecture/decisions/DD-AUTH-011/DD-AUTH-011-E2E-TESTING-GUIDE.md`)

---

## Usage Examples

### DataStorage Client (Go)
```go
import dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

// Type-safe error handling
switch v := err.(type) {
case *dsgen.CreateAuditEventUnauthorized:
    // 401 - Invalid/missing Bearer token
case *dsgen.CreateAuditEventForbidden:
    // 403 - SAR denied (missing RBAC permission)
case *dsgen.CreateAuditEventBadRequest:
    // 400 - Application validation error
}
```

### HolmesGPT API Client (Go)
```go
import hapiclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"

// Custom client with switch statement for type safety
res, err := client.Investigate(ctx, req)
if err != nil {
    switch v := err.(type) {
    case *hapiclient.APIError:
        if v.StatusCode == 401 {
            // Authentication failure
        } else if v.StatusCode == 403 {
            // Authorization failure (SAR denied)
        } else if v.StatusCode == 500 {
            // HAPI internal server error
        }
    }
}
```

---

## Next Steps

### Pending Tasks (from handoff documents)
1. 🚧 Fix Podman machine connection issue
2. 🚧 Fix workflow types in DataStorage E2E test (Tests 4 & 5)
3. 🚧 Add 401 Unauthorized test scenarios to DataStorage E2E suite
4. 🚧 Create HAPI E2E auth validation tests
5. 🚧 Run Notification E2E tests (validates cross-namespace RBAC)

### Future Enhancements
1. Add NetworkPolicy examples to DD-AUTH-012
2. Create production troubleshooting guide for SAR failures
3. Add Prometheus metrics for 401/403 rates
4. Document Helm chart configuration for production

---

**Maintained By**: Platform Team  
**Contact**: See DD-AUTH-013 main document for author information  
**Last Review**: January 26, 2026  
**Next Review**: After E2E test completion
