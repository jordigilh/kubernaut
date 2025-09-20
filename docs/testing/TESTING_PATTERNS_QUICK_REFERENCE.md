# Testing Patterns Quick Reference

## 🚀 **Quick Start Checklist**

### **Before Writing a Test**
- [ ] Identify the business requirement (BR-XXX-XXX)
- [ ] Use factory pattern for mocks (`mockFactory.CreateXXX()`)
- [ ] Write business-driven assertions (not just null checks)
- [ ] Include descriptive BR context in assertions

### **Before Submitting a PR**
- [ ] No weak assertions (`ToNot(BeNil())`, `ToNot(BeEmpty())` without business context)
- [ ] All mocks use factory pattern where available
- [ ] Tests compile without errors (`go test ./...`)
- [ ] Linting passes (`golangci-lint run`)

## ⚡ **Common Patterns**

### **Business Assertion Pattern**
```go
// Template
Expect(actualValue).To(BeExpectedCondition(),
    "BR-XXX-YYY: Business context explaining why this validation matters")

// Examples
Expect(response.ConfidenceScore).To(BeNumerically(">=", 0.8),
    "BR-AI-001-CONFIDENCE: AI analysis must return high confidence scores for reliable decision making")

Expect(len(workflow.Steps)).To(BeNumerically(">=", 1),
    "BR-WF-001-SUCCESS-RATE: Workflow must contain executable steps for success tracking")
```

### **Mock Factory Pattern**
```go
// Template
mockFactory := mocks.NewMockFactory(nil)
mockService := mockFactory.CreateServiceType(parameters)

// Examples
mockFactory := mocks.NewMockFactory(nil)
mockClient := mockFactory.CreateLLMClient([]string{"test-response"})
mockRepo := mockFactory.CreateExecutionRepository()
```

### **Configuration Helper Pattern**
```go
// Template
testconfig.ExpectBusinessRequirement(value, "BR-XXX-YYY", "environment", "description")

// Example
testconfig.ExpectBusinessRequirement(latency, "BR-PERF-001", "test",
    "API response time validation for performance requirements")
```

## 🎯 **Business Requirement Categories**

| **Category** | **BR Code Pattern** | **Example Usage** |
|-------------|-------------------|------------------|
| **AI/ML** | `BR-AI-00X` | Confidence scores, model accuracy |
| **Workflow** | `BR-WF-00X` | Success rates, execution states |
| **Database** | `BR-DATABASE-00X` | Connection pools, query performance |
| **Monitoring** | `BR-MON-00X` | Uptime, health checks, metrics |
| **Orchestration** | `BR-ORK-00X` | Resource optimization, scheduling |
| **Security** | `BR-SEC-00X` | Encryption, access controls |

## 🔧 **Available Factory Methods**

| **Mock Type** | **Factory Method** | **Usage** |
|--------------|-------------------|-----------|
| **LLM Client** | `CreateLLMClient(responses)` | AI/ML testing |
| **Execution Repository** | `CreateExecutionRepository()` | Workflow persistence |
| **Database Monitor** | `CreateDatabaseMonitor()` | DB health checks |
| **Safety Validator** | `CreateSafetyValidator()` | Risk assessment |
| **Adaptive Orchestrator** | `CreateAdaptiveOrchestrator()` | Performance optimization |

## ❌ **Anti-Patterns to Avoid**

### **Weak Assertions**
```go
// ❌ DON'T: Generic null/empty checks
Expect(result).ToNot(BeNil())
Expect(collection).ToNot(BeEmpty())
Expect(value).To(BeNumerically(">", 0))

// ✅ DO: Specific business validations
Expect(result.Status).To(Equal("success"))
Expect(len(collection.Items)).To(BeNumerically(">=", 1))
Expect(value.ProcessingTime).To(BeNumerically("<", maxAllowedTime))
```

### **Local Mock Creation**
```go
// ❌ DON'T: Direct instantiation
mockService := mocks.NewMockService()
mockClient := &LocalMockClient{}

// ✅ DO: Factory pattern
mockFactory := mocks.NewMockFactory(nil)
mockService := mockFactory.CreateService()
```

### **Generic Test Descriptions**
```go
// ❌ DON'T: Vague descriptions
It("should work correctly", func() {})
Context("service testing", func() {})

// ✅ DO: Business requirement context
It("should return valid confidence scores for business decision making", func() {})
Context("BR-AI-001: AI Analysis Confidence Requirements", func() {})
```

## 🔍 **Debugging Common Issues**

### **Compilation Errors**
```bash
# Check for missing imports
go mod tidy
go test ./... -v

# Fix import cycles
go list -json ./... | jq '.ImportPath,.Imports'
```

### **Linting Issues**
```bash
# Run linter
golangci-lint run

# Common fixes
- Remove unused variables: `_ = unusedVar`
- Fix shadowing: Rename conflicting variables
- Address cyclomatic complexity: Extract functions
```

### **Test Failures After Refactoring**
```bash
# Identify the specific assertion
go test -v ./path/to/test -run TestSpecificFunction

# Common issues:
- Method signatures changed: Check interface compatibility
- Mock expectations: Verify expected calls match usage
- Business thresholds: Check configuration values
```

## 📊 **Compliance Checking**

### **Manual Verification**
```bash
# Count remaining weak assertions
grep -r "ToNot.*BeNil" test/ | wc -l
grep -r "ToNot.*BeEmpty" test/ | wc -l

# Verify factory usage
grep -r "mocks\.New" test/ | grep -v "Factory"

# Check business requirement coverage
grep -r "BR-" test/ | wc -l
```

### **Automated CI/CD Integration**
- **Pipeline**: `.github/workflows/testing-compliance.yml`
- **Triggers**: PR creation, push to main branch
- **Checks**: Weak assertions, mock patterns, business requirements

## 🎓 **Learning Resources**

### **Key Files to Reference**
- **Factory Implementation**: `pkg/testutil/mocks/factory.go`
- **Helper Functions**: `pkg/testutil/config/helpers.go`
- **Business Thresholds**: `test/config/thresholds.yaml`
- **Project Guidelines**: `.cursor/rules/00-project-guidelines.mdc`

### **Example Test Files**
- **Good Patterns**: `test/unit/ai/llm/llm_algorithm_logic_test.go`
- **Mock Factory Usage**: `test/unit/ai/conditions/condition_impl_test.go`
- **Business Integration**: `test/unit/workflow-engine/workflow_state_consistency_validation_test.go`

### **Training Scenarios**
1. **Convert a weak assertion** to business validation
2. **Migrate direct mock creation** to factory pattern
3. **Add business requirement context** to existing tests
4. **Configure custom business thresholds** for new domains

---

*Keep this guide handy for quick reference during development!* 📋✨
