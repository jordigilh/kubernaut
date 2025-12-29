# [DD-XXX-YYY]: [Feature Name] - Implementation Plan

**Version**: 1.4
**Filename**: `[CONTEXT_PREFIX_]IMPLEMENTATION_PLAN[_FEATURE_SUFFIX]_V1.4.md`
**Status**: 📋 DRAFT
**Design Decision**: [Link to DD-XXX-YYY]
**Service**: [Service Name]
**Confidence**: [60-100]% ([Evidence-Based/Estimated])
**Estimated Effort**: [3-12] days (APDC cycle: [X] days implementation + [Y] days testing + [Z] days documentation)

⚠️ **CRITICAL**: Filename version MUST match document version at all times.
- Document v1.0 → Filename `V1.0.md`
- Document v1.3 → Filename `V1.3.md` (rename file when bumping version)

---

## 🚨 **CRITICAL: Read This First**

**Before starting implementation, you MUST review these 5 critical pitfalls** (see line 954 for full details):

1. **Insufficient TDD Discipline** → Write ONE test at a time (not batched)
2. **Missing Integration Tests** → Integration tests BEFORE E2E tests
3. **Critical Infrastructure Without Unit Tests** → ≥70% coverage for critical components
4. **Late E2E Discovery** → Follow test pyramid (Unit → Integration → E2E)
5. **No Test Coverage Gates** → Automated CI/CD coverage gates

⚠️ **These pitfalls caused production issues in Audit Implementation (DD-STORAGE-012).**

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.0** | YYYY-MM-DD | Initial template created | ⏸️ Superseded |
| **v1.1** | 2025-11-21 | **CRITICAL**: Added explicit test file location requirements (`test/unit/`, `test/integration/`, `test/e2e/`). Added warnings against co-locating tests with source files. Clarified package naming conventions. | ⏸️ Superseded |
| **v1.2** | 2025-11-22 | **CRITICAL**: Added 5 mandatory pitfalls from Audit Implementation lessons learned (DD-STORAGE-012). Includes: (1) Insufficient TDD Discipline, (2) Missing Integration Tests for New Endpoints, (3) Critical Infrastructure Without Unit Tests, (4) Late E2E Discovery (Testing Tier Inversion), (5) No Test Coverage Gates. Added enforcement checklists, automated coverage gates, and evidence from DD-IMPLEMENTATION-AUDIT-V1.0.md. | ⏸️ Superseded |
| **v1.3** | 2025-11-23 | **ENHANCEMENTS**: Added filename version consistency rule, critical pitfalls summary at top, success metrics justification format, confidence calculation methodology, integration test environment decision section, operational sections checklist, and version bump checklist. | ⏸️ Superseded |
| **v1.4** | 2025-11-27 | **CORRECTION**: Removed incorrect exception "(unless testing a shared library in pkg/)" from test file location rules. ALL tests must be in `test/` directory - no exceptions for pkg/ libraries. | ✅ **CURRENT** |

---

## 🎯 **Business Requirements**

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-[SERVICE]-XXX** | [Primary requirement] | [Measurable success criteria] |
| **BR-[SERVICE]-YYY** | [Secondary requirement] | [Measurable success criteria] |

### **Success Metrics**

**Format**: `[Metric]: [Target] - *Justification: [Why this target?]*`

**Examples**:
- **Embedding Accuracy**: 92% top-1 accuracy - *Justification: 7% improvement over Model A prevents 46% fewer wrong workflow selections*
- **Cache Hit Rate**: >80% after 1 hour - *Justification: Same workflows queried frequently in semantic search*
- **P95 Latency**: <200ms - *Justification: User experience requires sub-second response times*

**Your Metrics**:
- **[Metric 1]**: [Target value] - *Justification: [Why this target?]*
- **[Metric 2]**: [Target value] - *Justification: [Why this target?]*
- **[Metric 3]**: [Target value] - *Justification: [Why this target?]*

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

**⚠️ CRITICAL: TEST FILE LOCATIONS - MANDATORY**

**AUTHORITY**: [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)

**ALL tests MUST be in the following locations**:
- ✅ **Unit Tests**: `test/unit/[service]/[feature]_test.go`
- ✅ **Integration Tests**: `test/integration/[service]/[feature]_integration_test.go`
- ✅ **E2E Tests**: `test/e2e/[service]/[feature]_e2e_test.go`

**❌ NEVER place tests**:
- ❌ Co-located with source files (e.g., `internal/controller/[service]/[feature]_test.go`)
- ❌ In `pkg/[package]/[feature]_test.go`

**Package Naming**: Tests use white-box testing (same package as code under test).

---

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

#### **Operational Sections Checklist**

