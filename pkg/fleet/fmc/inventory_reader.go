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

package fmc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const (
	// DefaultInventoryStaleAfter is the remaining proof life below which a
	// proven partition is served as `stale` (DD-FLEET-017 D3.2a). It must be
	// strictly smaller than the cache TTL (45s default) and larger than the
	// sync interval (30s default); otherwise one missed cycle jumps straight
	// from `fresh` to `unavailable` and `stale` is unreachable (§9.4).
	DefaultInventoryStaleAfter = 40 * time.Second

	// DefaultInventoryScanBudget bounds how many lexicographic batches one page
	// request may read while it fills the managed projection. Reaching it
	// truncates the projection, which is `partial` — never a smaller page that
	// claims to be complete (DD-FLEET-017 D2.6).
	DefaultInventoryScanBudget = 20

	// reasonStaleProof is the store's reason when the proof a read resolved with
	// no longer matches the committed generation. It is the one failure a single
	// re-resolve can fix, because the only way it appears is a commit landing
	// between the page's header and records calls (DD-FLEET-017 D2.5b).
	reasonStaleProof = "stale_proof"

	// staleProofRetryDelay is the pause before the single re-resolve. Publication
	// of a new generation happens immediately after its commit, so this only has
	// to span a scheduling hop.
	staleProofRetryDelay = 2 * time.Millisecond

	// maxPageAttempts is one read plus one bounded retry. No further attempt is
	// made, so a genuinely stale fence still fails closed.
	maxPageAttempts = 2
)

// errStaleProof marks a page read whose proof was superseded mid-read.
var errStaleProof = errors.New("stale proof")

// InventoryStore is the proof-gated cache page boundary the reader needs. It
// is satisfied by *scopecache.ValkeyStatusStore; tests substitute a fake
// because Valkey is an external dependency.
type InventoryStore interface {
	ReadInventoryPageHeader(context.Context, scopecache.InventoryPageRequest) (scopecache.InventoryPageHeader, error)
	ReadInventoryPageRecords(context.Context, scopecache.InventoryPageRecordsRequest) (scopecache.InventoryPageRecords, error)
	ReadInventoryResource(context.Context, scopecache.InventoryResourceRequest) (scopecache.InventoryResourceRecord, error)
}

// InventoryReaderConfig is the bounded tuning surface of the page reader.
type InventoryReaderConfig struct {
	Now        func() time.Time
	StaleAfter time.Duration
	ScanBudget int
}

// InventoryReader serves the managed-only inventory projection from the proven
// cache (DD-FLEET-017 D2/D3). It never re-derives scroll visibility, and it
// never falls back to another cluster or to a local reader.
type InventoryReader struct {
	store        InventoryStore
	registration registry.CurrentRegistrationGuard
	readiness    *scopecache.FMCReadinessSnapshot
	now          func() time.Time
	staleAfter   time.Duration
	scanBudget   int
}

