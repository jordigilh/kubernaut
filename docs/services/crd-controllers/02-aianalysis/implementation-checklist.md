## Implementation Checklist

**Note**: Follow APDC-TDD phases for each implementation step (see Development Methodology section)

### Phase 0: Project Setup (30 min) [BEFORE ANALYSIS]

- [ ] **Verify cmd/ structure**: Check [cmd/README.md](../../../../cmd/README.md)
- [ ] **Create service directory**: `mkdir -p cmd/aianalysis` (no hyphens - Go convention)
- [ ] **Copy main.go template**: From `cmd/remediationorchestrator/main.go`
- [ ] **Update package imports**: Change to service-specific controller (AIAnalysisReconciler)
- [ ] **Verify build**: `go build -o bin/ai-analysis ./cmd/aianalysis` (binary can have hyphens)
- [ ] **Reference documentation**: [cmd/ directory guide](../../../../cmd/README.md)

**Note**: Directory names use Go convention (no hyphens), binaries can use hyphens for readability.

---

### Phase 1: ANALYSIS & CRD Setup (1-2 days) [RED Phase Preparation]

- [ ] **ANALYSIS**: Search existing AI implementations (`codebase_search "AI analysis implementations"`)
- [ ] **ANALYSIS**: Map business requirements across all V1 BRs:
  - **V1 Scope**: BR-AI-001 to BR-AI-050 (40+ BRs)
    - BR-AI-001 to 025: AI investigation & analysis (~25 BRs)
    - BR-AI-026 to 040: Remediation recommendations (~15 BRs)
    - BR-AI-041 to 050: Approval & workflow creation (~5 BRs)
  - **Reserved for V2**: BR-AI-051 to BR-AI-180 (multi-provider AI, ensemble decision-making)

### Logging Library

- **Library**: `sigs.k8s.io/controller-runtime/pkg/log/zap`
- **Rationale**: Official controller-runtime integration with opinionated defaults for Kubernetes controllers
- **Setup**: Initialize in `main.go` with `ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))`
- **Usage**: `log := ctrl.Log.WithName("aianalysis")`

---

- [ ] **ANALYSIS**: Identify integration points in cmd/aianalysis/
- [ ] **CRD RED**: Write AIAnalysisReconciler tests (should fail - no controller yet)
- [ ] **CRD GREEN**: Generate CRD + controller skeleton (tests pass)
  - [ ] Create AIAnalysis CRD schema (reference: `docs/design/CRD/03_AI_ANALYSIS_CRD.md`)
  - [ ] Generate Kubebuilder controller scaffold
  - [ ] Implement AIAnalysisReconciler with finalizers
  - [ ] Configure owner references to RemediationRequest CRD
- [ ] **CRD REFACTOR**: Enhance controller with error handling
  - [ ] Add controller-specific Prometheus metrics
  - [ ] Implement cross-CRD reference validation
  - [ ] Add phase timeout detection (default: 15 min)

### Phase 2: Package Structure & Business Logic (2-3 days) [RED-GREEN-REFACTOR]

- [ ] **Package RED**: Write tests for Analyzer interface (fail - no interface yet)
- [ ] **Package GREEN**: Create `pkg/ai/analysis/` package structure
  - [ ] Define `Analyzer` interface (no "Service" suffix)
  - [ ] Create phase handlers: investigating, analyzing, recommending, completed
- [ ] **Package REFACTOR**: Enhance with sophisticated logic
  - [ ] Implement response validation (BR-AI-021)
  - [ ] Implement hallucination detection (BR-AI-023)
  - [ ] Implement confidence scoring algorithms

### Phase 3: Reconciliation Phases (2-3 days) [RED-GREEN-REFACTOR]

- [ ] **Reconciliation RED**: Write tests for each phase (fail - no phase logic yet)
- [ ] **Reconciliation GREEN**: Implement minimal phase logic (tests pass)
  - [ ] **Investigating**: Read enrichment data from spec (Alternative 2), HolmesGPT investigation, root cause identification (BR-AI-011, BR-AI-012)
    - [ ] **Read EnrichmentData from spec** (NO API calls - Alternative 2)
    - [ ] **Extract monitoring context** (FRESH for recovery!)
    - [ ] **Extract business context** (FRESH for recovery!)
    - [ ] **Extract recovery context** (if isRecoveryAttempt = true)
  - [ ] **Analyzing**: Contextual analysis, confidence scoring, validation (BR-AI-001, BR-AI-002, BR-AI-003)
  - [ ] **Recommending**: Recommendation generation, ranking, historical success rate (BR-AI-006, BR-AI-007, BR-AI-008)
  - [ ] **Completed**: WorkflowExecution creation, investigation report (BR-AI-014)
