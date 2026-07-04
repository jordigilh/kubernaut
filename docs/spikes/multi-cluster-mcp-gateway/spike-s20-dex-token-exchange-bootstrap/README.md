# Spike S20: Dex-to-Dex RFC 8693 Token Exchange Bootstrap Ordering

**Date**: 2026-07-03 (live re-validation of a 2026-07-01/02 finding, plus new
kube-mcp-server compatibility testing)
**Author**: AI Assistant
**Related**: Issue #54, ADR-068, Spike S17 (Keycloak Standard Token Exchange),
Spike S18, Spike S19, DD-TEST-013

---

## Objective

Determine whether a genuine two-party Dex-to-Dex RFC 8693 token exchange flow
(Dex-A issuing tokens to clients, Dex-B exchanging Dex-A's tokens for its own
Dex-B-issued tokens via an OIDC connector) can replace Keycloak as the
identity provider for the `test/e2e/fleet` suite's remote-cluster,
`passthrough`+exchange augmentation, mirroring what Spike S17/S18/S19 already
proved for Keycloak but with a lighter-weight IdP (~128MB for 2x Dex vs.
~1.25-2GB for Keycloak).

This spike closes both open items left by the original (undocumented) live
session: (1) it re-validates the two-instance bootstrap-ordering finding with
concrete manifests and logs, and (2) it validates -- for the first time --
whether `kube-mcp-server`'s actual STS code path can complete an exchange
against Dex, which Spike S17/S18 never tested (Keycloak only).

## Test Environment

- **Dex**: `ghcr.io/dexidp/dex:latest` (same image `test/infrastructure/dex_e2e.go`
  already uses for the `fleet`/`kubernautagent` E2E lanes), two independent
  podman containers on a shared `dex-spike-net` bridge network, addressed by
  container name (`dex-a`, `dex-b`)
- **kube-mcp-server**: `ghcr.io/containers/kubernetes-mcp-server:latest` (same
  image `test/infrastructure/fleet_e2e.go` deploys), binary extracted from the
  image and inspected directly (`strings`) since no source checkout was
  available locally
- Both images pulled fresh for this spike; all containers/network removed
  after validation (see Cleanup)

## Part 1: Two-Instance Topology and Bootstrap Ordering (re-validated)

### Why self-exchange doesn't work

Dex's token-exchange grant (`urn:ietf:params:oauth:grant-type:token-exchange`)
is implemented in terms of a configured **OIDC connector** pointing at an
external upstream issuer: the `subject_token` presented to `/token` must have
been issued by that configured upstream connector, not by the same Dex
instance validating it. A single Dex instance cannot reference itself as its
own connector at startup (this was the original "chicken-and-egg" symptom
from the 2026-07-01/02 session: `dial tcp 127.0.0.1:5556: connect: connection
refused` while a self-referencing connector was still initializing). This
forces a genuine two-instance topology:

- **Dex-A** (primary): issues tokens to callers via `client_credentials`.
  No connectors of its own needed beyond `enablePasswordDB: true` (Dex
  refuses to start with zero connectors configured at all, even for
  client-credentials-only use -- `failed to initialize server: server: no
  connectors specified`).
- **Dex-B** (exchanger): configured with an `oidc` connector pointing at
  Dex-A's issuer URL, exchanges Dex-A-issued tokens for Dex-B-issued tokens
  audienced for a downstream resource server (e.g. `kube-mcp-server`).

### Confirmed manifests (Dex-A)

```yaml
issuer: http://dex-a:5556/dex
storage:
  type: memory
web:
  http: 0.0.0.0:5556
enablePasswordDB: true # required: Dex refuses to start with zero connectors
oauth2:
  grantTypes:
    - authorization_code
    - client_credentials
    - refresh_token
  responseTypes: ["code", "token", "id_token"]
  skipApprovalScreen: true
staticClients:
  - id: kubernaut-fleet-read
    name: 'Kubernaut Fleet Read (spike)'
    secret: e2e-fleet-secret
    grantTypes:
      - client_credentials
    clientCredentialsClaims:
      groups:
        - mcp-read
```

