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
{{- $explicitIssuerName := dig "certManager" "issuerRef" "name" "" .Values.tls -}}
{{- if $explicitIssuerName -}}
{{- $explicitIssuerName -}}
{{- else if eq (include "kubernaut.hasClusterAccess" .) "true" -}}
{{- $tlsV := include "kubernaut.mergedValues" (dict "root" . "service" "tls") | fromYaml -}}
{{- $kind := $tlsV.certManager.issuerRef.kind -}}
{{- $apiVersion := printf "%s/v1" $tlsV.certManager.issuerRef.group -}}
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
Merged fleet OAuth2 config (Issue #1707 follow-up): every field except
credentialsSecretRef is now global-only -- backend/endpoint/mcpGatewayType/
tlsCAFile/oauth2.enabled/tokenURL/scopes/tlsCAFile were removed from every
per-service `fleet.*` schema, since every fleet-integration-capable service
(gateway, signalprocessing, remediationorchestrator, effectivenessmonitor,
apifrontend, fleetmetadatacache) authenticates to the *same* MCP Gateway
with the same OAuth2 client in practice -- there is no known deployment that
needs a different value per service. Only `oauth2.credentialsSecretRef`
remains overridable per service (the one field ADR-068 anticipated could
legitimately differ, e.g. a per-namespace Secret naming convention).
Uses sprig `get` (not dot access) so this also works for callers whose
`svc` dict doesn't declare one of these keys at all (e.g. fleetmetadatacache's
own oauth2 dict only ever declared credentialsSecretRef) without erroring.
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
    "enabled" $g.enabled
    "tokenURL" $g.tokenURL
    "credentialsSecretRef" ((get $svc "credentialsSecretRef") | default $g.credentialsSecretRef)
    "scopes" $g.scopes
    "tlsCAFile" $g.tlsCAFile
  | toYaml -}}
{{- end }}

{{/*
Merged fleet MCP Gateway config (endpoint/type/CA/backend/token, distinct
from OAuth2 credentials -- see kubernaut.fleet.oauth2). Issue #1707
follow-up: backend/endpoint/mcpGatewayEndpoint/mcpGatewayType/tlsCAFile/
tokenSecretRef are now global-only -- removed from every per-service
`fleet.*` schema, since every fleet-integration-capable service points at
the *same* physical MCP Gateway instance and (for gateway/
remediationorchestrator) the same scope-check backend. `svc` is still
accepted and still consulted via sprig `get` (not dot access, so this also
works for callers whose `svc` dict doesn't declare one of these keys at all,
e.g. fleetmetadatacache's own dict never had a `backend` key) purely so a
future per-service exception could be reintroduced without changing this
helper's signature again -- today it always resolves to the global value.

`namespace` (Issue #1730) is the one exception that IS still genuinely
per-service (signalprocessing.fleet.namespace, fleetmetadatacache's
top-level namespace): it falls back to global.fleet.mcpGatewayNamespace
only when the caller's own field is unset, rather than always resolving to
the global value like every other key above. Callers that don't declare a
`namespace` key at all (e.g. gateway/remediationorchestrator/
workflowexecution's `svc` dicts) simply get the global fallback verbatim,
which is harmless since they don't consume this output key.
Usage:
  {{- $f := include "kubernaut.fleet.config" (dict "root" $ "svc" .Values.gateway.fleet) | fromYaml }}
  {{ $f.mcpGatewayEndpoint }}
*/}}
{{- define "kubernaut.fleet.config" -}}
{{- $g := .root.Values.global.fleet -}}
{{- $svc := .svc -}}
{{- dict
    "backend" ((get $svc "backend") | default $g.backend)
    "endpoint" ((get $svc "endpoint") | default $g.endpoint)
    "mcpGatewayEndpoint" ((get $svc "mcpGatewayEndpoint") | default $g.mcpGatewayEndpoint)
    "mcpGatewayType" ((get $svc "mcpGatewayType") | default $g.mcpGatewayType)
    "namespace" ((get $svc "namespace") | default $g.mcpGatewayNamespace)
    "tlsCAFile" ((get $svc "tlsCAFile") | default $g.tlsCAFile)
    "tokenSecretRef" ((get $svc "tokenSecretRef") | default $g.tokenSecretRef)
  | toYaml -}}
{{- end }}

{{/*
Combines kubernaut.fleet.oauth2 and kubernaut.fleet.config into a single
call, replacing the two-line `include | fromYaml` preamble every
fleet-capable service (gateway, remediationorchestrator, signalprocessing,
effectivenessmonitor, apifrontend, fleetmetadatacache) previously repeated
at its own top. Returns a dict with "oauth2" and "config" keys (both
already fromYaml-parsed).
Usage:
  {{- $f := include "kubernaut.fleet.preamble" (dict "root" $ "fleet" .Values.gateway.fleet "oauth2" .Values.gateway.fleet.oauth2) | fromYaml -}}
  {{ $f.oauth2.tokenURL }} / {{ $f.config.mcpGatewayEndpoint }}
*/}}
{{- define "kubernaut.fleet.preamble" -}}
{{- $oauth2 := include "kubernaut.fleet.oauth2" (dict "root" .root "svc" .oauth2) | fromYaml -}}
{{- $config := include "kubernaut.fleet.config" (dict "root" .root "svc" .fleet) | fromYaml -}}
{{- dict "oauth2" $oauth2 "config" $config | toYaml -}}
{{- end }}

