{{- define "kupra.operator.selectorLabels" -}}
app.kubernetes.io/name: kupra-operator
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "kupra.labels" -}}
app.kubernetes.io/part-of: kupra
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{- define "kupra.operator.labels" -}}
{{ include "kupra.operator.selectorLabels" . }}
{{ include "kupra.labels" . }}
{{- end }}
