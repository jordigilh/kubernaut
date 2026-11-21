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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const (
	// acmSearchChunk is the traversal chunk. It is a correctness parameter, not
	// policy, so it is deliberately not operator-configurable (DD-FLEET-017
	// D4.1a rule 7).
	acmSearchChunk = 50

	// acmSearchOrder is requested explicitly: the spike showed an invalid or
	// absent order is accepted silently, so the request cannot imply one.
	acmSearchOrder = "uid asc"

	// defaultACMRequestCap is the per-request ceiling. It matches the value the
	// shared ACM client already uses.
	defaultACMRequestCap = 10 * time.Second

	// defaultACMCommitReserve is held back from the staging budget so a
	// collected generation still has time to commit (D4.1a rule 7a).
	defaultACMCommitReserve = 5 * time.Second

	// defaultACMStartFloor is the remaining budget below which no request is
	// started at all: one that cannot finish would only burn the staging
	// window, so the collector abandons staging and lets the previous proof age
	// out instead.
	defaultACMStartFloor = 2 * time.Second

	// defaultACMCollectionBudget is the fallback wall clock for one kind when
	// the caller supplies no deadline. It stays under the 60 s staged-lifetime
	// cap so a single kind can never outlive its staging generation.
	defaultACMCollectionBudget = 55 * time.Second
)

// RepairCollectionBudget bounds the collection phase of one repair. It sits
// under the 60 s staged-generation lifetime so the commit still fits, and it is
// the value the repair path sets once per repair (D4.1a rule 7b). The collector
// never derives a budget of its own in production.
const RepairCollectionBudget = 55 * time.Second

// deadlineKey carries the staging deadline a sync cycle computed from its
// repair start. The collector never reads a clock it cannot bound.
type deadlineKey struct{}

// WithCollectionDeadline bounds one collection to the staging generation's
// remaining life.
func WithCollectionDeadline(ctx context.Context, deadline time.Time) context.Context {
	return context.WithValue(ctx, deadlineKey{}, deadline)
}

// isACMCollectionDeadlineSet reports whether the repair path already bounded
// this collection, so nobody wraps it twice.
func isACMCollectionDeadlineSet(ctx context.Context) bool {
	deadline, ok := ctx.Value(deadlineKey{}).(time.Time)
	return ok && !deadline.IsZero()
}

func collectionDeadline(ctx context.Context, now time.Time, fallback time.Duration) time.Time {
	if deadline, ok := ctx.Value(deadlineKey{}).(time.Time); ok && !deadline.IsZero() {
		return deadline
	}
	return now.Add(fallback)
}

// ACMSourceConfig configures the ACM Search collector (DD-FLEET-017 D4).
type ACMSourceConfig struct {
	// Search performs one chunk request.
	Search ACMSearch
	// Kinds maps a configured resource kind to its exact GVK. Kind is never
	// used to infer group or version (D4.1).
	Kinds map[string]schema.GroupVersionKind
	// Now is injectable for budget tests.
	Now func() time.Time
	// CollectionBudget bounds one kind when no deadline is supplied.
	CollectionBudget time.Duration
	// RequestCap, CommitReserve and StartFloor are the derived-timeout
	// parameters of D4.1a rule 7a.
	RequestCap    time.Duration
	CommitReserve time.Duration
	StartFloor    time.Duration
}

// ACMSource is the deployment-level ACM collector. It is an
// observation.ObservedReaderFactory: the syncer asks it for a reader per
// registered cluster, and every reader returns whole-kind observations.
type ACMSource struct {
	config ACMSourceConfig
}

