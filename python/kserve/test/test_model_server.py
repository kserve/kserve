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

import pytest
from kserve import ModelServer

UNKNOWN_MODEL_TYPE_ERR_MESSAGE = "Unknown model collection type"


def test_model_server_start_no_models():
    server = ModelServer()

    with pytest.raises(RuntimeError) as exc:
        server.start(models=None)

    assert exc.value.args[0] == UNKNOWN_MODEL_TYPE_ERR_MESSAGE


def test_model_server_ssl_switches_to_https_port():
    server = ModelServer(
        ssl_certfile="/etc/tls/private/tls.crt",
        ssl_keyfile="/etc/tls/private/tls.key",
    )
    assert server.http_port == 8443


def test_model_server_ssl_keeps_explicit_port():
    server = ModelServer(
        http_port=9000,
        ssl_certfile="/etc/tls/private/tls.crt",
        ssl_keyfile="/etc/tls/private/tls.key",
    )
    assert server.http_port == 9000


def test_model_server_ssl_keeps_explicit_default_http_port():
    server = ModelServer(
        http_port=8080,
        ssl_certfile="/etc/tls/private/tls.crt",
        ssl_keyfile="/etc/tls/private/tls.key",
    )
    assert server.http_port == 8080


def test_model_server_no_ssl_keeps_default_port():
    server = ModelServer()
    assert server.http_port == 8080


@pytest.mark.parametrize(
    "ssl_certfile,ssl_keyfile",
    [
        ("/etc/tls/private/tls.crt", None),
        (None, "/etc/tls/private/tls.key"),
    ],
)
def test_model_server_rejects_partial_ssl_configuration(ssl_certfile, ssl_keyfile):
    with pytest.raises(
        ValueError, match="ssl_certfile and ssl_keyfile must be configured together"
    ):
        ModelServer(ssl_certfile=ssl_certfile, ssl_keyfile=ssl_keyfile)
