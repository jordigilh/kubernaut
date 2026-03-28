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

package uuid

import "github.com/google/uuid"

// kubernautNamespace is the fixed UUIDv5 namespace for all kubernaut
// deterministic IDs. Changing this value breaks all previously generated
// deterministic UUIDs — treat it as immutable once deployed.
var kubernautNamespace = uuid.MustParse("6ba7b810-9dad-51d0-80b7-00c04fd430c8")

// DeterministicUUID derives a standards-compliant UUIDv5 (RFC 4122) from
// a content hash string. The same contentHash always produces the same UUID,
// enabling PVC-wipe resilience: re-registering the same CRD content after
// a database wipe recovers the original workflow_id.
func DeterministicUUID(contentHash string) string {
	return uuid.NewSHA1(kubernautNamespace, []byte(contentHash)).String()
}
