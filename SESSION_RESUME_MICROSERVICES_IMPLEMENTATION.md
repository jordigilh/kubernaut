# Kubernaut Microservices Implementation - Session Resume Guide

**Document Version**: 1.0
**Date**: January 2025
**Status**: Phase 1 Completion - Ready for Service Communication Integration
**Project**: Kubernaut Alert Processing System
**Architecture Pattern**: Strangler Fig Microservices Extraction

---

## 🎯 **EXECUTIVE SUMMARY**

**Current State**: Successfully extracted Webhook Service and AI Service as independent microservices with complete TDD implementation, Docker containerization, and Kubernetes deployment manifests.

**Next Critical Step**: Convert direct interface communication to HTTP REST API communication between services to complete true microservices architecture.

**Business Impact**: Enables independent scaling, fault isolation, and deployment flexibility for Kubernaut's intelligent alert processing system.

---

## 📊 **CURRENT ARCHITECTURE STATUS**

### **✅ COMPLETED WORK**

#### **1. AI Service Extraction (COMPLETE)**
- **Location**: `cmd/ai-service/main.go`
- **Functionality**: REST API for alert analysis with LLM integration and fallback
- **Endpoints**:
  - `POST /api/v1/analyze-alert` - Core AI analysis endpoint
  - `GET /api/v1/service-info` - Service discovery
  - `GET /health` - Liveness probe (port 8083)
  - `GET /ready` - Readiness probe (port 8083)
  - `GET /metrics` - Prometheus metrics (port 9092)
- **Docker Image**: Built with Red Hat UBI9 (`docker/ai-service.Dockerfile`)
- **Deployment**: `deploy/microservices/ai-service-deployment.yaml`
- **TDD Status**: Complete with passing tests in `cmd/ai-service/main_test.go`

#### **2. Webhook Service Extraction (COMPLETE)**
- **Location**: `cmd/webhook-service/main.go`
- **Functionality**: AlertManager webhook processing with business logic integration
- **Endpoints**:
  - `POST /alerts` - AlertManager webhook endpoint
  - `GET /health` - Liveness probe (port 8082)
  - `GET /ready` - Readiness probe (port 8082)
  - `GET /metrics` - Prometheus metrics (port 9091)
- **Docker Image**: Built with Red Hat UBI9 (`docker/webhook-service.Dockerfile`)
- **Deployment**: `deploy/microservices/webhook-service-deployment.yaml`
- **TDD Status**: Complete with passing tests in `cmd/webhook-service/main_test.go`

#### **3. Build System Integration (COMPLETE)**
- **Makefile Targets**:
  - `make build-microservices` - Build all microservice binaries
  - `make docker-build-microservices` - Build all container images
  - `make docker-push-microservices` - Push images to registry
- **Individual Service Targets**:
  - `make build-ai-service` / `make docker-build-ai-service`
  - `make build-webhook-service` / `make docker-build-webhook-service`

#### **4. Documentation Updates (COMPLETE)**
- **Architecture Documentation**: `docs/architecture/MICROSERVICES_COMMUNICATION_ARCHITECTURE.md`
- **README Updates**: Microservices section added
- **Deployment Guides**: Service-specific deployment instructions

### **🔄 CURRENT ARCHITECTURE DIAGRAM**

```ascii
┌─────────────────────────────────────────────────────────────────┐
│                    KUBERNAUT MICROSERVICES                     │
│                         PHASE 1 STATUS                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────┐    INTERFACE    ┌─────────────────┐       │
│  │ Webhook Service │ ──────────────▶ │ AI Service      │       │
│  │ Port: 8081      │   (NEEDS HTTP)  │ Port: 8093      │       │
│  │                 │                 │                 │       │
│  │ ✅ Extracted    │                 │ ✅ Extracted    │       │
│  │ ✅ Containerized│                 │ ✅ Containerized│       │
│  │ ✅ K8s Manifests│                 │ ✅ K8s Manifests│       │
│  │ ✅ TDD Complete │                 │ ✅ TDD Complete │       │
│  └─────────────────┘                 └─────────────────┘       │
│           │                                   │                 │
│           ▼                                   ▼                 │
│  ┌─────────────────┐                 ┌─────────────────┐       │
│  │ Kubernetes API  │                 │ Prometheus      │       │
│  │ Action Executor │                 │ Metrics         │       │
│  │ Database        │                 │ Health Checks   │       │
│  └─────────────────┘                 └─────────────────┘       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 🚨 **CRITICAL NEXT TASK: SERVICE COMMUNICATION**

### **PROBLEM STATEMENT**
Currently, the webhook service still uses direct Go interface calls to communicate with AI analysis functionality:

```go
// cmd/webhook-service/main.go:329-335 (CURRENT - NEEDS CHANGE)
realLLMClient, err := llm.NewClient(llmConfig, log)
if err != nil {
    log.WithError(err).Warn("⚠️  LLM client creation failed, using fallback")
    llmClient = NewFallbackLLMClient(log)
} else {
    llmClient = realLLMClient
}
```

### **REQUIRED SOLUTION**
Replace direct interface with HTTP client that consumes AI service REST API:

```go
// TARGET IMPLEMENTATION
type AIServiceHTTPClient struct {
    baseURL    string
    httpClient *http.Client
    log        *logrus.Logger
}

