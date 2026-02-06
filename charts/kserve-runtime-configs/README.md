# kserve-runtime-configs

![Version: v0.16.0](https://img.shields.io/badge/Version-v0.16.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.16.0](https://img.shields.io/badge/AppVersion-v0.16.0-informational?style=flat-square)

KServe Runtime Configurations - ClusterServingRuntimes and LLM Inference Configs

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| kserve.version | string | `"v0.16.0"` |  |
| llmisvcConfigs.enabled | bool | `false` |  |
| runtimes.enabled | bool | `true` |  |
| runtimes.huggingface.enabled | bool | `true` |  |
| runtimes.huggingface.image.repository | string | `"kserve/huggingfaceserver"` |  |
| runtimes.huggingface.image.tag | string | `""` |  |
| runtimes.huggingface.resources.limits.cpu | string | `"1"` |  |
| runtimes.huggingface.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.huggingface.resources.requests.cpu | string | `"1"` |  |
| runtimes.huggingface.resources.requests.memory | string | `"2Gi"` |  |
| runtimes.huggingfaceMultinode.enabled | bool | `true` |  |
| runtimes.huggingfaceMultinode.image.repository | string | `"kserve/huggingfaceserver"` |  |
| runtimes.huggingfaceMultinode.image.tag | string | `"v0.16.0-gpu"` |  |
| runtimes.huggingfaceMultinode.resources.limits.cpu | string | `"4"` |  |
| runtimes.huggingfaceMultinode.resources.limits.memory | string | `"12Gi"` |  |
| runtimes.huggingfaceMultinode.resources.requests.cpu | string | `"2"` |  |
| runtimes.huggingfaceMultinode.resources.requests.memory | string | `"6Gi"` |  |
| runtimes.lightgbm.enabled | bool | `true` |  |
| runtimes.lightgbm.image.repository | string | `"kserve/lgbserver"` |  |
| runtimes.lightgbm.image.tag | string | `""` |  |
| runtimes.lightgbm.resources.limits.cpu | string | `"1"` |  |
| runtimes.lightgbm.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.lightgbm.resources.requests.cpu | string | `"1"` |  |
| runtimes.lightgbm.resources.requests.memory | string | `"2Gi"` |  |
| runtimes.mlserver.enabled | bool | `true` |  |
| runtimes.mlserver.image.repository | string | `"docker.io/seldonio/mlserver"` |  |
| runtimes.mlserver.image.tag | string | `"1.5.0"` |  |
| runtimes.mlserver.resources.limits.cpu | string | `"1"` |  |
| runtimes.mlserver.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.mlserver.resources.requests.cpu | string | `"1"` |  |
| runtimes.mlserver.resources.requests.memory | string | `"2Gi"` |  |
| runtimes.paddle.enabled | bool | `true` |  |
| runtimes.paddle.image.repository | string | `"kserve/paddleserver"` |  |
| runtimes.paddle.image.tag | string | `""` |  |
| runtimes.paddle.resources.limits.cpu | string | `"1"` |  |
| runtimes.paddle.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.paddle.resources.requests.cpu | string | `"1"` |  |
| runtimes.paddle.resources.requests.memory | string | `"2Gi"` |  |
| runtimes.pmml.enabled | bool | `true` |  |
| runtimes.pmml.image.repository | string | `"kserve/pmmlserver"` |  |
| runtimes.pmml.image.tag | string | `""` |  |
| runtimes.pmml.resources.limits.cpu | string | `"1"` |  |
| runtimes.pmml.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.pmml.resources.requests.cpu | string | `"1"` |  |
| runtimes.pmml.resources.requests.memory | string | `"2Gi"` |  |
| runtimes.predictive.enabled | bool | `true` |  |
| runtimes.predictive.image.repository | string | `"kserve/predictiveserver"` |  |
| runtimes.predictive.image.tag | string | `""` |  |
| runtimes.predictive.resources.limits.cpu | string | `"1"` |  |
| runtimes.predictive.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.predictive.resources.requests.cpu | string | `"1"` |  |
| runtimes.predictive.resources.requests.memory | string | `"2Gi"` |  |
| runtimes.sklearn.enabled | bool | `true` |  |
| runtimes.sklearn.image.repository | string | `"kserve/sklearnserver"` |  |
| runtimes.sklearn.image.tag | string | `""` |  |
| runtimes.sklearn.resources.limits.cpu | string | `"1"` |  |
| runtimes.sklearn.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.sklearn.resources.requests.cpu | string | `"1"` |  |
| runtimes.sklearn.resources.requests.memory | string | `"2Gi"` |  |
| runtimes.tensorflow.enabled | bool | `true` |  |
| runtimes.tensorflow.image.repository | string | `"tensorflow/serving"` |  |
| runtimes.tensorflow.image.tag | string | `"2.6.2"` |  |
| runtimes.tensorflow.resources.limits.cpu | string | `"1"` |  |
| runtimes.tensorflow.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.tensorflow.resources.requests.cpu | string | `"1"` |  |
| runtimes.tensorflow.resources.requests.memory | string | `"2Gi"` |  |
| runtimes.torchserve.enabled | bool | `true` |  |
| runtimes.torchserve.image.repository | string | `"pytorch/torchserve-kfs"` |  |
| runtimes.torchserve.image.tag | string | `"0.9.0"` |  |
| runtimes.torchserve.resources.limits.cpu | string | `"1"` |  |
| runtimes.torchserve.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.torchserve.resources.requests.cpu | string | `"1"` |  |
| runtimes.torchserve.resources.requests.memory | string | `"2Gi"` |  |
| runtimes.triton.enabled | bool | `true` |  |
| runtimes.triton.image.repository | string | `"nvcr.io/nvidia/tritonserver"` |  |
| runtimes.triton.image.tag | string | `"23.05-py3"` |  |
| runtimes.triton.resources.limits.cpu | string | `"1"` |  |
| runtimes.triton.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.triton.resources.requests.cpu | string | `"1"` |  |
| runtimes.triton.resources.requests.memory | string | `"2Gi"` |  |
| runtimes.xgboost.enabled | bool | `true` |  |
| runtimes.xgboost.image.repository | string | `"kserve/xgbserver"` |  |
| runtimes.xgboost.image.tag | string | `""` |  |
| runtimes.xgboost.resources.limits.cpu | string | `"1"` |  |
| runtimes.xgboost.resources.limits.memory | string | `"2Gi"` |  |
| runtimes.xgboost.resources.requests.cpu | string | `"1"` |  |
| runtimes.xgboost.resources.requests.memory | string | `"2Gi"` |  |

