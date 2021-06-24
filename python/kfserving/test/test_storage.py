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
import tempfile
import binascii
import unittest.mock as mock

import botocore
import kfserving
import pytest

STORAGE_MODULE = 'kfserving.storage'
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

# *.tar.gz contains a single empty file model.pth
FILE_TAR_GZ_RAW_IN_DIR = binascii.unhexlify('1f8b0800000000000003edd14d0ac2400c86e11c654ea099ce64721e41a10b4bfda9f7b7ad'
                                            '08c585ba9945e9fb2c12480626f075fdf174de4b553a72b3a947375df63789594b4edaa49c'
                                            'c6b9bbb904ab7bd6cbe33e1c6e21487bfdfeeed77ea5ba39ffb9ee2e435be58f29e092f3ff'
                                            'f9c768a591a055aef9b0f1fc01000000000000000000000000acd71328c4bb3700280000')

# *.zip contains a single empty file model.pth inside directory model
FILE_ZIP_RAW_IN_DIR = binascii.unhexlify('504b03040a00000000008a74d65200000000000000000000000006001c006d6f64656c2f55540'
                                         '90003e384d1600f87d16075780b000104e803000004e8030000504b03040a00000000008a74d6'
                                         '520000000000000000000000000f0000006d6f64656c2f6d6f64656c2e707468504b01021e030'
                                         'a00000000008a74d652000000000000000000000000060018000000000000001000ed41000000'
                                         '006d6f64656c2f5554050003e384d16075780b000104e803000004e8030000504b01023f000a0'
                                         '0000000008a74d6520000000000000000000000000f0024000000000000008000000040000000'
                                         '6d6f64656c2f6d6f64656c2e7074680a0020000000000001001800a09deae83067d701408ed81'
                                         '93167d701a09deae83067d701504b05060000000002000200ad0000006d0000000000')

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
                kfserving.Storage.download(uri)
    else:
        def test(_):
            with tempfile.TemporaryDirectory() as out_dir:
                assert kfserving.Storage.download(uri, out_dir=out_dir) == out_dir
                assert os.path.exists(os.path.join(out_dir, 'model.pth'))
    mock.patch('requests.get', return_value=response)(test)()


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


@mock.patch(STORAGE_MODULE + '.boto3')
def test_storage_s3_exception(mock_boto3):
    path = 's3://foo/bar'
    # Create mock client
    mock_s3_resource = mock.MagicMock()
    mock_s3_resource.Bucket.side_effect = Exception()
    mock_boto3.resource.return_value = mock_s3_resource

    with pytest.raises(Exception):
        kfserving.Storage.download(path)


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
        kfserving.Storage.download(bad_s3_path)

def test_extract_tar(tmp_path):
    outdir = tmp_path / "outdir"
    outdir.mkdir()
    fpath = tmp_path / "test.tar.gz"
    fpath.write_bytes(FILE_TAR_GZ_RAW)
    kfserving.Storage._extract_tarfile(str(fpath.absolute()), outdir)
    assert os.path.exists(os.path.join(outdir, 'model.pth'))

def test_extract_zip(tmp_path):
    outdir = tmp_path / "outdir"
    outdir.mkdir()
    fpath = tmp_path / "test.zip"
    fpath.write_bytes(FILE_ZIP_RAW)
    kfserving.Storage._extract_zipfile(str(fpath.absolute()), outdir)
    assert os.path.exists(os.path.join(outdir, 'model.pth'))

def test_extract_tar_with_basedir(tmp_path):
    outdir = tmp_path / "outdir"
    outdir.mkdir()
    fpath = tmp_path / "model.tar.gz"
    fpath.write_bytes(FILE_TAR_GZ_RAW_IN_DIR)
    kfserving.Storage._extract_tarfile(str(fpath.absolute()), outdir)
    assert os.path.exists(os.path.join(outdir, 'model.pth'))

def test_extract_zip_with_basedir(tmp_path):
    outdir = tmp_path / "outdir"
    outdir.mkdir()
    fpath = tmp_path / "model.zip"
    fpath.write_bytes(FILE_ZIP_RAW_IN_DIR)
    kfserving.Storage._extract_zipfile(str(fpath.absolute()), outdir)
    assert os.path.exists(os.path.join(outdir, 'model.pth'))

test_params = [
    (FILE_TAR_GZ_RAW, "test1.tar.gz", "application/x-tar", None),
    (FILE_ZIP_RAW, "test2.zip", "application/zip", None),
    (FILE_TAR_GZ_RAW_IN_DIR, "model1.tar.gz", "application/x-tar", None),
    (FILE_ZIP_RAW_IN_DIR, "model2.zip", "application/zip", None),
    (b"", "test.txt", "text/html", RuntimeError),
]

@pytest.mark.parametrize("raw_data,filename, mimetype, expected_error", test_params)
def test_extract(tmp_path, raw_data, filename, mimetype, expected_error):
    outdir = tmp_path / "outdir"
    outdir.mkdir()
    fpath = tmp_path / filename
    fpath.write_bytes(raw_data)
    if expected_error:
        with pytest.raises(expected_error):
            kfserving.Storage._extract(str(fpath.absolute()), outdir, mimetype=mimetype)
    else:
        kfserving.Storage._extract(str(fpath.absolute()), outdir, mimetype=mimetype)
        assert os.path.exists(os.path.join(outdir, 'model.pth'))
