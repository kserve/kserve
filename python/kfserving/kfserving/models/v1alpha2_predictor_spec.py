# Copyright 2020 kubeflow.org.
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

"""
    KFServing

    Python SDK for KFServing  # noqa: E501

    OpenAPI spec version: v0.1
    
    Generated by: https://github.com/swagger-api/swagger-codegen.git
"""


import pprint
import re  # noqa: F401

import six

from kfserving.models.v1alpha2_custom_spec import V1alpha2CustomSpec  # noqa: F401,E501
from kfserving.models.v1alpha2_logger import V1alpha2Logger  # noqa: F401,E501
from kfserving.models.v1alpha2_onnx_spec import V1alpha2ONNXSpec  # noqa: F401,E501
from kfserving.models.v1alpha2_py_torch_spec import V1alpha2PyTorchSpec  # noqa: F401,E501
from kfserving.models.v1alpha2_sk_learn_spec import V1alpha2SKLearnSpec  # noqa: F401,E501
from kfserving.models.v1alpha2_tensor_rt_spec import V1alpha2TensorRTSpec  # noqa: F401,E501
from kfserving.models.v1alpha2_tensorflow_spec import V1alpha2TensorflowSpec  # noqa: F401,E501
from kfserving.models.v1alpha2_xg_boost_spec import V1alpha2XGBoostSpec  # noqa: F401,E501


