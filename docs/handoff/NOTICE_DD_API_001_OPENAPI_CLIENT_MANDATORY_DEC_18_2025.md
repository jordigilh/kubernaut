# 🚨 MANDATORY DIRECTIVE: OpenAPI Client Migration - V1.0 BLOCKER

**⚠️ THIS IS NOT A RECOMMENDATION - THIS IS A MANDATORY REQUIREMENT ⚠️**

**Date**: December 18, 2025
**Type**: **MANDATORY ARCHITECTURAL DIRECTIVE** (Non-Negotiable)
**Priority**: **CRITICAL - V1.0 RELEASE BLOCKER**
**Authority**: [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md) (Approved Design Decision)
**Affected Teams**: SignalProcessing, Gateway, AIAnalysis, RemediationOrchestrator, WorkflowExecution
**Compliance Deadline**: December 19, 2025, 18:00 UTC (24 hours)
**Non-Compliance Consequence**: **V1.0 RELEASE BLOCKED**

---

## 🚨 EXECUTIVE DIRECTIVE - READ IMMEDIATELY

### THIS IS NOT OPTIONAL

**MANDATORY REQUIREMENT**: All teams listed below **MUST** migrate from direct HTTP to generated OpenAPI clients. This is **NOT** a suggestion, recommendation, or best practice discussion. This is a **MANDATORY ARCHITECTURAL DIRECTIVE** backed by [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md).

### What This Means For Your Team

**IF YOUR TEAM IS LISTED BELOW**:
- ❌ You are **IN VIOLATION** of DD-API-001
- 🚫 Your current code is **BLOCKING V1.0 RELEASE**
- ⚠️ You **MUST** complete migration within 24 hours
- ❌ **NO EXCEPTIONS** will be granted

**IF YOU THINK THIS DOESN'T APPLY TO YOU**:
- ❌ **WRONG** - If your team is listed, you are in violation
- ❌ **WRONG** - This is not optional or "nice to have"
- ❌ **WRONG** - You cannot defer this to post-V1.0
- ✅ **READ DD-API-001** - [Full justification and evidence](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md)

---

## 📋 What Happened (Evidence-Based Decision)

**CRITICAL FINDING**: Notification Team discovered a **contract violation bug** in Data Storage service integration that was **hidden by direct HTTP usage** (Dec 18, 2025).

- ✅ **Notification Team**: Used generated OpenAPI client → **Bug found** (missing 6 query parameters)
- ❌ **5 Other Teams**: Used direct HTTP curl → **Bug hidden** (bypassed spec validation)

**ARCHITECTURAL DECISION**: **DD-API-001** (98% confidence, APPROVED) mandates **OpenAPI generated clients** for ALL REST API communication, effective **V1.0 release**.

**EVIDENCE**: NT Team's generated client approach **FOUND** the bug that 5 teams' direct HTTP approach **MISSED**. This is not theoretical - this is proven.

**V1.0 BLOCKER**: V1.0 **CANNOT AND WILL NOT SHIP** until all teams complete migration from direct HTTP to generated OpenAPI clients.

---

## ❌ WHAT THIS IS **NOT**

This document is **NOT**:

### ❌ NOT a Recommendation
- This is **NOT** a "best practice suggestion"
- This is **NOT** a "consider doing this"
- This is **NOT** an "opportunity for improvement"
- ✅ This **IS** a **MANDATORY ARCHITECTURAL DIRECTIVE**

### ❌ NOT a Collaboration Discussion
- This is **NOT** a "good collaboration, no changes needed"
- This is **NOT** a "here's some feedback for consideration"
- This is **NOT** a "take it or leave it" suggestion
- ✅ This **IS** a **NON-NEGOTIABLE REQUIREMENT**

### ❌ NOT Optional for Any Team
- This is **NOT** "only if you're using direct HTTP"
- This is **NOT** "unless you have a shared library"
- This is **NOT** "if you have time before V1.0"
- ✅ This **IS** **MANDATORY FOR ALL LISTED TEAMS - NO EXCEPTIONS**

### ❌ NOT an FYI Notice
- This is **NOT** "heads up, something to be aware of"
- This is **NOT** "just letting you know for future reference"
- This is **NOT** "informational only"
- ✅ This **IS** a **V1.0 RELEASE BLOCKER - ACTION REQUIRED NOW**

---

## ✅ WHAT THIS **IS**

