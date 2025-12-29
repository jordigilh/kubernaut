# DD-DOCS-001: Operational Documentation Template Standard

**Status**: ✅ APPROVED
**Date**: 2025-12-17
**Author**: SignalProcessing Team
**Stakeholders**: All Teams

---

## 📋 **Decision Summary**

**Decision**: Establish a standard template for service operational documentation (BUILD.md, OPERATIONS.md, DEPLOYMENT.md) with mandatory Helm values reference.

---

## 🎯 **Context**

As Kubernaut approaches V1.0 release, consistent operational documentation is critical for:
- Operations teams deploying and maintaining services
- On-call engineers troubleshooting production issues
- New team members onboarding to service ownership

Helm will be the standard deployment mechanism for V1.0.

---

## ✅ **Standard Document Set**

Every service MUST have these three operational documents:

| Document | Purpose | Primary Audience |
|----------|---------|------------------|
| **BUILD.md** | Build, test, and develop the service | Developers |
| **OPERATIONS.md** | Monitor, troubleshoot, and maintain the service | SREs, On-Call |
| **DEPLOYMENT.md** | Deploy and configure the service | Platform Engineers |

---

## 📄 **BUILD.md Template**

### Required Sections

```markdown
# {ServiceName} Service - Build Guide

**Version**: x.y
**Last Updated**: {date}

---

## Overview
{Brief description of the service}

---

## Prerequisites

### Required Tools
| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.21+ | Language runtime |
| Docker/Podman | 20.10+ | Container builds |
| kubectl | 1.28+ | Kubernetes CLI |
| make | 4.0+ | Build automation |

### Optional Tools
| Tool | Version | Purpose |
|------|---------|---------|
| Kind | 0.20+ | Local K8s cluster |
| golangci-lint | 1.55+ | Linting |
| Ginkgo | 2.13+ | BDD testing |

---

## Quick Start
{5-line getting started commands}

---

## Build Commands

### Binary Build
{Commands to build binary with version info}

### Container Build
{Docker/Podman build commands}

### CRD Generation (if applicable)
{make manifests, make generate}

---

## Project Structure
{Directory tree with descriptions}

---

## Testing

### Unit Tests
{Commands and coverage targets}

### Integration Tests
{ENVTEST setup and commands}

### E2E Tests
{Kind setup and commands}

---

## Development Workflow
{TDD/APDC workflow, local development commands}

---

## Code Quality
{Linting, formatting, security scanning}

---

## Dependencies
{Key packages and update instructions}

---

## CI/CD Integration
{GitHub Actions example}

---

## Troubleshooting
{Common build issues table}

---

## References
{Links to related DDs, ADRs, rules}
```

---

## 📄 **OPERATIONS.md Template**

### Required Sections

```markdown
# {ServiceName} Service - Operations Guide

**Version**: x.y
**Last Updated**: {date}

---

## Service Overview
| Property | Value |
|----------|-------|
| **Service Name** | {service-name} |
| **Health Port** | {port} |
| **Metrics Port** | {port} |
| **CRD** (if applicable) | {crd.kubernaut.ai} |
| **Namespace** | kubernaut-system |

---

## Health Checks
{Endpoints and Kubernetes probe configurations}

---

## Monitoring

### Key Metrics (DD-005 Compliant)
{Table of metrics with type and description}

### Prometheus Queries
{Common PromQL queries for dashboards}

### Alert Rules
{Standard alert definitions}

---

## Logging

### Log Levels
{Level usage guidelines}

### Log Configuration
{Environment variables and ConfigMap}

### Log Analysis
{kubectl + jq examples for common searches}

---

## Configuration

### Environment Variables
{Complete table with defaults and descriptions}

### ConfigMap
{Full ConfigMap example}

### Helm Values (V1.0+)
{Reference to DEPLOYMENT.md Helm section}

---

## Runbooks

### {Runbook 1: Common Issue}
**Symptoms**: {what operators see}
**Check**: {diagnostic commands}
**Solutions**: {resolution steps}

### {Runbook 2: Common Issue}
...

---

## Scaling
{Resource recommendations by workload size}

---

## Disaster Recovery
{Backup considerations and recovery procedures}

---

## Support
{Team contact, Slack channels, PagerDuty}

---

## References
{Links to related docs}
```

---

## 📄 **DEPLOYMENT.md Template**

### Required Sections

