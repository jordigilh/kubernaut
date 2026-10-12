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

package investigator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"gopkg.in/yaml.v3"
)

// DD-KA-2496: observations are ephemeral, not raw audit/conversation storage.
// The existing correlation-scoped cleanup loop also reclaims abandoned turns.
type evidenceRuntime struct {
	budget              int
	sanitizer           *sanitization.Pipeline
	additionalSanitizer *sanitization.Pipeline
	mu                  sync.Mutex
	entries             map[string]*evidenceEntry
}

type evidenceEntry struct {
	observations []handoffObservation
	bytes        int
	failure      string
	lastAccess   time.Time
}

type handoffObservation struct {
	ObservedAt time.Time `json:"observed_at"`
	Content    string    `json:"content"`
}

func newEvidenceRuntime(p Pipeline) *evidenceRuntime {
	budget := p.MaxEvidenceHandoffBytes
	if budget <= 0 {
		budget = katypes.DefaultMaxEvidenceHandoffBytes
	}
	// DD-KA-2496: packet safety cannot be disabled by optional tool-output
	// settings. Operator/custom stages add to, never replace, this baseline.
	sanitizer := sanitization.NewPipeline(sanitization.NewSecretSanitizer(),
		sanitization.NewCredentialSanitizer(), sanitization.NewInjectionSanitizer(nil))
	return &evidenceRuntime{budget: budget, sanitizer: sanitizer, additionalSanitizer: p.Sanitizer,
		entries: make(map[string]*evidenceEntry)}
}

func (inv *Investigator) sanitizeEvidenceText(ctx context.Context, text string) (string, error) {
	clean, err := inv.evidence.sanitizer.Run(ctx, text)
	if err != nil {
		return "", err
	}
	if inv.evidence.additionalSanitizer != nil {
		return inv.evidence.additionalSanitizer.Run(ctx, clean)
	}
	return clean, nil
}

func (inv *Investigator) resetEvidence(id string) {
	inv.evidence.mu.Lock()
	defer inv.evidence.mu.Unlock()
	delete(inv.evidence.entries, id)
}

func (inv *Investigator) evidenceEntryLocked(id string) *evidenceEntry {
	entry := inv.evidence.entries[id]
	if entry == nil {
		entry = &evidenceEntry{}
		inv.evidence.entries[id] = entry
	}
	entry.lastAccess = time.Now()
	return entry
}

func (inv *Investigator) failEvidence(id, reason string) {
	if id == "" {
		return // Never share an empty-ID bucket between unrelated callers.
	}
	inv.evidence.mu.Lock()
	defer inv.evidence.mu.Unlock()
	inv.evidenceEntryLocked(id).failure = reason
}

func (inv *Investigator) captureEvidence(ctx context.Context, id, toolName, output string) {
	if id == "" || strings.HasPrefix(toolName, "todo_") ||
		toolName == "get_namespaced_resource_context" || toolName == "get_cluster_resource_context" {
		return // History is consolidated separately for exactly the Phase 2 target.
	}
	// Secret detection runs recursively before removing kind/source metadata.
	// Neither this buffer nor the correlation store ever retains raw output.
	projected, err := projectObservation(ctx, output)
	if err == nil {
		projected, err = inv.sanitizeEvidenceText(ctx, projected)
	}
	if err != nil {
		inv.logger.Error(err, "evidence observation could not be sanitized/projected", "correlation_id", id)
		inv.failEvidence(id, "observation sanitization/projection failed")
		return
	}
	inv.evidence.mu.Lock()
	defer inv.evidence.mu.Unlock()
	entry := inv.evidenceEntryLocked(id)
	if entry.failure != "" {
		return
	}
	if len(projected) > inv.evidence.budget-entry.bytes {
		// Do not retain an unbounded slice or silently hand over a partial
		// packet. The Phase 2 freeze will route the whole request to review.
		entry.failure = "observation budget exceeded"
		entry.observations = nil
		return
	}
	entry.bytes += len(projected)
	entry.observations = append(entry.observations, handoffObservation{ObservedAt: time.Now().UTC(), Content: projected})
}

func (inv *Investigator) evidenceSnapshot(id string) ([]handoffObservation, error) {
	inv.evidence.mu.Lock()
	defer inv.evidence.mu.Unlock()
	entry := inv.evidence.entries[id]
	if entry == nil {
		return []handoffObservation{}, nil
	}
	entry.lastAccess = time.Now()
	if entry.failure != "" {
		return nil, fmt.Errorf("evidence handoff: %s", entry.failure)
	}
	return slices.Clone(entry.observations), nil
}

func (inv *Investigator) pruneEvidence(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	inv.evidence.mu.Lock()
	defer inv.evidence.mu.Unlock()
	removed := 0
	for id, entry := range inv.evidence.entries {
		if entry.lastAccess.Before(cutoff) {
			delete(inv.evidence.entries, id)
			removed++
		}
	}
	return removed
}

