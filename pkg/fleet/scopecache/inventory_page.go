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
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// DD-FLEET-017 D2.5a: the per-partition lexicographic identity index orders
// and filters a committed generation without enumerating it. The member is
// "namespace \x00 kind \x00 group \x00 version \x00 name \x00 publicID";
// none of those values can contain NUL, so byte order is unambiguous. The
// cluster is omitted because it is fixed per partition.
const lexFieldSeparator = "\x00"

// scriptMissing is the atomic script's fail-closed reply token.
const scriptMissing = "missing"

// errInvalidRecordLabel reports a record whose stored label state is outside
// the bounded enum. It fails the page closed like any other unprovable record.
var errInvalidRecordLabel = errors.New("inventory record label state is invalid")

// lexicalMember composes one lexicographic index member.
func lexicalMember(identity ResourceIdentity, publicID string) string {
	return strings.Join([]string{
		identity.Namespace,
		identity.Kind,
		identity.Group,
		identity.Version,
		identity.Name,
		publicID,
	}, lexFieldSeparator)
}

// LexicalMember composes the identity-index member of one record. Readers that
// already know a record's identity (an exact read) use it to request the same
// page-shaped payload the list route uses, including the namespace records
// needed for inheritance.
func LexicalMember(identity ResourceIdentity, publicID string) string {
	return lexicalMember(identity, publicID)
}

// InventoryPrefix is the widest identity filter that can be served as a
// lexicographic range scan. Only a leading run of separator-terminated fields
// can be encoded: a kind-only or name-only query without a namespace cannot,
// and instead scans the partition and filters in the page reader (DD-FLEET-017
// D2.5a — documented, not hidden).
type InventoryPrefix struct {
	Namespace string
	Kind      string
	Group     string
	Version   string
	Name      string
}

// RangeScan reports whether the filter can be expressed as a lexicographic
// prefix. When false the page reader must scan the partition and filter.
func (p InventoryPrefix) RangeScan() bool {
	return p.Namespace != ""
}

// key returns the encoded prefix. Empty means "from the start of the
// partition".
func (p InventoryPrefix) key() string {
	if p.Namespace == "" {
		return ""
	}
	fields := []string{p.Namespace}
	if p.Kind == "" {
		return strings.Join(fields, lexFieldSeparator) + lexFieldSeparator
	}
	fields = append(fields, p.Kind)
	// group and version are only encodable as a pair; a lone group or version
	// stays a read-time filter.
	if p.Group != "" && p.Version != "" {
		fields = append(fields, p.Group, p.Version)
		if p.Name != "" {
			fields = append(fields, p.Name)
		}
	}
	return strings.Join(fields, lexFieldSeparator) + lexFieldSeparator
}

// Matches applies the residual filters that the encoded prefix cannot carry.
func (p InventoryPrefix) Matches(identity ResourceIdentity) bool {
	return (p.Namespace == "" || p.Namespace == identity.Namespace) &&
		(p.Kind == "" || p.Kind == identity.Kind) &&
		(p.Group == "" || p.Group == identity.Group) &&
		(p.Version == "" || p.Version == identity.Version) &&
		(p.Name == "" || p.Name == identity.Name)
}

// InventoryReadState is the outcome of one proof-gated inventory read. It is
// deliberately smaller than the wire contract: page assembly, not the store,
// decides success/empty/partial/unavailable.
type InventoryReadState string

const (
	// InventoryReadOK reports a committed generation that carries members.
	InventoryReadOK InventoryReadState = "ok"
	// InventoryReadEmpty reports a proven-empty partition
	// (empty_verified=1 and observed_count=0). A proven-empty partition has
	// no indexes at all, so this state is derived from the proof only.
	InventoryReadEmpty InventoryReadState = "empty"
	// InventoryReadMissing reports an identity that is not present in the
	// committed generation. On the wire that is "not found", never an outage.
	InventoryReadMissing InventoryReadState = "missing"
	// InventoryReadUnproven reports that the partition has no servable
	// committed proof: expired, cleaned away, staging-only, or inconsistent.
	// Every reason maps to "unavailable" (state-only) on the wire.
	InventoryReadUnproven InventoryReadState = "unproven"
	// InventoryReadDrift reports a cursor whose identity is gone from the
	// committed generation. The page reader re-runs the projection from the
	// start; it is never caller error.
	InventoryReadDrift InventoryReadState = "drift"
)

