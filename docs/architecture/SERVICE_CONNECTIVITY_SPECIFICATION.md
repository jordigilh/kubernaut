# Kubernaut - Service Connectivity Specification

**Document Version**: 1.0
**Date**: September 27, 2025
**Status**: **APPROVED** - Official Service Integration Specification
**Architecture**: Microservices with Justified Connectivity

---

> **⚠️ HISTORICAL (2026-08-02, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))**: This document
> predates Kubernaut's CRD-controller architecture and describes an earlier, sequential-HTTP-call design
> (`Remediation Processor`, `Workflow Orchestrator`, `K8s Executor`, `Intelligence Service`, etc.) that was never
> built as specified here. The current production architecture is 12 CRD-controller/stateless services — see
> [Kubernaut Service Catalog](KUBERNAUT_SERVICE_CATALOG.md) for the authoritative decomposition. Two corrections
> specific to this issue: (1) **HolmesGPT** (the "External AI & Investigation Tools" integration below) was
> rewritten in Go and renamed **Kubernaut Agent (KA)** in v1.3 (issue [#433](https://github.com/jordigilh/kubernaut/issues/433));
> it is now a first-class Kubernaut service (`cmd/kubernautagent/`), not an external tool consumed via a
> "Context API Service". (2) **Context API** (referenced throughout this document as the HolmesGPT-facing
> service) was deprecated and folded into Data Storage per
> [DD-CONTEXT-006](decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md); it no longer exists as a running service.
> The rest of this document's service list/ports/flows is preserved as-is for historical reference and has not
> been otherwise re-verified against current source.

---

## 🎯 **PURPOSE**

This document defines the **approved service connectivity patterns** for Kubernaut's microservices architecture, providing detailed justification for each service connection and integration pattern based on business requirements.

---

## 🔗 **SERVICE CONNECTIVITY MATRIX**

**Service Specifications**: For detailed service descriptions and responsibilities, see [Service Catalog](KUBERNAUT_SERVICE_CATALOG.md). This document focuses on connectivity patterns, protocols, and business requirement justifications for service interactions.

### **Internal Service Connections**

| **From Service** | **To Service** | **Protocol** | **Endpoint** | **Purpose** | **Business Requirement** |
|------------------|----------------|--------------|--------------|-------------|-------------------------|
| **🔗 Gateway** | **🧠 Remediation Processor** | HTTP/REST | `POST /process-alert` | Route validated alerts for processing | **BR-WH-001** (receive alerts) → **BR-SP-001** (process alerts) |
| **🧠 Remediation Processor** | **🤖 AI Analysis** | HTTP/REST | `POST /analyze-alert` | Get AI-powered remediation recommendations | **BR-SP-016** (AI integration) → **BR-AI-001** (AI analysis) |
| **🤖 AI Analysis** | **🎯 Workflow Orchestrator** | HTTP/REST | `POST /create-workflow` | Convert AI recommendations into executable workflows | **BR-AI-007** (workflow generation) → **BR-WF-001** (workflow execution) |
| **🎯 Workflow Orchestrator** | **⚡ K8s Executor** | HTTP/REST | `POST /execute-action` | Execute individual workflow steps as K8s actions | **BR-WF-010** (action execution) → **BR-EX-001** (K8s operations) |
| **⚡ K8s Executor** | **📊 Data Storage** | HTTP/REST | `POST /store-action` | Store action execution results and history | **BR-EX-020** (result tracking) → **BR-STOR-001** (data persistence) |
| **📊 Data Storage** | **🔍 Intelligence** | HTTP/REST | `GET /get-patterns` | Provide historical data for pattern discovery | **BR-STOR-015** (data retrieval) → **BR-INT-001** (pattern analysis) |
| **🔍 Intelligence** | **📈 Effectiveness Monitor** | HTTP/REST | `POST /assess-effectiveness` | Supply pattern insights for effectiveness assessment | **BR-INT-020** (insights delivery) → **BR-INS-001** (effectiveness assessment) |
| **📈 Effectiveness Monitor** | **🌐 Context API** | HTTP/REST | `GET /get-context` | Provide assessment context for external AI services | **BR-INS-010** (context provision) → **BR-CTX-001** (context orchestration) |
| **🌐 Context API** | **📢 Notifications** | HTTP/REST | `POST /send-notification` | Trigger notifications based on context changes | **BR-CTX-020** (notification triggers) → **BR-NOTIF-001** (notification delivery) |

---

## 🌐 **EXTERNAL SYSTEM INTEGRATIONS**

### **Monitoring & Alerting Systems**

#### **Prometheus AlertManager Integration**
- **Connected Service**: 🔗 Gateway Service
- **Protocol**: HTTP/HTTPS Webhook
- **Endpoint**: `POST /webhook/prometheus`
- **Business Requirement**: **BR-INT-001** - Integrate with Prometheus webhook format
- **Purpose**: Receive production alerts from monitoring infrastructure
- **Data Format**: Prometheus AlertManager webhook JSON payload
- **Security**: Webhook signature verification, TLS encryption

#### **Grafana Integration**
- **Connected Service**: 🔗 Gateway Service
- **Protocol**: HTTP/HTTPS Webhook
- **Endpoint**: `POST /webhook/grafana`
- **Business Requirement**: **BR-INT-002** - Support Grafana alert webhook integration
- **Purpose**: Receive dashboard-driven alerts and annotations
- **Data Format**: Grafana webhook JSON payload
- **Security**: API key authentication, TLS encryption

---

### **AI & Machine Learning Providers**

#### **OpenAI Integration**
- **Connected Service**: 🤖 AI Analysis Service
- **Protocol**: HTTPS REST API
- **Endpoint**: `https://api.openai.com/v1/chat/completions`
- **Business Requirement**: **BR-AI-003** - Multi-provider LLM integration
- **Purpose**: Enterprise-grade AI analysis with GPT-4/GPT-3.5
- **Authentication**: API key with rotation support
- **Rate Limiting**: Respect OpenAI rate limits and quotas

#### **Anthropic Claude Integration**
- **Connected Service**: 🤖 AI Analysis Service
- **Protocol**: HTTPS REST API
- **Endpoint**: `https://api.anthropic.com/v1/messages`
- **Business Requirement**: **BR-AI-004** - Alternative LLM provider support
- **Purpose**: Alternative AI analysis with Claude models
- **Authentication**: API key with secure storage
- **Fallback**: Automatic fallback if OpenAI unavailable

#### **Azure OpenAI Integration**
- **Connected Service**: 🤖 AI Analysis Service
- **Protocol**: HTTPS REST API
- **Endpoint**: `https://{resource}.openai.azure.com/openai/deployments/{model}/chat/completions`
- **Business Requirement**: **BR-AI-005** - Enterprise Azure integration
- **Purpose**: Enterprise Azure-hosted AI analysis
- **Authentication**: Azure AD authentication with managed identity
- **Compliance**: Enterprise security and compliance requirements

---

### **Kubernetes Infrastructure**

#### **Multi-Cluster Kubernetes Integration**
- **Connected Service**: ⚡ K8s Executor Service
- **Protocol**: Kubernetes API (HTTPS)
- **Endpoint**: Multiple cluster endpoints via kubeconfig
- **Business Requirement**: **BR-EX-002** - Multi-cluster operations support
- **Purpose**: Execute remediation actions across multiple clusters
- **Authentication**: Service account tokens, RBAC
- **Security**: TLS client certificates, network policies

---

### **Vector Database Providers**

#### **PostgreSQL with PGVector**
- **Connected Service**: 📊 Data Storage Service
- **Protocol**: PostgreSQL Wire Protocol
- **Endpoint**: `postgresql://host:5432/database`
- **Business Requirement**: **BR-VDB-001** - Primary vector database integration
- **Purpose**: High-performance vector similarity search and storage
- **Authentication**: Database credentials with connection pooling
- **Performance**: Optimized for high-throughput vector operations

#### **Pinecone Vector Database**
- **Connected Service**: 📊 Data Storage Service
- **Protocol**: HTTPS REST API
- **Endpoint**: `https://{index}.svc.{environment}.pinecone.io`
- **Business Requirement**: **BR-VDB-003** - Managed vector database option
- **Purpose**: Scalable managed vector database for production workloads
- **Authentication**: API key authentication
- **Features**: Auto-scaling, managed infrastructure

#### **Weaviate Knowledge Graph**
- **Connected Service**: 📊 Data Storage Service
- **Protocol**: HTTPS REST API + GraphQL
- **Endpoint**: `https://{cluster}.weaviate.network`
- **Business Requirement**: **BR-VDB-004** - Knowledge graph capabilities
- **Purpose**: Semantic search with knowledge graph relationships
- **Authentication**: API key or OIDC authentication
- **Features**: Semantic search, knowledge graph traversal

---

### **External AI & Investigation Tools**

#### **HolmesGPT Integration** — HISTORICAL, see banner above
> As of v1.3, HolmesGPT was rewritten in Go and renamed **Kubernaut Agent (KA)**; it runs as a first-class
> Kubernaut service (real ports: API `8443`, health `8081`, metrics `9090` — see
> `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`), not as an externally-integrated tool behind
> a "Context API Service" (deprecated, DD-CONTEXT-006). The entry below is preserved unmodified as a historical
> record of the original (never-built-as-specified) design.
- **Connected Service**: 🌐 Context API Service
- **Protocol**: HTTPS REST API
- **Endpoint**: `http://holmesgpt-service:8090/api/v1/`
- **Business Requirement**: **BR-EXTERNAL-001** - HolmesGPT custom toolset integration
- **Purpose**: Provide dynamic context for AI-powered investigations
- **Data Exchange**: Context orchestration and toolset configuration
- **Performance**: 100ms cached, 500ms fresh context retrieval

---

### **Notification & Communication Platforms**

#### **Slack Integration**
- **Connected Service**: 📢 Notification Service
- **Protocol**: HTTPS REST API + Webhooks
- **Endpoint**: `https://hooks.slack.com/services/{webhook}`
- **Business Requirement**: **BR-NOTIF-005** - Multi-channel notification delivery
- **Purpose**: Real-time team notifications and incident updates
- **Authentication**: Webhook URLs, OAuth tokens
- **Features**: Rich message formatting, interactive buttons

#### **Microsoft Teams Integration**
- **Connected Service**: 📢 Notification Service
- **Protocol**: HTTPS REST API + Webhooks
- **Endpoint**: `https://outlook.office.com/webhook/{webhook}`
- **Business Requirement**: **BR-NOTIF-006** - Enterprise communication platform
- **Purpose**: Enterprise team notifications and collaboration
- **Authentication**: Webhook URLs, Azure AD integration
- **Features**: Adaptive cards, threaded conversations

#### **PagerDuty Integration**
- **Connected Service**: 📢 Notification Service
- **Protocol**: HTTPS REST API
- **Endpoint**: `https://api.pagerduty.com/incidents`
- **Business Requirement**: **BR-INT-003** - Incident management integration
- **Purpose**: Critical incident escalation and on-call management
- **Authentication**: API key authentication
- **Features**: Incident creation, escalation policies, acknowledgments

---

## 🔄 **DATA FLOW PATTERNS**

### **Primary Alert Processing Flow**
```
Prometheus Alert → Gateway → Remediation Processor → AI Analysis → Workflow Orchestrator → K8s Executor → Data Storage
```

### **Learning & Improvement Flow**
```
Data Storage → Intelligence → Effectiveness Monitor → Context API → (Feedback to AI Analysis)
```

### **Notification Flow**
```
Context API → Notifications → (Slack/Teams/Email/PagerDuty)
```

### **External AI Integration Flow**
```
HolmesGPT ← Context API ← Effectiveness Monitor ← Intelligence ← Data Storage
```

---

## 🛠️ **IMPLEMENTATION STANDARDS**

### **API Standards**
- **Protocol**: HTTP/REST with JSON payloads
- **Authentication**: JWT tokens for internal, API keys for external
- **Versioning**: Semantic versioning with backward compatibility
- **Documentation**: OpenAPI/Swagger specifications
- **Error Handling**: Standardized error response formats

### **Communication Patterns**
- **Synchronous**: HTTP/REST for request-response patterns
- **Asynchronous**: Message queues for event-driven patterns
- **Streaming**: WebSockets for real-time updates
- **Batch**: Bulk APIs for high-throughput operations

### **Resilience Patterns**
- **Circuit Breaker**: Prevent cascade failures
- **Retry Logic**: Exponential backoff with jitter
- **Timeout Management**: Configurable timeouts per service
- **Graceful Degradation**: Fallback mechanisms for service unavailability

---

## 📋 **SERVICE DEPENDENCIES**

### **Dependency Graph**
```
Gateway Service (Entry Point)
    ↓
Remediation Processor Service
    ↓
AI Analysis Service ←→ (External LLM Providers)
    ↓
Workflow Orchestrator Service
    ↓
K8s Executor Service ←→ (Kubernetes Clusters)
    ↓
Data Storage Service ←→ (Vector Databases)
    ↓
Intelligence Service
    ↓
Effectiveness Monitor Service
    ↓
Context API Service ←→ (HolmesGPT)
    ↓
Notification Service ←→ (Slack/Teams/PagerDuty)
```

### **Critical Path Services**
- **Gateway Service**: Single point of entry - critical for availability
- **AI Analysis Service**: Core intelligence - critical for decision quality
- **K8s Executor Service**: Action execution - critical for remediation
- **Data Storage Service**: Persistence layer - critical for learning

### **Optional Services**
- **Intelligence Service**: Pattern discovery - enhances effectiveness
- **Effectiveness Monitor Service**: Assessment - improves over time
- **Context API Service**: External integration - enhances investigation
- **Notification Service**: Communication - improves visibility

---

**Document Status**: ✅ **APPROVED**
**Connectivity Confidence**: **100%**
**Integration Ready**: ✅ **YES**

This specification ensures all service connections are business-justified, properly secured, and aligned with the Single Responsibility Principle.
