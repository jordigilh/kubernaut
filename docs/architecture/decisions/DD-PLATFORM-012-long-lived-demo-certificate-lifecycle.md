# DD-PLATFORM-012: Long-lived Demo Certificate Lifecycle

**Date**: 2026-11-04
**Status**: ✅ **APPROVED**
**Confidence**: 90% (architecture approved; live renewal and rollout evidence remains part of implementation verification)
**Related**: Issue #2602, BR-PLATFORM-014, DD-PLATFORM-001

---

## 🎯 Decision

The operator-backed long-lived fleet demo (`make setup-fleet-demo-infra`) SHALL use
cert-manager to own certificates whose expiry currently limits the environment's lifetime.
This behavior is demo-only; the short-lived FMC and Ginkgo fleet/fullpipeline suites retain their
existing one-shot certificate paths.

The demo SHALL use the following lifecycle:

1. A stable demo monitoring CA is bootstrapped once and made available in both hub and spoke
   clusters. The CA is valid for at least one year.
2. Each cluster receives its own cert-manager-managed `monitoring-tls` leaf Certificate with
   90-day validity and a renewal window. The spoke does not depend on a continuously copied hub
   leaf Secret.
3. `monitoring-ca`, `monitoring-client-ca`, and host-side monitoring CA files are public-trust
   projections of the stable CA. The static `web-config.yml` is stored in a dedicated
   `monitoring-web-config` Secret because cert-manager owns and rebuilds the `monitoring-tls`
   data map during issuance.
4. Managed Prometheus uses its native certificate/configuration reload path where available.
   Direct AlertManager, Thanos, and effectiveness-monitor consumers use an explicit reload or
   rollout mechanism. Reloader is used for workloads that need a pod-template change.
5. Before the operator reconciles demo DataStorage, cert-manager creates its
   `datastorage-signing-cert` with RSA-2048, one-year validity, 30-day `renewBefore`, and explicit
   `rotationPolicy: Always`. The demo CR references this Secret through
   `spec.dataStorage.audit.signingCertificate.secretName`; the operator hashes its resource
   version into `kubernaut.ai/datastorage-signing-material-revision`, which triggers a controlled
   DataStorage restart because the service loads the signing certificate during process
   construction.

---

## Context

The current demo path creates 24-hour monitoring and DataStorage certificates once. Monitoring
also copies the hub leaf Secret to the spoke once, so the copied leaf and all derived trust
material expire together without a renewal owner. DataStorage reads `tls.crt` and `tls.key` only
during startup; updating its mounted Secret alone does not replace the in-memory signer.

The existing Keycloak demo path already installs cert-manager and Reloader. The issue is therefore
best solved by extending the established demo dependency path rather than introducing a custom
renewal controller or changing operator/service production code.

---

## Alternatives considered

### A. cert-manager leaves per cluster plus stable shared monitoring CA — **chosen**

- ✅ Uses the existing cert-manager dependency and its observable Certificate status.
- ✅ Removes continuous cross-cluster leaf synchronization and gives hub/spoke independent renewal.
- ✅ Keeps trust-root sharing explicit while limiting the issue to disposable demo infrastructure.
- ✅ Handles startup-only DataStorage signing through the operator's source-managed
  `datastorage-signing-material-revision` rollout annotation.
- ➖ The CA root is intentionally not rotated by this issue; root rotation is a separate lifecycle
  event.

### B. Custom renewal and synchronization loop

- ❌ Duplicates cert-manager renewal, retry, ownership, and readiness behavior.
- ❌ Requires custom hub/spoke synchronization and consumer-restart logic.
- ❌ Still requires special handling for DataStorage's startup-only signer.

### C. cert-manager plus trust-manager or External Secrets

- ✅ Could provide a reusable multi-cluster trust distribution layer.
- ❌ Adds another dependency and a broader cross-cluster control-plane contract for a test/demo
  environment.
- ❌ Does not remove the need to restart DataStorage after signer renewal.

---

## Consequences

- The long-lived demo can outlive the current 24-hour certificate window without manual
  re-signing or leaf copying.
- Hub and spoke may renew their monitoring leaves at different times while retaining a common CA.
- DataStorage's signing key changes on renewal. The current export contract records the certificate
  fingerprint, but this issue does not archive historical certificates or guarantee external
  verification of signatures made with superseded keys.
- Reloader and cert-manager remain dependencies of the long-lived demo only; short-lived suites
  do not pay their installation or startup cost.
- A lifecycle test must prove the dedicated `monitoring-web-config` Secret remains separate from
  cert-manager's renewable `monitoring-tls` Secret.

---

## Implementation and verification

- Add manifest-contract tests for demo-only Issuer/Certificate resources, SANs, durations, renewal
  windows, `rotationPolicy`, trust projections, and reload annotations.
- Provision the monitoring CA and per-cluster leaf Certificates before applying the operator CR.
- Stop the demo path from overwriting the cert-manager-owned DataStorage Secret; retain the manual
  generator for short-lived callers. Configure the demo CR's signing Secret reference so the
  operator owns the durable restart trigger.
- Verify monitoring HTTPS after renewal, workload readiness, preservation of `web-config.yml`, and
  a successful DataStorage signed export after its renewal-triggered restart.
- Reference this decision from the demo-only implementation comments and report short-lived suite
  regression evidence separately.
