# DD-CONSOLE-001: Intent-Driven Navigation Architecture for RHDH Console

**Date**: 2026-04-27  
**Status**: 📝 Draft  
**Target Version**: v1.5  
**Related**: #855, #856, DD-PLAYBOOK-010, #874 (MCP Interactive Flow)

---

## Proposal

Define the technical architecture for **intent-driven navigation** in the Kubernaut RHDH (Red Hat Developer Hub) plugin, where natural language input from the user controls the page layout, active view, filters, and rendered components — making the NL command bar the primary navigation mechanism.

---

## 1. Problem Statement

Traditional operational dashboards use fixed layouts with hierarchical navigation. Operators must know *where* to look before they can find information. At fleet scale (100+ clusters, dozens of active investigations), this creates friction:

- Navigating to the right tab, applying the right filters, and scrolling to the right row takes multiple clicks
- Context switching between views loses the operator's mental model
- Each view shows the same layout regardless of the operator's current question

**Intent-driven navigation** solves this by letting the operator *describe what they want to see*, and the UI adapts the layout to answer that question directly.

---

## 2. Architecture Overview

```
┌───────────────────────────────────────────────────────┐
│  RHDH Plugin (React + Backstage)                      │
│                                                       │
│  ┌─────────────────────────────────────────────────┐  │
│  │  NL Command Bar                                 │  │
│  │  "show me all critical alerts across prod"      │  │
│  └──────────────────────┬──────────────────────────┘  │
│                         │                             │
│                         ▼                             │
│  ┌──────────────────────────────────────────────────┐ │
│  │  Intent Router                                   │ │
│  │  ┌─────────┐  ┌───────────┐  ┌───────────────┐  │ │
│  │  │ Intent  │  │ Layout    │  │ Component     │  │ │
│  │  │ Parser  │→ │ Resolver  │→ │ Compositor    │  │ │
│  │  └─────────┘  └───────────┘  └───────────────┘  │ │
│  └──────────────────────┬───────────────────────────┘ │
│                         │                             │
│           ┌─────────────┼─────────────┐               │
│           ▼             ▼             ▼               │
│     ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│     │ Sidebar  │ │ Content  │ │ Response │           │
│     │ State    │ │ Area     │ │ Bubble   │           │
│     └──────────┘ └──────────┘ └──────────┘           │
└───────────────────────────────────────────────────────┘
                         │
                         │ MCP/SSE
                         ▼
┌───────────────────────────────────────────────────────┐
│  kubernaut-apifrontend (MCP Server)                   │
│  - Intent classification                              │
│  - Data retrieval (fleet, investigations, workflows)  │
│  - Navigation metadata in response                    │
└───────────────────────────────────────────────────────┘
```

---

## 3. Intent Taxonomy

### 3.1 Core Intents (v1.5 — 8 intents)

| Intent ID | Example NL Query | Target View | Layout |
|-----------|-----------------|-------------|--------|
| `fleet-overview` | "show me fleet status" | Fleet Overview | metrics + grid + alerts table |
| `fleet-overview-filtered` | "show me all critical alerts across production" | Fleet Overview | metrics + grid + **filtered** alerts table |
| `investigation-detail` | "investigate the pvc failure in prod-west-11" | Investigation Detail | RCA timeline + streaming log + evidence |
| `investigation-list` | "what investigations are running?" | Investigations | sortable investigation table |
| `workflow-catalog` | "show available workflows for OOM" | Workflows | filtered workflow catalog |
| `workflow-execution` | "what workflow ran for the etcd issue?" | Workflow Execution Detail | execution timeline + parameters + outcome |
| `cluster-detail` | "show me prod-east-14 health" | Cluster Detail | cluster metrics + active investigations + capacity |
| `comparison` | "compare resolution times prod-east vs prod-west" | Comparison View | side-by-side metrics / time-series |

### 3.2 Intent Resolution Rules

```
Priority order for ambiguous queries:
1. Explicit entity reference → entity-specific view
   "prod-west-11" → cluster-detail OR investigation-detail (depends on context)
2. Action verb → action-appropriate view
   "investigate" → investigation-detail
   "approve"    → investigation-detail (approval tab)
   "show"       → read-only view
   "compare"    → comparison view
3. Scope modifier → filtered parent view
   "all critical" → fleet-overview-filtered
   "across production" → fleet-overview-filtered
4. Default → fleet-overview
```

### 3.3 Future Intents (v2.0+)