// NewACMSource validates and creates the ACM collector.
func NewACMSource(config ACMSourceConfig) (*ACMSource, error) {
	if config.Search == nil {
		return nil, fmt.Errorf("acm source requires a search client")
	}
	if len(config.Kinds) == 0 {
		return nil, fmt.Errorf("acm source requires at least one resource kind")
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.CollectionBudget <= 0 {
		config.CollectionBudget = defaultACMCollectionBudget
	}
	if config.RequestCap <= 0 {
		config.RequestCap = defaultACMRequestCap
	}
	if config.CommitReserve <= 0 {
		config.CommitReserve = defaultACMCommitReserve
	}
	if config.StartFloor <= 0 {
		config.StartFloor = defaultACMStartFloor
	}
	return &ACMSource{config: config}, nil
}

// ReaderFor returns the collector's reader for one registered cluster.
// Membership stays the registration CR's decision (DD-FLEET-017 D1.3a): this
// only refuses to invent a cluster identity the registry did not provide.
func (s *ACMSource) ReaderFor(_ context.Context, cluster registry.ClusterInfo) (observation.ObservedReader, error) {
	if cluster.ID == "" {
		return nil, fmt.Errorf("acm source requires an explicit cluster identity")
	}
	return &acmClusterReader{source: s, clusterID: cluster.ID}, nil
}

// acmClusterReader collects one cluster's kinds from ACM Search.
type acmClusterReader struct {
	source    *ACMSource
	clusterID string
}

// ListObserved walks every ACM Search page of one kind under the traversal
// guards of DD-FLEET-017 D4.3 and returns a complete observation, or an
// incomplete one that names why it cannot be trusted.
func (r *acmClusterReader) ListObserved(ctx context.Context, request observation.ListRequest) (observation.ListObservation, error) {
	gvk, known := r.source.config.Kinds[request.Kind]
	if !known {
		return incompleteObservation(observation.FailureTool,
			fmt.Errorf("acm source has no GVK for kind %q", request.Kind))
	}
	deadline := collectionDeadline(ctx, r.source.config.Now(), r.source.config.CollectionBudget)
	return r.traverse(ctx, deadline, gvk, request.Kind)
}

// searchFailureClass maps a chunk failure onto the observation failure the
// syncer reports, so an auth or GraphQL answer is not logged as a network
// outage and a corrupt body is not logged as a source problem.
func searchFailureClass(err error) observation.ObservationFailure {
	var searchErr *ACMSearchError
	if errors.As(err, &searchErr) {
		switch searchErr.Failure {
		case ACMSearchFailureTransport:
			return observation.FailureTransport
		case ACMSearchFailureMalformed:
			return observation.FailureMalformedContent
		case ACMSearchFailureTool:
			return observation.FailureTool
		}
	}
	return observation.FailureTransport
}

// acmWalk is the mutable state of one guarded traversal. Keeping it in a
// value lets the guards be tested and reasoned about one at a time instead of
// as one long function.
type acmWalk struct {
	items    []observation.ObservedItem
	offset   int
	pages    int
	restarts int
	total    int
	lastUID  string
}

func newACMWalk() *acmWalk {
	return &acmWalk{total: -1}
}

// traverse is the guarded offset walk: explicit ascending uid order, strict
// per-page uid increase, a stable total count, one bounded restart when the
// count moves, and an incomplete result for every other anomaly.
//
// Known residual (spike 2026-11-18, mutation probe verdict 4): balanced
// delete+create churn can keep the count stable while shifting offsets, so a
// skipped identity is not detectable by these guards. Offset continuation is not
// a snapshot; keyset continuation is unavailable on this Search build. This is a
// documented limitation, never a completeness guarantee.
func (r *acmClusterReader) traverse(ctx context.Context, deadline time.Time, gvk schema.GroupVersionKind, kind string) (observation.ListObservation, error) {
	walk := newACMWalk()
	for {
		// The page budget is enforced before the next request, so an
		// exhausted budget is reported without sending a request the guard
		// would reject. Reaching the top of the loop with a full budget means
		// the previous page was not final; a page that completes the walk
		// breaks out below and never returns here. The item ceiling in
		// addPage stays as defense in depth: with 50 items per page and 100
		// pages the walk can never legitimately reach it.
		if walk.pages >= observation.MaxObservationPages {
			return incompleteObservation(observation.FailureBounds,
				fmt.Errorf("acm collection exceeded %d pages", observation.MaxObservationPages))
		}
		timeout, err := r.chunkTimeout(ctx, r.source.config.Now(), deadline)
		if err != nil {
			// D4.1a rule 7a: a request that cannot finish must not start, so
			// the collection is abandoned and the previous proof ages out.
			return incompleteObservation(observation.FailureBounds, err)
		}
		page, err := r.searchChunk(ctx, timeout, gvk, kind, walk.offset)
		if err != nil {
			return incompleteObservation(searchFailureClass(err), err)
		}
		walk.pages++
		restarted, err := walk.applyCount(page.Count)
		if err != nil {
			return incompleteObservation(observation.FailureBounds, err)
		}
		if restarted {
			continue
		}
		if err := walk.addPage(page.Items, r.clusterID, gvk, kind); err != nil {
			return incompleteObservation(observation.FailureOf(err), err)
		}
		if walk.chunkEndsWalk(len(page.Items)) {
			break
		}
	}
	return walk.observation()
}

// chunkTimeout derives this request's ceiling from the staging budget rather
// than a configured value (D4.1a rule 7a). The caller's context deadline, when
// earlier, tightens the budget: a request the caller has already bounded
// shorter must not outlive that bound. A context that is already done refuses
// the request outright.
func (r *acmClusterReader) chunkTimeout(ctx context.Context, now time.Time, deadline time.Time) (time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("acm collection context is done: %w", err)
	}
	if ctxDeadline, hasDeadline := ctx.Deadline(); hasDeadline && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	remaining := deadline.Sub(now) - r.source.config.CommitReserve
	if remaining < r.source.config.StartFloor {
		return 0, fmt.Errorf("acm collection has %s of budget left, below the %s start floor",
			remaining.Round(time.Millisecond), r.source.config.StartFloor)
	}
	if remaining < r.source.config.RequestCap {
		return remaining, nil
	}
	return r.source.config.RequestCap, nil
}

