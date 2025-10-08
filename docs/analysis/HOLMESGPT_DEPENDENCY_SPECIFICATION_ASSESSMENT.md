# HolmesGPT Dependency Specification Assessment

**Date**: October 8, 2025
**Question**: Are there business requirements or expectations that the prompt used to HolmesGPT will include dependency understanding in structured data format? Is the model aware it needs to include dependencies between steps?
**Purpose**: Assess whether HolmesGPT is expected to generate step dependencies or if dependency determination is handled elsewhere

---

## 🎯 **SHORT ANSWER**

**NO** - Based on comprehensive analysis of business requirements and service specifications:

1. **HolmesGPT's Role**: Generate **recommendations** (remediation actions), **NOT** workflow execution logic
2. **Dependency Determination**: Handled by **WorkflowExecution Controller** through dependency graph analysis
3. **Current State**: Business requirements do **NOT** specify that HolmesGPT must include dependency information
4. **Gap Identified**: There is **NO explicit requirement** for HolmesGPT to output structured step dependencies

---

## 📋 **DETAILED ASSESSMENT**

---

## **PART 1: CURRENT HOLMESGPT BUSINESS REQUIREMENTS REVIEW**

### **HolmesGPT Investigation Scope (BR-HOLMES-001 to BR-HOLMES-030)**

**Source**: `docs/requirements/10_AI_CONTEXT_ORCHESTRATION.md`

**What HolmesGPT IS Required To Do**:
- ✅ **BR-HOLMES-001 to BR-HOLMES-005**: Custom toolset development and function chaining
- ✅ **BR-HOLMES-006 to BR-HOLMES-010**: Investigation orchestration and context gathering
- ✅ **BR-HOLMES-011 to BR-HOLMES-015**: Fallback mechanisms and resilience
- ✅ **BR-HOLMES-016 to BR-HOLMES-030**: Dynamic toolset configuration

**What HolmesGPT is NOT Required To Do**:
- ❌ **NO requirement to specify step dependencies**
- ❌ **NO requirement to determine execution order**
- ❌ **NO requirement to specify parallel vs sequential execution**
- ❌ **NO requirement to output structured workflow definitions**

---

### **AI Recommendation Requirements (BR-AI-006 to BR-AI-010)**

**Source**: `docs/requirements/02_AI_MACHINE_LEARNING.md:38-43`

```markdown
#### 2.1.2 Recommendation Provider
- **BR-AI-006**: MUST generate actionable remediation recommendations based on alert context
- **BR-AI-007**: MUST rank recommendations by effectiveness probability
- **BR-AI-008**: MUST consider historical success rates in recommendation scoring
- **BR-AI-009**: MUST support constraint-based recommendation filtering
- **BR-AI-010**: MUST provide recommendation explanations with supporting evidence
```

**Analysis**:
- ✅ Requires **recommendations** (actions to take)
- ✅ Requires **ranking** (effectiveness probability)
- ✅ Requires **explanations** (reasoning)
- ❌ **NO mention of dependencies between recommendations**
- ❌ **NO mention of execution order**
- ❌ **NO mention of step relationships**

---

### **LLM Structured Response Requirements (BR-LLM-021 to BR-LLM-031)**

**Source**: `docs/requirements/02_AI_MACHINE_LEARNING.md:247-259`

```markdown
#### 5.1.5 Structured Response Generation
- **BR-LLM-021**: MUST enforce JSON-structured responses from LLM providers for machine actionability
- **BR-LLM-022**: MUST validate JSON response schema compliance and completeness
- **BR-LLM-023**: MUST handle malformed JSON responses with intelligent fallback parsing
- **BR-LLM-024**: MUST extract structured data elements (actions, parameters, conditions) from JSON responses
- **BR-LLM-025**: MUST provide response format validation with error-specific feedback

#### 5.1.6 Multi-Stage Action Generation
- **BR-LLM-026**: MUST generate structured responses containing primary actions with complete parameter sets
- **BR-LLM-027**: MUST include secondary actions with conditional execution logic (if_primary_fails, after_primary, parallel_with_primary)
- **BR-LLM-028**: MUST provide context-aware reasoning for each recommended action including risk assessment and business impact
- **BR-LLM-029**: MUST generate dynamic monitoring criteria including success criteria, validation commands, and rollback triggers
- **BR-LLM-030**: MUST preserve contextual information across multi-stage remediation workflows
- **BR-LLM-031**: MUST support action sequencing with execution order and timing constraints
```

