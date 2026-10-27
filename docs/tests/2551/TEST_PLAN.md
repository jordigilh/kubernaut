# Test Plan: Legacy Deploy Manifest Audit and Relocation

**Test Plan Identifier**: TP-2551-v1.0
**Feature**: Remove ambiguous application deployment manifests from `deploy/` and relocate active test fixtures to test-owned paths.
**Version**: 1.0
**Created**: 2026-10-27
**Author**: Kubernaut development team
**Status**: Active
**Branch**: `issue-2551-deploy-manifest-audit-v3`

## 1. Purpose and objectives

This plan proves that the repository no longer presents `deploy/` as a supported
application deployment surface after the operator migration, while preserving the
test and integration fixtures that still have active callers.

Objectives:

1. Every retained fixture has a test-owned source path and an active caller.
2. No active script or test references a removed `deploy/` path.
3. Operator-backed FullPipeline, Fleet, and demo paths remain unchanged.
4. Security-sensitive RBAC, TLS, NetworkPolicy, and secret fixtures are not silently
   weakened by relocation.

## 2. Authority and controls

- Issue #2551: Audit and relocate legacy deploy manifests after operator migration.
- Issue #2561 / PR #2562: application Helm chart removal; separate scope.
- `AGENTS.md`: TDD, wiring, coverage, and compliance methodology.
- BR-PLATFORM-014: operator-backed qualification/demo deployment behavior.
- BR-PLATFORM-005: deployment security parity and least-privilege expectations.
- FedRAMP AC-4, AC-6, SC-8, SI-10, AU-2, AU-3, AU-9, AU-11.
- SOC 2 CC7.2 and CC8.1.
- OWASP ASVS access control, communications security, secure configuration, and logging
  objectives.

## 3. Scope

### In scope

- `deploy/` manifest-family ownership classification.
- Relocation of active APIFrontend, integration-dependency, and Mock LLM fixtures.
- Relocation of demo-owned MCP Gateway fixtures and user-owned monitoring inputs.
- Removal of orphaned standalone application manifests and callers.
- Active documentation and script references required for the relocation.
- Static contract tests for fixture ownership and path integrity.

### Out of scope

- `charts/kubernaut/` removal and broad supported-deployment documentation, owned by
  PR #2562.
- Changes to the external `kubernaut-operator` repository.
- New application behavior or audit-event types.
- Rewriting historical test plans and architecture records; those receive a historical
  classification where needed.

## 4. Risks and mitigations

| ID | Risk | Impact | Mitigation | Tests |
|----|------|--------|------------|-------|
| R1 | A live test caller keeps an old path | CI or local E2E failure | Search and path-contract assertions | UT-INFRA-DEPLOY-2551-001, IT-INFRA-DEPLOY-2551-001 |
| R2 | An obsolete standalone manifest is mistaken for a fixture | Unsupported deployment surface remains | Require active caller evidence for every retained file | UT-INFRA-DEPLOY-2551-002 |
| R3 | RBAC/TLS/secret fixture content drifts during move | AC-6, SC-8, or secret-handling regression | Byte/content comparison before and after relocation; render moved Kustomize roots | IT-INFRA-DEPLOY-2551-002 |
| R4 | Operator qualification paths are changed accidentally | FP/Fleet/demo regression | Exclude PR #2562-owned files and run operator qualification smoke checks | IT-INFRA-DEPLOY-2551-003 |

## 5. TDD approach

### RED

- Add Ginkgo/Gomega ownership and path-contract scenarios.
- Assert required test-owned fixture paths exist.
- Assert the application `deploy/` root and obsolete standalone script paths are absent.
- Assert active callers use the relocated paths.

### GREEN

- Move active fixtures.
- Update test and script callers.
- Delete orphaned standalone application manifests/scripts.
- Update active deployment guidance with the operator ownership boundary.

### REFACTOR

- Consolidate duplicate path constants/helpers.
- Mark historical references explicitly instead of rewriting historical records.
- Validate RBAC, TLS, NetworkPolicy, and secret content after relocation.

## 6. Wiring manifest

| Component | Entry point | Wiring location | IT test |
|-----------|-------------|-----------------|---------|
| APIFrontend E2E fixtures | APIFrontend infrastructure setup | `test/infrastructure/apifrontend_e2e.go`, `apifrontend_dex_test.go`, `apifrontend_fleet_test.go` | IT-INFRA-DEPLOY-2551-001 |
| Integration dependency fixtures | External dependency bootstrap | `scripts/bootstrap-external-deps.sh` | IT-INFRA-DEPLOY-2551-002 |
| Mock LLM fixture | Mock LLM/APIFrontend fixture tests | `test/services/mock-llm/` | IT-INFRA-DEPLOY-2551-003 |
| Data Storage/cert-manager fixtures | Data Storage infrastructure tests | `test/infrastructure/datastorage.go` | IT-INFRA-DEPLOY-2551-004 |
| MCP Gateway demo fixtures | Demo-owned fixture directory | `hack/setup-demo-infra/fixtures/mcp-gateway/` | IT-INFRA-DEPLOY-2551-005 |
| Monitoring observability inputs | User-owned config | `config/monitoring/` | IT-INFRA-DEPLOY-2551-006 |

