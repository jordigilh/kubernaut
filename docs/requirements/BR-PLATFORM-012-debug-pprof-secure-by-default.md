# BR-PLATFORM-012: Secure-by-Default Profiling Toggle Across All Services

**Business Requirement ID**: BR-PLATFORM-012
**Category**: Platform
**Priority**: P2
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-08-24

---

## Business Need

### Problem Statement

Four services (`apifrontend`, `gateway`, `datastorage`, `kubernautagent`) exposed Go's
`net/http/pprof` profiling endpoints (`/debug/pprof/*`) on their health listener, gated by a
`server.disableProfiling` config field. This field had two problems:

1. **Inverted, confusing semantics** — `disableProfiling: false` (the double negative required to
   *enable* profiling) is easy to misread during a security review or incident response, exactly
   when a reviewer's attention is most needed.
2. **Inconsistent scope** — the remaining 9 Go services (`aianalysis`, `authwebhook`,
   `effectivenessmonitor`, `notification`, `remediationorchestrator`, `signalprocessing`,
   `workflowexecution`, and `fleetmetadatacache`) had no equivalent toggle at all:
   `fleetmetadatacache` hardcoded profiling **on** unconditionally (`enableProfiling: true`
   literal, no config field), and the 7 `controller-runtime`-based services had no pprof wiring
   whatsoever — dropping the capability entirely rather than exposing it safely.

`/debug/pprof/*` is a diagnostic surface, not a business API: it can leak goroutine stacks,
heap contents, and command-line arguments to anyone who can reach the listener (AC-6, least
privilege). An operator should have to make an explicit, informed choice to turn it on in any
given environment, and that choice should be named consistently across every service so it can be
reasoned about, documented, and audited in one place instead of 13 different ways.

