# DD-TEST-001 Alignment Analysis

**Date**: December 22, 2025
**Status**: 🚨 **MISALIGNMENT FOUND**
**Issue**: Proposed ports do NOT fully align with DD-TEST-001

---

## 🚨 **CRITICAL FINDING: Services Missing from DD-TEST-001**

### **Services with FULL DD-TEST-001 Documentation**

| Service | DD-TEST-001 Detailed Section | Ports Documented |
|---------|------------------------------|------------------|
| **DataStorage** | ✅ Lines 83-128 | PostgreSQL: 15433, Redis: 16379, API: 18090 |
| **Gateway** | ✅ Lines 131-168 | Redis: 16380, API: 18080, DataStorage: 18091 |
| **Effectiveness Monitor** | ✅ Lines 172-208 | PostgreSQL: 15434, API: 18100, DataStorage: 18092 |
| **Workflow Engine** | ✅ Lines 212-238 | API: 18110, DataStorage: 18093 |
| **SignalProcessing** | ✅ Lines 242-263 | PostgreSQL: 15436, Redis: 16382, DataStorage: 18094 |

### **Services MISSING from DD-TEST-001 Detailed Sections**

| Service | Status in DD-TEST-001 | Current Code Ports |
|---------|----------------------|-------------------|
| **RemediationOrchestrator** | ⚠️ Note at line 580: "undocumented, requires DD-TEST-001 update" | PostgreSQL: 15435, Redis: 16381, DataStorage: 18140 |
| **Notification** | ❌ NOT documented | Unknown (no infrastructure yet) |
| **AIAnalysis** | ❌ NOT documented in Integration section | PostgreSQL: 15434, Redis: 16380, DataStorage: 18091 |
| **WorkflowExecution** | ❌ Only E2E documented | Unknown (no integration tests?) |

---

## 🔍 **Port Conflict Analysis**

### **Issue 1: AIAnalysis Conflicts**

**DD-TEST-001 says:**
- Gateway should use Redis: **16380**
- Gateway DataStorage dependency: **18091**

**Current code has:**
- AIAnalysis using Redis: **16380** ⚠️ **CONFLICTS WITH GATEWAY**
- AIAnalysis using DataStorage: **18091** ⚠️ **CONFLICTS WITH GATEWAY**

**ROOT CAUSE**: AIAnalysis is using ports that DD-TEST-001 allocated to Gateway!

### **Issue 2: Gateway Itself Not Following DD-TEST-001**

**DD-TEST-001 says Gateway should use:**
- Redis: **16380**
- Gateway API: **18080**
- DataStorage dependency: **18091**

**Current code has Gateway using:**
- Redis: **16383** ⚠️ **MISMATCH**
- Gateway API: **Not in integration tests** (Gateway is stateless, just depends on DataStorage)
- DataStorage: **18091** ✅ **CORRECT**

**ROOT CAUSE**: Gateway implementation doesn't match DD-TEST-001!

### **Issue 3: Effectiveness Monitor vs AIAnalysis PostgreSQL**

**DD-TEST-001 says:**
- Effectiveness Monitor: PostgreSQL **15434**

**Current code has:**
- AIAnalysis: PostgreSQL **15434** ⚠️ **POTENTIAL CONFLICT**

**Question**: Does AIAnalysis actually need PostgreSQL, or is it just using DataStorage?

---

## 🤔 **FUNDAMENTAL QUESTION**

### **Which Services ACTUALLY Need Full Infrastructure?**

**Services that run DataStorage instances** (need PostgreSQL + Redis + DataStorage):
- ✅ DataStorage service itself (reference implementation)
- ✅ Gateway (confirmed - current code has full stack)
- ✅ SignalProcessing (confirmed - DD-TEST-001 documents full stack)
- ✅ RemediationOrchestrator (confirmed - current code has full stack)
- ❓ AIAnalysis (current code has full stack, but NOT in DD-TEST-001)
- ❓ Notification (unknown - no infrastructure code found)
- ❓ WorkflowExecution (unknown - no integration tests found)

