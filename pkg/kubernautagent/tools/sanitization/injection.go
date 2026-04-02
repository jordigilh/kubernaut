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

package sanitization

import (
	"context"
	"regexp"
	"strings"
)

// InjectionSanitizer strips prompt injection patterns from tool output (I1 per DD-HAPI-019-003).
// Each matching line or segment is removed entirely; surrounding context is preserved.
type InjectionSanitizer struct {
	patterns []*regexp.Regexp
}

// NewInjectionSanitizer creates an I1 sanitizer with the given regex patterns.
func NewInjectionSanitizer(patterns []*regexp.Regexp) *InjectionSanitizer {
	if patterns == nil {
		patterns = DefaultInjectionPatterns()
	}
	return &InjectionSanitizer{patterns: patterns}
}

// defaultInjectionPatterns are the production I1 patterns per DD-HAPI-019-003,
// compiled once at package init to avoid per-call regex compilation overhead.
//
// Pattern categories:
//  1. Imperative instructions ("ignore all previous", "disregard prior", "forget previous")
//  2. Role impersonation ("system:", "assistant:", "user:" at line start)
//  3. Workflow selection injection ("select workflow", "choose workflow", "use workflow")
//  4. JSON response mimicry (blocks containing "workflow_id" + "confidence" keys)
//  5. Closing tag injection (</tool_result>, </system>, </function>)
//  6. Prompt escape sequences (boundary markers: ---/====/####)
var defaultInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?previous\s+instructions`),
	regexp.MustCompile(`(?i)disregard\s+(all\s+)?prior`),
	regexp.MustCompile(`(?i)forget\s+(all\s+)?previous`),
	regexp.MustCompile(`(?i)you\s+are\s+now\b`),
	regexp.MustCompile(`(?im)^(system|assistant|user)\s*:\s`),
	regexp.MustCompile(`(?i)(select|choose|use)\s+workflow\b`),
	regexp.MustCompile(`\{[^}]*"workflow_id"\s*:[^}]*\}`),
	regexp.MustCompile(`(?i)"needs_human_review"\s*:`),
	regexp.MustCompile(`</tool_result>`),
	regexp.MustCompile(`</system>`),
	regexp.MustCompile(`</function>`),
	regexp.MustCompile(`\n\n---\n\n`),
	regexp.MustCompile(`={4,}`),
	regexp.MustCompile(`#{4,}\s+[A-Z]`),
}

// DefaultInjectionPatterns returns the production I1 patterns per DD-HAPI-019-003.
// The returned slice is a copy so callers cannot mutate the package-level patterns.
func DefaultInjectionPatterns() []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(defaultInjectionPatterns))
	copy(out, defaultInjectionPatterns)
	return out
}

// Name implements Stage.
func (s *InjectionSanitizer) Name() string { return "I1" }

// Sanitize implements Stage. Strips segments matching injection patterns from each line.
// Lines that become empty after stripping are removed entirely.
func (s *InjectionSanitizer) Sanitize(_ context.Context, input string) (string, error) {
	result := input
	for _, p := range s.patterns {
		result = p.ReplaceAllString(result, "")
	}

	// Remove lines that became blank after stripping
	lines := strings.Split(result, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n"), nil
}
