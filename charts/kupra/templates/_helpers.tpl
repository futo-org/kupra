{{- define "kupra.operator.selectorLabels" -}}
app.kubernetes.io/name: kupra-operator
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- /*
kupra.labelsWith renders an object's labels as one map: the given labels, which win, then the chart's own, then extra, then commonLabels.
It takes a dict with ctx, the root context, labels, and optionally extra, and noCommon, which leaves commonLabels out.
*/}}
{{- define "kupra.labelsWith" -}}
{{- $ctx := .ctx }}
{{- $own := dict
  "app.kubernetes.io/part-of" "kupra"
  "app.kubernetes.io/version" $ctx.Chart.AppVersion
  "app.kubernetes.io/managed-by" $ctx.Release.Service
  "helm.sh/chart" (printf "%s-%s" $ctx.Chart.Name $ctx.Chart.Version) }}
{{- $common := ternary dict ($ctx.Values.commonLabels | default dict) (.noCommon | default false) }}
{{- toYaml (merge (deepCopy (.labels | default dict)) $own (deepCopy (.extra | default dict)) (deepCopy $common)) }}
{{- end }}

{{- /* ClusterRoles don't get commonLabels: other ClusterRoles can aggregate them by any label, which would add their rules to those roles. */}}
{{- define "kupra.clusterRoleLabels" -}}
{{- include "kupra.labelsWith" (dict "ctx" . "noCommon" true) }}
{{- end }}

{{- define "kupra.labels" -}}
{{- include "kupra.labelsWith" (dict "ctx" .) }}
{{- end }}

{{- define "kupra.operator.labels" -}}
{{- include "kupra.labelsWith" (dict "ctx" . "labels" (include "kupra.operator.selectorLabels" . | fromYaml)) }}
{{- end }}
