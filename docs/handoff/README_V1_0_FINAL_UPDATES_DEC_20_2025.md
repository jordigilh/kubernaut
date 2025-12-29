# README V1.0 Final Updates - December 20, 2025

**Date**: December 20, 2025
**Status**: 🔄 **IN PROGRESS**
**Purpose**: Final README.md updates for V1.0 completion

---

## 📋 **Executive Summary**

**Two Critical Updates Required**:
1. ✅ **Dynamic Toolset Cleanup**: Remove all references (code deleted, avoid confusion)
2. ✅ **Must Gather Feature Addition**: Add BR-PLATFORM-001 (P1 priority, parallel to SOC2)

---

## 🗑️ **UPDATE 1: Dynamic Toolset Reference Cleanup**

### **User Requirement**
> "triage for the Dynamic Toolset service references in the main README.md and other authoritative documents to remove references to it. It's reimplementation will depend on feedback of the current v1.0. For now I don't want to reference it in the main documentation to avoid confusion since it won't be in v1.x"

### **Rationale**
- ✅ Code deleted (commit 9f0c2c71: `pkg/toolset/`, `cmd/dynamictoolset/`)
- ✅ Deprecation notices added
- ❌ Avoid confusion - reimplementation depends on V1.0 feedback
- ❌ Not in v1.x - no user-facing references

---

### **README.md Changes Required (6 locations)**

#### **Change 1: Remove Service Status Table Row (Line 79)**
**Current**:
```markdown
| **~~Dynamic Toolset~~** | ❌ **Deferred to V2.0** | Service discovery (DD-016) | 8 BRs (redundant with HolmesGPT-API) |
```

**Fix**: **DELETE entire line**

**Rationale**: Don't reference in user-facing service list

---

#### **Change 2: Update Service Count Comment (Line 100)**
**Current**:
```markdown
- 📊 **V1.0 Service Count**: 8 production-ready services (11 original - Context API deprecated - Dynamic Toolset deferred to V2.0 - Effectiveness Monitor deferred to V1.1)
```

**Fix**:
```markdown
- 📊 **V1.0 Service Count**: 8 production-ready services (10 original - Context API deprecated - Effectiveness Monitor deferred to V1.1)
```

**Rationale**: Update count math (10 original not 11)

---

#### **Change 3: Remove Build Command (Line 125)**
**Current**:
```bash
go build -o bin/dynamic-toolset ./cmd/dynamictoolset
```

**Fix**: **DELETE entire line**

**Rationale**: cmd/dynamictoolset no longer exists

---

#### **Change 4: Remove from Service Documentation Navigation (Line 254)**
**Current**:
```markdown
- **[Stateless Services](docs/services/stateless/)**: Gateway Service, Dynamic Toolset, Data Storage Service, HolmesGPT API, Notification Service, Effectiveness Monitor
```

**Fix**:
```markdown
- **[Stateless Services](docs/services/stateless/)**: Gateway Service, Data Storage Service, HolmesGPT API, Notification Service, Effectiveness Monitor
```

**Rationale**: Remove from user-facing navigation

---

#### **Change 5: Remove Test Status Table Row (Line 278)**
**Current**:
```markdown
| **Dynamic Toolset** | - | - | - | **Deferred to V2.0** | **DD-016** |
```

**Fix**: **DELETE entire table row**

**Rationale**: Not part of V1.0 test status

---

#### **Change 6: Update Test Count Note (Line 285)**
**Current**:
```markdown
*Note: ... Dynamic Toolset (245 tests) deferred to V2.0 per DD-016.*
```

**Fix**: **DELETE "Dynamic Toolset (245 tests) deferred to V2.0 per DD-016." from note**

**Rationale**: Don't mention in V1.0 test status

---

## 🔧 **UPDATE 2: Must Gather Feature Addition**

### **User Requirement**
> "ah, and we're missing the Must Gather feature for v1.0. There is already a BR or and ADR that documents it. We should make sure it's also reflected in the @README.md . Implementation will be in parallel to SOC2"

### **Must Gather Overview**

