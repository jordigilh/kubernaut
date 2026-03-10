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

// Package workflowschema provides conversion functions between the WorkflowSchema
// (parser/DS model) and RemediationWorkflowSpec (CRD API type).
//
// RF-4: These two types are structurally near-identical but differ in how they
// represent freeform data (interface{} vs *apiextensionsv1.JSON) and nullability
// (pointer vs value for execution/dependencies). This converter bridges the gap.
package workflowschema

import (
	"encoding/json"
	"fmt"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// SpecToSchema converts a CRD RemediationWorkflowSpec to the DS WorkflowSchema model.
// Used by the AW handler when forwarding CRD content to DS for registration.
func SpecToSchema(spec rwv1alpha1.RemediationWorkflowSpec) (*models.WorkflowSchema, error) {
	schema := &models.WorkflowSchema{
		Metadata: models.WorkflowSchemaMetadata{
			WorkflowName: spec.Metadata.WorkflowName,
			Version:      spec.Metadata.Version,
			Description: models.WorkflowDescription{
				What:          spec.Metadata.Description.What,
				WhenToUse:     spec.Metadata.Description.WhenToUse,
				WhenNotToUse:  spec.Metadata.Description.WhenNotToUse,
				Preconditions: spec.Metadata.Description.Preconditions,
			},
		},
		ActionType: spec.ActionType,
		Labels: models.WorkflowSchemaLabels{
			Severity:    spec.Labels.Severity,
			Environment: spec.Labels.Environment,
			Component:   spec.Labels.Component,
			Priority:    spec.Labels.Priority,
		},
		CustomLabels: spec.CustomLabels,
	}

	for _, m := range spec.Metadata.Maintainers {
		schema.Metadata.Maintainers = append(schema.Metadata.Maintainers, models.WorkflowMaintainer{
			Name:  m.Name,
			Email: m.Email,
		})
	}

	if spec.DetectedLabels != nil && spec.DetectedLabels.Raw != nil {
		dl := &models.DetectedLabelsSchema{}
		if err := json.Unmarshal(spec.DetectedLabels.Raw, dl); err != nil {
			return nil, fmt.Errorf("unmarshal detectedLabels: %w", err)
		}
		schema.DetectedLabels = dl
	}

	exec := &models.WorkflowExecution{
		Engine:       spec.Execution.Engine,
		Bundle:       spec.Execution.Bundle,
		BundleDigest: spec.Execution.BundleDigest,
	}
	if spec.Execution.EngineConfig != nil && spec.Execution.EngineConfig.Raw != nil {
		var raw interface{}
		if err := json.Unmarshal(spec.Execution.EngineConfig.Raw, &raw); err != nil {
			return nil, fmt.Errorf("unmarshal engineConfig: %w", err)
		}
		exec.EngineConfig = raw
	}
	schema.Execution = exec

	if spec.Dependencies != nil {
		deps := &models.WorkflowDependencies{}
		for _, s := range spec.Dependencies.Secrets {
			deps.Secrets = append(deps.Secrets, models.ResourceDependency{Name: s.Name})
		}
		for _, cm := range spec.Dependencies.ConfigMaps {
			deps.ConfigMaps = append(deps.ConfigMaps, models.ResourceDependency{Name: cm.Name})
		}
		schema.Dependencies = deps
	}

	for _, p := range spec.Parameters {
		schema.Parameters = append(schema.Parameters, convertCRDParamToSchema(p))
	}
	for _, p := range spec.RollbackParameters {
		schema.RollbackParameters = append(schema.RollbackParameters, convertCRDParamToSchema(p))
	}

	return schema, nil
}

// SchemaToSpec converts a DS WorkflowSchema model to a CRD RemediationWorkflowSpec.
// Used when populating CRD objects from parsed workflow schemas.
func SchemaToSpec(schema *models.WorkflowSchema) (*rwv1alpha1.RemediationWorkflowSpec, error) {
	spec := &rwv1alpha1.RemediationWorkflowSpec{
		Metadata: rwv1alpha1.RemediationWorkflowMetadata{
			WorkflowName: schema.Metadata.WorkflowName,
			Version:      schema.Metadata.Version,
			Description: rwv1alpha1.RemediationWorkflowDescription{
				What:          schema.Metadata.Description.What,
				WhenToUse:     schema.Metadata.Description.WhenToUse,
				WhenNotToUse:  schema.Metadata.Description.WhenNotToUse,
				Preconditions: schema.Metadata.Description.Preconditions,
			},
		},
		ActionType: schema.ActionType,
		Labels: rwv1alpha1.RemediationWorkflowLabels{
			Severity:    schema.Labels.Severity,
			Environment: schema.Labels.Environment,
			Component:   schema.Labels.Component,
			Priority:    schema.Labels.Priority,
		},
		CustomLabels: schema.CustomLabels,
	}

	for _, m := range schema.Metadata.Maintainers {
		spec.Metadata.Maintainers = append(spec.Metadata.Maintainers, rwv1alpha1.RemediationWorkflowMaintainer{
			Name:  m.Name,
			Email: m.Email,
		})
	}

	if schema.DetectedLabels != nil {
		raw, err := json.Marshal(schema.DetectedLabels)
		if err != nil {
			return nil, fmt.Errorf("marshal detectedLabels: %w", err)
		}
		spec.DetectedLabels = &apiextensionsv1.JSON{Raw: raw}
	}

	if schema.Execution != nil {
		spec.Execution = rwv1alpha1.RemediationWorkflowExecution{
			Engine:       schema.Execution.Engine,
			Bundle:       schema.Execution.Bundle,
			BundleDigest: schema.Execution.BundleDigest,
		}
		if schema.Execution.EngineConfig != nil {
			raw, err := json.Marshal(schema.Execution.EngineConfig)
			if err != nil {
				return nil, fmt.Errorf("marshal engineConfig: %w", err)
			}
			spec.Execution.EngineConfig = &apiextensionsv1.JSON{Raw: raw}
		}
	}

	if schema.Dependencies != nil {
		deps := &rwv1alpha1.RemediationWorkflowDependencies{}
		for _, s := range schema.Dependencies.Secrets {
			deps.Secrets = append(deps.Secrets, rwv1alpha1.RemediationWorkflowResourceDependency{Name: s.Name})
		}
		for _, cm := range schema.Dependencies.ConfigMaps {
			deps.ConfigMaps = append(deps.ConfigMaps, rwv1alpha1.RemediationWorkflowResourceDependency{Name: cm.Name})
		}
		spec.Dependencies = deps
	}

	for _, p := range schema.Parameters {
		cp, err := convertSchemaParamToCRD(p)
		if err != nil {
			return nil, fmt.Errorf("convert parameter %q: %w", p.Name, err)
		}
		spec.Parameters = append(spec.Parameters, cp)
	}
	for _, p := range schema.RollbackParameters {
		cp, err := convertSchemaParamToCRD(p)
		if err != nil {
			return nil, fmt.Errorf("convert rollback parameter %q: %w", p.Name, err)
		}
		spec.RollbackParameters = append(spec.RollbackParameters, cp)
	}

	return spec, nil
}

func convertCRDParamToSchema(p rwv1alpha1.RemediationWorkflowParameter) models.WorkflowParameter {
	wp := models.WorkflowParameter{
		Name:        p.Name,
		Type:        p.Type,
		Required:    p.Required,
		Description: p.Description,
		Enum:        p.Enum,
		Pattern:     p.Pattern,
		Minimum:     p.Minimum,
		Maximum:     p.Maximum,
		DependsOn:   p.DependsOn,
	}
	if p.Default != nil && p.Default.Raw != nil {
		var v interface{}
		if err := json.Unmarshal(p.Default.Raw, &v); err == nil {
			wp.Default = v
		}
	}
	return wp
}

func convertSchemaParamToCRD(p models.WorkflowParameter) (rwv1alpha1.RemediationWorkflowParameter, error) {
	cp := rwv1alpha1.RemediationWorkflowParameter{
		Name:        p.Name,
		Type:        p.Type,
		Required:    p.Required,
		Description: p.Description,
		Enum:        p.Enum,
		Pattern:     p.Pattern,
		Minimum:     p.Minimum,
		Maximum:     p.Maximum,
		DependsOn:   p.DependsOn,
	}
	if p.Default != nil {
		raw, err := json.Marshal(p.Default)
		if err != nil {
			return cp, fmt.Errorf("marshal default value: %w", err)
		}
		cp.Default = &apiextensionsv1.JSON{Raw: raw}
	}
	return cp, nil
}
