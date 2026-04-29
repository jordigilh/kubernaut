# CP-0: Design Gate — Test Case Specifications

**Checkpoint**: CP-0
**Gate Type**: Document Review (no executable tests)
**Total Checks**: 22
**Merge Criteria**: All 22 review checks pass before PR1 begins
**Reviewer**: Architecture Team + Project Lead

---

## Overview

CP-0 validates that all design documentation, threat models, and governance artifacts exist and are internally consistent before any code implementation begins. These are manual review checks, not automated tests.

---

## Test Categories

| Category | Check Count | Severity |
|----------|-------------|----------|
| Threat Model | 5 | Critical |
| Security Audit | 6 | Critical |
| QE Coverage | 5 | High |
| Governance | 6 | Medium |

---

## THREAT: Threat Model Completeness

### THREAT-01: Authentication bypass paths enumerated

**BR**: BR-INTERACTIVE-001, BR-INTERACTIVE-002
**Type**: Document Review
**Category**: Security

**Description**: Verify that DD-AUTH-MCP-001 enumerates all authentication bypass paths and documents mitigations for each.

**Preconditions**:
- DD-AUTH-MCP-001 exists at `docs/architecture/decisions/DD-AUTH-MCP-001-mcp-endpoint-security.md`

**Review Steps**:
1. Open DD-AUTH-MCP-001 §Decision → Impersonation Model
2. Verify Pattern A (direct) and Pattern B (delegated) are both documented
3. Verify header stripping is documented as defense-in-depth
4. Verify rejection cases listed: no auth, expired token, forged bearer, missing impersonation header
5. Verify each path has a corresponding test in CP-2 (PEN-01 through PEN-14)

**Acceptance Criteria**:
- [ ] Both authentication patterns documented with full request flow
- [ ] All rejection scenarios enumerated (>=8 distinct failure modes)
- [ ] Each rejection maps to a named PEN-XX test
- [ ] Header stripping mechanism documented with code snippet

---

### THREAT-02: Impersonation escalation paths documented

**BR**: BR-INTERACTIVE-002
**Type**: Document Review
**Category**: Security

**Description**: Verify that privilege escalation via impersonation is analyzed and mitigated.

**Preconditions**:
- DD-AUTH-MCP-001 exists
- Known Limitations section present

**Review Steps**:
1. Open DD-AUTH-MCP-001 §Known Limitations
2. Verify "Impersonation scope unbounded (H-SEC-1)" is documented
3. Verify defense-in-depth layers are enumerated (apifrontend CEL, NetworkPolicy, K8s audit, KA audit)
4. Verify `Impersonate-Extra-*` headers are stripped (not just User/Group)
5. Verify self-impersonation scenario is documented as rejected

**Acceptance Criteria**:
- [ ] H-SEC-1 known limitation documented with 4 defense layers
- [ ] `Impersonate-Extra-*` stripping confirmed in code snippet
- [ ] Self-impersonation explicitly called out as rejected
- [ ] SAR verification for Pattern B impersonate RBAC documented

---

### THREAT-03: Session hijacking attack surface documented

**BR**: BR-INTERACTIVE-004, BR-INTERACTIVE-005
**Type**: Document Review
**Category**: Security

**Description**: Verify that session hijacking vectors are analyzed in DD-INTERACTIVE-002.

**Preconditions**:
- DD-INTERACTIVE-002 exists at `docs/architecture/decisions/DD-INTERACTIVE-002-dynamic-takeover-model.md`

**Review Steps**:
1. Open DD-INTERACTIVE-002 §Observer/Driver Model
2. Verify single-driver guarantee documented (K8s Lease)
3. Verify session ID is crypto-random (not sequential/predictable)
4. Verify that takeover requires explicit `action: takeover` (not implicit)
5. Verify identity stored in session metadata (cannot be spoofed post-creation)

**Acceptance Criteria**:
- [ ] Single-driver enforced by K8s Lease documented
- [ ] Session ID generation specified as crypto-random
- [ ] Explicit takeover requirement documented with JSON example
- [ ] Identity binding to session at creation time documented

---

### THREAT-04: Data exposure via audit trail

**BR**: BR-INTERACTIVE-003
**Type**: Document Review
**Category**: Security

**Description**: Verify that data classification policy prevents sensitive data exposure through audit events.

**Preconditions**:
- DD-AUTH-MCP-001 §Data Classification Policy exists

