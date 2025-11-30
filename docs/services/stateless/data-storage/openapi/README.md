# Data Storage Service - OpenAPI Specifications

This directory contains OpenAPI 3.0+ specifications for the Data Storage Service REST API.

---

## Current Version

**v3** (`v3.yaml`): DD-STORAGE-011 Workflow Catalog CRUD API
- **Note**: "v3" refers to the OpenAPI spec version (3.0.0), NOT the API URL path
- **API Path**: All endpoints use `/api/v1/` (we only support v1 API)
- **Terminology**: Per DD-NAMING-001, using "Remediation Workflow" (not "Remediation Playbook")
- Status: ✅ Production-ready
- **NEW in v3** (DD-STORAGE-011 Workflow CRUD):
  - `POST /api/v1/workflows` - Create workflow with ADR-043 schema validation
  - `GET /api/v1/workflows/{workflow_id}/{version}` - Get workflow by ID and version
  - `GET /api/v1/workflows/{workflow_id}/versions` - List all versions of a workflow
  - `PATCH /api/v1/workflows/{workflow_id}/{version}/disable` - Disable workflow (soft delete)
  - `POST /api/v1/workflows/search` - Semantic search with hybrid scoring
- **From v2** (ADR-033 Success Rate):
  - `GET /api/v1/success-rate/incident-type` - Incident-type success rate
  - `GET /api/v1/success-rate/workflow` - Workflow success rate
- **From v1** (Incidents):
  - `GET /api/v1/incidents` - List incidents with filters
  - `GET /api/v1/incidents/{id}` - Get incident by ID
  - `GET /health`, `/health/ready`, `/health/live` - Health checks
- Features:
  - Workflow catalog CRUD with ADR-043 schema validation
  - Synchronous embedding generation (DD-STORAGE-011)
  - Workflow immutability (DD-WORKFLOW-012)
  - Semantic search with hybrid weighted scoring (BR-STORAGE-013)
  - Multi-dimensional success tracking (ADR-033)

---

## Previous Versions

**v2** (`v2.yaml`): ADR-033 Multi-Dimensional Success Tracking
- Status: ✅ Stable (superseded by v3)
- Added success rate analytics endpoints
- Does not include workflow CRUD endpoints

**v1** (`v1.yaml`): Phase 1 Read API (Legacy)
- Status: ✅ Stable (no longer actively developed)
- Endpoints:
  - `GET /api/v1/incidents` - List incidents with filters
  - `GET /api/v1/incidents/{id}` - Get incident by ID
  - `GET /health`, `/health/ready`, `/health/live` - Health checks
- Note: v1 does not include ADR-033 success rate analytics or workflow CRUD

---

## Versioning Strategy

### ⚠️ Important: API vs Spec Versioning
- **API URL Path**: `/api/v1/` (we only support v1 API)
- **OpenAPI Spec Version**: `v3.yaml` refers to spec version 3.0.0 (with workflow CRUD features)
- **Clarification**: The file name `v3.yaml` does NOT mean we have a `/api/v3/` endpoint

### URL Versioning
- API versions are specified in the URL path: `/api/v{major}/resource`
- **v1**: `/api/v1/workflows`, `/api/v1/incidents` (current and only supported version)
- **v2**: `/api/v2/` (future, breaking changes - not yet implemented)

### File Versioning
- OpenAPI spec files track feature evolution, not API URL versions
- **Pattern**: `v{major}.yaml` (spec version, not API version)
- **Current**: `v3.yaml` = spec version 3.0.0 with workflow CRUD, still serving `/api/v1/` endpoints
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

### Generate Client (v3 - Current)
```bash
# From repository root
oapi-codegen \
  --package datastorage \
  --generate types,client \
  docs/services/stateless/data-storage/openapi/v3.yaml \
  > pkg/datastorage/client/generated.go
```

### Generated Code Includes (v3)
- ✅ `RemediationWorkflow` struct (NEW in v3)
- ✅ `CreateWorkflowRequest` struct (NEW in v3)
- ✅ `DisableWorkflowRequest` struct (NEW in v3)
- ✅ `WorkflowVersionSummary` struct (NEW in v3)
- ✅ `WorkflowSearchRequest` struct (NEW in v3)
- ✅ `WorkflowSearchResponse` struct (NEW in v3)
- ✅ `WorkflowSearchResult` struct (NEW in v3)
- ✅ `Incident` struct
- ✅ `IncidentListResponse` struct
- ✅ `IncidentTypeSuccessRateResponse` struct
- ✅ `RFC7807Error` struct
- ✅ `Client` interface with all methods:
  - `CreateWorkflow()`, `GetWorkflow()`, `ListWorkflowVersions()`, `DisableWorkflow()`, `SearchWorkflows()` (v3)
  - `ListIncidents()`, `GetIncidentByID()` (v1)
  - `GetSuccessRateByIncidentType()` (v2)
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


