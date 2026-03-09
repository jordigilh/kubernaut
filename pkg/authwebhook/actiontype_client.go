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

package authwebhook

import (
	"context"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ActionTypeCatalogClient defines the DS REST API operations required by the AW
// handler to manage action types on behalf of CRD lifecycle events.
// BR-WORKFLOW-007: ActionType CRD lifecycle management via AW bridge.
type ActionTypeCatalogClient interface {
	CreateActionType(ctx context.Context, name string, description ogenclient.ActionTypeDescription, registeredBy string) (*ActionTypeRegistrationResult, error)
	UpdateActionType(ctx context.Context, name string, description ogenclient.ActionTypeDescription, updatedBy string) (*ActionTypeUpdateResult, error)
	DisableActionType(ctx context.Context, name string, disabledBy string) (*ActionTypeDisableResult, error)
}
