# Effectiveness Monitor Service - Integration Points

**Version**: 1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service (Assessment & Analysis)
**Port**: 8080 (REST + Health), 9090 (Metrics)

---

## 🔗 Upstream Clients (Services Calling Effectiveness Monitor)

### **1. Context API Service** (Port 8080)

**Use Case**: Retrieve effectiveness assessments for historical intelligence

```go
// pkg/context/effectiveness_client.go
package context

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    "go.uber.org/zap"
)

func (c *ContextAPIService) GetEffectivenessAssessment(ctx context.Context, actionID string) (*EffectivenessData, error) {
    url := fmt.Sprintf("http://effectiveness-monitor-service:8080/api/v1/assess/effectiveness/%s", actionID)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getServiceAccountToken()))

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        c.logger.Error("Failed to call Effectiveness Monitor",
            zap.Error(err),
            zap.String("action_id", actionID),
        )
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("effectiveness monitor returned status %d", resp.StatusCode)
    }

    var assessment EffectivenessData
    if err := json.NewDecoder(resp.Body).Decode(&assessment); err != nil {
        return nil, err
    }

    return &assessment, nil
}
```

---

### **2. HolmesGPT API Service** (Port 8090)

**Use Case**: Include effectiveness context in AI investigations

```python
# holmesgpt-api/effectiveness_integration.py
from typing import Dict
import requests


def get_effectiveness_assessment(action_id: str) -> Dict:
    """Retrieve effectiveness assessment for given action."""
    headers = {"Authorization": f"Bearer {get_service_account_token()}"}

    response = requests.get(
        f"http://effectiveness-monitor-service:8080/api/v1/assess/effectiveness/{action_id}",
        headers=headers,
        timeout=10
    )

    if response.status_code == 200:
        return response.json()
    elif response.status_code == 404:
        # Action not yet assessed
        return {"status": "pending_assessment"}
    else:
        raise Exception(f"Effectiveness Monitor returned {response.status_code}")
```

---

## 🔽 Downstream Dependencies (External Services)

### **1. Data Storage Service** (Port 8085)

**Purpose**: Action history retrieval, assessment result persistence

#### **Action History Retrieval**

```go
// pkg/effectiveness/data_storage_client.go
package effectiveness

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "go.uber.org/zap"
)

type DataStorageClient struct {
    baseURL       string
    httpClient    *http.Client
    logger        *zap.Logger
    serviceToken  string
}

func NewDataStorageClient(baseURL string, logger *zap.Logger) *DataStorageClient {
    return &DataStorageClient{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
        logger: logger,
    }
}

func (c *DataStorageClient) GetActionHistory(ctx context.Context, actionType string, window time.Duration) ([]ActionHistory, error) {
    url := fmt.Sprintf("%s/api/v1/audit/actions?action_type=%s&time_range=%s",
        c.baseURL, actionType, window.String())

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceToken))

    resp, err := c.httpClient.Do(req)
    if err != nil {
        c.logger.Error("Failed to retrieve action history from Data Storage",
            zap.Error(err),
            zap.String("action_type", actionType),
        )
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("data storage returned status %d", resp.StatusCode)
    }

    var history []ActionHistory
    if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
        return nil, err
    }

    c.logger.Debug("Retrieved action history",
        zap.String("action_type", actionType),
        zap.Int("count", len(history)),
    )

    return history, nil
}

func (c *DataStorageClient) GetOldestAction(ctx context.Context) (*ActionHistory, error) {
    url := fmt.Sprintf("%s/api/v1/audit/actions/oldest", c.baseURL)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceToken))

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("data storage returned status %d", resp.StatusCode)
    }

    var action ActionHistory
    if err := json.NewDecoder(resp.Body).Decode(&action); err != nil {
        return nil, err
    }

    return &action, nil
}
```

#### **Assessment Result Persistence**

```go
func (c *DataStorageClient) PersistAssessment(ctx context.Context, assessment *EffectivenessScore) error {
    url := fmt.Sprintf("%s/api/v1/audit/effectiveness", c.baseURL)

    payload, err := json.Marshal(assessment)
    if err != nil {
        return err
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payload))
    if err != nil {
        return err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceToken))
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        c.logger.Error("Failed to persist assessment to Data Storage",
            zap.Error(err),
            zap.String("assessment_id", assessment.AssessmentID),
        )
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("data storage returned status %d", resp.StatusCode)
    }

    c.logger.Info("Assessment persisted successfully",
        zap.String("assessment_id", assessment.AssessmentID),
    )

    return nil
}
```

---

### **2. Infrastructure Monitoring Service** (Port 8094)

**Purpose**: Metrics correlation for environmental impact assessment

