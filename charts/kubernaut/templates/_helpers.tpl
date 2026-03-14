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
Usage: {{ include "kubernaut.image" (dict "service" "gateway" "global" .Values.global "appVersion" .Chart.AppVersion) }}
*/}}
{{- define "kubernaut.image" -}}
{{- if .global.image.digest -}}
{{- printf "%s/%s@%s" .global.image.registry .service .global.image.digest -}}
{{- else -}}
{{- printf "%s/%s:%s" .global.image.registry .service (.global.image.tag | default .appVersion) -}}
{{- end -}}
{{- end }}

{{/*
Return the Secret name for PostgreSQL credentials.
When using external PostgreSQL, falls through to the external auth settings.
*/}}
{{- define "kubernaut.postgresql.secretName" -}}
{{- if .Values.postgresql.enabled -}}
  {{- if .Values.postgresql.auth.existingSecret -}}
    {{- .Values.postgresql.auth.existingSecret -}}
  {{- else -}}
    postgresql-secret
  {{- end -}}
{{- else -}}
  {{- if .Values.externalPostgresql.auth.existingSecret -}}
    {{- .Values.externalPostgresql.auth.existingSecret -}}
  {{- else -}}
    postgresql-secret
  {{- end -}}
{{- end -}}
{{- end }}

{{/*
Return the Secret name for DataStorage DB credentials.
DataStorage uses a db-secrets.yaml key (different format from PostgreSQL's
POSTGRES_USER/PASSWORD/DB keys), so it supports its own existingSecret field.
Precedence: datastorage.dbExistingSecret > chart-managed "datastorage-db-secret".
*/}}
{{- define "kubernaut.datastorage.dbSecretName" -}}
{{- if .Values.datastorage.dbExistingSecret -}}
{{- .Values.datastorage.dbExistingSecret -}}
{{- else -}}
datastorage-db-secret
{{- end -}}
{{- end }}

{{/*
Return the Secret name for Valkey credentials.
When using external Valkey, falls through to the external settings.
*/}}
{{- define "kubernaut.valkey.secretName" -}}
{{- if .Values.valkey.enabled -}}
  {{- if .Values.valkey.existingSecret -}}
    {{- .Values.valkey.existingSecret -}}
  {{- else -}}
    valkey-secret
  {{- end -}}
{{- else -}}
  {{- if .Values.externalValkey.existingSecret -}}
    {{- .Values.externalValkey.existingSecret -}}
  {{- else -}}
    valkey-secret
  {{- end -}}
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
{{- required "externalPostgresql.host is required when postgresql.enabled=false" .Values.externalPostgresql.host -}}
{{- end -}}
{{- end }}

{{/*
Return the PostgreSQL port.
*/}}
{{- define "kubernaut.postgresql.port" -}}
{{- if .Values.postgresql.enabled -}}
5432
{{- else -}}
{{- .Values.externalPostgresql.port | default 5432 -}}
{{- end -}}
{{- end }}

{{/*
Return the PostgreSQL username (for config files / readiness probes).
*/}}
{{- define "kubernaut.postgresql.username" -}}
{{- if .Values.postgresql.enabled -}}
{{- .Values.postgresql.auth.username -}}
{{- else -}}
{{- .Values.externalPostgresql.auth.username | default "slm_user" -}}
{{- end -}}
{{- end }}

{{/*
Return the PostgreSQL database name.
*/}}
{{- define "kubernaut.postgresql.database" -}}
{{- if .Values.postgresql.enabled -}}
{{- .Values.postgresql.auth.database -}}
{{- else -}}
{{- .Values.externalPostgresql.auth.database | default "action_history" -}}
{{- end -}}
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
Return the data directory mount path for the PostgreSQL variant.
upstream: /var/lib/postgresql/data   ocp: /var/lib/pgsql/data
*/}}
{{- define "kubernaut.postgresql.dataDir" -}}
{{- if eq (include "kubernaut.postgresql.variant" .) "ocp" -}}/var/lib/pgsql/data{{- else -}}/var/lib/postgresql/data{{- end -}}
{{- end }}

{{/*
NOTE: kubernaut.postgresql.clientImage was removed in #351 (C1).
Migration hooks now use hooks.migrations.image directly (db-migrate image
with goose + psql pre-installed). No runtime binary downloads needed.
*/}}

{{/*
Return the Valkey data directory mount path.
upstream: /data   ocp: /var/lib/valkey/data
*/}}
{{- define "kubernaut.valkey.dataDir" -}}
{{- if eq (include "kubernaut.postgresql.variant" .) "ocp" -}}/var/lib/valkey/data{{- else -}}/data{{- end -}}
{{- end }}

{{/*
Return the Valkey address (host:port).
*/}}
{{- define "kubernaut.valkey.addr" -}}
{{- if .Values.valkey.enabled -}}
valkey.{{ .Release.Namespace }}.svc.cluster.local:6379
{{- else -}}
{{- $host := required "externalValkey.host is required when valkey.enabled=false" .Values.externalValkey.host -}}
{{- printf "%s:%d" $host (int (.Values.externalValkey.port | default 6379)) -}}
{{- end -}}
{{- end }}

{{/*
Return the in-cluster DataStorage service URL.
Derives the FQDN from .Release.Namespace so the chart works in any namespace.
*/}}
{{- define "kubernaut.datastorage.url" -}}
http://data-storage-service.{{ .Release.Namespace }}.svc.cluster.local:8080
{{- end }}

{{/*
Return the in-cluster Gateway service URL.
*/}}
{{- define "kubernaut.gateway.url" -}}
http://gateway-service.{{ .Release.Namespace }}.svc.cluster.local:8080
{{- end }}

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
