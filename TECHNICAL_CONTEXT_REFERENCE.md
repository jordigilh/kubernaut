# Technical Context Reference for Session Resumption

## 🏗️ ARCHITECTURE OVERVIEW

### **Core System Architecture**
```
kubernaut/
├── AI/ML Layer (pkg/ai/)
│   ├── holmesgpt/           # Primary AI service integration
│   ├── llm/                 # Enhanced LLM client (RULE 12 compliant)
│   └── insights/            # Analytics and effectiveness assessment
├── Workflow Engine (pkg/workflow/engine/)
│   ├── intelligent_workflow_builder_impl.go  # AI-generated workflows
│   ├── models.go            # Core type definitions (RECENTLY FIXED)
│   └── ai_service_integration.go             # AI integration patterns
├── Platform Layer (pkg/platform/)
│   ├── k8s/                 # Kubernetes operations (25+ actions)
│   └── monitoring/          # Prometheus/AlertManager integration
└── Storage Layer (pkg/storage/)
    ├── vector/              # PostgreSQL + pgvector
    └── embedding/           # Multi-provider embedding generation
```

### **Integration Patterns Applied**

#### **RULE 12 COMPLIANCE - Enhanced LLM Client Pattern**:
```go
// ✅ CURRENT (Post-migration):
type Client interface {
    AnalyzeAlert(ctx context.Context, alert interface{}) (*AnalyzeAlertResponse, error)
    GenerateWorkflow(ctx context.Context, objective *WorkflowObjective) (*WorkflowGenerationResult, error)
    OptimizeWorkflow(ctx context.Context, workflow interface{}, executionHistory interface{}) (interface{}, error)
    SuggestOptimizations(ctx context.Context, workflow interface{}) (interface{}, error)
    // ... 20+ enhanced methods
}

// ❌ DEPRECATED (Removed):
// type SelfOptimizer interface { ... }
// type AIConditionEvaluator interface { ... }
```

#### **Error Handling Standards** (From Technical Implementation Rules):
```go
// ✅ REQUIRED PATTERN:
if err != nil {
    return fmt.Errorf("operation description: %w", err)
}

// ✅ STRUCTURED LOGGING:
logger.WithError(err).WithField("operation", "validate").Error("validation failed")

// ❌ CURRENT VIOLATIONS (82 errcheck issues):
defer resp.Body.Close()  // Should check error
defer conn.Close()       // Should check error
os.Setenv(key, value)    // Should check error
```

---

## 🧪 TESTING ARCHITECTURE

### **Three-Tier Testing Strategy** (Defense-in-Depth):
```
test/
├── unit/ (70%+ coverage)
│   ├── Real business logic components
│   ├── Mock external dependencies only
│   └── Ginkgo/Gomega BDD framework
├── integration/ (20% coverage)
│   ├── Cross-component scenarios
│   ├── Real databases with isolation
│   └── Build tags: //go:build integration
└── e2e/ (10% coverage)
    ├── Complete workflow scenarios
    ├── Kind clusters for K8s testing
    └── Production-like environments
```

### **Mock Usage Decision Matrix**:
| Component Type | Action | Reason |
|---|---|---|
| **External AI APIs** | Always mock | Cost control, reliability |
| **External Databases** | Always mock in unit tests | Speed, isolation |
| **Business Logic** | **NEVER mock** | Validate actual business logic |
| **Kubernetes APIs** | Always mock in unit tests | Safety, reproducibility |

### **Test Infrastructure Status**:
- ✅ **Integration Tests**: All compile with proper build tags
- ✅ **Helper Functions**: Consolidated, no duplication
- ✅ **Mock Factories**: Standardized patterns in `pkg/testutil/`
- ✅ **Build Tags**: Proper `//go:build integration` usage

---

## 🔧 DEVELOPMENT TOOLS & WORKFLOWS

### **Quality Assurance Commands**:
```bash
# Build verification
go build ./...                    # All packages
go test -c ./test/...            # Test compilation

# Lint analysis
golangci-lint run --timeout=10m --max-issues-per-linter=0 --max-same-issues=0
golangci-lint run --disable-all --enable=errcheck     # Error handling
golangci-lint run --disable-all --enable=staticcheck  # Code quality
golangci-lint run --disable-all --enable=unused       # Dead code

# Development environment
make bootstrap-dev               # Setup everything
make test-integration-dev        # Integration tests
make cleanup-dev                # Environment cleanup
make dev-status                 # Health check
```

