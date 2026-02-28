{{/*
Expand the name of the chart.
*/}}
{{- define "kubernaut.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

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
{{- printf "%s/%s:%s" .global.image.registry .service (.global.image.tag | default .appVersion) }}
{{- end }}

{{/*
Return the Secret name for PostgreSQL credentials.
Uses existingSecret if set, otherwise the chart-managed "postgresql-secret".
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
Uses existingSecret if set, otherwise the chart-managed "datastorage-db-secret".
*/}}
{{- define "kubernaut.datastorage.dbSecretName" -}}
{{- if .Values.postgresql.auth.existingSecret -}}
{{- .Values.postgresql.auth.existingSecret -}}
{{- else -}}
datastorage-db-secret
{{- end -}}
{{- end }}

{{/*
Return the Secret name for Redis credentials.
Uses existingSecret if set, otherwise the chart-managed "redis-secret".
*/}}
{{- define "kubernaut.redis.secretName" -}}
{{- if .Values.redis.existingSecret -}}
{{- .Values.redis.existingSecret -}}
{{- else -}}
redis-secret
{{- end -}}
{{- end }}
