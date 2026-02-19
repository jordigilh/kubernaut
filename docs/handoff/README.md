# Cross-Team Questions & Coordination

> **Note (ADR-056/ADR-055):** References to `EnrichmentResults.DetectedLabels` and `EnrichmentResults.OwnerChain` in this document are historical. These fields were removed: DetectedLabels is now computed by HAPI post-RCA (ADR-056), and OwnerChain is resolved via get_resource_context (ADR-055).

**Last Updated**: December 3, 2025
**Purpose**: Centralized Q&A hub for cross-team coordination and integration alignment

---

## Overview

This directory contains questions and coordination documents across teams for:
1. **Feature Integration**: HolmesGPT-API v3.2 integration questions
2. **V1.0 Timeline Coordination**: Production readiness and API contract alignment (4 weeks to launch)

---

## ✅ **V1.0 Timeline Coordination** (ALL RESPONDED)

| Document | Priority | Audience | Status | Topics |
|----------|----------|----------|--------|--------|
| [V1.0-TIMELINE-QUESTIONS.md](./V1.0-TIMELINE-QUESTIONS.md) | 🔴 **CRITICAL** | All 4 remaining teams | ✅ **ALL RESPONDED** (Dec 2) | Production readiness, test coverage, API contracts, timeline risks |

**Context**: 4/8 services production-ready, 4 weeks remaining to V1.0 launch
**Status**: ✅ All 4 teams responded (Signal Processing, AI Analysis, Workflow Execution, Remediation Orchestrator)

**Summary of Responses**:
- **Signal Processing**: 14-17 days, target Dec 16-19, design complete
- **AI Analysis**: 2 weeks, target Dec 16, depends on SP CRDs
- **Workflow Execution**: 2 weeks, target Dec 16, ~60 tests currently
- **Remediation Orchestrator**: Design complete, implementation in progress

**Key Concerns** (from responses):
- Test coverage standards (70%+ unit, >50% integration)
- API contract finalization (lock down by Dec 5)
- Service deferrals: Effectiveness Monitor → V1.1 (DD-017), Dynamic Toolset → V2.0 (DD-016)
- Integration testing coordination: AIAnalysis depends on SignalProcessing CRDs

---

## 📋 **Documentation Standardization Requests** (Dec 2-3, 2025)

| Team | Document | Priority | Status | Effort | Topics |
|------|----------|----------|--------|--------|--------|
| **Data Storage** | [DOCUMENTATION_STANDARDIZATION_REQUEST.md](./DOCUMENTATION_STANDARDIZATION_REQUEST.md) | ✅ **DONE** | ✅ **v2.1 COMPLETE** | 2-3 hours | Doc Index, File Organization, Implementation Structure |
| **Gateway** | [DOCUMENTATION_STANDARDIZATION_REQUEST.md](./DOCUMENTATION_STANDARDIZATION_REQUEST.md) | ✅ **DONE** | ✅ **v1.5 COMPLETE** | 1-2 hours | Doc Index, File Organization, Implementation Structure |
| **HolmesGPT API** | [DOCUMENTATION_STANDARDIZATION_REQUEST_HOLMESGPT_API.md](./DOCUMENTATION_STANDARDIZATION_REQUEST_HOLMESGPT_API.md) | 🟢 **P3** | ⏳ **PENDING** | 30 min | Add Implementation Structure only |

**Status**: 89% compliance (8/9 services) - P1 items completed ahead of schedule! 🎉
**Remaining**: HolmesGPT API (2/3 sections - only needs Implementation Structure, P3 optional)

---

## 📋 **HolmesGPT-API Integration Questions** (v3.2 Release)