### **Environment Configuration**:
```bash
# Core settings
KUBECONFIG=/path/to/kubeconfig
LOG_LEVEL=info
CONFIG_FILE=config/development.yaml

# AI/ML integration
LLM_ENDPOINT=http://192.168.1.169:8080
LLM_PROVIDER=ollama
HOLMESGPT_ENDPOINT=http://localhost:8090

# Testing
USE_MOCK_LLM=false              # Set true for CI
USE_FAKE_K8S_CLIENT=false       # Set true for unit tests
SKIP_SLOW_TESTS=false           # Set true for quick runs

# Database
DB_HOST=localhost
DB_PORT=5433
DB_NAME=action_history
```

---

## 🎯 BUSINESS REQUIREMENTS CONTEXT

### **Core Business Requirements Served**:
- **BR-AI-001**: AI confidence requirements (≥80% confidence)
- **BR-WF-001**: Workflow execution success rates (≥90%)
- **BR-ORCH-001**: Self-optimization capabilities (≥15% performance gains)
- **BR-ORCH-003**: Execution scheduling optimization
- **BR-ORCH-004**: Learning from failures (≥80% confidence)
- **BR-SEC-005**: Safety recommendation generation
- **BR-K8S-001**: 25+ production-ready Kubernetes operations

### **AI/ML Business Integration**:
```go
// Business requirement mapping in code:
type ConfidenceLevel struct {
    High   float64 // >0.8 - Execute automatically (BR-AI-001)
    Medium float64 // 0.5-0.8 - Require approval
    Low    float64 // <0.5 - Log only, no action
}

// Performance requirements:
type SystemHealthMetrics struct {
    OverallHealth    float64 // ≥85% requirement (BR-ORCH-011)
    SuccessRate      float64 // ≥90% requirement (BR-WF-001)
    // ...
}
```

---

## 🚀 DEPLOYMENT & OPERATIONS

### **Kubernetes Integration**:
```yaml
# Required RBAC permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-operator
rules:
- apiGroups: [""]
  resources: ["pods", "nodes", "events", "configmaps", "secrets"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
```

### **Supported Kubernetes Operations** (25+ actions):
```go
// Scaling & Resource Management
scale_deployment     // Horizontal scaling with validation
increase_resources   // Vertical scaling with limits
update_hpa          // HPA modifications with safety bounds

// Pod & Application Lifecycle
restart_pod         // Safe restart with readiness checks
rollback_deployment // Rollback with revision validation
quarantine_pod      // Pod isolation for investigation

// Node Operations
drain_node          // Graceful draining with timeout
cordon_node         // Mark unschedulable with confirmation
restart_daemonset   // DaemonSet restart with rolling update
```

### **Safety Validation Framework**:
```go
type SafetyValidator interface {
    ValidateAction(ctx context.Context, action ActionType, params map[string]interface{}) error
    CheckPrerequisites(ctx context.Context, resource ResourceSpec) error
    ValidatePermissions(ctx context.Context, namespace, resource string) error
}

// Safety checks:
// 1. Resource existence confirmation
// 2. Resource state validation
// 3. Dependency checking
// 4. RBAC permission verification
// 5. Impact assessment (blast radius)
```

---

## 🔍 AI/ML TECHNICAL DETAILS

### **Supported AI Providers**:
| Provider | Use Case | Integration Status |
|---|---|---|
| **HolmesGPT** | Primary AI service | ✅ Production ready |
| **OpenAI** | GPT-3.5, GPT-4 models | ✅ Integrated |
| **Anthropic** | Claude models | ✅ Integrated |
| **Azure OpenAI** | Enterprise deployment | ✅ Integrated |
| **Ollama** | Local LLM deployment | ✅ Integrated |
| **Ramalama** | Local model serving | ✅ Integrated |

### **AI Response Processing Pipeline**:
1. **Structure Validation**: Response schema verification
2. **Confidence Scoring**: AI recommendation confidence evaluation
3. **Safety Validation**: Safety policy compliance checking
4. **Business Rule Validation**: Business logic alignment verification

### **Vector Database Integration**:
```go
// PostgreSQL + pgvector for similarity search
vectorDB := vector.NewClient(config.VectorDB)
embeddings, err := vectorDB.SimilaritySearch(ctx, query, limit)

// Multi-provider embedding generation
type EmbeddingProvider interface {
    GenerateEmbedding(ctx context.Context, text string) ([]float64, error)
}

// Supported: OpenAI, HuggingFace, local models
```

---

## 🛡️ SECURITY & SAFETY PATTERNS

