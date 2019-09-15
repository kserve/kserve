import pytest
import tornado
import unittest.mock as mock
from kfserving.protocols.tensorflow_http import TensorflowRequestHandler
from requests.exceptions import ConnectionError


def test_data_plane_tensorflow_protocol_client():
    mock_resp = mock.Mock()
    mock_resp.status_code = 200
    mock_resp.json = mock.Mock(return_value={"predictions": [[0.1, 0.9]]})
    with mock.patch('requests.post', return_value=mock_resp):
        res = TensorflowRequestHandler.predict([[1, 2]], 'http://flower.default.svc.cluster.local/v1/models/flower')
        assert res == [[0.1, 0.9]]


def test_data_plane_tensorflow_protocol_client_raise_error():
    with mock.patch('requests.post', side_effect=ConnectionError("fail to connect")):
        with pytest.raises(ConnectionError):
            TensorflowRequestHandler.predict([[1, 2]], 'http://flower.default.svc.cluster.local/v1/models/flower')


def test_data_plane_tensorflow_protocol_client_raise_http_error():
    mock_resp = mock.Mock()
    mock_resp.status_code = 500
    mock_resp.reason = "Internal Server Error"
    with mock.patch('requests.post', return_value=mock_resp):
        with pytest.raises(tornado.web.HTTPError):
            TensorflowRequestHandler.predict([[1, 2]], 'http://flower.default.svc.cluster.local/v1/models/flower')

