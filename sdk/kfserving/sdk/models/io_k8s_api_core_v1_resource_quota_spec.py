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

from kfserving.sdk.models.io_k8s_api_core_v1_scope_selector import IoK8sApiCoreV1ScopeSelector  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_apimachinery_pkg_api_resource_quantity import IoK8sApimachineryPkgApiResourceQuantity  # noqa: F401,E501


class IoK8sApiCoreV1ResourceQuotaSpec(object):
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
        'hard': 'dict(str, IoK8sApimachineryPkgApiResourceQuantity)',
        'scope_selector': 'IoK8sApiCoreV1ScopeSelector',
        'scopes': 'list[str]'
    }

    attribute_map = {
        'hard': 'hard',
        'scope_selector': 'scopeSelector',
        'scopes': 'scopes'
    }

    def __init__(self, hard=None, scope_selector=None, scopes=None):  # noqa: E501
        """IoK8sApiCoreV1ResourceQuotaSpec - a model defined in Swagger"""  # noqa: E501

        self._hard = None
        self._scope_selector = None
        self._scopes = None
        self.discriminator = None

        if hard is not None:
            self.hard = hard
        if scope_selector is not None:
            self.scope_selector = scope_selector
        if scopes is not None:
            self.scopes = scopes

    @property
    def hard(self):
        """Gets the hard of this IoK8sApiCoreV1ResourceQuotaSpec.  # noqa: E501

        hard is the set of desired hard limits for each named resource. More info: https://kubernetes.io/docs/concepts/policy/resource-quotas/  # noqa: E501

        :return: The hard of this IoK8sApiCoreV1ResourceQuotaSpec.  # noqa: E501
        :rtype: dict(str, IoK8sApimachineryPkgApiResourceQuantity)
        """
        return self._hard

    @hard.setter
    def hard(self, hard):
        """Sets the hard of this IoK8sApiCoreV1ResourceQuotaSpec.

        hard is the set of desired hard limits for each named resource. More info: https://kubernetes.io/docs/concepts/policy/resource-quotas/  # noqa: E501

        :param hard: The hard of this IoK8sApiCoreV1ResourceQuotaSpec.  # noqa: E501
        :type: dict(str, IoK8sApimachineryPkgApiResourceQuantity)
        """

        self._hard = hard

    @property
    def scope_selector(self):
        """Gets the scope_selector of this IoK8sApiCoreV1ResourceQuotaSpec.  # noqa: E501

        scopeSelector is also a collection of filters like scopes that must match each object tracked by a quota but expressed using ScopeSelectorOperator in combination with possible values. For a resource to match, both scopes AND scopeSelector (if specified in spec), must be matched.  # noqa: E501

        :return: The scope_selector of this IoK8sApiCoreV1ResourceQuotaSpec.  # noqa: E501
        :rtype: IoK8sApiCoreV1ScopeSelector
        """
        return self._scope_selector

    @scope_selector.setter
    def scope_selector(self, scope_selector):
        """Sets the scope_selector of this IoK8sApiCoreV1ResourceQuotaSpec.

        scopeSelector is also a collection of filters like scopes that must match each object tracked by a quota but expressed using ScopeSelectorOperator in combination with possible values. For a resource to match, both scopes AND scopeSelector (if specified in spec), must be matched.  # noqa: E501

        :param scope_selector: The scope_selector of this IoK8sApiCoreV1ResourceQuotaSpec.  # noqa: E501
        :type: IoK8sApiCoreV1ScopeSelector
        """

        self._scope_selector = scope_selector

    @property
    def scopes(self):
        """Gets the scopes of this IoK8sApiCoreV1ResourceQuotaSpec.  # noqa: E501

        A collection of filters that must match each object tracked by a quota. If not specified, the quota matches all objects.  # noqa: E501

        :return: The scopes of this IoK8sApiCoreV1ResourceQuotaSpec.  # noqa: E501
        :rtype: list[str]
        """
        return self._scopes

    @scopes.setter
    def scopes(self, scopes):
        """Sets the scopes of this IoK8sApiCoreV1ResourceQuotaSpec.

        A collection of filters that must match each object tracked by a quota. If not specified, the quota matches all objects.  # noqa: E501

        :param scopes: The scopes of this IoK8sApiCoreV1ResourceQuotaSpec.  # noqa: E501
        :type: list[str]
        """

        self._scopes = scopes

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
        if issubclass(IoK8sApiCoreV1ResourceQuotaSpec, dict):
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
        if not isinstance(other, IoK8sApiCoreV1ResourceQuotaSpec):
            return False

        return self.__dict__ == other.__dict__

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        return not self == other
