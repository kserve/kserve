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
from tabulate import tabulate

from ..constants import constants
from ..utils import utils


def isvc_watch(name=None, namespace=None, timeout_seconds=600, generation=0):
    """Watch the created or patched InferenceService in the specified namespace"""

    if namespace is None:
        namespace = utils.get_default_target_namespace()

    headers = ["NAME", "READY", "PREV", "LATEST", "URL"]
    table_fmt = "plain"

    stream = k8s_watch.Watch().stream(
        client.CustomObjectsApi().list_namespaced_custom_object,
        constants.KSERVE_GROUP,
        constants.KSERVE_V1BETA1_VERSION,
        namespace,
        constants.KSERVE_PLURAL_INFERENCESERVICE,
        timeout_seconds=timeout_seconds,
    )

    for event in stream:
        isvc = event["object"]
        isvc_name = isvc["metadata"]["name"]
        if name and name != isvc_name:
            continue
        else:
            status = "Unknown"
            if isvc.get("status", ""):
                url = isvc["status"].get("url", "")
                traffic = (
                    isvc["status"]
                    .get("components", {})
                    .get("predictor", {})
                    .get("traffic", [])
                )
                traffic_percent = 100
                if constants.OBSERVED_GENERATION in isvc["status"]:
                    observed_generation = isvc["status"][constants.OBSERVED_GENERATION]
                    for t in traffic:
                        if t["latestRevision"]:
                            traffic_percent = t["percent"]

                    if generation != 0 and observed_generation != generation:
                        continue
                    for condition in isvc["status"].get("conditions", {}):
                        if condition.get("type", "") == "Ready":
                            status = condition.get("status", "Unknown")
                    print(
                        tabulate(
                            [
                                [
                                    isvc_name,
                                    status,
                                    100 - traffic_percent,
                                    traffic_percent,
                                    url,
                                ]
                            ],
                            headers=headers,
                            tablefmt=table_fmt,
                        )
                    )
                    if status == "True":
                        break

            else:
                print(
                    tabulate(
                        [[isvc_name, status, "", "", ""]],
                        headers=headers,
                        tablefmt=table_fmt,
                    )
                )
                # Sleep 2 to avoid status section is not generated within a very short time.
                time.sleep(2)
                continue
