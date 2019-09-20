# Copyright 2019 kubeflow.org.
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

import pytest
import tornado
import unittest.mock as mock
import requests
from kfserving.protocols.tensorflow_http import TensorflowRequestHandler


def test_data_plane_tensorflow_protocol_client():
    mock_resp = mock.Mock()
    mock_resp.status_code = 200
    mock_resp.json = mock.Mock(return_value={"predictions": [[0.1, 0.9]]})
    with mock.patch('requests.post', return_value=mock_resp):
        res = TensorflowRequestHandler.predict([[1, 2]], 'http://flower.default/v1/models/flower')
        assert res == [[0.1, 0.9]]


def test_data_plane_tensorflow_protocol_client_raise_error():
    with mock.patch('requests.post', side_effect=
                    requests.exceptions.ConnectionError("fail to connect")):
        with pytest.raises(requests.exceptions.ConnectionError):
            TensorflowRequestHandler.predict([[1, 2]], 'http://flower.default/v1/models/flower')


def test_data_plane_tensorflow_protocol_client_raise_http_error():
    mock_resp = mock.Mock()
    mock_resp.status_code = 500
    mock_resp.reason = "Internal Server Error"
    with mock.patch('requests.post', return_value=mock_resp):
        with pytest.raises(tornado.web.HTTPError):
            TensorflowRequestHandler.predict([[1, 2]], 'http://flower.default/v1/models/flower')
