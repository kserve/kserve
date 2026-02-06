# llmisvc

![Version: v0.16.0](https://img.shields.io/badge/Version-v0.16.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.16.0](https://img.shields.io/badge/AppVersion-v0.16.0-informational?style=flat-square)

KServe Generative AI - Large Language Models and Foundation Model Serving

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| certManager.enabled | string | `""` |  |
| inferenceServiceConfig.agent.cpuLimit | string | `"1"` |  |
| inferenceServiceConfig.agent.cpuRequest | string | `"100m"` |  |
| inferenceServiceConfig.agent.image | string | `"kserve/agent"` |  |
| inferenceServiceConfig.agent.memoryLimit | string | `"1Gi"` |  |
| inferenceServiceConfig.agent.memoryRequest | string | `"100Mi"` |  |
| inferenceServiceConfig.agent.tag | string | `""` |  |
| inferenceServiceConfig.autoscaler.scaleDownStabilizationWindowSeconds | string | `"300"` |  |
| inferenceServiceConfig.autoscaler.scaleUpStabilizationWindowSeconds | string | `"0"` |  |
| inferenceServiceConfig.batcher.cpuLimit | string | `"1"` |  |
| inferenceServiceConfig.batcher.cpuRequest | string | `"1"` |  |
| inferenceServiceConfig.batcher.image | string | `"kserve/agent"` |  |
| inferenceServiceConfig.batcher.maxBatchSize | string | `"32"` |  |
| inferenceServiceConfig.batcher.maxLatency | string | `"5000"` |  |
| inferenceServiceConfig.batcher.memoryLimit | string | `"1Gi"` |  |
| inferenceServiceConfig.batcher.memoryRequest | string | `"1Gi"` |  |
| inferenceServiceConfig.batcher.tag | string | `""` |  |
| inferenceServiceConfig.credentials.gcs.gcsCredentialFileName | string | `"gcloud-application-credentials.json"` |  |
| inferenceServiceConfig.credentials.s3.s3AccessKeyIDName | string | `"AWS_ACCESS_KEY_ID"` |  |
| inferenceServiceConfig.credentials.s3.s3CABundle | string | `""` |  |
| inferenceServiceConfig.credentials.s3.s3CABundleConfigMap | string | `""` |  |
| inferenceServiceConfig.credentials.s3.s3Endpoint | string | `""` |  |
| inferenceServiceConfig.credentials.s3.s3Region | string | `""` |  |
| inferenceServiceConfig.credentials.s3.s3SecretAccessKeyName | string | `"AWS_SECRET_ACCESS_KEY"` |  |
| inferenceServiceConfig.credentials.s3.s3UseAccelerate | string | `""` |  |
| inferenceServiceConfig.credentials.s3.s3UseAnonymousCredential | string | `""` |  |
| inferenceServiceConfig.credentials.s3.s3UseHttps | string | `""` |  |
| inferenceServiceConfig.credentials.s3.s3UseVirtualBucket | string | `""` |  |
| inferenceServiceConfig.credentials.s3.s3VerifySSL | string | `""` |  |
| inferenceServiceConfig.credentials.storageSecretNameAnnotation | string | `"serving.kserve.io/storageSecretName"` |  |
| inferenceServiceConfig.credentials.storageSpecSecretName | string | `"storage-config"` |  |
| inferenceServiceConfig.deploy.defaultDeploymentMode | string | `"Serverless"` |  |
| inferenceServiceConfig.enabled | string | `""` |  |
| inferenceServiceConfig.explainers.art.image | string | `"kserve/art-explainer"` |  |
| inferenceServiceConfig.explainers.art.tag | string | `"latest"` |  |
| inferenceServiceConfig.inferenceService.resource.cpuLimit | string | `"1"` |  |
| inferenceServiceConfig.inferenceService.resource.cpuRequest | string | `"1"` |  |
| inferenceServiceConfig.inferenceService.resource.memoryLimit | string | `"2Gi"` |  |
| inferenceServiceConfig.inferenceService.resource.memoryRequest | string | `"2Gi"` |  |
| inferenceServiceConfig.ingress.disableIngressCreation | bool | `false` |  |
| inferenceServiceConfig.ingress.disableIstioVirtualHost | bool | `false` |  |
| inferenceServiceConfig.ingress.domainTemplate | string | `"{{ .Name }}-{{ .Namespace }}.{{ .IngressDomain }}"` |  |
| inferenceServiceConfig.ingress.enableGatewayApi | bool | `false` |  |
| inferenceServiceConfig.ingress.ingressClassName | string | `"istio"` |  |
| inferenceServiceConfig.ingress.ingressDomain | string | `"example.com"` |  |
| inferenceServiceConfig.ingress.ingressGateway | string | `"knative-serving/knative-ingress-gateway"` |  |
| inferenceServiceConfig.ingress.kserveIngressGateway | string | `"kserve/kserve-ingress-gateway"` |  |
| inferenceServiceConfig.ingress.localGateway | string | `"knative-serving/knative-local-gateway"` |  |
| inferenceServiceConfig.ingress.localGatewayService | string | `"knative-local-gateway.istio-system.svc.cluster.local"` |  |
| inferenceServiceConfig.ingress.urlScheme | string | `"http"` |  |
| inferenceServiceConfig.localModel.defaultJobImage | string | `"kserve/storage-initializer"` |  |
| inferenceServiceConfig.localModel.defaultJobImageTag | string | `""` |  |
| inferenceServiceConfig.localModel.disableVolumeManagement | bool | `false` |  |
| inferenceServiceConfig.localModel.enabled | bool | `false` |  |
| inferenceServiceConfig.localModel.fsGroup | int | `1000` |  |
| inferenceServiceConfig.localModel.jobNamespace | string | `"kserve-localmodel-jobs"` |  |
| inferenceServiceConfig.localModel.jobTTLSecondsAfterFinished | int | `3600` |  |
| inferenceServiceConfig.localModel.reconcilationFrequencyInSecs | int | `60` |  |
| inferenceServiceConfig.logger.cpuLimit | string | `"1"` |  |
| inferenceServiceConfig.logger.cpuRequest | string | `"100m"` |  |
| inferenceServiceConfig.logger.defaultUrl | string | `"http://default-broker"` |  |
| inferenceServiceConfig.logger.image | string | `"kserve/agent"` |  |
| inferenceServiceConfig.logger.memoryLimit | string | `"1Gi"` |  |
| inferenceServiceConfig.logger.memoryRequest | string | `"100Mi"` |  |
| inferenceServiceConfig.logger.tag | string | `""` |  |
| inferenceServiceConfig.metricsAggregator.enableMetricAggregation | string | `"false"` |  |
| inferenceServiceConfig.metricsAggregator.enablePrometheusScraping | string | `"false"` |  |
| inferenceServiceConfig.opentelemetryCollector.metricReceiverEndpoint | string | `"keda-otel-scaler.keda.svc:4317"` |  |
| inferenceServiceConfig.opentelemetryCollector.metricScalerEndpoint | string | `"keda-otel-scaler.keda.svc:4318"` |  |
| inferenceServiceConfig.opentelemetryCollector.resource.cpuLimit | string | `"1"` |  |
| inferenceServiceConfig.opentelemetryCollector.resource.cpuRequest | string | `"200m"` |  |
| inferenceServiceConfig.opentelemetryCollector.resource.memoryLimit | string | `"2Gi"` |  |
| inferenceServiceConfig.opentelemetryCollector.resource.memoryRequest | string | `"512Mi"` |  |
| inferenceServiceConfig.opentelemetryCollector.scrapeInterval | string | `"5s"` |  |
| inferenceServiceConfig.router.cpuLimit | string | `"1"` |  |
| inferenceServiceConfig.router.cpuRequest | string | `"100m"` |  |
| inferenceServiceConfig.router.image | string | `"kserve/router"` |  |
| inferenceServiceConfig.router.imagePullPolicy | string | `"IfNotPresent"` |  |
| inferenceServiceConfig.router.memoryLimit | string | `"1Gi"` |  |
| inferenceServiceConfig.router.memoryRequest | string | `"100Mi"` |  |
| inferenceServiceConfig.router.tag | string | `""` |  |
| inferenceServiceConfig.security.autoMountServiceAccountToken | bool | `true` |  |
| inferenceServiceConfig.storageInitializer.caBundleConfigMapName | string | `""` |  |
| inferenceServiceConfig.storageInitializer.caBundleVolumeMountPath | string | `"/etc/ssl/custom-certs"` |  |
| inferenceServiceConfig.storageInitializer.cpuLimit | string | `"1"` |  |
| inferenceServiceConfig.storageInitializer.cpuModelcar | string | `"10m"` |  |
| inferenceServiceConfig.storageInitializer.cpuRequest | string | `"100m"` |  |
| inferenceServiceConfig.storageInitializer.enableModelcar | bool | `true` |  |
| inferenceServiceConfig.storageInitializer.image | string | `"kserve/storage-initializer"` |  |
| inferenceServiceConfig.storageInitializer.memoryLimit | string | `"1Gi"` |  |
| inferenceServiceConfig.storageInitializer.memoryModelcar | string | `"15Mi"` |  |
| inferenceServiceConfig.storageInitializer.memoryRequest | string | `"100Mi"` |  |
| inferenceServiceConfig.storageInitializer.tag | string | `""` |  |
| inferenceServiceConfig.storageInitializer.uidModelcar | int | `1010` |  |
| kserve.createSharedResources | bool | `true` |  |
| kserve.version | string | `"v0.16.0"` |  |
| llmisvc.controller.containers.manager.image | string | `"kserve/llmisvc-controller"` |  |
| llmisvc.controller.containers.manager.imagePullPolicy | string | `"Always"` |  |
| llmisvc.controller.containers.manager.resources.limits.cpu | string | `"100m"` |  |
| llmisvc.controller.containers.manager.resources.limits.memory | string | `"300Mi"` |  |
| llmisvc.controller.containers.manager.resources.requests.cpu | string | `"100m"` |  |
| llmisvc.controller.containers.manager.resources.requests.memory | string | `"300Mi"` |  |
| llmisvc.controller.containers.manager.tag | string | `""` |  |

