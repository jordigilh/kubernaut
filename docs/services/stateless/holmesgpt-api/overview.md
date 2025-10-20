# HolmesGPT API Service - Overview

**Version**: v1.1
**Last Updated**: October 16, 2025
**Status**: ✅ Complete (Restructured from monolithic + Self-Documenting JSON)
**Service Type**: Stateless HTTP Service (Python REST API)
**Port**: 8080 (REST API + Health), 9090 (Metrics)
**Prompt Format**: Self-Documenting JSON (DD-HOLMESGPT-009)

**IMPORTANT UPDATE (October 16, 2025)**: HolmesGPT API now accepts **Self-Documenting JSON format** for all investigation requests:
- ✅ **60% token reduction** (~730 → ~180 tokens per investigation)
- ✅ **$1,980/year cost savings** on LLM API costs
- ✅ **150ms latency improvement** per investigation
- ✅ **98% parsing accuracy maintained**

**Decision Document**: `docs/architecture/decisions/DD-HOLMESGPT-009-Ultra-Compact-JSON-Format.md`

---

## Table of Contents

1. [Purpose & Scope](#purpose--scope)
2. [Architecture Overview](#architecture-overview)
3. [Toolset Architecture](#toolset-architecture)
4. [RBAC Requirements](#rbac-requirements)
5. [Integration Points](#integration-points)
6. [V1 Scope Boundaries](#v1-scope-boundaries)

---

## Purpose & Scope

### Core Purpose

HolmesGPT API Service is the **AI-powered investigation engine** for Kubernaut. It provides:

1. **REST API wrapper** for HolmesGPT Python SDK
2. **Dynamic toolset configuration** via ConfigMap polling
3. **Multi-provider LLM integration** (OpenAI, Claude, local models)
4. **Kubernetes investigation** using read-only cluster access
5. **Prometheus metrics analysis** for root cause identification

### Why HolmesGPT API Exists

**Problem**: Without HolmesGPT API, AI Analysis service would need to:
- **Directly integrate HolmesGPT SDK** → Python dependency in Go codebase
- **Manage LLM providers** → Duplicate configuration
- **Handle toolset discovery** → Complex cluster access patterns

**Solution**: HolmesGPT API provides **centralized AI investigation** that:
- ✅ Isolates Python dependencies in dedicated service
- ✅ Single LLM provider configuration for all investigations
- ✅ Reusable toolset architecture for future data sources
- ✅ Clear RBAC boundaries for cluster access

---

## Architecture Overview

### Service Characteristics

- **Type**: Stateless HTTP API (Python-based)
- **Deployment**: Kubernetes Deployment with 2-3 replicas
- **State Management**: No internal state, ConfigMap-based configuration
- **Integration Pattern**: REST API → HolmesGPT SDK → LLM providers + Data sources

### Component Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                   HolmesGPT API Service                         │
│                                                                 │
│  ┌──────────────┐       ┌──────────────┐                      │
│  │ Investigation│       │   LLM        │                      │
│  │   REST API   │       │  Integration │                      │
│  └──────┬───────┘       └──────┬───────┘                      │
│         │                      │                               │
│         └──────────┬───────────┘                               │
│                    │                                           │
│         ┌──────────▼──────────┐                                │
│         │   HolmesGPT SDK     │                                │
│         │   (Python Library)  │                                │
│         └──────────┬──────────┘                                │
│                    │                                           │
│         ┌──────────▼──────────┐                                │
│         │  Toolset Manager    │◄─────────────┐                │
│         │  (Dynamic Config)   │              │                │
│         └──────────┬──────────┘         ┌────┴────┐           │
│                    │                    │ConfigMap│           │
│         ┌──────────▼──────────┐         │ Volume  │           │
│         │  Toolset Executors  │         └─────────┘           │
│         │  - Kubernetes       │                                │
│         │  - Prometheus       │                                │
│         │  - Grafana          │                                │
│         └──────────┬──────────┘                                │
│                    │                                           │
│                    ▼                                           │
│          Investigation Report                                  │
└─────────────────────────────────────────────────────────────────┘
         │                    │                    │
         │ Kubernetes API     │ Prometheus         │ LLM Provider
         ▼                    ▼                    ▼
    ┌──────────┐         ┌──────────┐        ┌──────────┐
    │K8s Logs  │         │Metrics   │        │OpenAI    │
    │Events    │         │Queries   │        │Claude    │
    │Resources │         │Data      │        │Local LLM │
    └──────────┘         └──────────┘        └──────────┘
```

---

## Toolset Architecture

### Built-in Toolsets (V1)

#### 1. Kubernetes Toolset (PRIMARY)
**Purpose**: Fetch cluster data for investigation

**Capabilities**:
- Pod logs retrieval
- Event analysis
- Resource describe operations
- Multi-namespace support

**Requirements**:
- ✅ Read-only Kubernetes RBAC
- ✅ Cluster-wide access
- ✅ ServiceAccount: `holmesgpt-api`

---

#### 2. Prometheus Toolset (PRIMARY)
**Purpose**: Query metrics for root cause analysis

**Capabilities**:
- PromQL query execution
- Time-series data retrieval
- Metric correlation

**Requirements**:
- ✅ Prometheus service endpoint
- ❌ No Kubernetes RBAC needed (HTTP client)

---

#### 3. Grafana Toolset (OPTIONAL)
**Purpose**: Query Grafana dashboards and data sources

**Capabilities**:
- Dashboard data retrieval
- Alert history

**Requirements**:
- ✅ Grafana API endpoint
- ✅ Grafana API token
- ❌ No Kubernetes RBAC needed (HTTP client)

---

### Dynamic Toolset Configuration

**ConfigMap-Based**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: holmesgpt-api-toolsets
  namespace: kubernaut-system
data:
  kubernetes-toolset.yaml: |
    toolset: kubernetes
    enabled: true
    config:
      incluster: true
      namespaces: ["*"]

  prometheus-toolset.yaml: |
    toolset: prometheus
    enabled: true
    config:
      url: "http://prometheus:9090"
```

**Polling Strategy**:
- **Interval**: 60 seconds
- **Reload**: Graceful (no downtime)
- **Validation**: Schema check before applying

---

## RBAC Requirements

### ServiceAccount Configuration

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: holmesgpt-api-kubernetes-toolset
rules:
# Pod logs and events (kubernetes toolset - PRIMARY)
- apiGroups: [""]
  resources: ["pods", "pods/log", "events"]
  verbs: ["get", "list"]

# Resource describe operations (kubernetes toolset)
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets", "replicasets", "daemonsets"]
  verbs: ["get", "list"]

- apiGroups: [""]
  resources: ["nodes", "services", "persistentvolumeclaims"]
  verbs: ["get", "list"]

# ConfigMaps and Secrets metadata (describe only, NOT data)
- apiGroups: [""]
  resources: ["configmaps", "secrets"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: holmesgpt-api-kubernetes-toolset
subjects:
- kind: ServiceAccount
  name: holmesgpt-api
  namespace: kubernaut-system
roleRef:
  kind: ClusterRole
  name: holmesgpt-api-kubernetes-toolset
  apiGroup: rbac.authorization.k8s.io
```

### Security Characteristics

- ✅ **Read-only access**: No create/update/delete permissions
- ✅ **Cluster-wide scope**: Can investigate any namespace
- ✅ **No secret data access**: Can list secrets, but not read values
- ✅ **No ConfigMap API access**: Configuration via volume mount

---

## Integration Points

### Client: AI Analysis Controller

```go
// AI Analysis Controller calls HolmesGPT API
investigationReq := HolmesGPTInvestigationRequest{
    Context: map[string]interface{}{
        "namespace":   "production",
        "podName":     "api-server-abc123",
        "alertName":   "HighMemoryUsage",
        "timeRange":   "15m",
    },
    LLMProvider: "openai",
    LLMModel:    "gpt-4",
    Toolsets:    []string{"kubernetes", "prometheus"},
}

resp, err := holmesGPTClient.Investigate(ctx, investigationReq)
```

---

### External Dependencies

| Dependency | Purpose | Required |
|------------|---------|----------|
| **Kubernetes API** | Cluster data (logs, events, resources) | Yes (kubernetes toolset) |
| **Prometheus** | Metrics queries | Yes (prometheus toolset) |
| **OpenAI API** | LLM inference | Optional (one LLM required) |
| **Anthropic Claude** | LLM inference | Optional (one LLM required) |
| **Local LLM** | LLM inference | Optional (one LLM required) |
| **Grafana** | Dashboard data | Optional |

---

## V1 Scope Boundaries

### ✅ In Scope for V1

1. **Core Toolsets**
   - Kubernetes toolset (logs, events, resources)
   - Prometheus toolset (metrics queries)
   - Grafana toolset (optional)

2. **LLM Providers**
   - OpenAI (gpt-4, gpt-3.5-turbo)
   - Anthropic Claude
   - Local LLM models

3. **Investigation API**
   - POST /api/v1/investigate
   - Synchronous investigation
   - JSON response format

4. **Configuration**
   - ConfigMap-based toolset configuration
   - 60-second polling interval
   - Graceful reload

5. **RBAC**
   - Read-only Kubernetes access
   - Cluster-wide scope
   - No secret data access

---

### ❌ Out of Scope for V1

1. **Advanced Toolsets**
   - AlertManager integration
   - AWS CloudWatch
   - Azure Monitor
   - Datadog
   - Custom toolsets

2. **Advanced Features**
   - Investigation caching
   - Multi-cluster coordination
   - Custom prompt engineering
   - Investigation history

3. **Async Operations**
   - Webhook notifications
   - Long-running investigations
   - Background processing

---

## Service Configuration

### Port Configuration
- **Port 8080**: REST API and health probes
- **Port 9090**: Metrics endpoint
- **Authentication**: Kubernetes TokenReviewer API

### Environment Variables

```bash
# LLM Configuration
LLM_PROVIDER=openai              # openai, anthropic, local
LLM_MODEL=gpt-4                  # Model name
LLM_API_KEY=sk-...               # API key (if required)

# Toolset Configuration
CONFIG_MAP_PATH=/etc/holmesgpt-api/toolsets/
CONFIG_POLL_INTERVAL=60          # Seconds

# Kubernetes Configuration
IN_CLUSTER=true                  # Use in-cluster config
KUBECONFIG=/path/to/kubeconfig   # If IN_CLUSTER=false

# Service Configuration
SERVICE_PORT=8080
METRICS_PORT=9090
LOG_LEVEL=info
```

---

## Performance Characteristics

### Target SLOs

| Metric | Target | Notes |
|--------|--------|-------|
| **Availability** | 99.5% | Per replica |
| **Investigation Latency (p95)** | < 30s | LLM-dependent |
| **Investigation Latency (p99)** | < 60s | Complex investigations |
| **Throughput** | 10 investigations/min | Per replica |
| **Error Rate** | < 2% | LLM + toolset errors |

---

## Related Documentation

### Core Specifications
- [API Specification](./api-specification.md) - REST API endpoints
- [LLM Providers](./llm-providers.md) - Provider integration details
- [ORIGINAL_MONOLITHIC.md](./ORIGINAL_MONOLITHIC.md) - Complete original specification

### Architecture References
- [Service Dependency Map](../../../../architecture/SERVICE_DEPENDENCY_MAP.md)
- [Dynamic Toolset Configuration Architecture](../../../../architecture/DYNAMIC_TOOLSET_CONFIGURATION_ARCHITECTURE.md)

---

## Business Requirements Mapping

| Business Requirement | Implementation | Validation |
|---------------------|----------------|------------|
| **BR-HOLMES-001**: HolmesGPT investigation API endpoint | `POST /api/v1/investigate` | Integration test: end-to-end investigation flow |
| **BR-HOLMES-002**: LLM provider integration | `llm.Client` interface with OpenAI/Claude/local providers | Unit test: provider failover & fallback |
| **BR-HOLMES-003**: Prompt generation for investigations | `GenerateInvestigationPrompt()` | Unit test: context-aware prompt construction |
| **BR-HOLMES-004**: Toolset discovery from ConfigMap | `ToolsetManager.LoadFromConfigMap()` | Integration test: ConfigMap polling & reload |
| **BR-HOLMES-005**: Kubernetes read access (RBAC) | ServiceAccount + ClusterRole for logs/events | E2E test: toolset permissions validation |
| **BR-HOLMES-010**: Kubernetes cluster investigation | `kubernetes` toolset with `kubectl` commands | Integration test: log/event fetching |
| **BR-HOLMES-011**: Context API integration | `GET /api/v1/context/similar-remediations` | Integration test: similar context retrieval |
| **BR-HOLMES-012**: Custom toolset configurations | ConfigMap-based toolset definitions | Unit test: toolset parsing & validation |
| **BR-HOLMES-020**: Prometheus metrics analysis | `prometheus` toolset with PromQL queries | Integration test: metric querying |
| **BR-HOLMES-030**: Multi-provider LLM support | `llm.ClientFactory` with provider selection | Unit test: provider-specific configuration |
| **BR-HOLMES-040**: Dynamic toolset configuration | ConfigMap hot-reload (60s interval) | Integration test: ConfigMap update detection |
| **BR-HOLMES-171**: Fail-fast startup validation | `ValidateRequiredToolsets()` on startup | Unit test: missing toolset error handling |
| **BR-HOLMES-172**: Startup validation error messages | Structured error messages for missing dependencies | Unit test: error message clarity |
| **BR-HOLMES-174**: Runtime toolset failure tracking | Metrics: `toolset_failure_total` by toolset name | Integration test: failure metric increment |
| **BR-HOLMES-175**: Auto-reload ConfigMap on failures | ConfigMap re-poll on persistent toolset errors | Integration test: ConfigMap reload on failure |
| **BR-HOLMES-176**: Graceful toolset reload | Zero-downtime ConfigMap updates | Integration test: investigation during reload |

**Core Capabilities**:
- **Investigation API**: BR-HOLMES-001 to BR-HOLMES-050 (HolmesGPT SDK integration)
- **Toolset Management**: BR-HOLMES-051 to BR-HOLMES-100 (ConfigMap-based discovery & reload)
- **LLM Provider Integration**: BR-HOLMES-101 to BR-HOLMES-150 (Multi-provider support with fallback)
- **Error Handling**: BR-HOLMES-151 to BR-HOLMES-170 (Resilience patterns)
- **Service Reliability**: BR-HOLMES-171 to BR-HOLMES-180 (Fail-fast validation & graceful degradation)

**For Complete BR Specification**: See [ORIGINAL_MONOLITHIC.md](./ORIGINAL_MONOLITHIC.md) for all BR-HOLMES-001 to BR-HOLMES-180 details.

---

**Document Status**: ✅ Complete (Restructured)
**Original**: See [ORIGINAL_MONOLITHIC.md](./ORIGINAL_MONOLITHIC.md) for complete 2,100+ line specification
**Last Updated**: October 6, 2025
**Version**: 1.0
