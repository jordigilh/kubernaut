# [DD-XXX-YYY]: [Feature Name] - Implementation Plan

**Version**: 1.0
**Status**: 📋 DRAFT
**Design Decision**: [Link to DD-XXX-YYY]
**Service**: [Service Name]
**Confidence**: [60-100]% ([Evidence-Based/Estimated])
**Estimated Effort**: [3-12] days (APDC cycle: [X] days implementation + [Y] days testing + [Z] days documentation)

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.0** | YYYY-MM-DD | Initial implementation plan created | ✅ **CURRENT** |

---

## 🎯 **Business Requirements**

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-[SERVICE]-XXX** | [Primary requirement] | [Measurable success criteria] |
| **BR-[SERVICE]-YYY** | [Secondary requirement] | [Measurable success criteria] |

### **Success Metrics**

- **[Metric 1]**: [Target value with justification]
- **[Metric 2]**: [Target value with justification]
- **[Metric 3]**: [Target value with justification]

---

## 📅 **Timeline Overview**

### **Phase Breakdown**

| Phase | Duration | Days | Purpose | Key Deliverables |
|-------|----------|------|---------|------------------|
| **ANALYSIS** | [X] hours | Day 0 (pre-work) | Comprehensive context understanding | Analysis document, risk assessment, existing code review |
| **PLAN** | [Y] hours | Day 0 (pre-work) | Detailed implementation strategy | This document, TDD phase mapping, success criteria |
| **DO (Implementation)** | [N] days | Days 1-[N] | Controlled TDD execution | Core feature logic, integration |
| **CHECK (Testing)** | [M] days | Days [N+1]-[N+M] | Comprehensive result validation | Test suite (unit/integration/E2E), BR validation |
| **PRODUCTION READINESS** | [P] days | Days [N+M+1]-[N+M+P] | Documentation & deployment prep | Runbooks, handoff docs, confidence report |

### **[X]-Day Implementation Timeline**

| Day | Phase | Focus | Hours | Key Milestones |
|-----|-------|-------|-------|----------------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | [X]h | ✅ Analysis complete, Plan approved (this document) |
| **Day 1** | DO-RED | Foundation + Tests | 8h | Test framework, interfaces, failing tests |
| **Day 2** | DO-GREEN | Core logic | 8h | [Core feature implementation] |
| **Day 3** | DO-GREEN | [Feature aspect 2] | 8h | [Implementation detail] |
| **Day [N]** | DO-REFACTOR | Integration | 8h | [Integration with existing code] |
| **Day [N+1]** | CHECK | Unit tests | 8h | [Coverage target]+ unit tests |
| **Day [N+2]** | CHECK | Integration tests | 8h | [Integration test scenarios] |
| **Day [N+3]** | CHECK | E2E tests | 8h | Full feature lifecycle, BR validation |
| **Day [N+M+1]** | PRODUCTION | Documentation | 8h | API docs, runbooks, troubleshooting guides |
| **Day [N+M+P]** | PRODUCTION | Readiness review | 8h | Confidence report, handoff summary, deployment plan |

### **Critical Path Dependencies**

```
Day 1 (Foundation) → Day 2 (Core) → Day 3 ([Feature Aspect])
                                   ↓
Day [N] (Integration) → Days [N+1]-[N+M] (Testing) → Days [N+M+1]-[N+M+P] (Production)
```

### **Daily Progress Tracking**

**EOD Documentation Required**:
- **Day 1 Complete**: Foundation checkpoint
- **Day [N/2] Midpoint**: Implementation progress checkpoint
- **Day [N] Complete**: Implementation complete checkpoint
- **Day [N+M] Testing Complete**: Test validation checkpoint
- **Day [N+M+P] Production Ready**: Final handoff checkpoint

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Phase**: ANALYSIS + PLAN
**Duration**: [X] hours
**Status**: ✅ COMPLETE (this document represents Day 0 completion)

**Deliverables**:
- ✅ Analysis document: [Key analysis findings]
- ✅ Implementation plan (this document v1.0): [X]-day timeline, test examples
- ✅ Risk assessment: [N] critical pitfalls identified with mitigation strategies
- ✅ Existing code review: [Files analyzed]
- ✅ BR coverage matrix: [N] primary BRs mapped to test scenarios

