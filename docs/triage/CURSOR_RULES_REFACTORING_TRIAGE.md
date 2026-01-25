# Cursor Rules Refactoring Triage

**Date**: January 21, 2026
**Status**: Analysis Complete - Awaiting Decision
**Priority**: High - Rules verbosity impacts AI assistant performance

---

## 📊 **Executive Summary**

**Problem**: Cursor rules are verbose (6,389 lines across 22 files) with significant duplication and implementation details that should be in external documentation.

**Impact**:
- AI assistant context pollution (rules loaded on every interaction)
- Duplication across 3-5 files for core concepts
- Implementation examples and bash scripts embedded in rules
- Unclear hierarchy with 5 files claiming "foundational" status

**Recommendation**: **Option C** - Start with 3 worst offenders as proof of concept (79% reduction: 2,274 → 480 lines)

---

## 🔍 **Current State Analysis**

### **Verbosity Breakdown**

| File | Lines | Status | Primary Issues |
|------|-------|--------|----------------|
| `03-testing-strategy.mdc` | **1090** | 🔴 Critical | Full code examples, test patterns, mock implementations |
| `00-core-development-methodology.mdc` | **563** | 🔴 Critical | Massive APDC details, duplicates 00-project-guidelines |
| `08-testing-anti-patterns.mdc` | **474** | 🔴 Critical | Full bash detection scripts, git hooks |
| `14-design-decisions-documentation.mdc` | **458** | 🟡 Review | Documentation templates (should be in docs/) |
| `00-ai-assistant-behavioral-constraints-consolidated.mdc` | **377** | 🔴 Critical | Detailed tool call examples, XML syntax |
| `00-ai-assistant-methodology-enforcement.mdc` | **363** | 🟡 Review | Overlaps with constraints file |
| `kubernaut-collaboration-rules.mdc` | **110** | ✅ **MODEL** | **THIS IS THE TARGET FORMAT** |

**Total**: 6,389 lines (should be ~800-1000 lines)

---

## 🎯 **Existing Documentation Discovery**

### **✅ APDC Methodology - ALREADY EXISTS**

**Location**: `docs/development/methodology/`
- ✅ `APDC_FRAMEWORK.md` (632 lines) - **Complete APDC guide with examples**
- ✅ `APDC_QUICK_REFERENCE.md` (210 lines) - **Quick reference card**

**Status**: **NO NEED TO CREATE NEW DOCS** - Already comprehensive!

**Action**: Rules should **reference** these docs, not duplicate them.

---

### **✅ Testing Patterns - ALREADY EXISTS**

**Location**: `docs/testing/`
- ✅ `README.md` - Testing documentation index
- ✅ `PYRAMID_TEST_MIGRATION_GUIDE.md` - Complete pyramid strategy
- ✅ `TESTING_PATTERNS_QUICK_REFERENCE.md` (189 lines) - **Pattern quick reference**
- ✅ `QUICK_REFERENCE.md` (235 lines) - **CI/CD and test tier reference**
- ✅ `INTEGRATION_E2E_NO_MOCKS_POLICY.md` - Mock policy details
- ✅ `TESTING_GUIDELINES_TRANSFORMATION_GUIDE.md` - Transformation patterns

**Status**: **NO NEED TO CREATE NEW DOCS** - Already comprehensive!

**Action**: Rules should **reference** these docs, not duplicate test patterns.

---

### **✅ AI/ML Patterns - PARTIALLY EXISTS**

**Location**: `docs/architecture/decisions/`
- ✅ `DD-LLM-003-mock-first-development-strategy.md` - Mock-first strategy
- ✅ `DD-TEST-011-mock-llm-self-discovery-pattern.md` - Self-discovery pattern
- ✅ `DD-TEST-005-metrics-unit-testing-standard.md` - Metrics testing

**Status**: **SUFFICIENT** - Design decisions cover AI/ML patterns

**Action**: Rules should **reference** these DDs, not duplicate AI patterns.

---

### **❌ Testing Anti-Patterns - MISSING**

**Location**: None found
**Status**: **NEEDS CREATION** - But only detection logic, not full scripts

**Recommendation**: Create `docs/testing/ANTI_PATTERN_DETECTION.md` with:
- Anti-pattern definitions (from rule file)
- Detection commands (reference to scripts/)
- Examples of violations and fixes

---

### **❌ Validation Scripts - PARTIALLY EXISTS**

