# AIAnalysis Service - Initial Prompt Design

**Date**: November 14, 2025
**Status**: 📝 DRAFT (For Testing with AIAnalysis Service Implementation)
**Owner**: AIAnalysis Service + HolmesGPT API
**Version**: 0.1 (Pre-Implementation)

---

## 🎯 Purpose

This document defines the **initial prompt** that will be used to test the AIAnalysis service integration with HolmesGPT API. This prompt will be refined during AIAnalysis service development based on real-world testing.

**Note**: This is a **starting point** for testing, not a final design. The prompt will evolve as we implement and test the AIAnalysis service.

---

## 📋 Context

### What We Know

1. **Signal Processing** enriches alerts with 7 mandatory labels (DD-PLAYBOOK-001)
2. **AIAnalysis Controller** creates AIAnalysis CRD and calls HolmesGPT API
3. **HolmesGPT API** orchestrates LLM (Claude 3.5 Sonnet) investigation
4. **MCP Tools** available: `search_playbook_catalog`, `get_playbook_details`
5. **LLM Output**: Natural language with playbook selection and reasoning

### What We Need to Test

1. Does the LLM understand the 7 mandatory labels and their hints?
2. Does the LLM correctly use MCP tools to search playbooks?
3. Does the LLM provide clear reasoning for playbook selection?
4. Does the LLM handle edge cases (false positives, misdiagnosed alerts)?

---

## 🎨 Initial Prompt Structure

### System Prompt (Sets Context)

```
You are Kubernaut AI, an expert Kubernetes SRE assistant specializing in
incident investigation and remediation.

<role>
You investigate Kubernetes alerts to determine root causes and recommend
appropriate remediation playbooks.
</role>

<principles>
- Investigate thoroughly before recommending remediation
- Consider operational context (environment, risk tolerance, business impact)
- Prefer root cause fixes over symptom treatments
- Explain your reasoning clearly
</principles>

<available_tools>
- kubernetes_investigate: Query Kubernetes resources and metrics
- search_playbook_catalog: Search for remediation playbooks using natural language
- get_playbook_details: Get full details of a specific playbook
</available_tools>

<workflow>
1. Investigate the alert using available Kubernetes tools
2. Determine the root cause based on your investigation
3. Search for appropriate playbooks using search_playbook_catalog
4. Select the best playbook and explain your reasoning
</workflow>

You have full autonomy to investigate and make decisions. The context provided
is for guidance, not constraint.
```

---

### User Prompt Template (Alert Investigation)

```
INCIDENT ALERT

Alert Name: {{ alert.name }}
Timestamp: {{ alert.timestamp }}
Namespace: {{ alert.namespace }}
Resource: {{ alert.resource }}
Description: {{ alert.description }}

---

SIGNAL PROCESSING ANALYSIS

The Signal Processing system has analyzed this alert and provided the following
categorization. These are suggestions based on the initial alert data - your
investigation may reveal different findings.

Signal Type: {{ labels.signal_type }}
  ℹ️  Initial categorization. Your investigation may reveal a different root cause.

Severity: {{ labels.severity }}
  ℹ️  {{ severity_hint }}

Component: {{ labels.component }}
  ℹ️  The primary Kubernetes resource affected.

Environment: {{ labels.environment }}
  ℹ️  {{ environment_hint }}

Priority: {{ labels.priority }}
  ℹ️  {{ priority_hint }}

Risk Tolerance: {{ labels.risk_tolerance }}
  ℹ️  {{ risk_tolerance_hint }}

Business Category: {{ labels.business_category }}
  ℹ️  {{ business_category_hint }}

---

CLUSTER CONTEXT

Namespace: {{ cluster.namespace }}
Current State:
  - Pod Count: {{ cluster.pod_count }}
  - Resource Quotas: {{ cluster.quotas }}
  - Recent Events: {{ cluster.recent_events }}

Recent Metrics (Last 6 Hours):
  - CPU Usage: {{ metrics.cpu }}
  - Memory Usage: {{ metrics.memory }}
  - Network I/O: {{ metrics.network }}

---

YOUR TASK

1. INVESTIGATE: Use kubernetes_investigate to analyze the alert and cluster state
2. DIAGNOSE: Determine the root cause based on your investigation
3. SEARCH: Use search_playbook_catalog to find appropriate remediation playbooks
4. SELECT: Choose the best playbook and explain your reasoning

Remember: The Signal Processing labels are initial suggestions. Your investigation
findings take precedence.
```

---

## 💡 Context Hints (Dynamic)

### Severity Hints
```python
severity_hints = {
    "critical": "This is a critical issue requiring immediate attention. Consider the fastest safe remediation approach.",
    "warning": "This is a warning-level issue. You have time to investigate thoroughly before acting.",
    "info": "This is informational. Investigation recommended but no immediate action required."
}
```

### Environment Hints
```python
environment_hints = {
    "production": "This is a PRODUCTION environment. Prioritize stability and minimize downtime. Consider gradual rollouts and rollback plans.",
    "staging": "This is a STAGING environment. You can be more aggressive with fixes, but still maintain some caution.",
    "development": "This is a DEVELOPMENT environment. You can use aggressive fixes and experimental approaches."
}
```

### Priority Hints
```python
priority_hints = {
    "P0": "P0 priority means this is business-critical and requires immediate action. Downtime is unacceptable.",
    "P1": "P1 priority means this is high-priority but not immediately business-critical. You have some time to investigate.",
    "P2": "P2 priority means this is medium-priority. Thorough investigation is recommended before acting.",
    "P3": "P3 priority means this is low-priority. Take your time to find the best long-term solution."
}
```

