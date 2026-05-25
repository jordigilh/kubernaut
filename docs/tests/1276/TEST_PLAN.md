# Test Plan: Per-Request Prompt Personalization (Issue #1276)

## 1. Test Plan Identifier

TP-AF-1276-v1.0

## 2. References

- Issue: https://github.com/jordigilh/kubernaut/issues/1276
- BR: BR-API-1276 — AF per-request prompt personalization (role-aware, client-type)
- FedRAMP Controls: AC-6 (Least Privilege), SC-7 (Boundary Protection), AU-2 (Audit Events)
- ADK: `google.golang.org/adk v1.2.0` — `llmagent.InstructionProvider`
- Related: #1275 (static context), #1278 (SAR-based role derivation)

## 3. Introduction

This test plan validates that the AF agent prompt is dynamically enriched per-request with role-aware behavioral guidance based on the authenticated user's JWT groups. The implementation uses ADK's native `InstructionProvider` callback, which is invoked on every LLM invocation with the request context.

## 4. Test Items

| Component | Version | Description |
|---|---|---|
| `pkg/apifrontend/agent/prompt.go` | v1.5.0 | NewInstructionProvider + roleGuidance |
| `pkg/apifrontend/agent/config.go` | v1.5.0 | InstructionProvider field in AgentConfig |
| `pkg/apifrontend/agent/root.go` | v1.5.0 | Wire InstructionProvider into llmagent.Config |
| `cmd/apifrontend/main.go` | v1.5.0 | Use NewInstructionProvider(namespace) |

## 5. Software Risk Issues

| Risk | Impact | Mitigation |
|---|---|---|
| UserIdentity not available in InstructionProvider context | High | Spike confirmed: context propagates through ADK chain |
| Group names are deployment-specific | Medium | Use known test groups; #1278 tracks SAR-based approach |
| InstructionProvider adds latency per-request | Low | No I/O, only string concatenation (~1μs) |
| Role guidance could leak sensitive group names | Medium | Only emit guidance text, never raw group names |

## 6. Features to be Tested

### 6.1 InstructionProvider Wiring (SC-7)

- F-1: InstructionProvider takes priority over static Instruction field
- F-2: Base instruction (from BuildInstruction) always included in output
- F-3: Core prompt section is immutable regardless of user identity
- F-4: Provider returns no error for nil/empty identity (graceful degradation)

### 6.2 Role Guidance Composition (AC-6)

- F-5: SRE group produces full-access guidance paragraph
- F-6: Viewer/observability group produces read-only guidance paragraph
- F-7: Approver group produces approval-focused guidance paragraph
- F-8: CICD group produces automation/terse guidance paragraph
- F-9: Audit group produces compliance-focused guidance paragraph
- F-10: Multiple groups produce multiple paragraphs (additive composition)
- F-11: Unknown groups produce no guidance (no crash, no noise)
- F-12: Empty groups array produces no role section at all

### 6.3 Security (SC-7, AC-6)

- F-13: Raw JWT group names never appear in prompt output
- F-14: Role guidance is informational only — does not override SAR enforcement
- F-15: Malicious group names (injection attempts) do not corrupt prompt structure

### 6.4 Integration (AU-2)

- F-16: A2A handler uses InstructionProvider (not static Instruction)
- F-17: Agent responds contextually based on role guidance in Kind cluster

## 7. Features Not to be Tested

- SAR-based role derivation (deferred to #1278)
- Client-type awareness via MCP clientInfo (deferred to future enhancement)
- Session preamble with active cluster state (deferred)
- LLM behavioral compliance with guidance (subjective)

## 8. Approach

### Test Pyramid

| Tier | Scope | Count |
|---|---|---|
| Unit | InstructionProvider, roleGuidance, composition, edge cases | 15+ |
| Integration | Full A2A handler with identity-injected context | 3 |
| E2E | Agent with authenticated user in Kind cluster | 1 |

### FedRAMP Control Mapping

| Test ID | Control | Behavior |
|---|---|---|
| UT-AF-1276-001 | SC-7 | InstructionProvider preserves core prompt immutability |
| UT-AF-1276-002 | AC-6 | SRE group adds full-access guidance |
| UT-AF-1276-003 | AC-6 | Viewer group adds read-only guidance |
| UT-AF-1276-004 | AC-6 | Approver group adds approval guidance |
| UT-AF-1276-005 | AC-6 | CICD group adds automation guidance |
| UT-AF-1276-006 | AC-6 | Audit group adds compliance guidance |
| UT-AF-1276-007 | AC-6 | Multi-role user gets additive guidance |
| UT-AF-1276-008 | SC-7 | Nil identity returns base instruction only |
| UT-AF-1276-009 | SC-7 | Empty groups returns base instruction only |
| UT-AF-1276-010 | SC-7 | Unknown groups produce no extra guidance |
| UT-AF-1276-011 | SC-7 | Raw group names not leaked into prompt |
| UT-AF-1276-012 | SI-10 | Malicious group names sanitized |
| IT-AF-1276-001 | SC-7 | A2A handler uses InstructionProvider |
| IT-AF-1276-002 | AC-6 | SRE user gets full-access instruction |
| IT-AF-1276-003 | AC-6 | Viewer user gets read-only instruction |

## 9. Item Pass/Fail Criteria

- All unit tests pass with `go test ./pkg/apifrontend/agent/...`
- Zero regressions in existing agent and prompt tests
- Code coverage >= 80% for new code paths
- No raw group names in InstructionProvider output (assertion)
- `go build ./...` succeeds
- `golangci-lint run` produces no new findings

## 10. Test Deliverables

- `pkg/apifrontend/agent/prompt_test.go` — InstructionProvider specs
- `pkg/apifrontend/agent/role_guidance_test.go` — roleGuidance composition specs
- `test/integration/apifrontend/instruction_provider_test.go` — IT specs

## 11. Environmental Needs

- Unit: `go test` (no external deps, mock context with UserIdentity)
- Integration: `httptest` server with A2A handler + mock LLM + identity context
- E2E: Kind cluster with AF + JWT auth configured

## 12. Schedule

| Phase | Duration |
|---|---|
| RED (failing tests) | 15 min |
| GREEN (implementation) | 20 min |
| REFACTOR (quality) | 10 min |
| Checkpoint audit | 5 min |
