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
Whether we have real cluster access for `lookup`-based auto-discovery of
values that would otherwise require an explicit override. During
`helm template` / `helm install --dry-run=client` -- and for GitOps
controllers (ArgoCD, Flux) that render manifests via `helm template`
internally rather than a live install/upgrade -- `lookup` always returns
empty, so those callers MUST set the relevant override value(s) explicitly;
auto-discovery only works for a real `helm install`/`upgrade` against a live
cluster. Mirrors the canary in templates/infrastructure/secrets.yaml.
Usage: {{ if eq (include "kubernaut.hasClusterAccess" .) "true" }}...{{ end }}
*/}}
{{- define "kubernaut.hasClusterAccess" -}}
{{- if lookup "v1" "Namespace" "" "kube-system" -}}true{{- end -}}
{{- end }}

{{/*
cert-manager Issuer/ClusterIssuer name for tls.mode=cert-manager.

Precedence: explicit tls.certManager.issuerRef.name always wins (the only
path available during `helm template` / GitOps rendering, since `lookup`
has no live cluster access there). Otherwise, during a real
`helm install`/`upgrade`, auto-select it if exactly one
Issuer/ClusterIssuer (per tls.certManager.issuerRef.kind) exists in the
cluster -- most clusters only run one. Fails loudly with remediation
instructions if the name can't be determined either way: zero found (no
cert-manager issuer provisioned), 2+ found (ambiguous -- picking the wrong
CA silently would be a security-relevant mistake, never guess), or no live
cluster access (helm template/GitOps must set this explicitly).
Usage: {{ include "kubernaut.tls.issuerName" . }}
*/}}
{{- define "kubernaut.tls.issuerName" -}}
{{- if .Values.tls.certManager.issuerRef.name -}}
{{- .Values.tls.certManager.issuerRef.name -}}
{{- else if eq (include "kubernaut.hasClusterAccess" .) "true" -}}
{{- $kind := .Values.tls.certManager.issuerRef.kind -}}
{{- $apiVersion := printf "%s/v1" .Values.tls.certManager.issuerRef.group -}}
{{- $ns := "" -}}
{{- if eq $kind "Issuer" -}}{{- $ns = .Release.Namespace -}}{{- end -}}
{{- $result := lookup $apiVersion $kind $ns "" -}}
{{- $issuers := ($result.items | default list) -}}
{{- if eq (len $issuers) 1 -}}
{{- (index $issuers 0).metadata.name -}}
{{- else if eq (len $issuers) 0 -}}
{{- fail (printf "tls.mode=cert-manager but no %s was found and tls.certManager.issuerRef.name is not set. Install cert-manager and create a %s, or set tls.certManager.issuerRef.name explicitly (e.g. \"letsencrypt-prod\" or \"selfsigned-issuer\")." $kind $kind) -}}
{{- else -}}
{{- $names := list -}}
{{- range $issuers -}}{{- $names = append $names .metadata.name -}}{{- end -}}
{{- fail (printf "tls.mode=cert-manager but tls.certManager.issuerRef.name is not set and multiple %ss exist, so auto-selection is ambiguous. Set tls.certManager.issuerRef.name to one of: %s" $kind (join ", " $names)) -}}
{{- end -}}
{{- else -}}
{{- fail "tls.certManager.issuerRef.name is required when tls.mode=cert-manager and rendering without live cluster access (helm template / GitOps via ArgoCD or Flux always renders this way) -- auto-discovery only works during a real helm install/upgrade." -}}
{{- end -}}
{{- end }}

