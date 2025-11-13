# BR-REMEDIATION-017: Track AI Execution Mode (Hybrid Model)

**Business Requirement ID**: BR-REMEDIATION-017
**Category**: RemediationExecutor Service
**Priority**: P1
**Target Version**: V1
**Status**: ✅ Approved
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 defines a **Hybrid Model** for AI capabilities: 90% catalog-based selection, 9% playbook chaining, 1% manual escalation. The RemediationExecutor Service must track which execution mode was used for each remediation to enable analysis of AI decision patterns and validate the 90-9-1 distribution target.

**Current Limitations**:
- ❌ No tracking of AI execution mode (catalog selection vs chaining vs manual)
- ❌ Cannot validate if AI adheres to ADR-033 Hybrid Model distribution
- ❌ Cannot analyze which execution modes are most effective
- ❌ No visibility into AI decision-making patterns
- ❌ Cannot identify if AI is over-relying on manual escalation

**Impact**:
- Cannot validate ADR-033 architectural compliance (90-9-1 target)
- Missing data for AI algorithm optimization
- No insights into when/why AI escalates to manual intervention
- Cannot measure effectiveness of chained playbooks vs single playbooks

---

## 🎯 **Business Objective**

**Track AI execution mode (catalog selection, playbook chaining, manual escalation) in audit records to enable validation of ADR-033 Hybrid Model and analysis of AI decision patterns.**

### **Success Criteria**
1. ✅ RemediationExecutor populates `ai_selected_playbook`, `ai_chained_playbooks`, `ai_manual_escalation` flags
2. ✅ Populates `ai_playbook_customization` JSONB field for AI parameter adjustments
3. ✅ 100% of AI-driven remediations have execution mode flags populated
4. ✅ Execution mode distribution measured and compared against ADR-033 targets
5. ✅ Dashboard displays execution mode breakdown (catalog: 90%, chained: 9%, manual: 1%)
6. ✅ Alerting if manual escalation rate exceeds 5% (indicates AI effectiveness issue)
7. ✅ Historical trend analysis of execution mode distribution

---

## 📊 **Use Cases**

### **Use Case 1: AI Catalog-Based Selection (90% of cases)**

**Scenario**: AI selects single playbook `pod-oom-recovery v1.2` from catalog.

**Current Flow** (Without BR-REMEDIATION-017):
```
1. AI queries Playbook Catalog for pod-oom-killer incident
2. AI selects pod-oom-recovery v1.2 (highest success rate)
3. RemediationExecutor executes playbook
4. RemediationExecutor creates audit:
   {
     "playbook_id": "pod-oom-recovery",
     "playbook_version": "v1.2",
     "ai_selected_playbook": null,  ← ❌ NULL
     "ai_chained_playbooks": null,  ← ❌ NULL
     "ai_manual_escalation": null   ← ❌ NULL
   }
5. ❌ Cannot distinguish catalog selection from chained or manual
6. ❌ Cannot validate 90% catalog selection target
```

**Desired Flow with BR-REMEDIATION-017**:
```
1. AI selects pod-oom-recovery v1.2 (catalog-based)
2. AI provides execution context to RemediationExecutor:
   {
     "execution_mode": "catalog_selection",
     "playbook_id": "pod-oom-recovery",
     "playbook_version": "v1.2"
   }
3. RemediationExecutor executes playbook
4. RemediationExecutor creates audit:
   {
     "playbook_id": "pod-oom-recovery",
     "playbook_version": "v1.2",
     "ai_selected_playbook": true,   ← ✅ CATALOG SELECTION
     "ai_chained_playbooks": false,
     "ai_manual_escalation": false
   }
5. ✅ Execution mode tracked: catalog_selection
6. ✅ Data Storage can aggregate: catalog_selection rate = 90.2%
7. ✅ Validates ADR-033 target (90%)
```

---

### **Use Case 2: AI Playbook Chaining (9% of cases)**

**Scenario**: AI chains two playbooks for complex remediation: `scale-deployment v1.0` → `restart-pods v1.1`.

**Current Flow**:
```
1. AI determines single playbook insufficient
2. AI chains: scale-deployment → restart-pods
3. RemediationExecutor executes both playbooks
4. ❌ Audits show two separate executions (no chaining indication)
5. ❌ Cannot measure chained playbook effectiveness
```

