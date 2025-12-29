# 📋 Webhook Implementation Worklog - Next Sprint

**Sprint**: Post-Branch-Merge (after PR to main)
**Date**: December 20, 2025
**Prerequisites**: ✅ Webhook design documents merged to main
**Teams**: WE Team, RO Team
**Priority**: **P0 (CRITICAL)** - SOC2 v1.0 Compliance Requirement

---

## 🎯 **Sprint Goal**

Implement authenticated user webhooks for WorkflowExecution (WE) and RemediationOrchestrator (RO) services to achieve SOC2 Type II compliance (CC8.1 Attribution).

**Success Criteria**:
- ✅ WE webhook implemented and tested (BR-WE-013)
- ✅ RO webhook implemented and tested (ADR-040)
- ✅ SOC2 compliance validated for both services
- ✅ Must gather procedures implemented and documented

---

## 📊 **Sprint Overview**

| Sprint Phase | Duration | Owner | Deliverables |
|--------------|----------|-------|--------------|
| **Phase 1: WE Webhook** | 3-4 days | WE Team | Shared library + WE webhook + tests |
| **Phase 2: RO Webhook** | 2-3 days | RO Team | RO webhook + tests (reuses shared lib) |
| **Phase 3: SOC2 Validation** | 2 days | Both Teams | Compliance evidence + documentation |
| **Phase 4: Must Gather** | 1 day | Platform Team | Diagnostic tooling + runbooks |
| **Total** | **8-10 days** | Cross-team | Production-ready webhooks |

---

## 📋 **Task Breakdown**

### **Phase 1: WorkflowExecution (WE) Webhook Implementation** (WE Team)

#### **Day 1: Shared Library Development** (WE Team)

**Objective**: Create reusable authentication library for all webhook implementations.

**Tasks**:
- [ ] **T1.1**: Create `pkg/authwebhook/types.go` (~80 LOC)
  - Define `AuthContext` struct
  - Implement `String()` method for audit formatting
  - **Acceptance**: Type compiles without errors

- [ ] **T1.2**: Create `pkg/authwebhook/authenticator.go` (~100 LOC)
  - Implement `Authenticator` struct
  - Implement `ExtractUser(ctx, req)` method
  - Extract user from `req.UserInfo.Username` and `req.UserInfo.UID`
  - **Acceptance**: 8 unit tests pass

- [ ] **T1.3**: Create `pkg/authwebhook/validator.go` (~60 LOC)
  - Implement `ValidateReason(reason, minLength)` function
  - Implement `ValidateTimestamp(ts)` function
  - **Acceptance**: 4 unit tests pass

- [ ] **T1.4**: Create `pkg/authwebhook/audit.go` (~60 LOC)
  - Implement `AuditClient` wrapper
  - Implement `EmitAuthenticationEvent()` method
  - **Acceptance**: 4 unit tests pass

- [ ] **T1.5**: Create `pkg/authwebhook/suite_test.go`
  - Set up Ginkgo test suite
  - **Acceptance**: `go test ./pkg/authwebhook/...` passes

**Deliverable**: ✅ `pkg/authwebhook` shared library with 18 passing unit tests

