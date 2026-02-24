# DD-006: Controller Scaffolding Strategy

**Date**: 2025-10-31
**Status**: ✅ **APPROVED**
**Context**: Kubernaut V1 Architecture - Controller Development Tooling
**Deciders**: Development Team
**Technical Story**: [BR-PLATFORM-001, BR-REMEDIATION-001]

---

## Context and Problem Statement

Kubernaut requires building **10+ CRD-based controllers** following a microservices architecture (ADR-001). Each controller must:
- Implement consistent configuration patterns (YAML + environment overrides)
- Follow DD-005 Observability Standards (Prometheus metrics, structured logging)
- Include health checks, graceful shutdown, leader election
- Use Red Hat UBI9 base images for production deployment
- Maintain architectural consistency across all controllers

**Current State**:
- ✅ 3 controllers implemented: `remediationprocessor`, `datastorage`, `gateway`
- ⏳ 5+ controllers remaining: `aianalysis`, `workflowexecution`, ~~`kubernetesexecution`~~ (DEPRECATED - ADR-025), `notification`, and more
- ⏱️ Average controller implementation time: **10-15 hours** per controller (including configuration, metrics, deployment)

**Key Challenges**:
1. **Consistency**: Manual creation leads to divergent patterns across controllers
2. **DD-005 Compliance**: Metrics naming and logging standards require manual implementation
3. **Time Consumption**: Boilerplate code (config, metrics, main.go) takes 4-6 hours per controller
4. **Onboarding**: New developers must learn patterns from existing controllers
5. **Standards Enforcement**: No automated way to ensure new controllers follow approved patterns

**Decision Required**: How should we scaffold new CRD controllers?

---

## Decision Drivers

### **Business Requirements**:
- **BR-PLATFORM-001**: Kubernetes-native architecture with consistent controller patterns
- **BR-REMEDIATION-001**: Rapid development of 5+ remaining controllers

### **Technical Drivers**:
- **Consistency**: All controllers must follow same configuration, logging, metrics patterns
- **DD-005 Compliance**: Observability standards must be enforced automatically
- **Developer Productivity**: Reduce boilerplate implementation time
- **Maintainability**: Centralized templates easier to update than scattered implementations
- **Onboarding**: New developers need clear scaffolding approach

### **Non-Functional Requirements**:
- **Time Savings**: Target 40-60% reduction in controller creation time
- **Standards Enforcement**: Automated DD-005 compliance
- **Flexibility**: Templates must be customizable for controller-specific needs

---

## Considered Options

### **Option 1: Kubebuilder Native Scaffolding**
Standard Kubebuilder v3 code generation.

### **Option 2: Operator SDK Templates**
Red Hat Operator SDK with community templates.

### **Option 3: Manual Creation (Copy-Paste)**
Copy existing controller and modify for new use case.

### **Option 4: Custom Production Templates** ⭐ **CHOSEN**
Project-specific templates enforcing Kubernaut standards.

---

## Decision Outcome

**Chosen option**: **"Option 4: Custom Production Templates"**

**Rationale**: Custom templates provide the best balance of:
- ✅ **DD-005 Enforcement**: Metrics and logging patterns built-in
- ✅ **Time Savings**: 40-60% reduction in boilerplate implementation (4-6 hours saved)
- ✅ **Consistency**: All controllers start from same foundation
- ✅ **Customization**: Templates include placeholders for controller-specific logic
- ✅ **Documentation**: Templates include inline guidance and examples

**Template Library Location**: `docs/templates/crd-controller-gap-remediation/`

**Templates Provided**:
1. **`cmd-main-template.go.template`**: Main entry point with configuration, health checks, controller manager setup
2. **`config-template.go.template`**: Configuration package with YAML + environment variable overrides
3. **`config-test-template.go.template`**: Configuration unit tests
4. **`metrics-template.go.template`**: Prometheus metrics following DD-005 standards
5. **`dockerfile-template`**: Red Hat UBI9 multi-arch Dockerfile
6. **`makefile-targets-template`**: Standard Makefile targets for building and deployment
7. **`configmap-template.yaml`**: Kubernetes ConfigMap for controller configuration

**Scaffolding Tool**: Makefile target `make scaffold-controller` for interactive directory creation

**Usage Guide**: `docs/templates/crd-controller-gap-remediation/GAP_REMEDIATION_GUIDE.md`

---

## Pros and Cons of the Options

### Option 1: Kubebuilder Native Scaffolding

