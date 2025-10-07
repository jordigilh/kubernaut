# Context API Service - Documentation Hub

**Version**: 1.0
**Last Updated**: 2025-10-06
**Service Type**: Stateless HTTP API (Read-Only)
**Status**: ⏸️ Design Complete, Ready for Implementation

---

## 📋 Quick Navigation

1. **[overview.md](./overview.md)** - Service architecture, responsibilities, and design decisions
2. **[api-specification.md](./api-specification.md)** - 4 GET endpoints with schemas
3. **[database-schema.md](./database-schema.md)** - PostgreSQL + Vector DB schema

---

## 🎯 Purpose

**Provide historical intelligence for informed remediation decisions.**

**Read-only service** that answers:
- What's the environment context?
- Have we seen similar issues before?
- What's the success rate of remediation patterns?
- What's semantically similar to this alert?

---

## 🔌 Service Configuration

| Aspect | Value |
|--------|-------|
| **HTTP Port** | 8080 (REST API, `/health`, `/ready`) |
| **Metrics Port** | 9090 (Prometheus `/metrics` with auth) |
| **Namespace** | `prometheus-alerts-slm` |
| **ServiceAccount** | `context-api-sa` |

---

## 📊 API Endpoints

| Endpoint | Method | Purpose | Latency Target |
|----------|--------|---------|----------------|
| `/api/v1/context/environment` | GET | Environment classification | < 100ms |
| `/api/v1/context/patterns` | GET | Historical pattern matching | < 200ms |
| `/api/v1/context/success-rate` | GET | Success rate calculation | < 150ms |
| `/api/v1/context/semantic-search` | GET | Vector similarity search | < 250ms |

---

## 🗄️ Data Storage

**Reads from**:
- PostgreSQL (primary data)
- Vector DB (embeddings for semantic search)
- Redis (query result cache)

**No Writes**: This is a read-only service

---

## 🎯 Key Features

- ✅ Multi-tier caching (in-memory + Redis)
- ✅ Query result cache (5-30 min TTL)
- ✅ Read replicas for performance
- ✅ Monthly partitioned tables
- ✅ Optimized indexes for fast queries

---

## 🔗 Integration Points

**Clients** (Services that call Context API):
1. **Remediation Processor** - Environment classification, historical patterns
2. **AI Analysis** - Success rate data, semantic search
3. **Workflow Execution** - Pattern matching for workflow selection

---

## 📊 Performance

- **Latency**: < 200ms (p95)
- **Throughput**: 50 requests/second
- **Scaling**: 2-4 read replicas
- **Caching Hit Rate**: Target 70-80%

---

## 🚀 Getting Started

**Total Reading Time**: 30 minutes

1. **[overview.md](./overview.md)** (10 min) - Architecture overview
2. **[api-specification.md](./api-specification.md)** (15 min) - API contracts
3. **[database-schema.md](./database-schema.md)** (5 min) - Data model

---

## 📞 Quick Links

- **Parent**: [../README.md](../README.md) - All stateless services
- **Write Layer**: [../data-storage/](../data-storage/) - Complementary write service
- **Architecture**: [../../architecture/](../../architecture/)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete

