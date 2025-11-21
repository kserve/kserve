# kserve-llmisvc-resources

A Helm chart for Kubernetes

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.1.0](https://img.shields.io/badge/AppVersion-0.1.0-informational?style=flat-square)

## Installing the Chart

To install the chart, run the following:

```console
$ helm install kserve-llmisvc oci://ghcr.io/kserve/charts/kserve-llmisvc-resources --version 0.1.0
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
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
| kserveLlmisvcControllerManager.nodeSelector | object | `{}` |  |
| kserveLlmisvcControllerManager.podSecurityContext.runAsNonRoot | bool | `true` |  |
| kserveLlmisvcControllerManager.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| kserveLlmisvcControllerManager.replicas | int | `1` |  |
| kserveLlmisvcControllerManager.strategy.rollingUpdate.maxSurge | int | `1` |  |
| kserveLlmisvcControllerManager.strategy.rollingUpdate.maxUnavailable | int | `0` |  |
| kserveLlmisvcControllerManager.strategy.type | string | `"RollingUpdate"` |  |
| kserveLlmisvcControllerManager.tolerations | list | `[]` |  |
| kserveLlmisvcControllerManager.topologySpreadConstraints | list | `[]` |  |
| kubernetesClusterDomain | string | `"cluster.local"` |  |
| llmisvcMgrSvc.ports[0].name | string | `"https"` |  |
| llmisvcMgrSvc.ports[0].port | int | `8443` |  |
| llmisvcMgrSvc.ports[0].protocol | string | `"TCP"` |  |
| llmisvcMgrSvc.ports[0].targetPort | string | `"metrics"` |  |
| llmisvcMgrSvc.type | string | `"ClusterIP"` |  |
| llmisvcWebhookSvc.ports[0].port | int | `443` |  |
| llmisvcWebhookSvc.ports[0].targetPort | string | `"webhook-server"` |  |
| llmisvcWebhookSvc.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.automount | bool | `true` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` |  |
