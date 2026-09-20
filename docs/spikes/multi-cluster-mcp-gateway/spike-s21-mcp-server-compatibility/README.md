# Spike S21: User-Provided Kubernetes MCP Server Compatibility

**Date**: 2026-09-20
**Status**: Closed / Parked
**Outcome**: Keep the existing Kubernetes MCP server contract; defer a generic compatibility layer until a concrete user requirement exists.
**Authority**: ADR-068, DD-FLEET-005, BR-FLEET-054, BR-INTEGRATION-054, BR-INTEGRATION-065

## Objective

Assess whether Kubernaut should become agnostic to the Kubernetes MCP server used behind the
fleet MCP Gateway, including whether dynamic discovery or a pretrained transformer could map
arbitrary MCP tools to the non-LLM operations used by WorkflowExecution (WE) and
EffectivenessMonitor (EM).

The motivating concern is that users may eventually want to provide their own Kubernetes MCP
server instead of the `containers/kubernetes-mcp-server` implementation used by Kubernaut.

## Executive Summary

Kubernaut is gateway-agnostic but not Kubernetes-MCP-server-agnostic. Envoy AI Gateway and
Kuadrant prefixes are abstracted, while non-LLM Kubernetes operations depend on the current
server's tool names, request fields, response shapes, and error behavior.

Several open source Kubernetes MCP servers expose equivalent high-level capabilities, but their
wire contracts differ substantially. Tool discovery or semantic classification can identify
likely operations, but it cannot safely derive argument translation, result decoding, Kubernetes
error normalization, or write authorization.

There is no demonstrated user requirement that justifies adding and maintaining a generic
compatibility framework now. A user-provided server can instead satisfy the existing Kubernaut
wire contract directly or through an adapter deployed behind the MCP Gateway.

**Decision**: preserve and document the current contract. Reopen this topic only when a concrete
user, server, and incompatibility are available to drive the design.

## Current Production Contract

The canonical tool names are defined in `pkg/fleet/mcpclient/tool_names.go` and were validated
against the real server in Spike S13:

| Logical operation | MCP tool | Required arguments | Required result behavior |
|---|---|---|---|
| Get | `resources_get` | `apiVersion`, `kind`, `name`, optional `namespace` | Full Kubernetes object in supported structured content or JSON/YAML text |
| List | `resources_list` | `apiVersion`, `kind`, optional `namespace`, optional `labelSelector` | Full Kubernetes objects, including metadata, spec, and status |
| Create or update | `resources_create_or_update` | `resource` containing a complete JSON/YAML manifest | Apply the supplied resource and report MCP/application errors |
| Delete | `resources_delete` | `apiVersion`, `kind`, `name`, optional `namespace` | Delete the resource and expose not-found/authorization failures |

Gateway-specific prefixes are separate from this backend contract. For example, Envoy AI
Gateway may expose `prod_east__resources_get`, while Kuadrant may publish a configured prefix.
The base operation remains `resources_get`.

### WE Minimum Surface

WE uses the contract to:

- Create Jobs and Tekton PipelineRuns.
- Get Job and PipelineRun status objects.
- Get or list supporting objects such as TaskRuns, Pods, and Events.
- Delete Jobs and PipelineRuns during cleanup.
- Recognize Kubernetes outcomes such as not found, forbidden, and creation collisions.

The effective minimum is Get, List, Create/Update, and Delete. Status polling is a Get followed by
inspection of the Kubernetes object's `.status`; it does not require a separate MCP status tool.

### EM Minimum Surface

EM uses Get and List to:

- Fetch arbitrary remediation targets and their full specs/status.
- List Pods with namespace and label selectors.
- Read Pod health, conditions, timestamps, and deletion state.
- Fetch referenced ConfigMaps for post-remediation hash comparison.

A lossy table or name-only list is insufficient for EM assessment.

## Existing Coupling

The dependency is broader than the four names:

| Concern | Current location |
|---|---|
| Tool names | `pkg/fleet/mcpclient/tool_names.go` |
| Get/List argument encoding | `pkg/fleet/mcpclient/client.go` |
| Create/Delete argument encoding | `pkg/fleet/mcpclient/writer.go` |
| Object/list response parsing | `pkg/fleet/mcpclient/client.go`, `pkg/fleet/mcpclient/parse.go` |
| Kubernetes error reconstruction | `pkg/fleet/mcpclient/notfound.go`, `pkg/fleet/mcpclient/writer.go` |
| Prefix discovery | `pkg/fleet/mcpclient/discover.go` |
| WE client composition | `pkg/workflowexecution/executor/client_factory.go` |
| EM remote reader wiring | `cmd/effectivenessmonitor/main.go` |
| Gateway read/write policies | `deploy/mcp-gateway/02-auth-policy.yaml` |

