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

package response

import (
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// PAGINATION RESPONSE BUILDER (V1.0 REFACTOR)
// 📋 Authority: docs/handoff/DS_REFACTORING_OPPORTUNITIES.md (Opportunity 3.3)
// ========================================
//
// This helper reduces duplication in pagination response building
// across query handlers.
//
// V1.0 REFACTOR Goals:
// - Consistent pagination response format
// - Reduced code duplication
// - Type-safe response construction
//
// Business Value:
// - Easier maintenance (change pagination format once)
// - Consistent API responses
// - Clearer intent in handler code
//
// ========================================

// PaginatedResponse wraps data with pagination metadata.
// This is the standard response format for all paginated queries.
type PaginatedResponse struct {
	Data       interface{}                    `json:"data"`
	Pagination *repository.PaginationMetadata `json:"pagination"`
}

// NewPaginatedResponse creates a paginated response with metadata.
//
// Usage:
//
//	// Before (5 lines):
//	response := &AuditEventsQueryResponse{
//	    Data: events,
//	    Pagination: &repository.PaginationMetadata{
//	        Limit:  limit,
//	        Offset: offset,
//	        Total:  int(totalCount),
//	    },
//	}
//
//	// After (1 line):
//	response := response.NewPaginatedResponse(events, limit, offset, totalCount)
func NewPaginatedResponse(data interface{}, limit, offset int, totalCount int64) *PaginatedResponse {
	return &PaginatedResponse{
		Data: data,
		Pagination: &repository.PaginationMetadata{
			Limit:  limit,
			Offset: offset,
			Total:  int(totalCount), // Convert int64 to int for PaginationMetadata
		},
	}
}
