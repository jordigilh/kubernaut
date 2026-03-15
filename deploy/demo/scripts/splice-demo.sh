#!/usr/bin/env bash
# Assemble a final demo video from multiple raw VHS recordings using splice.conf.
#
# Unlike postprocess-demo.sh (single source), this reads from multiple raw MP4s
# (pods, rr, screens) and assembles them in an arbitrary narrative order defined
# by splice.conf.
#
# Usage:
#   bash deploy/demo/scripts/splice-demo.sh <scenario-name>
#
# Expects (under deploy/demo/scenarios/<name>/):
#   splice.conf                  (assembly manifest)
#   <name>-screens-raw.mp4       (narration screens tape)
#   <name>-pods-raw.mp4          (resource watcher tape)
#   <name>-rr-raw.mp4            (RR lifecycle tape)
#
# Produces:
#   <name>.mp4                   (final assembled video)
#
# splice.conf format:
#   # Scene-index mode (uses detect-scenes.sh for boundaries):
#   <source>  <scene_index>  [action]  [param]
#
#   # Timestamp mode (direct start/end in seconds):
#   <source>  time  <start_sec>  <end_sec>
#
#   Sources: screens, pods, rr (mapped to <name>-<source>-raw.mp4)
#
#   Actions (scene-index mode):
#     keep          - include the full scene (default)
#     keep_first N  - include only the first N seconds
#     keep_last N   - include only the last N seconds
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

SCENARIO="${1:?Usage: splice-demo.sh <scenario-name> [subdir]}"
SUBDIR="${2:-recording}"
SCENARIO_DIR="${REPO_ROOT}/deploy/demo/scenarios/${SCENARIO}/${SUBDIR}"
SPLICE_CONF="${SCENARIO_DIR}/splice.conf"
FINAL_MP4="${SCENARIO_DIR}/${SCENARIO}.mp4"

if [ ! -f "${SPLICE_CONF}" ]; then
  echo "Error: splice config not found: ${SPLICE_CONF}" >&2
  exit 1
fi

echo "═══════════════════════════════════════════"
echo "  splice-demo.sh: ${SCENARIO}"
echo "═══════════════════════════════════════════"
echo ""

# Discover which sources are referenced in splice.conf and find their raw MP4s.
# Build a source->path mapping file for the Python stage.
SOURCE_MAP=$(mktemp)

