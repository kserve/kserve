# Copyright 2021 The KServe Authors.
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

import io
import os
import tempfile
import binascii
import unittest.mock as mock
import mimetypes
import subprocess
from pathlib import Path

import pytest

from kserve_storage import Storage

STORAGE_MODULE = "kserve_storage.kserve_storage"
HTTPS_URI_TARGZ = "https://foo.bar/model.tar.gz"
HTTPS_URI_TARGZ_WITH_QUERY = HTTPS_URI_TARGZ + "?foo=bar"

# *.tar.gz contains a single empty file model.pth
FILE_TAR_GZ_RAW = binascii.unhexlify(
    "1f8b0800bac550600003cbcd4f49cdd12b28c960a01d3030303033315100d1e666a660dac008c28"
    "701054313a090a189919981998281a1b1b1a1118382010ddd0407a5c525894540a754656466e464e"
    "2560754969686c71ca83fe0f4281805a360140c7200009f7e1bb400060000"
)
# *.zip contains a single empty file model.pth
FILE_ZIP_RAW = binascii.unhexlify(
    "504b030414000800080035b67052000000000000000000000000090020006d6f64656c2e70746855540"
    "d000786c5506086c5506086c5506075780b000104f501000004140000000300504b07080000000002000"
    "00000000000504b0102140314000800080035b6705200000000020000000000000009002000000000000"
    "0000000a481000000006d6f64656c2e70746855540d000786c5506086c5506086c5506075780b000104f"
    "50100000414000000504b0506000000000100010057000000590000000000"
)


def test_storage_local_path():
    abs_path = "file:///"
    relative_path = "file://."
    assert Storage.download(abs_path) == abs_path.replace("file://", "", 1)
    assert Storage.download(relative_path) == relative_path.replace("file://", "", 1)


def test_storage_local_path_exception():
    not_exist_path = "file:///some/random/path"
    with pytest.raises(Exception):
        Storage.download(not_exist_path)


def test_no_prefix_local_path():
    abs_path = "/"
    relative_path = "."
    assert Storage.download(abs_path) == abs_path
    assert Storage.download(relative_path) == relative_path


def test_local_path_with_out_dir_exist():
    abs_path = "file:///tmp"
    out_dir = "/tmp"
    assert Storage.download(abs_path, out_dir=out_dir) == out_dir


def test_local_path_with_out_dir_not_exist():
    abs_path = "file:///tmp"
    out_dir = "/tmp/test-abc"
    assert Storage.download(abs_path, out_dir=out_dir) == out_dir


class MockHttpResponse(object):
    def __init__(self, status_code=404, raw=b"", content_type=""):
        self.status_code = status_code
        self.raw = io.BytesIO(raw)
        self.headers = {"Content-Type": content_type}

    def __enter__(self):
        return self

    def __exit__(self, ex_type, ex_val, traceback):
        pass


@mock.patch(
    "requests.get",
    return_value=MockHttpResponse(
        status_code=200, content_type="application/octet-stream"
    ),
)
def test_http_uri_path(_):
    http_uri = "http://foo.bar/model.joblib"
    http_with_query_uri = "http://foo.bar/model.joblib?foo=bar"
    out_dir = "."
    assert Storage.download(http_uri, out_dir=out_dir) == out_dir
    assert Storage.download(http_with_query_uri, out_dir=out_dir) == out_dir
    os.remove("./model.joblib")


@mock.patch(
    "requests.get",
    return_value=MockHttpResponse(
        status_code=200, content_type="application/octet-stream"
    ),
)
def test_https_uri_path(_):
    https_uri = "https://foo.bar/model.joblib"
    https_with_query_uri = "https://foo.bar/model.joblib?foo=bar"
    out_dir = "."
    assert Storage.download(https_uri, out_dir=out_dir) == out_dir
    assert Storage.download(https_with_query_uri, out_dir=out_dir) == out_dir
    os.remove("./model.joblib")


