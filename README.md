# Kubernaut

An intelligent Kubernetes remediation agent that autonomously analyzes alerts and executes sophisticated automated actions using LLM-powered decision making, historical learning, and advanced pattern recognition.

## 🚀 Current Status

**Production-Ready Core System** with comprehensive testing framework and AI-enhanced decision making:
- ✅ **Core Architecture**: Go + Python hybrid system with HolmesGPT v0.13.1 integration
- ✅ **25+ Remediation Actions**: Production-ready Kubernetes operations with safety controls
- ✅ **Multi-LLM Support**: OpenAI, Anthropic, Azure, AWS Bedrock, Ollama, Ramalama
- ✅ **Historical Learning**: PostgreSQL-based action history with effectiveness scoring
- ✅ **Comprehensive Testing**: Ginkgo/Gomega framework with model comparison suite
- 🔄 **Advanced Features**: Vector database integration, RAG enhancement, workflow engine (in development)

## Architecture Overview

The system implements a sophisticated hybrid architecture combining Go-based analytics with Python REST API integration for advanced AI capabilities:

```mermaid
graph TB
    subgraph "External Systems"
        PROM[Prometheus<br/>Alerts]
        K8SAPI[Kubernetes<br/>API Server]
        LLM[LLM Providers<br/>OpenAI, Anthropic, Azure<br/>AWS, Ollama, Ramalama]
    end

    subgraph "Kubernaut System"
        subgraph "Go Service Layer"
            WH[Webhook Handler<br/>:8080]
            PROC[Alert Processor]
            EXEC[Action Executor<br/>25+ Actions]
            EFF[Effectiveness<br/>Assessor]
            K8SC[Kubernetes<br/>Client]
        end

        subgraph "Python API Layer"
            FAPI[FastAPI Service<br/>:8000]
            HGS[HolmesGPT Service<br/>v0.13.1]
            KCP[Kubernetes Context<br/>Provider]
            ACP[Action History Context<br/>Provider]
        end

        subgraph "Storage Layer"
            PG[(PostgreSQL<br/>Action History)]
            VDB[(Vector Database<br/>Pattern Matching)]
        end

        subgraph "Monitoring"
            METRICS[Prometheus Metrics<br/>:9090]
            HEALTH[Health Checks]
        end
    end

    %% External connections
    PROM --> WH
    K8SAPI --> K8SC
    LLM --> HGS

    %% Go service flow
    WH --> PROC
    PROC --> EXEC
    PROC --> EFF
    EXEC --> K8SC
    EFF --> PG

    %% Python API integration
    PROC --> FAPI
    FAPI --> HGS
    HGS --> KCP
    HGS --> ACP
    KCP --> K8SAPI
    ACP --> PG

    %% Storage connections
    EFF --> VDB
    ACP --> VDB

    %% Monitoring connections
    WH --> METRICS
    FAPI --> METRICS
    WH --> HEALTH
    FAPI --> HEALTH

    style EXEC fill:#e1f5fe
    style HGS fill:#f3e5f5
    style VDB fill:#fff3e0
    style PG fill:#e8f5e8
```

## Data Flow & Processing

The system processes alerts through an intelligent pipeline with AI-enhanced decision making:

```mermaid
sequenceDiagram
    participant Prometheus
    participant Go Service
    participant Python API
    participant HolmesGPT
    participant LLM
    participant K8s API
    participant PostgreSQL

    Prometheus->>Go Service: Alert Webhook
    Go Service->>Go Service: Process & Filter Alert

    note over Go Service,Python API: Context Enrichment Phase
    Go Service->>Python API: POST /investigate
    Python API->>K8s API: Get Pod/Node/Event Data
    K8s API-->>Python API: Cluster Context
    Python API->>PostgreSQL: Query Action History
    PostgreSQL-->>Python API: Historical Patterns

    note over Python API,LLM: AI Analysis Phase
    Python API->>HolmesGPT: Enhanced Alert + Context
    HolmesGPT->>LLM: Structured Prompt
    LLM-->>HolmesGPT: Analysis & Recommendations
    HolmesGPT-->>Python API: Action Recommendation
    Python API-->>Go Service: JSON Response

    note over Go Service,K8s API: Decision & Execution
    Go Service->>Go Service: Validate & Score Action
    Go Service->>K8s API: Execute Remediation
    K8s API-->>Go Service: Execution Result
    Go Service->>PostgreSQL: Store Action Trace

    note over Go Service,PostgreSQL: Learning & Assessment
    Go Service->>Go Service: Schedule Effectiveness Assessment
    Go Service->>PostgreSQL: Update Effectiveness Score
```