**Review Steps**:
1. Open DD-AUTH-MCP-001 §Data Classification Policy
2. Verify `aiagent.interactive.k8s_call` classified as Sensitive with redaction
3. Verify Secret data values are redacted from K8s call payloads
4. Verify LLM request/response configurable verbosity
5. Verify session events classified as Operational (no redaction needed)

**Acceptance Criteria**:
- [ ] Data classification table present with >=4 event types classified
- [ ] Secret redaction explicitly required for k8s_call events
- [ ] LLM verbosity configurable (not always full payload)
- [ ] Classification aligns with SOC2 CC8.1

---

### THREAT-05: Denial of service vectors documented

**BR**: BR-INTERACTIVE-005, BR-INTERACTIVE-008
**Type**: Document Review
**Category**: Security

**Description**: Verify that DoS vectors (resource exhaustion, session flooding) are analyzed and mitigated.

**Preconditions**:
- DD-AUTH-MCP-001 §Error Taxonomy exists
- DD-INTERACTIVE-002 §Timeout Model exists

**Review Steps**:
1. Verify rate limiting documented (`rate_limited` error code, 429 + Retry-After)
2. Verify global timeout (1h) as hard cap prevents indefinite session hold
3. Verify Lease (30s) prevents multiple sessions per investigation
4. Verify inactivity timeout (10m) releases idle sessions
5. Verify NotificationBus slow consumer doesn't block publisher (bounded buffer + drop policy)

**Acceptance Criteria**:
- [ ] Rate limiting with 429 + Retry-After documented
- [ ] Global timeout hard cap documented (1h)
- [ ] Lease-based exclusivity documented (30s expiry)
- [ ] Inactivity timeout documented (10m default, configurable)
- [ ] NotificationBus bounded buffer + drop policy documented

---

## SEC: Security Audit of DD-AUTH-MCP-001

### SEC-01: Header stripping completeness

**BR**: BR-INTERACTIVE-002
**Type**: Document Review
**Category**: Security

**Description**: Verify header stripping covers all K8s impersonation headers per K8s API spec.

**Preconditions**:
- DD-AUTH-MCP-001 §Header Stripping exists with code snippet

**Review Steps**:
1. Verify `Impersonate-User` stripped
2. Verify `Impersonate-Group` stripped
3. Verify `Impersonate-Uid` stripped
4. Verify `Impersonate-Extra-*` (prefix match, case-insensitive) stripped
5. Verify stripping happens BEFORE any other processing (middleware order)

**Acceptance Criteria**:
- [ ] All 4 header types stripped (User, Group, Uid, Extra-*)
- [ ] Case-insensitive matching for Extra-* prefix
- [ ] Middleware ordering guarantee documented (stripping is first)
- [ ] Code snippet matches expected implementation

---

### SEC-02: SAR verification for Pattern B

**BR**: BR-INTERACTIVE-002
**Type**: Document Review
**Category**: Security

**Description**: Verify SAR check prevents unauthorized impersonation delegation.

**Preconditions**:
- DD-AUTH-MCP-001 §Effective User Extraction exists

**Review Steps**:
1. Verify SAR checks `impersonate` verb on `users` resource for the target user
2. Verify SAR failure returns `ErrForbiddenImpersonation` (not a generic error)
3. Verify SAR is checked BEFORE using impersonation headers
4. Verify error taxonomy includes `rbac_denied` for this case

**Acceptance Criteria**:
- [ ] SAR resource: `users`, verb: `impersonate` documented
- [ ] Named error type for SAR failure
- [ ] SAR precedes identity extraction (fail-closed)
- [ ] Maps to `rbac_denied` in error taxonomy

---

### SEC-03: Token validation extracts groups

**BR**: BR-INTERACTIVE-002
**Type**: Document Review
**Category**: Security

**Description**: Verify `ValidateTokenFull` returns groups (not just username) from TokenReview.

**Preconditions**:
- Issue kubernaut#895 referenced in DD-AUTH-MCP-001

**Review Steps**:
1. Verify DD-AUTH-MCP-001 states groups are extracted from TokenReview response
2. Verify groups are included in `ImpersonationConfig` for K8s API calls
3. Verify Pattern A uses groups from TokenReview; Pattern B uses groups from `Impersonate-Group` headers
4. Verify empty groups is valid (not rejected)

**Acceptance Criteria**:
- [ ] Groups extraction from TokenReview.Status.User.Groups documented
- [ ] Groups included in `rest.ImpersonationConfig{Groups: ...}` 
- [ ] Both patterns document group source (TokenReview vs header)
- [ ] Empty groups handled (defaults to `["system:authenticated"]` or similar)

