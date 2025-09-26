#!/bin/bash
# integration-check-runtime-path.sh - Verify code executes in normal workflows
INTERFACE_METHOD="$1"  # e.g., "AnalyzeContext"

if [ -z "$INTERFACE_METHOD" ]; then
    echo "❌ ERROR: Interface method required"
    echo "Usage: $0 <MethodName>"
    exit 1
fi

echo "🔍 Checking runtime execution path for $INTERFACE_METHOD..."
# Check if method is called in workflow processors
WORKFLOW_USAGE=$(grep -r "$INTERFACE_METHOD" pkg/workflow/ pkg/processor/ pkg/api/ --include="*.go" | wc -l)

if [ "$WORKFLOW_USAGE" -eq 0 ]; then
    echo "❌ RUNTIME PATH FAILURE: $INTERFACE_METHOD not found in workflow execution paths"
    echo "📁 Checked directories: pkg/workflow/, pkg/processor/, pkg/api/"
    echo "🔧 Required: Ensure method is called during normal business workflows"
    echo ""
    echo "Example integration:"
    echo "// In processor.go or workflow engine:"
    echo "result, err := llmClient.$INTERFACE_METHOD(ctx, input)"
    echo "if err != nil {"
    echo "    return handleError(err)"
    echo "}"
    exit 1
fi

echo "✅ Runtime path verified: $INTERFACE_METHOD found in $WORKFLOW_USAGE workflow files"
