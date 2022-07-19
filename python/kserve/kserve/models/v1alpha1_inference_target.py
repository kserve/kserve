# Copyright 2022 The KServe Authors.
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


class V1alpha1InferenceTarget(object):
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
        'node_name': 'str',
        'service_name': 'str',
        'service_url': 'str'
    }

    attribute_map = {
        'node_name': 'nodeName',
        'service_name': 'serviceName',
        'service_url': 'serviceUrl'
    }

    def __init__(self, node_name=None, service_name=None, service_url=None, local_vars_configuration=None):  # noqa: E501
        """V1alpha1InferenceTarget - a model defined in OpenAPI"""  # noqa: E501
        if local_vars_configuration is None:
            local_vars_configuration = Configuration()
        self.local_vars_configuration = local_vars_configuration

        self._node_name = None
        self._service_name = None
        self._service_url = None
        self.discriminator = None

        if node_name is not None:
            self.node_name = node_name
        if service_name is not None:
            self.service_name = service_name
        if service_url is not None:
            self.service_url = service_url

    @property
    def node_name(self):
        """Gets the node_name of this V1alpha1InferenceTarget.  # noqa: E501

        The node name for routing as next step  # noqa: E501

        :return: The node_name of this V1alpha1InferenceTarget.  # noqa: E501
        :rtype: str
        """
        return self._node_name

    @node_name.setter
    def node_name(self, node_name):
        """Sets the node_name of this V1alpha1InferenceTarget.

        The node name for routing as next step  # noqa: E501

        :param node_name: The node_name of this V1alpha1InferenceTarget.  # noqa: E501
        :type: str
        """

        self._node_name = node_name

    @property
    def service_name(self):
        """Gets the service_name of this V1alpha1InferenceTarget.  # noqa: E501

        named reference for InferenceService  # noqa: E501

        :return: The service_name of this V1alpha1InferenceTarget.  # noqa: E501
        :rtype: str
        """
        return self._service_name

    @service_name.setter
    def service_name(self, service_name):
        """Sets the service_name of this V1alpha1InferenceTarget.

        named reference for InferenceService  # noqa: E501

        :param service_name: The service_name of this V1alpha1InferenceTarget.  # noqa: E501
        :type: str
        """

        self._service_name = service_name

    @property
    def service_url(self):
        """Gets the service_url of this V1alpha1InferenceTarget.  # noqa: E501

        InferenceService URL, mutually exclusive with ServiceName  # noqa: E501

        :return: The service_url of this V1alpha1InferenceTarget.  # noqa: E501
        :rtype: str
        """
        return self._service_url

    @service_url.setter
    def service_url(self, service_url):
        """Sets the service_url of this V1alpha1InferenceTarget.

        InferenceService URL, mutually exclusive with ServiceName  # noqa: E501

        :param service_url: The service_url of this V1alpha1InferenceTarget.  # noqa: E501
        :type: str
        """

        self._service_url = service_url

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
        if not isinstance(other, V1alpha1InferenceTarget):
            return False

        return self.to_dict() == other.to_dict()

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        if not isinstance(other, V1alpha1InferenceTarget):
            return True

        return self.to_dict() != other.to_dict()
