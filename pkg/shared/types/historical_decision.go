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

package types

import (
	"strings"
	"unicode/utf8"

	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

// HistoricalDecisionTextLimit is the per-field bound for model-authored
// historical decision context (BR-KA-016, FedRAMP SI-10). Values beyond the
// bound are unavailable rather than silently truncated.
const HistoricalDecisionTextLimit = 2000

// HistoricalDecisionContext is an allow-listed, non-executable projection of
// a prior AIAnalysis decision. It must never carry parameters, bundles,
// transcripts, secrets, or private chain-of-thought.
type HistoricalDecisionContext struct {
	WorkflowRationale   string               `json:"workflow_rationale,omitempty"`
	RepeatJustification *RepeatJustification `json:"repeat_justification,omitempty"`
}

var historicalDecisionSanitizer = sanitization.NewSanitizer()

// SafeHistoricalDecisionContext returns a bounded, sanitized copy. Invalid,
// blank, non-UTF-8, or oversized values remain unavailable; accepted text is
// never truncated into apparently valid reasoning.
func SafeHistoricalDecisionContext(context HistoricalDecisionContext) HistoricalDecisionContext {
	result := HistoricalDecisionContext{
		WorkflowRationale: BoundedHistoricalDecisionText(context.WorkflowRationale),
	}
	if context.RepeatJustification == nil {
		return result
	}

	justification := &RepeatJustification{
		EvidenceBasis:         BoundedHistoricalDecisionText(context.RepeatJustification.EvidenceBasis),
		PriorOutcomeRelevance: BoundedHistoricalDecisionText(context.RepeatJustification.PriorOutcomeRelevance),
		ExpectedMechanism:     BoundedHistoricalDecisionText(context.RepeatJustification.ExpectedMechanism),
	}
	if justification.EvidenceBasis != "" && justification.PriorOutcomeRelevance != "" && justification.ExpectedMechanism != "" {
		result.RepeatJustification = justification
	}
	return result
}

// BoundedHistoricalDecisionText validates and sanitizes one model-authored
// historical field without manufacturing a value or silently truncating it.
func BoundedHistoricalDecisionText(value string) string {
	if !validHistoricalDecisionText(value) {
		return ""
	}
	clean := historicalDecisionSanitizer.Sanitize(value)
	if !validHistoricalDecisionText(clean) {
		return ""
	}
	return clean
}

func validHistoricalDecisionText(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" &&
		utf8.RuneCountInString(value) <= HistoricalDecisionTextLimit
}
