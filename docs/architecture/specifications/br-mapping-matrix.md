# Business Requirements (BR) Mapping Matrix

**Date**: October 6, 2025
**Status**: ✅ Complete
**Purpose**: Quick reference for BR-to-Service mapping across all stateless services

---

## Visual BR Distribution

```
Service              BR Prefix      V1 Active    Reserved    Total Range
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Gateway              BR-GATEWAY-*   001-092 (19) 093-180     180
Context API          BR-CTX-*       001-010 (5)  011-180     180
Data Storage         BR-STORAGE-*   001-010 (5)  011-180     180
HolmesGPT API        BR-HOLMES-*    001-180 (16) N/A         180
Dynamic Toolset      BR-TOOLSET-*   001-010 (5)  011-180     180
Notification         BR-NOT-*       001-037 (19) 038-040     40
Effectiveness        BR-INS-*       001-010 (7)  011-100     100
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
TOTAL                               76 active    914 reserved 990
```

---

## Detailed BR Mapping by Service

### 1. Gateway Service (BR-GATEWAY-001 to BR-GATEWAY-180)

**19 Active BRs in V1**

```
Range        Category                  Count   BRs
───────────────────────────────────────────────────────────────────────
001-023      Alert Ingestion          12      001,002,005,006,010,011,
                                               015,016,020,021,022,023
024-050      (Reserved)               27      -
051-053      Environment Class        3       051,052,053
054-070      (Reserved)               17      -
071-072      GitOps Integration       2       071,072
073-090      (Reserved)               18      -
091-092      Downstream Notify        2       091,092
093-180      (Reserved)               88      -
───────────────────────────────────────────────────────────────────────
TOTAL                                 180     19 active, 161 reserved
```

**Documentation**: overview.md, testing-strategy.md, implementation-checklist.md

---

### 2. Context API (BR-CTX-001 to BR-CTX-180)

**5 Active BRs in V1**

```
Range        Category                  Count   BRs
───────────────────────────────────────────────────────────────────────
001-010      V1 Core Queries          5       001,002,003,004,005
011-180      V2/V3 Reserved           170     -
───────────────────────────────────────────────────────────────────────
TOTAL                                 180     5 active, 175 reserved
```

**V1 Scope**: Read-only query API with PostgreSQL + Redis caching
**V2 Reserved**: Advanced analytics, ML predictions, multi-cluster context

**Documentation**: testing-strategy.md, implementation-checklist.md (with V1 scope note)

---

### 3. Data Storage (BR-STORAGE-001 to BR-STORAGE-180)

**5 Active BRs in V1**

```
Range        Category                  Count   BRs
───────────────────────────────────────────────────────────────────────
001-010      V1 Core Persistence      5       001,002,003,004,005
011-180      V2/V3 Reserved           170     -
───────────────────────────────────────────────────────────────────────
TOTAL                                 180     5 active, 175 reserved
```

**V1 Scope**: PostgreSQL + pgvector for audit trail and embeddings
**V2 Reserved**: Multi-region replication, advanced indexing, time-series optimization

**Documentation**: testing-strategy.md, implementation-checklist.md (with V1 scope note)

---

### 4. HolmesGPT API (BR-HOLMES-001 to BR-HOLMES-180)

**16 Documented BRs (Full range 001-180 claimed)**

```
Range        Category                  Count   Key BRs Documented
───────────────────────────────────────────────────────────────────────
001-050      Investigation API        50      001,002,003,004,005,
                                               010,011,012,020,030,040
051-100      Toolset Management       50      (Range claimed in
                                               ORIGINAL_MONOLITHIC.md)
101-150      LLM Provider Integ       50      (Range claimed)
151-170      Error Handling           20      (Range claimed)
171-180      Service Reliability      10      171,172,173,174,175,176
───────────────────────────────────────────────────────────────────────
TOTAL                                 180     16 explicitly documented
                                              (full 180 range claimed)
```

