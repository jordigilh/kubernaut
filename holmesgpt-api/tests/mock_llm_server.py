"""
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
"""

"""
Mock LLM Server for Integration Tests

Provides a mock Ollama-compatible LLM endpoint that returns predictable responses.
Used for integration testing without requiring a real LLM provider.

Usage:
    # In pytest fixture
    with MockLLMServer() as server:
        os.environ["LLM_ENDPOINT"] = server.url
        os.environ["LLM_MODEL"] = "mock-model"
        # Run tests...
"""

import json
import threading
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import Optional, Dict, Any
import logging

logger = logging.getLogger(__name__)


class MockLLMRequestHandler(BaseHTTPRequestHandler):
    """HTTP request handler that mimics Ollama/OpenAI API responses."""

    def log_message(self, format, *args):
        """Suppress default logging to reduce test noise."""
        pass

    def do_POST(self):
        """Handle POST requests (chat completions, generate, etc.)."""
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length).decode('utf-8')

        try:
            request_data = json.loads(body) if body else {}
        except json.JSONDecodeError:
            request_data = {}

        # Route based on endpoint
        if self.path == "/api/generate" or self.path == "/api/chat":
            # Ollama-style endpoint
            response = self._handle_ollama_request(request_data)
        elif self.path in ["/v1/chat/completions", "/chat/completions"]:
            # OpenAI-style endpoint (with or without /v1 prefix)
            response = self._handle_openai_request(request_data)
        else:
            response = self._handle_generic_request(request_data)

        self._send_json_response(response)

    def do_GET(self):
        """Handle GET requests (health checks, model list, etc.)."""
        if self.path == "/api/tags" or self.path == "/v1/models":
            # Model list endpoint
            response = {
                "models": [
                    {"name": "mock-model", "size": 1000000}
                ]
            }
        else:
            response = {"status": "ok"}

        self._send_json_response(response)

    def _handle_ollama_request(self, request_data: Dict[str, Any]) -> Dict[str, Any]:
        """Generate Ollama-compatible response."""
        prompt = request_data.get("prompt", "")
        messages = request_data.get("messages", [])

        # Extract the actual prompt content
        if messages:
            prompt = messages[-1].get("content", "") if messages else ""

        # Generate mock response based on prompt content
        mock_response = self._generate_mock_response(prompt)

        return {
            "model": request_data.get("model", "mock-model"),
            "created_at": "2025-11-30T00:00:00Z",
            "response": mock_response,
            "done": True,
            "context": [],
            "total_duration": 1000000000,
            "load_duration": 100000000,
            "prompt_eval_count": 100,
            "eval_count": 50
        }

    def _handle_openai_request(self, request_data: Dict[str, Any]) -> Dict[str, Any]:
        """Generate OpenAI-compatible response."""
        messages = request_data.get("messages", [])
        prompt = messages[-1].get("content", "") if messages else ""

        mock_response = self._generate_mock_response(prompt)

        return {
            "id": "chatcmpl-mock-123",
            "object": "chat.completion",
            "created": 1701388800,
            "model": request_data.get("model", "mock-model"),
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": mock_response
                    },
                    "finish_reason": "stop"
                }
            ],
            "usage": {
                "prompt_tokens": 100,
                "completion_tokens": 50,
                "total_tokens": 150
            }
        }

    def _handle_generic_request(self, request_data: Dict[str, Any]) -> Dict[str, Any]:
        """Handle unknown endpoints with generic response."""
        return {
            "status": "ok",
            "message": "Mock LLM server received request",
            "path": self.path
        }

    def _generate_mock_response(self, prompt: str) -> str:
        """
        Generate a mock LLM response based on the prompt content.

        Returns structured JSON responses that the HolmesGPT API expects.
        """
        prompt_lower = prompt.lower()

        # Recovery analysis request
        if "recovery" in prompt_lower or "previous remediation" in prompt_lower:
            return self._recovery_response(prompt)

        # Incident analysis request
        if "incident" in prompt_lower or "root cause" in prompt_lower:
            return self._incident_response(prompt)

        # Default response
        return self._default_response()

    def _recovery_response(self, prompt: str) -> str:
        """Generate mock recovery analysis response."""
        return """Based on my investigation of the recovery scenario:

## Analysis

The previous remediation attempt failed. I've analyzed the current cluster state and determined an alternative approach.

```json
{
  "recovery_analysis": {
    "previous_attempt_assessment": {
      "failure_understood": true,
      "failure_reason_analysis": "The previous workflow exceeded resource limits",
      "state_changed": false,
      "current_signal_type": "OOMKilled"
    },
    "current_rca": {
      "summary": "Memory exhaustion persists, requires different approach",
      "severity": "high",
      "signal_type": "OOMKilled",
      "contributing_factors": ["insufficient_memory_limits", "memory_leak"]
    }
  },
  "selected_workflow": {
    "workflow_id": "memory-optimize-v1",
    "version": "1.0.0",
    "confidence": 0.85,
    "rationale": "This workflow reduces memory footprint instead of scaling",
    "parameters": {
      "MEMORY_LIMIT": "512Mi",
      "OPTIMIZATION_LEVEL": "aggressive"
    }
  },
  "recovery_strategy": {
    "approach": "Optimize memory usage rather than horizontal scaling",
    "differs_from_previous": true,
    "why_different": "Previous scaling approach failed due to cluster resource constraints"
  }
}
```
"""

    def _incident_response(self, prompt: str) -> str:
        """Generate mock incident analysis response."""
        return """Based on my investigation of the incident:

## Root Cause Analysis

I've analyzed the cluster state and identified the root cause of the issue.

```json
{
  "root_cause_analysis": {
    "summary": "Container exceeded memory limits due to traffic spike",
    "severity": "high",
    "contributing_factors": ["traffic_spike", "insufficient_memory_limits"]
  },
  "selected_workflow": {
    "workflow_id": "scale-horizontal-v1",
    "version": "1.0.0",
    "confidence": 0.90,
    "rationale": "Horizontal scaling will distribute load across more pods",
    "parameters": {
      "TARGET_REPLICAS": "5",
      "NAMESPACE": "production"
    }
  }
}
```
"""

    def _default_response(self) -> str:
        """Generate default mock response."""
        return """I've analyzed the request.

```json
{
  "analysis": "Mock analysis complete",
  "recommendation": "No specific action required"
}
```
"""

    def _send_json_response(self, response: Dict[str, Any], status_code: int = 200):
        """Send JSON response with appropriate headers."""
        response_body = json.dumps(response).encode('utf-8')

        self.send_response(status_code)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', len(response_body))
        self.end_headers()
        self.wfile.write(response_body)


