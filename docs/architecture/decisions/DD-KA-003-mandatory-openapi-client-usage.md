# DD-KA-003: Mandatory OpenAPI Client Usage

## Status
**✅ Approved** (2025-12-29)
**Last Reviewed**: 2026-08-01
**Confidence**: 95%
**Priority**: P0 - BLOCKER

> **Note (2026-08-01, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))**: Renamed `DD-HAPI-003` → `DD-KA-003` and rewritten against the Go implementation. The manual client this decision forbade (`pkg/holmesgpt/client/holmesgpt.go`) and the client it mandated (a hand-written HAPI OpenAPI client) have both been superseded — KA is now Go-native, and AIAnalysis talks to it via `pkg/agentclient` (an `ogen`-generated OpenAPI client with a hand-written business-friendly wrapper). The core mandate (generated client only, no manual HTTP calls) is unchanged and still enforced; the "Enforcement" section below has been corrected to describe the actual current mechanism (`.golangci.yml`'s `generated: strict` exclusion, not the shell-script + CI-step scheme this document originally proposed, which was never implemented).

## Context & Problem

**Problem (historical)**: The AIAnalysis service was using a manual HTTP client wrapper (`pkg/holmesgpt/client/holmesgpt.go`) to call the (then-Python) HolmesGPT-API endpoints, instead of using an auto-generated OpenAPI client. This caused:

1. **E2E Test Failures**: HTTP 500 errors in E2E tests due to request format mismatches
2. **Type Safety Violations**: Manual JSON marshaling bypassed compile-time type checking
3. **Contract Drift**: No guarantee that requests matched the HAPI OpenAPI specification
4. **Maintenance Burden**: Manual updates required whenever the HAPI API changed

**Key Requirements**:
- **BR-AI-006**: KA integration with type-safe requests/responses
- **ADR-031**: OpenAPI Specification Standard for REST APIs
- **DD-AUDIT-004**: Type-safe audit event data structures

**Root Cause**: The manual HTTP client was created before OpenAPI client generation tooling was established, and was never migrated.

## Alternatives Considered

### Alternative 1: Continue Using Manual HTTP Client (Status Quo)
**Approach**: Keep the manual HTTP client wrapper in `pkg/holmesgpt/client/holmesgpt.go`

**Cons**:
- ❌ **BLOCKER**: Causing E2E test failures (HTTP 500 errors)
- ❌ No compile-time type safety
- ❌ Request format could drift from the API spec
- ❌ Violates ADR-031 (OpenAPI Standard)

**Confidence**: 0% (rejected - causes production failures)

---

### Alternative 2: Use Generated OpenAPI Client (RECOMMENDED, APPROVED)
**Approach**: Migrate all client code to use an auto-generated OpenAPI client

**Pros**:
- ✅ **Compile-time type safety** - Invalid requests caught at build time
- ✅ **Contract compliance** - Guaranteed to match the OpenAPI spec
- ✅ **Auto-regeneration** - Run `go generate` when the spec changes
- ✅ **Consistent with Data Storage** - Same pattern (`ogen`) across all OpenAPI services
- ✅ **ADR-031 compliance** - Aligns with architectural standard

**Confidence**: 95% (approved)

---

### Alternative 3: Dual Client Support (Manual + OpenAPI)
**Approach**: Support both manual and generated clients, gradually migrate

**Cons**:
- ❌ Confusing for developers - which client should I use?
- ❌ Maintenance burden - two codepaths to maintain
- ❌ Technical debt - manual client eventually removed anyway

**Confidence**: 10% (rejected - adds complexity without benefit)

---

## Decision

**APPROVED: Alternative 2** - Use Generated OpenAPI Client (Mandatory)

**Rationale** (unchanged since original approval):
1. Fixes request-format-mismatch failures by guaranteeing contract compliance
2. Type safety is non-negotiable per ADR-031 and DD-AUDIT-004
3. Consistency across services — Data Storage and KA both use `ogen`-generated clients
4. Architectural compliance with ADR-031's OpenAPI mandate

## Implementation (Current, Go)

### Primary Implementation Files

**Production Code**:
- `pkg/agentclient/client.go` — `KubernautAgentClient`: hand-written, business-friendly wrapper around the generated client (constructors `NewKubernautAgentClient` / `NewKubernautAgentClientWithTransport`)
- `pkg/agentclient/oas_client_gen.go` — Auto-generated OpenAPI client (`ogen`)
- `pkg/agentclient/oas_schemas_gen.go` — Auto-generated types
- `pkg/aianalysis/handlers/interfaces.go` — `AgentClientInterface`, the interface AIAnalysis's handlers depend on (enables mocking in unit tests without touching the generated client)

**Test Code**:
- `test/shared/mocks/agentclient.go` — `MockAgentClient` (interface-level mock, not a fake HTTP server)
- `test/integration/aianalysis/*` — exercise `KubernautAgentClient` against a real or `httptest` KA instance
- `test/e2e/kubernautagent/*` — use `agentclient.NewClient(...)` directly against a running KA

### Migration Pattern

**Forbidden (manual client)**:
```go
// FORBIDDEN: Manual HTTP client
func (c *HolmesGPTClient) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
    body, err := json.Marshal(req)
    // ... manual HTTP handling
}
```

**Required (generated OpenAPI client)** (`pkg/agentclient/client.go`):
```go
// ✅ CORRECT: Generated OpenAPI client, wrapped for ergonomics
func (c *KubernautAgentClient) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
    // c.client is the ogen-generated client; Investigate additionally handles
    // the interactive-session polling loop (awaitSession) transparently.
    ...
}
```

`KubernautAgentClient` exposes several methods beyond the original single-call `Investigate` — `SubmitInvestigation`, `PollSession`, `GetSessionResult`, `CancelSession` — reflecting KA's session-based interactive investigation model (BR-INTERACTIVE-010), which did not exist when this DD was first approved.

## Consequences

### Positive
- ✅ Compile-time type safety for all AIAnalysis ↔ KA communication
- ✅ Contract compliance guaranteed by `ogen` generation from KA's OpenAPI spec
- ✅ Automated regeneration when KA's spec changes
- ✅ Consistent architecture with Data Storage's OpenAPI client pattern

### Negative
- ⚠️ Generated code is verbose (mitigated: developers only interact with the hand-written wrapper in `client.go`)
- ⚠️ Response type assertions still required in places where the generated client returns interface types

## Related Decisions
- **Supports**: ADR-031 (OpenAPI Specification Standard for REST APIs)
- **Supports**: DD-AUDIT-004 (Type-Safe Audit Event Data)
- **Consistent With**: Data Storage OpenAPI client pattern (`pkg/datastorage/ogen-client`)
- **Supersedes**: Manual HTTP client in the retired `pkg/holmesgpt/client/holmesgpt.go`

---

## Enforcement (Corrected to Match Actual Current Mechanism)

The original version of this document proposed a dedicated `scripts/validate-openapi-client-usage.sh` linter script wired into `.golangci.yml` and a CI workflow step. The script was written, but **the CI/lint wiring was never implemented** — it had no `Makefile` target and no CI workflow reference. It also hardcoded the retired `pkg/holmesgpt/client` path and the pre-KA service list (`aianalysis`, `datastorage`, `gateway`, `notification`, `remediationorchestrator`, `signalprocessing`, `workflowexecution` — no `kubernautagent`/`agentclient` awareness), so even if it had been wired up, it would not have validated the current KA client path. It has been deleted as part of the [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806) HAPI documentation cleanup rather than updated, since no automation ever depended on it.

**What actually enforces this mandate today**:
- `pkg/agentclient/client.go` (the hand-written wrapper) is the *only* hand-written file in `pkg/agentclient/`; every other file is `ogen`-generated (`oas_*_gen.go`) and carries a "Code generated ... DO NOT EDIT" header
- `.golangci.yml`'s `generated: strict` exclusion setting, combined with an explicit `pkg/agentclient/oas_.*\.go` path exclusion (added specifically so linters don't flood findings on generated boilerplate), means `client.go` itself remains fully linted while the generated files are exempted — in practice this makes hand-written HTTP-calling code in `pkg/agentclient/` immediately visible in code review, since it would sit outside the one file that's supposed to contain it
- No automated CI gate currently blocks a new manual HTTP client from being added elsewhere in the codebase; enforcement is by code review convention, not automation

**Recommendation**: if automated enforcement is desired, a real follow-up (writing a new validation script scoped to `pkg/agentclient` and wiring it into CI) should be tracked as its own issue rather than assumed to already exist.

---

**PRIORITY**: P0 - BLOCKER
**ENFORCEMENT**: Code-review convention (no automated CI gate currently exists — see above)