| Intent ID | Example | Target View |
|-----------|---------|-------------|
| `sla-trend` | "show me SLA trend this week" | Time-series chart view |
| `policy-audit` | "who approved the last 5 remediations?" | Policy audit trail |
| `capacity-forecast` | "which clusters will hit PVC limits this month?" | Capacity planning |
| `drill-down` | "zoom into that etcd alert" | Context-dependent detail |

---

## 4. MCP Response Schema

### 4.1 Navigation-Enriched Response

The MCP server returns a standard response body augmented with navigation metadata. The `navigation` field is the key addition that enables intent-driven layout.

```json
{
  "session_id": "sess_abc123",
  "response": {
    "summary": "Found 7 critical alerts across 5 production clusters.",
    "detail": "3 active investigations · 2 auto-resolved · 1 escalated · 1 pending approval",
    "severity": "critical",
    "actions": [
      {
        "label": "View full RCA for escalated",
        "intent": "investigation-detail",
        "params": { "cluster": "prod-west-11", "alert": "pvc-capacity-95pct" }
      }
    ]
  },
  "navigation": {
    "intent": "fleet-overview-filtered",
    "view": "fleet-overview",
    "filters": {
      "severity": ["critical"],
      "environment": ["production"]
    },
    "layout": "split-metrics-table",
    "sidebar_active": "fleet-overview",
    "breadcrumb": ["Kubernaut", "Fleet Overview", "Critical · Production"]
  },
  "data": {
    "metrics": {
      "active_investigations": 8,
      "resolved_24h": 247,
      "critical_alerts": 7,
      "critical_clusters": 5,
      "avg_resolution_minutes": 2.68,
      "auto_resolution_pct": 94
    },
    "clusters": [
      { "name": "prod-east-01", "status": "ok" },
      { "name": "prod-east-07", "status": "critical" }
    ],
    "alerts": [
      {
        "name": "payments-oom-crash",
        "cluster": "prod-east-14",
        "severity": "critical",
        "status": "running",
        "duration_seconds": 108,
        "workflow": "rollback-deploy",
        "root_cause": "OOM from unbounded cache",
        "investigation_id": "inv-a1b2c3"
      }
    ]
  },
  "followups": [
    {
      "label": "investigate the pvc failure in prod-west-11",
      "intent": "investigation-detail",
      "params": { "cluster": "prod-west-11", "alert": "pvc-capacity-95pct" },
      "category": "drill-down"
    },
    {
      "label": "approve the cert rotation",
      "intent": "investigation-detail",
      "params": { "investigation_id": "inv-x7y8z9", "tab": "approval" },
      "category": "action"
    },
    {
      "label": "show me SLA trend this week",
      "intent": "sla-trend",
      "params": { "range": "7d" },
      "category": "analysis"
    }
  ]
}
```

### 4.2 Field Descriptions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `navigation.intent` | string | yes | Classified intent from the taxonomy |
| `navigation.view` | string | yes | Target React route/view component |
| `navigation.filters` | object | no | Key-value filters to apply to the view's data |
| `navigation.layout` | string | yes | Layout variant for the target view |
| `navigation.sidebar_active` | string | yes | Which sidebar item to highlight |
| `navigation.breadcrumb` | string[] | yes | Breadcrumb trail reflecting NL context |
| `followups[].intent` | string | yes | Intent to trigger when the follow-up is selected |
| `followups[].params` | object | yes | Parameters to pass to the intent handler |
| `followups[].category` | string | yes | `drill-down`, `action`, `analysis`, `compare` |

### 4.3 Layout Variants

| Layout ID | Components | Used By |
|-----------|-----------|---------|
| `split-metrics-table` | Metric cards (top) + data table (bottom) | fleet-overview, fleet-overview-filtered |
| `split-metrics-grid-table` | Metric cards + cluster grid + data table | fleet-overview (full) |
| `timeline-detail` | RCA timeline (left) + evidence panel (right) | investigation-detail |
| `catalog-grid` | Filter sidebar + workflow cards | workflow-catalog |
| `side-by-side` | Two metric panels + comparison chart | comparison |
| `single-entity` | Entity header + tabbed detail | cluster-detail, workflow-execution |

---

## 5. Frontend Architecture

### 5.1 Intent Router (React)

The Intent Router is a React context provider + reducer that manages the current navigation state derived from MCP responses.

```typescript
interface NavigationState {
  intent: string;
  view: string;
  filters: Record<string, string[]>;
  layout: string;
  sidebarActive: string;
  breadcrumb: string[];
  data: Record<string, unknown>;
  followups: Followup[];
  conversationHistory: ConversationEntry[];
}

type NavigationAction =
  | { type: 'NL_RESPONSE'; payload: MCPResponse }
  | { type: 'FOLLOWUP_SELECTED'; payload: Followup }
  | { type: 'SIDEBAR_NAV'; payload: string }
  | { type: 'CLEAR_CONVERSATION' };
```

