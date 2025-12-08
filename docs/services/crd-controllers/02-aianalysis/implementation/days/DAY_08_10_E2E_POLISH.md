# Days 8-10: E2E Testing & Production Polish

**Part of**: AI Analysis Implementation Plan V1.0
**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md)
**Duration**: 18-24 hours (3 days)
**Target Confidence**: 95%+ (Production Ready)

---

## 🎯 Day 8 Objectives: E2E Testing

| Objective | Priority | BR Reference |
|-----------|----------|--------------|
| Health endpoint E2E tests | P0 | BR-AI-025 |
| Metrics endpoint E2E tests | P0 | BR-AI-022 |
| Full user journey E2E test | P0 | BR-AI-001 |
| NodePort service deployment | P0 | DD-TEST-001 |

---

## 🔧 Day 8: E2E Test Implementation

### E2E Test Suite Setup

```go
// test/e2e/aianalysis/suite_test.go
package aianalysis

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestAIAnalysisE2E(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "AIAnalysis E2E Test Suite")
}

var _ = BeforeSuite(func() {
    // Verify KIND cluster is running with correct port mappings
    By("Verifying KIND cluster")

    // Check health endpoint is accessible via NodePort
    resp, err := http.Get("http://localhost:8184/healthz")
    Expect(err).NotTo(HaveOccurred())
    defer resp.Body.Close()
    Expect(resp.StatusCode).To(Equal(http.StatusOK))
})
```

### Health Endpoint E2E Tests

```go
// test/e2e/aianalysis/health_endpoints_test.go
package aianalysis

import (
    "io"
    "net/http"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

const (
    // Per DD-TEST-001 port allocation
    HealthURL  = "http://localhost:8184/healthz"
    ReadyURL   = "http://localhost:8184/readyz"
    MetricsURL = "http://localhost:9184/metrics"
)

var _ = Describe("Health Endpoints E2E", func() {
    Context("Liveness probe (/healthz)", func() {
        It("should return 200 OK - BR-AI-025", func() {
            resp, err := http.Get(HealthURL)
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()

            Expect(resp.StatusCode).To(Equal(http.StatusOK))

            body, _ := io.ReadAll(resp.Body)
            Expect(string(body)).To(ContainSubstring("ok"))
        })
    })

    Context("Readiness probe (/readyz)", func() {
        It("should return 200 OK when ready - BR-AI-025", func() {
            resp, err := http.Get(ReadyURL)
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()

            Expect(resp.StatusCode).To(Equal(http.StatusOK))
        })

        It("should include dependency checks", func() {
            resp, err := http.Get(ReadyURL + "?verbose=true")
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()

            body, _ := io.ReadAll(resp.Body)
            // Should show dependency health
            Expect(string(body)).To(ContainSubstring("holmesgpt"))
        })
    })
})
```

### Metrics Endpoint E2E Tests

```go
// test/e2e/aianalysis/metrics_test.go
package aianalysis

import (
    "io"
    "net/http"
    "strings"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Metrics Endpoint E2E", func() {
    Context("Prometheus metrics (/metrics)", func() {
        It("should expose metrics in Prometheus format - BR-AI-022", func() {
            resp, err := http.Get(MetricsURL)
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()

            Expect(resp.StatusCode).To(Equal(http.StatusOK))
            Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/plain"))
        })

        It("should include reconciliation metrics", func() {
            resp, err := http.Get(MetricsURL)
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()

            body, _ := io.ReadAll(resp.Body)
            metricsText := string(body)

            // Verify expected metrics exist
            expectedMetrics := []string{
                "aianalysis_reconciliations_total",
                "aianalysis_phase_duration_seconds",
                "aianalysis_holmesgpt_api_calls_total",
                "aianalysis_errors_total",
            }

            for _, metric := range expectedMetrics {
                Expect(metricsText).To(ContainSubstring(metric),
                    "Missing metric: %s", metric)
            }
        })

        It("should include Go runtime metrics", func() {
            resp, err := http.Get(MetricsURL)
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()

            body, _ := io.ReadAll(resp.Body)

            // Standard Go metrics
            Expect(string(body)).To(ContainSubstring("go_goroutines"))
            Expect(string(body)).To(ContainSubstring("go_memstats"))
        })
    })
})
```

### Full User Journey E2E Test

