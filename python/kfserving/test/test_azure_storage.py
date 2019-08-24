# Copyright 2019 kubeflow.org.
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

import pytest
import kfserving
import unittest.mock as mock
import itertools

def create_mock_item(path):
    mock_obj = mock.MagicMock()
    mock_obj.name = path
    return mock_obj

def create_mock_blob(mock_storage, paths):
    mock_blob = mock_storage.return_value
    mock_objs = [create_mock_item(path) for path in paths]
    mock_blob.list_blobs.return_value = mock_objs
    return mock_blob

def get_call_args(call_args_list):
    arg_list = []
    for call in call_args_list:
        args, kwargs = call
        arg_list.append(args)
    return arg_list    

@mock.patch('kfserving.storage.BlockBlobService')
@mock.patch('kfserving.storage.Storage._get_azure_storage_token')
def test_blob(mock_get_token, mock_storage):
    
    # given
    blob_path = 'https://kfserving.blob.core.windows.net/tensorrt/simple_string/'
    paths = ['simple_string/1/model.graphdef', 'simple_string/config.pbtxt']
    mock_blob = create_mock_blob(mock_storage, paths)

    # when
    kfserving.Storage._download_blob(blob_path, "dest_path")

    # then
    arg_list = get_call_args(mock_blob.get_blob_to_path.call_args_list)
    assert arg_list == [
        ('tensorrt', 'simple_string/1/model.graphdef', 'dest_path/1/model.graphdef'),
        ('tensorrt', 'simple_string/config.pbtxt', 'dest_path/config.pbtxt')
        ]

@mock.patch('kfserving.storage.BlockBlobService')
@mock.patch('kfserving.storage.Storage._get_azure_storage_token')
def test_deep_blob(mock_get_token, mock_storage):
    # given
    blob_path = 'https://accountname.blob.core.windows.net/container/some/deep/blob/path'
    paths = ['f1', 'f2', 'd1/', 'd1/f3', 'd1/d2/f21', 'd1/d2/d3/', 'd4/f41']
    fq_item_paths = ['some/deep/blob/path/' + p for p in paths]
    expected_dest_paths = ['some/dest/path/' + p for p in paths]
    expected_args = zip(itertools.repeat('container'), fq_item_paths, expected_dest_paths)
    expected_calls = [(args) for args in zipped_args]

    # when
    mock_blob = create_mock_blob(mock_storage, fq_item_paths)
    kfserving.Storage._download_blob(blob_path, "some/dest/path")

    # then
    actual_calls = get_call_args(mock_blob.get_blob_to_path.call_args_list)
    assert actual_calls == expected_args