# Copyright 2023 The KServe Authors.
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

from unittest.mock import AsyncMock, Mock

import pytest

from kserve.protocol.rest import server as rest_mod


@pytest.mark.parametrize(
    "loop_value,expected",
    [
        ("auto", "auto"),
        ("asyncio", "asyncio"),
        ("uvloop", "uvloop"),
        ("invalid-value", "auto"),  # invalid falls back to 'auto'
    ],
)
def test_config_loop_value(loop_value, expected, monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    data_plane = Mock()
    model_repo_ext = Mock()

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=data_plane,
        model_repository_extension=model_repo_ext,
        http_port=8080,
        event_loop=loop_value,
    )

    assert rs.config.loop == expected


@pytest.mark.parametrize(
    "timeout_keep_alive,expected",
    [
        (65, 65),  # default value
        (120, 120),  # custom value
        (5, 5),  # uvicorn default
    ],
)
def test_config_timeout_keep_alive(timeout_keep_alive, expected, monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    data_plane = Mock()
    model_repo_ext = Mock()

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=data_plane,
        model_repository_extension=model_repo_ext,
        http_port=8080,
        timeout_keep_alive=timeout_keep_alive,
    )

    assert rs.config.timeout_keep_alive == expected


def test_config_timeout_keep_alive_default(monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    data_plane = Mock()
    model_repo_ext = Mock()

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=data_plane,
        model_repository_extension=model_repo_ext,
        http_port=8080,
    )

    assert rs.config.timeout_keep_alive == 65


@pytest.mark.asyncio
async def test_ssl_cert_refresher_lifecycle(monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    monkeypatch.setattr(rest_mod.uvicorn.Server, "serve", AsyncMock())
    refresher = Mock()
    refresher_class = Mock(return_value=refresher)
    monkeypatch.setattr(rest_mod, "SSLCertRefresher", refresher_class)
    ssl_context = Mock()

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=Mock(),
        model_repository_extension=Mock(),
        http_port=8443,
        ssl_certfile="/etc/tls/tls.crt",
        ssl_keyfile="/etc/tls/tls.key",
    )

    def load_config():
        rs.config.loaded = True
        rs.config.ssl = ssl_context

    monkeypatch.setattr(rs.config, "load", load_config)

    await rs.start()

    refresher_class.assert_called_once_with(
        ssl_context=ssl_context,
        key_path="/etc/tls/tls.key",
        cert_path="/etc/tls/tls.crt",
    )
    refresher.stop.assert_called_once_with()
