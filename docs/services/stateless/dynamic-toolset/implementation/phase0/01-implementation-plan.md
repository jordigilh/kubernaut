# Dynamic Toolset Service - Phase 0 Implementation Plan

**Version**: v1.0
**Phase**: 0 (Foundation)
**Duration**: Week 1 (5 days)
**Status**: ⏸️ Ready to Start

---

## Phase 0 Overview

**Goal**: Establish core service discovery and ConfigMap generation functionality
**Timeline**: 5 days (40 hours)
**Confidence**: 90% (Very High - based on Gateway service implementation experience)

---

## Day 1: Service Discovery Foundation (8 hours)

### Morning (4 hours)
**Task 1.1**: Create package structure
**Files**:
- `pkg/toolset/discovery/discoverer.go`
- `pkg/toolset/discovery/detector.go`
- `pkg/toolset/types.go`

**Deliverables**:
- `ServiceDiscoverer` interface
- `ServiceDetector` interface
- `DiscoveredService` type definition
- Basic package structure

**Test**: None (interfaces only)

### Afternoon (4 hours)
**Task 1.2**: Implement Prometheus detector
**Files**:
- `pkg/toolset/discovery/prometheus_detector.go`
- `test/unit/toolset/prometheus_detector_test.go`

**Deliverables**:
- `PrometheusDetector` implementation
- Detection logic (labels, service name, ports)
- Health check implementation
- Unit tests (70%+ coverage)

**Test**:
```bash
go test ./test/unit/toolset/prometheus_detector_test.go -v
```

---

## Day 2: Additional Detectors (8 hours)

### Morning (4 hours)
**Task 2.1**: Implement Grafana detector
**Files**:
- `pkg/toolset/discovery/grafana_detector.go`
- `test/unit/toolset/grafana_detector_test.go`

**Deliverables**:
- `GrafanaDetector` implementation
- Detection logic
- Health check implementation
- Unit tests

**Test**:
```bash
go test ./test/unit/toolset/grafana_detector_test.go -v
```

### Afternoon (4 hours)
**Task 2.2**: Implement Jaeger and Elasticsearch detectors
**Files**:
- `pkg/toolset/discovery/jaeger_detector.go`
- `pkg/toolset/discovery/elasticsearch_detector.go`
- Corresponding test files

**Deliverables**:
- `JaegerDetector` implementation
- `ElasticsearchDetector` implementation
- Unit tests for both

**Test**:
```bash
go test ./test/unit/toolset/ -v
```

---

## Day 3: Service Discoverer Implementation (8 hours)

### Morning (4 hours)
**Task 3.1**: Implement ServiceDiscoverer
**Files**:
- `pkg/toolset/discovery/discoverer.go` (implementation)
- `test/unit/toolset/discoverer_test.go`

**Deliverables**:
- `ServiceDiscovererImpl` struct
- `DiscoverServices()` method
- `RegisterDetector()` method
- Discovery loop with ticker
- Unit tests with mock K8s client

**Test**:
```bash
go test ./test/unit/toolset/discoverer_test.go -v
```

### Afternoon (4 hours)
**Task 3.2**: Integration test with Kind cluster
**Files**:
- `test/integration/toolset/kind_discovery_test.go`

**Deliverables**:
- Integration test suite setup
- Deploy mock Prometheus/Grafana services to Kind
- Test discovery against real K8s API
- Verify health checks work

**Test**:
```bash
make test-integration-toolset
```

---

## Day 4: ConfigMap Generation (8 hours)

### Morning (4 hours)
**Task 4.1**: Implement toolset generators
**Files**:
- `pkg/toolset/generator/generator.go`
- `pkg/toolset/generator/prometheus_toolset.go`
- `pkg/toolset/generator/grafana_toolset.go`
- `pkg/toolset/generator/kubernetes_toolset.go`

**Deliverables**:
- `ConfigMapGenerator` interface
- Generator implementations for Prometheus, Grafana, Kubernetes
- YAML generation logic
- Unit tests

