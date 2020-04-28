{{/* vim: set filetype=mustache: */}}
{{- define "k8s-crdguard.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "k8s-crdguard.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Create chart name and version as used by the chart label. */}}
{{- define "k8s-crdguard.chartref" -}}
{{- replace "+" "_" .Chart.Version | printf "%s-%s" .Chart.Name -}}
{{- end }}

{{/* Resource labels */}}
{{/* https://github.com/helm/charts/blob/master/REVIEW_GUIDELINES.md#names-and-labels */}}
{{- define "k8s-crdguard.labels" }}
"app.kubernetes.io/name": {{ template "k8s-crdguard.name" . }}
"app.kubernetes.io/instance": {{ .Release.Name }}
"app.kubernetes.io/managed-by": {{ .Release.Service }}
"helm.sh/chart": {{ include "k8s-crdguard.chartref" . }}
{{- if .Values.commonLabels }}
{{ toYaml .Values.commonLabels }}
{{- end }}
{{- end }}

{{/* Selector labels */}}
{{- define "k8s-crdguard.selectorlabels" }}
"app.kubernetes.io/name": {{ include "k8s-crdguard.name" . }}
"app.kubernetes.io/instance": {{ .Release.Name }}
{{- end }}

{{/* Component label */}}
{{- define "k8s-crdguard.component" }}
"app.kubernetes.io/component": {{ . }}
{{- end}}