```go
// test/e2e/aianalysis/full_flow_test.go
package aianalysis

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Full User Journey E2E", func() {
    const (
        timeout  = 3 * time.Minute
        interval = 2 * time.Second
    )

    Context("Production incident analysis", func() {
        var analysis *aianalysisv1.AIAnalysis

        BeforeEach(func() {
            analysis = &aianalysisv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "e2e-prod-incident-" + randomSuffix(),
                    Namespace: "kubernaut-system",
                },
                Spec: aianalysisv1.AIAnalysisSpec{
                    SignalContext: aianalysisv1.SignalContextInput{
                        Environment:      "production",
                        BusinessPriority: "P1",
                        TargetResource: aianalysisv1.TargetResource{
                            Kind:      "Deployment",
                            Name:      "payment-service",
                            Namespace: "payments",
                        },
                        EnrichmentResults: &sharedtypes.EnrichmentResults{
                            DetectedLabels: &sharedtypes.DetectedLabels{
                                GitOpsManaged:   true,
                                GitOpsTool:      "argocd",
                                PDBProtected:    true,
                                HPAEnabled:      true,
                                NetworkIsolated: true,
                                ServiceMesh:     "istio",
                            },
                            OwnerChain: []sharedtypes.OwnerChainEntry{
                                {Namespace: "payments", Kind: "Deployment", Name: "payment-service"},
                            },
                            CustomLabels: map[string][]string{
                                "team":        {"payments"},
                                "cost_center": {"revenue"},
                            },
                        },
                    },
                },
            }
        })

        AfterEach(func() {
            // Cleanup
            _ = k8sClient.Delete(ctx, analysis)
        })

        It("should complete full E2E flow - BR-AI-001", func() {
            ctx := context.Background()

            By("Creating AIAnalysis for production incident")
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            By("Verifying phase transitions")
            // Per reconciliation-phases.md v2.0: 4-phase flow
            // Validating and Recommending phases removed in v1.4 and v1.8
            phases := []string{
                "Pending",
                "Investigating",
                "Analyzing",
                "Completed",
            }

            for _, phase := range phases {
                By("Waiting for phase: " + phase)
                Eventually(func() string {
                    _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
                    return string(analysis.Status.Phase)
                }, timeout, interval).Should(Equal(phase))
            }

            By("Verifying final status")
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

            // Production should require approval
            Expect(analysis.Status.ApprovalRequired).To(BeTrue())

            // Should have workflow selected
            Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())

            // Should have completion timestamp
            Expect(analysis.Status.CompletedAt).NotTo(BeZero())

            // Should capture targetInOwnerChain
            Expect(analysis.Status.TargetInOwnerChain).NotTo(BeNil())
        })

        It("should handle data quality warnings gracefully", func() {
            ctx := context.Background()

            // Add failed detections
            analysis.Spec.SignalContext.EnrichmentResults.DetectedLabels.FailedDetections = []string{"gitOpsManaged"}

            By("Creating AIAnalysis with detection failures")
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            By("Waiting for completion")
            Eventually(func() string {
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Completed"))

            By("Verifying approval required due to data quality")
            Expect(analysis.Status.ApprovalRequired).To(BeTrue())
            Expect(analysis.Status.ApprovalReason).To(ContainSubstring("detection"))
        })
    })
})
```

---

## 🎯 Day 9 Objectives: Production Polish

| Objective | Priority | BR Reference |
|-----------|----------|--------------|
| Documentation finalization | P0 | — |
| Code review and cleanup | P0 | — |
| Performance validation | P1 | BR-AI-024 |
| Security review | P1 | BR-AI-025 |

---

## 🔧 Day 9: Production Polish

### Performance Validation

```bash
# Benchmark reconciliation performance
go test -bench=. -benchmem ./pkg/aianalysis/...

# Profile CPU usage
go test -cpuprofile=cpu.prof -bench=. ./pkg/aianalysis/...
go tool pprof cpu.prof

# Profile memory usage
go test -memprofile=mem.prof -bench=. ./pkg/aianalysis/...
go tool pprof mem.prof
```

### Security Checklist

- [ ] RBAC permissions minimized
- [ ] Secrets not logged
- [ ] API keys not exposed in metrics
- [ ] Input validation complete
- [ ] Error messages don't leak sensitive info

### Code Quality Verification

```bash
# Full lint check
golangci-lint run ./pkg/aianalysis/... --enable-all

# Security scan
gosec ./pkg/aianalysis/...

# Dependency audit
go list -m all | nancy sleuth
```

### Documentation Verification

```bash
# Check for TODO/FIXME markers
grep -r "TODO\|FIXME\|XXX" pkg/aianalysis/ docs/services/crd-controllers/02-aianalysis/

# Check for dead links
find docs/services/crd-controllers/02-aianalysis/ -name "*.md" -exec grep -l "\[.*\](.*)" {} \; | while read f; do
    grep -oP '\[.*?\]\(\K[^)]+' "$f" | while read link; do
        if [[ "$link" != http* ]] && [[ ! -f "$(dirname $f)/$link" ]]; then
            echo "Dead link in $f: $link"
        fi
    done
done
```

---

## 🎯 Day 10 Objectives: Final Validation

