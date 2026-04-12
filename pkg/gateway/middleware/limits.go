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

package middleware

import (
	"encoding/json"
	"net/http"

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"
)

// MaxRequestBodySize is the global body size limit applied by all middleware and
// handlers that read the request body. This constant is the single source of truth
// for the body cap -- both freshness validators and readParseValidateSignal use it.
// Issue #673 C-ADV-1: Ensures the limit is enforced at the earliest body-reading layer.
const MaxRequestBodySize int64 = 256 * 1024 // 256KB

// respondPayloadTooLarge writes an RFC 7807 compliant 413 error response.
// Used by freshness middleware when MaxBytesReader detects an oversized body.
func respondPayloadTooLarge(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)

	errorResponse := gwerrors.RFC7807Error{
		Type:   gwerrors.ErrorTypePayloadTooLarge,
		Title:  gwerrors.TitlePayloadTooLarge,
		Detail: "Request body too large",
		Status: http.StatusRequestEntityTooLarge,
	}
	_ = json.NewEncoder(w).Encode(errorResponse)
}
