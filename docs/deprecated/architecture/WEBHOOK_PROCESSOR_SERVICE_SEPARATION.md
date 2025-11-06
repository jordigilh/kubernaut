# ⚠️ **DEPRECATED** - Kubernaut - 10-Service Microservices Architecture Implementation

**Document Version**: 2.0
**Date**: September 28, 2025
**Status**: **DEPRECATED** - Implementation Completed, Document Obsolete
**Implementation**: Production Ready - 10-Service Microservices

---

## 🚨 **DEPRECATION NOTICE**

**This document is DEPRECATED and should not be used for current development.**

- **Reason**: Webhook/Processor service separation has been completed
- **Replacement**: See current V1 architecture in [KUBERNAUT_ARCHITECTURE_OVERVIEW.md](KUBERNAUT_ARCHITECTURE_OVERVIEW.md)
- **Current Status**: V1 implementation with 10 services is active
- **Last Updated**: January 2025

**⚠️ Do not use this information for architectural decisions.**

---

---

## 1. Executive Summary

### 1.1 Architecture Overview
This document defines the implementation of Kubernaut's **approved 10-service microservices architecture**, enabling complete fault isolation, independent scaling, and operational excellence through Single Responsibility Principle compliance. Each service has exactly one responsibility and can be deployed, scaled, and maintained independently.

### 1.2 Key Architectural Decisions
- **10-Service Architecture**: Complete decomposition following Single Responsibility Principle
- **Service Portfolio**: Gateway, Alert Processor, AI Analysis, Workflow Orchestrator, K8s Executor, Data Storage, Intelligence, Effectiveness Monitor, Context API, Notifications
- **Communication Protocol**: HTTP REST API communication between services
- **Deployment Model**: Independent Kubernetes deployments with service discovery
- **Fault Tolerance**: Circuit breaker patterns with graceful degradation
- **Business Requirements Coverage**: All 1,500+ business requirements mapped to services

### 1.3 Business Value
- **Complete Fault Isolation**: Independent failure domains for all 10 services
- **Optimal Scaling**: Each service scales based on its specific workload characteristics
- **Deployment Velocity**: Independent deployment cycles for each service
- **Operational Excellence**: Clear service boundaries, monitoring, and maintenance
- **Business Alignment**: Services directly map to business capabilities

---

## 2. Approved 10-Service Microservices Architecture

### 2.1 Service Architecture Overview

**Reference**: `docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md`

| Service | Responsibility | Port | Business Requirements | External Connections |
|---------|---------------|------|----------------------|---------------------|
| **🔗 Gateway** | HTTP Gateway & Security Only | 8080 | BR-WH-001 to BR-WH-015 | Prometheus, Grafana |
| **🧠 Alert Processor** | Alert Processing Logic Only | 8081 | BR-SP-001 to BR-SP-050 | None (internal only) |
| **🤖 AI Analysis** | AI Analysis & Decision Making Only | 8082 | BR-AI-001 to BR-AI-140 | OpenAI, Anthropic, Azure, AWS, Ollama |
| **🎯 Workflow Orchestrator** | Workflow Execution Only | 8083 | BR-WF-001 to BR-WF-165 | None (internal only) |
| **⚡ K8s Executor** | Kubernetes Operations Only | 8084 | BR-EX-001 to BR-EX-155 | Kubernetes Clusters |
| **📊 Data Storage** | Data Persistence Only | 8085 | BR-STOR-001 to BR-STOR-135 | PostgreSQL, Vector DBs |
| **🔍 Intelligence** | Pattern Discovery Only | 8086 | BR-INT-001 to BR-INT-150 | None (internal only) |
| **📈 Effectiveness Monitor** | Effectiveness Assessment Only | 8087 | BR-INS-001 to BR-INS-010 | None (internal only) |
| **🌐 Context API** | Context Orchestration Only | 8088 | BR-CTX-001 to BR-CTX-180 | HolmesGPT, External AI |
| **📢 Notifications** | Multi-Channel Notifications Only | 8089 | BR-NOTIF-001 to BR-NOTIF-120 | Slack, Teams, Email, PagerDuty |

### 2.2 Approved Service Flow Architecture