**Location**: `scripts/`
- ✅ `validate-openapi-client-usage.sh`
- ✅ `validate-service-maturity.sh`
- ✅ `verify-test-package-names.sh`
- ❌ Missing: `check-test-anti-patterns.sh`
- ❌ Missing: `check-business-integration.sh`
- ❌ Missing: `check-tdd-compliance.sh`

**Status**: **NEEDS CREATION** - Extract bash scripts from rule files

**Action**: Create missing validation scripts, reference from rules.

---

## 🔧 **Refactoring Options**

### **Option A: Radical Consolidation** (Recommended for Long-term)

**Structure**:
```
.cursor/rules/
  ├── 00-kubernaut-core-rules.mdc       [NEW - 250 lines]
  │   ├── Mandatory Principles
  │   ├── TDD Workflow (reference APDC_FRAMEWORK.md)
  │   ├── AI Assistant Behavior (checkpoints only)
  │   ├── Testing Strategy (reference testing/)
  │   └── Code Quality Standards
  │
  ├── 01-specialized-rules.mdc          [NEW - 150 lines]
  │   ├── AI/ML Patterns (reference DDs)
  │   ├── Kubernetes Safety
  │   └── Container Deployment
  │
  └── [Archive existing files]
```

**Pros**:
- Maximum clarity (single source of truth)
- Minimal duplication
- Fastest AI assistant context loading

**Cons**:
- Requires team review and approval
- Significant restructuring effort
- Existing references need updating

**Effort**: 4-6 hours

---

### **Option B: Aggressive Trimming** (Alternative)

**Keep existing structure, reduce each file by 60-80%**:

| File | Current | Target | Reduction |
|------|---------|--------|-----------|
| `03-testing-strategy.mdc` | 1090 → **250** | 77% | Extract examples to docs/testing/ |
| `00-core-development-methodology.mdc` | 563 → **150** | 73% | Reference APDC_FRAMEWORK.md |
| `00-ai-assistant-*.mdc` (2 files) | 740 → **200** | 73% | Consolidate, remove examples |
| `08-testing-anti-patterns.mdc` | 474 → **100** | 79% | Move scripts to scripts/ |

**Total Reduction**: 2,867 → 700 lines (76% reduction)

**Pros**:
- Preserves existing structure
- Incremental approach
- Lower risk

**Cons**:
- Still some duplication
- Multiple files to maintain
- Less dramatic improvement

**Effort**: 3-4 hours

---

### **Option C: Proof of Concept** (Recommended for Immediate Action)

**Start with 3 worst offenders**:

1. **`03-testing-strategy.mdc`** (1090 → 200 lines)
   - Keep: Pyramid strategy, mock matrix, K8s client mandate
   - Remove: All code examples → reference `docs/testing/TESTING_PATTERNS_QUICK_REFERENCE.md`
   - Remove: Detailed patterns → reference `docs/testing/PYRAMID_TEST_MIGRATION_GUIDE.md`

2. **Merge `00-core-methodology` + `00-project-guidelines`** (710 → 200 lines)
   - Keep: APDC overview, TDD workflow, BR mandate
   - Remove: Full phase specs → reference `docs/development/methodology/APDC_FRAMEWORK.md`
   - Remove: Approval gates → reference `docs/development/methodology/APDC_QUICK_REFERENCE.md`

3. **`08-testing-anti-patterns.mdc`** (474 → 80 lines)
   - Keep: Anti-pattern definitions, quick examples
   - Remove: Bash scripts → create `scripts/validation/check-test-anti-patterns.sh`
   - Remove: Git hooks → create `docs/testing/ANTI_PATTERN_DETECTION.md`

**Result**: 2,274 → 480 lines (79% reduction)

**Pros**:
- Immediate impact on worst offenders
- Proves the approach works
- Can expand to other files if successful
- Leverages existing docs (no duplication)

**Cons**:
- Partial solution
- Other files still verbose

**Effort**: 2-3 hours

---

## 📋 **Detailed Refactoring Plan**

### **Phase 1: Create Missing Documentation** (30 minutes)

#### **1.1 Anti-Pattern Detection Guide**
**File**: `docs/testing/ANTI_PATTERN_DETECTION.md`
**Content**:
- Anti-pattern definitions (from `08-testing-anti-patterns.mdc`)
- Detection commands (reference to scripts/)
- Examples of violations and fixes
- Integration with CI/CD

**Source**: Extract from `08-testing-anti-patterns.mdc` lines 60-375

---

#### **1.2 Validation Scripts**
**Location**: `scripts/validation/`

