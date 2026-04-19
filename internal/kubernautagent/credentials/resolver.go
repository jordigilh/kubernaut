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

package credentials

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var providerKeyFiles = map[string]string{
	"openai":      "OPENAI_API_KEY",
	"anthropic":   "ANTHROPIC_API_KEY",
	"mistral":     "MISTRAL_API_KEY",
	"huggingface": "HUGGINGFACEHUB_API_TOKEN",
	"vertex":      "GOOGLE_APPLICATION_CREDENTIALS",
	"vertex_ai":   "GOOGLE_APPLICATION_CREDENTIALS",
}

// ResolveCredentialsFile reads the LLM API key from a Helm-mounted credentials
// directory. The Helm chart mounts the credentialsSecretName as a volume; each
// secret key becomes a file. Providers use different key-file names
// (OPENAI_API_KEY, ANTHROPIC_API_KEY, etc.), so we try the provider-specific
// key first, then fall back to any single file.
//
// For GCP providers (vertex, vertex_ai), the file content may be either the
// actual credentials JSON or a file path pointing to the real credentials (#686).
// When the content looks like a path rather than JSON, the function follows
// the indirection and reads the target file.
func ResolveCredentialsFile(provider, credDir string, logger *slog.Logger) string {
	if keyFile, ok := providerKeyFiles[provider]; ok {
		path := filepath.Join(credDir, keyFile)
		if data, err := os.ReadFile(path); err == nil {
			key := strings.TrimSpace(string(data))
			if key != "" {
				key = ResolveGCPCredentialIndirection(provider, key, credDir, logger)
				logger.Info("resolved LLM API key from credentials file", "path", path)
				return key
			}
		}
	}

	entries, err := os.ReadDir(credDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(credDir, e.Name())
		if data, readErr := os.ReadFile(path); readErr == nil {
			key := strings.TrimSpace(string(data))
			if key != "" {
				key = ResolveGCPCredentialIndirection(provider, key, credDir, logger)
				logger.Info("resolved LLM API key from credentials file (fallback)", "path", path)
				return key
			}
		}
	}
	return ""
}

const maxCredentialFileSize = 1 << 20 // 1 MB (F-05)

// ResolveGCPCredentialIndirection handles the case where the mounted
// GOOGLE_APPLICATION_CREDENTIALS file contains a path to the real
// credentials JSON rather than the JSON itself (#686). This happens when the
// Secret key stores a path string and the actual credentials (service_account
// or authorized_user with refresh_token) are in a sibling file.
//
// Security guards:
//   - F-01: path traversal — target must be absolute and inside credDir
//   - F-02: only JSON objects (starting with '{') are treated as credentials
//   - F-05: target file must be <= 1 MB
//   - F-10: relative paths are rejected
//   - Gap 5: content is trimmed before the JSON-object check
//   - Gap 6: returns empty string on failure (not the original content)
func ResolveGCPCredentialIndirection(provider, content, credDir string, logger *slog.Logger) string {
	switch provider {
	case "vertex", "vertex_ai":
	default:
		return content
	}

	trimmed := strings.TrimSpace(content)

	// F-02 + Gap 5: only a JSON object (starts with '{') is valid credentials.
	// json.Valid accepts scalars like "null", "true", "123" — those are not
	// credential objects, so we use the '{' prefix check instead.
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return content
	}

	// Content is not a JSON object — treat as a file path.
	target := trimmed

	// F-10: reject relative paths — they make no sense in a container context
	// and are ambiguous about the working directory.
	if !filepath.IsAbs(target) {
		logger.Warn("GCP credentials indirection rejected: path is not absolute",
			"path", target)
		return ""
	}

	// F-01: path traversal guard — the resolved path must be inside credDir.
	cleaned := filepath.Clean(target)
	if !strings.HasPrefix(cleaned, filepath.Clean(credDir)+string(filepath.Separator)) &&
		cleaned != filepath.Clean(credDir) {
		logger.Warn("GCP credentials indirection rejected: path escapes credential directory",
			"path", target, "credDir", credDir)
		return ""
	}

	// F-05: enforce file size limit before reading.
	info, err := os.Stat(cleaned)
	if err != nil {
		logger.Warn("GCP credentials indirection target is unreadable",
			"path", cleaned, "error", err)
		return "" // Gap 6: return empty, not the original content
	}
	if info.Size() > maxCredentialFileSize {
		logger.Warn("GCP credentials indirection target exceeds size limit",
			"path", cleaned, "size", info.Size(), "limit", maxCredentialFileSize)
		return ""
	}

	data, err := os.ReadFile(cleaned)
	if err != nil {
		logger.Warn("GCP credentials indirection target read failed",
			"path", cleaned, "error", err)
		return ""
	}

	resolved := strings.TrimSpace(string(data))
	if resolved == "" {
		logger.Warn("GCP credentials indirection target is empty", "path", cleaned)
		return ""
	}

	logger.Info("followed GCP credential path indirection", "target", cleaned)
	return resolved
}
