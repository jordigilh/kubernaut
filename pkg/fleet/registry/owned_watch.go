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

package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

const registryRequestBudget = 30 * time.Second

var registryRetry = backoff.Config{BasePeriod: time.Second, MaxPeriod: 30 * time.Second, Multiplier: 2, JitterPercent: 10}

type registryWatchState struct {
	mu                    sync.RWMutex
	clusters              map[string]ClusterInfo
	revision              uint64
	registrationRevisions map[string]uint64
	ready                 bool
	started               bool
	stopped               bool
	watchError            error
	session               ownedWatch
	eventCh               chan ClusterEvent
	closeEvents           sync.Once
}

func newRegistryWatchState(size int) registryWatchState {
	return registryWatchState{
		clusters: make(map[string]ClusterInfo), registrationRevisions: make(map[string]uint64),
		eventCh: make(chan ClusterEvent, size),
	}
}

func (s *registryWatchState) begin() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return 0, fmt.Errorf("fleet cluster registry stopped")
	}
	s.ready = false
	s.revision++
	return s.revision, nil
}

func (s *registryWatchState) install(ctx context.Context, expected uint64, clusters map[string]ClusterInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.stopped || s.revision != expected {
		return fmt.Errorf("fleet registry session retired before publication")
	}
	publishRegistrations(&s.clusters, clusters, s.registrationRevisions, &s.revision)
	s.ready, s.started, s.watchError = true, true, nil
	return nil
}

func (s *registryWatchState) invalidate(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready, s.watchError = false, err
	s.revision++
}

// Called only while the caller holds mu, including the delete helper's
// update exclusion. A retired owner may not mutate even deleted membership.
func (s *registryWatchState) currentMaps() (map[string]ClusterInfo, map[string]uint64) {
	if s.stopped {
		return nil, nil
	}
	return s.clusters, s.registrationRevisions
}

func (s *registryWatchState) stop() {
	s.mu.Lock()
	if !s.stopped {
		s.stopped, s.ready = true, false
		s.revision++
	}
	s.mu.Unlock()
	s.session.stop()
	s.closeEvents.Do(func() { close(s.eventCh) })
}

// DD-FLEET-015: one owner serializes establishment, publication and observed
// stream retirement. Client-go owns decoding, not our readiness lifecycle.
type ownedWatch struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	initial chan struct{}
	stopped bool
}

type watchPublication struct {
	begin      func() (uint64, error)
	install    func(context.Context, uint64, map[string]ClusterInfo) error
	invalidate func(error)
	apply      func(watch.Event) error
}

type registryWatchConfig struct {
	client    dynamic.Interface
	gvr       schema.GroupVersionResource
	config    EAIGWRegistryConfig
	trackable func(*unstructured.Unstructured) (ClusterInfo, bool)
	publish   watchPublication
	logger    logr.Logger
}

func (o *ownedWatch) start(ctx context.Context, cfg registryWatchConfig) error {
	o.mu.Lock()
	if o.stopped {
		o.mu.Unlock()
		return fmt.Errorf("fleet cluster registry stopped")
	}
	if o.done == nil {
		runCtx, cancel := context.WithCancel(ctx)
		o.cancel, o.done, o.initial = cancel, make(chan struct{}), make(chan struct{})
		go o.run(runCtx, cfg)
	}
	done, initial := o.done, o.initial
	o.mu.Unlock()
	select {
	case <-ctx.Done():
		o.stop()
		return ctx.Err()
	case <-done:
		return fmt.Errorf("fleet cluster registry stopped before initial watch")
	case <-initial:
		if err := ctx.Err(); err != nil {
			o.stop()
			return err
		}
		return nil
	}
}

