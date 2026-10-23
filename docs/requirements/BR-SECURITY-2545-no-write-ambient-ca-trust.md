# BR-SECURITY-2545: No-Write Ambient CA Trust Injection

**Business Requirement ID**: BR-SECURITY-2545
**Category**: Security / Platform
**Priority**: P1 (HIGH)
**Status**: Approved for implementation
**Issue**: [#2545](https://github.com/jordigilh/kubernaut/issues/2545)
**Related**: BR-SECURITY-493, BR-PLATFORM-005, BR-WE-018, DD-TLS-001

## Business need

Kubernaut services run with `readOnlyRootFilesystem: true`. Ambient CA injection
must not require a writable `/tmp`, `emptyDir`, or other writable directory that
could be abused by a compromised process for staging or command-and-control
artifacts.

## Requirements

1. Configured private CA trust MUST work together with the image's public/system
   trust store on supported Linux/UBI images.
2. Ambient injection MUST use a dedicated read-only CA directory through
   `SSL_CERT_DIR`; it MUST NOT create, modify, or delete a generated trust file.
3. The runtime owns the ambient environment contract: it clears
   `SSL_CERT_FILE` and sets `SSL_CERT_DIR` to the dedicated CA directory plus
   supported system CA directories. Legacy static ambient environment
   declarations are not supported.
4. `TLS_CA_FILE` MUST continue to point at the configured CA file so explicit
   shared transports and their CA file watcher retain reload behavior.
5. Missing, unreadable, malformed, or relative CA paths MUST fail before
   environment mutation and MUST be observable at service startup.
6. CA rotation MUST preserve last-known-good behavior for explicit reloadable
   transports. Consumers of Go's process-cached system pool require rollout or
   restart to adopt changed ambient trust.
7. Helm/operator deployments MUST remove legacy static ambient environment
   declarations and ambient-only writable `/tmp` mounts. Other business-required
   scratch paths remain in scope only when independently justified.

## Control mapping

| Requirement | FedRAMP/NIST | SOC 2 | OWASP ASVS 5.0.0 |
|---|---|---|---|
| Public and private CA trust over TLS | SC-8, SC-12, SC-13 | CC7.2 | V12.3.1–V12.3.4 |
| No ambient-generated file or writable ambient-only directory | AC-6, CM-7 | CC7.2 | V15.2.5 |
| Fail-closed CA validation and observable failure | SI-10, CM-6 | CC7.2 | V12.3.2, V16.3.4, V16.5.3 |
| Explicit CA transport rotation | SC-12, SC-13, CM-6 | CC7.2 | V12.3.4, V13.1.3 |

This requirement introduces no new audit-table event. CC8.1 applies to the
approved change-management process, not to this runtime TLS behavior.

## Acceptance criteria

- Private-CA HTTPS and public/system-CA HTTPS both succeed in a read-only UBI
  container without a writable `/tmp`.
- `SSL_CERT_FILE` is empty after ambient bootstrap; `SSL_CERT_DIR` contains the
  dedicated CA directory and supported system CA directories.
- Explicit `TLS_CA_FILE` transports still reload valid CA rotations and retain
  the previous transport after invalid rotations.
- Invalid CA input fails without partial environment mutation.
- Focused Helm cleanup removes ambient-only `/tmp` mounts while preserving
  legitimate writable paths.
- Operator deployment qualification covers the same contract in its separate
  migration worktree.
