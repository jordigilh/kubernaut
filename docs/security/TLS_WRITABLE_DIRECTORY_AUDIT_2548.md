# TLS/SSL Writable-Directory Audit — Issue #2548

**Status**: Complete (upstream scope)
**Business requirement**: BR-SECURITY-2548
**Design decision**: DD-TLS-001 (no-write ambient CA trust injection)
**Predecessor**: Issue #2545, PR #2546
**Operator follow-up**: `jordigilh/kubernaut-operator#571` (generated
production operand Deployments)

## Threat model

A writable temporary directory is not a container-escape primitive, but it
gives a compromised process a convenient location for C2 artifacts, payload
staging, and exfiltration buffers. The audit's objective is to minimize
disk-backed writable locations reachable from TLS/SSL handling code in
long-running production containers.

## Inventory method

Exhaustive searches over the repository (Engram/zvec-grep and direct
inspection) for:

- temp-file/dir creation APIs (`os.CreateTemp`, `os.MkdirTemp`,
  `ioutil.TempFile`, `os.Create`, `os.OpenFile`, `os.WriteFile`,
  `os.Mkdir`, `os.Rename`, `os.Symlink`, `os.Remove`, `os.Truncate`) in
  non-test sources under `pkg/`, `cmd/`, `internal/`;
- ambient trust environment variables (`SSL_CERT_FILE`, `SSL_CERT_DIR`,
  `TLS_CA_FILE`) in Go sources and YAML manifests;
- writable volume types (`emptyDir`, `hostPath`) backing CA/TLS material in
  workload manifests under `test/`, `config/`, `hack/`;
- the CI-rendered production deployment surface
  (`operator-bootstrap-rendered.yaml`, `operator-dependencies-rendered.yaml`
  from `.github/workflows/ci-pipeline.yml`).

## Findings

### Clean (no writable TLS/SSL path)

| Surface | Evidence |
|---|---|
| Ambient CA injection runtime | `pkg/shared/tls/tls.go` `InjectAmbientCACerts` clears `SSL_CERT_FILE`, sets runtime-owned `SSL_CERT_DIR`, sets `TLS_CA_FILE`; zero write APIs in non-test sources (guarded by UT-TLS-2548-004) |
| Ambient env ownership | `SSL_CERT_FILE`/`SSL_CERT_DIR` literals appear only in `pkg/shared/tls/tls.go` (guarded by UT-TLS-2548-005) |
| Combined-CA generation | No residual combined-bundle generation path in production code; the only historical generator was the pre-#2546 `InjectAmbientCACerts` |
| DataStorage signing certificate | `pkg/datastorage/server/server_lifecycle.go` only *reads* cert-manager-provisioned `tls.crt`/`tls.key` |
| Repository workload manifests | No static `SSL_CERT_FILE`/`SSL_CERT_DIR` injection and no writable CA/TLS mount in any scanned workload document (guarded by UT-TLS-2548-001/002) |
| CI-rendered operator bundles | Guarded by the `tls-rendered-contract` CI job running IT-TLS-2548-001 against the operator-qualification artifact |

### Out of scope (reviewed, justified writers — none write certificate material)

| Writer | Location | Rationale |
|---|---|---|
| Notification output dir startup writability probe | `cmd/notification/main.go` | Probes the notification *output* directory (business-required delivery), not CA/TLS material |
| Notification file delivery | `pkg/notification/delivery/file.go` | Notification content delivery, independently justified; not certificate handling |
| Workflow execution pod scratch (`/tmp`) | BR-WE-018 | Independently justified business scratch path |
| Development-only certificate bootstrap | `test/infrastructure/datastorage_bootstrap.go` (`generateBootstrapSigningCert`), `test/fixtures/kubernetes/apifrontend/e2e/generate-certs.sh` | Test/dev-only; never rendered in production workloads |
| E2E fixtures with emptyDir (alertmanager storage, redis data, monitoring data dirs) | `test/e2e/gateway/*.yaml`, `test/manifests/monitoring/*` | Non-CA application data volumes; the segment-based CA heuristic provably does not match them |

### Exclusions (pinned, asserted in the guardrail)

| Path | Pin | Reason |
|---|---|---|
| `test/fixtures/tekton/releases/` | `v0.59.0` (asserted by filename in the exclusion check) | Vendored upstream Tekton release manifest; its `SSL_CERT_FILE`/`SSL_CERT_DIR` point at configmap-mounted read-only paths |
| Heredoc-embedded manifests in `docs/development/e2e-testing/setup-complete-e2e-environment*.sh` | n/a (documented exclusion) | Dev-only environment setup scripts; they render the same dev fixtures and contain no CA/TLS material |

### Documentation-only notes

- `docs/spikes/multi-cluster-mcp-gateway/spike-s3-auth-security/WS6_LIVE_VALIDATION.md`
  describes a combined-CA-bundle workaround applied manually on an OCP
  spike cluster. It is a spike record, not runtime or deployment code.
- The deprecated application Helm chart was fully removed in commit
  `a45436df4`; findings scoped to it are historical (see DD-TLS-001).

### Scan-root notes

`config/` and `hack/` currently contain no workload-kind YAML documents
beyond the scanned fixtures; they remain scan roots prophylactically so a
future addition cannot silently escape the contract.

## Guardrails introduced

