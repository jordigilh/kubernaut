#!/bin/bash
#
# LLM-Only Test Runner
# Scenario 1: Independent LLM testing without K8s dependencies
# Business Requirement: BR-HAPI-046 - Integration tests with adaptive LLM support
#

echo "🧠 Running LLM-only integration tests (no K8s dependencies)..."
echo "  ├── LLM Endpoint: ${LLM_ENDPOINT:-http://localhost:8080}"
echo "  ├── LLM Provider: ${LLM_PROVIDER:-auto-detect}"
echo "  ├── Use Mock LLM: ${USE_MOCK_LLM:-false}"
echo "  └── Scenario: Independent LLM testing"

# Set default values
export LLM_ENDPOINT=${LLM_ENDPOINT:-http://localhost:8080}
export LLM_PROVIDER=${LLM_PROVIDER:-auto-detect}
export USE_MOCK_LLM=${USE_MOCK_LLM:-false}

# Run the tests
# Use PYTHON_CMD environment variable if set, otherwise default to /usr/local/bin/python3
PYTHON_EXEC=${PYTHON_CMD:-/usr/local/bin/python3}
echo "  ├── Python Executable: $PYTHON_EXEC"
PYTHONPATH=./src $PYTHON_EXEC -m pytest tests/integration/test_llm_only.py -v --tb=short -m "llm_only"

exit_code=$?

if [ $exit_code -eq 0 ]; then
    echo ""
    echo "✅ LLM-only tests completed successfully!"
    echo "   All business requirements validated:"
    echo "   - BR-HAPI-046.1: Real LLM integration ✅"
    echo "   - BR-HAPI-046.2: Mock LLM fallback ✅"
    echo "   - BR-HAPI-046.3: Auto-detection ✅"
    echo "   - BR-HAPI-046.4: Graceful degradation ✅"
    echo "   - BR-HAPI-046.5: Performance testing ✅"
else
    echo ""
    echo "❌ LLM-only tests failed (exit code: $exit_code)"
    echo "   Check the output above for details"
fi

exit $exit_code

