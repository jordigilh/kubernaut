# Quick Start: V1.0 Maturity Tests

**Purpose**: Minimal checklist to add required maturity tests to your service.
**For comprehensive details**: See [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](./V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)

> ⚠️ **WARNING**: This quick-start is for reference only. You MUST follow the full template for production readiness. Skipping the full template will result in PR rejection during code review.

---

## Service Type

Choose your service type:
- [ ] **CRD Controller** → Complete sections 1-5
- [ ] **Stateless HTTP** → Complete sections 1-3, skip 4-5

---

## 1. Metrics Integration Test (Required)

**File**: `test/integration/{service}/metrics_test.go`

```go
It("should record metrics after reconciliation", func() {
    // Trigger operation that records metrics
    // ...

    // Verify via registry inspection
    families, _ := ctrlmetrics.Registry.Gather()
    var found bool
    for _, family := range families {
        if family.GetName() == "{service}_reconciliations_total" {
            found = true
        }
    }
    Expect(found).To(BeTrue())
})
```

---

## 2. Metrics E2E Test (Required)

**File**: `test/e2e/{service}/suite_test.go` or `test/e2e/{service}/metrics_test.go`

```go
It("should expose metrics on /metrics endpoint", func() {
    resp, err := http.Get(metricsURL)
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode).To(Equal(200))

    body, _ := io.ReadAll(resp.Body)
    Expect(string(body)).To(ContainSubstring("{service}_"))
})
```

---

## 3. Audit Trace Test (Required if service has audit)

**File**: `test/integration/{service}/audit_test.go`

```go
It("should emit audit trace with correct fields", func() {
    // Trigger audit event
    // ...

    // Query via OpenAPI client
    events, _, err := auditClient.AuditAPI.QueryAuditEvents(ctx).
        Service("{service}").
        CorrelationId(string(resource.UID)).
        Execute()

    Expect(err).ToNot(HaveOccurred())
    Expect(len(events.Events)).To(BeNumerically(">", 0))

    event := events.Events[0]
    Expect(event.Service).To(Equal("{service}"))
    Expect(event.EventType).To(Equal("{expected_type}"))
})
```

---

## 4. EventRecorder E2E Test (CRD Controllers Only)

**File**: `test/e2e/{service}/events_test.go`

```go
It("should emit Kubernetes events", func() {
    // Create resource
    // Wait for processing

    var events corev1.EventList
    err := k8sClient.List(ctx, &events,
        client.InNamespace(namespace),
        client.MatchingFields{"involvedObject.name": resource.Name})

    Expect(err).ToNot(HaveOccurred())
    Expect(len(events.Items)).To(BeNumerically(">", 0))
})
```

---

## 5. Graceful Shutdown Test (CRD Controllers Only)

**File**: `test/unit/{service}/shutdown_test.go`

```go
It("should flush audit on shutdown", func() {
    mockStore := &mockAuditStore{}
    // Run main with mocks, simulate shutdown
    Expect(mockStore.closeCalled).To(BeTrue())
})
```

---

## Checklist Before PR

- [ ] Metrics integration test added
- [ ] Metrics E2E test added
- [ ] Audit trace test added (if applicable)
- [ ] EventRecorder test added (CRD controllers)
- [ ] Graceful shutdown test added (CRD controllers)
- [ ] All tests passing locally
- [ ] Reviewed full template for completeness

---

## Next Steps

1. Run `make test-unit-{service}` to verify unit tests
2. Run `make test-integration-{service}` to verify integration tests
3. Run `make test-e2e-{service}` to verify E2E tests
4. Run `./scripts/validate-service-maturity.sh` to verify compliance
5. Submit PR for review

---

## References

- [Full Test Plan Template](./V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md) - **REQUIRED READING**
- [TESTING_GUIDELINES.md](../business-requirements/TESTING_GUIDELINES.md)
- [DD-005: Observability Standards](../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)

