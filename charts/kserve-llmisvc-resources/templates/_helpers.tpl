{{/*
Expand the name of the chart.
*/}}
{{- define "kserve-llmisvc-resources.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kserve-llmisvc-resources.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "kserve-llmisvc-resources.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kserve-llmisvc-resources.labels" -}}
helm.sh/chart: {{ include "kserve-llmisvc-resources.chart" . }}
{{ include "kserve-llmisvc-resources.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kserve-llmisvc-resources.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kserve-llmisvc-resources.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kserve-llmisvc-resources.serviceAccountName" -}}
{{- $default := (include "kserve-llmisvc-resources.fullname" .) }}
{{- with .Values.serviceAccount }}
{{- if .create }}
{{- default $default .name }}
{{- else }}
{{- default "default" .name }}
{{- end }}
{{- end }}
{{- end }}