func (o *ownedWatch) stop() {
	o.mu.Lock()
	o.stopped = true
	if o.cancel != nil {
		o.cancel()
	}
	done := o.done
	o.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (o *ownedWatch) run(ctx context.Context, cfg registryWatchConfig) {
	defer close(o.done)
	defer cfg.publish.invalidate(fmt.Errorf("fleet registry watch owner stopped"))
	initial := true
	for attempt := int32(1); ctx.Err() == nil; attempt++ {
		err := cfg.session(ctx, func() {
			if initial {
				close(o.initial)
				initial = false
			}
			attempt = 0
		})
		// Retire before waiting or issuing another authoritative LIST. A lost
		// interval always requires a new LIST, never a watch-only reconnect.
		cfg.publish.invalidate(err)
		if ctx.Err() != nil {
			return
		}
		cfg.logger.Error(err, "fleet registry session retired")
		timer := time.NewTimer(registryRetry.Calculate(max(attempt, 1)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (cfg registryWatchConfig) session(ctx context.Context, installed func()) (sessionErr error) {
	generation, err := cfg.publish.begin()
	if err != nil {
		return err
	}
	listCtx, listCancel := context.WithTimeout(ctx, registryRequestBudget)
	clusters, rv, err := cfg.list(listCtx)
	listCancel()
	if err != nil {
		return err
	}
	streamCtx, stream, streamCancel, err := cfg.establish(ctx, rv)
	if err != nil {
		return err
	}
	defer streamCancel()
	defer stream.Stop()
	// Retire observed failures before stream teardown, which may wait for
	// transport/decoder cleanup. No guarded proof may remain healthy there.
	defer func() { cfg.publish.invalidate(sessionErr) }()
	if err := streamCtx.Err(); err != nil {
		return err
	}
	// Reject an already-observed close/error, retaining a valid pending event
	// for ordered application after the authoritative snapshot is installed.
	var pending *watch.Event
	select {
	case event, open := <-stream.ResultChan():
		if err := cfg.validateEvent(event, open); err != nil {
			return err
		}
		pending = &event
	default:
	}
	if err := cfg.publish.install(streamCtx, generation, clusters); err != nil {
		return err
	}
	installed()
	if pending != nil {
		if err := cfg.publish.apply(*pending); err != nil {
			return err
		}
	}
	return cfg.consume(streamCtx, stream)
}

func (cfg registryWatchConfig) consume(ctx context.Context, stream watch.Interface) error {
	rotation := time.NewTimer(defaultResyncPeriod)
	defer rotation.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-rotation.C:
			return fmt.Errorf("fleet registry watch rotation")
		case event, open := <-stream.ResultChan():
			if err := cfg.validateEvent(event, open); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := cfg.publish.apply(event); err != nil {
				return err
			}
		}
	}
}

func (cfg registryWatchConfig) establish(ctx context.Context, rv string) (context.Context, watch.Interface, context.CancelFunc, error) {
	// A timeout context would also kill the established stream at 30s. Stop
	// the establishment timer after Watch returns; retain parent cancellation.
	streamCtx, cancel := context.WithCancel(ctx)
	establishment := time.AfterFunc(registryRequestBudget, cancel)
	stream, err := cfg.client.Resource(cfg.gvr).Namespace(cfg.config.Namespace).Watch(streamCtx, metav1.ListOptions{
		LabelSelector: ManagedLabel + "=true", ResourceVersion: rv,
	})
	establishment.Stop()
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("establish fleet registry watch: %w", err)
	}
	if stream == nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("fleet registry watch is nil")
	}
	return streamCtx, stream, cancel, nil
}

func (cfg registryWatchConfig) validateEvent(event watch.Event, open bool) error {
	if err := validRegistryEvent(event, open); err != nil {
		return err
	}
	if event.Type == watch.Bookmark {
		return nil
	}
	obj, ok := event.Object.(*unstructured.Unstructured)
	if !ok || obj.GetNamespace() == "" || (cfg.config.Namespace != "" && obj.GetNamespace() != cfg.config.Namespace) {
		return fmt.Errorf("fleet registry watch violates namespace scope")
	}
	return nil
}

func validRegistryEvent(event watch.Event, open bool) error {
	if !open {
		return fmt.Errorf("fleet registry watch closed")
	}
	switch event.Type {
	case watch.Added, watch.Modified, watch.Deleted:
		obj, ok := event.Object.(*unstructured.Unstructured)
		if !ok || obj.GetName() == "" || obj.GetUID() == "" || obj.GetResourceVersion() == "" {
			return fmt.Errorf("fleet registry watch has invalid registration metadata")
		}
	case watch.Bookmark:
		return nil
	default:
		return fmt.Errorf("fleet registry watch error or invalid event")
	}
	return nil
}

func (cfg registryWatchConfig) list(ctx context.Context) (map[string]ClusterInfo, string, error) {
	clusters := make(map[string]ClusterInfo)
	options := metav1.ListOptions{LabelSelector: ManagedLabel + "=true", Limit: 500}
	versions := ""
	seen := make(map[string]bool)
	for {
		page, err := cfg.client.Resource(cfg.gvr).Namespace(cfg.config.Namespace).List(ctx, options)
		if err != nil {
			return nil, "", fmt.Errorf("list fleet registrations: %w", err)
		}
		if page == nil || page.GetResourceVersion() == "" || (versions != "" && versions != page.GetResourceVersion()) {
			return nil, "", fmt.Errorf("fleet registration list has no consistent resource version")
		}
		versions = page.GetResourceVersion()
		if err := cfg.addPage(clusters, page); err != nil {
			return nil, "", err
		}
		next := page.GetContinue()
		if next == "" {
			if remaining := page.GetRemainingItemCount(); remaining != nil && *remaining != 0 {
				return nil, "", fmt.Errorf("fleet registration pagination is incomplete")
			}
			return clusters, versions, nil
		}
		if seen[next] {
			return nil, "", fmt.Errorf("fleet registration pagination repeated a token")
		}
		seen[next] = true
		options.Continue = next
	}
}

func (cfg registryWatchConfig) addPage(clusters map[string]ClusterInfo, page *unstructured.UnstructuredList) error {
	for i := range page.Items {
		obj := &page.Items[i]
		if obj.GetName() == "" || obj.GetUID() == "" || obj.GetNamespace() == "" || (cfg.config.Namespace != "" && obj.GetNamespace() != cfg.config.Namespace) {
			return fmt.Errorf("fleet registration list has invalid identity")
		}
		if obj.GetLabels()[ManagedLabel] != managedLabelValue {
			return fmt.Errorf("fleet registration list violates managed selector")
		}
		info, included := cfg.trackable(obj)
		if !included {
			continue
		}
		if _, duplicate := clusters[info.ID]; duplicate {
			return fmt.Errorf("fleet registration list has ambiguous registration name")
		}
		clusters[info.ID] = info
	}
	return nil
}
