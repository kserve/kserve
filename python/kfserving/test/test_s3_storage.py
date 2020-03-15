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
import kfserving


def create_mock_obj(path):
    mock_obj = mock.MagicMock()
    mock_obj.object_name = path
    mock_obj.is_dir = False
    return mock_obj


def create_mock_minio_client(mock_storage, paths):
    mock_minio_client = mock_storage.return_value
    mock_minio_client.list_objects.return_value = [create_mock_obj(p) for p in paths]
    return mock_minio_client


def get_call_args(call_args_list):
    arg_list = []
    for call in call_args_list:
        args, _ = call
        arg_list.append(args)
    return arg_list


def expected_call_args_list(bucket_name, parent_key, dest, paths):
    return [(bucket_name, f'{parent_key}/{p}'.strip('/'), f'{dest}/{p}'.strip('/'))
            for p in paths]

# pylint: disable=protected-access


@mock.patch('kfserving.storage.Minio')
def test_parent_key(mock_storage):

    # given
    bucket_name = 'foo'
    paths = ['models/weights.pt', '0002.h5', 'a/very/long/path/config.json']
    object_paths = ['bar/' + p for p in paths]

    # when
    mock_minio_client = create_mock_minio_client(mock_storage, object_paths)
    kfserving.Storage._download_s3(f's3://{bucket_name}/bar', 'dest_path')

    # then
    arg_list = get_call_args(mock_minio_client.fget_object.call_args_list)
    assert arg_list == expected_call_args_list(bucket_name, 'bar', 'dest_path', paths)

    mock_minio_client.list_objects.assert_called_with(bucket_name, prefix='bar', recursive=True)


@mock.patch('kfserving.storage.Minio')
def test_no_key(mock_storage):

    # given
    bucket_name = 'foo'
    object_paths = ['models/weights.pt', '0002.h5', 'a/very/long/path/config.json']

    # when
    mock_minio_client = create_mock_minio_client(mock_storage, object_paths)
    kfserving.Storage._download_s3(f's3://{bucket_name}/', 'dest_path')

    # then
    arg_list = get_call_args(mock_minio_client.fget_object.call_args_list)
    assert arg_list == expected_call_args_list(bucket_name, '', 'dest_path', object_paths)

    mock_minio_client.list_objects.assert_called_with(bucket_name, prefix='', recursive=True)


@mock.patch('kfserving.storage.Minio')
def test_full_name_key(mock_storage):

    # given
    bucket_name = 'foo'
    object_key = 'path/to/model/name.pt'

    # when
    mock_minio_client = create_mock_minio_client(mock_storage, [object_key])
    kfserving.Storage._download_s3(f's3://{bucket_name}/{object_key}', 'dest_path')

    # then
    arg_list = get_call_args(mock_minio_client.fget_object.call_args_list)
    assert arg_list == expected_call_args_list(bucket_name, '', 'dest_path',
                                               [object_key])

    mock_minio_client.list_objects.assert_called_with(bucket_name, prefix=object_key,
                                                      recursive=True)
