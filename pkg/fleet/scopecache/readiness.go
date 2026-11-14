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
	"fmt"
	"sync"
	"time"
)

// PartitionStateKey identifies one registration incarnation in the materialized
// cache. RegistrationUID prevents a delete/recreate event from reusing clean
// state that belongs to an older incarnation.
type PartitionStateKey struct {
	ClusterID       string
	RegistrationUID string
}

func (k PartitionStateKey) valid() bool {
	return k.ClusterID != "" && k.RegistrationUID != ""
}

// Validate rejects a partition key without a cluster or registration
// incarnation. The registration UID is never optional for cache state.
func (k PartitionStateKey) Validate() error {
	if !k.valid() {
		return fmt.Errorf("partition state key requires cluster and registration identity")
	}
	return nil
}

// PartitionReadinessState is the only state used by the shared FMC readiness
// snapshot. Unknown, repairing, and uncertain states are unavailable; only a
// clean state with a complete proof may authorize a read.
type PartitionReadinessState string

const (
	PartitionUnknown   PartitionReadinessState = "unknown"
	PartitionRepairing PartitionReadinessState = "repairing"
	PartitionUncertain PartitionReadinessState = "uncertain"
	PartitionClean     PartitionReadinessState = "clean"
)

// LeasePolicy controls whether the process-wide Lease must be active for FMC
// service readiness. A process-wide optional Lease may be absent when there are
// no remote partitions, but an active Lease must still be valid.
type LeasePolicy string

const (
	LeasePolicyProcessWideRequired LeasePolicy = "process-wide-required"
	LeasePolicyProcessWideOptional LeasePolicy = "process-wide-optional"
)

// LeaseView is the readiness snapshot of the Lease session that supplied the
// writer fence. The heartbeat and fence are retained together so readiness
// cannot be reconstructed from an epoch alone.
type LeaseView struct {
	Policy         LeasePolicy
	Active         bool
	Valid          bool
	Epoch          int64
	SessionUID     string
	WriterFence    WriterFence
	HeartbeatAt    time.Time
	HeartbeatValid bool
	Invalid        bool
}

// FMCGlobalReadiness records prerequisites shared by all cache partitions.
type FMCGlobalReadiness struct {
	MCPInitialSync       bool
	RegistryInitialSync  bool
	RegistryWatchHealthy bool
	ScriptReady          bool
	ValkeyReady          bool
	Lease                LeaseView
	ActiveLeaseValid     bool
	LeasePolicy          LeasePolicy
}

// ServiceReady reports whether global FMC prerequisites permit service
// readiness. Partition-specific proof is checked by FMCReadinessSnapshot.
func (g FMCGlobalReadiness) ServiceReady(partitionCount int) bool {
	if partitionCount < 0 || !g.globalDependenciesReady() {
		return false
	}
	if g.Lease.Invalid || (g.Lease.Active && !g.Lease.healthy()) {
		return false
	}
	if g.LeasePolicy == LeasePolicyProcessWideRequired {
		if partitionCount == 0 {
			return true
		}
		return g.ActiveLeaseValid && g.Lease.Active && g.Lease.healthy()
	}
	if g.LeasePolicy == LeasePolicyProcessWideOptional {
		return !g.Lease.Active || g.ActiveLeaseValid
	}
	return false
}

func (g FMCGlobalReadiness) globalDependenciesReady() bool {
	return g.MCPInitialSync && g.RegistryInitialSync && g.RegistryWatchHealthy && g.ScriptReady && g.ValkeyReady
}

func (lease LeaseView) healthy() bool {
	return lease.Valid && lease.HeartbeatValid
}

// PartitionReadiness is the proof-bearing readiness state for one partition.
// Every validity predicate is explicit so callers cannot infer clean state from
// a boolean or from a partial record.
type PartitionReadiness struct {
	Key                  PartitionStateKey
	RegistrationUID      string
	RegistrationRevision uint64
	LeaseEpoch           int64
	LeaseSessionUID      string
	WriterFence          WriterFence
	LeaseHeartbeatAt     time.Time
	LeaseHeartbeatValid  bool
	Generation           uint64
	State                PartitionReadinessState
	CleanCommit          bool
	RecordValid          bool
	IndexValid           bool
	StatusValid          bool
	RecordTTLValid       bool
	IndexTTLValid        bool
	StatusTTLValid       bool
	InvalidLease         bool
}

// FMCReadinessView is an immutable value returned by View. The map is copied
// on every read so callers cannot mutate the snapshot without validation.
type FMCReadinessView struct {
	Global     FMCGlobalReadiness
	Partitions map[PartitionStateKey]PartitionReadiness
}

