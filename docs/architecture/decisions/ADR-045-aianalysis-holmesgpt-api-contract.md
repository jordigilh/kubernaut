# ADR-045: AIAnalysis ↔ HolmesGPT-API Service Contract

**ADR ID**: ADR-045
**Status**: ✅ APPROVED
**Date**: December 2, 2025
**Decision Makers**: HolmesGPT-API Team, AIAnalysis Team

---

## Context

AIAnalysis is the **sole consumer** of HolmesGPT-API. A clear, authoritative API contract is needed to ensure:
- Correct request/response schemas between services
- Clear ownership of responsibilities
- No ambiguity in integration
- Consistent behavior across service boundaries

This ADR establishes the cross-service contract as an architectural decision affecting both services.

---

## Decision

We will establish a formal API contract between AIAnalysis and HolmesGPT-API with the following specifications.

---

## API Specification

### OpenAPI Specification (Auto-Generated)

**Source**: FastAPI auto-generates OpenAPI 3.0 from Pydantic models.

**Live Endpoints** (when `dev_mode: true`):
- `/openapi.json` - Raw OpenAPI 3.0 spec
- `/docs` - Swagger UI
- `/redoc` - ReDoc UI

**Export Location**: `holmesgpt-api/api/openapi.json`

**Export Command**:
```bash
# Start the server and export the spec
cd holmesgpt-api
python3 -c "
from src.main import app
import json
with open('api/openapi.json', 'w') as f:
    json.dump(app.openapi(), f, indent=2)
print('OpenAPI spec exported to api/openapi.json')
"
```

**AIAnalysis Go Client Generation**:
```bash
# Generate Go client from OpenAPI spec using oapi-codegen
oapi-codegen -package holmesgpt -generate types,client \
    holmesgpt-api/api/openapi.json > pkg/clients/holmesgpt/client.go
```

**Source Models** (Pydantic):
- `src/models/incident_models.py` - `IncidentRequest`, `IncidentResponse`, `DetectedLabels`
- `src/models/recovery_models.py` - `RecoveryRequest`, `RecoveryResponse`
- `src/models/postexec_models.py` - Post-execution analysis models

**Design Decisions**:
- **DD-RECOVERY-002**: Direct AIAnalysis recovery flow
- **DD-RECOVERY-003**: Recovery prompt design with DetectedLabels

---

### Endpoint

**URL**: `POST /api/v1/investigate`

**Base URL**: `http://holmesgpt-api:8080` (in-cluster)

**Content-Type**: `application/json`

**Authentication**: Service-to-service (K8s network policy, optional mTLS)

---

### Request Schema

```yaml
# OpenAPI 3.0 Schema
InvestigateRequest:
  type: object
  required:
    - signalContext
  properties:
    signalContext:
      $ref: '#/components/schemas/SignalContext'
    recoveryContext:
      $ref: '#/components/schemas/RecoveryContext'

SignalContext:
  type: object
  required:
    - signalId
    - signalName
    - targetResource
  properties:
    signalId:
      type: string
      description: Unique signal identifier
    signalName:
      type: string
      description: Semantic signal name (e.g., OOMKilled, HighCPULoad)
    severity:
      type: string
      enum: [critical, high, medium, low]
      default: medium
    timestamp:
      type: string
      format: date-time
    businessPriority:
      type: string
    targetResource:
      $ref: '#/components/schemas/TargetResource'
    enrichmentResults:
      $ref: '#/components/schemas/EnrichmentResults'

TargetResource:
  type: object
  required:
    - apiVersion
    - kind
    - name
  properties:
    apiVersion:
      type: string
    kind:
      type: string
    namespace:
      type: string
    name:
      type: string

EnrichmentResults:
  type: object
  properties:
    kubernetesContext:
      type: object
    detectedLabels:
      $ref: '#/components/schemas/DetectedLabels'
    ownerChain:
      type: array
      items:
        $ref: '#/components/schemas/OwnerChainEntry'
    customLabels:
      type: object
      additionalProperties:
        type: array
        items:
          type: string
    enrichmentQuality:
      type: number
      minimum: 0.0
      maximum: 1.0

DetectedLabels:
  type: object
  description: "AUTHORITATIVE SOURCE: pkg/shared/types/enrichment.go"
  properties:
    gitOpsManaged:
      type: boolean
    gitOpsTool:
      type: string
      enum: [argocd, flux, ""]
    pdbProtected:
      type: boolean
    hpaEnabled:
      type: boolean
    stateful:
      type: boolean
    helmManaged:
      type: boolean
    networkIsolated:
      type: boolean
    podSecurityLevel:
      type: string
      enum: [privileged, baseline, restricted, ""]
    serviceMesh:
      type: string
      enum: [istio, linkerd, ""]

OwnerChainEntry:
  type: object
  required:
    - kind
    - name
  properties:
    namespace:
      type: string
    kind:
      type: string
    name:
      type: string

RecoveryContext:
  type: object
  properties:
    isRecovery:
      type: boolean
    previousExecutionId:
      type: string
    naturalLanguageSummary:
      type: string
```

