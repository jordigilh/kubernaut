# Test Plan Best Practices - When and Why to Use Each Section

**Version**: 1.0.0
**Last Updated**: December 22, 2025
**Status**: AUTHORITATIVE

**Cross-References**:
- [Test Plan Template](../../../holmesgpt-api/tests/e2e/TEST_PLAN_WORKFLOW_CATALOG_EDGE_CASES.md) - Complete template with all sections
- [NT Test Plan](../../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md) - Real-world implementation example
- [Testing Strategy](./../../../.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth approach
- [Testing Guidelines](../business-requirements/TESTING_GUIDELINES.md) - BR coverage vs code coverage

---

## 🎯 **Purpose of This Document**

This document explains **when** and **why** to use each section of the Test Plan Template. Not all sections are mandatory for every test plan. Use this guide to decide what your test plan needs.

---

## 📋 **Decision Matrix: Which Sections Do I Need?**

| Your Situation | Required Sections | Optional Sections |
|---|---|---|
| **New feature with no existing tests** | All sections | None (all recommended) |
| **Adding tests to existing feature** | Current Test Status, Defense-in-Depth Summary, Tier sections, Execution Commands | Pre/Post Comparison, Day-by-Day Timeline |
| **Simple feature (<5 new tests)** | Defense-in-Depth Summary, Tier sections, Execution Commands | Current Test Status, Infrastructure Setup, Timeline |
| **Complex feature (10+ new tests)** | All sections | None (all recommended) |
| **MVP/time-constrained** | All sections (shows value proposition) | None (all needed for stakeholders) |
| **Template for other teams** | All sections with placeholders | None (comprehensive template) |

---

## 📊 **Section-by-Section Guidance**

### 1. **Header & Metadata** (MANDATORY)

**When to Use**: Always

**What to Include**:
- Version number (semantic versioning)
- Last updated date
- Status (DRAFT | READY FOR EXECUTION | ACTIVE | COMPLETE)
- Business Requirements covered (BR-XXX-XXX)
- Design Decisions referenced (DD-XXX-XXX)
- Cross-references to authoritative documents

**Example**:
```markdown
# [Service Name] [Feature Name] - Test Plan

**Version**: 1.0.0
**Last Updated**: 2025-12-22
**Status**: READY FOR EXECUTION

**Business Requirements**: BR-NOT-052 (Retry), BR-NOT-053 (Delivery)
**Design Decisions**: DD-METRICS-001 (Metrics Wiring)
```

**Why**: Provides context and traceability

---

### 2. **Changelog** (MANDATORY)

**When to Use**: Always (start from v1.0.0)

**What to Include**:
- Version number with date
- List of changes (ADDED, UPDATED, FIXED, REMOVED)
- Keep all historical versions

**Example**:
```markdown
## 📋 **Changelog**

### Version 1.1.0 (2025-12-22)
- **ADDED**: E2E infrastructure setup section
- **UPDATED**: Tier headers to include code coverage

### Version 1.0.0 (2025-12-22)
- Initial test plan for [Feature]
```

**Why**: Tracks evolution of test plan, shows what changed

---

### 3. **Testing Scope** (RECOMMENDED)

**When to Use**: When scope isn't obvious from title alone

**What to Include**:
- Architecture diagram showing components under test
- Clear scope definition (what's tested)
- Out-of-scope items (what's NOT tested)

**Example** ([NT Test Plan](../../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#-testing-scope)):
```markdown
## 🎯 **Testing Scope**

[ASCII diagram showing controller → delivery services]

**Scope**: Validate retry logic, multi-channel fanout, priority routing
**Out of Scope** (Post-MVP): Email channel, PagerDuty channel
```

**Why**: Prevents scope creep, sets stakeholder expectations

---

### 4. **Defense-in-Depth Testing Summary** (MANDATORY)

**When to Use**: Always

**What to Include**:
- Table with BR Coverage (overlapping) + Code Coverage (cumulative)
- Defense-in-depth principle explanation
- Example showing same BR/code tested at multiple tiers
- Current status (existing tests passing)
- MVP target (new tests needed)

**Template**:
```markdown
## 📊 **Defense-in-Depth Testing Summary**

**Strategy**: Overlapping BR coverage + cumulative code coverage approaching 100%

| Tier | Tests | Infrastructure | BR Coverage | Code Coverage | Status |
|------|-------|----------------|-------------|---------------|--------|
| Unit | [N] | None (mocked external deps) | 70%+ of ALL BRs | 70%+ | ✅ [X]/[N] passing |
| Integration | [M] | Real K8s (envtest) | >50% of ALL BRs | 50% | ✅ [Y]/[M] passing |
| E2E | [P] + [Q] NEW | Real K8s (Kind) | <10% BR coverage | 50% | ⏸️ [P] existing, [Q] new |

**Example - [BR-XXX-XXX: Feature Name]**:
- **Unit (70%)**: [What's tested] - tests `[file/function]`
- **Integration (50%)**: [What's tested] - tests same code with [real component]
- **E2E (50%)**: [What's tested] - tests same code in [deployed environment]
```

**Why**: Shows defense-in-depth strategy, clarifies overlapping coverage

**Reference**: [NT Test Plan Summary](../../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#-defense-in-depth-testing-summary)

---

### 5. **Current Test Status** (RECOMMENDED FOR EXISTING CODEBASES)

**When to Use**: When adding tests to existing codebase (not greenfield)

**What to Include**:
- Pre-[Feature] test status (what's already passing)
- Assessment: Are new tests needed per tier?
- Justification for "no new tests needed"

**Example** ([NT Test Plan](../../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#current-unit-test-coverage)):
```markdown
## 📊 **Current Test Status**

### Pre-MVP Status

| Test Suite | Tests | Status | Coverage |
|---|---|---|---|
| Controller reconciliation | 35 | ✅ Passing | BR-NOT-052, 053, 056 |
| Delivery services | 25 | ✅ Passing | BR-NOT-053 |
| Retry logic | 18 | ✅ Passing | BR-NOT-052 |

**Total Existing Tests**: 131 tests (117 unit + 9 integration + 5 E2E)

### Assessment

**Unit Tests**: ✅ **NO NEW UNIT TESTS NEEDED** - Existing coverage is comprehensive
**Integration Tests**: ✅ **NO NEW INTEGRATION TESTS NEEDED** - Existing coverage is sufficient
**E2E Tests**: ⏸️ **3 NEW E2E TESTS NEEDED** - Validate retry, fanout, priority routing
```

**Why**: Shows stakeholders what's already done vs. new work needed

**Skip If**: Greenfield project with no existing tests

---

### 6. **Tier Sections** (MANDATORY)

**When to Use**: Always (one section per tier: Unit, Integration, E2E)

**What to Include per Tier**:
- Location (file path)
- Infrastructure requirements
- Execution commands
- Infrastructure setup (see next section)
- Test suites with detailed scenarios
- Success criteria per test

**Header Format**:
```markdown
# 🧪 **TIER 1: UNIT TESTS** (70%+ BR Coverage | 70%+ Code Coverage)

**Location**: `[path/to/unit/tests]`
**Infrastructure**: None (mocked external dependencies)
**Execution**: `[command to run unit tests]`
```

**Why**: Provides complete test specification for implementation

**Reference**: [Template Tier Sections](../../../holmesgpt-api/tests/e2e/TEST_PLAN_WORKFLOW_CATALOG_EDGE_CASES.md#-tier-1-unit-tests-70-br-coverage--70-code-coverage)

---

### 7. **Infrastructure Setup** (CONDITIONAL)

**When to Use**:

| Tier | When to Include | When to Skip |
|------|----------------|--------------|
| **Unit** | Requires special fixtures, test data, or setup | Simple unit tests with just mocks |
| **Integration** | **MANDATORY** - Always include | Never skip |
| **E2E** | **MANDATORY** - Always include | Never skip |

**What to Include**:
- Prerequisites (cluster, services, dependencies)
- Setup commands (step-by-step)
- Verification commands (confirm setup worked)
- Cleanup commands (teardown)

**Example** ([NT Test Plan](../../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#%EF%B8%8F-e2e-infrastructure-setup)):
```markdown
## 🏗️ **E2E Infrastructure Setup**

### Prerequisites
- Kind cluster running
- Notification controller deployed
- File delivery channel configured

### Setup Commands
```bash
# 1. Create Kind cluster
make kind-up

# 2. Deploy controller
make deploy-notification

# 3. Verify controller
kubectl get pods -n kubernaut-system | grep notification
```

### Infrastructure Validation
```bash
make validate-e2e-notification-infrastructure

# Expected checks:
# ✅ Kind cluster accessible
# ✅ Controller deployed
# ✅ CRDs registered
```
```

**Why**: Ensures reproducible test environment, helps new team members

**Skip If**: Unit tests with no special setup

---

### 8. **Test Outcomes by Tier** (RECOMMENDED)

**When to Use**: When stakeholders need to understand what failures mean

**What to Include**:
- What each tier proves
- What a failure in that tier means
- Code coverage contribution

**Example**:
```markdown
# 🎯 **Test Outcomes by Tier**

| Tier | What It Proves | Failure Means | Code Coverage |
|------|----------------|---------------|---------------|
| Unit | Retry algorithm is correct | Bug in retry logic code | 70%+ |
| Integration | Retry works with real K8s API | Kubernetes API integration issue | 50% |
| E2E | Retry works in deployed controller | System doesn't retry in production | 50% |
```

**Why**: Helps teams diagnose failures faster

**Skip If**: Obvious from test names what each tier tests

---

### 9. **Expected Outcomes (Pre/Post Comparison)** (RECOMMENDED FOR STAKEHOLDERS)

**When to Use**: When you need to show value proposition to stakeholders

**What to Include**:
- Pre-[Feature] status (existing tests, confidence)
- Post-[Feature] target (new tests, improved confidence)
- Confidence improvement (quantified)
- Rationale (why confidence improves)

**Example** ([NT Test Plan](../../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#-expected-outcomes)):
```markdown
## 🎉 **Expected Outcomes**

### Pre-MVP Status:
- ✅ 131 tests passing (117 unit + 9 integration + 5 E2E)
- ✅ 100% pass rate
- ✅ 95% confidence for production

### Post-MVP Status (Target):
- ✅ 134 tests passing (117 unit + 9 integration + 8 E2E)
- ✅ 100% pass rate
- ✅ 99% confidence for production

### Confidence Improvement:
- **Before MVP**: 95% confidence
- **After MVP**: 99% confidence
- **Improvement**: +4% confidence increase

**Rationale**: Retry logic validated in real cluster, multi-channel fanout tested end-to-end
```

**Why**: Shows stakeholders clear ROI for test development effort

**Skip If**: Internal team test plan with no stakeholder review

---

### 10. **Implementation Checklist** (MANDATORY)

**When to Use**: Always (high-level tracking)

**What to Include**:
- Week-level checklist (not day-level)
- Test groups or phases
- Clear completion criteria

**Example**:
```markdown
# ✅ **Implementation Checklist**

## Week 1: Unit Tests
- [ ] U1.1-U1.8: Input validation
- [ ] U2.1-U2.5: Response transformation

## Week 2: Integration & E2E
- [ ] I1.1-I1.5: Contract validation
- [ ] E1.1-E1.2: Critical paths
```

**Why**: High-level progress tracking

**Always Include**: Use for all test plans

---

### 11. **Execution Timeline (Day-by-Day)** (RECOMMENDED FOR COMPLEX FEATURES)

**When to Use**: 

| Situation | When to Include |
|---|---|
| **Complex feature** (10+ new tests, multiple owners) | **MANDATORY** |
| **MVP/time-constrained** | **MANDATORY** (shows feasibility) |
| **Simple feature** (<5 new tests) | Optional (checklist is enough) |
| **Single owner, flexible timeline** | Optional |

**What to Include**:
- Day-by-day breakdown (not just week-level)
- Task name, time estimate, owner, deliverable
- Total time estimate
- Critical path dependencies

**Example** ([NT Test Plan](../../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#%EF%B8%8F-execution-timeline)):
```markdown
## ⏱️ **Execution Timeline**

### Week 1: Core MVP E2E Tests

| Day | Task | Time | Owner | Deliverable |
|---|---|---|---|---|
| **Day 1** | E2E-1: Retry and Exponential Backoff | 1 day | NT Team | Test file + passing |
| **Day 2 AM** | E2E-2: Multi-Channel Fanout | 0.5 day | NT Team | Test file + passing |
| **Day 2 PM** | E2E-3: Priority-Based Routing | 0.5 day | NT Team | Test file + passing |

**Total Time**: **2 days**
**Critical Path**: E2E tests require Kind cluster setup first
```

**Why**: Actionable execution plan, helps with resource allocation

**Skip If**: Simple feature or single owner with flexible timeline

---

### 12. **Execution Commands** (MANDATORY)

**When to Use**: Always

**What to Include**:
- Commands to run each tier
- Commands to run full suite
- Commands for specific tests (if needed)
- Commands with coverage reporting

**Example**:
```markdown
# 📊 **Execution Commands**

```bash
# Run unit tests
make test-unit-notification

# Run integration tests
make test-integration-notification

# Run E2E tests
make test-e2e-notification

# Run full suite
make test-notification-all

# Run with coverage
go test ./test/unit/notification/... -v -coverprofile=coverage.out
```
```

**Why**: Makes test plan immediately actionable

**Always Include**: Use for all test plans

---

### 13. **File Structure** (RECOMMENDED)

**When to Use**: When file organization isn't obvious

**What to Include**:
- Directory tree showing test files
- Existing vs. new files marked clearly
- Suite setup files

**Example** ([NT Test Plan](../../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#-file-structure)):
```markdown
# 📁 **File Structure**

```
test/e2e/notification/
├── notification_e2e_suite_test.go                     # Suite setup (existing)
│
├── ✅ EXISTING E2E TESTS (5 tests - ALL PASSING)
├── 01_notification_lifecycle_audit_test.go
├── 02_audit_correlation_test.go
│
├── ⏸️ NEW MVP E2E TESTS (3 tests - TO IMPLEMENT)
├── 05_retry_exponential_backoff_test.go               # NEW
├── 06_multi_channel_fanout_test.go                    # NEW
└── 07_priority_routing_test.go                        # NEW
```
```

**Why**: Helps teams navigate test codebase

**Skip If**: Single test file or obvious structure

---

### 14. **References** (RECOMMENDED)

**When to Use**: When test plan references authoritative documents

**What to Include**:
- Authoritative documents (BRs, DDs, guidelines)
- Existing test files
- Planning documents
- Cross-references to related test plans

**Example** ([NT Test Plan](../../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#-references)):
```markdown
# 📚 **References**

### Authoritative Documents
- `BUSINESS_REQUIREMENTS.md` - 18 BRs with acceptance criteria
- `testing-strategy.md` - Defense-in-depth approach
- `.cursor/rules/03-testing-strategy.mdc` - Overlapping BR coverage

### Existing Tests
- `test/e2e/notification/01_notification_lifecycle_audit_test.go`
- `test/e2e/notification/02_audit_correlation_test.go`

### Planning Documents
- `NT_V1_0_ROADMAP.md` - Master roadmap
```

**Why**: Provides traceability and context

**Skip If**: Standalone test plan with no external references

---

## 🎯 **Quick Decision Guide**

### "I'm writing a test plan for..."

#### **New MVP feature with stakeholder review**
✅ Use: **All sections**
- Stakeholders need value proposition (Pre/Post Comparison)
- Timeline shows feasibility (Day-by-Day)
- Infrastructure setup ensures reproducibility

#### **Adding E2E tests to existing well-tested feature**
✅ Use: Current Test Status, Defense-in-Depth Summary, Tier 3 (E2E), Infrastructure Setup, Execution Commands
❌ Skip: Pre/Post Comparison (stakeholders already approved), Day-by-Day Timeline (small scope)

#### **Simple feature with 2-3 new unit tests**
✅ Use: Defense-in-Depth Summary, Tier 1 (Unit), Execution Commands
❌ Skip: Current Test Status (obvious), Infrastructure Setup (no infrastructure), Timeline (too small)

#### **Complex refactoring with 20+ new tests across all tiers**
✅ Use: **All sections**
- Current Test Status shows baseline
- Day-by-Day Timeline is critical for resource planning
- Pre/Post Comparison justifies effort

---

## 📊 **Common Mistakes to Avoid**

### ❌ **Mistake 1**: Copying template sections without customization
**Why Bad**: Generic placeholders don't help teams
**Fix**: Replace ALL `[placeholders]` with actual values, or remove unused sections

### ❌ **Mistake 2**: Skipping Infrastructure Setup for Integration/E2E tests
**Why Bad**: New team members can't reproduce tests
**Fix**: Always include Infrastructure Setup for Integration and E2E tiers

### ❌ **Mistake 3**: Using old tier headers (70% Coverage) instead of new notation
**Why Bad**: Ambiguous - is it BR coverage or code coverage?
**Fix**: Always use `(70%+ BR Coverage | 70%+ Code Coverage)` notation

### ❌ **Mistake 4**: No version number or changelog
**Why Bad**: Can't track test plan evolution
**Fix**: Start at v1.0.0, increment with each change, maintain changelog

### ❌ **Mistake 5**: Day-by-Day Timeline for simple features
**Why Bad**: Overkill for 2-3 tests, creates maintenance burden
**Fix**: Use Implementation Checklist for simple features, Timeline only for complex

---

## ✅ **Quality Checklist**

Before finalizing your test plan, verify:

- [ ] Version number and changelog present
- [ ] Defense-in-Depth Summary includes BR coverage + code coverage
- [ ] Tier headers use new notation: `(70%+ BR Coverage | 70%+ Code Coverage)`
- [ ] Infrastructure Setup included for Integration and E2E tiers
- [ ] Execution commands are copy-pasteable and work
- [ ] Cross-references to authoritative documents are valid
- [ ] All `[placeholders]` replaced with actual values or sections removed
- [ ] Current Test Status included if adding to existing codebase
- [ ] Pre/Post Comparison included if stakeholder review needed
- [ ] Day-by-Day Timeline included if complex feature (10+ tests) or MVP

---

## 🔗 **Cross-References**

- **[Test Plan Template](../../../holmesgpt-api/tests/e2e/TEST_PLAN_WORKFLOW_CATALOG_EDGE_CASES.md)** - Complete template with all sections and placeholders
- **[NT Test Plan](../../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md)** - Real-world example showing all sections in use
- **[Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)** - Defense-in-depth approach (authoritative)
- **[Testing Guidelines](../business-requirements/TESTING_GUIDELINES.md)** - BR coverage vs code coverage distinction

---

**Status**: ✅ **AUTHORITATIVE** - Use this guide for all Kubernaut test plans
**Owner**: Testing Standards Team
**Next Review**: Q1 2026 (or after 5 new test plans created)

