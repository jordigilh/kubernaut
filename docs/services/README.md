# Kubernaut Services Documentation

**Purpose**: This directory contains specifications, implementation plans, and documentation for all Kubernaut services.

**Last Updated**: 2025-12-07

---

## 📋 Template Index - Quick Reference

### **Primary Templates** (Start Here)

| Template | Use When | Location | Version | Lines |
|----------|----------|----------|---------|-------|
| **[SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md)** | Creating a **NEW service** from scratch | `docs/services/` | v3.0 | 8,187 |
| **[FEATURE_EXTENSION_PLAN_TEMPLATE.md](FEATURE_EXTENSION_PLAN_TEMPLATE.md)** | Adding features to **EXISTING service** | `docs/services/` | v1.4 | 1,279 |
| **[CRD_SERVICE_SPECIFICATION_TEMPLATE.md](../development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md)** | Writing specs for **CRD controllers** | `docs/development/templates/` | v1.1 | 2,095 |

---

## 🎯 Decision Matrix: Which Template to Use?

```
┌─────────────────────────────────────────────────────────────────┐
│ Are you creating a NEW service or extending an EXISTING one?   │
└─────────────────────────────────────────────────────────────────┘
                         │
         ┌───────────────┴───────────────┐
         │                               │
    Creating NEW                  Extending EXISTING
         │                               │
         ▼                               ▼
┌─────────────────────┐       ┌─────────────────────┐
│ What type of        │       │ FEATURE_EXTENSION_  │
│ service?            │       │ PLAN_TEMPLATE.md    │
└─────────────────────┘       └─────────────────────┘
         │
    ┌────┴────┐
    │         │
Stateless  CRD Controller
    │         │
    ▼         ▼
┌──────────┐ ┌──────────────────────┐
│ SERVICE_ │ │ 1. CRD_SERVICE_     │
│ IMPLEM-  │ │    SPECIFICATION_   │
│ ENTATION_│ │    TEMPLATE.md      │
│ PLAN_    │ │ 2. SERVICE_         │
│ TEMPLATE │ │    IMPLEMENTATION_  │
│ .md      │ │    PLAN_TEMPLATE.md │
└──────────┘ └──────────────────────┘
```

---

## 📚 Template Descriptions

### 1. SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md (v3.0)

**Purpose**: Comprehensive 11-12 day implementation plan for new services (stateless or CRD controller).

**Key Features**:
- ✅ Cross-team validation patterns (HANDOFF/RESPONSE)
- ✅ Pre-implementation validation gate
- ✅ CRD API group standard (DD-CRD-001)
- ✅ Day-by-day implementation phases (Day 1-12)
- ✅ Business Requirements (BR) documentation
- ✅ Design Decisions (DD) templates
- ✅ Testing strategy (Unit → Integration → E2E)
- ✅ Port allocation standards (DD-TEST-001)
- ✅ Logging framework (DD-005 unified `logr.Logger`)
- ✅ Industry best practices tables

**Use For**:
- New stateless services (Gateway, Data Storage, HolmesGPT API)
- New CRD controllers (SignalProcessing, AIAnalysis, WorkflowExecution)
- Services requiring 11-12+ days of implementation

**File Naming Convention**: `IMPLEMENTATION_PLAN_V<semantic_version>.md`

**Example Usage**:
```bash
# Copy template for new service
cp docs/services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md \
   docs/services/stateless/my-new-service/IMPLEMENTATION_PLAN_V1.0.md
```

---

### 2. FEATURE_EXTENSION_PLAN_TEMPLATE.md (v1.4)

**Purpose**: Implementation plan for adding features to existing services (3-12 days).

**Key Features**:
- ✅ APDC methodology integration (Analysis-Plan-Do-Check)
- ✅ Feature-scoped implementation (not full service)
- ✅ TDD discipline and test pyramid compliance
- ✅ Critical pitfalls from past implementations
- ✅ Phase-by-phase breakdown with validation gates
- ✅ Design Decision (DD) integration
- ✅ Business Requirement (BR) mapping

**Use For**:
- Adding new endpoints to existing HTTP services
- Extending CRD controller reconciliation logic
- Feature flags and capability extensions
- Bug fixes requiring significant refactoring

**File Naming Convention**: `[CONTEXT_PREFIX_]IMPLEMENTATION_PLAN[_FEATURE_SUFFIX]_V1.4.md`

**Example Usage**:
```bash
# Copy template for feature extension
cp docs/services/FEATURE_EXTENSION_PLAN_TEMPLATE.md \
   docs/services/stateless/gateway-service/STORM_AGGREGATION_IMPLEMENTATION_V1.0.md
```

---

### 3. CRD_SERVICE_SPECIFICATION_TEMPLATE.md (v1.1)

**Purpose**: Specification template for CRD-based services (directory structure format).

