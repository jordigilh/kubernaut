# Scenario #133: cert-manager Certificate Failure (CRD/Operator)

## Overview

Demonstrates Kubernaut detecting a cert-manager Certificate stuck in NotReady because the CA Secret
backing the ClusterIssuer has been deleted, and performing automatic remediation by recreating
the CA Secret to restore certificate issuance.

**Signal**: `CertManagerCertNotReady` -- from `certmanager_certificate_ready_status`
**Root cause**: CA Secret deleted; ClusterIssuer cannot sign certificates
**Remediation**: `fix-certificate-v1` workflow recreates the CA Secret

## Signal Flow

```
certmanager_certificate_ready_status == 0 for 2m → CertManagerCertNotReady alert
  → Gateway → SP → AA (HAPI + real LLM)
  → LLM diagnoses missing CA Secret causing Certificate NotReady
  → Selects FixCertificate workflow
  → RO → WE (recreate CA Secret, trigger re-issuance)
  → EM verifies Certificate is Ready
```

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | `deploy/demo/scenarios/kind-config-singlenode.yaml` |
| Kubernaut services | Gateway, SP, AA, RO, WE, EM deployed |
| LLM backend | Real LLM (not mock) via HAPI |
| Prometheus | With cert-manager metrics |
| cert-manager | Installed (run.sh installs if missing) |
| Workflow catalog | `fix-certificate-v1` registered in DataStorage |

## Automated Run

```bash
./deploy/demo/scenarios/cert-failure/run.sh
```

## Manual Step-by-Step

### 1. Install cert-manager (if not present)

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.1/cert-manager.yaml
kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s
```

### 2. Generate CA and deploy scenario resources

```bash
# Generate self-signed CA (run.sh does this automatically)
TMPDIR=$(mktemp -d)
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "${TMPDIR}/ca.key" -out "${TMPDIR}/ca.crt" \
  -days 365 -subj "/CN=Demo CA/O=Kubernaut"
kubectl create secret tls demo-ca-key-pair \
  --cert="${TMPDIR}/ca.crt" --key="${TMPDIR}/ca.key" \
  -n cert-manager --dry-run=client -o yaml | kubectl apply -f -
rm -rf "${TMPDIR}"

kubectl apply -f deploy/demo/scenarios/cert-failure/manifests/namespace.yaml
kubectl apply -f deploy/demo/scenarios/cert-failure/manifests/clusterissuer.yaml
kubectl apply -f deploy/demo/scenarios/cert-failure/manifests/certificate.yaml
kubectl apply -f deploy/demo/scenarios/cert-failure/manifests/deployment.yaml
kubectl apply -f deploy/demo/scenarios/cert-failure/manifests/prometheus-rule.yaml
```

### 3. Verify healthy state

```bash
kubectl get certificate -n demo-cert-failure
# demo-app-cert should show Ready=True
kubectl get pods -n demo-cert-failure
# All pods should be Running
```

### 4. Inject failure

```bash
bash deploy/demo/scenarios/cert-failure/inject-broken-issuer.sh
```

The script deletes the `demo-ca-key-pair` Secret in cert-manager namespace and triggers
certificate re-issuance. cert-manager will fail to issue because the ClusterIssuer
can no longer sign.

### 5. Observe Certificate NotReady

```bash
kubectl get certificate -n demo-cert-failure -w
# Certificate status will show Ready=False
```

### 6. Wait for alert and pipeline

```bash
# Alert fires after 2 min of NotReady
# Check: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090 &
#        then open http://localhost:9090/alerts
kubectl get rr,sp,aa,we,ea -n demo-cert-failure -w
```

### 7. Verify remediation

```bash
kubectl get certificate -n demo-cert-failure
# Certificate should show Ready=True after workflow completes
kubectl get secret demo-ca-key-pair -n cert-manager
# CA Secret should exist (recreated by workflow)
```

## Cleanup

```bash
./deploy/demo/scenarios/cert-failure/cleanup.sh
```

## BDD Specification

```gherkin
Given a Kind cluster with Kubernaut services and a real LLM backend
  And Prometheus is scraping cert-manager metrics
  And the "fix-certificate-v1" workflow is registered in the DataStorage catalog
  And cert-manager is installed with a CA ClusterIssuer
  And the "demo-app-cert" Certificate is Ready in namespace "demo-cert-failure"

When the CA Secret backing the ClusterIssuer is deleted
  And the TLS secret is deleted to trigger re-issuance
  And cert-manager fails to issue the certificate (ClusterIssuer cannot sign)
  And the CertManagerCertNotReady alert fires (NotReady for 2 min)

Then Kubernaut Gateway receives the alert via Alertmanager webhook
  And Signal Processing enriches the signal with business labels
  And AI Analysis (HAPI + LLM) diagnoses missing CA Secret as root cause
  And the LLM selects the "FixCertificate" workflow (fix-certificate-v1)
  And Remediation Orchestrator creates a WorkflowExecution
  And Workflow Execution recreates the CA Secret and triggers re-issuance
  And cert-manager successfully issues the certificate
  And Effectiveness Monitor confirms the Certificate is Ready
```

## Acceptance Criteria

- [ ] Certificate starts Ready with valid CA Secret
- [ ] CA Secret deletion causes Certificate to become NotReady
- [ ] Alert fires within 2-3 minutes of Certificate NotReady
- [ ] LLM correctly diagnoses missing CA Secret as root cause
- [ ] FixCertificate workflow recreates the CA Secret
- [ ] cert-manager re-issues the certificate after CA restoration
- [ ] Certificate becomes Ready after remediation
- [ ] EM confirms successful remediation
