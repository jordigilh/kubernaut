{{/*
Create a default fully qualified app name.
*/}}
{{- define "kubernaut.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels applied to every resource.
*/}}
{{- define "kubernaut.labels" -}}
helm.sh/chart: {{ include "kubernaut.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: kubernaut
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Chart label value.
*/}}
{{- define "kubernaut.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
True if this render has live cluster access (a real `helm install`/
`upgrade`), false during `helm template` / GitOps rendering (ArgoCD, Flux),
where `lookup` always returns an empty result. Used to gate auto-discovery
helpers (e.g. kubernaut.np.apiServerPeers) that need `lookup`.
Usage: {{ if eq (include "kubernaut.hasClusterAccess" .) "true" }}...{{ end }}
*/}}
{{- define "kubernaut.hasClusterAccess" -}}
{{- if lookup "v1" "Namespace" "" "kube-system" -}}true{{- end -}}
{{- end }}

{{/*
Render imagePullSecrets from global.imagePullSecrets for private registries.
Usage: {{ include "kubernaut.imagePullSecrets" . | nindent 6 }}
*/}}
{{- define "kubernaut.imagePullSecrets" -}}
{{- with .Values.global.imagePullSecrets }}
imagePullSecrets:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Render nodeSelector and tolerations for a component pod spec.
Component-level values override global defaults.
Usage: {{ include "kubernaut.scheduling" (dict "component" .Values.gateway "global" .Values.global) | nindent 6 }}
*/}}
{{- define "kubernaut.scheduling" -}}
{{- $nodeSelector := coalesce .component.nodeSelector .global.nodeSelector -}}
{{- with $nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- $tolerations := coalesce .component.tolerations .global.tolerations -}}
{{- with $tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Render the container image for a Kubernaut service.
Constructs: {registry}/{namespace}{separator}{service}:{tag|@digest}
  separator="/" → quay.io/kubernaut-ai/gateway:tag        (nested registries)
  separator="-" → quay.io/myorg/kubernaut-ai-gateway:tag   (flat registries)
When namespace is empty the separator is omitted: {registry}/{service}:{tag}
Usage: {{ include "kubernaut.image" (dict "service" "gateway" "global" .Values.global "appVersion" .Chart.AppVersion) }}
*/}}
{{- define "kubernaut.image" -}}
{{- $ns := .global.image.namespace | default "" -}}
{{- $sep := .global.image.separator | default "/" -}}
{{- $repo := ternary (printf "%s%s%s" $ns $sep .service) .service (ne $ns "") -}}
{{- if .global.image.digest -}}
{{- printf "%s/%s@%s" .global.image.registry $repo .global.image.digest -}}
{{- else -}}
{{- printf "%s/%s:%s" .global.image.registry $repo (.global.image.tag | default .appVersion) -}}
{{- end -}}
{{- end }}

{{/*
Return the Secret name for PostgreSQL credentials.
When using external PostgreSQL, falls through to the external auth settings.
*/}}
{{- define "kubernaut.postgresql.secretName" -}}
{{- if .Values.postgresql.auth.existingSecret -}}
  {{- .Values.postgresql.auth.existingSecret -}}
{{- else -}}
  postgresql-secret
{{- end -}}
{{- end }}

{{/*
Return the Secret name for DataStorage DB credentials.
DataStorage reads db-secrets.yaml (YAML with username + password) from the
consolidated postgresql-secret. This ensures a single source of truth for DB
credentials, eliminating password mismatch risks (#557).
Precedence: datastorage.dbExistingSecret (deprecated) > postgresql.auth.existingSecret > "postgresql-secret".
*/}}
{{- define "kubernaut.datastorage.dbSecretName" -}}
{{- if .Values.datastorage.dbExistingSecret -}}
{{- .Values.datastorage.dbExistingSecret -}}
{{- else -}}
{{- include "kubernaut.postgresql.secretName" . -}}
{{- end -}}
{{- end }}

{{/*
Return the Secret name for Valkey credentials.
When using external Valkey, falls through to the external settings.
*/}}
{{- define "kubernaut.valkey.secretName" -}}
{{- if .Values.valkey.existingSecret -}}
  {{- .Values.valkey.existingSecret -}}
{{- else -}}
  valkey-secret
{{- end -}}
{{- end }}