- [ ] **Reconciliation REFACTOR**: Enhance with sophisticated algorithms
  - [ ] **Enhanced prompt engineering** (include all contexts from EnrichmentData)
  - [ ] **Recovery-specific prompts** (leverage historical failure context - BR-WF-RECOVERY-011)
  - [ ] Add Rego-based policy evaluation for auto-approval (BR-AI-030)
  - [ ] Implement optimized requeue strategy (phase-specific intervals)
  - [ ] Add selective embedding for large payloads (BR-AI-027)

### Phase 4: Integration & Testing (2-3 days) [RED-GREEN-REFACTOR]

**Integration Points**:
- [ ] **Integration RED**: Write tests for HolmesGPT-API integration (fail - no client yet)
- [ ] **Integration GREEN**: Implement HolmesGPT-API client (tests pass)
  - [ ] Investigation endpoint integration (port 8080)
  - [ ] Recovery analysis endpoint
  - [ ] Safety analysis endpoint
- [ ] **Integration REFACTOR**: Enhance with error handling and retries
- [ ] **Storage RED**: Write tests for Data Storage Service integration (fail)
- [ ] **Storage GREEN**: Implement storage client (tests pass)
  - [ ] Historical pattern lookup (BR-AI-011)
  - [ ] Success rate retrieval (BR-AI-008)
- [ ] **CRD Integration**: WorkflowExecution creation with owner references
- [ ] **Main App Integration**: Verify AIAnalysisReconciler instantiated in cmd/ai/analysis/ (MANDATORY)

### Phase 5: Testing & Validation (1-2 days) [CHECK Phase]

- [ ] **CHECK**: Verify 70%+ unit test coverage (test/unit/ai/analysis/)
  - [ ] Reconciler tests with fake K8s client, mocked HolmesGPT-API
  - [ ] Investigation phase tests (BR-AI-011, BR-AI-012)
  - [ ] Analysis phase tests (BR-AI-001, BR-AI-002, BR-AI-003)
  - [ ] Recommendation phase tests (BR-AI-006, BR-AI-007, BR-AI-008)
  - [ ] Response validation tests (BR-AI-021, BR-AI-023)
  - [ ] Rego policy evaluation tests (BR-AI-030)
- [ ] **CHECK**: Run integration tests - 20% coverage target (test/integration/ai/analysis/)
  - [ ] Real HolmesGPT-API integration
  - [ ] Real K8s API (KIND) CRD lifecycle tests
  - [ ] Cross-CRD coordination tests
- [ ] **CHECK**: Execute E2E tests - 10% coverage target (test/e2e/ai/analysis/)
  - [ ] Complete investigation-to-workflow workflow
  - [ ] Multi-alert correlation scenarios
- [ ] **CHECK**: Validate business requirement coverage (BR-AI-001 to BR-AI-185)
- [ ] **CHECK**: Performance validation (per-phase <15min, total <45min)
- [ ] **CHECK**: Provide confidence assessment (92% with Rego policy approach)

### Phase 6: Metrics, Audit & Deployment (1 day)

- [ ] **Metrics**: Define and implement Prometheus metrics
  - [ ] Investigation, analysis, recommendation phase metrics
  - [ ] Implement metrics recording in reconciler
  - [ ] Setup metrics server on port 9090 (with auth)
  - [ ] Create Grafana dashboard queries
  - [ ] Set performance targets (p95 < 120s, confidence > 0.8)
- [ ] **Audit**: Database integration for compliance
  - [ ] Implement audit client (`integration/audit.go`)
  - [ ] Record investigation results to PostgreSQL
  - [ ] Record recommendations to PostgreSQL
  - [ ] Store investigation embeddings in vector DB
  - [ ] Implement historical success rate queries
- [ ] **Deployment**: Binary and infrastructure
  - [ ] Create `cmd/aianalysis/main.go` entry point
  - [ ] Configure Kubebuilder manager with leader election
  - [ ] Add RBAC permissions for CRD operations
  - [ ] Create Kubernetes deployment manifests
  - [ ] Configure HolmesGPT-API service discovery (port 8080)

### Phase 7: Documentation

- [ ] Update API documentation with AIAnalysis CRD
- [ ] Document HolmesGPT integration patterns
- [ ] Add troubleshooting guide for AI analysis
- [ ] Create runbook for hallucination detection
- [ ] Document Rego policy authoring guide (BR-AI-030)

---