**Desired Flow with BR-REMEDIATION-017**:
```
1. AI determines chaining required
2. AI provides execution context:
   {
     "execution_mode": "playbook_chaining",
     "chained_playbooks": [
       {"playbook_id": "scale-deployment", "version": "v1.0"},
       {"playbook_id": "restart-pods", "version": "v1.1"}
     ],
     "chaining_rationale": "Scale first to prevent cascading failures"
   }
3. RemediationExecutor executes both playbooks with shared execution_id
4. RemediationExecutor creates audits:
   - Audit 1 (scale-deployment):
     {
       "playbook_id": "scale-deployment",
       "playbook_version": "v1.0",
       "ai_selected_playbook": false,
       "ai_chained_playbooks": true,  ← ✅ CHAINED
       "ai_manual_escalation": false,
       "playbook_execution_id": "exec-chain-123"
     }
   - Audit 2 (restart-pods):
     {
       "playbook_id": "restart-pods",
       "playbook_version": "v1.1",
       "ai_selected_playbook": false,
       "ai_chained_playbooks": true,  ← ✅ CHAINED
       "ai_manual_escalation": false,
       "playbook_execution_id": "exec-chain-123"  ← SAME ID
     }
5. ✅ Data Storage groups chained playbooks by execution_id
6. ✅ Team can analyze: chained playbook success rate vs single playbook
7. ✅ Validates ADR-033 target (9%)
```

---

### **Use Case 3: AI Manual Escalation (1% of cases)**

**Scenario**: AI cannot find suitable playbook for rare incident, escalates to human operator.

**Current Flow**:
```
1. AI receives rare incident: "database-corruption-rare"
2. No playbooks in catalog for this incident type
3. AI escalates to operator
4. Operator executes manual remediation
5. ❌ No indication this was AI escalation (vs direct manual action)
6. ❌ Cannot measure AI escalation rate
```

**Desired Flow with BR-REMEDIATION-017**:
```
1. AI receives rare incident
2. AI determines no suitable playbook exists
3. AI provides execution context:
   {
     "execution_mode": "manual_escalation",
     "escalation_reason": "No playbook found for incident_type=database-corruption-rare",
     "ai_recommendations": [
       "Check database logs for corruption patterns",
       "Consider restore from last known good backup"
     ]
   }
4. Operator executes manual remediation
5. RemediationExecutor creates audit:
   {
     "playbook_id": "manual-remediation",
     "playbook_version": "v1.0",
     "ai_selected_playbook": false,
     "ai_chained_playbooks": false,
     "ai_manual_escalation": true,  ← ✅ MANUAL ESCALATION
     "ai_playbook_customization": {
       "escalation_reason": "No playbook found",
       "ai_recommendations": [...]
     }
   }
6. ✅ Team can analyze: manual escalation rate = 0.8%
7. ✅ Validates ADR-033 target (<1%)
8. ✅ Team identifies: Need to create playbook for database-corruption incidents
```

---

### **Use Case 4: AI Parameter Customization**

**Scenario**: AI selects `scale-deployment v1.0` but customizes memory increase from 1.2x to 1.5x based on historical data.

**Current Flow**:
```
1. AI selects scale-deployment playbook (default: 1.2x memory increase)
2. AI customizes: 1.5x memory increase (historical data suggests higher success rate)
3. RemediationExecutor executes with custom parameters
4. ❌ No tracking of AI customization (looks like default playbook execution)
5. ❌ Cannot analyze effectiveness of AI customizations
```

**Desired Flow with BR-REMEDIATION-017**:
```
1. AI selects scale-deployment playbook
2. AI provides execution context:
   {
     "execution_mode": "catalog_selection",
     "playbook_id": "scale-deployment",
     "playbook_version": "v1.0",
     "customization": {
       "memory_multiplier": 1.5,  // AI customized from default 1.2
       "customization_reason": "Historical data shows 1.5x has 95% success rate vs 85% for 1.2x"
     }
   }
3. RemediationExecutor executes with custom parameters
4. RemediationExecutor creates audit:
   {
     "playbook_id": "scale-deployment",
     "playbook_version": "v1.0",
     "ai_selected_playbook": true,
     "ai_playbook_customization": {
       "memory_multiplier": 1.5,
       "customization_reason": "Historical data optimization"
     }
   }
5. ✅ Data Storage tracks AI customization patterns
6. ✅ Team can analyze: AI customizations improve success rate by 10%
7. ✅ Team can promote successful customizations to playbook defaults
```

---