**Required Sections** (add to plan):
- [ ] Prometheus Metrics (metrics + alert rules)
- [ ] Grafana Dashboard (panels + queries)
- [ ] Troubleshooting Guide (common issues + debug commands)
- [ ] Error Handling Philosophy (graceful degradation)
- [ ] Security Considerations (threat model + mitigations)
- [ ] Known Limitations (V1.0 limitations)
- [ ] Future Work (V1.1/V1.2/V2.0 roadmap)

**Optional Sections** (add if applicable):
- [ ] Performance Benchmarking (scenarios + baseline)
- [ ] Migration Strategy (if replacing existing functionality)
- [ ] Deployment Checklist (production readiness)

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

### **📁 Test File Locations - MANDATORY (READ THIS FIRST)**

**AUTHORITY**: [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)

**🚨 CRITICAL**: Test files MUST be in specific directories, NOT co-located with source code!

| Test Type | File Location | Example | ❌ WRONG Location |
|-----------|---------------|---------|-------------------|
| **Unit Tests** | `test/unit/[service]/` | `test/unit/notification/audit_test.go` | ❌ `internal/controller/notification/audit_test.go` |
| **Integration Tests** | `test/integration/[service]/` | `test/integration/notification/audit_integration_test.go` | ❌ `pkg/notification/audit_integration_test.go` |
| **E2E Tests** | `test/e2e/[service]/` | `test/e2e/notification/audit_e2e_test.go` | ❌ `cmd/notification/audit_e2e_test.go` |

**⚠️ Common Mistake**: Placing unit tests next to source files. This violates project structure.

**✅ Correct Pattern**:
```
internal/controller/notification/
    └── notificationrequest_controller.go   (source code)

test/unit/notification/
    └── audit_test.go                        (unit tests)
```

**❌ Wrong Pattern** (DO NOT DO THIS):
```
internal/controller/notification/
    ├── notificationrequest_controller.go   (source code)
    └── audit_test.go                        ❌ WRONG LOCATION
```

---

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

## 🏗️ **Integration Test Environment Decision**

### **Environment Strategy**

**Decision**: [Podman/KIND/envtest/Mocks]

**Environment Comparison**:

| Environment | Pros | Cons | Decision |
|-------------|------|------|----------|
| **Podman** | Fast, isolated, already available | Requires Podman installed | [✅ Selected / ❌ Not Selected] |
| **KIND** | Full K8s, realistic | Slow startup (30s+), overkill for integration | [✅ Selected / ❌ Not Selected] |
| **envtest** | Fast, lightweight | No real containers, limited realism | [✅ Selected / ❌ Not Selected] |
| **Mocks** | Fastest | Not integration tests, low confidence | [✅ Selected / ❌ Not Selected] |

**Rationale**: [Why this environment? Consider speed, realism, maintenance, CI/CD compatibility]

**Example** (Podman for Data Storage):
- **Decision**: Podman (PostgreSQL + Redis containers)
- **Rationale**: Fast startup (<5s), isolated per test process, reuses existing integration test infrastructure, realistic database interactions without K8s overhead

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

### **⚠️ LESSONS LEARNED FROM AUDIT IMPLEMENTATION (November 2025)**

**Context**: During the Audit Trail implementation (DD-STORAGE-012), we discovered critical mistakes that led to missing functionality (DLQ fallback) being caught only by E2E tests after the handler was considered "complete". These lessons are now mandatory to prevent in all future implementations.

**Evidence**: `docs/services/stateless/data-storage/DD-IMPLEMENTATION-AUDIT-V1.0.md`

---

### **1. Insufficient TDD Discipline** 🔴 **CRITICAL**

- ❌ **Problem**: New handler (`handleCreateAuditEvent`) was implemented **WITHOUT writing tests first**. DLQ fallback was added to code **WITHOUT corresponding test coverage**. Tests were written **AFTER** implementation, not before.
- ✅ **Solution**:
  - Write **ONE test at a time** (not in batches)
  - Follow strict **RED-GREEN-REFACTOR** sequence for every feature
  - **NEVER** write implementation code before writing a failing test
  - Each test must map to a specific **BR-[SERVICE]-XXX** requirement
- **Impact**: **CRITICAL** - DLQ functionality was **MISSING** in production code until E2E test caught it. High risk of data loss in production. Required emergency fix and comprehensive test backfill.
- **Evidence**: Unit Tests: 0/1 DLQ tests (0% coverage), Integration Tests: 1/3 DLQ tests (33% coverage), E2E Tests: 1/1 DLQ tests (100% coverage) - but **TOO LATE**

**Enforcement Checklist** (BLOCKING):
```
Before writing ANY implementation code:
- [ ] Unit test written and FAILING (RED phase)
- [ ] Test validates business behavior, not implementation
- [ ] Test uses specific assertions, not weak checks (no `ToNot(BeNil())`)
- [ ] Test maps to specific BR-[SERVICE]-XXX requirement
- [ ] Test run confirms FAIL with "not implemented yet" error
```

