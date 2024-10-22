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
import uuid

from kserve.protocol.grpc.grpc_predict_v2_pb2 import InferParameter
from typing import Dict, Union, List

from kserve.utils.numpy_codec import from_np_dtype
import pandas as pd
import numpy as np
import psutil
from cloudevents.conversion import to_binary, to_structured
from cloudevents.http import CloudEvent
from grpc import ServicerContext
from kserve.protocol.infer_type import InferOutput, InferRequest, InferResponse
from ..constants.constants import PredictorProtocol
from ..errors import InvalidInput


def is_running_in_k8s():
    return os.path.isdir("/var/run/secrets/kubernetes.io/")


def get_current_k8s_namespace():
    with open("/var/run/secrets/kubernetes.io/serviceaccount/namespace", "r") as f:
        return f.readline()


def get_default_target_namespace():
    if not is_running_in_k8s():
        return "default"
    return get_current_k8s_namespace()


def get_isvc_namespace(inferenceservice):
    return inferenceservice.metadata.namespace or get_default_target_namespace()


def get_ig_namespace(inferencegraph):
    return inferencegraph.metadata.namespace or get_default_target_namespace()


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
    return (
        "time" in body
        and "type" in body
        and "source" in body
        and "id" in body
        and "specversion" in body
        and "data" in body
    )


def create_response_cloudevent(
    model_name: str, response: Dict, req_attributes: Dict, binary_event=False
) -> tuple:
    ce_attributes = {}

    if os.getenv("CE_MERGE", "false").lower() == "true":
        if binary_event:
            ce_attributes = req_attributes
            if "datacontenttype" in ce_attributes:  # Optional field so must check
                del ce_attributes["datacontenttype"]
        else:
            ce_attributes = req_attributes

        # Remove these fields so we generate new ones
        del ce_attributes["id"]
        del ce_attributes["time"]

    ce_attributes["type"] = os.getenv("CE_TYPE", "io.kserve.inference.response")
    ce_attributes["source"] = os.getenv(
        "CE_SOURCE", f"io.kserve.inference.{model_name}"
    )

    event = CloudEvent(ce_attributes, response)

    if binary_event:
        event_headers, event_body = to_binary(event)
    else:
        event_headers, event_body = to_structured(event)

    return event_headers, event_body


def generate_uuid() -> str:
    return str(uuid.uuid4())


def to_headers(context: ServicerContext) -> Dict[str, str]:
    metadata = context.invocation_metadata()
    if hasattr(context, "trailing_metadata"):
        metadata += context.trailing_metadata()
    headers = {}
    for metadatum in metadata:
        headers[metadatum.key] = metadatum.value

    return headers


def get_predict_input(
    payload: Union[Dict, InferRequest], columns: List = None
) -> Union[np.ndarray, pd.DataFrame, List[str]]:
    if isinstance(payload, Dict):
        instances = payload["inputs"] if "inputs" in payload else payload["instances"]
        if len(instances) == 0:
            return np.array(instances)
        if isinstance(instances[0], Dict) or (
            isinstance(instances[0], List)
            and len(instances[0]) != 0
            and isinstance(instances[0][0], Dict)
        ):
            dfs = []
            for instance in instances:
                dfs.append(pd.DataFrame(instance, columns=columns))
            inputs = pd.concat(dfs, axis=0)
            return inputs
        else:
            if isinstance(instances[0], str):
                return instances
            return np.array(instances)
    elif isinstance(payload, InferRequest):
        content_type = ""
        parameters = payload.parameters
        if parameters:
            if isinstance(parameters.get("content_type"), InferParameter):
                # for v2 grpc, we get InferParameter obj eg: {"content_type": string_param: "pd"}
                content_type = str(parameters.get("content_type").string_param)
            else:
                # for v2 http, we get string eg: {"content_type": "pd"}
                content_type = parameters.get("content_type")

        if content_type == "pd":
            return payload.as_dataframe()
        else:
            input = payload.inputs[0]
            if (
                input.datatype == "BYTES"
                and len(input.data) > 0
                and isinstance(input.data[0], str)
            ):
                return input.data
            return input.as_numpy()


def get_predict_response(
    payload: Union[Dict, InferRequest],
    result: Union[np.ndarray, List, pd.DataFrame],
    model_name: str,
) -> Union[Dict, InferResponse]:
    if isinstance(payload, Dict):
        infer_outputs = result
        if isinstance(result, pd.DataFrame):
            infer_outputs = []
            for label, row in result.iterrows():
                infer_outputs.append(row.to_dict())
        elif isinstance(result, np.ndarray):
            infer_outputs = result.tolist()
        return {"predictions": infer_outputs}
    elif isinstance(payload, InferRequest):
        infer_outputs = []
        if isinstance(result, pd.DataFrame):
            for col in result.columns:
                infer_output = InferOutput(
                    name=col,
                    shape=list(result[col].shape),
                    datatype=from_np_dtype(result[col].dtype),
                )
                infer_output.set_data_from_numpy(
                    result[col].to_numpy(), binary_data=payload.use_binary_outputs
                )
                infer_outputs.append(infer_output)
        elif (
            isinstance(result, list) and len(result) > 0 and isinstance(result[0], str)
        ):
            infer_output = InferOutput(
                name="output-0",
                shape=[len(result)],
                datatype="BYTES",
            )
            infer_output.set_data_from_numpy(
                np.array(result, dtype=np.object_),
                binary_data=payload.use_binary_outputs,
            )
            infer_outputs.append(infer_output)
        else:
            if isinstance(result, list):
                result = np.array(result)
            infer_output = InferOutput(
                name="output-0",
                shape=list(result.shape),
                datatype=from_np_dtype(result.dtype),
            )
            infer_output.set_data_from_numpy(
                result, binary_data=payload.use_binary_outputs
            )
            infer_outputs.append(infer_output)
        return InferResponse(
            model_name=model_name,
            infer_outputs=infer_outputs,
            response_id=payload.id if payload.id else generate_uuid(),
            use_binary_outputs=payload.use_binary_outputs,
            requested_outputs=payload.request_outputs,
        )
    else:
        raise InvalidInput(f"unsupported payload type {type(payload)}")


def strtobool(val: str) -> bool:
    """Convert a string representation of truth to True or False.

    True values are 'y', 'yes', 't', 'true', 'on', and '1'; false values
    are 'n', 'no', 'f', 'false', 'off', and '0'.  Raises ValueError if
    'val' is anything else.

    Adapted from deprecated `distutils`
    https://github.com/python/cpython/blob/3.11/Lib/distutils/util.py
    """
    val = val.lower()
    if val in ("y", "yes", "t", "true", "on", "1"):
        return True
    elif val in ("n", "no", "f", "false", "off", "0"):
        return False
    else:
        raise ValueError("invalid truth value %r" % (val,))


def is_v2(protocol: Union[str, PredictorProtocol]) -> bool:
    return protocol == PredictorProtocol.REST_V2 or (
        isinstance(protocol, str)
        and protocol.lower() == PredictorProtocol.REST_V2.value.lower()
    )


def is_v1(protocol: Union[str, PredictorProtocol]) -> bool:
    return protocol == PredictorProtocol.REST_V1 or (
        isinstance(protocol, str)
        and protocol.lower() == PredictorProtocol.REST_V1.value.lower()
    )
