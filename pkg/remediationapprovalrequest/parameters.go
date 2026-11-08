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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ValidateSnapshotPresence rejects old or incomplete approval values.
// Schema and declared names are read from the verified live workflow.
func ValidateSnapshotPresence(spec *remediationv1.RemediationApprovalRequestSpec) error {
	if spec == nil || spec.Workflow == nil || spec.Workflow.WorkflowID == "" || spec.Parameters == nil || spec.TargetContext == nil {
		return fmt.Errorf("required approval snapshot is missing")
	}
	return nil
}

// ValidateTypedParametersAgainstWorkflow validates a complete typed parameter
// map against the schema on a verified live RemediationWorkflow. The workflow
// is ephemeral and its schema is never copied into a CRD snapshot.
func ValidateTypedParametersAgainstWorkflow(workflow *remediationworkflowv1.RemediationWorkflow, values sharedtypes.TypedParameterMap) error {
	if workflow == nil || workflow.Status.WorkflowID == "" {
		return fmt.Errorf("verified workflow is missing workflow identity")
	}
	if values == nil {
		return fmt.Errorf("parameter values are missing")
	}
	if _, err := json.Marshal(values); err != nil {
		return fmt.Errorf("validate typed parameter transport: %w", err)
	}
	schema := buildValidationSchemaFromWorkflow(workflow, values)
	return validateTypedParameterSet(schema, values)
}

// ValidateTypedParameterSet validates typed values against an ephemeral live
// workflow schema. It is used by KA before handoff when the catalog adapter has
// already read the authoritative RemediationWorkflow spec. Managed target
// parameters are admitted for the same reason as the controller path: KA
// injects them from the authenticated remediation target rather than from the
// caller's selection.
func ValidateTypedParameterSet(schema []models.WorkflowParameter, values sharedtypes.TypedParameterMap) error {
	if values == nil {
		values = make(sharedtypes.TypedParameterMap)
	}
	return validateTypedParameterSet(augmentValidationSchema(schema, values), values)
}

// ValidateParameterOverrideWithWorkflow validates the complete effective
// parameter map against the verified live workflow schema. The workflow is an
// ephemeral trusted read; its schema is never copied into the approval object.
func ValidateParameterOverrideWithWorkflow(
	spec *remediationv1.RemediationApprovalRequestSpec,
	workflow *remediationworkflowv1.RemediationWorkflow,
	changes sharedtypes.TypedParameterMap,
) error {
	if spec == nil || spec.Workflow == nil || spec.Workflow.WorkflowID == "" || spec.Parameters == nil || spec.TargetContext == nil {
		return fmt.Errorf("required approval values are missing")
	}
	if workflow == nil || workflow.Status.WorkflowID == "" || workflow.Status.WorkflowID != spec.Workflow.WorkflowID {
		return fmt.Errorf("verified workflow does not match the reviewed workflow ID")
	}
	effective, err := mergeParameterChanges(spec.Parameters, changes)
	if err != nil {
		return err
	}
	return ValidateTypedParametersAgainstWorkflow(workflow, effective)
}

func mergeParameterChanges(original, changes sharedtypes.TypedParameterMap) (sharedtypes.TypedParameterMap, error) {
	effective := original.DeepCopy()
	if effective == nil {
		effective = make(sharedtypes.TypedParameterMap, len(original))
	}
	for name, value := range changes {
		originalValue, exists := effective[name]
		if !exists {
			return nil, fmt.Errorf("parameter %q is absent from original parameters", name)
		}
		if isManagedTargetParameter(name) && !reflect.DeepEqual(value, originalValue) {
			return nil, fmt.Errorf("parameter %q is locked", name)
		}
		effective[name] = value
	}
	return effective, nil
}

func buildValidationSchemaFromWorkflow(workflow *remediationworkflowv1.RemediationWorkflow, effective sharedtypes.TypedParameterMap) []models.WorkflowParameter {
	schema := make([]models.WorkflowParameter, 0, len(workflow.Spec.Parameters))
	for _, parameter := range workflow.Spec.Parameters {
		schema = append(schema, models.WorkflowParameter{
			Name:        parameter.Name,
			Type:        parameter.Type,
			Required:    parameter.Required,
			Description: parameter.Description,
			Enum:        parameter.Enum,
			Pattern:     parameter.Pattern,
			Minimum:     parameter.Minimum,
			Maximum:     parameter.Maximum,
			DependsOn:   parameter.DependsOn,
		})
	}
	return augmentValidationSchema(schema, effective)
}

func augmentValidationSchema(schema []models.WorkflowParameter, effective sharedtypes.TypedParameterMap) []models.WorkflowParameter {
	declared := make(map[string]struct{}, len(schema))
	for _, parameter := range schema {
		declared[parameter.Name] = struct{}{}
	}
	// Target identity values are injected by the remediation context and may
	// not be declared by a workflow. Keep them visible for dependency checks,
	// while rejecting every other undeclared value through the shared validator.
	for name := range effective {
		if isManagedTargetParameter(name) {
			if _, exists := declared[name]; !exists {
				schema = append(schema, models.WorkflowParameter{Name: name, Type: "string"})
				declared[name] = struct{}{}
			}
		}
	}
	for _, parameter := range schema {
		for _, dependency := range parameter.DependsOn {
			if isManagedTargetParameter(dependency) {
				if _, exists := declared[dependency]; !exists {
					schema = append(schema, models.WorkflowParameter{Name: dependency, Type: "string"})
					declared[dependency] = struct{}{}
				}
			}
		}
	}
	return schema
}

func validateTypedParameterSet(schema []models.WorkflowParameter, values sharedtypes.TypedParameterMap) error {
	if values == nil {
		return fmt.Errorf("parameter values are missing")
	}
	parameters := make(map[string]models.WorkflowParameter, len(schema))
	for _, parameter := range schema {
		parameters[parameter.Name] = parameter
	}
	engineValues := make(map[string]string, len(values))
	for name, value := range values {
		parameter, declared := parameters[name]
		if !declared {
			encoded, err := sharedtypes.TypedParameterValueToEngineValue(value)
			if err != nil {
				return fmt.Errorf("parameter %q: %w", name, err)
			}
			engineValues[name] = encoded
			continue
		}
		expectedKind := parameterKind(parameter.Type)
		if value.Kind != expectedKind {
			return fmt.Errorf("parameter %q: expected %s value, got %s", name, parameter.Type, value.Kind)
		}
		ordinary, err := sharedtypes.TypedParameterValueToJSONValue(value)
		if err != nil {
			return fmt.Errorf("parameter %q: %w", name, err)
		}
		encoded, err := models.TypedParameterValue(parameter, ordinary)
		if err != nil {
			return err
		}
		engineValues[name] = encoded
	}
	return models.ValidateParameterSet(schema, engineValues)
}

func parameterKind(schemaType string) string {
	if schemaType == "array" {
		return "array"
	}
	return schemaType
}

func isManagedTargetParameter(name string) bool {
	return strings.HasPrefix(name, "TARGET_RESOURCE_")
}
