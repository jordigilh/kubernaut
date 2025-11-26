# Testing Guidelines: Business Requirements vs Unit Tests

This document provides clear guidance on **when** and **how** to use each type of test in the kubernaut system.

## 🎯 **Decision Framework**

### Quick Decision Tree
```
📝 QUESTION: What are you trying to validate?

├─ 💼 "Does it solve the business problem?"
│  ├─ User-facing functionality ────────────► BUSINESS REQUIREMENT TEST
│  ├─ Performance/reliability requirements ─► BUSINESS REQUIREMENT TEST
│  ├─ Business value delivery ─────────────► BUSINESS REQUIREMENT TEST
│  └─ Cross-component workflows ───────────► BUSINESS REQUIREMENT TEST
│
└─ 🔧 "Does the code work correctly?"
   ├─ Function/method behavior ────────────► UNIT TEST
   ├─ Error handling & edge cases ────────► UNIT TEST
   ├─ Internal component logic ───────────► UNIT TEST
   └─ Code correctness & robustness ──────► UNIT TEST
```

## 📊 **Test Type Comparison**

| Aspect | Business Requirement Tests | Unit Tests |
|--------|----------------------------|------------|
| **Purpose** | Validate business value delivery | Validate business behavior + implementation correctness |
| **Focus** | External behavior & outcomes | Internal code mechanics |
| **Audience** | Business stakeholders + developers | Developers |
| **Metrics** | Business KPIs (accuracy, cost, time) | Technical metrics (coverage, performance) |
| **Dependencies** | Realistic/controlled mocks | Minimal mocks |
| **Execution Time** | Slower (seconds to minutes) | Fast (milliseconds) |
| **Change Frequency** | Stable (business requirements) | Higher (implementation changes) |

## 🏗️ **When to Use Business Requirement Tests**

### ✅ **Use Business Requirements Tests For:**

#### 1. **User-Facing Features**
```go
// ✅ GOOD: Tests user-visible behavior
Describe("BR-AI-001: System Must Reduce Alert Noise by 80%", func() {
    It("should dramatically reduce duplicate alerts through correlation", func() {
        // Given: 100 similar alerts per hour (baseline)
        // When: Alert correlation is enabled
        // Then: Alert volume should be <20 alerts per hour
    })
})
```

#### 2. **Performance & Reliability Requirements**
```go
// ✅ GOOD: Tests business SLA compliance
Describe("BR-WF-003: Workflows Must Complete Within 30-Second SLA", func() {
    It("should process standard operations within performance threshold", func() {
        // Validates business requirement for operational responsiveness
    })
})
```

#### 3. **Business Value Delivery**
```go
// ✅ GOOD: Tests measurable business improvement
Describe("BR-AI-002: System Must Improve Accuracy by 25% Over 30 Days", func() {
    It("should demonstrate measurable learning and improvement", func() {
        // Tests quantifiable business value delivery
    })
})
```

#### 4. **Cross-Component Workflows**
```go
// ✅ GOOD: Tests end-to-end business processes
Describe("BR-INT-001: Alert-to-Resolution Must Complete Under 5 Minutes", func() {
    It("should handle complete alert lifecycle within business SLA", func() {
        // Tests complete business process across multiple components
    })
})
```

### ❌ **Don't Use Business Requirements Tests For:**

#### 1. **Implementation Details**
```go
// ❌ BAD: Tests internal implementation
Describe("validateWorkflowSteps function", func() {
    It("should return ValidationError for invalid step", func() {
        // This tests code behavior, not business value
    })
})
```

#### 2. **Technical Edge Cases**
```go
// ❌ BAD: Tests technical error handling
Describe("ProcessPendingAssessments with nil context", func() {
    It("should return context error", func() {
        // This tests defensive programming, not business requirements
    })
})
```

## 🔧 **When to Use Unit Tests**

### ✅ **Use Unit Tests For:**

#### 1. **Function/Method Behavior**
```go
// ✅ GOOD: Tests specific function behavior
Describe("ValidationEngine.ValidateWorkflow", func() {
    It("should detect circular dependencies", func() {
        workflow := createWorkflowWithCircularDeps()
        err := validator.ValidateWorkflow(workflow)
        Expect(err).To(MatchError(CircularDependencyError))
    })
})
```

#### 2. **Error Handling & Edge Cases**
```go
// ✅ GOOD: Tests error conditions
Describe("EffectivenessAssessor.ProcessPendingAssessments", func() {
    Context("when repository is unavailable", func() {
        It("should return repository error", func() {
            mockRepo.SetError("connection failed")
            err := assessor.ProcessPendingAssessments(ctx)
            Expect(err).To(MatchError(ContainSubstring("connection failed")))
        })
    })
})
```

#### 3. **Internal Logic Validation**
```go
// ✅ GOOD: Tests internal computation
Describe("calculateConfidenceAdjustment", func() {
    It("should reduce confidence proportionally to failure rate", func() {
        failureRate := 0.8
        adjustment := calculateConfidenceAdjustment(failureRate)
        Expect(adjustment).To(BeNumerically("<", 0))
    })
})
```

