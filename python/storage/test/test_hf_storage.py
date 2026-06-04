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
import pytest

from kserve_storage import Storage


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
        etag_timeout=30,
    )


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model_with_allow_patterns(mock_snapshot_download):
    uri = "hf://example.com/model"

    Storage._download_hf(uri, "/tmp/out", allow_patterns=["*.safetensors", "*.json"])

    mock_snapshot_download.assert_called_once_with(
        repo_id="example.com/model",
        revision=None,
        local_dir="/tmp/out",
        etag_timeout=30,
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
        etag_timeout=30,
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
        etag_timeout=30,
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
        etag_timeout=30,
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
def test_custom_etag_timeout_from_env(mock_snapshot_download):
    uri = "hf://example.com/model"

    with mock.patch.dict(os.environ, {"HF_HUB_ETAG_TIMEOUT": "120"}):
        Storage._download_hf(uri, "/tmp/out")

    call_kwargs = mock_snapshot_download.call_args[1]
    assert call_kwargs.get("etag_timeout") == 120


@mock.patch("huggingface_hub.snapshot_download")
@mock.patch("kserve_storage.kserve_storage.time.sleep")
def test_retry_on_transient_failure(mock_sleep, mock_snapshot_download):
    mock_snapshot_download.side_effect = [
        ConnectionError("stalled"),
        ConnectionError("stalled again"),
        None,
    ]
    uri = "hf://example.com/model"

    Storage._download_hf(uri, "/tmp/out")

    assert mock_snapshot_download.call_count == 3
    assert mock_sleep.call_count == 2


@mock.patch("huggingface_hub.snapshot_download")
@mock.patch("kserve_storage.kserve_storage.time.sleep")
def test_retry_exhaustion_raises(mock_sleep, mock_snapshot_download):
    mock_snapshot_download.side_effect = ConnectionError("stalled")
    uri = "hf://example.com/model"

    with pytest.raises(ConnectionError, match="stalled"):
        Storage._download_hf(uri, "/tmp/out")

    assert mock_snapshot_download.call_count == 3


@mock.patch("huggingface_hub.snapshot_download")
@mock.patch("kserve_storage.kserve_storage.time.sleep")
def test_custom_retry_count_from_env(mock_sleep, mock_snapshot_download):
    mock_snapshot_download.side_effect = ConnectionError("stalled")
    uri = "hf://example.com/model"

    with mock.patch.dict(os.environ, {"HF_HUB_DOWNLOAD_RETRIES": "5"}):
        with pytest.raises(ConnectionError):
            Storage._download_hf(uri, "/tmp/out")

    assert mock_snapshot_download.call_count == 5


@mock.patch("huggingface_hub.snapshot_download")
def test_no_retry_on_repository_not_found(mock_snapshot_download):
    from huggingface_hub.utils import RepositoryNotFoundError

    mock_snapshot_download.side_effect = RepositoryNotFoundError("not found")
    uri = "hf://example.com/model"

    with pytest.raises(Exception):
        Storage._download_hf(uri, "/tmp/out")

    assert mock_snapshot_download.call_count == 1


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
