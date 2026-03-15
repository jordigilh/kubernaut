#!/usr/bin/env bash
# Post-processes crd-ref-docs markdown output to strip internal references
# that should not appear in user-facing documentation.
set -euo pipefail

FILE="${1:?Usage: clean-output.sh <file>}"

# ── Pass 1: Strip internal IDs and tagged references ──────────────────────────
sed -E -i.bak \
  -e 's/={5,}<br \/>//g' \
  -e 's/<br \/>={5,}//g' \
  -e 's/={5,}//g' \
  -e 's/BR-[A-Z_]+-[0-9]*(\.[0-9]+)*:?//g' \
  -e 's/ADR-[A-Z]*-?[0-9]+(\.[0-9]+)*:?//g' \
  -e 's/DD-[A-Z_-]+-?[0-9]+(\s+v[0-9.]+)?:?//g' \
  -e 's/Gap #[0-9]+:?//g' \
  -e 's/HAPI-[0-9]+(\.[0-9]+)*:?//g' \
  -e 's/Issue #[0-9]+:?//g' \
  -e 's/\(issue #[0-9]+:[^)]*\)//g' \
  -e 's/PHASE [0-9]+ ADDITION//g' \
  -e 's/\(Dec [0-9]+\)//g' \
  "$FILE"
rm -f "${FILE}.bak"

# ── Pass 2: Strip internal doc/planning references ────────────────────────────
sed -E -i.bak \
  -e 's/\([^)]*_CLARIFICATION\.md[^)]*\)//g' \
  -e 's/\([^)]*_STATUS\.md[^)]*\)//g' \
  -e 's/\([^)]*_PLAN\.md[^)]*\)//g' \
  -e 's/GAP-[A-Z0-9]+-[0-9]+ [A-Z]+:[^|<]*/  /g' \
  -e 's/Design Decision:[^|<]*//g' \
  -e 's/Implementation Plan Day [0-9]+:[^|<]*//g' \
  -e 's/[Pp]er [A-Za-z][a-z-]*\.md[^|<]*//g' \
  -e 's/[Pp]er REQUEST_[A-Z_]+\.md[^|<]*//g' \
  -e 's/\(-ADDENDUM[^)]*\)//g' \
  -e 's/per the Viceversa Pattern//g' \
  -e 's/Viceversa Pattern:[^|<]*//g' \
  -e 's/NOTE: [A-Z][A-Za-z]+ REMOVED *//g' \
  -e 's/<br \/>Reason:[^|<]*//g' \
  -e 's/<br \/>Observability:[^|<]*//g' \
  -e 's/<br \/>Correlation:[^|<]*//g' \
  -e 's/Gateway Team Fix \([0-9-]*\):[^|<]*//g' \
  -e 's/POST-RCA CONTEXT *//g' \
  "$FILE"
rm -f "${FILE}.bak"

# ── Pass 3: Strip truncated DD- leftovers and orphan fragments ────────────────
sed -E -i.bak \
  -e 's/Simplified per - [^|<]*//g' \
  -e 's/Enhanced per - [^|<]*//g' \
  -e 's/Renamed from [A-Za-z]+ per *//g' \
  -e 's/\(flat hierarchy per *\)//g' \
  -e 's/\(normalized per *\)//g' \
  -e 's/\(DEPRECATED per *\)//g' \
  -e 's/\(per *\)//g' \
  -e 's/ per \)/)/' \
  -e 's/\(UPPER_SNAKE_CASE keys per *\)//g' \
  -e 's/\(#[0-9]+\)//g' \
  -e 's/-0[0-9][0-9]: [^|<]*//g' \
  -e 's/pkg\/shared\/[a-z/]*\.go//g' \
  -e 's/pkg\/shared\/backoff//g' \
  -e 's/Uses shared types from *//g' \
  "$FILE"
rm -f "${FILE}.bak"

# ── Pass 4: Strip emojis and formatting noise ─────────────────────────────────
sed -E -i.bak \
  -e 's/🏛️ *//g' \
  -e 's/<br \/>[A-Z][A-Z /]{2,}[A-Z]( *\([^)]*\))?<br \/>/<br \/>/g' \
  -e 's/\|[[:space:]]*[A-Z][A-Z /]{2,}[A-Z]( *\([^)]*\))?<br \/>/| /g' \
  -e 's/<br \/>Reference:[^|]*//g' \
  -e 's/Reference:[^|]*\|/|/g' \
  -e '/^Reference:/d' \
  -e '/^Implementation Plan/d' \
  -e '/^Simplified per/d' \
  -e '/^Enhanced per/d' \
  -e '/^Renamed from/d' \
  -e '/^Design Decision/d' \
  -e '/^Per [A-Z]/d' \
  -e '/^[A-Z][A-Z ]{5,}$/d' \
  "$FILE"
rm -f "${FILE}.bak"

# ── Pass 5: Normalize whitespace and empty fragments ──────────────────────────
sed -E -i.bak \
  -e 's/  +/ /g' \
  -e 's/\( +\)/()/g' \
  -e 's/\(\)//g' \
  -e 's/ +\|/|/g' \
  -e 's/ +:/:/g' \
  -e 's/<br \/> *<br \/>/<br \/>/g' \
  -e 's/<br \/> *\|/|/g' \
  -e 's/\| *<br \/>/| /g' \
  "$FILE"
rm -f "${FILE}.bak"

# ── Pass 6: Collapse excessive blank lines (3+ → 2) ──────────────────────────
awk 'NF{blank=0} !NF{blank++} blank<=2' "$FILE" > "${FILE}.tmp" && mv "${FILE}.tmp" "$FILE"

# ── Pass 7: Convert shared types to plain text ────────────────────────────────
# crd-ref-docs emits these as markdown links or bold when the type is outside
# the source path. Convert both [TypeName](#anchor) and __TypeName__ to italic.
sed -E -i.bak \
  -e 's/\[KubernetesContext\]\(#kubernetescontext\)/_KubernetesContext_/g' \
  -e 's/\[BusinessClassification\]\(#businessclassification\)/_BusinessClassification_/g' \
  -e 's/\[OwnerChainEntry\]\(#ownerchainentry\)/_OwnerChainEntry_/g' \
  -e 's/\[NamespaceContext\]\(#namespacecontext\)/_NamespaceContext_/g' \
  -e 's/\[WorkloadDetails\]\(#workloaddetails\)/_WorkloadDetails_/g' \
  -e 's/__KubernetesContext__/_KubernetesContext_/g' \
  -e 's/__BusinessClassification__/_BusinessClassification_/g' \
  -e 's/__OwnerChainEntry__/_OwnerChainEntry_/g' \
  -e 's/__NamespaceContext__/_NamespaceContext_/g' \
  -e 's/__WorkloadDetails__/_WorkloadDetails_/g' \
  -e 's/__DetectedLabels__/_DetectedLabels_/g' \
  -e 's/__DeduplicationInfo__/_DeduplicationInfo_/g' \
  -e 's/__EnrichmentResults__/_EnrichmentResults_/g' \
  "$FILE"
rm -f "${FILE}.bak"