#### 4. **Interface Compliance**
```go
// ✅ GOOD: Tests interface contracts
Describe("MockEffectivenessRepository", func() {
    It("should implement EffectivenessRepository interface", func() {
        var repo EffectivenessRepository = NewMockEffectivenessRepository()
        Expect(repo).ToNot(BeNil())
    })
})
```

### ❌ **Don't Use Unit Tests For:**

#### 1. **Business Value Validation**
```go
// ❌ BAD: Tries to test business value with unit test
Describe("ProcessPendingAssessments", func() {
    It("should improve system accuracy", func() {
        // Business outcomes need business requirement tests
    })
})
```

#### 2. **End-to-End Workflows**
```go
// ❌ BAD: Complex integration in unit test
Describe("CompleteAlertResolution", func() {
    It("should process alert from detection to resolution", func() {
        // This belongs in business requirement or integration tests
    })
})
```

## 📋 **Testing Strategies by Component**

### AI & ML Components

#### Business Requirements Tests:
- Learning and adaptation over time
- Recommendation accuracy improvements
- Response time SLAs
- Business value delivery (cost reduction, time savings)

#### Unit Tests:
- Algorithm correctness
- Model training edge cases
- Data validation and preprocessing
- Error handling for invalid inputs

### Workflow Engine

#### Business Requirements Tests:
- End-to-end workflow execution
- Performance SLAs (30-second completion)
- Rollback and recovery capabilities
- Real Kubernetes operations

#### Unit Tests:
- Step validation logic
- Dependency resolution algorithms
- Error propagation between steps
- Configuration parsing

### Infrastructure & Platform

#### Business Requirements Tests:
- System scalability (handle 10K alerts/hour)
- Reliability and uptime requirements
- Performance under load
- Cost efficiency improvements

#### Unit Tests:
- Connection pool management
- Resource allocation algorithms
- Health check implementations
- Configuration validation

## 🔄 **Test Development Workflow**

### 1. **Start with Business Requirements**
```go
// Step 1: Define business requirement
Describe("BR-AI-001: System Must Learn From Failures", func() {
    // Define what business outcome is expected
})
```

### 2. **Build Supporting Unit Tests**
```go
// Step 2: Test the implementation that delivers business value
Describe("EffectivenessAssessor.ProcessPendingAssessments", func() {
    // Test the mechanics that make business requirement possible
})
```

### 3. **Validate Integration Points**
```go
// Step 3: Ensure components work together for business value
// (Integration tests or broader business requirement tests)
```

## 🎯 **Quality Gates**

### Business Requirement Tests Must:
- [ ] **Map to documented business requirements** (BR-XXX-### IDs)
- [ ] **Be understandable by non-technical stakeholders**
- [ ] **Measure business value** (accuracy, performance, cost)
- [ ] **Use realistic data and scenarios**
- [ ] **Validate end-to-end outcomes**
- [ ] **Include business success criteria**

### Unit Tests Must:
- [ ] **Focus on implementation correctness**
- [ ] **Execute quickly** (<100ms per test)
- [ ] **Have minimal external dependencies**
- [ ] **Test edge cases and error conditions**
- [ ] **Provide clear developer feedback**
- [ ] **Maintain high code coverage**

## 📊 **Success Metrics**

### Business Requirements Test Success:
- **90%** of tests validate business requirements rather than implementation
- **Business stakeholders** can understand test results
- **Business value** is measurable and tracked
- **SLA compliance** is validated continuously

### Unit Test Success:
- **95%** code coverage for critical components
- **<10ms** average test execution time
- **Fast feedback** for developers during development
- **Reliable detection** of implementation regressions

## 🚀 **Migration Strategy**

### Converting Existing Tests

#### 1. **Identify Test Purpose**
Ask: "What is this test really validating?"

#### 2. **Business Value Test → Keep as Business Requirement**
```go
// Keep in business-requirements/
It("should reduce alert noise by 80%", func() {
    // This validates business value
})
```

#### 3. **Implementation Test → Keep as Unit Test**
```go
// Keep in pkg/component/
It("should return error for invalid input", func() {
    // This validates implementation correctness
})
```

#### 4. **Mixed Tests → Split**
```go
// BEFORE: Mixed concerns
It("should process assessments and improve accuracy", func() {
    // Tests both implementation AND business value
})

// AFTER: Separated
// Unit Test:
It("should process assessments without error", func() {
    // Tests implementation
})

// Business Requirement Test:
It("should improve recommendation accuracy through learning", func() {
    // Tests business value
})
```

## 💡 **Pro Tips**

### 1. **Start with Business Requirements**
Always begin with "What business problem are we solving?" before writing code or tests.

### 2. **Use the Right Granularity**
- **Business tests**: Coarse-grained, end-to-end scenarios
- **Unit tests**: Fine-grained, focused on specific functions

### 3. **Choose Appropriate Mocks**
- **Business tests**: Realistic mocks that simulate real behavior
- **Unit tests**: Simple mocks that isolate the component under test

### 4. **Measure What Matters**
- **Business tests**: Business KPIs and stakeholder success criteria
- **Unit tests**: Technical correctness and edge case handling

### 5. **Make Tests Sustainable**
- **Business tests**: Should remain stable as business requirements are stable
- **Unit tests**: Should be fast and provide immediate developer feedback
