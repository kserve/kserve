# kserve-localmodel-resources

![Version: v0.18.0](https://img.shields.io/badge/Version-v0.18.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.18.0](https://img.shields.io/badge/AppVersion-v0.18.0-informational?style=flat-square)

KServe LocalModel - Local Model Storage and Caching for Edge and On-Premise Deployments

**Homepage:** <https://kserve.github.io/website/>

## Installing the Chart

To install the chart, run the following:

```console
$ helm install kserve-localmodel-resources oci://ghcr.io/kserve/charts/kserve-localmodel-resources --version v0.18.0
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
| kserve.version | string | `"v0.18.0"` |  |