## Feature Status Matrix

Current implementation status of core capabilities:

```mermaid
graph TD
    subgraph "✅ IMPLEMENTED FEATURES"
        subgraph "Core Architecture"
            A1[Go Service Layer<br/>Alert Processing & Execution]
            A2[Python API Layer<br/>HolmesGPT v0.13.1 Integration]
            A3[Hybrid Architecture<br/>REST API Communication]
        end

        subgraph "AI & Intelligence"
            B1[Multi-LLM Support<br/>OpenAI, Anthropic, Azure, AWS, Ollama]
            B2[Context Providers<br/>Kubernetes & Action History]
            B3[Action Effectiveness Scoring<br/>PostgreSQL-based Learning]
        end

        subgraph "Kubernetes Operations"
            C1[25+ Remediation Actions<br/>Scale, Restart, Drain, etc.]
            C2[Kubernetes Client<br/>Comprehensive API Coverage]
            C3[Safety Mechanisms<br/>Validation & Timeouts]
        end

        subgraph "Testing & Quality"
            D1[Ginkgo/Gomega Framework<br/>BDD Testing]
            D2[Model Comparison Suite<br/>Automated Evaluation]
            D3[Integration Testing<br/>Fake Kubernetes Environment]
        end
    end

    subgraph "⚠️ PARTIALLY IMPLEMENTED"
        E1[Workflow Engine<br/>Core implemented, builder missing]
        E2[Vector Database<br/>Interface ready, integration pending]
        E3[Monitoring Integration<br/>Framework ready, clients stub]
    end

    subgraph "🔄 PLANNED FEATURES"
        subgraph "Intelligence Enhancement"
            F1[RAG-Enhanced Decisions<br/>Historical Context Retrieval]
            F2[Pattern Discovery Engine<br/>Cross-Resource Learning]
            F3[Intelligent Workflow Builder<br/>AI-Generated Workflows]
        end

        subgraph "Production Features"
            G1[Chaos Engineering Testing<br/>Litmus Framework Integration]
            G2[Advanced Observability<br/>Grafana Dashboards]
            G3[Kubernetes Operator<br/>Professional Distribution]
        end

        subgraph "Enterprise Capabilities"
            H1[Cost Management Context<br/>Financial Intelligence]
            H2[Security Intelligence<br/>CVE-Aware Decisions]
            H3[Multi-Cloud Integration<br/>Provider Cost APIs]
        end
    end

    style A1 fill:#c8e6c9
    style B1 fill:#c8e6c9
    style C1 fill:#c8e6c9
    style D1 fill:#c8e6c9
    style E1 fill:#fff3e0
    style E2 fill:#fff3e0
    style E3 fill:#fff3e0
    style F1 fill:#e3f2fd
    style G1 fill:#e3f2fd
    style H1 fill:#e3f2fd
```

## Core Components

### Go Service Layer (Production-Ready)
- **Webhook Handler**: Receives Prometheus alerts via HTTP webhooks
- **Alert Processor**: Filters and processes incoming alerts with contextual analysis
- **Action Executor**: Executes 25+ Kubernetes remediation actions with comprehensive safety checks
- **Effectiveness Assessor**: Learns from action outcomes and improves decision making over time
- **Kubernetes Client**: Unified client with comprehensive API coverage for all operations

### Python API Layer (HolmesGPT Integration)
- **FastAPI Service**: High-performance async API service with comprehensive monitoring
- **HolmesGPT Integration**: Direct Python API integration with HolmesGPT v0.13.1
- **Context Providers**: Intelligent context injection replacing deprecated MCP Bridge patterns
  - **Kubernetes Context Provider**: Real-time cluster data and resource analysis
  - **Action History Context Provider**: Historical action patterns and effectiveness insights

### Storage & Learning
- **PostgreSQL**: Action history storage with advanced partitioning and stored procedures
- **Vector Database**: Pattern matching and similarity search (interface ready, integration in development)
- **Effectiveness Framework**: AI-driven assessment of action outcomes with continuous learning

