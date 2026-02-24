# Duplicate Alert Suppression Demo

## Quick Start

```bash
./run.sh
```

## What It Demonstrates

Gateway-level deduplication via OwnerResolver fingerprinting. When 5 pods from the same Deployment crash simultaneously, Prometheus fires 5 alerts. The Gateway maps each pod alert to its owning Deployment via OwnerResolver, producing a single fingerprint. Instead of creating 5 RemediationRequests, the Gateway creates **1 RR** with `OccurrenceCount=5`.

## Key Insight

- **5 crashing pods** → same Deployment (`api-gateway`)
- **1 fingerprint** → `SHA256(demo-alert-storm:deployment:api-gateway)`
- **1 RemediationRequest** → `status.deduplication.occurrenceCount=5`

## Pipeline Path

```
5× Alert → Gateway (dedup to 1 RR) → SP → AA → RO → WE (rollback) → EM
```

## Business Requirement

- **BR-DEDUP-001**: Gateway deduplication via OwnerResolver fingerprinting

## Issue

- #170
