package policyread

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
)

// ScopeResolver is the structured local scope seam. scope.Manager is the
// production implementation; tests can use a deterministic typed fake.
type ScopeResolver interface {
	Resolve(ctx context.Context, resource scope.ResourceIdentity) (scope.Resolution, error)
}

type metadataScopeResolver interface {
	ResolveWithResourceLabels(ctx context.Context, resource scope.ResourceIdentity, resourceLabels map[string]string) (scope.Resolution, error)
}

// LocalResourceKind is an explicit, non-sensitive inventory allowlist entry.
// Secrets and ConfigMaps are excluded from the Gateway-aligned default; policy
// source ConfigMap reads use a separate explicit source.
type LocalResourceKind struct {
	Group      string
	Version    string
	Resource   string
	Kind       string
	Namespaced bool
}

// DefaultLocalResourceKinds is the bounded, Gateway-aligned hub inventory
// allowlist. Secrets and ConfigMaps are intentionally excluded.
var DefaultLocalResourceKinds = []LocalResourceKind{
	{Group: "", Version: "v1", Resource: "pods", Kind: "Pod", Namespaced: true},
	{Group: "", Version: "v1", Resource: "services", Kind: "Service", Namespaced: true},
	{Group: "", Version: "v1", Resource: "nodes", Kind: "Node"},
	{Group: "", Version: "v1", Resource: "persistentvolumes", Kind: "PersistentVolume"},
	{Group: "", Version: "v1", Resource: "namespaces", Kind: "Namespace"},
	{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true},
	{Group: "apps", Version: "v1", Resource: "statefulsets", Kind: "StatefulSet", Namespaced: true},
	{Group: "apps", Version: "v1", Resource: "daemonsets", Kind: "DaemonSet", Namespaced: true},
	{Group: "apps", Version: "v1", Resource: "replicasets", Kind: "ReplicaSet", Namespaced: true},
	{Group: "batch", Version: "v1", Resource: "jobs", Kind: "Job", Namespaced: true},
	{Group: "batch", Version: "v1", Resource: "cronjobs", Kind: "CronJob", Namespaced: true},
}

// LocalInventoryConfig constructs the local hub reader.
type LocalInventoryConfig struct {
	MetadataClient  metadata.Interface
	ScopeResolver   ScopeResolver
	Now             func() time.Time
	Kinds           []LocalResourceKind
	WorkingSetBytes int64
	ReadGuard       *ReadGuard
	// BackendPageLimit is an internal per-request Kubernetes API-server
	// item-count hint for metadata traversal. It is not the Console page limit
	// or an inventory total cap. Zero selects the centralized positive default.
	BackendPageLimit   int64
	OperationTimeout   time.Duration
	MaxConcurrentReads int
}

// LocalInventoryReader reads only allowlisted hub metadata and emits the
// canonical sanitized inventory contract.
type LocalInventoryReader struct {
	metadataClient   metadata.Interface
	scopeResolver    ScopeResolver
	now              func() time.Time
	kinds            []LocalResourceKind
	workingSetBytes  int64
	backendPageLimit int64
	operationTimeout time.Duration
	readGuard        *ReadGuard
}