| ID | Tier | Proof |
|---|---|---|
| UT-TLS-2548-001 | UT (static manifest) | No static ambient CA env injection in repo workload manifests, including `envFrom` sources carrying ambient env keys |
| UT-TLS-2548-002 | UT (static manifest) | CA/TLS mounts are read-only secret/configMap/projected, never emptyDir/hostPath; minimum-match anchor on the DataStorage `/etc/certs` mount |
| UT-TLS-2548-003 | UT (detector self-test) | Planted violations are caught (static env, writable/emptyDir CA mount, envFrom key injection); `replicas-cache` lookalike and ConfigMap-embedded workload text are not |
| UT-TLS-2548-004 | UT (source guard) | `pkg/shared/tls` non-test sources contain no write/temp APIs (AST-based, comment-immune, import-alias aware) |
| UT-TLS-2548-005 | UT (source guard) | `SSL_CERT_FILE`/`SSL_CERT_DIR` literals confined to `pkg/shared/tls` across `pkg/`, `cmd/`, `internal/` |
| UT-TLS-2548-006 | UT (detector self-test) | Template placeholders in one document never mask a syntax error or a violation in another; CRLF streams split correctly; malformed template documents skip individually |
| UT-TLS-2548-007 | UT (detector self-test) | `envFrom` sources resolve across corpus files and union-merge same-named sources (cross-file and in-stream shadowing both closed) |
| UT-TLS-2548-008 | UT (detector self-test) | `ScanManifestCorpus` resolves `envFrom` across rendered-bundle streams and aborts fail-closed on any malformed stream |
| IT-TLS-2548-001 | IT (CI) | Rendered operator bundles satisfy the same contract (`tls-rendered-contract` job, gated by the `summary` job) |
| IT-TLS-2545-001 | IT | Read-only UBI private+public CA bootstrap (pre-existing from #2545) |
| IT-TLS-2545-002 | IT | `TLS_CA_FILE` hot-reload with last-known-good retention, proven in a read-only Linux container via the `rotatechild` podman child (implemented in this issue; the #2545 test plan had specified it but it was never implemented). A second valid rotation after the invalid one proves the watcher kept processing events (non-vacuous rejection) |

## Detection envelope (accepted risks)

The manifest detector is deliberately fail-closed but heuristic; its known
envelope is:

- **CA-material naming.** Mount paths, subPaths, subPathExprs, volume
  names, and volume source names (secret/configMap/projected) are matched by whole-segment
  or token matching (`ca`, `cert`, `certs`, `pki`, `tls`, `x509`, `.crt`,
  `.pem`, `.key` suffixes). An arbitrarily-named CA volume (e.g. a secret
  named `cache` mounted at `/var/lib/app`) is not detected. Token matching
  accepts compound names like `signing-cert`, `webhook-certs`, and
  `tls-ca` while rejecting lookalikes like `replicas-cache`.
- **envFrom external sources.** `envFrom` references are resolved against
  a corpus-wide index of ConfigMap/Secret documents: across all scanned
  files (`ScanManifestFiles`), across the streams of a rendered
  deployment surface (`ScanManifestCorpus`, used by the operator-bundle
  gate), or within a single stream (`ScanManifestData`). `envFrom.prefix`
  is applied when matching keys. Rendered surfaces never skip template
  documents: a `{{`-bearing parse failure aborts the corpus scan. Same-named sources across files/namespaces are union-merged:
  the error direction is a possible false positive, never a missed
  violation. Sources defined outside the scanned manifests (e.g.
  cluster-user-created objects) cannot be statically verified.
- **PVC/CSI mounts.** Read-only persistentVolumeClaim-backed certificate
  mounts are reported as violations by design (fail-closed): the supported
  contract mounts trust material from secret/configMap/projected sources
  only. A future legitimate PVC mount must be a documented contract
  change, not a quiet weakening of the check.
- **Template documents.** A YAML document whose own bytes fail to parse
  and contain Go-template placeholders (`{{`) is skipped as a
  runtime-substituted template (it cannot be applied by kubectl/helm
  as-is); every other document of the same stream is still decoded and
  scanned. Placeholders in one document never mask a syntax error or a
  violation in another. Genuine YAML syntax errors in non-template
  documents abort the scan with the offending file path (never silently
  skipped, never retried in a loop). CRLF line endings are normalized
  before splitting, and a segment that still contains multiple documents
  after splitting (an unrecognized separator form) aborts the scan
  fail-closed rather than silently decoding only the first.
- **kind: List and non-YAML files.** Top-level `kind: List` wrapper
  documents are unpacked and their `items` scanned like top-level
  documents. `.json` manifest files are not scanned (only `.yaml`/`.yml`
  files); no such documents exist in the scanned corpus today.
- **Init-only certificate-generation exception.** A `kind: Job`
  workload (runs to completion) may mount writable CA/TLS material when
  the volume is a memory-backed emptyDir (`medium: Memory`, bounded
  tmpfs) and its root filesystem is otherwise read-only -- the operator
  chart's `certificate-bootstrap` Job pattern (issue #2548 acceptance
  criterion 3). Disk-backed staging and every long-running kind
  (Deployment/StatefulSet/DaemonSet/ReplicaSet/CronJob/Pod) remain
  violations. Verified in UT-TLS-2548-009.
- **Malformed workloads.** A workload document without a pod
  spec/template is reported as a violation (fail-closed) rather than
  silently skipped.
- **ephemeralContainers** are scanned like init containers.
- **Source guards.** The AST scans exclude `_test.go`, `testdata`, and
  hidden directories; import aliases are resolved through each file's
  import list. Dot-imports of `os` are not resolved (none exist in the
  repository; a dot-import would fail lint anyway).


## Residual risks

- The operator repository's operand Deployment rendering is qualified
  upstream of this repo's rendered-bundle gate only through that gate and
  the operator's own follow-up (`jordigilh/kubernaut-operator#571`).
- The `tls-rendered-contract` job consumes the `build-operator` artifact
  and is wired into the `summary` job's needs; branch protection must
  include `summary` for merge gating.