---

### SEC-04: Error taxonomy prevents information disclosure

**BR**: BR-INTERACTIVE-008
**Type**: Document Review
**Category**: Security

**Description**: Verify error messages don't leak internal state to unauthenticated callers.

**Preconditions**:
- DD-AUTH-MCP-001 §Error Taxonomy exists

**Review Steps**:
1. Verify 401 errors don't reveal which auth mechanism failed
2. Verify 403 errors don't reveal the specific RBAC rule that blocked
3. Verify `lease_held` includes username of current holder (intentional for UX)
4. Verify rate limit errors include only `Retry-After` seconds (not internal counters)
5. Verify no stack traces in error responses

**Acceptance Criteria**:
- [ ] 401 messages generic ("Authentication required" / "Token validation failed")
- [ ] 403 messages generic ("Insufficient permissions")
- [ ] `lease_held` includes `{user}` and `{time}` (documented as intentional)
- [ ] No internal state leaked in any error response
- [ ] Error taxonomy table present with all stable codes

---

### SEC-05: Feature gate isolation verified

**BR**: BR-INTERACTIVE-008
**Type**: Document Review
**Category**: Security

**Description**: Verify feature gate `interactive.enabled: false` results in zero attack surface.

**Preconditions**:
- DD-AUTH-MCP-001 §Feature Gate Naming Convention exists

**Review Steps**:
1. Verify config field location: `Interactive InteractiveConfig`
2. Verify Helm path: `kubernautAgent.interactive.enabled`
3. Verify when disabled: MCP handler not registered (404, not 403)
4. Verify no MCP-related goroutines or listeners when disabled
5. Verify default is `false` (disabled by default)

**Acceptance Criteria**:
- [ ] Feature gate naming consistent across all locations (config, Helm, operator)
- [ ] Disabled = 404 (handler not registered, not authorization failure)
- [ ] Default is `false`
- [ ] No residual listeners/goroutines when disabled

---

### SEC-06: Non-K8s tool trust boundary documented

**BR**: BR-INTERACTIVE-002
**Type**: Document Review
**Category**: Security

**Description**: Verify that non-K8s tools (Prometheus, DS) using KA SA is an explicit, documented trust boundary.

**Preconditions**:
- DD-AUTH-MCP-001 §Non-K8s Tool Calls exists
- DD-AUTH-MCP-001 §Known Limitations exists

**Review Steps**:
1. Verify Prometheus queries documented as KA SA (no user auth available)
2. Verify DS queries documented as KA SA (internal service)
3. Verify this is listed in Known Limitations
4. Verify interactive audit events still attribute `acting_user` correctly even though underlying query uses KA SA

**Acceptance Criteria**:
- [ ] Non-K8s tool trust boundary explicitly documented
- [ ] Listed in Known Limitations section
- [ ] Audit attribution still works (user identity in event, SA for actual query)
- [ ] No path for user to escalate via Prometheus/DS queries

---

## QE: Test Plan Coverage Audit

### QE-01: All BRs have test coverage

**BR**: All BR-INTERACTIVE-*
**Type**: Document Review
**Category**: Coverage

**Description**: Verify every BR-INTERACTIVE requirement maps to at least one test.

**Preconditions**:
- TEST_PLAN.md §BR Coverage Matrix exists
- BR-INTERACTIVE.md exists

**Review Steps**:
1. Count BRs in BR-INTERACTIVE.md (should be 8)
2. Verify all 8 appear in TEST_PLAN.md §BR Coverage Matrix
3. Verify each has at least one test tier assigned
4. Verify P0 BRs have Unit + Integration + E2E tiers

**Acceptance Criteria**:
- [ ] 8/8 BRs mapped in coverage matrix
- [ ] P0 BRs (001-004) have tests in all 3 tiers
- [ ] P1 BRs (005-006) have tests in Unit + Integration
- [ ] P2 BRs (007-008) have tests in at least Unit

---

### QE-02: Security-critical coverage >=90% target documented

**BR**: BR-INTERACTIVE-002
**Type**: Document Review
**Category**: Coverage

**Description**: Verify elevated coverage targets for security-critical files.

**Preconditions**:
- TEST_PLAN.md §Success Metrics exists

**Review Steps**:
1. Verify security-critical files listed: `auth/`, `mcp/auth.go`, `mcp/impersonate.go`
2. Verify >=90% target for these files
3. Verify standard files have >=80% target
4. Verify coverage measurement commands documented

