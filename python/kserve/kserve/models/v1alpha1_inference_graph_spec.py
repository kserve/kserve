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


class V1alpha1InferenceGraphSpec(object):
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
        'affinity': 'V1Affinity',
        'image_pull_secrets': 'list[V1LocalObjectReference]',
        'max_replicas': 'int',
        'min_replicas': 'int',
        'node_name': 'str',
        'node_selector': 'dict(str, str)',
        'nodes': 'dict(str, V1alpha1InferenceRouter)',
        'resources': 'V1ResourceRequirements',
        'scale_metric': 'str',
        'scale_target': 'int',
        'service_account_name': 'str',
        'timeout': 'int',
        'tolerations': 'list[V1Toleration]'
    }

    attribute_map = {
        'affinity': 'affinity',
        'image_pull_secrets': 'imagePullSecrets',
        'max_replicas': 'maxReplicas',
        'min_replicas': 'minReplicas',
        'node_name': 'nodeName',
        'node_selector': 'nodeSelector',
        'nodes': 'nodes',
        'resources': 'resources',
        'scale_metric': 'scaleMetric',
        'scale_target': 'scaleTarget',
        'service_account_name': 'serviceAccountName',
        'timeout': 'timeout',
        'tolerations': 'tolerations'
    }

    def __init__(self, affinity=None, image_pull_secrets=None, max_replicas=None, min_replicas=None, node_name=None, node_selector=None, nodes=None, resources=None, scale_metric=None, scale_target=None, service_account_name=None, timeout=None, tolerations=None, local_vars_configuration=None):  # noqa: E501
        """V1alpha1InferenceGraphSpec - a model defined in OpenAPI"""  # noqa: E501
        if local_vars_configuration is None:
            local_vars_configuration = Configuration()
        self.local_vars_configuration = local_vars_configuration

        self._affinity = None
        self._image_pull_secrets = None
        self._max_replicas = None
        self._min_replicas = None
        self._node_name = None
        self._node_selector = None
        self._nodes = None
        self._resources = None
        self._scale_metric = None
        self._scale_target = None
        self._service_account_name = None
        self._timeout = None
        self._tolerations = None
        self.discriminator = None

        if affinity is not None:
            self.affinity = affinity
        if image_pull_secrets is not None:
            self.image_pull_secrets = image_pull_secrets
        if max_replicas is not None:
            self.max_replicas = max_replicas
        if min_replicas is not None:
            self.min_replicas = min_replicas
        if node_name is not None:
            self.node_name = node_name
        if node_selector is not None:
            self.node_selector = node_selector
        self.nodes = nodes
        if resources is not None:
            self.resources = resources
        if scale_metric is not None:
            self.scale_metric = scale_metric
        if scale_target is not None:
            self.scale_target = scale_target
        if service_account_name is not None:
            self.service_account_name = service_account_name
        if timeout is not None:
            self.timeout = timeout
        if tolerations is not None:
            self.tolerations = tolerations

    @property
    def affinity(self):
        """Gets the affinity of this V1alpha1InferenceGraphSpec.  # noqa: E501


        :return: The affinity of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: V1Affinity
        """
        return self._affinity

    @affinity.setter
    def affinity(self, affinity):
        """Sets the affinity of this V1alpha1InferenceGraphSpec.


        :param affinity: The affinity of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: V1Affinity
        """

        self._affinity = affinity

    @property
    def image_pull_secrets(self):
        """Gets the image_pull_secrets of this V1alpha1InferenceGraphSpec.  # noqa: E501

        ImagePullSecrets specifies the image pull secrets for the InferenceGraph. https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/  # noqa: E501

        :return: The image_pull_secrets of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: list[V1LocalObjectReference]
        """
        return self._image_pull_secrets

    @image_pull_secrets.setter
    def image_pull_secrets(self, image_pull_secrets):
        """Sets the image_pull_secrets of this V1alpha1InferenceGraphSpec.

        ImagePullSecrets specifies the image pull secrets for the InferenceGraph. https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/  # noqa: E501

        :param image_pull_secrets: The image_pull_secrets of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: list[V1LocalObjectReference]
        """

        self._image_pull_secrets = image_pull_secrets

    @property
    def max_replicas(self):
        """Gets the max_replicas of this V1alpha1InferenceGraphSpec.  # noqa: E501

        Maximum number of replicas for autoscaling.  # noqa: E501

        :return: The max_replicas of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: int
        """
        return self._max_replicas

    @max_replicas.setter
    def max_replicas(self, max_replicas):
        """Sets the max_replicas of this V1alpha1InferenceGraphSpec.

        Maximum number of replicas for autoscaling.  # noqa: E501

        :param max_replicas: The max_replicas of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: int
        """

        self._max_replicas = max_replicas

    @property
    def min_replicas(self):
        """Gets the min_replicas of this V1alpha1InferenceGraphSpec.  # noqa: E501

        Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero.  # noqa: E501

        :return: The min_replicas of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: int
        """
        return self._min_replicas

    @min_replicas.setter
    def min_replicas(self, min_replicas):
        """Sets the min_replicas of this V1alpha1InferenceGraphSpec.

        Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero.  # noqa: E501

        :param min_replicas: The min_replicas of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: int
        """

        self._min_replicas = min_replicas

    @property
    def node_name(self):
        """Gets the node_name of this V1alpha1InferenceGraphSpec.  # noqa: E501

        NodeName specifies the node name for the InferenceGraph. https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/  # noqa: E501

        :return: The node_name of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: str
        """
        return self._node_name

    @node_name.setter
    def node_name(self, node_name):
        """Sets the node_name of this V1alpha1InferenceGraphSpec.

        NodeName specifies the node name for the InferenceGraph. https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/  # noqa: E501

        :param node_name: The node_name of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: str
        """

        self._node_name = node_name

    @property
    def node_selector(self):
        """Gets the node_selector of this V1alpha1InferenceGraphSpec.  # noqa: E501

        NodeSelector specifies the node selector for the InferenceGraph. https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/  # noqa: E501

        :return: The node_selector of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: dict(str, str)
        """
        return self._node_selector

    @node_selector.setter
    def node_selector(self, node_selector):
        """Sets the node_selector of this V1alpha1InferenceGraphSpec.

        NodeSelector specifies the node selector for the InferenceGraph. https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/  # noqa: E501

        :param node_selector: The node_selector of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: dict(str, str)
        """

        self._node_selector = node_selector

    @property
    def nodes(self):
        """Gets the nodes of this V1alpha1InferenceGraphSpec.  # noqa: E501

        Map of InferenceGraph router nodes Each node defines the router which can be different routing types  # noqa: E501

        :return: The nodes of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: dict(str, V1alpha1InferenceRouter)
        """
        return self._nodes

    @nodes.setter
    def nodes(self, nodes):
        """Sets the nodes of this V1alpha1InferenceGraphSpec.

        Map of InferenceGraph router nodes Each node defines the router which can be different routing types  # noqa: E501

        :param nodes: The nodes of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: dict(str, V1alpha1InferenceRouter)
        """
        if self.local_vars_configuration.client_side_validation and nodes is None:  # noqa: E501
            raise ValueError("Invalid value for `nodes`, must not be `None`")  # noqa: E501

        self._nodes = nodes

    @property
    def resources(self):
        """Gets the resources of this V1alpha1InferenceGraphSpec.  # noqa: E501


        :return: The resources of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: V1ResourceRequirements
        """
        return self._resources

    @resources.setter
    def resources(self, resources):
        """Sets the resources of this V1alpha1InferenceGraphSpec.


        :param resources: The resources of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: V1ResourceRequirements
        """

        self._resources = resources

    @property
    def scale_metric(self):
        """Gets the scale_metric of this V1alpha1InferenceGraphSpec.  # noqa: E501

        ScaleMetric defines the scaling metric type watched by autoscaler possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics).  # noqa: E501

        :return: The scale_metric of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: str
        """
        return self._scale_metric

    @scale_metric.setter
    def scale_metric(self, scale_metric):
        """Sets the scale_metric of this V1alpha1InferenceGraphSpec.

        ScaleMetric defines the scaling metric type watched by autoscaler possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics).  # noqa: E501

        :param scale_metric: The scale_metric of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: str
        """

        self._scale_metric = scale_metric

    @property
    def scale_target(self):
        """Gets the scale_target of this V1alpha1InferenceGraphSpec.  # noqa: E501

        ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for. concurrency and rps targets are supported by Knative Pod Autoscaler (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/).  # noqa: E501

        :return: The scale_target of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: int
        """
        return self._scale_target

    @scale_target.setter
    def scale_target(self, scale_target):
        """Sets the scale_target of this V1alpha1InferenceGraphSpec.

        ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for. concurrency and rps targets are supported by Knative Pod Autoscaler (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/).  # noqa: E501

        :param scale_target: The scale_target of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: int
        """

        self._scale_target = scale_target

    @property
    def service_account_name(self):
        """Gets the service_account_name of this V1alpha1InferenceGraphSpec.  # noqa: E501

        ServiceAccountName specifies the service account name for the InferenceGraph. https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/  # noqa: E501

        :return: The service_account_name of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: str
        """
        return self._service_account_name

    @service_account_name.setter
    def service_account_name(self, service_account_name):
        """Sets the service_account_name of this V1alpha1InferenceGraphSpec.

        ServiceAccountName specifies the service account name for the InferenceGraph. https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/  # noqa: E501

        :param service_account_name: The service_account_name of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: str
        """

        self._service_account_name = service_account_name

    @property
    def timeout(self):
        """Gets the timeout of this V1alpha1InferenceGraphSpec.  # noqa: E501

        TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component.  # noqa: E501

        :return: The timeout of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: int
        """
        return self._timeout

    @timeout.setter
    def timeout(self, timeout):
        """Sets the timeout of this V1alpha1InferenceGraphSpec.

        TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component.  # noqa: E501

        :param timeout: The timeout of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: int
        """

        self._timeout = timeout

    @property
    def tolerations(self):
        """Gets the tolerations of this V1alpha1InferenceGraphSpec.  # noqa: E501

        Toleration specifies the toleration for the InferenceGraph. https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/  # noqa: E501

        :return: The tolerations of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :rtype: list[V1Toleration]
        """
        return self._tolerations

    @tolerations.setter
    def tolerations(self, tolerations):
        """Sets the tolerations of this V1alpha1InferenceGraphSpec.

        Toleration specifies the toleration for the InferenceGraph. https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/  # noqa: E501

        :param tolerations: The tolerations of this V1alpha1InferenceGraphSpec.  # noqa: E501
        :type: list[V1Toleration]
        """

        self._tolerations = tolerations

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
        if not isinstance(other, V1alpha1InferenceGraphSpec):
            return False

        return self.to_dict() == other.to_dict()

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        if not isinstance(other, V1alpha1InferenceGraphSpec):
            return True

        return self.to_dict() != other.to_dict()