func (c *AIServiceHTTPClient) AnalyzeAlert(ctx context.Context, alert types.Alert) (*llm.AnalyzeAlertResponse, error) {
    // HTTP POST to http://ai-service:8093/api/v1/analyze-alert
    reqBody := map[string]interface{}{
        "alert":   alert,
        "context": map[string]interface{}{},
    }

    jsonData, _ := json.Marshal(reqBody)
    req, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/analyze-alert", bytes.NewBuffer(jsonData))
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("AI service request failed: %w", err)
    }
    defer resp.Body.Close()

    var response llm.AnalyzeAlertResponse
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return nil, fmt.Errorf("failed to decode AI service response: %w", err)
    }

    return &response, nil
}
```

---

## 📋 **DETAILED TASK BREAKDOWN**

### **TASK 1: Implement HTTP Client for AI Service**
**Priority**: CRITICAL - BLOCKING ALL OTHER TASKS
**Estimated Time**: 2-3 hours
**Files to Create/Modify**:

#### **1.1 Create HTTP Client Implementation**
**File**: `pkg/ai/http/client.go`
```go
package http

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/jordigilh/kubernaut/pkg/ai/llm"
    "github.com/jordigilh/kubernaut/pkg/shared/types"
    "github.com/sirupsen/logrus"
)

type AIServiceHTTPClient struct {
    baseURL    string
    httpClient *http.Client
    log        *logrus.Logger
}

func NewAIServiceHTTPClient(baseURL string, log *logrus.Logger) llm.Client {
    return &AIServiceHTTPClient{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
        log: log,
    }
}

func (c *AIServiceHTTPClient) AnalyzeAlert(ctx context.Context, alert interface{}) (*llm.AnalyzeAlertResponse, error) {
    // Implementation as shown above
}

// Implement all other llm.Client interface methods with HTTP calls or no-ops
func (c *AIServiceHTTPClient) IsHealthy() bool { /* HTTP health check */ }
func (c *AIServiceHTTPClient) LivenessCheck(ctx context.Context) error { /* HTTP liveness */ }
func (c *AIServiceHTTPClient) ReadinessCheck(ctx context.Context) error { /* HTTP readiness */ }
// ... other interface methods
```

#### **1.2 Update Webhook Service to Use HTTP Client**
**File**: `cmd/webhook-service/main.go`
**Lines to Modify**: 318-336

```go
// REPLACE THIS SECTION:
// llmConfig := config.LLMConfig{...}
// realLLMClient, err := llm.NewClient(llmConfig, log)

// WITH THIS:
aiServiceURL := getEnvOrDefault("AI_SERVICE_URL", "http://ai-service:8093")
llmClient := http.NewAIServiceHTTPClient(aiServiceURL, log)
log.Info("✅ AI Service HTTP client initialized")
```

#### **1.3 Update Webhook Service Tests**
**File**: `cmd/webhook-service/main_test.go`
**Action**: Update tests to mock HTTP responses instead of direct interface calls

### **TASK 2: Deploy and Test Microservices**
**Priority**: HIGH
**Estimated Time**: 1-2 hours

#### **2.1 Deploy AI Service**
```bash
# Build and deploy AI service
make docker-build-ai-service
kubectl apply -f deploy/microservices/ai-service-deployment.yaml

