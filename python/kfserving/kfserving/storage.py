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
import re
from minio import Minio
from google.cloud import storage
from google.auth import exceptions

_GCS_PREFIX = "gs://"
_S3_PREFIX = "s3://"
_LOCAL_PREFIX = "file://"


class Storage(object): # pylint: disable=too-few-public-methods
    @staticmethod
    def download(uri: str) -> str:
        logging.info("Copying contents of %s to local", uri)
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

        logging.info("Successfully copied %s to %s", uri, temp_dir)
        return temp_dir

    @staticmethod
    def _download_s3(uri, temp_dir: str):
        client = Storage._create_minio_client()
        bucket_args = uri.replace(_S3_PREFIX, "", 1).split("/", 1)
        bucket_name = bucket_args[0]
        bucket_path = bucket_args[1] if len(bucket_args) > 1 else ""
        objects = client.list_objects(bucket_name, prefix=bucket_path, recursive=True)
        for obj in objects:
            # Replace any prefix from the object key with temp_dir
            subdir_object_key = obj.object_name.replace(bucket_path, "", 1).strip("/")
            client.fget_object(bucket_name, obj.object_name,
                               os.path.join(temp_dir, subdir_object_key))

    @staticmethod
    def _download_gcs(uri, temp_dir: str):
        try:
            storage_client = storage.Client()
        except exceptions.DefaultCredentialsError:
            storage_client = storage.Client.create_anonymous_client()
        bucket_args = uri.replace(_GCS_PREFIX, "", 1).split("/", 1)
        bucket_name = bucket_args[0]
        bucket_path = bucket_args[1] if len(bucket_args) > 1 else ""
        bucket = storage_client.bucket(bucket_name)
        blobs = bucket.list_blobs(prefix=bucket_path)
        for blob in blobs:
            # Replace any prefix from the object key with temp_dir
            subdir_object_key = blob.name.replace(bucket_path, "", 1).strip("/")
            # Create necessary subdirectory to store the object locally
            if "/" in subdir_object_key:
                local_object_dir = os.path.join(temp_dir, subdir_object_key.rsplit("/", 1)[0])
                if not os.path.isdir(local_object_dir):
                    os.makedirs(local_object_dir, exist_ok=True)
            blob.download_to_filename(os.path.join(temp_dir, subdir_object_key))

    @staticmethod
    def _download_local(uri):
        local_path = uri.replace(_LOCAL_PREFIX, "", 1)
        if not os.path.exists(local_path):
            raise Exception("Local path %s does not exist." % (uri))
        return local_path

    @staticmethod
    def _create_minio_client():
        # Remove possible http scheme for Minio
        url = re.compile(r"https?://")
        minioClient = Minio(url.sub("", os.getenv("S3_ENDPOINT", "")),
                            access_key=os.getenv("AWS_ACCESS_KEY_ID", ""),
                            secret_key=os.getenv("AWS_SECRET_ACCESS_KEY", ""),
                            secure=True)
        return minioClient
