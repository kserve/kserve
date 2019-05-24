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

import logging
import tempfile
import os
import subprocess
import re
from minio import Minio, error
from google.cloud import storage, exceptions

_GCS_PREFIX = "gs://"
_S3_PREFIX = "s3://"
_LOCAL_PREFIX = "file://"


class Storage(object):
    @staticmethod
    def download(uri: str) -> str:
        logging.info("Copying contents of %s to local" % uri)
        if uri.startswith(_LOCAL_PREFIX) or os.path.exists(uri):
            return Storage._download_local(uri)

        temp_dir = tempfile.mkdtemp()
        if uri.startswith(_GCS_PREFIX):
            Storage._download_gcs(uri, temp_dir)
        elif uri.startswith(_S3_PREFIX):
            Storage._download_s3(uri, temp_dir)
        else:
            raise Exception("Cannot recognize storage type for " + uri +
                            "\n'%s', '%s', and '%s' are the current available storage type." %
                            (_GCS_PREFIX, _S3_PREFIX, _LOCAL_PREFIX))

        logging.info("Successfully copied %s to %s" % (uri, temp_dir))
        return temp_dir

    @staticmethod
    def _download_s3(uri, temp_dir: str):
        client = Storage._create_minio_client()
        try:
            bucket_args = uri.replace(_S3_PREFIX, "", 1).split("/", 1)
            bucket_name = bucket_args[0]
            bucket_path = bucket_args[1] if len(bucket_args) > 1 else ""
            objects = client.list_objects(bucket_name, prefix=bucket_path, recursive=True)
            for obj in objects:
                object_file_name = obj.object_name.replace(bucket_path, "", 1).strip("/")
                client.fget_object(bucket_name, obj.object_name, temp_dir + "/" + object_file_name)
        except error.NoSuchBucket as e:
            raise error.NoSuchBucket("Bucket is not found, please double check the bucket name.")
        except error.AccessDenied as e:
            raise error.AccessDenied("Access Denied. Make sure the S3 credentials has the right access to this bucket.")

    @staticmethod
    def _download_gcs(uri, temp_dir: str):
        try:
            storage_client = storage.Client()
            bucket_args = uri.replace(_GCS_PREFIX, "", 1).split("/", 1)
            bucket_name = bucket_args[0]
            bucket_path = bucket_args[1] if len(bucket_args) > 1 else ""
            bucket = storage_client.get_bucket(bucket_name)
            blobs = bucket.list_blobs(prefix=bucket_path)
            for blob in blobs:
                object_file_name = blob.name.replace(bucket_path, "", 1).strip("/")
                blob.download_to_filename(temp_dir + "/" + object_file_name)
        except exceptions.Forbidden as e:
            logging.info("Google cloud storage Python SDK failed. Trying with gsutil to access the data in public")
            uri_files = uri.strip("/") + "/*"
            process = subprocess.run(["gsutil", "cp", "-r", uri_files, temp_dir], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            logging.info(process.stdout)
            logging.info(process.stderr)
            if process.returncode != 0:
                raise exceptions.Forbidden("gsutil error: files didn't copy to temp_dir. Please double check the GCS path or credentials.")
        except exceptions.NotFound as e:
            raise exceptions.NotFound("Bucket is not found, please double check the bucket name.")

    @staticmethod
    def _download_local(uri):
        local_path = uri.replace(_LOCAL_PREFIX, "", 1)
        if not os.path.exists(local_path):
            raise Exception("Local path %s does not exist." % (uri))
        return local_path

    @staticmethod
    def _create_minio_client():
        try:
            # Remove possible http scheme for Minio
            url = re.compile(r"https?://")
            minioClient = Minio(url.sub("", os.getenv("S3_ENDPOINT", "")),
                                access_key=os.getenv("AWS_ACCESS_KEY_ID", ""),
                                secret_key=os.getenv("AWS_SECRET_ACCESS_KEY", ""),
                                secure=True)
        except IndexError as err:
            raise IndexError('Error: Incorrect syntax with the S3 endpoint or credentials. \n' + err)
        return minioClient
