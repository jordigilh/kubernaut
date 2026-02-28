# Slide 7: The Kubernaut Architecture

**Act**: 3 - The Solution  
**Theme**: "How Kubernaut Delivers Autonomous Remediation"

---

## 🎯 Slide Goal

**Show the "hamburger" architecture** - prove engineering depth + CRD-native design.

---

## 📖 Content

### Title
**"Kubernaut V1 Architecture: Kubernetes-Native by Design"**

### Subtitle
*"12 microservices, CRD-based orchestration, AI-powered intelligence"*

---

## 🏗️ The Hamburger Architecture

```mermaid
graph TB
    subgraph TOP[" "]
        direction LR
        Orchestrator["<b>🎯 ORCHESTRATOR</b><br/>RemediationOrchestrator<br/>(CRD-based workflow coordination)"]
    end
    
    subgraph MIDDLE[" "]
        direction LR
        
        subgraph Left["Signal Normalization"]
            Gateway["Gateway<br/>(Multi-signal ingestion)"]
            Processor["RemediationProcessor<br/>(Context enrichment)"]
            Context["Context API<br/>(Dynamic context)"]
            Monitoring["Effectiveness Monitor<br/>(Learning loop)"]
        end
        
        subgraph Center["AI Investigation"]
            AIAnalysis["AIAnalysis<br/>(Root cause)"]
            Holmes["HolmesGPT-API<br/>(LLM integration)"]
        end
        
        subgraph Right1["Remediation"]
            Workflow["Workflow Engine<br/>(Action generation)"]
            K8sExec["K8s Execution<br/>(Safe actions)"]
        end
        
        subgraph Right2["Notification"]
            Notify["Notification Service<br/>(Stakeholder alerts)"]
        end
    end
    
    subgraph BOTTOM[" "]
        direction LR
        Storage["<b>📊 DATA STORAGE</b><br/>PostgreSQL (action history)<br/>Redis (caching, state)<br/>Vector DB (pattern learning)"]
    end
    
    TOP --> MIDDLE
    MIDDLE --> BOTTOM
    
    style TOP fill:#4CAF50,stroke:#000,stroke-width:3px
    style BOTTOM fill:#2196F3,stroke:#000,stroke-width:3px
    style Left fill:#FFF3E0,stroke:#FF9800,stroke-width:2px
    style Center fill:#E3F2FD,stroke:#2196F3,stroke-width:2px
    style Right1 fill:#F3E5F5,stroke:#9C27B0,stroke-width:2px
    style Right2 fill:#E8F5E9,stroke:#4CAF50,stroke-width:2px
```

---

## 🔍 Layer Breakdown

### Top Layer: Orchestrator
**RemediationOrchestrator (CRD Controller)**

**What It Does**:
- Watches for `AIAnalysis` CRDs (incident detection)
- Creates `WorkflowExecution` CRDs (remediation tasks)
- Monitors `KubernetesExecution` (DEPRECATED - ADR-025) CRDs (action status)
- Tracks remediation lifecycle end-to-end

**Key Features**:
- ✅ CRD-native (no external message bus required)
- ✅ Kubernetes-native orchestration
- ✅ Built-in state management via etcd
- ✅ Audit trail via CRD events

---

### Middle Layer - Section 1: Signal Normalization
**Multi-Signal Ingestion & Context Enrichment**

```mermaid
graph LR
    A1[Prometheus Alerts] --> Gateway
    A2[CloudWatch Alarms] --> Gateway
    A3[Kubernetes Events] --> Gateway
    A4[Custom Webhooks] --> Gateway
    
    Gateway --> Processor[RemediationProcessor<br/>Dedupe + Priority]
    Processor --> Context[Context API<br/>Topology + Dependencies]
    Context --> Monitoring[Effectiveness Monitor<br/>Learning Loop]
    
    Monitoring --> NextStep[AIAnalysis CRD<br/>created]
    
    style Gateway fill:#FF9800,stroke:#000,stroke-width:2px,color:#fff
    style NextStep fill:#4CAF50,stroke:#000,stroke-width:2px,color:#fff
```

