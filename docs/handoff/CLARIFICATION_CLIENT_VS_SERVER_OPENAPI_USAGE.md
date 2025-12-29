# ❓ CLARIFICATION: Client vs. Server OpenAPI Usage

**To**: All Service Teams
**From**: Data Storage Team
**Date**: December 15, 2025
**Re**: [CROSS_SERVICE_OPENAPI_EMBED_MANDATE.md](./CROSS_SERVICE_OPENAPI_EMBED_MANDATE.md) - Team Questions

---

## 🎯 **Question from Teams**

> "Do we need to add the same file to our code so our Data Storage client can validate the payloads?"

**Short Answer**: ❌ **NO** - You do NOT need to embed specs for validation.
**What You Need**: ✅ **YES** - Generate type-safe clients from specs (optional but recommended).

---

## 📊 **Two Different Use Cases**

The mandate document covers TWO DISTINCT use cases. Most teams only need one.

| Use Case | Who Needs This? | Purpose | Required? |
|----------|----------------|---------|-----------|
| **1. Server-Side Validation** | Services that PROVIDE REST APIs | Validate INCOMING requests | Only Data Storage |
| **2. Client-Side Type Safety** | Services that CONSUME REST APIs | Generate type-safe clients | All consumers (recommended) |

---

## 🔍 **Use Case 1: Server-Side Validation (Embedding Specs)**

**Who Needs This**: Services that PROVIDE REST APIs with OpenAPI validation middleware

**Current V1.0 Services**:
- ✅ **Data Storage** (already done) - Validates incoming audit/workflow requests
- ✅ **Audit Shared Library** (already done) - Pre-validates audit events before sending

**Future Services** (V2.0+):
- Gateway (if adding OpenAPI validation)
- Notification (if adding OpenAPI validation)
- HolmesGPT API (if adding OpenAPI validation)

### Why Embedding for Validation?

**Problem**: Server needs to load OpenAPI spec to validate incoming HTTP requests
**Solution**: Embed spec in server binary so it's always available

**Example**: Data Storage Service

```go
// pkg/datastorage/server/middleware/openapi_spec.go
package middleware

import _ "embed"

//go:generate sh -c "cp ../../../../api/openapi/data-storage-v1.yaml openapi_spec_data.yaml"
//go:embed openapi_spec_data.yaml
var embeddedOpenAPISpec []byte

// Used by validation middleware to check incoming requests
```

**Validation Flow**:
```
Incoming HTTP Request (from Gateway/SP/etc.)
  ↓
Data Storage Validation Middleware
  ↓
Load embeddedOpenAPISpec
  ↓
Validate request against spec
  ↓
✅ Valid → Process request
❌ Invalid → Return HTTP 400 (RFC 7807)
```

**Result**: Data Storage rejects invalid requests automatically.

---

## 🔍 **Use Case 2: Client-Side Type Safety (Generating Clients)**

**Who Needs This**: Services that CONSUME REST APIs (call Data Storage, HAPI, etc.)

**V1.0 Services That Consume Data Storage**:
- ✅ **Gateway** - Sends audit events
- ✅ **SignalProcessing** - Sends audit events
- ✅ **AIAnalysis** - Sends audit events, queries workflows
- ✅ **RemediationOrchestrator** - Sends audit events
- ✅ **WorkflowExecution** - Sends audit events
- ✅ **Notification** - Sends audit events

**V1.0 Services That Consume HAPI**:
- ✅ **AIAnalysis** - Calls HolmesGPT for investigations

### Why Client Generation?

**Problem**: Manually writing HTTP client code is error-prone and tedious
**Solution**: Auto-generate type-safe clients from OpenAPI spec

**Example**: Gateway calling Data Storage

**BEFORE** (Manual Client - ❌ Error-Prone):
```go
// pkg/gateway/datastorage/client.go
func (c *Client) SendAudit(ctx context.Context, event map[string]interface{}) error {
    body, _ := json.Marshal(event) // No type safety!
    req, _ := http.NewRequest("POST", c.baseURL+"/api/v1/audit/events", bytes.NewReader(body))
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return err
    }
    if resp.StatusCode != 201 {
        return fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }
    return nil
}

// Usage (NO compile-time safety):
client.SendAudit(ctx, map[string]interface{}{
    "event_type": "gateway.request.received",
    "event_tmestamp": time.Now(), // ❌ Typo! Runtime error only
})
```

