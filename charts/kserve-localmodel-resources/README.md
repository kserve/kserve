# kserve-localmodel-resources

![Version: v0.18.0-rc1](https://img.shields.io/badge/Version-v0.18.0--rc1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.18.0-rc1](https://img.shields.io/badge/AppVersion-v0.18.0--rc1-informational?style=flat-square)

KServe LocalModel - Local Model Storage and Caching for Edge and On-Premise Deployments

**Homepage:** <https://kserve.github.io/website/>

## Installing the Chart

To install the chart, run the following:

```console
$ helm install kserve-localmodel-resources oci://ghcr.io/kserve/charts/kserve-localmodel-resources --version v0.18.0-rc1
```

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| KServe Team |  | <https://github.com/kserve/kserve> |

## Source Code

* <https://github.com/kserve/kserve>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| kserve.agent.image | string | `"kserve/agent"` |  |
| kserve.agent.tag | string | `""` |  |
| kserve.autoscaler.scaleDownStabilizationWindowSeconds | string | `"300"` |  |
| kserve.autoscaler.scaleUpStabilizationWindowSeconds | string | `"0"` |  |
| kserve.certManager.enabled | string | `""` |  |
| kserve.createSharedResources | bool | `true` |  |
| kserve.inferenceServiceConfig.enabled | string | `""` |  |
| kserve.inferenceservice.resources.limits.cpu | string | `"1"` |  |
| kserve.inferenceservice.resources.limits.memory | string | `"2Gi"` |  |
| kserve.inferenceservice.resources.requests.cpu | string | `"1"` |  |
| kserve.inferenceservice.resources.requests.memory | string | `"2Gi"` |  |
| kserve.localmodel.agent.affinity | object | `{}` |  |
| kserve.localmodel.agent.hostPath | string | `"/mnt/models"` |  |
| kserve.localmodel.agent.image | string | `"kserve/kserve-localmodelnode-agent"` |  |
| kserve.localmodel.agent.nodeSelector | object | `{}` |  |
| kserve.localmodel.agent.reconcilationFrequencyInSecs | int | `60` |  |
| kserve.localmodel.agent.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.localmodel.agent.securityContext.runAsUser | int | `1000` |  |
| kserve.localmodel.agent.tag | string | `""` |  |
| kserve.localmodel.agent.tolerations | list | `[]` |  |
| kserve.localmodel.controller.image | string | `"kserve/kserve-localmodel-controller"` |  |
| kserve.localmodel.controller.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.localmodel.controller.resources.limits.cpu | string | `"100m"` |  |
| kserve.localmodel.controller.resources.limits.memory | string | `"300Mi"` |  |
| kserve.localmodel.controller.resources.requests.cpu | string | `"100m"` |  |
| kserve.localmodel.controller.resources.requests.memory | string | `"200Mi"` |  |
| kserve.localmodel.controller.tag | string | `""` |  |
| kserve.localmodel.defaultJobImage | string | `"kserve/storage-initializer"` |  |
| kserve.localmodel.defaultJobTag | string | `""` |  |
| kserve.localmodel.disableVolumeManagement | bool | `false` |  |
| kserve.localmodel.enabled | bool | `false` |  |
| kserve.localmodel.jobNamespace | string | `"kserve-localmodel-jobs"` |  |
| kserve.localmodel.jobTTLSecondsAfterFinished | int | `3600` |  |
| kserve.localmodel.securityContext.fsGroup | int | `1000` |  |
| kserve.localmodelnode.controller.affinity | object | `{}` |  |
| kserve.localmodelnode.controller.image | string | `"kserve/kserve-localmodelnode-agent"` |  |
| kserve.localmodelnode.controller.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.localmodelnode.controller.nodeSelector.kserve/localmodel | string | `"worker"` |  |
| kserve.localmodelnode.controller.resources.limits.cpu | string | `"100m"` |  |
| kserve.localmodelnode.controller.resources.limits.memory | string | `"300Mi"` |  |
| kserve.localmodelnode.controller.resources.requests.cpu | string | `"100m"` |  |
| kserve.localmodelnode.controller.resources.requests.memory | string | `"200Mi"` |  |
| kserve.localmodelnode.controller.tag | string | `""` |  |
| kserve.localmodelnode.controller.tolerations | list | `[]` |  |
| kserve.metricsaggregator.enableMetricAggregation | string | `"false"` |  |
| kserve.metricsaggregator.enablePrometheusScraping | string | `"false"` |  |
| kserve.opentelemetryCollector.metricReceiverEndpoint | string | `"keda-otel-scaler.keda.svc:4317"` |  |
| kserve.opentelemetryCollector.metricScalerEndpoint | string | `"keda-otel-scaler.keda.svc:4318"` |  |
| kserve.opentelemetryCollector.resource.cpuLimit | string | `"1"` |  |
| kserve.opentelemetryCollector.resource.cpuRequest | string | `"200m"` |  |
| kserve.opentelemetryCollector.resource.memoryLimit | string | `"2Gi"` |  |
| kserve.opentelemetryCollector.resource.memoryRequest | string | `"512Mi"` |  |
| kserve.opentelemetryCollector.scrapeInterval | string | `"5s"` |  |
| kserve.router.image | string | `"kserve/router"` |  |
| kserve.router.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.router.imagePullSecrets | list | `[]` |  |
| kserve.router.tag | string | `""` |  |
| kserve.security.autoMountServiceAccountToken | bool | `true` |  |
| kserve.service.serviceClusterIPNone | bool | `false` |  |
| kserve.servingruntime.art.defaultVersion | string | `""` |  |
| kserve.servingruntime.art.image | string | `"kserve/art-explainer"` |  |
| kserve.storage.caBundleConfigMapName | string | `""` |  |
| kserve.storage.caBundleVolumeMountPath | string | `"/etc/ssl/custom-certs"` |  |
| kserve.storage.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.storage.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.storage.containerSecurityContext.privileged | bool | `false` |  |
| kserve.storage.containerSecurityContext.runAsNonRoot | bool | `true` |  |
| kserve.storage.cpuModelcar | string | `"10m"` |  |
| kserve.storage.enableModelcar | bool | `true` |  |
| kserve.storage.image | string | `"kserve/storage-initializer"` |  |
| kserve.storage.memoryModelcar | string | `"15Mi"` |  |
| kserve.storage.resources.limits.cpu | string | `"1"` |  |
| kserve.storage.resources.limits.memory | string | `"1Gi"` |  |
| kserve.storage.resources.requests.cpu | string | `"100m"` |  |
| kserve.storage.resources.requests.memory | string | `"100Mi"` |  |
| kserve.storage.s3.CABundle | string | `""` |  |
| kserve.storage.s3.CABundleConfigMap | string | `""` |  |
| kserve.storage.s3.accessKeyIdName | string | `"AWS_ACCESS_KEY_ID"` |  |
| kserve.storage.s3.endpoint | string | `""` |  |
| kserve.storage.s3.region | string | `""` |  |
| kserve.storage.s3.secretAccessKeyName | string | `"AWS_SECRET_ACCESS_KEY"` |  |
| kserve.storage.s3.useAnonymousCredential | string | `""` |  |
| kserve.storage.s3.useHttps | string | `""` |  |
| kserve.storage.s3.useVirtualBucket | string | `""` |  |
| kserve.storage.s3.verifySSL | string | `""` |  |
| kserve.storage.storageSecretNameAnnotation | string | `"serving.kserve.io/secretName"` |  |
| kserve.storage.storageSpecSecretName | string | `"storage-config"` |  |
| kserve.storage.tag | string | `""` |  |
| kserve.storage.uidModelcar | int | `1010` |  |
| kserve.storagecontainer.enabled | string | `""` |  |
| kserve.version | string | `"v0.18.0-rc1"` |  |