## Supported Remediation Actions

The system supports **25+ production-ready Kubernetes operations** across multiple categories:

### Scaling & Resource Management
- `scale_deployment` - Horizontal scaling of deployments
- `increase_resources` - Vertical scaling (CPU/memory limits)
- `update_hpa` - Horizontal Pod Autoscaler modifications
- `scale_statefulset` - StatefulSet scaling with proper ordering

### Pod & Application Lifecycle
- `restart_pod` - Safe pod restart with validation
- `rollback_deployment` - Rollback to previous deployment revision
- `quarantine_pod` - Isolate problematic pods for investigation
- `migrate_workload` - Move workloads between nodes

### Node Operations
- `drain_node` - Graceful node draining for maintenance
- `cordon_node` - Mark nodes as unschedulable
- `restart_daemonset` - Restart DaemonSet pods across nodes

### Storage Operations
- `expand_pvc` - Persistent Volume Claim expansion
- `cleanup_storage` - Clean up old data/logs when disk space is critical
- `backup_data` - Trigger emergency backups before disruptive actions
- `compact_storage` - Trigger storage compaction operations

### Network & Connectivity
- `update_network_policy` - Modify network policies for connectivity issues
- `restart_network` - Restart network components (CNI, DNS)
- `reset_service_mesh` - Reset service mesh configuration

### Database & Stateful Services
- `failover_database` - Trigger database failover to replica
- `repair_database` - Run database repair/consistency checks

### Security & Compliance
- `rotate_secrets` - Rotate compromised credentials/certificates
- `audit_logs` - Trigger detailed security audit collection

### Diagnostics & Monitoring
- `collect_diagnostics` - Gather comprehensive diagnostic data
- `enable_debug_mode` - Enable debug logging temporarily
- `create_heap_dump` - Trigger memory dumps for analysis
- `notify_only` - Notification-only mode for monitoring

All actions include comprehensive safety validation, rollback capabilities, and effectiveness tracking.

## Quick Start

### Prerequisites
- **Go 1.23.9+** for the core service
- **Python 3.11+** for HolmesGPT API integration
- **Kubernetes/OpenShift cluster** with RBAC permissions
- **PostgreSQL database** for action history storage
- **LLM provider API keys** (OpenAI, Anthropic, etc.) or local LLM setup

### Installation Methods

#### Option 1: Docker Compose (Recommended for Development)
```bash
# Clone and start all services
git clone https://github.com/jordigilh/kubernaut.git
cd prometheus-alerts-slm
docker-compose up -d

# Verify services
curl http://localhost:8080/health  # Go service
curl http://localhost:8000/health  # Python API
curl http://localhost:8000/docs    # API documentation
```

#### Option 2: Manual Build & Deploy
```bash
# Build Go service
make build

# Setup Python API
cd python-api
pip install -r requirements.txt
cp env.example .env
# Configure LLM provider credentials in .env

# Start services
./bin/prometheus-alerts-slm --config config/development.yaml
cd python-api && uvicorn app.main:app --host 0.0.0.0 --port 8000
```

#### Option 3: Kubernetes Deployment
```bash
# Deploy with Kustomize
kubectl apply -k deploy/

# Or use individual manifests
kubectl apply -f deploy/manifests/
```

### Configuration

#### Go Service Configuration
```yaml
# config/development.yaml
database:
  enabled: true
  host: localhost
  port: 5432
  database: kubernaut
  username: kubernaut
  password: your_password

effectiveness:
  enable_holmes_gpt: true
  holmes_api:
    base_url: "http://localhost:8000"
    timeout: "300s"

actions:
  dry_run: false
  timeout: "5m"
```

#### Python API Configuration
```env
# python-api/.env

# Required: LLM Provider
HOLMES_LLM_PROVIDER=openai
OPENAI_API_KEY=sk-your-api-key-here
HOLMES_DEFAULT_MODEL=gpt-4

# Alternative providers:
# HOLMES_LLM_PROVIDER=anthropic
# ANTHROPIC_API_KEY=sk-ant-your-key

# For local LLM (no API key required):
# HOLMES_LLM_PROVIDER=ollama
# OLLAMA_BASE_URL=http://localhost:11434
# HOLMES_DEFAULT_MODEL=llama3.1:8b

# Optional performance settings
CACHE_ENABLED=true
CACHE_TTL=300
REQUEST_TIMEOUT=600
```