| Team | Document | Priority | Status | Topics |
|------|----------|----------|--------|--------|
| **Workflow Engine** | [QUESTIONS_FOR_WORKFLOW_ENGINE_TEAM.md](./QUESTIONS_FOR_WORKFLOW_ENGINE_TEAM.md) | High | ✅ **RESPONDED** | Parameter validation architecture (DD-HAPI-002), ValidateParameters implementation, metrics |
| **Data Storage** | [QUESTIONS_FOR_DATA_STORAGE_TEAM.md](./QUESTIONS_FOR_DATA_STORAGE_TEAM.md) | Medium | ✅ **RESPONDED** | container_image in search, pagination, detected_labels population |
| **SignalProcessing** | [QUESTIONS_FOR_SIGNALPROCESSING_TEAM.md](./QUESTIONS_FOR_SIGNALPROCESSING_TEAM.md) | Medium | ✅ **RESPONDED** | custom_labels source/extraction, RCA context fields, error handling |
| **AIAnalysis** | [QUESTIONS_FOR_AIANALYSIS_TEAM.md](./QUESTIONS_FOR_AIANALYSIS_TEAM.md) | High | ✅ **RESPONDED** | Response format, WorkflowExecution creation, confidence thresholds |

---

## 📋 **AIAnalysis Integration Questions** (V1.0 Contract Alignment)

| To Team | Document | Priority | Status | Topics |
|---------|----------|----------|--------|--------|
| **Remediation Orchestrator** | [AIANALYSIS_TO_RO_TEAM.md](./AIANALYSIS_TO_RO_TEAM.md) | High | ✅ **RESOLVED** | CustomLabels mapping, shared types import, free-text Environment |
| **SignalProcessing** | [AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md](./AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md) | High | ✅ **RESOLVED** (Dec 2) | Type aliases, EnrichmentResults population, OwnerChain, DetectedLabels |
| **HolmesGPT-API** | [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](./AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) | High | ✅ **RESOLVED** (Dec 2) | MCP connection, DetectedLabels filtering, request/response schemas |

---

## Quick Summary of Key Questions

### Workflow Engine Team ✅ FULLY RESPONDED
1. **DD-HAPI-002 Approval**: ✅ **APPROVED** → **SIMPLIFIED** to 2-layer (WE validation removed)
2. **ValidateParameters**: ❌ **CANCELLED** - BR-WE-001 cancelled, HAPI is sole validator
3. **Metrics**: ✅ **UPDATED** - Resource locking metrics (skip reasons, active locks)
4. **Q-GW-01 (TargetResource)**: ✅ **ANSWERED** - Option C: WE validates format, rejects malformed with `ConfigurationError`
5. **V1.0 Timeline**: ✅ **ANSWERED** - Production-ready target: Week 4 (Dec 16, 2025)

**WE v3.1 Updates**: `FailureDetails` with `wasExecutionFailure` flag, `SkipDetails` for resource locking, `targetResource` validation.

### Data Storage Team ✅ FULLY RESPONDED
1. **container_image**: ✅ **CONFIRMED** - Fields ARE included in search responses (recent fix applied)
2. **detected_labels**: ✅ **CLARIFIED** - Auto-population NOT DS responsibility (see DD-WORKFLOW-001 v2.0)
3. **Pagination**: ✅ **CONFIRMED** - `top_k` max 100, offset-based pagination supported

> **Note (Dec 2, 2025)**: Original Q3 was based on a misunderstanding. `detected_labels` in workflow catalog is **author-defined metadata**, not auto-populated. **SignalProcessing** auto-populates incident `DetectedLabels` from live K8s (V1.0 ✅ IMPLEMENTED).

### SignalProcessing Team ✅ RESPONDED
1. **custom_labels**: ✅ **Rego-based extraction** (NOT Alertmanager directly) - from KubernetesContext + DetectedLabels
2. **RCA Context**: ✅ **ALL CONFIRMED** - `resource_kind`, `cluster_name`, `owner_chain[]` all included
3. **Validation**: ✅ **ENFORCED** - max 10 keys, max 5 values/key, 63 char keys, 100 char values
4. **Error Handling**: ✅ **DOCUMENTED** - Exponential backoff retry (5 attempts), CRD status updates

**SP Follow-up Questions ANSWERED**: Prompt size (~64k soft), custom_labels NOT in prompt (auto-append per DD-HAPI-001)

