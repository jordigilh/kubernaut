# Kubernaut Development Quick Reference Card

> **Keep this handy during development to ensure business requirement-driven development**

## 🚨 **RED FLAGS - STOP IMMEDIATELY**

```go
❌ // Stub implementation
❌ // TODO: Implement
❌ return nil // (in function body with no logic)
❌ return &Struct{}, nil // (empty struct with no logic)
❌ _ = variable // (suppressing unused variable)
```

**If you see these patterns: STOP and implement real functionality**

---

## ✅ **DEVELOPMENT FLOW**

### **1. BEFORE WRITING CODE**
```
□ Business requirement documented
□ Success criteria defined
□ Failure scenarios identified
□ User impact described
```

**Template:**
```
Business Requirement: [What business problem does this solve?]
Success Criteria: [How do we measure success?]
User Impact: [How does this help end users?]
Failure Handling: [What happens when this fails?]
```

### **2. IMPLEMENTATION CHECKLIST**
```
□ Real business logic (no stubs)
□ Error handling for actual scenarios
□ Input validation and sanitization
□ Logging for observability
□ No TODO comments
```

### **3. TESTING CHECKLIST**
```
□ Tests validate business outcomes
□ Uses real dependencies where possible
□ Measures business success criteria
□ Would make sense to business stakeholder
```

---

## 🎯 **TESTING PATTERNS**

### **❌ IMPLEMENTATION-BASED (Wrong)**
```go
It("should return nil error", func() {
    result, err := method()
    Expect(err).ToNot(HaveOccurred())
    Expect(result).ToNot(BeNil())
})
```

### **✅ BUSINESS REQUIREMENT-BASED (Correct)**
```go
It("should improve recommendation accuracy after learning from failures", func() {
    // Given: Real business scenario
    failures := createRealFailureData(10)
    system.ProcessFailures(failures)

    // When: System makes new recommendations
    newRecommendation := system.GetRecommendation(similarAlert)

    // Then: Business outcome validation
    Expect(newRecommendation.Accuracy).To(BeNumerically(">", 0.8))
    Expect(newRecommendation.ConfidenceScore).To(BeNumerically(">", 0.7))
})
```

---

## 🔍 **CODE REVIEW QUESTIONS**

### **For Authors:**
1. "What business problem does this solve?"
2. "How do I know this actually works for users?"
3. "What business requirement does my test validate?"

### **For Reviewers:**
1. "Can I understand the business value without reading code?"
2. "Do the tests check business outcomes or implementation details?"
3. "Would this work in production for real users?"

---

## 📊 **QUALITY METRICS**

### **Code Quality**
- **Stub Count**: Must be 0 (any stub blocks merge)
- **TODO Count**: Must be 0 in production code
- **Business Logic**: Every method performs real business function

### **Test Quality**
- **Business Coverage**: >80% of tests validate business outcomes
- **Real Dependencies**: >90% of integration tests use real services
- **Business Understanding**: Business stakeholder can understand test results

---

## ⚡ **DAILY QUICK CHECKS**

**Before Committing:**
```
□ "What business problem am I solving?"
□ "Are there any stub implementations?"
□ "Do my tests validate business outcomes?"
□ "How does this help end users?"
```

**Before Code Review:**
```
□ Business requirement linked to PR
□ Tests validate business success criteria
□ No stub methods or TODO comments
□ Error handling covers real failure scenarios
```

---

## 🆘 **WHEN STUCK**

### **"I can't implement this without stubs"**
**Solution:** Break it down smaller
1. Find the tiniest piece of real functionality
2. Implement that completely
3. Test that real functionality
4. Iterate and expand

### **"The test is too complex with real dependencies"**
**Solution:** Simplify the scenario
1. Use smallest realistic test case
2. Use controlled test data, not fake responses
3. Focus on one business outcome per test

### **"This will take too long"**
**Solution:** Reduce scope, not quality
1. Deliver smaller functionality completely
2. Document future requirements separately
3. Never compromise on "real implementation"

---

## 🎯 **CRITICAL COMPONENTS PRIORITY**

### **Phase 1 - CRITICAL (Must implement first)**
1. **AI Effectiveness Assessment** - System must learn from failures
2. **Workflow Action Execution** - Workflows must perform real K8s operations
3. **Business Requirement Testing** - Tests must validate business outcomes

### **Phase 2 - Important**
4. Pattern Discovery Engine completion
5. Vector Database external integrations
6. Enhanced monitoring integration

---

## 📋 **EMERGENCY CONTACTS**

**If you find critical stub implementations:**
1. **DO NOT MERGE** - Block PR immediately
2. **Document business requirement** - What should this actually do?
3. **Implement real functionality** - No shortcuts
4. **Write business outcome tests** - Validate actual functionality

**Remember:** Better to deliver small real functionality than large sophisticated stubs.

---

## 🏆 **SUCCESS INDICATORS**

**You're on the right track when:**
- ✅ Business stakeholders can understand your tests
- ✅ Your code solves real user problems
- ✅ Tests fail when business requirements aren't met
- ✅ Every method performs its intended business function
- ✅ CI/CD passes stub detection automatically

**You're going wrong when:**
- ❌ Tests pass but business requirement isn't met
- ❌ Sophisticated mocks hide missing functionality
- ❌ "We'll implement the real logic later"
- ❌ Business stakeholder can't understand test results
