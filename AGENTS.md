# Kubernaut Development Methodology

This file is the **single authoritative source** for Kubernaut's development methodology. Every contributor -- human or AI agent, regardless of IDE or tooling -- must follow these rules.

For Cursor-specific implementation patterns (code examples, contextual snippets), see [`.cursor/rules/`](.cursor/rules/). Those files supplement this document but never override it.

---

## Getting Started for Contributors

If you are new to Kubernaut, here is the minimum path to your first contribution:

1. **Read this file** -- it defines what is mandatory and what will block your PR
2. **Identify the business requirement** your change serves (see [Business Requirements Mandate](#business-requirements-mandate))
3. **Write a test plan** if your change is non-trivial (see [Testing Requirements](#testing-requirements))
4. **Follow TDD**: write a failing test, make it pass with minimal code, then refactor
5. **Run the checks** before submitting:
   ```bash
   go build ./...
   golangci-lint run --timeout=5m
   make test
   ```
6. **Map audit events** to SOC2/FedRAMP controls if your change emits audit traces (see [SOC2 and FedRAMP Compliance](#soc2-and-fedramp-compliance))

For complex changes (multi-component, architectural), follow the full [Pre-Implementation Workflow](#pre-implementation-workflow).

---

## Table of Contents

1. [Getting Started for Contributors](#getting-started-for-contributors)
2. [Pre-Implementation Workflow](#pre-implementation-workflow)
3. [TDD Workflow](#tdd-workflow)
4. [Wiring Verification](#wiring-verification)
5. [SOC2 and FedRAMP Compliance](#soc2-and-fedramp-compliance)
6. [Go Anti-Pattern Checklist](#go-anti-pattern-checklist)
7. [Business Requirements Mandate](#business-requirements-mandate)
8. [Testing Requirements](#testing-requirements)
9. [AI Agent Checkpoints](#ai-agent-checkpoints)
10. [Code Quality Standards](#code-quality-standards)
11. [GA Readiness Audit](#ga-readiness-audit)
12. [Completion Requirements](#completion-requirements)
13. [TDD Anti-Patterns](#tdd-anti-patterns)
14. [Collaboration Rules](#collaboration-rules)

---

## Pre-Implementation Workflow

Before writing any implementation code, every non-trivial task must pass through this workflow in order.

### Step 1: Preflight Checks

Analyze the existing codebase to understand blast radius:

- Search for existing implementations of the target component
- Map dependencies and callers
- Identify integration points in `cmd/` and handler code
- Assess architecture impact (minimal / moderate / significant)
- Verify no conflicting work in progress
- Map business requirement: identify BR-[CATEGORY]-[NUMBER]
- Assess rule compliance: Go standards, testing strategy, SOC2/FedRAMP

**Gate**: Preflight confidence must reach **95%** before proceeding. If below 95%, identify what is unknown and proceed to Step 2.

### Step 2: Spikes (If Needed)

Time-boxed investigation (max 2 hours) to resolve unknowns identified in preflight:

- Prototype the uncertain approach in an isolated spike
- Validate assumptions against real code or infrastructure
- Document findings with evidence

**Gate**: Each spike must produce a clear YES/NO decision on the approach.

### Step 3: Confidence Score

After preflight + spikes, declare overall confidence:

- **95-100%**: Proceed directly to planning
- **90-94%**: Proceed with caution, flag remaining risks
- **Below 90%**: STOP. Escalate unknowns to the user before proceeding

### Step 4: Plan

Create an implementation plan with:

1. **TDD phase mapping**: RED, GREEN, REFACTOR sequence with estimated durations
2. **Wiring Manifest**: Required for all new components (see [Wiring Verification](#wiring-verification))
3. **Success criteria**: Measurable outcomes
4. **Risk mitigation**: Contingency and rollback plans

**Gate**: User approval required before proceeding to implementation.

### Step 5: Readiness Audit (Pre-Implementation)

Before starting TDD, verify readiness against the [GA Readiness Audit](#ga-readiness-audit) dimensions to ensure the plan addresses all quality gates.

### Step 6: TDD Implementation

Execute TDD in sub-phases:

1. **DISCOVERY**: Search existing implementations (CHECKPOINT B)
2. **RED**: Write failing tests (UT + IT for wiring points)
3. **GREEN**: Minimal implementation + mandatory integration + CHECKPOINT W
4. **REFACTOR**: Enhance with sophisticated logic + Go anti-pattern validation

See [TDD Workflow](#tdd-workflow) for full details on each phase.

### Step 7: Verification

After implementation:

1. **Business verification**: BR fulfilled
2. **Technical validation**: Build, lint, tests pass
3. **Integration confirmation**: `cmd/` integration verified
4. **Compliance verification**: SOC2/FedRAMP audit trace mapping confirmed
5. **Confidence assessment**: Percentage + justification

### When to Use the Full Workflow

- Complex feature development (multiple components)
- Significant refactoring (architectural changes)
- New component creation (business logic)
- Cross-component integration work
- Performance optimization (system-wide impact)
- Build error fixing (systematic remediation)

### When to Use Standard TDD Only (Skip Steps 1-5)

- Simple bug fixes (single file)
- Documentation updates
- Configuration changes
- Test-only modifications

---

## TDD Workflow

Every code change follows RED, GREEN, REFACTOR. No exceptions.

### RED Phase

Write failing tests that define the business contract:

- Unit tests for logic (pure functions, validators, builders, scoring)
- Integration tests for wiring (HTTP handlers, K8s reconcilers, DB adapters)
- Tests MUST use Ginkgo/Gomega BDD framework
- Tests MUST reference business requirements: `BR-[CATEGORY]-[NUMBER]`
- Tests SHOULD use Test Scenario IDs when a test plan exists: `UT-XX-NNN-NNN`

### GREEN Phase

Minimal implementation to make tests pass:

- Wire the component into production code (`cmd/`, handlers, callbacks)
- Execute CHECKPOINT W (see [Wiring Verification](#wiring-verification))
- No sophisticated logic in GREEN -- keep it minimal
- GREEN is NOT complete until both UT and IT pass

### REFACTOR Phase

Enhance the implementation with production-quality logic, keeping the RED-phase tests green
throughout as a safety net:

- Apply the [Go Anti-Pattern Checklist](#go-anti-pattern-checklist) -- and fix what it finds,
  not just run it
- Optimize algorithms and data structures
- Improve error messages and observability
- Eliminate duplication or awkward structure left behind by GREEN's minimal implementation
- NEVER create new types or components in REFACTOR -- enhance existing only

**REFACTOR is content, not validation.** Running `go build ./...` and confirming the test
suite still passes (see [Post-Refactor Validation](#post-refactor-validation-mandatory)) is a
*mandatory precondition* for calling REFACTOR complete -- it proves the improvements above were
behavior-preserving. It is not itself a REFACTOR bullet, and "confirm build/lint/tests pass" is
never a sufficient description of what REFACTOR did.

**REFACTOR is legitimately `N/A`** when GREEN left no code behind to improve -- e.g. a pure
deletion, or a milestone that only adds plain data-type fields with no logic. Mark it `N/A` with
a one-line reason rather than filling the slot with the validation step above.

### The Pyramid Invariant

> UT proves logic. IT proves wiring. E2E proves the journey.
> A component with only UT coverage is prototyped, not implemented.
> GREEN is not complete until the IT test for every wiring point passes.

---

## Wiring Verification

Prevents the "built but not wired" failure where components exist in `pkg/` with passing unit tests but are never connected to production code.

### Wiring Manifest (Plan Phase -- Mandatory)

Every plan introducing new components MUST include this table:

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|-----------|
| *function/handler/pool* | *where constructed in production* | *exact file and function* | *IT test proving wiring* |

### CHECKPOINT W (GREEN Phase -- Mandatory)

After GREEN phase, verify for each Wiring Manifest row:

- [ ] Component constructed/called in `cmd/` or handler code (not just tests)
- [ ] IT test exercises production dispatch path (test ID + PASS)
- [ ] No component in `pkg/` without a corresponding production caller
- [ ] No "TODO: wire later" deferred wiring

**Violation**: `WIRING CHECKPOINT FAILED: [Component] has no production caller`

### Wiring-First TDD Sequence

```
RED:   Write IT test calling component through production entry point -> fails
       Write UT test for component logic -> fails
GREEN: Wire component in production code -> IT passes
       Implement component logic -> UT passes
REFACTOR: Clean up (name what's being cleaned up; N/A if GREEN left nothing to clean)
```

### Detection Commands

**Preference hierarchy**: gopls MCP > gopls CLI > grep

#### gopls (preferred -- type-safe, import-aware)

gopls provides precise reference lookups using the Go type system. Results are identical
regardless of interface; MCP is preferred for AI agents because it avoids shell parsing.

**Setup**:
```bash
# Install gopls (prerequisite: Go toolchain)
go install golang.org/x/tools/gopls@latest

# Start as MCP server (for AI agents that support MCP)
gopls mcp
# or with remote caching:
gopls -remote=auto mcp
```

For Cursor IDE, the gopls MCP server is pre-configured (`user-gopls`). For other
MCP-compatible agents (Claude Code, Windsurf, etc.), add `gopls mcp` to your MCP
server configuration. For human developers, your IDE's "Find All References" uses
gopls under the hood.

**MCP usage** (AI agents):
```
go_symbol_references(file="/path/to/file.go", symbol="NewComponent")
go_symbol_references(file="/path/to/file.go", symbol="pkg.HandleX")
go_search(query="NewComponent")
```

**CLI usage** (universal):
```bash
# Find all references to a symbol at a specific position
gopls references /path/to/file.go:42:6

# Search symbols by name
gopls symbols -query="NewComponent"
```

Verify that each new exported function/type has at least one caller in production code
(`cmd/` or `handler/` paths, excluding `_test.go`).

#### grep (fallback -- when gopls is unavailable)

```bash
# Verify new components have production callers
grep -r "NewComponent\|HandleX\|RegisterY" cmd/ pkg/*/handler/ --include="*.go" | grep -v "_test.go"

# Find orphaned pkg/ code
for f in $(git diff --name-only --diff-filter=A -- 'pkg/**/*.go' | grep -v _test.go); do
  base=$(basename "$f" .go)
  if ! grep -rq "$base" cmd/ --include="*.go"; then
    echo "WARNING: $f may be orphaned (no cmd/ reference)"
  fi
done
```

---

## SOC2 and FedRAMP Compliance

Kubernaut captures audit traces as part of its business requirements. All audit-emitting services must maintain alignment with SOC2 and FedRAMP controls.

### SOC2 Trust Service Criteria (Actively Enforced)

| Control | Requirement | Kubernaut Application |
|---------|------------|----------------------|
| **CC8.1** | Audit completeness | Complete remediation request reconstruction from audit traces |
| **CC6.1** | Financial governance | Forensic/postmortem data completeness |
| **CC7.2** | Internal controls | Decision audit trails, workflow selection auditing |

### FedRAMP Controls (Actively Enforced)

| Control | Requirement | Kubernaut Application |
|---------|------------|----------------------|
| **AU-2** | Audit events | Auth audit events, session lifecycle |
| **AU-3** | Content of audit records | Structured event payloads with actor attribution |
| **AU-9** | Protection of audit information | Immutable storage, hash chains, digital signatures, legal hold |
| **AU-11** | Audit record retention | Category-based retention floors (7 years default) |
| **AC-4** | Information flow enforcement | Cross-service data flow controls |
| **AC-6** | Least privilege | SAR granularity, RBAC verb mapping |
| **SC-8** | Transmission confidentiality | TLS enforcement for all service communication |
| **SI-10** | Information input validation | OpenAPI schema validation, input sanitization |

### Mandatory Rules

1. **Every audit-emitting service** must map each event type to its SOC2/FedRAMP control(s)
2. **Every audit event** must carry: `event_id` (UUID), `EventAction`, `EventOutcome`, `ActorType`, `ActorID`
3. **Audit trace tests** must prove complete remediation request reconstruction via `correlation_id` queries
4. **New services** must declare audit requirements before implementation (reference DD-AUDIT-003)
5. **Error events** must include standardized `error_details` per DD-ERROR-001

### Authority Documents

- [DD-AUDIT-003](docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md): Per-service audit trace requirements (defines which of the 13 services MUST emit audit events and their event catalogs)
- [ADR-034](docs/architecture/decisions/ADR-034-unified-audit-table-design.md): Unified audit table design (event-sourcing pattern, hash chain integrity, retention policy)

### BR-AUDIT-005 v2.0: Core Audit Business Requirement

This is the root business requirement for audit compliance. Its mandates:

- All business-critical operations MUST produce audit events persisted to the unified audit table
- Audit events MUST be queryable by `remediation_id` (correlation) to enable full remediation request reconstruction
- Each event MUST conform to ADR-034 schema: `event_id`, `event_type`, `event_action`, `event_outcome`, `actor_type`, `actor_id`, `correlation_id`, `event_data` (typed JSON)
- SOC2 CC8.1 reconstruction test: given a `correlation_id`, the complete lifecycle (signal -> analysis -> workflow selection -> execution -> verification -> notification) MUST be reconstructable from audit traces alone
- Retention: minimum 7 years for compliance-category events (AU-11), configurable per event category

---

## Go Anti-Pattern Checklist

Validated during the REFACTOR phase. Based on [100 Go Mistakes and How to Avoid Them](https://100go.co/).

### Mandatory Checks

| Anti-Pattern | Detection | Resolution |
|-------------|-----------|-----------|
| Function/method with 8+ parameters | Count params in signature | Use Options pattern or config struct |
| Variable shadowing | `go vet -shadow` or linter | Rename inner variable, use explicit assignment |
| Unnecessary nesting (> 3 levels) | Visual inspection | Early returns, guard clauses, extract functions |
| Interface pollution (5+ methods) | Count interface methods | Split into focused role interfaces |
| Inefficient slice pre-allocation | `make([]T, 0)` without capacity when size is known | `make([]T, 0, expectedSize)` |
| Inefficient map pre-allocation | `make(map[K]V)` without size hint when count is known | `make(map[K]V, expectedSize)` |
| Naked returns in functions > 5 lines | Lint check | Use explicit return values |
| Error strings with uppercase or punctuation | `grep -r "fmt.Errorf.*[A-Z]"` | Lowercase, no trailing punctuation, no newlines |
| Context stored in struct fields | Grep for `ctx context.Context` as struct field | Pass context as first function parameter |
| Goroutine leaks | Missing context cancellation or done channel | Ensure every goroutine has an exit path |
| Deep package nesting (> 4 levels) | Directory depth check | Flatten package hierarchy |
| `any`/`interface{}` usage | Grep for `any\|interface{}` | Use specific types or generics |
| Ignoring errors | Grep for unchecked error returns | Handle every error, log with context |
| God structs (15+ fields) | Count struct fields | Decompose into focused sub-structs |

### Validation Command

```bash
# Quick anti-pattern scan on changed files
for f in $(git diff --name-only -- '*.go'); do
  echo "=== $f ==="
  # Check parameter count
  grep -n "^func " "$f" | while read line; do
    params=$(echo "$line" | grep -o ',' | wc -l)
    if [ "$params" -ge 7 ]; then
      echo "  WARNING: 8+ params at $line"
    fi
  done
done
```

---

## Business Requirements Mandate

Every code change MUST be backed by at least one business requirement.

### Format

`BR-[CATEGORY]-[NUMBER]` (e.g., BR-WORKFLOW-001, BR-AI-056)

### Categories

WORKFLOW, AI, INTEGRATION, SECURITY, PLATFORM, API, STORAGE, MONITORING, SAFETY, PERFORMANCE, AUDIT, GATEWAY, INTERACTIVE, EFFECTIVENESS, SCOPE, SEVERITY, COMMON

### Rules

- All tests must map to specific business requirements
- All implementation code must serve documented business needs
- No speculative or "nice to have" code without business backing
- Business requirements live in `docs/requirements/`

---

## Testing Requirements

### Framework (Mandatory)

- **Ginkgo/Gomega BDD** framework -- NO standard Go `testing.T` for business logic tests
- **Test identification**: Test Scenario IDs (`UT-WF-197-001`, `IT-GW-045-010`) or BR references
- **Test plans**: Create formal test plan BEFORE implementation using the [IEEE 829-2008 hybrid template](docs/testing/TEST_PLAN_TEMPLATE.md)

### Exception: Go Native Fuzz Tests

Native Go fuzzing (`func FuzzXxx(f *testing.F)`) is the **sole, narrowly-scoped exception**
to the Ginkgo/Gomega mandate above. This is a language/toolchain constraint, not a style
choice:

- Go's built-in fuzzing engine (`go test -fuzz=`) only recognizes the exact stdlib signature
  `func FuzzXxx(f *testing.F)`. No third-party framework, including Ginkgo, can hook into it.
- OpenSSF Scorecard's `Fuzzing` check performs exact regex matching for this same signature
  in `*_test.go` files -- there is no Ginkgo-detectable equivalent for Go.
- Fuzz tests are not business-logic behavior specs (no Given/When/Then, no business-outcome
  assertion). Their only contract is "the function under test must not panic on adversarial
  byte/string input" -- a distinct testing tier (adversarial-input robustness) from the BDD
  unit/integration/E2E pyramid.

Rules for this exception:

- Fuzz functions live in dedicated `*_fuzz_test.go` files, never mixed into a Ginkgo spec file.
- Target only functions that parse/deserialize untrusted external input (webhook payloads,
  auth tokens, request bodies) -- not general business logic, which still requires Ginkgo
  coverage for its actual behavior separately.
- The fuzz body must not assert business outcomes; it should only call the target function
  and let a panic (if any) fail the test, e.g. `_, _ = target.Parse(ctx, rawData)`.
- Seed corpus entries (`f.Add(...)`) should include at least one valid, one malformed, and
  one edge-case (empty/null) payload.

### Coverage Targets

| Tier | Target | Metric | Rationale |
|------|--------|--------|-----------|
| Unit | 100% of business logic | Line coverage of unit-testable code | Every BR has a UT verifying behavior tied to FedRAMP/SOC2 control objectives |
| Integration | 100% of wiring points + 100% of FedRAMP controls assessed | Wiring Manifest completeness + control objective coverage | IT proves components are wired and controls are implemented -- not line % |
| E2E | 100% of SOC2/FedRAMP control objectives have at least one proving journey | Control objective coverage | Validates end-to-end compliance flows in production-like conditions |
| All Tiers (merged) | >= 80% | Line-by-line dedup across all tiers | Any tier covering a line counts; this is the CI gate |

Unit tests are the foundation -- they verify business-level behavior, not implementation details.
Integration and E2E tests serve wiring proof and control assessment respectively;
their value is measured by **what they prove** (wiring completeness, control objective coverage),
not by line coverage percentage. Individual IT/E2E line coverage is reported but not gated.

### How Coverage Is Measured

This methodology distinguishes **structural coverage** (line/branch) from **requirements-based coverage**
(ISO/IEC/IEEE 29119-4:2021, Section 5.2.10 and 6.2.12). Unit tests use structural coverage. Integration
and E2E tests use requirements-based coverage derived from compliance mandates.

**Unit Test Coverage (structural -- line coverage)**:
- Measured automatically by `go tool cover` and reported by `scripts/coverage/coverage_report.py`
- CI gate: `--check-gate` on the `unit_testable` and `all_tiers` columns
- Target: 100% of business logic (unit-testable code as categorized in `.coverage-patterns.yaml`)
- Rationale: Google's engineering practices recommend per-commit coverage gates with high thresholds
  for business-critical code (ref: "Software Engineering at Google", Ch. 11, Arguelles 2024)

**Integration Test Coverage (requirements-based -- wiring completeness)**:
- Measured by the **Wiring Manifest** created during the Plan phase of each feature
- Each row = one wiring point: component + production entry point + IT test ID
- Coverage = rows with passing IT / total rows in manifest (target: 100%)
- Enforcement: CHECKPOINT W during TDD GREEN phase (verified per-PR in review)
- Additionally: every FedRAMP control mapped in the BR must have at least one IT proving
  the implementation is wired (ref: NIST SP 800-53A Rev. 5 -- all applicable controls must
  be assessed during the authorization period; FedRAMP Consolidated Rules 2026 --
  "Providers MUST have all applicable Rev5 Controls included in independent assessments")

**E2E Coverage (requirements-based -- control objective assessment)**:
- Measured by mapping SOC2/FedRAMP control objectives to E2E test scenarios
- The BR Coverage Matrix (`docs/services/*/BR_COVERAGE_MATRIX.md`) tracks which controls have proving journeys
- Coverage = control objectives with at least one passing E2E / total control objectives in scope (target: 100%)
- Enforcement: pre-merge review validates control coverage for compliance-mapped BRs
- Rationale: SOC2 Type 2 audits require sampling from a **complete population** of all control activities
  (AICPA TSC CC8.1); FedRAMP requires 100% of applicable controls assessed (NIST SP 800-53A Rev. 5, Ch. 4)

**All Tiers Merged (structural -- CI gate)**:
- Measured by `scripts/coverage/coverage_report.py` using line-by-line deduplication across all tiers
- A line covered by ANY tier counts once (logical OR merge)
- CI gate: >= 80% (currently non-blocking during V1.5 rollout, becomes blocking after Phase 9)
- Rationale: 80% merged all-tiers aligns with industry benchmarks (Google guidelines: 60% acceptable,
  75% commendable, 90% exemplary; 80% is a pragmatic floor validated by actual PR data)

### Authoritative References

| Standard | Relevance |
|----------|-----------|
| ISO/IEC/IEEE 29119-4:2021 | Defines requirements-based test coverage (Section 6.2.12) vs structural coverage (Section 6.3) |
| NIST SP 800-53A Rev. 5 | "All controls are assessed during the authorization period" (Ch. 4) |
| FedRAMP Consolidated Rules 2026 | "Providers MUST have all applicable Rev5 Controls included in independent assessments" |
| AICPA TSC (SOC2) CC8.1 | All changes authorized, tested, documented; auditor samples from complete population |
| Software Engineering at Google, Ch. 11 | Coverage as floor not ceiling; per-commit gates; behavior-focused testing |

### Mock Strategy

**Mock ONLY external dependencies:**
- External APIs (LLM, HolmesGPT, OpenAI)
- Databases (PostgreSQL, Vector DB, Redis)
- Kubernetes API (use `fake.NewClientBuilder()`)
- Network services (external HTTP/gRPC)

**Use real business logic:**
- ALL `pkg/` code
- ALL internal algorithms
- ALL business validators/analyzers/optimizers

### CI Parallel Safety

- Use `httptest.NewServer` (`:0` port) -- no port conflicts
- Each test creates its own Store, Manager, Handler -- no shared state
- Process-level E2E uses subprocess with random port flag

---

## AI Agent Checkpoints

Mandatory validation gates for AI coding agents. Human contributors should follow these as mental checklists.

### CHECKPOINT A: Type Reference Validation

**Trigger**: About to reference any struct field.

**Action**: Read the type definition file BEFORE referencing fields. Verify the field exists in the struct definition.

**Violation**: Type reference without validation -- STOP.

### CHECKPOINT B: Implementation Discovery

**Trigger**: About to create a test file or new component.

**Action**: Search for existing implementations first. Enhance existing patterns instead of creating new ones.

**Violation**: Creation without searching existing code -- STOP.

### CHECKPOINT C: Business Integration Validation

**Trigger**: Creating new business types or interfaces.

**Action**: Verify main application integration. Business code MUST be integrated in `cmd/`.
Use `go_symbol_references` (gopls) to confirm the new type/function has at least one production caller
outside of test files.

**Violation**: Business component without `cmd/` integration -- STOP.

### CHECKPOINT D: Build Error Investigation

**Trigger**: Build errors or undefined symbols reported.

**Action**: Execute comprehensive symbol analysis. Present options with evidence before implementing.

**Required format**:
```
UNDEFINED SYMBOL ANALYSIS:
Symbol: [undefined_symbol]
References found: [N files with paths]
Dependent infrastructure: [list missing types/functions]
Scope: [minimal/medium/extensive with evidence]

OPTIONS (Evidence-Based):
A) Implement complete infrastructure ([X] files affected)
B) Create minimal stub ([Z] files affected, may break [W] files)
C) Alternative approach: [evidence-based alternative]

MANDATORY USER DECISION REQUIRED: Which approach? (A/B/C)
```

### CHECKPOINT DD: Design Decision Validation

**Trigger**: Proposing or implementing a significant architectural change (new CRD interaction patterns, technology choices, data flow changes, integration point designs).

**Action**: Before implementing, execute this sequence:

1. Search for similar architectural patterns in the codebase
2. Identify 2-3 alternative approaches with pros/cons
3. Present alternatives to the user for approval
4. After approval, create DD-XXX entry in `docs/architecture/DESIGN_DECISIONS.md`
5. Reference DD-XXX in implementation code comments

**Violation**: Architectural change without documented alternatives and user approval -- STOP.

**Skip if**: Simple bug fix, adding a function to an existing pattern, config change, test-only change.

---

## Code Quality Standards

### Error Handling (Mandatory)

- ALWAYS handle errors -- never ignore them
- ALWAYS add log entry for every error
- Wrap errors with context: `fmt.Errorf("operation description: %w", err)`
- Use structured error types from `internal/errors/`
- Error strings: lowercase, no trailing punctuation, no newlines

### Type System

- AVOID `any` or `interface{}` unless absolutely necessary
- Use structured field values with specific types
- AVOID local type definitions to resolve import cycles
- Use shared types from `pkg/shared/types/` instead

### Business Integration

- MANDATORY: Integrate all new business code with main code (`cmd/`)
- Remove any code not backed by business requirements
- Ensure seamless integration with existing architecture

---

## GA Readiness Audit

A 13-dimension quality gate applied before declaring a service or feature production-ready.

### Dimensions

| # | Dimension | Pass Criteria |
|---|-----------|--------------|
| 1 | **Build** | `go build ./...` succeeds with zero errors |
| 2 | **Lint** | `golangci-lint run` produces zero new warnings |
| 3 | **Unit Tests** | 100% pass rate, 100% coverage on business logic |
| 4 | **Integration Tests** | 100% pass rate, all wiring points + FedRAMP controls assessed |
| 5 | **Wiring Verification** | CHECKPOINT W passes for all components |
| 6 | **BDD Framework** | Zero standard `testing.T` usage in business tests (native Go fuzz tests are the sole exception -- see [Exception: Go Native Fuzz Tests](#exception-go-native-fuzz-tests)) |
| 7 | **Test ID Assignment** | All tests have scenario IDs or BR references |
| 8 | **SOC2/FedRAMP Compliance** | All audit events mapped to controls, reconstruction proven |
| 9 | **100 Go Mistakes** | Zero violations from [anti-pattern checklist](#go-anti-pattern-checklist) |
| 10 | **Business Requirement Satisfaction** | All BRs in scope have passing tests |
| 11 | **Regression** | Zero regressions in existing test suites |
| 12 | **Fail-Open Safety** | No silent failures -- all error paths are observable |
| 13 | **Domain-Specific** | Service-specific checks (K8s safety, rate limiting, auth, etc.) |

### When to Apply

- Before merging a feature branch with > 100 lines changed
- Before declaring a service GA-ready
- As the final gate after implementation (Step 7: Verification)

---

## Completion Requirements

### Post-Development Checklist (Mandatory)

After completing any development task:

1. **Build validation**: Code builds without errors
2. **Lint compliance**: No new lint errors
3. **Test pass**: All affected tests pass
4. **Business integration**: New code integrated in `cmd/` where applicable
5. **Compliance**: Audit events mapped to SOC2/FedRAMP controls where applicable
6. **Anti-patterns**: Refactored code passes Go anti-pattern checklist

### Confidence Assessment Format (Required)

Provide BOTH:
- **Percentage**: 60-100% confidence rating
- **Justification**: Risks, assumptions, validation approach

Example:
```
Confidence: 85%
Justification: Implementation follows established patterns in pkg/workflow/engine/
and integrates with existing HolmesGPT client. Risk: Minor performance impact on
high-alert scenarios. Validation: Unit tests cover 90% of edge cases.
```

---

## TDD Anti-Patterns

Forbidden patterns with detection rules.

| Anti-Pattern | Description | Rule |
|-------------|-------------|------|
| **Discovery Skip** | Creating without searching existing | Use CHECKPOINT B FIRST |
| **RED Skip** | Implementation without failing tests | Write tests FIRST |
| **GREEN Complexity** | Sophisticated logic in GREEN phase | Minimal implementation only |
| **REFACTOR Creation** | New types/components in REFACTOR | Enhance existing only |
| **Integration Delay** | Component not integrated in GREEN | Wire in GREEN, not later |
| **UT-Only GREEN** | Declaring GREEN when only UT passes | IT must also pass |
| **Pending Tests** | Using `XIt` or `Skip()` | Implement or remove |
| **Refactor Without Build** | Refactoring without checking build | Run `go build ./...` after ANY refactor |
| **REFACTOR-as-Validation** | REFACTOR step names only "confirm build/lint/tests pass," with no concrete code-quality change | Name the actual improvement (dedup, error handling, structure, naming); build/test is the safety net proving it's safe, not the improvement itself |

### Post-Refactor Validation (Mandatory)

```bash
go build ./...
go test ./... -run=^$ -timeout=30s
grep -r "OldFieldName\|OldTypeName" . --include="*.go"
```

---

## Collaboration Rules

### Rule 1: Pause, Assess, Communicate

Before executing any significant action (tests, commits, refactors, new implementations, destructive operations), pause and assess.

**ALWAYS share if you have:**
- Questions: Ambiguities or missing information
- Concerns: Potential issues, risks, or anti-patterns
- Alternatives: Different approaches worth considering
- Confidence gaps: Uncertainty below 90%
- Recommendations: Improvements with rationale

**SKIP if:**
- Everything is clear, straightforward, and routine
- High confidence (> 95%) with no concerns
- User explicitly said "proceed without asking"

**Significant actions requiring assessment:**
- Running integration/E2E tests (especially > 5 min)
- Committing code changes
- Refactoring existing code
- Creating new files or modules
- Destructive operations (delete, force push)
- Architectural changes

### Rule 2: No Pending Tests

Never use `XIt`, `PIt`, or `Skip()` to defer test implementation. Either:
- Implement the test following TDD
- Remove it from the test plan

### Rule 3: Critical Decision Escalation

MANDATORY: Ask for input on ALL critical decisions:
- Architecture changes and design patterns
- New dependencies or external integrations
- Performance trade-offs
- Security implementations
- Refactoring that affects system complexity

Provide a recommendation with detailed justification when asking.

---

## Quick Reference

```bash
# Build and lint
go build ./...
golangci-lint run --timeout=5m

# Test pyramid
make test                          # Unit tests
make test-integration-[service]    # Integration tests
make test-e2e-[service]           # E2E tests
make test-helm                    # Helm chart unit tests (helm-unittest, charts/kubernaut/tests/)

# Validation
make lint-test-patterns           # Test anti-patterns
make lint-business-integration    # Business code integration
make lint-tdd-compliance          # TDD and BDD framework
```

---

## Authority and References

- [Test Plan Template](docs/testing/TEST_PLAN_TEMPLATE.md) -- IEEE 829-2008 hybrid
- [DD-AUDIT-003](docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) -- per-service audit requirements
- [ADR-034](docs/architecture/decisions/ADR-034-unified-audit-table-design.md) -- unified audit table
- [100 Go Mistakes](https://100go.co/) -- anti-pattern reference
