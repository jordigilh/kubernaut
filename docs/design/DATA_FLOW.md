# Data Flow Diagram

## System Context

```mermaid
graph TB
    subgraph External
        Agent[AI Agent / MCP Client]
        PromScrape[Prometheus<br/>metrics scrape]
    end

    subgraph kubernaut-apifrontend
        Router[HTTP Router]
        Auth[Auth Middleware<br/>JWT + Impersonation]
        MCP[MCP Handler<br/>Streamable HTTP]
        Bridge[Tool Bridge<br/>RBAC + Dispatch]
        Triager[Severity Triager]
        Metrics[Metrics :9090]
        Health[Health :8081]
        Audit[Audit Emitter]
    end

    subgraph Downstream
        KA[kubernaut-agent<br/>REST + MCP]
        DS[data-storage<br/>REST]
        K8s[Kubernetes API<br/>Dynamic Client]
        PromAPI[Prometheus<br/>Query API]
        LLMProv[LLM Provider<br/>Vertex AI]
    end

    Agent -->|POST /mcp| Router
    Router --> Auth
    Auth --> MCP
    MCP --> Bridge
    Bridge -->|CRD tools| K8s
    Bridge -->|KA tools| KA
    Bridge -->|DS tools| DS
    Bridge -->|af_create_rr| Triager
    Triager -->|/api/v1/alerts,rules,query| PromAPI
    Triager -->|Tier 2.5/3 fallback| LLMProv
    Bridge --> Audit
    PromScrape -->|GET /metrics| Metrics
```

## Request Flow: MCP Tool Call

```mermaid
sequenceDiagram
    participant Agent as AI Agent
    participant AF as apifrontend
    participant RBAC as RBAC Check
    participant Sem as Semaphore
    participant Handler as Tool Handler
    participant Downstream as KA / DS / K8s

    Agent->>AF: POST /mcp (tools/call)
    AF->>AF: JWT validation + extract identity
    AF->>RBAC: checkRBAC(user, tool)
    alt Denied
        RBAC-->>AF: error
        AF-->>Agent: isError=true, "permission denied"
        AF->>AF: emit EventMCPToolDenied + metric
    else Allowed
        RBAC-->>AF: nil
        AF->>Sem: Acquire(1)
        alt Throttled
            Sem-->>AF: context canceled
            AF-->>Agent: isError=true, "server busy"
        else Acquired
            Sem-->>AF: ok
            AF->>Handler: execute with timeout ctx
            Handler->>Downstream: API call (impersonated)
            Downstream-->>Handler: response
            Handler-->>AF: result
            AF->>Sem: Release(1)
            AF->>AF: emit EventMCPToolInvoked + metrics
            AF-->>Agent: CallToolResult (JSON)
        end
    end
```

## Data Classification by Flow

```mermaid
graph LR
    subgraph PII Flow
        direction TB
        JWT[JWT Token<br/>username, groups]
        Ctx[Context<br/>UserIdentity]
        Impersonate[K8s Impersonation<br/>username, groups]
        AuditLog[Audit Event<br/>user_id only]
    end

    subgraph Non-PII Flow
        direction TB
        ToolArgs[Tool Arguments<br/>namespace, name, kind]
        CRD[CRD Operations<br/>spec, status]
        KACall[KA REST<br/>correlation IDs]
        DSCall[DS REST<br/>event queries]
    end

    JWT --> Ctx
    Ctx --> Impersonate
    Ctx --> AuditLog

    ToolArgs --> CRD
    ToolArgs --> KACall
    ToolArgs --> DSCall
```

## Downstream Dependencies

| Downstream | Protocol | Auth | Circuit Breaker | Retry | Timeout |
|-----------|----------|------|-----------------|-------|---------|
| Kubernetes API | Dynamic Client | Impersonation | Yes (K8s-specific) | No | 30s |
| kubernaut-agent | REST + MCP | JWT forwarding | Yes | 2 retries, exp backoff | 30s |
| data-storage | REST (ogen) | Service identity | Yes | 3 retries, exp backoff | 10s |
| Prometheus | HTTP `/api/v1/*` | Bearer token (SA) | Planned | No | 30s |
| LLM Provider (Vertex AI) | gRPC/HTTP | ADC | Yes | No | Configurable |

## Error Redaction

All errors flowing back to the AI agent are redacted:
- URLs (any scheme) → `[URL_REDACTED]`
- File paths → `[PATH_REDACTED]`
- Secrets in values → `[REDACTED]`

The original error is logged server-side with full context for SRE diagnosis.