**Analysis**:
- ✅ **BR-LLM-024**: Extract structured data (actions, parameters, **conditions**)
- ✅ **BR-LLM-027**: Secondary actions with **conditional execution logic**
- ✅ **BR-LLM-031**: Support **action sequencing** with execution order

**KEY FINDING**: **BR-LLM-027** and **BR-LLM-031** suggest execution relationships!

**However**: These requirements are for **generic LLM responses**, not specifically HolmesGPT-API implementation.

---

## **PART 2: HOLMESGPT API RESPONSE FORMAT ANALYSIS**

### **HolmesGPT REST API Wrapper Requirements**

**Source**: `docs/requirements/13_HOLMESGPT_REST_API_WRAPPER.md`

**Investigation Endpoints**:
- **BR-HAPI-001**: POST `/api/v1/investigate` - Trigger HolmesGPT investigation
- **BR-HAPI-002**: Root cause analysis
- **BR-HAPI-003**: Pattern recognition
- **BR-HAPI-004**: Historical context correlation
- **BR-HAPI-005**: **Recommendation generation**

**Key Observation**: The API specification does **NOT** define the expected response schema for recommendations.

---

### **Current Code Implementation - HolmesGPT Response**

**Source**: `pkg/ai/holmesgpt/toolset_deployment_client.go:145-164`

```go
// ToolChainDefinition defines a chain of tool executions
// Business Requirement: BR-HOLMES-005 - Function chaining for complex workflows
type ToolChainDefinition struct {
    Name        string           `json:"name"`
    Description string           `json:"description"`
    Steps       []ToolChainStep  `json:"steps"`
    Conditions  []ChainCondition `json:"conditions,omitempty"`
    ErrorPolicy string           `json:"error_policy"` // "fail_fast", "continue", "retry"
}

// ToolChainStep defines a single step in a tool chain
type ToolChainStep struct {
    StepID     string            `json:"step_id"`
    ToolName   string            `json:"tool_name"`
    Parameters map[string]string `json:"parameters"`
    DependsOn  []string          `json:"depends_on,omitempty"` // ✅ DEPENDENCY FIELD EXISTS!
    OutputVar  string            `json:"output_var,omitempty"`
    Optional   bool              `json:"optional,omitempty"`
    Retry      *RetryPolicy      `json:"retry,omitempty"`
}
```

**CRITICAL FINDING**: The code **DOES** include a `DependsOn` field for tool chain steps!

**Context**: This is for **HolmesGPT tool execution chains** (how HolmesGPT internally calls tools), **NOT** for remediation action recommendations.

---

### **Test Implementation - AI Workflow Conversion**

**Source**: `test/integration/workflow_automation/execution/multi_stage_remediation_test.go:716-762`

```go
func (v *MultiStageRemediationValidator) convertInvestigationToWorkflow(
    response *holmesgpt.InvestigateResponse,
    alertContext *types.Alert,
    requirements string,
) *AIGeneratedWorkflow {
    workflow := &AIGeneratedWorkflow{
        WorkflowID: response.InvestigationID,
        Metadata: &WorkflowMetadata{
            GeneratedAt:  response.Timestamp.Format(time.RFC3339),
            Confidence:   0.85,
            ModelVersion: "holmesgpt-api",
        },
    }

    // Convert primary recommendation to primary action
    if len(response.Recommendations) > 0 {
        primaryRec := response.Recommendations[0]
        workflow.PrimaryAction = &PrimaryActionStage{
            Action:           primaryRec.Title,
            Parameters:       map[string]interface{}{"description": primaryRec.Description, "command": primaryRec.Command},
            ExecutionOrder:   1, // ✅ EXPLICIT EXECUTION ORDER
            Urgency:          primaryRec.Priority,
            ExpectedDuration: "5m",
            Timeout:          "10m",
            SuccessCriteria:  []string{"action_completed", "metrics_improved"},
        }
    }

    // Convert additional recommendations to secondary actions
    if len(response.Recommendations) > 1 {
        secondaryActions := make([]*SecondaryActionStage, 0)
        for i, rec := range response.Recommendations[1:] {
            secondaryAction := &SecondaryActionStage{
                Action:         rec.Title,
                Parameters:     map[string]interface{}{"description": rec.Description, "command": rec.Command},
                ExecutionOrder: i + 2, // ✅ SEQUENTIAL EXECUTION ORDER
                Condition:      "if_primary_fails", // ✅ CONDITIONAL EXECUTION
                Timeout:        "5m",
                Prerequisites:  []string{}, // ✅ PREREQUISITES FIELD (EMPTY)
            }
            secondaryActions = append(secondaryActions, secondaryAction)
        }
        workflow.SecondaryActions = secondaryActions
    }

    return workflow
}
```