Consequently, replacing only the tool-name constants would support servers that renamed the tools
but retained every other detail. It would not provide general compatibility.

## Open Source Alternatives Reviewed

This was a source-level compatibility review. The alternatives were not deployed through the
Kubernaut fleet Gateway or exercised against a live cluster in this spike.

| Server | Compatibility finding | Disposition |
|---|---|---|
| `containers/kubernetes-mcp-server` | Exact current contract and supported transports | Keep as canonical backend |
| `openshift/openshift-mcp-server` | Same core resource contract and implementation lineage | Compatible, but not independent portability proof |
| `Flux159/mcp-server-kubernetes` | Has get, create/apply, delete, HTTP transport, and inline manifests; uses different arguments and text output | Best independent candidate if a future adapter is required |
| `reza-gholizade/k8s-mcp-server` | Promising native API implementation with CRUD tools and HTTP transports; exact create/error semantics require live validation | Re-evaluate if requested by a user |
| `skyhook-io/radar` | Mature native implementation with strong apply/safety behavior; no generic delete was found in the reviewed tool surface | Does not currently satisfy WE cleanup contract |
| `Azure/mcp-kubernetes` | Primarily a generic `call_kubectl` command interface and stdio transport; no safe inline manifest contract for WE | Not suitable as a direct Fleet backend |
| `strowk/mcp-k8s-go` | Stdio-only, no generic delete, lossy list behavior, and apply semantics that do not match WE lifecycle requirements | Not suitable without upstream changes/fork |
| `alexei-led/k8s-mcp-server` | Command-oriented and archived | Do not target |

### Why Flux159 Was the Closest Independent Alternative

Flux159 exposes the necessary high-level actions and supports Streamable HTTP. A compatibility
profile would still need to:

- Map `kind` and `apiVersion` to kubectl resource names.
- Map Kubernaut's manifest field to the server's inline manifest input.
- Keep Create distinct from Apply so WE locking/collision behavior is not weakened.
- Force and parse full JSON/YAML output rather than lossy summaries.
- Normalize text/process failures into typed Kubernetes errors.
- Add least-privilege Tekton RBAC and exclude generic command, exec, Helm, node, and context tools.

This confirms that a server adapter is an executable protocol implementation, not a name alias.

## Dynamic Discovery Findings

MCP `tools/list` provides callable tool descriptors. Depending on protocol/server versions, a
descriptor may include a name, description, input schema, output schema, annotations, and
server-specific metadata.

MCP does not define a standard Kubernetes operation taxonomy. It does not state that an
arbitrary tool means Kubernetes Get, List, Create, Apply, or Delete. Generic annotations such as
read-only or destructive are advisory and are not sufficient for authorization.

Dynamic discovery is useful for validating a selected server contract, but it cannot establish
that contract safely by itself.

## Transformer-Assisted Classification Findings

A pretrained bidirectional transformer could rank discovered tools against canonical capability
descriptions. This could reduce manual work when proposing an adapter, especially when names vary.

It would not determine:

- How to encode GVK, namespace, selectors, or manifests.
- Whether a write tool performs atomic create, update, or server-side apply.
- Whether a list response preserves full Kubernetes objects.
- How the server represents not found, forbidden, conflict, and timeout errors.
- Whether a generic command tool is safe for a particular invocation.
- Which concrete tool names must be placed in gateway read/write authorization policies.

Remote tool metadata is untrusted input. A probabilistic classifier must therefore not authorize
or select remediation writes at runtime. If this topic is reopened, a model may propose bindings,
but deterministic validation and explicit approval must precede use by WE.

No model dependency, training corpus, inference service, or runtime classifier will be added as a
result of this spike.

## Alternatives Considered

### A. Infer Operations Dynamically at Runtime

Use names, descriptions, schemas, annotations, heuristics, or a transformer to choose arbitrary
tools for every controller call.

**Rejected for now.** This is probabilistic at the write boundary and does not solve request,
response, error, or authorization translation.

### B. Build Versioned Backend Profiles and Adapters Now

Define logical Kubernetes operations with per-server tool bindings, request encoders, response
decoders, error classifiers, and authorization classes.

**Technically viable but deferred.** The codebase has a suitable seam in `pkg/fleet/mcpclient`,
and existing WE/EM interfaces could remain unchanged. There is no concrete consumer or alternate
server requirement to justify the configuration, operator/Helm surface, tests, and maintenance
cost today.