### AIAnalysis Team ✅ FULLY RESPONDED
1. **Response Format**: ✅ **CLARIFIED** - Per ADR-045: `containerImage`, `containerDigest`, `rationale` included; `version` NOT included (use containerImage)
2. **WorkflowExecution**: ✅ **CLARIFIED** - AIAnalysis doesn't create WE, RO does. Parameters are direct passthrough
3. **Confidence Thresholds**: ✅ **DOCUMENTED** - AIAnalysis Rego policies determine approval (not HolmesGPT-API)
4. **Audit Trail**: ✅ **CLARIFIED** - HAPI maintains internally (NOT in API response)

> **Note (Dec 2, 2025)**: Original action items were based on initial assumptions. See **ADR-045** and **AIANALYSIS_TO_HOLMESGPT_API_TEAM.md** for authoritative contract. `version` is NOT included (use `containerImage` + `containerDigest`); `severity`/`signal_type` ARE already in `root_cause_analysis`.

---

### AIAnalysis → Teams (Questions Raised)

#### To Remediation Orchestrator ✅ RESOLVED (Dec 2, 2025)
- ~~CustomLabels mapping after field removals~~ → Pass-through, RO doesn't transform
- ~~Shared types import approach~~ → Type aliases work, no changes needed
- ~~Free-text Environment/BusinessPriority acceptance~~ → No concerns, RO treats as opaque strings

#### To SignalProcessing ✅ RESOLVED (Dec 2, 2025)
- ~~Type alias compile compatibility~~ → Compiles successfully
- ~~EnrichmentResults field population~~ → 4/5 fields planned; EnrichmentQuality NOT implementing
- ~~OwnerChain traversal implementation~~ → Depth limit: None (traverse to root)
- ~~CustomLabels Rego extraction~~ → Sandboxed OPA, Days 8-9
- ~~DetectedLabels auto-detection methods~~ → All 9 fields with documented methods

#### To HolmesGPT-API ✅ RESOLVED (Dec 2, 2025)
- ~~MCP server connection details~~ → No MCP server, use HTTP REST
- ~~DetectedLabels filtering strategy~~ → Subset matching, correct schema provided
- ~~OwnerChain validation implementation~~ → Implemented with degraded mode
- ~~Request/Response schema confirmation~~ → Corrected schemas with OpenAPI reference
- ~~Approval determination method~~ → HAPI doesn't determine; AIAnalysis Rego does
- ~~Retry/timeout recommendations~~ → 30s timeout, RFC 7807 errors
- ~~CustomLabels LLM prompt incorporation~~ → NOT in prompt, auto-appended to search

---

## 📋 **HolmesGPT-API Execution Decisions** (NEW)

**Document**: [DECISIONS_HAPI_EXECUTION_RESPONSIBILITIES.md](./DECISIONS_HAPI_EXECUTION_RESPONSIBILITIES.md)
**Context**: Key architectural decisions about HAPI's execution/recovery responsibilities

| Decision | Summary |
|----------|---------|
| **naturalLanguageSummary** | HAPI consumes WE-generated summary for recovery prompts |
| **No Retry in HAPI** | HAPI reports results, RO decides all retry/recovery actions |
| **Flexible Parameters** | No hardcoded format - workflow schema defines parameter casing |

---

## 📁 **WorkflowExecution Integration Questions** (v3.1 Release)

**Document**: [QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md](./QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md)
**Context**: WE v3.1 - Resource Locking & Safety Features (DD-WE-001, BR-WE-009/010/011)

| To Team | Questions | Priority | Status |
|---------|-----------|----------|--------|
| **HolmesGPT-API** | 3 | High | ✅ **RESOLVED** (Dec 1) |
| **RO** | 5 | High | ✅ **RESOLVED** (Dec 2) |
| **Gateway** | 3 | Medium | ✅ **RESOLVED** (Dec 1) |
| **AIAnalysis** | 4 | Medium | ✅ **RESOLVED** (Dec 2) |
| **Notification** | 5 | Medium | ✅ **RESOLVED** (Dec 1) |
| ~~**DataStorage**~~ | ~~5~~ | ~~High~~ | ✅ **CANCELLED** |

### Resolved Questions Summary - ✅ ALL COMPLETE

| ID | To Team | Resolution |
|----|---------|------------|
| WE→HAPI-001 | HolmesGPT-API | ✅ Yes - will use `naturalLanguageSummary` for recovery prompts |
| WE→HAPI-002 | HolmesGPT-API | ✅ HAPI doesn't retry - RO decides retry/recovery policy |
| WE→HAPI-003 | HolmesGPT-API | ✅ No hardcoded format - workflow defines parameter casing |
| WE→GW-* | Gateway | ✅ All resolved - namespace empty for cluster-scoped, uses NormalizedSignal |
| WE→DS-* | DataStorage | ✅ **CANCELLED** - BR-WE-001 cancelled, HAPI is sole validator |
| WE→RO-001 | RO | ✅ Format: `namespace/kind/name` (namespaced) or `kind/name` (cluster-scoped) |
| WE→RO-002 | RO | ✅ Option D - Per-reason skip handling (ResourceBusy, RecentlyRemediated) |
| WE→RO-003 | RO | ✅ NO auto-retry for execution failures, manual review notification |
| WE→RO-004 | RO | ✅ Complete pass-through per DD-CONTRACT-002 v1.2 |
| WE→RO-005 | RO | ✅ One WE per RR by design |
| WE→AIA-* | AIAnalysis | ✅ All resolved - skip notifications consolidated to completion

---

## 📁 **SignalProcessing Integration Questions** (v1.0 Release)

**Document**: [QUESTIONS_FROM_SIGNALPROCESSING_TEAM.md](./QUESTIONS_FROM_SIGNALPROCESSING_TEAM.md)
**Context**: SignalProcessing v1.0 - Integration alignment with AIAnalysis, Gateway, RO

| To Team | Questions | Priority | Status |
|---------|-----------|----------|--------|
| **AIAnalysis** | 3 | 🔴 High | ✅ **RESOLVED** (Dec 2) |
| **Gateway** | 3 | 🟡 Medium | ✅ **RESOLVED** (Dec 2) |
| **RO** | 3 | 🔴 High | ✅ **RESOLVED** |

### Key Questions (Summary) - ✅ ALL RESOLVED

| ID | To Team | Resolution |
|----|---------|------------|
| SP→AIA-001 | AIAnalysis | ✅ Path confirmed: `spec.analysisRequest.signalContext.enrichmentResults.*` |
| SP→AIA-002 | AIAnalysis | ✅ AIAnalysis passes through to HolmesGPT - all 9 fields used |
| SP→AIA-003 | AIAnalysis | ✅ Detection fails → `false` + error log (no "unknown" state) |
| SP→GW-001 | Gateway | ✅ Ordering doesn't matter for search filtering |
| SP→GW-002 | Gateway | ✅ Gateway doesn't enforce limits - SP limits sufficient |
| SP→GW-003 | Gateway | ✅ Rego sandbox + parameterized queries sufficient |

---

## How to Respond

### For V1.0 Timeline Questions (URGENT - by Dec 3, 2025)
1. Open [V1.0-TIMELINE-QUESTIONS.md](./V1.0-TIMELINE-QUESTIONS.md)
2. Find your service section
3. Copy the "Response Template" at the bottom of the document
4. Fill in all sections (Production Readiness Checklist, Test Count, Timeline, Blockers, etc.)
5. Add your response to the document and commit

### For HolmesGPT-API Integration Questions
1. Open the relevant document for your team
2. Scroll to the "Response" section at the bottom
3. Fill in your answers with date and responder name
4. Commit the changes or notify the HolmesGPT-API team

---

## 📚 **Complete Q&A Document Index**

### Documents in `docs/handoff/` (Primary Location)

| Document | From → To | Status |
|----------|-----------|--------|
| [DOCUMENTATION_STANDARDIZATION_REQUEST.md](./DOCUMENTATION_STANDARDIZATION_REQUEST.md) | Doc Team → DS/GW | ✅ Complete |
| [DOCUMENTATION_STANDARDIZATION_REQUEST_HOLMESGPT_API.md](./DOCUMENTATION_STANDARDIZATION_REQUEST_HOLMESGPT_API.md) | Doc Team → HGPT | ⏳ Pending (P3) |
| [QUESTIONS_FOR_WORKFLOW_ENGINE_TEAM.md](./QUESTIONS_FOR_WORKFLOW_ENGINE_TEAM.md) | HAPI → WE | ✅ Resolved |
| [QUESTIONS_FOR_DATA_STORAGE_TEAM.md](./QUESTIONS_FOR_DATA_STORAGE_TEAM.md) | HAPI → DS | ✅ Resolved |
| [QUESTIONS_FOR_SIGNALPROCESSING_TEAM.md](./QUESTIONS_FOR_SIGNALPROCESSING_TEAM.md) | HAPI → SP | ✅ Resolved |
| [QUESTIONS_FOR_AIANALYSIS_TEAM.md](./QUESTIONS_FOR_AIANALYSIS_TEAM.md) | HAPI → AIA | ✅ Resolved |
| [QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md](./QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md) | WE → Multiple | ✅ Resolved |
| [QUESTIONS_FROM_SIGNALPROCESSING_TEAM.md](./QUESTIONS_FROM_SIGNALPROCESSING_TEAM.md) | SP → Multiple | ✅ Resolved |
| [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](./AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) | AIA → HAPI | ✅ Resolved |
| [AIANALYSIS_TO_RO_TEAM.md](./AIANALYSIS_TO_RO_TEAM.md) | AIA → RO | ✅ Resolved |
| [AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md](./AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md) | AIA → SP | ✅ Resolved |
| [RESPONSE_TO_AIANALYSIS_TEAM.md](./RESPONSE_TO_AIANALYSIS_TEAM.md) | HAPI → AIA | ✅ Complete |
| [V1.0-TIMELINE-QUESTIONS.md](./V1.0-TIMELINE-QUESTIONS.md) | All Teams | ✅ Resolved |

### Documents in Service Directories (Legacy Location)

> **Note**: These documents remain in service directories for historical context.
> New Q&A documents should be created in `docs/handoff/`.

| Document | Location | Status |
|----------|----------|--------|
| GATEWAY_QUESTIONS_FOR_RO.md | `docs/services/crd-controllers/05-remediationorchestrator/` | ✅ Resolved |
| GATEWAY_QUESTIONS_FOR_SP.md | `docs/services/crd-controllers/01-signalprocessing/` | ✅ Resolved |
| QUESTIONS_FOR_AIANALYSIS_TEAM.md | `docs/services/crd-controllers/01-signalprocessing/` | ✅ Resolved |
| QUESTIONS_FOR_GATEWAY_TEAM.md | `docs/services/crd-controllers/01-signalprocessing/` | ✅ Resolved |
| RESPONSE_SIGNALPROCESSING_INTEGRATION_VALIDATION.md | `docs/services/crd-controllers/02-aianalysis/` | ✅ Complete |
| RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md | `docs/services/crd-controllers/01-signalprocessing/` | ✅ Complete |

### Naming Convention

**New documents should follow this pattern**:
```
QUESTIONS_FROM_[ASKING_TEAM]_TO_[ANSWERING_TEAM].md
RESPONSE_[TOPIC].md
```

**Team Abbreviations**:
- HAPI = HolmesGPT-API
- DS = Data Storage
- WE = Workflow Engine
- SP = SignalProcessing
- AIA = AIAnalysis
- RO = Remediation Orchestrator
- GW = Gateway
- NOT = Notification

---

## Related Documents

- [DD-HAPI-001: Custom Labels Auto-Append](../architecture/decisions/DD-HAPI-001-custom-labels-auto-append.md)
- [DD-HAPI-002: Workflow Parameter Validation](../architecture/decisions/DD-HAPI-002-workflow-parameter-validation.md)
- [DD-WORKFLOW-001: Mandatory Label Schema](../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) - **AUTHORITATIVE** for DetectedLabels
- [pkg/shared/types/enrichment.go](../../pkg/shared/types/enrichment.go) - **AUTHORITATIVE** Go schema for DetectedLabels
- [HolmesGPT-API README](../../holmesgpt-api/README.md)

---

## Contact

For questions about these documents, contact the HolmesGPT-API team.

