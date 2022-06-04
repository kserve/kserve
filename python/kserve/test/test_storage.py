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
from pathlib import Path

import botocore
import kserve
import pytest

STORAGE_MODULE = 'kserve.storage'
HTTPS_URI_TARGZ = 'https://foo.bar/model.tar.gz'
HTTPS_URI_TARGZ_WITH_QUERY = HTTPS_URI_TARGZ + '?foo=bar'

# *.tar.gz contains a single empty file model.pth
FILE_TAR_GZ_RAW = binascii.unhexlify('1f8b0800bac550600003cbcd4f49cdd12b28c960a01d3030303033315100d1e666a660dac008c28'
                                     '701054313a090a189919981998281a1b1b1a1118382010ddd0407a5c525894540a754656466e464e'
                                     '2560754969686c71ca83fe0f4281805a360140c7200009f7e1bb400060000')
# *.zip contains a single empty file model.pth
FILE_ZIP_RAW = binascii.unhexlify('504b030414000800080035b67052000000000000000000000000090020006d6f64656c2e70746855540'
                                  'd000786c5506086c5506086c5506075780b000104f501000004140000000300504b07080000000002000'
                                  '00000000000504b0102140314000800080035b6705200000000020000000000000009002000000000000'
                                  '0000000a481000000006d6f64656c2e70746855540d000786c5506086c5506086c5506075780b000104f'
                                  '50100000414000000504b0506000000000100010057000000590000000000')


def test_storage_local_path():
    abs_path = 'file:///'
    relative_path = 'file://.'
    assert kserve.Storage.download(abs_path) == abs_path.replace("file://", "", 1)
    assert kserve.Storage.download(relative_path) == relative_path.replace("file://", "", 1)


def test_storage_local_path_exception():
    not_exist_path = 'file:///some/random/path'
    with pytest.raises(Exception):
        kserve.Storage.download(not_exist_path)


def test_no_prefix_local_path():
    abs_path = '/'
    relative_path = '.'
    assert kserve.Storage.download(abs_path) == abs_path
    assert kserve.Storage.download(relative_path) == relative_path


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
    assert kserve.Storage.download(http_uri, out_dir=out_dir) == out_dir
    assert kserve.Storage.download(http_with_query_uri, out_dir=out_dir) == out_dir
    os.remove('./model.joblib')


@mock.patch('requests.get', return_value=MockHttpResponse(status_code=200, content_type='application/octet-stream'))
def test_https_uri_path(_):
    https_uri = 'https://foo.bar/model.joblib'
    https_with_query_uri = 'https://foo.bar/model.joblib?foo=bar'
    out_dir = '.'
    assert kserve.Storage.download(https_uri, out_dir=out_dir) == out_dir
    assert kserve.Storage.download(https_with_query_uri, out_dir=out_dir) == out_dir
    os.remove('./model.joblib')


http_uri_path_testparams = [
    (HTTPS_URI_TARGZ, MockHttpResponse(200, FILE_TAR_GZ_RAW, 'application/x-tar'), None),
    (HTTPS_URI_TARGZ, MockHttpResponse(200, FILE_TAR_GZ_RAW, 'application/x-gtar'), None),
    (HTTPS_URI_TARGZ, MockHttpResponse(200, FILE_TAR_GZ_RAW, 'application/x-gzip'), None),
    (HTTPS_URI_TARGZ, MockHttpResponse(200, FILE_TAR_GZ_RAW, 'application/gzip'), None),
    (HTTPS_URI_TARGZ, MockHttpResponse(200, FILE_TAR_GZ_RAW, 'application/zip'), RuntimeError),
    (HTTPS_URI_TARGZ_WITH_QUERY, MockHttpResponse(200, FILE_TAR_GZ_RAW, 'application/x-tar'), None),
    (HTTPS_URI_TARGZ_WITH_QUERY, MockHttpResponse(200, FILE_TAR_GZ_RAW, 'application/x-gtar'), None),
    (HTTPS_URI_TARGZ_WITH_QUERY, MockHttpResponse(200, FILE_TAR_GZ_RAW, 'application/x-gzip'), None),
    (HTTPS_URI_TARGZ_WITH_QUERY, MockHttpResponse(200, FILE_TAR_GZ_RAW, 'application/gzip'), None),
    ('https://foo.bar/model.zip', MockHttpResponse(200, FILE_ZIP_RAW, 'application/zip'), None),
    ('https://foo.bar/model.zip', MockHttpResponse(200, FILE_ZIP_RAW, 'application/x-zip-compressed'), None),
    ('https://foo.bar/model.zip', MockHttpResponse(200, FILE_ZIP_RAW, 'application/zip-compressed'), None),
    ('https://foo.bar/model.zip?foo=bar', MockHttpResponse(200, FILE_ZIP_RAW, 'application/zip'), None),
    ('https://foo.bar/model.zip?foo=bar', MockHttpResponse(200, FILE_ZIP_RAW, 'application/x-zip-compressed'), None),
    ('https://foo.bar/model.zip?foo=bar', MockHttpResponse(200, FILE_ZIP_RAW, 'application/zip-compressed'), None),
    ('https://theabyss.net/model.joblib', MockHttpResponse(404), RuntimeError),
    ('https://some.site.com/test.model', MockHttpResponse(status_code=200, content_type='text/html'), RuntimeError),
    ('https://foo.bar/test/', MockHttpResponse(200), ValueError),
]


