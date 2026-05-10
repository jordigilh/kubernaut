# Changelog

All notable changes to Kubernaut will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Parallelized workflow validation** (#1070) — External validation checks (action-type taxonomy, OCI bundle existence, K8s dependency validation) now run concurrently during workflow registration, reducing registration latency from sum-of-three to max-of-three backend calls. Error priority contract preserved via typed-result-slot pattern (ADR-060).
- **Concurrency cap on dependency validation** (#1070) — `ValidateDependencies` now limits concurrent K8s API calls to 10 via `errgroup.SetLimit`, preventing API server overload from schemas with many dependencies.
- **Validation timeout budget** (#1070) — `validateExternalChecks` enforces a 10-second timeout to prevent degraded backends from consuming the full server WriteTimeout.

### Fixed

- **Request body size limit** — `HandleCreateWorkflow` now caps request body at 2 MiB via `http.MaxBytesReader` to prevent memory exhaustion from oversized payloads.
- **Deployment manifest probe paths** — Fixed `deploy/data-storage/deployment.yaml` liveness and readiness probes from `/health` to `/healthz` and `/readyz` to match the health server implementation.
- **OpenAPI domain mismatch** (UX-2) — Fixed `kubernaut.io` → `kubernaut.ai` in RFC 7807 problem type URIs across all OpenAPI specs (5 files). Domain now matches the URIs emitted by Go code.
- **Copyright year** (COMPAT-3) — Updated copyright headers from 2025 to 2026 in test files modified by this PR.

### Added

- **Workflow validation duration metric** — New `datastorage_workflow_validation_duration_seconds` Prometheus histogram with `phase` and `result` labels for per-phase observability.
- **ADR-060** — Architecture decision record documenting the parallel validation patterns and error priority contract.
- **RFC 7807 type constraint** (UX-1) — OpenAPI `RFC7807Problem.type` field now uses a `pattern` constraint (`^https://kubernaut\.ai/problems/.+`) with documented common types across all Data Storage API specs.
- **Concurrency guidelines** (DX-5) — Added concurrency patterns section to project guidelines documenting `errgroup`, typed-result-slot, and timeout budget patterns.
- **DD-WE-006 v2.2** (DOC-4) — Added changelog entry noting dependency validation parallelization per Issue #1070.
- **CONTRIBUTING.md Go version** (DX-4) — Updated prerequisite Go version from 1.25.3+ to 1.25.6+ to match `go.mod`.

## [1.2.0] - 2026-04-06

### Added

#### CRD Schema Hardening (#453–#459, #483)

- **Typed enums for all CRD status and reason fields** — Replaced raw strings with typed Go enums across all 9 CRDs: `SkipReason`, `BlockReason`, `FailurePhase` (RemediationRequest), `AnalysisType`, `Reason`, `PolicyDecision` (AIAnalysis), `Environment`, `Priority` (SignalProcessing), `ExecutionStatus` as `ConditionStatus` (WorkflowExecution), `Criticality`, `SLARequirement` (shared BusinessClassification), `CatalogStatus` (RemediationWorkflow, ActionType). Provides compile-time safety and OpenAPI validation.
- **Duration fields migrated to `metav1.Duration`** (#455) — WorkflowExecution duration strings replaced with structured `metav1.Duration` types.
- **Wide printer columns for RemediationRequest** (#387) — `kubectl get remediationrequests` now shows phase, outcome, severity, target, and age in wide format.
- **OAS catalog status alignment** (#483) — DataStorage OpenAPI enum values aligned with PascalCase CRD convention.

#### Per-Workflow ServiceAccount (DD-WE-005 v2.0, #481)

- **End-to-end SA propagation** — `serviceAccountName` field added to RemediationWorkflow, ActionType, AIAnalysis, RemediationRequest, and WorkflowExecution CRDs. Propagated through HAPI validation, RO controller, and WE executors.
- **Executor SA injection** — Ansible and Job executors use per-workflow SA instead of the hardcoded default, enabling least-privilege RBAC per remediation workflow.
- **DataStorage SA persistence** — Workflow catalog stores and returns `serviceAccountName` for catalog consistency.

#### WE Ansible TokenRequest Injection (#501)

- **TokenRequest API integration** — Ansible executor injects short-lived SA tokens via the Kubernetes TokenRequest API with configurable TTL validation, replacing long-lived secrets.
- **CRD schema migration** — `serviceAccount` string field migrated to structured `spec.serviceAccountName` with backward compatibility.

#### DS Resilience and Startup Reconciliation (#548)

- **Deterministic UUIDv5 workflow IDs** — Workflow IDs derived from content hash (UUIDv5), ensuring idempotent re-registration after PVC wipe.
- **Authwebhook startup reconciler** — New startup reconciler in authwebhook re-registers all RemediationWorkflows with DataStorage on controller startup, recovering from PVC data loss without manual intervention.

#### Hash-Capture Degradation Notifications (#546)

- **Degradation condition types** — New `PreRemediationHashCaptured` and `PostRemediationHashCaptured` condition types in VerificationContext surface hash-capture failures.
- **RO and EM reconciler integration** — Both controllers set degradation conditions when hash capture fails, propagating status to parent resources.
- **Completion notification enrichment** — Completion notifications now include hash-capture degradation status for operator visibility.

#### Effectiveness Monitor ADR-EM-001 Gaps (#573)

- **G1: Failed phase** — EM transitions EA to `Failed` for unrecoverable conditions (missing correlation, invalid spec).
- **G2: Scheduled event timing** — `effectiveness.assessment.scheduled` audit event emitted on all three entry transitions (WaitingForPropagation, Stabilizing, Assessing).
- **G3: Config knobs** — `prometheusLookback`, `maxConcurrentReconciles`, and `scrapeInterval` configurable via YAML.
- **G4: Assessment path differentiation** — Reconciler branches assessment depth based on WFE started/completed status. Partial-scope grace period (30s) handles async event propagation.

#### Feature Enrichments (#318, #366, #396, #435)

- **EA verification in completion notifications** (#318) — Completion notifications include effectiveness assessment results (health score, alert resolution, spec drift).
- **ResourceQuota detection** (#366) — Signal Processing detects namespace ResourceQuota/LimitRange constraints and surfaces them to the LLM via `ResourceQuotaConstrained` in DetectedLabels.
- **ConfigMap composite hashing** (#396) — RO and EM spec hash computation includes mounted ConfigMap content for drift detection across configuration changes.
- **LLM token usage in audit traces** (#435) — Token counts from HolmesGPT responses wired into audit events for cost tracking and usage analysis.

#### Notification Metrics (DD-METRICS-001)

- **1-layer metrics architecture** — Collapsed 3-layer notification metrics (interface → recorder → metrics) into a single `*Metrics` struct with direct Prometheus counter/histogram methods.

### Fixed

- **Duplicate scheduled audit event** — Removed duplicate `effectiveness.assessment.scheduled` emission from `emitAssessingTransitionEvents`.
- **HasWorkflowCompleted event type** — Corrected from `workflowexecution.execution.completed` to `workflowexecution.workflow.completed` to match the WE controller's actual event type.
- **Notification CircuitBreakerState type assertion** — Fixed `UpdateCircuitBreakerState` type assertion that would panic on non-string values.
- **RR CRD namespace and kind columns** (#622) — Added namespace and kind to `kubectl get rr -owide` output.
- **Notification field ordering and content** (#621, #626, #627) — Added RR name, reordered notification fields, included cluster name in timeout messages.
- **HAPI Phase 1 structured output** (#624) — Enabled `PHASE1_SECTIONS` for structured LLM output and refactored Pattern 2B to parse directly into Python dict, eliminating markdown round-trip.
- **EM validityWindow** (#625) — Increased EM `validityWindow` from 120s to 300s to prevent premature assessment expiry.
- **WE controller RBAC** (#637) — Added missing `serviceaccounts/token` create and `serviceaccounts` get permissions to the WorkflowExecution controller ClusterRole.
- **RR phase-specific Ready reasons** (#636) — Replaced generic Ready condition reasons with 12 phase-specific reasons (e.g., `Processing`, `Analyzing`, `AwaitingApproval`) for meaningful `kubectl get rr` REASON column.
- **RR kubectl column layout** (#635) — Overhauled `kubectl get rr` output with composite TARGET, WORKFLOW, CONFIDENCE, and ALERT columns. Added `FormatResourceDisplay`, `FormatWorkflowDisplay`, and `FormatConfidence` display helpers.
- **EM CPU metric query** (#639) — Corrected CPU metric query from raw `sum(container_cpu_usage_seconds_total)` to `sum(rate(...[5m]))`, preventing always-zero metric scores for counter-type metrics.
- **Graduated notification wording** (#639) — Replaced unconditional "anomaly persists" message with graduated wording based on `MetricsScore` (full improvement / partial improvement / minimal improvement / no improvement).
- **NotificationRequest PascalCase enums** (#640) — Migrated `NotificationType` and `NotificationPriority` enum values from lowercase to PascalCase for consistency with other CRD enums. Updated OpenAPI specs, ogen-client, routing attributes, and all test fixtures.
- **RR WorkflowDisplayName resolution** (#643) — Resolved workflow UUID to human-readable CRD name via `RemediationWorkflow` lookup. Fixed scheme registration, RBAC, and cache-blocking issue by using `apiReader` for direct API reads.
- **RR printer column layout** (#644) — Reordered `kubectl get rr -owide` columns to follow the pipeline flow, renamed "Target" to "RCA Target", added "Signal NS" column, removed redundant "Reason" column.

### Changed

- **CRD: Storm detection fields removed** (#448, DD-GATEWAY-015) — `stormCorrelationId`, `stormWindowId`, and related fields removed from SignalProcessing and RemediationRequest CRDs. All storm detection code removed from gateway.
- **EM client constructors simplified** — Prometheus and AlertManager HTTP clients now accept `(url, timeout)` instead of a pre-configured `*http.Client`.
- **Database migrations** — `002_add_service_account_name.sql` adds SA column; `003_capitalize_catalog_status.sql` normalizes enum casing.

### Upgrade Notes

- **Breaking: Storm detection removed** — `stormCorrelationId` and `stormWindowId` fields no longer exist in SignalProcessing and RemediationRequest CRDs. Any custom tooling reading these fields must be updated.
- **Breaking: CRD typed enums** — String fields replaced with typed enums across all CRDs. Existing resources with free-form string values that don't match the enum will fail validation on update.
- **Breaking: `metav1.Duration` fields** — WorkflowExecution duration fields now require Go duration format (e.g., `30s`, `5m`) instead of free-form strings.
- **Breaking: NotificationRequest PascalCase enums** (#640) — `NotificationType` values changed from lowercase (e.g., `escalation`, `manual-review`) to PascalCase (e.g., `Escalation`, `ManualReview`). `NotificationPriority` values changed similarly (e.g., `critical` → `Critical`). Update any routing configurations or tooling that matches on these values.
- **Database migration required** — Run migrations 002 and 003 before upgrading controllers. The Helm pre-upgrade hook handles this automatically.

[1.2.0]: https://github.com/jordigilh/kubernaut/compare/v1.1.0...v1.2.0

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
