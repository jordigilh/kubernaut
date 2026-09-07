# Test Plan: Fleet-Aware LabelDetector Enrichment (#2345)

**Business requirement**: BR-INTEGRATION-1489
**Scope**: KubernautAgent LabelDetector reads for fleet-target investigations
**Status**: Implemented; validation tracked by this plan

## Business Contract

When an investigation targets a remote cluster, LabelDetector must read the
target cluster through the fleet MCP overlay. It must not query the hub as a
fallback. Detected HPA/PDB and related infrastructure labels must be available
to workflow selection and persisted in the post-RCA context.

## Control Objectives

- **FedRAMP AC-4**: resource-enrichment data must flow from the selected cluster
  boundary, not an unrelated hub cluster.
- **FedRAMP AC-6**: fleet requests must not silently fall back to broader hub
  access when the target is remote.
- **FedRAMP SI-10**: MCP list arguments and responses must be validated and
  malformed responses must fail as detection errors, not produce false labels.
- **OWASP ASVS 5.1.x**: external MCP response data is parsed and validated
  before use.
- **OWASP ASVS 5.5.2**: cluster selection remains authoritative from request
  context; detected labels never select or switch the target cluster.

## Test Pyramid

### Unit Tests

- `UT-KA-FLEET-031`: `resources_list` arguments include kind, apiVersion,
  namespace, and label selector.
- `UT-KA-FLEET-032`: list responses populate unstructured items and reject
  malformed responses/items.
- `UT-KA-FLEET-033`: missing remote list capability is observable and does not
  fall back to the hub.

### Integration Tests

- `IT-KA-FLEET-2345`: production-style Enricher resolver routes a fleet-target
  investigation through remote `resources_get/resources_list`; HPA and PDB
  detected labels are returned and no corresponding detection failure is
  recorded.
- `IT-KA-FLEET-036`: existing owner-chain production-path test proves the same
  per-request overlay routing and hub-local regression behavior.

### End-to-End Tests

- `E2E-FLEET-2345`: real hub/spoke fleet lane creates a remote Deployment,
  HPA, and PDB; real KA investigates through the MCP Gateway; assertions verify
  `AIAnalysis.Status.PostRCAContext.DetectedLabels.HPAEnabled` and
  `PDBProtected` are true.

## Wiring Manifest

| Component | Production entry point | Wiring location | Test |
|---|---|---|---|
| Fleet LabelDetector | KA automatic pre-fetch enrichment | `cmd/kubernautagent/datastorage.go` → `Enricher.WithLabelDetectorResolver` | `IT-KA-FLEET-2345`, `E2E-FLEET-2345` |

## Validation

- Targeted enrichment, custom-tool, and KA tests must pass.
- `go build ./...` must pass.
- Targeted `golangci-lint` must pass.
- Fleet E2E must run in a real hub/spoke environment; an environment-gated
  skip is not evidence of E2E completion.
