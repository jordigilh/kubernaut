# Gateway & Webhook Dockerfile Merge Plan

**Date**: September 27, 2025
**Status**: **READY FOR IMPLEMENTATION**
**Objective**: Merge webhook-service.Dockerfile into gateway-service.Dockerfile per approved architecture

---

## 🎯 **MERGE OBJECTIVE**

Consolidate webhook processing functionality into the gateway service to align with the approved microservices architecture where the gateway service handles ALL HTTP gateway operations including webhook processing.

---

## 📊 **COMPATIBILITY ANALYSIS**

### **✅ COMPATIBLE ASPECTS**
| Component | Status | Details |
|-----------|--------|---------|
| **Base Images** | ✅ Identical | Both use `registry.access.redhat.com/ubi9/go-toolset:1.24` |
| **Build Process** | ✅ Compatible | Both use standard multi-stage Go builds |
| **Security Model** | ✅ Compatible | Both use non-root users with UID 1001 |
| **Runtime Environment** | ✅ Compatible | Both use UBI9 minimal runtime |
| **Health Check Pattern** | ✅ Compatible | Both use binary `--health-check` flag |

### **⚠️ CONFLICTING ASPECTS**
| Component | webhook-service | gateway-service | Resolution |
|-----------|----------------|-----------------|------------|
| **Ports** | `8080 9090 8081` | `8080 9090 8081` | ✅ Same ports - No conflict |
| **Binary Path** | `./cmd/webhook-service` | `./cmd/gateway-service` | ✅ Use gateway path |
| **User Name** | `webhook-user` | `gateway-user` | ✅ Use gateway-user |
| **Service Focus** | Webhook processing | HTTP Gateway | ✅ Combine descriptions |

---

## 🔄 **MERGE STRATEGY**

### **Primary Approach: Enhanced Gateway Service**
- **Keep**: `gateway-service.Dockerfile` as the primary Dockerfile
- **Enhance**: Add webhook processing capabilities to description and labels
- **Deprecate**: `webhook-service.Dockerfile` after merge
- **Maintain**: All existing gateway service functionality

### **Architecture Alignment**
```
BEFORE (Conflicting):
┌─────────────────┐    ┌─────────────────┐
│ webhook-service │    │ gateway-service │
│ Port: 8080      │ ❌ │ Port: 8080      │
│ Webhook only    │    │ Gateway only    │
└─────────────────┘    └─────────────────┘

AFTER (Aligned):
┌─────────────────────────────────────┐
│        gateway-service              │
│        Port: 8080                   │
│  ✅ HTTP Gateway + Webhook Processing │
│  ✅ Authentication & Authorization   │
│  ✅ Rate Limiting & Security        │
└─────────────────────────────────────┘
```

---

## 📋 **IMPLEMENTATION STEPS**

### **Step 1: Enhanced Gateway Dockerfile**
**File**: `docker/gateway-service.Dockerfile`
**Action**: Update labels and descriptions to reflect combined functionality

#### **Changes Required**:
```dockerfile
# CURRENT SUMMARY:
summary="Kubernaut Gateway Service - HTTP Gateway & Security Microservice"

# ENHANCED SUMMARY:
summary="Kubernaut Gateway Service - HTTP Gateway, Webhook Processing & Security Microservice"

# CURRENT DESCRIPTION:
description="A microservice component of Kubernaut that handles HTTP gateway operations, authentication, authorization, rate limiting, and security enforcement..."

# ENHANCED DESCRIPTION:
description="A microservice component of Kubernaut that handles HTTP gateway operations, webhook processing (Prometheus AlertManager, Grafana), authentication, authorization, rate limiting, and security enforcement..."

# CURRENT TAGS:
io.openshift.tags="kubernaut,gateway,security,http,microservice"

# ENHANCED TAGS:
io.openshift.tags="kubernaut,gateway,webhook,security,http,alertmanager,microservice"
```

### **Step 2: Code Integration Verification**
**Action**: Verify gateway service includes webhook functionality

#### **Validation Checklist**:
- [ ] **Binary Check**: Confirm `./cmd/gateway-service` exists and builds
- [ ] **Webhook Endpoints**: Verify AlertManager webhook endpoints in gateway service
- [ ] **Port Usage**: Confirm gateway service uses port 8080 for HTTP
- [ ] **Functionality**: Ensure webhook processing logic is integrated

#### **Verification Commands**:
```bash
# Check if gateway service binary path exists
ls -la cmd/gateway-service/

# Verify gateway service builds
go build ./cmd/gateway-service

# Check for webhook-related code in gateway service
grep -r "webhook\|alertmanager" cmd/gateway-service/ || echo "No webhook code found"

# Verify port configuration
grep -r "8080" cmd/gateway-service/ || echo "Port 8080 not configured"
```

### **Step 3: Build Script Updates**
**File**: `docker/build-all-services.sh`
**Action**: Remove webhook-service from build targets

#### **Changes Required**:
```bash
# REMOVE webhook-service from SERVICES array
SERVICES=(
    "gateway-service"      # ✅ Keep - handles gateway + webhook
    "alert-service"
    "ai-service"
    # ... other services
    # "webhook-service"    # ❌ Remove - merged into gateway
)

# REMOVE webhook-service from port mapping
# No changes needed in get_service_port() function since webhook-service not included
```

