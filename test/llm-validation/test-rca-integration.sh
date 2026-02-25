#!/bin/bash
# Test RCA Toolset Integration
# This script tests if the LLM performs RCA using Kubernetes and Prometheus tools

set -e

echo "========================================="
echo "RCA TOOLSET INTEGRATION TEST"
echo "========================================="
echo ""

# Check if running in cluster or need port-forward
if kubectl get svc holmesgpt-api -n kubernaut-system &>/dev/null; then
    echo "✅ Found holmesgpt-api service in cluster"
else
    echo "❌ holmesgpt-api service not found"
    exit 1
fi

# Port-forward to service
echo "Setting up port-forward..."
kubectl port-forward -n kubernaut-system svc/holmesgpt-api 8080:8080 > /dev/null 2>&1 &
PF_PID=$!
sleep 3

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."
    kill $PF_PID 2>/dev/null || true
}
trap cleanup EXIT

echo ""
echo "=== TEST 1: Send Incident Analysis Request ===" 
curl -s -X POST http://localhost:8080/api/v1/incident/analyze \
  -H "Content-Type: application/json" \
  -d @test/llm-validation/test-e2e-with-rca.json > /tmp/rca-response.json

echo "Response saved to /tmp/rca-response.json"
echo ""

echo "=== TEST 2: Check Toolsets Configuration ===" 
kubectl logs -n kubernaut-system -l app=holmesgpt-api --tail=100 | grep "toolsets" | tail -1
echo ""

echo "=== TEST 3: Check Playbook Count ===" 
PLAYBOOK_COUNT=$(kubectl logs -n kubernaut-system -l app=mock-mcp-server --tail=50 | grep "playbooks returned" | tail -1)
echo "Mock MCP Server: $PLAYBOOK_COUNT"
echo ""

echo "=== TEST 4: Check Tool Calls ===" 
TOOL_CALLS=$(cat /tmp/rca-response.json | python3 -c "import sys, json; print(json.load(sys.stdin)['metadata']['tool_calls'])")
echo "Tool calls made by LLM: $TOOL_CALLS"
echo ""

echo "=== TEST 5: Check LLM Response ===" 
kubectl logs -n kubernaut-system -l app=holmesgpt-api --tail=300 | grep -A30 "RAW LLM RESPONSE" | head -35
echo ""

echo "========================================="
echo "TEST RESULTS"
echo "========================================="
echo ""

# Validate results
if [ "$TOOL_CALLS" -gt 0 ]; then
    echo "✅ PASS: LLM made $TOOL_CALLS tool calls"
else
    echo "❌ FAIL: LLM made 0 tool calls (expected > 0)"
fi

if echo "$PLAYBOOK_COUNT" | grep -q "2 playbooks"; then
    echo "✅ PASS: Mock MCP Server returned 2 playbooks"
elif echo "$PLAYBOOK_COUNT" | grep -q "0 playbooks"; then
    echo "❌ FAIL: Mock MCP Server returned 0 playbooks (expected 2)"
else
    echo "⚠️  WARN: Could not determine playbook count"
fi

echo ""
echo "Full response:"
cat /tmp/rca-response.json | python3 -m json.tool