// NewLocalInventoryReader validates and creates a local inventory reader.
func NewLocalInventoryReader(cfg LocalInventoryConfig) (*LocalInventoryReader, error) {
	if cfg.MetadataClient == nil {
		return nil, fmt.Errorf("local inventory metadata client is required")
	}
	if cfg.ScopeResolver == nil {
		return nil, fmt.Errorf("local inventory scope resolver is required")
	}
	if cfg.WorkingSetBytes < 0 {
		return nil, fmt.Errorf("local inventory working-set budget cannot be negative")
	}
	if cfg.BackendPageLimit < 0 {
		return nil, fmt.Errorf("local inventory backend page limit cannot be negative")
	}
	if cfg.OperationTimeout < 0 {
		return nil, fmt.Errorf("local inventory operation timeout cannot be negative")
	}
	if cfg.MaxConcurrentReads < 0 {
		return nil, fmt.Errorf("local inventory concurrency cannot be negative")
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if len(cfg.Kinds) == 0 {
		cfg.Kinds = append([]LocalResourceKind(nil), DefaultLocalResourceKinds...)
	}
	if cfg.WorkingSetBytes == 0 {
		cfg.WorkingSetBytes = DefaultInventoryWorkingSetBytes
	}
	if cfg.BackendPageLimit == 0 {
		cfg.BackendPageLimit = defaultInventoryBackendPageLimit
	}
	if cfg.OperationTimeout == 0 {
		cfg.OperationTimeout = DefaultInventoryOperationTimeout
	}
	if cfg.MaxConcurrentReads == 0 {
		cfg.MaxConcurrentReads = DefaultInventoryMaxConcurrentReads
	}
	if cfg.ReadGuard == nil {
		cfg.ReadGuard = NewReadGuard(cfg.MaxConcurrentReads)
	}
	return &LocalInventoryReader{
		metadataClient:   cfg.MetadataClient,
		scopeResolver:    cfg.ScopeResolver,
		now:              cfg.Now,
		kinds:            cfg.Kinds,
		workingSetBytes:  cfg.WorkingSetBytes,
		backendPageLimit: cfg.BackendPageLimit,
		operationTimeout: cfg.OperationTimeout,
		readGuard:        cfg.ReadGuard,
	}, nil
}

// List returns only included managed resources with deterministic bounded
// pagination. Any incomplete allowlisted read fails closed.
func (r *LocalInventoryReader) List(ctx context.Context, filters inventory.Filters) (inventory.Page, error) {
	normalized, err := filters.Normalize()
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	if err := validateLocalFilters(normalized); err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}

	operationCtx, cancel := context.WithTimeout(ctx, r.operationTimeout)
	defer cancel()
	guardCtx, release, err := r.readGuard.Acquire(operationCtx)
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	defer release()

	return r.list(guardCtx, normalized)
}

func (r *LocalInventoryReader) list(ctx context.Context, filters inventory.Filters) (inventory.Page, error) {
	resources := make([]inventory.Resource, 0)
	workingSet := &retainedWorkingSet{limit: r.workingSetBytes}
	if inherited, ok := ctx.Value(workingSetContextKey{}).(*retainedWorkingSet); ok && inherited != nil {
		workingSet = inherited
	}
	for _, kind := range r.kinds {
		if !matchesKindFilter(kind, filters) {
			continue
		}
		if readErr := r.readKind(ctx, kind, filters, &resources, workingSet); readErr != nil {
			return inventory.StateOnly(inventory.StateUnavailable), readErr
		}
	}

	inventory.SortResources(resources)
	start, err := cursorStart(resources, filters.Cursor)
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	resources = resources[start:]
	now := r.now()
	page := inventory.Page{
		State:        inventory.StateSuccess,
		ObservedAt:   now,
		Freshness:    inventory.Freshness{State: inventory.FreshnessFresh, AsOf: now},
		Completeness: inventory.CompletenessComplete,
	}
	if len(resources) == 0 {
		page.State = inventory.StateEmpty
		return page, nil
	}
	limit := filters.Limit
	if len(resources) > limit {
		page.Items = append(page.Items, resources[:limit]...)
		page.NextCursor = page.Items[len(page.Items)-1].ID
	} else {
		page.Items = resources
	}
	return page, nil
}

func (r *LocalInventoryReader) readKind(ctx context.Context, kind LocalResourceKind, filters inventory.Filters, resources *[]inventory.Resource, workingSet *retainedWorkingSet) error {
	resourceGetter := r.metadataClient.Resource(schema.GroupVersionResource{Group: kind.Group, Version: kind.Version, Resource: kind.Resource})
	var resourceReader metadata.ResourceInterface = resourceGetter
	if kind.Namespaced && filters.Namespace != "" {
		resourceReader = resourceGetter.Namespace(filters.Namespace)
	}

	continuation := ""
	seenContinuations := make(map[string]struct{})
	for {
		metadataList, nextContinuation, err := r.readMetadataPage(ctx, resourceReader, continuation, seenContinuations, kind.Resource)
		if err != nil {
			return err
		}
		if err := r.projectMetadataPage(ctx, kind, filters, metadataList, resources, workingSet); err != nil {
			return err
		}
		if nextContinuation == "" {
			return nil
		}
		continuation = nextContinuation
	}
}

func (r *LocalInventoryReader) readMetadataPage(ctx context.Context, resourceReader metadata.ResourceInterface, continuation string, seenContinuations map[string]struct{}, resourceName string) (*metav1.PartialObjectMetadataList, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", fmt.Errorf("list local inventory metadata: %w", err)
	}
	// This Limit is an internal traversal chunk. The caller's inventory page
	// limit is applied only after the complete canonical projection is built.
	metadataList, err := resourceReader.List(ctx, metav1.ListOptions{
		Limit:    r.backendPageLimit,
		Continue: continuation,
	})
	if err != nil {
		return nil, "", fmt.Errorf("list local inventory metadata for %s: %w", resourceName, err)
	}
	if metadataList == nil {
		return nil, "", fmt.Errorf("%w: metadata list for %s was nil", inventory.ErrUnavailable, resourceName)
	}

	nextContinuation := metadataList.Continue
	if err := validateMetadataContinuation(continuation, nextContinuation, seenContinuations, resourceName); err != nil {
		return nil, "", err
	}
	return metadataList, nextContinuation, nil
}