http_uri_path_testparams = [
    (
        HTTPS_URI_TARGZ,
        MockHttpResponse(200, FILE_TAR_GZ_RAW, "application/x-tar"),
        None,
    ),
    (
        HTTPS_URI_TARGZ,
        MockHttpResponse(200, FILE_TAR_GZ_RAW, "application/x-gtar"),
        None,
    ),
    (
        HTTPS_URI_TARGZ,
        MockHttpResponse(200, FILE_TAR_GZ_RAW, "application/x-gzip"),
        None,
    ),
    (HTTPS_URI_TARGZ, MockHttpResponse(200, FILE_TAR_GZ_RAW, "application/gzip"), None),
    (
        HTTPS_URI_TARGZ,
        MockHttpResponse(200, FILE_TAR_GZ_RAW, "application/zip"),
        RuntimeError,
    ),
    (
        HTTPS_URI_TARGZ_WITH_QUERY,
        MockHttpResponse(200, FILE_TAR_GZ_RAW, "application/x-tar"),
        None,
    ),
    (
        HTTPS_URI_TARGZ_WITH_QUERY,
        MockHttpResponse(200, FILE_TAR_GZ_RAW, "application/x-gtar"),
        None,
    ),
    (
        HTTPS_URI_TARGZ_WITH_QUERY,
        MockHttpResponse(200, FILE_TAR_GZ_RAW, "application/x-gzip"),
        None,
    ),
    (
        HTTPS_URI_TARGZ_WITH_QUERY,
        MockHttpResponse(200, FILE_TAR_GZ_RAW, "application/gzip"),
        None,
    ),
    (
        "https://foo.bar/model.zip",
        MockHttpResponse(200, FILE_ZIP_RAW, "application/zip"),
        None,
    ),
    (
        "https://foo.bar/model.zip",
        MockHttpResponse(200, FILE_ZIP_RAW, "application/x-zip-compressed"),
        None,
    ),
    (
        "https://foo.bar/model.zip",
        MockHttpResponse(200, FILE_ZIP_RAW, "application/zip-compressed"),
        None,
    ),
    (
        "https://foo.bar/model.zip?foo=bar",
        MockHttpResponse(200, FILE_ZIP_RAW, "application/zip"),
        None,
    ),
    (
        "https://foo.bar/model.zip?foo=bar",
        MockHttpResponse(200, FILE_ZIP_RAW, "application/x-zip-compressed"),
        None,
    ),
    (
        "https://foo.bar/model.zip?foo=bar",
        MockHttpResponse(200, FILE_ZIP_RAW, "application/zip-compressed"),
        None,
    ),
    ("https://theabyss.net/model.joblib", MockHttpResponse(404), RuntimeError),
    (
        "https://some.site.com/test.model",
        MockHttpResponse(status_code=200, content_type="text/html"),
        RuntimeError,
    ),
    ("https://foo.bar/test/", MockHttpResponse(200), ValueError),
    (
        "https://foo.bar/download?path=/20210530/model.zip",
        MockHttpResponse(200, FILE_ZIP_RAW, "application/zip"),
        None,
    ),
    (
        "https://foo.bar/download?path=/20210530/model.zip",
        MockHttpResponse(200, FILE_ZIP_RAW, "application/x-zip" "-compressed"),
        None,
    ),
    (
        "https://foo.bar/download?path=/20210530/model.zip",
        MockHttpResponse(200, FILE_ZIP_RAW, "application/zip" "-compressed"),
        None,
    ),
]


@pytest.mark.parametrize("uri,response,expected_error", http_uri_path_testparams)
def test_http_uri_paths(uri, response, expected_error):
    if expected_error:

        def test(_):
            with pytest.raises(expected_error):
                Storage.download(uri)

    else:

        def test(_):
            with tempfile.TemporaryDirectory() as out_dir:
                assert Storage.download(uri, out_dir=out_dir) == out_dir
                assert os.path.exists(os.path.join(out_dir, "model.pth"))

    mock.patch("requests.get", return_value=response)(test)()


def test_storage_blob_exception():
    blob_path = "https://accountname.blob.core.windows.net/container/some/blob/"
    with pytest.raises(Exception):
        Storage.download(blob_path)


def test_unpack_tar_file():
    out_dir = "."
    tar_file = os.path.join(out_dir, "model.tgz")
    Path(tar_file).write_bytes(FILE_TAR_GZ_RAW)
    mimetype, _ = mimetypes.guess_type(tar_file)
    Storage._unpack_archive_file(tar_file, mimetype, out_dir)
    assert os.path.exists(os.path.join(out_dir, "model.pth"))
    os.remove(os.path.join(out_dir, "model.pth"))


def test_unpack_zip_file():
    out_dir = "."
    tar_file = os.path.join(out_dir, "model.zip")
    Path(tar_file).write_bytes(FILE_ZIP_RAW)
    mimetype, _ = mimetypes.guess_type(tar_file)
    Storage._unpack_archive_file(tar_file, mimetype, out_dir)
    assert os.path.exists(os.path.join(out_dir, "model.pth"))
    os.remove(os.path.join(out_dir, "model.pth"))


@mock.patch(STORAGE_MODULE + ".Storage._download_azure_blob")
def test_download_azure_blob_called_with_matching_uri(mock_download_azure_blob):
    azure_blob_uris = [
        "https://accountname.blob.core.windows.net/container/some/blob/",
        "https://accountname.z20.blob.storage.azure.net/container/some/blob/",
        "https://accountname.z2.blob.storage.azure.net/container/some/blob/",
    ]

    for uri in azure_blob_uris:
        Storage.download(uri, out_dir="dest_path")

    expected_calls = [mock.call(uri, "dest_path") for uri in azure_blob_uris]
    mock_download_azure_blob.assert_has_calls(expected_calls)


