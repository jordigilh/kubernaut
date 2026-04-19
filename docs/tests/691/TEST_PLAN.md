# Test Plan: Normalize holmesgpt-api to kubernaut-agent

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-691-v1
**Feature**: Rename all `holmesgpt-api` K8s resource names to `kubernaut-agent`
**Version**: 1.0
**Created**: 2026-04-18
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/722-691-outcome-rename`

---

## 1. Introduction

### 1.1 Purpose

Validate that all references to the legacy `holmesgpt-api` naming are replaced
with `kubernaut-agent` in critical paths (config, deploy manifests, Helm templates,
API specs). No backwards compatibility required.

### 1.2 Objectives

1. Zero `holmesgpt-api` references in critical runtime paths (config, deploy, API specs)
2. Dead code (`deployHolmesGPTAPIManifestOnly`) removed
3. Build passes, existing tests unaffected

---

## 2. Scope

### 2.1 Critical Paths (MUST rename)

| File | Category |
|------|----------|
| `config/aianalysis.yaml` | Config |
| `deploy/data-storage/client-rbac.yaml` | RBAC |
| `deploy/data-storage/client-rbac-v2.yaml` | RBAC |
| `deploy/dynamic-toolset-deployment.yaml` | Deploy |
| `deploy/secrets/kustomization.yaml` | Secrets |
| `deploy/secrets/production/kustomization.yaml` | Secrets |
| `internal/kubernautagent/api/openapi.json` | API Spec |
| `docker/kubernautagent.Dockerfile` | Build |
| `test/infrastructure/aianalysis_e2e.go` | Dead code DELETE |

### 2.2 Non-Critical Paths (docs, test plans — rename for consistency)

- `docs/**` — cosmetic, rename for consistency
- `cmd/README.md` — documentation
- Template files in `internal/kubernautagent/prompt/templates/` — prompt context

### 2.3 Features Not Tested

- Runtime behavior (rename is cosmetic, no logic changes)
- RBAC binding correctness (verified by existing E2E tests)

---

## 3. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test |
|----|----------------------------|
| `UT-RENAME-691-001` | No `holmesgpt-api` in config/ directory |
| `UT-RENAME-691-002` | No `holmesgpt-api` in deploy/ directory |
| `UT-RENAME-691-003` | No `holmesgpt-api` in internal/ Go code |
| `UT-RENAME-691-004` | No `holmesgpt-api` in docker/ |
| `UT-RENAME-691-005` | Dead function `deployHolmesGPTAPIManifestOnly` removed |

---

## 4. Approach

- **TDD RED**: Write a test that greps critical paths for `holmesgpt-api` and asserts zero matches
- **TDD GREEN**: Execute the rename, delete dead code
- **TDD REFACTOR**: Add CI lint guard script

---

## 5. Execution

```bash
go test ./test/unit/naming/... -ginkgo.focus="691"
```