```go
// pkg/effectiveness/infrastructure_monitoring_client.go
package effectiveness

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "go.uber.org/zap"
)

type InfrastructureMonitoringClient struct {
    baseURL      string
    httpClient   *http.Client
    logger       *zap.Logger
    serviceToken string
}

func NewInfrastructureMonitoringClient(baseURL string, logger *zap.Logger) *InfrastructureMonitoringClient {
    return &InfrastructureMonitoringClient{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
        logger: logger,
    }
}

func (c *InfrastructureMonitoringClient) GetMetricsAfterAction(ctx context.Context, actionID string, window time.Duration) (*EnvironmentalMetrics, error) {
    url := fmt.Sprintf("%s/api/v1/metrics/after-action?action_id=%s&window=%s",
        c.baseURL, actionID, window.String())

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceToken))

    resp, err := c.httpClient.Do(req)
    if err != nil {
        c.logger.Warn("Failed to retrieve metrics from Infrastructure Monitoring",
            zap.Error(err),
            zap.String("action_id", actionID),
        )
        // Graceful degradation: return nil metrics, not an error
        return &EnvironmentalMetrics{}, nil
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        c.logger.Warn("Infrastructure Monitoring returned non-OK status",
            zap.Int("status_code", resp.StatusCode),
            zap.String("action_id", actionID),
        )
        return &EnvironmentalMetrics{}, nil
    }

    var metrics EnvironmentalMetrics
    if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
        c.logger.Error("Failed to decode metrics response",
            zap.Error(err),
        )
        return &EnvironmentalMetrics{}, nil
    }

    c.logger.Debug("Retrieved environmental metrics",
        zap.String("action_id", actionID),
        zap.Float64("memory_improvement", metrics.MemoryImprovement),
        zap.Float64("cpu_impact", metrics.CPUImpact),
    )

    return &metrics, nil
}
```

---

## 📊 Data Flow Diagram

```
┌─────────────────────────────────────────────────┐
│       Upstream Clients                          │
│  ┌────────────┐  ┌────────────┐                │
│  │  Context   │  │ HolmesGPT  │                │
│  │    API     │  │    API     │                │
│  └──────┬─────┘  └──────┬─────┘                │
│         │                │                       │
│         │  Assessment    │  Assessment           │
│         │  Request       │  Request              │
└─────────┼────────────────┼───────────────────────┘
          │ HTTP GET       │ HTTP GET
          │ (Bearer Token) │ (Bearer Token)
          ▼                ▼
┌──────────────────────────────────────────────────┐
│   Effectiveness Monitor Service (Port 8080)      │
│  ┌────────────────────────────────────────────┐ │
│  │ 1. Authentication (TokenReviewer)          │ │
│  │ 2. Check Data Availability (8+ weeks?)     │ │
│  │ 3. Query Action History (Data Storage)     │ │
│  │ 4. Query Metrics (Infrastructure Monitor)  │ │
│  │ 5. Calculate Effectiveness Score           │ │
│  │ 6. Detect Side Effects                     │ │
│  │ 7. Analyze Trends                          │ │
│  │ 8. Persist Assessment (Data Storage)       │ │
│  │ 9. Return Assessment Result                │ │
│  └────────────────────────────────────────────┘ │
└──────┬─────────────────────────────────┬─────────┘
       │ HTTP GET/POST                   │ HTTP GET
       │ (Bearer Token)                  │ (Bearer Token)
       ▼                                 ▼
┌──────────────────┐          ┌──────────────────────────┐
│ Data Storage     │          │ Infrastructure Monitoring│
│ Service          │          │ Service                  │
│ (Port 8085)      │          │ (Port 8094)              │
│                  │          │                          │
│ - Action History │          │ - CPU/Memory Metrics     │
│ - Assessment     │          │ - Network Stability      │
│   Persistence    │          │ - Environmental Impact   │
└──────────────────┘          └──────────────────────────┘
```

---

## 🔄 Request Flow

### **Complete Assessment Request**

