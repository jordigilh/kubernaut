# RO Routing Scenarios Breakdown - needsHumanReview vs Unmanaged Resource

**Date**: January 20, 2026
**Purpose**: Clarify when `needsHumanReview` and "unmanaged resource" scenarios intersect in RO routing
**Status**: ✅ Authoritative Breakdown

---

## 🎯 **The Two Separate Scenarios**

### **Scenario A: `needsHumanReview=true`** (HAPI Decision - AI Reliability Issue)

**Source**: BR-HAPI-197 (Human Review Required Flag)

**Trigger**: HolmesGPT-API (HAPI) cannot produce a reliable remediation recommendation

**Reasons** (6 scenarios):
1. **Workflow Not Found**: LLM selected a workflow that doesn't exist in catalog
2. **Container Image Mismatch**: LLM-provided image doesn't match catalog
3. **Parameter Validation Failed**: Parameters don't conform to workflow schema
4. **No Workflows Matched**: Workflow search returned no results
5. **Low Confidence**: AI confidence below threshold
6. **LLM Parsing Error**: Cannot parse LLM response into structured format

**Key Characteristics**:
- ❌ **NO remediation plan exists** - AI couldn't produce reliable result
- ❌ **NO WorkflowExecution can be created** - nothing to execute
- ✅ **Human intervention required** - operator must manually decide next steps
- ✅ **Operator receives notification** - NotificationRequest CRD created

**RO Action**:
- Create **NotificationRequest** CRD
- **STOP** routing (no further checks)
- **NO retry/requeue** - manual intervention required

---

### **Scenario B: Unmanaged Resource** (Scope Validation)

**Source**: BR-SCOPE-001 (Resource Scope Management), BR-SCOPE-010 (RO Routing Validation)

**Trigger**: Target resource (RCA-determined or signal source) is NOT managed by Kubernaut

**Detection**: Resource/namespace missing `kubernaut.ai/managed=true` label

**Key Characteristics**:
- ✅ **Remediation plan exists** - AI produced a valid workflow recommendation
- ✅ **WorkflowExecution CAN be created** - but blocked by scope policy
- ✅ **Automatic retry enabled** - RO rechecks periodically (exponential backoff)
- ✅ **Operator receives notification** - NotificationRequest CRD created (configurable)

**RO Action**:
- **Block** RemediationRequest (status: `Blocked`, reason: `UnmanagedResource`)
- **Schedule automatic retry** - recheck scope every 5 minutes (configurable), exponential backoff
- Create **NotificationRequest** CRD (opt-out configurable)
- **Requeue** - RO re-reconciles until:
  - Resource becomes managed (label added) → Proceed to WorkflowExecution
  - RR timeout reached → Close RR
  - Operator manually closes RR

---

## 🔀 **Do These Scenarios Intersect?**

### **Short Answer: NO - They are mutually exclusive routing checks**

**Why?**

RO routing checks are **sequential** and **early-exit**:

```
Check 6: needsHumanReview Check (BR-HAPI-197)
├─ needsHumanReview = true?
│   ├─ YES → Create NotificationRequest + STOP ❌ (DO NOT continue to Check 8)
│   └─ NO → Continue to Check 7

Check 7: approvalRequired Check
├─ approvalRequired = true?
│   ├─ YES → Create RemediationApprovalRequest + STOP ❌
│   └─ NO → Continue to Check 8

Check 8: Scope Validation Check (BR-SCOPE-010) ✅ ONLY REACHED IF CHECKS 6 & 7 PASSED
├─ Is RCA target resource managed?
│   ├─ YES → Create WorkflowExecution
│   └─ NO → Block + Retry
```

**Key Insight**: **Scope validation (Check 8) is ONLY checked when:**
- ✅ `needsHumanReview=false` (Check 6 passed)
- ✅ `approvalRequired=false` (Check 7 passed)

**If `needsHumanReview=true`:**
- ❌ RO **does NOT** check scope (Check 8 skipped)
- ❌ RO **does NOT** requeue/retry
- ✅ RO creates NotificationRequest and **stops**

---

## 📊 **Complete RO Routing Decision Matrix**

### **All Possible Combinations**

