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
// UNIT TESTS: Shared scoring SQL builders
// ========================================
// Authority: DD-WORKFLOW-004 v1.5 (Label-Only Scoring)
// Authority: SQL_WILDCARD_WEIGHTING_IMPLEMENTATION.md
// Bugs: #220 (missing final_score), #215 (inverted wildcard), #212 (custom labels)

// --- Phase 1: Extraction correctness ---

func TestBuildDetectedLabelsBoostSQL_Nil(t *testing.T) {
	result := buildDetectedLabelsBoostSQL(nil)
	if result != "0.0" {
		t.Errorf("expected 0.0 for nil DetectedLabels, got: %s", result)
	}
}

func TestBuildDetectedLabelsBoostSQL_Empty(t *testing.T) {
	dl := &models.DetectedLabels{}
	result := buildDetectedLabelsBoostSQL(dl)
	if result != "0.0" {
		t.Errorf("expected 0.0 for empty DetectedLabels, got: %s", result)
	}
}

func TestBuildDetectedLabelsBoostSQL_GitOpsManaged(t *testing.T) {
	dl := &models.DetectedLabels{GitOpsManaged: true}
	result := buildDetectedLabelsBoostSQL(dl)
	if !strings.Contains(result, "gitOpsManaged") {
		t.Errorf("expected SQL to reference gitOpsManaged, got: %s", result)
	}
	if !strings.Contains(result, "0.10") {
		t.Errorf("expected 0.10 weight for gitOpsManaged, got: %s", result)
	}
}

func TestBuildDetectedLabelsBoostSQL_AllBooleans(t *testing.T) {
	dl := &models.DetectedLabels{
		GitOpsManaged:   true,
		PDBProtected:    true,
		HPAEnabled:      true,
		Stateful:        true,
		HelmManaged:     true,
		NetworkIsolated: true,
	}
	result := buildDetectedLabelsBoostSQL(dl)
	for _, field := range []string{"gitOpsManaged", "pdbProtected", "hpaEnabled", "stateful", "helmManaged", "networkIsolated"} {
		if !strings.Contains(result, field) {
			t.Errorf("expected SQL to reference %s, got: %s", field, result)
		}
	}
}

func TestBuildDetectedLabelsPenaltySQL_Nil(t *testing.T) {
	result := buildDetectedLabelsPenaltySQL(nil)
	if result != "0.0" {
		t.Errorf("expected 0.0 for nil DetectedLabels, got: %s", result)
	}
}

func TestBuildDetectedLabelsPenaltySQL_GitOpsManaged(t *testing.T) {
	dl := &models.DetectedLabels{GitOpsManaged: true}
	result := buildDetectedLabelsPenaltySQL(dl)
	if !strings.Contains(result, "gitOpsManaged") {
		t.Errorf("expected SQL to reference gitOpsManaged, got: %s", result)
	}
}

func TestBuildCustomLabelsBoostSQL_Nil(t *testing.T) {
	result := buildCustomLabelsBoostSQL(nil)
	if result != "0.0" {
		t.Errorf("expected 0.0 for nil customLabels, got: %s", result)
	}
}

func TestBuildCustomLabelsBoostSQL_Empty(t *testing.T) {
	result := buildCustomLabelsBoostSQL(map[string][]string{})
	if result != "0.0" {
		t.Errorf("expected 0.0 for empty customLabels, got: %s", result)
	}
}

func TestBuildCustomLabelsBoostSQL_SingleKey(t *testing.T) {
	labels := map[string][]string{
		"constraint": {"cost-constrained"},
	}
	result := buildCustomLabelsBoostSQL(labels)
	if !strings.Contains(result, "constraint") {
		t.Errorf("expected SQL to reference constraint key, got: %s", result)
	}
	if !strings.Contains(result, "0.05") {
		t.Errorf("expected 0.05 weight for exact match, got: %s", result)
	}
}

// --- Phase 2: Severity wildcard (#215 Gap 1) ---

func TestBuildContextFilterSQL_SeverityWildcard(t *testing.T) {
	filters := &models.WorkflowDiscoveryFilters{
		Severity: "critical",
	}
	sql, _ := buildContextFilterSQL(filters)
	if !strings.Contains(sql, "? '*'") {
		t.Errorf("expected severity filter to include wildcard fallback (? '*'), got: %s", sql)
	}
}

