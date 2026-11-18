# DD-FLEET-011: Shared Target-Resource Read Policy

**Status**: Accepted MVP direction; implementation and qualification pending
**Decision Date**: 2026-11-10
**Issues**: [#2593](https://github.com/jordigilh/kubernaut/issues/2593), [#2599](https://github.com/jordigilh/kubernaut/issues/2599)
**Related**: ADR-068, DD-AF-018, BR-SCOPE-001, BR-SECURITY-016, [kubernaut-operator#623](https://github.com/jordigilh/kubernaut-operator/issues/623)
**Applies To**: Gateway (GW), Remediation Orchestrator (RO), Fleet Metadata Cache (FMC), and the operator-managed target-resource read configuration

## Context

GW validates signal targets, RO performs managed-scope checks, and FMC provides
fleet metadata. The Console's inventory display is a separate concern from what
GW can access, but the services need a consistent administrator-configured
target-resource read boundary. Dynamic CRDs and administrator-supplied role
extensions must not require a second, independently maintained inventory
allowlist.

Today these services do not all receive the same operator-managed reads.
The operator binds `spec.additionalClusterRoles` to GW, Kubernaut Agent and
Effectiveness Monitor, not RO or FMC. RO has its own fixed target reads; FMC's
local role primarily supports fleet registration discovery. Operator PR #625
added a separate API Frontend (AF) inventory-read projection from GW's target
reads and configured additional roles. These observations describe existing
operator behavior, not an implemented GW/RO/FMC parity contract.

## Decision

1. For the MVP, the operator will define and maintain a **shared
   target-resource read configuration** for GW, RO and FMC, derived from the
   same default target-read rules and administrator-configured
   `spec.additionalClusterRoles` extensions. Apply it to the identity that
   actually reads target Kubernetes resources where that backend uses
   Kubernetes RBAC; do not assume FMC's local ServiceAccount is that identity.
2. Each service retains its own identity and separate operational permissions.
   Sharing target reads does not copy GW's RemediationRequest, Lease, review,
   write or other unrelated operational permissions to RO or FMC. Additional
   roles are projected to the applicable target reads, not blindly bound as
   full write-capable roles to every service.
3. Preserve administrator-granted boundaries, including API groups, resources,
   verbs, resource names and intentional wildcards. Secrets are not part of
   GW's default target reads; an administrator may explicitly grant sensitive
   resource reads. A read grant can authorize access to full Kubernetes objects
   even if an inventory response contains metadata only. Inventory must reject
   subresource requests and must not expose raw object contents.
4. The operator-managed configuration is not proof that the services' effective
   visibility is identical. Verify the actual principal, authorization and
   visibility separately for each backend and cluster. For ACM Search, the
   configured bearer token identifies the Search principal: FMC's local
   Kubernetes target roles do not authorize that query. In MCP mode, the
   identity that reaches the target Kubernetes API needs the relevant reads;
   authentication or token exchange may make it different from FMC's local
   ServiceAccount. Local role parity does not automatically grant either
   principal equivalent hub or spoke access.
5. Inventory capability and managed scope do not grant remediation or approval
   authority. Preserve the independent RemediationApprovalRequest authorization
   boundary and the existing fail-closed ACM inventory gate until the approved
   production path is qualified.

This decision records target-read parity and the MVP revocation target. It does
not approve a particular FMC/ACM adapter, source-side API, policy-delivery
implementation, AF inventory authorization change, or implementation plan.
Gateway access and what the Console displays remain distinct concerns.

### Asynchronous revocation and backend identities

The MVP uses an approximate **30-second range** for asynchronous inventory
revocation, not a hard end-to-end deadline. Existing in-flight requests and
cursors need not be synchronously invalidated. Qualify that relevant cached
entries expire after refresh stops and that loss of source access actually stops
refresh. Backend propagation delay and the cache TTL can accumulate; neither a
30-second TTL nor an observed delay establishes a hard revocation bound.

In ACM mode, changing only FMC's local Kubernetes role does not change the ACM
Search principal's access. Administrators configure that principal's Search
permissions; an empty Search response is not proof of complete visibility or
absence. In MCP mode, qualify the permissions of the actual target-cluster
caller. A separate operator-published policy or FMC-side filter is not assumed:
either would be an additional policy layer requiring a separate decision.
The exact cache and backend implementation remain subject to architecture and
plan approval; keep the inventory gate closed until qualified.

An ACM 2.17.4 SNO spike removed one temporary principal's `local-cluster` view
binding after a known object was visible. Search still returned it immediately
and first returned an empty result 31.06 seconds after removal. This single
observation does not establish an upper bound or test FMC cache expiration.

## Alternatives Considered

- **Independent service role lists**: preserves current isolation but invites
  configuration drift and requires repeated CRD extensions. Rejected for the MVP.
- **Copy every GW permission and full additional role to all services**:
  simpler binding, but can spread operational or write privileges. Rejected in
  favor of a shared target-read projection and separate service identities.
- **Derive Console inventory policy from ACM Search visibility alone**:
  rejected because Search visibility and the intended target-read policy are
  distinct; successful or empty Search results do not prove parity.

## Implementation and Verification Gates

- Obtain approval of the concrete architecture and TDD implementation plan;
  coordinate operator changes and document how role updates, deletion, missing
  roles, name restrictions, wildcards and revocation are handled.
- Prove the shared default and extension read contracts for GW, RO and FMC,
  while proving operational/write privileges are not copied. Test dynamic CRDs,
  explicitly approved sensitive reads, namespace and name boundaries, and
  wildcard/subresource handling. Keep the existing AF projection consistent.
- Prove production wiring and authorization using the applicable backend
  identities, including ACM hub/spoke visibility where claimed. Cover applicable
  FedRAMP AC-4, AC-6 and SI-10, relevant SOC2 objectives and OWASP ASVS access
  control, validation and logging objectives; do not claim certification.
- Use the project's requirements-based test pyramid: at least 90% of meaningful
  in-scope unit scenarios and all critical scenarios must pass, with integration
  proof for every wiring point and applicable end-to-end control objectives.

## Open Questions

- How the operator distributes and revokes the projected policy and how each
  backend verifies its actual principal's visibility.
- How remote cluster access and discovery map dynamic CRDs and name-restricted
  grants without silently broadening the intended policy.
- How FMC obtains and serves bounded, sanitized inventory in ACM and other
  modes, including incomplete source and pagination behavior.