```ascii
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    APPROVED 10-SERVICE MICROSERVICES ARCHITECTURE             │
└─────────────────────────────────────────────────────────────────────────────────┘

External Systems              Kubernaut Microservices                     Infrastructure
┌─────────────────┐          ┌─────────────────────────────────────────┐  ┌─────────────┐
│   Prometheus    │          │                                         │  │ PostgreSQL  │
│   AlertManager  │◄────────►│  🔗 GATEWAY SERVICE (8080)             │  │             │
│   Grafana       │ /webhook │  HTTP Gateway & Security Only           │  └─────────────┘
└─────────────────┘          │                                         │          ▲
                             └─────────────────┬───────────────────────┘          │
                                               │ HTTP POST                        │
                                               │ /process-alert                   │
                                               ▼                                  │
                             ┌─────────────────────────────────────────┐          │
                             │  🧠 ALERT PROCESSOR SERVICE (8081)    │          │
                             │  Alert Processing Logic Only           │          │
                             └─────────────────┬───────────────────────┘          │
                                               │ HTTP POST                        │
                                               │ /analyze-alert                   │
                                               ▼                                  │
┌─────────────────┐          ┌─────────────────────────────────────────┐          │
│   OpenAI        │◄────────►│  🤖 AI ANALYSIS SERVICE (8082)        │          │
│   Anthropic     │ LLM API  │  AI Analysis & Decision Making Only     │          │
│   Azure OpenAI  │          └─────────────────┬───────────────────────┘          │
│   AWS Bedrock   │                            │ HTTP POST                        │
│   Ollama        │                            │ /create-workflow                 │
└─────────────────┘                            ▼                                  │
                             ┌─────────────────────────────────────────┐          │
                             │  🎯 WORKFLOW ORCHESTRATOR (8083)       │          │
                             │  Workflow Execution Only                │          │
                             └─────────────────┬───────────────────────┘          │
                                               │ HTTP POST                        │
                                               │ /execute-action                  │
                                               ▼                                  │
┌─────────────────┐          ┌─────────────────────────────────────────┐          │
│   Kubernetes    │◄────────►│  ⚡ KUBERNETES EXECUTOR (8084)         │          │
│   Clusters      │ K8s API  │  Kubernetes Operations Only             │          │
└─────────────────┘          └─────────────────┬───────────────────────┘          │
                                               │ HTTP POST                        │
                                               │ /store-action                    │
                                               ▼                                  │
┌─────────────────┐          ┌─────────────────────────────────────────┐          │
│   Vector DB     │◄────────►│  📊 DATA STORAGE SERVICE (8085)       │◄─────────┤
│   (PGVector)    │          │  Data Persistence Only                  │          │
└─────────────────┘          └─────────────────┬───────────────────────┘          │
                                               │ HTTP GET                         │
                                               │ /get-patterns                    │
                                               ▼                                  │
                             ┌─────────────────────────────────────────┐          │
                             │  🔍 INTELLIGENCE SERVICE (8086)        │          │
                             │  Pattern Discovery Only                 │          │
                             └─────────────────┬───────────────────────┘          │
                                               │ HTTP POST                        │
                                               │ /assess-effectiveness            │
                                               ▼                                  │
                             ┌─────────────────────────────────────────┐          │
                             │  📈 EFFECTIVENESS MONITOR (8087)       │          │
                             │  Effectiveness Assessment Only          │          │
                             └─────────────────┬───────────────────────┘          │
                                               │ HTTP GET                         │
                                               │ /get-context                     │
                                               ▼                                  │
┌─────────────────┐          ┌─────────────────────────────────────────┐          │
│   HolmesGPT     │◄────────►│  🌐 CONTEXT API SERVICE (8088)        │          │
│   External AI   │ Context  │  Context Orchestration Only             │          │
└─────────────────┘          └─────────────────┬───────────────────────┘          │
                                               │ HTTP POST                        │
                                               │ /send-notification               │
                                               ▼                                  │
┌─────────────────┐          ┌─────────────────────────────────────────┐          │
│   Slack         │◄────────►│  📢 NOTIFICATION SERVICE (8089)        │          │
│   Teams         │          │  Multi-Channel Notifications Only       │          │
│   Email         │          └─────────────────────────────────────────┘          │
│   PagerDuty     │                                                               │
└─────────────────┘                                                               │

Benefits:
- Single Responsibility Principle: Each service has exactly one responsibility
- Complete Fault Isolation: Independent failure domains for all 10 services
- Independent Scaling: Each service scales based on its specific workload
- Business Requirements Coverage: All 1,500+ BRs mapped to services
- Operational Excellence: Clear service boundaries and monitoring
```

---

## 3. Service Specifications

### 3.1 Webhook Service

#### 3.1.1 Service Identity
- **Service Name**: `webhook-service`
- **Container Port**: `8080` (HTTP), `9090` (Metrics), `8081` (Health)
- **Kubernetes Service**: `webhook-service.kubernaut-system.svc.cluster.local:8080`
- **Docker Image**: `registry.kubernaut.io/webhook-service:latest`

#### 3.1.2 Responsibilities
```yaml
Primary Responsibilities:
  - HTTP webhook endpoint management (/alerts)
  - Request validation and authentication
  - Payload parsing and normalization
  - Rate limiting and security enforcement
  - Metrics collection and health monitoring
  - HTTP client management for processor communication

Secondary Responsibilities:
  - Circuit breaker management for processor service
  - Alert queuing for retry when processor unavailable (TRANSPORT ONLY)
  - Request correlation and tracing
  - Response aggregation and formatting
```

#### 3.1.3 Business Requirements Addressed
- **BR-WH-001**: Receive HTTP webhook requests from Prometheus Alertmanager
- **BR-WH-003**: Validate webhook payloads for completeness and format
- **BR-WH-004**: Implement webhook authentication and authorization
- **BR-WH-006**: Handle concurrent webhook requests with high throughput
- **BR-WH-011**: Provide appropriate HTTP response codes for all request types
- **BR-PERF-001**: Process webhook requests within 2 seconds
- **BR-PERF-002**: Handle 1000 concurrent webhook requests

