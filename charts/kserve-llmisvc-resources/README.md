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
| kubernetesClusterDomain | string | `"cluster.local"` |  |
| llmisvcControllerManager.manager.args[0] | string | `"--metrics-addr=127.0.0.1:8443"` |  |
| llmisvcControllerManager.manager.args[1] | string | `"--leader-elect"` |  |
| llmisvcControllerManager.manager.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| llmisvcControllerManager.manager.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| llmisvcControllerManager.manager.containerSecurityContext.privileged | bool | `false` |  |
| llmisvcControllerManager.manager.containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| llmisvcControllerManager.manager.containerSecurityContext.runAsNonRoot | bool | `true` |  |
| llmisvcControllerManager.manager.containerSecurityContext.runAsUser | int | `1000` |  |
| llmisvcControllerManager.manager.containerSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| llmisvcControllerManager.manager.image.repository | string | `"kserve/llmisvc-controller"` |  |
| llmisvcControllerManager.manager.image.tag | string | `"latest"` |  |
| llmisvcControllerManager.manager.imagePullPolicy | string | `"Always"` |  |
| llmisvcControllerManager.manager.resources.limits.cpu | string | `"100m"` |  |
| llmisvcControllerManager.manager.resources.limits.memory | string | `"300Mi"` |  |
| llmisvcControllerManager.manager.resources.requests.cpu | string | `"100m"` |  |
| llmisvcControllerManager.manager.resources.requests.memory | string | `"300Mi"` |  |
| llmisvcControllerManager.nodeSelector | object | `{}` |  |
| llmisvcControllerManager.podSecurityContext.runAsNonRoot | bool | `true` |  |
| llmisvcControllerManager.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| llmisvcControllerManager.replicas | int | `1` |  |
| llmisvcControllerManager.strategy.rollingUpdate.maxSurge | int | `1` |  |
| llmisvcControllerManager.strategy.rollingUpdate.maxUnavailable | int | `0` |  |
| llmisvcControllerManager.strategy.type | string | `"RollingUpdate"` |  |
| llmisvcControllerManager.tolerations | list | `[]` |  |
| llmisvcControllerManager.topologySpreadConstraints | list | `[]` |  |
| llmisvcControllerManagerService.ports[0].name | string | `"https"` |  |
| llmisvcControllerManagerService.ports[0].port | int | `8443` |  |
| llmisvcControllerManagerService.ports[0].protocol | string | `"TCP"` |  |
| llmisvcControllerManagerService.ports[0].targetPort | string | `"metrics"` |  |
| llmisvcControllerManagerService.type | string | `"ClusterIP"` |  |
| llmisvcWebhookServerService.ports[0].port | int | `443` |  |
| llmisvcWebhookServerService.ports[0].targetPort | string | `"webhook-server"` |  |
| llmisvcWebhookServerService.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.automount | bool | `true` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` |  |
