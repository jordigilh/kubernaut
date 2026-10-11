#!/bin/bash
set -euo pipefail
# BR-PLATFORM-001 / #2502: bounded owned-node evidence is supplied by the lane.
source_dir="${FLEET_DIAGNOSTICS_DIR:-}"
[[ -n "${source_dir}" ]] || exit 0
[[ -d "${source_dir}" ]] || { echo "Warning: owned Fleet diagnostic directory is unavailable"; exit 1; }
output_dir="${1:?collection directory required}/fleet-service-path"
mkdir -p "${output_dir}"
shopt -s nullglob
count=0
# Timestamped names sort chronologically. Prefer recent failures when a retained
# node has more than ten snapshots, without following any credential symlink.
snapshots=("${source_dir}"/service-path-*.log)
for (( index=${#snapshots[@]}-1; index>=0; index-- )); do
    snapshot="${snapshots[index]}"
    # Never follow symlinks or copy kubeconfigs, tokens, manifests or arbitrary
    # sibling files. Ten 128-KiB snapshots bound this optional archive section.
    [[ -f "${snapshot}" && ! -L "${snapshot}" ]] || continue
    (( count < 10 )) || { echo "Warning: Fleet snapshot limit reached"; break; }
    # Redaction can expand short markers. Bound the resulting bytes as well,
    # consuming the whole capped pipe so set -o pipefail never hides SIGPIPE.
    head -c 131072 "${snapshot}" |
        sed -E 's/([Bb]earer[[:blank:]]+)[A-Za-z0-9._-]+/\1[REDACTED]/g' |
        LC_ALL=C awk 'BEGIN { remaining=131072 }
            remaining > 0 {
                line=$0 "\n"
                if (length(line) > remaining) line=substr(line, 1, remaining)
                printf "%s", line
                remaining-=length(line)
            }' > "${output_dir}/${snapshot##*/}"
    count=$((count+1))
done
echo "Collected ${count} bounded owned Fleet Service-path snapshots"
