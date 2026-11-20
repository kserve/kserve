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

import os
import unittest.mock as mock
from pathlib import Path

import pytest
import requests

from kserve_storage import Storage

# Real (self-signed, parse-valid) certificate: the combined bundle is
# validated with ssl before being exported, so the fixture must parse.
TEST_CA_PEM = """-----BEGIN CERTIFICATE-----
MIIBhjCCAS2gAwIBAgIUMyxVv2w1Wp1oKdnW+DOnMeesPi8wCgYIKoZIzj0EAwIw
GTEXMBUGA1UEAwwOa3NlcnZlLXRlc3QtY2EwHhcNMjYwNzAyMTYyMDQwWhcNMzYw
NjI5MTYyMDQwWjAZMRcwFQYDVQQDDA5rc2VydmUtdGVzdC1jYTBZMBMGByqGSM49
AgEGCCqGSM49AwEHA0IABBUFVi0qWbwEv/l+HcofdpTKfJbNoWqqa2VZzRTPwLVT
gRgM4IwCS/9BqOk/4kgtaDmwkgaPHezDeSn6+KXGJzqjUzBRMB0GA1UdDgQWBBR1
nfvZnSy6d6wdttlst48UzrMwPDAfBgNVHSMEGDAWgBR1nfvZnSy6d6wdttlst48U
zrMwPDAPBgNVHRMBAf8EBTADAQH/MAoGCCqGSM49BAMCA0cAMEQCIHvDmtj+mck4
EHZ0148y6DFcWpDIAaPyKz2rVv/I0rA2AiA+yiYFjmPwtUwvCOj8yQI6IYMgrjsS
mjrvDJwPyARHZg==
-----END CERTIFICATE-----
"""


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model(mock_snapshot_download):
    uri = "hf://example.com/model:hash_value"
    repo = "example.com"
    model = "model"
    revision = "hash_value"

    Storage.download(uri)

    mock_snapshot_download.assert_called_once_with(
        repo_id=f"{repo}/{model}",
        revision=revision,
        local_dir=mock.ANY,
    )


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_with_global_ca_bundle(mock_snapshot_download, tmp_path):
    ca_bundle_path = tmp_path / "cabundle.crt"
    ca_bundle_path.write_text(TEST_CA_PEM)
    env = {
        "CA_BUNDLE_CONFIGMAP_NAME": "cabundle",
        "CA_BUNDLE_VOLUME_MOUNT_POINT": str(tmp_path),
    }
    with mock.patch.dict(os.environ, env):
        os.environ.pop("REQUESTS_CA_BUNDLE", None)
        os.environ.pop("SSL_CERT_FILE", None)
        Storage.download("hf://example.com/model", out_dir=str(tmp_path / "out"))
        combined_path = os.environ["REQUESTS_CA_BUNDLE"]
        assert os.environ["SSL_CERT_FILE"] == combined_path
        assert ca_bundle_path.read_text() in Path(combined_path).read_text()

    mock_snapshot_download.assert_called_once()


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_with_allow_patterns(mock_snapshot_download):
    uri = "hf://example.com/model"

    Storage._download_hf(uri, "/tmp/out", allow_patterns=["*.safetensors", "*.json"])

    mock_snapshot_download.assert_called_once_with(
        repo_id="example.com/model",
        revision=None,
        local_dir="/tmp/out",
        allow_patterns=["*.safetensors", "*.json"],
    )


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_with_ignore_patterns(mock_snapshot_download):
    uri = "hf://example.com/model"

    Storage._download_hf(uri, "/tmp/out", ignore_patterns=["*.bin", "*.gguf"])

    mock_snapshot_download.assert_called_once_with(
        repo_id="example.com/model",
        revision=None,
        local_dir="/tmp/out",
        ignore_patterns=["*.bin", "*.gguf"],
    )


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_with_both_patterns(mock_snapshot_download):
    uri = "hf://example.com/model"

    Storage._download_hf(
        uri,
        "/tmp/out",
        allow_patterns=["*.json"],
        ignore_patterns=["config.json"],
    )

    mock_snapshot_download.assert_called_once_with(
        repo_id="example.com/model",
        revision=None,
        local_dir="/tmp/out",
        allow_patterns=["*.json"],
        ignore_patterns=["config.json"],
    )


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_no_patterns_omits_kwargs(mock_snapshot_download):
    uri = "hf://example.com/model"

    Storage._download_hf(uri, "/tmp/out")

    mock_snapshot_download.assert_called_once_with(
        repo_id="example.com/model",
        revision=None,
        local_dir="/tmp/out",
    )


@mock.patch("huggingface_hub.snapshot_download")
def test_download_reads_env_patterns(mock_snapshot_download):
    uri = "hf://example.com/model"

    with mock.patch.dict(
        os.environ,
        {
            "STORAGE_ALLOW_PATTERNS": '["*.safetensors"]',
            "STORAGE_IGNORE_PATTERNS": "*.bin,*.gguf",
        },
    ):
        Storage.download(uri, out_dir="/tmp/out")

    mock_snapshot_download.assert_called_once()
    call_kwargs = mock_snapshot_download.call_args[1]
    assert call_kwargs.get("allow_patterns") == ["*.safetensors"]
    assert call_kwargs.get("ignore_patterns") == ["*.bin", "*.gguf"]


