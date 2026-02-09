# HolmesGPT-API Documentation Migration Plan

**Date**: December 3, 2025
**Purpose**: Align holmesgpt-api documentation with SERVICE_DOCUMENTATION_GUIDE.md standard
**Reference**: [SERVICE_DOCUMENTATION_GUIDE.md](../../SERVICE_DOCUMENTATION_GUIDE.md)

---

## 🔍 Gap Analysis

### Current State vs. Standard Structure

| # | Standard Document | HolmesGPT-API Status | Gap Assessment |
|---|------------------|---------------------|----------------|
| 1 | `README.md` | ✅ Present (483 lines) | **Minor** - Good navigation hub but mixes too much content |
| 2 | `overview.md` | ✅ Present (432 lines) | **None** - Complete |
| 3 | `BUSINESS_REQUIREMENTS.md` | ✅ Present | **None** - Complete |
| 4 | `BR_MAPPING.md` | ✅ Present | **None** - Complete |
| 5 | `implementation-checklist.md` | ✅ Present | **None** - Complete |
| 6 | `security-configuration.md` | ✅ Present | **None** - Complete |
| 7 | `observability-logging.md` | ✅ Present | **None** - Complete |
| 8 | **`metrics-slos.md`** | ⚠️ **SPLIT** | **MAJOR** - Content in `observability/PROMETHEUS_QUERIES.md` |
| 9 | `testing-strategy.md` | ✅ Present | **None** - Complete |
| 10 | `integration-points.md` | ✅ Present | **None** - Complete |
| 11 | `api-specification.md` | ✅ Present (SERVICE-SPECIFIC) | **None** - Appropriate for API service |

### Non-Standard Files Analysis

