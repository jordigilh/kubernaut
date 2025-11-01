# Data Storage Service - OpenAPI Specifications

This directory contains OpenAPI 3.0+ specifications for the Data Storage Service REST API.

---

## Current Version

**v1** (`v1.yaml`): Phase 1 Read API
- Status: ✅ Production-ready
- Endpoints:
  - `GET /api/v1/incidents` - List incidents with filters
  - `GET /api/v1/incidents/{id}` - Get incident by ID
  - `GET /health`, `/health/ready`, `/health/live` - Health checks

---

## Future Versions

**v2** (`v2.yaml`): Phase 2 Write API (Planned)
- Status: 🚧 Not yet implemented
- Additional endpoints:
  - `POST /api/v2/incidents` - Create new incident
  - `PUT /api/v2/incidents/{id}` - Update incident
  - `DELETE /api/v2/incidents/{id}` - Delete incident
  - Vector database integration

---

## Versioning Strategy

### URL Versioning
- API versions are specified in the URL path: `/api/v{major}/resource`
- **v1**: `/api/v1/incidents` (current)
- **v2**: `/api/v2/incidents` (future, breaking changes)

### File Versioning
- Each major API version has a separate OpenAPI file
- **Pattern**: `v{major}.yaml`
- **Reason**: Major versions have different schemas, endpoints, and behavior

### Semantic Versioning
- **Major** (breaking changes): New OpenAPI file (e.g., `v1.yaml` → `v2.yaml`)
- **Minor** (additive changes): Update existing OpenAPI file (e.g., new endpoint in v1)
- **Patch** (bug fixes): Update existing OpenAPI file (e.g., clarify description)

---

## Generating Go Client

### Prerequisites
```bash
go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest
```

### Generate Client
```bash
# From repository root
oapi-codegen \
  --package datastorage \
  --generate types,client \
  docs/services/stateless/data-storage/openapi/v1.yaml \
  > pkg/datastorage/client/generated.go
```

### Generated Code Includes
- ✅ `Incident` struct
- ✅ `IncidentListResponse` struct
- ✅ `Pagination` struct
- ✅ `RFC7807Error` struct
- ✅ `Client` interface with `ListIncidents()` and `GetIncidentByID()` methods
- ✅ HTTP client implementation with request/response handling

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