{{/*
Merged fleet OAuth2 config: a service's own fleet.oauth2 fields fall back to
global.fleet.oauth2 when unset, since every fleet-integration-capable
service (gateway, signalprocessing, remediationorchestrator,
effectivenessmonitor, apifrontend, fleetmetadatacache) authenticates to the
*same* MCP Gateway with the same OAuth2 client in practice -- set it once in
global.fleet.oauth2 instead of duplicating it per service. Per-service
`fleet.oauth2.enabled` (fleet integration on/off) is intentionally NOT
handled here and stays independent per service.
Named templates can only return a string, so the merged config is
serialized as YAML -- parse it back with `fromYaml` at the call site.
Usage:
  {{- $o := include "kubernaut.fleet.oauth2" (dict "root" $ "svc" .Values.gateway.fleet.oauth2) | fromYaml }}
  {{ $o.tokenURL }}
*/}}
{{- define "kubernaut.fleet.oauth2" -}}
{{- $g := .root.Values.global.fleet.oauth2 -}}
{{- $svc := .svc -}}
{{- dict
    "tokenURL" ($svc.tokenURL | default $g.tokenURL)
    "credentialsSecretRef" ($svc.credentialsSecretRef | default $g.credentialsSecretRef)
    "scopes" ($svc.scopes | default $g.scopes)
    "tlsCAFile" ($svc.tlsCAFile | default $g.tlsCAFile)
  | toYaml -}}
{{- end }}

{{/*
Merged fleet MCP Gateway config (endpoint/type/CA, distinct from OAuth2
credentials -- see kubernaut.fleet.oauth2): a service's own
fleet.mcpGatewayEndpoint/mcpGatewayType/tlsCAFile fall back to
global.fleet.* when unset, since every fleet-integration-capable service
points at the *same* physical MCP Gateway instance. Uses sprig `get` (not
dot access) so this also works for callers whose `svc` dict doesn't declare
one of these keys at all (e.g. signalprocessing has no top-level
fleet.tlsCAFile) without erroring.
Usage:
  {{- $f := include "kubernaut.fleet.config" (dict "root" $ "svc" .Values.gateway.fleet) | fromYaml }}
  {{ $f.mcpGatewayEndpoint }}
*/}}
{{- define "kubernaut.fleet.config" -}}
{{- $g := .root.Values.global.fleet -}}
{{- $svc := .svc -}}
{{- dict
    "mcpGatewayEndpoint" ((get $svc "mcpGatewayEndpoint") | default $g.mcpGatewayEndpoint)
    "mcpGatewayType" ((get $svc "mcpGatewayType") | default $g.mcpGatewayType)
    "tlsCAFile" ((get $svc "tlsCAFile") | default $g.tlsCAFile)
  | toYaml -}}
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
Return the Secret name for the DataStorage audit hash-chain HMAC key
(GAP-05, Issue #1505). Only relevant when datastorage.config.auditHashKey.enabled.
Precedence: datastorage.config.auditHashKey.existingSecret > "datastorage-audit-hmac-key".
*/}}
{{- define "kubernaut.datastorage.auditHashKeySecretName" -}}
{{- if .Values.datastorage.config.auditHashKey.existingSecret -}}
{{- .Values.datastorage.config.auditHashKey.existingSecret -}}
{{- else -}}
  datastorage-audit-hmac-key
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
Return the env var name for the PostgreSQL user.
Secret keys are always POSTGRES_*; kept as a helper for a single source of truth
across postgresql.yaml/valkey.yaml/datastorage.yaml.
*/}}
{{- define "kubernaut.postgresql.envVarUser" -}}POSTGRES_USER{{- end -}}

{{/*
Return the env var name for the PostgreSQL password.
*/}}
{{- define "kubernaut.postgresql.envVarPassword" -}}POSTGRES_PASSWORD{{- end -}}

{{/*
Return the env var name for the PostgreSQL database.
*/}}
{{- define "kubernaut.postgresql.envVarDatabase" -}}POSTGRES_DB{{- end -}}

{{/*
Return the data directory mount path for the PostgreSQL volume.
Issue #464: Use a single image-agnostic path.
*/}}
{{- define "kubernaut.postgresql.dataDir" -}}/var/lib/kubernaut-pg/data{{- end -}}