|| `needsHumanReview` | `approvalRequired` | Resource Managed? | RO Action | Requeue? | Outcome |
||------|-----|---|---|---|---|
|| **1** | `true` | `false` | N/A (not checked) | Create NotificationRequest | ❌ NO | Manual review required |
|| **2** | `true` | `true` | N/A (not checked) | Create NotificationRequest | ❌ NO | Manual review required (precedence) |
|| **3** | `false` | `true` | N/A (not checked yet) | Create RemediationApprovalRequest | ❌ NO | Awaiting approval |
|| **4** | `false` | `false` | ✅ **YES** | Create WorkflowExecution | ❌ NO | Automatic remediation |
|| **5** | `false` | `false` | ❌ **NO** | Block + Retry | ✅ **YES** | Exponential backoff retry |

---

## 🔍 **Detailed Scenario Breakdowns**

### **Scenario 1: `needsHumanReview=true`, `approvalRequired=false`**

**Example**: HAPI couldn't find a matching workflow for the alert type

```yaml
AIAnalysis:
  status:
    phase: "Failed"
    needsHumanReview: true
    humanReviewReason: "no_workflows_matched"
    approvalRequired: false
```

**RO Routing**:
1. Check 6: `needsHumanReview=true` → **STOP HERE**
2. **Create NotificationRequest**:
   ```yaml
   NotificationRequest:
     spec:
       notificationType: "human_review_required"
       message: "No matching workflows found for alert type. Manual intervention required."
       correlationId: "<RR.UID>"
   ```
3. **Update RR Status**:
   ```yaml
   RemediationRequest:
     status:
       phase: "RequiresReview"
       reason: "HumanReviewRequired"
   ```
4. **NO scope check** (Check 8 never reached)
5. **NO requeue** (RO stops, awaits manual intervention)

