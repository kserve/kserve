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

import time
from kubernetes import client
from kubernetes import watch as k8s_watch
from table_logger import TableLogger

from ..constants import constants
from ..utils import utils


def isvc_watch(name=None, namespace=None, timeout_seconds=600):
    """Watch the created or patched InferenceService in the specified namespace"""

    if namespace is None:
        namespace = utils.get_default_target_namespace()

    tbl = TableLogger(
        columns='NAME,READY,PREV,LATEST,URL',
        colwidth={'NAME': 20, 'READY': 10, 'PREV': 25, 'LATEST': 25, 'URL': 65},
        border=False)

    stream = k8s_watch.Watch().stream(
        client.CustomObjectsApi().list_namespaced_custom_object,
        constants.KSERVE_GROUP,
        constants.KSERVE_V1BETA1_VERSION,
        namespace,
        constants.KSERVE_PLURAL,
        timeout_seconds=timeout_seconds)

    for event in stream:
        isvc = event['object']
        isvc_name = isvc['metadata']['name']
        if name and name != isvc_name:
            continue
        else:
            if isvc.get('status', ''):
                url = isvc['status'].get('url', '')
                traffic = isvc['status'].get('components', {}).get(
                    'predictor', {}).get('traffic', [])
                traffic_percent = 100
                for t in traffic:
                    if t["latestRevision"]:
                        traffic_percent = t["percent"]
                status = 'Unknown'
                for condition in isvc['status'].get('conditions', {}):
                    if condition.get('type', '') == 'Ready':
                        status = condition.get('status', 'Unknown')
                tbl(isvc_name, status, 100-traffic_percent, traffic_percent, url)
            else:
                tbl(isvc_name, 'Unknown', '', '', '')
                # Sleep 2 to avoid status section is not generated within a very short time.
                time.sleep(2)
                continue

            if name == isvc_name and status == 'True':
                break
