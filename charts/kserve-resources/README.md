# kserve

Helm chart for deploying kserve resources

![Version: v0.13.0](https://img.shields.io/badge/Version-v0.13.0-informational?style=flat-square)

## Installing the Chart

To install the chart, run the following:

```console
$ helm install kserve oci://ghcr.io/kserve/charts/kserve --version v0.13.0
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| kserve.agent.image | string | `"kserve/agent"` |  |
| kserve.agent.tag | string | `"v0.13.0"` |  |
| kserve.controller.affinity | object | `{}` |  |
| kserve.controller.deploymentMode | string | `"Serverless"` |  |
| kserve.controller.gateway.additionalIngressDomains | list | `[]` |  |
| kserve.controller.gateway.disableIngressCreation | bool | `false` |  |
| kserve.controller.gateway.disableIstioVirtualHost | bool | `false` |  |
| kserve.controller.gateway.domain | string | `"example.com"` |  |
| kserve.controller.gateway.domainTemplate | string | `"{{ .Name }}-{{ .Namespace }}.{{ .IngressDomain }}"` |  |
| kserve.controller.gateway.ingressGateway.className | string | `"istio"` |  |
| kserve.controller.gateway.ingressGateway.gateway | string | `"knative-serving/knative-ingress-gateway"` |  |
| kserve.controller.gateway.ingressGateway.gatewayService | string | `"istio-ingressgateway.istio-system.svc.cluster.local"` |  |
| kserve.controller.gateway.localGateway.gateway | string | `"knative-serving/knative-local-gateway"` |  |
| kserve.controller.gateway.localGateway.gatewayService | string | `"knative-local-gateway.istio-system.svc.cluster.local"` |  |
| kserve.controller.gateway.urlScheme | string | `"http"` |  |
| kserve.controller.image | string | `"kserve/kserve-controller"` |  |
| kserve.controller.nodeSelector | object | `{}` |  |
| kserve.controller.rbacProxyImage | string | `"gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1"` |  |
| kserve.controller.resources.limits.cpu | string | `"100m"` |  |
| kserve.controller.resources.limits.memory | string | `"300Mi"` |  |
| kserve.controller.resources.requests.cpu | string | `"100m"` |  |
| kserve.controller.resources.requests.memory | string | `"300Mi"` |  |
| kserve.controller.tag | string | `"v0.13.0"` |  |
| kserve.controller.tolerations | list | `[]` |  |
| kserve.controller.topologySpreadConstraints | list | `[]` |  |
| kserve.metricsaggregator.enableMetricAggregation | string | `"false"` |  |
| kserve.metricsaggregator.enablePrometheusScraping | string | `"false"` |  |
| kserve.modelmesh.config.modelmeshImage | string | `"kserve/modelmesh"` |  |
| kserve.modelmesh.config.modelmeshImageTag | string | `"v0.12.0-rc0"` |  |
| kserve.modelmesh.config.modelmeshRuntimeAdapterImage | string | `"kserve/modelmesh-runtime-adapter"` |  |
| kserve.modelmesh.config.modelmeshRuntimeAdapterImageTag | string | `"v0.12.0-rc0"` |  |
| kserve.modelmesh.config.podsPerRuntime | int | `2` |  |
| kserve.modelmesh.config.restProxyImage | string | `"kserve/rest-proxy"` |  |
| kserve.modelmesh.config.restProxyImageTag | string | `"v0.12.0-rc0"` |  |
| kserve.modelmesh.controller.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.labelSelector.matchExpressions[0].key | string | `"control-plane"` |  |
| kserve.modelmesh.controller.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.labelSelector.matchExpressions[0].operator | string | `"In"` |  |
| kserve.modelmesh.controller.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.labelSelector.matchExpressions[0].values[0] | string | `"modelmesh-controller"` |  |
| kserve.modelmesh.controller.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.topologyKey | string | `"topology.kubernetes.io/zone"` |  |
| kserve.modelmesh.controller.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].weight | int | `100` |  |
| kserve.modelmesh.controller.image | string | `"kserve/modelmesh-controller"` |  |
| kserve.modelmesh.controller.nodeSelector | object | `{}` |  |
| kserve.modelmesh.controller.tag | string | `"v0.12.0-rc0"` |  |
| kserve.modelmesh.controller.tolerations | list | `[]` |  |
| kserve.modelmesh.controller.topologySpreadConstraints | list | `[]` |  |
| kserve.modelmesh.enabled | bool | `true` |  |
| kserve.modelmeshVersion | string | `"v0.12.0-rc0"` |  |
| kserve.router.image | string | `"kserve/router"` |  |
| kserve.router.tag | string | `"v0.13.0"` |  |
| kserve.servingruntime.art.defaultVersion | string | `"v0.13.0"` |  |
| kserve.servingruntime.art.image | string | `"kserve/art-explainer"` |  |
| kserve.servingruntime.huggingfaceserver.image | string | `"kserve/huggingfaceserver"` |  |
| kserve.servingruntime.huggingfaceserver.tag | string | `"v0.13.0"` |  |
| kserve.servingruntime.lgbserver.image | string | `"kserve/lgbserver"` |  |
| kserve.servingruntime.lgbserver.tag | string | `"v0.13.0"` |  |
| kserve.servingruntime.mlserver.image | string | `"docker.io/seldonio/mlserver"` |  |
| kserve.servingruntime.mlserver.modelClassPlaceholder | string | `"{{.Labels.modelClass}}"` |  |
| kserve.servingruntime.mlserver.tag | string | `"1.5.0"` |  |
| kserve.servingruntime.modelNamePlaceholder | string | `"{{.Name}}"` |  |
| kserve.servingruntime.paddleserver.image | string | `"kserve/paddleserver"` |  |
| kserve.servingruntime.paddleserver.tag | string | `"v0.13.0"` |  |
| kserve.servingruntime.pmmlserver.image | string | `"kserve/pmmlserver"` |  |
| kserve.servingruntime.pmmlserver.tag | string | `"v0.13.0"` |  |
| kserve.servingruntime.sklearnserver.image | string | `"kserve/sklearnserver"` |  |
| kserve.servingruntime.sklearnserver.tag | string | `"v0.13.0"` |  |
| kserve.servingruntime.tensorflow.image | string | `"tensorflow/serving"` |  |
| kserve.servingruntime.tensorflow.tag | string | `"2.6.2"` |  |
| kserve.servingruntime.torchserve.image | string | `"pytorch/torchserve-kfs"` |  |
| kserve.servingruntime.torchserve.serviceEnvelopePlaceholder | string | `"{{.Labels.serviceEnvelope}}"` |  |
| kserve.servingruntime.torchserve.tag | string | `"0.9.0"` |  |
| kserve.servingruntime.tritonserver.image | string | `"nvcr.io/nvidia/tritonserver"` |  |
| kserve.servingruntime.tritonserver.tag | string | `"23.05-py3"` |  |
| kserve.servingruntime.xgbserver.image | string | `"kserve/xgbserver"` |  |
| kserve.servingruntime.xgbserver.tag | string | `"v0.13.0"` |  |
| kserve.storage.caBundleConfigMapName | string | `""` |  |
| kserve.storage.caBundleVolumeMountPath | string | `"/etc/ssl/custom-certs"` |  |
| kserve.storage.cpuModelcar | string | `"10m"` |  |
| kserve.storage.enableModelcar | bool | `false` |  |
| kserve.storage.image | string | `"kserve/storage-initializer"` |  |
| kserve.storage.memoryModelcar | string | `"15Mi"` |  |
| kserve.storage.s3.CABundle | string | `""` |  |
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
| kserve.storage.tag | string | `"v0.13.0"` |  |
| kserve.version | string | `"v0.13.0"` |  |