| Property | Value |
|----------|-------|
| **BR Document** | BR-PLATFORM-001-must-gather-diagnostic-collection.md |
| **Priority** | P1 - Required for Production Supportability |
| **Purpose** | Standardized container-based diagnostic collection tool |
| **Pattern** | OpenShift must-gather industry standard |
| **Status** | BR complete, triage complete, ready for implementation |
| **Timeline** | **Parallel to SOC2 compliance (Week 1-3)** |

**Key Capabilities**:
- Container image: `quay.io/jordigilh/must-gather:latest`
- Collects all Kubernaut CRDs, service logs, configurations
- OpenShift & vanilla Kubernetes compatible
- Automated sanitization of sensitive data
- Reduces MTTR by 50-70%

---

### **README.md Changes Required (3 locations)**

#### **Addition 1: Add to "Key Capabilities" Section (After Line 32)**
**Current**:
```markdown
- **Production-Ready**: 2,450+ tests passing, SOC2-compliant audit traces, 8 of 8 V1.0 services ready
```

**Add After**:
```markdown
- **Enterprise Diagnostics**: Must-gather diagnostic collection following OpenShift industry standard (BR-PLATFORM-001)
```

**Rationale**: Highlight production supportability capability

---

#### **Addition 2: Add to Service Status Table (After Line 78)**
**Add Row**:
```markdown
| **Must-Gather Diagnostic Tool** | 🔄 **In Development (Week 1-3)** | Enterprise diagnostic collection | BR-PLATFORM-001 (parallel to SOC2) |
```

**Note Position**: Add as last row before `~~Effectiveness Monitor~~`

**Rationale**: Show active V1.0 development work

---

#### **Addition 3: Add to "Recent Updates" Section (After Line 99)**
**Add Line**:
```markdown
- 🔄 **Must-Gather V1.0**: Enterprise diagnostic collection tool (BR-PLATFORM-001), OpenShift-compatible, parallel to SOC2 implementation
```

**Rationale**: Highlight current development activity

---

#### **Addition 4: Add Quick Start Section for Must-Gather (After Line 211)**
**Add New Subsection**:
```markdown
### **Diagnostic Collection - Must-Gather**

Kubernaut provides industry-standard diagnostic collection following the OpenShift must-gather pattern:

```bash
# OpenShift-style (oc adm must-gather)
oc adm must-gather --image=quay.io/jordigilh/must-gather:latest

# Kubernetes-style (kubectl debug)
kubectl debug node/<node-name> \
  --image=quay.io/jordigilh/must-gather:latest \
  --image-pull-policy=Always -- /usr/bin/gather

# Direct pod execution (fallback)
kubectl run kubernaut-must-gather \
  --image=quay.io/jordigilh/must-gather:latest \
  --rm --attach -- /usr/bin/gather
```

**Collects**:
- All Kubernaut CRDs (RemediationRequests, SignalProcessings, AIAnalyses, etc.)
- Service logs (Gateway, Data Storage, HolmesGPT API, CRD controllers)
- Configurations (ConfigMaps, Secrets sanitized)
- Tekton Pipelines (PipelineRuns, TaskRuns, logs)
- Database infrastructure (PostgreSQL, Redis)
- Metrics snapshots and audit event samples

**Output**: Compressed tarball (`kubernaut-must-gather-<cluster>-<timestamp>.tar.gz`)

**Status**: 🔄 **In Development** (Week 1-3, parallel to SOC2 compliance)
**Documentation**: See [BR-PLATFORM-001](docs/requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md)
```

**Rationale**: Provide user guidance for diagnostic collection

---

## 📊 **Summary of All README Changes**

| Change Type | Count | Sections Affected |
|-------------|-------|-------------------|
| **Dynamic Toolset Deletions** | 6 | Service table, build, navigation, tests, notes |
| **Must Gather Additions** | 4 | Key capabilities, service table, recent updates, quick start |
| **Total Changes** | **10** | **7 sections** |

---

## ✅ **Implementation Plan**

### **Phase 1: Dynamic Toolset Cleanup** (15 minutes)
1. Delete service status table row (Line 79)
2. Update service count comment (Line 100)
3. Delete build command (Line 125)
4. Remove from service navigation (Line 254)
5. Delete test status table row (Line 278)
6. Update test count note (Line 285)