#### 3.1.4 API Endpoints
```http
# Webhook Reception
POST /alerts
Content-Type: application/json
Authorization: Bearer <token>

# Health Monitoring
GET /health          # Liveness probe
GET /ready           # Readiness probe (includes processor service health)
GET /metrics         # Prometheus metrics

# Service Information
GET /api/v1/info     # Service metadata and version
```

#### 3.1.5 Configuration
```yaml
# Environment Variables
WEBHOOK_PORT: "8080"
HEALTH_PORT: "8081"
METRICS_PORT: "9090"
PROCESSOR_SERVICE_URL: "http://processor-service:8095"
PROCESSOR_TIMEOUT: "60s"  # Increased to allow time for complex AI analysis
PROCESSOR_RETRY_COUNT: "3"
CIRCUIT_BREAKER_THRESHOLD: "5"
CIRCUIT_BREAKER_TIMEOUT: "60s"
AUTH_TOKEN_SECRET: "<secret>"
RATE_LIMIT_REQUESTS: "1000"
RATE_LIMIT_WINDOW: "60s"
```

#### 3.1.6 Dependencies
```yaml
Internal Dependencies:
  - processor-service:8095 (HTTP REST API)
  - Kubernetes DNS resolution

External Dependencies:
  - Prometheus AlertManager (webhook source)
  - Authentication token validation service
```

### 3.2 Processor Service

#### 3.2.1 Service Identity
- **Service Name**: `processor-service`
- **Container Port**: `8095` (HTTP), `9095` (Metrics), `8085` (Health)
- **Kubernetes Service**: `processor-service.kubernaut-system.svc.cluster.local:8095`
- **Docker Image**: `registry.kubernaut.io/processor-service:latest`

#### 3.2.2 Responsibilities
```yaml
Primary Responsibilities:
  - Alert processing and filtering logic
  - AI service integration and coordination
  - Action execution management and orchestration
  - Action history tracking and persistence
  - Business logic processing and validation

Secondary Responsibilities:
  - Performance metrics collection and reporting
  - Error handling and recovery coordination
  - Configuration management for processing rules
  - Integration with monitoring and logging systems
```

#### 3.2.3 Business Requirements Addressed
- **BR-SP-001**: Process incoming alerts through configurable filtering rules
- **BR-SP-016**: Integrate with AI components for intelligent alert analysis
- **BR-PA-006**: Analyze alerts using enterprise 20B+ parameter LLM providers
- **BR-PA-007**: Generate contextual remediation recommendations
- **BR-PA-011**: Execute 25+ supported Kubernetes remediation actions
- **BR-PERF-006**: Complete alert processing within 5 seconds for standard alerts
- **BR-PERF-009**: Support 100 concurrent alert processing workflows

#### 3.2.4 API Endpoints
```http
# Alert Processing
POST /api/v1/process-alert
Content-Type: application/json
{
  "alert": {
    "name": "HighMemoryUsage",
    "severity": "critical",
    "namespace": "default",
    "status": "firing",
    "labels": {...},
    "annotations": {...}
  },
  "context": {
    "request_id": "uuid",
    "timestamp": "2025-01-15T10:00:00Z",
    "source": "webhook-service"
  }
}

Response:
{
  "success": true,
  "processing_time": "2.5s",
  "actions_executed": 2,
  "confidence": 0.85,
  "request_id": "uuid"
}

# Alert Filtering Check (INTERNAL - not exposed to webhook service)
# Filtering is handled internally by processor service
# Webhook service does not make filtering decisions

# Health Monitoring
GET /health          # Liveness probe
GET /ready           # Readiness probe (includes AI service health)
GET /metrics         # Prometheus metrics

# Service Information
GET /api/v1/info     # Service metadata and capabilities
GET /api/v1/filters  # Available filtering configurations
```

#### 3.2.5 Configuration
```yaml
# Environment Variables
PROCESSOR_PORT: "8095"
HEALTH_PORT: "8085"
METRICS_PORT: "9095"
AI_SERVICE_URL: "http://ai-service:8093"
AI_SERVICE_TIMEOUT: "60s"  # Increased to allow time for complex AI analysis
DATABASE_URL: "postgresql://user:pass@postgres:5432/kubernaut"
KUBECONFIG: "/etc/kubeconfig/config"
LOG_LEVEL: "info"
PROCESSING_TIMEOUT: "300s"
MAX_CONCURRENT_PROCESSING: "100"
FILTER_CONFIG_PATH: "/etc/processor/filters.yaml"
```

#### 3.2.6 Dependencies
```yaml
Internal Dependencies:
  - ai-service:8093 (HTTP REST API)
  - PostgreSQL database (action history)
  - Kubernetes API server (action execution)

External Dependencies:
  - Vector database (pattern matching)
  - Prometheus (metrics collection)
  - External LLM providers (via AI service)
```

---

## 4. Separation of Concerns - Corrected Architecture

### 4.0 Proper Service Boundaries

#### 4.0.1 Webhook Service Responsibilities (ONLY)
```yaml
ALLOWED Responsibilities:
  - HTTP request/response handling
  - Authentication and authorization
  - Payload validation and parsing
  - Rate limiting and security
  - Metrics collection
  - HTTP client communication with processor
  - Alert queuing for retry (NO processing logic)

FORBIDDEN Responsibilities:
  - Alert filtering decisions
  - Business rule processing
  - Remediation logic
  - AI analysis coordination
  - Action execution decisions
```

