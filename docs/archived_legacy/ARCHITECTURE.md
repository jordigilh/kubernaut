# System Architecture

## Overview

Modern hybrid architecture combining Go-based analytics with Python REST API integration for HolmesGPT. Designed for production Kubernetes environments with direct context injection replacing deprecated MCP Bridge patterns.

## System Architecture

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        subgraph "Go Application Layer"
            A[Prometheus Alerts SLM<br/>Go Service]
            B[Enhanced Assessor]
            C[Analytics Engine]
            D[Vector Database]
            E[REST API Client]

            A --> B
            A --> C
            A --> D
            B --> E
        end

        subgraph "Python API Layer"
            F[FastAPI Service<br/>Port 8000]
            G[HolmesGPT Service]
            H[HolmesGPT Wrapper]
            I[Cache Layer]
            J[Metrics & Logging]
            KC[Kubernetes Context<br/>Provider]
            AH[Action History<br/>Context Provider]

            F --> G
            G --> H
            G --> KC
            G --> AH
            F --> I
            F --> J
        end

        subgraph "External Services"
            K[HolmesGPT v0.13.1<br/>Python Library]
            L[LLM Providers]
            M[Kubernetes API]
            N[Prometheus]
            O[Redis Cache]
            SQLite[SQLite DB<br/>Action History]

            H --> K
            K --> L
            KC --> M
            AH --> SQLite
            I --> O
        end

                subgraph "LLM Providers"
            P[OpenAI GPT-4]
            Q[Anthropic Claude]
            R[Azure OpenAI]
            S[AWS Bedrock]
            T[Ollama/Ramalama<br/>On-Premises]

            L --> P
            L --> Q
            L --> R
            L --> S
            L --> T
        end
    end

    E --> F

    style A fill:#e1f5fe
    style F fill:#f3e5f5
    style K fill:#e8f5e8
    style L fill:#fff3e0
```

## Components

### Go Application Layer
- EnhancedAssessor: Orchestrates traditional assessment with AI analysis
- AnalyticsEngine: Cost-benefit analysis and pattern learning
- VectorDatabase: Historical pattern similarity matching
- REST API Client: HTTP client for Python service communication

### Python API Service
- FastAPI application with direct HolmesGPT v0.13.1 integration
- Multi-LLM support (OpenAI, Anthropic, Azure, AWS, Ollama/Ramalama)
- Fail-fast startup validation
- Async processing with connection pooling
- **Context Providers**: Replace deprecated MCP Bridge with direct context injection
  - KubernetesContextProvider: Real-time cluster data (pods, nodes, events, quotas)
  - ActionHistoryContextProvider: Historical action analysis and oscillation detection

## Data Flow

### Request Processing Flow

```mermaid
sequenceDiagram
    participant A as Alert Source
    participant G as Go Service
    participant E as Enhanced Assessor
    participant R as REST Client
    participant P as Python API
    participant KC as K8s Context Provider
    participant AH as Action History Provider
    participant H as HolmesGPT
    participant L as LLM Provider
    participant K as Kubernetes

    A->>G: Alert Received
    G->>E: Process Alert

    Note over E: Traditional Assessment
    E->>E: Calculate Effectiveness Score
    E->>E: Analyze Historical Patterns
    E->>E: Perform Cost Analysis

    Note over E: Context Enrichment
    E->>K: Get Pod/Deployment Info
    K-->>E: Resource Details
    E->>K: Get Prometheus Metrics
    K-->>E: Metrics Data

    Note over E,P: REST API Integration
    E->>R: Build Investigation Request
    R->>P: POST /investigate

    Note over P,K: Context Enrichment
    P->>KC: Get Kubernetes Context
    KC->>K: Query Pods/Nodes/Events
    K-->>KC: Cluster Data
    KC-->>P: Enriched K8s Context

    P->>AH: Get Action History
    AH-->>P: Historical Actions & Patterns

    P->>H: Process with Enriched Context
    H->>L: Enhanced Prompt + Context
    L-->>H: AI Analysis
    H-->>P: Structured Response
    P-->>R: JSON Response
    R-->>E: Investigation Result

    Note over E: Decision Synthesis
    E->>E: Combine Traditional + AI Analysis
    E->>E: Apply Safety Controls
    E->>G: Final Decision
    G->>K: Execute Actions