---

### Response Schema

```yaml
InvestigateResponse:
  type: object
  required:
    - investigationId
    - status
    - rootCauseAnalysis
    - targetInOwnerChain
  properties:
    investigationId:
      type: string
    status:
      type: string
      enum: [completed, failed, partial]
    rootCauseAnalysis:
      $ref: '#/components/schemas/RootCauseAnalysis'
    selectedWorkflow:
      $ref: '#/components/schemas/SelectedWorkflow'
    alternativeWorkflows:
      type: array
      items:
        $ref: '#/components/schemas/AlternativeWorkflow'
      description: |
        INFORMATIONAL ONLY - NOT for automatic execution.
        Per APPROVAL_REJECTION_BEHAVIOR_DETAILED.md:
        - ✅ Purpose: Help operator make an informed approval decision
        - ✅ Content: Other workflows considered with confidence and rationale
        - ❌ NOT: A fallback queue for automatic execution
        Only selectedWorkflow is executed. Alternatives provide audit trail
        and context for operator approval decisions.
    targetInOwnerChain:
      type: boolean
      default: true
      description: |
        Whether RCA-identified target resource was found in OwnerChain.
        If false, DetectedLabels may be from different scope than affected resource.
        AIAnalysis can use this in Rego policies for approval decisions.
    warnings:
      type: array
      items:
        type: string
      description: |
        Non-fatal warnings for transparency:
        - "Target resource not found in OwnerChain - DetectedLabels may not apply"
        - "No workflows matched the search criteria"
        - "Low confidence selection (X%) - manual review recommended"
    analysisMetadata:
      $ref: '#/components/schemas/AnalysisMetadata'

RootCauseAnalysis:
  type: object
  required:
    - summary
    - confidence
  properties:
    summary:
      type: string
    severity:
      type: string
      enum: [critical, high, medium, low]
    signalName:
      type: string
    confidence:
      type: number
      minimum: 0.0
      maximum: 1.0
    affectedResource:
      $ref: '#/components/schemas/TargetResource'
    evidenceChain:
      type: array
      items:
        type: string

SelectedWorkflow:
  type: object
  required:
    - workflowId
    - containerImage
    - confidence
    - rationale
  properties:
    workflowId:
      type: string
      format: uuid
    containerImage:
      type: string
      description: OCI image reference
    containerDigest:
      type: string
      pattern: "^sha256:[a-f0-9]{64}$"
    parameters:
      type: object
      additionalProperties:
        type: string
      description: "Keys in UPPER_SNAKE_CASE per DD-WORKFLOW-003"
    confidence:
      type: number
      minimum: 0.0
      maximum: 1.0
    rationale:
      type: string

AlternativeWorkflow:
  type: object
  description: |
    Alternative workflow that was considered but not selected.
    For AUDIT and OPERATOR CONTEXT only - NOT for automatic execution.

    Purpose (per APPROVAL_REJECTION_BEHAVIOR_DETAILED.md):
    - Help operator understand what options were evaluated
    - Provide audit trail of AI decision-making
    - Enable informed approval decisions

    Only selectedWorkflow is executed by RemediationOrchestrator.
  properties:
    workflowId:
      type: string
      description: Workflow identifier that was considered
    containerImage:
      type: string
      description: OCI image reference for the alternative workflow
    confidence:
      type: number
      minimum: 0.0
      maximum: 1.0
      description: Confidence score for this alternative (typically lower than selectedWorkflow)
    rationale:
      type: string
      description: Why this workflow was considered but not selected (explains trade-offs)

AnalysisMetadata:
  type: object
  properties:
    processingTimeMs:
      type: integer
    llmTokensUsed:
      type: integer
    workflowCandidatesEvaluated:
      type: integer
```

---

### Error Response Schema

**Format**: RFC 7807 Problem Details (per DD-004)

**Content-Type**: `application/problem+json`

```yaml
ProblemDetails:
  type: object
  required:
    - type
    - title
    - status
  properties:
    type:
      type: string
      format: uri
      example: "https://kubernaut.io/errors/validation-error"
    title:
      type: string
    status:
      type: integer
    detail:
      type: string
    instance:
      type: string
```

**Error Types**:

| HTTP Status | Error Type URI | Retryable |
|-------------|----------------|-----------|
| 400 | `https://kubernaut.io/errors/validation-error` | ❌ No |
| 404 | `https://kubernaut.io/errors/signal-not-found` | ❌ No |
| 422 | `https://kubernaut.io/errors/unprocessable-entity` | ❌ No |
| 500 | `https://kubernaut.io/errors/internal-error` | ✅ Yes |
| 502 | `https://kubernaut.io/errors/llm-unavailable` | ✅ Yes |
| 503 | `https://kubernaut.io/errors/service-unavailable` | ✅ Yes |
| 504 | `https://kubernaut.io/errors/gateway-timeout` | ✅ Yes |

