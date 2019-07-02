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


class IoK8sApiCoreV1VolumeMount(object):
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
        'mount_path': 'str',
        'mount_propagation': 'str',
        'name': 'str',
        'read_only': 'bool',
        'sub_path': 'str'
    }

    attribute_map = {
        'mount_path': 'mountPath',
        'mount_propagation': 'mountPropagation',
        'name': 'name',
        'read_only': 'readOnly',
        'sub_path': 'subPath'
    }

    def __init__(self, mount_path=None, mount_propagation=None, name=None, read_only=None, sub_path=None):  # noqa: E501
        """IoK8sApiCoreV1VolumeMount - a model defined in Swagger"""  # noqa: E501

        self._mount_path = None
        self._mount_propagation = None
        self._name = None
        self._read_only = None
        self._sub_path = None
        self.discriminator = None

        self.mount_path = mount_path
        if mount_propagation is not None:
            self.mount_propagation = mount_propagation
        self.name = name
        if read_only is not None:
            self.read_only = read_only
        if sub_path is not None:
            self.sub_path = sub_path

    @property
    def mount_path(self):
        """Gets the mount_path of this IoK8sApiCoreV1VolumeMount.  # noqa: E501

        Path within the container at which the volume should be mounted.  Must not contain ':'.  # noqa: E501

        :return: The mount_path of this IoK8sApiCoreV1VolumeMount.  # noqa: E501
        :rtype: str
        """
        return self._mount_path

    @mount_path.setter
    def mount_path(self, mount_path):
        """Sets the mount_path of this IoK8sApiCoreV1VolumeMount.

        Path within the container at which the volume should be mounted.  Must not contain ':'.  # noqa: E501

        :param mount_path: The mount_path of this IoK8sApiCoreV1VolumeMount.  # noqa: E501
        :type: str
        """
        if mount_path is None:
            raise ValueError("Invalid value for `mount_path`, must not be `None`")  # noqa: E501

        self._mount_path = mount_path

    @property
    def mount_propagation(self):
        """Gets the mount_propagation of this IoK8sApiCoreV1VolumeMount.  # noqa: E501

        mountPropagation determines how mounts are propagated from the host to container and the other way around. When not set, MountPropagationNone is used. This field is beta in 1.10.  # noqa: E501

        :return: The mount_propagation of this IoK8sApiCoreV1VolumeMount.  # noqa: E501
        :rtype: str
        """
        return self._mount_propagation

    @mount_propagation.setter
    def mount_propagation(self, mount_propagation):
        """Sets the mount_propagation of this IoK8sApiCoreV1VolumeMount.

        mountPropagation determines how mounts are propagated from the host to container and the other way around. When not set, MountPropagationNone is used. This field is beta in 1.10.  # noqa: E501

        :param mount_propagation: The mount_propagation of this IoK8sApiCoreV1VolumeMount.  # noqa: E501
        :type: str
        """

        self._mount_propagation = mount_propagation

    @property
    def name(self):
        """Gets the name of this IoK8sApiCoreV1VolumeMount.  # noqa: E501

        This must match the Name of a Volume.  # noqa: E501

        :return: The name of this IoK8sApiCoreV1VolumeMount.  # noqa: E501
        :rtype: str
        """
        return self._name

    @name.setter
    def name(self, name):
        """Sets the name of this IoK8sApiCoreV1VolumeMount.

        This must match the Name of a Volume.  # noqa: E501

        :param name: The name of this IoK8sApiCoreV1VolumeMount.  # noqa: E501
        :type: str
        """
        if name is None:
            raise ValueError("Invalid value for `name`, must not be `None`")  # noqa: E501

        self._name = name

    @property
    def read_only(self):
        """Gets the read_only of this IoK8sApiCoreV1VolumeMount.  # noqa: E501

        Mounted read-only if true, read-write otherwise (false or unspecified). Defaults to false.  # noqa: E501

        :return: The read_only of this IoK8sApiCoreV1VolumeMount.  # noqa: E501
        :rtype: bool
        """
        return self._read_only

    @read_only.setter
    def read_only(self, read_only):
        """Sets the read_only of this IoK8sApiCoreV1VolumeMount.

        Mounted read-only if true, read-write otherwise (false or unspecified). Defaults to false.  # noqa: E501

        :param read_only: The read_only of this IoK8sApiCoreV1VolumeMount.  # noqa: E501
        :type: bool
        """

        self._read_only = read_only

    @property
    def sub_path(self):
        """Gets the sub_path of this IoK8sApiCoreV1VolumeMount.  # noqa: E501

        Path within the volume from which the container's volume should be mounted. Defaults to \"\" (volume's root).  # noqa: E501

        :return: The sub_path of this IoK8sApiCoreV1VolumeMount.  # noqa: E501
        :rtype: str
        """
        return self._sub_path

    @sub_path.setter
    def sub_path(self, sub_path):
        """Sets the sub_path of this IoK8sApiCoreV1VolumeMount.

        Path within the volume from which the container's volume should be mounted. Defaults to \"\" (volume's root).  # noqa: E501

        :param sub_path: The sub_path of this IoK8sApiCoreV1VolumeMount.  # noqa: E501
        :type: str
        """

        self._sub_path = sub_path

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
        if issubclass(IoK8sApiCoreV1VolumeMount, dict):
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
        if not isinstance(other, IoK8sApiCoreV1VolumeMount):
            return False

        return self.__dict__ == other.__dict__

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        return not self == other
