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

import unittest.mock as mock
import pytest
import shutil

from kserve_storage import Storage

STORAGE_MODULE = "kserve_storage.kserve_storage"


def create_mock_item(path):
    mock_obj = mock.MagicMock()
    mock_obj.name = path
    mock_obj.readall.return_value = b"test"
    return mock_obj


def create_mock_blob(mock_storage, paths):
    mock_objs = [create_mock_item(path) for path in paths]
    mock_container = mock.MagicMock()
    mock_container.walk_blobs.return_value = mock_objs
    mock_container.list_blobs.return_value = mock_objs
    mock_container.download_blob.return_value = mock_objs[0]
    mock_svc = mock.MagicMock()
    mock_svc.get_container_client.return_value = mock_container
    mock_storage.return_value = mock_svc
    return mock_storage, mock_container


def create_mock_dir(name):
    mock_file = mock.MagicMock()
    mock_file.name = name
    mock_file.is_directory = True
    return mock_file


def create_mock_file(name):
    mock_file = mock.MagicMock()
    mock_file.name = name
    mock_file.is_directory = False
    return mock_file


def create_mock_objects_for_file_share(mock_storage, mock_file_items):
    mock_share = mock.MagicMock()
    mock_share.list_directories_and_files.side_effect = mock_file_items
    mock_file = mock.MagicMock()
    mock_share.get_file_client.return_value = mock_file
    mock_data = mock.MagicMock()
    mock_file.download_file.return_value = mock_data
    mock_svc = mock.MagicMock()
    mock_svc.get_share_client.return_value = mock_share
    mock_storage.return_value = mock_svc
    return mock_storage, mock_share, mock_data


def get_call_args(call_args_list):
    arg_list = []
    for call in call_args_list:
        args, _ = call
        arg_list.append(args)
    return arg_list


