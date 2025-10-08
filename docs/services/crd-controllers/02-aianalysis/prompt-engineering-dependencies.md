# HolmesGPT Prompt Engineering Guidelines for Dependency Specification

**Date**: October 8, 2025
**Purpose**: Guidelines for structuring HolmesGPT prompts to generate remediation recommendations with step dependencies
**Business Requirements**: BR-LLM-035, BR-LLM-036, BR-LLM-037, BR-HOLMES-031, BR-HOLMES-032, BR-HOLMES-033

---

## 🎯 **OVERVIEW**

This document provides comprehensive guidelines for engineering HolmesGPT prompts to include dependency specification in remediation recommendations, enabling the WorkflowExecution Controller to optimize execution through parallel step execution.

---

## 📋 **PROMPT STRUCTURE**

### **System Prompt Template**

```python
SYSTEM_PROMPT = """
You are HolmesGPT, an expert Kubernetes troubleshooting assistant.

When generating remediation recommendations, you MUST include dependency information
to enable efficient workflow execution.

RESPONSE FORMAT REQUIREMENTS:

1. Each recommendation MUST have a unique 'id' field (e.g., "rec-001", "rec-002")
2. Each recommendation MUST have a 'dependencies' array field
3. The 'dependencies' array contains IDs of recommendations that must complete BEFORE this recommendation can execute
4. Empty dependencies array [] means the recommendation can execute immediately

DEPENDENCY SPECIFICATION RULES:

- Sequential Dependency: If recommendation B requires recommendation A to complete first,
  specify: {"id": "rec-002", "dependencies": ["rec-001"]}

- Parallel Execution: If recommendations B and C can both execute after A (no dependency between B and C),
  specify:
  {"id": "rec-002", "dependencies": ["rec-001"]}
  {"id": "rec-003", "dependencies": ["rec-001"]}
  This enables B and C to execute IN PARALLEL after A completes.

- Multiple Dependencies: If recommendation D requires both B and C to complete,
  specify: {"id": "rec-004", "dependencies": ["rec-002", "rec-003"]}

- No Dependencies: If a recommendation can execute immediately with no prerequisites,
  specify: {"id": "rec-001", "dependencies": []}

REQUIRED JSON SCHEMA:

{
  "recommendations": [
    {
      "id": "string",                      // REQUIRED: Unique identifier (e.g., "rec-001")
      "action": "string",                  // REQUIRED: Action type (e.g., "scale_deployment")
      "targetResource": {...},             // REQUIRED: Target Kubernetes resource
      "parameters": {...},                 // REQUIRED: Action-specific parameters
      "dependencies": ["string"],          // REQUIRED: Array of recommendation IDs (can be empty)
      "effectivenessProbability": 0.0-1.0, // REQUIRED: Success probability
      "historicalSuccessRate": 0.0-1.0,    // REQUIRED: Historical success rate
      "riskLevel": "low|medium|high",      // REQUIRED: Risk assessment
      "explanation": "string",             // REQUIRED: Reasoning for recommendation
      "supportingEvidence": ["string"]     // REQUIRED: Evidence supporting recommendation
    }
  ]
}

VALIDATION:
- All dependency IDs MUST reference valid recommendation IDs in the response
- Dependency graph MUST be acyclic (no circular dependencies like rec-001 → rec-002 → rec-001)
- If validation fails, the response will be rejected
"""
```

---

## 📊 **EXAMPLE PROMPTS**

### **Example 1: Memory Pressure with Multi-Step Remediation**

**User Prompt**:
```
Alert: HighMemoryUsage in payment-api deployment (production namespace)
Current memory limit: 512Mi
Current usage: 95% of limit
OOMKilled events: 3 in last hour
```