// InventoryPageRequest is one fenced page request over the committed
// generation's lexicographic identity index.
type InventoryPageRequest struct {
	Partition                PartitionStateKey
	RegistrationUID          string
	ExpectedRegistryRevision uint64
	WriterFence              WriterFence
	Generation               uint64
	// Prefix narrows the lexicographic range; the page reader applies the
	// residual filters itself.
	Prefix InventoryPrefix
	// Cursor is the canonical resource ID of the last item returned by a
	// previous page. Empty requests the first page.
	Cursor string
	// Limit is bounded by inventory.MaxLimit.
	Limit int
}

// Validate rejects an incomplete fence or an unbounded page request.
func (r InventoryPageRequest) Validate() error {
	if err := r.Partition.Validate(); err != nil {
		return err
	}
	if r.RegistrationUID == "" || r.RegistrationUID != r.Partition.RegistrationUID {
		return fmt.Errorf("inventory page registration identity is inconsistent")
	}
	if r.ExpectedRegistryRevision == 0 || r.Generation == 0 {
		return fmt.Errorf("inventory page revision and generation are required")
	}
	if err := r.WriterFence.Validate(); err != nil {
		return err
	}
	// The console limit is bounded by inventory.MaxLimit. The page reader
	// legitimately asks for one extra member to learn whether another page
	// exists, so MaxLimit+1 is the internal ceiling; anything above it is a
	// caller bug, not a probe.
	if r.Limit < 1 || r.Limit > inventory.MaxLimit+1 {
		return fmt.Errorf("inventory page limit is out of bounds")
	}
	if r.Cursor != "" {
		if err := inventory.ValidateResourceID(r.Cursor); err != nil {
			return err
		}
	}
	return nil
}

// InventoryMember is one identity of the committed generation's lexicographic
// page. Member is the raw index member; Cursor is its canonical resource ID.
type InventoryMember struct {
	Member     string
	PublicID   string
	Identity   ResourceIdentity
	Generation uint64
}

// InventoryPageHeader is the proof-validated identity page of one committed
// generation. It carries no records.
type InventoryPageHeader struct {
	State           InventoryReadState
	Reason          string
	Members         []InventoryMember
	Generation      uint64
	Fence           uint64
	ServerNowMillis int64
	StatusTTL       TTLPTTLValidation
	// RemainingProofTTL is the remaining life of the committed proof (the
	// status hash), which outlives the record and index TTLs. Page freshness is
	// derived from it (DD-FLEET-017 D3.2a); record readability is enforced
	// separately and fails closed on its own TTL.
	RemainingProofTTL time.Duration
}

// InventoryPageRecordsRequest reads the records of one already-proven page.
// Namespaces lists the distinct namespaces whose records the page needs for
// ADR-053 inheritance; they ride in the same call so a page never becomes an
// N+1 read.
type InventoryPageRecordsRequest struct {
	Partition                PartitionStateKey
	RegistrationUID          string
	ExpectedRegistryRevision uint64
	WriterFence              WriterFence
	Generation               uint64
	Members                  []string
	Namespaces               []string
}

// Validate rejects an incomplete fence or an unbounded record request.
func (r InventoryPageRecordsRequest) Validate() error {
	if err := r.Partition.Validate(); err != nil {
		return err
	}
	if r.RegistrationUID == "" || r.RegistrationUID != r.Partition.RegistrationUID {
		return fmt.Errorf("inventory records registration identity is inconsistent")
	}
	if r.ExpectedRegistryRevision == 0 || r.Generation == 0 {
		return fmt.Errorf("inventory records revision and generation are required")
	}
	if err := r.WriterFence.Validate(); err != nil {
		return err
	}
	// Members may carry the page plus its one-member look-ahead probe; the
	// namespace list stays bounded by the console limit.
	if len(r.Members) > inventory.MaxLimit+1 {
		return fmt.Errorf("inventory records page is out of bounds")
	}
	if len(r.Namespaces) > inventory.MaxLimit {
		return fmt.Errorf("inventory records namespace fan-out is out of bounds")
	}
	return nil
}

