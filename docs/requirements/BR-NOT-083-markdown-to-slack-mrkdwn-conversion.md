# BR-NOT-083: Markdown to Slack mrkdwn Conversion for Notification Body Formatting

**Service**: Notification Controller
**Category**: Message Formatting (Delivery Quality)
**Priority**: P1 (HIGH)
**Version**: 1.0
**Date**: 2026-02-06
**Status**: Implemented (Issue #48)
**Related ADRs**: DD-AUDIT-004 (Structured Types for Audit Event Payloads)
**Related BRs**: BR-NOT-051 (Multi-channel delivery), BR-NOT-055 (Message formatting with priority indicators)

---

## Overview

The Notification Service MUST convert standard Markdown syntax in notification body content to Slack's `mrkdwn` syntax before delivering messages via the Slack channel, ensuring notification bodies render correctly with proper formatting (bold, links, headers, strikethrough, code blocks).

**Business Value**: Prevents broken formatting in Slack notifications by automatically translating standard Markdown (produced by upstream services like AIAnalysis, RemediationOrchestrator, and HolmesGPT) into Slack's native `mrkdwn` syntax, improving notification readability and reducing operator confusion when reviewing remediation outcomes.

---

## BR-NOT-083: Markdown to Slack mrkdwn Conversion

### Description

Notification bodies (`NotificationRequest.Spec.Body`) are authored by upstream services using standard Markdown. Slack uses its own markup language called `mrkdwn` ([reference](https://docs.slack.dev/messaging/formatting-message-text/)) which differs from standard Markdown in several key ways. Without conversion, Markdown-formatted bodies render as literal syntax characters in Slack (e.g., `**bold**` appears as `**bold**` instead of **bold**).

The Notification Service MUST automatically convert the following Markdown constructs to their Slack mrkdwn equivalents:

| Standard Markdown | Slack mrkdwn | Behavior Without Conversion |
|---|---|---|
| `**bold**` | `*bold*` | Literal `**bold**` (broken) |
| `__bold__` | `*bold*` | Literal `__bold__` (broken) |
| `~~strike~~` | `~strike~` | Literal `~~strike~~` (broken) |
| `[text](url)` | `<url\|text>` | Literal `[text](url)` (broken) |
| `![alt](url)` | `<url\|alt>` | Literal `![alt](url)` (broken) |
| `# Header` | `*Header*` | Literal `# Header` (no rendering) |
| `` `code` `` | `` `code` `` | Correct (shared syntax) |
| ```` ``` ```` code blocks | ```` ``` ```` code blocks | Correct (shared syntax) |
| `_italic_` | `_italic_` | Correct (shared syntax) |
| `- list` | `- list` | Correct (shared syntax) |
| `> blockquote` | `> blockquote` | Correct (shared syntax) |

### Priority

**P1 (HIGH)** - Notification Quality for V1.0

### Rationale

#### Problem

Upstream services generate notification bodies using standard Markdown:
- **AIAnalysis**: RCA summaries with headers, bold labels, and links to dashboards
- **RemediationOrchestrator**: Status updates with structured formatting
- **HolmesGPT**: Investigation results with code blocks and linked resources

The `SlackDeliveryService` passes the body directly to Slack Block Kit with `Type: slack.MarkdownType`, but Slack does not interpret standard Markdown -- it uses its own `mrkdwn` syntax. This causes:
- Bold text (`**text**`) appearing as literal asterisks
- Links (`[text](url)`) appearing as literal bracket syntax
- Headers (`# Header`) appearing as literal hash characters
- Strikethrough (`~~text~~`) appearing as literal tildes

#### Impact

- **Operator confusion**: Remediation notifications in Slack are hard to read
- **Missed critical links**: Dashboard links and documentation references render as plaintext
- **Unprofessional appearance**: Broken formatting undermines trust in the system
- **Increased MTTR**: Operators must mentally parse broken Markdown to understand notifications

#### Solution

A `MarkdownToMrkdwn` converter function that:
1. Protects code blocks (fenced and inline) from conversion
2. Escapes `&`, `<`, `>` per Slack requirements (preserving blockquote `>` at line starts)
3. Converts images and links to Slack angle-bracket syntax
4. Converts bold, strikethrough, and headers to mrkdwn equivalents
5. Preserves shared syntax (italic, lists, code blocks, blockquotes)

### Implementation

#### Converter Function

**Location**: `pkg/notification/formatting/markdown_to_mrkdwn.go`

```go
func MarkdownToMrkdwn(input string) string
```

**Conversion Pipeline** (8 phases):
1. Extract and protect fenced code blocks (```` ``` ... ``` ````) with placeholders
2. Extract and protect inline code (`` ` ... ` ``) with placeholders
3. Escape HTML entities (`&` -> `&amp;`, `<` -> `&lt;`, `>` -> `&gt;`) preserving blockquote `>` at line starts
4. Convert images `![alt](url)` -> `<url|alt>`
5. Convert links `[text](url)` -> `<url|text>`
6. Convert bold `**text**` / `__text__` -> `*text*`
7. Convert strikethrough `~~text~~` -> `~text~`
8. Convert headers `# text` through `###### text` -> `*text*`
9. Restore protected code blocks from placeholders

**Performance**: All regex patterns pre-compiled at package init time.

#### Integration Points

1. **FormatSlackBlocks** (`pkg/notification/delivery/slack_blocks.go`):
   - Body text passed through `MarkdownToMrkdwn()` before setting `slack.MarkdownType`

2. **SlackFormatter** (`pkg/notification/formatting/slack.go`):
   - Previously stubbed `Format()` method now fully implemented using the converter
   - Includes 40KB payload size enforcement per Slack limits

### Acceptance Criteria

- [ ] `MarkdownToMrkdwn("")` returns `""`
- [ ] `**bold**` converts to `*bold*`
- [ ] `__bold__` converts to `*bold*`
- [ ] `~~strike~~` converts to `~strike~`
- [ ] `[text](url)` converts to `<url|text>`
- [ ] `![alt](url)` converts to `<url|alt>`
- [ ] `# Header` through `###### Header` converts to `*Header*`
- [ ] Inline code `` `**not bold**` `` is preserved without conversion
- [ ] Fenced code blocks are preserved without conversion
- [ ] `_italic_` is preserved as-is (shared syntax)
- [ ] List items (`- item`) are preserved as-is
- [ ] Blockquotes (`> text`) are preserved as-is
- [ ] `&` is escaped to `&amp;` in plain text
- [ ] `<` is escaped to `&lt;` in plain text
- [ ] `>` is escaped to `&gt;` in plain text (except at line start for blockquotes)
- [ ] Links within escaped text render correctly (angle brackets not double-escaped)
- [ ] Mixed content (Markdown + plain text + code blocks) converts correctly
- [ ] Realistic notification bodies (RCA summaries, remediation status) render correctly
- [ ] `FormatSlackBlocks()` uses the converter for body text
- [ ] `SlackFormatter.Format()` stub is fully implemented
- [ ] Existing E2E notification tests continue to pass

### Test Coverage

**Test File**: `test/unit/notification/markdown_to_mrkdwn_test.go`
**Test Count**: 27 unit tests
**Framework**: Ginkgo/Gomega BDD

| Test ID | Description | Category |
|---|---|---|
| UT-NOT-048-001 | Empty input returns empty string | Edge case |
| UT-NOT-048-002 | Plain text unchanged | Edge case |
| UT-NOT-048-010 | `**bold**` -> `*bold*` | Bold conversion |
| UT-NOT-048-011 | `__bold__` -> `*bold*` | Bold conversion |
| UT-NOT-048-012 | Multiple bold segments | Bold conversion |
| UT-NOT-048-020 | `~~strike~~` -> `~strike~` | Strikethrough |
| UT-NOT-048-021 | Multiple strikethrough segments | Strikethrough |
| UT-NOT-048-030 | `[text](url)` -> `<url\|text>` | Link conversion |
| UT-NOT-048-031 | Multiple links | Link conversion |
| UT-NOT-048-032 | Links with special chars in text | Link conversion |
| UT-NOT-048-040 | `![alt](url)` -> `<url\|alt>` | Image conversion |
| UT-NOT-048-050 | `# Header` -> `*Header*` | Header conversion |
| UT-NOT-048-051 | `## Header` -> `*Header*` | Header conversion |
| UT-NOT-048-052 | `### Header` -> `*Header*` | Header conversion |
| UT-NOT-048-053 | Headers in multiline text | Header conversion |
| UT-NOT-048-060 | Inline code preserved | Code preservation |
| UT-NOT-048-061 | Multiple inline code spans | Code preservation |
| UT-NOT-048-070 | Fenced code blocks preserved | Code preservation |
| UT-NOT-048-071 | Fenced code blocks with language | Code preservation |
| UT-NOT-048-080 | `_italic_` preserved | Preserved syntax |
| UT-NOT-048-081 | List items preserved | Preserved syntax |
| UT-NOT-048-082 | Blockquotes preserved | Preserved syntax |
| UT-NOT-048-090 | `&` escaped to `&amp;` | HTML escaping |
| UT-NOT-048-091 | `<` `>` escaped | HTML escaping |
| UT-NOT-048-092 | `<>` not escaped in links | HTML escaping |
| UT-NOT-048-100 | Realistic notification body | Mixed content |
| UT-NOT-048-101 | Code blocks + formatting + links | Mixed content |

---

## Dependencies

### Upstream Services (Producers)

Upstream services that generate `NotificationRequest.Spec.Body` content:
- **AIAnalysis Controller**: RCA summaries
- **RemediationOrchestrator Controller**: Status updates, approval requests
- **HolmesGPT API**: Investigation results

No changes required to upstream services -- they continue to produce standard Markdown.

### Downstream Dependencies

- **slack-go/slack SDK**: Existing dependency, no changes
- **Slack Block Kit API**: `mrkdwn` text type in section blocks

### Related BRs

- **BR-NOT-051**: Multi-channel delivery (Slack Block Kit format)
  - BR-NOT-083 ensures Slack channel body content renders correctly
- **BR-NOT-055**: Message formatting with priority indicators
  - BR-NOT-083 handles body formatting; priority indicators remain in header block

---

## Reference

- [Slack mrkdwn formatting documentation](https://docs.slack.dev/messaging/formatting-message-text/)
- GitHub Issue: [#48](https://github.com/jordigilh/kubernaut/issues/48)
- Implementation: `pkg/notification/formatting/markdown_to_mrkdwn.go`
- Tests: `test/unit/notification/markdown_to_mrkdwn_test.go`
- Integration: `pkg/notification/delivery/slack_blocks.go`, `pkg/notification/formatting/slack.go`

---

**File**: `docs/requirements/BR-NOT-083-markdown-to-slack-mrkdwn-conversion.md`
