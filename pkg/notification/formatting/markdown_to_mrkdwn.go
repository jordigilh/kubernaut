/*
Copyright 2025 Jordi Gil.

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

package formatting

import (
	"fmt"
	"regexp"
	"strings"
)

// Pre-compiled regex patterns for markdown-to-mrkdwn conversion.
// Compiled once at package init for performance (avoids recompilation per call).
var (
	// fencedCodeRe matches fenced code blocks: ```language\n...\n```
	fencedCodeRe = regexp.MustCompile("(?s)```[a-zA-Z]*\n?.*?```")
	// inlineCodeRe matches inline code: `...`
	inlineCodeRe = regexp.MustCompile("`[^`]+`")
	// blockquoteRe matches > at start of line (blockquote syntax)
	blockquoteRe = regexp.MustCompile(`(?m)^>`)
	// imageRe matches Markdown images: ![alt](url)
	imageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	// linkRe matches Markdown links: [text](url)
	linkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	// boldDoubleStarRe matches **bold** (double asterisks)
	boldDoubleStarRe = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	// boldDoubleUnderRe matches __bold__ (double underscores)
	boldDoubleUnderRe = regexp.MustCompile(`__([^_]+)__`)
	// strikeRe matches ~~strikethrough~~ (double tildes)
	strikeRe = regexp.MustCompile(`~~([^~]+)~~`)
	// headerRe matches Markdown headers: # through ###### at line start
	headerRe = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
)

// MarkdownToMrkdwn converts standard Markdown to Slack's mrkdwn syntax.
//
// Slack uses its own markup format called mrkdwn that differs from standard Markdown:
//   - **bold** / __bold__ → *bold*
//   - ~~strike~~ → ~strike~
//   - [text](url) → <url|text>
//   - ![alt](url) → <url|alt>
//   - # Header → *Header* (bold as fallback, no native header support)
//   - Inline code and fenced code blocks are preserved without conversion
//   - &, <, > are escaped in plain text per Slack requirements
//   - Blockquotes (> at line start) are preserved (shared syntax)
//
// Reference: https://docs.slack.dev/messaging/formatting-message-text/
//
// Business Requirements:
//   - BR-NOT-051: Multi-channel delivery (Slack mrkdwn format)
//   - Issue #48: Markdown to Slack mrkdwn converter
func MarkdownToMrkdwn(input string) string {
	if input == "" {
		return ""
	}

	// Phase 1: Extract and protect code blocks and inline code from conversion.
	// We replace them with placeholders, do all conversions, then restore them.
	var codeBlocks []string
	result := input

	// Protect fenced code blocks (```...```) first
	result = fencedCodeRe.ReplaceAllStringFunc(result, func(match string) string {
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, match)
		return fmt.Sprintf("\x00CODEBLOCK%d\x00", idx)
	})

	// Protect inline code (`...`)
	result = inlineCodeRe.ReplaceAllStringFunc(result, func(match string) string {
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, match)
		return fmt.Sprintf("\x00CODEBLOCK%d\x00", idx)
	})

	// Phase 2: Escape HTML entities in plain text (&, <, >).
	// Must happen BEFORE link conversion so that link angle brackets aren't escaped.
	// Note: > at start of line is blockquote syntax in both Markdown and Slack mrkdwn,
	// so we preserve it. We only escape > that appears mid-line.
	result = strings.ReplaceAll(result, "&", "&amp;")
	result = strings.ReplaceAll(result, "<", "&lt;")

	// Escape > only when NOT at the start of a line (preserve blockquotes)
	result = blockquoteRe.ReplaceAllString(result, "\x00BLOCKQUOTE\x00")
	result = strings.ReplaceAll(result, ">", "&gt;")
	result = strings.ReplaceAll(result, "\x00BLOCKQUOTE\x00", ">")

	// Phase 3: Convert images ![alt](url) → <url|alt>
	// Must be before link conversion to avoid ![alt](url) matching [alt](url)
	result = imageRe.ReplaceAllStringFunc(result, func(match string) string {
		submatches := imageRe.FindStringSubmatch(match)
		if len(submatches) < 3 {
			return match
		}
		return fmt.Sprintf("<%s|%s>", submatches[2], submatches[1])
	})

	// Phase 4: Convert links [text](url) → <url|text>
	result = linkRe.ReplaceAllStringFunc(result, func(match string) string {
		submatches := linkRe.FindStringSubmatch(match)
		if len(submatches) < 3 {
			return match
		}
		return fmt.Sprintf("<%s|%s>", submatches[2], submatches[1])
	})

	// Phase 5: Convert bold **text** → *text* (must be before single *)
	result = boldDoubleStarRe.ReplaceAllString(result, "*$1*")

	// Convert bold __text__ → *text*
	result = boldDoubleUnderRe.ReplaceAllString(result, "*$1*")

	// Phase 6: Convert strikethrough ~~text~~ → ~text~
	result = strikeRe.ReplaceAllString(result, "~$1~")

	// Phase 7: Convert headers (# Header → *Header*)
	result = headerRe.ReplaceAllString(result, "*$1*")

	// Phase 8: Restore protected code blocks
	for i, block := range codeBlocks {
		placeholder := fmt.Sprintf("\x00CODEBLOCK%d\x00", i)
		result = strings.Replace(result, placeholder, block, 1)
	}

	return result
}