class V1alpha2PredictorSpec(object):
    """NOTE: This class is auto generated by the swagger code generator program.

    Do not edit the class manually.
    """

    """
    Attributes:
      swagger_types (dict): The key is attribute name
                            and the value is attribute type.
      attribute_map (dict): The key is attribute name
                            and the value is json key in definition.
    """
    swagger_types = {
        'custom': 'V1alpha2CustomSpec',
        'logger': 'V1alpha2Logger',
        'max_replicas': 'int',
        'min_replicas': 'int',
        'onnx': 'V1alpha2ONNXSpec',
        'parallelism': 'int',
        'pytorch': 'V1alpha2PyTorchSpec',
        'service_account_name': 'str',
        'sklearn': 'V1alpha2SKLearnSpec',
        'tensorflow': 'V1alpha2TensorflowSpec',
        'triton': 'V1alpha2TensorRTSpec',
        'xgboost': 'V1alpha2XGBoostSpec'
    }

    attribute_map = {
        'custom': 'custom',
        'logger': 'logger',
        'max_replicas': 'maxReplicas',
        'min_replicas': 'minReplicas',
        'onnx': 'onnx',
        'parallelism': 'parallelism',
        'pytorch': 'pytorch',
        'service_account_name': 'serviceAccountName',
        'sklearn': 'sklearn',
        'tensorflow': 'tensorflow',
        'triton': 'triton',
        'xgboost': 'xgboost'
    }

    def __init__(self, custom=None, logger=None, max_replicas=None, min_replicas=None, onnx=None, parallelism=None, pytorch=None, service_account_name=None, sklearn=None, tensorflow=None, tensorrt=None, xgboost=None):  # noqa: E501
        """V1alpha2PredictorSpec - a model defined in Swagger"""  # noqa: E501

        self._custom = None
        self._logger = None
        self._max_replicas = None
        self._min_replicas = None
        self._onnx = None
        self._parallelism = None
        self._pytorch = None
        self._service_account_name = None
        self._sklearn = None
        self._tensorflow = None
        self._tensorrt = None
        self._xgboost = None
        self.discriminator = None

        if custom is not None:
            self.custom = custom
        if logger is not None:
            self.logger = logger
        if max_replicas is not None:
            self.max_replicas = max_replicas
        if min_replicas is not None:
            self.min_replicas = min_replicas
        if onnx is not None:
            self.onnx = onnx
        if parallelism is not None:
            self.parallelism = parallelism
        if pytorch is not None:
            self.pytorch = pytorch
        if service_account_name is not None:
            self.service_account_name = service_account_name
        if sklearn is not None:
            self.sklearn = sklearn
        if tensorflow is not None:
            self.tensorflow = tensorflow
        if tensorrt is not None:
            self.tensorrt = tensorrt
        if xgboost is not None:
            self.xgboost = xgboost

    @property
    def custom(self):
        """Gets the custom of this V1alpha2PredictorSpec.  # noqa: E501

        Spec for a custom predictor  # noqa: E501

        :return: The custom of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: V1alpha2CustomSpec
        """
        return self._custom

    @custom.setter
    def custom(self, custom):
        """Sets the custom of this V1alpha2PredictorSpec.

        Spec for a custom predictor  # noqa: E501

        :param custom: The custom of this V1alpha2PredictorSpec.  # noqa: E501
        :type: V1alpha2CustomSpec
        """

        self._custom = custom

    @property
    def logger(self):
        """Gets the logger of this V1alpha2PredictorSpec.  # noqa: E501

        Activate request/response logging  # noqa: E501

        :return: The logger of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: V1alpha2Logger
        """
        return self._logger

    @logger.setter
    def logger(self, logger):
        """Sets the logger of this V1alpha2PredictorSpec.

        Activate request/response logging  # noqa: E501

        :param logger: The logger of this V1alpha2PredictorSpec.  # noqa: E501
        :type: V1alpha2Logger
        """

        self._logger = logger

    @property
    def max_replicas(self):
        """Gets the max_replicas of this V1alpha2PredictorSpec.  # noqa: E501

        This is the up bound for autoscaler to scale to  # noqa: E501

        :return: The max_replicas of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: int
        """
        return self._max_replicas

    @max_replicas.setter
    def max_replicas(self, max_replicas):
        """Sets the max_replicas of this V1alpha2PredictorSpec.

        This is the up bound for autoscaler to scale to  # noqa: E501

        :param max_replicas: The max_replicas of this V1alpha2PredictorSpec.  # noqa: E501
        :type: int
        """

        self._max_replicas = max_replicas

    @property
    def min_replicas(self):
        """Gets the min_replicas of this V1alpha2PredictorSpec.  # noqa: E501

        Minimum number of replicas, pods won't scale down to 0 in case of no traffic  # noqa: E501

        :return: The min_replicas of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: int
        """
        return self._min_replicas

    @min_replicas.setter
    def min_replicas(self, min_replicas):
        """Sets the min_replicas of this V1alpha2PredictorSpec.

        Minimum number of replicas, pods won't scale down to 0 in case of no traffic  # noqa: E501

        :param min_replicas: The min_replicas of this V1alpha2PredictorSpec.  # noqa: E501
        :type: int
        """

        self._min_replicas = min_replicas

    @property
    def onnx(self):
        """Gets the onnx of this V1alpha2PredictorSpec.  # noqa: E501

        Spec for ONNX runtime (https://github.com/microsoft/onnxruntime)  # noqa: E501

        :return: The onnx of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: V1alpha2ONNXSpec
        """
        return self._onnx

    @onnx.setter
    def onnx(self, onnx):
        """Sets the onnx of this V1alpha2PredictorSpec.

        Spec for ONNX runtime (https://github.com/microsoft/onnxruntime)  # noqa: E501

        :param onnx: The onnx of this V1alpha2PredictorSpec.  # noqa: E501
        :type: V1alpha2ONNXSpec
        """

        self._onnx = onnx

    @property
    def parallelism(self):
        """Gets the parallelism of this V1alpha2PredictorSpec.  # noqa: E501

        Parallelism specifies how many requests can be processed concurrently, this sets the target concurrency for Autoscaling(KPA). For model servers that support tuning parallelism will use this value, by default the parallelism is the number of the CPU cores for most of the model servers.  # noqa: E501

        :return: The parallelism of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: int
        """
        return self._parallelism

    @parallelism.setter
    def parallelism(self, parallelism):
        """Sets the parallelism of this V1alpha2PredictorSpec.

        Parallelism specifies how many requests can be processed concurrently, this sets the target concurrency for Autoscaling(KPA). For model servers that support tuning parallelism will use this value, by default the parallelism is the number of the CPU cores for most of the model servers.  # noqa: E501

        :param parallelism: The parallelism of this V1alpha2PredictorSpec.  # noqa: E501
        :type: int
        """

        self._parallelism = parallelism

    @property
    def pytorch(self):
        """Gets the pytorch of this V1alpha2PredictorSpec.  # noqa: E501

        Spec for PyTorch predictor  # noqa: E501

        :return: The pytorch of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: V1alpha2PyTorchSpec
        """
        return self._pytorch

    @pytorch.setter
    def pytorch(self, pytorch):
        """Sets the pytorch of this V1alpha2PredictorSpec.

        Spec for PyTorch predictor  # noqa: E501

        :param pytorch: The pytorch of this V1alpha2PredictorSpec.  # noqa: E501
        :type: V1alpha2PyTorchSpec
        """

        self._pytorch = pytorch

    @property
    def service_account_name(self):
        """Gets the service_account_name of this V1alpha2PredictorSpec.  # noqa: E501

        ServiceAccountName is the name of the ServiceAccount to use to run the service  # noqa: E501

        :return: The service_account_name of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: str
        """
        return self._service_account_name

    @service_account_name.setter
    def service_account_name(self, service_account_name):
        """Sets the service_account_name of this V1alpha2PredictorSpec.

        ServiceAccountName is the name of the ServiceAccount to use to run the service  # noqa: E501

        :param service_account_name: The service_account_name of this V1alpha2PredictorSpec.  # noqa: E501
        :type: str
        """

        self._service_account_name = service_account_name

    @property
    def sklearn(self):
        """Gets the sklearn of this V1alpha2PredictorSpec.  # noqa: E501

        Spec for SKLearn predictor  # noqa: E501

        :return: The sklearn of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: V1alpha2SKLearnSpec
        """
        return self._sklearn

    @sklearn.setter
    def sklearn(self, sklearn):
        """Sets the sklearn of this V1alpha2PredictorSpec.

        Spec for SKLearn predictor  # noqa: E501

        :param sklearn: The sklearn of this V1alpha2PredictorSpec.  # noqa: E501
        :type: V1alpha2SKLearnSpec
        """

        self._sklearn = sklearn

    @property
    def tensorflow(self):
        """Gets the tensorflow of this V1alpha2PredictorSpec.  # noqa: E501

        Spec for Tensorflow Serving (https://github.com/tensorflow/serving)  # noqa: E501

        :return: The tensorflow of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: V1alpha2TensorflowSpec
        """
        return self._tensorflow

    @tensorflow.setter
    def tensorflow(self, tensorflow):
        """Sets the tensorflow of this V1alpha2PredictorSpec.

        Spec for Tensorflow Serving (https://github.com/tensorflow/serving)  # noqa: E501

        :param tensorflow: The tensorflow of this V1alpha2PredictorSpec.  # noqa: E501
        :type: V1alpha2TensorflowSpec
        """

        self._tensorflow = tensorflow

    @property
    def tensorrt(self):
        """Gets the triton of this V1alpha2PredictorSpec.  # noqa: E501

        Spec for TensorRT Inference Server (https://github.com/NVIDIA/tensorrt-inference-server)  # noqa: E501

        :return: The triton of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: V1alpha2TensorRTSpec
        """
        return self._tensorrt

    @tensorrt.setter
    def tensorrt(self, tensorrt):
        """Sets the triton of this V1alpha2PredictorSpec.

        Spec for TensorRT Inference Server (https://github.com/NVIDIA/tensorrt-inference-server)  # noqa: E501

        :param tensorrt: The triton of this V1alpha2PredictorSpec.  # noqa: E501
        :type: V1alpha2TensorRTSpec
        """

        self._tensorrt = tensorrt

    @property
    def xgboost(self):
        """Gets the xgboost of this V1alpha2PredictorSpec.  # noqa: E501

        Spec for XGBoost predictor  # noqa: E501

        :return: The xgboost of this V1alpha2PredictorSpec.  # noqa: E501
        :rtype: V1alpha2XGBoostSpec
        """
        return self._xgboost

    @xgboost.setter
    def xgboost(self, xgboost):
        """Sets the xgboost of this V1alpha2PredictorSpec.

        Spec for XGBoost predictor  # noqa: E501

        :param xgboost: The xgboost of this V1alpha2PredictorSpec.  # noqa: E501
        :type: V1alpha2XGBoostSpec
        """

        self._xgboost = xgboost

    def to_dict(self):
        """Returns the model properties as a dict"""
        result = {}

        for attr, _ in six.iteritems(self.swagger_types):
            value = getattr(self, attr)
            if isinstance(value, list):
                result[attr] = list(map(
                    lambda x: x.to_dict() if hasattr(x, "to_dict") else x,
                    value
                ))
            elif hasattr(value, "to_dict"):
                result[attr] = value.to_dict()
            elif isinstance(value, dict):
                result[attr] = dict(map(
                    lambda item: (item[0], item[1].to_dict())
                    if hasattr(item[1], "to_dict") else item,
                    value.items()
                ))
            else:
                result[attr] = value
        if issubclass(V1alpha2PredictorSpec, dict):
            for key, value in self.items():
                result[key] = value

        return result

    def to_str(self):
        """Returns the string representation of the model"""
        return pprint.pformat(self.to_dict())

    def __repr__(self):
        """For `print` and `pprint`"""
        return self.to_str()

    def __eq__(self, other):
        """Returns true if both objects are equal"""
        if not isinstance(other, V1alpha2PredictorSpec):
            return False

        return self.__dict__ == other.__dict__

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        return not self == other