```

### Data Flow Diagram

```mermaid
flowchart LR
    subgraph "Input"
        A1[Prometheus Alert]
        A2[Historical Data]
        A3[Kubernetes Context]
    end

    subgraph "Go Processing"
        B1[Traditional Assessment]
        B2[Pattern Matching]
        B3[Cost Analysis]
        B4[Context Builder]
    end

    subgraph "REST Integration"
        C1[HTTP Request]
        C2[JSON Payload]
        C3[Enhanced Context]
    end

    subgraph "Python Processing"
        D1[Request Validation]
        D2[Prompt Enhancement]
        D3[HolmesGPT API Call]
        D4[Response Processing]
    end

    subgraph "AI Analysis"
        E1[LLM Processing]
        E2[Root Cause Analysis]
        E3[Recommendations]
        E4[Confidence Scoring]
    end

    subgraph "Output"
        F1[Decision Synthesis]
        F2[Action Execution]
        F3[Monitoring & Logging]
    end

    A1 --> B1
    A2 --> B2
    A3 --> B4

    B1 --> C1
    B2 --> C2
    B3 --> C2
    B4 --> C3

    C1 --> D1
    C2 --> D2
    C3 --> D3

    D2 --> E1
    D3 --> E1
    E1 --> E2
    E2 --> E3
    E3 --> E4

    D4 --> F1
    E4 --> F1
    F1 --> F2
    F2 --> F3

    style B1 fill:#e1f5fe
    style D3 fill:#f3e5f5
    style E1 fill:#fff3e0
    style F2 fill:#e8f5e8
```

## Configuration

Go Service:
```yaml
effectiveness:
  enable_holmes_gpt: true
  holmes_api:
    base_url: "http://holmesgpt-api:8000"
    timeout: "300s"
```

Python Service:
```env
# Cloud LLM Provider
HOLMES_LLM_PROVIDER=openai
OPENAI_API_KEY=your_key
HOLMES_DEFAULT_MODEL=gpt-4

# Or Local On-Premises LLM
HOLMES_LLM_PROVIDER=ollama
OLLAMA_BASE_URL=http://ollama:11434
HOLMES_DEFAULT_MODEL=llama3.1:8b
```

## Performance

| Operation | Latency | Resource Usage |
|-----------|---------|----------------|
| Go Service | 50-200ms | 256Mi-512Mi |
| Python Service | 1-5s | 512Mi-2Gi |

## Deployment

### Deployment Architecture

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        subgraph "Ingress Layer"
            I[NGINX Ingress<br/>Load Balancer]
        end

        subgraph "Application Layer"
            subgraph "Go Services"
                G1[Go Service<br/>Replica 1]
                G2[Go Service<br/>Replica 2]
                G3[Go Service<br/>Replica 3]
            end

            subgraph "Python Services"
                P1[Python API<br/>Replica 1]
                P2[Python API<br/>Replica 2]
            end
        end

        subgraph "Data Layer"
            Redis[Redis Cache<br/>StatefulSet]
            Postgres[PostgreSQL<br/>StatefulSet]
            Vector[Vector Database<br/>StatefulSet]
        end

        subgraph "Monitoring Layer"
            Prom[Prometheus<br/>Metrics Collection]
            Graf[Grafana<br/>Dashboards]
        end

        subgraph "Configuration"
            CM[ConfigMaps<br/>App Config]
            Sec[Secrets<br/>API Keys]
        end
    end

    subgraph "External Services"
        LLM[LLM Providers<br/>OpenAI/Anthropic/Azure/AWS]
        LOCAL[Local LLM<br/>Ollama/Ramalama]
        K8sAPI[Kubernetes API<br/>Cluster Operations]
    end

    I --> G1
    I --> G2
    I --> G3

    G1 --> P1
    G2 --> P1
    G3 --> P2
    G1 --> P2
    G2 --> P2
    G3 --> P1

    G1 --> Redis
    G2 --> Redis
    G3 --> Redis
    G1 --> Postgres
    G2 --> Postgres
    G3 --> Postgres
    G1 --> Vector
    G2 --> Vector
    G3 --> Vector

    P1 --> Redis
    P2 --> Redis
    P1 --> LLM
    P2 --> LLM
    P1 --> LOCAL
    P2 --> LOCAL

    Prom --> G1
    Prom --> G2
    Prom --> G3
    Prom --> P1
    Prom --> P2

    Graf --> Prom

    G1 --> CM
    G2 --> CM
    G3 --> CM
    P1 --> CM
    P2 --> CM

    P1 --> Sec
    P2 --> Sec

    G1 --> K8sAPI
    G2 --> K8sAPI
    G3 --> K8sAPI

    style G1 fill:#e1f5fe
    style G2 fill:#e1f5fe
    style G3 fill:#e1f5fe
    style P1 fill:#f3e5f5
    style P2 fill:#f3e5f5
    style LLM fill:#fff3e0
```

