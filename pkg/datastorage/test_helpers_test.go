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

package datastorage_test

import (
	"context"
)

// mockActionTypeValidator implements server.ActionTypeValidator for testing.
// Shared across workflow handler test files to avoid implicit coupling.
type mockActionTypeValidator struct {
	existsFn func(ctx context.Context, actionType string) (bool, error)
}

func (m *mockActionTypeValidator) ActionTypeExists(ctx context.Context, actionType string) (bool, error) {
	if m.existsFn != nil {
		return m.existsFn(ctx, actionType)
	}
	return true, nil // default: all types valid
}