// FMCReadinessSnapshot is the single concurrency-safe readiness authority for
// FMC global and partition state.
type FMCReadinessSnapshot struct {
	mu   sync.RWMutex
	view FMCReadinessView
}

// ReadinessErrorCode identifies whether a readiness rejection is invalid input
// or an unavailable operational state.
type ReadinessErrorCode string

const (
	ReadinessUnavailable ReadinessErrorCode = "unavailable"
	ReadinessInvalid     ReadinessErrorCode = "invalid"
)

// ReadinessError is returned instead of a boolean false when a readiness or
// fenced-read proof cannot be established.
type ReadinessError struct {
	Code   ReadinessErrorCode
	Reason string
}

func (e ReadinessError) Error() string {
	return fmt.Sprintf("readiness %s: %s", e.Code, e.Reason)
}

// NewFMCReadinessSnapshot creates an empty readiness authority for one Lease
// policy. Unsupported policies fail closed at construction time.
func NewFMCReadinessSnapshot(policy LeasePolicy) (*FMCReadinessSnapshot, error) {
	if !validLeasePolicy(policy) {
		return nil, ReadinessError{Code: ReadinessInvalid, Reason: "unsupported lease policy"}
	}
	return &FMCReadinessSnapshot{
		view: FMCReadinessView{
			Global:     FMCGlobalReadiness{LeasePolicy: policy},
			Partitions: make(map[PartitionStateKey]PartitionReadiness),
		},
	}, nil
}

// PublishGlobal replaces the global prerequisite view after validating the
// Lease policy and active Lease predicates.
func (s *FMCReadinessSnapshot) PublishGlobal(global FMCGlobalReadiness) error {
	if s == nil {
		return ReadinessError{Code: ReadinessInvalid, Reason: "readiness snapshot is nil"}
	}
	if !validLeasePolicy(global.LeasePolicy) || global.Lease.Policy != global.LeasePolicy {
		return ReadinessError{Code: ReadinessInvalid, Reason: "global lease policy is inconsistent"}
	}
	if err := validateLeaseView(global.Lease); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.view.Global.LeasePolicy != "" && s.view.Global.LeasePolicy != global.LeasePolicy {
		return ReadinessError{Code: ReadinessInvalid, Reason: "global lease policy changed"}
	}
	s.view.Global = cloneGlobal(global)
	return nil
}

// BeginPartitionRepair records an unavailable repairing state before source
// collection begins. It always clears clean eligibility for the partition.
func (s *FMCReadinessSnapshot) BeginPartitionRepair(key PartitionStateKey, registrationUID string, revision uint64, lease LeaseView, generation uint64) error {
	if err := validatePartitionInputs(key, registrationUID, revision, lease, generation); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensurePartitions()
	s.view.Partitions[key] = PartitionReadiness{
		Key:                  key,
		RegistrationUID:      registrationUID,
		RegistrationRevision: revision,
		LeaseEpoch:           lease.Epoch,
		LeaseSessionUID:      lease.SessionUID,
		WriterFence:          lease.WriterFence,
		LeaseHeartbeatAt:     lease.HeartbeatAt,
		LeaseHeartbeatValid:  lease.HeartbeatValid,
		Generation:           generation,
		State:                PartitionRepairing,
	}
	return nil
}

// PublishPartitionClean publishes only a complete proof-bearing clean state.
func (s *FMCReadinessSnapshot) PublishPartitionClean(key PartitionStateKey, readiness PartitionReadiness) error {
	if err := validateCleanPartition(key, readiness); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensurePartitions()
	current, ok := s.view.Partitions[key]
	if !ok || current.State != PartitionRepairing || current.Generation != readiness.Generation {
		return ReadinessError{Code: ReadinessUnavailable, Reason: "partition is not in the requested repair generation"}
	}
	if current.RegistrationRevision != readiness.RegistrationRevision || !sameFence(current.WriterFence, readiness.WriterFence) {
		return ReadinessError{Code: ReadinessUnavailable, Reason: "partition repair proof is stale"}
	}
	s.view.Partitions[key] = clonePartition(readiness)
	return nil
}

