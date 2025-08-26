#!/bin/bash

# Download IBM Granite model for LocalAI
# This script downloads the GGUF format model from HuggingFace

set -e

MODEL_DIR="./models"
MODEL_NAME="granite-3.0-8b-instruct.gguf"
MODEL_URL="https://huggingface.co/bartowski/granite-3.0-8b-instruct-GGUF/resolve/main/granite-3.0-8b-instruct-Q4_K_M.gguf"

echo "Creating models directory..."
mkdir -p "$MODEL_DIR"

echo "Downloading IBM Granite 3.0 8B Instruct model (Q4_K_M quantization)..."
echo "This may take a while (model size: ~4.9GB)..."

# Check if model already exists
if [ -f "$MODEL_DIR/$MODEL_NAME" ]; then
    echo "Model already exists at $MODEL_DIR/$MODEL_NAME"
    echo "Remove it manually if you want to re-download"
    exit 0
fi

# Download with progress bar
curl -L --progress-bar -o "$MODEL_DIR/$MODEL_NAME.tmp" "$MODEL_URL"

# Verify download completed successfully
if [ $? -eq 0 ]; then
    mv "$MODEL_DIR/$MODEL_NAME.tmp" "$MODEL_DIR/$MODEL_NAME"
    echo "✅ Model downloaded successfully to $MODEL_DIR/$MODEL_NAME"
    echo "Model size: $(du -h "$MODEL_DIR/$MODEL_NAME" | cut -f1)"
else
    echo "❌ Download failed"
    rm -f "$MODEL_DIR/$MODEL_NAME.tmp"
    exit 1
fi

echo ""
echo "🚀 Ready to start LocalAI with:"
echo "   docker-compose up localai"
echo ""
echo "📊 Model info:"
echo "   - Name: IBM Granite 3.0 8B Instruct"
echo "   - Format: GGUF (Q4_K_M quantization)"
echo "   - Size: ~4.9GB"
echo "   - Memory requirement: ~6-8GB RAM"