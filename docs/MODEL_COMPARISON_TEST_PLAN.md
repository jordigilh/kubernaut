# Model Comparison Test Plan

**Date**: December 2024
**Objective**: Evaluate performance and reasoning capabilities of different LLM models for alert analysis
**Base Model**: granite3.1-dense:8b
**Infrastructure**: ramallama (replacing ollama)

## Test Objectives

### Primary Goals
1. **Performance Comparison**: Measure response time and throughput differences
2. **Reasoning Quality**: Evaluate decision accuracy and confidence calibration
3. **Remediation Accuracy**: Compare recommended actions against expected outcomes
4. **Infrastructure Validation**: Verify ramallama integration functionality

### Success Criteria
- Identify optimal model for production deployment based on measured metrics
- Quantify performance trade-offs between response time and accuracy
- Validate ramallama infrastructure as ollama replacement
- Generate quantitative model selection analysis

---

## Test Infrastructure Setup

### Models Under Test
```yaml
models:
  baseline:
    name: "granite3.1-dense:8b"
    status: "production_baseline"
    purpose: "performance_reference"

  candidates:
    - name: "deepseek-coder:7b-instruct"
      focus: "code_reasoning"
      domain: "kubernetes_troubleshooting"

    - name: "granite3.1-steiner:8b"
      focus: "general_reasoning"
      domain: "decision_making"
```

### **Infrastructure Changes**
```bash
# Replace ollama with ramallama in test environment
# Update configuration to use ramallama endpoints
# Maintain same model serving interface for compatibility
```

### **Test Environment Configuration**
```go
// Test configuration for each model
type ModelTestConfig struct {
    ModelName    string `json:"model_name"`
    ServerType   string `json:"server_type"`  // "ramallama"
    Endpoint     string `json:"endpoint"`
    MaxTokens    int    `json:"max_tokens"`   // 500 (consistent)
    Temperature  float64 `json:"temperature"` // 0.3 (consistent)
    Timeout      time.Duration `json:"timeout"` // 30s
}

var TestModels = []ModelTestConfig{
    {
        ModelName: "granite3.1-dense:8b",
        ServerType: "ramallama",
        Endpoint: "http://localhost:11434",
        MaxTokens: 500,
        Temperature: 0.3,
        Timeout: 30 * time.Second,
    },
    {
        ModelName: "deepseek-coder:7b-instruct",
        ServerType: "ramallama",
        Endpoint: "http://localhost:11435",
        MaxTokens: 500,
        Temperature: 0.3,
        Timeout: 30 * time.Second,
    },
    {
        ModelName: "granite3.1-steiner:8b",
        ServerType: "ramallama",
        Endpoint: "http://localhost:11436",
        MaxTokens: 500,
        Temperature: 0.3,
        Timeout: 30 * time.Second,
    },
}
```

---

## 📋 **Test Scenarios**

### **Selected Integration Test Subset**

Use existing integration tests that represent real-world alert scenarios:

#### **1. Memory Alert Scenarios**
```go
// test/integration/memory_alerts_test.go
var MemoryAlertTestCases = []AlertTestCase{
    {
        Name: "HighMemoryUsage_WebApp_Production",
        Alert: types.Alert{
            Name: "HighMemoryUsage",
            Namespace: "production",
            Labels: map[string]string{
                "app": "webapp",
                "severity": "warning",
            },
        },
        ExpectedAction: "increase_resources",
        ExpectedConfidence: 0.85,
        Reasoning: "Memory pressure should trigger resource increase",
    },
    {
        Name: "MemoryLeak_Suspected_CrashLoop",
        Alert: types.Alert{
            Name: "PodCrashLoopBackOff",
            Namespace: "production",
            Labels: map[string]string{
                "app": "api-service",
                "reason": "OutOfMemory",
            },
        },
        ExpectedAction: "restart_pod",
        ExpectedConfidence: 0.75,
        Reasoning: "Memory leak suspected, restart to clear state",
    },
}
```

