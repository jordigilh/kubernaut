# APDC Development Methodology Guide

## 🚀 **Analysis-Plan-Do-Check (APDC) Framework**

**APDC** is kubernaut's enhanced TDD methodology that provides systematic development through structured phases with comprehensive rule enforcement.

---

## 📖 **Table of Contents**

1. [Overview](#overview)
2. [APDC Phases](#apdc-phases)
3. [Rule Integration](#rule-integration)
4. [Shortcuts & Commands](#shortcuts--commands)
5. [Practical Examples](#practical-examples)
6. [Best Practices](#best-practices)
7. [Troubleshooting](#troubleshooting)

---

## 🎯 **Overview**

### **What is APDC?**

APDC (Analysis-Plan-Do-Check) is a systematic development methodology that enhances kubernaut's existing TDD workflow with structured phases and comprehensive rule enforcement.

```
🔄 APDC CYCLE:
Analysis → Plan → Do → Check → [Production Ready]
   ↓        ↓      ↓      ↓
 Context  Strategy Impl  Validation
   +        +      +      +
 Rules    Rules  Rules  Rules
```

### **Why APDC?**

- **Proactive Problem Prevention**: Analysis phase catches issues before implementation
- **Systematic Planning**: Structured approach reduces rework and technical debt
- **Quality Assurance**: Built-in rule validation throughout the process
- **Business Alignment**: Ensures all changes serve documented business requirements

### **When to Use APDC**

#### **✅ Use APDC for:**
- Complex feature development (multiple components)
- Significant refactoring (architectural changes)
- New component creation (business logic)
- Integration work (cross-component changes)
- Performance optimization (system-wide impact)
- AI/ML component development (sophisticated logic)
- Build error fixing (systematic remediation)

#### **❌ Use Standard TDD for:**
- Simple bug fixes (single file changes)
- Documentation updates (no code changes)
- Configuration changes (no business logic)
- Test-only modifications (no implementation)

---

## 🔍 **APDC Phases**

### **Phase 1: Analysis (5-15 minutes)**

**Purpose**: Comprehensive context understanding before any code changes

#### **Key Activities:**
1. **Business Requirement Mapping**: Identify and validate BR-XXX-XXX alignment
2. **Technical Impact Assessment**: Analyze existing implementations and dependencies
3. **Integration Point Identification**: Map main application usage and interfaces
4. **Risk and Complexity Evaluation**: Assess implementation complexity and risks
5. **Rule Compliance Assessment**: Evaluate Go standards, testing strategy, AI constraints

#### **Validation Commands:**
```bash
# Business requirement analysis
grep -r "BR-[A-Z]+-[0-9]+" docs/requirements/ --include="*.md"
codebase_search "business requirement [BR-XXX-XXX] existing implementations"

# Technical impact assessment
codebase_search "existing [ComponentType] implementations and dependencies"
grep -r "[ComponentName]" cmd/ pkg/ test/ --include="*.go" -c

# Integration point identification
grep -r "New[ComponentType]\|Create[ComponentType]" cmd/ --include="*.go"

# Risk and complexity evaluation
go mod graph | grep [target_package]
```

#### **Deliverable: Analysis Report**
```
🔍 APDC ANALYSIS COMPLETE:

BUSINESS CONTEXT:
- Business Requirement: [BR-XXX-XXX]
- Business Value: [High/Medium/Low]
- Stakeholder Impact: [description]
- Project Alignment: [aligned/partial/misaligned]

TECHNICAL CONTEXT:
- Existing Implementations: [N components found]
- Dependencies: [list key dependencies]
- Integration Points: [M integration points]
- Architecture Impact: [minimal/moderate/significant]

IMPACT ASSESSMENT:
- Files Affected: [X direct, Y indirect]
- Cascade Effects: [description]
- Performance Impact: [estimated impact]
- Breaking Changes: [yes/no + details]

RISK EVALUATION:
- Complexity Level: [minimal/medium/extensive]
- Risk Level: [low/medium/high]
- Critical Dependencies: [list]
- Mitigation Required: [yes/no + strategies]

RULE COMPLIANCE:
- Go Coding Standards: [assessment]
- Testing Strategy: [assessment]
- AI Behavioral Constraints: [assessment]

RECOMMENDATION:
- Proceed to Planning Phase: [yes/no]
- Recommended Approach: [enhance existing/create new/alternative]
- Estimated Timeline: [duration]
- Prerequisites: [list requirements]
```

### **Phase 2: Plan (10-20 minutes)**

**Purpose**: Detailed implementation strategy with TDD phase mapping and rule integration

#### **Key Activities:**
1. **TDD Phase Mapping**: Map Analysis → DO-RED → DO-GREEN → DO-REFACTOR sequence
2. **Resource Planning**: Estimate timeline, dependencies, and requirements
3. **Success Criteria Definition**: Define measurable outcomes and validation checkpoints
4. **Risk Mitigation Strategy**: Develop contingency plans and rollback procedures
5. **Rule Compliance Planning**: Integrate Go standards, testing strategy, AI constraints

#### **Validation Commands:**
```bash
# TDD phase mapping and timeline estimation
./scripts/estimate-tdd-phases.sh [component] [complexity]

# Resource and dependency planning
./scripts/validate-implementation-dependencies.sh [plan]

# Success criteria definition
./scripts/define-success-criteria.sh [business-requirement] [technical-goals]

# Risk mitigation strategy
./scripts/create-risk-mitigation-plan.sh [identified-risks]
```

#### **Deliverable: Implementation Plan**
```
📋 APDC PLANNING COMPLETE:

IMPLEMENTATION STRATEGY:
- Approach: [enhance existing/create new/hybrid]
- TDD Phase Mapping:
  * DO-DISCOVERY: [duration] - [specific actions]
  * DO-RED: [duration] - [test strategy]
  * DO-GREEN: [duration] - [implementation + integration]
  * DO-REFACTOR: [duration] - [enhancement plan]

TIMELINE ESTIMATION:
- Total Duration: [estimated time]
- Critical Path: [key dependencies]
- Milestones: [major checkpoints]
- Buffer Time: [contingency allocation]

SUCCESS CRITERIA:
- Business Outcomes: [measurable results]
- Technical Validation: [verification steps]
- Integration Requirements: [main app integration]
- Performance Targets: [benchmarks]

RULE COMPLIANCE PLAN:
- Go Standards Integration: [naming, error handling, types]
- Testing Strategy: [BDD framework, pyramid, mocks]
- AI Constraints: [validation checkpoints, tool usage]

RISK MITIGATION:
- High Risk Items: [list with mitigation]
- Medium Risk Items: [list with monitoring]
- Contingency Plans: [alternative approaches]
- Rollback Strategy: [recovery procedures]

APPROVAL STATUS: [PENDING USER APPROVAL]
```

**⚠️ MANDATORY**: User approval required before proceeding to DO phase

### **Phase 3: Do (Variable duration)**

**Purpose**: Controlled TDD execution following approved plan with continuous validation

#### **Sub-Phases:**

##### **DO-DISCOVERY (5-10 min): Analysis-Guided Component Research**
```bash
# Execute analysis-guided discovery
codebase_search "existing [Component] implementations"
# Follow analysis recommendations for search patterns
```

##### **DO-RED (10-15 min): Plan-Structured Test Creation**
```bash
# Follow planned test strategy with BDD framework validation
./scripts/phase2-red-validation.sh test_file.go

# MANDATORY: Validate Ginkgo/Gomega BDD framework
grep -r "func Test.*testing\.T" [target_file]
if [ $? -eq 0 ]; then
    echo "❌ VIOLATION: Standard Go testing found - MUST use Ginkgo/Gomega"
    exit 1
fi

# MANDATORY: Validate business requirement mapping
grep -r "BR-.*-.*:" [target_file]
if [ $? -ne 0 ]; then
    echo "❌ VIOLATION: Missing BR-XXX-XXX mapping in test descriptions"
    exit 1
fi
```

##### **DO-GREEN (15-20 min): Plan-Guided Implementation + Integration**
```bash
# Execute planned implementation strategy
./scripts/phase3-green-validation.sh component_name

# MANDATORY: Verify main application integration
grep -r "NewComponent" cmd/ --include="*.go" || echo "❌ Missing integration"

# MANDATORY: Type safety validation
read_file [type_definition_file]
# Verify all referenced fields exist in struct definitions
```

##### **DO-REFACTOR (20-30 min): Plan-Structured Enhancement**
```bash
# Follow planned enhancement strategy
./scripts/phase4-refactor-validation.sh

# MANDATORY: Preserve integration during refactoring
grep -r "[ComponentName]" cmd/ --include="*.go"
```

#### **Continuous Validation Checkpoints:**

##### **CHECKPOINT A: Type Reference Validation**
```bash
# MANDATORY before any struct field reference
read_file [type_definition_file]
grep -r "type.*[TypeName].*struct" pkg/ --include="*.go"
# RULE: All referenced fields must exist in struct definitions
```

##### **CHECKPOINT B: Function + Testing Framework Validation**
```bash
# MANDATORY before function calls or test modifications
grep -r "func.*[FunctionName]" . --include="*.go" -A 3
# RULE: Verify function exists and signature matches before calling
```

##### **CHECKPOINT C: Business Integration Validation**
```bash
# MANDATORY for business components
grep -r "New[ComponentType]" cmd/ --include="*.go"
# RULE: Business code must be integrated in main applications
```

### **Phase 4: Check (5-10 minutes)**

**Purpose**: Comprehensive result validation, confidence assessment, and rule violation triage

#### **Key Activities:**
1. **Business Verification**: Confirm BR-XXX-XXX requirements fulfilled
2. **Technical Validation**: Build, lint, and test execution verification
3. **Integration Confirmation**: Main application and cross-component validation
4. **Performance Assessment**: Benchmark and regression analysis
5. **Rule Compliance Verification**: Comprehensive rule validation
6. **Rule Violation Triage**: Address any violations with corrective action plans

#### **Validation Commands:**
```bash
# Technical validation
go build ./...                                    # Build success
golangci-lint run --timeout=5m                  # Lint compliance
go test ./... -timeout=10m                       # Test execution

# Rule compliance verification
# Go Coding Standards
grep -r "fmt\.Errorf.*%w" pkg/ --include="*.go"   # Error handling
grep -r "interface{}\|any" pkg/ --include="*.go"  # Type safety

# Testing Strategy
grep -r "Describe\|It\|Expect" test/ --include="*_test.go" | wc -l  # BDD framework
grep -r "BR-.*-.*:" test/ --include="*_test.go" | wc -l           # Business requirements

# AI Behavioral Constraints
grep -r "New.*\|Create.*" cmd/ --include="*.go" | wc -l           # Integration validation
```

#### **Rule Violation Triage:**
```
🚨 RULE VIOLATION TRIAGE:

VIOLATIONS DETECTED:
- Go Standards: [list violations with corrective actions]
- Testing Strategy: [list BDD/mock/requirement violations]
- AI Constraints: [list type/integration violations]

CORRECTIVE ACTION PLAN:
1. [Specific violation] → [Corrective action] → [Validation method]
2. [Specific violation] → [Corrective action] → [Validation method]

TRIAGE STATUS: [RESOLVED/REQUIRES_ACTION/ESCALATED]
```

#### **Deliverable: Validation Report**
```
✅ APDC CHECK COMPLETE:

BUSINESS VERIFICATION:
- Business Requirement: [BR-XXX-XXX] - [✅ FULFILLED]
- Business Value: [delivered/partial/not delivered]
- Stakeholder Impact: [positive/neutral/negative]
- Project Alignment: [aligned/partial/misaligned]

TECHNICAL VALIDATION:
- Build Status: [✅ Success / ❌ Failed]
- Lint Compliance: [✅ Clean / ❌ Issues Found]
- Test Results: [✅ All Passing / ❌ Failures]
- Test Coverage: [XX%] - [increased/maintained/decreased]

INTEGRATION CONFIRMATION:
- Main App Integration: [✅ Verified / ❌ Issues]
- Cross-Component: [✅ Functional / ❌ Broken]
- Data Flow: [✅ Intact / ❌ Disrupted]
- Interface Compatibility: [✅ Compatible / ❌ Breaking]

PERFORMANCE ASSESSMENT:
- Performance Impact: [improved/neutral/degraded]
- Benchmark Results: [XX% change]
- Resource Usage: [optimized/same/increased]
- Regression Status: [✅ None / ❌ Detected]

RULE COMPLIANCE:
- Go Coding Standards: [✅ COMPLIANT / ❌ VIOLATIONS TRIAGED]
- Testing Strategy: [✅ COMPLIANT / ❌ VIOLATIONS TRIAGED]
- AI Behavioral Constraints: [✅ COMPLIANT / ❌ VIOLATIONS TRIAGED]

CONFIDENCE ASSESSMENT: [XX%]
JUSTIFICATION: [detailed reasoning with evidence]
READY FOR PRODUCTION: [YES/NO/WITH_CONDITIONS]
```

---

## 🔧 **Rule Integration**

APDC methodology enforces three critical rule sets:

### **@02-go-coding-standards.mdc**
- **Business Domain Naming**: Use descriptive names (e.g., `EffectivenessAssessor`, `WorkflowEngine`)
- **Error Handling**: Wrap errors with context using `fmt.Errorf("description: %w", err)`
- **Type System**: Avoid `any`/`interface{}`, use structured types
- **Context Usage**: Accept `context.Context` as first parameter
- **Business Requirements**: Every component serves documented BR-XXX-XXX

### **@03-testing-strategy.mdc**
- **BDD Framework**: Ginkgo/Gomega MANDATORY (no standard Go testing)
- **TDD Workflow**: Tests first, then implementation
- **Business Requirements**: ALL tests reference BR-XXX-XXX
- **Mock Strategy**: Use `pkg/testutil/mock_factory.go` for consistent mocks
- **Test Pyramid**: 70%+ unit, <20% integration, <10% e2e

### **@00-ai-assistant-behavioral-constraints.mdc**
- **Type Reference Validation**: Read type definitions before referencing fields
- **Implementation Discovery**: Search existing patterns before creating new
- **Business Integration**: Verify main application usage
- **Symbol Analysis**: Comprehensive dependency analysis for undefined symbols

---

## ⚡ **Shortcuts & Commands**

### **Individual Phase Execution:**
```bash
/analyze [component/issue]     # Execute Analysis phase
/plan [analysis-results]       # Execute Planning phase
/do [approved-plan]           # Execute Implementation phase
/check [implementation]       # Execute Validation phase
```

### **Complete Workflows:**
```bash
/apdc-full                    # Complete APDC workflow overview
/fix-build-apdc              # APDC-enhanced build fixing
/refactor-apdc               # APDC-enhanced refactoring
```

### **Legacy Integration:**
```bash
/fix-build                   # Enhanced with APDC checkpoints
/refactor                    # Enhanced with APDC validation
```

---

## 💡 **Practical Examples**

### **Example 1: Build Error Fixing with APDC**

#### **Scenario**: Undefined symbol errors in workflow engine

##### **Analysis Phase:**
```bash
# Execute comprehensive symbol analysis
codebase_search "WorkflowOptimizer usage patterns and dependencies"
grep -r "WorkflowOptimizer" . --include="*.go" -n

# Map to business requirements
grep -r "BR-WF-" docs/requirements/ --include="*.md"
```

**Analysis Result**: `WorkflowOptimizer` referenced in 3 files but not defined. Maps to BR-WF-045 (workflow efficiency optimization).

##### **Planning Phase:**
```bash
# Plan implementation strategy
# Strategy: Enhance existing WorkflowEngine with optimization capability
# Timeline: 45 minutes (Analysis: 10, Plan: 10, Do: 20, Check: 5)
# Integration: Add to cmd/workflow-service/main.go
```

**Plan Approved**: Enhance existing engine rather than create new component.

##### **Do Phase:**
```bash
# DO-RED: Create tests for optimization interface
# DO-GREEN: Add optimization method to existing WorkflowEngine
# DO-REFACTOR: Implement ML-based optimization algorithm
```

##### **Check Phase:**
```bash
# Validate build success
go build ./...
# Verify integration
grep -r "WorkflowOptimizer" cmd/ --include="*.go"
# Confirm BR-WF-045 fulfillment
```

**Result**: Build errors resolved, 18% efficiency improvement achieved, BR-WF-045 fulfilled.

### **Example 2: New Feature Development with APDC**

#### **Scenario**: Implement AI-powered alert correlation

##### **Analysis Phase:**
- **Business Requirement**: BR-AI-067 (intelligent alert correlation)
- **Existing Components**: Found AlertProcessor, CorrelationEngine stub
- **Integration Points**: cmd/ai-service/main.go, pkg/ai/correlation/
- **Complexity**: Medium (enhance existing + new algorithms)

##### **Planning Phase:**
- **Strategy**: Enhance existing CorrelationEngine with ML algorithms
- **TDD Mapping**: RED (correlation tests) → GREEN (basic correlation) → REFACTOR (ML enhancement)
- **Timeline**: 90 minutes total
- **Success Criteria**: 85% correlation accuracy, <200ms latency

##### **Do Phase:**
- **DO-DISCOVERY**: Found existing correlation patterns in pkg/intelligence/
- **DO-RED**: Created comprehensive correlation test suite with BR-AI-067 mapping
- **DO-GREEN**: Implemented basic correlation with main app integration
- **DO-REFACTOR**: Added ML-based correlation algorithms

##### **Check Phase:**
- **Technical**: Build ✅, Lint ✅, Tests ✅
- **Business**: BR-AI-067 fulfilled with 87% accuracy
- **Integration**: Verified in cmd/ai-service/main.go
- **Performance**: 185ms average latency (within target)
- **Confidence**: 91%

---

## 🎯 **Best Practices**

### **Analysis Phase Best Practices:**
1. **Be Thorough**: Don't rush analysis - it prevents costly rework
2. **Map Business Value**: Always connect technical work to business requirements
3. **Assess Integration**: Check main application usage early
4. **Evaluate Complexity**: Honest assessment prevents timeline issues

### **Planning Phase Best Practices:**
1. **Get Approval**: Never skip user approval for implementation plan
2. **Plan Checkpoints**: Structure validation points throughout implementation
3. **Consider Alternatives**: Present multiple approaches when applicable
4. **Plan for Failure**: Always have rollback procedures

### **Do Phase Best Practices:**
1. **Follow Checkpoints**: Execute A/B/C validation religiously
2. **Preserve Integration**: Maintain main application usage throughout
3. **Use Existing Patterns**: Enhance rather than create when possible
4. **Validate Continuously**: Don't wait until the end to check compliance

### **Check Phase Best Practices:**
1. **Triage Violations**: Address rule violations systematically
2. **Measure Impact**: Quantify performance and quality improvements
3. **Document Confidence**: Provide detailed justification for confidence ratings
4. **Plan Follow-up**: Identify monitoring and maintenance needs

### **General APDC Best Practices:**
1. **Start Simple**: Use APDC for complex tasks, standard TDD for simple ones
2. **Document Decisions**: Capture rationale for future reference
3. **Learn from Results**: Use confidence assessments to improve process
4. **Collaborate**: Involve stakeholders in planning and validation

---

## 🔧 **Troubleshooting**

### **Common Issues and Solutions:**

#### **Analysis Phase Issues:**

**Problem**: Analysis taking too long (>20 minutes)
**Solution**:
- Focus on immediate scope, not entire system
- Use targeted codebase searches
- Limit business requirement mapping to directly related BRs

**Problem**: Can't find existing implementations
**Solution**:
- Try broader search terms
- Search in test files for usage patterns
- Check cmd/ directories for integration examples

#### **Planning Phase Issues:**

**Problem**: User rejects implementation plan
**Solution**:
- Revise approach based on feedback
- Present alternative strategies
- Break down complex plans into smaller phases

**Problem**: Timeline estimates consistently wrong
**Solution**:
- Track actual vs. estimated time
- Add buffer time for complex tasks
- Use historical data for similar tasks

#### **Do Phase Issues:**

**Problem**: Checkpoint validations failing
**Solution**:
- Stop implementation immediately
- Analyze root cause of validation failure
- Adjust approach or seek guidance

**Problem**: Integration breaking during implementation
**Solution**:
- Rollback to last working state
- Re-analyze integration requirements
- Use smaller, incremental changes

#### **Check Phase Issues:**

**Problem**: Rule violations detected
**Solution**:
- Execute systematic triage process
- Create corrective action plan
- Get approval before implementing fixes

**Problem**: Low confidence assessment
**Solution**:
- Identify specific risk factors
- Add additional validation steps
- Consider partial implementation with monitoring

### **Emergency Protocols:**

#### **If APDC Phase Failures:**
1. **Analysis Incomplete**: Re-execute comprehensive analysis with broader scope
2. **Planning Rejected**: Revise strategy with stakeholder input
3. **Implementation Blocked**: Rollback to last checkpoint, reassess approach
4. **Validation Failed**: Execute rule violation triage, provide corrective plan

#### **Escalation Criteria:**
- Multiple checkpoint failures in DO phase
- Repeated planning rejections
- Systematic rule violations across multiple areas
- Confidence assessment below 60% after remediation

---

## 📚 **Additional Resources**

### **Related Documentation:**
- [Core Development Methodology](/.cursor/rules/00-core-development-methodology.mdc) - Complete APDC rule integration
- [Go Coding Standards](/.cursor/rules/02-go-coding-standards.mdc) - Technical implementation standards
- [Testing Strategy](/.cursor/rules/03-testing-strategy.mdc) - Comprehensive testing approach
- [AI Behavioral Constraints](/.cursor/rules/00-ai-assistant-behavioral-constraints.mdc) - AI validation requirements

### **Validation Scripts:**
```bash
# APDC phase validation
./scripts/validate-apdc-analysis.sh [component] [business-requirement]
./scripts/validate-apdc-plan.sh [analysis-results] [implementation-strategy]
./scripts/validate-apdc-results.sh [implementation] [success-criteria]

# Rule compliance validation
./scripts/validate-rule-compliance.sh [implementation] [affected-rules]
./scripts/validate-business-requirement-fulfillment.sh "BR-XXX-XXX" [implementation]
```

### **Quick Reference:**
- **Analysis**: Context + Rules → Report
- **Plan**: Strategy + Approval → Implementation Plan
- **Do**: Checkpoints + Implementation → Working Code
- **Check**: Validation + Triage → Confidence Assessment

---

**APDC transforms complex development from chaotic to systematic, ensuring quality and compliance at every step.**
