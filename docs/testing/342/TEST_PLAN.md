# Test Plan: Helm Chart Smoke Tests (#342)

**Feature**: Validate all documented Helm chart scenarios end-to-end on Kind and OCP
**Version**: 1.0
**Created**: 2026-03-12
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `fix/v1.0.1-chart-platform-agnostic`

**Authority**:
- BR-PLATFORM-001: Helm chart must deploy on vanilla Kubernetes and OpenShift without modification
- BR-PLATFORM-002: First-time user experience must match documented instructions exactly

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Helm Chart README](../../charts/kubernaut/README.md)
- [GitHub Issue #342](https://github.com/jordigilh/kubernaut/issues/342)
- Related fixes: [#336](https://github.com/jordigilh/kubernaut/issues/336), [#339](https://github.com/jordigilh/kubernaut/issues/339), [PR #341](https://github.com/jordigilh/kubernaut/pull/341)

---

## 1. Scope

### In Scope

- **Chart lifecycle**: Full install/upgrade/uninstall cycle following README commands verbatim
- **TLS modes**: Hook (self-signed) and cert-manager certificate management
- **Platform agnosticism**: Same chart works on Kind (vanilla K8s) and OCP (restricted-v2 SCC)
- **Post-install verification**: Service health checks via port-forward + curl as documented
- **Data retention**: PVC and CRD retention behavior after uninstall
- **Edge cases**: Stuck workflow namespace recovery

### Out of Scope

- **ActionTypes**: User-defined testing artifacts, not chart functionality
- **Monitoring integration**: Requires kube-prometheus-stack; informational docs only
- **Remediation workflow execution**: Requires real alert pipeline and LLM connectivity
- **High Availability configuration**: Replica scaling, PDBs, affinity (configuration guidance only)
- **External database/Redis**: BYO PostgreSQL and Redis integration

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Smoke tier (ST) instead of UT/IT/E2E | These are operational tests against live clusters, not code-level tests |
| TAP output format | Machine-parseable, integrates with CI tooling, human-readable |
| Sequential execution | Scenarios have dependencies (install before verify, verify before upgrade) |
| README commands verbatim | Validates the exact user experience, not a simplified version |

---

## 2. Coverage Policy

### Scenario Coverage

This plan uses **scenario coverage** (16/16 documented scenarios) rather than line coverage, since the test target is Helm chart templates and operational behavior, not Go source code.

All 16 testable scenarios from the chart README are covered. Every scenario is executed on at least one platform, with critical scenarios (install, verify, upgrade, uninstall) executed on both Kind and OCP.

### Tier Rationale

Traditional unit/integration tiers do not apply to Helm chart smoke testing. A single `ST` (Smoke Test) tier is used. Each test validates a complete user-facing operation as documented in the README.

### Business Outcome Quality Bar

Each scenario validates: "Does the user get exactly what the README promises?" Acceptance criteria are pass/fail assertions on exit codes, pod counts, HTTP responses, and resource existence.

---

## 3. Testable Scenario Inventory

### Chart Templates Under Test

| File | What It Covers |
|------|----------------|
| `charts/kubernaut/templates/infrastructure/postgresql.yaml` | PostgreSQL deployment with Tier 2 security contexts |
| `charts/kubernaut/templates/infrastructure/redis.yaml` | Redis deployment with Tier 2 security contexts |
| `charts/kubernaut/templates/datastorage/datastorage.yaml` | DataStorage with wait-for-postgres init container |
| `charts/kubernaut/templates/event-exporter/event-exporter.yaml` | Event exporter with Tier 2 merge pattern |
| `charts/kubernaut/templates/hooks/tls-*.yaml` | TLS certificate generation hooks |
| `charts/kubernaut/templates/hooks/migration-job.yaml` | Database migration post-install hook |
| `charts/kubernaut/templates/NOTES.txt` | Post-install user instructions |
| `charts/kubernaut/values.yaml` | Default values including security contexts |
| `charts/kubernaut/crds/*.yaml` | 9 Custom Resource Definitions |

### Supporting Artifacts

| File | What It Covers |
|------|----------------|
| `charts/kubernaut/README.md` | All user-facing instructions (source of truth for test scenarios) |
| `charts/kubernaut/values-demo.yaml` | Demo/dev overlay values |

---

## 4. Scenario Coverage Matrix

| ID | Scenario | Priority | Platform | README Section | Status |
|----|----------|----------|----------|----------------|--------|
| ST-CHART-PRE-001 | Install CRDs | P0 | Both | Pre-Installation > 1. Install CRDs | Pending |
| ST-CHART-PRE-002 | Create namespace | P0 | Both | Pre-Installation > 2. Create the Namespace | Pending |
| ST-CHART-PRE-003 | Provision secrets | P0 | Both | Pre-Installation > 3. Provision Secrets | Pending |
| ST-CHART-INST-001 | Production install | P0 | Both | Installation > Production | Pending |
| ST-CHART-INST-002 | OCI registry install | P1 | Either | Installation > From OCI Registry | Pending |
| ST-CHART-INST-003 | Dev quick start | P0 | Kind | Installation > Development Quick Start | Pending |
| ST-CHART-VERIFY-001 | 13 pods healthy | P0 | Both | Installation > Post-Install Verification | Pending |
| ST-CHART-VERIFY-002 | HolmesGPT health endpoint | P0 | Both | Installation > Post-Install Verification | Pending |
| ST-CHART-VERIFY-003 | DataStorage health endpoint | P0 | Both | Installation > Post-Install Verification | Pending |
| ST-CHART-TLS-001 | Hook mode certs | P0 | Kind | TLS Certificate Management > Hook Mode | Pending |
| ST-CHART-TLS-002 | cert-manager mode | P0 | OCP | TLS Certificate Management > cert-manager Mode | Pending |
| ST-CHART-TLS-003 | Hook recovery | P1 | Kind | TLS Certificate Management > Hook Mode (recovery) | Pending |
| ST-CHART-UPG-001 | Helm upgrade | P0 | Both | Upgrading | Pending |
| ST-CHART-UNINST-001 | Standard uninstall | P0 | Both | Uninstalling | Pending |
| ST-CHART-UNINST-002 | Full cleanup | P0 | Both | Uninstalling > Full cleanup | Pending |
| ST-CHART-EDGE-001 | Stuck workflow namespace | P1 | Both | Uninstalling (stuck namespace) | Pending |

### Status Legend

- **Pending**: Specification complete, not yet executed
- **Pass**: Executed successfully on target platform(s)
- **Fail**: Executed with failures (details in Section 6)
- **Blocked**: Cannot execute due to infrastructure or dependency issue

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `ST-CHART-{CATEGORY}-{SEQ}`

- **ST**: Smoke Test tier
- **CHART**: Helm chart component
- **CATEGORY**: `PRE`, `INST`, `VERIFY`, `TLS`, `UPG`, `UNINST`, `EDGE`
- **SEQ**: Zero-padded 3-digit (001, 002, ...)

### Tier Skip Rationale

- **Unit (UT)**: Not applicable -- Helm templates are not Go source code; `helm lint` and `helm template` provide static validation
- **Integration (IT)**: Not applicable -- chart behavior requires a live cluster; smoke tests fulfill this role
- **E2E (E2E)**: Smoke tests are effectively E2E tests for the chart lifecycle

---

## 6. Test Cases (Detail)

### ST-CHART-PRE-001: Install CRDs

**Authority**: README.md "Pre-Installation > 1. Install CRDs"
**Platform**: Kind, OCP

**Given**: A Kubernetes cluster with kubectl access
**When**: User runs:
```bash
kubectl apply --server-side --force-conflicts -f charts/kubernaut/crds/
```
**Then**: All 9 CRDs are created successfully

**Acceptance Criteria**:
- Exit code 0
- `kubectl get crds | grep kubernaut.ai` returns 9 CRDs
- CRD names: `actiontypes`, `remediationworkflows`, `remediationrequests`, `aianalyses`, `workflowexecutions`, `effectivenessassessments`, `notificationrequests`, `remediationapprovalrequests`, `signalprocessings`

---

### ST-CHART-PRE-002: Create Namespace

**Authority**: README.md "Pre-Installation > 2. Create the Namespace"
**Platform**: Kind, OCP

**Given**: CRDs installed (ST-CHART-PRE-001)
**When**: User runs:
```bash
kubectl create namespace kubernaut-system
```
**Then**: Namespace exists

**Acceptance Criteria**:
- Exit code 0
- `kubectl get ns kubernaut-system` succeeds

---

### ST-CHART-PRE-003: Provision Secrets

**Authority**: README.md "Pre-Installation > 3. Provision Secrets"
**Platform**: Kind, OCP

**Given**: Namespace exists (ST-CHART-PRE-002)
**When**: User runs the 4 required secret creation commands from README:
```bash
kubectl create secret generic kubernaut-pg-credentials \
  --from-literal=POSTGRES_USER=slm_user \
  --from-literal=POSTGRES_PASSWORD=<password> \
  --from-literal=POSTGRES_DB=action_history \
  -n kubernaut-system

kubectl create secret generic kubernaut-ds-credentials \
  --from-literal=db-secrets.yaml=$'username: slm_user\npassword: <password>' \
  -n kubernaut-system

kubectl create secret generic kubernaut-redis-credentials \
  --from-literal=redis-secrets.yaml=$'password: <password>' \
  -n kubernaut-system

kubectl create secret generic kubernaut-llm-credentials \
  --from-literal=OPENAI_API_KEY=sk-... \
  -n kubernaut-system
```
**Then**: All 4 secrets exist with correct keys

**Acceptance Criteria**:
- Each `kubectl create secret` returns exit code 0
- `kubectl get secret kubernaut-pg-credentials -n kubernaut-system` exists
- `kubectl get secret kubernaut-ds-credentials -n kubernaut-system` exists
- `kubectl get secret kubernaut-redis-credentials -n kubernaut-system` exists
- `kubectl get secret kubernaut-llm-credentials -n kubernaut-system` exists

---

### ST-CHART-INST-001: Production Install

**Authority**: README.md "Installation > Production"
**Platform**: Kind (hook TLS), OCP (cert-manager TLS)

**Given**: CRDs applied, namespace exists, 4 secrets provisioned (PRE-001 through PRE-003)
**When**: User runs the exact helm install command from README:
```bash
helm install kubernaut charts/kubernaut/ \
  --namespace kubernaut-system \
  --set postgresql.auth.existingSecret=kubernaut-pg-credentials \
  --set datastorage.dbExistingSecret=kubernaut-ds-credentials \
  --set redis.existingSecret=kubernaut-redis-credentials \
  --set holmesgptApi.llm.provider=openai \
  --set holmesgptApi.llm.model=gpt-4o \
  --set holmesgptApi.llm.credentialsSecretName=kubernaut-llm-credentials \
  --set gateway.auth.signalSources[0].name=alertmanager \
  --set gateway.auth.signalSources[0].serviceAccount=alertmanager-kube-prometheus-stack-alertmanager \
  --set gateway.auth.signalSources[0].namespace=monitoring
```
With additional platform-specific flags:
- Kind: `--set tls.mode=hook --set effectivenessmonitor.external.prometheusEnabled=false --set effectivenessmonitor.external.alertManagerEnabled=false`
- OCP: `--set tls.mode=cert-manager --set tls.certManager.issuerRef.name=selfsigned-issuer --set effectivenessmonitor.external.prometheusEnabled=false --set effectivenessmonitor.external.alertManagerEnabled=false`

**Then**: Helm reports "deployed", NOTES.txt displays correctly

**Acceptance Criteria**:
- Exit code 0
- `helm status kubernaut -n kubernaut-system` shows `STATUS: deployed`
- NOTES.txt output includes post-install verification instructions
- NOTES.txt output shows correct TLS mode for the platform

---

### ST-CHART-INST-002: OCI Registry Install

**Authority**: README.md "Installation > From OCI Registry"
**Platform**: Either (run once)

**Given**: CRDs applied, namespace exists, secrets provisioned, chart published to OCI registry
**When**: User runs:
```bash
helm install kubernaut oci://ghcr.io/jordigilh/kubernaut/charts/kubernaut \
  --version 1.0.0 \
  --namespace kubernaut-system \
  -f my-values.yaml
```
**Then**: Helm pulls the chart from OCI and installs successfully

**Acceptance Criteria**:
- Exit code 0
- `helm status kubernaut -n kubernaut-system` shows `STATUS: deployed`
- Chart version matches `--version` flag

**Notes**: Requires chart to be published to the OCI registry. For pre-release testing, use `helm push` to publish an RC version first.

---

### ST-CHART-INST-003: Dev Quick Start

**Authority**: README.md "Installation > Development Quick Start"
**Platform**: Kind

**Given**: A clean cluster with no pre-created namespace or secrets
**When**: User runs the exact command from README:
```bash
helm install kubernaut charts/kubernaut/ \
  --namespace kubernaut-system --create-namespace \
  --set postgresql.auth.password=devpass \
  --set redis.password=redispass \
  --set effectivenessmonitor.external.prometheusEnabled=false \
  --set effectivenessmonitor.external.alertManagerEnabled=false
```
**Then**: Chart creates namespace, generates secrets internally, and deploys

**Acceptance Criteria**:
- Exit code 0
- Namespace `kubernaut-system` created automatically
- `helm status kubernaut -n kubernaut-system` shows `STATUS: deployed`
- No pre-created secrets required

---

### ST-CHART-VERIFY-001: 13 Pods Healthy

**Authority**: README.md "Installation > Post-Install Verification"
**Platform**: Kind, OCP

**Given**: Successful helm install (INST-001 or INST-003)
**When**: User runs:
```bash
kubectl get pods -n kubernaut-system
```
**Then**: All 13 pods are 1/1 Running

**Acceptance Criteria**:
- 13 pods in `Running` state with `1/1` ready
- Pod names include: `gateway`, `datastorage`, `aianalysis-controller`, `authwebhook`, `notification-controller`, `remediationorchestrator-controller`, `signalprocessing-controller`, `workflowexecution-controller`, `effectivenessmonitor-controller`, `holmesgpt-api`, `event-exporter`, `postgresql`, `redis`
- No pods in `CrashLoopBackOff`, `Error`, `CreateContainerConfigError`, or `Pending`
- Timeout: 5 minutes from install completion

---

### ST-CHART-VERIFY-002: HolmesGPT API Health Endpoint

**Authority**: README.md "Installation > Post-Install Verification"
**Platform**: Kind, OCP

**Given**: All pods running (VERIFY-001)
**When**: User runs:
```bash
kubectl port-forward -n kubernaut-system svc/holmesgpt-api 8080:8080
curl -s http://localhost:8080/health | jq '.'
```
**Then**: Health endpoint responds with valid JSON

**Acceptance Criteria**:
- Port-forward establishes successfully
- `curl` returns HTTP 200
- Response is valid JSON
- Port-forward process is cleaned up after test

---

### ST-CHART-VERIFY-003: DataStorage Health Endpoint

**Authority**: README.md "Installation > Post-Install Verification"
**Platform**: Kind, OCP

**Given**: All pods running (VERIFY-001)
**When**: User runs:
```bash
kubectl port-forward -n kubernaut-system svc/data-storage-service 8081:8080
curl -s http://localhost:8081/health | jq '.'
```
**Then**: Health endpoint responds with valid JSON

**Acceptance Criteria**:
- Port-forward establishes successfully
- `curl` returns HTTP 200
- Response is valid JSON
- Port-forward process is cleaned up after test

**Note**: DataStorage API endpoints under `/api/v1/` require a Kubernetes ServiceAccount bearer token (DD-AUTH-014). The `/health` endpoint is unauthenticated and suitable for post-install verification.

---

### ST-CHART-TLS-001: Hook Mode Certificates

**Authority**: README.md "TLS Certificate Management > Hook Mode"
**Platform**: Kind

**Given**: Successful install with `tls.mode=hook` (INST-001 on Kind)
**When**: User inspects TLS resources:
```bash
kubectl get secret authwebhook-tls -n kubernaut-system
kubectl get configmap authwebhook-ca -n kubernaut-system
kubectl get mutatingwebhookconfigurations authwebhook-mutating -o jsonpath='{.webhooks[0].clientConfig.caBundle}'
```
**Then**: TLS secret, CA configmap, and webhook caBundle all exist

**Acceptance Criteria**:
- `authwebhook-tls` Secret exists with `tls.crt` and `tls.key` keys
- `authwebhook-ca` ConfigMap exists with `ca.crt` key
- `caBundle` on mutating and validating webhook configurations is non-empty

---

### ST-CHART-TLS-002: cert-manager Mode

**Authority**: README.md "TLS Certificate Management > cert-manager Mode"
**Platform**: OCP

**Given**: cert-manager installed, `selfsigned-issuer` ClusterIssuer created
**When**: User installs with:
```bash
helm install kubernaut charts/kubernaut/ \
  --namespace kubernaut-system \
  --set tls.mode=cert-manager \
  --set tls.certManager.issuerRef.name=selfsigned-issuer \
  ...
```
**Then**: cert-manager provisions the TLS certificate

**Acceptance Criteria**:
- `Certificate` resource `authwebhook-cert` exists and is `Ready=True`
- `authwebhook-tls` Secret provisioned by cert-manager
- Webhook configurations have `cert-manager.io/inject-ca-from` annotation
- `caBundle` injected by cert-manager's cainjector

---

### ST-CHART-TLS-003: Hook Recovery

**Authority**: README.md "TLS Certificate Management > Hook Mode" (recovery section)
**Platform**: Kind

**Given**: Successful install with `tls.mode=hook`
**When**: User simulates accidental deletion and recovery:
```bash
kubectl delete secret authwebhook-tls -n kubernaut-system
helm upgrade kubernaut charts/kubernaut/ -n kubernaut-system ...
```
**Then**: Secret is regenerated by the pre-upgrade hook

**Acceptance Criteria**:
- `kubectl delete secret` succeeds
- `helm upgrade` succeeds (exit code 0)
- `authwebhook-tls` Secret exists again with fresh `tls.crt` and `tls.key`
- Webhook `caBundle` is updated to match new CA

---

### ST-CHART-UPG-001: Helm Upgrade

**Authority**: README.md "Upgrading"
**Platform**: Kind, OCP

**Given**: Successful install at revision 1 (INST-001)
**When**: User runs the upgrade sequence from README:
```bash
kubectl apply --server-side --force-conflicts -f charts/kubernaut/crds/
helm upgrade kubernaut charts/kubernaut/ \
  -n kubernaut-system ...
```
**Then**: Release upgrades to revision 2, all pods healthy

**Acceptance Criteria**:
- CRD apply returns exit code 0
- `helm upgrade` returns exit code 0
- `helm status kubernaut -n kubernaut-system` shows `REVISION: 2`
- All 13 pods reach 1/1 Running within 5 minutes
- Database migration hook runs successfully

**Note**: The README upgrade command references `kubernaut/kubernaut` (OCI repo style). The smoke test script uses the local chart path (`charts/kubernaut/`) to test the working-tree version. Both are valid; the local path is intentional for pre-release validation.

---

### ST-CHART-UNINST-001: Standard Uninstall

**Authority**: README.md "Uninstalling"
**Platform**: Kind, OCP

**Given**: Successful install (any revision)
**When**: User runs:
```bash
helm uninstall kubernaut -n kubernaut-system
```
**Then**: Release removed, PVCs and CRDs retained

**Acceptance Criteria**:
- Exit code 0
- `helm status kubernaut -n kubernaut-system` returns "not found"
- `kubectl get pvc postgresql-data -n kubernaut-system` exists (retained)
- `kubectl get pvc redis-data -n kubernaut-system` exists (retained)
- `kubectl get crds | grep kubernaut.ai` returns 9 CRDs (retained)
- All pods terminated within 60 seconds
- TLS hook cleanup: `authwebhook-tls` Secret deleted (hook mode), or cert-manager manages lifecycle (cert-manager mode)

---

### ST-CHART-UNINST-002: Full Cleanup

**Authority**: README.md "Uninstalling > Full cleanup"
**Platform**: Kind, OCP

**Given**: Standard uninstall completed (UNINST-001)
**When**: User runs the full cleanup commands from README:
```bash
kubectl delete pvc postgresql-data redis-data -n kubernaut-system
kubectl delete -f charts/kubernaut/crds/
kubectl delete namespace kubernaut-system
```
**Then**: All resources removed, cluster is clean

**Acceptance Criteria**:
- Each command returns exit code 0
- `kubectl get pvc -n kubernaut-system` returns no resources
- `kubectl get crds | grep kubernaut.ai` returns no results
- `kubectl get ns kubernaut-system` returns not found

---

### ST-CHART-EDGE-001: Stuck Workflow Namespace

**Authority**: README.md "Uninstalling" (stuck namespace section)
**Platform**: Kind, OCP

**Given**: Helm install has created the `kubernaut-workflows` namespace
**When**: User checks and cleans the namespace:
```bash
kubectl get all -n kubernaut-workflows
kubectl delete jobs --all -n kubernaut-workflows
```
**Then**: Namespace can be cleaned and eventually terminates

**Acceptance Criteria**:
- `kubectl get all` succeeds (even if empty)
- `kubectl delete jobs --all` succeeds (even if no jobs exist)
- Namespace is not stuck in `Terminating` state after uninstall

---

## 7. Test Infrastructure

### Kind (arm64/amd64)

- **Cluster**: Kind v0.27+ with Kubernetes 1.34+
- **Images**: Locally built or pulled from `quay.io/kubernaut-ai/*:{tag}-arm64` (local) or `*:{tag}-amd64` (CI)
- **Image loading**: `podman save | kind load image-archive` or `docker save | kind load image-archive`
- **TLS mode**: `hook` (default)
- **Additional flags**: `--set global.image.pullPolicy=IfNotPresent`, `--set effectivenessmonitor.external.prometheusEnabled=false`, `--set effectivenessmonitor.external.alertManagerEnabled=false`

### OCP (amd64)

- **Cluster**: OpenShift 4.14+ with restricted-v2 SCC
- **Images**: Pulled from `quay.io/kubernaut-ai/*:{tag}-amd64`
- **TLS mode**: `cert-manager` with `selfsigned-issuer` ClusterIssuer
- **Prerequisites**: cert-manager operator installed
- **Additional flags**: `--set effectivenessmonitor.external.prometheusEnabled=false`, `--set effectivenessmonitor.external.alertManagerEnabled=false`

### Automation

- **Script**: `scripts/helm-smoke-test.sh`
- **Output**: TAP (Test Anything Protocol) v13
- **CI**: `.github/workflows/helm-smoke-test.yml` (Kind only; OCP manual)

---

## 8. Execution

### Local (Kind)

```bash
scripts/helm-smoke-test.sh \
  --platform kind \
  --image-tag 1.0.1-rc1-arm64 \
  --chart-path charts/kubernaut/
```

### Local (OCP)

```bash
scripts/helm-smoke-test.sh \
  --platform ocp \
  --image-tag 1.0.1-rc1-amd64 \
  --chart-path charts/kubernaut/
```

### CI (GitHub Actions)

Triggered automatically on PRs modifying `charts/`, `docker/`, or `scripts/helm-smoke-test.sh` paths, and on release tags.

---

## 9. Execution Order

Scenarios must be executed in dependency order. The script runs two "flows":

### Flow A: Production Install Lifecycle (both platforms)

```
PRE-001 → PRE-002 → PRE-003 → INST-001 → VERIFY-001 → VERIFY-002 → VERIFY-003
→ TLS-001 (Kind) / TLS-002 (OCP) → UPG-001 → TLS-003 (Kind only)
→ EDGE-001 → UNINST-001 → UNINST-002
```

### Flow B: Dev Quick Start Lifecycle (Kind only)

```
INST-003 → VERIFY-001 → EDGE-001 → UNINST-001 → UNINST-002
```

### Flow C: OCI Registry Install (either platform, run once)

```
PRE-001 → PRE-002 → PRE-003 → INST-002 → VERIFY-001 → UNINST-001 → UNINST-002
```

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-12 | Initial test plan with 16 scenarios |
