# Copyright 2026 The KServe Authors.
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

import orjson
import pandas as pd
import pytest

from kserve.protocol.infer_type import InferInput, InferRequest, InferResponse
from kserve.utils.utils import get_predict_response


def _v2_request(parameters=None) -> InferRequest:
    return InferRequest(
        model_name="model",
        infer_inputs=[InferInput("input-0", [2], "FP32", [1.0, 2.0])],
        parameters=parameters,
    )


# "string" is a pandas extension dtype on pandas 2 and 3. A plain list of str is
# object dtype on pandas 2 and the default string dtype on pandas 3.
@pytest.mark.parametrize(
    "labels",
    [
        pd.array(["cat", "dog"], dtype="string"),
        ["cat", "dog"],
    ],
    ids=["string-extension-dtype", "default-str-inference"],
)
def test_dataframe_string_column_is_bytes_output(labels):
    result = pd.DataFrame({"label": labels, "score": [0.25, 0.75]})

    response = get_predict_response(_v2_request(), result, "model")

    body, _ = response.to_rest()
    outputs = {output["name"]: output for output in body["outputs"]}
    assert outputs["label"]["datatype"] == "BYTES"
    assert outputs["label"]["shape"] == [2]
    assert outputs["label"]["data"] == ["cat", "dog"]
    assert outputs["score"]["datatype"] == "FP64"
    assert outputs["score"]["data"] == [0.25, 0.75]


def test_dataframe_string_column_round_trips_over_grpc():
    result = pd.DataFrame({"label": pd.array(["cat", "dog"], dtype="string")})

    response = get_predict_response(_v2_request(), result, "model")

    grpc_output = response.to_grpc().outputs[0]
    assert grpc_output.datatype == "BYTES"
    assert list(grpc_output.shape) == [2]
    assert list(grpc_output.contents.bytes_contents) == [b"cat", b"dog"]


def test_dataframe_string_column_as_binary_output():
    result = pd.DataFrame({"label": pd.array(["cat", "dog"], dtype="string")})
    request = _v2_request(parameters={"binary_data_output": True})

    response = get_predict_response(request, result, "model")

    body, json_length = response.to_rest()
    header = orjson.loads(body[:json_length])["outputs"][0]
    assert header["datatype"] == "BYTES"
    assert "data" not in header
    # Each element is a 4-byte length prefix followed by the string.
    assert header["parameters"]["binary_data_size"] == 2 * (4 + 3)
    decoded = InferResponse.from_bytes(body, json_length).outputs[0]
    assert decoded.datatype == "BYTES"
    assert decoded.data == ["cat", "dog"]
