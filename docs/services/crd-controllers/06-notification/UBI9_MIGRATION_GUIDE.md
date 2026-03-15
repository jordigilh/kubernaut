# Notification Controller - Red Hat UBI10 Migration Guide

**Version**: 1.0
**Date**: 2025-10-21
**Based On**: ADR-027 (Multi-Architecture Build Strategy with Red Hat UBI)
**Target Release**: v1.1.0-ubi10
**Priority**: P1 - HIGH (Week 2 of ADR-027 rollout)

---

## 🎯 **MIGRATION OVERVIEW**

This guide details the migration of Notification Controller from alpine/distroless base images to Red Hat UBI10 enterprise images.

### **Why Migrate?**

Per **ADR-027**, all Kubernaut services must use Red Hat UBI10 base images for:
- ✅ Enterprise support and security certifications
- ✅ OpenShift Container Platform optimization
- ✅ Regular security updates (RHSA + CVE tracking)
- ✅ Standardized base across all services
- ✅ Long-term support and predictable lifecycle

---

## 📊 **CURRENT vs TARGET STATE**

| Aspect | Current (v1.0.x) | Target (v1.1.0-ubi10) | Change |
|---|---|---|---|
| **Build Image** | `golang:1.24-alpine` | `registry.access.redhat.com/ubi10/go-toolset:1.24` | ✅ UBI10 Go |
| **Runtime Image** | `gcr.io/distroless/static:nonroot` | `registry.access.redhat.com/ubi10/ubi-minimal:latest` | ✅ UBI10 Minimal |
| **User ID** | 65532 (distroless nonroot) | 1001 (UBI10 standard) | ⚠️ Breaking |
| **Image Size** | ~42MB | ~200-250MB | ⚠️ +150-200MB |
| **Package Manager** | apk (Alpine) | dnf/microdnf (Red Hat) | ✅ Enterprise |
| **Labels** | None | 13 Red Hat labels | ✅ Metadata |
| **Multi-Arch** | Implied | Explicit (ADR-027) | ✅ Documented |

---

## 🔧 **MIGRATION STEPS**

### **Step 1: Build UBI10 Image** (30 minutes)

```bash
# Navigate to repo root
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Build multi-architecture image with UBI10 (ADR-027 compliant)
podman build --platform linux/amd64,linux/arm64 \
  -t quay.io/jordigilh/notification:v1.1.0-ubi10 \
  -f docker/notification-controller.Dockerfile .

# Verify multi-arch manifest
podman manifest inspect quay.io/jordigilh/notification:v1.1.0-ubi10

# Expected output:
# - Manifests for amd64 and arm64
# - Both using UBI10 base layers
```

---

### **Step 2: Test Locally** (30 minutes)

#### **Test on arm64 (Mac Development)**

```bash
# Run container locally
podman run -d --rm \
  --name notification-test \
  -p 8080:8080 \
  -p 8081:8081 \
  -e KUBECONFIG=/path/to/kubeconfig \
  quay.io/jordigilh/notification:v1.1.0-ubi10

# Check health
curl http://localhost:8081/healthz
# Expected: {"status":"healthy"}

# Check metrics
curl http://localhost:8081/metrics | grep notification_

# Stop container
podman stop notification-test
```

#### **Test amd64 Compatibility** (via manifest inspection)

```bash
# Verify amd64 image exists in manifest
podman manifest inspect quay.io/jordigilh/notification:v1.1.0-ubi10 \
  | jq '.manifests[] | select(.platform.architecture == "amd64")'

# Expected: Non-empty output with amd64 platform
```

---

### **Step 3: Update Deployment Manifests** (15 minutes)

#### **Update Image Reference**

**File**: `deploy/notification/deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: notification-controller
  namespace: kubernaut-notifications
spec:
  template:
    spec:
      containers:
      - name: manager
        # OLD: quay.io/jordigilh/notification:v1.0.1
        image: quay.io/jordigilh/notification:v1.1.0-ubi10
        ports:
        - containerPort: 8080
          name: controller
        - containerPort: 8081
          name: health
        securityContext:
          # IMPORTANT: Update UID from 65532 to 1001 (UBI10 standard)
          runAsUser: 1001
          runAsNonRoot: true
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
```

#### **Verify Security Context**

The security context change is **critical**:

- **OLD (distroless)**: `runAsUser: 65532`
- **NEW (UBI10)**: `runAsUser: 1001`

If using PodSecurityPolicy or SecurityContextConstraints, verify UID 1001 is allowed.

---

### **Step 4: Deploy to Dev Cluster** (30 minutes)