{{/*
Return the Valkey data directory mount path.
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
Return the in-cluster FleetMetadataCache service URL.
FleetMetadataCache uses HTTP by default (internal scope query API, ADR-068).
*/}}
{{- define "kubernaut.fleetmetadatacache.url" -}}
http://fleetmetadatacache-service.{{ .Release.Namespace }}.svc.cluster.local:8080
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
Render affinity and topologySpreadConstraints for a component pod spec.
DD-PLATFORM-004: injects a default soft (preferred, weight 100) pod
anti-affinity spreading replicas across nodes by the given matchLabels,
merged with any user-supplied .affinity override — user values win on
conflicting keys, sibling keys (e.g. nodeAffinity, or an explicit empty
preferredDuringSchedulingIgnoredDuringExecution: [] to opt out) merge
additively. Ported from the Kubernaut Operator's preferredPodAntiAffinity
(kubernaut-operator/internal/resources/deployments.go).
Usage: {{ include "kubernaut.affinity" (dict "component" .Values.gateway "matchLabels" (dict "app" "gateway")) | nindent 6 }}
*/}}
{{- define "kubernaut.affinity" -}}
{{- $component := .component -}}
{{- $defaultAntiAffinity := dict "podAntiAffinity" (dict "preferredDuringSchedulingIgnoredDuringExecution" (list (dict "weight" 100 "podAffinityTerm" (dict "topologyKey" "kubernetes.io/hostname" "labelSelector" (dict "matchLabels" .matchLabels))))) -}}
{{- $userAffinity := $component.affinity | default dict -}}
affinity:
  {{- toYaml (merge $userAffinity $defaultAntiAffinity) | nindent 2 }}
{{- with $component.topologySpreadConstraints }}
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
Whether Prometheus integration is enabled.
*/}}
{{- define "kubernaut.monitoring.prometheus.enabled" -}}
{{- if .Values.monitoring.prometheus.enabled -}}true{{- end -}}
{{- end -}}

{{/*
Resolved Prometheus URL.
*/}}
{{- define "kubernaut.monitoring.prometheus.url" -}}
{{- .Values.monitoring.prometheus.url -}}
{{- end -}}

{{/*
Whether AlertManager integration is enabled.
*/}}
{{- define "kubernaut.monitoring.alertManager.enabled" -}}
{{- if .Values.monitoring.alertManager.enabled -}}true{{- end -}}
{{- end -}}

{{/*
Resolved AlertManager URL.
*/}}
{{- define "kubernaut.monitoring.alertManager.url" -}}
{{- .Values.monitoring.alertManager.url -}}
{{- end -}}

{{/*
Resolved Prometheus TLS CA file path.
*/}}
{{- define "kubernaut.monitoring.prometheus.tlsCaFile" -}}
{{- .Values.monitoring.prometheus.tlsCaFile -}}
{{- end -}}

{{/*
Resolved AlertManager TLS CA file path.
*/}}
{{- define "kubernaut.monitoring.alertManager.tlsCaFile" -}}
{{- .Values.monitoring.alertManager.tlsCaFile -}}
{{- end -}}

{{/*
Whether TLS CA trust is needed for monitoring connections.
True when any monitoring TLS CA file is explicitly configured.
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
{{- fail "monitoring.prometheus.enabled=true but no URL resolvable. Set monitoring.prometheus.url." -}}
{{- end -}}
{{- if and (include "kubernaut.monitoring.alertManager.enabled" .) (not (include "kubernaut.monitoring.alertManager.url" .)) -}}
{{- fail "monitoring.alertManager.enabled=true but no URL resolvable. Set monitoring.alertManager.url." -}}
{{- end -}}
{{- end -}}

{{/* ===== ServiceMonitor / PrometheusRule / HPA helpers (BR-PLATFORM-003, Issue #1589) ===== */}}

