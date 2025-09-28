# ADR-003: Kind Cluster as Primary Integration Environment

## Status
**ACCEPTED** - December 2024

## Context

The kubernaut project requires a robust integration testing environment that closely mirrors production Kubernetes deployments. Previously, the project used a hybrid approach combining docker-compose for supporting services and Kind clusters for Kubernetes-specific testing.

### Current Challenges
- **Production Parity Gap**: Docker-compose networking and service discovery differs significantly from Kubernetes
- **Integration Test Fidelity**: Simplified container networking doesn't validate real Kubernetes behavior
- **RBAC and Security Testing**: Cannot test Kubernetes-native security features with docker-compose
- **Multi-node Scenarios**: Docker-compose cannot simulate distributed Kubernetes workloads

### Requirements Analysis
- **BR-INTEGRATION-001**: Real Kubernetes API testing for workflow engine validation
- **BR-PERFORMANCE-002**: Resource constraint validation with Kubernetes QoS classes
- **BR-SECURITY-003**: RBAC and network policy testing in realistic environment
- **BR-RELIABILITY-004**: Multi-node failure scenarios and pod scheduling behavior

## Decision

**We will use Kind (Kubernetes in Docker) as the primary integration environment for all development and CI/CD workflows.**

### Key Components in Kind Cluster
1. **Core Services**:
   - PostgreSQL with pgvector extension
   - Redis cache
   - Prometheus monitoring stack
   - AlertManager

2. **Application Services**:
   - Kubernaut webhook service
   - AI service components
   - HolmesGPT integration

3. **Supporting Infrastructure**:
   - Vector database (PostgreSQL-based)
   - Monitoring and observability stack
   - Network policies and RBAC configurations

## Rationale

### Advantages of Kind Cluster Approach

#### 1. Production Parity (High Impact)
```yaml
production_alignment:
  kubernetes_api: "Real Kubernetes API behavior vs mocked interfaces"
  networking: "Native Kubernetes DNS and service mesh"
  security: "Actual RBAC, network policies, and pod security standards"
  scheduling: "Real pod scheduling, resource limits, and QoS classes"
```

#### 2. Integration Test Fidelity
- **Real Kubernetes Interactions**: Tests actual kubectl, client-go, and operator behaviors
- **Native Service Discovery**: Uses Kubernetes DNS instead of docker-compose networking
- **Resource Management**: Validates CPU/memory limits and requests in realistic environment
- **Multi-node Testing**: Simulates distributed workloads across worker nodes

#### 3. Development Workflow Benefits
```yaml
consistency_benefits:
  development: "Kind cluster (Kubernetes-native)"
  ci_cd: "Kind cluster (identical environment)"
  staging: "Real Kubernetes cluster"
  production: "Real Kubernetes cluster"
  consistency_score: "95% vs 60% with docker-compose"
```

#### 4. Operational Advantages
- **Debugging**: Same tools and patterns used in production (kubectl, logs, describe)
- **Monitoring**: Real Prometheus integration with Kubernetes metrics
- **Security**: Actual Kubernetes security model validation
- **Scalability**: Multi-replica testing and horizontal pod autoscaling

### Resource Requirements

#### Minimum Development Environment
```yaml
system_requirements:
  memory: "6GB RAM (4GB for Kind + 2GB for services)"
  cpu: "4 cores (2 for Kind control plane + 2 for workloads)"
  storage: "20GB (cluster data + container images)"
  network: "Stable connection for image pulls and LLM communication"
```

#### CI/CD Environment
```yaml
ci_requirements:
  memory: "8GB RAM (optimized for parallel test execution)"
  cpu: "4-6 cores (concurrent test suites)"
  storage: "30GB (multiple test environments)"
  duration: "Setup: 60-90 seconds, Tests: 10-15 minutes"
```

## Implementation Strategy

### Phase 1: Infrastructure Migration (Immediate)
1. **Update Make Targets**: Modify `bootstrap-dev` to use Kind as primary environment
2. **Create Kubernetes Manifests**: Deploy all services as Kubernetes resources
3. **Update Documentation**: Mark docker-compose as deprecated, document Kind workflow

### Phase 2: Enhanced Integration (Short-term)
1. **Network Policies**: Implement realistic network isolation
2. **RBAC Configuration**: Add comprehensive role-based access controls
3. **Resource Limits**: Configure realistic CPU/memory constraints
4. **Monitoring Integration**: Full Prometheus/Grafana stack deployment