// InvalidateLease makes every partition owned by the lost full fence
// unavailable and invalidates the global active-Lease view.
func (s *FMCReadinessSnapshot) InvalidateLease(fence WriterFence, _ string) error {
	if err := fence.Validate(); err != nil {
		return ReadinessError{Code: ReadinessInvalid, Reason: err.Error()}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, partition := range s.view.Partitions {
		if sameFence(partition.WriterFence, fence) {
			partition.State = PartitionUncertain
			partition.CleanCommit = false
			partition.InvalidLease = true
			s.view.Partitions[key] = partition
		}
	}
	s.view.Global.ActiveLeaseValid = false
	s.view.Global.Lease.Valid = false
	s.view.Global.Lease.Invalid = true
	return nil
}

// InvalidatePartition removes clean eligibility before a registry revision
// change or other partition-scoped authority change is observed by readers.
func (s *FMCReadinessSnapshot) InvalidatePartition(key PartitionStateKey, _ string) error {
	if !key.valid() {
		return ReadinessError{Code: ReadinessInvalid, Reason: "partition key is incomplete"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	partition, ok := s.view.Partitions[key]
	if !ok {
		return nil
	}
	partition.State = PartitionUncertain
	partition.CleanCommit = false
	partition.InvalidLease = false
	s.view.Partitions[key] = partition
	return nil
}

// ServiceReady reports readiness only when global prerequisites and every
// known partition have a complete clean proof.
func (s *FMCReadinessSnapshot) ServiceReady() (bool, error) {
	if s == nil {
		return false, ReadinessError{Code: ReadinessInvalid, Reason: "readiness snapshot is nil"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.view.Global.ServiceReady(len(s.view.Partitions)) {
		return false, ReadinessError{Code: ReadinessUnavailable, Reason: "global FMC prerequisites are not ready"}
	}
	for _, partition := range s.view.Partitions {
		if !partitionClean(partition) {
			return false, ReadinessError{Code: ReadinessUnavailable, Reason: "partition is not clean"}
		}
	}
	return true, nil
}

// FencedReadProof is returned only after all current revision, fence,
// generation, state, and TTL predicates are validated.
type FencedReadProof struct {
	Partition            PartitionStateKey
	RegistrationUID      string
	RegistrationRevision uint64
	Generation           uint64
	WriterFence          WriterFence
	RecordValid          bool
	IndexValid           bool
	StatusValid          bool
	RecordTTLValid       bool
	IndexTTLValid        bool
	StatusTTLValid       bool
}

// PartitionReady returns the proof required before a scope read may map a
// materialized record to an authorization decision.
func (s *FMCReadinessSnapshot) PartitionReady(partition PartitionStateKey, registrationUID string, expectedRevision uint64, fence WriterFence, generation uint64) (FencedReadProof, error) {
	if !partition.valid() || registrationUID == "" || expectedRevision == 0 || generation == 0 {
		return FencedReadProof{}, ReadinessError{Code: ReadinessInvalid, Reason: "fenced-read identity is incomplete"}
	}
	if err := fence.Validate(); err != nil {
		return FencedReadProof{}, ReadinessError{Code: ReadinessInvalid, Reason: err.Error()}
	}
	if s == nil {
		return FencedReadProof{}, ReadinessError{Code: ReadinessInvalid, Reason: "readiness snapshot is nil"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.view.Global.ServiceReady(len(s.view.Partitions)) {
		return FencedReadProof{}, ReadinessError{Code: ReadinessUnavailable, Reason: "global FMC prerequisites are not ready"}
	}
	current, ok := s.view.Partitions[partition]
	if !ok {
		return FencedReadProof{}, ReadinessError{Code: ReadinessUnavailable, Reason: "partition is unknown"}
	}
	if current.RegistrationUID != registrationUID || current.RegistrationRevision != expectedRevision {
		return FencedReadProof{}, ReadinessError{Code: ReadinessUnavailable, Reason: "registration revision proof is stale"}
	}
	if current.Generation != generation || !sameFence(current.WriterFence, fence) {
		return FencedReadProof{}, ReadinessError{Code: ReadinessUnavailable, Reason: "writer fence or generation proof is stale"}
	}
	if !partitionClean(current) {
		return FencedReadProof{}, ReadinessError{Code: ReadinessUnavailable, Reason: "partition is not clean"}
	}
	return FencedReadProof{
		Partition:            partition,
		RegistrationUID:      registrationUID,
		RegistrationRevision: expectedRevision,
		Generation:           generation,
		WriterFence:          fence,
		RecordValid:          current.RecordValid,
		IndexValid:           current.IndexValid,
		StatusValid:          current.StatusValid,
		RecordTTLValid:       current.RecordTTLValid,
		IndexTTLValid:        current.IndexTTLValid,
		StatusTTLValid:       current.StatusTTLValid,
	}, nil
}

// View returns a deep copy of the current readiness state.
func (s *FMCReadinessSnapshot) View() FMCReadinessView {
	if s == nil {
		return FMCReadinessView{Partitions: make(map[PartitionStateKey]PartitionReadiness)}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneView(s.view)
}

func (s *FMCReadinessSnapshot) ensurePartitions() {
	if s.view.Partitions == nil {
		s.view.Partitions = make(map[PartitionStateKey]PartitionReadiness)
	}
}

func validatePartitionInputs(key PartitionStateKey, registrationUID string, revision uint64, lease LeaseView, generation uint64) error {
	if !key.valid() || registrationUID == "" || registrationUID != key.RegistrationUID || revision == 0 || generation == 0 {
		return ReadinessError{Code: ReadinessInvalid, Reason: "partition repair identity is incomplete"}
	}
	if err := validateLeaseView(lease); err != nil {
		return err
	}
	return nil
}

func validateCleanPartition(key PartitionStateKey, readiness PartitionReadiness) error {
	if !cleanPartitionIdentityMatches(key, readiness) {
		return ReadinessError{Code: ReadinessInvalid, Reason: "clean partition identity is inconsistent"}
	}
	if !readiness.cleanProofComplete() {
		return ReadinessError{Code: ReadinessInvalid, Reason: "clean partition proof is incomplete"}
	}
	if err := readiness.WriterFence.Validate(); err != nil {
		return ReadinessError{Code: ReadinessInvalid, Reason: err.Error()}
	}
	if readiness.LeaseEpoch != readiness.WriterFence.LeaseEpoch || readiness.LeaseSessionUID != readiness.WriterFence.HolderSessionUID {
		return ReadinessError{Code: ReadinessInvalid, Reason: "clean partition lease proof is inconsistent"}
	}
	if readiness.LeaseHeartbeatAt.IsZero() {
		return ReadinessError{Code: ReadinessInvalid, Reason: "clean partition heartbeat is missing"}
	}
	return nil
}

func cleanPartitionIdentityMatches(key PartitionStateKey, readiness PartitionReadiness) bool {
	return readiness.Key == key && readiness.RegistrationUID == key.RegistrationUID &&
		readiness.RegistrationRevision != 0 && readiness.Generation != 0
}

func (readiness PartitionReadiness) cleanProofComplete() bool {
	return readiness.State == PartitionClean && readiness.CleanCommit && !readiness.InvalidLease &&
		readiness.RecordValid && readiness.IndexValid && readiness.StatusValid &&
		readiness.RecordTTLValid && readiness.IndexTTLValid && readiness.StatusTTLValid &&
		readiness.LeaseHeartbeatValid
}

func partitionClean(partition PartitionReadiness) bool {
	return partition.State == PartitionClean && partition.CleanCommit && !partition.InvalidLease &&
		partition.LeaseHeartbeatValid && partition.RecordValid && partition.IndexValid &&
		partition.StatusValid && partition.RecordTTLValid && partition.IndexTTLValid && partition.StatusTTLValid
}

func validateLeaseView(lease LeaseView) error {
	if !validLeasePolicy(lease.Policy) {
		return ReadinessError{Code: ReadinessInvalid, Reason: "unsupported lease policy"}
	}
	if !lease.Active {
		if lease.Valid || lease.Invalid || lease.HeartbeatValid || !lease.HeartbeatAt.IsZero() {
			return ReadinessError{Code: ReadinessInvalid, Reason: "inactive lease view contains active proof"}
		}
		return nil
	}
	if !lease.Valid || lease.Invalid || !lease.HeartbeatValid || lease.HeartbeatAt.IsZero() {
		return ReadinessError{Code: ReadinessUnavailable, Reason: "lease is not valid"}
	}
	if err := lease.WriterFence.Validate(); err != nil {
		return ReadinessError{Code: ReadinessInvalid, Reason: err.Error()}
	}
	if lease.Epoch != lease.WriterFence.LeaseEpoch || lease.SessionUID != lease.WriterFence.HolderSessionUID {
		return ReadinessError{Code: ReadinessInvalid, Reason: "lease view and writer fence differ"}
	}
	return nil
}

func validLeasePolicy(policy LeasePolicy) bool {
	return policy == LeasePolicyProcessWideRequired || policy == LeasePolicyProcessWideOptional
}

func sameFence(left, right WriterFence) bool {
	return left == right
}

func cloneGlobal(global FMCGlobalReadiness) FMCGlobalReadiness {
	return global
}

func clonePartition(partition PartitionReadiness) PartitionReadiness {
	return partition
}

func cloneView(view FMCReadinessView) FMCReadinessView {
	partitions := make(map[PartitionStateKey]PartitionReadiness, len(view.Partitions))
	for key, partition := range view.Partitions {
		partitions[key] = clonePartition(partition)
	}
	view.Partitions = partitions
	return view
}
