# DD-AUTH-011 Quick Start Guide

**Date**: January 26, 2026
**Status**: ✅ **APPROVED** - Ready to Execute
**Estimated Time**: 3.5 hours

---

## 🎯 **WHAT WE'RE DOING**

**Single RBAC ClusterRole with `verb:"create"` + Audit Tracking**

### **Key Changes**

1. **RBAC**: `data-storage-client` ClusterRole grants `verbs: ["create"]` (was `["get"]`)
2. **OAuth2-Proxy**: SAR checks `verb:"create"` (was `verb:"get"`)
3. **E2E Tests**: Use real ServiceAccount tokens (was pass-through/mock)
4. **Audit Tracking**: Workflow access logged (already implemented)

### **Why**

- **All services need `create`** for audit write operations
- **OAuth2-proxy limitation**: Only one global SAR check
- **User mandate**: "Use audit traces to track who interacts with these endpoints"

---

## ⚡ **QUICK EXECUTION STEPS**

### **Step 1: Backup Current RBAC** (2 min)

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

mkdir -p backup

kubectl get clusterrole data-storage-client -o yaml > backup/data-storage-client-backup-$(date +%Y%m%d).yaml

kubectl get rolebindings -n kubernaut-system -l component=rbac -o yaml > backup/data-storage-rolebindings-backup-$(date +%Y%m%d).yaml
```

---

### **Step 2: Deploy New RBAC** (5 min)

```bash
# Apply new RBAC configuration (DD-AUTH-011)
kubectl apply -f deploy/data-storage/client-rbac-v2.yaml

# Verify ClusterRole
kubectl get clusterrole data-storage-client -o jsonpath='{.rules[0].verbs}'
# Expected: ["create"]

# Verify RoleBindings count
kubectl get rolebindings -n kubernaut-system -l component=rbac | grep data-storage-client | wc -l
# Expected: 14 (8 services + 6 E2E)
```

---

### **Step 3: Update OAuth2-Proxy SAR** (5 min)

**Edit**: `deploy/data-storage/deployment.yaml` (around line 61)

```yaml
# Change this line:
- --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"get"}

# To:
- --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}
```

```bash
# Apply deployment
kubectl apply -f deploy/data-storage/deployment.yaml

# Watch rollout
kubectl rollout status deployment/data-storage-service -n kubernaut-system

# Verify SAR argument
kubectl get pods -n kubernaut-system -l app=data-storage-service \
  -o jsonpath='{.items[0].spec.containers[?(@.name=="oauth-proxy")].args}' | grep create
# Expected: Contains verb":"create"
```

---

### **Step 4: Test with Existing Services** (10 min)

```bash
# Test that services can still access DataStorage
# (They should all pass now with "create" permission)

# Check Gateway can write audit
kubectl logs -n kubernaut-system deployment/gateway --tail=50 | grep "audit"

# Check oauth-proxy logs (no 403 errors)
kubectl logs -n kubernaut-system deployment/data-storage-service -c oauth-proxy --tail=50
# Expected: No "403 Forbidden" or "SAR check failed"
```

---

### **Step 5: Implement E2E Real Auth** (2.5 hours)

**See**: `DD-AUTH-011-IMPLEMENTATION-PLAN.md` Phase 3 for detailed steps

**Quick Summary**:
1. Create `test/shared/auth/serviceaccount_transport.go`
2. Create `test/infrastructure/serviceaccount.go`
3. Update `test/infrastructure/datastorage.go` (remove pass-through)
4. Update `test/e2e/datastorage/datastorage_e2e_suite_test.go`

---

### **Step 6: Run E2E Tests** (30 min)

```bash
# Run DataStorage E2E tests
make test-e2e-datastorage

# Expected:
# ✅ E2E ServiceAccount created
# ✅ Real token authentication
# ✅ OAuth2-proxy SAR passes
# ✅ All tests pass
```

---

## 🚨 **ROLLBACK (If Needed)**

```bash
# Restore previous RBAC
kubectl apply -f backup/data-storage-client-backup-YYYYMMDD.yaml
kubectl apply -f backup/data-storage-rolebindings-backup-YYYYMMDD.yaml

# Revert deployment
git checkout HEAD~1 -- deploy/data-storage/deployment.yaml
kubectl apply -f deploy/data-storage/deployment.yaml
```

---

## ✅ **SUCCESS CHECKLIST**

After completion, verify:

- [ ] ClusterRole has `verbs: ["create"]`
- [ ] All 8 services have RoleBindings
- [ ] OAuth2-proxy uses `verb:"create"` in SAR
- [ ] No 403 errors in service logs
- [ ] E2E tests pass with real auth
- [ ] Audit logs capture workflow operations

---

## 📚 **REFERENCE DOCUMENTS**

- **DD-AUTH-011**: Full design decision (16-page analysis)
- **DD-AUTH-011-SUMMARY**: Executive summary (4 pages)
- **DD-AUTH-011-IMPLEMENTATION-PLAN**: Detailed implementation (full phases)
- **DD-AUTH-010**: E2E real authentication mandate
- **DD-AUTH-009**: OAuth2-proxy migration plan

---

## 🎯 **KEY FILES**

### **New Files Created**
- `deploy/data-storage/client-rbac-v2.yaml` - New RBAC configuration
- `test/shared/auth/serviceaccount_transport.go` - Real token transport
- `test/infrastructure/serviceaccount.go` - E2E ServiceAccount helpers

### **Files to Edit**
- `deploy/data-storage/deployment.yaml` - OAuth2-proxy SAR (line 61)
- `test/infrastructure/datastorage.go` - Remove pass-through mode
- `test/e2e/datastorage/datastorage_e2e_suite_test.go` - Real auth

---

## 💡 **IMPORTANT NOTES**

1. **Notification service is in different namespace**: `kubernaut-notifications` (not `kubernaut-system`). The RoleBinding correctly references this cross-namespace access.

2. **All services already need `create`** for audit writes - This isn't granting new permissions, just aligning RBAC with actual usage

2. **Audit logs track workflow access** - Security team can query:
   ```bash
   # Who accessed workflow endpoints?
   kubectl port-forward -n kubernaut-system svc/data-storage-service 8080:8080
   curl -H "Authorization: Bearer $(kubectl create token datastorage-e2e-sa -n kubernaut-system)" \
     http://localhost:8080/api/v1/audit?event_type=workflow.searched&limit=100
   ```

4. **OAuth2-proxy limitation is upstream** - No per-endpoint SAR support, confirmed in official docs

5. **E2E tests use production RBAC** - Tests validate actual security boundaries

---

**Ready to execute Phase 1 (Backup + Deploy RBAC)?**