---

### **2. Missing Integration Tests for New Endpoints** 🔴 **CRITICAL**

- ❌ **Problem**: New unified audit events endpoint (`/api/v1/audit/events`) was implemented with **NO integration tests**. DLQ fallback functionality was missing because integration tests would have caught it early in the development cycle.
- ✅ **Solution**:
  - **Integration tests MUST exist BEFORE E2E tests** (MANDATORY)
  - Every new HTTP endpoint **MUST have integration tests** covering:
    - Success path
    - Failure paths
    - Edge cases (empty results, invalid filters, timeouts)
  - Integration tests must validate component interactions (e.g., handler → repository → database)
- **Impact**: **CRITICAL** - Critical functionality (DLQ fallback) was missing. E2E test was the first to catch the issue (too late in development cycle). Required backfilling integration tests after implementation.
- **Evidence**: Missing `EnqueueAuditEvent()` integration test, missing DLQ fallback integration test for `/api/v1/audit/events` endpoint, missing DLQ recovery worker integration test

**Integration Test Mandate** (BLOCKING):
```
For EVERY new HTTP endpoint:
- [ ] Integration test MUST exist before E2E test
- [ ] Integration test MUST cover success path
- [ ] Integration test MUST cover failure paths
- [ ] Integration test MUST cover edge cases
- [ ] Integration test validates component interactions (not just HTTP status codes)
```

---

### **3. Critical Infrastructure Without Unit Tests** 🔴 **CRITICAL**

- ❌ **Problem**: DLQ client (`pkg/datastorage/dlq/client.go`) was implemented with **ZERO unit tests**. DLQ client is **CRITICAL** infrastructure (prevents audit data loss), yet had no unit-level validation.
- ✅ **Solution**:
  - Identify critical infrastructure components **BEFORE implementation**
  - Critical infrastructure **MUST have ≥70% unit test coverage BEFORE integration tests**
  - Examples of critical infrastructure:
    - Fault tolerance mechanisms (DLQ, retry logic, circuit breakers)
    - Data persistence layers (repositories, database clients)
    - External service integrations (API clients, message queues)
    - Security components (authentication, authorization, encryption)
- **Impact**: **CRITICAL** - DLQ functionality was untested at unit level. Integration tests would have been easier to write with unit tests as foundation. High risk of bugs in critical fault tolerance mechanism.
- **Evidence**: Unit Tests: 0/1 DLQ tests (0% coverage). No unit tests for DLQ client prevented early detection of missing functionality.

**Critical Infrastructure Checklist** (BLOCKING):
```
Before implementing critical infrastructure:
- [ ] Component identified as critical (fault tolerance, data persistence, security)
- [ ] Unit test plan created (≥70% coverage target)
- [ ] Unit tests written FIRST (RED phase)
- [ ] Unit tests validate business behavior (not implementation details)
- [ ] Unit test coverage verified (≥70%) BEFORE integration tests
```

---

### **4. Late E2E Discovery (Testing Tier Inversion)** 🟡 **MEDIUM**

- ❌ **Problem**: E2E test was the **FIRST** to catch DLQ fallback missing. Unit tests and integration tests were **MISSING** or **INSUFFICIENT**. Testing pyramid was inverted (E2E before Unit/Integration).
- ✅ **Solution**:
  - Follow **Testing Pyramid Enforcement** (MANDATORY):
    - Day 1 (DO-RED): Unit tests (70%+ coverage)
    - Day 1 (DO-GREEN): Integration tests (>50% coverage)
    - Day 2 (CHECK): E2E tests (<10% coverage, critical paths only)
  - **NEVER** write E2E tests before unit/integration tests
  - E2E tests should validate **end-to-end workflows**, not unit-level logic
- **Impact**: **MEDIUM** - Critical functionality was missing until E2E test. E2E tests are slow and expensive (should catch integration issues, not unit issues). Required emergency fix and backfilling of unit/integration tests.
- **Evidence**: Timeline: Nov 19, 2025 (handler implemented) → Nov 20, 2025 (E2E Scenario 2 revealed DLQ was NOT implemented) → Nov 20, 2025 (emergency fix)

**Test Tier Sequence Checklist** (BLOCKING):
```
Before writing E2E tests:
- [ ] Unit tests exist for all critical components (≥70% coverage)
- [ ] Integration tests exist for all HTTP endpoints (≥50% coverage)
- [ ] All unit tests passing
- [ ] All integration tests passing
- [ ] E2E tests focus on end-to-end workflows (not unit logic)
```

---

### **5. No Test Coverage Gates (Process Gap)** 🟡 **MEDIUM**

