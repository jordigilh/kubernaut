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

// Package scopecache provides a Valkey-backed scope cache client for checking
// whether remote cluster resources are managed by Kubernaut.
// Used by GW and RO services for low-latency federated scope checking.
package scopecache

import (
	"context"
	"errors"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// RecordState describes whether a cache entry is a verified observation or
// merely missing/uncertain. A missing entry is not verified unmanaged.
type RecordState string

const (
	RecordVerifiedPresent RecordState = "verified_present"
	RecordMissing         RecordState = "missing"
	RecordUncertain       RecordState = "uncertain"
)

// CachedObservation is the minimal presence-aware record needed by scope
// checks. It deliberately contains no resource body or arbitrary labels.
type CachedObservation struct {
	State      RecordState
	LabelState scope.LabelState
}

// ObservationReader is the required presence-aware cache-read boundary.
type ObservationReader interface {
	Read(ctx context.Context, key string) (CachedObservation, error)
}

// RegistrationUIDResolver supplies the current Kubernetes registration
// incarnation for a remote cluster. Without it, a status-store read cannot
// distinguish a deleted/recreated registration that reused the same name.
type RegistrationUIDResolver func(clusterID string) (string, bool)

// Client provides scope checking against the Fleet Metadata Cache in Valkey.
// Key format: kubernaut:managed:{clusterID}:{group}/{version}/{kind}:{namespace}/{name}
type Client struct {
	reader       ObservationReader
	statusStore  StatusStore
	registration registry.CurrentRegistrationGuard
	readiness    *FMCReadinessSnapshot
}

// NewClient creates a scope cache client backed by actual observations.
func NewClient(reader ObservationReader) *Client {
	return &Client{reader: reader}
}

// NewClientWithStatusStore creates a scope client that reads only the fenced
// v2 materialized state for the current registration incarnation.
func NewClientWithStatusStore(statusStore StatusStore, registration registry.CurrentRegistrationGuard, readiness *FMCReadinessSnapshot) (*Client, error) {
	if statusStore == nil {
		return nil, fmt.Errorf("scopecache: status store is required")
	}
	if registration == nil {
		return nil, fmt.Errorf("scopecache: registration guard is required")
	}
	if readiness == nil {
		return nil, fmt.Errorf("scopecache: readiness snapshot is required")
	}
	return &Client{statusStore: statusStore, registration: registration, readiness: readiness}, nil
}

// Check evaluates a resource using the presence-aware cache contract. A
// resource cache miss, expiry, backend error, invalid label, or uncertain
// record never falls through to a managed namespace. Namespace inheritance is
// considered only after the resource itself is verified present and unlabeled.
func (c *Client) Check(ctx context.Context, resource scope.ResourceIdentity) (scope.ScopeResult, error) {
	if ctx.Err() != nil {
		return unavailableResult("scope_canceled"), ctx.Err()
	}
	gvk, err := scope.GVKForIdentity(resource)
	if err != nil {
		return scope.ScopeResult{ //nolint:nilerr // DD-FLEET-015: invalid GVK is a completed domain outcome
			State:      scope.ScopeInvalid,
			Provenance: scope.ScopeSourceUnavailable,
			Reason:     "invalid_resource_gvk",
		}, nil
	}
	// BR-SCOPE-001 FR-SCOPE-004 / #2593: cluster-scoped remediation
	// remains deferred. This target boundary deliberately does not apply to
	// internal Namespace observation reads used for namespaced inheritance.
	if resource.Namespace == "" || scope.IsClusterScopedKind(resource.Kind) {
		return scope.ScopeResult{
			State: scope.ScopeUnsupported, Provenance: scope.ScopeSourceUnavailable,
			Reason: "cluster_scoped_target_unsupported",
		}, nil
	}
	if c.statusStore != nil {
		return c.checkFencedStatus(ctx, resource, gvk.Group, gvk.Version)
	}

	resourceKey, err := BuildKey(resource.ClusterID, gvk.Group, gvk.Version, gvk.Kind, resource.Namespace, resource.Name)
	if err != nil {
		return scope.ScopeResult{ //nolint:nilerr // DD-FLEET-015: invalid identity is a completed domain outcome
			State:      scope.ScopeInvalid,
			Provenance: scope.ScopeSourceUnavailable,
			Reason:     "invalid_resource_identity",
		}, nil
	}
	resourceRecord, err := c.readObservation(ctx, resourceKey)
	if err != nil {
		return unavailableResult("cache_error"), err
	}
	return c.checkResourceObservation(ctx, resource, resourceRecord)
}

func (c *Client) checkFencedStatus(ctx context.Context, resource scope.ResourceIdentity, group, version string) (scope.ScopeResult, error) {
	if c.registration == nil || c.readiness == nil || c.statusStore == nil {
		return unavailableResult("registration_unavailable"), fmt.Errorf("scopecache: fenced-read dependencies are incomplete")
	}
	snapshot, found := c.registration.Snapshot(resource.ClusterID)
	if !found || snapshot.Info.RegistrationUID == "" {
		return unavailableResult("registration_unavailable"), nil
	}
	partition := PartitionStateKey{ClusterID: resource.ClusterID, RegistrationUID: snapshot.Info.RegistrationUID}
	view := c.readiness.View()
	partitionReadiness, found := view.Partitions[partition]
	if !found {
		return unavailableResult("partition_unavailable"), nil
	}
	proof, err := c.readiness.PartitionReady(partition, snapshot.Info.RegistrationUID, snapshot.Revision,
		partitionReadiness.WriterFence, partitionReadiness.Generation)
	if err != nil {
		// The scope result carries the fail-closed reason; callers must not
		// convert an unavailable partition into managed=false.
		return unavailableResult("partition_unavailable"), nil //nolint:nilerr // readiness failure is intentionally represented in ScopeResult
	}
	request := FencedReadRequest{
		Partition:                partition,
		RegistrationUID:          snapshot.Info.RegistrationUID,
		ExpectedRegistryRevision: snapshot.Revision,
		WriterFence:              proof.WriterFence,
		Generation:               proof.Generation,
		Identity: ResourceIdentity{
			ClusterID: resource.ClusterID,
			Group:     group,
			Version:   version,
			Kind:      resource.Kind,
			Namespace: resource.Namespace,
			Name:      resource.Name,
		},
	}
	record, readErr := c.statusStore.ReadVerified(ctx, request)
	if readErr != nil {
		return unavailableResult("cache_error"), readErr
	}
	var result scope.ScopeResult
	callbackErr := c.registration.WithCurrent(ctx, resource.ClusterID, snapshot.Info.RegistrationUID, snapshot.Revision, func(_ registry.ClusterInfo) error {
		currentProof, proofErr := c.readiness.PartitionReady(partition, snapshot.Info.RegistrationUID, snapshot.Revision,
			proof.WriterFence, proof.Generation)
		if proofErr != nil {
			return proofErr
		}
		if !readMatchesProof(record, currentProof) {
			return fmt.Errorf("scopecache: fenced-read proof changed")
		}
		result, proofErr = c.mapFencedRecord(ctx, request, record, currentProof)
		return proofErr
	})
	if callbackErr != nil {
		return unavailableResult("registration_changed"), callbackErr
	}
	return result, nil
}

func (c *Client) mapFencedRecord(ctx context.Context, request FencedReadRequest, record FencedReadResult, proof FencedReadProof) (scope.ScopeResult, error) {
	if record.State != ReadVerified {
		return unavailableResult("cache_miss"), nil
	}
	switch record.Record.LabelState {
	case scope.LabelStateTrue:
		return verifiedResult(true, scope.ScopeSourceResourceLabel), nil
	case scope.LabelStateFalse:
		return verifiedResult(false, scope.ScopeSourceResourceLabel), nil
	case scope.LabelStateInvalid:
		return scope.ScopeResult{State: scope.ScopeInvalid, Provenance: scope.ScopeSourceUnavailable, Reason: "invalid_resource_label"}, nil
	case scope.LabelStateAbsent:
		if request.Identity.Namespace == "" || scope.IsClusterScopedKind(request.Identity.Kind) {
			return verifiedResult(false, scope.ScopeSourceNone), nil
		}
		namespaceRequest := request
		namespaceRequest.Identity = ResourceIdentity{
			ClusterID: request.Identity.ClusterID,
			Version:   "v1",
			Kind:      "Namespace",
			Name:      request.Identity.Namespace,
		}
		namespaceRecord, err := c.statusStore.ReadVerified(ctx, namespaceRequest)
		if err != nil {
			return unavailableResult("namespace_cache_error"), err
		}
		if namespaceRecord.State != ReadVerified {
			return unavailableResult("namespace_cache_miss"), nil
		}
		if !readMatchesProof(namespaceRecord, proof) {
			return unavailableResult("namespace_proof_mismatch"), fmt.Errorf("scopecache: namespace fenced-read proof changed")
		}
		switch namespaceRecord.Record.LabelState {
		case scope.LabelStateTrue:
			return verifiedResult(true, scope.ScopeSourceNamespaceLabel), nil
		case scope.LabelStateFalse:
			return verifiedResult(false, scope.ScopeSourceNamespaceLabel), nil
		case scope.LabelStateAbsent:
			return verifiedResult(false, scope.ScopeSourceNone), nil
		case scope.LabelStateInvalid:
			return scope.ScopeResult{State: scope.ScopeInvalid, Provenance: scope.ScopeSourceUnavailable, Reason: "invalid_namespace_label"}, nil
		default:
			return unavailableResult("uncertain_namespace_record"), nil
		}
	default:
		return unavailableResult("uncertain_record"), nil
	}
}

func readMatchesProof(result FencedReadResult, proof FencedReadProof) bool {
	return result.State == ReadVerified &&
		result.ProvenRegistryRevision == proof.RegistrationRevision &&
		result.ProvenGeneration == proof.Generation &&
		result.ProvenWriterFence == proof.WriterFence &&
		result.Record.Generation == proof.Generation &&
		result.RecordValid == proof.RecordValid &&
		result.IndexValid == proof.IndexValid &&
		result.StatusValid == proof.StatusValid &&
		result.RecordTTL.Valid == proof.RecordTTLValid &&
		result.IndexTTL.Valid == proof.IndexTTLValid &&
		result.StatusTTL.Valid == proof.StatusTTLValid
}

func (c *Client) checkResourceObservation(ctx context.Context, resource scope.ResourceIdentity, record CachedObservation) (scope.ScopeResult, error) {
	if record.State != RecordVerifiedPresent {
		if record.State == RecordMissing {
			return unavailableResult("cache_miss"), nil
		}
		return unavailableResult("uncertain_record"), nil
	}
	switch record.LabelState {
	case scope.LabelStateTrue:
		return verifiedResult(true, scope.ScopeSourceResourceLabel), nil
	case scope.LabelStateFalse:
		return verifiedResult(false, scope.ScopeSourceResourceLabel), nil
	case scope.LabelStateInvalid:
		return scope.ScopeResult{
			State:      scope.ScopeInvalid,
			Provenance: scope.ScopeSourceUnavailable,
			Reason:     "invalid_resource_label",
		}, nil
	case scope.LabelStateAbsent:
		// Continue below: only a verified-present, unlabeled resource can
		// inherit a separately verified namespace label.
	default:
		return unavailableResult("uncertain_record"), nil
	}

	return c.checkNamespaceObservation(ctx, resource)
}

func (c *Client) checkNamespaceObservation(ctx context.Context, resource scope.ResourceIdentity) (scope.ScopeResult, error) {
	if resource.Namespace == "" || scope.IsClusterScopedKind(resource.Kind) {
		return verifiedResult(false, scope.ScopeSourceNone), nil
	}

	nsKey, err := BuildKey(resource.ClusterID, "", "v1", "Namespace", "", resource.Namespace)
	if err != nil {
		return scope.ScopeResult{
			State:      scope.ScopeInvalid,
			Provenance: scope.ScopeSourceUnavailable,
			Reason:     "invalid_namespace_identity",
		}, err
	}
	nsRecord, err := c.readObservation(ctx, nsKey)
	if err != nil {
		return unavailableResult("namespace_cache_error"), err
	}
	if nsRecord.State != RecordVerifiedPresent {
		if nsRecord.State == RecordMissing {
			return unavailableResult("namespace_cache_miss"), nil
		}
		return unavailableResult("uncertain_namespace_record"), nil
	}
	switch nsRecord.LabelState {
	case scope.LabelStateTrue:
		return verifiedResult(true, scope.ScopeSourceNamespaceLabel), nil
	case scope.LabelStateFalse:
		return verifiedResult(false, scope.ScopeSourceNamespaceLabel), nil
	case scope.LabelStateAbsent:
		return verifiedResult(false, scope.ScopeSourceNone), nil
	case scope.LabelStateInvalid:
		return scope.ScopeResult{
			State:      scope.ScopeInvalid,
			Provenance: scope.ScopeSourceUnavailable,
			Reason:     "invalid_namespace_label",
		}, nil
	default:
		return unavailableResult("uncertain_namespace_record"), nil
	}
}

func (c *Client) readObservation(ctx context.Context, key string) (CachedObservation, error) {
	if c.reader == nil {
		return CachedObservation{State: RecordUncertain}, fmt.Errorf("scopecache: cache reader is nil")
	}
	return c.reader.Read(ctx, key)
}

func verifiedResult(managed bool, provenance scope.ScopeSource) scope.ScopeResult {
	state := scope.ScopeVerifiedUnmanaged
	if managed {
		state = scope.ScopeVerifiedManaged
	}
	return scope.ScopeResult{
		State:      state,
		Provenance: provenance,
	}
}

func unavailableResult(reason string) scope.ScopeResult {
	return scope.ScopeResult{
		State:      scope.ScopeUnavailable,
		Provenance: scope.ScopeSourceUnavailable,
		Reason:     reason,
		Retryable:  true,
	}
}

// Compile-time verification that Client satisfies scope.ScopeChecker.
var _ scope.ScopeChecker = (*Client)(nil)

// ErrEmptyClusterID is returned when clusterID is empty.
var ErrEmptyClusterID = errors.New("scopecache: clusterID must not be empty")

// ErrEmptyKind is returned when kind is empty.
var ErrEmptyKind = errors.New("scopecache: kind must not be empty")

// ErrEmptyVersion is returned when version is empty (#2592: the strict-GVK
// contract forbids building a cache key for a Kind-only identity).
var ErrEmptyVersion = errors.New("scopecache: version must not be empty")

// ErrEmptyName is returned when name is empty.
var ErrEmptyName = errors.New("scopecache: name must not be empty")

// BuildKey constructs the Valkey key for a managed resource entry.
// Returns an error if required parameters (clusterID, kind, name) are empty.
func BuildKey(clusterID, group, version, kind, namespace, name string) (string, error) {
	if clusterID == "" {
		return "", ErrEmptyClusterID
	}
	if kind == "" {
		return "", ErrEmptyKind
	}
	if name == "" {
		return "", ErrEmptyName
	}
	if version == "" {
		return "", ErrEmptyVersion
	}
	gvr := fmt.Sprintf("%s/%s/%s", group, version, kind)
	nsName := fmt.Sprintf("%s/%s", namespace, name)
	return fmt.Sprintf("kubernaut:managed:%s:%s:%s", clusterID, gvr, nsName), nil
}