---

### **Day 1: Foundation + Test Framework (DO-RED Phase)**

**Phase**: DO-RED
**Duration**: 8 hours
**TDD Focus**: Write failing tests first, enhance existing code

**⚠️ CRITICAL**: We are **ENHANCING existing code**, not creating from scratch!

**Existing Code to Enhance**:
- ✅ `[path/to/existing/file1.go]` ([LOC]) - [Current functionality]
- ✅ `[path/to/existing/file2.go]` - [Current integration point]
- ✅ `[path/to/config.go]` - [Current configuration]

**Morning (4 hours): Test Framework Setup + Code Analysis**

1. **Analyze existing implementation** (1 hour)
   - Read `[existing_file.go]` - understand current logic
   - Read `[integration_file.go]` - understand integration points
   - Identify what needs to be enhanced vs. created new

2. **Create test file** `test/unit/[service]/[feature]_test.go` (300-400 LOC)
   - Set up Ginkgo/Gomega test suite
   - Define test fixtures for NEW features
   - Create helper functions for feature testing

3. **Create integration test** `test/integration/[service]/[feature]_integration_test.go` (400-500 LOC)
   - Set up test infrastructure (Redis, K8s client, etc.)
   - Define integration test helpers for NEW features

**Afternoon (4 hours): Interface Enhancement + Failing Tests**

4. **Enhance** `[path/to/existing/file.go]` (add new methods, ~[X] LOC)
   ```go
   // EXISTING interface (keep as-is):
   type [ExistingType] struct {
       // ... existing fields ...
   }

   // NEW methods to add (interfaces only, no implementation yet):
   func ([receiver] *[ExistingType]) [NewMethod1]([params]) ([returns]) {
       return nil, fmt.Errorf("not implemented yet") // RED phase
   }
   ```

5. **Write failing tests** (strict TDD: ONE test at a time)
   - **TDD Cycle 1**: Test `[NewMethod1]` behavior
   - **TDD Cycle 2**: Test `[NewMethod2]` behavior
   - Run tests → Verify they FAIL (RED phase)

**EOD Deliverables**:
- ✅ Test framework complete
- ✅ [N] failing tests (RED phase)
- ✅ Enhanced interfaces defined
- ✅ Day 1 EOD report

**Validation Commands**:
```bash
# Verify tests fail (RED phase)
go test ./test/unit/[service]/[feature]_test.go -v 2>&1 | grep "FAIL"

# Expected: All tests should FAIL with "not implemented yet"
```

---

### **Day 2: Core Logic (DO-GREEN Phase)**

**Phase**: DO-GREEN
**Duration**: 8 hours
**TDD Focus**: Minimal implementation to pass tests

**Morning (4 hours): Core Feature Implementation**

1. **Implement** `[NewMethod1]` (minimal GREEN implementation)
   - Add basic logic to pass tests
   - No sophisticated logic yet (save for REFACTOR)
   - Focus on making tests pass

2. **Run tests** → Verify they PASS (GREEN phase)

**Afternoon (4 hours): Additional Methods**

3. **Implement** `[NewMethod2]` (minimal GREEN implementation)
4. **Run tests** → Verify they PASS (GREEN phase)

**EOD Deliverables**:
- ✅ Core methods implemented (GREEN phase)
- ✅ All unit tests passing
- ✅ Basic functionality working

**Validation Commands**:
```bash
# Verify tests pass (GREEN phase)
go test ./test/unit/[service]/[feature]_test.go -v

# Expected: All tests should PASS
```

---

### **Day 3-[N]: Additional Implementation Days**

**Phase**: DO-GREEN / DO-REFACTOR
**Duration**: 8 hours per day
**TDD Focus**: Complete feature implementation + enhancement

**Tasks** (adapt per day):
- Day 3: [Feature aspect 2]
- Day 4: [Feature aspect 3]
- Day [N-1]: [Integration preparation]
- Day [N]: **Integration with existing code**

**Integration Day Focus**:
1. **Enhance** `[path/to/server.go]` or main integration point
   - Wire up new feature to existing flow
   - Update configuration loading
   - Add feature flags if needed

2. **Update configuration** `[path/to/config.yaml]`
   - Add new configuration fields
   - Document default values
   - Add validation

