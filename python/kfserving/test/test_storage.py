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

import io
import os
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

class MockHttpResponse(object):
    def __init__(
        self,
        status_code=404,
        raw=b'',
        content_type=''
    ):
        self.status_code = status_code
        self.raw = io.BytesIO(raw)
        self.headers = {'Content-Type': content_type}

    def __enter__(self):
        return self
    def __exit__(self, ex_type, ex_val, traceback):
        pass                                                                                                                    

@mock.patch('requests.get', return_value=MockHttpResponse(status_code=200, content_type='application/octet-stream'))
def test_http_uri_path(_):
    http_uri = 'http://foo.bar/model.joblib'
    http_with_query_uri = 'http://foo.bar/model.joblib?foo=bar'
    out_dir = '.'
    assert kfserving.Storage.download(http_uri, out_dir=out_dir) == out_dir
    assert kfserving.Storage.download(http_with_query_uri, out_dir=out_dir) == out_dir
    os.remove('./model.joblib')

@mock.patch('requests.get', return_value=MockHttpResponse(status_code=200, content_type='application/octet-stream'))
def test_https_uri_path(_):
    https_uri = 'https://foo.bar/model.joblib' 
    https_with_query_uri = 'https://foo.bar/model.joblib?foo=bar'
    out_dir = '.'
    assert kfserving.Storage.download(https_uri, out_dir=out_dir) == out_dir
    assert kfserving.Storage.download(https_with_query_uri, out_dir=out_dir) == out_dir
    os.remove('./model.joblib')

@mock.patch('requests.get', return_value=MockHttpResponse(status_code=404))
def test_nonexistent_uri(_):
    non_existent_uri = 'https://theabyss.net/model.joblib'
    with pytest.raises(RuntimeError):
        kfserving.Storage.download(non_existent_uri)

@mock.patch('requests.get', return_value=MockHttpResponse(status_code=200))
def test_uri_no_filename(_):
    bad_uri = 'https://foo.bar/test/'
    with pytest.raises(ValueError):
        kfserving.Storage.download(bad_uri)

@mock.patch('requests.get', return_value=MockHttpResponse(status_code=200, content_type='text/html'))
def test_html_content_type(_):
    bad_uri = 'https://some.site.com/test.model'
    with pytest.raises(RuntimeError):
        kfserving.Storage.download(bad_uri)

@mock.patch(STORAGE_MODULE + '.storage')
def test_mock_gcs(mock_storage):
    gcs_path = 'gs://foo/bar'
    mock_obj = mock.MagicMock()
    mock_obj.name = 'mock.object'
    mock_storage.Client().bucket().list_blobs().__iter__.return_value = [mock_obj]
    assert kfserving.Storage.download(gcs_path)

def test_storage_blob_exception():
    blob_path = 'https://accountname.blob.core.windows.net/container/some/blob/'
    with pytest.raises(Exception):
        kfserving.Storage.download(blob_path)

@mock.patch('urllib3.PoolManager')
@mock.patch(STORAGE_MODULE + '.Minio')
def test_storage_s3_exception(mock_connection, mock_minio):
    minio_path = 's3://foo/bar'
    # Create mock connection
    mock_server = mock.MagicMock()
    mock_connection.return_value = mock_server
    # Create mock client
    mock_minio.return_value = Minio("s3.us.cloud-object-storage.appdomain.cloud", secure=True)
    with pytest.raises(Exception):
        kfserving.Storage.download(minio_path)

@mock.patch('urllib3.PoolManager')
@mock.patch(STORAGE_MODULE + '.Minio')
def test_no_permission_buckets(mock_connection, mock_minio):
    bad_s3_path = "s3://random/path"
    #bad_gcs_path = "gs://random/path"
    # Access private buckets without credentials
    mock_minio.return_value = Minio("s3.us.cloud-object-storage.appdomain.cloud", secure=True)
    mock_connection.side_effect = error.AccessDenied()
    with pytest.raises(error.AccessDenied):
        kfserving.Storage.download(bad_s3_path)
    #mock_connection.side_effect = exceptions.Forbidden(None)
    #with pytest.raises(exceptions.Forbidden):
    #    kfserving.Storage.download(bad_gcs_path)
