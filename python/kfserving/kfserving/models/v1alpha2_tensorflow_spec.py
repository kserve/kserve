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

from kubernetes.client import V1ResourceRequirements  # noqa: F401,E501


class V1alpha2TensorflowSpec(object):
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
        'resources': 'V1ResourceRequirements',
        'runtime_version': 'str',
        'storage_uri': 'str'
    }

    attribute_map = {
        'resources': 'resources',
        'runtime_version': 'runtimeVersion',
        'storage_uri': 'storageUri'
    }

    def __init__(self, resources=None, runtime_version=None, storage_uri=None):  # noqa: E501
        """V1alpha2TensorflowSpec - a model defined in Swagger"""  # noqa: E501

        self._resources = None
        self._runtime_version = None
        self._storage_uri = None
        self.discriminator = None

        if resources is not None:
            self.resources = resources
        if runtime_version is not None:
            self.runtime_version = runtime_version
        self.storage_uri = storage_uri

    @property
    def resources(self):
        """Gets the resources of this V1alpha2TensorflowSpec.  # noqa: E501

        Defaults to requests and limits of 1CPU, 2Gb MEM.  # noqa: E501

        :return: The resources of this V1alpha2TensorflowSpec.  # noqa: E501
        :rtype: V1ResourceRequirements
        """
        return self._resources

    @resources.setter
    def resources(self, resources):
        """Sets the resources of this V1alpha2TensorflowSpec.

        Defaults to requests and limits of 1CPU, 2Gb MEM.  # noqa: E501

        :param resources: The resources of this V1alpha2TensorflowSpec.  # noqa: E501
        :type: V1ResourceRequirements
        """

        self._resources = resources

    @property
    def runtime_version(self):
        """Gets the runtime_version of this V1alpha2TensorflowSpec.  # noqa: E501

        TFServing docker image version(https://hub.docker.com/r/tensorflow/serving), default version can be set in the inferenceservice configmap.  # noqa: E501

        :return: The runtime_version of this V1alpha2TensorflowSpec.  # noqa: E501
        :rtype: str
        """
        return self._runtime_version

    @runtime_version.setter
    def runtime_version(self, runtime_version):
        """Sets the runtime_version of this V1alpha2TensorflowSpec.

        TFServing docker image version(https://hub.docker.com/r/tensorflow/serving), default version can be set in the inferenceservice configmap.  # noqa: E501

        :param runtime_version: The runtime_version of this V1alpha2TensorflowSpec.  # noqa: E501
        :type: str
        """

        self._runtime_version = runtime_version

    @property
    def storage_uri(self):
        """Gets the storage_uri of this V1alpha2TensorflowSpec.  # noqa: E501

        The URI for the saved model(https://www.tensorflow.org/tutorials/keras/save_and_load)  # noqa: E501

        :return: The storage_uri of this V1alpha2TensorflowSpec.  # noqa: E501
        :rtype: str
        """
        return self._storage_uri

    @storage_uri.setter
    def storage_uri(self, storage_uri):
        """Sets the storage_uri of this V1alpha2TensorflowSpec.

        The URI for the saved model(https://www.tensorflow.org/tutorials/keras/save_and_load)  # noqa: E501

        :param storage_uri: The storage_uri of this V1alpha2TensorflowSpec.  # noqa: E501
        :type: str
        """
        if storage_uri is None:
            raise ValueError("Invalid value for `storage_uri`, must not be `None`")  # noqa: E501

        self._storage_uri = storage_uri

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
        if issubclass(V1alpha2TensorflowSpec, dict):
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
        if not isinstance(other, V1alpha2TensorflowSpec):
            return False

        return self.__dict__ == other.__dict__

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        return not self == other
