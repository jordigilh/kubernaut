# BR-HAPI-433: Go Language Migration — Document Index

**Business Requirement ID**: BR-HAPI-433
**Category**: HolmesGPT-API Service
**Priority**: P0
**Target Version**: v1.3
**Status**: ✅ Approved
**Date**: 2026-03-04
**GitHub Issue**: [#433](https://github.com/jordigilh/kubernaut/issues/433)

---

## Quick Links

### Core BR Document
- **[BR-HAPI-433: Go Language Migration](BR-HAPI-433-go-language-migration.md)** ⭐ **AUTHORITATIVE** — Business motivation, objective, scope boundaries, acceptance criteria

### Subdocuments

| ID | Document | Content |
|----|----------|---------|
| 001 | **[Framework Evaluation](BR-HAPI-433-001-framework-evaluation.md)** | Requirements comparison table (current HAPI vs LangChainGo vs Eino vs kagent), new capabilities |
| 002 | **[Kubernetes Toolset](BR-HAPI-433-002-kubernetes-toolset.md)** | 25 tools analyzed, Tier 1+2 selected (11 tools for v1.3) |
| 003 | **[Prometheus Toolset](BR-HAPI-433-003-prometheus-toolset.md)** | 8 tools analyzed, Tier 1+2 selected (6 tools for v1.3), rule fetching dropped |
| 004 | **[Security Requirements](BR-HAPI-433-004-security-requirements.md)** | Prompt injection defense, tool-output sanitization, behavioral anomaly detection |

---

## Directory Structure

```
BR-HAPI-433-go-language-migration/
├── README.md                                    (this file)
├── BR-HAPI-433-go-language-migration.md         (main BR — authoritative)
├── BR-HAPI-433-001-framework-evaluation.md      (framework comparison)
├── BR-HAPI-433-002-kubernetes-toolset.md        (K8s toolset scope)
├── BR-HAPI-433-003-prometheus-toolset.md        (Prometheus toolset scope)
└── BR-HAPI-433-004-security-requirements.md     (security layers)
```

---

## Related Design Decisions

- **[DD-HAPI-019: Go Rewrite Design](../../architecture/decisions/DD-HAPI-019-go-rewrite-design/)** — Technical design decisions for implementing this BR

## Related GitHub Issues

| Issue | Title | Role |
|-------|-------|------|
| [#433](https://github.com/jordigilh/kubernaut/issues/433) | Reimplement HolmesGPT SDK and HAPI in Go | Parent issue |
| [#505](https://github.com/jordigilh/kubernaut/issues/505) | Go LLM framework evaluation — kagent (NO-GO) | Framework spike |
| [#506](https://github.com/jordigilh/kubernaut/issues/506) | Go LLM framework evaluation — LangChainGo (SELECTED) | Framework spike |
| [#507](https://github.com/jordigilh/kubernaut/issues/507) | Go LLM framework evaluation — Eino (VIABLE, deferred) | Framework spike |
| [#508](https://github.com/jordigilh/kubernaut/issues/508) | Kubernetes toolset for HAPI Go rewrite (client-go, Tier 1+2) | Toolset scope |
| [#509](https://github.com/jordigilh/kubernaut/issues/509) | Prometheus toolset for HAPI Go rewrite (Go net/http, Tier 1+2) | Toolset scope |

---

**Document Version**: 1.0
**Last Updated**: 2026-03-04
