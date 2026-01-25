# Testing Anti-Pattern Detection Guide

**Authority**: [.cursor/rules/08-testing-anti-patterns.mdc](../../.cursor/rules/08-testing-anti-patterns.mdc)
**Status**: Active - Enforced in CI/CD

---

## 🚨 **Critical Anti-Patterns**

### **1. NULL-TESTING (Most Critical)**

**Definition**: Testing for basic existence rather than business outcomes

**Violations**:
```go
// ❌ NULL-TESTING: Tests nothing meaningful
Expect(result).ToNot(BeNil())
Expect(result).ToNot(BeEmpty())
Expect(len(items)).To(BeNumerically(">", 0))
Expect(err).To(BeNil())  // Weak error testing
```

**Correct Approach**:
```go
// ✅ BUSINESS OUTCOME TESTING
Expect(workflow.Template.SafetyValidation).To(ContainElement("resource-limits"))
Expect(analysis.ConfidenceScore).To(BeNumerically(">=", 0.8))
Expect(recommendation.Actions).To(HaveLen(3))
Expect(result.BusinessMetrics.SuccessRate).To(BeNumerically(">", 0.9))
```

---

### **2. STATIC DATA TESTING**

**Definition**: Testing hardcoded values instead of business logic

**Violations**:
```go
// ❌ STATIC DATA TESTING
input := "test-alert-name"
Expect(result.Name).To(Equal("test-alert-name"))

configValue := "production"
Expect(app.Environment).To(Equal("production"))
```

**Correct Approach**:
```go
// ✅ BUSINESS LOGIC TESTING
input := testutil.GenerateAlertName()
Expect(result.Name).To(MatchRegexp("^alert-[0-9]+$"))

configValue := testutil.GetConfigValue("environment")
Expect(app.Environment).To(Equal(configValue))
```

---

### **3. LIBRARY TESTING**

**Definition**: Testing third-party libraries instead of business logic

**Violations**:
```go
// ❌ LIBRARY TESTING: Testing logrus, not business logic
logger := logrus.New()
Expect(logger).ToNot(BeNil())

// ❌ LIBRARY TESTING: Testing context package
ctx := context.WithValue(context.Background(), "key", "value")
Expect(ctx.Value("key")).To(Equal("value"))
```

**Correct Approach**:
```go
// ✅ BUSINESS LOGIC TESTING
logger := business.NewStructuredLogger(config)
logger.LogWorkflowEvent("workflow-123", "completed")
Expect(logger.GetLastEvent().WorkflowID).To(Equal("workflow-123"))

// ✅ BUSINESS CONTEXT TESTING
ctx := business.NewRequestContext(userID, workflowID)
Expect(ctx.GetWorkflowID()).To(Equal(workflowID))
```

---

### **4. IMPLEMENTATION TESTING**

**Definition**: Testing how code works instead of what business outcome it produces

**Violations**:
```go
// ❌ IMPLEMENTATION TESTING: Testing internal methods
Expect(engine.parseWorkflowSteps(input)).To(HaveLen(3))
Expect(engine.validateStepOrder()).To(BeTrue())

// ❌ IMPLEMENTATION TESTING: Testing data structures
Expect(workflow.internalCache).To(HaveKey("step-1"))
```

**Correct Approach**:
```go
// ✅ BUSINESS OUTCOME TESTING
workflow, err := engine.CreateWorkflow(input)
Expect(err).ToNot(HaveOccurred())
Expect(workflow.Status).To(Equal(business.StatusReady))
Expect(workflow.CanExecute()).To(BeTrue())
```

---

### **5. MOCK OVERUSE**

**Definition**: Mocking internal business logic instead of external dependencies

**Violations**:
```go
// ❌ MOCK OVERUSE: Mocking business logic
mockValidator := mocks.NewMockWorkflowValidator()
mockAnalyzer := mocks.NewMockPerformanceAnalyzer()
mockOptimizer := mocks.NewMockResourceOptimizer()

// All business logic mocked = testing nothing
engine := NewEngine(mockValidator, mockAnalyzer, mockOptimizer)
```

**Correct Approach**:
```go
// ✅ REAL BUSINESS LOGIC: Mock only external dependencies
mockLLMClient := mocks.NewMockLLMClient()
mockK8sClient := fake.NewClientBuilder().Build()

// Use REAL business components
validator := business.NewWorkflowValidator()
analyzer := business.NewPerformanceAnalyzer()
optimizer := business.NewResourceOptimizer()

engine := NewEngine(validator, analyzer, optimizer, mockLLMClient, mockK8sClient)
```

---

## 🔧 **Detection Commands**

### **Manual Detection**

```bash
# Detect NULL-TESTING violations
grep -r "ToNot(BeNil())\|ToNot(BeEmpty())" test/ --include="*_test.go"

# Detect STATIC DATA violations
grep -r "Equal(\".*\")" test/ --include="*_test.go" | grep -v "BR-"

# Detect LIBRARY TESTING violations
grep -r "logrus\.New()\|context\.With" test/ --include="*_test.go"

# Check for missing business requirement references
find test/ -name "*_test.go" -exec grep -L "BR-" {} \;
```

### **Automated Detection**

```bash
# Run comprehensive anti-pattern check
make lint-test-patterns

# Or directly via script
./scripts/validation/check-test-anti-patterns.sh
```

---

## 🚀 **CI/CD Integration**

