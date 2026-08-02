# Kubernaut Agent - Testing Strategy

**Version**: 1.0
**Last Updated**: 2026-08-02
**Status**: ✅ Current

---

## Purpose

Describes KA's test pyramid, grounded in actual file counts as of this writing (not the original
Python/HolmesGPT-era test plan). Per [AGENTS.md](../../../../AGENTS.md#the-pyramid-invariant): unit
tests prove logic, integration tests prove wiring, E2E tests prove the journey.

---

## Test Pyramid: File Counts

| Tier | Location | File Count |
|---|---|---|
| Unit | `internal/kubernautagent/**/*_test.go` | 242 |
| Unit | `pkg/kubernautagent/**/*_test.go` | 67 |
| Unit | `cmd/kubernautagent/**/*_test.go` | 23 |
| **Unit (total)** | | **332** |
| Integration | `test/integration/kubernautagent/**/*_test.go` | 102 |
| E2E | `test/e2e/kubernautagent/**/*_test.go` | 29 |

KA's BRs are also exercised by cross-service test suites where the behavior spans a service
boundary — see [BR_MAPPING.md](./BR_MAPPING.md) (e.g. BR-KA-212's remediation-target validation is
also covered by a RemediationOrchestrator integration test).

---

## Framework

**Ginkgo/Gomega BDD** is the standard for all business-logic tests, per
[AGENTS.md](../../../../AGENTS.md#testing-requirements). Of KA's unit test files, 5 in
`internal/kubernautagent` do not import Ginkgo:

- `tools/custom/export_test.go`, `mcp/tools/export_test.go`, `audit/helpers_test.go`,
  `alignment/helpers_test.go` — pure test helpers (no `func Test...`; they exist only to expose
  internals or share fixtures with the Ginkgo suite files in the same package).
- `mcp/sdk_verify_test.go` — a single `func TestOfficialMCPGoSDKCompiles(t *testing.T)` smoke test
  verifying the third-party `modelcontextprotocol/go-sdk` API surface compiles as expected. This is
  a dependency/toolchain compile-check, not a business-behavior spec, and is in the same spirit as
  (though not formally covered by) the native-Go-fuzz-test exception in
  [AGENTS.md](../../../../AGENTS.md#exception-go-native-fuzz-tests).

---

## Mock-LLM Strategy

Per [AGENTS.md](../../../../AGENTS.md#mock-strategy), external dependencies are mocked — the LLM
provider is KA's primary external dependency. `test/services/mock-llm/` provides a standalone mock
LLM HTTP service used by integration and E2E tests, supporting:

- YAML-driven response overrides (scripted multi-turn conversations)
- Tool-call repetition and forced-text-response scenarios for adversarial/edge-case testing
- Its own metrics and auth-header verification (so tests can assert KA called it correctly)

Business requirements for this shared test infrastructure are tracked separately in
[`docs/services/test-infrastructure/mock-llm/BUSINESS_REQUIREMENTS.md`](../../test-infrastructure/mock-llm/BUSINESS_REQUIREMENTS.md).

Unit tests below the HTTP boundary mock the LLM client interface directly rather than going through
`test/services/mock-llm/`.

---

## Integration Test Infrastructure

Per [AGENTS.md](../../../../AGENTS.md#ci-parallel-safety): integration tests use `envtest` for the
Kubernetes API server and `httptest.NewServer` (`:0` port) for HTTP dependencies, avoiding port
conflicts under parallel CI execution. Each test constructs its own store/manager/handler — no
shared state across tests.

## E2E Test Infrastructure

E2E tests run against a `Kind` cluster and exercise the real KA binary as a subprocess (or
in-cluster Deployment, depending on suite), covering the full submit-poll-result session lifecycle
described in [api-specification.md](./api-specification.md).

---

## Related Documentation

- [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) / [BR_MAPPING.md](./BR_MAPPING.md) — what these tests prove
- [metrics-slos.md](./metrics-slos.md) — metrics validated by the observability E2E suite
- [overview.md](./overview.md) — architecture under test
