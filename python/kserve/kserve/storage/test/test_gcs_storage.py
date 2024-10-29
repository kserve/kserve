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

import unittest.mock as mock
import pytest
from kserve.storage import Storage

STORAGE_MODULE = "kserve.storage.storage"


def get_call_args(call_args_list):
    arg_list = []
    for call in call_args_list:
        args, _ = call
        arg_list.append(args)
    return arg_list


def create_mock_dir(name):
    mock_dir = mock.MagicMock()
    mock_dir.name = name
    return mock_dir


def create_mock_dir_with_file(dir_name, file_name):
    mock_obj = mock.MagicMock()
    mock_obj.name = f"{dir_name}/{file_name}"
    return mock_obj


@mock.patch("google.cloud.storage.Client")
def test_gcs_with_empty_dir(mock_client):
    gcs_path = "gs://foo/bar"
    mock_bucket = mock.MagicMock()

    mock_bucket.list_blobs().__iter__.return_value = [create_mock_dir("bar/")]
    mock_client.return_value.bucket.return_value = mock_bucket

    with pytest.raises(Exception):
        Storage.download(gcs_path)


@mock.patch("google.cloud.storage.Client")
def test_mock_gcs(mock_client):
    gcs_path = "gs://foo/bar"
    mock_root_dir = create_mock_dir("bar/")
    mock_file = create_mock_dir_with_file("bar", "mock.object")

    mock_bucket = mock.MagicMock()
    mock_bucket.list_blobs().__iter__.return_value = [mock_root_dir, mock_file]
    mock_client.return_value.bucket.return_value = mock_bucket
    assert Storage.download(gcs_path)


@mock.patch("google.cloud.storage.Client")
def test_gcs_with_nested_sub_dir(mock_client):
    gcs_path = "gs://foo/bar/test"

    mock_root_dir = create_mock_dir("bar/")
    mock_sub_dir = create_mock_dir("test/")
    mock_file = create_mock_dir_with_file("test", "mock.object")

    mock_bucket = mock.MagicMock()
    mock_bucket.list_blobs().__iter__.return_value = [
        mock_root_dir,
        mock_sub_dir,
        mock_file,
    ]
    mock_client.return_value.bucket.return_value = mock_bucket

    Storage.download(gcs_path)

    arg_list = get_call_args(mock_file.download_to_filename.call_args_list)
    assert "test/mock.object" in arg_list[0][0]


@mock.patch("google.cloud.storage.Client")
def test_download_model_from_gcs(mock_client):
    gcs_path = "gs://foo/bar"

    mock_dir = create_mock_dir("bar/")
    mock_file = create_mock_dir_with_file("bar", "mock.object")

    mock_bucket = mock.MagicMock()
    mock_bucket.list_blobs().__iter__.return_value = [
        mock_dir,
        mock_file,
    ]
    mock_client.return_value.bucket.return_value = mock_bucket

    Storage.download(gcs_path)

    arg_list = get_call_args(mock_file.download_to_filename.call_args_list)
    assert "/mock.object" in arg_list[0][0]


@mock.patch("google.cloud.storage.Client")
def test_download_model_from_gcs_as_single_file(mock_client):
    gcs_path = "gs://foo/bar/mock.object"
    mock_file = create_mock_dir_with_file("bar", "mock.object")

    mock_bucket = mock.MagicMock()
    mock_bucket.blob.return_value = mock_file
    mock_file.exists.return_value = True
    mock_client.return_value.bucket.return_value = mock_bucket

    Storage.download(gcs_path)
    arg_list = get_call_args(mock_file.download_to_filename.call_args_list)

    assert "/mock.object" in arg_list[0][0]


@mock.patch("os.remove")
@mock.patch("os.mkdir")
@mock.patch("zipfile.ZipFile")
@mock.patch("google.cloud.storage.Client")
def test_gcs_model_unpack_archive_file(
    mock_client, MockZipFile, mock_create, mock_remove
):
    gcs_path = "gs://foo/bar"
    output_dir = "test/out_dir"

    mock_dir = create_mock_dir("bar/")
    mock_file = create_mock_dir_with_file("bar", "mock.zip")
    MockZipFile.return_value = mock_file

    mock_bucket = mock.MagicMock()
    mock_bucket.list_blobs().__iter__.return_value = [
        mock_dir,
        mock_file,
    ]
    mock_client.return_value.bucket.return_value = mock_bucket

    Storage.download(gcs_path, output_dir)

    download_arg_list = get_call_args(mock_file.download_to_filename.call_args_list)

    extract_arg_list = get_call_args(mock_file.extractall.call_args_list)

    assert "/mock.zip" in download_arg_list[0][0]
    assert output_dir == extract_arg_list[0][0]
    assert mock_file.close.called
    assert mock_remove.called
