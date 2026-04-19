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
| DD-008 | Integration Test Infrastructure (Podman + Kind) | ✅ Approved | 2025-11-01 | [DD-008-integration-test-infrastructure.md](decisions/DD-008-integration-test-infrastructure.md) |
| DD-009 | Audit Write Error Recovery - Dead Letter Queue Pattern | ✅ Approved | TBD | [DD-009-audit-write-error-recovery.md](decisions/DD-009-audit-write-error-recovery.md) |
| DD-010 | PostgreSQL Driver Migration (lib/pq to pgx) | ✅ Approved | 2025-11-03 | [DD-010-postgresql-driver-migration.md](decisions/DD-010-postgresql-driver-migration.md) |
| DD-011 | PostgreSQL 16+ and pgvector 0.5.1+ Version Requirements | ✅ Approved | 2025-10-13 | [DD-011-postgresql-version-requirements.md](decisions/DD-011-postgresql-version-requirements.md) |
| DD-012 | Goose Database Migration Management | ✅ Approved | 2025-11-05 | [DD-012-goose-database-migration-management.md](decisions/DD-012-goose-database-migration-management.md) |
| DD-013 | Kubernetes Client Initialization Standard | ✅ Approved | 2025-11-08 | [DD-013-kubernetes-client-initialization-standard.md](decisions/DD-013-kubernetes-client-initialization-standard.md) |
| DD-014 | Binary Version Logging Standard | ✅ Approved | 2025-11-17 | [DD-014-binary-version-logging-standard.md](decisions/DD-014-binary-version-logging-standard.md) |
| DD-015 | Timestamp-Based CRD Naming for Unique Occurrences | ✅ Approved | 2025-11-17 | [DD-015-timestamp-based-crd-naming.md](decisions/DD-015-timestamp-based-crd-naming.md) |
| DD-016 | Dynamic Toolset Service V2.0 Deferral | ⏸️ Deferred to V2.0 | 2025-11-21 | [DD-016-dynamic-toolset-v2-deferral.md](decisions/DD-016-dynamic-toolset-v2-deferral.md) |
| DD-HTTP-001 | HTTP Router Strategy (chi for REST APIs, stdlib for simple services) | ✅ Approved | 2025-11-22 | [DD-HTTP-001-http-router-strategy.md](decisions/DD-HTTP-001-http-router-strategy.md) |
| ADR-001 | CRD-Based Microservices Architecture | ✅ Approved | TBD | [ADR-001-crd-microservices-architecture.md](decisions/ADR-001-crd-microservices-architecture.md) |
| ADR-002 | Native Kubernetes Jobs for Remediation Execution | ✅ Approved | TBD | [ADR-002-native-kubernetes-jobs.md](decisions/ADR-002-native-kubernetes-jobs.md) |
| ADR-003 | Kind Cluster as Primary Integration Environment | ✅ Approved | TBD | [ADR-003-KIND-INTEGRATION-ENVIRONMENT.md](decisions/ADR-003-KIND-INTEGRATION-ENVIRONMENT.md) |
| ADR-004 | Fake Kubernetes Client for Unit Testing | ✅ Approved | TBD | [ADR-004-fake-kubernetes-client.md](decisions/ADR-004-fake-kubernetes-client.md) |
| ADR-005 | >50% Integration Test Coverage for Microservices | ✅ Approved | TBD | [ADR-005-integration-test-coverage.md](decisions/ADR-005-integration-test-coverage.md) |
| ADR-014 | Notification Service Uses External Service Authentication | ✅ Approved | TBD | [ADR-014-notification-service-external-auth.md](decisions/ADR-014-notification-service-external-auth.md) |
| ADR-015 | Migrate from "Alert" to "Signal" Naming Convention | ✅ Approved | TBD | [ADR-015-alert-to-signal-naming-migration.md](decisions/ADR-015-alert-to-signal-naming-migration.md) |
| ADR-016 | Validation Responsibility Chain and Data Authority Model | ✅ Approved | TBD | [ADR-016-validation-responsibility-chain.md](decisions/ADR-016-validation-responsibility-chain.md) |
| ADR-017 | NotificationRequest CRD Creator Responsibility | ✅ Approved | TBD | [ADR-017-notification-crd-creator.md](decisions/ADR-017-notification-crd-creator.md) |
| ADR-018 | Approval Notification Integration in V1.0 | ✅ Approved | TBD | [ADR-018-approval-notification-v1-integration.md](decisions/ADR-018-approval-notification-v1-integration.md) |
| ADR-019 | HolmesGPT Circuit Breaker & Retry Strategy | ✅ Approved | TBD | [ADR-019-holmesgpt-circuit-breaker-retry-strategy.md](decisions/ADR-019-holmesgpt-circuit-breaker-retry-strategy.md) |
| ADR-020 | Workflow Parallel Execution Limits & Complexity Approval | ✅ Approved | TBD | [ADR-020-workflow-parallel-execution-limits.md](decisions/ADR-020-workflow-parallel-execution-limits.md) |
| ADR-021 | Workflow Dependency Cycle Detection & Validation | ✅ Approved | TBD | [ADR-021-workflow-dependency-cycle-detection.md](decisions/ADR-021-workflow-dependency-cycle-detection.md) |
| ADR-022 | V1 Native Jobs with V2 Tekton Migration Path | ✅ Approved | TBD | [ADR-022-v1-native-jobs-v2-tekton-migration.md](decisions/ADR-022-v1-native-jobs-v2-tekton-migration.md) |
| ADR-023 | Tekton Pipelines from V1 (Eliminate Custom Orchestration) | ✅ Approved | TBD | [ADR-023-tekton-from-v1.md](decisions/ADR-023-tekton-from-v1.md) |
| ADR-024 | Eliminate ActionExecution CRD Layer | ✅ Approved | TBD | [ADR-024-eliminate-actionexecution-layer.md](decisions/ADR-024-eliminate-actionexecution-layer.md) |
| ADR-025 | KubernetesExecutor Service Elimination | ✅ Approved | TBD | [ADR-025-kubernetesexecutor-service-elimination.md](decisions/ADR-025-kubernetesexecutor-service-elimination.md) |
| ADR-026 | Forced Recommendation and Manual Override (V2 Feature) | ✅ Approved | TBD | [ADR-026-forced-recommendation-manual-override.md](decisions/ADR-026-forced-recommendation-manual-override.md) |
| ADR-027 | Multi-Architecture Container Build Strategy with Red Hat UBI Base Images | ✅ Approved | TBD | [ADR-027-multi-architecture-build-strategy.md](decisions/ADR-027-multi-architecture-build-strategy.md) |
| ADR-028 | Container Image Registry and Base Image Policy | ✅ Approved | TBD | [ADR-028-container-registry-policy.md](decisions/ADR-028-container-registry-policy.md) |
| ADR-030 | Service Configuration Management via YAML Files | ✅ Approved | TBD | [ADR-030-service-configuration-management.md](decisions/ADR-030-service-configuration-management.md) |
| ADR-031 | OpenAPI Specification Standard for REST APIs | ✅ Approved | TBD | [ADR-031-openapi-specification-standard.md](decisions/ADR-031-openapi-specification-standard.md) |
| ADR-032 | Data Access Layer Isolation | ✅ Approved | TBD | [ADR-032-data-access-layer-isolation.md](decisions/ADR-032-data-access-layer-isolation.md) |
| ADR-033 | Remediation Playbook Catalog & Multi-Dimensional Success Tracking | ✅ Approved | 2025-11-04 | [ADR-033-remediation-playbook-catalog.md](decisions/ADR-033-remediation-playbook-catalog.md) |
| ADR-033-A | Cross-Service Business Requirements (6 Services, 20 BRs) | ✅ Approved | 2025-11-05 | [ADR-033-CROSS-SERVICE-BRS.md](decisions/ADR-033-CROSS-SERVICE-BRS.md) |
| ADR-033-B | BR Category Migration Plan (BR-WORKFLOW → BR-REMEDIATION) | 📋 Planned | 2025-11-05 | [ADR-033-BR-CATEGORY-MIGRATION-PLAN.md](decisions/ADR-033-BR-CATEGORY-MIGRATION-PLAN.md) |
| ADR-034 | Unified Audit Table Design with Event Sourcing Pattern | ✅ Approved | 2025-11-08 | [ADR-034-unified-audit-table-design.md](decisions/ADR-034-unified-audit-table-design.md) |
| ADR-035 | Remediation Execution Engine (Tekton Pipelines) | ✅ Approved | 2025-11-05 | [ADR-035-remediation-execution-engine.md](decisions/ADR-035-remediation-execution-engine.md) |
| ADR-036 | Authentication Authorization Strategy | ✅ Approved | 2025-11-09 | [ADR-036-authentication-authorization-strategy.md](decisions/ADR-036-authentication-authorization-strategy.md) |
| ADR-037 | Business Requirement (BR) Template Standard | ✅ Approved | 2025-11-05 | [ADR-037-business-requirement-template-standard.md](decisions/ADR-037-business-requirement-template-standard.md) |
| ADR-038 | Asynchronous Buffered Audit Ingestion | ✅ Approved | 2025-11-08 | [ADR-038-async-buffered-audit-ingestion.md](decisions/ADR-038-async-buffered-audit-ingestion.md) |
| ADR-039 | Complex Decision Documentation Pattern | ✅ Approved | 2025-11-06 | [ADR-039-complex-decision-documentation-pattern.md](decisions/ADR-039-complex-decision-documentation-pattern.md) |
| ADR-040 | RemediationApprovalRequest CRD Architecture | ✅ Approved | 2025-11-13 | [ADR-040-remediation-approval-request-architecture.md](decisions/ADR-040-remediation-approval-request-architecture.md) |
| DD-ARCH-001 | Data Access Pattern - Final Decision (Alternative 2 + REST) | ✅ Approved | 2025-11-02 | [DD-ARCH-001-FINAL-DECISION.md](decisions/DD-ARCH-001-FINAL-DECISION.md) |
| DD-ARCH-001-A | Data Access Pattern Assessment (3 Alternatives) | 📊 Analysis | 2025-11-01 | [DD-ARCH-001-DATA-ACCESS-PATTERN-ASSESSMENT.md](analysis/DD-ARCH-001-DATA-ACCESS-PATTERN-ASSESSMENT.md) |
| DD-ARCH-001-B | Interface Options Analysis (REST vs gRPC vs GraphQL) | 📊 Analysis | 2025-11-02 | [DD-ARCH-001-INTERFACE-OPTIONS-ANALYSIS.md](analysis/DD-ARCH-001-INTERFACE-OPTIONS-ANALYSIS.md) |
| DD-ARCH-002 | GraphQL Query Layer Assessment (V2 Candidate) | 📊 Evaluated for V2 | 2025-11-02 | [DD-ARCH-002-GRAPHQL-QUERY-LAYER-ASSESSMENT.md](decisions/DD-ARCH-002-GRAPHQL-QUERY-LAYER-ASSESSMENT.md) |
| DD-SCHEMA-001 | Data Storage Schema Authority | ✅ Approved | 2025-10-31 | [DD-SCHEMA-001-data-storage-schema-authority.md](decisions/DD-SCHEMA-001-data-storage-schema-authority.md) |
| DD-CONTEXT-001 | Cache Stampede Prevention (Alternative A) | ✅ Approved | 2025-10-20 | [DD-CONTEXT-001-cache-stampede-prevention.md](decisions/DD-CONTEXT-001-cache-stampede-prevention.md) |
| DD-CONTEXT-002 | Cache Size Limit Configuration (Alternative C) | ✅ Approved | 2025-10-20 | [DD-CONTEXT-002-cache-size-limit-configuration.md](decisions/DD-CONTEXT-002-cache-size-limit-configuration.md) |
| DD-CONTEXT-003 | Context Enrichment Placement (LLM-Driven Tool Call) | ✅ Approved | 2025-10-22 | [DD-CONTEXT-003-Context-Enrichment-Placement.md](decisions/DD-CONTEXT-003-Context-Enrichment-Placement.md) |
| DD-CONTEXT-004 | BR-AI-002 Ownership | ✅ Approved | 2025-10-22 | [DD-CONTEXT-004-BR-AI-002-Ownership.md](decisions/DD-CONTEXT-004-BR-AI-002-Ownership.md) |
| DD-CONTEXT-005 | Minimal LLM Response Schema (Filter Before LLM Pattern) | ✅ Approved | 2025-11-11 | [DD-CONTEXT-005-minimal-llm-response-schema.md](decisions/DD-CONTEXT-005-minimal-llm-response-schema.md) |
| DD-CONTEXT-006 | Signal Processor Recovery Data Source (Embed, Don't Query) | ✅ Approved | 2025-11-11 | [DD-CONTEXT-006-signal-processor-recovery-data-source.md](decisions/DD-CONTEXT-006-signal-processor-recovery-data-source.md) |
| DD-CATEGORIZATION-001 | Gateway vs Signal Processing Categorization Split Assessment | ✅ Approved | 2025-11-11 | [DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md](decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
| DD-EFFECTIVENESS-002 | Restart Recovery Idempotency | ✅ Approved | [Date] | [DD-EFFECTIVENESS-002-Restart-Recovery-Idempotency.md](decisions/DD-EFFECTIVENESS-002-Restart-Recovery-Idempotency.md) |
| DD-GATEWAY-004 | Redis Memory Optimization | ✅ Approved | 2025-10-24 | [DD-GATEWAY-004-redis-memory-optimization.md](decisions/DD-GATEWAY-004-redis-memory-optimization.md) |
| DD-GATEWAY-005 | Redis Cleanup on CRD Deletion | ✅ Approved | 2025-10-27 | [DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md](decisions/DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md) |
| DD-GATEWAY-006 | Authentication Strategy | ✅ Approved | 2025-10-27 | [DD-GATEWAY-006-authentication-strategy.md](decisions/DD-GATEWAY-006-authentication-strategy.md) |
| DD-GATEWAY-007 | Fallback Namespace Strategy | ✅ Approved | 2025-10-31 | [DD-GATEWAY-007-fallback-namespace-strategy.md](decisions/DD-GATEWAY-007-fallback-namespace-strategy.md) |
| DD-GATEWAY-008 | Storm Aggregation Windows | ❌ Superseded (DD-GATEWAY-015) | 2025-12-13 | [DD-GATEWAY-008-storm-aggregation-windows.md](decisions/DD-GATEWAY-008-storm-aggregation-windows.md) |
| DD-GATEWAY-009 | State-Based Deduplication Strategy (Alternative 3: Hybrid Redis Cache + CRD State) | ⏸️ Parked | 2025-11-17 | [DD-GATEWAY-009-state-based-deduplication.md](decisions/DD-GATEWAY-009-state-based-deduplication.md) |
| DD-GATEWAY-010 | Adapter Naming Convention (SignalSource vs SignalType) | ✅ Approved | 2025-11-21 | [DD-GATEWAY-010-adapter-naming-convention.md](decisions/DD-GATEWAY-010-adapter-naming-convention.md) |
| DD-GATEWAY-012 | Redis-free Storm Detection | ❌ Superseded | 2025-12-13 | Referenced in code but never formally documented; superseded by DD-GATEWAY-015 |
| DD-GATEWAY-014 | Service-Level Circuit Breaker Deferral | ⏸️ Deferred | 2025-12-13 | [DD-GATEWAY-014-circuit-breaker-deferral.md](decisions/DD-GATEWAY-014-circuit-breaker-deferral.md) |
| DD-GATEWAY-015 | Storm Detection Logic Removal | ✅ IMPLEMENTED | 2025-12-13 | [DD-GATEWAY-015-storm-detection-removal.md](decisions/DD-GATEWAY-015-storm-detection-removal.md) |
| DD-GATEWAY-016 | Kubernetes API Circuit Breaker Implementation | ✅ IMPLEMENTED | 2026-01-03 | [DD-GATEWAY-016-k8s-api-circuit-breaker.md](decisions/DD-GATEWAY-016-k8s-api-circuit-breaker.md) |
| DD-HOLMESGPT-005 | Test Strategy Validation | ✅ Validated | [Date] | [DD-HOLMESGPT-005-Test-Strategy-Validation.md](decisions/DD-HOLMESGPT-005-Test-Strategy-Validation.md) |
| DD-HOLMESGPT-006 | Implementation Plan Quality Gate | ✅ Approved | [Date] | [DD-HOLMESGPT-006-Implementation-Plan-Quality-Gate.md](decisions/DD-HOLMESGPT-006-Implementation-Plan-Quality-Gate.md) |
| DD-HOLMESGPT-007 | Service Boundaries Clarification | ✅ Approved | [Date] | [DD-HOLMESGPT-007-Service-Boundaries-Clarification.md](decisions/DD-HOLMESGPT-007-Service-Boundaries-Clarification.md) |
| DD-HOLMESGPT-008 | Safety-Aware Investigation | ✅ Approved | [Date] | [DD-HOLMESGPT-008-Safety-Aware-Investigation.md](decisions/DD-HOLMESGPT-008-Safety-Aware-Investigation.md) |
| DD-HOLMESGPT-013 | Vendor Local SDK Copy | ✅ Approved | [Date] | [DD-HOLMESGPT-013-Vendor-Local-SDK-Copy.md](decisions/DD-HOLMESGPT-013-Vendor-Local-SDK-Copy.md) |
| DD-HOLMESGPT-014 | MinimalDAL Stateless Architecture | ✅ Approved | [Date] | [DD-HOLMESGPT-014-MinimalDAL-Stateless-Architecture.md](decisions/DD-HOLMESGPT-014-MinimalDAL-Stateless-Architecture.md) |
| DD-HAPI-001 | Custom Labels Auto-Append Architecture | ✅ Approved | 2025-11-30 | [DD-HAPI-001-custom-labels-auto-append.md](decisions/DD-HAPI-001-custom-labels-auto-append.md) |
| DD-HAPI-002 | Workflow Parameter Validation Architecture | ✅ Approved | 2025-12-01 | [DD-HAPI-002-workflow-parameter-validation.md](decisions/DD-HAPI-002-workflow-parameter-validation.md) |
| DD-HAPI-003 | Mandatory OpenAPI Client Usage | ✅ Approved | 2025-12-29 | [DD-HAPI-003-mandatory-openapi-client-usage.md](decisions/DD-HAPI-003-mandatory-openapi-client-usage.md) |
| DD-HAPI-005 | Python OpenAPI Client Auto-Regeneration Pattern | ✅ Approved | 2025-12-29 | [DD-HAPI-005-python-openapi-client-regeneration.md](decisions/DD-HAPI-005-python-openapi-client-regeneration.md) |
| DD-EMBEDDING-001 | Embedding Service as MCP Playbook Catalog Server (Python Microservice) | ✅ Approved | 2025-11-14 | [DD-EMBEDDING-001-embedding-service-implementation.md](decisions/DD-EMBEDDING-001-embedding-service-implementation.md) |
| DD-PLAYBOOK-001 | Mandatory Playbook Label Schema (7 Labels) | ✅ Approved | 2025-11-14 | [DD-PLAYBOOK-001-mandatory-label-schema.md](decisions/DD-PLAYBOOK-001-mandatory-label-schema.md) |
| DD-PLAYBOOK-002 | MCP Playbook Catalog Architecture | ✅ Approved | 2025-11-14 | [DD-PLAYBOOK-002-MCP-PLAYBOOK-CATALOG-ARCHITECTURE.md](decisions/DD-PLAYBOOK-002-MCP-PLAYBOOK-CATALOG-ARCHITECTURE.md) |
| DD-INFRA-001 | ConfigMap Hot-Reload Pattern (Shared Infrastructure) | ✅ Approved | 2025-12-06 | [DD-INFRA-001-configmap-hotreload-pattern.md](decisions/DD-INFRA-001-configmap-hotreload-pattern.md) |
| DD-WE-001 | Resource Locking Safety (Prevent Parallel Workflows) | ✅ Approved | 2025-12-01 | [DD-WE-001-resource-locking-safety.md](decisions/DD-WE-001-resource-locking-safety.md) |
| DD-WE-002 | Dedicated Execution Namespace | ✅ Approved | 2025-12-01 | [DD-WE-002-dedicated-execution-namespace.md](decisions/DD-WE-002-dedicated-execution-namespace.md) |
| DD-WE-003 | Resource Lock Persistence (Deterministic PipelineRun Name) | ✅ Approved | 2025-12-01 | [DD-WE-003-resource-lock-persistence.md](decisions/DD-WE-003-resource-lock-persistence.md) |
| DD-WE-004 | Exponential Backoff Cooldown | ✅ Approved | 2025-12-06 | [DD-WE-004-exponential-backoff-cooldown.md](decisions/DD-WE-004-exponential-backoff-cooldown.md) |
| DD-PROD-001 | Production Readiness Checklist Standard | ✅ Approved | 2025-12-07 | [DD-PROD-001-production-readiness-checklist-standard.md](decisions/DD-PROD-001-production-readiness-checklist-standard.md) |
| DD-LLM-003 | Mock-First Development Strategy for LLM Integration | ✅ Approved | 2025-12-11 | [DD-LLM-003-mock-first-development-strategy.md](decisions/DD-LLM-003-mock-first-development-strategy.md) |
| DD-AIANALYSIS-004 | Storm Context NOT Exposed to LLM | ✅ Approved | 2025-12-13 | [DD-AIANALYSIS-004-storm-context-not-exposed.md](decisions/DD-AIANALYSIS-004-storm-context-not-exposed.md) |
| DD-TESTING-001 | Audit Event Validation Standards (Authoritative) | ✅ Approved | 2026-01-02 | [DD-TESTING-001-audit-event-validation-standards.md](decisions/DD-TESTING-001-audit-event-validation-standards.md) |
| DD-TESTING-002 | Integration Test Diagnostics (Must-Gather Pattern) | ✅ Approved | 2026-01-14 | [DD-TESTING-002-integration-test-diagnostics-must-gather.md](decisions/DD-TESTING-002-integration-test-diagnostics-must-gather.md) |
| DD-AUTH-004 | OpenShift OAuth-Proxy for SOC2 Legal Hold Authentication | ✅ Approved | 2026-01-07 | [DD-AUTH-004-openshift-oauth-proxy-legal-hold.md](decisions/DD-AUTH-004-openshift-oauth-proxy-legal-hold.md) |
| DD-AUTH-005 | DataStorage Client Authentication Pattern (Authoritative) | ✅ Approved | 2026-01-07 | [DD-AUTH-005-datastorage-client-authentication-pattern.md](decisions/DD-AUTH-005-datastorage-client-authentication-pattern.md) |

**Note**: For complete decision details, alternatives considered, implementation guidance, and consequences, see the individual DD-* files in `docs/architecture/decisions/`.

---

## 📄 Active Proposals

| ID | Proposal | Status | Date | File |
|---|---|---|---|---|
| PROPOSAL-EXT-001 | External Integration Strategy (MCP, A2A, API Frontend) | 📋 Under Review | 2026-04-15 | [PROPOSAL-EXT-001-external-integration-strategy.md](proposals/PROPOSAL-EXT-001-external-integration-strategy.md) |
| PROPOSAL-EXT-002 | Investigation Prompt Bundles (OCI-packaged prompts and skills) | 📋 Under Review | 2026-04-15 | [PROPOSAL-EXT-002-investigation-prompt-bundles.md](proposals/PROPOSAL-EXT-002-investigation-prompt-bundles.md) |
| PROPOSAL-EXT-003 | Goose Runtime Evaluation and Phased Adoption (ACP Go SDK, 6-phase pipeline, KA-as-compiler) | 📋 Under Review | 2026-04-15 | [PROPOSAL-EXT-003-goose-runtime-evaluation.md](proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) |

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
- **Analysis**: [docs/architecture/analysis/](analysis/) - Supporting analysis for architectural decisions
- **Service-Specific DDs**: Check individual service documentation in `docs/services/`
- **Business Requirements**: [docs/requirements/](../requirements/)
- **APDC Methodology**: [.cursor/rules/00-core-development-methodology.mdc](../../.cursor/rules/00-core-development-methodology.mdc)

---

**Last Updated**: April 15, 2026
**Maintained By**: Kubernaut Architecture Team
