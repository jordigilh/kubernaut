# Gap #8 E2E Webhook Test Issue - January 13, 2026

## 🔍 **Issue Summary**

**Status**: ⚠️ **E2E Test Failing** - Webhook audit event not being emitted  
**Test**: `test/e2e/authwebhook/02_gap8_remediationrequest_timeout_mutation_test.go`  
**Error**: Timeout waiting for `webhook.remediationrequest.timeout_modified` audit event (expected 1, got 0)

---

## 📋 **What Works**

✅ **RemediationRequest Creation**: CRD created successfully  
✅ **TimeoutConfig Initialization**: Manually set via Status().Update()  
✅ **Operator Mutation**: Status update submitted without errors  
✅ **Test Infrastructure**: Kind cluster, webhook server, DataStorage all running  
✅ **Integration Tests**: 47/47 passing (100%)  

---

## ❌ **What Doesn't Work**

**Webhook Audit Event Not Emitted**: After 30 seconds, no audit event found in DataStorage

```
Expected <int>: 0
to equal <int>: 1
```

**Query**: `correlation_id=<uuid>`, `event_type=webhook.remediationrequest.timeout_modified`  
**Result**: 0 events found

---

## 🔬 **Root Cause Investigation Needed**

### **Hypothesis 1: Webhook Not Intercept**ing

**Symptom**: Webhook server running but not intercepting RemediationRequest status updates

**Possible Causes**:
1. MutatingWebhookConfiguration not correctly registered for RemediationRequest
2. Namespace label `kubernaut.ai/audit-enabled=true` not being respected
3. Webhook path `/mutate-remediationrequest` not registered
4. TLS certificate issues preventing webhook communication

**Investigation Steps**:
```bash
# Check webhook configuration
kubectl get mutatingwebhookconfigurations authwebhook-mutating -o yaml

# Check webhook server logs
kubectl logs -n authwebhook-e2e -l app=authwebhook --tail=100

# Verify namespace label
kubectl get namespace gap8-webhook-test-XXXXXX -o yaml | grep audit-enabled

# Test webhook endpoint manually
kubectl exec -it <test-pod> -- curl -k https://authwebhook.authwebhook-e2e.svc:9443/healthz
```

---

### **Hypothesis 2: Webhook Not Emitting Audit Event**

**Symptom**: Webhook intercepts update but doesn't emit audit event

**Possible Causes**:
1. Audit store not configured in webhook handler
2. `pkg/webhooks/remediationrequest_handler.go` not calling `auditStore.StoreAudit()`
3. Audit event payload construction error
4. Webhook handler returning error before audit emission

**Investigation Steps**:
```bash
# Check webhook handler audit store initialization
grep -A 10 "NewRemediationRequestStatusHandler" cmd/webhooks/main.go

# Verify audit event construction
grep -A 20 "StoreAudit" pkg/webhooks/remediationrequest_handler.go

# Check for webhook errors
kubectl logs -n authwebhook-e2e -l app=authwebhook | grep -i error
```

---

### **Hypothesis 3: Audit Event Not Being Stored**

**Symptom**: Webhook emits event but DataStorage doesn't store it

**Possible Causes**:
1. Webhook → DataStorage network connectivity issues
2. DataStorage batch endpoint not receiving events
3. PostgreSQL write failure
4. Audit buffer not flushing

**Investigation Steps**:
```bash
# Check DataStorage logs for incoming requests
kubectl logs -n authwebhook-e2e -l app=datastorage | grep webhook

# Check PostgreSQL for webhook events
kubectl exec -it postgres-0 -- psql -U slm_user -d action_history \
  -c "SELECT COUNT(*) FROM audit_events WHERE event_type = 'webhook.remediationrequest.timeout_modified';"

# Query DataStorage API directly
curl "http://localhost:28099/api/v1/audit/events?event_type=webhook.remediationrequest.timeout_modified"
```

---

## 📊 **Test Execution Timeline**

| Phase | Duration | Status | Notes |
|-------|----------|--------|-------|
| **Cluster Setup** | ~2min | ✅ Success | Kind + webhook + DataStorage |
| **RR Creation** | ~1s | ✅ Success | Valid fingerprint |
| **TimeoutConfig Init** | ~1s | ✅ Success | Manual initialization |
| **Operator Mutation** | ~1s | ✅ Success | Status().Update() succeeded |
| **Webhook Interception** | ❓ Unknown | ⚠️ **Issue** | No evidence of interception |
| **Audit Event Query** | 30s | ❌ Timeout | 0 events found |

---

## 🛠️ **Attempted Fixes**

### **Fix #1: Valid SHA-256 Fingerprint** ✅
**Problem**: SignalFingerprint validation error  
**Solution**: Use valid 64-char hex string  
**Result**: ✅ RemediationRequest created successfully

### **Fix #2: Manual TimeoutConfig Initialization** ✅
**Problem**: RemediationOrchestrator controller not running in AuthWebhook E2E suite  
**Solution**: Manually initialize TimeoutConfig in test  
**Result**: ✅ Test proceeds to webhook validation

