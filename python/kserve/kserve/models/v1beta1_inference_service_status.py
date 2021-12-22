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


class V1beta1InferenceServiceStatus(object):
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
        'address': 'KnativeAddressable',
        'annotations': 'dict(str, str)',
        'components': 'dict(str, V1beta1ComponentStatusSpec)',
        'conditions': 'list[KnativeCondition]',
        'observed_generation': 'int',
        'url': 'KnativeURL'
    }

    attribute_map = {
        'address': 'address',
        'annotations': 'annotations',
        'components': 'components',
        'conditions': 'conditions',
        'observed_generation': 'observedGeneration',
        'url': 'url'
    }

    def __init__(self, address=None, annotations=None, components=None, conditions=None, observed_generation=None, url=None, local_vars_configuration=None):  # noqa: E501
        """V1beta1InferenceServiceStatus - a model defined in OpenAPI"""  # noqa: E501
        if local_vars_configuration is None:
            local_vars_configuration = Configuration()
        self.local_vars_configuration = local_vars_configuration

        self._address = None
        self._annotations = None
        self._components = None
        self._conditions = None
        self._observed_generation = None
        self._url = None
        self.discriminator = None

        if address is not None:
            self.address = address
        if annotations is not None:
            self.annotations = annotations
        if components is not None:
            self.components = components
        if conditions is not None:
            self.conditions = conditions
        if observed_generation is not None:
            self.observed_generation = observed_generation
        if url is not None:
            self.url = url

    @property
    def address(self):
        """Gets the address of this V1beta1InferenceServiceStatus.  # noqa: E501


        :return: The address of this V1beta1InferenceServiceStatus.  # noqa: E501
        :rtype: KnativeAddressable
        """
        return self._address

    @address.setter
    def address(self, address):
        """Sets the address of this V1beta1InferenceServiceStatus.


        :param address: The address of this V1beta1InferenceServiceStatus.  # noqa: E501
        :type: KnativeAddressable
        """

        self._address = address

    @property
    def annotations(self):
        """Gets the annotations of this V1beta1InferenceServiceStatus.  # noqa: E501

        Annotations is additional Status fields for the Resource to save some additional State as well as convey more information to the user. This is roughly akin to Annotations on any k8s resource, just the reconciler conveying richer information outwards.  # noqa: E501

        :return: The annotations of this V1beta1InferenceServiceStatus.  # noqa: E501
        :rtype: dict(str, str)
        """
        return self._annotations

    @annotations.setter
    def annotations(self, annotations):
        """Sets the annotations of this V1beta1InferenceServiceStatus.

        Annotations is additional Status fields for the Resource to save some additional State as well as convey more information to the user. This is roughly akin to Annotations on any k8s resource, just the reconciler conveying richer information outwards.  # noqa: E501

        :param annotations: The annotations of this V1beta1InferenceServiceStatus.  # noqa: E501
        :type: dict(str, str)
        """

        self._annotations = annotations

    @property
    def components(self):
        """Gets the components of this V1beta1InferenceServiceStatus.  # noqa: E501

        Statuses for the components of the InferenceService  # noqa: E501

        :return: The components of this V1beta1InferenceServiceStatus.  # noqa: E501
        :rtype: dict(str, V1beta1ComponentStatusSpec)
        """
        return self._components

    @components.setter
    def components(self, components):
        """Sets the components of this V1beta1InferenceServiceStatus.

        Statuses for the components of the InferenceService  # noqa: E501

        :param components: The components of this V1beta1InferenceServiceStatus.  # noqa: E501
        :type: dict(str, V1beta1ComponentStatusSpec)
        """

        self._components = components

    @property
    def conditions(self):
        """Gets the conditions of this V1beta1InferenceServiceStatus.  # noqa: E501

        Conditions the latest available observations of a resource's current state.  # noqa: E501

        :return: The conditions of this V1beta1InferenceServiceStatus.  # noqa: E501
        :rtype: list[KnativeCondition]
        """
        return self._conditions

    @conditions.setter
    def conditions(self, conditions):
        """Sets the conditions of this V1beta1InferenceServiceStatus.

        Conditions the latest available observations of a resource's current state.  # noqa: E501

        :param conditions: The conditions of this V1beta1InferenceServiceStatus.  # noqa: E501
        :type: list[KnativeCondition]
        """

        self._conditions = conditions

    @property
    def observed_generation(self):
        """Gets the observed_generation of this V1beta1InferenceServiceStatus.  # noqa: E501

        ObservedGeneration is the 'Generation' of the Service that was last processed by the controller.  # noqa: E501

        :return: The observed_generation of this V1beta1InferenceServiceStatus.  # noqa: E501
        :rtype: int
        """
        return self._observed_generation

    @observed_generation.setter
    def observed_generation(self, observed_generation):
        """Sets the observed_generation of this V1beta1InferenceServiceStatus.

        ObservedGeneration is the 'Generation' of the Service that was last processed by the controller.  # noqa: E501

        :param observed_generation: The observed_generation of this V1beta1InferenceServiceStatus.  # noqa: E501
        :type: int
        """

        self._observed_generation = observed_generation

    @property
    def url(self):
        """Gets the url of this V1beta1InferenceServiceStatus.  # noqa: E501


        :return: The url of this V1beta1InferenceServiceStatus.  # noqa: E501
        :rtype: KnativeURL
        """
        return self._url

    @url.setter
    def url(self, url):
        """Sets the url of this V1beta1InferenceServiceStatus.


        :param url: The url of this V1beta1InferenceServiceStatus.  # noqa: E501
        :type: KnativeURL
        """

        self._url = url

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
        if not isinstance(other, V1beta1InferenceServiceStatus):
            return False

        return self.to_dict() == other.to_dict()

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        if not isinstance(other, V1beta1InferenceServiceStatus):
            return True

        return self.to_dict() != other.to_dict()
