#!/bin/bash
# HolmesGPT API Runtime Configuration Examples
# This script demonstrates how to run the HolmesGPT API container with different model endpoints

set -e

CONTAINER_IMAGE="holmesgpt-api:localhost-8010"

echo "🚀 HolmesGPT API Runtime Configuration Examples"
echo "=============================================="

# Function to stop any running HolmesGPT containers
cleanup() {
    echo "🧹 Cleaning up existing containers..."
    podman stop $(podman ps -q --filter "ancestor=$CONTAINER_IMAGE") 2>/dev/null || true
    podman rm $(podman ps -aq --filter "ancestor=$CONTAINER_IMAGE") 2>/dev/null || true
}

# Example 1: Local model on port 8010
run_localhost_8010() {
    echo ""
    echo "📍 Example 1: Local model on localhost:8010"
    echo "Usage: Your current setup"

    podman run -d \
        --name holmesgpt-localhost-8010 \
        --network host \
        -e HOLMESGPT_LLM_BASE_URL=http://localhost:8010 \
        -e HOLMESGPT_LLM_PROVIDER=ollama \
        -e HOLMESGPT_LLM_MODEL=llama3.2 \
        -e HOLMESGPT_LOG_LEVEL=INFO \
        $CONTAINER_IMAGE

    echo "✅ Started: HolmesGPT API → localhost:8010"
    echo "   API: http://localhost:8090"
}

# Example 2: Ollama default port
run_ollama_default() {
    echo ""
    echo "📍 Example 2: Ollama on default port 11434"
    echo "Usage: Standard Ollama installation"

    podman run -d \
        --name holmesgpt-ollama-11434 \
        --network host \
        -e HOLMESGPT_LLM_BASE_URL=http://localhost:11434 \
        -e HOLMESGPT_LLM_PROVIDER=ollama \
        -e HOLMESGPT_LLM_MODEL=llama3.2 \
        -e HOLMESGPT_PORT=8091 \
        $CONTAINER_IMAGE

    echo "✅ Started: HolmesGPT API → localhost:11434"
    echo "   API: http://localhost:8091"
}

# Example 3: Remote model server
run_remote_model() {
    echo ""
    echo "📍 Example 3: Remote model server"
    echo "Usage: Model running on another machine"

    podman run -d \
        --name holmesgpt-remote-model \
        --network host \
        -e HOLMESGPT_LLM_BASE_URL=http://192.168.1.169:8080 \
        -e HOLMESGPT_LLM_PROVIDER=ollama \
        -e HOLMESGPT_LLM_MODEL=llama3.2 \
        -e HOLMESGPT_PORT=8092 \
        $CONTAINER_IMAGE

    echo "✅ Started: HolmesGPT API → 192.168.1.169:8080"
    echo "   API: http://localhost:8092"
}

# Example 4: OpenAI-compatible API
run_openai_compatible() {
    echo ""
    echo "📍 Example 4: OpenAI-compatible API"
    echo "Usage: OpenAI API or compatible service"

    podman run -d \
        --name holmesgpt-openai-api \
        --network host \
        -e HOLMESGPT_LLM_BASE_URL=https://api.openai.com/v1 \
        -e HOLMESGPT_LLM_PROVIDER=openai \
        -e HOLMESGPT_LLM_MODEL=gpt-4 \
        -e HOLMESGPT_LLM_API_KEY=your-api-key-here \
        -e HOLMESGPT_PORT=8093 \
        $CONTAINER_IMAGE

    echo "✅ Started: HolmesGPT API → OpenAI API"
    echo "   API: http://localhost:8093"
}

# Show running containers
show_status() {
    echo ""
    echo "📊 Current HolmesGPT Containers:"
    echo "================================"
    podman ps --filter "ancestor=$CONTAINER_IMAGE" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

    echo ""
    echo "🔗 Health Check Commands:"
    echo "curl http://localhost:8090/health  # localhost:8010 model"
    echo "curl http://localhost:8091/health  # localhost:11434 model"
    echo "curl http://localhost:8092/health  # remote model"
    echo "curl http://localhost:8093/health  # OpenAI API"
}

# Main menu
case "${1:-menu}" in
    "cleanup")
        cleanup
        ;;
    "localhost-8010")
        cleanup
        run_localhost_8010
        show_status
        ;;
    "ollama-default")
        cleanup
        run_ollama_default
        show_status
        ;;
    "remote")
        cleanup
        run_remote_model
        show_status
        ;;
    "openai")
        cleanup
        run_openai_compatible
        show_status
        ;;
    "all")
        cleanup
        run_localhost_8010
        sleep 2
        run_ollama_default
        sleep 2
        run_remote_model
        sleep 2
        show_status
        ;;
    "menu"|*)
        echo ""
        echo "Available commands:"
        echo "  ./run-examples.sh localhost-8010   # Use your current model (localhost:8010)"
        echo "  ./run-examples.sh ollama-default   # Use Ollama on port 11434"
        echo "  ./run-examples.sh remote           # Use remote model server"
        echo "  ./run-examples.sh openai           # Use OpenAI API"
        echo "  ./run-examples.sh all              # Run multiple examples"
        echo "  ./run-examples.sh cleanup          # Stop all containers"
        echo ""
        echo "🔧 Manual Runtime Configuration:"
        echo "podman run -d --network host \\"
        echo "  -e HOLMESGPT_LLM_BASE_URL=http://your-model:port \\"
        echo "  -e HOLMESGPT_LLM_PROVIDER=ollama \\"
        echo "  -e HOLMESGPT_LLM_MODEL=your-model \\"
        echo "  -e HOLMESGPT_PORT=8090 \\"
        echo "  $CONTAINER_IMAGE"
        ;;
esac
