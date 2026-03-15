{{- define "type_members" -}}
{{- $field := . -}}
{{- if eq $field.Name "metadata" -}}
Refer to the Kubernetes API documentation for fields of `metadata`.
{{- else -}}
{{ markdownRenderFieldDoc $field.Doc }}
{{- end -}}
{{- end -}}

{{- define "type_members_enum" -}}
{{- $field := . -}}
{{ markdownRenderFieldDoc $field.Doc }}
{{- end -}}
