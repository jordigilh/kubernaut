# E2E Testing Implementation Guide

**Document Version**: 1.0
**Date**: January 2025
**Status**: Implementation Ready
**Author**: Kubernaut Development Team

---

## Executive Summary

This implementation guide provides step-by-step instructions for implementing the **Top 10 E2E Use Cases** and the comprehensive testing automation framework. The guide follows development guidelines, leverages existing infrastructure, and ensures business outcome validation throughout the implementation process.

### Implementation Principles
- **Incremental Development**: Build upon existing test infrastructure
- **Business Requirements First**: Every implementation maps to specific BR-XXX requirements
- **Development Guidelines Compliance**: Follow established patterns and practices
- **Real Infrastructure Integration**: Leverage Kind cluster and oss-gpt:20b model
- **Automated Validation**: Continuous business outcome verification

---

## 🏗️ **Phase 1: Foundation Setup (Weeks 1-2)**

### **Step 1.1: Enhanced Test Factory Implementation**

#### **Extend Existing Test Infrastructure**
Build upon the existing `test/integration/shared/test_factory.go` to add e2e capabilities.

```bash
# Create the enhanced test factory
cd test/integration/shared/
cp test_factory.go e2e_test_factory.go
```

#### **Implementation: E2E Test Suite Extension**

```go
// File: test/integration/shared/e2e_test_factory.go
package shared

import (
    "context"
    "time"

    "github.com/jordigilh/kubernaut/pkg/chaos"
    "github.com/jordigilh/kubernaut/pkg/load"
    "github.com/jordigilh/kubernaut/pkg/validation"

    "github.com/sirupsen/logrus"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

// E2ETestSuite extends StandardTestSuite with chaos and business validation
type E2ETestSuite struct {
    *StandardTestSuite

    // Chaos Engineering Components
    ChaosEngine     chaos.Engine
    ChaosValidator  *chaos.ResultValidator

    // Load Testing Components
    LoadGenerator   *load.TestingEngine
    AIStressTest    *load.AIModelStressTest

    // Business Validation Components
    MetricsValidator *validation.BusinessMetricsValidator
    SLAMonitor      *validation.ServiceLevelMonitor

    // Integration Components
    HolmesGPTClient *holmesgpt.IntegrationClient
    ContextAPIClient *contextapi.TestClient
}

// NewE2ETestSuite creates enhanced test suite for e2e testing
func NewE2ETestSuite(suiteName string, opts ...TestOption) (*E2ETestSuite, error) {
    // Create base StandardTestSuite
    baseSuite, err := NewStandardTestSuite(suiteName, opts...)
    if err != nil {
        return nil, fmt.Errorf("failed to create base test suite: %w", err)
    }

    e2eSuite := &E2ETestSuite{
        StandardTestSuite: baseSuite,
    }

    // Initialize e2e-specific components
    if err := e2eSuite.initializeE2EComponents(); err != nil {
        return nil, fmt.Errorf("failed to initialize e2e components: %w", err)
    }

    return e2eSuite, nil
}

func (s *E2ETestSuite) initializeE2EComponents() error {
    // Initialize chaos engine
    chaosConfig := &chaos.Config{
        Namespace:    "chaos-testing",
        ServiceAccount: "chaos-e2e-runner",
        Logger:       s.Logger,
    }
    s.ChaosEngine = chaos.NewEngine(chaosConfig, s.TestEnv.KubeClient)
    s.ChaosValidator = chaos.NewResultValidator(s.Logger)

    // Initialize load testing
    loadConfig := &load.Config{
        DefaultConcurrency: 10,
        DefaultDuration:    5 * time.Minute,
        Logger:            s.Logger,
    }
    s.LoadGenerator = load.NewTestingEngine(loadConfig)
    s.AIStressTest = load.NewAIModelStressTest(loadConfig, s.LLMClient)

    // Initialize business validation
    metricsConfig := &validation.BusinessMetricsConfig{
        RequirementsPath: "docs/requirements/",
        SLAThresholds:   validation.DefaultSLAThresholds(),
        Logger:          s.Logger,
    }
    s.MetricsValidator = validation.NewBusinessMetricsValidator(metricsConfig)
    s.SLAMonitor = validation.NewServiceLevelMonitor(metricsConfig, s.TestEnv.PrometheusClient)

    return nil
}
```

#### **Create Chaos Engineering Package**

```bash
# Create chaos engineering package structure
mkdir -p pkg/chaos
mkdir -p pkg/load
mkdir -p pkg/validation
```

