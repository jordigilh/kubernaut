# Phase 1 Prerequisite: CRD Infrastructure Setup

**Date**: October 8, 2025
**Status**: 🔴 **BLOCKING** - Must complete before Phase 1
**Estimated Time**: 2-3 hours
**Priority**: CRITICAL

---

## 🚨 **CRITICAL FINDING**

**Issue**: The Kubernaut project has comprehensive CRD documentation but **no actual CRD type definitions** in the codebase.

**Evidence**:
- ✅ CRD schemas documented in `docs/architecture/CRD_SCHEMAS.md`
- ✅ CRD designs in `docs/design/CRD/`
- ✅ Service specifications reference CRDs
- ❌ **NO `api/` directory with CRD types**
- ❌ **NO Kubebuilder scaffolding**
- ❌ **NO CRD controllers**

**Impact**: **Phase 1 implementation is BLOCKED** until CRD infrastructure is created.

---

## 📋 **Prerequisite Tasks**

### **Option A: Create CRDs from Scratch** (Recommended)

Use Kubebuilder to scaffold the CRD infrastructure based on existing documentation.

#### **Step 1: Initialize Kubebuilder Project** (if not done)

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Check if already initialized
if [ ! -f "PROJECT" ]; then
    # Initialize Kubebuilder
    kubebuilder init \
        --domain kubernaut.io \
        --repo github.com/jordigilh/kubernaut \
        --plugins go/v4
fi
```

#### **Step 2: Create RemediationRequest CRD**

```bash
# Create the API scaffolding
kubebuilder create api \
    --group remediation \
    --version v1 \
    --kind RemediationRequest \
    --resource=true \
    --controller=true \
    --make=false

# This creates:
# - api/remediation/v1/remediationrequest_types.go
# - internal/controller/remediationrequest_controller.go
```

**Then update** `api/remediation/v1/remediationrequest_types.go` with fields from `docs/architecture/CRD_SCHEMAS.md`:
- Signal Identification fields
- Signal Classification fields
- Deduplication fields
- Provider data fields
- **Phase 1 NEW**: `SignalLabels`, `SignalAnnotations`

#### **Step 3: Create RemediationProcessing CRD**

```bash
# Create the API scaffolding
kubebuilder create api \
    --group remediationprocessing \
    --version v1 \
    --kind RemediationProcessing \
    --resource=true \
    --controller=true \
    --make=false

# This creates:
# - api/remediationprocessing/v1/remediationprocessing_types.go
# - internal/controller/remediationprocessing_controller.go
```

**Then update** `api/remediationprocessing/v1/remediationprocessing_types.go` with fields from documentation:
- Parent reference
- **Phase 1 NEW**: All 18 self-contained fields from RemediationRequest
- Enrichment configuration
- Status with EnrichmentResults
- **Phase 1 NEW**: Signal identifiers in status
- **Phase 1 NEW**: OriginalSignal type

#### **Step 4: Create AIAnalysis CRD**

```bash
# Create the API scaffolding
kubebuilder create api \
    --group aianalysis \
    --version v1 \
    --kind AIAnalysis \
    --resource=true \
    --controller=true \
    --make=false

# This creates:
# - api/aianalysis/v1/aianalysis_types.go
# - internal/controller/aianalysis_controller.go
```

**Then update** `api/aianalysis/v1/aianalysis_types.go` with fields from documentation:
- Parent reference
- Analysis request spec
- AI recommendations in status

#### **Step 5: Create Additional CRDs**

```bash
# WorkflowExecution CRD
kubebuilder create api \
    --group workflowexecution \
    --version v1 \
    --kind WorkflowExecution \
    --resource=true \
    --controller=true \
    --make=false

# KubernetesExecution CRD
kubebuilder create api \
    --group kubernetesexecution \
    --version v1 \
    --kind KubernetesExecution \
    --resource=true \
    --controller=true \
    --make=false

# RemediationOrchestrator CRD
kubebuilder create api \
    --group remediationorchestrator \
    --version v1 \
    --kind RemediationOrchestrator \
    --resource=true \
    --controller=true \
    --make=false
```

#### **Step 6: Generate CRD Manifests**

```bash
# Generate DeepCopy implementations
make generate

# Generate CRD manifests
make manifests

