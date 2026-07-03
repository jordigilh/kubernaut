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
	// Language tag allows letters, digits, hyphens, and plus signs (json5, c++, protobuf3).
	fencedCodeRe = regexp.MustCompile("(?s)```[a-zA-Z0-9+\\-]*\\s*\n?.*?```")
	// fencedLangTagRe strips language tags from fenced code block opening: ```yaml → ```
	fencedLangTagRe = regexp.MustCompile("```[a-zA-Z0-9+\\-]+")
	// emptyFencedBlockRe matches empty fenced code blocks (opening + closing with only whitespace)
	emptyFencedBlockRe = regexp.MustCompile("(?m)```\\s*```")
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
//   - BR-NOT-083: Markdown to Slack mrkdwn conversion for notification body formatting
//   - BR-NOT-051: Multi-channel delivery (Slack mrkdwn format)
//   - Issue #48: Markdown to Slack mrkdwn converter
func MarkdownToMrkdwn(input string) string {
	if input == "" {
		return ""
	}

	// Phase 1: Extract and protect code blocks and inline code from conversion.
	// We replace them with placeholders, do all conversions, then restore them.
	result, codeBlocks := protectCodeBlocks(input)

	// Phase 2: Escape HTML entities in plain text (&, <, >).
	result = escapeHTMLEntities(result)

	// Phase 3-4: Convert images and links to Slack's <url|text> syntax.
	result = convertImagesAndLinks(result)

	// Phase 5-7: Convert bold, strikethrough, and headers to mrkdwn equivalents.
	result = convertEmphasisAndHeaders(result)

	// Phase 8-10: Restore protected code blocks and clean up formatting artifacts.
	result = restoreCodeBlocksAndCleanup(result, codeBlocks)

	return result
}

// protectCodeBlocks extracts fenced and inline code blocks from the input,
// replacing each with a unique placeholder so that later conversion phases
// don't rewrite code content. Returns the placeholder-substituted text and
// the extracted blocks (indexed by placeholder number) for later restoration.
// Extracted from MarkdownToMrkdwn (Wave 6 6b GREEN: funlen remediation) —
// pure code motion, no behavior change.
func protectCodeBlocks(input string) (string, []string) {
	var codeBlocks []string
	protect := func(match string) string {
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, match)
		return fmt.Sprintf("\x00CODEBLOCK%d\x00", idx)
	}

	result := fencedCodeRe.ReplaceAllStringFunc(input, protect)
	result = inlineCodeRe.ReplaceAllStringFunc(result, protect)
	return result, codeBlocks
}

// escapeHTMLEntities escapes &, <, and > in plain text per Slack requirements.
// Must run BEFORE link conversion so that link angle brackets aren't escaped.
// Note: > at start of line is blockquote syntax in both Markdown and Slack
// mrkdwn, so it's temporarily protected and restored — only > appearing
// mid-line is escaped. Extracted from MarkdownToMrkdwn (Wave 6 6b GREEN:
// funlen remediation) — pure code motion, no behavior change.
func escapeHTMLEntities(input string) string {
	result := strings.ReplaceAll(input, "&", "&amp;")
	result = strings.ReplaceAll(result, "<", "&lt;")

	result = blockquoteRe.ReplaceAllString(result, "\x00BLOCKQUOTE\x00")
	result = strings.ReplaceAll(result, ">", "&gt;")
	result = strings.ReplaceAll(result, "\x00BLOCKQUOTE\x00", ">")
	return result
}

// convertImagesAndLinks converts Markdown images (![alt](url)) and links
// ([text](url)) into Slack's <url|text> syntax. Images are converted first
// to avoid ![alt](url) matching the link pattern. Extracted from
// MarkdownToMrkdwn (Wave 6 6b GREEN: funlen remediation) — pure code motion,
// no behavior change.
func convertImagesAndLinks(input string) string {
	toSlackLink := func(re *regexp.Regexp) func(string) string {
		return func(match string) string {
			submatches := re.FindStringSubmatch(match)
			if len(submatches) < 3 {
				return match
			}
			return fmt.Sprintf("<%s|%s>", submatches[2], submatches[1])
		}
	}

	result := imageRe.ReplaceAllStringFunc(input, toSlackLink(imageRe))
	result = linkRe.ReplaceAllStringFunc(result, toSlackLink(linkRe))
	return result
}

// convertEmphasisAndHeaders converts bold (**text**/__text__), strikethrough
// (~~text~~), and headers (# Header) into their mrkdwn equivalents. Extracted
// from MarkdownToMrkdwn (Wave 6 6b GREEN: funlen remediation) — pure code
// motion, no behavior change.
func convertEmphasisAndHeaders(input string) string {
	result := boldDoubleStarRe.ReplaceAllString(input, "*$1*")
	result = boldDoubleUnderRe.ReplaceAllString(result, "*$1*")
	result = strikeRe.ReplaceAllString(result, "~$1~")
	result = headerRe.ReplaceAllString(result, "*$1*")
	return result
}

// restoreCodeBlocksAndCleanup restores the placeholders created by
// protectCodeBlocks with Slack-compatible post-processed code (language tags
// stripped, empty blocks removed — Issue #588), fixes any unbalanced triple
// backtick left after restoration, and collapses blank lines left behind by
// removed empty code blocks. Extracted from MarkdownToMrkdwn (Wave 6 6b
// GREEN: funlen remediation) — pure code motion, no behavior change.
func restoreCodeBlocksAndCleanup(input string, codeBlocks []string) string {
	result := input
	for i, block := range codeBlocks {
		placeholder := fmt.Sprintf("\x00CODEBLOCK%d\x00", i)
		cleaned := fencedLangTagRe.ReplaceAllString(block, "```")
		cleaned = emptyFencedBlockRe.ReplaceAllString(cleaned, "")
		result = strings.Replace(result, placeholder, cleaned, 1)
	}

	// Handle unbalanced triple backticks remaining after code block restoration.
	// Count ``` occurrences — if odd, there's an unpaired fence. Escape only the
	// last (orphan) occurrence to avoid corrupting valid fenced blocks.
	if strings.Count(result, "```")%2 != 0 {
		lastIdx := strings.LastIndex(result, "```")
		if lastIdx >= 0 {
			result = result[:lastIdx] + "`\u200B`\u200B`" + result[lastIdx+3:]
		}
	}

	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return result
}