#### 4.0.2 Processor Service Responsibilities (ONLY)
```yaml
ALLOWED Responsibilities:
  - ALL alert processing logic
  - ALL filtering decisions
  - ALL business rule evaluation
  - AI service coordination
  - Action execution management
  - History tracking and persistence
  - Effectiveness assessment

FORBIDDEN Responsibilities:
  - HTTP webhook handling
  - Authentication/authorization
  - Rate limiting
  - Payload parsing from AlertManager
```

#### 4.0.3 Corrected Service Communication Strategy
```go
// Webhook Service (CORRECTED) - NO fallback processing logic
func (c *HTTPProcessorClient) ProcessAlert(ctx context.Context, alert types.Alert) error {
    // Circuit breaker check
    if !c.circuitBreaker.AllowRequest() {
        return c.queueAlertForRetry(alert)  // ONLY queue - NO processing
    }

    // Make HTTP call to processor service
    return c.makeHTTPRequest(ctx, alert)
}

// Processor Service handles ALL processing decisions
func (p *ProcessorService) ProcessAlert(ctx context.Context, alert types.Alert) error {
    // Apply filtering logic
    if !p.shouldProcess(alert) {
        return nil // Processor decides to skip
    }

    // Process with AI or fallback to rules
    return p.processWithAIOrFallback(ctx, alert)
}
```

## 5. Communication Patterns

### 4.1 Webhook to Processor Communication

#### 4.1.1 Request Flow
```ascii
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ AlertManager    │───▶│ Webhook Service │───▶│ Processor       │
│                 │    │                 │    │ Service         │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                              │                        │
                              ▼                        ▼
                       ┌─────────────────┐    ┌─────────────────┐
                       │ HTTP Response   │◀───│ Processing      │
                       │ (Success/Error) │    │ Result          │
                       └─────────────────┘    └─────────────────┘
```

#### 4.1.2 HTTP Client Implementation
```go
// pkg/integration/processor/http_client.go
type HTTPProcessorClient struct {
    baseURL       string
    httpClient    *http.Client
    circuitBreaker *CircuitBreaker
    retryConfig   *RetryConfig
    log           *logrus.Logger
}

type ProcessAlertRequest struct {
    Alert   types.Alert        `json:"alert"`
    Context *ProcessingContext `json:"context"`
}

type ProcessAlertResponse struct {
    Success         bool          `json:"success"`
    ProcessingTime  string        `json:"processing_time"`
    ActionsExecuted int           `json:"actions_executed"`
    Confidence      float64       `json:"confidence"`
    RequestID       string        `json:"request_id"`
    Error           string        `json:"error,omitempty"`
}

func (c *HTTPProcessorClient) ProcessAlert(ctx context.Context, alert types.Alert) error {
    // Circuit breaker check
    if !c.circuitBreaker.AllowRequest() {
        return c.queueAlertForRetry(alert)
    }

    // Prepare request
    req := ProcessAlertRequest{
        Alert: alert,
        Context: &ProcessingContext{
            RequestID: generateRequestID(),
            Timestamp: time.Now().UTC(),
            Source:    "webhook-service",
        },
    }

    // Make HTTP request with retry logic
    resp, err := c.makeRequestWithRetry(ctx, req)
    if err != nil {
        c.circuitBreaker.RecordFailure()
        return c.queueAlertForRetry(alert)
    }

    c.circuitBreaker.RecordSuccess()
    return c.validateResponse(resp)
}
```

#### 4.1.3 Circuit Breaker Configuration
```yaml
Circuit Breaker Settings:
  failure_threshold: 5        # Open circuit after 5 consecutive failures
  recovery_timeout: 60s       # Try to close circuit after 60 seconds
  success_threshold: 3        # Close circuit after 3 consecutive successes
  timeout: 60s               # Request timeout before considering failure (increased for complex AI analysis)

Retry Configuration:
  max_retries: 3             # Maximum retry attempts
  initial_delay: 100ms       # Initial retry delay
  max_delay: 5s              # Maximum retry delay
  backoff_multiplier: 2.0    # Exponential backoff multiplier
```

### 4.2 Processor to AI Service Communication

#### 4.2.1 Existing Pattern (Already Implemented)
```go
// Already implemented in pkg/ai/http/client.go
type AIServiceHTTPClient struct {
    baseURL    string
    httpClient *http.Client
    log        *logrus.Logger
}

// This pattern is already proven and working
func (c *AIServiceHTTPClient) AnalyzeAlert(ctx context.Context, alert interface{}) (*llm.AnalyzeAlertResponse, error) {
    // HTTP POST to AI service /api/v1/analyze-alert
    // Returns structured AI analysis response
}
```

### 4.3 Error Handling and Fallback Strategies