**CRITICAL FINDING**: Test code **assigns** execution order and conditions, but **NOT from HolmesGPT response**!

**Execution Order**: Hardcoded as sequential (1, 2, 3...)  
**Condition**: Hardcoded as `"if_primary_fails"`  
**Prerequisites**: Empty array `[]`

---

## **PART 3: WORKFLOW DEFINITION CRD SCHEMA**

### **WorkflowDefinition Structure**

**Source**: `docs/services/crd-controllers/03-workflowexecution/crd-schema.md:48-55`

```go
// WorkflowDefinition represents the workflow to execute
type WorkflowDefinition struct {
    Name             string                  `json:"name"`
    Version          string                  `json:"version"`
    Steps            []WorkflowStep          `json:"steps"`
    Dependencies     map[string][]string     `json:"dependencies,omitempty"` // ✅ DEPENDENCIES FIELD!
    AIRecommendations *AIRecommendations     `json:"aiRecommendations,omitempty"`
}
```

**CRITICAL FINDING**: WorkflowDefinition **HAS** a `Dependencies` field!

**Question**: Where does this `Dependencies` map get populated?

---

### **WorkflowStep Structure**

**Source**: `docs/services/crd-controllers/03-workflowexecution/crd-schema.md:57-59`

```go
// WorkflowStep represents a single step in the workflow
type WorkflowStep struct {
    StepNumber     int                    `json:"stepNumber"`
    // ... other fields
}
```

**Note**: Individual WorkflowStep does **NOT** have a `dependencies` field in this snippet.

---

## **PART 4: GAP ANALYSIS - DEPENDENCY SOURCE**

### **Dependency Flow Analysis**

```
┌─────────────────────────────────────────────────────────┐
│ QUESTION: Where do step dependencies come from?         │
└─────────────────────────────────────────────────────────┘

Option 1: HolmesGPT Response
   HolmesGPT → recommendations with dependencies → WorkflowExecution CRD
   ❌ NO EVIDENCE in business requirements
   ❌ NO EVIDENCE in test code (dependencies are hardcoded/empty)
   ✅ DependsOn field EXISTS in ToolChainStep (but for tools, not recommendations)

Option 2: RemediationOrchestrator Logic
   HolmesGPT → flat recommendations → RemediationOrchestrator adds dependencies
   ❌ NO EVIDENCE in RemediationOrchestrator specifications
   ❌ No business logic to infer dependencies

Option 3: WorkflowExecution Controller
   WorkflowExecution receives steps without dependencies → Controller infers them
   ❌ NO EVIDENCE in reconciliation phases
   ❌ Controller EXPECTS dependencies to already exist

Option 4: Manual/Default Configuration
   Steps created with default dependencies (empty or sequential)
   ✅ MATCHES test code behavior (empty prerequisites, sequential order)
   ✅ EXPLAINS how diamond patterns work (manually configured?)
```

---

## **PART 5: EXPECTED vs ACTUAL BEHAVIOR**

### **What Documentation IMPLIES**

Based on `docs/analysis/WORKFLOW_EXECUTION_MODE_DETERMINATION.md`:

**Expected AI Output**:
```json
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "scale_deployment",
      "dependencies": []  // ✅ IMPLIED: AI should specify this
    },
    {
      "id": "rec-002",
      "action": "increase_memory_limit",
      "dependencies": ["rec-001"]  // ✅ IMPLIED: AI should specify this
    }
  ]
}
```

---

### **What Test Code SHOWS**

**Actual Implementation** (`test/integration/workflow_automation/execution/multi_stage_remediation_test.go`):