{{/*
Whether the Prometheus Operator CRDs (monitoring.coreos.com/v1) are present on the target
cluster. Always false under `helm template`/`helm lint` (no live cluster, .Capabilities reflects
only what --api-versions passes in). Used to gate ServiceMonitor/PrometheusRule rendering so
enabling monitoring.serviceMonitor/prometheusRule without the CRDs installed renders nothing
instead of failing with "no matches for kind".
*/}}
{{- define "kubernaut.monitoring.crdsPresent" -}}
{{- if .Capabilities.APIVersions.Has "monitoring.coreos.com/v1" -}}true{{- end -}}
{{- end -}}

{{/*
Render a ServiceMonitor for one Kubernaut service, gated on monitoring.serviceMonitor.enabled +
CRD presence. Mirrors the Kubernaut Operator's componentServiceMonitor helper
(kubernaut-operator/internal/resources/monitoring.go) for parity.
Usage: {{ include "kubernaut.serviceMonitor" (dict "root" . "service" "gateway") }}
- "appLabel": the Service's `app` label to select on, when it differs from "service"
  (several controllers use a "-controller" suffixed app label, e.g. aianalysis-controller).
- "jobName": the "job" relabel value, when it differs from "service" (none currently do).
*/}}
{{- define "kubernaut.serviceMonitor" -}}
{{- $root := .root -}}
{{- $service := .service -}}
{{- $appLabel := .appLabel | default .service -}}
{{- $job := .jobName | default .service -}}
{{- if and $root.Values.monitoring.serviceMonitor.enabled (include "kubernaut.monitoring.crdsPresent" $root) }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ $service }}-monitor
  namespace: {{ $root.Release.Namespace }}
  labels:
    {{- include "kubernaut.labels" $root | nindent 4 }}
    app: {{ $appLabel }}
spec:
  jobLabel: app.kubernetes.io/name
  selector:
    matchLabels:
      app: {{ $appLabel }}
  namespaceSelector:
    matchNames:
      - {{ $root.Release.Namespace }}
  endpoints:
    - port: metrics
      path: /metrics
      interval: 15s
      relabelings:
        - sourceLabels: ["__address__"]
          targetLabel: job
          replacement: {{ $job }}
{{- end -}}
{{- end }}

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
BYO (Issue #848 v1.5): the chart no longer creates this ConfigMap — operators must
pre-create it with real CA data when monitoring.prometheus/alertManager.tlsCaFile is set.
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
Merged, de-duplicated list of API server backend endpoint ipBlock peers, one
per control-plane node. Most real clusters run multiple control-plane nodes
for HA, each a distinct backend endpoint behind the "kubernetes" Service --
ipBlock rules are evaluated against the post-DNAT destination on most CNIs,
so every backend IP needs its own allow entry, not just one (see PR #1571
investigation trail).

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

{{/* ===== Console helpers (BR-PLATFORM-006, Kubernaut Operator parity) ===== */}}

{{/*
Derive the OIDC issuer URL for the console's oauth2-proxy from APIFrontend's auth
config. Mirrors the Kubernaut Operator's KubernautSpec.ConsoleIssuerURL(): the first
jwtProviders entry takes precedence over the single-provider issuerURL shortcut.
Usage: {{ include "kubernaut.console.issuerURL" . }}
*/}}
{{- define "kubernaut.console.issuerURL" -}}
{{- $providers := .Values.apifrontend.config.auth.jwtProviders | default list -}}
{{- if gt (len $providers) 0 -}}
{{- (first $providers).issuerURL -}}
{{- else -}}
{{- .Values.apifrontend.config.auth.issuerURL -}}
{{- end -}}
{{- end }}

{{/*
In-cluster APIFrontend URL the console's nginx sidecar reverse-proxies to.
Usage: {{ include "kubernaut.console.apifrontendURL" . }}
*/}}
{{- define "kubernaut.console.apifrontendURL" -}}
{{- printf "https://apifrontend.%s.svc:%v" .Release.Namespace (.Values.apifrontend.config.server.httpPort | default 8443) -}}
{{- end }}
