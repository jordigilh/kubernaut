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

package notification

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/notification/formatting"
)

// BR-NOT-083: Markdown to Slack mrkdwn conversion for notification body formatting
// BR-NOT-051: Multi-channel delivery (Slack mrkdwn formatting)
// Issue #48: Markdown to Slack mrkdwn converter
var _ = Describe("BR-NOT-083: Markdown to Slack mrkdwn Converter", func() {

	Describe("MarkdownToMrkdwn", func() {

		Context("empty and nil input", func() {
			It("should return empty string for empty input (UT-NOT-048-001)", func() {
				Expect(formatting.MarkdownToMrkdwn("")).To(Equal(""))
			})

			It("should return plain text unchanged (UT-NOT-048-002)", func() {
				Expect(formatting.MarkdownToMrkdwn("hello world")).To(Equal("hello world"))
			})
		})

		Context("bold conversion", func() {
			It("should convert **bold** to *bold* (UT-NOT-048-010)", func() {
				Expect(formatting.MarkdownToMrkdwn("This is **bold** text")).
					To(Equal("This is *bold* text"))
			})

			It("should convert __bold__ to *bold* (UT-NOT-048-011)", func() {
				Expect(formatting.MarkdownToMrkdwn("This is __bold__ text")).
					To(Equal("This is *bold* text"))
			})

			It("should convert multiple bold segments (UT-NOT-048-012)", func() {
				Expect(formatting.MarkdownToMrkdwn("**first** and **second**")).
					To(Equal("*first* and *second*"))
			})
		})

		Context("strikethrough conversion", func() {
			It("should convert ~~strike~~ to ~strike~ (UT-NOT-048-020)", func() {
				Expect(formatting.MarkdownToMrkdwn("This is ~~deleted~~ text")).
					To(Equal("This is ~deleted~ text"))
			})

			It("should convert multiple strikethrough segments (UT-NOT-048-021)", func() {
				Expect(formatting.MarkdownToMrkdwn("~~old~~ replaced by ~~older~~")).
					To(Equal("~old~ replaced by ~older~"))
			})
		})

		Context("link conversion", func() {
			It("should convert [text](url) to <url|text> (UT-NOT-048-030)", func() {
				Expect(formatting.MarkdownToMrkdwn("[click here](https://example.com)")).
					To(Equal("<https://example.com|click here>"))
			})

			It("should convert multiple links (UT-NOT-048-031)", func() {
				Expect(formatting.MarkdownToMrkdwn("[a](https://a.com) and [b](https://b.com)")).
					To(Equal("<https://a.com|a> and <https://b.com|b>"))
			})

			It("should handle links with special characters in text (UT-NOT-048-032)", func() {
				Expect(formatting.MarkdownToMrkdwn("[pods & services](https://k8s.io)")).
					To(Equal("<https://k8s.io|pods &amp; services>"))
			})
		})

		Context("image conversion", func() {
			It("should convert ![alt](url) to <url|alt> (UT-NOT-048-040)", func() {
				Expect(formatting.MarkdownToMrkdwn("![screenshot](https://img.example.com/pic.png)")).
					To(Equal("<https://img.example.com/pic.png|screenshot>"))
			})
		})

		Context("header conversion", func() {
			It("should convert # Header to *Header* (UT-NOT-048-050)", func() {
				Expect(formatting.MarkdownToMrkdwn("# Main Header")).
					To(Equal("*Main Header*"))
			})

			It("should convert ## Header to *Header* (UT-NOT-048-051)", func() {
				Expect(formatting.MarkdownToMrkdwn("## Sub Header")).
					To(Equal("*Sub Header*"))
			})

			It("should convert ### Header to *Header* (UT-NOT-048-052)", func() {
				Expect(formatting.MarkdownToMrkdwn("### Sub Sub Header")).
					To(Equal("*Sub Sub Header*"))
			})

			It("should convert headers at start of lines in multiline text (UT-NOT-048-053)", func() {
				input := "Some text\n## Analysis Results\nMore text"
				expected := "Some text\n*Analysis Results*\nMore text"
				Expect(formatting.MarkdownToMrkdwn(input)).To(Equal(expected))
			})
		})

		Context("inline code preservation", func() {
			It("should preserve inline code without conversion (UT-NOT-048-060)", func() {
				Expect(formatting.MarkdownToMrkdwn("Use `**not bold**` in code")).
					To(Equal("Use `**not bold**` in code"))
			})

			It("should preserve multiple inline code spans (UT-NOT-048-061)", func() {
				Expect(formatting.MarkdownToMrkdwn("`code1` and **bold** and `code2`")).
					To(Equal("`code1` and *bold* and `code2`"))
			})
		})

		Context("fenced code block preservation", func() {
			It("should preserve fenced code blocks without conversion (UT-NOT-048-070)", func() {
				input := "Before\n```\n**not bold**\n[not a link](url)\n```\nAfter **bold**"
				expected := "Before\n```\n**not bold**\n[not a link](url)\n```\nAfter *bold*"
				Expect(formatting.MarkdownToMrkdwn(input)).To(Equal(expected))
			})

			It("should preserve fenced code blocks with language tag (UT-NOT-048-071)", func() {
				input := "```yaml\nkey: value\n```"
				Expect(formatting.MarkdownToMrkdwn(input)).To(Equal(input))
			})
		})

		Context("preserved syntax", func() {
			It("should preserve _italic_ as-is (UT-NOT-048-080)", func() {
				Expect(formatting.MarkdownToMrkdwn("This is _italic_ text")).
					To(Equal("This is _italic_ text"))
			})

			It("should preserve list items as-is (UT-NOT-048-081)", func() {
				input := "- item 1\n- item 2\n- item 3"
				Expect(formatting.MarkdownToMrkdwn(input)).To(Equal(input))
			})

			It("should preserve blockquotes as-is (UT-NOT-048-082)", func() {
				input := "> This is a quote"
				Expect(formatting.MarkdownToMrkdwn(input)).To(Equal(input))
			})
		})

		Context("HTML entity escaping", func() {
			It("should escape & in plain text (UT-NOT-048-090)", func() {
				Expect(formatting.MarkdownToMrkdwn("pods & services")).
					To(Equal("pods &amp; services"))
			})

			It("should escape < and > in plain text (UT-NOT-048-091)", func() {
				Expect(formatting.MarkdownToMrkdwn("value < 10 and value > 5")).
					To(Equal("value &lt; 10 and value &gt; 5"))
			})

			It("should not escape < > in converted links (UT-NOT-048-092)", func() {
				Expect(formatting.MarkdownToMrkdwn("[link](https://example.com)")).
					To(Equal("<https://example.com|link>"))
			})
		})

		Context("mixed content", func() {
			It("should handle a realistic notification body (UT-NOT-048-100)", func() {
				input := `## Remediation Complete

**Status:** Success
**Pod:** _my-app-pod-xyz_
**Duration:** 45s

The OOMKill remediation for [my-app](https://dashboard.example.com/apps/my-app) completed successfully.

### Actions Taken
- Increased memory limit from ~~512Mi~~ to 1Gi
- Restarted pod

> See the [Grafana dashboard](https://grafana.example.com/d/abc) for metrics.`

				expected := `*Remediation Complete*

*Status:* Success
*Pod:* _my-app-pod-xyz_
*Duration:* 45s

The OOMKill remediation for <https://dashboard.example.com/apps/my-app|my-app> completed successfully.

*Actions Taken*
- Increased memory limit from ~512Mi~ to 1Gi
- Restarted pod

> See the <https://grafana.example.com/d/abc|Grafana dashboard> for metrics.`

				Expect(formatting.MarkdownToMrkdwn(input)).To(Equal(expected))
			})

			It("should handle body with code blocks and formatting (UT-NOT-048-101)", func() {
				input := "**Error** in `kubectl get pods`:\n```\nError: **connection refused**\n```\nPlease check the [docs](https://k8s.io)."
				expected := "*Error* in `kubectl get pods`:\n```\nError: **connection refused**\n```\nPlease check the <https://k8s.io|docs>."
				Expect(formatting.MarkdownToMrkdwn(input)).To(Equal(expected))
			})
		})
	})
})
