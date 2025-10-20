package client

import "errors"

// Client errors
var (
	// ErrIncidentNotFound is returned when an incident is not found by ID
	ErrIncidentNotFound = errors.New("incident not found")

	// ErrConnectionFailed is returned when database connection fails
	ErrConnectionFailed = errors.New("database connection failed")

	// ErrQueryFailed is returned when a query execution fails
	ErrQueryFailed = errors.New("query execution failed")
)