**Description**: Use `kubebuilder create api` and `kubebuilder create webhook` for code generation.

**Pros**:
- ✅ Industry-standard tooling
- ✅ Well-documented with large community
- ✅ Automatic CRD generation and controller scaffolding
- ✅ Built-in RBAC generation
- ✅ Regular updates and maintenance

**Cons**:
- ❌ Generic templates don't enforce Kubernaut-specific patterns
- ❌ No DD-005 metrics naming conventions
- ❌ Uses `logr` logging instead of Zap structured logging
- ❌ Generates generic Dockerfile (not Red Hat UBI9)
- ❌ Configuration patterns don't match Kubernaut standards (YAML + env overrides)
- ❌ Requires significant post-generation customization (~3-4 hours)
- ❌ No integration with existing Kubernaut patterns (Context API, Data Storage Service)

**Time Analysis**:
- Scaffolding: 10 minutes
- Customization to Kubernaut standards: 3-4 hours
- **Total**: ~4 hours (still requires manual DD-005 implementation)

**Confidence**: 70% (good for generic controllers, insufficient for Kubernaut-specific needs)

---

### Option 2: Operator SDK Templates

**Description**: Use Red Hat Operator SDK with Go-based operator templates.

**Pros**:
- ✅ Red Hat supported tooling
- ✅ OLM (Operator Lifecycle Manager) integration
- ✅ Built-in scorecard validation
- ✅ OpenShift-friendly defaults
- ✅ Strong RBAC generation

**Cons**:
- ❌ Operator-focused patterns (we need microservice controllers)
- ❌ OLM dependencies not needed for Kubernaut architecture
- ❌ No DD-005 metrics conventions
- ❌ Heavier scaffolding than needed (includes OLM manifests)
- ❌ Configuration patterns differ from Kubernaut standards
- ❌ Requires learning Operator SDK CLI
- ❌ Post-generation customization similar to Kubebuilder (~3-4 hours)

**Time Analysis**:
- Scaffolding: 15 minutes
- Removing unnecessary OLM scaffolding: 30 minutes
- Customization to Kubernaut standards: 3-4 hours
- **Total**: ~4.5 hours