**Key Features**:
- ✅ Directory-per-service structure (modern approach)
- ✅ CRD schema design patterns
- ✅ Controller reconciliation logic patterns
- ✅ Finalizer and cleanup patterns
- ✅ Event recording standards
- ✅ Business Requirements mapping (`BR_MAPPING.md`)
- ✅ Service-specific file guidelines

**Use For**:
- Defining new CRD controllers
- Documenting CRD schema evolution
- Planning controller reconciliation logic
- Cross-referencing with implementation plans

**Directory Structure**:
```
docs/services/crd-controllers/
└── 06-newservice/
    ├── README.md                    # Service navigation hub
    ├── SPECIFICATION.md             # Core specification
    ├── CRD_SCHEMA.md               # Custom Resource Definition
    ├── CONTROLLER_LOGIC.md         # Reconciliation logic
    ├── BR_MAPPING.md               # Business Requirements
    └── implementation/
        └── IMPLEMENTATION_PLAN_V1.0.md
```

**Example Usage**:
```bash
# Create new CRD service directory
mkdir -p docs/services/crd-controllers/06-newservice
cp docs/development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md \
   docs/services/crd-controllers/06-newservice/SPECIFICATION.md
```

---

## 🗂️ Service Directory Structure

```
docs/services/
├── README.md                                    (THIS FILE)
├── SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md      (v3.0 - New services)
├── FEATURE_EXTENSION_PLAN_TEMPLATE.md           (v1.4 - Feature extensions)
│
├── stateless/                                   (HTTP-based services)
│   ├── gateway-service/
│   │   ├── SPECIFICATION.md
│   │   ├── BUSINESS_REQUIREMENTS.md
│   │   └── implementation/
│   │       └── IMPLEMENTATION_PLAN_V*.md
│   ├── data-storage/
│   ├── holmesgpt-api/
│   └── notification/
│
└── crd-controllers/                             (Kubernetes controllers)
    ├── 01-remediation-processor/
    ├── 02-aianalysis/
    ├── 03-workflowexecution/
    ├── 04-signalprocessing/
    ├── 05-remediationorchestrator/
    └── 06-newservice/                           (Example structure)
        ├── README.md
        ├── SPECIFICATION.md
        ├── CRD_SCHEMA.md
        ├── CONTROLLER_LOGIC.md
        ├── BR_MAPPING.md
        └── implementation/
            └── IMPLEMENTATION_PLAN_V1.0.md
```

---

## 📖 Additional Templates and Resources

### Supporting Templates

| Template | Purpose | Location |
|----------|---------|----------|
| **EOD_TEMPLATES.md** | End-of-Day reporting format | Various service directories |
| **DEPLOYMENT_YAML_TEMPLATE.md** | Kubernetes deployment manifests | `docs/architecture/` |
| **PODMAN_INTEGRATION_TEST_TEMPLATE.md** | Podman-based integration tests | `docs/testing/` |
| **KIND_CLUSTER_TEST_TEMPLATE.md** | KIND cluster test setup | `docs/testing/` |
| **GO_CODE_SAMPLE_TEMPLATE.md** | Go code examples and patterns | `docs/services/` |

### Key Design Decisions

| DD | Title | Relevance |
|----|-------|-----------|
| **DD-005** | Unified Logging Framework (`logr.Logger`) | All services MUST use `logr.Logger` for logging |
| **DD-CRD-001** | CRD API Group Standard (`.ai` domain) | All CRD controllers use unified API group |
| **DD-TEST-001** | Port Allocation Standards | Service port assignments |
| **DD-API-001** | HTTP Header vs JSON Body Pattern | API design consistency |
| **DD-006** | Controller Scaffolding Strategy | CRD controller patterns |

---

## 🚀 Quick Start Guide

### Creating a New Stateless Service

```bash
# Step 1: Create service directory
mkdir -p docs/services/stateless/my-service/implementation

# Step 2: Copy implementation plan template
cp docs/services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md \
   docs/services/stateless/my-service/implementation/IMPLEMENTATION_PLAN_V1.0.md

# Step 3: Edit the implementation plan
# - Replace [Service Name] with your service name
# - Define Business Requirements (BR-MYSERVICE-XXX)
# - Document Design Decisions (DD-MYSERVICE-XXX)
# - Plan Day 1-12 implementation phases

# Step 4: Create SPECIFICATION.md and BUSINESS_REQUIREMENTS.md
```

### Creating a New CRD Controller

```bash
# Step 1: Create CRD service directory
mkdir -p docs/services/crd-controllers/06-mycontroller/implementation

# Step 2: Copy CRD specification template
cp docs/development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md \
   docs/services/crd-controllers/06-mycontroller/SPECIFICATION.md

# Step 3: Copy implementation plan template
cp docs/services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md \
   docs/services/crd-controllers/06-mycontroller/implementation/IMPLEMENTATION_PLAN_V1.0.md

# Step 4: Edit both templates
# - Define CRD schema (API version: *.ai/v1alpha1)
# - Document reconciliation logic
# - Plan implementation phases (11-12 days)
```