```go
// File: pkg/chaos/engine.go
package chaos

import (
    "context"
    "fmt"
    "time"

    litmuschaos "github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "github.com/sirupsen/logrus"
)

type Engine interface {
    ExecuteExperiment(ctx context.Context, experiment *ChaosExperiment) (*ChaosResult, error)
    ValidateRecovery(ctx context.Context, result *ChaosResult) (*RecoveryValidation, error)
    CleanupExperiment(ctx context.Context, experimentID string) error
}

type DefaultChaosEngine struct {
    config     *Config
    kubeClient client.Client
    logger     *logrus.Logger
}

type ChaosExperiment struct {
    ID               string                 `json:"id"`
    Name             string                 `json:"name"`
    Type             string                 `json:"type"`
    TargetNamespace  string                 `json:"target_namespace"`
    Duration         time.Duration          `json:"duration"`
    Parameters       map[string]interface{} `json:"parameters"`
    BusinessRequirement string              `json:"business_requirement"`
    ExpectedOutcome  ExpectedOutcome        `json:"expected_outcome"`
}

type ChaosResult struct {
    ExperimentID     string        `json:"experiment_id"`
    Status           string        `json:"status"`
    StartTime        time.Time     `json:"start_time"`
    EndTime          time.Time     `json:"end_time"`
    Duration         time.Duration `json:"duration"`
    BusinessOutcome  BusinessOutcome `json:"business_outcome"`
    PerformanceData  PerformanceData `json:"performance_data"`
}

func NewEngine(config *Config, kubeClient client.Client) Engine {
    return &DefaultChaosEngine{
        config:     config,
        kubeClient: kubeClient,
        logger:     config.Logger,
    }
}

func (e *DefaultChaosEngine) ExecuteExperiment(ctx context.Context, experiment *ChaosExperiment) (*ChaosResult, error) {
    e.logger.Infof("Executing chaos experiment: %s", experiment.Name)

    // Create LitmusChaos ChaosEngine resource
    chaosEngine := &litmuschaos.ChaosEngine{
        ObjectMeta: metav1.ObjectMeta{
            Name:      experiment.ID,
            Namespace: experiment.TargetNamespace,
        },
        Spec: litmuschaos.ChaosEngineSpec{
            ChaosServiceAccount: e.config.ServiceAccount,
            EngineState:        "active",
            Experiments: []litmuschaos.ExperimentList{
                {
                    Name: experiment.Type,
                    Spec: litmuschaos.ExperimentAttributes{
                        Components: litmuschaos.ExperimentStatus{
                            StatusCheckTimeouts: litmuschaos.StatusCheckTimeout{
                                Delay:   2,
                                Timeout: 180,
                            },
                        },
                    },
                },
            },
        },
    }

    // Deploy chaos experiment
    if err := e.kubeClient.Create(ctx, chaosEngine); err != nil {
        return nil, fmt.Errorf("failed to create chaos engine: %w", err)
    }

    // Monitor experiment execution
    result, err := e.monitorExperiment(ctx, experiment)
    if err != nil {
        return nil, fmt.Errorf("failed to monitor experiment: %w", err)
    }

    return result, nil
}
```

### **Step 1.2: Business Requirements Validation**

#### **Create Business Metrics Validator**

```go
// File: pkg/validation/business_metrics.go
package validation

import (
    "context"
    "fmt"
    "time"

    "github.com/sirupsen/logrus"
)

type BusinessMetricsValidator struct {
    config         *BusinessMetricsConfig
    requirements   map[string]BusinessRequirement
    slaThresholds  map[string]float64
    logger         *logrus.Logger
}

type BusinessRequirement struct {
    ID              string                 `yaml:"id"`
    Description     string                 `yaml:"description"`
    SuccessCriteria map[string]interface{} `yaml:"success_criteria"`
    ValidationProbes []ValidationProbe     `yaml:"validation_probes"`
}

type ValidationResult struct {
    RequirementID   string                    `json:"requirement_id"`
    TestCase        string                    `json:"test_case"`
    Timestamp       time.Time                 `json:"timestamp"`
    Passed          bool                      `json:"passed"`
    Criteria        []CriteriaValidation      `json:"criteria"`
    BusinessImpact  float64                   `json:"business_impact"`
    Recommendations []string                  `json:"recommendations"`
}

func NewBusinessMetricsValidator(config *BusinessMetricsConfig) *BusinessMetricsValidator {
    validator := &BusinessMetricsValidator{
        config:        config,
        requirements:  make(map[string]BusinessRequirement),
        slaThresholds: config.SLAThresholds,
        logger:        config.Logger,
    }

    // Load business requirements from documentation
    if err := validator.loadRequirements(); err != nil {
        validator.logger.Errorf("Failed to load business requirements: %v", err)
    }

    return validator
}

func (v *BusinessMetricsValidator) ValidateBusinessRequirement(
    ctx context.Context,
    requirementID string,
    testMetrics TestMetrics,
) (*ValidationResult, error) {
    requirement, exists := v.requirements[requirementID]
    if !exists {
        return nil, fmt.Errorf("unknown business requirement: %s", requirementID)
    }

    result := &ValidationResult{
        RequirementID: requirementID,
        TestCase:      testMetrics.TestCase,
        Timestamp:     time.Now(),
        Criteria:      []CriteriaValidation{},
    }

    // Validate each success criteria
    for criteriaName, expectedValue := range requirement.SuccessCriteria {
        criteria := v.validateCriteria(criteriaName, expectedValue, testMetrics)
        result.Criteria = append(result.Criteria, criteria)
    }

    // Calculate overall pass/fail
    result.Passed = v.calculateOverallValidation(result.Criteria)
    result.BusinessImpact = v.calculateBusinessImpact(result)
    result.Recommendations = v.generateRecommendations(result)

    return result, nil
}

func (v *BusinessMetricsValidator) validateCriteria(
    criteriaName string,
    expectedValue interface{},
    testMetrics TestMetrics,
) CriteriaValidation {
    criteria := CriteriaValidation{
        Name:     criteriaName,
        Expected: expectedValue,
        Actual:   testMetrics.GetValue(criteriaName),
    }

    switch criteriaName {
    case "accuracy_threshold":
        threshold, _ := expectedValue.(float64)
        actual, _ := testMetrics.GetValue(criteriaName).(float64)
        criteria.Passed = actual >= threshold

    case "response_time_max":
        maxTime, _ := expectedValue.(string)
        maxDuration, _ := time.ParseDuration(maxTime)
        actualDuration, _ := testMetrics.GetValue(criteriaName).(time.Duration)
        criteria.Passed = actualDuration <= maxDuration

    case "zero_data_loss":
        expected, _ := expectedValue.(bool)
        actual, _ := testMetrics.GetValue(criteriaName).(bool)
        criteria.Passed = actual == expected

    default:
        criteria.Passed = false
        criteria.Error = fmt.Sprintf("unknown criteria: %s", criteriaName)
    }

    return criteria
}
```

### **Step 1.3: AI Model Stress Testing**

#### **Create AI Stress Testing Framework**