---

## Responsibility Matrix

| Aspect | HolmesGPT-API | AIAnalysis |
|--------|---------------|------------|
| RCA Analysis | ✅ Performs | Consumes result |
| Workflow Selection | ✅ Selects best match | Consumes result |
| Confidence Scoring | ✅ Calculates (per DD-HAPI-003) | Uses for Rego policy |
| Approval Decision | ❌ **Not responsible** | ✅ Determines via Rego |
| Parameter Formatting | ✅ `UPPER_SNAKE_CASE` | Passthrough to RO |
| Retry Logic | ❌ **Not responsible** | RO decides (per BR-HAPI-193) |
| Audit Trail Storage | ✅ Internal only | Captures response in CRD |

---

## Health Endpoint

**URL**: `GET /health`

**Response**:
```json
{
  "status": "healthy",
  "llm_connected": true,
  "data_storage_connected": true,
  "version": "v3.2.0"
}
```

---

## Timeout and Retry Guidance

| Aspect | Value |
|--------|-------|
| Recommended Timeout | 30 seconds |
| Max Retries (AIAnalysis) | 3 |
| Backoff Strategy | Exponential (1s, 2s, 4s) |

---

## Key Design Decisions

### What is NOT in Response

| Field | Reason |
|-------|--------|
| `approvalRequired` | AIAnalysis determines via Rego policies |
| `auditTrail` | HAPI maintains internally, not exposed |
| `version` | Human metadata, use `containerImage` + `containerDigest` |
| `historicalSuccessRate` | Not used in V1.0 (see DD-HAPI-003) |

### Informational vs Execution Fields

| Field | Purpose | Execution |
|-------|---------|-----------|
| `selectedWorkflow` | Primary recommendation | ✅ **Executed by RO** |
| `alternativeWorkflows` | Audit trail, operator context | ❌ **NOT executed** |
| `warnings` | Operator transparency | ❌ Informational |
| `targetInOwnerChain` | Rego policy input, audit | ❌ Informational |

**Key Principle** (per `APPROVAL_REJECTION_BEHAVIOR_DETAILED.md`):
> Alternatives are for **CONTEXT**, not **EXECUTION**. They help operators make informed approval decisions and provide audit trail of AI decision-making.

---

## Consequences

### Positive
- ✅ Clear contract between services
- ✅ Single source of truth for integration
- ✅ RFC 7807 error handling for consistency
- ✅ OpenAPI spec enables code generation

### Negative
- ⚠️ Contract changes require coordination between teams
- ⚠️ OpenAPI spec needs to be created and maintained

### Mitigation
- Version the API (`/api/v1/`)
- Use semantic versioning for breaking changes
- Generate client code from OpenAPI spec

---

## Action Items

| Item | Owner | Status |
|------|-------|--------|
| Create `holmesgpt-api/api/` directory | HAPI Team | ✅ Done |
| Export OpenAPI spec to `api/openapi.json` | HAPI Team | ✅ Done (19 schemas) |
| Add `targetInOwnerChain` field to response | HAPI Team | ✅ Done (v1.1) |
| Add `warnings[]` field to response | HAPI Team | ✅ Done (v1.1) |
| Implement OwnerChain validation logic | HAPI Team | ✅ Done (11 tests) |
| Add `alternativeWorkflows[]` for audit/context | HAPI Team | ✅ Done (v1.2) |
| Add export script to Makefile | HAPI Team | ✅ Done (`make export-openapi-holmesgpt-api`) |
| Generate Go client for AIAnalysis | AIAnalysis Team | ⏳ Ready (OpenAPI available, 19 schemas) |
| Add OpenAPI spec to CI validation | HAPI Team | ✅ Done (`.github/workflows/holmesgpt-api-ci.yml`) |

---

## Related Documents

- **ADR-031**: OpenAPI Specification Standard
- **DD-004**: RFC 7807 Error Responses
- **DD-HAPI-003**: V1.0 Confidence Scoring Methodology
- **DD-RECOVERY-002**: Direct AIAnalysis recovery flow
- **DD-RECOVERY-003**: Recovery prompt design with DetectedLabels
- **BR-HAPI-192**: Recovery Context Consumption
- **BR-HAPI-193**: Execution Outcome Reporting
- `pkg/shared/types/enrichment.go` - AUTHORITATIVE DetectedLabels schema (Go)
- `holmesgpt-api/src/models/incident_models.py` - DetectedLabels schema (Python/Pydantic)

---

## Version History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.2 | 2025-12-05 | HAPI Team | Implemented `alternativeWorkflows[]` for audit/context (NOT for execution) |
| 1.1 | 2025-12-02 | HAPI Team | Added `targetInOwnerChain` and `warnings[]` fields per AIAnalysis request |
| 1.0 | 2025-12-02 | HAPI Team | Initial creation (converted from DD-HAPI-004) |

