# Data Storage Service - OpenAPI Specifications

This directory contains OpenAPI 3.0+ specifications for the Data Storage Service REST API.

---

## Current Version

**v2** (`v2.yaml`): ADR-033 Multi-Dimensional Success Tracking
- **Note**: "v2" refers to the OpenAPI spec version (2.0.0), NOT the API URL path
- **API Path**: All endpoints use `/api/v1/` (we only support v1 API)
- **Terminology**: Per DD-NAMING-001, using "Remediation Workflow" (not "Remediation Playbook")
- Status: ✅ Production-ready (Day 15 complete)
- Endpoints:
  - `GET /api/v1/success-rate/incident-type` - **NEW**: Incident-type success rate (BR-STORAGE-031-01)
  - `GET /api/v1/success-rate/workflow` - **NEW**: Workflow success rate (BR-STORAGE-031-02)
  - `GET /api/v1/incidents` - List incidents with filters
  - `GET /api/v1/incidents/{id}` - Get incident by ID
  - `GET /health`, `/health/ready`, `/health/live` - Health checks
- Features:
  - Multi-dimensional success tracking (incident-type, workflow, AI mode)
  - Confidence-based recommendations (high/medium/low/insufficient_data)
  - AI execution mode distribution (catalog/chained/manual)
  - Workflow and incident-type breakdown analytics

---

## Previous Versions

**v1** (`v1.yaml`): Phase 1 Read API (Legacy)
- Status: ✅ Stable (no longer actively developed)
- Endpoints:
  - `GET /api/v1/incidents` - List incidents with filters
  - `GET /api/v1/incidents/{id}` - Get incident by ID
  - `GET /health`, `/health/ready`, `/health/live` - Health checks
- Note: v1 does not include ADR-033 success rate analytics

---

## Future Versions

**v3** (`v3.yaml`): Phase 2 Write API (Planned)
- Status: 🚧 Not yet implemented
- Additional endpoints:
  - `POST /api/v2/incidents` - Create new incident
  - `PUT /api/v2/incidents/{id}` - Update incident
  - `DELETE /api/v2/incidents/{id}` - Delete incident
  - Vector database integration

---

## Versioning Strategy

### ⚠️ Important: API vs Spec Versioning
- **API URL Path**: `/api/v1/` (we only support v1 API)
- **OpenAPI Spec Version**: `v2.yaml` refers to spec version 2.0.0 (with ADR-033 features)
- **Clarification**: The file name `v2.yaml` does NOT mean we have a `/api/v2/` endpoint

### URL Versioning
- API versions are specified in the URL path: `/api/v{major}/resource`
- **v1**: `/api/v1/incidents` (current and only supported version)
- **v2**: `/api/v2/incidents` (future, breaking changes - not yet implemented)

### File Versioning
- OpenAPI spec files track feature evolution, not API URL versions
- **Pattern**: `v{major}.yaml` (spec version, not API version)
- **Current**: `v2.yaml` = spec version 2.0.0 with ADR-033 features, still serving `/api/v1/` endpoints
- **Reason**: Allows tracking spec evolution while maintaining stable API URLs

### Semantic Versioning
- **Major** (breaking changes): New API URL path (e.g., `/api/v1/` → `/api/v2/`)
- **Minor** (additive changes): Update existing OpenAPI file (e.g., new endpoint in v1)
- **Patch** (bug fixes): Update existing OpenAPI file (e.g., clarify description)

---

## Generating Go Client

### Prerequisites
```bash
go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest
```

### Generate Client (v2 - Current)
```bash
# From repository root
oapi-codegen \
  --package datastorage \
  --generate types,client \
  docs/services/stateless/data-storage/openapi/v2.yaml \
  > pkg/datastorage/client/generated.go
```

### Generated Code Includes (v2)
- ✅ `Incident` struct
- ✅ `IncidentListResponse` struct
- ✅ `IncidentTypeSuccessRateResponse` struct (NEW in v2)
- ✅ `WorkflowSuccessRateResponse` struct (NEW in v2)
- ✅ `AIExecutionModeStats` struct (NEW in v2)
- ✅ `WorkflowBreakdownItem` struct (NEW in v2)
- ✅ `IncidentTypeBreakdownItem` struct (NEW in v2)
- ✅ `Pagination` struct
- ✅ `RFC7807Error` struct
- ✅ `Client` interface with all methods:
  - `ListIncidents()`, `GetIncidentByID()` (v1)
  - `GetSuccessRateByIncidentType()`, `GetSuccessRateByWorkflow()` (v2)
- ✅ HTTP client implementation with request/response handling

### Generate Client (v1 - Legacy)
```bash
# For legacy v1 client generation
oapi-codegen \
  --package datastorage \
  --generate types,client \
  docs/services/stateless/data-storage/openapi/v1.yaml \
  > pkg/datastorage/client/generated_v1.go
```

---

## Validating Specification

### Install Validator
```bash
npm install -g @openapitools/openapi-generator-cli
```

### Validate Spec
```bash
openapi-generator-cli validate -i docs/services/stateless/data-storage/openapi/v1.yaml
```

---

## Swagger UI (Local Testing)

### Install Swagger UI
```bash
docker run -p 8081:8080 \
  -e SWAGGER_JSON=/openapi/v1.yaml \
  -v $(pwd)/docs/services/stateless/data-storage/openapi:/openapi \
  swaggerapi/swagger-ui
```

### Access
Open browser: http://localhost:8081

---

## CI/CD Integration

The OpenAPI spec is validated in CI/CD pipelines:
- ✅ Spec syntax validation
- ✅ Schema validation
- ✅ Example validation
- ✅ Client generation test

See: `.github/workflows/openapi-validation.yaml`

---

## References

- [ADR-031: OpenAPI Specification Standard](../../../../architecture/decisions/ADR-031-openapi-specification-standard.md)
- [OpenAPI Specification 3.0.3](https://spec.openapis.org/oas/v3.0.3)
- [oapi-codegen Documentation](https://github.com/deepmap/oapi-codegen)
- [RFC 7807: Problem Details for HTTP APIs](https://datatracker.ietf.org/doc/html/rfc7807)


