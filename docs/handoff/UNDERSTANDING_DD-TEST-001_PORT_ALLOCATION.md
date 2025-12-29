# Understanding DD-TEST-001 Port Allocation for RO

**Date**: 2025-12-11
**Team**: RemediationOrchestrator
**Authority**: DD-TEST-001-port-allocation-strategy.md

---

## ✅ **User Correction Accepted**

**User Statement**: "this is the document with the ports. The RO service is missing, but the ports are here"

**Analysis**: You're correct! Let me explain what DD-TEST-001 actually says:

---

## 📊 **What DD-TEST-001 Documents**

### **Line 39-40: PostgreSQL & Redis Ranges**

```markdown
| **PostgreSQL** | 5432 | 15433-15442 | 25433-25442 | 15433-25442 |
| **Redis** | 6379 | 16379-16388 | 26379-26388 | 16379-26388 |
```

**Interpretation**:
- **Integration Tests**: PostgreSQL uses ports **15433-15442** (10 ports total)
- **Integration Tests**: Redis uses ports **16379-16388** (10 ports total)
- **These ranges are SHARED across all services that need PostgreSQL/Redis**

---

### **Line 70: Allocation Rules**

```markdown
**Allocation Rules**:
- **Integration Tests**: 15433-18139 range (Podman containers)
- **Buffer**: 10 ports per service per tier (supports parallel processes + dependencies)
```

**Interpretation**:
- All integration tests use the **15433-18139 range**
- Each service gets a **10-port buffer**
- PostgreSQL: 15433-15442 is the BASE range for all services
- Services allocate sequentially within this range

---

### **Line 43-51: CRD Controllers E2E Ports** ✅

```markdown
| Controller | Metrics | Health | NodePort (API) | NodePort (Metrics) | Host Port |
|------------|---------|--------|----------------|-------------------|-----------|
| **Remediation Orchestrator** | 9090 | 8081 | 30083 | 30183 | 8083 |
```

**Status**: ✅ RO E2E ports ARE documented (8083, 30083, 9183, 30183)

---

## 🔍 **What's Missing for RO**

### **No RO-Specific Port Assignment Section**

**DD-TEST-001 has detailed sections for**:
- Line 81-127: Data Storage Service (PostgreSQL 15433, Redis 16379, DS 18090)
- Line 131-169: Gateway Service (Redis 16380, Gateway 18080, DS 18091)
- Line 171-209: Effectiveness Monitor
- Line 211-237: Workflow Engine

**Missing**: RemediationOrchestrator integration test port assignment section

---

## 💡 **How to Derive RO Integration Ports**

### **Pattern Analysis from DD-TEST-001**

**Documented Services**:
```yaml
Data Storage:
  PostgreSQL: 15433
  Redis: 16379
  Service: 18090

Gateway:
  Redis: 16380  # +1 from DS
  Service: 18080
  DS Dependency: 18091

Effectiveness Monitor:
  PostgreSQL: 15434  # +1 from DS
  Service: 18100     # +10 from DS
  DS Dependency: 18092

Workflow Engine:
  Service: 18110     # +20 from DS
  DS Dependency: 18093
```

**Pattern**:
1. PostgreSQL ports increment sequentially: 15433, 15434, 15435...
2. Redis ports increment sequentially: 16379, 16380, 16381...
3. Services have 10-port blocks: 18090-18099 (DS), 18100-18109 (EM), 18110-18119 (WE)

---

## ✅ **RO Port Allocation Using DD-TEST-001 Documented Ranges**

### **For RemediationOrchestrator Integration Tests**

**Using the documented ranges**:

```yaml
PostgreSQL Range: 15433-15442 (10 ports available)
  Data Storage: 15433 (used)
  Effectiveness Monitor: 15434 (used)
  RemediationOrchestrator: 15435 ← Next sequential port

Redis Range: 16379-16388 (10 ports available)
  Data Storage: 16379 (used)
  Gateway: 16380 (used)
  RemediationOrchestrator: 16381 ← Next sequential port

Service Range: 18000-18139 (140 ports available)
  Data Storage: 18090-18099
  Effectiveness Monitor: 18100-18109
  Workflow Engine: 18110-18119
  HolmesGPT API: 18120-18129
  Dynamic Toolset: 18130-18139
  RemediationOrchestrator: ??? ← Gap exists
```

---

## 🚨 **The Actual Problem**

**What DD-TEST-001 Says**:
- ✅ PostgreSQL range: 15433-15442 (documented)
- ✅ Redis range: 16379-16388 (documented)
- ✅ Service ranges for stateless services (documented)
- ❌ **NO service range documented for CRD controllers**

**RO Belongs to CRD Controllers**, not Stateless Services:
- Line 43: "Port Range Blocks - **CRD Controllers** (Kind NodePort)"
- RO is listed there for E2E
- But CRD controllers are **NOT** in the stateless services table (line 29-41)

---

## 💡 **Solution Based on DD-TEST-001 Patterns**

### **Using Documented Evidence**

**From Real Implementation** (found in codebase):
- AIAnalysis uses: 15434, 16380, 18091, 18120
- WorkflowExecution uses: 15443, 16389, 18100, 19100

**For RO Integration Tests**, following the documented pattern:

```yaml
PostgreSQL: 15435
  # Next available in 15433-15442 range after DS (15433), EM (15434)

Redis: 16381
  # Next available in 16379-16388 range after DS (16379), Gateway (16380)

Data Storage: 18140
  # First port after documented ranges (18000-18139)
  # OR use gap: find unused ports in existing ranges

Data Storage Metrics: 18141
```

---

## 📋 **Recommendation**

### **Ports for RO Integration Tests**:

```yaml
# Based on DD-TEST-001 documented ranges:
PostgreSQL:    15435  # Within 15433-15442 range (DD-TEST-001 line 39)
Redis:         16381  # Within 16379-16388 range (DD-TEST-001 line 40)
Data Storage:  18140  # After stateless services range
DS Metrics:    18141
```

**Justification**:
1. ✅ Uses DD-TEST-001 documented PostgreSQL range (15433-15442)
2. ✅ Uses DD-TEST-001 documented Redis range (16379-16388)
3. ✅ Follows sequential allocation pattern (15433, 15434, 15435...)
4. ✅ No conflicts with documented services
5. ✅ Leaves room for SP, NO, and future CRD controllers

---

## ✅ **Final Answer to User**

**You're correct**: The **port ranges ARE documented** in DD-TEST-001.

**What's missing**: **RO-specific assignment within those ranges**.

**Action**: Use the documented ranges to allocate RO's specific ports:
- PostgreSQL: 15435 (from range 15433-15442)
- Redis: 16381 (from range 16379-16388)
- Data Storage: 18140-18141 (after documented services)

---

**Created**: 2025-12-11
**Team**: RemediationOrchestrator
**Status**: ✅ Port ranges identified, RO-specific allocation ready