### **Kubernetes Safety Principles**:
1. **Validation Before Action**: Always validate resources exist
2. **Dry-Run Support**: Use Kubernetes dry-run mode
3. **Rollback Capability**: Ensure operations can be reversed
4. **Timeout Enforcement**: All operations have explicit timeouts
5. **RBAC Compliance**: Respect role-based access controls

### **AI Safety Measures**:
```go
// Circuit breaker for AI service calls
breaker := circuitbreaker.New(&Config{
    Timeout:     30 * time.Second,
    MaxRequests: 100,
    Interval:    60 * time.Second,
})

// Fallback strategies:
// 1. Primary: HolmesGPT with full context
// 2. Secondary: Direct LLM provider with reduced context
// 3. Fallback: Rule-based decision making
// 4. Emergency: Safe default actions only
```

---

## 📊 MONITORING & OBSERVABILITY

### **Metrics Collection**:
```go
// AI service metrics
metrics.Counter("ai_requests_total").WithLabelValues(provider).Inc()
metrics.Histogram("ai_response_duration").Observe(duration.Seconds())

// Workflow execution metrics
metrics.Counter("workflow_executions_total").WithLabelValues(status).Inc()
metrics.Histogram("workflow_duration").Observe(duration.Seconds())

// Kubernetes operation metrics
metrics.Counter("k8s_operations_total").WithLabelValues(operation, result).Inc()
```

### **Health Checks**:
```go
// Component health endpoints
func (c *AIClient) HealthCheck(ctx context.Context) error {
    _, err := c.Ping(ctx)
    return err
}

// System health monitoring
type SystemHealthMetrics struct {
    OverallHealth    float64 // ≥85% requirement
    SuccessRate      float64 // ≥90% requirement
    ActiveWorkflows  int
    SystemThroughput float64
    AlertsActive     int
}
```

---

## 🔄 CONFIGURATION MANAGEMENT

### **Configuration Pattern**:
```go
type Config struct {
    Database    DatabaseConfig    `yaml:"database"`
    AI          AIConfig         `yaml:"ai"`
    Kubernetes  K8sConfig        `yaml:"kubernetes"`
    Monitoring  MonitoringConfig `yaml:"monitoring"`
}

// Environment variable overrides
func LoadConfig() (*Config, error) {
    config := &Config{}

    // Load from YAML
    if err := yaml.Unmarshal(configData, config); err != nil {
        return nil, err
    }

    // Override with environment variables
    if endpoint := os.Getenv("LLM_ENDPOINT"); endpoint != "" {
        config.AI.Endpoint = endpoint
    }

    return config.Validate()
}
```

### **Database Configuration**:
```go
// PostgreSQL with connection pooling
db := postgresql.NewPool(config.Database)

// Vector database operations
vectorDB := vector.NewClient(config.VectorDB)

// Action history repository
repo := actionhistory.NewRepository(db, logger)
```

---

## 🎯 PERFORMANCE OPTIMIZATION

### **Caching Strategy**:
```go
// Redis for distributed caching
cache := redis.NewClient(config.Redis)
embeddings := cache.GetEmbeddings(query)
if embeddings == nil {
    embeddings = generateEmbeddings(query)
    cache.SetEmbeddings(query, embeddings, ttl)
}

// Batch processing for efficiency
alerts := collectAlertsForBatch(batchSize)
responses := aiClient.AnalyzeBatch(ctx, alerts)
```

### **Concurrency Patterns**:
```go
// Worker pools with resource limits
pool := workerpool.New(maxWorkers)

// Circuit breakers for external services
breaker := circuitbreaker.New(failureThreshold)

// Prefer channels over shared memory
results := make(chan ProcessingResult, bufferSize)
```

---

## 🔗 INTEGRATION POINTS

### **External Service Integration**:
- **Prometheus**: Metrics collection and alerting
- **AlertManager**: Alert routing and notification
- **PostgreSQL**: Primary data storage + vector search
- **Redis**: Distributed caching and session storage
- **Kubernetes**: Container orchestration and operations

### **Internal Component Integration**:
```go
// Workflow Engine → AI Services
workflowEngine.SetLLMClient(llmClient)

// AI Services → Vector Database
aiClient.SetVectorDB(vectorDB)

// Platform → Monitoring
k8sClient.SetMetricsCollector(metricsCollector)
```

---

**Technical Context Status**: 📋 **COMPREHENSIVE & CURRENT**
**Last Updated**: September 26, 2025
**Next Review**: After Phase 1 completion (error handling fixes)
