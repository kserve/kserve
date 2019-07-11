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

import glob
import logging
import tempfile
import os
import re
from azure.storage.blob import BlockBlobService
from google.auth import exceptions
from google.cloud import storage
from minio import Minio

_GCS_PREFIX = "gs://"
_S3_PREFIX = "s3://"
_BLOB_RE = "https://(.+?).blob.core.windows.net/(.+)"
_LOCAL_PREFIX = "file://"


class Storage(object): # pylint: disable=too-few-public-methods
    @staticmethod
    def download(uri: str, out_dir: str = None) -> str:
        logging.info("Copying contents of %s to local", uri)

        is_local = False
        if uri.startswith(_LOCAL_PREFIX) or os.path.exists(uri):
            is_local = True

        if out_dir is None:
            if is_local:
                # noop if out_dir is not set and the path is local
                return Storage._download_local(uri)
            out_dir = tempfile.mkdtemp()

        if uri.startswith(_GCS_PREFIX):
            Storage._download_gcs(uri, out_dir)
        elif uri.startswith(_S3_PREFIX):
            Storage._download_s3(uri, out_dir)
        elif re.search(_BLOB_RE, uri):
            Storage._download_blob(uri, out_dir)
        elif is_local:
            return Storage._download_local(uri, out_dir)
        else:
            raise Exception("Cannot recognize storage type for " + uri +
                            "\n'%s', '%s', and '%s' are the current available storage type." %
                            (_GCS_PREFIX, _S3_PREFIX, _LOCAL_PREFIX))

        logging.info("Successfully copied %s to %s", uri, out_dir)
        return out_dir

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
        prefix = bucket_path
        if not prefix.endswith("/"):
            prefix = prefix + "/"
        blobs = bucket.list_blobs(prefix=prefix)
        for blob in blobs:
            # Replace any prefix from the object key with temp_dir
            subdir_object_key = blob.name.replace(bucket_path, "", 1).strip("/")

            # Create necessary subdirectory to store the object locally
            if "/" in subdir_object_key:
                local_object_dir = os.path.join(temp_dir, subdir_object_key.rsplit("/", 1)[0])
                if not os.path.isdir(local_object_dir):
                    os.makedirs(local_object_dir, exist_ok=True)
            if subdir_object_key.strip() != "":
                dest_path = os.path.join(temp_dir, subdir_object_key)
                logging.info("Downloading: %s", dest_path)
                blob.download_to_filename(dest_path)

    @staticmethod
    def _download_blob(uri, out_dir: str):
        match = re.search(_BLOB_RE, uri)
        account_name = match.group(1)
        storage_url = match.group(2)
        container_name, prefix = storage_url.split("/", 1)

        logging.info("Connecting to BLOB account: %s, contianer: %s", account_name, container_name)
        block_blob_service = BlockBlobService(account_name=account_name)
        blobs = block_blob_service.list_blobs(container_name, prefix=prefix)

        for blob in blobs:
            if "/" in blob.name:
                head, _ = os.path.split(blob.name)
                dir_path = os.path.join(out_dir, head)
                if not os.path.isdir(dir_path):
                    os.makedirs(dir_path)

            dest_path = os.path.join(out_dir, blob.name)
            logging.info("Downloading: %s", dest_path)
            block_blob_service.get_blob_to_path(container_name, blob.name, dest_path)

    @staticmethod
    def _download_local(uri, out_dir=None):
        local_path = uri.replace(_LOCAL_PREFIX, "", 1)
        if not os.path.exists(local_path):
            raise Exception("Local path %s does not exist." % (uri))

        if out_dir is None:
            return local_path
        elif not os.path.isdir(out_dir):
            os.makedirs(out_dir)

        if os.path.isdir(local_path):
            local_path = os.path.join(local_path, "*")

        for src in glob.glob(local_path):
            _, tail = os.path.split(src)
            dest_path = os.path.join(out_dir, tail)
            logging.info("Linking: %s to %s", src, dest_path)
            os.symlink(src, dest_path)
        return out_dir

    @staticmethod
    def _create_minio_client():
        # Remove possible http scheme for Minio
        url = re.compile(r"https?://")
        minioClient = Minio(url.sub("", os.getenv("S3_ENDPOINT", "")),
                            access_key=os.getenv("AWS_ACCESS_KEY_ID", ""),
                            secret_key=os.getenv("AWS_SECRET_ACCESS_KEY", ""),
                            secure=True)
        return minioClient