#### **2. CPU Alert Scenarios**
```go
var CPUAlertTestCases = []AlertTestCase{
    {
        Name: "HighCPUUsage_Scaling_Needed",
        Alert: types.Alert{
            Name: "HighCPUUsage",
            Namespace: "production",
            Labels: map[string]string{
                "app": "worker",
                "cpu_utilization": "90%",
            },
        },
        ExpectedAction: "scale_deployment",
        ExpectedConfidence: 0.8,
        Reasoning: "High CPU suggests need for horizontal scaling",
    },
}
```

#### **3. Storage Alert Scenarios**
```go
var StorageAlertTestCases = []AlertTestCase{
    {
        Name: "DiskSpaceLow_CleanupNeeded",
        Alert: types.Alert{
            Name: "DiskSpaceLow",
            Namespace: "production",
            Labels: map[string]string{
                "volume": "/var/logs",
                "usage": "85%",
            },
        },
        ExpectedAction: "cleanup_storage",
        ExpectedConfidence: 0.9,
        Reasoning: "Clear disk space before critical threshold",
    },
}
```

#### **4. Network Alert Scenarios**
```go
var NetworkAlertTestCases = []AlertTestCase{
    {
        Name: "NetworkConnectivity_ServiceMesh",
        Alert: types.Alert{
            Name: "ServiceUnavailable",
            Namespace: "production",
            Labels: map[string]string{
                "service": "payment-api",
                "error": "connection_refused",
            },
        },
        ExpectedAction: "restart_pod",
        ExpectedConfidence: 0.7,
        Reasoning: "Network connectivity issues often resolve with restart",
    },
}
```

### **Total Test Cases**: 10-15 representative scenarios

---

## 📊 **Evaluation Metrics**

### **Performance Metrics**
```go
type PerformanceMetrics struct {
    ResponseTime    PerformanceData `json:"response_time"`
    Throughput      PerformanceData `json:"throughput"`
    ResourceUsage   ResourceData    `json:"resource_usage"`
    ErrorRate       float64         `json:"error_rate"`
}

type PerformanceData struct {
    Mean    time.Duration `json:"mean"`
    P50     time.Duration `json:"p50"`
    P95     time.Duration `json:"p95"`
    P99     time.Duration `json:"p99"`
    Min     time.Duration `json:"min"`
    Max     time.Duration `json:"max"`
    StdDev  time.Duration `json:"stddev"`
}

type ResourceData struct {
    CPUUsage    float64 `json:"cpu_usage_percent"`
    MemoryUsage int64   `json:"memory_usage_mb"`
    TokensPerSec float64 `json:"tokens_per_second"`
}
```

### **Reasoning Quality Metrics**
```go
type ReasoningMetrics struct {
    ActionAccuracy      float64            `json:"action_accuracy"`       // % correct actions
    ConfidenceCalibration float64          `json:"confidence_calibration"` // How well confidence matches accuracy
    ReasoningQuality    ReasoningScores    `json:"reasoning_quality"`
    ConsistencyScore    float64            `json:"consistency_score"`     // Same input → same output
}

type ReasoningScores struct {
    Relevance       float64 `json:"relevance"`        // Reasoning mentions relevant factors
    Completeness    float64 `json:"completeness"`     // Covers key decision factors
    Clarity         float64 `json:"clarity"`          // Clear, understandable explanation
    TechnicalDepth  float64 `json:"technical_depth"`  // Demonstrates K8s understanding
}
```

### **Comparison Dashboard**
```yaml
# Expected output format for results
model_comparison_results:
  granite3.1-dense:8b:
    performance:
      response_time_p95: "1.94s"
      throughput: "15.2 requests/min"
      cpu_usage: "45%"
      memory_usage: "2.1GB"
    reasoning:
      action_accuracy: "94.4%"
      confidence_calibration: "0.87"
      reasoning_quality: "8.2/10"
      consistency_score: "0.92"

  deepseek-coder:7b-instruct:
    performance:
      response_time_p95: "TBD"
      throughput: "TBD"
      cpu_usage: "TBD"
      memory_usage: "TBD"
    reasoning:
      action_accuracy: "TBD"
      confidence_calibration: "TBD"
      reasoning_quality: "TBD"
      consistency_score: "TBD"
```

