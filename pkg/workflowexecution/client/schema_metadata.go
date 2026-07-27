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

// Package client provides clients for querying external services from the WFE controller.
package client

import (
	"encoding/json"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// SchemaMetadata bundles all workflow catalog artifacts needed by the WE
// reconciler. Issue #1661 Change 11e (DD-WORKFLOW-018) retired the
// DataStorage round-trip that used to populate this from
// OgenWorkflowQuerier.GetWorkflowSchemaMetadata -- resolveSchemaMetadata now
// builds executor.CreateOptions directly from the CRD-embedded WorkflowRef
// snapshot and always returns a nil *SchemaMetadata (kept only for call-site
// signature stability). The type itself is retained here as the documented
// shape of what a schema-derived catalog lookup would return, should one be
// reintroduced.
type SchemaMetadata struct {
	// Engine is the execution engine from the DS catalog entry (e.g. "tekton", "job", "ansible").
	// Issue #518: resolved at runtime, not from the WFE spec.
	Engine string
	// WorkflowName is the human-readable workflow name from the DS catalog entry.
	WorkflowName string
	// EngineConfig is the raw JSON engine-specific configuration extracted from
	// the schema's execution.engineConfig section. nil when absent.
	// DD-WORKFLOW-017: execution details come from the workflow catalog entry.
	EngineConfig json.RawMessage
	// Dependencies are the infrastructure resources (Secrets, ConfigMaps) declared
	// in the workflow schema. DD-WE-006.
	Dependencies *models.WorkflowDependencies
	// DeclaredParameterNames is the set of parameter names declared in the
	// workflow schema. #243: defense-in-depth parameter filtering.
	//   nil   → no schema content, no filtering (backward compatible)
	//   empty → schema exists but declares no params, strip all
	DeclaredParameterNames map[string]bool
	// ExecutionBundle is the OCI image reference for the workflow's execution bundle.
	// Empty when the catalog entry does not specify a bundle.
	ExecutionBundle string
	// ExecutionBundleDigest is the sha256 digest of the execution bundle image.
	// Empty when the catalog entry does not specify a digest.
	ExecutionBundleDigest string
}
