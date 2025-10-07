# ⚠️ DEPRECATED: cmd/kubernaut/ Directory

## 🚨 **THIS DIRECTORY IS DEPRECATED AND SHOULD NOT BE USED**

### **Reason for Deprecation**

According to the **APPROVED_MICROSERVICES_ARCHITECTURE.md**, there is **NO main kubernaut application**. The system has been decomposed into **10 independent microservices**, each with its own responsibility and deployment lifecycle.

### **Approved Microservices Architecture**

The Kubernaut system consists of these **10 independent microservices**:

| Service | Port | Container Image | Responsibility |
|---------|------|----------------|----------------|
| 🔗 **Gateway Service** | 8080 | `quay.io/jordigilh/gateway-service` | HTTP Gateway & Security |
| 🧠 **Alert Processor Service** | 8081 | `quay.io/jordigilh/alert-service` | Alert Processing Logic |
| 🤖 **AI Analysis Service** | 8082 | `quay.io/jordigilh/ai-service` | AI Analysis & Decision Making |
| 🎯 **Workflow Orchestrator Service** | 8083 | `quay.io/jordigilh/workflow-service` | Workflow Execution |
| ⚡ **K8s Executor Service** | 8084 | `quay.io/jordigilh/executor-service` | Kubernetes Operations |
| 📊 **Data Storage Service** | 8085 | `quay.io/jordigilh/storage-service` | Data Persistence |
| 🔍 **Intelligence Service** | 8086 | `quay.io/jordigilh/intelligence-service` | Pattern Discovery |
| 📈 **Effectiveness Monitor Service** | 8087 | `quay.io/jordigilh/monitor-service` | Effectiveness Assessment |
| 🌐 **Context API Service** | 8088 | `quay.io/jordigilh/context-service` | Context Orchestration |
| 📢 **Notification Service** | 8089 | `quay.io/jordigilh/notification-service` | Multi-Channel Notifications |

### **Migration Path**

Instead of using this deprecated binary, use the appropriate microservice:

- **For AI analysis**: Use `cmd/ai-service/` (Port 8082) ✅ **IMPLEMENTED**
- **For alert processing**: Implement `cmd/alert-service/` (Port 8081)
- **For workflow orchestration**: Implement `cmd/workflow-service/` (Port 8083)
- **For Kubernetes operations**: Implement `cmd/executor-service/` (Port 8084)
- **For data storage**: Implement `cmd/storage-service/` (Port 8085)
- **For intelligence**: Implement `cmd/intelligence-service/` (Port 8086)
- **For monitoring**: Implement `cmd/monitor-service/` (Port 8087)
- **For context API**: Implement `cmd/context-service/` (Port 8088)
- **For notifications**: Implement `cmd/notification-service/` (Port 8089)
- **For gateway**: Implement `cmd/gateway-service/` (Port 8080)

### **What Happens If You Try to Use This**

If you attempt to run the deprecated `kubernaut` binary, it will:

1. Display a deprecation warning
2. Show the list of available microservices
3. Exit with status code 1

### **Removal Timeline**

This directory and binary will be **completely removed** in a future version once all microservices are implemented.

### **Architecture Benefits**

The microservices architecture provides:

- **Single Responsibility Principle**: Each service has exactly one responsibility
- **Independent Scaling**: Services scale based on their specific workload
- **Independent Deployment**: Services can be deployed and updated independently
- **Fault Isolation**: Failure in one service doesn't affect others
- **Technology Diversity**: Each service can use the most appropriate technology stack

### **For Developers**

**DO NOT**:
- Reference `cmd/kubernaut/main.go` in new code
- Import packages from this directory
- Build or deploy this binary
- Use this as a template for new services

**DO**:
- Use the individual microservices in `cmd/[service-name]/`
- Follow the approved microservices architecture
- Implement missing microservices as needed
- Refer to `cmd/ai-service/` as the reference implementation

---

**Status**: ⚠️ **DEPRECATED**
**Replacement**: **10 Independent Microservices**
**Reference Implementation**: `cmd/ai-service/` ✅

