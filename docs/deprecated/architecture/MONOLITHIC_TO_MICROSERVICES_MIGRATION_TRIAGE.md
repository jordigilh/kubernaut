# ⚠️ **DEPRECATED** - Monolithic to Microservices Migration - Documentation Triage

**Document Version**: 1.0
**Date**: September 28, 2025
**Status**: **DEPRECATED** - Migration Completed, Document Obsolete
**Migration Target**: Align with APPROVED_MICROSERVICES_ARCHITECTURE.md

---

## 🚨 **DEPRECATION NOTICE**

**This document is DEPRECATED and should not be used for current development.**

- **Reason**: Migration from monolithic to microservices has been completed
- **Replacement**: See current V1 architecture in [KUBERNAUT_ARCHITECTURE_OVERVIEW.md](KUBERNAUT_ARCHITECTURE_OVERVIEW.md)
- **Current Status**: V1 implementation with 10 services is active
- **Last Updated**: January 2025

**⚠️ Do not use this information for architectural decisions.**

---

---

## 🎯 **EXECUTIVE SUMMARY**

This document identifies **34 documentation files** that contain references to the deprecated monolithic "kubernaut" binary and need to be updated to reflect the approved 10-microservice architecture. The main kubernaut binary has been properly deprecated with clear migration guidance.

### **Migration Status**
- ✅ **Main Binary**: `cmd/kubernaut/main.go` correctly deprecated with microservices guidance
- ❌ **Documentation**: 34 files still reference monolithic approach
- 🎯 **Target Architecture**: 10 independent microservices per APPROVED_MICROSERVICES_ARCHITECTURE.md

---

## 🚨 **CRITICAL CONFLICTS IDENTIFIED**

### **Category 1: Build & Deployment Instructions**
**Impact**: HIGH - Users will get incorrect setup instructions

| File | Issue | Required Update |
|------|-------|----------------|
| `docs/deployment/CONTEXT_API_DEPLOYMENT_ASSESSMENT.md` | References `cmd/kubernaut/main.go` and `go run ./cmd/kubernaut` | Update to Context API Service (Port 8088) |
| `docs/getting-started/INTEGRATION_EXAMPLE.md` | Shows `./kubernaut` execution | Update to service-specific examples |
| `docs/getting-started/setup/DEPLOYMENT.md` | Uses `kubectl logs -f deployment/kubernaut` | Update to individual service deployments |
| `docs/getting-started/setup/LLM_SETUP_GUIDE.md` | Shows `./kubernaut --config` commands | Update to AI Analysis Service (Port 8082) |
| `docs/deployment/VECTOR_DATABASE_SETUP.md` | Uses `./kubernaut --config` | Update to Data Storage Service (Port 8085) |

### **Category 2: Container Images & Registry**
**Impact**: HIGH - Wrong container references

| File | Issue | Required Update |
|------|-------|----------------|
| `docs/deployment/CONTAINER_REGISTRY.md` | References `kubernaut-go-builder`, `kubernaut-runtime` | Update to 10 service-specific images |
| `docs/implementation/PROCESSOR_SERVICE_IMPLEMENTATION_PLAN.md` | Shows single `kubernaut:v1.0.0` image | Update to microservices images |
| `docs/deployment/MICROSERVICES_DEPLOYMENT_GUIDE.md` | Mixed references to `charts/kubernaut/` | Update to individual service charts |

### **Category 3: Configuration & Architecture**
**Impact**: MEDIUM - Architectural misunderstanding

| File | Issue | Required Update |
|------|-------|----------------|
| `docs/deployment/HOLMESGPT_HYBRID_SETUP_GUIDE.md` | Single `kubernaut-toolset.yaml` config | Update to service-specific configs |
| `docs/deployment/HOLMESGPT_HYBRID_ARCHITECTURE.md` | References `kubernaut-context-api:8091` | Update to context-service:8091 |
| `docs/backup-rules/01-project-structure.mdc` | States "Main kubernaut service" | Update to microservices structure |

---

## 📋 **DETAILED FILE INVENTORY**

### **High Priority - Deployment/Setup Files**
1. `docs/deployment/CONTEXT_API_DEPLOYMENT_ASSESSMENT.md`
   - **Issues**: cmd/kubernaut/main.go references, single binary deployment
   - **Update**: Context API Service architecture and deployment

2. `docs/getting-started/INTEGRATION_EXAMPLE.md`
   - **Issues**: `./kubernaut` execution examples
   - **Update**: Service-specific integration examples

3. `docs/getting-started/setup/DEPLOYMENT.md`
   - **Issues**: Single `deployment/kubernaut` kubectl commands
   - **Update**: 10 individual service deployments

4. `docs/getting-started/setup/LLM_SETUP_GUIDE.md`
   - **Issues**: `./kubernaut --config` commands
   - **Update**: AI Analysis Service configuration

