# Architecture Decision Records (ADRs)

**Purpose**: This directory contains all significant architectural decisions made for the Kubernaut project.

**Format**: Each ADR follows the naming convention `NNN-short-title.md` where NNN is a zero-padded sequential number.

---

## 📋 ADR Index

### **Core Architecture Decisions**

|| # | Title | Status | Date | Impact |
||---|-------|--------|------|--------|
|| 001 | [CRD API Group Rationale](./001-crd-api-group-rationale.md) | ✅ Accepted | 2025 | Why `remediation.kubernaut.io` API group |
|| 002 | [E2E GitOps Strategy](./002-e2e-gitops-strategy.md) | ✅ Accepted | 2025 | E2E testing approach for GitOps |
|| 003 | [GitOps Priority Order](./003-gitops-priority-order.md) | ✅ Accepted | 2025 | Implementation priority for GitOps features |
|| 004 | [Metrics Authentication](./004-metrics-authentication.md) | ✅ Accepted | 2025 | Authentication strategy for metrics endpoints |
|| 005 | [Owner Reference Architecture](./005-owner-reference-architecture.md) | ✅ Accepted | 2025 | CRD lifecycle and ownership patterns |
|| 006 | [Effectiveness Monitor V1 Inclusion](./006-effectiveness-monitor-v1-inclusion.md) | ✅ Accepted | 2025-10 | Moving Effectiveness Monitor from V2 to V1 |
|| 027 | [Multi-Architecture Build Strategy](./ADR-027-multi-architecture-build-strategy.md) | ✅ Accepted | 2025-10-20 | All services built for amd64 + arm64 by default |

### **Business Requirement (BR) Migration Decisions**

|| # | Title | Service | Status | Impact |
||---|-------|---------|--------|--------|
|| 007 | [Gateway BR Legacy Mapping](./007-gateway-br-legacy-mapping.md) | Gateway Service | ✅ Accepted | BR standardization for gateway |
|| 008 | [Gateway BR Standardization](./008-gateway-br-standardization.md) | Gateway Service | ✅ Accepted | BR format migration strategy |
|| 009 | [HolmesGPT BR Legacy Mapping](./009-holmesgpt-br-legacy-mapping.md) | HolmesGPT API | ✅ Accepted | BR mapping for AI service |
|| 010 | [HolmesGPT BR Migration Plan](./010-holmesgpt-br-migration-plan.md) | HolmesGPT API | ✅ Accepted | BR migration execution plan |
|| 011 | [Remediation Processor BR Migration](./011-remediationprocessor-br-migration.md) | Remediation Processor | ✅ Accepted | BR standardization for processor |
|| 012 | [Kubernetes Executor BR Migration](./012-kubernetesexecutor-br-migration.md) | Kubernetes Executor | ✅ Accepted | BR standardization for executor |
|| 013 | [Remediation Orchestrator BR Migration](./013-remediationorchestrator-br-migration.md) | Remediation Orchestrator | ✅ Accepted | BR standardization for orchestrator |

### **Design Decisions (DD-PREFIX)**

#### **Project-Wide Standards**

|| ID | Title | Scope | Status | Date | Impact |
||---|-------|-------|--------|------|--------|
|| DD-001 | [Recovery Context Enrichment](./DD-001-recovery-context-enrichment.md) | RemediationProcessing / AIAnalysis | ✅ Approved | 2024-10-08 | Temporal consistency, fresh context for AI recovery |
|| DD-002 | [Per-Step Validation Framework](./DD-002-per-step-validation-framework.md) | WorkflowExecution / KubernetesExecutor | ✅ Approved | 2025-10-14 | 15-20% effectiveness improvement, cascade failure prevention |
|| DD-003 | [Forced Recommendation Manual Override](./DD-003-forced-recommendation-manual-override.md) | RemediationOrchestrator | ✅ Approved for V2 | 2025-10-20 | Operator autonomy, complete audit trail (V2 feature) |
|| DD-004 | [RFC 7807 Error Response Standard](./DD-004-RFC7807-ERROR-RESPONSES.md) | All HTTP Services | ✅ Approved | 2025-10-30 | Consistent error handling across all services |
|| DD-005 | [Observability Standards](./DD-005-OBSERVABILITY-STANDARDS.md) | All Services | ✅ Approved | 2025-10-31 | Metrics, logging, tracing standards |

