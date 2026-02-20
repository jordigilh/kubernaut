# Scenario #134: cert-manager Certificate Failure with GitOps

## Overview

Same fault as #133 (cert-manager Certificate stuck NotReady) but cert-manager resources (Certificate, ClusterIssuer) are managed by ArgoCD via Gitea. Remediation uses **git revert** instead of direct kubectl changes.

**Key differentiator**: The LLM detects `gitOpsManaged=true` and `gitOpsTool=argocd` from the environment and selects the GitOps-aware workflow (`fix-certificate-gitops-v1`) that reverts the bad commit rather than directly recreating the CA Secret.

**Signal**: `KubernautCertificateNotReady` — from `certmanager_certificate_ready_status`  
**Root cause**: Bad Git commit changed ClusterIssuer to reference non-existent CA Secret  
**Remediation**: `fix-certificate-gitops-v1` workflow performs git revert

## Signal Flow

```
certmanager_certificate_ready_status == 0 for 2m → KubernautCertificateNotReady alert
  → Gateway → SP → AA (HAPI + real LLM)
  → HAPI LabelDetector detects gitOpsManaged=true, gitOpsTool=argocd
  → LLM diagnoses broken ClusterIssuer (bad commit) as root cause
  → LLM selects fix-certificate-gitops-v1 (git-based fix) over fix-certificate-v1 (kubectl)
  → RO → WE Job (git revert HEAD, push to Gitea)
  → ArgoCD re-syncs restored ClusterIssuer
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
| Gitea | Lightweight Git server (run.sh installs if missing) |
| ArgoCD | Core install (run.sh installs if missing) |
| Workflow catalog | `fix-certificate-gitops-v1` registered in DataStorage |
| Memory budget | ~6.1GB total (4.6GB base + 1.5GB GitOps infra) |

## BDD Specification

```gherkin
Feature: cert-manager Certificate failure remediation via git revert (GitOps)

  Scenario: Broken ClusterIssuer causes Certificate NotReady in GitOps environment
    Given ArgoCD manages Certificate "demo-app-cert" and ClusterIssuer "demo-selfsigned-ca-gitops"
      And the Gitea repository contains healthy cert-manager manifests synced by ArgoCD
      And the Certificate is Ready and the demo-app Deployment is Running

    When a bad commit is pushed to Gitea changing ClusterIssuer to reference "nonexistent-ca-secret"
      And ArgoCD syncs the broken ClusterIssuer to the cluster
      And the TLS secret is deleted to trigger re-issuance
      And cert-manager fails to issue because the ClusterIssuer cannot sign

    Then Prometheus fires "KubernautCertificateNotReady" alert for namespace "demo-cert-gitops"
      And Gateway creates a RemediationRequest
      And Signal Processing enriches with namespace labels (environment=production, criticality=high)
      And HAPI LabelDetector detects "gitOpsManaged=true" and "gitOpsTool=argocd"
      And the LLM traces the Certificate NotReady to broken ClusterIssuer (bad Git commit)
      And the LLM selects "fix-certificate-gitops-v1" workflow (not "fix-certificate-v1")
      And Remediation Orchestrator creates WorkflowExecution
      And the WE Job clones the Gitea repo and runs "git revert HEAD"
      And ArgoCD syncs the reverted ClusterIssuer back to the cluster
      And Effectiveness Monitor verifies Certificate is Ready
```

## Acceptance Criteria

- [ ] Gitea + ArgoCD deployed and managing `demo-cert-gitops` namespace
- [ ] Certificate and ClusterIssuer are GitOps-managed (synced from Gitea)
- [ ] Bad ClusterIssuer commit causes Certificate to become NotReady
- [ ] SP enriches signal with business classification from namespace labels
- [ ] HAPI detects `gitOpsManaged=true` and `gitOpsTool=argocd` (DD-HAPI-018)
- [ ] LLM identifies broken ClusterIssuer (bad commit) as root cause
- [ ] LLM selects `fix-certificate-gitops-v1` (git revert) over `fix-certificate-v1` (kubectl)
- [ ] WE Job performs `git revert` in Gitea repository
- [ ] ArgoCD auto-syncs the reverted ClusterIssuer
- [ ] EM verifies Certificate is Ready
- [ ] Full pipeline: Gateway -> RO -> SP -> AA -> WE -> EM

## Automated Run

```bash
./deploy/demo/scenarios/cert-failure-gitops/run.sh
```

## Manual Step-by-Step

### 1. Install Prerequisites

```bash
# cert-manager (if not present)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.1/cert-manager.yaml

# GitOps infrastructure
./deploy/demo/scenarios/gitops/scripts/setup-gitea.sh
./deploy/demo/scenarios/gitops/scripts/setup-argocd.sh
```

### 2. Run the Scenario

```bash
./deploy/demo/scenarios/cert-failure-gitops/run.sh
```

The script will: create CA, push manifests to Gitea, deploy ArgoCD Application, establish baseline, inject failure via bad git push, and wait for the pipeline.

### 3. Observe Pipeline

```bash
kubectl get rr,sp,aa,we,ea -n demo-cert-gitops -w
```

### 4. Verify Remediation

```bash
kubectl get certificate -n demo-cert-gitops
# demo-app-cert should show Ready=True after workflow completes
```

### 5. Cleanup

```bash
./deploy/demo/scenarios/cert-failure-gitops/cleanup.sh
```

## Workflow Details

- **Workflow ID**: `fix-certificate-gitops-v1`
- **Action Type**: `GitRevertCommit`
- **Bundle**: `workflow/Dockerfile` (ubi9-minimal + git + kubectl)
- **Script**: `workflow/remediate.sh` (Validate -> Action -> Verify pattern)
