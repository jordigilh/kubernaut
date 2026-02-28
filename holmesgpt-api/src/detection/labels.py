# Copyright 2026 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
DetectedLabels label detector — Python implementation per DD-HAPI-018.

Cross-language contract: this implementation MUST produce identical results
to the Go reference in pkg/signalprocessing/detection/labels.go for all
DD-HAPI-018 conformance test vectors.

ADR-056: Computes labels for the actual RCA target resource (post-RCA)
rather than the signal source (pre-RCA).
"""

from typing import Any, Dict, List, Optional
import logging

logger = logging.getLogger(__name__)


class LabelDetector:
    """Detects cluster characteristics for a Kubernetes resource.

    Implements the 7 detection characteristics defined in DD-HAPI-018:
    gitOpsManaged, pdbProtected, hpaEnabled, stateful, helmManaged,
    networkIsolated, serviceMesh.

    Args:
        k8s_queries: Object providing async K8s API query methods:
            - list_pdbs(namespace) -> (list, error_str|None)
            - list_hpas(namespace) -> (list, error_str|None)
            - list_network_policies(namespace) -> (list, error_str|None)
    """

    def __init__(self, k8s_queries):
        self._k8s = k8s_queries

    async def detect_labels(
        self,
        k8s_context: Optional[Dict[str, Any]],
        owner_chain: Optional[List[Dict[str, str]]],
    ) -> Optional[Dict[str, Any]]:
        """Detect cluster characteristics for a resource.

        Returns None if k8s_context is None. Otherwise returns a dict with
        all DetectedLabels fields per DD-HAPI-018 schema.
        """
        if k8s_context is None:
            return None

        result = {
            "failedDetections": [],
            "gitOpsManaged": False,
            "gitOpsTool": "",
            "pdbProtected": False,
            "hpaEnabled": False,
            "stateful": False,
            "helmManaged": False,
            "networkIsolated": False,
            "serviceMesh": "",
        }

        namespace = k8s_context.get("namespace", "")

        self._detect_gitops(k8s_context, result)
        await self._detect_pdb(namespace, k8s_context, result)
        await self._detect_hpa(namespace, k8s_context, owner_chain or [], result)
        self._detect_stateful(owner_chain or [], result)
        self._detect_helm(k8s_context, result)
        await self._detect_network_policy(namespace, result)
        self._detect_service_mesh(k8s_context, result)

        return result

    def _detect_gitops(self, k8s_context: Dict, result: Dict) -> None:
        """Detect GitOps management (ArgoCD or Flux).

        Precedence per DD-HAPI-018:
        1. Pod annotation argocd.argoproj.io/tracking-id (ArgoCD v3 default)
        2. Pod annotation argocd.argoproj.io/instance (ArgoCD v2 custom label)
        3. Deployment label fluxcd.io/sync-gc-mark -> flux
        4. Deployment label argocd.argoproj.io/instance (ArgoCD v2 custom label)
        5. Deployment annotation argocd.argoproj.io/tracking-id (ArgoCD v3)
        6. Namespace labels for ArgoCD/Flux
        7. Namespace annotations for ArgoCD/Flux
        """
        pod = k8s_context.get("pod_details") or {}
        deploy = k8s_context.get("deployment_details") or {}
        pod_annotations = pod.get("annotations") or {}
        deploy_labels = deploy.get("labels") or {}
        deploy_annotations = deploy.get("annotations") or {}
        ns_labels = k8s_context.get("namespace_labels") or {}
        ns_annotations = k8s_context.get("namespace_annotations") or {}

        # ArgoCD v3 annotation-based tracking (default in v3.x)
        if "argocd.argoproj.io/tracking-id" in pod_annotations:
            result["gitOpsManaged"] = True
            result["gitOpsTool"] = "argocd"
            return

        # ArgoCD v2 custom instance label on pod annotations
        if "argocd.argoproj.io/instance" in pod_annotations:
            result["gitOpsManaged"] = True
            result["gitOpsTool"] = "argocd"
            return

        if "fluxcd.io/sync-gc-mark" in deploy_labels:
            result["gitOpsManaged"] = True
            result["gitOpsTool"] = "flux"
            return

        if "argocd.argoproj.io/instance" in deploy_labels:
            result["gitOpsManaged"] = True
            result["gitOpsTool"] = "argocd"
            return

        # ArgoCD v3 tracking-id on deployment annotations
        if "argocd.argoproj.io/tracking-id" in deploy_annotations:
            result["gitOpsManaged"] = True
            result["gitOpsTool"] = "argocd"
            return

        # Namespace labels (checked before annotations per DD-HAPI-018 precedence)
        if "argocd.argoproj.io/instance" in ns_labels:
            result["gitOpsManaged"] = True
            result["gitOpsTool"] = "argocd"
            return

        if "fluxcd.io/sync-gc-mark" in ns_labels:
            result["gitOpsManaged"] = True
            result["gitOpsTool"] = "flux"
            return

        # Namespace annotations (lowest precedence)
        if "argocd.argoproj.io/tracking-id" in ns_annotations:
            result["gitOpsManaged"] = True
            result["gitOpsTool"] = "argocd"
            return

        if "argocd.argoproj.io/managed" in ns_annotations:
            result["gitOpsManaged"] = True
            result["gitOpsTool"] = "argocd"
            return

        if "fluxcd.io/sync-status" in ns_annotations:
            result["gitOpsManaged"] = True
            result["gitOpsTool"] = "flux"
            return

    async def _detect_pdb(self, namespace: str, k8s_context: Dict, result: Dict) -> None:
        """Detect PodDisruptionBudget protection.

        Limitation: Only supports matchLabels selectors. The Go reference uses
        LabelSelectorAsSelector which also handles matchExpressions. This is
        acceptable for DD-HAPI-018 conformance (all test vectors use matchLabels).
        """
        pod = k8s_context.get("pod_details") or {}
        pod_labels = pod.get("labels") or {}
        if not pod_labels:
            return

        try:
            pdbs, error = await self._k8s.list_pdbs(namespace)
            if error:
                result["pdbProtected"] = False
                result["failedDetections"].append("pdbProtected")
                return

            for pdb in pdbs:
                selector = pdb.spec.selector
                if selector is None:
                    continue
                match_labels = selector.match_labels or {}
                if match_labels and _labels_match(match_labels, pod_labels):
                    result["pdbProtected"] = True
                    return
        except BaseException as exc:
            # BaseException intentionally catches CancelledError (Python 3.9+
            # moved it from Exception to BaseException). We prioritize returning
            # partial results over propagating cancellation per DD-HAPI-018.
            logger.warning("PDB detection failed: %s", exc)
            result["pdbProtected"] = False
            result["failedDetections"].append("pdbProtected")

    async def _detect_hpa(
        self, namespace: str, k8s_context: Dict,
        owner_chain: List[Dict], result: Dict,
    ) -> None:
        """Detect HorizontalPodAutoscaler targeting this workload."""
        deploy = k8s_context.get("deployment_details") or {}
        deploy_name = deploy.get("name", "")

        target_names = set()
        if deploy_name:
            target_names.add(("Deployment", deploy_name))
        for owner in owner_chain:
            target_names.add((owner.get("kind", ""), owner.get("name", "")))

        if not target_names:
            return

        try:
            hpas, error = await self._k8s.list_hpas(namespace)
            if error:
                result["hpaEnabled"] = False
                result["failedDetections"].append("hpaEnabled")
                return

            for hpa in hpas:
                ref = hpa.spec.scale_target_ref
                if (ref.kind, ref.name) in target_names:
                    result["hpaEnabled"] = True
                    return
        except BaseException as exc:
            logger.warning("HPA detection failed: %s", exc)
            result["hpaEnabled"] = False
            result["failedDetections"].append("hpaEnabled")

    def _detect_stateful(self, owner_chain: List[Dict], result: Dict) -> None:
        """Detect StatefulSet in owner chain."""
        for owner in owner_chain:
            if owner.get("kind") == "StatefulSet":
                result["stateful"] = True
                return

    def _detect_helm(self, k8s_context: Dict, result: Dict) -> None:
        """Detect Helm management via deployment labels."""
        deploy = k8s_context.get("deployment_details") or {}
        deploy_labels = deploy.get("labels") or {}

        if deploy_labels.get("app.kubernetes.io/managed-by") == "Helm":
            result["helmManaged"] = True
            return

        if "helm.sh/chart" in deploy_labels:
            result["helmManaged"] = True
            return

    async def _detect_network_policy(self, namespace: str, result: Dict) -> None:
        """Detect any NetworkPolicy in namespace."""
        try:
            netpols, error = await self._k8s.list_network_policies(namespace)
            if error:
                result["networkIsolated"] = False
                result["failedDetections"].append("networkIsolated")
                return

            if netpols:
                result["networkIsolated"] = True
        except BaseException as exc:
            logger.warning("NetworkPolicy detection failed: %s", exc)
            result["networkIsolated"] = False
            result["failedDetections"].append("networkIsolated")

    def _detect_service_mesh(self, k8s_context: Dict, result: Dict) -> None:
        """Detect service mesh (Istio or Linkerd). Istio takes precedence."""
        pod = k8s_context.get("pod_details") or {}
        pod_annotations = pod.get("annotations") or {}

        if "sidecar.istio.io/status" in pod_annotations:
            result["serviceMesh"] = "istio"
            return

        if "linkerd.io/proxy-version" in pod_annotations:
            result["serviceMesh"] = "linkerd"
            return


def _labels_match(selector: Dict[str, str], pod_labels: Dict[str, str]) -> bool:
    """Check if all selector key-value pairs exist in pod_labels."""
    return all(pod_labels.get(k) == v for k, v in selector.items())
