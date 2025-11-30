# DD-WORKFLOW-008: Workflow Feature Roadmap

**Date**: 2025-11-15
**Status**: Planning
**Related**: DD-WORKFLOW-012 (Workflow Immutability)

---

## 🔗 **Workflow Immutability Reference**

**CRITICAL**: All versioning strategies in this roadmap assume workflow immutability.

**Authority**: **DD-WORKFLOW-012: Workflow Immutability Constraints**
- Workflows are immutable at the (workflow_id, version) level
- To change workflow content, create a new version
- Version history is preserved for audit trail

**Cross-Reference**: All version management features (v1.1, v1.2, v2.0) operate within DD-WORKFLOW-012 immutability constraints.

---

---

## Version Roadmap

### v1.0 (Current/Baseline)
**Status**: Foundation

**Features**:
- ✅ Tekton pipeline execution
- ✅ Workflow catalog table with semantic search (PostgreSQL + pgvector)
- ✅ Semantic search REST API (`GET /api/v1/playbooks/search`)
- ✅ Workflow execution history (audit trail for effectiveness monitoring)
- ✅ Operator-defined remediation scripts (container images)
- ✅ Manual workflow registration via SQL inserts

**Limitations**:
- ❌ No parameter schema extraction or validation
- ❌ No workflow write REST API (SQL-only management)
- ❌ No CRD-based workflow management
- ❌ No workflow catalog controller
- ❌ No automated registration workflow
- ❌ Operators must manually insert workflow metadata, parameters, and labels via SQL

---

### v1.1 (Planned) - CRD-Based Management & Schema Validation
**Status**: In Planning
**Focus**: Automated workflow lifecycle management with CRD + REST API

#### Core Features

**1. Workflow CRUD REST API** (NEW)
- POST /api/v1/playbooks (create/update playbook)
- PATCH /api/v1/playbooks/{id} (disable/enable)
- GET /api/v1/playbooks/versions (version history)
- Semantic version validation
- Lifecycle management (draft → active → deprecated)

**2. RemediationWorkflow CRD** (NEW)
- Kubernetes-native workflow registration
- CRD: `kind: RemediationWorkflow`
- RBAC controls for workflow registration
- Audit trail via Kubernetes events

**3. Workflow Registry Controller** (NEW)
- Watches RemediationWorkflow CRDs
- Validates workflow specs
- Calls Data Storage REST API
- Reconciles CRD state with database

**4. Parameter Schema Extraction & Validation** (NEW)
- Extract `/playbook-schema.json` from container images
- JSON Schema format validation
- Registration-time parameter validation
- Parameter type checking and dependency validation

**5. Automated Registration Workflow** (NEW)
- CRD creation triggers registration
- Image pull and schema extraction
- Schema validation before catalog update
- Atomic catalog updates

**6. Business Context Integration** (NEW)
- Business context in incident data
- Business context in workflow labels
- LLM considers business constraints
- Documented in BR-AI-057

#### Out of Scope for v1.1
- ❌ Image signing (Sigstore/Cosign) → v1.2 or v2.0
- ❌ Workflow versioning/rollback → v1.2
- ❌ Multi-playbook orchestration → v2.0

---

### v1.2 (Future) - Security & Advanced Features
**Status**: Planned
**Focus**: Image signing, versioning, advanced remediation

#### Planned Features

**1. Image Signing (Sigstore/Cosign)**
- Workflow containers signed with Sigstore
- Signature verification during registration
- Policy enforcement (only signed images)
- Audit trail of signatures

**2. Workflow Versioning & Rollback**
- Multiple versions in catalog
- Version selection logic
- Rollback capabilities
- Deprecation workflow

**3. Additional Findings Integration**
- LLM discoveries beyond workflow scope
- Multi-resource remediation
- Dependency tracking

---

### v2.0 (Vision) - Advanced Orchestration & Custom Agent
**Status**: Vision
**Focus**: Multi-playbook workflows, custom agent architecture, toolset strategy

#### Strategic Evaluations Required

