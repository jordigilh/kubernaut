# DD-HAPI-019: Go Rewrite Design — Document Index

**Status**: ✅ Approved
**Decision Date**: 2026-03-04
**Version**: 1.0
**Applies To**: HolmesGPT-API (HAPI)

---

## Quick Links

### Core DD Document
- **[DD-HAPI-019: Go Rewrite Design](DD-HAPI-019-go-rewrite-design.md)** ⭐ **AUTHORITATIVE** — Kubernaut-owned interface architecture, component design

### Subdocuments

| ID | Document | Content |
|----|----------|---------|
| 001 | **[Framework Selection](DD-HAPI-019-001-framework-selection.md)** | Alternatives analysis (kagent, LangChainGo, Eino, openai-go), decision rationale |
| 002 | **[Toolset Implementation](DD-HAPI-019-002-toolset-implementation.md)** | client-go + net/http design, tool interface, structured output |
| 003 | **[Security Architecture](DD-HAPI-019-003-security-architecture.md)** | Prompt injection defense layers, sanitization pipeline, anomaly detection |

---

## Directory Structure

```
DD-HAPI-019-go-rewrite-design/
├── README.md                                  (this file)
├── DD-HAPI-019-go-rewrite-design.md           (main DD — authoritative)
├── DD-HAPI-019-001-framework-selection.md     (framework decision)
├── DD-HAPI-019-002-toolset-implementation.md  (toolset design)
└── DD-HAPI-019-003-security-architecture.md   (security design)
```

---

## Related Business Requirements

- **[BR-HAPI-433: Go Language Migration](../../../requirements/BR-HAPI-433-go-language-migration/)** — Business requirements this DD implements

## Related GitHub Issues

| Issue | Title |
|-------|-------|
| [#433](https://github.com/jordigilh/kubernaut/issues/433) | Reimplement HolmesGPT SDK and HAPI in Go |
| [#505](https://github.com/jordigilh/kubernaut/issues/505) | kagent evaluation (NO-GO) |
| [#506](https://github.com/jordigilh/kubernaut/issues/506) | LangChainGo evaluation (SELECTED) |
| [#507](https://github.com/jordigilh/kubernaut/issues/507) | Eino evaluation (VIABLE, deferred) |
| [#508](https://github.com/jordigilh/kubernaut/issues/508) | Kubernetes toolset scope |
| [#509](https://github.com/jordigilh/kubernaut/issues/509) | Prometheus toolset scope |

---

**Document Version**: 1.0
**Last Updated**: 2026-03-04