### **Step 4: Validation Updates**
**Action**: Update validation to exclude webhook-service

#### **Documentation Updates**:
- Update container registry documentation
- Update deployment guides
- Update architecture documentation

### **Step 5: Cleanup**
**Action**: Archive and remove webhook-service.Dockerfile

#### **Cleanup Steps**:
```bash
# Archive for reference
cp docker/webhook-service.Dockerfile docker/ARCHIVED_webhook-service.Dockerfile.backup

# Add archive note
echo "# ARCHIVED: Merged into gateway-service.Dockerfile on $(date)" >> docker/ARCHIVED_webhook-service.Dockerfile.backup

# Remove original (after verification)
rm docker/webhook-service.Dockerfile
```

---

## 🧪 **TESTING PLAN**

### **Pre-Merge Testing**
1. **Build Test**: Verify current gateway-service builds successfully
2. **Functionality Test**: Confirm gateway service includes webhook processing
3. **Port Test**: Verify no port conflicts exist

### **Post-Merge Testing**
1. **Build Test**: Verify enhanced gateway-service builds successfully
2. **Validation Test**: Run `./docker/build-all-services.sh validate`
3. **Container Test**: Build and run gateway service container
4. **Integration Test**: Test webhook endpoints in gateway service

### **Testing Commands**
```bash
# Pre-merge validation
./docker/build-all-services.sh validate

# Build enhanced gateway service
./docker/build-all-services.sh build gateway-service

# Test container functionality
docker run --rm -p 8080:8080 quay.io/jordigilh/gateway-service:latest --health-check

# Test webhook endpoints (if available)
curl -X POST http://localhost:8080/webhook/prometheus -d '{"test": "data"}'
```

---

## ⚠️ **RISK ASSESSMENT**

### **Low Risk Items** ✅
- **Dockerfile Compatibility**: Both use identical base images and patterns
- **Port Conflicts**: Both services use same ports (8080, 9090, 8081)
- **Security Model**: Both use compatible security approaches
- **Build Process**: Standard Go multi-stage builds

### **Medium Risk Items** ⚠️
- **Code Integration**: Need to verify webhook functionality exists in gateway service
- **Endpoint Availability**: Webhook endpoints must be available in gateway service
- **Configuration**: Gateway service must handle webhook-specific configuration

### **Mitigation Strategies**
1. **Verification First**: Confirm code integration before Dockerfile changes
2. **Incremental Approach**: Test each step before proceeding
3. **Backup Strategy**: Archive webhook-service.Dockerfile before removal
4. **Rollback Plan**: Keep ability to restore webhook-service if needed

---

## 📈 **SUCCESS CRITERIA**

### **Technical Success**
- [ ] Enhanced gateway-service.Dockerfile builds successfully
- [ ] Gateway service container includes webhook processing
- [ ] All validation tests pass
- [ ] No port conflicts or service disruptions
- [ ] Build automation works with merged service

### **Architecture Success**
- [ ] Single gateway service handles all HTTP gateway operations
- [ ] Webhook processing integrated into gateway service
- [ ] Approved architecture compliance achieved
- [ ] Documentation reflects merged service

### **Operational Success**
- [ ] Container registry updated with correct service
- [ ] Deployment manifests use gateway service
- [ ] Monitoring and health checks functional
- [ ] No service disruption during transition

---

## 🎯 **IMPLEMENTATION TIMELINE**

| Phase | Duration | Tasks | Validation |
|-------|----------|-------|------------|
| **Phase 1** | 15 min | Code integration verification | Binary builds, endpoints exist |
| **Phase 2** | 10 min | Enhanced Dockerfile creation | Dockerfile builds successfully |
| **Phase 3** | 10 min | Build script updates | Validation passes |
| **Phase 4** | 10 min | Testing and validation | All tests pass |
| **Phase 5** | 5 min | Cleanup and documentation | Archive complete |

**Total Estimated Time**: 50 minutes

---

## 📋 **CHECKLIST**

### **Pre-Implementation**
- [ ] Verify gateway service includes webhook functionality
- [ ] Confirm no other services depend on webhook-service
- [ ] Backup current webhook-service.Dockerfile
- [ ] Review approved architecture requirements

### **Implementation**
- [ ] Update gateway-service.Dockerfile with enhanced descriptions
- [ ] Update build script to remove webhook-service
- [ ] Test enhanced gateway service builds
- [ ] Validate all services still build correctly

### **Post-Implementation**
- [ ] Archive webhook-service.Dockerfile
- [ ] Update documentation references
- [ ] Verify container registry compliance
- [ ] Test deployment with merged service

### **Validation**
- [ ] Run `./docker/build-all-services.sh validate`
- [ ] Build and test gateway service container
- [ ] Verify webhook endpoints accessible
- [ ] Confirm architecture compliance

---

**Plan Status**: ✅ **READY FOR IMPLEMENTATION**
**Risk Level**: 🟡 **MEDIUM** (requires code verification)
**Estimated Effort**: 50 minutes
**Dependencies**: Gateway service must include webhook processing functionality
