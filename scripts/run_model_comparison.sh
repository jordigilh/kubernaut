#!/bin/bash
set -e

# Model Comparison Test Runner Script
# Executes the model comparison test suite and generates reports

echo "🧪 Running Model Comparison Tests..."

# Configuration
TEST_PACKAGE="./test/integration/model_comparison"
RESULTS_DIR="model_comparison_results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
REPORT_DIR="${RESULTS_DIR}/${TIMESTAMP}"

# Function to check if servers are healthy
check_servers_health() {
    local servers=(
        "11434:granite3.1-dense:8b"
        "11435:deepseek-coder:7b-instruct"
        "11436:granite3.1-steiner:8b"
    )

    echo "🔍 Checking model servers health..."

    for server in "${servers[@]}"; do
        local port=$(echo "$server" | cut -d: -f1)
        local model=$(echo "$server" | cut -d: -f2-)
        local endpoint="http://localhost:$port"

        echo "   Checking $model at $endpoint..."

        # Test models endpoint
        if ! curl -s -f "$endpoint/v1/models" >/dev/null; then
            echo "❌ $model server is not healthy at port $port"
            echo "   Please run: ./scripts/setup_model_comparison.sh"
            return 1
        fi

        # Test simple completion
        local response
        response=$(curl -s -X POST "$endpoint/v1/chat/completions" \
            -H "Content-Type: application/json" \
            -d '{
                "model": "'"$model"'",
                "messages": [{"role": "user", "content": "Test"}],
                "max_tokens": 5
            }')

        if ! echo "$response" | jq -e '.choices[0].message.content' >/dev/null 2>&1; then
            echo "❌ $model server failed completion test"
            echo "Response: $response"
            return 1
        fi

        echo "✅ $model server is healthy"
    done

    return 0
}

# Function to setup test environment
setup_test_env() {
    echo "🔧 Setting up test environment..."

    # Create results directory
    mkdir -p "$REPORT_DIR"

    # Set environment variables for tests
    export MODEL_COMPARISON_RESULTS_DIR="$REPORT_DIR"
    export MODEL_COMPARISON_TIMESTAMP="$TIMESTAMP"

    echo "   Results will be saved to: $REPORT_DIR"
}

# Function to run tests with timeout and retry
run_tests_with_retry() {
    local max_attempts=3
    local attempt=1
    local test_timeout=1800  # 30 minutes

    while [ $attempt -le $max_attempts ]; do
        echo "🧪 Running test attempt $attempt/$max_attempts..."

        # Run tests with timeout
        local test_cmd="go test $TEST_PACKAGE -v -timeout ${test_timeout}s"

        if timeout $test_timeout $test_cmd; then
            echo "✅ Tests completed successfully!"
            return 0
        else
            local exit_code=$?
            echo "❌ Test attempt $attempt failed (exit code: $exit_code)"

            if [ $attempt -eq $max_attempts ]; then
                echo "💥 All test attempts failed!"
                return $exit_code
            fi

            echo "⏳ Waiting 30 seconds before retry..."
            sleep 30
            attempt=$((attempt + 1))
        fi
    done
}