// InventoryRecord is one committed record of a proven generation.
type InventoryRecord struct {
	Identity        ResourceIdentity
	LabelState      scope.LabelState
	Generation      uint64
	ExpiresAtMillis int64
}

// InventoryNamespaceLabel is the inherited label state of one namespace.
// A namespace whose record is absent from the generation is reported as not
// present so the page reader can fail closed instead of treating the resource
// as unmanaged.
type InventoryNamespaceLabel struct {
	Present    bool
	LabelState scope.LabelState
}

// InventoryPageRecords is the record payload of one proven page.
type InventoryPageRecords struct {
	State             InventoryReadState
	Reason            string
	Records           []InventoryRecord
	NamespaceLabels   map[string]InventoryNamespaceLabel
	Generation        uint64
	Fence             uint64
	ServerNowMillis   int64
	RemainingProofTTL time.Duration
}

// InventoryResourceRequest is one exact, proof-gated read by canonical ID.
type InventoryResourceRequest struct {
	Partition                PartitionStateKey
	RegistrationUID          string
	ExpectedRegistryRevision uint64
	WriterFence              WriterFence
	Generation               uint64
	PublicID                 string
}

// Validate rejects an incomplete fence or a malformed canonical ID.
func (r InventoryResourceRequest) Validate() error {
	if err := r.Partition.Validate(); err != nil {
		return err
	}
	if r.RegistrationUID == "" || r.RegistrationUID != r.Partition.RegistrationUID {
		return fmt.Errorf("inventory exact read registration identity is inconsistent")
	}
	if r.ExpectedRegistryRevision == 0 || r.Generation == 0 {
		return fmt.Errorf("inventory exact read revision and generation are required")
	}
	if err := r.WriterFence.Validate(); err != nil {
		return err
	}
	return inventory.ValidateResourceID(r.PublicID)
}

// InventoryResourceRecord is the result of one exact read.
type InventoryResourceRecord struct {
	State             InventoryReadState
	Reason            string
	Record            InventoryRecord
	Generation        uint64
	Fence             uint64
	ServerNowMillis   int64
	RemainingProofTTL time.Duration
}

// ReadInventoryPageHeader validates the committed proof once and returns the
// lexicographic identity page of that generation.
func (s *ValkeyStatusStore) ReadInventoryPageHeader(ctx context.Context, request InventoryPageRequest) (InventoryPageHeader, error) {
	result := InventoryPageHeader{State: InventoryReadMissing}
	if err := request.Validate(); err != nil {
		return result, err
	}
	items, err := s.evalMutation(ctx,
		[]string{statusKey(request.Partition), lexIndexKey(request.Partition), recordKey(request.Partition, request.Cursor)},
		"inventory_page_header", request.RegistrationUID, request.ExpectedRegistryRevision,
		request.WriterFence.HolderPodUID, request.WriterFence.HolderSessionUID,
		request.WriterFence.LeaseEpoch, request.WriterFence.Fence, request.Generation,
		request.Prefix.key(), request.Limit, request.Cursor)
	if err != nil {
		return result, classifyRepairError(err)
	}
	return decodeInventoryHeader(items, request)
}

