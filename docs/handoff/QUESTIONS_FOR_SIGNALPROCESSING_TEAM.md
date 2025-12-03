# Questions from HolmesGPT-API Team

**From**: HolmesGPT-API Team
**To**: SignalProcessing Controller Team
**Date**: December 1, 2025
**Re**: custom_labels Passthrough and RCA Context

---

## Context

The HolmesGPT-API team has implemented `custom_labels` pass-through from SignalProcessing → HolmesGPT-API → Data Storage as documented in `DD-HAPI-001-custom-labels-auto-append.md`. This enables customer-defined workflow filtering.

---

## Questions

### Q1: custom_labels Source

**Current Understanding**: SignalProcessing extracts `custom_labels` from:
1. Alertmanager webhook payload (external labels)
2. SignalProcessing CRD annotations

**Questions**:
1. Is this extraction implemented?
2. What's the exact JSON path for custom_labels in the Alertmanager payload?
3. Are there any label key restrictions (e.g., reserved prefixes)?

---

### Q2: Label Validation

**Question**: Does SignalProcessing validate custom_labels before forwarding?
- Max key length?
- Max value length?
- Max number of labels?
- Allowed characters?

**HolmesGPT-API Impact**: We currently pass through without validation. Should we add validation?

---

### Q3: RCA Context Enrichment

**Observation**: HolmesGPT-API receives the following from SignalProcessing:
```json
{
  "signal_type": "OOMKilled",
  "severity": "critical",
  "namespace": "production",
  "resource_name": "payment-service-abc123",
  "custom_labels": {...}
}
```

**Questions**:
1. Is `resource_kind` (Pod, Deployment, etc.) included?
2. Is `cluster_name` included for multi-cluster scenarios?
3. Are there additional context fields planned?

---

### Q4: Error Handling for HolmesGPT-API Failures

**Question**: What happens if HolmesGPT-API returns an error?
- Retry policy?
- Dead letter queue?
- Status update on SignalProcessing CRD?

---

## Confirmed Working ✅

The following are confirmed (no questions):
- Basic incident payload format
- signal_type and severity fields
- Namespace and resource information

---

## Action Items

| Item | Owner | Status |
|------|-------|--------|
| Clarify custom_labels extraction | SP Team | ✅ **ANSWERED** |
| Confirm RCA context fields | SP Team | ✅ **ANSWERED** |
| Document error handling | SP Team | ✅ **ANSWERED** |

**Note**: See detailed responses below (Q1-Q4 Response sections)

---

## Response

**Date**: December 1, 2025
**Responder**: SignalProcessing Team

---

### Q1 Response: custom_labels Source

**Correction**: Custom labels are NOT extracted directly from Alertmanager webhooks.

Per **DD-WORKFLOW-001 v1.8**, SignalProcessing uses a **Rego-based extraction system**:

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│ KubernetesContext│ →  │  Rego Engine     │ →  │  CustomLabels   │
│ + DetectedLabels │    │  (Sandboxed)     │    │  map[string][]  │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

**How it works**:

1. **Input to Rego**:
   - `KubernetesContext` (namespace, labels, annotations, ownerChain)
   - `DetectedLabels` (GitOps, PDB, HPA, StatefulSet, etc.)

2. **Policy Source**: ConfigMap `signal-processing-policies` in controller namespace

3. **Output Format**: `map[string][]string` (subdomain → multiple values)
   ```json
   {
     "constraint": ["cost-constrained", "stateful-safe"],
     "team": ["name=platform"],
     "region": ["zone=us-east-1"]
   }
   ```

4. **Label Key Restrictions**:
   - Reserved prefixes: `kubernaut.ai/`, `system/`
   - Max key length: 63 chars (K8s label key limit)
   - Allowed chars: `[a-zA-Z0-9._-]`

**Not implemented**: Direct Alertmanager label extraction. All custom labels come from Rego evaluation.

---

### Q2 Response: Label Validation

**Yes**, SignalProcessing validates custom_labels:

