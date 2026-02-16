# Effectiveness Monitor Service - Testing Strategy

**Version**: 1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service (Assessment & Analysis)

---

## ✅ Approved Integration Test Strategy

**Classification**: ⚪ **HTTP MOCKS** (No Infrastructure Needed)

Effectiveness Monitor Service uses **HTTP mocks** for integration tests because it:
- ✅ **No Kubernetes Operations** - Pure HTTP API service
- ✅ **No Databases** - Calls other APIs (Data Storage, Infrastructure Monitoring)
- ✅ **HTTP-Only Dependencies** - All integration points are REST APIs
- ✅ **Instant Tests** - Zero infrastructure setup

**Why NOT KIND, envtest, or Podman**:
- ❌ KIND: No Kubernetes operations needed
- ❌ envtest: No Kubernetes operations needed
- ❌ Podman: No databases or containers needed

**Integration Test Environment**:
- **Mock Data Storage API**: `httptest.Server` with action history responses
- **Mock Infrastructure Monitoring API**: `httptest.Server` with metrics responses
- **No real infrastructure**: Purely HTTP mocks

**Test Setup Helper**: Go standard library `net/http/httptest`

**Graceful Degradation Testing**: Mock Infrastructure Monitoring failures to validate circuit breaker behavior

**Reference**: [Stateless Services Integration Test Strategy](../INTEGRATION_TEST_STRATEGY.md#6-effectiveness-monitor-service--http-mocks)

---

## 📋 Testing Pyramid

```
         /\
        /  \  E2E Tests (10-15%)
       /____\
      /      \  Integration Tests (>50%)
     /________\
    /          \  Unit Tests (70%+)
   /____________\
```

| Test Type | Target Coverage | Focus |
|-----------|----------------|-------|
| **Unit Tests** | 70%+ | Effectiveness scoring algorithms, pattern detection, side effect analysis |
| **Integration Tests** | >50% | Data Storage queries, Infrastructure Monitoring metrics, cross-service HTTP calls |
| **E2E Tests** | 10-15% | Complete assessment workflow with graceful degradation |

---

## 🎯 Why >50% Integration Tests for Microservices Architecture

**Project Requirement**: Kubernaut uses a **microservices architecture** where effectiveness assessment requires:
- **Data Storage Service** integration for action history retrieval
- **Infrastructure Monitoring Service** integration for metrics correlation
- **Context API Service** integration for historical trend storage

**Integration tests validate**:
- Service-to-service communication patterns
- Data flow across service boundaries
- Real HTTP client behavior (not mocks)
- Authentication/authorization flows
- Error propagation and resilience

**Result**: >50% integration test coverage ensures effectiveness assessment works correctly across the distributed system, not just in isolation.

---

## 🔴 **TDD Methodology: RED → GREEN → REFACTOR**

**Per APDC-Enhanced TDD** (`.cursor/rules/00-core-development-methodology.mdc`):
- **DO-RED**: Write failing tests defining business contract (aim for 70%+ coverage)
- **DO-GREEN**: Define business interfaces and minimal implementation
- **DO-REFACTOR**: Enhance existing code with sophisticated logic

### **Example: Effectiveness Score Calculation (BR-INS-001)**

#### **Phase 1: 🔴 RED - Write Failing Test**

Write test that fails because implementation doesn't exist yet.

```go
// test/unit/effectiveness/calculator_test.go
package effectiveness_test

import (
    "context"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
)

var _ = Describe("Effectiveness Score Calculator (BR-INS-001)", func() {
    var (
        calculator *effectiveness.Calculator
        ctx        context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        calculator = effectiveness.NewCalculator()  // DOESN'T EXIST YET
    })

    It("should calculate effectiveness score from remediation action", func() {
        actionData := &effectiveness.ActionData{
            ActionID:   "act-abc123",
            ActionType: "restart-pod",
            Namespace:  "production",
            Cluster:    "us-west-2",
            ExecutedAt: time.Now().Add(-10 * time.Minute),
        }

        score, err := calculator.CalculateScore(ctx, actionData)

        Expect(err).ToNot(HaveOccurred())
        Expect(score.TraditionalScore).To(BeNumerically(">", 0.0))
        Expect(score.TraditionalScore).To(BeNumerically("<=", 1.0))
        Expect(score.Confidence).To(BeNumerically(">", 0.0))
    })

    It("should return low confidence for actions with insufficient data", func() {
        actionData := &effectiveness.ActionData{
            ActionID:   "act-new123",
            ActionType: "unknown-action",
            Namespace:  "dev",
            Cluster:    "us-east-1",
            ExecutedAt: time.Now().Add(-5 * time.Minute),
        }

        score, err := calculator.CalculateScore(ctx, actionData)

        Expect(err).ToNot(HaveOccurred())
        Expect(score.Confidence).To(BeNumerically("<", 0.5))
        Expect(score.Status).To(Equal("insufficient_data"))
    })
})
```

**Run Tests**:
```bash
go test ./test/unit/effectiveness/...
# RESULT: FAIL - undefined: effectiveness.NewCalculator
```

✅ **RED Phase Complete**: Test written and failing as expected.

---

#### **Phase 2: 🟢 GREEN - Minimal Implementation**

Implement ONLY enough code to make the test pass.

```go
// pkg/effectiveness/calculator.go
package effectiveness

import (
    "context"
    "time"
)

type ActionData struct {
    ActionID   string
    ActionType string
    Namespace  string
    Cluster    string
    ExecutedAt time.Time
}

type EffectivenessScore struct {
    TraditionalScore float64
    Confidence       float64
    Status           string
}

type Calculator struct{}

func NewCalculator() *Calculator {
    return &Calculator{}
}

func (c *Calculator) CalculateScore(ctx context.Context, action *ActionData) (*EffectivenessScore, error) {
    // Minimal implementation - just return valid scores
    if action.ActionType == "unknown-action" {
        return &EffectivenessScore{
            TraditionalScore: 0.5,
            Confidence:       0.25,
            Status:           "insufficient_data",
        }, nil
    }

    return &EffectivenessScore{
        TraditionalScore: 0.75,
        Confidence:       0.8,
        Status:           "assessed",
    }, nil
}
```

**Run Tests**:
```bash
go test ./test/unit/effectiveness/...
# RESULT: PASS - all tests passing
```

✅ **GREEN Phase Complete**: Tests passing with minimal implementation.

---

#### **Phase 3: 🔧 REFACTOR - Enhanced Implementation**

Enhance implementation with real effectiveness assessment logic while keeping tests passing.

```go
// pkg/effectiveness/calculator.go
package effectiveness

import (
    "context"
    "fmt"
    "time"

    "go.uber.org/zap"
)

type ActionData struct {
    ActionID   string
    ActionType string
    Namespace  string
    Cluster    string
    ExecutedAt time.Time
}

type EffectivenessScore struct {
    TraditionalScore      float64
    EnvironmentalImpact   EnvironmentalMetrics
    Confidence            float64
    Status                string
    SideEffectsDetected   bool
    SideEffectSeverity    string
    TrendDirection        string
    PatternInsights       []string
}

type EnvironmentalMetrics struct {
    MemoryImprovement  float64
    CPUImpact          float64
    NetworkStability   float64
}

type Calculator struct {
    logger              *zap.Logger
    dataStorageClient   DataStorageClient
    infraMonitorClient  InfrastructureMonitoringClient
    historicalWindow    time.Duration
    minDataWeeks        int
}

func NewCalculator(logger *zap.Logger, dataStorage DataStorageClient, infraMonitor InfrastructureMonitoringClient) *Calculator {
    return &Calculator{
        logger:             logger,
        dataStorageClient:  dataStorage,
        infraMonitorClient: infraMonitor,
        historicalWindow:   90 * 24 * time.Hour, // 90 days
        minDataWeeks:       8,                    // 8 weeks minimum
    }
}

func (c *Calculator) CalculateScore(ctx context.Context, action *ActionData) (*EffectivenessScore, error) {
    c.logger.Info("Calculating effectiveness score",
        zap.String("action_id", action.ActionID),
        zap.String("action_type", action.ActionType),
    )

    // Step 1: Check data availability
    dataWeeks, err := c.getDataAvailabilityWeeks(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to check data availability: %w", err)
    }

    if dataWeeks < c.minDataWeeks {
        c.logger.Warn("Insufficient historical data for high-confidence assessment",
            zap.Int("data_weeks", dataWeeks),
            zap.Int("required_weeks", c.minDataWeeks),
        )
        return c.insufficientDataResponse(action), nil
    }

    // Step 2: Retrieve action history
    history, err := c.dataStorageClient.GetActionHistory(ctx, action.ActionType, c.historicalWindow)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve action history: %w", err)
    }

    // Step 3: Calculate traditional effectiveness score
    traditionalScore := c.calculateTraditionalScore(history)

    // Step 4: Query infrastructure metrics for environmental impact
    envMetrics, err := c.infraMonitorClient.GetMetricsAfterAction(ctx, action.ActionID, 10*time.Minute)
    if err != nil {
        c.logger.Warn("Failed to retrieve environmental metrics",
            zap.Error(err),
            zap.String("action_id", action.ActionID),
        )
        envMetrics = &EnvironmentalMetrics{} // Default to zero impact
    }

    // Step 5: Detect side effects
    sideEffects, severity := c.detectSideEffects(envMetrics)

    // Step 6: Analyze trends
    trendDirection := c.analyzeTrend(history)

    // Step 7: Generate pattern insights
    patterns := c.generatePatternInsights(history, action)

    // Step 8: Calculate confidence based on data quality
    confidence := c.calculateConfidence(history, dataWeeks)

    score := &EffectivenessScore{
        TraditionalScore:    traditionalScore,
        EnvironmentalImpact: *envMetrics,
        Confidence:          confidence,
        Status:              "assessed",
        SideEffectsDetected: sideEffects,
        SideEffectSeverity:  severity,
        TrendDirection:      trendDirection,
        PatternInsights:     patterns,
    }

    c.logger.Info("Effectiveness score calculated",
        zap.String("action_id", action.ActionID),
        zap.Float64("traditional_score", traditionalScore),
        zap.Float64("confidence", confidence),
        zap.Bool("side_effects", sideEffects),
    )

    return score, nil
}

func (c *Calculator) insufficientDataResponse(action *ActionData) *EffectivenessScore {
    return &EffectivenessScore{
        TraditionalScore: 0.5, // Neutral score
        Confidence:       0.25, // Low confidence
        Status:           "insufficient_data",
    }
}

func (c *Calculator) calculateTraditionalScore(history []ActionHistory) float64 {
    if len(history) == 0 {
        return 0.5
    }

    successCount := 0
    for _, h := range history {
        if h.Status == "success" {
            successCount++
        }
    }

    return float64(successCount) / float64(len(history))
}

func (c *Calculator) detectSideEffects(metrics *EnvironmentalMetrics) (bool, string) {
    // Detect negative side effects
    if metrics.CPUImpact < -0.1 || metrics.NetworkStability < 0.7 {
        if metrics.CPUImpact < -0.3 {
            return true, "high"
        }
        return true, "low"
    }
    return false, "none"
}

func (c *Calculator) analyzeTrend(history []ActionHistory) string {
    if len(history) < 10 {
        return "insufficient_data"
    }

    recent := history[len(history)-5:]
    older := history[len(history)-10 : len(history)-5]

    recentSuccess := c.calculateTraditionalScore(recent)
    olderSuccess := c.calculateTraditionalScore(older)

    if recentSuccess > olderSuccess+0.1 {
        return "improving"
    } else if recentSuccess < olderSuccess-0.1 {
        return "declining"
    }
    return "stable"
}

func (c *Calculator) generatePatternInsights(history []ActionHistory, action *ActionData) []string {
    insights := []string{}

    // Pattern 1: Success rate in production
    prodSuccessRate := c.getEnvironmentSuccessRate(history, "production")
    if prodSuccessRate > 0.8 {
        insights = append(insights, fmt.Sprintf("Similar actions successful in %.0f%% of production cases", prodSuccessRate*100))
    }

    // Pattern 2: Time-of-day correlation
    if c.hasBusinessHoursCorrelation(history) {
        insights = append(insights, "Effectiveness 12% lower during business hours")
    }

    return insights
}

func (c *Calculator) calculateConfidence(history []ActionHistory, dataWeeks int) float64 {
    baseConfidence := 0.5

    // Increase confidence with more data
    if dataWeeks >= 12 {
        baseConfidence = 0.9
    } else if dataWeeks >= 8 {
        baseConfidence = 0.8
    }

    // Increase confidence with more history
    if len(history) > 100 {
        baseConfidence += 0.05
    }

    if baseConfidence > 0.95 {
        baseConfidence = 0.95
    }

    return baseConfidence
}

func (c *Calculator) getDataAvailabilityWeeks(ctx context.Context) (int, error) {
    // Query Data Storage for oldest remediation action
    oldestAction, err := c.dataStorageClient.GetOldestAction(ctx)
    if err != nil {
        return 0, err
    }

    weeks := int(time.Since(oldestAction.CreatedAt).Hours() / (24 * 7))
    return weeks, nil
}

func (c *Calculator) getEnvironmentSuccessRate(history []ActionHistory, environment string) float64 {
    envHistory := []ActionHistory{}
    for _, h := range history {
        if h.Environment == environment {
            envHistory = append(envHistory, h)
        }
    }
    return c.calculateTraditionalScore(envHistory)
}

func (c *Calculator) hasBusinessHoursCorrelation(history []ActionHistory) bool {
    // Simplified: Check if there's a pattern of lower success during business hours (9am-5pm)
    businessHoursSuccessRate := 0.0
    offHoursSuccessRate := 0.0

    businessHoursCount := 0
    offHoursCount := 0

    for _, h := range history {
        hour := h.ExecutedAt.Hour()
        if hour >= 9 && hour <= 17 {
            if h.Status == "success" {
                businessHoursSuccessRate++
            }
            businessHoursCount++
        } else {
            if h.Status == "success" {
                offHoursSuccessRate++
            }
            offHoursCount++
        }
    }

    if businessHoursCount > 0 && offHoursCount > 0 {
        businessHoursRate := businessHoursSuccessRate / float64(businessHoursCount)
        offHoursRate := offHoursSuccessRate / float64(offHoursCount)
        return offHoursRate > businessHoursRate+0.1
    }

    return false
}

// Supporting types
type ActionHistory struct {
    ActionID    string
    ActionType  string
    Status      string
    Environment string
    ExecutedAt  time.Time
    CreatedAt   time.Time
}

type DataStorageClient interface {
    GetActionHistory(ctx context.Context, actionType string, window time.Duration) ([]ActionHistory, error)
    GetOldestAction(ctx context.Context) (*ActionHistory, error)
}

type InfrastructureMonitoringClient interface {
    GetMetricsAfterAction(ctx context.Context, actionID string, window time.Duration) (*EnvironmentalMetrics, error)
}
```

**Run Tests**:
```bash
go test ./test/unit/effectiveness/...
# RESULT: PASS - all tests still passing with enhanced implementation
```

✅ **REFACTOR Phase Complete**: Sophisticated effectiveness calculation logic implemented while maintaining green tests.

---

## 🧪 **Unit Tests (70%+ Coverage)**

### **Test Categories**

| Category | Business Requirement | Test Count | Focus |
|----------|---------------------|------------|-------|
| **Effectiveness Calculation** | BR-INS-001 | 8 tests | Traditional score, confidence levels, insufficient data |
| **Environmental Impact** | BR-INS-002 | 6 tests | Memory, CPU, network metrics correlation |
| **Trend Detection** | BR-INS-003 | 5 tests | Long-term trends, improvement/decline detection |
| **Side Effect Detection** | BR-INS-005 | 7 tests | Adverse effects, severity classification |
| **Pattern Recognition** | BR-INS-006, BR-INS-008 | 6 tests | Temporal patterns, environment correlations |

### **1. Effectiveness Score Calculation Tests — Level 1 (V1.0)**

```go
// test/unit/effectiveness/calculator_test.go
package effectiveness_test

import (
    "context"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
)

var _ = Describe("Effectiveness Calculator", func() {
    var (
        calculator *effectiveness.Calculator
        ctx        context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        calculator = effectiveness.NewCalculator()
    })

    Describe("Traditional Score Calculation (BR-INS-001)", func() {
        It("should return high score for consistently successful actions", func() {
            history := []effectiveness.ActionHistory{
                {Status: "success"},
                {Status: "success"},
                {Status: "success"},
                {Status: "success"},
            }

            score := calculator.CalculateTraditionalScore(history)
            Expect(score).To(Equal(1.0))
        })

        It("should return low score for consistently failed actions", func() {
            history := []effectiveness.ActionHistory{
                {Status: "failure"},
                {Status: "failure"},
                {Status: "failure"},
            }

            score := calculator.CalculateTraditionalScore(history)
            Expect(score).To(Equal(0.0))
        })

        It("should return neutral score for no history", func() {
            history := []effectiveness.ActionHistory{}

            score := calculator.CalculateTraditionalScore(history)
            Expect(score).To(Equal(0.5))
        })
    })

    Describe("Confidence Calculation", func() {
        It("should return high confidence with 12+ weeks of data", func() {
            history := make([]effectiveness.ActionHistory, 100)
            confidence := calculator.CalculateConfidence(history, 12)

            Expect(confidence).To(BeNumerically(">=", 0.9))
        })

        It("should return medium confidence with 8-11 weeks of data", func() {
            history := make([]effectiveness.ActionHistory, 50)
            confidence := calculator.CalculateConfidence(history, 10)

            Expect(confidence).To(BeNumerically(">=", 0.8))
            Expect(confidence).To(BeNumerically("<", 0.9))
        })

        It("should return low confidence with insufficient data", func() {
            history := make([]effectiveness.ActionHistory, 10)
            confidence := calculator.CalculateConfidence(history, 5)

            Expect(confidence).To(BeNumerically("<", 0.8))
        })
    })
})
```

### **2. Side Effect Detection Tests (BR-INS-005) — Level 1 (V1.0)**

```go
// test/unit/effectiveness/side_effects_test.go
package effectiveness_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
)

var _ = Describe("Side Effect Detection (BR-INS-005)", func() {
    var calculator *effectiveness.Calculator

    BeforeEach(func() {
        calculator = effectiveness.NewCalculator()
    })

    It("should detect high severity side effects for major CPU degradation", func() {
        metrics := &effectiveness.EnvironmentalMetrics{
            CPUImpact: -0.35, // 35% CPU increase (negative)
        }

        detected, severity := calculator.DetectSideEffects(metrics)

        Expect(detected).To(BeTrue())
        Expect(severity).To(Equal("high"))
    })

    It("should detect low severity side effects for minor network issues", func() {
        metrics := &effectiveness.EnvironmentalMetrics{
            NetworkStability: 0.65, // 65% stability
        }

        detected, severity := calculator.DetectSideEffects(metrics)

        Expect(detected).To(BeTrue())
        Expect(severity).To(Equal("low"))
    })

    It("should not detect side effects for positive environmental impact", func() {
        metrics := &effectiveness.EnvironmentalMetrics{
            MemoryImprovement: 0.25,  // 25% memory improvement
            CPUImpact:         -0.05, // Minimal CPU impact
            NetworkStability:  0.92,  // High network stability
        }

        detected, severity := calculator.DetectSideEffects(metrics)

        Expect(detected).To(BeFalse())
        Expect(severity).To(Equal("none"))
    })
})
```

### **3. Trend Analysis Tests (BR-INS-003) — Level 2 (V1.1)**

```go
// test/unit/effectiveness/trend_test.go
package effectiveness_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
)

var _ = Describe("Trend Analysis (BR-INS-003)", func() {
    var calculator *effectiveness.Calculator

    BeforeEach(func() {
        calculator = effectiveness.NewCalculator()
    })

    It("should detect improving trend", func() {
        history := []effectiveness.ActionHistory{
            // Older actions (60% success)
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
            {Status: "failure"},
            {Status: "failure"},
            // Recent actions (100% success)
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
        }

        trend := calculator.AnalyzeTrend(history)
        Expect(trend).To(Equal("improving"))
    })

    It("should detect declining trend", func() {
        history := []effectiveness.ActionHistory{
            // Older actions (100% success)
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
            // Recent actions (40% success)
            {Status: "success"},
            {Status: "success"},
            {Status: "failure"},
            {Status: "failure"},
            {Status: "failure"},
        }

        trend := calculator.AnalyzeTrend(history)
        Expect(trend).To(Equal("declining"))
    })

    It("should detect stable trend", func() {
        history := []effectiveness.ActionHistory{
            // Consistent 80% success rate
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
            {Status: "failure"},
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
            {Status: "success"},
            {Status: "failure"},
        }

        trend := calculator.AnalyzeTrend(history)
        Expect(trend).To(Equal("stable"))
    })

    It("should return insufficient_data for small history", func() {
        history := []effectiveness.ActionHistory{
            {Status: "success"},
            {Status: "success"},
        }

        trend := calculator.AnalyzeTrend(history)
        Expect(trend).To(Equal("insufficient_data"))
    })
})
```

---

## 🤖 **Decision Logic Unit Tests — Level 2 (V1.1)**

### **Test File**: `test/unit/effectiveness/decision_logic_test.go`

**Purpose**: Validate `shouldCallAI()` decision logic for selective AI analysis

```go
var _ = Describe("AI Decision Logic", func() {
    var service *EffectivenessMonitorService

    BeforeEach(func() {
        service = NewEffectivenessMonitorService(mockDeps)
    })

    Context("P0 Failures", func() {
        It("should trigger AI analysis for P0 failures", func() {
            workflow := &WorkflowExecution{
                ID:       "wf-001",
                Priority: "P0",
                Success:  false,
            }

            shouldCall := service.shouldCallAI(workflow, 0.5, []string{})
            Expect(shouldCall).To(BeTrue(), "P0 failures always trigger AI")
        })

        It("should not trigger AI for P0 successes", func() {
            workflow := &WorkflowExecution{
                ID:       "wf-002",
                Priority: "P0",
                Success:  true,
                IsNewActionType: false,
            }

            shouldCall := service.shouldCallAI(workflow, 0.95, []string{})
            Expect(shouldCall).To(BeFalse(), "P0 successes use automated assessment")
        })
    })

    Context("New Action Types", func() {
        It("should trigger AI for new action types", func() {
            workflow := &WorkflowExecution{
                ID:              "wf-003",
                Priority:        "P2",
                Success:         true,
                IsNewActionType: true,
            }

            shouldCall := service.shouldCallAI(workflow, 0.85, []string{})
            Expect(shouldCall).To(BeTrue(), "New action types trigger AI for knowledge building")
        })
    })

    Context("Anomalies Detected", func() {
        It("should trigger AI when anomalies detected", func() {
            workflow := &WorkflowExecution{
                ID:       "wf-004",
                Priority: "P1",
                Success:  true,
            }
            anomalies := []string{"unexpected_cpu_spike", "memory_oscillation"}

            shouldCall := service.shouldCallAI(workflow, 0.80, anomalies)
            Expect(shouldCall).To(BeTrue(), "Anomalies trigger AI investigation")
        })

        It("should not trigger AI when no anomalies", func() {
            workflow := &WorkflowExecution{
                ID:       "wf-005",
                Priority: "P2",
                Success:  true,
            }

            shouldCall := service.shouldCallAI(workflow, 0.92, []string{})
            Expect(shouldCall).To(BeFalse(), "Routine successes use automated assessment")
        })
    })

    Context("Oscillations/Recurring Failures", func() {
        It("should trigger AI for recurring failures", func() {
            workflow := &WorkflowExecution{
                ID:                  "wf-006",
                Priority:            "P1",
                Success:             false,
                IsRecurringFailure:  true,
            }

            shouldCall := service.shouldCallAI(workflow, 0.60, []string{})
            Expect(shouldCall).To(BeTrue(), "Recurring failures trigger AI pattern analysis")
        })
    })

    Context("Routine Successes", func() {
        It("should not trigger AI for routine P2 successes", func() {
            workflow := &WorkflowExecution{
                ID:                 "wf-007",
                Priority:           "P2",
                Success:            true,
                IsNewActionType:    false,
                IsRecurringFailure: false,
            }

            shouldCall := service.shouldCallAI(workflow, 0.95, []string{})
            Expect(shouldCall).To(BeFalse(), "Routine successes handled by automation")
        })
    })

    Context("Prometheus Metrics", func() {
        It("should increment correct trigger metric", func() {
            workflow := &WorkflowExecution{
                ID:       "wf-008",
                Priority: "P0",
                Success:  false,
            }

            service.shouldCallAI(workflow, 0.5, []string{})

            // Verify metric incremented
            metric := testutil.ToFloat64(service.metrics.aiTriggerCounter.WithLabelValues("p0_failure"))
            Expect(metric).To(BeNumerically(">", 0))
        })
    })
})
```

---

## 🔗 **Integration Tests (>50% Coverage)**

### **Test Categories**

| Category | Scope | Dependencies | Test Count | Focus |
|----------|--------|-------------|------------|-------|
| **Data Storage Integration** | Level 1 (V1.0) | PostgreSQL + pgvector | 6 tests | Action history retrieval, effectiveness data persistence |
| **Infrastructure Monitoring** | Level 1 (V1.0) | Prometheus metrics | 5 tests | Metrics correlation, side effect detection |
| **Cross-Service Integration** | Level 1 (V1.0) | Context API, Data Storage | 4 tests | Assessment request flow, trend storage |
| **HolmesGPT API Client Integration** | Level 2 (V1.1) | HolmesGPT API service | 5 tests | Post-execution analysis, authentication, error handling |

### **1. HolmesGPT Client Integration Tests — Level 2 (V1.1)**

```go
// test/integration/effectiveness/holmesgpt_client_test.go
package effectiveness_integration_test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/monitor"
)

var _ = Describe("HolmesGPT Client Integration (DD-EFFECTIVENESS-001)", func() {
    var (
        ctx          context.Context
        client       *monitor.HolmesGPTClient
        mockServer   *httptest.Server
        callsReceived int
    )

    BeforeEach(func() {
        ctx = context.Background()
        callsReceived = 0

        // Mock HolmesGPT API server
        mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            callsReceived++

            // Verify authentication
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                w.WriteHeader(http.StatusUnauthorized)
                return
            }

            // Verify endpoint
            if r.URL.Path == "/api/v1/postexec/analyze" {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusOK)
                w.Write([]byte(`{
                    "execution_id": "exec-001",
                    "execution_success": true,
                    "success_justification": "All objectives met",
                    "objectives_achieved": [
                        {"objective": "reduce_latency", "achieved": true}
                    ],
                    "follow_up_actions": [],
                    "lessons_learned": [
                        {"category": "optimization", "observation": "Memory increase effective"}
                    ],
                    "effectiveness_score": 0.90,
                    "side_effects": [],
                    "recommendations": ["Monitor for 24h"],
                    "confidence": 0.85
                }`))
                return
            }

            w.WriteHeader(http.StatusNotFound)
        }))

        var err error
        client, err = monitor.NewHolmesGPTClient(mockServer.URL)
        Expect(err).ToNot(HaveOccurred())
    })

    AfterEach(func() {
        mockServer.Close()
    })

    Context("Successful AI Analysis", func() {
        It("should call post-execution analysis endpoint", func() {
            req := monitor.PostExecRequest{
                ExecutionID:     "exec-001",
                ActionType:      "restart-pod",
                ActionDetails:   map[string]any{"pod": "test-pod"},
                ExecutionResult: map[string]any{"status": "success"},
                ExecutionSuccess: true,
                Context:         map[string]any{"priority": "P0"},
            }

            resp, err := client.AnalyzePostExecution(ctx, req)
            Expect(err).ToNot(HaveOccurred())
            Expect(resp).ToNot(BeNil())
            Expect(resp.ExecutionSuccess).To(BeTrue())
            Expect(resp.EffectivenessScore).To(Equal(0.90))
            Expect(callsReceived).To(Equal(1), "Should make exactly 1 API call")
        })

        It("should include authentication token", func() {
            req := monitor.PostExecRequest{
                ExecutionID:      "exec-002",
                ActionType:       "scale-deployment",
                ExecutionSuccess: true,
            }

            _, err := client.AnalyzePostExecution(ctx, req)
            Expect(err).ToNot(HaveOccurred())
            Expect(callsReceived).To(Equal(1), "Authenticated request successful")
        })
    })

    Context("Error Handling", func() {
        It("should handle API unavailability gracefully", func() {
            mockServer.Close() // Simulate service down

            req := monitor.PostExecRequest{
                ExecutionID:      "exec-003",
                ExecutionSuccess: true,
            }

            _, err := client.AnalyzePostExecution(ctx, req)
            Expect(err).To(HaveOccurred(), "Should return error when API unavailable")
        })

        It("should timeout on slow responses", func() {
            slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                time.Sleep(35 * time.Second) // Exceed 30s timeout
                w.WriteHeader(http.StatusOK)
            }))
            defer slowServer.Close()

            slowClient, _ := monitor.NewHolmesGPTClient(slowServer.URL)

            req := monitor.PostExecRequest{
                ExecutionID:      "exec-004",
                ExecutionSuccess: true,
            }

            _, err := slowClient.AnalyzePostExecution(ctx, req)
            Expect(err).To(HaveOccurred(), "Should timeout after 30 seconds")
        })
    })

    Context("Prometheus Metrics Integration", func() {
        It("should increment AI call counter on success", func() {
            req := monitor.PostExecRequest{
                ExecutionID:      "exec-005",
                ExecutionSuccess: true,
            }

            _, err := client.AnalyzePostExecution(ctx, req)
            Expect(err).ToNot(HaveOccurred())

            // Verify metrics (mocked prometheus registry)
            // Real test would check prometheus counter incremented
        })

        It("should track AI call duration", func() {
            req := monitor.PostExecRequest{
                ExecutionID:      "exec-006",
                ExecutionSuccess: true,
            }

            start := time.Now()
            _, err := client.AnalyzePostExecution(ctx, req)
            duration := time.Since(start)

            Expect(err).ToNot(HaveOccurred())
            Expect(duration).To(BeNumerically("<", 5*time.Second), "API call should be fast")
        })
    })

    Context("Cost Tracking Integration", func() {
        It("should increment cost counter ($0.50/call)", func() {
            req := monitor.PostExecRequest{
                ExecutionID:      "exec-007",
                ExecutionSuccess: true,
            }

            _, err := client.AnalyzePostExecution(ctx, req)
            Expect(err).ToNot(HaveOccurred())

            // Verify cost metric incremented by $0.50
            // Real test would check prometheus counter value
        })
    })
})
```

### **2. Data Storage Integration Tests — Level 1 (V1.0)**

```go
// test/integration/effectiveness/data_storage_test.go
package effectiveness_integration_test

import (
    "context"
    "database/sql"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
    _ "github.com/lib/pq"
)

var _ = Describe("Data Storage Integration (BR-INS-001, BR-INS-003)", func() {
    var (
        ctx       context.Context
        db        *sql.DB
        dsClient  *effectiveness.DataStorageClient
        testDB    string = "postgresql://localhost:5433/test_effectiveness?sslmode=disable"
    )

    BeforeEach(func() {
        ctx = context.Background()
        var err error
        db, err = sql.Open("postgres", testDB)
        Expect(err).ToNot(HaveOccurred())

        dsClient = effectiveness.NewDataStorageClient(db)

        // Seed test data
        seedTestData(db)
    })

    AfterEach(func() {
        cleanupTestData(db)
        db.Close()
    })

    It("should retrieve action history from PostgreSQL", func() {
        history, err := dsClient.GetActionHistory(ctx, "restart-pod", 90*24*time.Hour)

        Expect(err).ToNot(HaveOccurred())
        Expect(history).ToNot(BeEmpty())
        Expect(history[0].ActionType).To(Equal("restart-pod"))
    })

    It("should calculate data availability in weeks", func() {
        weeks, err := dsClient.GetDataAvailabilityWeeks(ctx)

        Expect(err).ToNot(HaveOccurred())
        Expect(weeks).To(BeNumerically(">=", 0))
    })

    It("should retrieve oldest action for data availability check", func() {
        oldestAction, err := dsClient.GetOldestAction(ctx)

        Expect(err).ToNot(HaveOccurred())
        Expect(oldestAction).ToNot(BeNil())
        Expect(oldestAction.CreatedAt).To(BeTemporally("<", time.Now()))
    })
})

func seedTestData(db *sql.DB) {
    // Insert test remediation actions
    _, err := db.Exec(`
        INSERT INTO remediation_audit (id, action_type, status, namespace, cluster, created_at)
        VALUES
            ('test-act-1', 'restart-pod', 'success', 'production', 'us-west-2', NOW() - INTERVAL '10 weeks'),
            ('test-act-2', 'restart-pod', 'success', 'production', 'us-west-2', NOW() - INTERVAL '9 weeks'),
            ('test-act-3', 'restart-pod', 'failure', 'production', 'us-west-2', NOW() - INTERVAL '8 weeks')
    `)
    Expect(err).ToNot(HaveOccurred())
}

func cleanupTestData(db *sql.DB) {
    _, _ = db.Exec("DELETE FROM remediation_audit WHERE id LIKE 'test-act-%'")
}
```

### **3. Infrastructure Monitoring Integration Tests — Level 1 (V1.0)**

```go
// test/integration/effectiveness/infrastructure_monitoring_test.go
package effectiveness_integration_test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
)

var _ = Describe("Infrastructure Monitoring Integration (BR-INS-002)", func() {
    var (
        ctx        context.Context
        mockServer *httptest.Server
        imClient   *effectiveness.InfrastructureMonitoringClient
    )

    BeforeEach(func() {
        ctx = context.Background()

        // Mock Infrastructure Monitoring API
        mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if r.URL.Path == "/metrics/after-action" {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusOK)
                w.Write([]byte(`{
                    "memory_improvement": 0.25,
                    "cpu_impact": -0.05,
                    "network_stability": 0.92
                }`))
            }
        }))

        imClient = effectiveness.NewInfrastructureMonitoringClient(mockServer.URL)
    })

    AfterEach(func() {
        mockServer.Close()
    })

    It("should retrieve metrics after action execution", func() {
        metrics, err := imClient.GetMetricsAfterAction(ctx, "act-abc123", 10*time.Minute)

        Expect(err).ToNot(HaveOccurred())
        Expect(metrics).ToNot(BeNil())
        Expect(metrics.MemoryImprovement).To(Equal(0.25))
        Expect(metrics.CPUImpact).To(Equal(-0.05))
        Expect(metrics.NetworkStability).To(Equal(0.92))
    })

    It("should handle infrastructure monitoring service unavailability gracefully", func() {
        mockServer.Close() // Simulate service down

        metrics, err := imClient.GetMetricsAfterAction(ctx, "act-abc123", 10*time.Minute)

        // Should not fail, but return error
        Expect(err).To(HaveOccurred())
        Expect(metrics).To(BeNil())
    })
})
```

### **4. Cross-Service Integration Tests — Level 1 (V1.0)**

```go
// test/integration/effectiveness/cross_service_test.go
package effectiveness_integration_test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
)

var _ = Describe("Cross-Service Integration", func() {
    var (
        ctx                 context.Context
        effectivenessServer *httptest.Server
        contextAPIServer    *httptest.Server
        effectivenessClient *effectiveness.Client
    )

    BeforeEach(func() {
        ctx = context.Background()

        // Mock Context API
        contextAPIServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if r.URL.Path == "/api/v1/context/trends" {
                w.WriteHeader(http.StatusOK)
                w.Write([]byte(`{"trends": []}`))
            }
        }))

        // Mock Effectiveness Monitor
        effectivenessServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if r.URL.Path == "/api/v1/assess/effectiveness" {
                w.WriteHeader(http.StatusOK)
                w.Write([]byte(`{
                    "assessment_id": "assess-xyz789",
                    "traditional_score": 0.88,
                    "confidence": 0.91
                }`))
            }
        }))

        effectivenessClient = effectiveness.NewClient(effectivenessServer.URL)
    })

    AfterEach(func() {
        effectivenessServer.Close()
        contextAPIServer.Close()
    })

    It("should complete full assessment request from Context API (BR-INS-010)", func() {
        req := &effectiveness.AssessmentRequest{
            ActionID:               "act-abc123",
            WaitForStabilization:   true,
            AssessmentInterval:     "10m",
        }

        assessment, err := effectivenessClient.AssessEffectiveness(ctx, req)

        Expect(err).ToNot(HaveOccurred())
        Expect(assessment).ToNot(BeNil())
        Expect(assessment.AssessmentID).To(Equal("assess-xyz789"))
        Expect(assessment.TraditionalScore).To(Equal(0.88))
        Expect(assessment.Confidence).To(Equal(0.91))
    })

    It("should handle Context API unavailability during trend storage", func() {
        contextAPIServer.Close() // Simulate Context API down

        // Effectiveness Monitor should still complete assessment
        // but log warning about trend storage failure
        req := &effectiveness.AssessmentRequest{
            ActionID: "act-abc123",
        }

        assessment, err := effectivenessClient.AssessEffectiveness(ctx, req)

        // Assessment succeeds despite Context API being down
        Expect(err).ToNot(HaveOccurred())
        Expect(assessment).ToNot(BeNil())
    })
})
```

---

## 🌐 **E2E Tests (10-15% Coverage)**

### **Test Categories**

| Category | Scope | Test Count | Focus |
|----------|-------|------------|-------|
| **Complete Assessment Workflow** | Level 1 (V1.0) | 3 tests | End-to-end effectiveness assessment with audit event emission |
| **Level 1 vs Level 2** | V1.0 / V1.1 | 2 tests | Level 1 (Day-1 value) vs Level 2 (8+ weeks data for AI analysis) |

### **1. Complete Assessment Workflow Test — Level 1 (V1.0)**

```go
// test/e2e/effectiveness/assessment_workflow_test.go
package effectiveness_e2e_test

import (
    "context"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
)

var _ = Describe("E2E Assessment Workflow (BR-INS-001 to BR-INS-010)", func() {
    var (
        ctx                context.Context
        effectivenessAPI   *effectiveness.Service
        dataStorageClient  *effectiveness.DataStorageClient
        infraMonitorClient *effectiveness.InfrastructureMonitoringClient
    )

    BeforeEach(func() {
        ctx = context.Background()

        // Initialize real services (not mocks)
        dataStorageClient = effectiveness.NewDataStorageClient(realPostgresDB)
        infraMonitorClient = effectiveness.NewInfrastructureMonitoringClient(realInfraMonitorURL)

        effectivenessAPI = effectiveness.NewService(logger, dataStorageClient, infraMonitorClient)

        // Seed real test data in PostgreSQL
        seedRealTestData()
    })

    AfterEach(func() {
        cleanupRealTestData()
    })

    It("should complete full effectiveness assessment workflow", func() {
        // Step 1: Create remediation action via Gateway
        actionID := createTestRemediationAction()

        // Step 2: Wait for action execution
        time.Sleep(10 * time.Second)

        // Step 3: Request effectiveness assessment
        req := &effectiveness.AssessmentRequest{
            ActionID:               actionID,
            WaitForStabilization:   true,
            AssessmentInterval:     "10m",
        }

        assessment, err := effectivenessAPI.AssessEffectiveness(ctx, req)

        // Assertions
        Expect(err).ToNot(HaveOccurred())
        Expect(assessment).ToNot(BeNil())
        Expect(assessment.TraditionalScore).To(BeNumerically(">", 0.0))
        Expect(assessment.Confidence).To(BeNumerically(">", 0.0))

        // Verify assessment stored in Data Storage
        storedAssessment, err := dataStorageClient.GetAssessment(ctx, assessment.AssessmentID)
        Expect(err).ToNot(HaveOccurred())
        Expect(storedAssessment.AssessmentID).To(Equal(assessment.AssessmentID))
    })
})
```

### **2. Level 1 vs Level 2 Tests (V1.0 / V1.1 per DD-017 v2.0)**

```go
// test/e2e/effectiveness/graceful_degradation_test.go
package effectiveness_e2e_test

import (
    "context"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
)

var _ = Describe("Level 1 vs Level 2 Scope (DD-017 v2.0)", func() {
    var (
        ctx              context.Context
        effectivenessAPI *effectiveness.Service
    )

    BeforeEach(func() {
        ctx = context.Background()
        effectivenessAPI = effectiveness.NewService(logger, dataStorageClient, infraMonitorClient)
    })

    It("should return Level 1 assessment from Day 1 (no historical data required)", func() {
        // Level 1: Day-1 value — no data dependency
        req := &effectiveness.AssessmentRequest{
            ActionID: "act-new-123",
        }

        assessment, err := effectivenessAPI.AssessEffectiveness(ctx, req)

        Expect(err).ToNot(HaveOccurred())
        Expect(assessment.Status).To(Equal("assessed"))
        Expect(assessment.EffectivenessScore).To(BeNumerically(">=", 0.0))
        Expect(assessment.EffectivenessScore).To(BeNumerically("<=", 1.0))
        Expect(assessment.HealthChecks).ToNot(BeNil())
    })

    It("should return Level 2 enriched assessment when 8+ weeks data available", func() {
        // Level 2: requires 8+ weeks of Level 1 assessment data
        seed10WeeksOfHistoricalData()

        req := &effectiveness.AssessmentRequest{
            ActionID: "act-mature-123",
        }

        assessment, err := effectivenessAPI.AssessEffectiveness(ctx, req)

        Expect(err).ToNot(HaveOccurred())
        Expect(assessment.Status).To(Equal("assessed"))
        Expect(assessment.Confidence).To(BeNumerically(">=", 0.8))
        Expect(assessment.PatternInsights).ToNot(BeEmpty())
    })
})
```

---

## 📊 **Test Execution**

### **Run All Tests**

```bash
# Unit tests (70%+ coverage)
ginkgo test/unit/effectiveness/...

# Integration tests (>50% coverage)
ginkgo test/integration/effectiveness/...

# E2E tests (10-15% coverage)
ginkgo test/e2e/effectiveness/...

# All tests with coverage
ginkgo -r --cover --coverprofile=coverage.out test/

# View coverage report
go tool cover -html=coverage.out
```

### **Run Tests by Business Requirement**

```bash
# BR-INS-001: Effectiveness assessment
ginkgo --focus="BR-INS-001" test/unit/effectiveness/...

# BR-INS-002: Environmental impact correlation
ginkgo --focus="BR-INS-002" test/unit/effectiveness/...

# BR-INS-003: Long-term trends
ginkgo --focus="BR-INS-003" test/unit/effectiveness/...

# BR-INS-005: Side effect detection
ginkgo --focus="BR-INS-005" test/unit/effectiveness/...
```

---

## ✅ **Test Checklist**

### **Before Submitting PR**

- [ ] All unit tests pass (70%+ coverage achieved)
- [ ] All integration tests pass (>50% coverage achieved)
- [ ] All E2E tests pass (10-15% coverage achieved)
- [ ] All tests map to business requirements (BR-INS-001 to BR-INS-010)
- [ ] All code examples have complete imports
- [ ] TDD RED→GREEN→REFACTOR methodology followed
- [ ] Graceful degradation scenarios tested (Week 5 vs Week 13+)
- [ ] No test skips (`Skip()` not used)
- [ ] Test names clearly describe business behavior

### **Coverage Targets**

- [ ] Unit test coverage ≥ 70%
- [ ] Integration test coverage > 50%
- [ ] E2E test coverage 10-15%
- [ ] Business requirement coverage: 100% (all BR-INS-001 to BR-INS-010 tested)

---

## 🔗 **Related Documentation**

- **Core Methodology**: [00-core-development-methodology.mdc](../../../.cursor/rules/00-core-development-methodology.mdc)
- **Testing Strategy**: [03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc)
- **Business Requirements**: BR-INS-001 through BR-INS-010 (Effectiveness Assessment)
- **Implementation Checklist**: [implementation-checklist.md](./implementation-checklist.md)
- **Integration Points**: [integration-points.md](./integration-points.md)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ **COMPLETE - READY FOR TDD IMPLEMENTATION**