**V1 Scope**: Full 180 BR range claimed for comprehensive HolmesGPT investigation capabilities
**Documentation**: overview.md (16 key BRs mapped), ORIGINAL_MONOLITHIC.md (complete spec)

---

### 5. Dynamic Toolset (BR-TOOLSET-001 to BR-TOOLSET-180)

**5 Active BRs in V1**

```
Range        Category                  Count   BRs
───────────────────────────────────────────────────────────────────────
001-010      V1 Auto-Discovery        5       001,002,003,004,005
011-180      V2/V3 Reserved           170     -
───────────────────────────────────────────────────────────────────────
TOTAL                                 180     5 active, 175 reserved
```

**V1 Scope**: Kubernetes Service watch + ConfigMap reconciliation
**V2 Reserved**: Multi-cluster discovery, advanced toolset orchestration, version management

**Documentation**: testing-strategy.md, implementation-checklist.md (with V1 scope note)

---

### 6. Notification Service (BR-NOT-001 to BR-NOT-040)

**19 Active BRs in V1**

```
Range        Category                  Count   BRs
───────────────────────────────────────────────────────────────────────
001-005      Multi-Channel Delivery   5       001,002,003,004,005
006-007      Template Management      2       006,007
008-025      (Gap in numbering)       -       -
026-037      Escalation Notify        12      026,027,028,029,030,031,
                                               032,033,034,035,036,037
038-040      V2 Future                3       038,039,040 (deferred)
───────────────────────────────────────────────────────────────────────
TOTAL                                 40      19 active (14 V1, 5 doc),
                                              3 V2 future
```

**V1 Scope**: Escalation notifications (BR-NOT-026 to 037) + Multi-channel (001-005)
**V2 Future**: Access-aware rendering (038), i18n support (039), template validation (040)

**Documentation**: overview.md, testing-strategy.md (complete BR coverage table), integration-points.md

---

### 7. Effectiveness Monitor (BR-INS-001 to BR-INS-100)

**7 Active BRs in V1**

```
Range        Category                  Count   BRs
───────────────────────────────────────────────────────────────────────
001-010      V1 Core Assessment       7       001,002,003,005,006,
                                               008,010
011-100      V2 Multi-Cloud Reserved  90      -
───────────────────────────────────────────────────────────────────────
TOTAL                                 100     7 active, 93 reserved
```

**V1 Scope**: Core effectiveness assessment with graceful degradation (Week 5 → Week 13+)
**V2 Reserved**: Multi-cloud support (AWS CloudWatch, Azure Monitor, Datadog, GCP), ML prediction

**Rationale**: Limited V1 scope due to:
- Graceful degradation strategy
- Kubernetes-only observability for V1
- V2 expansion to multi-cloud and ML

**Documentation**: overview.md (with rationale), implementation-checklist.md (with V1 scope note)

---

## BR Prefix Quick Reference

| BR Prefix | Service | Active Count | Reserved Count | Total Range |
|-----------|---------|--------------|----------------|-------------|
| **BR-GATEWAY-*** | Gateway Service | 19 | 161 | 180 |
| **BR-CTX-*** | Context API | 5 | 175 | 180 |
| **BR-STORAGE-*** | Data Storage | 5 | 175 | 180 |
| **BR-HOLMES-*** | HolmesGPT API | 16+ (180 claimed) | 0 | 180 |
| **BR-TOOLSET-*** | Dynamic Toolset | 5 | 175 | 180 |
| **BR-NOT-*** | Notification Service | 19 | 21 | 40 |
| **BR-INS-*** | Effectiveness Monitor | 7 | 93 | 100 |

---

## Cross-Service BR Dependencies

### **Gateway Service**

**Downstream BRs** (CRD creation triggers):
- BR-GATEWAY-091 → Triggers Notification Service escalation (formerly BR-NOT-026)
- BR-GATEWAY-092 → Provides notification metadata (formerly BR-NOT-037)

