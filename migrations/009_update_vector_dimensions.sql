-- Update Vector Database Schema for 384-dim Embeddings (Ramalama/20B LLM)
-- Date: 2025-09-26
-- Description: Updates action_patterns table to use 384-dimensional embeddings for ramalama provider
-- Business Requirement: BR-AI-VDB-001 - Support ramalama provider with 20B LLM embeddings

-- Drop existing vector index (required before altering column)
DROP INDEX IF EXISTS action_patterns_embedding_idx;

-- Update embedding column to use 384 dimensions (ramalama/20B LLM standard)
ALTER TABLE action_patterns
ALTER COLUMN embedding TYPE vector(384);

-- Recreate vector similarity index with 384 dimensions
CREATE INDEX action_patterns_embedding_idx
ON action_patterns
USING ivfflat (embedding vector_cosine_ops)
WITH (lists = 50);

-- Add comment for documentation
COMMENT ON COLUMN action_patterns.embedding IS 'Vector embedding (384 dimensions) for ramalama provider with 20B LLM';
