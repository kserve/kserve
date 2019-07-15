# Copyright 2019 The Kubeflow Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os

from kubernetes import client, config, watch
from kfserving.constants import constants
from kfserving.utils import utils

class KFServingApi(object):
    """KFServing Apis."""

    def __init__(self):
        if utils.is_running_in_k8s():
            config.load_incluster_config()
        else:
            config.load_kube_config()

        self.api_instance = client.CustomObjectsApi()
    
    def deploy(self, kfservice, namespace=None):
        """Create the provided KFService in the specified namespace"""

        if namespace == None:
            namespace = utils.set_kfsvc_namespace(kfservice)

        try:
            return self.api_instance.create_namespaced_custom_object(
                constants.KFSERVING_GROUP,
                constants.KFSERVING_VERSION,
                namespace,
                constants.KFSERVING_PLURAL,
                kfservice)
        except client.rest.ApiException as e:
            raise RuntimeError("Exception when calling CustomObjectsApi->create_namespaced_custom_object: %s\n" % e)

    def get(self, name, namespace=None):
        """Get the deployed KFService in the specified namespace"""

        if namespace == None:
            namespace = utils.get_default_target_namespace()

        try:
            return self.api_instance.get_namespaced_custom_object(
                constants.KFSERVING_GROUP,
                constants.KFSERVING_VERSION,
                namespace,
                constants.KFSERVING_PLURAL,
                name)
        except client.rest.ApiException as e:
            raise RuntimeError("Exception when calling CustomObjectsApi->get_namespaced_custom_object: %s\n" % e)

    def patch(self, name, kfservice, namespace=None):
        """Patch the deployed KFService in the specified namespace"""

        if namespace == None:
            namespace = utils.set_kfsvc_namespace(kfservice)

        try:
            return self.api_instance.patch_namespaced_custom_object(
                constants.KFSERVING_GROUP,
                constants.KFSERVING_VERSION,
                namespace,
                constants.KFSERVING_PLURAL,
                name,
                kfservice)
        except client.rest.ApiException as e:
            raise RuntimeError("Exception when calling CustomObjectsApi->patch_namespaced_custom_object: %s\n" % e)

    def delete(self, name, namespace=None):
        """Delete the provided KFService in the specified namespace"""

        if namespace == None:
            namespace = utils.get_default_target_namespace()

        try:
            return self.api_instance.delete_namespaced_custom_object(
                constants.KFSERVING_GROUP,
                constants.KFSERVING_VERSION,
                namespace,
                constants.KFSERVING_PLURAL,
                name,
                client.V1DeleteOptions())
        except client.rest.ApiException as e:
            raise RuntimeError("Exception when calling CustomObjectsApi->delete_namespaced_custom_object: %s\n" % e)