**No Upstream BRs** (Gateway is the entry point)

---

### **Context API**

**Upstream Dependencies**:
- None (read-only query service)

**Downstream Dependencies**:
- Queries Data Storage (BR-STORAGE-001 to BR-STORAGE-005)

**Used By**:
- HolmesGPT API (BR-HOLMES-011: Context API integration)

---

### **Data Storage**

**Upstream Dependencies**:
- None (persistence layer)

**Downstream Dependencies**:
- None (foundational service)

**Used By**:
- All services (audit trail, embeddings, vector search)

---

### **HolmesGPT API**

**Upstream Dependencies**:
- Context API (BR-HOLMES-011: similar remediation context)
- Dynamic Toolset (BR-HOLMES-004: toolset discovery)

**Downstream Dependencies**:
- Kubernetes API (BR-HOLMES-005: RBAC for logs/events)
- Prometheus (BR-HOLMES-020: metrics analysis)

**Used By**:
- AI Analysis Controller (investigates alerts)

---

### **Dynamic Toolset**

**Upstream Dependencies**:
- Kubernetes API (BR-TOOLSET-001: service discovery)

**Downstream Dependencies**:
- HolmesGPT API (BR-TOOLSET-004: ConfigMap updates)

**Used By**:
- HolmesGPT API (toolset configuration consumer)

---

### **Notification Service**

**Upstream Dependencies**:
- Gateway Service (BR-GATEWAY-091: escalation trigger)

**Downstream Dependencies**:
- Email (SMTP) - BR-NOT-001
- Slack - BR-NOT-002
- Teams - BR-NOT-005
- SMS (Twilio) - BR-NOT-004
- PagerDuty - BR-NOT-002

**Triggered By**:
- Remediation Orchestrator (timeout escalations)
- AI Analysis Controller (low confidence escalations)
- Workflow Execution (failures)
- Kubernetes Executor (safety violations)

---

### **Effectiveness Monitor**

**Upstream Dependencies**:
- Data Storage (BR-INS-001: historical action data)
- Infrastructure Monitoring (BR-INS-002: environmental metrics)

**Downstream Dependencies**:
- None (assessment service)

**Used By**:
- Context API (effectiveness scores for remediation selection)
- HolmesGPT API (effectiveness insights for AI analysis)

---

## Implementation Order Based on BR Dependencies

```
Phase 1: Foundation
┌─────────────────┐
│  Data Storage   │ (No upstream dependencies)
│  BR-STORAGE-*   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌──────────────────┐
│  Gateway        │────▶│  Notification    │
│  BR-GATEWAY-*   │     │  BR-NOT-*        │
└────────┬────────┘     └──────────────────┘
         │
         ▼
Phase 2: Core Services
┌─────────────────┐     ┌──────────────────┐
│  Context API    │◀────│  HolmesGPT API   │
│  BR-CTX-*       │     │  BR-HOLMES-*     │
└─────────────────┘     └────────┬─────────┘
                                 │
                                 ▼
                        ┌──────────────────┐
                        │  Dynamic Toolset │
                        │  BR-TOOLSET-*    │
                        └──────────────────┘

Phase 3: Enhancement
┌─────────────────────────┐
│  Effectiveness Monitor  │
│  BR-INS-*               │
└─────────────────────────┘
```

---

## BR Documentation Checklist

✅ **All services have**:
- [ ] Dedicated BR prefix
- [ ] BR range defined (001-XXX)
- [ ] V1 scope clarified (for services with reserved ranges)
- [ ] BR mapping in overview.md or testing-strategy.md
- [ ] BR references in implementation-checklist.md
- [ ] BR-to-test coverage mapping
- [ ] Zero legacy BR references

✅ **Cross-service BR dependencies documented**
✅ **BR ownership transfers completed** (BR-NOT-026/037 → BR-GATEWAY-091/092)
✅ **Implementation order respects BR dependencies**

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Reference Matrix
