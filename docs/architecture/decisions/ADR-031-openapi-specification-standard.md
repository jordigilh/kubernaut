# ADR-031: OpenAPI Specification Standard for REST APIs

**Status**: ✅ **APPROVED**
**Date**: November 2, 2025
**Applies To**: All stateless services with REST APIs
**Exception**: Gateway Service (webhook receiver, not REST API)

---

## Context

Kubernaut has multiple stateless services that expose REST APIs for inter-service communication:
- **Data Storage Service**: READ API for incident queries
- **Context API Service**: Context orchestration API
- **Notification Service**: Alert notification API
- **Dynamic Toolset Service**: Dynamic tool registration API

Currently, these services lack standardized API documentation, requiring manual client implementation for each consuming service. This leads to:
- **Inconsistency**: Different services use different API patterns
- **Maintenance Burden**: Manual clients require updates when APIs change
- **Type Safety Issues**: Manual clients prone to field name typos, type mismatches
- **Schema Drift**: Clients diverge from API implementation over time
- **Documentation Gaps**: API consumers must read source code to understand endpoints

**Industry Standard**: OpenAPI Specification (OAS) 3.0+ is the de facto standard for REST API documentation, used by AWS, Google Cloud, Azure, Stripe, GitHub, and Kubernetes.

---

## Decision

**MANDATE**: All stateless services that expose REST APIs MUST provide an OpenAPI 3.0+ specification.

### Scope

**Included Services** (REST API providers):
- ✅ Data Storage Service
- ✅ Context API Service
- ✅ Notification Service
- ✅ Dynamic Toolset Service
- ✅ Effectiveness Monitor Service (future)

**Excluded Services**:
- ❌ **Gateway Service**: Webhook receiver, not REST API provider
- ❌ CRD Controllers: Use Kubernetes API, not REST

---

## OpenAPI Specification Standard

### 1. Specification File Location

**Standard Directory Structure**:
```
docs/services/stateless/<service>/
├── README.md
├── api-specification.md          # Human-readable API docs
├── openapi/
│   ├── v1.yaml                    # OpenAPI 3.0+ spec for API v1
│   ├── v2.yaml                    # OpenAPI 3.0+ spec for API v2 (when released)
│   └── README.md                  # OpenAPI versioning guide
└── implementation/
    └── IMPLEMENTATION_PLAN_*.md
```

**Example**: Data Storage Service
```
docs/services/stateless/data-storage/
├── README.md
├── api-specification.md
├── openapi/
│   ├── v1.yaml                    # Current: Phase 1 Read API
│   └── v2.yaml                    # Future: Phase 2 Write API
└── implementation/
    └── API-GATEWAY-MIGRATION.md
```

---

### 2. API Versioning Strategy

**URL Versioning** (Industry Standard):
- **Pattern**: `/api/v{major}/resource`
- **Examples**:
  - `/api/v1/incidents`
  - `/api/v2/incidents` (breaking changes)
  - `/api/v1/health` (health endpoints versioned separately or unversioned)

**File Versioning** (Separate OpenAPI files per major version):
- **Pattern**: `docs/services/stateless/<service>/openapi/v{major}.yaml`
- **Rationale**: Major API versions have different schemas, endpoints, behavior
- **Examples**:
  - `openapi/v1.yaml` (API v1)
  - `openapi/v2.yaml` (API v2 with breaking changes)

**Semantic Versioning**:
- **Major**: Breaking changes → New OpenAPI file (`v2.yaml`)
- **Minor**: Additive changes → Update existing OpenAPI file (e.g., new endpoint)
- **Patch**: Bug fixes → Update existing OpenAPI file (e.g., clarify description)

**Git Tagging**:
- Tag OpenAPI specs with service version: `data-storage-v1.0.0`
- CI/CD generates spec archive on release

---

### 3. OpenAPI Document Structure

**Required Sections**:

