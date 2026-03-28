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

package transport

import "github.com/jordigilh/kubernaut/pkg/kubernautagent/config"

// IsSensitiveSource returns true if the header value comes from a secret-backed
// source (secretKeyRef or filePath). Value-sourced headers are considered
// non-sensitive since they are placed in plaintext config by the operator.
//
// Authority: DD-HAPI-019-003 (G4: Credential Scrubbing)
func IsSensitiveSource(def config.HeaderDefinition) bool {
	return def.SecretKeyRef != "" || def.FilePath != ""
}

// RedactHeaderValue returns "[REDACTED]" if sensitive is true, otherwise
// returns the original value. Used for log output, error messages, and metrics labels.
//
// Authority: DD-HAPI-019-003 (G4: Credential Scrubbing)
func RedactHeaderValue(value string, sensitive bool) string {
	if sensitive {
		return "[REDACTED]"
	}
	return value
}
