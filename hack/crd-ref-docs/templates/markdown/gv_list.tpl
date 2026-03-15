{{- define "gvList" -}}
{{- $groupVersions := . -}}

# Custom Resources (CRDs)

Kubernaut API reference for all Custom Resource Definitions.

API Group: `kubernaut.ai/v1alpha1`

{{ range $groupVersions }}
{{ template "gvDetails" . }}
{{ end }}

{{- end -}}