{{/*
HorizontalPodAutoscaler (autoscaling/v2) for a component, targeting a
same-named Deployment. Ported from the Kubernaut Operator's HPA builders
(kubernaut-operator/internal/resources/hpa.go); identical across every
autoscaling-capable service — only name/autoscaling values vary.
Callers MUST pass the schema-default-merged `autoscaling` block (via
`kubernaut.mergedValues`), not `.Values.<service>.autoscaling` directly --
`cpuTarget`/`memoryTarget`/`minReplicas`/`maxReplicas` have no literal entry
in values.yaml post-DD-PLATFORM-006, so a raw `.Values` read yields an empty
`averageUtilization` and a Kubernetes API rejection the moment autoscaling is
enabled without every field also being explicitly set.
Usage: {{ $v := include "kubernaut.mergedValues" (dict "root" $ "service" "datastorage") | fromYaml }}
       {{ include "kubernaut.hpa" (dict "root" $ "name" "datastorage" "autoscaling" $v.autoscaling) }}
*/}}
{{- define "kubernaut.hpa" -}}
{{- if .autoscaling.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ .name }}
  namespace: {{ .root.Release.Namespace }}
  labels:
    {{- include "kubernaut.labels" .root | nindent 4 }}
    app: {{ .name }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ .name }}
  minReplicas: {{ .autoscaling.minReplicas }}
  maxReplicas: {{ .autoscaling.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .autoscaling.cpuTarget }}
    {{- if gt (.autoscaling.memoryTarget | int) 0 }}
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: {{ .autoscaling.memoryTarget }}
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

DD-PLATFORM-006 Decision Area 14 (PR9): takes "root" (the top-level `$`/`.`),
not a pre-fetched "global" dict, and resolves global.image.* through
kubernaut.mergedValues -- registry/namespace/separator all have non-zero
schema defaults ("quay.io"/"kubernaut-ai"/"/"), so once global.image is
removed from values.yaml a raw .Values.global.image.* read would silently
regress to an empty/wrong value instead of the schema default.
Usage: {{ include "kubernaut.image" (dict "service" "gateway" "root" $ "appVersion" .Chart.AppVersion) }}
*/}}
{{- define "kubernaut.image" -}}
{{- $g := include "kubernaut.mergedValues" (dict "root" .root "service" "global") | fromYaml -}}
{{- $ns := $g.image.namespace -}}
{{- $sep := $g.image.separator -}}
{{- $repo := ternary (printf "%s%s%s" $ns $sep .service) .service (ne $ns "") -}}
{{- if $g.image.digest -}}
{{- printf "%s/%s@%s" $g.image.registry $repo $g.image.digest -}}
{{- else -}}
{{- printf "%s/%s:%s" $g.image.registry $repo (default .appVersion $g.image.tag) -}}
{{- end -}}
{{- end }}

{{/*
Return the effective image pull policy (global.image.pullPolicy, schema
default "IfNotPresent"). DD-PLATFORM-006 Decision Area 14 (PR9): routed
through kubernaut.mergedValues for the same reason as kubernaut.image --
once global.image is removed from values.yaml, a raw .Values read (with no
`| default` guard at all today) would render an empty imagePullPolicy.
Usage: {{ include "kubernaut.imagePullPolicy" $ }}
*/}}
{{- define "kubernaut.imagePullPolicy" -}}
{{- (include "kubernaut.mergedValues" (dict "root" . "service" "global") | fromYaml).image.pullPolicy -}}
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
(GAP-05, Issue #1505). Mandatory, no enabled gate (DD-PLATFORM-006 DA6).
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
{{- $v := include "kubernaut.mergedValues" (dict "root" . "service" "postgresql") | fromYaml -}}
{{- if $v.enabled -}}
postgresql.{{ .Release.Namespace }}.svc.cluster.local
{{- else -}}
{{- required "postgresql.host is required when postgresql.enabled=false" .Values.postgresql.host -}}
{{- end -}}
{{- end }}

{{/*
Return the PostgreSQL port.
*/}}
{{- define "kubernaut.postgresql.port" -}}
{{- $v := include "kubernaut.mergedValues" (dict "root" . "service" "postgresql") | fromYaml -}}
{{- if $v.enabled -}}
5432
{{- else -}}
{{- $v.port -}}
{{- end -}}
{{- end }}

{{/*
Return the PostgreSQL username (for config files / readiness probes).
*/}}
{{- define "kubernaut.postgresql.username" -}}
{{- (include "kubernaut.mergedValues" (dict "root" . "service" "postgresql") | fromYaml).auth.username -}}
{{- end }}

{{/*
Return the PostgreSQL database name.
*/}}
{{- define "kubernaut.postgresql.database" -}}
{{- (include "kubernaut.mergedValues" (dict "root" . "service" "postgresql") | fromYaml).auth.database -}}
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
DD-PLATFORM-006 DA8: the in-chart Valkey is TLS-only on 6380. The BYO branch
(valkey.enabled=false) is unaffected -- an external Valkey/Redis's port is the
operator's own concern (default 6379 matches upstream Redis/Valkey's own
plaintext default, not this chart's TLS decision).
*/}}
{{- define "kubernaut.valkey.addr" -}}
{{- $v := include "kubernaut.mergedValues" (dict "root" . "service" "valkey") | fromYaml -}}
{{- if $v.enabled -}}
valkey.{{ .Release.Namespace }}.svc.cluster.local:6380
{{- else -}}
{{- $host := required "valkey.host is required when valkey.enabled=false" .Values.valkey.host -}}
{{- printf "%s:%d" $host (int $v.port) -}}
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
Issue #1683: FleetMetadataCache's API port presents TLS by default
(ConfigureConditionalTLS), matching every other Kubernaut HTTP-API service.
*/}}
{{- define "kubernaut.fleetmetadatacache.url" -}}
https://fleetmetadatacache-service.{{ .Release.Namespace }}.svc.cluster.local:8080
{{- end }}

