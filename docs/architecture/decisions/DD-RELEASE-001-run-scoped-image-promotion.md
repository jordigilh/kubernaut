# DD-RELEASE-001: Run-Scoped Image Promotion

**Status**: Approved
**Date**: 2026-10-26
**Business requirement**: BR-PLATFORM-RELEASE-001

## Context

Release qualification builds application images before the operator-managed
FullPipeline and Fleet journeys run. Publishing those images directly under the
release-version architecture tags allows a failed or rerun qualification to leave
partially qualified release artifacts, or overwrite an existing tag.

## Decision

All release-workflow builds publish architecture images and the temporary manifest
under a unique run-scoped `qualification_tag`. Qualification, scanning, signing, and
provenance consume that tag. A promotion job creates the immutable release-version
architecture tags and manifest list only after every required gate succeeds. GA
`:latest` tagging runs after promotion and the same gates.

Manual workflow-dispatch fallback runs use the same staging path but never promote,
sign a release, tag `:latest`, or create a GitHub Release.

## Alternatives considered

1. **Publish release-version tags before qualification** — rejected because failed or
   repeated runs can leave unqualified or overwritten release artifacts.
2. **Reject only existing release tags** — rejected because a failed first attempt can
   still leave release-version artifacts that are not qualification-proven.
3. **Run qualification from local image archives** — rejected for this workflow because
   it duplicates the existing registry-based E2E image contract and adds large artifact
   handoff complexity.

## Consequences

- A promotion failure leaves only run-scoped staging artifacts, not a mutable release
  version.
- Promotion adds one registry-copy job after qualification and attestation gates.
- Reruns use a new staging tag and cannot overwrite prior release-version architecture
  tags.