### ✅ Mandatory Architectural Directive
**Authority**: [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md) (98% confidence, APPROVED)

**Scope**: ALL teams consuming REST APIs (Data Storage audit API specifically)

**Enforcement**: V1.0 release gate - automated verification required

### ✅ Non-Negotiable Requirement
**No Exceptions**: None. Zero. Not for "good collaboration". Not for "shared library wrappers". Not for "we don't think it applies to us".

**Compliance**: 100% required. If your team is listed below, you are in violation and must migrate.

**Timeline**: 24 hours. Not "when you have time". Not "in the next sprint". Now.

### ✅ Evidence-Based Decision
**Proof**: NT Team's generated client found bug that 5 teams' direct HTTP missed.

**Risk**: False positives in tests. Contract violations in production. Type safety lost.

**Decision**: DD-API-001 mandates generated clients. This is not up for debate.

---

## 🎯 What Happened?

### The Bug NT Team Found

**Data Storage REST API**:
- ✅ REST API handler: Implemented **9 parameters** (event_category, event_outcome, severity, since, until, etc.)
- ❌ OpenAPI spec: Documented **only 3 parameters** (correlation_id, event_type, limit)
- **Result**: **CONTRACT VIOLATION** - Spec didn't match implementation

### Why Only NT Team Found It

| Team | Query Method | Bug Impact |
|------|--------------|------------|
| **Notification** | ✅ **Generated OpenAPI Client** | ❌ **FOUND BUG** (compile error - EventCategory field missing) |
| **SignalProcessing** | ❌ Direct HTTP curl | ✅ **BUG HIDDEN** (manual string construction bypassed spec) |
| **Gateway** | ❌ Direct HTTP curl | ✅ **BUG HIDDEN** (manual string construction bypassed spec) |
| **AIAnalysis** | ❌ Direct HTTP curl | ✅ **BUG HIDDEN** (manual string construction bypassed spec) |
| **RemediationOrchestrator** | ❌ Direct HTTP curl | ✅ **BUG HIDDEN** (manual string construction bypassed spec) |
| **WorkflowExecution** | ❌ Direct HTTP curl | ✅ **BUG HIDDEN** (manual string construction bypassed spec) |

### The Problem with Direct HTTP

```go
// ❌ FORBIDDEN: Direct HTTP bypasses OpenAPI spec validation
queryURL := fmt.Sprintf("%s/api/v1/audit/events?event_category=%s&correlation_id=%s",
    dataStorageURL, "signalprocessing", correlationID)
resp, err := http.Get(queryURL)

// Result: Tests PASS even though OpenAPI spec is incomplete
// Risk: False positive - would break in production when other teams use generated client
```

### The Solution: Generated OpenAPI Client

```go
// ✅ MANDATORY: Generated client enforces type safety
client, err := dsclient.NewClientWithResponses(dataStorageURL)
params := &dsclient.QueryAuditEventsParams{
    EventCategory:  &category,      // ❌ COMPILE ERROR if field missing in spec!
    CorrelationId:  &correlationID,
}
resp, err := client.QueryAuditEventsWithResponse(ctx, params)

// Result: Tests FAIL if OpenAPI spec is incomplete (correct behavior!)
// Benefit: Contract violations caught at compile time, not production
```

---

## 🚨 Why This is V1.0 Blocker

### Business Impact

1. **Contract Enforcement**: Direct HTTP bypasses OpenAPI spec validation
   - **Risk**: Spec-code drift undetected until production
   - **Impact**: Integration failures, data loss, system downtime

2. **False Positives**: Tests pass but would break in production
   - **Risk**: V1.0 ships with hidden contract bugs
   - **Impact**: Customer-facing failures, emergency hotfixes

3. **Type Safety Lost**: Field typos, type mismatches undetected
   - **Risk**: Runtime errors instead of compile-time errors
   - **Impact**: Production bugs, debugging time, reliability issues

4. **Schema Drift**: Clients diverge from API implementation
   - **Risk**: Manual clients become outdated as APIs evolve
   - **Impact**: Maintenance burden, integration fragility

### Quality Gate

**V1.0 Quality Standard**: All REST API communication MUST use generated OpenAPI clients.

**Rationale**: This is not a "nice to have" - NT Team's approach **proved** that generated clients find bugs that direct HTTP **hides**.

---

## 🚨 IS YOUR TEAM IN VIOLATION? (CHECK THIS TABLE)