// projectObservation removes raw parameters/bundles and duplicated source
// metadata while preserving the full sanitized observation, without a count cap.
func projectObservation(ctx context.Context, output string) (string, error) {
	if !utf8.ValidString(output) {
		return "", fmt.Errorf("observation is not valid UTF-8")
	}
	trimmed := strings.TrimSpace(output)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if !json.Valid([]byte(trimmed)) {
			return "", fmt.Errorf("structured observation is not valid JSON")
		}
		projected, err := projectObservationJSON(ctx, json.RawMessage(trimmed))
		return string(projected), err
	}
	documents, structured, err := projectObservationDocuments(ctx, output)
	if err != nil {
		return "", err
	}
	if !structured {
		return output, nil
	}
	return encodeObservationDocuments(documents)
}

// Preserve complete structured streams. Non-YAML logs return structured=false
// so the caller sanitizes the complete original text, not a decoded prefix.
func projectObservationDocuments(ctx context.Context, output string) ([]yaml.Node, bool, error) {
	decoder := yaml.NewDecoder(strings.NewReader(output))
	documents := make([]yaml.Node, 0, 1)
	structured := false
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// DD-KA-2496: malformed resource/partial streams cannot fall back
			// to raw text; Secret redaction requires the structural context.
			if structured || hasResourceYAMLHeader(output) {
				return nil, false, fmt.Errorf("decode structured observation: %w", err)
			}
			// Ordinary logs/tabular output need not be YAML. Sanitize the
			// complete original text, never just the decoded prefix.
			return nil, false, nil
		}
		if len(document.Content) > 0 && document.Content[0].Kind != yaml.ScalarNode {
			structured = true
		}
		if err := projectObservationYAML(ctx, &document); err != nil {
			return nil, false, err
		}
		documents = append(documents, document)
	}
	return documents, structured, nil
}

func encodeObservationDocuments(documents []yaml.Node) (string, error) {
	var projected bytes.Buffer
	encoder := yaml.NewEncoder(&projected)
	for i := range documents {
		if err := encoder.Encode(&documents[i]); err != nil {
			if closeErr := encoder.Close(); closeErr != nil {
				return "", fmt.Errorf("close failed observation encoder: %w", errors.Join(err, closeErr))
			}
			return "", fmt.Errorf("serialize projected observation: %w", err)
		}
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("close observation encoder: %w", err)
	}
	return projected.String(), nil
}

func hasResourceYAMLHeader(output string) bool {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
		key, _, found := strings.Cut(line, ":")
		if found && slices.Contains([]string{"kind", "apiVersion"}, strings.Trim(key, `"'`)) {
			return true
		}
	}
	return false
}

func omittedObservationKey(key string) bool {
	switch strings.ToLower(key) {
	case "metadata", "parameters", "execution_bundle", "execution_bundle_digest", "engine_config",
		"source_kind", "source_type", "source_ref", "source_resource", "apiversion", "kind":
		return true
	default:
		return false
	}
}

func projectObservationJSON(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty structured observation")
	}
	switch strings.TrimSpace(string(raw))[0] {
	case '{':
		return projectObservationObject(ctx, raw)
	case '[':
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, fmt.Errorf("decode observation array: %w", err)
		}
		for i, value := range values {
			projected, err := projectObservationJSON(ctx, value)
			if err != nil {
				return nil, err
			}
			values[i] = projected
		}
		return json.Marshal(values)
	default:
		return raw, nil
	}
}

func projectObservationObject(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	secretSafe, err := sanitization.NewSecretSanitizer().Sanitize(ctx, string(raw))
	if err != nil {
		return nil, fmt.Errorf("redact observation Secret: %w", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(secretSafe), &object); err != nil {
		return nil, fmt.Errorf("decode observation object: %w", err)
	}
	for key, value := range object {
		if omittedObservationKey(key) {
			delete(object, key)
			continue
		}
		projected, err := projectObservationJSON(ctx, value)
		if err != nil {
			return nil, err
		}
		object[key] = projected
	}
	return json.Marshal(object)
}

func projectObservationYAML(ctx context.Context, node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		return projectObservationYAMLMapping(ctx, node)
	}
	for _, child := range node.Content {
		if err := projectObservationYAML(ctx, child); err != nil {
			return err
		}
	}
	return nil
}

func redactObservationYAML(ctx context.Context, node *yaml.Node) error {
	raw, err := yaml.Marshal(node)
	if err != nil {
		return fmt.Errorf("serialize observation resource: %w", err)
	}
	secretSafe, err := sanitization.NewSecretSanitizer().Sanitize(ctx, string(raw))
	if err != nil {
		return fmt.Errorf("redact observation Secret: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(secretSafe), &document); err != nil {
		return fmt.Errorf("decode Secret-safe observation resource: %w", err)
	}
	if len(document.Content) != 1 {
		return fmt.Errorf("secret-safe observation resource is missing")
	}
	*node = *document.Content[0]
	return nil
}

func projectObservationYAMLMapping(ctx context.Context, node *yaml.Node) error {
	if err := redactObservationYAML(ctx, node); err != nil {
		return err
	}
	kept := make([]*yaml.Node, 0, len(node.Content))
	for i := 0; i+1 < len(node.Content); i += 2 {
		if omittedObservationKey(node.Content[i].Value) {
			continue
		}
		if err := projectObservationYAML(ctx, node.Content[i+1]); err != nil {
			return err
		}
		kept = append(kept, node.Content[i], node.Content[i+1])
	}
	node.Content = kept
	return nil
}
