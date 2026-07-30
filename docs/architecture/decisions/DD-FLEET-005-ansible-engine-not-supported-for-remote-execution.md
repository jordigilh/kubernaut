# DD-FLEET-005: Ansible/AAP Engine Not Supported for Fleet (Remote) WorkflowExecution

## Status

**Approved**: 2026-07-30
**Confidence**: 97% (raised from 95% on 2026-07-30 after Phase 3 closed the one
open question about whether a machine-to-machine AAP auth path could avoid Option 1's
gateway-credential blocker — see Validation Results Phase 3)
**Milestone**: v1.6
**Related Issue**: #1761

## Context & Problem

`AnsibleExecutor` (`pkg/workflowexecution/executor/ansible.go`) has zero `ClusterID`
awareness. Unlike `JobExecutor`/`TektonExecutor`, which route through
`ClientFactory.ClientFor(ctx, wfe.Spec.ClusterID)` to reach a remote cluster via the
fleet MCP Gateway, `AnsibleExecutor` unconditionally calls the hub's own local AWX/AAP
instance via `AWXHTTPClient`, and `injectK8sCredential` unconditionally injects the
**hub's own** in-cluster K8s credentials (`ReadInClusterCredentials()`) as the AWX
credential used by `kubernetes.core` playbook modules.

The practical effect: a `WorkflowExecution` with a non-empty `Spec.ClusterID` (targeting
a remote/managed cluster) using the `ansible` engine does not fail — it silently
executes the playbook against the **hub cluster**, using the hub's own K8s API
credentials, while the operator believes it targeted the remote cluster. Issue #1761
flagged this as a correctness/safety bug and proposed, as a safe minimum fallback,
failing closed on any non-empty `ClusterID` if a real remote-execution path proved
infeasible.

ADR-068 (Decision #9, and Alternative H) had already designed for a fix: register the
AAP MCP Server as a `Backend`/`MCPServerRegistration` behind the same unified MCP
Gateway used for K8s remote access, eliminating the need for AAP-specific credential
handling. This was never implemented. Two candidate implementations of that fix — plus
one CRD-based alternative discovered mid-spike — were evaluated against **real, live
infrastructure** (not documentation or assumptions).

## Alternatives Considered

### Option 1: AAP MCP Server as a fleet backend (the ADR-068-designed approach)

**Approach**: Register the AAP MCP Server (GA since AAP 2.7, June 2026) as a second
backend behind the same Kuadrant/EAIGW gateway already used for K8s remote access, per
ADR-068 Decision #9. `AnsibleExecutor` would call `job_management` toolset tools
(`job_templates_launch_create`, `jobs_retrieve`, `jobs_cancel_create`, etc.) through the
gateway instead of raw AWX REST.

**Pros**:
- Matches the already-approved ADR-068 architecture exactly — no new design needed
- GA component, not experimental — 101 tools discovered correctly with proper
  `aap_cluster_`-style prefixing once registered
- Direct 1:1 tool mapping for the job-launch/status/cancel path (confirmed via live
  `tools/list` introspection against a real AAP 2.7 instance)

**Cons**:
- **No `credentials_destroy` tool is exposed by any default toolset in the deployed
  101-tool surface.** Confirmed by exhaustively checking every toolset's `tools/list`
  output. Kubernaut's ephemeral AWX credential cleanup (`cleanupEphemeralCredentials`,
  BR-WE-015) has no MCP equivalent today — ephemeral credentials would accumulate in AAP
  indefinitely. **Root-caused (2026-07-30) as a toolset-curation gap, not a capability
  gap**: the AAP Controller API underlying the MCP server does support
  `DELETE /api/v2/credentials/{id}/` (`credentials_destroy`, confirmed present in the
  OpenAPI schema bundled in
  [`ansible/aap-mcp-server`](https://github.com/ansible/aap-mcp-server)`/data/controller-schema.json`)
  — it is simply never included in any of the server's default toolset definitions
  (`aap-mcp.sample.yaml` includes `credential_types_destroy`, i.e. delete a credential
  *type*, but never `credentials_destroy`, delete a credential *instance*, in any
  toolset, including `security_compliance`). The server supports fully custom toolset
  configs, so this is technically addressable via configuration or an upstream PR — as
  of this writing, no issue or PR in that 4-issue repo tracks it. This does not change
  the verdict below: it does not resolve Option 1's second, independent blocker (gateway
  auth), and no fix has actually landed.
- **`MCPServerRegistration.credentialRef` is discovery-only, never injected into actual
  `tools/call` requests** — confirmed two ways against a real, live Kuadrant gateway
  (v0.7.1): (a) the CRD's own field docs state this explicitly; (b) empirically, calling
  a harmless read tool through the gateway returned `500: ... server returned 4xx for
  initialize POST`, traced to the broker forwarding the caller's Keycloak JWT (which AAP
  does not understand) instead of the registration's own credential. Every real
  `tools/call` to an AAP-backed tool fails authentication.