**Key Features**:
- **Gateway**: Ingests from ANY monitoring tool (vendor-neutral)
- **RemediationProcessor**: Deduplication, correlation, prioritization
- **Context API**: Dynamic topology discovery (what's connected to what)
- **Effectiveness Monitor**: Learns from past remediations (feedback loop)

---

### Middle Layer - Section 2: AI Investigation
**Root Cause Analysis via AI**

```mermaid
sequenceDiagram
    participant CRD as AIAnalysis CRD
    participant AI as AIAnalysis Service
    participant Holmes as HolmesGPT-API
    participant LLM as External LLM
    
    CRD->>AI: New incident detected
    AI->>Holmes: Request root cause analysis
    Holmes->>LLM: Send logs, metrics, topology
    LLM->>Holmes: Root cause + remediation
    Holmes->>AI: Structured analysis
    AI->>CRD: Update with findings
    
    Note over CRD: Status: Analysis Complete
```

**Key Features**:
- **AIAnalysis Service**: Orchestrates AI investigation
- **HolmesGPT-API**: Multi-LLM integration (OpenAI, Anthropic, Local LLMs)
- **Context-Aware**: Uses topology, history, patterns from Context API
- **Structured Output**: Generates actionable remediation recommendations

---

### Middle Layer - Section 3: Remediation Execution
**Workflow Generation & Safe Execution**

```mermaid
graph TB
    Analysis[AIAnalysis CRD<br/>with recommendations] --> Workflow[Workflow Engine]
    
    Workflow --> Gen[Generate Actions<br/>pod restart, scale, rollback, etc.]
    Gen --> Safety[Safety Validation<br/>RBAC, quotas, dry-run]
    
    Safety --> Execute[K8s Execution Service]
    Execute --> K8s[Kubernetes API]
    
    K8s --> Status[KubernetesExecution (DEPRECATED - ADR-025) CRD<br/>Status: Running/Complete/Failed]
    
    style Workflow fill:#9C27B0,stroke:#000,stroke-width:2px,color:#fff
    style Safety fill:#FF9800,stroke:#000,stroke-width:2px,color:#fff
    style Execute fill:#4CAF50,stroke:#000,stroke-width:2px,color:#fff
```

**Key Features**:
- **Workflow Engine**: Converts AI recommendations to executable steps
- **Safety Validation**: RBAC checks, resource quotas, dry-run testing
- **K8s Execution**: Safe, auditable Kubernetes API operations
- **25+ Action Types**: Pods, deployments, scaling, rollbacks, nodes, storage, network, etc.

---

### Middle Layer - Section 4: Notification
**Stakeholder Communication**

**Notification Service**:
- Sends alerts to Slack, PagerDuty, email, webhooks
- Provides remediation summaries
- Links to audit trails and logs

---

### Bottom Layer: Data Storage
**Persistence, State, Learning**

```mermaid
graph LR
    Postgres[(PostgreSQL<br/>Action History<br/>Audit Logs)]
    Redis[(Redis<br/>Caching<br/>Distributed State)]
    Vector[(Vector DB<br/>Pattern Learning<br/>Context Embeddings)]
    
    All[All Services] --> Postgres
    All --> Redis
    Context[Context API +<br/>Effectiveness Monitor] --> Vector
    
    style Postgres fill:#336791,stroke:#000,stroke-width:2px,color:#fff
    style Redis fill:#DC382D,stroke:#000,stroke-width:2px,color:#fff
    style Vector fill:#4CAF50,stroke:#000,stroke-width:2px,color:#fff
```

---

## 🎯 Key Architectural Advantages

### 1. CRD-Native Communication
✅ **No external message bus** (Kafka, RabbitMQ)  
✅ **Kubernetes-native orchestration** (etcd state)  
✅ **Built-in audit trail** (CRD events)  
✅ **Standard Kubernetes tooling** (kubectl, kustomize)

### 2. Multi-Signal Ingestion
✅ **Vendor-neutral** (works with any monitoring tool)  
✅ **Multi-cloud** (AWS, GCP, Azure, on-prem)  
✅ **Extensible** (custom webhook support)

### 3. AI-Powered Intelligence
✅ **Multi-LLM support** (not locked to one provider)  
✅ **Context-aware** (topology, history, patterns)  
✅ **Continuous learning** (effectiveness feedback loop)

### 4. Safety-First Design
✅ **RBAC validation** (respects Kubernetes permissions)  
✅ **Dry-run testing** (preview before execution)  
✅ **Audit trail** (full remediation history)

---

## 🎯 Key Takeaway

> **"Kubernaut's architecture is Kubernetes-native by design. No external dependencies. No vendor lock-in. Just 12 microservices orchestrated via CRDs, powered by AI, and built for production safety."**

---

## ➡️ Transition to Next Slide

*"This architecture enables something powerful: transforming the engineer experience. Let's see what that looks like..."*

→ **Slide 8: The User Experience Transformation**

