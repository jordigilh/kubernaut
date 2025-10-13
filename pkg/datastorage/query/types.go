/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package query

import "github.com/jordigilh/kubernaut/pkg/datastorage/models"

// SemanticResult represents a semantic search result with similarity score
// BR-STORAGE-012: Semantic search result type
type SemanticResult struct {
	*models.RemediationAudit
	Similarity float32 `json:"similarity"` // Cosine similarity score (0-1)
}

// PaginationResult contains paginated results with metadata
// BR-STORAGE-006: Pagination support
type PaginationResult struct {
	Data       interface{} `json:"data"`
	TotalCount int64       `json:"total_count"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

