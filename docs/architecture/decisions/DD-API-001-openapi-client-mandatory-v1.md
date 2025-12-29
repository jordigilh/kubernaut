# DD-API-001: OpenAPI Generated Client MANDATORY for REST API Communication

## Status
**✅ APPROVED** (2025-12-18)
**Last Reviewed**: 2025-12-18
**Confidence**: 98%
**Effective**: V1.0 Release (MANDATORY)

---

## Context & Problem

**CRITICAL FINDING** (Dec 18, 2025): During V1.0 release preparation, Notification Team discovered a critical bug in Data Storage service integration:
- **NT Team**: Used generated OpenAPI client → **Bug found** (missing 6 query parameters)
- **5 Other Teams**: Used direct HTTP curl → **Bug hidden** (bypassed spec validation)

### The Problem

**Direct HTTP "Escape Hatch" Pattern**:
```go
// ❌ FORBIDDEN: Direct HTTP bypasses type safety
queryURL := fmt.Sprintf("%s/api/v1/audit/events?event_category=%s&correlation_id=%s",
    dataStorageURL, category, correlationID)
resp, err := http.Get(queryURL)
```

**OpenAPI Client Pattern** (Notification Team):
```go
// ✅ MANDATORY: Generated client enforces type safety
params := &dsgen.QueryAuditEventsParams{
    EventCategory:  &category,      // ❌ COMPILE ERROR if field missing!
    CorrelationId:  &correlationID,
}
resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
```

### Why This Matters

**The Bug That Was Hidden**:
- REST API handler implemented 9 parameters (event_category, event_outcome, severity, etc.)
- OpenAPI spec documented only 3 parameters (correlation_id, event_type, limit)
- Generated client had only 3 fields → **CONTRACT VIOLATION**
- Direct HTTP bypassed the spec → **5 teams had broken tests that "passed"**

**Business Impact**:
- ❌ **Schema Drift**: Clients diverged from API implementation (spec incomplete)
- ❌ **False Positives**: Tests passed but would break in production
- ❌ **Integration Failures**: NT Team blocked by missing parameters
- ❌ **Type Safety Lost**: Field typos, type mismatches undetected
- ❌ **Maintenance Burden**: Manual clients require updates when APIs change

**NT Team's Excellence**:
- ✅ Adopted generated client for type safety (best practices)
- ✅ Found critical bug that would have bitten ALL teams eventually
- ✅ Prevented V1.0 release with broken API contract

---

## Key Requirements

**For V1.0 Release**:
1. ✅ **Type Safety**: Compile-time validation of API contracts
2. ✅ **Contract Enforcement**: Spec-code sync validation
3. ✅ **Maintainability**: Auto-generated clients reduce manual updates
4. ✅ **Schema Consistency**: No divergence between spec and implementation
5. ✅ **Integration Reliability**: No hidden bugs in production

---

## Alternatives Considered

### Alternative 1: Continue Direct HTTP Usage (Current State - 5 Teams)

**Approach**: Allow teams to manually construct HTTP requests

**Pros**:
- ✅ No initial setup time
- ✅ Full control over request construction
- ✅ No dependency on code generation tools

**Cons**:
- ❌ **CRITICAL**: Bypasses OpenAPI spec validation (hides contract bugs)
- ❌ **Type Safety Lost**: Field typos, type mismatches undetected until runtime
- ❌ **Schema Drift**: Clients diverge from API implementation
- ❌ **False Positives**: Tests pass but would break in production
- ❌ **Maintenance Burden**: Manual updates when APIs change
- ❌ **Documentation Gaps**: Must read source code to understand endpoints

**Evidence**: This approach FAILED to detect the missing 6 parameters bug. Only found when NT Team adopted generated client.

**Confidence**: 2% ❌ **STRONGLY REJECT** (proven to hide critical bugs)

---

### Alternative 2: OpenAPI Generated Client (Notification Team Approach)

**Approach**: ALL teams MUST use generated OpenAPI clients for REST API communication