// applyCount enforces a stable total. A transition is the only reliable signal
// that the index moved mid-walk, so it earns exactly one clean restart.
func (w *acmWalk) applyCount(count int) (bool, error) {
	switch {
	case w.total < 0:
		w.total = count
		return false, nil
	case count == w.total:
		return false, nil
	case w.restarts >= 1:
		return false, fmt.Errorf("acm collection count changed twice (%d then %d)", w.total, count)
	default:
		w.restarts++
		w.offset = 0
		w.items = nil
		w.lastUID = ""
		w.total = count
		return true, nil
	}
}

// addPage guards one page and projects it. Every failure names the invariant,
// because a completed-looking walk that lost an item is worse than a failure.
func (w *acmWalk) addPage(rawItems []map[string]json.RawMessage, clusterID string, gvk schema.GroupVersionKind, kind string) error {
	if len(rawItems) > acmSearchChunk {
		return observationError(observation.FailureMalformedContent,
			fmt.Errorf("acm page returned %d items for a %d chunk", len(rawItems), acmSearchChunk))
	}
	if len(w.items)+len(rawItems) > observation.MaxObservationItems {
		return observationError(observation.FailureBounds,
			fmt.Errorf("acm collection exceeded %d items", observation.MaxObservationItems))
	}
	for _, raw := range rawItems {
		uid, err := searchString(raw, "_uid")
		if err != nil {
			return observationError(observation.FailureMalformedContent, err)
		}
		if uid == "" {
			return observationError(observation.FailureMalformedContent, fmt.Errorf("acm page item has no _uid"))
		}
		// Bytewise comparison, matching `orderBy: "uid asc"` for the
		// lowercase-hex uids Search returns; a collation divergence would
		// fail the kind closed here, never publish a reordered walk. This
		// strict-increase check also rejects any later occurrence of an
		// already-accepted uid, so no separate duplicate set is needed.
		if w.lastUID != "" && uid <= w.lastUID {
			return observationError(observation.FailureMalformedContent,
				fmt.Errorf("acm traversal uid %q does not follow %q", uid, w.lastUID))
		}
		w.lastUID = uid
		item, failure, err := observedItemFromSearch(raw, clusterID, gvk, kind)
		if err != nil {
			return observationError(failure, err)
		}
		w.items = append(w.items, item)
	}
	return nil
}

// chunkEndsWalk reports whether the walk is finished: a short page is the final
// one, and a full page whose items reach the reported total is the last needed.
func (w *acmWalk) chunkEndsWalk(received int) bool {
	w.offset += received
	return received < acmSearchChunk || w.offset >= w.total
}

// observation turns the walk into the collector's result, rejecting a
// completed-looking walk whose total does not match its items.
func (w *acmWalk) observation() (observation.ListObservation, error) {
	if w.total < 0 {
		return incompleteObservation(observation.FailureMalformedContent, fmt.Errorf("acm collection returned no page"))
	}
	if len(w.items) != w.total {
		return incompleteObservation(observation.FailureMalformedContent,
			fmt.Errorf("acm collection returned %d items for a reported count of %d", len(w.items), w.total))
	}
	return observation.ListObservation{
		Items:     w.items,
		Complete:  true,
		PageCount: w.pages,
		ItemCount: len(w.items),
	}, nil
}

// observationError keeps the typed failure attached to the error so callers can
// classify without parsing text.
func observationError(failure observation.ObservationFailure, err error) error {
	return &observation.ObservationError{Failure: failure, Err: err}
}

