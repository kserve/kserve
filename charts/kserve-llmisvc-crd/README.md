# kserve-llmisvc-crd

Helm chart for deploying LLMInferenceService crds

![Version: v0.20.0-rc0](https://img.shields.io/badge/Version-v0.20.0--rc0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.20.0-rc0](https://img.shields.io/badge/AppVersion-v0.20.0--rc0-informational?style=flat-square)

## Installing the Chart

To install the chart, run the following:

```console
$ helm install kserve-llmisvc-crd oci://ghcr.io/kserve/charts/kserve-llmisvc-crd --version v0.20.0-rc0
```

## Self-signed webhook certificates (without cert-manager)

By default the CRDs carry the `cert-manager.io/inject-ca-from` annotation, so cert-manager must be
installed for the conversion-webhook `caBundle` to be injected. To install without cert-manager, set
`kserve.llmisvc.selfSignedCerts.enabled=true`. The chart then provisions a self-signed CA, stores the
keypair in the Secret named by `kserve.llmisvc.selfSignedCerts.caSecretName` (annotated with
`helm.sh/resource-policy: keep` and reused on upgrade, so certificates are never rotated), and embeds
the CA certificate directly as the conversion-webhook `caBundle` in place of the annotation.

Enable the matching `kserve.llmisvc.selfSignedCerts.enabled` flag on the `kserve-llmisvc-resources`
chart so the webhook serving certificate is issued from the same CA.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| kserve.llmisvc.selfSignedCerts.caSecretName | string | `"llmisvc-selfsigned-ca"` |  |
| kserve.llmisvc.selfSignedCerts.enabled | bool | `false` |  |
| kserve.llmisvc.selfSignedCerts.validityDays | int | `7300` |  |