func validateMetadataContinuation(continuation, nextContinuation string, seenContinuations map[string]struct{}, resourceName string) error {
	if nextContinuation == "" {
		return nil
	}
	if nextContinuation == continuation {
		return fmt.Errorf("%w: metadata continuation did not progress for %s", inventory.ErrUnavailable, resourceName)
	}
	if _, seen := seenContinuations[nextContinuation]; seen {
		return fmt.Errorf("%w: metadata continuation repeated for %s", inventory.ErrUnavailable, resourceName)
	}
	seenContinuations[nextContinuation] = struct{}{}
	return nil
}

func (r *LocalInventoryReader) projectMetadataPage(ctx context.Context, kind LocalResourceKind, filters inventory.Filters, metadataList *metav1.PartialObjectMetadataList, resources *[]inventory.Resource, workingSet *retainedWorkingSet) error {
	for index := range metadataList.Items {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("project local inventory metadata: %w", err)
		}
		resource, included, err := r.projectMetadata(ctx, kind, &metadataList.Items[index], filters)
		if err != nil {
			return err
		}
		if !included {
			continue
		}
		if err := workingSet.retain(resource); err != nil {
			return err
		}
		*resources = append(*resources, resource)
	}

	// Do not retain backend page objects after their canonical projections
	// have been appended. This releases metadata labels and any other
	// server-provided fields before the next page is requested.
	metadataList.Items = nil
	return nil
}

func (r *LocalInventoryReader) projectMetadata(ctx context.Context, kind LocalResourceKind, object *metav1.PartialObjectMetadata, filters inventory.Filters) (inventory.Resource, bool, error) {
	// scope.Manager is the local Kubernetes resolver and rejects non-empty
	// cluster IDs. The canonical projection adds the explicit hub ID after
	// local resolution succeeds.
	scopeIdentity := scope.ResourceIdentity{
		ClusterID: "",
		Group:     kind.Group,
		Version:   kind.Version,
		Kind:      kind.Kind,
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}
	if !matchesIdentityFilters(scopeIdentity, filters) {
		return inventory.Resource{}, false, nil
	}
	var resolution scope.Resolution
	var err error
	if metadataResolver, ok := r.scopeResolver.(metadataScopeResolver); ok {
		resolution, err = metadataResolver.ResolveWithResourceLabels(ctx, scopeIdentity, object.GetLabels())
	} else {
		resolution, err = r.scopeResolver.Resolve(ctx, scopeIdentity)
	}
	if err != nil {
		return inventory.Resource{}, false, fmt.Errorf("resolve local resource scope: %w", err)
	}
	if !resolution.Included {
		return inventory.Resource{}, false, nil
	}
	canonicalIdentity := scopeIdentity
	canonicalIdentity.ClusterID = LocalClusterID
	resource := inventory.Resource{
		ID:         inventory.CanonicalResourceID(canonicalIdentity),
		ClusterID:  LocalClusterID,
		Group:      kind.Group,
		Version:    kind.Version,
		Kind:       kind.Kind,
		Namespace:  scopeIdentity.Namespace,
		Name:       scopeIdentity.Name,
		Scope:      toInventoryScope(resolution),
		ObservedAt: r.now(),
	}
	if filters.ResourceID != "" && resource.ID != filters.ResourceID {
		return inventory.Resource{}, false, nil
	}
	return resource, true, nil
}

// Get resolves an opaque ID by applying the same bounded list projection; no
// resource identity is encoded in or parsed from the opaque ID.
func (r *LocalInventoryReader) Get(ctx context.Context, clusterID, resourceID string) (inventory.Resource, error) {
	if err := inventory.ValidateResourceID(resourceID); err != nil {
		return inventory.Resource{}, err
	}
	normalized, err := (inventory.Filters{ClusterID: clusterID, ResourceID: resourceID, Limit: 1}).Normalize()
	if err != nil {
		return inventory.Resource{}, err
	}
	if err := validateLocalFilters(normalized); err != nil {
		return inventory.Resource{}, err
	}
	operationCtx, cancel := context.WithTimeout(ctx, r.operationTimeout)
	defer cancel()
	guardCtx, release, err := r.readGuard.Acquire(operationCtx)
	if err != nil {
		return inventory.Resource{}, err
	}
	defer release()

	page, err := r.list(guardCtx, normalized)
	if err != nil {
		return inventory.Resource{}, err
	}
	if len(page.Items) == 0 {
		return inventory.Resource{}, inventory.ErrNotFound
	}
	return page.Items[0], nil
}

