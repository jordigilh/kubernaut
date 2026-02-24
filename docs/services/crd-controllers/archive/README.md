# Archived Service Specifications

**Archive Date**: 2025-01-15
**Reason**: Documentation Restructure to Directory-per-Service
**Status**: ✅ Superseded by new directory structure

---

## 📦 What's in This Archive

This directory contains the **original monolithic service specification documents** that have been superseded by the new directory-per-service structure.

| Original File | Size | Lines | Superseded By |
|---------------|------|-------|---------------|
| `01-alert-processor.md` | 178K | ~5,007 | [../01-alertprocessor/](../01-alertprocessor/) (14 documents) |
| `02-ai-analysis.md` | 178K | ~5,249 | [../02-aianalysis/](../02-aianalysis/) (15 documents) |
| `03-workflow-execution.md` | 106K | ~2,807 | [../03-workflowexecution/](../03-workflowexecution/) (14 documents) |
| `04-kubernetes-executor.md` | 83K | ~2,359 | [../04-kubernetesexecutor/](../04-kubernetesexecutor/) (15 documents) (DEPRECATED - ADR-025) |
| `05-remediation-orchestrator.md` | 109K | ~3,046 | [../05-remediationorchestrator/](../05-remediationorchestrator/) (15 documents) |
| `05-central-controller.md` | 110K | ~3,149 | **OBSOLETE** - Uses deprecated "AlertRemediation" CRD naming (removed 2025-10-06) |

**Total**: 6 files (5 migrated + 1 obsolete), 764K, ~22,458 lines → **73 focused documents** in structured directories

---

## 🚫 DO NOT USE THESE FILES

**Status**: These files are **archived** and should **NOT** be used for:
- ❌ Implementation guidance (use new directories instead)
- ❌ Reference material (use structured directories instead)
- ❌ Updates or modifications (update structured directories instead)

**Reason**: Content has been migrated to self-contained directories with improved organization.

---

## ⚠️ NAMING DEPRECATION NOTICE

**CRITICAL**: These archived documents use **"Alert" prefix naming** extensively (e.g., `AlertService`, `AlertProcessing`, `ProcessAlert()`), which is **DEPRECATED** and **semantically incorrect**.

### **Why "Alert" Prefix is Deprecated**

Kubernaut processes **multiple signal types**, not just Prometheus alerts:
- ✅ Prometheus Alerts
- ✅ Kubernetes Events
- ✅ AWS CloudWatch Alarms
- ✅ Custom Webhooks
- ✅ Future Signal Sources

Using "Alert" prefix creates **semantic confusion** and implies the system only handles alerts.

### **Current Naming Standards (Active Codebase)**

| Deprecated (Archive) | Current (Active Code) | Reason |
|---------------------|----------------------|---------|
| `AlertService` | `SignalProcessorService` | Handles ANY signal type |
| `AlertProcessing` (CRD) | `RemediationProcessing` | CRD processes all signals |
| `AlertRemediation` (CRD) | `RemediationOrchestration` | Orchestrates all signal types |
| `AlertContext` | `SignalContext` | Context for any signal |
| `AlertMetrics` | `SignalProcessingMetrics` | Metrics per signal type |
| `ProcessAlert()` | `ProcessSignal()` | Method handles any signal |

### **References**

- **Migration Decision**: [ADR-015: Alert to Signal Naming Migration](../../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)
- **Detailed Analysis**: [Alert Prefix Naming Triage Report](../../../analysis/ALERT_PREFIX_NAMING_TRIAGE.md)
- **Evidence**: RemediationOrchestrator CRD uses `SignalType` field (see [crd-schema.md](../05-remediationorchestrator/crd-schema.md:41))

### **⚠️ Warning for Developers**

If you copy code examples from these archived documents:
1. ⚠️ **Replace** `Alert` prefix with `Signal` prefix
2. ⚠️ **Use** `SignalType` field for signal discrimination
3. ⚠️ **Reference** active documentation in parent directories
4. ⚠️ **Do NOT** create parallel services for each signal type (e.g., `EventService`, `CloudWatchService`)

**DO NOT copy-paste from archive without updating naming conventions.**

---

## ✅ Use This Instead

### **Modern Documentation Structure**

