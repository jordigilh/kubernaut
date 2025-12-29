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

package audit

import (
	_ "embed"
)

// Auto-generate OpenAPI spec copy before build
// DD-API-002: OpenAPI Spec Loading Standard
// Source: api/openapi/data-storage-v1.yaml (single source of truth per ADR-031)
// Target: pkg/audit/openapi_spec_data.yaml (auto-generated)
//
// Usage: go generate ./... (or make generate)
//
//go:generate sh -c "cp ../../api/openapi/data-storage-v1.yaml openapi_spec_data.yaml"

// Embed OpenAPI spec at compile time
// Authority: api/openapi/data-storage-v1.yaml
// DD-API-002: OpenAPI Spec Loading Standard
//
// This file uses go:generate to automatically copy the spec from api/openapi/ to this directory
// before embedding. This maintains ADR-031 compliance (specs in api/openapi/) while working
// around the go:embed limitation (no ".." in paths).
//
//go:embed openapi_spec_data.yaml
var embeddedOpenAPISpec []byte
