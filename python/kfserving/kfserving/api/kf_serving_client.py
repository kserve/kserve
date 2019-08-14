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

from kubernetes import client, config

from ..constants import constants
from ..utils import utils
from .set_credentials import create_gcp_credentials, create_aws_credentials, set_service_account


class KFServingClient(object):
    '''KFServing Client Apis.'''

    def __init__(self, config_file=None, context=None,
                 client_configuration=None, persist_config=True):
        if utils.is_running_in_k8s():
            config.load_incluster_config()
        else:
            config.load_kube_config(
                config_file=config_file,
                context=context,
                client_configuration=client_configuration,
                persist_config=persist_config)

        self.api_instance = client.CustomObjectsApi()

    def create_creds(self, platform, namespace=None, **kwargs):
        '''
        Create GCP and AWS Credentials for KFServing.
        :param str platform: Valid value: GCP or AWS.
        :param str namespace: The kubenertes namespace.
        :return: str  The name of created service account.
        '''
        if namespace is None:
            namespace = utils.get_default_target_namespace()

        if platform.upper() == 'GCP':
            sa_name = create_gcp_credentials(namespace=namespace, **kwargs)
        elif platform.upper() == 'AWS':
            sa_name = create_aws_credentials(namespace=namespace, **kwargs)
        else:
            raise RuntimeError("Invalid platform: %s, only support GCP and AWS\
                currently.\n" % platform)
        
        return sa_name

    def create(self, kfservice, namespace=None):
        """Create the provided KFService in the specified namespace"""

        if namespace is None:
            namespace = utils.set_kfsvc_namespace(kfservice)

        try:
            return self.api_instance.create_namespaced_custom_object(
                constants.KFSERVING_GROUP,
                constants.KFSERVING_VERSION,
                namespace,
                constants.KFSERVING_PLURAL,
                kfservice)
        except client.rest.ApiException as e:
            raise RuntimeError(
                "Exception when calling CustomObjectsApi->create_namespaced_custom_object:\
                 %s\n" % e)

    def get(self, name, namespace=None):
        """Get the created KFService in the specified namespace"""

        if namespace is None:
            namespace = utils.get_default_target_namespace()

        try:
            return self.api_instance.get_namespaced_custom_object(
                constants.KFSERVING_GROUP,
                constants.KFSERVING_VERSION,
                namespace,
                constants.KFSERVING_PLURAL,
                name)
        except client.rest.ApiException as e:
            raise RuntimeError(
                "Exception when calling CustomObjectsApi->get_namespaced_custom_object: %s\n" % e)

    def patch(self, name, kfservice, namespace=None):
        """Patch the created KFService in the specified namespace"""

        if namespace is None:
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
            raise RuntimeError(
                "Exception when calling CustomObjectsApi->patch_namespaced_custom_object:\
                 %s\n" % e)

    def delete(self, name, namespace=None):
        """Delete the provided KFService in the specified namespace"""

        if namespace is None:
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
            raise RuntimeError(
                "Exception when calling CustomObjectsApi->delete_namespaced_custom_object:\
                 %s\n" % e)