#### 4.3.1 Webhook Service Fallback (CORRECTED - No Processing Logic)
```go
func (c *HTTPProcessorClient) queueAlertForRetry(alert types.Alert) error {
    c.log.WithField("alert", alert.Name).Warn("Processor service unavailable, queuing for retry")

    // ONLY responsibility: Queue alert for retry when processor service recovers
    // NO business logic, NO rule processing, NO filtering in webhook service
    retryItem := &RetryQueueItem{
        Alert:     alert,
        Timestamp: time.Now(),
        Attempts:  0,
        NextRetry: time.Now().Add(c.getRetryDelay(0)),
    }

    return c.retryQueue.Enqueue(retryItem)
}

// Background retry processor - ONLY retries HTTP calls, NO processing logic
func (c *HTTPProcessorClient) processRetryQueue() {
    for {
        item := c.retryQueue.Dequeue()
        if item == nil {
            time.Sleep(1 * time.Second)
            continue
        }

        // Simply retry the HTTP call - processor service handles all logic
        if err := c.ProcessAlert(context.Background(), item.Alert); err != nil {
            item.Attempts++
            if item.Attempts < c.maxRetries {
                item.NextRetry = time.Now().Add(c.getRetryDelay(item.Attempts))
                c.retryQueue.Enqueue(item)
            } else {
                c.log.WithField("alert", item.Alert.Name).Error("Max retries exceeded, dropping alert")
            }
        }
    }
}
```

#### 4.3.2 Processor Service Resilience
```go
func (p *ProcessorService) processAlertWithResilience(ctx context.Context, alert types.Alert) error {
    // Try AI service first
    if p.aiClient.IsHealthy() {
        return p.processWithAI(ctx, alert)
    }

    // Fallback to rule-based processing
    p.log.Warn("AI service unavailable, using rule-based fallback")
    return p.processWithRules(ctx, alert)
}
```

---

## 5. Implementation Plan

### 5.1 Phase 1: Foundation (Week 1-2)

#### 5.1.1 Create Processor Service Skeleton
```bash
# Directory structure
cmd/processor-service/
├── main.go                    # Service entry point
├── main_test.go              # Integration tests
└── Dockerfile                # Container image

pkg/integration/processor/
├── http_client.go            # HTTP client for webhook service
├── http_client_test.go       # Client tests
├── service.go                # Processor service implementation
└── service_test.go           # Service tests

deploy/microservices/
├── processor-service-deployment.yaml
├── processor-service-service.yaml
└── processor-service-configmap.yaml
```

#### 5.1.2 Processor Service Implementation
```go
// cmd/processor-service/main.go
package main

import (
    "context"
    "fmt"
    "net/http"
    "os"
    "time"

    "github.com/jordigilh/kubernaut/pkg/integration/processor"
    "github.com/sirupsen/logrus"
)

func main() {
    log := logrus.New()
    log.Info("🚀 Starting Kubernaut Processor Service")

    // Initialize processor service
    service, err := processor.NewService(log)
    if err != nil {
        log.WithError(err).Fatal("Failed to create processor service")
    }

    // Setup HTTP server
    mux := http.NewServeMux()
    service.RegisterRoutes(mux)

    server := &http.Server{
        Addr:    ":8095",
        Handler: mux,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 30 * time.Second,
    }

    // Start server
    log.Info("🌐 Processor service listening on :8095")
    if err := server.ListenAndServe(); err != nil {
        log.WithError(err).Fatal("Server failed")
    }
}
```

#### 5.1.3 HTTP Processor Client
```go
// pkg/integration/processor/http_client.go
package processor

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/jordigilh/kubernaut/pkg/shared/types"
    "github.com/sirupsen/logrus"
)

type HTTPProcessorClient struct {
    baseURL    string
    httpClient *http.Client
    log        *logrus.Logger
}

func NewHTTPProcessorClient(baseURL string, log *logrus.Logger) Processor {
    return &HTTPProcessorClient{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
        log: log,
    }
}

func (c *HTTPProcessorClient) ProcessAlert(ctx context.Context, alert types.Alert) error {
    req := ProcessAlertRequest{
        Alert: alert,
        Context: &ProcessingContext{
            RequestID: generateRequestID(),
            Timestamp: time.Now().UTC(),
            Source:    "webhook-service",
        },
    }

    jsonData, err := json.Marshal(req)
    if err != nil {
        return fmt.Errorf("failed to marshal request: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/process-alert", bytes.NewBuffer(jsonData))
    if err != nil {
        return fmt.Errorf("failed to create HTTP request: %w", err)
    }
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return fmt.Errorf("HTTP request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("processor service returned status %d", resp.StatusCode)
    }

    var response ProcessAlertResponse
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return fmt.Errorf("failed to decode response: %w", err)
    }

    if !response.Success {
        return fmt.Errorf("processing failed: %s", response.Error)
    }

    c.log.WithFields(logrus.Fields{
        "processing_time":   response.ProcessingTime,
        "actions_executed": response.ActionsExecuted,
        "confidence":       response.Confidence,
    }).Info("Alert processed successfully")

    return nil
}

func (c *HTTPProcessorClient) ShouldProcess(alert types.Alert) bool {
    // REMOVED: Webhook service should NOT make filtering decisions
    // The processor service handles ALL filtering logic
    // Webhook service only validates, parses, and forwards alerts

    // Always return true - let processor service handle filtering
    return true
}
```

### 5.2 Phase 2: Integration (Week 3-4)