# Verify deployment
kubectl get pods -l app.kubernetes.io/name=kubernaut-ai-service
kubectl logs -l app.kubernetes.io/name=kubernaut-ai-service
```

#### **2.2 Deploy Webhook Service**
```bash
# Build and deploy webhook service
make docker-build-webhook-service
kubectl apply -f deploy/microservices/webhook-service-deployment.yaml

# Verify deployment
kubectl get pods -l app.kubernetes.io/name=kubernaut-webhook-service
kubectl logs -l app.kubernetes.io/name=kubernaut-webhook-service
```

#### **2.3 Test Service Communication**
```bash
# Port forward AI service
kubectl port-forward svc/kubernaut-ai-service 8093:8093 &

# Test AI service directly
curl -X POST http://localhost:8093/api/v1/analyze-alert \
  -H "Content-Type: application/json" \
  -d '{"alert":{"name":"test-alert","severity":"critical","namespace":"default"}}'

# Port forward webhook service
kubectl port-forward svc/kubernaut-webhook-service 8081:8081 &

# Test webhook service (should call AI service internally)
curl -X POST http://localhost:8081/alerts \
  -H "Content-Type: application/json" \
  -d '{"alerts":[{"status":"firing","labels":{"alertname":"TestAlert","severity":"critical"}}]}'
```

### **TASK 3: Update Integration Tests**
**Priority**: MEDIUM
**Estimated Time**: 2-3 hours

#### **3.1 Update Bootstrap Script**
**File**: `scripts/bootstrap-dev-environment.sh`
**Action**: Deploy microservices instead of monolithic application

```bash
# Add to setup_kubernetes function:
echo "🚀 Deploying Kubernaut microservices..."
kubectl apply -f deploy/microservices/ai-service-deployment.yaml
kubectl apply -f deploy/microservices/webhook-service-deployment.yaml

# Wait for deployments
kubectl wait --for=condition=available --timeout=300s deployment/kubernaut-ai-service -n kubernaut
kubectl wait --for=condition=available --timeout=300s deployment/kubernaut-webhook-service -n kubernaut
```

#### **3.2 Update AlertManager Configuration**
**File**: `test/manifests/monitoring/alertmanager-config.yaml`
**Lines to Modify**: webhook_configs section

```yaml
webhook_configs:
- url: 'http://kubernaut-webhook-service.e2e-test:8081/alerts'  # Updated endpoint
  http_config:
    headers:
      Authorization: 'Bearer test-token'
```

### **TASK 4: Validate End-to-End Flow**
**Priority**: HIGH
**Estimated Time**: 1 hour

#### **4.1 Integration Test Execution**
```bash
# Run complete integration test
make bootstrap-dev
make test-integration-kind

# Verify services are running
kubectl get pods -n kubernaut
kubectl get pods -n e2e-test

# Check service logs
kubectl logs -l app.kubernetes.io/name=kubernaut-ai-service -n kubernaut
kubectl logs -l app.kubernetes.io/name=kubernaut-webhook-service -n e2e-test
```

#### **4.2 Manual Alert Flow Test**
```bash
# Trigger test alert through AlertManager
kubectl port-forward svc/alertmanager 9093:9093 -n monitoring &

# Create test alert (should flow: AlertManager → Webhook Service → AI Service → Kubernetes Action)
curl -X POST http://localhost:9093/api/v1/alerts \
  -H "Content-Type: application/json" \
  -d '[{"labels":{"alertname":"TestMemoryAlert","severity":"critical","namespace":"default"}}]'
```

---

## 🔧 **TECHNICAL IMPLEMENTATION DETAILS**

### **AI Service JSON API Contract**

#### **Request Format** (`POST /api/v1/analyze-alert`):
```json
{
  "alert": {
    "name": "HighMemoryUsage",
    "severity": "critical",
    "namespace": "default",
    "resource": "webapp-deployment",
    "status": "firing",
    "labels": {
      "alertname": "HighMemoryUsage",
      "pod": "webapp-123"
    }
  },
  "context": {
    "cluster_info": {},
    "historical_data": {}
  }
}
```

#### **Response Format**:
```json
{
  "action": "restart_pod",
  "confidence": 0.85,
  "reasoning": {
    "summary": "Critical memory alert detected - pod restart recommended",
    "primary_reason": "Memory usage exceeded 90% threshold",
    "historical_context": "Previous restarts resolved similar issues",
    "oscillation_risk": "Low - no recent restart attempts"
  },
  "parameters": {
    "namespace": "default",
    "resource": "webapp-deployment",
    "strategy": "rolling_restart",
    "grace_period": "30s"
  }
}
```

### **Service Discovery and Configuration**

#### **Environment Variables for Webhook Service**:
```bash
# AI Service Communication
AI_SERVICE_URL=http://ai-service:8093
AI_SERVICE_TIMEOUT=30s
AI_SERVICE_RETRY_COUNT=3

