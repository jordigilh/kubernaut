# Interactive Mode Architecture

## Component Diagram

```mermaid
graph TB
    subgraph "MCP Client (IDE/CLI)"
        C[MCP SDK Client]
    end

    subgraph "Kubernaut Agent Pod"
        subgraph "HTTP Layer"
            MW[Auth Middleware<br/>TokenReview + SAR + UserInfo]
            RL[User Rate Limiter<br/>per-IP throttle]
            MH[MCP StreamableHTTP Handler]
        end

        subgraph "MCP Tools Layer"
            IT[InvestigateTool<br/>start/message/complete/cancel/takeover/status]
            ET[EnrichTool<br/>K8s resource enrichment]
            SWT[SelectWorkflowTool<br/>workflow catalog]
        end

        subgraph "Session Management"
            LSM[LeaseSessionManager<br/>Lease-backed, single-driver]
            TM[TimeoutManager<br/>inactivity + TTL]
            SN[SessionNotifier<br/>push warnings to client]
            SRL[SessionRateLimiter<br/>per-session message cap]
        end

        subgraph "Lifecycle"
            DES[DelegatingEventStore<br/>disconnect detection]
            SCH[SessionClosedHandler<br/>release + reconstruct]
            RS[ReconstructionSpawner<br/>autonomous handoff]
        end

        subgraph "Transport"
            IRT[ImpersonatingRoundTripper<br/>inject Impersonate-User/Group]
        end
    end

    subgraph "Kubernetes API Server"
        K8S[API Server<br/>RBAC enforcement]
    end

    C -->|"POST /api/v1/mcp<br/>Bearer token"| MW
    MW -->|"strips Impersonate-* headers<br/>validates token (full UserInfo)"| RL
    RL --> MH
    MH --> IT
    MH --> ET
    MH --> SWT

    IT -->|"Takeover/Release"| LSM
    IT -->|"StartTracking/Reset/Stop"| TM
    IT -->|"Allow(sessionID, msgSize)"| SRL
    IT -->|"WithImpersonatedUser(ctx)"| IRT
    ET -->|"WithImpersonatedUser(ctx)"| IRT

    TM -->|"onExpire → Release"| LSM
    TM -->|"warning intervals"| SN
    SN -->|"ServerSession.Log"| MH

    DES -->|"SessionClosed channel"| SCH
    SCH -->|"Release + SpawnReconstruct"| LSM
    SCH --> RS

    LSM -->|"Create/Delete Lease"| K8S
    IRT -->|"K8s calls as user"| K8S
```

## Request Flow (action=message)

```mermaid
sequenceDiagram
    participant Client as MCP Client
    participant MW as Auth Middleware
    participant IT as InvestigateTool
    participant SRL as SessionRateLimiter
    participant TM as TimeoutManager
    participant IRT as ImpersonatingRoundTripper
    participant K8S as K8s API Server

    Client->>MW: POST /api/v1/mcp (Bearer token)
    MW->>MW: ValidateTokenFull → UserInfo{username, groups}
    MW->>MW: CheckAccess (SAR)
    MW->>IT: Handle(ctx+UserInfo, input)

    IT->>IT: ValidateInput (action=message)
    IT->>IT: GetDriver → verify caller == driver
    IT->>SRL: Allow(sessionID, len(msg))
    SRL-->>IT: ok

    IT->>TM: ResetInactivity(sessionID)
    IT->>IT: WithImpersonatedUser(ctx, username, groups)
    IT->>IRT: RunInteractiveTurn → K8s API calls
    IRT->>K8S: GET /api/v1/pods (Impersonate-User: alice)
    K8S-->>IRT: 200 OK (RBAC allows)
    IRT-->>IT: LLM response

    IT-->>Client: {status: "message_received", response: "..."}
```

## Disconnect + Reconstruction Flow

```mermaid
sequenceDiagram
    participant Client as MCP Client
    participant DES as DelegatingEventStore
    participant SCH as SessionClosedHandler
    participant LSM as LeaseSessionManager
    participant RS as ReconstructionSpawner

    Client-xDES: TCP disconnect
    DES->>DES: SessionClosed(mcpSessionID)
    DES->>SCH: closedChan <- mcpSessionID

    SCH->>DES: LookupInteractiveSession(mcpSessionID)
    DES-->>SCH: interactiveSessionID

    SCH->>LSM: GetSessionInfo(sessionID) → rrID, signalMeta
    SCH->>LSM: Release(sessionID, "disconnect")
    SCH->>RS: SpawnReconstruct(rrID, signalMeta)
    RS->>RS: Reconstruct context + start autonomous investigation
```

## RBAC Model

| Resource | Scope | Verbs | Purpose |
|----------|-------|-------|---------|
| `coordination.k8s.io/leases` | Namespace (Role) | get, create, update, delete | Session ownership tracking |
| `users`, `groups`, `serviceaccounts` | Cluster (ClusterRole) | impersonate | Execute K8s calls as the user |

The Lease RBAC is namespace-scoped (least privilege). Impersonation must be
cluster-wide because users may investigate resources across namespaces.
