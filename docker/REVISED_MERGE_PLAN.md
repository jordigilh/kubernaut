# REVISED Gateway & Webhook Merge Plan

**Date**: September 27, 2025
**Status**: **CRITICAL DISCOVERY - PLAN UPDATED**
**Discovery**: No `cmd/gateway-service/` implementation exists, only `cmd/webhook-service/`

---

## 🚨 **CRITICAL DISCOVERY**

### **Current Reality**
```
EXPECTED (Architecture):           ACTUAL (Implementation):
┌─────────────────┐               ┌─────────────────┐
│ gateway-service │  ❌           │ webhook-service │  ✅ EXISTS
│ (Port 8080)     │  NOT FOUND    │ (Port 8080)     │  IMPLEMENTED
└─────────────────┘               └─────────────────┘
```

### **Implementation Analysis**
| Service | Status | Implementation | Dockerfile | Architecture Role |
|---------|--------|---------------|------------|-------------------|
| **webhook-service** | ✅ **EXISTS** | Full HTTP server with routing | ✅ **EXISTS** | Should be gateway |
| **gateway-service** | ❌ **MISSING** | No implementation | ✅ **Created** | Approved architecture |

---

## 🎯 **REVISED STRATEGY**

### **RECOMMENDED: Option A - Rename webhook-service to gateway-service**

**Approach**: Transform existing webhook-service into gateway-service to align with approved architecture

#### **Why This Approach**:
1. **Preserves Working Code**: webhook-service is fully implemented and functional
2. **Architecture Alignment**: Aligns implementation with approved microservices architecture
3. **Minimal Risk**: Rename operation with no functional changes
4. **Single Responsibility**: Gateway service handles all HTTP gateway operations including webhooks

---

## 📋 **IMPLEMENTATION PLAN**

### **Phase 1: Code Rename (cmd directory)**
**Action**: Rename webhook-service to gateway-service in implementation

#### **Steps**:
```bash
# 1. Rename directory
mv cmd/webhook-service cmd/gateway-service

# 2. Update binary name in main.go
sed -i 's/webhook-service/gateway-service/g' cmd/gateway-service/main.go

# 3. Update service name in logs and metrics
sed -i 's/"service": "webhook-service"/"service": "gateway-service"/g' cmd/gateway-service/main.go

# 4. Update package documentation
# Update comments to reflect gateway service role
```

### **Phase 2: Dockerfile Updates**
**Action**: Update webhook-service.Dockerfile to become gateway-service.Dockerfile

#### **Changes Required**:
```dockerfile
# CHANGE: Build command
FROM:
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -o webhook-service \
    ./cmd/webhook-service

TO:
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -o gateway-service \
    ./cmd/gateway-service

# CHANGE: Binary copy and permissions
FROM:
COPY --from=builder /opt/app-root/src/webhook-service /usr/local/bin/webhook-service
RUN chmod +x /usr/local/bin/webhook-service

TO:
COPY --from=builder /opt/app-root/src/gateway-service /usr/local/bin/gateway-service
RUN chmod +x /usr/local/bin/gateway-service

# CHANGE: User name
FROM: webhook-user
TO: gateway-user

# CHANGE: Entrypoint
FROM: ENTRYPOINT ["/usr/local/bin/webhook-service"]
TO: ENTRYPOINT ["/usr/local/bin/gateway-service"]

# CHANGE: Health check
FROM: CMD ["/usr/local/bin/webhook-service", "--health-check"]
TO: CMD ["/usr/local/bin/gateway-service", "--health-check"]

# CHANGE: Labels (already correct in gateway-service.Dockerfile)
```

### **Phase 3: File Management**
**Action**: Rename and clean up Dockerfiles

#### **Steps**:
```bash
# 1. Remove the created gateway-service.Dockerfile (no implementation behind it)
rm docker/gateway-service.Dockerfile

# 2. Update webhook-service.Dockerfile to become gateway-service.Dockerfile
mv docker/webhook-service.Dockerfile docker/gateway-service.Dockerfile

# 3. Update the renamed Dockerfile with correct paths and names
```

### **Phase 4: Build Script Updates**
**Action**: Update build automation

#### **Changes**:
```bash
# Update SERVICES array in docker/build-all-services.sh
FROM:
SERVICES=(
    "gateway-service"    # ❌ No implementation
    "webhook-service"    # ✅ Has implementation
    ...
)

TO:
SERVICES=(
    "gateway-service"    # ✅ Renamed from webhook-service
    ...
    # Remove webhook-service entry
)
```