## Development Workflow

### Testing Framework
The project uses **Ginkgo/Gomega** for BDD-style testing with comprehensive coverage:

```bash
# Run all tests
make test

# Run integration tests
make test-integration

# Run with coverage
make test-coverage

# Model comparison tests
make model-comparison
```

### Test Organization
```
test/
├── integration/        # Integration tests with fake Kubernetes
│   ├── core/          # Core functionality tests
│   ├── e2e/           # End-to-end scenarios
│   ├── production/    # Production readiness tests
│   └── shared/        # Shared test utilities
└── scenarios/         # Test scenarios and fixtures
```

### Adding New Actions
```go
// 1. Define action in pkg/executor/actions.go
func (e *Executor) executeMyNewAction(ctx context.Context, params map[string]interface{}) error {
    // Implementation with safety validation
    return nil
}

// 2. Register action in pkg/executor/registry.go
func init() {
    registerAction("my_new_action", (*Executor).executeMyNewAction)
}

// 3. Add comprehensive tests
var _ = Describe("MyNewAction", func() {
    It("should handle the action safely", func() {
        // Test implementation
    })
})
```

## Deployment Options

### Development Environment
- **Docker Compose**: Full stack with PostgreSQL and Redis
- **Local Services**: Go service + Python API + external dependencies
- **Kind Cluster**: Local Kubernetes testing with `scripts/setup-kind-cluster.sh`

### Staging/Production
- **Kubernetes Manifests**: Production-ready deployments in `deploy/manifests/`
- **Helm Charts**: *(Planned)* - Professional Kubernetes distribution
- **Operator Pattern**: *(In Development)* - Kubernetes operator for enterprise deployment

### Resource Requirements

| Component | CPU | Memory | Storage | Notes |
|-----------|-----|---------|---------|-------|
| Go Service | 0.1-0.5 CPU | 128-512Mi | 1Gi | Scales with alert volume |
| Python API | 0.2-1.0 CPU | 512Mi-2Gi | 1Gi | Depends on LLM model size |
| PostgreSQL | 0.1-0.5 CPU | 256-1Gi | 10-50Gi | Action history storage |
| Vector DB | 0.2-1.0 CPU | 1-4Gi | 5-20Gi | Pattern matching (optional) |

### High Availability
- **Go Service**: Stateless, can run multiple replicas
- **Python API**: Stateless, supports horizontal scaling
- **Database**: PostgreSQL with replication for production
- **Load Balancing**: Kubernetes services with ingress controllers

## Monitoring & Observability

### Prometheus Metrics
Both services expose comprehensive metrics:
- **Go Service**: `:9090/metrics` - Action execution, effectiveness scores, system health
- **Python API**: `:8000/metrics` - HolmesGPT performance, context provider usage, cache hits

### Health Endpoints
- **Go Service**: `GET /health`, `GET /ready` - Service and dependency health
- **Python API**: `GET /health` - LLM connectivity and component status

### Logging
Structured JSON logging with configurable levels:
```bash
# Environment configuration
LOG_LEVEL=info          # debug, info, warn, error
LOG_FORMAT=json         # json, text
```

### Dashboards *(Planned)*
- **Operational Dashboard**: Real-time system metrics and alert processing
- **Intelligence Dashboard**: AI decision quality, model performance, learning trends
- **Business Impact Dashboard**: Cost savings, incident resolution times, SLA metrics

## Security Considerations

### RBAC Configuration
The system requires specific Kubernetes permissions:
```yaml
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
# ... additional permissions
```

### Secrets Management
- **LLM API Keys**: Kubernetes secrets with rotation capabilities
- **Database Credentials**: Encrypted connection strings
- **Certificates**: TLS certificates for secure communication

### Network Security
- **Service Mesh**: Compatible with Istio/Linkerd for mTLS
- **Network Policies**: Kubernetes network policies for traffic isolation
- **Ingress Security**: TLS termination and authentication at ingress layer

## Performance Characteristics

