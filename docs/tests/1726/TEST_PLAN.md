# Test Plan — #1726: Phase-Override `apiKeyFile` Resolution

**IEEE 829 Compliant** | **Issue**: [#1726](https://github.com/jordigilh/kubernaut/issues/1726) | **Milestone**: v1.6

## 1. Test Plan Identifier

TP-1726-PHASE-APIKEYFILE-RESOLUTION

## 2. Introduction

### 2.1 Purpose

`phaseModels`/`LLMOverrideConfig.APIKeyFile` (and `AlignmentCheckConfig.LLM.APIKeyFile`) is parsed
and threaded through config merging, but is never actually resolved into an API key at runtime.
`LLMRuntimeConfig.EffectivePhaseConfig` and `AlignmentCheckConfig.EffectiveLLM` both copy the base
profile's already-resolved `APIKey` into the override's output struct and only ever overwrite
`APIKeyFile`. Three production call sites (`buildLLMClients`, `buildAlignmentStack`,
`reloadSinglePhaseClient`) then build LLM clients directly from this merged config with no
resolution step in between — every phase/alignment client silently authenticates with the base
profile's credentials regardless of its own `apiKeyFile`, with no error, log line, or validation
failure. This test plan proves the fix: each of the 3 call sites resolves its own `apiKeyFile` into
`APIKey` before building a client.

### 2.2 Objectives

1. **Correct credential resolution**: a phase or alignment-check override with its own distinct
   `apiKeyFile` builds a client that authenticates with that file's content, not the base profile's.
2. **Backward compatibility**: a phase override with no `apiKeyFile` set continues to authenticate
   with the base profile's key, byte-for-byte unchanged.
3. **Fail loud, not silent**: a phase's own `apiKeyFile` set but unreadable/missing causes the
   affected client build to fail observably (boot: process exit; hot-reload: reload rejected,
   existing client preserved) — never a silent fallback to the wrong credentials or a directory scan.

### 2.3 Business Requirements

- BR-SECURITY-1726: Phase-scoped LLM credential resolution correctness (defect against the
  implicit contract of BR-AI-1470, per-phase model routing — "phase override" implies phase-scoped
  everything, including credentials)

## 3. Features to be Tested

- F-1: `buildLLMClients` phase loop resolves each phase override's own `apiKeyFile`
  (`cmd/kubernautagent/bootstrap.go`)
- F-2: `buildAlignmentStack` resolves the shadow/alignment-checker override's own `apiKeyFile`
  (`cmd/kubernautagent/bootstrap.go`)
- F-3: `reloadSinglePhaseClient` (hot-reload) resolves each phase override's own `apiKeyFile`
  (`cmd/kubernautagent/llm_builder.go`)
- F-4: A phase/alignment override with no `apiKeyFile` set continues to inherit the base profile's
  resolved key unchanged
- F-5: A distinct-but-unreadable phase `apiKeyFile` fails the client build observably, without
  falling back to the credentials-dir scan

## 4. Features Not to be Tested

- Base-profile credential resolution (`resolveLLMCredentials`, `main.go`) — already correct and
  already covered by existing tests; not touched by this fix
- `types.LLMConfig.ResolveAPIKey()`'s own file-read/trim/empty-check logic — already unit-tested
  (`UT-SH-1488-003/004`, `pkg/shared/types/llm_test.go`); this fix only adds callers, no new logic
- `vertexAuth`'s ambient-ADC fallback (secondary risk noted in the issue) — independent code path,
  tracked separately if it needs its own fix

## 5. Approach

### Test Pyramid

| Tier | Scope | Count |
|---|---|---|
| Unit | None new — underlying `ResolveAPIKey()` logic already covered by `UT-SH-1488-003/004` | 0 |
| Integration | Wiring proof at all 3 call sites + backward-compat + fail-loud | 5 |
| E2E | Deferred — no user-facing journey change beyond credential correctness, covered by IT | 0 |

### FedRAMP / SOC2 Control Mapping

| Control | Objective | Behavioral Assurance | Test IDs |
|---|---|---|---|
| IA-5 (Authenticator Management) | The correct, phase-scoped credential authenticates each LLM client | Phase/alignment clients with a distinct `apiKeyFile` authenticate with their own key, never the base's | IT-KA-1726-001/002/003 |
| CM-6 (Configuration Settings) | Configured `apiKeyFile` overrides are consistently reflected in runtime behavior | An override that sets `apiKeyFile` measurably changes which credential is used; an override that doesn't set it measurably does not | IT-KA-1726-004 |
| SI-10 (Information Input Validation) | An unreadable/invalid phase-specific credential file fails loudly, not silently | Hot-reload with a distinct-but-unreadable `apiKeyFile` is rejected, existing client preserved | IT-KA-1726-005 |

## 6. Test Cases

| ID | Test Case | Success Criteria | Control | Status |
|---|---|---|---|---|
| IT-KA-1726-001 | `buildLLMClients` phase override with distinct `apiKeyFile` | Phase `SwappableClient`'s outgoing request carries the phase's own key (via `Authorization` header), distinct from the base client's | IA-5 | Pass |
| IT-KA-1726-002 | `reloadSinglePhaseClient` (hot-reload) phase override with distinct `apiKeyFile` | Same as IT-KA-1726-001, driven through `llmRuntimeReloadCallback` → `reloadPhaseClients` | IA-5 | Pass |
| IT-KA-1726-003 | `buildAlignmentStack` shadow client with distinct `apiKeyFile` | Shadow client's outgoing request carries its own key, distinct from the base's | IA-5 | Pass |
| IT-KA-1726-004 | Phase override with no `apiKeyFile` set (regression) | Phase client's outgoing request carries the same key as the base client's, unchanged | CM-6 | Pass |
| IT-KA-1726-005 | Hot-reload phase override's `apiKeyFile` unreadable/missing | Reload is rejected (`reloadSinglePhaseClient` logs and returns without swapping); the existing phase client is preserved | SI-10 | Pass |

All 5 implemented in [cmd/kubernautagent/llm_builder_1726_test.go](../../../cmd/kubernautagent/llm_builder_1726_test.go), run via
`go test ./cmd/kubernautagent/... -args --ginkgo.focus="1726"` — 5/5 pass, zero regressions
across the full 89-spec `cmd/kubernautagent` suite.

## 7. Pass/Fail Criteria

- [x] All 5 integration tests pass: `go test ./cmd/kubernautagent/... -args --ginkgo.focus="1726"` (5/5 pass)
- [x] Zero regressions in existing test suites that interact with the touched call sites:
  `llm_builder_1616_test.go`, `reload_callback_1470_test.go`, `reload_callback_1616_test.go`,
  `config_1470_test.go`, `llm_builder_azure_1600_test.go`, `llm_builder_effort_1604_test.go`
  (full `cmd/kubernautagent` suite: 89/89 pass)
- [x] `go build ./...` succeeds
- [x] `golangci-lint run --timeout=5m ./cmd/kubernautagent/...` produces zero issues (a `nestif`
  complexity warning introduced transiently in `buildAlignmentStack` during GREEN was resolved in
  REFACTOR by removing a redundant `APIKeyFile != ""` guard — `ResolveAPIKey()` already no-ops on an
  empty `APIKeyFile`, so the guard was dead weight at all 3 call sites, not just this one)
- [x] Pyramid Invariant satisfied: the underlying logic (`ResolveAPIKey()`) already has UT coverage;
  this fix adds only wiring, proven by IT

## 8. Pyramid Invariant Compliance

| Component | UT (proves logic) | IT (proves wiring) | Status |
|---|---|---|---|
| `ResolveAPIKey()` file-read/trim/empty-check logic | `UT-SH-1488-003/004` (pre-existing) | -- | Compliant (no new logic introduced) |
| `buildLLMClients` phase-loop wiring | -- | IT-KA-1726-001, IT-KA-1726-004 | Compliant |
| `buildAlignmentStack` wiring | -- | IT-KA-1726-003 | Compliant |
| `reloadSinglePhaseClient` wiring | -- | IT-KA-1726-002, IT-KA-1726-005 | Compliant |

## 9. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Error Handling | IT Test ID |
|---|---|---|---|---|
| `merged.ResolveAPIKey()` call | KA boot — phase clients | `buildLLMClients`, `cmd/kubernautagent/bootstrap.go` | `os.Exit(1)` (matches existing phase-client-build failure handling) | IT-KA-1726-001 |
| `merged.ResolveAPIKey()` call | KA boot — shadow/alignment client | `buildAlignmentStack`, `cmd/kubernautagent/bootstrap.go` | `os.Exit(1)` (matches existing fail-closed shadow-client pattern) | IT-KA-1726-003 |
| `merged.ResolveAPIKey()` call | KA hot-reload — phase clients | `reloadSinglePhaseClient`, `cmd/kubernautagent/llm_builder.go` | `logger.Error` + `return` (reject reload, keep existing client) | IT-KA-1726-002, IT-KA-1726-005 |

## 10. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-07-24 | Initial test plan |
| 1.1 | 2026-07-24 | RED/GREEN/REFACTOR complete: all 5 IT tests pass, zero regressions, build/lint clean; status column added to test case table |