### **Pre-Commit Hook**

```bash
#!/bin/bash
# .git/hooks/pre-commit

# Run anti-pattern detection
./scripts/validation/check-test-anti-patterns.sh

if [ $? -ne 0 ]; then
    echo "❌ Test anti-patterns detected. Commit blocked."
    echo "Run 'make lint-test-patterns' for details."
    exit 1
fi

echo "✅ All test pattern validations passed"
```

### **GitHub Actions**

```yaml
# .github/workflows/test-quality.yml
name: Test Quality Checks

on: [pull_request]

jobs:
  test-patterns:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Check for test anti-patterns
        run: make lint-test-patterns
```

---

## 📊 **Violation Metrics**

### **Track Progress**

```bash
# Count NULL-TESTING violations
NULL_TESTS=$(grep -r "ToNot(BeNil())\|ToNot(BeEmpty())" test/ --include="*_test.go" | wc -l)
echo "NULL-TESTING violations: $NULL_TESTS"

# Count tests missing BR references
MISSING_BR=$(find test/ -name "*_test.go" -exec grep -L "BR-" {} \; | wc -l)
echo "Tests missing BR references: $MISSING_BR"

# Count mock overuse (more than 3 mocks in a test)
MOCK_OVERUSE=$(grep -r "mock.*:=.*New" test/ --include="*_test.go" -c | awk '$1 > 3' | wc -l)
echo "Potential mock overuse: $MOCK_OVERUSE"
```

### **Historical Tracking**

```bash
# Generate anti-pattern report
./scripts/validation/check-test-anti-patterns.sh --report > anti-pattern-report.txt

# Track over time (add to CI)
echo "$(date),${NULL_TESTS},${MISSING_BR},${MOCK_OVERUSE}" >> anti-pattern-history.csv
```

---

## 🔄 **Remediation Workflow**

### **Step 1: Identify Violations**

```bash
# Run detection with file locations
./scripts/validation/check-test-anti-patterns.sh --verbose

# Example output:
# ❌ NULL-TESTING: test/unit/workflow/engine_test.go:45
# ❌ STATIC DATA: test/unit/gateway/handler_test.go:123
# ❌ LIBRARY TESTING: test/unit/ai/client_test.go:78
```

### **Step 2: Prioritize Fixes**

1. **High Priority**: NULL-TESTING (most common, easiest to fix)
2. **Medium Priority**: STATIC DATA (requires testutil updates)
3. **Low Priority**: LIBRARY TESTING (rare, requires refactoring)

### **Step 3: Fix Pattern**

```go
// BEFORE (NULL-TESTING)
It("should return a result", func() {
    result := engine.Process(input)
    Expect(result).ToNot(BeNil())
})

// AFTER (BUSINESS OUTCOME)
It("should process workflow and return ready status (BR-WF-001)", func() {
    result := engine.Process(input)
    Expect(result.Status).To(Equal(business.StatusReady))
    Expect(result.CanExecute()).To(BeTrue())
})
```

### **Step 4: Validate Fix**

```bash
# Re-run tests
go test ./test/unit/workflow/... -v

# Re-run anti-pattern detection
./scripts/validation/check-test-anti-patterns.sh

# Verify BR reference added
grep "BR-WF-001" test/unit/workflow/engine_test.go
```

---

## 🎯 **Best Practices**

### **Writing Tests**

1. ✅ **Always** reference business requirements (BR-XXX-XXX)
2. ✅ **Always** test business outcomes, not implementation
3. ✅ **Always** use real business logic, mock only external deps
4. ✅ **Always** use descriptive assertions with business context

### **Reviewing Tests**

1. ✅ **Check** for weak assertions (BeNil, BeEmpty without context)
2. ✅ **Check** for hardcoded test data (use testutil generators)
3. ✅ **Check** for mock overuse (>3 mocks = red flag)
4. ✅ **Check** for BR references in test descriptions

### **Refactoring Tests**

1. ✅ **Start** with high-impact violations (NULL-TESTING)
2. ✅ **Batch** similar violations across multiple files
3. ✅ **Validate** after each batch (ensure tests still pass)
4. ✅ **Document** refactoring decisions in commit messages

---

## 📚 **Related Documentation**

- **Testing Strategy**: [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)
- **Testing Patterns**: [TESTING_PATTERNS_QUICK_REFERENCE.md](TESTING_PATTERNS_QUICK_REFERENCE.md)
- **Pyramid Strategy**: [PYRAMID_TEST_MIGRATION_GUIDE.md](PYRAMID_TEST_MIGRATION_GUIDE.md)
- **Mock Policy**: [INTEGRATION_E2E_NO_MOCKS_POLICY.md](INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 🆘 **Need Help?**

### **Common Questions**

**Q**: How do I know if my assertion is weak?
**A**: If you're testing `BeNil()` or `BeEmpty()` without business context, it's weak.

**Q**: What counts as "business logic" vs "external dependency"?
**A**: Business logic = `pkg/` code. External = databases, APIs, K8s, LLM services.

**Q**: Can I ever use `ToNot(BeNil())`?
**A**: Only with business context: `Expect(result).ToNot(BeNil(), "BR-WF-001: Workflow must return result for tracking")`

**Q**: How many mocks are too many?
**A**: >3 mocks in a single test = red flag. Use real business components instead.

---

**Remember**: Tests validate business value, not technical implementation. Focus on outcomes, not mechanics.
