# Copyright 2020 kubeflow.org.
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
import kfserving
import shutil


def create_mock_item(path):
    mock_obj = mock.MagicMock()
    mock_obj.name = path
    mock_obj.readall.return_value = b"test"
    return mock_obj


def create_mock_blob(mock_storage, paths):
    mock_objs = [create_mock_item(path) for path in paths]
    mock_container = mock.MagicMock()
    mock_container.list_blobs.return_value = mock_objs
    mock_container.download_blob.return_value = mock_objs[0]
    mock_svc = mock.MagicMock()
    mock_svc.get_container_client.return_value = mock_container
    mock_storage.return_value = mock_svc
    return mock_storage, mock_container


def get_call_args(call_args_list):
    arg_list = []
    for call in call_args_list:
        args, _ = call
        arg_list.append(args)
    return arg_list


# pylint: disable=protected-access
@pytest.fixture(scope='session', autouse=True)
def test_cleanup():
    yield None
    # Will be executed after the last test
    shutil.rmtree('some', ignore_errors=True)
    shutil.rmtree('dest_path', ignore_errors=True)


@mock.patch('kfserving.storage.os.makedirs')
@mock.patch('kfserving.storage.BlobServiceClient')
def test_blob(mock_storage, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = 'https://kfserving.blob.core.windows.net/triton/simple_string/'
    paths = ['simple_string/1/model.graphdef', 'simple_string/config.pbtxt']
    mock_blob, mock_container = create_mock_blob(mock_storage, paths)

    # when
    kfserving.Storage._download_blob(blob_path, "dest_path")

    # then
    arg_list = get_call_args(mock_container.download_blob.call_args_list)
    assert arg_list == [('simple_string/1/model.graphdef',),
                        ('simple_string/config.pbtxt',)]

    mock_storage.assert_called_with('https://kfserving.blob.core.windows.net',
                                    credential=None)


@mock.patch('kfserving.storage.os.makedirs')
@mock.patch('kfserving.storage.Storage._get_azure_storage_token')
@mock.patch('kfserving.storage.BlobServiceClient')
def test_secure_blob(mock_storage, mock_get_token, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = 'https://kfsecured.blob.core.windows.net/triton/simple_string/'
    mock_get_token.return_value = "some_token"

    # when
    with pytest.raises(RuntimeError):
        kfserving.Storage._download_blob(blob_path, "dest_path")

    # then
    mock_get_token.assert_called()
    arg_list = []
    for call in mock_storage.call_args_list:
        _, kwargs = call
        arg_list.append(kwargs)
    assert arg_list == [{'credential': 'some_token'}]


@mock.patch('kfserving.storage.os.makedirs')
@mock.patch('kfserving.storage.BlobServiceClient')
def test_deep_blob(mock_storage, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = 'https://accountname.blob.core.windows.net/container/some/deep/blob/path'
    paths = ['f1', 'f2', 'd1/f11', 'd1/d2/f21', 'd1/d2/d3/f1231', 'd4/f41']
    fq_item_paths = ['some/deep/blob/path/' + p for p in paths]
    expected_calls = [(f,) for f in fq_item_paths]

    # when
    mock_blob, mock_container = create_mock_blob(mock_storage, fq_item_paths)
    kfserving.Storage._download_blob(blob_path, "some/dest/path")

    # then
    actual_calls = get_call_args(mock_container.download_blob.call_args_list)
    assert actual_calls == expected_calls


@mock.patch('kfserving.storage.os.makedirs')
@mock.patch('kfserving.storage.BlobServiceClient')
def test_blob_file(mock_storage, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = 'https://accountname.blob.core.windows.net/container/somefile'
    paths = ['somefile']
    fq_item_paths = paths
    expected_calls = [(f,) for f in fq_item_paths]

    # when
    mock_blob, mock_container = create_mock_blob(mock_storage, paths)
    kfserving.Storage._download_blob(blob_path, "some/dest/path")

    # then
    actual_calls = get_call_args(mock_container.download_blob.call_args_list)
    assert actual_calls == expected_calls


@mock.patch('kfserving.storage.os.makedirs')
@mock.patch('kfserving.storage.BlobServiceClient')
def test_blob_fq_file(mock_storage, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = 'https://accountname.blob.core.windows.net/container/folder/somefile'
    paths = ['somefile']
    fq_item_paths = ['folder/' + p for p in paths]
    expected_calls = [(f,) for f in fq_item_paths]

    # when
    mock_blob, mock_container = create_mock_blob(mock_storage, fq_item_paths)
    kfserving.Storage._download_blob(blob_path, "some/dest/path")

    # then
    actual_calls = get_call_args(mock_container.download_blob.call_args_list)
    assert actual_calls == expected_calls


@mock.patch('kfserving.storage.os.makedirs')
@mock.patch('kfserving.storage.BlobServiceClient')
def test_blob_no_prefix(mock_storage, mock_makedirs):  # pylint: disable=unused-argument

    # given
    blob_path = 'https://accountname.blob.core.windows.net/container/'
    paths = ['somefile', 'somefolder/somefile']
    fq_item_paths = ['' + p for p in paths]
    expected_calls = [(f,) for f in fq_item_paths]

    # when
    mock_blob, mock_container = create_mock_blob(mock_storage, fq_item_paths)
    kfserving.Storage._download_blob(blob_path, "some/dest/path")

    # then
    actual_calls = get_call_args(mock_container.download_blob.call_args_list)
    assert actual_calls == expected_calls