```yaml
openapi: 3.0.3
info:
  title: Data Storage Service API
  version: 1.0.0
  description: |
    REST API for querying Kubernetes remediation action history.
    This API provides read-only access to incident data.
  contact:
    name: Kubernaut Team
    url: https://github.com/jordigilh/kubernaut
  license:
    name: Apache 2.0
    url: https://www.apache.org/licenses/LICENSE-2.0.html

servers:
  - url: http://data-storage.kubernaut-system.svc.cluster.local:8080
    description: Kubernetes cluster (in-cluster)
  - url: http://localhost:8080
    description: Local development

paths:
  /api/v1/incidents:
    get:
      summary: List incidents with filters
      operationId: listIncidents
      tags:
        - Incidents
      parameters:
        - name: alert_name
          in: query
          schema:
            type: string
          description: Filter by alert name pattern
        - name: severity
          in: query
          schema:
            type: string
            enum: [low, medium, high, critical]
          description: Filter by alert severity
      responses:
        '200':
          description: Successful response
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/IncidentListResponse'
        '400':
          description: Bad request
          content:
            application/problem+json:
              schema:
                $ref: '#/components/schemas/RFC7807Error'

components:
  schemas:
    IncidentListResponse:
      type: object
      required:
        - data
        - pagination
      properties:
        data:
          type: array
          items:
            $ref: '#/components/schemas/Incident'
        pagination:
          $ref: '#/components/schemas/Pagination'

    Incident:
      type: object
      required:
        - id
        - alert_name
        - alert_severity
        - action_timestamp
      properties:
        id:
          type: integer
          format: int64
          example: 12345
        alert_name:
          type: string
          example: "prod-cpu-high"
        alert_severity:
          type: string
          enum: [low, medium, high, critical]
          example: "critical"

    RFC7807Error:
      type: object
      required:
        - type
        - title
        - status
      properties:
        type:
          type: string
          format: uri
          example: "https://kubernaut.io/errors/invalid-filter"
        title:
          type: string
          example: "Invalid Filter Parameter"
        status:
          type: integer
          example: 400
        detail:
          type: string
          example: "The 'severity' filter value must be one of: low, medium, high, critical"
```

---

### 4. Tooling Standards

#### **Spec Generation** (Code → OpenAPI)

