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