### 5.2 View Routing

The view router maps `navigation.view` to React component trees:

```typescript
const VIEW_REGISTRY: Record<string, React.ComponentType<ViewProps>> = {
  'fleet-overview':        FleetOverviewView,
  'investigation-detail':  InvestigationDetailView,
  'investigation-list':    InvestigationListView,
  'workflow-catalog':      WorkflowCatalogView,
  'workflow-execution':    WorkflowExecutionView,
  'cluster-detail':        ClusterDetailView,
  'comparison':            ComparisonView,
};
```

### 5.3 Layout Compositor

Each view accepts a `layout` prop and renders the appropriate component arrangement:

```typescript
const FleetOverviewView: React.FC<ViewProps> = ({ layout, data, filters }) => {
  switch (layout) {
    case 'split-metrics-table':
      return (
        <>
          <MetricCards data={data.metrics} />
          <AlertTable data={data.alerts} filters={filters} />
        </>
      );
    case 'split-metrics-grid-table':
      return (
        <>
          <MetricCards data={data.metrics} />
          <ClusterGrid data={data.clusters} />
          <AlertTable data={data.alerts} filters={filters} />
        </>
      );
    default:
      return <FleetOverviewDefault data={data} />;
  }
};
```

### 5.4 Conversation State Management

The NL conversation history persists across view transitions, allowing the user to scroll back through prior queries and responses:

```typescript
interface ConversationEntry {
  id: string;
  query: string;
  response: MCPResponse['response'];
  navigation: MCPResponse['navigation'];
  followups: Followup[];
  timestamp: number;
}
```

---

## 6. Backend: kubernaut-apifrontend Changes

### 6.1 Intent Classification

The MCP server adds an intent classification step before data retrieval:

```
NL Query → LLM Intent Classifier → (intent, params, filters) → Data Retrieval → Response Assembly
```

The intent classifier can be:
- **v1.5**: Rule-based with keyword/entity extraction (fast, deterministic)
- **v2.0**: LLM-powered classification with fine-tuned prompt (flexible, handles ambiguity)

### 6.2 Rule-Based Classifier (v1.5)

```go
type IntentClassification struct {
    Intent  string            `json:"intent"`
    View    string            `json:"view"`
    Filters map[string][]string `json:"filters"`
    Params  map[string]string `json:"params"`
    Layout  string            `json:"layout"`
}

func ClassifyIntent(query string, sessionContext *SessionContext) *IntentClassification {
    tokens := tokenize(query)

    // Entity extraction
    clusters := extractClusters(tokens)
    alerts := extractAlerts(tokens)
    severities := extractSeverities(tokens)
    environments := extractEnvironments(tokens)

    // Intent matching (priority order)
    switch {
    case containsVerb(tokens, "investigate") && len(alerts) > 0:
        return &IntentClassification{
            Intent: "investigation-detail",
            View:   "investigation-detail",
            Params: map[string]string{"alert": alerts[0], "cluster": clusters[0]},
            Layout: "timeline-detail",
        }
    case containsVerb(tokens, "compare"):
        return &IntentClassification{
            Intent: "comparison",
            View:   "comparison",
            Params: extractComparisonTargets(tokens),
            Layout: "side-by-side",
        }
    case len(severities) > 0 || len(environments) > 0:
        return &IntentClassification{
            Intent:  "fleet-overview-filtered",
            View:    "fleet-overview",
            Filters: map[string][]string{
                "severity":    severities,
                "environment": environments,
            },
            Layout: "split-metrics-table",
        }
    case len(clusters) == 1:
        return &IntentClassification{
            Intent: "cluster-detail",
            View:   "cluster-detail",
            Params: map[string]string{"cluster": clusters[0]},
            Layout: "single-entity",
        }
    default:
        return &IntentClassification{
            Intent: "fleet-overview",
            View:   "fleet-overview",
            Layout: "split-metrics-grid-table",
        }
    }
}
```

### 6.3 Follow-up Generation

Follow-ups are generated based on the current response data, not hardcoded:

