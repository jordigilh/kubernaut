/*
Copyright 2025 Jordi Gil Heredia.

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

package models

// ========================================
// HEALTH CHECK RESPONSE TYPES
// Business Requirement: BR-STORAGE-028 (DD-007 Graceful Shutdown)
// ========================================
//
// These structured types replace map[string]interface{} for health/readiness check responses,
// providing compile-time type safety for Kubernetes probe endpoints.
//
// Anti-Pattern Addressed: Using map[string]interface{} eliminates type safety (IMPLEMENTATION_PLAN_V4.9 #21)
// ========================================

// ReadinessResponse represents the response from GET /health/ready
// DD-007: Kubernetes readiness probe coordination during graceful shutdown
type ReadinessResponse struct {
	Status string `json:"status"` // "ready", "not_ready"
	Reason string `json:"reason,omitempty"` // "shutting_down", "database_unreachable"
	Error  string `json:"error,omitempty"`  // Error message if database unreachable
}

// HealthResponse represents the response from GET /health
// Basic health check endpoint
type HealthResponse struct {
	Status   string `json:"status"`   // "healthy", "unhealthy"
	Database string `json:"database,omitempty"` // "connected", "disconnected"
	Error    string `json:"error,omitempty"`     // Error message if unhealthy
}

// LivenessResponse represents the response from GET /health/live
// Kubernetes liveness probe endpoint
type LivenessResponse struct {
	Status string `json:"status"` // "alive"
}

Copyright 2025 Jordi Gil Heredia.

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

package models

// ========================================
// HEALTH CHECK RESPONSE TYPES
// Business Requirement: BR-STORAGE-028 (DD-007 Graceful Shutdown)
// ========================================
//
// These structured types replace map[string]interface{} for health/readiness check responses,
// providing compile-time type safety for Kubernetes probe endpoints.
//
// Anti-Pattern Addressed: Using map[string]interface{} eliminates type safety (IMPLEMENTATION_PLAN_V4.9 #21)
// ========================================

// ReadinessResponse represents the response from GET /health/ready
// DD-007: Kubernetes readiness probe coordination during graceful shutdown
type ReadinessResponse struct {
	Status string `json:"status"` // "ready", "not_ready"
	Reason string `json:"reason,omitempty"` // "shutting_down", "database_unreachable"
	Error  string `json:"error,omitempty"`  // Error message if database unreachable
}

// HealthResponse represents the response from GET /health
// Basic health check endpoint
type HealthResponse struct {
	Status   string `json:"status"`   // "healthy", "unhealthy"
	Database string `json:"database,omitempty"` // "connected", "disconnected"
	Error    string `json:"error,omitempty"`     // Error message if unhealthy
}

// LivenessResponse represents the response from GET /health/live
// Kubernetes liveness probe endpoint
type LivenessResponse struct {
	Status string `json:"status"` // "alive"
}

