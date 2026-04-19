# Test Plan: Forward Hash Chain Detection for Ineffective Remediation Escalation

**Feature**: Detect repeated remediation cycles where the same action type changes the spec each time but the signal recurs, and escalate to manual review
**Version**: 2.0
**Created**: 2026-03-04
**Updated**: 2026-03-04
**Author**: AI Assistant
**Status**: Implemented
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- BR-ORCH-042.7: Forward hash chain detection (Issue #525)
- BR-ORCH-042.5: Ineffective remediation chain escalation
- Issue #214: CheckIneffectiveRemediationChain implementation
- Issue #525: Escalation not triggered after repeated OOMKill cycles (regression)
- Issue #224: Completed-but-recurring detection (HAPI prompt)
- Issue #528: WorkflowType -> ActionType rename (tech-debt, v1.2)

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **RO routing engine** (`pkg/remediationorchestrator/routing/blocking.go`): Updated `countForwardChain` function with five-condition detection: same action type (ActionType), failed EA (SignalResolved==false), 1h time window, hash link continuity, threshold=2
- **HAPI prompt builder** (`kubernaut-agent/src/extensions/remediation_history_prompt.py`): Strengthen the existing `_detect_completed_but_recurring` warning from advisory ("Recommend selecting") to mandatory ("You MUST NOT re-select") when effectivenessScore is zero

### Out of Scope

- `CheckConsecutiveFailures` behavior (unchanged -- only counts Failed/Blocked RRs by design)
- DataStorage remediation history API (data is already correct; only consumer logic changes)
- Integration test for full RO controller + DS (existing IT-RO-214 tests cover the wiring)
- E2E test (requires multi-cycle OOMKill scenario -- tracked separately in demo-scenarios repo)
- **Issue #528** (ActionType naming): Platform/OpenAPI use `ActionType`; this plan documents scenarios with that name only (no separate rename work under #525).

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Threshold = 2 (not 3) | 2 DS entries + incoming RR = 3rd attempt blocked. Faster detection than Layer 1+2's threshold of 3. Separate `ForwardChainThreshold` config field. |
| Same ActionType matching | Different action types mean different remediation approaches were tried. Only block when the *same* approach keeps failing. Uses `ActionType` field from `RemediationHistoryEntry` (DD-WORKFLOW-016 taxonomy). |
| SignalResolved == false required | If a previous entry's EA succeeded (signal resolved), the signal recurring is a *new* problem, not a continuation. The pipeline guarantees EA completes before new RRs arrive -- no null handling needed. |
| 1h ForwardChainWindow (not 4h) | Tighter window than IneffectiveTimeWindow. Forward chain pattern is acute (rapid repeated failures). Separate `ForwardChainWindow` config field. |
| `countForwardChain` sorts entries ascending internally | DS returns entries descending (most recent first). Sorting a copy ascending decouples the algorithm from DS sort order. |
| Inserted between Layer 1+2 and Layer 3 | Layer 1+2 short-circuits first (preserves existing behavior); forward chain is Layer 1b that catches the gap. |
| `actionType` parameter on `CheckPostAnalysisConditions` | Reconciler already has the value from `AIAnalysis.Status.SelectedWorkflow.ActionType`. Consistent API: all routing-relevant data passed explicitly. |
| Prompt uses two variants based on effectivenessScore | Zero-effectiveness entries get mandatory "MUST NOT" language; mixed-effectiveness entries keep strengthened advisory. |
| Prompt change is belt-and-suspenders | RO guardrail is the primary defense; prompt escalation is secondary. |

### Risk Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| DS returns entries descending; forward chain needs chronological order | HIGH | `countForwardChain` sorts a copy ascending. Test mock data uses descending order to match production. |
| Existing UT-RO-214 test data could accidentally trigger forward chain | MEDIUM | Verified: existing test data uses `preHash="abc123hash"` with postHash `"post1"/"post2"/"post3"` -- no forward links. Also, no ActionType or SignalResolved set, so filter excludes them. |
| Python UT-RH-PROMPT-035 assertion breakage from text change | MEDIUM | New text preserves "Escalate" keyword. UT-HAPI-525-002 validates variant B path (mixed effectiveness). |
| `_detect_completed_but_recurring` needs effectivenessScore conditional | LOW | Two-variant generation: mandatory for zero-effectiveness, advisory for mixed. Separate test for each variant. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit (Go)**: >=80% of `countForwardChain` and integration point in `CheckIneffectiveRemediationChain` (10 scenarios cover all branches including ActionType mismatch, EA success, window expiry)
- **Unit (Python)**: >=80% of modified warning generation in `build_remediation_history_section` (2 scenarios cover both variants)

### 2-Tier Minimum

- **Go RO fix**: Unit tests (new scenarios) + existing integration tests (IT-RO-214 wiring). Satisfies 2-tier minimum.
- **Python HAPI fix**: Unit tests (updated warning text). Integration tier not applicable (prompt text generation is pure logic).

### Business Outcome Quality Bar

Tests validate that the **operator** sees remediation blocked with a clear escalation reason when the platform detects a forward hash chain proving repeated ineffective remediation of the same action type. Tests also validate that the **LLM** receives mandatory (not advisory) escalation directives when history shows zero-effectiveness completed remediations.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/routing/blocking.go` | `countForwardChain` (updated with 5-condition filter), integration point in `CheckIneffectiveRemediationChain`, `ForwardChainThreshold`/`ForwardChainWindow` config | ~50 |
| `kubernaut-agent/src/extensions/remediation_history_prompt.py` | Warning variant selection in `build_remediation_history_section`, `_all_zero_effectiveness` helper | ~25 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/routing/blocking.go` | `CheckIneffectiveRemediationChain` (DS query + chain evaluation) | Existing ~60 lines -- covered by IT-RO-214 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-042.7 | Forward hash chain: 2 linked entries (same action type, failed EA, 1h window) block with IneffectiveChain | P0 | Unit | UT-RO-525-001 | Pass |
| BR-ORCH-042.7 | Forward chain requires link to incoming RR's preHash | P0 | Unit | UT-RO-525-002 | Pass |
| BR-ORCH-042.7 | Broken forward chain does not block | P0 | Unit | UT-RO-525-003 | Pass |
| BR-ORCH-042.7 | Forward chain below threshold does not block | P1 | Unit | UT-RO-525-004 | Pass |
| BR-ORCH-042.7 | Missing postHash entries fail-open | P1 | Unit | UT-RO-525-005 | Pass |
| BR-ORCH-042.7 | Layer 1+2 regression takes precedence over forward chain | P1 | Unit | UT-RO-525-006 | Pass |
| BR-ORCH-042.7 | Memory-escalation scenario blocked (Issue #525 regression) | P0 | Unit | UT-RO-525-007 | Pass |
| BR-ORCH-042.7 | Different action type breaks chain | P0 | Unit | UT-RO-525-008 | Pass |
| BR-ORCH-042.7 | Successful EA (SignalResolved=true) breaks chain | P0 | Unit | UT-RO-525-009 | Pass |
| BR-ORCH-042.7 | Entries outside 1h window excluded | P1 | Unit | UT-RO-525-010 | Pass |
| Issue #224 | Zero-effectiveness recurring entries produce mandatory escalation | P1 | Unit (Py) | UT-HAPI-525-001 | Pass |
| Issue #224 | Mixed-effectiveness recurring entries produce advisory escalation | P1 | Unit (Py) | UT-HAPI-525-002 | Pass |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: `RO` (Remediation Orchestrator), `HAPI` (HolmesGPT-API)
- **BR_NUMBER**: 525 (Issue number)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests (Go -- RO Routing)

**Testable code scope**: `countForwardChain` in `pkg/remediationorchestrator/routing/blocking.go` -- target >=80% of new code

**Constraint**: All mock data in DESCENDING order (most recent first) to match production DS behavior. All entries use `newForwardChainEntry` helper with explicit `ActionType` and `SignalResolved` fields.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-525-001` | Operator sees RR blocked with `IneffectiveChain` when 2 entries (same action type, failed EA, within 1h) form forward hash chain | Pass |
| `UT-RO-525-002` | Forward chain requires incoming RR's preHash to match last entry's postHash | Pass |
| `UT-RO-525-003` | Forward chain with gap in hash links does NOT block (chain length 1 < threshold 2) | Pass |
| `UT-RO-525-004` | Forward chain of 1 entry (below threshold 2) does NOT block | Pass |
| `UT-RO-525-005` | Entries with missing postHash fail-open (no false positives) | Pass |
| `UT-RO-525-006` | Layer 1+2 regression detection takes precedence over forward chain | Pass |
| `UT-RO-525-007` | Memory-escalation scenario: 2 increasing-limits cycles (same action type) detected and blocked | Pass |
| `UT-RO-525-008` | Different ActionType on entries prevents chain formation | Pass |
| `UT-RO-525-009` | Entry with SignalResolved=true (successful EA) prevents chain formation | Pass |
| `UT-RO-525-010` | Entries outside the 1h ForwardChainWindow are excluded from chain detection | Pass |

### Tier 1: Unit Tests (Python -- HAPI Prompt)

**Testable code scope**: `build_remediation_history_section` in `kubernaut-agent/src/extensions/remediation_history_prompt.py` -- target >=80% of modified lines

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-525-001` | Zero-effectiveness recurring entries produce mandatory "MUST NOT" language | Pass |
| `UT-HAPI-525-002` | Mixed-effectiveness recurring entries produce advisory with "Escalate" (not "MUST NOT") | Pass |

### Tier Skip Rationale

- **Integration (Go)**: No new integration-testable code. `countForwardChain` is pure logic on `[]RemediationHistoryEntry`. DS wiring covered by existing IT-RO-214 tests.
- **E2E**: Multi-cycle OOMKill scenario requires Kind cluster with demo workloads. Tracked in demo-scenarios repo.

---

## 6. Test Cases (Detail)

### UT-RO-525-001: Forward hash chain of 2 entries triggers block

**BR**: BR-ORCH-042.7
**Type**: Unit
**File**: `test/unit/remediationorchestrator/routing/ineffective_chain_test.go`

**Given**: DS returns 2 entries in descending order (most recent first), both with `ActionType=IncreaseMemoryLimits`, `SignalResolved=false`, within 1h:
  - Entry 0: `preHash=B, postHash=C` (completed 20min ago) -- most recent
  - Entry 1: `preHash=A, postHash=B` (completed 40min ago) -- oldest
  - Incoming RR has `preRemediationSpecHash=C`, `actionType=IncreaseMemoryLimits`

**When**: `CheckIneffectiveRemediationChain` is called

**Then**: Returns `BlockingCondition` with `Reason=IneffectiveChain` and message containing "forward hash chain"

**Acceptance Criteria**:
- `blocked` is non-nil
- `blocked.Reason` equals `string(remediationv1.BlockReasonIneffectiveChain)`
- `blocked.Message` contains "forward hash chain"

---

### UT-RO-525-002: Forward chain requires link to incoming RR

**BR**: BR-ORCH-042.7
**Type**: Unit

**Given**: DS returns 2 forward-linked entries (A->B, B->C) within 1h, same action type, failed EA. Incoming RR has `preRemediationSpecHash=X` (does NOT match `C`)

**When**: `CheckIneffectiveRemediationChain` is called

**Then**: Returns nil

---

### UT-RO-525-003: Broken forward chain does not block

**BR**: BR-ORCH-042.7
**Type**: Unit

**Given**: DS returns 3 entries within 1h, same action type, failed EA. Entry[1]'s postHash does NOT match entry[0]'s preHash:
  - Entry 0: `preHash=C, postHash=D` (10min ago)
  - Entry 1: `preHash=X, postHash=Y` (20min ago) -- gap: Y != C
  - Entry 2: `preHash=A, postHash=B` (30min ago)
  - Incoming RR preHash=D

**When**: `CheckIneffectiveRemediationChain` is called

**Then**: Returns nil (chain broken, only 1 linked entry below threshold 2)

---

### UT-RO-525-004: Below threshold does not block

**BR**: BR-ORCH-042.7
**Type**: Unit

**Given**: DS returns 1 entry (A->B), incoming preHash=B

**When**: `CheckIneffectiveRemediationChain` is called

**Then**: Returns nil (chain length 1 < threshold 2)

---

### UT-RO-525-005: Missing postHash entries fail-open

**BR**: BR-ORCH-042.7
**Type**: Unit

**Given**: DS returns 2 entries with no `PostRemediationSpecHash` set

**When**: `CheckIneffectiveRemediationChain` is called

**Then**: Returns nil (fail-open)

---

### UT-RO-525-006: Layer 1+2 takes precedence over forward chain

**BR**: BR-ORCH-042.7
**Type**: Unit

**Given**: DS returns 3 entries matching BOTH Layer 1+2 (all preHash == currentPreHash, regression) AND forming a forward chain

**When**: `CheckIneffectiveRemediationChain` is called

**Then**: Returns `BlockingCondition` with message "hash chain match" (Layer 1+2), NOT "forward hash chain"

---

### UT-RO-525-007: Memory-escalation scenario (Issue #525 regression)

**BR**: BR-ORCH-042.7, Issue #525
**Type**: Unit

**Given**: DS returns 2 entries modeling the OOMKill memory-escalation scenario:
  - Entry 0: `preHash=hash256, postHash=hash512` (20min ago) -- 256->512Mi
  - Entry 1: `preHash=hash128, postHash=hash256` (40min ago) -- 128->256Mi
  - Both: `ActionType=IncreaseMemoryLimits`, `SignalResolved=false`
  - Incoming RR `preRemediationSpecHash=hash512` (OOMKill recurred at 512Mi)

**When**: `CheckIneffectiveRemediationChain` is called

**Then**: Returns `BlockingCondition`

---

### UT-RO-525-008: Different action type breaks chain

**BR**: BR-ORCH-042.7
**Type**: Unit

**Given**: 2 forward-linked entries within 1h, both `SignalResolved=false`, but different `ActionType` values (`"RestartPod"` vs `"IncreaseMemoryLimits"`)

**When**: `CheckIneffectiveRemediationChain` called with `actionType=IncreaseMemoryLimits`

**Then**: Returns nil -- different action types mean different remediation approaches were tried

---

### UT-RO-525-009: Successful EA breaks chain

**BR**: BR-ORCH-042.7
**Type**: Unit

**Given**: 2 forward-linked entries within 1h, same `ActionType`, but entry[1] has `SignalResolved=true`

**When**: `CheckIneffectiveRemediationChain` is called

**Then**: Returns nil -- successful EA proves the action type works; signal recurrence is a new problem

---

### UT-RO-525-010: Entries outside 1h forward chain window

**BR**: BR-ORCH-042.7
**Type**: Unit

**Given**: 2 forward-linked entries, same `ActionType`, both `SignalResolved=false`, but oldest entry is 2h ago (outside 1h ForwardChainWindow)

**When**: `CheckIneffectiveRemediationChain` is called

**Then**: Returns nil -- entries outside the forward chain window are excluded

---

### UT-HAPI-525-001: Mandatory escalation for zero-effectiveness recurring remediations

**BR**: Issue #224, Issue #525
**Type**: Unit (Python)
**File**: `kubernaut-agent/tests/unit/test_remediation_history_prompt.py`

**Given**: Remediation history context with tier1 chain containing 2 entries:
  - Entry 1: `actionType=IncreaseMemoryLimits`, `outcome=completed`, `effectivenessScore=0.0`, `signalResolved=false`, `signalType=OOMKilled`
  - Entry 2: same workflow/signal, `effectivenessScore=0.0`, `signalResolved=false`

**When**: `build_remediation_history_section` is called with `escalation_threshold=2`

**Then**: Output contains mandatory "MUST NOT" language

**Acceptance Criteria**:
- Output contains "MUST NOT"
- Output does NOT contain "Recommend selecting"
- Output references `IncreaseMemoryLimits`

---

### UT-HAPI-525-002: Advisory escalation for mixed-effectiveness recurring remediations

**BR**: Issue #224
**Type**: Unit (Python)
**File**: `kubernaut-agent/tests/unit/test_remediation_history_prompt.py`

**Given**: Remediation history context with tier1 chain containing 2 entries:
  - Entry 1: `actionType=IncreaseMemoryLimits`, `outcome=completed`, `effectivenessScore=0.8`, `signalType=OOMKilled`
  - Entry 2: same workflow/signal, `effectivenessScore=0.75`

**When**: `build_remediation_history_section` is called with `escalation_threshold=2`

**Then**: Output contains advisory warning with "Escalate" but NOT mandatory "MUST NOT"

**Acceptance Criteria**:
- Output contains "REPEATED INEFFECTIVE" (upper case)
- Output contains "Escalate" (preserves keyword for existing UT-RH-PROMPT-035)
- Output does NOT contain "MUST NOT"

---

## 7. Test Infrastructure

### Unit Tests (Go)

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `mockHistoryQuerier` (existing -- mocks DS client, external dependency)
- **Helpers**: `newDSEntry`, `newDSEntryNoHash`, `newForwardChainEntry` (new), `setupEngine` (existing)
- **Location**: `test/unit/remediationorchestrator/routing/ineffective_chain_test.go`

### Unit Tests (Python)

- **Framework**: pytest (existing HAPI test framework)
- **Mocks**: None (pure function testing)
- **Location**: `kubernaut-agent/tests/unit/test_remediation_history_prompt.py`

---

## 8. Execution

```bash
# Go unit tests -- all RO routing (existing + new)
go test ./test/unit/remediationorchestrator/routing/... -v

# Go unit tests -- focus on Issue #525
go test ./test/unit/remediationorchestrator/routing/... -ginkgo.focus="Issue #525"

# Python unit tests -- all prompt tests
cd kubernaut-agent && python3 -m pytest tests/unit/test_remediation_history_prompt.py -v

# Python unit tests -- focus on Issue #525
cd kubernaut-agent && python3 -m pytest tests/unit/test_remediation_history_prompt.py -k "525" -v

# Build validation
go build ./...
go vet ./pkg/remediationorchestrator/routing/...
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for Issue #525 forward hash chain detection |
| 2.0 | 2026-03-04 | Revised design: threshold=2, 1h window, ActionType matching, SignalResolved==false EA check. Added UT-RO-525-008/009/010. Updated from BR-ORCH-042.5 to BR-ORCH-042.7. All 12 tests passing. |
