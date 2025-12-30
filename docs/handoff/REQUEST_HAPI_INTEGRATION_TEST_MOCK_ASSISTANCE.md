# REQUEST: HAPI Team Assistance for Integration Test Mock Conversion

| **Field** | **Value** |
|-----------|-----------|
| **From** | AIAnalysis Team |
| **To** | HolmesGPT-API (HAPI) Team |
| **Date** | 2025-12-09 |
| **Priority** | 🟡 Medium |
| **Type** | Technical Assistance Request |
| **Status** | ✅ HAPI Response Provided (Dec 9, 2025) |

---

## 📋 Summary

The AIAnalysis team has created integration tests in `test/integration/aianalysis/holmesgpt_integration_test.go` that test the HolmesGPT client. Currently, these tests expect a **real HAPI server** running, which creates a dependency on HAPI infrastructure during AIAnalysis integration testing.

We request the HAPI team's assistance to:
1. Review our integration test approach
2. Advise on the best mock pattern for HAPI client integration tests
3. Optionally provide a reusable mock server or test fixture

---

## 🎯 Current State

### Test File Location
```
test/integration/aianalysis/holmesgpt_integration_test.go
```

### Current Test Expectations
The tests currently expect:
- A real HolmesGPT-API server at `http://localhost:8088`
- Working `/api/v1/analyze/investigate` endpoint
- Full response structure matching `IncidentResponse`

### Test Scenarios Covered
1. **Successful Investigation** - Basic incident analysis
2. **Target in Owner Chain** - Kubernetes resource hierarchy
3. **Alternative Workflows** - Multiple workflow suggestions
4. **Human Review Flag** - `needs_human_review=true` scenarios

---

## ❓ Questions for HAPI Team

### Q1: Recommended Mock Pattern

**Question**: What is the HAPI team's recommended approach for other teams to mock HAPI in their integration tests?

**Options we considered**:
- **A)** Use a shared mock HTTP server (like `httptest.Server` with canned responses)
- **B)** Use `testutil.MockHolmesGPTClient` (already exists for unit tests)
- **C)** HAPI team provides a test container image with mock mode
- **D)** Other approach

**AIAnalysis Team Preference**: Option B (using existing mock client), but want HAPI team guidance.

### Q2: Mock Response Fixtures

**Question**: Does the HAPI team have canonical test fixtures (JSON responses) for common scenarios that other teams should use?

**Examples needed**:
- Successful workflow selection with high confidence
- `needs_human_review=true` with all 7 `human_review_reason` values
- `investigation_inconclusive` scenario
- Problem resolved (no workflow needed) scenario

### Q3: Contract Testing

**Question**: Does HAPI have consumer contract tests that AIAnalysis should participate in?

**Context**: We want to ensure our client stays in sync with HAPI's API evolution.

---

## 📂 Current Integration Test Code

```go
// test/integration/aianalysis/holmesgpt_integration_test.go

var _ = Describe("HolmesGPT-API Integration", Label("integration", "holmesgpt"), func() {
    var (
        hgClient *client.HolmesGPTClient
        ctx      context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        hgClient = client.NewHolmesGPTClient(client.Config{
            BaseURL: "http://localhost:8088", // Expects real server
            Timeout: 30 * time.Second,
        })
    })

    It("should successfully investigate an incident", func() {
        req := &client.IncidentRequest{
            Context: "CrashLoopBackOff in production...",
        }
        resp, err := hgClient.Investigate(ctx, req)
        Expect(err).ToNot(HaveOccurred())
        // ... assertions
    })
})
```

---

## 🔄 Proposed Conversion (Pending HAPI Guidance)

Based on HAPI team's response, we plan to convert to:

```go
// Option B: Use existing mock client (if HAPI approves)

var _ = Describe("HolmesGPT-API Integration", Label("integration", "holmesgpt"), func() {
    var (
        mockClient *testutil.MockHolmesGPTClient
        ctx        context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        mockClient = testutil.NewMockHolmesGPTClient().
            WithFullResponse(/* canonical fixture from HAPI */)
    })

    It("should successfully investigate an incident", func() {
        req := &client.IncidentRequest{Context: "..."}
        resp, err := mockClient.Investigate(ctx, req)
        // ... same assertions
    })
})
```

---

## 📊 Impact

| Metric | Current | After Conversion |
|--------|---------|------------------|
| External dependency | HAPI server required | None |
| Test execution time | ~30s (network I/O) | ~1s |
| CI reliability | Medium (depends on HAPI) | High |
| Contract validation | Real | Mock (less rigorous) |

---

## ⏱️ Timeline

| Task | Owner | ETA | Status |
|------|-------|-----|--------|
| HAPI team responds with recommendation | HAPI | Dec 10, 2025 | ✅ Done (Dec 9) |
| AIAnalysis implements conversion | AIAnalysis | Dec 11, 2025 | ✅ Done (Dec 9) |
| Verify integration tests pass | AIAnalysis | Dec 11, 2025 | ✅ Done (Dec 9) - **12/12 tests passing** |

---

## 📚 References

- `test/integration/aianalysis/holmesgpt_integration_test.go` - Current test file
- `pkg/testutil/mock_holmesgpt_client.go` - Existing mock client
- `docs/services/crd-controllers/02-aianalysis/IMPLEMENTATION_PLAN_V1.0.md` - Day 7 integration tests
- `DD-TEST-001` - Port allocation strategy