**Create**:
```bash
scripts/validation/
  ├── check-test-anti-patterns.sh      # From 08-testing-anti-patterns.mdc
  ├── check-business-integration.sh    # From 00-core-methodology.mdc
  └── check-tdd-compliance.sh          # From 00-ai-assistant-*.mdc
```

**Source**: Extract bash scripts from rule files

---

### **Phase 2: Refactor Rule Files** (1.5-2 hours)

#### **2.1 Testing Strategy** (`03-testing-strategy.mdc`)

**Before** (1090 lines):
- Full code examples (300+ lines)
- Detailed Ginkgo/Gomega patterns
- Mock implementation examples
- Test suite structures

**After** (200 lines):
```markdown
# Testing Strategy

## Defense-in-Depth Pyramid
- **Unit**: 70%+ coverage (real business logic, external mocks only)
- **Integration**: <20% coverage (critical interactions)
- **E2E**: <10% coverage (essential user journeys)

**See**: [Complete Testing Guide](../../docs/testing/PYRAMID_TEST_MIGRATION_GUIDE.md)

## Kubernetes Client Mandate
**MANDATORY**: Use `fake.NewClientBuilder()` for unit tests
**See**: [ADR-004](../../docs/architecture/decisions/ADR-004-fake-kubernetes-client.md)

## Mock Strategy
**External Dependencies**: Mock (K8s API, LLM, Vector DB)
**Business Logic**: Real (all pkg/ components)
**See**: [Testing Patterns Quick Reference](../../docs/testing/TESTING_PATTERNS_QUICK_REFERENCE.md)

## Anti-Patterns
**Forbidden**: NULL-TESTING, STATIC DATA, LIBRARY TESTING
**Detection**: Run `make lint-test-patterns`
**See**: [Anti-Pattern Detection Guide](../../docs/testing/ANTI_PATTERN_DETECTION.md)
```

---

#### **2.2 Core Methodology** (Merge 2 files)

**Before** (710 lines):
- `00-core-development-methodology.mdc` (563 lines)
- `00-project-guidelines.mdc` (147 lines)

**After** (200 lines):
```markdown
# Kubernaut Core Development Rules

## Mandatory Principles
1. **Business Requirements**: Every change backed by BR-[CATEGORY]-[NUMBER]
2. **TDD Workflow**: RED → GREEN → REFACTOR (tests first, always)
3. **Critical Decisions**: Ask for input on architecture, dependencies, security
4. **APDC Methodology**: Analysis → Plan → Do → Check for complex tasks

**See**: [APDC Framework Guide](../../docs/development/methodology/APDC_FRAMEWORK.md)

## APDC Quick Reference
- **Analysis** (5-15 min): Context + business alignment + risk assessment
- **Plan** (10-20 min): Strategy + TDD mapping + **user approval required**
- **Do** (Variable): RED → GREEN → REFACTOR with checkpoints
- **Check** (5-10 min): Validation + confidence assessment

**See**: [APDC Quick Reference](../../docs/development/methodology/APDC_QUICK_REFERENCE.md)

## AI Assistant Checkpoints
- **CHECKPOINT A**: Validate type definitions before field access
- **CHECKPOINT B**: Search existing implementations before creating new
- **CHECKPOINT C**: Verify main application integration for business code
- **CHECKPOINT D**: Complete symbol analysis before fixing build errors

## Code Quality Standards
- **Error Handling**: Always handle errors, log every error
- **Type Safety**: Avoid `any`/`interface{}` unless necessary
- **Business Integration**: All business code integrated in cmd/
- **Testing**: Ginkgo/Gomega BDD framework, map tests to BRs

**See**: [Go Coding Standards](.cursor/rules/02-go-coding-standards.mdc)
```

---

#### **2.3 Testing Anti-Patterns** (`08-testing-anti-patterns.mdc`)

**Before** (474 lines):
- Full bash detection scripts (100+ lines)
- Git hook implementations
- Automated detection logic

**After** (80 lines):
```markdown
# Testing Anti-Patterns

## Critical Anti-Patterns

### NULL-TESTING (Most Critical)
**Violation**: Testing existence instead of business outcomes
```go
// ❌ FORBIDDEN
Expect(result).ToNot(BeNil())
Expect(items).ToNot(BeEmpty())

// ✅ CORRECT
Expect(workflow.SafetyValidation).To(ContainElement("resource-limits"))
Expect(analysis.ConfidenceScore).To(BeNumerically(">=", 0.8))
```

### STATIC DATA TESTING
**Violation**: Testing hardcoded values instead of business logic
```go
// ❌ FORBIDDEN
input := "test-alert-name"
Expect(result.Name).To(Equal("test-alert-name"))

