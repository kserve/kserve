# Copyright 2024 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""The `llmman serve` client: endpoint resolution and the daemon protocol.

Exercised against a real HTTP server on a loopback port rather than mocks, so
the NDJSON streaming contract is genuinely tested.
"""

import http.server
import json
import socketserver
import tempfile
import threading

import pytest

from kserve_storage import llmman


@pytest.mark.parametrize(
    "host,want",
    [
        ("", "http://127.0.0.1:17434"),
        ("1.2.3.4:9999", "http://1.2.3.4:9999"),
        ("1.2.3.4", "http://1.2.3.4:17434"),
        ("http://1.2.3.4:9999", "http://1.2.3.4:9999"),
        ("http://1.2.3.4:9999/ignored", "http://1.2.3.4:9999"),
        ('"1.2.3.4:9999"', "http://1.2.3.4:9999"),
        # A wildcard bind is meaningful to the server but not to a client,
        # which cannot connect to "every interface".
        ("0.0.0.0:9999", "http://127.0.0.1:9999"),
        ("[::]:9999", "http://[::1]:9999"),
    ],
)
def test_endpoint_parsing(monkeypatch, host, want):
    monkeypatch.setenv(llmman.HOST_ENV, host)
    assert llmman.endpoint() == want


def test_binary_default_and_override(monkeypatch):
    monkeypatch.delenv(llmman.BIN_ENV, raising=False)
    assert llmman.llmman_bin() == "llmman"
    monkeypatch.setenv(llmman.BIN_ENV, "/opt/bin/llmman")
    assert llmman.llmman_bin() == "/opt/bin/llmman"
    # An empty override is a mistake, not a request to run the empty string.
    monkeypatch.setenv(llmman.BIN_ENV, "   ")
    assert llmman.llmman_bin() == "llmman"


def _ndjson(*objs):
    return "".join(json.dumps(o) + "\n" for o in objs)


class _FakeDaemon:
    """A minimal stand-in for `llmman serve`, on a real loopback port."""

    def __init__(self):
        self.version = {"version": "0.1.0", "pid": 1}
        self.pull_body = _ndjson({"status": "success"})
        self.pull_status = 200
        self.last_request = None
        daemon = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def _send(self, status, body, ctype):
                raw = body.encode()
                self.send_response(status)
                self.send_header("Content-Type", ctype)
                self.send_header("Content-Length", str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

            def do_GET(self):
                assert self.path == "/api/version"
                self._send(200, json.dumps(daemon.version), "application/json")

            def do_POST(self):
                assert self.path == "/api/pull"
                length = int(self.headers.get("Content-Length", 0))
                daemon.last_request = json.loads(self.rfile.read(length))
                self._send(daemon.pull_status, daemon.pull_body, "application/x-ndjson")

        self._server = socketserver.TCPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self._server.server_address[1]}"
        threading.Thread(target=self._server.serve_forever, daemon=True).start()

    def close(self):
        self._server.shutdown()
        self._server.server_close()


@pytest.fixture
def daemon():
    d = _FakeDaemon()
    yield d
    d.close()


def test_check_daemon_accepts_a_llmman_daemon(daemon):
    llmman.check_daemon(daemon.url)


def test_check_daemon_rejects_a_non_llmman_server(daemon):
    daemon.version = {"hello": "world"}
    with pytest.raises(RuntimeError, match="not an llmman daemon"):
        llmman.check_daemon(daemon.url)


def test_check_daemon_reports_nothing_listening():
    with pytest.raises(RuntimeError, match="llmman serve"):
        llmman.check_daemon("http://127.0.0.1:1")


def test_pull_succeeds_and_forwards_progress(daemon):
    daemon.pull_body = _ndjson(
        {"status": "pulling manifest"},
        {"status": "pulling blobs", "completed": 50, "total": 100},
        {"status": "success"},
    )
    seen = []
    llmman.pull(daemon.url, "ghcr.io/org/model:tag", lambda *a: seen.append(a))

    assert daemon.last_request == {"model": "ghcr.io/org/model:tag"}
    assert seen == [("pulling manifest", 0, 0), ("pulling blobs", 50, 100)]


def test_pull_reports_in_band_error_at_http_200(daemon):
    # The daemon streams errors in-band, so a 200 does not mean success.
    daemon.pull_body = _ndjson({"status": "pulling"}, {"error": "unauthorized"})
    with pytest.raises(RuntimeError, match="unauthorized"):
        llmman.pull(daemon.url, "ref")


def test_pull_rejects_a_stream_that_ends_without_success(daemon):
    daemon.pull_body = _ndjson({"status": "pulling blobs"})
    with pytest.raises(RuntimeError, match="without reporting success"):
        llmman.pull(daemon.url, "ref")


def test_pull_tolerates_a_non_json_diagnostic_line(daemon):
    daemon.pull_body = "not json\n" + _ndjson({"status": "success"})
    llmman.pull(daemon.url, "ref")


def test_pull_reports_non_ok_status(daemon):
    daemon.pull_status = 400
    daemon.pull_body = '{"error":"bad request"}'
    with pytest.raises(RuntimeError):
        llmman.pull(daemon.url, "ref")


def test_parse_resolve_output_accepts_the_documented_contract():
    with tempfile.TemporaryDirectory() as path:
        line = json.dumps({"reference": "r", "path": path, "format": "safetensors"})
        assert llmman.parse_resolve_output(line, "r") == path


def test_parse_resolve_output_tolerates_leaked_diagnostics():
    with tempfile.TemporaryDirectory() as path:
        out = "pulling...\n" + json.dumps({"path": path}) + "\n"
        assert llmman.parse_resolve_output(out, "r") == path


def test_parse_resolve_output_ignores_unknown_fields():
    with tempfile.TemporaryDirectory() as path:
        line = json.dumps({"path": path, "format": "gguf", "mmproj": "/x", "new": 1})
        assert llmman.parse_resolve_output(line, "r") == path


@pytest.mark.parametrize(
    "bad",
    [
        "",
        "   \n\n",
        "not json",
        '["a", "list"]',
        '{"no_path": 1}',
        '{"path": ""}',
        '{"path": 3}',
        '{"path": "/nonexistent/xyzzy"}',
    ],
)
def test_parse_resolve_output_rejects_malformed_output(bad):
    with pytest.raises(RuntimeError):
        llmman.parse_resolve_output(bad, "r")


def test_resolve_reports_a_missing_binary(monkeypatch):
    monkeypatch.setenv(llmman.BIN_ENV, "/definitely/not/here/llmman")
    with pytest.raises(RuntimeError, match="not found"):
        llmman.resolve("ref")
