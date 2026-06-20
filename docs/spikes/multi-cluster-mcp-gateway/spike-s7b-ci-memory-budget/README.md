# Spike S7b — CI Memory Budget for Fleet E2E

## Goal

Determine what fleet E2E infrastructure can fit within the 6GB memory constraint
of GitHub Actions `ubuntu-latest` runners (using Podman, not Docker).

## Memory Budget Analysis

### GitHub Actions `ubuntu-latest` specs (2026)

- **Total RAM**: 7GB
- **OS + system overhead**: ~1GB (kernel, systemd, apt cache)
- **Available for tests**: ~6GB

### Kind Cluster Memory Usage (empirical data)

| Component | Memory (idle) | Memory (loaded) |
|-----------|--------------|-----------------|
| Kind single-node control plane | ~450-600 MB | ~800 MB-1.2 GB |
| etcd | ~100-200 MB | ~300 MB (many CRDs) |
| kube-apiserver | ~200-300 MB | ~500 MB |
| kube-controller-manager | ~50 MB | ~100 MB |
| kube-scheduler | ~30 MB | ~50 MB |
| CoreDNS | ~30 MB | ~50 MB |
| kindnet (CNI) | ~20 MB | ~30 MB |

### Current Full-Pipeline E2E Memory Estimate

Full pipeline deploys 14 services + databases:

| Component Group | Est. Memory |
|----------------|-------------|
| Kind control plane | ~800 MB |
| PostgreSQL | ~100 MB |
| Redis/Valkey | ~50 MB |
| Gateway | ~100 MB |
| DataStorage | ~80 MB |
| KubernautAgent | ~150 MB |
| APIFrontend | ~150 MB |
| RemediationOrchestrator | ~80 MB |
| SignalProcessing | ~60 MB |
| AIAnalysis | ~60 MB |
| WorkflowExecution | ~60 MB |
| Notification | ~40 MB |
| EffectivenessMonitor | ~40 MB |
| MockLLM | ~50 MB |
| Prometheus + AlertManager | ~200 MB |
| **Total (single cluster)** | **~2.0 GB** |

### Fleet E2E Scenarios

#### Option A: Single cluster + Mock MCP Gateway Pod (~2.1 GB)

- Current full pipeline: ~2.0 GB
- Mock MCP Gateway Pod: ~50 MB (lightweight Go binary)
- FMC Writer: ~30 MB
- **Total: ~2.1 GB** (fits comfortably in 6 GB)

#### Option B: Two minimal clusters (~3.5-4.0 GB)

- Hub cluster (full pipeline): ~2.0 GB
- Spoke cluster (control plane + K8s MCP Server only): ~700-900 MB
- Cross-cluster networking overhead: ~100 MB
- **Total: ~2.8-3.0 GB** (fits, but tight with Go test binary + compilation)

#### Option C: Two full clusters (NOT feasible)

- Hub cluster (full pipeline): ~2.0 GB
- Spoke cluster (full pipeline): ~2.0 GB
- **Total: ~4.0 GB** + Go toolchain ~1.5 GB → **exceeds 6 GB**

## Decision

### Recommended: Option A (Single cluster + Mock MCP Gateway Pod)

**Rationale:**
- Fits comfortably in 6 GB with 3.9 GB headroom
- Mock MCP Gateway already exists as `test/services/mock-mcp-gateway/`
- Proves the same code paths as a real second cluster
- No cross-cluster networking complexity in CI
- Fastest to set up and most reliable

### Stretch (validate with spike): Option B

Option B MAY fit with careful management:
- Go test binary compilation uses ~1.5 GB (peak)
- After compilation, memory is released
- Spoke cluster uses minimal images (control plane only)
- Requires sequential setup: build → start hub → start spoke

### How to validate Option B

Run the GitHub Actions workflow in `.github/workflows/fleet-e2e-memory-spike.yml`
and check the memory measurements.

## Implementation for Phase 4 (E2E)

Based on this analysis, Phase 4 should:

1. Deploy Mock MCP Gateway as an in-cluster Pod (Option A)
2. Pre-seed Valkey with scope cache data for the mock cluster
3. Apply `MCPServerRegistration` CRD to declare the mock cluster
4. Test GW alert ingestion with cluster label → RR with clusterID
5. Test AF preflight investigation via MCP Gateway → mock cluster response
6. Test KA tool discovery via MCP Gateway → BridgeTool execution

If CI memory allows (per spike workflow), we can optionally add:
7. Second Kind cluster with K8s MCP Server for realistic spoke (Option B)

## Spike Workflow

See `.github/workflows/fleet-e2e-memory-spike.yml` for the validation workflow.
This workflow measures actual memory usage at each stage and reports results.