**Confidence**: 60% (over-engineered for Kubernaut's microservices architecture)

---

### Option 3: Manual Creation (Copy-Paste)

**Description**: Copy existing controller (e.g., `remediationprocessor`) and modify for new use case.

**Pros**:
- ✅ Inherits all Kubernaut-specific patterns
- ✅ DD-005 compliant metrics included
- ✅ Configuration patterns match existing controllers
- ✅ No new tooling to learn
- ✅ Works with existing Makefile targets

**Cons**:
- ❌ Inconsistent starting points (which controller to copy?)
- ❌ Risk of copying controller-specific logic by mistake
- ❌ No centralized pattern updates (changes require updating 10+ controllers)
- ❌ Harder to onboard new developers (no clear "source of truth")
- ❌ Manual find-replace for controller names prone to errors
- ❌ No documentation of scaffolding process
- ❌ Old patterns persist (no way to enforce latest standards)

**Time Analysis**:
- Copying and renaming: 30 minutes
- Removing controller-specific logic: 1 hour
- Updating imports and package names: 30 minutes
- Testing build: 30 minutes
- **Total**: ~2.5 hours (faster, but error-prone)

**Confidence**: 50% (fast but unmaintainable long-term)

---

### Option 4: Custom Production Templates ⭐ **CHOSEN**

**Description**: Kubernaut-specific templates with placeholders for controller customization.

**Approach**:
- Templates in `docs/templates/crd-controller-gap-remediation/`
- `.template` suffix to prevent Go build errors
- Placeholders: `{{CONTROLLER_NAME}}`, `{{PACKAGE_PATH}}`, `{{CRD_GROUP}}/{{CRD_VERSION}}/{{CRD_KIND}}`
- Makefile target: `make scaffold-controller` for directory creation
- Usage guide: `GAP_REMEDIATION_GUIDE.md` with step-by-step instructions

**Pros**:
- ✅ **DD-005 Compliance**: Metrics and logging patterns built-in
- ✅ **Time Savings**: 40-60% reduction (4-6 hours saved per controller)
- ✅ **Consistency**: Single source of truth for all controllers
- ✅ **Centralized Updates**: Template improvements benefit all future controllers
- ✅ **Documentation**: Inline comments explain WHY patterns exist
- ✅ **Customization**: Placeholders make adaptation clear
- ✅ **Standards References**: Templates link to DD-005, LOGGING_STANDARD.md, etc.
- ✅ **Architecture Compliance**: Enforces Multi-CRD Reconciliation Architecture patterns
- ✅ **Onboarding**: New developers follow clear scaffolding guide

**Cons**:
- ⚠️ **Maintenance Burden**: Templates must be updated when standards change
  - **Mitigation**: Templates live in `docs/templates/` with versioning and changelog
- ⚠️ **Manual Placeholder Replacement**: No automated `sed` script (user does find-replace)
  - **Mitigation**: Clear instructions in `GAP_REMEDIATION_GUIDE.md`, limited placeholders (5 total)
- ⚠️ **Template Divergence**: Risk of templates diverging from production controllers
  - **Mitigation**: Templates derived from production controllers, linked to DD-005 standards

**Time Analysis**:
- Run `make scaffold-controller`: 1 minute
- Copy templates to new controller: 2 minutes
- Replace placeholders (5 locations): 5 minutes
- Implement controller-specific reconciliation logic: 3-4 hours
- Testing and integration: 1 hour
- **Total**: ~5 hours (vs. 10-15 hours manual, **50% time savings**)

**Confidence**: 85% (best balance of speed, consistency, and maintainability)

---

## Decision

**APPROVED: Option 4** - Custom Production Templates

**Rationale**:

1. **DD-005 Enforcement**: Templates are the PRIMARY mechanism for ensuring all controllers follow Observability Standards
   - Metrics naming: `{service}_{component}_{metric}_{unit}`
   - Standard controller metrics: `reconcile_total`, `reconcile_duration_seconds`, `errors_total`
   - Structured logging with controller-runtime/log/zap

2. **Proven Time Savings**: Analysis of existing controllers shows 4-6 hours spent on boilerplate
   - Configuration package: ~1.5 hours
   - Metrics setup: ~1 hour
   - Main.go entry point: ~1 hour
   - Dockerfile and deployment: ~1.5 hours
   - Templates reduce this to ~15 minutes

3. **5+ Controllers Remaining**: Time savings compound across remaining development
   - 5 controllers × 4-6 hours saved = **20-30 hours of development time saved**
   - Consistency across all controllers improves maintainability

4. **Centralized Standards**: Single source of truth easier to maintain than scattered implementations
   - Template updates propagate to all future controllers
   - Standards evolution (e.g., DD-005 updates) reflected in one place

**Key Insight**: Templates are not just boilerplate reduction—they're **standards enforcement mechanisms** that ensure architectural consistency across 10+ microservices.

---

## Implementation

**Primary Implementation Files**:
- **Template Library**: `docs/templates/crd-controller-gap-remediation/`
  - `cmd-main-template.go.template` - Main entry point
  - `config-template.go.template` - Configuration package
  - `config-test-template.go.template` - Config tests
  - `metrics-template.go.template` - DD-005 compliant metrics
  - `dockerfile-template` - Red Hat UBI9 Dockerfile
  - `makefile-targets-template` - Build targets
  - `configmap-template.yaml` - K8s ConfigMap

- **Usage Guide**: `docs/templates/crd-controller-gap-remediation/GAP_REMEDIATION_GUIDE.md`
- **Scaffolding Tool**: `Makefile` target `scaffold-controller` (lines 342-390)
- **Integration Documentation**: `docs/development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md` (lines 72-96)
- **Main README Reference**: `README.md` (lines 531-547)

**Scaffolding Workflow**:
1. Developer runs: `make scaffold-controller`
2. Enters controller name (e.g., `aianalysis`)
3. Tool creates directory structure:
   - `cmd/aianalysis/`
   - `pkg/aianalysis/config/`
   - `pkg/aianalysis/metrics/`
   - `api/aianalysis/v1alpha1/`
   - `internal/controller/aianalysis/`
4. Developer copies templates and replaces placeholders:
   - `{{CONTROLLER_NAME}}` → `aianalysis`
   - `{{PACKAGE_PATH}}` → `github.com/jordigilh/kubernaut`
   - `{{CRD_GROUP}}/{{CRD_VERSION}}/{{CRD_KIND}}` → `aianalysis.kubernaut.io/v1alpha1/AIAnalysis`
5. Implements controller-specific reconciliation logic
6. Runs `make manifests generate fmt vet` to generate CRD and DeepCopy methods

**Standards Compliance**:
- **DD-005**: Metrics naming, logging patterns, health checks
- **LOGGING_STANDARD.md**: Zap structured logging, log levels
- **MULTI_CRD_RECONCILIATION_ARCHITECTURE.md**: Controller patterns, owner references
- **12-Factor App**: Configuration via YAML + environment variables

---

## Consequences

**Positive**:
- ✅ **40-60% Time Savings**: 4-6 hours saved per controller × 5+ controllers = 20-30 hours total
- ✅ **Automatic DD-005 Compliance**: New controllers inherit observability standards
- ✅ **Consistency**: All controllers follow same patterns (configuration, metrics, logging)
- ✅ **Onboarding**: New developers follow clear scaffolding guide
- ✅ **Maintainability**: Centralized templates easier to update than scattered code
- ✅ **Documentation**: Templates include inline guidance and architectural references

**Negative**:
- ⚠️ **Template Maintenance**: Templates must be updated when standards evolve
  - **Mitigation**: Templates versioned with changelog, linked to DD-005 and other standards
- ⚠️ **Manual Placeholder Replacement**: No automated `sed` script for placeholder substitution
  - **Mitigation**: Only 5 placeholders, clear instructions in GAP_REMEDIATION_GUIDE.md
- ⚠️ **Template Divergence Risk**: Templates might drift from production controllers
  - **Mitigation**: Periodic review of templates against production controllers, templates derived from real implementations

**Neutral**:
- 🔄 **Learning Curve**: Developers must learn template placeholders
- 🔄 **Template Discovery**: Developers must know templates exist (addressed by README.md integration)

---

## Validation Results

**Confidence Assessment Progression**:
- Initial assessment (based on existing controllers): 75% confidence
- After template creation and triage: 80% confidence
- After Makefile integration and README updates: 85% confidence

**Key Validation Points**:
- ✅ Templates derived from production controllers (`remediationprocessor`, `datastorage`)
- ✅ DD-005 compliance verified in metrics template
- ✅ Logging standard compliance verified in cmd-main template
- ✅ Makefile scaffolding target tested and functional
- ✅ Documentation integration complete (README, CRD_SERVICE_SPECIFICATION_TEMPLATE)
- ✅ Templates renamed to `.template` suffix to prevent build errors

**Time Savings Validation**:
- Manual controller creation: 10-15 hours (based on `remediationprocessor` implementation)
- Template-based creation: 5-6 hours (estimated)
- **Savings**: 4-6 hours per controller (40-50%)
- **Cumulative Savings**: 20-30 hours across 5+ remaining controllers

---

## Related Decisions

- **Builds On**: [DD-005: Observability Standards](DD-005-OBSERVABILITY-STANDARDS.md) - Templates enforce DD-005 metrics and logging
- **Builds On**: [ADR-001: CRD-Based Microservices Architecture](ADR-001-crd-microservices-architecture.md) - Templates support microservices pattern
- **Supports**: [BR-PLATFORM-001](../../requirements/01_OVERALL_SYSTEM_ARCHITECTURE.md) - Kubernetes-native controller patterns
- **Supports**: [BR-REMEDIATION-001](../../requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md) - Rapid controller development

**References**:
- **Architecture**: `docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md`
- **Logging**: `docs/architecture/LOGGING_STANDARD.md`
- **Observability**: `docs/architecture/PRODUCTION_MONITORING.md`

---

## Review & Evolution

**When to Revisit**:
- If DD-005 Observability Standards change significantly
- If controller architecture patterns evolve (e.g., new reconciliation patterns)
- If template maintenance becomes unsustainable (>2 hours per update)
- If automated placeholder replacement becomes necessary (>10 controllers created)
- If template divergence causes issues (templates don't match production patterns)

**Success Metrics**:
- **Time Savings**: Average controller creation time ≤6 hours (target: 50% reduction from 10-15 hours)
- **DD-005 Compliance**: 100% of new controllers pass observability standards validation
- **Developer Satisfaction**: Positive feedback from developers using templates
- **Template Usage**: 100% of new controllers use template scaffolding
- **Maintenance Burden**: Template updates require <2 hours per standards change

**Review Schedule**:
- **Quarterly Review**: Validate templates match production controller patterns
- **Post-Controller Review**: After each new controller creation, gather feedback on templates
- **Standards Update Review**: Update templates within 1 week of DD-005 or other standards changes

---

## Appendix: Template Customization Guide

### Common Placeholder Replacements

| Placeholder | Description | Example |
|-------------|-------------|---------|
| `{{CONTROLLER_NAME}}` | Controller name (lowercase, no hyphens) | `aianalysis` |
| `{{PACKAGE_PATH}}` | Full Go module path | `github.com/jordigilh/kubernaut` |
| `{{CRD_GROUP}}` | CRD API group | `aianalysis.kubernaut.io` |
| `{{CRD_VERSION}}` | CRD API version | `v1alpha1` |
| `{{CRD_KIND}}` | CRD Kind name | `AIAnalysis` |

### Controller-Specific Customization Areas

After copying templates, developers must customize:

1. **Reconciliation Logic** (`internal/controller/{{CONTROLLER_NAME}}/{{CONTROLLER_NAME}}_controller.go`)
   - Implement `Reconcile()` function with business logic
   - Define watched resources and event filters
   - Implement status updates

2. **Configuration** (`pkg/{{CONTROLLER_NAME}}/config/config.go`)
   - Add controller-specific config fields
   - Implement validation rules for custom fields
   - Add environment variable mappings

3. **Metrics** (`pkg/{{CONTROLLER_NAME}}/metrics/metrics.go`)
   - Add controller-specific metrics beyond standard reconciliation metrics
   - Define custom metric labels (keep cardinality low)
   - Implement helper functions for recording custom metrics

4. **CRD Schema** (`api/{{CONTROLLER_NAME}}/v1alpha1/{{CONTROLLER_NAME}}_types.go`)
   - Define Spec and Status structs
   - Add validation markers (e.g., `+kubebuilder:validation:Required`)
   - Document fields with comments

---

## Appendix: Service Documentation Structure

### Standard Directory Layout

Each CRD controller service uses a directory-per-service structure under `docs/services/crd-controllers/`:

```
XX-servicename/
├── 📄 README.md                    - Service index & navigation (COMMON)
├── 📘 overview.md                  - High-level architecture (COMMON)
├── 🔧 crd-schema.md               - CRD type definitions (COMMON)
├── ⚙️  controller-implementation.md - Reconciler logic (COMMON)
├── 🔄 reconciliation-phases.md     - Phase details & coordination (COMMON)
├── 🧹 finalizers-lifecycle.md      - Cleanup & lifecycle management (COMMON)
├── 🧪 testing-strategy.md          - Test patterns (COMMON)
├── 🔒 security-configuration.md    - Security patterns (COMMON)
├── 📊 observability-logging.md     - Logging & tracing (COMMON)
├── 📈 metrics-slos.md              - Prometheus & Grafana (COMMON)
├── 💾 database-integration.md      - Audit storage & schema (COMMON)
├── 🔗 integration-points.md        - Service coordination (COMMON)
├── 🔀 migration-current-state.md   - Existing code & migration (COMMON)
├── ✅ implementation-checklist.md  - APDC-TDD phases & tasks (COMMON)
├── 📋 BR_MAPPING.md                - Business requirements (COMMON)
└── 🔧 [SERVICE-SPECIFIC FILES]     - Domain-specific documents (SERVICE-SPECIFIC)
```

### Document Classification

| Classification | Description | Examples |
|----------------|-------------|----------|
| **COMMON PATTERN** | Standard files present in ALL CRD services. Structure and sections are templated, but content is service-specific. | `testing-strategy.md`, `metrics-slos.md`, `security-configuration.md` |
| **SERVICE-SPECIFIC** | Files unique to a service's domain. Not all services will have these. | `REGO_POLICY_EXAMPLES.md` (AIAnalysis), Tekton pipeline specs (WorkflowExecution), Notification channel configs (Notification) |

### Service-Specific Document Guidelines

When a service requires domain-specific documentation not covered by common patterns:

1. **Create appropriately named files** that clearly indicate the domain:
   - AIAnalysis: `REGO_POLICY_EXAMPLES.md`, `ai-holmesgpt-approval.md`
   - WorkflowExecution: `tekton-pipeline-spec.md`, `workflow-parameters.md`
   - Notification: `notification-channels.md`, `template-engine.md`

2. **Reference in README.md** with `(SERVICE-SPECIFIC)` annotation

3. **Cross-reference from CRD_SERVICE_SPECIFICATION_TEMPLATE.md** if the pattern may apply to future services

### Template Reference

- **Documentation Template**: `docs/development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md`
- **Code Templates**: `docs/templates/crd-controller-gap-remediation/`

---

**Last Updated**: November 30, 2025
**Approved By**: Development Team
**Next Review**: January 31, 2026

