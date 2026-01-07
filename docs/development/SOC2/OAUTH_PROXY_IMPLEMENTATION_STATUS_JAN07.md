# OAuth-Proxy Sidecar Implementation Status - SOC2 Gap #8

**Date**: January 7, 2026  
**Status**: 🔄 **IN PROGRESS** (1/7 tasks complete)  
**Authority**: DD-AUTH-004, SOC2 Gap #8 Legal Hold Authentication

---

## 📊 Implementation Progress

### ✅ Task 1: Design Decision Document (COMPLETE)
- **File**: `docs/architecture/decisions/DD-AUTH-004-openshift-oauth-proxy-legal-hold.md`
- **Commit**: `7dcc980a0`
- **Status**: ✅ Complete
- **Summary**: Comprehensive DD documenting OpenShift oauth-proxy sidecar pattern for legal hold authentication

---

### 🔄 Task 2: Update DataStorage K8s Deployment (IN PROGRESS)
- **File**: `test/infrastructure/datastorage.go` (E2E deployment)
- **Status**: 🔄 In Progress
- **Changes Needed**:
  1. Add oauth-proxy sidecar container to deployment (line ~1210)
  2. Add TLS certificate volume for oauth-proxy
  3. Add oauth-proxy secret for cookie-secret
  4. Update service to expose port 8443 (oauth-proxy HTTPS)
  5. Create ServiceAccount with RBAC for oauth-proxy SAR

---

### ⬜ Task 3: Update Handler Logic (PENDING)
- **Files**:
  - `pkg/datastorage/server/legal_hold_handler.go`
- **Status**: ⬜ Pending
- **Changes Needed**:
  1. Replace `X-User-ID` with `X-Auth-Request-User` header
  2. Keep 401 validation (defense-in-depth)
  3. Update error messages to reference oauth-proxy
  4. Update logging to indicate security control failure

---

### ⬜ Task 4: Update Integration Tests (PENDING)
- **File**: `test/integration/datastorage/legal_hold_integration_test.go`
- **Status**: ⬜ Pending
- **Changes Needed**:
  1. Replace `X-User-ID` with `X-Auth-Request-User` header in all tests
  2. Keep test for 401 when header missing (defense-in-depth validation)
  3. Update test documentation

---

### ⬜ Task 5: Update E2E Infrastructure (PENDING)
- **File**: `test/infrastructure/datastorage.go`
- **Status**: ⬜ Pending
- **Changes Needed**:
  1. Deploy oauth-proxy sidecar in Kind (deployDataStorageInNamespace function)
  2. Create ServiceAccount `datastorage` in namespace
  3. Create RBAC ClusterRoleBinding for oauth-proxy SAR
  4. Create TLS certificate secret for oauth-proxy
  5. Create oauth-proxy cookie-secret
  6. Update Kind config for port 8443 exposure (if needed)

---

### ⬜ Task 6: Update OpenAPI Spec (PENDING)
- **File**: `api/openapi/data-storage-v1.yaml`
- **Status**: ⬜ Pending
- **Changes Needed**:
  1. Update security scheme description to reference oauth-proxy
  2. Document X-Auth-Request-User header (injected by oauth-proxy)
  3. Update authentication flow documentation
  4. Regenerate Go and Python clients

---

### ⬜ Task 7: Run Tests to Verify (PENDING)
- **Test Suites**:
  - Integration tests: `test/integration/datastorage/legal_hold_integration_test.go`
  - E2E tests: TBD (if legal hold E2E tests exist)
- **Status**: ⬜ Pending
- **Success Criteria**:
  - All integration tests pass (7/7)
  - oauth-proxy sidecar deploys successfully in E2E
  - Defense-in-depth validation works (401 on missing header)

---

## 📋 Files to Modify

### Core Implementation
1. ✅ `docs/architecture/decisions/DD-AUTH-004-openshift-oauth-proxy-legal-hold.md` (DONE)
2. 🔄 `test/infrastructure/datastorage.go` (IN PROGRESS)
3. ⬜ `pkg/datastorage/server/legal_hold_handler.go`
4. ⬜ `test/integration/datastorage/legal_hold_integration_test.go`
5. ⬜ `api/openapi/data-storage-v1.yaml`

### Generated Files (After OpenAPI Update)
6. ⬜ `pkg/datastorage/client/generated.go` (Go client)
7. ⬜ `holmesgpt-api/src/clients/datastorage/` (Python client)

---

## 🎯 Next Steps

### Immediate (Task 2):
1. Add oauth-proxy sidecar container to DataStorage deployment
2. Add necessary volumes (TLS cert, oauth-proxy secret)
3. Update Service to expose port 8443
4. Create ServiceAccount and RBAC for oauth-proxy SAR

### After Task 2:
1. Update handler logic to use `X-Auth-Request-User`
2. Update integration tests to mock new header
3. Update OpenAPI spec and regenerate clients
4. Run full test suite to verify

---

## 🔗 Related Documents

- [DD-AUTH-004](../../../architecture/decisions/DD-AUTH-004-openshift-oauth-proxy-legal-hold.md) - Design Decision
- [DD-AUTH-003](../../../architecture/decisions/DD-AUTH-003-externalized-authorization-sidecar.md) - Parent Pattern
- [GAP8_LEGAL_HOLD_COMPLETE_JAN06.md](./GAP8_LEGAL_HOLD_COMPLETE_JAN06.md) - Legal Hold Implementation
- [OpenShift oauth-proxy README](https://github.com/openshift/oauth-proxy/blob/master/README.md) - Authoritative Reference

---

## ⚠️ Critical Considerations

### For Production Deployment
1. **TLS Certificates**: Use real certificates, not self-signed (cert-manager or OpenShift service serving certs)
2. **Cookie Secret**: Generate strong random secret (32+ bytes)
3. **SAR Permissions**: Carefully scope required permissions (verb: update on services/datastorage)
4. **Network Policy**: Add network policy to enforce sidecar path

### For E2E Tests
1. **Kind Compatibility**: oauth-proxy works on Kind with service accounts
2. **Port Mapping**: May need to expose port 8443 via Kind extraPortMappings (per DD-TEST-001)
3. **Coverage Mode**: Ensure E2E coverage mode compatible with oauth-proxy sidecar

### For Integration Tests
1. **No oauth-proxy**: Integration tests mock `X-Auth-Request-User` header directly
2. **Defense-in-Depth Test**: Keep test for 401 when header missing
3. **Backwards Compatibility**: Integration tests should NOT need oauth-proxy running

---

**Status**: 1/7 tasks complete (14%)  
**Estimated Remaining Time**: 4-6 hours  
**Next Action**: Complete Task 2 (DataStorage K8s deployment with oauth-proxy sidecar)