// NewInventoryReader validates and creates the FMC inventory page reader.
func NewInventoryReader(store InventoryStore, registration registry.CurrentRegistrationGuard, readiness *scopecache.FMCReadinessSnapshot, cfg InventoryReaderConfig) (*InventoryReader, error) {
	if store == nil {
		return nil, fmt.Errorf("inventory page reader requires a cache store")
	}
	if registration == nil {
		return nil, fmt.Errorf("inventory page reader requires a registration guard")
	}
	if readiness == nil {
		return nil, fmt.Errorf("inventory page reader requires a readiness snapshot")
	}
	if cfg.StaleAfter < 0 {
		return nil, fmt.Errorf("inventory staleness threshold cannot be negative")
	}
	if cfg.ScanBudget < 0 {
		return nil, fmt.Errorf("inventory scan budget cannot be negative")
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.StaleAfter == 0 {
		cfg.StaleAfter = DefaultInventoryStaleAfter
	}
	if cfg.ScanBudget == 0 {
		cfg.ScanBudget = DefaultInventoryScanBudget
	}
	return &InventoryReader{
		store:        store,
		registration: registration,
		readiness:    readiness,
		now:          cfg.Now,
		staleAfter:   cfg.StaleAfter,
		scanBudget:   cfg.ScanBudget,
	}, nil
}

// List returns one bounded page of managed resources for a proven partition.
func (r *InventoryReader) List(ctx context.Context, filters inventory.Filters) (inventory.Page, error) {
	normalized, err := filters.Normalize()
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	// DD-FLEET-017 D2.5b: a page is read under one proof, but its header and
	// records are two calls. A commit landing between them supersedes the proof
	// and the page fails closed. Re-resolve once, after a short pause, so an
	// ordinary rotation is invisible to the caller; a second failure still
	// fails closed.
	var page inventory.Page
	var listErr error
	for attempt := 0; attempt < maxPageAttempts; attempt++ {
		if attempt > 0 {
			if waitErr := waitForRetry(ctx); waitErr != nil {
				return inventory.StateOnly(inventory.StateUnavailable), waitErr
			}
		}
		fence, resolveErr := r.resolve(normalized.ClusterID)
		if resolveErr != nil {
			return inventory.StateOnly(inventory.StateUnavailable), resolveErr
		}
		page, listErr = r.listUnderProof(ctx, fence, normalized)
		if listErr == nil || !errors.Is(listErr, errStaleProof) {
			break
		}
	}
	return page, listErr
}

// listUnderProof runs one page read under an already resolved proof.
func (r *InventoryReader) listUnderProof(ctx context.Context, fence partitionFence, filters inventory.Filters) (inventory.Page, error) {
	if filters.ResourceID != "" {
		return r.listExact(ctx, fence, filters)
	}
	var page inventory.Page
	callbackErr := r.registration.WithCurrent(ctx, filters.ClusterID, fence.registrationUID, fence.revision, func(_ registry.ClusterInfo) error {
		var listErr error
		page, listErr = r.listFrom(ctx, fence, filters)
		return listErr
	})
	if callbackErr != nil {
		if errors.Is(callbackErr, inventory.ErrInvalidFilter) || errors.Is(callbackErr, inventory.ErrUnavailable) || errors.Is(callbackErr, inventory.ErrNotFound) {
			return pageOrUnavailable(page), callbackErr
		}
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: %w", inventory.ErrUnavailable, callbackErr)
	}
	return page, nil
}

// waitForRetry is the bounded pause before the single proof re-resolve.
func waitForRetry(ctx context.Context) error {
	timer := time.NewTimer(staleProofRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Get returns one managed resource by its opaque canonical ID.
func (r *InventoryReader) Get(ctx context.Context, clusterID, resourceID string) (inventory.Resource, error) {
	if err := inventory.ValidateResourceID(resourceID); err != nil {
		return inventory.Resource{}, err
	}
	fence, err := r.resolve(clusterID)
	if err != nil {
		return inventory.Resource{}, err
	}
	resource, _, err := r.readExact(ctx, fence, clusterID, resourceID)
	if err != nil {
		return inventory.Resource{}, err
	}
	return resource, nil
}

// partitionFence is the resolved proof identity of one cluster partition.
type partitionFence struct {
	partition       scopecache.PartitionStateKey
	registrationUID string
	revision        uint64
	writerFence     scopecache.WriterFence
	generation      uint64
}

func (r *InventoryReader) resolve(clusterID string) (partitionFence, error) {
	snapshot, found := r.registration.Snapshot(clusterID)
	if !found || snapshot.Info.RegistrationUID == "" {
		return partitionFence{}, fmt.Errorf("%w: cluster %q is not registered", inventory.ErrUnavailable, clusterID)
	}
	partition := scopecache.PartitionStateKey{ClusterID: clusterID, RegistrationUID: snapshot.Info.RegistrationUID}
	partitionReadiness, found := r.readiness.View().Partitions[partition]
	if !found {
		return partitionFence{}, fmt.Errorf("%w: partition for cluster %q is unknown", inventory.ErrUnavailable, clusterID)
	}
	proof, err := r.readiness.PartitionReady(partition, snapshot.Info.RegistrationUID, snapshot.Revision,
		partitionReadiness.WriterFence, partitionReadiness.Generation)
	if err != nil {
		return partitionFence{}, fmt.Errorf("%w: partition for cluster %q is not proven", inventory.ErrUnavailable, clusterID)
	}
	return partitionFence{
		partition:       partition,
		registrationUID: snapshot.Info.RegistrationUID,
		revision:        snapshot.Revision,
		writerFence:     proof.WriterFence,
		generation:      proof.Generation,
	}, nil
}

func (r *InventoryReader) listFrom(ctx context.Context, fence partitionFence, filters inventory.Filters) (inventory.Page, error) {
	prefix := scopecache.InventoryPrefix{
		Namespace: filters.Namespace,
		Kind:      filters.Kind,
		Group:     filters.Group,
		Version:   filters.Version,
		Name:      filters.Name,
	}
	page, drifted, err := r.scan(ctx, fence, prefix, filters, filters.Cursor)
	if err != nil {
		return page, err
	}
	if !drifted {
		return page, nil
	}
	// DD-FLEET-017 D2.5: the cursor's identity is gone because the cache moved
	// on. Re-run the sorted projection from the start — never 400, never
	// unavailable — exactly like policyread.cursorStart.
	page, drifted, err = r.scan(ctx, fence, prefix, filters, "")
	if err != nil {
		return page, err
	}
	if drifted {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: cursor drift without a cursor", inventory.ErrUnavailable)
	}
	return page, nil
}

// inventoryBatch is one lexicographic batch of the projection: the identities
// of the range, the records the page needs, and the resolved managed set.
type inventoryBatch struct {
	state             scopecache.InventoryReadState
	reason            string
	remainingProofTTL time.Duration
	included          []inventory.Resource
	rangeExhausted    bool
	lastMemberID      string
}

// scan reads lexicographic batches until the page is full, the range ends, or
// the scan budget truncates the projection.
func (r *InventoryReader) scan(ctx context.Context, fence partitionFence, prefix scopecache.InventoryPrefix, filters inventory.Filters, startCursor string) (inventory.Page, bool, error) {
	projected := make([]inventory.Resource, 0, filters.Limit+1)
	cursor := startCursor
	remainingProof := time.Duration(0)
	haveProof := false
	for batch := 0; batch < r.scanBudget; batch++ {
		result, err := r.readBatch(ctx, fence, prefix, filters.Limit+1-len(projected), cursor)
		if err != nil {
			return inventory.StateOnly(inventory.StateUnavailable), false, err
		}
		switch result.state {
		case scopecache.InventoryReadDrift:
			return inventory.Page{}, true, nil
		case scopecache.InventoryReadEmpty:
			return r.assemble(nil, false, result.remainingProofTTL), false, nil
		case scopecache.InventoryReadOK:
		default:
			if result.reason == reasonStaleProof {
				return inventory.StateOnly(inventory.StateUnavailable), false,
					fmt.Errorf("%w: %w", inventory.ErrUnavailable, errStaleProof)
			}
			return inventory.StateOnly(inventory.StateUnavailable), false,
				fmt.Errorf("%w: inventory proof is not available (%s)", inventory.ErrUnavailable, result.reason)
		}
		if !haveProof {
			remainingProof, haveProof = result.remainingProofTTL, true
		}
		projected = append(projected, result.included...)
		if len(projected) > filters.Limit {
			return r.assemble(projected[:filters.Limit], true, remainingProof), false, nil
		}
		if result.rangeExhausted {
			return r.assemble(projected, false, remainingProof), false, nil
		}
		cursor = result.lastMemberID
	}
	// The projection could not be completed within its scan budget, so this
	// page is truncated rather than complete (DD-FLEET-017 D2.6).
	return inventory.StateOnly(inventory.StatePartial), false,
		fmt.Errorf("%w: inventory projection exceeded its scan budget", inventory.ErrUnavailable)
}

// readBatch reads one identity page and the records it needs, then resolves the
// managed projection for it.
func (r *InventoryReader) readBatch(ctx context.Context, fence partitionFence, prefix scopecache.InventoryPrefix, limit int, cursor string) (inventoryBatch, error) {
	header, err := r.store.ReadInventoryPageHeader(ctx, scopecache.InventoryPageRequest{
		Partition:                fence.partition,
		RegistrationUID:          fence.registrationUID,
		ExpectedRegistryRevision: fence.revision,
		WriterFence:              fence.writerFence,
		Generation:               fence.generation,
		Prefix:                   prefix,
		Cursor:                   cursor,
		Limit:                    limit,
	})
	if err != nil {
		return inventoryBatch{}, err
	}
	result := inventoryBatch{
		state:             header.State,
		reason:            header.Reason,
		remainingProofTTL: header.RemainingProofTTL,
	}
	if header.State != scopecache.InventoryReadOK {
		return result, nil
	}
	members := make([]scopecache.InventoryMember, 0, len(header.Members))
	for _, member := range header.Members {
		if prefix.Matches(member.Identity) {
			members = append(members, member)
		}
	}
	// Fewer identities than requested is the only proof that the range ended.
	result.rangeExhausted = len(header.Members) < limit
	if len(header.Members) > 0 {
		result.lastMemberID = header.Members[len(header.Members)-1].PublicID
	}
	if len(members) == 0 {
		return result, nil
	}
	records, err := r.store.ReadInventoryPageRecords(ctx, scopecache.InventoryPageRecordsRequest{
		Partition:                fence.partition,
		RegistrationUID:          fence.registrationUID,
		ExpectedRegistryRevision: fence.revision,
		WriterFence:              fence.writerFence,
		Generation:               fence.generation,
		Members:                  membersOf(members),
		Namespaces:               namespacesOf(members),
	})
	if err != nil {
		return inventoryBatch{}, err
	}
	if records.State != scopecache.InventoryReadOK {
		if records.Reason == reasonStaleProof {
			return inventoryBatch{}, fmt.Errorf("%w: %w", inventory.ErrUnavailable, errStaleProof)
		}
		return inventoryBatch{}, fmt.Errorf("%w: inventory records are not available (%s)", inventory.ErrUnavailable, records.Reason)
	}
	for _, record := range records.Records {
		resource, included, projectErr := r.project(record, records.NamespaceLabels)
		if projectErr != nil {
			return inventoryBatch{}, projectErr
		}
		if included {
			result.included = append(result.included, resource)
		}
	}
	return result, nil
}

// assemble builds the wire page through the shared inventory.AssemblePage
// (TP-2661 refactor-in-cycle dedup): `as_of` is the projection completion
// time and freshness is derived from remaining proof life, never from a query
// time (DD-FLEET-017 D3.1/D3.2a).
func (r *InventoryReader) assemble(items []inventory.Resource, hasMore bool, remainingProof time.Duration) inventory.Page {
	now := r.now()
	return inventory.AssemblePage(items, hasMore, now, inventory.Freshness{State: r.freshnessState(remainingProof), AsOf: now})
}

// freshnessState is derived from remaining proof life, never from a query time
// (DD-FLEET-017 D3.1/D3.2a).
func (r *InventoryReader) freshnessState(remainingProof time.Duration) inventory.FreshnessState {
	if remainingProof >= r.staleAfter {
		return inventory.FreshnessFresh
	}
	return inventory.FreshnessStale
}

// project resolves the managed projection for one committed record. Only
// Scope.Included records are part of the page (DD-FLEET-017 D2.2); inheritance
// reuses the shared ADR-053 resolver (§3.7).
func (r *InventoryReader) project(record scopecache.InventoryRecord, namespaceLabels map[string]scopecache.InventoryNamespaceLabel) (inventory.Resource, bool, error) {
	identity := record.Identity
	clusterScoped := scope.IsClusterScopedKind(identity.Kind) || identity.Namespace == ""
	namespaceState := scope.LabelStateAbsent
	// ADR-053 precedence: an explicit true/false (or an invalid value) settles
	// the decision on the resource alone, so the namespace record is only
	// required when the resource label is absent and inheritance can apply.
	if !clusterScoped && record.LabelState == scope.LabelStateAbsent {
		namespaceLabel, found := namespaceLabels[identity.Namespace]
		if !found || !namespaceLabel.Present {
			// Fail closed: without the namespace record the inherited label is
			// unproven, and an unproven resource must never be published as
			// unmanaged.
			return inventory.Resource{}, false, fmt.Errorf("%w: namespace record for %q is not observable", inventory.ErrUnavailable, identity.Namespace)
		}
		namespaceState = namespaceLabel.LabelState
	}
	resolution := scope.ResolveLabelStates(record.LabelState, namespaceState, clusterScoped)
	if !resolution.Included {
		return inventory.Resource{}, false, nil
	}
	scopeIdentity := scope.ResourceIdentity{
		ClusterID: identity.ClusterID,
		Group:     identity.Group,
		Version:   identity.Version,
		Kind:      identity.Kind,
		Namespace: identity.Namespace,
		Name:      identity.Name,
	}
	resourceID, err := inventory.CanonicalResourceIDChecked(scopeIdentity)
	if err != nil {
		return inventory.Resource{}, false, fmt.Errorf("%w: %w", inventory.ErrUnavailable, err)
	}
	return inventory.Resource{
		ID:         resourceID,
		ClusterID:  identity.ClusterID,
		Group:      identity.Group,
		Version:    identity.Version,
		Kind:       identity.Kind,
		Namespace:  identity.Namespace,
		Name:       identity.Name,
		Scope:      inventoryScope(resolution),
		ObservedAt: r.now(),
	}, true, nil
}

// listExact answers a resource_id filter with the same proof-derived
// freshness the list projection uses (DD-FLEET-017 D3.2a): a nearly-expired
// proof is stale, and a proof that cannot be served is unavailable state-only.
func (r *InventoryReader) listExact(ctx context.Context, fence partitionFence, filters inventory.Filters) (inventory.Page, error) {
	resource, remainingProof, err := r.readExact(ctx, fence, filters.ClusterID, filters.ResourceID)
	if err != nil {
		if errors.Is(err, inventory.ErrNotFound) {
			return r.assemble(nil, false, remainingProof), nil
		}
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	return r.assemble([]inventory.Resource{resource}, false, remainingProof), nil
}

// exactOutcome is the result of reading one record under a resolved proof.
type exactOutcome struct {
	resource       inventory.Resource
	remainingProof time.Duration
	unproven       bool
	unprovenReason string
	notFound       bool
}

// readExact returns the projected resource with the remaining life of the proof
// it was read under, so page assembly never invents a freshness claim.
func (r *InventoryReader) readExact(ctx context.Context, fence partitionFence, clusterID, resourceID string) (inventory.Resource, time.Duration, error) {
	var outcome exactOutcome
	callbackErr := r.registration.WithCurrent(ctx, clusterID, fence.registrationUID, fence.revision, func(_ registry.ClusterInfo) error {
		var readErr error
		outcome, readErr = r.readExactRecord(ctx, fence, resourceID)
		return readErr
	})
	if callbackErr != nil {
		if errors.Is(callbackErr, inventory.ErrUnavailable) {
			return inventory.Resource{}, outcome.remainingProof, callbackErr
		}
		return inventory.Resource{}, outcome.remainingProof, fmt.Errorf("%w: %w", inventory.ErrUnavailable, callbackErr)
	}
	if outcome.unproven {
		if outcome.unprovenReason == reasonStaleProof {
			return inventory.Resource{}, 0, fmt.Errorf("%w: %w", inventory.ErrUnavailable, errStaleProof)
		}
		return inventory.Resource{}, 0, fmt.Errorf("%w: partition proof for cluster %q is not available", inventory.ErrUnavailable, clusterID)
	}
	if outcome.notFound {
		return inventory.Resource{}, outcome.remainingProof, fmt.Errorf("%w: resource %q", inventory.ErrNotFound, resourceID)
	}
	return outcome.resource, outcome.remainingProof, nil
}

// readExactRecord performs the single record read and projects it. A missing or
// expired proof is an outage, never an assertion that the resource is absent.
func (r *InventoryReader) readExactRecord(ctx context.Context, fence partitionFence, resourceID string) (exactOutcome, error) {
	exact, err := r.store.ReadInventoryResource(ctx, scopecache.InventoryResourceRequest{
		Partition:                fence.partition,
		RegistrationUID:          fence.registrationUID,
		ExpectedRegistryRevision: fence.revision,
		WriterFence:              fence.writerFence,
		Generation:               fence.generation,
		PublicID:                 resourceID,
	})
	if err != nil {
		return exactOutcome{}, err
	}
	if exact.State == scopecache.InventoryReadUnproven {
		return exactOutcome{unproven: true, unprovenReason: exact.Reason}, nil
	}
	if exact.State != scopecache.InventoryReadOK {
		return exactOutcome{notFound: true, remainingProof: exact.RemainingProofTTL}, nil
	}
	if canonical, canonicalErr := inventory.CanonicalResourceIDChecked(scopeIdentityOf(exact.Record)); canonicalErr != nil || canonical != resourceID {
		return exactOutcome{}, fmt.Errorf("%w: record identity does not match the requested resource", inventory.ErrUnavailable)
	}
	namespaceLabels, err := r.namespaceLabels(ctx, fence, exact.Record)
	if err != nil {
		return exactOutcome{}, err
	}
	projected, included, err := r.project(exact.Record, namespaceLabels)
	if err != nil {
		return exactOutcome{}, err
	}
	if !included {
		return exactOutcome{notFound: true, remainingProof: exact.RemainingProofTTL}, nil
	}
	return exactOutcome{resource: projected, remainingProof: exact.RemainingProofTTL}, nil
}

// namespaceLabels reads the namespace record of an exact match. This is the
// only page-shaped call an exact read makes, and it is bounded to one record.
func (r *InventoryReader) namespaceLabels(ctx context.Context, fence partitionFence, record scopecache.InventoryRecord) (map[string]scopecache.InventoryNamespaceLabel, error) {
	identity := record.Identity
	if identity.Namespace == "" || scope.IsClusterScopedKind(identity.Kind) {
		return map[string]scopecache.InventoryNamespaceLabel{}, nil
	}
	publicID, err := inventory.CanonicalResourceIDChecked(scopeIdentityOf(record))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", inventory.ErrUnavailable, err)
	}
	page, err := r.store.ReadInventoryPageRecords(ctx, scopecache.InventoryPageRecordsRequest{
		Partition:                fence.partition,
		RegistrationUID:          fence.registrationUID,
		ExpectedRegistryRevision: fence.revision,
		WriterFence:              fence.writerFence,
		Generation:               fence.generation,
		Members:                  []string{scopecache.LexicalMember(identity, publicID)},
		Namespaces:               []string{identity.Namespace},
	})
	if err != nil {
		return nil, err
	}
	if page.State != scopecache.InventoryReadOK {
		return nil, fmt.Errorf("%w: namespace record is not observable (%s)", inventory.ErrUnavailable, page.Reason)
	}
	return page.NamespaceLabels, nil
}

func membersOf(members []scopecache.InventoryMember) []string {
	values := make([]string, 0, len(members))
	for _, member := range members {
		values = append(values, member.Member)
	}
	return values
}

func namespacesOf(members []scopecache.InventoryMember) []string {
	seen := make(map[string]struct{}, len(members))
	values := make([]string, 0, len(members))
	for _, member := range members {
		namespace := member.Identity.Namespace
		if namespace == "" || scope.IsClusterScopedKind(member.Identity.Kind) {
			continue
		}
		if _, duplicate := seen[namespace]; duplicate {
			continue
		}
		seen[namespace] = struct{}{}
		values = append(values, namespace)
	}
	return values
}

// inventoryScope projects the shared resolution onto the sanitized wire
// contract. No raw label, annotation or object data crosses this boundary.
func inventoryScope(resolution scope.Resolution) inventory.Scope {
	return inventory.Scope{
		Included:       resolution.Included,
		ResourceLabel:  inventory.LabelState(resolution.ResourceLabel),
		NamespaceLabel: inventory.LabelState(resolution.NamespaceLabel),
		Source:         inventory.ScopeSource(resolution.Source),
		ClusterScoped:  resolution.ClusterScoped,
	}
}

func scopeIdentityOf(record scopecache.InventoryRecord) scope.ResourceIdentity {
	return scope.ResourceIdentity{
		ClusterID: record.Identity.ClusterID,
		Group:     record.Identity.Group,
		Version:   record.Identity.Version,
		Kind:      record.Identity.Kind,
		Namespace: record.Identity.Namespace,
		Name:      record.Identity.Name,
	}
}

func pageOrUnavailable(page inventory.Page) inventory.Page {
	if page.State == "" {
		return inventory.StateOnly(inventory.StateUnavailable)
	}
	return page
}

var _ inventory.Reader = (*InventoryReader)(nil)