Tracked in [Issue #2275](https://github.com/jordigilh/kubernaut/issues/2275).

---

## Business Objective

Rename the existing double-negative toggle to a single, positively-worded, shared config type
(`debug.pprofEnabled`, default `false`) and extend it uniformly to all 12 Go services (the
`console` UI is a Node.js/React service with no Go pprof surface, and is out of scope), so that:

- Profiling is off by default everywhere, with no exceptions and no hardcoded always-on path.
- Enabling it is a single, consistently-named, self-documenting opt-in per service.
- The same shared Go type and Helm schema fragment back every service, eliminating drift between
  how different services expose (or fail to expose) this diagnostic surface.

### Success Criteria

1. `internal/config.DebugConfig{PprofEnabled bool}` is the single shared Go type used by all 12
   services' config structs (`debug.pprofEnabled` in YAML), replacing every prior
   `server.disableProfiling` field and the FMC hardcoded literal.
2. For the 4 services with a custom HTTP health mux (`apifrontend`, `gateway`, `datastorage`,
   `fleetmetadatacache`), `cfg.Debug.PprofEnabled` gates whether `/debug/pprof/*` handlers are
   registered on that mux.
3. For the 7 `controller-runtime`-managed services (`aianalysis`, `authwebhook`,
   `effectivenessmonitor`, `notification`, `remediationorchestrator`, `signalprocessing`,
   `workflowexecution`), `cfg.Debug.PprofEnabled` gates `ctrl.Options.PprofBindAddress` (a
   dedicated `:6060` listener — `controller-runtime`'s health-probe mux does not expose itself for
   custom handler registration, so pprof must run on its own listener for these services).
   `kubernautagent` keeps its existing custom-mux pattern (it is not `controller-runtime`-managed)
   but nests the field under `runtime.debug.pprofEnabled` to match its existing `runtime.*` config
   domain grouping (ADR-030).
4. `charts/kubernaut/values.schema.json` declares one shared `#/definitions/debug` fragment
   (`pprofEnabled: boolean`, default `false`) referenced by all 12 services; the 7
   `controller-runtime` services' Deployment templates conditionally add a `containerPort: 6060`
   only when the flag is true.
5. `go build ./...`, `go test ./...` (all 12 services' config packages), `helm unittest
   charts/kubernaut/`, `make check-helm-coverage`, and `make verify-helm-defaults-parity` all pass.
6. `CHANGELOG.md` documents this as a breaking rename with upgrade guidance for any existing
   `server.disableProfiling: true|false` values in operator ConfigMaps/Helm overrides.

---

## Functional Requirements

- **FR-1 (Shared type)**: `internal/config/debug.go` defines `DebugConfig{PprofEnabled bool}`,
  `DefaultDebugConfig()` (returns `PprofEnabled: false`), and `PprofBindAddress(enabled bool)
  string` (returns `":6060"` when enabled, `""` — meaning "listener disabled" — otherwise, per
  `ctrl.Options.PprofBindAddress`'s own contract).
- **FR-2 (4 custom-mux services)**: `apifrontend`, `gateway`, `datastorage` each gain a top-level
  `Debug sharedconfig.DebugConfig` field (removing `ServerConfig.DisableProfiling`); their
  `buildHealthMux`/`NewHealthServer` call sites are updated to pass `cfg.Debug.PprofEnabled`
  directly (no double negative).
- **FR-3 (FMC hardcoded-value fix)**: `fleetmetadatacache` gains the same `Debug` field and its
  health-mux construction switches from the `enableProfiling: true` literal to
  `cfg.Debug.PprofEnabled` (default `false`) — this is a behavior change (profiling was
  unconditionally on; now it is off by default), not just a rename, since no config field
  previously existed to opt out.
- **FR-4 (7 controller-runtime services)**: each of `aianalysis`, `authwebhook`,
  `effectivenessmonitor`, `notification`, `remediationorchestrator`, `signalprocessing`,
  `workflowexecution` gains a top-level `Debug sharedconfig.DebugConfig` field; each service's
  `main.go` passes `internalconfig.PprofBindAddress(cfg.Debug.PprofEnabled)` into
  `ctrl.Options.PprofBindAddress` when constructing the manager.
- **FR-5 (kubernautagent)**: `RuntimeConfig` gains `Debug internalconfig.DebugConfig` (nested
  under `runtime`, matching ADR-030's domain grouping — `runtime.debug.pprofEnabled` in YAML);
  `cmd/kubernautagent/health.go`'s conditional flips from `!cfg.Runtime.Server.DisableProfiling` to
  `cfg.Runtime.Debug.PprofEnabled`.
- **FR-6 (Helm schema)**: `values.schema.json` gains a shared `#/definitions/debug` fragment,
  referenced via `$ref` from all 12 services' `properties` (13th, `console`, excluded — no Go
  pprof surface). `_generated_defaults.tpl` is regenerated (`make generate-helm-defaults`) so the
  new `debug.pprofEnabled: false` default materializes without requiring an explicit
  `values.yaml` entry (DA14 trim convention).
- **FR-7 (Helm templates)**: each of the 12 services' `configYAML` define block renders a
  `debug: pprofEnabled: <bool>` block (nested under `runtime:` for `kubernautAgent`); the 7
  `controller-runtime` services' Deployment templates conditionally add a `containerPort: 6060`
  (named `pprof`) only when `debug.pprofEnabled` is `true`.
- **FR-8 (Helm test coverage)**: `charts/kubernaut/tests/debug_pprof_toggle_test.yaml` (11
  services) and `fleetmetadatacache_debug_pprof_test.yaml` (FMC) assert both the default-`false`
  ConfigMap rendering and the explicit-override-`true` rendering (including the conditional
  `containerPort: 6060` presence/absence) for every service.

---

## Non-Goals

- Does not add a Prometheus metric, alert, or audit event for pprof being enabled — this is a
  local operator-facing debug toggle, not a business/security event stream.
- Does not add TLS, auth, or network-policy restrictions specifically for the pprof listener
  beyond what already applies to the service's existing listeners/NetworkPolicy — restricting
  *who* can reach port 6060 is an existing, unchanged NetworkPolicy/RBAC concern, not something
  this BR introduces or changes.
- Does not extend this toggle to the `console` service (Node.js/React; no `net/http/pprof`
  surface) or to the Kubernaut Operator's own Helm chart (out of scope; the operator's downstream
  adoption of this same rename is tracked separately — see the comment left on
  [Issue #2275](https://github.com/jordigilh/kubernaut/issues/2275) for the operator team).
- Does not change the pprof port away from `6060` for the 7 `controller-runtime` services, or
  attempt to multiplex it onto an existing listener — `controller-runtime`'s
  `HealthProbeBindAddress` mux is not exposed for custom handler registration, so a dedicated
  listener via `ctrl.Options.PprofBindAddress` (native since `controller-runtime` v0.24.1) is the
  only available integration point.

---

## FedRAMP / NIST 800-53 Control Mapping

| Control | Requirement Satisfied |
|---|---|
| **AC-6** (Least Privilege) | FR-2/FR-3/FR-4/FR-5: profiling endpoints, which can leak process memory/stack contents, default to disabled on every service with no exceptions (closing FMC's prior always-on gap); enabling them is now a single, positively-named, explicit opt-in rather than a double-negative flag or no flag at all. |
| **CM-6** (Configuration Settings) | FR-1/FR-6: one shared Go type and one shared Helm schema fragment back this setting across all 12 services, eliminating the prior per-service drift (present-but-inverted on 3 services, present-but-hardcoded-on on 1, entirely absent on 8). |
| **SI-10** (Information Input Validation) | FR-8: `helm-unittest` coverage proves the schema-declared default and explicit override both render through to the Go config on every service, closing the same class of "accepted but not actually wired" gap BR-PLATFORM-011 exists to detect. |

---

## Related Decisions

- **Tracked in**: [Issue #2275](https://github.com/jordigilh/kubernaut/issues/2275).
- **Depends on**: `controller-runtime` v0.24.1+ for native `ctrl.Options.PprofBindAddress` support
  (FR-4).
- **Complements**: [BR-PLATFORM-011](BR-PLATFORM-011-helm-chart-config-knob-test-coverage.md) —
  this BR's new `debug.pprofEnabled` knob is covered by real `helm-unittest` assertions (FR-8) from
  the moment it's introduced, rather than being seeded into the coverage allowlist.
- **Downstream**: the Kubernaut Operator (separate repo) has its own equivalent
  `DisableProfiling`-style field; a comment was left on Issue #2275 so that team can adopt the same
  `debug.pprofEnabled` naming and default-off posture independently.

---

**Document Status**: ✅ Approved
**Priority**: P2 — closes an AC-6 least-privilege gap (FMC's unconditional profiling) and a CM-6
consistency gap (inverted/absent toggle naming) across all 12 Go services.
