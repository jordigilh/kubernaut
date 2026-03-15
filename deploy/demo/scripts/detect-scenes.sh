#!/usr/bin/env bash
# Detect blank-frame scene boundaries in a VHS-recorded MP4.
#
# Every `clear` command in a VHS tape produces a blank frame (terminal background
# only, Catppuccin Mocha: #1E1E2E).  Polling waits produce long blank intervals
# (idle terminal).
#
# This script:
#   1. Finds blank-frame intervals via ffmpeg blackdetect
#   2. Merges intervals < 0.1s apart (double-clears from sentinel + scene start)
#   3. Categorizes: short (< 1s) = transition boundary, long (>= 1s) = wait scene
#   4. Builds a scene list where wait scenes can be cut by postprocess-demo.sh
#
# Usage: bash deploy/demo/scripts/detect-scenes.sh <raw-mp4> [--json]
#
# The scene list is written to <raw-mp4>.scenes.json.
set -euo pipefail

MP4_PATH="${1:?Usage: detect-scenes.sh <raw-mp4> [--json]}"
JSON_FLAG="${2:-}"

if [ ! -f "${MP4_PATH}" ]; then
  echo "Error: file not found: ${MP4_PATH}" >&2
  exit 1
fi

SCENES_JSON="${MP4_PATH}.scenes.json"

DURATION=$(ffprobe -v quiet -show_entries format=duration -of csv=p=0 "${MP4_PATH}")

# Detect blank frames tuned for Catppuccin Mocha dark theme.
# pix_th=0.15: per-pixel luminance threshold (bg #1E1E2E ≈ 0.125)
# pic_th=0.998: 99.8% of pixels must be below threshold → catches only truly blank frames
# d=0.05: minimum 50ms blank duration
BOUNDARIES=$(ffmpeg -i "${MP4_PATH}" \
  -vf "blackdetect=d=0.05:pix_th=0.15:pic_th=0.998" \
  -an -f null - 2>&1 \
  | grep 'blackdetect' \
  | sed -n 's/.*black_start:\([0-9.]*\).*black_end:\([0-9.]*\).*/\1 \2/p' || true)

python3 << PYEOF
import json, sys

duration = float(${DURATION})
scenes_json = "${SCENES_JSON}"
json_flag = "${JSON_FLAG}"

raw = """${BOUNDARIES}"""

intervals = []
for line in raw.strip().split('\n'):
    if not line.strip():
        continue
    parts = line.strip().split()
    if len(parts) >= 2:
        intervals.append((float(parts[0]), float(parts[1])))

if not intervals:
    print("Warning: no blank frames detected.", file=sys.stderr)
    scenes = [{"scene": 0, "start": 0.0, "end": duration, "duration": round(duration, 3), "type": "content"}]
    with open(scenes_json, 'w') as f:
        json.dump(scenes, f, indent=2)
    print(json.dumps(scenes, indent=2))
    sys.exit(0)

# Merge intervals with gaps < 0.1s
merged = [intervals[0]]
for start, end in intervals[1:]:
    ps, pe = merged[-1]
    if start - pe < 0.1:
        merged[-1] = (ps, max(pe, end))
    else:
        merged.append((start, end))

# Build boundaries: short blanks (< 1s) → single midpoint boundary,
# long blanks (>= 1s) → two boundaries (start and end), creating a "wait" scene.
LONG_THRESHOLD = 1.0
boundaries = []
for start, end in merged:
    dur = end - start
    if dur < LONG_THRESHOLD:
        boundaries.append(("transition", (start + end) / 2.0))
    else:
        boundaries.append(("wait_start", start))
        boundaries.append(("wait_end", end))

# Build scenes from boundaries
scenes = []
prev = 0.0
scene_idx = 0
i = 0
while i < len(boundaries):
    btype, btime = boundaries[i]

    if btype == "transition":
        if btime > prev:
            scenes.append({
                "scene": scene_idx,
                "start": round(prev, 3),
                "end": round(btime, 3),
                "duration": round(btime - prev, 3),
                "type": "content"
            })
            scene_idx += 1
        prev = btime
        i += 1

    elif btype == "wait_start":
        # Content scene before the wait
        if btime > prev:
            scenes.append({
                "scene": scene_idx,
                "start": round(prev, 3),
                "end": round(btime, 3),
                "duration": round(btime - prev, 3),
                "type": "content"
            })
            scene_idx += 1

        # The wait scene itself (wait_start → wait_end)
        if i + 1 < len(boundaries) and boundaries[i + 1][0] == "wait_end":
            wend = boundaries[i + 1][1]
            scenes.append({
                "scene": scene_idx,
                "start": round(btime, 3),
                "end": round(wend, 3),
                "duration": round(wend - btime, 3),
                "type": "wait"
            })
            scene_idx += 1
            prev = wend
            i += 2
        else:
            prev = btime
            i += 1
    else:
        i += 1

# Final scene
if prev < duration - 0.1:
    scenes.append({
        "scene": scene_idx,
        "start": round(prev, 3),
        "end": round(duration, 3),
        "duration": round(duration - prev, 3),
        "type": "content"
    })

with open(scenes_json, 'w') as f:
    json.dump(scenes, f, indent=2)

if json_flag == "--json":
    print(json.dumps(scenes, indent=2))
else:
    n_content = sum(1 for s in scenes if s['type'] == 'content')
    n_wait = sum(1 for s in scenes if s['type'] == 'wait')
    print(f"Detected {len(merged)} blank intervals -> {len(scenes)} scenes ({n_content} content, {n_wait} wait)")
    print(f"Video duration: {duration:.1f}s")
    print()
    for s in scenes:
        ms, ss = int(s['start'] // 60), s['start'] % 60
        me, se = int(s['end'] // 60), s['end'] % 60
        tag = "  WAIT" if s['type'] == 'wait' else ""
        print(f"  Scene {s['scene']:2d}:  {ms:02d}:{ss:05.2f} - {me:02d}:{se:05.2f}   ({s['duration']:6.2f}s){tag}")
    print()
    print(f"Scene data written to: {scenes_json}")
PYEOF