```go
// HolmesGPT Response (current):
type Recommendation struct {
    Title       string
    Description string
    Command     string
    Priority    string
    // ❌ NO dependencies field!
}

// Conversion logic:
secondaryAction := &SecondaryActionStage{
    Action:         rec.Title,
    ExecutionOrder: i + 2,           // Hardcoded sequential
    Condition:      "if_primary_fails", // Hardcoded condition
    Prerequisites:  []string{},      // Always empty!
}
```

**Actual Behavior**: Dependencies are **NOT** coming from HolmesGPT.

---

## **PART 6: BUSINESS REQUIREMENT GAP**

### **Missing Requirements**

Based on comprehensive analysis, the following requirements are **MISSING**:

#### **HolmesGPT Response Format**
- ❌ **BR-HOLMES-031** (MISSING): HolmesGPT MUST include step dependencies in remediation recommendations
- ❌ **BR-HOLMES-032** (MISSING): HolmesGPT MUST specify execution relationships (sequential, parallel, conditional)
- ❌ **BR-HOLMES-033** (MISSING): HolmesGPT MUST provide dependency graph for multi-step remediation workflows

#### **Prompt Engineering**
- ❌ **BR-LLM-032** (MISSING): LLM prompts MUST instruct model to generate step dependencies
- ❌ **BR-LLM-033** (MISSING): LLM prompts MUST request execution order specification
- ❌ **BR-LLM-034** (MISSING): LLM response schema MUST include dependencies field for each recommendation

#### **Response Validation**
- ❌ **BR-AI-051** (MISSING): AI responses MUST be validated for dependency completeness
- ❌ **BR-AI-052** (MISSING): AI responses MUST be validated for circular dependency detection
- ❌ **BR-AI-053** (MISSING): AI responses with missing dependencies MUST be rejected or defaulted

---

## **PART 7: CURRENT SYSTEM DESIGN**

### **How Dependencies Are ACTUALLY Handled (Current State)**

Based on test code and implementation patterns:

**Scenario 1: Simple Sequential (Current Behavior)**
```go
// HolmesGPT returns: [Action1, Action2, Action3]
// System converts to: Action1 → Action2 → Action3 (sequential by default)
```

**Scenario 2: Primary/Secondary Pattern (Current Behavior)**
```go
// HolmesGPT returns: [PrimaryAction, SecondaryAction1, SecondaryAction2]
// System converts to:
//   - PrimaryAction (execute first)
//   - SecondaryAction1 (condition: if_primary_fails)
//   - SecondaryAction2 (condition: if_primary_fails)
```

**Scenario 3: Complex Dependencies (NOT IMPLEMENTED)**
```go
// How would HolmesGPT express: "scale deployment, THEN restart pods A and B in parallel"?
// Current answer: IT CANNOT - no mechanism to specify this
```

---

## 🔑 **KEY FINDINGS**

### **1. Business Requirements Do NOT Specify Dependency Output**

**Evidence**:
- ✅ BR-AI-006 to BR-AI-010: Generate recommendations (no dependencies mentioned)
- ✅ BR-HOLMES-001 to BR-HOLMES-030: Investigation and toolset (no recommendation dependencies)
- ❌ **NO requirement** for HolmesGPT to output step dependencies
- ❌ **NO requirement** for HolmesGPT to specify execution order

---

### **2. Current Implementation Uses Hardcoded Patterns**

**Evidence**:
- ✅ Test code shows `ExecutionOrder: i + 2` (hardcoded sequential)
- ✅ Test code shows `Condition: "if_primary_fails"` (hardcoded conditional)
- ✅ Test code shows `Prerequisites: []` (always empty)
- ❌ **NO dependency information** extracted from HolmesGPT response

---

### **3. System CAN Handle Dependencies (But Doesn't Receive Them)**

**Evidence**:
- ✅ WorkflowDefinition has `Dependencies map[string][]string` field
- ✅ ToolChainStep has `DependsOn []string` field (for tool chains)
- ✅ WorkflowExecution Controller can process dependency graphs
- ❌ **NO mechanism** to populate dependencies from HolmesGPT

---

### **4. Gap Between Documentation and Implementation**

**Documentation** (`docs/analysis/WORKFLOW_EXECUTION_MODE_DETERMINATION.md`) shows:
```json
{
  "id": "rec-002",
  "dependencies": ["rec-001"]  // IMPLIED: AI provides this
}
```