**Pros**:
- ✅ **CRITICAL**: Compile-time validation catches spec-code drift
- ✅ **Type Safety**: Field typos, type mismatches caught at build time
- ✅ **Contract Enforcement**: Clients can't diverge from OpenAPI spec
- ✅ **False Positive Prevention**: Tests fail if spec incomplete (correct behavior)
- ✅ **Maintainability**: Auto-generated clients reduce manual updates
- ✅ **Documentation**: OpenAPI spec serves as living documentation
- ✅ **Industry Standard**: Used by AWS, Google, Azure, Stripe, GitHub

**Cons**:
- ⚠️ **Initial Setup**: ~1-2 hours per consuming service to migrate
- ⚠️ **Code Generation**: Requires `oapi-codegen` in build pipeline
- ⚠️ **Learning Curve**: Teams must learn generated client patterns

**Evidence**: NT Team's approach FOUND the bug that 5 other teams missed.

**Confidence**: 98% ✅ **STRONGLY APPROVE**

---

### Alternative 3: Hybrid Approach (Generated Client Optional)

**Approach**: Recommend generated clients but allow direct HTTP

**Pros**:
- ✅ Flexibility for teams to choose their approach
- ✅ No forced migration for existing code

**Cons**:
- ❌ **CRITICAL**: Doesn't prevent schema drift (allows "escape hatch")
- ❌ **Inconsistent Quality**: Some teams get type safety, others don't
- ❌ **False Positives**: Direct HTTP tests still hide contract bugs
- ❌ **Maintenance Burden**: Must maintain both patterns

