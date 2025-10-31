# DEPRECATED: This directory is no longer used

**All decision documents have been consolidated to:**
`docs/architecture/decisions/`

**Reason**: Single source of truth for all architectural and design decisions

**Date**: October 31, 2025

**Migration Summary**:
- All DD-GATEWAY-* files moved to `docs/architecture/decisions/`
- All DD-HOLMESGPT-* files moved to `docs/architecture/decisions/`
- All DD-EFFECTIVENESS-* files moved to `docs/architecture/decisions/`
- Project-wide DD files (DD-004, DD-005) moved to `docs/architecture/decisions/`

Please refer to [docs/architecture/decisions/README.md](../architecture/decisions/README.md) for the official index.

---

## Why This Change?

Having three separate locations for decision documents (docs/decisions/, docs/architecture/, docs/architecture/decisions/) created confusion and duplicate IDs. Consolidating to a single location provides:

- ✅ Clear rule: ALL decisions go in ONE place
- ✅ No confusion about placement  
- ✅ Easier to maintain index
- ✅ Consistent with official README
- ✅ Eliminates duplicate IDs

---

## Finding Old Documents

If you're looking for a document that was previously in this directory, check:

**`docs/architecture/decisions/`**

All documents have been moved there with their original names preserved (except for renumbering to resolve duplicate IDs).

