{{- define "kupra.operator.selectorLabels" -}}
app.kubernetes.io/name: kupra-operator
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- /* commonLabels can add labels, but not replace the chart's own. */}}
{{- define "kupra.labels" -}}
{{- $labels := dict
  "app.kubernetes.io/part-of" "kupra"
  "app.kubernetes.io/version" .Chart.AppVersion
  "app.kubernetes.io/managed-by" .Release.Service
  "helm.sh/chart" (printf "%s-%s" .Chart.Name .Chart.Version) }}
{{- toYaml (merge $labels (.Values.commonLabels | default dict)) }}
{{- end }}

{{- define "kupra.operator.labels" -}}
{{- toYaml (merge (include "kupra.operator.selectorLabels" . | fromYaml) (include "kupra.labels" . | fromYaml)) }}
{{- end }}
