# BR-SECURITY-2548: TLS/SSL Writable-Directory Audit and Deployment-Contract Guardrails

**Business Requirement ID**: BR-SECURITY-2548
**Category**: Security / Platform
**Priority**: P1 (HIGH)
**Status**: Approved for implementation
**Issue**: [#2548](https://github.com/jordigilh/kubernaut/issues/2548)
**Related**: BR-SECURITY-2545, BR-SECURITY-493, BR-PLATFORM-005, BR-WE-018, DD-TLS-001

## Business need

Issue #2545 removed the writable ambient-CA generation path from the runtime.
Post-v1.6.0 GA, the remaining security objective is to keep the entire
TLS/SSL surface free of writable directories in supported production
deployment paths: a writable temporary directory is a convenient
post-compromise location for C2 artifacts, payload staging, and exfiltration
buffers. Because the application Helm chart is deprecated and removed
(commit `a45436df4`), the production deployment surface is the operator
repository's chart rendered by this repository's CI; upstream owns the
shared runtime behavior and the upstream deployment-contract guardrails.

## Requirements

1. No supported production application container receives a writable
   directory solely for ambient CA injection, `SSL_CERT_FILE`,
   `SSL_CERT_DIR`, or TLS/SSL certificate assembly.
2. `SSL_CERT_FILE` and `SSL_CERT_DIR` MUST NOT be statically injected as
   deployment configuration; ambient trust is configured only by the runtime
   adapter (`pkg/shared/tls`, per DD-TLS-001). `TLS_CA_FILE` remains
   available where explicit reloadable transports require it.
3. CA/TLS certificate material in workload manifests MUST be mounted
   read-only from `secret`, `configMap`, or `projected` volumes; `emptyDir`
   and `hostPath` MUST NOT back certificate material.
4. Repository-owned workload manifests and the CI-rendered operator
   deployment bundles MUST be mechanically guarded against regressions of
   requirements 2 and 3 (static manifest contract tests plus a CI job over
   the rendered production surface).
5. Ambient trust environment ownership MUST NOT drift out of
   `pkg/shared/tls`: no other non-test Go source may set or reference
   `SSL_CERT_FILE` / `SSL_CERT_DIR`.
6. The no-write property of `pkg/shared/tls` (DD-TLS-001) MUST be guarded
   against reintroduction of file-creation/mutation APIs.
7. Explicit `TLS_CA_FILE` transports MUST hot-reload valid CA rotations and
   retain last-known-good after invalid rotations, proven through real
   HTTPS round trips in a read-only Linux container.
8. Independently justified writable paths (workflow execution scratch per
   BR-WE-018, notification output, development-only certificate bootstrap,
   test fixtures) remain out of scope unless accidentally rendered in
   production workloads; each is enumerated in the audit document
   (`docs/security/TLS_WRITABLE_DIRECTORY_AUDIT_2548.md`).

## Control mapping

| Requirement | FedRAMP/NIST | SOC 2 | OWASP ASVS 5.0.0 |
|---|---|---|---|
| No writable TLS/SSL directory in production containers | AC-6, CM-7 | CC8.1 | V15.2.5 |
| Ambient env ownership confined to the runtime adapter | AC-6, CM-7, SI-10 | CC8.1 | V12.3.4, V16.5.3 |
| Read-only CA/TLS mounts, never emptyDir/hostPath | SC-8, SC-13, AC-6 | CC8.1 | V12.3.1–V12.3.4 |
| Rendered-deployment contract gated in CI | CM-6, CM-7 | CC8.1 | V15.2.5 |
| CA rotation with last-known-good retention | SC-12, SC-13, CM-6 | CC7.2 | V12.3.4, V13.1.3 |

Per the 2026-08-17 correction in AGENTS.md, the guardrail tests themselves
are change-management controls (CC8.1); CC7.2 applies only to the
audit-evidence/monitoring aspects of CA rotation.

## Acceptance criteria

- Static manifest contract tests (UT-TLS-2548-001..005) pass over the
  repository corpus with anti-vacuity anchors.
- The CI `tls-rendered-contract` job proves the rendered operator bundles
  (operator-bootstrap-rendered.yaml, operator-dependencies-rendered.yaml)
  satisfy the same contract.
- IT-TLS-2545-001 (read-only UBI private+public trust) and IT-TLS-2545-002
  (rotation with last-known-good) pass in the `shared` integration lane,
  which runs in CI.
- The audit document enumerates all findings, exclusions (with pinned
  versions), and out-of-scope writers with rationale.
- Work is scheduled after v1.6.0 GA (guardrails run in the normal unit and
  integration lanes; no GA-gated scheduling).
