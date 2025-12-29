# ⚠️ **DEPRECATED** - Dynamic Toolset Deployment

**Status**: ❌ **SERVICE CODE DELETED**
**Date**: December 20, 2025
**Reason**: Deferred to V2.0 per DD-016
**Authority**: [DD-016 - Dynamic Toolset V2.0 Deferral](../../docs/architecture/decisions/DD-016-dynamic-toolset-v2-deferral.md)

---

## 📋 **Notice**

The Dynamic Toolset Service has been **removed from V1.0** and deferred to V2.0. This deployment directory is preserved for:

1. **Historical Reference**: Document deployment patterns for V2.0
2. **Architecture Reference**: Kubernetes resource structure
3. **V2.0 Planning**: Starting point for future implementation

---

## 🚫 **Do Not Deploy**

**These manifests will not work** because the service code (`pkg/toolset/`, `cmd/dynamictoolset/`) has been deleted.

To deploy Dynamic Toolset:
1. **Wait for V2.0** (estimated Q3 2026)
2. **Or** use git history to recover deleted code (not recommended - outdated methodology)

---

## 📚 **For V2.0 Reference**

This directory contains deployment patterns that may be useful for V2.0:
- Kubernetes manifest structure
- ServiceAccount and RBAC configuration
- ConfigMap patterns
- Service and deployment specs

**See**: `README.md` (original) for deployment patterns

---

## 🔗 **References**

- **Deprecation Notice**: [docs/services/stateless/dynamic-toolset/DEPRECATED_V1_0.md](../../docs/services/stateless/dynamic-toolset/DEPRECATED_V1_0.md)
- **Design Decision**: [DD-016 - Dynamic Toolset V2.0 Deferral](../../docs/architecture/decisions/DD-016-dynamic-toolset-v2-deferral.md)
- **V1.0 Status**: Service removed, documentation preserved

---

**Last Updated**: December 20, 2025
**Maintainer**: Kubernaut Architecture Team
**Status**: ❌ Deprecated - V2.0 Rebuild Planned












