# REQUEST: HAPI Team Assistance for Integration Test Mock Conversion

| **Field** | **Value** |
|-----------|-----------|
| **From** | AIAnalysis Team |
| **To** | HolmesGPT-API (HAPI) Team |
| **Date** | 2025-12-09 |
| **Priority** | 🟡 Medium |
| **Type** | Technical Assistance Request |
| **Status** | ⏳ Awaiting HAPI Response |

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

| Task | Owner | ETA |
|------|-------|-----|
| HAPI team responds with recommendation | HAPI | Dec 10, 2025 |
| AIAnalysis implements conversion | AIAnalysis | Dec 11, 2025 |
| Verify integration tests pass | AIAnalysis | Dec 11, 2025 |

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

---

## HAPI Team Response

**Date**: _Pending_
**Responder**: _Pending_

### Q1 Response (Mock Pattern)


### Q2 Response (Test Fixtures)


### Q3 Response (Contract Testing)


---

**END OF REQUEST**

