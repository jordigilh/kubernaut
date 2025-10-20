# Safety-Aware Investigation Pattern

**Date**: October 16, 2025
**Purpose**: Document how RemediationProcessor provides safety context to the LLM for intelligent action selection
**Status**: ✅ APPROVED Architecture
**Replaces**: HolmesGPT API safety endpoint (removed)

---

## 🎯 **Overview**

**Safety-aware investigation** embeds risk information directly into the investigation prompt, allowing the LLM to make intelligent action recommendations with full safety context. This replaces the deprecated separate safety endpoint approach.

---

## 🏗️ **Architecture**

### **Flow**

```
1. RemediationProcessor enriches context
   ↓ (includes safety information)
2. AIAnalysis Controller calls HolmesGPT API
   ↓ POST /api/v1/investigate
3. HolmesGPT API receives enriched context
   ↓ (safety info in prompt)
4. LLM analyzes WITH safety awareness
   ↓ (considers risks in recommendations)
5. Returns safe, context-aware recommendations
```

### **Key Principle**

> **The model decides which action is best, knowing beforehand the criticality and priority of the signal.**

---

## 📊 **Safety Context Schema**

### **RemediationProcessor Enrichment**

**Format**: Self-Documenting JSON (DD-HOLMESGPT-009)
**Token Efficiency**: ~290 tokens (60% reduction from verbose format)
**Legend**: ZERO tokens (no legend needed - all keys are self-documenting)
**Decision Document**: `docs/architecture/decisions/DD-HOLMESGPT-009-Self-Documenting-JSON-Format.md`

```json
{
  "investigation_id": "mem-spike-prod-abc123",
  "priority": "P0",
  "environment": "production",
  "service": "payment-api",
  "safety_constraints": {
    "max_downtime_seconds": 60,
    "requires_approval": false,
    "allowed_actions": ["scale", "restart", "rollback"],
    "forbidden_actions": ["delete_*"]
  },
  "dependencies": [
    {"service": "api-gateway", "impact": "critical"},
    {"service": "auth-service", "impact": "high"}
  ],
  "data_criticality": "high",
  "user_impact": "critical",
  "alert": {
    "name": "OOMKilled",
    "namespace": "production",
    "pod": "web-app-789",
    "memory_usage": "498/512"
  },
  "kubernetes": {
    "deployment": "web-app",
    "replicas": 3,
    "node": "node-1"
  },
  "monitoring": {
    "related_alerts": 3,
    "cpu_trend": "stable",
    "memory_trend": "increasing"
  },
  "scope": {
    "time_window": "24h",
    "detail_level": "detailed",
    "include_history": true
  },
  "rego_policy": {
    "version": "v1.2",
    "rules": ["production_constraints", "p0_restrictions"],
    "dry_run": false
  },
  "task": "Generate 2-3 recommendations with dependencies for parallel execution. Return structured format with id, action, target, params, dependencies, probability, risk, and rationale."
}
```

**Key Benefits:**
- 100% self-documenting - no legend lookup required
- Natural language keys work with any LLM without training
- Excellent readability in logs and debuggers
- Zero maintenance overhead (no legend synchronization)

**Verbose Format (Legacy Reference)**:
```json
{
  "alert_context": {...},
  "monitoring_context": {...},
  "business_context": {...},
  "safety_context": {
    "priority": "P0",
    "criticality": "high",
    "environment": "production",
    "action_constraints": {
      "max_downtime_seconds": 60,
      "requires_approval": false,
      "allowed_action_types": ["scale", "restart", "rollback"],
      "forbidden_action_types": ["delete_deployment", "delete_namespace"]
    },
    "risk_factors": {
      "service_dependencies": [
        {"service": "api-gateway", "impact": "critical"},
        {"service": "auth-service", "impact": "high"}
      ],
      "data_criticality": "high",
      "user_impact_potential": "critical"
    },
    "rego_policy_context": {
      "policy_version": "v1.2",
      "applicable_rules": ["production_constraints", "p0_restrictions"],
      "dry_run_required": false
    }
  }
}
```