```go
// File: pkg/load/ai_stress_test.go
package load

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/jordigilh/kubernaut/pkg/llm"
    "github.com/sirupsen/logrus"
)

type AIModelStressTest struct {
    config    *Config
    llmClient llm.Client
    logger    *logrus.Logger
}

type AIStressTestConfig struct {
    ConcurrentRequests int           `yaml:"concurrent_requests"`
    Duration          time.Duration `yaml:"duration"`
    ModelEndpoint     string        `yaml:"model_endpoint"`
    Model             string        `yaml:"model"`
    AccuracyThreshold float64       `yaml:"accuracy_threshold"`
    TimeoutThreshold  time.Duration `yaml:"timeout_threshold"`
    TestScenario      string        `yaml:"test_scenario"`
}

type AIStressTestResult struct {
    TestID            string        `json:"test_id"`
    StartTime         time.Time     `json:"start_time"`
    EndTime           time.Time     `json:"end_time"`
    Duration          time.Duration `json:"duration"`
    TotalRequests     int           `json:"total_requests"`
    SuccessfulRequests int          `json:"successful_requests"`
    FailedRequests    int           `json:"failed_requests"`
    TimeoutRequests   int           `json:"timeout_requests"`
    DecisionAccuracy  float64       `json:"decision_accuracy"`
    AverageResponseTime time.Duration `json:"average_response_time"`
    ThroughputRPS     float64       `json:"throughput_rps"`
    BusinessRequirementMet bool     `json:"business_requirement_met"`
}

func NewAIModelStressTest(config *Config, llmClient llm.Client) *AIModelStressTest {
    return &AIModelStressTest{
        config:    config,
        llmClient: llmClient,
        logger:    config.Logger,
    }
}

func (a *AIModelStressTest) ExecuteStressTest(
    ctx context.Context,
    testConfig *AIStressTestConfig,
) (*AIStressTestResult, error) {
    a.logger.Infof("Starting AI stress test with %d concurrent requests for %v",
        testConfig.ConcurrentRequests, testConfig.Duration)

    result := &AIStressTestResult{
        TestID:    fmt.Sprintf("ai-stress-%d", time.Now().Unix()),
        StartTime: time.Now(),
    }

    // Create test scenarios
    scenarios, err := a.generateTestScenarios(testConfig.TestScenario, testConfig.ConcurrentRequests)
    if err != nil {
        return nil, fmt.Errorf("failed to generate test scenarios: %w", err)
    }

    // Execute concurrent stress test
    var wg sync.WaitGroup
    results := make(chan AIRequestResult, testConfig.ConcurrentRequests*2)

    ctx, cancel := context.WithTimeout(ctx, testConfig.Duration)
    defer cancel()

    // Start concurrent workers
    for i := 0; i < testConfig.ConcurrentRequests; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            a.executeWorker(ctx, workerID, scenarios, results)
        }(i)
    }

    // Collect results
    go func() {
        wg.Wait()
        close(results)
    }()

    // Aggregate results
    requestResults := []AIRequestResult{}
    for requestResult := range results {
        requestResults = append(requestResults, requestResult)
    }

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)

    // Calculate metrics
    a.calculateMetrics(result, requestResults, testConfig)

    return result, nil
}

func (a *AIModelStressTest) executeWorker(
    ctx context.Context,
    workerID int,
    scenarios []TestScenario,
    results chan<- AIRequestResult,
) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            // Select random scenario
            scenario := scenarios[workerID%len(scenarios)]

            // Execute AI request
            requestResult := a.executeAIRequest(ctx, scenario)
            results <- requestResult

            // Small delay between requests
            time.Sleep(100 * time.Millisecond)
        }
    }
}

func (a *AIModelStressTest) executeAIRequest(
    ctx context.Context,
    scenario TestScenario,
) AIRequestResult {
    startTime := time.Now()

    result := AIRequestResult{
        WorkerID:  scenario.WorkerID,
        Scenario:  scenario.Type,
        StartTime: startTime,
    }

    // Create AI request based on scenario
    alert := a.createAlertFromScenario(scenario)

    // Execute AI analysis
    recommendation, err := a.llmClient.AnalyzeAlert(ctx, alert)

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)

    if err != nil {
        result.Success = false
        result.Error = err.Error()
        if isTimeoutError(err) {
            result.Timeout = true
        }
    } else {
        result.Success = true
        result.Recommendation = recommendation
        result.DecisionQuality = a.evaluateDecisionQuality(scenario, recommendation)
    }

    return result
}
```

---

## 🧪 **Phase 2: Core Use Case Implementation (Weeks 3-4)**

### **Step 2.1: Implement Use Case #1 - AI-Driven Pod Resource Exhaustion Recovery**

#### **Create Test Implementation**

