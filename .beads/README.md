# Gas Town Integration for Kubernaut Development

## Overview

This directory contains Gas Town formulas and templates for orchestrating kubernaut development workflows across service teams.

## Purpose

Gas Town provides:
- **Convoy Tracking**: Monitor progress across multiple service teams
- **Structured Communication**: Mail system for document handoffs
- **Workflow Automation**: APDC + TDD formulas with mandatory quality gates
- **Human-in-the-Loop**: Checkpoints for review and approval before critical phases
- **Quality Assurance**: 100% test pass requirement before day transitions

## Formula: integration-test-full-validation

**Version**: 2.1
**File**: `.beads/formulas/integration-test-full-validation.toml`

### Workflow Phases

1. **ANALYSIS** (15m)
   - Review authoritative documentation
   - Map business requirements
   - Define test scope
   - **CHECKPOINT**: Ask questions, share concerns

2. **PLAN** (20m)
   - Define test structure
   - Plan mock strategy
   - Map TDD sequence
   - Identify integration points
   - **CHECKPOINT**: Share plan, request approval

3. **DO - Infrastructure** (10m)
   - Set up HolmesAPI
   - Verify dependencies
   - Prepare test environment

4. **DO - TDD RED** (15m)
   - Write failing tests
   - Define expected behavior
   - Validate no `Skip()` calls

5. **DO - TDD GREEN** (20m)
   - Minimal implementation
   - Make tests pass
   - Avoid over-engineering

6. **DO - TDD REFACTOR** (15m)
   - Add edge cases
   - Enhance error handling
   - Add comprehensive logging

7. **CHECK - Test Validation** (10m) 🚨 MANDATORY GATE
   - Run all tests
   - Verify 100% pass rate
   - Detect anti-patterns
   - **CHECKPOINT**: Confirm 100% pass before proceeding

8. **CHECK - Compliance Triage** (30-45m)
   - Audit against authoritative V1.0 docs
   - Validate testing standards compliance
   - Check SOC2 requirements
   - Detect anti-patterns
   - Generate compliance report
   - **CHECKPOINT**: Review triage, approve or fix

9. **DOCUMENT** (10m)
   - Update test plan
   - Commit compliance report
   - Document resolutions

**Total Duration**: 2.5-3.5 hours
**Human Interaction Points**: 4 checkpoints
**Blocking Quality Gates**: 2 (test validation + compliance)

### Critical Quality Gate: 100% Test Pass Requirement

**Rule**: Tests must pass at 100% before moving to next day.

**Validation**:
- All tests pass (no failures)
- No skipped tests (`Skip()` forbidden)
- No anti-patterns detected
- Test validation checkpoint approved

**Blocking Behavior**:
- If test pass rate < 100% → block progression
- If anti-patterns detected → require fixes
- If checkpoint rejected → return to do-refactor

**Next Day Criteria**:
```toml
[progression_rules]
day_completion_criteria = [
    "All tests pass at 100%",
    "Test validation checkpoint approved",
    "Compliance triage completed",
    "Compliance checkpoint approved",
    "Documentation committed"
]
```

### Anti-Patterns Automatically Detected

From `TESTING_GUIDELINES.md`:

1. **time.Sleep() before assertions** (CRITICAL)
   - Use `Eventually()` instead
   - Validation: `grep -r 'time\.Sleep' test/`

2. **Skip() calls** (CRITICAL)
   - Forbidden in all tests
   - Validation: `grep -r '\.Skip(' test/`

3. **Direct audit infrastructure testing** (HIGH)
   - Test business logic, not audit store
   - Check for direct `AuditStore` method calls

4. **Direct metrics infrastructure testing** (HIGH)
   - Test business behavior, not metrics methods
   - Check for direct `registry.MustRegister()` calls

5. **podman-compose race conditions** (MEDIUM)
   - Use proper health checks
   - Implement startup synchronization

6. **Eventually() misuse** (MEDIUM)
   - Proper timeout/interval values
   - Correct assertion patterns

7. **Kubeconfig isolation violations** (HIGH)
   - Service-specific kubeconfig required
   - No shared kubeconfig in E2E tests

### Compliance Documentation Sources

**Authoritative V1.0 Documentation**:
```toml
authoritative_sources = [
    "docs/requirements/*.md",
    "docs/architecture/decisions/DD-*.md",
    "docs/development/SOC2/*.md",
    "docs/development/business-requirements/TESTING_GUIDELINES.md",
    ".cursor/rules/*.mdc",
    "test/integration/*/README.md"
]
```

**Validation Categories**:
1. Business requirement mapping
2. Testing standards compliance
3. Testing anti-patterns detection
4. API contract alignment
5. APDC methodology adherence
6. SOC2 requirements
7. Project rules compliance
8. Defense-in-depth coverage

### Using the Formula

#### Starting a New Molecule (Work Item)

```bash
# Example: Day 3 SOC2 Hybrid Capture Integration Tests
gastown molecule create \
  --formula integration-test-full-validation \
  --name "day3-soc2-hybrid-integration-tests" \
  --convoy "soc2-audit-trail" \
  --inputs "docs/development/SOC2/DAY2_HYBRID_AUDIT_COMPLETE.md"
```

#### Tracking Progress

