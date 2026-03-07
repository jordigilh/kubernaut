#!/usr/bin/env bash
# Fix MP4 timing for VHS-generated recordings.
#
# VHS generates GIF frames with variable delays (including very long ones for
# static Sleep periods). The GIF plays correctly, but VHS's MP4 encoder and
# ffmpeg both ignore long frame delays, producing sped-up MP4s.
#
# This script extracts frame-accurate timing from the GIF binary, duplicates
# long-delay frames to fill the time at 25fps, and produces a correctly-timed MP4.
#
# Usage: bash deploy/demo/scripts/fix-mp4-timing.sh <path-to-gif> [output-mp4]
set -euo pipefail

GIF_PATH="${1:?Usage: fix-mp4-timing.sh <gif-path> [output-mp4]}"
MP4_PATH="${2:-${GIF_PATH%.gif}.mp4}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

python3 << PYEOF
import struct, os, subprocess, sys
from PIL import Image

gif_path = "${GIF_PATH}"
tmpdir = "${TMPDIR}"
fps = 25
frame_ms = 1000 // fps  # 40ms

# Parse frame delays from raw GIF binary (PIL doesn't read variable delays)
with open(gif_path, 'rb') as f:
    data = f.read()

delays_cs = []
pos = 0
while True:
    pos = data.find(b'\x21\xf9\x04', pos + 1)
    if pos == -1:
        break
    delay = struct.unpack('<H', data[pos+4:pos+6])[0]
    delays_cs.append(delay)

# Extract frames with PIL (just for the image data)
gif = Image.open(gif_path)
frames = []
try:
    while True:
        frames.append(gif.copy())
        gif.seek(gif.tell() + 1)
except EOFError:
    pass

# Align: delays_cs may have one extra entry (trailing GCE without image)
n = min(len(frames), len(delays_cs))
print(f"GIF: {n} frames, {len(delays_cs)} delay entries")

concat_lines = []
total_out = 0

for i in range(n):
    delay_ms = delays_cs[i] * 10  # centiseconds -> milliseconds
    if delay_ms < frame_ms:
        delay_ms = frame_ms

    n_copies = max(1, round(delay_ms / frame_ms))

    png_path = os.path.join(tmpdir, f"frame_{i:06d}.png")
    frames[i].save(png_path)

    for _ in range(n_copies):
        concat_lines.append(f"file '{png_path}'")
        concat_lines.append(f"duration {1.0/fps:.6f}")
        total_out += 1

concat_path = os.path.join(tmpdir, "concat.txt")
with open(concat_path, 'w') as f:
    f.write('\n'.join(concat_lines) + '\n')
    f.write(f"file '{png_path}'\n")

print(f"Output: {total_out} frames at {fps}fps = {total_out/fps:.1f}s")
PYEOF

echo "Converting to MP4..."
ffmpeg -y -f concat -safe 0 -i "${TMPDIR}/concat.txt" \
  -c:v libx264 -pix_fmt yuv420p -preset slow -crf 18 \
  "${MP4_PATH}" 2>/dev/null

DURATION=$(ffprobe -v quiet -show_entries format=duration -of csv=p=0 "${MP4_PATH}")
echo "Done: ${MP4_PATH} (${DURATION}s)"
