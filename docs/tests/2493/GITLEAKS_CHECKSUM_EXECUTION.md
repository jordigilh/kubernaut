# PR #2493: Verified Archive Checksum False Positive

**Date**: 2026-10-11
**Requirement**: BR-SECURITY-2493 §9
**Status**: Locally verified; included in the user-requested batched publication
**Review**: https://github.com/jordigilh/kubernaut/pull/2493#discussion_r4239946051
**Alert**: https://github.com/jordigilh/kubernaut/security/code-scanning/42

## Assessment and narrowly scoped exception

The `generic-api-key` finding at `docs/tests/2502/TEST_PLAN.md:107–108` extracts
the documented archive SHA-256. Recomputing the retained archive's digest on
Helios08 returned the **same checksum**; its size is **637,565 bytes**. The original
archive verification inspected **237 members** for the actual collector credential.
The document publishes the digest, **not that credential**.

This finding is a **verified non-secret false positive**, not a vulnerability
whose exposure is being accepted. No token rotation or six-month CVE exception
is needed for this checksum. It has no credential authority to revoke.

The new `.gitleaks.toml` allowance targets only **generic-api-key** and requires
**both** the exact document path **and** the exact extracted checksum. Default
rules remain enabled. No entire file, commit, directory or rule is exempted.
Because CI scans Git history, the current configuration also classifies the
original historical finding without rewriting or deleting historical evidence.
The existing Gitleaks workflow and nonzero-findings gate are unchanged.

## TDD and actual-scanner verification

| Scenario / check | Result |
|---|---|
| UT-SEC-2493-010/011 RED | Seed `1791717000`: 4 expected failures because the scoped allowance did not exist |
| IT-SEC-2493-005 RED | CI-pinned Gitleaks 8.30.1 reproduces exactly the generic-api-key finding on original commit `d56cc6c2fae953e1b30bba6780987217db92ab54`, lines 107–108; exit 1 |
| Infrastructure GREEN | Race-enabled seed `1791717100`: 280/280 specs pass, no failed/pending/filtered specs |
| Historical finding GREEN | Same scanner, original commit and updated configuration: exit 0, no findings |
| Known digest / correct document | Actual scanner: exit 0 |
| Different synthetic value / same document | Actual scanner: exit 1, one generic-api-key finding |
| Same documented value / different document | Actual scanner: exit 1, one generic-api-key finding |
| Build | `GOTOOLCHAIN=go1.26.6 go build ./...`: PASS |
| Affected lint | `golangci-lint run --timeout=5m ./test/infrastructure/...`: 0 issues |
| REFACTOR | N/A: exact configuration allowance; no new production behavior |

Controls used owned temporary Git fixtures, the published report and a
runtime-generated deterministic **synthetic non-credential**. Reports are
redacted. No fixture values or temporary repositories are committed to Kubernaut.
The BDD contracts use the already-present TOML dependency, not a new scanner
dependency in unit-test CI. Actual scanner integration is recorded separately.

## Boundaries and evidence

This does not address application Go vulnerabilities, lint debt, hidden Trivy
output, or the two unclosed historical test/setup causes. It does not dismiss
any other secret-scan alert. Remote scan results remain a separate delivery gate.

Owned RCA files under `../kubernaut-pr-2493-rca/`:
- `fleet-external-archive-proof-v10.json`
- `gitleaks-checksum-red-v16.{json,log}`
- `gitleaks-checksum-infrastructure-green-v16.{json,log}`
- `gitleaks-checksum-history-{red,green}-v16.{json,log}`
- `gitleaks-controls-verification-v16.json`
- `gitleaks-control-*-v16.{json,log}`

**Confidence: 99%** that alert #42 identifies only the verified public digest,
based on the independently recomputed archive hash, original scanner reproduction
and both boundary controls. This does not establish full PR merge readiness.