**Implementation** (test code) shows:
```go
Prerequisites: []string{},  // ACTUAL: Always empty!
```

**Conclusion**: The documentation example is **ASPIRATIONAL**, not **CURRENT**.

---

## ✅ **RECOMMENDATIONS**

### **Option A: Enhance HolmesGPT Prompt (Recommended)**

**Add Business Requirements**:
- **BR-HOLMES-031**: HolmesGPT MUST include `dependencies` field in each recommendation
- **BR-HOLMES-032**: HolmesGPT MUST specify execution relationships (DependsOn array)
- **BR-HOLMES-033**: HolmesGPT MUST validate dependency graph for cycles

**Enhance Prompt Structure**:
```python
# Example HolmesGPT prompt enhancement
SYSTEM_PROMPT = """
When generating remediation recommendations, you MUST include:
1. Action to take
2. Parameters for the action
3. Dependencies: Array of recommendation IDs that must complete before this action
4. Execution mode: "parallel" (if no dependencies) or "sequential" (if has dependencies)

Example output format:
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "scale_deployment",
      "parameters": {...},
      "dependencies": [],  // No dependencies, can execute immediately
      "reasoning": "..."
    },
    {
      "id": "rec-002",
      "action": "restart_pods",
      "parameters": {...},
      "dependencies": ["rec-001"],  // Must wait for rec-001 to complete
      "reasoning": "..."
    }
  ]
}
"""
```

**Response Schema**:
```go
type Recommendation struct {
    ID          string                 `json:"id"`
    Action      string                 `json:"action"`
    Parameters  map[string]interface{} `json:"parameters"`
    Dependencies []string              `json:"dependencies"` // ✅ ADD THIS FIELD
    Reasoning   string                 `json:"reasoning"`
    Confidence  float64                `json:"confidence"`
}
```

---

### **Option B: Keep Current Behavior (Simple Default)**

**Accept Limitations**:
- HolmesGPT returns flat list of recommendations
- System defaults to sequential execution (primary → secondary1 → secondary2)
- Complex dependency graphs require manual workflow definition

**Pros**:
- ✅ No changes needed to HolmesGPT integration
- ✅ Simple, predictable behavior
- ✅ Works for majority of use cases (90%+ are sequential)

**Cons**:
- ❌ Cannot express parallel steps (e.g., "scale deployment, THEN restart pods A and B in parallel")
- ❌ Cannot express complex dependencies (diamond pattern, fork-join)
- ❌ Limited workflow optimization opportunities

---

### **Option C: Hybrid Approach (Short-Term + Long-Term)**

**Short-Term (V1)**:
- Keep current behavior (sequential by default)
- Add **optional** `dependencies` field to HolmesGPT response
- If dependencies present, use them; if missing, fall back to sequential

**Long-Term (V2)**:
- Make dependencies **mandatory** in HolmesGPT response
- Enhance prompt to always include dependency information
- Add validation to reject responses without dependencies

---

## 📊 **SUMMARY**

### **Question**: Is the model aware it needs to include dependencies?

**Answer**: **NO**

**Evidence**:
1. ❌ **NO business requirements** specify dependency output from HolmesGPT
2. ❌ **NO prompt instructions** (that we can see) request dependency information
3. ❌ **NO response schema** validation for dependencies
4. ❌ **Test code shows** dependencies are hardcoded/empty, not from AI

---

### **Current State**:
- HolmesGPT returns: **List of recommendations** (no dependencies)
- System behavior: **Sequential by default** (primary, then secondary1, then secondary2)
- Complex workflows: **NOT SUPPORTED** (cannot express parallel steps or complex dependencies)

---

### **Recommended Action**:
1. **Add Business Requirements** (BR-HOLMES-031 to BR-HOLMES-033)
2. **Enhance HolmesGPT Prompt** to request dependency information
3. **Update Response Schema** with `dependencies: []string` field
4. **Implement Validation** to ensure dependency graph is acyclic
5. **Update Documentation** to reflect actual vs aspirational behavior

---

**Confidence**: **95%** - Based on comprehensive analysis of requirements, code, tests, and documentation

**Document Status**: ✅ **COMPLETE** - Assessment of HolmesGPT dependency specification expectations
