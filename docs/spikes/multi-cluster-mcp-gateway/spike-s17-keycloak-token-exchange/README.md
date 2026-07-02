# Spike S17 — Live Keycloak Standard Token Exchange Validation

## Goal

Empirically prove (not just doc-verify) that a single Keycloak instance/realm
can perform RFC 8693 Standard Token Exchange between two clients — the
architectural conclusion reached earlier in the same session when comparing
Dex's chicken-and-egg self-referencing limitation against Keycloak's native
support — and confirm the exact wire protocol required matches what
`kube-mcp-server`'s existing `keycloak-v1` exchange strategy already sends.

This follows up on the live 2-Dex-instance spike (same session, undocumented
as a formal spike at the time) that proved Dex requires two separate
instances plus human-oriented claim-mapping workarounds (`userNameKey`,
fake `email` mapping) to exchange a machine (`client_credentials`) token.

## Context

- Prior finding (doc-based only): Keycloak's "Standard Token Exchange" (GA
  since 26.2, RFC 8693-compliant) supports internal-internal exchange within
  one realm, no second instance, no connector bootstrap ordering problem.
- Prior finding (live-tested): Dex cannot self-reference for token exchange
  (`dial tcp 127.0.0.1:5556: connect: connection refused` during its own
  connector-initialization phase) — a consequence of Dex modeling "myself"
  as an external upstream IdP via its connector abstraction. Two Dex
  instances (~128MB combined) work around this, at the cost of
  human-oriented claim-mapping hacks since Dex's OIDC connector expects
  `name`/`email` claims that machine tokens don't carry.
- `kube-mcp-server` already ships a `keycloak-v1` exchange strategy
  (`pkg/tokenexchange/keycloak_v1_exchanger.go`) that sends a standard
  `grant_type=urn:ietf:params:oauth:grant-type:token-exchange` request with
  `subject_token`, `subject_token_type`, `audience`, optional `scope`, and
  client credentials — this spike validates that exact wire shape against a
  real server, not just the Go code in isolation.

## Test Environment

- **Keycloak**: `quay.io/keycloak/keycloak:26.6.4` (podman container,
  `start-dev` mode, H2 dev DB)
- **Feature flags**: `--features=admin-fine-grained-authz:v1` (tested, see
  Finding 3 below — turned out to be a red herring for Standard Exchange)
- **Realm**: `kubernaut-fleet` (throwaway, created via `kcadm.sh`)
- **Clients**:
  - `kubernaut-fleet-read` — confidential, `serviceAccountsEnabled=true`,
    `standard.token.exchange.enabled=true` (mirrors FMC's real gateway client)
  - `kube-mcp-server` — confidential, `serviceAccountsEnabled=true` (mirrors
    the real backend audience)
  - `workflow-execution-write` — confidential, second target client used only
    to test the negative/authorization-boundary case

## Key Findings

### 1. Single-realm, single-instance exchange works — no chicken-and-egg

A `client_credentials` token minted for `kubernaut-fleet-read` (baseline
`aud: account`) was successfully exchanged, in the same realm and the same
running instance, for a new token audienced for `kube-mcp-server`:

```
POST /realms/kubernaut-fleet/protocol/openid-connect/token
  grant_type=urn:ietf:params:oauth:grant-type:token-exchange
  client_id=kubernaut-fleet-read&client_secret=***
  subject_token=<client_credentials token>
  subject_token_type=urn:ietf:params:oauth:token-type:access_token
  requested_token_type=urn:ietf:params:oauth:token-type:access_token
  audience=kube-mcp-server
```

Result: `200 OK`, new JWT with `aud: kube-mcp-server`, `azp:
kubernaut-fleet-read` (original identity preserved), no restart, no second
process, no claim-mapping workaround. Directly falsifies any assumption that
the 2-instance pattern proven for Dex is a general Token Exchange
requirement — it is Dex-specific (connector/plugin architecture), not
inherent to RFC 8693 or a property of "any IdP doing token exchange."

### 2. No human-oriented claim-mapping hacks needed

Unlike Dex (which required `userNameKey: sub` and `claimMapping.email: sub`
overrides to accept a machine token through its OIDC-connector-based
exchange path), Keycloak's Standard Token Exchange validates the subject
token directly against realm/client state — the exchanged token above
required zero claim remapping. This matches the architectural distinction
identified earlier: Dex's exchange is connector-based (built for federating
external human IdPs *into* Dex); Keycloak's is a native realm capability
(built for re-scoping tokens Keycloak already issued).

### 3. Surprising finding: FGAP `token-exchange` admin permission does NOT gate Standard (v2) Exchange

Keycloak's docs describe a "token-exchange scope permission" (via Admin
Console → Client → Permissions) as the access-control mechanism, requiring
Fine-Grained Admin Permissions (`admin-fine-grained-authz:v1` — the doc
explicitly says v2 dropped token-exchange permission support). We tested
this exhaustively:

| Configuration on target client | Exchange result |
|---|---|
| No FGAP, no audience mapper | ❌ `Requested audience not available` |
| No FGAP, audience mapper assigned to requester | ✅ succeeds |
| FGAP enabled, `token-exchange` scope permission with an **allow** policy naming the requester | ✅ succeeds |
| FGAP enabled, `token-exchange` scope permission with a policy that **excludes** the requester | ✅ **still succeeds** (unexpected) |
| Audience-mapper scope **unassigned** from requester (FGAP irrelevant) | ❌ `Requested audience not available` |

**Conclusion**: for Standard (v2) Token Exchange in Keycloak 26.6, the real
and only access-control lever we could empirically confirm is **client-scope
assignment** — whether the requesting client has been given a client scope
containing an `oidc-audience-mapper` for the target `client_id`. The legacy
FGAP `token-exchange` admin permission (documented for the older Token
Exchange V1 mechanism) had **no observable effect** on the Standard exchange
grant in this version. This is a materially simpler (and more auditable)
model than initially assumed: least-privilege (AC-6) is enforced by which
audience-scopes a realm admin assigns to which client — a standard,
reviewable IaC-friendly config (realm export JSON / Terraform), not a
separate authorization-policy subsystem.

**Caveat**: this was tested against Keycloak 26.6.4 dev-mode only. Not
verified whether newer/older minor versions or non-dev storage backends
change this behavior; if we build on this finding, re-validate against
whatever Keycloak version we'd actually deploy.

### 4. Wire-compatible with `kube-mcp-server`'s existing `keycloak-v1` strategy

`pkg/tokenexchange/keycloak_v1_exchanger.go` in `kube-mcp-server` sends
exactly the form parameters validated live in Finding 1
(`grant_type`, `subject_token`, `subject_token_type`, `audience`, optional
`scope`, plus client auth via `injectClientAuth`). No code changes to
`kube-mcp-server` would be needed to talk to a real Keycloak instance
configured this way — only realm/client configuration (client-scope +
audience-mapper assignment, `standard.token.exchange.enabled=true` on the
requesting client) is required on the Keycloak side.

## Validation Results

| Test | Result |
|---|---|
| Baseline `client_credentials` token from `kubernaut-fleet-read` | `aud: account` |
| Exchange to `kube-mcp-server` (no audience mapper yet) | `invalid_request: Requested audience not available` |
| Exchange to `kube-mcp-server` (audience mapper + default client-scope added) | `200 OK`, `aud: kube-mcp-server`, `azp: kubernaut-fleet-read` |
| Exchange to `workflow-execution-write` (audience mapper present, no FGAP configured) | `200 OK` (scope-assignment alone was sufficient) |
| Exchange to `workflow-execution-write` (FGAP enabled, policy excludes requester) | `200 OK` (FGAP had no effect) |
| Exchange to `kube-mcp-server` after removing the audience-scope assignment | `invalid_request: Requested audience not available` (confirms scope assignment is the real gate) |

## Decision

**YES** — Keycloak Standard Token Exchange is viable for Kubernaut's
prod/dev federated-delegation architecture with a single instance/realm, no
claim-mapping workarounds, and is wire-compatible with `kube-mcp-server`'s
existing `keycloak-v1` strategy today. This confirms and upgrades the
doc-based conclusion from earlier in the session to a live-validated one.

## Scope Note: What This Spike Does NOT Cover

This was a throwaway, standalone Keycloak container — **not** wired into any
CI lane (fleet E2E, fleetmetadatacache E2E, or otherwise), and did not
exercise `kube-mcp-server`'s real binary against it. Per the prior session's
"middle path" recommendation, the next step (if pursued) should still be a
dedicated, narrow `testcontainers`-based integration test that runs
`kube-mcp-server`'s actual `keycloak-v1` exchanger against a real Keycloak
container — decoupled from the fleet/fleetmetadatacache Kind-based E2E
lanes (which should keep using Dex, per the resource-footprint decision
already made: 2×Dex ≈ 128MB vs. 1×Keycloak ≈ 1.25–2GB).

## Risks / Open Questions for a Future Formal Plan

| Risk | Notes |
|---|---|
| FGAP finding (§3) needs re-validation before relying on it for a security control | Only tested on 26.6.4 dev-mode; if least-privilege enforcement depends on scope-assignment alone, that must be explicitly documented as the control, not assumed to be defense-in-depth via FGAP too |
| Realm/client bootstrap is currently manual (`kcadm.sh` shell commands) | Production/dev would need a realm-export JSON or Terraform provider, not ad hoc CLI calls |
| `kube-mcp-server`'s `keycloak-v1` strategy not yet exercised against a real Keycloak in an automated test | This spike validated the wire protocol by hand (curl), not the actual Go exchanger code path |
| No test of token refresh, expiry, or revocation semantics for exchanged tokens | Out of scope for this spike |

## Confidence Assessment: 90%

- Standard Token Exchange (Finding 1) is validated with direct, repeatable
  live evidence (not just docs) — high confidence.
- The FGAP-has-no-effect finding (Finding 3) is surprising and only tested
  in one version/mode; flagged as needing re-validation before being
  load-bearing for a security design.
- `kube-mcp-server` wire-compatibility (Finding 4) is a code-read
  cross-check, not an automated test — the actual Go binary was not run
  against this Keycloak instance.

## Cleanup

All spike containers (`keycloak-spike`) were removed after validation. No
persistent infrastructure was created or left running.