**Why No Scope Check?**
- No remediation plan exists (AI couldn't produce one)
- Nothing to validate scope against (no WorkflowExecution to create)
- Operator will manually decide next steps (including checking scope if needed)

---

### **Scenario 2: `needsHumanReview=true`, `approvalRequired=true` (Edge Case)**

**Example**: HAPI has low confidence, AND Rego policy requires approval

```yaml
AIAnalysis:
  status:
    phase: "Completed"
    needsHumanReview: true         # HAPI decision: low confidence
    humanReviewReason: "low_confidence"
    approvalRequired: true         # Rego decision: high-risk action
    approvalReason: "production_namespace"
    selectedWorkflow:
      workflowId: "delete-pvc-v1"  # AI has a plan, but unreliable
```

**RO Routing**:
1. Check 6: `needsHumanReview=true` → **STOP HERE** (takes precedence)
2. **Create NotificationRequest** (NOT RemediationApprovalRequest):
   ```yaml
   NotificationRequest:
     spec:
       notificationType: "human_review_required"
       message: "AI confidence below threshold. Manual intervention required."
       additionalContext: "Note: Rego policy also flagged this as high-risk (production_namespace)"
       correlationId: "<RR.UID>"
   ```
3. **Update RR Status**: `phase="RequiresReview"`, `reason="HumanReviewRequired"`
4. **NO RemediationApprovalRequest created** (approval is moot if AI result is unreliable)
5. **NO scope check**
6. **NO requeue**

**Rationale**: **AI reliability issues (`needsHumanReview`) take precedence over policy decisions (`approvalRequired`)** - if AI can't produce a reliable result, there's no point in approving an unreliable plan.

---

### **Scenario 3: `needsHumanReview=false`, `approvalRequired=true`**

**Example**: HAPI has high confidence, but Rego policy requires approval

```yaml
AIAnalysis:
  status:
    phase: "Completed"
    needsHumanReview: false
    approvalRequired: true
    approvalReason: "high_risk_action"
    selectedWorkflow:
      workflowId: "restart-deployment-v1"
      confidence: 0.95
```

**RO Routing**:
1. Check 6: `needsHumanReview=false` → Continue
2. Check 7: `approvalRequired=true` → **STOP HERE**
3. **Create RemediationApprovalRequest**:
   ```yaml
   RemediationApprovalRequest:
     spec:
       workflowId: "restart-deployment-v1"
       reason: "high_risk_action"
       correlationId: "<RR.UID>"
   ```
4. **Update RR Status**: `phase="AwaitingApproval"`, `reason="ApprovalRequired"`
5. **NO scope check** (Check 8 not reached until approval granted)
6. **NO requeue** (awaiting human approval)

**When Approved**:
- Operator approves → RO re-reconciles → Check 6 & 7 pass → **NOW Check 8 (scope validation) runs**
- If resource is unmanaged at approval time → Block + Retry

---

### **Scenario 4: `needsHumanReview=false`, `approvalRequired=false`, Resource **MANAGED****

**Example**: Normal automatic remediation path

```yaml
AIAnalysis:
  status:
    phase: "Completed"
    needsHumanReview: false
    approvalRequired: false
    selectedWorkflow:
      workflowId: "restart-pod-v1"
      confidence: 0.85
    rootCauseAnalysis:
      targetResource:
        kind: "Pod"
        name: "crashloop-pod"
        namespace: "production"  # Has kubernaut.ai/managed=true label
```

**RO Routing**:
1. Check 6: `needsHumanReview=false` → Continue
2. Check 7: `approvalRequired=false` → Continue
3. **Check 8: Scope Validation**:
   - Get AIAnalysis.Status.RootCauseAnalysis.TargetResource
   - Check if `production/Pod/crashloop-pod` is managed
   - **Result: ✅ MANAGED** (namespace has `kubernaut.ai/managed=true`)
4. **Create WorkflowExecution**:
   ```yaml
   WorkflowExecution:
     spec:
       workflowId: "restart-pod-v1"
       targetResource:
         kind: "Pod"
         name: "crashloop-pod"
         namespace: "production"
   ```
5. **Update RR Status**: `phase="InProgress"`, `reason="WorkflowExecuting"`
6. **NO requeue** (WorkflowExecution takes over)

---

### **Scenario 5: `needsHumanReview=false`, `approvalRequired=false`, Resource **UNMANAGED****

**Example**: Automatic remediation path, but resource not managed

```yaml
AIAnalysis:
  status:
    phase: "Completed"
    needsHumanReview: false
    approvalRequired: false
    selectedWorkflow:
      workflowId: "restart-pod-v1"
      confidence: 0.85
    rootCauseAnalysis:
      targetResource:
        kind: "Pod"
        name: "test-pod"
        namespace: "development"  # Missing kubernaut.ai/managed label
```

**RO Routing**:
1. Check 6: `needsHumanReview=false` → Continue
2. Check 7: `approvalRequired=false` → Continue
3. **Check 8: Scope Validation**:
   - Get AIAnalysis.Status.RootCauseAnalysis.TargetResource
   - Check if `development/Pod/test-pod` is managed
   - **Result: ❌ UNMANAGED** (namespace missing label)
4. **Block RemediationRequest**:
   ```yaml
   RemediationRequest:
     status:
       overallPhase: "Blocked"
       blockReason: "UnmanagedResource"
       blockMessage: "Resource development/Pod/test-pod is not managed by Kubernaut. Add label 'kubernaut.ai/managed=true' to namespace development to enable remediation."
   ```
5. **Create NotificationRequest** (configurable):
   ```yaml
   NotificationRequest:
     spec:
       notificationType: "remediation_blocked"
       message: "Remediation blocked: resource not managed"
       correlationId: "<RR.UID>"
   ```
6. **Schedule Automatic Retry** (Exponential Backoff):
   - T+0m: Block detected → Retry in 5 seconds
   - T+5s: Retry #1 → Still unmanaged? → Retry in 10 seconds
   - T+15s: Retry #2 → Still unmanaged? → Retry in 20 seconds
   - T+35s: Retry #3 → Still unmanaged? → Retry in 40 seconds
   - ...continues until:
     - ✅ Resource becomes managed (label added) → Create WorkflowExecution
     - ❌ RR timeout reached → Close RR
     - ❌ Operator manually closes RR
7. **✅ YES REQUEUE** - RO re-reconciles every X seconds

**Why Retry?**
- Remediation plan exists (AI produced valid workflow)
- Operator can make resource managed by adding label
- Automatic retry provides good UX (no manual trigger needed)
- Scope can change at any time (temporal drift protection)

---

## 🎯 **Key Architectural Insights**

### **1. Sequential Checks with Early Exit**

```
Check 6 (needsHumanReview) → IF true, STOP (no further checks)
  ↓ (if false)
Check 7 (approvalRequired) → IF true, STOP (no further checks)
  ↓ (if false)
Check 8 (Scope Validation) → IF unmanaged, Block + Retry
  ↓ (if managed)
Create WorkflowExecution
```

**Implication**: `needsHumanReview=true` **prevents** scope validation from running.

---

### **2. Two Types of "Manual Intervention Required"**

|| Scenario | AI State | Remediation Plan | Next Steps | Requeue? |
||----------|----------|------------------|-----------|----------|
|| **`needsHumanReview=true`** | AI **cannot** answer | ❌ NO plan exists | Operator manually decides | ❌ NO |
|| **Unmanaged resource** | AI **has** answer | ✅ Plan exists but blocked | Operator adds label → auto-proceeds | ✅ YES |

**Key Difference**:
- `needsHumanReview` = **AI problem** (cannot produce reliable result)
- Unmanaged resource = **Policy problem** (AI has valid plan, but resource not opted-in)

---

### **3. Retry Behavior**

|| Scenario | Requeue? | Retry Mechanism | Duration |
||----------|----------|-----------------|----------|
|| **`needsHumanReview=true`** | ❌ NO | N/A | Manual intervention (no timeout) |
|| **`approvalRequired=true`** | ❌ NO | N/A | Awaiting approval (no timeout) |
|| **Unmanaged resource** | ✅ **YES** | Exponential backoff | 5s → 10s → 20s → 40s → ... until RR timeout |

---

### **4. Notification Behavior**

|| Scenario | Notification Type | Message |
||----------|-------------------|---------|
|| **`needsHumanReview=true`** | `"human_review_required"` | "AI cannot produce reliable result: [reason]" |
|| **Unmanaged resource** | `"remediation_blocked"` (opt-out) | "Remediation blocked: resource not managed" |

---

## 📋 **Summary: When Do They Intersect?**

### **Answer: They DON'T intersect**

**Routing Logic**:
- If `needsHumanReview=true` → RO **never checks scope** (Check 8 skipped)
- If `needsHumanReview=false` + `approvalRequired=false` → RO **checks scope** (Check 8 runs)

**Mutual Exclusivity**:
- You **cannot** have both `needsHumanReview=true` AND scope validation in the same routing pass
- `needsHumanReview` takes precedence (checked first)

**Edge Case**: After NotificationRequest is sent (for `needsHumanReview=true`):
- RR status = `RequiresReview`
- **NO automatic requeue** - RO does not re-check scope
- **NO automatic retry** - awaits manual intervention
- Operator responsibility:
  - Review situation manually
  - Check if resource is managed (if relevant)
  - Make resource managed (if needed)
  - Close RR (standard practice)

---

## ✅ **Correct Understanding**

### **What Happens When `needsHumanReview=true` and Resource is Unmanaged?**

1. **RO Check 6**: `needsHumanReview=true` → **STOP**
2. **RO Action**: Create NotificationRequest
3. **RO does NOT check scope** - Check 8 never reached
4. **RO does NOT requeue** - manual intervention required
5. **Operator receives notification**: "AI cannot produce reliable result"
6. **Operator manually reviews**:
   - Investigates why AI failed
   - Checks if resource is managed (part of manual review)
   - Makes resource managed if needed
   - Decides on next steps (manual remediation, close RR, etc.)
7. **RR closes** (standard practice for `RequiresReview` phase)

**No intersection** - scope validation is irrelevant when AI can't produce a remediation plan.

---

**Document Status**: ✅ Complete Breakdown
**Created**: January 20, 2026 (Evening)
**Purpose**: Clarify RO routing scenarios to resolve confusion between `needsHumanReview` and unmanaged resource handling
**Authoritative Sources**: BR-HAPI-197, BR-SCOPE-001, BR-SCOPE-010
