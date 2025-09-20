# Kubernaut Integration Test Plan - Milestone 1

**Document Version**: 1.0
**Date**: January 2025
**Scope**: Milestone 1 Production Readiness Integration Testing
**Environment**: envtest (lightweight K8s) + Local Infrastructure + Optional Kind cluster
**Execution**: Automated integration tests with manual validation

---

## 🎯 **Executive Summary**

This integration test plan validates **Milestone 1 production readiness** by testing core business requirements against a real Kubernetes environment. The plan focuses on replacing fake clients with real K8s integration while maintaining the high confidence levels achieved in unit testing.

### **Key Objectives**
- ✅ Validate 99.9% availability under production-like load
- ✅ Confirm <5 second alert processing time compliance
- ✅ Verify 100 concurrent request handling capability
- ✅ Test 25+ Kubernetes remediation actions with 95% success rate
- ✅ Validate enterprise-grade security and RBAC integration

### **Test Strategy**
- **Environment**: envtest for K8s API testing + Optional Kind cluster for full integration
- **Data**: Synthetic alert generation matching real Prometheus formats
- **Execution**: Automated integration tests with Go/Ginkgo framework
- **Infrastructure**: Local host + LLM model at 192.168.1.169:8080 + Redis with authentication

---

## 📊 **Business Requirements Coverage**

### **Core Production Requirements**
| Business Requirement | Test Suite | Success Criteria | Priority |
|----------------------|------------|------------------|----------|
| **BR-PA-001** | Alert Reception | 99.9% availability | Critical |
| **BR-PA-003** | Processing Time | <5 second response | Critical |
| **BR-PA-004** | Concurrent Load | 100 concurrent requests | Critical |
| **BR-PA-006** | LLM Integration | Multi-provider support | High |
| **BR-PA-008** | Effectiveness | Historical analysis | High |
| **BR-PA-011** | K8s Actions | 25+ actions, 95% success | Critical |
| **BR-PA-012** | Safety Mechanisms | Zero destructive actions | Critical |

### **Platform & Security Requirements**
| Business Requirement | Test Suite | Success Criteria | Priority |
|----------------------|------------|------------------|----------|
| **Platform Uptime** | Platform Operations | 99.9% service availability | Critical |
| **RBAC Integration** | Security Validation | Permission enforcement | High |
| **State Persistence** | State Management | Recovery from restarts | High |
| **Circuit Breakers** | Resilience Testing | Graceful degradation | Medium |

---

## 🏗️ **Test Environment Architecture**

```
Local Host Environment:
├── Kind Cluster (Docker)
│   ├── Control Plane (1 node)
│   ├── Worker Nodes (2 nodes)
│   ├── Prometheus (monitoring)
│   ├── PostgreSQL (state storage)
│   └── Test Workloads (targets)
├── Kubernaut Application
│   ├── Prometheus Alerts SLM
│   ├── Context API Server
│   └── Dynamic Toolset Server
├── LLM Model (192.168.1.169:8080)
│   └── Local LLM for AI processing (ramalama - gpt-oss:20b)
├── Redis (localhost:6379)
│   └── Authentication: integration_redis_password
└── Test Infrastructure
    ├── Alert Generator
    ├── Load Testing Tools
    └── Monitoring/Validation
```

---

## 📋 **Test Suite Structure**

```
docs/test/integration/
├── README.md                           # This overview
├── ENVIRONMENT_SETUP.md               # Complete setup instructions
├── INTEGRATION_TEST_COVERAGE_EXTENSION_PLAN.md   # Comprehensive integration test plan
├── TEST_EXECUTION_GUIDE.md           # Manual execution procedures
└── test_suites/
    ├── 00_environment_validation/
    │   ├── kind_cluster_health.md
    │   ├── ollama_connectivity.md
    │   └── infrastructure_readiness.md
    ├── 01_alert_processing/
    │   ├── BR-PA-001_availability_test.md
    │   ├── BR-PA-003_processing_time_test.md
    │   ├── BR-PA-004_concurrent_load_test.md
    │   └── BR-PA-005_alert_ordering_test.md
    ├── 02_ai_decision_making/
    │   ├── BR-PA-006_llm_provider_test.md
    │   ├── BR-PA-007_remediation_recommendations.md
    │   ├── BR-PA-008_effectiveness_assessment_test.md
    │   ├── BR-PA-009_confidence_scoring_test.md
    │   └── BR-PA-010_dry_run_mode_test.md
    ├── 03_remediation_actions/
    │   ├── BR-PA-011_k8s_actions_test.md
    │   ├── BR-PA-012_safety_mechanisms_test.md
    │   ├── BR-PA-013_rollback_test.md
    │   └── k8s_action_catalog_validation.md
    ├── 04_platform_operations/
    │   ├── platform_uptime_test.md
    │   ├── concurrent_execution_test.md
    │   ├── k8s_api_performance_test.md
    │   └── monitoring_integration_test.md
    ├── 05_security_state/
    │   ├── rbac_integration_test.md
    │   ├── state_persistence_test.md
    │   ├── circuit_breaker_test.md
    │   └── security_boundary_test.md
    └── 99_performance_analysis/
        ├── load_testing_results.md
        ├── performance_benchmarks.md
        └── capacity_analysis.md
```

---

## 🚀 **Test Execution Phases**

### **Phase 1: Environment Setup & Validation** (Day 1)
**Duration**: 4-6 hours
**Objective**: Establish reliable test environment