**Recommended Tool**: [`swaggo/swag`](https://github.com/swaggo/swag) for Go

**Usage**:
```go
// In handler code, add Swagger annotations
// @Summary List incidents with filters
// @Description Retrieve incidents filtered by alert name, severity, etc.
// @Tags Incidents
// @Accept json
// @Produce json
// @Param alert_name query string false "Filter by alert name"
// @Param severity query string false "Filter by severity" Enums(low, medium, high, critical)
// @Success 200 {object} IncidentListResponse
// @Failure 400 {object} RFC7807Error
// @Router /api/v1/incidents [get]
func (h *Handler) ListIncidents(w http.ResponseWriter, r *http.Request) {
    // Implementation
}
```

```bash
# Generate OpenAPI spec from annotations
swag init --dir ./cmd/datastorage --output ./docs/services/stateless/data-storage/openapi/
```

---

#### **Client Generation** (OpenAPI → Go Client)

**Recommended Tool**: [`oapi-codegen`](https://github.com/deepmap/oapi-codegen)

**Usage**:
```bash
# Generate Go client from OpenAPI spec
oapi-codegen \
  --package datastorage \
  --generate types,client \
  --old-config-style \
  docs/services/stateless/data-storage/openapi/v1.yaml \
  > pkg/datastorage/client/client.go
```

**Result**: Type-safe Go client with:
- ✅ Auto-generated structs (no manual typing)
- ✅ Request/response validation
- ✅ Enum types
- ✅ OpenAPI-compliant error handling

---

#### **Spec Validation**

**Recommended Tool**: [`openapi-generator validate`](https://openapi-generator.tech/)

```bash
# Validate OpenAPI spec
openapi-generator validate \
  -i docs/services/stateless/data-storage/openapi/v1.yaml
```

---

### 5. CI/CD Integration

**Mandatory CI/CD Checks** (for all PR changes to services with REST APIs):

```yaml
# .github/workflows/openapi-validation.yaml
name: OpenAPI Validation

on:
  pull_request:
    paths:
      - 'docs/services/**/openapi/*.yaml'
      - 'pkg/*/server/*.go'
      - 'cmd/*/main.go'

jobs:
  validate-openapi:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v3

      - name: Validate OpenAPI specs
        run: |
          for spec in docs/services/stateless/*/openapi/*.yaml; do
            echo "Validating $spec"
            openapi-generator validate -i "$spec"
          done

      - name: Check spec-code sync (Data Storage)
        run: |
          # Generate spec from code annotations
          swag init --dir ./cmd/datastorage --output /tmp/generated-spec/

          # Compare with committed spec
          diff -u docs/services/stateless/data-storage/openapi/v1.yaml /tmp/generated-spec/swagger.yaml

      - name: Generate clients for testing
        run: |
          # Generate Context API client from Data Storage spec
          oapi-codegen \
            --package datastorage \
            --generate types,client \
            docs/services/stateless/data-storage/openapi/v1.yaml \
            > /tmp/datastorage-client.go

          # Verify generated client compiles
          go build /tmp/datastorage-client.go
```

---

### 6. Documentation Standards

**Swagger UI Deployment**:
- Deploy Swagger UI for each service in development/staging environments
- URL: `http://<service>.kubernaut-system.svc.cluster.local:8080/swagger/`

**README.md Requirements**:
Each service's README must include:
```markdown
## API Documentation

### OpenAPI Specification
- **Version 1**: [openapi/v1.yaml](./openapi/v1.yaml)
- **Swagger UI**: http://data-storage.kubernaut-system.svc.cluster.local:8080/swagger/

### API Versioning
- Current: `v1` (Phase 1: Read API)
- Future: `v2` (Phase 2: Write API with breaking changes)

### Generating Clients
```bash
# Generate Go client
oapi-codegen --package datastorage --generate types,client openapi/v1.yaml > client.go
```
```

---

## Benefits

### 1. Type Safety & Consistency (High Impact)
- ✅ Auto-generated clients eliminate manual typing errors
- ✅ Schema validation ensures request/response correctness
- ✅ Enum types prevent invalid values

### 2. Reduced Maintenance Burden (High Impact)
- ✅ Update spec once, regenerate all clients
- ✅ No schema drift between API and clients
- ✅ CI/CD validates spec-code sync

### 3. Improved Developer Experience (Medium Impact)
- ✅ Self-documenting APIs with Swagger UI
- ✅ Clear API contracts before implementation
- ✅ Faster client integration (regenerate vs. manual coding)

### 4. Industry Alignment (Medium Impact)
- ✅ OpenAPI is industry standard (AWS, Google, Azure, Stripe, GitHub)
- ✅ Mature tooling ecosystem (Swagger, Postman, Insomnia)
- ✅ Easier onboarding for new developers

---

## Risks & Mitigation

### Risk 1: Spec-Code Drift (High Impact, Medium Probability)
**Risk**: OpenAPI spec diverges from actual implementation.

**Mitigation**:
- ✅ Use code-first generation (`swaggo/swag` annotations in Go code)
- ✅ CI/CD validation: Compare generated spec vs. committed spec
- ✅ Make spec updates mandatory in PRs changing API endpoints

**Severity**: MEDIUM (fixable, but breaks consuming services)

---

### Risk 2: Initial Setup Time (Medium Impact, High Probability)
**Risk**: 4-6 hours per service to create initial OpenAPI spec.

**Mitigation**:
- ✅ Start with highest-ROI service (Data Storage: 2 consumers)
- ✅ Create reusable templates (RFC 7807 errors, pagination)
- ✅ Use existing code as reference for schema

**Severity**: LOW (one-time cost, high long-term benefit)

---

### Risk 3: Tooling Complexity (Low Impact, Low Probability)
**Risk**: Learning curve for `oapi-codegen` and `swaggo/swag`.

**Mitigation**:
- ✅ Document in this ADR with examples
- ✅ Provide working examples (Data Storage spec)
- ✅ Use same tools across all services (consistency)

**Severity**: LOW (well-documented tools)

---

## Implementation Plan

### Phase 1: Data Storage Service (IMMEDIATE - 4-6 hours)
**Priority**: **HIGH** (Context API + Effectiveness Monitor depend on it)

**Tasks**:
1. Create `docs/services/stateless/data-storage/openapi/v1.yaml`
2. Define existing endpoints:
   - `GET /api/v1/incidents`
   - `GET /api/v1/incidents/:id`
   - RFC 7807 error schemas
3. Validate spec: `openapi-generator validate -i v1.yaml`
4. Generate Go client for Context API: `oapi-codegen`

**Deliverable**: Context API uses auto-generated client instead of manual implementation

---

### Phase 2: Context API Service (MEDIUM - 4-6 hours)
**Priority**: **MEDIUM** (Gateway may consume context orchestration API)

**Tasks**:
1. Create `docs/services/stateless/context-api/openapi/v1.yaml`
2. Define context orchestration endpoints
3. Generate client for Gateway (if needed)

---

### Phase 3: Notification Service (LOW - 4-6 hours)
**Priority**: **LOW** (stable API, fewer consumers)

---

### Phase 4: Dynamic Toolset Service (LOW - 4-6 hours)
**Priority**: **LOW** (Gateway is only consumer)

---

## Alternatives Considered

### Alternative 1: Manual Client Implementation (Current State)
**Pros**:
- No initial setup time
- Full control over client behavior

**Cons**:
- ❌ Error-prone (field typos, type mismatches)
- ❌ Schema drift (clients diverge from API)
- ❌ High maintenance burden (update each client manually)
- ❌ No documentation (must read source code)

**Decision**: ❌ **REJECTED** (current pain points justify OpenAPI adoption)

---

### Alternative 2: gRPC + Protocol Buffers
**Pros**:
- ✅ Type safety built-in
- ✅ Auto-generated clients
- ✅ Better performance (binary protocol)

**Cons**:
- ❌ Not REST (incompatible with existing HTTP infrastructure)
- ❌ Requires service refactoring (breaking change)
- ❌ Less human-readable (binary format)
- ❌ Overkill for current use case (no streaming needed)

**Decision**: ❌ **REJECTED** (REST is sufficient, too disruptive)

---

### Alternative 3: GraphQL
**Pros**:
- ✅ Flexible queries (clients request only needed fields)
- ✅ Single endpoint for all queries

**Cons**:
- ❌ Overkill for simple CRUD APIs
- ❌ More complex implementation
- ❌ Not needed for service-to-service communication

**Decision**: ⏸️ **DEFERRED** (consider for user-facing APIs in V2)

---

## Success Metrics

### Phase 1 (Data Storage):
- ✅ OpenAPI spec created and validated
- ✅ Context API client generated from spec
- ✅ 0 manual client code in Context API (100% generated)
- ✅ CI/CD validates spec-code sync

### Long-Term (All Services):
- ✅ 100% of REST APIs have OpenAPI specs
- ✅ 0 manual client implementations (all generated)
- ✅ <5% spec-code drift incidents
- ✅ Developer satisfaction: "API integration is easy"

---

## References

- [OpenAPI Specification 3.0.3](https://spec.openapis.org/oas/v3.0.3)
- [swaggo/swag](https://github.com/swaggo/swag) - Go annotations → OpenAPI
- [oapi-codegen](https://github.com/deepmap/oapi-codegen) - OpenAPI → Go client
- [RFC 7807: Problem Details for HTTP APIs](https://datatracker.ietf.org/doc/html/rfc7807)
- [Swagger UI](https://swagger.io/tools/swagger-ui/) - Interactive API documentation

---

## Confidence Assessment

**Confidence**: **92%** ✅ **STRONGLY RECOMMEND**

**Justification**:
- ✅ **Industry Standard**: OpenAPI used by AWS, Google, Azure, Stripe, GitHub
- ✅ **High ROI**: 4-6 hours setup saves 4-6 hours per consuming service
- ✅ **Risk Mitigation**: Code-first generation prevents spec-code drift
- ✅ **Proven Tooling**: Mature ecosystem with `swaggo/swag` and `oapi-codegen`

**Remaining 8% Uncertainty**: Initial setup time and spec-code sync maintenance.

---

**Status**: ✅ **APPROVED**
**Next Step**: Create Data Storage OpenAPI spec (Phase 1)