// ReadInventoryPageRecords re-validates the same fence and generation and
// returns only the requested page's records.
func (s *ValkeyStatusStore) ReadInventoryPageRecords(ctx context.Context, request InventoryPageRecordsRequest) (InventoryPageRecords, error) {
	result := InventoryPageRecords{State: InventoryReadMissing}
	if err := request.Validate(); err != nil {
		return result, err
	}
	keys := make([]string, 0, 2+len(request.Members)+len(request.Namespaces))
	keys = append(keys, statusKey(request.Partition), lexIndexKey(request.Partition))
	for _, member := range request.Members {
		keys = append(keys, recordKey(request.Partition, memberPublicID(member)))
	}
	for _, namespace := range request.Namespaces {
		keys = append(keys, recordKey(request.Partition, namespaceRecordID(request.Partition, namespace)))
	}
	args := make([]interface{}, 0, 8+1+len(request.Members)+1+len(request.Namespaces))
	args = append(args, "inventory_page_records", request.RegistrationUID, request.ExpectedRegistryRevision,
		request.WriterFence.HolderPodUID, request.WriterFence.HolderSessionUID,
		request.WriterFence.LeaseEpoch, request.WriterFence.Fence, request.Generation,
		len(request.Members))
	for _, member := range request.Members {
		args = append(args, member)
	}
	args = append(args, len(request.Namespaces))
	for _, namespace := range request.Namespaces {
		args = append(args, namespace)
	}
	items, err := s.evalMutation(ctx, keys, args...)
	if err != nil {
		return result, classifyRepairError(err)
	}
	return decodeInventoryRecords(items, request)
}

// ReadInventoryResource reads one committed record by canonical ID.
func (s *ValkeyStatusStore) ReadInventoryResource(ctx context.Context, request InventoryResourceRequest) (InventoryResourceRecord, error) {
	result := InventoryResourceRecord{State: InventoryReadMissing}
	if err := request.Validate(); err != nil {
		return result, err
	}
	items, err := s.evalMutation(ctx,
		[]string{statusKey(request.Partition), recordKey(request.Partition, request.PublicID)},
		"inventory_exact", request.RegistrationUID, request.ExpectedRegistryRevision,
		request.WriterFence.HolderPodUID, request.WriterFence.HolderSessionUID,
		request.WriterFence.LeaseEpoch, request.WriterFence.Fence, request.Generation,
		request.PublicID)
	if err != nil {
		return result, classifyRepairError(err)
	}
	return decodeInventoryExact(items, request)
}

func decodeInventoryHeader(items []interface{}, request InventoryPageRequest) (InventoryPageHeader, error) {
	result := InventoryPageHeader{State: InventoryReadMissing, Reason: scriptString(items, 1)}
	switch scriptString(items, 0) {
	case scriptMissing:
		return result, nil
	case "drift":
		result.State = InventoryReadDrift
		return result, nil
	case "empty":
		return decodeInventoryEmptyHeader(items)
	case "ok":
	default:
		return result, fmt.Errorf("invalid inventory page header response")
	}
	if len(items) < 7 {
		return result, fmt.Errorf("short inventory page header response")
	}
	now, err := scriptInt(items, 1)
	if err != nil {
		return result, fmt.Errorf("invalid inventory header server time: %w", err)
	}
	statusExpires, err := scriptInt(items, 2)
	if err != nil {
		return result, fmt.Errorf("invalid inventory header status expiry: %w", err)
	}
	statusPTTL, err := scriptInt(items, 3)
	if err != nil {
		return result, fmt.Errorf("invalid inventory header status pttl: %w", err)
	}
	generation, err := scriptInt(items, 4)
	if err != nil {
		return result, fmt.Errorf("invalid inventory header generation: %w", err)
	}
	fence, err := scriptInt(items, 5)
	if err != nil {
		return result, fmt.Errorf("invalid inventory header fence: %w", err)
	}
	remaining, err := scriptInt(items, 6)
	if err != nil {
		return result, fmt.Errorf("invalid inventory header remaining proof: %w", err)
	}
	result.State = InventoryReadOK
	result.Reason = ""
	result.ServerNowMillis = now
	result.Generation = uint64(generation)
	result.Fence = uint64(fence)
	result.StatusTTL = ttlValidation(now, statusExpires, statusPTTL)
	result.RemainingProofTTL = remainingMillis(remaining)
	if !result.StatusTTL.Valid {
		return InventoryPageHeader{State: InventoryReadMissing, Reason: "status_ttl"}, nil
	}
	members := make([]InventoryMember, 0, len(items)-7)
	for index := 7; index < len(items); index++ {
		member, err := parseInventoryMember(scriptString(items, index), request.Partition.ClusterID, uint64(generation))
		if err != nil {
			return InventoryPageHeader{State: InventoryReadMissing, Reason: "invalid_member"}, err
		}
		members = append(members, member)
	}
	result.Members = members
	return result, nil
}

