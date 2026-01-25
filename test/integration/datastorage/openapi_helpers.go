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

package datastorage

import (
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// OPENAPI CLIENT HELPERS FOR INTEGRATION TESTS
// ========================================
// These helpers provide a clean interface for integration tests
// to use the typed OpenAPI client instead of raw HTTP + maps
//
// Benefits:
// - Type safety at compile time
// - API contract validation
// - Better IDE support and autocomplete
// - Easier refactoring when API changes
// ========================================

// createOpenAPIClient creates a configured OpenAPI client for integration tests
//
//nolint:unused // shared test helper used by multiple test files in this package
func createOpenAPIClient(baseURL string) (*ogenclient.Client, error) {
	return ogenclient.NewClient(baseURL)
}