```go
// Example: Complete assessment request flow
func (s *EffectivenessMonitorService) AssessEffectiveness(ctx context.Context, req *AssessmentRequest) (*EffectivenessScore, error) {
    // Step 1: Check data availability (8+ weeks required)
    dataWeeks, sufficient := s.checkDataAvailability(ctx)
    if !sufficient {
        return s.insufficientDataResponse(dataWeeks), nil
    }

    // Step 2: Retrieve action history from Data Storage
    history, err := s.dataStorageClient.GetActionHistory(ctx, req.ActionType, 90*24*time.Hour)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve action history: %w", err)
    }

    // Step 3: Calculate traditional effectiveness score
    traditionalScore := s.calculator.CalculateTraditionalScore(history)

    // Step 4: Query metrics from Infrastructure Monitoring (graceful degradation)
    metrics, err := s.infraMonitorClient.GetMetricsAfterAction(ctx, req.ActionID, 10*time.Minute)
    if err != nil {
        s.logger.Warn("Failed to retrieve environmental metrics, continuing with basic assessment",
            zap.Error(err),
        )
        metrics = &EnvironmentalMetrics{} // Default to zero impact
    }

    // Step 5: Detect side effects
    sideEffects, severity := s.calculator.DetectSideEffects(metrics)

    // Step 6: Analyze trends
    trendDirection := s.calculator.AnalyzeTrend(history)

    // Step 7: Generate pattern insights
    patterns := s.calculator.GeneratePatternInsights(history, req.ActionData)

    // Step 8: Calculate confidence
    confidence := s.calculator.CalculateConfidence(history, dataWeeks)

    // Step 9: Build assessment result
    assessment := &EffectivenessScore{
        AssessmentID:        generateAssessmentID(),
        ActionID:            req.ActionID,
        ActionType:          req.ActionType,
        TraditionalScore:    traditionalScore,
        EnvironmentalImpact: *metrics,
        Confidence:          confidence,
        Status:              "assessed",
        SideEffectsDetected: sideEffects,
        SideEffectSeverity:  severity,
        TrendDirection:      trendDirection,
        PatternInsights:     patterns,
        AssessedAt:          time.Now(),
    }

    // Step 10: Persist assessment to Data Storage (best-effort)
    if err := s.dataStorageClient.PersistAssessment(ctx, assessment); err != nil {
        s.logger.Error("Failed to persist assessment, continuing",
            zap.Error(err),
            zap.String("assessment_id", assessment.AssessmentID),
        )
    }

    return assessment, nil
}
```

---

## 🔄 Circuit Breaker Pattern

### **Graceful Degradation for Infrastructure Monitoring**

```go
package effectiveness

import (
    "context"
    "time"

    "go.uber.org/zap"
)

type CircuitBreaker struct {
    failureCount      int
    lastFailureTime   time.Time
    threshold         int
    resetTimeout      time.Duration
    halfOpenRequests  int
    logger            *zap.Logger
}

func NewCircuitBreaker(threshold int, resetTimeout time.Duration, logger *zap.Logger) *CircuitBreaker {
    return &CircuitBreaker{
        threshold:    threshold,
        resetTimeout: resetTimeout,
        logger:       logger,
    }
}

func (cb *CircuitBreaker) Call(ctx context.Context, fn func(context.Context) (*EnvironmentalMetrics, error)) (*EnvironmentalMetrics, error) {
    // If circuit is open, return default metrics immediately
    if cb.isOpen() {
        cb.logger.Warn("Circuit breaker open, returning default metrics")
        return &EnvironmentalMetrics{}, nil
    }

    // Attempt call
    metrics, err := fn(ctx)
    if err != nil {
        cb.recordFailure()
        cb.logger.Warn("Circuit breaker recorded failure",
            zap.Int("failure_count", cb.failureCount),
        )
        return &EnvironmentalMetrics{}, nil
    }

    cb.recordSuccess()
    return metrics, nil
}

func (cb *CircuitBreaker) isOpen() bool {
    if cb.failureCount >= cb.threshold {
        if time.Since(cb.lastFailureTime) < cb.resetTimeout {
            return true
        }
        // Reset to half-open state
        cb.failureCount = 0
        cb.halfOpenRequests = 0
    }
    return false
}

func (cb *CircuitBreaker) recordFailure() {
    cb.failureCount++
    cb.lastFailureTime = time.Now()
}

func (cb *CircuitBreaker) recordSuccess() {
    cb.failureCount = 0
}
```

---

## 📊 Error Handling Strategy

| Dependency | Failure Mode | Handling Strategy |
|-----------|--------------|-------------------|
| **Data Storage** | Unavailable | Return error (critical dependency) |
| **Infrastructure Monitoring** | Unavailable | Graceful degradation (log warning, continue with basic assessment) |
| **Context API** | Not applicable | Effectiveness Monitor does not depend on Context API |

---

## ✅ Integration Checklist

### **Pre-Deployment**

- [ ] Data Storage Service connection tested (action history retrieval)
- [ ] Infrastructure Monitoring Service connection tested (metrics query)
- [ ] Circuit breaker configured for Infrastructure Monitoring
- [ ] Graceful degradation tested (Infrastructure Monitoring unavailable)
- [ ] Assessment persistence tested (Data Storage write)

### **Runtime Integration**

- [ ] All HTTP clients use Bearer token authentication
- [ ] Timeouts configured (10s for Data Storage, 10s for Infrastructure Monitoring)
- [ ] Circuit breaker operational for Infrastructure Monitoring
- [ ] Assessment results persisted to Data Storage (best-effort)
- [ ] Metrics correlation works when Infrastructure Monitoring available

### **Monitoring**

- [ ] Data Storage call duration tracked in metrics
- [ ] Infrastructure Monitoring call duration tracked in metrics
- [ ] Circuit breaker state exposed in metrics
- [ ] Graceful degradation events logged
- [ ] Assessment persistence failures alerted

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

