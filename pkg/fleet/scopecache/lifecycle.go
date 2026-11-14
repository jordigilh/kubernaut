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
	"errors"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// WriterFence is the complete identity of the current FMC writer. LeaseEpoch
// is the only monotonic source; Fence is its non-negative Valkey encoding.
type WriterFence struct {
	HolderPodUID     string
	HolderSessionUID string
	LeaseEpoch       int64
	Fence            uint64
}

// Validate rejects incomplete or internally inconsistent writer identities.
func (f WriterFence) Validate() error {
	if f.HolderPodUID == "" || f.HolderSessionUID == "" {
		return errors.New("writer fence holder identity is required")
	}
	if f.LeaseEpoch <= 0 || f.Fence != uint64(f.LeaseEpoch) {
		return errors.New("writer fence epoch and fence must be equal and positive")
	}
	return nil
}

// CycleToken identifies one guarded write cycle.
type CycleToken struct {
	Partition  PartitionStateKey
	Fence      WriterFence
	Generation uint64
	Token      string
}

func (t CycleToken) Validate() error {
	if err := t.Partition.Validate(); err != nil {
		return err
	}
	if err := t.Fence.Validate(); err != nil {
		return err
	}
	if t.Generation == 0 || t.Token == "" {
		return errors.New("cycle token generation and token are required")
	}
	return nil
}

// ResourceIdentity is the public six-field identity stored in a partition.
// It intentionally excludes registration UID.
type ResourceIdentity struct {
	ClusterID string
	Group     string
	Version   string
	Kind      string
	Namespace string
	Name      string
}

func (r ResourceIdentity) Validate() error {
	if r.ClusterID == "" || r.Kind == "" || r.Name == "" {
		return errors.New("resource identity requires cluster, kind and name")
	}
	if r.Version == "" {
		return errors.New("resource identity requires an explicit version")
	}
	return nil
}

// Observation is the bounded source observation written to the cache.
type Observation struct {
	Identity   ResourceIdentity
	LabelState scope.LabelState
	TTL        time.Duration
}

func (o Observation) Validate() error {
	if err := o.Identity.Validate(); err != nil {
		return err
	}
	switch o.LabelState {
	case scope.LabelStateTrue, scope.LabelStateFalse, scope.LabelStateAbsent:
	default:
		return fmt.Errorf("unsupported observation label state %q", o.LabelState)
	}
	if o.TTL <= 0 {
		return errors.New("observation TTL must be positive")
	}
	return nil
}

// WriteOutcome describes the script's write result without collapsing an
// ambiguous reply into success.
type WriteOutcome string

const (
	WriteVerified  WriteOutcome = "verified"
	WriteUncertain WriteOutcome = "uncertain"
)

// ResourceRecord is a verified materialized record returned by StatusStore.
type ResourceRecord struct {
	Identity   ResourceIdentity
	LabelState scope.LabelState
	Generation uint64
	ExpiresAt  time.Time
}

// ReadState is deliberately distinct from managed=false. Missing and
// uncertain state cannot authorize a remote request.
type ReadState string

const (
	ReadVerified  ReadState = "verified"
	ReadMissing   ReadState = "missing"
	ReadUncertain ReadState = "uncertain"
)

// CycleStore is the normal post-repair cycle boundary. A normal cycle cannot
// establish clean state for an unrepaired partition.
type CycleStore interface {
	BeginCycle(context.Context, PartitionStateKey, WriterFence) (CycleToken, error)
	PutObserved(context.Context, CycleToken, Observation) (WriteOutcome, error)
	CommitCycle(context.Context, CycleToken, int) error
	AbortCycle(context.Context, CycleToken, string) error
	MarkUncertain(context.Context, CycleToken, string) error
}

// StatusStore is the proof-bearing repair and fenced-read boundary. Repair and
// read methods intentionally use request/result types so token, revision, full
// fence, generation, and TTL proof cannot be dropped at a call site.
type StatusStore interface {
	BeginRepair(context.Context, BeginRepairRequest) (RepairStartResult, error)
	MarkRepairUncertain(context.Context, RepairStartResult, RepairFailure) error
	Repair(context.Context, RepairRequest) (RepairResult, error)
	CommitRepair(context.Context, RepairCommitRequest) (RepairCommitResult, error)
	ReadVerified(context.Context, FencedReadRequest) (FencedReadResult, error)
}

// CacheStore is the concrete production composition used by the syncer.
type CacheStore interface {
	StatusStore
	CycleStore
}

// LeaseCoordinator owns the Kubernetes Lease that supplies WriterFence.
type LeaseCoordinator interface {
	Acquire(context.Context) (LeaseSession, error)
	Verify(context.Context, WriterFence) error
	Renew(context.Context, WriterFence) error
	Release(context.Context, WriterFence) error
}

// LeaseSession binds a loss channel to one exact writer fence. A newer
// acquisition must never reuse or close an older session's channel.
type LeaseSession struct {
	Fence WriterFence
	Loss  <-chan struct{}
}

var (
	ErrStaleFence        = errors.New("stale writer fence")
	ErrAmbiguousWrite    = errors.New("ambiguous cache write")
	ErrScriptUnavailable = errors.New("cache script unavailable")
	ErrLeaseHeld         = errors.New("cache lease held by another writer")
)
