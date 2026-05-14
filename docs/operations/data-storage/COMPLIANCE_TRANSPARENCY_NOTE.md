# Compliance transparency: audit hash chain and historical rows (PROD-M2)

## Context

Tamper-evident **hash chaining** (`event_hash` / `previous_event_hash`) applies to audit events persisted **after** the hashing rollout. Migration **`migrations/v0-archived/023_add_event_hashing.sql`** adds these columns with explicit backwards compatibility:

- Rows created **before** hashing: **`event_hash` and `previous_event_hash` remain NULL**.
- Rows created **after** hashing: hashes are populated on insert per the documented chain rules.

See that migration for the authoritative schema rationale and SOC2-oriented intent.

## Verification behavior

Chain verification (**`POST /api/v1/audit/verify-chain`**) verifies integrity for events that participate in the hash chain. Rows with **missing chain material** (`NULL` hashes where the chain validator expects hashed links) do **not** establish prior integrity—they are accounted for separately (for example **`skipped_null_hash`** in verification results).

Operators should interpret counts of skipped rows as **“pre-migration / pre-chain”** exclusions, **not** as proof of tampering.

## Retroactive hashing

There is **no plan to backfill hashes** onto historical audit rows:

- Updating stored historical rows would **change authoritative records** recorded before the hashing feature existed, defeating the immutable-audit premise for that period.

Retention, export, and other controls remain in force for legacy rows; only **cryptographic chain membership** applies from the hashing cutover onward.
