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

import sys
from kubernetes import client, config
from kubernetes import watch as k8s_watch
from table_logger import TableLogger

from ..constants import constants
from ..utils import utils
from .creds_utils import set_gcs_credentials, set_s3_credentials


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

    def set_credentials(self, storage_type, namespace=None, credentials_file=None,
                        service_account=constants.DEFAULT_SA_NAME, **kwargs):
        '''
        Set GCS and S3 Credentials for KFServing.
        Args:
            storage_type(str): Valid value: GCS or S3 (required).
            namespace(str): The kubenertes namespace (Optional).
            credentials_file(str): The path for the credentials file.
            service_account(str): The name of service account.
            kwargs(dict): Others parameters for each storage_type.
        '''
        if namespace is None:
            namespace = utils.get_default_target_namespace()

        if storage_type.lower() == 'gcs':
            if credentials_file is None:
                credentials_file = constants.GCS_DEFAULT_CREDS_FILE
            set_gcs_credentials(namespace=namespace,
                                credentials_file=credentials_file,
                                service_account=service_account)
        elif storage_type.lower() == 's3':
            if credentials_file is None:
                credentials_file = constants.S3_DEFAULT_CREDS_FILE
            set_s3_credentials(namespace=namespace,
                               credentials_file=credentials_file,
                               service_account=service_account,
                               **kwargs)
        else:
            raise RuntimeError("Invalid storage_type: %s, only support GCS and S3\
                currently.\n" % storage_type)


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

    def watch(self, name=None, namespace=None, timeout_seconds=900):
        """Watch the created KFService in the specified namespace"""

        if namespace is None:
            namespace = utils.get_default_target_namespace()

        tbl = TableLogger(
            columns='NAME,READY,DEFAULT TRAFFIC,CANARY TRAFFIC,URL',
            colwidth={'NAME': 20, 'READY':7, 'DEFAULT TRAFFIC':15, 'CANARY TRAFFIC':14, 'URL': 45},
            file=sys.stdout)

        stream = k8s_watch.Watch().stream(
            client.CustomObjectsApi().list_namespaced_custom_object,
            constants.KFSERVING_GROUP,
            constants.KFSERVING_VERSION,
            namespace,
            constants.KFSERVING_PLURAL,
            timeout_seconds=timeout_seconds)
        for event in stream:
            kfserivce = event['object']

            kfsvc_name = kfserivce['metadata']['name']
            if name and name != kfsvc_name:
                continue
            else:
                url = kfserivce['status'].get('url', 'Unknown')
                default_traffic = kfserivce['status'].get('default', {}).get('traffic', '')
                canary_traffic = kfserivce['status'].get('canary', {}).get('traffic', '')
                status = 'Unknown'
                for condition in kfserivce['status'].get('conditions', {}):
                    if condition.get('type', '') == 'Ready':
                        status = condition.get('status', 'Unknown')

                tbl(kfsvc_name, status, default_traffic, canary_traffic, url)

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
