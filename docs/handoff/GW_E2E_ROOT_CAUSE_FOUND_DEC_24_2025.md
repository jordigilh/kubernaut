# Gateway E2E Coverage - ROOT CAUSE IDENTIFIED (Dec 24, 2025)

## 🎯 **Critical Finding**

**ROOT CAUSE**: Image name prefix mismatch between Kind cluster and Kubernetes manifest

### **The Problem**
When images are loaded into Kind using `kind load image-archive`, they **retain the `localhost/` prefix**:
```bash
# After loading with: kind load image-archive /tmp/gateway.tar
podman exec gateway-debug-worker crictl images | grep gateway
localhost/kubernaut/gateway    debug    abc123    150MB
```

**However**, our code was stripping the `localhost/` prefix before deploying to Kubernetes:
```go
// gateway_e2e.go:351 (INCORRECT FIX)
k8sImageName := strings.TrimPrefix(gatewayImageName, "localhost/")
DeployGatewayCoverageManifest(k8sImageName, kubeconfigPath, writer)
```

This caused the Gateway manifest to reference `kubernaut/gateway:tag` but Kind only has `localhost/kubernaut/gateway:tag`, resulting in:
- ✅ Deployment created successfully
- ❌ Pod stuck in `ImagePullBackOff` or `ErrImageNeverPull`
- ❌ Health check timeout (pod never starts)

### **Verification**
Tested with DataStorage in debug cluster:
```bash
# DataStorage manifest with WRONG image name:
image: kubernaut/datastorage:debug
Result: ErrImageNeverPull ❌

# DataStorage patched with CORRECT image name:
kubectl patch deployment datastorage -p '[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value": "localhost/kubernaut/datastorage:debug"}]'
Result: Pod started successfully ✅
```

## 🔧 **The Fix**

### **REMOVE the localhost/ prefix stripping** in `gateway_e2e.go`:

```go
// gateway_e2e.go:347-355 (BEFORE - WRONG)
fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying Gateway (coverage-enabled)...")

// Strip "localhost/" prefix from image name for Kubernetes manifest
// After loading into Kind, images are available without the localhost/ prefix
k8sImageName := strings.TrimPrefix(gatewayImageName, "localhost/")

// Deploy Gateway with coverage manifest (includes GOCOVERDIR and /coverdata mount)
if err := DeployGatewayCoverageManifest(k8sImageName, kubeconfigPath, writer); err != nil {
	return fmt.Errorf("failed to deploy Gateway with coverage: %w", err)
}
```

```go
// gateway_e2e.go:347-352 (AFTER - CORRECT)
fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying Gateway (coverage-enabled)...")

// Deploy Gateway with coverage manifest (includes GOCOVERDIR and /coverdata mount)
// Use gatewayImageName directly - Kind retains the localhost/ prefix
if err := DeployGatewayCoverageManifest(gatewayImageName, kubeconfigPath, writer); err != nil {
	return fmt.Errorf("failed to deploy Gateway with coverage: %w", err)
}
```

## 📊 **Why This Was Hard to Debug**

1. **Auto-cleanup**: E2E tests delete Kind cluster immediately after failure, preventing pod inspection
2. **Misleading error**: "timeout waiting for Gateway health check" suggested networking/config issue, not image pull failure
3. **Success messages**: Build/load/deploy all reported success, masking the underlying ImagePullBackOff
4. **Inconsistent behavior**: Some services work because they use different image loading methods

## 🔍 **How We Found It**

Created persistent Kind cluster (`gateway-debug`) that persisted after failure:
```bash
# 1. Created cluster
kind create cluster --name gateway-debug --config test/infrastructure/kind-gateway-config.yaml

# 2. Loaded image
podman save -o /tmp/datastorage.tar localhost/kubernaut/datastorage:debug
kind load image-archive /tmp/datastorage.tar --name gateway-debug

# 3. Inspected what's actually in Kind
podman exec gateway-debug-worker crictl images | grep datastorage
localhost/kubernaut/datastorage    debug    46dfa7b54c5ed    151MB
                                   ^^^^^^^^ CRITICAL: localhost/ prefix retained!

# 4. Deployed with wrong name → Failed
image: kubernaut/datastorage:debug
Result: ErrImageNeverPull

# 5. Patched with correct name → Success!
image: localhost/kubernaut/datastorage:debug
Result: Pod started
```

## ✅ **Next Steps**

1. **Revert the incorrect fix** in `gateway_e2e.go` (remove `strings.TrimPrefix`)
2. **Test with simplified setup** (single service, persistent cluster)
3. **Verify Gateway health check succeeds** with correct image name
4. **Run full E2E suite** to validate coverage collection
5. **Update documentation** to clarify `localhost/` prefix behavior

## 📝 **Key Learnings**

### **Kind + Podman Image Naming Rules**
1. **Build**: Always use `localhost/` prefix for podman builds
2. **Export**: Use `podman save` with the `localhost/` prefix
3. **Load**: Use `kind load image-archive` (podman compatible)
4. **Deploy**: Use **exact same name** as what's in Kind (keep `localhost/`)

### **Debugging Strategy**
- Always create persistent clusters for debugging
- Inspect actual images in Kind worker nodes: `podman exec <node> crictl images`
- Check pod events: `kubectl describe pod` shows ImagePullBackOff
- Don't trust "deployment created" - verify pod status

## 🎯 **Success Probability**

**Confidence: 95%** that this fix will resolve the Gateway health check timeout.

**Evidence**:
- Same issue reproduced with DataStorage ✅
- Fix verified with DataStorage ✅
- Root cause identified and understood ✅
- Solution is simple one-line change ✅

---

**File**: `test/infrastructure/gateway_e2e.go`
**Line**: 351
**Change**: Remove `k8sImageName := strings.TrimPrefix(gatewayImageName, "localhost/")`
**Use**: `gatewayImageName` directly in `DeployGatewayCoverageManifest()`

**Status**: Ready to implement and test
**Next**: Revert the incorrect fix and run E2E tests








