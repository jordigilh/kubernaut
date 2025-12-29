# OpenAPI Specification - MOVED

**Date**: 2025-12-13
**Status**: 🔴 **DEPRECATED** - This directory is no longer used

---

## ✅ **AUTHORITATIVE SPEC LOCATION**

The authoritative OpenAPI specification for Data Storage Service is now:

```
api/openapi/data-storage-v1.yaml
```

**All client generation should use this spec.**

---

## 📋 **REASON FOR MOVE**

**Problem**: Multiple OpenAPI specs caused drift and integration issues:
- `docs/services/stateless/data-storage/openapi/v3.yaml` (1782 lines) - Used by HAPI
- `api/openapi/data-storage-v1.yaml` (701 lines) - Used for Go client

**Solution**: Consolidated to single authoritative spec in standard location (`api/openapi/`)

---

## 🚀 **CLIENT GENERATION**

### **Go Client**:
```bash
oapi-codegen -package client -generate types,client \
  -o pkg/datastorage/client/generated.go \
  api/openapi/data-storage-v1.yaml
```

### **Python Client** (HAPI):
```bash
podman run --rm -v ${PWD}:/local:z openapitools/openapi-generator-cli generate \
  -i /local/api/openapi/data-storage-v1.yaml \
  -g python \
  -o /local/holmesgpt-api/src/clients/datastorage \
  --package-name datastorage_client
```

---

## 📚 **REFERENCES**

- **Authoritative Spec**: `api/openapi/data-storage-v1.yaml`
- **Service Documentation**: `docs/services/stateless/data-storage/README.md`
- **Migration Issue**: `docs/handoff/HANDOFF_HAPI_TO_DS_OPENAPI_SPEC_ISSUE.md`

---

**This directory will be removed in a future cleanup.**
