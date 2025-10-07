# Data Storage Service - Documentation Hub

**Version**: 1.0
**Last Updated**: 2025-10-06
**Service Type**: Stateless HTTP API (Write-Only)
**Status**: ⏸️ Design Complete, Ready for Implementation

---

## 📋 Quick Navigation

1. **[overview.md](./overview.md)** - Service architecture, responsibilities, and design decisions
2. **[api-specification.md](./api-specification.md)** - 4 POST endpoints with schemas

---

## 🎯 Purpose

**Persist complete audit trail for all remediation activities.**

**Write-only service** that stores:
- Remediation audit records
- AI analysis results
- Workflow execution history
- Kubernetes action logs

---

## 🔌 Service Configuration

| Aspect | Value |
|--------|-------|
| **HTTP Port** | 8080 (REST API, `/health`, `/ready`) |
| **Metrics Port** | 9090 (Prometheus `/metrics` with auth) |
| **Namespace** | `prometheus-alerts-slm` |
| **ServiceAccount** | `data-storage-sa` |

---

## 📊 API Endpoints

| Endpoint | Method | Purpose | Latency Target |
|----------|--------|---------|----------------|
| `/api/v1/store/remediation` | POST | Store remediation audit | < 250ms |
| `/api/v1/store/aianalysis` | POST | Store AI analysis result | < 250ms |
| `/api/v1/store/workflow` | POST | Store workflow execution | < 250ms |
| `/api/v1/store/execution` | POST | Store K8s action log | < 250ms |

---

## 🗄️ Data Storage

**Writes to**:
- PostgreSQL (primary data + audit trail)
- Vector DB (embeddings for semantic search)

**Dual-Write Pattern**: Both succeed or both fail (transaction consistency)

---

## 🎯 Key Features

- ✅ Dual-write (PostgreSQL + Vector DB)
- ✅ On-the-fly embedding generation (OpenAI ada-002)
- ✅ Transaction consistency
- ✅ Embedding cache (5 min TTL)
- ✅ Monthly partitioned tables

---

## 🔗 Integration Points

**Clients** (Services that call Data Storage):
1. **Remediation Processor** - Store enriched remediation data
2. **AI Analysis** - Store AI analysis results
3. **Workflow Execution** - Store workflow execution history
4. **Kubernetes Executor** - Store action execution logs

---

## 📊 Performance

- **Latency**: < 250ms (p95)
- **Throughput**: 50 requests/second
- **Scaling**: 2-3 write replicas
- **Embedding Cache Hit Rate**: Target 60-70%

---

## 🚀 Getting Started

**Total Reading Time**: 20 minutes

1. **[overview.md](./overview.md)** (10 min) - Architecture overview
2. **[api-specification.md](./api-specification.md)** (10 min) - API contracts

---

## 📞 Quick Links

- **Parent**: [../README.md](../README.md) - All stateless services
- **Read Layer**: [../context-api/](../context-api/) - Complementary read service
- **Architecture**: [../../architecture/](../../architecture/)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete

