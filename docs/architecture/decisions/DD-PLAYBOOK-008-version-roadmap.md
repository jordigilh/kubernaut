# DD-PLAYBOOK-008: Playbook Feature Roadmap

**Date**: 2025-11-15
**Status**: Planning

---

## Version Roadmap

### v1.0 (Current/Baseline)
**Status**: Foundation

**Features**:
- ✅ Tekton pipeline execution
- ✅ Playbook catalog table with semantic search (PostgreSQL + pgvector)
- ✅ Semantic search REST API (`GET /api/v1/playbooks/search`)
- ✅ Playbook execution history (audit trail for effectiveness monitoring)
- ✅ Operator-defined remediation scripts (container images)
- ✅ Manual playbook registration via SQL inserts

**Limitations**:
- ❌ No parameter schema extraction or validation
- ❌ No playbook write REST API (SQL-only management)
- ❌ No CRD-based playbook management
- ❌ No playbook catalog controller
- ❌ No automated registration workflow
- ❌ Operators must manually insert playbook metadata, parameters, and labels via SQL

---

### v1.1 (Planned) - CRD-Based Management & Schema Validation
**Status**: In Planning
**Focus**: Automated playbook lifecycle management with CRD + REST API

#### Core Features

**1. Playbook CRUD REST API** (NEW)
- POST /api/v1/playbooks (create/update playbook)
- PATCH /api/v1/playbooks/{id} (disable/enable)
- GET /api/v1/playbooks/versions (version history)
- Semantic version validation
- Lifecycle management (draft → active → deprecated)

**2. RemediationPlaybook CRD** (NEW)
- Kubernetes-native playbook registration
- CRD: `kind: RemediationPlaybook`
- RBAC controls for playbook registration
- Audit trail via Kubernetes events

**3. Playbook Registry Controller** (NEW)
- Watches RemediationPlaybook CRDs
- Validates playbook specs
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
- Business context in playbook labels
- LLM considers business constraints
- Documented in BR-AI-057

#### Out of Scope for v1.1
- ❌ Image signing (Sigstore/Cosign) → v1.2 or v2.0
- ❌ Playbook versioning/rollback → v1.2
- ❌ Multi-playbook orchestration → v2.0

---

### v1.2 (Future) - Security & Advanced Features
**Status**: Planned
**Focus**: Image signing, versioning, advanced remediation

#### Planned Features

**1. Image Signing (Sigstore/Cosign)**
- Playbook containers signed with Sigstore
- Signature verification during registration
- Policy enforcement (only signed images)
- Audit trail of signatures

**2. Playbook Versioning & Rollback**
- Multiple versions in catalog
- Version selection logic
- Rollback capabilities
- Deprecation workflow

**3. Additional Findings Integration**
- LLM discoveries beyond playbook scope
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
  - **Option A**: Embed tools directly in container (e.g., kubectl in kubernaut-agent image)
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
- Playbook dependencies
- Sequential/parallel execution
- Conditional workflows
- Rollback strategies

**4. AI-Driven Playbook Selection** ⚠️ **NEEDS REVIEW**
- Historical success rate analysis (circumstantial - relevant to operators only)
- Context-aware recommendations (unclear requirements)
- A/B testing of remediation strategies (unclear requirements)
- **Note**: Low confidence in success - requires further analysis and user validation

**5. LLM-Assisted Playbook Recommendations**
- LLM recommends alternative remediation strategies with confidence scores
- Recommendations included in remediation response structure
- Operators manually review and create playbooks based on recommendations
- **Critical**: LLMs do NOT create playbooks automatically - production requires deterministic management
- Operators maintain full control over playbook creation and approval

---

## v1.1 Scope - FOCUSED

### What We're Building (v1.1)

**Core**: CRD-based playbook lifecycle management with parameter schema validation

**Components**:
1. **Data Storage Service - Playbook CRUD REST API** (Go service)
   - POST /api/v1/playbooks (create/update)
   - PATCH /api/v1/playbooks/{id} (disable/enable)
   - GET /api/v1/playbooks/versions (version history)
   - PostgreSQL storage with pgvector
   - Semantic version validation

2. **RemediationPlaybook CRD** (Kubernetes Custom Resource)
   - CRD definition: `kind: RemediationPlaybook`
   - Operator-friendly YAML format
   - RBAC controls for registration
   - Kubernetes-native audit trail

3. **Playbook Registry Controller** (Go service)
   - Watches RemediationPlaybook CRDs
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
**Goal**: Implement playbook CRUD REST API in Data Storage Service

- [ ] Implement POST /api/v1/playbooks (create/update)
- [ ] Implement PATCH /api/v1/playbooks/{id} (disable/enable)
- [ ] Implement GET /api/v1/playbooks/versions (version history)
- [ ] Add semantic version validation (golang.org/x/mod/semver)
- [ ] Add unit and integration tests

### Phase 2: RemediationPlaybook CRD (Week 3-4)
**Goal**: Define and deploy CRD

- [ ] Create CRD definition YAML
- [ ] Define CRD spec schema (playbook metadata, container image, parameters)
- [ ] Add OpenAPI validation schema
- [ ] Deploy CRD to cluster
- [ ] Document CRD usage for operators

### Phase 3: Playbook Registry Controller (Week 5-7)
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

- [ ] Update DD-PLAYBOOK-001 with parameter schema spec
- [ ] Create example playbook schemas
- [ ] Implement JSON Schema validation logic
- [ ] Document schema format for operators
- [ ] Create playbook container examples

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
- **Dependencies**: Playbook Registry Controller must exist first

- **Target**: v1.2
- **Dependencies**: Operator feedback on CRD workflow

**Playbook Versioning/Rollback**
- **Reason**: Complexity, need experience with v1.1 first
- **Target**: v1.2
- **Dependencies**: Playbook Registry Controller, execution history

**Multi-Playbook Orchestration**
- **Reason**: Advanced feature, significant complexity
- **Target**: v2.0
- **Dependencies**: v1.1 + v1.2 features stable

---

## Decision: Focus on v1.1 CRD-Based Lifecycle Management

### What We're Implementing NOW

**Scope**: CRD-based playbook lifecycle management with parameter schema validation

**Key Documents**:
1. ✅ `BR-PLAYBOOK-001` - Playbook Registry Management (Target Version: V1)
2. ✅ `ADR-033` - Remediation Playbook Catalog
3. ✅ `DD-STORAGE-011` - Data Storage Service V1.1 Implementation Plan
4. ✅ `DD-PLAYBOOK-003-parameterized-actions-REVISED.md` - Parameter passing
5. ⏳ `DD-PLAYBOOK-001` - Update with parameter schema spec (TODO)

**Implementation**:
- Data Storage Service - Playbook CRUD REST API
- RemediationPlaybook CRD (Kubernetes Custom Resource)
- Playbook Registry Controller (watches CRDs, registers playbooks)
- Parameter schema extraction and validation (JSON Schema)
- LLM integration (parameter population via Data Storage API)

**NOT Implementing in v1.1**:
- ❌ Image signing (Sigstore) → v1.2
- ❌ Versioning/rollback → v1.2

---

## Summary

**v1.0**: Playbook catalog with semantic search + manual SQL registration + execution history (audit trail)
**v1.1**: CRD-based lifecycle management + REST API + parameter schema validation ← **CURRENT FOCUS**
**v2.0**: Multi-playbook orchestration + LLM-assisted recommendations (⚠️ needs review)

**v1.1 Core**: RemediationPlaybook CRD + Playbook Registry Controller + Data Storage REST API

**Key Architecture**:
```
Operator creates CRD
    ↓
Playbook Registry Controller watches CRD
    ↓
Controller pulls image + extracts schema
    ↓
Controller validates schema
    ↓
Controller calls Data Storage REST API
    ↓
PostgreSQL stores playbook + parameters
    ↓
LLM queries Data Storage API for playbooks
```

**Status**: Ready to implement v1.1 features
**Next Step**: Implement Data Storage REST API (Phase 1)