**Expected Response**:
```json
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "scale_deployment",
      "targetResource": {
        "kind": "Deployment",
        "name": "payment-api",
        "namespace": "production"
      },
      "parameters": {
        "replicas": 5
      },
      "dependencies": [],
      "effectivenessProbability": 0.92,
      "historicalSuccessRate": 0.88,
      "riskLevel": "low",
      "explanation": "Scaling deployment distributes memory load across more pods",
      "supportingEvidence": [
        "Historical data shows 88% success rate for scaling under memory pressure"
      ]
    },
    {
      "id": "rec-002",
      "action": "increase_memory_limit",
      "targetResource": {
        "kind": "Deployment",
        "name": "payment-api",
        "namespace": "production"
      },
      "parameters": {
        "newMemoryLimit": "1Gi"
      },
      "dependencies": ["rec-001"],
      "effectivenessProbability": 0.90,
      "historicalSuccessRate": 0.85,
      "riskLevel": "medium",
      "explanation": "Increase memory limit after scaling to provide adequate resources per pod",
      "supportingEvidence": [
        "Similar workloads require 800Mi-1Gi for stable operation"
      ]
    },
    {
      "id": "rec-003",
      "action": "restart_pods",
      "targetResource": {
        "kind": "Pod",
        "namespace": "production",
        "labelSelector": "app=payment-api"
      },
      "parameters": {
        "gracePeriodSeconds": 30
      },
      "dependencies": ["rec-001"],
      "effectivenessProbability": 0.75,
      "historicalSuccessRate": 0.72,
      "riskLevel": "medium",
      "explanation": "Restart pods after scaling to apply new resource settings",
      "supportingEvidence": [
        "Pod restart clears memory leaks and applies new limits"
      ]
    },
    {
      "id": "rec-004",
      "action": "verify_deployment",
      "targetResource": {
        "kind": "Deployment",
        "name": "payment-api",
        "namespace": "production"
      },
      "parameters": {
        "healthCheckEndpoint": "/health"
      },
      "dependencies": ["rec-002", "rec-003"],
      "effectivenessProbability": 0.95,
      "historicalSuccessRate": 0.93,
      "riskLevel": "low",
      "explanation": "Verify deployment health after remediation steps complete",
      "supportingEvidence": [
        "Health verification ensures remediation effectiveness"
      ]
    }
  ]
}
```

**Dependency Analysis**:
- **Batch 1**: rec-001 (scale deployment) - no dependencies, executes first
- **Batch 2**: rec-002 (increase memory) and rec-003 (restart pods) - **PARALLEL EXECUTION** after rec-001
- **Batch 3**: rec-004 (verify) - sequential after rec-002 AND rec-003 complete

---

### **Example 2: Pure Sequential Workflow**

**User Prompt**:
```
Alert: DeploymentRolloutFailed in webapp deployment
Reason: ImagePullBackOff
Image: registry.example.com/webapp:v2.0.0
```

**Expected Response**:
```json
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "check_image_exists",
      "parameters": {
        "imageName": "registry.example.com/webapp:v2.0.0"
      },
      "dependencies": [],
      "explanation": "Verify image exists in registry before attempting fix"
    },
    {
      "id": "rec-002",
      "action": "rollback_deployment",
      "targetResource": {
        "kind": "Deployment",
        "name": "webapp",
        "namespace": "production"
      },
      "parameters": {
        "revision": "previous"
      },
      "dependencies": ["rec-001"],
      "explanation": "Rollback to previous working version if image doesn't exist"
    },
    {
      "id": "rec-003",
      "action": "verify_deployment",
      "targetResource": {
        "kind": "Deployment",
        "name": "webapp",
        "namespace": "production"
      },
      "dependencies": ["rec-002"],
      "explanation": "Verify rollback succeeded and deployment is healthy"
    }
  ]
}
```

**Dependency Analysis**:
- **Pure sequential**: rec-001 → rec-002 → rec-003
- **No parallel opportunities**: Each step depends on previous step completion

---

### **Example 3: Fork-Join Pattern**

**User Prompt**:
```
Alert: Multiple components failing in microservices application
Components: frontend, backend-api, database-proxy
Issue: Network connectivity issues
```