---

## 🔧 **DETAILED IMPLEMENTATION STEPS**

### **Step 1: Rename Implementation**
```bash
# Navigate to project root
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Rename the service directory
mv cmd/webhook-service cmd/gateway-service

# Update main.go service name
sed -i '' 's/"service": "webhook-service"/"service": "gateway-service"/g' cmd/gateway-service/main.go

# Update any other references
grep -r "webhook-service" cmd/gateway-service/ && echo "Manual updates needed"
```

### **Step 2: Update Dockerfile**
```bash
# Remove the incorrect gateway-service.Dockerfile
rm docker/gateway-service.Dockerfile

# Rename webhook-service.Dockerfile
mv docker/webhook-service.Dockerfile docker/gateway-service.Dockerfile

# Update the Dockerfile content (see detailed changes above)
```

### **Step 3: Update Build Script**
```bash
# The build script already expects gateway-service, so it will work after rename
# Verify with:
./docker/build-all-services.sh validate
```

### **Step 4: Test the Changes**
```bash
# Test that gateway-service builds
go build ./cmd/gateway-service

# Test Dockerfile builds
docker build -f docker/gateway-service.Dockerfile -t test-gateway .

# Test validation passes
./docker/build-all-services.sh validate
```

---

## ⚠️ **RISK ASSESSMENT**

### **Low Risk** ✅
- **File Rename**: Simple directory and file rename operations
- **Dockerfile Update**: Straightforward path and name changes
- **Build Script**: Already configured for gateway-service

### **Medium Risk** ⚠️
- **Service References**: May need to update configuration files
- **Integration Points**: Other services may reference webhook-service
- **Documentation**: Need to update all references

### **Mitigation**
1. **Search and Replace**: Comprehensive search for all webhook-service references
2. **Testing**: Test build and basic functionality after each step
3. **Rollback**: Keep backup of original files

---

## 🧪 **TESTING PLAN**

### **Pre-Implementation Tests**
```bash
# Verify current webhook-service works
go build ./cmd/webhook-service
./docker/build-all-services.sh validate

# Check for references to webhook-service
grep -r "webhook-service" . --exclude-dir=.git --exclude-dir=vendor
```

### **Post-Implementation Tests**
```bash
# Verify gateway-service builds
go build ./cmd/gateway-service

# Verify Dockerfile builds
./docker/build-all-services.sh build gateway-service

# Verify validation passes
./docker/build-all-services.sh validate

# Test container runs
docker run --rm quay.io/jordigilh/gateway-service:latest --health-check
```

---

## 📋 **EXECUTION CHECKLIST**

### **Pre-Execution**
- [ ] Backup current webhook-service directory
- [ ] Backup current webhook-service.Dockerfile
- [ ] Verify webhook-service builds and runs
- [ ] Document current configuration

### **Execution**
- [ ] Rename cmd/webhook-service to cmd/gateway-service
- [ ] Update service name in main.go
- [ ] Remove incorrect gateway-service.Dockerfile
- [ ] Rename webhook-service.Dockerfile to gateway-service.Dockerfile
- [ ] Update Dockerfile paths and names
- [ ] Test build and validation

### **Post-Execution**
- [ ] Verify gateway-service builds successfully
- [ ] Verify Dockerfile builds container
- [ ] Run validation tests
- [ ] Update documentation references
- [ ] Test container functionality

---

## 🎯 **SUCCESS CRITERIA**

### **Technical Success**
- [ ] `cmd/gateway-service/` exists and builds
- [ ] `docker/gateway-service.Dockerfile` builds successfully
- [ ] Build validation passes for gateway-service
- [ ] No references to webhook-service remain
- [ ] Container runs and responds to health checks

### **Architecture Success**
- [ ] Implementation aligns with approved architecture
- [ ] Gateway service handles HTTP gateway operations
- [ ] Single service owns port 8080
- [ ] Service naming consistent with architecture

---

**Revised Plan Status**: ✅ **READY FOR IMPLEMENTATION**
**Risk Level**: 🟡 **MEDIUM** (rename operations)
**Estimated Time**: 30 minutes
**Key Change**: Rename existing webhook-service to gateway-service instead of merging