{{/*
Return the PostgreSQL host.
Uses in-chart service DNS when postgresql.enabled, otherwise externalPostgresql.host.
*/}}
{{- define "kubernaut.postgresql.host" -}}
{{- if .Values.postgresql.enabled -}}
postgresql.{{ .Release.Namespace }}.svc.cluster.local
{{- else -}}
{{- required "postgresql.host is required when postgresql.enabled=false" .Values.postgresql.host -}}
{{- end -}}
{{- end }}

{{/*
Return the PostgreSQL port.
*/}}
{{- define "kubernaut.postgresql.port" -}}
{{- if .Values.postgresql.enabled -}}
5432
{{- else -}}
{{- .Values.postgresql.port | default 5432 -}}
{{- end -}}
{{- end }}

{{/*
Return the PostgreSQL username (for config files / readiness probes).
*/}}
{{- define "kubernaut.postgresql.username" -}}
{{- .Values.postgresql.auth.username | default "slm_user" -}}
{{- end }}

{{/*
Return the PostgreSQL database name.
*/}}
{{- define "kubernaut.postgresql.database" -}}
{{- .Values.postgresql.auth.database | default "action_history" -}}
{{- end }}

{{/*
Return the PostgreSQL variant ("upstream" or "ocp").
*/}}
{{- define "kubernaut.postgresql.variant" -}}
{{- .Values.postgresql.variant | default "upstream" -}}
{{- end }}

{{/*
Return the env var name for the PostgreSQL user, by variant.
Secret keys are always POSTGRES_*; env var names differ per image.
*/}}
{{- define "kubernaut.postgresql.envVarUser" -}}
{{- if eq (include "kubernaut.postgresql.variant" .) "ocp" -}}POSTGRESQL_USER{{- else -}}POSTGRES_USER{{- end -}}
{{- end }}

{{/*
Return the env var name for the PostgreSQL password, by variant.
*/}}
{{- define "kubernaut.postgresql.envVarPassword" -}}
{{- if eq (include "kubernaut.postgresql.variant" .) "ocp" -}}POSTGRESQL_PASSWORD{{- else -}}POSTGRES_PASSWORD{{- end -}}
{{- end }}

{{/*
Return the env var name for the PostgreSQL database, by variant.
*/}}
{{- define "kubernaut.postgresql.envVarDatabase" -}}
{{- if eq (include "kubernaut.postgresql.variant" .) "ocp" -}}POSTGRESQL_DATABASE{{- else -}}POSTGRES_DB{{- end -}}
{{- end }}

{{/*
Return the data directory mount path for the PostgreSQL volume.
Issue #464: Use a single image-agnostic path so switching between upstream
(postgres:16-alpine) and OCP (rhel10/postgresql-16) images does not change
the data directory and silently lose data.
*/}}
{{- define "kubernaut.postgresql.dataDir" -}}/var/lib/kubernaut-pg/data{{- end -}}

{{/*
Return the Valkey data directory mount path.
upstream: /data   ocp: /var/lib/valkey/data
*/}}
{{- define "kubernaut.valkey.dataDir" -}}
/data
{{- end }}

{{/*
Return the Valkey address (host:port).
*/}}
{{- define "kubernaut.valkey.addr" -}}
{{- if .Values.valkey.enabled -}}
valkey.{{ .Release.Namespace }}.svc.cluster.local:6379
{{- else -}}
{{- $host := required "valkey.host is required when valkey.enabled=false" .Values.valkey.host -}}
{{- printf "%s:%d" $host (int (.Values.valkey.port | default 6379)) -}}
{{- end -}}
{{- end }}

{{/*
Return the in-cluster DataStorage service URL.
Derives the FQDN from .Release.Namespace so the chart works in any namespace.
Issue #753: always HTTPS — inter-service TLS is mandatory.
*/}}
{{- define "kubernaut.datastorage.url" -}}
https://data-storage-service.{{ .Release.Namespace }}.svc.cluster.local:8080
{{- end }}

{{/*
Return the in-cluster Gateway service URL.
Issue #678: switches to https:// when tls.interService.enabled is true.
*/}}
{{- define "kubernaut.gateway.url" -}}
https://gateway-service.{{ .Release.Namespace }}.svc.cluster.local:8080
{{- end }}

