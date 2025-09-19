# HolmesGPT Port Update Summary - 8080 → 8090

**Date**: January 2025
**Reason**: Avoid port conflict with ramalama default port 8080

---

## ✅ **Updated References - HolmesGPT Service Port 8080 → 8090**

### **Configuration Files**
- ✅ `config/local-llm.yaml` - holmesgpt endpoint
- ✅ `config/production-holmesgpt.yaml` - holmesgpt endpoint
- ✅ `deploy/holmesgpt-e2e-values.yaml` - service port, target port, API port, environment variables

### **Scripts**
- ✅ `scripts/test-holmesgpt-integration.sh` - HOLMES_URL and health check
- ✅ `scripts/deploy-holmesgpt-e2e.sh` - service definition, port forward, health checks
- ✅ `scripts/run-holmesgpt-local.sh` - API port, service URLs, health checks

### **Documentation**
- ✅ `docs/development/HOLMESGPT_QUICKSTART.md` - URLs and port forwards
- ✅ `docs/development/HOLMESGPT_DIRECT_INTEGRATION.md` - endpoints and examples
- ✅ `README.md` - service URLs and metrics
- ✅ `PHASE_2_MIGRATION_SUMMARY.md` - configuration example

---

## ✅ **Preserved References - LLM Service Port (Stays 8080)**

### **LLM Endpoints (Correctly Unchanged)**
- ✅ `192.168.1.169:8080` - ramalama/LocalAI service endpoint
- ✅ `http://192.168.1.169:8080/v1` - LLM API endpoints
- ✅ Network policy egress port 8080 for LLM connectivity

### **Kubernaut Service (Correctly Unchanged)**
- ✅ `webhook_port: "8080"` - Go service webhook port
- ✅ `server.webhook_port` - Main application ports

---

## 🎯 **Port Allocation Summary**

| Service | Port | Purpose |
|---------|------|---------|
| **Ramalama/LocalAI** | 8080 | LLM inference service |
| **Kubernaut Go Service** | 8080 | Webhook receiver, health endpoints |
| **HolmesGPT** | **8090** | AI investigation service (**Updated**) |
| **Prometheus Metrics** | 9090 | Metrics collection |
| **Health Check** | 8081 | Health monitoring |

---

## 🧪 **Updated Testing Commands**

### **Local Development**
```bash
# Start HolmesGPT (now on port 8090)
./scripts/run-holmesgpt-local.sh

# Test HolmesGPT health
curl http://localhost:8090/health

# Test HolmesGPT API docs
curl http://localhost:8090/docs

# Test LLM (still on port 8080)
curl http://192.168.1.169:8080/v1/models
```

### **Kubernetes Deployment**
```bash
# Deploy HolmesGPT
./scripts/deploy-holmesgpt-e2e.sh

# Port forward (updated port)
kubectl port-forward -n kubernaut-system svc/holmesgpt-e2e 8090:8090

# Test in cluster
kubectl run debug --image=curlimages/curl -it --rm -- \
  curl http://holmesgpt-e2e.kubernaut-system.svc.cluster.local:8090/health
```

---

## ✅ **Configuration Examples Updated**

### **Development Config**
```yaml
ai_services:
  holmesgpt:
    enabled: true
    endpoint: "http://localhost:8090"  # Updated
    mode: "development"
```

### **Production Config**
```yaml
ai_services:
  holmesgpt:
    enabled: true
    endpoint: "http://holmesgpt-e2e.kubernaut-system.svc.cluster.local:8090"  # Updated
    mode: "production"
```

---

**🎉 Port conflict resolved! HolmesGPT now runs on 8090, ramalama stays on 8080.**
