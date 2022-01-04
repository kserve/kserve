# Copyright 2021 The KServe Authors.
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
    KServe

    Python SDK for KServe  # noqa: E501

    The version of the OpenAPI document: v0.1
    Generated by: https://openapi-generator.tech
"""


import pprint
import re  # noqa: F401

import six

from kserve.configuration import Configuration


class V1alpha1BuiltInAdapter(object):
    """NOTE: This class is auto generated by OpenAPI Generator.
    Ref: https://openapi-generator.tech

    Do not edit the class manually.
    """

    """
    Attributes:
      openapi_types (dict): The key is attribute name
                            and the value is attribute type.
      attribute_map (dict): The key is attribute name
                            and the value is json key in definition.
    """
    openapi_types = {
        'mem_buffer_bytes': 'int',
        'model_loading_timeout_millis': 'int',
        'runtime_management_port': 'int',
        'server_type': 'str'
    }

    attribute_map = {
        'mem_buffer_bytes': 'memBufferBytes',
        'model_loading_timeout_millis': 'modelLoadingTimeoutMillis',
        'runtime_management_port': 'runtimeManagementPort',
        'server_type': 'serverType'
    }

    def __init__(self, mem_buffer_bytes=None, model_loading_timeout_millis=None, runtime_management_port=None, server_type=None, local_vars_configuration=None):  # noqa: E501
        """V1alpha1BuiltInAdapter - a model defined in OpenAPI"""  # noqa: E501
        if local_vars_configuration is None:
            local_vars_configuration = Configuration()
        self.local_vars_configuration = local_vars_configuration

        self._mem_buffer_bytes = None
        self._model_loading_timeout_millis = None
        self._runtime_management_port = None
        self._server_type = None
        self.discriminator = None

        if mem_buffer_bytes is not None:
            self.mem_buffer_bytes = mem_buffer_bytes
        if model_loading_timeout_millis is not None:
            self.model_loading_timeout_millis = model_loading_timeout_millis
        if runtime_management_port is not None:
            self.runtime_management_port = runtime_management_port
        if server_type is not None:
            self.server_type = server_type

    @property
    def mem_buffer_bytes(self):
        """Gets the mem_buffer_bytes of this V1alpha1BuiltInAdapter.  # noqa: E501

        Fixed memory overhead to subtract from runtime container's memory allocation to determine model capacity  # noqa: E501

        :return: The mem_buffer_bytes of this V1alpha1BuiltInAdapter.  # noqa: E501
        :rtype: int
        """
        return self._mem_buffer_bytes

    @mem_buffer_bytes.setter
    def mem_buffer_bytes(self, mem_buffer_bytes):
        """Sets the mem_buffer_bytes of this V1alpha1BuiltInAdapter.

        Fixed memory overhead to subtract from runtime container's memory allocation to determine model capacity  # noqa: E501

        :param mem_buffer_bytes: The mem_buffer_bytes of this V1alpha1BuiltInAdapter.  # noqa: E501
        :type: int
        """

        self._mem_buffer_bytes = mem_buffer_bytes

    @property
    def model_loading_timeout_millis(self):
        """Gets the model_loading_timeout_millis of this V1alpha1BuiltInAdapter.  # noqa: E501

        Timeout for model loading operations in milliseconds  # noqa: E501

        :return: The model_loading_timeout_millis of this V1alpha1BuiltInAdapter.  # noqa: E501
        :rtype: int
        """
        return self._model_loading_timeout_millis

    @model_loading_timeout_millis.setter
    def model_loading_timeout_millis(self, model_loading_timeout_millis):
        """Sets the model_loading_timeout_millis of this V1alpha1BuiltInAdapter.

        Timeout for model loading operations in milliseconds  # noqa: E501

        :param model_loading_timeout_millis: The model_loading_timeout_millis of this V1alpha1BuiltInAdapter.  # noqa: E501
        :type: int
        """

        self._model_loading_timeout_millis = model_loading_timeout_millis

    @property
    def runtime_management_port(self):
        """Gets the runtime_management_port of this V1alpha1BuiltInAdapter.  # noqa: E501

        Port which the runtime server listens for model management requests  # noqa: E501

        :return: The runtime_management_port of this V1alpha1BuiltInAdapter.  # noqa: E501
        :rtype: int
        """
        return self._runtime_management_port

    @runtime_management_port.setter
    def runtime_management_port(self, runtime_management_port):
        """Sets the runtime_management_port of this V1alpha1BuiltInAdapter.

        Port which the runtime server listens for model management requests  # noqa: E501

        :param runtime_management_port: The runtime_management_port of this V1alpha1BuiltInAdapter.  # noqa: E501
        :type: int
        """

        self._runtime_management_port = runtime_management_port

    @property
    def server_type(self):
        """Gets the server_type of this V1alpha1BuiltInAdapter.  # noqa: E501

        ServerType can be one of triton/mlserver and the runtime's container must have the same name  # noqa: E501

        :return: The server_type of this V1alpha1BuiltInAdapter.  # noqa: E501
        :rtype: str
        """
        return self._server_type

    @server_type.setter
    def server_type(self, server_type):
        """Sets the server_type of this V1alpha1BuiltInAdapter.

        ServerType can be one of triton/mlserver and the runtime's container must have the same name  # noqa: E501

        :param server_type: The server_type of this V1alpha1BuiltInAdapter.  # noqa: E501
        :type: str
        """

        self._server_type = server_type

    def to_dict(self):
        """Returns the model properties as a dict"""
        result = {}

        for attr, _ in six.iteritems(self.openapi_types):
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

        return result

    def to_str(self):
        """Returns the string representation of the model"""
        return pprint.pformat(self.to_dict())

    def __repr__(self):
        """For `print` and `pprint`"""
        return self.to_str()

    def __eq__(self, other):
        """Returns true if both objects are equal"""
        if not isinstance(other, V1alpha1BuiltInAdapter):
            return False

        return self.to_dict() == other.to_dict()

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        if not isinstance(other, V1alpha1BuiltInAdapter):
            return True

        return self.to_dict() != other.to_dict()