### Confirmed manifests (Dex-B)

```yaml
issuer: http://dex-b:5557/dex
storage:
  type: memory
web:
  http: 0.0.0.0:5557
oauth2:
  grantTypes:
    - authorization_code
    - urn:ietf:params:oauth:grant-type:token-exchange
  responseTypes: ["code", "token", "id_token"]
  skipApprovalScreen: true
connectors:
  - type: oidc
    id: dex-a
    name: Dex-A
    config:
      issuer: http://dex-a:5556/dex
      clientID: dex-b-connector # required field, but its value is unused by the exchange flow itself
      clientSecret: dex-b-connector-secret
      redirectURI: http://dex-b:5557/dex/callback
      getUserInfo: true # REQUIRED: subject token is an access_token, not id_token
      insecureSkipEmailVerified: true
      userNameKey: sub # REQUIRED for machine tokens -- see Finding 2 below
      claimMapping:
        email: sub # REQUIRED for machine tokens -- see Finding 2 below
staticClients:
  - id: kube-mcp-server
    name: 'kube-mcp-server (spike)'
    secret: kube-mcp-server-secret
    grantTypes:
      - urn:ietf:params:oauth:grant-type:token-exchange
```

### Finding 1: Bootstrap ordering is a strict, one-directional, fail-fast requirement (not a retry loop)

Empirically re-confirmed by starting Dex-B **before** Dex-A:

```
time=... level=ERROR msg="server: Failed to open connector" id=dex-a
  err="failed to open connector: failed to create connector dex-a: failed to get provider:
  Get \"http://dex-a:5556/dex/.well-known/openid-configuration\": dial tcp: lookup dex-a on
  10.89.2.1:53: no such host"
failed to initialize server: server: failed to open all connectors (1/1)
```

The process **exits immediately** (exit code 2) -- this is not a retry/backoff
loop inside Dex itself. Starting Dex-A and then simply restarting the same
(unmodified) Dex-B container succeeds immediately:

```
time=... level=INFO msg="listening on" server=http address=0.0.0.0:5557
```