```
docs/services/crd-controllers/
├── 01-alertprocessor/              ✅ USE THIS
│   ├── README.md (navigation hub)
│   └── [13 focused documents]
│
├── 02-aianalysis/                  ✅ USE THIS
│   ├── README.md (navigation hub)
│   └── [14 focused documents]
│
├── 03-workflowexecution/           ✅ USE THIS
│   ├── README.md (navigation hub)
│   └── [13 focused documents]
│
├── 04-kubernetesexecutor/          ✅ USE THIS (DEPRECATED - ADR-025)
│   ├── README.md (navigation hub)
│   └── [14 focused documents]
│
└── 05-remediationorchestrator/           ✅ USE THIS
    ├── README.md (navigation hub)
    └── [14 focused documents]
```

### **How to Navigate New Structure**

1. **Go to service directory**: `cd ../01-alertprocessor/`
2. **Read README first**: `cat README.md` - provides navigation hub
3. **Follow links**: README points to specific documents for different purposes
4. **Progressive disclosure**: Read only what you need (30 min vs 2+ hours)

---

## 📋 What Changed

### **Before (Archived Files)**

- ❌ **5,000+ line monolithic documents**
- ❌ **Hard to navigate** (endless scrolling)
- ❌ **Merge conflicts** (single file editing)
- ❌ **Cognitive overload** (too much information at once)
- ❌ **No progressive disclosure** (all or nothing)

### **After (New Directories)**

- ✅ **14-15 focused documents per service** (~200-800 lines each)
- ✅ **Easy navigation** (README hub + targeted files)
- ✅ **Zero merge conflicts** (multiple files for parallel work)
- ✅ **Bite-sized reading** (focus on what you need)
- ✅ **Progressive disclosure** (30 min to understand vs 2+ hours)

---

## 🔍 Why We Archived

### **Problem Statement**

**Original monolithic documents caused**:
1. **Cognitive Overload**: 5,000+ lines impossible to digest
2. **Navigation Difficulty**: Hard to find specific information
3. **Merge Conflicts**: Multiple developers editing same file
4. **Poor Git Diffs**: Unclear what section changed
5. **Slow Onboarding**: 2+ hours to understand a service

### **Solution: Directory Structure**

**New structured directories provide**:
1. **Self-Containment**: Everything for a service in one directory
2. **Progressive Disclosure**: README → overview → deep dive
3. **Parallel Work**: 14+ files = zero conflicts
4. **Clear Git Diffs**: See exactly which section changed
5. **Fast Onboarding**: 30 minutes to understand

### **Metrics**

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Max Document Size** | 5,249 lines | 916 lines | 82% reduction |
| **Avg Document Size** | 3,862 lines | 735 lines | 81% reduction |
| **Time to Understand** | 2+ hours | 30 min | 75% faster |
| **Merge Conflicts** | Frequent | Rare | ⭐⭐⭐⭐⭐ |

---

## 📚 Migration Details

### **No Content Loss**

✅ **All content preserved** - every line from original documents exists in new directories
✅ **Enhanced organization** - content reorganized into logical sections
✅ **Improved readability** - common patterns marked with headers
✅ **Better navigation** - README provides clear entry points

### **Structure Improvements**

**Original**: Single monolithic markdown file per service
**New**: 13-15 focused documents per service:

1. `README.md` - Navigation hub & quick start
2. `overview.md` - Architecture & key decisions
3. `crd-schema.md` - CRD type definitions
4. `controller-implementation.md` - Reconciler logic
5. `reconciliation-phases.md` - Phase transitions
6. `finalizers-lifecycle.md` - Cleanup & lifecycle
7. `testing-strategy.md` - Test patterns
8. `security-configuration.md` - Security patterns
9. `observability-logging.md` - Logging & tracing
10. `metrics-slos.md` - Prometheus & Grafana
11. `database-integration.md` - Audit storage
12. `integration-points.md` - Service coordination
13. `migration-current-state.md` - Existing code
14. `implementation-checklist.md` - APDC-TDD phases
15. `[service-specific-unique].md` - Service-unique documents

---

## 🔗 See Also

- [RESTRUCTURE_COMPLETE.md](../RESTRUCTURE_COMPLETE.md) - Full restructure documentation
- [MAINTENANCE_GUIDE.md](../MAINTENANCE_GUIDE.md) - How to maintain new structure
- [CRD Service Specification Template](../../../development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md) - Updated template

---

## 📞 Questions?

**Why keep these archived files?**
- Historical reference
- Audit trail of migration
- Verification that no content was lost

**Can I reference these files?**
- For historical purposes: Yes
- For implementation guidance: No (use new directories)
- For updates: No (update new directories)

**When can these be deleted?**
- After implementation phase completes
- After team confirms new structure works
- Recommended: Keep for 3-6 months, then delete

---

**Archive Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-01-15
**Status**: ✅ Superseded - Use new directories instead

