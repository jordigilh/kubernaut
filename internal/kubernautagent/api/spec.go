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

package api

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var OpenAPISpec []byte

// SpecHandler returns an http.HandlerFunc that serves the embedded
// OpenAPI specification as application/json.
func SpecHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(OpenAPISpec) //nolint:errcheck // best-effort write to HTTP response
	}
}
