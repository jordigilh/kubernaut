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

package workflowcatalog

import (
	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	dsschema "github.com/jordigilh/kubernaut/pkg/datastorage/schema"
)

// BuildWorkflowMeta converts catalog metadata into the single KA validation
// snapshot used by autonomous and interactive workflow selection.
//
// Schema parsing is fail-closed: a malformed schema leaves schema-derived
// fields empty so unvalidated LLM parameters cannot reach execution. The
// catalog-authoritative execution fields are copied independently.
func BuildWorkflowMeta(w *models.RemediationWorkflow, logger logr.Logger) parser.WorkflowMeta {
	meta := parser.WorkflowMeta{}
	if w == nil {
		return meta
	}

	meta.ExecutionEngine = string(w.ExecutionEngine)
	meta.Version = w.Version
	meta.Component = append([]string(nil), w.Labels.Component...)
	meta.ActionType = w.ActionType
	meta.WorkflowName = w.WorkflowName
	if w.ExecutionBundle != nil {
		meta.ExecutionBundle = *w.ExecutionBundle
	}
	if w.ExecutionBundleDigest != nil {
		meta.ExecutionBundleDigest = *w.ExecutionBundleDigest
	}
	if w.ServiceAccountName != nil {
		meta.ServiceAccountName = *w.ServiceAccountName
	}
	if w.ExecutionClusterID != nil {
		meta.ClusterID = *w.ExecutionClusterID
	}
	if w.EngineConfig != nil {
		raw := *w.EngineConfig
		meta.EngineConfig = &apiextensionsv1.JSON{Raw: raw}
	}

	if w.Content == "" {
		return meta
	}

	schemaParser := dsschema.NewParser()
	parsed, err := schemaParser.Parse(w.Content)
	if err != nil {
		logger.Error(err, "failed to parse workflow schema content, parameter validation will strip all LLM params",
			"workflow_id", w.WorkflowID)
		return meta
	}

	meta.Parameters = parsed.Parameters
	meta.DeclaredParameterNames = declaredParameterNames(parsed.Parameters)
	meta.Dependencies = schemaParser.ExtractDependencies(parsed)
	resources, err := schemaParser.ExtractResources(parsed)
	if err != nil {
		logger.Error(err, "failed to extract workflow execution resources, resources left unset",
			"workflow_id", w.WorkflowID)
		return meta
	}
	meta.Resources = resources
	return meta
}

func declaredParameterNames(params []models.WorkflowParameter) map[string]bool {
	names := make(map[string]bool, len(params))
	for _, parameter := range params {
		names[parameter.Name] = true
	}
	return names
}
