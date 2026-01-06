import os
import sys
import builtins
import types
import pytest
import numpy as np
import pandas as pd
from unittest.mock import MagicMock, patch

from kserve.utils import utils
from kserve.protocol.infer_type import InferRequest, InferOutput, InferResponse
from kserve.constants.constants import PredictorProtocol
from kserve.errors import InvalidInput

# -------------------------
# K8s utils tests
# -------------------------

def test_is_running_in_k8s(monkeypatch):
    monkeypatch.setattr(os.path, "isdir", lambda path: True)
    assert utils.is_running_in_k8s() is True
    monkeypatch.setattr(os.path, "isdir", lambda path: False)
    assert utils.is_running_in_k8s() is False

def test_get_default_target_namespace(monkeypatch):
    monkeypatch.setattr(utils, "is_running_in_k8s", lambda: False)
    assert utils.get_default_target_namespace() == "default"

    monkeypatch.setattr(utils, "is_running_in_k8s", lambda: True)
    monkeypatch.setattr(utils, "get_current_k8s_namespace", lambda: "ns1")
    assert utils.get_default_target_namespace() == "ns1"

# -------------------------
# CPU count test
# -------------------------

@patch("psutil.Process")
def test_cpu_count(mock_process):
    # Mock cpu_affinity
    mock_process.return_value.cpu_affinity.return_value = [0,1]
    # Mock open to simulate cgroups
    with patch("builtins.open", mock_open=MagicMock()) as m_open:
        m_open.return_value.__enter__.return_value.read.side_effect = ["200000", "100000"]
        count = utils.cpu_count()
        assert count > 0

# -------------------------
# CloudEvent tests
# -------------------------

def test_is_structured_cloudevent():
    body = {
        "time": "t", "type": "x", "source": "s",
        "id": "i", "specversion": "v", "data": {}
    }
    assert utils.is_structured_cloudevent(body) is True
    body.pop("time")
    assert utils.is_structured_cloudevent(body) is False

def test_create_response_cloudevent(monkeypatch):
    monkeypatch.setenv("CE_MERGE", "false")
    h, b = utils.create_response_cloudevent("model1", {"a": 1}, {}, binary_event=False)
    assert isinstance(h, dict) and isinstance(b, bytes)

# -------------------------
# UUID and headers tests
# -------------------------

def test_generate_uuid():
    uid = utils.generate_uuid()
    assert isinstance(uid, str)

def test_to_headers():
    from types import SimpleNamespace
    import kserve.utils as utils
    from unittest.mock import MagicMock

    context = MagicMock()
    # gRPC metadata: list of objects with .key and .value
    context.invocation_metadata.return_value = [
        SimpleNamespace(key="x", value="y")
    ]
    # trailing_metadata may exist, return empty list
    context.trailing_metadata = MagicMock(return_value=[])

    headers = utils.to_headers(context)
    assert headers["x"] == "y"


# -------------------------
# Predict input/output tests
# -------------------------

def test_get_predict_input_numpy():
    payload = {"inputs": [[1, 2], [3, 4]]}
    out = utils.get_predict_input(payload)
    assert isinstance(out, np.ndarray)
    assert out.shape == (2, 2)

def test_get_predict_input_dataframe():
    payload = {"inputs": [{"a": [1], "b":[2]}, {"a":[3], "b":[4]}]}  # <-- values as lists
    df = utils.get_predict_input(payload)
    assert isinstance(df, pd.DataFrame)
    assert df.shape[0] == 2


def test_get_predict_response_dict_numpy():
    payload = {"inputs": [[1, 2]]}
    result = np.array([[5, 6]])
    resp = utils.get_predict_response(payload, result, "model1")
    assert resp["predictions"] == [[5,6]]

def test_strtobool_true_false():
    for val in ["y", "YES", "true", "1"]:
        assert utils.strtobool(val) is True
    for val in ["n", "no", "False", "0"]:
        assert utils.strtobool(val) is False
    with pytest.raises(ValueError):
        utils.strtobool("invalid")

def test_is_v1_v2():
    assert utils.is_v1(PredictorProtocol.REST_V1)
    assert utils.is_v2(PredictorProtocol.REST_V2)
    # also test string versions (exact match)
    assert utils.is_v1(PredictorProtocol.REST_V1.value)
    assert utils.is_v2(PredictorProtocol.REST_V2.value)