## 🔧 **Functional Requirements**

### **FR-REMEDIATION-017-01: Execution Mode Flag Population**

**Requirement**: RemediationExecutor SHALL populate AI execution mode flags based on AI decision context.

**Implementation Example**:
```go
package remediationexecutor

// AIExecutionMode represents AI decision mode per ADR-033 Hybrid Model
type AIExecutionMode struct {
    CatalogSelection bool   // True if AI selected single playbook from catalog (90%)
    ChainedPlaybooks bool   // True if AI chained multiple playbooks (9%)
    ManualEscalation bool   // True if AI escalated to human operator (1%)
}

// ExtractAIExecutionMode extracts execution mode from AI decision context
func ExtractAIExecutionMode(aiContext *AIDecisionContext) AIExecutionMode {
    switch aiContext.ExecutionMode {
    case "catalog_selection":
        return AIExecutionMode{CatalogSelection: true}
    case "playbook_chaining":
        return AIExecutionMode{ChainedPlaybooks: true}
    case "manual_escalation":
        return AIExecutionMode{ManualEscalation: true}
    default:
        // Fallback: Assume catalog selection (most common)
        return AIExecutionMode{CatalogSelection: true}
    }
}

// PopulateAIExecutionMode populates AI execution mode fields in audit
func (r *RemediationExecutor) PopulateAIExecutionMode(audit *datastorage.NotificationAudit, aiContext *AIDecisionContext) {
    mode := ExtractAIExecutionMode(aiContext)

    audit.AISelectedPlaybook = mode.CatalogSelection
    audit.AIChainedPlaybooks = mode.ChainedPlaybooks
    audit.AIManualEscalation = mode.ManualEscalation

    // Populate customization JSONB if AI customized playbook parameters
    if aiContext.Customization != nil {
        customizationJSON, _ := json.Marshal(aiContext.Customization)
        audit.AIPlaybookCustomization = customizationJSON
    }
}
```

**Acceptance Criteria**:
- ✅ Exactly ONE of the three flags is true (mutually exclusive modes)
- ✅ `ai_selected_playbook=true` for catalog-based selection
- ✅ `ai_chained_playbooks=true` for playbook chaining
- ✅ `ai_manual_escalation=true` for manual escalation
- ✅ All three flags default to false for non-AI remediations

---

### **FR-REMEDIATION-017-02: AI Customization Tracking**

**Requirement**: RemediationExecutor SHALL populate `ai_playbook_customization` JSONB field for AI parameter adjustments.

**Customization Schema**:
```json
{
  "customized_parameters": {
    "memory_multiplier": 1.5,
    "replicas": 5,
    "timeout_seconds": 300
  },
  "customization_reason": "Historical data shows 1.5x memory increase has 95% success rate",
  "default_parameters": {
    "memory_multiplier": 1.2,
    "replicas": 3,
    "timeout_seconds": 120
  }
}
```

**Acceptance Criteria**:
- ✅ JSONB field captures customized parameters (key-value pairs)
- ✅ Includes customization_reason (AI rationale)
- ✅ Includes default_parameters for comparison
- ✅ Field is NULL if no AI customization applied

---

### **FR-REMEDIATION-017-03: Chained Playbook Execution ID**

**Requirement**: RemediationExecutor SHALL use shared `playbook_execution_id` for all steps in chained playbook execution.

**Implementation**:
```go
// ExecuteChainedPlaybooks executes multiple playbooks with shared execution_id
func (r *RemediationExecutor) ExecuteChainedPlaybooks(ctx context.Context, chain []PlaybookRef) error {
    // Generate shared execution_id for entire chain
    sharedExecutionID := fmt.Sprintf("exec-chain-%s", uuid.New().String()[:8])

    for i, playbookRef := range chain {
        executionCtx := &ExecutionContext{
            PlaybookID:        playbookRef.PlaybookID,
            PlaybookVersion:   playbookRef.Version,
            ExecutionID:       sharedExecutionID,  // SHARED across chain
            CurrentStepNumber: i + 1,
        }

        // Execute playbook and create audit
        if err := r.ExecutePlaybook(ctx, playbookRef, executionCtx); err != nil {
            return fmt.Errorf("chained playbook %d failed: %w", i+1, err)
        }
    }

    return nil
}
```

**Acceptance Criteria**:
- ✅ All playbooks in chain share same `playbook_execution_id`
- ✅ Execution ID format: `exec-chain-{uuid}`
- ✅ Data Storage can GROUP BY execution_id to reconstruct chain
- ✅ Chain order preserved via step_number or timestamp