- ADR-068's own stated mitigation for this exact risk (Alternative H: *"the
  `MCPRoute.backendRefs[].securityPolicy` handles upstream credential injection"*) does
  not hold for the Kuadrant gateway this repo actually uses — confirmed live, not assumed.
- **Fixing the `credentialRef` injection bug alone would not be enough — confirmed
  2026-07-30 by inspecting AAP's own auth surface directly**, not by inference from
  Kuadrant's side. `GET /api/gateway/v1/authenticator_plugins/` against the live AAP 2.7
  instance lists all 11 supported authenticator plugin types (`azuread`, `github` +4
  variants, `google_oauth2`, `keycloak`, `ldap`, `local`, `oidc`, `radius`, `saml`,
  `tacacs`) — none support machine-to-machine JWT exchange. See
  [Root Cause](#root-cause-why-aap-cannot-accept-a-keycloak-issued-token-for-api-authorization)
  below for the full mechanism and why this isn't fixable by a PR to Kuadrant, the AAP
  MCP Server, or anything on kubernaut's side.

**Confidence in rejecting**: high — both blockers reproduced against real, live
components (real AAP 2.7, real Kuadrant gateway, real Keycloak token exchange), not
simulated.

#### Root Cause: Why AAP Cannot Accept a Keycloak-Issued Token for API Authorization

This is not a Kuadrant bug, not a kubernaut gap, and not something addressable by
contributing a PR to `ansible/aap-mcp-server` or the Kuadrant `mcp-gateway` project —
unlike the two toolset/injection issues above, it is architectural, on AAP's own side,
and independent of both. Fixing either or both of those two issues would not unblock
Option 1 without this also being fixed:

- AAP's platform gateway is a Django + Django REST Framework application using
  `django-oauth-toolkit` (DOT) as its OAuth2 provider. DOT's token model is
  **stateful and database-backed**: an issued access token is an opaque string
  persisted as an `AccessToken` row, linked by foreign key to a local Django `User`
  row. Authenticating a request means a **database lookup** — "does this token value
  exist, is it unexpired, which `User` does it point to?" — not cryptographic
  signature verification against an issuer's public key.
- Every downstream permission check, audit record, and ownership relationship in AAP
  (RBAC, `created_by`/`modified_by` attribution, Organization/Team membership) is a
  foreign key to that same local `User` row. A cryptographically valid Keycloak JWT
  has no such row behind it — AAP's authorization stack has nothing to attach it to.
- Creating that row (JIT-provisioning) is precisely what the interactive `keycloak`/
  `oidc` authenticator plugins' login pipeline does the first time a given external
  identity logs in (per Red Hat's own SSO docs: *"if this is your first time logging
  in with this SSO method, platform gateway creates a user linked to your... user"*).
  This is not a side effect of requiring a browser — it **is** the mechanism, and it
  has no headless equivalent: there is no supported API that performs this
  resolve-or-create step outside the interactive login view.
- Keycloak **is** already meaningfully delegated to for interactive users today — it
  handles authentication and can drive AAP Team/Role assignment via the
  `GROUPS_CLAIM` mapping at login time. What's missing is specifically a
  **stateless, per-call** path for headless/machine callers: nothing in AAP's request
  authentication stack validates a live, externally-issued JWT signature on an
  ordinary API call the way a JWT-bearer resource server would.
- **This is a design choice, not a limitation of OAuth2/OIDC itself.** Contrast with
  Kubernetes' own API server, which implements OIDC authentication in a genuinely
  stateless, claims-based way (`--oidc-issuer-url`): a JWT's `sub`/`groups` claims map
  directly to RBAC subjects, with **no persisted "User" object required anywhere**.
  This is exactly why Option 2 (creating `tower.ansible.com` CRDs through the
  K8s-native `kube-mcp-server`) worked cleanly in this same spike — the fleet
  identity resolved live to `keycloak:service-account-kubernaut-fleet-read` for RBAC
  purposes, no login ceremony, no local row required — while Option 1 (AAP's own API)
  structurally cannot work the same way, independent of any Kuadrant-side fix.
- **Practical consequence**: fixing Kuadrant's `credentialRef` injection bug alone
  would not unlock Option 1. Even a bug-free Kuadrant could only ever forward a
  static, pre-provisioned AAP credential (OAuth Application or Personal Access
  Token) configured on the `MCPServerRegistration` — never one dynamically derived
  from the caller's actual Keycloak identity — because AAP has no mechanism to accept
  the latter at all. That reintroduces the same per-cluster raw-credential-vending
  shape already rejected as Option 3, just relocated into a Kuadrant CRD field
  instead of kubernaut's own code.

---

### Option 2: `tower.ansible.com` Resource Operator CRDs via the existing K8s MCP server

**Approach**: Use the AAP Operator's bundled Resource Operator, which provides
`AnsibleJob`/`AnsibleCredential`/`JobTemplate` CRDs. `AnsibleExecutor` would create these
CRs through the **same generic K8s CRUD tools** (`resources_get/list/create_or_update/delete`)
already used by `JobExecutor`/`TektonExecutor` for remote clusters — no new gateway
plumbing, no new tool-prefix discovery.

**Pros**:
- Reuses proven `ClientFactory`/K8s-MCP plumbing verbatim — zero new code in
  `pkg/fleet/mcpclient`
- Job launch (`AnsibleJob`, GA component) and K8s-credential injection (via the
  dedicated, proper secret-ref field `AnsibleCredential.spec.kubernetes_bearer_token_secret`)
  both fully validated live through a real Kuadrant gateway with **zero blockers**
- Credential deletion works via ordinary `resources_delete` — the exact capability
  missing from Option 1

**Cons**:
- `AnsibleCredential` is explicitly marked "(Tech Preview)" in its own live CRD schema
  (`AnsibleJob`/`JobTemplate` carry no such marker)
- ~55x latency overhead per launch (runner-pod-per-job: pod scheduling + image pull +
  REST polling) vs. sub-second direct REST/MCP calls — measured on a trivial job
  template; real playbooks would differ but the mechanism-level overhead is structural
- **Fatal for generic secrets**: the only field available for non-K8s/non-SSH credential
  shapes — `spec.inputs`, exactly what arbitrary `dependencies.secrets` injection needs —
  is (a) a **plaintext string, not a secret-ref** (unlike every other field on this CRD),
  a real AC-4/AU-3 audit-trail regression since today's direct-REST/MCP flow never
  persists secret bytes in any object; and (b) **functionally broken, independent of the
  compliance concern**, per the upstream repo
  ([`github.com/ansible/awx-resource-operator`](https://github.com/ansible/awx-resource-operator),
  which its own README states is "maintained by the Red Hat Ansible Team" — the AAP
  Operator bundle ships this identical project, not a separately-patched fork):
  - [Issue #130](https://github.com/ansible/awx-resource-operator/issues/130) (open since
    2023-06-30, still open): the `inputs` field never reaches the underlying
    `awx.awx.credential` call. Root cause confirmed by a contributor:
    [PR #124](https://github.com/ansible/awx-resource-operator/pull/124) (merged
    2023-04-25) renamed the internal variable `inputs` -> `credential_inputs` everywhere
    except `roles/credential/templates/job_definition.yml.j2`, which still looks for the
    old name. The value is silently dropped.
  - [Issue #164](https://github.com/ansible/awx-resource-operator/issues/164) (open since
    2024-08-27): confirms this is systemic, not isolated — every credential type has its
    own hardcoded, narrow field allow-list (e.g. SSH only exposes `ssh_key_data`/
    `username`, not even `ssh_key_unlock`). There is no generic key-value passthrough
    anywhere in this operator.

**Confidence in rejecting the generic-secrets sub-case**: very high — reproduced findings
plus two independently-filed, still-open upstream issues spanning 2023-2024 with no fix
merged as of 2026-07.

---

### Option 3 (rejected pre-spike): Make `AWXHTTPClient` itself `ClusterID`-aware

**Approach**: Resolve a target cluster's raw host/bearer-token/CA and hand it directly to
today's AWX REST client instead of routing through MCP at all.

**Cons**: Requires inventing a raw-per-cluster-credential-vending mechanism that
contradicts ADR-068 Decision #9's explicit security posture (*"No per-cluster SA tokens
are maintained by Kubernaut services"*) and Alternative H's rejection of exactly this
shape of solution. No such mechanism exists anywhere in the codebase
(`pkg/fleet/registry.ClusterInfo` holds only `ID`/`MCPEndpoint`/`ToolPrefix`/`Labels`,
never credentials).

**Confidence in rejecting**: high — would be a larger, contradictory architectural change,
not a simpler one.

## Decision

**Neither remote-execution option is adopted.** The Ansible/AAP engine is **not
supported** for fleet (`Spec.ClusterID != ""`) `WorkflowExecution` in v1.6.
`AnsibleExecutor.Create` fails closed with a clear, actionable error whenever
`wfe.Spec.ClusterID != ""` — this is exactly the "safe minimum" fallback issue #1761
itself proposed if a real remote path proved infeasible. Local/hub execution via
`AWXHTTPClient` is completely unchanged — zero regression risk to the proven, working
path.

**Rationale**:

1. **A clean binary cut beats partial/conditional engine support.** Option 2's core
   capability (job launch + K8s-credential injection) has zero confirmed blockers, so a
   *scoped* remote-Ansible capability (support job launch + K8s-credential injection,
   fail closed only for workflows that also declare `dependencies.secrets`) was
   seriously considered. It was rejected: it would force operators to reason about which
   workflows are "remote-Ansible-safe" based on their secret-injection needs — a support
   burden and footgun risk judged worse than a single, predictable rule. An engine is
   either fully supported on a given `ClusterID`, or it is not; mixing engines per
   workflow capability need is the wrong UX to hand operators.
2. **Neither Red Hat-supported path is actually a distinct, better-maintained
   alternative to the other.** The Resource Operator shipped inside the AAP Operator
   bundle is the *identical* open-source `ansible/awx-resource-operator` project, not a
   separately-patched downstream fork — confirmed via the repo's own README and matching
   field-for-field CRD documentation across the community and Red Hat product docs. The
   AAP MCP Server (Option 1) is a genuinely distinct, newer component, but carries its
   own two independent, live-confirmed blockers.
3. **Both alternatives were validated against real, live infrastructure**, not
   documentation or assumptions — see [Validation Results](#validation-results).

## Implementation

- `AnsibleExecutor.Create` returns an error immediately when `wfe.Spec.ClusterID != ""`,
  before any AWX/AAP interaction — mirrors the existing `localClientFactory.ClientFor`
  fail-closed pattern in `pkg/workflowexecution/executor/client_factory.go:63-68` (used by
  non-fleet deployments for Job/Tekton), for consistency of error shape across engines.
- Error message must name the engine, the requested `ClusterID`, and point at this DD for
  the "why" (avoids a bare/unexplained rejection).
- No changes to `GetStatus`/`Cleanup` behavior beyond what naturally follows from `Create`
  never having launched a remote job (there is no remote `ExecutionRef` to poll or clean
  up).
- No changes to local/hub (`ClusterID == ""`) behavior anywhere in `ansible.go`.

## Consequences

**Positive**:
- Closes issue #1761's actual safety concern (silent wrong-cluster execution) with a
  minimal, low-risk code change instead of a multi-day implementation
- Zero regression to local/hub Ansible execution — today's proven `AWXHTTPClient` path is
  untouched
- Avoids taking a compliance-sensitive execution path (SOC2/FedRAMP AC-4/AU-3 in scope,
  per AGENTS.md) into production dependency on two Tech-Preview-or-worse components
- Avoids maintaining a partially-capable remote-Ansible code path that would need its own
  ongoing test/support burden for a fraction of full parity

**Negative**:
- Remote/fleet clusters cannot use the Ansible/AAP engine at all in v1.6 — workflows
  targeting a managed/remote cluster must use the Job or Tekton engine instead
- If a genuine business need for fleet-Ansible remediation emerges, it is blocked on
  upstream fixes outside kubernaut's control (see Review & Evolution)

**Neutral**:
- `docs/spikes/multi-cluster-mcp-write/spike-aap-mcp-server.md`'s previously-open item
  ("Gateway credential injection") is now closed with a definitive NO, not left
  unresolved

## Validation Results

Both alternatives were validated against real, live infrastructure across three spike
phases:

- **Phase 1**: A real `AnsibleAutomationPlatform` v2.7 aggregate CR was stood up on a live
  OCP dev cluster (controller + MCP enabled), subscription activated via the real
  Gateway API, and the combined `/mcp` endpoint's `tools/list` introspected directly
  (101 real tools across 6 toolsets). The `credentials_destroy` gap was found here, then
  root-caused (2026-07-30) against the upstream
  [`ansible/aap-mcp-server`](https://github.com/ansible/aap-mcp-server) repo's own bundled
  OpenAPI schema (`data/controller-schema.json`) and sample toolset config
  (`aap-mcp.sample.yaml`) — confirmed as a toolset-curation omission, not a missing AAP
  API capability (see Option 1 Cons above). Checked that repo's issue/PR history (4
  issues total, none related) — no existing tracking.
- **Phase 2**: Used this repo's own CI-validated `PRESERVE_E2E_CLUSTER=true make
  test-e2e-fleetmetadatacache-kuadrant` automation to stand up a real, isolated Kind
  cluster with a live Keycloak + Istio + Kuadrant MCP Gateway + `kube-mcp-server` stack.
  Option 2's `AnsibleJob`/`AnsibleCredential` CRDs (extracted from the real AAP 2.7
  install) were applied and exercised through the real gateway
  (`resources_create_or_update`/`resources_get`/`resources_delete`, all PASS). Option 1's
  AAP MCP Server was registered as a second live backend on the same gateway
  (`credentialRef`-based registration, discovery PASS, all `tools/call` FAIL with `500`).
- **Phase 3** (2026-07-30, after the Kind clusters were torn down): re-verified against
  the still-live OCP AAP 2.7 instance whether the Phase 2 gateway-auth blocker could be
  closed by any AAP-side mechanism rather than a Kuadrant fix. Queried
  `GET /api/gateway/v1/authenticator_plugins/` directly (all 11 plugin types + full
  configuration schemas) and `GET /api/gateway/v1/authenticators/` (confirmed only
  "Local Database Authenticator" configured, `ALLOW_OAUTH2_FOR_EXTERNAL_USERS: false` —
  no external IdP wired in at all today). Result: no plugin supports machine-to-machine
  JWT exchange; see Option 1 Cons for the full finding. This closes the residual
  uncertainty flagged when this DD was first drafted.
- Upstream evidence: [awx-resource-operator#130](https://github.com/ansible/awx-resource-operator/issues/130),
  [#164](https://github.com/ansible/awx-resource-operator/issues/164), and
  [PR #124](https://github.com/ansible/awx-resource-operator/pull/124) (root cause).

**Residual 5%**: (a) the dev cluster's loopback-only topology (`local-cluster` only)
still caps how conclusively genuinely-remote-cluster behavior for Option 1/2's *working*
parts was proven, though the Kind `fmc-e2e`/`fmc-e2e-remote` split is a closer
approximation than the OCP cluster alone; (b) upstream GitHub issues can theoretically be
fixed without kubernaut's knowledge between this decision and its next review.

## Related Decisions

- **Amends**: [ADR-068](ADR-068-fleet-federation-architecture.md) — Decision #9 and
  Alternative H's assumption that the AAP MCP Server would be registered as a fleet
  backend is retired *for the Ansible engine specifically*. The unified-gateway
  chokepoint principle itself is unaffected and remains correct for K8s MCP backends
  (Job/Tekton engines).
- **Resolves**: Issue #1761 (via fail-closed, not via full remote support)

## Review & Evolution

**When to Revisit**:
- If [awx-resource-operator#130](https://github.com/ansible/awx-resource-operator/issues/130)
  is fixed upstream **and** `AnsibleCredential` is promoted out of Tech Preview
- If **both** of the following land upstream — fixing either alone is not sufficient,
  confirmed 2026-07-30: (a) the Kuadrant `mcp-gateway` project adds per-call credential
  injection for non-K8s backends, **and** (b) AAP's platform gateway adds a
  machine-to-machine JWT-bearer/resource-server auth mode capable of validating an
  externally-federated identity per-call (no such mode exists in any of AAP's 11
  authenticator plugins today — all are interactive, redirect-based; see
  [Root Cause](#root-cause-why-aap-cannot-accept-a-keycloak-issued-token-for-api-authorization)
  for why this is an AAP-side architectural gap, not a Kuadrant one). A
  `credentials_destroy` tool would also still need to be added to a toolset, via custom
  config or an upstream PR to `ansible/aap-mcp-server` — a smaller, independently
  actionable fix, not blocked on either of the above.
- If a genuine, prioritized business requirement for fleet-Ansible remediation emerges —
  per direct guidance, escalate the gateway-auth and JWT-federation gaps to the
  Kuadrant/Ansible teams directly rather than re-attempting a kubernaut-side workaround,
  since these root causes are upstream defects/gaps, not architecture gaps on
  kubernaut's side

**Success Metrics**:
- `AnsibleExecutor.Create` returns a clear, actionable error (never a silent
  hub-execution) for any `wfe.Spec.ClusterID != ""`
- Zero behavior change for `wfe.Spec.ClusterID == ""`
