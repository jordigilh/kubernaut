# DD-FLEET-014: Authoritative Gateway owner-GVK handoff

Status: approved bounded direction, 2026-11-16, PR #2637 / issue #2593.
Requirements: BR-GATEWAY-004, BR-SCOPE-001/002, BR-INTEGRATION-065.

## Decision and alternatives

The user selected complete owner-GVK handoff, not an unchanged-identity-only
patch. Extend the existing owner resolver with a version-bearing contract using
the existing shared `OwnerChainEntry` identity. Gateway adapters must use that
contract. Keep the old kind/name entry point for compatibility with existing
non-Gateway callers; it is not an authority for Gateway scope decisions.

1. **Carry authority through the existing traversal — selected.** Source identity
   comes from the event or discovery-backed resource selection. Each controller
   transition replaces kind, name and API version together from that exact
   `OwnerReference`, including same-kind/different-name transitions. This avoids
   a second lookup and preserves the existing cache/fallback and depth behavior.
2. **Re-resolve the final kind after traversal — rejected.** Smaller interface
   change, but loses group/version provenance and races with discovery or owner
   changes; the hub registry cannot establish a remote owner's version.
3. **Preserve only unchanged identity or copy the child's version — rejected.**
   The former leaves the changed-owner defect; the latter supplies false GVK.

## Contract

- Explicit input GVK controls metadata reads. Discovery may supply an absent
  initial version only when a registry is available; static kind tables are not
  version authority for the new contract. Unknown authority returns an error.
- An owner reference's own version controls subsequent reads and the final
  output, even if discovery prefers a different version. Missing/malformed owner
  authority fails closed rather than retaining the child's version.
- Standalone identity retains its input version. CRD traversal stop, missing
  intermediate parent and maximum depth retain the last proven owner reference.
- Service selection uses the typed core Pod schema for the selected Pod, not
  the Service's version. Unresolved selection retains the original Service.
- Cache miss and trust-but-verify paths carry the same reference authority.
- Fingerprint format, cluster dispatch, public v1, scope presence and fencing
  are unchanged. No dependencies, RBAC or HA changes are authorized.

## Verification

Ginkgo unit scenarios cover changed-kind, same-kind, unchanged identity, explicit
version overriding registry preference, fallback, Service selection and unknown
authority. Integration exercises production adapter dispatch through a real MCP
client to a controlled external endpoint, proving remote GVK and cluster routing.
FMC stale-fence RCA and pinned producer qualification remain separate blockers.