**EOD Deliverables** (per day):
- ✅ [Feature aspect] implemented
- ✅ All tests passing
- ✅ Integration complete (Day [N])

---

### **Day [N+1]: Unit Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: Comprehensive unit test coverage

**Morning (4 hours): Core Unit Tests**

1. **Expand unit tests** to [70%+] coverage
   - Test edge cases
   - Test error conditions
   - Test boundary values

2. **Behavior & Correctness Validation**
   - Tests validate WHAT the system does (not HOW)
   - Clear business scenarios in test names
   - Specific assertions (not `ToNot(BeNil())`)

**Afternoon (4 hours): Test Refinement**

3. **Refactor tests** for clarity
   - Use `DescribeTable` for similar scenarios
   - Add business requirement comments (`// BR-XXX-YYY`)
   - Ensure tests survive implementation changes

**EOD Deliverables**:
- ✅ [70%+] unit test coverage
- ✅ All tests passing
- ✅ Tests follow behavior/correctness protocol

**Validation Commands**:
```bash
# Run unit tests with coverage
go test ./test/unit/[service]/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# Expected: total coverage ≥70%
```

---

### **Day [N+2]: Integration Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: Component interaction validation

**Morning (4 hours): Integration Test Implementation**

1. **Create integration tests** ([>50%] coverage of integration points)
   - Test with real dependencies (Redis, K8s API, etc.)
   - Test cross-component interactions
   - Test failure scenarios

2. **Infrastructure setup**
   - Ensure test infrastructure is reliable
   - Add cleanup logic (`AfterEach` with async waits)
   - Handle port collisions, resource conflicts

**Afternoon (4 hours): Integration Scenarios**

3. **Test realistic scenarios**
   - Multi-step workflows
   - Concurrent operations
   - Error recovery

**EOD Deliverables**:
- ✅ [N] integration test scenarios
- ✅ All integration tests passing
- ✅ Test infrastructure stable

**Validation Commands**:
```bash
# Run integration tests
make test-integration

# Expected: All integration tests pass
```

---

### **Day [N+3]: E2E Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: End-to-end feature validation

**Morning (4 hours): E2E Test Implementation**

1. **Create E2E tests** ([<10%] coverage, critical paths only)
   - Test complete feature lifecycle
   - Test business requirement fulfillment
   - Test user-facing behavior

2. **Business requirement validation**
   - Map each E2E test to BR-XXX-YYY
   - Validate success criteria
   - Document business outcomes

**Afternoon (4 hours): E2E Edge Cases**

3. **Test edge cases** (if critical)
   - Concurrent operations
   - State transitions
   - Failure recovery

**EOD Deliverables**:
- ✅ [N] E2E test scenarios
- ✅ All E2E tests passing
- ✅ BR validation complete

**Validation Commands**:
```bash
# Run E2E tests
make test-e2e

# Expected: All E2E tests pass, BRs validated
```

---

### **Day [N+M+1]: Documentation (PRODUCTION Phase)**

**Phase**: PRODUCTION
**Duration**: 8 hours
**Focus**: Finalize documentation and knowledge transfer

**📝 Note**: Most documentation is created **DURING** implementation (Days 1-[N+M]). This day is for **finalizing** and **consolidating** documentation.

---

#### **📊 Documentation Timeline - What Gets Created When**

```
Day 1-[N] (Implementation):
    ├── Code Documentation (inline GoDoc, BR references)
    ├── Daily EOD Reports (progress checkpoints)
    └── Configuration Comments (YAML inline docs)

Days [N+1]-[N+M] (Testing):
    ├── Test Documentation (test descriptions, BR mapping)
    ├── Test Helper Documentation
    └── Edge Case Documentation

Day [N+M+1] (Documentation Day): ⭐ YOU ARE HERE
    ├── Finalize Service Docs (update existing files)
    │   ├── overview.md (add feature, update version)
    │   ├── BUSINESS_REQUIREMENTS.md (add BRs, links)
    │   ├── testing-strategy.md (add test examples)
    │   ├── metrics-slos.md (add new metrics)
    │   └── security-configuration.md (update RBAC)
    │
    └── Create Operational Docs (new files if needed)
        ├── Runbook (if feature affects operations)
        ├── Migration Guide (if breaking changes)
        └── Configuration Guide (inline YAML comments)

Day [N+M+P] (Production Readiness):
    └── Handoff Summary (executive summary, lessons learned)
```

