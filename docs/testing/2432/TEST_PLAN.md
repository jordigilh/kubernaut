# Test Plan: Gateway NetworkPolicy AlertManager Namespace Derivation

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2432-v1
**Feature**: Derive Gateway NetworkPolicy ingress from an in-cluster AlertManager URL
**Version**: 1.0
**Created**: 2026-09-17
**Author**: Kubernaut Platform
**Status**: Complete
**Branch**: `fix/2417-ka-fleet-overlay-all-flows`

---

## 1. Introduction

### 1.1 Purpose

This plan verifies that enabling the Gateway through Helm supports the common
in-cluster AlertManager topology without requiring a second NetworkPolicy
override, while preserving the chart's default-deny behavior for external or
ambiguous monitoring endpoints.

### 1.2 Objectives

1. **Explicit-source precedence**: Existing Gateway ingress namespaces,
   namespace selectors, and CIDRs remain authoritative and are not broadened by
   automatic derivation.
2. **In-cluster usability**: An enabled AlertManager integration using an
   unambiguous Kubernetes Service DNS URL derives exactly the source namespace.
3. **Fail-closed external behavior**: External, malformed, or ambiguous URLs do
   not generate an implicit ingress allow rule.
4. **Documentation traceability**: Users have a copy-paste post-install command
   and clear guidance for external AlertManager sources.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Helm template test pass rate | 100% | `helm unittest charts/kubernaut` |
| Explicit ingress regression | 0 regressions | Rendered NetworkPolicy assertions |
| Automatic ingress scope | Exactly one namespace for a valid in-cluster URL | Rendered NetworkPolicy assertion |
| External fail-closed behavior | No implicit ingress rule | Rendered NetworkPolicy assertion |
| Documentation coverage | Manual post-install and external-source paths documented | `charts/kubernaut/README.md` review |

---

## 2. References

### 2.1 Authority