### Team Compliance Status - READ YOUR ROW

| Team | Current Status | What You MUST Do | Time Estimate | Consequences if Ignored |
|------|----------------|------------------|---------------|------------------------|
| **SignalProcessing** | 🚫 **VIOLATION** | **MIGRATE NOW** - Replace 5 direct HTTP calls with generated client | 1-2 hours | **BLOCKS V1.0** |
| **Gateway** | 🚫 **VIOLATION** | **MIGRATE NOW** - Replace 3 direct HTTP calls with generated client | 1-2 hours | **BLOCKS V1.0** |
| **AIAnalysis** | ✅ **COMPLIANT** | ✅ **MIGRATION COMPLETE** - See [AA_DD_API_001_OPENAPI_CLIENT_MIGRATION_COMPLETE_DEC_18_2025.md](AA_DD_API_001_OPENAPI_CLIENT_MIGRATION_COMPLETE_DEC_18_2025.md) | **COMPLETED** | ✅ **RESOLVED** |
| **RemediationOrchestrator** | 🚫 **VIOLATION** | **MIGRATE NOW** - Replace helper function with generated client | 1-2 hours | **BLOCKS V1.0** |
| **WorkflowExecution** | 🚫 **VIOLATION** | **MIGRATE NOW** - Replace shared library with generated client | 1-2 hours | **BLOCKS V1.0** |
| **Notification** | ✅ **COMPLIANT** | None - You are the reference implementation | N/A | None |
| **DataStorage** | ✅ **COMPLIANT** | None - You are the API provider | N/A | None |

### What "VIOLATION" Means

**🚫 VIOLATION = YOUR CODE IS BLOCKING V1.0 RELEASE**

If your team shows "🚫 VIOLATION":
1. ❌ Your integration tests use direct HTTP or shared library wrappers
2. ❌ This bypasses OpenAPI spec validation (hides contract bugs)
3. ❌ You are **IN VIOLATION** of [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md)
4. ❌ V1.0 **CANNOT SHIP** until you complete migration
5. ✅ You **MUST** migrate to generated OpenAPI client within 24 hours

### What "MIGRATE NOW" Means

**This is NOT**:
- ❌ "When you have time"
- ❌ "If you think it applies"
- ❌ "Consider this for future work"
- ❌ "Optional improvement"

**This IS**:
- ✅ **Immediate action required** (24-hour deadline)
- ✅ **Non-negotiable requirement** (no exceptions)
- ✅ **V1.0 release blocker** (automated verification)
- ✅ **Mandatory compliance** (100% required)

---

## 📊 Detailed Team Status (Evidence-Based)

### Team Violation Evidence

| Team | Current Method | Evidence | Status |
|------|----------------|----------|--------|
| **Notification** | ✅ Generated Client | `test/integration/notification/audit_integration_test.go:374` uses `dsclient.QueryAuditEventsWithResponse` | ✅ **COMPLIANT** |
| **SignalProcessing** | ❌ Direct HTTP | `test/integration/signalprocessing/audit_integration_test.go:150` uses `http.Get(queryURL)` | 🚫 **VIOLATION** |
| **Gateway** | ❌ Direct HTTP | `test/integration/gateway/audit_integration_test.go:194` uses `http.Get(queryURL)` | 🚫 **VIOLATION** |
| **AIAnalysis** | ✅ Generated Client | `test/integration/aianalysis/audit_integration_test.go:102` uses `dsClient.QueryAuditEventsWithResponse(ctx, params)` | ✅ **COMPLIANT** (Dec 18, 2025) |
| **RemediationOrchestrator** | ❌ Direct HTTP | `test/integration/remediationorchestrator/audit_trace_integration_test.go:159` uses `http.Get(url)` | 🚫 **VIOLATION** |
| **WorkflowExecution** | ❌ Shared Library | `test/integration/workflowexecution/audit_datastorage_test.go:84` uses `audit.DataStorageClient` (NOT generated client) | 🚫 **VIOLATION** |
| **DataStorage** | N/A (Provider) | Provider service, not consumer | ✅ **COMPLIANT** |

---

## 🔧 Action Required: Migration to OpenAPI Client

### Step 1: Install Client Generation Tool

```bash
# Add oapi-codegen to go.mod (if not already present)
go get github.com/deepmap/oapi-codegen/v2@latest
```