#### 5.2.1 Update Webhook Service
```go
// cmd/integration-webhook-server/main.go - Updated
func createProcessor(log *logrus.Logger) (processor.Processor, error) {
    processorServiceURL := os.Getenv("PROCESSOR_SERVICE_URL")
    if processorServiceURL == "" {
        processorServiceURL = "http://processor-service:8095"
    }

    useHTTPProcessor := os.Getenv("USE_HTTP_PROCESSOR")
    if useHTTPProcessor == "true" {
        log.Info("✅ Using HTTP Processor Client for microservices architecture")
        return processor.NewHTTPProcessorClient(processorServiceURL, log), nil
    }

    // Fallback to direct processor (for gradual migration)
    log.Info("✅ Using Direct Processor (legacy mode)")
    return createDirectProcessor(log)
}
```

#### 5.2.2 Feature Flag Configuration
```yaml
# ConfigMap for gradual migration
apiVersion: v1
kind: ConfigMap
metadata:
  name: webhook-service-config
data:
  USE_HTTP_PROCESSOR: "false"  # Start with false, gradually enable
  PROCESSOR_SERVICE_URL: "http://processor-service:8095"
  PROCESSOR_TIMEOUT: "60s"  # Increased to allow time for complex AI analysis
  CIRCUIT_BREAKER_ENABLED: "true"
```

### 5.3 Phase 3: Deployment (Week 5-6)

#### 5.3.1 Kubernetes Deployments
```yaml
# deploy/microservices/processor-service-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: processor-service
  namespace: kubernaut-system
spec:
  replicas: 3
  selector:
    matchLabels:
      app: processor-service
  template:
    metadata:
      labels:
        app: processor-service
    spec:
      containers:
      - name: processor-service
        image: registry.kubernaut.io/processor-service:latest
        ports:
        - containerPort: 8095
          name: http
        - containerPort: 8085
          name: health
        - containerPort: 9095
          name: metrics
        env:
        - name: PROCESSOR_PORT
          value: "8095"
        - name: AI_SERVICE_URL
          value: "http://ai-service:8093"
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: processor-secrets
              key: database_url
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8085
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8085
          initialDelaySeconds: 10
          periodSeconds: 10

---
apiVersion: v1
kind: Service
metadata:
  name: processor-service
  namespace: kubernaut-system
spec:
  selector:
    app: processor-service
  ports:
  - name: http
    port: 8095
    targetPort: 8095
  - name: metrics
    port: 9095
    targetPort: 9095
```

#### 5.3.2 Migration Strategy
```bash
# Step 1: Deploy processor service alongside existing system
kubectl apply -f deploy/microservices/processor-service-deployment.yaml

# Step 2: Verify processor service health
kubectl get pods -l app=processor-service
kubectl logs -l app=processor-service

# Step 3: Enable HTTP processor gradually
kubectl patch configmap webhook-service-config -p '{"data":{"USE_HTTP_PROCESSOR":"true"}}'

# Step 4: Monitor metrics and health
kubectl port-forward svc/processor-service 9095:9095
curl http://localhost:9095/metrics

# Step 5: Validate end-to-end functionality
# Send test alerts and verify processing
```

---

## 6. Monitoring and Observability

### 6.1 Service Metrics

#### 6.1.1 Webhook Service Metrics
```go
// Prometheus metrics for webhook service
var (
    webhookRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "webhook_requests_total",
            Help: "Total number of webhook requests received",
        },
        []string{"status", "method"},
    )

    processorClientRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "processor_client_requests_total",
            Help: "Total requests to processor service",
        },
        []string{"status", "endpoint"},
    )

    processorClientDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "processor_client_request_duration_seconds",
            Help: "Duration of processor service requests",
        },
        []string{"endpoint"},
    )

    circuitBreakerState = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "circuit_breaker_state",
            Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
        },
        []string{"service"},
    )
)
```

#### 6.1.2 Processor Service Metrics
```go
// Prometheus metrics for processor service
var (
    alertsProcessedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "alerts_processed_total",
            Help: "Total number of alerts processed",
        },
        []string{"status", "severity"},
    )

    processingDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "alert_processing_duration_seconds",
            Help: "Duration of alert processing",
        },
        []string{"severity"},
    )

    aiServiceRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ai_service_requests_total",
            Help: "Total requests to AI service",
        },
        []string{"status"},
    )

    activeProcessingGauge = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "active_alert_processing",
            Help: "Number of alerts currently being processed",
        },
    )
)
```

### 6.2 Health Checks

#### 6.2.1 Webhook Service Health
```go
func (s *WebhookService) healthCheck(w http.ResponseWriter, r *http.Request) {
    health := map[string]interface{}{
        "status":    "healthy",
        "timestamp": time.Now().UTC(),
        "version":   "1.0.0",
        "checks": map[string]interface{}{
            "processor_service": s.checkProcessorService(),
            "memory_usage":     s.checkMemoryUsage(),
            "goroutines":       runtime.NumGoroutine(),
        },
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}

func (s *WebhookService) readinessCheck(w http.ResponseWriter, r *http.Request) {
    // Check if processor service is reachable
    if !s.processorClient.IsHealthy() {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "not_ready",
            "reason": "processor_service_unavailable",
        })
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "status": "ready",
    })
}
```

