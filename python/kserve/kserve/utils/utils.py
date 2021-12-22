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

import os
import sys
import psutil
from typing import Dict, Union
from cloudevents.http import CloudEvent, to_binary, to_structured


def is_running_in_k8s():
    return os.path.isdir('/var/run/secrets/kubernetes.io/')


def get_current_k8s_namespace():
    with open('/var/run/secrets/kubernetes.io/serviceaccount/namespace', 'r') as f:
        return f.readline()


def get_default_target_namespace():
    if not is_running_in_k8s():
        return 'default'
    return get_current_k8s_namespace()


def set_isvc_namespace(inferenceservice):
    isvc_namespace = inferenceservice.metadata.namespace
    namespace = isvc_namespace or get_default_target_namespace()
    return namespace


def cpu_count():
    """Get the available CPU count for this system.
    Takes the minimum value from the following locations:
    - Total system cpus available on the host.
    - CPU Affinity (if set)
    - Cgroups limit (if set)
    """
    count = os.cpu_count()

    # Check CPU affinity if available
    try:
        affinity_count = len(psutil.Process().cpu_affinity())
        if affinity_count > 0:
            count = min(count, affinity_count)
    except Exception:
        pass

    # Check cgroups if available
    if sys.platform == "linux":
        try:
            with open("/sys/fs/cgroup/cpu,cpuacct/cpu.cfs_quota_us") as f:
                quota = int(f.read())
            with open("/sys/fs/cgroup/cpu,cpuacct/cpu.cfs_period_us") as f:
                period = int(f.read())
            cgroups_count = int(quota / period)
            if cgroups_count > 0:
                count = min(count, cgroups_count)
        except Exception:
            pass

    return count


def is_structured_cloudevent(body: Dict) -> bool:
    """Returns True if the JSON request body resembles a structured CloudEvent"""
    return "time" in body \
           and "type" in body \
           and "source" in body \
           and "id" in body \
           and "specversion" in body \
           and "data" in body


def create_response_cloudevent(model_name: str, body: Union[Dict, CloudEvent], response: Dict,
                               binary_event=False) -> tuple:
    ce_attributes = {}

    if os.getenv("CE_MERGE", "false").lower() == "true":
        if binary_event:
            ce_attributes = body._attributes
            if "datacontenttype" in ce_attributes:  # Optional field so must check
                del ce_attributes["datacontenttype"]
        else:
            ce_attributes = body
            del ce_attributes["data"]

        # Remove these fields so we generate new ones
        del ce_attributes["id"]
        del ce_attributes["time"]

    ce_attributes["type"] = os.getenv("CE_TYPE", "io.kserve.inference.response")
    ce_attributes["source"] = os.getenv("CE_SOURCE", f"io.kserve.kfserver.{model_name}")

    event = CloudEvent(ce_attributes, response)

    if binary_event:
        event_headers, event_body = to_binary(event)
    else:
        event_headers, event_body = to_structured(event)

    return event_headers, event_body
