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

import asyncio
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
        if hasattr(socket, "SO_PROTOCOL"):
            assert sock.proto == socket.IPPROTO_TCP

            tcp_nodelay = asyncio.get_running_loop().create_future()

            class Protocol(asyncio.Protocol):
                def connection_made(self, transport):
                    accepted = transport.get_extra_info("socket")
                    tcp_nodelay.set_result(
                        accepted.getsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY)
                    )
                    transport.close()

            asyncio_server = await asyncio.get_running_loop().create_server(
                Protocol, sock=sock
            )
            _, writer = await asyncio.open_connection(*sock.getsockname())
            try:
                assert await asyncio.wait_for(tcp_nodelay, timeout=1) != 0
            finally:
                writer.close()
                await writer.wait_closed()
                asyncio_server.close()
                await asyncio_server.wait_closed()
    finally:
        sock.close()
