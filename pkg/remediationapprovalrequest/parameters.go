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

package remediationapprovalrequest

import (
	"fmt"
	"strings"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ValidateSnapshotPresence rejects old or incomplete approval snapshots.
func ValidateSnapshotPresence(spec *remediationv1.RemediationApprovalRequestSpec) error {
	if spec == nil || spec.Workflow == nil || spec.Workflow.WorkflowID == "" || spec.Workflow.DeclaredParameterNames == nil || spec.Parameters == nil || spec.ParameterSchema == nil || spec.TargetContext == nil {
		return fmt.Errorf("required approval snapshot is missing")
	}
	return nil
}

// ValidateParameterOverride checks the complete effective map against the
// frozen schema without changing the reviewed snapshot or submitted strings.
func ValidateParameterOverride(spec *remediationv1.RemediationApprovalRequestSpec, changes map[string]string) error {
	if err := ValidateSnapshotPresence(spec); err != nil {
		return err
	}
	effective, err := mergeParameterChanges(spec.Parameters, changes)
	if err != nil {
		return err
	}
	schema, err := buildValidationSchema(spec, effective)
	if err != nil {
		return err
	}
	return models.ValidateParameterSet(schema, effective)
}

func mergeParameterChanges(original, changes map[string]string) (map[string]string, error) {
	effective := make(map[string]string, len(original))
	for name, value := range original {
		effective[name] = value
	}
	for name, value := range changes {
		originalValue, exists := effective[name]
		if !exists {
			return nil, fmt.Errorf("parameter %q is absent from original parameters", name)
		}
		if isManagedTargetParameter(name) && value != originalValue {
			return nil, fmt.Errorf("parameter %q is locked", name)
		}
		effective[name] = value
	}
	return effective, nil
}

func buildValidationSchema(spec *remediationv1.RemediationApprovalRequestSpec, effective map[string]string) ([]models.WorkflowParameter, error) {
	schema := make([]models.WorkflowParameter, 0, len(spec.ParameterSchema))
	declaredParameterCount := 0
	for name := range spec.Workflow.DeclaredParameterNames {
		if !isManagedTargetParameter(name) {
			declaredParameterCount++
		}
	}
	schemaParameterCount := 0
	for _, parameter := range spec.ParameterSchema {
		if isManagedTargetParameter(parameter.Name) {
			continue
		}
		schemaParameterCount++
		if !spec.Workflow.DeclaredParameterNames[parameter.Name] {
			return nil, fmt.Errorf("parameter %q is not declared for execution", parameter.Name)
		}
		schema = append(schema, models.WorkflowParameter{
			Name: parameter.Name, Type: parameter.Type, Required: parameter.Required,
			Description: parameter.Description, Enum: parameter.Enum, Pattern: parameter.Pattern,
			Minimum: parameter.Minimum, Maximum: parameter.Maximum, DependsOn: parameter.DependsOn,
		})
	}
	if declaredParameterCount != schemaParameterCount {
		return nil, fmt.Errorf("parameter declarations do not match the approval schema")
	}
	managedNames := make(map[string]struct{})
	for name := range effective {
		if isManagedTargetParameter(name) {
			managedNames[name] = struct{}{}
		}
	}
	for _, parameter := range schema {
		for _, dependency := range parameter.DependsOn {
			if isManagedTargetParameter(dependency) {
				managedNames[dependency] = struct{}{}
			}
		}
	}
	// Keep managed target values visible only for dependency presence checks.
	// Synthetic unconstrained declarations prevent the shared validator from
	// rejecting injected keys as undeclared workflow parameters.
	for name := range managedNames {
		schema = append(schema, models.WorkflowParameter{Name: name, Type: "string"})
	}
	return schema, nil
}

func isManagedTargetParameter(name string) bool {
	return strings.HasPrefix(name, "TARGET_RESOURCE_")
}
