# Test Plan: OCP Console Plugin — Operator Dashboard and RAR Conversational Interface

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-632-v1.0
**Feature**: Embed Kubernaut into the OpenShift Console as a native console plugin with a dashboard overview and conversational RAR interface
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the OCP Console Plugin introduced by Issue #632. The plugin provides operators with a dashboard showing remediation status, a conversational interface for RAR decision-making (consuming the #592 chat API), and action controls for approving/rejecting/overriding RARs (consuming the #594 override types).

### 1.2 Objectives

1. **Dashboard widgets**: Real-time data from CRD list/watch (RR, RAR, WE, EA) renders correctly.
2. **RAR detail view**: Structured context panel shows AIA summary, workflow, parameters, confidence, target resource.
3. **Chat interface**: Operator messages reach KA conversation API; SSE responses stream and render in real-time.
4. **Action controls**: Approve/Reject/Override buttons produce correct RAR status patches via K8s API.
5. **Plugin registration**: `ConsolePlugin` CR created; plugin loads in OCP Console.
6. **Auth delegation**: Plugin uses OCP session token; console proxy passes auth to K8s API and chat backend.
7. **Notification integration**: Pending RAR count in console badge.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `npm test -- --watchAll=false` |
| Integration test pass rate | 100% | Cypress component tests |
| Unit-testable code coverage | >=80% | Components, hooks, utilities |
| Visual regression | 0 regressions | Storybook snapshots |

---

## 2. References

### 2.1 Authority

- Issue #632: OCP Console Plugin — operator dashboard and RAR conversational interface
- Issue #592: Conversational RAR API backend (this plugin is the v1.4 client)
- Issue #594: RAR operator overrides (override UI elements depend on these types)
- OCP Console Plugin SDK: `@openshift-console/dynamic-plugin-sdk`
- Design Decision: Standalone plugin (not Lightspeed integration) — see #632 comments

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- OCP Console Plugin documentation
- [`openshift/lightspeed-console`](https://github.com/openshift/lightspeed-console) — SDK reference patterns (read-only, no code dependency)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | OCP Console SDK version incompatibility | Plugin fails to load | Low | IT-CP-632-001 | Pin SDK version; test with target OCP version |
| R2 | SSE proxy connection dropped | Chat response interrupted | Medium | UT-CP-632-015, IT-CP-632-006 | Reconnection with Last-Event-ID; display partial response |
| R3 | RBAC insufficient for CRD list/watch | Dashboard shows empty/error | Medium | UT-CP-632-004, IT-CP-632-003 | Check permissions on load; show RBAC guidance |
| R4 | Large RR/WE dataset causes browser OOM | Dashboard tab crashes | Low | UT-CP-632-005 | Pagination, virtual scroll, namespace filter |
| R5 | Chat backend unavailable | Chat panel shows error | Medium | UT-CP-632-016 | Graceful error state; retry with exponential backoff |
| R6 | Override UI submits malformed patch | RAR status corrupted | Medium | UT-CP-632-020, IT-CP-632-007 | Client-side validation; server (webhook) rejects |

### 3.1 Risk-to-Test Traceability

- **R1** (SDK compat): IT-CP-632-001
- **R2** (SSE proxy): UT-CP-632-015, IT-CP-632-006
- **R3** (RBAC): UT-CP-632-004, IT-CP-632-003
- **R6** (malformed patch): UT-CP-632-020, IT-CP-632-007

---

## 4. Scope

### 4.1 Features to be Tested

**Plugin Infrastructure**:
- `ConsolePlugin` CR registration
- Plugin webpack build and module federation
- Navigation integration (nav item under Kubernaut section)
- Console proxy configuration for K8s API and chat backend

**Dashboard Page** (`plugin/src/pages/DashboardPage/`):
- Active remediations by phase widget (RR status.phase counts)
- Pending RAR queue widget (RAR with AwaitingApproval)
- Recent WE outcomes widget (success/failure trend)
- Effectiveness summary widget (EA outcome counts, last 7d)
- Signal ingestion rate widget (RR creation rate)
- Namespace filter
- Auto-refresh via list/watch

**RAR Detail Page** (`plugin/src/pages/RARDetailPage/`):
- Structured context panel: RCA summary, selected workflow, params, confidence, target resource, history
- Chat panel: message input, SSE streaming, markdown rendering, message history
- Action controls: Approve, Reject (free-text reason), Override (workflow selector, param editor)

**Shared Components**:
- SSE client hook (`useSSEStream`)
- K8s resource hooks (list/watch via console SDK)
- Phase badge component
- Notification badge (pending RAR count)

### 4.2 Features Not to be Tested

- **Chat backend API** (#592): Separate issue — this plugin consumes it
- **Override CRD types/webhook** (#594): Separate issue — this plugin produces patches
- **Multi-cluster views** (#54, v1.5+): Future enhancement
- **Historical trend charts**: Future enhancement
- **Session persistence**: Backend concern (#592)
- **Real OCP OAuth**: Inherited from OCP session; no plugin-side auth code

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% — React component tests (React Testing Library), hook tests, utility tests
- **Integration**: >=80% — Cypress component tests with mocked K8s API and chat backend
- **Visual**: Storybook stories for all major components; snapshot tests
- **E2E**: Manual testing on real OCP cluster. Automated E2E deferred until OCP test environment available.

### 5.2 Pass/Fail Criteria

**PASS**: All unit tests pass; Cypress integration tests pass; dashboard renders all 5 widgets; chat SSE works; action controls produce correct patches; plugin loads in OCP.

**FAIL**: Any P0 test fails; action control produces malformed patch; plugin fails to register; SSE stream doesn't render.

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-UI-001 | Dashboard: active remediations by phase renders correct counts | P0 | Unit | UT-CP-632-001 | Pending |
| BR-UI-001 | Dashboard: pending RAR queue shows AwaitingApproval RARs | P0 | Unit | UT-CP-632-002 | Pending |
| BR-UI-001 | Dashboard: WE outcome trend renders chart | P1 | Unit | UT-CP-632-003 | Pending |
| BR-UI-001 | Dashboard: RBAC error → shows guidance message | P0 | Unit | UT-CP-632-004 | Pending |
| BR-UI-001 | Dashboard: namespace filter limits CRD scope | P1 | Unit | UT-CP-632-005 | Pending |
| BR-UI-001 | Dashboard: effectiveness summary widget | P1 | Unit | UT-CP-632-006 | Pending |
| BR-UI-002 | RAR detail: context panel shows AIA summary fields | P0 | Unit | UT-CP-632-007 | Pending |
| BR-UI-002 | RAR detail: context panel shows remediation history | P1 | Unit | UT-CP-632-008 | Pending |
| BR-UI-003 | Chat: message input sends to conversation API | P0 | Unit | UT-CP-632-009 | Pending |
| BR-UI-003 | Chat: SSE stream renders tokens incrementally | P0 | Unit | UT-CP-632-010 | Pending |
| BR-UI-003 | Chat: message history maintained across turns | P0 | Unit | UT-CP-632-011 | Pending |
| BR-UI-003 | Chat: markdown rendering in responses | P1 | Unit | UT-CP-632-012 | Pending |
| BR-UI-003 | Chat: session creation on first message | P0 | Unit | UT-CP-632-013 | Pending |
| BR-UI-003 | Chat: rate limit (429) → user-friendly message | P1 | Unit | UT-CP-632-014 | Pending |
| BR-UI-003 | Chat: SSE reconnection on dropped connection | P0 | Unit | UT-CP-632-015 | Pending |
| BR-UI-003 | Chat: backend unavailable → error state with retry | P0 | Unit | UT-CP-632-016 | Pending |
| BR-UI-004 | Action: Approve button → PATCH RAR status.decision=Approved | P0 | Unit | UT-CP-632-017 | Pending |
| BR-UI-004 | Action: Reject button → PATCH with reason | P0 | Unit | UT-CP-632-018 | Pending |
| BR-UI-004 | Action: Override → workflow selector + param editor → PATCH with WorkflowOverride | P0 | Unit | UT-CP-632-019 | Pending |
| BR-UI-004 | Action: Override → client-side validation (workflow required) | P0 | Unit | UT-CP-632-020 | Pending |
| BR-UI-004 | Action: RAR already decided → buttons disabled | P0 | Unit | UT-CP-632-021 | Pending |
| BR-UI-005 | Notification: pending RAR count badge in console nav | P1 | Unit | UT-CP-632-022 | Pending |
| BR-UI-001 | Integration: dashboard loads with mock CRD data | P0 | Integration | IT-CP-632-001 | Pending |
| BR-UI-002 | Integration: RAR detail loads with mock AIA/RR data | P0 | Integration | IT-CP-632-002 | Pending |
| BR-UI-001 | Integration: RBAC 403 → dashboard shows RBAC message | P0 | Integration | IT-CP-632-003 | Pending |
| BR-UI-003 | Integration: chat session lifecycle (create → message → stream) | P0 | Integration | IT-CP-632-004 | Pending |
| BR-UI-004 | Integration: approve action → correct K8s API patch | P0 | Integration | IT-CP-632-005 | Pending |
| BR-UI-003 | Integration: SSE drop → reconnection → stream resumes | P0 | Integration | IT-CP-632-006 | Pending |
| BR-UI-004 | Integration: override action → correct PATCH with WorkflowOverride | P0 | Integration | IT-CP-632-007 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests (22 tests)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-CP-632-001` | Dashboard: RR list with 3 Pending, 2 Executing, 1 Completed → chart shows correct counts | Pending |
| `UT-CP-632-002` | Dashboard: RAR queue shows 2 AwaitingApproval items with name, namespace, age | Pending |
| `UT-CP-632-003` | Dashboard: WE outcome chart renders success/failure bars from mock data | Pending |
| `UT-CP-632-004` | Dashboard: k8sListWatch returns 403 → "Insufficient permissions" message with RBAC hint | Pending |
| `UT-CP-632-005` | Dashboard: namespace filter changes → CRD watch restarted with new namespace | Pending |
| `UT-CP-632-006` | Dashboard: EA widget shows 80% effective, 10% ineffective, 10% inconclusive | Pending |
| `UT-CP-632-007` | RAR detail: context panel renders AIA rootCause, selectedWorkflow.name, params, confidence | Pending |
| `UT-CP-632-008` | RAR detail: history panel shows previous RR attempts for same resource | Pending |
| `UT-CP-632-009` | Chat: typing message + submit → POST to `/api/v1/conversations/{id}/messages` | Pending |
| `UT-CP-632-010` | Chat: `useSSEStream` hook receives 5 SSE events → renders 5 token chunks progressively | Pending |
| `UT-CP-632-011` | Chat: after 3 turns, all 6 messages (3 user + 3 assistant) visible in order | Pending |
| `UT-CP-632-012` | Chat: response with `**bold**` and `` `code` `` → rendered as styled markdown | Pending |
| `UT-CP-632-013` | Chat: first message → POST to create session → then POST message | Pending |
| `UT-CP-632-014` | Chat: 429 response → "Rate limit exceeded. Try again in X seconds." banner | Pending |
| `UT-CP-632-015` | Chat: SSE connection drops → reconnects with Last-Event-ID → stream continues | Pending |
| `UT-CP-632-016` | Chat: backend 503 → "Chat unavailable" error state with "Retry" button | Pending |
| `UT-CP-632-017` | Action: click Approve → confirmation dialog → PATCH `status.decision: Approved` | Pending |
| `UT-CP-632-018` | Action: click Reject → reason input → PATCH `status.decision: Rejected, decisionMessage: reason` | Pending |
| `UT-CP-632-019` | Action: click Override → select workflow from dropdown → edit params → PATCH with WorkflowOverride | Pending |
| `UT-CP-632-020` | Action: Override with empty workflow selector → submit disabled, "Select a workflow" validation | Pending |
| `UT-CP-632-021` | Action: RAR with decision=Approved → all action buttons disabled with "Already decided" tooltip | Pending |
| `UT-CP-632-022` | Notification: 3 pending RARs → badge shows "3" in console nav | Pending |

### Tier 2: Integration Tests (7 tests, Cypress)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-CP-632-001` | Dashboard page mounts; 5 widgets render with mock CRD data from intercepted API calls | Pending |
| `IT-CP-632-002` | RAR detail page mounts; context panel + chat panel render with mock data | Pending |
| `IT-CP-632-003` | Dashboard: K8s API returns 403 → RBAC guidance displayed | Pending |
| `IT-CP-632-004` | Chat: create session → send message → receive SSE tokens → full response rendered | Pending |
| `IT-CP-632-005` | Approve: click → confirm → K8s API PATCH intercepted with correct body | Pending |
| `IT-CP-632-006` | Chat: SSE connection severed → reconnection attempt → stream resumes (mock server) | Pending |
| `IT-CP-632-007` | Override: select workflow → set params → submit → PATCH intercepted with WorkflowOverride body | Pending |

### Tier Skip Rationale

- **E2E on real OCP**: Requires OCP cluster with Kubernaut deployed. Manual testing until automated OCP test env is available.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Jest + React Testing Library
- **Mocks**: `@openshift-console/dynamic-plugin-sdk` mock, MSW (Mock Service Worker) for K8s API + chat API
- **Location**: `plugin/src/**/__tests__/`

### 10.2 Integration Tests

- **Framework**: Cypress Component Testing
- **Mocks**: Cypress intercepts for K8s API and chat backend SSE
- **Location**: `plugin/cypress/component/`

### 10.3 Visual Tests

- **Framework**: Storybook + chromatic (optional)
- **Location**: `plugin/src/**/*.stories.tsx`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact | Workaround |
|------------|------|--------|--------|------------|
| #592 Conversation API | Backend | Planned (v1.4) | Chat panel needs API | Mock chat backend for frontend dev |
| #594 WorkflowOverride types | Backend | Planned (v1.4) | Override UI needs types | Define TypeScript types from #594 design spec |
| OCP Console Plugin SDK | External | Available | Plugin infrastructure | Pin known-good version |
| Kubernaut CRDs (RR, RAR, WE, EA, AIA) | Backend | Exists | Dashboard/detail data | Available via K8s API |
| RemediationWorkflow CRD (catalog) | Backend | Exists | Override workflow selector | Available via K8s API |

### 11.2 Execution Order

1. **Phase 1**: Plugin scaffolding + ConsolePlugin CR + build pipeline
2. **Phase 2**: Dashboard widgets (CRD list/watch)
3. **Phase 3**: RAR detail view (context panel)
4. **Phase 4**: Chat interface (SSE client, message rendering)
5. **Phase 5**: Action controls (approve/reject/override)
6. **Phase 6**: Notification badge
7. **Phase 7**: Integration tests + visual tests

---

## 12. Execution

```bash
# Unit tests
cd plugin && npm test -- --watchAll=false

# Cypress component tests
cd plugin && npx cypress run --component

# Storybook
cd plugin && npm run storybook
```

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
| 1.1 | 2026-03-04 | Add Lightspeed design decision reference; standalone plugin confirmed |