# Function to collect and organize results
collect_results() {
    echo "📊 Collecting test results..."

    # Move generated reports to results directory
    for file in model_comparison_*.md model_comparison_*.json; do
        if [[ -f "$file" ]]; then
            mv "$file" "$REPORT_DIR/"
            echo "   Moved $file to results directory"
        fi
    done

    # Create summary index
    cat > "$REPORT_DIR/README.md" << EOF
# Model Comparison Results - $TIMESTAMP

This directory contains the results of the model comparison test run.

## Files
- \`model_comparison_results.json\` - Raw test results data
- \`model_comparison_report.md\` - Human-readable comparison report
- \`model_recommendation.md\` - Model selection recommendation

## Test Configuration
- **Date**: $(date)
- **Models Tested**:
  - granite3.1-dense:8b (baseline)
  - deepseek-coder:7b-instruct
  - granite3.1-steiner:8b
- **Test Scenarios**: 9 alert scenarios across different categories
- **Runs per Model**: 3 runs per scenario for consistency testing

## Quick Summary
EOF

    # Add quick summary if results exist
    if [[ -f "$REPORT_DIR/model_comparison_results.json" ]]; then
        echo "📈 Generating quick summary..."

        # Extract key metrics using jq (if available)
        if command -v jq >/dev/null 2>&1; then
            echo "" >> "$REPORT_DIR/README.md"
            echo "### Key Metrics" >> "$REPORT_DIR/README.md"
            echo "" >> "$REPORT_DIR/README.md"

            # Process each model's results
            jq -r 'keys[]' "$REPORT_DIR/model_comparison_results.json" | while read -r model; do
                local accuracy=$(jq -r ".[\"$model\"].Reasoning.ActionAccuracy" "$REPORT_DIR/model_comparison_results.json")
                local success_rate=$(jq -r ".[\"$model\"].Performance.SuccessfulRuns" "$REPORT_DIR/model_comparison_results.json")
                local total_requests=$(jq -r ".[\"$model\"].Performance.TotalRequests" "$REPORT_DIR/model_comparison_results.json")
                local avg_response=$(jq -r ".[\"$model\"].Performance.ResponseTime.Mean" "$REPORT_DIR/model_comparison_results.json")
                local rating=$(jq -r ".[\"$model\"].Summary.OverallRating" "$REPORT_DIR/model_comparison_results.json")

                if [[ "$accuracy" != "null" && "$success_rate" != "null" ]]; then
                    local success_percentage=$(echo "scale=1; $success_rate * 100 / $total_requests" | bc 2>/dev/null || echo "N/A")
                    echo "**$model**:" >> "$REPORT_DIR/README.md"
                    echo "- Overall Rating: $rating" >> "$REPORT_DIR/README.md"
                    echo "- Action Accuracy: $(echo "scale=1; $accuracy * 100" | bc 2>/dev/null || echo "N/A")%" >> "$REPORT_DIR/README.md"
                    echo "- Success Rate: $success_percentage%" >> "$REPORT_DIR/README.md"
                    echo "- Avg Response Time: $avg_response" >> "$REPORT_DIR/README.md"
                    echo "" >> "$REPORT_DIR/README.md"
                fi
            done
        fi
    fi

    echo "✅ Results organized in $REPORT_DIR"
}

# Function to display final summary
display_summary() {
    echo ""
    echo "==============================================="
    echo "📊 Model Comparison Test Summary"
    echo "==============================================="
    echo ""
    echo "📁 Results Location: $REPORT_DIR"
    echo ""
    echo "📋 Generated Files:"

    for file in "$REPORT_DIR"/*; do
        if [[ -f "$file" ]]; then
            local basename=$(basename "$file")
            local size=$(ls -lh "$file" | awk '{print $5}')
            echo "   • $basename ($size)"
        fi
    done

    echo ""
    echo "🔍 View Results:"
    echo "   cat $REPORT_DIR/model_comparison_report.md"
    echo "   cat $REPORT_DIR/model_recommendation.md"
    echo ""
    echo "📊 Raw Data:"
    echo "   cat $REPORT_DIR/model_comparison_results.json | jq ."
    echo ""

    if [[ -f "$REPORT_DIR/model_recommendation.md" ]]; then
        echo "🎯 Recommendation:"
        cat "$REPORT_DIR/model_recommendation.md" | head -5
        echo ""
    fi
}

# Function to cleanup on script exit
cleanup() {
    echo ""
    echo "🧹 Cleaning up..."

    # Remove any temporary files
    rm -f model_comparison_*.tmp 2>/dev/null || true

    # Unset environment variables
    unset MODEL_COMPARISON_RESULTS_DIR
    unset MODEL_COMPARISON_TIMESTAMP

    echo "✅ Cleanup complete"
}

# Main function
main() {
    echo "==============================================="
    echo "🧪 Model Comparison Test Execution"
    echo "==============================================="

    # Check if running from correct directory
    if [[ ! -f "go.mod" ]] || [[ ! -d "test/integration" ]]; then
        echo "❌ Please run this script from the project root directory"
        exit 1
    fi

    # Check if model comparison test exists
    if [[ ! -d "test/integration/model_comparison" ]]; then
        echo "❌ Model comparison test directory not found"
        echo "   Expected: test/integration/model_comparison/"
        exit 1
    fi

    # Check dependencies
    if ! command -v jq >/dev/null 2>&1; then
        echo "⚠️  jq not found - some result processing features will be limited"
        echo "   Install with: brew install jq (macOS) or apt-get install jq (Linux)"
    fi

    if ! command -v bc >/dev/null 2>&1; then
        echo "⚠️  bc not found - some calculations will be limited"
        echo "   Install with: brew install bc (macOS) or apt-get install bc (Linux)"
    fi

    # Setup test environment
    setup_test_env

    # Check server health
    if ! check_servers_health; then
        echo ""
        echo "💡 To start model servers:"
        echo "   ./scripts/setup_model_comparison.sh"
        exit 1
    fi

    echo ""
    echo "🚀 Starting model comparison tests..."
    echo "   This may take 15-30 minutes depending on model response times"
    echo ""

    # Run tests with retry
    if run_tests_with_retry; then
        echo ""
        echo "🎉 Model comparison tests completed successfully!"

        # Collect and organize results
        collect_results

        # Display summary
        display_summary

    else
        echo ""
        echo "💥 Model comparison tests failed!"
        echo ""
        echo "🔍 Troubleshooting:"
        echo "   1. Check server health: ./scripts/setup_model_comparison.sh"
        echo "   2. Check logs in logs/ directory"
        echo "   3. Verify Go test can run: go test $TEST_PACKAGE -v -short"
        echo ""
        exit 1
    fi
}

# Setup cleanup trap
trap cleanup EXIT INT TERM

# Run main function
main "$@"
