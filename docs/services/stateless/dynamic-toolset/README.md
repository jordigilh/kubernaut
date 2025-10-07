# Dynamic Toolset Service - Documentation Hub

**Version**: 1.0
**Last Updated**: 2025-10-06
**Service Type**: Stateless HTTP API
**Status**: ⏸️ Design Complete, Ready for Implementation

---

## 📋 Quick Navigation

1. **[overview.md](./overview.md)** - Service architecture, automatic discovery, design decisions
2. **[api-specification.md](./api-specification.md)** - ConfigMap management API

---

## 🎯 Purpose

**Automatically discover and configure HolmesGPT toolsets.**

**Dynamic configuration** that provides:
- Automatic Kubernetes resource discovery
- HolmesGPT toolset generation
- ConfigMap-based hot-reload
- Toolset validation and compatibility checks

---

## 🔌 Service Configuration

| Aspect | Value |
|--------|-------|
| **HTTP Port** | 8080 (REST API, `/health`, `/ready`) |
| **Metrics Port** | 9090 (Prometheus `/metrics` with auth) |
| **Namespace** | `prometheus-alerts-slm` |
| **ServiceAccount** | `dynamic-toolset-sa` |

---

## 📊 API Endpoints

| Endpoint | Method | Purpose | Latency Target |
|----------|--------|---------|----------------|
| `/api/v1/toolsets/discover` | POST | Discover available K8s resources | < 300ms |
| `/api/v1/toolsets/generate` | POST | Generate toolset configuration | < 200ms |
| `/api/v1/toolsets/validate` | POST | Validate toolset compatibility | < 100ms |

---

## 🔍 Discovery Capabilities

**Automatically Discovers**:
- Available namespaces
- Deployments, StatefulSets, DaemonSets
- Services and Ingresses
- ConfigMaps and Secrets (metadata only)
- Prometheus instances
- Grafana instances

---

## 🎯 Key Features

- ✅ Automatic resource discovery
- ✅ ConfigMap generation for HolmesGPT
- ✅ Hot-reload support (HolmesGPT watches ConfigMaps)
- ✅ Toolset validation
- ✅ RBAC-aware discovery (only shows accessible resources)

---

## 🔗 Integration Points

**Clients**:
1. **HolmesGPT API** - Reads generated toolset ConfigMaps

**Generates**:
- ConfigMaps in `prometheus-alerts-slm` namespace
- Format: HolmesGPT toolset configuration

---

## 📊 Performance

- **Latency**: < 300ms (p95)
- **Throughput**: 5 requests/second
- **Scaling**: 1-2 replicas
- **Discovery Interval**: Every 5 minutes (configurable)

---

## 🚀 Getting Started

**Total Reading Time**: 20 minutes

1. **[overview.md](./overview.md)** (10 min) - Architecture and discovery
2. **[api-specification.md](./api-specification.md)** (10 min) - API contracts

---

## 📞 Quick Links

- **Parent**: [../README.md](../README.md) - All stateless services
- **Consumer**: [../holmesgpt-api/](../holmesgpt-api/) - Uses generated toolsets
- **Architecture**: [../../architecture/](../../architecture/)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete

