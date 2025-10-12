# Service Development Order Strategy
## Optimized for Test Coverage and Dependency Management

**Created**: October 10, 2025
**Purpose**: Define optimal development order for kubernaut services that maximizes unit and integration test coverage
**Methodology**: Dependency-driven, self-contained-first approach

---

## 📊 Service Dependency Analysis

### All Services Overview

| Service | Type | Status | Dependencies | Can Test Independently |
|---------|------|--------|--------------|----------------------|
| **Gateway Service** | Stateless HTTP | ✅ **COMPLETE** | Redis (external) | ✅ Yes |
| **Dynamic Toolset** | Stateless HTTP | ⏸️ Pending | K8s API (external) | ✅ Yes |
| **Data Storage** | Stateless HTTP | ⏸️ Pending | PostgreSQL, Vector DB (external) | ✅ Yes |
| **Context API** | Stateless HTTP | ⏸️ Pending | Data Storage (writes), PostgreSQL (reads) | ⚠️ Partial |
| **HolmesGPT API** | Stateless HTTP (Python) | ⏸️ Pending | Dynamic Toolset, LLM Provider (external) | ⚠️ Partial |
| **RemediationProcessor** | CRD Controller | ⏸️ Pending | Context API (optional), Data Storage | ✅ Yes |
| **AIAnalysis** | CRD Controller | ⏸️ Pending | HolmesGPT API (required), Context API (optional) | ❌ No |
| **WorkflowExecution** | CRD Controller | ⏸️ Pending | Context API (optional), Data Storage | ✅ Yes |
| **KubernetesExecutor** | CRD Controller | ⏸️ Pending | K8s API (external), Data Storage | ✅ Yes |
| **RemediationOrchestrator** | CRD Controller | ⏸️ Pending | All CRD schemas (create/watch) | ❌ No |
| **Notification Service** | Stateless HTTP | ⏸️ Pending | Email/Slack/Teams (external) | ✅ Yes |
| **Effectiveness Monitor** | Stateless HTTP | ⏸️ Pending | Data Storage (required) | ❌ No |

---

## 🔗 Dependency Graph

### Visual Dependency Map

```
┌─────────────────────────────────────────────────────────────────┐
│                        External Systems                          │
│  Redis, PostgreSQL, Vector DB, K8s API, LLM Provider, Email     │
└────────────┬────────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────────┐
│                   PHASE 1: Foundation Services                   │
│                     (Self-Contained)                             │
│                                                                  │
│  ✅ Gateway Service (COMPLETE)                                  │
│  ├─ Depends: Redis (external only)                             │
│  └─ Integration Tests: Redis containers                         │
│                                                                  │
│  1️⃣ Dynamic Toolset Service                                     │
│  ├─ Depends: K8s API (external only)                           │
│  └─ Integration Tests: Fake K8s client                          │
│                                                                  │
│  2️⃣ Data Storage Service                                        │
│  ├─ Depends: PostgreSQL, Vector DB (external only)             │
│  └─ Integration Tests: Testcontainers (PostgreSQL + pgvector)  │
│                                                                  │
│  3️⃣ Notification Service                                        │
│  ├─ Depends: Email/Slack APIs (external only)                  │
│  └─ Integration Tests: Mock SMTP/Slack servers                  │
└────────────┬────────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────────┐
│              PHASE 2: Intelligence & Context Layer               │
│                 (Depends on Phase 1 Data Storage)                │
│                                                                  │
│  4️⃣ Context API (Read Layer)                                    │
│  ├─ Depends: Data Storage (writes), PostgreSQL (reads)         │
│  ├─ Integration Tests: Real Data Storage service + PostgreSQL  │
│  └─ Note: Data Storage must be running for integration tests   │
│                                                                  │
│  5️⃣ HolmesGPT API Service                                       │
│  ├─ Depends: Dynamic Toolset, LLM Provider (external)          │
│  ├─ Integration Tests: Real Dynamic Toolset + Mock LLM         │
│  └─ Note: Dynamic Toolset must be running for integration tests│
└────────────┬────────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────────┐
│        PHASE 3: Core CRD Controllers (Business Logic)            │
│        (Can be developed in parallel, depend on Phase 1 & 2)     │
│                                                                  │
│  6️⃣ RemediationProcessor Controller (Parallel Track A)          │
│  ├─ Depends: Context API (optional), Data Storage              │
│  ├─ Integration Tests: Real Context API + Data Storage         │
│  └─ Note: Can gracefully degrade without Context API           │
│                                                                  │
│  7️⃣ WorkflowExecution Controller (Parallel Track B)             │
│  ├─ Depends: Context API (optional), Data Storage              │
│  ├─ Integration Tests: Real Context API + Data Storage         │
│  └─ Note: Can gracefully degrade without Context API           │
│                                                                  │
│  8️⃣ KubernetesExecutor Controller (Parallel Track C)            │
│  ├─ Depends: K8s API (external), Data Storage                  │
│  ├─ Integration Tests: Kind cluster + Data Storage             │
│  └─ Note: Self-contained, no other kubernaut services          │
└────────────┬────────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────────┐
│      PHASE 4: AI Integration Layer (Depends on HolmesGPT)       │
│                                                                  │
│  9️⃣ AIAnalysis Controller                                       │
│  ├─ Depends: HolmesGPT API (required), Context API (optional)  │
│  ├─ Integration Tests: Real HolmesGPT API + Context API        │
│  └─ Note: Cannot run integration tests without HolmesGPT API   │
└────────────┬────────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────────┐
│   PHASE 5: Orchestration & Observability (Final Integration)    │
│                                                                  │
│  🔟 RemediationOrchestrator Controller                          │
│  ├─ Depends: All 4 CRD schemas (create/watch/coordinate)       │
│  ├─ Integration Tests: All CRD controllers must exist          │
│  └─ Note: Last controller, orchestrates entire workflow        │
│                                                                  │
│  1️⃣1️⃣ Effectiveness Monitor Service                              │
│  ├─ Depends: Data Storage (required for historical data)       │
│  ├─ Integration Tests: Real Data Storage with historical data  │
│  └─ Note: Requires 8+ weeks of data for full capability        │
└─────────────────────────────────────────────────────────────────┘
```

---

## 🎯 Recommended Development Order

### **Phase 1: Foundation (3 services) - WEEKS 1-3**

**Goal**: Establish core infrastructure services with no kubernaut dependencies

#### 1️⃣ **Dynamic Toolset Service** (Week 1, 3-4 days)
- **Why First**:
  - ✅ Zero dependencies on other kubernaut services
  - ✅ 100% unit testable (K8s API can be faked)
  - ✅ Simple service discovery logic
  - ✅ Required by HolmesGPT API (Phase 2)
- **Testing**:
  - Unit: 70%+ (service discovery algorithms, config generation)
  - Integration: 20% (Fake K8s client, Kind cluster)
  - E2E: <10% (Real cluster, verify toolset discovery)
- **Effort**: 16-24 hours
- **Confidence**: 90%

#### 2️⃣ **Data Storage Service** (Week 1-2, 4-5 days)
- **Why Second**:
  - ✅ Zero dependencies on other kubernaut services
  - ✅ 100% unit testable (database logic)
  - ✅ Integration tests with Testcontainers (PostgreSQL + pgvector)
  - ✅ Required by Context API, all CRD controllers (Phase 2+)
- **Testing**:
  - Unit: 70%+ (write logic, embedding generation, dual-write coordination)
  - Integration: 20% (Testcontainers PostgreSQL + pgvector)
  - E2E: <10% (Real databases, verify embeddings)
- **Effort**: 24-32 hours
- **Confidence**: 85%

#### 3️⃣ **Notification Service** (Week 2-3, 3-4 days)
- **Why Third**:
  - ✅ Zero dependencies on other kubernaut services
  - ✅ 100% unit testable (notification routing, channel adapters)
  - ✅ Integration tests with mock SMTP/Slack servers
  - ✅ Can be developed in parallel with Phase 2
- **Testing**:
  - Unit: 70%+ (routing logic, sanitization, formatting)
  - Integration: 20% (Mock SMTP, Mock Slack API)
  - E2E: <10% (Real email/Slack, verify delivery)
- **Effort**: 16-24 hours
- **Confidence**: 90%

**Phase 1 Total**: 56-80 hours (7-10 days)

---