{{/*
DD-PLATFORM-006 Decision Area 10: derive fleetmetadatacache.enabled's default from
global.fleet.enabled + global.fleet.backend instead of requiring a second, easy-to-miss manual
toggle. Gateway/RemediationOrchestrator's own fleet config already resolves an empty
global.fleet.backend to "fleetmetadatacache" (see gateway.yaml/remediationorchestrator.yaml), so
there is no scenario where global.fleet.enabled=true and the resolved backend is
"fleetmetadatacache" but FMC should stay off. An explicit, contradictory override
(fleetmetadatacache.enabled: false while the derived default would be true) has no sane
interpretation -- detected via hasKey (not `default`/`coalesce`, which can't distinguish "unset"
from "explicitly false"). Issue #1984 Phase C: this contradictory-override case is now rejected by
a values.schema.json `not`/`allOf` block (BR-PLATFORM-010) before this helper ever runs, so the
`hasKey`+false branch below is only reachable for the non-contradictory
fleetmetadatacache.enabled=false case.
Usage: {{ include "kubernaut.fleetmetadatacache.effectiveEnabled" . }} -- renders the literal
string "true" or "false"; compare with `eq (include "kubernaut.fleetmetadatacache.effectiveEnabled" .) "true"`.
*/}}
{{- define "kubernaut.fleetmetadatacache.effectiveEnabled" -}}
{{- $backend := default "fleetmetadatacache" .Values.global.fleet.backend -}}
{{- $derivedDefault := and .Values.global.fleet.enabled (eq $backend "fleetmetadatacache") -}}
{{- if hasKey .Values.fleetmetadatacache "enabled" -}}
  {{- printf "%t" .Values.fleetmetadatacache.enabled -}}
{{- else -}}
  {{- printf "%t" $derivedDefault -}}
{{- end -}}
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
{{- (include "kubernaut.mergedValues" (dict "root" . "service" "tls") | fromYaml).interService.certDir -}}
{{- end -}}

{{/*
Inter-service TLS CA file path (client side).
*/}}
{{- define "kubernaut.interServiceTLS.caFile" -}}
{{- (include "kubernaut.mergedValues" (dict "root" . "service" "tls") | fromYaml).interService.caFile -}}
{{- end -}}

{{/*
Return the namespace used for workflow execution (Jobs, PipelineRuns).
Defaults to "kubernaut-workflows".
*/}}
{{- define "kubernaut.workflowNamespace" -}}
{{- (include "kubernaut.mergedValues" (dict "root" . "service" "workflowexecution") | fromYaml).workflowNamespace -}}
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
Render one ClusterRoleBinding per entry in a service's
additionalClusterRoleBindings list, binding an operator-supplied,
pre-existing ClusterRole (referenced by name only) to the given
ServiceAccount. Generalizes kubernaut-operator's KubernautAgent-only
AdditionalClusterRoleBindings mechanism to any Helm-chart service (#1069,
DD-GATEWAY-018): it lets operators grant Kubernaut read access to resource
kinds Kubernaut doesn't anticipate or ship RBAC for (e.g. OLM, ArgoCD,
Kafka/Strimzi, custom CRDs), without a Kubernaut code change, while keeping
the grant declarative and GitOps-tracked in the same values.yaml as the rest
of the deployment. The operator creates and owns the referenced ClusterRole;
Kubernaut only binds it -- no PolicyRule content is authored by this chart.

Params (dict):
  - prefix: naming prefix for generated ClusterRoleBindings, e.g. "gateway"
  - serviceAccount: name of the ServiceAccount to bind
  - names: list of pre-existing ClusterRole names to reference (may be empty/nil)
  - Release: .Release (for the ServiceAccount's namespace)
  - labels: rendered `kubernaut.labels` output
*/}}
{{- define "kubernaut.additionalClusterRoleBindings" -}}
{{- $prefix := .prefix -}}
{{- $sa := .serviceAccount -}}
{{- $labels := .labels -}}
{{- $release := .Release -}}
{{- range .names }}
{{- $crName := . -}}
{{- $crbName := printf "%s-ext-%s" $prefix $crName -}}
{{- if gt (len $crbName) 253 }}
{{- $hash := sha256sum $crName | trunc 8 -}}
{{- $maxRoleLen := sub (sub (sub 253 (len (printf "%s-ext-" $prefix))) 1) 8 -}}
{{- $crbName = printf "%s-ext-%s-%s" $prefix (trunc $maxRoleLen $crName) $hash -}}
{{- end }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ $crbName }}
  labels:
    {{- $labels | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ $crName }}
subjects:
  - kind: ServiceAccount
    name: {{ $sa }}
    namespace: {{ $release.Namespace }}
{{- end }}
{{- end -}}

{{/*
Render a namespace-scoped Role + RoleBinding granting a fleet
ClusterRegistry's CRD watch (MCPServerRegistration for kuadrant;
Backend/MCPRoute for eaigw), used in place of the equivalent cluster-wide
ClusterRole rule whenever a service's fleet namespace-scoping knob
(<service>.fleet.namespace / fleetmetadatacache.namespace) is set (#1686,
BR-RBAC-020). Each rule is opted in independently via a bool so the exact
rule content (apiGroups/resources/verbs) matches the ClusterRole variant it
replaces verbatim -- callers copy the same literal rule block used in their
own ClusterRole rather than re-typing it, so the two RBAC kinds cannot drift
apart.
Params:
  name                    - resource name prefix (e.g. "apifrontend")
  serviceAccount          - ServiceAccount name to bind (usually == name)
  namespace               - namespace to scope the watch/RBAC to
  appLabels               - pre-rendered "app identifying" label line(s),
                             e.g. "app: apifrontend" or FMC's two-line
                             app.kubernetes.io/name+component convention
  mcpServerRegistrations  - bool: grant mcp.kuadrant.io/mcpserverregistrations
  kuadrantGateways        - bool: grant gateway.networking.k8s.io/gateways+httproutes (FMC only)
  eaigwBackends           - bool: grant gateway.envoyproxy.io/backends + aigateway.envoyproxy.io/mcproutes
  Release, labels         - as in kubernaut.nsRoleForSecrets above
Usage: {{ include "kubernaut.fleet.registryNsRBAC" (dict "name" "apifrontend" "serviceAccount" "apifrontend" "namespace" $ns "appLabels" "app: apifrontend" "mcpServerRegistrations" true "Release" .Release "labels" (include "kubernaut.labels" .)) }}
*/}}
{{- define "kubernaut.fleet.registryNsRBAC" -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ .name }}-fleet-registry
  namespace: {{ .namespace }}
  labels:
    {{- .appLabels | nindent 4 }}
    {{- .labels | nindent 4 }}