func decodeInventoryEmptyHeader(items []interface{}) (InventoryPageHeader, error) {
	if len(items) < 7 {
		return InventoryPageHeader{State: InventoryReadMissing}, fmt.Errorf("short inventory empty header response")
	}
	now, err := scriptInt(items, 1)
	if err != nil {
		return InventoryPageHeader{State: InventoryReadMissing}, fmt.Errorf("invalid inventory empty server time: %w", err)
	}
	statusExpires, err := scriptInt(items, 2)
	if err != nil {
		return InventoryPageHeader{State: InventoryReadMissing}, fmt.Errorf("invalid inventory empty status expiry: %w", err)
	}
	statusPTTL, err := scriptInt(items, 3)
	if err != nil {
		return InventoryPageHeader{State: InventoryReadMissing}, fmt.Errorf("invalid inventory empty status pttl: %w", err)
	}
	generation, err := scriptInt(items, 4)
	if err != nil {
		return InventoryPageHeader{State: InventoryReadMissing}, fmt.Errorf("invalid inventory empty generation: %w", err)
	}
	fence, err := scriptInt(items, 5)
	if err != nil {
		return InventoryPageHeader{State: InventoryReadMissing}, fmt.Errorf("invalid inventory empty fence: %w", err)
	}
	remaining, err := scriptInt(items, 6)
	if err != nil {
		return InventoryPageHeader{State: InventoryReadMissing}, fmt.Errorf("invalid inventory empty remaining proof: %w", err)
	}
	statusTTL := ttlValidation(now, statusExpires, statusPTTL)
	if !statusTTL.Valid {
		return InventoryPageHeader{State: InventoryReadMissing, Reason: "status_ttl"}, nil
	}
	return InventoryPageHeader{
		State:             InventoryReadEmpty,
		Generation:        uint64(generation),
		Fence:             uint64(fence),
		ServerNowMillis:   now,
		StatusTTL:         statusTTL,
		RemainingProofTTL: remainingMillis(remaining),
		Members:           nil,
	}, nil
}

func decodeInventoryRecords(items []interface{}, request InventoryPageRecordsRequest) (InventoryPageRecords, error) {
	result := InventoryPageRecords{State: InventoryReadMissing, Reason: scriptString(items, 1)}
	switch scriptString(items, 0) {
	case scriptMissing:
		return result, nil
	case "ok":
	default:
		return result, fmt.Errorf("invalid inventory page records response")
	}
	if len(items) < 7 {
		return result, fmt.Errorf("short inventory page records response")
	}
	now, err := scriptInt(items, 1)
	if err != nil {
		return result, fmt.Errorf("invalid inventory records server time: %w", err)
	}
	generation, err := scriptInt(items, 2)
	if err != nil {
		return result, fmt.Errorf("invalid inventory records generation: %w", err)
	}
	fence, err := scriptInt(items, 3)
	if err != nil {
		return result, fmt.Errorf("invalid inventory records fence: %w", err)
	}
	remaining, err := scriptInt(items, 4)
	if err != nil {
		return result, fmt.Errorf("invalid inventory records remaining proof: %w", err)
	}
	count, err := scriptInt(items, 5)
	if err != nil {
		return result, fmt.Errorf("invalid inventory records count: %w", err)
	}
	const recordFields = 8
	offset := 6
	if int64(len(items)) < int64(offset)+count*recordFields+1 {
		return result, fmt.Errorf("short inventory page records payload")
	}
	result.State = InventoryReadOK
	result.Reason = ""
	result.ServerNowMillis = now
	result.Generation = uint64(generation)
	result.Fence = uint64(fence)
	result.RemainingProofTTL = remainingMillis(remaining)
	result.Records = make([]InventoryRecord, 0, count)
	for index := int64(0); index < count; index++ {
		record, err := decodeInventoryRecord(items, offset+int(index)*recordFields, request.Partition.ClusterID, uint64(generation))
		if err != nil {
			return result, err
		}
		if errors.Is(err, errInvalidRecordLabel) {
			return InventoryPageRecords{State: InventoryReadMissing, Reason: "invalid_label"}, nil
		}
		result.Records = append(result.Records, *record)
	}
	labels, err := decodeNamespaceLabels(items, offset+int(count)*recordFields)
	if err != nil {
		return result, err
	}
	result.NamespaceLabels = labels
	return result, nil
}

