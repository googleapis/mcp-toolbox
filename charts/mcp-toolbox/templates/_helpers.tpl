{{/*
Expand the name of the chart.
*/}}
{{- define "mcp-toolbox.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name, truncated to 63 characters.
*/}}
{{- define "mcp-toolbox.fullname" -}}
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

{{- define "mcp-toolbox.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "mcp-toolbox.labels" -}}
helm.sh/chart: {{ include "mcp-toolbox.chart" . }}
{{ include "mcp-toolbox.selectorLabels" . }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "mcp-toolbox.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mcp-toolbox.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "mcp-toolbox.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "mcp-toolbox.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Name of the Secret holding tools.yaml, or empty when using prebuilt tools only.
*/}}
{{- define "mcp-toolbox.configSecretName" -}}
{{- if .Values.config.existingSecret }}
{{- .Values.config.existingSecret }}
{{- else if .Values.config.toolsYaml }}
{{- printf "%s-config" (include "mcp-toolbox.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Fail early on an invalid tool configuration.
*/}}
{{- define "mcp-toolbox.validateConfig" -}}
{{- if and .Values.config.toolsYaml .Values.config.existingSecret }}
{{- fail "mcp-toolbox: config.toolsYaml and config.existingSecret are mutually exclusive" }}
{{- end }}
{{- if not (or .Values.config.toolsYaml .Values.config.existingSecret .Values.config.prebuilt) }}
{{- fail "mcp-toolbox: set config.toolsYaml, config.existingSecret, or config.prebuilt" }}
{{- end }}
{{- end }}
