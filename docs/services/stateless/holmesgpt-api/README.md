# HolmesGPT API Service - Documentation Hub

**Version**: 1.0
**Last Updated**: 2025-10-06
**Service Type**: Stateless HTTP API (Python)
**Status**: ⏸️ Design Complete, Ready for Implementation

---

## 📋 Quick Navigation

1. **[overview.md](./overview.md)** - Service architecture, HolmesGPT integration, design decisions
2. **[api-specification.md](./api-specification.md)** - Investigation API with Python implementation
3. **[ORIGINAL_MONOLITHIC.md](./ORIGINAL_MONOLITHIC.md)** - Complete original 2,100-line specification (reference)

---

## 🎯 Purpose

**Provide AI-powered root cause analysis using HolmesGPT Python SDK.**

**REST wrapper** that provides:
- AI investigation for Kubernetes issues
- Multi-provider LLM support (OpenAI, Claude, local)
- Dynamic toolset configuration
- Read-only Kubernetes access

---

## 🔌 Service Configuration

| Aspect | Value |
|--------|-------|
| **HTTP Port** | 8080 (REST API, `/health`, `/ready`) |
| **Metrics Port** | 9090 (Prometheus `/metrics` with auth) |
| **Language** | Python (FastAPI + HolmesGPT SDK) |
| **Namespace** | `prometheus-alerts-slm` |
| **ServiceAccount** | `holmesgpt-api-sa` |

---

## 📊 API Endpoints

| Endpoint | Method | Purpose | Latency Target |
|----------|--------|---------|----------------|
| `/api/v1/investigate` | POST | AI-powered root cause analysis | < 5s |
| `/api/v1/toolsets` | GET | List available toolsets | < 100ms |

---

## 🤖 AI Capabilities

**Toolsets**:
- **Kubernetes**: Pod logs, events, describe resources
- **Prometheus**: Metrics queries, alert history
- **Grafana**: Dashboard queries, panel data

**LLM Providers**:
- OpenAI (GPT-4, GPT-3.5)
- Anthropic (Claude)
- Local LLMs (via Ollama)

---

## 🎯 Key Features

- ✅ HolmesGPT Python SDK integration
- ✅ Multi-provider LLM support
- ✅ Dynamic toolset configuration (ConfigMaps)
- ✅ Read-only RBAC (no cluster modifications)
- ✅ Structured AI response format

---

## 🔗 Integration Points

**Clients**:
1. **AI Analysis Controller** - Request AI investigation for remediation requests

**Dependencies**:
- **Dynamic Toolset Service** - Provides toolset configurations
- **Kubernetes API** - Read-only access for investigations

---

## 📊 Performance

- **Latency**: < 5s (p95) - AI calls are inherently slow
- **Throughput**: 10 requests/second
- **Scaling**: 2-3 replicas
- **Timeout**: 30s for AI investigation

---

## 🚀 Getting Started

**Total Reading Time**: 30 minutes

1. **[overview.md](./overview.md)** (15 min) - Architecture and HolmesGPT integration
2. **[api-specification.md](./api-specification.md)** (15 min) - API contracts

**For Full Details**: See [ORIGINAL_MONOLITHIC.md](./ORIGINAL_MONOLITHIC.md) (2,100 lines)

---

## 📞 Quick Links

- **Parent**: [../README.md](../README.md) - All stateless services
- **Toolset Config**: [../dynamic-toolset/](../dynamic-toolset/) - Toolset management
- **Architecture**: [../../architecture/](../../architecture/)
- **HolmesGPT SDK**: External Python library

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete

