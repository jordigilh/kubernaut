# Implementation Plan: OCP Console Plugin — Operator Dashboard and RAR Conversational Interface

**Issue**: #632
**Test Plan**: [TP-632-v1.0](TEST_PLAN.md)
**Branch**: `development/v1.4`
**Created**: 2026-03-04

---

## Overview

This plan implements an OpenShift Console Plugin that embeds Kubernaut into the operator console with a dashboard overview and a conversational RAR decision-making interface. The plugin is a React SPA using the `@openshift-console/dynamic-plugin-sdk`, served by a lightweight nginx pod, and registered via a `ConsolePlugin` CR.

### Design Decision: Standalone Plugin (not Lightspeed)

**Decision**: Build a standalone Kubernaut console plugin. Coexists with OpenShift Lightspeed — each serves a different purpose.

Lightspeed is a general-purpose, stateless, read-only OCP/K8s assistant. Kubernaut needs RAR-scoped conversations with audit-seeded investigation context, actionable controls (approve/reject/override), a remediation dashboard, and persistent audit trail. Integrating with Lightspeed would mean fighting its architecture.

| Concern | Lightspeed | Kubernaut Plugin |
|---------|-----------|-----------------|
| Conversation scope | General OCP questions | Specific RAR + investigation audit trail |
| LLM context | Starts fresh | Audit chain reconstruction from #592 API |
| Actions | Read-only | Approve, reject, override (CRD patches) |
| Dashboard | None | RR/RAR/WE/EA metrics |
| Backend | Lightspeed service (Python) | KA conversation API (#592, Go) |

**Reference implementation**: [`openshift/lightspeed-console`](https://github.com/openshift/lightspeed-console) (Apache 2.0) used as read-only reference for OCP Dynamic Plugin SDK patterns — plugin scaffolding, `ConsolePlugin` CR registration, SSE consumption in React, console proxy configuration. No code dependency, no runtime integration, no shared components. Estimated 2-3 days saved on initial SDK ramp-up (reflected in Phase 1 estimate).

**Chat backend**: KA conversation API (#592, Option A) — KA already has the LLM client, K8s toolset, audit context. The console proxies chat requests via `ConsolePlugin.spec.proxy`.

### Architecture

```
┌─────────────────────────────────────────────┐
│            OpenShift Console                  │
│                                               │
│  ┌─────────────────────────────────────────┐  │
│  │        Kubernaut Plugin (React)          │  │
│  │                                          │  │
│  │  ┌──────────┐  ┌─────────────────────┐   │  │
│  │  │Dashboard │  │  RAR Detail + Chat  │   │  │
│  │  │  Page    │  │                     │   │  │
│  │  │          │  │  Context  │  Chat   │   │  │
│  │  │ Widgets  │  │  Panel    │  Panel  │   │  │
│  │  └────┬─────┘  └──┬───────┴────┬────┘   │  │
│  └───────┼────────────┼────────────┼────────┘  │
│          │            │            │            │
│    ┌─────▼────────────▼─┐    ┌─────▼────────┐  │
│    │  Console Proxy     │    │ Console Proxy │  │
│    │  (K8s API)         │    │ (Chat Backend)│  │
│    └─────┬──────────────┘    └──────┬────────┘  │
└──────────┼──────────────────────────┼───────────┘
           │                          │
    ┌──────▼──────┐           ┌───────▼───────┐
    │  K8s API    │           │ KA Chat API   │
    │  (CRDs)     │           │ (#592)        │
    └─────────────┘           └───────────────┘
```

### Technology Stack

| Layer | Technology |
|-------|-----------|
| Framework | React 18 + TypeScript |
| Plugin SDK | `@openshift-console/dynamic-plugin-sdk` |
| Build | Webpack 5 (module federation) |
| Styling | PatternFly 5 (OCP design system) |
| Charts | PatternFly Charts (Victory-based) |
| Markdown | `react-markdown` + `remark-gfm` |
| Testing | Jest + React Testing Library + Cypress |
| Serving | nginx alpine container |

### Plugin Location

```
plugin/
├── package.json
├── tsconfig.json
├── webpack.config.ts
├── console-extensions.json        # OCP plugin extension points
├── src/
│   ├── index.ts                   # Plugin entry
│   ├── pages/
│   │   ├── DashboardPage/
│   │   │   ├── DashboardPage.tsx
│   │   │   ├── widgets/
│   │   │   │   ├── ActiveRemediationsWidget.tsx
│   │   │   │   ├── PendingRARQueueWidget.tsx
│   │   │   │   ├── WEOutcomeTrendWidget.tsx
│   │   │   │   ├── EffectivenessWidget.tsx
│   │   │   │   └── SignalRateWidget.tsx
│   │   │   └── __tests__/
│   │   └── RARDetailPage/
│   │       ├── RARDetailPage.tsx
│   │       ├── ContextPanel.tsx
│   │       ├── ChatPanel.tsx
│   │       ├── ActionControls.tsx
│   │       ├── OverrideModal.tsx
│   │       └── __tests__/
│   ├── hooks/
│   │   ├── useSSEStream.ts
│   │   ├── useCRDWatch.ts
│   │   ├── useConversation.ts
│   │   └── __tests__/
│   ├── components/
│   │   ├── PhaseBadge.tsx
│   │   ├── ConfidenceIndicator.tsx
│   │   ├── NotificationBadge.tsx
│   │   └── __tests__/
│   ├── types/
│   │   ├── crd.ts                 # RR, RAR, WE, EA, AIA, RW TypeScript types
│   │   └── conversation.ts        # Chat session, message types
│   └── utils/
│       ├── k8sPatches.ts          # RAR status patch builders
│       └── sseClient.ts           # EventSource wrapper with reconnection
├── cypress/
│   └── component/
└── Dockerfile                     # nginx serving built plugin
```

---

## Phase 1: Plugin Scaffolding + Build Pipeline (Day 1)

**SDK ramp-up accelerated** by [`openshift/lightspeed-console`](https://github.com/openshift/lightspeed-console) reference patterns for: plugin webpack config, `ConsolePlugin` CR, SSE consumption, console proxy setup.

### Phase 1.1: Project setup

- Initialize `plugin/package.json` with:
  - `@openshift-console/dynamic-plugin-sdk`
  - `@patternfly/react-core`, `@patternfly/react-charts`
  - `react`, `react-dom`, `react-router-dom`
  - `webpack`, `ts-loader`, `@openshift-console/plugin-webpack`
  - `jest`, `@testing-library/react`, `cypress`
- `tsconfig.json` with strict mode
- `webpack.config.ts` with module federation (`@openshift-console/plugin-webpack`) — reference Lightspeed's webpack config for SDK-specific patterns
- `console-extensions.json` declaring:
  - `console.page/route` for `/kubernaut/dashboard` and `/kubernaut/rar/:name`
  - `console.navigation/section` for "Kubernaut"
  - `console.navigation/href` for dashboard and RAR list

### Phase 1.2: ConsolePlugin CR and Helm integration

**Files**:
- `charts/kubernaut/templates/console-plugin/consoleplugin.yaml` — `ConsolePlugin` CR
- `charts/kubernaut/templates/console-plugin/deployment.yaml` — nginx pod serving built plugin
- `charts/kubernaut/templates/console-plugin/service.yaml` — ClusterIP service

The `ConsolePlugin` CR registers the plugin with OCP:
```yaml
apiVersion: console.openshift.io/v1
kind: ConsolePlugin
metadata:
  name: kubernaut-console-plugin
spec:
  displayName: Kubernaut
  backend:
    type: Service
    service:
      name: kubernaut-console-plugin
      namespace: {{ .Release.Namespace }}
      port: 9443
  proxy:
    - alias: chat-backend
      endpoint:
        type: Service
        service:
          name: kubernaut-agent
          namespace: {{ .Release.Namespace }}
          port: 8443
      authorization: UserToken
```

Helm values:
```yaml
consolePlugin:
  enabled: false
  image:
    repository: ""
    tag: ""
  replicas: 1
  resources:
    limits:
      memory: 128Mi
      cpu: 100m
```

### Phase 1.3: Dockerfile

```dockerfile
FROM node:18-alpine AS build
WORKDIR /app
COPY plugin/ .
RUN npm ci && npm run build

FROM nginx:1.25-alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY plugin/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 9443
```

### Phase 1 Checkpoint

- [ ] `npm run build` produces module-federated bundle
- [ ] `ConsolePlugin` CR template renders correctly
- [ ] Helm lint passes

---

## Phase 2: TDD RED + GREEN — Dashboard Page (Days 3-5)

### Phase 2.1: TypeScript CRD types

**File**: `plugin/src/types/crd.ts`

Define TypeScript interfaces mirroring the Go CRD types for: `RemediationRequest`, `RemediationApprovalRequest` (including `WorkflowOverride`), `WorkflowExecution`, `EffectivenessAssessment`, `AIAnalysis`, `RemediationWorkflow`.

### Phase 2.2: Dashboard widget tests (RED)

**Files**: `plugin/src/pages/DashboardPage/__tests__/`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-CP-632-001 | ActiveRemediationsWidget renders correct phase counts | Component doesn't exist |
| UT-CP-632-002 | PendingRARQueueWidget shows AwaitingApproval items | Same |
| UT-CP-632-003 | WEOutcomeTrendWidget renders chart | Same |
| UT-CP-632-004 | Dashboard shows RBAC error message on 403 | Same |
| UT-CP-632-005 | Namespace filter changes → watch restarts | Same |
| UT-CP-632-006 | EffectivenessWidget renders percentage breakdown | Same |

### Phase 2.3: Dashboard implementation (GREEN)

- `DashboardPage.tsx`: Layout with PatternFly Grid, namespace filter, 5 widget slots
- `ActiveRemediationsWidget.tsx`: `useK8sWatchResource` for RR list → group by `status.overallPhase` → PatternFly donut chart
- `PendingRARQueueWidget.tsx`: `useK8sWatchResource` for RAR list → filter `status.decision == ""` → PatternFly table with name, namespace, age, recommended workflow
- `WEOutcomeTrendWidget.tsx`: WE list → group by `status.phase` (Completed/Failed) over time → bar chart
- `EffectivenessWidget.tsx`: EA list → count outcomes → donut chart
- `SignalRateWidget.tsx`: RR creation timestamps → compute rate → sparkline

### Phase 2 Checkpoint

- [ ] UT-CP-632-001 through -006 pass
- [ ] Dashboard renders with mock data
- [ ] `npm run build` succeeds

---

## Phase 3: TDD RED + GREEN — RAR Detail Page (Days 5-7)

### Phase 3.1: Context panel tests (RED)

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-CP-632-007 | Context panel renders AIA rootCause, workflow, params, confidence | Component doesn't exist |
| UT-CP-632-008 | History panel shows previous RR attempts | Same |

### Phase 3.2: Context panel implementation (GREEN)

- `ContextPanel.tsx`: Reads RAR spec → fetches AIA by ref → fetches RR by ref → renders structured view
  - RCA summary from `ai.status.rootCauseAnalysis`
  - Selected workflow from `ai.status.selectedWorkflow`
  - Parameters as key-value table
  - Confidence with color-coded indicator
  - Target resource
  - Previous RR attempts (list RRs with same target resource)

### Phase 3 Checkpoint

- [ ] UT-CP-632-007, -008 pass
- [ ] Context panel renders with mock CRD data

---

## Phase 4: TDD RED + GREEN — Chat Interface (Days 7-10)

### Phase 4.1: SSE hook and chat tests (RED)

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-CP-632-009 | Chat input submit → POST to conversation API | Hook doesn't exist |
| UT-CP-632-010 | useSSEStream renders tokens progressively | Same |
| UT-CP-632-011 | Message history maintained across turns | Same |
| UT-CP-632-012 | Markdown in responses rendered | Same |
| UT-CP-632-013 | First message creates session then sends | Same |
| UT-CP-632-014 | 429 response → rate limit banner | Same |
| UT-CP-632-015 | SSE reconnection on drop | Same |
| UT-CP-632-016 | Backend unavailable → error state | Same |

### Phase 4.2: Chat implementation (GREEN)

- `useSSEStream.ts`: Custom hook wrapping `EventSource`
  - Opens SSE connection to `/api/proxy/plugin/kubernaut-console-plugin/chat-backend/api/v1/conversations/{id}/messages`
  - Handles `Last-Event-ID` for reconnection
  - Returns `{data, isStreaming, error, reconnect}`
- `useConversation.ts`: Manages session lifecycle
  - `createSession(rarName, namespace)` → POST to conversation API
  - `sendMessage(sessionId, message)` → POST, open SSE for response
  - Returns `{messages, isLoading, sendMessage, error}`
- `ChatPanel.tsx`: PatternFly-based chat UI
  - Message list with user/assistant role styling
  - Input with send button
  - Streaming indicator
  - Markdown rendering via `react-markdown`
  - Rate limit and error states

### Phase 4 Checkpoint

- [ ] UT-CP-632-009 through -016 pass
- [ ] Chat renders with mock SSE data

---

## Phase 5: TDD RED + GREEN — Action Controls (Days 10-13)

### Phase 5.1: Action tests (RED)

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-CP-632-017 | Approve → PATCH RAR status | Component doesn't exist |
| UT-CP-632-018 | Reject → PATCH with reason | Same |
| UT-CP-632-019 | Override → workflow selector + param editor → PATCH with WorkflowOverride | Same |
| UT-CP-632-020 | Override client-side validation | Same |
| UT-CP-632-021 | Already decided → buttons disabled | Same |

### Phase 5.2: Action implementation (GREEN)

- `ActionControls.tsx`: Three PatternFly buttons (Approve/Reject/Override)
  - Approve: Confirmation modal → `k8sPatch` on RAR status with `decision: "Approved"`
  - Reject: Modal with reason textarea → `k8sPatch` with `decision: "Rejected", decisionMessage: reason`
  - Override: Opens `OverrideModal`
- `OverrideModal.tsx`: Full override form
  - Workflow selector: `useK8sWatchResource` for RW list → dropdown filtered by `status.catalogStatus == Ready`
  - Parameter editor: Dynamic key-value editor pre-populated from AIA params
  - Rationale textarea
  - Client-side validation: workflow required if changing from AI recommendation
  - Submit: `k8sPatch` with `decision: "Approved", workflowOverride: {workflowName, parameters, rationale}`
- `k8sPatches.ts`: Patch builder functions
  - `buildApprovePatch()`: `{status: {decision: "Approved"}}`
  - `buildRejectPatch(reason)`: `{status: {decision: "Rejected", decisionMessage: reason}}`
  - `buildOverridePatch(override)`: `{status: {decision: "Approved", workflowOverride: {...}}}`

### Phase 5 Checkpoint

- [ ] UT-CP-632-017 through -021 pass
- [ ] Action controls produce correct K8s patches

---

## Phase 6: TDD RED + GREEN — Notification Badge (Day 13)

### Phase 6.1: Notification tests (RED)

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-CP-632-022 | Pending RAR count renders as badge | Component doesn't exist |

### Phase 6.2: Notification implementation (GREEN)

- `NotificationBadge.tsx`: Uses `useK8sWatchResource` for RAR list → count pending → PatternFly badge
- Registered via `console-extensions.json` as `console.navigation/href` with dynamic badge

### Phase 6 Checkpoint

- [ ] UT-CP-632-022 passes

---

## Phase 7: Integration Tests + Visual Tests (Days 14-16)

### Phase 7.1: Cypress component tests

| Test ID | What it asserts |
|---------|----------------|
| IT-CP-632-001 | Dashboard page: all 5 widgets render with intercepted CRD data |
| IT-CP-632-002 | RAR detail page: context + chat panels render with mock data |
| IT-CP-632-003 | Dashboard: 403 → RBAC message displayed |
| IT-CP-632-004 | Chat: create session → send → receive SSE → rendered |
| IT-CP-632-005 | Approve: click → confirm → intercepted PATCH body correct |
| IT-CP-632-006 | Chat: SSE drop → reconnect → resume |
| IT-CP-632-007 | Override: select workflow → params → intercepted PATCH with WorkflowOverride |

### Phase 7.2: Storybook stories

Create stories for all major components:
- `DashboardPage` (with mock data)
- `PendingRARQueueWidget` (empty, with items)
- `ContextPanel` (with AIA data)
- `ChatPanel` (empty, with messages, streaming)
- `ActionControls` (pending, decided)
- `OverrideModal` (with workflow list)

### Phase 7 Checkpoint

- [ ] All 7 Cypress tests pass
- [ ] Storybook builds and renders all stories

---

## Phase 8: TDD REFACTOR — Code Quality (Days 16-17)

### Phase 8.1: Performance optimization

- Virtual scroll for large CRD lists
- Debounce namespace filter
- Memoize computed widget data
- Lazy-load RAR detail page

### Phase 8.2: Accessibility

- ARIA labels on all interactive elements
- Keyboard navigation for chat, actions, modal
- Screen reader support for live regions (SSE streaming)

### Phase 8.3: Error boundaries

- React error boundaries per widget (dashboard doesn't crash on single widget failure)
- Chat error boundary with retry

### Phase 8 Checkpoint

- [ ] All tests pass
- [ ] Lighthouse accessibility score >=90
- [ ] No console errors

---

## Phase 9: Due Diligence & Commit (Days 17-18)

### Phase 9.1: Comprehensive audit

- [ ] Plugin loads in OCP Console (manual verification)
- [ ] Dashboard renders all 5 widgets with real CRD data
- [ ] Chat session lifecycle works end-to-end with KA
- [ ] Action controls produce correct RAR patches
- [ ] Override modal validates and submits correctly
- [ ] SSE reconnection works through console proxy
- [ ] No sensitive data leaked in client-side code
- [ ] Helm chart includes plugin deployment (opt-in)

### Phase 9.2: Commit in logical groups

| Commit # | Scope |
|----------|-------|
| 1 | `feat(#632): OCP console plugin scaffolding + webpack + ConsolePlugin CR` |
| 2 | `feat(#632): TypeScript CRD types for RR, RAR, WE, EA, AIA, RW` |
| 3 | `test(#632): TDD RED — failing dashboard widget tests` |
| 4 | `feat(#632): dashboard page with 5 CRD-powered widgets` |
| 5 | `feat(#632): RAR detail page with structured context panel` |
| 6 | `test(#632): TDD RED — failing chat interface tests` |
| 7 | `feat(#632): chat panel with SSE streaming and markdown rendering` |
| 8 | `feat(#632): useSSEStream and useConversation hooks` |
| 9 | `feat(#632): action controls (approve/reject/override) with validation` |
| 10 | `feat(#632): override modal with workflow selector and param editor` |
| 11 | `feat(#632): notification badge for pending RARs` |
| 12 | `test(#632): Cypress integration tests + Storybook stories` |
| 13 | `feat(#632): Helm chart templates for plugin deployment` |
| 14 | `refactor(#632): performance, accessibility, error boundaries` |

---

## Estimated Effort

| Phase | Effort |
|-------|--------|
| Phase 1 (Scaffolding + Build) | 1 day (accelerated by Lightspeed SDK reference) |
| Phase 2 (Dashboard) | 2.5 days |
| Phase 3 (RAR Detail) | 2 days |
| Phase 4 (Chat Interface) | 2 days (SSE patterns from Lightspeed reference) |
| Phase 5 (Action Controls) | 3 days |
| Phase 6 (Notification) | 0.5 day |
| Phase 7 (Integration + Visual) | 2.5 days |
| Phase 8 (REFACTOR) | 1.5 days |
| Phase 9 (Due Diligence) | 1 day |
| **Total** | **16 days** (reduced from 18d — Lightspeed reference saves ~2d) |

---

## Open Questions (from #632)

1. ~~**Chat backend**: Option A (KA endpoint) vs Option B (dedicated service).~~ **RESOLVED**: Option A — KA conversation API (#592). Confirmed in design decision comment.
2. **Chat session persistence**: Should chat history be persisted (audit trail) or ephemeral? This plan assumes ephemeral per #592 scope.
3. **Notification triggers**: This plan covers pending RAR badge only. WE failures, EA low-effectiveness could be added later.
4. **Plugin naming**: This plan uses `kubernaut-console-plugin`.

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial implementation plan |
| 1.1 | 2026-03-04 | Add Lightspeed design decision (standalone plugin, coexistence). Reference `lightspeed-console` for SDK patterns. Resolve chat backend as Option A (KA). Effort reduced from 18d to 16d. |
