# Kubernaut Guides

**Purpose**: Task-oriented how-to guides for accomplishing specific tasks.

Per [DOCUMENTATION_STRUCTURE.md](../DOCUMENTATION_STRUCTURE.md), this directory contains **how-to guides** following the Diátaxis framework.

---

## 📁 Directory Structure

```
guides/
├── user/                      # End-user guides
│   └── workflow-authoring.md  # Creating Tekton workflows for Kubernaut
└── admin/                     # Administrator guides
    └── (future guides)
```

---

## 📖 Available Guides

### User Guides

| Guide | Description | Audience |
|-------|-------------|----------|
| [Workflow Authoring](./user/workflow-authoring.md) | How to create, package, and deploy Tekton workflows | Platform Engineers, SREs |

### Admin Guides

| Guide | Description | Audience |
|-------|-------------|----------|
| *(Coming soon)* | Scaling, HA, backup/restore | Administrators |

---

## 🎯 Guide vs Tutorial vs Reference

| Type | Purpose | Location |
|------|---------|----------|
| **Tutorial** | Learning-oriented, step-by-step | `docs/getting-started/` |
| **Guide** (you are here) | Task-oriented, problem-solving | `docs/guides/` |
| **Reference** | Information-oriented, facts | `docs/reference/` |

---

## 📝 Contributing

When adding a new guide:
1. Determine if it's user-facing (`user/`) or admin-facing (`admin/`)
2. Follow the template structure in existing guides
3. Update this README with the new guide
4. Link from relevant service documentation






