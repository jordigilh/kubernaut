# SOC2 Week 2 - Complete Implementation Plan v1.1

**Version**: v1.1
**Date**: January 7, 2026
**Previous Version**: v1.0 (implicitly from AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md)
**Status**: ⏳ **Days 9-10.5 PENDING** (Days 1-8 Complete - 80%)
**Authority**: AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md

---

## 📋 **CHANGELOG**

### **v1.1 (January 7, 2026)** - Current Version

**Added**:
- ✅ **Day 10.5**: Auth Webhook Deployment & Integration (4-5 hours)
  - Production deployment manifests (`deploy/authwebhook/`)
  - Complete user attribution for CRD operations
  - SOC2 CC8.1 compliance (user attribution requirement)
- ✅ Additional completed work documentation:
  - OAuth-Proxy Integration (DataStorage & HAPI) - Jan 7
  - E2E OpenAPI Migration (100% compliant) - Jan 7

**Updated**:
- Timeline: 9-11 hours → 13-16 hours (added Day 10.5)
- SOC2 Progress: 80% complete (Days 1-8 of 10.5)
- Completion criteria: Added webhook deployment requirement
- Dependencies: All satisfied, ready for Days 9-10.5

**Rationale**:
- User question: "Are webhooks part of the plan?"
- Analysis: Webhooks are REQUIRED for complete SOC2 user attribution (CC8.1)
- Webhook implementation: 97% complete (just needs deployment manifests)
- Decision: Add as Day 10.5 before declaring SOC2 "100% complete"

---

### **v1.0 (December 18, 2025)** - Baseline

**Original Plan**:
- Week 1 (Days 1-6): RR Reconstruction
- Week 2 Days 7-8: Event Hashing + Legal Hold
- Week 2 Days 9-10: Export + RBAC + PII

**Missing from v1.0**:
- Auth webhook deployment (now added as Day 10.5)
- OAuth-proxy integration (completed out-of-band)
- E2E OpenAPI migration (completed out-of-band)

---

## ✅ **COMPLETED WORK** (Days 1-8 + Additional)

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
  - **Doc**: `SOC2/GAP9_HASH_CHAIN_COMPLETE_JAN06.md`