---

## 🔬 **Test Implementation**

### **Test Runner Structure**
```go
// test/integration/model_comparison/model_comparison_test.go
package model_comparison_test

import (
    "context"
    "testing"
    "time"

    "github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
    "github.com/jordigilh/prometheus-alerts-slm/pkg/types"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Model Comparison Tests", func() {
    var (
        testCases []AlertTestCase
        models    []ModelTestConfig
        results   map[string]*ModelResults
    )

    BeforeEach(func() {
        testCases = LoadTestScenarios()
        models = LoadModelConfigurations()
        results = make(map[string]*ModelResults)
    })

    // Run each test case against each model
    Context("Performance and Reasoning Evaluation", func() {
        for _, model := range models {
            modelName := model.ModelName

            Context(fmt.Sprintf("Model: %s", modelName), func() {
                var client slm.Client

                BeforeEach(func() {
                    var err error
                    client, err = slm.NewClientWithConfig(model.ToSLMConfig(), logger)
                    Expect(err).NotTo(HaveOccurred())
                    Expect(client.IsHealthy()).To(BeTrue())
                })

                for _, testCase := range testCases {
                    testCaseName := testCase.Name

                    It(fmt.Sprintf("should handle %s correctly", testCaseName), func() {
                        // Run test case and collect metrics
                        result := RunTestCase(client, testCase)

                        // Store results for comparison
                        if results[modelName] == nil {
                            results[modelName] = &ModelResults{}
                        }
                        results[modelName].AddTestResult(testCaseName, result)

                        // Basic validation
                        Expect(result.Error).To(BeNil())
                        Expect(result.Recommendation).NotTo(BeNil())
                        Expect(result.ResponseTime).To(BeNumerically("<", 30*time.Second))
                    })
                }
            })
        }
    })

    AfterSuite(func() {
        // Generate comparison report
        GenerateComparisonReport(results)
        ExportMetricsToJSON(results)
    })
})

func RunTestCase(client slm.Client, testCase AlertTestCase) *TestResult {
    startTime := time.Now()

    recommendation, err := client.AnalyzeAlert(context.Background(), testCase.Alert)

    return &TestResult{
        TestCase:       testCase,
        Recommendation: recommendation,
        ResponseTime:   time.Since(startTime),
        Error:          err,
        Timestamp:      startTime,
    }
}
```

### **Metrics Collection**
```go
type MetricsCollector struct {
    performanceTracker *PerformanceTracker
    reasoningEvaluator *ReasoningEvaluator
}

func (m *MetricsCollector) EvaluateTestResult(result *TestResult) *EvaluationScore {
    return &EvaluationScore{
        Performance: m.performanceTracker.Analyze(result),
        Reasoning:   m.reasoningEvaluator.Score(result),
        Accuracy:    m.calculateAccuracy(result),
    }
}

func (m *MetricsCollector) calculateAccuracy(result *TestResult) float64 {
    expected := result.TestCase.ExpectedAction
    actual := result.Recommendation.Action

    if expected == actual {
        return 1.0
    }

    // Partial credit for reasonable alternatives
    if isReasonableAlternative(expected, actual) {
        return 0.7
    }

    return 0.0
}
```

---

## 🚀 **Execution Plan**

### **Phase 1: Infrastructure Setup (Week 1)**
```bash
# Day 1-2: ramallama Setup
- Install and configure ramallama
- Download and configure test models
- Validate ramallama endpoints working

# Day 3-4: Test Infrastructure
- Create model comparison test structure
- Implement metrics collection framework
- Set up test data and scenarios

# Day 5: Validation
- Run smoke tests with granite3.1-dense:8b
- Verify all test cases execute successfully
- Confirm metrics collection working
```