// searchChunk performs one request bounded by the remaining budget.
func (r *acmClusterReader) searchChunk(ctx context.Context, timeout time.Duration, gvk schema.GroupVersionKind, kind string, offset int) (ACMSearchPage, error) {
	chunkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	page, err := r.source.config.Search.Search(chunkCtx, ACMSearchOptions{
		ClusterID: r.clusterID,
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      kind,
		OrderBy:   acmSearchOrder,
		Offset:    offset,
		Limit:     acmSearchChunk,
	})
	if err != nil {
		return ACMSearchPage{}, fmt.Errorf("acm search offset %d: %w", offset, err)
	}
	return page, nil
}

// observedItemFromSearch validates one item's identity and label state. A
// cross-cluster item, a missing or mismatched apiVersion, or an invalid managed
// label fails the kind, not just the item (DD-FLEET-017 D4.5).
func observedItemFromSearch(raw map[string]json.RawMessage, clusterID string, gvk schema.GroupVersionKind, kind string) (observation.ObservedItem, observation.ObservationFailure, error) {
	if err := requireClusterOwnership(raw, clusterID); err != nil {
		return observation.ObservedItem{}, observationFailureOf(err), err
	}
	apiVersion, err := requireExactAPIVersion(raw, gvk, kind)
	if err != nil {
		return observation.ObservedItem{}, observationFailureOf(err), err
	}
	name, err := searchString(raw, "name")
	if err != nil {
		return observation.ObservedItem{}, observation.FailureItem, err
	}
	if name == "" {
		return observation.ObservedItem{}, observation.FailureItem, fmt.Errorf("acm item has no name")
	}
	namespace, err := requireNamespaceFor(raw, kind, name)
	if err != nil {
		return observation.ObservedItem{}, observationFailureOf(err), err
	}
	labelState, labelsPresent := acmManagedLabelState(raw)
	return buildObservedItem(apiVersion, kind, name, namespace, labelState, labelsPresent), observation.FailureNone, nil
}

// identityError keeps the typed failure with the reason without threading a
// third return value through every check.
type identityError struct {
	failure observation.ObservationFailure
	err     error
}

func (e *identityError) Error() string { return e.err.Error() }
func (e *identityError) Unwrap() error { return e.err }

func identityFailure(failure observation.ObservationFailure, err error) error {
	return &identityError{failure: failure, err: err}
}

func observationFailureOf(err error) observation.ObservationFailure {
	var typed *identityError
	if errors.As(err, &typed) {
		return typed.failure
	}
	return observation.FailureItem
}

// requireClusterOwnership refuses an item whose cluster is missing or different:
// Search visibility is not membership, so ownership is never assumed.
func requireClusterOwnership(raw map[string]json.RawMessage, clusterID string) error {
	itemCluster, present := searchStringValue(raw, "cluster")
	if !present || itemCluster == "" {
		return identityFailure(observation.FailureItem,
			fmt.Errorf("acm item has no cluster attribution; ownership is never assumed"))
	}
	if itemCluster != clusterID {
		return identityFailure(observation.FailureConflict,
			fmt.Errorf("acm item belongs to cluster %q while collecting %q", itemCluster, clusterID))
	}
	return nil
}

// requireExactAPIVersion enforces exact GVK: a missing or mismatched
// group/version fails the kind because group/version are never inferred
// (D4.1). The wire shape is the one the live Check lane recorded (#2592) and
// our own request builder mirrors: `apigroup` and `apiversion` are separate
// item properties, and an absent apigroup means the core group.
func requireExactAPIVersion(raw map[string]json.RawMessage, gvk schema.GroupVersionKind, kind string) (string, error) {
	apiGroup, err := searchString(raw, "apigroup")
	if err != nil {
		return "", identityFailure(observation.FailureItem, err)
	}
	apiVersion, err := searchString(raw, "apiversion")
	if err != nil {
		return "", identityFailure(observation.FailureItem, err)
	}
	if apiVersion == "" {
		return "", identityFailure(observation.FailureItem,
			fmt.Errorf("acm item has no apiversion; GVK is never inferred"))
	}
	itemGV := schema.GroupVersion{Group: apiGroup, Version: apiVersion}
	itemKind, err := searchString(raw, "kind")
	if err != nil {
		return "", identityFailure(observation.FailureItem, err)
	}
	if itemKind != kind || itemGV != gvk.GroupVersion() {
		return "", identityFailure(observation.FailureConflict,
			fmt.Errorf("acm item %s/%s does not match the requested %s", itemGV.String(), itemKind, gvk.String()))
	}
	return itemGV.String(), nil
}

