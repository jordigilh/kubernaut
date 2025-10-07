# ⚠️ DEPRECATED: kubernaut main binary

## 🚨 **THIS BINARY IS DEPRECATED**

This directory contains a **deprecated** main kubernaut binary that should **NOT** be used.

## 🏗️ **Use Microservices Instead**

According to the **APPROVED_MICROSERVICES_ARCHITECTURE.md**, Kubernaut is now composed of **10 independent microservices**:

### **Available Services**

✅ **Implemented**:
- **🤖 AI Analysis Service** - `cmd/ai-service/` (Port 8082)

🚧 **To Be Implemented**:
- **🔗 Gateway Service** - `cmd/gateway-service/` (Port 8080)
- **🧠 Alert Processor Service** - `cmd/alert-service/` (Port 8081)
- **🎯 Workflow Orchestrator Service** - `cmd/workflow-service/` (Port 8083)
- **⚡ K8s Executor Service** - `cmd/executor-service/` (Port 8084)
- **📊 Data Storage Service** - `cmd/storage-service/` (Port 8085)
- **🔍 Intelligence Service** - `cmd/intelligence-service/` (Port 8086)
- **📈 Effectiveness Monitor Service** - `cmd/monitor-service/` (Port 8087)
- **🌐 Context API Service** - `cmd/context-service/` (Port 8088)
- **📢 Notification Service** - `cmd/notification-service/` (Port 8089)

## 🚀 **Quick Start**

To use Kubernaut, run the individual microservices:

```bash
# AI Analysis Service (Ready to use)
cd cmd/ai-service/
go run .

# Other services (To be implemented)
# cd cmd/gateway-service/ && go run .
# cd cmd/alert-service/ && go run .
# etc.
```

## 📚 **Documentation**

- **Architecture**: See `docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md`
- **AI Service**: See `cmd/ai-service/MICROSERVICE_COMPLIANCE_REPORT.md`
- **Testing**: See `test/unit/ai/service/TESTING_STRATEGY_COMPLIANCE_REPORT.md`

## ⚠️ **Migration Notice**

If you were previously using `cmd/kubernaut/main.go`:

1. **Stop using it immediately** - it's deprecated
2. **Use the appropriate microservice** instead
3. **For AI functionality**: Use `cmd/ai-service/`
4. **For other functionality**: Implement the respective microservice

## 🗑️ **Removal Timeline**

This directory will be **completely removed** once all microservices are implemented.

---

**Status**: ⚠️ **DEPRECATED**
**Replacement**: **Independent Microservices**
**Next Steps**: Implement remaining microservices per approved architecture