### **Phase 2: Intelligence Layer (2 services) - WEEKS 3-5**

**Goal**: Build AI and context services that depend on Phase 1

#### 4️⃣ **Context API** (Week 3-4, 4-5 days)
- **Why Fourth**:
  - ⚠️ Depends on Data Storage (Phase 1) for writes
  - ✅ Can run integration tests with real Data Storage
  - ✅ Used by all CRD controllers (optional but valuable)
  - ✅ Read-only service, simpler than write layer
- **Testing**:
  - Unit: 70%+ (caching logic, query patterns, pattern matching)
  - Integration: 20% (Real Data Storage + PostgreSQL, verify caching)
  - E2E: <10% (End-to-end query with real historical data)
- **Effort**: 24-32 hours
- **Confidence**: 85%
- **Prerequisite**: Data Storage service operational

#### 5️⃣ **HolmesGPT API Service** (Week 4-5, 5-6 days)
- **Why Fifth**:
  - ⚠️ Depends on Dynamic Toolset (Phase 1)
  - ✅ Can run integration tests with real Dynamic Toolset + mock LLM
  - ✅ Required by AIAnalysis controller (Phase 4)
  - ⚠️ Python service (different tech stack, consider effort)
- **Testing**:
  - Unit: 70%+ (investigation orchestration, toolset integration)
  - Integration: 20% (Real Dynamic Toolset + Mock LLM responses)
  - E2E: <10% (Real LLM, verify root cause analysis)
- **Effort**: 32-40 hours
- **Confidence**: 80% (Python wrapper complexity)
- **Prerequisite**: Dynamic Toolset service operational

**Phase 2 Total**: 56-72 hours (7-9 days)

---

### **Phase 3: Core CRD Controllers (3 services, PARALLEL) - WEEKS 5-8**

**Goal**: Build business logic controllers that can be developed simultaneously

#### **Parallel Track A: RemediationProcessor Controller** (Week 5-6, 5-6 days)
- **Why Parallel Track A**:
  - ⚠️ Depends on Context API (optional) and Data Storage
  - ✅ Can run full integration tests with real services
  - ✅ Self-contained business logic (alert enrichment)
  - ✅ First in CRD chain (created by Orchestrator)
- **Testing**:
  - Unit: 70%+ (enrichment logic, targeting data extraction)
  - Integration: 20% (Real Context API + Data Storage + Kind cluster)
  - E2E: <10% (Complete RemediationProcessing lifecycle)
- **Effort**: 32-40 hours
- **Confidence**: 85%
- **Prerequisites**: Context API, Data Storage operational

#### **Parallel Track B: WorkflowExecution Controller** (Week 6-7, 6-7 days)
- **Why Parallel Track B**:
  - ⚠️ Depends on Context API (optional) and Data Storage
  - ✅ Can run full integration tests with real services
  - ✅ Self-contained workflow orchestration logic
  - ✅ Third in CRD chain (after AIAnalysis)
- **Testing**:
  - Unit: 70%+ (workflow building, step sequencing, validation)
  - Integration: 20% (Real Context API + Data Storage + Kind cluster)
  - E2E: <10% (Complete workflow execution with actions)
- **Effort**: 40-48 hours
- **Confidence**: 80% (Workflow complexity)
- **Prerequisites**: Context API, Data Storage operational

#### **Parallel Track C: KubernetesExecutor Controller** (Week 7-8, 5-6 days)
- **Why Parallel Track C**:
  - ⚠️ Depends on Data Storage (audit trail)
  - ✅ Can run full integration tests with Kind cluster
  - ✅ Self-contained K8s action execution logic
  - ✅ Final in CRD chain (executes actions)
- **Testing**:
  - Unit: 70%+ (action execution logic, safety validation, rollback)
  - Integration: 20% (Kind cluster + Data Storage, verify actions)
  - E2E: <10% (Complete action execution with real K8s resources)
- **Effort**: 32-40 hours
- **Confidence**: 85%
- **Prerequisites**: Data Storage operational, Kind cluster

**Phase 3 Total (Parallel)**: 104-128 hours (13-16 days if sequential, 5-7 days if parallel with 3 developers)

---

### **Phase 4: AI Integration (1 service) - WEEKS 8-10**

**Goal**: Integrate AI analysis capability

