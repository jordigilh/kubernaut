# Data Storage API client integration

Short guide for calling the Data Storage REST API from Go using the **ogen**-generated client in `pkg/datastorage/ogen-client`.

## Generated client (`ogen`)

The directory `pkg/datastorage/ogen-client` is generated from `api/openapi/data-storage-v1.yaml` (see `gen.go`). The **Go package name is `api`**, imported from module path **`github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client`**.

### Constructing the client

```go
import (
    dsapi "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

client, err := dsapi.NewClient("https://data-storage.namespace.svc:8443")
if err != nil {
    return err
}

// Typical production wiring: timeouts + transport that attaches identity headers
inner := &http.Client{Timeout: 30 * time.Second, Transport: yourTransport()}
client, err = dsapi.NewClient(baseURL, dsapi.WithClient(inner))
```

Pass a configured **`http.Client`** with **`Transport`** wrapping (see Authentication) whenever you inject headers, custom TLS, or budgets; tests and services use **`ogenclient.WithClient(&http.Client{...})`** the same way.

### Authentication: `X-Auth-Request-User` (oauth-proxy pattern)

Production paths expect **`X-Auth-Request-User`** to identify the authenticated subject after oauth-proxy validates the bearer token/session. Typical approach: wrap **`http.Transport`** so every request adds:

```go
req.Header.Set("X-Auth-Request-User", username)
```

Pass that **`http.Client`** with `dsapi.WithClient`. Without **`X-Auth-Request-User`** on routes that enforce identity, callers receive **401** with **RFC 7807** bodies.

Do not forward spoofed impersonation headers from untrusted callers; gateways strip or inject authoritative identity.

### Key endpoints (examples)

| Use case | Client method | HTTP |
|----------|---------------|------|
| Batch audit write | `CreateAuditEventsBatch(ctx, []AuditEventRequest)` | `POST /api/v1/audit/events/batch` |
| Signed export | `ExportAuditEvents(ctx, ExportAuditEventsParams)` | `GET /api/v1/audit/export` (+ query filters) |
| Hash chain verification | `VerifyAuditChain(ctx, &VerifyChainRequest{...})` | `POST /api/v1/audit/verify-chain` |
| RR reconstruction | `ReconstructRemediationRequest(ctx, ReconstructRemediationRequestParams)` | `POST /api/v1/audit/remediation-requests/{correlation_id}/reconstruct` |

Consult `oas_client_gen.go` (method names mirror OpenAPI operations) or the YAML for precise parameters and enums.

### Error handling: RFC 7807 Problem Details

Validation, auth policy, conflicts, rate limits, and many server faults return **RFC 7807 Problem Details**. The generator maps these to **`RFC7807Problem`**-derived response types (`CreateWorkflowBadRequest`, `ExportAuditEventsUnauthorized`, etc.).

Recommended pattern:

1. Inspect the **typed result** discriminator for each operation (ogon `*Res` unions).
2. Use **`errors.As`** on returned errors where the wrapper carries HTTP status/metadata.
3. Log **`RFC7807Problem.Title`**, **`Detail`**, and **`Type`** (URI) for diagnostics; surface **`Detail`** to operators, not raw upstream SQL/DLQ messages.

This keeps callers aligned with the API contract and avoids leaking internals.
