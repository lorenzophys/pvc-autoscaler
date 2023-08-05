{{/* Full name of the chart */}}
{{- define "pvcautoscaler.fullname" -}}
{{- .Chart.Name }}-{{ .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/* Common labels for all resources */}}
{{- define "pvcautoscaler.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: {{ include "pvcautoscaler.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/* Selector labels for all resources */}}
{{- define "pvcautoscaler.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pvcautoscaler.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
