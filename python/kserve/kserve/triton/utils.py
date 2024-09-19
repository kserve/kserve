# Copyright 2024 The KServe Authors.
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

import argparse
from argparse import ArgumentParser
from typing import Optional

import numpy

from kserve import InferRequest, InferResponse, InferOutput
from kserve.logging import logger

try:
    import tritonserver
    from tritonserver import DataType
    from kserve.triton.triton_configuration import TritonOptions

    _triton = True
except ImportError:
    _triton = False


def is_triton_available() -> bool:
    return _triton


def triton_dtype_to_str(triton_dtype):
    if triton_dtype == DataType.BOOL:
        return "BOOL"
    elif triton_dtype == DataType.INT8:
        return "INT8"
    elif triton_dtype == DataType.INT16:
        return "INT16"
    elif triton_dtype == DataType.INT32:
        return "INT32"
    elif triton_dtype == DataType.INT64:
        return "INT64"
    elif triton_dtype == DataType.UINT8:
        return "UNIT8"
    elif triton_dtype == DataType.UINT16:
        return "UINT16"
    elif triton_dtype == DataType.UINT32:
        return "UINT32"
    elif triton_dtype == DataType.UINT64:
        return "UINT64"
    elif triton_dtype == DataType.FP16:
        return "FP16"
    elif triton_dtype == DataType.FP32:
        return "FP32"
    elif triton_dtype == DataType.BYTES:
        return "BYTES"
    else:
        raise ValueError(f"Invalid datatype {triton_dtype}")


def create_triton_infer_request(
    request: InferRequest, model: "tritonserver.Model"
) -> "tritonserver.InferenceRequest":
    infer_req = model.create_request()
    infer_req.request_id = request.id
    infer_req.parameters = request.parameters if request.parameters else {}
    inputs = {}
    for inferInput in request.inputs:
        inputs[inferInput.name] = inferInput.as_numpy()
    infer_req.inputs = inputs
    return infer_req


def to_infer_response(response):
    infer_outputs = []
    logger.info("res: %s", response)
    logger.info("res type: %s", type(response.outputs))
    for name, output in response.outputs.items():
        infer_output = InferOutput(
            name=name,
            datatype=triton_dtype_to_str(output.data_type),
            shape=output.shape,
        )
        infer_output.set_data_from_numpy(numpy.from_dlpack(output))
        infer_outputs.append(infer_output)
    res = InferResponse(
        response_id=response.request_id,
        parameters=response.parameters,
        model_name=response.model.name,
        model_version=str(response.model.version),
        infer_outputs=infer_outputs,
    )
    return res


def maybe_add_triton_cli_parser(parser: ArgumentParser) -> ArgumentParser:
    """Add Triton CLI arguments to the parser if Triton is available.

    Args:
        parser (ArgumentParser): The parser to add the Triton CLI arguments to.
    Returns:
        ArgumentParser: The parser with the Triton CLI arguments added.
    """
    if _triton:
        parser = TritonOptions.add_cli_args(parser)
    return parser


def build_triton_options(args: argparse.Namespace) -> Optional["tritonserver.Options"]:
    """Build TritonArgs object from the parsed arguments.

    Args:
        args: The parsed arguments.
    Returns:
        TritonArgs: The TritonArgs object.
    """
    if not _triton:
        return None
    return TritonOptions.from_cli_args(args)
