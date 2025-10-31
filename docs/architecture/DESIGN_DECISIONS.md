# Architectural Design Decisions

**Purpose**: This document provides a quick reference index to all architectural design decisions made for the Kubernaut project.

**Format**: Each decision is documented in a separate file following the DD-* naming convention in `docs/architecture/decisions/`.

---

## 📋 Quick Reference

| ID | Decision | Status | Date | File |
|---|---|---|---|---|
| DD-001 | Recovery Context Enrichment (Alternative 2) | ✅ Approved | 2024-10-08 | [DD-001-recovery-context-enrichment.md](decisions/DD-001-recovery-context-enrichment.md) |
| DD-002 | Per-Step Validation Framework (Alternative 2) | ✅ Approved | 2025-10-14 | [DD-002-per-step-validation-framework.md](decisions/DD-002-per-step-validation-framework.md) |
| DD-003 | Forced Recommendation Manual Override (V2) | ✅ Approved for V2 | 2025-10-20 | [DD-003-forced-recommendation-manual-override.md](decisions/DD-003-forced-recommendation-manual-override.md) |
| DD-004 | RFC 7807 Error Response Standard | ✅ Approved | 2025-10-30 | [DD-004-RFC7807-ERROR-RESPONSES.md](decisions/DD-004-RFC7807-ERROR-RESPONSES.md) |
| DD-005 | Observability Standards (Metrics and Logging) | ✅ Approved | 2025-10-31 | [DD-005-OBSERVABILITY-STANDARDS.md](decisions/DD-005-OBSERVABILITY-STANDARDS.md) |
| DD-006 | Controller Scaffolding Strategy (Custom Templates) | ✅ Approved | 2025-10-31 | [DD-006-controller-scaffolding-strategy.md](decisions/DD-006-controller-scaffolding-strategy.md) |
| DD-007 | Kubernetes-Aware Graceful Shutdown Pattern | ✅ Approved | 2025-10-31 | [DD-007-kubernetes-aware-graceful-shutdown.md](decisions/DD-007-kubernetes-aware-graceful-shutdown.md) |
| DD-SCHEMA-001 | Data Storage Schema Authority | ✅ Approved | 2025-10-31 | [DD-SCHEMA-001-data-storage-schema-authority.md](decisions/DD-SCHEMA-001-data-storage-schema-authority.md) |
| DD-CONTEXT-001 | Cache Stampede Prevention (Alternative A) | ✅ Approved | 2025-10-20 | [DD-CONTEXT-001-cache-stampede-prevention.md](decisions/DD-CONTEXT-001-cache-stampede-prevention.md) |
| DD-CONTEXT-002 | Cache Size Limit Configuration (Alternative C) | ✅ Approved | 2025-10-20 | [DD-CONTEXT-002-cache-size-limit-configuration.md](decisions/DD-CONTEXT-002-cache-size-limit-configuration.md) |
| DD-CONTEXT-003 | Context Enrichment Placement (LLM-Driven Tool Call) | ✅ Approved | 2025-10-22 | [DD-CONTEXT-003-Context-Enrichment-Placement.md](decisions/DD-CONTEXT-003-Context-Enrichment-Placement.md) |
| DD-CONTEXT-004 | BR-AI-002 Ownership | ✅ Approved | 2025-10-22 | [DD-CONTEXT-004-BR-AI-002-Ownership.md](decisions/DD-CONTEXT-004-BR-AI-002-Ownership.md) |
| DD-EFFECTIVENESS-002 | Restart Recovery Idempotency | ✅ Approved | [Date] | [DD-EFFECTIVENESS-002-Restart-Recovery-Idempotency.md](decisions/DD-EFFECTIVENESS-002-Restart-Recovery-Idempotency.md) |
| DD-GATEWAY-004 | Redis Memory Optimization | ✅ Approved | 2025-10-24 | [DD-GATEWAY-004-redis-memory-optimization.md](decisions/DD-GATEWAY-004-redis-memory-optimization.md) |
| DD-GATEWAY-005 | Redis Cleanup on CRD Deletion | ✅ Approved | 2025-10-27 | [DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md](decisions/DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md) |
| DD-GATEWAY-006 | Authentication Strategy | ✅ Approved | 2025-10-27 | [DD-GATEWAY-006-authentication-strategy.md](decisions/DD-GATEWAY-006-authentication-strategy.md) |
| DD-GATEWAY-007 | Fallback Namespace Strategy | ✅ Approved | 2025-10-31 | [DD-GATEWAY-007-fallback-namespace-strategy.md](decisions/DD-GATEWAY-007-fallback-namespace-strategy.md) |
| DD-HOLMESGPT-005 | Test Strategy Validation | ✅ Validated | [Date] | [DD-HOLMESGPT-005-Test-Strategy-Validation.md](decisions/DD-HOLMESGPT-005-Test-Strategy-Validation.md) |
| DD-HOLMESGPT-006 | Implementation Plan Quality Gate | ✅ Approved | [Date] | [DD-HOLMESGPT-006-Implementation-Plan-Quality-Gate.md](decisions/DD-HOLMESGPT-006-Implementation-Plan-Quality-Gate.md) |
| DD-HOLMESGPT-007 | Service Boundaries Clarification | ✅ Approved | [Date] | [DD-HOLMESGPT-007-Service-Boundaries-Clarification.md](decisions/DD-HOLMESGPT-007-Service-Boundaries-Clarification.md) |
| DD-HOLMESGPT-008 | Safety-Aware Investigation | ✅ Approved | [Date] | [DD-HOLMESGPT-008-Safety-Aware-Investigation.md](decisions/DD-HOLMESGPT-008-Safety-Aware-Investigation.md) |
| DD-HOLMESGPT-013 | Vendor Local SDK Copy | ✅ Approved | [Date] | [DD-HOLMESGPT-013-Vendor-Local-SDK-Copy.md](decisions/DD-HOLMESGPT-013-Vendor-Local-SDK-Copy.md) |
| DD-HOLMESGPT-014 | MinimalDAL Stateless Architecture | ✅ Approved | [Date] | [DD-HOLMESGPT-014-MinimalDAL-Stateless-Architecture.md](decisions/DD-HOLMESGPT-014-MinimalDAL-Stateless-Architecture.md) |

**Note**: For complete decision details, alternatives considered, implementation guidance, and consequences, see the individual DD-* files in `docs/architecture/decisions/`.

---

## 📝 When to Create a New DD

Create a new DD document for decisions that:
- ✅ Affect multiple services or the overall architecture
- ✅ Have long-term implications (>6 months)
- ✅ Involve trade-offs between alternatives
- ✅ Set precedents for future decisions
- ✅ Change existing architectural patterns

---

## 🔗 Related Documentation

- **ADRs**: [docs/architecture/decisions/](decisions/) - Architectural Decision Records (ADR-001 through ADR-028)
- **Service-Specific DDs**: Check individual service documentation in `docs/services/`
- **Business Requirements**: [docs/requirements/](../requirements/)
- **APDC Methodology**: [.cursor/rules/00-core-development-methodology.mdc](../../.cursor/rules/00-core-development-methodology.mdc)

---

**Last Updated**: October 31, 2025
**Maintained By**: Kubernaut Architecture Team