**Confidence**: 10% ❌ **REJECT** (doesn't solve the root problem)

---

## Decision

**APPROVED: Alternative 2** - OpenAPI Generated Client MANDATORY

### Rationale

1. **Evidence-Based**: NT Team's generated client approach FOUND the bug that 5 other teams' direct HTTP approach MISSED
2. **Type Safety**: Compile-time validation prevents runtime errors and integration failures
3. **Contract Enforcement**: Generated clients enforce spec-code sync (spec completeness validated)
4. **Industry Standard**: OpenAPI clients are the de facto standard (AWS, Google, Azure, Stripe)
5. **V1.0 Quality**: This is a blocker for V1.0 release (can't ship with hidden contract bugs)

**Key Insight**: Direct HTTP is an "escape hatch" that bypasses contract validation. It creates false positives in tests and hides critical bugs that would break production integrations.

---

## Implementation

### Primary Implementation Pattern

**Consumer Services** (MUST use generated clients):
- ✅ Notification Service (consuming Data Storage audit API)
- ✅ SignalProcessing Service (consuming Data Storage audit API)
- ✅ Gateway Service (consuming Data Storage audit API)
- ✅ AIAnalysis Service (consuming Data Storage audit API)
- ✅ RemediationOrchestrator Service (consuming Data Storage audit API)
- ✅ WorkflowExecution Service (consuming Data Storage audit API)

**Provider Services** (MUST maintain OpenAPI specs):
- ✅ Data Storage Service (`api/openapi/data-storage-v1.yaml`)
- ✅ Context API Service (`api/openapi/context-api-v1.yaml`)
- ✅ Notification Service (`api/openapi/notification-v1.yaml`)
- ✅ Dynamic Toolset Service (`api/openapi/dynamic-toolset-v1.yaml`)

### Data Flow

```
┌─────────────────────────────────────────────────────┐
│ REST API Implementation (Provider)                  │
│ - pkg/datastorage/server/audit_events_handler.go   │
│ - Supports 9 parameters                             │
│ - ✅ AUTHORITATIVE SOURCE                          │
└─────────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────────┐
│ OpenAPI Specification (Contract)                    │
│ - api/openapi/data-storage-v1.yaml                  │
│ - MUST document ALL parameters                      │
│ - ✅ ENFORCED by CI/CD                             │
└─────────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────────┐
│ Generated Client (Consumer)                         │
│ - pkg/datastorage/client/generated.go              │
│ - Auto-generated via oapi-codegen                   │
│ - ✅ TYPE-SAFE, VALIDATED                          │
└─────────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────────┐
│ Consumer Service (Integration Tests)                │
│ - test/integration/notification/audit_*_test.go    │
│ - Uses generated client (MANDATORY)                 │
│ - ✅ COMPILE-TIME VALIDATION                       │
└─────────────────────────────────────────────────────┘
```

### Migration Steps (Per Team)

**Step 1**: Install OpenAPI client generation tool
```bash
# Add oapi-codegen to go.mod
go get github.com/deepmap/oapi-codegen/v2@latest
```

**Step 2**: Generate client from OpenAPI spec
```bash
# Generate Go client
oapi-codegen \
  --package dsclient \
  --generate types,client \
  api/openapi/data-storage-v1.yaml \
  > pkg/datastorage/client/generated.go
```

**Step 3**: Replace direct HTTP with generated client
```go
// ❌ BEFORE: Direct HTTP (FORBIDDEN)
queryURL := fmt.Sprintf("%s/api/v1/audit/events?event_category=%s", url, category)
resp, err := http.Get(queryURL)

// ✅ AFTER: Generated client (MANDATORY)
client, err := dsclient.NewClientWithResponses(url)
params := &dsclient.QueryAuditEventsParams{
    EventCategory: &category,
}
resp, err := client.QueryAuditEventsWithResponse(ctx, params)
```

**Step 4**: Update integration tests
```go
// ❌ BEFORE: Manual JSON parsing (FORBIDDEN)
var events []map[string]interface{}
json.NewDecoder(resp.Body).Decode(&events)

// ✅ AFTER: Type-safe response (MANDATORY)
if resp.JSON200 == nil {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode())
}
events := resp.JSON200.Data
```

### Graceful Degradation

**CI/CD Enforcement**:
- ✅ OpenAPI spec validation in PR checks
- ✅ Client generation as part of build pipeline
- ✅ Integration tests MUST use generated clients
- ❌ Direct HTTP usage flagged by linter

**Exception Process**:
- NO exceptions for V1.0
- Post-V1.0: Exception requires DD-API-XXX approval

---

## Consequences

### Positive

- ✅ **Contract Enforcement**: OpenAPI spec completeness validated at compile time
- ✅ **Type Safety**: Field typos, type mismatches caught at build time
- ✅ **Schema Consistency**: No divergence between spec and implementation
- ✅ **Integration Reliability**: No hidden bugs in production
- ✅ **Maintainability**: Auto-generated clients reduce manual updates
- ✅ **Documentation**: OpenAPI spec serves as living documentation
- ✅ **Industry Alignment**: Follows AWS, Google, Azure, Stripe best practices

### Negative

- ⚠️ **Migration Effort**: ~1-2 hours per consuming service (5 services = 5-10 hours total)
  - **Mitigation**: NT Team's implementation serves as reference example
  - **Mitigation**: Migration can be parallelized across teams
  - **Mitigation**: One-time cost, long-term maintainability benefit

- ⚠️ **Build Dependency**: Requires `oapi-codegen` in build pipeline
  - **Mitigation**: Already used for HolmesGPT-API (ADR-045)
  - **Mitigation**: Standard tool, well-maintained, stable

- ⚠️ **Learning Curve**: Teams must learn generated client patterns
  - **Mitigation**: NT Team's tests serve as examples
  - **Mitigation**: OpenAPI clients are industry standard (transferable skill)

### Neutral

- 🔄 **Code Generation**: Clients regenerated when spec changes (automation required)
- 🔄 **Spec Maintenance**: OpenAPI spec must stay in sync with implementation

---

## Validation Results

### Confidence Assessment Progression

- **Initial assessment**: 75% confidence (NT Team found bug, but was it representative?)
- **After analysis**: 92% confidence (5/6 teams using direct HTTP, all missed bug)
- **After implementation review**: 98% confidence (NT Team's approach is proven)

### Key Validation Points

- ✅ **NT Team Success**: Generated client FOUND critical bug (6 missing parameters)
- ✅ **5 Teams Failure**: Direct HTTP MISSED critical bug (false positive tests)
- ✅ **Industry Validation**: OpenAPI clients are standard (AWS, Google, Azure, Stripe)
- ✅ **ADR-031 Alignment**: Builds on existing OpenAPI spec mandate
- ✅ **Tooling Maturity**: `oapi-codegen` is stable, well-maintained

---

## Related Decisions

- **Builds On**: ADR-031 (OpenAPI Specification Standard for REST APIs)
- **Supports**: ADR-045 (AIAnalysis ↔ HolmesGPT-API Service Contract)
- **Enforces**: DD-TEST-001 (Infrastructure Image Cleanup)
- **Mandates**: BR-API-001 to BR-API-050 (API Communication Business Requirements)

---

## Review & Evolution

### When to Revisit

- ✅ If OpenAPI spec-code drift incidents exceed 5% (quarterly review)
- ✅ If client generation tools become unmaintained (evaluate alternatives)
- ✅ If teams consistently struggle with generated client patterns (training needed)

### Success Metrics

- **Target**: 0% spec-code drift incidents in V1.0
- **Target**: 100% of REST API consumers use generated clients by V1.0 release
- **Target**: <2 hours migration time per consuming service
- **Target**: 0 hidden contract bugs in V1.0 release

---

## V1.0 Enforcement Timeline

### Immediate Actions (Dec 18-19, 2025)

**Phase 1: Triage & Notification** (2 hours)
- ✅ Document DD-API-001 decision
- ✅ Notify all teams of mandatory migration
- ✅ Identify teams in violation (5 teams: SP, Gateway, AI, RO, WE)

**Phase 2: Migration Execution** (1-2 hours per team)
- ⏳ SignalProcessing Team: Migrate to generated client
- ⏳ Gateway Team: Migrate to generated client
- ⏳ AIAnalysis Team: Migrate to generated client
- ⏳ RemediationOrchestrator Team: Migrate to generated client
- ⏳ WorkflowExecution Team: Migrate to generated client

**Phase 3: Validation** (1 hour)
- ⏳ All integration tests pass with generated clients
- ⏳ No direct HTTP usage in integration tests (grep validation)
- ⏳ CI/CD enforces generated client usage

### V1.0 Release Blocker

**BLOCKER**: V1.0 CANNOT ship until ALL teams migrate to generated clients.

**Rationale**: Direct HTTP usage creates false positives in tests and hides critical contract bugs. This is a V1.0 quality gate.

---

## References

- [ADR-031: OpenAPI Specification Standard](./ADR-031-openapi-specification-standard.md)
- [ADR-045: AIAnalysis ↔ HolmesGPT-API Service Contract](./ADR-045-aianalysis-holmesgpt-api-contract.md)
- [NT_DS_API_QUERY_ISSUE_DEC_18_2025.md](../../handoff/NT_DS_API_QUERY_ISSUE_DEC_18_2025.md) - Bug report
- [oapi-codegen Documentation](https://github.com/deepmap/oapi-codegen)
- [OpenAPI Specification 3.0.3](https://spec.openapis.org/oas/v3.0.3)

---

## Confidence Assessment

**Confidence**: **98%** ✅ **STRONGLY APPROVE**

**Justification**:
- ✅ **Evidence-Based**: NT Team's approach FOUND the bug that 5 teams MISSED
- ✅ **Industry Standard**: OpenAPI clients used by AWS, Google, Azure, Stripe
- ✅ **Type Safety**: Compile-time validation prevents runtime errors
- ✅ **Contract Enforcement**: Generated clients enforce spec-code sync
- ✅ **Proven Tooling**: `oapi-codegen` is stable, well-maintained
- ✅ **V1.0 Quality Gate**: Prevents hidden contract bugs in production

**Remaining 2% Uncertainty**: Migration effort estimation (may take longer than 1-2 hours per team if unforeseen issues arise).

---

**Status**: ✅ **APPROVED**
**Effective**: V1.0 Release (MANDATORY - NO EXCEPTIONS)
**Next Step**: Notify all teams and coordinate migration

