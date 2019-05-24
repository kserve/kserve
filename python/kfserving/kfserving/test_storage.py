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
import os
from minio import error
from google.cloud import exceptions

# Environment for testing
GCS_PRIVATE_PATH = 'gs://bucket/path'
GOOGLE_APPLICATION_CREDENTIALS = ''

S3_PATH = 's3://bucket/path'
S3_ENDPOINT = ''
AWS_ACCESS_KEY_ID = ''
AWS_SECRET_ACCESS_KEY = ''


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


def test_public_gcs():
    gcs_path = 'gs://kfserving-samples/models/tensorflow/flowers'
    assert kfserving.Storage.download(gcs_path)


def test_private_gcs():
    if GOOGLE_APPLICATION_CREDENTIALS:
        os.environ["GOOGLE_APPLICATION_CREDENTIALS"] = GOOGLE_APPLICATION_CREDENTIALS
        assert kfserving.Storage.download(GCS_PRIVATE_PATH)
    else:
        print('Ignore private GCS bucket test since credentials are not provided')


def test_private_s3():
    if S3_ENDPOINT and AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY:
        os.environ["S3_ENDPOINT"] = S3_ENDPOINT
        os.environ["AWS_ACCESS_KEY_ID"] = AWS_ACCESS_KEY_ID
        os.environ["AWS_SECRET_ACCESS_KEY"] = AWS_SECRET_ACCESS_KEY
        assert kfserving.Storage.download(S3_PATH)
    else:
        print('Ignore S3 bucket test since credentials are not provided')


def test_no_permission_buckets():
    bad_s3_path = "s3://random/path"
    bad_gcs_path = "gs://random/path"
    with pytest.raises(error.AccessDenied):
        kfserving.Storage.download(bad_s3_path)
    with pytest.raises(exceptions.Forbidden):
        kfserving.Storage.download(bad_gcs_path)