{{/*
Inter-service TLS cert directory (server side).
*/}}
{{- define "kubernaut.interServiceTLS.certDir" -}}
{{- if and .Values.tls .Values.tls.interService .Values.tls.interService.certDir -}}
{{- .Values.tls.interService.certDir -}}
{{- else -}}
/etc/tls
{{- end -}}
{{- end -}}

{{/*
Inter-service TLS CA file path (client side).
*/}}
{{- define "kubernaut.interServiceTLS.caFile" -}}
{{- if and .Values.tls .Values.tls.interService .Values.tls.interService.caFile -}}
{{- .Values.tls.interService.caFile -}}
{{- else -}}
/etc/tls-ca/ca.crt
{{- end -}}
{{- end -}}

{{/*
Return the namespace used for workflow execution (Jobs, PipelineRuns).
Defaults to "kubernaut-workflows".
*/}}
{{- define "kubernaut.workflowNamespace" -}}
{{- .Values.workflowexecution.workflowNamespace | default "kubernaut-workflows" -}}
{{- end }}

{{/*
Render a namespace-scoped Role + RoleBinding for configmaps/secrets read access (#229).
Keeps sensitive resources out of ClusterRoles while providing necessary namespace-local access.
Usage: {{ include "kubernaut.nsRoleForSecrets" (dict "name" "gateway" "serviceAccount" "gateway" "Release" .Release "labels" (include "kubernaut.labels" .)) }}
*/}}
{{- define "kubernaut.nsRoleForSecrets" -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ .name }}-ns-role
  namespace: {{ .Release.Namespace }}
  labels:
    app: {{ .serviceAccount }}
    {{- .labels | nindent 4 }}
rules:
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{ .name }}-ns-rolebinding
  namespace: {{ .Release.Namespace }}
  labels:
    app: {{ .serviceAccount }}
    {{- .labels | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ .name }}-ns-role
subjects:
  - kind: ServiceAccount
    name: {{ .serviceAccount }}
    namespace: {{ .Release.Namespace }}
{{- end }}

{{/*
Render optional affinity and topologySpreadConstraints for a component pod spec.
Usage: {{ include "kubernaut.affinity" .Values.gateway | nindent 6 }}
*/}}
{{- define "kubernaut.affinity" -}}
{{- with .affinity }}
affinity:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .topologySpreadConstraints }}
topologySpreadConstraints:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Default pod-level securityContext for the restricted PodSecurity profile.
Override per-component via <component>.podSecurityContext in values.yaml.
Usage: {{ include "kubernaut.podSecurityContext" .Values.gateway | nindent 6 }}
*/}}
{{- define "kubernaut.podSecurityContext" -}}
{{- $defaults := dict "runAsNonRoot" true "seccompProfile" (dict "type" "RuntimeDefault") -}}
{{- $override := .podSecurityContext | default dict -}}
{{- toYaml (merge $override $defaults) }}
{{- end }}

{{/* ===== Unified Monitoring Helpers (Issue #463) ===== */}}

{{/*
Whether the cluster is OCP (presence of route.openshift.io/v1 API).
DEPRECATED v1.4 (Issue #848): OCP auto-detection will be removed in v1.5.
Use the Kubernaut Operator for OpenShift deployments.
*/}}
{{- define "kubernaut.monitoring.isOCP" -}}
{{- if .Capabilities.APIVersions.Has "route.openshift.io/v1" -}}true{{- end -}}
{{- end -}}

{{/*
Whether Prometheus integration is enabled.
*/}}
{{- define "kubernaut.monitoring.prometheus.enabled" -}}
{{- if .Values.monitoring.prometheus.enabled -}}true{{- end -}}
{{- end -}}

{{/*
Resolved Prometheus URL. On OCP, defaults to Thanos querier when empty.
*/}}
{{- define "kubernaut.monitoring.prometheus.url" -}}
{{- if .Values.monitoring.prometheus.url -}}
{{- .Values.monitoring.prometheus.url -}}
{{- else if include "kubernaut.monitoring.isOCP" . -}}
https://prometheus-k8s.openshift-monitoring.svc:9091
{{- end -}}
{{- end -}}

