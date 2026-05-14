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

package dlq

import "github.com/jordigilh/kubernaut/pkg/datastorage/eventdata"

const (
	MaxEventDataSize  = eventdata.MaxEventDataSize
	MaxEventDataDepth = eventdata.MaxEventDataDepth
)

// ValidateEventData delegates to the shared eventdata package.
// Kept for backward compatibility with existing callers.
func ValidateEventData(data []byte) error {
	return eventdata.ValidateEventData(data)
}

// ValidateJSONDepth delegates to the shared eventdata package.
func ValidateJSONDepth(data []byte, maxDepth int) error {
	return eventdata.ValidateJSONDepth(data, maxDepth)
}
