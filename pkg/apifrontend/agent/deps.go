package agent

import (
	_ "github.com/Alcova-AI/adk-anthropic-go"      // Claude Sonnet via Vertex AI model adapter (wired in PR5)
	_ "github.com/modelcontextprotocol/go-sdk/mcp" // Real MCP client for KA select_workflow (wired in PR6)
)