# This creates CRD YAMLs in config/crd/bases/
```

#### **Step 7: Verify Structure**

```bash
# Check directory structure
tree api/
# Expected:
# api/
# ├── remediation/v1/
# │   ├── groupversion_info.go
# │   ├── remediationrequest_types.go
# │   └── zz_generated.deepcopy.go
# ├── remediationprocessing/v1/
# ├── aianalysis/v1/
# ├── workflowexecution/v1/
# ├── kubernetesexecution/v1/
# └── remediationorchestrator/v1/

# Check controller structure
tree internal/controller/
# Expected:
# internal/controller/
# ├── remediationrequest_controller.go
# ├── remediationprocessing_controller.go
# ├── aianalysis_controller.go
# ├── workflowexecution_controller.go
# ├── kubernetesexecution_controller.go
# └── remediationorchestrator_controller.go
```

---

### **Option B: Manual CRD Creation** (Not Recommended)

If Kubebuilder is not available, manually create the directory structure and files. **This is significantly more work and error-prone.**

---

## ✅ **Completion Checklist**

### **Infrastructure**
- [ ] Kubebuilder initialized (PROJECT file exists)
- [ ] `api/` directory structure created
- [ ] `internal/controller/` directory structure created
- [ ] All 6 CRD APIs scaffolded

### **RemediationRequest CRD**
- [ ] `api/remediation/v1/remediationrequest_types.go` exists
- [ ] Spec fields match documentation
- [ ] Status fields match documentation
- [ ] Kubebuilder markers added
- [ ] DeepCopy generated

### **RemediationProcessing CRD**
- [ ] `api/remediationprocessing/v1/remediationprocessing_types.go` exists
- [ ] Spec fields match documentation
- [ ] Status fields match documentation
- [ ] Supporting types defined (ResourceIdentifier, DeduplicationContext)
- [ ] DeepCopy generated

### **AIAnalysis CRD**
- [ ] `api/aianalysis/v1/aianalysis_types.go` exists
- [ ] Spec fields match documentation
- [ ] Status fields match documentation
- [ ] Recommendation types defined
- [ ] DeepCopy generated

### **Additional CRDs**
- [ ] WorkflowExecution CRD created
- [ ] KubernetesExecution CRD created
- [ ] RemediationOrchestrator CRD created

### **Generation**
- [ ] `make generate` completes successfully
- [ ] `make manifests` creates CRD YAMLs
- [ ] No compilation errors

---

## 🎯 **After Completion**

**Then you can proceed with Phase 1**:
1. Task 1: Add `SignalLabels` and `SignalAnnotations` to RemediationRequest
2. Task 2: Add 18 self-contained fields to RemediationProcessing.spec
3. Task 3: Add signal identifiers and OriginalSignal to RemediationProcessing.status

---

## 📚 **Reference Documentation**

**CRD Schemas**:
- `docs/architecture/CRD_SCHEMAS.md` - Authoritative schema definitions
- `docs/design/CRD/01_REMEDIATION_REQUEST_CRD.md` - RemediationRequest design
- `docs/design/CRD/02_REMEDIATION_PROCESSING_CRD.md` - RemediationProcessing design
- `docs/design/CRD/03_AI_ANALYSIS_CRD.md` - AIAnalysis design

**Service Specifications**:
- `docs/services/crd-controllers/01-remediationprocessor/` - RemediationProcessor service
- `docs/services/crd-controllers/02-aianalysis/` - AIAnalysis service
- `docs/services/crd-controllers/03-workflowexecution/` - WorkflowExecution service
- `docs/services/crd-controllers/04-kubernetesexecutor/` - KubernetesExecutor service
- `docs/services/crd-controllers/05-remediationorchestrator/` - RemediationOrchestrator service

---

## ⏱️ **Time Estimate**

| Task | Time |
|------|------|
| Initialize Kubebuilder (if needed) | 15 min |
| Create RemediationRequest CRD | 30 min |
| Create RemediationProcessing CRD | 30 min |
| Create AIAnalysis CRD | 30 min |
| Create additional 3 CRDs | 45 min |
| Generate manifests and verify | 15 min |
| **Total** | **~2.5 hours** |

---

## 🚨 **BLOCKER STATUS**

**Current**: Phase 1 implementation is **BLOCKED**
**Reason**: No CRD type definitions exist in codebase
**Resolution**: Complete this prerequisite setup
**Then**: Proceed with Phase 1 implementation

---

**Status**: 🔴 **BLOCKING PREREQUISITE**
**Action Required**: Complete CRD scaffolding before Phase 1
**Reference**: `docs/analysis/PHASE_1_IMPLEMENTATION_GUIDE.md` (blocked until this is complete)