### Risk Tolerance Hints
```python
risk_tolerance_hints = {
    "low": "Low risk tolerance means this resource requires conservative, well-tested remediation approaches. Avoid experimental fixes. Consider gradual rollouts and extensive testing.",
    "medium": "Medium risk tolerance means you can use standard remediation approaches. Some risk is acceptable if it speeds up resolution.",
    "high": "High risk tolerance means aggressive fixes are acceptable. Speed of resolution is prioritized over caution."
}
```

### Business Category Hints
```python
business_category_hints = {
    "payments": "This is a payment processing service. Consider PCI compliance, transaction integrity, and revenue impact. Downtime directly affects revenue.",
    "auth": "This is an authentication service. Security and availability are critical. Users cannot access the system if this fails.",
    "analytics": "This is an analytics service. Downtime is less critical than data integrity. Consider data loss prevention.",
    "internal-tools": "This is an internal tool. Downtime affects productivity but not customer-facing services."
}
```

---

## 🧪 Test Scenarios (To Validate Prompt)

### Scenario 1: Simple OOMKilled (Resource Limit)
**Expected Behavior**:
- LLM investigates and finds memory usage correlates with traffic
- LLM searches for playbooks addressing resource limits
- LLM selects conservative memory increase playbook (low risk tolerance)

### Scenario 2: OOMKilled but Root Cause is Memory Leak
**Expected Behavior**:
- LLM investigates and finds linear memory growth (not traffic-correlated)
- LLM overrides initial signal_type ("OOMKilled" → "MemoryLeak")
- LLM searches for memory leak playbooks, excludes "increase memory limit"
- LLM selects leak remediation playbook

### Scenario 3: False Positive (Scheduled Job)
**Expected Behavior**:
- LLM investigates and finds CPU spike is scheduled job
- LLM determines this is NOT an incident
- LLM does NOT search for playbooks
- LLM recommends alert adjustment

---

## 📊 Success Metrics (For Testing)

| Metric | Target | How to Measure |
|--------|--------|----------------|
| **Root Cause Accuracy** | >90% | LLM identifies correct root cause vs. initial signal_type |
| **Playbook Selection Accuracy** | >85% | Selected playbook resolves issue successfully |
| **Context Hint Usage** | 60-80% | LLM references hints in reasoning |
| **False Positive Detection** | >95% | LLM correctly identifies non-issues |
| **Reasoning Quality** | >80% | Human reviewers rate reasoning as clear and logical |

---

## 🔄 Iteration Plan

### Phase 1: Initial Testing (AIAnalysis Service Development)
1. Implement this initial prompt
2. Test with 10-20 different alert scenarios
3. Collect LLM responses and analyze quality
4. Identify prompt improvements

### Phase 2: Refinement (Based on Testing)
1. Adjust context hints based on LLM behavior
2. Add few-shot examples if needed
3. Optimize prompt length (balance context vs. token cost)
4. Test refined prompt with same scenarios

### Phase 3: Production Deployment
1. Deploy refined prompt to production
2. Monitor LLM decision quality
3. Collect feedback from remediation outcomes
4. Continue iterative improvements

---

## 🚨 Open Questions (To Answer During Testing)

1. **Few-Shot Examples**: Do we need example investigations in the prompt?
2. **Token Budget**: Is the prompt too long? (Current: ~4,500 tokens)
3. **Hint Effectiveness**: Do context hints actually influence LLM decisions?
4. **Tool Usage**: Does LLM correctly use MCP tools without explicit examples?
5. **Edge Cases**: How does LLM handle ambiguous or incomplete alert data?

---

## 📝 Notes for AIAnalysis Service Implementation

### Integration Points

1. **AIAnalysis Controller** creates AIAnalysis CRD with:
   - Alert data
   - 7 enriched labels
   - Cluster context

2. **AIAnalysis Controller** calls HolmesGPT API:
   ```
   POST /api/v1/investigate
   {
     "alert": {...},
     "labels": {...},
     "cluster_context": {...}
   }
   ```

3. **HolmesGPT API** constructs prompt using this template

4. **HolmesGPT API** calls Claude 3.5 Sonnet with MCP tools

5. **HolmesGPT API** parses LLM response and returns:
   ```
   {
     "root_cause": "...",
     "selected_playbook": {
       "playbook_id": "...",
       "version": "..."
     },
     "reasoning": "...",
     "confidence": 0.95
   }
   ```

6. **AIAnalysis Controller** updates AIAnalysis CRD status

---

## 🔗 Related Documents

- [DD-PLAYBOOK-001](../../architecture/decisions/DD-PLAYBOOK-001-mandatory-label-schema.md) - Mandatory label schema
- [DD-PLAYBOOK-002](../../architecture/decisions/DD-PLAYBOOK-002-MCP-PLAYBOOK-CATALOG-ARCHITECTURE.md) - MCP architecture
- [BR-AI-001 to BR-AI-050](../../requirements/02_AI_MACHINE_LEARNING.md) - AI Analysis BRs
- [HolmesGPT API BRs](../stateless/holmesgpt-api/BUSINESS_REQUIREMENTS.md) - HolmesGPT API BRs

---

## ✅ Next Steps

1. ⏸️ **Wait for AIAnalysis Service Implementation** to begin
2. 🚧 **Implement this initial prompt** in HolmesGPT API
3. 🚧 **Test with real alerts** during AIAnalysis development
4. 🚧 **Refine prompt** based on testing results
5. 🚧 **Document final prompt** after validation

---

**Document Version**: 0.1 (Pre-Implementation Draft)
**Last Updated**: November 14, 2025
**Status**: 📝 DRAFT (Awaiting AIAnalysis Service Implementation)
**Next Review**: When AIAnalysis service development begins

