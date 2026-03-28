# Changelog

All notable changes to Kubernaut will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2026-03-28

### Added

#### AI Analysis (HAPI)

- **Three-phase RCA architecture** (#529) -- Rewrote the HolmesGPT integration with a three-phase protocol: Investigation, Enrichment, and Workflow Selection. Each phase runs as a separate LLM conversation with dedicated tools and prompts, replacing the single-shot approach from v1.0.
- **Anti-confirmation-bias guardrails** (#462) -- Investigation prompt now includes explicit instructions to consider alternative root causes and avoid anchoring on the first hypothesis.
- **Enrichment audit events** (#533) -- Phase 2 enrichment events (`aiagent.enrichment.*`) provide SOC2 chain-of-custody for the label detection and workflow discovery steps.
- **Tool rename and conditional injection** (#524) -- Renamed `get_resource_context` to scoped tools (`get_namespaced_resource_context`, `get_cluster_resource_context`) with conditional injection based on the target resource type.
- **Target resource mismatch detection** (#496) -- Detects when the LLM-identified remediation target differs from the signal source and resolves to the Kubernetes root owner.

#### Workflow Execution

- **AWX credential injection** (#500) -- Injects the WE controller ServiceAccount token into AWX/AAP jobs, giving Ansible playbooks authenticated access to the Kubernetes API without hardcoded credentials.
- **Runtime engine resolution** (#518) -- The `executionEngine` field moved from WFE spec to status; the WE controller now resolves the engine at runtime from the DataStorage workflow catalog.
- **Ansible playbook RBAC** (#551) -- Added missing RBAC rules for Ansible-executed playbooks to the `kubernaut-workflow-runner` ClusterRole.
- **Custom K8s credential type** (#552) -- Fixed `kubeconfig-file` injection in AWX/AAP execution environments with Jinja2-templated credential types and structured Go data models.

#### Effectiveness Monitor

- **TLS CA support** (#452) -- External HTTP clients (Prometheus, AlertManager) now accept custom TLS CA bundles for environments using internal certificate authorities.
- **Buffered audit ingestion** (ADR-038) -- Audit events are buffered and batch-flushed to DataStorage, reducing per-event HTTP overhead.

#### Notification

- **Workflow name enrichment** (#553) -- Notification bodies now display human-readable workflow names instead of raw UUIDs. The Notification controller resolves names from the DataStorage workflow catalog via a new `WorkflowNameResolver` interface.

#### Helm Chart

- **OCP support** (#348, #452) -- OpenShift Container Platform deployment with RHEL10 base images, service-serving CA injection, cluster-monitoring-view RBAC, and `values-ocp.yaml` overlay.
- **Pre-upgrade CRD hook** (#521) -- Helm pre-upgrade hook job applies CRD schema updates during `helm upgrade`, ensuring new fields are available before controllers restart.
- **Externalized Rego policies** (#404) -- AI Analysis approval policies and Signal Processing classification policies are loaded from user-provided ConfigMaps.
- **Externalized notification routing** (#405) -- Notification routing configuration loaded from user-provided ConfigMap.
- **Helm smoke tests** (#342) -- 14 IEEE 829-compliant smoke tests validate ConfigMap split, Prometheus integration, and SDK tiers.
- **OCI chart distribution** (#403) -- Chart published to `oci://quay.io/kubernaut-ai/kubernaut` with embedded defaults for demo-ready deployment.
- **Istio RBAC rules** (#373) -- ClusterRole rules for HolmesGPT, WorkflowExecution, Remediation Orchestrator, and Effectiveness Monitor in Istio-enabled clusters.
- **Vertex AI support** -- Helm chart supports Google Vertex AI as an LLM provider with project/region environment variables and GCP credential mounting.

#### Remediation Orchestrator

- **Forward hash chain detection** (#525) -- Detects escalation regression by comparing pre-remediation spec hashes across consecutive remediations.
- **NoActionRequiredDelayHours** (#353) -- Configurable cooldown period before re-processing signals that previously resulted in no action.
- **RBAC for cert-manager CRDs** (#545) -- RO and EM ClusterRoles now include read access for cert-manager Certificate resources used as remediation targets.

#### DataStorage

- **Query metrics and DLQ observability** -- Prometheus metrics for query latency, dead-letter queue depth, and audit event ingestion rates.
- **RFC 7807 error details** (#446) -- `DSClientAdapter` surfaces structured error details from DataStorage API responses.

#### Signal Processing

- **Consolidated Rego policies** (#414, #415) -- Five separate Rego policy files consolidated into a single unified policy file for simpler configuration and reasoning.

#### Infrastructure

- **Automated CRD API reference** -- Auto-generated CRD documentation covering 9 CRDs and 80 types using `crd-ref-docs`.
- **Pre-commit anti-pattern detection** -- CRD docs drift detection and test anti-pattern checks in pre-commit hooks.

### Fixed

#### AI Analysis (HAPI)

- **Scope enforcement** (#513) -- Deny-by-default when scopeChecker is nil; fail-closed on `IsManaged` errors.
- **Remediation history wiring** (#540) -- `query_remediation_history` correctly wired into EnrichmentService with spec hash context.
- **Root owner resolution** (#535) -- `list_available_actions` now uses the resolved root owner kind (e.g., Deployment) instead of the symptom resource kind (e.g., Pod).
- **Structured output retry** (#372) -- Returns failed `ValidationResult` on LLM format failures instead of silently proceeding.
- **LLM credential validation** (#487) -- Fail-fast at startup when LLM credentials are missing instead of failing on first request.
- **AffectedResource → RemediationTarget rename** (#542) -- LLM schema, prompts, mock LLM, and all Go types updated consistently.

#### Workflow Execution

- **AWX credential merge** (#365) -- Template credentials correctly merged with ephemeral credentials on AWX job launch.
- **Pre-execution Job cleanup** (#374, #375) -- Completed Jobs from previous runs are cleaned up before new execution, with ownership checks to prevent cross-WFE Job deletion (#383).
- **Explicit engine dispatch** -- Removed silent Tekton fallback; engine dispatch is now explicit in `reconcilePending`.

#### Effectiveness Monitor

- **DataStorage JSON deserialization** (#575) -- EM was decoding the DS audit events response as a bare array, but the API returns a paginated envelope `{"data": [...]}`. Fixed with typed structs matching the OpenAPI contract.
- **Audit event type mismatch** (#579) -- `HasWorkflowStarted` queried for `workflowexecution.workflow.started`, but the WE controller emits `workflowexecution.execution.started`. Also removed the dead `RecordWorkflowStarted` method that emitted the wrong event type.
- **OCP AlertManager RBAC** (#576) -- Replaced `nonResourceURLs` with resource-level RBAC (`monitoring.coreos.com/alertmanagers/api`) for OCP's `kube-rbac-proxy`.
- **OCP AlertManager ClusterRole** (#471) -- Added `kubernaut-alertmanager-view` ClusterRole for EM on OCP.

#### Remediation Orchestrator

- **ManualReviewRequired completion** (#550) -- `ManualReviewRequired` remediations now transition to `PhaseCompleted` with `Outcome=ManualReviewRequired` instead of `PhaseFailed`.
- **Resilient hash capture** (#545) -- Pre-remediation hash capture uses 3-tuple return with graceful fallback when target resource is temporarily unavailable.

#### Helm Chart

- **PostgreSQL data directory** (#464) -- Pinned `PGDATA` to image-agnostic `/var/lib/kubernaut-pg/data`, preventing silent data loss when switching between upstream and OCP PostgreSQL images.
- **PostgreSQL secret rotation** (#557) -- Removed auto-generation of PostgreSQL passwords; secrets must be pre-created. Consolidated `postgresql-secret` and `datastorage-db-secret` into a single secret. Added `fail` validation with canary lookup guard.
- **CRD upgrade hook RBAC race** (#558) -- Added RBAC readiness check (`kubectl auth can-i`) before applying CRDs in the pre-upgrade hook job.
- **Notification routing** (#571) -- Replaced specific but incorrect routes with a catch-all `slack-and-console` receiver that correctly routes all notification types.
- **Restricted PodSecurity** (#449) -- `wait-for-postgres` init container complies with restricted PodSecurity standards.
- **LLM credentials mandatory** (#429) -- `llm-credentials` Secret is now mandatory; chart fails early if missing.
- **Self-healing caBundle** (#377) -- AuthWebhook caBundle is injected via init-container instead of relying on stale cert-manager annotations.

#### DataStorage

- **Wildcard label matching** (#464) -- `buildContextFilterSQL` array branch now includes `OR labels->'priority' ? '*'` for consistent wildcard matching.
- **Authwebhook catalog consistency** (#418, #512) -- Finalizer controller ensures DS catalog consistency on RemediationWorkflow deletion; recovery from orphaned DS workflows.

#### Signal Processing

- **Stale cache classification** -- Signal Processing refetches via APIReader to prevent stale informer cache from causing misclassification.

#### Gateway

- **Deleted pod webhook handling** (#451) -- Gateway no longer drops the entire webhook payload when one alert references a deleted pod.

### Changed

- **CRD: AffectedResource → RemediationTarget** (#542) -- Breaking rename across all CRDs, Go types, Rego policies, LLM schema, and documentation.
- **CRD: executionEngine moved to WFE status** (#518) -- Runtime-resolved field removed from spec; WE controller sets it during reconciliation.
- **Helm: Simplified values.yaml** (#403–#412) -- Consolidated `postgresql`/`externalPostgresql` and `valkey`/`externalValkey` into single groups; removed `eventExporter`, `nodePort`, and security context noise.
- **Helm: RHEL10 base images** (#348) -- PostgreSQL and Valkey migrated to RHEL10 direct pulls; Redis replaced with Valkey 8.
- **Helm: OCI registry** (#357) -- Chart URI moved from `ghcr.io` to `oci://quay.io/kubernaut-ai`.
- **Metrics port standardization** (#283) -- DataStorage and Notification metrics port aligned to `:9090`.
- **Rego: approval policies** (#542) -- `affected_resource` renamed to `remediation_target` in all approval and classification policies.
- **Rego: OPA v1.0 compliance** -- Replaced inline if/else with split rules for OPA v1.0 compatibility.
- **DB migrations squashed** -- All migrations consolidated into single `001_v1_schema.sql` for clean installations.

### Upgrade Notes

- **Breaking: CRD rename** — `AffectedResource` renamed to `RemediationTarget` across all CRDs. Existing `AIAnalysis` and `RemediationRequest` resources referencing the old field name must be recreated. See [blast radius documentation](docs/architecture/decisions/DD-CRD-004-affected-resource-rename.md) for migration guide.
- **Breaking: PostgreSQL secrets** — Auto-generation of PostgreSQL passwords has been removed. Secrets must be pre-created before `helm install`. See [README](charts/kubernaut/README.md) for required secret format. The separate `datastorage-db-secret` has been consolidated into `postgresql-secret`.
- **Breaking: Rego policies** — Approval policies now use `remediation_target` instead of `affected_resource`. Update any custom Rego policies.
- **PostgreSQL data directory** — Moved to `/var/lib/kubernaut-pg/data`. Existing installations from v1.0.0 must either copy data from the old path or repopulate the catalog after upgrade.
- **Helm values restructured** — `externalPostgresql`/`externalValkey` sections removed; use `postgresql.enabled=false` with `postgresql.auth.existingSecret` instead. `eventExporter` removed entirely.
- **OCI chart location** — Chart moved from `ghcr.io` to `oci://quay.io/kubernaut-ai/kubernaut`.

[1.1.0]: https://github.com/jordigilh/kubernaut/compare/v1.0.0...v1.1.0

## [1.0.0] - 2026-03-02

### Added

- **Signal Processing** -- Ingests Prometheus AlertManager webhooks and Kubernetes Events with fingerprint-based deduplication, Rego policy evaluation, and custom label injection
- **AI Analysis** -- HolmesGPT-powered root cause analysis with live `kubectl` access, remediation history context, and structured workflow recommendations
- **Workflow Execution** -- Multi-engine remediation via Kubernetes Jobs, Tekton Pipelines, and Ansible (AWX/AAP) with OCI-packaged workflow images
- **Remediation Orchestrator** -- Full pipeline coordination from signal to outcome, including approval gates, GitOps-aware delays, and escalation policies
- **Effectiveness Monitor** -- Post-remediation health checks using alert resolution, spec hash drift detection, and effectiveness scoring fed back into future analyses
- **DataStorage** -- Centralized workflow catalog with hybrid weighted-label scoring, remediation history tracking, and audit trail (SOC2-ready)
- **Gateway** -- Alert ingestion with distributed locking, storm detection, and suppression for NoActionRequired outcomes
- **Notification Service** -- Slack and console notification channels for remediation lifecycle events
- **Human Approval Gates** -- RemediationApprovalRequest CRD with configurable auto-approve and timeout policies
- **Helm Chart** -- Single-command deployment via `helm install` with CRD management and configurable RBAC
- **22 Demo Scenarios** -- End-to-end validated scenarios covering CrashLoopBackOff, OOMKill, HPA scaling, node failures, GitOps drift, certificate expiry, network policies, and more (see [kubernaut-demo-scenarios](https://github.com/jordigilh/kubernaut-demo-scenarios))

[1.0.0]: https://github.com/jordigilh/kubernaut/compare/v0.0.0...v1.0.0
