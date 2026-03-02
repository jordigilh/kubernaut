# BR-HAPI-198: Operator-Configurable Confidence Thresholds

**Business Requirement ID**: BR-HAPI-198
**Category**: HolmesGPT-API / AIAnalysis
**Priority**: P2
**Target Version**: V1.1
**Status**: 📋 PLANNED
**Date**: December 6, 2025

---

## 📋 Business Need

### Problem Statement

V1.0 implements a **global 70% confidence threshold** for determining when human review is required. However, operators have different risk tolerances based on:

| Factor | Higher Threshold Needed | Lower Threshold Acceptable |
|--------|------------------------|---------------------------|
| **Environment** | Production | Development, Staging |
| **Severity** | Critical, High | Low, Warning |
| **Resource Type** | StatefulSet, Database | Stateless apps |
| **Business Category** | Revenue-critical | Internal tools |

**Current V1.0 Limitation**:
- Single global threshold (70%) for all scenarios
- Operators cannot tune based on their specific risk tolerance
- One-size-fits-all approach may be too conservative or too aggressive

### Business Value

| Benefit | Impact |
|---------|--------|
| **Risk Management** | Operators can set stricter thresholds for critical workloads |
| **Efficiency** | Lower thresholds for dev/staging reduces unnecessary manual reviews |
| **Flexibility** | Different teams can tune for their specific needs |
| **Compliance** | Meet regulatory requirements for critical systems |

---

## 🎯 Requirements

### BR-HAPI-198.1: Rule-Based Threshold Configuration

**MUST**: AIAnalysis service SHALL support rule-based confidence threshold configuration.

```yaml
# AIAnalysis ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: aianalysis-config
  namespace: kubernaut-system
data:
  confidence-rules.yaml: |
    confidence_rules:
      # Rule 1: Critical production workloads need 90% confidence
      - name: critical-production
        match:
          severity: [critical, high]
          environment: production
        threshold: 0.90
        description: "High bar for critical production remediation"

      # Rule 2: Stateful workloads need higher confidence
      - name: stateful-workloads
        match:
          resource_kind: [StatefulSet, PersistentVolumeClaim]
        threshold: 0.85
        description: "Extra caution for stateful resources"

      # Rule 3: Development environments can be more aggressive
      - name: dev-environment
        match:
          environment: [development, dev, sandbox]
        threshold: 0.60
        description: "Lower bar for non-production"

      # Default rule (required)
      - name: default
        match: {}
        threshold: 0.70
        description: "Default threshold for unmatched scenarios"
```

### BR-HAPI-198.2: Match Criteria

**MUST**: Rules SHALL support matching on the following criteria from `IncidentRequest`:

| Field | Type | Example Values |
|-------|------|----------------|
| `severity` | string[] | `["critical", "high", "medium", "low"]` |
| `environment` | string[] | `["production", "staging", "development"]` |
| `resource_kind` | string[] | `["Pod", "Deployment", "StatefulSet"]` |
| `resource_namespace` | string[] | `["kube-system", "production"]` |
| `business_category` | string[] | `["revenue-critical", "internal"]` |
| `cluster_name` | string[] | `["prod-us-east", "dev-cluster"]` |

### BR-HAPI-198.3: Rule Evaluation Order

**MUST**: Rules SHALL be evaluated in order, and the **first matching rule** SHALL be applied.

**MUST**: A `default` rule with empty `match: {}` SHALL be required as the last rule.

### BR-HAPI-198.4: Threshold Application

**MUST**: When AIAnalysis receives `IncidentResponse` from HAPI:
1. Extract `selected_workflow.confidence` value
2. Evaluate rules in order to find matching threshold
3. If `confidence < threshold`, set internal `requiresHumanReview = true`

**Note**: HAPI remains stateless and threshold-agnostic. It returns `confidence` only.

### BR-HAPI-198.5: Audit Logging

**SHOULD**: When a threshold rule is applied, AIAnalysis SHOULD log:
- Rule name that matched
- Threshold value applied
- Confidence value from HAPI
- Decision (pass/fail)

```json
{
  "level": "info",
  "msg": "Confidence threshold evaluated",
  "rule_name": "critical-production",
  "threshold": 0.90,
  "confidence": 0.85,
  "decision": "requires_human_review",
  "incident_id": "inc-123"
}
```

---

## 📊 Use Cases