#### 9️⃣ **AIAnalysis Controller** (Week 8-10, 6-7 days)
- **Why After Phase 3**:
  - ⚠️ **HARD DEPENDENCY**: Requires HolmesGPT API (Phase 2)
  - ⚠️ Depends on Context API (optional)
  - ❌ **Cannot run integration tests** until HolmesGPT API operational
  - ✅ Second in CRD chain (after RemediationProcessing)
- **Testing**:
  - Unit: 70%+ (investigation request building, result processing)
  - Integration: 20% (Real HolmesGPT API + Context API + Kind cluster)
  - E2E: <10% (Complete AI analysis with real LLM)
- **Effort**: 40-48 hours
- **Confidence**: 75% (AI integration complexity)
- **Prerequisites**: HolmesGPT API, Context API, Data Storage operational

**Phase 4 Total**: 40-48 hours (5-6 days)

---

### **Phase 5: Orchestration & Observability (2 services) - WEEKS 10-13**

**Goal**: Complete end-to-end workflow orchestration and monitoring

#### 🔟 **RemediationOrchestrator Controller** (Week 10-12, 7-8 days)
- **Why Last Controller**:
  - ⚠️ **HARD DEPENDENCY**: Requires ALL 4 CRD schemas (create/watch)
  - ❌ **Cannot run integration tests** until all controllers operational
  - ✅ Central coordinator, must wait for all services
  - ✅ Watch-based, less complex than individual controllers
- **Testing**:
  - Unit: 70%+ (CRD creation logic, status aggregation, timeout detection)
  - Integration: 20% (All 4 controllers + Kind cluster, end-to-end workflow)
  - E2E: <10% (Complete remediation flow: Gateway → Resolution)
- **Effort**: 48-56 hours
- **Confidence**: 70% (Orchestration complexity, depends on all services)
- **Prerequisites**: ALL Phase 3 + Phase 4 controllers operational

#### 1️⃣1️⃣ **Effectiveness Monitor Service** (Week 12-13, 4-5 days)
- **Why Last Service**:
  - ⚠️ **HARD DEPENDENCY**: Requires Data Storage (historical data)
  - ⚠️ **DATA DEPENDENCY**: Requires 8+ weeks of remediation data for full capability
  - ✅ Can deploy in Week 5 with "insufficient_data" responses
  - ✅ Observability layer, not in critical path
- **Testing**:
  - Unit: 70%+ (assessment algorithms, trend analysis, side effect detection)
  - Integration: 20% (Real Data Storage with historical data)
  - E2E: <10% (End-to-end effectiveness assessment with real data)
- **Effort**: 24-32 hours
- **Confidence**: 85%
- **Prerequisites**: Data Storage operational, 8+ weeks historical data (for full capability)

**Phase 5 Total**: 72-88 hours (9-11 days)

---

## 📊 Summary: Total Effort & Timeline

### By Phase
| Phase | Services | Effort (Sequential) | Effort (Parallel) | Duration (1 dev) | Duration (3 devs) |
|-------|----------|-------------------|-------------------|------------------|------------------|
| **Phase 1** | 3 services | 56-80 hours | 56-80 hours | 7-10 days | 3-4 days |
| **Phase 2** | 2 services | 56-72 hours | 56-72 hours | 7-9 days | 4-5 days |
| **Phase 3** | 3 controllers | 104-128 hours | **40-48 hours** | 13-16 days | **5-7 days** |
| **Phase 4** | 1 controller | 40-48 hours | 40-48 hours | 5-6 days | 5-6 days |
| **Phase 5** | 2 services | 72-88 hours | 48-56 hours | 9-11 days | 6-7 days |
| **TOTAL** | 11 services | **328-416 hours** | **240-304 hours** | **41-52 days** | **23-29 days** |

### Critical Path (3 developers, parallel Phase 3)
- **Week 1-3**: Phase 1 (Foundation) - 3-4 days
- **Week 3-5**: Phase 2 (Intelligence) - 7-9 days
- **Week 5-8**: Phase 3 (Core Controllers, PARALLEL) - 5-7 days
- **Week 8-10**: Phase 4 (AI Integration) - 5-6 days
- **Week 10-13**: Phase 5 (Orchestration) - 9-11 days
- **Total**: ~29-37 days with 3 developers

