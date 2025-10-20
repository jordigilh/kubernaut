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

|| ID | Title | Service/Component | Status | Date | Impact |
||---|-------|-------------------|--------|------|--------|
|| DD-EFFECTIVENESS-001 | [Hybrid Automated + AI Analysis](./DD-EFFECTIVENESS-001-Hybrid-Automated-AI-Analysis.md) | Effectiveness Monitor | ✅ Approved | 2025-10-16 | 85-90% effectiveness, 11x ROI |
|| DD-EFFECTIVENESS-003 | [RemediationRequest Watch Strategy](./DD-EFFECTIVENESS-003-RemediationRequest-Watch-Strategy.md) | Effectiveness Monitor | ✅ Approved | 2025-10-16 | 92% confidence, future-proof design |
|| DD-HOLMESGPT-009 | [Self-Documenting JSON Format](./DD-HOLMESGPT-009-Ultra-Compact-JSON-Format.md) | HolmesGPT API / All AI Services | ✅ Approved | 2025-10-16 | 60% token reduction, $5,500/year savings, 100% readability, zero legend overhead |
|| DD-HOLMESGPT-009-ADD | [YAML Evaluation Addendum](./DD-HOLMESGPT-009-ADDENDUM-YAML-Evaluation.md) | HolmesGPT API / All AI Services | ✅ JSON Reaffirmed | 2025-10-16 | YAML evaluated: 17.5% token savings insufficient, JSON proven superior |

**Note**: DD-* prefix is used for detailed design decisions with comprehensive alternatives analysis, implementation strategy, and validation plans. ADR-* prefix is used for architectural records.

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
