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

import socket
from unittest.mock import AsyncMock, Mock

import pytest

from kserve.protocol.rest import server as rest_mod
from kserve.protocol.rest.multiprocess.server import RESTServerMultiProcess


@pytest.mark.asyncio
async def test_multiprocess_server_uses_tcp_protocol_socket(monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)

    server = RESTServerMultiProcess(
        app="dummy:app",
        data_plane=Mock(),
        model_repository_extension=Mock(),
        http_port=0,
        workers=2,
    )

    captured_sockets = []

    def capture_sockets(sockets):
        captured_sockets.extend(sockets)

    monkeypatch.setattr(server, "init_processes", capture_sockets)
    monkeypatch.setattr(server, "terminate_all", AsyncMock())

    server.should_exit.set()

    await server.start()

    assert len(captured_sockets) == 1

    sock = captured_sockets[0]
    try:
        assert sock.proto == socket.IPPROTO_TCP
    finally:
        sock.close()