```go
// File: test/e2e/use_cases/pod_resource_exhaustion_test.go
package use_cases

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/test/integration/shared"
    "github.com/jordigilh/kubernaut/pkg/chaos"
    "github.com/jordigilh/kubernaut/pkg/validation"
)

var _ = Describe("Use Case #1: AI-Driven Pod Resource Exhaustion Recovery", func() {
    var (
        ctx       context.Context
        e2eSuite  *shared.E2ETestSuite
        testNamespace string
    )

    BeforeEach(func() {
        ctx = context.Background()
        testNamespace = "uc1-pod-resource-test"

        // Setup E2E test suite
        var err error
        e2eSuite, err = shared.NewE2ETestSuite("pod-resource-exhaustion",
            shared.WithRealDatabase(),
            shared.WithRealLLM(),
            shared.WithRealVectorDB(),
        )
        Expect(err).ToNot(HaveOccurred())

        // Create test namespace
        err = e2eSuite.TestEnv.CreateNamespace(testNamespace)
        Expect(err).ToNot(HaveOccurred())
    })

    AfterEach(func() {
        if e2eSuite != nil {
            err := e2eSuite.Cleanup()
            Expect(err).ToNot(HaveOccurred())
        }
    })

    Context("BR-AI-001: Intelligent Alert Analysis with Contextual Awareness", func() {
        It("should execute complete pod resource exhaustion recovery workflow", func() {
            By("Deploying memory-intensive test application")
            testApp, err := e2eSuite.DeployTestApplication(ctx, &shared.TestApplicationConfig{
                Name:        "memory-intensive-app",
                Namespace:   testNamespace,
                Type:        "memory-intensive",
                Replicas:    3,
                Resources: shared.ResourceRequirements{
                    Memory: "512Mi",
                    CPU:    "500m",
                },
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(testApp.IsReady(ctx)).To(BeTrue())

            By("Configuring chaos experiment for memory pressure")
            chaosExperiment := &chaos.ChaosExperiment{
                ID:              "uc1-memory-pressure-001",
                Name:            "pod-memory-hog",
                Type:            "pod-memory-hog",
                TargetNamespace: testNamespace,
                Duration:        10 * time.Minute,
                Parameters: map[string]interface{}{
                    "MEMORY_CONSUMPTION": "80%",
                    "TARGET_PODS":        testApp.PodSelector(),
                    "CHAOS_DURATION":     "600s",
                },
                BusinessRequirement: "BR-AI-001",
                ExpectedOutcome: chaos.ExpectedOutcome{
                    AlertsGenerated:     []string{"HighMemoryUsage", "PodMemoryPressure"},
                    AIActionsExpected:   []string{"increase_resources", "scale_deployment"},
                    RecoveryTimeMax:     30 * time.Second,
                    AccuracyThreshold:   0.90,
                },
            }

            By("Executing chaos experiment and monitoring AI response")
            chaosResult, err := e2eSuite.ChaosEngine.ExecuteExperiment(ctx, chaosExperiment)
            Expect(err).ToNot(HaveOccurred())
            Expect(chaosResult.Status).To(Equal("completed"))

            By("Validating AI decision-making and action execution")
            // Verify alerts were generated
            alerts, err := e2eSuite.GetGeneratedAlerts(ctx, testNamespace, chaosResult.Duration)
            Expect(err).ToNot(HaveOccurred())
            Expect(alerts).To(ContainElement(HaveField("Type", "HighMemoryUsage")))

            // Verify AI analysis was triggered
            aiDecisions, err := e2eSuite.GetAIDecisions(ctx, testNamespace, chaosResult.Duration)
            Expect(err).ToNot(HaveOccurred())
            Expect(aiDecisions).To(HaveLen(BeNumerically(">=", 1)))

            // Verify actions were executed
            executedActions, err := e2eSuite.GetExecutedActions(ctx, testNamespace, chaosResult.Duration)
            Expect(err).ToNot(HaveOccurred())
            Expect(executedActions).To(ContainElement(HaveField("Type", "increase_resources")))

            By("Validating business requirements compliance")
            businessValidation, err := e2eSuite.MetricsValidator.ValidateBusinessRequirement(
                ctx, "BR-AI-001", validation.TestMetrics{
                    TestCase:         "pod-resource-exhaustion",
                    AccuracyRate:     calculateAccuracyRate(aiDecisions, executedActions),
                    ResponseTime:     calculateAverageResponseTime(aiDecisions),
                    RecoveryTime:     chaosResult.BusinessOutcome.RecoveryTime,
                    ZeroDataLoss:     chaosResult.BusinessOutcome.DataIntegrity,
                })
            Expect(err).ToNot(HaveOccurred())
            Expect(businessValidation.Passed).To(BeTrue())

            By("Validating system recovery and stability")
            Eventually(func() bool {
                return testApp.IsHealthy(ctx)
            }, 2*time.Minute, 10*time.Second).Should(BeTrue())

            // Verify no resource oscillation
            oscillationCheck, err := e2eSuite.CheckResourceOscillation(ctx, testApp, 5*time.Minute)
            Expect(err).ToNot(HaveOccurred())
            Expect(oscillationCheck.Oscillating).To(BeFalse())

            By("Generating test report with business metrics")
            testReport := &shared.TestReport{
                TestCase:         "Use Case #1: Pod Resource Exhaustion Recovery",
                BusinessRequirement: "BR-AI-001",
                ExecutionTime:    chaosResult.Duration,
                BusinessOutcome:  chaosResult.BusinessOutcome,
                Validation:       businessValidation,
                Recommendations:  businessValidation.Recommendations,
            }

            err = e2eSuite.SaveTestReport(ctx, testReport)
            Expect(err).ToNot(HaveOccurred())
        })

        It("should handle concurrent memory pressure across multiple pods", func() {
            By("Deploying multiple memory-intensive applications")
            apps := []*shared.TestApplication{}
            for i := 0; i < 3; i++ {
                app, err := e2eSuite.DeployTestApplication(ctx, &shared.TestApplicationConfig{
                    Name:      fmt.Sprintf("memory-app-%d", i),
                    Namespace: testNamespace,
                    Type:      "memory-intensive",
                    Replicas:  2,
                })
                Expect(err).ToNot(HaveOccurred())
                apps = append(apps, app)
            }

            By("Executing concurrent chaos experiments")
            var chaosResults []*chaos.ChaosResult
            for i, app := range apps {
                chaosExperiment := &chaos.ChaosExperiment{
                    ID:              fmt.Sprintf("uc1-concurrent-memory-%d", i),
                    Name:            fmt.Sprintf("concurrent-memory-pressure-%d", i),
                    Type:            "pod-memory-hog",
                    TargetNamespace: testNamespace,
                    Duration:        5 * time.Minute,
                    Parameters: map[string]interface{}{
                        "MEMORY_CONSUMPTION": "85%",
                        "TARGET_PODS":        app.PodSelector(),
                    },
                }

                result, err := e2eSuite.ChaosEngine.ExecuteExperiment(ctx, chaosExperiment)
                Expect(err).ToNot(HaveOccurred())
                chaosResults = append(chaosResults, result)
            }

            By("Validating AI handles concurrent scenarios effectively")
            // Verify AI correlates related alerts
            alertCorrelation, err := e2eSuite.GetAlertCorrelation(ctx, testNamespace)
            Expect(err).ToNot(HaveOccurred())
            Expect(alertCorrelation.CorrelatedGroups).To(HaveLen(BeNumerically(">=", 1)))

            // Verify efficient resource allocation across apps
            resourceAllocation, err := e2eSuite.GetResourceAllocation(ctx, testNamespace)
            Expect(err).ToNot(HaveOccurred())
            Expect(resourceAllocation.EfficiencyScore).To(BeNumerically(">=", 0.85))
        })
    })

    Context("Performance and Scalability Validation", func() {
        It("should maintain performance under high alert volume", func() {
            By("Generating high-volume alert scenario")
            alertGenerator := e2eSuite.GetAlertGenerator()

            // Generate 100 alerts/minute for 10 minutes
            alertScenario, err := alertGenerator.GenerateAlertScenario("high-volume-memory", 10*time.Minute)
            Expect(err).ToNot(HaveOccurred())

            By("Monitoring AI performance under load")
            performanceMonitor, err := e2eSuite.StartPerformanceMonitoring(ctx, &shared.PerformanceMonitorConfig{
                MetricsInterval: 30 * time.Second,
                Namespace:       testNamespace,
            })
            Expect(err).ToNot(HaveOccurred())
            defer performanceMonitor.Stop()

            By("Executing alert scenario")
            err = alertGenerator.ExecuteScenario(ctx, alertScenario)
            Expect(err).ToNot(HaveOccurred())

            By("Validating performance metrics")
            performanceResult := performanceMonitor.GetResults()
            Expect(performanceResult.AverageResponseTime).To(BeNumerically("<", 30*time.Second))
            Expect(performanceResult.ThroughputAlertsPerMinute).To(BeNumerically(">=", 95))
            Expect(performanceResult.AccuracyRate).To(BeNumerically(">=", 0.90))
        })
    })
})
```