{{/*
Whether AlertManager integration is enabled.
*/}}
{{- define "kubernaut.monitoring.alertManager.enabled" -}}
{{- if .Values.monitoring.alertManager.enabled -}}true{{- end -}}
{{- end -}}

{{/*
Resolved AlertManager URL. On OCP, defaults to alertmanager-main when empty.
*/}}
{{- define "kubernaut.monitoring.alertManager.url" -}}
{{- if .Values.monitoring.alertManager.url -}}
{{- .Values.monitoring.alertManager.url -}}
{{- else if include "kubernaut.monitoring.isOCP" . -}}
https://alertmanager-main.openshift-monitoring.svc:9094
{{- end -}}
{{- end -}}

{{/*
Resolved Prometheus TLS CA file path. On OCP, defaults to service-serving CA.
*/}}
{{- define "kubernaut.monitoring.prometheus.tlsCaFile" -}}
{{- if .Values.monitoring.prometheus.tlsCaFile -}}
{{- .Values.monitoring.prometheus.tlsCaFile -}}
{{- else if include "kubernaut.monitoring.isOCP" . -}}
/etc/ssl/certs/service-ca.crt
{{- end -}}
{{- end -}}

{{/*
Resolved AlertManager TLS CA file path. On OCP, defaults to service-serving CA.
*/}}
{{- define "kubernaut.monitoring.alertManager.tlsCaFile" -}}
{{- if .Values.monitoring.alertManager.tlsCaFile -}}
{{- .Values.monitoring.alertManager.tlsCaFile -}}
{{- else if include "kubernaut.monitoring.isOCP" . -}}
/etc/ssl/certs/service-ca.crt
{{- end -}}
{{- end -}}

{{/*
Whether OCP monitoring RBAC should be created.
True when monitoring is enabled and cluster is OCP.
DEPRECATED v1.4 (Issue #848): OCP RBAC helpers will be removed in v1.5.
Use the Kubernaut Operator for OpenShift deployments.
*/}}
{{- define "kubernaut.monitoring.ocpRbac" -}}
{{- if and (or (include "kubernaut.monitoring.prometheus.enabled" .) (include "kubernaut.monitoring.alertManager.enabled" .)) (include "kubernaut.monitoring.isOCP" .) -}}true{{- end -}}
{{- end -}}

{{/*
Whether TLS CA trust is needed for monitoring connections.
True when any monitoring TLS CA file is configured (explicitly or via OCP defaults).
*/}}
{{- define "kubernaut.monitoring.tlsEnabled" -}}
{{- if or (include "kubernaut.monitoring.prometheus.tlsCaFile" .) (include "kubernaut.monitoring.alertManager.tlsCaFile" .) -}}true{{- end -}}
{{- end -}}

{{/*
Fail-fast validation: reject prometheus.enabled without a resolvable URL.
Invoked once from the EM template to catch misconfig at render time.
*/}}
{{- define "kubernaut.monitoring.validate" -}}
{{- if and (include "kubernaut.monitoring.prometheus.enabled" .) (not (include "kubernaut.monitoring.prometheus.url" .)) -}}
{{- fail "monitoring.prometheus.enabled=true but no URL resolvable. Set monitoring.prometheus.url or deploy on OCP for auto-detection." -}}
{{- end -}}
{{- if and (include "kubernaut.monitoring.alertManager.enabled" .) (not (include "kubernaut.monitoring.alertManager.url" .)) -}}
{{- fail "monitoring.alertManager.enabled=true but no URL resolvable. Set monitoring.alertManager.url or deploy on OCP for auto-detection." -}}
{{- end -}}
{{- end -}}

{{/* ===== Kubernaut Agent TLS Helpers (delegating to monitoring) ===== */}}

{{/*
Kubernaut Agent TLS CA mount directory (single source of truth).
*/}}
{{- define "kubernaut.agent.tlsCaDir" -}}/etc/ssl/kubernaut-agent{{- end -}}

{{/*
Whether Kubernaut Agent TLS CA trust is enabled.
Delegates to unified monitoring TLS detection.
*/}}
{{- define "kubernaut.agent.tlsEnabled" -}}
{{- include "kubernaut.monitoring.tlsEnabled" . -}}
{{- end -}}

