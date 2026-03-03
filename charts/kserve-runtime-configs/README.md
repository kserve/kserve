# kserve-runtime-configs

![Version: v0.16.0](https://img.shields.io/badge/Version-v0.16.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.16.0](https://img.shields.io/badge/AppVersion-v0.16.0-informational?style=flat-square)

KServe Runtime Configurations - ClusterServingRuntimes and LLM Inference Configs

**Homepage:** <https://kserve.github.io/website/>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| KServe Team |  | <https://github.com/kserve/kserve> |

## Source Code

* <https://github.com/kserve/kserve>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| kserve.autoscaler.scaleDownStabilizationWindowSeconds | string | `"300"` |  |
| kserve.autoscaler.scaleUpStabilizationWindowSeconds | string | `"0"` |  |
| kserve.inferenceservice.resources.limits.cpu | string | `"1"` |  |
| kserve.inferenceservice.resources.limits.memory | string | `"2Gi"` |  |
| kserve.inferenceservice.resources.requests.cpu | string | `"1"` |  |
| kserve.inferenceservice.resources.requests.memory | string | `"2Gi"` |  |
| kserve.llmisvcConfigs.enabled | bool | `false` |  |
| kserve.opentelemetryCollector.metricReceiverEndpoint | string | `"keda-otel-scaler.keda.svc:4317"` |  |
| kserve.opentelemetryCollector.metricScalerEndpoint | string | `"keda-otel-scaler.keda.svc:4318"` |  |
| kserve.opentelemetryCollector.resource.cpuLimit | string | `"1"` |  |
| kserve.opentelemetryCollector.resource.cpuRequest | string | `"200m"` |  |
| kserve.opentelemetryCollector.resource.memoryLimit | string | `"2Gi"` |  |
| kserve.opentelemetryCollector.resource.memoryRequest | string | `"512Mi"` |  |
| kserve.opentelemetryCollector.scrapeInterval | string | `"5s"` |  |
| kserve.security.autoMountServiceAccountToken | bool | `true` |  |
| kserve.servingruntime.art.defaultVersion | string | `""` |  |
| kserve.servingruntime.art.image | string | `"kserve/art-explainer"` |  |
| kserve.servingruntime.art.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.enabled | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.devShm.enabled | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.devShm.sizeLimit | string | `""` |  |
| kserve.servingruntime.huggingfaceserver.disabled | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.hostIPC.enabled | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.image | string | `"kserve/huggingfaceserver"` |  |
| kserve.servingruntime.huggingfaceserver.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.huggingfaceserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.huggingfaceserver.lmcacheUseExperimental | string | `"True"` |  |
| kserve.servingruntime.huggingfaceserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.huggingfaceserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.huggingfaceserver.tag | string | `""` |  |
| kserve.servingruntime.huggingfaceserver_multinode.disabled | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver_multinode.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.huggingfaceserver_multinode.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver_multinode.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.huggingfaceserver_multinode.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver_multinode.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.huggingfaceserver_multinode.shm.enabled | bool | `true` |  |
| kserve.servingruntime.huggingfaceserver_multinode.shm.sizeLimit | string | `"3Gi"` |  |
| kserve.servingruntime.lgbserver.disabled | bool | `false` |  |
| kserve.servingruntime.lgbserver.image | string | `"kserve/lgbserver"` |  |
| kserve.servingruntime.lgbserver.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.lgbserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.lgbserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.lgbserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.lgbserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.lgbserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.lgbserver.tag | string | `""` |  |
| kserve.servingruntime.mlserver.disabled | bool | `false` |  |
| kserve.servingruntime.mlserver.image | string | `"docker.io/seldonio/mlserver"` |  |
| kserve.servingruntime.mlserver.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.mlserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.mlserver.modelClassPlaceholder | string | `"{{.Labels.modelClass}}"` |  |
| kserve.servingruntime.mlserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.mlserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.mlserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.mlserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.mlserver.tag | string | `"1.5.0"` |  |
| kserve.servingruntime.modelNamePlaceholder | string | `"{{.Name}}"` |  |
| kserve.servingruntime.paddleserver.disabled | bool | `false` |  |
| kserve.servingruntime.paddleserver.image | string | `"kserve/paddleserver"` |  |
| kserve.servingruntime.paddleserver.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.paddleserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.paddleserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.paddleserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.paddleserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.paddleserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.paddleserver.tag | string | `""` |  |
| kserve.servingruntime.pmmlserver.disabled | bool | `false` |  |
| kserve.servingruntime.pmmlserver.image | string | `"kserve/pmmlserver"` |  |
| kserve.servingruntime.pmmlserver.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.pmmlserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.pmmlserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.pmmlserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.pmmlserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.pmmlserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.pmmlserver.tag | string | `""` |  |
| kserve.servingruntime.predictiveserver.disabled | bool | `false` |  |
| kserve.servingruntime.predictiveserver.image | string | `"kserve/predictiveserver"` |  |
| kserve.servingruntime.predictiveserver.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.predictiveserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.predictiveserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.predictiveserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.predictiveserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.predictiveserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.predictiveserver.tag | string | `""` |  |
| kserve.servingruntime.sklearnserver.disabled | bool | `false` |  |
| kserve.servingruntime.sklearnserver.image | string | `"kserve/sklearnserver"` |  |
| kserve.servingruntime.sklearnserver.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.sklearnserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.sklearnserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.sklearnserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.sklearnserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.sklearnserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.sklearnserver.tag | string | `""` |  |
| kserve.servingruntime.tensorflow.disabled | bool | `false` |  |
| kserve.servingruntime.tensorflow.image | string | `"tensorflow/serving"` |  |
| kserve.servingruntime.tensorflow.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.tensorflow.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.tensorflow.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.tensorflow.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.tensorflow.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.tensorflow.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.tensorflow.securityContext.runAsUser | int | `1000` |  |
| kserve.servingruntime.tensorflow.tag | string | `"2.6.2"` |  |
| kserve.servingruntime.torchserve.disabled | bool | `false` |  |
| kserve.servingruntime.torchserve.image | string | `"pytorch/torchserve-kfs"` |  |
| kserve.servingruntime.torchserve.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.torchserve.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.torchserve.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.torchserve.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.torchserve.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.torchserve.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.torchserve.securityContext.runAsUser | int | `1000` |  |
| kserve.servingruntime.torchserve.serviceEnvelopePlaceholder | string | `"{{.Labels.serviceEnvelope}}"` |  |
| kserve.servingruntime.torchserve.tag | string | `"0.9.0"` |  |
| kserve.servingruntime.tritonserver.disabled | bool | `false` |  |
| kserve.servingruntime.tritonserver.image | string | `"nvcr.io/nvidia/tritonserver"` |  |
| kserve.servingruntime.tritonserver.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.tritonserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.tritonserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.tritonserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.tritonserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.tritonserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.tritonserver.securityContext.runAsUser | int | `1000` |  |
| kserve.servingruntime.tritonserver.tag | string | `"23.05-py3"` |  |
| kserve.servingruntime.xgbserver.disabled | bool | `false` |  |
| kserve.servingruntime.xgbserver.image | string | `"kserve/xgbserver"` |  |
| kserve.servingruntime.xgbserver.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.servingruntime.xgbserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.xgbserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.xgbserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.xgbserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.xgbserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.xgbserver.tag | string | `""` |  |
| kserve.version | string | `"v0.16.0"` |  |