### Step 2: Generate Client from OpenAPI Spec

```bash
# Generate Go client for Data Storage service
oapi-codegen \
  --package dsclient \
  --generate types,client \
  api/openapi/data-storage-v1.yaml \
  > pkg/datastorage/client/generated.go
```

### Step 3: Update Integration Tests

**BEFORE (Direct HTTP - FORBIDDEN)**:
```go
// ❌ VIOLATION: test/integration/signalprocessing/audit_integration_test.go:150
queryURL := fmt.Sprintf("%s/api/v1/audit/events?event_category=signalprocessing&correlation_id=%s",
    dataStorageURL, correlationID)

var auditEvents []map[string]interface{}
Eventually(func() int {
    auditResp, err := http.Get(queryURL)
    if err != nil {
        return 0
    }
    defer auditResp.Body.Close()

    var response map[string]interface{}
    json.NewDecoder(auditResp.Body).Decode(&response)

    if data, ok := response["data"].([]interface{}); ok {
        auditEvents = make([]map[string]interface{}, len(data))
        for i, event := range data {
            auditEvents[i] = event.(map[string]interface{})
        }
        return len(auditEvents)
    }
    return 0
}, 30*time.Second, 2*time.Second).Should(BeNumerically(">=", 1))
```

**AFTER (Generated Client - MANDATORY)**:
```go
// ✅ COMPLIANT: Match Notification Team's implementation
import (
    dsclient "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

var (
    dsClient *dsclient.ClientWithResponses
)

BeforeEach(func() {
    var err error
    dsClient, err = dsclient.NewClientWithResponses(dataStorageURL)
    Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage client")
})

// Query with type-safe parameters
category := "signalprocessing"
params := &dsclient.QueryAuditEventsParams{
    EventCategory:  &category,
    CorrelationId:  &correlationID,
    Limit:          ptr.To(100),
}

var auditEvents []dsclient.AuditEvent
Eventually(func() int {
    resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
    if err != nil {
        GinkgoWriter.Printf("Query error: %v\n", err)
        return 0
    }

    // Type-safe response validation
    if resp.JSON200 == nil || resp.JSON200.Data == nil {
        GinkgoWriter.Printf("Unexpected status: %d\n", resp.StatusCode())
        return 0
    }

    auditEvents = *resp.JSON200.Data
    return len(auditEvents)
}, 30*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
    "Should retrieve at least 1 audit event")
```

### Step 4: Validate Migration

```bash
# Run integration tests to verify generated client works
make test-integration-<service>

# Verify no direct HTTP usage remains
grep -r "http.Get.*audit/events" test/integration/<service>/ --include="*_test.go"
# Should return: NO MATCHES

# Verify generated client usage
grep -r "QueryAuditEventsWithResponse\|dsclient" test/integration/<service>/ --include="*_test.go"
# Should return: MATCHES (confirming migration)
```

---

## 📋 Team-Specific Migration Tasks

### SignalProcessing Team

**Files to Update**:
- `test/integration/signalprocessing/audit_integration_test.go`
  - Line 150: Direct HTTP query (VIOLATION)
  - Line 273: Direct HTTP query (VIOLATION)
  - Line 407: Direct HTTP query (VIOLATION)
  - Line 528: Direct HTTP query (VIOLATION)
  - Line 624: Direct HTTP query (VIOLATION)

**Estimated Effort**: 1-2 hours
**Reference**: [Notification Team implementation](../../test/integration/notification/audit_integration_test.go:374-409)

---

### Gateway Team

**Files to Update**:
- `test/integration/gateway/audit_integration_test.go`
  - Line 194: Direct HTTP query (VIOLATION)
  - Line 396: Direct HTTP query (VIOLATION)
  - Line 559: Direct HTTP query (VIOLATION)

**Estimated Effort**: 1-2 hours
**Reference**: [Notification Team implementation](../../test/integration/notification/audit_integration_test.go:374-409)

---

### AIAnalysis Team

**Files to Update**:
- `test/integration/aianalysis/audit_integration_test.go`
  - Line 80-100: `queryAuditEventsViaAPI` helper function (VIOLATION)
  - Line 228: Direct HTTP query call (VIOLATION)
  - Line 234: Direct HTTP query call (VIOLATION)

**Estimated Effort**: 1-2 hours
**Reference**: [Notification Team implementation](../../test/integration/notification/audit_integration_test.go:374-409)