// requireNamespaceFor enforces the namespace contract: a cluster-scoped item
// must not carry one, a namespaced item must, and a malformed field is an item
// failure rather than a silent reclassification.
func requireNamespaceFor(raw map[string]json.RawMessage, kind, name string) (string, error) {
	namespace, present := searchStringValue(raw, "namespace")
	if _, found := raw["namespace"]; found && !present {
		return "", identityFailure(observation.FailureItem,
			fmt.Errorf("acm item %s/%s has a namespace field that is not a string", kind, name))
	}
	clusterScoped := scope.IsClusterScopedKind(kind)
	if clusterScoped && namespace != "" {
		return "", identityFailure(observation.FailureScope,
			fmt.Errorf("cluster-scoped ACM item %s/%s carries namespace %q", kind, name, namespace))
	}
	if !clusterScoped && namespace == "" {
		return "", identityFailure(observation.FailureScope,
			fmt.Errorf("namespaced ACM item %s/%s has no namespace", kind, name))
	}
	return namespace, nil
}

func buildObservedItem(apiVersion, kind, name, namespace string, labelState scope.LabelState, labelsPresent bool) observation.ObservedItem {
	object := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name},
	}}
	if namespace != "" {
		object.SetNamespace(namespace)
	}
	return observation.ObservedItem{Object: object, LabelsPresent: labelsPresent, LabelState: labelState}
}

// acmManagedLabelState reads the projected label field. Search projects labels
// as a semicolon-separated `label` string and omits the field entirely when the
// object has none (spike 2026-11-18). Malformed projected data must never become
// "absent" or a silently-won last value: an invalid label stays invalid so the
// syncer fails the kind (ADR-053).
func acmManagedLabelState(raw map[string]json.RawMessage) (scope.LabelState, bool) {
	// Search answers with either the semicolon `label` field (present and
	// complete, or absent entirely) or the legacy per-key form. More than one
	// managed source at once is corruption: resolving it would silently invent a
	// precedence the source never promised (DD-FLEET-017 D4.2).
	labelValue, hasProjection := raw["label"]
	presentKeys := presentFallbackKeys(raw)
	sources := 0
	if hasProjection {
		sources++
	}
	sources += len(presentKeys)
	switch {
	case sources == 0:
		return scope.LabelStateAbsent, false
	case sources > 1:
		return scope.LabelStateInvalid, true
	case hasProjection:
		return projectedLabelState(labelValue)
	default:
		return fallbackKeyState(raw, presentKeys[0])
	}
}

// presentFallbackKeys lists which legacy per-key managed labels the answer
// carries, in a stable order.
func presentFallbackKeys(raw map[string]json.RawMessage) []string {
	keys := make([]string, 0, 3)
	for _, key := range fallbackManagedLabelKeys() {
		if _, present := raw[key]; present {
			keys = append(keys, key)
		}
	}
	return keys
}

// projectedLabelState reads the semicolon `label` field Search projects. A
// missing field means the object has no labels; a present-but-unusable field is
// corruption (ADR-053: invalid is never absent).
func projectedLabelState(labelValue json.RawMessage) (scope.LabelState, bool) {
	if isJSONNull(labelValue) {
		return scope.LabelStateInvalid, true
	}
	var projected string
	if err := json.Unmarshal(labelValue, &projected); err != nil {
		return scope.LabelStateInvalid, true
	}
	states, malformed := parseProjectedLabels(projected)
	if malformed {
		return scope.LabelStateInvalid, true
	}
	var found scope.LabelState
	occurrences := 0
	for _, key := range managedLabelKeys() {
		state, ok := states[key]
		if !ok {
			continue
		}
		occurrences++
		found = state
	}
	switch {
	case occurrences > 1 || found == scope.LabelStateInvalid:
		return scope.LabelStateInvalid, true
	case occurrences == 1:
		return found, true
	default:
		// Deliberate asymmetry with fallbackKeyState: a present-but-empty
		// semicolon projection is a well-formed "object has no labels"
		// answer, so it is Absent — while a present-but-empty legacy key is
		// unusable data and Invalid. Neither can become true or false.
		return scope.LabelStateAbsent, true
	}
}

