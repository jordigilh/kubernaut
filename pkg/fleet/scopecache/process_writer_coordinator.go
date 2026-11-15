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

package scopecache

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// ProcessWriterCoordinator supplies a writer fence entirely within one FMC
// process (DD-FLEET-012). It is intentionally suitable only for the current
// singleton FMC deployment; it provides no cross-process or cross-pod fencing.
type ProcessWriterCoordinator struct {
	mu       sync.Mutex
	holderID string
	epoch    int64
	current  WriterFence
	lostCh   chan struct{}
}

// NewProcessWriterCoordinator creates the singleton writer coordinator.
func NewProcessWriterCoordinator() *ProcessWriterCoordinator {
	return &ProcessWriterCoordinator{holderID: uuid.NewString()}
}

// Acquire starts a new process-local writer session.
func (c *ProcessWriterCoordinator) Acquire(_ context.Context) (LeaseSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.current != (WriterFence{}) {
		return LeaseSession{}, ErrLeaseHeld
	}
	if c.epoch == int64(^uint64(0)>>1) {
		return LeaseSession{}, fmt.Errorf("process writer fence epoch is exhausted")
	}
	c.epoch++
	fence := WriterFence{
		HolderPodUID:     c.holderID,
		HolderSessionUID: uuid.NewString(),
		LeaseEpoch:       c.epoch,
		Fence:            uint64(c.epoch),
	}
	c.current = fence
	c.lostCh = make(chan struct{})
	return LeaseSession{Fence: fence, Loss: c.lostCh}, nil
}

// Verify confirms that the process still owns the supplied writer fence.
func (c *ProcessWriterCoordinator) Verify(_ context.Context, expected WriterFence) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != expected {
		return ErrStaleFence
	}
	return nil
}

// Renew is a no-op heartbeat for a process-local writer session.
func (c *ProcessWriterCoordinator) Renew(ctx context.Context, expected WriterFence) error {
	return c.Verify(ctx, expected)
}

// Release ends the process-local writer session.
func (c *ProcessWriterCoordinator) Release(_ context.Context, expected WriterFence) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != expected {
		return ErrStaleFence
	}
	close(c.lostCh)
	c.current = WriterFence{}
	c.lostCh = nil
	return nil
}

var _ LeaseCoordinator = (*ProcessWriterCoordinator)(nil)