```bash
# Push image to registry
podman manifest push quay.io/jordigilh/notification:v1.1.0-ubi10 \
  docker://quay.io/jordigilh/notification:v1.1.0-ubi10

# Update deployment manifest (see Step 3)
vim deploy/notification/deployment.yaml

# Apply to dev cluster
kubectl apply -f deploy/notification/

# Watch rollout
kubectl rollout status deployment/notification-controller \
  -n kubernaut-notifications

# Verify pods are running
kubectl get pods -n kubernaut-notifications

# Check logs
kubectl logs -f deployment/notification-controller \
  -n kubernaut-notifications
```

---

### **Step 5: Validate Functionality** (30 minutes)

#### **Create Test NotificationRequest**

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: ubi10-migration-test
  namespace: kubernaut-notifications
spec:
  type: simple
  priority: medium
  subject: "UBI10 Migration Test"
  body: "Testing notification controller after UBI10 migration"
  channels:
    - console
  recipients:
    - {} # Console needs no recipient
```

```bash
# Apply test notification
kubectl apply -f test-notification.yaml

# Watch status
kubectl get notificationrequest ubi10-migration-test \
  -n kubernaut-notifications \
  -o jsonpath='{.status.phase}'

# Expected: "Sent"

# Check delivery attempts
kubectl get notificationrequest ubi10-migration-test \
  -n kubernaut-notifications \
  -o jsonpath='{.status.deliveryAttempts}'

# Verify logs show successful delivery
kubectl logs deployment/notification-controller \
  -n kubernaut-notifications \
  | grep "ubi10-migration-test"
```

#### **Validation Checklist**

```bash
✅ Container starts successfully
✅ Health checks pass (/healthz endpoint)
✅ Reconciliation loop functions correctly
✅ NotificationRequests are processed
✅ Console delivery works
✅ Status updates persist
✅ Metrics endpoint operational
✅ Multi-arch manifest present (amd64 + arm64)
✅ Image size acceptable (<100MB increase)
```

---

### **Step 6: Performance Comparison** (15 minutes)

#### **Image Size Comparison**

```bash
# Alpine/distroless (v1.0.1)
podman images quay.io/jordigilh/notification:v1.0.1
# Expected: ~42MB

# UBI10 (v1.1.0-ubi10)
podman images quay.io/jordigilh/notification:v1.1.0-ubi10
# Expected: ~200-250MB (delta: +150-200MB)

# Impact Assessment:
# - Storage cost: ~$0.02/GB/month (S3/Quay) = ~$0.004/month per image
# - Pull time: +2-3 seconds on first pull (layers cached after)
# - Enterprise benefits: Red Hat support, security updates, compliance
```

#### **Runtime Performance**

```bash
# Compare startup times
# Alpine/distroless: ~2-3 seconds
# UBI10: ~3-4 seconds (negligible difference)

# Memory usage should be identical (both run same Go binary)
kubectl top pod -n kubernaut-notifications
```

---

## 🚨 **BREAKING CHANGES**

### **User ID Change** (Critical)

**Before (distroless)**:
```dockerfile
USER 65532:65532
```

**After (UBI10)**:
```dockerfile
USER 1001
```

**Impact**:
- ⚠️ If PersistentVolumes are used, file ownership may need adjustment
- ⚠️ If PodSecurityPolicy/SCC restricts UIDs, policy must allow 1001
- ⚠️ Custom RBAC policies using UID-based rules need updates

**Mitigation**:
```bash
# For PersistentVolumes (if used):
kubectl exec -it <pod> -- chown -R 1001:0 /data

# For OpenShift SCC (if restricted):
oc adm policy add-scc-to-user anyuid \
  system:serviceaccount:kubernaut-notifications:notification-controller
```

---

## 🔄 **ROLLBACK PLAN**

If issues occur with UBI10 migration:

### **Immediate Rollback** (<5 minutes)

```bash
# Revert to previous version
kubectl set image deployment/notification-controller \
  manager=quay.io/jordigilh/notification:v1.0.1 \
  -n kubernaut-notifications

# Revert security context (if changed)
kubectl patch deployment notification-controller \
  -n kubernaut-notifications \
  -p '{"spec":{"template":{"spec":{"securityContext":{"runAsUser":65532}}}}}'

# Verify rollback
kubectl rollout status deployment/notification-controller \
  -n kubernaut-notifications
