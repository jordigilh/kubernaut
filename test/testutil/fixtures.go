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

// Package testutil provides shared test helpers for loading workflow fixtures
// and building type-safe workflow schema test inputs.
//
// Issue #330: Centralizes inline YAML test fixtures into shared files and
// a Go struct builder, eliminating brittle string concatenation in tests.
package testutil

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ValidBundleRef is a syntactically valid OCI bundle reference with sha256 digest
// for use in test fixtures. Tests that don't care about the specific bundle value
// can use this default.
const ValidBundleRef = "quay.io/kubernaut-cicd/test-workflows/generic:v1@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

// LoadWorkflowFixture reads a workflow-schema.yaml from the shared fixture directory.
// The fixture must exist at test/fixtures/workflows/<name>/workflow-schema.yaml
// relative to the workspace root (located by walking up to find go.mod).
//
// Panics on error because fixture loading failures in tests are programming errors,
// not conditions tests should handle.
func LoadWorkflowFixture(name string) string {
	root, err := FindWorkspaceRoot()
	if err != nil {
		panic(fmt.Sprintf("testutil.LoadWorkflowFixture: find workspace root: %v", err))
	}
	path := filepath.Join(root, "test", "fixtures", "workflows", name, "workflow-schema.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("testutil.LoadWorkflowFixture(%q): %v", name, err))
	}
	return string(data)
}

// MarshalWorkflowCRD serializes a WorkflowSchemaCRD to YAML suitable for
// passing to schema.Parser.ParseAndValidate().
//
// Panics on nil input or marshal error because these are programming errors.
func MarshalWorkflowCRD(crd *models.WorkflowSchemaCRD) string {
	if crd == nil {
		panic("testutil.MarshalWorkflowCRD: crd must not be nil")
	}
	data, err := yaml.Marshal(crd)
	if err != nil {
		panic(fmt.Sprintf("testutil.MarshalWorkflowCRD: %v", err))
	}
	return string(data)
}

// NewTestWorkflowCRD returns a valid baseline WorkflowSchemaCRD with sensible defaults.
// Tests should mutate specific fields before calling MarshalWorkflowCRD.
//
// The returned CRD passes ParseAndValidate out of the box with:
//   - apiVersion: kubernaut.ai/v1alpha1
//   - kind: RemediationWorkflow
//   - Valid labels (severity, environment, component, priority)
//   - Valid execution (engine + bundle with sha256 digest)
//   - One required string parameter (NAMESPACE)
//   - Structured description (what + whenToUse)
func NewTestWorkflowCRD(name, actionType, engine string) *models.WorkflowSchemaCRD {
	return &models.WorkflowSchemaCRD{
		APIVersion: "kubernaut.ai/v1alpha1",
		Kind:       "RemediationWorkflow",
		Metadata:   models.WorkflowCRDMetadata{Name: name},
		Spec: models.WorkflowSchema{
			Version: "1.0.0",
			Description: sharedtypes.StructuredDescription{
				What:      fmt.Sprintf("Test workflow: %s", name),
				WhenToUse: "When running automated tests",
			},
			ActionType: actionType,
			Labels: models.WorkflowSchemaLabels{
				Severity:    []string{"critical"},
				Environment: []string{"production"},
				Component:   "pod",
				Priority:    "P1",
			},
			Execution: &models.WorkflowExecution{
				Engine: engine,
				Bundle: ValidBundleRef,
			},
			Parameters: []models.WorkflowParameter{
				{
					Name:        "NAMESPACE",
					Type:        "string",
					Required:    true,
					Description: "Target namespace",
				},
			},
		},
	}
}

// FindWorkspaceRoot walks up the directory tree from cwd looking for go.mod.
// This ensures fixture paths resolve correctly regardless of where ginkgo
// sets the working directory (it uses the test package directory).
func FindWorkspaceRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}
