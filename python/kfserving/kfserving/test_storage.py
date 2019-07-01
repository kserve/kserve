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
from minio import Minio, error
from google.cloud import exceptions
import unittest.mock as mock

STORAGE_MODULE = 'kfserving.storage'


def test_storage_local_path():
    abs_path = 'file:///'
    relative_path = 'file://.'
    assert kfserving.Storage.download(abs_path) == abs_path.replace("file://", "", 1)
    assert kfserving.Storage.download(relative_path) == relative_path.replace("file://", "", 1)


def test_storage_local_path_exception():
    not_exist_path = 'file:///some/random/path'
    with pytest.raises(Exception):
        kfserving.Storage.download(not_exist_path)


def test_no_prefix_local_path():
    abs_path = '/'
    relative_path = '.'
    assert kfserving.Storage.download(abs_path) == abs_path
    assert kfserving.Storage.download(relative_path) == relative_path


@mock.patch(STORAGE_MODULE + '.storage')
def test_mock_gcs(mock_storage):
    gcs_path = 'gs://foo/bar'
    mock_obj = mock.MagicMock()
    mock_obj.name = 'mock.object'
    mock_storage.Client().bucket().list_blobs().__iter__.return_value = [mock_obj]
    assert kfserving.Storage.download(gcs_path)

@mock.patch(STORAGE_MODULE + '.BlockBlobService')
def test_mock_blob(mock_storage):
    blob_path = 'https://accountname.blob.core.windows.net/container/some/blob/'
    mock_obj = mock.MagicMock()
    mock_obj.name = 'mock.object'
    mock_storage.list_blobs.__iter__.return_value = [mock_obj]
    assert kfserving.Storage.download(blob_path)

@mock.patch('urllib3.PoolManager')
@mock.patch(STORAGE_MODULE + '.Minio')
def test_mock_minio(mock_connection, mock_minio):
    minio_path = 's3://foo/bar'
    # Create mock connection
    mock_server = mock.MagicMock()
    mock_connection.return_value = mock_server
    # Create mock client
    mock_minio.return_value = Minio("s3.us.cloud-object-storage.appdomain.cloud", secure=True)
    mock_obj = mock.MagicMock()
    mock_obj.object_name = 'mock.object'
    mock_minio.list_objects().__iter__.return_value = [mock_obj]
    assert kfserving.Storage.download(minio_path)


@mock.patch('urllib3.PoolManager')
@mock.patch(STORAGE_MODULE + '.Minio')
def test_no_permission_buckets(mock_connection, mock_minio):
    bad_s3_path = "s3://random/path"
    bad_gcs_path = "gs://random/path"
    # Access private buckets without credentials
    mock_minio.return_value = Minio("s3.us.cloud-object-storage.appdomain.cloud", secure=True)
    mock_connection.side_effect = error.AccessDenied(None)
    with pytest.raises(error.AccessDenied):
        kfserving.Storage.download(bad_s3_path)
    mock_connection.side_effect = exceptions.Forbidden(None)
    with pytest.raises(exceptions.Forbidden):
        kfserving.Storage.download(bad_gcs_path)