#### **Service-Specific Decisions**

|| ID | Title | Service/Component | Status | Date | Impact |
||---|-------|-------------------|--------|------|--------|
|| DD-CONTEXT-001 | [Cache Stampede Prevention](./DD-CONTEXT-001-cache-stampede-prevention.md) | Context API | ✅ Approved | 2025-10-20 | 90% DB query reduction, single-flight deduplication |
|| DD-CONTEXT-002 | [Cache Size Limit Configuration](./DD-CONTEXT-002-cache-size-limit-configuration.md) | Context API | ✅ Approved | 2025-10-20 | OOM prevention, configurable limits |
|| DD-CONTEXT-003 | [Context Enrichment Placement](./DD-CONTEXT-003-Context-Enrichment-Placement.md) | Context API / HolmesGPT API | ✅ Approved | 2025-10-22 | LLM-driven tool call pattern, 36% token cost reduction |
|| DD-CONTEXT-004 | [BR-AI-002 Ownership](./DD-CONTEXT-004-BR-AI-002-Ownership.md) | AIAnalysis / Context API | ✅ Approved | 2025-10-22 | Keep BR-AI-002 in AIAnalysis (revised scope) |
|| DD-EFFECTIVENESS-001 | [Hybrid Automated + AI Analysis](./DD-EFFECTIVENESS-001-Hybrid-Automated-AI-Analysis.md) | Effectiveness Monitor | ✅ Approved | 2025-10-16 | 85-90% effectiveness, 11x ROI |
|| DD-EFFECTIVENESS-002 | [Restart Recovery Idempotency](./DD-EFFECTIVENESS-002-Restart-Recovery-Idempotency.md) | Effectiveness Monitor | ✅ Approved | 2025-10-16 | Idempotent restart recovery |
|| DD-EFFECTIVENESS-003 | [RemediationRequest Watch Strategy](./DD-EFFECTIVENESS-003-RemediationRequest-Watch-Strategy.md) | Effectiveness Monitor | ✅ Approved | 2025-10-16 | 92% confidence, future-proof design |
|| DD-GATEWAY-004 | [Redis Memory Optimization](./DD-GATEWAY-004-redis-memory-optimization.md) | Gateway Service | ✅ Approved | 2025-10-24 | 93% memory reduction, lightweight metadata |
|| DD-GATEWAY-005 | [Redis Cleanup on CRD Deletion](./DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md) | Gateway Service | ✅ Approved | 2025-10-27 | No cleanup needed (TTL-based expiration) |
|| DD-GATEWAY-006 | [Authentication Strategy](./DD-GATEWAY-006-authentication-strategy.md) | Gateway Service | ✅ Approved | 2025-10-27 | Network-level security, no OAuth2 |
|| DD-GATEWAY-007 | [Fallback Namespace Strategy](./DD-GATEWAY-007-fallback-namespace-strategy.md) | Gateway Service | ✅ Approved | 2025-10-31 | kubernaut-system fallback for cluster-scoped signals |
|| DD-HOLMESGPT-005 | [Test Strategy Validation](./DD-HOLMESGPT-005-Test-Strategy-Validation.md) | HolmesGPT API | ✅ Validated | 2025-10-20 | Zero SDK overlap, 211 tests validated |
|| DD-HOLMESGPT-006 | [Implementation Plan Quality Gate](./DD-HOLMESGPT-006-Implementation-Plan-Quality-Gate.md) | HolmesGPT API | ✅ Pending | [Pending] | Plan quality validation |
|| DD-HOLMESGPT-007 | [Service Boundaries Clarification](./DD-HOLMESGPT-007-Service-Boundaries-Clarification.md) | HolmesGPT API | ✅ Approved | 2025-10-20 | Clear service boundaries |
|| DD-HOLMESGPT-008 | [Safety-Aware Investigation](./DD-HOLMESGPT-008-Safety-Aware-Investigation.md) | HolmesGPT API | ✅ Approved | 2025-10-16 | Safety-aware AI investigations |
|| DD-HOLMESGPT-009 | [Self-Documenting JSON Format](./DD-HOLMESGPT-009-Ultra-Compact-JSON-Format.md) | HolmesGPT API / All AI Services | ✅ Approved | 2025-10-16 | 60% token reduction, $5,500/year savings |
|| DD-HOLMESGPT-009-ADD | [YAML Evaluation Addendum](./DD-HOLMESGPT-009-ADDENDUM-YAML-Evaluation.md) | HolmesGPT API / All AI Services | ✅ JSON Reaffirmed | 2024-10-16 | YAML evaluated: 17.5% token savings insufficient |
|| DD-HOLMESGPT-013 | [Vendor Local SDK Copy](./DD-HOLMESGPT-013-Vendor-Local-SDK-Copy.md) | HolmesGPT API | ✅ Approved | 2025-10-18 | Stability through vendored SDK |
|| DD-HOLMESGPT-014 | [MinimalDAL Stateless Architecture](./DD-HOLMESGPT-014-MinimalDAL-Stateless-Architecture.md) | HolmesGPT API | ✅ Approved | 2025-10-20 | Stateless architecture, no Robusta Platform |