class MockLLMServer:
    """
    Context manager for running a mock LLM server.

    Usage:
        with MockLLMServer(port=11434) as server:
            print(f"Mock LLM running at {server.url}")
            # Run tests...
    """

    def __init__(self, host: str = "127.0.0.1", port: int = 0):
        """
        Initialize mock LLM server.

        Args:
            host: Host to bind to (default: 127.0.0.1)
            port: Port to bind to (default: 0 = auto-assign)
        """
        self.host = host
        self.port = port
        self.server: Optional[HTTPServer] = None
        self.thread: Optional[threading.Thread] = None

    def __enter__(self) -> "MockLLMServer":
        """Start the mock server."""
        self.start()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Stop the mock server."""
        self.stop()

    def start(self):
        """Start the mock LLM server in a background thread."""
        self.server = HTTPServer((self.host, self.port), MockLLMRequestHandler)

        # Get the actual port if auto-assigned
        if self.port == 0:
            self.port = self.server.server_address[1]

        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

        logger.info(f"Mock LLM server started at {self.url}")

    def stop(self):
        """Stop the mock LLM server."""
        if self.server:
            self.server.shutdown()
            self.server.server_close()
            logger.info("Mock LLM server stopped")

    @property
    def url(self) -> str:
        """Get the server URL."""
        return f"http://{self.host}:{self.port}"


# Pytest fixture helper
def create_mock_llm_fixture():
    """
    Create a pytest fixture for the mock LLM server.

    Usage in conftest.py:
        from tests.mock_llm_server import create_mock_llm_fixture
        mock_llm = create_mock_llm_fixture()
    """
    import pytest

    @pytest.fixture(scope="session")
    def mock_llm_server():
        """Session-scoped mock LLM server."""
        with MockLLMServer() as server:
            yield server

    return mock_llm_server