# Database Configuration
DB_HOST=postgres-service
DB_PORT=5432
DB_NAME=kubernaut
DB_USER=kubernaut
DB_PASSWORD=<from-secret>

# Kubernetes Configuration
KUBE_CONTEXT=
KUBE_NAMESPACE=default
DRY_RUN=false
MAX_CONCURRENT_ACTIONS=10
```

#### **Environment Variables for AI Service**:
```bash
# LLM Configuration (for real analysis)
LLM_PROVIDER=localai
LLM_ENDPOINT=http://localai-service:8080
LLM_MODEL=granite-3.0-8b-instruct

# Service Configuration
AI_SERVICE_PORT=8093
AI_METRICS_PORT=9092
AI_HEALTH_PORT=8083
LOG_LEVEL=info
```

### **Health Check Endpoints**

#### **AI Service Health Checks**:
- **Liveness**: `GET http://ai-service:8083/health`
- **Readiness**: `GET http://ai-service:8083/ready`
- **Expected Response**:
```json
{
  "healthy": true,
  "service": "ai-service",
  "uptime": "2h30m15s",
  "components": ["fallback_llm_client", "real_llm_client"],
  "issues": []
}
```

#### **Webhook Service Health Checks**:
- **Liveness**: `GET http://webhook-service:8082/health`
- **Readiness**: `GET http://webhook-service:8082/ready`
- **Expected Response**:
```json
{
  "healthy": true,
  "components": ["database", "kubernetes", "ai_service"],
  "issues": []
}
```

---

## 🚨 **TROUBLESHOOTING GUIDE**

### **Common Issues and Solutions**

#### **1. Service Communication Failures**
**Symptom**: Webhook service logs show "AI service request failed"
**Solutions**:
- Verify AI service is running: `kubectl get pods -l app.kubernetes.io/name=kubernaut-ai-service`
- Check service DNS: `kubectl exec -it webhook-pod -- nslookup ai-service`
- Validate network policies allow communication
- Check AI service logs: `kubectl logs -l app.kubernetes.io/name=kubernaut-ai-service`

#### **2. Container Image Issues**
**Symptom**: `ErrImagePull` or `ImagePullBackOff`
**Solutions**:
- Rebuild images: `make docker-build-microservices`
- Check image tags in deployment manifests
- Verify registry access if using external registry
- Use `imagePullPolicy: Never` for local Kind testing

#### **3. Database Connection Failures**
**Symptom**: Webhook service fails to start with database errors
**Solutions**:
- Verify PostgreSQL is running: `kubectl get pods -l app=postgres`
- Check database credentials in secrets
- Validate database initialization scripts
- Test connection manually: `kubectl exec -it postgres-pod -- psql -U kubernaut`

#### **4. AlertManager Integration Issues**
**Symptom**: Alerts not reaching webhook service
**Solutions**:
- Check AlertManager configuration: `kubectl get configmap alertmanager-config`
- Verify webhook service is accessible from monitoring namespace
- Test webhook endpoint manually with curl
- Check AlertManager logs: `kubectl logs -l app=alertmanager`

### **Validation Commands**

#### **Service Status Check**:
```bash
# Check all microservices
kubectl get pods -l app.kubernetes.io/part-of=kubernaut

# Check service endpoints
kubectl get svc -l app.kubernetes.io/part-of=kubernaut

# Check ingress/routes (if applicable)
kubectl get ingress -l app.kubernetes.io/part-of=kubernaut
```

#### **Log Analysis**:
```bash
# AI Service logs
kubectl logs -l app.kubernetes.io/name=kubernaut-ai-service --tail=100

# Webhook Service logs
kubectl logs -l app.kubernetes.io/name=kubernaut-webhook-service --tail=100

# Follow logs in real-time
kubectl logs -f deployment/kubernaut-webhook-service
```