### **Step 2.2: Create Chaos Experiment Templates**

#### **LitmusChaos Templates**

```yaml
# File: test/e2e/chaos-experiments/pod-memory-hog-template.yaml
apiVersion: litmuschaos.io/v1alpha1
kind: ChaosEngine
metadata:
  name: pod-memory-hog-{{.TestID}}
  namespace: {{.Namespace}}
  labels:
    kubernaut.io/test-case: "{{.TestCase}}"
    kubernaut.io/business-requirement: "{{.BusinessRequirement}}"
spec:
  engineState: 'active'
  chaosServiceAccount: chaos-e2e-runner
  monitoring: true
  experiments:
  - name: pod-memory-hog
    spec:
      components:
        statusCheckTimeouts:
          delay: 2
          timeout: 180
        probe:
        - name: "kubernaut-health-probe"
          type: "httpProbe"
          mode: "Continuous"
          runProperties:
            probeTimeout: 5
            interval: 10
            retry: 3
          httpProbe/inputs:
            url: "http://kubernaut-service.kubernaut-system:8080/health"
            expectedResponseCodes: ["200"]
            method:
              get:
                criteria: ==
                responseCode: "200"

        - name: "business-requirement-validation"
          type: "k8sProbe"
          mode: "EOT"
          runProperties:
            probeTimeout: 10
            interval: 5
            retry: 3
          k8sProbe/inputs:
            group: ""
            version: "v1"
            resource: "pods"
            namespace: "{{.Namespace}}"
            fieldSelector: "status.phase=Running"
            operation: "present"

        env:
        - name: MEMORY_CONSUMPTION
          value: "{{.MemoryConsumption}}"

        - name: TARGET_PODS
          value: "{{.TargetPods}}"

        - name: CHAOS_DURATION
          value: "{{.Duration}}"

        - name: MEMORY_PERCENTAGE
          value: "{{.MemoryPercentage}}"
```

### **Step 2.3: Integration with Existing Makefile**

#### **Add E2E Test Targets**

```makefile
# Add to existing Makefile
.PHONY: test-e2e-use-cases
test-e2e-use-cases: ## Run all E2E use case tests
	@echo "Running E2E use case tests..."
	@echo "Using Kind cluster and oss-gpt:20b model..."
	KUBECONFIG=${KUBECONFIG} \
	LLM_ENDPOINT=http://localhost:8080 \
	LLM_MODEL=gpt-oss:20b \
	LLM_PROVIDER=localai \
	go test -v -tags=e2e ./test/e2e/use_cases/... -timeout=60m

.PHONY: test-e2e-chaos
test-e2e-chaos: ## Run E2E chaos engineering tests
	@echo "Running E2E chaos engineering tests..."
	@echo "Deploying LitmusChaos if needed..."
	./scripts/setup-chaos-testing.sh
	KUBECONFIG=${KUBECONFIG} \
	CHAOS_NAMESPACE=chaos-testing \
	go test -v -tags=e2e,chaos ./test/e2e/use_cases/... -run TestChaos -timeout=90m

.PHONY: test-e2e-stress
test-e2e-stress: ## Run AI model stress tests
	@echo "Running AI model stress tests..."
	./scripts/validate-localai-integration.sh
	KUBECONFIG=${KUBECONFIG} \
	LLM_ENDPOINT=http://localhost:8080 \
	LLM_MODEL=gpt-oss:20b \
	go test -v -tags=e2e,stress ./test/e2e/use_cases/... -run TestStress -timeout=45m

.PHONY: setup-e2e-environment
setup-e2e-environment: ## Setup complete E2E testing environment
	@echo "Setting up E2E testing environment..."
	./scripts/setup-e2e-environment.sh
	@echo "E2E environment ready for testing"

.PHONY: cleanup-e2e-environment
cleanup-e2e-environment: ## Cleanup E2E testing environment
	@echo "Cleaning up E2E testing environment..."
	./scripts/cleanup-e2e-environment.sh
	@echo "E2E environment cleaned up"
```

