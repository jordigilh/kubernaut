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

// Package uuid provides deterministic UUID v5 generation for Kubernaut workflows.
//
// All workflow identifiers across Kubernaut (DataStorage, Mock LLM, Kubernaut Agent) are
// derived from the workflow name using a fixed namespace, guaranteeing that
// independent services produce identical UUIDs for the same workflow without
// any cross-service synchronization (Issue #548).
package uuid

import (
	"crypto/sha1"
	"fmt"
)

// kubernautNamespace is the fixed UUID v5 namespace for all Kubernaut workflow IDs.
var kubernautNamespace = [16]byte{
	0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1,
	0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
}

// DeterministicUUID generates a UUID v5 from a workflow name.
// The same workflow name always produces the same UUID, enabling independent
// services to agree on workflow identity without ConfigMap synchronization.
func DeterministicUUID(workflowName string) string {
	h := sha1.New()
	h.Write(kubernautNamespace[:])
	h.Write([]byte(workflowName))
	sum := h.Sum(nil)

	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