**AFTER** (Generated Client - ✅ Type-Safe):
```go
// pkg/gateway/clients/datastorage/generated.go (AUTO-GENERATED)
type AuditEventRequest struct {
    EventType      string                 `json:"event_type"`      // Required
    EventTimestamp time.Time              `json:"event_timestamp"` // Required
    EventSource    string                 `json:"event_source"`    // Required
    EventData      map[string]interface{} `json:"event_data"`
}

func (c *Client) CreateAuditEvent(ctx context.Context, req *AuditEventRequest) (*AuditEventResponse, error) {
    // Generated HTTP client logic with proper error handling
}

// Usage (COMPILE-TIME safety):
client.CreateAuditEvent(ctx, &datastorage.AuditEventRequest{
    EventType:      "gateway.request.received",
    EventTimestamp: time.Now(),
    EventSource:    "gateway-service",
    EventData:      map[string]interface{}{"method": "POST"},
})
// ✅ Compiler catches missing required fields!
// ✅ Compiler catches typos (event_tmestamp → event_timestamp)!
```

**Client Generation Flow**:
```
api/openapi/data-storage-v1.yaml (source of truth)
  ↓
go:generate oapi-codegen (run automatically on build)
  ↓
pkg/gateway/clients/datastorage/generated.go (auto-generated)
  ↓
Gateway code imports and uses type-safe client
  ↓
✅ Compile-time safety
✅ Auto-updated when spec changes
✅ No manual HTTP client code
```

---

## 🤔 **Which Use Case Do I Need?**

### **Decision Matrix**

| Your Service | You PROVIDE REST API? | You CONSUME REST API? | What You Need |
|--------------|----------------------|----------------------|---------------|
| **Data Storage** | ✅ Yes (audit, workflow APIs) | ❌ No | ✅ Use Case 1 (embed spec for validation) |
| **Audit Library** | ✅ Yes (internal validation) | ❌ No | ✅ Use Case 1 (embed spec for validation) |
| **Gateway** | ❌ No OpenAPI validation | ✅ Yes (calls Data Storage) | ✅ Use Case 2 (generate DS client) |
| **SignalProcessing** | ❌ No OpenAPI validation | ✅ Yes (calls Data Storage) | ✅ Use Case 2 (generate DS client) |
| **AIAnalysis** | ❌ No OpenAPI validation | ✅ Yes (calls DS + HAPI) | ✅ Use Case 2 (generate DS + HAPI clients) |
| **RemediationOrchestrator** | ❌ No OpenAPI validation | ✅ Yes (calls Data Storage) | ✅ Use Case 2 (generate DS client) |
| **WorkflowExecution** | ❌ No OpenAPI validation | ✅ Yes (calls Data Storage) | ✅ Use Case 2 (generate DS client) |
| **Notification** | ❌ No OpenAPI validation | ✅ Yes (calls Data Storage) | ✅ Use Case 2 (generate DS client) |

---

## ✅ **What Teams Should Do**

### **Phase 3: Data Storage Client Consumers** (Gateway, SP, RO, WE, Notification)

**Status**: 📋 **OPTIONAL BUT RECOMMENDED** (not blocking V1.0)

**What to Implement**: Generate type-safe Data Storage client from `api/openapi/data-storage-v1.yaml`

**Benefits**:
1. ✅ **Compile-Time Safety**: Catch API contract violations at build time
2. ✅ **Auto-Sync**: Client updates automatically when spec changes
3. ✅ **Zero Drift**: Client always matches server API
4. ✅ **Less Code**: No manual HTTP client implementation
5. ✅ **Better Errors**: Type-safe error handling

**Effort**: 15-20 minutes per service

**Guide**: [CROSS_SERVICE_GO_GENERATE_IMPLEMENTATION_GUIDE.md](./CROSS_SERVICE_GO_GENERATE_IMPLEMENTATION_GUIDE.md) (Step 2A: Client Generation)

**Example Implementation**:
```go
// pkg/gateway/clients/datastorage/openapi_spec.go (NEW FILE)
package datastorage

//go:generate sh -c "cp ../../../../api/openapi/data-storage-v1.yaml openapi_spec_data.yaml"
//go:generate oapi-codegen -package datastorage -generate types,client openapi_spec_data.yaml > generated.go
```

**Makefile Integration**:
```makefile
.PHONY: generate-clients
generate-clients: ## Generate OpenAPI clients
	@echo "📦 Generating Data Storage client..."
	go generate ./pkg/gateway/clients/datastorage/...

.PHONY: build-gateway
build-gateway: generate-clients ## Build gateway service
	go build -o bin/gateway ./cmd/gateway
```

**Result**: Type-safe Data Storage client automatically generated and kept in sync.

---

### **Phase 4: HAPI Client Consumer** (AIAnalysis)

**Status**: 📋 **OPTIONAL BUT RECOMMENDED** (not blocking V1.0)

**What to Implement**: Generate type-safe HAPI client from `holmesgpt-api/api/openapi/hapi-v1.yaml`

**Same benefits as Phase 3, but for HAPI API.**

---

## ❌ **What Teams Should NOT Do**

### **DO NOT Embed Specs for Validation** (unless you provide REST APIs)

