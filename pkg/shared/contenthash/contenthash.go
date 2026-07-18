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

// Package contenthash provides the deterministic content-hash and workflow-ID
// derivation algorithms shared by DataStorage and AuthWebhook for
// RemediationWorkflow catalog entries (DD-WORKFLOW-018, #1661 Change 8a).
//
// Both the hash and the UUID derived from it are pure, side-effect-free
// functions of their input: the same workflow content always produces the
// same content hash, and the same content hash always produces the same
// workflow_id. This is what makes workflow_id stable across a Postgres/PVC
// wipe (DS's catalog cache rebuilds from etcd) and safe to compute
// independently in AuthWebhook without a DataStorage round-trip.
package contenthash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
)

// kubernautNamespace is the fixed UUIDv5 namespace for all Kubernaut
// deterministic workflow IDs. Changing this value breaks every previously
// generated workflow_id -- treat it as immutable once deployed. Ported
// byte-for-byte from the pre-#1661 pkg/datastorage/uuid package so that
// pre-existing workflow_ids remain stable across the migration.
var kubernautNamespace = uuid.MustParse("6ba7b810-9dad-51d0-80b7-00c04fd430c8")

// ComputeContentHash returns the SHA-256 hex digest of content. content is
// expected to be the canonicalized JSON representation of a workflow
// definition (e.g. AuthWebhook's marshalCleanCRDContent output), so that
// Kubernetes runtime metadata never leaks into the hash.
func ComputeContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// DeterministicUUID derives a standards-compliant UUIDv5 (RFC 4122) from a
// content hash string. The same contentHash always produces the same UUID,
// enabling PVC-wipe resilience: re-registering the same CRD content after a
// database wipe recovers the original workflow_id.
func DeterministicUUID(contentHash string) string {
	return uuid.NewSHA1(kubernautNamespace, []byte(contentHash)).String()
}

// cleanCRDMetadata/cleanCRD mirror only the fields relevant to a workflow's
// definition -- see MarshalCleanCRDContent.
type cleanCRDMetadata struct {
	Name string `json:"name"`
}

type cleanCRD struct {
	APIVersion string                             `json:"apiVersion"`
	Kind       string                             `json:"kind"`
	Metadata   cleanCRDMetadata                   `json:"metadata"`
	Spec       rwv1alpha1.RemediationWorkflowSpec `json:"spec"`
}

// MarshalCleanCRDContent produces a JSON representation of the CRD that only
// includes the fields relevant to the workflow definition: apiVersion, kind,
// metadata.name, and spec. Kubernetes runtime metadata (UID, resourceVersion,
// creationTimestamp, managedFields, etc.) is excluded so that the content hash
// is deterministic across CRD delete+recreate cycles. Moved from AuthWebhook
// (#1661 Phase 55) so test code can compute the exact same hash/workflow_id
// AuthWebhook would, without a live admission webhook (e.g. envtest-only
// integration suites that never deploy AuthWebhook).
func MarshalCleanCRDContent(rw *rwv1alpha1.RemediationWorkflow) ([]byte, error) {
	apiVersion := rw.APIVersion
	if apiVersion == "" {
		apiVersion = "kubernaut.ai/v1alpha1"
	}
	kind := rw.Kind
	if kind == "" {
		kind = "RemediationWorkflow"
	}

	clean := cleanCRD{
		APIVersion: apiVersion,
		Kind:       kind,
		Metadata:   cleanCRDMetadata{Name: rw.Name},
		Spec:       rw.Spec,
	}
	return json.Marshal(clean)
}
