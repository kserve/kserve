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

from unittest.mock import Mock

import numpy as np
import pytest
from paddle import inference

from kserve.errors import InferenceError
from kserve.protocol.infer_type import (
    InferInput,
    InferRequest,
    InferResponse,
    RequestedOutput,
)
from paddleserver import PaddleModel


@pytest.fixture
def model(tmp_path, monkeypatch):
    (tmp_path / "model.json").touch()
    (tmp_path / "model.pdiparams").touch()
    predictor = Mock()
    predictor.get_input_names.return_value = ["ids", "mask"]
    predictor.get_output_names.return_value = ["scores", "count"]
    inputs = {name: Mock() for name in ["ids", "mask"]}
    inputs["ids"].type.return_value = inference.DataType.INT64
    inputs["mask"].type.return_value = inference.DataType.FLOAT32
    outputs = {
        "scores": np.array([[1, 2], [3, 4]], dtype=np.float32),
        "count": np.array([2, 2], dtype=np.int64),
    }
    predictor.get_input_handle.side_effect = inputs.__getitem__
    predictor.get_output_handle.side_effect = lambda name: Mock(
        copy_to_cpu=Mock(return_value=outputs[name])
    )
    monkeypatch.setattr("paddleserver.model.Storage.download", lambda _: str(tmp_path))
    monkeypatch.setattr(inference, "Config", Mock())
    monkeypatch.setattr(inference, "create_predictor", lambda _: predictor)
    server = PaddleModel("multi", str(tmp_path))
    server.load()
    return server, predictor, inputs, outputs


def request(names=("mask", "ids"), binary=False):
    tensors = {
        "ids": InferInput("ids", [2, 2], "INT64", [[1, 2], [3, 4]]),
        "mask": InferInput("mask", [2, 2], "FP32", [[1, 1], [1, 1]]),
    }
    return InferRequest(
        model_name="multi",
        infer_inputs=[tensors[name] for name in names],
        request_id="request-1",
        parameters={"binary_data_output": binary},
    )


@pytest.mark.parametrize("binary", [False, True])
def test_v2_named_inputs_and_independent_outputs(model, binary):
    server, predictor, inputs, outputs = model
    response = server.predict(request(binary=binary))
    predictor.run.assert_called_once()
    ids = inputs["ids"].copy_from_cpu.call_args.args[0]
    assert ids.dtype == np.int64
    np.testing.assert_array_equal(ids, [[1, 2], [3, 4]])
    assert inputs["mask"].copy_from_cpu.call_args.args[0].dtype == np.float32
    assert response.id == "request-1"
    rest, header_length = response.to_rest()
    parsed = (
        InferResponse.from_bytes(rest, header_length)
        if binary
        else InferResponse.from_rest(rest)
    )
    for name, expected in outputs.items():
        output = parsed.get_output_by_name(name)
        np.testing.assert_array_equal(output.as_numpy(), expected)
        assert output.as_numpy().dtype == expected.dtype


@pytest.mark.parametrize(
    "payload",
    [
        {"inputs": {"ids": [[1, 2], [3, 4]], "mask": [[1, 1], [1, 1]]}},
        {
            "instances": [
                {"ids": [1, 2], "mask": [1, 1]},
                {"ids": [3, 4], "mask": [1, 1]},
            ]
        },
    ],
)
def test_v1_named_inputs(model, payload):
    server, _, inputs, _ = model
    assert server.predict(payload) == {
        "predictions": [
            {"scores": [1.0, 2.0], "count": 2},
            {"scores": [3.0, 4.0], "count": 2},
        ]
    }
    assert inputs["ids"].copy_from_cpu.call_args.args[0].dtype == np.int64


@pytest.mark.parametrize(
    "payload",
    [
        request(("ids",)),
        request(("ids", "ids")),
        {"inputs": {"ids": [[1]], "extra": [[1]]}},
        {"instances": [{"ids": [1], "mask": [1]}, {"ids": [2]}]},
    ],
)
def test_invalid_inputs_do_not_run_predictor(model, payload):
    server, predictor, _, _ = model
    with pytest.raises(InferenceError):
        server.predict(payload)
    predictor.run.assert_not_called()


