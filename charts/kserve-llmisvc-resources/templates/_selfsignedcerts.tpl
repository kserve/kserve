{{/*
Normalised kserve.llmisvc.selfSignedCerts configuration. Tolerates the
block (or any ancestor key) being absent — e.g. `helm upgrade
--reuse-values` from a release that predates it, where the new chart's
values.yaml defaults are not merged — by inlining the defaults here.

Returns a JSON dict: {"enabled": bool, "caSecretName": str, "validityDays": int}.
*/}}
{{- define "llm-isvc-resources.selfSignedCertsConfig" -}}
{{- $cfg := dict -}}
{{- with .Values.kserve }}{{- with .llmisvc }}{{- with .selfSignedCerts }}{{- $cfg = . -}}{{- end }}{{- end }}{{- end -}}
{{- dict "enabled" ($cfg.enabled | default false) "caSecretName" ($cfg.caSecretName | default "llmisvc-selfsigned-ca") "validityDays" (int ($cfg.validityDays | default 7300)) | toJson -}}
{{- end -}}

{{/*
Webhook serving-certificate material for the llmisvc webhook server,
used when kserve.llmisvc.selfSignedCerts.enabled is true.

CA selection: reuse the CA keypair from the <caSecretName> Secret when
it exists on the cluster (the kserve-llmisvc-crd chart creates it, and
embeds the same CA as the CRDs' conversion-webhook caBundle);
otherwise generate an ephemeral CA (first render via `helm template`,
or a standalone install without the CRD chart's flag).

Ownership guard: if llmisvc-webhook-server-cert exists but is not owned
by this release (typically issued by cert-manager on an install that
predates the flag), fail with migration steps. Helm would otherwise
abort later with a generic "invalid ownership metadata" error, and
cert-manager's Certificate would re-create the Secret if only the
Secret were deleted. lookup returns nothing under `helm template`, so
the guard never fires there.

Serving cert: reuse the existing llmisvc-webhook-server-cert contents
only when its ca.crt matches the current CA; otherwise issue a fresh
certificate signed by the CA. The result is memoised on .Values for the
duration of the render so the Secret and every webhook caBundle agree.

Returns a JSON dict: {"CACert": <pem>, "Cert": <pem>, "Key": <pem>}.
*/}}
{{- define "llm-isvc-resources.webhookCertData" -}}
{{- if not (hasKey .Values "__webhookCertData") -}}
{{- $cfg := include "llm-isvc-resources.selfSignedCertsConfig" . | fromJson -}}
{{- $svc := "llmisvc-webhook-server-service" -}}
{{- $data := dict -}}
{{- $caCrtB64 := "" -}}
{{- $caSecret := lookup "v1" "Secret" .Release.Namespace $cfg.caSecretName -}}
{{- if and $caSecret $caSecret.data (index $caSecret.data "tls.crt") (index $caSecret.data "tls.key") -}}
{{- $caCrtB64 = index $caSecret.data "tls.crt" -}}
{{- end -}}
{{- $existing := lookup "v1" "Secret" .Release.Namespace "llmisvc-webhook-server-cert" -}}
{{- if $existing -}}
{{- $labels := default dict $existing.metadata.labels -}}
{{- $annotations := default dict $existing.metadata.annotations -}}
{{- if not (and (eq (index $labels "app.kubernetes.io/managed-by" | default "") "Helm") (eq (index $annotations "meta.helm.sh/release-name" | default "") .Release.Name) (eq (index $annotations "meta.helm.sh/release-namespace" | default "") .Release.Namespace)) -}}
{{- fail (printf "kserve.llmisvc.selfSignedCerts.enabled=true, but Secret %s/llmisvc-webhook-server-cert already exists and is not owned by release %q (it is usually issued by cert-manager). To switch an existing install to self-signed certificates, first run: kubectl delete certificate llmisvc-serving-cert -n %s --ignore-not-found && kubectl delete secret llmisvc-webhook-server-cert -n %s; then re-run the upgrade. See the chart README." .Release.Namespace .Release.Name .Release.Namespace .Release.Namespace) -}}
{{- end -}}
{{- end -}}
{{- if and $existing $existing.data (ne $caCrtB64 "") (eq (default "" (index $existing.data "ca.crt")) $caCrtB64) (index $existing.data "tls.crt") (index $existing.data "tls.key") -}}
{{- $data = dict "CACert" ($caCrtB64 | b64dec) "Cert" (index $existing.data "tls.crt" | b64dec) "Key" (index $existing.data "tls.key" | b64dec) -}}
{{- else -}}
{{- $ca := dict -}}
{{- if ne $caCrtB64 "" -}}
{{- $ca = buildCustomCert $caCrtB64 (index $caSecret.data "tls.key") -}}
{{- else -}}
{{- $ca = genCA "llmisvc-selfsigned-ca" (int $cfg.validityDays) -}}
{{- end -}}
{{- $cn := printf "%s.%s.svc" $svc .Release.Namespace -}}
{{- $sans := list $svc (printf "%s.%s" $svc .Release.Namespace) $cn (printf "%s.cluster.local" $cn) -}}
{{- $cert := genSignedCert $cn nil $sans (int $cfg.validityDays) $ca -}}
{{- $data = dict "CACert" $ca.Cert "Cert" $cert.Cert "Key" $cert.Key -}}
{{- end -}}
{{- $_ := set .Values "__webhookCertData" $data -}}
{{- end -}}
{{- get .Values "__webhookCertData" | toJson -}}
{{- end -}}