**Expected Response**:
```json
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "restart_network_policy",
      "parameters": {
        "networkPolicy": "app-network-policy",
        "namespace": "production"
      },
      "dependencies": [],
      "explanation": "Reset network policy that may be blocking traffic"
    },
    {
      "id": "rec-002",
      "action": "restart_pods",
      "targetResource": {
        "kind": "Pod",
        "namespace": "production",
        "labelSelector": "app=frontend"
      },
      "dependencies": ["rec-001"],
      "explanation": "Restart frontend pods to re-establish connections"
    },
    {
      "id": "rec-003",
      "action": "restart_pods",
      "targetResource": {
        "kind": "Pod",
        "namespace": "production",
        "labelSelector": "app=backend-api"
      },
      "dependencies": ["rec-001"],
      "explanation": "Restart backend pods to re-establish connections"
    },
    {
      "id": "rec-004",
      "action": "restart_pods",
      "targetResource": {
        "kind": "Pod",
        "namespace": "production",
        "labelSelector": "app=database-proxy"
      },
      "dependencies": ["rec-001"],
      "explanation": "Restart database proxy pods to re-establish connections"
    },
    {
      "id": "rec-005",
      "action": "verify_connectivity",
      "parameters": {
        "testEndpoints": [
          "frontend → backend-api",
          "backend-api → database-proxy"
        ]
      },
      "dependencies": ["rec-002", "rec-003", "rec-004"],
      "explanation": "Verify all components can communicate after restarts"
    }
  ]
}
```

**Dependency Analysis**:
- **Batch 1**: rec-001 (reset network policy) - sequential
- **Batch 2**: rec-002, rec-003, rec-004 (restart all components) - **3-WAY PARALLEL EXECUTION**
- **Batch 3**: rec-005 (verify connectivity) - sequential, waits for all restarts

---

## 🔍 **DEPENDENCY PATTERNS**

### **Pattern 1: Sequential Chain**

```
rec-001 → rec-002 → rec-003
```

```json
[
  {"id": "rec-001", "dependencies": []},
  {"id": "rec-002", "dependencies": ["rec-001"]},
  {"id": "rec-003", "dependencies": ["rec-002"]}
]
```

**Use When**: Each step MUST complete before next step can begin

---

### **Pattern 2: Parallel Execution (Fork)**

```
rec-001
  ├─→ rec-002
  ├─→ rec-003
  └─→ rec-004
```

```json
[
  {"id": "rec-001", "dependencies": []},
  {"id": "rec-002", "dependencies": ["rec-001"]},
  {"id": "rec-003", "dependencies": ["rec-001"]},
  {"id": "rec-004", "dependencies": ["rec-001"]}
]
```

**Use When**: Multiple steps can execute simultaneously after common prerequisite

---

### **Pattern 3: Join (Multiple Prerequisites)**

```
rec-001 ─┐
rec-002 ─┼─→ rec-004
rec-003 ─┘
```

```json
[
  {"id": "rec-001", "dependencies": []},
  {"id": "rec-002", "dependencies": []},
  {"id": "rec-003", "dependencies": []},
  {"id": "rec-004", "dependencies": ["rec-001", "rec-002", "rec-003"]}
]
```

**Use When**: Step requires multiple previous steps to complete first

---

### **Pattern 4: Diamond (Fork + Join)**

```
     rec-001
    /       \
rec-002   rec-003
    \       /
     rec-004
```

```json
[
  {"id": "rec-001", "dependencies": []},
  {"id": "rec-002", "dependencies": ["rec-001"]},
  {"id": "rec-003", "dependencies": ["rec-001"]},
  {"id": "rec-004", "dependencies": ["rec-002", "rec-003"]}
]
```

**Use When**: Parallel steps converge to single final step

---

## ✅ **VALIDATION RULES**

### **Rule 1: Valid Dependency References (BR-AI-051)**

❌ **Invalid**:
```json
[
  {"id": "rec-001", "dependencies": ["rec-999"]}  // rec-999 doesn't exist
]
```

✅ **Valid**:
```json
[
  {"id": "rec-001", "dependencies": []},
  {"id": "rec-002", "dependencies": ["rec-001"]}  // rec-001 exists
]
```

---

### **Rule 2: Acyclic Dependency Graph (BR-AI-052)**

❌ **Invalid** (Circular):
```json
[
  {"id": "rec-001", "dependencies": ["rec-003"]},
  {"id": "rec-002", "dependencies": ["rec-001"]},
  {"id": "rec-003", "dependencies": ["rec-002"]}  // rec-001 → rec-002 → rec-003 → rec-001 (CYCLE)
]
```

✅ **Valid** (Acyclic):
```json
[
  {"id": "rec-001", "dependencies": []},
  {"id": "rec-002", "dependencies": ["rec-001"]},
  {"id": "rec-003", "dependencies": ["rec-002"]}  // rec-001 → rec-002 → rec-003 (NO CYCLE)
]
```