**Acceptance Criteria**:
- [ ] Security-critical file list present
- [ ] >=90% target for security files
- [ ] >=80% target for standard files
- [ ] Coverage commands in §Execution section

---

### QE-03: CP-2 penetration test count matches threat model

**BR**: BR-INTERACTIVE-002
**Type**: Document Review
**Category**: Coverage

**Description**: Verify that the number of penetration tests in CP-2 matches the enumerated threat vectors.

**Preconditions**:
- CP-2_SECURITY_GATE.md exists
- DD-AUTH-MCP-001 threat vectors documented

**Review Steps**:
1. Count distinct threat vectors in DD-AUTH-MCP-001 (auth bypass paths)
2. Count PEN-XX tests in CP-2
3. Verify 1:1 mapping (each threat has a test)
4. Verify no orphan tests (tests without corresponding threat)

**Acceptance Criteria**:
- [ ] At least 14 penetration test scenarios
- [ ] Each maps to a documented threat vector
- [ ] No gaps (threat without test)
- [ ] No orphans (test without threat)

---

### QE-04: Adversarial scenario coverage for takeover

**BR**: BR-INTERACTIVE-004
**Type**: Document Review
**Category**: Coverage

**Description**: Verify CP-3 TAKE-* tests cover all adversarial takeover scenarios.

**Preconditions**:
- CP-3_SESSION_AUDIT_GATE.md exists
- DD-INTERACTIVE-002 takeover flow documented

**Review Steps**:
1. Verify takeover mid-LLM-turn scenario exists (race condition)
2. Verify rapid connect/disconnect scenario exists (stability)
3. Verify concurrent takeover attempt scenario exists (Lease contention)
4. Verify DS failure during reconstruct scenario exists (degradation)
5. Verify identity verification on resume scenario exists (security)

**Acceptance Criteria**:
- [ ] At least 14 TAKE-* adversarial scenarios
- [ ] Covers: race conditions, rapid cycling, concurrency, failures, identity
- [ ] Each scenario has clear pass/fail criteria
- [ ] Integration tier tests use real K8s Lease (not mocked)

---

### QE-05: E2E scenario covers full user journey

**BR**: BR-INTERACTIVE-001, BR-INTERACTIVE-004
**Type**: Document Review
**Category**: Coverage

**Description**: Verify CP-5 E2E tests cover the complete interactive user journey end-to-end.

**Preconditions**:
- CP-5_RELEASE_GATE.md exists

**Review Steps**:
1. Verify E2E covers: connect → observe → takeover → drive → disconnect → reconstruct → autonomous completes
2. Verify E2E uses real Kind cluster (not envtest)
3. Verify E2E uses real MCP client library (not mock)
4. Verify E2E validates audit trail in DS after flow completes
5. Verify E2E has 20m timeout (separate from 15m autonomous)

**Acceptance Criteria**:
- [ ] Full 7-step user journey in single E2E test
- [ ] Kind cluster infrastructure
- [ ] Real MCP client (go-sdk)
- [ ] Post-flow DS audit validation
- [ ] 20m timeout configured

---

## GOV: Governance Documents Exist

### GOV-01: DD-AUTH-MCP-001 exists and is complete

**BR**: BR-INTERACTIVE-001, BR-INTERACTIVE-002
**Type**: Document Review
**Category**: Governance

**Description**: Design Decision for MCP endpoint security exists with all required sections.

**Preconditions**: None

**Review Steps**:
1. File exists at `docs/architecture/decisions/DD-AUTH-MCP-001-mcp-endpoint-security.md`
2. Has Status, Decision Date, Version
3. Has Context, Decision Drivers, Alternatives, Decision, Consequences, Risks
4. Has Validation Strategy referencing CP-2

**Acceptance Criteria**:
- [ ] File exists
- [ ] All DD template sections present
- [ ] Confidence >= 90%
- [ ] Cross-references DD-INTERACTIVE-002

---

### GOV-02: DD-INTERACTIVE-002 exists and supersedes DD-001

**BR**: BR-INTERACTIVE-004
**Type**: Document Review
**Category**: Governance

**Description**: Design Decision for dynamic takeover exists and explicitly supersedes DD-001.

**Preconditions**: None

**Review Steps**:
1. File exists at `docs/architecture/decisions/DD-INTERACTIVE-002-dynamic-takeover-model.md`
2. Has explicit supersession notice and delta table vs DD-001
3. DD-INTERACTIVE-001 has been marked as superseded
4. No conflicting statements between DD-001 and DD-002

