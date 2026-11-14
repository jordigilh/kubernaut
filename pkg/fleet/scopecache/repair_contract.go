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
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// RepairFailureCode classifies a repair failure without collapsing source,
// authority, fence, and persistence uncertainty into one generic error.
type RepairFailureCode string

const (
	RepairFailureNone               RepairFailureCode = ""
	RepairFailureInvalidRequest     RepairFailureCode = "invalid_request"
	RepairFailureSourceIncomplete   RepairFailureCode = "source_incomplete"
	RepairFailureBoundsExceeded     RepairFailureCode = "bounds_exceeded"
	RepairFailureLeaseLost          RepairFailureCode = "lease_lost"
	RepairFailureStaleRegistration  RepairFailureCode = "stale_registration"
	RepairFailureStaleFence         RepairFailureCode = "stale_fence"
	RepairFailureGenerationConflict RepairFailureCode = "generation_conflict"
	RepairFailureScriptUnavailable  RepairFailureCode = "script_unavailable"
	RepairFailureAmbiguousWrite     RepairFailureCode = "ambiguous_write"
	RepairFailureCountMismatch      RepairFailureCode = "count_mismatch"
	RepairFailureRecordInvalid      RepairFailureCode = "record_invalid"
	RepairFailureIndexInvalid       RepairFailureCode = "index_invalid"
	RepairFailureStatusInvalid      RepairFailureCode = "status_invalid"
	RepairFailureTTLInvalid         RepairFailureCode = "ttl_invalid"
	RepairFailureCommitRejected     RepairFailureCode = "commit_rejected"
)

// RepairFailure is the typed failure carried by repair and commit results.
type RepairFailure struct {
	Code      RepairFailureCode
	Message   string
	Retryable bool
}

func (f RepairFailure) Error() string {
	if f.Code == RepairFailureNone && f.Message == "" {
		return ""
	}
	if f.Message == "" {
		return string(f.Code)
	}
	if f.Code == RepairFailureNone {
		return f.Message
	}
	return fmt.Sprintf("%s: %s", f.Code, f.Message)
}

// RepairFailureError preserves a typed repair failure as an error return.
type RepairFailureError struct {
	Failure RepairFailure
}

func (e RepairFailureError) Error() string {
	return fmt.Sprintf("repair failed: %s", e.Failure.Error())
}

func (e RepairFailureError) Unwrap() error {
	switch e.Failure.Code {
	case RepairFailureStaleFence, RepairFailureLeaseLost:
		return ErrStaleFence
	case RepairFailureScriptUnavailable:
		return ErrScriptUnavailable
	case RepairFailureAmbiguousWrite:
		return ErrAmbiguousWrite
	default:
		return nil
	}
}

// RepairToken is an opaque token that binds BeginRepair through commit.
type RepairToken struct {
	Value string
}

func (t RepairToken) Validate() error {
	if t.Value == "" {
		return fmt.Errorf("repair token is required")
	}
	return nil
}

// RepairObservation is the validated, typed representation of one complete
// source item. APIVersion is retained separately to reject group/version
// inference at the repair boundary.
type RepairObservation struct {
	Identity      ResourceIdentity
	APIVersion    string
	LabelsPresent bool
	LabelState    scope.LabelState
	TTL           time.Duration
}

func (o RepairObservation) Validate() error {
	if err := o.Identity.Validate(); err != nil {
		return err
	}
	if o.APIVersion == "" {
		return fmt.Errorf("repair observation apiVersion is required")
	}
	gv, err := schema.ParseGroupVersion(o.APIVersion)
	if err != nil || gv.Version == "" || gv.Group != o.Identity.Group || gv.Version != o.Identity.Version {
		return fmt.Errorf("repair observation apiVersion does not match identity")
	}
	switch o.LabelState {
	case scope.LabelStateTrue, scope.LabelStateFalse, scope.LabelStateAbsent:
	default:
		return fmt.Errorf("repair observation label state is invalid")
	}
	if o.TTL <= 0 {
		return fmt.Errorf("repair observation ttl must be positive")
	}
	return nil
}