#### 6.2.2 Processor Service Health
```go
func (s *ProcessorService) healthCheck(w http.ResponseWriter, r *http.Request) {
    health := map[string]interface{}{
        "status":    "healthy",
        "timestamp": time.Now().UTC(),
        "version":   "1.0.0",
        "checks": map[string]interface{}{
            "ai_service":    s.checkAIService(),
            "database":      s.checkDatabase(),
            "kubernetes":    s.checkKubernetes(),
            "memory_usage":  s.checkMemoryUsage(),
        },
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}
```

### 6.3 Logging and Tracing

#### 6.3.1 Request Correlation
```go
type ProcessingContext struct {
    RequestID   string    `json:"request_id"`
    Timestamp   time.Time `json:"timestamp"`
    Source      string    `json:"source"`
    TraceID     string    `json:"trace_id,omitempty"`
    SpanID      string    `json:"span_id,omitempty"`
}

func generateRequestID() string {
    return fmt.Sprintf("req_%d_%s", time.Now().Unix(), uuid.New().String()[:8])
}
```

#### 6.3.2 Structured Logging
```go
func (c *HTTPProcessorClient) ProcessAlert(ctx context.Context, alert types.Alert) error {
    requestID := generateRequestID()

    logger := c.log.WithFields(logrus.Fields{
        "request_id":   requestID,
        "alert_name":   alert.Name,
        "alert_severity": alert.Severity,
        "service":      "webhook-service",
        "target":       "processor-service",
    })

    logger.Info("Starting alert processing request")

    // ... processing logic ...

    logger.WithField("processing_time", time.Since(start)).Info("Alert processing completed")
    return nil
}
```

---

## 7. Testing Strategy

### 7.1 Unit Testing

#### 7.1.1 HTTP Client Tests
```go
// pkg/integration/processor/http_client_test.go
func TestHTTPProcessorClient_ProcessAlert(t *testing.T) {
    // Create mock processor service
    mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "POST", r.Method)
        assert.Equal(t, "/api/v1/process-alert", r.URL.Path)

        var req ProcessAlertRequest
        err := json.NewDecoder(r.Body).Decode(&req)
        assert.NoError(t, err)
        assert.Equal(t, "TestAlert", req.Alert.Name)

        response := ProcessAlertResponse{
            Success:         true,
            ProcessingTime:  "1.5s",
            ActionsExecuted: 1,
            Confidence:      0.85,
            RequestID:       req.Context.RequestID,
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(response)
    }))
    defer mockServer.Close()

    client := NewHTTPProcessorClient(mockServer.URL, logrus.New())

    alert := types.Alert{
        Name:     "TestAlert",
        Severity: "critical",
        Status:   "firing",
    }

    err := client.ProcessAlert(context.Background(), alert)
    assert.NoError(t, err)
}
```

#### 7.1.2 Circuit Breaker Tests
```go
func TestCircuitBreaker_FailureHandling(t *testing.T) {
    // Test circuit breaker opens after failures
    // Test circuit breaker recovery
    // Test alert queuing behavior (NO processing logic)
}
```

### 7.2 Integration Testing

#### 7.2.1 End-to-End Service Tests
```go
func TestWebhookToProcessorIntegration(t *testing.T) {
    // Start both services
    // Send webhook request
    // Verify processor receives request
    // Verify response propagation
    // Verify metrics collection
}
```

#### 7.2.2 Failure Scenario Tests
```go
func TestProcessorServiceFailure(t *testing.T) {
    // Test webhook service behavior when processor is down
    // Test circuit breaker activation
    // Test alert queuing for retry (NO processing logic)
    // Test service recovery and retry processing
}
```

---

## 8. Security Considerations

### 8.1 Service-to-Service Authentication

#### 8.1.1 Internal Service Authentication
```yaml
# Service mesh or mutual TLS configuration
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: processor-service-auth
  namespace: kubernaut-system
spec:
  selector:
    matchLabels:
      app: processor-service
  mtls:
    mode: STRICT
```

#### 8.1.2 API Key Authentication (Alternative)
```go
func (s *ProcessorService) authenticateRequest(r *http.Request) error {
    apiKey := r.Header.Get("X-API-Key")
    if apiKey == "" {
        return fmt.Errorf("missing API key")
    }

    if !s.validateAPIKey(apiKey) {
        return fmt.Errorf("invalid API key")
    }

    return nil
}
```

### 8.2 Network Security

#### 8.2.1 Network Policies
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: processor-service-netpol
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: processor-service
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: webhook-service
    ports:
    - protocol: TCP
      port: 8095
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: ai-service
    ports:
    - protocol: TCP
      port: 8093
```

---

## 9. Performance Considerations

### 9.1 Latency Optimization

#### 9.1.1 Connection Pooling
```go
func NewHTTPProcessorClient(baseURL string, log *logrus.Logger) Processor {
    transport := &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
        DisableCompression:  false,
    }

    return &HTTPProcessorClient{
        baseURL: baseURL,
        httpClient: &http.Client{
            Transport: transport,
            Timeout:   30 * time.Second,
        },
        log: log,
    }
}
```

#### 9.1.2 Request Optimization
```go
// Optimize JSON marshaling
var jsonPool = sync.Pool{
    New: func() interface{} {
        return &bytes.Buffer{}
    },
}

