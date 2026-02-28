# kserve-localmodel-resources

![Version: v0.16.0](https://img.shields.io/badge/Version-v0.16.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.16.0](https://img.shields.io/badge/AppVersion-v0.16.0-informational?style=flat-square)

KServe LocalModel - Local Model Storage and Caching for Edge and On-Premise Deployments

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
| kserve.localmodel.controller.image | string | `"kserve/kserve-localmodel-controller"` |  |
| kserve.localmodel.controller.imagePullPolicy | string | `"Always"` |  |
| kserve.localmodel.controller.resources.limits.cpu | string | `"100m"` |  |
| kserve.localmodel.controller.resources.limits.memory | string | `"300Mi"` |  |
| kserve.localmodel.controller.resources.requests.cpu | string | `"100m"` |  |
| kserve.localmodel.controller.resources.requests.memory | string | `"200Mi"` |  |
| kserve.localmodel.controller.tag | string | `""` |  |
| kserve.localmodelnode.controller.affinity | object | `{}` |  |
| kserve.localmodelnode.controller.image | string | `"kserve/kserve-localmodelnode-agent"` |  |
| kserve.localmodelnode.controller.imagePullPolicy | string | `"Always"` |  |
| kserve.localmodelnode.controller.nodeSelector.kserve/localmodel | string | `"worker"` |  |
| kserve.localmodelnode.controller.resources.limits.cpu | string | `"100m"` |  |
| kserve.localmodelnode.controller.resources.limits.memory | string | `"300Mi"` |  |
| kserve.localmodelnode.controller.resources.requests.cpu | string | `"100m"` |  |
| kserve.localmodelnode.controller.resources.requests.memory | string | `"200Mi"` |  |
| kserve.localmodelnode.controller.tag | string | `""` |  |
| kserve.localmodelnode.controller.tolerations | list | `[]` |  |
| kserve.version | string | `"v0.16.0"` |  |