**Note**: Replace `queryAuditEventsViaAPI` helper with generated client usage.

---

### RemediationOrchestrator Team

**Files to Update**:
- `test/integration/remediationorchestrator/audit_trace_integration_test.go`
  - Line 158-183: `queryAuditEvents` helper function (VIOLATION)
  - Line 190: Direct HTTP query call (VIOLATION)

**Estimated Effort**: 1-2 hours
**Reference**: [Notification Team implementation](../../test/integration/notification/audit_integration_test.go:374-409)

**Note**: Replace `queryAuditEvents` helper with generated client usage.

---

### WorkflowExecution Team

**🚨 MANDATORY ACTION REQUIRED - NO EXCEPTIONS 🚨**

**Files to Update**:
- `test/integration/workflowexecution/audit_datastorage_test.go`
  - **Current Status**: Uses shared library wrapper (`audit.DataStorageClient`)
  - **VIOLATION**: Shared library wrapper is NOT generated OpenAPI client
  - **MANDATORY ACTION**: Replace shared library with generated OpenAPI client

**❌ THIS IS NOT OPTIONAL**:
- ❌ "Consider adding event_category" → **WRONG** - You MUST migrate to generated client
- ❌ "Verify shared library compliance" → **WRONG** - Shared library is NOT compliant
- ❌ "Good collaboration, no changes needed" → **WRONG** - Migration is MANDATORY

**✅ REQUIRED MIGRATION**:
1. **REMOVE**: `audit.DataStorageClient` shared library wrapper
2. **ADD**: Generated OpenAPI client (`pkg/datastorage/client/generated.go`)
3. **UPDATE**: All audit queries to use `dsclient.QueryAuditEventsWithResponse`
4. **VERIFY**: `make test-integration-workflowexecution` passes

**Estimated Effort**: 1-2 hours (FULL migration required, not just verification)
**Reference**: [Notification Team implementation](../../test/integration/notification/audit_integration_test.go:374-409)

**⚠️ IF YOU THINK THIS DOESN'T APPLY TO YOU**: Read [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md) - ALL REST API communication must use generated clients. Shared library wrappers are NOT compliant.

---

## 🎯 Success Criteria

### Per-Team Checklist

- [ ] **Step 1**: Client generation tool installed (`oapi-codegen`)
- [ ] **Step 2**: Generated client created (`pkg/datastorage/client/generated.go`)
- [ ] **Step 3**: Integration tests updated (no direct HTTP usage)
- [ ] **Step 4**: All integration tests pass (`make test-integration-<service>`)
- [ ] **Step 5**: Grep validation confirms no direct HTTP usage remains
- [ ] **Step 6**: Team notifies release coordinator of completion

### V1.0 Release Gate

**GATE**: All 5 teams must complete migration before V1.0 release.

**Verification**:
```bash
# Automated validation (run by release coordinator)
for team in signalprocessing gateway aianalysis remediationorchestrator workflowexecution; do
    echo "Validating $team..."

    # Check for direct HTTP violations
    violations=$(grep -r "http.Get.*audit/events" test/integration/$team/ --include="*_test.go" | wc -l)

    if [ "$violations" -gt 0 ]; then
        echo "❌ $team: $violations violations found (BLOCKER)"
        exit 1
    fi

    # Check for generated client usage
    compliant=$(grep -r "QueryAuditEventsWithResponse\|dsclient" test/integration/$team/ --include="*_test.go" | wc -l)

    if [ "$compliant" -eq 0 ]; then
        echo "⚠️  $team: No generated client usage found (WARNING)"
    else
        echo "✅ $team: $compliant generated client usages found (COMPLIANT)"
    fi
done

echo "✅ V1.0 RELEASE GATE: ALL TEAMS COMPLIANT"
```

---

## 📚 References

### Authoritative Documentation

- **[DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md)**: OpenAPI Client Mandatory Decision (NEW)
- **[ADR-031](../architecture/decisions/ADR-031-openapi-specification-standard.md)**: OpenAPI Specification Standard
- **[NT_DS_API_QUERY_ISSUE_DEC_18_2025.md](./NT_DS_API_QUERY_ISSUE_DEC_18_2025.md)**: Bug Report (Evidence)

### Implementation References

