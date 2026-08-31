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
from unittest.mock import Mock

import pytest
from watchfiles import Change

from kserve.protocol.rest import ssl_cert_refresher


@pytest.mark.asyncio
async def test_ssl_cert_refresher_reloads_changed_certificate(monkeypatch):
    cert_path = "/etc/tls/tls.crt"
    key_path = "/etc/tls/tls.key"

    async def changes(*paths):
        assert paths == (key_path, cert_path)
        yield {(Change.modified, cert_path)}

    monkeypatch.setattr(ssl_cert_refresher, "awatch", changes)
    ssl_context = Mock()

    refresher = ssl_cert_refresher.SSLCertRefresher(
        ssl_context=ssl_context,
        key_path=key_path,
        cert_path=cert_path,
    )
    await refresher._watch_task

    ssl_context.load_cert_chain.assert_called_once_with(cert_path, key_path)


@pytest.mark.asyncio
async def test_ssl_cert_refresher_keeps_watching_after_reload_error(
    monkeypatch, caplog
):
    cert_path = "/etc/tls/tls.crt"
    key_path = "/etc/tls/tls.key"

    async def changes(*_paths):
        yield {(Change.modified, cert_path)}
        yield {(Change.modified, key_path)}

    monkeypatch.setattr(ssl_cert_refresher, "awatch", changes)
    ssl_context = Mock()
    ssl_context.load_cert_chain.side_effect = [ValueError("invalid certificate"), None]

    refresher = ssl_cert_refresher.SSLCertRefresher(
        ssl_context=ssl_context,
        key_path=key_path,
        cert_path=cert_path,
    )
    await refresher._watch_task

    assert ssl_context.load_cert_chain.call_count == 2
    assert "Failed to reload SSL certificate chain" in caplog.text


@pytest.mark.asyncio
async def test_ssl_cert_refresher_stop_cancels_watcher(monkeypatch):
    watcher_started = asyncio.Event()

    async def changes(*_paths):
        watcher_started.set()
        await asyncio.Event().wait()
        yield

    monkeypatch.setattr(ssl_cert_refresher, "awatch", changes)
    refresher = ssl_cert_refresher.SSLCertRefresher(
        ssl_context=Mock(),
        key_path="/etc/tls/tls.key",
        cert_path="/etc/tls/tls.crt",
    )
    await watcher_started.wait()
    watch_task = refresher._watch_task

    refresher.stop()

    assert refresher._watch_task is None
    assert watch_task.cancelling()
