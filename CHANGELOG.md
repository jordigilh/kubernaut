# Changelog

All notable changes to Kubernaut will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0-rc3] - 2026-03-20

### Fixed

- **Helm chart: image-agnostic PostgreSQL data directory** (#464) -- Pin `PGDATA` and volume mount to `/var/lib/kubernaut-pg/data` regardless of the PostgreSQL image variant (upstream vs OCP). Switching between `postgres:16-alpine` and `registry.redhat.io/rhel10/postgresql-16` previously changed the data directory path, causing silent data loss when the PVC retained data from the old path.
- **DataStorage: priority array branch wildcard fallback** (#464) -- `buildContextFilterSQL` now includes `OR labels->'priority' ? '*'` in the JSONB array branch, making it consistent with the scalar branch for wildcard priority matching.
- **DataStorage: severity wildcard validation** (#464) -- `ValidateMandatoryLabels` now accepts `"*"` as a valid severity value per DD-WORKFLOW-001 v2.8.

### Upgrade Notes

- **PostgreSQL data directory changed**: The data directory moved from image-specific paths (`/var/lib/postgresql/data` for upstream, `/var/lib/pgsql/data` for OCP) to a single image-agnostic path `/var/lib/kubernaut-pg/data`. Existing installations upgrading from rc1/rc2 must either:
  1. Copy existing data from the old path to the new path before upgrading, or
  2. Delete and re-apply all ActionType and RemediationWorkflow CRDs after upgrading to repopulate the empty catalog.

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

[1.0.0]: https://github.com/jordigilh/kubernaut/releases/tag/v1.0.0