SOURCES=$(python3 -c "
sources = set()
with open('${SPLICE_CONF}') as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        if '#' in line:
            line = line[:line.index('#')].strip()
        if line:
            sources.add(line.split()[0])
for s in sorted(sources):
    print(s)
")

for src in $SOURCES; do
  raw="${SCENARIO_DIR}/${SCENARIO}-${src}-raw.mp4"
  if [ ! -f "$raw" ]; then
    echo "Error: raw recording not found for source '${src}': ${raw}" >&2
    rm -f "${SOURCE_MAP}"
    exit 1
  fi
  echo "==> Detecting scenes in ${src} tape..."
  bash "${SCRIPT_DIR}/detect-scenes.sh" "$raw"
  scenes_json="${raw}.scenes.json"
  if [ ! -f "$scenes_json" ]; then
    echo "Error: scene detection failed for ${src}" >&2
    rm -f "${SOURCE_MAP}"
    exit 1
  fi
  echo "${src} ${raw} ${scenes_json}" >> "${SOURCE_MAP}"
  echo ""
done

if [ ! -s "${SOURCE_MAP}" ]; then
  echo "Error: no sources found or scene detection failed" >&2
  rm -f "${SOURCE_MAP}"
  exit 1
fi

echo "==> Assembling from splice.conf..."

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR" "${SOURCE_MAP}"' EXIT

python3 << PYEOF
import json, subprocess, sys, os

splice_conf = "${SPLICE_CONF}"
final_mp4 = "${FINAL_MP4}"
tmpdir = "${TMPDIR}"
source_map_file = "${SOURCE_MAP}"

# Load source mapping
mp4_map = {}
scenes_json_map = {}
with open(source_map_file) as f:
    for line in f:
        parts = line.strip().split()
        if len(parts) == 3:
            src, mp4_path, json_path = parts
            mp4_map[src] = mp4_path
            scenes_json_map[src] = json_path

# Load scene data for each source
scene_data = {}
for src, json_path in scenes_json_map.items():
    with open(json_path) as f:
        scene_data[src] = json.load(f)

# Parse splice.conf into ordered segment list
segments = []  # (source_mp4, start, end, source_name, label)
with open(splice_conf) as f:
    for line_num, line in enumerate(f, 1):
        raw_line = line.strip()
        if '#' in raw_line:
            raw_line = raw_line[:raw_line.index('#')].strip()
        if not raw_line:
            continue

        parts = raw_line.split()
        source = parts[0]

        if source not in mp4_map:
            print(f"Error: unknown source '{source}' on line {line_num}", file=sys.stderr)
            sys.exit(1)

        raw_mp4 = mp4_map[source]

        if len(parts) >= 2 and parts[1] == 'time':
            if len(parts) < 4:
                print(f"Error: 'time' mode requires start and end on line {line_num}", file=sys.stderr)
                sys.exit(1)
            start = float(parts[2])
            end = float(parts[3])
            segments.append((raw_mp4, start, end, source, f"time {start:.1f}-{end:.1f}"))
        else:
            scene_idx = int(parts[1])
            action = parts[2] if len(parts) > 2 else 'keep'
            param = float(parts[3]) if len(parts) > 3 else None

            scenes = scene_data.get(source, [])
            scene = None
            for s in scenes:
                if s['scene'] == scene_idx:
                    scene = s
                    break

            if scene is None:
                print(f"Error: scene {scene_idx} not found in {source} "
                      f"(has {len(scenes)} scenes) on line {line_num}", file=sys.stderr)
                sys.exit(1)

            start = scene['start']
            end = scene['end']

            if action == 'keep':
                pass
            elif action == 'keep_first':
                end = min(end, start + param)
            elif action == 'keep_last':
                start = max(start, end - param)
            else:
                print(f"Warning: unknown action '{action}' on line {line_num}, "
                      f"keeping full scene", file=sys.stderr)

            label = f"scene {scene_idx}"
            if action != 'keep':
                label += f" {action} {param:.0f}"
            segments.append((raw_mp4, start, end, source, label))

if not segments:
    print("Error: no segments defined in splice.conf", file=sys.stderr)
    sys.exit(1)

# Report
total_duration = sum(e - s for _, s, e, _, _ in segments)
print(f"  Segments: {len(segments)}")
print(f"  Estimated duration: {total_duration:.1f}s")
print()

for i, (mp4, start, end, source, label) in enumerate(segments):
    dur = end - start
    ms, ss = int(start // 60), start % 60
    me, se = int(end // 60), end % 60
    print(f"    [{i:2d}] {source:8s}  {ms:02d}:{ss:05.2f} - {me:02d}:{se:05.2f}  ({dur:.2f}s)  {label}")

print()

# Extract each segment as a TS file
segment_files = []
for i, (mp4, start, end, source, label) in enumerate(segments):
    seg_path = os.path.join(tmpdir, f"seg_{i:03d}.ts")
    duration = end - start

    cmd = [
        "ffmpeg", "-y",
        "-ss", f"{start:.3f}",
        "-i", mp4,
        "-t", f"{duration:.3f}",
        "-c:v", "libx264",
        "-preset", "fast",
        "-crf", "18",
        "-pix_fmt", "yuv420p",
        "-an",
        "-f", "mpegts",
        seg_path
    ]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"Error extracting segment {i} ({source} {label}):", file=sys.stderr)
        print(result.stderr[-500:], file=sys.stderr)
        sys.exit(1)
    segment_files.append(seg_path)
    sys.stdout.write(f"\r  Extracted {i + 1}/{len(segments)} segments...")
    sys.stdout.flush()

print("\n")

# Concatenate all segments
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

# Report final duration
probe_cmd = [
    "ffprobe", "-v", "quiet",
    "-show_entries", "format=duration",
    "-of", "csv=p=0",
    final_mp4
]
final_dur = subprocess.run(probe_cmd, capture_output=True, text=True).stdout.strip()
print(f"  Output: {final_mp4}")
print(f"  Duration: {final_dur}s")
PYEOF

echo ""
echo "==> Splice complete."
