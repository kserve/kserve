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


class V1beta1MetricsSpec(object):
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
        'external': 'V1beta1ExternalMetricSource',
        'podmetric': 'V1beta1PodMetricSource',
        'resource': 'V1beta1ResourceMetricSource',
        'type': 'str'
    }

    attribute_map = {
        'external': 'external',
        'podmetric': 'podmetric',
        'resource': 'resource',
        'type': 'type'
    }

    def __init__(self, external=None, podmetric=None, resource=None, type='', local_vars_configuration=None):  # noqa: E501
        """V1beta1MetricsSpec - a model defined in OpenAPI"""  # noqa: E501
        if local_vars_configuration is None:
            local_vars_configuration = Configuration()
        self.local_vars_configuration = local_vars_configuration

        self._external = None
        self._podmetric = None
        self._resource = None
        self._type = None
        self.discriminator = None

        if external is not None:
            self.external = external
        if podmetric is not None:
            self.podmetric = podmetric
        if resource is not None:
            self.resource = resource
        self.type = type

    @property
    def external(self):
        """Gets the external of this V1beta1MetricsSpec.  # noqa: E501


        :return: The external of this V1beta1MetricsSpec.  # noqa: E501
        :rtype: V1beta1ExternalMetricSource
        """
        return self._external

    @external.setter
    def external(self, external):
        """Sets the external of this V1beta1MetricsSpec.


        :param external: The external of this V1beta1MetricsSpec.  # noqa: E501
        :type: V1beta1ExternalMetricSource
        """

        self._external = external

    @property
    def podmetric(self):
        """Gets the podmetric of this V1beta1MetricsSpec.  # noqa: E501


        :return: The podmetric of this V1beta1MetricsSpec.  # noqa: E501
        :rtype: V1beta1PodMetricSource
        """
        return self._podmetric

    @podmetric.setter
    def podmetric(self, podmetric):
        """Sets the podmetric of this V1beta1MetricsSpec.


        :param podmetric: The podmetric of this V1beta1MetricsSpec.  # noqa: E501
        :type: V1beta1PodMetricSource
        """

        self._podmetric = podmetric

    @property
    def resource(self):
        """Gets the resource of this V1beta1MetricsSpec.  # noqa: E501


        :return: The resource of this V1beta1MetricsSpec.  # noqa: E501
        :rtype: V1beta1ResourceMetricSource
        """
        return self._resource

    @resource.setter
    def resource(self, resource):
        """Sets the resource of this V1beta1MetricsSpec.


        :param resource: The resource of this V1beta1MetricsSpec.  # noqa: E501
        :type: V1beta1ResourceMetricSource
        """

        self._resource = resource

    @property
    def type(self):
        """Gets the type of this V1beta1MetricsSpec.  # noqa: E501

        type is the type of metric source.  It should be one of \"Resource\", \"External\", \"PodMetric\". \"Resource\" or \"External\" each mapping to a matching field in the object.  # noqa: E501

        :return: The type of this V1beta1MetricsSpec.  # noqa: E501
        :rtype: str
        """
        return self._type

    @type.setter
    def type(self, type):
        """Sets the type of this V1beta1MetricsSpec.

        type is the type of metric source.  It should be one of \"Resource\", \"External\", \"PodMetric\". \"Resource\" or \"External\" each mapping to a matching field in the object.  # noqa: E501

        :param type: The type of this V1beta1MetricsSpec.  # noqa: E501
        :type: str
        """
        if self.local_vars_configuration.client_side_validation and type is None:  # noqa: E501
            raise ValueError("Invalid value for `type`, must not be `None`")  # noqa: E501

        self._type = type

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
        if not isinstance(other, V1beta1MetricsSpec):
            return False

        return self.to_dict() == other.to_dict()

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        if not isinstance(other, V1beta1MetricsSpec):
            return True

        return self.to_dict() != other.to_dict()