### Phase 3: Advanced Features (Medium-term)
1. **Multi-cluster Testing**: Simulate federated Kubernetes environments
2. **Chaos Engineering**: Integrate chaos testing with Kind clusters
3. **Performance Benchmarking**: Standardized performance testing in Kind

## Migration Plan

### Immediate Changes (Week 1)
```bash
# Update primary make targets
make bootstrap-dev          # Now uses Kind cluster
make test-integration-dev   # Runs against Kind cluster
make cleanup-dev           # Cleans Kind cluster and resources

# Deprecated (maintained for compatibility)
make bootstrap-dev-compose  # Legacy docker-compose setup
```

### Documentation Updates
1. **Update README.md**: Reflect Kind as primary development environment
2. **Update Getting Started**: Kind-first installation instructions
3. **Create Migration Guide**: Help developers transition from docker-compose
4. **Update Troubleshooting**: Kind-specific debugging procedures

### Kubernetes Manifests Structure
```
deploy/integration/
├── namespace.yaml              # Integration testing namespace
├── postgresql/
│   ├── deployment.yaml         # PostgreSQL with pgvector
│   ├── service.yaml           # Database service
│   └── configmap.yaml         # Database configuration
├── redis/
│   ├── deployment.yaml         # Redis cache
│   └── service.yaml           # Redis service
├── monitoring/
│   ├── prometheus/            # Prometheus stack
│   ├── alertmanager/          # AlertManager configuration
│   └── grafana/               # Grafana dashboards
├── kubernaut/
│   ├── webhook-service.yaml   # Webhook service deployment
│   ├── ai-service.yaml        # AI service deployment
│   └── rbac.yaml              # RBAC configuration
└── holmesgpt/
    ├── deployment.yaml         # HolmesGPT integration
    └── service.yaml           # HolmesGPT service
```

## Consequences

### Positive Impacts
- **Higher Test Fidelity**: 95% production parity vs 60% with docker-compose
- **Better Debugging**: Same tools and workflows as production
- **Enhanced Security Testing**: Real Kubernetes security model validation
- **Improved CI/CD**: Consistent environment across all stages
- **Future-Proof Architecture**: Aligns with Kubernetes-native development practices

### Challenges and Mitigation
1. **Resource Overhead**:
   - **Challenge**: Higher memory/CPU requirements
   - **Mitigation**: Optimized Kind configuration, resource limits, cleanup automation

2. **Complexity Increase**:
   - **Challenge**: More complex debugging and troubleshooting
   - **Mitigation**: Comprehensive documentation, debugging guides, automation scripts

3. **Setup Time**:
   - **Challenge**: Longer bootstrap time (60-90 seconds vs 30-45 seconds)
   - **Mitigation**: Parallel initialization, caching, optimized images

### Backward Compatibility
- **Docker-compose**: Maintained as deprecated option for 6 months
- **Migration Period**: Gradual transition with clear migration path
- **Documentation**: Both approaches documented during transition period

## Success Metrics

### Technical Metrics
```yaml
success_criteria:
  test_fidelity: ">90% production behavior match"
  setup_time: "<90 seconds for full environment"
  test_execution: "<15 minutes for integration test suite"
  resource_usage: "<6GB RAM for development environment"
  failure_rate: "<5% environment setup failures"
```

### Developer Experience Metrics
- **Onboarding Time**: New developer environment setup <10 minutes
- **Debug Efficiency**: Reduced time to identify integration issues
- **CI/CD Reliability**: >95% successful test runs
- **Documentation Quality**: Complete migration within 2 weeks

## Related Decisions
- **ADR-001**: Kubernetes-Native Architecture (supports this decision)
- **ADR-002**: Integration Testing Strategy (implemented through this decision)
- **Future ADR**: Multi-cluster Federation Testing (enabled by this decision)

## References
- [Kind Documentation](https://kind.sigs.k8s.io/)
- [Kubernetes Integration Testing Best Practices](https://kubernetes.io/docs/reference/using-api/api-concepts/)
- [kubernaut Integration Testing Strategy](../test/integration/README.md)
- [Development Environment Setup Guide](../development/GETTING_STARTED.md)

---

**Decision Date**: December 2024
**Review Date**: June 2025
**Status**: ACCEPTED
**Impact**: HIGH - Affects all development and CI/CD workflows