// ValidatedObservationSet is the complete bounded aggregate supplied to
// Repair. Partial source observations are never eligible for cache mutation.
type ValidatedObservationSet struct {
	Items     []RepairObservation
	Complete  bool
	PageCount int
	ItemCount int
}

func (s ValidatedObservationSet) Validate() error {
	if !s.Complete {
		return fmt.Errorf("repair observation source is incomplete")
	}
	if s.PageCount <= 0 || s.PageCount > 100 {
		return fmt.Errorf("repair observation page count is out of bounds")
	}
	if s.ItemCount < 0 || s.ItemCount > 5000 || s.ItemCount != len(s.Items) {
		return fmt.Errorf("repair observation item count is invalid")
	}
	for index, item := range s.Items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("repair observation %d: %w", index, err)
		}
	}
	return nil
}

type BeginRepairRequest struct {
	Partition                PartitionStateKey
	RegistrationUID          string
	ExpectedRegistryRevision uint64
	WriterFence              WriterFence
}

type RepairStartResult struct {
	Partition                PartitionStateKey
	RegistrationUID          string
	ExpectedRegistryRevision uint64
	WriterFence              WriterFence
	Generation               uint64
	Token                    RepairToken
}

func (r RepairStartResult) Validate() error {
	if err := r.Partition.Validate(); err != nil {
		return err
	}
	if r.RegistrationUID == "" || r.RegistrationUID != r.Partition.RegistrationUID {
		return fmt.Errorf("repair start registration identity is inconsistent")
	}
	if r.ExpectedRegistryRevision == 0 || r.Generation == 0 {
		return fmt.Errorf("repair start revision and generation are required")
	}
	if err := r.WriterFence.Validate(); err != nil {
		return err
	}
	return r.Token.Validate()
}

type TTLPTTLValidation struct {
	ServerNowMillis int64
	ExpiresAtMillis int64
	PTTLMillis      int64
	DeltaMillis     int64
	Valid           bool
}

type RepairRequest struct {
	Partition                PartitionStateKey
	RegistrationUID          string
	ExpectedRegistryRevision uint64
	WriterFence              WriterFence
	Generation               uint64
	Token                    RepairToken
	Observations             ValidatedObservationSet
}

func (r RepairRequest) Validate() error {
	if err := r.Partition.Validate(); err != nil {
		return err
	}
	if r.RegistrationUID == "" || r.RegistrationUID != r.Partition.RegistrationUID {
		return fmt.Errorf("repair request registration identity is inconsistent")
	}
	if r.ExpectedRegistryRevision == 0 || r.Generation == 0 {
		return fmt.Errorf("repair request revision and generation are required")
	}
	if err := r.WriterFence.Validate(); err != nil {
		return err
	}
	if err := r.Token.Validate(); err != nil {
		return err
	}
	return r.Observations.Validate()
}

type RepairResult struct {
	Partition                PartitionStateKey
	RegistrationUID          string
	ExpectedRegistryRevision uint64
	ProvenRegistryRevision   uint64
	WriterFence              WriterFence
	ProvenGeneration         uint64
	ProvenWriterFence        WriterFence
	Token                    RepairToken
	ExpectedCount            int
	ObservedCount            int
	CleanupCount             int
	SourceComplete           bool
	RecordValid              bool
	IndexValid               bool
	StatusValid              bool
	RecordIndexChecksValid   bool
	RecordTTL                TTLPTTLValidation
	IndexTTL                 TTLPTTLValidation
	StatusTTL                TTLPTTLValidation
	CleanCommitEligible      bool
	Failure                  RepairFailure
}

