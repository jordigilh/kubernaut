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

// deadlineKey carries the staging deadline a sync cycle computed from its
// repair start. The collector never reads a clock it cannot bound.
type deadlineKey struct{}

// WithCollectionDeadline bounds one collection to the staging generation's
// remaining life.
func WithCollectionDeadline(ctx context.Context, deadline time.Time) context.Context {
	return context.WithValue(ctx, deadlineKey{}, deadline)
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
		return nil, fmt.Errorf("ACM source requires a search client")
	}
	if len(config.Kinds) == 0 {
		return nil, fmt.Errorf("ACM source requires at least one resource kind")
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
		return nil, fmt.Errorf("ACM source requires an explicit cluster identity")
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
			fmt.Errorf("ACM source has no GVK for kind %q", request.Kind))
	}
	deadline := collectionDeadline(ctx, r.source.config.Now(), r.source.config.CollectionBudget)
	return r.traverse(ctx, deadline, gvk, request.Kind)
}

// acmWalk is the mutable state of one guarded traversal. Keeping it in a
// value lets the guards be tested and reasoned about one at a time instead of
// as one long function.
type acmWalk struct {
	items    []observation.ObservedItem
	seenUIDs map[string]struct{}
	offset   int
	pages    int
	restarts int
	total    int
	lastUID  string
}

func newACMWalk() *acmWalk {
	return &acmWalk{seenUIDs: make(map[string]struct{}, observation.MaxObservationItems), total: -1}
}

// traverse is the guarded offset walk: explicit ascending uid order, strict
// per-page uid increase, a stable total count, one bounded restart when the
// count moves, and an incomplete result for every other anomaly.
func (r *acmClusterReader) traverse(ctx context.Context, deadline time.Time, gvk schema.GroupVersionKind, kind string) (observation.ListObservation, error) {
	walk := newACMWalk()
	for {
		timeout, err := r.chunkTimeout(deadline)
		if err != nil {
			// D4.1a rule 7a: a request that cannot finish must not start, so
			// the collection is abandoned and the previous proof ages out.
			return incompleteObservation(observation.FailureBounds, err)
		}
		page, err := r.searchChunk(ctx, timeout, gvk, kind, walk.offset)
		if err != nil {
			return incompleteObservation(observation.FailureTransport, err)
		}
		walk.pages++
		if walk.pages > observation.MaxObservationPages {
			return incompleteObservation(observation.FailureBounds,
				fmt.Errorf("ACM collection exceeded %d pages", observation.MaxObservationPages))
		}
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
// than a configured value (D4.1a rule 7a).
func (r *acmClusterReader) chunkTimeout(deadline time.Time) (time.Duration, error) {
	remaining := time.Until(deadline) - r.source.config.CommitReserve
	if remaining < r.source.config.StartFloor {
		return 0, fmt.Errorf("ACM collection has %s of budget left, below the %s start floor",
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
		return false, fmt.Errorf("ACM collection count changed twice (%d then %d)", w.total, count)
	default:
		w.restarts++
		w.offset = 0
		w.items = nil
		w.lastUID = ""
		w.total = count
		w.seenUIDs = make(map[string]struct{}, observation.MaxObservationItems)
		return true, nil
	}
}

// addPage guards one page and projects it. Every failure names the invariant,
// because a completed-looking walk that lost an item is worse than a failure.
func (w *acmWalk) addPage(rawItems []map[string]json.RawMessage, clusterID string, gvk schema.GroupVersionKind, kind string) error {
	if len(rawItems) > acmSearchChunk {
		return observationError(observation.FailureMalformedContent,
			fmt.Errorf("ACM page returned %d items for a %d chunk", len(rawItems), acmSearchChunk))
	}
	if len(w.items)+len(rawItems) > observation.MaxObservationItems {
		return observationError(observation.FailureBounds,
			fmt.Errorf("ACM collection exceeded %d items", observation.MaxObservationItems))
	}
	for _, raw := range rawItems {
		uid := searchString(raw, "_uid")
		if uid == "" {
			return observationError(observation.FailureMalformedContent, fmt.Errorf("ACM page item has no _uid"))
		}
		if w.lastUID != "" && uid <= w.lastUID {
			return observationError(observation.FailureMalformedContent,
				fmt.Errorf("ACM traversal uid %q does not follow %q", uid, w.lastUID))
		}
		if _, duplicate := w.seenUIDs[uid]; duplicate {
			return observationError(observation.FailureDuplicate,
				fmt.Errorf("ACM traversal returned uid %q twice", uid))
		}
		w.seenUIDs[uid] = struct{}{}
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
		return incompleteObservation(observation.FailureMalformedContent, fmt.Errorf("ACM collection returned no page"))
	}
	if len(w.items) != w.total {
		return incompleteObservation(observation.FailureMalformedContent,
			fmt.Errorf("ACM collection returned %d items for a reported count of %d", len(w.items), w.total))
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
		return ACMSearchPage{}, fmt.Errorf("ACM search offset %d: %w", offset, err)
	}
	return page, nil
}

// observedItemFromSearch validates one item's identity and label state. A
// cross-cluster item, a missing or mismatched apiVersion, or an invalid managed
// label fails the kind, not just the item (DD-FLEET-017 D4.5).
func observedItemFromSearch(raw map[string]json.RawMessage, clusterID string, gvk schema.GroupVersionKind, kind string) (observation.ObservedItem, observation.ObservationFailure, error) {
	itemCluster := searchString(raw, "cluster")
	if itemCluster != "" && itemCluster != clusterID {
		return observation.ObservedItem{}, observation.FailureConflict,
			fmt.Errorf("ACM item belongs to cluster %q while collecting %q", itemCluster, clusterID)
	}
	apiVersion := searchString(raw, "apiversion")
	if apiVersion == "" {
		return observation.ObservedItem{}, observation.FailureItem,
			fmt.Errorf("ACM item has no apiversion; GVK is never inferred")
	}
	itemGV, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return observation.ObservedItem{}, observation.FailureItem,
			fmt.Errorf("ACM item has an unparseable apiversion %q: %w", apiVersion, err)
	}
	itemKind := searchString(raw, "kind")
	if itemKind != kind || itemGV.Group != gvk.Group || itemGV.Version != gvk.Version {
		return observation.ObservedItem{}, observation.FailureConflict,
			fmt.Errorf("ACM item %s/%s does not match the requested %s", apiVersion, itemKind, gvk.String())
	}
	name := searchString(raw, "name")
	if name == "" {
		return observation.ObservedItem{}, observation.FailureItem, fmt.Errorf("ACM item has no name")
	}
	namespace := searchString(raw, "namespace")
	clusterScoped := scope.IsClusterScopedKind(kind)
	if clusterScoped && namespace != "" {
		return observation.ObservedItem{}, observation.FailureScope,
			fmt.Errorf("cluster-scoped ACM item %s/%s carries namespace %q", kind, name, namespace)
	}
	if !clusterScoped && namespace == "" {
		return observation.ObservedItem{}, observation.FailureScope,
			fmt.Errorf("namespaced ACM item %s/%s has no namespace", kind, name)
	}
	labelState, labelsPresent := acmManagedLabelState(raw)
	object := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name},
	}}
	if namespace != "" {
		object.SetNamespace(namespace)
	}
	return observation.ObservedItem{Object: object, LabelsPresent: labelsPresent, LabelState: labelState}, observation.FailureNone, nil
}

