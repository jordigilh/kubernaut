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

package boundary

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	tokenBytes  = 16 // 16 bytes → 32 hex chars → 2^128 entropy
	openPrefix  = "<<<EVAL_"
	openSuffix  = ">>>"
	closePrefix = "<<<END_EVAL_"
	closeSuffix = ">>>"
)

// Generate returns a crypto-random hex boundary token (16 bytes = 32 hex chars).
func Generate() string {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("boundary: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

// Wrap wraps content in random boundary markers.
// Format: <<<EVAL_{token}>>>\n{content}\n<<<END_EVAL_{token}>>>
func Wrap(content, token string) string {
	open := openPrefix + token + openSuffix
	close := closePrefix + token + closeSuffix
	return open + "\n" + content + "\n" + close
}

// ContainsEscape checks if content contains the closing boundary marker for the
// given token. Returns true if an escape attempt is detected.
func ContainsEscape(content, token string) bool {
	marker := closePrefix + token + closeSuffix
	return strings.Contains(content, marker)
}

// WrapOrFlag generates a boundary, checks for escape, and wraps if safe.
// Returns (wrapped, token, escaped). If escaped is true, wrapped is empty.
func WrapOrFlag(content string) (wrapped string, token string, escaped bool) {
	token = Generate()
	if ContainsEscape(content, token) {
		return "", token, true
	}
	return Wrap(content, token), token, false
}
