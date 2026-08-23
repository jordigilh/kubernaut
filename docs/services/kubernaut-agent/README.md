# Kubernaut Agent (KA) - Documentation Hub

**Version**: 2.0
**Last Updated**: 2026-08-23
**Service Type**: Hybrid — `AgentSession` CRD-dispatch (watch + Lease `Reconciler`, DD-AA-KA-001) + MCP service (native Go)
**Ports**: `8443` HTTPS (MCP endpoint only — AF's channel, DD-AF-004), `8081` (health/admin), `9090` (metrics). AA's dispatch channel is the `AgentSession` CRD, not a port.

---

## 🗂️ Documentation Index

| Document | Purpose | Lines |
|---|---|---|
| **[Overview](./overview.md)** | Purpose, architecture, key design decisions, system context diagram | 169 |
| **[Business Requirements](./BUSINESS_REQUIREMENTS.md)** | Catalog of KA's `BR-KA-*` requirement documents | 60 |
| **[BR Mapping](./BR_MAPPING.md)** | BR-to-test-file traceability | 74 |
| **[API Specification](./api-specification.md)** | `AgentSession` CRD contract (DD-AA-KA-001 dispatch model) | 151 |
| **[Integration Points](./integration-points.md)** | Upstream caller (AIAnalysis) and downstream dependencies | 94 |
| **[Observability & Logging](./observability-logging.md)** | Structured logging, correlation ID propagation | 67 |
| **[Testing Strategy](./testing-strategy.md)** | Test pyramid, framework, mock-LLM strategy | 88 |
| **[Metrics & SLOs](./metrics-slos.md)** | Prometheus metrics (`aiagent_*`/`aiagent_alignment_*`), SLIs | 108 |
| **[Security Configuration](./security-configuration.md)** | RBAC, ServiceAccount, network posture | 219 |
| **[Configuration Reference](./configuration-reference.md)** | Full static + hot-reloadable LLM configuration reference | 478 |
| **[Shadow Agent Configuration](./shadow-agent-configuration.md)** | Prompt-injection guardrail operational guide | 398 |
| **[Audit Event Catalog](./security/AUDIT_EVENT_CATALOG.md)** | All emitted audit events with NIST/SOC2 control mapping | 192 |

**Total**: ~2,098 lines across 12 documents.

---

## 🚀 Quick Start

**For integration** (calling KA from another service): read
[api-specification.md](./api-specification.md), then [integration-points.md](./integration-points.md).

**For operations** (running/configuring KA): read
[configuration-reference.md](./configuration-reference.md), then
[shadow-agent-configuration.md](./shadow-agent-configuration.md) if the Shadow Agent is enabled.

**For security review**: read [security-configuration.md](./security-configuration.md) and
[security/AUDIT_EVENT_CATALOG.md](./security/AUDIT_EVENT_CATALOG.md).

**For compliance/audit**: read [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) and
[BR_MAPPING.md](./BR_MAPPING.md).

---

## 📁 File Organization

```
kubernaut-agent/
├── README.md                       - You are here
├── overview.md                     - Architecture & design decisions
├── BUSINESS_REQUIREMENTS.md        - BR catalog
├── BR_MAPPING.md                   - BR-to-test traceability
├── api-specification.md            - AgentSession CRD contract
├── integration-points.md           - Upstream/downstream dependencies
├── observability-logging.md        - Logging & correlation IDs
├── testing-strategy.md             - Test pyramid & strategy
├── metrics-slos.md                 - Prometheus metrics & SLIs
├── security-configuration.md       - RBAC & network posture
├── configuration-reference.md      - Full configuration reference
├── shadow-agent-configuration.md   - Shadow Agent operational guide
└── security/
    └── AUDIT_EVENT_CATALOG.md      - Audit event catalog
```

---

## 🔗 Related Documentation

- [Stateless Services Overview](../stateless/README.md) — navigation hub for the other stateless services
- [DD-KA-019: Go Rewrite Design](../../architecture/decisions/DD-KA-019-go-rewrite-design/) — why KA is a native Go rewrite, not a Python/HolmesGPT SDK wrapper
- [DD-AA-KA-001: AgentSession CRD, HTTP Removal](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md) — why AA↔KA dispatch is a CRD, not HTTP
- [AGENTS.md](../../../AGENTS.md) — Kubernaut development methodology (TDD, BR mandate, SOC2/FedRAMP compliance)