### **Phase 2: Model Testing (Week 2)**
```bash
# Day 1-2: Baseline Testing
- Run complete test suite with granite3.1-dense:8b
- Establish performance and accuracy baselines
- Debug any test infrastructure issues

# Day 3-4: Alternative Model Testing
- Run tests with deepseek-coder:7b-instruct
- Run tests with granite3.1-steiner:8b
- Collect comprehensive metrics for all models

# Day 5: Analysis
- Compare results across all models
- Generate performance and reasoning reports
- Identify best performing model
```

### **Phase 3: Analysis & Recommendation (Week 3)**
```bash
# Day 1-2: Data Analysis
- Statistical analysis of performance differences
- Reasoning quality comparison
- Cost-benefit analysis

# Day 3-4: Report Generation
- Create comprehensive comparison report
- Generate recommendations for production
- Document ramallama migration benefits

# Day 5: Decision & Next Steps
- Present findings to team
- Decide on production model
- Plan production migration if needed
```

---

## 📈 **Expected Deliverables**

### **1. Performance Comparison Report**
```yaml
# docs/MODEL_PERFORMANCE_COMPARISON.md
sections:
  - executive_summary
  - methodology
  - performance_results
  - reasoning_analysis
  - recommendations
  - migration_plan
```

### **2. Test Results Data**
```json
// results/model_comparison_results.json
{
  "test_run_metadata": {
    "date": "2024-12-XX",
    "test_cases": 15,
    "runs_per_model": 3,
    "infrastructure": "ramallama"
  },
  "model_results": {
    "granite3.1-dense:8b": { /* detailed metrics */ },
    "deepseek-coder:7b-instruct": { /* detailed metrics */ },
    "granite3.1-steiner:8b": { /* detailed metrics */ }
  },
  "comparison_summary": { /* relative performance */ }
}
```

### **3. ramallama Migration Guide**
```yaml
# docs/RAMALLAMA_MIGRATION.md
sections:
  - installation_guide
  - configuration_changes
  - performance_improvements
  - troubleshooting_guide
```

### **4. Updated Integration Tests**
```go
// Enhanced test suite supporting multiple models
// Reusable test infrastructure for future model evaluations
// Automated performance benchmarking capabilities
```

---

## 🎯 **Success Metrics**

### **Quantitative Goals**
- **Performance Baseline**: Establish granite3.1-dense:8b metrics as reference
- **Speed Improvement**: Identify if any model provides >20% response time improvement
- **Accuracy Maintenance**: Ensure alternative models maintain >90% accuracy
- **Resource Efficiency**: Document memory and CPU usage differences

### **Qualitative Assessment**
- **Reasoning Quality**: Evaluate technical depth and clarity of explanations
- **Consistency**: Measure response consistency across multiple runs
- **Edge Case Handling**: Assess behavior on complex or ambiguous scenarios

### **Decision Framework**
```yaml
model_selection_criteria:
  performance_weight: 40%    # Response time, throughput
  accuracy_weight: 35%       # Correct actions, confidence calibration
  reasoning_weight: 15%      # Quality of explanations
  resource_usage_weight: 10% # Memory and CPU efficiency

minimum_thresholds:
  accuracy: "> 90%"
  response_time_p95: "< 5s"
  reasoning_quality: "> 7/10"
```

---

## 🔧 **Implementation Notes**

### **Test Configuration**
- Run each test case **3 times** per model for statistical significance
- Use **consistent parameters** (temperature, max_tokens) across models
- **Randomize test case order** to avoid bias
- **Monitor resource usage** during testing

### **Infrastructure Requirements**
- **3 ramallama instances** (one per model)
- **Sufficient GPU/CPU** resources for parallel model serving
- **Metrics collection** and storage infrastructure
- **Automated test execution** and reporting

### **Risk Mitigation**
- **Fallback plan**: Keep ollama configuration as backup
- **Resource monitoring**: Prevent resource exhaustion during testing
- **Data validation**: Verify test results are meaningful and comparable
- **Time management**: Limit test execution time per model

---

**Next Steps**: This test plan provides a comprehensive but manageable approach to evaluating model performance with ramallama. The focus on existing integration test scenarios ensures realistic evaluation while keeping scope controlled.

*Ready to implement when infrastructure migration and model comparison priorities align with development schedule.*
