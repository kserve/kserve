{{/*
Expand the name of the chart.
*/}}
{{- define "llm-isvc-resources.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "llm-isvc-resources.fullname" -}}
{{- if contains .Chart.Name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "llm-isvc-resources.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "llm-isvc-resources.labels" -}}
helm.sh/chart: {{ include "llm-isvc-resources.chart" . }}
{{ include "llm-isvc-resources.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "llm-isvc-resources.selectorLabels" -}}
app.kubernetes.io/name: {{ include "llm-isvc-resources.deploymentName" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the deployment name
*/}}
{{- define "llm-isvc-resources.deploymentName" -}}
kserve-llmisvc-controller-manager
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "llm-isvc-resources.serviceAccountName" -}}
{{- default (include "llm-isvc-resources.deploymentName" .) .Values.kserve.llmisvc.controller.serviceAccount.name }}
{{- end }}

{{/*
Return the proper image name
*/}}
{{- define "llm-isvc-resources.image" -}}
{{- $repositoryName := .Values.kserve.llmisvc.controller.image -}}
{{- $tag := .Values.kserve.llmisvc.controller.tag | toString -}}
{{- printf "%s:%s" $repositoryName $tag -}}
{{- end }}

{{/*
Return the proper image pull policy
*/}}
{{- define "llm-isvc-resources.imagePullPolicy" -}}
{{- .Values.kserve.llmisvc.controller.imagePullPolicy | default "IfNotPresent" }}
{{- end }}

{{/*
Return the proper image pull secrets
*/}}
{{- define "llm-isvc-resources.imagePullSecrets" -}}
{{- if .Values.kserve.llmisvc.controller.imagePullSecrets }}
imagePullSecrets:
{{- range .Values.kserve.llmisvc.controller.imagePullSecrets }}
  - name: {{ . }}
{{- end }}
{{- end }}
{{- end }}