**Acceptance Criteria**:
- [ ] File exists with supersession notice
- [ ] Delta table present (DD-001 vs DD-002)
- [ ] DD-001 marked as superseded
- [ ] No contradictions between documents

---

### GOV-03: BR-INTERACTIVE requirements document exists

**BR**: All
**Type**: Document Review
**Category**: Governance

**Description**: Business requirements for interactive mode are formally documented.

**Preconditions**: None

**Review Steps**:
1. File exists at `docs/requirements/BR-INTERACTIVE.md`
2. Contains BR-INTERACTIVE-001 through BR-INTERACTIVE-008
3. Each BR has: Business Need, Success Criteria, Priority, Status
4. Traceability matrix maps BRs to PRs and CPs

**Acceptance Criteria**:
- [ ] File exists with 8 BRs
- [ ] Each BR has Business Need + Success Criteria
- [ ] Priorities assigned (P0/P1/P2)
- [ ] Traceability matrix present

---

### GOV-04: Test plan exists with IEEE 829 structure

**BR**: All
**Type**: Document Review
**Category**: Governance

**Description**: Formal test plan exists following IEEE 829 hybrid format.

**Preconditions**: None

**Review Steps**:
1. `docs/tests/703/TEST_PLAN.md` exists
2. Has IEEE 829 sections: Introduction, References, Risks, Scope, Approach, Pass/Fail, Suspension/Resumption, Deliverables, Responsibilities
3. Has per-checkpoint detail files (CP-0 through CP-5)
4. Has BR Coverage Matrix

**Acceptance Criteria**:
- [ ] Master test plan exists
- [ ] IEEE 829 structural compliance
- [ ] 6 per-checkpoint detail files exist
- [ ] 138 total checks mapped

---

### GOV-05: DD-INTERACTIVE-001 marked as superseded

**BR**: BR-INTERACTIVE-004
**Type**: Document Review
**Category**: Governance

**Description**: The old design decision is explicitly marked to prevent confusion.

**Preconditions**: None

**Review Steps**:
1. Open `docs/architecture/decisions/DD-INTERACTIVE-001-interactive-mode-crd-placement-and-timeouts.md`
2. Verify "Superseded" status at top of document
3. Verify pointer to DD-INTERACTIVE-002

**Acceptance Criteria**:
- [ ] Status field says "Superseded"
- [ ] References DD-INTERACTIVE-002 as successor
- [ ] Original content preserved (not deleted)

---

### GOV-06: Error taxonomy and metrics documented

**BR**: BR-INTERACTIVE-008
**Type**: Document Review
**Category**: Governance

**Description**: Stable error codes and Prometheus metrics formally documented.

**Preconditions**: None

**Review Steps**:
1. DD-AUTH-MCP-001 §Error Taxonomy table exists with >=8 stable codes
2. DD-AUTH-MCP-001 §Metric Definitions table exists with >=4 metrics
3. Each error code has: code, HTTP status, human message, next step
4. Each metric has: name, type, description

**Acceptance Criteria**:
- [ ] Error taxonomy table with >=8 codes
- [ ] Metric definitions table with >=4 metrics
- [ ] Error codes follow kebab-case convention
- [ ] Metrics follow `kubernaut_interactive_*` naming

---

## Automated Verification (CI)

```bash
# CP-0 automated file-existence checks (run in CI)
ls docs/architecture/decisions/DD-AUTH-MCP-001-mcp-endpoint-security.md
ls docs/architecture/decisions/DD-INTERACTIVE-002-dynamic-takeover-model.md
ls docs/requirements/BR-INTERACTIVE.md
ls docs/tests/703/TEST_PLAN.md
ls docs/tests/703/CP-0_DESIGN_GATE.md
ls docs/tests/703/CP-1_CRD_CONTROLLER_GATE.md
ls docs/tests/703/CP-2_SECURITY_GATE.md
ls docs/tests/703/CP-3_SESSION_AUDIT_GATE.md
ls docs/tests/703/CP-4_TOOL_COMPLETENESS_GATE.md
ls docs/tests/703/CP-5_RELEASE_GATE.md

# Verify DD-INTERACTIVE-001 contains "Superseded"
grep -q "Superseded" docs/architecture/decisions/DD-INTERACTIVE-001-interactive-mode-crd-placement-and-timeouts.md
```