func (c *HTTPProcessorClient) marshalRequest(req ProcessAlertRequest) ([]byte, error) {
    buf := jsonPool.Get().(*bytes.Buffer)
    defer jsonPool.Put(buf)
    buf.Reset()

    encoder := json.NewEncoder(buf)
    if err := encoder.Encode(req); err != nil {
        return nil, err
    }

    return buf.Bytes(), nil
}
```

### 9.2 Throughput Optimization

#### 9.2.1 Concurrent Processing
```go
func (s *ProcessorService) handleProcessAlert(w http.ResponseWriter, r *http.Request) {
    // Use worker pool for concurrent processing
    select {
    case s.workerPool <- struct{}{}:
        defer func() { <-s.workerPool }()
        s.processAlertHandler(w, r)
    default:
        // Return 503 if worker pool is full
        http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
    }
}
```

---

## 10. Migration and Rollback Plan

### 10.1 Migration Steps

#### 10.1.1 Pre-Migration Checklist
- [ ] Processor service deployed and healthy
- [ ] HTTP client implementation tested
- [ ] Circuit breaker configuration validated
- [ ] Monitoring and alerting configured
- [ ] Rollback procedures documented

#### 10.1.2 Migration Execution
```bash
# Phase 1: Deploy processor service (0% traffic)
kubectl apply -f deploy/microservices/processor-service-deployment.yaml
kubectl wait --for=condition=ready pod -l app=processor-service

# Phase 2: Enable HTTP client (10% traffic)
kubectl patch configmap webhook-service-config -p '{"data":{"USE_HTTP_PROCESSOR":"true","HTTP_PROCESSOR_PERCENTAGE":"10"}}'

# Phase 3: Gradual rollout (25%, 50%, 75%, 100%)
kubectl patch configmap webhook-service-config -p '{"data":{"HTTP_PROCESSOR_PERCENTAGE":"25"}}'
# Monitor metrics and health
kubectl patch configmap webhook-service-config -p '{"data":{"HTTP_PROCESSOR_PERCENTAGE":"50"}}'
# Monitor metrics and health
kubectl patch configmap webhook-service-config -p '{"data":{"HTTP_PROCESSOR_PERCENTAGE":"100"}}'

# Phase 4: Remove legacy code (after validation period)
# Update webhook service to remove direct processor integration
```

### 10.2 Rollback Procedures

#### 10.2.1 Immediate Rollback
```bash
# Emergency rollback to direct processor
kubectl patch configmap webhook-service-config -p '{"data":{"USE_HTTP_PROCESSOR":"false"}}'

# Restart webhook service pods to pick up config change
kubectl rollout restart deployment/webhook-service
```

#### 10.2.2 Rollback Validation
```bash
# Verify webhook service health after rollback
kubectl get pods -l app=webhook-service
kubectl logs -l app=webhook-service --tail=100

# Send test alerts to verify functionality
curl -X POST http://webhook-service:8080/alerts \
  -H "Content-Type: application/json" \
  -d @test-alert.json
```

---

## 11. Success Criteria and Validation

### 11.1 Functional Success Criteria
- [ ] All webhook requests continue to be processed successfully
- [ ] Alert processing latency remains under 2 seconds (BR-PERF-001)
- [ ] 99.9% service availability maintained during migration
- [ ] No data loss or processing errors during transition
- [ ] Circuit breaker activates correctly during processor service failures

### 11.2 Performance Success Criteria
- [ ] HTTP communication adds <5ms additional latency
- [ ] Webhook service can scale independently from processor service
- [ ] Processor service can handle 100+ concurrent requests
- [ ] Circuit breaker prevents cascade failures
- [ ] Fallback processing maintains basic functionality

### 11.3 Operational Success Criteria
- [ ] Independent deployment of webhook and processor services
- [ ] Separate monitoring and alerting for each service
- [ ] Clear service boundaries and responsibilities
- [ ] Simplified troubleshooting and debugging
- [ ] Successful rollback capability demonstrated

---

## 12. Conclusion

This architecture document provides a comprehensive plan for separating the webhook and processor components into independent microservices. The separation enables:

- **Fault Isolation**: Independent failure domains prevent cascade failures
- **Scalability**: Independent scaling based on workload characteristics
- **Operational Excellence**: Simplified deployment, monitoring, and maintenance
- **Future Flexibility**: Clear service boundaries enable technology evolution

The implementation plan provides a gradual migration path with comprehensive testing, monitoring, and rollback capabilities to ensure zero-disruption transition to the new architecture.

---

## 13. References

### 13.1 Related Documentation
- [Microservices Communication Architecture](MICROSERVICES_COMMUNICATION_ARCHITECTURE.md)
- [Integration Layer Business Requirements](../requirements/06_INTEGRATION_LAYER.md)
- [AI Service Architecture](AI_SERVICE_ARCHITECTURE.md)
- [Alert Processing Flow](ALERT_PROCESSING_FLOW.md)

### 13.2 Implementation Files
- `pkg/integration/processor/processor.go` - Current processor interface
- `pkg/integration/webhook/handler.go` - Current webhook handler
- `pkg/ai/http/client.go` - HTTP client pattern reference
- `cmd/ai-service/main.go` - Microservice implementation reference

---

*This document serves as the definitive specification for implementing the webhook and processor service separation. All implementation should follow the patterns and guidelines defined in this architecture document.*