**Tasks**:
1. **Kind Cluster Setup**: Bootstrap 3-node cluster with networking
2. **Infrastructure Deployment**: Deploy PostgreSQL, Prometheus, test workloads
3. **Kubernaut Deployment**: Deploy all Milestone 1 components
4. **Connectivity Validation**: Verify Ollama model integration
5. **Baseline Testing**: Confirm basic functionality before load testing

**Success Criteria**: All infrastructure components healthy, basic alert processing functional

### **Phase 2: Core Functionality Validation** (Day 2)
**Duration**: 6-8 hours
**Objective**: Validate core business requirements

**Test Sequence**:
1. **Alert Processing Tests**: BR-PA-001-005 validation
2. **AI Decision Making Tests**: BR-PA-006-010 validation
3. **Remediation Action Tests**: BR-PA-011-013 validation
4. **Basic Performance Tests**: Confirm response time requirements

**Success Criteria**: All core business requirements pass, no critical failures

### **Phase 3: Load & Performance Testing** (Day 3)
**Duration**: 6-8 hours
**Objective**: Validate performance under production-like load

**Test Sequence**:
1. **Concurrent Load Testing**: 100 concurrent requests validation
2. **Throughput Testing**: 1000 alerts/minute processing
3. **Availability Testing**: 99.9% uptime under continuous load
4. **Resource Monitoring**: CPU, memory, network utilization

**Success Criteria**: All performance requirements met, system stability confirmed

### **Phase 4: Security & Resilience Testing** (Day 4)
**Duration**: 4-6 hours
**Objective**: Validate production security and resilience

**Test Sequence**:
1. **RBAC Integration**: Permission enforcement validation
2. **State Persistence**: Recovery from various failure scenarios
3. **Circuit Breaker**: Service degradation and recovery testing
4. **Security Boundaries**: Safety mechanism enforcement

**Success Criteria**: Security requirements met, graceful failure handling confirmed

### **Phase 5: Results Analysis & Documentation** (Day 5)
**Duration**: 4-6 hours
**Objective**: Comprehensive results analysis and reporting

**Tasks**:
1. **Performance Analysis**: Detailed performance characteristic documentation
2. **Requirement Compliance**: Business requirement pass/fail summary
3. **Issue Documentation**: Any failures or concerns identified
4. **Pilot Readiness Assessment**: Go/no-go recommendation for pilot deployment

---

## 📊 **Success Criteria & Metrics**

### **Critical Success Criteria** (Must Pass)
- **BR-PA-001**: 99.9% availability (max 8.6 seconds downtime in 24 hours)
- **BR-PA-003**: <5 second alert processing time (95th percentile)
- **BR-PA-004**: 100 concurrent requests handled successfully
- **BR-PA-011**: 25+ K8s actions execute with 95% success rate
- **BR-PA-012**: Zero destructive actions executed in safety mode

### **Performance Benchmarks**
- **Throughput**: 1000 alerts/minute sustained processing
- **Response Time**: <5 seconds for 99% of requests
- **Resource Usage**: <80% CPU/memory utilization under load
- **Error Rate**: <1% processing errors under normal load

### **Quality Gates**
- **All Critical Tests Pass**: No compromises on critical business requirements
- **Performance Within SLA**: All timing and throughput requirements met
- **Security Validated**: RBAC and safety mechanisms functioning
- **Stability Confirmed**: No system crashes or data corruption

---

## 🔧 **Infrastructure Requirements**

### **Local Host Requirements**
- **Docker**: Latest version with Kind support (optional for Phase 1)
- **Resources**: 8GB+ RAM, 4+ CPU cores, 20GB+ disk
- **Network**: Internet access for container images
- **LLM Service**: Model running at 192.168.1.169:8080 (ramalama/ollama)
- **Redis**: Running with password: integration_redis_password

### **Kind Cluster Configuration** (Optional for Phase 1)
- **Nodes**: 1 control plane + 2 workers
- **Networking**: Calico or default CNI
- **Storage**: Local persistent volumes
- **Load Balancer**: MetalLB for service exposure
- **Note**: Phase 1 tests use envtest for lightweight K8s API testing

### **Monitoring & Observability**
- **Prometheus**: Metrics collection and alerting
- **Grafana**: Real-time dashboard (optional)
- **Custom Metrics**: Application-specific monitoring

---

## 📝 **Test Data Strategy**

### **Synthetic Alert Generation**
- **Alert Types**: High memory, CPU usage, pod crashes, storage issues, network problems
- **Severity Levels**: Critical, warning, info alerts
- **Volume Control**: Configurable rates (10/min to 1000/min)
- **Realistic Format**: Match Prometheus AlertManager webhook format

### **Test Scenarios**
- **Normal Operations**: Steady state with typical alert rates
- **Burst Testing**: Sudden spikes in alert volume
- **Mixed Workload**: Different alert types and severities
- **Edge Cases**: Malformed alerts, timeout scenarios

---

## 🎯 **Next Steps**

1. **Review Performance Testing Options**: Choose preferred approach (Options 1, 2, or 3)
2. **Environment Setup**: Begin Kind cluster bootstrap and infrastructure deployment
3. **Test Execution**: Follow phase-by-phase manual execution
4. **Results Analysis**: Comprehensive evaluation of Milestone 1 readiness

**Estimated Total Time**: 5 days of focused testing (2-person team)
**Expected Outcome**: High-confidence assessment of production readiness for pilot deployment

---

## 📚 **Related Documentation**
- [Environment Setup Guide](ENVIRONMENT_SETUP.md)
- [Integration Test Coverage Extension Plan](INTEGRATION_TEST_COVERAGE_EXTENSION_PLAN.md)
- [Test Execution Procedures](TEST_EXECUTION_GUIDE.md)
- [Performance Analysis Framework](test_suites/99_performance_analysis/README.md)
