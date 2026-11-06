{{- define "kupra.operator.selectorLabels" -}}
app.kubernetes.io/name: kupra-operator
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- /*
kupra.labelsWith renders an object's labels as one map: the given labels, which win, then the chart's own, then extra, then commonLabels.
It takes a dict with ctx, the root context, labels, and optionally extra.
*/}}
{{- define "kupra.labelsWith" -}}
{{- $ctx := .ctx }}
{{- $own := dict
  "app.kubernetes.io/part-of" "kupra"
  "app.kubernetes.io/version" $ctx.Chart.AppVersion
  "app.kubernetes.io/managed-by" $ctx.Release.Service
  "helm.sh/chart" (printf "%s-%s" $ctx.Chart.Name $ctx.Chart.Version) }}
{{- toYaml (merge (deepCopy (.labels | default dict)) $own (deepCopy (.extra | default dict)) (deepCopy ($ctx.Values.commonLabels | default dict))) }}
{{- end }}

{{- define "kupra.labels" -}}
{{- include "kupra.labelsWith" (dict "ctx" .) }}
{{- end }}

{{- define "kupra.operator.labels" -}}
{{- include "kupra.labelsWith" (dict "ctx" . "labels" (include "kupra.operator.selectorLabels" . | fromYaml)) }}
{{- end }}
