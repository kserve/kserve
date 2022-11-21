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

import os
import unittest.mock as mock

from botocore.client import Config
from botocore import UNSIGNED

from kserve.storage import Storage

STORAGE_MODULE = 'kserve.storage.storage'


def create_mock_obj(path):
    mock_obj = mock.MagicMock()
    mock_obj.key = path
    mock_obj.is_dir = False
    return mock_obj


def create_mock_boto3_bucket(mock_storage, paths):
    mock_s3_resource = mock.MagicMock()
    mock_s3_bucket = mock.MagicMock()
    mock_s3_bucket.objects.filter.return_value = [create_mock_obj(p) for p in paths]

    mock_s3_resource.Bucket.return_value = mock_s3_bucket
    mock_storage.resource.return_value = mock_s3_resource

    return mock_s3_bucket


def get_call_args(call_args_list):
    arg_list = []
    for call in call_args_list:
        args, _ = call
        arg_list.append(args)
    return arg_list


def expected_call_args_list_single_obj(dest, path):
    return [(
        f'{path}'.strip('/'),
        f'{dest}/{path.rsplit("/", 1)[-1]}'.strip('/'))]


def expected_call_args_list(parent_key, dest, paths):
    return [(f'{parent_key}/{p}'.strip('/'), f'{dest}/{p}'.strip('/'))
            for p in paths]


# pylint: disable=protected-access


@mock.patch(STORAGE_MODULE + '.boto3')
def test_parent_key(mock_storage):
    # given
    bucket_name = 'foo'
    paths = ['models/weights.pt', '0002.h5', 'a/very/long/path/config.json']
    object_paths = ['bar/' + p for p in paths]

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, object_paths)
    Storage._download_s3(f's3://{bucket_name}/bar', 'dest_path')

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list == expected_call_args_list('bar', 'dest_path', paths)

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix='bar')


@mock.patch(STORAGE_MODULE + '.boto3')
def test_no_key(mock_storage):
    # given
    bucket_name = 'foo'
    object_paths = ['models/weights.pt', '0002.h5', 'a/very/long/path/config.json']

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, object_paths)
    Storage._download_s3(f's3://{bucket_name}/', 'dest_path')

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list == expected_call_args_list('', 'dest_path', object_paths)

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix='')


@mock.patch(STORAGE_MODULE + '.boto3')
def test_full_name_key(mock_storage):
    # given
    bucket_name = 'foo'
    object_key = 'path/to/model/name.pt'

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, [object_key])
    Storage._download_s3(f's3://{bucket_name}/{object_key}', 'dest_path')

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list == expected_call_args_list_single_obj('dest_path',
                                                          object_key)

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix=object_key)


@mock.patch(STORAGE_MODULE + '.boto3')
def test_full_name_key_root_bucket_dir(mock_storage):
    # given
    bucket_name = 'foo'
    object_key = 'name.pt'

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, [object_key])
    Storage._download_s3(f's3://{bucket_name}/{object_key}', 'dest_path')

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list == expected_call_args_list_single_obj('dest_path',
                                                          object_key)

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix=object_key)


AWS_TEST_CREDENTIALS = {"AWS_ACCESS_KEY_ID": "testing",
                        "AWS_SECRET_ACCESS_KEY": "testing",
                        "AWS_SECURITY_TOKEN": "testing",
                        "AWS_SESSION_TOKEN": "testing"}


@mock.patch('kserve.storage.boto3')
def test_files_with_no_extension(mock_storage):

    # given
    bucket_name = 'foo'
    paths = ['churn-pickle', 'churn-pickle-logs', 'churn-pickle-report']
    object_paths = ['test/' + p for p in paths]

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, object_paths)
    kserve.Storage._download_s3(f's3://{bucket_name}/test/churn-pickle', 'dest_path')

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list == expected_call_args_list('test', 'dest_path', paths)

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix='test/churn-pickle')


def test_get_S3_config():
    DEFAULT_CONFIG = Config()
    ANON_CONFIG = Config(signature_version=UNSIGNED)
    VIRTUAL_CONFIG = Config(s3={"addressing_style": "virtual"})

    with mock.patch.dict(os.environ, {}):
        config1 = Storage.get_S3_config()
    assert vars(config1) == vars(DEFAULT_CONFIG)

    with mock.patch.dict(os.environ, {"awsAnonymousCredential": "False"}):
        config2 = Storage.get_S3_config()
    assert vars(config2) == vars(DEFAULT_CONFIG)

    with mock.patch.dict(os.environ, AWS_TEST_CREDENTIALS):
        config3 = Storage.get_S3_config()
    assert vars(config3) == vars(DEFAULT_CONFIG)

    with mock.patch.dict(os.environ, {"awsAnonymousCredential": "True"}):
        config4 = Storage.get_S3_config()
    assert config4.signature_version == ANON_CONFIG.signature_version

    # assuming Python 3.5 or greater for joining dictionaries
    credentials_and_anon = {**AWS_TEST_CREDENTIALS, "awsAnonymousCredential": "True"}
    with mock.patch.dict(os.environ, credentials_and_anon):
        config5 = Storage.get_S3_config()
    assert config5.signature_version == ANON_CONFIG.signature_version

    with mock.patch.dict(os.environ, {"S3_USER_VIRTUAL_BUCKET": "False"}):
        config6 = Storage.get_S3_config()
    assert vars(config6) == vars(DEFAULT_CONFIG)

    with mock.patch.dict(os.environ, {"S3_USER_VIRTUAL_BUCKET": "True"}):
        config7 = Storage.get_S3_config()
    assert config7.s3["addressing_style"] == VIRTUAL_CONFIG.s3["addressing_style"]
