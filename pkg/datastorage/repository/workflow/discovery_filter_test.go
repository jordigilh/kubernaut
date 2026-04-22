/*
Copyright 2026 Jordi Gil.

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

package workflow

import (
	"strings"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// UNIT TESTS: buildContextFilterSQL with DetectedLabels
// ========================================
// Authority: DD-WORKFLOW-001 v2.7 (DetectedLabels matching semantics)
// Bug: Issue #197 (discovery SQL ignores detectedLabels)
//
// These tests validate that buildContextFilterSQL generates correct SQL
// conditions for DetectedLabels boolean and string fields per DD-WORKFLOW-001
// matching semantics:
//   - Boolean true: workflow must require it OR have no requirement (absent)
//   - String value: workflow must match exact OR wildcard "*" OR absent
//   - FailedDetections: fields listed in FailedDetections are excluded
// ========================================

func TestBuildContextFilterSQL_DetectedLabels_PDBProtected(t *testing.T) {
	// DL-HP-10 end-to-end: When pdbProtected=true is detected, SQL must
	// filter workflows to only return those that require PDB protection
	// or have no PDB requirement (generic workflows).
	filters := &models.WorkflowDiscoveryFilters{
		DetectedLabels: &models.DetectedLabels{
			PDBProtected: true,
		},
	}

	sql, args := buildContextFilterSQL(filters)

	if sql == "" {
		t.Fatal("expected non-empty SQL when pdbProtected=true is set")
	}
	if !strings.Contains(sql, "pdbProtected") {
		t.Errorf("expected SQL to reference pdbProtected, got: %s", sql)
	}
	if len(args) == 0 {
		t.Error("expected at least one arg for pdbProtected filter")
	}
}

func TestBuildContextFilterSQL_DetectedLabels_GitOpsTool(t *testing.T) {
	// DD-WORKFLOW-001 v1.6: gitOpsTool string matching with wildcard support.
	// When gitOpsTool="argocd", workflows with "argocd", "*", or absent match.
	filters := &models.WorkflowDiscoveryFilters{
		DetectedLabels: &models.DetectedLabels{
			GitOpsManaged: true,
			GitOpsTool:    "argocd",
		},
	}

	sql, args := buildContextFilterSQL(filters)

	if sql == "" {
		t.Fatal("expected non-empty SQL when gitOpsTool is set")
	}
	if !strings.Contains(sql, "gitOpsTool") {
		t.Errorf("expected SQL to reference gitOpsTool, got: %s", sql)
	}
	if !strings.Contains(sql, "gitOpsManaged") {
		t.Errorf("expected SQL to reference gitOpsManaged, got: %s", sql)
	}
	if len(args) < 1 {
		t.Error("expected args for gitOpsTool filter")
	}
}

func TestBuildContextFilterSQL_DetectedLabels_WithMandatoryFilters(t *testing.T) {
	// When both mandatory and detected labels are present, SQL must contain
	// conditions for both.
	filters := &models.WorkflowDiscoveryFilters{
		Severity:  "critical",
		Component: "Deployment",
		DetectedLabels: &models.DetectedLabels{
			PDBProtected: true,
		},
	}

	sql, args := buildContextFilterSQL(filters)

	if sql == "" {
		t.Fatal("expected non-empty SQL")
	}
	if !strings.Contains(sql, "severity") {
		t.Errorf("expected SQL to reference severity, got: %s", sql)
	}
	if !strings.Contains(sql, "pdbProtected") {
		t.Errorf("expected SQL to reference pdbProtected, got: %s", sql)
	}
	if len(args) < 2 {
		t.Errorf("expected at least 2 args (severity + pdbProtected), got: %d", len(args))
	}
}

func TestBuildContextFilterSQL_DetectedLabels_NilDoesNotFilter(t *testing.T) {
	// When DetectedLabels is nil, no detected label conditions are added.
	filters := &models.WorkflowDiscoveryFilters{
		Severity: "critical",
	}

	sql, args := buildContextFilterSQL(filters)

	if strings.Contains(sql, "detected_labels") {
		t.Errorf("expected no detected_labels in SQL when DetectedLabels is nil, got: %s", sql)
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg (severity only), got: %d", len(args))
	}
}

func TestBuildContextFilterSQL_DetectedLabels_AllBooleanFields(t *testing.T) {
	// When all boolean fields are true, SQL must reference all of them.
	filters := &models.WorkflowDiscoveryFilters{
		DetectedLabels: &models.DetectedLabels{
			GitOpsManaged:   true,
			PDBProtected:    true,
			HPAEnabled:      true,
			Stateful:        true,
			HelmManaged:     true,
			NetworkIsolated: true,
		},
	}

	sql, _ := buildContextFilterSQL(filters)

	for _, field := range []string{"gitOpsManaged", "pdbProtected", "hpaEnabled", "stateful", "helmManaged", "networkIsolated"} {
		if !strings.Contains(sql, field) {
			t.Errorf("expected SQL to reference %s, got: %s", field, sql)
		}
	}
}

func TestBuildContextFilterSQL_DetectedLabels_ServiceMesh(t *testing.T) {
	// DD-WORKFLOW-001 v1.6: serviceMesh string matching with wildcard support.
	filters := &models.WorkflowDiscoveryFilters{
		DetectedLabels: &models.DetectedLabels{
			ServiceMesh: "istio",
		},
	}

	sql, args := buildContextFilterSQL(filters)

	if !strings.Contains(sql, "serviceMesh") {
		t.Errorf("expected SQL to reference serviceMesh, got: %s", sql)
	}
	if len(args) < 1 {
		t.Error("expected args for serviceMesh filter")
	}
}

// ========================================
// UNIT TESTS: Issue #464 — Mandatory Label Wildcard Matching
// ========================================
// Authority: DD-WORKFLOW-001 v2.8 (wildcard support for all 4 mandatory labels)
// Bug report: Issue #464 (discovery returns 0 when workflows use '*' wildcards)
//
// These tests validate that buildContextFilterSQL generates SQL that would
// correctly match workflows with wildcard ('*') mandatory labels against
// specific query values. The SQL must include wildcard fallback conditions
// for every mandatory label.
// ========================================

func TestBuildContextFilterSQL_Issue464_ComponentWildcard(t *testing.T) {
	// UT-DS-464-001: When component filter is "Pod", the SQL must include
	// a wildcard fallback so workflows with component='*' are matched.
	filters := &models.WorkflowDiscoveryFilters{
		Component: "Pod",
	}

	sql, args := buildContextFilterSQL(filters)

	if !strings.Contains(sql, "labels->'component' ? '*'") {
		t.Errorf("UT-DS-464-001: expected component wildcard fallback (labels->'component' ? '*'), got: %s", sql)
	}
	if !strings.Contains(sql, "jsonb_array_elements_text(labels->'component')") {
		t.Errorf("UT-DS-464-001: expected array-based component matching, got: %s", sql)
	}
	if len(args) != 1 || args[0] != "Pod" {
		t.Errorf("UT-DS-464-001: expected args=[Pod], got: %v", args)
	}
}

func TestBuildContextFilterSQL_Issue464_PriorityScalarWildcard(t *testing.T) {
	// UT-DS-464-002: When priority filter is "P1", the SQL ELSE branch
	// must include a wildcard fallback so workflows with priority='*' (scalar) are matched.
	filters := &models.WorkflowDiscoveryFilters{
		Priority: "P1",
	}

	sql, args := buildContextFilterSQL(filters)

	if !strings.Contains(sql, "labels->>'priority' = '*'") {
		t.Errorf("UT-DS-464-002: expected scalar priority wildcard fallback, got: %s", sql)
	}
	if len(args) != 1 || args[0] != "P1" {
		t.Errorf("UT-DS-464-002: expected args=[P1], got: %v", args)
	}
}

func TestBuildContextFilterSQL_Issue464_PriorityArrayWildcard(t *testing.T) {
	// UT-DS-464-003: The priority array CASE branch must also include
	// a wildcard fallback (labels->'priority' ? '*') so that array-stored
	// ["*"] priorities match any query value.
	// BUG: Current code only has labels->'priority' ? $N in the array branch.
	filters := &models.WorkflowDiscoveryFilters{
		Priority: "P1",
	}

	sql, _ := buildContextFilterSQL(filters)

	// The THEN branch (array case) must include wildcard: ? '*'
	// Current code: THEN labels->'priority' ? $1
	// Expected:     THEN labels->'priority' ? $1 OR labels->'priority' ? '*'
	thenIdx := strings.Index(sql, "THEN")
	elseIdx := strings.Index(sql, "ELSE")
	if thenIdx == -1 || elseIdx == -1 {
		t.Fatalf("UT-DS-464-003: expected CASE/THEN/ELSE structure, got: %s", sql)
	}
	arrayBranch := sql[thenIdx:elseIdx]
	if !strings.Contains(arrayBranch, "? '*'") {
		t.Errorf("UT-DS-464-003: priority array branch missing wildcard fallback (? '*'), got array branch: %s", arrayBranch)
	}
}

func TestBuildContextFilterSQL_Issue464_EnvironmentWildcard(t *testing.T) {
	// UT-DS-464-005: When environment filter is "staging", the SQL must
	// include a wildcard fallback so workflows with environment=["*"] are matched.
	filters := &models.WorkflowDiscoveryFilters{
		Environment: "staging",
	}

	sql, args := buildContextFilterSQL(filters)

	if !strings.Contains(sql, "labels->'environment' ? '*'") {
		t.Errorf("UT-DS-464-005: expected environment wildcard fallback, got: %s", sql)
	}
	if len(args) != 1 || args[0] != "staging" {
		t.Errorf("UT-DS-464-005: expected args=[staging], got: %v", args)
	}
}

func TestBuildContextFilterSQL_Issue464_AllMandatoryWildcards(t *testing.T) {
	// UT-DS-464-006: The exact query from issue #464. All 4 mandatory filters
	// provided. SQL must have wildcard fallbacks for every label so that
	// all-wildcard workflows are always discoverable.
	filters := &models.WorkflowDiscoveryFilters{
		Severity:    "critical",
		Component:   "Pod",
		Environment: "staging",
		Priority:    "P1",
	}

	sql, args := buildContextFilterSQL(filters)

	// Severity wildcard
	if !strings.Contains(sql, "labels->'severity' ? '*'") {
		t.Errorf("UT-DS-464-006: missing severity wildcard fallback, got: %s", sql)
	}
	// Component wildcard (Issue #790: now array-based)
	if !strings.Contains(sql, "labels->'component' ? '*'") {
		t.Errorf("UT-DS-464-006: missing component wildcard fallback, got: %s", sql)
	}
	// Environment wildcard
	if !strings.Contains(sql, "labels->'environment' ? '*'") {
		t.Errorf("UT-DS-464-006: missing environment wildcard fallback, got: %s", sql)
	}
	// Priority wildcard (scalar branch)
	if !strings.Contains(sql, "labels->>'priority' = '*'") {
		t.Errorf("UT-DS-464-006: missing priority scalar wildcard fallback, got: %s", sql)
	}
	// 4 args: one per mandatory label
	if len(args) != 4 {
		t.Errorf("UT-DS-464-006: expected 4 args, got: %d", len(args))
	}
	expected := []interface{}{"critical", "Pod", "staging", "P1"}
	for i, exp := range expected {
		if args[i] != exp {
			t.Errorf("UT-DS-464-006: args[%d] = %v, want %v", i, args[i], exp)
		}
	}
}

// ========================================
// UNIT TESTS: Issue #595 — Case-Insensitive Mandatory Label Matching
// ========================================
// Authority: DD-WORKFLOW-001 v2.9 (case-insensitive JSONB array matching)
// Bug report: Issue #595 (discovery filters are case-sensitive; SP produces
// PascalCase environment, DS stores lowercase in workflow labels)
//
// These tests validate that buildContextFilterSQL generates SQL using
// EXISTS/jsonb_array_elements_text/LOWER instead of the case-sensitive
// JSONB ? operator for severity, environment, and priority array branches.
// ========================================

func TestBuildContextFilterSQL_Issue595_EnvironmentCaseInsensitive(t *testing.T) {
	// UT-DS-595-001: Environment filter SQL must use case-insensitive
	// JSONB array matching via EXISTS/jsonb_array_elements_text/LOWER
	// instead of the case-sensitive ? operator.
	filters := &models.WorkflowDiscoveryFilters{
		Environment: "Production",
	}

	sql, args := buildContextFilterSQL(filters)

	if !strings.Contains(sql, "jsonb_array_elements_text(labels->'environment')") {
		t.Errorf("UT-DS-595-001: expected jsonb_array_elements_text for environment, got: %s", sql)
	}
	if !strings.Contains(sql, "LOWER(elem) = LOWER(") {
		t.Errorf("UT-DS-595-001: expected LOWER(elem) = LOWER() pattern, got: %s", sql)
	}
	if len(args) != 1 || args[0] != "Production" {
		t.Errorf("UT-DS-595-001: expected args=[Production], got: %v", args)
	}
}

func TestBuildContextFilterSQL_Issue595_SeverityCaseInsensitive(t *testing.T) {
	// UT-DS-595-002: Severity filter SQL must use case-insensitive
	// JSONB array matching via EXISTS/jsonb_array_elements_text/LOWER.
	filters := &models.WorkflowDiscoveryFilters{
		Severity: "Critical",
	}

	sql, args := buildContextFilterSQL(filters)

	if !strings.Contains(sql, "jsonb_array_elements_text(labels->'severity')") {
		t.Errorf("UT-DS-595-002: expected jsonb_array_elements_text for severity, got: %s", sql)
	}
	if !strings.Contains(sql, "LOWER(elem) = LOWER(") {
		t.Errorf("UT-DS-595-002: expected LOWER(elem) = LOWER() pattern, got: %s", sql)
	}
	if len(args) != 1 || args[0] != "Critical" {
		t.Errorf("UT-DS-595-002: expected args=[Critical], got: %v", args)
	}
}

func TestBuildContextFilterSQL_Issue595_PriorityArrayCaseInsensitive(t *testing.T) {
	// UT-DS-595-003: Priority CASE WHEN array branch (THEN) must use
	// case-insensitive matching via jsonb_array_elements_text/LOWER.
	filters := &models.WorkflowDiscoveryFilters{
		Priority: "p1",
	}

	sql, _ := buildContextFilterSQL(filters)

	thenIdx := strings.Index(sql, "THEN")
	elseIdx := strings.Index(sql, "ELSE")
	if thenIdx == -1 || elseIdx == -1 {
		t.Fatalf("UT-DS-595-003: expected CASE/THEN/ELSE structure, got: %s", sql)
	}
	arrayBranch := sql[thenIdx:elseIdx]
	if !strings.Contains(arrayBranch, "jsonb_array_elements_text(labels->'priority')") {
		t.Errorf("UT-DS-595-003: array branch must use jsonb_array_elements_text, got: %s", arrayBranch)
	}
	if !strings.Contains(arrayBranch, "LOWER(elem) = LOWER(") {
		t.Errorf("UT-DS-595-003: array branch must use LOWER() for case-insensitive match, got: %s", arrayBranch)
	}
}

func TestBuildContextFilterSQL_Issue595_CombinedCaseInsensitive(t *testing.T) {
	// UT-DS-595-004: All 4 mandatory filters provided with mixed-case values.
	// Severity, environment, and priority array branch must all use
	// case-insensitive pattern; component uses existing LOWER; 4 args preserved.
	filters := &models.WorkflowDiscoveryFilters{
		Severity:    "Critical",
		Component:   "Deployment",
		Environment: "Staging",
		Priority:    "p1",
	}

	sql, args := buildContextFilterSQL(filters)

	if !strings.Contains(sql, "jsonb_array_elements_text(labels->'severity')") {
		t.Errorf("UT-DS-595-004: expected case-insensitive severity pattern, got: %s", sql)
	}
	if !strings.Contains(sql, "jsonb_array_elements_text(labels->'environment')") {
		t.Errorf("UT-DS-595-004: expected case-insensitive environment pattern, got: %s", sql)
	}
	if !strings.Contains(sql, "jsonb_array_elements_text(labels->'component')") {
		t.Errorf("UT-DS-595-004: expected array-based component matching, got: %s", sql)
	}
	thenIdx := strings.Index(sql, "THEN")
	elseIdx := strings.Index(sql, "ELSE")
	if thenIdx != -1 && elseIdx != -1 {
		arrayBranch := sql[thenIdx:elseIdx]
		if !strings.Contains(arrayBranch, "jsonb_array_elements_text(labels->'priority')") {
			t.Errorf("UT-DS-595-004: priority array branch must use jsonb_array_elements_text, got: %s", arrayBranch)
		}
	}
	if len(args) != 4 {
		t.Errorf("UT-DS-595-004: expected 4 args, got: %d", len(args))
	}
}

func TestBuildContextFilterSQL_Issue595_WildcardPreservation(t *testing.T) {
	// UT-DS-595-005: Wildcard fallback (? '*') must still be present
	// alongside the new EXISTS/LOWER pattern for severity and environment.
	filters := &models.WorkflowDiscoveryFilters{
		Severity:    "critical",
		Environment: "production",
	}

	sql, _ := buildContextFilterSQL(filters)

	if !strings.Contains(sql, "labels->'severity' ? '*'") {
		t.Errorf("UT-DS-595-005: severity wildcard fallback missing, got: %s", sql)
	}
	if !strings.Contains(sql, "labels->'environment' ? '*'") {
		t.Errorf("UT-DS-595-005: environment wildcard fallback missing, got: %s", sql)
	}
}

func TestBuildContextFilterSQL_Issue595_PriorityScalarBranchPreserved(t *testing.T) {
	// UT-DS-595-006: Priority scalar ELSE branch must still use the
	// labels->>'priority' extraction (scalar path), not jsonb_array_elements_text.
	filters := &models.WorkflowDiscoveryFilters{
		Priority: "P0",
	}

	sql, _ := buildContextFilterSQL(filters)

	thenIdx := strings.Index(sql, "THEN")
	elseIdx := strings.Index(sql, "ELSE")
	endIdx := strings.Index(sql, "END")
	if thenIdx == -1 || elseIdx == -1 || endIdx == -1 {
		t.Fatalf("UT-DS-595-006: expected CASE/THEN/ELSE/END structure, got: %s", sql)
	}
	scalarBranch := sql[elseIdx:endIdx]
	if !strings.Contains(scalarBranch, "LOWER(labels->>'priority')") {
		t.Errorf("UT-DS-595-006: scalar branch must use LOWER(labels->>'priority') for case-insensitive matching, got: %s", scalarBranch)
	}
	if strings.Contains(scalarBranch, "jsonb_array_elements_text") {
		t.Errorf("UT-DS-595-006: scalar branch must NOT use jsonb_array_elements_text, got: %s", scalarBranch)
	}
}
