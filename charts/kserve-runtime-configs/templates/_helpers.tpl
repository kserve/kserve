{{/*
Expand the name of the chart.
*/}}
{{- define "kserve-runtime-configs.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "kserve-runtime-configs.fullname" -}}
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
{{- define "kserve-runtime-configs.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kserve-runtime-configs.labels" -}}
helm.sh/chart: {{ include "kserve-runtime-configs.chart" . }}
{{ include "kserve-runtime-configs.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kserve-runtime-configs.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kserve-runtime-configs.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Render an image name with its tag or digest cleanly.
Usage: {{ include "kserve-runtime-configs.image" (dict "image" .image "tag" .tag) }}
*/}}
{{- define "kserve-runtime-configs.image" -}}
{{- $tag := "" -}}
{{- if ne .tag nil -}}
{{- $tag = toString .tag -}}
{{- end -}}
{{- if hasPrefix "@" $tag -}}
{{- printf "%s%s" .image $tag -}}
{{- else if hasPrefix "sha256:" $tag -}}
{{- printf "%s@%s" .image $tag -}}
{{- else if ne $tag "" -}}
{{- printf "%s:%s" .image $tag -}}
{{- else -}}
{{- printf "%s" .image -}}
{{- end -}}
{{- end }}