rules:
  {{- if .mcpServerRegistrations }}
  - apiGroups: ["mcp.kuadrant.io"]
    resources: ["mcpserverregistrations"]
    verbs: ["get", "list", "watch"]
  {{- end }}
  {{- if .kuadrantGateways }}
  - apiGroups: ["gateway.networking.k8s.io"]
    resources: ["gateways", "httproutes"]
    verbs: ["get", "list", "watch"]
  {{- end }}
  {{- if .eaigwBackends }}
  - apiGroups: ["gateway.envoyproxy.io"]
    resources: ["backends"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["aigateway.envoyproxy.io"]
    resources: ["mcproutes"]
    verbs: ["get", "list", "watch"]
  {{- end }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{ .name }}-fleet-registry-binding
  namespace: {{ .namespace }}
  labels:
    {{- .appLabels | nindent 4 }}
    {{- .labels | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ .name }}-fleet-registry
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
volumeMount for the inter-service-ca ConfigMap (mounts the shared CA bundle
used to validate peer mTLS certs; see DD-PLATFORM-001). Identical across
every consuming service — only the surrounding indentation varies.
Usage: {{ include "kubernaut.tlsCaVolumeMount" . | nindent 12 }}
*/}}
{{- define "kubernaut.tlsCaVolumeMount" -}}
- name: tls-ca
  mountPath: /etc/tls-ca
  readOnly: true
{{- end }}

{{/*
volume for the inter-service-ca ConfigMap (optional so hook-mode/plain-HTTP
installs without it still start; see DD-PLATFORM-001). Identical across
every consuming service — only the surrounding indentation varies.
Usage: {{ include "kubernaut.tlsCaVolume" . | nindent 8 }}
*/}}
{{- define "kubernaut.tlsCaVolume" -}}
- name: tls-ca
  configMap:
    name: inter-service-ca
    optional: true
{{- end }}

{{/*
Common `metadata:` block for a NetworkPolicy manifest: name
({{ kubernaut.fullname }}-<nameSuffix>), namespace, and labels
(app: <appLabel> plus kubernaut.labels). Identical across every
NetworkPolicy template — only nameSuffix/appLabel vary per component.
Usage: {{ include "kubernaut.np.metadata" (dict "root" $ "nameSuffix" "gateway" "appLabel" "gateway") }}
*/}}
{{- define "kubernaut.np.metadata" -}}
metadata:
  name: {{ include "kubernaut.fullname" .root }}-{{ .nameSuffix }}
  namespace: {{ .root.Release.Namespace }}
  labels:
    app: {{ .appLabel }}
    {{- include "kubernaut.labels" .root | nindent 4 }}
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

{{/*
Generic merge(override, defaults) -> toYaml for a securityContext block.
Used by postgresql/valkey (and any component needing custom defaults).
Defaults now include restricted-PSA-oriented settings (runAsNonRoot,
capabilities.drop ALL) while keeping readOnlyRootFilesystem false so
upstream images can write their data directories. Overrides in values.yaml
always win via merge.
Usage: {{ include "kubernaut.mergedSecurityContext" (dict "override" .Values.postgresql.podSecurityContext "defaults" (dict "seccompProfile" (dict "type" "RuntimeDefault"))) | nindent 8 }}
*/}}
{{- define "kubernaut.mergedSecurityContext" -}}
{{- $override := .override | default dict -}}
{{- toYaml (merge $override .defaults) }}
{{- end }}