No new production `cmd/` component is introduced. Checkpoint W is satisfied by proving
each relocated fixture has an existing active caller and no deleted family has a caller.

## 7. Requirements-based scenarios

| ID | Business outcome | Tier | Priority |
|----|------------------|------|----------|
| UT-INFRA-DEPLOY-2551-001 | Required fixture paths exist under test ownership and old application deploy root does not | Unit/contract | P0 |
| UT-INFRA-DEPLOY-2551-002 | Every retained fixture family has an active caller; orphaned application families are absent | Unit/contract | P0 |
| IT-INFRA-DEPLOY-2551-001 | APIFrontend E2E tests load the relocated RBAC, DEX, TLS, config, and Mock LLM fixtures | Integration | P0 |
| IT-INFRA-DEPLOY-2551-002 | External dependency bootstrap resolves relocated PostgreSQL, Redis, Prometheus, and Alertmanager fixtures | Integration | P0 |
| IT-INFRA-DEPLOY-2551-003 | Mock LLM fixture tests load the relocated scenario manifest | Integration | P1 |
| IT-INFRA-DEPLOY-2551-004 | Data Storage tests resolve relocated RBAC, certificate, and Kustomize fixtures | Integration | P0 |
| IT-INFRA-DEPLOY-2551-005 | Demo MCP Gateway fixtures remain available outside the application deployment root | Integration | P1 |
| IT-INFRA-DEPLOY-2551-006 | Audit and Gateway monitoring inputs remain available outside the application deployment root | Integration | P0 |
| E2E-INFRA-DEPLOY-2551-001 | Existing operator-backed FullPipeline and Fleet journeys remain unchanged | E2E | P0 |

The in-scope scenario denominator is nine; all nine are critical or directly support a
critical wiring path, so the target is 9/9 passing. No coverage-only scenarios are added.

## 8. Scoped validation

```bash
go test ./test/infrastructure -run TestInfrastructure -ginkgo.focus='DEPLOY-2551' -count=1
go test ./test/services/mock-llm/... -count=1
python3 - <<'PY'
from pathlib import Path
import yaml
for root in (Path("test/fixtures/kubernetes"),):
    for path in root.rglob("*.yaml"):
        list(yaml.safe_load_all(path.read_text()))
PY
kustomize build test/fixtures/kubernetes/mock-llm
go build ./...
go test ./... -run=^$ -timeout=30s
if git grep -n -E 'deploy/(apifrontend|integration|mock-llm|cert-manager|signalprocessing|data-storage|notification|gateway|authwebhook|secrets|monitoring|manifests|microservices|context-api|mcp-gateway)|deploy/kustomization\.yaml|build-and-deploy\.sh' -- .github Makefile scripts test cmd config \
  | grep -v -E 'test/infrastructure/deploy_manifest_audit_test\.go|test/services/mock-llm/README\.md'; then
  exit 1
fi
git diff --check
```

## 9. Acceptance criteria

- No application Kustomize root remains under `deploy/`.
- Active test fixtures are under `test/fixtures/kubernetes/`.
- No active script/test caller references a moved or deleted path.
- PR #2562-owned chart-removal files remain untouched by this issue.
- All locally runnable P0 scenarios pass; the operator-backed FullPipeline/Fleet E2E
  scenario is a CI/live-environment gate and must pass before merge.

## 10. Execution record

- RED: `UT-INFRA-DEPLOY-2551-001/002` and the two initial integration contracts failed
  before relocation, proving the tests detected the old ownership boundary.
- GREEN: all `DEPLOY-2551` scenarios pass; the full 362-spec infrastructure suite passes.
- Relocated YAML parsing: 50 files parsed successfully with PyYAML.
- Mock LLM package tests pass.
- `go build ./...` passes.
- `go test ./... -run=^$ -timeout=30s` passes.
- Changed-code lint passes with `golangci-lint --new-from-rev=origin/pr-2562`.
- Full `make test` remains blocked by nine unrelated pre-existing APIFrontend
  investigation timeout failures; no changed production APIFrontend files are part of
  this issue. Full repository lint also retains one pre-existing `gocyclo` warning in
  `hack/setup-demo-infra/main.go`.
- Live FullPipeline/Fleet E2E requires operator image/chart artifacts and LLM credentials;
  those inputs were not present in this checkout. Their compile-only contracts and the
  full Infrastructure suite pass; CI/live qualification remains the final environment-
  gated check.