- ❌ **Problem**: No automated enforcement of test coverage requirements. No blocking PR merge if critical components lack 2/3 tier coverage. Manual review was insufficient to catch DLQ test gap.
- ✅ **Solution**:
  - Add **automated test coverage gates** in CI/CD pipeline:
    - Unit test coverage gate: ≥70% (BLOCKING)
    - Integration test coverage gate: ≥50% (BLOCKING)
  - Add **manual review checklist** for PR approval:
    - All unit tests passing (≥70% coverage)
    - All integration tests passing (≥50% coverage)
    - Critical infrastructure has unit tests
    - All HTTP endpoints have integration tests
    - TDD RED-GREEN-REFACTOR sequence followed (evidence in commit history)
- **Impact**: **MEDIUM** - Critical functionality (DLQ) had insufficient test coverage. Issue was not caught until E2E test. Required process improvement recommendation.
- **Evidence**: Process Improvements recommendation: "Block PR merge if critical components (DLQ, graceful shutdown) lack 2/3 tier coverage"

**Test Coverage Gates** (AUTOMATED):
```bash
# In CI/CD pipeline (GitHub Actions)

# Step 1: Unit test coverage gate
- name: Check unit test coverage
  run: |
    go test ./pkg/[service]/... -coverprofile=coverage.out
    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
    if (( $(echo "$COVERAGE < 70" | bc -l) )); then
      echo "❌ Unit test coverage ($COVERAGE%) below 70% threshold"
      exit 1
    fi

# Step 2: Integration test coverage gate
- name: Check integration test coverage
  run: |
    go test ./test/integration/[service]/... -coverprofile=coverage.out
    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
    if (( $(echo "$COVERAGE < 50" | bc -l) )); then
      echo "❌ Integration test coverage ($COVERAGE%) below 50% threshold"
      exit 1
    fi
```

---

### **6. [Feature-Specific Pitfall]**
- ❌ **Problem**: [Description specific to your feature]
- ✅ **Solution**: [Mitigation strategy specific to your feature]
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

## 📊 **Confidence Calculation Methodology**

**Overall Confidence**: [X]% (Evidence-Based)

**Component Breakdown**:

| Component | Confidence | Evidence |
|-----------|-----------|----------|
| **[Component 1]** | [X]% | [Evidence: proven patterns, industry standards, battle-tested in production, etc.] |
| **[Component 2]** | [X]% | [Evidence: reference implementations, existing codebase patterns, etc.] |
| **[Component 3]** | [X]% | [Evidence: unit test coverage, integration test validation, etc.] |

**Example** (Embedding Service):
| Component | Confidence | Evidence |
|-----------|-----------|----------|
| **Model Selection** | 95% | Industry-standard `all-mpnet-base-v2`, proven accuracy in semantic search |
| **Sidecar Architecture** | 90% | Proven pattern in Gateway service, low-latency localhost communication |
| **Redis Cache** | 95% | Reusing Gateway's battle-tested Redis implementation |
| **Overall** | **93%** | Weighted average: (95% + 90% + 95%) / 3 |

**Risk Assessment**:
- **[Y]% Risk**: [Risk description with mitigation strategy]
- **Assumptions**: [Key assumptions that affect confidence]
- **Validation Approach**: [How will you validate these assumptions?]

**Calculation Formula**:
```
Overall Confidence = Σ(Component Confidence × Component Weight) / Σ(Component Weight)
```

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

**Document Status**: 📋 **TEMPLATE**
**Last Updated**: 2025-11-27
**Version**: 1.4
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

## 📋 **Version Bump Checklist**

### **When to Bump Version**

- **Patch (1.0.X)**: Bug fixes, clarifications, typo corrections
- **Minor (1.X.0)**: New sections added, scope expansion, template compliance fixes
- **Major (X.0.0)**: Significant architectural changes, complete rewrites

### **Version Bump Steps**

1. [ ] Update `**Version**` field in header (line 3)
2. [ ] Update `**Filename**` field in header (line 4)
3. [ ] Rename file to match new version (e.g., `V1.0.md` → `V1.3.md`)
4. [ ] Add entry to Version History table with detailed changes
5. [ ] Update all cross-references in related documents
6. [ ] Mark previous version as "⏸️ Superseded"
7. [ ] Mark current version as "✅ CURRENT"

**Example Version Bump** (1.2 → 1.3):
```bash
# Step 1: Rename file
mv docs/services/data-storage/IMPLEMENTATION_PLAN_V1.2.md \
   docs/services/data-storage/IMPLEMENTATION_PLAN_V1.3.md

# Step 2: Update cross-references
grep -r "IMPLEMENTATION_PLAN_V1.2.md" docs/ | # Find all references
# Update each reference to V1.3.md

# Step 3: Update version history in document
# Add v1.3 entry, mark v1.2 as Superseded
```

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

