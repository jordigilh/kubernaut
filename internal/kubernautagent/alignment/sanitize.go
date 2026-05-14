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

package alignment

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxExplanationLen      = 1024
	explanationTruncMarker = "...[truncated]"
)

// SanitizeExplanation removes control characters and truncates the explanation
// to prevent log injection and unbounded storage from shadow LLM output.
// Truncation is rune-aware to avoid splitting multi-byte UTF-8 sequences.
func SanitizeExplanation(s string) string {
	if s == "" {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	cleaned := b.String()

	if utf8.RuneCountInString(cleaned) > maxExplanationLen {
		runes := []rune(cleaned)
		return string(runes[:maxExplanationLen]) + explanationTruncMarker
	}
	return cleaned
}