### Use Case 1: Production Safety

```yaml
# Operator wants 90% confidence for production critical workloads
- name: prod-critical
  match:
    environment: production
    severity: critical
  threshold: 0.90
```

**Result**: AI recommendation with 85% confidence → Human review required

### Use Case 2: Dev Environment Efficiency

```yaml
# Operator accepts lower confidence in dev to reduce manual reviews
- name: dev-permissive
  match:
    environment: development
  threshold: 0.50
```

**Result**: AI recommendation with 55% confidence → Auto-execute allowed

### Use Case 3: Database Protection

```yaml
# Extra caution for any database-related workloads
- name: database-protection
  match:
    resource_kind: [StatefulSet]
    resource_namespace: [database, postgres, mysql]
  threshold: 0.95
```

**Result**: Even 90% confidence → Human review required for databases

---

## 🔗 Dependencies

| Component | Role |
|-----------|------|
| **HAPI** | Returns `confidence` (no changes needed) |
| **AIAnalysis** | Implements rule evaluation and threshold application |
| **ConfigMap** | Stores operator-defined rules |

---

## ✅ Acceptance Criteria

### AC-1: Rules Loaded from ConfigMap

```gherkin
Given a ConfigMap with confidence_rules is deployed
When AIAnalysis controller starts
Then it SHALL load and parse the confidence rules
And log the number of rules loaded
```

### AC-2: First Matching Rule Applied

```gherkin
Given confidence_rules with multiple rules
And an IncidentResponse with severity=critical, environment=production
When AIAnalysis evaluates the confidence threshold
Then the first matching rule SHALL be applied
And audit log SHALL show which rule matched
```

### AC-3: Default Rule Required

```gherkin
Given confidence_rules without a default rule
When AIAnalysis attempts to load the configuration
Then it SHALL fail with error "default rule required"
```

### AC-4: Hot Reload Support

```gherkin
Given AIAnalysis is running
When the ConfigMap is updated with new rules
Then AIAnalysis SHALL reload the rules within 60 seconds
And log "Confidence rules reloaded: N rules"
```

---

## 📋 Implementation Notes

### V1.0 Compatibility

- **V1.0 (original)**: Hardcoded threshold in Rego policy (80% for auto-approval, per BR-AI-003/BR-AI-076)
- **V1.0 (#225)**: Threshold is now configurable via `input.confidence_threshold` passed from the AIAnalysis controller config (`rego.confidenceThreshold`). The Rego policy defines a built-in default (0.8) that applies when no override is configured. This is a stepping stone toward V1.1's rule-based system.
- **V1.1 (planned)**: Full rule-based threshold configuration per environment/severity/resource

### V1.0 Configurable Threshold (#225)

Operators can now override the confidence threshold without editing the Rego policy:

```yaml
# AIAnalysis config.yaml — set a global threshold
rego:
  policyPath: "/etc/aianalysis/policies/approval.rego"
  confidenceThreshold: 0.85  # Overrides the Rego policy's default (0.8)
```

The Rego policy uses `input.confidence_threshold` with a default fallback:

```rego
# Built-in default (applies when input.confidence_threshold is not set)
default confidence_threshold := 0.8

# Override from controller config
confidence_threshold := input.confidence_threshold if {
    input.confidence_threshold
}

is_high_confidence if {
    input.confidence >= confidence_threshold
}
```

### V1.1 Migration Path

```yaml
# V1.0 equivalent (global threshold via config)
rego:
  policyPath: "/etc/aianalysis/policies/approval.rego"
  confidenceThreshold: 0.80

# V1.1 operator customization (rule-based — planned)
confidence_rules:
  - name: critical-production
    match: {severity: critical, environment: production}
    threshold: 0.90
  - name: default
    match: {}
    threshold: 0.80
```

---

## 📎 Related Documents

- [BR-HAPI-197: Human Review Required Flag](./BR-HAPI-197-needs-human-review-field.md)
- [Q18 Response in AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md)
- [DD-HAPI-002: Workflow Response Validation](../architecture/decisions/DD-HAPI-002-workflow-parameter-validation.md)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-06 | Initial business requirement for V1.1 |
| 1.1 | 2026-02-28 | Updated V1.0 compatibility: threshold now configurable via `input.confidence_threshold` (#225). Fixed 70% → 80% discrepancy (actual V1.0 default is 80% per BR-AI-003). |