**Note**: Verbose format deprecated as of DD-HOLMESGPT-009. Use self-documenting JSON for all new implementations.

### **Field Descriptions**

| Field | Type | Purpose | Example |
|-------|------|---------|---------|
| **priority** | string | Signal priority (P0-P3) | `"P0"` (most urgent) |
| **criticality** | string | Service criticality level | `"high"` |
| **environment** | string | Deployment environment | `"production"` |
| **action_constraints** | object | Hard constraints on actions | Max downtime, forbidden actions |
| **risk_factors** | object | Known risk information | Dependencies, data criticality |
| **rego_policy_context** | object | Rego policy metadata | Policy version, applicable rules |

---

## 🤖 **LLM Integration**

### **Investigation Prompt (Self-Documenting JSON)**

**Format**: DD-HOLMESGPT-009 Self-Documenting JSON
**Legend**: ZERO tokens (no legend needed)

```json
{
  "investigation_id": "oom-api-svc-abc123",
  "priority": "P0",
  "environment": "production",
  "service": "api-service",
  "safety_constraints": {
    "max_downtime_seconds": 60,
    "requires_approval": false,
    "allowed_actions": ["scale", "restart", "rollback"],
    "forbidden_actions": ["delete_deployment", "delete_namespace"]
  },
  "dependencies": [
    {"service": "api-gateway", "impact": "critical"},
    {"service": "auth-service", "impact": "high"}
  ],
  "data_criticality": "high",
  "user_impact": "critical",
  "alert": {
    "name": "OOMKilled",
    "namespace": "production",
    "pod": "api-service-789",
    "memory_usage": "498/512",
    "event_count": 3
  },
  "kubernetes": {
    "deployment": "api-service",
    "replicas": 3,
    "node": "node-1",
    "image": "api:v2.0"
  },
  "monitoring": {
    "related_alerts": 2,
    "cpu_trend": "stable",
    "memory_trend": "increasing",
    "latency_trend": "increasing",
    "error_rate_trend": "increasing"
  },
  "scope": {
    "time_window": "24h",
    "detail_level": "detailed",
    "include_history": true
  },
  "rego_policy": {
    "version": "v1.2",
    "rules": ["production_constraints", "p0_restrictions"],
    "dry_run": false
  },
  "task": "Analyze OOMKilled incident. Generate 2-3 recommendations with dependencies for parallel execution. Return structured format with id, action, target, params, dependencies, probability, risk, and rationale. Respect 60 second downtime limit."
}
```

**Token Count**: ~290 tokens (vs ~800 tokens for verbose nested format)
**Readability**: Excellent - self-explanatory keys, no legend lookup required

**Legacy Verbose Format** (Deprecated):
```
You are investigating an incident in a PRODUCTION environment.

**Incident Details**:
- Alert: OOMKilled pods in api-service deployment
- Priority: P0 (CRITICAL)
- Criticality: HIGH
- Namespace: production
- Affected service: api-service

**Safety Context**:
- Environment: production
- Max downtime allowed: 60 seconds
- Service has 2 critical dependencies:
  * api-gateway (impact: critical)
  * auth-service (impact: high)
- Data criticality: high
- User impact potential: critical

**Action Constraints**:
- Allowed actions: scale, restart, rollback
- Forbidden actions: delete_deployment, delete_namespace
- Approval NOT required for recommended actions

**Your Task**:
Analyze the incident and recommend actions that:
1. Resolve the OOMKilled issue
2. Respect the 60-second downtime constraint
3. Minimize impact to dependent services
4. Consider data criticality and user impact
5. Use ONLY allowed action types

Provide 2-3 action recommendations ranked by safety and effectiveness.
```

### **LLM Response (Safety-Aware)**