```bash
# View convoy status
gastown convoy show soc2-audit-trail

# View molecule status
gastown molecule status day3-soc2-hybrid-integration-tests

# View checkpoint status
gastown checkpoint list day3-soc2-hybrid-integration-tests
```

#### Responding to Checkpoints

```bash
# Approve analysis checkpoint
gastown checkpoint approve day3-soc2-hybrid-integration-tests analysis-checkpoint

# Reject test-validation checkpoint (send back to do-refactor)
gastown checkpoint reject day3-soc2-hybrid-integration-tests test-validation-checkpoint \
  --reason "3 tests failing, anti-pattern detected in line 145"

# Approve compliance checkpoint
gastown checkpoint approve day3-soc2-hybrid-integration-tests compliance-checkpoint
```

### Compliance Report Template

**Location**: `.beads/templates/compliance-triage-report-v2.md`

**Sections**:
1. Executive Summary (status, pass rate, recommendation)
2. Test Validation Status (100% pass requirement)
3. Business Requirement Mapping
4. Testing Standards Compliance (TESTING_GUIDELINES.md)
5. API Contract Alignment
6. APDC Methodology Adherence
7. SOC2 Requirements
8. Project Rules Compliance
9. Critical Issues (blocking)
10. Warnings (non-blocking)
11. Recommendations
12. Gaps Identified
13. Sign-Off Recommendation

**Generated**: Automatically by compliance-triage step
**Reviewed**: At compliance-checkpoint

## Convoy: SOC2 Audit Trail

**Current Status**: Day 2 Complete, Day 3 Planning
**Teams Involved**:
- AI Analysis (Go service)
- HolmesAPI (Python service)
- Data Storage (PostgreSQL)
- Gateway (API gateway)

**Day 3 Scope** (from `DAY2_HYBRID_AUDIT_COMPLETE.md`):
- Hybrid capture integration tests
- RR reconstruction validation
- Consumer context preservation tests
- End-to-end audit trail verification

## Benefits for Kubernaut Development

### 1. Automated Coordination
- **Before**: Manual status updates, document handoffs via Slack/email
- **After**: Structured mail system, automatic progress tracking

### 2. Quality Assurance
- **Before**: Ad-hoc testing, inconsistent standards
- **After**: 100% test pass requirement, automated anti-pattern detection

### 3. Knowledge Preservation
- **Before**: Tribal knowledge, inconsistent documentation
- **After**: Compliance reports, documented decisions, gap tracking

### 4. Cross-Team Visibility
- **Before**: Limited visibility into other service team progress
- **After**: Convoy tracking, shared molecule states, coordination points

### 5. Human Oversight
- **Before**: AI implementations without review
- **After**: 4 mandatory checkpoints, explicit approval required

## Adoption Strategy

### Parallel Adoption (Non-Disruptive)

**Phase 1**: New Work Only
- Use Gas Town for Day 3+ SOC2 work
- Keep existing Day 2 work outside Gas Town
- Learn the system with low-risk work items

**Phase 2**: Expand to Other Convoys
- Create convoys for other epics (e.g., dynamic toolset, remediation)
- Apply learnings from SOC2 convoy
- Build team confidence

**Phase 3**: Full Integration
- All new work starts as Gas Town molecules
- Existing work migrates opportunistically
- Gas Town becomes primary orchestration tool

### Team Training

**For Service Teams**:
1. Understanding convoy tracking
2. Using the mail system
3. Responding to checkpoints
4. Reading compliance reports

**For AI Polecats**:
1. Following formulas
2. Generating compliance reports
3. Detecting anti-patterns
4. Human-in-the-loop communication

## Directory Structure

```
.beads/
├── README.md                                    # This file
├── formulas/
│   └── integration-test-full-validation.toml   # APDC + TDD formula
├── templates/
│   └── compliance-triage-report-v2.md          # Compliance report template
└── convoys/
    └── soc2-audit-trail/                        # (Created by Gas Town runtime)
        ├── convoy.toml                          # Convoy configuration
        └── molecules/                           # Active work items
            └── day3-soc2-hybrid-integration-tests/
                ├── molecule.toml                # Molecule state
                ├── checkpoints/                 # Checkpoint history
                └── reports/                     # Compliance reports
```

## Next Steps

1. **Install Gas Town**: Follow https://github.com/steveyegge/gastown
2. **Create SOC2 Convoy**: Define the overall SOC2 audit trail epic
3. **Launch Day 3 Molecule**: Start with hybrid integration tests
4. **Monitor First Checkpoints**: Learn the approval workflow
5. **Review Compliance Report**: Validate quality gate effectiveness
6. **Iterate Formula**: Adjust based on real-world usage

## Support

- **Formula Issues**: Create `.beads/FORMULA_ISSUES.md`
- **Checkpoint Patterns**: Document in `.beads/CHECKPOINT_GUIDELINES.md`
- **Team Feedback**: Track in `.beads/ADOPTION_FEEDBACK.md`

## Version History

- **v2.1** (2026-01-05): Added 100% test pass requirement gate
- **v2.0** (2026-01-05): Added compliance triage + TESTING_GUIDELINES.md validation
- **v1.1** (2026-01-05): Added human checkpoints (analysis, plan, validate)
- **v1.0** (2026-01-05): Initial formula (APDC + TDD)

---

**Maintained By**: kubernaut-dev
**Last Updated**: 2026-01-05
**Gas Town Version**: Compatible with latest