**Services that only consume DataStorage** (just need DataStorage URL):
- ✅ Effectiveness Monitor (DD-TEST-001: has own PostgreSQL but uses DataStorage dependency)
- ✅ Workflow Engine (DD-TEST-001: only lists DataStorage dependency 18093)

---

## 📋 **CORRECT APPROACH: Follow DD-TEST-001 Exactly**

### **Step 1: Validate Which Services Need Full DS Infrastructure**

Before allocating ports, we need to answer:

1. **AIAnalysis**: Does it run its own DataStorage instance?
   - If YES: Needs PostgreSQL + Redis + DataStorage ports
   - If NO: Just needs DataStorage URL from another service

2. **Notification**: Does it run its own DataStorage instance?
   - If YES: Needs full stack
   - If NO: Can use shared DataStorage

3. **WorkflowExecution**: Does it need integration tests with DataStorage?
   - If YES: Needs full stack
   - If NO: May only need E2E tests

### **Step 2: Align Current Code with DD-TEST-001**

**For services documented in DD-TEST-001:**

| Service | Action Required |
|---------|----------------|
| **Gateway** | Fix Redis port: 16383 → **16380** (per DD-TEST-001 line 137) |
| **DataStorage** | ✅ Already correct |
| **SignalProcessing** | ✅ Already correct |
| **Effectiveness Monitor** | Check if implemented, align with DD-TEST-001 |
| **Workflow Engine** | Check if implemented, align with DD-TEST-001 |

**For services NOT documented in DD-TEST-001:**

| Service | Action Required |
|---------|----------------|
| **RemediationOrchestrator** | Keep current ports (15435/16381/18140), **UPDATE DD-TEST-001** |
| **AIAnalysis** | Investigate infrastructure needs, then allocate ports |
| **Notification** | Investigate infrastructure needs, then allocate ports |
| **WorkflowExecution** | Investigate infrastructure needs, then allocate ports |

### **Step 3: Update DD-TEST-001 with Missing Services**

Add detailed sections for:
- RemediationOrchestrator (note at line 580 says this is needed)
- AIAnalysis (if it needs full infrastructure)
- Notification (if it needs full infrastructure)
- WorkflowExecution (if it needs integration tests)

---

## 🎯 **RECOMMENDED: Investigation First, Then Port Allocation**

### **Investigation Questions for User**

1. **AIAnalysis**:
   - Does AIAnalysis run its own DataStorage instance in integration tests?
   - Or does it connect to a shared DataStorage?
   - Why does current code use 15434/16380/18091 (conflicts with Gateway + EffectivenessMonitor)?

2. **Notification**:
   - Does Notification need to run DataStorage in integration tests?
   - Or does it only connect to DataStorage for audit events?

3. **WorkflowExecution**:
   - Does WorkflowExecution have integration tests?
   - If yes, does it need its own DataStorage instance?

4. **Gateway Integration Tests**:
   - Why is Gateway using Redis 16383 instead of 16380 (DD-TEST-001 allocation)?
   - Was this intentional or an oversight?

---

## ✅ **CORRECTED: DD-TEST-001 Compliant Port Matrix**

### **Services with Full DD-TEST-001 Documentation** (AUTHORITATIVE)

| Service | PostgreSQL | Redis | DataStorage | Source |
|---------|------------|-------|-------------|--------|
| **DataStorage** | 15433 | 16379 | 18090 | DD-TEST-001 lines 83-105 |
| **Gateway** | N/A | **16380** | 18091 | DD-TEST-001 lines 131-150 |
| **Effectiveness Monitor** | 15434 | N/A | 18092 | DD-TEST-001 lines 172-190 |
| **Workflow Engine** | N/A | N/A | 18093 | DD-TEST-001 lines 212-225 |
| **SignalProcessing** | 15436 | 16382 | 18094 | DD-TEST-001 lines 242-263 |

### **Services Documented in Port Collision Matrix** (Line 569-580)