---

#### **Documentation Created DURING Implementation** (Days 1-[N+M])

These are created as you code (not at the end):

**Daily EOD Reports** (created each day):
- Day 1 EOD: Foundation checkpoint
- Day [N/2] EOD: Midpoint progress
- Day [N] EOD: Implementation complete
- Day [N+M] EOD: Testing complete

**Code Documentation** (created as you write code):
- GoDoc comments for public APIs
- Business requirement references (`// BR-XXX-YYY`)
- Inline explanations for complex logic
- Configuration field documentation

**Test Documentation** (created as you write tests):
- Test descriptions with business scenarios
- BR mapping in test comments
- Edge case documentation
- Test helper documentation

---

#### **Morning (4 hours): Finalize Service Documentation**

**What to Update** (these are existing service docs, not new files):

1. **Update `overview.md`** (service's main document)
   - Add feature description to "Features" section
   - Update architecture diagram (if changed)
   - Add feature to Table of Contents
   - Update version and changelog

2. **Update `BUSINESS_REQUIREMENTS.md`**
   - Add new BRs (BR-[SERVICE]-XXX)
   - Mark BRs as implemented
   - Link to implementation files
   - Link to test files

3. **Update `testing-strategy.md`**
   - Add test examples for new feature
   - Document test coverage for feature
   - Add any new testing patterns used

4. **Update `metrics-slos.md`** (if feature adds metrics)
   - Document new Prometheus metrics
   - Add Grafana dashboard panels
   - Update SLI/SLO targets

5. **Update `security-configuration.md`** (if feature changes RBAC)
   - Update ClusterRole permissions
   - Document new security considerations

---

#### **Afternoon (4 hours): Operational Documentation**

**What to Create/Update**:

1. **Create/Update Runbook** (if feature affects operations)
   - **File**: `docs/services/[service]/operations/[feature]-runbook.md`
   - **Content**:
     - Feature overview
     - Configuration guide
     - Operational procedures
     - Troubleshooting guide
     - Common issues and solutions
     - Monitoring and alerting

2. **Update Configuration Guide**
   - **File**: `config/[service].yaml` (inline comments)
   - **Content**:
     - Document new config fields
     - Provide examples
     - Document defaults
     - Document validation rules

3. **Create Migration Guide** (if breaking changes)
   - **File**: `docs/services/[service]/migrations/[version]-migration.md`
   - **Content**:
     - What changed
     - Migration steps
     - Backward compatibility notes
     - Rollback procedure

---

#### **EOD Deliverables**:
- ✅ Service documentation updated (`overview.md`, `BUSINESS_REQUIREMENTS.md`, etc.)
- ✅ Runbook created/updated (if needed)
- ✅ Configuration documented
- ✅ Migration guide created (if breaking changes)
- ✅ All inline code documentation complete

---

#### **Documentation Checklist**

**Service Documentation** (updates to existing files):
- [ ] `overview.md` - Feature added, version bumped, changelog updated
- [ ] `BUSINESS_REQUIREMENTS.md` - New BRs documented, links added
- [ ] `testing-strategy.md` - Test examples added
- [ ] `metrics-slos.md` - New metrics documented (if any)
- [ ] `security-configuration.md` - RBAC updated (if needed)

**Operational Documentation** (new files if needed):
- [ ] Runbook created (if feature affects operations)
- [ ] Configuration guide updated (inline comments in YAML)
- [ ] Migration guide created (if breaking changes)

**Code Documentation** (inline, created during implementation):
- [ ] GoDoc comments for all public APIs
- [ ] BR references in code comments
- [ ] Complex logic explained
- [ ] Configuration fields documented

**Test Documentation** (inline, created during testing):
- [ ] Test descriptions with business scenarios
- [ ] BR mapping in test comments
- [ ] Edge cases documented
- [ ] Test helpers documented

---

### **Day [N+M+P]: Production Readiness (PRODUCTION Phase)**

**Phase**: PRODUCTION
**Duration**: 8 hours
**Focus**: Final validation and handoff

**Morning (4 hours): Production Readiness Checklist**

1. **Complete production readiness checklist** (109-point system)
   - Build validation
   - Lint compliance
   - Test coverage
   - Documentation completeness
   - Deployment readiness

2. **Confidence assessment**
   - Calculate evidence-based confidence (60-100%)
   - Document assumptions
   - Identify risks

**Afternoon (4 hours): Handoff Summary**

3. **Create handoff summary** (450+ lines)
   - Executive summary
   - Architecture overview
   - Key decisions
   - Lessons learned
   - Known limitations
   - Future work

4. **Deployment plan**
   - Rollout strategy
   - Rollback plan
   - Monitoring plan
   - Success metrics

**EOD Deliverables**:
- ✅ Production readiness report
- ✅ Confidence assessment (≥60%)
- ✅ Handoff summary
- ✅ Deployment plan

---

## 🧪 **TDD Do's and Don'ts - MANDATORY**

### **✅ DO: Strict TDD Discipline**

1. **Write ONE test at a time** (not batched)
   ```go
   // ✅ CORRECT: TDD Cycle 1
   It("should [specific behavior]", func() {
       // Test for NewMethod1
   })
   // Run test → FAIL (RED)
   // Implement NewMethod1 → PASS (GREEN)
   // Refactor if needed

   // ✅ CORRECT: TDD Cycle 2 (after Cycle 1 complete)
   It("should [another specific behavior]", func() {
       // Test for NewMethod2
   })
   ```

2. **Test WHAT the system does** (behavior), not HOW (implementation)
   ```go
   // ✅ CORRECT: Behavior-focused
   It("should delay aggregation when threshold not reached", func() {
       _, shouldAggregate, err := feature.Process(ctx, input)
       Expect(err).ToNot(HaveOccurred())
       Expect(shouldAggregate).To(BeFalse(), "Should delay below threshold")
   })
   ```

3. **Use specific assertions** (not weak checks)
   ```go
   // ✅ CORRECT: Specific business assertions
   Expect(result.Status).To(Equal("aggregated"))
   Expect(result.Count).To(Equal(5))
   ```

### **❌ DON'T: Anti-Patterns to Avoid**

1. **DON'T batch test writing**
   ```go
   // ❌ WRONG: Writing 10 tests before any implementation
   It("test 1", func() { ... })
   It("test 2", func() { ... })
   It("test 3", func() { ... })
   // ... 7 more tests
   // Then implementing all at once
   ```

2. **DON'T test implementation details**
   ```go
   // ❌ WRONG: Testing internal state
   Expect(feature.internalBuffer).To(HaveLen(5))
   Expect(redisClient.Get(ctx, "internal:key")).To(Equal("value"))
   ```

3. **DON'T use weak assertions (NULL-TESTING)**
   ```go
   // ❌ WRONG: Weak assertions
   Expect(result).ToNot(BeNil())
   Expect(list).ToNot(BeEmpty())
   Expect(count).To(BeNumerically(">", 0))
   ```

**Reference**: `.cursor/rules/08-testing-anti-patterns.mdc` for automated detection

---

## 📊 **Test Examples**

### **📦 Package Naming Conventions - MANDATORY**

**AUTHORITY**: [TEST_PACKAGE_NAMING_STANDARD.md](../../testing/TEST_PACKAGE_NAMING_STANDARD.md)

**CRITICAL**: ALL tests use same package name as code under test (white-box testing).

| Test Type | Package Name | NO Exceptions |
|-----------|--------------|---------------|
| **Unit Tests** | `package [service]` | ✅ |
| **Integration Tests** | `package [service]` | ✅ |
| **E2E Tests** | `package [service]` | ✅ |

**Key Rule**: **NEVER** use `_test` suffix for ANY test type.

---

### **Unit Test Example**

```go
package [service]  // White-box testing - same package as code under test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "[import/path/to/types]"
    // No need to import [service] package - we're already in it
)

var _ = Describe("[Feature] Unit Tests", func() {
    var (
        ctx     context.Context
        feature *[FeatureType]
    )

    BeforeEach(func() {
        ctx = context.Background()
        feature = [FeatureConstructor]([dependencies])
    })

    Context("when [business scenario]", func() {
        It("should [business behavior]", func() {
            // BUSINESS SCENARIO: [Describe real-world situation]
            input := &types.[InputType]{
                Field1: "value1",
                Field2: "value2",
            }

            // BEHAVIOR: [What does the system do?]
            result, err := feature.[Method](ctx, input)

            // CORRECTNESS: [Are the results correct?]
            Expect(err).ToNot(HaveOccurred(), "System should [behavior]")
            Expect(result.[Field]).To(Equal([expected]), "[Business outcome]")

            // BUSINESS OUTCOME: [What business value was validated?]
            // This validates BR-[SERVICE]-XXX: [Requirement description]
        })
    })
})
```

### **Integration Test Example**

```go
package [service]  // White-box testing - same package as code under test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "[import/path/to/infrastructure]"
    // No need to import [service] package - we're already in it
)

var _ = Describe("[Feature] Integration Tests", func() {
    var (
        ctx         context.Context
        testServer  *[TestServer]
        redisClient *redis.Client
    )

    BeforeEach(func() {
        ctx = context.Background()
        testServer = infrastructure.NewTestServer([config])
        redisClient = infrastructure.NewTestRedis()
    })

    AfterEach(func() {
        testServer.Cleanup()
        redisClient.FlushDB(ctx)
    })

    Context("when [integration scenario]", func() {
        It("should [integration behavior]", func() {
            // BUSINESS SCENARIO: [Real-world integration scenario]

            // BEHAVIOR: [How do components interact?]
            response, err := testServer.Process([input])

            // CORRECTNESS: [Are integration results correct?]
            Expect(err).ToNot(HaveOccurred())
            Expect(response.Status).To(Equal([expected]))

            // BUSINESS OUTCOME: [Integration value validated]
        })
    })
})
```

### **E2E Test Example**

```go
package [service]  // White-box testing - same package as code under test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "[import/path/to/e2e/infrastructure]"
)

var _ = Describe("[Feature] E2E Tests", func() {
    var (
        ctx        context.Context
        k8sClient  client.Client
        testServer *[TestServer]
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = infrastructure.NewK8sClient()
        testServer = infrastructure.NewE2EServer([config])
    })

    AfterEach(func() {
        infrastructure.CleanupCRDs(ctx, k8sClient)
    })

    Context("when [end-to-end scenario]", func() {
        It("should [complete workflow behavior]", func() {
            // BUSINESS SCENARIO: [Complete user journey]

            // BEHAVIOR: [Full system workflow]
            result, err := testServer.ExecuteWorkflow([input])

            // CORRECTNESS: [Is the complete workflow correct?]
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Outcome).To(Equal([expected]))

            // BUSINESS OUTCOME: [End-to-end value validated]
            // This validates BR-[SERVICE]-XXX: [Complete requirement]
        })
    })
})
```

---

## 🎯 **BR Coverage Matrix**

| BR ID | Description | Unit Tests | Integration Tests | E2E Tests | Status |
|-------|-------------|------------|-------------------|-----------|--------|
| **BR-[SERVICE]-XXX** | [Requirement 1] | [Test files] | [Test files] | [Test files] | ✅ |
| **BR-[SERVICE]-YYY** | [Requirement 2] | [Test files] | [Test files] | [Test files] | ✅ |

**Coverage Calculation**:
- **Unit**: [X]/[Y] BRs covered ([Z]%)
- **Integration**: [X]/[Y] BRs covered ([Z]%)
- **E2E**: [X]/[Y] BRs covered ([Z]%)
- **Total**: [X]/[Y] BRs covered ([Z]%)

---

## 🚨 **Critical Pitfalls to Avoid**

### **1. [Pitfall Name]**
- ❌ **Problem**: [Description]
- ✅ **Solution**: [Mitigation strategy]
- **Impact**: [Business/technical impact]

### **2. [Pitfall Name]**
- ❌ **Problem**: [Description]
- ✅ **Solution**: [Mitigation strategy]
- **Impact**: [Business/technical impact]

---

## 📈 **Success Criteria**

### **Technical Success**
- ✅ All tests passing (Unit [70%+], Integration [>50%], E2E [<10%])
- ✅ No lint errors
- ✅ Code integrated with main application
- ✅ Documentation complete

### **Business Success**
- ✅ BR-[SERVICE]-XXX validated
- ✅ BR-[SERVICE]-YYY validated
- ✅ Success metrics achieved

### **Confidence Assessment**
- **Target**: ≥60% confidence
- **Calculation**: Evidence-based (test coverage + BR validation + integration status)

---

## 🔄 **Rollback Plan**

### **Rollback Triggers**
- Critical bug discovered in production
- Performance degradation >20%
- Business requirement not met

### **Rollback Procedure**
1. Revert feature flag (if used)
2. Deploy previous version
3. Verify rollback success
4. Document rollback reason

---

## 📚 **References**

### **Templates**
- [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](./SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) - Full service implementation
- [SERVICE_DOCUMENTATION_GUIDE.md](./SERVICE_DOCUMENTATION_GUIDE.md) - Documentation standards

### **Standards**
- [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc) - Testing framework
- [02-go-coding-standards.mdc](../../.cursor/rules/02-go-coding-standards.mdc) - Go patterns
- [08-testing-anti-patterns.mdc](../../.cursor/rules/08-testing-anti-patterns.mdc) - Testing anti-patterns

### **Examples**
- [DD-GATEWAY-008](./stateless/gateway-service/DD_GATEWAY_008_IMPLEMENTATION_PLAN.md) - Storm buffering (12 days)
- [DD-GATEWAY-009](./stateless/gateway-service/DD_GATEWAY_009_IMPLEMENTATION_PLAN.md) - State-based deduplication (5 days)

---

**Document Status**: 📋 **DRAFT**
**Last Updated**: YYYY-MM-DD
**Version**: 1.0
**Maintained By**: Development Team

---

## 📝 **Usage Instructions**

### **How to Use This Template**

1. **Copy this template** to your service directory:
   ```bash
   cp docs/services/FEATURE_EXTENSION_PLAN_TEMPLATE.md \
      docs/services/[service-type]/[service-name]/DD_[SERVICE]_[NUMBER]_IMPLEMENTATION_PLAN.md
   ```

2. **Replace all placeholders**:
   - `[DD-XXX-YYY]` → Your design decision number
   - `[Feature Name]` → Your feature name
   - `[Service Name]` → Your service name
   - `[X]`, `[Y]`, `[N]`, `[M]`, `[P]` → Your timeline numbers
   - `[BR-SERVICE-XXX]` → Your business requirements
   - All other `[placeholders]` with actual content

3. **Adjust timeline**:
   - **3-5 days**: Simple feature (1-2 files, minimal integration)
   - **5-8 days**: Medium feature (3-5 files, moderate integration)
   - **8-12 days**: Complex feature (5+ files, significant integration)

4. **Complete Day 0 (ANALYSIS + PLAN)**:
   - Analyze existing code
   - Identify integration points
   - Document risks and mitigation
   - Get plan approval

5. **Follow APDC-TDD methodology**:
   - Analysis → Plan → Do (RED → GREEN → REFACTOR) → Check

6. **Update version history** as you progress

---

## 🎯 **Template Customization Guide**

### **Timeline Adjustment**

**Simple Feature (3-5 days)**:
- Day 1: Foundation + Core logic (combined RED + GREEN)
- Day 2: Integration + Refactor
- Day 3: Unit tests
- Day 4: Integration + E2E tests
- Day 5: Documentation + Production readiness

**Medium Feature (5-8 days)**:
- Days 1-2: Foundation + Core logic
- Days 3-4: Integration + Additional features
- Day 5: Unit tests
- Day 6: Integration tests
- Day 7: E2E tests
- Day 8: Documentation + Production readiness

**Complex Feature (8-12 days)**:
- Use full template as-is (similar to DD-GATEWAY-008)

### **Section Customization**

**Minimal Plan** (simple features):
- Keep: Business Requirements, Timeline, Day-by-Day Breakdown, TDD Do's and Don'ts, Test Examples
- Optional: BR Coverage Matrix, Critical Pitfalls, Rollback Plan

**Comprehensive Plan** (complex features):
- Keep all sections
- Add: Performance Benchmarking, Troubleshooting Guide, Migration Guide (if breaking changes)

---

**Ready to implement your feature?** Start with Day 0 (ANALYSIS + PLAN) and follow the APDC-TDD methodology!