---

## 📈 **Non-Functional Requirements**

### **NFR-REMEDIATION-017-01: Performance**

- ✅ Execution mode flag population adds <5ms latency
- ✅ JSONB serialization for customization <10ms
- ✅ No impact on remediation execution time

### **NFR-REMEDIATION-017-02: Data Accuracy**

- ✅ Execution mode flags accurately reflect AI decision
- ✅ Customization JSONB is valid JSON (schema-validated)
- ✅ Manual escalation reason captured for audit trail

### **NFR-REMEDIATION-017-03: Observability**

- ✅ Prometheus metrics: `remediation_executor_ai_mode_total{mode="catalog|chained|manual"}`
- ✅ Alerting: If `manual` mode >5%, trigger investigation alert
- ✅ Dashboard: Real-time execution mode distribution (90-9-1 gauge)

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Hybrid Model definition (90-9-1 distribution)
- ✅ BR-STORAGE-031-03: Schema migration (ai_selected_playbook, ai_chained_playbooks, ai_manual_escalation columns)
- ✅ AI Service: Provides execution mode in decision context

### **Downstream Impacts**
- ✅ BR-STORAGE-031-04: Data Storage exposes AI execution mode in aggregation responses
- ✅ BR-EFFECTIVENESS-002: Effectiveness Monitor analyzes execution mode effectiveness
- ✅ Operations Dashboard: Displays execution mode breakdown

---

## 🚀 **Implementation Phases**

### **Phase 1: Execution Mode Extraction** (Day 11 - 3 hours)
- Implement `ExtractAIExecutionMode()` function
- Add `PopulateAIExecutionMode()` helper
- Unit tests for mode extraction (6+ test cases)

### **Phase 2: Customization Tracking** (Day 11 - 2 hours)
- Implement JSONB serialization for `ai_playbook_customization`
- Add schema validation for customization JSON
- Unit tests for customization tracking

### **Phase 3: Chained Playbook Support** (Day 12 - 3 hours)
- Implement shared execution_id for chains
- Update `ExecuteChainedPlaybooks()` to populate flags
- Integration tests with real Data Storage

### **Phase 4: Monitoring & Alerting** (Day 12 - 2 hours)
- Add Prometheus metrics for execution mode distribution
- Add alerting for high manual escalation rate (>5%)
- Dashboard visualization for 90-9-1 distribution

**Total Estimated Effort**: 10 hours (1.25 days)

---

## 📊 **Success Metrics**

### **ADR-033 Hybrid Model Compliance**
- **Target**: Execution mode distribution: 90% catalog, 9% chained, 1% manual
- **Measure**: Aggregate `ai_selected_playbook`, `ai_chained_playbooks`, `ai_manual_escalation` flags

### **Manual Escalation Rate**
- **Target**: <1% manual escalation rate
- **Measure**: `remediation_executor_ai_mode_total{mode="manual"}` / total executions
- **Alert**: If >5%, trigger investigation

### **AI Customization Effectiveness**
- **Target**: AI customizations improve success rate by 5%+
- **Measure**: Compare success rate (customized vs default parameters)

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Single Execution Mode Enum**

**Approach**: Use single `execution_mode` enum field instead of 3 boolean flags

**Rejected Because**:
- ❌ Harder to query (WHERE execution_mode='catalog' vs WHERE ai_selected_playbook=true)
- ❌ Cannot support future hybrid modes (e.g., catalog+customization)
- ❌ Less flexible for aggregation queries

---

### **Alternative 2: No AI Customization Tracking**

**Approach**: Track execution mode but not AI parameter customizations

**Rejected Because**:
- ❌ Cannot measure AI customization effectiveness
- ❌ Loss of valuable optimization insights
- ❌ Cannot promote successful customizations to playbook defaults

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P1 priority (validates ADR-033 Hybrid Model compliance)
**Rationale**: Required to measure AI decision patterns and validate 90-9-1 distribution target
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-STORAGE-031-04: Data Storage exposes AI execution mode in aggregation
- BR-REMEDIATION-016: Populate playbook metadata
- BR-EFFECTIVENESS-002: Effectiveness Monitor analyzes execution mode trends

### **Related Documents**
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-037: BR Template Standard](../architecture/decisions/ADR-037-business-requirement-template-standard.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

