# TDD Execution Prompt Template

Use this prompt when asking the AI to execute a TDD plan for a PR.

---

## The Prompt

> Perform a rigorous due diligence and address all risks, gaps, and any other findings you encounter first, then create the test plan using @docs/testing/TEST_PLAN_TEMPLATE.md and store it in `docs/tests/{issue_number}/`. Tests must provide behavioral assurance against business acceptance criteria covering at least 80% of the testable code per tier and avoiding anti-patterns as defined in @docs/development/business-requirements/TESTING_GUIDELINES.md.
>
> Break down each TDD phase into an individual implementation phase: TDD Red (write failing tests), then TDD Green (implement to pass), then TDD Refactor (improve code quality), each captured in a single phase. During the TDD Refactor phase, validate that the business code is free of the Go mistakes listed in https://github.com/teivah/100-go-mistakes.
>
> Add checkpoints at your discretion. At each checkpoint, before advancing to the next phase, perform an audit covering **all** of the following categories. For each category, cite the specific test(s) that satisfy it or write them if missing:
>
> 1. **Observability wiring**: For every metric/gauge/counter defined in the metrics registry, there must be a test that calls the production code path and asserts the metric value changed. Metrics defined but never incremented is a P1.
>
> 2. **Adversarial inputs**: For every string parameter accepted from outside the package (IDs, names, messages), there must be a test with: empty string, max-length+1, characters outside the valid set (e.g. `"../../etc/passwd"`), and Unicode edge cases. Invalid inputs must be rejected, not silently accepted.
>
> 3. **Resource bounds**: For every map, slice, or cache that grows with usage, there must be a test that runs 50+ create/delete lifecycle cycles and asserts the structure does not grow unboundedly.
>
> 4. **Concurrency**: For every method protected by a mutex or accessed from multiple goroutines, there must be a test with 10+ goroutines calling it concurrently under `-race`. Include at least one test where two competing state transitions race (e.g. cancel vs reconnect).
>
> 5. **Nil/zero edge cases**: For every struct field that can be `nil` or zero-valued, there must be a test exercising that path through every downstream consumer.
>
> 6. **Error-path observability**: For every error return, verify the log line includes enough structured context (resource name, namespace, phase) for an SRE to diagnose without reading source code.
>
> 7. **Cross-phase integration**: When phase N defines a component (e.g. a metric) and phase M defines code that should use it (e.g. a service), the phase M checkpoint must include a test proving they are wired together.
>
> 8. **Spec compliance**: For any protocol (SSE, HTTP, K8s naming), verify all required fields are present per the spec (e.g. `id:`, `retry:` for SSE; RFC 1123 for CRD names).
>
> 9. **API surface hygiene**: No test helpers, internal constants, or debug functions should be exported from production packages. Flag any exported symbol only used in `_test.go` files.
>
> Do not advance past a checkpoint until all 9 categories are satisfied. Escalate if you need input.

---

## Why Each Section Exists

| Section | What it prevents |
|---|---|
| Due diligence first | Executing a plan with incorrect API contracts or missing dependencies |
| Test plan template | Ensures IEEE 829 traceability between requirements and tests |
| 80% coverage + anti-patterns | Prevents hollow tests that pass but prove nothing |
| Red/Green/Refactor separation | Ensures tests fail first (proving they test the right thing) |
| 100-go-mistakes validation | Catches common Go pitfalls during refactor phase |
| 9-category checkpoint audit | Catches the systematic blind spots TDD naturally misses (see below) |

## What the 9 Categories Catch That TDD Alone Misses

| Category | Typical TDD blind spot | Real example from PR4 |
|---|---|---|
| Observability wiring | Metric defined in registry, never incremented in code | `af_sessions_active` gauge never called |
| Adversarial inputs | Tests use well-formed IDs like `"sess-1"` | SessionID used as CRD name without RFC 1123 validation |
| Resource bounds | Tests create 1-6 items, leak invisible at small N | `crdIndex` map grows unboundedly on terminal sessions |
| Concurrency | Sequential tests pass, races lurk | No test for concurrent Create or cancel-vs-reconnect race |
| Nil/zero edge cases | Tests use fully-populated structs | SSE `done` event with nil Content produced empty payload |
| Error-path observability | Tests assert error returns, not log quality | TTL controller errors logged without session context |
| Cross-phase integration | Components tested in isolation per phase | Metrics registry tested separately from service that should emit them |
| Spec compliance | Functional tests pass, spec fields missing | SSE frames missing `retry:` field, CRD names not RFC 1123 validated |
| API surface hygiene | Test helpers work, nobody checks export scope | `CreateRequestWithDefaults` exported from production package |
