-- +goose Up
-- +goose StatementBegin

-- ========================================
-- MIGRATION 016: Update Embedding Dimensions
-- ========================================
-- BR-STORAGE-014: Update embedding dimensions from 384 to 768
-- Design Decision: DD-EMBEDDING-001 (Model B: all-mpnet-base-v2)
-- Rationale: 92% accuracy vs 85% (7% improvement = 46% fewer errors)
--
-- Changes:
-- 1. Drop existing HNSW index (dimension-dependent)
-- 2. Alter embedding column to 768 dimensions
-- 3. Recreate HNSW index with new dimensions
-- 4. Update column comment
--
-- Note: Existing embeddings will be NULL after this migration
-- They will be regenerated automatically on next workflow update
-- ========================================

-- Step 1: Drop existing HNSW index
DROP INDEX IF EXISTS idx_workflow_embedding_hnsw;

-- Step 2: Alter embedding column dimensions (384 → 768)
ALTER TABLE remediation_workflow_catalog
    ALTER COLUMN embedding TYPE vector(768);

-- Step 3: Recreate HNSW index with new dimensions
-- HNSW parameters optimized for 768 dimensions:
-- - m=16: Number of connections per layer (good balance)
-- - ef_construction=64: Build-time accuracy (higher = better recall)
CREATE INDEX idx_workflow_embedding_hnsw
    ON remediation_workflow_catalog
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Step 4: Update column comment
COMMENT ON COLUMN remediation_workflow_catalog.embedding IS 'Vector embedding for semantic search (768 dimensions, sentence-transformers/all-mpnet-base-v2, 92% accuracy)';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Rollback: Revert to 384 dimensions
DROP INDEX IF EXISTS idx_workflow_embedding_hnsw;

ALTER TABLE remediation_workflow_catalog
    ALTER COLUMN embedding TYPE vector(384);

CREATE INDEX idx_workflow_embedding_hnsw
    ON remediation_workflow_catalog
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

COMMENT ON COLUMN remediation_workflow_catalog.embedding IS 'Vector embedding for semantic search (384 dimensions, sentence-transformers/all-MiniLM-L6-v2)';

-- +goose StatementEnd