// acmManagedLabelState reads the projected label field. Search projects labels
// as a semicolon-separated `label` string and omits the field entirely when the
// object has none (spike 2026-11-18), which is why absence and an unparseable
// value are distinguished.
func acmManagedLabelState(raw map[string]json.RawMessage) (scope.LabelState, bool) {
	if labelValue, present := raw["label"]; present {
		var projected string
		if err := json.Unmarshal(labelValue, &projected); err == nil {
			labels := parseProjectedLabels(projected)
			for _, key := range managedLabelKeys() {
				if value, found := labels[key]; found {
					return labelStateOf(value), true
				}
			}
			return scope.LabelStateAbsent, true
		}
	}
	for _, key := range []string{"label.kubernaut.ai/managed", "labels.kubernaut.ai/managed", "label.kubernaut.ai~1managed"} {
		if value := searchString(raw, key); value != "" {
			return labelStateOf(value), true
		}
	}
	return scope.LabelStateAbsent, false
}

func managedLabelKeys() []string {
	return []string{scope.ManagedLabelKey, "kubernaut.ai~1managed"}
}

func parseProjectedLabels(projected string) map[string]string {
	labels := make(map[string]string)
	for _, entry := range strings.Split(projected, ";") {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		labels[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return labels
}

func labelStateOf(value string) scope.LabelState {
	switch value {
	case "true":
		return scope.LabelStateTrue
	case "false":
		return scope.LabelStateFalse
	case "":
		return scope.LabelStateAbsent
	default:
		return scope.LabelStateInvalid
	}
}

func searchString(raw map[string]json.RawMessage, key string) string {
	value, present := raw[key]
	if !present {
		return ""
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return ""
	}
	return decoded
}

// incompleteObservation wraps a failure so the syncer aborts before any write
// and the previous proven generation stays readable (DD-FLEET-017 D4.4).
func incompleteObservation(failure observation.ObservationFailure, err error) (observation.ListObservation, error) {
	if err == nil {
		err = errors.New(string(failure))
	}
	return observation.ListObservation{Complete: false, Failure: failure}, observationError(failure, err)
}
