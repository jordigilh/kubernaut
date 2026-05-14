// Package reconstruction provides RemediationRequest CRD reconstruction from audit traces.
//
// # Business Requirement
//
// BR-AUDIT-005 v2.0: Enterprise-Grade Audit Integrity and Compliance
// - RR Reconstruction Support: Reconstruct complete RR CRD from audit trail
// - SOC2 CC8.1: Tamper-evident audit logs for compliance
//
// # Architecture
//
// This package implements a 5-phase reconstruction pipeline:
//
//  1. Query:    Retrieve all audit events by correlation ID
//  2. Parse:    Extract RR fields from audit event payloads
//  3. Map:      Aggregate fields into Spec/Status structures
//  4. Build:    Generate valid RR CRD YAML
//  5. Validate: Ensure reconstructed RR meets all constraints
//
// # Usage
//
//	// Query audit events
//	events, err := QueryAuditEventsForReconstruction(ctx, correlationID)
//
//	// Parse and map fields
//	spec, status, err := MapAuditToRRFields(events)
//
//	// Build RR CRD
//	rr, err := BuildRemediationRequest(spec, status)
//
//	// Validate
//	err = ValidateReconstructedRR(rr)
//
// # Testing Strategy
//
// TDD RED-GREEN-REFACTOR:
// - RED: Write failing tests with expected reconstruction behavior
// - GREEN: Implement minimal logic to pass tests
// - REFACTOR: Enhance with edge case handling and optimization
//
// # References
//
// - Design Decision: docs/architecture/decisions/DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md
// - Business Requirement: BR-AUDIT-005 v2.0
package reconstruction
