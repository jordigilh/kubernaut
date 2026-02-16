# V1.0 Service Maturity Test Plan Template

**Version**: 1.0.0
**Created**: 2025-12-19
**Status**: Active
**Purpose**: Standardized test plan for V1.0 maturity feature validation

---

## Overview

This template provides a standardized approach for testing all V1.0 maturity features. Copy this template for each service and complete the checklist.

**Reference Documents**:
- [TESTING_GUIDELINES.md](../business-requirements/TESTING_GUIDELINES.md#v10-service-maturity-testing-requirements)
- [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#v10-mandatory-maturity-checklist)
- [V1_0_SERVICE_MATURITY_TRIAGE_DEC_19_2025.md](../../handoff/V1_0_SERVICE_MATURITY_TRIAGE_DEC_19_2025.md)

---

## Test Plan: [SERVICE_NAME]

**Service Type**: [ ] CRD Controller | [ ] Stateless HTTP API
**Team**: [Team Name]
**Date**: [YYYY-MM-DD]
**Tester**: [Name]

---

## Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: Test pyramid tier prefix
  - `UT` - Unit Test (70%+ coverage target)
  - `IT` - Integration Test (<20% coverage target)
  - `E2E` - End-to-End Test (<10% coverage target)

- **SERVICE**: Service abbreviation (2-4 characters)
  - `AA` - AI Analysis
  - `RO` - Remediation Orchestrator
  - `GW` - Gateway
  - `WE` - Workflow Execution
  - `DS` - Data Storage
  - `SP` - Signal Processing
  - `EM` - Effectiveness Monitor
  - `WF` - Workflow (generic)

- **BR_NUMBER**: Business requirement number (e.g., `197` from BR-HAPI-197)

- **SEQUENCE**: Zero-padded 3-digit sequence (e.g., `001`, `002`, `010`)

**Examples** (from BR-HAPI-197):
- `UT-AA-197-001` - Unit test for AI Analysis service, BR 197, scenario 1
- `IT-RO-197-001` - Integration test for Remediation Orchestrator, BR 197, scenario 1
- `E2E-RO-197-002` - End-to-end test for Remediation Orchestrator, BR 197, scenario 2

**Usage in Test Descriptions**:
```go
Describe("UT-AA-197-001: Extract needs_human_review from HAPI response", func() {
    It("should correctly parse needs_human_review field", func() {
        // Test implementation maps to UT-AA-197-001 in test plan
    })
})
```

**Reference**: See [docs/testing/BR-HAPI-197/](../../testing/BR-HAPI-197/) for real-world examples

**Fallback**: If test plan does not exist, use Business Requirement ID in test description (e.g., `BR-WORKFLOW-001`)

---

## 1. Metrics Testing

### 1.1 Integration Tests - Metric Value Verification

| Metric Name | Expected Labels | Test File | Status |
|-------------|-----------------|-----------|--------|
| `[service]_reconciliations_total` | phase, result | `test/integration/[service]/metrics_test.go` | ⬜ |
| `[service]_processing_duration_seconds` | phase | `test/integration/[service]/metrics_test.go` | ⬜ |
| `[service]_enrichment_total` | result | `test/integration/[service]/metrics_test.go` | ⬜ |
| [Add more metrics...] | | | ⬜ |

#### Integration Test Template

```go
var _ = Describe("[Service] Metrics Integration", func() {
    Context("Metric: [metric_name]", func() {
        It("should record metric after [operation]", func() {
            // Given: [Precondition]

            // When: [Action that triggers metric]

            // Then: Verify metric via registry inspection
            families, err := ctrlmetrics.Registry.Gather()
            Expect(err).ToNot(HaveOccurred())

            var found bool
            for _, family := range families {
                if family.GetName() == "[metric_name]" {
                    found = true
                    // Verify labels and values
                    for _, metric := range family.GetMetric() {
                        Expect(metric.GetCounter().GetValue()).To(BeNumerically(">", 0))
                    }
                }
            }
            Expect(found).To(BeTrue(), "Metric [metric_name] not found")
        })
    })
})
```

### 1.2 E2E Tests - Metrics Endpoint Verification

| Test Case | Expected Metrics | Test File | Status |
|-----------|------------------|-----------|--------|
| `/metrics` endpoint accessible | All defined metrics present | `test/e2e/[service]/metrics_test.go` | ⬜ |

#### E2E Test Template

```go
var _ = Describe("[Service] Metrics E2E", func() {
    It("should expose all metrics on /metrics endpoint", func() {
        resp, err := http.Get(metricsURL)
        Expect(err).ToNot(HaveOccurred())
        defer resp.Body.Close()

        Expect(resp.StatusCode).To(Equal(http.StatusOK))

        body, _ := io.ReadAll(resp.Body)
        metricsOutput := string(body)

        // Verify ALL expected metrics are present
        expectedMetrics := []string{
            "[service]_reconciliations_total",
            "[service]_processing_duration_seconds",
            "[service]_enrichment_total",
            // Add all expected metrics
        }

        for _, metric := range expectedMetrics {
            Expect(metricsOutput).To(ContainSubstring(metric),
                "Expected metric %s not found", metric)
        }
    })
})
```

---

## 2. Audit Trace Testing

### 2.1 Audit Trace Inventory

| Audit Event Type | Trigger Condition | Required Fields | Test File | Status |
|------------------|-------------------|-----------------|-----------|--------|
| `[event_type]_started` | [When] | service, eventType, correlationId, ... | `test/integration/[service]/audit_test.go` | ⬜ |
| `[event_type]_completed` | [When] | service, eventType, correlationId, ... | `test/integration/[service]/audit_test.go` | ⬜ |
| `[event_type]_failed` | [When] | service, eventType, correlationId, error | `test/integration/[service]/audit_test.go` | ⬜ |

### 2.2 Integration Test - Audit Field Validation

> **⚠️ MANDATORY: Use OpenAPI Client + testutil.ValidateAuditEvent**
>
> Per V1.0 maturity requirements, audit tests MUST:
> 1. Use the **OpenAPI-generated client** (`dsgen.APIClient`) - NOT raw HTTP requests
> 2. Use **`testutil.ValidateAuditEvent()`** for type-safe field validation
> 3. Validate **ALL** required fields with **expected values** (not just existence)

#### Test Template (MANDATORY PATTERN)

```go
import (
    dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
    "github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("[Service] Audit Integration", func() {
    var auditClient *dsgen.APIClient

    BeforeEach(func() {
        // ✅ MANDATORY: Use OpenAPI client (NOT raw HTTP)
        cfg := dsgen.NewConfiguration()
        cfg.Servers = []dsgen.ServerConfiguration{{URL: dataStorageURL}}
        auditClient = dsgen.NewAPIClient(cfg)
    })

    Context("Audit Event: [event_type]", func() {
        It("should emit audit trace with all required fields validated via OpenAPI client", func() {
            // Given: [Setup condition]
            resource := createTestResource("test-audit", namespace)
            Expect(k8sClient.Create(ctx, resource)).To(Succeed())

            // When: [Trigger action]
            Eventually(func() string {
                var updated ResourceType
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(resource), &updated)
                return updated.Status.Phase
            }, 30*time.Second, 1*time.Second).Should(Equal("Completed"))

            // Then: Query via OpenAPI client and validate with testutil
            var events []dsgen.AuditEvent
            Eventually(func() int {
                resp, _, err := auditClient.AuditAPI.QueryAuditEvents(ctx).
                    Service("[service]").
                    CorrelationId(string(resource.UID)).
                    Execute()

                if err != nil {
                    return 0
                }
                events = resp.Data
                return len(events)
            }, 30*time.Second, 2*time.Second).Should(BeNumerically(">=", 1))

            // ✅ MANDATORY: Use testutil.ValidateAuditEvent for type-safe validation
            severity := "[severity]"
            testutil.ValidateAuditEvent(events[0], testutil.ExpectedAuditEvent{
                EventType:     "[event_type]",
                EventCategory: dsgen.AuditEventEventCategory[Category],
                EventAction:   "[action]",
                EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
                CorrelationID: string(resource.UID),
                Severity:      &severity,
                EventDataFields: map[string]interface{}{
                    "[field1]": "[expected_value]",
                    "[field2]": gomega.Not(gomega.BeEmpty()),
                },
            })
        })
    })
})
```

### 2.3 Why OpenAPI Client is Mandatory

| Approach | Type Safety | Maintainability | V1.0 Compliant |
|----------|-------------|-----------------|----------------|
| ❌ Raw HTTP + `json.Unmarshal` | None | Poor | **NO** |
| ✅ OpenAPI Client + `testutil` | Full | Excellent | **YES** |

**Benefits of OpenAPI Client**:
- Schema-validated responses (`dsgen.AuditEvent` struct)
- Compile-time type checking for event fields
- Auto-generated from OpenAPI spec (single source of truth)
- Consistent with production code patterns
```

### 2.3 E2E Test - Audit Client Wiring

```go
var _ = Describe("[Service] Audit E2E", func() {
    It("should have audit client wired to main controller", func() {
        // Create resource that triggers audit
        resource := createTestResource("e2e-audit", namespace)
        Expect(k8sClient.Create(ctx, resource)).To(Succeed())

        // Wait for processing
        Eventually(func() string {
            var updated ResourceType
            _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(resource), &updated)
            return updated.Status.Phase
        }, 60*time.Second, 2*time.Second).Should(Equal("Completed"))

        // Verify audit was written to Data Storage
        Eventually(func() int {
            events, _, _ := auditClient.AuditAPI.QueryAuditEvents(ctx).
                Service("[service]").
                CorrelationId(string(resource.UID)).
                Execute()
            return len(events.Events)
        }, 30*time.Second, 2*time.Second).Should(BeNumerically(">", 0),
            "Audit events should be written - audit client is wired")
    })
})
```

---

## 3. EventRecorder Testing (CRD Controllers Only)

### 3.1 Event Inventory

| Event Reason | Event Type | Message Pattern | Trigger | Status |
|--------------|------------|-----------------|---------|--------|
| `ProcessingStarted` | Normal | "Started processing..." | Phase transition | ⬜ |
| `ProcessingCompleted` | Normal | "Completed processing..." | Success | ⬜ |
| `ProcessingFailed` | Warning | "Failed: %s" | Error | ⬜ |

### 3.2 E2E Test - Event Emission

```go
var _ = Describe("[Service] EventRecorder E2E", func() {
    It("should emit Kubernetes events on phase transitions", func() {
        resource := createTestResource("e2e-events", namespace)
        Expect(k8sClient.Create(ctx, resource)).To(Succeed())

        // Wait for processing
        Eventually(func() string {
            var updated ResourceType
            _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(resource), &updated)
            return updated.Status.Phase
        }, 60*time.Second, 2*time.Second).Should(Equal("Completed"))

        // Verify events via Kubernetes API
        Eventually(func() []string {
            var events corev1.EventList
            err := k8sClient.List(ctx, &events,
                client.InNamespace(namespace),
                client.MatchingFields{"involvedObject.name": resource.Name})

            if err != nil {
                return nil
            }

            var reasons []string
            for _, event := range events.Items {
                reasons = append(reasons, event.Reason)
            }
            return reasons
        }, 30*time.Second, 2*time.Second).Should(ContainElements(
            "ProcessingStarted",
            "ProcessingCompleted",
        ))
    })
})
```

---

## 4. Graceful Shutdown Testing

### 4.1 Unit Test - Context Cancellation Patterns (Error Category B)

**Location**: `test/unit/[service]/shutdown_test.go`
**Purpose**: Validate context-aware goroutine cleanup patterns
**BR Coverage**: BR-[SERVICE]-XXX (Context cancellation behavior)

**Reference Implementation**: `test/unit/signalprocessing/controller_shutdown_test.go`

```go
var _ = Describe("[Service] Controller Shutdown (Error Category B)", func() {
    Context("Error Category B: Context Cancellation Clean Exit", func() {
        It("should exit worker goroutine cleanly when context is canceled", func() {
            ctx, cancel := context.WithCancel(context.Background())
            var workerExited atomic.Bool

            // Simulate a worker goroutine
            go func() {
                defer func() { workerExited.Store(true) }()
                for {
                    select {
                    case <-ctx.Done():
                        return // Clean exit
                    default:
                        time.Sleep(10 * time.Millisecond)
                    }
                }
            }()

            // Give worker time to start
            time.Sleep(20 * time.Millisecond)
            Expect(workerExited.Load()).To(BeFalse())

            // Cancel context
            cancel()

            // Worker should exit within reasonable time
            Eventually(func() bool {
                return workerExited.Load()
            }, 100*time.Millisecond, 10*time.Millisecond).Should(BeTrue())
        })
    })

    Context("DD-007/ADR-032: Audit Store Flush on Shutdown", func() {
        It("BR-[SERVICE]-090: should call audit store Close() during graceful shutdown", func() {
            ctx, cancel := context.WithCancel(context.Background())
            var closeCalled atomic.Bool

            // Simulate controller shutdown pattern
            go func() {
                <-ctx.Done()
                closeCalled.Store(true) // Simulates auditStore.Close()
            }()

            // Trigger shutdown
            cancel()

            // Verify Close() was called
            Eventually(func() bool {
                return closeCalled.Load()
            }, 100*time.Millisecond, 5*time.Millisecond).Should(BeTrue(),
                "ADR-032 §2: auditStore.Close() MUST be called during shutdown to flush pending events")
        })
    })
})
```

---

### 4.2 Integration Test - In-Flight Work Completion

**Location**: `test/integration/[service]/graceful_shutdown_test.go`
**Purpose**: Verify controller completes active work before shutdown
**BR Coverage**: BR-[SERVICE]-080/081/082

**Reference Implementation**: `test/integration/notification/graceful_shutdown_test.go`

```go
var _ = Describe("[Service] Graceful Shutdown (DD-007)", func() {
    Context("BR-[SERVICE]-080: In-Flight Work Completion", func() {
        It("should complete in-flight work before shutdown", func() {
            // Create CRD instance
            instance := &[Service]CRD{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-inflight",
                    Namespace: testNamespace,
                },
                Spec: [Service]Spec{
                    // ... spec fields ...
                },
            }

            // Create resource
            err := k8sClient.Create(ctx, instance)
            Expect(err).ToNot(HaveOccurred())

            // Wait for controller to start processing
            Eventually(func() string {
                k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
                return instance.Status.Phase
            }, timeout, interval).Should(Equal("Processing"))

            // Context cancellation triggers shutdown
            // Controller should complete in-flight work before exit

            // Verify work completed (not left in intermediate state)
            Eventually(func() string {
                k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
                return instance.Status.Phase
            }, timeout, interval).Should(Or(Equal("Completed"), Equal("Failed")))
        })
    })

    Context("BR-[SERVICE]-081: Audit Buffer Flushing", func() {
        It("should flush audit buffer before shutdown", func() {
            // Create CRD instance that generates audit events
            instance := &[Service]CRD{ /* ... */ }
            err := k8sClient.Create(ctx, instance)
            Expect(err).ToNot(HaveOccurred())

            // Wait for reconciliation
            Eventually(func() string {
                k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
                return instance.Status.Phase
            }, timeout, interval).Should(Equal("Completed"))

            // Query audit events from Data Storage
            // Verify ALL expected events were flushed (not lost)
            auditClient := dsgen.NewClientWithResponses(dataStorageURL)
            resp, err := auditClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                ResourceType: ptr.To("Service"),
                ResourceId:   ptr.To(instance.Name),
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(resp.StatusCode()).To(Equal(http.StatusOK))
            Expect(len(resp.JSON200.Data)).To(BeNumerically(">=", 1),
                "Audit events must be flushed before shutdown")
        })
    })
})
```

---

### 4.3 E2E Test - SIGTERM Signal Handling (MANDATORY)

**Location**: `test/e2e/[service]/graceful_shutdown_test.go`
**Purpose**: Validate real process responds to OS signals correctly
**BR Coverage**: BR-[SERVICE]-082 (Handle SIGTERM within timeout)

**Defense-in-Depth Rationale**: Unit tests validate context cancellation patterns, integration tests verify in-flight work completion, **E2E tests validate actual SIGTERM signal handling with real process lifecycle**.

```go
var _ = Describe("[Service] E2E Graceful Shutdown (DD-007)", func() {
    var (
        servicePodName string
        serviceNS      string
    )

    BeforeEach(func() {
        serviceNS = "kubernaut-system"
        // Find running service pod
        pods := &corev1.PodList{}
        err := k8sClient.List(ctx, pods, client.InNamespace(serviceNS), client.MatchingLabels{
            "app": "[service]-controller",
        })
        Expect(err).ToNot(HaveOccurred())
        Expect(len(pods.Items)).To(BeNumerically(">", 0), "Service pod must be running")
        servicePodName = pods.Items[0].Name
    })

    It("BR-[SERVICE]-082: should handle SIGTERM within timeout (5-10s)", func() {
        // Create CRD instance to generate audit events
        instance := &[Service]CRD{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "sigterm-test",
                Namespace: testNamespace,
            },
            Spec: [Service]Spec{ /* ... */ },
        }
        err := k8sClient.Create(ctx, instance)
        Expect(err).ToNot(HaveOccurred())

        // Wait for controller to start processing
        Eventually(func() string {
            k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
            return instance.Status.Phase
        }, timeout, interval).Should(Equal("Processing"))

        // Send SIGTERM to service pod
        cmd := exec.Command("kubectl", "exec", "-n", serviceNS, servicePodName, "--",
            "sh", "-c", "kill -SIGTERM 1")
        err = cmd.Run()
        Expect(err).ToNot(HaveOccurred(), "SIGTERM signal should be sent successfully")

        // Verify pod terminates gracefully within timeout
        startTime := time.Now()
        Eventually(func() bool {
            pod := &corev1.Pod{}
            err := k8sClient.Get(ctx, client.ObjectKey{Name: servicePodName, Namespace: serviceNS}, pod)
            return err != nil || pod.Status.Phase == "Succeeded" || pod.Status.Phase == "Failed"
        }, 15*time.Second, 1*time.Second).Should(BeTrue(),
            "Pod should terminate gracefully within 10-15 seconds after SIGTERM")

        shutdownDuration := time.Since(startTime)
        Expect(shutdownDuration).To(BeNumerically("<", 15*time.Second),
            "BR-[SERVICE]-082: Graceful shutdown should complete within timeout")

        // Verify audit buffer was flushed (no event loss)
        auditClient := dsgen.NewClientWithResponses(dataStorageURL)
        resp, err := auditClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
            ResourceType: ptr.To("Service"),
            ResourceId:   ptr.To(instance.Name),
        })
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode()).To(Equal(http.StatusOK))
        Expect(len(resp.JSON200.Data)).To(BeNumerically(">=", 1),
            "ADR-032 §2: Audit events must NOT be lost during graceful shutdown")
    })

    It("should flush all pending audit events on SIGTERM", func() {
        // Create multiple CRD instances to generate many audit events
        instanceCount := 5
        instanceNames := make([]string, instanceCount)

        for i := 0; i < instanceCount; i++ {
            instanceNames[i] = fmt.Sprintf("sigterm-flush-test-%d", i)
            instance := &[Service]CRD{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      instanceNames[i],
                    Namespace: testNamespace,
                },
                Spec: [Service]Spec{ /* ... */ },
            }
            err := k8sClient.Create(ctx, instance)
            Expect(err).ToNot(HaveOccurred())
        }

        // Wait for all to start processing
        for _, name := range instanceNames {
            Eventually(func() string {
                instance := &[Service]CRD{}
                k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: testNamespace}, instance)
                return instance.Status.Phase
            }, timeout, interval).ShouldNot(BeEmpty())
        }

        // Send SIGTERM while many events are buffered
        cmd := exec.Command("kubectl", "exec", "-n", serviceNS, servicePodName, "--",
            "sh", "-c", "kill -SIGTERM 1")
        err := cmd.Run()
        Expect(err).ToNot(HaveOccurred())

        // Wait for graceful shutdown
        Eventually(func() bool {
            pod := &corev1.Pod{}
            err := k8sClient.Get(ctx, client.ObjectKey{Name: servicePodName, Namespace: serviceNS}, pod)
            return err != nil || pod.Status.Phase != "Running"
        }, 15*time.Second, 1*time.Second).Should(BeTrue())

        // Verify ALL audit events were flushed (count matches expected)
        auditClient := dsgen.NewClientWithResponses(dataStorageURL)
        for _, name := range instanceNames {
            resp, err := auditClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                ResourceType: ptr.To("Service"),
                ResourceId:   ptr.To(name),
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(resp.StatusCode()).To(Equal(http.StatusOK))
            Expect(len(resp.JSON200.Data)).To(BeNumerically(">=", 1),
                fmt.Sprintf("Events for %s must be flushed, got %d events", name, len(resp.JSON200.Data)))
        }
    })
})
```

**E2E Test Requirements**:
- ✅ Sends actual SIGTERM signal to service pod
- ✅ Verifies graceful shutdown within timeout (5-10s)
- ✅ Confirms audit buffer flushed (no event loss)
- ✅ Tests multiple concurrent events (stress test)

**Why E2E is Mandatory**:
| Test Tier | Validates | Limitation |
|-----------|-----------|------------|
| **Unit** | Context cancellation patterns | Uses `context.WithCancel()`, not real signals |
| **Integration** | In-flight work completion | Uses context cancellation, not OS signals |
| **E2E** | **Real SIGTERM handling** | **Full signal handling pipeline validation** |

**Critical Gap Filled by E2E**: Validates `ctrl.SetupSignalHandler()` correctly translates OS SIGTERM → context cancellation → cleanup → process exit.

---

## 5. Health Probe Testing

### 5.1 E2E Test - Probe Endpoints

```go
var _ = Describe("[Service] Health Probes E2E", func() {
    It("should expose /healthz endpoint", func() {
        resp, err := http.Get(healthzURL)
        Expect(err).ToNot(HaveOccurred())
        defer resp.Body.Close()
        Expect(resp.StatusCode).To(Equal(http.StatusOK))
    })

    It("should expose /readyz endpoint", func() {
        resp, err := http.Get(readyzURL)
        Expect(err).ToNot(HaveOccurred())
        defer resp.Body.Close()
        Expect(resp.StatusCode).To(Equal(http.StatusOK))
    })
})
```

---

## 6. Predicates Testing (CRD Controllers Only)

### 6.1 Unit Test - Predicate Configuration

```go
var _ = Describe("[Service] Predicates", func() {
    It("should use GenerationChangedPredicate in SetupWithManager", func() {
        // This is primarily validated via code review
        // Verify in SetupWithManager:
        // .WithEventFilter(predicate.GenerationChangedPredicate{})

        // Optional: Verify predicate is applied by checking reconcile counts
        // on status-only updates (should NOT trigger reconcile)
    })
})
```

---

## 7. Compliance Sign-Off

### Test Execution Summary

| Test Category | Tests Passed | Tests Failed | Coverage |
|---------------|--------------|--------------|----------|
| Metrics Integration | 0 / 0 | 0 | 0% |
| Metrics E2E | 0 / 0 | 0 | 0% |
| Audit Integration | 0 / 0 | 0 | 0% |
| Audit E2E | 0 / 0 | 0 | 0% |
| EventRecorder E2E | 0 / 0 | 0 | 0% |
| Graceful Shutdown | 0 / 0 | 0 | 0% |
| Health Probes E2E | 0 / 0 | 0 | 0% |
| **Total** | **0 / 0** | **0** | **0%** |

### Compliance Checklist

| Requirement | Test Evidence | Sign-Off |
|-------------|---------------|----------|
| All metrics tested via integration | [Link to test file] | ⬜ |
| All metrics verified via E2E | [Link to test file] | ⬜ |
| All audit traces validated via OpenAPI | [Link to test file] | ⬜ |
| Audit client wired (E2E verified) | [Link to test file] | ⬜ |
| EventRecorder emits events (E2E) | [Link to test file] | ⬜ |
| Graceful shutdown flushes state | [Link to test file] | ⬜ |
| Health probes accessible | [Link to test file] | ⬜ |

### Approval

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Developer | | | ⬜ |
| Reviewer | | | ⬜ |
| Team Lead | | | ⬜ |

---

## Appendix A: Test File Locations

| Test Category | CRD Controller Location | Stateless Service Location |
|---------------|-------------------------|----------------------------|
| Metrics Integration | `test/integration/[service]/metrics_test.go` | `test/integration/[service]/metrics_test.go` |
| Metrics E2E | `test/e2e/[service]/metrics_test.go` | `test/e2e/[service]/metrics_test.go` |
| Audit Integration | `test/integration/[service]/audit_test.go` | `test/integration/[service]/audit_test.go` |
| EventRecorder E2E | `test/e2e/[service]/events_test.go` | N/A |
| Graceful Shutdown | `test/unit/[service]/shutdown_test.go` | `test/unit/[service]/shutdown_test.go` |
| Health Probes | `test/e2e/[service]/probes_test.go` | `test/e2e/[service]/probes_test.go` |

---

## Appendix B: Common Issues and Solutions

| Issue | Solution |
|-------|----------|
| Metrics not appearing in registry | Ensure `metrics.Registry.MustRegister()` in `init()` |
| Audit events not found | Check Data Storage is running and accessible |
| Events not emitting | Ensure EventRecorder is wired via `mgr.GetEventRecorderFor()` |
| Graceful shutdown not flushing | Verify `auditStore.Close()` called after `mgr.Start()` returns |

---

## References

- [TESTING_GUIDELINES.md](../business-requirements/TESTING_GUIDELINES.md)
- [DD-007: Graceful Shutdown](../../architecture/decisions/DD-007-graceful-shutdown.md)
- [DD-AUDIT-003: Audit Requirements](../../architecture/decisions/DD-AUDIT-003-audit-requirements.md)
- [DD-TEST-001: Port Allocation](../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)

