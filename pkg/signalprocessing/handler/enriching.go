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

// Package handler provides phase-specific handler logic extracted from the monolithic controller.
//
// Reference: CONTROLLER_REFACTORING_PATTERN_LIBRARY.md - Pattern 5 (Controller Decomposition)
//
// This package decomposes the SignalProcessing controller into separate handler files:
// - enriching.go: Handles K8s context enrichment phase (BR-SP-001, BR-SP-100, BR-SP-101)
// - classifying.go: Handles environment and priority classification phase (BR-SP-051-053, BR-SP-070-072)
// - categorizing.go: Handles business categorization phase (BR-SP-002, BR-SP-080, BR-SP-081)
//
// TODO: Complete controller decomposition (Phase 2 refactoring)
// - Extract reconcileEnriching from controller (~150 lines)
// - Extract reconcileClassifying from controller (~70 lines)
// - Extract reconcileCategorizing from controller (~85 lines)
// - Extract detection helpers (detectLabels, hasPDB, hasHPA, etc.)
// - Update controller to delegate to handlers
// - Update integration tests to use handlers
//
// Estimated effort: 3-4 days
// Expected benefits: ~400 lines removed from controller, improved testability
package handler

// TODO: Implement EnrichingHandler
// This handler will encapsulate the logic from reconcileEnriching, including:
// - K8s context enrichment (K8sEnricher)
// - Owner chain traversal (OwnerChainBuilder)
// - Detected labels (LabelDetector)
// - Custom labels (RegoEngine)
// - Phase transition logic
// - Audit event recording