---

## 📝 Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| v1.0 | 2025-12-09 | AIAnalysis Team | Initial request |
| v1.1 | 2025-12-09 | HAPI Team | Provided responses to Q1-Q3 |
| v1.2 | 2025-12-09 | AIAnalysis Team | Implemented conversion to MockHolmesGPTClient |
| v1.1 | 2025-12-09 | HAPI Team | Response provided: Use MockHolmesGPTClient |

---

## HAPI Team Response

**Date**: December 9, 2025
**Responder**: HAPI Team

### Q1 Response (Mock Pattern)

✅ **Recommended: Option B - Use existing `testutil.MockHolmesGPTClient`**

Per `TESTING_GUIDELINES.md` (lines 340-342):

| Test Type | Infrastructure | LLM |
|-----------|----------------|-----|
| **Unit Tests** | Mock ✅ | Mock ✅ |
| **Integration Tests** | Mock ✅ | Mock ✅ |
| **E2E Tests** | **REAL** ❌ | Mock ✅ |

The existing mock client at `pkg/testutil/mock_holmesgpt_client.go` is **comprehensive** with 20+ helper methods covering all ADR-045 contract scenarios. **Do not use `httptest.Server` or real HAPI** for integration tests.

**Conversion example**:
```go
// ✅ CORRECT: Use MockHolmesGPTClient for integration tests
import "github.com/jordigilh/kubernaut/pkg/testutil"

var _ = Describe("HolmesGPT Integration", func() {
    var mockClient *testutil.MockHolmesGPTClient

    BeforeEach(func() {
        mockClient = testutil.NewMockHolmesGPTClient()
    })

    It("should handle successful investigation", func() {
        mockClient.WithFullResponse(
            "Analysis result",
            0.85,
            true, // targetInOwnerChain
            []string{},
            &client.RootCauseAnalysis{Summary: "OOMKilled", Severity: "high"},
            &client.SelectedWorkflow{WorkflowID: "restart-pod", Confidence: 0.85},
            nil,
        )
        // ... test assertions
    })
})
```

### Q2 Response (Test Fixtures)

✅ **Canonical fixtures are provided via mock helper methods**:

| Scenario | Helper Method | Example |
|----------|---------------|---------|
| **Successful workflow** | `WithFullResponse()` | High confidence, workflow selected |
| **Human review required** | `WithHumanReviewReasonEnum(reason, warnings)` | All 7 enum values supported |
| **Investigation inconclusive** | `WithHumanReviewReasonEnum("investigation_inconclusive", ...)` | BR-HAPI-200 Outcome B |
| **Problem resolved** | `WithProblemResolved(confidence, warnings, analysis)` | BR-HAPI-200 Outcome A |
| **LLM self-correction** | `WithHumanReviewAndHistory(reason, warnings, attempts)` | DD-HAPI-002 v1.4 |
| **API errors** | `WithAPIError(statusCode, message)` | 4xx/5xx responses |

**Human review reason enum values** (all 7):
```go
// Supported values for human_review_reason
mockClient.WithHumanReviewReasonEnum("workflow_not_found", warnings)
mockClient.WithHumanReviewReasonEnum("image_mismatch", warnings)
mockClient.WithHumanReviewReasonEnum("parameter_validation_failed", warnings)
mockClient.WithHumanReviewReasonEnum("no_matching_workflows", warnings)
mockClient.WithHumanReviewReasonEnum("low_confidence", warnings)
mockClient.WithHumanReviewReasonEnum("llm_parsing_error", warnings)
mockClient.WithHumanReviewReasonEnum("investigation_inconclusive", warnings)
```

### Q3 Response (Contract Testing)

✅ **Contract is established via ADR-045 + OpenAPI spec**

| Resource | Location | Purpose |
|----------|----------|---------|
| **ADR-045** | `docs/architecture/decisions/ADR-045-aianalysis-holmesgpt-api-contract.md` | Authoritative contract definition |
| **OpenAPI Spec** | `holmesgpt-api/api/openapi.json` | 19 schemas, auto-generated from Pydantic |
| **Go Client Types** | `pkg/aianalysis/client/` | Client interface + types |

**Contract validation approach**:
1. HAPI exports OpenAPI spec via `make export-openapi-holmesgpt-api`
2. CI validates spec hasn't broken (`make validate-openapi-holmesgpt-api`)
3. AIAnalysis uses `client.IncidentRequest` / `client.IncidentResponse` types
4. Mock client implements same interface guaranteeing type compatibility

**No formal consumer contract tests** (like Pact) are implemented. The mock client + ADR-045 + OpenAPI spec provide sufficient contract validation for V1.0.

---

## ✅ Summary for AIAnalysis Team

| Question | Answer |
|----------|--------|
| Q1: Mock pattern | **Option B** - Use `testutil.MockHolmesGPTClient` |
| Q2: Test fixtures | **Helper methods** provide all scenarios |
| Q3: Contract testing | **ADR-045 + OpenAPI** - no Pact needed for V1.0 |

**Next Steps**:
1. Update `test/integration/aianalysis/holmesgpt_integration_test.go` to use `testutil.MockHolmesGPTClient`
2. Remove dependency on real HAPI server (`localhost:8088`)
3. Use helper methods for scenario fixtures

---

**END OF REQUEST**