{{/*
Name of the ConfigMap containing the CA certificate for Kubernaut Agent.
On OCP, uses chart-created service-CA ConfigMap with auto-injection annotation.
*/}}
{{- define "kubernaut.agent.tlsCaConfigMapName" -}}
kubernaut-agent-service-ca
{{- end -}}

{{/*
Key inside the CA ConfigMap that holds the PEM certificate.
*/}}
{{- define "kubernaut.agent.tlsCaKey" -}}
service-ca.crt
{{- end -}}

{{/*
Default container-level securityContext for the restricted PodSecurity profile.
Override per-component via <component>.containerSecurityContext in values.yaml.
Usage: {{ include "kubernaut.containerSecurityContext" .Values.gateway | nindent 10 }}
*/}}
{{- define "kubernaut.containerSecurityContext" -}}
{{- $defaults := dict "allowPrivilegeEscalation" false "readOnlyRootFilesystem" true "capabilities" (dict "drop" (list "ALL")) -}}
{{- $override := .containerSecurityContext | default dict -}}
{{- toYaml (merge $override $defaults) }}
{{- end }}

{{/* ===== NetworkPolicy Helpers (Issue #285) ===== */}}

{{/*
DNS egress rule: allow UDP+TCP 53 to kube-system.
Usage: {{ include "kubernaut.np.dnsEgress" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.dnsEgress" -}}
- ports:
    - port: 53
      protocol: UDP
    - port: 53
      protocol: TCP
  to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: kube-system
{{- end }}

{{/*
#1712 backport (main PR #1571): replaced the old single-CIDR-at-port-443
mechanism below with auto-discovery of the API server's real backend
endpoint(s). RCA (helios08 live repro): NetworkPolicy ipBlock rules are
evaluated against the post-DNAT destination on most CNIs (confirmed here
against kindnetd) -- an allow rule for the "kubernetes" Service's ClusterIP
(10.96.0.1:443) is silently ignored, and the real backend commonly listens
on 6443, not 443. The old mechanism used the wrong IP *and* the wrong port,
so setting networkPolicies.apiServerCIDR to the ClusterIP (the only option
it exposed) could never have worked once a CNI actually enforced Egress --
it was purely inert under the older kindnet used previously, not a
functioning security control.

Merged, de-duplicated list of API server backend endpoint ipBlock peers, one
per control-plane node. Most real clusters run multiple control-plane nodes
for HA, each a distinct backend endpoint behind the "kubernetes" Service --
every backend IP needs its own allow entry, not just one (see PR #1571
investigation trail upstream).

Precedence: explicit networkPolicies.apiServerCIDR (singular) and/or
apiServerCIDRs (list) always win when set, merged together -- this is the
only path available during `helm template` / GitOps rendering (ArgoCD,
Flux), since `lookup` has no live cluster access there. Otherwise, during a
real `helm install`/`upgrade`, auto-discover every backend address from the
live "kubernetes" Endpoints object so most users never need to set this at
all. If neither an override nor discovery succeeds (e.g. the installer
ServiceAccount lacks permission to read Endpoints), fail loudly with
remediation instructions rather than silently omitting the rule -- pods
would otherwise crash-loop against a default-deny NetworkPolicy with no
indication why.

Renders as a raw (unindented) list of `- ipBlock: {cidr: ...}` entries
usable under either an egress `to:` or ingress `from:` key -- shared because
NetworkPolicyPeer is identical for both. Empty output if apiServerCIDR(s) is
deliberately left unset AND there's no live cluster access (helm template).
Usage: {{ include "kubernaut.np.apiServerPeers" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.apiServerPeers" -}}
{{- $cidrs := list -}}
{{- if .Values.networkPolicies.apiServerCIDR -}}
{{- $cidrs = append $cidrs .Values.networkPolicies.apiServerCIDR -}}
{{- end -}}
{{- if .Values.networkPolicies.apiServerCIDRs -}}
{{- $cidrs = concat $cidrs .Values.networkPolicies.apiServerCIDRs -}}
{{- end -}}
{{- if not $cidrs -}}
{{- if eq (include "kubernaut.hasClusterAccess" .) "true" -}}
{{- $ep := lookup "v1" "Endpoints" "default" "kubernetes" -}}
{{- if $ep -}}
{{- range ($ep.subsets | default list) -}}
{{- range (.addresses | default list) -}}
{{- $cidrs = append $cidrs (printf "%s/32" .ip) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- if not $cidrs -}}
{{- fail "networkPolicies.enabled=true but could not auto-discover the kube-apiserver endpoint (`lookup \"v1\" \"Endpoints\" \"default\" \"kubernetes\"` returned no addresses -- possible causes: the Helm installer ServiceAccount lacks permission to read Endpoints in the default namespace, or this is an unusual cluster). Set networkPolicies.apiServerCIDR (single control-plane) or networkPolicies.apiServerCIDRs (HA, one entry per control-plane node) explicitly -- see `kubectl get endpoints kubernetes -o wide`." -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $lines := list -}}
{{- range (uniq $cidrs) -}}
{{- $lines = append $lines (printf "- ipBlock:\n    cidr: %s" .) -}}
{{- end -}}
{{- join "\n" $lines -}}
{{- end }}

{{/*
Port the API server's backend endpoint(s) listen on (commonly 6443, not the
"kubernetes" Service's port 443). Precedence: explicit
networkPolicies.apiServerPort (nonzero) always wins; otherwise, when no
apiServerCIDR(s) override is set either, auto-discovered from the same live
Endpoints lookup as kubernaut.np.apiServerPeers; otherwise defaults to 443
(only correct if the real backend happens to also listen on 443).
Usage: {{ include "kubernaut.np.apiServerPort" . }}
*/}}
{{- define "kubernaut.np.apiServerPort" -}}
{{- $port := 0 -}}
{{- if .Values.networkPolicies.apiServerPort -}}
{{- $port = .Values.networkPolicies.apiServerPort -}}
{{- else if and (not .Values.networkPolicies.apiServerCIDR) (not .Values.networkPolicies.apiServerCIDRs) (eq (include "kubernaut.hasClusterAccess" .) "true") -}}
{{- $ep := lookup "v1" "Endpoints" "default" "kubernetes" -}}
{{- if $ep -}}
{{- $subsets := $ep.subsets | default list -}}
{{- if gt (len $subsets) 0 -}}
{{- $ports := (index $subsets 0).ports | default list -}}
{{- if gt (len $ports) 0 -}}
{{- $port = (index $ports 0).port -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- if not $port -}}{{- $port = 443 -}}{{- end -}}
{{- $port -}}
{{- end }}