type RepairCommitRequest struct {
	Partition                PartitionStateKey
	RegistrationUID          string
	ExpectedRegistryRevision uint64
	WriterFence              WriterFence
	Generation               uint64
	Token                    RepairToken
	ExpectedCount            int
	Proof                    RepairResult
}

func (r RepairCommitRequest) Validate() error {
	if err := r.Partition.Validate(); err != nil {
		return err
	}
	if r.RegistrationUID == "" || r.RegistrationUID != r.Partition.RegistrationUID {
		return fmt.Errorf("repair commit registration identity is inconsistent")
	}
	if r.ExpectedRegistryRevision == 0 || r.Generation == 0 || r.ExpectedCount < 0 {
		return fmt.Errorf("repair commit revision, generation, and count are invalid")
	}
	if err := r.WriterFence.Validate(); err != nil {
		return err
	}
	return r.Token.Validate()
}

func (r RepairResult) ValidateForCommit(request RepairCommitRequest) error {
	if !r.CleanCommitEligible || r.Failure.Code != RepairFailureNone {
		return fmt.Errorf("repair result is not clean-commit eligible")
	}
	if !r.proofIdentityMatches(request) {
		return fmt.Errorf("repair proof identity does not match commit request")
	}
	if !r.proofCompletenessMatches(request) {
		return fmt.Errorf("repair proof is incomplete")
	}
	if err := r.WriterFence.Validate(); err != nil {
		return err
	}
	return nil
}

func (r RepairResult) proofIdentityMatches(request RepairCommitRequest) bool {
	return r.Partition == request.Partition && r.RegistrationUID == request.RegistrationUID &&
		r.ExpectedRegistryRevision == request.ExpectedRegistryRevision &&
		r.ProvenRegistryRevision == request.ExpectedRegistryRevision &&
		r.ProvenGeneration == request.Generation && r.ProvenWriterFence == request.WriterFence &&
		r.Token == request.Token
}

func (r RepairResult) proofCompletenessMatches(request RepairCommitRequest) bool {
	return r.ExpectedCount == request.ExpectedCount && r.ObservedCount == request.ExpectedCount &&
		r.CleanupCount >= 0 && r.SourceComplete && r.RecordValid && r.IndexValid &&
		r.StatusValid && r.RecordIndexChecksValid && r.RecordTTL.Valid &&
		r.IndexTTL.Valid && r.StatusTTL.Valid
}

type RepairCommitResult struct {
	Partition                PartitionStateKey
	RegistrationUID          string
	ExpectedRegistryRevision uint64
	ProvenRegistryRevision   uint64
	WriterFence              WriterFence
	ProvenGeneration         uint64
	ProvenWriterFence        WriterFence
	Token                    RepairToken
	Committed                bool
	CleanCommit              bool
	Failure                  RepairFailure
}

type FencedReadRequest struct {
	Partition                PartitionStateKey
	RegistrationUID          string
	ExpectedRegistryRevision uint64
	WriterFence              WriterFence
	Generation               uint64
	Identity                 ResourceIdentity
}

func (r FencedReadRequest) Validate() error {
	if err := r.Partition.Validate(); err != nil {
		return err
	}
	if r.RegistrationUID == "" || r.RegistrationUID != r.Partition.RegistrationUID {
		return fmt.Errorf("fenced read registration identity is inconsistent")
	}
	if r.ExpectedRegistryRevision == 0 || r.Generation == 0 {
		return fmt.Errorf("fenced read revision and generation are required")
	}
	if err := r.WriterFence.Validate(); err != nil {
		return err
	}
	return r.Identity.Validate()
}

type FencedReadResult struct {
	Record                 ResourceRecord
	State                  ReadState
	ProvenRegistryRevision uint64
	ProvenGeneration       uint64
	ProvenWriterFence      WriterFence
	RecordValid            bool
	IndexValid             bool
	StatusValid            bool
	RecordTTL              TTLPTTLValidation
	IndexTTL               TTLPTTLValidation
	StatusTTL              TTLPTTLValidation
}