```json
{
  "root_cause": "Memory limit (512Mi) insufficient for current load",
  "confidence": 0.92,
  "recommendations": [
    {
      "action": "scale_deployment",
      "priority": 1,
      "details": {
        "deployment": "api-service",
        "replicas": 5,
        "strategy": "gradual_scale"
      },
      "rationale": "Immediate relief without downtime. Honors production constraints.",
      "estimated_downtime_seconds": 0,
      "risk_level": "low",
      "impact_on_dependencies": "minimal"
    },
    {
      "action": "increase_memory_limit",
      "priority": 2,
      "details": {
        "deployment": "api-service",
        "new_limit": "2Gi",
        "rolling_restart": true
      },
      "rationale": "Addresses root cause. 45s downtime within 60s limit.",
      "estimated_downtime_seconds": 45,
      "risk_level": "medium",
      "impact_on_dependencies": "moderate"
    },
    {
      "action": "rollback_deployment",
      "priority": 3,
      "details": {
        "deployment": "api-service",
        "target_revision": "previous"
      },
      "rationale": "Fallback if memory increase fails. 30s downtime.",
      "estimated_downtime_seconds": 30,
      "risk_level": "low",
      "impact_on_dependencies": "minimal"
    }
  ]
}
```

**Key Features**:
- ✅ LLM considered safety context in recommendations
- ✅ All actions respect constraints (no delete operations)
- ✅ Downtime estimates within 60s limit
- ✅ Dependency impact assessed for each action
- ✅ Risk levels explicitly stated

---

## 🔄 **Workflow Integration**

### **Step 1: RemediationProcessor Enrichment**

```go
// pkg/processor/context_enrichment.go
func (p *RemediationProcessor) EnrichWithSafetyContext(
    ctx context.Context,
    alert AlertData,
) (*EnrichedContext, error) {
    enriched := &EnrichedContext{
        Alert:      alert,
        Monitoring: p.getMonitoringContext(ctx, alert),
        Business:   p.getBusinessContext(ctx, alert),
    }

    // Add safety context
    enriched.Safety = &SafetyContext{
        Priority:     alert.Priority,
        Criticality:  p.determineServiceCriticality(alert.Namespace),
        Environment:  p.getEnvironment(alert.Namespace),
        Constraints:  p.getActionConstraints(alert.Priority, alert.Namespace),
        RiskFactors:  p.assessRiskFactors(ctx, alert),
        RegoContext:  p.getRegoPolicyContext(alert.Priority),
    }

    return enriched, nil
}
```

### **Step 2: AIAnalysis Controller Prompt Construction**

```go
// pkg/aianalysis/prompt_builder.go
func (b *PromptBuilder) BuildInvestigationPrompt(
    enriched *EnrichedContext,
) string {
    return fmt.Sprintf(`
You are investigating an incident in a %s environment.

**Incident Details**:
- Alert: %s
- Priority: %s (CRITICAL)
- Criticality: %s
- Namespace: %s

**Safety Context**:
- Environment: %s
- Max downtime allowed: %d seconds
- Service dependencies: %s
- Data criticality: %s
- User impact potential: %s

**Action Constraints**:
- Allowed actions: %s
- Forbidden actions: %s
- Approval required: %t

**Your Task**:
Analyze the incident and recommend actions that respect all safety constraints.
`,
        enriched.Safety.Environment,
        enriched.Alert.AlertName,
        enriched.Safety.Priority,
        enriched.Safety.Criticality,
        enriched.Alert.Namespace,
        enriched.Safety.Environment,
        enriched.Safety.Constraints.MaxDowntimeSeconds,
        formatDependencies(enriched.Safety.RiskFactors.ServiceDependencies),
        enriched.Safety.RiskFactors.DataCriticality,
        enriched.Safety.RiskFactors.UserImpactPotential,
        strings.Join(enriched.Safety.Constraints.AllowedActionTypes, ", "),
        strings.Join(enriched.Safety.Constraints.ForbiddenActionTypes, ", "),
        enriched.Safety.Constraints.RequiresApproval,
    )
}
```

### **Step 3: WorkflowExecution Validation (Rego)**