- [BR-PLATFORM-005](../../requirements/BR-PLATFORM-005-helm-chart-operator-security-parity.md): Helm chart security and NetworkPolicy parity
- [BR-PLATFORM-009](../../requirements/BR-PLATFORM-009-helm-chart-gateway-apifrontend-ingress-parity.md): Gateway/APIFrontend ingress parity
- [Issue #2432](https://github.com/jordigilh/kubernaut/issues/2432): Gateway NetworkPolicy AlertManager namespace derivation
- [Issue #2162](https://github.com/jordigilh/kubernaut/issues/2162): `gateway.enabled` component toggle

### 2.2 Cross-References

- `charts/kubernaut/templates/gateway/networkpolicy.yaml`
- `charts/kubernaut/templates/_helpers.tpl`
- `charts/kubernaut/tests/networkpolicy_ingress_flexibility_test.yaml`
- `charts/kubernaut/README.md`

---

## 3. Risks and Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | URL parsing treats an external endpoint as an in-cluster source | Unauthorized workloads could reach Gateway | Low | UT-NP-2432-003, UT-NP-2432-004 | Match only the Kubernetes Service DNS shape and emit no fallback rule otherwise |
| R2 | Automatic derivation overrides an operator's narrower source policy | Unexpected ingress expansion | Low | UT-NP-2432-001 | Suppress derivation whenever any explicit Gateway ingress source is configured |
| R3 | Prometheus or Thanos is incorrectly treated as an ingress source | Incorrect policy and unnecessary exposure | Medium | UT-NP-2432-005 | Derive only from `monitoring.alertManager.url`; document Prometheus/Thanos as egress/read-side integrations |
| R4 | Existing Gateway default-deny behavior changes for installations without AlertManager integration | Security regression | Low | UT-NP-2432-006 | Keep the derived rule conditional on Gateway and AlertManager being enabled |

---

## 4. Scope

### 4.1 Features to Be Tested

- Gateway NetworkPolicy rendering and source-rule derivation
- Helm monitoring AlertManager enabled/URL configuration
- Manual post-install Gateway enablement documentation

### 4.2 Features Not to Be Tested

- Runtime AlertManager delivery or Gateway request processing: covered by
  existing integration and E2E suites
- Prometheus/Thanos deployment or URL validation: those components are outside
  the Kubernaut Helm chart
- Gateway authentication and RBAC semantics: unchanged by this issue

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use only `monitoring.alertManager.url` for derivation | AlertManager sends webhook signals to Gateway; Prometheus and Thanos do not |
| Derive only from an unambiguous in-cluster Service DNS name | URL parsing cannot reliably identify source pods for external, proxied, or arbitrary endpoints |
| Explicit Gateway ingress values suppress derivation | Operators retain complete control over the fail-closed ingress boundary |
| Keep no-rule behavior for unsupported URLs | The safer failure mode is unavailable signal ingestion rather than implicit exposure |

---

## 5. Approach

### 5.1 Test Framework

This is Helm-template behavior. Tests use the repository-approved
`helm-unittest` framework under `charts/kubernaut/tests/`; no Go test is
required.

### 5.2 Test Cases

| Test ID | Scenario | Expected Result |
|---------|----------|-----------------|
| UT-NP-2432-001 | Explicit `ingressNamespaces` with in-cluster AlertManager URL | Explicit list is rendered; no derived duplicate or additional namespace |
| UT-NP-2432-002 | Explicit `ingressCIDRs` or raw namespace selectors with in-cluster AlertManager URL | Explicit rules are rendered; no implicit namespace rule is added |
| UT-NP-2432-003 | AlertManager URL `http://alertmanager.monitoring.svc.cluster.local:9093` | One ingress rule permits namespace `monitoring` on Gateway port 8080 |
| UT-NP-2432-004 | External URL such as `https://alerts.example.com` | No implicit namespace ingress rule is rendered |
| UT-NP-2432-005 | Prometheus or Thanos URL is in-cluster while AlertManager integration is disabled | No Gateway ingress rule is derived from read-side URLs |
| UT-NP-2432-006 | AlertManager integration disabled or URL absent | Existing default-deny Gateway ingress behavior remains unchanged |

### 5.3 Pass/Fail Criteria

**PASS** requires all listed test cases to pass, existing chart tests to remain
green, and the README to contain the post-install and external-source guidance.

**FAIL** occurs on any implicit ingress rule for an unsupported URL, any loss of
explicit-source precedence, or any regression in the existing Gateway toggle
tests.

---

## 6. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-PLATFORM-005 | Preserve default-deny and operator-controlled ingress boundaries | P0 | Integration | UT-NP-2432-001..006 | Pass |
| BR-PLATFORM-009 | Provide a supported Gateway ingress configuration for common in-cluster deployment | P1 | Integration | UT-NP-2432-003 | Pass |

---

## 7. Tier Skip Rationale

No separate Go unit or runtime integration test is planned. The behavior under
test is entirely Helm/Sprig rendering and is inherently covered at the chart
integration tier. Runtime Gateway and AlertManager delivery behavior is not
changed and remains covered by existing service integration/E2E suites.

---

## 8. Execution

Run from the repository root:

```bash
helm unittest charts/kubernaut
```

Additional validation:

```bash
helm lint charts/kubernaut
helm template kubernaut charts/kubernaut \
  --set gateway.enabled=true \
  --set monitoring.alertManager.enabled=true \
  --set monitoring.alertManager.url=http://alertmanager.monitoring.svc.cluster.local:9093 \
  --set aianalysis.policies.existingConfigMap=test-cm \
  --set signalprocessing.policies.existingConfigMap=test-cm \
  --set global.llmProfiles.primary.provider=openai \
  --set global.llmProfiles.primary.model=gpt-4 \
  --set global.llmProfiles.primary.credentialsSecretName=llm-credentials-primary \
  --set kubernautAgent.llmProfileRef=primary
```

Validation result on 2026-09-17:

- `helm unittest charts/kubernaut`: 89 suites, 703 tests passed
- `helm lint charts/kubernaut` with required policy/profile test values: passed
- `git diff --check`: passed
