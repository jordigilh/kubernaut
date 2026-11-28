# Test Plan: Issue #2548 — TLS/SSL Writable-Directory Audit and Deployment-Contract Guardrails

**Test Plan Identifier**: TP-2548-v1.0
**Feature**: Upstream TLS/SSL writable-directory audit guardrails (BR-SECURITY-2548)
**Version**: 1.0
**Created**: 2026-11-27
**Author**: Kubernaut development team
**Status**: Active
**Branch**: `security/2548-tls-writable-dirs`
**Controls**: FedRAMP SC-8/SC-12/SC-13/AC-6/CM-6/CM-7/SI-10; SOC 2 CC7.2/CC8.1; OWASP ASVS 5.0.0 V12.3.1–V12.3.4, V13.1.3, V15.2.5, V16.5.3 (same baseline as BR-SECURITY-2548)

## 1. Scope and objectives

- Mechanically guard the repository manifest corpus against static ambient
  CA env injection and writable CA/TLS mounts.
- Guard the CI-rendered production deployment surface (operator chart
  bundles) with the same YAML-aware contract.
- Guard source-level invariants: no-write `pkg/shared/tls`, ambient env
  ownership confined to that package.
- Implement the previously-specified but missing IT-TLS-2545-002 (CA
  rotation with last-known-good) as a round-trip HTTPS proof.
- No runtime code changes; the change is tests, CI wiring, and
  documentation.

### In scope

- `test/shared/tlscontract` (detector support package)
- `test/infrastructure/tls_ambient_contract_2548_test.go`
- `test/integration/tls2548rendered` (CI-only rendered-contract suite)
- `test/integration/shared/tls` (IT-TLS-2545-002 + `rotatechild`)
- `.github/workflows/ci-pipeline.yml` (shared integration matrix entry,
  `tls-rendered-contract` job, `summary` needs)

### Out of scope

- Operator operand Deployment rendering (tracked in
  `jordigilh/kubernaut-operator#571`)
- Dev-only certificate bootstrap and E2E fixtures (enumerated in
  `docs/security/TLS_WRITABLE_DIRECTORY_AUDIT_2548.md`)

## 2. Requirements-based scenario matrix

| ID | Tier | Business behavior | BR/control mapping |
|---|---|---|---|
| UT-TLS-2548-001 | UT | Repo workload manifests never statically inject `SSL_CERT_FILE`/`SSL_CERT_DIR` | BR-SECURITY-2548; AC-6/CM-7, SI-10; CC8.1; V12.3.4, V16.5.3 |
| UT-TLS-2548-002 | UT | CA/TLS material mounts are read-only secret/configMap/projected, never emptyDir/hostPath (documented exception: init-only memory-backed Job staging with read-only root, UT-TLS-2548-009) | BR-SECURITY-2548; SC-8/SC-13, AC-6; CC8.1; V12.3.1–V12.3.4, V15.2.5 |
| UT-TLS-2548-003 | UT | Detector catches planted violations; ignores `replicas-cache` lookalikes and ConfigMap-embedded workload text | BR-SECURITY-2548; SI-10; CC8.1; V16.5.3 |
| UT-TLS-2548-004 | UT | `pkg/shared/tls` non-test sources contain no write/temp-file APIs | BR-SECURITY-2548/BR-SECURITY-2545; AC-6/CM-7; CC8.1; V15.2.5 |
| UT-TLS-2548-005 | UT | Ambient env ownership stays in `pkg/shared/tls` across `pkg/`, `cmd/`, `internal/` | BR-SECURITY-2548; AC-6/CM-7, SI-10; CC8.1; V12.3.4 |
| UT-TLS-2548-006 | UT | Go-template placeholders in one document never mask a YAML syntax error or a violation in another document of the same stream (per-document fail-closed decode) | BR-SECURITY-2548; SI-10; CC8.1; V16.5.3 |
| UT-TLS-2548-007 | UT | `envFrom` ambient env sources resolve across files of the scanned corpus, not just within one stream | BR-SECURITY-2548; SI-10; CC8.1; V12.3.4 |
| UT-TLS-2548-008 | UT | Rendered-corpus entry point resolves `envFrom` across streams and fails closed on any malformed stream | BR-SECURITY-2548; SI-10; CC8.1; V12.3.4 |
| UT-TLS-2548-009 | UT | Init-only Job cert staging allows memory-backed emptyDir only; disk-backed staging and long-running kinds stay violations | BR-SECURITY-2548; AC-6/CM-7, SC-13; CC8.1; V15.2.5 |
| IT-TLS-2548-001 | IT | CI-rendered operator bundles satisfy the ambient CA deployment contract (fail-closed on missing input) | BR-SECURITY-2548; SC-8/SC-13, AC-6/CM-7; CC8.1; V12.3.1–V12.3.4, V15.2.5 |
| IT-TLS-2545-001 | IT | Read-only UBI ambient bootstrap with private+public CA trust (pre-existing) | BR-SECURITY-2545; SC-8/AC-6/CM-7; CC7.2; V12.3.1–V12.3.4/V15.2.5 |
| IT-TLS-2545-002 | IT | `TLS_CA_FILE` hot-reload on valid rotation; last-known-good retained after invalid rotation, proven by HTTPS round trips in a read-only Linux container | BR-SECURITY-2545/2548; SC-12/SC-13/CM-6; CC7.2; V12.3.4, V13.1.3 |

Target: 9/9 in-scope UT scenarios and 100% of critical security scenarios
(all are critical), 3/3 in-repository IT rows.

## 3. Pyramid invariant

- **UT** proves the manifest/source contract logic (detector) and its own
  non-vacuity (planted-violation self-test with a negative control).
- **IT** proves wiring: the rendered production bundles in CI, and the
  real TLS rotation journey through `StartCAFileWatcher` -> fsnotify ->
  `CAReloader` -> live HTTPS request inside a read-only Linux container
  (`rotatechild` under podman; macOS kqueue directory watches cannot
  attribute events to file names, so the proof must run on Linux
  fsnotify).
- **E2E / CI**: the `tls-rendered-contract` job runs both the
  repository-corpus UT specs (`-ginkgo.focus='TLS-2548'`) and the
  rendered-bundle IT suite, so neither guardrail can be silently
  unwired; the `shared` integration matrix entry and the operator
  repository own the remaining deployment journeys.

## 4. Vacuity controls

- UT-TLS-2548-001 asserts a minimum workload-document count over the
  corpus.
- UT-TLS-2548-002 asserts a minimum CA-material mount match count (the
  DataStorage `/etc/certs` readOnly secret mount is the positive anchor).
- IT-TLS-2548-001 asserts the rendered bundles contain workload documents
  and fails closed when `TLS_2548_RENDERED_DIR` is unset (no Skip).
- The Tekton exclusion is pinned by version assertion, not silent root
  omission.

## 5. Execution

```bash
go test ./test/infrastructure -ginkgo.focus='TLS-2548' -count=1
make test-integration-shared
# CI-only (requires the operator-qualification artifact):
TLS_2548_RENDERED_DIR=/path/to/artifact go test ./test/integration/tls2548rendered -count=1
```

Required final checks: `go build ./...`, `golangci-lint run --timeout=5m`,
`make lint-test-patterns`, affected test suites.

## 6. Wiring manifest

N/A — test-only change (no new production components; the detector is a
test-owned support package under `test/shared/tlscontract`, never linked
into production binaries).
