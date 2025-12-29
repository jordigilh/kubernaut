"""
E2E Tests for HolmesGPT-API

These tests validate the complete workflow from request to response,
using a deterministic mock LLM server that returns predictable tool calls.

Test Architecture:
    E2E Test → HolmesGPT-API → Mock LLM (tool calls) → Mock Data Storage

Features:
- Deterministic: Same input → same output
- Fast: No real LLM calls
- Free: No API costs
- Validates integration without LLM variability
"""