### Response Times
| Operation | Typical | 95th Percentile | Notes |
|-----------|---------|-----------------|-------|
| Alert Processing | 1-3s | 5s | Without AI analysis |
| AI Decision Making | 2-8s | 15s | Includes LLM inference |
| Action Execution | 0.5-2s | 5s | Kubernetes API calls |
| Effectiveness Assessment | 0.1-0.5s | 1s | Database queries |

### Throughput
- **Alert Ingestion**: 100+ alerts/minute per Go service instance
- **Concurrent Processing**: 10-50 simultaneous alert investigations
- **AI Analysis**: 5-20 requests/minute per Python API instance (depends on LLM provider)

### Scalability
- **Horizontal Scaling**: Both Go and Python services support multiple replicas
- **Database Scaling**: PostgreSQL with read replicas and connection pooling
- **LLM Scaling**: Multiple model instances with intelligent routing *(planned)*

## Roadmap & Future Features

### Phase 1: Intelligence Enhancement (Q1 2025)
- **Vector Database Integration**: Semantic search for action history patterns
- **RAG-Enhanced Decisions**: Historical context retrieval for smarter recommendations
- **Advanced Model Comparison**: Evaluation of 6+ additional 2B parameter models
- **Intelligent Workflow Builder**: AI-generated multi-step remediation workflows

### Phase 2: Production Features (Q2 2025)
- **Kubernetes Operator**: Professional enterprise distribution via OperatorHub
- **Chaos Engineering**: Litmus framework integration for resilience testing
- **Advanced Observability**: Grafana dashboards with business intelligence
- **Security Intelligence**: CVE-aware decision making and compliance validation

### Phase 3: Enterprise Capabilities (Q3+ 2025)
- **Cost Management Integration**: Multi-cloud cost APIs for financial intelligence
- **Multi-Tenant Architecture**: Namespace isolation and policy management
- **Governance Workflows**: Approval systems for high-risk operations
- **Advanced Analytics**: Predictive maintenance and capacity planning

For detailed roadmap information, see [ROADMAP.md](docs/ROADMAP.md).

## Contributing

We welcome contributions! The project follows standard Go and Python development practices:

### Development Environment
```bash
# Setup development environment
make setup-dev

# Run tests before committing
make test
make lint

# Format code
make format
```

### Code Standards
- **Go**: Standard Go conventions with comprehensive error handling
- **Python**: PEP 8 compliance with type hints and async patterns
- **Testing**: BDD tests with Ginkgo/Gomega, >80% coverage requirement
- **Documentation**: Comprehensive inline documentation and architectural decisions

### Pull Request Process
1. **Feature Branch**: Create feature branch from `main`
2. **Implementation**: Implement with comprehensive tests and documentation
3. **Testing**: Ensure all tests pass including integration tests
4. **Review**: Code review with focus on safety and production readiness
5. **Merge**: Squash merge after approval

## Documentation

### Technical Documentation
- **[Architecture Guide](docs/ARCHITECTURE.md)**: Detailed system architecture and design decisions
- **[Deployment Guide](docs/DEPLOYMENT.md)**: Production deployment strategies and configurations
- **[API Documentation](python-api/README.md)**: Python API service documentation
- **[Testing Framework](docs/TESTING_FRAMEWORK.md)**: Comprehensive testing approach and utilities

### Analysis & Planning
- **[Competitive Analysis](docs/COMPETITIVE_ANALYSIS.md)**: Market positioning and feature comparison
- **[Vector Database Analysis](docs/VECTOR_DATABASE_ANALYSIS.md)**: Storage architecture decisions
- **[RAG Enhancement Analysis](docs/RAG_ENHANCEMENT_ANALYSIS.md)**: AI decision enhancement strategies
- **[Workflow Documentation](docs/WORKFLOWS.md)**: Multi-step remediation orchestration

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

## Support & Community

- **Issues**: [GitHub Issues](https://github.com/jordigilh/kubernaut/issues)
- **Discussions**: [GitHub Discussions](https://github.com/jordigilh/kubernaut/discussions)
- **Documentation**: Comprehensive guides in the `docs/` directory

---

**Kubernaut** represents the next evolution of Kubernetes operations - combining the reliability of traditional automation with the intelligence of modern AI to create a self-improving, context-aware remediation system that learns from every action and continuously enhances your cluster's resilience.