| Objective | Priority | BR Reference |
|-----------|----------|--------------|
| Final test run | P0 | — |
| Production readiness checklist | P0 | — |
| Handoff documentation | P0 | — |
| Confidence assessment | P0 | — |

---

## 🔧 Day 10: Final Validation

### Production Readiness Checklist

```markdown
## AIAnalysis Service Production Readiness

### Code Quality
- [ ] Zero lint errors
- [ ] Zero security vulnerabilities
- [ ] All tests passing
- [ ] Coverage >= 70%

### Deployment
- [ ] CRD manifests generated
- [ ] Controller deployment YAML complete
- [ ] RBAC configured
- [ ] ConfigMaps for Rego policies

### Observability
- [ ] Health endpoints working (/healthz, /readyz)
- [ ] Metrics exposed (/metrics)
- [ ] Structured logging enabled
- [ ] All specified metrics implemented

### Integration
- [ ] HolmesGPT-API integration tested
- [ ] Data Storage audit integration tested
- [ ] Rego policy evaluation tested
- [ ] Full reconciliation loop tested

### Documentation
- [ ] All spec documents up to date
- [ ] Implementation checklist complete
- [ ] No dead links
- [ ] EOD documents created
```

### Final Test Execution

```bash
# Full test suite
make test

# Or individually:
go test -v ./test/unit/aianalysis/...
go test -v -tags=integration ./test/integration/aianalysis/...
go test -v -tags=e2e ./test/e2e/aianalysis/...

# Coverage report
go test -coverprofile=coverage.out ./pkg/aianalysis/...
go tool cover -html=coverage.out -o coverage.html
echo "Coverage: $(go tool cover -func=coverage.out | grep total | awk '{print $3}')"
```

### Final Confidence Assessment

```markdown
# AIAnalysis Final Confidence Assessment

**Date**: YYYY-MM-DD
**Version**: V1.0
**Overall Confidence**: 95%

## Component Scores

| Component | Score | Weight | Contribution |
|-----------|-------|--------|--------------|
| Implementation Accuracy | 95% | 30% | 28.5 |
| Test Coverage | 92% | 25% | 23.0 |
| BR Coverage | 94% | 20% | 18.8 |
| Production Readiness | 93% | 15% | 14.0 |
| Documentation Quality | 95% | 10% | 9.5 |
| **Total** | — | — | **93.8%** |

## Summary
- ✅ All 31 V1.0 BRs implemented
- ✅ All 4 phase handlers complete
- ✅ HolmesGPT-API integration working
- ✅ Rego policy evaluation working
- ✅ Error handling (5 categories) complete
- ✅ Metrics and observability complete
- ✅ E2E tests passing

## Known Limitations
- Circuit breaker timeout tuning may need production data
- Rego policy complexity limited to V1.0 scope

## Recommendations
- Monitor HolmesGPT-API latency in production
- Consider caching for repeated workflow queries
```

---

## ⭐ Day 10 Final Checkpoint

### Create Final EOD Document

**File**: `docs/services/crd-controllers/02-aianalysis/implementation/phase0/04-implementation-complete.md`

```markdown
# AIAnalysis Implementation Complete

**Date**: YYYY-MM-DD
**Final Confidence**: 95%
**Status**: ✅ Production Ready

## Summary
AIAnalysis V1.0 implementation complete with all business requirements met.

## Deliverables
- [x] pkg/aianalysis/ - Core implementation
- [x] test/unit/aianalysis/ - Unit tests (XX passing)
- [x] test/integration/aianalysis/ - Integration tests (XX passing)
- [x] test/e2e/aianalysis/ - E2E tests (XX passing)
- [x] config/crd/bases/aianalysis*.yaml - CRD manifest
- [x] docs/services/crd-controllers/02-aianalysis/ - Documentation

## Test Results
- Unit tests: XX passing, 0 failing
- Integration tests: XX passing, 0 failing
- E2E tests: XX passing, 0 failing
- Coverage: XX%

## BR Coverage
- 31/31 V1.0 BRs implemented (100%)
- See BR_MAPPING.md for details

## Handoff Notes
1. Rego policies in config/rego/aianalysis/
2. HolmesGPT-API required at startup
3. Data Storage **REQUIRED** for audit events (per DD-AUDIT-003, TESTING_GUIDELINES.md)
4. NodePort 30084 for E2E tests

## Next Steps
- Deploy to staging environment
- Monitor performance metrics
- Gather production feedback for V1.1
```

---

## 📚 Related Documents

- [DAY_05_07_INTEGRATION_TESTING.md](./DAY_05_07_INTEGRATION_TESTING.md) - Previous phase
- [APPENDIX_C_CONFIDENCE_METHODOLOGY.md](../appendices/APPENDIX_C_CONFIDENCE_METHODOLOGY.md) - Confidence calculation
- [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md) - Main plan