### C. Require the Existing Kubernaut MCP Contract

Document the four-tool contract and require a user-provided MCP backend to expose it directly or
through an adapter/gateway translation layer.

**Selected.** This is deterministic, preserves the tested safety model, and allows users with a
real requirement to integrate another implementation without adding speculative complexity to
Kubernaut.

## Decision

1. Kubernaut continues to support `containers/kubernetes-mcp-server` as the canonical backend.
2. `openshift/openshift-mcp-server` is compatible where its core contract remains aligned.
3. Kubernaut does not promise compatibility with arbitrary Kubernetes MCP servers.
4. A user-provided backend must expose the current four-tool API contract, either natively or via
   a compatibility adapter in front of that server.
5. Kubernaut will not add dynamic semantic inference, a generic transformation DSL, provider
   profiles, or transformer inference until an actual user requirement demonstrates the needed
   variation.
6. The current contract remains enforceable through startup/readiness validation if such
   validation is added independently; this spike does not implement it.

## Reopen Criteria

Reopen this decision when at least one of the following is true:

- A user identifies a specific alternate MCP server they need to operate with Kubernaut.
- The alternate server cannot expose the current four-tool contract through configuration or a
  small external adapter.
- The canonical server becomes unavailable, unmaintained, or operationally unsuitable.
- Multiple independent servers adopt a stable Kubernetes MCP capability standard.
- Maintaining external adapters becomes common enough to justify a first-class profile API.

The reopened investigation must begin with the real server's `tools/list` output, deployment and
authentication model, and live WE/EM lifecycle tests. It must not design solely from hypothetical
tool names.

## Future Design Constraints

If a first-class compatibility layer is eventually required, it should:

- Keep logical operations independent from wire tool names.
- Version profiles and pin them to a tested server contract.
- Validate all required tools and schemas before reporting ready.
- Separate Create from Apply/Update even when one backend tool implements multiple operations.
- Normalize responses and Kubernetes errors behind `pkg/fleet/mcpclient`.
- Derive application dispatch and gateway read/write authorization from the same approved profile.
- Fail closed on missing, ambiguous, or changed write capabilities.
- Treat model output as a proposal, never as runtime write authorization.
- Preserve the existing `client.Reader` and WE `ExecutorClient` boundaries where practical.

## Scope and Non-Goals

This spike makes no production changes. It does not add:

- New code or dependencies.
- New Helm or operator configuration.
- A custom resource for MCP profiles.
- A transformation language.
- A classifier or model runtime.
- Support claims for any independently implemented alternative server.

## References

- [ADR-068: Fleet Federation Architecture](../../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
- [DD-FLEET-005: Cluster-Transparent Tool Exposure](../../../architecture/decisions/DD-FLEET-005-cluster-transparent-tool-exposure.md)
- [Spike S11: WE Remote Execution](../spike-s11-we-remote-execution/README.md)
- [Spike S13: Empirical Tool Coverage Validation](../spike-s13-tool-coverage-validation/README.md)
- [Spike S15: FMC Multi-Format Response Parsing](../spike-s15-fmc-multiformat-parse/README.md)
- [Spike S16: Structured Content Validation](../spike-s16-structured-content-validation/README.md)
- [`containers/kubernetes-mcp-server`](https://github.com/containers/kubernetes-mcp-server)
- [`openshift/openshift-mcp-server`](https://github.com/openshift/openshift-mcp-server)
- [`Flux159/mcp-server-kubernetes`](https://github.com/Flux159/mcp-server-kubernetes)
- [`reza-gholizade/k8s-mcp-server`](https://github.com/reza-gholizade/k8s-mcp-server)
- [`skyhook-io/radar`](https://github.com/skyhook-io/radar)
- [`Azure/mcp-kubernetes`](https://github.com/Azure/mcp-kubernetes)
- [`strowk/mcp-k8s-go`](https://github.com/strowk/mcp-k8s-go)
- [`alexei-led/k8s-mcp-server`](https://github.com/alexei-led/k8s-mcp-server)

## Confidence Assessment: 97%

The current WE/EM call paths and canonical server contract were traced directly in the codebase.
Alternative-server conclusions are based on source-level reviews of their published tool and
transport implementations. The unresolved portion is intentionally deferred: no independent
alternative was deployed through Kubernaut's Gateway for live lifecycle testing because no user
requirement currently selects one.

## Closure

This spike is closed and parked. There is no implementation follow-up. The documented current
contract is the compatibility boundary until the reopen criteria are met.
