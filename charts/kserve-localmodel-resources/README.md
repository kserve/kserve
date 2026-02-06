# kserve-localmodel-resources

![Version: v0.16.0](https://img.shields.io/badge/Version-v0.16.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.16.0](https://img.shields.io/badge/AppVersion-v0.16.0-informational?style=flat-square)

KServe LocalModel - Local Model Storage and Caching for Edge and On-Premise Deployments

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| kserve.version | string | `"v0.16.0"` |  |
| localmodel.controller.containers.manager.image | string | `"kserve/kserve-localmodel-controller"` |  |
| localmodel.controller.containers.manager.imagePullPolicy | string | `"Always"` |  |
| localmodel.controller.containers.manager.resources.limits.cpu | string | `"100m"` |  |
| localmodel.controller.containers.manager.resources.limits.memory | string | `"300Mi"` |  |
| localmodel.controller.containers.manager.resources.requests.cpu | string | `"100m"` |  |
| localmodel.controller.containers.manager.resources.requests.memory | string | `"200Mi"` |  |
| localmodel.controller.containers.manager.tag | string | `""` |  |
| localmodel.nodeAgent.affinity | object | `{}` |  |
| localmodel.nodeAgent.containers.manager.image | string | `"kserve/kserve-localmodelnode-agent"` |  |
| localmodel.nodeAgent.containers.manager.imagePullPolicy | string | `"Always"` |  |
| localmodel.nodeAgent.containers.manager.resources.limits.cpu | string | `"100m"` |  |
| localmodel.nodeAgent.containers.manager.resources.limits.memory | string | `"300Mi"` |  |
| localmodel.nodeAgent.containers.manager.resources.requests.cpu | string | `"100m"` |  |
| localmodel.nodeAgent.containers.manager.resources.requests.memory | string | `"200Mi"` |  |
| localmodel.nodeAgent.containers.manager.tag | string | `""` |  |
| localmodel.nodeAgent.nodeSelector.kserve/localmodel | string | `"worker"` |  |
| localmodel.nodeAgent.tolerations | list | `[]` |  |

