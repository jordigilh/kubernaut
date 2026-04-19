# Test Plan: HolmesGPT SDK ConfigMap Split

**Feature**: Split HAPI configuration into service and SDK ConfigMaps with external reference support
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- Issue #390: Expose HolmesGPT SDK toolset configuration in Helm chart
- BR-HAPI-002: Enable Kubernetes toolsets by default
- BR-HAPI-250: Workflow catalog toolset

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 1. Scope

### In Scope

- `kubernaut-agent/src/main.py`: Two-file config loading from well-known SDK path
- `kubernaut-agent/src/models/config_models.py`: `AppConfig` TypedDict update
- `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`: ConfigMap split, volume mounts, conditional SDK template
- `charts/kubernaut/values.yaml` / `values.schema.json`: New fields validation

### Out of Scope

- ConfigManager hot-reload for SDK config (deferred to v1.2, Issue #391)
- E2E Full Pipeline migration to Helm chart (deferred to v1.2)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Well-known fixed path (`/etc/holmesgpt/sdk/sdk-config.yaml`) instead of `sdk_config_ref` | Python code reads files, not K8s API. Mount path is Helm-controlled. Simpler, no indirection. |
| No backward compatibility | Helm upgrade creates both ConfigMaps atomically. SDK config is mandatory. |
| `SDK_CONFIG_FILE` env var for test overrides | Matches existing `CONFIG_FILE` pattern, enables isolated unit testing. |
| Fail-fast on missing file, empty file, or missing `llm` key | HAPI is non-functional without LLM config. Toolsets and MCP servers are optional. |
| LLM config in SDK ConfigMap | Future-proofs for multi-LLM scenarios where different toolsets target different providers. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (config loading logic, merge semantics, validation)
- **Integration**: >=80% of integration-testable code (Helm template rendering, volume mounts)

### 2-Tier Minimum

- **Unit tests** (Python pytest): Config loading correctness, merge behavior, error handling
- **Integration tests** (Helm shell): Template rendering, schema validation, lint compliance

### Business Outcome Quality Bar

Tests validate **business outcomes**: "Does the operator get Prometheus tools available to the LLM when they configure `toolsets.prometheus/metrics`?" — not "does `load_config()` call `yaml.safe_load()`?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `kubernaut-agent/src/main.py` | `load_config()` SDK merge logic | ~30 new lines |
| `kubernaut-agent/src/models/config_models.py` | `AppConfig` TypedDict | ~5 new lines |

### Integration-Testable Code (I/O, rendering, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml` | ConfigMap + Deployment templates | ~50 changed lines |
| `charts/kubernaut/values.yaml` | New fields | ~15 new lines |
| `charts/kubernaut/values.schema.json` | Schema validation | ~20 new lines |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #390-LOAD | SDK config loaded and merged into app_config | P0 | Unit | UT-HAPI-390-001 | Pass |
| #390-MERGE | SDK llm values override main config llm values | P0 | Unit | UT-HAPI-390-002 | Pass |
| #390-TOOLSETS | Toolsets from SDK config available in app_config | P0 | Unit | UT-HAPI-390-003 | Pass |
| #390-MCP | mcp_servers from SDK config available in app_config | P1 | Unit | UT-HAPI-390-004 | Pass |
| #390-MISSING | Missing SDK config file causes fail-fast error | P0 | Unit | UT-HAPI-390-005 | Pass |
| #390-EMPTY | Empty SDK config file causes fail-fast error | P0 | Unit | UT-HAPI-390-006 | Pass |
| #390-NO-LLM | SDK config without `llm` key causes fail-fast error | P0 | Unit | UT-HAPI-390-007 | Pass |
| #390-LLM-ONLY | SDK config with `llm` only (no toolsets) is valid | P0 | Unit | UT-HAPI-390-008 | Pass |
| #390-E2E-SPLIT | E2E infrastructure deploys HAPI with split ConfigMaps | P0 | E2E | ET-HAPI-390-001 | Pass |
| #390-E2E-PROM | FP E2E injects prometheus/metrics toolset into SDK ConfigMap | P1 | E2E | ET-HAPI-390-002 | Pass |
| #390-HELM-SPLIT | Helm renders two separate ConfigMaps | P0 | Integration | IT-HAPI-390-001 | Pending |
| #390-HELM-EXIST | existingSdkConfigMap skips SDK template | P0 | Integration | IT-HAPI-390-002 | Pending |
| #390-HELM-VOL | Deployment has sdk-config volume mount | P0 | Integration | IT-HAPI-390-003 | Pending |
| #390-HELM-LINT | helm lint passes for all modes | P1 | Integration | IT-HAPI-390-004 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-HAPI-390-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: HAPI (HolmesGPT API)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests (Python pytest)

**Testable code scope**: `kubernaut-agent/src/main.py` `load_config()` — targeting >=80% of new SDK merge logic

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-390-001` | Operator gets merged config when both service and SDK config files exist | Pass |
| `UT-HAPI-390-002` | SDK llm settings override main config llm settings (operator controls LLM via SDK ConfigMap) | Pass |
| `UT-HAPI-390-003` | Operator-configured toolsets (e.g. Prometheus) are available to LLM via app_config | Pass |
| `UT-HAPI-390-004` | Operator-configured mcp_servers are available to LLM via app_config | Pass |
| `UT-HAPI-390-005` | Deployment fails fast with clear error when SDK config file missing (no silent degradation) | Pass |
| `UT-HAPI-390-006` | Deployment fails fast with clear error when SDK config file empty (no silent degradation) | Pass |
| `UT-HAPI-390-007` | SDK config without `llm` key causes fail-fast error (HAPI non-functional without LLM) | Pass |
| `UT-HAPI-390-008` | SDK config with `llm` only (no toolsets) is valid — kubernetes/core enabled by code | Pass |

### Tier 2: Integration Tests (Helm template/lint, shell)

**Testable code scope**: `charts/kubernaut/templates/kubernaut-agent/` — targeting >=80% of template logic paths

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-HAPI-390-001` | Helm renders both kubernaut-agent-config and holmesgpt-sdk-config as separate ConfigMaps | Pending |
| `IT-HAPI-390-002` | Setting existingSdkConfigMap causes chart to skip generating holmesgpt-sdk-config | Pending |
| `IT-HAPI-390-003` | Deployment spec includes sdk-config volume and /etc/holmesgpt/sdk mount | Pending |
| `IT-HAPI-390-004` | helm lint passes for default values, with toolsets, and with existingSdkConfigMap | Pending |

### Tier 3: E2E Tests (Go Kind cluster)

**Testable code scope**: `test/infrastructure/holmesgpt_api.go` — E2E validates HAPI boots with split ConfigMaps in a real Kind cluster

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `ET-HAPI-390-001` | HAPI-only E2E boots successfully with split ConfigMaps (service + SDK) | Pass |
| `ET-HAPI-390-002` | FP E2E HAPI receives prometheus/metrics toolset via SDK ConfigMap | Pass |

---

## 6. Test Cases (Detail)

### UT-HAPI-390-001: SDK config loaded and merged

**BR**: #390-LOAD
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_sdk_config_loading.py`

**Given**: Main config at `CONFIG_FILE` with `logging`, `data_storage`, `audit`. SDK config at `SDK_CONFIG_FILE` with `llm`, `toolsets`.
**When**: `load_config()` is called.
**Then**: Returned config contains keys from both files: `logging` from main, `llm` and `toolsets` from SDK.

**Acceptance Criteria**:
- `config["logging"]["level"]` equals main config value
- `config["llm"]["provider"]` equals SDK config value
- `config["toolsets"]` contains SDK-provided toolsets
- Log entry includes `sdk_config_loaded` event

---

### UT-HAPI-390-002: SDK llm overrides main config llm

**BR**: #390-MERGE
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_sdk_config_loading.py`

**Given**: Main config has default `llm.max_retries: 3`. SDK config has `llm.provider: "vertex_ai"`, `llm.model: "claude-sonnet-4"`.
**When**: `load_config()` is called.
**Then**: SDK `llm` values are present; main config defaults for unspecified keys are preserved.

**Acceptance Criteria**:
- `config["llm"]["provider"]` == `"vertex_ai"` (from SDK)
- `config["llm"]["model"]` == `"claude-sonnet-4"` (from SDK)
- `config["llm"]["max_retries"]` == `3` (default, not in SDK)

---

### UT-HAPI-390-003: Toolsets available in app_config

**BR**: #390-TOOLSETS
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_sdk_config_loading.py`

**Given**: SDK config contains `toolsets: {"prometheus/metrics": {"enabled": true, "config": {"prometheus_url": "http://prom:9090"}}}`.
**When**: `load_config()` is called and result passed to `prepare_toolsets_config_for_sdk()`.
**Then**: Prometheus toolset appears in the prepared toolsets dict with the configured URL.

**Acceptance Criteria**:
- `toolsets["prometheus/metrics"]["enabled"]` is `True`
- `toolsets["prometheus/metrics"]["config"]["prometheus_url"]` == `"http://prom:9090"`
- Default kubernetes toolsets are also present (BR-HAPI-002)

---

### UT-HAPI-390-004: MCP servers available in app_config

**BR**: #390-MCP
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_sdk_config_loading.py`

**Given**: SDK config contains `mcp_servers: {"custom": {"url": "http://mcp:8080"}}`.
**When**: `load_config()` is called.
**Then**: `config["mcp_servers"]` contains the custom MCP server.

**Acceptance Criteria**:
- `config["mcp_servers"]["custom"]["url"]` == `"http://mcp:8080"`

---

### UT-HAPI-390-005: Missing SDK config fails fast

**BR**: #390-MISSING
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_sdk_config_loading.py`

**Given**: Main config exists. `SDK_CONFIG_FILE` points to a non-existent path.
**When**: `load_config()` is called.
**Then**: `FileNotFoundError` is raised with a message mentioning the SDK config path.

**Acceptance Criteria**:
- Exception type is `FileNotFoundError`
- Exception message contains the path that was not found
- Exception message mentions "SDK" for operator clarity

---

### UT-HAPI-390-006: Empty SDK config fails fast

**BR**: #390-EMPTY
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_sdk_config_loading.py`

**Given**: Main config exists. SDK config file exists but is empty.
**When**: `load_config()` is called.
**Then**: `ValueError` is raised with a message indicating the SDK config is empty.

**Acceptance Criteria**:
- Exception type is `ValueError`
- Exception message contains "empty"
- Exception message contains the SDK config path

---

### UT-HAPI-390-007: Missing LLM key fails fast

**BR**: #390-NO-LLM
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_sdk_config_loading.py`

**Given**: SDK config file exists with `toolsets` but no `llm` section.
**When**: `merge_sdk_config()` is called.
**Then**: `ValueError` is raised with a message mentioning `llm`.

**Acceptance Criteria**:
- Exception type is `ValueError`
- Exception message contains "llm" for operator clarity
- Exception message contains the SDK config path

---

### UT-HAPI-390-008: LLM only (no toolsets) is valid

**BR**: #390-LLM-ONLY
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_sdk_config_loading.py`

**Given**: SDK config file has only `llm` section (no `toolsets` or `mcp_servers`).
**When**: `merge_sdk_config()` is called.
**Then**: Main config is returned with LLM settings merged; no `toolsets` key present.

**Acceptance Criteria**:
- Config contains `llm` from SDK
- Config does not contain `toolsets` key
- Main config keys (logging, data_storage, audit) are preserved

---

### ET-HAPI-390-001: HAPI-only E2E boots with split ConfigMaps

**BR**: #390-E2E-SPLIT
**Type**: E2E (Kind cluster)
**File**: `test/infrastructure/holmesgpt_api.go`

**Given**: `deployHAPIOnly()` creates both `kubernaut-agent-config` and `holmesgpt-sdk-config` ConfigMaps.
**When**: HAPI pod starts in the Kind cluster.
**Then**: HAPI boots successfully, loads both configs, and passes readiness probe.

**Acceptance Criteria**:
- Pod reaches Ready state
- `/readyz` returns 200
- No crash loop due to missing SDK config

---

### ET-HAPI-390-002: FP Prometheus toolset in SDK ConfigMap

**BR**: #390-E2E-PROM
**Type**: E2E (Kind cluster, Full Pipeline)
**File**: `test/infrastructure/fullpipeline_e2e.go`

**Given**: Full Pipeline deploys HAPI with `HAPIDeployOpts.SdkToolsets` containing `prometheus/metrics` toolset pointing to `http://prometheus-svc:9090`.
**When**: HAPI pod starts and loads the SDK config.
**Then**: HAPI has `prometheus/metrics` toolset available for LLM investigations.

**Acceptance Criteria**:
- SDK ConfigMap contains `prometheus/metrics` toolset configuration
- HAPI boots successfully with the toolset
- Prometheus service URL resolves within the Kind cluster

---

### IT-HAPI-390-001: Helm renders two ConfigMaps

**BR**: #390-HELM-SPLIT
**Type**: Integration (Helm)
**Method**: `helm template` output inspection

**Given**: Default `values.yaml` with `holmesgptApi.llm.provider: "openai"`.
**When**: `helm template` is run.
**Then**: Output contains both `kubernaut-agent-config` and `holmesgpt-sdk-config` ConfigMaps. HAPI ConfigMap has `logging`, `data_storage`, `audit` but NOT `llm`. SDK ConfigMap has `llm` section.

**Acceptance Criteria**:
- Two ConfigMaps with distinct names in rendered output
- `kubernaut-agent-config` does not contain `llm:` key
- `holmesgpt-sdk-config` contains `llm:` with rendered values

---

### IT-HAPI-390-002: existingSdkConfigMap skips SDK template

**BR**: #390-HELM-EXIST
**Type**: Integration (Helm)
**Method**: `helm template` with `--set`

**Given**: `holmesgptApi.existingSdkConfigMap: "my-custom-config"`.
**When**: `helm template --set holmesgptApi.existingSdkConfigMap=my-custom-config` is run.
**Then**: `holmesgpt-sdk-config` ConfigMap is NOT rendered. Deployment volume references `my-custom-config`.

**Acceptance Criteria**:
- No `holmesgpt-sdk-config` ConfigMap in output
- Deployment `.spec.volumes[].configMap.name` includes `my-custom-config`

---

### IT-HAPI-390-003: Deployment includes SDK volume mount

**BR**: #390-HELM-VOL
**Type**: Integration (Helm)
**Method**: `helm template` output inspection

**Given**: Default `values.yaml`.
**When**: `helm template` is run.
**Then**: Deployment has `sdk-config` volume and corresponding mount at `/etc/holmesgpt/sdk`.

**Acceptance Criteria**:
- Volume named `sdk-config` present in `.spec.volumes`
- VolumeMount with `mountPath: /etc/holmesgpt/sdk` present in container spec
- VolumeMount is `readOnly: true`

---

### IT-HAPI-390-004: helm lint passes for all modes

**BR**: #390-HELM-LINT
**Type**: Integration (Helm)
**Method**: `helm lint`

**Given**: Chart with new fields.
**When**: `helm lint` is run with: (a) default values, (b) toolsets enabled, (c) existingSdkConfigMap set.
**Then**: No errors from helm lint in any mode.

**Acceptance Criteria**:
- Exit code 0 for all three lint runs
- No WARNING or ERROR messages related to kubernaut-agent

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Python pytest (HAPI is a Python service)
- **Mocks**: Filesystem only (temp config files via `tempfile`)
- **Location**: `kubernaut-agent/tests/unit/test_sdk_config_loading.py`
- **Env Vars**: `CONFIG_FILE` and `SDK_CONFIG_FILE` pointed to temp files

### Integration Tests

- **Framework**: Shell (helm template + helm lint + grep/assertions)
- **Mocks**: None (Helm template rendering is deterministic)
- **Location**: Validated inline during implementation; no persistent test script needed beyond `helm lint`

---

## 8. Execution

```bash
# Unit tests (Python)
cd kubernaut-agent && python -m pytest tests/unit/test_sdk_config_loading.py -v

# Helm lint (all modes)
helm lint charts/kubernaut/
helm lint charts/kubernaut/ --set holmesgptApi.existingSdkConfigMap=my-custom

# Helm template inspection
helm template test charts/kubernaut/ | grep -A5 "holmesgpt-sdk-config"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
