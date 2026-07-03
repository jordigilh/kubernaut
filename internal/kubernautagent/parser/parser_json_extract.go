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

package parser

import (
	"encoding/json"
	"strings"
)

// extractJSON finds JSON content using a priority chain:
// 1. Fenced ```json ... ``` code blocks
// 2. Fenced ``` ... ``` code blocks containing JSON
// 3. Raw string starting with {
// 4. Balanced brace extraction (GAP-003: handles JSON embedded in prose)
func extractJSON(content string) string {
	if idx := strings.Index(content, "```json"); idx != -1 {
		start := idx + len("```json")
		end := strings.Index(content[start:], "```")
		if end != -1 {
			return strings.TrimSpace(content[start : start+end])
		}
	}
	if idx := strings.Index(content, "```"); idx != -1 {
		start := idx + len("```")
		end := strings.Index(content[start:], "```")
		if end != -1 {
			candidate := strings.TrimSpace(content[start : start+end])
			if len(candidate) > 0 && candidate[0] == '{' {
				return candidate
			}
		}
	}
	trimmed := strings.TrimSpace(content)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed
	}
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return unwrapSingleElementArray(trimmed)
	}
	return extractBalancedJSON(content)
}

// unwrapSingleElementArray handles LLMs that wrap their response in a JSON
// array (e.g. `[{"rca_summary":...}]`). Single-element arrays are unwrapped
// to the inner object; multi-element arrays are rejected as ambiguous.
func unwrapSingleElementArray(s string) string {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return ""
	}
	if len(arr) != 1 {
		return ""
	}
	elem := strings.TrimSpace(string(arr[0]))
	if len(elem) > 0 && elem[0] == '{' {
		return elem
	}
	return ""
}

// extractBalancedJSON finds the first complete JSON object in content
// by counting balanced braces. Handles JSON embedded in prose text,
// mirroring KA's json_utils.py balanced extraction.
//
// M3-fix: skip `{` chars that are likely prose (not followed by `"`, `\n`, or
// another `{`). Real JSON objects start with `{"` or `{\n`.
func extractBalancedJSON(content string) string {
	pos := 0
	for pos < len(content) {
		start := strings.IndexByte(content[pos:], '{')
		if start == -1 {
			return ""
		}
		start += pos

		if !looksLikeJSONObjectStart(content, start) {
			pos = start + 1
			continue
		}

		if end := scanToBalancedClose(content, start); end != -1 {
			return content[start:end]
		}
		pos = start + 1
	}
	return ""
}

// looksLikeJSONObjectStart reports whether the '{' at content[start] is
// plausibly the start of a JSON object, rather than a stray brace embedded
// in prose (e.g. "the {foo} placeholder"). A real JSON object's opening
// brace is followed by a quote, whitespace, or another brace.
func looksLikeJSONObjectStart(content string, start int) bool {
	if start+1 >= len(content) {
		return true
	}
	switch content[start+1] {
	case '"', '\n', '\r', ' ', '\t', '{', '}':
		return true
	default:
		return false
	}
}

// scanToBalancedClose scans content starting at the opening brace index
// start, tracking string-literal state and escape sequences, and returns the
// index one past the matching closing brace (i.e. a valid content[start:end]
// slice bound). Returns -1 if the braces never balance before EOF.
func scanToBalancedClose(content string, start int) int {
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(content); i++ {
		ch := content[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}