```

### **Root Cause Analysis**

Common issues and solutions:

| Issue | Cause | Solution |
|---|---|---|
| **Pods CrashLoopBackOff** | UID 1001 not allowed by SCC | Add SCC policy or use `runAsUser: null` |
| **Permission Denied** | PV file ownership mismatch | `chown -R 1001:0` on volume |
| **Image Pull Error** | Multi-arch manifest issue | Build single-arch for testing |
| **Health Check Failing** | UBI10 missing dependencies | Verify `microdnf install ca-certificates tzdata` |

---

## 📋 **POST-MIGRATION TASKS**

### **Update Documentation** (15 minutes)

1. **Update Deployment Guides**:
   - `deploy/notification/README.md` - Reference UBI10 image
   - Update image tags in all examples

2. **Update Makefile**:
   ```makefile
   NOTIFICATION_IMG ?= quay.io/jordigilh/notification:v1.1.0-ubi10

   .PHONY: docker-build-notification
   docker-build-notification:
   	@echo "🔨 Building multi-architecture UBI10 image..."
   	podman build --platform linux/amd64,linux/arm64 \
   		-t $(NOTIFICATION_IMG) \
   		-f docker/notification-controller.Dockerfile .
   ```

3. **Archive Old Dockerfile**:
   ```bash
   # Rename old Dockerfile
   mv docker/notification-controller.Dockerfile \
      docker/notification-controller-legacy-alpine.Dockerfile

   # Rename UBI10 Dockerfile to standard name
   mv docker/notification-controller.Dockerfile \
      docker/notification-controller.Dockerfile
   ```

4. **Update CI/CD Pipelines** (if applicable):
   - Change build commands to use new Dockerfile
   - Update image tags
   - Verify multi-arch builds work in CI

---

## 📊 **MIGRATION CHECKLIST**

### **Pre-Migration**

- [ ] ✅ ADR-027 reviewed and understood
- [ ] ✅ UBI10 Dockerfile created (`docker/notification-controller.Dockerfile`)
- [ ] ✅ Local podman build tested
- [ ] ✅ Multi-arch manifest verified
- [ ] ✅ Backup current production deployment

### **Migration**

- [ ] ⏳ Build UBI10 image with multi-arch support
- [ ] ⏳ Push to quay.io registry
- [ ] ⏳ Update deployment manifests (image + UID)
- [ ] ⏳ Deploy to dev cluster
- [ ] ⏳ Run validation tests
- [ ] ⏳ Monitor for 24 hours in dev

### **Post-Migration**

- [ ] ⏳ Performance comparison documented
- [ ] ⏳ Update all deployment guides
- [ ] ⏳ Update Makefile targets
- [ ] ⏳ Archive old alpine Dockerfile
- [ ] ⏳ Update CI/CD pipelines
- [ ] ⏳ Promote to staging (after dev validation)
- [ ] ⏳ Promote to production (after staging validation)

---

## 🎯 **SUCCESS CRITERIA**

### **Technical**

- ✅ Multi-arch image contains both amd64 and arm64 manifests
- ✅ Image uses Red Hat UBI10 base images (verified with `podman inspect`)
- ✅ Image includes all 13 required Red Hat labels
- ✅ Container runs as UID 1001 (UBI10 standard)
- ✅ Image size acceptable (<300MB total)
- ✅ No hardcoded configuration files in image

### **Functional**

- ✅ Reconciliation loop processes NotificationRequests
- ✅ Console delivery functional
- ✅ Slack delivery functional (if configured)
- ✅ Health checks pass on both architectures
- ✅ Metrics endpoint operational
- ✅ Exponential backoff retry working
- ✅ Status updates persist correctly

### **Operational**

- ✅ Zero deployment failures on dev cluster
- ✅ No increase in error rates
- ✅ No performance degradation
- ✅ Rollback plan validated
- ✅ Documentation updated
- ✅ Team trained on new image structure

---

## 📚 **REFERENCES**

### **Internal Documentation**

- [ADR-027: Multi-Architecture Build Strategy with Red Hat UBI](../../../architecture/decisions/ADR-027-multi-architecture-build-strategy.md)
- [Notification Implementation Plan v3.2](./implementation/IMPLEMENTATION_PLAN_V3.0.md)
- [Container Registry Standards](../../../deployment/CONTAINER_REGISTRY.md)

### **External Resources**

- [Red Hat UBI10 Go Toolset](https://catalog.redhat.com/software/containers/ubi10/go-toolset/615aee9fc739c0a4123a87e1)
- [UBI10 Minimal Image](https://catalog.redhat.com/software/containers/ubi10/ubi-minimal/615bd9b4075b022acc111bf5)
- [Podman Multi-Architecture Builds](https://podman.io/blogs/2021/10/11/multiarch.html)

---

## 🆘 **SUPPORT**

### **Issues or Questions**

- **Technical Issues**: Check rollback plan above
- **Architecture Questions**: Review ADR-027
- **Build Problems**: Verify podman version ≥4.0

### **Migration Timeline**

- **Week 2 of ADR-027 Rollout**: Notification Controller (Priority 1)
- **Effort**: 2-3 hours total
- **Validation**: 24 hours in dev before promoting

---

**Migration Prepared**: 2025-10-21
**Migration Owner**: Platform Team
**Priority**: P1 - HIGH (Week 2 Pilot Service for UBI10)