---

## ✅ Testability Matrix

### Service-by-Service Test Coverage Achievability

| Service | Unit Tests | Integration Tests | E2E Tests | Overall Confidence |
|---------|-----------|------------------|-----------|-------------------|
| ✅ **Gateway** | 100% (mocks) | 95% (Redis + Kind) | 100% (full flow) | **98%** (DONE) |
| **Dynamic Toolset** | 100% (Fake K8s) | 95% (Kind cluster) | 90% (real cluster) | **95%** |
| **Data Storage** | 100% (mocks) | 95% (Testcontainers) | 90% (real DBs) | **95%** |
| **Notification** | 100% (mocks) | 95% (mock servers) | 80% (real delivery) | **90%** |
| **Context API** | 100% (mocks) | 85% (needs Data Storage) | 80% (end-to-end) | **85%** |
| **HolmesGPT API** | 100% (mock LLM) | 80% (needs Dynamic Toolset) | 70% (real LLM) | **80%** |
| **RemediationProcessor** | 100% (mocks) | 90% (needs Context + Data) | 85% (full CRD) | **90%** |
| **WorkflowExecution** | 100% (mocks) | 90% (needs Context + Data) | 80% (full workflow) | **85%** |
| **KubernetesExecutor** | 100% (mocks) | 95% (Kind + Data Storage) | 90% (real actions) | **95%** |
| **AIAnalysis** | 100% (mocks) | 80% (needs HolmesGPT) | 70% (real AI) | **80%** |
| **RemediationOrchestrator** | 100% (mocks) | 70% (needs ALL controllers) | 90% (end-to-end) | **80%** |
| **Effectiveness Monitor** | 100% (mocks) | 60% (needs historical data) | 70% (8+ weeks data) | **75%** |

---

## 🎯 Key Benefits of This Order

### 1. **Maximizes Independent Testing** ✅
- Phase 1 & 2: 5 services can be fully tested independently
- Phase 3: 3 controllers can be tested with Phase 1+2 services
- Early high confidence (90-95%) for foundation services

### 2. **Enables Parallel Development** ✅
- Phase 3: 3 CRD controllers can be developed simultaneously
- Reduces Phase 3 from 13-16 days to 5-7 days (3 developers)
- 40% faster completion with parallel work

### 3. **Manages Risk Progressively** ✅
- High-confidence services first (90-95%)
- Complex integrations last (70-80%)
- Hard dependencies clearly identified

### 4. **Supports Continuous Integration** ✅
- Each phase delivers working, testable services
- Integration tests possible as dependencies complete
- No "big bang" integration at the end

### 5. **Optimizes Resource Allocation** ✅
- Clear handoff points between phases
- Parallel work opportunities identified
- Critical path optimized (29-37 days with 3 devs)

---

## 🚧 Dependencies at Each Phase

### Phase 1 (Foundation)
**External Only**:
- Redis (Gateway - already done)
- PostgreSQL + Vector DB (Data Storage)
- Kubernetes API (Dynamic Toolset)
- Email/Slack APIs (Notification)

**Kubernaut**: None ✅

### Phase 2 (Intelligence)
**External**:
- LLM Provider (HolmesGPT API)

**Kubernaut**:
- Data Storage (Context API writes)
- Dynamic Toolset (HolmesGPT API toolset discovery)

### Phase 3 (Core Controllers)
**External**:
- Kubernetes API (all controllers)

**Kubernaut**:
- Context API (optional for all 3)
- Data Storage (required for all 3)

### Phase 4 (AI Integration)
**External**:
- Kubernetes API

**Kubernaut**:
- HolmesGPT API (required) ⚠️ HARD DEPENDENCY
- Context API (optional)
- Data Storage (required)

### Phase 5 (Orchestration)
**External**:
- Kubernetes API

**Kubernaut**:
- ALL Phase 3 + Phase 4 controllers ⚠️ HARD DEPENDENCY
- Data Storage (Effectiveness Monitor)

---

## 📝 Implementation Notes

### Testing Strategy Per Phase

#### Phase 1
- **Unit**: Mock all external dependencies (Redis, PostgreSQL, K8s API)
- **Integration**: Testcontainers (PostgreSQL), Kind cluster (K8s), Redis containers
- **E2E**: Real external systems, verify end-to-end behavior
- **Confidence**: 90-95% (self-contained services)