| Service | PostgreSQL | Redis | API | DataStorage Dep | Status |
|---------|------------|-------|-----|-----------------|--------|
| **DataStorage** | 15433 | 16379 | 18090 | N/A | ✅ Reference |
| **Gateway** | N/A | 16380 | 18080 | 18091 | ⚠️ Code uses Redis 16383 |
| **Effectiveness Monitor** | 15434 | N/A | 18100 | 18092 | ❓ Need to verify code |
| **Workflow Engine** | N/A | N/A | 18110 | 18093 | ❓ Need to verify code |
| **SignalProcessing** | 15436 | 16382 | N/A | 18094 | ✅ Code matches |

### **Undocumented Services** (Need DD-TEST-001 Update)

| Service | Current Code Ports | DD-TEST-001 Status |
|---------|-------------------|-------------------|
| **RemediationOrchestrator** | PostgreSQL: 15435, Redis: 16381, DataStorage: 18140 | "undocumented, requires DD-TEST-001 update" (line 580) |
| **AIAnalysis** | PostgreSQL: 15434, Redis: 16380, DataStorage: 18091 | ❌ NOT documented, **CONFLICTS** |
| **Notification** | Unknown | ❌ NOT documented |
| **WorkflowExecution** | Unknown | ❌ NOT documented |

---

## 🚨 **BLOCKER: Cannot Proceed Without Clarification**

**I cannot finalize port allocations without answering:**

1. **Which services actually need full DataStorage infrastructure** (PostgreSQL + Redis + DataStorage)?
2. **Why is AIAnalysis using ports allocated to other services** in DD-TEST-001?
3. **Why is Gateway using Redis 16383** instead of 16380 (DD-TEST-001 allocation)?
4. **Should we follow existing code** or **follow DD-TEST-001 document**?

---

## 🎯 **PROPOSED: Two-Step Approach**

### **Step 1: Audit Current Infrastructure** (Investigation)

For each service, determine:
- ✅ Does it run DataStorage in integration tests? (YES/NO)
- ✅ What ports is it currently using? (Actual code)
- ✅ What ports does DD-TEST-001 say it should use? (Document)
- ✅ Are there conflicts? (YES/NO)

### **Step 2: Resolve Conflicts** (Based on Audit)

**Option A: Follow DD-TEST-001 Strictly**
- Change code to match DD-TEST-001 allocations
- Update any services not in DD-TEST-001
- Resolve all conflicts in favor of DD-TEST-001

**Option B: Update DD-TEST-001 to Match Code**
- Document current code allocations in DD-TEST-001
- Add missing services to DD-TEST-001
- Resolve conflicts by reallocating ports in DD-TEST-001

**Option C: Hybrid Approach**
- Keep proven allocations (e.g., RO at 15435/16381)
- Fix conflicts (e.g., AIAnalysis using Gateway's ports)
- Update DD-TEST-001 with final decisions

---

## ❓ **QUESTIONS FOR USER**

### **Critical Questions**

1. **Authority**: Should I follow DD-TEST-001 exactly, or should DD-TEST-001 be updated to match working code?

2. **Gateway Redis Port**: DD-TEST-001 says 16380, code uses 16383. Which is correct?

3. **AIAnalysis Infrastructure**:
   - Does AIAnalysis actually need PostgreSQL/Redis/DataStorage in integration tests?
   - Why is it using ports allocated to Gateway and EffectivenessMonitor in DD-TEST-001?

4. **Missing Services**:
   - Should Notification, WorkflowExecution be added to DD-TEST-001?
   - If yes, do they need full infrastructure or just DataStorage URLs?

5. **Migration Scope**:
   - Should we ONLY migrate services that follow DD-TEST-001?
   - Or should we first FIX services to match DD-TEST-001, then migrate?

---

**Document Status**: 🚨 **BLOCKED** - Need clarification before proceeding
**Confidence**: **100%** that current allocations have conflicts with DD-TEST-001
**Next Action**: User clarification on DD-TEST-001 authority and service infrastructure needs











