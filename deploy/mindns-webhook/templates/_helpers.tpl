{{/* vim: set filetype=mustache: */}}
{{/*
The API group name for webhook registration.
This must match the GroupName constant in main.go.
*/}}
{{- define "mindns-webhook.groupName" -}}
acme.greatlion.tech
{{- end -}}

{{/*
Expand the name of the chart.
*/}}
{{- define "mindns-webhook.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "mindns-webhook.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "mindns-webhook.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "mindns-webhook.selfSignedIssuer" -}}
{{ printf "%s-selfsign" (include "mindns-webhook.fullname" .) }}
{{- end -}}

{{- define "mindns-webhook.rootCAIssuer" -}}
{{ printf "%s-ca" (include "mindns-webhook.fullname" .) }}
{{- end -}}

{{- define "mindns-webhook.rootCACertificate" -}}
{{ printf "%s-ca" (include "mindns-webhook.fullname" .) }}
{{- end -}}

{{- define "mindns-webhook.servingCertificate" -}}
{{ printf "%s-webhook-tls" (include "mindns-webhook.fullname" .) }}
{{- end -}}
