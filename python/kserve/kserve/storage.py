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

import glob
import gzip
import logging
import mimetypes
import os
import re
import json
import shutil
import tarfile
import tempfile
import zipfile
from urllib.parse import urlparse
import requests
from pathlib import Path
from azure.storage.blob import BlobServiceClient
from azure.storage.blob._list_blobs_helper import BlobPrefix

from botocore.client import Config
from botocore import UNSIGNED
import boto3
from google.auth import exceptions
from google.cloud import storage

from kserve.model_repository import MODEL_MOUNT_DIRS

_GCS_PREFIX = "gs://"
_S3_PREFIX = "s3://"
_BLOB_RE = "https://(.+?).blob.core.windows.net/(.+)"
_ACCOUNT_RE = "https://(.+?).blob.core.windows.net"
_LOCAL_PREFIX = "file://"
_URI_RE = "https?://(.+)/(.+)"
_HTTP_PREFIX = "http(s)://"
_HEADERS_SUFFIX = "-headers"
_PVC_PREFIX = "/mnt/pvc"


class Storage(object):  # pylint: disable=too-few-public-methods
    @staticmethod
    def download(uri: str, out_dir: str = None) -> str:
        logging.info("Copying contents of %s to local", uri)

        if uri.startswith(_PVC_PREFIX) and not os.path.exists(uri):
            raise Exception(f"Cannot locate source uri {uri} for PVC")

        is_local = False
        if uri.startswith(_LOCAL_PREFIX) or os.path.exists(uri):
            is_local = True

        if out_dir is None:
            if is_local:
                # noop if out_dir is not set and the path is local
                return Storage._download_local(uri)
            out_dir = tempfile.mkdtemp()
        elif not os.path.exists(out_dir):
            os.mkdir(out_dir)

        if uri.startswith(_GCS_PREFIX):
            Storage._download_gcs(uri, out_dir)
        elif uri.startswith(_S3_PREFIX):
            Storage._download_s3(uri, out_dir)
        elif re.search(_BLOB_RE, uri):
            Storage._download_blob(uri, out_dir)
        elif is_local:
            return Storage._download_local(uri, out_dir)
        elif re.search(_URI_RE, uri):
            return Storage._download_from_uri(uri, out_dir)
        elif uri.startswith(MODEL_MOUNT_DIRS):
            # Don't need to download models if this InferenceService is running in the multi-model
            # serving mode. The model agent will download models.
            return out_dir
        else:
            raise Exception("Cannot recognize storage type for " + uri +
                            "\n'%s', '%s', '%s', and '%s' are the current available storage type." %
                            (_GCS_PREFIX, _S3_PREFIX, _LOCAL_PREFIX, _HTTP_PREFIX))

        logging.info("Successfully copied %s to %s", uri, out_dir)
        return out_dir

    @staticmethod
    def get_S3_config():
        # anon environment variable defined in s3_secret.go
        anon = ("True" == os.getenv("awsAnonymousCredential", "false").capitalize())
        if anon:
            return Config(signature_version=UNSIGNED)
        else:
            return None

    @staticmethod
    def _download_s3(uri, temp_dir: str):
        # Boto3 looks at various configuration locations until it finds configuration values.
        # lookup order:
        # 1. Config object passed in as the config parameter when creating S3 resource
        #    if awsAnonymousCredential env var true, passed in via config
        # 2. Environment variables
        # 3. ~/.aws/config file
        s3 = boto3.resource('s3',
                            endpoint_url=os.getenv("AWS_ENDPOINT_URL", "http://s3.amazonaws.com"),
                            config=Storage.get_S3_config())
        parsed = urlparse(uri, scheme='s3')
        bucket_name = parsed.netloc
        bucket_path = parsed.path.lstrip('/')

        count = 0
        bucket = s3.Bucket(bucket_name)
        for obj in bucket.objects.filter(Prefix=bucket_path):
            # Skip where boto3 lists the directory as an object
            if obj.key.endswith("/"):
                continue
            # In the case where bucket_path points to a single object, set the target key to bucket_path
            # Otherwise, remove the bucket_path prefix, strip any extra slashes, then prepend the target_dir
            # Example:
            # s3://test-bucket
            # Objects: /a/b/c/model.bin /a/model.bin /model.bin
            #
            # If 'uri' is set to "s3://test-bucket", then the downloader will
            # download all the objects listed above, re-creating their subpaths
            # under the temp_dir.
            # If 'uri' is set to "s3://test-bucket/a", then the downloader will
            # add to temp_dir: b/c/model.bin and model.bin.
            # If 'uri' is set to "s3://test-bucket/a/b/c/model.bin", then
            # the downloader will add to temp dir: model.bin
            # (without any subpaths).
            target_key = (
                obj.key.rsplit("/", 1)[-1]
                if bucket_path == obj.key
                else obj.key.replace(bucket_path, "", 1).lstrip("/")
            )
            target = f"{temp_dir}/{target_key}"
            if not os.path.exists(os.path.dirname(target)):
                os.makedirs(os.path.dirname(target), exist_ok=True)
            bucket.download_file(obj.key, target)
            logging.info('Downloaded object %s to %s' % (obj.key, target))
            count = count + 1
        if count == 0:
            raise RuntimeError(
                "Failed to fetch model. No model found in %s." % bucket_path)

        # Unpack compressed file, supports .tgz, tar.gz and zip file formats.
        if count == 1:
            mimetype, _ = mimetypes.guess_type(target)
            if mimetype in ["application/x-tar", "application/zip"]:
                Storage._unpack_archive_file(target, mimetype, temp_dir)

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
        count = 0
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
            count = count + 1
        if count == 0:
            raise RuntimeError(
                "Failed to fetch model. No model found in %s." % uri)

        # Unpack compressed file, supports .tgz, tar.gz and zip file formats.
        if count == 1:
            mimetype, _ = mimetypes.guess_type(blob.name)
            if mimetype in ["application/x-tar", "application/zip"]:
                Storage._unpack_archive_file(dest_path, mimetype, temp_dir)

    @staticmethod
    def _download_blob(uri, out_dir: str):  # pylint: disable=too-many-locals
        match = re.search(_BLOB_RE, uri)
        account_url = re.search(_ACCOUNT_RE, uri).group(0)
        account_name = match.group(1)
        storage_url = match.group(2)
        container_name, prefix = storage_url.split("/", 1)

        logging.info("Connecting to BLOB account: [%s], container: [%s], prefix: [%s]",
                     account_name,
                     container_name,
                     prefix)
        token = Storage._get_azure_storage_token()
        if token is None:
            logging.warning("Azure credentials not found, retrying anonymous access")
        blob_service_client = BlobServiceClient(account_url, credential=token)
        container_client = blob_service_client.get_container_client(container_name)
        count = 0
        blobs = []
        max_depth = 5
        stack = [(prefix, max_depth)]
        while stack:
            curr_prefix, depth = stack.pop()
            if depth < 0:
                continue
            for item in container_client.walk_blobs(
                            name_starts_with=curr_prefix):
                if isinstance(item, BlobPrefix):
                    stack.append((item.name, depth - 1))
                else:
                    blobs += container_client.list_blobs(name_starts_with=item.name,
                                                         include=['snapshots'])
        for blob in set(blobs):
            dest_path = os.path.join(out_dir, blob.name.replace(prefix, "", 1).lstrip("/"))
            Path(os.path.dirname(dest_path)).mkdir(parents=True, exist_ok=True)
            logging.info("Downloading: %s to %s", blob.name, dest_path)
            downloader = container_client.download_blob(blob.name)
            with open(dest_path, "wb+") as f:
                f.write(downloader.readall())
            count = count + 1
        if count == 0:
            raise RuntimeError(
                "Failed to fetch model. No model found in %s." % (uri))

        # Unpack compressed file, supports .tgz, tar.gz and zip file formats.
        if count == 1:
            mimetype, _ = mimetypes.guess_type(dest_path)
            if mimetype in ["application/x-tar", "application/zip"]:
                Storage._unpack_archive_file(dest_path, mimetype, out_dir)

    @staticmethod
    def _get_azure_storage_token():
        tenant_id = os.getenv("AZ_TENANT_ID", "")
        client_id = os.getenv("AZ_CLIENT_ID", "")
        client_secret = os.getenv("AZ_CLIENT_SECRET", "")
        subscription_id = os.getenv("AZ_SUBSCRIPTION_ID", "")

        if tenant_id == "" or client_id == "" or client_secret == "" or subscription_id == "":
            return None

        # note the SP must have "Storage Blob Data Owner" perms for this to work
        from azure.identity import ClientSecretCredential
        token_credential = ClientSecretCredential(tenant_id,
                                                  client_id, client_secret)

        logging.info("Retrieved SP token credential for client_id: %s",
                     client_id)
        return token_credential

    @staticmethod
    def _download_local(uri, out_dir=None):
        local_path = uri.replace(_LOCAL_PREFIX, "", 1)
        if not os.path.exists(local_path):
            raise RuntimeError("Local path %s does not exist." % (uri))

        if out_dir is None:
            return local_path
        elif not os.path.isdir(out_dir):
            os.makedirs(out_dir)

        if os.path.isdir(local_path):
            local_path = os.path.join(local_path, "*")

        count = 0
        for src in glob.glob(local_path):
            _, tail = os.path.split(src)
            dest_path = os.path.join(out_dir, tail)
            logging.info("Linking: %s to %s", src, dest_path)
            os.symlink(src, dest_path)
            count = count + 1
        if count == 0:
            raise RuntimeError(
                "Failed to fetch model. No model found in %s." % (uri))
        # Unpack compressed file, supports .tgz, tar.gz and zip file formats.
        if count == 1:
            mimetype, _ = mimetypes.guess_type(dest_path)
            if mimetype in ["application/x-tar", "application/zip"]:
                Storage._unpack_archive_file(dest_path, mimetype, out_dir)

        return out_dir

    @staticmethod
    def _download_from_uri(uri, out_dir=None):
        url = urlparse(uri)
        filename = os.path.basename(url.path)
        mimetype, encoding = mimetypes.guess_type(url.path)
        local_path = os.path.join(out_dir, filename)

        if filename == '':
            raise ValueError('No filename contained in URI: %s' % (uri))

        # Get header information from host url
        headers = {}
        host_uri = url.hostname

        headers_json = os.getenv(host_uri + _HEADERS_SUFFIX, "{}")
        headers = json.loads(headers_json)

        with requests.get(uri, stream=True, headers=headers) as response:
            if response.status_code != 200:
                raise RuntimeError("URI: %s returned a %s response code." % (uri, response.status_code))
            zip_content_types = ('application/x-zip-compressed', 'application/zip', 'application/zip-compressed')
            if mimetype == 'application/zip' and not response.headers.get('Content-Type', '')\
                    .startswith(zip_content_types):
                raise RuntimeError("URI: %s did not respond with any of following \'Content-Type\': " % uri +
                                   ", ".join(zip_content_types))
            tar_content_types = ('application/x-tar', 'application/x-gtar', 'application/x-gzip', 'application/gzip')
            if mimetype == 'application/x-tar' and not response.headers.get('Content-Type', '')\
                    .startswith(tar_content_types):
                raise RuntimeError("URI: %s did not respond with any of following \'Content-Type\': " % uri +
                                   ", ".join(tar_content_types))
            if (mimetype != 'application/zip' and mimetype != 'application/x-tar') and \
                    not response.headers.get('Content-Type', '').startswith('application/octet-stream'):
                raise RuntimeError("URI: %s did not respond with \'Content-Type\': \'application/octet-stream\'"
                                   % uri)

            if encoding == 'gzip':
                stream = gzip.GzipFile(fileobj=response.raw)
                local_path = os.path.join(out_dir, f'{filename}.tar')
            else:
                stream = response.raw
            with open(local_path, 'wb') as out:
                shutil.copyfileobj(stream, out)

        if mimetype in ["application/x-tar", "application/zip"]:
            Storage._unpack_archive_file(local_path, mimetype, out_dir)

        return out_dir

    @staticmethod
    def _unpack_archive_file(file_path, mimetype, target_dir=None):
        if not target_dir:
            target_dir = os.path.dirname(file_path)

        try:
            logging.info("Unpacking: %s", file_path)
            if mimetype == "application/x-tar":
                archive = tarfile.open(file_path, 'r', encoding='utf-8')
            else:
                archive = zipfile.ZipFile(file_path, 'r')
            archive.extractall(target_dir)
            archive.close()
        except (tarfile.TarError, zipfile.BadZipfile):
            raise RuntimeError("Failed to unpack archive file. \
The file format is not valid.")
        os.remove(file_path)