| Constraint | Limit | Rationale |
|------------|-------|-----------|
| Max keys | 10 | Prevent prompt bloat |
| Max values per key | 5 | Reasonable multi-value |
| Max key length | 63 chars | K8s label compatibility |
| Max value length | 100 chars | Prompt efficiency |
| Allowed key chars | `[a-zA-Z0-9._-]` | K8s label compatible |
| Allowed value chars | UTF-8 printable | Prompt safety |

**Security**: Rego policies run in a **sandboxed OPA runtime** with:
- No network access
- No filesystem access
- Timeout limits (5s default)
- Memory limits

**Recommendation for HolmesGPT-API**: You can trust these limits are enforced. No additional validation needed unless you want stricter prompt-specific limits.

---

### Q3 Response: RCA Context Enrichment

**All fields confirmed**:

| Field | Included | Source |
|-------|----------|--------|
| `resource_kind` | ✅ YES | `KubernetesContext.Kind` |
| `cluster_name` | ✅ YES | `KubernetesContext.ClusterName` |
| `namespace` | ✅ YES | `KubernetesContext.Namespace` |
| `resource_name` | ✅ YES | `KubernetesContext.Name` |
| `owner_chain` | ✅ YES (NEW) | `OwnerChain[]` - full ownership hierarchy |

**Additional context fields available** (per DD-WORKFLOW-001 v1.8):

```json
{
  "kubernetes_context": {
    "kind": "Pod",
    "name": "payment-service-abc123",
    "namespace": "production",
    "cluster_name": "prod-us-east-1",
    "labels": {...},
    "annotations": {...}
  },
  "owner_chain": [
    {"kind": "ReplicaSet", "name": "payment-service-789", "namespace": "production"},
    {"kind": "Deployment", "name": "payment-service", "namespace": "production"}
  ],
  "detected_labels": {
    "is_gitops_managed": true,
    "gitops_tool": "argocd",
    "has_pdb": true,
    "is_stateful_set": false,
    "has_network_policy": true,
    "pod_security_standard": "restricted"
  },
  "custom_labels": {
    "constraint": ["cost-constrained"],
    "team": ["name=platform"]
  }
}
```

---

### Q4 Response: Error Handling for HolmesGPT-API Failures

**SignalProcessing handles HolmesGPT-API errors as follows**:

| Scenario | Behavior |
|----------|----------|
| **Transient errors** (5xx, timeout) | Exponential backoff retry (max 5 attempts) |
| **Client errors** (4xx) | No retry, immediate failure |
| **Rate limiting** (429) | Respect `Retry-After` header |

**CRD Status Updates**:

```yaml
status:
  phase: Failed  # or Retrying
  conditions:
  - type: HolmesGPTAPIReady
    status: "False"
    reason: "APIError"
    message: "HolmesGPT-API returned 503: Service Unavailable"
    lastTransitionTime: "2025-12-01T12:00:00Z"
  retryCount: 3
  lastRetryTime: "2025-12-01T12:05:00Z"
```

**Events**:
```bash
kubectl describe signalprocessing payment-oomkilled-abc123
# Events:
#   Type     Reason              Message
#   Warning  HolmesGPTAPIError   Failed to call HolmesGPT-API: 503 Service Unavailable
#   Normal   Retrying            Retry attempt 3/5 after 30s backoff
```

**Dead Letter Queue**: Not implemented. Failed CRDs remain in `Failed` phase for manual intervention or reprocessing.

**Metrics**:
- `signalprocessing_holmesgpt_api_errors_total{error_type="5xx|4xx|timeout"}`
- `signalprocessing_holmesgpt_api_retry_count_histogram`

---

## Action Items Updated

| Item | Owner | Status |
|------|-------|--------|
| Clarify custom_labels extraction | SP Team | ✅ **ANSWERED** - Rego-based, not Alertmanager |
| Confirm RCA context fields | SP Team | ✅ **ANSWERED** - All fields confirmed |
| Document error handling | SP Team | ✅ **ANSWERED** - Retry + status updates |

---

## Follow-Up Questions for HolmesGPT-API

### FQ1: Prompt Size Limits

**Question**: What's the max size HolmesGPT-API can handle for the enrichment context?

**Answer (HolmesGPT-API Team - Dec 1, 2025)**:

- **Empirical observation**: Prompts typically don't exceed ~50k tokens
- **Soft limit recommendation**: 64k tokens (if you need a target)
- **Current policy**: **No hard limit enforced**

**Rationale**: We're using Claude Haiku which handles large contexts efficiently without significant time penalty. We don't want to artificially limit context size at this stage.

**Recommendation for SP Team**: Your current limits (10 keys × 5 values × 100 chars ≈ 5KB max for custom_labels) are well within bounds.

---

### FQ2: CustomLabels in Prompt

**Question**: How are `custom_labels` included in the LLM prompt?

**Answer (HolmesGPT-API Team - Dec 1, 2025)**:

**`custom_labels` are NOT included in the LLM prompt.**

Architecture (per DD-HAPI-001 Auto-Append):

```
┌──────────────────────────────────────────────────────────────┐
│                    HolmesGPT-API                             │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Incoming Request:                                     │  │
│  │  - detected_labels (GitOps, HPA, PDB...) → LLM SEES    │  │
│  │  - custom_labels (team, region, constraint) → STORED   │  │
│  └────────────────────────────────────────────────────────┘  │
│                           │                                  │
│                           ▼                                  │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  LLM Prompt:                                           │  │
│  │  - RCA context (signal, namespace, resource)           │  │
│  │  - detected_labels for environment reasoning           │  │
│  │  - NO custom_labels (invisible to LLM)                 │  │
│  │  - Available tools: search_workflow_catalog            │  │
│  └────────────────────────────────────────────────────────┘  │
│                           │                                  │
│                           ▼ LLM calls tool                   │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Tool Call: search_workflow_catalog(query, signal_type)│  │
│  │                        │                               │  │
│  │                        ▼                               │  │
│  │  AUTO-INJECT both labels before sending to DS:         │  │
│  │  - detected_labels (conditional - per DD-WORKFLOW-001) │  │
│  │  - custom_labels (always - per DD-HAPI-001)            │  │
│  └────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

**Why this design** (per DD-WORKFLOW-001 v1.8 + DD-HAPI-001):

| Label Type | In LLM Prompt? | In Search Query? | Purpose |
|------------|----------------|------------------|---------|
| `detected_labels` | ✅ YES | ✅ YES (conditional) | Environment reasoning + workflow filtering |
| `custom_labels` | ❌ NO | ✅ YES (auto-append) | Customer filtering constraints only |

**Rationale**:
- **detected_labels in prompt**: LLM can reason about environment (e.g., "This is GitOps-managed via ArgoCD, avoid direct kubectl changes")
- **custom_labels NOT in prompt**: Pure filtering constraints, LLM doesn't need to reason about them
- **Both auto-injected to search**: Ensures deterministic workflow filtering

---

## SignalProcessing Team Acknowledgment

**Date**: December 1, 2025

### ✅ Answers Received and Understood

| Question | Key Insight | Impact on SP |
|----------|-------------|--------------|
| **FQ1: Prompt Size** | No hard limit, 64k tokens soft limit | ✅ Our 5KB max for custom_labels is fine |
| **FQ2: Labels in Prompt** | `detected_labels` → LLM sees, `custom_labels` → search only | ✅ Critical architecture insight |

### 🎯 Key Takeaway for SignalProcessing

**`detected_labels` quality is critical** because:
- LLM reasons about them for workflow selection
- Example: "GitOps-managed via ArgoCD → avoid direct kubectl"
- SP must ensure accurate detection of:
  - `is_gitops_managed` / `gitops_tool`
  - `has_pdb` / `has_hpa`
  - `is_stateful_set`
  - `pod_security_standard`

**`custom_labels` are purely filtering** - less critical for accuracy, more for customer-defined constraints.

### No Further Questions

The HolmesGPT-API team's answers are comprehensive. **No follow-up questions.**

---

## ✅ Document Status: COMPLETE

| Section | Status |
|---------|--------|
| HAPI → SP Questions (Q1-Q4) | ✅ Answered |
| SP → HAPI Follow-ups (FQ1-FQ2) | ✅ Answered |
| SP Acknowledgment | ✅ Complete |

**All questions resolved. This document is closed.**

---