#### **Performance Testing**:
```bash
# Load test AI service
for i in {1..10}; do
  curl -X POST http://localhost:8093/api/v1/analyze-alert \
    -H "Content-Type: application/json" \
    -d '{"alert":{"name":"load-test-'$i'","severity":"warning"}}' &
done

# Monitor response times
kubectl top pods -l app.kubernetes.io/part-of=kubernaut
```

---

## 📊 **SUCCESS CRITERIA**

### **Phase 1 Completion Checklist**

#### **✅ Service Independence**
- [ ] AI service runs in separate pod with independent lifecycle
- [ ] Webhook service runs in separate pod with independent lifecycle
- [ ] Services can be scaled independently
- [ ] Services can be deployed independently

#### **✅ HTTP Communication**
- [ ] Webhook service makes HTTP POST requests to AI service
- [ ] AI service responds with proper JSON format
- [ ] Error handling works for service unavailability
- [ ] Timeout and retry logic implemented

#### **✅ End-to-End Flow**
- [ ] AlertManager sends webhook to webhook service
- [ ] Webhook service calls AI service for analysis
- [ ] AI service returns action recommendation
- [ ] Webhook service executes Kubernetes actions
- [ ] Action results stored in database

#### **✅ Monitoring and Observability**
- [ ] All services expose Prometheus metrics
- [ ] Health checks respond correctly
- [ ] Logs are structured and searchable
- [ ] Service discovery works in Kubernetes

#### **✅ Testing**
- [ ] Unit tests pass for both services
- [ ] Integration tests pass with HTTP communication
- [ ] End-to-end tests validate complete flow
- [ ] Load testing shows acceptable performance

### **Performance Targets**
- **Alert Processing Time**: <5 seconds end-to-end
- **AI Service Response Time**: <3 seconds for analysis
- **Service Availability**: >99% uptime
- **Resource Usage**: <1GB memory per service under normal load

---

## 🔮 **FUTURE PHASES ROADMAP**

### **Phase 2: Service Mesh and Advanced Patterns**
- **Service Mesh**: Implement Istio or Linkerd for traffic management
- **Circuit Breakers**: Add resilience patterns between services
- **Distributed Tracing**: Implement OpenTelemetry for request tracing
- **Load Balancing**: Advanced routing and load balancing strategies

### **Phase 3: Event-Driven Architecture**
- **Message Queues**: Implement NATS or RabbitMQ for async communication
- **Event Sourcing**: Store events for audit and replay capabilities
- **CQRS**: Separate read and write models for better scalability
- **Saga Patterns**: Handle distributed transactions

### **Phase 4: Advanced Microservices**
- **Context API Service**: Extract context orchestration into separate service
- **Workflow Engine Service**: Extract workflow management
- **Notification Service**: Extract notification handling
- **Data Service**: Centralized data management and caching

---

## 📁 **KEY FILES REFERENCE**

### **Implementation Files**
- `cmd/ai-service/main.go` - AI service implementation
- `cmd/webhook-service/main.go` - Webhook service implementation
- `pkg/ai/http/client.go` - HTTP client for AI service (TO CREATE)
- `docker/ai-service.Dockerfile` - AI service container
- `docker/webhook-service.Dockerfile` - Webhook service container

### **Deployment Files**
- `deploy/microservices/ai-service-deployment.yaml` - AI service K8s manifests
- `deploy/microservices/webhook-service-deployment.yaml` - Webhook service K8s manifests
- `test/manifests/monitoring/alertmanager-config.yaml` - AlertManager configuration

### **Test Files**
- `cmd/ai-service/main_test.go` - AI service tests
- `cmd/webhook-service/main_test.go` - Webhook service tests
- `test/integration/microservices/` - Integration tests (TO CREATE)

### **Configuration Files**
- `Makefile` - Build and deployment targets
- `scripts/bootstrap-dev-environment.sh` - Development environment setup
- `.env.integration` - Integration test environment variables

---

## 🎯 **IMMEDIATE NEXT ACTIONS**

1. **START HERE**: Implement HTTP client in `pkg/ai/http/client.go`
2. **THEN**: Update webhook service to use HTTP client
3. **THEN**: Deploy both services to Kind cluster
4. **THEN**: Test end-to-end alert processing flow
5. **FINALLY**: Update integration tests and documentation

**Estimated Total Time**: 6-8 hours for complete Phase 1 implementation

---

*This document contains all necessary context to resume microservices implementation work without requiring additional information or context gathering.*
