#!/usr/bin/env bash
# Shared monitoring-stack helpers for demo scenarios.
# Source this from run.sh:
#   source "$(dirname "$0")/../../scripts/monitoring-helper.sh"

MONITORING_NS="${MONITORING_NS:-monitoring}"
DEMO_HELM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../helm" && pwd)"

# ── kube-prometheus-stack ────────────────────────────────────────────────────
# Installs kube-prometheus-stack via Helm (idempotent).
# Provides: Prometheus Operator, Prometheus, AlertManager, Grafana,
#           kube-state-metrics, node-exporter.
ensure_monitoring_stack() {
    if helm status kube-prometheus-stack -n "${MONITORING_NS}" &>/dev/null; then
        echo "  kube-prometheus-stack already installed."
        return 0
    fi

    echo "==> Installing kube-prometheus-stack..."
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null || true
    helm repo update prometheus-community

    kubectl create namespace "${MONITORING_NS}" --dry-run=client -o yaml | kubectl apply -f -

    helm upgrade --install kube-prometheus-stack \
        prometheus-community/kube-prometheus-stack \
        --namespace "${MONITORING_NS}" \
        --values "${DEMO_HELM_DIR}/kube-prometheus-stack-values.yaml" \
        --wait --timeout 5m

    echo "  kube-prometheus-stack installed in ${MONITORING_NS}."

    ensure_grafana_dashboard
}

# ── Grafana dashboard ConfigMap ──────────────────────────────────────────────
# Applies the Kubernaut Operations dashboard ConfigMap with the
# grafana_dashboard label so the Grafana sidecar autodiscovers it.
ensure_grafana_dashboard() {
    local dashboard_cm="${DEMO_HELM_DIR}/grafana-dashboard-kubernaut.yaml"

    if [ -f "$dashboard_cm" ]; then
        kubectl apply -f "$dashboard_cm" -n "${MONITORING_NS}"
        echo "  Kubernaut operations dashboard provisioned."
    fi
}

# ── cert-manager ─────────────────────────────────────────────────────────────
# Used by: cert-failure, cert-failure-gitops
ensure_cert_manager() {
    if helm status cert-manager -n cert-manager &>/dev/null; then
        echo "  cert-manager already installed."
        return 0
    fi

    echo "==> Installing cert-manager..."
    helm repo add jetstack https://charts.jetstack.io 2>/dev/null || true
    helm repo update jetstack

    kubectl create namespace cert-manager --dry-run=client -o yaml | kubectl apply -f -

    helm upgrade --install cert-manager jetstack/cert-manager \
        --namespace cert-manager \
        --set crds.enabled=true \
        --set prometheus.enabled=true \
        --wait --timeout 3m

    echo "  cert-manager installed."
}

# ── metrics-server ───────────────────────────────────────────────────────────
# Used by: hpa-maxed, autoscale (HPA requires real CPU/memory metrics)
ensure_metrics_server() {
    if kubectl get deployment metrics-server -n kube-system &>/dev/null; then
        echo "  metrics-server already installed."
        return 0
    fi

    echo "==> Installing metrics-server..."
    helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/ 2>/dev/null || true
    helm repo update metrics-server

    helm upgrade --install metrics-server metrics-server/metrics-server \
        --namespace kube-system \
        --set args='{--kubelet-insecure-tls}' \
        --wait --timeout 3m

    echo "  metrics-server installed."
}

# ── Linkerd ──────────────────────────────────────────────────────────────────
# Used by: mesh-routing-failure
ensure_linkerd() {
    if kubectl get namespace linkerd &>/dev/null; then
        echo "  Linkerd already installed."
        return 0
    fi

    echo "==> Installing Linkerd..."

    if ! command -v linkerd &>/dev/null; then
        echo "  Installing Linkerd CLI..."
        curl -fsL https://run.linkerd.io/install | sh
        export PATH="$HOME/.linkerd2/bin:$PATH"
    fi

    linkerd install --crds | kubectl apply -f -
    linkerd install | kubectl apply -f -
    linkerd check --wait 5m

    echo "  Linkerd installed."
}

# ── Blackbox Exporter ────────────────────────────────────────────────────────
# Used by: slo-burn (probe_success metric)
ensure_blackbox_exporter() {
    if helm status prometheus-blackbox-exporter -n "${MONITORING_NS}" &>/dev/null; then
        echo "  blackbox-exporter already installed."
        return 0
    fi

    echo "==> Installing blackbox-exporter..."
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null || true

    helm upgrade --install prometheus-blackbox-exporter \
        prometheus-community/prometheus-blackbox-exporter \
        --namespace "${MONITORING_NS}" \
        --wait --timeout 2m

    echo "  blackbox-exporter installed."
}
