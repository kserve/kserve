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

    The version of the OpenAPI document: v0.1
    Generated by: https://openapi-generator.tech
"""


import pprint
import re  # noqa: F401

import six

from kfserving.configuration import Configuration


class V1beta1InferenceServicesConfig(object):
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
        'explainers': 'V1beta1ExplainersConfig',
        'predictors': 'V1beta1PredictorsConfig',
        'transformers': 'V1beta1TransformersConfig'
    }

    attribute_map = {
        'explainers': 'explainers',
        'predictors': 'predictors',
        'transformers': 'transformers'
    }

    def __init__(self, explainers=None, predictors=None, transformers=None, local_vars_configuration=None):  # noqa: E501
        """V1beta1InferenceServicesConfig - a model defined in OpenAPI"""  # noqa: E501
        if local_vars_configuration is None:
            local_vars_configuration = Configuration()
        self.local_vars_configuration = local_vars_configuration

        self._explainers = None
        self._predictors = None
        self._transformers = None
        self.discriminator = None

        self.explainers = explainers
        self.predictors = predictors
        self.transformers = transformers

    @property
    def explainers(self):
        """Gets the explainers of this V1beta1InferenceServicesConfig.  # noqa: E501


        :return: The explainers of this V1beta1InferenceServicesConfig.  # noqa: E501
        :rtype: V1beta1ExplainersConfig
        """
        return self._explainers

    @explainers.setter
    def explainers(self, explainers):
        """Sets the explainers of this V1beta1InferenceServicesConfig.


        :param explainers: The explainers of this V1beta1InferenceServicesConfig.  # noqa: E501
        :type: V1beta1ExplainersConfig
        """
        if self.local_vars_configuration.client_side_validation and explainers is None:  # noqa: E501
            raise ValueError("Invalid value for `explainers`, must not be `None`")  # noqa: E501

        self._explainers = explainers

    @property
    def predictors(self):
        """Gets the predictors of this V1beta1InferenceServicesConfig.  # noqa: E501


        :return: The predictors of this V1beta1InferenceServicesConfig.  # noqa: E501
        :rtype: V1beta1PredictorsConfig
        """
        return self._predictors

    @predictors.setter
    def predictors(self, predictors):
        """Sets the predictors of this V1beta1InferenceServicesConfig.


        :param predictors: The predictors of this V1beta1InferenceServicesConfig.  # noqa: E501
        :type: V1beta1PredictorsConfig
        """
        if self.local_vars_configuration.client_side_validation and predictors is None:  # noqa: E501
            raise ValueError("Invalid value for `predictors`, must not be `None`")  # noqa: E501

        self._predictors = predictors

    @property
    def transformers(self):
        """Gets the transformers of this V1beta1InferenceServicesConfig.  # noqa: E501


        :return: The transformers of this V1beta1InferenceServicesConfig.  # noqa: E501
        :rtype: V1beta1TransformersConfig
        """
        return self._transformers

    @transformers.setter
    def transformers(self, transformers):
        """Sets the transformers of this V1beta1InferenceServicesConfig.


        :param transformers: The transformers of this V1beta1InferenceServicesConfig.  # noqa: E501
        :type: V1beta1TransformersConfig
        """
        if self.local_vars_configuration.client_side_validation and transformers is None:  # noqa: E501
            raise ValueError("Invalid value for `transformers`, must not be `None`")  # noqa: E501

        self._transformers = transformers

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
        if not isinstance(other, V1beta1InferenceServicesConfig):
            return False

        return self.to_dict() == other.to_dict()

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        if not isinstance(other, V1beta1InferenceServicesConfig):
            return True

        return self.to_dict() != other.to_dict()
