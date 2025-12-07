# Kubernaut Documentation Structure

**Version**: v1.0
**Last Updated**: 2025-12-06
**Status**: ✅ AUTHORITATIVE
**Based On**: Diátaxis Framework + CNCF Project Patterns (Kubernetes, Prometheus, Grafana)

---

## 📋 Overview

This document defines the **authoritative documentation structure** for Kubernaut, following industry-standard patterns for cloud-native and observability projects.

**Framework**: [Diátaxis](https://diataxis.fr/) - A systematic approach to technical documentation authoring.

---

## 🗂️ Directory Structure

```
docs/
├── getting-started/           # 🎓 TUTORIALS - Learning-oriented
│   ├── installation.md        # Installation guide
│   ├── quickstart.md          # 5-minute quick start
│   └── first-remediation.md   # First workflow execution
│
├── guides/                    # 📖 HOW-TO GUIDES - Task-oriented
│   ├── user/                  # End-user guides
│   │   ├── workflow-authoring.md      # Creating Tekton workflows
│   │   ├── alert-configuration.md     # Configuring alerts
│   │   └── notification-setup.md      # Setting up notifications
│   └── admin/                 # Administrator guides
│       ├── scaling.md                 # Horizontal scaling
│       ├── high-availability.md       # HA setup
│       └── backup-restore.md          # Data backup/restore
│
├── reference/                 # 📚 REFERENCE - Information-oriented
│   ├── api/                   # REST API reference
│   │   └── openapi.yaml       # OpenAPI specification
│   ├── crds/                  # CRD schema reference
│   │   ├── remediationrequest.md
│   │   ├── workflowexecution.md
│   │   ├── aianalysis.md
│   │   └── notification.md
│   ├── cli/                   # CLI reference
│   ├── configuration/         # Configuration reference
│   │   ├── gateway.md
│   │   ├── datastorage.md
│   │   └── holmesgpt-api.md
│   └── metrics/               # Metrics reference
│       └── prometheus-metrics.md
│
├── concepts/                  # 💡 EXPLANATION - Understanding-oriented
│   ├── architecture.md        # System architecture
│   ├── crd-reconciliation.md  # CRD coordination patterns
│   ├── ai-integration.md      # HolmesGPT integration
│   └── safety-framework.md    # Remediation safety
│
├── operations/                # 🔧 OPERATIONS - SRE/Ops-oriented
│   ├── runbooks/              # Service runbooks
│   │   ├── gateway-runbook.md
│   │   ├── datastorage-runbook.md
│   │   ├── holmesgpt-api-runbook.md
│   │   ├── workflowexecution-runbook.md
│   │   └── notification-runbook.md
│   ├── monitoring/            # Monitoring setup
│   │   ├── prometheus-setup.md
│   │   ├── grafana-dashboards.md
│   │   └── alerting-rules.md
│   └── maintenance/           # Maintenance procedures
│       ├── upgrades.md
│       └── database-maintenance.md
│
├── troubleshooting/           # 🔍 TROUBLESHOOTING - Problem-oriented
│   ├── common-issues.md       # FAQ and common issues
│   ├── debugging-guide.md     # Debug techniques
│   ├── log-analysis.md        # Log interpretation
│   └── service-specific/      # Per-service troubleshooting
│       ├── gateway-issues.md
│       ├── datastorage-issues.md
│       └── workflowexecution-issues.md
│
├── architecture/              # 🏗️ ARCHITECTURE DECISIONS
│   ├── decisions/             # ADRs and DDs
│   │   ├── ADR-XXX-*.md
│   │   └── DD-XXX-*.md
│   └── diagrams/              # Architecture diagrams
│
├── services/                  # 📦 SERVICE SPECIFICATIONS
│   ├── crd-controllers/       # CRD controller specs
│   │   ├── 01-signalprocessing/
│   │   ├── 02-aianalysis/
│   │   ├── 03-workflowexecution/
│   │   ├── 05-remediationorchestrator/
│   │   └── 06-notification/
│   └── stateless/             # Stateless service specs
│       ├── gateway-service/
│       ├── datastorage-service/
│       └── holmesgpt-api-service/
│
├── development/               # 👨‍💻 DEVELOPMENT
│   ├── contributing.md        # Contribution guide
│   ├── code-style.md          # Code standards
│   ├── testing-guide.md       # Testing standards
│   └── business-requirements/ # BR documentation
│
├── requirements/              # 📋 BUSINESS REQUIREMENTS
│   └── BR-*.md                # Individual BR docs
│
└── templates/                 # 📄 TEMPLATES
    ├── service-spec.md
    ├── adr-template.md
    └── runbook-template.md
```

---

## 📖 Diátaxis Framework Mapping

| Diátaxis Type | Directory | Purpose | Audience |
|---------------|-----------|---------|----------|
| **Tutorials** | `getting-started/` | Learning-oriented, practical steps | New users |
| **How-to Guides** | `guides/` | Task-oriented, problem-solving | Users with specific goals |
| **Reference** | `reference/` | Information-oriented, accurate facts | Developers, operators |
| **Explanation** | `concepts/` | Understanding-oriented, context | Anyone seeking understanding |

### Additional Categories (Cloud-Native/SRE Extensions)

| Category | Directory | Purpose | Audience |
|----------|-----------|---------|----------|
| **Operations** | `operations/` | Runbooks, monitoring, maintenance | SREs, Operators |
| **Troubleshooting** | `troubleshooting/` | Debug, diagnose, resolve | Support, Operators |
| **Architecture** | `architecture/` | ADRs, design decisions | Architects, Developers |
| **Services** | `services/` | Service specifications | Implementers |

---

## 📝 Document Types and Locations

### User-Facing Documents

| Document Type | Location | Example |
|---------------|----------|---------|
| Quick Start | `getting-started/quickstart.md` | 5-minute intro |
| User Guide | `guides/user/` | `workflow-authoring.md` |
| Admin Guide | `guides/admin/` | `scaling.md` |
| API Reference | `reference/api/` | OpenAPI spec |
| CRD Reference | `reference/crds/` | CRD schemas |
| Troubleshooting | `troubleshooting/` | Common issues |

### Operations Documents

| Document Type | Location | Example |
|---------------|----------|---------|
| Runbook | `operations/runbooks/` | `workflowexecution-runbook.md` |
| Monitoring Setup | `operations/monitoring/` | `prometheus-setup.md` |
| Maintenance | `operations/maintenance/` | `upgrades.md` |

### Developer Documents

| Document Type | Location | Example |
|---------------|----------|---------|
| Service Spec | `services/{type}/{name}/` | Complete service documentation |
| Implementation Plan | `services/{type}/{name}/implementation/` | Day-by-day plans |
| ADR | `architecture/decisions/` | `ADR-044-*.md` |
| Design Decision | `architecture/decisions/` | `DD-WE-001-*.md` |
| Business Requirement | `requirements/` | `BR-WE-001-*.md` |

---

## 🎯 Document Placement Decision Tree

```
📝 QUESTION: Where should this document go?

├─ 🎓 "How do I get started?"
│  └─ getting-started/
│
├─ 📖 "How do I accomplish [specific task]?"
│  └─ guides/{user|admin}/
│
├─ 📚 "What is the exact [API|config|metric]?"
│  └─ reference/{api|crds|configuration|metrics}/
│
├─ 💡 "Why does [system|feature] work this way?"
│  └─ concepts/
│
├─ 🔧 "How do I operate/maintain this in production?"
│  └─ operations/{runbooks|monitoring|maintenance}/
│
├─ 🔍 "Why isn't [thing] working? How do I fix it?"
│  └─ troubleshooting/
│
├─ 🏗️ "What architectural decision was made?"
│  └─ architecture/decisions/
│
├─ 📦 "What is the complete spec for [service]?"
│  └─ services/{type}/{name}/
│
└─ 👨‍💻 "How do I develop/contribute?"
   └─ development/
```

---

## 📋 Industry Standards Referenced

### CNCF Project Patterns

| Project | Pattern Adopted |
|---------|-----------------|
| **Kubernetes** | `concepts/`, `reference/`, clear separation of tutorials/tasks/reference |
| **Prometheus** | `operations/`, `alerting/`, metrics reference |
| **Grafana** | `guides/`, dashboard documentation patterns |
| **Helm** | `getting-started/`, chart documentation |

### SRE Best Practices

| Source | Pattern Adopted |
|--------|-----------------|
| **Google SRE Book** | Runbook structure, error budgets, SLOs |
| **PagerDuty Runbooks** | Incident response procedures |
| **Datadog** | Monitoring and troubleshooting guides |

---

## 🔄 Migration from Current Structure

### Phase 1: Create Missing Directories
```bash
mkdir -p docs/guides/{user,admin}
mkdir -p docs/reference/{api,crds,configuration,metrics}
mkdir -p docs/operations/{runbooks,monitoring,maintenance}
mkdir -p docs/troubleshooting/service-specific
```

### Phase 2: Consolidate Runbooks
```
# Current location (scattered in service specs)
docs/services/crd-controllers/03-workflowexecution/implementation/APPENDIX_B_PRODUCTION_RUNBOOKS.md

# New location (centralized)
docs/operations/runbooks/workflowexecution-runbook.md
```

### Phase 3: Create User Guides
```
# New guides to create
docs/guides/user/workflow-authoring.md  # How to create Tekton workflows
docs/guides/user/alert-configuration.md # How to configure alerts
docs/guides/admin/scaling.md            # How to scale Kubernaut
```

### Phase 4: Consolidate Troubleshooting
```
# Current (scattered)
docs/troubleshooting/DATASTORAGE_VERSION_ERRORS.md

# New structure
docs/troubleshooting/common-issues.md           # FAQ
docs/troubleshooting/service-specific/          # Per-service issues
```

---

## ✅ Checklist for New Documentation

Before creating a new document, verify:

- [ ] **Location**: Does it follow the decision tree above?
- [ ] **Type**: Is it clearly one of: Tutorial, How-to, Reference, Explanation?
- [ ] **Audience**: Is the target audience clear?
- [ ] **Navigation**: Is it linked from the appropriate index/README?
- [ ] **Naming**: Does filename match content type?
- [ ] **Template**: Does it use appropriate template from `templates/`?

---

## 📞 Related Documents

- [06-documentation-standards.mdc](../.cursor/rules/06-documentation-standards.mdc) - Writing style and code documentation
- [DOCUMENTATION_REORGANIZATION_PROPOSAL.md](./DOCUMENTATION_REORGANIZATION_PROPOSAL.md) - Archival strategy
- [README.md](./README.md) - Documentation index

---

## 📝 Changelog

| Version | Date | Changes |
|---------|------|---------|
| v1.0 | 2025-12-06 | Initial authoritative structure definition |


