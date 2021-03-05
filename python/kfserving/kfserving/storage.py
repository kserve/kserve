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

import glob
import logging
import tempfile
import mimetypes
import os
import re
import json
import shutil
import tarfile
import zipfile
import gzip
from urllib.parse import urlparse
import requests 
from azure.storage.blob import BlockBlobService
from google.auth import exceptions
from google.cloud import storage
from minio import Minio
from kfserving.kfmodel_repository import MODEL_MOUNT_DIRS

_GCS_PREFIX = "gs://"
_S3_PREFIX = "s3://"
_BLOB_RE = "https://(.+?).blob.core.windows.net/(.+)"
_LOCAL_PREFIX = "file://"
_URI_RE = "https?://(.+)/(.+)"
_HTTP_PREFIX = "http(s)://"
_HEADERS_SUFFIX = "-headers"

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
    def _download_s3(uri, temp_dir: str):
        client = Storage._create_minio_client()
        bucket_args = uri.replace(_S3_PREFIX, "", 1).split("/", 1)
        bucket_name = bucket_args[0]
        bucket_path = bucket_args[1] if len(bucket_args) > 1 else ""
        objects = client.list_objects(bucket_name, prefix=bucket_path, recursive=True)
        count = 0
        for obj in objects:
            # Replace any prefix from the object key with temp_dir
            subdir_object_key = obj.object_name.replace(bucket_path, "", 1).strip("/")
            # fget_object handles directory creation if does not exist
            if not obj.is_dir:
                if subdir_object_key == "":
                    subdir_object_key = obj.object_name
                client.fget_object(bucket_name, obj.object_name,
                                   os.path.join(temp_dir, subdir_object_key))
            count = count + 1
        if count == 0:
            raise RuntimeError("Failed to fetch model. \
The path or model %s does not exist." % (uri))

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
            raise RuntimeError("Failed to fetch model. \
The path or model %s does not exist." % (uri))

    @staticmethod
    def _download_blob(uri, out_dir: str): # pylint: disable=too-many-locals
        match = re.search(_BLOB_RE, uri)
        account_name = match.group(1)
        storage_url = match.group(2)
        container_name, prefix = storage_url.split("/", 1)

        logging.info("Connecting to BLOB account: [%s], container: [%s], prefix: [%s]",
                     account_name,
                     container_name,
                     prefix)
        try:
            block_blob_service = BlockBlobService(account_name=account_name)
            blobs = block_blob_service.list_blobs(container_name, prefix=prefix)
        except Exception: # pylint: disable=broad-except
            token = Storage._get_azure_storage_token()
            if token is None:
                logging.warning("Azure credentials not found, retrying anonymous access")
            block_blob_service = BlockBlobService(account_name=account_name, token_credential=token)
            blobs = block_blob_service.list_blobs(container_name, prefix=prefix)
        count = 0
        for blob in blobs:
            dest_path = os.path.join(out_dir, blob.name)
            if "/" in blob.name:
                head, tail = os.path.split(blob.name)
                if prefix is not None:
                    head = head[len(prefix):]
                if head.startswith('/'):
                    head = head[1:]
                dir_path = os.path.join(out_dir, head)
                dest_path = os.path.join(dir_path, tail)
                if not os.path.isdir(dir_path):
                    os.makedirs(dir_path)

            logging.info("Downloading: %s to %s", blob.name, dest_path)
            block_blob_service.get_blob_to_path(container_name, blob.name, dest_path)
            count = count + 1
        if count == 0:
            raise RuntimeError("Failed to fetch model. \
The path or model %s does not exist." % (uri))

    @staticmethod
    def _get_azure_storage_token():
        tenant_id = os.getenv("AZ_TENANT_ID", "")
        client_id = os.getenv("AZ_CLIENT_ID", "")
        client_secret = os.getenv("AZ_CLIENT_SECRET", "")
        subscription_id = os.getenv("AZ_SUBSCRIPTION_ID", "")

        if tenant_id == "" or client_id == "" or client_secret == "" or subscription_id == "":
            return None

        # note the SP must have "Storage Blob Data Owner" perms for this to work
        import adal
        from azure.storage.common import TokenCredential

        authority_url = "https://login.microsoftonline.com/" + tenant_id

        context = adal.AuthenticationContext(authority_url)

        token = context.acquire_token_with_client_credentials(
            "https://storage.azure.com/",
            client_id,
            client_secret)

        token_credential = TokenCredential(token["accessToken"])

        logging.info("Retrieved SP token credential for client_id: %s", client_id)

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

        for src in glob.glob(local_path):
            _, tail = os.path.split(src)
            dest_path = os.path.join(out_dir, tail)
            logging.info("Linking: %s to %s", src, dest_path)
            os.symlink(src, dest_path)
        return out_dir

    @staticmethod
    def _download_from_uri(uri, out_dir=None):
        url = urlparse(uri)
        filename = os.path.basename(url.path)
        mimetype, encoding = mimetypes.guess_type(uri)
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
            if mimetype == 'application/zip' and not response.headers.get('Content-Type', '').startswith('application/zip'):
                raise RuntimeError("URI: %s did not respond with \'Content-Type\': \'application/zip\'" % (uri))
            if mimetype == 'application/x-tar' and not response.headers.get('Content-Type', '').startswith('application/x-tar'):
                raise RuntimeError("URI: %s did not respond with \'Content-Type\': \'application/x-tar\'" % (uri))
            if (mimetype != 'application/zip' and mimetype != 'application/x-tar') and not response.headers.get('Content-Type', '').startswith('application/octet-stream'):
                raise RuntimeError("URI: %s did not respond with \'Content-Type\': \'application/octet-stream\'" % (uri))

            if encoding == 'gzip':
                stream = gzip.GzipFile(fileobj=response.raw)
                local_path = os.path.join(out_dir, f'{filename}.tar')
            else:
                stream = response.raw
            with open(local_path, 'wb') as out:
                shutil.copyfileobj(stream, out)
        
        if mimetype in ["application/x-tar", "application/zip"]:
            if mimetype == "application/x-tar":
                archive = tarfile.open(local_path, 'r', encoding='utf-8')
            else:
                archive = zipfile.ZipFile(local_path, 'r')
            archive.extractall(out_dir)
            archive.close()
            os.remove(local_path)

        return out_dir

    @staticmethod
    def _create_minio_client():
        # Adding prefixing "http" in urlparse is necessary for it to be the netloc
        url = urlparse(os.getenv("AWS_ENDPOINT_URL", "http://s3.amazonaws.com"))
        use_ssl = url.scheme == 'https' if url.scheme else bool(os.getenv("S3_USE_HTTPS", "true"))
        return Minio(url.netloc,
                     access_key=os.getenv("AWS_ACCESS_KEY_ID", ""),
                     secret_key=os.getenv("AWS_SECRET_ACCESS_KEY", ""),
                     region=os.getenv("AWS_REGION", ""),
                     secure=use_ssl)
