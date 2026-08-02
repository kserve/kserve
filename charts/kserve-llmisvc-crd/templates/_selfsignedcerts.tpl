{{/*
Normalised kserve.llmisvc.selfSignedCerts configuration. Tolerates the
block (or any ancestor key) being absent — e.g. `helm upgrade
--reuse-values` from a release that predates it, where the new chart's
values.yaml defaults are not merged — by inlining the defaults here.

Returns a JSON dict: {"enabled": bool, "caSecretName": str, "validityDays": int}.
*/}}
{{- define "kserve-llmisvc-crd.selfSignedCertsConfig" -}}
{{- $cfg := dict -}}
{{- with .Values.kserve }}{{- with .llmisvc }}{{- with .selfSignedCerts }}{{- $cfg = . -}}{{- end }}{{- end }}{{- end -}}
{{- dict "enabled" ($cfg.enabled | default false) "caSecretName" ($cfg.caSecretName | default "llmisvc-selfsigned-ca") "validityDays" (int ($cfg.validityDays | default 7300)) | toJson -}}
{{- end -}}

{{/*
Self-signed CA for the llmisvc webhook stack, used when
kserve.llmisvc.selfSignedCerts.enabled is true.

Reuses the CA stored in the <caSecretName> Secret when it exists on
the cluster, so helm upgrades never rotate the CA; generates a fresh
one otherwise (first install, or `helm template` where lookup returns
nothing). The result is memoised on .Values for the duration of the
render so the CA Secret and every CRD conversion caBundle observe the
same CA.

Returns a JSON dict: {"Cert": <pem>, "Key": <pem>}.
*/}}
{{- define "kserve-llmisvc-crd.selfSignedCA" -}}
{{- if not (hasKey .Values "__selfSignedCA") -}}
{{- $cfg := include "kserve-llmisvc-crd.selfSignedCertsConfig" . | fromJson -}}
{{- $ca := dict -}}
{{- $existing := lookup "v1" "Secret" .Release.Namespace $cfg.caSecretName -}}
{{- if and $existing $existing.data (index $existing.data "tls.crt") (index $existing.data "tls.key") -}}
{{- $ca = dict "Cert" (index $existing.data "tls.crt" | b64dec) "Key" (index $existing.data "tls.key" | b64dec) -}}
{{- else -}}
{{- $gen := genCA "llmisvc-selfsigned-ca" (int $cfg.validityDays) -}}
{{- $ca = dict "Cert" $gen.Cert "Key" $gen.Key -}}
{{- end -}}
{{- $_ := set .Values "__selfSignedCA" $ca -}}
{{- end -}}
{{- get .Values "__selfSignedCA" | toJson -}}
{{- end -}}
