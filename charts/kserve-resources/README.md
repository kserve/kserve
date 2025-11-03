# kserve

Helm chart for deploying kserve resources

![Version: v0.16.0](https://img.shields.io/badge/Version-v0.16.0-informational?style=flat-square)

## Installing the Chart

To install the chart, run the following:

```console
$ helm install kserve oci://ghcr.io/kserve/charts/kserve --version v0.16.0
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| inferenceserviceConfig.Example | string | `""` |  |
| kserve.localmodel.enabled | bool | `false` |  |
| kserveControllerManager.kubeRbacProxy.args[0] | string | `"--secure-listen-address=0.0.0.0:8443"` |  |
| kserveControllerManager.kubeRbacProxy.args[1] | string | `"--upstream=http://127.0.0.1:8080/"` |  |
| kserveControllerManager.kubeRbacProxy.args[2] | string | `"--logtostderr=true"` |  |
| kserveControllerManager.kubeRbacProxy.args[3] | string | `"--v=10"` |  |
| kserveControllerManager.kubeRbacProxy.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserveControllerManager.kubeRbacProxy.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserveControllerManager.kubeRbacProxy.containerSecurityContext.privileged | bool | `false` |  |
| kserveControllerManager.kubeRbacProxy.containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| kserveControllerManager.kubeRbacProxy.containerSecurityContext.runAsNonRoot | bool | `true` |  |
| kserveControllerManager.kubeRbacProxy.image.repository | string | `"quay.io/brancz/kube-rbac-proxy"` |  |
| kserveControllerManager.kubeRbacProxy.image.tag | string | `"v0.18.0"` |  |
| kserveControllerManager.manager.args[0] | string | `"--metrics-addr=127.0.0.1:8080"` |  |
| kserveControllerManager.manager.args[1] | string | `"--leader-elect"` |  |
| kserveControllerManager.manager.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserveControllerManager.manager.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserveControllerManager.manager.containerSecurityContext.privileged | bool | `false` |  |
| kserveControllerManager.manager.containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| kserveControllerManager.manager.containerSecurityContext.runAsNonRoot | bool | `true` |  |
| kserveControllerManager.manager.env.secretName | string | `"kserve-webhook-server-cert"` |  |
| kserveControllerManager.manager.image.repository | string | `"kserve/kserve-controller"` |  |
| kserveControllerManager.manager.image.tag | string | `"latest"` |  |
| kserveControllerManager.manager.imagePullPolicy | string | `"Always"` |  |
| kserveControllerManager.manager.resources.limits.cpu | string | `"100m"` |  |
| kserveControllerManager.manager.resources.limits.memory | string | `"300Mi"` |  |
| kserveControllerManager.manager.resources.requests.cpu | string | `"100m"` |  |
| kserveControllerManager.manager.resources.requests.memory | string | `"200Mi"` |  |
| kserveControllerManager.podSecurityContext.runAsNonRoot | bool | `true` |  |
| kserveControllerManager.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| kserveControllerManager.serviceAccount.annotations | object | `{}` |  |
| kserveControllerManagerMetricsService.ports[0].name | string | `"https"` |  |
| kserveControllerManagerMetricsService.ports[0].port | int | `8443` |  |
| kserveControllerManagerMetricsService.ports[0].targetPort | string | `"https"` |  |
| kserveControllerManagerMetricsService.type | string | `"ClusterIP"` |  |
| kserveControllerManagerService.ports[0].port | int | `8443` |  |
| kserveControllerManagerService.ports[0].protocol | string | `"TCP"` |  |
| kserveControllerManagerService.ports[0].targetPort | string | `"https"` |  |
| kserveControllerManagerService.type | string | `"ClusterIP"` |  |
| kserveLlmisvcControllerManager.manager.args[0] | string | `"--metrics-addr=127.0.0.1:8443"` |  |
| kserveLlmisvcControllerManager.manager.args[1] | string | `"--leader-elect"` |  |
| kserveLlmisvcControllerManager.manager.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserveLlmisvcControllerManager.manager.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserveLlmisvcControllerManager.manager.containerSecurityContext.privileged | bool | `false` |  |
| kserveLlmisvcControllerManager.manager.containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| kserveLlmisvcControllerManager.manager.containerSecurityContext.runAsNonRoot | bool | `true` |  |
| kserveLlmisvcControllerManager.manager.containerSecurityContext.runAsUser | int | `1000` |  |
| kserveLlmisvcControllerManager.manager.containerSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| kserveLlmisvcControllerManager.manager.image.repository | string | `"kserve/llmisvc-controller"` |  |
| kserveLlmisvcControllerManager.manager.image.tag | string | `"latest"` |  |
| kserveLlmisvcControllerManager.manager.imagePullPolicy | string | `"Always"` |  |
| kserveLlmisvcControllerManager.manager.resources.limits.cpu | string | `"100m"` |  |
| kserveLlmisvcControllerManager.manager.resources.limits.memory | string | `"300Mi"` |  |
| kserveLlmisvcControllerManager.manager.resources.requests.cpu | string | `"100m"` |  |
| kserveLlmisvcControllerManager.manager.resources.requests.memory | string | `"300Mi"` |  |
| kserveLlmisvcControllerManager.podSecurityContext.runAsNonRoot | bool | `true` |  |
| kserveLlmisvcControllerManager.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| kserveLlmisvcControllerManager.replicas | int | `1` |  |
| kserveLlmisvcControllerManager.strategy.rollingUpdate.maxSurge | int | `1` |  |
| kserveLlmisvcControllerManager.strategy.rollingUpdate.maxUnavailable | int | `0` |  |
| kserveLlmisvcControllerManager.strategy.type | string | `"RollingUpdate"` |  |
| kserveLocalmodelControllerManager.manager.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserveLocalmodelControllerManager.manager.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserveLocalmodelControllerManager.manager.containerSecurityContext.privileged | bool | `false` |  |
| kserveLocalmodelControllerManager.manager.containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| kserveLocalmodelControllerManager.manager.containerSecurityContext.runAsNonRoot | bool | `true` |  |
| kserveLocalmodelControllerManager.manager.image.repository | string | `"kserve/kserve-localmodel-controller"` |  |
| kserveLocalmodelControllerManager.manager.image.tag | string | `"latest"` |  |
| kserveLocalmodelControllerManager.manager.imagePullPolicy | string | `"Always"` |  |
| kserveLocalmodelControllerManager.manager.resources.limits.cpu | string | `"100m"` |  |
| kserveLocalmodelControllerManager.manager.resources.limits.memory | string | `"300Mi"` |  |
| kserveLocalmodelControllerManager.manager.resources.requests.cpu | string | `"100m"` |  |
| kserveLocalmodelControllerManager.manager.resources.requests.memory | string | `"200Mi"` |  |
| kserveLocalmodelControllerManager.podSecurityContext.runAsNonRoot | bool | `true` |  |
| kserveLocalmodelControllerManager.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| kserveLocalmodelControllerManager.serviceAccount.annotations | object | `{}` |  |
| kserveLocalmodelnodeAgent.manager.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserveLocalmodelnodeAgent.manager.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserveLocalmodelnodeAgent.manager.containerSecurityContext.privileged | bool | `false` |  |
| kserveLocalmodelnodeAgent.manager.containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| kserveLocalmodelnodeAgent.manager.containerSecurityContext.runAsNonRoot | bool | `true` |  |
| kserveLocalmodelnodeAgent.manager.image.repository | string | `"kserve/kserve-localmodelnode-agent"` |  |
| kserveLocalmodelnodeAgent.manager.image.tag | string | `"latest"` |  |
| kserveLocalmodelnodeAgent.manager.imagePullPolicy | string | `"Always"` |  |
| kserveLocalmodelnodeAgent.manager.resources.limits.cpu | string | `"100m"` |  |
| kserveLocalmodelnodeAgent.manager.resources.limits.memory | string | `"300Mi"` |  |
| kserveLocalmodelnodeAgent.manager.resources.requests.cpu | string | `"100m"` |  |
| kserveLocalmodelnodeAgent.manager.resources.requests.memory | string | `"200Mi"` |  |
| kserveLocalmodelnodeAgent.nodeSelector.kserve/localmodel | string | `"worker"` |  |
| kserveLocalmodelnodeAgent.podSecurityContext.runAsNonRoot | bool | `true` |  |
| kserveLocalmodelnodeAgent.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| kserveLocalmodelnodeAgent.serviceAccount.annotations | object | `{}` |  |
| kserveWebhookServerService.ports[0].port | int | `443` |  |
| kserveWebhookServerService.ports[0].targetPort | string | `"webhook-server"` |  |
| kserveWebhookServerService.type | string | `"ClusterIP"` |  |
| kubernetesClusterDomain | string | `"cluster.local"` |  |
| llmisvcControllerManager.serviceAccount.annotations | object | `{}` |  |
| llmisvcControllerManagerService.ports[0].name | string | `"https"` |  |
| llmisvcControllerManagerService.ports[0].port | int | `8443` |  |
| llmisvcControllerManagerService.ports[0].protocol | string | `"TCP"` |  |
| llmisvcControllerManagerService.ports[0].targetPort | string | `"metrics"` |  |
| llmisvcControllerManagerService.type | string | `"ClusterIP"` |  |
| llmisvcWebhookServerService.ports[0].port | int | `443` |  |
| llmisvcWebhookServerService.ports[0].targetPort | string | `"webhook-server"` |  |
| llmisvcWebhookServerService.type | string | `"ClusterIP"` |  |
