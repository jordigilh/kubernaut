# HANDOFF: pgvector Removal Complete - HAPI Service

**Date**: 2026-02-08
**Service**: HolmesGPT-API (HAPI)
**Type**: Cross-Service Cleanup
**Status**: COMPLETE
**Authority**: DD-WORKFLOW-015 (V1.0 Label-Only Architecture)
**Tracking**: https://github.com/jordigilh/kubernaut/issues/51

---

## Summary

All pgvector, embedding, and semantic search references have been removed from the HAPI codebase as part of the V1.0 label-only architecture mandate (DD-WORKFLOW-015).

## Changes Made

### Production Code
- **src/toolsets/workflow_catalog.py**: Updated docstrings to reflect V1.0 label-only search. Changed `min_similarity` to `min_score`.
- **src/clients/datastorage/datastorage/models/search_execution_metadata.py**: Removed `embedding_dimensions` and `embedding_model` fields (aligned with DS OpenAPI spec change).

### Test Infrastructure
- **tests/infrastructure/data_storage.py**: Removed `CREATE EXTENSION vector`, changed image from `pgvector:pg16` to `postgres:16-alpine`, removed vector migration reference.

### Test Files
- **tests/e2e/test_workflow_catalog_data_storage_integration.py**: Changed `min_similarity` to `min_score`.
- **tests/integration/test_workflow_catalog_data_storage_integration.py**: Same.
- **tests/integration/test_workflow_catalog_data_storage.py**: Same.
- **src/clients/test/test_workflow_search_request.py**: Same.
- **tests/unit/test_workflow_catalog_toolset.py**: Updated semantic search test expectations for V1.0.

### Config Files
- **tests/integration/data-storage-integration.yaml**: Removed Embedding Service config section.
- **tests/integration/data-storage-host.yaml**: Removed Embedding Service config section.

## API Contract Changes

The DataStorage `SearchExecutionMetadata` schema no longer includes:
- `embedding_dimensions` (removed)
- `embedding_model` (removed)

Only `duration_ms` remains as a required field.

## Impact

- No functional impact on V1.0 label-only workflow search
- HAPI Python client models updated to match DS OpenAPI spec
- Test infrastructure uses standard `postgres:16-alpine` image (no pgvector extension needed)