def test_missing_input_after_successful_request(model):
    server, predictor, _, _ = model
    server.predict(request())
    predictor.run.reset_mock()
    with pytest.raises(InferenceError, match="Expected inputs"):
        server.predict(request(("ids",)))
    predictor.run.assert_not_called()


def test_requested_output(model):
    server, _, _, _ = model
    payload = request()
    payload.request_outputs = [RequestedOutput("count")]
    rest, _ = server.predict(payload).to_rest()
    assert [output["name"] for output in rest["outputs"]] == ["count"]
    assert rest["outputs"][0]["data"] == [2, 2]


@pytest.mark.parametrize("value", [np.array(2), np.array([1, 2, 3])])
def test_v1_rejects_outputs_without_shared_batch(model, value):
    server, _, _, outputs = model
    outputs["count"] = value
    server.output_tensors["count"].copy_to_cpu.return_value = value
    with pytest.raises(InferenceError, match="batch dimension"):
        server.predict({"inputs": {"ids": [[1, 2], [3, 4]], "mask": [[1, 1], [1, 1]]}})
    # V2 can represent independently shaped outputs without a batch constraint.
    assert server.predict(request()).get_output_by_name("count").shape == list(
        value.shape
    )


def test_single_tensor_compatibility(model):
    server, predictor, inputs, _ = model
    predictor.get_input_names.return_value = ["mask"]
    predictor.get_output_names.return_value = ["scores"]
    server.load()
    assert server.predict({"instances": [[1, 2]]}) == {
        "predictions": [[1.0, 2.0], [3.0, 4.0]]
    }
    response = server.predict(
        InferRequest(
            model_name="multi",
            infer_inputs=[InferInput("inputs", [1, 2], "FP32", [[1, 2]])],
        )
    )
    assert response.outputs[0].name == "output-0"
    assert inputs["mask"].copy_from_cpu.call_count == 2


def test_cpu_multi_input_model(tmp_path):
    import paddle

    class TwoInputs(paddle.nn.Layer):
        def __init__(self):
            super().__init__()
            self.projection = paddle.nn.Linear(2, 3)

        def forward(self, ids, mask):
            return self.projection(ids.astype("float32") * mask), ids.sum(axis=1)

    network = TwoInputs()
    network.eval()
    ids = np.array([[1, 2], [3, 4]], dtype=np.int64)
    mask = np.array([[1, 0], [0, 1]], dtype=np.float32)
    expected = network(paddle.to_tensor(ids), paddle.to_tensor(mask))
    static_model = paddle.jit.to_static(
        network,
        input_spec=[
            paddle.static.InputSpec([None, 2], "int64", "ids"),
            paddle.static.InputSpec([None, 2], "float32", "mask"),
        ],
        full_graph=True,
    )
    paddle.jit.save(static_model, str(tmp_path / "model"))
    server = PaddleModel("cpu-multi", str(tmp_path))
    server.load()
    response = server.predict(
        InferRequest(
            model_name="cpu-multi",
            infer_inputs=[
                InferInput("mask", list(mask.shape), "FP32", mask),
                InferInput("ids", list(ids.shape), "INT64", ids),
            ],
        )
    )
    assert len(response.outputs) == 2
    assert {output.name for output in response.outputs} == set(
        server.predictor.get_output_names()
    )
    actual = {tuple(output.shape): output.as_numpy() for output in response.outputs}
    for tensor in expected:
        np.testing.assert_allclose(
            actual[tuple(tensor.shape)], tensor.numpy(), rtol=1e-5
        )
    v1 = server.predict({"inputs": {"ids": ids.tolist(), "mask": mask.tolist()}})
    for output in response.outputs:
        values = [row[output.name] for row in v1["predictions"]]
        np.testing.assert_allclose(values, output.as_numpy(), rtol=1e-5)