{{/*
Generous startupProbe for components whose startup can legitimately stall
past a steady-state liveness/readiness budget (DD-PLATFORM-008). Any
component that builds a registry.ClusterRegistry / MCP client connection at
boot (fleet-aware services: apifrontend, effectivenessmonitor,
workflowexecution, gateway, remediationorchestrator, fleetmetadatacache,
signalprocessing as of this writing) can block for 100+ seconds under a
CPU-constrained or noisy-neighbor node -- well past a liveness probe tuned
for steady-state response times -- causing kubelet to kill and restart the
pod before it ever reports ready (a self-inflicted crash loop, confirmed via
live debugging: the same services observed 10+ minute crash loops in a
resource-constrained Kind/podman E2E cluster, each recovering instantly once
retried under lighter load). A startupProbe defers liveness/readiness
enforcement entirely until it passes once, then hands off to the existing
probes unchanged -- steady-state behavior is untouched. This is the DEFAULT
pattern for any new component with the same "slow, one-time startup
dependency" shape; add a startupProbe rather than loosening the steady-state
liveness/readiness thresholds (which would also mask a genuinely hung
process for that much longer). See DD-PLATFORM-008 for the full rationale
and alternatives considered.

failureThreshold default of 60 (initialDelaySeconds=5 + 60*periodSeconds=5 =
305s total grace) rather than an earlier 30 (155s): live re-validation of
this same design decision (#1755 DD-TEST-015 fleet E2E hardening) caught
effectivenessmonitor/remediationorchestrator/workflowexecution still being
killed by the startupProbe itself under a 12-way-parallel Ginkgo fleet E2E
run -- confirmed via direct cgroup v2 `cpu.stat` inspection
(`kubectl exec <pod> -- cat /sys/fs/cgroup/cpu.stat`) showing nr_throttled/
nr_periods ratios as high as 23/24 (96%) immediately after a fresh restart,
i.e. near-constant CFS bandwidth-quota throttling against the 500m cpu
limit during the cache-sync + MCP-client-connect cold-start burst (the
well-documented "CPU limits throttle bursty startup work even when average
utilization looks moderate" behavior of the Linux CFS bandwidth controller,
kubernetes/kubernetes#67577). 305s matches the 5-minute ceiling already
adopted elsewhere in this same investigation for mcpclient.NewResilient's
own MaxElapsedTime backoff budget (pkg/fleet/mcpclient/resilience.go) and
for the equivalent kubectl-rollout-status timeout in
test/infrastructure/fullpipeline_e2e_helm.go -- the startupProbe should not
give up before the client it's waiting on does.
Usage: {{ include "kubernaut.startupProbe" (dict "port" "health") | nindent 10 }}
*/}}
{{- define "kubernaut.startupProbe" -}}
startupProbe:
  httpGet:
    path: {{ .path | default "/healthz" }}
    port: {{ .port | default "health" }}
  initialDelaySeconds: {{ .initialDelaySeconds | default 5 }}
  periodSeconds: {{ .periodSeconds | default 5 }}
  timeoutSeconds: {{ .timeoutSeconds | default 5 }}
  failureThreshold: {{ .failureThreshold | default 60 }}
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
{{- fail "NetworkPolicies are mandatory (DD-PLATFORM-006) but could not auto-discover the kube-apiserver endpoint (`lookup \"v1\" \"Endpoints\" \"default\" \"kubernetes\"` returned no addresses -- possible causes: the Helm installer ServiceAccount lacks permission to read Endpoints in the default namespace, or this is an unusual cluster). Set networkPolicies.apiServerCIDR (single control-plane) or networkPolicies.apiServerCIDRs (HA, one entry per control-plane node) explicitly -- see `kubectl get endpoints kubernetes -o wide`." -}}
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
FleetMetadataCache egress rule: Gateway and RemediationOrchestrator call FMC
directly for fleet scope-checking (kubernaut.fleetmetadatacache.url, port
8080) whenever fleet federation is wired up. Issue #1737 gap found: FMC's own
NetworkPolicy (fleetmetadatacache.yaml) already allows ingress FROM these two
callers, but neither caller's NetworkPolicy ever had the reciprocal egress
rule -- a one-sided wiring gap that silently timed out Gateway's/RO's fleet
readiness probe ("FMC unreachable: ... context deadline exceeded").
Unconditional (not gated on global.fleet.enabled or fleetmetadatacache.enabled):
a podSelector matching zero live pods is a no-op, and the "fleet" E2E suite
never sets either of those chart values true -- it deploys FMC via a raw
kubectl manifest outside the chart (test/infrastructure/fleet_e2e.go's
deployFMC, "app: fleetmetadatacache") and patches fleet config into
Gateway/RO's ConfigMaps post-install rather than via `helm upgrade --set
global.fleet.*` -- so gating on either value would leave the E2E lane
unmatched. Both label conventions are listed to cover the chart's own
template ("app.kubernetes.io/name") and that E2E-only path ("app").
Usage: {{ include "kubernaut.np.fleetmetadatacacheEgress" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.fleetmetadatacacheEgress" -}}
- ports:
    - port: 8080
      protocol: TCP
  to:
    - podSelector:
        matchLabels:
          app: fleetmetadatacache
    - podSelector:
        matchLabels:
          app.kubernetes.io/name: fleetmetadatacache
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

{{/*
Ingress rule(s) for a list of raw CIDR strings (ipBlock-based). Needed for
peers whose traffic isn't associated with any pod/namespace, so
podSelector/namespaceSelector can never match it regardless of how broadly
they're scoped -- e.g. NodePort traffic (SNAT'd to the node's own IP by
kube-proxy under the default externalTrafficPolicy: Cluster) and
hostNetwork-mode ingress controllers/routers (e.g. OpenShift Router,
typically a DaemonSet with hostNetwork: true). Confirmed empirically during
Issue #1737: a wildcard `namespaceSelector: {}` did NOT unblock NodePort
ingress, but `ipBlock: {cidr: 0.0.0.0/0}` did.
Usage: {{ include "kubernaut.np.ingressCIDRs" (dict "cidrs" .Values.networkPolicies.gateway.ingressCIDRs "port" 8080) | nindent 4 }}
*/}}
{{- define "kubernaut.np.ingressCIDRs" -}}
{{- $port := .port -}}
{{- range .cidrs }}
- ports:
    - port: {{ $port }}
      protocol: TCP
  from:
    - ipBlock:
        cidr: {{ . }}
{{- end }}
{{- end }}

{{/*
Ingress rule(s) for a list of raw namespaceSelector label-selector objects
(matchLabels/matchExpressions) -- more flexible than the simple name-based
ingressNamespaces list (which can only match by exact
kubernetes.io/metadata.name label value). Still pod/namespace-identity-based:
does NOT help with NodePort- or hostNetwork-sourced traffic (see
kubernaut.np.ingressCIDRs for that case).
Usage: {{ include "kubernaut.np.ingressNamespaceSelectors" (dict "selectors" .Values.networkPolicies.gateway.ingressNamespaceSelectors "port" 8080) | nindent 4 }}
*/}}
{{- define "kubernaut.np.ingressNamespaceSelectors" -}}
{{- $port := .port -}}
{{- range .selectors }}
- ports:
    - port: {{ $port }}
      protocol: TCP
  from:
    - namespaceSelector:
        {{- toYaml . | nindent 8 }}
{{- end }}
{{- end }}

{{/*
Egress rule to reach the OIDC identity provider (token exchange, JWKS
discovery). Defaults to "anywhere on 443" -- correct for a real external IDP
over HTTPS and identical to the hardcoded rule this replaces. Override
networkPolicies.idp.cidr/port for an in-cluster or non-standard-port IDP
(e.g. a test-only OIDC provider double reachable only on the pod CIDR at a
non-443 port).

networkPolicies.idp.extraPorts (list of ints, default none) additively opens
egress on further ports against the SAME cidr, for deployments where a
single service must reach two different IdPs on two different ports (e.g.
APIFrontend validating end-user JWTs against one IdP on :5556 while also
fetching its own fleet-MCP OAuth2 token from a second IdP on :8443 --
confirmed root cause of issue #1782's E2E-FLEET-016 401s: the two suites
each set networkPolicies.idp.port to their own single IdP's port via `helm
--set`, so whichever one applied LAST silently clobbered the other's egress
rule instead of both being additive).
Usage: {{ include "kubernaut.np.idpEgress" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.idpEgress" -}}
{{- $v := include "kubernaut.mergedValues" (dict "root" . "service" "networkPolicies") | fromYaml -}}
- ports:
    - port: {{ $v.idp.port }}
      protocol: TCP
    {{- range $v.idp.extraPorts }}
    - port: {{ . }}
      protocol: TCP
    {{- end }}
  to:
    - ipBlock:
        cidr: {{ $v.idp.cidr }}
{{- end }}

{{/*
Egress rule to reach the MCP Gateway (global.fleet.mcpGatewayEndpoint) that
every fleet-aware service's mcpclient.NewResilient connects to directly
(owner-chain resolution, remote-cluster tool calls, etc.) -- independent of
FMC's own scope-check API (kubernaut.np.fleetmetadatacacheEgress). #1755
DD-TEST-015 regression RCA (5th finding): FMC's networkpolicy has always
carried its own ad hoc "namespaceSelector: {} + 443/8080" rule that happens
to cover this by accident, but gateway/remediationorchestrator/
effectivenessmonitor/workflowexecution/signalprocessing/apifrontend never
had ANY egress rule for it at all. Confirmed live via
remediationorchestrator's own /proc/net/tcp: a connection to the
mcp-gateway-istio Service ClusterIP:8080 stuck in SYN_SENT (default-deny
silently dropping the SYN) for 19+ minutes across 4 startupProbe-triggered
restarts, blocking wireRemediationOrchestratorDependencies indefinitely --
the exact same failure signature as the Keycloak/API-server/Valkey gaps
above, just for a fourth distinct egress target. Uses ipBlock (like
idpEgress/llmEgress) rather than namespaceSelector/podSelector because the
MCP Gateway can be Kuadrant's mcp-gateway-istio (gateway-system namespace)
or Envoy AI Gateway's equivalent (envoy-ai-gateway-system namespace) --
either a different namespace than the calling pod, or genuinely external in
a real deployment -- so no single podSelector/namespaceSelector could match
both topologies. Both gateway variants listen on 8080
(test/infrastructure/fleet_e2e.go's deployKubeMCPServerAndRegister and
deployEnvoyAIGatewayInfra both hardcode :8080).
Usage: {{ include "kubernaut.np.mcpGatewayEgress" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.mcpGatewayEgress" -}}
{{- $v := include "kubernaut.mergedValues" (dict "root" . "service" "networkPolicies") | fromYaml -}}
- ports:
    - port: {{ $v.mcpGateway.port }}
      protocol: TCP
  to:
    - ipBlock:
        cidr: {{ $v.mcpGateway.cidr }}
{{- end }}

{{/*
Egress rule allowing traffic to the LLM provider APIFrontend's A2A launcher
agent calls directly (apifrontend.config.agent.llm.endpoint). Mirrors
kubernaut.np.idpEgress -- an ipBlock rather than podSelector/namespaceSelector
because the target is normally an external HTTPS API, not a chart-managed pod.
Usage: {{ include "kubernaut.np.llmEgress" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.llmEgress" -}}
{{- $v := include "kubernaut.mergedValues" (dict "root" . "service" "networkPolicies") | fromYaml -}}
- ports:
    - port: {{ $v.llm.port }}
      protocol: TCP
  to:
    - ipBlock:
        cidr: {{ $v.llm.cidr }}
{{- end }}

{{/*
Egress rule allowing traffic to Prometheus, which apifrontend.config.
severityTriage queries directly (GetAlerts/GetRules/InstantQuery) whenever
monitoring.prometheus.enabled=true. Mirrors kubernaut.np.idpEgress/llmEgress
-- an ipBlock rather than podSelector/namespaceSelector because Prometheus is
never chart-managed by this Helm chart itself (no Deployment/Service
template exists for it here); it's always an externally-provisioned or
E2E-test-provisioned instance referenced only by URL.
Issue #1839 RCA: this rule never existed, so AF's severity-triage calls to
Prometheus were silently dropped by the default-deny NetworkPolicy on any
Kind cluster new enough to enforce NetworkPolicy (kindnetd, Kind v0.24+) --
a total, consistent "dial tcp: i/o timeout"/"context deadline exceeded" on
every single call, not intermittent slowness. Tier 3's now-removed ungrounded
LLM fallback had silently absorbed this failure for an unknown amount of
time, since a NetworkPolicy-dropped connection and "no alert data available"
produce the identical Triage() error shape.
Usage: {{ include "kubernaut.np.prometheusEgress" . | nindent 4 }}
*/}}
{{- define "kubernaut.np.prometheusEgress" -}}
{{- $v := include "kubernaut.mergedValues" (dict "root" . "service" "networkPolicies") | fromYaml -}}
- ports:
    - port: {{ $v.prometheus.port }}
      protocol: TCP
  to:
    - ipBlock:
        cidr: {{ $v.prometheus.cidr }}
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
{{- $v := include "kubernaut.mergedValues" (dict "root" . "service" "apifrontend") | fromYaml -}}
{{- printf "https://apifrontend.%s.svc:%v" .Release.Namespace $v.config.server.httpPort -}}
{{- end }}

{{/* ===== LLM Profile Consolidation Helpers (DD-PLATFORM-007 / BR-PLATFORM-008) ===== */}}

{{/*
Resolve a named LLM profile from global.llmProfiles. fail()s loudly when the
name is not defined -- mirrors the Kubernaut Operator's
ResolveLLMProfile-consuming validation (VL-011/VL-014); callers are
responsible for their own fail() when the *reference itself* is empty (see
e.g. kubernautAgent.llmProfileRef being required), since an empty name and
an unknown name warrant distinct error messages (IT-PLATFORM-LLM-001).
Returns the profile's raw field dict exactly as authored under
global.llmProfiles.<name> -- NOT schema defaults, since values.schema.json
validates but never injects defaults into a free-form additionalProperties
map. Callers apply their own `| default` for fields with a non-empty
sensible default, same as the pre-DD-PLATFORM-007 literal-block templates
did for kubernautAgent.llm.*.
Usage:
  {{- $p := include "kubernaut.llm.resolveProfile" (dict "root" $ "name" .Values.kubernautAgent.llmProfileRef) | fromYaml }}
  {{ $p.provider }}
*/}}
{{- define "kubernaut.llm.resolveProfile" -}}
{{- $profiles := .root.Values.global.llmProfiles | default dict -}}
{{- $profile := index $profiles .name -}}
{{- if not $profile -}}
{{- fail (printf "global.llmProfiles[%q] is not defined -- add it under global.llmProfiles, or fix the llmProfileRef/phaseModels entry that references it." .name) -}}
{{- end -}}
{{- $profile | toYaml -}}
{{- end }}

{{/*
Count-based inference for kubernautAgent.llmProfileRef (Issue #1987,
DD-PLATFORM-006 DA4 Addendum): mirrors kubernaut.tls.certManagerIssuerName's
count-then-select-or-fail pattern above and the Kubernaut Operator's
v1alpha2 EffectiveKALLMProfileRef. An explicit non-empty ref always wins
outright. An empty ref (omitted, or explicitly set to "" -- Helm's schema
defaulting makes the two indistinguishable once merged) is inferred from
global.llmProfiles: exactly one entry -> use its name; zero or 2+ entries ->
fail() naming the candidates, since silent auto-selection among ambiguous
options would be an input-validation gap (SI-10).
Usage:
  {{- $kaEffectiveRef := include "kubernaut.llm.effectiveKALLMProfileRef" (dict "root" $ "ref" $kaV.llmProfileRef) }}
*/}}
{{- define "kubernaut.llm.effectiveKALLMProfileRef" -}}
{{- if .ref -}}
{{- .ref -}}
{{- else -}}
{{- $names := keys (.root.Values.global.llmProfiles | default dict) -}}
{{- if eq (len $names) 1 -}}
{{- index $names 0 -}}
{{- else if eq (len $names) 0 -}}
{{- fail "kubernautAgent.llmProfileRef is not set and global.llmProfiles defines no profiles -- add exactly one profile under global.llmProfiles, or set kubernautAgent.llmProfileRef explicitly." -}}
{{- else -}}
{{- fail (printf "kubernautAgent.llmProfileRef is not set and global.llmProfiles defines %d profiles, so auto-selection is ambiguous. Set kubernautAgent.llmProfileRef to one of: %s" (len $names) (join ", " (sortAlpha $names))) -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Credential file name within an LLM profile's mounted Secret volume:
vertex_ai uses a JSON service-account key ("credentials.json"); every other
provider uses a flat API key file ("api_key"). Mirrors the Kubernaut
Operator's configmaps.go credFile branch exactly (kubernaut-operator#233/
#234).
Usage: {{ include "kubernaut.llm.credFile" $profile.provider }}
*/}}
{{- define "kubernaut.llm.credFile" -}}
{{- if eq . "vertex_ai" -}}credentials.json{{- else -}}api_key{{- end -}}
{{- end }}

{{/*
Full in-container path to a resolved profile's mounted credential file:
"<dir>/api_key" or "<dir>/credentials.json" depending on provider (see
kubernaut.llm.credFile). Consolidates the identical printf+credFile pairing
repeated at every dedicated-mount call site (KA phaseModels, KA
alignmentCheck, AF's own agent.llm, AF severityTriage) -- each site only
supplies its own mount directory and resolved profile's provider.
Usage:
  {{ include "kubernaut.llm.mountedKeyFile" (dict "dir" "/etc/apifrontend/llm-credentials" "provider" $afProfile.provider) }}
*/}}
{{- define "kubernaut.llm.mountedKeyFile" -}}
{{- printf "%s/%s" .dir (include "kubernaut.llm.credFile" .provider) -}}
{{- end }}

{{/*
Render one phase-shaped LLM override's field subset (DD-PLATFORM-007):
provider/model/endpoint/vertexProject/vertexLocation/reasoning, plus a
conditional apiKeyFile supplied by the caller. Shared by
kubernautAgent.phaseModels (Kubernaut Agent's LLMRuntimeConfig.PhaseModels)
and kubernautAgent.alignmentCheck.llm (AlignmentCheckConfig.LLM) -- both
consume the identical Go type (internal/kubernautagent/config.
LLMOverrideConfig), which has no oauth2/tlsCaFile fields, so neither is
rendered here. Deliberately excludes azureApiVersion/bedrockRegion even
though LLMOverrideConfig has both fields -- matches the Kubernaut
Operator's own configmaps.go rendering precedent for phaseModels (verified:
llmPhaseOverrideYAML has no azureApiVersion/bedrockRegion field), which
this chart mirrors for both consumers of LLMOverrideConfig for consistency.
apiKeyFile is rendered verbatim when non-empty -- the caller computes the
correct mount path (or passes "" when the resolved profile shares the
base/KA profile's credentialsSecretName and should inherit its mount).
Usage:
  {{ include "kubernaut.llm.overrideBlock" (dict "profile" $p "apiKeyFile" $apiKeyFile) | nindent 8 }}
*/}}
{{- define "kubernaut.llm.overrideBlock" -}}
{{- $p := .profile -}}
provider: {{ $p.provider | quote }}
{{- if $p.model }}
model: {{ $p.model | quote }}
{{- end }}
{{- if $p.endpoint }}
endpoint: {{ $p.endpoint | quote }}
{{- end }}
{{- if $p.vertexProject }}
vertexProject: {{ $p.vertexProject | quote }}
{{- end }}
{{- if $p.vertexLocation }}
vertexLocation: {{ $p.vertexLocation | quote }}
{{- end }}
{{- $reasoning := $p.reasoning | default dict -}}
{{- if or $reasoning.enabled $reasoning.effort $reasoning.capabilityOverride $reasoning.budgetTokens }}
reasoning:
{{- if $reasoning.enabled }}
  enabled: true
{{- end }}
{{- if $reasoning.budgetTokens }}
  budgetTokens: {{ $reasoning.budgetTokens }}
{{- end }}
{{- if $reasoning.effort }}
  effort: {{ $reasoning.effort | quote }}
{{- end }}
{{- if $reasoning.capabilityOverride }}
  capabilityOverride: {{ $reasoning.capabilityOverride | quote }}
{{- end }}
{{- end }}
{{- if .apiKeyFile }}
apiKeyFile: {{ .apiKeyFile | quote }}
{{- end }}
{{- end }}

{{/*
Render an agent.llm / severityTriage.llm block from a resolved LLM profile
(DD-PLATFORM-007), field-for-field against pkg/apifrontend/config's
types.LLMConfig. apiKeyFile is always rendered when a mount path is
supplied, including for vertex_ai (kubernaut#1801) -- cmd/apifrontend reads
it two ways depending on the resolved model family: Gemini-on-Vertex passes
its content as explicit credential bytes (zero env var touched, matching
Kubernaut Agent), while Claude-on-Vertex's SDK (adk-anthropic-go) has no
explicit-bytes option and can only discover credentials via ambient ADC, so
cmd/apifrontend injects GOOGLE_APPLICATION_CREDENTIALS=apiKeyFile
in-process at construction time instead (pkg/apifrontend/launcher.
InjectAmbientGoogleCredentials) rather than declaring it statically here in
the Deployment manifest. The caller computes apiKeyFile's mount path (AF's
own "llm-credentials" mount, or a dedicated "severity-triage-credentials"
mount when severityTriage resolves a distinct credentialsSecretName from
AF's own).
Usage:
  {{ include "kubernaut.llm.afBlock" (dict "profile" $afProfile "apiKeyFile" "/etc/apifrontend/llm-credentials/api_key") | nindent 8 }}
*/}}
{{- define "kubernaut.llm.afBlock" -}}
{{- $p := .profile -}}
provider: {{ $p.provider | quote }}
model: {{ $p.model | quote }}
{{- if $p.endpoint }}
endpoint: {{ $p.endpoint | quote }}
{{- end }}
{{- if .apiKeyFile }}
apiKeyFile: {{ .apiKeyFile | quote }}
{{- end }}
{{- if $p.vertexProject }}
vertexProject: {{ $p.vertexProject | quote }}
{{- end }}
{{- if $p.vertexLocation }}
vertexLocation: {{ $p.vertexLocation | quote }}
{{- end }}
{{- if $p.tlsCaFile }}
tlsCaFile: {{ $p.tlsCaFile | quote }}
{{- end }}
{{- $oauth2 := $p.oauth2 | default dict -}}
{{- if $oauth2.enabled }}
oauth2:
  enabled: true
  tokenURL: {{ $oauth2.tokenURL | quote }}
  credentialsDir: "/etc/apifrontend/oauth2"
{{- if $oauth2.scopes }}
  scopes:
{{- range (splitList " " $oauth2.scopes) }}
    - {{ . | quote }}
{{- end }}
{{- end }}
{{- end }}
{{- $reasoning := $p.reasoning | default dict -}}
{{- if or $reasoning.enabled $reasoning.effort $reasoning.capabilityOverride $reasoning.budgetTokens }}
reasoning:
{{- if $reasoning.enabled }}
  enabled: true
{{- end }}
{{- if $reasoning.budgetTokens }}
  budgetTokens: {{ $reasoning.budgetTokens }}
{{- end }}
{{- if $reasoning.effort }}
  effort: {{ $reasoning.effort | quote }}
{{- end }}
{{- if $reasoning.capabilityOverride }}
  capabilityOverride: {{ $reasoning.capabilityOverride | quote }}
{{- end }}
{{- end }}
{{- end }}

{{/*
DD-PLATFORM-006 Decision Area 14 (PR9): merges a service's block of
charts/kubernaut/templates/_generated_defaults.tpl (kubernaut.defaults,
schema-derived) with the user's own .Values.<service> overrides, user values
always winning -- including an explicit false/0/"" override of a
true/non-zero/non-empty default. Returns the merged block as a dict (via
fromYaml), so callers read $v.<field> instead of .Values.<service>.<field>.

Deliberately uses `mergeOverwrite (dict) $svcDefaults $svcValues`, NOT
`merge $svcValues $svcDefaults`. Sprig's `merge` gives the first argument's
keys precedence, but inherits a well-documented mergo limitation
(https://github.com/helm/helm/issues/13309,
https://github.com/Masterminds/sprig/issues/255): if the winning side's leaf
value is a Go zero value (false/0/""), mergo treats it as "unset" and lets
the losing side's non-zero value leak through anyway -- silently discarding
an explicit user override back to a true-defaulting boolean's zero value.
This is the exact bug class this PR exists to fix (three confirmed live
instances: apifrontend.config.mcp.enabled / .interactive.enabled /
.severityTriage.llmEnabled all silently re-enable themselves when a user
sets them to false via the old `| default true` pattern). `mergeOverwrite`
gives the *last* argument precedence unconditionally, including its zero
values, so ordering defaults first and user values last is what makes
explicit zero-value overrides survive. Starting from an empty `dict` (per
Sprig's own documented gotcha) avoids mutating $svcDefaults/$svcValues in
place. Verified empirically (not just from docs) against
bool/int/string zero-value overrides and nested partial-object overrides
before adoption -- see the PR9 plan's Preflight finding 6.

datastorage.config.retention.enabled is the one field deliberately NOT
routed through this helper (see datastorage.yaml) -- its schema default
(false) diverges from the intentionally-hardened values.yaml default (true,
FED-C2/AU-11 compliance), so it stays on a direct .Values read.

Usage: {{ $v := include "kubernaut.mergedValues" (dict "root" $ "service" "gateway") | fromYaml }}
       then read $v.replicas instead of .Values.gateway.replicas.
*/}}
{{- define "kubernaut.mergedValues" -}}
{{- $defaults := include "kubernaut.defaults" .root | fromYaml -}}
{{- $svcDefaults := deepCopy (default dict (index $defaults .service)) -}}
{{- $svcValues := deepCopy (default dict (index .root.Values .service)) -}}
{{- mergeOverwrite (dict) $svcDefaults $svcValues | toYaml -}}
{{- end -}}