**Reference**: [ADR-051 § Shared Library Implementation](../architecture/decisions/ADR-051-operator-sdk-webhook-scaffolding.md#step-2-implement-shared-library)

---

#### **Day 2: WE CRD Schema + Webhook Scaffolding** (WE Team)

**Objective**: Update WE CRD schema and scaffold webhook using operator-sdk.

**Tasks**:
- [ ] **T2.1**: Update `api/workflowexecution/v1alpha1/workflowexecution_types.go`
  - Add `BlockClearanceRequest` struct
  - Add `BlockClearanceDetails` struct
  - Add `blockClearanceRequest` field to `WorkflowExecutionStatus`
  - Add `blockClearance` field to `WorkflowExecutionStatus`
  - **Acceptance**: CRD compiles without errors

- [ ] **T2.2**: Regenerate CRD manifests
  ```bash
  make manifests
  ```
  - **Acceptance**: CRD YAML generated without errors

- [ ] **T2.3**: Scaffold webhook using operator-sdk
  ```bash
  kubebuilder create webhook \
      --group workflowexecution \
      --version v1alpha1 \
      --kind WorkflowExecution \
      --defaulting \
      --programmatic-validation
  ```
  - **Acceptance**: Webhook files generated

- [ ] **T2.4**: Review generated files
  - `api/workflowexecution/v1alpha1/workflowexecution_webhook.go`
  - `api/workflowexecution/v1alpha1/webhook_suite_test.go`
  - `config/webhook/manifests.yaml`
  - **Acceptance**: All files present and valid

**Deliverable**: ✅ WE CRD schema updated + webhook scaffolded

**Reference**: [ADR-051 § Step 1: Scaffold Webhook](../architecture/decisions/ADR-051-operator-sdk-webhook-scaffolding.md#step-1-scaffold-webhook-operator-sdk)

---

#### **Day 3: WE Webhook Implementation** (WE Team)

**Objective**: Implement authentication logic and mutual exclusion validation.

**Tasks**:
- [ ] **T3.1**: Implement `isControllerServiceAccount()` helper
  - Check for `system:serviceaccount:` prefix
  - Check for `workflowexecution-controller` in username
  - **Acceptance**: Helper function compiles

- [ ] **T3.2**: Implement `Default()` method (mutation)
  - Check if request is from controller SA → bypass
  - Check if `blockClearanceRequest` exists
  - Extract authenticated user using `pkg/authwebhook.Authenticator`
  - Populate `blockClearance` with authenticated fields
  - Clear `blockClearanceRequest` (consumed)
  - Emit audit event (best-effort)
  - **Acceptance**: Method compiles without errors

- [ ] **T3.3**: Implement `ValidateUpdate()` method (validation)
  - Check if request is from controller SA
    - If YES: Deny modification of `blockClearanceRequest` or `blockClearance`
    - If NO: Deny modification of controller-managed fields (phase, message, conditions)
  - Validate `blockClearanceRequest` if present
  - **Acceptance**: Method compiles without errors

- [ ] **T3.4**: Wire webhook in `cmd/workflowexecution/main.go`
  - Create Data Storage client
  - Call `SetupWebhookWithManager(mgr, dsClient)`
  - **Acceptance**: Main compiles without errors

**Deliverable**: ✅ WE webhook implementation complete

**Reference**: [ADR-051 § Step 3: Implement CRD Webhook](../architecture/decisions/ADR-051-operator-sdk-webhook-scaffolding.md#step-3-implement-crd-webhook-example-workflowexecution)

---

#### **Day 3-4: WE Webhook Testing** (WE Team)

**Objective**: Achieve 100% test coverage for WE webhook.

**Tasks**:
- [ ] **T3.5**: Write unit tests (18 tests)
  - [ ] Test controller SA bypasses authentication in `Default()`
  - [ ] Test controller SA bypasses validation for controller-managed fields
  - [ ] Test controller CANNOT modify `blockClearanceRequest`
  - [ ] Test controller CANNOT modify `blockClearance`
  - [ ] Test operator triggers authentication
  - [ ] Test users CANNOT modify `status.phase`
  - [ ] Test users CANNOT modify `status.message`
  - [ ] Test users CANNOT modify `status.conditions`
  - [ ] Test users CANNOT modify `blockClearance`
  - [ ] Test users CAN modify `blockClearanceRequest`
  - [ ] Test validation rejects empty `clearReason`
  - [ ] Test validation rejects short `clearReason` (<10 chars)
  - [ ] Test validation rejects future timestamps
  - [ ] Test webhook populates authenticated fields
  - [ ] Test webhook clears `blockClearanceRequest` after processing
  - [ ] Test bypass logging is observable
  - [ ] Test validation error messages are clear
  - [ ] Test mutual exclusion applies ONLY to Update operations
  - **Acceptance**: All 18 unit tests pass

- [ ] **T3.6**: Write integration tests (3 tests)
  - [ ] Test full webhook flow: request → authentication → clearance
  - [ ] Test audit event recorded with authenticated user
  - [ ] Test controller updates status without webhook interference
  - **Acceptance**: All 3 integration tests pass

- [ ] **T3.7**: Write E2E tests (2 tests)
  - [ ] Test operator clears block via `kubectl patch`
  - [ ] Test future executions allowed after clearance
  - **Acceptance**: All 2 E2E tests pass

**Deliverable**: ✅ WE webhook with 18 unit + 3 integration + 2 E2E tests

**Reference**: [ADR-051 § Testing Strategy](../architecture/decisions/ADR-051-operator-sdk-webhook-scaffolding.md#testing-strategy)

---

### **Phase 2: RemediationOrchestrator (RO) Webhook Implementation** (RO Team)

#### **Day 1: RO CRD Schema + Webhook Scaffolding** (RO Team)

**Objective**: Update RAR CRD schema and scaffold webhook.

**Tasks**:
- [ ] **T4.1**: Review `pkg/authwebhook` shared library (1 hour)
  - Understand `Authenticator.ExtractUser()` interface
  - Understand validator functions
  - **Acceptance**: Team understands shared library API

- [ ] **T4.2**: Update `api/remediationorchestrator/v1alpha1/remediationapprovalrequest_types.go`
  - Add `ApprovalRequest` struct
  - Add `approvalRequest` field to `RemediationApprovalRequestStatus`
  - Add `decision`, `decidedBy`, `decidedAt`, `decisionMessage` fields
  - **Acceptance**: CRD compiles without errors

- [ ] **T4.3**: Regenerate CRD manifests
  ```bash
  make manifests
  ```
  - **Acceptance**: CRD YAML generated without errors

- [ ] **T4.4**: Scaffold webhook using operator-sdk
  ```bash
  kubebuilder create webhook \
      --group remediationorchestrator \
      --version v1alpha1 \
      --kind RemediationApprovalRequest \
      --defaulting \
      --programmatic-validation
  ```
  - **Acceptance**: Webhook files generated

**Deliverable**: ✅ RAR CRD schema updated + webhook scaffolded

**Reference**: [ADR-051 § Step 1: Scaffold Webhook](../architecture/decisions/ADR-051-operator-sdk-webhook-scaffolding.md#step-1-scaffold-webhook-operator-sdk)

---

#### **Day 2: RO Webhook Implementation** (RO Team)

**Objective**: Implement authentication logic and mutual exclusion validation.

**Tasks**:
- [ ] **T5.1**: Implement `isControllerServiceAccount()` helper
  - Check for `remediationorchestrator-controller` in username
  - **Acceptance**: Helper function compiles

- [ ] **T5.2**: Implement `Default()` method (mutation)
  - Check if request is from controller SA → bypass
  - Check if `approvalRequest` exists
  - Extract authenticated user using `pkg/authwebhook.Authenticator`
  - Populate `decidedBy`, `decidedAt`, `decision`, `decisionMessage`
  - Clear `approvalRequest` (consumed)
  - Emit audit event
  - **Acceptance**: Method compiles without errors

- [ ] **T5.3**: Implement `ValidateUpdate()` method (validation)
  - Implement mutual exclusion (controller vs operator fields)
  - Validate `approvalRequest` if present
  - **Acceptance**: Method compiles without errors

- [ ] **T5.4**: Wire webhook in `cmd/remediationorchestrator/main.go`
  - Create Data Storage client
  - Call `SetupWebhookWithManager(mgr, dsClient)`
  - **Acceptance**: Main compiles without errors

**Deliverable**: ✅ RO webhook implementation complete

**Reference**: [INDEPENDENT_WEBHOOK_NOTIFICATION_TO_RO_TEAM_DEC_20_2025.md](./INDEPENDENT_WEBHOOK_NOTIFICATION_TO_RO_TEAM_DEC_20_2025.md)

---

#### **Day 2-3: RO Webhook Testing** (RO Team)

**Objective**: Achieve 100% test coverage for RO webhook.

**Tasks**:
- [ ] **T5.5**: Write unit tests (18 tests)
  - Same test pattern as WE webhook
  - Adapted for RAR-specific fields
  - **Acceptance**: All 18 unit tests pass

- [ ] **T5.6**: Write integration tests (3 tests)
  - Test full approval workflow
  - Test audit event recorded
  - Test controller updates status without interference
  - **Acceptance**: All 3 integration tests pass

- [ ] **T5.7**: Write E2E tests (2 tests)
  - Test operator approves remediation via `kubectl patch`
  - Test operator rejects remediation via `kubectl patch`
  - **Acceptance**: All 2 E2E tests pass

**Deliverable**: ✅ RO webhook with 18 unit + 3 integration + 2 E2E tests

---

### **Phase 3: SOC2 Compliance Validation** (Both Teams)

#### **Day 1-2: Compliance Evidence Collection** (Both Teams)

**Objective**: Validate SOC2 controls and collect compliance evidence.

**Tasks**:
- [ ] **T6.1**: Validate CC8.1 (Attribution) for WE
  - Test authenticated user captured in audit events
  - Verify K8s authentication context used (not user input)
  - **Acceptance**: All block clearances show authenticated user

- [ ] **T6.2**: Validate CC8.1 (Attribution) for RO
  - Test authenticated user captured in approval decisions
  - Verify K8s authentication context used
  - **Acceptance**: All approval decisions show authenticated user

- [ ] **T6.3**: Validate CC7.3 (Immutability) for WE
  - Test original failed WFE preserved (not deleted)
  - **Acceptance**: Failed WFEs remain in cluster after clearance

- [ ] **T6.4**: Validate CC7.4 (Completeness) for WE + RO
  - Test 100% audit trail coverage
  - Verify no gaps in audit events
  - **Acceptance**: All auth operations have audit events

- [ ] **T6.5**: Validate CC4.2 (Change Tracking) for WE + RO
  - Test WHO made changes tracked in audit trail
  - **Acceptance**: All audit events show authenticated actor

- [ ] **T6.6**: Validate CC6.1 (Integrity) for WE + RO
  - Test mutual exclusion prevents field forgery
  - Test users cannot modify controller-managed fields
  - Test controller cannot modify operator-managed fields
  - **Acceptance**: All forgery attempts rejected

- [ ] **T6.7**: Document compliance evidence
  - Create compliance report for each control
  - Include test results and audit event samples
  - **Acceptance**: Compliance documentation complete

**Deliverable**: ✅ SOC2 compliance validated with evidence

**Reference**: [DD-WEBHOOK-001 § SOC2 Compliance Requirements](../architecture/decisions/DD-WEBHOOK-001-crd-webhook-requirements-matrix.md#soc2-compliance-requirements)

---

### **Phase 4: Must Gather Implementation** (Platform Team)

#### **Day 1: Diagnostic Tooling** (Platform Team)

**Objective**: Implement diagnostic tooling for webhook troubleshooting.

**Tasks**:
- [ ] **T7.1**: Create `scripts/must-gather-webhook.sh`
  - Collect webhook logs
  - Collect CRD events
  - Collect webhook configuration
  - Collect certificate status
  - Collect audit events
  - **Acceptance**: Script runs without errors

- [ ] **T7.2**: Create webhook troubleshooting runbook
  - Document common webhook issues
  - Document diagnostic steps
  - Document resolution procedures
  - **Acceptance**: Runbook reviewed by teams

- [ ] **T7.3**: Test must gather script
  - Test on working webhook
  - Test on broken webhook scenarios
  - **Acceptance**: Script collects all required data

**Deliverable**: ✅ Must gather tooling + runbooks

**Reference**: [DD-WEBHOOK-001 § Must Gather](../architecture/decisions/DD-WEBHOOK-001-crd-webhook-requirements-matrix.md#must-gather---webhook-troubleshooting)

---

## 📊 **Dependencies**

| Task | Depends On | Blocking |
|------|-----------|----------|
| T1.x (Shared Library) | None | T3.2 (WE Default()), T5.2 (RO Default()) |
| T2.x (WE Scaffolding) | None | T3.x (WE Implementation) |
| T3.x (WE Implementation) | T1.x (Shared Library) | T4.1 (RO Review) |
| T4.x (RO Scaffolding) | T1.x (Shared Library) | T5.x (RO Implementation) |
| T5.x (RO Implementation) | T1.x (Shared Library), T3.x (WE Complete) | T6.x (SOC2 Validation) |
| T6.x (SOC2 Validation) | T3.x (WE Complete), T5.x (RO Complete) | None |
| T7.x (Must Gather) | T3.x (WE Complete) | None (parallel) |

---

## 📋 **Acceptance Criteria (Sprint Success)**

### **WE Webhook** ✅
- [ ] CRD schema updated with auth fields
- [ ] Webhook scaffolded and implemented
- [ ] Controller bypass implemented (both methods)
- [ ] Mutual exclusion validation implemented
- [ ] 18 unit tests passing
- [ ] 3 integration tests passing
- [ ] 2 E2E tests passing
- [ ] SOC2 CC8.1, CC7.3, CC7.4, CC4.2, CC6.1 validated

### **RO Webhook** ✅
- [ ] CRD schema updated with auth fields
- [ ] Webhook scaffolded and implemented
- [ ] Controller bypass implemented (both methods)
- [ ] Mutual exclusion validation implemented
- [ ] 18 unit tests passing
- [ ] 3 integration tests passing
- [ ] 2 E2E tests passing
- [ ] SOC2 CC8.1, CC7.3, CC7.4, CC4.2, CC6.1 validated

### **Shared Library** ✅
- [ ] `pkg/authwebhook` package created
- [ ] 18 unit tests passing
- [ ] Used by both WE and RO webhooks (no code duplication)

### **SOC2 Compliance** ✅
- [ ] All 5 SOC2 controls validated for both services
- [ ] Compliance evidence documented
- [ ] Audit trail completeness verified (100%)

### **Must Gather** ✅
- [ ] Diagnostic script created and tested
- [ ] Troubleshooting runbook documented
- [ ] Platform team trained on diagnostic procedures

---

## 🎯 **Sprint Risks**

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|-----------|
| Operator-SDK scaffolding issues | Low | High | Review Kubebuilder docs; WE team goes first |
| Shared library API changes | Medium | Medium | Freeze API after Day 1; versioning |
| Certificate manager setup | Medium | High | Pre-validate cert-manager in staging |
| Audit event integration | Low | Medium | Data Storage API already exists |
| SOC2 compliance gaps | Low | Critical | Early validation (Day 4-5) |

---

## 📚 **Required Reading (Before Sprint Start)**

### **All Teams** (MANDATORY)
1. **[DD-WEBHOOK-001: CRD Webhook Requirements Matrix](../architecture/decisions/DD-WEBHOOK-001-crd-webhook-requirements-matrix.md)** - WHEN/WHY webhooks needed
2. **[ADR-051: Operator-SDK Webhook Scaffolding](../architecture/decisions/ADR-051-operator-sdk-webhook-scaffolding.md)** - HOW to implement webhooks
3. **[WEBHOOK_CONTROLLER_BYPASS_PATTERN_DEC_20_2025.md](./WEBHOOK_CONTROLLER_BYPASS_PATTERN_DEC_20_2025.md)** - Controller bypass + mutual exclusion

### **WE Team**
4. **[BR-WE-013: Audit-Tracked Block Clearing](../requirements/BR-WE-013-audit-tracked-block-clearing.md)** - WE business requirements

### **RO Team**
5. **[INDEPENDENT_WEBHOOK_NOTIFICATION_TO_RO_TEAM_DEC_20_2025.md](./INDEPENDENT_WEBHOOK_NOTIFICATION_TO_RO_TEAM_DEC_20_2025.md)** - RO webhook notification

---

## 📞 **Sprint Coordination**

### **Daily Standup (Required)**
- **Time**: 9:00 AM daily
- **Duration**: 15 minutes
- **Attendees**: WE Team, RO Team, Platform Team
- **Focus**: Dependencies, blockers, progress

### **Mid-Sprint Review** (Day 4)
- **Purpose**: Validate WE webhook complete before RO starts Day 2
- **Attendees**: Both teams + Product Owner
- **Decision Point**: RO team proceeds to implementation

### **Sprint Demo** (Day 8-10)
- **Purpose**: Demonstrate working webhooks to stakeholders
- **Attendees**: All teams + Product Owner + SOC2 Auditor (optional)
- **Demo**: Live `kubectl patch` with authentication

---

## ✅ **Definition of Done**

Sprint is complete when:
- ✅ All acceptance criteria met
- ✅ All unit/integration/E2E tests passing
- ✅ SOC2 compliance validated with documented evidence
- ✅ Must gather tooling implemented and tested
- ✅ Documentation updated (runbooks, architecture docs)
- ✅ Code reviewed and merged to main
- ✅ Sprint demo completed successfully

---

## 📅 **Timeline Summary**

```
Day 1:  [WE: Shared Library]
Day 2:  [WE: Scaffolding]
Day 3:  [WE: Implementation + Testing]
Day 4:  [WE: Testing Complete] → Mid-Sprint Review ✅
Day 5:  [RO: Scaffolding + Shared Lib Review]
Day 6:  [RO: Implementation]
Day 7:  [RO: Testing]
Day 8:  [SOC2 Validation - Both Teams]
Day 9:  [SOC2 Documentation + Must Gather]
Day 10: [Sprint Demo] → Sprint Complete ✅
```

---

**Document Status**: ✅ **READY FOR SPRINT PLANNING**
**Prerequisites**: All design documents merged to main via PR
**Sprint Start**: Post-branch-merge to main
**Estimated Velocity**: 8-10 days (2 weeks)

