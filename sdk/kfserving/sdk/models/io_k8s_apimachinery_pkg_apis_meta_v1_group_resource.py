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


class IoK8sApimachineryPkgApisMetaV1GroupResource(object):
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
        'group': 'str',
        'resource': 'str'
    }

    attribute_map = {
        'group': 'group',
        'resource': 'resource'
    }

    def __init__(self, group=None, resource=None):  # noqa: E501
        """IoK8sApimachineryPkgApisMetaV1GroupResource - a model defined in Swagger"""  # noqa: E501

        self._group = None
        self._resource = None
        self.discriminator = None

        self.group = group
        self.resource = resource

    @property
    def group(self):
        """Gets the group of this IoK8sApimachineryPkgApisMetaV1GroupResource.  # noqa: E501


        :return: The group of this IoK8sApimachineryPkgApisMetaV1GroupResource.  # noqa: E501
        :rtype: str
        """
        return self._group

    @group.setter
    def group(self, group):
        """Sets the group of this IoK8sApimachineryPkgApisMetaV1GroupResource.


        :param group: The group of this IoK8sApimachineryPkgApisMetaV1GroupResource.  # noqa: E501
        :type: str
        """
        if group is None:
            raise ValueError("Invalid value for `group`, must not be `None`")  # noqa: E501

        self._group = group

    @property
    def resource(self):
        """Gets the resource of this IoK8sApimachineryPkgApisMetaV1GroupResource.  # noqa: E501


        :return: The resource of this IoK8sApimachineryPkgApisMetaV1GroupResource.  # noqa: E501
        :rtype: str
        """
        return self._resource

    @resource.setter
    def resource(self, resource):
        """Sets the resource of this IoK8sApimachineryPkgApisMetaV1GroupResource.


        :param resource: The resource of this IoK8sApimachineryPkgApisMetaV1GroupResource.  # noqa: E501
        :type: str
        """
        if resource is None:
            raise ValueError("Invalid value for `resource`, must not be `None`")  # noqa: E501

        self._resource = resource

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
        if issubclass(IoK8sApimachineryPkgApisMetaV1GroupResource, dict):
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
        if not isinstance(other, IoK8sApimachineryPkgApisMetaV1GroupResource):
            return False

        return self.__dict__ == other.__dict__

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        return not self == other
