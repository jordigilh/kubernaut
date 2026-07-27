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

// export_test.go re-exports unexported symbols for black-box unit tests
// (package custom_test). This is the standard Go pattern for narrowing the
// API surface while preserving test access (#1053).
package custom

import "encoding/json"

var ListAvailableActionsSchema = func() json.RawMessage { return listAvailableActionsSchema }
var ListWorkflowsSchema = func() json.RawMessage { return listWorkflowsSchemaJSON }
var GetWorkflowSchema = func() json.RawMessage { return getWorkflowSchemaJSON }

var TransformPagination = transformPagination
var DecodeCursor = decodeCursor
