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

// Package agentclient provides the Kubernaut Agent OpenAPI client.
package agentclient

// To regenerate the client:
//   1. Ensure ogen is installed: go install github.com/ogen-go/ogen/cmd/ogen@latest
//   2. Run: go generate ./pkg/agentclient/
//
// The OpenAPI spec is located at: api/openapi.json
//
// Note: Makefile sets PATH to include $(LOCALBIN) before running go generate
//go:generate sh -c "ogen --target . --package agentclient --clean ../../../api/openapi.json 2>/dev/null || $(go env GOPATH)/bin/ogen --target . --package agentclient --clean ../../../api/openapi.json"