func validateLocalFilters(filters inventory.Filters) error {
	if filters.ClusterID != LocalClusterID {
		return fmt.Errorf("%w: local reader cannot serve cluster %q", inventory.ErrUnavailable, filters.ClusterID)
	}
	if filters.Kind != "" && scope.IsClusterScopedKind(filters.Kind) && filters.Namespace != "" {
		return fmt.Errorf("%w: cluster-scoped kind cannot use namespace", inventory.ErrInvalidFilter)
	}
	return nil
}

func matchesKindFilter(kind LocalResourceKind, filters inventory.Filters) bool {
	return (filters.Group == "" || filters.Group == kind.Group) &&
		(filters.Version == "" || filters.Version == kind.Version) &&
		(filters.Kind == "" || filters.Kind == kind.Kind)
}

func matchesIdentityFilters(identity scope.ResourceIdentity, filters inventory.Filters) bool {
	return (filters.Namespace == "" || filters.Namespace == identity.Namespace) &&
		(filters.Name == "" || filters.Name == identity.Name)
}

func cursorStart(resources []inventory.Resource, cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	if err := inventory.ValidateResourceID(cursor); err != nil {
		return 0, err
	}
	for index, resource := range resources {
		if resource.ID == cursor {
			return index + 1, nil
		}
	}
	// A resource may be deleted between page requests. Re-run the current
	// sorted query rather than treating that backend drift as caller input.
	return 0, nil
}

type retainedWorkingSet struct {
	limit    int64
	retained int64
}

func (s *retainedWorkingSet) retain(resource inventory.Resource) error {
	_, err := s.retainValue(resource)
	return err
}

func (s *retainedWorkingSet) retainValue(value any) (int64, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0, fmt.Errorf("%w: calculate retained projection size: %w", inventory.ErrUnavailable, err)
	}
	size := int64(len(encoded))
	if size > s.limit || s.retained > s.limit-size {
		return 0, fmt.Errorf("%w: local inventory working-set budget exhausted", inventory.ErrUnavailable)
	}
	s.retained += size
	return size, nil
}

func (s *retainedWorkingSet) release(size int64) {
	if size >= s.retained {
		s.retained = 0
		return
	}
	s.retained -= size
}

type inventorySemaphore struct {
	slots chan struct{}
}

// ReadGuard bounds concurrent Console projection reads within one AF replica.
// A guard already held in the request context is reused by nested local list
// and policy-history reads, preventing deadlocks while preserving one budget.
type ReadGuard struct {
	semaphore *inventorySemaphore
}

type readGuardContextKey struct{}
type workingSetContextKey struct{}

// NewReadGuard creates a process-local projection read guard.
func NewReadGuard(limit int) *ReadGuard {
	if limit <= 0 {
		limit = DefaultInventoryMaxConcurrentReads
	}
	return &ReadGuard{semaphore: newInventorySemaphore(limit)}
}

// Acquire holds one projection-read slot unless the request already owns this
// guard. The returned context carries ownership to nested readers.
func (g *ReadGuard) Acquire(ctx context.Context) (context.Context, func(), error) {
	if held, ok := ctx.Value(readGuardContextKey{}).(*ReadGuard); ok && held == g {
		return ctx, func() {}, nil
	}
	if err := g.semaphore.acquire(ctx); err != nil {
		return ctx, nil, err
	}
	return context.WithValue(ctx, readGuardContextKey{}, g), g.semaphore.release, nil
}

func newInventorySemaphore(limit int) *inventorySemaphore {
	return &inventorySemaphore{slots: make(chan struct{}, limit)}
}

func (s *inventorySemaphore) acquire(ctx context.Context) error {
	select {
	case s.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *inventorySemaphore) release() {
	<-s.slots
}

func toInventoryScope(resolution scope.Resolution) inventory.Scope {
	return inventory.Scope{
		Included:       resolution.Included,
		ResourceLabel:  inventory.LabelState(resolution.ResourceLabel),
		NamespaceLabel: inventory.LabelState(resolution.NamespaceLabel),
		Source:         inventory.ScopeSource(resolution.Source),
		ClusterScoped:  resolution.ClusterScoped,
	}
}