#### Phase 2
- **Unit**: Mock external dependencies + Phase 1 services
- **Integration**: Real Phase 1 services + mock external (LLM)
- **E2E**: Real Phase 1 services + real external systems
- **Confidence**: 80-85% (depends on Phase 1 quality)

#### Phase 3 (Parallel Development)
- **Unit**: Mock all dependencies (K8s API, Phase 1+2 services)
- **Integration**: Real Phase 1+2 services + Kind cluster
- **E2E**: Complete CRD lifecycle with real dependencies
- **Confidence**: 85-90% (parallel development, clear contracts)

#### Phase 4
- **Unit**: Mock HolmesGPT API responses
- **Integration**: Real HolmesGPT API + Phase 2 services
- **E2E**: Real LLM analysis, verify AI integration
- **Confidence**: 75-80% (AI complexity, external LLM)

#### Phase 5
- **Unit**: Mock all controllers
- **Integration**: Real all controllers + Kind cluster
- **E2E**: Complete Gateway → Resolution flow
- **Confidence**: 70-80% (orchestration complexity, many dependencies)

---

## 🎯 Success Criteria

### Phase Completion Gates

#### Phase 1 Complete When:
- ✅ All 3 services pass unit tests (70%+ coverage)
- ✅ Integration tests run successfully with external deps
- ✅ E2E tests validate end-to-end service behavior
- ✅ Services deployable to Kind cluster
- ✅ Metrics and health checks functional

#### Phase 2 Complete When:
- ✅ Context API can query Data Storage successfully
- ✅ HolmesGPT API can fetch toolsets from Dynamic Toolset
- ✅ Integration tests verify Phase 1 + Phase 2 interactions
- ✅ Mock LLM responses validate HolmesGPT API behavior

#### Phase 3 Complete When:
- ✅ All 3 controllers watch and reconcile their CRDs
- ✅ RemediationProcessing enriches alerts
- ✅ WorkflowExecution builds workflows
- ✅ KubernetesExecutor executes actions
- ✅ Integration tests with Phase 1+2 services pass

#### Phase 4 Complete When:
- ✅ AIAnalysis controller invokes HolmesGPT API
- ✅ Real LLM integration works (OpenAI/Claude/Local)
- ✅ Investigation results stored in CRD status
- ✅ E2E test: RemediationProcessing → AIAnalysis works

#### Phase 5 Complete When:
- ✅ RemediationOrchestrator creates all 4 CRD types
- ✅ Status aggregation works across all CRDs
- ✅ Timeout detection and escalation functional
- ✅ Complete Gateway → Resolution E2E test passes
- ✅ Effectiveness Monitor operational (Week 13+ for full capability)

---

## 📈 Confidence Assessment

**Overall Strategy Confidence**: **90%** (Very High)

**Rationale**:
- ✅ Dependency analysis is comprehensive
- ✅ Self-contained-first approach proven effective
- ✅ Parallel development opportunities maximize efficiency
- ✅ Clear testing strategy for each phase
- ✅ Risk managed through progressive integration
- ⚠️ AI complexity (HolmesGPT, LLM) introduces uncertainty
- ⚠️ Orchestrator depends on all services (late integration risk)

**Risk Mitigation**:
- Start with high-confidence services (90-95%)
- Validate contracts at each phase boundary
- Use mocks extensively for unit tests
- Run integration tests continuously as dependencies complete
- Reserve Effectiveness Monitor for last (observability, not critical path)

---

## 🚀 Next Steps

1. **Review with Team**: Validate dependency analysis and development order
2. **Resource Planning**: Assign developers to parallel Phase 3 tracks
3. **Setup Infrastructure**: Prepare Testcontainers, Kind clusters, mock servers
4. **Start Phase 1**: Begin with Dynamic Toolset Service (Week 1)
5. **Track Progress**: Monitor phase completion gates and adjust as needed

---

**Document Status**: ✅ Complete Development Order Strategy
**Last Updated**: October 10, 2025
**Confidence**: 90% (Very High)
**Recommended Action**: Begin Phase 1, Dynamic Toolset Service