// fallbackKeyState reads one present per-key projection, held to the same rule
// as the projected field: a present-but-unusable value is corruption, and
// reporting LabelsPresent=false for it would claim the field was never there.
func fallbackKeyState(raw map[string]json.RawMessage, key string) (scope.LabelState, bool) {
	labelValue := raw[key]
	if isJSONNull(labelValue) {
		return scope.LabelStateInvalid, true
	}
	value, isString := searchStringValue(raw, key)
	if !isString || value == "" {
		return scope.LabelStateInvalid, true
	}
	return labelStateOf(value), true
}

func isJSONNull(value json.RawMessage) bool {
	return strings.TrimSpace(string(value)) == "null"
}

// fallbackManagedLabelKeys are the per-key projections older Search answers use
// instead of the semicolon `label` field.
func fallbackManagedLabelKeys() []string {
	return []string{"label.kubernaut.ai/managed", "labels.kubernaut.ai/managed", "label.kubernaut.ai~1managed"}
}

func managedLabelKeys() []string {
	return []string{scope.ManagedLabelKey, "kubernaut.ai~1managed"}
}

// parseProjectedLabels turns Search's `k=v;k=v` projection into label states.
// An entry without `=` or with an empty managed value is malformed: reporting it
// keeps the caller from treating corruption as "no label".
func parseProjectedLabels(projected string) (map[string]scope.LabelState, bool) {
	states := make(map[string]scope.LabelState)
	if strings.TrimSpace(projected) == "" {
		return states, false
	}
	seen := make(map[string]struct{}, 8)
	for _, entry := range strings.Split(projected, ";") {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		key, value, found := strings.Cut(trimmed, "=")
		if !found {
			return states, true
		}
		// A space around the separator cannot come from a valid Kubernetes
		// label, so it is corruption. The Check lane classifies the same input
		// as malformed (pkg/fleet/acm/client_test.go, "malformed managed key
		// cannot inherit"); both lanes must agree instead of one lane reading it
		// as managed.
		if key != strings.TrimSpace(key) || value != strings.TrimSpace(value) {
			return states, true
		}
		key = strings.TrimSpace(key)
		if _, duplicate := seen[key]; duplicate {
			// Two values for one key cannot be resolved into a single label
			// state, and quietly winning the last one would invent an answer.
			return states, true
		}
		seen[key] = struct{}{}
		trimmedValue := strings.TrimSpace(value)
		if trimmedValue == "" {
			// `kubernaut.ai/managed=` is corruption, not an absent label.
			states[key] = scope.LabelStateInvalid
			continue
		}
		states[key] = labelStateOf(trimmedValue)
	}
	return states, false
}

func labelStateOf(value string) scope.LabelState {
	switch value {
	case "true":
		return scope.LabelStateTrue
	case "false":
		return scope.LabelStateFalse
	default:
		// Callers pre-check the empty value (an empty managed value is
		// corruption, never absence), so it lands here as invalid too.
		return scope.LabelStateInvalid
	}
}

// searchStringValue distinguishes "field absent or not a JSON string" from
// "field present with an empty value": ownership and namespace checks need the
// difference.
func searchStringValue(raw map[string]json.RawMessage, key string) (string, bool) {
	value, present := raw[key]
	if !present {
		return "", false
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return "", false
	}
	return decoded, true
}

// searchString reads one string-valued identity field. A JSON null is
// corruption, not a missing field: it is rejected rather than read as an
// empty string, while an omitted field or an explicit empty string keeps its
// existing meaning so cluster-scoped items stay valid.
func searchString(raw map[string]json.RawMessage, key string) (string, error) {
	value, present := raw[key]
	if !present {
		return "", nil
	}
	if isJSONNull(value) {
		return "", fmt.Errorf("acm item field %q is JSON null, not a string", key)
	}
	var decoded string
	// A present-but-unstringable value keeps its historical meaning of
	// "unusable": the caller's empty-string check fails the item, and the
	// JSON null case above carries the explicit corruption verdict.
	_ = json.Unmarshal(value, &decoded) //nolint:errcheck // an unstringable value reads as empty and fails the caller's check
	return decoded, nil
}

// incompleteObservation wraps a failure so the syncer aborts before any write
// and the previous proven generation stays readable (DD-FLEET-017 D4.4).
func incompleteObservation(failure observation.ObservationFailure, err error) (observation.ListObservation, error) {
	if err == nil {
		err = errors.New(string(failure))
	}
	return observation.ListObservation{Complete: false, Failure: failure}, observationError(failure, err)
}
