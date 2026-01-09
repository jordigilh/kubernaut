# SOC2 Week 2 - Complete Implementation Plan (Updated Jan 7, 2026)

**Date**: January 7, 2026  
**Status**: ⏳ **Days 9-10.5 PENDING** (Days 1-8 Complete)  
**Authority**: AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md  
**Updated**: Added Day 10.5 (Auth Webhook Deployment) for complete user attribution  

---

## ✅ **COMPLETED WORK** (Days 1-8)

### **Week 1: RR Reconstruction** (Days 1-6) ✅
- ✅ Day 1: Gateway - OriginalPayload, SignalLabels, SignalAnnotations
- ✅ Day 2: AI Analysis - ProviderData (Holmes response)
- ✅ Day 3: Workflow & Execution - SelectedWorkflowRef, ExecutionRef
- ✅ Day 4: Error Details Standardization (Gap #7)
- ✅ Day 5: TimeoutConfig & Audit Reconstruction API
- ✅ Day 6: CLI Tool (deferred to post-V1.0)

### **Week 2: Enterprise Compliance** (Days 7-8) ✅
- ✅ **Day 7 (Gap #9)**: Event Hashing (Tamper-Evidence)
  - Hash chain implementation (blockchain-style)
  - `event_hash` and `previous_event_hash` columns
  - Verification API endpoint
  - **Status**: ✅ COMPLETE (Jan 6)

- ✅ **Day 8 (Gap #8)**: Legal Hold & Retention Policies
  - `legal_hold` column with correlation tracking
  - `audit_retention_policies` table
  - Legal hold API endpoints (POST, DELETE, GET)
  - **Status**: ✅ COMPLETE (Jan 6)

### **Additional Completed Work** ✅
- ✅ **OAuth-Proxy Integration**: DataStorage & HAPI (Jan 7)
  - External authentication/authorization
  - Subject Access Review (SAR)
  - User identity injection via `X-Auth-Request-User`
  
- ✅ **E2E OpenAPI Migration**: DataStorage & HAPI (Jan 7)
  - 7/7 DataStorage E2E files migrated to OpenAPI client
  - 8/8 HAPI E2E files verified compliant
  - Pre-generation validation added

**Current Progress**: **80% Complete** (Days 1-8 of 10.5)

---

## 🔄 **REMAINING WORK** (Days 9-10.5)

### **Day 9: Signed Export + Verification** ⏳ (5-6 hours)

**SOC2 Requirements**: CC8.1 (Audit Export), AU-9 (Audit Protection)

#### **9.1: Signed Audit Export API** (3-4 hours)

**Deliverables**:
1. **Export API Endpoint**: `/api/v1/audit/export`
   - Query parameters: `start_time`, `end_time`, `correlation_id`, `event_category`
   - Export formats: JSON, CSV
   - Pagination support for large exports
   - Includes hash chain verification data

2. **Digital Signature Implementation**:
   - Sign exports with GPG or x509 certificate
   - Include signature in export metadata
   - Detached signature file option

3. **Export Metadata**:
   - Export timestamp
   - Query filters used
   - Total records
   - Hash chain integrity status
   - Digital signature

**Files to Create/Modify**:
- `pkg/datastorage/server/audit_export_handler.go` (new)
- `pkg/datastorage/repository/audit_export.go` (new)
- `api/openapi/data-storage-v1.yaml` (add export endpoint)

**Testing**:
- Unit tests: Export logic, signature generation
- Integration tests: Export with various filters
- E2E tests: Full export + verification flow

---

#### **9.2: Audit Verification Tools** (2-3 hours)

**Deliverables**:
1. **Hash Chain Verification**:
   - Verify hash chain integrity across exported events
   - Detect any tampering or missing events
   - Report verification status

2. **Signature Verification**:
   - Verify digital signature on exported files
   - Check certificate validity
   - Tamper detection

3. **CLI Tool** (optional):
   - `kubernaut-audit verify-export --file export.json`
   - User-friendly verification reports
   - Exit codes for automation

**Files to Create/Modify**:
- `pkg/datastorage/verification/hash_chain.go` (new)
- `pkg/datastorage/verification/signature.go` (new)
- `cmd/audit-verify/main.go` (new, optional)

**Testing**:
- Unit tests: Verification logic
- Integration tests: Verify tampered vs valid exports
- E2E tests: End-to-end verification workflow

---

### **Day 10: RBAC + PII Redaction + E2E Tests** ⏳ (4-5 hours)

**SOC2 Requirements**: AC-3 (Access Control), GDPR (PII Protection)

#### **10.1: RBAC for Audit Queries** (2-3 hours)

**Deliverables**:
1. **Fine-Grained Permissions**:
   - `auditor`: Read-only access to all audit events
   - `admin`: Full access (read + export + legal hold)
   - `operator`: Read own events only (filtered by actor_id)

2. **Kubernetes RBAC Integration**:
   - Leverage existing OAuth-proxy infrastructure
   - Subject Access Review (SAR) for audit endpoints
   - ClusterRole definitions for audit access

3. **Access Control Matrix**:
   ```
   Role      | Query | Export | Legal Hold | Verify Chain
   ----------|-------|--------|------------|-------------
   auditor   | ✅    | ✅     | ❌         | ✅
   admin     | ✅    | ✅     | ✅         | ✅
   operator  | ⚠️ *  | ❌     | ❌         | ❌
   
   * filtered to own events only
   ```

**Files to Create/Modify**:
- `deploy/data-storage/rbac-audit.yaml` (new)
- `pkg/datastorage/server/middleware/rbac.go` (new, or enhance oauth-proxy config)
- Update oauth-proxy SAR configuration

**Testing**:
- Integration tests: RBAC enforcement
- E2E tests: Access control verification

---

#### **10.2: PII Redaction** (1-2 hours)

**Deliverables**:
1. **PII Detection**:
   - Email addresses (regex: `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
   - IP addresses (regex: `\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
   - Phone numbers, SSN, credit cards (optional)

2. **Redaction Rules**:
   - Automatic redaction in exports (GDPR compliance)
   - Configurable redaction patterns
   - Preserve audit integrity (redact display, keep original hash)

3. **Redaction Modes**:
   - `none`: No redaction (admin only)
   - `partial`: Redact PII, keep structure (e.g., `user@*****.com`)
   - `full`: Replace with placeholder (e.g., `[REDACTED]`)

**Files to Create/Modify**:
- `pkg/datastorage/redaction/pii.go` (new)
- `pkg/datastorage/server/audit_export_handler.go` (add redaction)

**Testing**:
- Unit tests: PII detection and redaction
- Integration tests: Export with redaction

---

#### **10.3: E2E Compliance Tests** (1 hour)

**Deliverables**:
1. **Hash Chain E2E Test**:
   - Create 10+ events
   - Export events
   - Verify hash chain integrity
   - Attempt tampering (should detect)

2. **Legal Hold E2E Test**:
   - Place legal hold on correlation_id
   - Attempt deletion (should fail)
   - Release legal hold
   - Verify retention policies

3. **Export/Verification E2E Test**:
   - Export large dataset (100+ events)
   - Verify signature
   - Verify hash chain
   - Test with tampering

**Files to Create/Modify**:
- `test/e2e/datastorage/05_soc2_compliance_test.go` (new)

**Testing**:
- Run full E2E compliance suite
- Verify all SOC2 requirements met

---

### **Day 10.5: Auth Webhook Deployment** ⏳ (4-5 hours) **← NEW**

**SOC2 Requirements**: CC8.1 (User Attribution), AU-2 (Auditable Events)

**Why Added**: Complete user attribution for CRD operations (block clearance, approvals, cancellations)

#### **10.5.1: Production Deployment Manifests** (2-3 hours)

**Deliverables** (`deploy/authwebhook/`):
1. **`01-namespace.yaml`** (if needed)
2. **`02-serviceaccount.yaml`** - ServiceAccount for webhook
3. **`03-rbac.yaml`** - ClusterRole + ClusterRoleBinding
   - Read/write access to CRDs (WorkflowExecution, RemediationApprovalRequest, NotificationRequest)
   - Read access to Users (for validation)

4. **`04-tls-secret.yaml`** - Certificate management
   - Self-signed cert generation script
   - Or cert-manager integration

5. **`05-deployment.yaml`** - Webhook pod deployment
   - Image: `quay.io/jordigilh/kubernaut-webhooks:v1.0.0`
   - Port: 9443 (HTTPS)
   - Env vars:
     - `DATA_STORAGE_URL=http://data-storage:8080`
     - `LOG_LEVEL=info`
     - `TLS_CERT_PATH=/etc/webhook/certs/tls.crt`
     - `TLS_KEY_PATH=/etc/webhook/certs/tls.key`
   - Resources:
     - CPU: 100m request, 500m limit
     - Memory: 128Mi request, 512Mi limit
   - Health probes: `/healthz` endpoint

6. **`06-service.yaml`** - HTTPS service
   - Port: 9443 (HTTPS)
   - TargetPort: 9443
   - ClusterIP (internal only)

7. **`07-mutating-webhook-config.yaml`** - MutatingWebhookConfiguration
   - WorkflowExecution mutation (`/mutate-workflowexecution`)
   - RemediationApprovalRequest mutation (`/mutate-remediationapprovalrequest`)
   - Failure policy: Fail (strict enforcement)
   - CA bundle: base64-encoded certificate

8. **`08-validating-webhook-config.yaml`** - ValidatingWebhookConfiguration
   - NotificationRequest deletion validation (`/validate-notificationrequest-delete`)
   - Failure policy: Fail (strict enforcement)
   - CA bundle: base64-encoded certificate

9. **`kustomization.yaml`** - Kustomize configuration

**Pattern**: Follow `deploy/data-storage/` structure

---

#### **10.5.2: Deploy to Development Cluster** (1 hour)

**Steps**:
1. **Generate TLS Certificates**:
   ```bash
   # Use script or cert-manager
   ./scripts/generate-webhook-certs.sh
   ```

2. **Apply Manifests**:
   ```bash
   kubectl apply -k deploy/authwebhook/
   ```

3. **Verify Deployment**:
   ```bash
   # Check pod running
   kubectl get pods -n kubernaut-system -l app=kubernaut-auth-webhook
   
   # Check webhook registration
   kubectl get mutatingwebhookconfigurations
   kubectl get validatingwebhookconfigurations
   
   # Check logs
   kubectl logs -n kubernaut-system -l app=kubernaut-auth-webhook
   ```

4. **Smoke Test**:
   ```bash
   # Create test WorkflowExecution
   kubectl apply -f test/e2e/authwebhook/fixtures/test-wfe.yaml
   
   # Verify user attribution
   kubectl get workflowexecution test-wfe -o yaml | grep clearedBy
   
   # Verify audit event in DataStorage
   curl http://localhost:30081/api/v1/audit?event_type=workflowexecution.block.cleared
   ```

---

#### **10.5.3: Integration with E2E Tests** (30 min)

**Update E2E Tests**:
1. **DataStorage E2E**: Already uses CRDs, verify user attribution in audit events
2. **AuthWebhook E2E**: Fix path issue, run full suite
3. **Integration Tests**: Verify webhook → DataStorage flow

**Verification**:
```bash
make test-e2e-authwebhook
make test-e2e-datastorage
```

---

#### **10.5.4: Documentation** (30 min)

**Update**:
1. **Deployment Docs**: `docs/operations/webhook-deployment.md`
   - How to deploy webhooks
   - Certificate management
   - Troubleshooting

2. **Operations Runbook**: `docs/operations/webhook-operations.md`
   - Certificate rotation
   - Scaling considerations
   - Common issues

3. **SOC2 Compliance Docs**: `docs/compliance/soc2-user-attribution.md`
   - User attribution architecture
   - Audit trail completeness
   - Compliance verification

---

## 📊 **UPDATED TIMELINE**

| Day | Task | Effort | Status | SOC2 Impact |
|-----|------|--------|--------|-------------|
| **1-6** | RR Reconstruction | 30-40 hours | ✅ COMPLETE | Foundation |
| **7** | Event Hashing (Gap #9) | 6 hours | ✅ COMPLETE | Tamper-evidence |
| **8** | Legal Hold (Gap #8) | 5 hours | ✅ COMPLETE | Retention |
| **-** | OAuth-Proxy Integration | 8 hours | ✅ COMPLETE | Authentication |
| **-** | E2E OpenAPI Migration | 8 hours | ✅ COMPLETE | Test quality |
| **9** | Signed Export + Verification | 5-6 hours | ⏳ PENDING | Audit export |
| **10** | RBAC + PII + E2E Tests | 4-5 hours | ⏳ PENDING | Access control |
| **10.5** | **Auth Webhook Deployment** | **4-5 hours** | ⏳ **PENDING** | **User attribution** |
| **TOTAL** | | **13-16 hours** | **80% → 100%** | **Full SOC2 Type II** |

**Estimated Time to Complete**: ~13-16 hours (~1.5-2 days)

---

## 🎯 **EXECUTION PLAN**

### **Phase 1: Verification** (1-2 hours) ⏳ **NEXT**
Run E2E tests to verify completed work:
```bash
# Verify DataStorage (hash chains, legal hold, oauth-proxy)
make test-e2e-datastorage

# Verify HAPI E2E
cd holmesgpt-api && pytest tests/e2e/

# Verify AuthWebhook (fix path issue first)
make test-e2e-authwebhook
```

**Goal**: Confidence in completed work before proceeding

---

### **Phase 2: Day 9 Implementation** (5-6 hours)
1. Signed audit export API (3-4 hours)
2. Verification tools (2-3 hours)
3. Testing (included)

**Goal**: Audit export with tamper-evident signatures

---

### **Phase 3: Day 10 Implementation** (4-5 hours)
1. RBAC for audit queries (2-3 hours)
2. PII redaction (1-2 hours)
3. E2E compliance tests (1 hour)

**Goal**: Access control and GDPR compliance

---

### **Phase 4: Day 10.5 Implementation** (4-5 hours)
1. Production deployment manifests (2-3 hours)
2. Deploy to dev cluster (1 hour)
3. Integration testing (30 min)
4. Documentation (30 min)

**Goal**: Complete user attribution for CRD operations

---

### **Phase 5: Final Verification** (1 hour)
Run full E2E compliance suite:
```bash
make test-e2e-datastorage  # Includes SOC2 compliance tests
make test-e2e-authwebhook  # Webhook integration
make test-integration-authwebhook  # Audit event flow
```

**Goal**: 100% SOC2 compliance verified

---

## ✅ **COMPLETION CRITERIA**

**SOC2 Week 2 is COMPLETE when**:
- ✅ All audit events have hash chains (tamper-evidence)
- ✅ Legal hold prevents deletion (retention compliance)
- ✅ Signed exports available (audit export)
- ✅ Hash chain verification tools working (integrity verification)
- ✅ RBAC enforced for audit queries (access control)
- ✅ PII redaction implemented (GDPR compliance)
- ✅ **Auth webhooks deployed (user attribution)** ← NEW
- ✅ E2E compliance tests passing (validation)

**Result**: **100% SOC 2 Type II Readiness** 🏆

---

## 📋 **DEPENDENCIES**

### **External Dependencies**:
- ✅ DataStorage service (deployed)
- ✅ PostgreSQL with pgvector (deployed)
- ✅ Redis (deployed)
- ✅ OAuth-proxy integration (complete)

### **Code Dependencies**:
- ✅ OpenAPI clients (generated and migrated)
- ✅ Hash chain implementation (complete)
- ✅ Legal hold implementation (complete)
- ✅ Auth webhook implementation (complete)

**All dependencies satisfied!** Ready to proceed with Days 9-10.5.

---

## 🎉 **IMPACT**

### **Before Week 2**:
- ⚠️ Audit logs vulnerable to tampering
- ⚠️ No retention policy enforcement
- ⚠️ Manual audit export
- ⚠️ No access control
- ⚠️ PII exposure risk
- ⚠️ **Incomplete user attribution for CRDs** ← NEW

### **After Week 2**:
- ✅ Tamper-evident audit logs (hash chains)
- ✅ Legal hold enforcement (compliance)
- ✅ Signed audit exports (integrity)
- ✅ RBAC for audit access (security)
- ✅ PII redaction (GDPR)
- ✅ **Complete user attribution (webhooks)** ← NEW

**Result**: Enterprise-grade audit infrastructure, SOC 2 Type II ready! 🚀

---

## 📚 **RELATED DOCUMENTS**

- `AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md` (authoritative plan)
- `SOC2/GAP9_HASH_CHAIN_COMPLETE_JAN06.md` (Day 7 complete)
- `SOC2/GAP8_LEGAL_HOLD_COMPLETE_JAN06.md` (Day 8 complete)
- `SOC2/OAUTH_PROXY_IMPLEMENTATION_STATUS_JAN07.md` (OAuth-proxy complete)
- `handoff/DS_E2E_OPENAPI_MIGRATION_COMPLETE_JAN07.md` (E2E migration complete)
- `handoff/AUTHWEBHOOK_DEPLOYMENT_TRIAGE_JAN07.md` (webhook triage)

---

**Status**: ⏳ **Ready to execute Days 9-10.5** (13-16 hours to 100% SOC2)  
**Updated**: January 7, 2026 - Added Day 10.5 (Auth Webhook Deployment)  
**Authority**: DD-AUTH-001, DD-WEBHOOK-001, SOC2 CC8.1, AU-9, AC-3, GDPR


