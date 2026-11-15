{{/*
Fullname helper: release name unless overridden.
*/}}
{{- define "cohort-builder.name" -}}
{{- default .Release.Name .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}
