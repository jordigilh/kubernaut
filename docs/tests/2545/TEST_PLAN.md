# Test Plan: Issue #2545 — No-Write Ambient CA Trust Injection

**Test Plan Identifier**: TP-2545-v1
**Status**: Approved for implementation
**Authority**: BR-SECURITY-2545, DD-TLS-001, Issue #2545
**Controls**: FedRAMP SC-8/SC-12/SC-13/AC-6/CM-7/SI-10/CM-6; SOC 2 CC7.2; OWASP ASVS V12.3.1–V12.3.4, V15.2.5, V16.3.4, V16.5.3

## Scope and objectives

- Replace temp-file ambient CA injection with read-only `SSL_CERT_DIR` trust.
- Preserve explicit `TLS_CA_FILE` transport and CA-reloader behavior.
- Prove private/system trust and startup behavior with a read-only UBI
  container contract.
- Remove only ambient-only Helm `/tmp` mounts; preserve legitimate scratch paths.

## Requirements-based scenario matrix

| ID | Tier | Business behavior | BR/control mapping |
|---|---|---|---|
| UT-TLS-2545-001 | UT | Valid CA owns `SSL_CERT_DIR` and creates no file | BR-SECURITY-2545; AC-6/CM-7; V15.2.5 |
| UT-TLS-2545-002 | UT | Legacy `SSL_CERT_FILE`/`SSL_CERT_DIR` values are replaced by the owned contract | BR-SECURITY-2545; SI-10; V12.3.4 |
| UT-TLS-2545-003 | UT | Supported system directories and private CA directory are both rendered | BR-SECURITY-2545/BR-PLATFORM-005; SC-8/SC-13; V12.3.1–V12.3.4 |
| UT-TLS-2545-004 | UT | Empty configuration remains a clean no-op | BR-SECURITY-493; SC-8 |
| UT-TLS-2545-005 | UT | Invalid/relative CA input fails before environment mutation | BR-SECURITY-2545; SI-10/CM-6; V16.3.4/V16.5.3 |
| UT-TLS-2545-006 | UT | Explicit `TLS_CA_FILE` reload path remains wired | BR-SECURITY-2545; SC-12/SC-13; V12.3.4 |
| UT-TLS-2545-007 | UT | Environment mutation rollback is observable and complete | BR-SECURITY-2545; SI-10; V16.5.3 |
| IT-TLS-2545-001 | IT | Production bootstrap works in read-only UBI with private/system CA trust | BR-SECURITY-2545; SC-8/AC-6/CM-7; CC7.2; V12.3.1–V12.3.4/V15.2.5 |
| IT-TLS-2545-002 | IT | Valid CA rotation updates explicit transports; invalid rotation keeps last-known-good | BR-SECURITY-2545; SC-12/SC-13/CM-6; CC7.2 |
| IT-HELM-2545-003 | IT | Deprecated chart has no ambient-only `/tmp` mount and keeps CA mount read-only | BR-SECURITY-2545; AC-6/CM-7; V15.2.5 |

Target: 7/7 meaningful UT scenarios, 3/3 in-repository IT rows, and 100% of
critical safety/security scenarios. The operator qualification is a separate
cross-repository gate.

## Pyramid invariant

- **UT** proves environment ownership, validation, rollback, and path logic.
- **IT** proves production bootstrap, real TLS trust, and CA reload behavior.
- **Deployment/E2E** proves the read-only filesystem and rendered-volume
  contract; operator qualification is performed in the operator repository.

## Execution

```bash
go test ./pkg/shared/tls/...
go test ./cmd/effectivenessmonitor/...
go test ./test/integration/shared/tls/...
helm unittest charts/kubernaut
```

Required final checks remain `go build ./...`, affected tests, lint, and the
project-mandated validation commands. Existing unrelated failures are reported
separately.