5. `docs/deployment/VECTOR_DATABASE_SETUP.md`
   - **Issues**: Single binary configuration
   - **Update**: Data Storage Service setup

### **Medium Priority - Development/Testing Files**
6. `docs/testing/PYRAMID_TEST_MIGRATION_GUIDE.md`
   - **Issues**: Multiple cmd/kubernaut/main.go references
   - **Update**: Service-specific testing approaches

7. `docs/implementation/PROCESSOR_SERVICE_IMPLEMENTATION_PLAN.md`
   - **Issues**: Single kubernaut container image references
   - **Update**: Alert Processor Service specifics

8. `docs/backup-rules/01-project-structure.mdc`
   - **Issues**: "Main kubernaut service" description
   - **Update**: Microservices project structure

### **Low Priority - Documentation Rules & Guides**
9-34. Various backup rules, implementation plans, and guides containing:
   - Import path references (`github.com/jordigilh/kubernaut/pkg/...`)
   - cmd/kubernaut/main.go integration examples
   - Single binary assumptions

---

## 🔄 **MICROSERVICES MAPPING**

### **Service Responsibility Mapping**
| Old Monolithic Reference | New Microservice | Port | Image |
|--------------------------|------------------|------|-------|
| `./kubernaut` (webhooks) | Gateway Service | 8080 | quay.io/jordigilh/gateway-service |
| `./kubernaut` (alerts) | Alert Processor Service | 8081 | quay.io/jordigilh/alert-service |
| `./kubernaut` (AI/LLM) | AI Analysis Service | 8082 | quay.io/jordigilh/ai-service |
| `./kubernaut` (workflows) | Workflow Orchestrator Service | 8083 | quay.io/jordigilh/workflow-service |
| `./kubernaut` (k8s ops) | K8s Executor Service | 8084 | quay.io/jordigilh/executor-service |
| `./kubernaut` (storage) | Data Storage Service | 8085 | quay.io/jordigilh/storage-service |
| `./kubernaut` (patterns) | Intelligence Service | 8086 | quay.io/jordigilh/intelligence-service |
| `./kubernaut` (monitoring) | Effectiveness Monitor Service | 8087 | quay.io/jordigilh/monitor-service |
| `./kubernaut` (context) | Context API Service | 8088 | quay.io/jordigilh/context-service |
| `./kubernaut` (notifications) | Notification Service | 8089 | quay.io/jordigilh/notification-service |

---

## ✅ **UPDATE CHECKLIST**

### **Phase 1: Critical User-Facing Updates**
- [ ] Update deployment instructions to use individual services
- [ ] Replace single binary execution with service-specific commands
- [ ] Update container image references to service-specific images
- [ ] Fix kubectl commands to target individual service deployments
- [ ] Update configuration examples for microservices

### **Phase 2: Development Documentation**
- [ ] Update project structure documentation
- [ ] Replace cmd/kubernaut/main.go references with service-specific entry points
- [ ] Update testing guides for microservices approach
- [ ] Fix integration examples to use service-to-service communication

### **Phase 3: Implementation Plans & Guides**
- [ ] Update backup rules and development methodology
- [ ] Fix implementation plans to reflect microservices
- [ ] Update import path examples for service contexts
- [ ] Align all documentation with APPROVED_MICROSERVICES_ARCHITECTURE.md

---

## 🎯 **RECOMMENDED UPDATE STRATEGY**

### **1. Immediate Priority (Week 1)**
Focus on user-facing deployment and setup documentation that could mislead new users:
- Deployment guides
- Setup instructions
- Getting started examples
- Container registry documentation

### **2. Development Priority (Week 2)**
Update development-focused documentation:
- Testing guides
- Implementation plans
- Project structure documentation
- Development methodology

### **3. Cleanup Priority (Week 3)**
Address remaining references in:
- Backup rules
- Archived documentation
- Implementation plans
- Legacy guides

---

## 📊 **MIGRATION IMPACT ASSESSMENT**

### **Documentation Health**
- **Total Files Affected**: 34 files
- **Critical Path Files**: 8 files (deployment/setup)
- **Development Files**: 12 files (testing/implementation)
- **Supporting Files**: 14 files (rules/guides)

### **User Impact**
- **High**: New users following setup guides will be confused
- **Medium**: Developers referencing implementation docs
- **Low**: Users reading backup rules or archived content

### **Migration Effort**
- **Estimated Time**: 3-4 weeks for complete migration
- **Complexity**: Medium (systematic replacement required)
- **Risk**: Low (documentation updates only)

---

**Document Status**: ✅ **TRIAGE COMPLETE**
**Next Step**: Begin systematic documentation updates per approved microservices architecture
**Priority**: HIGH - User-facing documentation needs immediate attention

This triage provides the roadmap for aligning all documentation with the approved 10-microservice architecture while ensuring users receive accurate setup and deployment guidance.