@pytest.mark.parametrize('uri,response,expected_error', http_uri_path_testparams)
def test_http_uri_paths(uri, response, expected_error):
    if expected_error:
        def test(_):
            with pytest.raises(expected_error):
                kserve.Storage.download(uri)
    else:
        def test(_):
            with tempfile.TemporaryDirectory() as out_dir:
                assert kserve.Storage.download(uri, out_dir=out_dir) == out_dir
                assert os.path.exists(os.path.join(out_dir, 'model.pth'))
    mock.patch('requests.get', return_value=response)(test)()


@mock.patch(STORAGE_MODULE + '.storage')
def test_mock_gcs(mock_storage):
    gcs_path = 'gs://foo/bar'
    mock_obj = mock.MagicMock()
    mock_obj.name = 'mock.object'
    mock_storage.Client().bucket().list_blobs().__iter__.return_value = [mock_obj]
    assert kserve.Storage.download(gcs_path)


def test_storage_blob_exception():
    blob_path = 'https://accountname.blob.core.windows.net/container/some/blob/'
    with pytest.raises(Exception):
        kserve.Storage.download(blob_path)


@mock.patch(STORAGE_MODULE + '.boto3')
def test_storage_s3_exception(mock_boto3):
    path = 's3://foo/bar'
    # Create mock client
    mock_s3_resource = mock.MagicMock()
    mock_s3_resource.Bucket.side_effect = Exception()
    mock_boto3.resource.return_value = mock_s3_resource

    with pytest.raises(Exception):
        kserve.Storage.download(path)


@mock.patch(STORAGE_MODULE + '.boto3')
@mock.patch('urllib3.PoolManager')
def test_no_permission_buckets(mock_connection, mock_boto3):
    bad_s3_path = "s3://random/path"
    # Access private buckets without credentials
    mock_s3_resource = mock.MagicMock()
    mock_s3_bucket = mock.MagicMock()
    mock_s3_bucket.objects.filter.return_value = [mock.MagicMock()]
    mock_s3_bucket.objects.filter.side_effect = botocore.exceptions.ClientError(
        {}, "GetObject"
    )
    mock_s3_resource.Bucket.return_value = mock_s3_bucket
    mock_boto3.resource.return_value = mock_s3_resource

    with pytest.raises(botocore.exceptions.ClientError):
        kserve.Storage.download(bad_s3_path)


def test_unpack_tar_file():
    out_dir = '.'
    tar_file = os.path.join(out_dir, "model.tgz")
    Path(tar_file).write_bytes(FILE_TAR_GZ_RAW)
    mimetype, _ = mimetypes.guess_type(tar_file)
    kserve.Storage._unpack_archive_file(tar_file, mimetype, out_dir)
    assert os.path.exists(os.path.join(out_dir, 'model.pth'))
    os.remove(os.path.join(out_dir, 'model.pth'))


def test_unpack_zip_file():
    out_dir = '.'
    tar_file = os.path.join(out_dir, "model.zip")
    Path(tar_file).write_bytes(FILE_ZIP_RAW)
    mimetype, _ = mimetypes.guess_type(tar_file)
    kserve.Storage._unpack_archive_file(tar_file, mimetype, out_dir)
    assert os.path.exists(os.path.join(out_dir, 'model.pth'))
    os.remove(os.path.join(out_dir, 'model.pth'))