```go
func GenerateFollowups(intent *IntentClassification, data *ResponseData) []Followup {
    var followups []Followup

    // If there are failed/escalated items, suggest drill-down
    for _, alert := range data.Alerts {
        if alert.Status == "failed" || alert.Status == "escalated" {
            followups = append(followups, Followup{
                Label:    fmt.Sprintf("investigate the %s failure in %s", alert.Name, alert.Cluster),
                Intent:   "investigation-detail",
                Params:   map[string]string{"alert": alert.Name, "cluster": alert.Cluster},
                Category: "drill-down",
            })
        }
    }

    // If there are pending approvals, suggest action
    for _, alert := range data.Alerts {
        if alert.Status == "pending-approval" {
            followups = append(followups, Followup{
                Label:    fmt.Sprintf("approve the %s", alert.Workflow),
                Intent:   "investigation-detail",
                Params:   map[string]string{"investigation_id": alert.InvestigationID, "tab": "approval"},
                Category: "action",
            })
        }
    }

    // Always offer an analysis follow-up
    followups = append(followups, Followup{
        Label:    "show me SLA trend this week",
        Intent:   "sla-trend",
        Params:   map[string]string{"range": "7d"},
        Category: "analysis",
    })

    return capped(followups, 3)
}
```

---

## 7. Graceful Degradation

When the AI backend is unavailable, the console must remain functional:

| Scenario | Behavior |
|----------|----------|
| MCP server down | NL bar shows "AI assistant unavailable" badge; sidebar navigation works normally |
| Intent classification fails | Fall back to default fleet-overview layout |
| Partial response (no navigation field) | Use current view, render response bubble only |
| Streaming interrupted | Show partial response with "connection lost" indicator; retry button |

---

## 8. Implementation Phases

### Phase 1: Foundation (Sprint 1-2)
- [ ] Backstage plugin scaffold with 5 route-based views
- [ ] NL command bar component (UI only, no AI integration)
- [ ] Static layout compositor with all 6 layout variants
- [ ] Sidebar with reactive active-state management

### Phase 2: MCP Integration (Sprint 3-4)
- [ ] Connect NL command bar to kubernaut-apifrontend MCP endpoint
- [ ] Implement rule-based intent classifier in Go
- [ ] Add `navigation` field to MCP response schema
- [ ] Wire intent router to view/layout switching
- [ ] Implement response bubble rendering with severity indicators

### Phase 3: Follow-ups & Conversation (Sprint 5-6)
- [ ] Dynamic follow-up generation from response data
- [ ] Follow-up pill click → intent dispatch → view navigation
- [ ] Conversation history persistence across view transitions
- [ ] Breadcrumb updates from NL context

### Phase 4: Polish & Degradation (Sprint 7)
- [ ] Graceful degradation when AI backend unavailable
- [ ] Loading states and streaming response rendering
- [ ] Keyboard shortcuts (Cmd+K for command bar focus)
- [ ] Accessibility audit (screen reader support for NL responses)

---

## 9. Testing Strategy

| Layer | Approach |
|-------|----------|
| Intent classifier | Unit tests with table-driven NL inputs → expected intents (target: 50+ test cases) |
| MCP response schema | Contract tests validating navigation field presence and structure |
| View routing | Component tests: given a navigation state, correct view renders |
| Layout compositor | Snapshot tests for each layout variant with mock data |
| Follow-up generation | Unit tests: given response data, correct follow-ups generated |
| E2E | Cypress tests: type NL query → verify view switch + data rendering |
| Degradation | Integration tests: kill MCP server → verify fallback behavior |

---

## 10. Security Considerations

- NL input must be sanitized before sending to MCP server (prevent prompt injection)
- Follow-up params must be validated server-side (don't trust client-provided intent params)
- RBAC: NL queries should respect the user's Backstage permissions — if a user can't see cluster X, queries about cluster X return "insufficient permissions"
- Audit: all NL queries and resulting navigations should be logged for operational audit

---

## 11. Open Questions

1. **Session continuity**: Should conversation history persist across browser sessions (localStorage) or be ephemeral?
2. **Multi-user**: If two operators are looking at the same fleet, should they see each other's active investigations in real-time?
3. **LLM classifier timing**: For v2.0, is the latency of an LLM-powered intent classifier acceptable (~200-500ms) or should we keep rule-based?
4. **Backstage theming**: Should the NL command bar follow PatternFly's dark header or Backstage's light theme?

---

## References

- Slide A5: Kubernaut Console — RHDH Plugin (15-rhdh-console.svg)
- #855: Enhancement Proposal: Kubernaut Console — Backstage Plugin
- #856: feat(console): NL-first interaction model with intent-driven navigation
- #874: v1.5: Agentic Integration — End-to-End MCP Interactive Flow
- DD-PLAYBOOK-010: MCP Playbook Catalog Integration