---

### **Rule 3: Self-Reference Prevention**

❌ **Invalid**:
```json
[
  {"id": "rec-001", "dependencies": ["rec-001"]}  // Cannot depend on itself
]
```

✅ **Valid**:
```json
[
  {"id": "rec-001", "dependencies": []}
]
```

---

## 🚨 **COMMON MISTAKES**

### **Mistake 1: Missing Dependencies Field**

❌ **Wrong**:
```json
{
  "id": "rec-001",
  "action": "scale_deployment"
  // Missing dependencies field!
}
```

✅ **Correct**:
```json
{
  "id": "rec-001",
  "action": "scale_deployment",
  "dependencies": []  // REQUIRED: Include even if empty
}
```

---

### **Mistake 2: Sequential When Parallel Possible**

❌ **Suboptimal**:
```json
[
  {"id": "rec-001", "dependencies": []},
  {"id": "rec-002", "dependencies": ["rec-001"]},
  {"id": "rec-003", "dependencies": ["rec-002"]}  // Why does rec-003 depend on rec-002?
]
```

✅ **Optimized** (if rec-002 and rec-003 can run in parallel):
```json
[
  {"id": "rec-001", "dependencies": []},
  {"id": "rec-002", "dependencies": ["rec-001"]},
  {"id": "rec-003", "dependencies": ["rec-001"]}  // rec-002 and rec-003 run in parallel
]
```

---

### **Mistake 3: Unnecessary Dependencies**

❌ **Over-constrained**:
```json
[
  {"id": "rec-001", "dependencies": []},
  {"id": "rec-002", "dependencies": []},
  {"id": "rec-003", "dependencies": ["rec-001", "rec-002"]}  // Does rec-003 really need both?
]
```

✅ **Minimal Dependencies**:
```json
[
  {"id": "rec-001", "dependencies": []},
  {"id": "rec-002", "dependencies": []},
  {"id": "rec-003", "dependencies": ["rec-002"]}  // Only necessary dependency
]
```

---

## 🎯 **IMPLEMENTATION CHECKLIST**

### **For Prompt Engineering**:
- [ ] Include dependency specification instructions in system prompt (BR-LLM-035)
- [ ] Provide JSON schema with dependencies field (BR-LLM-037)
- [ ] Include examples showing sequential, parallel, and mixed patterns (BR-LLM-036)
- [ ] Add validation rules to prompt (acyclic graph, valid references)
- [ ] Specify error handling for missing/invalid dependencies

### **For AIAnalysis Service**:
- [ ] Validate dependency completeness (BR-AI-051)
- [ ] Detect circular dependencies (BR-AI-052)
- [ ] Handle missing dependencies with fallback (BR-AI-053)
- [ ] Log validation failures for debugging
- [ ] Notify via Notification Service on validation errors

### **For Testing**:
- [ ] Test sequential workflow (A → B → C)
- [ ] Test parallel workflow (A → [B, C])
- [ ] Test diamond pattern (A → [B, C] → D)
- [ ] Test circular dependency detection
- [ ] Test invalid dependency reference handling

---

## 📚 **REFERENCES**

- **Business Requirements**:
  - BR-LLM-035: Instruct LLM to generate dependencies
  - BR-LLM-036: Request execution order specification
  - BR-LLM-037: Define response schema with dependencies
  - BR-HOLMES-031: Include step dependencies
  - BR-HOLMES-032: Specify execution relationships
  - BR-HOLMES-033: Dependency graph validation
  - BR-AI-051: Validate dependency completeness
  - BR-AI-052: Detect circular dependencies
  - BR-AI-053: Handle missing/invalid dependencies

- **Related Documents**:
  - `docs/analysis/WORKFLOW_EXECUTION_MODE_DETERMINATION.md`
  - `docs/analysis/HOLMESGPT_DEPENDENCY_SPECIFICATION_ASSESSMENT.md`
  - `docs/analysis/AI_TO_WORKFLOW_DETAILED_FLOW.md`
  - `docs/services/crd-controllers/02-aianalysis/crd-schema.md`

---

**Document Status**: ✅ **COMPLETE** - Comprehensive prompt engineering guidelines for dependency specification