```markdown
# {ServiceName} Service - Deployment Guide

**Version**: x.y
**Last Updated**: {date}

---

## Overview
{Brief description of deployment options}

---

## Prerequisites

### Cluster Requirements
| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| Kubernetes | 1.27+ | 1.28+ |
| Nodes | 1 | 3 (HA) |
| CPU | {min} | {recommended} |
| Memory | {min} | {recommended} |

### Required Components
{Dependencies that must be installed first}

---

## Helm Deployment (Recommended)

### Add Repository
```bash
helm repo add kubernaut https://charts.kubernaut.ai
helm repo update
```

### Install
```bash
helm install {service-name} kubernaut/{service-name} \
  --namespace kubernaut-system \
  --create-namespace
```

### Upgrade
```bash
helm upgrade {service-name} kubernaut/{service-name} \
  --namespace kubernaut-system \
  --reuse-values
```

### Uninstall
```bash
helm uninstall {service-name} --namespace kubernaut-system
```

---

## Helm Values Reference

### Global Values
| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `global.imageRegistry` | string | `ghcr.io` | Container registry |
| `global.imagePullSecrets` | list | `[]` | Image pull secrets |

### Image Configuration
| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `image.repository` | string | `ghcr.io/jordigilh/kubernaut/{service}` | Image repository |
| `image.tag` | string | `latest` | Image tag |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy |

### Resources
| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `resources.requests.cpu` | string | `100m` | CPU request |
| `resources.requests.memory` | string | `128Mi` | Memory request |
| `resources.limits.cpu` | string | `500m` | CPU limit |
| `resources.limits.memory` | string | `256Mi` | Memory limit |

### Replicas and HA
| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `replicaCount` | int | `1` | Number of replicas |
| `leaderElection.enabled` | bool | `true` | Enable leader election |
| `podDisruptionBudget.enabled` | bool | `false` | Enable PDB |
| `podDisruptionBudget.minAvailable` | int | `1` | Minimum available pods |

### Logging
| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `logging.level` | string | `info` | Log level |
| `logging.format` | string | `json` | Log format |

### Metrics
| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `metrics.enabled` | bool | `true` | Enable metrics |
| `metrics.port` | int | `9090` | Metrics port |
| `serviceMonitor.enabled` | bool | `false` | Create ServiceMonitor |

### Health Probes
| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `health.port` | int | `8081` | Health probe port |
| `livenessProbe.initialDelaySeconds` | int | `15` | Liveness initial delay |
| `readinessProbe.initialDelaySeconds` | int | `5` | Readiness initial delay |

### Service-Specific Configuration
| Value | Type | Default | Description |
|-------|------|---------|-------------|
| {service-specific values} | | | |

### Example values.yaml
```yaml
# Production deployment
replicaCount: 3
podDisruptionBudget:
  enabled: true
  minAvailable: 1
resources:
  requests:
    cpu: 250m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 512Mi
logging:
  level: info
metrics:
  enabled: true
serviceMonitor:
  enabled: true
```

---

## Manual Deployment (Alternative)

### Quick Deploy
```bash
kubectl apply -f config/crd/bases/
kubectl apply -f config/rbac/{service}/
kubectl apply -f config/manager/{service}/
```

### Step-by-Step
{Detailed manual deployment steps}

---

## Verification
{Commands to verify successful deployment}

---

## High Availability
{Multi-replica configuration, PDB}

---

## Upgrades
{Rolling update procedures, CRD upgrade notes}

---

## Uninstallation
{Complete removal steps}

---

## Network Policies
{Security-hardened network policy examples}

---

## References
{Links to BUILD.md, OPERATIONS.md, security docs}
```

---

## 🔧 **Enforcement**

### V1.0 Requirement
All services MUST have:
- ✅ BUILD.md with all required sections
- ✅ OPERATIONS.md with all required sections
- ✅ DEPLOYMENT.md with Helm values reference

### V1.1+ Requirement
- ✅ Helm chart must be published for each service
- ✅ Helm values must match DEPLOYMENT.md specification
- ✅ ServiceMonitor must be included for Prometheus integration

---

## 📊 **Migration Plan**

### Phase 1: Template Adoption (V1.0)
1. ✅ Create DD-DOCS-001 (this document)
2. [ ] Update existing SP docs with Helm values spec
3. [ ] Create template for other services

### Phase 2: Helm Charts (V1.0 Release)
1. [ ] Create umbrella Helm chart for Kubernaut
2. [ ] Create sub-charts for each service
3. [ ] Publish to Helm repository

### Phase 3: Validation (Post-V1.0)
1. [ ] Automate doc validation in CI
2. [ ] Helm chart testing pipeline
3. [ ] ServiceMonitor validation

---

## ✅ **Approval**

- [x] **SignalProcessing Team** - @jgil - 2025-12-17 - Approved

---

## 📚 **References**

- [DD-005: Observability Standards](DD-005-OBSERVABILITY-STANDARDS.md)
- [DD-007: Graceful Shutdown](DD-007-kubernetes-aware-graceful-shutdown.md)
- [03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc)