---

## 🔧 **Phase 3: Advanced Implementation (Weeks 5-6)**

### **Step 3.1: Business Outcome Reporting**

#### **Create Business Report Generator**

```go
// File: pkg/reporting/business_report.go
package reporting

import (
    "context"
    "fmt"
    "time"

    "github.com/jordigilh/kubernaut/pkg/validation"
)

type BusinessReportGenerator struct {
    config    *ReportConfig
    validator *validation.BusinessMetricsValidator
    logger    *logrus.Logger
}

type BusinessStakeholderReport struct {
    ExecutionSummary    ExecutionSummary             `json:"execution_summary"`
    BusinessOutcomes    map[string]BusinessOutcome   `json:"business_outcomes"`
    PerformanceMetrics  PerformanceReport            `json:"performance_metrics"`
    RecommendedActions  []RecommendedAction          `json:"recommended_actions"`
    RiskAssessment      RiskAssessment               `json:"risk_assessment"`
    ROIAnalysis         ROIAnalysis                  `json:"roi_analysis"`
}

type ExecutionSummary struct {
    TotalTestCases       int           `json:"total_test_cases"`
    PassedTestCases      int           `json:"passed_test_cases"`
    FailedTestCases      int           `json:"failed_test_cases"`
    ExecutionDuration    time.Duration `json:"execution_duration"`
    BusinessValueScore   float64       `json:"business_value_score"`
    ReadinessAssessment  string        `json:"readiness_assessment"`
    CoveragePercentage   float64       `json:"coverage_percentage"`
}

type ROIAnalysis struct {
    EstimatedTimeSavings    time.Duration `json:"estimated_time_savings"`
    EstimatedCostReduction  float64       `json:"estimated_cost_reduction"`
    IncidentPreventionRate  float64       `json:"incident_prevention_rate"`
    OperationalEfficiency   float64       `json:"operational_efficiency"`
    BusinessValueCreated    float64       `json:"business_value_created"`
}

func (r *BusinessReportGenerator) GenerateStakeholderReport(
    ctx context.Context,
    testResults []validation.ValidationResult,
) (*BusinessStakeholderReport, error) {
    report := &BusinessStakeholderReport{
        ExecutionSummary:   r.generateExecutionSummary(testResults),
        BusinessOutcomes:   r.aggregateBusinessOutcomes(testResults),
        PerformanceMetrics: r.aggregatePerformanceMetrics(testResults),
    }

    // Generate actionable recommendations
    report.RecommendedActions = r.generateRecommendations(report)
    report.RiskAssessment = r.assessRisks(report)
    report.ROIAnalysis = r.calculateROI(report)

    // Determine readiness assessment
    report.ExecutionSummary.ReadinessAssessment = r.determineReadiness(report)

    return report, nil
}

func (r *BusinessReportGenerator) generateRecommendations(
    report *BusinessStakeholderReport,
) []RecommendedAction {
    recommendations := []RecommendedAction{}

    // Analyze performance metrics for recommendations
    if report.PerformanceMetrics.AverageResponseTime > 30*time.Second {
        recommendations = append(recommendations, RecommendedAction{
            Priority:    "High",
            Category:    "Performance",
            Action:      "Optimize AI model response time",
            Description: "AI response time exceeds SLA threshold of 30 seconds",
            EstimatedImpact: "20% improvement in incident response time",
            Timeline:    "2 weeks",
        })
    }

    if report.ExecutionSummary.BusinessValueScore < 0.85 {
        recommendations = append(recommendations, RecommendedAction{
            Priority:    "Medium",
            Category:    "Business Value",
            Action:      "Enhance AI decision accuracy",
            Description: "Business value score below target threshold",
            EstimatedImpact: "15% improvement in recommendation accuracy",
            Timeline:    "3 weeks",
        })
    }

    // Check for failed business requirements
    for requirementID, outcome := range report.BusinessOutcomes {
        if !outcome.RequirementMet {
            recommendations = append(recommendations, RecommendedAction{
                Priority:    "Critical",
                Category:    "Business Requirement",
                Action:      fmt.Sprintf("Address failing requirement %s", requirementID),
                Description: outcome.FailureReason,
                EstimatedImpact: outcome.BusinessImpact,
                Timeline:    "1 week",
            })
        }
    }

    return recommendations
}

func (r *BusinessReportGenerator) determineReadiness(
    report *BusinessStakeholderReport,
) string {
    passRate := float64(report.ExecutionSummary.PassedTestCases) /
               float64(report.ExecutionSummary.TotalTestCases)

    businessValueScore := report.ExecutionSummary.BusinessValueScore

    if passRate >= 0.95 && businessValueScore >= 0.90 {
        return "Production Ready"
    } else if passRate >= 0.85 && businessValueScore >= 0.80 {
        return "Pre-Production Ready"
    } else if passRate >= 0.70 && businessValueScore >= 0.70 {
        return "Development Complete"
    } else {
        return "Additional Development Required"
    }
}
```

### **Step 3.2: Automated Environment Setup Scripts**

#### **Create Environment Setup Script**