@mock.patch(STORAGE_MODULE + ".Storage._download_azure_file_share")
def test_download_azure_file_share_called_with_matching_uri(
    mock_download_azure_file_share,
):
    azure_file_uris = [
        "https://accountname.file.core.windows.net/container/some/blob",
        "https://accountname.z20.file.storage.azure.net/container/some/blob",
        "https://accountname.z2.file.storage.azure.net/container/some/blob",
    ]

    for uri in azure_file_uris:
        Storage.download(uri, out_dir="dest_path")

    expected_calls = [mock.call(uri, "dest_path") for uri in azure_file_uris]
    mock_download_azure_file_share.assert_has_calls(expected_calls)


# Git repository download tests
git_repo_test_params = [
    # (uri, use_ssh_key, expected_ssh_command_suffix)
    ("https://github.com/user/repo.git", False, ""),
    ("ssh://git@github.com/user/repo.git", False, ""),
    ("git@github.com:kserve/kserve.git", False, ""),
    ("git@bitbucket.org:user/repo.git", True, " -i {ssh_key_path}"),
]


@pytest.mark.parametrize("uri,use_ssh_key,expected_ssh_suffix", git_repo_test_params)
@mock.patch("subprocess.run")
def test_git_repo_download_success(
    mock_subprocess_run, uri, use_ssh_key, expected_ssh_suffix
):
    """Test successful git repository downloads with various URI formats and SSH configurations."""
    out_dir = "/tmp/test_model"

    ssh_key_file = tempfile.NamedTemporaryFile()
    ssh_key_file_name = None

    if use_ssh_key:
        ssh_key_file_name = ssh_key_file.name
        expected_ssh_suffix = expected_ssh_suffix.format(ssh_key_path=ssh_key_file.name)

    with mock.patch(STORAGE_MODULE + "._GIT_SSH_KEY_PATH", ssh_key_file_name):
        result = Storage.download(uri, out_dir=out_dir)

    assert result == out_dir

    # Verify subprocess.run was called with correct arguments
    expected_cmd = ["git", "clone", "--depth", "1", uri, out_dir]
    mock_subprocess_run.assert_called_once()

    call_args = mock_subprocess_run.call_args
    assert call_args[0][0] == expected_cmd
    assert call_args[1]["check"] is True

    # Verify environment contains correct GIT_SSH_COMMAND
    env = call_args[1]["env"]
    expected_ssh_command = (
        "ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
        + expected_ssh_suffix
    )
    assert env["GIT_SSH_COMMAND"] == expected_ssh_command


git_error_test_params = [
    # (error_type, error_args, expected_message_part)
    (
        subprocess.CalledProcessError,
        {
            "returncode": 128,
            "cmd": ["git", "clone"],
            "stderr": "fatal: repository not found",
        },
        "fatal: repository not found",
    ),
    (
        subprocess.CalledProcessError,
        {"returncode": 1, "cmd": ["git", "clone"], "stderr": None},
        "Command '['git', 'clone']' returned non-zero exit status 1",
    ),
    (OSError, ("Permission denied",), "Permission denied"),
]


@pytest.mark.parametrize(
    "error_type,error_args,expected_message_part", git_error_test_params
)
@mock.patch("subprocess.run")
def test_git_repo_download_errors(
    mock_subprocess_run, error_type, error_args, expected_message_part
):
    """Test git clone error handling scenarios via main download() entry point."""
    uri = "https://github.com/user/nonexistent.git"
    out_dir = "/tmp/test_model"

    # Setup subprocess to raise the specified error
    if isinstance(error_args, dict):
        mock_subprocess_run.side_effect = error_type(**error_args)
    else:
        mock_subprocess_run.side_effect = error_type(*error_args)

    with pytest.raises(RuntimeError) as exc_info:
        Storage.download(uri, out_dir=out_dir)

    # Verify error message contains expected content
    assert f"git clone {uri} failed:" in str(exc_info.value)
    assert expected_message_part in str(exc_info.value)


@mock.patch("subprocess.run")
@mock.patch(STORAGE_MODULE + ".logger.error")
def test_git_repo_download_nonexistent_ssh_key(mock_logger_error, mock_subprocess_run):
    """Test git clone when SSH key path is specified but doesn't exist."""
    uri = "git@github.com:user/repo.git"
    out_dir = "/tmp/test_model"
    nonexistent_path = "/nonexistent/ssh_key"

    # Patch the global variable directly
    with mock.patch(STORAGE_MODULE + "._GIT_SSH_KEY_PATH", nonexistent_path):
        result = Storage.download(uri, out_dir=out_dir)

    # Verify it still succeeds but logs error about missing key
    assert result == out_dir
    mock_logger_error.assert_called_with(
        f"GIT_SSH_KEY_PATH {nonexistent_path} does not exist; ignoring"
    )

    # Verify SSH command doesn't include the nonexistent key
    call_args = mock_subprocess_run.call_args
    env = call_args[1]["env"]
    expected_ssh_cmd = "ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
    assert env["GIT_SSH_COMMAND"] == expected_ssh_cmd