### **Fix #3: Audit Query Integration** ✅
**Problem**: Test had TODO placeholders  
**Solution**: Integrate `helpers.QueryAuditEvents()`  
**Result**: ✅ Query executes (but finds 0 events)

---

## 📝 **Comparison with Working Tests**

###** E2E-MULTI-01 & E2E-MULTI-02** (Passing):

| Feature | E2E-MULTI-01/02 | E2E-GAP8-01 |
|---------|-----------------|-------------|
| **CRD Type** | WorkflowExecution, RAR, NR | RemediationRequest |
| **Webhook Path** | `/mutate-workflowexecution` | `/mutate-remediationrequest` |
| **Audit Event** | `workflowexecution.block.cleared` | `webhook.remediationrequest.timeout_modified` |
| **Status** | ✅ Passing | ❌ Failing |

**Key Difference**: E2E-MULTI tests use WorkflowExecution CRDs which are working, but RemediationRequest webhook appears not to be intercepting.

---

## 🎯 **Recommended Next Steps**

### **Immediate** (30 minutes):

1. **Check webhook deployment manifest**:
   ```bash
   kubectl get mutatingwebhookconfigurations authwebhook-mutating -o yaml | grep -A 20 remediationrequest
   ```

2. **Verify webhook handler registration**:
   ```bash
   grep -A 10 "/mutate-remediationrequest" cmd/webhooks/main.go
   ```

3. **Test webhook manually**:
   ```bash
   # Create RR and check webhook logs in real-time
   kubectl logs -f -n authwebhook-e2e -l app=authwebhook
   ```

---

### **Short-term** (2 hours):

1. **Add debug logging to webhook handler**:
   ```go
   func (h *RemediationRequestStatusHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
       log.Info("Webhook received RemediationRequest update", "name", req.Name)
       // ... existing code ...
   }
   ```

2. **Test webhook in isolation**:
   - Create minimal test case
   - Add verbose logging
   - Verify webhook interception

3. **Compare with working webhook**:
   - Review WorkflowExecution webhook (known working)
   - Check for differences in configuration

---

### **Long-term** (4 hours):

1. **Comprehensive webhook testing**:
   - Add unit tests for webhook handler
   - Add integration tests with envtest + webhook server
   - Add E2E tests with full cluster

2. **Production validation**:
   - Deploy to staging
   - Test with real operator actions (`kubectl edit`)
   - Validate audit events in production DataStorage

---

## 📚 **Related Files**

| File | Purpose | Status |
|------|---------|--------|
| `test/e2e/authwebhook/02_gap8_remediationrequest_timeout_mutation_test.go` | E2E test | ⚠️ Failing |
| `pkg/webhooks/remediationrequest_handler.go` | Webhook handler | ✅ Implemented |
| `test/e2e/authwebhook/manifests/authwebhook-deployment.yaml` | Webhook deployment | ✅ Deployed |
| `cmd/webhooks/main.go` | Webhook server | ✅ Running |
| `test/integration/remediationorchestrator/gap8_timeout_config_audit_test.go` | Integration tests | ✅ 2/2 Passing |

---

## 🎓 **Lessons Learned**

1. **E2E testing reveals integration issues**: Integration tests passed but E2E exposed webhook interception issue
2. **Webhook testing requires full infrastructure**: Can't test webhooks in isolation
3. **Manual vs. automated testing**: Need to test webhook manually with `kubectl edit` to validate
4. **Debug logging is essential**: Need more logging in webhook handler for troubleshooting

---

## 📊 **Overall Gap #8 Status**

| Component | Status | Evidence |
|-----------|--------|----------|
| **Implementation** | ✅ Complete | `pkg/webhooks/remediationrequest_handler.go` |
| **Integration Tests** | ✅ 2/2 Passing | Controller initialization working |
| **E2E Test** | ❌ Failing | Webhook audit event not emitted |
| **Production Ready** | ⚠️ **Blocked** | E2E validation required |

**Blocker**: Webhook not intercepting RemediationRequest status updates in E2E environment

---

## 🚀 **Workaround for Production**

**Option 1**: Deploy to staging and test manually
- Use `kubectl edit` to modify TimeoutConfig
- Verify audit event in DataStorage
- If working, E2E test may have environment-specific issue

**Option 2**: Skip webhook for now
- Controller initialization audit event (`orchestrator.lifecycle.created`) working
- Operator mutation audit can be added later
- Partial SOC2 compliance better than blocking deployment

**Option 3**: Fix webhook configuration
- Deep dive into webhook deployment manifest
- Compare with working WorkflowExecution webhook
- Debug webhook interception path

---

**Document Version**: 1.0  
**Created**: January 13, 2026  
**Status**: ⚠️ **Issue Documented - Investigation Needed**  
**Priority**: **High** - Blocks Gap #8 E2E validation  
**Estimated Fix Time**: 2-4 hours  
**BR-AUDIT-005 v2.0**: Gap #8 - TimeoutConfig mutation audit capture
