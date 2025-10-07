# APDC Quick Reference Card

## 🚀 **Analysis-Plan-Do-Check (APDC) at a Glance**

**APDC** is kubernaut's systematic development methodology that enhances TDD with structured phases and comprehensive rule enforcement.

---

## ⚡ **Quick Commands**

### **Individual Phases:**
```bash
/analyze [component/issue]     # 🔍 Analysis: Context + rules assessment
/plan [analysis-results]       # 📋 Plan: Strategy + user approval
/do [approved-plan]           # ⚡ Do: Implementation + checkpoints
/check [implementation]       # ✅ Check: Validation + rule triage
```

### **Complete Workflows:**
```bash
/apdc-full                    # 🔄 Complete APDC workflow overview
/fix-build-apdc              # 🔧 APDC-enhanced build fixing
/refactor-apdc               # 🔄 APDC-enhanced refactoring
```

---

## 🎯 **When to Use APDC**

### **✅ Use APDC for:**
- Complex feature development (multiple components)
- Significant refactoring (architectural changes)
- New component creation (business logic)
- Build error fixing (systematic remediation)
- AI/ML component development (sophisticated logic)
- Integration work (cross-component changes)
- Performance optimization (system-wide impact)

### **❌ Use Standard TDD for:**
- Simple bug fixes (single file changes)
- Documentation updates (no code changes)
- Configuration changes (no business logic)
- Test-only modifications (no implementation)

---

## 📋 **APDC Phase Overview**

| Phase | Duration | Purpose | Key Deliverable |
|-------|----------|---------|-----------------|
| **🔍 Analysis** | 5-15 min | Context understanding + rule assessment | Analysis report with business alignment |
| **📋 Plan** | 10-20 min | Strategy + TDD mapping + rule integration | Implementation plan (requires approval) |
| **⚡ Do** | Variable | Controlled implementation + checkpoints | Working code with integration |
| **✅ Check** | 5-10 min | Validation + rule triage + confidence | Compliance report with confidence rating |

---

## 🔧 **Rule Integration**

APDC enforces three critical rule sets:

### **@02-go-coding-standards.mdc**
- Business domain naming (e.g., `WorkflowEngine`, `AlertProcessor`)
- Error handling with context: `fmt.Errorf("description: %w", err)`
- Type safety (avoid `any`/`interface{}`)
- Business requirement mapping (BR-XXX-XXX)

### **@03-testing-strategy.mdc**
- Ginkgo/Gomega BDD framework (NO standard Go testing)
- TDD workflow (tests first, then implementation)
- Business requirement references in ALL tests
- Test pyramid: 70%+ unit, <20% integration, <10% e2e

### **@00-ai-assistant-behavioral-constraints.mdc**
- Type validation before field access
- Implementation discovery before creation
- Business integration verification
- Comprehensive symbol analysis

---

## 🔍 **Critical Checkpoints**

### **Analysis Phase Checkpoints:**
```bash
# Business requirement mapping
grep -r "BR-[A-Z]+-[0-9]+" docs/requirements/ --include="*.md"

# Technical impact assessment
codebase_search "existing [ComponentType] implementations"

# Integration point identification
grep -r "New[ComponentType]" cmd/ --include="*.go"
```

### **Do Phase Checkpoints:**
```bash
# CHECKPOINT A: Type Reference Validation
read_file [type_definition_file]

# CHECKPOINT B: Function + BDD Validation
grep -r "func.*[FunctionName]" . --include="*.go" -A 3

# CHECKPOINT C: Business Integration Validation
grep -r "New[ComponentType]" cmd/ --include="*.go"
```

### **Check Phase Validation:**
```bash
# Technical validation
go build ./...
golangci-lint run --timeout=5m
go test ./... -timeout=10m

# Rule compliance verification
grep -r "Describe\|It\|Expect" test/ --include="*_test.go" | wc -l
grep -r "BR-.*-.*:" test/ --include="*_test.go" | wc -l
```

---

## 📊 **Success Criteria**

### **Analysis Success:**
- ✅ Business requirement clearly mapped (BR-XXX-XXX)
- ✅ Technical impact comprehensively assessed
- ✅ Integration points identified and analyzed
- ✅ Risk evaluation completed with mitigation strategies

### **Planning Success:**
- ✅ TDD phase mapping completed with realistic timelines
- ✅ Resource requirements identified and validated
- ✅ Success criteria defined with measurable outcomes
- ✅ **USER APPROVAL RECEIVED** (mandatory)

### **Implementation Success:**
- ✅ All TDD phases executed according to approved plan
- ✅ Tests written first and failing appropriately (RED)
- ✅ Minimal implementation passes tests (GREEN)
- ✅ Code enhanced while preserving functionality (REFACTOR)
- ✅ Main application integration maintained throughout

### **Validation Success:**
- ✅ Business requirement fulfillment verified
- ✅ Technical validation completed successfully
- ✅ Integration confirmation achieved
- ✅ Performance assessment completed
- ✅ Rule compliance confirmed with violation triage
- ✅ Confidence assessment generated (≥60%)

---

## 🚨 **Emergency Protocols**

### **If Analysis Incomplete:**
- Re-execute comprehensive analysis with broader scope
- Focus on immediate business impact
- Seek stakeholder clarification

### **If Planning Rejected:**
- Revise strategy based on feedback
- Present alternative approaches
- Break complex plans into smaller phases

### **If Implementation Blocked:**
- Rollback to last successful checkpoint
- Re-analyze approach with stakeholder input
- Consider alternative implementation strategies

### **If Validation Failed:**
- Execute systematic rule violation triage
- Create detailed corrective action plan
- Get approval before implementing fixes

---

## 💡 **Pro Tips**

### **Analysis Phase:**
- Be thorough but focused - analysis prevents costly rework
- Always map to business requirements (BR-XXX-XXX)
- Check main application integration early

### **Planning Phase:**
- Never skip user approval - it's mandatory
- Plan validation checkpoints throughout implementation
- Always have rollback procedures ready

### **Do Phase:**
- Execute checkpoints religiously (A/B/C)
- Preserve main application integration throughout
- Use existing patterns - enhance rather than create

### **Check Phase:**
- Triage rule violations systematically
- Provide detailed confidence justification
- Plan follow-up monitoring and maintenance

---

## 📚 **Resources**

- **[Complete APDC Guide](APDC_FRAMEWORK.md)** - Comprehensive methodology documentation
- **[Core Development Methodology](/.cursor/rules/00-core-development-methodology.mdc)** - Complete APDC rule integration
- **[Development Guidelines](../development/project%20guidelines.md)** - Updated with APDC integration

---

**APDC transforms complex development from chaotic to systematic, ensuring quality and compliance at every step.**