{{/*
K8s API server egress rule: allow TCP to the configured API server backend
endpoint CIDR(s). See kubernaut.np.apiServerPeers for the CIDR discovery
logic.
Usage: {{ include "kubernaut.np.apiServerEgress" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.apiServerEgress" -}}
{{- $peers := include "kubernaut.np.apiServerPeers" . -}}
{{- if $peers }}
- ports:
    - port: {{ include "kubernaut.np.apiServerPort" . }}
      protocol: TCP
  to:
    {{- $peers | nindent 4 }}
{{- end }}
{{- end }}

{{/*
Common egress rules included in every NetworkPolicy: DNS + API server.
Usage: {{ include "kubernaut.np.commonEgress" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.commonEgress" -}}
{{- include "kubernaut.np.dnsEgress" . }}
{{- include "kubernaut.np.apiServerEgress" . }}
{{- end }}

{{/*
DataStorage egress rule: allow TCP 8080 to datastorage pods.
Usage: {{ include "kubernaut.np.datastorageEgress" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.datastorageEgress" -}}
- ports:
    - port: 8080
      protocol: TCP
  to:
    - podSelector:
        matchLabels:
          app: datastorage
{{- end }}

{{/*
Metrics scraping ingress rule: allow Prometheus scrape from monitoring namespace.
Usage: {{ include "kubernaut.np.metricsIngress" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.metricsIngress" -}}
{{- if .Values.networkPolicies.monitoring.namespace }}
- ports:
    - port: 9090
      protocol: TCP
  from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: {{ .Values.networkPolicies.monitoring.namespace }}
{{- end }}
{{- end }}
