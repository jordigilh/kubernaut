# BR-PLATFORM-014: Demo Environment Certificate and State Lifecycle

**Business Requirement ID**: BR-PLATFORM-014
**Category**: Platform
**Priority**: P2
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-08-31

---

## Business Need

### Problem Statement

The fleet demo/QE environment (`hack/setup-fleet-infra/main.go` -> `SetupFleetCoreInfrastructureWithGateway`)
is meant to run for days at a time so QE can exercise the full fleet stack against a stable
cluster. Two compounding gaps made it fragile instead:

1. **No certificate renewal.** `ensureKeycloakTLSFromChartCA` (`test/infrastructure/fullpipeline_e2e_helm.go`)
   signs Keycloak's TLS leaf certificate with a hardcoded 24-hour validity, once, at cluster
   bring-up. Nothing re-signs it afterward. Fine for a CI E2E run that lives 15-40 minutes; fatal
   for a demo cluster kept alive for days.
2. **No state persistence.** `deployKeycloakInNamespace` (`test/infrastructure/keycloak_e2e.go`)
   runs Keycloak in `start-dev` mode with the default `dev-file` H2 database and no
   `PersistentVolumeClaim`. Every pod restart -- whether from the cert expiring, a crash, a node
   drain, or an upgrade -- wipes Keycloak's in-container database, regenerating fresh realm
   signing keys, client secrets, and users. This cascades into every service that trusts a
   Keycloak-issued token (e.g. the Envoy AI Gateway's cached JWKS) reporting `401 Unauthorized`
   until manually fixed.

Confirmed in production incident: the 24-hour cert expired, the pod restarted to pick up a
manually-issued replacement, Keycloak's signing keys churned, and the Envoy AI Gateway started
rejecting every token until its own deployment was manually restarted too.

### Business Impact

Manual intervention (re-issuing certs, restarting gateways) every time a demo/QE cluster's
Keycloak pod restarts blocks QE testing, wastes engineering time on a problem that should be
solved once, and makes the demo environment untrustworthy as a stand-in for how the fleet stack
behaves in a long-lived deployment.

---

## Business Objective

The fleet demo/QE environment's TLS material and stateful services MUST survive routine pod
restarts (certificate rotation, crash, node drain, upgrade) without manual intervention or
environment rebuild -- scoped strictly to the demo entry point, with zero behavior change for the
short-lived FMC and Ginkgo "fleet"/"fullpipeline" E2E test suites, which intentionally remain
ephemeral.

### Success Criteria

1. Keycloak's TLS certificate in the demo environment auto-renews via `cert-manager` well before
   expiry, with no manual re-signing step.
2. Keycloak's realm state (signing keys, imported realm, client secrets) survives a pod restart
   in the demo environment via persistent storage.
3. When cert-manager renews Keycloak's certificate, the Keycloak deployment automatically picks
   up the new certificate (via `stakater/Reloader`) without an engineer manually triggering a
   restart.
4. Because Keycloak's signing keys no longer churn on restart (success criterion 2), dependent
   services' cached JWKS (e.g. the Envoy AI Gateway) remain valid across a Keycloak restart
   without needing their own restart.
5. The short-lived FMC and Ginkgo "fleet"/"fullpipeline" E2E suites are unaffected: same
   ephemeral Keycloak, same ad hoc 24-hour chart-CA-signed certificate, zero added dependencies or
   startup time.

---

## Functional Requirements

- **FR-1 (Persistence)**: `DeployKeycloakInfra`/`deployKeycloakInNamespace` gain a `persistent
  bool` parameter. When `true`, a `PersistentVolumeClaim` is mounted at Keycloak's `dev-file` H2
  database path so realm state survives pod restarts. When `false` (the FMC and Ginkgo suites'
  existing behavior), the manifest is unchanged.
- **FR-2 (Auto-renewing certificate)**: In the demo path only, Keycloak's TLS certificate is
  issued by a namespaced `cert-manager` `Issuer` (type `ca`, chained to the same inter-service CA
  every other service already trusts) instead of the one-shot ad hoc leaf cert, so cert-manager
  renews it automatically ahead of expiry.
- **FR-3 (Automatic reload)**: The demo path installs `stakater/Reloader` and annotates the
  Keycloak Deployment so a certificate renewal triggers an automatic rolling restart, with no
  engineer intervention.
- **FR-4 (Scoping)**: All new behavior (FR-1 through FR-3) is gated on the existing
  `keycloakNamespace != namespace` signal in `provisionFleetCoreInfra`
  (`test/infrastructure/fleet_e2e.go`), which already distinguishes the demo entry point from the
  FMC and Ginkgo suites' shared-namespace, short-lived Keycloak deployments.

---

## Non-Goals

- Does not change certificate lifecycle for the short-lived FMC or Ginkgo "fleet"/"fullpipeline"
  E2E suites -- their Keycloak deployments remain ephemeral by design.
- Does not add production-grade external-DB backing for Keycloak (e.g. `KC_DB=postgres`) -- a
  `PersistentVolumeClaim` under the existing `dev-file` H2 mode is sufficient for a single-replica
  demo environment.
- Does not introduce a general-purpose secret-rotation framework -- this is scoped to Keycloak's
  TLS certificate in the demo environment specifically.

---

## FedRAMP / NIST 800-53 Control Mapping

This BR governs test/demo infrastructure lifecycle, not a production audit or access-control
capability, so no SOC2/FedRAMP control mapping applies (see AGENTS.md's Business Requirements
Mandate: control mapping is required for audit-emitting production services, not local E2E/demo
tooling).

---

## Related Decisions

- **Root cause incident**: manual Keycloak cert expiry + signing-key churn resolved by hand on the
  running `~/tmp/hub.yaml` demo cluster; this BR codifies the permanent fix.
- **Reuses**: `test/infrastructure/datastorage.go`'s existing `InstallCertManager`/
  `WaitForCertManagerReady` helpers (already validated by
  `test/e2e/datastorage/05_soc2_compliance_test.go`), and `ApplyCertManagerIssuer`'s retry/backoff
  pattern for webhook-propagation races.

---

**Document Status**: ✅ Approved
**Priority**: P2 -- unblocks reliable, long-lived QE testing against the fleet demo environment
without recurring manual intervention.