### **Phase 2: Must Gather Addition** (20 minutes)
1. Add to Key Capabilities (Line 32)
2. Add to Service Status Table (Line 78)
3. Add to Recent Updates (Line 99)
4. Add Quick Start section (Line 211)

### **Phase 3: Validation** (10 minutes)
1. Verify all Dynamic Toolset references removed
2. Verify Must Gather properly positioned
3. Check markdown formatting
4. Build verification (`go build ./...`)

**Total Time**: ~45 minutes

---

## 🎯 **Validation Checklist**

**Dynamic Toolset Cleanup**:
- [ ] ZERO mentions of "Dynamic Toolset" in user-facing sections
- [ ] Service count accurate (8 production-ready, 10 original)
- [ ] Test count table excludes Dynamic Toolset
- [ ] Build commands exclude `cmd/dynamictoolset`
- [ ] Service navigation lists exclude Dynamic Toolset
- [ ] `docs/services/stateless/dynamic-toolset/` still exists (preserved with deprecation notice)

**Must Gather Addition**:
- [ ] Mentioned in Key Capabilities
- [ ] Listed in Service Status Table (with "In Development" status)
- [ ] Included in Recent Updates (parallel to SOC2)
- [ ] Quick Start section added with usage examples
- [ ] BR-PLATFORM-001 referenced
- [ ] Timeline clarified (Week 1-3, parallel to SOC2)

**General**:
- [ ] Markdown formatting valid
- [ ] No broken links
- [ ] Consistent terminology
- [ ] `go build ./...` succeeds

---

## 📝 **Git Commit Message**

```
docs: README V1.0 final updates - Dynamic Toolset cleanup + Must Gather addition

Complete final README updates for V1.0 release preparation

Dynamic Toolset Cleanup (User Request: Avoid confusion):
❌ Remove from service status table
❌ Remove from build commands (cmd/dynamictoolset deleted)
❌ Remove from service navigation
❌ Remove from test status table
❌ Update service count (10 original → 8 v1.0)
✅ Keep documentation for V2.0 planning (with deprecation notices)

Must Gather Feature Addition (BR-PLATFORM-001):
✅ Add to Key Capabilities (enterprise diagnostics)
✅ Add to Service Status Table (In Development, Week 1-3)
✅ Add to Recent Updates (parallel to SOC2 compliance)
✅ Add Quick Start section with usage examples
   - OpenShift-style: oc adm must-gather
   - Kubernetes-style: kubectl debug
   - Direct pod execution
✅ Link to BR-PLATFORM-001 for full specification

Rationale:
- Dynamic Toolset: Code deleted (9f0c2c71), reimplementation depends on V1.0 feedback
- Must Gather: P1 production supportability, industry-standard diagnostic collection
- Timeline: Must Gather implementation parallel to SOC2 compliance (Week 1-3)

V1.0 Status:
- 8 production-ready services (100%)
- SOC2 compliance complete (Week 1)
- Must Gather in active development (Week 1-3)
- Effectiveness Monitor deferred to V1.1
- Dynamic Toolset deferred to V2.0 (pending V1.0 feedback)

Authority:
- User guidance: Dec 20, 2025 (Dynamic Toolset cleanup)
- BR-PLATFORM-001: Must-Gather Diagnostic Collection (P1 priority)
- DD-016: Dynamic Toolset V2.0 deferral
- DD-017: Effectiveness Monitor V1.1 deferral

Impact: Accurate V1.0 feature set, clear development timeline, production-ready documentation
```

---

## 🚀 **Next Steps**

1. ✅ **This Document**: Created comprehensive update plan
2. ⏳ **Apply Changes**: Execute 10 README updates
3. ⏳ **Validate**: Run validation checklist
4. ⏳ **Commit**: Use provided git commit message
5. ⏳ **Verify**: Ensure no Dynamic Toolset references in user-facing docs

**Status**: Plan complete, ready for execution
**ETA**: ~45 minutes total

---

**Created**: December 20, 2025
**Authority**: User guidance + BR-PLATFORM-001 + DD-016
**Confidence**: 95% - Clear requirements, straightforward updates
**Next Step**: Apply 10 README changes (6 deletions + 4 additions)