// --- Phase 3: Detected labels boost inversion (#215 Gap 2) ---

func TestBuildDetectedLabelsBoostSQL_GitOpsTool_WorkflowWildcard(t *testing.T) {
	dl := &models.DetectedLabels{GitOpsTool: "argocd"}
	result := buildDetectedLabelsBoostSQL(dl)
	// The SQL should check the WORKFLOW side for '*', not the request side.
	// It should give half boost when workflow has detected_labels->>'gitOpsTool' = '*'
	if !strings.Contains(result, "= '*'") {
		t.Errorf("expected SQL to check workflow-side wildcard (= '*'), got: %s", result)
	}
	if !strings.Contains(result, "0.05") {
		t.Errorf("expected 0.05 (half boost) for workflow wildcard, got: %s", result)
	}
	if !strings.Contains(result, "'argocd'") {
		t.Errorf("expected exact match for 'argocd', got: %s", result)
	}
}

func TestBuildDetectedLabelsBoostSQL_ServiceMesh_WorkflowWildcard(t *testing.T) {
	dl := &models.DetectedLabels{ServiceMesh: "linkerd"}
	result := buildDetectedLabelsBoostSQL(dl)
	if !strings.Contains(result, "= '*'") {
		t.Errorf("expected SQL to check workflow-side wildcard (= '*'), got: %s", result)
	}
	if !strings.Contains(result, "0.03") {
		t.Errorf("expected half-boost (0.03 = 0.05/2 rounded) for serviceMesh wildcard, got: %s", result)
	}
}

// --- Query-side wildcard: query passes "*" to match any workflow value ---

func TestBuildDetectedLabelsBoostSQL_ServiceMesh_QueryWildcard(t *testing.T) {
	dl := &models.DetectedLabels{ServiceMesh: "*"}
	result := buildDetectedLabelsBoostSQL(dl)
	if !strings.Contains(result, "serviceMesh") {
		t.Errorf("expected SQL to reference serviceMesh for query-side wildcard, got: %s", result)
	}
	if !strings.Contains(result, "IS NOT NULL") {
		t.Errorf("expected IS NOT NULL check for query-side wildcard, got: %s", result)
	}
	if !strings.Contains(result, "0.03") {
		t.Errorf("expected half-boost (0.03 = 0.05/2 rounded) for query-side wildcard, got: %s", result)
	}
}

func TestBuildDetectedLabelsBoostSQL_GitOpsTool_QueryWildcard(t *testing.T) {
	dl := &models.DetectedLabels{GitOpsTool: "*"}
	result := buildDetectedLabelsBoostSQL(dl)
	if !strings.Contains(result, "gitOpsTool") {
		t.Errorf("expected SQL to reference gitOpsTool for query-side wildcard, got: %s", result)
	}
	if !strings.Contains(result, "IS NOT NULL") {
		t.Errorf("expected IS NOT NULL check for query-side wildcard, got: %s", result)
	}
	if !strings.Contains(result, "0.05") {
		t.Errorf("expected half-boost (0.05 = 0.10/2) for query-side wildcard, got: %s", result)
	}
}

// --- Phase 6: Custom labels wired into discovery scoring (#212 Gap 2+3) ---

func TestBuildCustomLabelsBoostSQL_MultipleKeys(t *testing.T) {
	labels := map[string][]string{
		"constraint": {"cost-constrained"},
		"team":       {"payments"},
	}
	result := buildCustomLabelsBoostSQL(labels)
	if !strings.Contains(result, "constraint") {
		t.Errorf("expected SQL to reference constraint key, got: %s", result)
	}
	if !strings.Contains(result, "team") {
		t.Errorf("expected SQL to reference team key, got: %s", result)
	}
	if !strings.Contains(result, "0.05") {
		t.Errorf("expected exact match weight 0.05, got: %s", result)
	}
	// Wildcard half-boost present
	if !strings.Contains(result, `'"*"'`) {
		t.Errorf("expected wildcard check for '\"*\"', got: %s", result)
	}
}

func TestBuildCustomLabelsBoostSQL_EmptyValues(t *testing.T) {
	labels := map[string][]string{
		"constraint": {},
	}
	result := buildCustomLabelsBoostSQL(labels)
	if result != "0.0" {
		t.Errorf("expected 0.0 for empty values, got: %s", result)
	}
}
