{{- define "type" -}}
{{- $type := . -}}
{{- if markdownShouldRenderType $type -}}

{{- if $type.GVK }}
## {{ $type.Name }}
{{- else }}
### {{ $type.Name }}
{{- end }}

{{ if $type.IsAlias }}_Underlying type:_ _{{ markdownRenderTypeLink $type.UnderlyingType  }}_{{ end }}

{{ $type.Doc }}

{{ if $type.References -}}
_Appears in:_
{{- range $type.SortedReferences }}
- {{ markdownRenderTypeLink . }}
{{- end }}
{{- end }}

{{ if $type.Members -}}
| Field | Type | Description |
| --- | --- | --- |
{{ if $type.GVK -}}
| `apiVersion` | _string_ | `{{ $type.GVK.Group }}/{{ $type.GVK.Version }}` |
| `kind` | _string_ | `{{ $type.GVK.Kind }}` |
{{ end -}}
{{ range $type.Members -}}
| `{{ .Name }}` | _{{ markdownRenderType .Type }}_ | {{ template "type_members" . }} |
{{ end -}}
{{ end -}}

{{ if $type.Validation -}}
_Validation:_
{{- range $type.Validation }}
- {{ . }}
{{- end }}
{{- end }}

{{ if $type.EnumValues -}}
| Value | Description |
| --- | --- |
{{ range $type.EnumValues -}}
| `{{ .Name }}` | {{ template "type_members_enum" . }} |
{{ end -}}
{{ end -}}

{{- end -}}
{{- end -}}
