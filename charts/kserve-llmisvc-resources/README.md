# kserve-llmisvc-resources

Helm chart for deploying KServe LLMInferenceService resources

![Version: v0.16.0](https://img.shields.io/badge/Version-v0.16.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.16.0](https://img.shields.io/badge/AppVersion-v0.16.0-informational?style=flat-square)

## Installing the Chart

To install the chart, run the following:

```console
$ helm install kserve-llmisvc oci://ghcr.io/kserve/charts/kserve-llmisvc-resources --version v0.16.0
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| commonAnnotations | object | `{}` |  |
| commonLabels | object | `{}` |  |
| kserve.agent.image | string | `"kserve/agent"` |  |
| kserve.agent.tag | string | `""` |  |
| kserve.autoscaler.scaleDownStabilizationWindowSeconds | string | `"300"` |  |
| kserve.autoscaler.scaleUpStabilizationWindowSeconds | string | `"0"` |  |
| kserve.certManager.enabled | string | `""` |  |
| kserve.controller.deploymentMode | string | `"Knative"` |  |
| kserve.controller.gateway.additionalIngressDomains | list | `[]` |  |
| kserve.controller.gateway.disableIngressCreation | bool | `false` |  |
| kserve.controller.gateway.disableIstioVirtualHost | bool | `false` |  |
| kserve.controller.gateway.domain | string | `"example.com"` |  |
| kserve.controller.gateway.domainTemplate | string | `"{{ .Name }}-{{ .Namespace }}.{{ .IngressDomain }}"` |  |
| kserve.controller.gateway.ingressGateway.className | string | `"istio"` |  |
| kserve.controller.gateway.ingressGateway.createGateway | bool | `false` |  |
| kserve.controller.gateway.ingressGateway.enableGatewayApi | bool | `false` |  |
| kserve.controller.gateway.ingressGateway.gateway | string | `"knative-serving/knative-ingress-gateway"` |  |
| kserve.controller.gateway.ingressGateway.kserveGateway | string | `"kserve/kserve-ingress-gateway"` |  |
| kserve.controller.gateway.localGateway.gateway | string | `"knative-serving/knative-local-gateway"` |  |
| kserve.controller.gateway.localGateway.gatewayService | string | `"knative-local-gateway.istio-system.svc.cluster.local"` |  |
| kserve.controller.gateway.localGateway.knativeGatewayService | string | `""` |  |
| kserve.controller.gateway.pathTemplate | string | `""` |  |
| kserve.controller.gateway.urlScheme | string | `"http"` |  |
| kserve.createSharedResources | bool | `true` |  |
| kserve.inferenceServiceConfig.enabled | string | `""` |  |
| kserve.inferenceservice.resources.limits.cpu | string | `"1"` |  |
| kserve.inferenceservice.resources.limits.memory | string | `"2Gi"` |  |
| kserve.inferenceservice.resources.requests.cpu | string | `"1"` |  |
| kserve.inferenceservice.resources.requests.memory | string | `"2Gi"` |  |
| kserve.llmisvc.controller.affinity | object | `{}` |  |
| kserve.llmisvc.controller.annotations | object | `{}` |  |
| kserve.llmisvc.controller.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.llmisvc.controller.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.llmisvc.controller.containerSecurityContext.privileged | bool | `false` |  |
| kserve.llmisvc.controller.containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| kserve.llmisvc.controller.containerSecurityContext.runAsNonRoot | bool | `true` |  |
| kserve.llmisvc.controller.containerSecurityContext.runAsUser | int | `1000` |  |
| kserve.llmisvc.controller.containerSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| kserve.llmisvc.controller.env | list | `[]` |  |
| kserve.llmisvc.controller.extraArgs | list | `[]` |  |
| kserve.llmisvc.controller.extraVolumeMounts | list | `[]` |  |
| kserve.llmisvc.controller.extraVolumes | list | `[]` |  |
| kserve.llmisvc.controller.image | string | `"kserve/llmisvc-controller"` |  |
| kserve.llmisvc.controller.imagePullPolicy | string | `"IfNotPresent"` |  |
| kserve.llmisvc.controller.imagePullSecrets | list | `[]` |  |
| kserve.llmisvc.controller.labels | object | `{}` |  |
| kserve.llmisvc.controller.livenessProbe.enabled | bool | `true` |  |
| kserve.llmisvc.controller.livenessProbe.failureThreshold | int | `5` |  |
| kserve.llmisvc.controller.livenessProbe.httpGet.path | string | `"/healthz"` |  |
| kserve.llmisvc.controller.livenessProbe.httpGet.port | int | `8081` |  |
| kserve.llmisvc.controller.livenessProbe.initialDelaySeconds | int | `30` |  |
| kserve.llmisvc.controller.livenessProbe.periodSeconds | int | `10` |  |
| kserve.llmisvc.controller.livenessProbe.timeoutSeconds | int | `5` |  |
| kserve.llmisvc.controller.metricsBindAddress | string | `"127.0.0.1"` |  |
| kserve.llmisvc.controller.metricsBindPort | string | `"8443"` |  |
| kserve.llmisvc.controller.nodeSelector | object | `{}` |  |
| kserve.llmisvc.controller.podAnnotations | object | `{}` |  |
| kserve.llmisvc.controller.podLabels | object | `{}` |  |
| kserve.llmisvc.controller.readinessProbe.enabled | bool | `true` |  |
| kserve.llmisvc.controller.readinessProbe.failureThreshold | int | `5` |  |
| kserve.llmisvc.controller.readinessProbe.httpGet.path | string | `"/readyz"` |  |
| kserve.llmisvc.controller.readinessProbe.httpGet.port | int | `8081` |  |
| kserve.llmisvc.controller.readinessProbe.initialDelaySeconds | int | `30` |  |
| kserve.llmisvc.controller.readinessProbe.periodSeconds | int | `5` |  |
| kserve.llmisvc.controller.readinessProbe.timeoutSeconds | int | `5` |  |
| kserve.llmisvc.controller.replicas | int | `1` |  |
| kserve.llmisvc.controller.resources.limits.cpu | string | `"100m"` |  |
| kserve.llmisvc.controller.resources.limits.memory | string | `"300Mi"` |  |
| kserve.llmisvc.controller.resources.requests.cpu | string | `"100m"` |  |
| kserve.llmisvc.controller.resources.requests.memory | string | `"300Mi"` |  |
| kserve.llmisvc.controller.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.llmisvc.controller.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| kserve.llmisvc.controller.service.port | int | `8443` |  |
| kserve.llmisvc.controller.service.targetPort | string | `"metrics"` |  |
| kserve.llmisvc.controller.service.type | string | `"ClusterIP"` |  |
| kserve.llmisvc.controller.serviceAccount.name | string | `""` |  |
| kserve.llmisvc.controller.serviceAnnotations | object | `{}` |  |
| kserve.llmisvc.controller.strategy.rollingUpdate.maxSurge | int | `1` |  |
| kserve.llmisvc.controller.strategy.rollingUpdate.maxUnavailable | int | `0` |  |
| kserve.llmisvc.controller.strategy.type | string | `"RollingUpdate"` |  |
| kserve.llmisvc.controller.tag | string | `""` |  |
| kserve.llmisvc.controller.terminationGracePeriodSeconds | int | `10` |  |
| kserve.llmisvc.controller.tolerations | list | `[]` |  |
| kserve.llmisvc.controller.topologySpreadConstraints | list | `[]` |  |
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
| kserve.localmodel.controller.tag | string | `""` |  |
| kserve.localmodel.defaultJobImage | string | `"kserve/storage-initializer"` |  |
| kserve.localmodel.defaultJobTag | string | `""` |  |
| kserve.localmodel.disableVolumeManagement | bool | `false` |  |
| kserve.localmodel.enabled | bool | `false` |  |
| kserve.localmodel.jobNamespace | string | `"kserve-localmodel-jobs"` |  |
| kserve.localmodel.jobTTLSecondsAfterFinished | int | `3600` |  |
| kserve.localmodel.securityContext.fsGroup | int | `1000` |  |
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
| kserve.version | string | `"v0.16.0"` |  |