// decodeInventoryRecord decodes one record of the page payload and rejects any
// record whose stored identity does not hash to its own public ID.
func decodeInventoryRecord(items []interface{}, base int, clusterID string, generation uint64) (*InventoryRecord, error) {
	labelState := scope.LabelState(scriptString(items, base+6))
	expires, err := scriptInt(items, base+7)
	if err != nil {
		return nil, fmt.Errorf("invalid inventory record expiry: %w", err)
	}
	identity := ResourceIdentity{
		ClusterID: clusterID,
		Group:     scriptString(items, base+1),
		Version:   scriptString(items, base+2),
		Kind:      scriptString(items, base+3),
		Namespace: scriptString(items, base+4),
		Name:      scriptString(items, base+5),
	}
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	if !validRecordLabelState(labelState) {
		return nil, errInvalidRecordLabel
	}
	publicID := scriptString(items, base)
	canonical, canonicalErr := canonicalResourceID(identity)
	if canonicalErr != nil || canonical != publicID {
		return nil, fmt.Errorf("inventory record identity does not match its canonical ID")
	}
	return &InventoryRecord{
		Identity:        identity,
		LabelState:      labelState,
		Generation:      generation,
		ExpiresAtMillis: expires,
	}, nil
}

// decodeNamespaceLabels reads the inherited-label payload that rides with the
// page so a page is never an N+1 read.
func decodeNamespaceLabels(items []interface{}, offset int) (map[string]InventoryNamespaceLabel, error) {
	count, err := scriptInt(items, offset)
	if err != nil {
		return nil, fmt.Errorf("invalid inventory namespace count: %w", err)
	}
	if int64(len(items)) < int64(offset)+1+count*3 {
		return nil, fmt.Errorf("short inventory namespace payload")
	}
	labels := make(map[string]InventoryNamespaceLabel, count)
	for index := int64(0); index < count; index++ {
		base := offset + 1 + int(index)*3
		namespace := scriptString(items, base)
		present := scriptString(items, base+1) == "1"
		labelState := scope.LabelState(scriptString(items, base+2))
		if present && !validRecordLabelState(labelState) {
			return nil, fmt.Errorf("invalid inventory namespace label state")
		}
		labels[namespace] = InventoryNamespaceLabel{Present: present, LabelState: labelState}
	}
	return labels, nil
}

// inventoryExactNumbers carries the numeric fields of an exact-read reply.
type inventoryExactNumbers struct {
	now        int64
	generation int64
	fence      int64
	remaining  int64
	expires    int64
}

// decodeInventoryExactNumbers converts the numeric reply fields, keeping the
// per-field error messages that the exact-read contract reports.
func decodeInventoryExactNumbers(items []interface{}) (inventoryExactNumbers, error) {
	var numbers inventoryExactNumbers
	fields := []struct {
		index int
		name  string
		dest  *int64
	}{
		{1, "server time", &numbers.now},
		{2, "generation", &numbers.generation},
		{3, "fence", &numbers.fence},
		{4, "remaining proof", &numbers.remaining},
		{12, "record expiry", &numbers.expires},
	}
	for _, field := range fields {
		value, err := scriptInt(items, field.index)
		if err != nil {
			return inventoryExactNumbers{}, fmt.Errorf("invalid inventory exact %s: %w", field.name, err)
		}
		*field.dest = value
	}
	return numbers, nil
}