**Test**:
```bash
go test ./test/unit/toolset/generator_test.go -v
```

### Afternoon (4 hours)
**Task 4.2**: Implement ConfigMap builder
**Files**:
- `pkg/toolset/generator/generator.go` (ToolsetConfigMapBuilder)
- `test/unit/toolset/configmap_builder_test.go`

**Deliverables**:
- `ToolsetConfigMapBuilder` struct
- `BuildConfigMap()` method
- Override merging logic
- Unit tests

**Test**:
```bash
go test ./test/unit/toolset/configmap_builder_test.go -v
```

---

## Day 5: HTTP Server & Integration (8 hours)

### Morning (4 hours)
**Task 5.1**: Implement HTTP server
**Files**:
- `pkg/toolset/server.go`
- `pkg/toolset/handlers.go`
- `test/unit/toolset/server_test.go`

**Deliverables**:
- `Server` struct
- HTTP route registration
- API handlers (/health, /ready, /api/v1/toolsets, /api/v1/services)
- Unit tests

**Test**:
```bash
go test ./test/unit/toolset/server_test.go -v
```

### Afternoon (4 hours)
**Task 5.2**: Main application entry point
**Files**:
- `cmd/dynamic-toolset/main.go`
- Integration test

**Deliverables**:
- Main application with dependency injection
- Component initialization
- Graceful shutdown
- End-to-end integration test

**Test**:
```bash
make run-dynamic-toolset  # Manual test
make test-integration-toolset  # Automated test
```

---

## Success Criteria

### Day 1
- [ ] Service discovery interfaces defined
- [ ] Prometheus detector implemented and tested
- [ ] Unit tests passing

### Day 2
- [ ] Grafana detector implemented and tested
- [ ] Jaeger detector implemented and tested
- [ ] Elasticsearch detector implemented and tested
- [ ] All unit tests passing

### Day 3
- [ ] ServiceDiscoverer fully implemented
- [ ] Discovery loop working with ticker
- [ ] Integration test with Kind cluster passing

### Day 4
- [ ] ConfigMap generators implemented
- [ ] ConfigMap builder working
- [ ] YAML generation correct
- [ ] All unit tests passing

### Day 5
- [ ] HTTP server running
- [ ] All API endpoints working
- [ ] Main application deployable
- [ ] End-to-end test passing

---

## Risk Assessment

### High Risk
- **Kubernetes API access**: Requires proper RBAC setup
  - **Mitigation**: Test with mock K8s client first, then real Kind cluster

- **Health check timeouts**: Network issues may cause false negatives
  - **Mitigation**: Configurable timeouts, retry logic

### Medium Risk
- **Service detection false positives**: Detecting non-Prometheus services as Prometheus
  - **Mitigation**: Multi-criteria detection (labels + ports + name)

- **ConfigMap size limits**: Many services may exceed ConfigMap size limit
  - **Mitigation**: Monitor ConfigMap size, consider splitting if needed

### Low Risk
- **YAML generation errors**: Incorrect YAML format
  - **Mitigation**: Comprehensive unit tests with example outputs

---

## Dependencies

### External Dependencies
- Kubernetes client-go library
- zap logger
- gorilla/mux HTTP router
- Ginkgo/Gomega test framework
- Kind cluster for integration tests

### Internal Dependencies
- None (first service to be implemented after Gateway)

---

## Rollout Plan

### Phase 0 Completion Checklist
- [ ] All unit tests passing (>70% coverage)
- [ ] All integration tests passing
- [ ] Main application runs successfully
- [ ] ConfigMap created and validated
- [ ] No linter errors
- [ ] Documentation updated

### Phase 0 Handoff
**To**: Phase 1 (Reconciliation)
**Deliverables**:
- Working service discovery
- ConfigMap generation
- HTTP server
- Test suite
- Implementation status document

---

**Document Status**: ✅ Complete Implementation Plan
**Last Updated**: October 10, 2025
**Confidence**: 90% (Very High)