| File | Assessment | Action |
|------|------------|--------|
| `observability/PROMETHEUS_QUERIES.md` | Should be `metrics-slos.md` | **RENAME + MOVE** |
| `observability/grafana-dashboard.json` | Good - reference from metrics-slos.md | **KEEP** |
| `implementation/archive/` | Good - archived implementation plans | **KEEP** |
| `implementation/design/` | Good - design documents | **KEEP** |
| `IMPLEMENTATION_PLAN_V3.0.md` | Active plan - should be in `implementation/` | **MOVE** |
| `IMPLEMENTATION_PLAN_V3.1_*.md` | Active plan - should be in `implementation/` | **MOVE** |
| `BR-HAPI-046-050-*.md` | BR subdocuments - keep at root | **KEEP** |
| `HANDOFF_*.md` | Handoff docs - keep at root | **KEEP** |
| `REMEDIATION-ID-PROPAGATION-*.md` | Implementation doc | **MOVE to implementation/** |
| `SERVICE_LOCATION_DECISION.md` | Design doc | **MOVE to implementation/design/** |
| `PROMPT_GENERATION_UNIT_TEST_GAP_ANALYSIS.md` | Testing doc | **MOVE to implementation/** |

---

## 📋 Migration Tasks

### Phase 1: Create Standard Files (Priority: HIGH)

#### Task 1.1: Create `metrics-slos.md`

**Source**: `observability/PROMETHEUS_QUERIES.md` + `observability/grafana-dashboard.json` reference
**Action**: Create top-level `metrics-slos.md` with standard structure

**Standard Structure** (from SERVICE_DOCUMENTATION_GUIDE.md):
```markdown
# HolmesGPT API - Metrics & SLOs

**Version**: v1.0
**Last Updated**: {date}
**Status**: ✅ Complete

---

## 📊 Service Level Indicators (SLIs)

### Availability SLI
...

### Latency SLI
...

### Error Rate SLI
...

---

## 🎯 Service Level Objectives (SLOs)

| SLO | Target | Measurement |
|-----|--------|-------------|
...

---

## 📈 Prometheus Metrics

### Request Metrics
...

### Latency Metrics
...

### Error Metrics
...

---

## 📊 Grafana Dashboard

**Dashboard JSON**: [grafana-dashboard.json](./observability/grafana-dashboard.json)

### Panels
...

---

## 🚨 Alert Rules

### Critical Alerts
...

### Warning Alerts
...
```

**Effort**: 1-2 hours

---

### Phase 2: Reorganize Files (Priority: MEDIUM)

#### Task 2.1: Move Implementation Plans

| File | Current Location | New Location |
|------|-----------------|--------------|
| `IMPLEMENTATION_PLAN_V3.0.md` | Root | `implementation/IMPLEMENTATION_PLAN_V3.0.md` |
| `IMPLEMENTATION_PLAN_V3.1_RFC7807_GRACEFUL_SHUTDOWN.md` | Root | `implementation/IMPLEMENTATION_PLAN_V3.1_RFC7807_GRACEFUL_SHUTDOWN.md` |
| `REMEDIATION-ID-PROPAGATION-IMPLEMENTATION_PLAN_V1.4.md` | Root | `implementation/REMEDIATION-ID-PROPAGATION-V1.4.md` |
| `PROMPT_GENERATION_UNIT_TEST_GAP_ANALYSIS.md` | Root | `implementation/PROMPT_GENERATION_UNIT_TEST_GAP_ANALYSIS.md` |
| `SERVICE_LOCATION_DECISION.md` | Root | `implementation/design/SERVICE_LOCATION_DECISION.md` |

**Effort**: 30 minutes

#### Task 2.2: Update Cross-References

After moving files, update all internal links in:
- `README.md`
- `overview.md`
- Any other documents referencing moved files

**Effort**: 30 minutes

---

### Phase 3: Enhance README.md (Priority: LOW)

#### Task 3.1: Restructure README.md

Current README.md is 483 lines and mixes:
- Navigation index ✅
- Production status details (should be in overview.md)
- Handoff tracking (should be in separate doc)

**Standard README Structure** (from 01-signalprocessing/README.md):
```markdown
# HolmesGPT API Service

**Version**: vX.X
**Status**: ...
**Service Type**: ...
**Port**: ...

---

## 📋 Changelog
| Version | Date | Changes | Reference |
...

---

## 🗂️ Documentation Index
| Document | Purpose | Lines | Status |
...

---

## 📁 File Organization
```
holmesgpt-api/
├── 📄 README.md              - Service index & navigation
├── 📘 overview.md            - High-level architecture
...
```

---

## 🎯 Quick Start
...

---

## 📞 Contact
...
```

**Action**:
1. Move production status details to `overview.md`
2. Create separate `HANDOFF_STATUS.md` for cross-team tracking
3. Simplify README to be navigation-focused

**Effort**: 2-3 hours

---

## 📁 Target File Organization

After migration, the directory should look like:

```
holmesgpt-api/
├── 📄 README.md                           - Service index & navigation (COMMON)
├── 📘 overview.md                         - High-level architecture (COMMON)
├── 🔧 api-specification.md               - API contract (SERVICE-SPECIFIC)
├── 📋 BUSINESS_REQUIREMENTS.md           - BR catalog (COMMON)
├── 📋 BR_MAPPING.md                       - Test-BR traceability (COMMON)
├── ✅ implementation-checklist.md         - APDC-TDD phases (COMMON)
├── 🔒 security-configuration.md          - Security patterns (COMMON)
├── 📊 observability-logging.md           - Logging & tracing (COMMON)
├── 📈 metrics-slos.md                    - Prometheus & SLOs (COMMON) ⬅️ NEW
├── 🧪 testing-strategy.md                - Test patterns (COMMON)
├── 🔗 integration-points.md              - Service coordination (COMMON)
│
├── 📁 observability/                      - Observability assets
│   └── grafana-dashboard.json            - Grafana dashboard
│
├── 📁 implementation/                     - Implementation docs (reorganized)
│   ├── IMPLEMENTATION_PLAN_V3.0.md
│   ├── IMPLEMENTATION_PLAN_V3.1_RFC7807_GRACEFUL_SHUTDOWN.md
│   ├── REMEDIATION-ID-PROPAGATION-V1.4.md
│   ├── PROMPT_GENERATION_UNIT_TEST_GAP_ANALYSIS.md
│   ├── 📁 archive/                       - Old versions
│   │   └── ...
│   └── 📁 design/
│       ├── README.md
│       └── SERVICE_LOCATION_DECISION.md
│
├── 📁 handoff/                           - Cross-team handoffs (optional - or keep at root)
│   └── HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md
│
└── 📋 BR subdocuments (keep at root)
    ├── BR-HAPI-046-050-DATA-STORAGE-PLAYBOOK-TOOL.md
    └── BR-HAPI-046-050-DATA-STORAGE-WORKFLOW-TOOL.md
```

---

## ⏱️ Estimated Effort

| Phase | Tasks | Effort | Priority |
|-------|-------|--------|----------|
| **Phase 1** | Create `metrics-slos.md` | 1-2 hours | HIGH |
| **Phase 2** | Move files + update references | 1 hour | MEDIUM |
| **Phase 3** | Restructure README.md | 2-3 hours | LOW |
| **TOTAL** | | **4-6 hours** | |

---

## ✅ Acceptance Criteria

After migration, holmesgpt-api documentation should:

1. ✅ Have all 10 standard documents present at root level
2. ✅ Have `metrics-slos.md` following standard structure
3. ✅ Have implementation plans organized in `implementation/` subdirectory
4. ✅ Have README.md focused on navigation (< 200 lines)
5. ✅ Have all internal cross-references working
6. ✅ Match the structure of 01-signalprocessing/ (reference service)

---

## 🔗 References

- [SERVICE_DOCUMENTATION_GUIDE.md](../../SERVICE_DOCUMENTATION_GUIDE.md) - Authoritative guide
- [01-signalprocessing/README.md](../../crd-controllers/01-signalprocessing/README.md) - Reference service
- [DD-006-controller-scaffolding-strategy.md](../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md) - Standard layout

---

**Document Status**: ✅ **COMPLETED**
**Author**: Kubernaut Development Team
**Completed**: December 3, 2025

---

## ✅ Migration Execution Summary

| Phase | Task | Status |
|-------|------|--------|
| **Phase 1** | Created `metrics-slos.md` with standard structure | ✅ Complete |
| **Phase 2** | Moved 5 files to `implementation/` directory | ✅ Complete |
| **Phase 3** | Restructured README.md (483 → ~150 lines) | ✅ Complete |
| **Phase 4** | Updated 4 cross-references | ✅ Complete |
| **Phase 5** | Verified final structure | ✅ Complete |

### Files Created
- `metrics-slos.md` (9,790 bytes)

### Files Moved
- `IMPLEMENTATION_PLAN_V3.0.md` → `implementation/`
- `IMPLEMENTATION_PLAN_V3.1_RFC7807_GRACEFUL_SHUTDOWN.md` → `implementation/`
- `REMEDIATION-ID-PROPAGATION-IMPLEMENTATION_PLAN_V1.4.md` → `implementation/REMEDIATION-ID-PROPAGATION-V1.4.md`
- `PROMPT_GENERATION_UNIT_TEST_GAP_ANALYSIS.md` → `implementation/`
- `SERVICE_LOCATION_DECISION.md` → `implementation/design/`

### Cross-References Updated
- `BR-HAPI-046-050-DATA-STORAGE-WORKFLOW-TOOL.md`
- `BR-HAPI-046-050-DATA-STORAGE-PLAYBOOK-TOOL.md`
- `BUSINESS_REQUIREMENTS.md`
- `BR_MAPPING.md`

### Deprecated Files
- `observability/PROMETHEUS_QUERIES.md` - Content migrated to `metrics-slos.md` (kept for reference)