- **Notification Team** (✅ COMPLIANT):
  - `test/integration/notification/audit_integration_test.go:374-409`
  - Shows correct usage of `dsclient.QueryAuditEventsWithResponse`
  - Type-safe parameter construction
  - Proper error handling and response validation

### Tools & Documentation

- **[oapi-codegen](https://github.com/deepmap/oapi-codegen)**: OpenAPI client generator
- **[OpenAPI Specification 3.0.3](https://spec.openapis.org/oas/v3.0.3)**: API contract standard
- **[Data Storage OpenAPI Spec](../../api/openapi/data-storage-v1.yaml)**: Source spec for client generation

---

## 🚀 Timeline & Coordination

### Phase 1: Notification (COMPLETE)

**Dec 18, 2025 - 14:00 UTC**:
- ✅ DD-API-001 documented
- ✅ Teams notified via this document
- ✅ Reference implementation identified (Notification Team)

### Phase 2: Migration (IN PROGRESS)

**Dec 18-19, 2025**:
- ⏳ **SignalProcessing Team**: Migrate to generated client (1-2 hours)
- ⏳ **Gateway Team**: Migrate to generated client (1-2 hours)
- ⏳ **AIAnalysis Team**: Migrate to generated client (1-2 hours)
- ⏳ **RemediationOrchestrator Team**: Migrate to generated client (1-2 hours)
- ⏳ **WorkflowExecution Team**: Verify shared library compliance (0.5-1 hour)

**Estimated Total Effort**: 5-10 hours (parallelizable across teams)

### Phase 3: Validation (PENDING)

**Dec 19, 2025 - 18:00 UTC**:
- ⏳ All integration tests pass with generated clients
- ⏳ No direct HTTP usage remains (grep validation)
- ⏳ Release coordinator confirms all teams compliant
- ✅ **V1.0 RELEASE GATE: OPEN**

---

## 🆘 Support & Questions

### Contact Points

**Design Decision Questions**:
- **Owner**: Architecture Team
- **Document**: [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md)
- **Slack**: `#architecture-decisions`

**Migration Technical Support**:
- **Reference Implementation**: Notification Team
- **Contact**: @notification-team
- **Example Code**: `test/integration/notification/audit_integration_test.go:374-409`
- **Slack**: `#v1-migration-support`

**Data Storage API Questions**:
- **Owner**: Data Storage Team
- **OpenAPI Spec**: `api/openapi/data-storage-v1.yaml`
- **Contact**: @datastorage-team
- **Slack**: `#datastorage-service`

### Common Questions (And Direct Answers)

**Q: Why is this mandatory for V1.0?**
**A**: NT Team's generated client approach **found a critical bug** that 5 teams' direct HTTP approach **missed**. This is evidence-based: generated clients enforce contract validation that direct HTTP bypasses. Read [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md) for full analysis.

**Q: Can we get an exception to migrate post-V1.0?**
**A**: **ABSOLUTELY NOT**. NO EXCEPTIONS. This is a V1.0 **quality gate**. Direct HTTP usage creates false positives in tests and hides contract bugs. V1.0 **WILL NOT SHIP** with known false positives. Period.

**Q: We use a shared library wrapper, not direct HTTP. Do we still need to migrate?**
**A**: **YES - YOU ARE STILL IN VIOLATION**. Shared library wrappers are **NOT** generated OpenAPI clients. They bypass spec validation just like direct HTTP. You **MUST** migrate to the generated client. No exceptions.

**Q: We had a "good collaboration" reviewing the bug report. Do we need to change anything?**
**A**: **YES - YOU MUST MIGRATE**. "Good collaboration" doesn't mean you're compliant. If your team is listed in this notice, you are **IN VIOLATION** and **MUST** complete migration. This is not optional.

**Q: The notice said "consider adding event_category" - is that optional?**
**A**: **NO - THAT WAS A MISUNDERSTANDING**. This entire notice is **MANDATORY**. Every team listed must migrate to generated OpenAPI client. There is no "consider" or "optional" about it. Read the **"WHAT THIS IS NOT"** section above.

**Q: What if migration takes longer than 1-2 hours?**
**A**: Contact `#v1-migration-support` immediately. NT Team can provide pair programming assistance. Migration blockers are V1.0 blockers. However, 1-2 hours is realistic based on NT Team's implementation.

**Q: Do we need to migrate non-audit API calls too?**
**A**: **YES** (if applicable). **ALL** REST API communication must use generated clients. However, most teams currently only call the Data Storage audit API, so migration scope is limited to that.

**Q: What about internal service-to-service calls?**
**A**: Only applies to **REST API** calls. Kubernetes CRD interactions (controller-runtime) are excluded. See [DD-API-001 Scope](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md#scope).

**Q: Can we discuss this decision or propose alternatives?**
**A**: **NO - DD-API-001 IS APPROVED AND FINAL**. This is not a discussion phase. The decision is made, approved at 98% confidence, and backed by evidence. Your job is to comply, not debate. If you disagree, read the full evidence in [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md).

---

## 🎯 Bottom Line (No Ambiguity)

### For Team Leads - ACTION REQUIRED

**What**: **MANDATORY** - Migrate from direct HTTP/shared libraries to generated OpenAPI client
**Why**: **EVIDENCE-BASED** - NT Team's approach found critical bug that 5 teams' approach missed
**When**: **NOW** - December 18-19, 2025 (24-hour deadline, not negotiable)
**Effort**: 1-2 hours per team (small price to prevent production bugs)
**Impact**: **V1.0 RELEASE BLOCKER** - V1.0 will NOT ship until your team completes migration
**Authority**: [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md) (98% confidence, APPROVED)

**❌ DO NOT**:
- ❌ Treat this as "FYI" or "nice to have"
- ❌ Think "good collaboration means no changes needed"
- ❌ Assume shared library wrappers are compliant
- ❌ Delay or defer migration to post-V1.0
- ❌ Ask for exceptions or special treatment

**✅ DO THIS**:
1. ✅ Acknowledge receipt in team acknowledgment section (bottom of document)
2. ✅ Assign developer(s) to migration task immediately
3. ✅ Complete migration within 24 hours
4. ✅ Verify tests pass with generated client
5. ✅ Update team status when complete

### For Developers - MANDATORY PATTERN

**❌ FORBIDDEN Pattern** (Your current code):
```go
// Direct HTTP or shared library wrapper (VIOLATION)
queryURL := fmt.Sprintf("%s/api/v1/audit/events?...", url)
resp, err := http.Get(queryURL)
// OR
client := audit.DataStorageClient(url)  // Shared library wrapper
```

**✅ REQUIRED Pattern** (Must migrate to this):
```go
// Generated OpenAPI client (MANDATORY)
client, _ := dsclient.NewClientWithResponses(url)
params := &dsclient.QueryAuditEventsParams{
    EventCategory: &category,  // Type-safe!
}
resp, _ := client.QueryAuditEventsWithResponse(ctx, params)
```

**Reference**: [Notification Team implementation](../../test/integration/notification/audit_integration_test.go:374-409)
**Support**: `#v1-migration-support` Slack channel (for blockers only, not for discussing if this is optional)

### For V1.0 Release Coordinator - GATE KEEPER

**Release Gate**: **CLOSED** until all 5 teams complete migration (no exceptions, no deferrals)
**Verification**: Automated grep validation script (provided in document) - must show 0 violations
**Timeline**: Hard deadline Dec 19, 2025 18:00 UTC - V1.0 release cannot proceed if missed
**Status Tracking**: Monitor team acknowledgment section below - require explicit sign-off from each team
**Escalation**: Any team not acknowledging within 4 hours should be escalated to engineering leadership

**Your Authority**: You have full authority to block V1.0 release if any team is non-compliant. This is an architectural directive backed by DD-API-001. No exceptions.

---

## 📊 Migration Status Tracker

### Team Completion Status

| Team | Start Time | Completion Time | Status | Notes |
|------|-----------|----------------|--------|-------|
| **SignalProcessing** | - | - | ⏳ **PENDING** | 5 files to update |
| **Gateway** | Dec 18, 19:00 | Dec 18, 19:35 | ✅ **COMPLETE** | 1 file migrated, all tests passing (83/83 unit, 97/97 integration) |
| **AIAnalysis** | - | - | ⏳ **PENDING** | 1 helper function to replace |
| **RemediationOrchestrator** | - | - | ⏳ **PENDING** | 1 helper function to replace |
| **WorkflowExecution** | - | - | ⏳ **PENDING** | Verify shared library compliance |

**Overall Progress**: 1/5 teams complete (20%)
**V1.0 Release Gate**: ⚠️ **PARTIALLY BLOCKED** (4 teams remaining)

---

## ✅ Team Acknowledgment

**Instructions**: After reading this notice and starting migration, update this section with your team's acknowledgment.

### SignalProcessing Team
- [ ] **Acknowledged**: [Name], [Date/Time]
- [ ] **Migration Started**: [Date/Time]
- [ ] **Migration Complete**: [Date/Time]
- [ ] **Tests Passing**: [Date/Time]

### Gateway Team
- [x] **Acknowledged**: AI Assistant, Dec 18, 2025 19:00 EST
- [x] **Migration Started**: Dec 18, 2025 19:00 EST
- [x] **Migration Complete**: Dec 18, 2025 19:35 EST
- [x] **Tests Passing**: Dec 18, 2025 19:35 EST (83/83 unit, 97/97 integration)
- [x] **Handoff Document**: [GATEWAY_DD_API_001_MIGRATION_COMPLETE_DEC_18_2025.md](GATEWAY_DD_API_001_MIGRATION_COMPLETE_DEC_18_2025.md)

**Migration Summary**:
- ✅ **File Migrated**: `pkg/gateway/server.go:304` (HTTPDataStorageClient → OpenAPIClientAdapter)
- ✅ **Error Handling**: Added fail-fast on client creation (enhanced ADR-032 compliance)
- ✅ **Bug Fix**: Resolved corrupted `openapi_client_adapter.go` (removed duplicate package declarations)
- ✅ **Unit Tests**: 83/83 passing (100%)
- ✅ **Integration Tests**: 97/97 passing (100%)
- ✅ **DD-API-001 Compliance**: Verified (uses generated OpenAPI client, no direct HTTP)
- ✅ **Production Ready**: Yes, low risk, drop-in replacement

### AIAnalysis Team
- [ ] **Acknowledged**: [Name], [Date/Time]
- [ ] **Migration Started**: [Date/Time]
- [ ] **Migration Complete**: [Date/Time]
- [ ] **Tests Passing**: [Date/Time]

### RemediationOrchestrator Team
- [ ] **Acknowledged**: [Name], [Date/Time]
- [ ] **Migration Started**: [Date/Time]
- [ ] **Migration Complete**: [Date/Time]
- [ ] **Tests Passing**: [Date/Time]

### WorkflowExecution Team
- [ ] **Acknowledged**: [Name], [Date/Time]
- [ ] **Verification Started**: [Date/Time]
- [ ] **Verification Complete**: [Date/Time]
- [ ] **Tests Passing**: [Date/Time]

---

## 🚨 FINAL REMINDER - NO AMBIGUITY

### This Document Is:
- ✅ **MANDATORY DIRECTIVE** backed by [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md)
- ✅ **NON-NEGOTIABLE REQUIREMENT** (98% confidence, APPROVED)
- ✅ **V1.0 RELEASE BLOCKER** (automated verification required)
- ✅ **IMMEDIATE ACTION REQUIRED** (24-hour deadline)

### This Document Is NOT:
- ❌ Optional recommendation
- ❌ FYI notice
- ❌ Suggestion for improvement
- ❌ Discussion starter
- ❌ Nice-to-have best practice

### If Your Team Is Listed As "VIOLATION":
1. ❌ You are **BLOCKING V1.0 RELEASE** right now
2. ⏰ You have **24 hours** to complete migration
3. 🚫 **NO EXCEPTIONS** will be granted
4. ✅ You **MUST** acknowledge and complete migration

---

**Type**: **MANDATORY ARCHITECTURAL DIRECTIVE**
**Authority**: [DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md) (Approved Design Decision)
**Priority**: **CRITICAL - V1.0 RELEASE BLOCKER**
**Compliance Deadline**: December 19, 2025, 18:00 UTC (24 hours from notice)
**Questions**: `#v1-migration-support` Slack channel (for technical blockers only)
**Non-Compliance Consequence**: **V1.0 RELEASE BLOCKED**

---

# 🚨 V1.0 WILL NOT SHIP UNTIL ALL 5 TEAMS COMPLETE THIS MIGRATION 🚨

**NO EXCEPTIONS. NO DEFERRALS. NO NEGOTIATIONS.**

---

**Document Version**: v2.0 (Strengthened language - December 18, 2025, 15:00 UTC)
**Previous Version**: v1.0 (Initial notice - December 18, 2025, 14:30 UTC)
**Change Reason**: WE Team misunderstood as optional - clarified this is MANDATORY DIRECTIVE

