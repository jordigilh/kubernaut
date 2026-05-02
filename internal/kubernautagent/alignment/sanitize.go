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
)

const (
	maxExplanationLen      = 1024
	explanationTruncMarker = "...[truncated]"
)

// SanitizeExplanation removes control characters and truncates the explanation
// to prevent log injection and unbounded storage from shadow LLM output.
func SanitizeExplanation(s string) string {
	if s == "" {
		return s
	}

	growLen := len(s)
	cap := maxExplanationLen + len(explanationTruncMarker)
	if growLen > cap {
		growLen = cap
	}

	var b strings.Builder
	b.Grow(growLen)
	for _, r := range s {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
		if b.Len() >= maxExplanationLen {
			return b.String()[:maxExplanationLen] + explanationTruncMarker
		}
	}
	return b.String()
}
