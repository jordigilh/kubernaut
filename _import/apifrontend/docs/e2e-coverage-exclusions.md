# E2E Coverage Exclusions

This document lists production code paths that are **intentionally excluded** from E2E
coverage measurement. Each exclusion is justified by existing unit/integration test
coverage and the impracticality of exercising these paths in an E2E cluster environment.

## Exclusion Rationale

E2E binary coverage (DD-TEST-007) instruments the compiled binary with
`GOFLAGS=-cover`. However, certain code paths only execute during process startup,
TLS negotiation, or one-time configuration loading — they complete before the E2E
test suite begins interacting with the service. These paths are fully covered by
unit and integration tests.

## Excluded Paths

| Code Path | Reason | Existing Coverage |
|-----------|--------|-------------------|
| `cmd/apifrontend/main.go` — `run()` startup sequence (lines 61-225) | Executes once at boot before E2E begins; config parsing, listener bind, signal handler registration | `cmd/apifrontend/main_wiring_test.go` (UT), IT suite startup validation |
| `cmd/apifrontend/main.go` — TLS server setup (`buildTLSConfig`, cert watcher init) | Requires matching TLS cert/key paths specific to deployment env | `internal/config/tls_test.go` (UT), IT suite with real TLS |
| `internal/config/config.go` — `Load()`, `ResolveDefaults()`, `Validate()` | Config parsing/validation; tested exhaustively with fixture YAML | `internal/config/config_test.go` (UT, 98% coverage) |
| `internal/config/hotreload.go` — `FileWatcher.Start()` fsnotify setup | OS-level inotify/kqueue registration; tested via mock FS events | `internal/config/hotreload_test.go` (UT) |
| `internal/auth/jwt.go` — JWKS discovery + key rotation | Depends on DEX OIDC provider startup timing | `internal/auth/jwt_test.go` (UT), IT suite auth flow |
| `internal/resilience/circuitbreaker.go` — initial CB state transitions | First request triggers state machine; CB tested in isolation | `internal/resilience/circuitbreaker_test.go` (UT) |
| `internal/controller/ttl.go` — reconciler registration + leader election | controller-runtime bootstrap; no user-facing effect until TTL fires | `internal/controller/ttl_test.go` (UT with envtest) |

## Coverage Target

With these exclusions documented, the E2E coverage target is **80%** of
non-excluded production code. The `go tool covdata percent` command applied to
collected profiles should exceed this threshold.

## Cross-References

- DD-TEST-007: Coverage instrumentation pattern
- `test/infrastructure/coverage.go`: `CollectE2EBinaryCoverage` implementation
- `.github/workflows/e2e.yml`: Coverage collection + gate step
- `Dockerfile`: `GOFLAGS=-cover -tags e2e` build branch
