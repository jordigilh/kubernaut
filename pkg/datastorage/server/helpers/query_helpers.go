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

package helpers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
)

// ========================================
// AUDIT EVENTS QUERY HELPERS
// ========================================
// Authority: DD-STORAGE-010 (Audit Query API)
// BR-STORAGE-033: Query audit events
//
// These functions handle parsing and building audit event queries
// from HTTP request parameters.
// ========================================

// QueryFilters holds parsed query parameters for audit events
// Using ADR-034 field names (event_category, event_outcome)
type QueryFilters struct {
	CorrelationID string
	EventType     string
	EventCategory string // ADR-034: event_category
	EventOutcome  string // ADR-034: event_outcome
	Severity      string
	Since         *time.Time
	Until         *time.Time
	Limit         int
	Offset        int
}

// ParseQueryFilters extracts and validates query parameters from HTTP request
// DD-STORAGE-010: Parse audit event query filters
// Returns QueryFilters and error if parsing fails
func ParseQueryFilters(r *http.Request) (*QueryFilters, error) {
	query := r.URL.Query()

	filters := &QueryFilters{
		CorrelationID: query.Get("correlation_id"),
		EventType:     query.Get("event_type"),
		EventCategory: query.Get("event_category"), // ADR-034
		EventOutcome:  query.Get("event_outcome"),  // ADR-034
		Severity:      query.Get("severity"),
		Limit:         100, // Default limit
		Offset:        0,   // Default offset
	}

	// Parse time parameters
	if sinceParam := query.Get("since"); sinceParam != "" {
		since, err := ParseTimeParam(sinceParam)
		if err != nil {
			return nil, err
		}
		filters.Since = &since
	}

	if untilParam := query.Get("until"); untilParam != "" {
		until, err := ParseTimeParam(untilParam)
		if err != nil {
			return nil, err
		}
		filters.Until = &until
	}

	// Parse pagination parameters
	if limitParam := query.Get("limit"); limitParam != "" {
		var limit int
		if _, err := fmt.Sscanf(limitParam, "%d", &limit); err != nil {
			return nil, fmt.Errorf("invalid limit parameter: must be an integer")
		}
		filters.Limit = limit
	}

	if offsetParam := query.Get("offset"); offsetParam != "" {
		var offset int
		if _, err := fmt.Sscanf(offsetParam, "%d", &offset); err != nil {
			return nil, fmt.Errorf("invalid offset parameter: must be an integer")
		}
		filters.Offset = offset
	}

	return filters, nil
}

// ParseTimeParam parses time parameters (relative or absolute)
// DD-STORAGE-010: Time parsing for query API
// Delegates to pkg/datastorage/query.ParseTimeParam for actual parsing
func ParseTimeParam(param string) (time.Time, error) {
	return query.ParseTimeParam(param)
}

// BuildQueryFromFilters creates an AuditEventsQueryBuilder from parsed filters
// DD-STORAGE-010: Build SQL query from filters
// Returns configured query builder ready for execution
func BuildQueryFromFilters(filters *QueryFilters, logger logr.Logger) *query.AuditEventsQueryBuilder {
	builder := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logger))

	if filters.CorrelationID != "" {
		builder = builder.WithCorrelationID(filters.CorrelationID)
	}
	if filters.EventType != "" {
		builder = builder.WithEventType(filters.EventType)
	}
	if filters.EventCategory != "" {
		builder = builder.WithService(filters.EventCategory) // WithService maps to event_category
	}
	if filters.EventOutcome != "" {
		builder = builder.WithOutcome(filters.EventOutcome) // WithOutcome maps to event_outcome
	}
	if filters.Severity != "" {
		builder = builder.WithSeverity(filters.Severity)
	}
	if filters.Since != nil {
		builder = builder.WithSince(*filters.Since)
	}
	if filters.Until != nil {
		builder = builder.WithUntil(*filters.Until)
	}

	builder = builder.WithLimit(filters.Limit).WithOffset(filters.Offset)

	return builder
}