**Implication for E2E infra**: this is safe to rely on in a Kubernetes
Deployment (kubelet's own crash-loop-backoff restart naturally retries Dex-B
until Dex-A's Service is resolvable), but a plain podman/docker-compose or
`SynchronizedBeforeSuite` setup must **explicitly sequence** Dex-A readiness
before creating Dex-B -- there is no self-healing retry to lean on outside of
a container orchestrator's restart policy. This is **not** a true circular
dependency (Dex-A never needs to reach Dex-B to start); it is a strict
one-directional ordering constraint.

### Finding 2: Machine (`client_credentials`) tokens require claim-mapping overrides on the exchanger's connector

Dex's OIDC connector is designed around human-user login flows and expects a
`name` claim in the upstream userinfo/ID-token response. A `client_credentials`
token from Dex-A carries no such claim
(`{"iss":"...","sub":"...","aud":"kubernaut-fleet-read","groups":["mcp-read"]}`
-- confirmed via `GET /dex/userinfo` with the access token). Attempting the
exchange without `userNameKey: sub` + `claimMapping.email: sub` on Dex-B's
connector fails:

```
$ curl -u kube-mcp-server:*** http://localhost:15557/dex/token \
    --data-urlencode connector_id=dex-a \
    --data-urlencode grant_type=urn:ietf:params:oauth:grant-type:token-exchange \
    ... --data-urlencode subject_token=$DEX_A_TOKEN ...
{"error":"access_denied"}   # HTTP 401

# dex-b log:
level=ERROR msg="failed to verify subject token" err="missing \"name\" claim"
```

Adding `userNameKey: sub` and `claimMapping.email: sub` to Dex-B's connector
config (mapping the machine token's `sub` claim into the slots Dex's
human-oriented connector code requires) resolves this and produces a
successful exchange (`200 OK`):

```json
{
  "access_token": "<JWT>",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "token_type": "bearer",
  "expires_in": 86399
}
```

Decoded exchanged-token claims:

```json
{
  "iss": "http://dex-b:5557/dex",
  "sub": "Ch5DaFJyZFdKbGNtNWhkWFF0Wm14bFpYUXRjbVZoWkESBWRleC1h",
  "aud": "kube-mcp-server",
  "federated_claims": { "connector_id": "dex-a", "user_id": "<base64 of dex-a's sub>" }
}
```

Note the exchanged token does **not** carry the original `groups` claim
forward automatically (unlike Keycloak's exchange, which preserves the
original client's identity via `azp` -- Spike S17 Finding 1). If any
downstream authorization decision depends on `groups`, that mapping would
need to be reconstructed from `federated_claims` or added via a Dex
connector/claim-mapping extension -- not validated further in this spike
(out of scope; this spike's downstream consumer, `kube-mcp-server`, does not
reach this point at all -- see Part 2).

## Part 2: kube-mcp-server STS Compatibility (new finding -- BLOCKING)

Spike S17/S18 validated `kube-mcp-server`'s STS wiring against Keycloak only,
explicitly deferring Dex validation ("Open Items" in the prior draft of this
spike). This session closes that gap, with a decisive negative result.

### `kube-mcp-server`'s STS path is not Dex-compatible

`test/infrastructure/fleet_e2e.go` documents that `kube-mcp-server`'s FMC/fleet
E2E config deliberately leaves `token_exchange_strategy` **unset**, routing
through the generic `pkg/kubernetes/sts.go` path built on
`golang.org/x/oauth2/google/externalaccount`'s `stsexchange.ExchangeToken`
(chosen specifically because the alternative pluggable `keycloak-v1` strategy
has a known bug: it never sets `subject_token_type`, Spike S18).

Reading `stsexchange.ExchangeToken`
(`golang.org/x/oauth2@v0.36.0/google/internal/stsexchange/sts_exchange.go`),
the exact wire request it sends is fixed and non-extensible:

```go
data.Set("audience", request.Audience)
data.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
data.Set("requested_token_type", "urn:ietf:params:oauth:token-type:access_token")
data.Set("subject_token_type", request.SubjectTokenType)
data.Set("subject_token", request.SubjectToken)
data.Set("scope", strings.Join(request.Scope, " ")) // always sent, even empty
// + client_id/client_secret via Basic auth or body params
```

**There is no `connector_id` field anywhere in this request, the
`TokenExchangeRequest` struct, or the optional `options` JSON blob** (which is
sent as a single opaque `options` form value, not spread into individual
params -- Dex's handler reads `connector_id` as its own top-level form field,
so `options` cannot smuggle it through).

Per Dex's own documentation and the RFC 8693 enhancement proposal
(`dexidp/dex` docs), **`connector_id` is a mandatory, Dex-specific extension
parameter** with no default/fallback even when exactly one connector is
configured (confirmed both by live testing and by Dex's own enhancement doc
listing "the `audience` field could be made optional if there is a single
connector" as unimplemented future work -- there is no equivalent behavior
for `connector_id` today).

**Live confirmation** -- replaying the exact request shape
`stsexchange.ExchangeToken` sends (no `connector_id`, empty `scope`, Basic
auth) against Dex-B:

```
$ curl -u kube-mcp-server:*** http://localhost:15557/dex/token \
    --data-urlencode audience=kube-mcp-server \
    --data-urlencode grant_type=urn:ietf:params:oauth:grant-type:token-exchange \
    --data-urlencode requested_token_type=urn:ietf:params:oauth:token-type:access_token \
    --data-urlencode subject_token_type=urn:ietf:params:oauth:token-type:access_token \
    --data-urlencode subject_token=$DEX_A_TOKEN \
    --data-urlencode scope=""

{"error":"invalid_request","error_description":"Requested connector does not exist."}
HTTP 400
```

**Binary-level confirmation**: extracting the `kube-mcp-server:latest` binary
and searching its embedded strings for every `sts_*`/`token_exchange*`
literal turns up `sts_client_id`, `sts_client_secret`, `sts_audience`,
`sts_scopes`, `sts_auth_style`, `sts_client_cert_file`, `sts_client_key_file`,
`sts_federated_token_file`, `token_exchange_strategy`, and exactly one
strategy name: `keycloak-v1`. **No `connector_id`, `dex-v1`, or any other
strategy string exists in the binary.** This is not a config-only gap --
there is no code path in the current `kube-mcp-server` release that can ever
send a `connector_id` parameter, whether through the default generic path or
the `keycloak-v1` pluggable strategy.

### Conclusion: 2x Dex + `kube-mcp-server`'s real STS is not viable today, without a kube-mcp-server code change

This directly falsifies the assumption (carried over from the original
2026-07-01/02 session and the initial version of this spike/plan) that the
Dex-to-Dex topology validated in Part 1 is a drop-in replacement for
Keycloak in the `fleet` E2E `passthrough`+exchange lane. The token-exchange
*protocol* (RFC 8693 shape, response format) is compatible -- Dex-B's
response (`access_token`/`issued_token_type`/`token_type`/`expires_in`) is
exactly what `stsexchange.Response` expects to unmarshal -- but the
*request* `kube-mcp-server` sends can never satisfy Dex's mandatory
`connector_id` requirement without an upstream code change to
`kube-mcp-server` (e.g. a new `dex-v1` strategy, or a generic
`sts_extra_params` config surface) that we do not control from this repo.

## Cleanup

All spike containers (`dex-a`, `dex-b`), the `dex-spike-net` podman network,
and pulled images (`ghcr.io/dexidp/dex:latest`,
`ghcr.io/containers/kubernetes-mcp-server:latest`) were removed after
validation. No persistent infrastructure was created or left running.

## Confidence Assessment: 92%

| Aspect | Confidence | Evidence |
|--------|-----------|----------|
| Two-Dex topology is required; self-exchange doesn't work | 95% | Reproduced the connector-based design constraint from Dex's own docs; consistent with the original 2026-07-01/02 finding |
| Startup ordering (Dex-A before Dex-B, fail-fast not retry-loop) | 95% | Live-reproduced with logs and exit codes in this session |
| Machine-token exchange requires `userNameKey`/`claimMapping` overrides | 95% | Live-reproduced both the failure (`missing "name" claim`) and the fix in this session |
| `kube-mcp-server`'s current STS path cannot exchange against Dex (`connector_id` gap) | 90% | Live-reproduced the exact failure by replaying the library's precise request shape, cross-checked against the binary's embedded strings (no `connector_id`/`dex-v1` support exists) -- not a full run of the real binary end-to-end against a live K8s API server, so a narrow chance some untested code path (e.g. a config knob not visible in strings output) differs |

## Recommendation

Do **not** proceed with the "switch `fleet` E2E's `passthrough`+exchange lane
to 2x Dex" plan as originally scoped -- it cannot work against
`kube-mcp-server`'s current release. Options, in order of preference pending
user decision:

1. **Keep Keycloak for the `passthrough`+exchange lane** (mirror FMC exactly,
   per Spike S17/S18/S19), accepting the ~1.25-2GB memory cost documented
   there, and reserve 2x Dex for lanes that only need plain OIDC/JWT
   validation (no token exchange) -- e.g. the existing `fleet`/`kubernautagent`
   password and client-credentials grants, which have no `connector_id`
   dependency.
2. **File an upstream `kube-mcp-server` issue/PR** adding a `dex-v1` STS
   strategy (or a generic extra-params passthrough) that sends `connector_id`,
   then revisit this plan once merged and released -- out of this repo's
   control and timeline.
3. **Re-scope the `fleet` E2E remote-cluster plan** to keep the existing
   `kubeconfig` auth mode (no token exchange) for the remote cluster, since
   the wiring-gap bug (AF/EM not consuming `FleetReaderFactory`/
   `ClusterRegistry`) this whole effort originated from is orthogonal to
   which auth mode is used -- `kubeconfig` mode alone is sufficient to
   reproduce and fix it.

## Authority

Issue #54, ADR-068, Spike S17, Spike S18, Spike S19, DD-TEST-013.