### Extending an Existing Service

```bash
# Step 1: Navigate to service directory
cd docs/services/stateless/gateway-service/implementation

# Step 2: Copy feature extension template
cp ../../FEATURE_EXTENSION_PLAN_TEMPLATE.md \
   MY_FEATURE_IMPLEMENTATION_V1.0.md

# Step 3: Edit the feature plan
# - Reference existing Design Decision (DD-XXX-YYY)
# - Document new Business Requirements (BR-GATEWAY-XXX)
# - Plan 3-12 day implementation phases
```

---

## 📋 Standard File Naming Conventions

| File Type | Naming Convention | Example |
|-----------|-------------------|---------|
| **Implementation Plan** | `IMPLEMENTATION_PLAN_V<version>.md` | `IMPLEMENTATION_PLAN_V1.3.md` |
| **Feature Extension** | `[CONTEXT_]IMPLEMENTATION_PLAN[_FEATURE]_V<version>.md` | `STORM_AGGREGATION_IMPLEMENTATION_V1.0.md` |
| **Specification** | `SPECIFICATION.md` | `SPECIFICATION.md` |
| **Business Requirements** | `BUSINESS_REQUIREMENTS.md` | `BUSINESS_REQUIREMENTS.md` |
| **Design Decision** | `DD-<SERVICE>-<NUMBER>-<title>.md` | `DD-GATEWAY-009-state-based-deduplication.md` |

---

## 🔍 Template Version History

### SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md

| Version | Date | Key Changes |
|---------|------|-------------|
| v3.0 | 2025-12-01 | Cross-team validation + CRD API group standard |
| v2.9 | 2025-11-30 | Code example DD-005 compliance |
| v2.8 | 2025-11-28 | Unified logging framework (DD-005 v2.0) |
| v2.7 | 2025-11-28 | Scope annotations + test helpers + OpenAPI pre-phase |
| v2.6 | 2025-11-28 | Pre-implementation design decisions + API patterns |

### FEATURE_EXTENSION_PLAN_TEMPLATE.md

| Version | Date | Key Changes |
|---------|------|-------------|
| v1.4 | 2025-11-28 | APDC methodology integration |
| v1.3 | 2025-11-20 | TDD discipline and test pyramid |

### CRD_SERVICE_SPECIFICATION_TEMPLATE.md

| Version | Date | Key Changes |
|---------|------|-------------|
| v1.1 | 2025-11-30 | Document classification + BR_MAPPING.md |
| v1.0 | 2025-10-15 | Initial template structure |

---

## 🛠️ Validation and Compliance

### Before Starting Implementation

- [ ] Template version matches filename version
- [ ] Business Requirements (BR-XXX-XXX) documented
- [ ] Design Decisions (DD-XXX-XXX) created and referenced
- [ ] Cross-team dependencies identified (if applicable)
- [ ] Port allocation validated (DD-TEST-001)
- [ ] Logging framework compliance (DD-005)
- [ ] CRD API group compliance (DD-CRD-001, if CRD controller)

### Validation Scripts

```bash
# Validate ADR/DD references
./scripts/validate_adr_references.sh

# Check service port allocations
grep -r "ServicePort" docs/services/ --include="*.md"

# Verify BR documentation
find docs/services -name "BUSINESS_REQUIREMENTS.md" -type f
```

---

## 📞 Support and Questions

**Template Issues**: If you find issues with templates, update them and bump the version number.

**Design Decision Conflicts**: Refer to [DD-013-conflict-resolution-matrix.md](../architecture/decisions/DD-013-conflict-resolution-matrix.md)

**BR Template Standards**: Refer to [ADR-037-business-requirement-template-standard.md](../architecture/decisions/ADR-037-business-requirement-template-standard.md)

---

## 🎯 Template Selection Checklist

Use this checklist to select the right template:

```
□ Are you creating a NEW service?
  └─ YES → Use SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md
  └─ NO  → Continue to next question

□ Are you extending an EXISTING service?
  └─ YES → Use FEATURE_EXTENSION_PLAN_TEMPLATE.md
  └─ NO  → Continue to next question

□ Are you documenting a CRD controller specification?
  └─ YES → Use CRD_SERVICE_SPECIFICATION_TEMPLATE.md
  └─ NO  → Consult with team lead

□ Is your implementation < 3 days?
  └─ YES → Consider if a formal plan is needed
  └─ NO  → Use appropriate template above
```

---

**Last Updated**: 2025-12-07
**Maintained By**: Kubernaut Development Team
**Template Versions**: SERVICE v3.0 | FEATURE v1.4 | CRD v1.1
