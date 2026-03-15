#!/usr/bin/env bash
# Post-process a raw VHS recording using scene-based rules.
#
# 1. Runs detect-scenes.sh to find blank-frame scene boundaries
# 2. Reads a scenes.conf file for per-scene rules
# 3. Uses ffmpeg to assemble the final trimmed MP4
#
# Usage:
#   bash deploy/demo/scripts/postprocess-demo.sh <scenario-name>
#
# Expects:
#   deploy/demo/scenarios/<name>/<name>-raw.mp4   (raw VHS recording)
#   deploy/demo/scenarios/<name>/scenes.conf      (scene rules)
#
# Produces:
#   deploy/demo/scenarios/<name>/<name>.mp4       (final trimmed video)
#
# scenes.conf format:
#   # scene_index  action  [params]
#   0   keep
#   5   cut
#   18  keep_last 5
#   19  keep_first 3
#   20  extend 12
#
# Actions:
#   keep          - include the full scene
#   cut           - remove the scene entirely
#   keep_last N   - include only the last N seconds of the scene
#   keep_first N  - include only the first N seconds of the scene
#   extend N      - include full scene; hold last frame until at least N seconds total
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

SCENARIO="${1:?Usage: postprocess-demo.sh <scenario-name> [subdir]}"
SUBDIR="${2:-recording}"
SCENARIO_DIR="${REPO_ROOT}/deploy/demo/scenarios/${SCENARIO}/${SUBDIR}"
RAW_MP4="${SCENARIO_DIR}/${SCENARIO}-raw.mp4"
SCENES_CONF="${SCENARIO_DIR}/scenes.conf"
FINAL_MP4="${SCENARIO_DIR}/${SCENARIO}.mp4"

if [ ! -f "${RAW_MP4}" ]; then
  echo "Error: raw recording not found: ${RAW_MP4}" >&2
  exit 1
fi

if [ ! -f "${SCENES_CONF}" ]; then
  echo "Error: scenes config not found: ${SCENES_CONF}" >&2
  exit 1
fi

echo "==> Detecting scenes in ${RAW_MP4}..."
bash "${SCRIPT_DIR}/detect-scenes.sh" "${RAW_MP4}"
SCENES_JSON="${RAW_MP4}.scenes.json"

if [ ! -f "${SCENES_JSON}" ]; then
  echo "Error: scene detection failed -- no ${SCENES_JSON}" >&2
  exit 1
fi

echo "==> Applying scene rules from ${SCENES_CONF}..."

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

python3 << PYEOF
import json, subprocess, sys, os

scenes_json_path = "${SCENES_JSON}"
scenes_conf_path = "${SCENES_CONF}"
raw_mp4 = "${RAW_MP4}"
final_mp4 = "${FINAL_MP4}"
tmpdir = "${TMPDIR}"

with open(scenes_json_path) as f:
    scenes = json.load(f)

# Parse scenes.conf
rules = {}
with open(scenes_conf_path) as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        parts = line.split()
        idx = int(parts[0])
        action = parts[1]
        param = float(parts[2]) if len(parts) > 2 else None
        rules[idx] = (action, param)

# Default: keep content scenes, cut wait scenes (unless overridden)
for s in scenes:
    if s['scene'] not in rules:
        if s.get('type') == 'wait':
            rules[s['scene']] = ('cut', None)
        else:
            rules[s['scene']] = ('keep', None)

# Build segment list: (start, end, extend_to_or_None)
segments = []
for s in scenes:
    idx = s['scene']
    action, param = rules.get(idx, ('keep', None))
    start = s['start']
    end = s['end']

    if action == 'cut':
        continue
    elif action == 'keep':
        segments.append((start, end, None))
    elif action == 'keep_last':
        n = param
        seg_start = max(start, end - n)
        segments.append((seg_start, end, None))
    elif action == 'keep_first':
        n = param
        seg_end = min(end, start + n)
        segments.append((start, seg_end, None))
    elif action == 'extend':
        segments.append((start, end, param))
    else:
        print(f"Warning: unknown action '{action}' for scene {idx}, keeping full scene", file=sys.stderr)
        segments.append((start, end, None))

if not segments:
    print("Error: no segments to keep -- check scenes.conf", file=sys.stderr)
    sys.exit(1)

# Report
total_raw = scenes[-1]['end'] if scenes else 0
total_final = sum(max(e - s, ext or 0) for s, e, ext in segments)
print(f"  Raw duration:   {total_raw:.1f}s")
print(f"  Final duration: {total_final:.1f}s  (with extend padding)")
print(f"  Segments: {len(segments)}")
print()

for i, (start, end, ext) in enumerate(segments):
    ms, ss = int(start // 60), start % 60
    me, se = int(end // 60), end % 60
    actual = end - start
    ext_label = f" -> extend to {ext:.0f}s" if ext else ""
    print(f"    [{i:2d}] {ms:02d}:{ss:05.2f} - {me:02d}:{se:05.2f}  ({actual:.2f}s{ext_label})")

# Extract each segment as a separate TS, then concatenate.
segment_files = []
for i, (start, end, extend_to) in enumerate(segments):
    seg_path = os.path.join(tmpdir, f"seg_{i:03d}.ts")
    duration = end - start

    vf_filters = []
    pad = 0
    if extend_to is not None and duration < extend_to:
        pad = extend_to - duration
        vf_filters.append(f"tpad=stop_mode=clone:stop_duration={pad:.3f}")

    # When tpad is used, -t must be an INPUT option (before -i) so it only
    # limits reading; the padded frames are then added by the filter.
    # Without tpad, -t goes after -i as an output option.
    if vf_filters:
        cmd = [
            "ffmpeg", "-y",
            "-ss", f"{start:.3f}",
            "-t", f"{duration:.3f}",
            "-i", raw_mp4,
            "-vf", ",".join(vf_filters),
        ]
    else:
        cmd = [
            "ffmpeg", "-y",
            "-ss", f"{start:.3f}",
            "-i", raw_mp4,
            "-t", f"{duration:.3f}",
        ]
    cmd.extend([
        "-c:v", "libx264",
        "-preset", "fast",
        "-crf", "18",
        "-pix_fmt", "yuv420p",
        "-an",
        "-f", "mpegts",
        seg_path
    ])
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"Error extracting segment {i}: {result.stderr[-500:]}", file=sys.stderr)
        sys.exit(1)
    segment_files.append(seg_path)

# Concatenate using concat protocol
concat_input = "|".join(segment_files)
cmd = [
    "ffmpeg", "-y",
    "-i", f"concat:{concat_input}",
    "-c:v", "libx264",
    "-preset", "slow",
    "-crf", "18",
    "-pix_fmt", "yuv420p",
    "-movflags", "+faststart",
    "-an",
    final_mp4
]
result = subprocess.run(cmd, capture_output=True, text=True)
if result.returncode != 0:
    print(f"Error concatenating: {result.stderr[-500:]}", file=sys.stderr)
    sys.exit(1)

# Report final
probe_cmd = ["ffprobe", "-v", "quiet", "-show_entries", "format=duration", "-of", "csv=p=0", final_mp4]
final_dur = subprocess.run(probe_cmd, capture_output=True, text=True).stdout.strip()
print()
print(f"  Output: {final_mp4}")
print(f"  Duration: {final_dur}s")
PYEOF

echo "==> Post-processing complete."