```bash
#!/bin/bash
# File: scripts/setup-e2e-environment.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Configuration
CLUSTER_SIZE="${CLUSTER_SIZE:-medium}"
CHAOS_ENABLED="${CHAOS_ENABLED:-true}"
MONITORING_STACK="${MONITORING_STACK:-full}"
AI_MODEL="${AI_MODEL:-gpt-oss:20b}"
VECTOR_DB="${VECTOR_DB:-postgresql}"
TEST_DATA="${TEST_DATA:-realistic}"
DURATION="${DURATION:-8h}"
AUTO_CLEANUP="${AUTO_CLEANUP:-true}"

echo "🚀 Setting up Kubernaut E2E Testing Environment"
echo "================================================="
echo "Cluster Size: $CLUSTER_SIZE"
echo "AI Model: $AI_MODEL"
echo "Duration: $DURATION"
echo ""

# Step 1: Validate prerequisites
echo "📋 Step 1: Validating prerequisites..."
"${SCRIPT_DIR}/validate-e2e-prerequisites.sh"

# Step 2: Setup Kind cluster if needed
echo "🏗️  Step 2: Setting up Kind cluster..."
if ! oc cluster-info >/dev/null 2>&1; then
    echo "Setting up Kind cluster with KCLI..."
    cd "${PROJECT_ROOT}/docs/development/e2e-testing"
    ./deploy-kcli-cluster.sh kubernaut-e2e kcli-baremetal-params.yml

    # Wait for cluster to be ready
    echo "Waiting for cluster to be ready..."
    timeout 1800 bash -c 'until oc get nodes | grep -q "Ready"; do sleep 30; done'
fi

# Step 3: Install LitmusChaos
if [ "$CHAOS_ENABLED" = "true" ]; then
    echo "🌪️  Step 3: Installing LitmusChaos..."
    "${SCRIPT_DIR}/setup-litmus-chaos.sh"
fi

# Step 4: Setup monitoring stack
echo "📊 Step 4: Setting up monitoring stack..."
"${SCRIPT_DIR}/setup-monitoring-stack.sh" --type "$MONITORING_STACK"

# Step 5: Deploy vector database
echo "🗄️  Step 5: Setting up vector database..."
"${SCRIPT_DIR}/setup-vector-database.sh" --type "$VECTOR_DB"

# Step 6: Validate AI model connectivity
echo "🤖 Step 6: Validating AI model connectivity..."
"${SCRIPT_DIR}/validate-localai-integration.sh"

# Step 7: Setup test data
echo "📊 Step 7: Setting up test data..."
"${SCRIPT_DIR}/setup-test-data.sh" --type "$TEST_DATA"

# Step 8: Create test namespaces and RBAC
echo "🔐 Step 8: Setting up test namespaces and RBAC..."
"${SCRIPT_DIR}/setup-test-rbac.sh"

# Step 9: Deploy kubernaut for testing
echo "🚀 Step 9: Deploying kubernaut..."
make deploy-test-environment

# Step 10: Validate complete setup
echo "✅ Step 10: Validating complete setup..."
"${SCRIPT_DIR}/validate-e2e-setup.sh"

echo ""
echo "🎉 E2E Testing Environment Setup Complete!"
echo "=========================================="
echo "Cluster Status: $(oc get nodes --no-headers | wc -l) nodes ready"
echo "AI Model: $AI_MODEL at localhost:8080"
echo "Vector DB: $VECTOR_DB ready"
echo "Chaos Testing: $([ "$CHAOS_ENABLED" = "true" ] && echo "Enabled" || echo "Disabled")"
echo ""
echo "🧪 Ready for E2E testing!"
echo "Run: make test-e2e-use-cases"
echo ""

# Setup auto-cleanup if requested
if [ "$AUTO_CLEANUP" = "true" ]; then
    echo "⏰ Auto-cleanup scheduled for $DURATION"
    (sleep "${DURATION//h/*3600}" && "${SCRIPT_DIR}/cleanup-e2e-environment.sh") &
    echo "Cleanup PID: $!"
fi
```

#### **Create LitmusChaos Setup Script**

```bash
#!/bin/bash
# File: scripts/setup-litmus-chaos.sh

set -e

echo "🌪️  Installing LitmusChaos for E2E Testing"
echo "==========================================="

# Create chaos testing namespace
oc create namespace chaos-testing --dry-run=client -o yaml | oc apply -f -

# Install LitmusChaos operator
echo "Installing LitmusChaos operator..."
oc apply -f https://litmuschaos.github.io/litmus/3.0.0/litmus-3.0.0.yaml

# Wait for operator to be ready
echo "Waiting for LitmusChaos operator to be ready..."
oc wait --for=condition=Ready pod -l app.kubernetes.io/name=litmus -n litmus --timeout=300s

# Create chaos testing RBAC
cat <<EOF | oc apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: chaos-e2e-runner
  namespace: chaos-testing
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: chaos-e2e-runner
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints", "persistentvolumeclaims", "events", "configmaps", "secrets", "nodes"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "daemonsets", "replicasets", "statefulsets"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["litmuschaos.io"]
  resources: ["chaosengines", "chaosexperiments", "chaosresults"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: chaos-e2e-runner
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: chaos-e2e-runner
subjects:
- kind: ServiceAccount
  name: chaos-e2e-runner
  namespace: chaos-testing
EOF

# Install chaos experiments
echo "Installing chaos experiments..."
oc apply -f https://hub.litmuschaos.io/api/chaos/3.0.0?file=charts/generic/experiments.yaml -n chaos-testing

# Validate installation
echo "Validating LitmusChaos installation..."
oc get pods -n litmus
oc get chaosexperiments -n chaos-testing

echo "✅ LitmusChaos installation complete!"
```

### **Step 3.3: Continuous Integration Integration**

#### **Create GitHub Actions Workflow**