**Deployment Configuration:**
- Go Service: 3 replicas for high availability
- Python API: 2 replicas for LLM processing
- StatefulSets for persistent data storage
- Secrets management for LLM API keys

## Monitoring

### Monitoring Architecture

```mermaid
graph TB
    subgraph "Application Services"
        G[Go Service<br/>:8080]
        P[Python API<br/>:8000]
    end

    subgraph "Metrics Collection"
        PM[Prometheus<br/>:9090]
        PG[Pushgateway<br/>:9091]
    end

    subgraph "Visualization"
        GR[Grafana<br/>:3000]
        AL[AlertManager<br/>:9093]
    end

    subgraph "Health Checks"
        HC1[Go Health<br/>/health]
        HC2[Python Health<br/>/health]
        HC3[Readiness Probes]
        HC4[Liveness Probes]
    end

    subgraph "Metrics Endpoints"
        M1[Go Metrics<br/>/metrics]
        M2[Python Metrics<br/>/metrics]
    end

    G --> HC1
    G --> M1
    P --> HC2
    P --> M2

    PM --> M1
    PM --> M2
    PM --> PG

    GR --> PM
    AL --> PM

    HC1 --> HC3
    HC2 --> HC3
    HC1 --> HC4
    HC2 --> HC4

    style G fill:#e1f5fe
    style P fill:#f3e5f5
    style PM fill:#e8f5e8
    style GR fill:#fff3e0
```

**Key Metrics:**
- Request counts and durations
- Error rates and response codes
- HolmesGPT operation success rates
- Resource utilization (CPU, memory)
- Cache hit rates and performance

## Security

### Security Architecture

```mermaid
graph TB
    subgraph "External Access"
        U[Users/Systems]
        EXT[External APIs]
    end

    subgraph "Ingress Security"
        TLS[TLS Termination]
        AUTH[Authentication]
        RATE[Rate Limiting]
    end

    subgraph "Application Security"
        RBAC[RBAC<br/>Kubernetes API]
        SEC[Secrets Management<br/>API Keys]
        NET[Network Policies<br/>Pod-to-Pod]
    end

    subgraph "Service Layer"
        GO[Go Service<br/>mTLS]
        PY[Python API<br/>Internal Auth]
    end

    subgraph "Data Security"
        ENC[Encryption at Rest]
        AUDIT[Audit Logging]
        BACKUP[Secure Backups]
    end

    U --> TLS
    TLS --> AUTH
    AUTH --> RATE
    RATE --> GO

    GO --> RBAC
    GO --> SEC
    GO --> NET
    PY --> SEC

    GO --> PY

    GO --> ENC
    PY --> ENC
    GO --> AUDIT
    PY --> AUDIT

    SEC --> EXT

    style TLS fill:#ffebee
    style SEC fill:#ffebee
    style RBAC fill:#ffebee
    style ENC fill:#ffebee
```

**Security Controls:**
- TLS encryption for all external communications
- Kubernetes RBAC for API access control
- Secrets management for LLM API keys
- Network policies for service isolation
- Audit logging for security events
- No sensitive data persistence in Python service