@mock.patch("huggingface_hub.snapshot_download")
def test_explicit_patterns_override_env(mock_snapshot_download):
    uri = "hf://example.com/model"

    with mock.patch.dict(
        os.environ,
        {"STORAGE_ALLOW_PATTERNS": '["*.bin"]'},
    ):
        Storage.download(uri, out_dir="/tmp/out", allow_patterns=["*.safetensors"])

    call_kwargs = mock_snapshot_download.call_args[1]
    assert call_kwargs.get("allow_patterns") == ["*.safetensors"]


@mock.patch("huggingface_hub.snapshot_download")
@pytest.mark.parametrize(
    "invalid_uri, error_message",
    [
        (
            "hf://",
            "Invalid Hugging Face URI format",
        ),  # Missing repo and model
        (
            "hf://repo_only",
            "Invalid Hugging Face URI format",
        ),  # Missing model
        ("hf:///model_only", "repository owner cannot be empty"),  # Missing repo
        (
            "hf://repo/:hash_value",
            "model name cannot be empty",
        ),  # Missing model name, hash exists
    ],
)
def test_invalid_uri(mock_snapshot_download, invalid_uri, error_message):
    with pytest.raises(RuntimeError, match=error_message):
        Storage.download(invalid_uri)

    # Ensure that snapshot_download was never called
    mock_snapshot_download.assert_not_called()


@pytest.fixture
def dragonfly_proxy(monkeypatch):
    """Fake dfdaemon proxy recording the requests it receives."""
    import http.server
    import threading

    import huggingface_hub
    from huggingface_hub.utils import _http

    received = []

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            received.append((self.path, dict(self.headers)))
            self.send_response(200)
            self.send_header("Content-Length", "2")
            self.end_headers()
            self.wfile.write(b"ok")

        def log_message(self, *args):
            pass

    server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    monkeypatch.setenv("DRAGONFLY_PROXY", f"http://127.0.0.1:{server.server_port}")
    # The code under test changes these globals: let monkeypatch restore them.
    monkeypatch.setenv("HF_HUB_DISABLE_XET", "0")
    monkeypatch.setattr(
        huggingface_hub.constants,
        "HF_HUB_ENABLE_HF_TRANSFER",
        huggingface_hub.constants.HF_HUB_ENABLE_HF_TRANSFER,
    )
    yield received
    server.shutdown()
    huggingface_hub.configure_http_backend(_http._default_backend_factory)


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_through_dragonfly(mock_snapshot_download, dragonfly_proxy):
    from huggingface_hub import constants
    from huggingface_hub.utils import get_session

    Storage.download("hf://example.com/model")

    mock_snapshot_download.assert_called_once()
    assert os.environ["HF_HUB_DISABLE_XET"] == "1"
    assert constants.HF_HUB_ENABLE_HF_TRANSFER is False

    cdn_url = "https://cdn-lfs.hf.co/repos/ab/cd/blob?Expires=1&Signature=x"
    response = get_session().get(cdn_url, headers={"authorization": "Bearer hf_x"})
    assert response.content == b"ok"
    path, headers = dragonfly_proxy[0]
    assert path == cdn_url.replace("https://", "http://", 1)
    assert "authorization" not in {k.lower() for k in headers}


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_dragonfly_skips_hub_api(
    mock_snapshot_download, dragonfly_proxy
):
    from huggingface_hub import constants
    from huggingface_hub.utils import get_session

    Storage.download("hf://example.com/model")

    hub_url = f"{constants.ENDPOINT}/api/models/example.com/model"
    response = requests.Response()
    response.status_code = 200
    with mock.patch(
        "requests.adapters.HTTPAdapter.send", return_value=response
    ) as send:
        get_session().get(hub_url, headers={"authorization": "Bearer hf_x"})
    request = send.call_args.args[0]
    assert request.url == hub_url
    assert request.headers["authorization"] == "Bearer hf_x"
    assert dragonfly_proxy == []


@pytest.mark.parametrize("proxy", ["https://127.0.0.1:4001", "127.0.0.1:4001"])
def test_download_model_dragonfly_rejects_invalid_proxy(monkeypatch, proxy):
    monkeypatch.setenv("DRAGONFLY_PROXY", proxy)
    with mock.patch("huggingface_hub.snapshot_download") as mock_snapshot_download:
        with pytest.raises(ValueError, match="DRAGONFLY_PROXY"):
            Storage._download_hf("hf://example.com/model", "/tmp/out")
    mock_snapshot_download.assert_not_called()


@mock.patch("huggingface_hub.configure_http_backend")
@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_without_dragonfly(
    mock_snapshot_download, mock_configure_http_backend, monkeypatch
):
    monkeypatch.delenv("DRAGONFLY_PROXY", raising=False)
    Storage._download_hf("hf://example.com/model", "/tmp/out")
    mock_configure_http_backend.assert_not_called()