```rego
# config/rego/safety_policies.rego
package safety

import future.keywords.if

# Validate recommended action against policy
validate_action(action) if {
    # Check action type is allowed
    action.action in data.allowed_actions[input.priority]

    # Check downtime constraint
    action.estimated_downtime_seconds <= data.max_downtime[input.environment]

    # Check approval requirements
    not requires_approval(action)
}

requires_approval(action) if {
    input.priority == "P0"
    action.risk_level == "high"
}

requires_approval(action) if {
    input.environment == "production"
    action.action in ["delete_deployment", "delete_statefulset"]
}
```

---

## 📊 **Benefits vs. Separate Safety Endpoint**

| Aspect | Safety Endpoint (Removed) | Safety-Aware Investigation (Current) |
|--------|---------------------------|--------------------------------------|
| **API Calls** | 2 calls (investigate + safety) | 1 call (investigate with safety) |
| **Latency** | ~4-6 seconds | ~2-3 seconds |
| **Context Loss** | Separate calls lose context | Full context in single prompt |
| **LLM Intelligence** | Limited (two-step process) | Full (holistic decision) |
| **Cost** | 2× LLM calls | 1× LLM call |
| **Maintenance** | 2 endpoints | 1 endpoint |
| **Recommendation Quality** | Lower (separate analysis) | Higher (integrated analysis) |

---

## ✅ **Validation**

### **RemediationProcessor Validation**

```go
// test/unit/processor/safety_enrichment_test.go
var _ = Describe("Safety Context Enrichment", func() {
    It("should include all safety fields", func() {
        enriched, err := processor.EnrichWithSafetyContext(ctx, alert)
        Expect(err).ToNot(HaveOccurred())
        Expect(enriched.Safety).ToNot(BeNil())
        Expect(enriched.Safety.Priority).To(Equal("P0"))
        Expect(enriched.Safety.Criticality).To(Equal("high"))
        Expect(enriched.Safety.Environment).To(Equal("production"))
        Expect(enriched.Safety.Constraints).ToNot(BeNil())
        Expect(enriched.Safety.RiskFactors).ToNot(BeNil())
    })

    It("should enforce action constraints based on priority", func() {
        enriched, _ := processor.EnrichWithSafetyContext(ctx, p0Alert)
        Expect(enriched.Safety.Constraints.AllowedActionTypes).To(
            ContainElements("scale", "restart", "rollback"),
        )
        Expect(enriched.Safety.Constraints.ForbiddenActionTypes).To(
            ContainElement("delete_deployment"),
        )
    })
})
```

### **AIAnalysis Prompt Validation**

```go
// test/unit/aianalysis/prompt_test.go
var _ = Describe("Safety-Aware Prompt", func() {
    It("should include safety context in prompt", func() {
        prompt := builder.BuildInvestigationPrompt(enriched)
        Expect(prompt).To(ContainSubstring("Priority: P0"))
        Expect(prompt).To(ContainSubstring("Criticality: high"))
        Expect(prompt).To(ContainSubstring("Max downtime allowed: 60 seconds"))
        Expect(prompt).To(ContainSubstring("Allowed actions:"))
        Expect(prompt).To(ContainSubstring("Forbidden actions:"))
    })
})
```

---

## 📝 **Design Decisions**

**See**: `docs/decisions/DD-HOLMESGPT-008-Safety-Aware-Investigation.md`

**Key Decision**: Embed safety context in investigation prompt instead of separate endpoint.

**Alternatives Considered**:
1. **Separate Safety Endpoint** (rejected - 2× cost, context loss)
2. **Post-Investigation Filtering** (rejected - wastes LLM analysis)
3. **Safety-Aware Investigation** (selected - optimal cost/quality)

---

## 🔗 **Related Documentation**

- **Architecture**: `docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md`
- **Decision**: `docs/decisions/DD-HOLMESGPT-008-Safety-Aware-Investigation.md`
- **RemediationProcessor**: `docs/services/stateless/remediation-processor/README.md`
- **WorkflowExecution**: `docs/services/stateless/workflow-execution/README.md`
- **Rego Policies**: `config/rego/safety_policies.rego`

