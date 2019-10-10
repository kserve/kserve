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

from kubernetes import client
from kubernetes import watch as k8s_watch
from table_logger import TableLogger

from ..constants import constants
from ..utils import utils


def watch(name=None, namespace=None, timeout_seconds=600):
    """Watch the created or patched InferenceService in the specified namespace"""

    if namespace is None:
        namespace = utils.get_default_target_namespace()

    tbl = TableLogger(
        columns='NAME,READY,DEFAULT_TRAFFIC,CANARY_TRAFFIC,URL',
        colwidth={'NAME': 20, 'READY':10, 'DEFAULT_TRAFFIC':15, 'CANARY_TRAFFIC':15, 'URL': 50},
        border=False)

    stream = k8s_watch.Watch().stream(
        client.CustomObjectsApi().list_namespaced_custom_object,
        constants.KFSERVING_GROUP,
        constants.KFSERVING_VERSION,
        namespace,
        constants.KFSERVING_PLURAL,
        timeout_seconds=timeout_seconds)

    for event in stream:
        kfserivce = event['object']
        isvc_name = kfserivce['metadata']['name']
        if name and name != isvc_name:
            continue
        else:
            url = kfserivce['status'].get('url', '')
            default_traffic = kfserivce['status'].get('traffic', '')
            canary_traffic = kfserivce['status'].get('canaryTraffic', '')
            status = 'Unknown'
            for condition in kfserivce['status'].get('conditions', {}):
                if condition.get('type', '') == 'Ready':
                    status = condition.get('status', 'Unknown')
            tbl(isvc_name, status, default_traffic, canary_traffic, url)

            if name == isvc_name and status == 'True':
                break
