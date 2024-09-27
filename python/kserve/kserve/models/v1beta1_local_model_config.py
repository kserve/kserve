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


class V1beta1LocalModelConfig(object):
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
        'default_job_image': 'str',
        'fs_group': 'int',
        'job_namespace': 'str'
    }

    attribute_map = {
        'default_job_image': 'defaultJobImage',
        'fs_group': 'fsGroup',
        'job_namespace': 'jobNamespace'
    }

    def __init__(self, default_job_image=None, fs_group=None, job_namespace='', local_vars_configuration=None):  # noqa: E501
        """V1beta1LocalModelConfig - a model defined in OpenAPI"""  # noqa: E501
        if local_vars_configuration is None:
            local_vars_configuration = Configuration()
        self.local_vars_configuration = local_vars_configuration

        self._default_job_image = None
        self._fs_group = None
        self._job_namespace = None
        self.discriminator = None

        if default_job_image is not None:
            self.default_job_image = default_job_image
        if fs_group is not None:
            self.fs_group = fs_group
        self.job_namespace = job_namespace

    @property
    def default_job_image(self):
        """Gets the default_job_image of this V1beta1LocalModelConfig.  # noqa: E501


        :return: The default_job_image of this V1beta1LocalModelConfig.  # noqa: E501
        :rtype: str
        """
        return self._default_job_image

    @default_job_image.setter
    def default_job_image(self, default_job_image):
        """Sets the default_job_image of this V1beta1LocalModelConfig.


        :param default_job_image: The default_job_image of this V1beta1LocalModelConfig.  # noqa: E501
        :type: str
        """

        self._default_job_image = default_job_image

    @property
    def fs_group(self):
        """Gets the fs_group of this V1beta1LocalModelConfig.  # noqa: E501


        :return: The fs_group of this V1beta1LocalModelConfig.  # noqa: E501
        :rtype: int
        """
        return self._fs_group

    @fs_group.setter
    def fs_group(self, fs_group):
        """Sets the fs_group of this V1beta1LocalModelConfig.


        :param fs_group: The fs_group of this V1beta1LocalModelConfig.  # noqa: E501
        :type: int
        """

        self._fs_group = fs_group

    @property
    def job_namespace(self):
        """Gets the job_namespace of this V1beta1LocalModelConfig.  # noqa: E501


        :return: The job_namespace of this V1beta1LocalModelConfig.  # noqa: E501
        :rtype: str
        """
        return self._job_namespace

    @job_namespace.setter
    def job_namespace(self, job_namespace):
        """Sets the job_namespace of this V1beta1LocalModelConfig.


        :param job_namespace: The job_namespace of this V1beta1LocalModelConfig.  # noqa: E501
        :type: str
        """
        if self.local_vars_configuration.client_side_validation and job_namespace is None:  # noqa: E501
            raise ValueError("Invalid value for `job_namespace`, must not be `None`")  # noqa: E501

        self._job_namespace = job_namespace

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
        if not isinstance(other, V1beta1LocalModelConfig):
            return False

        return self.to_dict() == other.to_dict()

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        if not isinstance(other, V1beta1LocalModelConfig):
            return True

        return self.to_dict() != other.to_dict()
