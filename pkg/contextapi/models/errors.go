package models

import "errors"

// Validation errors
var (
	// Parameter validation errors
	ErrInvalidLimit              = errors.New("invalid limit: must be between 1 and 100")
	ErrLimitTooLarge             = errors.New("limit too large: maximum 100")
	ErrInvalidOffset             = errors.New("invalid offset: must be >= 0")
	ErrInvalidPhase              = errors.New("invalid phase: must be pending, processing, completed, or failed")
	ErrInvalidSeverity           = errors.New("invalid severity: must be critical, warning, or info")
	ErrMissingEmbedding          = errors.New("missing embedding vector")
	ErrInvalidEmbeddingDimension = errors.New("invalid embedding dimension: must be 384")
	ErrMissingQuery              = errors.New("missing query text or embedding")
	ErrInvalidThreshold          = errors.New("invalid threshold value, must be between 0.0 and 1.0")
)