**1. HolmesGPT Replacement Evaluation** 🔍 **REQUIRES DETAILED ANALYSIS**
- **Problem**: HolmesGPT not designed as REST API service, causing dependency risks
- **Proposal**: Replace with custom kubernaut agent
- **Timeline**: After v1.1 release
- **Decision Needed**: Build vs. continue with HolmesGPT
- **Analysis Required**:
  - Compatibility risks with HolmesGPT SDK evolution
  - Maintenance burden of forked/wrapped HolmesGPT
  - Cost/benefit of custom agent development
  - Migration path from v1.x to v2.0
  - Impact on existing integrations

**2. Toolset Integration Strategy Evaluation** 🔍 **REQUIRES DETAILED ANALYSIS**
- **Problem**: Two approaches for exposing tools to LLM
  - **Option A**: Embed tools directly in container (e.g., kubectl in holmesgpt-api image)
  - **Option B**: Expose tools via MCP (current MVP approach)
- **Timeline**: Evaluate for v2.0 (not needed for v1.x)
- **Decision Needed**: Direct embedding vs. MCP-based toolsets
- **Analysis Required**:
  - Performance comparison (latency, throughput)
  - Maintenance complexity (image size, dependencies)
  - Flexibility (adding new tools, versioning)
  - Security implications (tool access control)
  - Scalability (tool isolation, resource limits)
  - Developer experience (debugging, testing)
  - Cost analysis (infrastructure, development time)

#### Envisioned Features

**3. Multi-Playbook Orchestration**
- Workflow dependencies
- Sequential/parallel execution
- Conditional workflows
- Rollback strategies

**4. AI-Driven Workflow Selection** ⚠️ **NEEDS REVIEW**
- Historical success rate analysis (circumstantial - relevant to operators only)
- Context-aware recommendations (unclear requirements)
- A/B testing of remediation strategies (unclear requirements)
- **Note**: Low confidence in success - requires further analysis and user validation

**5. LLM-Assisted Workflow Recommendations**
- LLM recommends alternative remediation strategies with confidence scores
- Recommendations included in remediation response structure
- Operators manually review and create playbooks based on recommendations
- **Critical**: LLMs do NOT create playbooks automatically - production requires deterministic management
- Operators maintain full control over workflow creation and approval

---

## v1.1 Scope - FOCUSED

### What We're Building (v1.1)

**Core**: CRD-based workflow lifecycle management with parameter schema validation

**Components**:
1. **Data Storage Service - Workflow CRUD REST API** (Go service)
   - POST /api/v1/playbooks (create/update)
   - PATCH /api/v1/playbooks/{id} (disable/enable)
   - GET /api/v1/playbooks/versions (version history)
   - PostgreSQL storage with pgvector
   - Semantic version validation

2. **RemediationWorkflow CRD** (Kubernetes Custom Resource)
   - CRD definition: `kind: RemediationWorkflow`
   - Operator-friendly YAML format
   - RBAC controls for registration
   - Kubernetes-native audit trail

3. **Playbook Registry Controller** (Go service)
   - Watches RemediationWorkflow CRDs
   - Pulls container images
   - Extracts `/playbook-schema.json`
   - Validates schema format
   - Calls Data Storage REST API
   - Reconciles CRD state with database

4. **Parameter Schema Validation** (JSON Schema)
   - Parameter definitions
   - Type validation
   - Dependency tracking
   - Label metadata
   - Registration-time validation

5. **LLM Integration** (HolmesGPT API - existing)
   - Reads schema from catalog via Data Storage API
   - Populates parameters from RCA
   - Passes to Tekton as environment variables

---

## v1.1 Implementation Priority

### Phase 1: Data Storage REST API (Week 1-2)
**Goal**: Implement workflow CRUD REST API in Data Storage Service

- [ ] Implement POST /api/v1/playbooks (create/update)
- [ ] Implement PATCH /api/v1/playbooks/{id} (disable/enable)
- [ ] Implement GET /api/v1/playbooks/versions (version history)
- [ ] Add semantic version validation (golang.org/x/mod/semver)
- [ ] Add unit and integration tests

### Phase 2: RemediationWorkflow CRD (Week 3-4)
**Goal**: Define and deploy CRD

- [ ] Create CRD definition YAML
- [ ] Define CRD spec schema (workflow metadata, container image, parameters)
- [ ] Add OpenAPI validation schema
- [ ] Deploy CRD to cluster
- [ ] Document CRD usage for operators

### Phase 3: Workflow Registry Controller (Week 5-7)
**Goal**: Build controller to watch CRDs and register playbooks

- [ ] Create Go controller scaffold (kubebuilder/controller-runtime)
- [ ] Implement CRD watch and reconciliation loop
- [ ] Implement image pull logic (using crane or similar)
- [ ] Implement schema extraction from `/playbook-schema.json`
- [ ] Implement schema validation (JSON Schema)
- [ ] Integrate with Data Storage REST API
- [ ] Add error handling and status updates to CRD

### Phase 4: Parameter Schema Validation (Week 8-9)
**Goal**: Define and validate parameter schemas

- [ ] Update DD-WORKFLOW-001 with parameter schema spec
- [ ] Create example workflow schemas
- [ ] Implement JSON Schema validation logic
- [ ] Document schema format for operators
- [ ] Create workflow container examples

### Phase 5: End-to-End Testing (Week 10)
**Goal**: Validate complete workflow

- [ ] Test CRD creation → controller registration → database storage
- [ ] Test LLM parameter population from catalog
- [ ] Test Tekton PipelineRun generation with parameters
- [ ] Test schema validation error handling
- [ ] Document operator workflows

---

## What's NOT in v1.1

### Deferred to v1.2 or Later

**Image Signing (Sigstore/Cosign)**
- **Reason**: Security enhancement, not blocking for CRD-based registration
- **Target**: v1.2
- **Dependencies**: Workflow Registry Controller must exist first

- **Target**: v1.2
- **Dependencies**: Operator feedback on CRD workflow

**Playbook Versioning/Rollback**
- **Reason**: Complexity, need experience with v1.1 first
- **Target**: v1.2
- **Dependencies**: Workflow Registry Controller, execution history

**Multi-Playbook Orchestration**
- **Reason**: Advanced feature, significant complexity
- **Target**: v2.0
- **Dependencies**: v1.1 + v1.2 features stable

---

## Decision: Focus on v1.1 CRD-Based Lifecycle Management

### What We're Implementing NOW

**Scope**: CRD-based workflow lifecycle management with parameter schema validation

**Key Documents**:
1. ✅ `BR-WORKFLOW-001` - Workflow Registry Management (Target Version: V1)
2. ✅ `ADR-033` - Remediation Workflow Catalog
3. ✅ `DD-STORAGE-011` - Data Storage Service V1.1 Implementation Plan
4. ✅ `DD-WORKFLOW-003-parameterized-actions-REVISED.md` - Parameter passing
5. ⏳ `DD-WORKFLOW-001` - Update with parameter schema spec (TODO)

**Implementation**:
- Data Storage Service - Workflow CRUD REST API
- RemediationWorkflow CRD (Kubernetes Custom Resource)
- Workflow Registry Controller (watches CRDs, registers playbooks)
- Parameter schema extraction and validation (JSON Schema)
- LLM integration (parameter population via Data Storage API)

**NOT Implementing in v1.1**:
- ❌ Image signing (Sigstore) → v1.2
- ❌ Versioning/rollback → v1.2

---

## Summary

**v1.0**: Workflow catalog with semantic search + manual SQL registration + execution history (audit trail)
**v1.1**: CRD-based lifecycle management + REST API + parameter schema validation ← **CURRENT FOCUS**
**v2.0**: Multi-playbook orchestration + LLM-assisted recommendations (⚠️ needs review)

**v1.1 Core**: RemediationWorkflow CRD + Workflow Registry Controller + Data Storage REST API

**Key Architecture**:
```
Operator creates CRD
    ↓
Workflow Registry Controller watches CRD
    ↓
Controller pulls image + extracts schema
    ↓
Controller validates schema
    ↓
Controller calls Data Storage REST API
    ↓
PostgreSQL stores workflow + parameters
    ↓
LLM queries Data Storage API for playbooks
```

**Status**: Ready to implement v1.1 features
**Next Step**: Implement Data Storage REST API (Phase 1)