# pylint: disable=protected-access
@pytest.fixture(scope="session", autouse=True)
def test_cleanup():
    yield None
    # Will be executed after the last test
    shutil.rmtree("some", ignore_errors=True)
    shutil.rmtree("dest_path", ignore_errors=True)


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch("azure.storage.blob.BlobServiceClient")
def test_blob(mock_storage, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = "https://kfserving.blob.core.windows.net/triton/simple_string/"
    paths = ["simple_string/1/model.graphdef", "simple_string/config.pbtxt"]
    mock_blob, mock_container = create_mock_blob(mock_storage, paths)

    # when
    Storage._download_azure_blob(blob_path, "dest_path")

    # then
    arg_list = get_call_args(mock_container.download_blob.call_args_list)
    assert set(arg_list) == set(
        [("simple_string/1/model.graphdef",), ("simple_string/config.pbtxt",)]
    )

    mock_storage.assert_called_with(
        "https://kfserving.blob.core.windows.net", credential=None
    )


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch("azure.storage.blob.BlobServiceClient")
def test_blob_file_direct(
    mock_storage, mock_makedirs
):  # pylint: disable=unused-argument

    # given
    blob_path = "https://accountname.blob.core.windows.net/container/somefile.text"
    paths = ["somefile.text"]
    mock_blob, mock_container = create_mock_blob(mock_storage, paths)

    # when
    Storage._download_azure_blob(blob_path, "dest_path")

    # then
    arg_list = get_call_args(mock_container.download_blob.call_args_list)
    assert arg_list == [("somefile.text",)]
    mock_storage.assert_called_with(
        "https://accountname.blob.core.windows.net", credential=None
    )


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch(STORAGE_MODULE + ".Storage._get_azure_storage_token")
@mock.patch("azure.storage.blob.BlobServiceClient")
def test_secure_blob(
    mock_storage, mock_get_token, mock_makedirs
):  # pylint: disable=unused-argument

    # given
    blob_path = "https://kfsecured.blob.core.windows.net/triton/simple_string/"
    mock_get_token.return_value = "some_token"

    # when
    with pytest.raises(RuntimeError):
        Storage._download_azure_blob(blob_path, "dest_path")

    # then
    mock_get_token.assert_called()
    arg_list = []
    for call in mock_storage.call_args_list:
        _, kwargs = call
        arg_list.append(kwargs)
    assert arg_list == [{"credential": "some_token"}]


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch("azure.storage.blob.BlobServiceClient")
def test_deep_blob(mock_storage, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = (
        "https://accountname.blob.core.windows.net/container/some/deep/blob/path"
    )
    paths = ["f1", "f2", "d1/f11", "d1/d2/f21", "d1/d2/d3/f1231", "d4/f41"]
    fq_item_paths = ["some/deep/blob/path/" + p for p in paths]
    expected_calls = [(f,) for f in fq_item_paths]

    # when
    mock_blob, mock_container = create_mock_blob(mock_storage, fq_item_paths)
    try:
        Storage._download_azure_blob(blob_path, "some/dest/path")
    except OSError:  # Permissions Error Handling
        pass

    # then
    actual_calls = get_call_args(mock_container.download_blob.call_args_list)
    assert set(actual_calls) == set(expected_calls)


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch("azure.storage.blob.BlobServiceClient")
def test_blob_file(mock_storage, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = "https://accountname.blob.core.windows.net/container/somefile.text"
    paths = ["somefile"]
    fq_item_paths = paths
    expected_calls = [(f,) for f in fq_item_paths]

    # when
    mock_blob, mock_container = create_mock_blob(mock_storage, paths)
    Storage._download_azure_blob(blob_path, "some/dest/path")

    # then
    actual_calls = get_call_args(mock_container.download_blob.call_args_list)
    assert actual_calls == expected_calls


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch("azure.storage.blob.BlobServiceClient")
def test_blob_fq_file(mock_storage, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = (
        "https://accountname.blob.core.windows.net/container/folder/somefile.text"
    )
    paths = ["somefile"]
    fq_item_paths = ["folder/" + p for p in paths]
    expected_calls = [(f,) for f in fq_item_paths]

    # when
    mock_blob, mock_container = create_mock_blob(mock_storage, fq_item_paths)
    Storage._download_azure_blob(blob_path, "some/dest/path")

    # then
    actual_calls = get_call_args(mock_container.download_blob.call_args_list)
    assert actual_calls == expected_calls


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch("azure.storage.blob.BlobServiceClient")
def test_blob_no_prefix(mock_storage, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = "https://accountname.blob.core.windows.net/container/"
    paths = ["somefile.text", "somefolder/somefile.text"]
    fq_item_paths = ["" + p for p in paths]
    expected_calls = [(f,) for f in fq_item_paths]

    # when
    mock_blob, mock_container = create_mock_blob(mock_storage, fq_item_paths)
    Storage._download_azure_blob(blob_path, "some/dest/path")

    # then
    actual_calls = get_call_args(mock_container.download_blob.call_args_list)
    assert set(actual_calls) == set(expected_calls)


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch(STORAGE_MODULE + ".Storage._get_azure_storage_access_key")
@mock.patch("azure.storage.fileshare.ShareServiceClient")
def test_file_share(
    mock_storage, mock_get_access_key, mock_makedirs
):  # pylint: disable=unused-argument

    # given
    file_share_path = "https://kfserving.file.core.windows.net/triton/simple_string/"
    mock_get_access_key.return_value = "some_token"

    mock_file_share, mock_file, mock_data = create_mock_objects_for_file_share(
        mock_storage,
        [
            [create_mock_dir("1"), create_mock_file("config.pbtxt")],
            [create_mock_file("model.graphdef")],
            [],
        ],
    )

    # when
    Storage._download_azure_file_share(file_share_path, "dest_path")

    # then
    arg_list = get_call_args(mock_file.get_file_client.call_args_list)
    assert set(arg_list) == set(
        [("simple_string/1/model.graphdef",), ("simple_string/config.pbtxt",)]
    )

    # then
    mock_get_access_key.assert_called()
    mock_storage.assert_called_with(
        "https://kfserving.file.core.windows.net", credential="some_token"
    )


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch(STORAGE_MODULE + ".Storage._get_azure_storage_access_key")
@mock.patch("azure.storage.fileshare.ShareServiceClient")
def test_deep_file_share(
    mock_storage, mock_get_access_key, mock_makedirs
):  # pylint: disable=unused-argument

    file_share_path = (
        "https://accountname.file.core.windows.net/container/some/deep/blob/path"
    )
    paths = ["f1", "f2", "d1/f11", "d1/d2/f21", "d1/d2/d3/f1231", "d4/f41"]
    fq_item_paths = ["some/deep/blob/path/" + p for p in paths]
    expected_calls = [(f,) for f in fq_item_paths]
    mock_get_access_key.return_value = "some_token"

    # when
    mock_file_share, mock_file, mock_data = create_mock_objects_for_file_share(
        mock_storage,
        [
            [
                create_mock_dir("d1"),
                create_mock_dir("d4"),
                create_mock_file("f1"),
                create_mock_file("f2"),
            ],
            [create_mock_file("f41")],
            [create_mock_dir("d2"), create_mock_file("f11")],
            [create_mock_dir("d3"), create_mock_file("f21")],
            [create_mock_file("f1231")],
            [],
        ],
    )
    try:
        Storage._download_azure_file_share(file_share_path, "some/dest/path")
    except OSError:  # Permissions Error Handling
        pass

    # then
    actual_calls = get_call_args(mock_file.get_file_client.call_args_list)
    assert set(actual_calls) == set(expected_calls)

    # then
    mock_get_access_key.assert_called()
    mock_storage.assert_called_with(
        "https://accountname.file.core.windows.net", credential="some_token"
    )


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch(STORAGE_MODULE + ".Storage._get_azure_storage_access_key")
@mock.patch("azure.storage.fileshare.ShareServiceClient")
def test_file_share_fq_file(
    mock_storage, mock_get_access_key, mock_makedirs
):  # pylint: disable=unused-argument

    # given
    file_share_path = "https://accountname.file.core.windows.net/container/folder/"
    paths = ["somefile.text"]
    fq_item_paths = ["folder/" + p for p in paths]
    expected_calls = [(f,) for f in fq_item_paths]

    # when
    mock_get_access_key.return_value = "some_token"
    mock_file_share, mock_file, mock_data = create_mock_objects_for_file_share(
        mock_storage, [[create_mock_file("somefile.text")], []]
    )
    Storage._download_azure_file_share(file_share_path, "some/dest/path")

    # then
    actual_calls = get_call_args(mock_file.get_file_client.call_args_list)
    assert actual_calls == expected_calls

    # then
    mock_get_access_key.assert_called()
    mock_storage.assert_called_with(
        "https://accountname.file.core.windows.net", credential="some_token"
    )


@mock.patch(STORAGE_MODULE + ".os.makedirs")
@mock.patch(STORAGE_MODULE + ".Storage._get_azure_storage_access_key")
@mock.patch("azure.storage.fileshare.ShareServiceClient")
def test_file_share_no_prefix(
    mock_storage, mock_get_access_key, mock_makedirs
):  # pylint: disable=unused-argument

    # given
    file_share_path = "https://accountname.file.core.windows.net/container/"
    paths = ["somefile.text", "somefolder/somefile.text"]
    fq_item_paths = ["" + p for p in paths]
    expected_calls = [(f,) for f in fq_item_paths]

    # when
    mock_get_access_key.return_value = "some_token"
    mock_file_share, mock_file, mock_data = create_mock_objects_for_file_share(
        mock_storage,
        [
            [create_mock_dir("somefolder"), create_mock_file("somefile.text")],
            [create_mock_file("somefile.text")],
            [],
        ],
    )
    Storage._download_azure_file_share(file_share_path, "some/dest/path")

    # then
    arg_list = get_call_args(mock_file.get_file_client.call_args_list)
    assert set(arg_list) == set(expected_calls)

    # then
    mock_get_access_key.assert_called()
    mock_storage.assert_called_with(
        "https://accountname.file.core.windows.net", credential="some_token"
    )
