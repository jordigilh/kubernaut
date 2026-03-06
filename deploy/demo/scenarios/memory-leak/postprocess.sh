#!/usr/bin/env bash
# ════════════════════════════════════════════════════════════════════════════════
# Post-processing script for memory-leak demo recording
#
# VHS records the scenario linearly (pod transition before AI analysis) because
# the transition is time-sensitive and must be captured live. This script uses
# ffmpeg to reorder the sections so the final video shows:
#   AI analysis → pod transition (the desired narrative order)
#
# Reads timestamp markers written by the VHS tape to /tmp/vhs-marker-* files
# to compute the exact cut points in the video.
#
# Usage: ./postprocess.sh [raw-mp4] [output-mp4]
#   Defaults: raw-mp4 = memory-leak.mp4, output-mp4 = memory-leak.mp4
# ════════════════════════════════════════════════════════════════════════════════

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RAW_MP4="${1:-${SCRIPT_DIR}/memory-leak.mp4}"
OUTPUT_MP4="${2:-${SCRIPT_DIR}/memory-leak.mp4}"
OUTPUT_GIF="${OUTPUT_MP4%.mp4}.gif"

# Read markers
read_marker() {
    local file="/tmp/vhs-marker-$1"
    if [[ ! -f "$file" ]]; then
        echo "ERROR: Marker file $file not found" >&2
        exit 1
    fi
    cat "$file"
}

VIDEO_START=$(read_marker "video-start")
T_START=$(read_marker "T-start")
T_END=$(read_marker "T-end")
A_START=$(read_marker "A-start")
A_END=$(read_marker "A-end")

# Compute video-relative offsets (seconds from video start)
offset() {
    python3 -c "print(f'{$1 - $VIDEO_START:.3f}')"
}

T_START_S=$(offset "$T_START")
T_END_S=$(offset "$T_END")
A_START_S=$(offset "$A_START")
A_END_S=$(offset "$A_END")

echo "═══════════════════════════════════════════════"
echo "Post-processing: reorder AI analysis before transition"
echo "═══════════════════════════════════════════════"
echo "Video start:        0.000s"
echo "Transition (T):     ${T_START_S}s → ${T_END_S}s"
echo "AI Analysis (A):    ${A_START_S}s → ${A_END_S}s"
echo ""
echo "Reorder: [0..T_START] [A_START..A_END] [T_START..T_END] [A_END..end]"
echo "═══════════════════════════════════════════════"

# Get total duration
DURATION=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$RAW_MP4")

# Use a temp file for the reordered output
TEMP_MP4="/tmp/memory-leak-reordered.mp4"

# ffmpeg filter_complex to reorder sections:
#   Part 1: [0, T_START)        — everything before transition
#   Part 2: [A_START, A_END)    — AI analysis (moved here)
#   Part 3: [T_START, T_END)    — pod transition (moved here)
#   Part 4: [A_END, end]        — everything after AI analysis
ffmpeg -y -i "$RAW_MP4" \
    -filter_complex "
        [0:v]trim=start=0:end=${T_START_S},setpts=PTS-STARTPTS[p1];
        [0:v]trim=start=${A_START_S}:end=${A_END_S},setpts=PTS-STARTPTS[p2];
        [0:v]trim=start=${T_START_S}:end=${T_END_S},setpts=PTS-STARTPTS[p3];
        [0:v]trim=start=${A_END_S},setpts=PTS-STARTPTS[p4];
        [p1][p2][p3][p4]concat=n=4:v=1:a=0[out]
    " \
    -map "[out]" \
    -c:v libx264 -pix_fmt yuv420p -movflags +faststart -crf 23 -preset medium \
    "$TEMP_MP4" 2>/dev/null

mv -f "$TEMP_MP4" "$OUTPUT_MP4"
echo "✅ MP4 created: $OUTPUT_MP4"

# Generate GIF from the reordered MP4
echo "Creating GIF..."
ffmpeg -y -i "$OUTPUT_MP4" \
    -vf "fps=10,scale=1200:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=256[p];[s1][p]paletteuse=dither=bayer" \
    "$OUTPUT_GIF" 2>/dev/null

echo "✅ GIF created: $OUTPUT_GIF"
echo ""
echo "Done. Sections reordered: AI analysis shown before pod transition."
