#!/bin/bash

# Convert markdown to PowerPoint (.pptx) with speaker notes
# Requires: pandoc (install via: brew install pandoc)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INPUT_FILE="$SCRIPT_DIR/slide-01-opening-pptx.md"
OUTPUT_FILE="$SCRIPT_DIR/slide-01-opening.pptx"

echo "🎯 Converting markdown to PowerPoint..."
echo "Input: $INPUT_FILE"
echo "Output: $OUTPUT_FILE"

# Check if pandoc is installed
if ! command -v pandoc &> /dev/null; then
    echo "❌ Error: pandoc is not installed"
    echo ""
    echo "Install pandoc using one of these methods:"
    echo "  macOS:   brew install pandoc"
    echo "  Linux:   apt-get install pandoc"
    echo "  Windows: choco install pandoc"
    echo ""
    echo "Or download from: https://pandoc.org/installing.html"
    exit 1
fi

# Convert markdown to PowerPoint
pandoc "$INPUT_FILE" \
    -o "$OUTPUT_FILE" \
    --slide-level=1 \
    -t pptx \
    --reference-doc="$SCRIPT_DIR/template.pptx" 2>/dev/null || \
pandoc "$INPUT_FILE" \
    -o "$OUTPUT_FILE" \
    --slide-level=1 \
    -t pptx

echo "✅ Conversion complete!"
echo ""
echo "📄 Output file: $OUTPUT_FILE"
echo ""
echo "Next steps:"
echo "  1. Open $OUTPUT_FILE in PowerPoint or LibreOffice"
echo "  2. Upload to Google Drive"
echo "  3. Right-click → Open with → Google Slides"
echo ""
echo "Note: Speaker notes are included in the 'Notes' section of each slide"