```yaml
# File: .github/workflows/e2e-testing.yml
name: E2E Testing

on:
  schedule:
    # Run e2e tests daily at 2 AM UTC
    - cron: '0 2 * * *'
  workflow_dispatch:
    inputs:
      test_suite:
        description: 'Test suite to run'
        required: true
        default: 'all'
        type: choice
        options:
        - all
        - use-cases
        - chaos
        - stress
      duration:
        description: 'Test duration'
        required: true
        default: '2h'
        type: string

env:
  KUBECONFIG: /tmp/kubeconfig
  LLM_ENDPOINT: http://localhost:8080
  LLM_MODEL: gpt-oss:20b
  LLM_PROVIDER: localai

jobs:
  setup-environment:
    runs-on: self-hosted
    outputs:
      cluster-ready: ${{ steps.setup.outputs.cluster-ready }}
    steps:
    - name: Checkout code
      uses: actions/checkout@v4

    - name: Setup Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'

    - name: Setup E2E environment
      id: setup
      run: |
        ./scripts/setup-e2e-environment.sh
        echo "cluster-ready=true" >> $GITHUB_OUTPUT

    - name: Validate environment
      run: |
        make validate-e2e-environment

  e2e-use-cases:
    needs: setup-environment
    if: ${{ needs.setup-environment.outputs.cluster-ready == 'true' && (github.event.inputs.test_suite == 'all' || github.event.inputs.test_suite == 'use-cases') }}
    runs-on: self-hosted
    strategy:
      matrix:
        use-case: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
    steps:
    - name: Run Use Case ${{ matrix.use-case }}
      run: |
        make test-e2e-use-case-${{ matrix.use-case }}

    - name: Upload test results
      uses: actions/upload-artifact@v3
      if: always()
      with:
        name: use-case-${{ matrix.use-case }}-results
        path: test-results/use-case-${{ matrix.use-case }}/

  e2e-chaos-testing:
    needs: setup-environment
    if: ${{ needs.setup-environment.outputs.cluster-ready == 'true' && (github.event.inputs.test_suite == 'all' || github.event.inputs.test_suite == 'chaos') }}
    runs-on: self-hosted
    steps:
    - name: Run chaos engineering tests
      run: |
        make test-e2e-chaos

    - name: Upload chaos test results
      uses: actions/upload-artifact@v3
      if: always()
      with:
        name: chaos-test-results
        path: test-results/chaos/

  ai-stress-testing:
    needs: setup-environment
    if: ${{ needs.setup-environment.outputs.cluster-ready == 'true' && (github.event.inputs.test_suite == 'all' || github.event.inputs.test_suite == 'stress') }}
    runs-on: self-hosted
    steps:
    - name: Run AI model stress tests
      run: |
        make test-e2e-stress

    - name: Upload stress test results
      uses: actions/upload-artifact@v3
      if: always()
      with:
        name: stress-test-results
        path: test-results/stress/

  generate-business-report:
    needs: [e2e-use-cases, e2e-chaos-testing, ai-stress-testing]
    if: always()
    runs-on: self-hosted
    steps:
    - name: Download all test results
      uses: actions/download-artifact@v3

    - name: Generate business stakeholder report
      run: |
        make generate-business-report

    - name: Upload business report
      uses: actions/upload-artifact@v3
      with:
        name: business-stakeholder-report
        path: reports/business-report.html

  cleanup:
    needs: [e2e-use-cases, e2e-chaos-testing, ai-stress-testing, generate-business-report]
    if: always()
    runs-on: self-hosted
    steps:
    - name: Cleanup E2E environment
      run: |
        ./scripts/cleanup-e2e-environment.sh
```

---

## 📊 **Success Metrics and Validation**

### **Implementation Success Criteria**

| Phase | Success Metric | Target | Validation Method |
|-------|---------------|--------|------------------|
| **Phase 1** | Foundation Setup | 100% infrastructure ready | Automated environment validation |
| **Phase 2** | Use Case Implementation | 5/10 use cases complete | Business requirement validation |
| **Phase 3** | Advanced Features | 10/10 use cases + reporting | Comprehensive business report |

### **Business Value Validation**

| Business Requirement | Success Criteria | Measurement Method |
|---------------------|------------------|-------------------|
| **BR-AI-001** | 90% AI decision accuracy | Automated accuracy measurement |
| **BR-SAFETY-001** | Zero critical failures | Continuous safety monitoring |
| **BR-MONITOR-002** | 90% alert noise reduction | Alert correlation effectiveness |
| **BR-VDB-001** | Pattern history preservation | Vector database integrity checks |

### **Quality Gates**

Each implementation phase must pass these quality gates:

1. **Code Quality**: All linting and static analysis passes
2. **Test Coverage**: 95%+ business requirement coverage
3. **Performance**: All SLA targets met
4. **Security**: Security validation passes
5. **Documentation**: Complete implementation documentation

---

## 🎯 **Next Steps and Recommendations**

### **Immediate Actions (Week 1)**
1. **Environment Setup**: Follow Phase 1 setup instructions
2. **Stakeholder Review**: Present approach to business stakeholders
3. **Resource Allocation**: Assign development team members
4. **Timeline Confirmation**: Validate 6-week implementation timeline

### **Risk Mitigation**
1. **Dependency Management**: Ensure Kind cluster and AI model availability
2. **Testing Infrastructure**: Validate LitmusChaos compatibility
3. **Performance Baseline**: Establish baseline metrics before implementation
4. **Rollback Planning**: Prepare rollback procedures for each phase

### **Long-term Considerations**
1. **Maintenance Strategy**: Plan for ongoing test maintenance
2. **Scalability Planning**: Design for future use case expansion
3. **Integration Evolution**: Consider CI/CD pipeline integration
4. **Knowledge Transfer**: Document learnings and best practices

This implementation guide provides a comprehensive roadmap for implementing the Top 10 E2E Use Cases while ensuring business outcome validation, development efficiency, and operational excellence. The phased approach allows for incremental delivery and continuous validation throughout the implementation process.