// ✅ CORRECT
input := testutil.GenerateAlertName()
Expect(result.Name).To(MatchRegexp("^alert-[0-9]+$"))
```

### LIBRARY TESTING
**Violation**: Testing third-party libraries instead of business logic
**Rule**: If it's not in `pkg/`, don't test it

## Detection
**Command**: `make lint-test-patterns`
**Script**: `scripts/validation/check-test-anti-patterns.sh`

**See**: [Anti-Pattern Detection Guide](../../docs/testing/ANTI_PATTERN_DETECTION.md)
```

---

### **Phase 3: Update Makefile** (15 minutes)

**Add validation targets**:

```makefile
# Cursor rule compliance checks
.PHONY: lint-rules
lint-rules: lint-test-patterns lint-business-integration lint-tdd-compliance

.PHONY: lint-test-patterns
lint-test-patterns:
	@echo "Checking for test anti-patterns..."
	@scripts/validation/check-test-anti-patterns.sh

.PHONY: lint-business-integration
lint-business-integration:
	@echo "Checking business code integration..."
	@scripts/validation/check-business-integration.sh

.PHONY: lint-tdd-compliance
lint-tdd-compliance:
	@echo "Checking TDD compliance..."
	@scripts/validation/check-tdd-compliance.sh
```

---

### **Phase 4: Validation** (30 minutes)

1. **Verify references**: Ensure all referenced docs exist
2. **Test AI assistant**: Check if rules still work with reduced content
3. **Run validation scripts**: Ensure scripts work as expected
4. **Update README**: Document new structure

---

## 📊 **Impact Assessment**

### **Before Refactoring**
- **Total Lines**: 6,389 lines across 22 files
- **Duplication**: 3-5 files repeat core concepts
- **AI Context**: ~6.4KB loaded per interaction
- **Maintenance**: High (update 3-5 files for single concept change)

### **After Refactoring (Option C)**
- **Total Lines**: 4,595 lines across 22 files (28% reduction)
- **Worst Offenders**: 2,274 → 480 lines (79% reduction)
- **AI Context**: ~4.6KB loaded per interaction (28% reduction)
- **Maintenance**: Medium (still some duplication in untouched files)

### **After Full Refactoring (Option A)**
- **Total Lines**: ~1,000 lines across 2-3 files (84% reduction)
- **Duplication**: Eliminated
- **AI Context**: ~1KB loaded per interaction (84% reduction)
- **Maintenance**: Low (single source of truth)

---

## 🚀 **Recommended Next Steps**

### **Immediate Action (Option C)**

1. ✅ **Approve this triage** (5 minutes)
2. ✅ **Create missing docs** (30 minutes)
   - `docs/testing/ANTI_PATTERN_DETECTION.md`
   - `scripts/validation/check-test-anti-patterns.sh`
   - `scripts/validation/check-business-integration.sh`
   - `scripts/validation/check-tdd-compliance.sh`
3. ✅ **Refactor 3 worst files** (1.5 hours)
   - `03-testing-strategy.mdc` (1090 → 200 lines)
   - Merge `00-core-methodology` + `00-project-guidelines` (710 → 200 lines)
   - `08-testing-anti-patterns.mdc` (474 → 80 lines)
4. ✅ **Update Makefile** (15 minutes)
5. ✅ **Validate changes** (30 minutes)

**Total Effort**: 2.5-3 hours
**Impact**: 79% reduction in worst offenders (2,274 → 480 lines)

---

### **Future Iterations**

If Option C proves successful:
1. **Iteration 2**: Consolidate AI assistant files (740 → 200 lines)
2. **Iteration 3**: Review specialized rules (trim 20-30%)
3. **Iteration 4**: Consider Option A (radical consolidation)

---

## ❓ **Decision Required**

**Which option should we proceed with?**

- **Option A**: Radical consolidation (single core file) - 4-6 hours
- **Option B**: Aggressive trimming (preserve structure) - 3-4 hours
- **Option C**: Proof of concept (3 worst files) - 2-3 hours ⭐ **RECOMMENDED**

**Recommendation**: Start with **Option C** to prove the approach, then expand if successful.

---

## 📞 **Questions Before Proceeding**

1. Do you approve Option C (proof of concept with 3 files)?
2. Should I create the missing documentation and scripts first?
3. Do you want to review each refactored file before I proceed to the next?
4. Should I archive the original files or replace them directly?

---

**Status**: ⏸️ **AWAITING USER DECISION**
