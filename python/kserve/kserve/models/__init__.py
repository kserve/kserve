# Copyright 2023 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# coding: utf-8

# flake8: noqa
"""
    KServe

    Python SDK for KServe  # noqa: E501

    The version of the OpenAPI document: v0.1
    Generated by: https://openapi-generator.tech
"""


from __future__ import absolute_import

# import models into model package
from kserve.models.v1alpha1_built_in_adapter import V1alpha1BuiltInAdapter
from kserve.models.v1alpha1_cluster_serving_runtime import V1alpha1ClusterServingRuntime
from kserve.models.v1alpha1_cluster_serving_runtime_list import V1alpha1ClusterServingRuntimeList
from kserve.models.v1alpha1_cluster_storage_container import V1alpha1ClusterStorageContainer
from kserve.models.v1alpha1_cluster_storage_container_list import V1alpha1ClusterStorageContainerList
from kserve.models.v1alpha1_inferece_graph_router_timeouts import V1alpha1InfereceGraphRouterTimeouts
from kserve.models.v1alpha1_inference_graph import V1alpha1InferenceGraph
from kserve.models.v1alpha1_inference_graph_list import V1alpha1InferenceGraphList
from kserve.models.v1alpha1_inference_graph_spec import V1alpha1InferenceGraphSpec
from kserve.models.v1alpha1_inference_graph_status import V1alpha1InferenceGraphStatus
from kserve.models.v1alpha1_inference_router import V1alpha1InferenceRouter
from kserve.models.v1alpha1_inference_step import V1alpha1InferenceStep
from kserve.models.v1alpha1_inference_target import V1alpha1InferenceTarget
from kserve.models.v1alpha1_llm_inference_service import V1alpha1LLMInferenceService
from kserve.models.v1alpha1_llm_inference_service_config import V1alpha1LLMInferenceServiceConfig
from kserve.models.v1alpha1_llm_inference_service_config_list import V1alpha1LLMInferenceServiceConfigList
from kserve.models.v1alpha1_llm_inference_service_list import V1alpha1LLMInferenceServiceList
from kserve.models.v1alpha1_local_model_cache import V1alpha1LocalModelCache
from kserve.models.v1alpha1_local_model_cache_list import V1alpha1LocalModelCacheList
from kserve.models.v1alpha1_local_model_cache_spec import V1alpha1LocalModelCacheSpec
from kserve.models.v1alpha1_local_model_node import V1alpha1LocalModelNode
from kserve.models.v1alpha1_local_model_node_group import V1alpha1LocalModelNodeGroup
from kserve.models.v1alpha1_local_model_node_group_list import V1alpha1LocalModelNodeGroupList
from kserve.models.v1alpha1_local_model_node_group_spec import V1alpha1LocalModelNodeGroupSpec
from kserve.models.v1alpha1_local_model_node_list import V1alpha1LocalModelNodeList
from kserve.models.v1alpha1_local_model_node_spec import V1alpha1LocalModelNodeSpec
from kserve.models.v1alpha1_model_spec import V1alpha1ModelSpec
from kserve.models.v1alpha1_serving_runtime import V1alpha1ServingRuntime
from kserve.models.v1alpha1_serving_runtime_list import V1alpha1ServingRuntimeList
from kserve.models.v1alpha1_serving_runtime_pod_spec import V1alpha1ServingRuntimePodSpec
from kserve.models.v1alpha1_serving_runtime_spec import V1alpha1ServingRuntimeSpec
from kserve.models.v1alpha1_storage_container_spec import V1alpha1StorageContainerSpec
from kserve.models.v1alpha1_storage_helper import V1alpha1StorageHelper
from kserve.models.v1alpha1_supported_model_format import V1alpha1SupportedModelFormat
from kserve.models.v1alpha1_supported_uri_format import V1alpha1SupportedUriFormat
from kserve.models.v1alpha1_trained_model import V1alpha1TrainedModel
from kserve.models.v1alpha1_trained_model_list import V1alpha1TrainedModelList
from kserve.models.v1alpha1_trained_model_spec import V1alpha1TrainedModelSpec
from kserve.models.v1beta1_art_explainer_spec import V1beta1ARTExplainerSpec
from kserve.models.v1beta1_authentication_ref import V1beta1AuthenticationRef
from kserve.models.v1beta1_auto_scaling_spec import V1beta1AutoScalingSpec
from kserve.models.v1beta1_batcher import V1beta1Batcher
from kserve.models.v1beta1_component_extension_spec import V1beta1ComponentExtensionSpec
from kserve.models.v1beta1_component_status_spec import V1beta1ComponentStatusSpec
from kserve.models.v1beta1_custom_explainer import V1beta1CustomExplainer
from kserve.models.v1beta1_custom_predictor import V1beta1CustomPredictor
from kserve.models.v1beta1_custom_transformer import V1beta1CustomTransformer
from kserve.models.v1beta1_deploy_config import V1beta1DeployConfig
from kserve.models.v1beta1_explainer_config import V1beta1ExplainerConfig
from kserve.models.v1beta1_explainer_extension_spec import V1beta1ExplainerExtensionSpec
from kserve.models.v1beta1_explainer_spec import V1beta1ExplainerSpec
from kserve.models.v1beta1_explainers_config import V1beta1ExplainersConfig
from kserve.models.v1beta1_ext_metric_authentication import V1beta1ExtMetricAuthentication
from kserve.models.v1beta1_external_metric_source import V1beta1ExternalMetricSource
from kserve.models.v1beta1_external_metrics import V1beta1ExternalMetrics
from kserve.models.v1beta1_failure_info import V1beta1FailureInfo
from kserve.models.v1beta1_hugging_face_runtime_spec import V1beta1HuggingFaceRuntimeSpec
from kserve.models.v1beta1_inference_service import V1beta1InferenceService
from kserve.models.v1beta1_inference_service_list import V1beta1InferenceServiceList
from kserve.models.v1beta1_inference_service_spec import V1beta1InferenceServiceSpec
from kserve.models.v1beta1_inference_service_status import V1beta1InferenceServiceStatus
from kserve.models.v1beta1_inference_services_config import V1beta1InferenceServicesConfig
from kserve.models.v1beta1_ingress_config import V1beta1IngressConfig
from kserve.models.v1beta1_light_gbm_spec import V1beta1LightGBMSpec
from kserve.models.v1beta1_local_model_config import V1beta1LocalModelConfig
from kserve.models.v1beta1_logger_spec import V1beta1LoggerSpec
from kserve.models.v1beta1_logger_storage_spec import V1beta1LoggerStorageSpec
from kserve.models.v1beta1_metric_target import V1beta1MetricTarget
from kserve.models.v1beta1_metrics_spec import V1beta1MetricsSpec
from kserve.models.v1beta1_model_copies import V1beta1ModelCopies
from kserve.models.v1beta1_model_format import V1beta1ModelFormat
from kserve.models.v1beta1_model_revision_states import V1beta1ModelRevisionStates
from kserve.models.v1beta1_model_spec import V1beta1ModelSpec
from kserve.models.v1beta1_model_status import V1beta1ModelStatus
from kserve.models.v1beta1_model_storage_spec import V1beta1ModelStorageSpec
from kserve.models.v1beta1_multi_node_config import V1beta1MultiNodeConfig
from kserve.models.v1beta1_onnx_runtime_spec import V1beta1ONNXRuntimeSpec
from kserve.models.v1beta1_otel_collector_config import V1beta1OtelCollectorConfig
from kserve.models.v1beta1_pmml_spec import V1beta1PMMLSpec
from kserve.models.v1beta1_paddle_server_spec import V1beta1PaddleServerSpec
from kserve.models.v1beta1_pod_metric_source import V1beta1PodMetricSource
from kserve.models.v1beta1_pod_metrics import V1beta1PodMetrics
from kserve.models.v1beta1_pod_spec import V1beta1PodSpec
from kserve.models.v1beta1_predictor_extension_spec import V1beta1PredictorExtensionSpec
from kserve.models.v1beta1_predictor_spec import V1beta1PredictorSpec
from kserve.models.v1beta1_resource_config import V1beta1ResourceConfig
from kserve.models.v1beta1_resource_metric_source import V1beta1ResourceMetricSource
from kserve.models.v1beta1_sk_learn_spec import V1beta1SKLearnSpec
from kserve.models.v1beta1_security_config import V1beta1SecurityConfig
from kserve.models.v1beta1_service_config import V1beta1ServiceConfig
from kserve.models.v1beta1_storage_spec import V1beta1StorageSpec
from kserve.models.v1beta1_tf_serving_spec import V1beta1TFServingSpec
from kserve.models.v1beta1_torch_serve_spec import V1beta1TorchServeSpec
from kserve.models.v1beta1_transformer_spec import V1beta1TransformerSpec
from kserve.models.v1beta1_triton_spec import V1beta1TritonSpec
from kserve.models.v1beta1_worker_spec import V1beta1WorkerSpec
from kserve.models.v1beta1_xg_boost_spec import V1beta1XGBoostSpec
