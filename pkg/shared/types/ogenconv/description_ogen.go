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

package ogenconv

import (
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// OgenDescriptionToShared converts an ogen-generated ActionTypeDescription
// (snake_case REST wire format) to the canonical shared type.
func OgenDescriptionToShared(d ogenclient.ActionTypeDescription) sharedtypes.StructuredDescription {
	return sharedtypes.StructuredDescription{
		What:          d.What,
		WhenToUse:     d.WhenToUse,
		WhenNotToUse:  d.WhenNotToUse.Value,
		Preconditions: d.Preconditions.Value,
	}
}

// SharedDescriptionToOgen converts the canonical shared type to an ogen-generated
// ActionTypeDescription (snake_case REST wire format).
func SharedDescriptionToOgen(d sharedtypes.StructuredDescription) ogenclient.ActionTypeDescription {
	desc := ogenclient.ActionTypeDescription{
		What:      d.What,
		WhenToUse: d.WhenToUse,
	}
	if d.WhenNotToUse != "" {
		desc.WhenNotToUse.SetTo(d.WhenNotToUse)
	}
	if d.Preconditions != "" {
		desc.Preconditions.SetTo(d.Preconditions)
	}
	return desc
}
