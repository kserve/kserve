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
from types import SimpleNamespace

import pytest
from huggingface_hub.utils import disable_progress_bars, enable_progress_bars

from kserve_storage import Storage
from kserve_storage.huggingface_progress import (
    HuggingFaceLogProgress,
    create_huggingface_log_progress,
    get_huggingface_repo_size_bytes,
)

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


@pytest.fixture(autouse=True)
def non_tty_stderr():
    with (
        mock.patch(
            "kserve_storage.kserve_storage.sys.stderr.isatty", return_value=False
        ),
        mock.patch(
            "kserve_storage.huggingface_progress.get_huggingface_repo_size_bytes",
            return_value=None,
        ),
    ):
        yield


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
        tqdm_class=mock.ANY,
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
        tqdm_class=mock.ANY,
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
        tqdm_class=mock.ANY,
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
        tqdm_class=mock.ANY,
    )


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_no_patterns_omits_kwargs(mock_snapshot_download):
    uri = "hf://example.com/model"

    Storage._download_hf(uri, "/tmp/out")

    mock_snapshot_download.assert_called_once_with(
        repo_id="example.com/model",
        revision=None,
        local_dir="/tmp/out",
        tqdm_class=mock.ANY,
    )

    progress_class = mock_snapshot_download.call_args.kwargs["tqdm_class"]
    assert issubclass(progress_class, HuggingFaceLogProgress)
    with mock.patch.object(progress_class, "_start_heartbeat"):
        progress = progress_class(total=1)
    assert progress.local_dir == "/tmp/out"
    progress.close()


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_binds_total_size_to_progress(mock_snapshot_download):
    with mock.patch(
        "kserve_storage.huggingface_progress.get_huggingface_repo_size_bytes",
        return_value=16_020_000_000,
    ):
        Storage._download_hf("hf://example.com/model", "/tmp/out")

    progress_class = mock_snapshot_download.call_args.kwargs["tqdm_class"]
    with mock.patch.object(progress_class, "_start_heartbeat"):
        progress = progress_class(total=9)

    assert progress.total_size_bytes == 16_020_000_000
    progress.close()


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_in_tty_keeps_default_progress(mock_snapshot_download):
    with mock.patch(
        "kserve_storage.kserve_storage.sys.stderr.isatty", return_value=True
    ):
        Storage._download_hf("hf://example.com/model", "/tmp/out")

    assert "tqdm_class" not in mock_snapshot_download.call_args.kwargs


def test_hf_log_progress_emits_line_oriented_heartbeat(caplog):
    progress_class = create_huggingface_log_progress(
        "/tmp/models", total_size_bytes=59_550_000_000
    )
    with mock.patch.object(progress_class, "_start_heartbeat"):
        progress = progress_class(total=2)

    progress.n = 1
    with (
        mock.patch(
            "kserve_storage.huggingface_progress._get_local_size_bytes",
            return_value=42_730_000_000,
        ),
        mock.patch.object(
            progress._heartbeat_stop,
            "wait",
            side_effect=[False, True],
        ),
        caplog.at_level("INFO", logger="storage.initializer"),
    ):
        progress._heartbeat()

    progress.close()

    heartbeat = next(
        record.message
        for record in caplog.records
        if record.message.startswith("HF progress:")
        and "42.73/59.55GB" in record.message
    )
    assert "(71.8%)" in heartbeat
    assert "1/2 files" in heartbeat
    assert heartbeat.endswith("1/2 files")
    assert "\r" not in heartbeat


def test_hf_log_progress_emits_completion(caplog):
    progress_class = create_huggingface_log_progress(
        "/tmp/models", total_size_bytes=59_550_000_000
    )
    with (
        mock.patch.object(progress_class, "_start_heartbeat"),
        mock.patch(
            "kserve_storage.huggingface_progress._get_local_size_bytes",
            return_value=59_550_000_000,
        ),
        caplog.at_level("INFO", logger="storage.initializer"),
    ):
        progress = progress_class(total=2)
        progress.update(2)
        progress.close()

    completion_logs = [
        record.message
        for record in caplog.records
        if record.message.endswith(", complete")
    ]
    assert len(completion_logs) == 1
    assert "59.55/59.55GB (100.0%)" in completion_logs[0]
    assert "2/2 files" in completion_logs[0]


def test_hf_log_progress_falls_back_when_total_size_is_unknown(caplog):
    progress_class = create_huggingface_log_progress("/tmp/models")
    with (
        mock.patch.object(progress_class, "_start_heartbeat"),
        mock.patch(
            "kserve_storage.huggingface_progress._get_local_size_bytes",
            return_value=42_730_000_000,
        ),
        caplog.at_level("INFO", logger="storage.initializer"),
    ):
        progress = progress_class(total=2)
        progress.close()

    completion = next(
        record.message
        for record in caplog.records
        if record.message.endswith(", stopped")
    )
    assert "HF progress: 42.73GB" in completion
    assert "%" not in completion


@mock.patch("huggingface_hub.HfApi")
def test_get_huggingface_repo_size_bytes_applies_patterns(mock_hf_api):
    mock_hf_api.return_value.model_info.return_value.siblings = [
        SimpleNamespace(rfilename="config.json", size=1_000),
        SimpleNamespace(rfilename="model-00001.safetensors", size=4_000),
        SimpleNamespace(rfilename="model-00002.safetensors", size=5_000),
        SimpleNamespace(rfilename="pytorch_model.bin", size=6_000),
    ]

    total_size = get_huggingface_repo_size_bytes(
        "example/model",
        revision="main",
        allow_patterns=["*.json", "*.safetensors"],
        ignore_patterns=["model-00002.safetensors"],
    )

    assert total_size == 5_000
    mock_hf_api.return_value.model_info.assert_called_once_with(
        repo_id="example/model",
        revision="main",
        files_metadata=True,
    )


@mock.patch("huggingface_hub.HfApi")
def test_get_huggingface_repo_size_bytes_returns_none_on_failure(mock_hf_api):
    mock_hf_api.return_value.model_info.side_effect = RuntimeError("metadata failed")

    assert get_huggingface_repo_size_bytes("example/model") is None


def test_hf_log_progress_respects_disabled_progress_bars():
    disable_progress_bars()
    try:
        with mock.patch.object(HuggingFaceLogProgress, "_start_heartbeat") as start:
            progress = HuggingFaceLogProgress(total=2)

        assert progress.disable
        start.assert_not_called()
        progress.close()
    finally:
        enable_progress_bars()


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
