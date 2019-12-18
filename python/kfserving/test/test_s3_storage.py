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

import os
import tempfile
import unittest
from unittest.mock import MagicMock
from unittest.mock import Mock
from unittest.mock import call
from unittest.mock import patch

import kfserving


class TestS3Storage(unittest.TestCase):

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

        self.whole_bucket_download_calls = [
            call('kfserving-storage-test', 'file1', unittest.mock.ANY),
            call('kfserving-storage-test', 'file2', unittest.mock.ANY),
            call('kfserving-storage-test', 'subdir1/file1', unittest.mock.ANY),
            call('kfserving-storage-test', 'subdir1/file2', unittest.mock.ANY),
            call('kfserving-storage-test', 'subdir2/file1', unittest.mock.ANY),
            call('kfserving-storage-test', 'subdir2/file2', unittest.mock.ANY)
        ]

        self.under_prefix_download_calls = [
            call('kfserving-storage-test', 'model-prefix/file1', unittest.mock.ANY),
            call('kfserving-storage-test', 'model-prefix/file2', unittest.mock.ANY),
            call('kfserving-storage-test', 'model-prefix/subdir1/file1', unittest.mock.ANY),
            call('kfserving-storage-test', 'model-prefix/subdir1/file2', unittest.mock.ANY),
            call('kfserving-storage-test', 'model-prefix/subdir2/file1', unittest.mock.ANY),
            call('kfserving-storage-test', 'model-prefix/subdir2/file2', unittest.mock.ANY)
        ]

    @patch('boto3.client')
    def testDownloadWholeS3Bucket(self, client):
        storage = kfserving.Storage()
        with tempfile.TemporaryDirectory() as tmpdir:
            s3client = MagicMock()
            s3client.download_fileobj = MagicMock()
            client.return_value = s3client
            s3client.list_objects_v2 = Mock(return_value=TestS3Storage._generate_s3_list_objects_response())
            storage.download('s3://kfserving-storage-test', tmpdir)
            s3client.list_objects_v2.assert_called_with(Bucket='kfserving-storage-test')
            s3client.download_fileobj.assert_has_calls(self.whole_bucket_download_calls, any_order=True)
            self._verify_download(tmpdir)

    @patch('boto3.client')
    def testDownloadWholeBucketTrailingSlash(self, client):
        storage = kfserving.Storage()
        with tempfile.TemporaryDirectory() as tmpdir:
            s3client = MagicMock()
            s3client.download_fileobj = MagicMock()
            client.return_value = s3client
            s3client.list_objects_v2 = Mock(return_value=TestS3Storage._generate_s3_list_objects_response())
            storage.download('s3://kfserving-storage-test/', tmpdir)
            s3client.list_objects_v2.assert_called_with(Bucket='kfserving-storage-test')
            s3client.download_fileobj.assert_has_calls(self.whole_bucket_download_calls, any_order=True)
            self._verify_download(tmpdir)

    @patch('boto3.client')
    def testDownloadUnderPrefix(self, client):
        storage = kfserving.Storage()
        with tempfile.TemporaryDirectory() as tmpdir:
            s3client = MagicMock()
            s3client.download_fileobj = MagicMock()
            client.return_value = s3client
            s3client.list_objects_v2 = Mock(
                return_value=TestS3Storage._generate_s3_list_objects_response(prefix='model-prefix'))
            storage.download('s3://kfserving-storage-test/model-prefix', tmpdir)
            s3client.list_objects_v2.assert_called_with(Bucket='kfserving-storage-test', Prefix='model-prefix')
            s3client.download_fileobj.assert_has_calls(self.under_prefix_download_calls, any_order=True)
            self._verify_download(tmpdir)

    @patch('boto3.client')
    def testDownloadUnderPrefixTrailingSlash(self, client):
        storage = kfserving.Storage()
        with tempfile.TemporaryDirectory() as tmpdir:
            s3client = MagicMock()
            s3client.download_fileobj = MagicMock()
            client.return_value = s3client
            s3client.list_objects_v2 = Mock(
                return_value=TestS3Storage._generate_s3_list_objects_response(prefix='model-prefix'))
            storage.download('s3://kfserving-storage-test/model-prefix/', tmpdir)
            s3client.list_objects_v2.assert_called_with(Bucket='kfserving-storage-test', Prefix='model-prefix/')
            s3client.download_fileobj.assert_has_calls(self.under_prefix_download_calls, any_order=True)
            self._verify_download(tmpdir)

    @patch('boto3.client')
    def testDownloadNonExistentPrefix(self, client):
        storage = kfserving.Storage()
        with tempfile.TemporaryDirectory() as tmpdir:
            s3client = MagicMock()
            with self.assertRaises(RuntimeError):
                s3client.download_fileobj = MagicMock()
                client.return_value = s3client
                s3client.list_objects_v2 = Mock(
                    return_value=TestS3Storage._generate_s3_list_objects_response(nfiles=0, ndirs=0, depth=0))
                storage.download('s3://kfserving-storage-test/nonexistent', tmpdir)
            s3client.list_objects_v2.assert_called_with(Bucket='kfserving-storage-test',
                                                        Prefix='nonexistent')
            s3client.download_fileobj.assert_not_called()
            self.assertListEqual([(tmpdir, [], [])], list(os.walk(tmpdir)))

    @patch('boto3.client')
    def testContinuation(self, client):
        storage = kfserving.Storage()
        with tempfile.TemporaryDirectory() as tmpdir:
            s3client = MagicMock()
            s3client.download_fileobj = MagicMock()
            client.return_value = s3client
            s3client.list_objects_v2 = TestS3Storage._generate_truncated_response
            storage.download('s3://kfserving-storage-test', tmpdir)
            s3client.download_fileobj.assert_has_calls(self.whole_bucket_download_calls, any_order=True)
            self._verify_download(tmpdir)

    @patch('boto3.client')
    def testEmptyDir(self, client):
        storage = kfserving.Storage()
        with tempfile.TemporaryDirectory() as tmpdir:
            s3client = MagicMock()
            s3client.download_fileobj = MagicMock()
            client.return_value = s3client
            s3client.list_objects_v2 = Mock(
                return_value=TestS3Storage._generate_s3_list_objects_response(nfiles=1, ndirs=1, depth=0,
                                                                              empty_dirs=True))
            storage.download('s3://kfserving-storage-test', tmpdir)
            s3client.list_objects_v2.assert_called_with(Bucket='kfserving-storage-test')
            s3client.download_fileobj.assert_called_with('kfserving-storage-test', 'file1', unittest.mock.ANY)
            self.assertListEqual([(tmpdir, ['subdir1'], ['file1']), (os.path.join(tmpdir, 'subdir1'), [], [])],
                                 list(os.walk(tmpdir)))

    def _verify_download(self, tmpdir, prefix=None):
        if prefix:
            tmpdir = os.path.join(tmpdir, prefix)
        for root, dirs, files in os.walk(tmpdir):
            if root == tmpdir:
                self.assertListEqual(sorted(['subdir1', 'subdir2']), sorted(dirs))
                self.assertListEqual(sorted(['file1', 'file2']), sorted(files))
            elif root == os.path.join(tmpdir, 'subdir1') or root == os.path.join(tmpdir, 'subdir2'):
                self.assertListEqual([], dirs)
                self.assertListEqual(sorted(['file1', 'file2']), sorted(files))
            else:
                self.fail('Unexpected download directory: {}'.format(root))

    @staticmethod
    def _generate_keys(prefix, nfiles, ndirs, depth, empty_dirs):
        keys = []

        for file_index in range(1, nfiles + 1):
            key = 'file{}'.format(file_index)
            if prefix:
                key = '{}/{}'.format(prefix, key)
            keys.append(key)

        if depth > 1:
            for dir_index in range(1, ndirs + 1):
                r_prefix = 'subdir{}'.format(dir_index)
                if prefix:
                    r_prefix = '{}/{}'.format(prefix, r_prefix)
                keys += TestS3Storage._generate_keys(r_prefix, nfiles, ndirs, depth - 1, empty_dirs)
        elif ndirs > 0 and empty_dirs:
            for dir_index in range(1, ndirs + 1):
                key = 'subdir{}/'.format(dir_index)
                if prefix:
                    key = '{}/{}'.format(prefix, key)
                keys.append(key)

        return keys

    @staticmethod
    def _generate_s3_list_objects_response(prefix='', nfiles=2, ndirs=2, depth=2, empty_dirs=False):
        keys = TestS3Storage._generate_keys(prefix, nfiles, ndirs, depth, empty_dirs)
        return {'Contents': [{'ETag': None,
                              'Key': key,
                              'LastModified': None,
                              'Size': None,
                              'StorageClass': 'STANDARD'} for key in keys],
                'EncodingType': 'url',
                'IsTruncated': False,
                'KeyCount': len(keys),
                'MaxKeys': 1000,
                'Name': 'kfserving-storage-test',
                'Prefix': prefix,
                'ResponseMetadata': {'HTTPHeaders': {'content-type': 'application/xml',
                                                     'date': None,
                                                     'server': None,
                                                     'transfer-encoding': None,
                                                     'x-amz-bucket-region': None,
                                                     'x-amz-id-2': None,
                                                     'x-amz-request-id': None},
                                     'HTTPStatusCode': 200,
                                     'HostId': None,
                                     'RequestId': None,
                                     'RetryAttempts': 0}}

    @staticmethod
    def _generate_truncated_response(Bucket, Prefix='', ContinuationToken=0):
        response = TestS3Storage._generate_s3_list_objects_response(prefix=Prefix)
        response['MaxKeys'] = 1
        if ContinuationToken < len(response['Contents']) - 1:
            response['IsTruncated'] = True
            response['NextContinuationToken'] = ContinuationToken + 1
        response['Contents'] = [response['Contents'][ContinuationToken]]
        return response


if __name__ == '__main__':
    unittest.main()