func decodeInventoryExact(items []interface{}, request InventoryResourceRequest) (InventoryResourceRecord, error) {
	result := InventoryResourceRecord{State: InventoryReadMissing, Reason: scriptString(items, 1)}
	switch scriptString(items, 0) {
	case scriptMissing:
		// A record-level miss is still answered under the valid proof, so it
		// carries that proof's remaining life for page assembly.
		if len(items) > 2 {
			if remaining, err := scriptInt(items, 2); err == nil {
				result.RemainingProofTTL = remainingMillis(remaining)
			}
		}
		return result, nil
	case "unproven":
		result.State = InventoryReadUnproven
		return result, nil
	case "ok":
	default:
		return result, fmt.Errorf("invalid inventory exact read response")
	}
	const fields = 8
	if len(items) < fields+5 {
		return result, fmt.Errorf("short inventory exact read response")
	}
	numbers, err := decodeInventoryExactNumbers(items)
	if err != nil {
		return result, err
	}
	publicID := scriptString(items, 5)
	labelState := scope.LabelState(scriptString(items, 11))
	now, generation, fence := numbers.now, numbers.generation, numbers.fence
	remaining, expires := numbers.remaining, numbers.expires
	if publicID != request.PublicID {
		return InventoryResourceRecord{State: InventoryReadMissing, Reason: "record_identity_mismatch"}, nil
	}
	if !validRecordLabelState(labelState) {
		return InventoryResourceRecord{State: InventoryReadMissing, Reason: "invalid_label"}, nil
	}
	identity := ResourceIdentity{
		ClusterID: request.Partition.ClusterID,
		Group:     scriptString(items, 6),
		Version:   scriptString(items, 7),
		Kind:      scriptString(items, 8),
		Namespace: scriptString(items, 9),
		Name:      scriptString(items, 10),
	}
	if err := identity.Validate(); err != nil {
		return InventoryResourceRecord{State: InventoryReadMissing, Reason: "invalid_record"}, err
	}
	canonical, canonicalErr := canonicalResourceID(identity)
	if canonicalErr != nil || canonical != publicID {
		return InventoryResourceRecord{State: InventoryReadMissing, Reason: "record_identity_mismatch"},
			fmt.Errorf("inventory record identity does not match its canonical ID")
	}
	return InventoryResourceRecord{
		State:             InventoryReadOK,
		Record:            InventoryRecord{Identity: identity, LabelState: labelState, Generation: uint64(generation), ExpiresAtMillis: expires},
		Generation:        uint64(generation),
		Fence:             uint64(fence),
		ServerNowMillis:   now,
		RemainingProofTTL: remainingMillis(remaining),
	}, nil
}

func parseInventoryMember(member, clusterID string, generation uint64) (InventoryMember, error) {
	fields := strings.Split(member, lexFieldSeparator)
	if len(fields) != 6 {
		return InventoryMember{}, fmt.Errorf("inventory lexicographic member has %d fields", len(fields))
	}
	identity := ResourceIdentity{
		ClusterID: clusterID,
		Namespace: fields[0],
		Kind:      fields[1],
		Group:     fields[2],
		Version:   fields[3],
		Name:      fields[4],
	}
	return InventoryMember{
		Member:     member,
		PublicID:   fields[5],
		Identity:   identity,
		Generation: generation,
	}, nil
}

func memberPublicID(member string) string {
	if index := strings.LastIndex(member, lexFieldSeparator); index >= 0 {
		return member[index+len(lexFieldSeparator):]
	}
	return ""
}

// namespaceRecordID is the canonical ID of the Namespace record that carries
// the inherited label state for one namespace in this partition.
func namespaceRecordID(partition PartitionStateKey, namespace string) string {
	return inventory.CanonicalResourceID(scope.ResourceIdentity{
		ClusterID: partition.ClusterID,
		Version:   "v1",
		Kind:      "Namespace",
		Name:      namespace,
	})
}

func validRecordLabelState(state scope.LabelState) bool {
	return state == scope.LabelStateTrue || state == scope.LabelStateFalse || state == scope.LabelStateAbsent
}

func remainingMillis(millis int64) time.Duration {
	if millis < 0 {
		millis = 0
	}
	return time.Duration(millis) * time.Millisecond
}

// minRemainingMillis is the minimum remaining life across the supplied
// expiries, sampled against one server time.
func minRemainingMillis(now int64, expiries ...int64) int64 {
	if len(expiries) == 0 {
		return 0
	}
	remaining := expiries[0] - now
	for _, expires := range expiries[1:] {
		if expires-now < remaining {
			remaining = expires - now
		}
	}
	return remaining
}
