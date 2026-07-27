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

package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// BufferedDSAuditStore wraps pkg/audit.BufferedAuditStore to implement KA's
// internal AuditStore interface. Events are converted from KA's AuditEvent
// format to the OpenAPI AuditEventRequest format, then enqueued into the
// platform buffered store for batched writes with retry.
//
// Uses the same OpenAPIClientAdapter + BufferedAuditStore stack as every other
// platform service (DD-AUDIT-002 alignment).
type BufferedDSAuditStore struct {
	inner sharedaudit.AuditStore
}

// BufferedDSAuditStoreOption allows callers to override RecommendedConfig fields.
type BufferedDSAuditStoreOption func(*sharedaudit.Config)

// WithFlushInterval overrides the default flush interval from RecommendedConfig.
func WithFlushInterval(d time.Duration) BufferedDSAuditStoreOption {
	return func(c *sharedaudit.Config) {
		if d > 0 {
			c.FlushInterval = d
		}
	}
}

// WithBufferSize overrides the default buffer size from RecommendedConfig.
func WithBufferSize(n int) BufferedDSAuditStoreOption {
	return func(c *sharedaudit.Config) {
		if n > 0 {
			c.BufferSize = n
		}
	}
}

// WithBatchSize overrides the default batch size from RecommendedConfig.
func WithBatchSize(n int) BufferedDSAuditStoreOption {
	return func(c *sharedaudit.Config) {
		if n > 0 {
			c.BatchSize = n
		}
	}
}

// NewBufferedDSAuditStore creates a KA audit store backed by the platform
// BufferedAuditStore. The caller provides a DataStorageClient (typically
// created via audit.NewOpenAPIClientAdapterWithTransport to share auth
// transport with the rest of KA's DS operations).
//
// Optional BufferedDSAuditStoreOption values override the RecommendedConfig
// defaults, allowing integration/E2E tests to control flush behaviour via
// the same YAML fields that KA v1.2 used (flush_interval_seconds, etc.).
func NewBufferedDSAuditStore(dsClient sharedaudit.DataStorageClient, logger logr.Logger, opts ...BufferedDSAuditStoreOption) (*BufferedDSAuditStore, error) {
	cfg := sharedaudit.RecommendedConfig("kubernaut-agent")
	for _, o := range opts {
		o(&cfg)
	}
	inner, err := sharedaudit.NewBufferedStore(dsClient, cfg, "kubernaut-agent", logger)
	if err != nil {
		return nil, fmt.Errorf("create buffered audit store: %w", err)
	}
	return &BufferedDSAuditStore{inner: inner}, nil
}

func (s *BufferedDSAuditStore) StoreAudit(ctx context.Context, event *AuditEvent) error {
	req := &ogenclient.AuditEventRequest{
		Version:        "1.0",
		EventType:      event.EventType,
		EventTimestamp: time.Now().UTC(),
		EventCategory:  ogenclient.AuditEventRequestEventCategory(event.EventCategory),
		EventAction:    event.EventAction,
		EventOutcome:   ogenclient.AuditEventRequestEventOutcome(event.EventOutcome),
		CorrelationID:  event.CorrelationID,
	}
	if event.ActingUser != "" {
		req.ActorType.SetTo("User")
		req.ActorID.SetTo(event.ActingUser)
	} else {
		bActorType := "Service"
		bActorID := "kubernaut-agent"
		if event.ActorID != "" {
			bActorID = event.ActorID
		}
		if event.ActorType != "" {
			bActorType = event.ActorType
		}
		req.ActorType.SetTo(bActorType)
		req.ActorID.SetTo(bActorID)
	}
	if event.ParentEventID != nil {
		req.ParentEventID.SetTo(*event.ParentEventID)
	}
	if event.ClusterID != "" {
		req.ClusterID.SetTo(event.ClusterID)
	}
	if event.ResourceType != "" {
		req.ResourceType.SetTo(event.ResourceType)
	}
	if event.ResourceID != "" {
		req.ResourceID.SetTo(event.ResourceID)
	}

	if ed, ok := buildEventData(event); ok {
		req.EventData = ed
	}

	return s.inner.StoreAudit(ctx, req)
}

// Flush forces all buffered events to be written to DataStorage.
func (s *BufferedDSAuditStore) Flush(ctx context.Context) error {
	return s.inner.Flush(ctx)
}

// Close flushes remaining events and stops the background worker.
func (s *BufferedDSAuditStore) Close() error {
	return s.inner.Close()
}

var _ AuditStore = (*BufferedDSAuditStore)(nil)
