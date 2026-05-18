# Kagenti Integration: API Frontend Agent Discovery in OpenShift

**Version**: 1.0
**Last Updated**: 2026-05-02
**Status**: Planning
**Tracking Issue**: [#27](https://github.com/jordigilh/kubernaut-apifrontend/issues/27)

---

## Overview

[Kagenti](https://github.com/kagenti/kagenti) is a Kubernetes-native platform for deploying
and orchestrating AI agents. It provides agent discovery, lifecycle management, and
secure inter-agent communication via the [A2A protocol](https://a2aprotocol.ai/).

The Kubernaut API Frontend must integrate with kagenti so that it is **discoverable** as
an A2A-capable agent in OpenShift (OCP) clusters where kagenti is deployed. This document
captures what works out of the box, what gaps remain, and the decisions needed.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        OpenShift Cluster                            │
│                                                                     │
│  ┌──────────────────────────────────┐                               │
│  │     kagenti-system namespace     │                               │
│  │  ┌────────────────────────────┐  │                               │
│  │  │  kagenti-operator          │  │                               │
│  │  │  ├─ AgentCard Sync Ctrl    │──┐  watches Deployments with     │
│  │  │  ├─ AgentCard Controller   │  │  kagenti.io/type: agent       │
│  │  │  └─ NetworkPolicy Ctrl     │  │                               │
│  │  └────────────────────────────┘  │                               │
│  └──────────────────────────────────┘                               │
│                                      │                              │
│                                      │  creates AgentCard CR        │
│                                      ▼                              │
│  ┌──────────────────────────────────┐                               │
│  │     kubernaut namespace          │                               │
│  │                                  │                               │
│  │  ┌────────────────────────────┐  │                               │
│  │  │  kubernaut-apifrontend     │◄─── AgentCard Controller fetches │
│  │  │  (Deployment)             │     /.well-known/agent-card.json  │
│  │  │                            │                                  │
│  │  │  Labels:                   │                                  │
│  │  │    kagenti.io/type: agent  │                                  │
│  │  │    protocol.kagenti.io/a2a │                                  │
│  │  │    protocol.kagenti.io/mcp │                                  │
│  │  └────────────────────────────┘                                  │
│  └──────────────────────────────────┘                               │
│                                                                     │
│  ┌──────────────────────────────────┐                               │
│  │     gateway-system namespace     │  (optional)                   │
│  │  ┌────────────────────────────┐  │                               │
│  │  │  MCP Gateway (Envoy)      │  │  Routes MCP traffic via       │
│  │  │  + mcp-broker-controller  │  │  HTTPRoute + MCPServerReg     │
│  │  └────────────────────────────┘  │                               │
│  └──────────────────────────────────┘                               │
└─────────────────────────────────────────────────────────────────────┘
```

---

## What We Have (ADK provides out of the box)

Google ADK Go's A2A launcher (`cmd/launcher/web/a2a/a2a.go`) provides:

| Capability | Status | Details |
|------------|--------|---------|
| **Agent Card endpoint** | Working | Served at `/.well-known/agent-card.json` via `a2asrv.NewStaticAgentCardHandler()` |
| **Agent Card path** | Compatible | ADK uses v0.3.x path (`agent-card.json`), matches kagenti |
| **Skills auto-generation** | Partial | `adka2a.BuildAgentSkills()` extracts from registered ADK tools and sub-agents |
| **Streaming capability** | Working | `Capabilities.Streaming: true` set by default |
| **Transport protocol** | Working | `PreferredTransport: JSONRPC` (A2A standard) |
| **A2A JSON-RPC endpoint** | Working | Served at `/a2a/invoke` |
| **Input/Output modes** | Defaults | `["text/plain"]` — may need enrichment |

### ADK A2A Launcher Code Reference

```go
// From google/adk-go cmd/launcher/web/a2a/a2a.go
agentCard := &a2acore.AgentCard{
    Name:               rootAgent.Name(),
    Description:        rootAgent.Description(),
    DefaultInputModes:  []string{"text/plain"},
    DefaultOutputModes: []string{"text/plain"},
    URL:                publicURL,
    PreferredTransport: a2acore.TransportProtocolJSONRPC,
    Skills:             adka2a.BuildAgentSkills(rootAgent),
    Capabilities:       a2acore.AgentCapabilities{Streaming: true},
}
router.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(agentCard))
```

---

## What's Missing (Gaps)

### Gap 1: Agent Card Content Enrichment — [#28](https://github.com/jordigilh/kubernaut-apifrontend/issues/28)

**Severity**: Required for v1.5

ADK's auto-generated card is minimal. The API Frontend has 13 MCP tools (3 proxied + 10
native) across two data sources (K8s CRDs + Data Store). The agent card must:

- Enumerate all 13 skills with accurate descriptions
- Declare authentication requirements (OIDC/JWT)
- Use a cluster-resolvable URL (not `localhost`)
- Include provider metadata
- Support `application/json` input mode

**Recommendation**: Build the AgentCard manually rather than relying on ADK's auto-inference,
which may miss proxied tools that are not registered as local ADK tools.

---

### Gap 2: Kagenti Discovery Labels — [#29](https://github.com/jordigilh/kubernaut-apifrontend/issues/29)

**Severity**: Required for v1.5

Kagenti's AgentCard Sync Controller only watches workloads with specific labels:

```yaml
kagenti.io/type: agent              # Trigger for auto-discovery
protocol.kagenti.io/a2a: ""         # A2A protocol support
protocol.kagenti.io/mcp: ""         # MCP protocol support
kagenti.io/framework: google-adk    # Framework identifier
```

These must be on **both** the Deployment metadata and Pod template labels.

| Deployment Vector | Action |
|-------------------|--------|
| **kubernaut-operator** (production) | Operator must inject labels — track in operator repo |
| **Dev Helm chart** (#12) | Add labels to chart templates |

Without `kagenti.io/type: agent`, the API Frontend is invisible to kagenti.

---

### Gap 3: Agent Card JWS Signing — [#30](https://github.com/jordigilh/kubernaut-apifrontend/issues/30)

**Severity**: Decision needed (may be optional for v1.5)

Kagenti supports JWS-based cryptographic verification of Agent Cards:
- **Signed cards** → marked "verified" → permissive NetworkPolicy
- **Unsigned cards** → marked "unverified" → restrictive NetworkPolicy (or rejected in strict mode)

**Options**:
- **A**: No signing for v1.5 (simpler, works in permissive mode)
- **B**: JWS signing for v1.5 (full compatibility, requires key management)
- **C**: Feature-flagged signing (flexibility)

**Decision status**: Open — depends on target OCP cluster's kagenti configuration.

---

### Gap 4: SPIFFE Workload Identity Binding — [#31](https://github.com/jordigilh/kubernaut-apifrontend/issues/31)

**Severity**: Decision needed (depends on OCP environment)

Kagenti can bind Agent Cards to SPIFFE workload identities (SVIDs) for zero-trust
verification. This requires SPIRE to be running in the cluster.

**Decision status**: Open — depends on whether target OCP runs SPIRE.

If SPIRE is present:
- API Frontend needs a SPIRE CSI driver volume mount
- SPIFFE ID: `spiffe://<trust-domain>/ns/kubernaut/sa/kubernaut-apifrontend`
- Agent Card signature bound to SVID

If SPIRE is not present:
- Out of scope for v1.5
- Kagenti falls back to signature-only or permissive verification

---

### Gap 5: MCP Gateway Tool Registration — [#32](https://github.com/jordigilh/kubernaut-apifrontend/issues/32)

**Severity**: Decision needed (depends on OCP environment)

Kagenti includes an MCP Gateway (Kuadrant/mcp-gateway, Envoy-based) for cluster-wide
MCP tool discovery and routing. If present, the API Frontend's tools should be registered:

```yaml
# HTTPRoute — route MCP traffic to API Frontend
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: kubernaut-apifrontend-mcp
  labels:
    mcp-server: "true"
spec:
  parentRefs:
    - name: mcp-gateway
      namespace: gateway-system
  rules:
    - backendRefs:
        - name: kubernaut-apifrontend
          port: 8443

# MCPServerRegistration — register tools with gateway
apiVersion: mcp.kagenti.com/v1alpha1
kind: MCPServerRegistration
metadata:
  name: kubernaut-tools
spec:
  toolPrefix: kubernaut_
  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: kubernaut-apifrontend-mcp
```

**Decision status**: Open — depends on whether MCP Gateway is deployed in target OCP.

---

## Summary Matrix

| Component | ADK Provides | Gap | Issue | v1.5 Required? |
|-----------|-------------|-----|-------|----------------|
| Agent Card endpoint (`/.well-known/agent-card.json`) | Yes | None | — | — |
| Agent Card path (v0.3.x) | Yes | None | — | — |
| A2A JSON-RPC endpoint (`/a2a/invoke`) | Yes | None | — | — |
| Streaming support | Yes | None | — | — |
| Rich skills/auth/URL in card | Partial | Enrichment needed | #28 | Yes |
| Kagenti discovery labels | No | Must add | #29 | Yes |
| JWS signing | No | Decision needed | #30 | TBD |
| SPIFFE identity binding | No | Decision needed | #31 | TBD |
| MCP Gateway registration | No | Decision needed | #32 | TBD |

---

## Kagenti Label Quick Reference

```yaml
# Required labels for kagenti discovery
# Must be on Deployment metadata.labels AND spec.template.metadata.labels

kagenti.io/type: agent                          # Triggers AgentCard Sync Controller
protocol.kagenti.io/a2a: ""                     # Advertises A2A support
protocol.kagenti.io/mcp: ""                     # Advertises MCP support
kagenti.io/framework: google-adk                # Framework used
app.kubernetes.io/name: kubernaut-apifrontend   # Standard K8s label
app.kubernetes.io/component: api-frontend       # Component role
app.kubernetes.io/part-of: kubernaut            # Parent application
```

---

## References

- [Kagenti Operator](https://github.com/kagenti/kagenti-operator) — Agent lifecycle, AgentCard CRD
- [Kagenti Components](https://github.com/kagenti/kagenti/blob/main/docs/components.md) — Architecture, label standards
- [Kagenti Demo Deployment](https://github.com/redhat-et/kagenti-demo-deployment/blob/main/docs/07-platform/agents.md) — Agent deployment patterns
- [Google ADK Go — A2A Quickstart](https://google.github.io/adk-docs/a2a/quickstart-exposing-go/) — Exposing agents via A2A
- [ADK Go — A2A Launcher](https://github.com/google/adk-go/blob/main/cmd/launcher/web/a2a/a2a.go) — Agent Card auto-generation
- [A2A Protocol](https://a2aprotocol.ai/) — Agent Card specification
- [Kuadrant MCP Gateway](https://github.com/Kuadrant/mcp-gateway) — MCP tool routing
