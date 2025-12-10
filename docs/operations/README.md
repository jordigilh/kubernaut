# Kubernaut Operations

**Purpose**: Operational procedures, runbooks, and maintenance guides for SREs and operators.

Per [DOCUMENTATION_STRUCTURE.md](../DOCUMENTATION_STRUCTURE.md), this directory contains operational documentation for running Kubernaut in production.

---

## 📁 Directory Structure

```
operations/
├── runbooks/                           # Service-specific runbooks
│   └── workflowexecution-runbook.md    # WorkflowExecution production procedures
├── monitoring/                         # Monitoring setup (future)
└── maintenance/                        # Maintenance procedures (future)
```

---

## 📚 Available Runbooks

| Service | Runbook | Alerts Covered |
|---------|---------|----------------|
| WorkflowExecution | [workflowexecution-runbook.md](./runbooks/workflowexecution-runbook.md) | 6 runbooks (RB-WE-001 to RB-WE-006) |

---

## 🔧 Runbook Standards

All runbooks follow a consistent structure:

1. **Alert Definition**: Prometheus alerting rule
2. **Symptoms**: What you'll observe
3. **Diagnosis Steps**: Commands to investigate
4. **Resolution**: How to fix
5. **Prevention**: How to avoid in future

---

## 🔗 Related Documentation

- [Troubleshooting Guide](../troubleshooting/) - Common issues and FAQ
- [Metrics & SLOs](../services/) - Per-service metrics documentation
- [Architecture Decisions](../architecture/decisions/) - Design rationale

---

## 📝 Contributing

When adding a new runbook:
1. Use the template from existing runbooks
2. Include Prometheus alert definition
3. Provide copy-paste diagnostic commands
4. Link to related troubleshooting docs
5. Update this README



