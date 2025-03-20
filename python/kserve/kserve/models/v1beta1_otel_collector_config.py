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


class V1beta1OtelCollectorConfig(object):
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
        'otel_receiver_endpoint': 'str',
        'otel_scaler_endpoint': 'str',
        'scrape_interval': 'str'
    }

    attribute_map = {
        'otel_receiver_endpoint': 'otelReceiverEndpoint',
        'otel_scaler_endpoint': 'otelScalerEndpoint',
        'scrape_interval': 'scrapeInterval'
    }

    def __init__(self, otel_receiver_endpoint=None, otel_scaler_endpoint=None, scrape_interval=None, local_vars_configuration=None):  # noqa: E501
        """V1beta1OtelCollectorConfig - a model defined in OpenAPI"""  # noqa: E501
        if local_vars_configuration is None:
            local_vars_configuration = Configuration()
        self.local_vars_configuration = local_vars_configuration

        self._otel_receiver_endpoint = None
        self._otel_scaler_endpoint = None
        self._scrape_interval = None
        self.discriminator = None

        if otel_receiver_endpoint is not None:
            self.otel_receiver_endpoint = otel_receiver_endpoint
        if otel_scaler_endpoint is not None:
            self.otel_scaler_endpoint = otel_scaler_endpoint
        if scrape_interval is not None:
            self.scrape_interval = scrape_interval

    @property
    def otel_receiver_endpoint(self):
        """Gets the otel_receiver_endpoint of this V1beta1OtelCollectorConfig.  # noqa: E501


        :return: The otel_receiver_endpoint of this V1beta1OtelCollectorConfig.  # noqa: E501
        :rtype: str
        """
        return self._otel_receiver_endpoint

    @otel_receiver_endpoint.setter
    def otel_receiver_endpoint(self, otel_receiver_endpoint):
        """Sets the otel_receiver_endpoint of this V1beta1OtelCollectorConfig.


        :param otel_receiver_endpoint: The otel_receiver_endpoint of this V1beta1OtelCollectorConfig.  # noqa: E501
        :type: str
        """

        self._otel_receiver_endpoint = otel_receiver_endpoint

    @property
    def otel_scaler_endpoint(self):
        """Gets the otel_scaler_endpoint of this V1beta1OtelCollectorConfig.  # noqa: E501


        :return: The otel_scaler_endpoint of this V1beta1OtelCollectorConfig.  # noqa: E501
        :rtype: str
        """
        return self._otel_scaler_endpoint

    @otel_scaler_endpoint.setter
    def otel_scaler_endpoint(self, otel_scaler_endpoint):
        """Sets the otel_scaler_endpoint of this V1beta1OtelCollectorConfig.


        :param otel_scaler_endpoint: The otel_scaler_endpoint of this V1beta1OtelCollectorConfig.  # noqa: E501
        :type: str
        """

        self._otel_scaler_endpoint = otel_scaler_endpoint

    @property
    def scrape_interval(self):
        """Gets the scrape_interval of this V1beta1OtelCollectorConfig.  # noqa: E501


        :return: The scrape_interval of this V1beta1OtelCollectorConfig.  # noqa: E501
        :rtype: str
        """
        return self._scrape_interval

    @scrape_interval.setter
    def scrape_interval(self, scrape_interval):
        """Sets the scrape_interval of this V1beta1OtelCollectorConfig.


        :param scrape_interval: The scrape_interval of this V1beta1OtelCollectorConfig.  # noqa: E501
        :type: str
        """

        self._scrape_interval = scrape_interval

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
        if not isinstance(other, V1beta1OtelCollectorConfig):
            return False

        return self.to_dict() == other.to_dict()

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        if not isinstance(other, V1beta1OtelCollectorConfig):
            return True

        return self.to_dict() != other.to_dict()
