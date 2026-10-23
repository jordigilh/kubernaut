# DD-TLS-001: No-Write Ambient CA Trust Injection

**Date**: 2026-10-23
**Status**: Approved for implementation
**Issue**: #2545
**Related**: BR-SECURITY-2545, BR-SECURITY-493, BR-PLATFORM-005, DD-FLEET-010

## Decision

`sharedtls.InjectAmbientCACerts` SHALL stop generating a combined PEM file.
It SHALL:

1. Validate the configured absolute CA file before mutating process state.
   Reject CA directory paths containing the platform path-list separator so the
   directory cannot become multiple `SSL_CERT_DIR` entries.
2. Set `TLS_CA_FILE` to the configured CA path for explicit reloadable transports.
3. Clear `SSL_CERT_FILE` unconditionally; static process-wide file overrides are
   not part of the supported deployment contract.
4. Set `SSL_CERT_DIR` to the dedicated CA directory plus Go's supported Linux
   system certificate directories (`/etc/ssl/certs` and `/etc/pki/tls/certs`).
5. Perform environment updates transactionally and never write a trust file.

Helm/operator deployments must remove legacy static ambient environment
declarations and ambient-only writable `/tmp` mounts before consuming this
runtime contract. The CA directory is mounted read-only and contains only the
configured trust material.

## Context

The previous implementation concatenated the private CA and an image system
bundle into `/tmp/kubernaut-ambient-ca-*.pem`, then set `SSL_CERT_FILE` to that
file. This forced a writable volume into services otherwise protected by
`readOnlyRootFilesystem: true`.

Go's Linux `crypto/x509` loader reads the normal system CA bundle when
`SSL_CERT_FILE` is unset and additionally reads PEM files from `SSL_CERT_DIR`.
A UBI 10.2 read-only-container spike verified that a ConfigMap-style `ca.crt`
directory preserves both private and public/system trust.

## Alternatives rejected

### Per-client in-memory `x509.CertPool`

Rejected for this issue. It would require auditing and rewiring 11 services and
third-party clients that currently use process-wide Go trust. Scoped Fleet TLS
already has its own design in DD-FLEET-010.

### Writable or memory-backed `emptyDir`

Rejected. It reduces persistence but still exposes a writable staging location
to a compromised process and does not satisfy BR-SECURITY-2545.

### Legacy environment compatibility

Rejected by explicit decision. Static `SSL_CERT_FILE`/`SSL_CERT_DIR` declarations
are removed from Helm/operator deployments rather than preserved in runtime
compatibility branches.

## Consequences

- No ambient trust file is created at runtime.
- The process-wide system pool remains startup-cached; explicit
  `TLS_CA_FILE` transports retain CA-file hot reload.
- Deployments that retain legacy ambient environment declarations fail the
  deployment contract and must be migrated.
- A dedicated read-only CA directory is now a required deployment invariant.

## Verification contract

- UT: owned environment, system/private directory list, absolute-path and PEM
  validation, transactional rollback, empty-config behavior, and explicit
  `TLS_CA_FILE` reload wiring.
- IT: production bootstrap with a read-only temp/root filesystem and a UBI
  child process verifying private and public/system CA trust.
- Helm: no ambient-only `/tmp` mount and read-only CA mount; legitimate scratch
  paths remain.
- Operator: separate migration PR proves the same contract before release
  qualification.