**Note**: DD-* prefix is used for detailed design decisions with comprehensive alternatives analysis, implementation strategy, and validation plans. ADR-* prefix is used for architectural records.

---

## 📝 DD Numbering Principles

### Chronological Order Based on Decision Date

DD numbers are assigned based on **when the decision was made** (decision date), not when the file was created or migrated.

**Key Principles**:
- Older decisions keep lower numbers when conflicts arise
- Decision date is found in the file header (`**Date**: YYYY-MM-DD`)
- Sequential numbering within each service prefix (DD-CONTEXT-001, DD-CONTEXT-002, etc.)

**Example**: DD-CONTEXT-001 (Cache Stampede, 2025-10-20) comes before DD-CONTEXT-003 (Context Enrichment, 2025-10-22) because the decision was made 2 days earlier, even though both files were migrated on 2025-10-31.

### Renumbering History

During the 2025-10-31 migration, some DD files were renumbered to maintain chronological order:

| Original ID | Decision | Date | New ID | Reason |
|---|---|---|---|---|
| DD-CONTEXT-001 | Context Enrichment Placement | 2025-10-22 | DD-CONTEXT-003 | DESIGN_DECISIONS.md files (2025-10-20) were older |
| DD-CONTEXT-002 | BR-AI-002 Ownership | 2025-10-22 | DD-CONTEXT-004 | DESIGN_DECISIONS.md files (2025-10-20) were older |

**Result**: DD-CONTEXT-001 and DD-CONTEXT-002 now refer to Cache Stampede and Cache Size decisions (2025-10-20), maintaining chronological order.

---

## 📝 ADR Guidelines

### **When to Create an ADR**

Create an ADR for decisions that:
- ✅ Affect multiple services or the overall architecture
- ✅ Have long-term implications (>6 months)
- ✅ Involve trade-offs between alternatives
- ✅ Set precedents for future decisions
- ✅ Change existing architectural patterns

### **ADR Template**

```markdown
# ADR-NNN: [Short Title]

**Status**: [Proposed | Accepted | Deprecated | Superseded]
**Date**: YYYY-MM-DD
**Decision Makers**: [Names/Roles]
**Impact**: [High | Medium | Low]

## Context
What is the issue we're facing?

## Decision
What is our decision?

## Consequences
What are the trade-offs and implications?

## Alternatives Considered
What other options did we evaluate?

## Related Decisions
- ADR-XXX: [Related decision]
```

### **ADR Status Values**

- **Proposed**: Under discussion, not yet decided
- **Accepted**: Decision made and implemented
- **Deprecated**: No longer recommended, but not removed
- **Superseded**: Replaced by a newer ADR (link to replacement)

---

## 🔗 Related Documentation

- **Specifications**: Cross-service technical specifications → [../specifications/](../specifications/)
- **References**: Visual diagrams and reference materials → [../references/](../references/)
- **Service Docs**: Individual service specifications → [../../services/](../../services/)

---

**Maintained By**: Kubernaut Architecture Team
**Last Updated**: October 16, 2025