**Common Mistake**:
```go
// ❌ WRONG: Gateway trying to embed Data Storage spec for validation
// pkg/gateway/middleware/datastorage_validator.go

//go:embed ../../../../api/openapi/data-storage-v1.yaml
var embeddedDataStorageSpec []byte

func (g *Gateway) validateBeforeSending(req *AuditRequest) error {
    // ❌ NO! Client-side validation is redundant and error-prone
    // The Data Storage server ALREADY validates incoming requests!
    return validateAgainstSpec(req, embeddedDataStorageSpec)
}
```

**Why This is Wrong**:
1. ❌ **Redundant**: Data Storage server already validates (you can't bypass it)
2. ❌ **Double Maintenance**: Two validation points to keep in sync
3. ❌ **Spec Drift Risk**: Client validation might differ from server
4. ❌ **False Confidence**: Passing client validation ≠ server acceptance

**Correct Approach**:
```go
// ✅ CORRECT: Use generated client, let server validate
client := datastorage.NewClient(baseURL)
resp, err := client.CreateAuditEvent(ctx, &datastorage.AuditEventRequest{
    EventType: "gateway.request.received",
    // ... type-safe struct fields ...
})
// Server validates and returns 400 if invalid (RFC 7807)
if err != nil {
    // Handle server validation errors
    return fmt.Errorf("audit event rejected by server: %w", err)
}
```

**Result**: Server validation is the single source of truth. Client code is simpler.

---

## 📋 **FAQ**

### Q1: Is client generation required for V1.0?

**A**: ❌ **NO** - Client generation is OPTIONAL but RECOMMENDED.

**Why Optional**: Your current manual HTTP client code works fine.

**Why Recommended**:
- Type safety catches bugs at compile time
- Auto-sync prevents spec drift
- Less code to maintain

**Timeline**: January 15, 2026 (non-blocking)

---

### Q2: If Data Storage validates incoming requests, why generate clients?

**A**: Type safety at compile time vs. runtime errors.

**Without Generated Client**:
```go
// Manual client - runtime error
req := map[string]interface{}{
    "event_type": "audit.created",
    // ❌ Missing required "event_timestamp" field
}
resp, err := httpClient.Post(url, req) // ✅ Compiles fine
// ❌ Data Storage returns HTTP 400 at runtime
```

**With Generated Client**:
```go
// Generated client - compile error
req := &datastorage.AuditEventRequest{
    EventType: "audit.created",
    // ❌ Compiler error: missing required field "EventTimestamp"
}
// Won't even compile until you fix it!
```

**Result**: Catch API contract violations during development, not in production.

---

### Q3: Do I need to validate payloads before sending to Data Storage?

**A**: ❌ **NO** - You don't need to implement validation yourself.

**Why**:
- ✅ **Audit Library does client-side validation internally** (transparent to you)
- ✅ **Data Storage does server-side validation** (final authority)
- ✅ **You just use Audit Library API** - validation happens automatically

**Data Storage Server Validates**:
1. ✅ Required fields present
2. ✅ Field types correct
3. ✅ Enum values valid
4. ✅ Returns RFC 7807 errors (HTTP 400) if invalid

**Your Responsibility**:
1. ✅ Handle HTTP 400 errors gracefully (if they occur)
2. ✅ Handle validation errors from `auditStore.StoreAudit()`
3. ✅ Log validation failures for debugging
4. ✅ Use generated client for type safety (optional)

**DO NOT**:
- ❌ Embed Data Storage spec in YOUR service code (Audit Library already has it)
- ❌ Duplicate validation logic in YOUR service (Audit Library already does it)
- ❌ Parse OpenAPI spec in YOUR service runtime (Audit Library already does it)

**Result**: Use Audit Library → validation handled transparently by shared library

---

### Q3.5: Wait, does Audit Library do client-side validation or not?

**A**: ✅ **YES** - But you don't need to think about it!

**Under the Hood** (Audit Library Implementation):
1. ✅ Audit Library embeds Data Storage OpenAPI spec (`pkg/audit/openapi_spec.go`)
2. ✅ Audit Library validates events BEFORE sending (`pkg/audit/openapi_validator.go`)
3. ✅ Catches validation errors early (~1-2μs vs 10ms+ network round-trip)
4. ✅ Same validation rules as Data Storage server (same spec embedded in both)

**Why This Design** (Defense-in-Depth):
- **Early Error Detection**: Catch bugs in development, not production
- **Performance**: Avoid network round-trip for invalid events
- **Developer Experience**: Clear errors with field-level details
- **Zero Drift**: Same spec validates on both client and server sides

**Your Responsibility** (Service Team):
1. ✅ Use Audit Library API (`audit.NewAuditEventRequest()`, `audit.Set*()`)
2. ✅ Handle validation errors from `auditStore.StoreAudit()`
3. ❌ Don't implement your own validation (Audit Library does it)
4. ❌ Don't embed your own copy of the spec (Audit Library has it)

**Example - What Actually Happens**:
```go
// Your service code (Gateway, SP, RO, WE, etc.)
event := audit.NewAuditEventRequest()
audit.SetEventType(event, "gateway.signal.received")

// Audit Library validates HERE (before network call)
err := auditStore.StoreAudit(ctx, event)
// ↑ Validation happens inside Audit Library (transparent to you)
// ↑ Uses embedded OpenAPI spec (same as Data Storage server)
// ↑ Catches errors early (performance benefit)

if err != nil {
    // Could be validation error OR network error
    log.Error(err, "Failed to store audit event")
}
```

**Result**: Audit Library does client-side validation for you - just use the API!

---

### Q4: What's the difference between embedding and generating?

**A**: Two completely different use cases:

**Embedding** (Server-Side Validation):
```go
//go:embed openapi_spec.yaml
var spec []byte // Loaded at compile time, used at runtime for validation
```
- **Purpose**: Validate INCOMING HTTP requests
- **Who**: Services that PROVIDE REST APIs (Data Storage)
- **Result**: Reject invalid requests with HTTP 400

**Generating** (Client-Side Type Safety):
```go
//go:generate oapi-codegen ... > generated.go
// Creates type-safe Go client code from spec
```
- **Purpose**: Generate type-safe client code
- **Who**: Services that CONSUME REST APIs (Gateway, SP, etc.)
- **Result**: Compile-time safety, auto-sync with spec

**You can do both, one, or neither** - depends on your service role.

---

### Q5: I'm still confused. What should I do?

**A**: Answer these questions:

**Question 1**: Does your service PROVIDE a REST API with OpenAPI validation?
- ✅ **YES** → You need Use Case 1 (embed spec for validation)
- ❌ **NO** → Skip Use Case 1

**Question 2**: Does your service CALL Data Storage or HAPI APIs?
- ✅ **YES** → You should consider Use Case 2 (generate client)
- ❌ **NO** → Skip Use Case 2

**For Most Teams** (Gateway, SP, RO, WE, Notification):
- ❌ You do NOT provide REST APIs with validation → Skip Use Case 1
- ✅ You DO call Data Storage → Consider Use Case 2 (client generation)

**Result**: Most teams only need client generation (optional, not validation).

---

## 📊 **Summary Table**

| Team | Embed Spec? | Generate Client? | Deadline | Priority |
|------|-------------|------------------|----------|----------|
| **Data Storage** | ✅ Done (validation) | N/A | ✅ Complete | P0 |
| **Audit Library** | ✅ Done (validation + embed)* | N/A | ✅ Complete | P0 |
| **Gateway** | ❌ No | ✅ Yes (DS client) | Jan 15, 2026 | P1 (optional) |
| **SignalProcessing** | ❌ No | ✅ Yes (DS client) | Jan 15, 2026 | P1 (optional) |
| **AIAnalysis** | ❌ No | ✅ Yes (DS + HAPI) | Jan 15, 2026 | P1 (optional) |
| **RemediationOrchestrator** | ❌ No | ✅ Yes (DS client) | Jan 15, 2026 | P1 (optional) |
| **WorkflowExecution** | ❌ No | ✅ Yes (DS client) | Jan 15, 2026 | P1 (optional) |
| **Notification** | ❌ No | ✅ Yes (DS client) | Jan 15, 2026 | P1 (optional) |

**\*Note on Audit Library**: Audit Library uses embedded OpenAPI spec for client-side validation (transparent to consuming services). All services (Gateway, SP, RO, WE, Notification, AIAnalysis) automatically get this validation by using `audit.NewBufferedStore()`. Services use Audit Library API - validation happens automatically without service-specific implementation.

---

## 📞 **Contact**

**Questions**: Reach out to Data Storage Team
**Implementation Guide**: [CROSS_SERVICE_GO_GENERATE_IMPLEMENTATION_GUIDE.md](./CROSS_SERVICE_GO_GENERATE_IMPLEMENTATION_GUIDE.md)
**Design Decision**: [DD-API-002: OpenAPI Spec Loading Standard](../architecture/decisions/DD-API-002-openapi-spec-loading-standard.md)

---

**Document Version**: 1.1
**Last Updated**: December 15, 2025 (Updated to clarify Audit Library client-side validation)
**Status**: ✅ **CLARIFICATION COMPLETE** (Enhanced)

**Version History**:
- **v1.1** (Dec 15, 2025): Added FAQ Q3.5 "Under the Hood" section, updated Q3 to acknowledge Audit Library validation, added footnote to summary table
- **v1.0** (Dec 15, 2025): Initial clarification document from Data Storage team