- ✅ **Day 8 (Gap #8)**: Legal Hold & Retention Policies
  - `legal_hold` column with correlation tracking
  - `audit_retention_policies` table
  - Legal hold API endpoints (POST, DELETE, GET)
  - **Status**: ✅ COMPLETE (Jan 6)
  - **Doc**: `SOC2/GAP8_LEGAL_HOLD_COMPLETE_JAN06.md`

### **Additional Completed Work** ✅ (v1.1)
- ✅ **OAuth-Proxy Integration**: DataStorage & HAPI (Jan 7)
  - External authentication/authorization
  - Subject Access Review (SAR)
  - User identity injection via `X-Auth-Request-User`
  - **Doc**: `SOC2/OAUTH_PROXY_IMPLEMENTATION_STATUS_JAN07.md`

- ✅ **E2E OpenAPI Migration**: DataStorage & HAPI (Jan 7)
  - 7/7 DataStorage E2E files migrated to OpenAPI client
  - 8/8 HAPI E2E files verified compliant
  - Pre-generation validation added
  - **Doc**: `handoff/DS_E2E_OPENAPI_MIGRATION_COMPLETE_JAN07.md`

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

---

#### **9.2: Audit Verification Tools** (2-3 hours)

**Deliverables**:
1. Hash chain verification across exported events
2. Digital signature verification
3. CLI tool (optional): `kubernaut-audit verify-export`

**Files to Create/Modify**:
- `pkg/datastorage/verification/hash_chain.go` (new)
- `pkg/datastorage/verification/signature.go` (new)
- `cmd/audit-verify/main.go` (new, optional)

---

### **Day 10: RBAC + PII Redaction + E2E Tests** ⏳ (4-5 hours)

**SOC2 Requirements**: AC-3 (Access Control), GDPR (PII Protection)

#### **10.1: RBAC for Audit Queries** (2-3 hours)

**Deliverables**:
1. Fine-grained permissions (auditor, admin, operator)
2. Kubernetes RBAC integration (leverage oauth-proxy)
3. Subject Access Review (SAR) for audit endpoints

**Access Control Matrix**:
```
Role      | Query | Export | Legal Hold | Verify Chain
----------|-------|--------|------------|-------------
auditor   | ✅    | ✅     | ❌         | ✅
admin     | ✅    | ✅     | ✅         | ✅
operator  | ⚠️ *  | ❌     | ❌         | ❌

* filtered to own events only
```

---

#### **10.2: PII Redaction** (1-2 hours)

**Deliverables**:
1. PII detection (emails, IPs, phone numbers)
2. Redaction rules (configurable patterns)
3. Redaction modes (none, partial, full)

---

#### **10.3: E2E Compliance Tests** (1 hour)

**Deliverables**:
1. Hash chain E2E test
2. Legal hold E2E test
3. Export/verification E2E test

**File**: `test/e2e/datastorage/05_soc2_compliance_test.go` (new)

---

### **Day 10.5: Auth Webhook Deployment** ⏳ (4-5 hours) **← NEW in v1.1**

**SOC2 Requirements**: CC8.1 (User Attribution), AU-2 (Auditable Events)

**Why Added** (v1.1):
- Complete user attribution for CRD operations (WHO cleared block, WHO approved, WHO cancelled)
- SOC2 CC8.1 requires comprehensive audit trail including operator actions
- Webhook implementation 97% complete, only deployment manifests needed
- User confirmed: "We need to integrate auth webhooks into required services"

#### **10.5.1: Production Deployment Manifests** (2-3 hours)

**Deliverables** (`deploy/authwebhook/`):
1. **`01-namespace.yaml`** (if needed)
2. **`02-serviceaccount.yaml`** - ServiceAccount for webhook
3. **`03-rbac.yaml`** - ClusterRole + ClusterRoleBinding
4. **`04-tls-secret.yaml`** - Certificate management
5. **`05-deployment.yaml`** - Webhook pod deployment
   - Image: `quay.io/jordigilh/kubernaut-webhooks:v1.0.0`
   - Port: 9443 (HTTPS)
   - Env vars: `DATA_STORAGE_URL`, `LOG_LEVEL`
6. **`06-service.yaml`** - HTTPS service (port 9443)
7. **`07-mutating-webhook-config.yaml`** - MutatingWebhookConfiguration
8. **`08-validating-webhook-config.yaml`** - ValidatingWebhookConfiguration
9. **`kustomization.yaml`** - Kustomize configuration

**Pattern**: Follow `deploy/data-storage/` structure

---

#### **10.5.2: Deploy to Development Cluster** (1 hour)

**Steps**:
1. Generate TLS certificates
2. Apply manifests: `kubectl apply -k deploy/authwebhook/`
3. Verify deployment (pod running, webhook registration)
4. Smoke test (create test CRD, verify user attribution)

---

#### **10.5.3: Integration with E2E Tests** (30 min)

**Update E2E Tests**:
- DataStorage E2E: Verify user attribution in audit events
- AuthWebhook E2E: Fix path issue, run full suite
- Integration tests: Verify webhook → DataStorage flow

---

#### **10.5.4: Documentation** (30 min)

**Documents to Create/Update**:
- `docs/operations/webhook-deployment.md` (deployment guide)
- `docs/operations/webhook-operations.md` (operations runbook)
- `docs/compliance/soc2-user-attribution.md` (compliance docs)

---

## 📊 **VERSION COMPARISON**

| Aspect | v1.0 (Dec 18) | v1.1 (Jan 7) | Change |
|--------|---------------|--------------|--------|
| **Total Days** | 10 days | 10.5 days | +0.5 day |
| **Remaining Effort** | 9-11 hours | 13-16 hours | +4-5 hours |
| **SOC2 Progress** | 75% | 80% | +5% (oauth-proxy, e2e migration) |
| **User Attribution** | Incomplete | Complete | +Webhook deployment |
| **Completion Criteria** | 7 items | 8 items | +Webhook requirement |

---

## 📊 **UPDATED TIMELINE** (v1.1)

| Day | Task | Effort | Status | v1.1 Change |
|-----|------|--------|--------|-------------|
| **1-6** | RR Reconstruction | 30-40 hours | ✅ COMPLETE | - |
| **7** | Event Hashing (Gap #9) | 6 hours | ✅ COMPLETE | - |
| **8** | Legal Hold (Gap #8) | 5 hours | ✅ COMPLETE | - |
| **-** | OAuth-Proxy Integration | 8 hours | ✅ COMPLETE | ✅ Added in v1.1 |
| **-** | E2E OpenAPI Migration | 8 hours | ✅ COMPLETE | ✅ Added in v1.1 |
| **9** | Signed Export + Verification | 5-6 hours | ⏳ PENDING | - |
| **10** | RBAC + PII + E2E Tests | 4-5 hours | ⏳ PENDING | - |
| **10.5** | **Auth Webhook Deployment** | **4-5 hours** | ⏳ **PENDING** | ✅ **NEW in v1.1** |
| **TOTAL REMAINING** | | **13-16 hours** | **80% → 100%** | **+4-5 hours** |

**Estimated Time to Complete**: ~13-16 hours (~1.5-2 days)

---

## 🎯 **EXECUTION PLAN** (v1.1)

### **Phase 1: Verification** (1-2 hours) ⏳ **NEXT**
Run E2E tests to verify completed work:
```bash
make test-e2e-datastorage      # Hash chains, legal hold, oauth-proxy
cd holmesgpt-api && pytest tests/e2e/  # HAPI E2E
make test-e2e-authwebhook      # Webhook integration (fix path first)
```

**Goal**: Confidence in completed work

---

### **Phase 2: Day 9 Implementation** (5-6 hours)
1. Signed audit export API (3-4 hours)
2. Verification tools (2-3 hours)

**Goal**: Audit export with tamper-evident signatures

---

### **Phase 3: Day 10 Implementation** (4-5 hours)
1. RBAC for audit queries (2-3 hours)
2. PII redaction (1-2 hours)
3. E2E compliance tests (1 hour)

**Goal**: Access control and GDPR compliance

---

### **Phase 4: Day 10.5 Implementation** (4-5 hours) **← NEW in v1.1**
1. Production deployment manifests (2-3 hours)
2. Deploy to dev cluster (1 hour)
3. Integration testing (30 min)
4. Documentation (30 min)

**Goal**: Complete user attribution for CRD operations

---

### **Phase 5: Final Verification** (1 hour)
Run full E2E compliance suite

**Goal**: 100% SOC2 compliance verified

---

## ✅ **COMPLETION CRITERIA** (v1.1)

**SOC2 Week 2 is COMPLETE when**:
- ✅ All audit events have hash chains (tamper-evidence)
- ✅ Legal hold prevents deletion (retention compliance)
- ✅ Signed exports available (audit export)
- ✅ Hash chain verification tools working (integrity verification)
- ✅ RBAC enforced for audit queries (access control)
- ✅ PII redaction implemented (GDPR compliance)
- ✅ **Auth webhooks deployed (user attribution)** ← **NEW in v1.1**
- ✅ E2E compliance tests passing (validation)

**Result**: **100% SOC 2 Type II Readiness** 🏆

---

## 📋 **DEPENDENCIES**

### **External Dependencies** (All Satisfied ✅):
- ✅ DataStorage service (deployed)
- ✅ PostgreSQL with pgvector (deployed)
- ✅ Redis (deployed)
- ✅ OAuth-proxy integration (complete)

### **Code Dependencies** (All Satisfied ✅):
- ✅ OpenAPI clients (generated and migrated)
- ✅ Hash chain implementation (complete)
- ✅ Legal hold implementation (complete)
- ✅ Auth webhook implementation (97% complete)

**All dependencies satisfied!** Ready for Days 9-10.5.

---

## 🎉 **IMPACT**

### **Before Week 2**:
- ⚠️ Audit logs vulnerable to tampering
- ⚠️ No retention policy enforcement
- ⚠️ Manual audit export
- ⚠️ No access control
- ⚠️ PII exposure risk
- ⚠️ **Incomplete user attribution for CRDs** ← **Fixed in v1.1**

### **After Week 2 (v1.1)**:
- ✅ Tamper-evident audit logs (hash chains)
- ✅ Legal hold enforcement (compliance)
- ✅ Signed audit exports (integrity)
- ✅ RBAC for audit access (security)
- ✅ PII redaction (GDPR)
- ✅ **Complete user attribution (webhooks)** ← **NEW in v1.1**

**Result**: Enterprise-grade audit infrastructure, SOC 2 Type II ready! 🚀

---

## 📚 **RELATED DOCUMENTS**

### **Authoritative Plans**:
- `AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md` (v1.0 baseline)
- This document: `SOC2_WEEK2_COMPLETE_PLAN_V1_1_JAN07.md` (current version)

### **Implementation Documentation**:
- `SOC2/GAP9_HASH_CHAIN_COMPLETE_JAN06.md` (Day 7)
- `SOC2/GAP8_LEGAL_HOLD_COMPLETE_JAN06.md` (Day 8)
- `SOC2/OAUTH_PROXY_IMPLEMENTATION_STATUS_JAN07.md` (OAuth-proxy)
- `handoff/DS_E2E_OPENAPI_MIGRATION_COMPLETE_JAN07.md` (E2E migration)
- `handoff/AUTHWEBHOOK_DEPLOYMENT_TRIAGE_JAN07.md` (Webhook triage)

### **Technical References**:
- DD-AUTH-001: Authentication architecture
- DD-WEBHOOK-001: Webhook patterns
- SOC2 CC8.1: User attribution requirements
- SOC2 AU-9: Audit protection requirements
- SOC2 AC-3: Access control requirements

---

## 📝 **VERSION HISTORY**

| Version | Date | Author | Changes | Reason |
|---------|------|--------|---------|--------|
| v1.0 | Dec 18, 2025 | SOC2 Team | Initial plan (Days 1-10) | Baseline SOC2 plan |
| v1.1 | Jan 7, 2026 | SOC2 Team | Added Day 10.5 (Webhooks) | User attribution requirement |

---

**Status**: ⏳ **Ready to execute Days 9-10.5** (13-16 hours to 100% SOC2)
**Version**: v1.1 (Updated January 7, 2026)
**Authority**: DD-AUTH-001, DD-WEBHOOK-001, SOC2 CC8.1, AU-9, AC-3, GDPR
**Next Action**: Run E2E tests (Phase 1 Verification)

