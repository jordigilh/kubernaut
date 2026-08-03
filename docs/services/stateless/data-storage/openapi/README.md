# OpenAPI Specification - MOVED

**Date**: 2025-12-13
**Status**: 🔴 **DEPRECATED** - This directory is no longer used

> **CORRECTED (2026-08-02, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))**: The
> "Reason for Move" section below is retained as historical context — it correctly describes the
> pre-rewrite state where the Python HolmesGPT-API ("HAPI") used its own copy of the spec. HAPI was
> rewritten in Go as **Kubernaut Agent (KA)** (Issue #433) and, like every other Go service, now
> consumes the single authoritative spec directly. The "Client Generation" section has been rewritten
> below to describe the current (ogen-based, Go-only) generation flow — there is no Python client.

---

## ✅ **AUTHORITATIVE SPEC LOCATION**

The authoritative OpenAPI specification for Data Storage Service is now:

```
api/openapi/data-storage-v1.yaml
```

**All client generation should use this spec.**

---

## 📋 **REASON FOR MOVE (historical)**

**Problem**: Multiple OpenAPI specs caused drift and integration issues:
- `docs/services/stateless/data-storage/openapi/v3.yaml` (1782 lines) - Used by the pre-rewrite Python HolmesGPT-API ("HAPI")
- `api/openapi/data-storage-v1.yaml` (701 lines) - Used for Go client

**Solution**: Consolidated to single authoritative spec in standard location (`api/openapi/`)

---

## 🚀 **CLIENT GENERATION**

All consumers — including Kubernaut Agent (KA), which was rewritten in Go (Issue #433) and no longer
has a Python client — generate a single shared Go client from the authoritative spec via `ogen`
(`pkg/datastorage/ogen-client/gen.go`):

```bash
make generate-datastorage-client
# equivalent to: go generate ./pkg/datastorage/ogen-client/...
# regenerates pkg/datastorage/ogen-client/oas_*_gen.go from api/openapi/data-storage-v1.yaml
```

---

## 📚 **REFERENCES**

- **Authoritative Spec**: `api/openapi/data-storage-v1.yaml`
- **Service Documentation**: `docs/services/stateless/data-storage/README.md`
- **Migration Issue**: (internal development reference, removed in v1.0)

---

**This directory will be removed in a future cleanup.**
